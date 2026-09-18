package middleware

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// A reserved job remains readable after its reservation consumes the balance.
// Use registered route templates, not a prefix, so this exemption cannot grant
// access to future chargeable endpoints. Authentication/IP/user/group policy
// still runs; the media handler verifies both owner and original API key.
func isSeedanceTaskRead(c *gin.Context, apiKey *service.APIKey) bool {
	if c.Request.Method != http.MethodGet || apiKey.Group == nil || apiKey.Group.Platform != service.PlatformSeedance {
		return false
	}
	switch c.FullPath() {
	case "/v1/media/videos/:task_id", "/v1/media/videos/:task_id/content", "/media/videos/:task_id", "/media/videos/:task_id/content":
		return true
	default:
		return false
	}
}
