package routes

import (
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// Scope is checked before composite routing. A Seedance key must never enter
// text/Grok scheduling, and another platform cannot access independent media.
func seedanceGatewayScope() gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey, ok := middleware.GetAPIKeyFromContext(c)
		if !ok || apiKey == nil || apiKey.Group == nil {
			c.Next()
			return
		}
		path := c.FullPath()
		mediaRoute := strings.HasPrefix(path, "/v1/media/") || strings.HasPrefix(path, "/media/")
		mediaRoute = mediaRoute || path == "/v1/contents/generations/tasks" || path == "/v1/contents/generations/tasks/:task_id"
		seedance := apiKey.Group.Platform == service.PlatformSeedance
		shared := path == "/v1/usage" || path == "/v1/sub2api/billing" || path == "/v1/models" || path == "/models"
		if mediaRoute != seedance && !shared {
			service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalFeatureGate)
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": gin.H{
				"type": "not_found_error", "message": "This endpoint is not supported for this platform",
			}})
			return
		}
		c.Next()
	}
}
