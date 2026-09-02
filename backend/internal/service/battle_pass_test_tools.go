package service

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const battlePassTestToolsEnv = "BATTLE_PASS_TEST_TOOLS_ENABLED"

type BattlePassTestUser struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
}

type BattlePassTestState struct {
	Season   *BattlePassSeason       `json:"season"`
	User     BattlePassTestUser      `json:"user"`
	Progress *BattlePassUserProgress `json:"progress,omitempty"`
	Tasks    []BattlePassTaskState   `json:"tasks"`
	Rewards  []BattlePassRewardState `json:"rewards"`
}

type BattlePassTestRequest struct {
	SeasonID int64 `json:"season_id"`
	UserID   int64 `json:"user_id"`
	TaskID   int64 `json:"task_id,omitempty"`
}

type BattlePassTestCompleteResult struct {
	CompletedCount int                 `json:"completed_count"`
	State          BattlePassTestState `json:"state"`
}

func BattlePassTestToolsEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(battlePassTestToolsEnv))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func requireBattlePassTestTools() error {
	if !BattlePassTestToolsEnabled() {
		return infraerrors.NotFound("BATTLE_PASS_TEST_TOOLS_DISABLED", "battle pass test tools are not available")
	}
	return nil
}

func (s *BattlePassService) GetTestState(ctx context.Context, seasonID, userID int64, now time.Time) (*BattlePassTestState, error) {
	if err := requireBattlePassTestTools(); err != nil {
		return nil, err
	}
	if seasonID <= 0 || userID <= 0 {
		return nil, infraerrors.BadRequest("BATTLE_PASS_TEST_INPUT_INVALID", "season_id and user_id are required")
	}
	season, err := s.getSeason(ctx, seasonID, now)
	if err != nil {
		return nil, err
	}
	if season.Status == BattlePassStatusDraft || season.Status == BattlePassStatusArchived {
		return nil, infraerrors.Conflict("BATTLE_PASS_TEST_SEASON_UNAVAILABLE", "select a published battle pass season")
	}
	if err := s.requireEligibleUser(ctx, userID); err != nil {
		return nil, err
	}
	state := &BattlePassTestState{Season: season, User: BattlePassTestUser{ID: userID}}
	if err := s.db.QueryRowContext(ctx, `SELECT email FROM users WHERE id=$1`, userID).Scan(&state.User.Email); err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to load battle pass test user")
	}
	state.Progress, err = s.loadUserProgress(ctx, seasonID, userID)
	if err != nil {
		return nil, err
	}
	state.Tasks, err = s.getTasksForSeasonUser(ctx, *season, userID, now)
	if err != nil {
		return nil, err
	}
	state.Rewards, err = s.listRewardsForSeasonUser(ctx, seasonID, userID)
	if err != nil {
		return nil, err
	}
	return state, nil
}

func (s *BattlePassService) ActivateSeasonForTest(ctx context.Context, seasonID int64, now time.Time) (*BattlePassSeason, error) {
	if err := requireBattlePassTestTools(); err != nil {
		return nil, err
	}
	if err := s.requireEnabled(ctx); err != nil {
		return nil, err
	}
	if seasonID <= 0 {
		return nil, infraerrors.BadRequest("BATTLE_PASS_TEST_INPUT_INVALID", "season_id is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to start battle pass test activation")
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(254, 1)`); err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to lock battle pass test activation")
	}
	var status string
	var startAt, endAt time.Time
	if err := tx.QueryRowContext(ctx, `SELECT status, start_at, end_at FROM battle_pass_seasons WHERE id=$1 FOR UPDATE`, seasonID).Scan(&status, &startAt, &endAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, infraerrors.NotFound("BATTLE_PASS_SEASON_NOT_FOUND", "battle pass season not found")
		}
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to load battle pass test season")
	}
	if status != BattlePassStatusScheduled || !endAt.After(now) {
		return nil, infraerrors.Conflict("BATTLE_PASS_TEST_SEASON_UNAVAILABLE", "only a published, non-ended season can be activated")
	}
	if startAt.After(now) {
		var activeCount int
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM battle_pass_seasons
			WHERE id<>$1 AND status IN ('scheduled','paused') AND start_at <= $2 AND end_at > $2
		`, seasonID, now).Scan(&activeCount); err != nil {
			return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to check active battle pass seasons")
		}
		if activeCount > 0 {
			return nil, infraerrors.Conflict("BATTLE_PASS_TEST_SEASON_OVERLAP", "another battle pass season is already active")
		}
		boundary := now.Add(-time.Second)
		if _, err := tx.ExecContext(ctx, `
			UPDATE battle_pass_seasons
			SET start_at=$2, statistics_start_at=$2, enabled_at_snapshot=COALESCE(enabled_at_snapshot,$2),
			    activation_epoch=activation_epoch+1, updated_at=NOW()
			WHERE id=$1
		`, seasonID, boundary); err != nil {
			return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to activate battle pass test season")
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to commit battle pass test activation")
	}
	return s.getSeason(ctx, seasonID, now)
}

func (s *BattlePassService) CompleteTasksForTest(ctx context.Context, req BattlePassTestRequest, now time.Time) (*BattlePassTestCompleteResult, error) {
	if err := requireBattlePassTestTools(); err != nil {
		return nil, err
	}
	if err := s.requireEnabled(ctx); err != nil {
		return nil, err
	}
	if req.SeasonID <= 0 || req.UserID <= 0 {
		return nil, infraerrors.BadRequest("BATTLE_PASS_TEST_INPUT_INVALID", "season_id and user_id are required")
	}
	if err := s.requireEligibleUser(ctx, req.UserID); err != nil {
		return nil, err
	}
	season, err := s.getSeason(ctx, req.SeasonID, now)
	if err != nil {
		return nil, err
	}
	if runtimeSeasonStatus(*season, now) != "active" {
		return nil, infraerrors.Conflict("BATTLE_PASS_TEST_SEASON_NOT_ACTIVE", "activate the battle pass season before completing test tasks")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to start battle pass test task completion")
	}
	defer func() { _ = tx.Rollback() }()
	tasks, err := loadBattlePassRuntimeTasks(ctx, tx, season.ID)
	if err != nil {
		return nil, err
	}
	selected := make([]battlePassRuntimeTask, 0, len(tasks))
	for _, task := range tasks {
		if req.TaskID == 0 || task.ID == req.TaskID {
			selected = append(selected, task)
		}
	}
	if len(selected) == 0 {
		return nil, infraerrors.NotFound("BATTLE_PASS_TEST_TASK_NOT_FOUND", "battle pass test task not found")
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO battle_pass_user_progress (season_id,user_id)
		SELECT $1,id FROM users WHERE id=$2 AND status='active' AND deleted_at IS NULL
		ON CONFLICT (season_id,user_id) DO NOTHING
	`, season.ID, req.UserID); err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to initialize battle pass test progress")
	}
	location, err := time.LoadLocation(season.Timezone)
	if err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_CONFIG_INVALID", "battle pass timezone is invalid")
	}
	completedCount := 0
	for _, task := range selected {
		periodKey := "season"
		if task.PeriodType == "daily" {
			periodKey = now.In(location).Format("2006-01-02")
		}
		alreadyCompleted := false
		err := tx.QueryRowContext(ctx, `
			SELECT completed FROM battle_pass_task_progress
			WHERE season_id=$1 AND task_id=$2 AND user_id=$3 AND period_key=$4
		`, season.ID, task.ID, req.UserID, periodKey).Scan(&alreadyCompleted)
		if err != nil && err != sql.ErrNoRows {
			return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to load battle pass test task progress")
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO battle_pass_task_progress (season_id,task_id,user_id,period_key,current_value,completed,completed_at)
			VALUES ($1,$2,$3,$4,$5,TRUE,NOW())
			ON CONFLICT (season_id,task_id,user_id,period_key) DO UPDATE
			SET current_value=GREATEST(battle_pass_task_progress.current_value,EXCLUDED.current_value),
			    completed=TRUE, completed_at=COALESCE(battle_pass_task_progress.completed_at,NOW()), updated_at=NOW()
		`, season.ID, task.ID, req.UserID, periodKey, task.TargetValue); err != nil {
			return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to complete battle pass test task")
		}
		if err := s.awardTaskExperienceTx(ctx, tx, season.ID, req.UserID, task.ID, periodKey, task.ExpReward); err != nil {
			return nil, err
		}
		if !alreadyCompleted {
			completedCount++
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to commit battle pass test task completion")
	}
	state, err := s.GetTestState(ctx, season.ID, req.UserID, now)
	if err != nil {
		return nil, err
	}
	return &BattlePassTestCompleteResult{CompletedCount: completedCount, State: *state}, nil
}
