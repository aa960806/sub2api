package media

import (
	"context"
	"encoding/base64"
	"encoding/json"
	goerrors "errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"

	"github.com/stretchr/testify/require"
)

// directUpstream 让 provider 直接打到 httptest 服务器，跳过代理与并发治理。
type directUpstream struct{ client *http.Client }

func (d *directUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return d.client.Do(req)
}

func (d *directUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return d.Do(req, proxyURL, accountID, accountConcurrency)
}

// errMeta 取出 ApplicationError 上的元数据（上游 request_id、retry_after 等）。
func errMeta(err error) map[string]string {
	var appErr *infraerrors.ApplicationError
	if goerrors.As(err, &appErr) && appErr != nil {
		return appErr.Metadata
	}
	return map[string]string{}
}

func newSeedanceTestEnv(t *testing.T, handler http.HandlerFunc) (*SeedanceProvider, *Account) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	provider := NewSeedanceProvider(&directUpstream{client: srv.Client()})
	account := &Account{
		ID:       7,
		Platform: PlatformSeedance,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "sk-test-key",
			"base_url": srv.URL,
		},
	}
	return provider, account
}

func TestSeedanceCreateAcceptsHTTP202(t *testing.T) {
	var gotAuth, gotIdem, gotBody string
	provider, account := newSeedanceTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/videos", r.URL.Path)
		gotAuth = r.Header.Get("Authorization")
		gotIdem = r.Header.Get("Idempotency-Key")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"id":"job-abc","status":"queued"}`))
	})

	job, err := provider.Create(context.Background(), account, &MediaVideoCreateRequest{
		Prompt: "a cat", Model: SeedanceModel20, DurationSeconds: 5, Ratio: "16:9",
	}, []string{"img-1"}, "idem-123")

	require.NoError(t, err)
	require.Equal(t, "job-abc", job.JobID)
	require.Equal(t, MediaTaskStatusQueued, job.Status)
	require.False(t, job.Idempotent)
	// Bearer 后必须有空格。
	require.Equal(t, "Bearer sk-test-key", gotAuth)
	require.Equal(t, "idem-123", gotIdem)
	require.Contains(t, gotBody, `"image_ids":["img-1"]`)
}

func TestSeedanceCreateSurfacesUpstreamIdempotentFlag(t *testing.T) {
	provider, account := newSeedanceTestEnv(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"id":"job-abc","status":"queued","idempotent":"1"}`))
	})
	job, err := provider.Create(context.Background(), account, &MediaVideoCreateRequest{
		Prompt: "a cat", Model: SeedanceModel20, DurationSeconds: 5,
	}, nil, "idem-123")
	require.NoError(t, err)
	require.True(t, job.Idempotent, "上游 idempotent=1 表示本次为幂等重放")
}

func TestSeedanceCreateRejectsEmptyJobID(t *testing.T) {
	provider, account := newSeedanceTestEnv(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"queued"}`))
	})
	_, err := provider.Create(context.Background(), account, &MediaVideoCreateRequest{
		Prompt: "a cat", Model: SeedanceModel20, DurationSeconds: 5,
	}, nil, "k")
	require.Error(t, err)
	require.Equal(t, "SEEDANCE_EMPTY_JOB_ID", infraerrors.Reason(err))
}

func TestSeedanceStatusReturnsRawStatusForFailClosed(t *testing.T) {
	provider, account := newSeedanceTestEnv(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"job-1","status":"some_future_state"}`))
	})
	job, err := provider.Status(context.Background(), account, "job-1")
	require.NoError(t, err)
	// provider 不猜测映射，保留原值让上层 fail-closed。
	require.Equal(t, "some_future_state", job.Status)
	require.Empty(t, NormalizeMediaStatus(job.Status))
}

func TestSeedanceStatusRejectsDifferentTask(t *testing.T) {
	provider, account := newSeedanceTestEnv(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"another-users-job","status":"succeeded"}`))
	})
	job, err := provider.Status(context.Background(), account, "job-1")
	require.Nil(t, job)
	require.Equal(t, "SEEDANCE_TASK_ID_MISMATCH", infraerrors.Reason(err))
}

func TestSeedanceCreateOnlyDefinitiveStructuredRejectionsAllowRefund(t *testing.T) {
	for _, tc := range []struct {
		name     string
		status   int
		body     string
		rejected bool
	}{
		{"invalid_request", 422, `{"error":{"code":"invalid_request"}}`, true},
		{"invalid_credential", 401, `{"error":{"type":"unauthorized"}}`, true},
		{"proxy_forbidden_html", 403, `<html>Forbidden</html>`, false},
		{"proxy_timeout", 502, `{"error":{"code":"timeout"}}`, false},
		{"html_homepage", 200, `<html>Home</html>`, false},
		{"invalid_accepted_payload", 202, `<html>Home</html>`, false},
		{"ambiguous_rate_limit", 429, `{"error":{"code":"rate_limit"}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider, account := newSeedanceTestEnv(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			job, err := provider.Create(context.Background(), account, &MediaVideoCreateRequest{
				Prompt: "a cat", Model: SeedanceModel20, DurationSeconds: 5,
			}, nil, "stable-key")
			require.Nil(t, job)
			require.Error(t, err)
			var rejected *SubmissionRejected
			require.Equal(t, tc.rejected, goerrors.As(err, &rejected))
		})
	}
}

func TestSeedanceStatusMapsAllFourStates(t *testing.T) {
	for _, st := range []string{"queued", "running", "succeeded", "failed"} {
		provider, account := newSeedanceTestEnv(t, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"id":"job-1","status":"` + st + `"}`))
		})
		job, err := provider.Status(context.Background(), account, "job-1")
		require.NoError(t, err)
		require.Equal(t, st, NormalizeMediaStatus(job.Status))
	}
}

func TestSeedanceSignedURLConflictMeansNotReady(t *testing.T) {
	provider, account := newSeedanceTestEnv(t, func(w http.ResponseWriter, _ *http.Request) {
		// 上游对「结果尚未就绪」返回 409 而不是 404。
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":{"code":"409","type":"not_found","message":"not ready","request_id":"r1"}}`))
	})
	ref, err := provider.FetchDownloadRef(context.Background(), account, "job-1")
	require.NoError(t, err, "409 必须映射为暂不可下载，而不是失败")
	require.True(t, ref.NotReady)
}

func TestSeedanceSignedURLResolvesRelativePath(t *testing.T) {
	var base string
	provider, account := newSeedanceTestEnv(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"url":"/v1/videos/job-1/file?exp=1&sig=abc","expires_at":123}`))
	})
	base = account.GetSeedanceBaseURL()

	ref, err := provider.FetchDownloadRef(context.Background(), account, "job-1")
	require.NoError(t, err)
	require.Equal(t, base+"/v1/videos/job-1/file?exp=1&sig=abc", ref.URL)
	require.EqualValues(t, 123, ref.ExpiresAt)
}

func TestSeedanceSignedURLRejectsForeignAndEscapingRefs(t *testing.T) {
	// 上游实测返回同源绝对 URL，故绝对形式本身不再一律拒绝；
	// 但指向其它主机的引用必须拒绝，否则上游能把网关引到任意地址（SSRF）。
	// 试图逃出 /v1/videos/ 前缀的同样拒绝。
	for _, bad := range []string{
		`https://evil.example/v1/videos/x/file`,
		`http://evil.example/v1/videos/x/file`,
		`//evil.example/v1/videos/x/file`,
		`/v1/videos/../../etc/passwd`,
		`/v1/files/img-1`,
		``,
		`v1/videos/x/file`,
	} {
		provider, account := newSeedanceTestEnv(t, func(w http.ResponseWriter, _ *http.Request) {
			body, _ := marshalJSONString(bad)
			_, _ = w.Write([]byte(`{"url":` + body + `,"expires_at":1}`))
		})
		_, err := provider.FetchDownloadRef(context.Background(), account, "job-1")
		require.Errorf(t, err, "should reject %q", bad)
		require.Equal(t, "SEEDANCE_INVALID_SIGNED_URL", infraerrors.Reason(err), "ref=%q", bad)
	}
}

// 生产实测：上游 /signed_url 返回的是同源绝对 URL，而不是站点相对路径。
// 早期实现只接受相对路径，于是 FetchDownloadRef 恒失败 —— 连锁导致
// downloadable 永远 false、settle 从不执行（账单不落库、冻结额悬挂），
// 取内容则报 SEEDANCE_INVALID_SIGNED_URL。
func TestSeedanceSignedURLAcceptsSameOriginAbsoluteURL(t *testing.T) {
	provider, account := newSeedanceTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		abs := "http://" + r.Host + "/v1/videos/job-1/file?exp=1788603697&sig=abc&stream=1"
		body, _ := marshalJSONString(abs)
		_, _ = w.Write([]byte(`{"url":` + body + `,"expires_at":1788603697}`))
	})
	ref, err := provider.FetchDownloadRef(context.Background(), account, "job-1")
	require.NoError(t, err)
	require.NotNil(t, ref)
	require.False(t, ref.NotReady)
	require.Contains(t, ref.URL, "/v1/videos/job-1/file")
	// 查询串必须原样保留：exp 与 sig 少一个，上游都会拒绝下载。
	require.Contains(t, ref.URL, "sig=abc")
	require.Contains(t, ref.URL, "exp=1788603697")
}

func TestSeedanceDownloadForwardsRangeAndPartialContent(t *testing.T) {
	provider, account := newSeedanceTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/signed_url") {
			_, _ = w.Write([]byte(`{"url":"/v1/videos/job-1/file","expires_at":1}`))
			return
		}
		require.Equal(t, "bytes=0-3", r.Header.Get("Range"))
		w.Header().Set("Content-Range", "bytes 0-3/8")
		w.Header().Set("Content-Type", "video/mp4")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("MP4D"))
	})
	ref, err := provider.FetchDownloadRef(context.Background(), account, "job-1")
	require.NoError(t, err)

	resp, err := provider.Download(context.Background(), account, ref, "bytes=0-3")
	require.NoError(t, err)
	defer func() { _ = resp.Close() }()
	require.Equal(t, http.StatusPartialContent, resp.StatusCode)
	require.Equal(t, "bytes 0-3/8", resp.Header.Get("Content-Range"))
	body, _ := io.ReadAll(resp.Body)
	require.Equal(t, "MP4D", string(body))
}

func TestSeedanceDownloadPassesThroughRangeNotSatisfiable(t *testing.T) {
	provider, account := newSeedanceTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/signed_url") {
			_, _ = w.Write([]byte(`{"url":"/v1/videos/job-1/file","expires_at":1}`))
			return
		}
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
	})
	ref, _ := provider.FetchDownloadRef(context.Background(), account, "job-1")
	resp, err := provider.Download(context.Background(), account, ref, "bytes=999-")
	require.NoError(t, err)
	defer func() { _ = resp.Close() }()
	require.Equal(t, http.StatusRequestedRangeNotSatisfiable, resp.StatusCode)
}

func TestSeedanceErrorMappingPerStatus(t *testing.T) {
	cases := []struct {
		upstream int
		reason   string
		outbound int
	}{
		{http.StatusPaymentRequired, "SEEDANCE_INSUFFICIENT_BALANCE", http.StatusPaymentRequired},
		{http.StatusUnauthorized, "SEEDANCE_KEY_UNAUTHORIZED", http.StatusBadGateway},
		{http.StatusForbidden, "SEEDANCE_FORBIDDEN", http.StatusBadGateway},
		{http.StatusNotFound, "SEEDANCE_UPSTREAM_NOT_FOUND", http.StatusNotFound},
		{http.StatusUnprocessableEntity, "SEEDANCE_UPSTREAM_REJECTED", http.StatusBadRequest},
		{http.StatusInternalServerError, "SEEDANCE_UPSTREAM_ERROR", http.StatusBadGateway},
	}
	for _, tc := range cases {
		provider, account := newSeedanceTestEnv(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.upstream)
			_, _ = w.Write([]byte(`{"error":{"code":"x","type":"validation_error","message":"boom","request_id":"req-9"}}`))
		})
		_, err := provider.Status(context.Background(), account, "job-1")
		require.Errorf(t, err, "upstream=%d", tc.upstream)
		require.Equal(t, tc.reason, infraerrors.Reason(err), "upstream=%d", tc.upstream)
		require.Equal(t, tc.outbound, infraerrors.Code(err), "upstream=%d", tc.upstream)
		// 上游 request_id 必须保留，便于向上游反馈问题。
		require.Contains(t, errMeta(err)["upstream_request_id"], "req-9")
	}
}

func TestSeedanceRateLimitTakesRetryAfterFromHeaderOrBody(t *testing.T) {
	// 头优先。
	provider, account := newSeedanceTestEnv(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "12")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"type":"rpm","message":"slow down","request_id":"r"},"retry_after":30}`))
	})
	_, err := provider.Status(context.Background(), account, "job-1")
	require.Error(t, err)
	require.Equal(t, "12", errMeta(err)["retry_after"])

	// 无头时回落到 JSON 体。
	provider2, account2 := newSeedanceTestEnv(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"type":"rpm","message":"slow down","request_id":"r"},"retry_after":30}`))
	})
	_, err = provider2.Status(context.Background(), account2, "job-1")
	require.Error(t, err)
	require.Equal(t, "30", errMeta(err)["retry_after"])
}

func TestSeedanceUploadReferenceImage(t *testing.T) {
	provider, account := newSeedanceTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/files", r.URL.Path)
		b, _ := io.ReadAll(r.Body)
		// 走 JSON image_b64，不是 multipart。
		require.Contains(t, string(b), `"image_b64"`)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"image_id":"img-9","format":"png","size":1024,"expires_at":999}`))
	})
	out, err := provider.UploadReferenceImage(context.Background(), account,
		base64.StdEncoding.EncodeToString([]byte("hello")))
	require.NoError(t, err)
	require.Equal(t, "img-9", out.ImageID)
	require.EqualValues(t, 999, out.ExpiresAt)
}

func TestSeedanceUploadRejectsOversizedImageBeforeUpstream(t *testing.T) {
	called := false
	provider, account := newSeedanceTestEnv(t, func(http.ResponseWriter, *http.Request) { called = true })
	provider.WithImageLimits(1024, 4096)

	_, err := provider.UploadReferenceImage(context.Background(), account, strings.Repeat("A", 64*1024))
	require.Error(t, err)
	require.Equal(t, "SEEDANCE_IMAGE_TOO_LARGE", infraerrors.Reason(err))
	require.False(t, called, "超限图片不应送到上游")
}

func TestSeedanceProbeCredentialUsesProtectedEndpoint(t *testing.T) {
	var hitPath, auth string
	provider, account := newSeedanceTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		hitPath = r.URL.Path
		auth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"balance":12345,"ledger":[]}`))
	})
	require.NoError(t, provider.ProbeCredential(context.Background(), account))
	// 必须打受保护端点：公开的 /v1/catalog、/health 不带鉴权也返回 200，
	// 通过它们无法证明该账号的 Key 有效。
	require.Equal(t, "/v1/balance", hitPath)
	require.Equal(t, "Bearer sk-test-key", auth)
}

func TestSeedanceProbeRejectsWellFormedButUnexpectedPayload(t *testing.T) {
	provider, account := newSeedanceTestEnv(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	err := provider.ProbeCredential(context.Background(), account)
	require.Error(t, err)
	require.Equal(t, "SEEDANCE_PROBE_INVALID_RESPONSE", infraerrors.Reason(err))
}

func TestSeedanceValidateCreateEnforcesModelDurationAndImageCount(t *testing.T) {
	provider := NewSeedanceProvider(nil)

	require.NoError(t, provider.ValidateCreate(&MediaVideoCreateRequest{
		Prompt: "x", Model: SeedanceModel25, DurationSeconds: 30,
	}))
	// 2.5 只支持 30 秒。
	require.Error(t, provider.ValidateCreate(&MediaVideoCreateRequest{
		Prompt: "x", Model: SeedanceModel25, DurationSeconds: 15,
	}))
	// mini 只支持 5/10。
	require.Error(t, provider.ValidateCreate(&MediaVideoCreateRequest{
		Prompt: "x", Model: SeedanceModel20Mini, DurationSeconds: 15,
	}))
	// 空 prompt。
	require.Error(t, provider.ValidateCreate(&MediaVideoCreateRequest{
		Prompt: "  ", Model: SeedanceModel20, DurationSeconds: 5,
	}))
	// 超长 prompt。
	require.Error(t, provider.ValidateCreate(&MediaVideoCreateRequest{
		Prompt: strings.Repeat("字", 6001), Model: SeedanceModel20, DurationSeconds: 5,
	}))
	// 张数上限：2.0 系列为 9 张，上传图与直链合并计数。
	tooMany := make([]string, 10)
	for i := range tooMany {
		tooMany[i] = "f"
	}
	err := provider.ValidateCreate(&MediaVideoCreateRequest{
		Prompt: "x", Model: SeedanceModel20, DurationSeconds: 5, FileIDs: tooMany,
	})
	require.Error(t, err)
	require.Equal(t, "SEEDANCE_TOO_MANY_IMAGES", infraerrors.Reason(err))
}

func TestSeedanceValidateCreateRejectsUnsafeImageURLs(t *testing.T) {
	provider := NewSeedanceProvider(nil)
	for _, bad := range []string{
		"http://example.com/a.png",                 // 非 https
		"https://127.0.0.1/a.png",                  // loopback
		"https://10.0.0.5/a.png",                   // RFC1918
		"https://[::1]/a.png",                      // IPv6 loopback
		"https://169.254.169.254/latest/meta-data", // link-local 元数据端点
	} {
		err := provider.ValidateCreate(&MediaVideoCreateRequest{
			Prompt: "x", Model: SeedanceModel20, DurationSeconds: 5, ImageURLs: []string{bad},
		})
		require.Errorf(t, err, "should reject %s", bad)
		require.Equal(t, "SEEDANCE_INVALID_IMAGE_URL", infraerrors.Reason(err), "url=%s", bad)
	}
}

func TestSeedanceMissingCredentialFailsClosed(t *testing.T) {
	provider, account := newSeedanceTestEnv(t, func(http.ResponseWriter, *http.Request) {
		t.Fatal("upstream must not be called without a credential")
	})
	account.Credentials["api_key"] = ""
	_, err := provider.Status(context.Background(), account, "job-1")
	require.Error(t, err)
	require.Equal(t, "SEEDANCE_MISSING_CREDENTIAL", infraerrors.Reason(err))
}

// marshalJSONString 把任意字符串编码成 JSON 字面量，供构造异常响应体。
func marshalJSONString(s string) (string, error) {
	b, err := json.Marshal(s)
	return string(b), err
}
