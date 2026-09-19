//go:build unit

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSeedanceTaskReadsRetainAuthenticationWithoutBillingGate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name, platform, status string
		userActive             bool
		want                   int
	}{
		{"reserved balance", service.PlatformSeedance, service.StatusAPIKeyQuotaExhausted, true, http.StatusOK},
		{"expired key", service.PlatformSeedance, service.StatusAPIKeyExpired, true, http.StatusOK},
		{"disabled key", service.PlatformSeedance, service.StatusDisabled, true, http.StatusUnauthorized},
		{"disabled user", service.PlatformSeedance, service.StatusActive, false, http.StatusUnauthorized},
		{"other platform", service.PlatformOpenAI, service.StatusAPIKeyQuotaExhausted, true, http.StatusTooManyRequests},
	} {
		t.Run(test.name, func(t *testing.T) {
			group := &service.Group{ID: 1, Platform: test.platform, Status: service.StatusActive, Hydrated: true, SubscriptionType: service.SubscriptionTypeStandard}
			user := &service.User{ID: 2, Role: service.RoleUser, Status: service.StatusActive, Balance: 0, Concurrency: 1}
			if !test.userActive {
				user.Status = service.StatusDisabled
			}
			key := &service.APIKey{ID: 3, Key: "test", UserID: user.ID, User: user, GroupID: &group.ID, Group: group, Status: test.status, Quota: 1, QuotaUsed: 1}
			repo := &stubApiKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) { clone := *key; return &clone, nil }, updateLastUsed: func(context.Context, int64, time.Time) error { return nil }}
			cfg := &config.Config{RunMode: config.RunModeStandard}
			svc := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, cfg)
			r := gin.New()
			r.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(svc, nil, cfg)))
			r.GET("/v1/contents/generations/tasks/:task_id", func(c *gin.Context) { c.Status(http.StatusOK) })
			for _, prefix := range []string{"/v1", ""} {
				r.GET(prefix+"/media/videos/:task_id", func(c *gin.Context) { c.Status(http.StatusOK) })
				r.GET(prefix+"/media/videos/:task_id/content", func(c *gin.Context) { c.Status(http.StatusOK) })
			}
			for _, path := range []string{"/v1/media/videos/job", "/v1/media/videos/job/content", "/media/videos/job", "/media/videos/job/content", "/v1/contents/generations/tasks/job"} {
				w := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodGet, path, nil)
				req.Header.Set("x-api-key", key.Key)
				r.ServeHTTP(w, req)
				require.Equal(t, test.want, w.Code, "%s: %s", path, w.Body.String())
			}
			if test.want == http.StatusOK {
				for _, route := range []string{"/v1/media/models", "/v1/media/videos/:task_id/charge", "/v1/media/videos"} {
					r.GET(route, func(c *gin.Context) { c.Status(http.StatusOK) })
					w := httptest.NewRecorder()
					req := httptest.NewRequest(http.MethodGet, strings.ReplaceAll(route, ":task_id", "job"), nil)
					req.Header.Set("x-api-key", key.Key)
					r.ServeHTTP(w, req)
					require.NotEqual(t, http.StatusOK, w.Code, route)
				}
			}
		})
	}
}
