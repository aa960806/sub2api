package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const GrokVideoResponseFormatOpenAI = "openai"

const grokVideoResponseFormatContextKey = "grok_video_response_format"

// GrokVideoCreateResponseFormat distinguishes the OpenAI-compatible create
// alias from xAI's native /videos/generations endpoint.
func GrokVideoCreateResponseFormat(c *gin.Context) string {
	if c == nil || c.Request == nil || c.Request.URL == nil || c.Request.Method != http.MethodPost {
		return ""
	}
	switch c.Request.URL.Path {
	case "/videos", "/v1/videos":
		return GrokVideoResponseFormatOpenAI
	default:
		return ""
	}
}

// IsGrokVideoShortStatusRequest identifies the shared OpenAI/xAI lookup route.
// The task's persisted create-time format, not this path alone, determines the
// response protocol so existing native clients keep receiving status=done.
func IsGrokVideoShortStatusRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.URL == nil || c.Request.Method != http.MethodGet {
		return false
	}
	path := strings.TrimPrefix(c.Request.URL.Path, "/v1")
	if !strings.HasPrefix(path, "/videos/") {
		return false
	}
	id := strings.TrimPrefix(path, "/videos/")
	return id != "" && !strings.Contains(id, "/")
}

// BindGrokVideoResponseFormat must be called only after task ownership has
// been verified. A missing format is the historical native xAI contract.
func BindGrokVideoResponseFormat(c *gin.Context, format string) {
	if c != nil {
		c.Set(grokVideoResponseFormatContextKey, format)
	}
}

// grokVideoClientResponse changes presentation only. The caller must compute
// usage and billing metadata from the untouched upstream response first.
func grokVideoClientResponse(c *gin.Context, endpoint GrokMediaEndpoint, requestID string, body []byte) ([]byte, error) {
	create := endpoint == GrokMediaEndpointVideosGenerations && GrokVideoCreateResponseFormat(c) == GrokVideoResponseFormatOpenAI
	lookup := endpoint == GrokMediaEndpointVideoStatus && IsGrokVideoShortStatusRequest(c) && c.GetString(grokVideoResponseFormatContextKey) == GrokVideoResponseFormatOpenAI
	if !create && !lookup {
		return body, nil
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil || payload == nil {
		return nil, fmt.Errorf("xAI returned an invalid video task response")
	}
	id := strings.TrimSpace(requestID)
	if create {
		id = extractGrokMediaVideoRequestID(body)
	}
	if id == "" {
		return nil, fmt.Errorf("xAI returned a video task without a request ID")
	}
	payload["id"] = grokVideoJSONValue(id)
	payload["object"] = grokVideoJSONValue("video")
	if _, exists := payload["request_id"]; !exists {
		payload["request_id"] = grokVideoJSONValue(id)
	}
	status := strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "status").String()))
	switch status {
	case "", "pending":
		if create {
			status = "queued"
		} else {
			status = "in_progress"
		}
	case "queued", "in_progress":
	case "done":
		if strings.TrimSpace(gjson.GetBytes(body, "video.url").String()) == "" {
			return nil, fmt.Errorf("xAI completed a video task without a video URL")
		}
		status = "completed"
	case "expired", "failed":
		if _, exists := payload["error"]; !exists {
			payload["error"] = grokVideoJSONValue(map[string]string{"code": "video_generation_" + status, "message": "Video generation " + status})
		}
		status = "failed"
	default:
		return nil, fmt.Errorf("xAI returned an unsupported video task status")
	}
	payload["status"] = grokVideoJSONValue(status)
	return json.Marshal(payload)
}
