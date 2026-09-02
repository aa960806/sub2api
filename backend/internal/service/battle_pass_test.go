package service

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

type battlePassSettingRepoStub struct {
	values map[string]string
	err    error
}

func (s *battlePassSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	if s != nil && s.err != nil {
		return "", s.err
	}
	if s == nil || s.values == nil {
		return "", nil
	}
	return s.values[key], nil
}

func (s *battlePassSettingRepoStub) Set(_ context.Context, key, value string) error {
	if s.values == nil {
		s.values = map[string]string{}
	}
	s.values[key] = value
	return nil
}

func TestBattlePassIsEnabledFailClosed(t *testing.T) {
	svc := NewBattlePassService(nil, &battlePassSettingRepoStub{values: map[string]string{}})
	enabled, err := svc.IsEnabled(context.Background())
	require.NoError(t, err)
	require.False(t, enabled)

	for _, invalid := range []string{"TRUE", "1", " true ", "yes"} {
		svc = NewBattlePassService(nil, &battlePassSettingRepoStub{values: map[string]string{SettingKeyBattlePassEnabled: invalid}})
		enabled, err = svc.IsEnabled(context.Background())
		require.NoError(t, err)
		require.False(t, enabled, "unexpected enablement for %q", invalid)
	}

	svc = NewBattlePassService(nil, &battlePassSettingRepoStub{values: map[string]string{SettingKeyBattlePassEnabled: "true"}})
	enabled, err = svc.IsEnabled(context.Background())
	require.NoError(t, err)
	require.True(t, enabled)

	svc = NewBattlePassService(nil, &battlePassSettingRepoStub{err: sql.ErrConnDone})
	enabled, err = svc.IsEnabled(context.Background())
	require.NoError(t, err)
	require.False(t, enabled)
}

func TestBattlePassSetEnabledCapturesBoundaryBeforePublishingSwitch(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := &battlePassSettingRepoStub{values: map[string]string{SettingKeyBattlePassEnabled: "false"}}
	mock.ExpectExec("UPDATE battle_pass_seasons").WillReturnResult(sqlmock.NewResult(0, 1))

	svc := NewBattlePassService(db, repo)
	var notifications int
	svc.SetSettingsUpdatedNotifier(func() { notifications++ })
	settings, err := svc.SetEnabled(context.Background(), true)
	require.NoError(t, err)
	require.True(t, settings.Enabled)
	require.Equal(t, "true", repo.values[SettingKeyBattlePassEnabled])
	require.Equal(t, 1, notifications)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBattlePassSetEnabledSnapshotFailureLeavesSwitchClosed(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := &battlePassSettingRepoStub{values: map[string]string{SettingKeyBattlePassEnabled: "false"}}
	mock.ExpectExec("UPDATE battle_pass_seasons").WillReturnError(sql.ErrConnDone)

	svc := NewBattlePassService(db, repo)
	var notifications int
	svc.SetSettingsUpdatedNotifier(func() { notifications++ })
	_, err = svc.SetEnabled(context.Background(), true)
	require.Error(t, err)
	require.Contains(t, err.Error(), "BATTLE_PASS_SAVE_SWITCH_FAILED")
	require.Equal(t, "false", repo.values[SettingKeyBattlePassEnabled])
	require.Zero(t, notifications)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBattlePassSetEnabledWithoutDatabaseLeavesSwitchClosed(t *testing.T) {
	repo := &battlePassSettingRepoStub{values: map[string]string{SettingKeyBattlePassEnabled: "false"}}
	svc := NewBattlePassService(nil, repo)

	_, err := svc.SetEnabled(context.Background(), true)
	require.Error(t, err)
	require.Contains(t, err.Error(), "BATTLE_PASS_SAVE_SWITCH_FAILED")
	require.Equal(t, "false", repo.values[SettingKeyBattlePassEnabled])
}

func TestBattlePassSetDisabledEpochFailureKeepsSwitchClosed(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := &battlePassSettingRepoStub{values: map[string]string{SettingKeyBattlePassEnabled: "true"}}
	mock.ExpectExec("UPDATE battle_pass_seasons").WillReturnError(sql.ErrConnDone)

	svc := NewBattlePassService(db, repo)
	var notifications int
	svc.SetSettingsUpdatedNotifier(func() { notifications++ })
	_, err = svc.SetEnabled(context.Background(), false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "BATTLE_PASS_SAVE_SWITCH_FAILED")
	require.Equal(t, "false", repo.values[SettingKeyBattlePassEnabled])
	require.Equal(t, 1, notifications)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBattlePassTestToolsFailClosedAndSurfaceExplicitEnablement(t *testing.T) {
	t.Setenv(battlePassTestToolsEnv, "")
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	svc := NewBattlePassService(db, &battlePassSettingRepoStub{values: map[string]string{SettingKeyBattlePassEnabled: "true"}})
	_, err = svc.GetTestState(context.Background(), 1, 1, time.Now())
	require.Error(t, err)
	require.Contains(t, err.Error(), "BATTLE_PASS_TEST_TOOLS_DISABLED")
	require.NoError(t, mock.ExpectationsWereMet())

	t.Setenv(battlePassTestToolsEnv, "true")
	settings, err := svc.GetSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.Enabled)
	require.True(t, settings.TestToolsEnabled)
}

func TestBattlePassGetCurrentDisabled(t *testing.T) {
	svc := NewBattlePassService(nil, &battlePassSettingRepoStub{values: map[string]string{SettingKeyBattlePassEnabled: "false"}})
	_, err := svc.GetCurrent(context.Background(), time.Now())
	require.Error(t, err)
	require.Contains(t, err.Error(), "BATTLE_PASS_DISABLED")
}

func TestBattlePassScanUsageOnceDisabledDoesNotQuery(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	svc := NewBattlePassService(db, &battlePassSettingRepoStub{values: map[string]string{SettingKeyBattlePassEnabled: "false"}})
	n, err := svc.ScanUsageOnce(context.Background(), time.Now())
	require.NoError(t, err)
	require.Equal(t, 0, n)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPauseBattlePassSeasonRejectsLostConcurrentUpdate(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()
	mock.ExpectBegin()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	mock.ExpectExec("UPDATE battle_pass_seasons").
		WithArgs(int64(9)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	err = pauseBattlePassSeasonTx(ctx, tx, 9, 7, "", time.Now())
	require.Error(t, err)
	require.Contains(t, err.Error(), "BATTLE_PASS_SEASON_LOCKED")
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBattlePassUserAccessUsesIndependentGate(t *testing.T) {
	for _, tc := range []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{name: "closed", raw: "false", wantErr: true},
		{name: "missing", raw: "", wantErr: true},
		{name: "enabled without activity card", raw: "true", wantErr: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			svc := NewBattlePassService(db, &battlePassSettingRepoStub{values: map[string]string{
				SettingKeyBattlePassEnabled: tc.raw,
			}})
			err = svc.requireUserAccess(context.Background(), time.Now())
			if tc.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), "BATTLE_PASS_DISABLED")
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestBattlePassEligibilityAllowsAnyActiveAccountRole(t *testing.T) {
	for _, role := range []string{"user", "admin"} {
		t.Run(role, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()

			// The query deliberately does not filter by role: administrators can
			// participate in a season, while inactive/deleted accounts remain barred.
			mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM users WHERE id=\$1 AND status='active' AND deleted_at IS NULL\)`).
				WithArgs(int64(42)).
				WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

			svc := NewBattlePassService(db, nil)
			require.NoError(t, svc.requireEligibleUser(context.Background(), 42))
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestBattlePassEligibilityStillRejectsInactiveOrDeletedAccount(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM users WHERE id=\$1 AND status='active' AND deleted_at IS NULL\)`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	svc := NewBattlePassService(db, nil)
	err = svc.requireEligibleUser(context.Background(), 42)
	require.Error(t, err)
	require.Contains(t, err.Error(), "BATTLE_PASS_INELIGIBLE")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBattlePassEnsureProgressAllowsActiveAdminAccount(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT value FROM settings WHERE key=\$1 FOR UPDATE`).
		WithArgs(SettingKeyBattlePassEnabled).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("true"))
	mock.ExpectExec(`INSERT INTO battle_pass_user_progress \(season_id, user_id\)\s+SELECT \$1, id FROM users WHERE id=\$2 AND status='active' AND deleted_at IS NULL`).
		WithArgs(int64(7), int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT exp, level, premium_unlocked, updated_at\s+FROM battle_pass_user_progress WHERE season_id=\$1 AND user_id=\$2`).
		WithArgs(int64(7), int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"exp", "level", "premium_unlocked", "updated_at"}).AddRow(int64(10), 1, false, now))
	mock.ExpectQuery(`SELECT config_snapshot FROM battle_pass_seasons WHERE id=\$1`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"config_snapshot"}).AddRow([]byte(`{"levels":[{"level":1,"required_exp":0},{"level":2,"required_exp":100}]}`)))
	mock.ExpectCommit()

	svc := NewBattlePassService(db, nil)
	progress, err := svc.ensureUserProgress(context.Background(), 7, 42)
	require.NoError(t, err)
	require.Equal(t, int64(10), progress.Exp)
	require.Equal(t, int64(100), *progress.NextLevelExp)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBattlePassEnsureProgressRechecksDisabledSwitchBeforeWrite(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT value FROM settings WHERE key=\$1 FOR UPDATE`).
		WithArgs(SettingKeyBattlePassEnabled).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("false"))
	mock.ExpectRollback()

	svc := NewBattlePassService(db, &battlePassSettingRepoStub{values: map[string]string{SettingKeyBattlePassEnabled: "true"}})
	progress, err := svc.ensureUserProgress(context.Background(), 7, 42)
	require.Nil(t, progress)
	require.Error(t, err)
	require.Contains(t, err.Error(), "BATTLE_PASS_DISABLED")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestValidateSeasonForPublishRequiresTracks(t *testing.T) {
	start := time.Now().Add(time.Hour)
	detail := BattlePassSeasonDetail{
		BattlePassSeason: BattlePassSeason{
			Name: "s1", Timezone: "Asia/Shanghai", StartAt: start, EndAt: start.Add(24 * time.Hour),
			PremiumPrice: 9.9, MaxLevel: 1,
		},
		Levels: []BattlePassLevelInput{{Level: 1, RequiredExp: 0}},
		Tasks: []BattlePassTaskInput{{
			Name: "daily requests", TaskType: "request_count", PeriodType: "daily",
			TargetValue: 3, ExpReward: 10, Enabled: true,
		}},
	}
	result := validateSeasonForPublish(detail, time.Now())
	require.False(t, result.OK)
	detail.Rewards = []BattlePassRewardInput{
		{Level: 1, Track: "free", RewardType: "balance", Payload: map[string]any{"amount": 0.2}},
		{Level: 1, Track: "premium", RewardType: "balance", Payload: map[string]any{"amount": 1.0}},
	}
	result = validateSeasonForPublish(detail, time.Now())
	require.True(t, result.OK)
}

func TestValidateSeasonForPublishRejectsUnsafeRewardAndTaskFilters(t *testing.T) {
	start := time.Now().Add(time.Hour)
	detail := BattlePassSeasonDetail{
		BattlePassSeason: BattlePassSeason{Name: "s1", Timezone: "Asia/Shanghai", StartAt: start, EndAt: start.Add(24 * time.Hour), PremiumPrice: 9.9, MaxLevel: 1},
		Levels:           []BattlePassLevelInput{{Level: 1, RequiredExp: 0}},
		Tasks: []BattlePassTaskInput{{
			Name: "requests", TaskType: "request_count", PeriodType: "daily", TargetValue: 1, ExpReward: 1,
			FilterScope: "all", FilterValues: []string{"must-not-be-here"}, Enabled: true,
		}},
		Rewards: []BattlePassRewardInput{
			{Level: 1, Track: "free", RewardType: "balance", Payload: map[string]any{"amount": -1.0}},
			{Level: 1, Track: "premium", RewardType: "badge", Payload: map[string]any{"code": "../unsafe", "name": "Unsafe"}},
		},
	}
	result := validateSeasonForPublish(detail, time.Now())
	require.False(t, result.OK)
	require.NotEmpty(t, result.Errors)
}

func TestValidateSeasonForPublishRejectsDuplicateRewardSlot(t *testing.T) {
	start := time.Now().Add(time.Hour)
	detail := BattlePassSeasonDetail{
		BattlePassSeason: BattlePassSeason{Name: "s1", Timezone: "Asia/Shanghai", StartAt: start, EndAt: start.Add(24 * time.Hour), PremiumPrice: 9.9, MaxLevel: 1},
		Levels:           []BattlePassLevelInput{{Level: 1, RequiredExp: 0}},
		Tasks:            []BattlePassTaskInput{{Name: "requests", TaskType: "request_count", PeriodType: "daily", TargetValue: 1, ExpReward: 1, FilterScope: "all", Enabled: true}},
		Rewards: []BattlePassRewardInput{
			{Level: 1, Track: "free", RewardType: "balance", Payload: map[string]any{"amount": 1.0}},
			{Level: 1, Track: "free", RewardType: "concurrency", Payload: map[string]any{"amount": 1.0}},
			{Level: 1, Track: "premium", RewardType: "balance", Payload: map[string]any{"amount": 1.0}},
		},
	}
	result := validateSeasonForPublish(detail, time.Now())
	require.False(t, result.OK)
	require.Contains(t, result.Errors, BattlePassValidationIssue{Level: "error", Code: "REWARD", Message: "only one reward is allowed for each level and track"})
}

func TestValidateSeasonForPublishRestrictsFactBasedTasks(t *testing.T) {
	start := time.Now().Add(time.Hour)
	detail := BattlePassSeasonDetail{
		BattlePassSeason: BattlePassSeason{Name: "s1", Timezone: "Asia/Shanghai", StartAt: start, EndAt: start.Add(24 * time.Hour), PremiumPrice: 9.9, MaxLevel: 1},
		Levels:           []BattlePassLevelInput{{Level: 1, RequiredExp: 0}},
		Tasks: []BattlePassTaskInput{
			{Name: "recharge", TaskType: "recharge_amount", PeriodType: "daily", TargetValue: 1, ExpReward: 1, FilterScope: "all", Enabled: true},
			{Name: "invite", TaskType: "valid_invite_count", PeriodType: "season", TargetValue: 1, ExpReward: 1, FilterScope: "model_family", FilterValues: []string{"openai"}, Enabled: true},
		},
		Rewards: []BattlePassRewardInput{
			{Level: 1, Track: "free", RewardType: "balance", Payload: map[string]any{"amount": 1.0}},
			{Level: 1, Track: "premium", RewardType: "balance", Payload: map[string]any{"amount": 1.0}},
		},
	}
	result := validateSeasonForPublish(detail, time.Now())
	require.False(t, result.OK)
	require.Contains(t, result.Errors, BattlePassValidationIssue{Level: "error", Code: "TASK_PERIOD", Message: "recharge_amount tasks must use season period"})
	require.Contains(t, result.Errors, BattlePassValidationIssue{Level: "error", Code: "TASK_FILTER", Message: "valid_invite_count tasks cannot filter by model"})
}

func TestValidateSeasonForPublishAllowsEveryImplementedTaskType(t *testing.T) {
	start := time.Now().Add(time.Hour)
	detail := BattlePassSeasonDetail{
		BattlePassSeason: BattlePassSeason{Name: "all task types", Timezone: "Asia/Shanghai", StartAt: start, EndAt: start.Add(24 * time.Hour), PremiumPrice: 9.9, MaxLevel: 1},
		Levels:           []BattlePassLevelInput{{Level: 1, RequiredExp: 0}},
		Rewards: []BattlePassRewardInput{
			{Level: 1, Track: "free", RewardType: "balance", Payload: map[string]any{"amount": 1.0}},
			{Level: 1, Track: "premium", RewardType: "balance", Payload: map[string]any{"amount": 1.0}},
		},
	}
	for taskType := range battlePassAllowedTaskTypes {
		period := "daily"
		if taskType == "active_days" || taskType == "distinct_model_families" || taskType == "recharge_count" || taskType == "recharge_amount" || taskType == "valid_invite_count" || taskType == "invitee_recharge_count" {
			period = "season"
		}
		detail.Tasks = append(detail.Tasks, BattlePassTaskInput{
			Name: taskType, TaskType: taskType, PeriodType: period, TargetValue: 1,
			ExpReward: 1, FilterScope: "all", Enabled: true,
		})
	}

	result := validateSeasonForPublish(detail, time.Now())
	require.True(t, result.OK, result.Errors)
}

func TestBattlePassUsageContributionUsesSeasonLocalDailyKeys(t *testing.T) {
	usage := battlePassUsageLog{ID: 10, UserID: 42, Model: "gpt-5", CreatedAt: time.Date(2026, 8, 31, 16, 30, 0, 0, time.UTC)}
	season := BattlePassSeason{Timezone: "Asia/Shanghai"}
	location, err := time.LoadLocation(season.Timezone)
	require.NoError(t, err)
	periodKey := usage.CreatedAt.In(location).Format("2006-01-02")
	require.Equal(t, "2026-09-01", periodKey)
	value, sourceType, sourceID := battlePassUsageContribution("active_days", usage, "openai")
	require.Equal(t, 1.0, value)
	require.Equal(t, "usage_active_day", sourceType)
	// The runtime replaces this source key with the season-local date key.
	require.NotEqual(t, sourceID, battlePassStableID("42:"+periodKey))
}

func TestBattlePassTaskMatchingAndModelFamilies(t *testing.T) {
	task := BattlePassTaskInput{FilterScope: "model_family", FilterValues: []string{"openai"}}
	require.True(t, battlePassTaskMatches(task, "gpt-5", battlePassModelFamily("gpt-5")))
	require.False(t, battlePassTaskMatches(task, "claude-sonnet", battlePassModelFamily("claude-sonnet")))
	require.Equal(t, "other", battlePassModelFamily("unrecognized-model"))
}

func TestRuntimeSeasonStatusUsesClock(t *testing.T) {
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	season := BattlePassSeason{Status: BattlePassStatusScheduled, StartAt: start, EndAt: end}
	require.Equal(t, BattlePassStatusScheduled, runtimeSeasonStatus(season, start.Add(-time.Hour)))
	require.Equal(t, "active", runtimeSeasonStatus(season, start.Add(time.Hour)))
	require.Equal(t, BattlePassStatusEnded, runtimeSeasonStatus(season, end))
	season.Status = BattlePassStatusPaused
	require.Equal(t, BattlePassStatusPaused, runtimeSeasonStatus(season, start.Add(time.Hour)))
}

func TestBattlePassCurrentPublishedSeasonIncludesNearestUpcomingSeason(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	start := now.Add(time.Hour)
	end := start.Add(30 * 24 * time.Hour)
	columns := []string{"id", "name", "description", "status", "timezone", "start_at", "end_at", "premium_price", "max_level", "activation_epoch", "published_at", "enabled_at_snapshot"}
	mock.ExpectQuery(`(?s)FROM battle_pass_seasons\s+WHERE status IN \('scheduled', 'paused', 'ended'\)\s+ORDER BY.*WHEN status='scheduled' AND start_at > \$1 THEN 1`).
		WithArgs(now).
		WillReturnRows(sqlmock.NewRows(columns).AddRow(int64(2), "upcoming", "", BattlePassStatusScheduled, "UTC", start, end, 9.9, 1, 0, now, now))

	svc := NewBattlePassService(db, nil)
	season, err := svc.currentPublishedSeason(context.Background(), now)
	require.NoError(t, err)
	require.NotNil(t, season)
	require.Equal(t, "upcoming", season.Name)
	require.Equal(t, BattlePassStatusScheduled, season.RuntimeStatus)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBattlePassEndedSeasonStillDrainsUsageWindow(t *testing.T) {
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	season := BattlePassSeason{Status: BattlePassStatusScheduled, StartAt: start, EndAt: end}
	require.True(t, battlePassSeasonShouldScan(season, end.Add(time.Minute)))
	season.Status = BattlePassStatusEnded
	require.True(t, battlePassSeasonShouldScan(season, end.Add(24*time.Hour)))
	season.Status = BattlePassStatusPaused
	require.False(t, battlePassSeasonShouldScan(season, end.Add(time.Minute)))
}

func TestListHistoricalEndedBattlePassSeasonsIncludesNaturallyEndedScheduled(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	now := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	start := now.Add(-48 * time.Hour)
	end := now.Add(-time.Hour)
	columns := []string{"id", "name", "description", "status", "timezone", "start_at", "end_at", "premium_price", "max_level", "activation_epoch", "published_at", "enabled_at_snapshot"}
	rows := sqlmock.NewRows(columns).
		AddRow(int64(1), "scheduled-old", "", BattlePassStatusScheduled, "UTC", start, end, 0.0, 1, 0, nil, nil).
		AddRow(int64(2), "ended-old", "", BattlePassStatusEnded, "UTC", start, end, 0.0, 1, 0, nil, nil)
	mock.ExpectQuery(`(?s)FROM battle_pass_seasons\s+WHERE status IN \('scheduled', 'ended'\) AND start_at <= \$1 AND end_at <= \$1`).
		WithArgs(now).
		WillReturnRows(rows)

	svc := NewBattlePassService(db, &battlePassSettingRepoStub{})
	seasons, err := svc.listHistoricalEndedBattlePassSeasons(context.Background(), now)
	require.NoError(t, err)
	require.Len(t, seasons, 2)
	require.Equal(t, BattlePassStatusScheduled, seasons[0].Status)
	require.Equal(t, BattlePassStatusEnded, seasons[1].Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBattlePassLevelForExpUsesPublishedSnapshot(t *testing.T) {
	levels := []BattlePassLevelInput{
		{Level: 1, RequiredExp: 0},
		{Level: 2, RequiredExp: 100},
		{Level: 3, RequiredExp: 250},
	}
	require.Equal(t, 1, battlePassLevelForExp(levels, 99))
	require.Equal(t, 2, battlePassLevelForExp(levels, 100))
	require.Equal(t, 3, battlePassLevelForExp(levels, 999))
	// Invalid entries are ignored rather than allowing a tampered snapshot to
	// lower a user's already-earned level.
	require.Equal(t, 2, battlePassLevelForExp([]BattlePassLevelInput{
		{Level: 1, RequiredExp: 0},
		{Level: 2, RequiredExp: 100},
		{Level: 4, RequiredExp: -1},
	}, 999))
}

func TestBattlePassLevelBoundsUsesPublishedThresholds(t *testing.T) {
	levels := []BattlePassLevelInput{
		{Level: 1, RequiredExp: 0},
		{Level: 2, RequiredExp: 100},
		{Level: 3, RequiredExp: 250},
	}
	start, next := battlePassLevelBounds(levels, 2)
	require.Equal(t, int64(100), start)
	require.NotNil(t, next)
	require.Equal(t, int64(250), *next)
	start, next = battlePassLevelBounds(levels, 3)
	require.Equal(t, int64(250), start)
	require.Nil(t, next)
	// Invalid or missing levels fail closed to a zero-based progress range.
	start, next = battlePassLevelBounds([]BattlePassLevelInput{{Level: 1, RequiredExp: 0}, {Level: 2, RequiredExp: -1}}, 2)
	require.Zero(t, start)
	require.Nil(t, next)
	start, next = battlePassLevelBounds([]BattlePassLevelInput{{Level: 3, RequiredExp: 300}, {Level: 2, RequiredExp: 100}, {Level: 4, RequiredExp: 450}}, 2)
	require.Equal(t, int64(100), start)
	require.NotNil(t, next)
	require.Equal(t, int64(300), *next)
}

func TestBattlePassPaymentContributionRules(t *testing.T) {
	tests := []struct {
		name         string
		status       string
		payAmount    float64
		refundAmount float64
		wantEligible bool
		wantNet      float64
	}{
		{name: "completed", status: "COMPLETED", payAmount: 20, wantEligible: true, wantNet: 20},
		{name: "partial refund", status: "PARTIALLY_REFUNDED", payAmount: 20, refundAmount: 3, wantEligible: true, wantNet: 17},
		{name: "full refund", status: "REFUNDED", payAmount: 20, refundAmount: 20, wantEligible: true, wantNet: 0},
		{name: "failed", status: "FAILED", payAmount: 20, wantEligible: false, wantNet: 0},
		{name: "negative refund is clamped", status: "COMPLETED", payAmount: 20, refundAmount: -3, wantEligible: true, wantNet: 20},
		{name: "refund exceeds payment", status: "REFUNDED", payAmount: 20, refundAmount: 30, wantEligible: true, wantNet: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.wantEligible, battlePassPaymentStatusEligible(tt.status))
			require.Equal(t, tt.wantNet, battlePassPaymentNetContribution(tt.status, tt.payAmount, tt.refundAmount))
		})
	}
}

func TestBattlePassPaymentCountIsPerOrder(t *testing.T) {
	order := battlePassPaymentOrder{Status: "COMPLETED", PayAmount: 123.45, CompletedAt: sql.NullTime{Valid: true, Time: time.Now()}, EligibleUser: true}
	require.Equal(t, 1.0, battlePassPaymentContribution("recharge_count", order.Status, order.PayAmount, order.RefundAmount))
	require.Equal(t, 123.45, battlePassPaymentContribution("recharge_amount", order.Status, order.PayAmount, order.RefundAmount))
	order.RefundAmount = 123.45
	require.Equal(t, 0.0, battlePassPaymentContribution("recharge_count", order.Status, order.PayAmount, order.RefundAmount))
}

func TestBattlePassAffiliateContributionIsOneTimeBoolean(t *testing.T) {
	require.Equal(t, 1.0, battlePassAffiliateContribution(true, true))
	require.Equal(t, 0.0, battlePassAffiliateContribution(false, true))
	require.Equal(t, 0.0, battlePassAffiliateContribution(true, false))
}
