package media

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

// The journal survives cache loss. Even terminal task IDs are retained so an
// old idempotency key cannot create and charge a second upstream video.
type mediaTaskStore struct{ db *sql.DB }

var _ DurableMediaTaskStore = (*mediaTaskStore)(nil)

func NewMediaTaskStore(db *sql.DB) MediaTaskStore { return &mediaTaskStore{db: db} }

func (s *mediaTaskStore) CreateTask(ctx context.Context, task *MediaTaskRecord) (*MediaTaskRecord, bool, error) {
	if s == nil || s.db == nil || !validJournalTask(task) {
		return nil, false, ErrMediaTaskUnavailable
	}
	// Replays must remain available even if the key's quota has since been spent.
	existing, err := s.GetTask(ctx, task.ID)
	if err == nil {
		return compareJournalTask(existing, task)
	}
	if !errors.Is(err, ErrMediaTaskNotFound) {
		return nil, false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	// Serialize new async reservations for a key without changing its ledger.
	// Quota checking must be a separate statement after acquiring this lock, so
	// READ COMMITTED sees any task committed while waiting for the row lock.
	var keyID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM api_keys
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL FOR UPDATE`, task.APIKeyID, task.UserID).Scan(&keyID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, ErrMediaTaskUnavailable
	}
	if err != nil {
		return nil, false, err
	}
	var stored []byte
	err = tx.QueryRowContext(ctx, `SELECT record FROM subnexus_seedance_tasks WHERE task_id = $1`, task.ID).Scan(&stored)
	if err == nil {
		existing, decodeErr := decodeMediaTask(stored)
		if decodeErr != nil {
			return nil, false, decodeErr
		}
		return compareJournalTask(existing, task)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, false, err
	}
	var quotaExceeded, rateLimited, expired, quotaEnabled, rateEnabled bool
	var keyStatus string
	err = tx.QueryRowContext(ctx, `SELECT quota > 0 AND quota_used + pending.amount > quota,
		(rate_limit_5h > 0 AND (CASE WHEN window_5h_start IS NULL OR window_5h_start + INTERVAL '5 hours' <= clock_timestamp()
			THEN 0 ELSE usage_5h END) + pending.amount > rate_limit_5h)
		OR (rate_limit_1d > 0 AND (CASE WHEN window_1d_start IS NULL OR window_1d_start + INTERVAL '24 hours' <= clock_timestamp()
			THEN 0 ELSE usage_1d END) + pending.amount > rate_limit_1d)
		OR (rate_limit_7d > 0 AND (CASE WHEN window_7d_start IS NULL OR window_7d_start + INTERVAL '7 days' <= clock_timestamp()
			THEN 0 ELSE usage_7d END) + pending.amount > rate_limit_7d),
		status, expires_at IS NOT NULL AND expires_at <= clock_timestamp(),
		quota > 0, rate_limit_5h > 0 OR rate_limit_1d > 0 OR rate_limit_7d > 0
		FROM api_keys CROSS JOIN (
			SELECT COALESCE(SUM(unit_price), 0) + $2::numeric AS amount FROM subnexus_seedance_tasks
			WHERE api_key_id = $1 AND phase NOT IN ('settled', 'released', 'rejected')
		) pending WHERE id = $1`, task.APIKeyID, task.UnitPrice).
		Scan(&quotaExceeded, &rateLimited, &keyStatus, &expired, &quotaEnabled, &rateEnabled)
	if err != nil {
		return nil, false, err
	}
	if keyStatus == "quota_exhausted" {
		return nil, false, ErrMediaKeyQuotaExceeded
	}
	if keyStatus != "active" || expired {
		return nil, false, ErrMediaKeyUnavailable
	}
	if quotaExceeded {
		return nil, false, ErrMediaKeyQuotaExceeded
	}
	if rateLimited {
		return nil, false, ErrMediaKeyRateLimited
	}
	// Auth may have returned an older cached snapshot. Freshly enabled limits
	// must still receive their usage charge when this asynchronous task settles.
	task.BillingQuota = task.BillingQuota || quotaEnabled
	task.BillingRateLimits = task.BillingRateLimits || rateEnabled
	data, err := json.Marshal(task)
	if err != nil {
		return nil, false, err
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO subnexus_seedance_tasks
		(task_id, user_id, api_key_id, request_hash, unit_price, phase, next_attempt_at, record)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb)
		ON CONFLICT (task_id) DO NOTHING RETURNING record`, task.ID, task.UserID, task.APIKeyID,
		task.RequestHash, task.UnitPrice, task.Phase, task.NextAttemptAt, string(data)).Scan(&stored)
	if errors.Is(err, sql.ErrNoRows) {
		// This is only expected for an ID collision with a different API key.
		err = tx.QueryRowContext(ctx, `SELECT record FROM subnexus_seedance_tasks WHERE task_id = $1`, task.ID).Scan(&stored)
		if err != nil {
			return nil, false, err
		}
		existing, decodeErr := decodeMediaTask(stored)
		if decodeErr != nil {
			return nil, false, decodeErr
		}
		return compareJournalTask(existing, task)
	}
	if err != nil {
		return nil, false, err
	}
	created, err := decodeMediaTask(stored)
	if err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return created, true, nil
}

func compareJournalTask(existing, requested *MediaTaskRecord) (*MediaTaskRecord, bool, error) {
	if existing.UserID != requested.UserID || existing.APIKeyID != requested.APIKeyID || existing.RequestHash != requested.RequestHash {
		return nil, false, ErrMediaIdempotencyConflict
	}
	return existing, false, nil
}

// SaveTask only updates previously journaled tasks. TTL is deliberately ignored:
// age does not prove that an uncertain upstream submission failed or was free.
func (s *mediaTaskStore) SaveTask(ctx context.Context, task *MediaTaskRecord, _ time.Duration) error {
	if s == nil || s.db == nil || !validJournalTask(task) {
		return ErrMediaTaskUnavailable
	}
	data, err := json.Marshal(task)
	if err != nil {
		return err
	}
	token, _ := ctx.Value(mediaLeaseContextKey{}).(string)
	result, err := s.db.ExecContext(ctx, `UPDATE subnexus_seedance_tasks
		SET record = $2::jsonb, phase = $3, next_attempt_at = $4, updated_at = clock_timestamp()
		WHERE task_id = $1 AND user_id = $5 AND api_key_id = $6 AND request_hash = $7 AND unit_price = $9
		  AND (phase NOT IN ('settled', 'released', 'rejected') OR phase = $3)
		  AND (($8 = '' AND (lease_until IS NULL OR lease_until <= clock_timestamp()))
		    OR ($8 <> '' AND lease_token = $8 AND lease_until > clock_timestamp()))`,
		task.ID, string(data), task.Phase, task.NextAttemptAt, task.UserID, task.APIKeyID, task.RequestHash, token, task.UnitPrice)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrMediaLeaseLost
	}
	return nil
}

func (s *mediaTaskStore) GetTask(ctx context.Context, id string) (*MediaTaskRecord, error) {
	if s == nil || s.db == nil {
		return nil, ErrMediaTaskUnavailable
	}
	var data []byte
	err := s.db.QueryRowContext(ctx, `SELECT record FROM subnexus_seedance_tasks WHERE task_id = $1`, strings.TrimSpace(id)).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrMediaTaskNotFound
	}
	if err != nil {
		return nil, err
	}
	return decodeMediaTask(data)
}

func (s *mediaTaskStore) AcquireTask(ctx context.Context, id, token string, ttl time.Duration) (bool, error) {
	if s == nil || s.db == nil || strings.TrimSpace(id) == "" || strings.TrimSpace(token) == "" || ttl < time.Millisecond {
		return false, ErrMediaTaskUnavailable
	}
	result, err := s.db.ExecContext(ctx, `UPDATE subnexus_seedance_tasks
		SET lease_token = $2, lease_until = clock_timestamp() + ($3 * interval '1 millisecond')
		WHERE task_id = $1 AND (lease_until IS NULL OR lease_until <= clock_timestamp())`, id, token, ttl.Milliseconds())
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

func (s *mediaTaskStore) ReleaseTask(ctx context.Context, id, token string) error {
	if s == nil || s.db == nil {
		return ErrMediaTaskUnavailable
	}
	if strings.TrimSpace(id) == "" || strings.TrimSpace(token) == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `UPDATE subnexus_seedance_tasks SET lease_token = '', lease_until = NULL
		WHERE task_id = $1 AND lease_token = $2`, id, token)
	return err
}

func (s *mediaTaskStore) ListRecoverableTasks(ctx context.Context, limit int) ([]*MediaTaskRecord, error) {
	if s == nil || s.db == nil {
		return nil, ErrMediaTaskUnavailable
	}
	rows, err := s.db.QueryContext(ctx, `SELECT record FROM subnexus_seedance_tasks
		WHERE phase NOT IN ('settled', 'released', 'rejected', 'manual_review')
		  AND next_attempt_at <= EXTRACT(EPOCH FROM clock_timestamp())::bigint
		  AND (lease_until IS NULL OR lease_until <= clock_timestamp())
		ORDER BY next_attempt_at, created_at, task_id LIMIT $1`, mediaStoreLimit(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks := make([]*MediaTaskRecord, 0)
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		task, err := decodeMediaTask(data)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (s *mediaTaskStore) SaveFile(ctx context.Context, file *MediaFileRecord, ttl time.Duration) error {
	if s == nil || s.db == nil || file == nil || strings.TrimSpace(file.ID) == "" || ttl <= 0 {
		return ErrMediaTaskUnavailable
	}
	data, err := json.Marshal(file)
	if err != nil {
		return err
	}
	expiresAt := time.Now().Add(ttl)
	if file.ExpiresAt > 0 && time.Unix(file.ExpiresAt, 0).Before(expiresAt) {
		expiresAt = time.Unix(file.ExpiresAt, 0)
	}
	// A reference ID can never be rebound to another account or upstream image.
	_, err = s.db.ExecContext(ctx, `INSERT INTO subnexus_seedance_files (file_id, record, expires_at)
		VALUES ($1, $2::jsonb, $3)`, file.ID, string(data), expiresAt)
	return err
}

func (s *mediaTaskStore) GetFile(ctx context.Context, id string) (*MediaFileRecord, error) {
	if s == nil || s.db == nil {
		return nil, ErrMediaTaskUnavailable
	}
	var data []byte
	err := s.db.QueryRowContext(ctx, `SELECT record FROM subnexus_seedance_files
		WHERE file_id = $1 AND expires_at > clock_timestamp()`, strings.TrimSpace(id)).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrMediaTaskNotFound
	}
	if err != nil {
		return nil, err
	}
	var file MediaFileRecord
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("decode media reference: %w", err)
	}
	return &file, nil
}

// CleanupExpiredFiles restores the delivery package's reference TTL behavior
// without deleting task journals, idempotency tombstones, or money records.
func (s *mediaTaskStore) CleanupExpiredFiles(ctx context.Context, limit int) (int64, error) {
	if s == nil || s.db == nil {
		return 0, ErrMediaTaskUnavailable
	}
	result, err := s.db.ExecContext(ctx, `WITH expired AS (
		SELECT file_id FROM subnexus_seedance_files WHERE expires_at <= clock_timestamp()
		ORDER BY expires_at, file_id LIMIT $1 FOR UPDATE SKIP LOCKED
	) DELETE FROM subnexus_seedance_files AS f USING expired WHERE f.file_id = expired.file_id`, mediaStoreLimit(limit))
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// Legacy settlement claims require an explicit release. Production lifecycle
// code instead uses fenced task leases, making process crashes recoverable.
func (s *mediaTaskStore) ClaimSettlement(ctx context.Context, taskID string, _ time.Duration) (bool, error) {
	if s == nil || s.db == nil || strings.TrimSpace(taskID) == "" {
		return false, ErrMediaTaskUnavailable
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO subnexus_seedance_settlement_claims (task_id) VALUES ($1)
		ON CONFLICT (task_id) DO NOTHING`, taskID)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

func (s *mediaTaskStore) ReleaseSettlement(ctx context.Context, taskID string) error {
	if s == nil || s.db == nil {
		return ErrMediaTaskUnavailable
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM subnexus_seedance_settlement_claims WHERE task_id = $1`, strings.TrimSpace(taskID))
	return err
}

// Pending holds never expire. DueAt schedules reconciliation, not an automatic
// refund: only a confirmed upstream failure/rejection permits a release.
func (s *mediaTaskStore) TrackPendingHold(ctx context.Context, hold *MediaPendingHold, _ time.Duration) error {
	if s == nil || s.db == nil {
		return ErrMediaTaskUnavailable
	}
	if hold == nil || strings.TrimSpace(hold.TaskID) == "" {
		return nil
	}
	data, err := json.Marshal(hold)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO subnexus_seedance_pending_holds (task_id, user_id, api_key_id, amount, due_at, record)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb)
		ON CONFLICT (task_id) DO UPDATE SET due_at = EXCLUDED.due_at, record = EXCLUDED.record
		WHERE subnexus_seedance_pending_holds.user_id = EXCLUDED.user_id
		  AND subnexus_seedance_pending_holds.api_key_id = EXCLUDED.api_key_id
		  AND subnexus_seedance_pending_holds.amount = EXCLUDED.amount`,
		hold.TaskID, hold.UserID, hold.APIKeyID, hold.Amount, hold.DueAt, string(data))
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrMediaIdempotencyConflict
	}
	return nil
}

func (s *mediaTaskStore) ListDuePendingHolds(ctx context.Context, now time.Time, limit int) ([]*MediaPendingHold, error) {
	if s == nil || s.db == nil {
		return nil, ErrMediaTaskUnavailable
	}
	rows, err := s.db.QueryContext(ctx, `SELECT record FROM subnexus_seedance_pending_holds
		WHERE due_at <= $1 ORDER BY due_at, task_id LIMIT $2`, now.Unix(), mediaStoreLimit(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	holds := make([]*MediaPendingHold, 0)
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var hold MediaPendingHold
		if err := json.Unmarshal(data, &hold); err != nil {
			return nil, fmt.Errorf("decode pending media hold: %w", err)
		}
		holds = append(holds, &hold)
	}
	return holds, rows.Err()
}

func (s *mediaTaskStore) DeletePendingHold(ctx context.Context, taskID string) error {
	if s == nil || s.db == nil {
		return ErrMediaTaskUnavailable
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM subnexus_seedance_pending_holds WHERE task_id = $1`, strings.TrimSpace(taskID))
	return err
}

func validJournalTask(task *MediaTaskRecord) bool {
	return task != nil && strings.TrimSpace(task.ID) != "" && task.UserID > 0 && task.APIKeyID > 0 &&
		strings.TrimSpace(task.RequestHash) != "" && strings.TrimSpace(task.Phase) != "" &&
		!math.IsNaN(task.UnitPrice) && !math.IsInf(task.UnitPrice, 0) && task.UnitPrice >= 0
}

func decodeMediaTask(data []byte) (*MediaTaskRecord, error) {
	var task MediaTaskRecord
	if err := json.Unmarshal(data, &task); err != nil {
		return nil, fmt.Errorf("decode media task: %w", err)
	}
	return &task, nil
}

func mediaStoreLimit(limit int) int {
	if limit <= 0 {
		return 100
	}
	if limit > 1000 {
		return 1000
	}
	return limit
}
