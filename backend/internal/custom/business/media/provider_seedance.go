package media

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// Seedance 上游协议适配。规格来源为上游 Seedance API 的冻结参考件：
//   - OpenAPI 机器规范（v1.3.0）
//   - 运行时能力快照（GET /v1/catalog）
//   - 开发者接入指南页快照
//
// 其中「资源按账号隔离」「signed_url 为站点相对路径」「未就绪返回 409」
// 三条只见于指南页，机器规范里没有，改动前必须回查该快照。
const (
	seedancePathVideos    = "/v1/videos"
	seedancePathFiles     = "/v1/files"
	seedancePathBalance   = "/v1/balance"
	seedanceSignedURLPath = "/signed_url"
	seedanceFilePath      = "/file"

	// 上游建议客户端读超时 ≥ 60 秒（指南页「接入须知」）。
	seedanceUpstreamTimeout = 90 * time.Second

	// 单图上限取「接口稳定优先」口径：规范的 ImageUploadResponse.size 声明
	// 上限为 12 MiB，而指南页正文写 50 MiB。两者冲突时取能保证上游不拒的
	// 那个——放行后被上游拒会浪费用户一次完整上传。实测确认可放宽后改此值即可。
	SeedanceDefaultMaxImageBytes = 12 * 1024 * 1024
	// 多图合计上限，指南页与规范一致。
	SeedanceDefaultMaxImagesTotalBytes = 70 * 1024 * 1024
)

var (
	ErrSeedanceInsufficientBalance = infraerrors.New(http.StatusPaymentRequired, "SEEDANCE_INSUFFICIENT_BALANCE", "upstream account has insufficient balance")
	ErrSeedanceKeyUnauthorized     = infraerrors.New(http.StatusBadGateway, "SEEDANCE_KEY_UNAUTHORIZED", "upstream rejected the account credential")
	ErrSeedanceForbidden           = infraerrors.New(http.StatusBadGateway, "SEEDANCE_FORBIDDEN", "upstream account is not entitled to this resource")
	ErrSeedanceUpstreamNotFound    = infraerrors.New(http.StatusNotFound, "SEEDANCE_UPSTREAM_NOT_FOUND", "upstream resource not found or not owned by this account")
	ErrSeedanceInvalidSignedURL    = infraerrors.New(http.StatusBadGateway, "SEEDANCE_INVALID_SIGNED_URL", "upstream returned an unsupported download reference")
	ErrSeedanceImageTooLarge       = infraerrors.New(http.StatusRequestEntityTooLarge, "SEEDANCE_IMAGE_TOO_LARGE", "reference image exceeds the allowed size")
	ErrSeedanceInvalidModel        = infraerrors.New(http.StatusBadRequest, "SEEDANCE_INVALID_MODEL", "unsupported model or duration combination")
	ErrSeedanceTooManyImages       = infraerrors.New(http.StatusBadRequest, "SEEDANCE_TOO_MANY_IMAGES", "too many reference images for this model")
	ErrSeedanceInvalidImageURL     = infraerrors.New(http.StatusBadRequest, "SEEDANCE_INVALID_IMAGE_URL", "reference image URL is not allowed")
	ErrSeedanceMissingCredential   = infraerrors.New(http.StatusBadGateway, "SEEDANCE_MISSING_CREDENTIAL", "seedance account has no api key configured")
)

// SeedanceProvider 实现 MediaProvider，只承载协议细节，不含任务生命周期。
type SeedanceProvider struct {
	upstream            HTTPUpstream
	maxImageBytes       int64
	maxImagesTotalBytes int64
}

func NewSeedanceProvider(upstream HTTPUpstream) *SeedanceProvider {
	return &SeedanceProvider{
		upstream:            upstream,
		maxImageBytes:       SeedanceDefaultMaxImageBytes,
		maxImagesTotalBytes: SeedanceDefaultMaxImagesTotalBytes,
	}
}

// WithImageLimits 覆盖参考图上限，供配置项注入与实测放宽后调整。
func (p *SeedanceProvider) WithImageLimits(single, total int64) *SeedanceProvider {
	if single > 0 {
		p.maxImageBytes = single
	}
	if total > 0 {
		p.maxImagesTotalBytes = total
	}
	return p
}

func (p *SeedanceProvider) Platform() string { return PlatformSeedance }

func (p *SeedanceProvider) Capabilities() MediaCapabilities {
	models := make([]MediaModelCapability, 0, 4)
	for _, id := range SeedanceModelIDs() {
		models = append(models, MediaModelCapability{
			ID:           id,
			Durations:    SeedanceModelDurations(id),
			MaxRefImages: SeedanceMaxReferenceImages(id),
		})
	}
	return MediaCapabilities{
		Platform:            PlatformSeedance,
		Models:              models,
		Ratios:              []string{"1:1", "3:4", "4:3", "9:16", "16:9", "21:9"},
		Resolutions:         []string{SeedanceResolution720},
		CameraMoves:         []string{"auto", "fixed"},
		MaxImageBytes:       p.maxImageBytes,
		MaxImagesTotalBytes: p.maxImagesTotalBytes,
	}
}

// ValidateCreate 在冻结余额与调用上游之前完成本地校验，避免无谓的上游往返。
func (p *SeedanceProvider) ValidateCreate(req *MediaVideoCreateRequest) error {
	if req == nil {
		return ErrSeedanceInvalidModel
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" || len([]rune(prompt)) > 6000 {
		return infraerrors.BadRequest("SEEDANCE_INVALID_PROMPT", "prompt is required and must be at most 6000 characters")
	}
	if !IsValidSeedanceDuration(req.Model, req.DurationSeconds) {
		return ErrSeedanceInvalidModel
	}
	if req.Resolution != "" && req.Resolution != SeedanceResolution720 {
		return infraerrors.BadRequest("SEEDANCE_INVALID_RESOLUTION", "only 720p is supported")
	}
	if req.Ratio != "" {
		valid := false
		for _, ratio := range p.Capabilities().Ratios {
			if req.Ratio == ratio {
				valid = true
			}
		}
		if !valid {
			return infraerrors.BadRequest("SEEDANCE_INVALID_RATIO", "unsupported video ratio")
		}
	}
	if req.CameraMovement != "" && req.CameraMovement != "auto" && req.CameraMovement != "fixed" {
		return infraerrors.BadRequest("SEEDANCE_INVALID_CAMERA", "unsupported camera movement")
	}
	// 上传图与直链合计计入同一张数上限。
	if total := len(req.FileIDs) + len(req.ImageURLs) + len(req.InlineImages); total > SeedanceMaxReferenceImages(req.Model) {
		return ErrSeedanceTooManyImages
	}
	for _, raw := range req.ImageURLs {
		if err := validateSeedancePublicImageURL(raw); err != nil {
			return err
		}
	}
	if err := validateSeedanceInlineImages(req.InlineImages, p.Capabilities()); err != nil {
		return err
	}
	return nil
}

// validateSeedancePublicImageURL 校验用户提供的公网参考图直链。
//
// 复用 channel_monitor_ssrf.go 的私网判定：isPrivateOrLoopbackHost 会解析
// 全部 A/AAAA 记录并逐个比对 loopback / RFC1918 / link-local / ULA 与主机名
// 黑名单，任一命中即拒绝。
func validateSeedancePublicImageURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 2048 {
		return ErrSeedanceInvalidImageURL
	}
	parsed, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.User != nil || parsed.Hostname() == "" {
		return ErrSeedanceInvalidImageURL
	}
	blocked, err := service.IsPrivateOrLoopbackHostForMedia(context.Background(), parsed.Hostname())
	if err != nil || blocked {
		return ErrSeedanceInvalidImageURL
	}
	return nil
}

type seedanceCreateBody struct {
	Prompt         string   `json:"prompt"`
	Model          string   `json:"model"`
	Duration       int      `json:"duration"`
	Ratio          string   `json:"ratio,omitempty"`
	Resolution     string   `json:"resolution,omitempty"`
	CameraMovement string   `json:"camera_movement,omitempty"`
	ImageIDs       []string `json:"image_ids,omitempty"`
	ImageURLs      []string `json:"image_urls,omitempty"`
}

type seedanceJobAccepted struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	Idempotent string `json:"idempotent"`
}

type seedanceVideoJob struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Model    string `json:"model"`
	Duration int    `json:"duration"`
	Error    string `json:"error"`
}

type seedanceSignedURL struct {
	URL       string `json:"url"`
	ExpiresAt int64  `json:"expires_at"`
}

type seedanceUploadResponse struct {
	ImageID   string `json:"image_id"`
	Format    string `json:"format"`
	Size      int64  `json:"size"`
	ExpiresAt int64  `json:"expires_at"`
}

type seedanceBalance struct {
	Balance *int64 `json:"balance"`
}

// Create 建单。上游成功码是 202，把 202 当失败是最常见的对接错误。
func (p *SeedanceProvider) Create(ctx context.Context, account *Account, req *MediaVideoCreateRequest, upstreamImageIDs []string, idempotencyKey string) (*MediaProviderJob, error) {
	body := seedanceCreateBody{
		Prompt:         strings.TrimSpace(req.Prompt),
		Model:          req.Model,
		Duration:       req.DurationSeconds,
		Ratio:          req.Ratio,
		Resolution:     req.Resolution,
		CameraMovement: req.CameraMovement,
		ImageIDs:       upstreamImageIDs,
		ImageURLs:      req.ImageURLs,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	// Persisted, owner-scoped upstream key is stable across retries and restarts.
	resp, err := p.do(ctx, account, http.MethodPost, seedancePathVideos, payload, map[string]string{
		"Idempotency-Key": idempotencyKey,
	})
	if err != nil {
		return nil, err
	}
	defer closeBody(resp)

	if resp.StatusCode != http.StatusAccepted {
		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		var problem seedanceErrorBody
		structured := json.Unmarshal(raw, &problem) == nil && (problem.Error.Code != "" || problem.Error.Type != "")
		resp.Body = io.NopCloser(bytes.NewReader(raw))
		err := p.upstreamError(resp)
		switch resp.StatusCode {
		case 400, 401, 402, 403, 413, 422:
			if structured {
				return nil, &SubmissionRejected{Cause: err}
			}
		}
		return nil, err
	}
	var accepted seedanceJobAccepted
	if err := decodeJSON(resp, &accepted); err != nil {
		return nil, err
	}
	if strings.TrimSpace(accepted.ID) == "" {
		return nil, infraerrors.New(http.StatusBadGateway, "SEEDANCE_EMPTY_JOB_ID", "upstream accepted the job without an id")
	}
	status := NormalizeMediaStatus(accepted.Status)
	if status == "" {
		status = MediaTaskStatusQueued
	}
	return &MediaProviderJob{
		JobID:  accepted.ID,
		Status: status,
		// 上游用 "1" 标记本次为幂等重放，仅供对账。
		Idempotent: accepted.Idempotent == "1",
	}, nil
}

// Status 查询任务状态。未知状态返回空字符串交由上层 fail-closed。
func (p *SeedanceProvider) Status(ctx context.Context, account *Account, jobID string) (*MediaProviderJob, error) {
	if strings.TrimSpace(jobID) == "" {
		return nil, ErrSeedanceUpstreamNotFound
	}
	resp, err := p.do(ctx, account, http.MethodGet, seedancePathVideos+"/"+url.PathEscape(jobID), nil, nil)
	if err != nil {
		return nil, err
	}
	defer closeBody(resp)
	if resp.StatusCode != http.StatusOK {
		return nil, p.upstreamError(resp)
	}
	var job seedanceVideoJob
	if err := decodeJSON(resp, &job); err != nil {
		return nil, err
	}
	if strings.TrimSpace(job.ID) == "" || job.ID != jobID {
		return nil, infraerrors.New(http.StatusBadGateway, "SEEDANCE_TASK_ID_MISMATCH", "upstream returned a different task")
	}
	return &MediaProviderJob{
		JobID: job.ID,
		// 不在此处归一化：保留上游原值，让上层的 NormalizeMediaStatus
		// 对未知状态 fail-closed，而不是在这里猜测成某个已知态。
		Status: job.Status,
		Error:  job.Error,
	}, nil
}

// FetchDownloadRef 取短期下载地址。
//
// 两个只见于指南页的关键差异：上游返回的是**站点相对路径**而非绝对 URL；
// 结果未就绪时返回 **409 而不是 404**。
func (p *SeedanceProvider) FetchDownloadRef(ctx context.Context, account *Account, jobID string) (*MediaDownloadRef, error) {
	if strings.TrimSpace(jobID) == "" {
		return nil, ErrSeedanceUpstreamNotFound
	}
	resp, err := p.do(ctx, account, http.MethodGet,
		seedancePathVideos+"/"+url.PathEscape(jobID)+seedanceSignedURLPath, nil, nil)
	if err != nil {
		return nil, err
	}
	defer closeBody(resp)

	// 409 = 尚未就绪，必须映射为「成功但暂不可下载」，不得当作 404 或失败。
	if resp.StatusCode == http.StatusConflict {
		return &MediaDownloadRef{NotReady: true}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, p.upstreamError(resp)
	}
	var signed seedanceSignedURL
	if err := decodeJSON(resp, &signed); err != nil {
		return nil, err
	}
	abs, err := resolveSeedanceDownloadURL(account.GetSeedanceBaseURL(), signed.URL)
	if err != nil {
		return nil, err
	}
	return &MediaDownloadRef{URL: abs, ExpiresAt: signed.ExpiresAt}, nil
}

// resolveSeedanceDownloadURL 校验并补全上游返回的下载地址。
//
// 上游返回的是站点相对路径，因此校验规则比「主机白名单」更严：必须是相对
// 路径（无 scheme、无 host、无 userinfo，以 / 开头），且规范化后不得逃出
// /v1/videos/ 前缀。任何带 scheme 或 host 的返回值一律拒绝——这样根本
// 不存在 SSRF 面，不需要维护主机白名单。
func resolveSeedanceDownloadURL(baseURL, rawRef string) (string, error) {
	ref := strings.TrimSpace(rawRef)
	if ref == "" {
		return "", ErrSeedanceInvalidSignedURL
	}
	base, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	if err != nil || base.Host == "" {
		return "", ErrSeedanceInvalidSignedURL
	}
	parsed, err := url.Parse(ref)
	if err != nil || parsed.User != nil {
		return "", ErrSeedanceInvalidSignedURL
	}

	// 上游实测返回的是绝对 URL（https://<base host>/v1/videos/<job>/file?exp=&sig=），
	// 而非站点相对路径。绝对形式只在 scheme 与 host 同账号 base_url 完全一致时放行：
	// 指向任何其它主机的引用一律拒绝，SSRF 面因此仍然为零。
	// 相对形式继续沿用原有口径（必须 / 开头且不得为 //，后者会被解析成 host）。
	if parsed.Scheme != "" || parsed.Host != "" {
		if !strings.EqualFold(parsed.Scheme, base.Scheme) || !strings.EqualFold(parsed.Host, base.Host) {
			return "", ErrSeedanceInvalidSignedURL
		}
	} else if !strings.HasPrefix(ref, "/") || strings.HasPrefix(ref, "//") {
		return "", ErrSeedanceInvalidSignedURL
	}

	cleaned := path.Clean(parsed.EscapedPath())
	if !strings.HasPrefix(cleaned, seedancePathVideos+"/") {
		return "", ErrSeedanceInvalidSignedURL
	}
	out := *base
	out.Path = cleaned
	out.RawQuery = parsed.RawQuery
	return out.String(), nil
}

// Download 透传下载响应，含 Range 与 206 / 416。
func (p *SeedanceProvider) Download(ctx context.Context, account *Account, ref *MediaDownloadRef, rangeHeader string) (*MediaDownloadResponse, error) {
	if ref == nil || strings.TrimSpace(ref.URL) == "" {
		return nil, ErrSeedanceInvalidSignedURL
	}
	headers := map[string]string{}
	if strings.TrimSpace(rangeHeader) != "" {
		headers["Range"] = rangeHeader
	}
	resp, err := p.doAbsolute(ctx, account, http.MethodGet, ref.URL, nil, headers)
	if err != nil {
		return nil, err
	}
	switch resp.StatusCode {
	case http.StatusOK, http.StatusPartialContent, http.StatusRequestedRangeNotSatisfiable:
		out := &MediaDownloadResponse{
			StatusCode: resp.StatusCode,
			Header:     filterSeedanceDownloadHeaders(resp.Header),
			Body:       resp.Body,
			Close:      resp.Body.Close,
		}
		return out, nil
	default:
		defer closeBody(resp)
		return nil, p.upstreamError(resp)
	}
}

// filterSeedanceDownloadHeaders 只转发下载所需的响应头，避免把上游的
// 鉴权、限流等内部头透给客户端。
func filterSeedanceDownloadHeaders(in http.Header) http.Header {
	out := http.Header{}
	for _, k := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "Content-Disposition", "Last-Modified", "ETag"} {
		if v := in.Get(k); v != "" {
			out.Set(k, v)
		}
	}
	return out
}

// UploadReferenceImage 上传参考图。上游收 JSON 的 image_b64，不走 multipart。
func (p *SeedanceProvider) UploadReferenceImage(ctx context.Context, account *Account, imageB64 string) (*MediaProviderFile, error) {
	trimmed := strings.TrimSpace(imageB64)
	if trimmed == "" {
		return nil, infraerrors.BadRequest("SEEDANCE_EMPTY_IMAGE", "image_b64 is required")
	}
	// 按解码后体积估算拦截（base64 每 4 字符解出 3 字节），避免把明显超限的
	// 图片送到上游再被拒，白白浪费一次完整上传。
	if estimated := int64(len(trimmed)) / 4 * 3; estimated > p.maxImageBytes {
		return nil, ErrSeedanceImageTooLarge
	}
	payload, err := json.Marshal(map[string]string{"image_b64": trimmed})
	if err != nil {
		return nil, err
	}
	resp, err := p.do(ctx, account, http.MethodPost, seedancePathFiles, payload, nil)
	if err != nil {
		return nil, err
	}
	defer closeBody(resp)
	if resp.StatusCode != http.StatusCreated {
		return nil, p.upstreamError(resp)
	}
	var uploaded seedanceUploadResponse
	if err := decodeJSON(resp, &uploaded); err != nil {
		return nil, err
	}
	if strings.TrimSpace(uploaded.ImageID) == "" {
		return nil, infraerrors.New(http.StatusBadGateway, "SEEDANCE_EMPTY_IMAGE_ID", "upstream stored the image without an id")
	}
	return &MediaProviderFile{
		ImageID:   uploaded.ImageID,
		Format:    uploaded.Format,
		Size:      uploaded.Size,
		ExpiresAt: uploaded.ExpiresAt,
	}, nil
}

// ProbeCredential 用受保护端点探测凭证有效性。
//
// 刻意不用公开的 /v1/catalog 或 /health：它们不带鉴权也返回 200，
// 探测通过并不能证明该账号的 Key 可用。
func (p *SeedanceProvider) ProbeCredential(ctx context.Context, account *Account) error {
	resp, err := p.do(ctx, account, http.MethodGet, seedancePathBalance, nil, nil)
	if err != nil {
		return err
	}
	defer closeBody(resp)
	if resp.StatusCode != http.StatusOK {
		return p.upstreamError(resp)
	}
	var balance seedanceBalance
	if err := decodeJSON(resp, &balance); err != nil {
		return err
	}
	// 结构有效性同样是判据：2xx 但解析不出 balance 说明打到的不是预期端点。
	if balance.Balance == nil {
		return infraerrors.New(http.StatusBadGateway, "SEEDANCE_PROBE_INVALID_RESPONSE", "balance probe returned an unexpected payload")
	}
	return nil
}

func (p *SeedanceProvider) do(ctx context.Context, account *Account, method, relPath string, body []byte, headers map[string]string) (*http.Response, error) {
	base := account.GetSeedanceBaseURL()
	if base == "" {
		return nil, ErrSeedanceMissingCredential
	}
	return p.doAbsolute(ctx, account, method, base+relPath, body, headers)
}

func (p *SeedanceProvider) doAbsolute(ctx context.Context, account *Account, method, absURL string, body []byte, headers map[string]string) (*http.Response, error) {
	if p.upstream == nil {
		return nil, infraerrors.New(http.StatusServiceUnavailable, "SEEDANCE_UPSTREAM_UNAVAILABLE", "upstream transport is not configured")
	}
	apiKey := account.GetSeedanceAPIKey()
	if apiKey == "" {
		return nil, ErrSeedanceMissingCredential
	}

	reqCtx, cancel := context.WithTimeout(ctx, seedanceUpstreamTimeout)
	// 取消函数挂到响应体关闭上；下载响应体在上层读完后才关。
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(reqCtx, method, absURL, reader)
	if err != nil {
		cancel()
		return nil, err
	}
	// Bearer 后必须有空格；不接受 X-API-Key 之类的替代写法。
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		if strings.TrimSpace(v) != "" {
			req.Header.Set(k, v)
		}
	}

	// 与 Grok 媒体同一套出口治理：账号级代理与并发计数。
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := p.upstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		cancel()
		return nil, err
	}
	resp.Body = &seedanceBodyWithCancel{ReadCloser: resp.Body, cancel: cancel}
	return resp, nil
}

// seedanceBodyWithCancel 保证请求 context 在响应体关闭时才释放，
// 避免下载还没读完就被超时取消。
type seedanceBodyWithCancel struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *seedanceBodyWithCancel) Close() error {
	err := b.ReadCloser.Close()
	b.cancel()
	return err
}

type seedanceErrorBody struct {
	Error struct {
		Code      string `json:"code"`
		Type      string `json:"type"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
	} `json:"error"`
	RetryAfter int `json:"retry_after"`
}

// upstreamError 把上游错误映射为对外错误，保留 type / message / request_id。
//
// 402 / 403 / 404 各有明确语义，不得笼统转成 500：402 意味着上游账号余额
// 耗尽（运营事件），403 是该 Key 未绑定或无权，404 常见于资源不属于当前账号。
func (p *SeedanceProvider) upstreamError(resp *http.Response) error {
	var parsed seedanceErrorBody
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	_ = json.Unmarshal(raw, &parsed)

	msg := strings.TrimSpace(parsed.Error.Message)
	if msg == "" {
		msg = fmt.Sprintf("upstream returned %d", resp.StatusCode)
	}
	meta := map[string]string{}
	if rid := strings.TrimSpace(parsed.Error.RequestID); rid != "" {
		meta["upstream_request_id"] = rid
	}
	if t := strings.TrimSpace(parsed.Error.Type); t != "" {
		meta["upstream_type"] = t
	}

	var base *infraerrors.ApplicationError
	switch resp.StatusCode {
	case http.StatusPaymentRequired:
		base = ErrSeedanceInsufficientBalance
	case http.StatusUnauthorized:
		base = ErrSeedanceKeyUnauthorized
	case http.StatusForbidden:
		base = ErrSeedanceForbidden
	case http.StatusNotFound:
		base = ErrSeedanceUpstreamNotFound
	case http.StatusTooManyRequests:
		// Retry-After 有两个来源：响应头与 JSON 体，取先出现的那个。
		retry := strings.TrimSpace(resp.Header.Get("Retry-After"))
		if retry == "" && parsed.RetryAfter > 0 {
			retry = strconv.Itoa(parsed.RetryAfter)
		}
		if retry != "" {
			meta["retry_after"] = retry
		}
		base = infraerrors.New(http.StatusTooManyRequests, "SEEDANCE_RATE_LIMITED", msg)
	case http.StatusRequestEntityTooLarge:
		base = ErrSeedanceImageTooLarge
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		base = infraerrors.BadRequest("SEEDANCE_UPSTREAM_REJECTED", msg)
	default:
		base = infraerrors.New(http.StatusBadGateway, "SEEDANCE_UPSTREAM_ERROR", msg)
	}
	if len(meta) > 0 {
		return base.WithMetadata(meta)
	}
	return base
}

func decodeJSON(resp *http.Response, out any) error {
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return infraerrors.New(http.StatusBadGateway, "SEEDANCE_INVALID_RESPONSE", "upstream returned an unparsable payload")
	}
	return nil
}

func closeBody(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
}
