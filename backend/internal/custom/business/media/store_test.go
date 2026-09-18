package media

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func journalFixture() *MediaTaskRecord {
	return &MediaTaskRecord{ID: "media-task-1", UserID: 7, APIKeyID: 11, AccountID: 23,
		RequestHash: "request-sha256", UnitPrice: 0.5, Phase: "prepared", CreatedAt: 100}
}

func newJournalMock(t *testing.T) (*mediaTaskStore, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, mock.ExpectationsWereMet()); _ = db.Close() })
	return NewMediaTaskStore(db).(*mediaTaskStore), mock
}

func journalJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return string(data)
}

func expectJournalRead(mock sqlmock.Sqlmock, id string, data string) {
	rows := sqlmock.NewRows([]string{"record"})
	if data != "" {
		rows.AddRow(data)
	}
	mock.ExpectQuery(`SELECT record FROM subnexus_seedance_tasks WHERE task_id = \$1`).WithArgs(id).WillReturnRows(rows)
}

func expectJournalNewKeyLock(mock sqlmock.Sqlmock, task *MediaTaskRecord) {
	expectJournalRead(mock, task.ID, "")
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM api_keys.*FOR UPDATE`).WithArgs(task.APIKeyID, task.UserID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(task.APIKeyID))
	expectJournalRead(mock, task.ID, "")
}

func journalLimitRows(quotaExceeded, rateLimited bool, status string, expired, quotaEnabled, rateEnabled bool) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"quota_exceeded", "rate_limited", "status", "expired", "quota_enabled", "rate_enabled"}).
		AddRow(quotaExceeded, rateLimited, status, expired, quotaEnabled, rateEnabled)
}

func TestJournalCreateTaskPersistsBeforeReturning(t *testing.T) {
	s, mock := newJournalMock(t)
	task := journalFixture()
	expectJournalNewKeyLock(mock, task)
	mock.ExpectQuery(`SELECT quota > 0 AND quota_used.*SUM\(unit_price\)`).WithArgs(task.APIKeyID, task.UnitPrice).
		WillReturnRows(journalLimitRows(false, false, "active", false, false, false))
	mock.ExpectQuery(`INSERT INTO subnexus_seedance_tasks.*ON CONFLICT \(task_id\) DO NOTHING RETURNING record`).
		WithArgs(task.ID, task.UserID, task.APIKeyID, task.RequestHash, task.UnitPrice, task.Phase, task.NextAttemptAt, journalJSON(t, task)).
		WillReturnRows(sqlmock.NewRows([]string{"record"}).AddRow(journalJSON(t, task)))
	mock.ExpectCommit()
	stored, created, err := s.CreateTask(context.Background(), task)
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, task, stored)
}

func TestJournalCreateTaskReplayPreservesOriginalPriceAndSkipsQuota(t *testing.T) {
	s, mock := newJournalMock(t)
	task := journalFixture()
	task.Phase = "settled"
	expectJournalRead(mock, task.ID, journalJSON(t, task))
	request := *task
	request.UnitPrice = 100
	request.Phase = "prepared"
	stored, created, err := s.CreateTask(context.Background(), &request)
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, 0.5, stored.UnitPrice)
	require.Equal(t, "settled", stored.Phase)
}

func TestJournalCreateTaskConflictingReplay(t *testing.T) {
	for _, mutate := range []func(*MediaTaskRecord){
		func(r *MediaTaskRecord) { r.UserID++ },
		func(r *MediaTaskRecord) { r.APIKeyID++ },
		func(r *MediaTaskRecord) { r.RequestHash = "different-request" },
	} {
		s, mock := newJournalMock(t)
		task := journalFixture()
		expectJournalRead(mock, task.ID, journalJSON(t, task))
		request := *task
		mutate(&request)
		stored, created, err := s.CreateTask(context.Background(), &request)
		require.ErrorIs(t, err, ErrMediaIdempotencyConflict)
		require.False(t, created)
		require.Nil(t, stored)
	}
}

func TestJournalCreateTaskRejectsPendingQuotaWithoutInsertion(t *testing.T) {
	s, mock := newJournalMock(t)
	task := journalFixture()
	expectJournalNewKeyLock(mock, task)
	mock.ExpectQuery(`SELECT quota > 0 AND quota_used.*SUM\(unit_price\)`).WithArgs(task.APIKeyID, task.UnitPrice).
		WillReturnRows(journalLimitRows(true, false, "active", false, true, false))
	mock.ExpectRollback()
	_, created, err := s.CreateTask(context.Background(), task)
	require.ErrorIs(t, err, ErrMediaKeyQuotaExceeded)
	require.False(t, created)
}

func TestJournalCreateTaskRejectsFreshRateLimitOrDisabledKey(t *testing.T) {
	for _, tc := range []struct {
		name        string
		status      string
		expired     bool
		rateLimited bool
		want        error
	}{
		{"rate limited", "active", false, true, ErrMediaKeyRateLimited},
		{"disabled", "disabled", false, false, ErrMediaKeyUnavailable},
		{"expired", "active", true, false, ErrMediaKeyUnavailable},
		{"exhausted", "quota_exhausted", false, false, ErrMediaKeyQuotaExceeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, mock := newJournalMock(t)
			task := journalFixture()
			expectJournalNewKeyLock(mock, task)
			mock.ExpectQuery(`SELECT quota > 0 AND quota_used`).WithArgs(task.APIKeyID, task.UnitPrice).
				WillReturnRows(journalLimitRows(false, tc.rateLimited, tc.status, tc.expired, false, true))
			mock.ExpectRollback()
			_, created, err := s.CreateTask(context.Background(), task)
			require.ErrorIs(t, err, tc.want)
			require.False(t, created)
		})
	}
}

func TestJournalCreateTaskSnapshotsFreshLimitFlags(t *testing.T) {
	s, mock := newJournalMock(t)
	task := journalFixture()
	expectJournalNewKeyLock(mock, task)
	mock.ExpectQuery(`SELECT quota > 0 AND quota_used`).WithArgs(task.APIKeyID, task.UnitPrice).
		WillReturnRows(journalLimitRows(false, false, "active", false, true, true))
	expected := *task
	expected.BillingQuota, expected.BillingRateLimits = true, true
	mock.ExpectQuery(`INSERT INTO subnexus_seedance_tasks`).
		WithArgs(task.ID, task.UserID, task.APIKeyID, task.RequestHash, task.UnitPrice, task.Phase, task.NextAttemptAt, journalJSON(t, &expected)).
		WillReturnRows(sqlmock.NewRows([]string{"record"}).AddRow(journalJSON(t, &expected)))
	mock.ExpectCommit()
	stored, created, err := s.CreateTask(context.Background(), task)
	require.NoError(t, err)
	require.True(t, created)
	require.True(t, stored.BillingQuota)
	require.True(t, stored.BillingRateLimits)
}

func TestJournalCreateTaskRechecksReplayAfterKeyLock(t *testing.T) {
	s, mock := newJournalMock(t)
	task := journalFixture()
	expectJournalRead(mock, task.ID, "")
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM api_keys.*FOR UPDATE`).WithArgs(task.APIKeyID, task.UserID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(task.APIKeyID))
	expectJournalRead(mock, task.ID, journalJSON(t, task))
	mock.ExpectRollback()
	stored, created, err := s.CreateTask(context.Background(), task)
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, task, stored)
}

func TestJournalCreateTaskCommitFailureIsNotSuccess(t *testing.T) {
	s, mock := newJournalMock(t)
	task := journalFixture()
	expectJournalNewKeyLock(mock, task)
	mock.ExpectQuery(`SELECT quota > 0 AND quota_used`).WillReturnRows(journalLimitRows(false, false, "active", false, false, false))
	mock.ExpectQuery(`INSERT INTO subnexus_seedance_tasks`).WillReturnRows(sqlmock.NewRows([]string{"record"}).AddRow(journalJSON(t, task)))
	mock.ExpectCommit().WillReturnError(errors.New("connection lost during commit"))
	_, created, err := s.CreateTask(context.Background(), task)
	require.Error(t, err)
	require.False(t, created)
}

func TestJournalSaveTaskFencesExpiredOrReplacedLease(t *testing.T) {
	for _, token := range []string{"", "worker-1"} {
		s, mock := newJournalMock(t)
		task := journalFixture()
		ctx := WithMediaLeaseContext(context.Background(), token)
		mock.ExpectExec(`UPDATE subnexus_seedance_tasks.*lease_token = \$8 AND lease_until > clock_timestamp\(\)`).
			WithArgs(task.ID, journalJSON(t, task), task.Phase, task.NextAttemptAt, task.UserID, task.APIKeyID, task.RequestHash, token, task.UnitPrice).
			WillReturnResult(sqlmock.NewResult(0, 0))
		require.ErrorIs(t, s.SaveTask(ctx, task, time.Second), ErrMediaLeaseLost)
	}
}

func TestJournalLeaseReleaseCannotClearAnotherWorker(t *testing.T) {
	s, mock := newJournalMock(t)
	mock.ExpectExec(`UPDATE subnexus_seedance_tasks SET lease_token = '', lease_until = NULL WHERE task_id = \$1 AND lease_token = \$2`).
		WithArgs("task-1", "stale-token").WillReturnResult(sqlmock.NewResult(0, 0))
	require.NoError(t, s.ReleaseTask(context.Background(), "task-1", "stale-token"))
}

func TestJournalAcquireTaskChecksExpiredLease(t *testing.T) {
	s, mock := newJournalMock(t)
	mock.ExpectExec(`UPDATE subnexus_seedance_tasks.*lease_until IS NULL OR lease_until <= clock_timestamp\(\)`).
		WithArgs("task-1", "token-1", int64(1500)).WillReturnResult(sqlmock.NewResult(0, 0))
	ok, err := s.AcquireTask(context.Background(), "task-1", "token-1", 1500*time.Millisecond)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestJournalGetTaskMissingAndCorruptedAreDistinct(t *testing.T) {
	s, mock := newJournalMock(t)
	expectJournalRead(mock, "absent", "")
	_, err := s.GetTask(context.Background(), "absent")
	require.ErrorIs(t, err, ErrMediaTaskNotFound)
	expectJournalRead(mock, "corrupt", "not-json")
	_, err = s.GetTask(context.Background(), "corrupt")
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrMediaTaskNotFound)
}

func TestJournalListRecoveryExcludesTerminalAndLeasedTasks(t *testing.T) {
	s, mock := newJournalMock(t)
	task := journalFixture()
	task.Phase = "submission_unknown"
	mock.ExpectQuery(`SELECT record FROM subnexus_seedance_tasks WHERE phase NOT IN \('settled', 'released', 'rejected', 'manual_review'\).*lease_until <= clock_timestamp\(\).*ORDER BY next_attempt_at, created_at, task_id LIMIT \$1`).
		WithArgs(100).WillReturnRows(sqlmock.NewRows([]string{"record"}).AddRow(journalJSON(t, task)))
	tasks, err := s.ListRecoverableTasks(context.Background(), 0)
	require.NoError(t, err)
	require.Equal(t, []*MediaTaskRecord{task}, tasks)
}

func TestJournalPendingHoldNoTTLAndNoSilentDecodeLoss(t *testing.T) {
	s, mock := newJournalMock(t)
	hold := &MediaPendingHold{TaskID: "task-1", UserID: 7, APIKeyID: 11, Amount: 0.5, DueAt: 100}
	mock.ExpectExec(`INSERT INTO subnexus_seedance_pending_holds.*amount = EXCLUDED.amount`).
		WithArgs(hold.TaskID, hold.UserID, hold.APIKeyID, hold.Amount, hold.DueAt, journalJSON(t, hold)).WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, s.TrackPendingHold(context.Background(), hold, -time.Hour))
	mock.ExpectQuery(`SELECT record FROM subnexus_seedance_pending_holds WHERE due_at <= \$1 ORDER BY due_at, task_id LIMIT \$2`).
		WithArgs(int64(999), 10).WillReturnRows(sqlmock.NewRows([]string{"record"}).AddRow("corrupt"))
	_, err := s.ListDuePendingHolds(context.Background(), time.Unix(999, 0), 10)
	require.Error(t, err)
}

func TestJournalDatabaseFailureDoesNotBecomeNotFound(t *testing.T) {
	s, mock := newJournalMock(t)
	mock.ExpectQuery(`SELECT record FROM subnexus_seedance_tasks`).WillReturnError(sql.ErrConnDone)
	_, _, err := s.CreateTask(context.Background(), journalFixture())
	require.ErrorIs(t, err, sql.ErrConnDone)
	require.NotErrorIs(t, err, ErrMediaTaskNotFound)
}

func TestJournalCleanupOnlyExpiredReferenceFilesAndBoundsBatch(t *testing.T) {
	s, mock := newJournalMock(t)
	mock.ExpectExec(`WITH expired AS \( SELECT file_id FROM subnexus_seedance_files WHERE expires_at <= clock_timestamp\(\).*LIMIT \$1 FOR UPDATE SKIP LOCKED \) DELETE FROM subnexus_seedance_files`).
		WithArgs(1000).WillReturnResult(sqlmock.NewResult(0, 4))
	count, err := s.CleanupExpiredFiles(context.Background(), 50000)
	require.NoError(t, err)
	require.Equal(t, int64(4), count)
}
