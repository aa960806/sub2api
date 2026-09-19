package media

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const compatBody = `{"model":"seedance2.0","content":[{"type":"text","text":"a cat"}],"duration":5,"ratio":"16:9","resolution":"720p","generate_audio":true,"watermark":false}`
const compatPath = "/v1/contents/generations/tasks"
const compatPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aOWQAAAAASUVORK5CYII="

func setupCompatEnv(t *testing.T) *mediaTestEnv {
	e := setupMediaEnv(t)
	e.handler.tasks.withDownloadSigningSecret("unit-test-signing-secret")
	e.router.GET("/v1/models", e.handler.SeedanceModels)
	e.router.POST(compatPath, e.handler.CreateSeedanceTask)
	e.router.GET(compatPath+"/:task_id", e.handler.GetSeedanceTask)
	e.router.GET("/v1/media/results/:task_id", e.handler.GetSeedanceResult)
	return e
}

func compatJSON(t *testing.T, r *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var result map[string]any
	require.NoError(t, json.Unmarshal(r.Body.Bytes(), &result), r.Body.String())
	return result
}

func compatID(t *testing.T, r *httptest.ResponseRecorder) string {
	t.Helper()
	require.Equal(t, http.StatusAccepted, r.Code, r.Body.String())
	return compatJSON(t, r)["id"].(string)
}

func TestSeedanceCompatLifecycleAndCapabilityDownload(t *testing.T) {
	e := setupCompatEnv(t)
	r := e.do("POST", compatPath, compatBody, "")
	id := compatID(t, r)
	require.NotEmpty(t, r.Header().Get("Idempotency-Key"))
	require.Equal(t, "queued", compatJSON(t, r)["status"])
	e.provider.setStatus(e.provider.createdJobs[0], MediaTaskStatusSucceeded)
	billing := e.handler.tasks.Billing().(*lifecycleBilling)
	billing.captureErr = errors.New("settlement temporarily unavailable")
	require.Error(t, e.handler.advanceTask(context.Background(), id, false))
	r = e.do("GET", compatPath+"/"+id, "", "")
	require.Equal(t, "running", compatJSON(t, r)["status"])
	require.NotContains(t, compatJSON(t, r), "content")
	billing.captureErr = nil
	require.NoError(t, e.handler.advanceTask(context.Background(), id, false))
	r = e.do("GET", compatPath+"/"+id, "", "")
	response := compatJSON(t, r)
	require.Equal(t, "succeeded", response["status"])
	resultURL := response["content"].(map[string]any)["video_url"].(string)
	require.NotContains(t, resultURL, "api_key")
	require.NotContains(t, resultURL, "up-img")
	// Separate unauthenticated router proves the signed capability is enough.
	public := gin.New()
	public.GET("/v1/media/results/:task_id", e.handler.GetSeedanceResult)
	request := httptest.NewRequest("GET", resultURL, nil)
	request.Header.Set("Range", "bytes=0-3")
	download := httptest.NewRecorder()
	public.ServeHTTP(download, request)
	require.Equal(t, 206, download.Code, download.Body.String())
	require.Equal(t, "MP4DATA", download.Body.String())
	require.Equal(t, "bytes 0-3/8", download.Header().Get("Content-Range"))
	require.Equal(t, "no-store", download.Header().Get("Cache-Control"))
	require.Equal(t, 2.0, billing.charged)

	for _, mutate := range []func(*url.URL){
		func(u *url.URL) { u.RawQuery = "" },
		func(u *url.URL) { q := u.Query(); q.Set("signature", "bad"); u.RawQuery = q.Encode() },
		func(u *url.URL) { u.Path = "/v1/media/results/" + NewMediaID() },
		func(u *url.URL) {
			q := u.Query()
			exp := strconv.FormatInt(time.Now().Add(-time.Minute).Unix(), 10)
			q.Set("expires", exp)
			q.Set("signature", base64.RawURLEncoding.EncodeToString(e.handler.tasks.resultSignature(id, exp)))
			u.RawQuery = q.Encode()
		},
		func(u *url.URL) {
			exp := strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)
			u.RawQuery = url.Values{"expires": {exp}, "signature": {base64.RawURLEncoding.EncodeToString(e.handler.tasks.resultSignature(id, exp))}}.Encode()
		},
	} {
		u, err := url.Parse(resultURL)
		require.NoError(t, err)
		mutate(u)
		w := httptest.NewRecorder()
		public.ServeHTTP(w, httptest.NewRequest("GET", u.String(), nil))
		require.Equal(t, 403, w.Code)
	}
	// Key rotation invalidates a previously issued result URL.
	e.handler.tasks.withDownloadSigningSecret("rotated")
	w := httptest.NewRecorder()
	public.ServeHTTP(w, httptest.NewRequest("GET", resultURL, nil))
	require.Equal(t, 403, w.Code)
}

func TestSeedanceCompatOwnershipIdempotencyAndDisable(t *testing.T) {
	e := setupCompatEnv(t)
	id := compatID(t, e.do("POST", compatPath, compatBody, "repeat"))
	r := e.do("POST", compatPath, compatBody, "repeat")
	require.Equal(t, id, compatID(t, r))
	require.Equal(t, "true", r.Header().Get("X-Idempotency-Replayed"))
	require.Equal(t, 1, e.provider.createCalls)
	require.Equal(t, 409, e.do("POST", compatPath, strings.Replace(compatBody, "a cat", "a dog", 1), "repeat").Code)
	other := *currentAPIKey
	other.ID++
	currentAPIKey = &other
	require.Equal(t, 404, e.do("GET", compatPath+"/"+id, "", "").Code)
	other.ID--
	other.UserID++
	require.Equal(t, 404, e.do("GET", compatPath+"/"+id, "", "").Code)
	other.UserID--
	newID := compatID(t, e.do("POST", compatPath, compatBody, ""))
	require.NotEqual(t, id, newID, "same prompt with no key is an intentional new task")
	e.handler.tasks.WithEnabled(false)
	require.Equal(t, 404, e.do("POST", compatPath, compatBody, "").Code)
	require.Equal(t, 200, e.do("GET", compatPath+"/"+id, "", "").Code)
}

type compatAuditSpy struct {
	body  []byte
	block bool
}

func (a *compatAuditSpy) Check(_ context.Context, request securityaudit.Request) (*securityaudit.LegacyDecision, error) {
	a.body = append([]byte(nil), request.Body...)
	return &securityaudit.LegacyDecision{Allowed: !a.block, Blocked: a.block, StatusCode: 403, ErrorCode: "blocked", Message: "blocked"}, nil
}

func TestSeedanceCompatInlineImagesAuditAndReplay(t *testing.T) {
	e := setupCompatEnv(t)
	input := strings.Replace(compatBody, `"text":"a cat"}`, `"text":"a cat"},{"type":"image_url","role":"reference_image","image_url":{"url":"data:image/png;base64,`+compatPNG+`"}}`, 1)
	audit := &compatAuditSpy{block: true}
	e.handler.securityAuditCoordinator = securityaudit.NewCoordinator(audit, nil)
	r := e.do("POST", compatPath, input, "images")
	require.Equal(t, 403, r.Code, r.Body.String())
	require.Empty(t, e.store.tasks)
	require.Empty(t, e.handler.tasks.Billing().(*lifecycleBilling).reserved)
	parsed := service.ExtractContentModerationInput(service.ContentModerationProtocolOpenAIImages, audit.body)
	require.Equal(t, "a cat", parsed.Text)
	require.Equal(t, []string{"data:image/png;base64," + compatPNG}, parsed.Images)
	audit.block = false
	id := compatID(t, e.do("POST", compatPath, input, "images"))
	task, err := e.store.GetTask(context.Background(), id)
	require.NoError(t, err)
	require.Empty(t, task.Request.InlineImages, "image bytes must not remain in journal")
	require.Equal(t, []string{e.provider.uploadedID}, e.provider.lastImageIDs)
	require.EqualValues(t, 1, e.provider.lastAccount)
	e.resolver.selected = 2
	require.Equal(t, id, compatID(t, e.do("POST", compatPath, input, "images")))
	require.Equal(t, 1, e.provider.createCalls)
}

func TestSeedanceCompatRejectsUnsupportedInputsBeforeBilling(t *testing.T) {
	for name, body := range map[string]string{
		"duration":       strings.Replace(compatBody, `"duration":5`, `"duration":6`, 1),
		"resolution":     strings.Replace(compatBody, `"720p"`, `"1080p"`, 1),
		"watermark":      strings.Replace(compatBody, `"watermark":false`, `"watermark":true`, 1),
		"audio switch":   strings.Replace(compatBody, `"generate_audio":true`, `"generate_audio":false`, 1),
		"video":          strings.Replace(compatBody, `"type":"text"`, `"type":"video_url"`, 1),
		"invalid inline": strings.Replace(compatBody, `"type":"text","text":"a cat"`, `"type":"image_url","image_url":{"url":"data:image/png;base64,broken"}`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			e := setupCompatEnv(t)
			r := e.do("POST", compatPath, body, "")
			require.Equal(t, 400, r.Code, r.Body.String())
			require.Zero(t, e.provider.createCalls)
			require.Empty(t, e.store.tasks)
			require.Empty(t, e.handler.tasks.Billing().(*lifecycleBilling).reserved)
		})
	}
}

func TestSeedanceCompatModelsFilterAndGenericFailure(t *testing.T) {
	e := setupCompatEnv(t)
	r := e.do("GET", "/v1/models", "", "")
	require.Equal(t, "seedance2.0", compatJSON(t, r)["data"].([]any)[0].(map[string]any)["id"])
	currentAPIKey.Group.ModelAllowlist = service.GroupModelAllowlist{Enabled: true, Models: []string{"seedance2.0mini"}}
	require.Empty(t, compatJSON(t, e.do("GET", "/v1/models", "", ""))["data"])
	id := compatID(t, e.do("POST", compatPath, compatBody, "failed"))
	e.provider.setStatus(e.provider.createdJobs[0], MediaTaskStatusFailed)
	require.NoError(t, e.handler.advanceTask(context.Background(), id, false))
	r = e.do("GET", compatPath+"/"+id, "", "")
	require.Equal(t, "failed", compatJSON(t, r)["status"])
	require.Equal(t, "Video generation failed", compatJSON(t, r)["error"].(map[string]any)["message"])
	require.NotContains(t, compatJSON(t, r), "content")
	require.Equal(t, 100.0, e.handler.tasks.Billing().(*lifecycleBilling).balance)
}

func TestSeedanceNativeHashRemainsCompatible(t *testing.T) {
	req := &MediaVideoCreateRequest{Prompt: "test", Model: "seedance2.0", DurationSeconds: 5}
	encoded, err := json.Marshal(req)
	require.NoError(t, err)
	require.JSONEq(t, `{"Prompt":"test","Model":"seedance2.0","DurationSeconds":5,"Ratio":"","Resolution":"","CameraMovement":"","FileIDs":null,"ImageURLs":null}`, string(encoded))
}
