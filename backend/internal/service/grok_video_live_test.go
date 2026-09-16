//go:build live

package service

// Explicit opt-in integration check. This creates a billable video using the
// supplied gateway account; regular tests never run it or require credentials.
import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type grokVideoLiveTransport struct {
	HTTPUpstream
	client *http.Client
}

func (u *grokVideoLiveTransport) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return u.client.Do(req)
}

func TestGrokVideoLiveMultipartLifecycle(t *testing.T) {
	base, key := os.Getenv("SUBNEXUS_GROK_LIVE_URL"), os.Getenv("SUBNEXUS_GROK_LIVE_KEY")
	if base == "" || key == "" {
		t.Skip("set SUBNEXUS_GROK_LIVE_URL and SUBNEXUS_GROK_LIVE_KEY to authorize one paid video generation")
	}
	gin.SetMode(gin.TestMode)
	u, err := url.Parse(base)
	require.NoError(t, err)
	require.Equal(t, "https", u.Scheme)
	require.Empty(t, u.User)
	outDir := os.Getenv("SUBNEXUS_GROK_LIVE_OUTPUT")
	if outDir == "" {
		outDir = t.TempDir()
	}
	require.NoError(t, os.MkdirAll(outDir, 0700))
	client := &http.Client{Timeout: 90 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &grokVideoLiveTransport{client: client}}
	account := &Account{ID: 1, Platform: PlatformGrok, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"base_url": strings.TrimRight(base, "/"), "api_key": key}}
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for k, v := range map[string]string{"model": "grok-imagine-video", "prompt": "A blue ball rolls slowly on a white tabletop. Static camera, no text.", "seconds": "6", "size": "1280x720"} {
		require.NoError(t, w.WriteField(k, v))
	}
	if imagePath := os.Getenv("SUBNEXUS_GROK_LIVE_REFERENCE_IMAGE"); imagePath != "" {
		imageBytes, err := os.ReadFile(imagePath)
		require.NoError(t, err)
		part, err := w.CreateFormFile("input_reference", filepath.Base(imagePath))
		require.NoError(t, err)
		_, err = part.Write(imageBytes)
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	call := func(endpoint GrokMediaEndpoint, id string, payload []byte, contentType string, partial bool) (*OpenAIForwardResult, *httptest.ResponseRecorder) {
		method, path := http.MethodGet, "/v1/videos/"+id
		if endpoint == GrokMediaEndpointVideosGenerations {
			method, path = http.MethodPost, "/v1/videos"
		} else if endpoint == GrokMediaEndpointVideoContent {
			path += "/content"
		}
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(method, "https://local-gateway.example"+path, bytes.NewReader(payload)).WithContext(ctx)
		c.Request.Header.Set("Content-Type", contentType)
		if partial {
			c.Request.Header.Set("Range", "bytes=0-1023")
		}
		result, err := svc.ForwardGrokMedia(ctx, c, account, endpoint, id, payload, contentType)
		require.NoError(t, err, "endpoint=%s HTTP=%d body=%s", endpoint, recorder.Code, recorder.Body.String())
		return result, recorder
	}
	created, response := call(GrokMediaEndpointVideosGenerations, "", body.Bytes(), w.FormDataContentType(), false)
	require.Equal(t, http.StatusOK, response.Code)
	require.NotEmpty(t, created.ResponseID)
	require.Equal(t, 6, created.VideoDurationSeconds)
	require.Equal(t, "720p", created.VideoResolution)
	taskID := created.ResponseID
	require.NoError(t, os.WriteFile(filepath.Join(outDir, "task-id.txt"), []byte(taskID), 0600))
	t.Logf("created task=%s duration=%d resolution=%s", taskID, created.VideoDurationSeconds, created.VideoResolution)
	var completed *httptest.ResponseRecorder
	for {
		_, status := call(GrokMediaEndpointVideoStatus, taskID, nil, "", false)
		state := gjson.GetBytes(status.Body.Bytes(), "status").String()
		t.Logf("poll task=%s http=%d state=%s", taskID, status.Code, state)
		if state == "done" {
			completed = status
			break
		}
		require.NotContains(t, []string{"failed", "expired"}, state, "body=%s", status.Body.String())
		select {
		case <-ctx.Done():
			t.Fatalf("task did not finish before test deadline: %s", taskID)
		case <-time.After(5 * time.Second):
		}
	}
	require.Equal(t, "/v1/videos/"+taskID+"/content", gjson.GetBytes(completed.Body.Bytes(), "video.url").String())
	require.Equal(t, int64(6), gjson.GetBytes(completed.Body.Bytes(), "video.duration").Int())
	_, video := call(GrokMediaEndpointVideoContent, taskID, nil, "", false)
	require.Equal(t, http.StatusOK, video.Code)
	require.Contains(t, video.Header().Get("Content-Type"), "video/mp4")
	require.Greater(t, video.Body.Len(), 1024)
	require.Contains(t, string(video.Body.Bytes()[:64]), "ftyp")
	require.NoError(t, os.WriteFile(filepath.Join(outDir, "fixed-video.mp4"), video.Body.Bytes(), 0600))
	_, part := call(GrokMediaEndpointVideoContent, taskID, nil, "", true)
	require.Contains(t, []int{http.StatusOK, http.StatusPartialContent}, part.Code)
	if part.Code == http.StatusPartialContent {
		require.Equal(t, 1024, part.Body.Len())
		require.True(t, strings.HasPrefix(part.Header().Get("Content-Range"), "bytes 0-1023/"))
	}
	_, repeated := call(GrokMediaEndpointVideoStatus, taskID, nil, "", false)
	require.Equal(t, "done", gjson.GetBytes(repeated.Body.Bytes(), "status").String())
	digest := sha256.Sum256(video.Body.Bytes())
	summary, err := json.MarshalIndent(map[string]any{"task_id": taskID, "result": "passed", "duration": 6,
		"resolution": "720p", "content_bytes": video.Body.Len(), "content_sha256": hex.EncodeToString(digest[:]),
		"range_status": part.Code, "repeat_status": "done", "upstream": u.Host,
		"reference_image": os.Getenv("SUBNEXUS_GROK_LIVE_REFERENCE_IMAGE") != ""}, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(outDir, "live-result.json"), summary, 0600))
	fmt.Println(string(summary))
}
