package media

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const seedanceResultTTL = 10 * time.Minute

// Derive a separate domain key; no JWT, API key or upstream signed URL is
// exposed to the browser. Rotation of JWT secret also revokes these URLs.
func (s *MediaTaskService) withDownloadSigningSecret(secret string) {
	if strings.TrimSpace(secret) == "" {
		return
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("subnexus.seedance.result.v1"))
	s.downloadSigningKey = mac.Sum(nil)
}

func (s *MediaTaskService) resultSignature(id, expires string) []byte {
	mac := hmac.New(sha256.New, s.downloadSigningKey)
	_, _ = mac.Write([]byte("seedance-result\x00" + id + "\x00" + expires))
	return mac.Sum(nil)
}

func (h *MediaTaskHandler) seedanceResultURL(c *gin.Context, task *MediaTaskRecord) (string, error) {
	if len(h.tasks.downloadSigningKey) == 0 || task.Phase != mediaPhaseSettled || task.Platform != PlatformSeedance {
		return "", ErrMediaTaskUnavailable
	}
	// Use the request's authority (never X-Forwarded-Host). A signature confers
	// access to this already-paid result only, not to any other API operation.
	scheme := "http"
	if c.Request.TLS != nil || strings.EqualFold(strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")), "https") {
		scheme = "https"
	}
	base, err := url.Parse(scheme + "://" + c.Request.Host)
	if err != nil || base.Host == "" || base.User != nil || base.Path != "" || base.RawQuery != "" || base.Fragment != "" {
		return "", ErrMediaTaskUnavailable
	}
	expires := strconv.FormatInt(time.Now().Add(seedanceResultTTL).Unix(), 10)
	base.Path = "/v1/media/results/" + task.ID
	base.RawQuery = url.Values{"expires": {expires}, "signature": {base64.RawURLEncoding.EncodeToString(h.tasks.resultSignature(task.ID, expires))}}.Encode()
	return base.String(), nil
}

// GetSeedanceResult is a narrowly scoped capability endpoint for clients that
// cannot attach Authorization to video URLs. Only an authenticated task owner
// can obtain the signature through GetSeedanceTask. It expires after 10 min;
// refreshing the authenticated task issues a new URL. No query API keys allowed.
func (h *MediaTaskHandler) GetSeedanceResult(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("X-Content-Type-Options", "nosniff")
	if !h.pollable() || len(h.tasks.downloadSigningKey) == 0 {
		mediaError(c, ErrMediaTaskUnavailable)
		return
	}
	id, expires := c.Param("task_id"), c.Query("expires")
	exp, err := strconv.ParseInt(expires, 10, 64)
	_, idErr := uuid.Parse(id)
	signature, sigErr := base64.RawURLEncoding.DecodeString(c.Query("signature"))
	now := time.Now().Unix()
	if err != nil || idErr != nil || sigErr != nil || exp <= now || exp > now+int64(seedanceResultTTL.Seconds()) ||
		!hmac.Equal(signature, h.tasks.resultSignature(id, expires)) {
		mediaJSONError(c, http.StatusForbidden, "invalid_result_signature", "Result URL is invalid or expired; query the task again with your API key")
		return
	}
	task, err := h.tasks.Store().GetTask(c.Request.Context(), id)
	if err != nil {
		mediaError(c, err)
		return
	}
	if task.Platform != PlatformSeedance || task.Phase != mediaPhaseSettled {
		mediaError(c, ErrMediaTaskNotFound)
		return
	}
	account, ref, err := h.fetchDownloadForTask(c.Request.Context(), nil, task)
	if err != nil {
		mediaError(c, err)
		return
	}
	if ref == nil || ref.NotReady {
		mediaJSONError(c, http.StatusConflict, "not_ready", "media result is not ready yet")
		return
	}
	if err := h.streamDownload(c, account, ref); err != nil {
		mediaError(c, err)
	}
}
