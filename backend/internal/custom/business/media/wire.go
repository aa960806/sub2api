package media

import (
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/google/wire"
)

// ProviderSet 是媒体任务子系统的装配集合，由 cmd/server 注册。
var ProviderSet = wire.NewSet(
	NewMediaTaskStore,
	ProvideMediaTaskService,
	ProvideMediaHoldSweeper,
	NewMediaTaskHandler,
	// Handlers 以接口持有本包的处理器，handler 包因此不必 import media。
	wire.Bind(new(handler.MediaTaskRoutes), new(*MediaTaskHandler)),
)

// init 把 Seedance 凭证探测器注册回 service 包。
//
// 账号测试位于 service.AccountTestService，依赖它的未导出字段与事件方法，
// 无法迁入本包；而 service 又不能 import media（会形成环）。故由本包主动注册。
func init() {
	service.RegisterSeedanceProberFactory(func(u service.HTTPUpstream) service.SeedanceCredentialProber {
		return NewSeedanceProvider(u)
	})
}

// ProvideMediaTaskService 构造通用媒体任务服务，接入 Seedance provider 与结算。
//
// 结算生效还需管理端为计费模型 id "seedance" 配置一条 per_request 价格：
// 未配置时创建会 fail-closed 被拒（设计行为，防止免费出片）。
func ProvideMediaTaskService(
	store MediaTaskStore,
	httpUpstream service.HTTPUpstream,
	gateway *service.GatewayService,
	apiKeyService *service.APIKeyService,
	cfg *config.Config,
) *MediaTaskService {
	provider := NewSeedanceProvider(httpUpstream)
	if cfg != nil {
		// 参考图上限来自配置：上游规范与指南页对单图上限的表述冲突，
		// 默认取保守值，实测确认可放宽时改配置即可，不必改代码。
		provider = provider.WithImageLimits(cfg.Gateway.Media.MaxImageBytes, cfg.Gateway.Media.MaxImagesTotalBytes)
	}
	svc := NewMediaTaskService(store, service.PlatformSeedance, provider)
	if cfg != nil {
		svc.withDownloadSigningSecret(cfg.JWT.Secret)
	}
	return svc.WithBilling(NewMediaTaskBilling(gateway, apiKeyService)).WithEnabled(cfg != nil && cfg.Gateway.Media.Enabled)
}

// Recovery remains active while new creation is disabled.
func ProvideMediaHoldSweeper(h *MediaTaskHandler) *MediaHoldSweeper {
	worker := NewMediaHoldSweeper(h)
	worker.Start()
	return worker
}
