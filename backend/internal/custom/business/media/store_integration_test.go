//go:build integration

package media

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// Tests create a unique scratch schema and never use an existing application's
// tables. Set SEEDANCE_TEST_DATABASE_URL to a disposable local PostgreSQL DB.
func newJournalPostgres(t *testing.T) (*mediaTaskStore, *sql.DB) {
	t.Helper()
	dsn := os.Getenv("SEEDANCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SEEDANCE_TEST_DATABASE_URL is not set")
	}
	u, err := url.Parse(dsn)
	require.NoError(t, err)
	if u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost" && u.Hostname() != "::1" {
		t.Fatal("Seedance integration tests require a local disposable database")
	}
	admin, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	schema := "seedance_test_" + uuid.New().String()[:8]
	_, err = admin.Exec(`CREATE SCHEMA ` + schema)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := admin.Exec(`DROP SCHEMA ` + schema + ` CASCADE`)
		require.NoError(t, err)
		_ = admin.Close()
	})
	query := u.Query()
	query.Set("search_path", schema)
	u.RawQuery = query.Encode()
	db, err := sql.Open("postgres", u.String())
	require.NoError(t, err)
	db.SetMaxOpenConns(16)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`CREATE TABLE api_keys (id BIGINT PRIMARY KEY, user_id BIGINT NOT NULL,
		quota NUMERIC(20,8) NOT NULL DEFAULT 0, quota_used NUMERIC(20,8) NOT NULL DEFAULT 0,
		rate_limit_5h NUMERIC(20,8) NOT NULL DEFAULT 0, rate_limit_1d NUMERIC(20,8) NOT NULL DEFAULT 0, rate_limit_7d NUMERIC(20,8) NOT NULL DEFAULT 0,
		usage_5h NUMERIC(20,8) NOT NULL DEFAULT 0, usage_1d NUMERIC(20,8) NOT NULL DEFAULT 0, usage_7d NUMERIC(20,8) NOT NULL DEFAULT 0,
		window_5h_start TIMESTAMPTZ, window_1d_start TIMESTAMPTZ, window_7d_start TIMESTAMPTZ,
		status TEXT NOT NULL DEFAULT 'active', expires_at TIMESTAMPTZ, deleted_at TIMESTAMPTZ)`)
	require.NoError(t, err)
	migration, err := migrations.FS.ReadFile("9017_subnexus_seedance_media.sql")
	require.NoError(t, err)
	_, err = db.Exec(string(migration))
	require.NoError(t, err)
	// The migration must be safe to apply twice and cannot depend on existing
	// user, balance, usage, order, or subscription tables.
	_, err = db.Exec(string(migration))
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO api_keys (id,user_id,quota) VALUES (11,7,0)`)
	require.NoError(t, err)
	return NewMediaTaskStore(db).(*mediaTaskStore), db
}

func TestJournalPostgresConcurrentCreateAndQuotaReservation(t *testing.T) {
	s, db := newJournalPostgres(t)
	ctx := context.Background()
	t.Run("same key creates exactly once", func(t *testing.T) {
		var createdCount atomic.Int32
		var wg sync.WaitGroup
		errs := make(chan error, 12)
		for i := 0; i < 12; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, created, err := s.CreateTask(ctx, journalFixture())
				if created {
					createdCount.Add(1)
				}
				errs <- err
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			require.NoError(t, err)
		}
		require.Equal(t, int32(1), createdCount.Load())
	})
	t.Run("concurrent pending tasks cannot overspend key quota", func(t *testing.T) {
		_, err := db.Exec(`INSERT INTO api_keys (id,user_id,quota) VALUES (12,7,1)`)
		require.NoError(t, err)
		var accepted atomic.Int32
		var wg sync.WaitGroup
		errs := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				task := journalFixture()
				task.ID, task.APIKeyID, task.UnitPrice = fmt.Sprintf("quota-%d", i), 12, 0.6
				_, created, err := s.CreateTask(ctx, task)
				if created {
					accepted.Add(1)
				}
				errs <- err
			}(i)
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				require.ErrorIs(t, err, ErrMediaKeyQuotaExceeded)
			}
		}
		require.Equal(t, int32(1), accepted.Load())
		var quotaUsed float64
		require.NoError(t, db.QueryRow(`SELECT quota_used FROM api_keys WHERE id=12`).Scan(&quotaUsed))
		require.Zero(t, quotaUsed, "journal reservation must not mutate existing key billing counters")
	})
}

func TestJournalPostgresLeaseFencesStaleWorker(t *testing.T) {
	s, db := newJournalPostgres(t)
	ctx := context.Background()
	task, _, err := s.CreateTask(ctx, journalFixture())
	require.NoError(t, err)
	ok, err := s.AcquireTask(ctx, task.ID, "worker-1", time.Minute)
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = s.AcquireTask(ctx, task.ID, "worker-2", time.Minute)
	require.NoError(t, err)
	require.False(t, ok)
	task.Phase = "submitting"
	require.NoError(t, s.SaveTask(WithMediaLeaseContext(ctx, "worker-1"), task, time.Nanosecond))
	_, err = db.Exec(`UPDATE subnexus_seedance_tasks SET lease_until = clock_timestamp() - interval '1 second' WHERE task_id=$1`, task.ID)
	require.NoError(t, err)
	ok, err = s.AcquireTask(ctx, task.ID, "worker-2", time.Minute)
	require.NoError(t, err)
	require.True(t, ok)
	task.Phase = "released"
	require.ErrorIs(t, s.SaveTask(WithMediaLeaseContext(ctx, "worker-1"), task, time.Hour), ErrMediaLeaseLost)
	require.ErrorIs(t, s.SaveTask(ctx, task, time.Hour), ErrMediaLeaseLost)
	require.NoError(t, s.ReleaseTask(ctx, task.ID, "worker-1"))
	var leaseToken string
	require.NoError(t, db.QueryRow(`SELECT lease_token FROM subnexus_seedance_tasks WHERE task_id=$1`, task.ID).Scan(&leaseToken))
	require.Equal(t, "worker-2", leaseToken)
	task.Phase = "settled"
	require.NoError(t, s.SaveTask(WithMediaLeaseContext(ctx, "worker-2"), task, time.Hour))
	task.Phase = "prepared"
	require.ErrorIs(t, s.SaveTask(WithMediaLeaseContext(ctx, "worker-2"), task, time.Hour), ErrMediaLeaseLost)
	require.NoError(t, s.ReleaseTask(ctx, task.ID, "worker-2"))
	_, err = db.Exec(`UPDATE subnexus_seedance_tasks SET created_at=clock_timestamp()-interval '2 years' WHERE task_id=$1`, task.ID)
	require.NoError(t, err)
	stored, created, err := s.CreateTask(ctx, journalFixture())
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, "settled", stored.Phase, "terminal tombstones must survive task TTL")
}

func TestJournalPostgresRecoveryAndManualReviewQuota(t *testing.T) {
	s, db := newJournalPostgres(t)
	ctx := context.Background()
	for _, phase := range []string{"prepared", "reserving", "reserved", "submitting", "submission_unknown", "submitted", "capturing", "releasing", "manual_review", "settled", "released", "rejected"} {
		task := journalFixture()
		task.ID, task.Phase = phase, phase
		_, _, err := s.CreateTask(ctx, task)
		require.NoError(t, err)
	}
	tasks, err := s.ListRecoverableTasks(ctx, 100)
	require.NoError(t, err)
	require.Len(t, tasks, 8)
	for _, task := range tasks {
		require.NotContains(t, []string{"manual_review", "settled", "released", "rejected"}, task.Phase)
	}
	_, err = db.Exec(`INSERT INTO api_keys (id,user_id,quota) VALUES (12,7,1)`)
	require.NoError(t, err)
	task := journalFixture()
	task.ID, task.APIKeyID, task.UnitPrice, task.Phase = "manual-review-hold", 12, 0.7, "manual_review"
	_, _, err = s.CreateTask(ctx, task)
	require.NoError(t, err)
	task.ID, task.Phase = "cannot-ignore-manual-review", "prepared"
	_, _, err = s.CreateTask(ctx, task)
	require.ErrorIs(t, err, ErrMediaKeyQuotaExceeded)
}

func TestJournalPostgresPendingFundsNeverExpireAndReferencesDo(t *testing.T) {
	s, _ := newJournalPostgres(t)
	ctx := context.Background()
	hold := &MediaPendingHold{TaskID: "old-pending", UserID: 7, APIKeyID: 11, Amount: 0.5, DueAt: time.Now().Add(-365 * 24 * time.Hour).Unix()}
	require.NoError(t, s.TrackPendingHold(ctx, hold, -time.Hour))
	holds, err := s.ListDuePendingHolds(ctx, time.Now(), 100)
	require.NoError(t, err)
	require.Equal(t, []*MediaPendingHold{hold}, holds)
	conflicting := *hold
	conflicting.Amount = 0.6
	require.ErrorIs(t, s.TrackPendingHold(ctx, &conflicting, time.Hour), ErrMediaIdempotencyConflict)
	require.NoError(t, s.DeletePendingHold(ctx, hold.TaskID))
	holds, err = s.ListDuePendingHolds(ctx, time.Now(), 100)
	require.NoError(t, err)
	require.Empty(t, holds)
	file := &MediaFileRecord{ID: "expired-reference", UserID: 7, APIKeyID: 11, AccountID: 23, ImageID: "private-image", ExpiresAt: time.Now().Add(-time.Hour).Unix()}
	require.NoError(t, s.SaveFile(ctx, file, time.Hour))
	_, err = s.GetFile(ctx, file.ID)
	require.ErrorIs(t, err, ErrMediaTaskNotFound)
	file.ID, file.ExpiresAt = "live-reference", time.Now().Add(time.Hour).Unix()
	require.NoError(t, s.SaveFile(ctx, file, time.Hour))
	stored, err := s.GetFile(ctx, file.ID)
	require.NoError(t, err)
	require.Equal(t, file, stored)
	file.AccountID++
	require.Error(t, s.SaveFile(ctx, file, time.Hour), "duplicate reference cannot rebind the upstream account")
	ok, err := s.ClaimSettlement(ctx, "task-1", -time.Hour)
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = s.ClaimSettlement(ctx, "task-1", -time.Hour)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestJournalPostgresRateWindowsReservePendingTasks(t *testing.T) {
	for _, window := range []struct{ suffix, duration string }{{"5h", "5 hours"}, {"1d", "24 hours"}, {"7d", "7 days"}} {
		t.Run(window.suffix, func(t *testing.T) {
			s, db := newJournalPostgres(t)
			ctx := context.Background()
			// A current window's prior usage plus the new reservation exceeds cap.
			_, err := db.Exec(fmt.Sprintf(`UPDATE api_keys SET rate_limit_%s=1, usage_%s=0.5, window_%s_start=clock_timestamp() WHERE id=11`, window.suffix, window.suffix, window.suffix))
			require.NoError(t, err)
			task := journalFixture()
			task.UnitPrice = 0.6
			_, _, err = s.CreateTask(ctx, task)
			require.ErrorIs(t, err, ErrMediaKeyRateLimited)
			// The same stored usage does not count once its window has expired.
			_, err = db.Exec(fmt.Sprintf(`UPDATE api_keys SET window_%s_start=clock_timestamp()-interval '%s'-interval '1 second' WHERE id=11`, window.suffix, window.duration))
			require.NoError(t, err)
			stored, created, err := s.CreateTask(ctx, task)
			require.NoError(t, err)
			require.True(t, created)
			require.True(t, stored.BillingRateLimits)
			// Old/ambiguous pending work must remain reserved across window resets.
			stored.Phase = "manual_review"
			stored.CreatedAt = time.Now().Add(-365 * 24 * time.Hour).Unix()
			require.NoError(t, s.SaveTask(ctx, stored, time.Nanosecond))
			task.ID = "next-task"
			_, _, err = s.CreateTask(ctx, task)
			require.ErrorIs(t, err, ErrMediaKeyRateLimited)
			// A confirmed release frees the reservation; SQL window fields are not
			// reset or charged by admission itself.
			stored.Phase = "released"
			require.NoError(t, s.SaveTask(ctx, stored, time.Hour))
			_, _, err = s.CreateTask(ctx, task)
			require.NoError(t, err)
			var usage float64
			require.NoError(t, db.QueryRow(fmt.Sprintf(`SELECT usage_%s FROM api_keys WHERE id=11`, window.suffix)).Scan(&usage))
			require.Equal(t, 0.5, usage)
		})
	}
}

func TestJournalPostgresConcurrentRateLimitAndNullWindow(t *testing.T) {
	s, db := newJournalPostgres(t)
	_, err := db.Exec(`UPDATE api_keys SET rate_limit_5h=1, usage_5h=100, window_5h_start=NULL WHERE id=11`)
	require.NoError(t, err)
	var accepted atomic.Int32
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			task := journalFixture()
			task.ID, task.UnitPrice = fmt.Sprintf("window-%d", i), 0.6
			_, created, err := s.CreateTask(context.Background(), task)
			if created {
				accepted.Add(1)
			}
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			require.ErrorIs(t, err, ErrMediaKeyRateLimited)
		}
	}
	require.Equal(t, int32(1), accepted.Load())
}

func TestJournalPostgresFreshAdmissionDoesNotBlockReplay(t *testing.T) {
	s, db := newJournalPostgres(t)
	ctx := context.Background()
	_, _, err := s.CreateTask(ctx, journalFixture())
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE api_keys SET status='disabled', expires_at=clock_timestamp()-interval '1 day' WHERE id=11`)
	require.NoError(t, err)
	_, created, err := s.CreateTask(ctx, journalFixture())
	require.NoError(t, err)
	require.False(t, created)
	task := journalFixture()
	task.ID = "new-task"
	_, _, err = s.CreateTask(ctx, task)
	require.ErrorIs(t, err, ErrMediaKeyUnavailable)
	_, err = db.Exec(`UPDATE api_keys SET status='active' WHERE id=11`)
	require.NoError(t, err)
	_, _, err = s.CreateTask(ctx, task)
	require.ErrorIs(t, err, ErrMediaKeyUnavailable)
}

func TestJournalPostgresReferenceCleanupIsBoundedAndKeepsLedger(t *testing.T) {
	s, db := newJournalPostgres(t)
	ctx := context.Background()
	_, _, err := s.CreateTask(ctx, journalFixture())
	require.NoError(t, err)
	hold := &MediaPendingHold{TaskID: "keep-hold", UserID: 7, APIKeyID: 11, Amount: 0.5}
	require.NoError(t, s.TrackPendingHold(ctx, hold, time.Nanosecond))
	for i := 0; i < 3; i++ {
		file := &MediaFileRecord{ID: fmt.Sprintf("expired-%d", i), UserID: 7, APIKeyID: 11, ExpiresAt: time.Now().Add(-time.Hour).Unix()}
		require.NoError(t, s.SaveFile(ctx, file, time.Hour))
	}
	live := &MediaFileRecord{ID: "live", UserID: 7, APIKeyID: 11, ExpiresAt: time.Now().Add(time.Hour).Unix()}
	require.NoError(t, s.SaveFile(ctx, live, time.Hour))
	deleted, err := s.CleanupExpiredFiles(ctx, 2)
	require.NoError(t, err)
	require.Equal(t, int64(2), deleted)
	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM subnexus_seedance_files`).Scan(&count))
	require.Equal(t, 2, count)
	_, err = s.GetFile(ctx, "live")
	require.NoError(t, err)
	_, err = s.GetTask(ctx, journalFixture().ID)
	require.NoError(t, err)
	holds, err := s.ListDuePendingHolds(ctx, time.Now(), 100)
	require.NoError(t, err)
	require.Len(t, holds, 1)
}
