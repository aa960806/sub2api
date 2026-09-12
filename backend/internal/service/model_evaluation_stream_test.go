package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type modelEvaluationTerminalThenErrorReader struct {
	data []byte
	done bool
}

func (r *modelEvaluationTerminalThenErrorReader) Read(p []byte) (int, error) {
	if !r.done {
		r.done = true
		n := copy(p, r.data)
		return n, nil
	}
	return 0, io.ErrUnexpectedEOF
}

func evaluationSSE(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return "data: " + string(raw) + "\r\n\r\n"
}

func TestModelEvaluationStreamsPreserveCompleteHTML(t *testing.T) {
	for _, format := range []string{"responses", "chat_completions", "messages"} {
		t.Run(format, func(t *testing.T) {
			var events string
			switch format {
			case "responses":
				events = evaluationSSE(t, map[string]any{"type": "response.created"})
				events += evaluationSSE(t, map[string]any{"type": "response.output_text.delta", "delta": evaluationTestHTML[:50]})
				events += evaluationSSE(t, map[string]any{"type": "response.output_text.delta", "delta": evaluationTestHTML[50:]})
				events += evaluationSSE(t, map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed", "output": []any{map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]string{"type": "output_text", "text": evaluationTestHTML}}}}}})
			case "chat_completions":
				for _, fragment := range []string{evaluationTestHTML[:50], evaluationTestHTML[50:]} {
					events += evaluationSSE(t, map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]string{"content": fragment}}}})
				}
				events += evaluationSSE(t, map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]string{}, "finish_reason": "stop"}}}) + "data: [DONE]\n\n"
			case "messages":
				events = evaluationSSE(t, map[string]any{"type": "content_block_start", "content_block": map[string]string{"type": "text", "text": evaluationTestHTML[:50]}})
				events += evaluationSSE(t, map[string]any{"type": "content_block_delta", "delta": map[string]string{"type": "text_delta", "text": evaluationTestHTML[50:]}})
				events += evaluationSSE(t, map[string]any{"type": "message_delta", "delta": map[string]string{"stop_reason": "end_turn"}})
				events += evaluationSSE(t, map[string]any{"type": "message_stop"})
			}
			svc := NewModelEvaluationService(nil, nil, nil, evaluationTestEncryptor{})
			defer svc.Stop()
			svc.client.Transport = evaluationTestTransport(func(req *http.Request) (*http.Response, error) {
				var payload map[string]any
				require.NoError(t, json.NewDecoder(req.Body).Decode(&payload))
				require.Equal(t, true, payload["stream"])
				require.Contains(t, req.Header.Get("Accept"), "text/event-stream")
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream; charset=utf-8"}}, Body: io.NopCloser(strings.NewReader(": keepalive\r\n\r\n" + events))}, nil
			})
			result := svc.execute(context.Background(), &ModelEvaluationTask{Endpoint: "https://provider.test/v1/responses", APIFormat: format, APIKeyEncrypted: "encrypted:fixture-secret", Model: "configured-model"})
			require.Equal(t, "success", result.Status, result.ErrorMessage)
			require.Equal(t, evaluationTestHTML, result.HTML)
		})
	}
}

func TestModelEvaluationStreamRejectsPartialAndProviderErrors(t *testing.T) {
	for name, tc := range map[string]struct{ format, events string }{
		"responses truncated":        {"responses", evaluationSSE(t, map[string]any{"type": "response.output_text.delta", "delta": evaluationTestHTML})},
		"responses incomplete":       {"responses", evaluationSSE(t, map[string]any{"type": "response.incomplete", "response": map[string]any{"status": "incomplete"}})},
		"responses false completion": {"responses", evaluationSSE(t, map[string]any{"type": "response.completed", "response": map[string]any{"status": "incomplete"}})},
		"responses error":            {"responses", evaluationSSE(t, map[string]any{"type": "error", "message": "provider-secret"})},
		"responses invalid delta":    {"responses", "data: {\"type\":\"response.output_text.delta\",\"delta\":{}}\n\n"},
		"chat no stop":               {"chat_completions", "data: [DONE]\n\n"},
		"chat no done":               {"chat_completions", "data: {\"choices\":[{\"finish_reason\":\"stop\",\"delta\":{}}]}\n\n"},
		"chat truncated":             {"chat_completions", "data: {\"choices\":[{\"finish_reason\":\"length\",\"delta\":{}}]}\n\n"},
		"messages no stop reason":    {"messages", "event: message_stop\ndata: {}\n\n"},
		"messages truncated":         {"messages", "data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"max_tokens\"}}\n\ndata: {\"type\":\"message_stop\"}\n\n"},
		"invalid json":               {"responses", "data: {broken provider-secret}\n\n"},
	} {
		t.Run(name, func(t *testing.T) {
			content, reason := readModelEvaluationStream(tc.format, strings.NewReader(tc.events))
			require.Empty(t, content)
			require.NotEmpty(t, reason)
			require.NotContains(t, reason, "provider-secret")
		})
	}
}

func TestModelEvaluationStreamReturnsOnCompletionBeforeEOF(t *testing.T) {
	event := evaluationSSE(t, map[string]any{
		"type": "response.completed",
		"response": map[string]any{
			"status": "completed",
			"output": []any{map[string]any{
				"type": "message", "role": "assistant",
				"content": []any{map[string]string{"type": "output_text", "text": evaluationTestHTML}},
			}},
		},
	})
	content, reason := readModelEvaluationStream("responses", &modelEvaluationTerminalThenErrorReader{data: []byte(event)})
	require.Empty(t, reason)
	require.Equal(t, evaluationTestHTML, content)
}

func TestModelEvaluationStreamAllowsWirePayloadsAboveTwoMiB(t *testing.T) {
	comments := strings.Repeat(": keepalive\n\n", (2*1024*1024)/12+512)
	event := evaluationSSE(t, map[string]any{
		"type": "response.completed",
		"response": map[string]any{
			"status": "completed",
			"output": []any{map[string]any{
				"type": "message", "role": "assistant",
				"content": []any{map[string]string{"type": "output_text", "text": evaluationTestHTML}},
			}},
		},
	})
	content, reason := readModelEvaluationStream("responses", strings.NewReader(comments+event))
	require.Empty(t, reason)
	require.Equal(t, evaluationTestHTML, content)
}

func TestModelEvaluationStreamBoundsWireAndText(t *testing.T) {
	for name, events := range map[string]string{
		"oversized delta":    evaluationSSE(t, map[string]any{"type": "response.output_text.delta", "delta": strings.Repeat("x", ModelEvaluationMaxHTMLBytes+1)}),
		"oversized comments": strings.Repeat(": heartbeat\n\n", ModelEvaluationMaxResponseBytes/13+100),
		"oversized line":     "data: " + strings.Repeat("x", ModelEvaluationMaxResponseBytes+2),
	} {
		t.Run(name, func(t *testing.T) {
			content, reason := readModelEvaluationStream("responses", strings.NewReader(events))
			require.Empty(t, content)
			require.Contains(t, reason, "限制")
		})
	}
}

func TestModelEvaluationHTTPTimeoutClassification(t *testing.T) {
	for _, status := range []int{524, 504, 408} {
		message := modelEvaluationHTTPError(status)
		require.Contains(t, message, "超时")
		require.NotContains(t, message, "额度")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.Equal(t, "请求已取消", modelEvaluationRequestError(ctx, ctx.Err()))
	require.Contains(t, modelEvaluationRequestError(context.Background(), context.DeadlineExceeded), "600 秒")
}

func TestModelEvaluationOutboundEffortMapsLegacyUltra(t *testing.T) {
	require.Equal(t, "max", normalizeModelEvaluationOutboundEffort("ultra"))
	require.Equal(t, "xhigh", normalizeModelEvaluationOutboundEffort("xhigh"))
	require.True(t, isRetryableModelEvaluationStatus(500))
	require.True(t, isRetryableModelEvaluationStatus(429))
	require.False(t, isRetryableModelEvaluationStatus(400))
}

func TestModelEvaluationProviderErrorDetailIsBoundedAndRedacted(t *testing.T) {
	raw := []byte(`{"error":{"code":"invalid_value","param":"reasoning.effort","type":"invalid_request_error","message":"Invalid value ultra for ` + ModelEvaluationPrompt + ` key-secret"}}`)
	detail := modelEvaluationProviderErrorDetail(raw, "key-secret")
	require.Contains(t, detail, "code=invalid_value")
	require.Contains(t, detail, "param=reasoning.effort")
	require.NotContains(t, detail, "key-secret")
	require.NotContains(t, detail, ModelEvaluationPrompt)
}
