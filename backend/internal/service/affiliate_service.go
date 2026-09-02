package service

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

var (
	ErrAffiliateProfileNotFound = infraerrors.NotFound("AFFILIATE_PROFILE_NOT_FOUND", "affiliate profile not found")
	ErrAffiliateCodeInvalid     = infraerrors.BadRequest("AFFILIATE_CODE_INVALID", "invalid affiliate code")
	ErrAffiliateCodeTaken       = infraerrors.Conflict("AFFILIATE_CODE_TAKEN", "affiliate code already in use")
	ErrAffiliateAlreadyBound    = infraerrors.Conflict("AFFILIATE_ALREADY_BOUND", "affiliate inviter already bound")
	ErrAffiliateQuotaEmpty      = infraerrors.BadRequest("AFFILIATE_QUOTA_EMPTY", "no affiliate quota available to transfer")
)

const (
	affiliateInviteesLimit = 100
	// AffiliateCodeMinLength / AffiliateCodeMaxLength bound both system-generated
	// 12-char codes and admin-customized codes (e.g. "VIP2026").
	AffiliateCodeMinLength = 4
	AffiliateCodeMaxLength = 32
)

// affiliateCodeValidChar accepts uppercase letters, digits, underscore and dash.
// All input passes through strings.ToUpper before validation, so lowercase from
// users is normalized — admins may supply mixed case in their UI.
var affiliateCodeValidChar = func() [256]bool {
	var tbl [256]bool
	for c := byte('A'); c <= 'Z'; c++ {
		tbl[c] = true
	}
	for c := byte('0'); c <= '9'; c++ {
		tbl[c] = true
	}
	tbl['_'] = true
	tbl['-'] = true
	return tbl
}()

// isValidAffiliateCodeFormat validates code format for both binding (user input)
// and admin updates. Caller is expected to upper-case the input first.
func isValidAffiliateCodeFormat(code string) bool {
	if len(code) < AffiliateCodeMinLength || len(code) > AffiliateCodeMaxLength {
		return false
	}
	for i := 0; i < len(code); i++ {
		if !affiliateCodeValidChar[code[i]] {
			return false
		}
	}
	return true
}

type AffiliateSummary struct {
	UserID               int64     `json:"user_id"`
	AffCode              string    `json:"aff_code"`
	AffCodeCustom        bool      `json:"aff_code_custom"`
	AffRebateRatePercent *float64  `json:"aff_rebate_rate_percent,omitempty"`
	InviterID            *int64    `json:"inviter_id,omitempty"`
	AffCount             int       `json:"aff_count"`
	AffQuota             float64   `json:"aff_quota"`
	AffFrozenQuota       float64   `json:"aff_frozen_quota"`
	AffHistoryQuota      float64   `json:"aff_history_quota"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type AffiliateInvitee struct {
	UserID      int64      `json:"user_id"`
	Email       string     `json:"email"`
	Username    string     `json:"username"`
	CreatedAt   *time.Time `json:"created_at,omitempty"`
	TotalRebate float64    `json:"total_rebate"`
}

type AffiliateDetail struct {
	UserID          int64   `json:"user_id"`
	AffCode         string  `json:"aff_code"`
	InviterID       *int64  `json:"inviter_id,omitempty"`
	AffCount        int     `json:"aff_count"`
	AffQuota        float64 `json:"aff_quota"`
	AffFrozenQuota  float64 `json:"aff_frozen_quota"`
	AffHistoryQuota float64 `json:"aff_history_quota"`
	// EffectiveRebateRatePercent 是当前用户作为邀请人时实际生效的返利比例：
	// 优先用户自己的专属比例（aff_rebate_rate_percent），否则回退到全局比例。
	// 用于在用户的 /affiliate 页面直观展示「分享后能拿到多少」。
	EffectiveRebateRatePercent float64            `json:"effective_rebate_rate_percent"`
	Invitees                   []AffiliateInvitee `json:"invitees"`
}

type AffiliateRepository interface {
	EnsureUserAffiliate(ctx context.Context, userID int64) (*AffiliateSummary, error)
	GetAffiliateByCode(ctx context.Context, code string) (*AffiliateSummary, error)
	BindInviter(ctx context.Context, userID, inviterID int64) (bool, error)
	AccrueQuota(ctx context.Context, inviterID, inviteeUserID int64, amount float64, freezeHours int, sourceOrderID *int64) (bool, error)
	GetAccruedRebateFromInvitee(ctx context.Context, inviterID, inviteeUserID int64) (float64, error)
	ThawFrozenQuota(ctx context.Context, userID int64) (float64, error)
	TransferQuotaToBalance(ctx context.Context, userID int64) (float64, float64, error)
	ListInvitees(ctx context.Context, inviterID int64, limit int) ([]AffiliateInvitee, error)

	// 管理端：用户级专属配置
	UpdateUserAffCode(ctx context.Context, userID int64, newCode string) error
	ResetUserAffCode(ctx context.Context, userID int64) (string, error)
	SetUserRebateRate(ctx context.Context, userID int64, ratePercent *float64) error
	BatchSetUserRebateRate(ctx context.Context, userIDs []int64, ratePercent *float64) error
	ListUsersWithCustomSettings(ctx context.Context, filter AffiliateAdminFilter) ([]AffiliateAdminEntry, int64, error)
	ListAffiliateInviteRecords(ctx context.Context, filter AffiliateRecordFilter) ([]AffiliateInviteRecord, int64, error)
	ListAffiliateRebateRecords(ctx context.Context, filter AffiliateRecordFilter) ([]AffiliateRebateRecord, int64, error)
	ListAffiliateTransferRecords(ctx context.Context, filter AffiliateRecordFilter) ([]AffiliateTransferRecord, int64, error)
	GetAffiliateUserOverview(ctx context.Context, userID int64) (*AffiliateUserOverview, error)
}

// AffiliateSignupRewardRepository is an optional migration capability. Keeping
// it separate preserves the upstream AffiliateRepository contract for tests and
// alternate repository implementations while the SubNexus switch is disabled.
type AffiliateSignupRewardRepository interface {
	GrantSignupReward(ctx context.Context, inviterID, inviteeUserID int64, inviterAmount, inviteeAmount float64, clientIP string, ipLimitEnabled bool, ipDailyLimit int) (AffiliateSignupRewardResult, error)
}

// AffiliateSignupRewardPending is the immutable policy snapshot persisted
// after an invite binding.  Keeping the amounts and risk-control inputs on the
// job means a retry cannot silently pick up a later administrator change.
type AffiliateSignupRewardPending struct {
	InviterID      int64
	InviteeUserID  int64
	InviterAmount  float64
	InviteeAmount  float64
	ClientIP       string
	IPLimitEnabled bool
	IPDailyLimit   int
}

// AffiliateSignupRewardProcessResult describes one durable pending-job
// transition.  A transient reward error is returned to the caller while the
// repository keeps the row pending with a bounded backoff.
type AffiliateSignupRewardProcessResult struct {
	Applied        bool
	Completed      bool
	Skipped        bool
	RetryScheduled bool
	SkipReason     string
	InviterID      int64
	InviteeUserID  int64
}

// AffiliateSignupRewardReconcileResult is intentionally small and suitable
// for scheduler logs/metrics.  AffectedUserIDs lets the service invalidate
// balance/affiliate caches after a background grant without coupling the
// repository to cache implementations.
type AffiliateSignupRewardReconcileResult struct {
	Examined        int
	Applied         int
	Completed       int
	Skipped         int
	Retried         int
	AffectedUserIDs []int64
}

// AffiliateSignupRewardPendingRepository is an optional capability layered on
// top of the upstream affiliate repository.  Keeping it separate preserves
// compatibility with old tests and alternate repository implementations.
type AffiliateSignupRewardPendingRepository interface {
	EnqueueSignupReward(ctx context.Context, pending AffiliateSignupRewardPending) (jobID int64, inserted bool, err error)
	ProcessSignupReward(ctx context.Context, jobID int64) (AffiliateSignupRewardProcessResult, error)
	ReconcileSignupRewards(ctx context.Context, limit int) (AffiliateSignupRewardReconcileResult, error)
}

// AffiliateSignupRewardAtomicBinder is implemented by the production SQL
// repository so an inviter binding and its durable signup-reward job are
// committed together. It is optional to preserve compatibility with the
// upstream AffiliateRepository contract and lightweight test doubles.
type AffiliateSignupRewardAtomicBinder interface {
	BindInviterAndEnqueueSignupReward(ctx context.Context, userID, inviterID int64, pending AffiliateSignupRewardPending) (bound bool, jobID int64, err error)
}

// AffiliateSignupRewardResult describes a registration reward attempt. A
// skipped result is intentionally not an error: idempotent retries and risk
// controls must never turn a successful registration into a failure.
type AffiliateSignupRewardResult struct {
	Applied    bool
	Skipped    bool
	SkipReason string
}

type affiliateSignupIPContextKey struct{}

// WithAffiliateSignupIP carries the IP resolved by the HTTP boundary into the
// registration service. Callers must pass a trusted value (normally
// ip.GetTrustedClientIP); this helper does not parse or trust forwarding headers.
func WithAffiliateSignupIP(ctx context.Context, clientIP string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	clientIP = strings.TrimSpace(clientIP)
	if clientIP == "" {
		return ctx
	}
	return context.WithValue(ctx, affiliateSignupIPContextKey{}, clientIP)
}

func affiliateSignupIPFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if value, ok := ctx.Value(affiliateSignupIPContextKey{}).(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

// AffiliateAdminFilter 列表筛选条件
type AffiliateAdminFilter struct {
	Search   string
	Page     int
	PageSize int
}

// AffiliateAdminEntry 专属用户列表条目
type AffiliateAdminEntry struct {
	UserID               int64    `json:"user_id"`
	Email                string   `json:"email"`
	Username             string   `json:"username"`
	AffCode              string   `json:"aff_code"`
	AffCodeCustom        bool     `json:"aff_code_custom"`
	AffRebateRatePercent *float64 `json:"aff_rebate_rate_percent,omitempty"`
	AffCount             int      `json:"aff_count"`
}

type AffiliateRecordFilter struct {
	Search   string
	Page     int
	PageSize int
	StartAt  *time.Time
	EndAt    *time.Time
	SortBy   string
	SortDesc bool
}

type AffiliateInviteRecord struct {
	InviterID       int64     `json:"inviter_id"`
	InviterEmail    string    `json:"inviter_email"`
	InviterUsername string    `json:"inviter_username"`
	InviteeID       int64     `json:"invitee_id"`
	InviteeEmail    string    `json:"invitee_email"`
	InviteeUsername string    `json:"invitee_username"`
	AffCode         string    `json:"aff_code"`
	TotalRebate     float64   `json:"total_rebate"`
	CreatedAt       time.Time `json:"created_at"`
}

type AffiliateRebateRecord struct {
	OrderID         int64     `json:"order_id"`
	OutTradeNo      string    `json:"out_trade_no"`
	InviterID       int64     `json:"inviter_id"`
	InviterEmail    string    `json:"inviter_email"`
	InviterUsername string    `json:"inviter_username"`
	InviteeID       int64     `json:"invitee_id"`
	InviteeEmail    string    `json:"invitee_email"`
	InviteeUsername string    `json:"invitee_username"`
	OrderAmount     float64   `json:"order_amount"`
	PayAmount       float64   `json:"pay_amount"`
	RebateAmount    float64   `json:"rebate_amount"`
	PaymentType     string    `json:"payment_type"`
	OrderStatus     string    `json:"order_status"`
	CreatedAt       time.Time `json:"created_at"`
}

type AffiliateTransferRecord struct {
	LedgerID            int64     `json:"ledger_id"`
	UserID              int64     `json:"user_id"`
	UserEmail           string    `json:"user_email"`
	Username            string    `json:"username"`
	Amount              float64   `json:"amount"`
	BalanceAfter        *float64  `json:"balance_after,omitempty"`
	AvailableQuotaAfter *float64  `json:"available_quota_after,omitempty"`
	FrozenQuotaAfter    *float64  `json:"frozen_quota_after,omitempty"`
	HistoryQuotaAfter   *float64  `json:"history_quota_after,omitempty"`
	SnapshotAvailable   bool      `json:"snapshot_available"`
	CurrentBalance      float64   `json:"-"`
	RemainingQuota      float64   `json:"-"`
	FrozenQuota         float64   `json:"-"`
	HistoryQuota        float64   `json:"-"`
	CreatedAt           time.Time `json:"created_at"`
}

type AffiliateUserOverview struct {
	UserID              int64   `json:"user_id"`
	Email               string  `json:"email"`
	Username            string  `json:"username"`
	AffCode             string  `json:"aff_code"`
	RebateRatePercent   float64 `json:"rebate_rate_percent"`
	RebateRateCustom    bool    `json:"-"`
	InvitedCount        int     `json:"invited_count"`
	RebatedInviteeCount int     `json:"rebated_invitee_count"`
	AvailableQuota      float64 `json:"available_quota"`
	HistoryQuota        float64 `json:"history_quota"`
}

type AffiliateService struct {
	repo                 AffiliateRepository
	settingService       *SettingService
	authCacheInvalidator APIKeyAuthCacheInvalidator
	billingCacheService  *BillingCacheService
}

func NewAffiliateService(repo AffiliateRepository, settingService *SettingService, authCacheInvalidator APIKeyAuthCacheInvalidator, billingCacheService *BillingCacheService) *AffiliateService {
	return &AffiliateService{
		repo:                 repo,
		settingService:       settingService,
		authCacheInvalidator: authCacheInvalidator,
		billingCacheService:  billingCacheService,
	}
}

// IsEnabled reports whether the affiliate (邀请返利) feature is turned on.
func (s *AffiliateService) IsEnabled(ctx context.Context) bool {
	if s == nil || s.settingService == nil {
		return AffiliateEnabledDefault
	}
	return s.settingService.IsAffiliateEnabled(ctx)
}

func (s *AffiliateService) EnsureUserAffiliate(ctx context.Context, userID int64) (*AffiliateSummary, error) {
	if userID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_USER", "invalid user")
	}
	if s == nil || s.repo == nil {
		return nil, infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "affiliate service unavailable")
	}
	return s.repo.EnsureUserAffiliate(ctx, userID)
}

func (s *AffiliateService) GetAffiliateDetail(ctx context.Context, userID int64) (*AffiliateDetail, error) {
	// Lazy thaw: move any matured frozen quota to available before reading.
	if s != nil && s.repo != nil {
		// best-effort: thaw failure is non-fatal
		_, _ = s.repo.ThawFrozenQuota(ctx, userID)
	}

	summary, err := s.EnsureUserAffiliate(ctx, userID)
	if err != nil {
		return nil, err
	}
	invitees, err := s.listInvitees(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &AffiliateDetail{
		UserID:                     summary.UserID,
		AffCode:                    summary.AffCode,
		InviterID:                  summary.InviterID,
		AffCount:                   summary.AffCount,
		AffQuota:                   summary.AffQuota,
		AffFrozenQuota:             summary.AffFrozenQuota,
		AffHistoryQuota:            summary.AffHistoryQuota,
		EffectiveRebateRatePercent: s.resolveRebateRatePercent(ctx, summary),
		Invitees:                   invitees,
	}, nil
}

func (s *AffiliateService) BindInviterByCode(ctx context.Context, userID int64, rawCode string) error {
	code := strings.ToUpper(strings.TrimSpace(rawCode))
	if code == "" {
		return nil
	}
	if s == nil || s.repo == nil {
		return infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "affiliate service unavailable")
	}
	// 总开关关闭时，注册阶段静默忽略 aff 参数（不报错，避免阻断注册流程）
	if !s.IsEnabled(ctx) {
		return nil
	}
	if !isValidAffiliateCodeFormat(code) {
		return ErrAffiliateCodeInvalid
	}

	selfSummary, err := s.repo.EnsureUserAffiliate(ctx, userID)
	if err != nil {
		return err
	}
	if selfSummary.InviterID != nil {
		return nil
	}

	inviterSummary, err := s.repo.GetAffiliateByCode(ctx, code)
	if err != nil {
		if errors.Is(err, ErrAffiliateProfileNotFound) {
			return ErrAffiliateCodeInvalid
		}
		return err
	}
	if inviterSummary == nil || inviterSummary.UserID <= 0 || inviterSummary.UserID == userID {
		return ErrAffiliateCodeInvalid
	}

	// Resolve the optional reward policy before binding. An unavailable or
	// malformed policy preserves the upstream binding behavior, while a valid
	// policy enables the atomic binding+enqueue path below.
	pending, rewardReady, rewardConfigErr := s.buildSignupRewardPending(ctx, inviterSummary.UserID, userID)
	if rewardConfigErr != nil {
		logger.LegacyPrintf("service.affiliate", "[Affiliate] SubNexus signup reward policy unavailable: inviter_id=%d invitee_id=%d err=%v", inviterSummary.UserID, userID, rewardConfigErr)
	}

	// The production SQL repository implements this capability. Keeping the
	// binding and durable job insert in one transaction closes the only window
	// in which a successful binding could otherwise lose its reward job.
	if rewardReady {
		if atomicBinder, ok := s.repo.(AffiliateSignupRewardAtomicBinder); ok {
			bound, jobID, err := atomicBinder.BindInviterAndEnqueueSignupReward(ctx, userID, inviterSummary.UserID, pending)
			if err != nil {
				return err
			}
			if !bound {
				return ErrAffiliateAlreadyBound
			}
			if jobID > 0 {
				// The binding and pending row are already committed together.  An
				// immediate balance attempt is deliberately best-effort: a transient
				// failure is persisted by ProcessSignupReward and must never turn a
				// successful upstream registration/binding into an error.  The
				// background scanner will retry the pending row with backoff.
				if processErr := s.processSignupRewardJob(ctx, jobID, inviterSummary.UserID, userID); processErr != nil {
					logger.LegacyPrintf("service.affiliate", "[Affiliate] failed to process durable SubNexus signup reward: inviter_id=%d invitee_id=%d job_id=%d err=%v", inviterSummary.UserID, userID, jobID, processErr)
				}
			}
			return nil
		}
	}

	bound, err := s.repo.BindInviter(ctx, userID, inviterSummary.UserID)
	if err != nil {
		return err
	}
	if !bound {
		return ErrAffiliateAlreadyBound
	}
	// Alternate repositories that do not expose the atomic capability retain a
	// compatibility path. Production always takes the atomic branch above;
	// failures here remain non-blocking for registration.
	if rewardReady {
		if err := s.grantSubNexusSignupRewardAfterBind(ctx, inviterSummary.UserID, userID, pending); err != nil {
			logger.LegacyPrintf("service.affiliate", "[Affiliate] failed to grant SubNexus signup reward: inviter_id=%d invitee_id=%d err=%v", inviterSummary.UserID, userID, err)
		}
	}
	return nil
}

func (s *AffiliateService) buildSignupRewardPending(ctx context.Context, inviterID, inviteeUserID int64) (AffiliateSignupRewardPending, bool, error) {
	var pending AffiliateSignupRewardPending
	if s == nil || s.repo == nil || s.settingService == nil {
		return pending, false, nil
	}
	config, err := s.settingService.GetSubNexusInviteSignupRewardRuntime(ctx)
	if err != nil {
		return pending, false, err
	}
	if !config.Enabled {
		return pending, false, nil
	}
	if config.InviterAmount <= 0 && config.InviteeAmount <= 0 {
		return pending, false, nil
	}
	clientIP := affiliateSignupIPFromContext(ctx)
	if config.IPLimitEnabled && clientIP == "" {
		return pending, false, nil
	}
	return AffiliateSignupRewardPending{
		InviterID:      inviterID,
		InviteeUserID:  inviteeUserID,
		InviterAmount:  config.InviterAmount,
		InviteeAmount:  config.InviteeAmount,
		ClientIP:       clientIP,
		IPLimitEnabled: config.IPLimitEnabled,
		IPDailyLimit:   config.IPDailyLimit,
	}, true, nil
}

func (s *AffiliateService) processSignupRewardJob(ctx context.Context, jobID, inviterID, inviteeUserID int64) error {
	pendingRepo, ok := s.repo.(AffiliateSignupRewardPendingRepository)
	if !ok {
		return errors.New("affiliate signup reward pending repository capability unavailable")
	}
	result, processErr := pendingRepo.ProcessSignupReward(ctx, jobID)
	if result.Applied {
		s.invalidateAffiliateCaches(ctx, inviterID)
		s.invalidateAffiliateCaches(ctx, inviteeUserID)
	}
	return processErr
}

func (s *AffiliateService) grantSubNexusSignupRewardAfterBind(ctx context.Context, inviterID, inviteeUserID int64, pending AffiliateSignupRewardPending) error {
	if s == nil || s.repo == nil {
		return nil
	}

	// Newer repositories persist the reward policy before attempting the
	// balance write.  This closes the only durable-loss window in the old
	// fire-and-forget path: a transient database failure after binding leaves a
	// pending row which the reconciler can retry.  The capability is optional so
	// upstream-compatible repository doubles continue to use the direct path.
	if pendingRepo, ok := s.repo.(AffiliateSignupRewardPendingRepository); ok {
		jobID, _, err := pendingRepo.EnqueueSignupReward(ctx, pending)
		if err != nil {
			return err
		}
		if jobID <= 0 {
			// The repository re-checks the gate in the enqueue transaction.  A
			// concurrent disable therefore results in a deliberate no-op.
			return nil
		}
		return s.processSignupRewardJob(ctx, jobID, inviterID, inviteeUserID)
	}

	rewardRepo, ok := s.repo.(AffiliateSignupRewardRepository)
	if !ok {
		return errors.New("affiliate signup reward repository capability unavailable")
	}
	result, err := rewardRepo.GrantSignupReward(
		ctx,
		inviterID,
		inviteeUserID,
		pending.InviterAmount,
		pending.InviteeAmount,
		pending.ClientIP,
		pending.IPLimitEnabled,
		pending.IPDailyLimit,
	)
	if err != nil {
		return err
	}
	if result.Applied {
		s.invalidateAffiliateCaches(ctx, inviterID)
		s.invalidateAffiliateCaches(ctx, inviteeUserID)
	}
	return nil
}

// ReconcileAffiliateSignupRewardsOnce processes a bounded batch of persisted
// signup-reward jobs.  The independent rollout gate is checked before any
// balance write; when disabled or malformed, pending rows remain untouched so
// an operator can safely pause the feature and resume it later.
func (s *AffiliateService) ReconcileAffiliateSignupRewardsOnce(ctx context.Context, limit int) (AffiliateSignupRewardReconcileResult, error) {
	var result AffiliateSignupRewardReconcileResult
	if s == nil || s.repo == nil || s.settingService == nil {
		return result, nil
	}
	config, err := s.settingService.GetSubNexusInviteSignupRewardRuntime(ctx)
	if err != nil {
		return result, err
	}
	if !config.Enabled || (config.InviterAmount <= 0 && config.InviteeAmount <= 0) {
		return result, nil
	}
	pendingRepo, ok := s.repo.(AffiliateSignupRewardPendingRepository)
	if !ok {
		return result, nil
	}
	result, err = pendingRepo.ReconcileSignupRewards(ctx, limit)
	for _, userID := range result.AffectedUserIDs {
		if userID > 0 {
			s.invalidateAffiliateCaches(ctx, userID)
		}
	}
	return result, err
}

func (s *AffiliateService) AccrueInviteRebate(ctx context.Context, inviteeUserID int64, baseRechargeAmount float64) (float64, error) {
	return s.AccrueInviteRebateForOrder(ctx, inviteeUserID, baseRechargeAmount, nil)
}

func (s *AffiliateService) AccrueInviteRebateForOrder(ctx context.Context, inviteeUserID int64, baseRechargeAmount float64, sourceOrderID *int64) (float64, error) {
	if s == nil || s.repo == nil {
		return 0, nil
	}
	if inviteeUserID <= 0 || baseRechargeAmount <= 0 || math.IsNaN(baseRechargeAmount) || math.IsInf(baseRechargeAmount, 0) {
		return 0, nil
	}
	// 总开关关闭时，新充值不再产生返利
	if !s.IsEnabled(ctx) {
		return 0, nil
	}

	inviteeSummary, err := s.repo.EnsureUserAffiliate(ctx, inviteeUserID)
	if err != nil {
		return 0, err
	}
	if inviteeSummary.InviterID == nil || *inviteeSummary.InviterID <= 0 {
		return 0, nil
	}

	// 加载邀请人 profile，优先使用专属比例（覆盖全局）
	inviterSummary, err := s.repo.EnsureUserAffiliate(ctx, *inviteeSummary.InviterID)
	if err != nil {
		return 0, err
	}
	// 有效期检查：超过返利有效期后不再产生返利
	if s.settingService != nil {
		if durationDays := s.settingService.GetAffiliateRebateDurationDays(ctx); durationDays > 0 {
			if time.Now().After(inviteeSummary.CreatedAt.AddDate(0, 0, durationDays)) {
				return 0, nil
			}
		}
	}

	rebateRatePercent := s.resolveRebateRatePercent(ctx, inviterSummary)
	rebate := roundTo(baseRechargeAmount*(rebateRatePercent/100), 8)
	if rebate <= 0 {
		return 0, nil
	}

	// 单人上限检查：精确截断到剩余额度
	if s.settingService != nil {
		if perInviteeCap := s.settingService.GetAffiliateRebatePerInviteeCap(ctx); perInviteeCap > 0 {
			existing, err := s.repo.GetAccruedRebateFromInvitee(ctx, *inviteeSummary.InviterID, inviteeUserID)
			if err != nil {
				return 0, err
			}
			if existing >= perInviteeCap {
				return 0, nil
			}
			if remaining := perInviteeCap - existing; rebate > remaining {
				rebate = roundTo(remaining, 8)
			}
		}
	}

	var freezeHours int
	if s.settingService != nil {
		freezeHours = s.settingService.GetAffiliateRebateFreezeHours(ctx)
	}

	applied, err := s.repo.AccrueQuota(ctx, *inviteeSummary.InviterID, inviteeUserID, rebate, freezeHours, sourceOrderID)
	if err != nil {
		return 0, err
	}
	if !applied {
		return 0, nil
	}
	return rebate, nil
}

// resolveRebateRatePercent returns the inviter's exclusive rate when set,
// otherwise the global setting value (clamped to [Min, Max]).
func (s *AffiliateService) resolveRebateRatePercent(ctx context.Context, inviter *AffiliateSummary) float64 {
	if inviter != nil && inviter.AffRebateRatePercent != nil {
		v := *inviter.AffRebateRatePercent
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return s.globalRebateRatePercent(ctx)
		}
		return clampAffiliateRebateRate(v)
	}
	return s.globalRebateRatePercent(ctx)
}

// globalRebateRatePercent reads the system-wide rebate rate via SettingService,
// returning the documented default when SettingService is unavailable.
func (s *AffiliateService) globalRebateRatePercent(ctx context.Context) float64 {
	if s == nil || s.settingService == nil {
		return AffiliateRebateRateDefault
	}
	return s.settingService.GetAffiliateRebateRatePercent(ctx)
}

func (s *AffiliateService) TransferAffiliateQuota(ctx context.Context, userID int64) (float64, float64, error) {
	if s == nil || s.repo == nil {
		return 0, 0, infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "affiliate service unavailable")
	}

	transferred, balance, err := s.repo.TransferQuotaToBalance(ctx, userID)
	if err != nil {
		return 0, 0, err
	}
	if transferred > 0 {
		s.invalidateAffiliateCaches(ctx, userID)
	}
	return transferred, balance, nil
}

func (s *AffiliateService) listInvitees(ctx context.Context, inviterID int64) ([]AffiliateInvitee, error) {
	if s == nil || s.repo == nil {
		return nil, infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "affiliate service unavailable")
	}
	invitees, err := s.repo.ListInvitees(ctx, inviterID, affiliateInviteesLimit)
	if err != nil {
		return nil, err
	}
	for i := range invitees {
		invitees[i].Email = maskEmail(invitees[i].Email)
	}
	return invitees, nil
}

func roundTo(v float64, scale int) float64 {
	factor := math.Pow10(scale)
	return math.Round(v*factor) / factor
}

func maskEmail(email string) string {
	email = strings.TrimSpace(email)
	if email == "" {
		return ""
	}
	at := strings.Index(email, "@")
	if at <= 0 || at >= len(email)-1 {
		return "***"
	}

	local := email[:at]
	domain := email[at+1:]
	dot := strings.LastIndex(domain, ".")

	maskedLocal := maskSegment(local)
	if dot <= 0 || dot >= len(domain)-1 {
		return maskedLocal + "@" + maskSegment(domain)
	}

	domainName := domain[:dot]
	tld := domain[dot:]
	return maskedLocal + "@" + maskSegment(domainName) + tld
}

func maskSegment(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return "***"
	}
	if len(r) == 1 {
		return string(r[0]) + "***"
	}
	return string(r[0]) + "***"
}

func (s *AffiliateService) invalidateAffiliateCaches(ctx context.Context, userID int64) {
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
	}
	if s.billingCacheService != nil {
		if err := s.billingCacheService.InvalidateUserBalance(ctx, userID); err != nil {
			logger.LegacyPrintf("service.affiliate", "[Affiliate] Failed to invalidate billing cache for user %d: %v", userID, err)
		}
	}
}

// =========================
// Admin: 专属配置管理
// =========================

// validateExclusiveRate ensures a per-user override is finite and within
// [Min, Max]. nil is always valid (means "clear / fall back to global").
func validateExclusiveRate(ratePercent *float64) error {
	if ratePercent == nil {
		return nil
	}
	v := *ratePercent
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return infraerrors.BadRequest("INVALID_RATE", "invalid rebate rate")
	}
	if v < AffiliateRebateRateMin || v > AffiliateRebateRateMax {
		return infraerrors.BadRequest("INVALID_RATE", "rebate rate out of range")
	}
	return nil
}

// AdminUpdateUserAffCode 管理员改写用户的邀请码（专属邀请码）。
func (s *AffiliateService) AdminUpdateUserAffCode(ctx context.Context, userID int64, rawCode string) error {
	if s == nil || s.repo == nil {
		return infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "affiliate service unavailable")
	}
	code := strings.ToUpper(strings.TrimSpace(rawCode))
	if !isValidAffiliateCodeFormat(code) {
		return ErrAffiliateCodeInvalid
	}
	return s.repo.UpdateUserAffCode(ctx, userID, code)
}

// AdminResetUserAffCode 重置用户邀请码为系统随机码。
func (s *AffiliateService) AdminResetUserAffCode(ctx context.Context, userID int64) (string, error) {
	if s == nil || s.repo == nil {
		return "", infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "affiliate service unavailable")
	}
	return s.repo.ResetUserAffCode(ctx, userID)
}

// AdminSetUserRebateRate 设置/清除用户专属返利比例。ratePercent==nil 表示清除。
func (s *AffiliateService) AdminSetUserRebateRate(ctx context.Context, userID int64, ratePercent *float64) error {
	if s == nil || s.repo == nil {
		return infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "affiliate service unavailable")
	}
	if err := validateExclusiveRate(ratePercent); err != nil {
		return err
	}
	return s.repo.SetUserRebateRate(ctx, userID, ratePercent)
}

// AdminBatchSetUserRebateRate 批量设置/清除用户专属返利比例。
func (s *AffiliateService) AdminBatchSetUserRebateRate(ctx context.Context, userIDs []int64, ratePercent *float64) error {
	if s == nil || s.repo == nil {
		return infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "affiliate service unavailable")
	}
	if err := validateExclusiveRate(ratePercent); err != nil {
		return err
	}
	cleaned := make([]int64, 0, len(userIDs))
	for _, uid := range userIDs {
		if uid > 0 {
			cleaned = append(cleaned, uid)
		}
	}
	if len(cleaned) == 0 {
		return nil
	}
	return s.repo.BatchSetUserRebateRate(ctx, cleaned, ratePercent)
}

// AdminListCustomUsers 列出有专属配置的用户。
func (s *AffiliateService) AdminListCustomUsers(ctx context.Context, filter AffiliateAdminFilter) ([]AffiliateAdminEntry, int64, error) {
	if s == nil || s.repo == nil {
		return nil, 0, infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "affiliate service unavailable")
	}
	return s.repo.ListUsersWithCustomSettings(ctx, filter)
}

func (s *AffiliateService) AdminListInviteRecords(ctx context.Context, filter AffiliateRecordFilter) ([]AffiliateInviteRecord, int64, error) {
	if s == nil || s.repo == nil {
		return nil, 0, infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "affiliate service unavailable")
	}
	return s.repo.ListAffiliateInviteRecords(ctx, normalizeAffiliateRecordFilter(filter))
}

func (s *AffiliateService) AdminListRebateRecords(ctx context.Context, filter AffiliateRecordFilter) ([]AffiliateRebateRecord, int64, error) {
	if s == nil || s.repo == nil {
		return nil, 0, infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "affiliate service unavailable")
	}
	return s.repo.ListAffiliateRebateRecords(ctx, normalizeAffiliateRecordFilter(filter))
}

func (s *AffiliateService) AdminListTransferRecords(ctx context.Context, filter AffiliateRecordFilter) ([]AffiliateTransferRecord, int64, error) {
	if s == nil || s.repo == nil {
		return nil, 0, infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "affiliate service unavailable")
	}
	return s.repo.ListAffiliateTransferRecords(ctx, normalizeAffiliateRecordFilter(filter))
}

func (s *AffiliateService) AdminGetUserOverview(ctx context.Context, userID int64) (*AffiliateUserOverview, error) {
	if userID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_USER", "invalid user")
	}
	if s == nil || s.repo == nil {
		return nil, infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "affiliate service unavailable")
	}
	overview, err := s.repo.GetAffiliateUserOverview(ctx, userID)
	if err != nil {
		return nil, err
	}
	if overview != nil {
		if !overview.RebateRateCustom {
			overview.RebateRatePercent = s.globalRebateRatePercent(ctx)
		}
		overview.RebateRatePercent = clampAffiliateRebateRate(overview.RebateRatePercent)
	}
	return overview, nil
}

func normalizeAffiliateRecordFilter(filter AffiliateRecordFilter) AffiliateRecordFilter {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	if filter.PageSize > 100 {
		filter.PageSize = 100
	}
	filter.Search = strings.TrimSpace(filter.Search)
	filter.SortBy = strings.TrimSpace(filter.SortBy)
	return filter
}
