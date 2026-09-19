package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// 本文件是企业定制的桥接层：媒体任务子系统位于
// internal/custom/business/media，它需要本包的安全审计封装，而本包又要持有
// 它的路由处理器。直接互相 import 会成环，故这里做两件事：
//
//   - MediaTaskRoutes 让 Handlers 以接口持有 media 的处理器，本包不 import media
//   - RunSecurityAuditForMedia 等把本包内可见的审计封装暴露给 media
//
// 上游永远不会碰本文件；官方源文件因此只需把字段类型写成接口。

// MediaTaskRoutes 是媒体任务对外暴露的路由处理集合，由 media.MediaTaskHandler 实现。
// 面向你终端用户的 /v1/media/* 路由由这些方法承载。
type MediaTaskRoutes interface {
	Models(c *gin.Context)
	CreateVideo(c *gin.Context)
	GetVideo(c *gin.Context)
	GetVideoContent(c *gin.Context)
	UploadFile(c *gin.Context)
	SeedanceModels(c *gin.Context)
	CreateSeedanceTask(c *gin.Context)
	GetSeedanceTask(c *gin.Context)
	GetSeedanceResult(c *gin.Context)
}

// RunSecurityAuditForMedia 让媒体任务复用与网关一致的审计链路。
// 媒体请求同样携带用户 prompt，必须过同一套审计，不能另起一套判定。
func RunSecurityAuditForMedia(
	c *gin.Context,
	reqLog *zap.Logger,
	coordinator *securityaudit.Coordinator,
	legacy *service.ContentModerationService,
	apiKey *service.APIKey,
	subject middleware2.AuthSubject,
	protocol, model string,
	body []byte,
	stage string,
) *securityaudit.Decision {
	return runSecurityAudit(c, reqLog, coordinator, legacy, apiKey, subject, protocol, model, body, stage)
}

// SecurityAuditStatusForMedia 返回审计决策对应的 HTTP 状态码。
func SecurityAuditStatusForMedia(decision *securityaudit.Decision) int {
	return securityAuditStatus(decision)
}

// SecurityAuditErrorCodeForMedia 返回审计决策对应的错误码。
func SecurityAuditErrorCodeForMedia(decision *securityaudit.Decision) string {
	return securityAuditErrorCode(decision)
}

// SecurityAuditMessageForMedia 返回审计决策对应的用户可见消息。
func SecurityAuditMessageForMedia(decision *securityaudit.Decision) string {
	return securityAuditMessage(decision)
}

// ConcurrencyReady 报告并发助手是否具备占用账号槽位的能力。
// 媒体任务在服务缺失时降级为不占槽而非崩溃，故调用方需要先问一句。
func ConcurrencyReady(h *ConcurrencyHelper) bool {
	return h != nil && h.concurrencyService != nil
}
