package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

type battlePassGrant struct {
	ID         int64
	SeasonID   int64
	UserID     int64
	RewardID   int64
	RewardType string
	Payload    map[string]any
}

const battlePassMaxAutomaticRewardAttempts = 5

type BattlePassCosmetic struct {
	ID         int64     `json:"id"`
	Kind       string    `json:"kind"`
	Code       string    `json:"code"`
	Name       string    `json:"name"`
	ColorToken string    `json:"color_token"`
	AssetKey   string    `json:"asset_key"`
	Equipped   bool      `json:"equipped"`
	GrantedAt  time.Time `json:"granted_at"`
}

type BattlePassAdminUserProgress struct {
	UserID          int64     `json:"user_id"`
	Email           string    `json:"email"`
	Exp             int64     `json:"exp"`
	Level           int       `json:"level"`
	PremiumUnlocked bool      `json:"premium_unlocked"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type BattlePassAdminGrant struct {
	ID         int64      `json:"id"`
	UserID     int64      `json:"user_id"`
	UserEmail  string     `json:"user_email"`
	RewardID   int64      `json:"reward_id"`
	RewardType string     `json:"reward_type"`
	Track      string     `json:"track"`
	Level      int        `json:"level"`
	Status     string     `json:"status"`
	Attempts   int        `json:"attempt_count"`
	LastError  string     `json:"last_error,omitempty"`
	GrantedAt  *time.Time `json:"granted_at,omitempty"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

func (s *BattlePassService) ListSeasonUsers(ctx context.Context, seasonID int64) ([]BattlePassAdminUserProgress, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.user_id, u.email, p.exp, p.level, p.premium_unlocked, p.updated_at
		FROM battle_pass_user_progress p JOIN users u ON u.id=p.user_id
		WHERE p.season_id=$1 ORDER BY p.updated_at DESC, p.user_id DESC LIMIT 500
	`, seasonID)
	if err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to list battle pass users")
	}
	defer rows.Close()
	items := make([]BattlePassAdminUserProgress, 0)
	for rows.Next() {
		var item BattlePassAdminUserProgress
		if err := rows.Scan(&item.UserID, &item.Email, &item.Exp, &item.Level, &item.PremiumUnlocked, &item.UpdatedAt); err != nil {
			return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to scan battle pass user")
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *BattlePassService) ListSeasonGrants(ctx context.Context, seasonID int64) ([]BattlePassAdminGrant, error) {
	var seasonStatus string
	if err := s.db.QueryRowContext(ctx, `SELECT status FROM battle_pass_seasons WHERE id=$1`, seasonID).Scan(&seasonStatus); err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to load battle pass season status")
	}
	rewardByID := make(map[int64]BattlePassRewardInput)
	if seasonStatus != BattlePassStatusDraft {
		snapshot, err := loadBattlePassConfigSnapshot(ctx, s.db, seasonID)
		if err != nil {
			return nil, err
		}
		rewardByID = make(map[int64]BattlePassRewardInput, len(snapshot.Rewards))
		for _, reward := range snapshot.Rewards {
			if reward.ID <= 0 {
				return nil, infraerrors.InternalServer("BATTLE_PASS_CONFIG_INVALID", "published reward is missing its id")
			}
			rewardByID[reward.ID] = reward
		}
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT g.id, g.user_id, u.email, g.reward_id,
		       g.status, g.attempt_count, g.last_error, g.granted_at, g.updated_at
		FROM battle_pass_reward_grants g
		JOIN users u ON u.id=g.user_id
		WHERE g.season_id=$1 ORDER BY g.updated_at DESC, g.id DESC LIMIT 500
	`, seasonID)
	if err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to list battle pass grants")
	}
	defer rows.Close()
	items := make([]BattlePassAdminGrant, 0)
	for rows.Next() {
		var item BattlePassAdminGrant
		var granted sql.NullTime
		if err := rows.Scan(&item.ID, &item.UserID, &item.UserEmail, &item.RewardID, &item.Status, &item.Attempts, &item.LastError, &granted, &item.UpdatedAt); err != nil {
			return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to scan battle pass grant")
		}
		reward, ok := rewardByID[item.RewardID]
		if !ok {
			item.Status = "blocked_config"
			item.LastError = "published reward is missing from its configuration snapshot"
		} else {
			item.RewardType = reward.RewardType
			item.Track = reward.Track
			item.Level = reward.Level
		}
		if granted.Valid {
			value := granted.Time
			item.GrantedAt = &value
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to read battle pass grants")
	}
	return items, nil
}

func (s *BattlePassService) ListSeasonPurchases(ctx context.Context, seasonID int64) ([]BattlePassPurchase, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, season_id, user_id, price, balance_before, balance_after, idempotency_key, purchased_at
		FROM battle_pass_purchases WHERE season_id=$1 ORDER BY purchased_at DESC, id DESC LIMIT 500
	`, seasonID)
	if err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to list battle pass purchases")
	}
	defer rows.Close()
	items := make([]BattlePassPurchase, 0)
	for rows.Next() {
		var item BattlePassPurchase
		if err := rows.Scan(&item.ID, &item.SeasonID, &item.UserID, &item.Price, &item.BalanceBefore, &item.BalanceAfter, &item.IdempotencyKey, &item.PurchasedAt); err != nil {
			return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to scan battle pass purchase")
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *BattlePassService) RetryRewardGrant(ctx context.Context, grantID int64) error {
	if grantID <= 0 {
		return infraerrors.BadRequest("BATTLE_PASS_GRANT_INVALID", "grant id is invalid")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to start battle pass reward retry")
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.requireEnabledTx(ctx, tx); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE battle_pass_reward_grants
		SET status='pending', last_error='', updated_at=NOW()
		WHERE id=$1 AND status='failed'
	`, grantID)
	if err != nil {
		return infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to retry battle pass reward")
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return infraerrors.Conflict("BATTLE_PASS_GRANT_NOT_RETRYABLE", "only failed battle pass rewards can be retried")
	}
	if err := tx.Commit(); err != nil {
		return infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to commit battle pass reward retry")
	}
	return nil
}

// ProcessRewardGrantsOnce is independent from all request and payment paths.
// When the switch is disabled it returns before selecting any battle-pass rows.
func (s *BattlePassService) ProcessRewardGrantsOnce(ctx context.Context, now time.Time) (int, error) {
	enabled, err := s.IsEnabled(ctx)
	if err != nil || !enabled {
		return 0, err
	}
	processed := 0
	for range battlePassRewardBatch {
		grant, changedUserID, err := s.processOneRewardGrant(ctx, now)
		if err != nil {
			return processed, err
		}
		if grant == nil {
			break
		}
		processed++
		if changedUserID > 0 {
			s.invalidateUserCaches(ctx, changedUserID)
		}
	}
	return processed, nil
}

// ReconcilePremiumRewardGrantsOnce is retained as a scheduler-compatible no-op.
// Rewards are now enqueued only by an authenticated, explicit user claim.
func (s *BattlePassService) ReconcilePremiumRewardGrantsOnce(ctx context.Context, now time.Time) error {
	enabled, err := s.IsEnabled(ctx)
	if err != nil || !enabled {
		return err
	}
	_ = now
	return nil
}

func (s *BattlePassService) processOneRewardGrant(ctx context.Context, now time.Time) (*battlePassGrant, int64, error) {
	if s == nil || s.db == nil {
		return nil, 0, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "battle pass database is unavailable")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, 0, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to start battle pass reward transaction")
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.requireEnabledTx(ctx, tx); err != nil {
		return nil, 0, err
	}
	grant, err := claimBattlePassRewardGrant(ctx, tx, now)
	if err != nil {
		return nil, 0, err
	}
	if grant == nil {
		return nil, 0, tx.Commit()
	}
	return s.applyClaimedBattlePassRewardTx(ctx, tx, *grant)
}

func (s *BattlePassService) applyClaimedBattlePassRewardTx(ctx context.Context, tx *sql.Tx, grant battlePassGrant) (*battlePassGrant, int64, error) {
	reward, err := loadBattlePassRewardSnapshot(ctx, tx, grant.SeasonID, grant.RewardID)
	if err != nil {
		return s.blockBattlePassGrant(ctx, tx, grant, err)
	}
	grant.RewardType = reward.RewardType
	grant.Payload = reward.Payload
	// Subscription assignment is owned by SubscriptionService and is made
	// idempotent by its durable marker. Keep the reward transaction open while
	// assigning the subscription so its rollout-switch lock remains held until
	// the grant and the assignment have both committed.
	if grant.RewardType == "subscription_days" {
		return s.applySubscriptionRewardTx(ctx, tx, grant)
	}
	status, metadata, changed, err := applyBattlePassRewardTx(ctx, tx, grant)
	if err != nil {
		return s.failBattlePassGrant(ctx, tx, grant, err)
	}
	encoded, _ := json.Marshal(metadata)
	if _, err := tx.ExecContext(ctx, `
		UPDATE battle_pass_reward_grants
		SET status=$2, metadata=$3, last_error='', granted_at=NOW(), updated_at=NOW()
		WHERE id=$1 AND status='processing'
	`, grant.ID, status, encoded); err != nil {
		return nil, 0, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to complete battle pass reward")
	}
	if err := tx.Commit(); err != nil {
		return nil, 0, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to commit battle pass reward")
	}
	if changed {
		return &grant, grant.UserID, nil
	}
	return &grant, 0, nil
}

func (s *BattlePassService) processRewardGrantByID(ctx context.Context, grantID, userID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to start selected battle pass reward")
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.requireEnabledTx(ctx, tx); err != nil {
		return err
	}
	var grant battlePassGrant
	err = tx.QueryRowContext(ctx, `
		UPDATE battle_pass_reward_grants
		SET status='processing', attempt_count=attempt_count+1, updated_at=NOW()
		WHERE id=$1 AND user_id=$2 AND status='pending'
		RETURNING id, season_id, user_id, reward_id
	`, grantID, userID).Scan(&grant.ID, &grant.SeasonID, &grant.UserID, &grant.RewardID)
	if err == sql.ErrNoRows {
		return tx.Commit()
	}
	if err != nil {
		return infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to claim selected battle pass reward")
	}
	_, changedUserID, err := s.applyClaimedBattlePassRewardTx(ctx, tx, grant)
	if err == nil && changedUserID > 0 {
		s.invalidateUserCaches(ctx, changedUserID)
	}
	return err
}

func (s *BattlePassService) ClaimRewardForUser(ctx context.Context, userID, rewardID int64, now time.Time) (*BattlePassClaimResult, error) {
	if rewardID <= 0 {
		return nil, infraerrors.BadRequest("BATTLE_PASS_REWARD_INVALID", "reward id is invalid")
	}
	season, states, err := s.prepareRewardClaim(ctx, userID, now)
	if err != nil {
		return nil, err
	}
	var target *BattlePassRewardState
	for i := range states {
		if states[i].ID == rewardID {
			target = &states[i]
			break
		}
	}
	if target == nil {
		return nil, infraerrors.NotFound("BATTLE_PASS_REWARD_NOT_FOUND", "battle pass reward not found")
	}
	if err := validateBattlePassRewardClaimState(*target); err != nil {
		return nil, err
	}
	if target.Status != "claimable" {
		return &BattlePassClaimResult{Rewards: states}, nil
	}
	grantIDs, err := s.enqueueManualRewardClaims(ctx, season.ID, userID, []int64{rewardID})
	if err != nil {
		return nil, err
	}
	if err := s.processManualRewardClaims(ctx, userID, grantIDs); err != nil {
		return nil, err
	}
	states, err = s.listRewardsForSeasonUser(ctx, season.ID, userID)
	return &BattlePassClaimResult{ClaimedCount: len(grantIDs), Rewards: states}, err
}

func validateBattlePassRewardClaimState(state BattlePassRewardState) error {
	switch state.Status {
	case "claimable", "granted", "granted_capped", "pending", "processing":
		return nil
	case "premium_locked":
		return infraerrors.Forbidden("BATTLE_PASS_PREMIUM_REQUIRED", "premium battle pass is required for this reward")
	case "locked":
		return infraerrors.Conflict("BATTLE_PASS_LEVEL_REQUIRED", "battle pass reward level has not been reached")
	default:
		return infraerrors.Conflict("BATTLE_PASS_REWARD_UNAVAILABLE", "battle pass reward is not claimable")
	}
}

func (s *BattlePassService) ClaimAllRewardsForUser(ctx context.Context, userID int64, now time.Time) (*BattlePassClaimResult, error) {
	season, states, err := s.prepareRewardClaim(ctx, userID, now)
	if err != nil {
		return nil, err
	}
	grantIDs, err := s.enqueueManualRewardClaims(ctx, season.ID, userID, claimableBattlePassRewardIDs(states))
	if err != nil {
		return nil, err
	}
	if err := s.processManualRewardClaims(ctx, userID, grantIDs); err != nil {
		return nil, err
	}
	states, err = s.listRewardsForSeasonUser(ctx, season.ID, userID)
	return &BattlePassClaimResult{ClaimedCount: len(grantIDs), Rewards: states}, err
}

func (s *BattlePassService) prepareRewardClaim(ctx context.Context, userID int64, now time.Time) (*BattlePassSeason, []BattlePassRewardState, error) {
	if err := s.requireUserAccess(ctx, now); err != nil {
		return nil, nil, err
	}
	if err := s.requireEligibleUser(ctx, userID); err != nil {
		return nil, nil, err
	}
	season, err := s.currentPublishedSeason(ctx, now)
	if err != nil {
		return nil, nil, err
	}
	if season == nil || runtimeSeasonStatus(*season, now) != "active" {
		return nil, nil, infraerrors.Forbidden("BATTLE_PASS_CLAIM_UNAVAILABLE", "battle pass rewards cannot be claimed right now")
	}
	if _, err := s.ensureUserProgress(ctx, season.ID, userID); err != nil {
		return nil, nil, err
	}
	states, err := s.listRewardsForSeasonUser(ctx, season.ID, userID)
	return season, states, err
}

func (s *BattlePassService) enqueueManualRewardClaims(ctx context.Context, seasonID, userID int64, rewardIDs []int64) ([]int64, error) {
	if len(rewardIDs) == 0 {
		return []int64{}, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to start battle pass reward claim")
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.requireEnabledTx(ctx, tx); err != nil {
		return nil, err
	}
	grantIDs := make([]int64, 0, len(rewardIDs))
	for _, rewardID := range rewardIDs {
		var grantID int64
		err := tx.QueryRowContext(ctx, `
			INSERT INTO battle_pass_reward_grants (season_id, user_id, reward_id)
			VALUES ($1,$2,$3)
			ON CONFLICT (season_id, user_id, reward_id) DO NOTHING
			RETURNING id
		`, seasonID, userID, rewardID).Scan(&grantID)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to enqueue battle pass reward claim")
		}
		grantIDs = append(grantIDs, grantID)
	}
	if err := tx.Commit(); err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to commit battle pass reward claim")
	}
	return grantIDs, nil
}

func (s *BattlePassService) processManualRewardClaims(ctx context.Context, userID int64, grantIDs []int64) error {
	for _, grantID := range grantIDs {
		if err := s.processRewardGrantByID(ctx, grantID, userID); err != nil {
			return err
		}
	}
	return nil
}

func claimBattlePassRewardGrant(ctx context.Context, tx *sql.Tx, now time.Time) (*battlePassGrant, error) {
	row := tx.QueryRowContext(ctx, `
		WITH candidate AS (
			SELECT g.id
			FROM battle_pass_reward_grants g
			JOIN battle_pass_seasons s ON s.id=g.season_id
			WHERE (g.status='pending' OR (g.status='failed' AND g.updated_at < $1 AND g.attempt_count < $3) OR (g.status='processing' AND g.updated_at < $2 AND g.attempt_count < $3))
			  AND ((s.status='scheduled' AND s.start_at <= $4) OR s.status='ended')
			ORDER BY g.updated_at, g.id
			FOR UPDATE OF g SKIP LOCKED
			LIMIT 1
		), claimed AS (
			UPDATE battle_pass_reward_grants g
			SET status='processing', attempt_count=g.attempt_count+1, updated_at=NOW()
			FROM candidate WHERE g.id=candidate.id
			RETURNING g.id, g.season_id, g.user_id, g.reward_id
		)
		SELECT c.id, c.season_id, c.user_id, c.reward_id
		FROM claimed c
	`, now.Add(-time.Minute), now.Add(-5*time.Minute), battlePassMaxAutomaticRewardAttempts, now)
	var grant battlePassGrant
	err := row.Scan(&grant.ID, &grant.SeasonID, &grant.UserID, &grant.RewardID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to claim battle pass reward")
	}
	return &grant, nil
}

func loadBattlePassRewardSnapshot(ctx context.Context, q battlePassRowQueryer, seasonID, rewardID int64) (BattlePassRewardInput, error) {
	snapshot, err := loadBattlePassConfigSnapshot(ctx, q, seasonID)
	if err != nil {
		return BattlePassRewardInput{}, err
	}
	for _, reward := range snapshot.Rewards {
		if reward.ID == rewardID {
			return reward, nil
		}
	}
	return BattlePassRewardInput{}, infraerrors.InternalServer("BATTLE_PASS_CONFIG_INVALID", "published reward is missing from its configuration snapshot")
}

func (s *BattlePassService) blockBattlePassGrant(ctx context.Context, tx *sql.Tx, grant battlePassGrant, cause error) (*battlePassGrant, int64, error) {
	message := truncateBattlePassError(cause.Error())
	if _, err := tx.ExecContext(ctx, `
		UPDATE battle_pass_reward_grants SET status='blocked_config', last_error=$2, updated_at=NOW()
		WHERE id=$1 AND status='processing'
	`, grant.ID, message); err != nil {
		return nil, 0, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to record blocked battle pass reward")
	}
	if err := tx.Commit(); err != nil {
		return nil, 0, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to commit blocked battle pass reward")
	}
	return &grant, 0, nil
}

func applyBattlePassRewardTx(ctx context.Context, tx *sql.Tx, grant battlePassGrant) (string, map[string]any, bool, error) {
	if err := validateBattlePassRewardPayload(BattlePassRewardInput{RewardType: grant.RewardType, Payload: grant.Payload}); err != nil {
		return "blocked_config", map[string]any{"reason": err.Error()}, false, nil
	}
	switch grant.RewardType {
	case "balance":
		amount := grant.Payload["amount"].(float64)
		var before, after float64
		err := tx.QueryRowContext(ctx, `
			UPDATE users SET balance=balance+$2, updated_at=NOW()
			WHERE id=$1 AND status='active' AND deleted_at IS NULL
			RETURNING balance-$2, balance
		`, grant.UserID, amount).Scan(&before, &after)
		if err == sql.ErrNoRows {
			return "blocked_config", map[string]any{"reason": "user is no longer eligible"}, false, nil
		}
		if err != nil {
			return "", nil, false, err
		}
		return "granted", map[string]any{"balance_before": before, "balance_after": after, "amount": amount}, true, nil
	case "concurrency":
		amount := int(grant.Payload["amount"].(float64))
		var before, after int
		err := tx.QueryRowContext(ctx, `
			WITH target AS (
				SELECT id, concurrency FROM users WHERE id=$1 AND status='active' AND deleted_at IS NULL FOR UPDATE
			), updated AS (
				UPDATE users u SET concurrency=LEAST(target.concurrency+$2, $3), updated_at=NOW()
				FROM target WHERE u.id=target.id RETURNING target.concurrency AS before_value, u.concurrency AS after_value
			) SELECT before_value, after_value FROM updated
		`, grant.UserID, amount, battlePassConcurrencyCap).Scan(&before, &after)
		if err == sql.ErrNoRows {
			return "blocked_config", map[string]any{"reason": "user is no longer eligible"}, false, nil
		}
		if err != nil {
			return "", nil, false, err
		}
		status := "granted"
		if after-before < amount {
			status = "granted_capped"
		}
		return status, map[string]any{"concurrency_before": before, "concurrency_after": after, "amount": amount}, before != after, nil
	case "badge", "title":
		var eligibleUserID int64
		if err := tx.QueryRowContext(ctx, `
			SELECT id FROM users
			WHERE id=$1 AND status='active' AND deleted_at IS NULL
			FOR UPDATE
		`, grant.UserID).Scan(&eligibleUserID); err == sql.ErrNoRows {
			return "blocked_config", map[string]any{"reason": "user is no longer eligible"}, false, nil
		} else if err != nil {
			return "", nil, false, err
		}
		code := strings.TrimSpace(grant.Payload["code"].(string))
		name := strings.TrimSpace(grant.Payload["name"].(string))
		color, _ := grant.Payload["color_token"].(string)
		asset, _ := grant.Payload["asset_key"].(string)
		if !battlePassSafeCosmeticToken(color, 32) && color != "" || !battlePassSafeCosmeticToken(asset, 120) && asset != "" {
			return "blocked_config", map[string]any{"reason": "cosmetic visual token is invalid"}, false, nil
		}
		var cosmeticID int64
		err := tx.QueryRowContext(ctx, `
			INSERT INTO battle_pass_cosmetics (season_id, kind, code, name, color_token, asset_key)
			VALUES ($1,$2,$3,$4,$5,$6)
			ON CONFLICT (season_id, kind, code) DO UPDATE SET code=EXCLUDED.code
			RETURNING id
		`, grant.SeasonID, grant.RewardType, code, name, color, asset).Scan(&cosmeticID)
		if err != nil {
			return "", nil, false, err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO battle_pass_user_cosmetics (user_id, cosmetic_id)
			SELECT $1,$2 WHERE EXISTS (SELECT 1 FROM users WHERE id=$1 AND status='active' AND deleted_at IS NULL)
			ON CONFLICT (user_id, cosmetic_id) DO NOTHING
		`, grant.UserID, cosmeticID); err != nil {
			return "", nil, false, err
		}
		return "granted", map[string]any{"cosmetic_id": cosmeticID, "code": code}, false, nil
	default:
		return "blocked_config", map[string]any{"reason": "unsupported reward type"}, false, nil
	}
}

func (s *BattlePassService) failBattlePassGrant(ctx context.Context, tx *sql.Tx, grant battlePassGrant, cause error) (*battlePassGrant, int64, error) {
	message := strings.TrimSpace(cause.Error())
	if len(message) > 160 {
		message = message[:160]
	}
	if _, err := tx.ExecContext(ctx, `UPDATE battle_pass_reward_grants SET status='failed', last_error=$2, updated_at=NOW() WHERE id=$1`, grant.ID, message); err != nil {
		return nil, 0, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to record battle pass reward failure")
	}
	if err := tx.Commit(); err != nil {
		return nil, 0, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to commit battle pass reward failure")
	}
	return &grant, 0, nil
}

// applySubscriptionRewardTx applies a subscription reward while the caller's
// reward transaction is still open. The caller has already acquired the
// Battle Pass rollout-switch row lock in that transaction; committing the
// subscription assignment before releasing it closes the disable/write race.
func (s *BattlePassService) applySubscriptionRewardTx(ctx context.Context, rewardTx *sql.Tx, grant battlePassGrant) (*battlePassGrant, int64, error) {
	if rewardTx == nil {
		return nil, 0, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "battle pass reward transaction is unavailable")
	}
	groupID, groupOK := grant.Payload["group_id"].(float64)
	days, daysOK := grant.Payload["days"].(float64)
	if !groupOK || !daysOK || groupID < 1 || days < 1 || math.Trunc(groupID) != groupID || math.Trunc(days) != days {
		return s.completeSubscriptionGrantTx(ctx, rewardTx, grant, "blocked_config", "subscription payload is invalid")
	}
	groupIDInt := int64(groupID)
	marker := fmt.Sprintf("battle_pass_reward:%d", grant.ID)
	var alreadyApplied bool
	err := rewardTx.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM user_subscriptions
			WHERE user_id=$1 AND group_id=$2
			  AND $3 = ANY(string_to_array(COALESCE(notes, ''), E'\n'))
		)
	`, grant.UserID, groupIDInt, marker).Scan(&alreadyApplied)
	if err != nil {
		return nil, 0, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to check subscription reward marker")
	}

	if !alreadyApplied {
		if s == nil || s.subscriptions == nil || s.subscriptions.entClient == nil || s.subscriptions.groupRepo == nil || s.subscriptions.userSubRepo == nil {
			return s.completeSubscriptionGrantTx(ctx, rewardTx, grant, "blocked_config", "subscription service is unavailable")
		}

		assignmentTx, err := s.subscriptions.entClient.Tx(ctx)
		if err != nil {
			return s.failBattlePassGrant(ctx, rewardTx, grant, fmt.Errorf("begin subscription assignment transaction: %w", err))
		}
		assignmentCommitted := false
		defer func() {
			if !assignmentCommitted {
				_ = assignmentTx.Rollback()
			}
		}()

		assignmentCtx := dbent.NewTxContext(ctx, assignmentTx)
		if _, _, err := s.subscriptions.assignOrExtendSubscription(assignmentCtx, &AssignSubscriptionInput{
			UserID:       grant.UserID,
			GroupID:      groupIDInt,
			ValidityDays: int(days),
			AssignedBy:   0,
			Notes:        marker,
		}, true); err != nil {
			return s.failBattlePassGrant(ctx, rewardTx, grant, fmt.Errorf("assign subscription reward: %w", err))
		}
		if err := assignmentTx.Commit(); err != nil {
			return s.failBattlePassGrant(ctx, rewardTx, grant, fmt.Errorf("commit subscription assignment: %w", err))
		}
		assignmentCommitted = true
	}

	result, changedUserID, err := s.completeSubscriptionGrantTx(ctx, rewardTx, grant, "granted", "")
	if err != nil {
		return nil, 0, err
	}
	if result != nil && s != nil && s.subscriptions != nil {
		if err := s.subscriptions.invalidateSubscriptionCaches(grant.UserID, groupIDInt); err != nil {
			return result, changedUserID, infraerrors.InternalServer("BATTLE_PASS_CACHE_UNAVAILABLE", "failed to invalidate subscription reward cache")
		}
	}
	return result, changedUserID, nil
}

// applySubscriptionReward is retained as a transaction-owning compatibility
// wrapper for package-local callers. Production reward processing uses the
// transaction-aware method above so the gate lock spans the assignment.
func (s *BattlePassService) applySubscriptionReward(ctx context.Context, grant battlePassGrant) (*battlePassGrant, int64, error) {
	if s == nil || s.db == nil {
		return nil, 0, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "battle pass database is unavailable")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, 0, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to start subscription reward transaction")
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.requireEnabledTx(ctx, tx); err != nil {
		return nil, 0, err
	}
	return s.applySubscriptionRewardTx(ctx, tx, grant)
}

func (s *BattlePassService) completeSubscriptionGrantTx(ctx context.Context, tx *sql.Tx, grant battlePassGrant, status, reason string) (*battlePassGrant, int64, error) {
	result, err := tx.ExecContext(ctx, `
		UPDATE battle_pass_reward_grants
		SET status=$2, last_error=$3, granted_at=CASE WHEN $2='granted' THEN NOW() ELSE granted_at END, updated_at=NOW()
		WHERE id=$1 AND status='processing'
	`, grant.ID, status, truncateBattlePassError(reason))
	if err != nil {
		return nil, 0, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to complete subscription reward")
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return nil, 0, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to complete subscription reward")
	}
	if err := tx.Commit(); err != nil {
		return nil, 0, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to commit subscription reward")
	}
	if changed == 0 {
		return nil, 0, nil
	}
	return &grant, 0, nil
}

// completeSubscriptionGrant is retained for package-local compatibility and
// now owns a transaction so it cannot update a grant outside a commit boundary.
func (s *BattlePassService) completeSubscriptionGrant(ctx context.Context, grant battlePassGrant, status, reason string) (*battlePassGrant, int64, error) {
	if s == nil || s.db == nil {
		return nil, 0, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "battle pass database is unavailable")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, 0, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to start subscription reward transaction")
	}
	defer func() { _ = tx.Rollback() }()
	return s.completeSubscriptionGrantTx(ctx, tx, grant, status, reason)
}

func truncateBattlePassError(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 160 {
		return value[:160]
	}
	return value
}

func (s *BattlePassService) invalidateUserCaches(ctx context.Context, userID int64) {
	if userID <= 0 {
		return
	}
	if s.authCache != nil {
		s.authCache.InvalidateAuthCacheByUserID(ctx, userID)
	}
	if s.billingCache != nil {
		_ = s.billingCache.InvalidateUserBalance(ctx, userID)
	}
}

func (s *BattlePassService) PurchasePremium(ctx context.Context, userID int64, idempotencyKey string, now time.Time) (*BattlePassPurchase, error) {
	if err := s.requireUserAccess(ctx, now); err != nil {
		return nil, err
	}
	if err := s.requireEligibleUser(ctx, userID); err != nil {
		return nil, err
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if !battlePassSafeCosmeticToken(idempotencyKey, 160) {
		return nil, infraerrors.BadRequest("BATTLE_PASS_IDEMPOTENCY_KEY_INVALID", "a safe Idempotency-Key is required")
	}
	season, err := s.currentPublishedSeason(ctx, now)
	if err != nil || season == nil || runtimeSeasonStatus(*season, now) != "active" {
		return nil, infraerrors.Forbidden("BATTLE_PASS_PURCHASE_UNAVAILABLE", "battle pass purchase is unavailable")
	}
	if season.PremiumPrice <= 0 {
		return nil, infraerrors.BadRequest("BATTLE_PASS_CONFIG_INVALID", "premium price is invalid")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to start battle pass purchase")
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.requireEnabledTx(ctx, tx); err != nil {
		return nil, err
	}
	var existing BattlePassPurchase
	err = tx.QueryRowContext(ctx, `
		SELECT id, season_id, price, balance_before, balance_after, idempotency_key, purchased_at
		FROM battle_pass_purchases WHERE user_id=$1 AND idempotency_key=$2
	`, userID, idempotencyKey).Scan(&existing.ID, &existing.SeasonID, &existing.Price, &existing.BalanceBefore, &existing.BalanceAfter, &existing.IdempotencyKey, &existing.PurchasedAt)
	if err == nil {
		if existing.SeasonID != season.ID {
			return nil, infraerrors.Conflict("BATTLE_PASS_IDEMPOTENCY_KEY_CONFLICT", "idempotency key belongs to another battle pass season")
		}
		return &existing, tx.Commit()
	}
	if err != sql.ErrNoRows {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to load battle pass purchase")
	}
	var existingSeason int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM battle_pass_purchases WHERE season_id=$1 AND user_id=$2`, season.ID, userID).Scan(&existingSeason)
	if err == nil {
		return nil, infraerrors.Conflict("BATTLE_PASS_ALREADY_PURCHASED", "premium track is already unlocked")
	}
	if err != sql.ErrNoRows {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to load premium state")
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO battle_pass_user_progress (season_id, user_id) VALUES ($1,$2)
		ON CONFLICT (season_id, user_id) DO NOTHING
	`, season.ID, userID); err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to initialize battle pass progress")
	}
	var before, after float64
	err = tx.QueryRowContext(ctx, `
		UPDATE users SET balance=balance-$2, updated_at=NOW()
		WHERE id=$1 AND status='active' AND deleted_at IS NULL AND balance >= $2
		RETURNING balance+$2, balance
	`, userID, season.PremiumPrice).Scan(&before, &after)
	if err == sql.ErrNoRows {
		return nil, infraerrors.BadRequest("BATTLE_PASS_BALANCE_INSUFFICIENT", "insufficient balance for battle pass purchase")
	}
	if err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to deduct battle pass balance")
	}
	purchase := &BattlePassPurchase{SeasonID: season.ID, Price: season.PremiumPrice, BalanceBefore: before, BalanceAfter: after, IdempotencyKey: idempotencyKey, PurchasedAt: now}
	err = tx.QueryRowContext(ctx, `
		INSERT INTO battle_pass_purchases (season_id, user_id, price, balance_before, balance_after, idempotency_key)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (season_id, user_id) DO NOTHING
		RETURNING id, purchased_at
	`, season.ID, userID, season.PremiumPrice, before, after, idempotencyKey).Scan(&purchase.ID, &purchase.PurchasedAt)
	if err == sql.ErrNoRows {
		// A concurrent request won the season/user uniqueness race. Roll back
		// this transaction so its provisional balance deduction is not retained,
		// then resolve an idempotent replay if the competing request used the
		// same key.
		_ = tx.Rollback()
		var existing BattlePassPurchase
		lookupErr := s.db.QueryRowContext(ctx, `
			SELECT id, season_id, price, balance_before, balance_after, idempotency_key, purchased_at
			FROM battle_pass_purchases WHERE user_id=$1 AND idempotency_key=$2
		`, userID, idempotencyKey).Scan(&existing.ID, &existing.SeasonID, &existing.Price, &existing.BalanceBefore, &existing.BalanceAfter, &existing.IdempotencyKey, &existing.PurchasedAt)
		if lookupErr == nil {
			return &existing, nil
		}
		if lookupErr != sql.ErrNoRows {
			return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to resolve concurrent battle pass purchase")
		}
		return nil, infraerrors.Conflict("BATTLE_PASS_ALREADY_PURCHASED", "premium track is already unlocked")
	}
	if err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to record battle pass purchase")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE battle_pass_user_progress SET premium_unlocked=TRUE, updated_at=NOW() WHERE season_id=$1 AND user_id=$2`, season.ID, userID); err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to unlock premium battle pass")
	}
	if err := tx.Commit(); err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to commit battle pass purchase")
	}
	s.invalidateUserCaches(ctx, userID)
	return purchase, nil
}

func (s *BattlePassService) ListCosmeticsForUser(ctx context.Context, userID int64, now time.Time) ([]BattlePassCosmetic, error) {
	if err := s.requireUserAccess(ctx, now); err != nil {
		return nil, err
	}
	if err := s.requireEligibleUser(ctx, userID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id, c.kind, c.code, c.name, c.color_token, c.asset_key, uc.equipped, uc.granted_at
		FROM battle_pass_user_cosmetics uc JOIN battle_pass_cosmetics c ON c.id=uc.cosmetic_id
		WHERE uc.user_id=$1 ORDER BY c.kind, c.name, c.id
	`, userID)
	if err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to load battle pass cosmetics")
	}
	defer rows.Close()
	items := make([]BattlePassCosmetic, 0)
	for rows.Next() {
		var item BattlePassCosmetic
		if err := rows.Scan(&item.ID, &item.Kind, &item.Code, &item.Name, &item.ColorToken, &item.AssetKey, &item.Equipped, &item.GrantedAt); err != nil {
			return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to scan battle pass cosmetic")
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *BattlePassService) EquipCosmetic(ctx context.Context, userID, cosmeticID int64, now time.Time) error {
	if err := s.requireUserAccess(ctx, now); err != nil {
		return err
	}
	if err := s.requireEligibleUser(ctx, userID); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to start cosmetic update")
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.requireEnabledTx(ctx, tx); err != nil {
		return err
	}
	var kind string
	err = tx.QueryRowContext(ctx, `
		SELECT c.kind FROM battle_pass_user_cosmetics uc JOIN battle_pass_cosmetics c ON c.id=uc.cosmetic_id
		WHERE uc.user_id=$1 AND uc.cosmetic_id=$2 FOR UPDATE
	`, userID, cosmeticID).Scan(&kind)
	if err == sql.ErrNoRows {
		return infraerrors.NotFound("BATTLE_PASS_COSMETIC_NOT_FOUND", "battle pass cosmetic is not owned")
	}
	if err != nil {
		return infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to load battle pass cosmetic")
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE battle_pass_user_cosmetics uc SET equipped=FALSE
		FROM battle_pass_cosmetics c WHERE uc.cosmetic_id=c.id AND uc.user_id=$1 AND c.kind=$2
	`, userID, kind); err != nil {
		return infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to clear equipped cosmetic")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE battle_pass_user_cosmetics SET equipped=TRUE WHERE user_id=$1 AND cosmetic_id=$2`, userID, cosmeticID); err != nil {
		return infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to equip battle pass cosmetic")
	}
	return tx.Commit()
}
