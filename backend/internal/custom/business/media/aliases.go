package media

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// 本文件把媒体任务子系统用到的 service 包符号别名到本包。
//
// 这样迁入本目录的实现代码无需逐处加包前缀，同时这份清单本身就是
// media 对 service 的完整依赖面——新增一项就要在这里显式登记一行，
// 上游改动波及到哪些符号也能一眼看全。

type (
	Group                        = service.Group
	BatchImageBalanceHoldResult  = service.BatchImageBalanceHoldResult
	APIKey                       = service.APIKey
	APIKeyQuotaUpdater           = service.APIKeyQuotaUpdater
	Account                      = service.Account
	BatchImageBalanceHoldCommand = service.BatchImageBalanceHoldCommand
	GatewayService               = service.GatewayService
	HTTPUpstream                 = service.HTTPUpstream
	UsageBillingCommand          = service.UsageBillingCommand
	UsageBillingApplyResult      = service.UsageBillingApplyResult
	UsageLog                     = service.UsageLog
	User                         = service.User
)

const (
	BillingModeImage      = service.BillingModeImage
	BillingModePerRequest = service.BillingModePerRequest
	BillingModeVideo      = service.BillingModeVideo
	BillingTypeBalance    = service.BillingTypeBalance
	PlatformSeedance      = service.PlatformSeedance
	RequestTypeSync       = service.RequestTypeSync
)

var (
	SubscriptionTypeSubscription           = service.SubscriptionTypeSubscription
	AccountTypeAPIKey                      = service.AccountTypeAPIKey
	BatchImageCaptureRequestID             = service.BatchImageCaptureRequestID
	BatchImageHoldRequestID                = service.BatchImageHoldRequestID
	BatchImageReleaseRequestID             = service.BatchImageReleaseRequestID
	ErrBatchImageInsufficientBalance       = service.ErrBatchImageInsufficientBalance
	ErrBatchImageSettlementCostExceedsHold = service.ErrBatchImageSettlementCostExceedsHold
	ErrUsageBillingRequestConflict         = service.ErrUsageBillingRequestConflict
	ErrUserNotFound                        = service.ErrUserNotFound
)

// 并发槽位与 SSE 心跳沿用网关的实现，来自 handler 包。
type ConcurrencyHelper = handler.ConcurrencyHelper

var NewConcurrencyHelper = handler.NewConcurrencyHelper

const SSEPingFormatNone = handler.SSEPingFormatNone
