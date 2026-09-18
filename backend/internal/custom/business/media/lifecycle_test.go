package media

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// Embed the unused cache methods; exercised account admission is explicit.
type mediaConcurrencyFake struct {
	service.ConcurrencyCache
	deny bool
}

func (f *mediaConcurrencyFake) AcquireAccountSlot(context.Context, int64, int, string) (bool, error) {
	return !f.deny, nil
}
func (f *mediaConcurrencyFake) ReleaseAccountSlot(context.Context, int64, string) error { return nil }

type lifecycleBilling struct {
	mu                           sync.Mutex
	balance, frozen, charged     float64
	reserved, captured, released map[string]bool
	captureErr, releaseErr       error
}

func newLifecycleBilling() *lifecycleBilling {
	return &lifecycleBilling{balance: 100, reserved: map[string]bool{}, captured: map[string]bool{}, released: map[string]bool{}}
}
func (b *lifecycleBilling) Quote(context.Context, string, *APIKey) (float64, error) { return 2, nil }
func (b *lifecycleBilling) Reserve(_ context.Context, t *MediaTaskRecord, _ *APIKey, _ *User, _ *Account) (float64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.reserved[t.ID] {
		b.balance -= t.UnitPrice
		b.frozen += t.UnitPrice
		b.reserved[t.ID] = true
	}
	return t.UnitPrice, nil
}
func (b *lifecycleBilling) Capture(_ context.Context, t *MediaTaskRecord, _ *APIKey, _ *User, _ *Account) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.captureErr != nil {
		return b.captureErr
	}
	if !b.captured[t.ID] {
		b.frozen -= t.UnitPrice
		b.charged += t.UnitPrice
		b.captured[t.ID] = true
	}
	return nil
}
func (b *lifecycleBilling) Release(_ context.Context, t *MediaTaskRecord, _ *APIKey) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.releaseErr != nil {
		return b.releaseErr
	}
	if b.reserved[t.ID] && !b.released[t.ID] {
		b.balance += t.UnitPrice
		b.frozen -= t.UnitPrice
		b.released[t.ID] = true
	}
	return nil
}

func (s *memMediaStore) CreateTask(_ context.Context, t *MediaTaskRecord) (*MediaTaskRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if old := s.tasks[t.ID]; old != nil {
		got, _, err := compareJournalTask(old, t)
		return got, false, err
	}
	cp := *t
	s.tasks[t.ID] = &cp
	return &cp, true, nil
}
func (s *memMediaStore) AcquireTask(_ context.Context, id, token string, _ time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.leases == nil {
		s.leases = map[string]string{}
	}
	if s.leases[id] != "" {
		return false, nil
	}
	s.leases[id] = token
	return true, nil
}
func (s *memMediaStore) ReleaseTask(_ context.Context, id, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.leases[id] == token {
		delete(s.leases, id)
	}
	return nil
}
func (s *memMediaStore) ListRecoverableTasks(_ context.Context, _ int) ([]*MediaTaskRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*MediaTaskRecord
	for _, t := range s.tasks {
		if !mediaFinalPhase(t.Phase) && t.NextAttemptAt <= time.Now().Unix() {
			cp := *t
			out = append(out, &cp)
		}
	}
	return out, nil
}

func TestMediaUnknownSubmissionDoesNotRefundOrSwitchAccount(t *testing.T) {
	e := setupMediaEnv(t)
	e.provider.createErr = errors.New("connection lost after submit")
	r := e.do(http.MethodPost, "/v1/media/videos", validCreateBody, "unknown")
	require.Equal(t, 202, r.Code)
	id := taskIDFrom(t, r)
	b := e.handler.tasks.Billing().(*lifecycleBilling)
	require.Equal(t, 98.0, b.balance)
	require.Equal(t, 2.0, b.frozen)
	require.Empty(t, b.released)
	task, _ := e.store.GetTask(context.Background(), id)
	require.Equal(t, mediaPhaseUnknown, task.Phase)
	e.resolver.selected = 2
	for i := 0; i < 4; i++ {
		_ = e.handler.advanceTask(context.Background(), id, false)
	}
	task, _ = e.store.GetTask(context.Background(), id)
	require.Equal(t, mediaPhaseReview, task.Phase)
	require.EqualValues(t, 1, task.AccountID)
	require.Empty(t, b.released)
	require.Empty(t, b.captured)
}

func TestMediaSettlementFailureBlocksDownloadAndRecovers(t *testing.T) {
	e := setupMediaEnv(t)
	r := e.do("POST", "/v1/media/videos", validCreateBody, "capture")
	id := taskIDFrom(t, r)
	task, _ := e.store.GetTask(context.Background(), id)
	e.provider.setStatus(task.JobID, MediaTaskStatusSucceeded)
	b := e.handler.tasks.Billing().(*lifecycleBilling)
	b.captureErr = errors.New("database unavailable")
	r = e.do("GET", "/v1/media/videos/"+id+"/content", "", "")
	require.Equal(t, 409, r.Code)
	require.NotContains(t, r.Body.String(), "MP4DATA")
	task, _ = e.store.GetTask(context.Background(), id)
	require.Equal(t, mediaPhaseCapturing, task.Phase)
	b.captureErr = nil
	r = e.do("GET", "/v1/media/videos/"+id+"/content", "", "")
	require.Equal(t, 200, r.Code)
	require.Equal(t, "MP4DATA", r.Body.String())
	r = e.do("GET", "/v1/media/videos/"+id+"/content", "", "")
	require.Equal(t, 200, r.Code)
	require.Equal(t, 2.0, b.charged)
	require.Zero(t, b.frozen)
	require.Empty(t, b.released)
}

func TestMediaFailedTaskRefundRetriesAndNeverCaptures(t *testing.T) {
	e := setupMediaEnv(t)
	r := e.do("POST", "/v1/media/videos", validCreateBody, "refund")
	id := taskIDFrom(t, r)
	task, _ := e.store.GetTask(context.Background(), id)
	e.provider.setStatus(task.JobID, MediaTaskStatusFailed)
	b := e.handler.tasks.Billing().(*lifecycleBilling)
	b.releaseErr = errors.New("database unavailable")
	_ = e.handler.advanceTask(context.Background(), id, false)
	task, _ = e.store.GetTask(context.Background(), id)
	require.Equal(t, mediaPhaseReleasing, task.Phase)
	// Once refund intent is saved, a contradictory upstream status cannot charge.
	e.provider.setStatus(task.JobID, MediaTaskStatusSucceeded)
	b.releaseErr = nil
	require.NoError(t, e.handler.advanceTask(context.Background(), id, false))
	require.NoError(t, e.handler.advanceTask(context.Background(), id, false))
	require.Equal(t, 100.0, b.balance)
	require.Zero(t, b.frozen)
	require.Empty(t, b.captured)
}

func TestMediaExpiredAgeDoesNotRefundRunningTask(t *testing.T) {
	e := setupMediaEnv(t)
	r := e.do("POST", "/v1/media/videos", validCreateBody, "long")
	id := taskIDFrom(t, r)
	task, _ := e.store.GetTask(context.Background(), id)
	task.CreatedAt = time.Now().Add(-72 * time.Hour).Unix()
	task.NextAttemptAt = 0
	require.NoError(t, e.store.SaveTask(context.Background(), task, time.Hour))
	w := NewMediaHoldSweeper(e.handler)
	_, failed := w.SweepOnce(context.Background())
	require.Zero(t, failed)
	b := e.handler.tasks.Billing().(*lifecycleBilling)
	require.Empty(t, b.released)
	require.Equal(t, 2.0, b.frozen)
}

func TestMediaFeatureOffStillSettlesAcceptedTasks(t *testing.T) {
	e := setupMediaEnv(t)
	r := e.do("POST", "/v1/media/videos", validCreateBody, "disable")
	id := taskIDFrom(t, r)
	task, _ := e.store.GetTask(context.Background(), id)
	e.provider.setStatus(task.JobID, MediaTaskStatusSucceeded)
	e.handler.tasks.WithEnabled(false)
	r = e.do("POST", "/v1/media/videos", validCreateBody, "new")
	require.Equal(t, 404, r.Code)
	r = e.do("GET", "/v1/media/videos/"+id, "", "")
	require.Equal(t, 200, r.Code)
	require.Contains(t, r.Body.String(), `"downloadable":true`)
}

func TestMediaAccountCredentialChangeStopsRecovery(t *testing.T) {
	e := setupMediaEnv(t)
	r := e.do("POST", "/v1/media/videos", validCreateBody, "changed")
	id := taskIDFrom(t, r)
	e.resolver.accounts[1].Credentials = map[string]any{"api_key": "replacement"}
	require.Error(t, e.handler.advanceTask(context.Background(), id, false))
	b := e.handler.tasks.Billing().(*lifecycleBilling)
	require.Empty(t, b.captured)
	require.Empty(t, b.released)
}
