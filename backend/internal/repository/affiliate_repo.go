package repository

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net/netip"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

const (
	affiliateCodeLength      = 12
	affiliateCodeMaxAttempts = 12

	subNexusSignupRewardSavepoint = "subnexus_signup_reward"

	subNexusSignupRewardJobPending       = "pending"
	subNexusSignupRewardJobCompleted     = "completed"
	subNexusSignupRewardJobSkipped       = "skipped"
	subNexusSignupRewardRetryBase        = 5 * time.Second
	subNexusSignupRewardRetryMaxBackoff  = time.Hour
	subNexusSignupRewardDefaultScanLimit = 50
)

var affiliateCodeCharset = []byte("ABCDEFGHJKLMNPQRSTUVWXYZ23456789")

const affiliateUserOverviewSQL = `
SELECT ua.user_id,
       COALESCE(u.email, ''),
       COALESCE(u.username, ''),
       ua.aff_code,
       COALESCE(ua.aff_rebate_rate_percent, 0)::double precision,
       (ua.aff_rebate_rate_percent IS NOT NULL) AS has_custom_rate,
       ua.aff_count,
       COALESCE(rebated.rebated_invitee_count, 0),
       (ua.aff_quota + COALESCE(matured.matured_frozen_quota, 0))::double precision,
       ua.aff_history_quota::double precision
FROM user_affiliates ua
JOIN users u ON u.id = ua.user_id
LEFT JOIN (
    SELECT user_id, COUNT(DISTINCT source_user_id)::integer AS rebated_invitee_count
    FROM user_affiliate_ledger
    WHERE action = 'accrue' AND source_user_id IS NOT NULL
    GROUP BY user_id
) rebated ON rebated.user_id = ua.user_id
LEFT JOIN (
    SELECT user_id, COALESCE(SUM(amount), 0)::double precision AS matured_frozen_quota
    FROM user_affiliate_ledger
    WHERE action = 'accrue' AND frozen_until IS NOT NULL AND frozen_until <= NOW()
    GROUP BY user_id
) matured ON matured.user_id = ua.user_id
WHERE ua.user_id = $1
LIMIT 1`

type affiliateQueryExecer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type affiliateRepository struct {
	client *dbent.Client
}

var _ service.AffiliateSignupRewardRepository = (*affiliateRepository)(nil)
var _ service.AffiliateSignupRewardPendingRepository = (*affiliateRepository)(nil)

func NewAffiliateRepository(client *dbent.Client, _ *sql.DB) service.AffiliateRepository {
	return &affiliateRepository{client: client}
}

func (r *affiliateRepository) EnsureUserAffiliate(ctx context.Context, userID int64) (*service.AffiliateSummary, error) {
	if userID <= 0 {
		return nil, service.ErrUserNotFound
	}
	client := clientFromContext(ctx, r.client)
	return ensureUserAffiliateWithClient(ctx, client, userID)
}

func (r *affiliateRepository) GetAffiliateByCode(ctx context.Context, code string) (*service.AffiliateSummary, error) {
	client := clientFromContext(ctx, r.client)
	return queryAffiliateByCode(ctx, client, code)
}

func (r *affiliateRepository) BindInviter(ctx context.Context, userID, inviterID int64) (bool, error) {
	var bound bool
	err := r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		if _, err := ensureUserAffiliateWithClient(txCtx, txClient, userID); err != nil {
			return err
		}
		if _, err := ensureUserAffiliateWithClient(txCtx, txClient, inviterID); err != nil {
			return err
		}

		res, err := txClient.ExecContext(txCtx,
			"UPDATE user_affiliates SET inviter_id = $1, updated_at = NOW() WHERE user_id = $2 AND inviter_id IS NULL",
			inviterID, userID,
		)
		if err != nil {
			return fmt.Errorf("bind inviter: %w", err)
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			bound = false
			return nil
		}

		if _, err = txClient.ExecContext(txCtx,
			"UPDATE user_affiliates SET aff_count = aff_count + 1, updated_at = NOW() WHERE user_id = $1",
			inviterID,
		); err != nil {
			return fmt.Errorf("increment inviter aff_count: %w", err)
		}
		bound = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return bound, nil
}

// BindInviterAndEnqueueSignupReward atomically binds an invitee and persists
// the immutable signup-reward policy snapshot. If either write fails the
// transaction is rolled back, so a successful binding can never be left
// without a durable reward job on the production path.
func (r *affiliateRepository) BindInviterAndEnqueueSignupReward(ctx context.Context, userID, inviterID int64, pending service.AffiliateSignupRewardPending) (bool, int64, error) {
	if r == nil || r.client == nil {
		return false, 0, errors.New("affiliate repository unavailable")
	}
	var bound bool
	var jobID int64
	err := r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		if _, err := ensureUserAffiliateWithClient(txCtx, txClient, userID); err != nil {
			return err
		}
		if _, err := ensureUserAffiliateWithClient(txCtx, txClient, inviterID); err != nil {
			return err
		}

		res, err := txClient.ExecContext(txCtx,
			"UPDATE user_affiliates SET inviter_id = $1, updated_at = NOW() WHERE user_id = $2 AND inviter_id IS NULL",
			inviterID, userID,
		)
		if err != nil {
			return fmt.Errorf("bind inviter: %w", err)
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			bound = false
			return nil
		}
		if _, err = txClient.ExecContext(txCtx,
			"UPDATE user_affiliates SET aff_count = aff_count + 1, updated_at = NOW() WHERE user_id = $1",
			inviterID,
		); err != nil {
			return fmt.Errorf("increment inviter aff_count: %w", err)
		}
		bound = true

		// EnqueueSignupReward detects the transaction in the context and runs
		// against txClient, without opening a second transaction.
		var enqueueErr error
		jobID, _, enqueueErr = r.EnqueueSignupReward(txCtx, pending)
		if enqueueErr != nil {
			return enqueueErr
		}
		return nil
	})
	if err != nil {
		return false, 0, err
	}
	return bound, jobID, nil
}

// GrantSignupReward atomically grants the optional SubNexus registration
// balances and writes one audit ledger row per recipient. The invitee lock and
// partial unique indexes make retries/concurrent callbacks idempotent without
// touching upstream affiliate quota semantics.
func (r *affiliateRepository) GrantSignupReward(ctx context.Context, inviterID, inviteeUserID int64, inviterAmount, inviteeAmount float64, clientIP string, ipLimitEnabled bool, ipDailyLimit int) (service.AffiliateSignupRewardResult, error) {
	result := service.AffiliateSignupRewardResult{}
	if inviterID <= 0 || inviteeUserID <= 0 || inviterID == inviteeUserID {
		return skippedSignupReward("invalid_request"), nil
	}
	if !validSignupRewardAmount(inviterAmount) || !validSignupRewardAmount(inviteeAmount) || (inviterAmount <= 0 && inviteeAmount <= 0) {
		return skippedSignupReward("invalid_amount"), nil
	}
	clientIP = strings.TrimSpace(clientIP)
	if clientIP != "" {
		addr, err := netip.ParseAddr(clientIP)
		if err != nil {
			return skippedSignupReward("invalid_client_ip"), nil
		}
		clientIP = addr.Unmap().String()
	}
	if ipLimitEnabled {
		// A missing trusted IP must never bypass the configured anti-abuse limit.
		if clientIP == "" {
			return skippedSignupReward("missing_client_ip"), nil
		}
		if ipDailyLimit <= 0 || ipDailyLimit > service.SubNexusInviteSignupRewardIPDailyMax {
			ipDailyLimit = service.SubNexusInviteSignupRewardIPDailyDefault
		}
	}

	err := r.withSubNexusSignupRewardTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		if _, err := txClient.ExecContext(txCtx, "SELECT pg_advisory_xact_lock(1397638990, hashint8($1::bigint))", inviteeUserID); err != nil {
			return fmt.Errorf("lock SubNexus signup reward: %w", err)
		}
		bound, err := querySubNexusSignupRewardBindingExists(txCtx, txClient, inviterID, inviteeUserID)
		if err != nil {
			return fmt.Errorf("verify SubNexus signup reward binding: %w", err)
		}
		if !bound {
			result = skippedSignupReward("inviter_mismatch")
			return nil
		}

		exists, err := querySubNexusSignupRewardExists(txCtx, txClient, inviteeUserID)
		if err != nil {
			return fmt.Errorf("check SubNexus signup reward idempotency: %w", err)
		}
		if exists {
			result = skippedSignupReward("already_granted")
			return nil
		}

		if ipLimitEnabled {
			if _, err := txClient.ExecContext(txCtx, "SELECT pg_advisory_xact_lock(1397638991, hashtext($1))", clientIP); err != nil {
				return fmt.Errorf("lock SubNexus signup reward IP: %w", err)
			}
			count, err := querySubNexusSignupRewardIPDailyCount(txCtx, txClient, clientIP)
			if err != nil {
				return fmt.Errorf("check SubNexus signup reward IP limit: %w", err)
			}
			if count >= ipDailyLimit {
				result = skippedSignupReward("ip_daily_limit")
				return nil
			}
		}

		if inviterAmount > 0 {
			if err := grantSubNexusSignupBalanceTx(txCtx, txClient, inviterID, inviterAmount, "signup_bonus_inviter", inviteeUserID, clientIP); err != nil {
				return err
			}
		}
		if inviteeAmount > 0 {
			if err := grantSubNexusSignupBalanceTx(txCtx, txClient, inviteeUserID, inviteeAmount, "signup_bonus_invitee", inviteeUserID, clientIP); err != nil {
				return err
			}
		}
		result.Applied = true
		return nil
	})
	return result, err
}

// EnqueueSignupReward persists the reward policy snapshot after an inviter
// binding.  The gate is re-read and locked in this transaction so a concurrent
// disable cannot race an enqueue into a balance-writing path.  Existing rows
// are returned unchanged; the unique invitee key makes retries idempotent and
// preserves the original policy snapshot.
func (r *affiliateRepository) EnqueueSignupReward(ctx context.Context, pending service.AffiliateSignupRewardPending) (jobID int64, inserted bool, err error) {
	if r == nil || r.client == nil {
		return 0, false, errors.New("affiliate repository unavailable")
	}
	if pending.InviterID <= 0 || pending.InviteeUserID <= 0 || pending.InviterID == pending.InviteeUserID {
		return 0, false, nil
	}
	if !validSignupRewardAmount(pending.InviterAmount) || !validSignupRewardAmount(pending.InviteeAmount) || (pending.InviterAmount <= 0 && pending.InviteeAmount <= 0) {
		return 0, false, nil
	}
	if pending.IPDailyLimit <= 0 || pending.IPDailyLimit > service.SubNexusInviteSignupRewardIPDailyMax {
		pending.IPDailyLimit = service.SubNexusInviteSignupRewardIPDailyDefault
	}
	pending.ClientIP = strings.TrimSpace(pending.ClientIP)
	if pending.ClientIP != "" {
		addr, parseErr := netip.ParseAddr(pending.ClientIP)
		if parseErr != nil {
			// Preserve the row for an auditable deterministic skip.  The reward
			// processor will mark it skipped without touching balances.
			pending.ClientIP = strings.TrimSpace(pending.ClientIP)
		} else {
			pending.ClientIP = addr.Unmap().String()
		}
	}

	apply := func(txCtx context.Context, txClient *dbent.Client) error {
		gateEnabled, gateErr := querySubNexusInviteSignupRewardGateTx(txCtx, txClient)
		if gateErr != nil {
			return fmt.Errorf("verify SubNexus signup reward gate: %w", gateErr)
		}
		if !gateEnabled {
			jobID = 0
			inserted = false
			return nil
		}

		row, queryErr := txClient.QueryContext(txCtx, `
INSERT INTO subnexus_affiliate_signup_reward_jobs (
    inviter_id, invitee_user_id, inviter_amount, invitee_amount,
    client_ip, ip_limit_enabled, ip_daily_limit, status,
    attempt_count, next_attempt_at, last_error, skip_reason,
    created_at, updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, 'pending', 0, NOW(), '', '', NOW(), NOW())
ON CONFLICT (invitee_user_id) DO NOTHING
RETURNING id`,
			pending.InviterID,
			pending.InviteeUserID,
			pending.InviterAmount,
			pending.InviteeAmount,
			pending.ClientIP,
			pending.IPLimitEnabled,
			pending.IPDailyLimit,
		)
		if queryErr != nil {
			return fmt.Errorf("enqueue SubNexus signup reward: %w", queryErr)
		}
		if row.Next() {
			if scanErr := row.Scan(&jobID); scanErr != nil {
				_ = row.Close()
				return fmt.Errorf("scan SubNexus signup reward job: %w", scanErr)
			}
			inserted = true
		} else if rowErr := row.Err(); rowErr != nil {
			_ = row.Close()
			return fmt.Errorf("read SubNexus signup reward insert result: %w", rowErr)
		}
		if closeErr := row.Close(); closeErr != nil {
			return fmt.Errorf("close SubNexus signup reward insert: %w", closeErr)
		}
		if inserted {
			return nil
		}

		// ON CONFLICT DO NOTHING has no RETURNING row.  Fetch the existing job
		// so a process that was interrupted between enqueue and immediate
		// processing can safely resume it.
		existing, fetchErr := querySignupRewardJobByInvitee(txCtx, txClient, pending.InviteeUserID)
		if errors.Is(fetchErr, sql.ErrNoRows) {
			return nil
		}
		if fetchErr != nil {
			return fmt.Errorf("load existing SubNexus signup reward job: %w", fetchErr)
		}
		jobID = existing.ID
		return nil
	}

	if tx := dbent.TxFromContext(ctx); tx != nil {
		err = r.withSubNexusSignupRewardTx(ctx, apply)
		return jobID, inserted, err
	}
	err = r.withTx(ctx, apply)
	return jobID, inserted, err
}

// ProcessSignupReward performs one durable state transition.  The pending row
// remains locked while GrantSignupReward executes, so concurrent workers cannot
// issue duplicate balances.  A reward error is persisted with backoff and is
// returned to the caller for observability; it is not allowed to abort the
// registration or lose the job.
func (r *affiliateRepository) ProcessSignupReward(ctx context.Context, jobID int64) (service.AffiliateSignupRewardProcessResult, error) {
	var result service.AffiliateSignupRewardProcessResult
	if r == nil || r.client == nil || jobID <= 0 {
		return result, nil
	}
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return result, fmt.Errorf("begin SubNexus signup reward job transaction: %w", err)
	}
	txCtx := dbent.NewTxContext(ctx, tx)
	defer func() { _ = tx.Rollback() }()

	// Acquire the rollout row before the job row.  EnqueueSignupReward uses
	// the same order; keeping lock ordering consistent avoids a gate/job
	// deadlock when a duplicate registration and the scanner race.
	gateEnabled, err := querySubNexusInviteSignupRewardGateTx(txCtx, tx.Client())
	if err != nil {
		return result, fmt.Errorf("verify SubNexus signup reward gate: %w", err)
	}
	if !gateEnabled {
		if err := tx.Commit(); err != nil {
			return result, fmt.Errorf("commit disabled SubNexus signup reward job lookup: %w", err)
		}
		return result, nil
	}

	job, err := querySignupRewardJobByID(txCtx, tx.Client(), jobID, true)
	if errors.Is(err, sql.ErrNoRows) {
		if commitErr := tx.Commit(); commitErr != nil {
			return result, fmt.Errorf("commit missing SubNexus signup reward job lookup: %w", commitErr)
		}
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("load SubNexus signup reward job: %w", err)
	}
	result.InviterID = job.InviterID
	result.InviteeUserID = job.InviteeUserID
	if job.Status != subNexusSignupRewardJobPending {
		if err := tx.Commit(); err != nil {
			return result, fmt.Errorf("commit completed SubNexus signup reward job lookup: %w", err)
		}
		return result, nil
	}

	rewardResult, rewardErr := r.GrantSignupReward(txCtx, job.InviterID, job.InviteeUserID, job.InviterAmount, job.InviteeAmount, job.ClientIP, job.IPLimit, job.IPDailyLimit)
	if rewardErr != nil {
		attempt := job.AttemptCount + 1
		nextAttempt := time.Now().UTC().Add(subNexusSignupRewardRetryDelay(attempt))
		if _, updateErr := tx.Client().ExecContext(txCtx, `
UPDATE subnexus_affiliate_signup_reward_jobs
SET attempt_count = $1,
    next_attempt_at = $2,
    last_error = $3,
    updated_at = NOW()
WHERE id = $4 AND status = 'pending'`,
			attempt,
			nextAttempt,
			truncateSignupRewardError(rewardErr),
			job.ID,
		); updateErr != nil {
			return result, fmt.Errorf("schedule SubNexus signup reward retry: %w (original: %v)", updateErr, rewardErr)
		}
		if commitErr := tx.Commit(); commitErr != nil {
			return result, fmt.Errorf("commit SubNexus signup reward retry: %w (original: %v)", commitErr, rewardErr)
		}
		result.RetryScheduled = true
		return result, rewardErr
	}

	status := subNexusSignupRewardJobSkipped
	skipReason := strings.TrimSpace(rewardResult.SkipReason)
	if rewardResult.Applied || skipReason == "already_granted" {
		status = subNexusSignupRewardJobCompleted
		result.Completed = true
		result.Applied = rewardResult.Applied
	} else if rewardResult.Skipped {
		result.Skipped = true
	} else {
		// A repository implementation should always describe a result.  Treat
		// an empty result as a deterministic skip rather than risking an
		// endlessly hot retry loop.
		result.Skipped = true
		skipReason = "empty_result"
	}
	if _, err := tx.Client().ExecContext(txCtx, `
UPDATE subnexus_affiliate_signup_reward_jobs
SET status = $1,
    skip_reason = $2,
    last_error = '',
    updated_at = NOW()
WHERE id = $3 AND status = 'pending'`, status, skipReason, job.ID); err != nil {
		return result, fmt.Errorf("complete SubNexus signup reward job: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return result, fmt.Errorf("commit SubNexus signup reward job: %w", err)
	}
	result.SkipReason = skipReason
	return result, nil
}

// ReconcileSignupRewards processes a bounded due batch.  Each job is handled
// in its own transaction so one transient failure cannot roll back unrelated
// completions.  A gate race or an empty queue stops the scan without spinning.
func (r *affiliateRepository) ReconcileSignupRewards(ctx context.Context, limit int) (service.AffiliateSignupRewardReconcileResult, error) {
	var result service.AffiliateSignupRewardReconcileResult
	if r == nil || r.client == nil {
		return result, nil
	}
	if limit <= 0 || limit > 1000 {
		limit = subNexusSignupRewardDefaultScanLimit
	}
	for i := 0; i < limit; i++ {
		jobID, err := r.findDueSignupRewardJob(ctx)
		if errors.Is(err, sql.ErrNoRows) {
			break
		}
		if err != nil {
			return result, fmt.Errorf("find due SubNexus signup reward job: %w", err)
		}
		processed, processErr := r.ProcessSignupReward(ctx, jobID)
		if processed.InviterID > 0 {
			result.Examined++
		}
		if processed.Applied {
			result.Applied++
			result.Completed++
			result.AffectedUserIDs = appendUniqueInt64(result.AffectedUserIDs, processed.InviterID, processed.InviteeUserID)
		} else if processed.Completed {
			result.Completed++
		} else if processed.Skipped {
			result.Skipped++
		} else if processed.RetryScheduled {
			result.Retried++
		}
		if processErr != nil && !processed.RetryScheduled {
			return result, processErr
		}
		if processErr != nil && processed.RetryScheduled {
			// Continue with the next due row; the current row now has a future
			// next_attempt_at and cannot be selected again in this scan.
			continue
		}
		if processed.InviterID == 0 {
			break
		}
	}
	return result, nil
}

type subNexusSignupRewardJob struct {
	ID            int64
	InviterID     int64
	InviteeUserID int64
	InviterAmount float64
	InviteeAmount float64
	ClientIP      string
	IPLimit       bool
	IPDailyLimit  int
	Status        string
	AttemptCount  int
}

func querySignupRewardJobByID(ctx context.Context, client affiliateQueryExecer, jobID int64, lock bool) (*subNexusSignupRewardJob, error) {
	query := `
SELECT id, inviter_id, invitee_user_id,
       inviter_amount::double precision,
       invitee_amount::double precision,
       client_ip, ip_limit_enabled, ip_daily_limit, status, attempt_count
FROM subnexus_affiliate_signup_reward_jobs
WHERE id = $1`
	if lock {
		query += " FOR UPDATE"
	}
	return scanSignupRewardJob(ctx, client, query, jobID)
}

func querySignupRewardJobByInvitee(ctx context.Context, client affiliateQueryExecer, inviteeUserID int64) (*subNexusSignupRewardJob, error) {
	return scanSignupRewardJob(ctx, client, `
SELECT id, inviter_id, invitee_user_id,
       inviter_amount::double precision,
       invitee_amount::double precision,
       client_ip, ip_limit_enabled, ip_daily_limit, status, attempt_count
FROM subnexus_affiliate_signup_reward_jobs
WHERE invitee_user_id = $1
LIMIT 1`, inviteeUserID)
}

func scanSignupRewardJob(ctx context.Context, client affiliateQueryExecer, query string, args ...any) (*subNexusSignupRewardJob, error) {
	rows, err := client.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, sql.ErrNoRows
	}
	var job subNexusSignupRewardJob
	if err := rows.Scan(&job.ID, &job.InviterID, &job.InviteeUserID, &job.InviterAmount, &job.InviteeAmount, &job.ClientIP, &job.IPLimit, &job.IPDailyLimit, &job.Status, &job.AttemptCount); err != nil {
		return nil, err
	}
	return &job, nil
}

func (r *affiliateRepository) findDueSignupRewardJob(ctx context.Context) (int64, error) {
	rows, err := r.client.QueryContext(ctx, `
SELECT id
FROM subnexus_affiliate_signup_reward_jobs
WHERE status = 'pending' AND next_attempt_at <= NOW()
ORDER BY next_attempt_at, id
LIMIT 1`)

	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return 0, err
		}
		return 0, sql.ErrNoRows
	}
	var id int64
	if err := rows.Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

func querySubNexusInviteSignupRewardGateTx(ctx context.Context, client affiliateQueryExecer) (bool, error) {
	rows, err := client.QueryContext(ctx, `SELECT value FROM settings WHERE key = $1 FOR UPDATE`, service.SettingKeySubNexusInviteRewardsEnabled)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return false, err
		}
		return false, nil
	}
	var raw string
	if err := rows.Scan(&raw); err != nil {
		return false, err
	}
	return raw == "true", nil
}

func subNexusSignupRewardRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := subNexusSignupRewardRetryBase
	for i := 1; i < attempt && delay < subNexusSignupRewardRetryMaxBackoff; i++ {
		delay *= 2
		if delay >= subNexusSignupRewardRetryMaxBackoff {
			return subNexusSignupRewardRetryMaxBackoff
		}
	}
	return delay
}

func truncateSignupRewardError(err error) string {
	if err == nil {
		return ""
	}
	text := strings.TrimSpace(err.Error())
	const maxLength = 2000
	if len(text) > maxLength {
		return text[:maxLength]
	}
	return text
}

func appendUniqueInt64(values []int64, additions ...int64) []int64 {
	for _, value := range additions {
		if value <= 0 {
			continue
		}
		found := false
		for _, existing := range values {
			if existing == value {
				found = true
				break
			}
		}
		if !found {
			values = append(values, value)
		}
	}
	return values
}

// withSubNexusSignupRewardTx isolates the optional reward work from a caller's
// existing transaction. PostgreSQL keeps a transaction aborted after any SQL
// error until it is rolled back; the savepoint lets registration continue when
// the service deliberately treats a reward failure as non-fatal.
func (r *affiliateRepository) withSubNexusSignupRewardTx(ctx context.Context, fn func(txCtx context.Context, txClient *dbent.Client) error) error {
	tx := dbent.TxFromContext(ctx)
	if tx == nil {
		return r.withTx(ctx, fn)
	}

	txClient := tx.Client()
	if _, err := txClient.ExecContext(ctx, "SAVEPOINT "+subNexusSignupRewardSavepoint); err != nil {
		return fmt.Errorf("create SubNexus signup reward savepoint: %w", err)
	}
	if err := fn(ctx, txClient); err != nil {
		if _, rollbackErr := txClient.ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+subNexusSignupRewardSavepoint); rollbackErr != nil {
			return fmt.Errorf("SubNexus signup reward failed: %w; rollback savepoint: %v", err, rollbackErr)
		}
		if _, releaseErr := txClient.ExecContext(ctx, "RELEASE SAVEPOINT "+subNexusSignupRewardSavepoint); releaseErr != nil {
			return fmt.Errorf("SubNexus signup reward failed: %w; release savepoint: %v", err, releaseErr)
		}
		return err
	}
	if _, err := txClient.ExecContext(ctx, "RELEASE SAVEPOINT "+subNexusSignupRewardSavepoint); err != nil {
		return fmt.Errorf("release SubNexus signup reward savepoint: %w", err)
	}
	return nil
}

func querySubNexusSignupRewardBindingExists(ctx context.Context, client affiliateQueryExecer, inviterID, inviteeUserID int64) (bool, error) {
	rows, err := client.QueryContext(ctx, `
SELECT EXISTS (
    SELECT 1
    FROM user_affiliates
    WHERE user_id = $1
      AND inviter_id = $2
)`, inviteeUserID, inviterID)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return false, err
		}
		return false, nil
	}
	var exists bool
	if err := rows.Scan(&exists); err != nil {
		return false, err
	}
	return exists, rows.Close()
}

func skippedSignupReward(reason string) service.AffiliateSignupRewardResult {
	return service.AffiliateSignupRewardResult{Skipped: true, SkipReason: reason}
}

func validSignupRewardAmount(value float64) bool {
	return value >= 0 && value <= service.SubNexusInviteSignupRewardAmountMax && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func (r *affiliateRepository) AccrueQuota(ctx context.Context, inviterID, inviteeUserID int64, amount float64, freezeHours int, sourceOrderID *int64) (bool, error) {
	if amount <= 0 {
		return false, nil
	}

	var applied bool
	err := r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		// The service performs an inexpensive gate check before entering this
		// transaction, but that read is not a concurrency boundary.  Lock the
		// canonical Affiliate switch here, before touching quota or the ledger,
		// so a concurrent disable is linearized either before this mutation or
		// after the whole transaction commits.  Missing/malformed values fail
		// closed and preserve the upstream default for a partially migrated DB.
		affiliateEnabled, gateErr := queryAffiliateEnabledTx(txCtx, txClient)
		if gateErr != nil {
			return fmt.Errorf("verify affiliate gate: %w", gateErr)
		}
		if !affiliateEnabled {
			applied = false
			return nil
		}

		// freezeHours > 0: add to frozen quota; == 0: add to available quota directly
		var updateSQL string
		if freezeHours > 0 {
			updateSQL = "UPDATE user_affiliates SET aff_frozen_quota = aff_frozen_quota + $1, aff_history_quota = aff_history_quota + $1, updated_at = NOW() WHERE user_id = $2"
		} else {
			updateSQL = "UPDATE user_affiliates SET aff_quota = aff_quota + $1, aff_history_quota = aff_history_quota + $1, updated_at = NOW() WHERE user_id = $2"
		}
		res, err := txClient.ExecContext(txCtx, updateSQL, amount, inviterID)
		if err != nil {
			return err
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			applied = false
			return nil
		}

		if freezeHours > 0 {
			if _, err = txClient.ExecContext(txCtx, `
INSERT INTO user_affiliate_ledger (user_id, action, amount, source_user_id, source_order_id, frozen_until, created_at, updated_at)
VALUES ($1, 'accrue', $2, $3, $4, NOW() + make_interval(hours => $5), NOW(), NOW())`,
				inviterID, amount, inviteeUserID, nullableInt64Arg(sourceOrderID), freezeHours); err != nil {
				return fmt.Errorf("insert affiliate accrue ledger: %w", err)
			}
		} else {
			if _, err = txClient.ExecContext(txCtx, `
INSERT INTO user_affiliate_ledger (user_id, action, amount, source_user_id, source_order_id, created_at, updated_at)
VALUES ($1, 'accrue', $2, $3, $4, NOW(), NOW())`, inviterID, amount, inviteeUserID, nullableInt64Arg(sourceOrderID)); err != nil {
				return fmt.Errorf("insert affiliate accrue ledger: %w", err)
			}
		}

		applied = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return applied, nil
}

// queryAffiliateEnabledTx reads the canonical Affiliate rollout switch from
// the transaction that will perform the quota mutation.  A missing row is a
// deliberate disabled result; a query failure is returned so callers fail
// closed rather than crediting a rebate they could not authorize.
func queryAffiliateEnabledTx(ctx context.Context, client affiliateQueryExecer) (bool, error) {
	if client == nil {
		return false, errors.New("affiliate gate client unavailable")
	}
	rows, err := client.QueryContext(ctx,
		`SELECT value FROM settings WHERE key = $1 FOR UPDATE`,
		service.SettingKeyAffiliateEnabled,
	)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return false, err
		}
		return false, nil
	}
	var raw string
	if err := rows.Scan(&raw); err != nil {
		return false, err
	}
	if err := rows.Close(); err != nil {
		return false, err
	}
	return raw == "true", nil
}

func (r *affiliateRepository) GetAccruedRebateFromInvitee(ctx context.Context, inviterID, inviteeUserID int64) (float64, error) {
	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx,
		`SELECT COALESCE(SUM(amount), 0)::double precision FROM user_affiliate_ledger WHERE user_id = $1 AND source_user_id = $2 AND action = 'accrue'`,
		inviterID, inviteeUserID)
	if err != nil {
		return 0, fmt.Errorf("query accrued rebate from invitee: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var total float64
	if rows.Next() {
		if err := rows.Scan(&total); err != nil {
			return 0, err
		}
	}
	return total, rows.Close()
}

func (r *affiliateRepository) ThawFrozenQuota(ctx context.Context, userID int64) (float64, error) {
	var thawed float64
	err := r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		var err error
		thawed, err = thawFrozenQuotaTx(txCtx, txClient, userID)
		return err
	})
	return thawed, err
}

// thawFrozenQuotaTx moves matured frozen quota to available quota within an existing tx.
func thawFrozenQuotaTx(txCtx context.Context, txClient *dbent.Client, userID int64) (float64, error) {
	rows, err := txClient.QueryContext(txCtx, `
WITH matured AS (
    UPDATE user_affiliate_ledger
    SET frozen_until = NULL, updated_at = NOW()
    WHERE user_id = $1
      AND frozen_until IS NOT NULL
      AND frozen_until <= NOW()
    RETURNING amount
)
SELECT COALESCE(SUM(amount), 0) FROM matured`, userID)
	if err != nil {
		return 0, fmt.Errorf("thaw frozen quota: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var thawed float64
	if rows.Next() {
		if err := rows.Scan(&thawed); err != nil {
			return 0, err
		}
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if thawed <= 0 {
		return 0, nil
	}

	_, err = txClient.ExecContext(txCtx, `
UPDATE user_affiliates
SET aff_quota = aff_quota + $1,
    aff_frozen_quota = GREATEST(aff_frozen_quota - $1, 0),
    updated_at = NOW()
WHERE user_id = $2`, thawed, userID)
	if err != nil {
		return 0, fmt.Errorf("move thawed quota: %w", err)
	}
	return thawed, nil
}

func (r *affiliateRepository) TransferQuotaToBalance(ctx context.Context, userID int64) (float64, float64, error) {
	var transferred float64
	var newBalance float64

	err := r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		if _, err := ensureUserAffiliateWithClient(txCtx, txClient, userID); err != nil {
			return err
		}

		// Thaw any matured frozen quota before transfer.
		if _, err := thawFrozenQuotaTx(txCtx, txClient, userID); err != nil {
			return fmt.Errorf("thaw before transfer: %w", err)
		}

		rows, err := txClient.QueryContext(txCtx, `
WITH claimed AS (
	SELECT aff_quota::double precision AS amount
	FROM user_affiliates
	WHERE user_id = $1
	  AND aff_quota > 0
	FOR UPDATE
),
cleared AS (
	UPDATE user_affiliates ua
	SET aff_quota = 0,
	    updated_at = NOW()
	FROM claimed c
	WHERE ua.user_id = $1
	RETURNING c.amount
)
SELECT amount
FROM cleared`, userID)
		if err != nil {
			return fmt.Errorf("claim affiliate quota: %w", err)
		}

		if !rows.Next() {
			_ = rows.Close()
			if err := rows.Err(); err != nil {
				return err
			}
			return service.ErrAffiliateQuotaEmpty
		}
		if err := rows.Scan(&transferred); err != nil {
			_ = rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if transferred <= 0 {
			return service.ErrAffiliateQuotaEmpty
		}

		affected, err := txClient.User.Update().
			Where(user.IDEQ(userID)).
			AddBalance(transferred).
			AddTotalRecharged(transferred).
			Save(txCtx)
		if err != nil {
			return fmt.Errorf("credit user balance by affiliate quota: %w", err)
		}
		if affected == 0 {
			return service.ErrUserNotFound
		}

		newBalance, err = queryUserBalance(txCtx, txClient, userID)
		if err != nil {
			return err
		}

		snapshot, err := queryAffiliateTransferSnapshot(txCtx, txClient, userID)
		if err != nil {
			return err
		}

		if _, err = txClient.ExecContext(txCtx, `
INSERT INTO user_affiliate_ledger (
    user_id,
    action,
    amount,
    source_user_id,
    balance_after,
    aff_quota_after,
    aff_frozen_quota_after,
    aff_history_quota_after,
    created_at,
    updated_at
)
VALUES ($1, 'transfer', $2, NULL, $3, $4, $5, $6, NOW(), NOW())`,
			userID,
			transferred,
			snapshot.BalanceAfter,
			snapshot.AvailableQuotaAfter,
			snapshot.FrozenQuotaAfter,
			snapshot.HistoryQuotaAfter,
		); err != nil {
			return fmt.Errorf("insert affiliate transfer ledger: %w", err)
		}

		return nil
	})
	if err != nil {
		return 0, 0, err
	}

	return transferred, newBalance, nil
}

func (r *affiliateRepository) ListInvitees(ctx context.Context, inviterID int64, limit int) ([]service.AffiliateInvitee, error) {
	if limit <= 0 {
		limit = 100
	}
	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx, `
SELECT ua.user_id,
       COALESCE(u.email, ''),
       COALESCE(u.username, ''),
       ua.created_at,
       COALESCE(SUM(ual.amount), 0)::double precision AS total_rebate
FROM user_affiliates ua
LEFT JOIN users u ON u.id = ua.user_id
LEFT JOIN user_affiliate_ledger ual
       ON ual.user_id = $1
      AND ual.source_user_id = ua.user_id
      AND ual.action = 'accrue'
WHERE ua.inviter_id = $1
GROUP BY ua.user_id, u.email, u.username, ua.created_at
ORDER BY ua.created_at DESC
LIMIT $2`, inviterID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	invitees := make([]service.AffiliateInvitee, 0)
	for rows.Next() {
		var item service.AffiliateInvitee
		var createdAt time.Time
		if err := rows.Scan(&item.UserID, &item.Email, &item.Username, &createdAt, &item.TotalRebate); err != nil {
			return nil, err
		}
		item.CreatedAt = &createdAt
		invitees = append(invitees, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return invitees, nil
}

func (r *affiliateRepository) ListAffiliateInviteRecords(ctx context.Context, filter service.AffiliateRecordFilter) ([]service.AffiliateInviteRecord, int64, error) {
	client := clientFromContext(ctx, r.client)
	where, args := buildAffiliateRecordWhere(filter, "ua.created_at", []string{
		"inviter.email", "inviter.username", "invitee.email", "invitee.username",
		"ua.inviter_id::text", "ua.user_id::text", "inviter_aff.aff_code",
	})

	total, err := queryAffiliateRecordCount(ctx, client, `
SELECT COUNT(*)
FROM user_affiliates ua
JOIN users invitee ON invitee.id = ua.user_id
JOIN users inviter ON inviter.id = ua.inviter_id
JOIN user_affiliates inviter_aff ON inviter_aff.user_id = ua.inviter_id
`+where, args...)
	if err != nil {
		return nil, 0, err
	}

	orderBy := buildAffiliateRecordOrderBy(filter, map[string]string{
		"inviter":      "inviter.email",
		"invitee":      "invitee.email",
		"aff_code":     "inviter_aff.aff_code",
		"total_rebate": "total_rebate",
		"created_at":   "ua.created_at",
	}, "ua.created_at")
	args = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)
	rows, err := client.QueryContext(ctx, `
SELECT ua.inviter_id,
       COALESCE(inviter.email, ''),
       COALESCE(inviter.username, ''),
       ua.user_id,
       COALESCE(invitee.email, ''),
       COALESCE(invitee.username, ''),
       COALESCE(inviter_aff.aff_code, ''),
       COALESCE(SUM(ual.amount), 0)::double precision AS total_rebate,
       ua.created_at
FROM user_affiliates ua
JOIN users invitee ON invitee.id = ua.user_id
JOIN users inviter ON inviter.id = ua.inviter_id
JOIN user_affiliates inviter_aff ON inviter_aff.user_id = ua.inviter_id
LEFT JOIN user_affiliate_ledger ual
       ON ual.user_id = ua.inviter_id
      AND ual.source_user_id = ua.user_id
      AND ual.action = 'accrue'
`+where+`
GROUP BY ua.inviter_id, inviter.email, inviter.username, ua.user_id, invitee.email, invitee.username, inviter_aff.aff_code, ua.created_at
`+orderBy+`
LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]service.AffiliateInviteRecord, 0)
	for rows.Next() {
		var item service.AffiliateInviteRecord
		if err := rows.Scan(
			&item.InviterID,
			&item.InviterEmail,
			&item.InviterUsername,
			&item.InviteeID,
			&item.InviteeEmail,
			&item.InviteeUsername,
			&item.AffCode,
			&item.TotalRebate,
			&item.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *affiliateRepository) ListAffiliateRebateRecords(ctx context.Context, filter service.AffiliateRecordFilter) ([]service.AffiliateRebateRecord, int64, error) {
	client := clientFromContext(ctx, r.client)
	where, args := buildAffiliateRecordWhere(filter, "ual.created_at", []string{
		"inviter.email", "inviter.username", "invitee.email", "invitee.username",
		"po.id::text", "po.out_trade_no", "po.payment_type", "po.status",
	})
	baseJoin := `
FROM user_affiliate_ledger ual
JOIN payment_orders po ON po.id = ual.source_order_id
JOIN users invitee ON invitee.id = ual.source_user_id
JOIN users inviter ON inviter.id = ual.user_id
WHERE ual.action = 'accrue'
  AND ual.source_order_id IS NOT NULL`
	if where != "" {
		where = strings.Replace(where, "WHERE ", " AND ", 1)
	}

	total, err := queryAffiliateRecordCount(ctx, client, "SELECT COUNT(*) "+baseJoin+where, args...)
	if err != nil {
		return nil, 0, err
	}

	orderBy := buildAffiliateRecordOrderBy(filter, map[string]string{
		"order":         "po.id",
		"inviter":       "inviter.email",
		"invitee":       "invitee.email",
		"order_amount":  "po.amount",
		"pay_amount":    "po.pay_amount",
		"rebate_amount": "ual.amount",
		"payment_type":  "po.payment_type",
		"order_status":  "po.status",
		"created_at":    "ual.created_at",
	}, "ual.created_at")
	args = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)
	rows, err := client.QueryContext(ctx, `
SELECT po.id,
       po.out_trade_no,
       ual.user_id,
       COALESCE(inviter.email, ''),
       COALESCE(inviter.username, ''),
       ual.source_user_id,
       COALESCE(invitee.email, ''),
       COALESCE(invitee.username, ''),
       po.amount::double precision,
       po.pay_amount::double precision,
       ual.amount::double precision,
       po.payment_type,
       po.status,
       ual.created_at
`+baseJoin+where+`
`+orderBy+`
LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]service.AffiliateRebateRecord, 0)
	for rows.Next() {
		var item service.AffiliateRebateRecord
		if err := rows.Scan(
			&item.OrderID,
			&item.OutTradeNo,
			&item.InviterID,
			&item.InviterEmail,
			&item.InviterUsername,
			&item.InviteeID,
			&item.InviteeEmail,
			&item.InviteeUsername,
			&item.OrderAmount,
			&item.PayAmount,
			&item.RebateAmount,
			&item.PaymentType,
			&item.OrderStatus,
			&item.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *affiliateRepository) ListAffiliateTransferRecords(ctx context.Context, filter service.AffiliateRecordFilter) ([]service.AffiliateTransferRecord, int64, error) {
	client := clientFromContext(ctx, r.client)
	where, args := buildAffiliateRecordWhere(filter, "ual.created_at", []string{
		"u.email", "u.username", "u.id::text",
	})
	baseJoin := `
FROM user_affiliate_ledger ual
JOIN users u ON u.id = ual.user_id
WHERE ual.action = 'transfer'`
	if where != "" {
		where = strings.Replace(where, "WHERE ", " AND ", 1)
	}

	total, err := queryAffiliateRecordCount(ctx, client, "SELECT COUNT(*) "+baseJoin+where, args...)
	if err != nil {
		return nil, 0, err
	}

	orderBy := buildAffiliateRecordOrderBy(filter, map[string]string{
		"user":                  "u.email",
		"amount":                "ual.amount",
		"balance_after":         "ual.balance_after",
		"available_quota_after": "ual.aff_quota_after",
		"frozen_quota_after":    "ual.aff_frozen_quota_after",
		"history_quota_after":   "ual.aff_history_quota_after",
		"created_at":            "ual.created_at",
	}, "ual.created_at")
	args = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)
	rows, err := client.QueryContext(ctx, `
SELECT ual.id,
       ual.user_id,
       COALESCE(u.email, ''),
       COALESCE(u.username, ''),
       ual.amount::double precision,
       ual.balance_after::double precision,
       ual.aff_quota_after::double precision,
       ual.aff_frozen_quota_after::double precision,
       ual.aff_history_quota_after::double precision,
       ual.created_at
`+baseJoin+where+`
`+orderBy+`
LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]service.AffiliateTransferRecord, 0)
	for rows.Next() {
		var item service.AffiliateTransferRecord
		var balanceAfter sql.NullFloat64
		var availableQuotaAfter sql.NullFloat64
		var frozenQuotaAfter sql.NullFloat64
		var historyQuotaAfter sql.NullFloat64
		if err := rows.Scan(
			&item.LedgerID,
			&item.UserID,
			&item.UserEmail,
			&item.Username,
			&item.Amount,
			&balanceAfter,
			&availableQuotaAfter,
			&frozenQuotaAfter,
			&historyQuotaAfter,
			&item.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		item.BalanceAfter = nullableFloat64Ptr(balanceAfter)
		item.AvailableQuotaAfter = nullableFloat64Ptr(availableQuotaAfter)
		item.FrozenQuotaAfter = nullableFloat64Ptr(frozenQuotaAfter)
		item.HistoryQuotaAfter = nullableFloat64Ptr(historyQuotaAfter)
		item.SnapshotAvailable = balanceAfter.Valid &&
			availableQuotaAfter.Valid &&
			frozenQuotaAfter.Valid &&
			historyQuotaAfter.Valid
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *affiliateRepository) GetAffiliateUserOverview(ctx context.Context, userID int64) (*service.AffiliateUserOverview, error) {
	if userID <= 0 {
		return nil, service.ErrUserNotFound
	}
	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx, affiliateUserOverviewSQL, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, service.ErrUserNotFound
	}

	var overview service.AffiliateUserOverview
	var customRate float64
	var hasCustomRate bool
	if err := rows.Scan(
		&overview.UserID,
		&overview.Email,
		&overview.Username,
		&overview.AffCode,
		&customRate,
		&hasCustomRate,
		&overview.InvitedCount,
		&overview.RebatedInviteeCount,
		&overview.AvailableQuota,
		&overview.HistoryQuota,
	); err != nil {
		return nil, err
	}
	if hasCustomRate {
		overview.RebateRatePercent = customRate
		overview.RebateRateCustom = true
	}
	return &overview, rows.Err()
}

func buildAffiliateRecordWhere(filter service.AffiliateRecordFilter, timeColumn string, searchColumns []string) (string, []any) {
	clauses := make([]string, 0, 3)
	args := make([]any, 0, 3)
	if filter.StartAt != nil {
		args = append(args, *filter.StartAt)
		clauses = append(clauses, fmt.Sprintf("%s >= $%d", timeColumn, len(args)))
	}
	if filter.EndAt != nil {
		args = append(args, *filter.EndAt)
		clauses = append(clauses, fmt.Sprintf("%s <= $%d", timeColumn, len(args)))
	}
	search := strings.TrimSpace(filter.Search)
	if search != "" && len(searchColumns) > 0 {
		args = append(args, "%"+strings.ToLower(search)+"%")
		parts := make([]string, 0, len(searchColumns))
		for _, col := range searchColumns {
			parts = append(parts, fmt.Sprintf("LOWER(%s) LIKE $%d", col, len(args)))
		}
		clauses = append(clauses, "("+strings.Join(parts, " OR ")+")")
	}
	if len(clauses) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

func buildAffiliateRecordOrderBy(filter service.AffiliateRecordFilter, sortColumns map[string]string, fallbackColumn string) string {
	column := sortColumns[filter.SortBy]
	if column == "" {
		column = fallbackColumn
	}
	direction := "DESC"
	if !filter.SortDesc {
		direction = "ASC"
	}
	return "ORDER BY " + column + " " + direction + " NULLS LAST"
}

func queryAffiliateRecordCount(ctx context.Context, client affiliateQueryExecer, query string, args ...any) (int64, error) {
	rows, err := client.QueryContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return 0, rows.Err()
	}
	var total int64
	if err := rows.Scan(&total); err != nil {
		return 0, err
	}
	return total, rows.Err()
}

func (r *affiliateRepository) withTx(ctx context.Context, fn func(txCtx context.Context, txClient *dbent.Client) error) error {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		return fn(ctx, tx.Client())
	}

	tx, err := r.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin affiliate transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	txCtx := dbent.NewTxContext(ctx, tx)
	if err := fn(txCtx, tx.Client()); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit affiliate transaction: %w", err)
	}
	return nil
}

func ensureUserAffiliateWithClient(ctx context.Context, client affiliateQueryExecer, userID int64) (*service.AffiliateSummary, error) {
	summary, err := queryAffiliateByUserID(ctx, client, userID)
	if err == nil {
		return summary, nil
	}
	if !errors.Is(err, service.ErrAffiliateProfileNotFound) {
		return nil, err
	}

	for i := 0; i < affiliateCodeMaxAttempts; i++ {
		code, codeErr := generateAffiliateCode()
		if codeErr != nil {
			return nil, codeErr
		}
		_, insertErr := client.ExecContext(ctx, `
INSERT INTO user_affiliates (user_id, aff_code, created_at, updated_at)
VALUES ($1, $2, NOW(), NOW())
ON CONFLICT (user_id) DO NOTHING`, userID, code)
		if insertErr == nil {
			break
		}
		if isAffiliateUniqueViolation(insertErr) {
			continue
		}
		return nil, insertErr
	}

	return queryAffiliateByUserID(ctx, client, userID)
}

func queryAffiliateByUserID(ctx context.Context, client affiliateQueryExecer, userID int64) (*service.AffiliateSummary, error) {
	rows, err := client.QueryContext(ctx, `
SELECT user_id,
       aff_code,
       aff_code_custom,
       aff_rebate_rate_percent,
       inviter_id,
       aff_count,
       aff_quota::double precision,
       aff_frozen_quota::double precision,
       aff_history_quota::double precision,
       created_at,
       updated_at
FROM user_affiliates
WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, service.ErrAffiliateProfileNotFound
	}

	var out service.AffiliateSummary
	var inviterID sql.NullInt64
	var rebateRate sql.NullFloat64
	if err := rows.Scan(
		&out.UserID,
		&out.AffCode,
		&out.AffCodeCustom,
		&rebateRate,
		&inviterID,
		&out.AffCount,
		&out.AffQuota,
		&out.AffFrozenQuota,
		&out.AffHistoryQuota,
		&out.CreatedAt,
		&out.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if inviterID.Valid {
		out.InviterID = &inviterID.Int64
	}
	if rebateRate.Valid {
		v := rebateRate.Float64
		out.AffRebateRatePercent = &v
	}
	return &out, nil
}

func queryAffiliateByCode(ctx context.Context, client affiliateQueryExecer, code string) (*service.AffiliateSummary, error) {
	rows, err := client.QueryContext(ctx, `
SELECT user_id,
       aff_code,
       aff_code_custom,
       aff_rebate_rate_percent,
       inviter_id,
       aff_count,
       aff_quota::double precision,
       aff_frozen_quota::double precision,
       aff_history_quota::double precision,
       created_at,
       updated_at
FROM user_affiliates
WHERE aff_code = $1
LIMIT 1`, strings.ToUpper(strings.TrimSpace(code)))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, service.ErrAffiliateProfileNotFound
	}

	var out service.AffiliateSummary
	var inviterID sql.NullInt64
	var rebateRate sql.NullFloat64
	if err := rows.Scan(
		&out.UserID,
		&out.AffCode,
		&out.AffCodeCustom,
		&rebateRate,
		&inviterID,
		&out.AffCount,
		&out.AffQuota,
		&out.AffFrozenQuota,
		&out.AffHistoryQuota,
		&out.CreatedAt,
		&out.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if inviterID.Valid {
		out.InviterID = &inviterID.Int64
	}
	if rebateRate.Valid {
		v := rebateRate.Float64
		out.AffRebateRatePercent = &v
	}
	return &out, nil
}

func queryUserBalance(ctx context.Context, client affiliateQueryExecer, userID int64) (float64, error) {
	rows, err := client.QueryContext(ctx,
		"SELECT balance::double precision FROM users WHERE id = $1 LIMIT 1",
		userID,
	)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return 0, err
		}
		return 0, service.ErrUserNotFound
	}
	var balance float64
	if err := rows.Scan(&balance); err != nil {
		return 0, err
	}
	return balance, nil
}

// querySubNexusSignupRewardExists checks the idempotency marker for a newly
// registered invitee. Both recipient rows carry source_user_id, so checking
// either action is sufficient even when only one side has a non-zero reward.
func querySubNexusSignupRewardExists(ctx context.Context, client affiliateQueryExecer, inviteeUserID int64) (bool, error) {
	rows, err := client.QueryContext(ctx, `
SELECT EXISTS (
    SELECT 1
    FROM user_affiliate_ledger
    WHERE source_user_id = $1
      AND action IN ('signup_bonus_inviter', 'signup_bonus_invitee')
)`, inviteeUserID)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return false, err
		}
		return false, nil
	}
	var exists bool
	if err := rows.Scan(&exists); err != nil {
		return false, err
	}
	return exists, rows.Close()
}

// querySubNexusSignupRewardIPDailyCount counts distinct invitees rewarded by
// one client IP since the database's current date. Counting invitees (rather
// than ledger rows) keeps inviter+invitee pairs from consuming two slots.
func querySubNexusSignupRewardIPDailyCount(ctx context.Context, client affiliateQueryExecer, clientIP string) (int, error) {
	rows, err := client.QueryContext(ctx, `
SELECT COUNT(DISTINCT source_user_id)
FROM user_affiliate_ledger
WHERE action IN ('signup_bonus_inviter', 'signup_bonus_invitee')
  AND source_ip = $1
  AND created_at >= CURRENT_DATE`, clientIP)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return 0, err
		}
		return 0, nil
	}
	var count int
	if err := rows.Scan(&count); err != nil {
		return 0, err
	}
	return count, rows.Close()
}

// grantSubNexusSignupBalanceTx updates a recipient and records the resulting
// balance in the same transaction as the idempotency checks. A failed ledger
// insert rolls the balance update back through the caller's transaction.
func grantSubNexusSignupBalanceTx(ctx context.Context, client *dbent.Client, userID int64, amount float64, action string, sourceUserID int64, clientIP string) error {
	affected, err := client.User.Update().
		Where(user.IDEQ(userID)).
		AddBalance(amount).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("grant SubNexus signup reward balance: %w", err)
	}
	if affected == 0 {
		return service.ErrUserNotFound
	}

	balanceAfter, err := queryUserBalance(ctx, client, userID)
	if err != nil {
		return err
	}
	if _, err = client.ExecContext(ctx, `
INSERT INTO user_affiliate_ledger (
    user_id,
    action,
    amount,
    source_user_id,
    source_ip,
    balance_after,
    created_at,
    updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())`,
		userID,
		action,
		amount,
		sourceUserID,
		clientIP,
		balanceAfter,
	); err != nil {
		return fmt.Errorf("insert SubNexus signup reward ledger: %w", err)
	}
	return nil
}

type affiliateTransferSnapshot struct {
	BalanceAfter        float64
	AvailableQuotaAfter float64
	FrozenQuotaAfter    float64
	HistoryQuotaAfter   float64
}

func queryAffiliateTransferSnapshot(ctx context.Context, client affiliateQueryExecer, userID int64) (*affiliateTransferSnapshot, error) {
	rows, err := client.QueryContext(ctx, `
SELECT u.balance::double precision,
       ua.aff_quota::double precision,
       ua.aff_frozen_quota::double precision,
       ua.aff_history_quota::double precision
FROM users u
JOIN user_affiliates ua ON ua.user_id = u.id
WHERE u.id = $1
LIMIT 1`, userID)
	if err != nil {
		return nil, fmt.Errorf("query affiliate transfer snapshot: %w", err)
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, service.ErrUserNotFound
	}

	var snapshot affiliateTransferSnapshot
	if err := rows.Scan(
		&snapshot.BalanceAfter,
		&snapshot.AvailableQuotaAfter,
		&snapshot.FrozenQuotaAfter,
		&snapshot.HistoryQuotaAfter,
	); err != nil {
		return nil, err
	}
	return &snapshot, rows.Err()
}

func nullableFloat64Ptr(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	return &v.Float64
}

func generateAffiliateCode() (string, error) {
	buf := make([]byte, affiliateCodeLength)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate affiliate code: %w", err)
	}
	for i := range buf {
		buf[i] = affiliateCodeCharset[int(buf[i])%len(affiliateCodeCharset)]
	}
	return string(buf), nil
}

func isAffiliateUniqueViolation(err error) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return string(pqErr.Code) == "23505"
	}
	return false
}

// UpdateUserAffCode 改写用户的邀请码（自定义专属邀请码）。
// 唯一性冲突返回 ErrAffiliateCodeTaken。
func (r *affiliateRepository) UpdateUserAffCode(ctx context.Context, userID int64, newCode string) error {
	if userID <= 0 {
		return service.ErrUserNotFound
	}
	code := strings.ToUpper(strings.TrimSpace(newCode))
	if code == "" {
		return service.ErrAffiliateCodeInvalid
	}

	return r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		if _, err := ensureUserAffiliateWithClient(txCtx, txClient, userID); err != nil {
			return err
		}
		res, err := txClient.ExecContext(txCtx, `
UPDATE user_affiliates
SET aff_code = $1,
    aff_code_custom = true,
    updated_at = NOW()
WHERE user_id = $2`, code, userID)
		if err != nil {
			if isAffiliateUniqueViolation(err) {
				return service.ErrAffiliateCodeTaken
			}
			return fmt.Errorf("update aff_code: %w", err)
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			return service.ErrUserNotFound
		}
		return nil
	})
}

// ResetUserAffCode 把 aff_code 还原为系统随机码，并清除 aff_code_custom 标记。
func (r *affiliateRepository) ResetUserAffCode(ctx context.Context, userID int64) (string, error) {
	if userID <= 0 {
		return "", service.ErrUserNotFound
	}
	var newCode string
	err := r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		if _, err := ensureUserAffiliateWithClient(txCtx, txClient, userID); err != nil {
			return err
		}
		for i := 0; i < affiliateCodeMaxAttempts; i++ {
			candidate, codeErr := generateAffiliateCode()
			if codeErr != nil {
				return codeErr
			}
			res, err := txClient.ExecContext(txCtx, `
UPDATE user_affiliates
SET aff_code = $1,
    aff_code_custom = false,
    updated_at = NOW()
WHERE user_id = $2`, candidate, userID)
			if err != nil {
				if isAffiliateUniqueViolation(err) {
					continue
				}
				return fmt.Errorf("reset aff_code: %w", err)
			}
			affected, _ := res.RowsAffected()
			if affected == 0 {
				return service.ErrUserNotFound
			}
			newCode = candidate
			return nil
		}
		return fmt.Errorf("reset aff_code: exhausted attempts")
	})
	if err != nil {
		return "", err
	}
	return newCode, nil
}

// SetUserRebateRate 设置或清除用户专属返利比例。ratePercent==nil 表示清除（沿用全局）。
func (r *affiliateRepository) SetUserRebateRate(ctx context.Context, userID int64, ratePercent *float64) error {
	if userID <= 0 {
		return service.ErrUserNotFound
	}
	return r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		if _, err := ensureUserAffiliateWithClient(txCtx, txClient, userID); err != nil {
			return err
		}
		// nullableArg lets us use a single UPDATE for both "set value" and
		// "clear" cases — database/sql converts nil interface{} to SQL NULL.
		res, err := txClient.ExecContext(txCtx, `
UPDATE user_affiliates
SET aff_rebate_rate_percent = $1,
    updated_at = NOW()
WHERE user_id = $2`, nullableArg(ratePercent), userID)
		if err != nil {
			return fmt.Errorf("set aff_rebate_rate_percent: %w", err)
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			return service.ErrUserNotFound
		}
		return nil
	})
}

// BatchSetUserRebateRate 批量为多个用户设置专属比例（nil 清除）。
func (r *affiliateRepository) BatchSetUserRebateRate(ctx context.Context, userIDs []int64, ratePercent *float64) error {
	if len(userIDs) == 0 {
		return nil
	}
	return r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		for _, uid := range userIDs {
			if uid <= 0 {
				continue
			}
			if _, err := ensureUserAffiliateWithClient(txCtx, txClient, uid); err != nil {
				return err
			}
		}
		_, err := txClient.ExecContext(txCtx, `
UPDATE user_affiliates
SET aff_rebate_rate_percent = $1,
    updated_at = NOW()
WHERE user_id = ANY($2)`, nullableArg(ratePercent), pq.Array(userIDs))
		if err != nil {
			return fmt.Errorf("batch set aff_rebate_rate_percent: %w", err)
		}
		return nil
	})
}

// nullableArg unwraps a *float64 into an interface{} suitable for SQL parameter
// binding: nil pointer → SQL NULL, non-nil → the float value.
func nullableArg(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullableInt64Arg(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

// ListUsersWithCustomSettings 列出有专属配置（自定义码或专属比例）的用户。
//
// 单一查询同时处理"无搜索"与"按邮箱/用户名模糊搜索"：
// 空 search 时拼接出的 LIKE 模式为 "%%"，匹配所有行；非空时按 ILIKE 子串匹配。
// 这避免了为两种情况维护两份 SQL 模板。
func (r *affiliateRepository) ListUsersWithCustomSettings(ctx context.Context, filter service.AffiliateAdminFilter) ([]service.AffiliateAdminEntry, int64, error) {
	page := filter.Page
	if page < 1 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize
	likePattern := "%" + strings.TrimSpace(filter.Search) + "%"

	const baseFrom = `
FROM user_affiliates ua
JOIN users u ON u.id = ua.user_id
WHERE (ua.aff_code_custom = true OR ua.aff_rebate_rate_percent IS NOT NULL)
  AND (u.email ILIKE $1 OR u.username ILIKE $1)`

	client := clientFromContext(ctx, r.client)

	total, err := scanInt64(ctx, client, "SELECT COUNT(*)"+baseFrom, likePattern)
	if err != nil {
		return nil, 0, fmt.Errorf("count affiliate admin entries: %w", err)
	}

	listQuery := `
SELECT ua.user_id,
       COALESCE(u.email, ''),
       COALESCE(u.username, ''),
       ua.aff_code,
       ua.aff_code_custom,
       ua.aff_rebate_rate_percent,
       ua.aff_count` + baseFrom + `
ORDER BY ua.updated_at DESC
LIMIT $2 OFFSET $3`

	rows, err := client.QueryContext(ctx, listQuery, likePattern, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list affiliate admin entries: %w", err)
	}
	defer func() { _ = rows.Close() }()

	entries := make([]service.AffiliateAdminEntry, 0)
	for rows.Next() {
		var e service.AffiliateAdminEntry
		var rebate sql.NullFloat64
		if err := rows.Scan(&e.UserID, &e.Email, &e.Username, &e.AffCode,
			&e.AffCodeCustom, &rebate, &e.AffCount); err != nil {
			return nil, 0, err
		}
		if rebate.Valid {
			v := rebate.Float64
			e.AffRebateRatePercent = &v
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return entries, total, nil
}

// scanInt64 runs a query expected to return a single int64 column (e.g. COUNT).
func scanInt64(ctx context.Context, client affiliateQueryExecer, query string, args ...any) (int64, error) {
	rows, err := client.QueryContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return 0, err
		}
		return 0, nil
	}
	var v int64
	if err := rows.Scan(&v); err != nil {
		return 0, err
	}
	return v, nil
}
