package media

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"go.uber.org/zap"
)

// 媒体任务使用原包两阶段计费：创建时冻结，确认成功且可下载时扣除，
// 确认拒绝或生成失败时退回。未知结果和任务年龄都不是退款依据。
// 异步生命周期单独编排，复用既有冻结与去重原语。
const (
	// mediaBillingModel 是计价用的兜底模型 id。
	//
	// 定价按「真实模型优先、此 id 兜底」两级解析（见 resolveUnitPrice）：
	// 管理端可为单个 Seedance 模型（seedance2.5 / 2.0fast 等）配差异化
	// per_request 价，未单独配价的模型回落到这一个 id 的通用价，因此不会
	// 漏配某个模型导致该模型被拒。usage_log.model 记真实模型；对账时若按真实
	// 模型查不到价，说明它走了兜底，需回落到此 id 再查。
	mediaBillingModel = "seedance"

	// mediaHoldBatchIDPrefix 复用既有余额冻结原语时的 BatchID 前缀。
	//
	// Reserve/Capture/Release 三个方法名带 BatchImage，但底层是 users.balance
	// 与 frozen_balance 的通用两阶段扣费，与批量图片业务无耦合。前缀保证与真实
	// 批量作业的 BatchID 空间不相交。
	mediaHoldBatchIDPrefix = "media:"
)

// MediaTaskBilling 实现 MediaBilling。
//
// apiKeyService 单独注入而非从 GatewayService 取：后者没有该字段，而配额
// 耗尽后必须失效鉴权缓存，否则该 Key 在缓存 TTL 内仍可继续用。
type MediaTaskBilling struct {
	gateway       *GatewayService
	apiKeyService APIKeyQuotaUpdater
	usageWriter   func(context.Context, *UsageLog) error
}

func NewMediaTaskBilling(gateway *GatewayService, apiKeyService APIKeyQuotaUpdater) *MediaTaskBilling {
	return &MediaTaskBilling{gateway: gateway, apiKeyService: apiKeyService,
		usageWriter: func(ctx context.Context, log *UsageLog) error { return service.WriteMediaUsageLog(ctx, gateway, log) }}
}

func mediaHoldBatchID(taskID string) string {
	return mediaHoldBatchIDPrefix + strings.TrimSpace(taskID)
}

// resolveUnitPrice 从管理端配置的渠道/分组定价取按条单价。
//
// 两级解析：先按任务的真实模型（seedance2.5 / 2.0fast 等）查独立定价，查不到
// 再回落到通用计价 id "seedance"。这样管理端既能给单个模型配差异化 per_request
// 价，又不必为每个模型都配一遍——未单独配价的模型走通用兜底价。
//
// 解析不到一律 fail-closed 拒绝创建，绝不按 0 元或默认值放行出片：
// 免费出片的损失不可追回，而拒绝创建只需管理员补一条配置。
func (b *MediaTaskBilling) resolveUnitPrice(ctx context.Context, model string, apiKey *APIKey) (float64, error) {
	if b == nil || b.gateway == nil {
		return 0, ErrMediaTaskUnavailable
	}
	// 真实模型名与通用兜底 id 不同才先试模型级定价，避免对同一 id 查两遍。
	if model = strings.TrimSpace(model); model != "" && !strings.EqualFold(model, mediaBillingModel) {
		if price, ok := b.perRequestPriceFor(ctx, model, apiKey); ok {
			return price, nil
		}
	}
	if price, ok := b.perRequestPriceFor(ctx, mediaBillingModel, apiKey); ok {
		return price, nil
	}
	return 0, errMediaPriceMissing()
}

// perRequestPriceFor 取某个计价 id 的按条单价；未配置或非按条模式返回 (0,false)。
//
// Only an explicit fixed per-request price is supported; no tier or duration reinterpretation.
func (b *MediaTaskBilling) perRequestPriceFor(ctx context.Context, billingModel string, apiKey *APIKey) (float64, bool) {
	resolved := service.ResolveChannelPricingForMedia(b.gateway, ctx, billingModel, apiKey)
	if resolved == nil {
		return 0, false
	}
	if resolved.Mode == BillingModePerRequest && len(resolved.RequestTiers) == 0 && resolved.DefaultPerRequestPrice > 0 {
		return resolved.DefaultPerRequestPrice, true
	}
	return 0, false
}

func errMediaPriceMissing() error {
	return infraerrors.New(http.StatusServiceUnavailable, "MEDIA_PRICE_NOT_CONFIGURED",
		"per-request price for media tasks is not configured")
}

// Quote validates pricing before a durable intent or any balance operation.
func (b *MediaTaskBilling) Quote(ctx context.Context, model string, key *APIKey) (float64, error) {
	if key == nil || key.Group == nil {
		return 0, ErrMediaTaskUnavailable
	}
	if key.Group.IsSubscriptionType() {
		return 0, ErrMediaSubscriptionUnsupported
	}
	if key.Group.RateMultiplier != 1 {
		return 0, infraerrors.BadRequest("MEDIA_MULTIPLIER_UNSUPPORTED", "media per-request prices require group multiplier 1")
	}
	if b.gateway != nil && b.gateway.ResolveUserGroupRateMultiplier(ctx, key.UserID, key.Group.ID, 1) != 1 {
		return 0, infraerrors.BadRequest("MEDIA_MULTIPLIER_UNSUPPORTED", "Seedance final prices do not support custom user group multipliers")
	}
	price, err := b.resolveUnitPrice(ctx, model, key)
	if err != nil {
		return 0, err
	}
	price = service.QuantizeUsageBillingAmount(price)
	if math.IsNaN(price) || math.IsInf(price, 0) || price <= 0 {
		return 0, errMediaPriceMissing()
	}
	return price, nil
}

// Reserve 在向上游建单之前冻结余额，返回冻结所用的单价快照。
//
// 顺序上先解析价格、再做订阅护栏、最后冻结：三步都在建单之前，任一步失败
// 都可以安全地把错误返回给客户端。
func (b *MediaTaskBilling) Reserve(ctx context.Context, task *MediaTaskRecord, apiKey *APIKey, user *User, account *Account) (float64, error) {
	if b == nil || b.gateway == nil || task == nil || apiKey == nil {
		return 0, ErrMediaTaskUnavailable
	}

	// 订阅护栏：两阶段冻结只作用于 users.balance，订阅额度没有冻结语义。
	// 静默按余额扣等于对已付费的订阅用户双重收费，因此 fail-closed 拒绝。
	if apiKey.Group != nil && apiKey.Group.IsSubscriptionType() {
		return 0, ErrMediaSubscriptionUnsupported
	}

	unitPrice := task.UnitPrice
	if unitPrice == 0 {
		var err error
		unitPrice, err = b.Quote(ctx, task.Model, apiKey)
		if err != nil {
			return 0, err
		}
	}
	unitPrice = service.QuantizeUsageBillingAmount(unitPrice)
	if math.IsNaN(unitPrice) || math.IsInf(unitPrice, 0) {
		return 0, errMediaPriceMissing()
	}
	if service.MediaBillingDepsOf(b.gateway).UsageBilling == nil {
		return 0, ErrMediaTaskUnavailable
	}
	if unitPrice <= 0 {
		// 单价为 0 视为未配置而非免费：免费出片必须是显式的产品决策，
		// 不能由一条缺失或写错的配置静默产生。
		return 0, errMediaPriceMissing()
	}

	cmd := &BatchImageBalanceHoldCommand{
		// RequestID 必须经既有助手生成：Release 内部会用
		// BatchImageHoldRequestID(cmd.BatchID) 反查 hold claim，
		// 自定义前缀会让释放被静默跳过、冻结额永久悬挂。
		RequestID:  BatchImageHoldRequestID(mediaHoldBatchID(task.ID)),
		APIKeyID:   apiKey.ID,
		UserID:     apiKey.UserID,
		BatchID:    mediaHoldBatchID(task.ID),
		HoldAmount: unitPrice,
	}
	if _, err := service.MediaBillingDepsOf(b.gateway).UsageBilling.ReserveBatchImageBalance(ctx, cmd); err != nil {
		return 0, mapMediaBillingError(err)
	}
	b.invalidateBalanceCache(ctx, apiKey.UserID)
	return unitPrice, nil
}

// Capture 在首次观察到终态成功且结果可下载时确认扣除。
//
// 落库顺序固定为「先装配 UsageLog → 再扣费 → 最后写库」。
func (b *MediaTaskBilling) Capture(ctx context.Context, task *MediaTaskRecord, apiKey *APIKey, user *User, account *Account) error {
	if b == nil || b.gateway == nil || task == nil || apiKey == nil || account == nil {
		return ErrMediaTaskUnavailable
	}
	// 用创建时的单价快照，不重新解析价格：capture 要求实扣不超过冻结额，
	// 管理员在任务在途期间调高价格会让重新解析的结果卡死结算。
	amount := task.UnitPrice
	if amount <= 0 {
		return errMediaPriceMissing()
	}

	usageLog := b.buildUsageLog(task, apiKey, account, amount)

	captureCmd := &BatchImageBalanceHoldCommand{
		RequestID:    BatchImageCaptureRequestID(mediaHoldBatchID(task.ID)),
		APIKeyID:     apiKey.ID,
		UserID:       apiKey.UserID,
		BatchID:      mediaHoldBatchID(task.ID),
		HoldAmount:   amount,
		ActualAmount: amount,
	}
	repo := service.MediaBillingDepsOf(b.gateway).UsageBilling
	capture := repo.CaptureBatchImageBalance
	if historical, ok := repo.(interface {
		CaptureSeedanceBalance(context.Context, *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error)
	}); ok {
		capture = historical.CaptureSeedanceBalance
	}
	if _, err := capture(ctx, captureCmd); err != nil {
		return mapMediaBillingError(err)
	}

	if err := b.applyNonBalanceDimensions(ctx, task, apiKey, account, amount, usageLog); err != nil {
		return err
	}
	if b.usageWriter == nil {
		return ErrMediaTaskUnavailable
	}
	return b.usageWriter(ctx, usageLog)
}

// Release 仅在明确拒绝或确认生成失败时退回冻结额，不按任务年龄退款。
func (b *MediaTaskBilling) Release(ctx context.Context, task *MediaTaskRecord, apiKey *APIKey) error {
	if b == nil || b.gateway == nil || task == nil || apiKey == nil {
		return nil
	}
	if task.UnitPrice <= 0 {
		return nil
	}
	return b.ReleaseHold(ctx, &MediaPendingHold{
		TaskID:   task.ID,
		UserID:   apiKey.UserID,
		APIKeyID: apiKey.ID,
		Amount:   task.UnitPrice,
	})
}

// ReleaseHold 复用原包冻结凭据接口；当前恢复流程会先持久化退款意图。
func (b *MediaTaskBilling) ReleaseHold(ctx context.Context, hold *MediaPendingHold) error {
	if b == nil || b.gateway == nil || hold == nil || hold.Amount <= 0 {
		return nil
	}
	cmd := &BatchImageBalanceHoldCommand{
		RequestID:  BatchImageReleaseRequestID(mediaHoldBatchID(hold.TaskID)),
		APIKeyID:   hold.APIKeyID,
		UserID:     hold.UserID,
		BatchID:    mediaHoldBatchID(hold.TaskID),
		HoldAmount: hold.Amount,
	}
	repo := service.MediaBillingDepsOf(b.gateway).UsageBilling
	release := repo.ReleaseBatchImageBalance
	if historical, ok := repo.(interface {
		ReleaseSeedanceBalance(context.Context, *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error)
	}); ok {
		release = historical.ReleaseSeedanceBalance
	}
	if _, err := release(ctx, cmd); err != nil {
		return mapMediaBillingError(err)
	}
	b.invalidateBalanceCache(ctx, hold.UserID)
	return nil
}

// buildUsageLog 自行装配用量行。
//
// 因为自行装配，video_resolution 与 video_duration_seconds 两列可以正常写入
// （service.UsageLog 本就有这两个字段），无需触碰共享的 ForwardResult。
func (b *MediaTaskBilling) buildUsageLog(task *MediaTaskRecord, apiKey *APIKey, account *Account, amount float64) *UsageLog {
	billingMode := string(BillingModeVideo)
	inbound := "/v1/media/videos"
	upstream := "seedance:/v1/videos"
	multiplier := 1.0

	log := &UsageLog{
		UserID:   apiKey.UserID,
		APIKeyID: apiKey.ID,
		// 账号固定为创建时绑定的那个：上游资源按账号隔离，用别的账号对账会错位。
		AccountID:        account.ID,
		RequestID:        mediaHoldBatchID(task.ID),
		Model:            task.Model,
		RequestedModel:   task.Model,
		InboundEndpoint:  &inbound,
		UpstreamEndpoint: &upstream,
		BillingType:      BillingTypeBalance,
		RequestType:      RequestTypeSync,
		BillingMode:      &billingMode,
		RateMultiplier:   multiplier,
		VideoCount:       1,
		TotalCost:        amount,
		ActualCost:       amount,
		GroupID:          task.GroupID,
		CreatedAt:        time.Unix(task.CreatedAt, 0),
	}
	if task.Resolution != "" {
		resolution := task.Resolution
		log.VideoResolution = &resolution
	}
	if task.DurationSeconds > 0 {
		// 真实时长直接落库，不经 NormalizeVideoBillingDurationSecondsOrDefault：
		// 那里的 15 秒上限会把 seedance2.5 的 30 秒截半。
		duration := task.DurationSeconds
		log.VideoDurationSeconds = &duration
	}
	return log
}

// applyNonBalanceDimensions 联动 API Key 配额、限流与账号配额。
//
// 刻意不复用 buildUsageBillingCommand / applyUsageBilling：前者在非订阅分支
// 必然从 ActualCost 推导 BalanceCost，而三个 shouldXxx 谓词同样以
// ActualCost > 0 为前提——无法做到「BalanceCost 为 0 但其余维度非 0」，
// 把 ActualCost 置 0 会让配额、限流、账号配额一起归零。余额已由 Capture 扣过，
// 这里再扣一次就是重复扣费。
//
// 同理不调用 finalizePostUsageBilling：其 syncBalanceCacheAfterDeduction 会
// QueueDeductBalance(ActualCost)，会让余额缓存再少一笔。余额缓存改用失效。
func (b *MediaTaskBilling) applyNonBalanceDimensions(
	ctx context.Context,
	task *MediaTaskRecord,
	apiKey *APIKey,
	account *Account,
	amount float64,
	usageLog *UsageLog,
) error {
	repo := service.MediaBillingDepsOf(b.gateway).UsageBilling
	if repo == nil {
		return ErrMediaTaskUnavailable
	}

	cmd := &UsageBillingCommand{
		RequestID:   BatchImageCaptureRequestID(mediaHoldBatchID(task.ID)) + ":usage",
		APIKeyID:    apiKey.ID,
		UserID:      apiKey.UserID,
		AccountID:   account.ID,
		AccountType: account.Type,
		Model:       usageLog.Model,
		MediaType:   string(BillingModeVideo),
		BillingType: BillingTypeBalance,
		// BalanceCost 与 SubscriptionCost 一律留 0：余额已由 Capture 扣过。
		BalanceCost:      0,
		SubscriptionCost: 0,
	}
	if apiKey.Quota > 0 || task.BillingQuota {
		cmd.APIKeyQuotaCost = amount
	}
	if apiKey.HasRateLimits() || task.BillingRateLimits {
		cmd.APIKeyRateLimitCost = amount
	}
	if task.BillingAccountQuota || (account.IsAPIKeyOrBedrock() && account.HasAnyQuotaLimit()) {
		cmd.AccountQuotaCost = amount
	}
	// 三个维度都为 0 时没有必要产生一条扣费记录。
	if cmd.APIKeyQuotaCost == 0 && cmd.APIKeyRateLimitCost == 0 && cmd.AccountQuotaCost == 0 {
		b.invalidateBalanceCache(ctx, apiKey.UserID)
		return nil
	}
	cmd.Normalize()

	apply := repo.Apply
	if historical, ok := repo.(interface {
		ApplySeedanceSettlement(context.Context, *UsageBillingCommand) (*UsageBillingApplyResult, error)
	}); ok {
		apply = historical.ApplySeedanceSettlement
	}
	result, err := apply(ctx, cmd)
	if err != nil {
		return mapMediaBillingError(err)
	}
	// Invalidate instead of incrementing the cache: retries may observe an already
	// applied SQL transaction after its response was lost.
	if result != nil && service.MediaBillingDepsOf(b.gateway).BillingCache != nil {
		if err := service.MediaBillingDepsOf(b.gateway).BillingCache.InvalidateAPIKeyRateLimit(ctx, apiKey.ID); err != nil {
			return err
		}
	}
	if service.MediaBillingDepsOf(b.gateway).Deferred != nil {
		service.MediaBillingDepsOf(b.gateway).Deferred.ScheduleLastUsedUpdate(account.ID)
	}
	b.invalidateBalanceCache(ctx, apiKey.UserID)
	return nil
}

// invalidateBalanceCache 让余额缓存失效而不是按增量扣减：真实余额由冻结/
// 确认/退回三个原语直接改动，缓存无法通过增量跟上。
func (b *MediaTaskBilling) invalidateBalanceCache(ctx context.Context, userID int64) {
	if inv, ok := b.apiKeyService.(interface{ InvalidateAuthCacheByUserID(context.Context, int64) }); ok {
		inv.InvalidateAuthCacheByUserID(ctx, userID)
	}
	if b.gateway == nil || service.MediaBillingDepsOf(b.gateway).BillingCache == nil || userID <= 0 {
		return
	}
	if err := service.MediaBillingDepsOf(b.gateway).BillingCache.InvalidateUserBalance(ctx, userID); err != nil {
		logger.L().Warn("media_billing.invalidate_balance_cache_failed",
			zap.Int64("user_id", userID), zap.Error(err))
	}
}

// mapMediaBillingError 把冻结原语的错误转成媒体接口自有错误。
//
// 底层错误值带批量图片语义（ErrBatchImageInsufficientBalance 等），
// 直接透出会让用户看到与自己请求无关的 "batch image" 字样。
func mapMediaBillingError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrBatchImageInsufficientBalance):
		return infraerrors.New(http.StatusPaymentRequired, "MEDIA_INSUFFICIENT_BALANCE",
			"insufficient balance for this media task")
	case errors.Is(err, ErrBatchImageSettlementCostExceedsHold):
		return infraerrors.New(http.StatusInternalServerError, "MEDIA_SETTLEMENT_EXCEEDS_HOLD",
			"settlement amount exceeds the reserved amount")
	case errors.Is(err, ErrUserNotFound):
		return infraerrors.New(http.StatusUnauthorized, "MEDIA_USER_NOT_FOUND", "user not found")
	default:
		return infraerrors.New(http.StatusServiceUnavailable, "MEDIA_BILLING_UNAVAILABLE",
			"media billing is temporarily unavailable").WithCause(err)
	}
}
