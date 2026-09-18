package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type mediaTaskRoutesStub struct{}

func (*mediaTaskRoutesStub) Models(c *gin.Context)          { c.Status(http.StatusNoContent) }
func (*mediaTaskRoutesStub) CreateVideo(c *gin.Context)     { c.Status(http.StatusAccepted) }
func (*mediaTaskRoutesStub) GetVideo(c *gin.Context)        { c.Status(http.StatusNoContent) }
func (*mediaTaskRoutesStub) GetVideoContent(c *gin.Context) { c.Status(http.StatusNoContent) }
func (*mediaTaskRoutesStub) UploadFile(c *gin.Context)      { c.Status(http.StatusCreated) }

func TestSeedanceRoutesAreIsolatedFromOtherPlatforms(t *testing.T) {
	for _, platform := range []string{service.PlatformSeedance, service.PlatformOpenAI, service.PlatformGrok, service.PlatformComposite} {
		router := newGatewayRoutesTestRouter(platform)
		for _, prefix := range []string{"/v1", ""} {
			for _, test := range []struct {
				method, path string
				status       int
			}{
				{http.MethodGet, "/media/models", http.StatusNoContent},
				{http.MethodPost, "/media/videos", http.StatusAccepted},
				{http.MethodPost, "/media/files", http.StatusCreated},
				{http.MethodGet, "/media/videos/abc", http.StatusNoContent},
				{http.MethodGet, "/media/videos/abc/content", http.StatusNoContent},
			} {
				w := httptest.NewRecorder()
				router.ServeHTTP(w, httptest.NewRequest(test.method, prefix+test.path, strings.NewReader(`{"model":"seedance2.0mini"}`)))
				want := test.status
				if platform != service.PlatformSeedance {
					want = http.StatusNotFound
				}
				require.Equal(t, want, w.Code, "%s %s %s", platform, test.method, prefix+test.path)
			}
		}
	}
}

func TestSeedanceKeyCannotReachTextOrGrokHandlers(t *testing.T) {
	router := newGatewayRoutesTestRouter(service.PlatformSeedance)
	for _, path := range []string{"/v1/messages", "/v1/chat/completions", "/v1/responses", "/v1/videos", "/v1/images/generations", "/chat/completions", "/responses", "/videos", "/backend-api/codex/responses"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"seedance2.0mini"}`)))
		require.Equal(t, http.StatusNotFound, w.Code, path)
	}
}

func TestSeedanceMediaCreationHonorsGroupModelAllowlist(t *testing.T) {
	router := newGatewayRoutesTestRouterWithGroup(allowlistGroup(service.PlatformSeedance, true, "seedance2.0mini"))
	for _, path := range []string{"/v1/media/videos", "/media/videos"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"seedance2.5"}`)))
		require.Equal(t, http.StatusNotFound, w.Code)
		require.Contains(t, w.Body.String(), "not available for this group")
	}
}
