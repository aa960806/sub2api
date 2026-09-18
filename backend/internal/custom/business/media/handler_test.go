package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// ==== 假 provider ====

// fakeMediaProvider 让媒体任务骨架脱离真实上游完成端到端验收。
// 它只实现 MediaProvider 冻结的接口，不含任何协议细节。
type fakeMediaProvider struct {
	mu           sync.Mutex
	createCalls  int
	createdJobs  []string
	statusByJob  map[string]string
	notReady     bool
	createErr    error
	uploadedID   string
	lastImageIDs []string
	lastAccount  int64
	keys         map[string]string
}

func newFakeMediaProvider() *fakeMediaProvider {
	return &fakeMediaProvider{statusByJob: map[string]string{}, uploadedID: "up-img-1"}
}

func (p *fakeMediaProvider) Platform() string { return service.PlatformSeedance }

func (p *fakeMediaProvider) Capabilities() MediaCapabilities {
	return MediaCapabilities{
		Platform:    service.PlatformSeedance,
		Models:      []MediaModelCapability{{ID: "seedance2.0", Durations: []int{5, 10, 15}, MaxRefImages: 9}},
		Resolutions: []string{"720p"},
	}
}

func (p *fakeMediaProvider) ValidateCreate(req *MediaVideoCreateRequest) error {
	if strings.TrimSpace(req.Prompt) == "" {
		return ErrMediaTaskNotFound
	}
	if !IsValidSeedanceDuration(req.Model, req.DurationSeconds) {
		return errors.New("invalid model/duration combination")
	}
	return nil
}

func (p *fakeMediaProvider) Create(_ context.Context, account *service.Account, _ *MediaVideoCreateRequest, upstreamImageIDs []string, idem string) (*MediaProviderJob, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.keys == nil {
		p.keys = map[string]string{}
	}
	if job := p.keys[idem]; job != "" {
		return &MediaProviderJob{JobID: job, Status: MediaTaskStatusQueued, Idempotent: true}, nil
	}
	if p.createErr != nil {
		return nil, p.createErr
	}
	p.createCalls++
	p.lastImageIDs = upstreamImageIDs
	if account != nil {
		p.lastAccount = account.ID
	}
	jobID := fmt.Sprintf("job-%d", p.createCalls)
	p.createdJobs = append(p.createdJobs, jobID)
	p.keys[idem] = jobID
	p.statusByJob[jobID] = MediaTaskStatusQueued
	return &MediaProviderJob{JobID: jobID, Status: MediaTaskStatusQueued}, nil
}

func (p *fakeMediaProvider) Status(_ context.Context, _ *service.Account, jobID string) (*MediaProviderJob, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	st, ok := p.statusByJob[jobID]
	if !ok {
		return nil, ErrMediaTaskNotFound
	}
	return &MediaProviderJob{JobID: jobID, Status: st}, nil
}

func (p *fakeMediaProvider) setStatus(jobID, status string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.statusByJob[jobID] = status
}

func (p *fakeMediaProvider) FetchDownloadRef(_ context.Context, _ *service.Account, jobID string) (*MediaDownloadRef, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.notReady {
		return &MediaDownloadRef{NotReady: true}, nil
	}
	return &MediaDownloadRef{URL: "https://example.invalid/file/" + jobID, ExpiresAt: time.Now().Add(time.Minute).Unix()}, nil
}

func (p *fakeMediaProvider) Download(_ context.Context, _ *service.Account, _ *MediaDownloadRef, rangeHeader string) (*MediaDownloadResponse, error) {
	status := http.StatusOK
	header := http.Header{}
	if strings.TrimSpace(rangeHeader) != "" {
		status = http.StatusPartialContent
		header.Set("Content-Range", "bytes 0-3/8")
	}
	return &MediaDownloadResponse{
		StatusCode: status,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader("MP4DATA")),
	}, nil
}

func (p *fakeMediaProvider) UploadReferenceImage(_ context.Context, _ *service.Account, _ string) (*MediaProviderFile, error) {
	return &MediaProviderFile{
		ImageID:   p.uploadedID,
		Format:    "png",
		Size:      1024,
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}, nil
}

func (p *fakeMediaProvider) ProbeCredential(_ context.Context, _ *service.Account) error { return nil }

// ==== 内存存储 ====

type memMediaStore struct {
	mu      sync.Mutex
	tasks   map[string]*MediaTaskRecord
	files   map[string]*MediaFileRecord
	claims  map[string]bool
	holds   map[string]*MediaPendingHold
	failGet bool
	leases  map[string]string
}

func newMemMediaStore() *memMediaStore {
	return &memMediaStore{
		tasks:  map[string]*MediaTaskRecord{},
		files:  map[string]*MediaFileRecord{},
		claims: map[string]bool{},
		holds:  map[string]*MediaPendingHold{},
	}
}

func (s *memMediaStore) SaveTask(_ context.Context, t *MediaTaskRecord, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *t
	s.tasks[t.ID] = &cp
	return nil
}

func (s *memMediaStore) GetTask(_ context.Context, id string) (*MediaTaskRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failGet {
		return nil, errors.New("redis down")
	}
	t, ok := s.tasks[id]
	if !ok {
		return nil, ErrMediaTaskNotFound
	}
	cp := *t
	return &cp, nil
}

func (s *memMediaStore) SaveFile(_ context.Context, f *MediaFileRecord, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *f
	s.files[f.ID] = &cp
	return nil
}

func (s *memMediaStore) GetFile(_ context.Context, id string) (*MediaFileRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.files[id]
	if !ok {
		return nil, ErrMediaTaskNotFound
	}
	cp := *f
	return &cp, nil
}

func (s *memMediaStore) ClaimSettlement(_ context.Context, taskID string, _ time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.claims[taskID] {
		return false, nil
	}
	s.claims[taskID] = true
	return true, nil
}

func (s *memMediaStore) ReleaseSettlement(_ context.Context, taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.claims, taskID)
	return nil
}

func (s *memMediaStore) TrackPendingHold(_ context.Context, h *MediaPendingHold, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *h
	s.holds[h.TaskID] = &cp
	return nil
}

func (s *memMediaStore) ListDuePendingHolds(_ context.Context, now time.Time, _ int) ([]*MediaPendingHold, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []*MediaPendingHold{}
	for _, h := range s.holds {
		if h.DueAt <= now.Unix() {
			out = append(out, h)
		}
	}
	return out, nil
}

func (s *memMediaStore) DeletePendingHold(_ context.Context, taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.holds, taskID)
	return nil
}

// ==== 假账号解析器 ====

type fakeAccountResolver struct {
	accounts map[int64]*service.Account
	deleted  map[int64]bool
	selected int64
	selErr   error
}

func (r *fakeAccountResolver) SelectAccountForModelWithExclusions(_ context.Context, _ *int64, _ string, _ string, _ map[int64]struct{}) (*service.Account, error) {
	if r.selErr != nil {
		return nil, r.selErr
	}
	return r.accounts[r.selected], nil
}

func (r *fakeAccountResolver) GetMediaTaskAccount(_ context.Context, id int64) (*service.Account, error) {
	acct, ok := r.accounts[id]
	if !ok {
		return nil, ErrMediaAccountUnavailable
	}
	return acct, nil
}

func (r *fakeAccountResolver) GetMediaTaskAccountForCreate(ctx context.Context, id int64) (*service.Account, error) {
	if r.deleted[id] {
		return nil, ErrMediaAccountUnavailable
	}
	return r.GetMediaTaskAccount(ctx, id)
}

// ==== 测试装配 ====

type mediaTestEnv struct {
	router   *gin.Engine
	provider *fakeMediaProvider
	store    *memMediaStore
	resolver *fakeAccountResolver
	handler  *MediaTaskHandler
}

// currentAPIKey 允许单个测试在同一 router 上切换调用方身份，用于验证跨用户隔离。
var currentAPIKey *service.APIKey

func setupMediaEnv(t *testing.T) *mediaTestEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)

	provider := newFakeMediaProvider()
	store := newMemMediaStore()
	tasks := NewMediaTaskService(store, service.PlatformSeedance, provider).WithBilling(newLifecycleBilling())
	resolver := &fakeAccountResolver{
		accounts: map[int64]*service.Account{
			1: {ID: 1, Platform: service.PlatformSeedance, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, GroupIDs: []int64{9}, Concurrency: 10},
			2: {ID: 2, Platform: service.PlatformSeedance, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, GroupIDs: []int64{9}, Concurrency: 10},
		},
		selected: 1,
	}
	h := &MediaTaskHandler{
		tasks:             tasks,
		gatewayService:    resolver,
		concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(&mediaConcurrencyFake{}), SSEPingFormatNone, 0),
	}

	group := &service.Group{ID: 9, Platform: service.PlatformSeedance, RateMultiplier: 1}
	currentAPIKey = &service.APIKey{ID: 100, UserID: 10, GroupID: &group.ID, Group: group}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware2.ContextKeyAPIKey), currentAPIKey)
		c.Next()
	})
	router.POST("/v1/media/videos", h.CreateVideo)
	router.GET("/v1/media/videos/:task_id", h.GetVideo)
	router.GET("/v1/media/videos/:task_id/content", h.GetVideoContent)
	router.POST("/v1/media/files", h.UploadFile)
	router.GET("/v1/media/models", h.Models)

	return &mediaTestEnv{router: router, provider: provider, store: store, resolver: resolver, handler: h}
}

func (e *mediaTestEnv) do(method, path, body, idemKey string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if idemKey != "" {
		req.Header.Set("Idempotency-Key", idemKey)
	}
	if method == http.MethodGet && strings.Contains(path, "/videos/") {
		for _, task := range e.store.tasks {
			_ = e.handler.advanceTask(context.Background(), task.ID, false)
		}
	}
	e.router.ServeHTTP(rec, req)
	if method == http.MethodPost && rec.Code == http.StatusAccepted {
		for _, task := range e.store.tasks {
			_ = e.handler.advanceTask(context.Background(), task.ID, false)
		}
	}
	return rec
}

const validCreateBody = `{"prompt":"a cat","model":"seedance2.0","duration":5}`

func taskIDFrom(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var out struct {
		TaskID string `json:"task_id"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.NotEmpty(t, out.TaskID)
	return out.TaskID
}

// ==== 用例 ====

func TestMediaCreatePollDownloadHappyPath(t *testing.T) {
	env := setupMediaEnv(t)

	rec := env.do(http.MethodPost, "/v1/media/videos", validCreateBody, "key-1")
	require.Equal(t, http.StatusAccepted, rec.Code)
	taskID := taskIDFrom(t, rec)

	// 未终态时不可下载。
	rec = env.do(http.MethodGet, "/v1/media/videos/"+taskID, "", "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"status":"queued"`)
	require.Contains(t, rec.Body.String(), `"downloadable":false`)

	env.provider.setStatus(env.provider.createdJobs[0], MediaTaskStatusSucceeded)
	rec = env.do(http.MethodGet, "/v1/media/videos/"+taskID, "", "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"status":"succeeded"`)
	require.Contains(t, rec.Body.String(), `"downloadable":true`)

	rec = env.do(http.MethodGet, "/v1/media/videos/"+taskID+"/content", "", "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "MP4DATA", rec.Body.String())
}

func TestMediaCreateRequiresIdempotencyKey(t *testing.T) {
	env := setupMediaEnv(t)
	// ObserveOnly 默认为 true 会绕过 coordinator 的 RequireKey，
	// 因此 400 必须由 handler 自己给出。
	rec := env.do(http.MethodPost, "/v1/media/videos", validCreateBody, "")
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Zero(t, env.provider.createCalls)
}

func TestMediaCreateReplaysSameKeyWithoutSecondUpstreamJob(t *testing.T) {
	env := setupMediaEnv(t)

	first := env.do(http.MethodPost, "/v1/media/videos", validCreateBody, "key-replay")
	require.Equal(t, http.StatusAccepted, first.Code)

	second := env.do(http.MethodPost, "/v1/media/videos", validCreateBody, "key-replay")
	require.Equal(t, http.StatusAccepted, second.Code)
	require.Equal(t, "true", second.Header().Get("X-Idempotency-Replayed"))
	require.Equal(t, 1, env.provider.createCalls, "同一幂等键不得产生第二个上游任务")
	require.Equal(t, taskIDFrom(t, first), taskIDFrom(t, second))
}

func TestMediaCreateSameKeyDifferentPayloadConflicts(t *testing.T) {
	env := setupMediaEnv(t)

	require.Equal(t, http.StatusAccepted, env.do(http.MethodPost, "/v1/media/videos", validCreateBody, "key-x").Code)

	other := `{"prompt":"a dog","model":"seedance2.0","duration":10}`
	rec := env.do(http.MethodPost, "/v1/media/videos", other, "key-x")
	require.Equal(t, http.StatusConflict, rec.Code)
	require.Equal(t, 1, env.provider.createCalls)
}

func TestMediaCreateSameKeyAcrossUsersDoesNotCollide(t *testing.T) {
	env := setupMediaEnv(t)

	require.Equal(t, http.StatusAccepted, env.do(http.MethodPost, "/v1/media/videos", validCreateBody, "shared-key").Code)

	// 换一个用户与 API Key，复用同一幂等键：ActorScope 生效时两者互不干扰。
	otherGroup := &service.Group{ID: 9, Platform: service.PlatformSeedance, RateMultiplier: 1}
	currentAPIKey = &service.APIKey{ID: 200, UserID: 20, GroupID: &otherGroup.ID, Group: otherGroup}

	rec := env.do(http.MethodPost, "/v1/media/videos", validCreateBody, "shared-key")
	require.Equal(t, http.StatusAccepted, rec.Code)
	require.Empty(t, rec.Header().Get("X-Idempotency-Replayed"))
	require.Equal(t, 2, env.provider.createCalls, "不同用户的同名幂等键必须各自建单")
}

func TestMediaTaskLookupIsScopedToOwner(t *testing.T) {
	env := setupMediaEnv(t)
	rec := env.do(http.MethodPost, "/v1/media/videos", validCreateBody, "key-own")
	taskID := taskIDFrom(t, rec)

	// 换用户后查询同一 task_id：不区分「不存在」与「无权」，一律 404。
	otherGroup := &service.Group{ID: 9, Platform: service.PlatformSeedance, RateMultiplier: 1}
	currentAPIKey = &service.APIKey{ID: 200, UserID: 20, GroupID: &otherGroup.ID, Group: otherGroup}

	require.Equal(t, http.StatusNotFound, env.do(http.MethodGet, "/v1/media/videos/"+taskID, "", "").Code)
	require.Equal(t, http.StatusNotFound, env.do(http.MethodGet, "/v1/media/videos/does-not-exist", "", "").Code)
}

func TestMediaTaskStoreFailureReturns503(t *testing.T) {
	env := setupMediaEnv(t)
	rec := env.do(http.MethodPost, "/v1/media/videos", validCreateBody, "key-503")
	taskID := taskIDFrom(t, rec)

	env.store.failGet = true
	require.Equal(t, http.StatusServiceUnavailable, env.do(http.MethodGet, "/v1/media/videos/"+taskID, "", "").Code)
}

func TestMediaCreateFailsClosedWithoutDurableJournal(t *testing.T) {
	env := setupMediaEnv(t)
	env.handler.tasks.store = &nonDurableStore{MediaTaskStore: env.store}

	rec := env.do(http.MethodPost, "/v1/media/videos", validCreateBody, "key-nc")
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Zero(t, env.provider.createCalls, "幂等设施不可用时不得降级为直接建单")
}

func TestMediaSucceededButNotReadyIsNotDownloadable(t *testing.T) {
	env := setupMediaEnv(t)
	rec := env.do(http.MethodPost, "/v1/media/videos", validCreateBody, "key-nr")
	taskID := taskIDFrom(t, rec)

	env.provider.setStatus(env.provider.createdJobs[0], MediaTaskStatusSucceeded)
	env.provider.notReady = true

	rec = env.do(http.MethodGet, "/v1/media/videos/"+taskID, "", "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"status":"succeeded"`)
	require.Contains(t, rec.Body.String(), `"downloadable":false`)

	// 内容接口对未就绪返回 409，客户端据此继续轮询而非当作失败。
	require.Equal(t, http.StatusConflict, env.do(http.MethodGet, "/v1/media/videos/"+taskID+"/content", "", "").Code)
}

func TestMediaUnknownUpstreamStatusFailsClosed(t *testing.T) {
	env := setupMediaEnv(t)
	rec := env.do(http.MethodPost, "/v1/media/videos", validCreateBody, "key-unk")
	taskID := taskIDFrom(t, rec)

	env.provider.setStatus(env.provider.createdJobs[0], "some_new_upstream_state")
	got := env.do(http.MethodGet, "/v1/media/videos/"+taskID, "", "")
	require.Equal(t, http.StatusOK, got.Code)
	require.Contains(t, got.Body.String(), `"downloadable":false`)
	require.Empty(t, env.handler.tasks.Billing().(*lifecycleBilling).captured)
	require.Empty(t, env.handler.tasks.Billing().(*lifecycleBilling).released)
}

func TestMediaDownloadForwardsRange(t *testing.T) {
	env := setupMediaEnv(t)
	rec := env.do(http.MethodPost, "/v1/media/videos", validCreateBody, "key-range")
	taskID := taskIDFrom(t, rec)
	env.provider.setStatus(env.provider.createdJobs[0], MediaTaskStatusSucceeded)

	require.NoError(t, env.handler.advanceTask(context.Background(), taskID, false))
	req := httptest.NewRequest(http.MethodGet, "/v1/media/videos/"+taskID+"/content", nil)
	req.Header.Set("Range", "bytes=0-3")
	out := httptest.NewRecorder()
	env.router.ServeHTTP(out, req)

	require.Equal(t, http.StatusPartialContent, out.Code)
	require.Equal(t, "bytes 0-3/8", out.Header().Get("Content-Range"))
}

func TestMediaReferenceImagePinsAccount(t *testing.T) {
	env := setupMediaEnv(t)

	// 上传时调度到账号 1。
	up := env.do(http.MethodPost, "/v1/media/files", `{"image_b64":"aGVsbG8="}`, "")
	require.Equal(t, http.StatusCreated, up.Code)
	var file struct {
		FileID string `json:"file_id"`
	}
	require.NoError(t, json.Unmarshal(up.Body.Bytes(), &file))
	require.NotEmpty(t, file.FileID)

	// 即使自由调度会选到账号 2，带参考图的创建也必须落在图片所属的账号 1。
	env.resolver.selected = 2
	body := fmt.Sprintf(`{"prompt":"a cat","model":"seedance2.0","duration":5,"file_ids":["%s"]}`, file.FileID)
	rec := env.do(http.MethodPost, "/v1/media/videos", body, "key-pin")
	require.Equal(t, http.StatusAccepted, rec.Code)
	require.Equal(t, int64(1), env.provider.lastAccount, "带参考图的创建必须锁定到图片所属账号")
	require.Equal(t, []string{"up-img-1"}, env.provider.lastImageIDs)
}

func TestMediaCrossAccountReferenceImagesRejected(t *testing.T) {
	env := setupMediaEnv(t)

	// 手工构造两张分属不同账号的参考图。
	for i, acct := range []int64{1, 2} {
		require.NoError(t, env.store.SaveFile(context.Background(), &MediaFileRecord{
			ID:        fmt.Sprintf("f-%d", i),
			UserID:    currentAPIKey.UserID,
			APIKeyID:  currentAPIKey.ID,
			AccountID: acct,
			ImageID:   fmt.Sprintf("img-%d", i),
			ExpiresAt: time.Now().Add(time.Hour).Unix(),
		}, time.Hour))
	}

	body := `{"prompt":"a cat","model":"seedance2.0","duration":5,"file_ids":["f-0","f-1"]}`
	rec := env.do(http.MethodPost, "/v1/media/videos", body, "key-cross")
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Zero(t, env.provider.createCalls)
}

func TestMediaExpiredReferenceImageRejected(t *testing.T) {
	env := setupMediaEnv(t)
	require.NoError(t, env.store.SaveFile(context.Background(), &MediaFileRecord{
		ID:        "f-expired",
		UserID:    currentAPIKey.UserID,
		APIKeyID:  currentAPIKey.ID,
		AccountID: 1,
		ImageID:   "img-old",
		ExpiresAt: time.Now().Add(-time.Minute).Unix(),
	}, time.Hour))

	body := `{"prompt":"a cat","model":"seedance2.0","duration":5,"file_ids":["f-expired"]}`
	require.Equal(t, http.StatusNotFound, env.do(http.MethodPost, "/v1/media/videos", body, "key-exp").Code)
	require.Zero(t, env.provider.createCalls)
}

func TestMediaInvalidModelDurationRejectedBeforeUpstream(t *testing.T) {
	env := setupMediaEnv(t)
	// seedance2.0 不支持 30 秒，必须在调用上游之前拦掉。
	body := `{"prompt":"a cat","model":"seedance2.0","duration":30}`
	require.Equal(t, http.StatusInternalServerError, env.do(http.MethodPost, "/v1/media/videos", body, "key-bad").Code)
	require.Zero(t, env.provider.createCalls)
}

func TestMediaDisabledRejectsCreateButKeepsInflightPollable(t *testing.T) {
	env := setupMediaEnv(t)
	rec := env.do(http.MethodPost, "/v1/media/videos", validCreateBody, "key-off")
	taskID := taskIDFrom(t, rec)

	// 关闭总开关：provider 置空使 Enabled() 为 false，但 store 仍在，Pollable() 为真。
	env.handler.tasks = NewMediaTaskService(env.store, service.PlatformSeedance, nil)

	require.Equal(t, http.StatusNotFound, env.do(http.MethodPost, "/v1/media/videos", validCreateBody, "key-off2").Code)
	require.Equal(t, http.StatusNotFound, env.do(http.MethodPost, "/v1/media/files", `{"image_b64":"aGVsbG8="}`, "").Code)

	// 在途任务仍可查询：已产生成本的任务不应被搁死。
	got := env.do(http.MethodGet, "/v1/media/videos/"+taskID, "", "")
	require.NotEqual(t, http.StatusNotFound, got.Code)
}

func TestMediaDefinitiveCreateRejectionRefundsAndRetainsTask(t *testing.T) {
	env := setupMediaEnv(t)
	env.provider.createErr = &SubmissionRejected{Cause: ErrSeedanceInsufficientBalance}

	rec := env.do(http.MethodPost, "/v1/media/videos", validCreateBody, "key-fail")
	require.Equal(t, http.StatusAccepted, rec.Code)
	id := taskIDFrom(t, rec)
	task, err := env.store.GetTask(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, mediaPhaseReleased, task.Phase)
	require.Equal(t, 100.0, env.handler.tasks.Billing().(*lifecycleBilling).balance)
}

func TestMediaTaskRecordWriteFailureStillReturns202(t *testing.T) {
	env := setupMediaEnv(t)
	// 上游建单成功后写记录失败：闭包绝不能返回 error，否则 coordinator 会在
	// backoff 后重新执行并产生第二个上游任务。
	env.handler.tasks.store = &failingSaveStore{memMediaStore: env.store}

	rec := env.do(http.MethodPost, "/v1/media/videos", validCreateBody, "key-wf")
	require.Equal(t, http.StatusAccepted, rec.Code)
	require.Equal(t, 1, env.provider.createCalls)

	// 同 Key 重试仍走重放，不产生第二个上游任务。
	again := env.do(http.MethodPost, "/v1/media/videos", validCreateBody, "key-wf")
	require.Equal(t, http.StatusAccepted, again.Code)
	require.Equal(t, 1, env.provider.createCalls)
}

type nonDurableStore struct{ MediaTaskStore }

type failingSaveStore struct {
	*memMediaStore
	failed bool
}

func (s *failingSaveStore) SaveTask(ctx context.Context, t *MediaTaskRecord, ttl time.Duration) error {
	if t.Phase == mediaPhaseSubmitted && !s.failed {
		s.failed = true
		return errors.New("database write failed after upstream accepted")
	}
	return s.memMediaStore.SaveTask(ctx, t, ttl)
}

func TestMediaModelsRendersCapabilities(t *testing.T) {
	env := setupMediaEnv(t)
	rec := env.do(http.MethodGet, "/v1/media/models", "", "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"platform":"seedance"`)
	require.Contains(t, rec.Body.String(), `"seedance2.0"`)
}

func TestMediaRejectsNonSeedancePlatformGroup(t *testing.T) {
	env := setupMediaEnv(t)
	grokGroup := &service.Group{ID: 9, Platform: service.PlatformGrok}
	currentAPIKey = &service.APIKey{ID: 300, UserID: 30, GroupID: &grokGroup.ID, Group: grokGroup}

	require.Equal(t, http.StatusNotFound, env.do(http.MethodPost, "/v1/media/videos", validCreateBody, "key-grok").Code)
	require.Zero(t, env.provider.createCalls)
}
