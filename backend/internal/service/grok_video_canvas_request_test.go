package service

import (
	"bytes"
	"mime/multipart"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestPrepareGrokVideoCanvasUploadAndResolution(t *testing.T) {
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	body, ctype := grokVideoMultipartTestBody(t, [][2]string{
		{"model", "grok-imagine-video"}, {"prompt", "animate"}, {"seconds", "6"},
		{"size", "960x960"}, {"resolution_name", "720p"}, {"preset", "normal"},
	}, "input_reference[]", png)
	out, _, err := PrepareGrokVideoGenerationRequest(body, ctype)
	require.NoError(t, err)
	require.Equal(t, "data:image/png;base64,iVBORw0KGgo=", gjson.GetBytes(out, "image.url").String())
	require.Equal(t, "1:1", gjson.GetBytes(out, "aspect_ratio").String())
	require.Equal(t, "720p", gjson.GetBytes(out, "resolution").String())
	require.Equal(t, int64(6), gjson.GetBytes(out, "duration").Int())
	require.False(t, gjson.GetBytes(out, "resolution_name").Exists())
	require.False(t, gjson.GetBytes(out, "input_reference[]").Exists())
	info := ParseGrokMediaRequest("application/json", out)
	require.True(t, info.HasInputImage())
	require.Equal(t, gjson.GetBytes(out, "image.url").String(), gjson.GetBytes(info.ModerationBody(), "images.0.image_url").String())
	// Explicit selected resolution wins even when canvas geometry is larger.
	out, _, err = PrepareGrokVideoGenerationRequest([]byte(`{"size":"1920x1080","resolution_name":"480p"}`), "application/json")
	require.NoError(t, err)
	require.Equal(t, "480p", gjson.GetBytes(out, "resolution").String())
	out, _, err = PrepareGrokVideoGenerationRequest([]byte(`{"resolution":"720p","resolution_name":"480p"}`), "application/json")
	require.NoError(t, err)
	require.Equal(t, "720p", gjson.GetBytes(out, "resolution").String())
}

func TestPrepareGrokVideoCanvasReferencesKeepEveryImage(t *testing.T) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	require.NoError(t, w.WriteField("model", "grok-imagine-video"))
	require.NoError(t, w.WriteField("input_reference[]", "https://example.com/one.png"))
	require.NoError(t, w.WriteField("input_reference[]", `{"url":"https://example.com/two.png"}`))
	require.NoError(t, w.Close())
	out, _, err := PrepareGrokVideoGenerationRequest(body.Bytes(), w.FormDataContentType())
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(out, "image").Exists())
	require.Equal(t, "https://example.com/one.png", gjson.GetBytes(out, "reference_images.0.url").String())
	require.Equal(t, "https://example.com/two.png", gjson.GetBytes(out, "reference_images.1.url").String())
	info := ParseGrokMediaRequest("application/json", out)
	require.Len(t, info.InputImageURLs, 2)
	require.Len(t, gjson.GetBytes(info.ModerationBody(), "images").Array(), 2)
}

func TestPrepareGrokVideoCanvasRejectsAmbiguousAndInvalidReferences(t *testing.T) {
	for _, body := range []string{
		`{"input_reference[]":[]}`, `{"input_reference[]":"https://example.com/a.png"}`,
		`{"input_reference[]":[""]}`, `{"input_reference[]":[true]}`,
		`{"input_reference[]":["https://example.com/a.png"],"image":{"url":"https://example.com/b.png"}}`,
		`{"input_reference[]":["https://example.com/a.png"],"reference_images":[{"url":"https://example.com/b.png"}]}`,
		`{"input_reference[]":["https://example.com/a.png"],"input_reference":"https://example.com/b.png"}`,
		`{"resolution_name":"4k"}`, `{"resolution_name":720}`,
	} {
		t.Run(body, func(t *testing.T) {
			_, _, err := PrepareGrokVideoGenerationRequest([]byte(body), "application/json")
			var requestErr *GrokVideoRequestError
			require.ErrorAs(t, err, &requestErr)
		})
	}
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for range 8 {
		require.NoError(t, w.WriteField("input_reference[]", "https://example.com/reference.png"))
	}
	require.NoError(t, w.Close())
	_, _, err := PrepareGrokVideoGenerationRequest(body.Bytes(), w.FormDataContentType())
	require.ErrorContains(t, err, "at most 7")
}
