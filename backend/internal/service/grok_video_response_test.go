package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func grokVideoResponseContext(method, path string) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, nil)
	return c, w
}

func TestGrokVideoCreateResponseProtocolByRoute(t *testing.T) {
	for _, path := range []string{"/videos", "/v1/videos", "/videos/generations", "/v1/videos/generations"} {
		t.Run(path, func(t *testing.T) {
			c, _ := grokVideoResponseContext(http.MethodPost, path)
			body := []byte(`{"request_id":"task","counter":9007199254740993}`)
			out, err := grokVideoClientResponse(c, GrokMediaEndpointVideosGenerations, "", body)
			require.NoError(t, err)
			if strings.HasSuffix(path, "/generations") {
				require.Equal(t, body, out)
				return
			}
			require.Equal(t, "task", gjson.GetBytes(out, "id").String())
			require.Equal(t, "task", gjson.GetBytes(out, "request_id").String())
			require.Equal(t, "queued", gjson.GetBytes(out, "status").String())
			require.Equal(t, "video", gjson.GetBytes(out, "object").String())
			require.Equal(t, "9007199254740993", gjson.GetBytes(out, "counter").String())
		})
	}
	for _, body := range []string{`{}`, `{"status":"pending"}`, `null`, `[]`, `not-json`} {
		c, _ := grokVideoResponseContext(http.MethodPost, "/v1/videos")
		_, err := grokVideoClientResponse(c, GrokMediaEndpointVideosGenerations, "", []byte(body))
		require.Error(t, err, "must not invent a task ID: %s", body)
	}
}

func TestGrokVideoStatusResponseUsesPersistedProtocol(t *testing.T) {
	for _, tt := range []struct {
		body, status string
		billable     bool
	}{
		{`{"status":"pending"}`, "in_progress", false},
		{`{"status":"done","video":{"url":"/v1/videos/task/content","duration":6}}`, "completed", true},
		{`{"status":"failed"}`, "failed", false},
		{`{"status":"expired"}`, "failed", false},
	} {
		for _, prefix := range []string{"", "/v1"} {
			c, _ := grokVideoResponseContext(http.MethodGet, prefix+"/videos/task")
			native, err := grokVideoClientResponse(c, GrokMediaEndpointVideoStatus, "task", []byte(tt.body))
			require.NoError(t, err)
			require.Equal(t, tt.body, string(native), "old tasks and native creates retain xAI status")
			BindGrokVideoResponseFormat(c, GrokVideoResponseFormatOpenAI)
			out, err := grokVideoClientResponse(c, GrokMediaEndpointVideoStatus, "task", []byte(tt.body))
			require.NoError(t, err)
			require.Equal(t, "task", gjson.GetBytes(out, "id").String(), "lookup path supplies the owner-checked ID when upstream omits it")
			require.Equal(t, tt.status, gjson.GetBytes(out, "status").String())
			require.Equal(t, tt.billable, IsGrokVideoStatusBillable([]byte(tt.body)))
			if tt.status == "failed" {
				require.NotEmpty(t, gjson.GetBytes(out, "error.message").String())
			}
		}
	}
	c, _ := grokVideoResponseContext(http.MethodGet, "/v1/videos/task")
	BindGrokVideoResponseFormat(c, GrokVideoResponseFormatOpenAI)
	_, err := grokVideoClientResponse(c, GrokMediaEndpointVideoStatus, "task", []byte(`{"status":"done"}`))
	require.Error(t, err, "a missing video URL must not be reported as completed")
	c.Request.URL.Path = "/v1/videos/generations/task"
	out, err := grokVideoClientResponse(c, GrokMediaEndpointVideoStatus, "task", []byte(`{"status":"done"}`))
	require.NoError(t, err)
	require.JSONEq(t, `{"status":"done"}`, string(out), "explicit native path is unchanged")
}

func TestForwardGrokVideoCompletedResponsePreservesNativeBilling(t *testing.T) {
	t.Setenv(xai.EnvAllowUnsafeURLOverrides, "true")
	c, w := grokVideoResponseContext(http.MethodGet, "/v1/videos/task")
	BindGrokVideoResponseFormat(c, GrokVideoResponseFormatOpenAI)
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK,
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   io.NopCloser(strings.NewReader(`{"status":"done","model":"grok-imagine-video","video":{"url":"https://vidgen.x.ai/private-signed/video.mp4","duration":6}}`))}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	account := &Account{ID: 1, Platform: PlatformGrok, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "test", "base_url": "https://xai.test/v1"}}
	result, err := svc.ForwardGrokMedia(context.Background(), c, account, GrokMediaEndpointVideoStatus, "task", nil, "")
	require.NoError(t, err)
	require.Equal(t, "completed", gjson.Get(w.Body.String(), "status").String())
	require.Equal(t, "task", gjson.Get(w.Body.String(), "id").String())
	require.Equal(t, "/v1/videos/task/content", gjson.Get(w.Body.String(), "video.url").String())
	require.NotContains(t, w.Body.String(), "private-signed")
	require.Equal(t, 1, result.VideoCount, "format adaptation must not erase the upstream done billing observation")
	require.Equal(t, 6, result.VideoDurationSeconds)
	require.Equal(t, "grok-imagine-video", result.BillingModel)
	require.Equal(t, "/v1/videos/task", upstream.lastReq.URL.Path)
}

func TestGrokVideoResponseDoesNotChangeImagesOrContent(t *testing.T) {
	c, _ := grokVideoResponseContext(http.MethodPost, "/v1/videos")
	BindGrokVideoResponseFormat(c, GrokVideoResponseFormatOpenAI)
	body := []byte(`{"data":[{"url":"https://example.com/image.png"}]}`)
	for _, endpoint := range []GrokMediaEndpoint{GrokMediaEndpointImagesGenerations, GrokMediaEndpointImagesEdits, GrokMediaEndpointVideosEdits, GrokMediaEndpointVideosExtensions, GrokMediaEndpointVideoContent} {
		out, err := grokVideoClientResponse(c, endpoint, "task", body)
		require.NoError(t, err)
		require.Equal(t, body, out)
	}
}
