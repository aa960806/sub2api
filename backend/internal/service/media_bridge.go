package service

import (
	"context"
	"errors"

	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
)

// 本文件是企业定制的桥接层：媒体任务子系统整体位于
// internal/custom/business/media，该包依赖本包的 Account、GatewayService 等类型，
// 因此本包不能反向 import 它。以下三项把 media 需要、但只在本包内可见的能力
// 暴露出去，并把「service 需要 media 提供」的唯一一处反转成工厂注册。
//
// 平台、路由和依赖注入接线保留在既有入口，媒体专用桥接集中于此。

// SeedanceCredentialProber 探测 Seedance 账号凭证是否可用。
// 由 media.SeedanceProvider 实现。
type SeedanceCredentialProber interface {
	ProbeCredential(ctx context.Context, account *Account) error
}

// newSeedanceProber 由 media 包在装配时经 RegisterSeedanceProberFactory 注入。
var newSeedanceProber func(HTTPUpstream) SeedanceCredentialProber

// RegisterSeedanceProberFactory 注册 Seedance 凭证探测器的构造方式。
// 未注册时账号测试会明确报错，而不是静默跳过——「测通了」和「没测」
// 在外部看起来一样，是这里最需要避免的情形。
func RegisterSeedanceProberFactory(f func(HTTPUpstream) SeedanceCredentialProber) {
	newSeedanceProber = f
}

// seedanceProberFor 返回探测器；未注册时返回 nil，由调用方决定如何报错。
func seedanceProberFor(upstream HTTPUpstream) SeedanceCredentialProber {
	if newSeedanceProber == nil {
		return nil
	}
	return newSeedanceProber(upstream)
}

// ResolveChannelPricingForMedia 暴露渠道定价解析给 media 包。
// 媒体任务按 per_request 计价，仍要走与网关一致的定价来源。
func ResolveChannelPricingForMedia(s *GatewayService, ctx context.Context, billingModel string, apiKey *APIKey) *ResolvedPricing {
	if s == nil {
		return nil
	}
	return s.resolveChannelPricing(ctx, billingModel, apiKey)
}

// IsPrivateOrLoopbackHostForMedia 暴露 SSRF 主机判定给 media 包。
// 媒体下载会跟随上游返回的 signed URL，必须用与官方一致的判定拦截内网地址。
func IsPrivateOrLoopbackHostForMedia(ctx context.Context, hostname string) (bool, error) {
	return isPrivateOrLoopbackHost(ctx, hostname)
}

// WriteUsageLogForMedia 复用网关的用量行写入（best-effort，失败只记日志）。
// 媒体任务的计费行必须与网关走同一条落库路径，否则账单口径会分叉。
func WriteUsageLogForMedia(ctx context.Context, s *GatewayService, usageLog *UsageLog, logKey string) {
	if s == nil {
		return
	}
	writeUsageLogBestEffort(ctx, s.usageLogRepo, usageLog, logKey)
}

// WriteMediaUsageLog synchronously persists the idempotent usage row. The task
// remains recoverable until this succeeds; a log-only failure loses accounting.
func WriteMediaUsageLog(ctx context.Context, s *GatewayService, usageLog *UsageLog) error {
	if s == nil || s.usageLogRepo == nil {
		return errors.New("media usage repository unavailable")
	}
	_, err := s.usageLogRepo.Create(ctx, usageLog)
	return err
}

// MediaBillingDeps 是媒体计费需要、但只在本包内可见的网关依赖。
//
// 媒体任务按 per_request 计价，冻结与结算必须复用网关同一套仓储与缓存，
// 否则账单口径会与网关分叉。这里把依赖显式列出而不是逐个开放访问器——
// 这份清单同时也是「media 计费依赖网关内部的全部内容」的说明。
type MediaBillingDeps struct {
	UsageBilling UsageBillingRepository
	BillingCache *BillingCacheService
	Deferred     *DeferredService
}

// MediaBillingDepsOf 取出网关的计费依赖；gateway 为 nil 时返回零值。
func MediaBillingDepsOf(s *GatewayService) MediaBillingDeps {
	if s == nil {
		return MediaBillingDeps{}
	}
	return MediaBillingDeps{
		UsageBilling: s.usageBillingRepo,
		BillingCache: s.billingCacheService,
		Deferred:     s.deferredService,
	}
}

// MediaAuthCacheInvalidator 是配额耗尽后使鉴权缓存失效的能力。
// APIKey 配额用尽时必须立刻失效缓存，否则该 key 会在缓存 TTL 内继续放行。
type MediaAuthCacheInvalidator interface {
	InvalidateAuthCacheByKey(ctx context.Context, key string)
}

// AuthCacheInvalidatorOf 在 apiKeyService 支持时返回失效器，否则返回 nil。
func AuthCacheInvalidatorOf(apiKeyService any) MediaAuthCacheInvalidator {
	if inv, ok := apiKeyService.(apiKeyAuthCacheInvalidator); ok {
		return inv
	}
	return nil
}

// GetMediaTaskAccount retrieves the original account for already accepted work.
// Soft deletion and quota/cooldown stop new work, not recovery of already
// accepted tasks. The media layer also pins the original credential/base URL
// fingerprint. Never use this historical read to admit a new reference task.
func (s *GatewayService) GetMediaTaskAccount(ctx context.Context, accountID int64) (*Account, error) {
	if s == nil || s.accountRepo == nil || accountID <= 0 {
		return nil, ErrMediaAccountUnavailableForMedia
	}
	account, err := s.accountRepo.GetByID(mixins.SkipSoftDelete(ctx), accountID)
	if err != nil || account == nil || !account.IsSeedance() || account.Type != AccountTypeAPIKey {
		return nil, ErrMediaAccountUnavailableForMedia
	}
	return account, nil
}

// GetMediaTaskAccountForCreate retains the normal soft-delete filter. Account
// does not expose DeletedAt, so checking IsSchedulable on a historical result
// cannot prove that it is eligible to receive newly created work.
func (s *GatewayService) GetMediaTaskAccountForCreate(ctx context.Context, accountID int64) (*Account, error) {
	if s == nil || s.accountRepo == nil || accountID <= 0 {
		return nil, ErrMediaAccountUnavailableForMedia
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil || account == nil || !account.IsSeedance() || account.Type != AccountTypeAPIKey {
		return nil, ErrMediaAccountUnavailableForMedia
	}
	return account, nil
}

// ErrMediaAccountUnavailableForMedia 与 media.ErrMediaAccountUnavailable 等价，
// 供本包内的薄包装返回；media 侧按 errors.Is 归一处理。
var ErrMediaAccountUnavailableForMedia = errors.New("media account unavailable")

// NewGatewayForMediaBillingTest 构造只装配计费仓储的最小网关，供 media 包的
// 计费单测使用。GatewayService 的字段未导出，跨包无法直接组装，而为一个测试
// 夹具把字段导出会放大生产代码的暴露面，故在此提供受限的构造入口。
//
// 仅用于测试：返回的实例缺少绝大多数依赖，走任何其它网关路径都会 panic。
func NewGatewayForMediaBillingTest(usageBilling UsageBillingRepository) *GatewayService {
	return &GatewayService{usageBillingRepo: usageBilling}
}

// NewGatewayForMediaPricingTest 在计费仓储之外再装配定价解析器，供 media 包
// 验证「按真实模型解析、回落通用 id」的分模型定价行为。resolver 字段未导出，
// 跨包无法直接设置，故与上面的构造入口同类提供。
//
// 仅用于测试：除计费与定价解析外的网关路径仍会 panic。
func NewGatewayForMediaPricingTest(usageBilling UsageBillingRepository, resolver *ModelPricingResolver) *GatewayService {
	return &GatewayService{usageBillingRepo: usageBilling, resolver: resolver}
}
