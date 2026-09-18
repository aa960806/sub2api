//go:build embed

package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmbeddedFrontendMediaRoutes(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		name := "settings_injection"
		if legacy {
			name = "legacy"
		}
		t.Run(name, func(t *testing.T) {
			provider := &mockSettingsProvider{settings: map[string]string{"site_name": "Media route test"}}
			server, err := NewFrontendServer(provider)
			require.NoError(t, err)
			frontend := server.Middleware()
			if legacy {
				frontend = ServeEmbeddedFrontend()
			}
			router := gin.New()
			router.Use(frontend)

			// Exercise actual middleware dispatch for every registered media route,
			// including the disabled-feature JSON error that SPA fallback swallowed.
			for _, prefix := range []string{"", "/v1"} {
				for _, route := range []struct {
					method string
					path   string
					status int
					body   string
				}{
					{http.MethodPost, "/media/videos", http.StatusNotFound, `{"error":{"code":"MEDIA_TASK_DISABLED"}}`},
					{http.MethodPost, "/media/files", http.StatusNotFound, `{"error":{"code":"MEDIA_TASK_DISABLED"}}`},
					{http.MethodGet, "/media/models", http.StatusOK, `{"data":[]}`},
					{http.MethodGet, "/media/videos/task-123", http.StatusOK, `{"id":"task-123","status":"pending"}`},
					{http.MethodGet, "/media/videos/task-123/content", http.StatusNotFound, `{"error":{"code":"MEDIA_CONTENT_NOT_READY"}}`},
				} {
					path := prefix + route.path
					t.Run(route.method+path, func(t *testing.T) {
						called := false
						router.Handle(route.method, path, func(c *gin.Context) {
							called = true
							c.Data(route.status, "application/json", []byte(route.body))
						})
						response := httptest.NewRecorder()
						request := httptest.NewRequest(route.method, path, strings.NewReader(`{"prompt":"test"}`))
						request.Header.Set("Content-Type", "application/json")
						router.ServeHTTP(response, request)

						require.True(t, called, "media requests must reach their backend handler")
						assert.Equal(t, route.status, response.Code)
						assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
						assert.JSONEq(t, route.body, response.Body.String())
					})
				}
			}
			assert.Zero(t, provider.called, "API responses must not load frontend settings")

			for _, path := range []string{"/", "/dashboard", "/keys", "/usage", "/monitor", "/subscriptions", "/purchase", "/orders", "/media-library", "/mediax/videos"} {
				t.Run("spa"+path, func(t *testing.T) {
					response := httptest.NewRecorder()
					router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))

					assert.Equal(t, http.StatusOK, response.Code)
					assert.Contains(t, response.Header().Get("Content-Type"), "text/html")
					assert.Contains(t, response.Body.String(), "<!doctype html>")
				})
			}
		})
	}
}
