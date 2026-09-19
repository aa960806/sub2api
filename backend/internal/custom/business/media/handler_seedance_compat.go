package media

// Compatibility for Canvas/Ark clients, isolated from OpenAI and Grok routes.
// Upstream submission, reservations, recovery and settlement remain in the
// existing media lifecycle; this file only translates the downstream protocol.
import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type seedanceContentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Role     string `json:"role"`
	ImageURL *struct {
		URL string `json:"url"`
	} `json:"image_url"`
}

type seedanceCompatRequest struct {
	Model         string                `json:"model"`
	Content       []seedanceContentPart `json:"content"`
	Duration      int                   `json:"duration"`
	Ratio         string                `json:"ratio"`
	Resolution    string                `json:"resolution"`
	CameraFixed   *bool                 `json:"camera_fixed"`
	GenerateAudio *bool                 `json:"generate_audio"`
	Watermark     *bool                 `json:"watermark"`
}

const seedanceCompatWarning = "This provider controls audio and watermark output; Canvas defaults are accepted but these switches are not configurable. Only reference images are supported."

func (h *MediaTaskHandler) SeedanceModels(c *gin.Context) {
	if h == nil || h.tasks == nil || h.tasks.Provider() == nil {
		mediaError(c, ErrMediaTaskDisabled)
		return
	}
	key, ok := h.mediaAuth(c)
	if !ok {
		return
	}
	data := make([]gin.H, 0)
	for _, model := range h.tasks.Provider().Capabilities().Models {
		if key.Group.ModelAllowlistEnabled() && !key.Group.ModelAllowlist.Allows(model.ID) {
			continue
		}
		data = append(data, gin.H{"id": model.ID, "object": "model", "created": 0, "owned_by": "seedance",
			"durations": model.Durations, "resolutions": []string{"720p"}, "max_reference_images": model.MaxRefImages})
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": data})
}

// CreateSeedanceTask feeds the normalized prompt/images into submitVideoCreate,
// whose checkSecurityAudit must finish before reference uploads or reservation.
func (h *MediaTaskHandler) CreateSeedanceTask(c *gin.Context) {
	if !h.enabled() {
		mediaError(c, ErrMediaTaskDisabled)
		return
	}
	key, ok := h.mediaAuth(c)
	if !ok {
		return
	}
	if len(h.tasks.downloadSigningKey) == 0 {
		mediaError(c, ErrMediaTaskUnavailable)
		return
	}
	if mediaType := c.ContentType(); mediaType != "application/json" {
		mediaJSONError(c, http.StatusBadRequest, "invalid_request_error", "application/json is required")
		return
	}
	raw, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	var input seedanceCompatRequest
	if err != nil || json.Unmarshal(raw, &input) != nil {
		mediaJSONError(c, http.StatusBadRequest, "invalid_request_error", "invalid request body")
		return
	}
	req, err := normalizeSeedanceCompat(input)
	if err != nil {
		mediaError(c, err)
		return
	}
	if err := validateSeedanceInlineImages(req.InlineImages, h.tasks.Provider().Capabilities()); err != nil {
		mediaError(c, err)
		return
	}
	idem := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if len(idem) > 200 {
		mediaJSONError(c, http.StatusBadRequest, "invalid_request_error", "Idempotency-Key must be at most 200 characters")
		return
	}
	// Canvas does not send a key. Each explicit click is a new task; worker
	// retries still share its durable upstream key. Never deduplicate prompts.
	if idem == "" {
		idem = NewMediaID()
	}
	c.Header("Idempotency-Key", idem)
	subject, _ := middleware2.GetAuthSubjectFromContext(c)
	images := make([]gin.H, 0)
	for _, part := range input.Content {
		if part.Type == "image_url" && part.ImageURL != nil {
			images = append(images, gin.H{"type": "image_url", "image_url": gin.H{"url": strings.TrimSpace(part.ImageURL.URL)}})
		}
	}
	auditBody, _ := json.Marshal(map[string]any{"model": req.Model, "prompt": req.Prompt, "images": images})
	data, replayed, err := h.submitVideoCreate(c, key, subject, auditBody, req, input, idem)
	if err != nil {
		if retry := service.RetryAfterSecondsFromError(err); retry > 0 {
			c.Header("Retry-After", strconv.Itoa(retry))
		}
		mediaError(c, err)
		return
	}
	if replayed {
		c.Header("X-Idempotency-Replayed", "true")
	}
	// Look up the durable record for the same settlement-gated view as polling.
	public := data.(*MediaTask)
	task, err := h.tasks.ResolveTask(c.Request.Context(), public.TaskID, key.UserID, key.ID)
	if err != nil {
		mediaError(c, err)
		return
	}
	h.writeSeedanceTask(c, task, http.StatusAccepted)
}

func normalizeSeedanceCompat(input seedanceCompatRequest) (*MediaVideoCreateRequest, error) {
	bad := func(message string) (*MediaVideoCreateRequest, error) {
		return nil, infraerrors.BadRequest("SEEDANCE_UNSUPPORTED_PARAMETER", message)
	}
	if input.GenerateAudio != nil && !*input.GenerateAudio || input.Watermark != nil && *input.Watermark {
		return bad("This Seedance provider does not support audio/watermark switches; keep Canvas defaults (generate_audio=true, watermark=false)")
	}
	req := &MediaVideoCreateRequest{Model: strings.TrimSpace(input.Model), DurationSeconds: input.Duration,
		Ratio: strings.TrimSpace(input.Ratio), Resolution: strings.TrimSpace(input.Resolution)}
	if !IsValidSeedanceDuration(req.Model, req.DurationSeconds) {
		return bad(fmt.Sprintf("%s supports durations %v seconds; select a supported duration in Canvas", req.Model, SeedanceModelDurations(req.Model)))
	}
	if req.Resolution == "" {
		req.Resolution = "720p"
	}
	if req.Resolution != "720p" {
		return bad("This Seedance provider only supports 720p")
	}
	// Canvas's auto/adaptive means no fixed aspect ratio. Preserve that meaning
	// by leaving the upstream optional ratio unset instead of inventing a ratio.
	if req.Ratio == "adaptive" || req.Ratio == "auto" {
		req.Ratio = ""
	}
	if input.CameraFixed != nil {
		req.CameraMovement = "auto"
		if *input.CameraFixed {
			req.CameraMovement = "fixed"
		}
	}
	var text []string
	for _, part := range input.Content {
		switch part.Type {
		case "text":
			if strings.TrimSpace(part.Text) != "" {
				text = append(text, part.Text)
			}
		case "image_url":
			if part.ImageURL == nil || strings.TrimSpace(part.ImageURL.URL) == "" {
				return bad("image_url.url is required")
			}
			if part.Role != "" && part.Role != "reference_image" {
				return bad("Only reference_image is supported; first/last-frame roles are not supported by this provider")
			}
			raw := strings.TrimSpace(part.ImageURL.URL)
			if strings.HasPrefix(raw, "data:") {
				prefix, encoded, found := strings.Cut(raw, ",")
				if !found || (prefix != "data:image/png;base64" && prefix != "data:image/jpeg;base64" && prefix != "data:image/webp;base64") {
					return bad("Inline references must be base64 PNG, JPEG or WebP images")
				}
				req.InlineImages = append(req.InlineImages, encoded)
			} else {
				req.ImageURLs = append(req.ImageURLs, raw)
			}
		default:
			return bad("This Seedance provider supports text and reference images only; reference video/audio and asset IDs are not supported")
		}
	}
	req.Prompt = strings.Join(text, "\n")
	if len(req.InlineImages)+len(req.ImageURLs) > SeedanceMaxReferenceImages(req.Model) {
		return nil, ErrSeedanceTooManyImages
	}
	// Keep reference order well-defined: upstream accepts image_ids then URLs.
	// Mixing the two would silently reorder numbered references in the prompt.
	if len(req.InlineImages) > 0 && len(req.ImageURLs) > 0 {
		return bad("Use either uploaded images or public image URLs in one task; mixing reference sources is not supported")
	}
	return req, nil
}

func validateSeedanceInlineImages(images []string, capabilities MediaCapabilities) error {
	var total int64
	maxSingle, maxTotal := capabilities.MaxImageBytes, capabilities.MaxImagesTotalBytes
	if maxSingle <= 0 {
		maxSingle = 10 << 20
	}
	if maxTotal <= 0 {
		maxTotal = 30 << 20
	}
	for _, encoded := range images {
		if int64(base64.StdEncoding.DecodedLen(len(encoded))) > maxSingle {
			return ErrSeedanceImageTooLarge
		}
		decoded, err := base64.StdEncoding.Strict().DecodeString(encoded)
		if err != nil || len(decoded) == 0 {
			return infraerrors.BadRequest("SEEDANCE_INVALID_IMAGE", "invalid base64 reference image")
		}
		switch http.DetectContentType(decoded) {
		case "image/png", "image/jpeg", "image/webp":
		default:
			return infraerrors.BadRequest("SEEDANCE_INVALID_IMAGE", "reference image must be PNG, JPEG or WebP")
		}
		total += int64(len(decoded))
		if total > maxTotal {
			return ErrSeedanceImageTooLarge
		}
	}
	return nil
}

func (h *MediaTaskHandler) uploadSeedanceInlineImages(ctx context.Context, account *Account, images []string) ([]string, error) {
	release, err := h.acquireAccountSlot(ctx, account)
	if err != nil {
		return nil, err
	}
	defer release()
	ids := make([]string, 0, len(images))
	for _, image := range images {
		file, err := h.tasks.Provider().UploadReferenceImage(ctx, account, image)
		if err != nil {
			return nil, err
		}
		if file == nil || file.ImageID == "" {
			return nil, ErrMediaTaskUnavailable
		}
		ids = append(ids, file.ImageID)
	}
	return ids, nil
}

func (h *MediaTaskHandler) GetSeedanceTask(c *gin.Context) {
	if !h.pollable() {
		mediaError(c, ErrMediaTaskUnavailable)
		return
	}
	key, ok := h.mediaAuth(c)
	if !ok {
		return
	}
	task, err := h.tasks.ResolveTask(c.Request.Context(), c.Param("task_id"), key.UserID, key.ID)
	if err != nil {
		mediaError(c, err)
		return
	}
	h.writeSeedanceTask(c, task, http.StatusOK)
}

func (h *MediaTaskHandler) writeSeedanceTask(c *gin.Context, task *MediaTaskRecord, code int) {
	status := task.Status
	if status == MediaTaskStatusSucceeded && task.Phase != mediaPhaseSettled {
		status = MediaTaskStatusRunning
	}
	response := gin.H{"id": task.ID, "model": task.Model, "status": status, "created_at": task.CreatedAt,
		"duration": task.DurationSeconds, "resolution": task.Resolution, "ratio": task.Ratio,
		"warnings": []string{seedanceCompatWarning}}
	if task.Phase == mediaPhaseSettled {
		resultURL, err := h.seedanceResultURL(c, task)
		if err != nil {
			mediaError(c, err)
			return
		}
		response["content"] = gin.H{"video_url": resultURL}
	} else if status == MediaTaskStatusFailed {
		response["error"] = gin.H{"code": "generation_failed", "message": "Video generation failed"}
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(code, response)
}
