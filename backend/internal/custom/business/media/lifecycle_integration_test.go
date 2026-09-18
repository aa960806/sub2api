//go:build integration

package media

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

// These tests use the production balance ledger, not a simulated balance.
// The shared helper creates and removes an isolated schema on a local test DB.
type postgresLifecycleEnv struct {
	store     *mediaTaskStore
	db        *sql.DB
	billing   *MediaTaskBilling
	handler   *MediaTaskHandler
	provider  *fakeMediaProvider
	failUsage atomic.Bool
}

func newPostgresLifecycleEnv(t *testing.T) *postgresLifecycleEnv {
	t.Helper()
	store, db := newJournalPostgres(t)
	_, err := db.Exec(`CREATE TABLE users (id BIGINT PRIMARY KEY, balance NUMERIC(20,8) NOT NULL,
		frozen_balance NUMERIC(20,8) NOT NULL DEFAULT 0, updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), deleted_at TIMESTAMPTZ);
		INSERT INTO users (id,balance) VALUES (7,100),(9,42);
		ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active', ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
		UPDATE api_keys SET quota=100 WHERE id=11;
		CREATE TABLE test_media_usage (request_id TEXT NOT NULL, api_key_id BIGINT NOT NULL, actual_cost NUMERIC(20,8) NOT NULL, PRIMARY KEY(request_id,api_key_id));`)
	require.NoError(t, err)
	for _, name := range []string{"071_add_usage_billing_dedup.sql", "073_add_usage_billing_dedup_archive.sql"} {
		migration, err := migrations.FS.ReadFile(name)
		require.NoError(t, err)
		_, err = db.Exec(string(migration))
		require.NoError(t, err)
	}
	provider := newFakeMediaProvider()
	billing := NewMediaTaskBilling(service.NewGatewayForMediaBillingTest(repository.NewUsageBillingRepository(nil, db)), nil)
	e := &postgresLifecycleEnv{store: store, db: db, billing: billing, provider: provider}
	billing.usageWriter = func(ctx context.Context, usage *UsageLog) error {
		if e.failUsage.Load() {
			return errors.New("injected usage storage outage")
		}
		_, err := db.ExecContext(ctx, `INSERT INTO test_media_usage (request_id,api_key_id,actual_cost) VALUES($1,$2,$3) ON CONFLICT(request_id,api_key_id) DO NOTHING`, usage.RequestID, usage.APIKeyID, usage.ActualCost)
		return err
	}
	account := &Account{ID: 23, Platform: PlatformSeedance, Type: service.AccountTypeAPIKey, Concurrency: 10, Credentials: map[string]any{"api_key": "test-only", "base_url": "https://example.invalid/seedance"}}
	e.handler = &MediaTaskHandler{
		tasks:             NewMediaTaskService(store, PlatformSeedance, provider).WithBilling(billing),
		gatewayService:    &fakeAccountResolver{accounts: map[int64]*Account{23: account}, selected: 23},
		concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(&mediaConcurrencyFake{}), SSEPingFormatNone, 0),
	}
	return e
}

func (e *postgresLifecycleEnv) task(id string) *MediaTaskRecord {
	task := journalFixture()
	task.ID, task.UnitPrice, task.CreatedAt = id, 2, time.Now().Unix()
	task.Platform, task.Model, task.Status = PlatformSeedance, "seedance2.0", MediaTaskStatusQueued
	task.BillingQuota, task.BillingAccountType = true, service.AccountTypeAPIKey
	task.Request = &MediaVideoCreateRequest{Model: task.Model, Prompt: "local fake provider test", DurationSeconds: 5}
	task.UpstreamKey = "local-test-" + id
	account, _ := e.handler.gatewayService.GetMediaTaskAccount(context.Background(), task.AccountID)
	task.CredentialHash = mediaAccountFingerprint(account)
	return task
}

func (e *postgresLifecycleEnv) balances(t *testing.T, balance, frozen, quota float64) {
	t.Helper()
	var actualBalance, actualFrozen, actualQuota, controlBalance, controlFrozen float64
	require.NoError(t, e.db.QueryRow(`SELECT balance,frozen_balance FROM users WHERE id=7`).Scan(&actualBalance, &actualFrozen))
	require.Equal(t, balance, actualBalance)
	require.Equal(t, frozen, actualFrozen)
	require.NoError(t, e.db.QueryRow(`SELECT quota_used FROM api_keys WHERE id=11`).Scan(&actualQuota))
	require.Equal(t, quota, actualQuota)
	require.NoError(t, e.db.QueryRow(`SELECT balance,frozen_balance FROM users WHERE id=9`).Scan(&controlBalance, &controlFrozen))
	require.Equal(t, 42.0, controlBalance, "unrelated user must remain untouched")
	require.Zero(t, controlFrozen)
}

func (e *postgresLifecycleEnv) phase(t *testing.T, id, phase string) *MediaTaskRecord {
	t.Helper()
	task, err := e.store.GetTask(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, phase, task.Phase)
	return task
}

func TestLifecyclePostgresConcurrentCreateAndCaptureUsageRetry(t *testing.T) {
	e := newPostgresLifecycleEnv(t)
	ctx := context.Background()
	task := e.task("concurrent-capture")
	var created atomic.Int32
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, fresh, err := e.store.CreateTask(ctx, task)
			if fresh {
				created.Add(1)
			}
			if err == nil {
				err = e.handler.advanceTask(ctx, task.ID, true)
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	require.Equal(t, int32(1), created.Load())
	e.phase(t, task.ID, mediaPhaseReserved)
	e.balances(t, 98, 2, 0)
	require.NoError(t, e.handler.advanceTask(ctx, task.ID, false))
	stored := e.phase(t, task.ID, mediaPhaseSubmitted)
	require.Equal(t, 1, e.provider.createCalls)
	e.provider.setStatus(stored.JobID, MediaTaskStatusSucceeded)
	e.failUsage.Store(true)
	require.ErrorContains(t, e.handler.advanceTask(ctx, task.ID, false), "usage storage outage")
	stored = e.phase(t, task.ID, mediaPhaseCapturing)
	e.balances(t, 98, 0, 2)
	key, _ := mediaBillingIdentity(stored)
	_, ref, err := e.handler.fetchDownloadForTask(ctx, key, stored)
	require.NoError(t, err)
	require.True(t, ref.NotReady, "no video download before accounting finishes")
	e.failUsage.Store(false)
	for i := 0; i < 3; i++ {
		require.NoError(t, e.handler.advanceTask(ctx, task.ID, false))
	}
	e.phase(t, task.ID, mediaPhaseSettled)
	e.balances(t, 98, 0, 2)
	var usageCount, dedupCount int
	require.NoError(t, e.db.QueryRow(`SELECT COUNT(*) FROM test_media_usage`).Scan(&usageCount))
	require.Equal(t, 1, usageCount)
	require.NoError(t, e.db.QueryRow(`SELECT COUNT(*) FROM usage_billing_dedup`).Scan(&dedupCount))
	require.Equal(t, 3, dedupCount, "one hold, one capture, one quota effect")
	replayed, fresh, err := e.store.CreateTask(ctx, task)
	require.NoError(t, err)
	require.False(t, fresh)
	require.Equal(t, mediaPhaseSettled, replayed.Phase)
	require.Equal(t, 1, e.provider.createCalls)
}

func TestLifecyclePostgresSettlesAfterOwnerOrKeySoftDeletion(t *testing.T) {
	for _, outcome := range []string{MediaTaskStatusSucceeded, MediaTaskStatusFailed} {
		for _, deleted := range []string{"api_keys", "users"} {
			t.Run(deleted+"/"+outcome, func(t *testing.T) {
				e := newPostgresLifecycleEnv(t)
				ctx := context.Background()
				_, err := e.db.Exec(`UPDATE api_keys SET rate_limit_5h=100 WHERE id=11`)
				require.NoError(t, err)
				task := e.task("deleted-identity")
				_, _, err = e.store.CreateTask(ctx, task)
				require.NoError(t, err)
				require.NoError(t, e.handler.advanceTask(ctx, task.ID, false))
				stored := e.phase(t, task.ID, mediaPhaseSubmitted)
				e.balances(t, 98, 2, 0)
				// Table names are fixed test cases, never request input.
				_, err = e.db.Exec("UPDATE " + deleted + " SET deleted_at=NOW()")
				require.NoError(t, err)
				e.provider.setStatus(stored.JobID, outcome)
				for i := 0; i < 3; i++ {
					require.NoError(t, e.handler.advanceTask(ctx, task.ID, false))
				}
				var quota, balance, rate float64
				var count int
				phase := mediaPhaseReleased
				balance = 100
				if outcome == MediaTaskStatusSucceeded {
					phase, balance, count = mediaPhaseSettled, 98, 1
					if deleted != "api_keys" {
						quota, rate = 2, 2
					}
				}
				e.phase(t, task.ID, phase)
				e.balances(t, balance, 0, quota)
				var usageCount int
				var actualRate float64
				require.NoError(t, e.db.QueryRow(`SELECT COUNT(*) FROM test_media_usage`).Scan(&usageCount))
				require.Equal(t, count, usageCount)
				require.NoError(t, e.db.QueryRow(`SELECT usage_5h FROM api_keys WHERE id=11`).Scan(&actualRate))
				require.Equal(t, rate, actualRate)
			})
		}
	}
}

func TestLifecyclePostgresHistoricalSettlementRejectsUnboundCommands(t *testing.T) {
	e := newPostgresLifecycleEnv(t)
	ctx := context.Background()
	task := e.task("guarded-settlement")
	_, _, err := e.store.CreateTask(ctx, task)
	require.NoError(t, err)
	require.NoError(t, e.handler.advanceTask(ctx, task.ID, true))
	task = e.phase(t, task.ID, mediaPhaseReserved)
	repo := service.MediaBillingDepsOf(e.billing.gateway).UsageBilling.(interface {
		CaptureSeedanceBalance(context.Context, *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error)
		ReleaseSeedanceBalance(context.Context, *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error)
		ApplySeedanceSettlement(context.Context, *UsageBillingCommand) (*UsageBillingApplyResult, error)
	})
	batchID := mediaHoldBatchID(task.ID)
	base := BatchImageBalanceHoldCommand{RequestID: BatchImageCaptureRequestID(batchID), BatchID: batchID,
		UserID: task.UserID, APIKeyID: task.APIKeyID, HoldAmount: 2, ActualAmount: 2}
	_, err = repo.CaptureSeedanceBalance(ctx, &base)
	require.Error(t, err, "reservation alone is not a persisted capture decision")
	task.Phase = mediaPhaseCapturing
	require.NoError(t, e.store.SaveTask(ctx, task, time.Hour))
	for _, mutate := range []func(*BatchImageBalanceHoldCommand){
		func(c *BatchImageBalanceHoldCommand) { c.UserID = 9 },
		func(c *BatchImageBalanceHoldCommand) { c.APIKeyID++ },
		func(c *BatchImageBalanceHoldCommand) { c.HoldAmount, c.ActualAmount = 3, 3 },
		func(c *BatchImageBalanceHoldCommand) { c.BatchID = "image:unrelated" },
	} {
		cmd := base
		mutate(&cmd)
		_, err = repo.CaptureSeedanceBalance(ctx, &cmd)
		require.Error(t, err)
	}
	_, err = repo.ReleaseSeedanceBalance(ctx, &BatchImageBalanceHoldCommand{
		RequestID: BatchImageReleaseRequestID(batchID), BatchID: batchID,
		UserID: task.UserID, APIKeyID: task.APIKeyID, HoldAmount: 2,
	})
	require.Error(t, err, "capture intent cannot change into a refund")
	usage := UsageBillingCommand{RequestID: base.RequestID + ":usage", APIKeyID: task.APIKeyID,
		UserID: task.UserID, AccountID: task.AccountID, AccountType: service.AccountTypeAPIKey,
		Model: task.Model, MediaType: "video", BillingType: BillingTypeBalance, APIKeyQuotaCost: 2}
	_, err = repo.ApplySeedanceSettlement(ctx, &usage)
	require.Error(t, err, "quota cannot settle without an actual capture claim")
	_, err = repo.CaptureSeedanceBalance(ctx, &base)
	require.NoError(t, err)
	for _, mutate := range []func(*UsageBillingCommand){
		func(c *UsageBillingCommand) { c.BalanceCost = 2 },
		func(c *UsageBillingCommand) { c.Model = "unrelated-model" },
		func(c *UsageBillingCommand) { c.AccountID++ },
		func(c *UsageBillingCommand) { c.APIKeyQuotaCost = 3 },
	} {
		cmd := usage
		cmd.RequestFingerprint = ""
		mutate(&cmd)
		_, err = repo.ApplySeedanceSettlement(ctx, &cmd)
		require.Error(t, err)
	}
	e.balances(t, 98, 0, 0)
	// Failed guard checks must roll back their dedup claims, allowing the exact
	// persisted command to complete later without a phantom success.
	require.NoError(t, e.handler.advanceTask(ctx, task.ID, false))
	e.phase(t, task.ID, mediaPhaseSettled)
	e.balances(t, 98, 0, 2)
}

func TestLifecyclePostgresRecoversReserveAndReleaseAfterMoneyCommit(t *testing.T) {
	e := newPostgresLifecycleEnv(t)
	ctx := context.Background()
	task := e.task("reserve-release-crash")
	task.Phase = mediaPhaseReserving
	_, _, err := e.store.CreateTask(ctx, task)
	require.NoError(t, err)
	key, account := mediaBillingIdentity(task)
	_, err = e.billing.Reserve(ctx, task, key, nil, account)
	require.NoError(t, err)
	// Simulate process death after SQL commit and before the journal save.
	e.phase(t, task.ID, mediaPhaseReserving)
	e.balances(t, 98, 2, 0)
	require.NoError(t, e.handler.advanceTask(ctx, task.ID, true))
	task = e.phase(t, task.ID, mediaPhaseReserved)
	e.balances(t, 98, 2, 0)
	task.Phase, task.Status = mediaPhaseReleasing, MediaTaskStatusFailed
	require.NoError(t, e.store.SaveTask(ctx, task, time.Hour))
	require.NoError(t, e.billing.Release(ctx, task, key))
	e.phase(t, task.ID, mediaPhaseReleasing)
	e.balances(t, 100, 0, 0)
	for i := 0; i < 3; i++ {
		require.NoError(t, e.handler.advanceTask(ctx, task.ID, false))
	}
	e.phase(t, task.ID, mediaPhaseReleased)
	e.balances(t, 100, 0, 0)
	var claims, usage int
	require.NoError(t, e.db.QueryRow(`SELECT COUNT(*) FROM usage_billing_dedup`).Scan(&claims))
	require.Equal(t, 2, claims)
	require.NoError(t, e.db.QueryRow(`SELECT COUNT(*) FROM test_media_usage`).Scan(&usage))
	require.Zero(t, usage)
	require.Zero(t, e.provider.createCalls)
}

func TestLifecyclePostgresRecoversCaptureAfterMoneyCommitAndDedupArchive(t *testing.T) {
	e := newPostgresLifecycleEnv(t)
	ctx := context.Background()
	task := e.task("capture-crash")
	_, _, err := e.store.CreateTask(ctx, task)
	require.NoError(t, err)
	require.NoError(t, e.handler.advanceTask(ctx, task.ID, true))
	task = e.phase(t, task.ID, mediaPhaseReserved)
	task.Phase, task.Status = mediaPhaseCapturing, MediaTaskStatusSucceeded
	require.NoError(t, e.store.SaveTask(ctx, task, time.Hour))
	key, account := mediaBillingIdentity(task)
	require.NoError(t, e.billing.Capture(ctx, task, key, nil, account))
	e.balances(t, 98, 0, 2)
	e.phase(t, task.ID, mediaPhaseCapturing)
	// Production cleanup archives old ledger identities; recovery must continue
	// to recognize them rather than charging the balance or quota again.
	_, err = e.db.Exec(`INSERT INTO usage_billing_dedup_archive (request_id,api_key_id,request_fingerprint,created_at) SELECT request_id,api_key_id,request_fingerprint,created_at FROM usage_billing_dedup; DELETE FROM usage_billing_dedup`)
	require.NoError(t, err)
	for i := 0; i < 3; i++ {
		require.NoError(t, e.handler.advanceTask(ctx, task.ID, false))
	}
	e.phase(t, task.ID, mediaPhaseSettled)
	e.balances(t, 98, 0, 2)
	var usage int
	require.NoError(t, e.db.QueryRow(`SELECT COUNT(*) FROM test_media_usage`).Scan(&usage))
	require.Equal(t, 1, usage)
}
