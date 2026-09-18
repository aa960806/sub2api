package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

const seedanceHoldBatchPrefix = "media:"

var errSeedanceSettlementInvalid = errors.New("invalid persisted Seedance settlement")

// These optional methods finish already accepted Seedance work even when its
// owner or key was subsequently soft deleted. New reservations and all ordinary
// usage billing keep their existing active-user/key requirements.
func (r *usageBillingRepository) ApplySeedanceSettlement(ctx context.Context, cmd *service.UsageBillingCommand) (*service.UsageBillingApplyResult, error) {
	if cmd == nil || cmd.MediaType != "video" || cmd.BillingType != service.BillingTypeBalance ||
		cmd.UserID <= 0 || cmd.APIKeyID <= 0 || cmd.AccountID <= 0 || cmd.SubscriptionID != nil ||
		cmd.BalanceCost != 0 || cmd.SubscriptionCost != 0 || cmd.AccountQuotaCost != 0 ||
		!seedanceSettlementAmount(cmd.APIKeyQuotaCost, true) || !seedanceSettlementAmount(cmd.APIKeyRateLimitCost, true) {
		return nil, errSeedanceSettlementInvalid
	}
	prefix := service.BatchImageCaptureRequestID(seedanceHoldBatchPrefix)
	id, ok := strings.CutPrefix(strings.TrimSpace(cmd.RequestID), prefix)
	if !ok || !strings.HasSuffix(id, ":usage") {
		return nil, errSeedanceSettlementInvalid
	}
	id = strings.TrimSuffix(id, ":usage")
	if id == "" || strings.TrimSpace(id) != id {
		return nil, errSeedanceSettlementInvalid
	}
	if r == nil || r.db == nil {
		return nil, errors.New("usage billing repository db is nil")
	}
	cmd.Normalize()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	applied, err := r.claimUsageBillingKey(ctx, tx, cmd)
	if err != nil {
		return nil, err
	}
	if !applied {
		return &service.UsageBillingApplyResult{Applied: false}, nil
	}

	// Lock before inspecting the deletion marker so deletion cannot race the
	// active-key effects. Never clear deleted_at or transfer usage to another key.
	var deletedAt sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT deleted_at FROM api_keys
		WHERE id = $1 AND user_id = $2 FOR UPDATE`, cmd.APIKeyID, cmd.UserID).Scan(&deletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrAPIKeyNotFound
	}
	if err != nil {
		return nil, err
	}
	task, err := loadSeedanceSettlementTask(ctx, tx, id, cmd.UserID, cmd.APIKeyID, "capturing")
	if err != nil {
		return nil, err
	}
	if task.AccountID != cmd.AccountID || task.AccountType != cmd.AccountType || task.Model != cmd.Model || task.AccountQuota ||
		cmd.APIKeyQuotaCost != seedanceSettlementQuotaAmount(task.UnitPrice, task.Quota) ||
		cmd.APIKeyRateLimitCost != seedanceSettlementQuotaAmount(task.UnitPrice, task.RateLimits) {
		return nil, errSeedanceSettlementInvalid
	}
	captured, err := batchImageHoldClaimExists(ctx, tx, service.BatchImageCaptureRequestID(seedanceHoldBatchPrefix+id), cmd.APIKeyID)
	if err != nil {
		return nil, err
	}
	if !captured {
		return nil, errSeedanceSettlementInvalid
	}
	result := &service.UsageBillingApplyResult{Applied: true}
	if !deletedAt.Valid {
		if err := r.applyUsageBillingEffects(ctx, tx, cmd, result); err != nil {
			return nil, err
		}
	}
	// A deleted key no longer grants quota. Claim the unchanged original command
	// nevertheless, so a retry (including an archived claim) cannot add it later.
	// The caller still records the actual settled amount in its usage log.
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *usageBillingRepository) CaptureSeedanceBalance(ctx context.Context, cmd *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	if !validSeedanceBalanceCommand(cmd, true) {
		return nil, errSeedanceSettlementInvalid
	}
	return r.applyBatchImageBalanceHold(ctx, cmd, func(ctx context.Context, tx *sql.Tx, cmd *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
		if err := validateSeedanceBalanceTask(ctx, tx, cmd, "capturing"); err != nil {
			return nil, err
		}
		var balance, frozen float64
		err := tx.QueryRowContext(ctx, `UPDATE users
			SET balance = balance + ($1::numeric - $2::numeric),
				frozen_balance = COALESCE(frozen_balance, 0) - $1, updated_at = NOW()
			WHERE id = $3 AND COALESCE(frozen_balance, 0) >= $1
			RETURNING balance, frozen_balance`, cmd.HoldAmount, cmd.ActualAmount, cmd.UserID).Scan(&balance, &frozen)
		return seedanceBalanceResult(ctx, tx, cmd.UserID, balance, frozen, err)
	})
}

func (r *usageBillingRepository) ReleaseSeedanceBalance(ctx context.Context, cmd *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	if !validSeedanceBalanceCommand(cmd, false) {
		return nil, errSeedanceSettlementInvalid
	}
	return r.applyBatchImageBalanceHold(ctx, cmd, func(ctx context.Context, tx *sql.Tx, cmd *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
		if err := validateSeedanceBalanceTask(ctx, tx, cmd, "releasing"); err != nil {
			return nil, err
		}
		var balance, frozen float64
		err := tx.QueryRowContext(ctx, `UPDATE users
			SET balance = balance + $1,
				frozen_balance = COALESCE(frozen_balance, 0) - $1, updated_at = NOW()
			WHERE id = $2 AND COALESCE(frozen_balance, 0) >= $1
			RETURNING balance, frozen_balance`, cmd.HoldAmount, cmd.UserID).Scan(&balance, &frozen)
		return seedanceBalanceResult(ctx, tx, cmd.UserID, balance, frozen, err)
	})
}

func validSeedanceBalanceCommand(cmd *service.BatchImageBalanceHoldCommand, capture bool) bool {
	if cmd == nil || cmd.UserID <= 0 || cmd.APIKeyID <= 0 || !seedanceSettlementAmount(cmd.HoldAmount, false) {
		return false
	}
	batchID := strings.TrimSpace(cmd.BatchID)
	id, ok := strings.CutPrefix(batchID, seedanceHoldBatchPrefix)
	if !ok || id == "" || strings.TrimSpace(id) != id {
		return false
	}
	if capture {
		// This integration sells a fixed per-request price: no partial or excess
		// capture is valid, and the amount must remain equal to its frozen intent.
		return cmd.ActualAmount == cmd.HoldAmount && strings.TrimSpace(cmd.RequestID) == service.BatchImageCaptureRequestID(batchID)
	}
	return cmd.ActualAmount == 0 && strings.TrimSpace(cmd.RequestID) == service.BatchImageReleaseRequestID(batchID)
}

func validateSeedanceBalanceTask(ctx context.Context, tx *sql.Tx, cmd *service.BatchImageBalanceHoldCommand, phase string) error {
	id := strings.TrimPrefix(cmd.BatchID, seedanceHoldBatchPrefix)
	task, err := loadSeedanceSettlementTask(ctx, tx, id, cmd.UserID, cmd.APIKeyID, phase)
	if err != nil {
		return err
	}
	if task.UnitPrice != cmd.HoldAmount {
		return errSeedanceSettlementInvalid
	}
	held, err := batchImageHoldClaimExists(ctx, tx, service.BatchImageHoldRequestID(cmd.BatchID), cmd.APIKeyID)
	if err != nil {
		return err
	}
	if !held {
		return errSeedanceSettlementInvalid
	}
	return nil
}

// Only the immutable billing subset is decoded here, keeping the repository
// independent of the media handler/provider package.
type seedanceSettlementTask struct {
	ID           string  `json:"id"`
	UserID       int64   `json:"user_id"`
	APIKeyID     int64   `json:"api_key_id"`
	AccountID    int64   `json:"account_id"`
	Platform     string  `json:"platform"`
	Model        string  `json:"model"`
	Phase        string  `json:"phase"`
	UnitPrice    float64 `json:"unit_price"`
	Quota        bool    `json:"billing_quota"`
	RateLimits   bool    `json:"billing_rate_limits"`
	AccountQuota bool    `json:"billing_account_quota"`
	AccountType  string  `json:"billing_account_type"`
}

func loadSeedanceSettlementTask(ctx context.Context, tx *sql.Tx, id string, userID, apiKeyID int64, phase string) (*seedanceSettlementTask, error) {
	var record []byte
	var unitPrice float64
	err := tx.QueryRowContext(ctx, `SELECT record, unit_price FROM subnexus_seedance_tasks
		WHERE task_id = $1 AND user_id = $2 AND api_key_id = $3 AND phase = $4
		FOR SHARE`, id, userID, apiKeyID, phase).Scan(&record, &unitPrice)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errSeedanceSettlementInvalid
	}
	if err != nil {
		return nil, err
	}
	var task seedanceSettlementTask
	if err := json.Unmarshal(record, &task); err != nil {
		return nil, err
	}
	if task.ID != id || task.UserID != userID || task.APIKeyID != apiKeyID || task.Phase != phase ||
		task.Platform != service.PlatformSeedance || task.AccountID <= 0 || task.UnitPrice != unitPrice ||
		!seedanceSettlementAmount(unitPrice, false) {
		return nil, errSeedanceSettlementInvalid
	}
	return &task, nil
}

func seedanceSettlementAmount(amount float64, allowZero bool) bool {
	return !math.IsNaN(amount) && !math.IsInf(amount, 0) && (amount > 0 || (allowZero && amount == 0))
}

func seedanceSettlementQuotaAmount(amount float64, enabled bool) float64 {
	if enabled {
		return amount
	}
	return 0
}

func seedanceBalanceResult(ctx context.Context, tx *sql.Tx, userID int64, balance, frozen float64, err error) (*service.BatchImageBalanceHoldResult, error) {
	if err == nil {
		return &service.BatchImageBalanceHoldResult{NewBalance: &balance, FrozenBalance: &frozen}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE id = $1)`, userID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, service.ErrUserNotFound
	}
	return nil, errors.New("Seedance frozen balance is insufficient")
}
