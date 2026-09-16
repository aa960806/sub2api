package service

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func grokVideoMultipartTestBody(t *testing.T, fields [][2]string, uploadField string, upload []byte) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for _, field := range fields {
		require.NoError(t, w.WriteField(field[0], field[1]))
	}
	if uploadField != "" {
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", `form-data; name="`+uploadField+`"; filename="reference.png"`)
		h.Set("Content-Type", "image/png")
		part, err := w.CreatePart(h)
		require.NoError(t, err)
		_, err = part.Write(upload)
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	return buf.Bytes(), w.FormDataContentType()
}

func TestPrepareGrokVideoMultipartMatchesRequestedGeometryAndBilling(t *testing.T) {
	input, contentType := grokVideoMultipartTestBody(t, [][2]string{
		{"model", "grok-imagine-video"}, {"prompt", "  waves at sunset  "},
		{"seconds", "6"}, {"size", "1280x720"},
	}, "", nil)
	out, normalizedType, err := PrepareGrokVideoGenerationRequest(input, contentType)
	require.NoError(t, err)
	require.Equal(t, "application/json", normalizedType)
	require.JSONEq(t, `{"model":"grok-imagine-video","prompt":"  waves at sunset  ","duration":6,"resolution":"720p","aspect_ratio":"16:9"}`, string(out))
	info := ParseGrokMediaRequest(normalizedType, out)
	require.Equal(t, 6, info.DurationSeconds)
	require.Equal(t, "720p", info.Resolution)
	meta := grokMediaUsageFromResponse(GrokMediaEndpointVideosGenerations, info, []byte(`{"request_id":"task"}`))
	require.Equal(t, 0, meta.VideoCount, "creating a task must not bill prematurely")
	require.Equal(t, 6, meta.VideoDurationSeconds)
	require.Equal(t, "720p", meta.VideoResolution)
}

func TestPrepareGrokVideoKeepsNativeJSONAndOfficialFileID(t *testing.T) {
	input := []byte(`{ "model":"grok-imagine-video-1.5", "prompt":"animate", "duration":6, "resolution":"1080p", "image":{"file_id":"file_abc"}, "extension":{"keep":true} }`)
	out, contentType, err := PrepareGrokVideoGenerationRequest(input, "application/json; charset=utf-8")
	require.NoError(t, err)
	require.Equal(t, input, out)
	require.Equal(t, "application/json", contentType)
	defaults := []byte(`{"model":"grok-imagine-video","prompt":"waves"}`)
	out, _, err = PrepareGrokVideoGenerationRequest(defaults, "application/json")
	require.NoError(t, err)
	require.Equal(t, defaults, out, "absent official defaults must remain absent")
}

func TestPrepareGrokVideoJSONAliasesAndExplicitGeometry(t *testing.T) {
	for _, seconds := range []string{`6`, `"6"`} {
		input := []byte(`{"model":"grok-imagine-video","prompt":"waves","seconds":` + seconds + `,"size":"720x1280","resolution":"480p","aspect_ratio":"1:1","image_url":"https://example.com/ref.png","extension":{"keep":true}}`)
		out, _, err := PrepareGrokVideoGenerationRequest(input, "application/json")
		require.NoError(t, err)
		require.Equal(t, int64(6), gjson.GetBytes(out, "duration").Int())
		require.False(t, gjson.GetBytes(out, "seconds").Exists())
		require.False(t, gjson.GetBytes(out, "size").Exists())
		require.False(t, gjson.GetBytes(out, "image_url").Exists())
		require.Equal(t, "480p", gjson.GetBytes(out, "resolution").String())
		require.Equal(t, "1:1", gjson.GetBytes(out, "aspect_ratio").String())
		require.Equal(t, "https://example.com/ref.png", gjson.GetBytes(out, "image.url").String())
		require.True(t, gjson.GetBytes(out, "extension.keep").Bool())
	}
	for _, size := range []string{"720x1280", "1080x1920", "auto"} {
		out, _, err := PrepareGrokVideoGenerationRequest([]byte(`{"model":"grok-imagine-video-1.5","size":"`+size+`"}`), "application/json")
		require.NoError(t, err)
		switch size {
		case "720x1280":
			require.Equal(t, "9:16", gjson.GetBytes(out, "aspect_ratio").String())
			require.Equal(t, "720p", gjson.GetBytes(out, "resolution").String())
		case "1080x1920":
			require.Equal(t, "1080p", gjson.GetBytes(out, "resolution").String())
		case "auto":
			require.False(t, gjson.GetBytes(out, "aspect_ratio").Exists())
			require.False(t, gjson.GetBytes(out, "resolution").Exists())
		}
	}
}

func TestPrepareGrokVideoInputReferenceIncludedInModeration(t *testing.T) {
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	input, contentType := grokVideoMultipartTestBody(t, [][2]string{{"model", "grok-imagine-video-1.5"}, {"duration", "6"}}, "input_reference", png)
	out, normalizedType, err := PrepareGrokVideoGenerationRequest(input, contentType)
	require.NoError(t, err)
	require.Equal(t, "data:image/png;base64,iVBORw0KGgo=", gjson.GetBytes(out, "image.url").String())
	info := ParseGrokMediaRequest(normalizedType, out)
	require.True(t, info.HasInputImage())
	require.Equal(t, gjson.GetBytes(out, "image.url").String(), gjson.GetBytes(info.ModerationBody(), "images.0.image_url").String())
	for _, reference := range []string{`"https://example.com/a.png"`, `{"url":"https://example.com/a.png"}`} {
		out, _, err = PrepareGrokVideoGenerationRequest([]byte(`{"model":"grok-imagine-video","input_reference":`+reference+`}`), "application/json")
		require.NoError(t, err)
		require.Equal(t, "https://example.com/a.png", gjson.GetBytes(out, "image.url").String())
	}
}

func TestPrepareGrokVideoRejectsInvalidRequestsWithoutTruncating(t *testing.T) {
	for _, body := range []string{
		`[]`, `null`, `{`,
		`{"duration":20}`, `{"seconds":"20"}`, `{"duration":0}`, `{"duration":-1}`,
		`{"duration":6.5}`, `{"seconds":"6.5"}`, `{"duration":"6"}`, `{"seconds":true}`, `{"duration":null}`,
		`{"seconds":6,"duration":8}`, `{"size":false}`, `{"size":"wide"}`, `{"resolution":"4k"}`,
		`{"duration":6,"duration":15}`, `{"duration":6} {"duration":15}`,
		`{"aspect_ratio":4}`, `{"aspect_ratio":"wide"}`, `{"prompt":{}}`,
		`{"input_reference":[]}`, `{"input_reference":""}`,
		`{"image":{"url":"https://example.com/a.png"},"input_reference":"https://example.com/b.png"}`,
	} {
		t.Run(body, func(t *testing.T) {
			out, _, err := PrepareGrokVideoGenerationRequest([]byte(body), "application/json")
			var requestErr *GrokVideoRequestError
			require.ErrorAs(t, err, &requestErr)
			require.Nil(t, out)
		})
	}
}

func TestPrepareGrokVideoRejectsMalformedMultipartAndUnsafeUploads(t *testing.T) {
	valid, validType := grokVideoMultipartTestBody(t, [][2]string{{"model", "grok-imagine-video"}}, "", nil)
	duplicate, duplicateType := grokVideoMultipartTestBody(t, [][2]string{{"model", "grok-imagine-video"}, {"model", "other"}}, "", nil)
	badImage, imageType := grokVideoMultipartTestBody(t, nil, "input_reference", []byte("this is not an image"))
	unknownFile, unknownType := grokVideoMultipartTestBody(t, nil, "audio", []byte("audio"))
	oversized, oversizedType := grokVideoMultipartTestBody(t, nil, "input_reference", bytes.Repeat([]byte("a"), openAIImageMaxUploadPartSize+1))
	for _, tt := range []struct {
		body        []byte
		contentType string
	}{
		{valid, "multipart/form-data"}, {valid[:len(valid)-10], validType},
		{duplicate, duplicateType}, {badImage, imageType}, {unknownFile, unknownType}, {oversized, oversizedType},
		{[]byte(`{"model":"grok-imagine-video"}`), "text/plain"},
	} {
		_, _, err := PrepareGrokVideoGenerationRequest(tt.body, tt.contentType)
		var requestErr *GrokVideoRequestError
		require.ErrorAs(t, err, &requestErr)
	}
}

func TestForwardGrokVideoMultipartUsesJSONAndModelMapping(t *testing.T) {
	t.Setenv(xai.EnvAllowUnsafeURLOverrides, "true")
	gin.SetMode(gin.TestMode)
	body, contentType := grokVideoMultipartTestBody(t, [][2]string{{"model", "grok-imagine-video"}, {"prompt", "waves"}, {"seconds", "6"}, {"size", "1280x720"}}, "", nil)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", contentType)
	account := &Account{ID: 1, Platform: PlatformGrok, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{"api_key": "test", "base_url": "https://xai.test/v1", "model_mapping": map[string]any{"grok-imagine-video": "vendor-video"}}}
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"request_id":"task"}`))}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	result, err := svc.ForwardGrokMedia(context.Background(), c, account, GrokMediaEndpointVideosGenerations, "", body, contentType)
	require.NoError(t, err)
	require.Equal(t, "application/json", upstream.lastReq.Header.Get("Content-Type"))
	require.Equal(t, "https://xai.test/v1/videos/generations", upstream.lastReq.URL.String())
	require.Equal(t, "vendor-video", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, "grok-imagine-video", result.BillingModel)
	require.Equal(t, 6, result.VideoDurationSeconds)
	require.Equal(t, "720p", result.VideoResolution)
	require.Equal(t, 0, result.VideoCount)
	require.Equal(t, "task", result.ResponseID)
}

func TestForwardGrokVideoOAuthHeavyMultipartInputReferenceUsesOfficialJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	body, contentType := grokVideoMultipartTestBody(t, [][2]string{
		{"model", "grok-imagine-video-1.5"}, {"prompt", "animate"},
		{"seconds", "6"}, {"size", "1280x720"},
	}, "input_reference", png)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", contentType)
	account := &Account{
		ID: 66, Name: "grok-oauth-heavy", Platform: PlatformGrok, Type: AccountTypeOAuth,
		Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{
			"access_token":      "oauth-access-token",
			"refresh_token":     "oauth-refresh-token",
			"expires_at":        time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
			"base_url":          xai.DefaultCLIBaseURL,
			"subscription_tier": "supergrok_heavy",
		},
	}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"request_id":"video-heavy-multipart"}`)),
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream, grokTokenProvider: NewGrokTokenProvider(nil, nil)}
	result, err := svc.ForwardGrokMedia(context.Background(), c, account, GrokMediaEndpointVideosGenerations, "", body, contentType)
	require.NoError(t, err)
	require.Equal(t, xai.DefaultBaseURL+"/videos/generations", upstream.lastReq.URL.String())
	require.Equal(t, http.MethodPost, upstream.lastReq.Method)
	require.Equal(t, "application/json", upstream.lastReq.Header.Get("Content-Type"))
	require.Equal(t, "Bearer oauth-access-token", upstream.lastReq.Header.Get("Authorization"))
	require.Empty(t, upstream.lastReq.Header.Get("X-XAI-Token-Auth"))
	require.Empty(t, upstream.lastReq.Header.Get("x-grok-client-version"))
	require.JSONEq(t, `{"model":"grok-imagine-video-1.5","prompt":"animate","duration":6,"resolution":"720p","aspect_ratio":"16:9","image":{"url":"data:image/png;base64,iVBORw0KGgo="}}`, string(upstream.lastBody))
	require.Equal(t, "grok-imagine-video-1.5", result.BillingModel)
	require.Equal(t, 6, result.VideoDurationSeconds)
	require.Equal(t, "720p", result.VideoResolution)
	require.Equal(t, 0, result.VideoCount, "OAuth video task creation must not be billed before completion")
	require.Equal(t, "video-heavy-multipart", result.ResponseID)
}
