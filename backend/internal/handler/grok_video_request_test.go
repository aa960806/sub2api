//go:build unit

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func grokVideoMultipartRequest(t *testing.T, fields map[string]string) ([]byte, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, value := range fields {
		require.NoError(t, writer.WriteField(name, value))
	}
	require.NoError(t, writer.Close())
	return body.Bytes(), writer.FormDataContentType()
}

func TestGrokVideoGenerationMultipartReachesUpstreamAsJSON(t *testing.T) {
	h, slots, bindings, upstream := newGrokMediaSlotHandler(t, false, false)
	body, contentType := grokVideoMultipartRequest(t, map[string]string{
		"model":   "grok-imagine-video",
		"prompt":  "A pelican riding a bicycle",
		"seconds": "6",
		"size":    "1280x720",
	})
	c, w := grokMediaSlotContext(context.Background(), true)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", contentType)
	original := upstream.call
	upstream.call = func(req *http.Request, accountID int64) (*http.Response, error) {
		require.Equal(t, http.MethodPost, req.Method)
		require.Equal(t, "/v1/videos/generations", req.URL.Path)
		require.Equal(t, "application/json", req.Header.Get("Content-Type"))
		forwarded, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.JSONEq(t, `{"model":"grok-imagine-video","prompt":"A pelican riding a bicycle","duration":6,"resolution":"720p","aspect_ratio":"16:9"}`, string(forwarded))
		return original(req, accountID)
	}

	h.GrokVideoGeneration(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.JSONEq(t, `{"request_id":"task","status":"pending"}`, w.Body.String())
	require.Equal(t, 1, upstream.calls)
	require.Positive(t, bindings.writes, "accepted tasks must remain bound to their owner")
	require.Empty(t, bindings.billed, "creating an asynchronous task must not charge for completion")
	slots.assertReleased(t)
}

func TestGrokVideoGenerationRejectsInvalidInputBeforeAdmission(t *testing.T) {
	for _, tc := range []struct {
		name    string
		seconds string
		body    string
		ctype   string
	}{
		{name: "multipart duration exceeds maximum", seconds: "20"},
		{name: "multipart fractional duration", seconds: "6.5"},
		{name: "multipart nonnumeric duration", seconds: "six"},
		{name: "multipart missing boundary", body: "invalid multipart body", ctype: "multipart/form-data"},
		{name: "multipart truncated part", body: "--video-boundary\r\nContent-Disposition: form-data; name=\"model\"\r\n\r\ngrok-imagine-video", ctype: "multipart/form-data; boundary=video-boundary"},
		{name: "native duration exceeds maximum", body: `{"model":"grok-imagine-video","prompt":"test","duration":20}`, ctype: "application/json"},
		{name: "native fractional duration", body: `{"model":"grok-imagine-video","prompt":"test","duration":6.5}`, ctype: "application/json"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, slots, bindings, upstream := newGrokMediaSlotHandler(t, false, false)
			body, contentType := []byte(tc.body), tc.ctype
			if tc.seconds != "" {
				body, contentType = grokVideoMultipartRequest(t, map[string]string{
					"model": "grok-imagine-video", "prompt": "test", "seconds": tc.seconds, "size": "1280x720",
				})
			}
			c, w := grokMediaSlotContext(context.Background(), true)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", contentType)

			h.GrokVideoGeneration(c)

			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			var response struct {
				Error struct {
					Type    string `json:"type"`
					Message string `json:"message"`
				} `json:"error"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
			require.Equal(t, "invalid_request_error", response.Error.Type)
			require.NotEmpty(t, response.Error.Message)
			require.Zero(t, upstream.calls)
			require.Zero(t, slots.userAcquired, "invalid input must not occupy a user slot")
			require.Zero(t, slots.acquired, "invalid input must not occupy an upstream account slot")
			require.Zero(t, bindings.writes, "invalid input must not create or modify task ownership")
			require.Empty(t, bindings.billed)
			slots.assertReleased(t)
		})
	}
}

func TestGrokVideoGenerationNativeJSONCompatibility(t *testing.T) {
	h, slots, bindings, upstream := newGrokMediaSlotHandler(t, false, false)
	body := `{"model":"grok-imagine-video","prompt":"A pelican riding a bicycle","duration":6,"resolution":"720p","aspect_ratio":"16:9"}`
	c, w := grokMediaSlotContext(context.Background(), true)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos/generations", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	original := upstream.call
	upstream.call = func(req *http.Request, accountID int64) (*http.Response, error) {
		require.Equal(t, "/v1/videos/generations", req.URL.Path)
		require.Equal(t, "application/json", req.Header.Get("Content-Type"))
		forwarded, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.JSONEq(t, body, string(forwarded), "existing xAI request parameters must retain their values")
		return original(req, accountID)
	}

	h.GrokVideoGeneration(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.JSONEq(t, `{"request_id":"task","status":"pending"}`, w.Body.String())
	require.Equal(t, 1, upstream.calls)
	require.Positive(t, bindings.writes)
	require.Empty(t, bindings.billed)
	slots.assertReleased(t)
}
