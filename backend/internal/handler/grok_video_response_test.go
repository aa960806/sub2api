//go:build unit

package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGrokVideoOpenAIClientCreatePollContentLifecycle(t *testing.T) {
	for _, prefix := range []string{"", "/v1"} {
		t.Run(prefix, func(t *testing.T) {
			h, slots, bindings, upstream := newGrokMediaSlotHandler(t, false, false)
			c, created := grokMediaSlotContext(context.Background(), true)
			c.Request.URL.Path = prefix + "/videos"
			h.GrokVideoGeneration(c)
			require.Equal(t, http.StatusOK, created.Code, created.Body.String())
			id := gjson.Get(created.Body.String(), "id").String()
			require.Equal(t, "task", id)
			require.Equal(t, "queued", gjson.Get(created.Body.String(), "status").String())
			require.Empty(t, bindings.billed)
			require.Empty(t, bindings.usageLogs)
			pending, err := h.gatewayService.LoadGrokVideoPendingBilling(context.Background(), id, 10, 20)
			require.NoError(t, err)
			require.Equal(t, service.GrokVideoResponseFormatOpenAI, pending.ResponseFormat)

			upstream.call = func(req *http.Request, _ int64) (*http.Response, error) {
				require.Equal(t, http.MethodGet, req.Method)
				if strings.HasSuffix(req.URL.Path, "/content") {
					require.Equal(t, "bytes=0-3", req.Header.Get("Range"))
					return &http.Response{StatusCode: http.StatusPartialContent, Header: http.Header{"Content-Type": {"video/mp4"}, "Content-Range": {"bytes 0-3/4"}}, Body: io.NopCloser(strings.NewReader("ftyp"))}, nil
				}
				require.Equal(t, "/v1/videos/"+id, req.URL.Path)
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"status":"done","model":"grok-imagine-video","video":{"url":"https://api.x.ai/v1/videos/task/content","duration":6}}`))}, nil
			}
			for range 2 {
				c, polled := grokMediaSlotContext(context.Background(), false)
				c.Request.URL.Path = prefix + "/videos/" + id
				h.GrokVideoStatus(c)
				require.Equal(t, http.StatusOK, polled.Code, polled.Body.String())
				require.Equal(t, "completed", gjson.Get(polled.Body.String(), "status").String())
				require.Equal(t, id, gjson.Get(polled.Body.String(), "id").String())
				require.Equal(t, prefix+"/videos/"+id+"/content", gjson.Get(polled.Body.String(), "video.url").String())
			}
			c, downloaded := grokMediaSlotContext(context.Background(), false)
			c.Request.URL.Path = prefix + "/videos/" + id + "/content"
			c.Request.Header.Set("Range", "bytes=0-3")
			h.GrokVideoContent(c)
			require.Equal(t, http.StatusPartialContent, downloaded.Code, downloaded.Body.String())
			require.Equal(t, "ftyp", downloaded.Body.String())
			require.Equal(t, "video/mp4", downloaded.Header().Get("Content-Type"))
			require.Len(t, bindings.billed, 1, "all completion observations must share a billing claim")
			require.Len(t, bindings.usageLogs, 1, "polling and content download must record exactly one video usage")
			usage := <-bindings.usageLogs
			require.Equal(t, service.StableGrokVideoBillingRequestID(id), usage.RequestID)
			require.Equal(t, 1, usage.VideoCount)
			require.NotNil(t, usage.VideoDurationSeconds)
			require.Equal(t, 6, *usage.VideoDurationSeconds)
			slots.assertReleased(t)
		})
	}
}

func TestGrokVideoStatusKeepsNativeAndLegacyTaskResponses(t *testing.T) {
	for _, tt := range []struct {
		name, format, path string
	}{
		{"legacy task without pending", "", "/v1/videos/task"},
		{"native task", "native", "/v1/videos/task"},
		{"explicit native lookup", service.GrokVideoResponseFormatOpenAI, "/v1/videos/generations/task"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h, _, _, upstream := newGrokMediaSlotHandler(t, false, false)
			if tt.format != "" {
				format := tt.format
				if format == "native" {
					format = ""
				}
				require.NoError(t, h.gatewayService.StoreGrokVideoPendingBilling(context.Background(), "task", 10, 20, service.GrokVideoPendingBilling{ResponseFormat: format, VideoDurationSeconds: 6}))
			}
			upstream.call = func(*http.Request, int64) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"status":"done","video":{"url":"https://api.x.ai/v1/videos/task/content","duration":6}}`))}, nil
			}
			c, w := grokMediaSlotContext(context.Background(), false)
			c.Request.URL.Path = tt.path
			h.GrokVideoStatus(c)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			require.Equal(t, "done", gjson.Get(w.Body.String(), "status").String())
			require.False(t, gjson.Get(w.Body.String(), "id").Exists())
		})
	}
}

func TestGrokVideoStatusFormatCacheFailureIsRetryableAfterOwnerCheck(t *testing.T) {
	for _, wrongOwner := range []bool{false, true} {
		h, slots, bindings, upstream := newGrokMediaSlotHandler(t, false, false)
		bindings.pendingErr = errors.New("test Redis unavailable")
		c, w := grokMediaSlotContext(context.Background(), false)
		if wrongOwner {
			c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 11, Concurrency: 5})
		}
		h.GrokVideoStatus(c)
		if wrongOwner {
			require.Equal(t, http.StatusNotFound, w.Code)
			require.Zero(t, bindings.pendingReads, "unowned tasks must not read their format snapshot")
		} else {
			require.Equal(t, http.StatusServiceUnavailable, w.Code)
			require.Equal(t, "1", w.Header().Get("Retry-After"))
			require.NotContains(t, w.Body.String(), "Redis")
			require.Equal(t, 1, bindings.pendingReads)
		}
		require.Zero(t, upstream.calls)
		require.Zero(t, bindings.writes)
		require.Empty(t, bindings.billed)
		slots.assertReleased(t)
	}
}

func TestGrokVideoNativeCreateDoesNotEnableOpenAIStatus(t *testing.T) {
	for _, path := range []string{"/videos/generations", "/v1/videos/generations"} {
		h, _, _, _ := newGrokMediaSlotHandler(t, false, false)
		c, w := grokMediaSlotContext(context.Background(), true)
		c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"grok-imagine-video","prompt":"test","duration":6}`))
		c.Request.Header.Set("Content-Type", "application/json")
		h.GrokVideoGeneration(c)
		require.Equal(t, http.StatusOK, w.Code)
		require.JSONEq(t, `{"request_id":"task","status":"pending"}`, w.Body.String())
		pending, err := h.gatewayService.LoadGrokVideoPendingBilling(context.Background(), "task", 10, 20)
		require.NoError(t, err)
		require.Empty(t, pending.ResponseFormat)
	}
}
