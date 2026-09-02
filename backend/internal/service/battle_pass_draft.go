package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var battlePassAllowedTaskTypes = map[string]struct{}{
	"request_count":           {},
	"cost_amount":             {},
	"active_days":             {},
	"distinct_model_families": {},
	"image_count":             {},
	"video_count":             {},
	"recharge_count":          {},
	"recharge_amount":         {},
	"valid_invite_count":      {},
	"invitee_recharge_count":  {},
}

var battlePassAllowedRewardTypes = map[string]struct{}{
	"balance":           {},
	"concurrency":       {},
	"subscription_days": {},
	"badge":             {},
	"title":             {},
}

const (
	battlePassMaxTasksPerSeason   = 50
	battlePassMaxRewardsPerSeason = 200
	battlePassMaxTaskTarget       = 1_000_000_000
	battlePassMaxTaskExp          = 1_000_000
	battlePassMaxBalanceReward    = 10_000
	battlePassMaxConcurrencyAward = 100
)

type BattlePassLevelInput struct {
	Level       int   `json:"level"`
	RequiredExp int64 `json:"required_exp"`
}

type BattlePassTaskInput struct {
	ID           int64    `json:"id,omitempty"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	TaskType     string   `json:"task_type"`
	PeriodType   string   `json:"period_type"`
	TargetValue  float64  `json:"target_value"`
	ExpReward    int64    `json:"exp_reward"`
	FilterScope  string   `json:"filter_scope"`
	FilterValues []string `json:"filter_values"`
	DisplayOrder int      `json:"display_order"`
	Enabled      bool     `json:"enabled"`
}

type BattlePassRewardInput struct {
	ID         int64          `json:"id,omitempty"`
	Level      int            `json:"level"`
	Track      string         `json:"track"`
	RewardType string         `json:"reward_type"`
	Payload    map[string]any `json:"payload"`
}

type BattlePassSeasonDraft struct {
	BattlePassSeasonInput
	Levels  []BattlePassLevelInput  `json:"levels"`
	Tasks   []BattlePassTaskInput   `json:"tasks"`
	Rewards []BattlePassRewardInput `json:"rewards"`
}

type BattlePassSeasonDetail struct {
	BattlePassSeason
	Levels  []BattlePassLevelInput  `json:"levels"`
	Tasks   []BattlePassTaskInput   `json:"tasks"`
	Rewards []BattlePassRewardInput `json:"rewards"`
}

type BattlePassValidationIssue struct {
	Level   string `json:"level"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type BattlePassValidationResult struct {
	OK       bool                        `json:"ok"`
	Errors   []BattlePassValidationIssue `json:"errors"`
	Warnings []BattlePassValidationIssue `json:"warnings"`
}

func (s *BattlePassService) GetSeason(ctx context.Context, id int64, now time.Time) (*BattlePassSeasonDetail, error) {
	season, err := s.getSeason(ctx, id, now)
	if err != nil {
		return nil, err
	}
	detail := &BattlePassSeasonDetail{BattlePassSeason: *season}
	if err := s.loadSeasonChildren(ctx, detail); err != nil {
		return nil, err
	}
	return detail, nil
}

func (s *BattlePassService) UpdateSeason(ctx context.Context, id int64, draft BattlePassSeasonDraft, now time.Time) (*BattlePassSeasonDetail, error) {
	season, err := s.getSeason(ctx, id, now)
	if err != nil {
		return nil, err
	}
	if season.Status != BattlePassStatusDraft {
		return nil, infraerrors.BadRequest("BATTLE_PASS_SEASON_LOCKED", "only draft seasons can be fully edited")
	}
	draft.BattlePassSeasonInput = normalizeBattlePassSeasonInput(draft.BattlePassSeasonInput)
	if err := validateDraftSeason(draft.BattlePassSeasonInput, now); err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to update battle pass season")
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockBattlePassSeasonMutation(ctx, tx); err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to lock battle pass draft")
	}
	updateResult, err := tx.ExecContext(ctx, `
		UPDATE battle_pass_seasons
		SET name=$1, description=$2, timezone=$3, start_at=$4, end_at=$5, premium_price=$6, max_level=$7, updated_at=NOW()
		WHERE id=$8 AND status='draft'
	`, draft.Name, draft.Description, draft.Timezone, draft.StartAt, draft.EndAt, draft.PremiumPrice, draft.MaxLevel, id)
	if err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to update battle pass season")
	}
	// A concurrent publish changes the row state before this update can replace
	// children. Treat a zero-row update as a conflict, never mutate published
	// configuration after the fact.
	rows, err := updateResult.RowsAffected()
	if err != nil || rows != 1 {
		return nil, infraerrors.Conflict("BATTLE_PASS_SEASON_LOCKED", "season was published while the draft was being updated")
	}
	if err := replaceSeasonChildren(ctx, tx, id, draft); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to update battle pass season")
	}
	return s.GetSeason(ctx, id, now)
}

func (s *BattlePassService) ValidateSeason(ctx context.Context, id int64, now time.Time) (*BattlePassValidationResult, error) {
	detail, err := s.GetSeason(ctx, id, now)
	if err != nil {
		return nil, err
	}
	result := validateSeasonForPublish(*detail, now)
	if overlap, err := s.hasOverlappingPublishedSeason(ctx, detail.ID, detail.StartAt, detail.EndAt); err != nil {
		return nil, err
	} else if overlap {
		result.Errors = append(result.Errors, BattlePassValidationIssue{Level: "error", Code: "OVERLAP", Message: "season time range overlaps another published season"})
		result.OK = false
	}
	return &result, nil
}

func (s *BattlePassService) PublishSeason(ctx context.Context, id int64, now time.Time) (*BattlePassSeasonDetail, error) {
	if s == nil || s.db == nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "battle pass database is unavailable")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to publish battle pass season")
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockBattlePassSeasonMutation(ctx, tx); err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to lock battle pass publish")
	}
	detail, err := s.GetSeason(ctx, id, now)
	if err != nil {
		return nil, err
	}
	if detail.Status != BattlePassStatusDraft {
		return nil, infraerrors.BadRequest("BATTLE_PASS_SEASON_LOCKED", "only draft seasons can be published")
	}
	result := validateSeasonForPublish(*detail, now)
	if overlap, err := s.hasOverlappingPublishedSeason(ctx, detail.ID, detail.StartAt, detail.EndAt); err != nil {
		return nil, err
	} else if overlap {
		result.Errors = append(result.Errors, BattlePassValidationIssue{Level: "error", Code: "OVERLAP", Message: "season time range overlaps another published season"})
		result.OK = false
	}
	if !result.OK {
		return nil, infraerrors.BadRequest("BATTLE_PASS_SEASON_INVALID", firstValidationMessage(&result))
	}
	snapshot, err := json.Marshal(map[string]any{"levels": detail.Levels, "tasks": detail.Tasks, "rewards": detail.Rewards})
	if err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_SNAPSHOT_FAILED", "failed to snapshot season config")
	}
	publishResult, err := tx.ExecContext(ctx, `
		UPDATE battle_pass_seasons
		SET status='scheduled', published_at=NOW(), config_snapshot=$1, config_version=config_version+1, updated_at=NOW()
		WHERE id=$2 AND status='draft'
	`, snapshot, id)
	if err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to publish battle pass season")
	}
	rows, err := publishResult.RowsAffected()
	if err != nil || rows != 1 {
		return nil, infraerrors.Conflict("BATTLE_PASS_SEASON_LOCKED", "season was modified while publishing")
	}
	if err := tx.Commit(); err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to publish battle pass season")
	}
	return s.GetSeason(ctx, id, now)
}

// A single advisory lock serializes draft updates and publish operations. It
// prevents two otherwise valid, overlapping seasons from being published in
// parallel without imposing locks on unrelated application tables.
func lockBattlePassSeasonMutation(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(254, 1)`)
	return err
}

func (s *BattlePassService) PauseSeason(ctx context.Context, id int64, adminID int64, reason string, now time.Time) (*BattlePassSeasonDetail, error) {
	detail, err := s.GetSeason(ctx, id, now)
	if err != nil {
		return nil, err
	}
	if detail.Status != BattlePassStatusScheduled {
		return nil, infraerrors.BadRequest("BATTLE_PASS_SEASON_LOCKED", "only scheduled seasons can be paused")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to pause battle pass season")
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockBattlePassSeasonMutation(ctx, tx); err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to lock battle pass season")
	}
	if err := pauseBattlePassSeasonTx(ctx, tx, id, adminID, reason, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to pause battle pass season")
	}
	return s.GetSeason(ctx, id, now)
}

func pauseBattlePassSeasonTx(ctx context.Context, tx *sql.Tx, id, adminID int64, reason string, now time.Time) error {
	result, err := tx.ExecContext(ctx, `
		UPDATE battle_pass_seasons SET status='paused', activation_epoch=activation_epoch+1, updated_at=NOW()
		WHERE id=$1 AND status='scheduled'
	`, id)
	if err != nil {
		return infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to pause battle pass season")
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return infraerrors.Conflict("BATTLE_PASS_SEASON_LOCKED", "season was modified while pausing")
	}
	var pausedBy any
	if adminID > 0 {
		pausedBy = adminID
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO battle_pass_pause_windows (season_id, paused_at, paused_by, reason)
		VALUES ($1,$2,$3,$4)
	`, id, now, pausedBy, strings.TrimSpace(reason)); err != nil {
		return infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to record pause window")
	}
	return nil
}

func (s *BattlePassService) ResumeSeason(ctx context.Context, id int64, now time.Time) (*BattlePassSeasonDetail, error) {
	detail, err := s.GetSeason(ctx, id, now)
	if err != nil {
		return nil, err
	}
	if detail.Status != BattlePassStatusPaused {
		return nil, infraerrors.BadRequest("BATTLE_PASS_SEASON_LOCKED", "only paused seasons can be resumed")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to resume battle pass season")
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockBattlePassSeasonMutation(ctx, tx); err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to lock battle pass season")
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE battle_pass_seasons SET status='scheduled', updated_at=NOW() WHERE id=$1 AND status='paused'
	`, id)
	if err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to resume battle pass season")
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return nil, infraerrors.Conflict("BATTLE_PASS_SEASON_LOCKED", "season was modified while resuming")
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE battle_pass_pause_windows SET resumed_at=$2
		WHERE id = (
			SELECT id FROM battle_pass_pause_windows
			WHERE season_id=$1 AND resumed_at IS NULL
			ORDER BY paused_at DESC, id DESC
			LIMIT 1
		)
	`, id, now); err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to close pause window")
	}
	if err := tx.Commit(); err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to resume battle pass season")
	}
	return s.GetSeason(ctx, id, now)
}

func (s *BattlePassService) EndSeason(ctx context.Context, id int64, now time.Time) (*BattlePassSeasonDetail, error) {
	detail, err := s.GetSeason(ctx, id, now)
	if err != nil {
		return nil, err
	}
	if detail.Status != BattlePassStatusScheduled && detail.Status != BattlePassStatusPaused {
		return nil, infraerrors.BadRequest("BATTLE_PASS_SEASON_LOCKED", "only published seasons can be ended")
	}
	if _, err := s.db.ExecContext(ctx, `
		UPDATE battle_pass_seasons SET status='ended', activation_epoch=activation_epoch+1, updated_at=NOW()
		WHERE id=$1 AND status IN ('scheduled','paused')
	`, id); err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to end battle pass season")
	}
	return s.GetSeason(ctx, id, now)
}

func (s *BattlePassService) requireUserAccess(ctx context.Context, now time.Time) error {
	_ = now
	if err := s.requireEnabled(ctx); err != nil {
		return err
	}
	return nil
}

// RequireUserAccess lets the HTTP layer fail closed with the same 404-style
// response before it inspects an authenticated subject. This avoids exposing a
// disabled module as a distinct unauthenticated endpoint.
func (s *BattlePassService) RequireUserAccess(ctx context.Context, now time.Time) error {
	return s.requireUserAccess(ctx, now)
}

func (s *BattlePassService) getSeason(ctx context.Context, id int64, now time.Time) (*BattlePassSeason, error) {
	if s == nil || s.db == nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "battle pass database is unavailable")
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, description, status, timezone, start_at, end_at, premium_price, max_level,
		       activation_epoch, published_at, enabled_at_snapshot
		FROM battle_pass_seasons WHERE id=$1
	`, id)
	item, err := scanBattlePassSeason(row)
	if err == sql.ErrNoRows {
		return nil, infraerrors.NotFound("BATTLE_PASS_SEASON_NOT_FOUND", "battle pass season not found")
	}
	if err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to load battle pass season")
	}
	enabled, _ := s.IsEnabled(ctx)
	item.RuntimeStatus = runtimeSeasonStatus(item, now)
	item.UserSideEnabled = enabled
	return &item, nil
}

func (s *BattlePassService) loadSeasonChildren(ctx context.Context, detail *BattlePassSeasonDetail) error {
	if detail.Status != BattlePassStatusDraft {
		snapshot, err := loadBattlePassConfigSnapshot(ctx, s.db, detail.ID)
		if err != nil {
			return err
		}
		detail.Levels = snapshot.Levels
		detail.Tasks = snapshot.Tasks
		detail.Rewards = snapshot.Rewards
		return nil
	}
	levelRows, err := s.db.QueryContext(ctx, `SELECT level, required_exp FROM battle_pass_levels WHERE season_id=$1 ORDER BY level`, detail.ID)
	if err != nil {
		return infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to load battle pass levels")
	}
	defer levelRows.Close()
	for levelRows.Next() {
		var item BattlePassLevelInput
		if err := levelRows.Scan(&item.Level, &item.RequiredExp); err != nil {
			return err
		}
		detail.Levels = append(detail.Levels, item)
	}
	if err := levelRows.Err(); err != nil {
		return infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to read battle pass levels")
	}
	taskRows, err := s.db.QueryContext(ctx, `
		SELECT id, name, description, task_type, period_type, target_value, exp_reward, filter_scope, filter_values, display_order, enabled
		FROM battle_pass_tasks WHERE season_id=$1 ORDER BY display_order, id
	`, detail.ID)
	if err != nil {
		return infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to load battle pass tasks")
	}
	defer taskRows.Close()
	for taskRows.Next() {
		var item BattlePassTaskInput
		var raw []byte
		if err := taskRows.Scan(&item.ID, &item.Name, &item.Description, &item.TaskType, &item.PeriodType, &item.TargetValue, &item.ExpReward, &item.FilterScope, &raw, &item.DisplayOrder, &item.Enabled); err != nil {
			return err
		}
		_ = json.Unmarshal(raw, &item.FilterValues)
		detail.Tasks = append(detail.Tasks, item)
	}
	if err := taskRows.Err(); err != nil {
		return infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to read battle pass tasks")
	}
	rewardRows, err := s.db.QueryContext(ctx, `SELECT id, level, track, reward_type, payload FROM battle_pass_rewards WHERE season_id=$1 ORDER BY level, track, id`, detail.ID)
	if err != nil {
		return infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to load battle pass rewards")
	}
	defer rewardRows.Close()
	for rewardRows.Next() {
		var item BattlePassRewardInput
		var raw []byte
		if err := rewardRows.Scan(&item.ID, &item.Level, &item.Track, &item.RewardType, &raw); err != nil {
			return err
		}
		_ = json.Unmarshal(raw, &item.Payload)
		if item.Payload == nil {
			item.Payload = map[string]any{}
		}
		detail.Rewards = append(detail.Rewards, item)
	}
	if err := rewardRows.Err(); err != nil {
		return infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to read battle pass rewards")
	}
	return nil
}

func (s *BattlePassService) hasOverlappingPublishedSeason(ctx context.Context, id int64, start, end time.Time) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM battle_pass_seasons
		WHERE id <> $1 AND status IN ('scheduled','paused') AND start_at < $3 AND end_at > $2
	`, id, start, end).Scan(&count)
	if err != nil {
		return false, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to check season overlap")
	}
	return count > 0, nil
}

func replaceSeasonChildren(ctx context.Context, tx *sql.Tx, seasonID int64, draft BattlePassSeasonDraft) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM battle_pass_rewards WHERE season_id=$1`, seasonID); err != nil {
		return infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to replace rewards")
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM battle_pass_tasks WHERE season_id=$1`, seasonID); err != nil {
		return infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to replace tasks")
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM battle_pass_levels WHERE season_id=$1`, seasonID); err != nil {
		return infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to replace levels")
	}
	for _, level := range draft.Levels {
		if _, err := tx.ExecContext(ctx, `INSERT INTO battle_pass_levels (season_id, level, required_exp) VALUES ($1,$2,$3)`, seasonID, level.Level, level.RequiredExp); err != nil {
			return infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to save levels")
		}
	}
	for _, task := range draft.Tasks {
		raw, _ := json.Marshal(task.FilterValues)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO battle_pass_tasks (season_id, name, description, task_type, period_type, target_value, exp_reward, filter_scope, filter_values, display_order, enabled)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		`, seasonID, strings.TrimSpace(task.Name), strings.TrimSpace(task.Description), task.TaskType, task.PeriodType, task.TargetValue, task.ExpReward, emptyDefault(task.FilterScope, "all"), raw, task.DisplayOrder, task.Enabled); err != nil {
			return infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to save tasks")
		}
	}
	for _, reward := range draft.Rewards {
		raw, _ := json.Marshal(reward.Payload)
		if len(raw) == 0 {
			raw = []byte("{}")
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO battle_pass_rewards (season_id, level, track, reward_type, payload)
			VALUES ($1,$2,$3,$4,$5)
		`, seasonID, reward.Level, reward.Track, reward.RewardType, raw); err != nil {
			return infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to save rewards")
		}
	}
	return nil
}

func validateSeasonForPublish(detail BattlePassSeasonDetail, now time.Time) BattlePassValidationResult {
	result := BattlePassValidationResult{OK: true}
	addErr := func(code, message string) {
		result.OK = false
		result.Errors = append(result.Errors, BattlePassValidationIssue{Level: "error", Code: code, Message: message})
	}
	if err := validateDraftSeason(BattlePassSeasonInput{
		Name: detail.Name, Description: detail.Description, Timezone: detail.Timezone,
		StartAt: detail.StartAt, EndAt: detail.EndAt, PremiumPrice: detail.PremiumPrice, MaxLevel: detail.MaxLevel,
	}, now); err != nil {
		addErr("SEASON", err.Error())
	}
	if detail.PremiumPrice <= 0 {
		addErr("PRICE", "premium_price must be greater than 0")
	}
	if len(detail.Levels) == 0 {
		addErr("LEVELS", "at least one level is required")
	}
	if len(detail.Levels) > 100 {
		addErr("LEVELS", "at most 100 levels are supported")
	}
	sort.Slice(detail.Levels, func(i, j int) bool { return detail.Levels[i].Level < detail.Levels[j].Level })
	for i, level := range detail.Levels {
		if level.Level != i+1 {
			addErr("LEVELS", "levels must be consecutive starting at 1")
			break
		}
		if i > 0 && level.RequiredExp <= detail.Levels[i-1].RequiredExp {
			addErr("LEVELS", "required_exp must strictly increase")
			break
		}
		if level.RequiredExp < 0 || level.RequiredExp > 100_000_000 {
			addErr("LEVELS", "required_exp is outside the supported range")
			break
		}
	}
	if len(detail.Levels) > 0 && detail.MaxLevel != detail.Levels[len(detail.Levels)-1].Level {
		addErr("LEVELS", "max_level must match the last configured level")
	}
	freeRewards := 0
	premiumRewards := 0
	rewardSlots := make(map[string]struct{}, len(detail.Rewards))
	if len(detail.Rewards) > battlePassMaxRewardsPerSeason {
		addErr("REWARD", "too many rewards")
	}
	for _, reward := range detail.Rewards {
		if _, ok := battlePassAllowedRewardTypes[reward.RewardType]; !ok {
			addErr("REWARD", fmt.Sprintf("unsupported reward type %s", reward.RewardType))
		}
		if reward.Track != "free" && reward.Track != "premium" {
			addErr("REWARD", "reward track must be free or premium")
		}
		if reward.Track == "free" {
			freeRewards++
		}
		if reward.Track == "premium" {
			premiumRewards++
		}
		slot := fmt.Sprintf("%d:%s", reward.Level, reward.Track)
		if _, exists := rewardSlots[slot]; exists {
			addErr("REWARD", "only one reward is allowed for each level and track")
		} else {
			rewardSlots[slot] = struct{}{}
		}
		if reward.Level < 1 || reward.Level > detail.MaxLevel {
			addErr("REWARD", "reward level must refer to a configured level")
		}
		if err := validateBattlePassRewardPayload(reward); err != nil {
			addErr("REWARD", err.Error())
		}
	}
	if freeRewards == 0 {
		addErr("REWARD", "at least one free reward is required")
	}
	if premiumRewards == 0 {
		addErr("REWARD", "at least one premium reward is required")
	}
	if len(detail.Tasks) == 0 {
		addErr("TASK", "at least one task is required")
	}
	if len(detail.Tasks) > battlePassMaxTasksPerSeason {
		addErr("TASK", "too many tasks")
	}
	for _, task := range detail.Tasks {
		if _, ok := battlePassAllowedTaskTypes[task.TaskType]; !ok {
			addErr("TASK", fmt.Sprintf("unsupported task type %s", task.TaskType))
		}
		if task.PeriodType != "daily" && task.PeriodType != "season" {
			addErr("TASK", "period_type must be daily or season")
		}
		if task.TaskType == "recharge_count" || task.TaskType == "recharge_amount" || task.TaskType == "valid_invite_count" || task.TaskType == "invitee_recharge_count" {
			if task.PeriodType != "season" {
				addErr("TASK_PERIOD", fmt.Sprintf("%s tasks must use season period", task.TaskType))
			}
			if emptyDefault(task.FilterScope, "all") != "all" || len(task.FilterValues) != 0 {
				addErr("TASK_FILTER", fmt.Sprintf("%s tasks cannot filter by model", task.TaskType))
			}
		}
		if (task.TaskType == "active_days" || task.TaskType == "distinct_model_families") && task.PeriodType != "season" {
			addErr("TASK_PERIOD", fmt.Sprintf("%s tasks must use season period", task.TaskType))
		}
		if task.TargetValue <= 0 || task.TargetValue > battlePassMaxTaskTarget || math.IsNaN(task.TargetValue) || math.IsInf(task.TargetValue, 0) || task.ExpReward <= 0 || task.ExpReward > battlePassMaxTaskExp || strings.TrimSpace(task.Name) == "" {
			addErr("TASK", "task name, target_value and exp_reward are required")
		}
		if task.FilterScope != "" && task.FilterScope != "all" && task.FilterScope != "model_family" && task.FilterScope != "exact_model" {
			addErr("TASK", "filter_scope must be all, model_family, or exact_model")
		}
		if err := validateBattlePassTaskFilter(task); err != nil {
			addErr("TASK", err.Error())
		}
	}
	return result
}

func validateBattlePassTaskFilter(task BattlePassTaskInput) error {
	scope := emptyDefault(task.FilterScope, "all")
	if scope == "all" {
		if len(task.FilterValues) != 0 {
			return fmt.Errorf("all task filters must not include filter_values")
		}
		return nil
	}
	if len(task.FilterValues) == 0 || len(task.FilterValues) > 30 {
		return fmt.Errorf("model task filters require between 1 and 30 values")
	}
	seen := make(map[string]struct{}, len(task.FilterValues))
	for _, value := range task.FilterValues {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > 100 {
			return fmt.Errorf("model task filter value is invalid")
		}
		if _, ok := seen[strings.ToLower(value)]; ok {
			return fmt.Errorf("model task filter values must be unique")
		}
		seen[strings.ToLower(value)] = struct{}{}
	}
	return nil
}

func validateBattlePassRewardPayload(reward BattlePassRewardInput) error {
	payload := reward.Payload
	if payload == nil {
		return fmt.Errorf("%s reward payload is required", reward.RewardType)
	}
	number := func(key string) (float64, bool) {
		value, ok := payload[key]
		if !ok {
			return 0, false
		}
		n, ok := value.(float64)
		return n, ok
	}
	switch reward.RewardType {
	case "balance":
		amount, ok := number("amount")
		if !ok || math.IsNaN(amount) || math.IsInf(amount, 0) || amount <= 0 || amount > battlePassMaxBalanceReward || math.Abs(amount*1e8-math.Round(amount*1e8)) > 1e-6 {
			return fmt.Errorf("balance reward amount must be a positive decimal with at most 8 places")
		}
	case "concurrency":
		amount, ok := number("amount")
		if !ok || amount < 1 || amount > battlePassMaxConcurrencyAward || math.Trunc(amount) != amount {
			return fmt.Errorf("concurrency reward amount must be an integer between 1 and %d", battlePassMaxConcurrencyAward)
		}
	case "subscription_days":
		groupID, groupOK := number("group_id")
		days, daysOK := number("days")
		if !groupOK || groupID < 1 || math.Trunc(groupID) != groupID || !daysOK || days < 1 || days > MaxValidityDays || math.Trunc(days) != days {
			return fmt.Errorf("subscription reward requires integer group_id and days")
		}
	case "badge", "title":
		code, codeOK := payload["code"].(string)
		name, nameOK := payload["name"].(string)
		if !codeOK || !nameOK || !battlePassSafeCosmeticToken(code, 64) || strings.TrimSpace(name) == "" || len(strings.TrimSpace(name)) > 120 {
			return fmt.Errorf("%s reward requires a safe code and a display name", reward.RewardType)
		}
	default:
		return fmt.Errorf("unsupported reward type %s", reward.RewardType)
	}
	return nil
}

func battlePassSafeCosmeticToken(value string, max int) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > max {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

func firstValidationMessage(result *BattlePassValidationResult) string {
	if result == nil || len(result.Errors) == 0 {
		return "season is invalid"
	}
	return result.Errors[0].Message
}

func emptyDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
