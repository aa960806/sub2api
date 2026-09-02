package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func expectBattlePassRuntimeTasks(mock sqlmock.Sqlmock, taskType string) {
	raw, err := json.Marshal(battlePassConfigSnapshot{Tasks: []BattlePassTaskInput{{ID: 7, Name: taskType, TaskType: taskType, PeriodType: "season", TargetValue: 100, ExpReward: 10, FilterScope: "all", FilterValues: []string{}, DisplayOrder: 1, Enabled: true}}})
	if err != nil {
		panic(err)
	}
	mock.ExpectQuery("SELECT config_snapshot FROM battle_pass_seasons").
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"config_snapshot"}).AddRow(raw))
}

func TestBattlePassScannerDisabledDoesNotReadOrWriteBusinessTables(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	svc := NewBattlePassService(db, &battlePassSettingRepoStub{values: map[string]string{
		SettingKeyBattlePassEnabled: "false",
	}})
	err = NewBattlePassScanner(svc).runOnce(context.Background(), time.Now())
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLoadBattlePassRuntimeTasksUsesPublishedSnapshot(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()
	mock.ExpectBegin()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	raw, err := json.Marshal(battlePassConfigSnapshot{Tasks: []BattlePassTaskInput{{
		ID: 17, Name: "requests", TaskType: "request_count", PeriodType: "daily",
		TargetValue: 2, ExpReward: 5, FilterScope: "all", Enabled: true,
	}}})
	require.NoError(t, err)
	mock.ExpectQuery("SELECT config_snapshot FROM battle_pass_seasons").
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"config_snapshot"}).AddRow(raw))
	mock.ExpectCommit()

	tasks, err := loadBattlePassRuntimeTasks(ctx, tx, 9)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.Equal(t, int64(17), tasks[0].ID)
	require.Equal(t, "request_count", tasks[0].TaskType)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLoadBattlePassRuntimeTasksRejectsSnapshotWithoutStableID(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()
	mock.ExpectBegin()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	raw, err := json.Marshal(battlePassConfigSnapshot{Tasks: []BattlePassTaskInput{{
		Name: "requests", TaskType: "request_count", PeriodType: "daily", TargetValue: 1,
		ExpReward: 5, FilterScope: "all", Enabled: true,
	}}})
	require.NoError(t, err)
	mock.ExpectQuery("SELECT config_snapshot FROM battle_pass_seasons").
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"config_snapshot"}).AddRow(raw))
	mock.ExpectRollback()

	_, err = loadBattlePassRuntimeTasks(ctx, tx, 9)
	require.Error(t, err)
	require.Contains(t, err.Error(), "BATTLE_PASS_CONFIG_INVALID")
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLoadBattlePassRewardSnapshotUsesPublishedPayload(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	raw, err := json.Marshal(battlePassConfigSnapshot{Rewards: []BattlePassRewardInput{{
		ID: 31, Level: 2, Track: "free", RewardType: "balance",
		Payload: map[string]any{"amount": 3.5},
	}}})
	require.NoError(t, err)
	mock.ExpectQuery("SELECT config_snapshot FROM battle_pass_seasons").
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"config_snapshot"}).AddRow(raw))

	reward, err := loadBattlePassRewardSnapshot(context.Background(), db, 9, 31)
	require.NoError(t, err)
	require.Equal(t, "balance", reward.RewardType)
	require.Equal(t, 3.5, reward.Payload["amount"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestClaimBattlePassRewardAllowsScheduledSeasonAfterEnd(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	mock.ExpectQuery(`(?s)WITH candidate.*s\.start_at <= \$4\) OR s\.status='ended'`).
		WithArgs(now.Add(-time.Minute), now.Add(-5*time.Minute), battlePassMaxAutomaticRewardAttempts, now).
		WillReturnRows(sqlmock.NewRows([]string{"id", "season_id", "user_id", "reward_id"}).AddRow(1, 2, 3, 4))
	mock.ExpectCommit()

	grant, err := claimBattlePassRewardGrant(ctx, tx, now)
	require.NoError(t, err)
	require.Equal(t, &battlePassGrant{ID: 1, SeasonID: 2, UserID: 3, RewardID: 4}, grant)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func expectBattlePassSourceContributionNoPrevious(mock sqlmock.Sqlmock, seasonID, taskID, userID int64, sourceType string, sourceID int64, value float64) {
	mock.ExpectQuery("SELECT contribution_value FROM battle_pass_source_contributions").
		WithArgs(seasonID, taskID, userID, sourceType, sourceID).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectExec("INSERT INTO battle_pass_source_contributions").
		WithArgs(seasonID, taskID, userID, sourceType, sourceID, value, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO battle_pass_user_progress").
		WithArgs(seasonID, userID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("INSERT INTO battle_pass_task_progress").
		WithArgs(seasonID, taskID, userID, value, 100.0).
		WillReturnRows(sqlmock.NewRows([]string{"completed"}).AddRow(false))
}

func TestBattlePassPaymentScannerPersistsNetContributionAndCursor(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	statisticsStart := now.Add(-72 * time.Hour)
	season := BattlePassSeason{ID: 3, EndAt: now.Add(24 * time.Hour), ActivationEpoch: 2}

	mock.ExpectBegin()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	expectBattlePassRuntimeTasks(mock, "recharge_amount")
	mock.ExpectQuery("SELECT COALESCE\\(statistics_start_at, start_at\\) FROM battle_pass_seasons").
		WithArgs(season.ID).WillReturnRows(sqlmock.NewRows([]string{"statistics_start"}).AddRow(statisticsStart))
	mock.ExpectExec("INSERT INTO battle_pass_source_cursors").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT last_id, last_updated_at, activation_epoch FROM battle_pass_source_cursors").
		WithArgs(season.ID).WillReturnRows(sqlmock.NewRows([]string{"last_id", "last_updated_at", "activation_epoch"}).AddRow(0, nil, season.ActivationEpoch))

	updatedAt := now.Add(-25 * time.Hour)
	completedAt := now.Add(-26 * time.Hour)
	mock.ExpectQuery("SELECT po.id, po.user_id, po.order_type, po.status").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "order_type", "status", "pay_amount", "refund_amount", "updated_at", "completed_at", "eligible",
		}).AddRow(44, 99, "balance", "PARTIALLY_REFUNDED", 50.0, 5.0, updatedAt, completedAt, true))
	expectBattlePassSourceContributionNoPrevious(mock, season.ID, 7, 99, "payment_order", 44, 45.0)
	mock.ExpectExec("UPDATE battle_pass_source_cursors").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	processed, err := (&BattlePassService{db: db}).scanPaymentOrdersTx(ctx, tx, season, now)
	require.NoError(t, err)
	require.Equal(t, 1, processed)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBattlePassAffiliateScannerPersistsEligibleInviteContribution(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	statisticsStart := now.Add(-72 * time.Hour)
	season := BattlePassSeason{ID: 4, EndAt: now.Add(24 * time.Hour), ActivationEpoch: 1}

	mock.ExpectBegin()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	expectBattlePassRuntimeTasks(mock, "valid_invite_count")
	mock.ExpectQuery("SELECT value FROM settings WHERE key=\\$1").
		WithArgs(SettingKeyAffiliateEnabled).WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("true"))
	mock.ExpectQuery("SELECT COALESCE\\(statistics_start_at, start_at\\) FROM battle_pass_seasons").
		WithArgs(season.ID).WillReturnRows(sqlmock.NewRows([]string{"statistics_start"}).AddRow(statisticsStart))

	createdAt := now.Add(-48 * time.Hour)
	mock.ExpectQuery("SELECT ua.user_id, ua.inviter_id, ua.created_at").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "inviter_id", "created_at", "eligible", "has_valid_recharge"}).
			AddRow(101, 202, createdAt, true, true))
	expectBattlePassSourceContributionNoPrevious(mock, season.ID, 7, 202, "affiliate", 101, 1.0)
	mock.ExpectCommit()

	processed, err := (&BattlePassService{db: db}).scanAffiliateRelationsTx(ctx, tx, season, now)
	require.NoError(t, err)
	require.Equal(t, 1, processed)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBattlePassActiveDaySourceIDUsesLocalCalendarDay(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)

	// These two UTC timestamps are the same local calendar day in Shanghai;
	// the following timestamp is the next local day.
	sameDayA := time.Date(2026, 8, 31, 23, 30, 0, 0, time.UTC)
	sameDayB := time.Date(2026, 9, 1, 1, 0, 0, 0, time.UTC)
	nextDay := time.Date(2026, 9, 1, 16, 0, 0, 0, time.UTC)

	require.Equal(t,
		battlePassActiveDaySourceID(42, sameDayA, location),
		battlePassActiveDaySourceID(42, sameDayB, location),
	)
	require.NotEqual(t,
		battlePassActiveDaySourceID(42, sameDayB, location),
		battlePassActiveDaySourceID(42, nextDay, location),
	)
}

func TestApplyBattlePassCosmeticBlocksIneligibleUser(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()
	mock.ExpectBegin()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	mock.ExpectQuery("SELECT id FROM users").
		WithArgs(int64(42)).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	status, metadata, changed, err := applyBattlePassRewardTx(ctx, tx, battlePassGrant{
		SeasonID:   1,
		UserID:     42,
		RewardID:   7,
		RewardType: "badge",
		Payload: map[string]any{
			"code": "badge-1",
			"name": "Badge",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "blocked_config", status)
	require.Equal(t, map[string]any{"reason": "user is no longer eligible"}, metadata)
	require.False(t, changed)
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}
