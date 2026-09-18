package media

import (
	"context"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"net/http"
	"strings"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"

	"github.com/stretchr/testify/require"
)

// fakeBillingRepo 记录三个冻结原语与通用 Apply 的调用参数。
type fakeBillingRepo struct {
	reserved []*BatchImageBalanceHoldCommand
	captured []*BatchImageBalanceHoldCommand
	released []*BatchImageBalanceHoldCommand
	applied  []*UsageBillingCommand

	reserveErr error
	captureErr error
	releaseErr error
}

func (f *fakeBillingRepo) Apply(_ context.Context, cmd *UsageBillingCommand) (*UsageBillingApplyResult, error) {
	f.applied = append(f.applied, cmd)
	return &UsageBillingApplyResult{Applied: true}, nil
}

func (f *fakeBillingRepo) ReserveBatchImageBalance(_ context.Context, cmd *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error) {
	if f.reserveErr != nil {
		return nil, f.reserveErr
	}
	f.reserved = append(f.reserved, cmd)
	return &BatchImageBalanceHoldResult{}, nil
}

func (f *fakeBillingRepo) CaptureBatchImageBalance(_ context.Context, cmd *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error) {
	if f.captureErr != nil {
		return nil, f.captureErr
	}
	f.captured = append(f.captured, cmd)
	return &BatchImageBalanceHoldResult{}, nil
}

func (f *fakeBillingRepo) ReleaseBatchImageBalance(_ context.Context, cmd *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error) {
	if f.releaseErr != nil {
		return nil, f.releaseErr
	}
	f.released = append(f.released, cmd)
	return &BatchImageBalanceHoldResult{}, nil
}

// newBillingEnv 只装配扣费所需的最小依赖；单测注入成功的用量写入器，
// 用量行字段直接测 buildUsageLog，落库失败恢复另由生命周期测试覆盖。
func newBillingEnv(t *testing.T) (*MediaTaskBilling, *fakeBillingRepo) {
	t.Helper()
	repo := &fakeBillingRepo{}
	gw := service.NewGatewayForMediaBillingTest(repo)
	b := NewMediaTaskBilling(gw, nil)
	b.usageWriter = func(context.Context, *UsageLog) error { return nil }
	return b, repo
}

func billingFixtures() (*MediaTaskRecord, *APIKey, *Account) {
	groupID := int64(9)
	task := &MediaTaskRecord{
		ID:              "task-1",
		UserID:          10,
		APIKeyID:        100,
		AccountID:       7,
		Model:           SeedanceModel25,
		DurationSeconds: 30,
		Resolution:      SeedanceResolution720,
		UnitPrice:       2.0,
	}
	apiKey := &APIKey{ID: 100, UserID: 10, GroupID: &groupID,
		Group: &Group{ID: 9, Platform: PlatformSeedance, RateMultiplier: 1}}
	account := &Account{ID: 7, Platform: PlatformSeedance, Type: AccountTypeAPIKey}
	return task, apiKey, account
}

// newPricingBillingEnv 在计费仓储之外再装配一个真实的定价解析器，用于验证
// 「按真实模型解析、查不到回落通用 id」的分模型定价。billingService 必须非
// nil：未单独配价的模型在回落解析时会触碰 GetModelPricing（nil 会 panic）；
// channelService 留 nil，分组定价直接读 Group.ModelPricing 不经过它。
func newPricingBillingEnv(t *testing.T) (*MediaTaskBilling, *fakeBillingRepo) {
	t.Helper()
	repo := &fakeBillingRepo{}
	resolver := service.NewModelPricingResolver(nil, service.NewBillingService(nil, nil))
	gw := service.NewGatewayForMediaPricingTest(repo, resolver)
	b := NewMediaTaskBilling(gw, nil)
	b.usageWriter = func(context.Context, *UsageLog) error { return nil }
	return b, repo
}

// TestMediaBillingReserveResolvesPerModelPriceThenFallsBack 覆盖两级定价解析：
// 单独配价的模型走模型级价，未配价的模型回落到通用兜底 id `seedance` 的价。
func TestMediaBillingReserveResolvesPerModelPriceThenFallsBack(t *testing.T) {
	price := func(v float64) *float64 { return &v }
	group := &Group{
		ID: 9, Platform: PlatformSeedance, RateMultiplier: 1,
		ModelPricing: []service.ChannelModelPricing{
			// seedance2.5 单独配价 5；通用 id seedance 兜底价 2。
			{Platform: "seedance", Models: []string{SeedanceModel25}, BillingMode: service.BillingModePerRequest, PerRequestPrice: price(5)},
			{Platform: "seedance", Models: []string{mediaBillingModel}, BillingMode: service.BillingModePerRequest, PerRequestPrice: price(2)},
		},
	}
	billing, repo := newPricingBillingEnv(t)
	apiKey := &APIKey{ID: 100, UserID: 10, GroupID: &group.ID, Group: group}
	account := &Account{ID: 7, Platform: PlatformSeedance, Type: AccountTypeAPIKey}

	// 单独配价的模型走模型级价 5。
	task25 := &MediaTaskRecord{ID: "t-25", UserID: 10, APIKeyID: 100, AccountID: 7, Model: SeedanceModel25}
	got25, err := billing.Reserve(context.Background(), task25, apiKey, nil, account)
	require.NoError(t, err)
	require.EqualValues(t, 5, got25)

	// 未单独配价的模型回落到通用 id seedance 的兜底价 2。
	task20 := &MediaTaskRecord{ID: "t-20", UserID: 10, APIKeyID: 100, AccountID: 7, Model: SeedanceModel20}
	got20, err := billing.Reserve(context.Background(), task20, apiKey, nil, account)
	require.NoError(t, err)
	require.EqualValues(t, 2, got20)

	// 冻结额与解析出的单价一致，且模型级价先于兜底价命中。
	require.Len(t, repo.reserved, 2)
	require.EqualValues(t, 5, repo.reserved[0].HoldAmount)
	require.EqualValues(t, 2, repo.reserved[1].HoldAmount)
}

func TestMediaBillingReserveRejectsSubscriptionGroups(t *testing.T) {
	billing, repo := newBillingEnv(t)
	task, apiKey, account := billingFixtures()
	apiKey.Group.SubscriptionType = SubscriptionTypeSubscription

	_, err := billing.Reserve(context.Background(), task, apiKey, nil, account)

	// 订阅额度没有冻结语义，静默按余额扣等于对已付费用户双重收费。
	require.Error(t, err)
	require.Equal(t, "MEDIA_SUBSCRIPTION_UNSUPPORTED", infraerrors.Reason(err))
	require.Empty(t, repo.reserved, "订阅护栏必须在冻结之前生效")
}

func TestMediaBillingReserveFailsClosedWithoutConfiguredPrice(t *testing.T) {
	billing, repo := newBillingEnv(t)
	task, apiKey, account := billingFixtures()

	task.UnitPrice = 0
	// resolver 为 nil ⇒ 解析不到价格。绝不能按 0 元放行出片。
	_, err := billing.Reserve(context.Background(), task, apiKey, nil, account)
	require.Error(t, err)
	require.Equal(t, "MEDIA_PRICE_NOT_CONFIGURED", infraerrors.Reason(err))
	require.Equal(t, http.StatusServiceUnavailable, infraerrors.Code(err))
	require.Empty(t, repo.reserved)
}

func TestMediaBillingCaptureUsesSnapshotPriceAndWritesVideoColumns(t *testing.T) {
	billing, repo := newBillingEnv(t)
	task, apiKey, account := billingFixtures()

	require.NoError(t, billing.Capture(context.Background(), task, apiKey, nil, account))

	require.Len(t, repo.captured, 1)
	cap := repo.captured[0]
	// 用创建时的单价快照，实扣不超过冻结额。
	require.EqualValues(t, 2.0, cap.HoldAmount)
	require.EqualValues(t, 2.0, cap.ActualAmount)

	log := billing.buildUsageLog(task, apiKey, account, 2.0)
	require.Equal(t, string(BillingModeVideo), *log.BillingMode)
	require.EqualValues(t, 2.0, log.ActualCost)
	require.Equal(t, 1, log.VideoCount)
	// 30 秒真实落库，不被 VideoBillingMaxDurationSeconds 的 15 秒上限截半。
	require.NotNil(t, log.VideoDurationSeconds)
	require.Equal(t, 30, *log.VideoDurationSeconds)
	require.NotNil(t, log.VideoResolution)
	require.Equal(t, SeedanceResolution720, *log.VideoResolution)
	// 记账模型是真实模型，与计价用的固定 id 不同。
	require.Equal(t, SeedanceModel25, log.Model)
}

func TestMediaBillingCaptureNeverDeductsBalanceTwice(t *testing.T) {
	billing, repo := newBillingEnv(t)
	task, apiKey, account := billingFixtures()
	apiKey.Quota = 100 // 触发 API Key 配额维度，确保 Apply 会被调用

	require.NoError(t, billing.Capture(context.Background(), task, apiKey, nil, account))

	require.Len(t, repo.applied, 1)
	cmd := repo.applied[0]
	// 余额已由 Capture 扣过，Apply 只能联动非余额维度。
	require.Zero(t, cmd.BalanceCost, "BalanceCost 必须为 0，否则与冻结机制重复扣费")
	require.Zero(t, cmd.SubscriptionCost)
	require.EqualValues(t, 2.0, cmd.APIKeyQuotaCost, "配额维度不能因 BalanceCost 归零而一起失效")
}

func TestMediaBillingSkipsApplyWhenNoNonBalanceDimensions(t *testing.T) {
	billing, repo := newBillingEnv(t)
	task, apiKey, account := billingFixtures()
	// 无配额、无限流、账号无配额限制 ⇒ 没有任何非余额维度需要联动。
	require.NoError(t, billing.Capture(context.Background(), task, apiKey, nil, account))
	require.Empty(t, repo.applied, "三个维度都为 0 时不应产生空扣费记录")
}

func TestMediaBillingHoldRequestIDsUseExistingHelpers(t *testing.T) {
	billing, repo := newBillingEnv(t)
	task, apiKey, _ := billingFixtures()

	require.NoError(t, billing.Release(context.Background(), task, apiKey))
	require.Len(t, repo.released, 1)

	batchID := mediaHoldBatchID(task.ID)
	require.Equal(t, "media:task-1", batchID)
	// Release 内部会用 BatchImageHoldRequestID(cmd.BatchID) 反查 hold claim，
	// 三个 RequestID 必须由既有助手生成；自定义前缀会让释放被静默跳过、
	// 冻结额永久退不回给用户。
	require.Equal(t, BatchImageReleaseRequestID(batchID), repo.released[0].RequestID)
	require.True(t, strings.HasPrefix(BatchImageHoldRequestID(batchID), "batch_image_hold:"))
	require.Equal(t, batchID, repo.released[0].BatchID)
}

func TestMediaBillingReleaseIsFullRefund(t *testing.T) {
	billing, repo := newBillingEnv(t)
	task, apiKey, _ := billingFixtures()

	require.NoError(t, billing.Release(context.Background(), task, apiKey))
	require.Len(t, repo.released, 1)
	// 全额退回，不留手续费。
	require.EqualValues(t, 2.0, repo.released[0].HoldAmount)
}

func TestMediaBillingFingerprintConflictNeedsReconciliation(t *testing.T) {
	billing, repo := newBillingEnv(t)
	task, apiKey, account := billingFixtures()

	repo.captureErr = ErrUsageBillingRequestConflict
	require.Error(t, billing.Capture(context.Background(), task, apiKey, nil, account))

	repo.releaseErr = ErrUsageBillingRequestConflict
	require.Error(t, billing.Release(context.Background(), task, apiKey))
}

func TestMediaBillingErrorsAreMappedAwayFromBatchImageSemantics(t *testing.T) {
	cases := []struct {
		in       error
		reason   string
		httpCode int
	}{
		{ErrBatchImageInsufficientBalance, "MEDIA_INSUFFICIENT_BALANCE", http.StatusPaymentRequired},
		{ErrBatchImageSettlementCostExceedsHold, "MEDIA_SETTLEMENT_EXCEEDS_HOLD", http.StatusInternalServerError},
		{errors.New("boom"), "MEDIA_BILLING_UNAVAILABLE", http.StatusServiceUnavailable},
	}
	for _, tc := range cases {
		out := mapMediaBillingError(tc.in)
		require.Equal(t, tc.reason, infraerrors.Reason(out))
		require.Equal(t, tc.httpCode, infraerrors.Code(out))
		// 对外错误体不得出现批量图片语义。
		require.NotContains(t, strings.ToLower(infraerrors.Message(out)), "batch image")
	}
	require.NoError(t, mapMediaBillingError(nil))
}
