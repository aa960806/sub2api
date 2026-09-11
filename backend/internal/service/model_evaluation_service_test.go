package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type evaluationTestSettings struct {
	SettingRepository
	value string
	err   error
}

func (s evaluationTestSettings) GetValue(context.Context, string) (string, error) {
	return s.value, s.err
}

type evaluationTestGroups struct{ GroupRepository }

func (evaluationTestGroups) GetByID(context.Context, int64) (*Group, error) {
	return &Group{ID: 1, Name: "Test group", Status: StatusActive}, nil
}

type evaluationTestEncryptor struct{}

func (evaluationTestEncryptor) Encrypt(v string) (string, error) { return "encrypted:" + v, nil }
func (evaluationTestEncryptor) Decrypt(v string) (string, error) {
	if !strings.HasPrefix(v, "encrypted:") {
		return "", errors.New("invalid")
	}
	return strings.TrimPrefix(v, "encrypted:"), nil
}

type evaluationTestTransport func(*http.Request) (*http.Response, error)

func (f evaluationTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestModelEvaluationConfigFailsClosedAndNoRequest(t *testing.T) {
	for _, s := range []evaluationTestSettings{{}, {value: "TRUE"}, {value: "1"}, {value: "true", err: errors.New("db unavailable")}} {
		svc := NewModelEvaluationService(nil, s, nil, nil)
		cfg, err := svc.GetConfig(context.Background())
		require.NoError(t, err)
		require.False(t, cfg.Enabled)
		require.ErrorIs(t, svc.RunNow(context.Background(), 1), ErrModelEvaluationDisabled)
		items, total, err := svc.ListResults(context.Background(), ModelEvaluationListParams{AllowedGroupIDs: []int64{1}})
		require.NoError(t, err)
		require.Empty(t, items)
		require.Zero(t, total)
		svc.Stop()
	}
}

func TestModelEvaluationSchedulerDisabledDoesNotClaimOrCleanup(t *testing.T) {
	// A nil repository panics if the loop touches any task/cleanup operation.
	svc := NewModelEvaluationService(nil, evaluationTestSettings{value: "false"}, nil, nil)
	svc.tickInterval = time.Millisecond
	svc.Start()
	time.Sleep(20 * time.Millisecond)
	svc.Stop()
}

func TestModelEvaluationTaskValidationEncryptionAndMasking(t *testing.T) {
	svc := NewModelEvaluationService(nil, nil, evaluationTestGroups{}, evaluationTestEncryptor{})
	defer svc.Stop()
	in := ModelEvaluationTaskInput{Name: "test", GroupID: 1, Endpoint: "https://8.8.8.8/v1/chat/completions", APIKey: "private-key", Model: "requested-model"}
	task, err := svc.prepareTask(context.Background(), in, nil)
	require.NoError(t, err)
	require.Equal(t, "encrypted:private-key", task.APIKeyEncrypted)
	require.True(t, task.HasAPIKey)
	require.Equal(t, 3600, task.IntervalSeconds)
	require.Equal(t, 7, task.RetentionDays)
	require.Equal(t, 50, task.MaxRecords)
	raw, err := json.Marshal(task)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "private-key")
	require.NotContains(t, string(raw), "encrypted:")
	in.APIKey = ""
	updated, err := svc.prepareTask(context.Background(), in, task)
	require.NoError(t, err)
	require.Equal(t, task.APIKeyEncrypted, updated.APIKeyEncrypted)
	for _, change := range []func(*ModelEvaluationTaskInput){func(i *ModelEvaluationTaskInput) { i.Model = "" }, func(i *ModelEvaluationTaskInput) { i.IntervalSeconds = 59 }, func(i *ModelEvaluationTaskInput) { i.RetentionDays = 91 }, func(i *ModelEvaluationTaskInput) { i.MaxRecords = 201 }, func(i *ModelEvaluationTaskInput) { i.APIFormat = "unknown" }, func(i *ModelEvaluationTaskInput) { i.APIKey = "secret\r\ninjected" }} {
		bad := in
		change(&bad)
		_, err := svc.prepareTask(context.Background(), bad, task)
		require.ErrorIs(t, err, ErrModelEvaluationInvalid)
	}
}

func TestModelEvaluationCredentialMustBeReenteredWhenBindingChanges(t *testing.T) {
	svc := NewModelEvaluationService(nil, nil, evaluationTestGroups{}, evaluationTestEncryptor{})
	defer svc.Stop()
	in := ModelEvaluationTaskInput{Name: "original", GroupID: 1, Endpoint: "https://8.8.8.8/v1/chat/completions", APIFormat: "chat_completions", APIKey: "original-secret", Model: "original-model"}
	old, err := svc.prepareTask(context.Background(), in, nil)
	require.NoError(t, err)
	for name, change := range map[string]func(*ModelEvaluationTaskInput){
		"endpoint host": func(i *ModelEvaluationTaskInput) { i.Endpoint = "https://8.8.4.4/v1/chat/completions" },
		"endpoint path": func(i *ModelEvaluationTaskInput) { i.Endpoint = "https://8.8.8.8/another-provider/chat/completions" },
		"API protocol":  func(i *ModelEvaluationTaskInput) { i.APIFormat = "responses" },
		"group":         func(i *ModelEvaluationTaskInput) { i.GroupID = 2 },
	} {
		t.Run(name, func(t *testing.T) {
			changed := in
			changed.APIKey = "  "
			change(&changed)
			_, err := svc.prepareTask(context.Background(), changed, old)
			require.ErrorIs(t, err, ErrModelEvaluationCredentialRequired)
			require.Contains(t, err.Error(), "re-entering the API Key")
			require.NotContains(t, err.Error(), "original-secret")
			require.NotContains(t, err.Error(), changed.Endpoint)
			changed.APIKey = "new-secret"
			updated, err := svc.prepareTask(context.Background(), changed, old)
			require.NoError(t, err)
			require.Equal(t, "encrypted:new-secret", updated.APIKeyEncrypted)
			require.Equal(t, "encrypted:original-secret", old.APIKeyEncrypted)
		})
	}
	in.APIKey = ""
	in.Name = "new name"
	in.Model = "new-model"
	in.IntervalSeconds = 120
	in.RetentionDays = 14
	in.MaxRecords = 75
	in.Enabled = true
	in.Endpoint = "  " + in.Endpoint + "  "
	updated, err := svc.prepareTask(context.Background(), in, old)
	require.NoError(t, err)
	require.Equal(t, old.APIKeyEncrypted, updated.APIKeyEncrypted)
}

func TestModelEvaluationEndpointAndTransportBlockSSRF(t *testing.T) {
	for _, endpoint := range []string{"http://8.8.8.8/v1/chat/completions", "https://localhost/v1/messages", "https://127.0.0.1/v1/messages", "https://10.0.0.1/v1/messages", "https://[::ffff:127.0.0.1]/v1/messages", "https://169.254.169.254/latest/meta-data", "https://224.0.0.1/v1/messages", "https://user:secret@8.8.8.8/v1/messages", "https://8.8.8.8/v1/messages?key=secret"} {
		require.ErrorIs(t, validateModelEvaluationEndpoint(context.Background(), endpoint), ErrModelEvaluationInvalid, endpoint)
	}
	require.ErrorIs(t, validateModelEvaluationEndpoint(context.Background(), "https://8.8.8.8"), ErrModelEvaluationEndpointPathRequired)
	for _, address := range []string{"127.0.0.1:443", "[::1]:443", "169.254.169.254:443", "224.0.0.1:443", "localhost:443"} {
		conn, err := modelEvaluationSafeDialContext(context.Background(), "tcp", address)
		require.Error(t, err)
		require.Nil(t, conn)
	}
	svc := NewModelEvaluationService(nil, nil, nil, nil)
	defer svc.Stop()
	transport := svc.client.Transport.(*http.Transport)
	require.Nil(t, transport.Proxy)
	require.Error(t, svc.client.CheckRedirect(nil, nil))
}

const evaluationTestHTML = `<!DOCTYPE html><html><body><svg><circle r="10"/></svg><script>/* original animation */</script></body></html>`

func TestModelEvaluationProtocolsPreservePromptModelAndHTML(t *testing.T) {
	for _, format := range []string{"chat_completions", "responses", "messages"} {
		t.Run(format, func(t *testing.T) {
			svc := NewModelEvaluationService(nil, nil, nil, evaluationTestEncryptor{})
			defer svc.Stop()
			svc.client.Transport = evaluationTestTransport(func(req *http.Request) (*http.Response, error) {
				require.Equal(t, "https://provider.test/custom/endpoint", req.URL.String())
				require.Equal(t, "POST", req.Method)
				var body map[string]any
				require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
				require.Equal(t, "configured-model", body["model"])
				require.Equal(t, true, body["stream"])
				if format == "responses" {
					require.Equal(t, ModelEvaluationPrompt, body["input"])
				} else {
					messages := body["messages"].([]any)
					require.Len(t, messages, 1)
					require.Equal(t, ModelEvaluationPrompt, messages[0].(map[string]any)["content"])
				}
				if format == "messages" {
					require.Equal(t, "test-api-key", req.Header.Get("x-api-key"))
					require.Equal(t, "2023-06-01", req.Header.Get("anthropic-version"))
					require.Empty(t, req.Header.Get("Authorization"))
				} else {
					require.Equal(t, "Bearer test-api-key", req.Header.Get("Authorization"))
				}
				var response any
				switch format {
				case "chat_completions":
					response = map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": "下面是完整 HTML 代码：\n```html\n" + evaluationTestHTML + "\n```\n保存为 index.html 后即可打开。"}}}}
				case "responses":
					response = map[string]any{"status": "completed", "output": []any{map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]string{"type": "output_text", "text": evaluationTestHTML}}}}}
				case "messages":
					response = map[string]any{"stop_reason": "end_turn", "content": []any{map[string]string{"type": "text", "text": evaluationTestHTML}}}
				}
				raw, err := json.Marshal(response)
				require.NoError(t, err)
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(raw))), Header: http.Header{}}, nil
			})
			result := svc.execute(context.Background(), &ModelEvaluationTask{ID: 1, GroupID: 2, Endpoint: "https://provider.test/custom/endpoint", Model: "configured-model", APIFormat: format, APIKeyEncrypted: "encrypted:test-api-key"})
			require.Equal(t, "success", result.Status)
			require.Empty(t, result.ErrorMessage)
			require.Equal(t, evaluationTestHTML, result.HTML)
		})
	}
}

func TestModelEvaluationRejectsTruncatedMalformedAndOversizedOutput(t *testing.T) {
	for _, input := range []string{"", "<svg></svg>", "<!doctype html><html><body>partial", "```html\n" + evaluationTestHTML, strings.Repeat("x", ModelEvaluationMaxHTMLBytes+1)} {
		html, reason := extractModelEvaluationHTML(input)
		require.Empty(t, html)
		require.NotEmpty(t, reason)
	}
	for _, tt := range []struct{ format, raw string }{{"chat_completions", `{"choices":[{"finish_reason":"length","message":{"content":"secret"}}]}`}, {"responses", `{"status":"incomplete","error":{"message":"secret"}}`}, {"messages", `{"stop_reason":"max_tokens","content":[{"type":"text","text":"secret"}]}`}, {"chat_completions", `<html>secret</html>`}} {
		html, reason := parseModelEvaluationResponse(tt.format, []byte(tt.raw))
		require.Empty(t, html)
		require.NotEmpty(t, reason)
		require.NotContains(t, reason, "secret")
	}
}

func TestModelEvaluationExtractsSingleDocumentWithoutRewriting(t *testing.T) {
	document := "<!DOCTYPE html>\r\n<HTML lang='zh-CN'>\r\n<head><title>鹈鹕骑自行车</title></head>\r\n<body>\t<svg viewBox='0 0 200 100'><text>骑行 &amp; 动画</text></svg>\r\n<!-- example: <html></html> -->\r\n<script>const example = '<html>literal</html>';\r\nconst ticks = `one\n```\ntwo`;\r\n</script>\r\n</body>\r\n</HTML>"
	for name, input := range map[string]string{
		"raw":                      document,
		"raw with explanations":    "下面是完整的 HTML，无需测试。\n" + document + "\n保存为 index.html 即可打开。",
		"fenced with explanations": "下面是完整的 HTML：\r\n````html\r\n" + document + "\r\n````\r\n保存后可在浏览器运行。",
		"uppercase language":       "```HTML\n" + evaluationTestHTML + "\n```\n以上是完整代码。",
		"unspecified language":     "说明\n```\n" + evaluationTestHTML + "\n```\n结束",
		"tilde fence":              "说明\n~~~html\n" + evaluationTestHTML + "\n~~~\n结束",
		"unrelated fenced text":    "```json\n{\"note\":\"example\"}\n```\n```html\n" + evaluationTestHTML + "\n```",
	} {
		t.Run(name, func(t *testing.T) {
			got, reason := extractModelEvaluationHTML(input)
			require.Empty(t, reason)
			expected := document
			if name == "uppercase language" || name == "unspecified language" || name == "tilde fence" || name == "unrelated fenced text" {
				expected = evaluationTestHTML
			}
			require.Equal(t, expected, got)
		})
	}
}

func TestModelEvaluationRejectsAmbiguousDocumentsAndBrokenFences(t *testing.T) {
	for name, input := range map[string]string{
		"two raw documents":         evaluationTestHTML + "\n" + evaluationTestHTML,
		"two fenced documents":      "```html\n" + evaluationTestHTML + "\n```\n另一个版本：\n```html\n" + evaluationTestHTML + "\n```",
		"raw and fenced":            evaluationTestHTML + "\n```html\n" + evaluationTestHTML + "\n```",
		"partial trailing document": evaluationTestHTML + "\n<!doctype html><html><body>truncated",
		"partial fenced document":   "前言\n```html\n<!doctype html><html><body>truncated\n```\n后记",
		"unterminated fence":        "前言\n```html\n" + evaluationTestHTML,
		"incorrect fence closure":   "````html\n" + evaluationTestHTML + "\n```",
		"NUL inside":                strings.Replace(evaluationTestHTML, "original", "original\x00", 1),
		"NUL explanation":           "invalid\x00\n" + evaluationTestHTML,
		"oversized document":        "```html\n<!doctype html><html><body>" + strings.Repeat("x", ModelEvaluationMaxHTMLBytes) + "</body></html>\n```",
		"unrelated code language":   "```javascript\n" + evaluationTestHTML + "\n```",
	} {
		t.Run(name, func(t *testing.T) {
			got, reason := extractModelEvaluationHTML(input)
			require.Empty(t, got)
			require.NotEmpty(t, reason)
		})
	}
}

func TestModelEvaluationProviderFailuresNeverEchoSecrets(t *testing.T) {
	for _, tt := range []struct {
		status         int
		body           string
		transportError bool
	}{{401, `secret-api-key`, false}, {200, `{"error":{"message":"secret-api-key"}}`, false}, {200, strings.Repeat("x", ModelEvaluationMaxResponseBytes+1), false}, {0, "", true}} {
		svc := NewModelEvaluationService(nil, nil, nil, evaluationTestEncryptor{})
		svc.client.Transport = evaluationTestTransport(func(*http.Request) (*http.Response, error) {
			if tt.transportError {
				return nil, errors.New("upstream secret-api-key")
			}
			return &http.Response{StatusCode: tt.status, Body: io.NopCloser(strings.NewReader(tt.body)), Header: http.Header{}}, nil
		})
		result := svc.execute(context.Background(), &ModelEvaluationTask{Endpoint: "https://provider.test/v1/chat/completions", APIFormat: "chat_completions", Model: "model", APIKeyEncrypted: "encrypted:secret-api-key"})
		require.Equal(t, "error", result.Status)
		require.Empty(t, result.HTML)
		require.NotContains(t, result.ErrorMessage, "secret-api-key")
		svc.Stop()
	}
}

type evaluationSchedulerRepo struct {
	ModelEvaluationRepository
	mu        sync.Mutex
	current   bool
	completed atomic.Int32
	released  chan struct{}
}

func (r *evaluationSchedulerRepo) Claim(context.Context, int64, bool) (*ModelEvaluationTask, error) {
	return &ModelEvaluationTask{ID: 1, LeaseToken: "lease", Endpoint: "https://provider.test/v1/chat/completions", APIFormat: "chat_completions", Model: "model", APIKeyEncrypted: "encrypted:test-key"}, nil
}
func (r *evaluationSchedulerRepo) LeaseCurrent(context.Context, *ModelEvaluationTask) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.current, nil
}
func (r *evaluationSchedulerRepo) Complete(context.Context, *ModelEvaluationTask, *ModelEvaluationResult) (bool, error) {
	r.completed.Add(1)
	return true, nil
}
func (r *evaluationSchedulerRepo) Release(context.Context, *ModelEvaluationTask) error {
	select {
	case r.released <- struct{}{}:
	default:
	}
	return nil
}
func (r *evaluationSchedulerRepo) SetEnabled(_ context.Context, b bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.current = b
	return nil
}

func TestModelEvaluationDisableAndStopCancelWithoutSaving(t *testing.T) {
	for _, stop := range []bool{false, true} {
		r := &evaluationSchedulerRepo{current: true, released: make(chan struct{}, 1)}
		svc := NewModelEvaluationService(r, evaluationTestSettings{value: "true"}, nil, evaluationTestEncryptor{})
		started := make(chan struct{})
		cancelled := make(chan struct{})
		svc.client.Transport = evaluationTestTransport(func(req *http.Request) (*http.Response, error) {
			close(started)
			<-req.Context().Done()
			close(cancelled)
			return nil, req.Context().Err()
		})
		require.NoError(t, svc.RunNow(context.Background(), 1))
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("request did not start")
		}
		if stop {
			svc.Stop()
		} else {
			_, err := svc.UpdateConfig(context.Background(), ModelEvaluationConfig{})
			require.NoError(t, err)
		}
		select {
		case <-cancelled:
		case <-time.After(time.Second):
			t.Fatal("request not cancelled")
		}
		select {
		case <-r.released:
		case <-time.After(time.Second):
			t.Fatal("lease not released")
		}
		svc.Stop()
		require.Zero(t, r.completed.Load())
	}
}

type evaluationPrivateTestRepo struct{ *evaluationSchedulerRepo }

func (r *evaluationPrivateTestRepo) ClaimTest(ctx context.Context, id int64) (*ModelEvaluationTask, error) {
	task, err := r.Claim(ctx, id, true)
	if task != nil {
		task.LeaseIsTest = true
	}
	return task, err
}
func (r *evaluationPrivateTestRepo) SetEnabled(context.Context, bool) error { return nil }

func TestModelEvaluationPrivateTestWorksWhileDisabledAndSurvivesGlobalDisable(t *testing.T) {
	r := &evaluationPrivateTestRepo{&evaluationSchedulerRepo{current: true, released: make(chan struct{}, 1)}}
	svc := NewModelEvaluationService(r, evaluationTestSettings{value: "false"}, nil, evaluationTestEncryptor{})
	defer svc.Stop()
	started := make(chan struct{})
	finish := make(chan struct{})
	cancelled := make(chan struct{})
	svc.client.Transport = evaluationTestTransport(func(req *http.Request) (*http.Response, error) {
		close(started)
		select {
		case <-req.Context().Done():
			close(cancelled)
			return nil, req.Context().Err()
		case <-finish:
		}
		body, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": evaluationTestHTML}}}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body))), Header: http.Header{}}, nil
	})
	require.ErrorIs(t, svc.RunNow(context.Background(), 1), ErrModelEvaluationDisabled)
	require.NoError(t, svc.TestTask(context.Background(), 1))
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("private test did not start")
	}
	_, err := svc.UpdateConfig(context.Background(), ModelEvaluationConfig{Enabled: false})
	require.NoError(t, err)
	select {
	case <-cancelled:
		t.Fatal("global scheduling switch cancelled explicit private test")
	default:
	}
	close(finish)
	select {
	case <-r.released:
	case <-time.After(time.Second):
		t.Fatal("private test did not finish")
	}
	require.Equal(t, int32(1), r.completed.Load())
}

type evaluationResultsRepo struct {
	ModelEvaluationRepository
	items []*ModelEvaluationResult
}

func (r *evaluationResultsRepo) ListResults(context.Context, ModelEvaluationListParams) ([]*ModelEvaluationResult, int64, error) {
	return r.items, int64(len(r.items)), nil
}
func (r *evaluationResultsRepo) GetResult(_ context.Context, id int64, _ ModelEvaluationListParams) (*ModelEvaluationResult, error) {
	return r.items[id-1], nil
}

func TestModelEvaluationUserResultsRedactDiagnosticsAndPrivateTestFailures(t *testing.T) {
	r := &evaluationResultsRepo{items: []*ModelEvaluationResult{{ID: 1, Status: "error", ErrorMessage: "private upstream diagnostic", HTML: "unsafe failed HTML"}, {ID: 2, Status: "error", IsTest: true, ErrorMessage: "private test failure"}, {ID: 3, Status: "success", HTML: evaluationTestHTML, ErrorMessage: "stray secret"}}}
	svc := NewModelEvaluationService(r, evaluationTestSettings{value: "true"}, nil, nil)
	defer svc.Stop()
	p := ModelEvaluationListParams{AllowedGroupIDs: []int64{1}}
	items, total, err := svc.ListResults(context.Background(), p)
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, int64(2), total)
	require.Equal(t, "生成失败", items[0].ErrorMessage)
	require.Empty(t, items[0].HTML)
	require.Empty(t, items[1].ErrorMessage)
	require.Equal(t, evaluationTestHTML, items[1].HTML)
	result, err := svc.GetResult(context.Background(), 1, p)
	require.NoError(t, err)
	require.Equal(t, "生成失败", result.ErrorMessage)
	_, err = svc.GetResult(context.Background(), 2, p)
	require.ErrorIs(t, err, ErrModelEvaluationNotFound)
	adminItems, total, err := svc.ListResults(context.Background(), ModelEvaluationListParams{Admin: true})
	require.NoError(t, err)
	require.Len(t, adminItems, 3)
	require.Equal(t, int64(3), total)
	require.Equal(t, "private upstream diagnostic", adminItems[0].ErrorMessage)
	require.Equal(t, "private test failure", adminItems[1].ErrorMessage)
}
