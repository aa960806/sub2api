package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// subNexusInviteActivitiesRepository contains only the aggregates and the
// atomic balance mutation needed by the three migrated invitation activities.
// Keeping this separate from AffiliateRepository makes the feature easy to
// disable and prevents a stale activity implementation from changing the
// normal affiliate/recharge code paths.
type subNexusInviteActivitiesRepository struct {
	db *sql.DB
}

// Keep the namespace distinct from affiliate signup (1397638990/1) and the
// other migrated features.  hashint8 returns PostgreSQL's integer hash type,
// which selects the well-defined (integer, integer) advisory-lock overload.
const subNexusInviteActivitiesAdvisoryNamespace int64 = 1397638992

var _ service.InviteActivitiesRepository = (*subNexusInviteActivitiesRepository)(nil)

// NewSubNexusInviteActivitiesRepository constructs the SQL adapter.  The
// constructor returns the narrow service interface so callers cannot depend on
// implementation details or issue arbitrary financial queries.
func NewSubNexusInviteActivitiesRepository(db *sql.DB) service.InviteActivitiesRepository {
	return &subNexusInviteActivitiesRepository{db: db}
}

func (r *subNexusInviteActivitiesRepository) usable() error {
	if r == nil || r.db == nil {
		return sql.ErrConnDone
	}
	return nil
}

// CountEligibleInvitees counts each invitee once when they have at least one
// positive-net real recharge.  A partially refunded order contributes only its
// remaining amount; a fully refunded order contributes no qualification.
// The amount column is intentionally used here (the credited USD amount), as
// in the legacy SubNexus implementation.  pay_amount is the gateway currency
// amount and must never drive these USD activity thresholds.
func (r *subNexusInviteActivitiesRepository) CountEligibleInvitees(ctx context.Context, userID int64) (int, error) {
	if err := r.usable(); err != nil {
		return 0, err
	}
	if userID <= 0 {
		return 0, fmt.Errorf("invalid inviter user id %d", userID)
	}
	var count int64
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*)::bigint
		FROM user_affiliates ua
		WHERE ua.inviter_id = $1
		  AND EXISTS (
			SELECT 1
			FROM payment_orders po
			WHERE po.user_id = ua.user_id
			  AND po.status IN ('COMPLETED', 'PARTIALLY_REFUNDED')
			  AND po.order_type IN ('balance', 'subscription', 'first_recharge_gift')
			  AND GREATEST(
					COALESCE(po.amount, 0) - GREATEST(COALESCE(po.refund_amount, 0), 0),
					0
				  ) > 0
		  )
	`, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count eligible invitees: %w", err)
	}
	return intCount(count), nil
}

// CountQualifiedInvitees counts invitees whose cumulative positive-net
// credited amount reaches threshold.  The grouping is by invitee, so multiple
// orders (and partial refunds) are aggregated without double-counting people.
func (r *subNexusInviteActivitiesRepository) CountQualifiedInvitees(ctx context.Context, userID int64, threshold float64) (int, error) {
	if err := r.usable(); err != nil {
		return 0, err
	}
	if userID <= 0 {
		return 0, fmt.Errorf("invalid inviter user id %d", userID)
	}
	if !validPositiveActivityAmount(threshold) {
		return 0, fmt.Errorf("invalid invitee recharge threshold %v", threshold)
	}
	var count int64
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*)::bigint
		FROM (
			SELECT ua.user_id
			FROM user_affiliates ua
			JOIN payment_orders po ON po.user_id = ua.user_id
			WHERE ua.inviter_id = $1
			  AND po.status IN ('COMPLETED', 'PARTIALLY_REFUNDED')
			  AND po.order_type IN ('balance', 'subscription', 'first_recharge_gift')
			GROUP BY ua.user_id
			HAVING COALESCE(SUM(GREATEST(
				COALESCE(po.amount, 0) - GREATEST(COALESCE(po.refund_amount, 0), 0),
				0
			)), 0) >= $2
		) qualified_invitees
	`, userID, threshold).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count qualified invitees: %w", err)
	}
	return intCount(count), nil
}

// SumCompletedRecharge returns the user's cumulative credited recharge net of
// refunds.  Only the statuses and order types used by the old activity rules
// are accepted.  REFUNDED is deliberately excluded even if a malformed row
// carries a zero refund amount; PARTIALLY_REFUNDED is admitted and clamped to
// zero when refund_amount is greater than amount.
func (r *subNexusInviteActivitiesRepository) SumCompletedRecharge(ctx context.Context, userID int64) (float64, error) {
	if err := r.usable(); err != nil {
		return 0, err
	}
	if userID <= 0 {
		return 0, fmt.Errorf("invalid recharge user id %d", userID)
	}
	var total float64
	err := r.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(GREATEST(
			COALESCE(po.amount, 0) - GREATEST(COALESCE(po.refund_amount, 0), 0),
			0
		))::double precision, 0)
		FROM payment_orders po
		WHERE po.user_id = $1
		  AND po.status IN ('COMPLETED', 'PARTIALLY_REFUNDED')
		  AND po.order_type IN ('balance', 'subscription', 'first_recharge_gift')
	`, userID).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("sum completed recharge: %w", err)
	}
	if math.IsNaN(total) || math.IsInf(total, 0) || total < 0 {
		return 0, fmt.Errorf("invalid completed recharge total %v", total)
	}
	return total, nil
}

func (r *subNexusInviteActivitiesRepository) CountRewards(ctx context.Context, userID int64, source string) (int, error) {
	if err := r.usable(); err != nil {
		return 0, err
	}
	if userID <= 0 || strings.TrimSpace(source) == "" {
		return 0, fmt.Errorf("invalid reward count arguments")
	}
	var count int64
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*)::bigint
		FROM activity_reward_logs
		WHERE user_id = $1 AND source = $2
	`, userID, source).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count activity rewards: %w", err)
	}
	return intCount(count), nil
}

func (r *subNexusInviteActivitiesRepository) ListClaimedMilestones(ctx context.Context, userID int64, source string) (map[int]bool, error) {
	claimed := make(map[int]bool)
	if err := r.usable(); err != nil {
		return claimed, err
	}
	if userID <= 0 || strings.TrimSpace(source) == "" {
		return claimed, fmt.Errorf("invalid milestone arguments")
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT period
		FROM activity_reward_logs
		WHERE user_id = $1 AND source = $2
	`, userID, source)
	if err != nil {
		return nil, fmt.Errorf("list claimed milestones: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var period string
		if err := rows.Scan(&period); err != nil {
			return nil, fmt.Errorf("scan claimed milestone: %w", err)
		}
		value, err := strconv.Atoi(strings.TrimSpace(period))
		if err == nil && value > 0 {
			claimed[value] = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate claimed milestones: %w", err)
	}
	return claimed, nil
}

// GrantReward inserts the idempotency marker and credits the user's balance in
// one transaction.  The transaction-scoped advisory lock serializes claims for
// one user, while ON CONFLICT protects retries and races that observed the
// same previously-used chance.  A missing/deleted user rolls the log insert
// back, so an orphaned reward can never be recorded.
func (r *subNexusInviteActivitiesRepository) GrantReward(ctx context.Context, userID int64, source, period string, amount float64, note string) (bool, error) {
	if err := r.usable(); err != nil {
		return false, err
	}
	if userID <= 0 || strings.TrimSpace(source) == "" || strings.TrimSpace(period) == "" {
		return false, fmt.Errorf("invalid reward arguments")
	}
	if !validPositiveActivityAmount(amount) {
		return false, fmt.Errorf("invalid reward amount %v", amount)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin activity reward transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`SELECT pg_advisory_xact_lock(1397638992, hashint8($1::bigint))`,
		userID,
	); err != nil {
		return false, fmt.Errorf("lock activity reward user: %w", err)
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO activity_reward_logs (user_id, source, period, rank, amount, note)
		VALUES ($1, $2, $3, 0, $4, $5)
		ON CONFLICT (source, period, user_id) DO NOTHING
	`, userID, source, period, amount, note)
	if err != nil {
		return false, fmt.Errorf("insert activity reward: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("inspect activity reward insert: %w", err)
	}
	if inserted == 0 {
		if err := tx.Commit(); err != nil {
			return false, fmt.Errorf("commit duplicate activity reward: %w", err)
		}
		return false, nil
	}

	result, err = tx.ExecContext(ctx, `
		UPDATE users
		SET balance = balance + $1,
		    total_recharged = total_recharged + $1,
		    updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
	`, amount, userID)
	if err != nil {
		return false, fmt.Errorf("credit activity reward balance: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("inspect activity reward balance update: %w", err)
	}
	if affected == 0 {
		return false, service.ErrUserNotFound
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit activity reward: %w", err)
	}
	return true, nil
}

// GrantRewardIfEnabled is the production path used by the migrated activity
// claims.  It repeats the rollout check inside the same transaction that
// inserts the idempotency marker and updates the balance.  Each setting row is
// locked with FOR SHARE, so an administrator update either linearizes before
// this claim (the claim is rejected) or after it commits (the claim is valid).
func (r *subNexusInviteActivitiesRepository) GrantRewardIfEnabled(ctx context.Context, userID int64, source, period string, amount float64, note, feature string, rule service.InviteActivityClaimRule) (bool, error) {
	if err := r.usable(); err != nil {
		return false, err
	}
	if userID <= 0 || strings.TrimSpace(source) == "" || strings.TrimSpace(period) == "" {
		return false, fmt.Errorf("invalid reward arguments")
	}
	if !validPositiveActivityAmount(amount) {
		return false, fmt.Errorf("invalid reward amount %v", amount)
	}
	childKey, ok := mapInviteActivityFeature(feature)
	if !ok {
		return false, fmt.Errorf("invalid invite activity feature %q", feature)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin activity reward transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx,
		`SELECT pg_advisory_xact_lock(1397638992, hashint8($1::bigint))`, userID,
	); err != nil {
		return false, fmt.Errorf("lock activity reward user: %w", err)
	}
	if enabled, err := inviteActivityGateEnabled(ctx, tx, childKey); err != nil {
		return false, err
	} else if !enabled {
		if err := tx.Commit(); err != nil {
			return false, fmt.Errorf("commit disabled activity claim: %w", err)
		}
		return false, service.ErrInviteActivityDisabledForFeature(feature)
	}
	var duplicate bool
	if err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM activity_reward_logs WHERE user_id = $1 AND source = $2 AND period = $3)`,
		userID, source, period,
	).Scan(&duplicate); err != nil {
		return false, fmt.Errorf("check duplicate activity reward: %w", err)
	}
	if duplicate {
		if err := tx.Commit(); err != nil {
			return false, fmt.Errorf("commit duplicate activity reward: %w", err)
		}
		return false, nil
	}
	if err := inviteActivityClaimEligible(ctx, tx, userID, source, rule); err != nil {
		return false, err
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO activity_reward_logs (user_id, source, period, rank, amount, note)
		VALUES ($1, $2, $3, 0, $4, $5)
		ON CONFLICT (source, period, user_id) DO NOTHING
	`, userID, source, period, amount, note)
	if err != nil {
		return false, fmt.Errorf("insert activity reward: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("inspect activity reward insert: %w", err)
	}
	if inserted == 0 {
		if err := tx.Commit(); err != nil {
			return false, fmt.Errorf("commit duplicate activity reward: %w", err)
		}
		return false, nil
	}
	result, err = tx.ExecContext(ctx, `
		UPDATE users
		SET balance = balance + $1,
		    total_recharged = total_recharged + $1,
		    updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
	`, amount, userID)
	if err != nil {
		return false, fmt.Errorf("credit activity reward balance: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("inspect activity reward balance update: %w", err)
	}
	if affected == 0 {
		return false, service.ErrUserNotFound
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit activity reward: %w", err)
	}
	return true, nil
}

func inviteActivityClaimEligible(ctx context.Context, tx *sql.Tx, userID int64, source string, rule service.InviteActivityClaimRule) error {
	if rule.RequiredCount < 0 || !validActivityThreshold(rule.Threshold) {
		return fmt.Errorf("invalid invite activity claim rule")
	}
	var used int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM activity_reward_logs WHERE user_id = $1 AND source = $2`, userID, source).Scan(&used); err != nil {
		return fmt.Errorf("count activity rewards for claim: %w", err)
	}
	switch source {
	case service.ActivitySourceRechargeWheel:
		if rule.Threshold <= 0 {
			return service.ErrRechargeWheelDisabled
		}
		var recharged float64
		if err := tx.QueryRowContext(ctx, `
			SELECT COALESCE(SUM(GREATEST(COALESCE(amount, 0) - GREATEST(COALESCE(refund_amount, 0), 0), 0)), 0)::double precision
			FROM payment_orders
			WHERE user_id = $1
			  AND status IN ('COMPLETED', 'PARTIALLY_REFUNDED')
			  AND order_type IN ('balance', 'subscription', 'first_recharge_gift')
		`, userID).Scan(&recharged); err != nil {
			return fmt.Errorf("recheck recharge wheel qualification: %w", err)
		}
		if rechargeWheelChancesForRule(recharged, rule.Threshold) <= used {
			return infraerrors.BadRequest("RECHARGE_WHEEL_NO_CHANCE", "no recharge wheel chance available")
		}
	case service.ActivitySourceInviteLottery, service.ActivitySourceInviteMilestone:
		eligible, qualified, err := countInviteActivityQualificationTx(ctx, tx, userID, rule)
		if err != nil {
			return err
		}
		if source == service.ActivitySourceInviteMilestone {
			if rule.RequiredCount <= 0 || eligible < rule.RequiredCount {
				return infraerrors.BadRequest("INVITE_MILESTONE_NOT_REACHED", "invite milestone has not been reached")
			}
			if rule.RechargeLimitEnable && qualified < rule.RequiredCount {
				return infraerrors.BadRequest("INVITE_MILESTONE_RECHARGE_REQUIRED", "invited users have not reached the required recharge amount")
			}
		} else {
			available := eligible
			if rule.RechargeLimitEnable {
				available = qualified
			}
			if available <= used {
				return infraerrors.BadRequest("INVITE_LOTTERY_NO_CHANCE", "no invite lottery chance available")
			}
		}
	default:
		return fmt.Errorf("invalid invite activity source %q", source)
	}
	return nil
}

func countInviteActivityQualificationTx(ctx context.Context, tx *sql.Tx, inviterID int64, rule service.InviteActivityClaimRule) (eligible, qualified int, err error) {
	if rule.RechargeLimitEnable && rule.Threshold <= 0 {
		return 0, 0, fmt.Errorf("invalid invitee recharge threshold %v", rule.Threshold)
	}
	var eligible64, qualified64 int64
	if err = tx.QueryRowContext(ctx, `
		SELECT COUNT(*)::bigint
		FROM user_affiliates ua
		WHERE ua.inviter_id = $1
		  AND EXISTS (
			SELECT 1 FROM payment_orders po
			WHERE po.user_id = ua.user_id
			  AND po.status IN ('COMPLETED', 'PARTIALLY_REFUNDED')
			  AND po.order_type IN ('balance', 'subscription', 'first_recharge_gift')
			  AND GREATEST(COALESCE(po.amount, 0) - GREATEST(COALESCE(po.refund_amount, 0), 0), 0) > 0
		  )
	`, inviterID).Scan(&eligible64); err != nil {
		return 0, 0, fmt.Errorf("recheck eligible invitees: %w", err)
	}
	if rule.RechargeLimitEnable {
		if err = tx.QueryRowContext(ctx, `
			SELECT COUNT(*)::bigint FROM (
				SELECT ua.user_id
				FROM user_affiliates ua JOIN payment_orders po ON po.user_id = ua.user_id
				WHERE ua.inviter_id = $1
				  AND po.status IN ('COMPLETED', 'PARTIALLY_REFUNDED')
				  AND po.order_type IN ('balance', 'subscription', 'first_recharge_gift')
				GROUP BY ua.user_id
				HAVING COALESCE(SUM(GREATEST(COALESCE(po.amount, 0) - GREATEST(COALESCE(po.refund_amount, 0), 0), 0)), 0) >= $2
			) qualified_invitees
		`, inviterID, rule.Threshold).Scan(&qualified64); err != nil {
			return 0, 0, fmt.Errorf("recheck qualified invitees: %w", err)
		}
	} else {
		qualified64 = eligible64
	}
	return intCount(eligible64), intCount(qualified64), nil
}

func validActivityThreshold(value float64) bool {
	return value >= 0 && value <= 1_000_000_000 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func rechargeWheelChancesForRule(recharged, threshold float64) int {
	if !validActivityThreshold(recharged) || recharged <= 0 || threshold <= 0 {
		return 0
	}
	return int(math.Floor((recharged + 1e-8) / threshold))
}

func mapInviteActivityFeature(feature string) (string, bool) {
	switch feature {
	case "invite_lottery":
		return "invite_lottery_enabled", true
	case "recharge_wheel":
		return "recharge_wheel_enabled", true
	case "invite_milestone":
		return "invite_milestone_enabled", true
	default:
		return "", false
	}
}

func inviteActivityGateEnabled(ctx context.Context, tx *sql.Tx, childKey string) (bool, error) {
	var aggregate string
	err := tx.QueryRowContext(ctx,
		`SELECT value FROM settings WHERE key = 'subnexus_invite_activities_enabled' FOR SHARE`,
	).Scan(&aggregate)
	if err == sql.ErrNoRows || aggregate != "true" {
		if err != nil && err != sql.ErrNoRows {
			return false, fmt.Errorf("read invite activity rollout switch: %w", err)
		}
		return false, nil
	}
	var raw string
	err = tx.QueryRowContext(ctx,
		`SELECT value FROM settings WHERE key = 'subnexus_invite_activities_config' FOR SHARE`,
	).Scan(&raw)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read invite activity policy: %w", err)
	}
	var cfg struct {
		InviteLotteryEnabled   bool `json:"invite_lottery_enabled"`
		RechargeWheelEnabled   bool `json:"recharge_wheel_enabled"`
		InviteMilestoneEnabled bool `json:"invite_milestone_enabled"`
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return false, nil
	}
	switch childKey {
	case "invite_lottery_enabled":
		return cfg.InviteLotteryEnabled, nil
	case "recharge_wheel_enabled":
		return cfg.RechargeWheelEnabled, nil
	case "invite_milestone_enabled":
		return cfg.InviteMilestoneEnabled, nil
	default:
		return false, nil
	}
}

func validPositiveActivityAmount(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0) && value <= 1_000_000_000
}

func intCount(value int64) int {
	if value <= 0 {
		return 0
	}
	maxInt := int64(^uint(0) >> 1)
	if value > maxInt {
		return int(maxInt)
	}
	return int(value)
}
