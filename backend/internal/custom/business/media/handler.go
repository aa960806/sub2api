package media

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// mediaCreateIdempotencyScopePrefix 是网关侧首个幂等 scope 的前缀。
//
// 调用方身份必须进 Scope 而不是只进 ActorScope：幂等记录的唯一键是
// (scope, idempotency_key_hash)，ActorScope 只参与请求指纹。若 Scope 对所有
// 用户相同，两个用户用同一个 Idempotency-Key 会命中同一条记录，因指纹不同
// 而互相返回 409——既拒绝了合法请求，也泄露了该键已被他人使用。
//
// 同理不能复用 executeUserIdempotentJSON：它的 ActorScope 取面板 JWT 主体，
// 网关请求会全部落到 user:0。
const mediaCreateIdempotencyScopePrefix = "gateway.media.videos.create"

// mediaCreateIdempotencyScope 按调用方隔离幂等作用域。
func mediaCreateIdempotencyScope(userID, apiKeyID int64) string {
	return mediaCreateIdempotencyScopePrefix + ":user:" + strconv.FormatInt(userID, 10) + ":key:" + strconv.FormatInt(apiKeyID, 10)
}

// MediaTaskHandler 提供与既有模型接口完全隔离的媒体任务链路。
//
// 刻意不挂到 GatewayHandler / OpenAIGatewayHandler 上：媒体任务有自己的门禁、
// 账号亲和与并发口径。回滚前仍需先结清已接收的异步任务。
// mediaAccountResolver 抽出账号选择与取回两件事。
//
// *service.GatewayService 已天然满足本接口；抽成接口是为了让骨架能在没有
// 真实调度器的情况下用假实现完成端到端验收，不影响生产注入。
type mediaAccountResolver interface {
	SelectAccountForModelWithExclusions(ctx context.Context, groupID *int64, sessionHash string, requestedModel string, excludedIDs map[int64]struct{}) (*service.Account, error)
	GetMediaTaskAccount(ctx context.Context, accountID int64) (*service.Account, error)
	GetMediaTaskAccountForCreate(ctx context.Context, accountID int64) (*service.Account, error)
}

type MediaTaskHandler struct {
	tasks             *MediaTaskService
	gatewayService    mediaAccountResolver
	concurrencyHelper *ConcurrencyHelper
	// 创建请求携带用户 prompt（上游允许 6000 字符），必须与其它带 prompt 的
	// 网关路由一样过内容审核协调器，不得绕开。
	securityAuditCoordinator *securityaudit.Coordinator
	contentModerationService *service.ContentModerationService
}

func NewMediaTaskHandler(
	tasks *MediaTaskService,
	gatewayService *service.GatewayService,
	concurrencyService *service.ConcurrencyService,
	securityAuditCoordinator *securityaudit.Coordinator,
	contentModerationService *service.ContentModerationService,
) *MediaTaskHandler {
	return &MediaTaskHandler{
		tasks:          tasks,
		gatewayService: gatewayService,
		// 媒体任务是短连接的建单/轮询/下载，不需要 SSE 心跳。
		concurrencyHelper:        NewConcurrencyHelper(concurrencyService, SSEPingFormatNone, 0),
		securityAuditCoordinator: securityAuditCoordinator,
		contentModerationService: contentModerationService,
	}
}

// checkSecurityAudit 让创建路径与其它带 prompt 的网关路由走同一个审核协调器。
// routes/prompt_audit_route_coverage_test.go 会强制每条网关 POST 路由要么在
// 此过审、要么显式声明「无 prompt」，本方法即该契约的落点。
func (h *MediaTaskHandler) checkSecurityAudit(
	c *gin.Context,
	reqLog *zap.Logger,
	apiKey *service.APIKey,
	subject middleware2.AuthSubject,
	model string,
	body []byte,
) *securityaudit.Decision {
	if h == nil {
		return nil
	}
	// 复用 openai_images 协议：其抽取器按 prompt + images 取内容，
	// 与媒体创建请求的形状一致，无需新增审核管线。
	return handler.RunSecurityAuditForMedia(c, reqLog, h.securityAuditCoordinator, h.contentModerationService,
		apiKey, subject, service.ContentModerationProtocolOpenAIImages, model, body, "http")
}

// enabled 决定是否接受新建任务与上传。
func (h *MediaTaskHandler) enabled() bool {
	return h != nil && h.tasks != nil && h.tasks.Enabled() && h.gatewayService != nil
}

// pollable 弱于 enabled：总开关关闭后在途任务仍可查询与下载，
// 避免已冻结余额的任务被搁死。
func (h *MediaTaskHandler) pollable() bool {
	return h != nil && h.tasks != nil && h.tasks.Pollable()
}

// resolveMediaAPIKey 取出 API Key 并校验分组平台归属，返回 infraerror。
//
// 抽成返回 error 而非直接写响应，是为了让不同入口复用同一套鉴权判定，
// 各自按自己的信封形状渲染错误。
func (h *MediaTaskHandler) resolveMediaAPIKey(c *gin.Context) (*service.APIKey, error) {
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil || apiKey.UserID <= 0 || apiKey.ID <= 0 {
		return nil, infraerrors.New(http.StatusUnauthorized, "authentication_error", "invalid API key")
	}
	platform := ""
	if apiKey.Group != nil {
		platform = apiKey.Group.Platform
	}
	// 平台不匹配时的 404 与既有 /v1/videos 对非支持平台的口径一致。
	// 平台取自 service 而非 provider：provider 未接入时仍需允许在途任务轮询。
	if h.tasks == nil || platform != h.tasks.Platform() {
		return nil, infraerrors.New(http.StatusNotFound, "not_found_error", "Media API is not supported for this platform")
	}
	return apiKey, nil
}

// mediaAuth 取出 API Key 并校验分组平台归属，失败时渲染 media 形状错误。
func (h *MediaTaskHandler) mediaAuth(c *gin.Context) (*service.APIKey, bool) {
	apiKey, err := h.resolveMediaAPIKey(c)
	if err != nil {
		mediaError(c, err)
		return nil, false
	}
	return apiKey, true
}

// Models 渲染 provider 声明的能力表。不调用上游，无额外鉴权开销。
func (h *MediaTaskHandler) Models(c *gin.Context) {
	if h == nil || h.tasks == nil || h.tasks.Provider() == nil {
		mediaError(c, ErrMediaTaskDisabled)
		return
	}
	if _, ok := h.mediaAuth(c); !ok {
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, h.tasks.Provider().Capabilities())
}

type mediaCreateRequest struct {
	Prompt         string   `json:"prompt"`
	Model          string   `json:"model"`
	Duration       int      `json:"duration"`
	Ratio          string   `json:"ratio"`
	Resolution     string   `json:"resolution"`
	CameraMovement string   `json:"camera_movement"`
	FileIDs        []string `json:"file_ids"`
	ImageURL       string   `json:"image_url"`
	ImageURLs      []string `json:"image_urls"`
}

// CreateVideo 创建视频任务并返回 202。
//
// 执行顺序：校验与审核 → 定价和原账号绑定 → 持久化意图 → 冻结 → 返回202。
// 独立恢复worker使用原请求和固定幂等键提交上游，持久化并结算结果。
func (h *MediaTaskHandler) CreateVideo(c *gin.Context) {
	if !h.enabled() {
		mediaError(c, ErrMediaTaskDisabled)
		return
	}
	apiKey, ok := h.mediaAuth(c)
	if !ok {
		return
	}

	// 幂等键在 handler 层强制校验，不依赖 coordinator 的 RequireKey：
	// IdempotencyConfig.ObserveOnly 默认为 true，会让缺 Key 的请求直接放行。
	idempotencyKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if idempotencyKey == "" || len(idempotencyKey) > 200 {
		mediaJSONError(c, http.StatusBadRequest, "invalid_request_error", "Idempotency-Key header is required")
		return
	}

	// 先取原始 body：内容审核需要原文，ShouldBindJSON 会消费掉 Body。
	rawBody, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil || len(rawBody) == 0 {
		mediaJSONError(c, http.StatusBadRequest, "invalid_request_error", "invalid request body")
		return
	}
	var req mediaCreateRequest
	if err := json.Unmarshal(rawBody, &req); err != nil {
		mediaJSONError(c, http.StatusBadRequest, "invalid_request_error", "invalid request body")
		return
	}

	subject, _ := middleware2.GetAuthSubjectFromContext(c)

	imageURLs := req.ImageURLs
	if strings.TrimSpace(req.ImageURL) != "" {
		imageURLs = append(imageURLs, req.ImageURL)
	}
	createReq := &MediaVideoCreateRequest{
		Prompt:          req.Prompt,
		Model:           strings.TrimSpace(req.Model),
		DurationSeconds: req.Duration,
		Ratio:           strings.TrimSpace(req.Ratio),
		Resolution:      strings.TrimSpace(req.Resolution),
		CameraMovement:  strings.TrimSpace(req.CameraMovement),
		FileIDs:         req.FileIDs,
		ImageURLs:       imageURLs,
	}

	data, replayed, err := h.submitVideoCreate(c, apiKey, subject, rawBody, createReq, req, idempotencyKey)
	if err != nil {
		if retryAfter := service.RetryAfterSecondsFromError(err); retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		mediaError(c, err)
		return
	}
	if replayed {
		c.Header("X-Idempotency-Replayed", "true")
	}
	// 初次与重放都返回 202，且不套 {code,message,data} 信封——与同类视频接口
	// 的异步语义保持一致；本地任务已持久化后由后台 worker 向上游提交。
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusAccepted, data)
}

// Preserve the supplied audit, validation and request contract. Persistence
// additionally records the intent before freezing or contacting the upstream.
func (h *MediaTaskHandler) submitVideoCreate(
	c *gin.Context, apiKey *service.APIKey, subject middleware2.AuthSubject,
	rawBody []byte, createReq *MediaVideoCreateRequest, payload any, idempotencyKey string,
) (data any, replayed bool, err error) {
	reqLog := logger.L().With(zap.Int64("user_id", apiKey.UserID), zap.Int64("api_key_id", apiKey.ID))
	if decision := h.checkSecurityAudit(c, reqLog, apiKey, subject, createReq.Model, rawBody); decision != nil && !decision.AllowNextStage {
		return nil, false, infraerrors.New(handler.SecurityAuditStatusForMedia(decision), handler.SecurityAuditErrorCodeForMedia(decision), handler.SecurityAuditMessageForMedia(decision))
	}
	if err := h.tasks.Provider().ValidateCreate(createReq); err != nil {
		return nil, false, err
	}
	return h.prepareTask(c.Request.Context(), apiKey, createReq, idempotencyKey)
}

// acquireAccountSlot enforces capacity; transport failures must not bypass it.
func (h *MediaTaskHandler) acquireAccountSlot(ctx context.Context, account *service.Account) (func(), error) {
	if h == nil || account == nil {
		return nil, ErrMediaAccountUnavailable
	}
	if !handler.ConcurrencyReady(h.concurrencyHelper) {
		return nil, ErrMediaTaskUnavailable
	}
	release, acquired, err := h.concurrencyHelper.TryAcquireAccountSlot(ctx, account.ID, account.Concurrency)
	if err != nil {
		return nil, ErrMediaTaskUnavailable
	}
	if !acquired {
		return nil, infraerrors.New(http.StatusTooManyRequests, "MEDIA_ACCOUNT_BUSY", "upstream account is busy; retry later")
	}
	return release, nil
}

// alertUpstreamBalanceExhausted 在上游返回 402 时打运营告警。
//
// 上游账号余额耗尽是运营事件而不是用户错误：用户侧只会看到出不了片，
// 没有这条告警运维无从知晓该补充上游额度。
func (h *MediaTaskHandler) alertUpstreamBalanceExhausted(err error, account *service.Account, apiKey *service.APIKey) {
	if err == nil || infraerrors.Reason(err) != infraerrors.Reason(ErrSeedanceInsufficientBalance) {
		return
	}
	logger.L().Error("ALERT media.upstream_account_balance_exhausted",
		zap.Int64("account_id", account.ID),
		zap.String("platform", account.Platform),
		zap.Int64("user_id", apiKey.UserID),
		zap.String("action", "top up the upstream account credits"),
	)
}

// selectAccount 选定上游账号。
//
// 带参考图时账号被锁定为图片所属账号且不允许更换——上游资源按账号隔离，
// 换账号必然 404，因此该账号不可用时直接失败而不是 failover。
func (h *MediaTaskHandler) selectAccount(ctx context.Context, apiKey *service.APIKey, model string, pinnedAccountID int64) (*service.Account, error) {
	if pinnedAccountID > 0 {
		account, err := h.gatewayService.GetMediaTaskAccountForCreate(ctx, pinnedAccountID)
		if err != nil || account == nil || !account.IsSchedulable() || !account.IsSeedance() || !account.IsModelSupported(model) {
			return nil, ErrMediaAccountUnavailable
		}
		belongs := false
		for _, id := range account.GroupIDs {
			if apiKey.GroupID != nil && id == *apiKey.GroupID {
				belongs = true
			}
		}
		if !belongs {
			return nil, ErrMediaAccountUnavailable
		}
		return account, nil
	}
	account, err := h.gatewayService.SelectAccountForModelWithExclusions(ctx, apiKey.GroupID, "", model, nil)
	if err != nil || account == nil || !account.IsSeedance() || account.Type != service.AccountTypeAPIKey {
		return nil, ErrMediaAccountUnavailable
	}
	return account, nil
}

// GetVideo 查询任务状态。状态与下载固定使用创建时绑定的账号。
func (h *MediaTaskHandler) GetVideo(c *gin.Context) {
	if !h.pollable() {
		mediaError(c, ErrMediaTaskUnavailable)
		return
	}
	apiKey, ok := h.mediaAuth(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	task, err := h.tasks.ResolveTask(ctx, c.Param("task_id"), apiKey.UserID, apiKey.ID)
	if err != nil {
		mediaError(c, err)
		return
	}

	downloadable, err := h.refreshTaskStatus(ctx, apiKey, task)
	if err != nil {
		mediaError(c, err)
		return
	}

	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, task.PublicTask(downloadable))
}

// Status reads the last durable worker result. Only fully accounted results
// are downloadable; transient upstream errors do not become false failures.
func (h *MediaTaskHandler) refreshTaskStatus(ctx context.Context, apiKey *service.APIKey, task *MediaTaskRecord) (bool, error) {
	// Status is a durable local read. The worker owns upstream polling and billing,
	// so a user's refresh frequency cannot fan out requests or duplicate charges.
	return task.Phase == mediaPhaseSettled, nil
}

// GetVideoContent 透传下载，支持 Range。签名地址每次重新获取，不持久化。
func (h *MediaTaskHandler) GetVideoContent(c *gin.Context) {
	if !h.pollable() {
		mediaError(c, ErrMediaTaskUnavailable)
		return
	}
	apiKey, ok := h.mediaAuth(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	task, err := h.tasks.ResolveTask(ctx, c.Param("task_id"), apiKey.UserID, apiKey.ID)
	if err != nil {
		mediaError(c, err)
		return
	}

	account, ref, err := h.fetchDownloadForTask(ctx, apiKey, task)
	if err != nil {
		mediaError(c, err)
		return
	}
	if ref == nil || ref.NotReady {
		mediaJSONError(c, http.StatusConflict, "not_ready", "media result is not ready yet")
		return
	}
	if streamErr := h.streamDownload(c, account, ref); streamErr != nil {
		mediaError(c, streamErr)
	}
}

// Download follows the source package's signed URL protocol, but only after
// capture, quotas and usage persistence are confirmed by the recovery worker.
func (h *MediaTaskHandler) fetchDownloadForTask(ctx context.Context, apiKey *service.APIKey, task *MediaTaskRecord) (*service.Account, *MediaDownloadRef, error) {
	if task.Phase != mediaPhaseSettled {
		return nil, &MediaDownloadRef{NotReady: true}, nil
	}
	if h.tasks.Provider() == nil {
		return nil, nil, ErrMediaTaskUnavailable
	}
	account, err := h.boundAccount(ctx, task)
	if err != nil {
		return nil, nil, err
	}
	ref, err := h.tasks.Provider().FetchDownloadRef(ctx, account, task.JobID)
	return account, ref, err
}

// streamDownload 透传下载字节，支持 Range；成功时写响应头与状态码并拷贝正文，
// 建单前失败以 error 返回交由各入口按自己的信封渲染。
func (h *MediaTaskHandler) streamDownload(c *gin.Context, account *service.Account, ref *MediaDownloadRef) error {
	resp, err := h.tasks.Provider().Download(c.Request.Context(), account, ref, c.GetHeader("Range"))
	if err != nil {
		return err
	}
	if resp.Close != nil {
		defer func() { _ = resp.Close() }()
	}
	for k, vs := range resp.Header {
		for _, v := range vs {
			c.Writer.Header().Add(k, v)
		}
	}
	c.Writer.Header().Set("Cache-Control", "no-store")
	c.Status(resp.StatusCode)
	if resp.Body != nil {
		_, _ = io.Copy(c.Writer, resp.Body)
	}
	return nil
}

// UploadFile 上传参考图，换取 SUB2 侧 file id。
//
// 上传不计费也不做幂等：重复上传只多占一份短期存储，不值得引入第二个幂等 scope。
func (h *MediaTaskHandler) UploadFile(c *gin.Context) {
	if !h.enabled() {
		mediaError(c, ErrMediaTaskDisabled)
		return
	}
	apiKey, ok := h.mediaAuth(c)
	if !ok {
		return
	}
	var body struct {
		ImageB64 string `json:"image_b64"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.ImageB64) == "" {
		mediaJSONError(c, http.StatusBadRequest, "invalid_request_error", "image_b64 is required")
		return
	}

	rec, err := h.uploadReferenceFile(c.Request.Context(), apiKey, body.ImageB64)
	if err != nil {
		mediaError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusCreated, rec.PublicFile())
}

// uploadReferenceFile 是上传参考图的形状无关编排：选账号 → 占槽 → 上传上游
// → 落库（TTL 不超过上游有效期）。供各上传入口共用；失败以 infraerror 返回。
func (h *MediaTaskHandler) uploadReferenceFile(ctx context.Context, apiKey *service.APIKey, imageB64 string) (*MediaFileRecord, error) {
	account, err := h.selectAccount(ctx, apiKey, "", 0)
	if err != nil || account == nil {
		return nil, ErrMediaAccountUnavailable
	}

	// 上传同样占用账号槽位，避免被当作免费图床刷量。
	release, slotErr := h.acquireAccountSlot(ctx, account)
	if slotErr != nil {
		return nil, slotErr
	}
	defer release()

	uploaded, err := h.tasks.Provider().UploadReferenceImage(ctx, account, imageB64)
	if err != nil {
		return nil, err
	}

	rec := &MediaFileRecord{
		ID:        NewMediaID(),
		UserID:    apiKey.UserID,
		APIKeyID:  apiKey.ID,
		AccountID: account.ID,
		Platform:  h.tasks.Provider().Platform(),
		ImageID:   uploaded.ImageID,
		Format:    uploaded.Format,
		Size:      uploaded.Size,
		ExpiresAt: uploaded.ExpiresAt,
		CreatedAt: time.Now().Unix(),
	}
	// 本地 TTL 不得超过上游有效期：上游过期后引用它必然失败。
	ttl := h.tasks.TaskTTL()
	if rec.ExpiresAt > 0 {
		if remain := time.Until(time.Unix(rec.ExpiresAt, 0)); remain > 0 && remain < ttl {
			ttl = remain
		}
	}
	if err := h.tasks.Store().SaveFile(ctx, rec, ttl); err != nil {
		return nil, ErrMediaTaskUnavailable.WithCause(err)
	}
	return rec, nil
}

func mediaError(c *gin.Context, err error) {
	status := infraerrors.Code(err)
	code := infraerrors.Reason(err)
	message := infraerrors.Message(err)
	if status <= 0 {
		status = http.StatusInternalServerError
	}
	if strings.TrimSpace(code) == "" {
		code = "MEDIA_TASK_ERROR"
	}
	mediaJSONError(c, status, code, message)
}

func mediaJSONError(c *gin.Context, status int, code, message string) {
	c.Header("Cache-Control", "no-store")
	c.JSON(status, gin.H{"error": gin.H{"type": code, "code": code, "message": message}})
}
