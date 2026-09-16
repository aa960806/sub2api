//go:build unit && live

package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// This opt-in test creates one paid upstream video. Task ownership and response
// format use the actual handler and an isolated in-memory cache. The upstream
// gateway performs real billing; local simple mode writes only to an isolated
// usage-log fixture and verifies that repeated polling/download records once.
func TestGrokVideoCanvasLiveLifecycle(t *testing.T) {
	base, key := os.Getenv("SUBNEXUS_GROK_LIVE_URL"), os.Getenv("SUBNEXUS_GROK_LIVE_KEY")
	if base == "" || key == "" {
		t.Skip("explicit gateway URL/key required; this creates one paid video")
	}
	logger.InitBootstrap()
	u, err := url.Parse(base)
	require.NoError(t, err)
	require.Equal(t, "https", u.Scheme)
	require.Empty(t, u.User)
	require.Contains(t, []string{"", "/", "/v1", "/v1/"}, u.Path)
	outDir := os.Getenv("SUBNEXUS_GROK_LIVE_OUTPUT")
	if outDir == "" {
		outDir = t.TempDir()
	}
	require.NoError(t, os.MkdirAll(outDir, 0700))
	h, slots, bindings, upstream := newGrokMediaSlotHandler(t, false, false)
	client := &http.Client{Timeout: 90 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	upstream.call = func(req *http.Request, _ int64) (*http.Response, error) {
		if id := os.Getenv("SUBNEXUS_GROK_LIVE_TASK_ID"); id != "" && req.Method == http.MethodPost {
			data, err := json.Marshal(map[string]string{"request_id": id})
			require.NoError(t, err)
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewReader(data))}, nil
		}
		copy := req.Clone(req.Context())
		copy.URL.Scheme, copy.URL.Host, copy.Host = u.Scheme, u.Host, u.Host
		copy.Header.Set("Authorization", "Bearer "+key)
		resp, err := client.Do(copy)
		if err != nil {
			t.Logf("upstream transport path=%s error=%v", copy.URL.Path, err)
			return nil, err
		}
		if strings.Contains(resp.Header.Get("Content-Type"), "json") {
			data, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if readErr != nil {
				return nil, readErr
			}
			resp.Body = io.NopCloser(bytes.NewReader(data))
			t.Logf("upstream path=%s status=%d body=%s", copy.URL.Path, resp.StatusCode, data)
			if copy.Method == http.MethodPost {
				require.NoError(t, os.WriteFile(filepath.Join(outDir, "upstream-create.json"), data, 0600))
			}
		}
		return resp, nil
	}
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for k, v := range map[string]string{"model": "grok-imagine-video", "prompt": "A blue ball rolls slowly on a white tabletop. Static camera, no text.", "seconds": "6", "size": "960x960", "resolution_name": "720p", "preset": "normal"} {
		require.NoError(t, w.WriteField(k, v))
	}
	if reference := os.Getenv("SUBNEXUS_GROK_LIVE_REFERENCE_IMAGE"); reference != "" {
		data, err := os.ReadFile(reference)
		require.NoError(t, err)
		part, err := w.CreateFormFile("input_reference[]", filepath.Base(reference))
		require.NoError(t, err)
		_, err = part.Write(data)
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	call := func(id, suffix string, partial bool) *httptest.ResponseRecorder {
		create := id == ""
		c, result := grokMediaSlotContext(ctx, create)
		if create {
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(body.Bytes())).WithContext(ctx)
			c.Request.Header.Set("Content-Type", w.FormDataContentType())
			h.GrokVideoGeneration(c)
		} else {
			c.Params = gin.Params{{Key: "request_id", Value: id}}
			c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/"+id+suffix, nil).WithContext(ctx)
			if partial {
				c.Request.Header.Set("Range", "bytes=0-1023")
			}
			if suffix == "/content" {
				h.GrokVideoContent(c)
			} else {
				h.GrokVideoStatus(c)
			}
		}
		return result
	}
	created := call("", "", false)
	require.Equal(t, http.StatusOK, created.Code, created.Body.String())
	id := gjson.GetBytes(created.Body.Bytes(), "id").String()
	require.NotEmpty(t, id, "the actual canvas parser reads only id")
	require.Equal(t, id, gjson.GetBytes(created.Body.Bytes(), "request_id").String())
	require.Empty(t, bindings.billed, "async create must not bill")
	require.NoError(t, os.WriteFile(filepath.Join(outDir, "task-id.txt"), []byte(id), 0600))
	t.Logf("created task=%s", id)
	var completed *httptest.ResponseRecorder
	for {
		status := call(id, "", false)
		require.Contains(t, []int{http.StatusOK, http.StatusAccepted}, status.Code, status.Body.String())
		state := gjson.GetBytes(status.Body.Bytes(), "status").String()
		t.Logf("poll task=%s status=%s", id, state)
		if state == "completed" {
			completed = status
			break
		}
		require.Contains(t, []string{"queued", "in_progress"}, state, status.Body.String())
		select {
		case <-ctx.Done():
			t.Fatal("video did not complete within canvas polling window; saved task ID permits follow-up")
		case <-time.After(5 * time.Second):
		}
	}
	require.Equal(t, "/v1/videos/"+id+"/content", gjson.GetBytes(completed.Body.Bytes(), "video.url").String())
	video := call(id, "/content", false)
	require.Equal(t, http.StatusOK, video.Code)
	require.Contains(t, video.Header().Get("Content-Type"), "video/mp4")
	require.Greater(t, video.Body.Len(), 1024)
	require.Contains(t, string(video.Body.Bytes()[:64]), "ftyp")
	require.NoError(t, os.WriteFile(filepath.Join(outDir, "canvas-video.mp4"), video.Body.Bytes(), 0600))
	partial := call(id, "/content", true)
	require.Equal(t, http.StatusPartialContent, partial.Code)
	require.Equal(t, 1024, partial.Body.Len())
	require.True(t, strings.HasPrefix(partial.Header().Get("Content-Range"), "bytes 0-1023/"))
	repeated := call(id, "", false)
	require.Equal(t, "completed", gjson.GetBytes(repeated.Body.Bytes(), "status").String())
	require.Len(t, bindings.billed, 1)
	require.Len(t, bindings.usageLogs, 1, "completion, download, range and repeated status must record once")
	usage := <-bindings.usageLogs
	require.Equal(t, "grok-video:"+id, usage.RequestID)
	fixtures, err := json.MarshalIndent(map[string]json.RawMessage{"create": created.Body.Bytes(), "status": completed.Body.Bytes()}, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(outDir, "client-fixtures.json"), fixtures, 0600))
	digest := sha256.Sum256(video.Body.Bytes())
	summary, err := json.MarshalIndent(map[string]any{"task_id": id, "result": "passed", "create_id": true, "status": "completed", "content_bytes": video.Body.Len(), "content_sha256": hex.EncodeToString(digest[:]), "range_status": partial.Code, "reference_image": os.Getenv("SUBNEXUS_GROK_LIVE_REFERENCE_IMAGE") != ""}, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(outDir, "result.json"), summary, 0600))
	slots.assertReleased(t)
}
