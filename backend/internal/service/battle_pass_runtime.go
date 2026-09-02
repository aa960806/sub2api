package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	battlePassUsageBatchSize    = 250
	battlePassRewardBatch       = 50
	battlePassConcurrencyCap    = 1000
	battlePassScanAdvisoryClass = 255
	// Payment state is deliberately confirmed after a short settlement window.
	// This prevents a just-created order from granting progress before a provider
	// callback or an immediate refund has settled. The source contribution is
	// still reconciled when the order is updated later.
	battlePassPaymentConfirmationDelay = 24 * time.Hour
)

type BattlePassUserProgress struct {
	Exp             int64     `json:"exp"`
	Level           int       `json:"level"`
	LevelStartExp   int64     `json:"level_start_exp"`
	NextLevelExp    *int64    `json:"next_level_exp,omitempty"`
	PremiumUnlocked bool      `json:"premium_unlocked"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type BattlePassTaskState struct {
	ID           int64    `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	TaskType     string   `json:"task_type"`
	PeriodType   string   `json:"period_type"`
	TargetValue  float64  `json:"target_value"`
	ExpReward    int64    `json:"exp_reward"`
	FilterScope  string   `json:"filter_scope"`
	FilterValues []string `json:"filter_values"`
	DisplayOrder int      `json:"display_order"`
	CurrentValue float64  `json:"current_value"`
	Completed    bool     `json:"completed"`
	PeriodKey    string   `json:"period_key"`
}

type BattlePassRewardState struct {
	ID         int64          `json:"id"`
	Level      int            `json:"level"`
	Track      string         `json:"track"`
	RewardType string         `json:"reward_type"`
	Payload    map[string]any `json:"payload"`
	Status     string         `json:"status"`
	LastError  string         `json:"last_error,omitempty"`
}

type BattlePassClaimResult struct {
	ClaimedCount int                     `json:"claimed_count"`
	Rewards      []BattlePassRewardState `json:"rewards"`
}

type battlePassGrantDisplayState struct {
	status    string
	lastError string
}

type BattlePassHistory struct {
	Experience []BattlePassExperienceEntry `json:"experience"`
	Purchases  []BattlePassPurchase        `json:"purchases"`
	Rewards    []BattlePassRewardState     `json:"rewards"`
}

type BattlePassExperienceEntry struct {
	ID        int64     `json:"id"`
	TaskID    *int64    `json:"task_id,omitempty"`
	PeriodKey string    `json:"period_key"`
	ExpDelta  int64     `json:"exp_delta"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

type BattlePassPurchase struct {
	ID             int64     `json:"id"`
	SeasonID       int64     `json:"season_id"`
	UserID         int64     `json:"user_id,omitempty"`
	Price          float64   `json:"price"`
	BalanceBefore  float64   `json:"balance_before"`
	BalanceAfter   float64   `json:"balance_after"`
	IdempotencyKey string    `json:"-"`
	PurchasedAt    time.Time `json:"purchased_at"`
}

type battlePassUsageLog struct {
	ID          int64
	UserID      int64
	Model       string
	ActualCost  float64
	ImageCount  int
	VideoCount  int
	CreatedAt   time.Time
	RequestType int16
	Eligible    bool
	Paused      bool
}

type battlePassRuntimeTask struct {
	ID int64
	BattlePassTaskInput
}

type battlePassConfigSnapshot struct {
	Levels  []BattlePassLevelInput  `json:"levels"`
	Tasks   []BattlePassTaskInput   `json:"tasks"`
	Rewards []BattlePassRewardInput `json:"rewards"`
}

type battlePassRowQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// loadBattlePassConfigSnapshot is the runtime source of truth for a published
// season. The editable child tables remain available to the admin draft view,
// but must not change the accounting or reward semantics after publication.
func loadBattlePassConfigSnapshot(ctx context.Context, q battlePassRowQueryer, seasonID int64) (battlePassConfigSnapshot, error) {
	var raw []byte
	if err := q.QueryRowContext(ctx, `SELECT config_snapshot FROM battle_pass_seasons WHERE id=$1`, seasonID).Scan(&raw); err != nil {
		return battlePassConfigSnapshot{}, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to load battle pass configuration snapshot")
	}
	var snapshot battlePassConfigSnapshot
	if len(raw) == 0 || json.Unmarshal(raw, &snapshot) != nil {
		return battlePassConfigSnapshot{}, infraerrors.InternalServer("BATTLE_PASS_CONFIG_INVALID", "battle pass configuration snapshot is invalid")
	}
	return snapshot, nil
}

type battlePassPaymentOrder struct {
	ID           int64
	UserID       int64
	OrderType    string
	Status       string
	PayAmount    float64
	RefundAmount float64
	UpdatedAt    time.Time
	CompletedAt  sql.NullTime
	EligibleUser bool
}

type battlePassAffiliateRelation struct {
	InviteeID        int64
	InviterID        int64
	CreatedAt        time.Time
	EligibleUser     bool
	HasValidRecharge bool
}

// GetCurrentForUser exposes only the current user state. It deliberately does
// not create a progress row while the season is scheduled, paused, or ended.
func (s *BattlePassService) GetCurrentForUser(ctx context.Context, userID int64, now time.Time) (*BattlePassCurrent, error) {
	if err := s.requireUserAccess(ctx, now); err != nil {
		return nil, err
	}
	if err := s.requireEligibleUser(ctx, userID); err != nil {
		return nil, err
	}
	season, err := s.currentPublishedSeason(ctx, now)
	if err != nil || season == nil {
		return &BattlePassCurrent{Season: season, Syncing: false, UserSideEnabled: true}, err
	}
	current := &BattlePassCurrent{Season: season, Syncing: false, UserSideEnabled: true}
	if runtimeSeasonStatus(*season, now) != "active" {
		current.Progress, err = s.loadUserProgress(ctx, season.ID, userID)
		if err != nil {
			return nil, err
		}
		return current, nil
	}
	progress, err := s.ensureUserProgress(ctx, season.ID, userID)
	if err != nil {
		return nil, err
	}
	current.Progress = progress
	return current, nil
}

func (s *BattlePassService) GetTasksForUser(ctx context.Context, userID int64, now time.Time) ([]BattlePassTaskState, error) {
	if err := s.requireUserAccess(ctx, now); err != nil {
		return nil, err
	}
	if err := s.requireEligibleUser(ctx, userID); err != nil {
		return nil, err
	}
	season, err := s.currentPublishedSeason(ctx, now)
	if err != nil || season == nil {
		return []BattlePassTaskState{}, err
	}
	return s.getTasksForSeasonUser(ctx, *season, userID, now)
}

func (s *BattlePassService) getTasksForSeasonUser(ctx context.Context, season BattlePassSeason, userID int64, now time.Time) ([]BattlePassTaskState, error) {
	location, err := time.LoadLocation(season.Timezone)
	if err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_CONFIG_INVALID", "battle pass timezone is invalid")
	}
	dailyKey := now.In(location).Format("2006-01-02")
	snapshot, err := loadBattlePassConfigSnapshot(ctx, s.db, season.ID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT task_id, period_key, current_value, completed
		FROM battle_pass_task_progress
		WHERE season_id=$1 AND user_id=$2 AND (period_key=$3 OR period_key='season')
	`, season.ID, userID, dailyKey)
	if err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to load battle pass tasks")
	}
	defer rows.Close()
	type progress struct {
		value     float64
		completed bool
		periodKey string
	}
	progressByTask := make(map[int64]progress)
	for rows.Next() {
		var taskID int64
		var item progress
		if err := rows.Scan(&taskID, &item.periodKey, &item.value, &item.completed); err != nil {
			return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to scan battle pass task progress")
		}
		progressByTask[taskID] = item
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	items := make([]BattlePassTaskState, 0, len(snapshot.Tasks))
	for _, task := range snapshot.Tasks {
		if !task.Enabled || task.ID <= 0 {
			continue
		}
		periodKey := "season"
		if task.PeriodType == "daily" {
			periodKey = dailyKey
		}
		item := BattlePassTaskState{ID: task.ID, Name: task.Name, Description: task.Description, TaskType: task.TaskType, PeriodType: task.PeriodType, TargetValue: task.TargetValue, ExpReward: task.ExpReward, FilterScope: task.FilterScope, FilterValues: task.FilterValues, DisplayOrder: task.DisplayOrder, PeriodKey: periodKey}
		if current, ok := progressByTask[task.ID]; ok && current.periodKey == periodKey {
			item.CurrentValue = current.value
			item.Completed = current.completed
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *BattlePassService) GetRewardsForUser(ctx context.Context, userID int64, now time.Time) ([]BattlePassRewardState, error) {
	if err := s.requireUserAccess(ctx, now); err != nil {
		return nil, err
	}
	if err := s.requireEligibleUser(ctx, userID); err != nil {
		return nil, err
	}
	season, err := s.currentPublishedSeason(ctx, now)
	if err != nil || season == nil {
		return []BattlePassRewardState{}, err
	}
	if runtimeSeasonStatus(*season, now) == "active" {
		if _, err := s.ensureUserProgress(ctx, season.ID, userID); err != nil {
			return nil, err
		}
	}
	return s.listRewardsForSeasonUser(ctx, season.ID, userID)
}

func (s *BattlePassService) GetHistoryForUser(ctx context.Context, userID int64, now time.Time) (*BattlePassHistory, error) {
	if err := s.requireUserAccess(ctx, now); err != nil {
		return nil, err
	}
	if err := s.requireEligibleUser(ctx, userID); err != nil {
		return nil, err
	}
	season, err := s.currentPublishedSeason(ctx, now)
	if err != nil || season == nil {
		return &BattlePassHistory{Experience: []BattlePassExperienceEntry{}, Purchases: []BattlePassPurchase{}, Rewards: []BattlePassRewardState{}}, err
	}
	return s.getHistoryForSeasonUser(ctx, season.ID, userID)
}

// GetHistoryForUserSeason returns history for a previously published season.
// Draft and archived seasons are intentionally hidden, and future scheduled
// seasons are not exposed before their public start time.
func (s *BattlePassService) GetHistoryForUserSeason(ctx context.Context, userID, seasonID int64, now time.Time) (*BattlePassHistory, error) {
	if err := s.requireUserAccess(ctx, now); err != nil {
		return nil, err
	}
	if err := s.requireEligibleUser(ctx, userID); err != nil {
		return nil, err
	}
	season, err := s.getSeason(ctx, seasonID, now)
	if err != nil {
		return nil, err
	}
	if season.Status == BattlePassStatusDraft || season.Status == BattlePassStatusArchived || season.StartAt.After(now) {
		return nil, infraerrors.NotFound("BATTLE_PASS_SEASON_NOT_FOUND", "battle pass season not found")
	}
	return s.getHistoryForSeasonUser(ctx, season.ID, userID)
}

func (s *BattlePassService) getHistoryForSeasonUser(ctx context.Context, seasonID, userID int64) (*BattlePassHistory, error) {
	history := &BattlePassHistory{Experience: []BattlePassExperienceEntry{}, Purchases: []BattlePassPurchase{}}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, task_id, period_key, exp_delta, reason, created_at
		FROM battle_pass_exp_ledger WHERE season_id=$1 AND user_id=$2
		ORDER BY id DESC LIMIT 100
	`, seasonID, userID)
	if err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to load battle pass history")
	}
	for rows.Next() {
		var item BattlePassExperienceEntry
		var taskID sql.NullInt64
		if err := rows.Scan(&item.ID, &taskID, &item.PeriodKey, &item.ExpDelta, &item.Reason, &item.CreatedAt); err != nil {
			rows.Close()
			return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to scan battle pass history")
		}
		if taskID.Valid {
			id := taskID.Int64
			item.TaskID = &id
		}
		history.Experience = append(history.Experience, item)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	purchaseRows, err := s.db.QueryContext(ctx, `
		SELECT id, season_id, price, balance_before, balance_after, idempotency_key, purchased_at
		FROM battle_pass_purchases WHERE season_id=$1 AND user_id=$2 ORDER BY id DESC
	`, seasonID, userID)
	if err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to load battle pass purchases")
	}
	for purchaseRows.Next() {
		var item BattlePassPurchase
		if err := purchaseRows.Scan(&item.ID, &item.SeasonID, &item.Price, &item.BalanceBefore, &item.BalanceAfter, &item.IdempotencyKey, &item.PurchasedAt); err != nil {
			purchaseRows.Close()
			return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to scan battle pass purchase")
		}
		history.Purchases = append(history.Purchases, item)
	}
	if err := purchaseRows.Close(); err != nil {
		return nil, err
	}
	history.Rewards, err = s.listRewardsForSeasonUser(ctx, seasonID, userID)
	return history, err
}

func (s *BattlePassService) ScanUsageOnce(ctx context.Context, now time.Time) (int, error) {
	enabled, err := s.IsEnabled(ctx)
	if err != nil || !enabled {
		return 0, err
	}
	season, err := s.currentPublishedSeason(ctx, now)
	if err != nil {
		return 0, err
	}
	processed := 0
	if season != nil && battlePassSeasonShouldScan(*season, now) {
		count, err := s.scanBattlePassSeasonOnce(ctx, *season, now)
		if err != nil {
			return processed, err
		}
		processed += count
	}
	// The normal current-season query intentionally returns only one season.
	// Drain older ended seasons separately so a busy final window cannot be
	// skipped merely because the next season has already become active.
	endedSeasons, err := s.listHistoricalEndedBattlePassSeasons(ctx, now)
	if err != nil {
		return processed, err
	}
	for _, ended := range endedSeasons {
		if season != nil && ended.ID == season.ID {
			continue
		}
		count, err := s.scanBattlePassSeasonOnce(ctx, ended, now)
		if err != nil {
			return processed, err
		}
		processed += count
	}
	return processed, nil
}

func (s *BattlePassService) scanBattlePassSeasonOnce(ctx context.Context, season BattlePassSeason, now time.Time) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to start battle pass scan")
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.requireEnabledTx(ctx, tx); err != nil {
		return 0, err
	}
	var locked bool
	if err := tx.QueryRowContext(ctx, `SELECT pg_try_advisory_xact_lock($1, $2)`, battlePassScanAdvisoryClass, season.ID).Scan(&locked); err != nil {
		return 0, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to acquire battle pass scan lock")
	}
	if !locked {
		return 0, nil
	}
	processed, err := s.scanUsageTx(ctx, tx, season, now)
	if err != nil {
		return 0, err
	}
	paymentProcessed, err := s.scanPaymentOrdersTx(ctx, tx, season, now)
	if err != nil {
		return 0, err
	}
	affiliateProcessed, err := s.scanAffiliateRelationsTx(ctx, tx, season, now)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to commit battle pass scan")
	}
	return processed + paymentProcessed + affiliateProcessed, nil
}

func (s *BattlePassService) listHistoricalEndedBattlePassSeasons(ctx context.Context, now time.Time) ([]BattlePassSeason, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, description, status, timezone, start_at, end_at, premium_price, max_level,
		       activation_epoch, published_at, enabled_at_snapshot
		FROM battle_pass_seasons
		WHERE status IN ('scheduled', 'ended') AND start_at <= $1 AND end_at <= $1
		ORDER BY end_at ASC, id ASC
		LIMIT 25
	`, now)
	if err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to list ended battle pass seasons")
	}
	defer rows.Close()
	items := make([]BattlePassSeason, 0)
	for rows.Next() {
		item, err := scanBattlePassSeason(rows)
		if err != nil {
			return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to scan ended battle pass season")
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to read ended battle pass seasons")
	}
	return items, nil
}

func battlePassSeasonShouldScan(season BattlePassSeason, now time.Time) bool {
	switch runtimeSeasonStatus(season, now) {
	case "active", BattlePassStatusEnded:
		// Continue draining records created before EndAt so the scheduler's
		// final interval cannot silently discard usage.
		return true
	default:
		return false
	}
}

func (s *BattlePassService) scanUsageTx(ctx context.Context, tx *sql.Tx, season BattlePassSeason, now time.Time) (int, error) {
	var statisticsStart time.Time
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(statistics_start_at, start_at) FROM battle_pass_seasons WHERE id=$1`, season.ID).Scan(&statisticsStart); err != nil {
		return 0, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to load battle pass statistics boundary")
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO battle_pass_source_cursors (season_id, source_type, activation_epoch)
		VALUES ($1, 'usage_log', $2)
		ON CONFLICT (season_id, source_type) DO NOTHING
	`, season.ID, season.ActivationEpoch); err != nil {
		return 0, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to initialize battle pass cursor")
	}
	var lastID int64
	var cursorEpoch int
	if err := tx.QueryRowContext(ctx, `
		SELECT last_id, activation_epoch FROM battle_pass_source_cursors
		WHERE season_id=$1 AND source_type='usage_log' FOR UPDATE
	`, season.ID).Scan(&lastID, &cursorEpoch); err != nil {
		return 0, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to lock battle pass cursor")
	}
	if cursorEpoch != season.ActivationEpoch {
		lastID = 0
		if _, err := tx.ExecContext(ctx, `UPDATE battle_pass_source_cursors SET last_id=0, activation_epoch=$2, updated_at=NOW() WHERE season_id=$1 AND source_type='usage_log'`, season.ID, season.ActivationEpoch); err != nil {
			return 0, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to reset battle pass cursor")
		}
	}
	tasks, err := loadBattlePassRuntimeTasks(ctx, tx, season.ID)
	if err != nil {
		return 0, err
	}
	if !hasBattlePassUsageTasks(tasks) {
		return 0, nil
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT ul.id, ul.user_id, COALESCE(NULLIF(ul.requested_model, ''), ul.model), ul.actual_cost,
		       ul.image_count, ul.video_count, ul.created_at, COALESCE(ul.request_type, 0),
		       (u.id IS NOT NULL),
		       EXISTS (
			SELECT 1 FROM battle_pass_pause_windows w
			WHERE w.season_id=$4 AND w.paused_at <= ul.created_at
			  AND (w.resumed_at IS NULL OR w.resumed_at > ul.created_at)
		)
		FROM usage_logs ul
		LEFT JOIN users u ON u.id=ul.user_id AND u.status='active' AND u.deleted_at IS NULL
		WHERE ul.id > $1
		  AND ul.created_at >= $2 AND ul.created_at < $3
		ORDER BY ul.id ASC LIMIT $5
	`, lastID, statisticsStart, season.EndAt, season.ID, battlePassUsageBatchSize)
	if err != nil {
		return 0, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to read usage logs for battle pass")
	}
	defer rows.Close()
	processed := 0
	maxID := lastID
	for rows.Next() {
		var usage battlePassUsageLog
		if err := rows.Scan(&usage.ID, &usage.UserID, &usage.Model, &usage.ActualCost, &usage.ImageCount, &usage.VideoCount, &usage.CreatedAt, &usage.RequestType, &usage.Eligible, &usage.Paused); err != nil {
			return 0, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to scan usage log for battle pass")
		}
		if usage.Eligible && usage.RequestType != int16(RequestTypeCyberBlocked) && !usage.Paused {
			for _, task := range tasks {
				if err := s.applyUsageContributionTx(ctx, tx, season, task, usage); err != nil {
					return 0, err
				}
			}
		}
		maxID = usage.ID
		processed++
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if processed > 0 {
		if _, err := tx.ExecContext(ctx, `
			UPDATE battle_pass_source_cursors
			SET last_id=$3, last_updated_at=$4, activation_epoch=$2, updated_at=NOW()
			WHERE season_id=$1 AND source_type='usage_log'
		`, season.ID, season.ActivationEpoch, maxID, now); err != nil {
			return 0, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to save battle pass cursor")
		}
	}
	return processed, nil
}

func hasBattlePassUsageTasks(tasks []battlePassRuntimeTask) bool {
	for _, task := range tasks {
		switch task.TaskType {
		case "request_count", "cost_amount", "active_days", "distinct_model_families", "image_count", "video_count":
			return true
		}
	}
	return false
}

func hasBattlePassPaymentTasks(tasks []battlePassRuntimeTask) bool {
	for _, task := range tasks {
		if task.TaskType == "recharge_count" || task.TaskType == "recharge_amount" {
			return true
		}
	}
	return false
}

func hasBattlePassAffiliateTasks(tasks []battlePassRuntimeTask) bool {
	for _, task := range tasks {
		if task.TaskType == "valid_invite_count" || task.TaskType == "invitee_recharge_count" {
			return true
		}
	}
	return false
}

func battlePassAffiliateContribution(eligibleUser, hasValidRecharge bool) float64 {
	if eligibleUser && hasValidRecharge {
		return 1
	}
	return 0
}

func battlePassPaymentStatusEligible(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "COMPLETED", "PARTIALLY_REFUNDED", "REFUNDED":
		return true
	default:
		return false
	}
}

func battlePassPaymentNetContribution(status string, payAmount, refundAmount float64) float64 {
	if !battlePassPaymentStatusEligible(status) || math.IsNaN(payAmount) || math.IsInf(payAmount, 0) || payAmount <= 0 {
		return 0
	}
	if math.IsNaN(refundAmount) || math.IsInf(refundAmount, 0) || refundAmount < 0 {
		refundAmount = 0
	}
	return math.Max(0, payAmount-refundAmount)
}

func battlePassPaymentContribution(taskType, status string, payAmount, refundAmount float64) float64 {
	net := battlePassPaymentNetContribution(status, payAmount, refundAmount)
	if taskType == "recharge_count" && net > 0 {
		return 1
	}
	if taskType == "recharge_amount" {
		return net
	}
	return 0
}

func (s *BattlePassService) scanPaymentOrdersTx(ctx context.Context, tx *sql.Tx, season BattlePassSeason, now time.Time) (int, error) {
	tasks, err := loadBattlePassRuntimeTasks(ctx, tx, season.ID)
	if err != nil {
		return 0, err
	}
	if !hasBattlePassPaymentTasks(tasks) {
		return 0, nil
	}
	var statisticsStart time.Time
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(statistics_start_at, start_at) FROM battle_pass_seasons WHERE id=$1`, season.ID).Scan(&statisticsStart); err != nil {
		return 0, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to load battle pass statistics boundary")
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO battle_pass_source_cursors (season_id, source_type, activation_epoch)
		VALUES ($1, 'payment_order', $2) ON CONFLICT (season_id, source_type) DO NOTHING
	`, season.ID, season.ActivationEpoch); err != nil {
		return 0, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to initialize payment cursor")
	}
	var lastID int64
	var lastUpdated sql.NullTime
	var cursorEpoch int
	if err := tx.QueryRowContext(ctx, `SELECT last_id, last_updated_at, activation_epoch FROM battle_pass_source_cursors WHERE season_id=$1 AND source_type='payment_order' FOR UPDATE`, season.ID).Scan(&lastID, &lastUpdated, &cursorEpoch); err != nil {
		return 0, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to lock payment cursor")
	}
	if cursorEpoch != season.ActivationEpoch {
		lastID = 0
		lastUpdated = sql.NullTime{}
		if _, err := tx.ExecContext(ctx, `UPDATE battle_pass_source_cursors SET last_id=0, last_updated_at=NULL, activation_epoch=$2, updated_at=NOW() WHERE season_id=$1 AND source_type='payment_order'`, season.ID, season.ActivationEpoch); err != nil {
			return 0, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to reset payment cursor")
		}
	}
	cutoff := now.Add(-battlePassPaymentConfirmationDelay)
	rows, err := tx.QueryContext(ctx, `
		SELECT po.id, po.user_id, po.order_type, po.status, po.pay_amount, po.refund_amount, po.updated_at, po.completed_at,
		       (u.id IS NOT NULL)
		FROM payment_orders po
		LEFT JOIN users u ON u.id=po.user_id AND u.status='active' AND u.deleted_at IS NULL
		WHERE po.order_type='balance' AND po.updated_at <= $1 AND po.updated_at >= $2
		  AND ($3::timestamptz IS NULL OR po.updated_at > $3 OR (po.updated_at = $3 AND po.id > $4))
		ORDER BY po.updated_at ASC, po.id ASC LIMIT $5
	`, cutoff, statisticsStart, nullTimeArg(lastUpdated), lastID, battlePassUsageBatchSize)
	if err != nil {
		return 0, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to read payment orders for battle pass")
	}
	defer rows.Close()
	processed := 0
	var maxUpdated time.Time
	maxID := lastID
	for rows.Next() {
		var order battlePassPaymentOrder
		if err := rows.Scan(&order.ID, &order.UserID, &order.OrderType, &order.Status, &order.PayAmount, &order.RefundAmount, &order.UpdatedAt, &order.CompletedAt, &order.EligibleUser); err != nil {
			return 0, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to scan payment order for battle pass")
		}
		for _, task := range tasks {
			if task.TaskType != "recharge_count" && task.TaskType != "recharge_amount" || task.PeriodType != "season" {
				continue
			}
			value := battlePassPaymentContribution(task.TaskType, order.Status, order.PayAmount, order.RefundAmount)
			if !order.EligibleUser || !order.CompletedAt.Valid || order.CompletedAt.Time.Before(statisticsStart) || !order.CompletedAt.Time.Before(season.EndAt) {
				value = 0
			}
			if err := s.applyBattlePassSourceContributionTx(ctx, tx, season, task, order.UserID, "payment_order", order.ID, order.UpdatedAt, value); err != nil {
				return 0, err
			}
		}
		processed++
		maxUpdated, maxID = order.UpdatedAt, order.ID
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if processed > 0 {
		if _, err := tx.ExecContext(ctx, `UPDATE battle_pass_source_cursors SET last_id=$3, last_updated_at=$4, activation_epoch=$2, updated_at=NOW() WHERE season_id=$1 AND source_type='payment_order'`, season.ID, season.ActivationEpoch, maxID, maxUpdated); err != nil {
			return 0, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to save payment cursor")
		}
	}
	return processed, nil
}

func nullTimeArg(value sql.NullTime) any {
	if !value.Valid {
		return nil
	}
	return value.Time
}

func (s *BattlePassService) scanAffiliateRelationsTx(ctx context.Context, tx *sql.Tx, season BattlePassSeason, now time.Time) (int, error) {
	tasks, err := loadBattlePassRuntimeTasks(ctx, tx, season.ID)
	if err != nil {
		return 0, err
	}
	if !hasBattlePassAffiliateTasks(tasks) {
		return 0, nil
	}
	var affiliateEnabled string
	if err := tx.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=$1`, SettingKeyAffiliateEnabled).Scan(&affiliateEnabled); err != nil && err != sql.ErrNoRows {
		return 0, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to read affiliate setting")
	}
	if strings.TrimSpace(affiliateEnabled) != "true" {
		return 0, nil
	}
	var statisticsStart time.Time
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(statistics_start_at, start_at) FROM battle_pass_seasons WHERE id=$1`, season.ID).Scan(&statisticsStart); err != nil {
		return 0, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to load battle pass statistics boundary")
	}
	cutoff := now.Add(-battlePassPaymentConfirmationDelay)
	rows, err := tx.QueryContext(ctx, `
		SELECT ua.user_id, ua.inviter_id, ua.created_at,
		       (invitee.id IS NOT NULL),
		       EXISTS (SELECT 1 FROM payment_orders po WHERE po.user_id=ua.user_id AND po.order_type='balance'
		           AND po.status IN ('COMPLETED','PARTIALLY_REFUNDED','REFUNDED')
		           AND po.completed_at >= GREATEST($1, ua.created_at) AND po.completed_at < $2 AND po.updated_at <= $3
		           AND GREATEST(po.pay_amount - GREATEST(po.refund_amount, 0), 0) > 0)
		FROM user_affiliates ua
		LEFT JOIN users invitee ON invitee.id=ua.user_id AND invitee.status='active' AND invitee.deleted_at IS NULL
		JOIN users inviter ON inviter.id=ua.inviter_id AND inviter.status='active' AND inviter.deleted_at IS NULL
		WHERE ua.inviter_id IS NOT NULL AND ua.created_at >= $1 AND ua.created_at < $2
	`, statisticsStart, season.EndAt, cutoff)
	if err != nil {
		return 0, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to read affiliate relations for battle pass")
	}
	defer rows.Close()
	processed := 0
	for rows.Next() {
		var relation battlePassAffiliateRelation
		if err := rows.Scan(&relation.InviteeID, &relation.InviterID, &relation.CreatedAt, &relation.EligibleUser, &relation.HasValidRecharge); err != nil {
			return 0, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to scan affiliate relation for battle pass")
		}
		value := battlePassAffiliateContribution(relation.EligibleUser, relation.HasValidRecharge)
		for _, task := range tasks {
			if task.TaskType != "valid_invite_count" && task.TaskType != "invitee_recharge_count" || task.PeriodType != "season" {
				continue
			}
			if err := s.applyBattlePassSourceContributionTx(ctx, tx, season, task, relation.InviterID, "affiliate", relation.InviteeID, relation.CreatedAt, value); err != nil {
				return 0, err
			}
		}
		processed++
	}
	return processed, rows.Err()
}

func (s *BattlePassService) applyBattlePassSourceContributionTx(ctx context.Context, tx *sql.Tx, season BattlePassSeason, task battlePassRuntimeTask, userID int64, sourceType string, sourceID int64, sourceUpdatedAt time.Time, value float64) error {
	var oldValue float64
	var exists bool
	err := tx.QueryRowContext(ctx, `SELECT contribution_value FROM battle_pass_source_contributions WHERE season_id=$1 AND task_id=$2 AND user_id=$3 AND source_type=$4 AND source_id=$5 FOR UPDATE`, season.ID, task.ID, userID, sourceType, sourceID).Scan(&oldValue)
	if err == sql.ErrNoRows {
		err = nil
	} else {
		exists = err == nil
	}
	if err != nil {
		return infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to load battle pass source contribution")
	}
	if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		value = 0
	}
	delta := value - oldValue
	if !exists && value == 0 {
		return nil
	}
	if exists {
		if _, err := tx.ExecContext(ctx, `UPDATE battle_pass_source_contributions SET contribution_value=$6, source_updated_at=$7, updated_at=NOW() WHERE season_id=$1 AND task_id=$2 AND user_id=$3 AND source_type=$4 AND source_id=$5`, season.ID, task.ID, userID, sourceType, sourceID, value, sourceUpdatedAt); err != nil {
			return infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to update battle pass source contribution")
		}
	} else if _, err := tx.ExecContext(ctx, `INSERT INTO battle_pass_source_contributions (season_id, task_id, user_id, source_type, source_id, contribution_value, source_updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7)`, season.ID, task.ID, userID, sourceType, sourceID, value, sourceUpdatedAt); err != nil {
		return infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to insert battle pass source contribution")
	}
	if delta == 0 {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO battle_pass_user_progress (season_id,user_id) VALUES ($1,$2) ON CONFLICT (season_id,user_id) DO NOTHING`, season.ID, userID); err != nil {
		return infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to create battle pass progress")
	}
	var completed bool
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO battle_pass_task_progress (season_id,task_id,user_id,period_key,current_value,completed,completed_at)
		VALUES ($1,$2,$3,'season',GREATEST($4,0),GREATEST($4,0)>=$5,CASE WHEN GREATEST($4,0)>=$5 THEN NOW() ELSE NULL END)
		ON CONFLICT (season_id,task_id,user_id,period_key) DO UPDATE SET current_value=GREATEST(0,battle_pass_task_progress.current_value+$4), completed=battle_pass_task_progress.completed OR battle_pass_task_progress.current_value+$4 >= $5, completed_at=CASE WHEN NOT battle_pass_task_progress.completed AND battle_pass_task_progress.current_value+$4 >= $5 THEN NOW() ELSE battle_pass_task_progress.completed_at END, updated_at=NOW()
		RETURNING completed`, season.ID, task.ID, userID, delta, task.TargetValue).Scan(&completed); err != nil {
		return infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to update battle pass task progress")
	}
	if delta > 0 && completed {
		return s.awardTaskExperienceTx(ctx, tx, season.ID, userID, task.ID, "season", task.ExpReward)
	}
	return nil
}

func loadBattlePassRuntimeTasks(ctx context.Context, tx *sql.Tx, seasonID int64) ([]battlePassRuntimeTask, error) {
	snapshot, err := loadBattlePassConfigSnapshot(ctx, tx, seasonID)
	if err != nil {
		return nil, err
	}
	items := make([]battlePassRuntimeTask, 0, len(snapshot.Tasks))
	for _, task := range snapshot.Tasks {
		if !task.Enabled {
			continue
		}
		if task.FilterValues == nil {
			task.FilterValues = []string{}
		}
		// IDs are stable within the published season and are required to keep
		// task progress and experience idempotency tied to the same task row.
		if task.ID <= 0 {
			return nil, infraerrors.InternalServer("BATTLE_PASS_CONFIG_INVALID", "published task is missing its id")
		}
		items = append(items, battlePassRuntimeTask{ID: task.ID, BattlePassTaskInput: task})
	}
	return items, nil
}

func (s *BattlePassService) applyUsageContributionTx(ctx context.Context, tx *sql.Tx, season BattlePassSeason, task battlePassRuntimeTask, usage battlePassUsageLog) error {
	location, err := time.LoadLocation(season.Timezone)
	if err != nil {
		return infraerrors.InternalServer("BATTLE_PASS_CONFIG_INVALID", "battle pass timezone is invalid")
	}
	periodKey := "season"
	if task.PeriodType == "daily" {
		periodKey = usage.CreatedAt.In(location).Format("2006-01-02")
	}
	family := battlePassModelFamily(usage.Model)
	if !battlePassTaskMatches(task.BattlePassTaskInput, usage.Model, family) {
		return nil
	}
	value, sourceType, sourceID := battlePassUsageContribution(task.TaskType, usage, family)
	if task.TaskType == "active_days" {
		// Active-days tasks use a season progress bucket, but their source key
		// must remain one-per-local-calendar-day so multiple days accumulate.
		sourceID = battlePassActiveDaySourceID(usage.UserID, usage.CreatedAt, location)
	}
	if task.TaskType == "distinct_model_families" && task.PeriodType == "daily" {
		sourceID = battlePassStableID(fmt.Sprintf("%d:%s:%s", usage.UserID, family, periodKey))
	}
	if value <= 0 {
		return nil
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO battle_pass_source_contributions (season_id, task_id, user_id, source_type, source_id, contribution_value)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (season_id, task_id, user_id, source_type, source_id) DO NOTHING
	`, season.ID, task.ID, usage.UserID, sourceType, sourceID, value)
	if err != nil {
		return infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to record battle pass source contribution")
	}
	inserted, err := result.RowsAffected()
	if err != nil || inserted == 0 {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO battle_pass_user_progress (season_id, user_id) VALUES ($1,$2)
		ON CONFLICT (season_id, user_id) DO NOTHING
	`, season.ID, usage.UserID); err != nil {
		return infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to create battle pass progress")
	}
	var completed bool
	err = tx.QueryRowContext(ctx, `
		INSERT INTO battle_pass_task_progress (season_id, task_id, user_id, period_key, current_value, completed, completed_at)
		VALUES ($1,$2,$3,$4,$5,$5 >= $6, CASE WHEN $5 >= $6 THEN NOW() ELSE NULL END)
		ON CONFLICT (season_id, task_id, user_id, period_key) DO UPDATE
		SET current_value=battle_pass_task_progress.current_value + EXCLUDED.current_value,
		    completed=battle_pass_task_progress.completed OR battle_pass_task_progress.current_value + EXCLUDED.current_value >= $6,
		    completed_at=CASE WHEN NOT battle_pass_task_progress.completed AND battle_pass_task_progress.current_value + EXCLUDED.current_value >= $6 THEN NOW() ELSE battle_pass_task_progress.completed_at END,
		    updated_at=NOW()
		RETURNING completed
	`, season.ID, task.ID, usage.UserID, periodKey, value, task.TargetValue).Scan(&completed)
	if err != nil {
		return infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to update battle pass task progress")
	}
	if completed {
		return s.awardTaskExperienceTx(ctx, tx, season.ID, usage.UserID, task.ID, periodKey, task.ExpReward)
	}
	return nil
}

func (s *BattlePassService) awardTaskExperienceTx(ctx context.Context, tx *sql.Tx, seasonID, userID, taskID int64, periodKey string, exp int64) error {
	key := fmt.Sprintf("task:%d:user:%d:period:%s", taskID, userID, periodKey)
	result, err := tx.ExecContext(ctx, `
		INSERT INTO battle_pass_exp_ledger (season_id, user_id, task_id, period_key, exp_delta, reason, idempotency_key)
		VALUES ($1,$2,$3,$4,$5,'task_complete',$6)
		ON CONFLICT (season_id, idempotency_key) DO NOTHING
	`, seasonID, userID, taskID, periodKey, exp, key)
	if err != nil {
		return infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to record battle pass experience")
	}
	inserted, err := result.RowsAffected()
	if err != nil || inserted == 0 {
		return err
	}
	snapshot, err := loadBattlePassConfigSnapshot(ctx, tx, seasonID)
	if err != nil {
		return err
	}
	if len(snapshot.Levels) == 0 {
		return infraerrors.InternalServer("BATTLE_PASS_CONFIG_INVALID", "published level configuration is empty")
	}
	var currentExp int64
	if err := tx.QueryRowContext(ctx, `
		SELECT exp FROM battle_pass_user_progress
		WHERE season_id=$1 AND user_id=$2 FOR UPDATE
	`, seasonID, userID).Scan(&currentExp); err != nil {
		return infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to lock battle pass progress")
	}
	level := battlePassLevelForExp(snapshot.Levels, currentExp+exp)
	if _, err := tx.ExecContext(ctx, `
		UPDATE battle_pass_user_progress p
		SET exp=p.exp+$3,
		    level=GREATEST(p.level, $4),
		    updated_at=NOW()
		WHERE p.season_id=$1 AND p.user_id=$2
	`, seasonID, userID, exp, level); err != nil {
		return infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to update battle pass level")
	}
	return nil
}

func battlePassLevelForExp(levels []BattlePassLevelInput, exp int64) int {
	level := 1
	for _, item := range levels {
		if item.Level > level && item.RequiredExp >= 0 && item.RequiredExp <= exp {
			level = item.Level
		}
	}
	return level
}

func battlePassUsageContribution(taskType string, usage battlePassUsageLog, family string) (float64, string, int64) {
	switch taskType {
	case "request_count":
		return 1, "usage_log", usage.ID
	case "cost_amount":
		return math.Max(0, usage.ActualCost), "usage_log", usage.ID
	case "image_count":
		return float64(max(0, usage.ImageCount)), "usage_log", usage.ID
	case "video_count":
		return float64(max(0, usage.VideoCount)), "usage_log", usage.ID
	case "active_days":
		return 1, "usage_active_day", battlePassStableID(fmt.Sprintf("%d:%s", usage.UserID, usage.CreatedAt.UTC().Format("2006-01-02")))
	case "distinct_model_families":
		if family == "other" {
			return 0, "", 0
		}
		return 1, "usage_model_family", battlePassStableID(fmt.Sprintf("%d:%s", usage.UserID, family))
	default:
		return 0, "", 0
	}
}

func battlePassActiveDaySourceID(userID int64, at time.Time, location *time.Location) int64 {
	if location == nil {
		location = time.UTC
	}
	return battlePassStableID(fmt.Sprintf("%d:%s", userID, at.In(location).Format("2006-01-02")))
}

func battlePassTaskMatches(task BattlePassTaskInput, model, family string) bool {
	switch emptyDefault(task.FilterScope, "all") {
	case "all":
		return true
	case "model_family":
		for _, value := range task.FilterValues {
			if strings.EqualFold(strings.TrimSpace(value), family) {
				return true
			}
		}
	case "exact_model":
		for _, value := range task.FilterValues {
			if strings.EqualFold(strings.TrimSpace(value), model) {
				return true
			}
		}
	}
	return false
}

func battlePassModelFamily(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.HasPrefix(model, "gpt"), strings.Contains(model, "openai"):
		return "openai"
	case strings.HasPrefix(model, "claude"):
		return "anthropic"
	case strings.HasPrefix(model, "gemini"):
		return "gemini"
	case strings.HasPrefix(model, "grok"):
		return "grok"
	case strings.HasPrefix(model, "deepseek"):
		return "deepseek"
	case strings.HasPrefix(model, "qwen"), strings.HasPrefix(model, "kimi"):
		return "cn_compatible"
	default:
		return "other"
	}
}

func battlePassStableID(value string) int64 {
	digest := sha256.Sum256([]byte(value))
	return int64(binary.BigEndian.Uint64(digest[:8]) & math.MaxInt64)
}

func (s *BattlePassService) ensureUserProgress(ctx context.Context, seasonID, userID int64) (*BattlePassUserProgress, error) {
	if s == nil || s.db == nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "battle pass database is unavailable")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to start battle pass progress transaction")
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.requireEnabledTx(ctx, tx); err != nil {
		return nil, err
	}
	progress, err := s.ensureUserProgressTx(ctx, tx, seasonID, userID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to commit battle pass progress")
	}
	return progress, nil
}

func (s *BattlePassService) ensureUserProgressTx(ctx context.Context, tx *sql.Tx, seasonID, userID int64) (*BattlePassUserProgress, error) {
	if tx == nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "battle pass database is unavailable")
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO battle_pass_user_progress (season_id, user_id)
		SELECT $1, id FROM users WHERE id=$2 AND status='active' AND deleted_at IS NULL
		ON CONFLICT (season_id, user_id) DO NOTHING
	`, seasonID, userID); err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to initialize battle pass progress")
	}
	var progress BattlePassUserProgress
	err := tx.QueryRowContext(ctx, `
		SELECT exp, level, premium_unlocked, updated_at FROM battle_pass_user_progress WHERE season_id=$1 AND user_id=$2
	`, seasonID, userID).Scan(&progress.Exp, &progress.Level, &progress.PremiumUnlocked, &progress.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, infraerrors.Forbidden("BATTLE_PASS_INELIGIBLE", "user is not eligible for battle pass")
	}
	if err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to load battle pass progress")
	}
	if err := s.populateProgressBoundsWithQueryer(ctx, tx, seasonID, &progress); err != nil {
		return nil, err
	}
	return &progress, nil
}

func (s *BattlePassService) loadUserProgress(ctx context.Context, seasonID, userID int64) (*BattlePassUserProgress, error) {
	var progress BattlePassUserProgress
	err := s.db.QueryRowContext(ctx, `
		SELECT exp, level, premium_unlocked, updated_at
		FROM battle_pass_user_progress WHERE season_id=$1 AND user_id=$2
	`, seasonID, userID).Scan(&progress.Exp, &progress.Level, &progress.PremiumUnlocked, &progress.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to load battle pass progress")
	}
	if err := s.populateProgressBounds(ctx, seasonID, &progress); err != nil {
		return nil, err
	}
	return &progress, nil
}

func (s *BattlePassService) populateProgressBounds(ctx context.Context, seasonID int64, progress *BattlePassUserProgress) error {
	return s.populateProgressBoundsWithQueryer(ctx, s.db, seasonID, progress)
}

func (s *BattlePassService) populateProgressBoundsWithQueryer(ctx context.Context, q battlePassRowQueryer, seasonID int64, progress *BattlePassUserProgress) error {
	if progress == nil {
		return nil
	}
	snapshot, err := loadBattlePassConfigSnapshot(ctx, q, seasonID)
	if err != nil {
		return err
	}
	progress.LevelStartExp, progress.NextLevelExp = battlePassLevelBounds(snapshot.Levels, progress.Level)
	return nil
}

func battlePassLevelBounds(levels []BattlePassLevelInput, level int) (int64, *int64) {
	if level < 1 {
		level = 1
	}
	var start int64
	var foundStart bool
	for _, item := range levels {
		if item.Level == level && item.RequiredExp >= 0 {
			start = item.RequiredExp
			foundStart = true
			break
		}
	}
	if !foundStart {
		start = 0
	}
	var nextValue int64
	foundNext := false
	for _, item := range levels {
		if item.Level > level && item.RequiredExp >= 0 {
			next := item.RequiredExp
			if next > start {
				if !foundNext || next < nextValue {
					nextValue = next
					foundNext = true
				}
			}
		}
	}
	if foundNext {
		return start, &nextValue
	}
	return start, nil
}

func (s *BattlePassService) requireEligibleUser(ctx context.Context, userID int64) error {
	if s == nil || s.db == nil || userID <= 0 {
		return infraerrors.Forbidden("BATTLE_PASS_INELIGIBLE", "user is not eligible for battle pass")
	}
	var exists bool
	// Battle-pass participation is a personal, authenticated-user feature. An
	// administrator can use it for local acceptance or a normal account, but
	// still has to pass the same active/not-deleted account gate as everyone
	// else. Admin-only mutation routes remain protected separately by admin and
	// step-up middleware.
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND status='active' AND deleted_at IS NULL)`, userID).Scan(&exists)
	if err != nil {
		return infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to check battle pass eligibility")
	}
	if !exists {
		return infraerrors.Forbidden("BATTLE_PASS_INELIGIBLE", "user is not eligible for battle pass")
	}
	return nil
}

func (s *BattlePassService) listRewardsForSeasonUser(ctx context.Context, seasonID, userID int64) ([]BattlePassRewardState, error) {
	snapshot, err := loadBattlePassConfigSnapshot(ctx, s.db, seasonID)
	if err != nil {
		return nil, err
	}
	level := 0
	premiumUnlocked := false
	err = s.db.QueryRowContext(ctx, `
		SELECT level, premium_unlocked FROM battle_pass_user_progress
		WHERE season_id=$1 AND user_id=$2
	`, seasonID, userID).Scan(&level, &premiumUnlocked)
	if err != nil && err != sql.ErrNoRows {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to load battle pass reward eligibility")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT reward_id, status, last_error
		FROM battle_pass_reward_grants WHERE season_id=$1 AND user_id=$2
	`, seasonID, userID)
	if err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to load battle pass reward grants")
	}
	defer rows.Close()
	grants := make(map[int64]battlePassGrantDisplayState)
	for rows.Next() {
		var rewardID int64
		var state battlePassGrantDisplayState
		if err := rows.Scan(&rewardID, &state.status, &state.lastError); err != nil {
			return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to scan battle pass reward grant")
		}
		grants[rewardID] = state
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return buildBattlePassRewardStates(snapshot.Rewards, level, premiumUnlocked, grants)
}

func buildBattlePassRewardStates(rewards []BattlePassRewardInput, level int, premiumUnlocked bool, grants map[int64]battlePassGrantDisplayState) ([]BattlePassRewardState, error) {
	items := make([]BattlePassRewardState, 0, len(rewards))
	for _, reward := range rewards {
		if reward.ID <= 0 {
			return nil, infraerrors.InternalServer("BATTLE_PASS_CONFIG_INVALID", "published reward is missing its id")
		}
		state := BattlePassRewardState{ID: reward.ID, Level: reward.Level, Track: reward.Track, RewardType: reward.RewardType, Payload: reward.Payload, Status: "locked"}
		if state.Payload == nil {
			state.Payload = map[string]any{}
		}
		if grant, ok := grants[reward.ID]; ok {
			state.Status, state.LastError = grant.status, grant.lastError
		} else if reward.Level <= level {
			if reward.Track == "premium" && !premiumUnlocked {
				state.Status = "premium_locked"
			} else {
				state.Status = "claimable"
			}
		}
		items = append(items, state)
	}
	return items, nil
}

func claimableBattlePassRewardIDs(states []BattlePassRewardState) []int64 {
	ids := make([]int64, 0, len(states))
	for _, state := range states {
		if state.Status == "claimable" && state.ID > 0 {
			ids = append(ids, state.ID)
		}
	}
	return ids
}
