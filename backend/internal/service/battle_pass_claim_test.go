package service

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestBuildBattlePassRewardStatesDistinguishesClaimability(t *testing.T) {
	rewards := []BattlePassRewardInput{
		{ID: 11, Level: 1, Track: "free", RewardType: "balance", Payload: map[string]any{"amount": 1.0}},
		{ID: 12, Level: 1, Track: "premium", RewardType: "concurrency", Payload: map[string]any{"amount": 1.0}},
		{ID: 21, Level: 2, Track: "free", RewardType: "badge", Payload: map[string]any{"name": "先锋"}},
		{ID: 31, Level: 3, Track: "free", RewardType: "balance", Payload: map[string]any{"amount": 3.0}},
	}
	grants := map[int64]battlePassGrantDisplayState{
		21: {status: "granted"},
	}

	states, err := buildBattlePassRewardStates(rewards, 2, false, grants)
	require.NoError(t, err)
	require.Equal(t, "claimable", states[0].Status)
	require.Equal(t, "premium_locked", states[1].Status)
	require.Equal(t, "granted", states[2].Status)
	require.Equal(t, "locked", states[3].Status)
}

func TestClaimableBattlePassRewardIDsOnlyReturnsEligibleUnclaimedRewards(t *testing.T) {
	states := []BattlePassRewardState{
		{ID: 1, Status: "claimable"},
		{ID: 2, Status: "locked"},
		{ID: 3, Status: "premium_locked"},
		{ID: 4, Status: "pending"},
		{ID: 5, Status: "granted"},
		{ID: 6, Status: "claimable"},
	}

	require.Equal(t, []int64{1, 6}, claimableBattlePassRewardIDs(states))
}

func TestValidateBattlePassRewardClaimStateSeparatesLevelAndPremiumLocks(t *testing.T) {
	require.NoError(t, validateBattlePassRewardClaimState(BattlePassRewardState{Status: "claimable"}))
	require.NoError(t, validateBattlePassRewardClaimState(BattlePassRewardState{Status: "granted"}))

	err := validateBattlePassRewardClaimState(BattlePassRewardState{Status: "locked"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "BATTLE_PASS_LEVEL_REQUIRED")

	err = validateBattlePassRewardClaimState(BattlePassRewardState{Status: "premium_locked"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "BATTLE_PASS_PREMIUM_REQUIRED")
}

func TestEnqueueManualRewardClaimsIsIdempotent(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT value FROM settings WHERE key=\$1 FOR UPDATE`).
		WithArgs(SettingKeyBattlePassEnabled).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("true"))
	mock.ExpectQuery(`INSERT INTO battle_pass_reward_grants`).
		WithArgs(int64(7), int64(42), int64(11)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(101)))
	mock.ExpectQuery(`INSERT INTO battle_pass_reward_grants`).
		WithArgs(int64(7), int64(42), int64(12)).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectCommit()

	svc := NewBattlePassService(db, &battlePassSettingRepoStub{values: map[string]string{SettingKeyBattlePassEnabled: "true"}})
	grantIDs, err := svc.enqueueManualRewardClaims(context.Background(), 7, 42, []int64{11, 12})
	require.NoError(t, err)
	require.Equal(t, []int64{101}, grantIDs)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestEnqueueManualRewardClaimsRechecksDisabledSwitchInsideTransaction(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT value FROM settings WHERE key=\$1 FOR UPDATE`).
		WithArgs(SettingKeyBattlePassEnabled).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("false"))
	mock.ExpectRollback()

	svc := NewBattlePassService(db, &battlePassSettingRepoStub{values: map[string]string{SettingKeyBattlePassEnabled: "true"}})
	_, err = svc.enqueueManualRewardClaims(context.Background(), 7, 42, []int64{11})
	require.Error(t, err)
	require.Contains(t, err.Error(), "BATTLE_PASS_DISABLED")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReconcilePremiumRewardGrantsDoesNotAutoEnqueue(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	svc := NewBattlePassService(db, &battlePassSettingRepoStub{values: map[string]string{SettingKeyBattlePassEnabled: "true"}})
	require.NoError(t, svc.ReconcilePremiumRewardGrantsOnce(context.Background(), time.Now()))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplySubscriptionRewardMatchesIdempotencyMarkerAsWholeNoteLine(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	grant := battlePassGrant{
		ID:         12,
		UserID:     42,
		RewardType: "subscription_days",
		Payload: map[string]any{
			"group_id": float64(7),
			"days":     float64(30),
		},
	}
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT value FROM settings WHERE key=\$1 FOR UPDATE`).
		WithArgs(SettingKeyBattlePassEnabled).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("true"))
	mock.ExpectQuery(`(?s)SELECT EXISTS\(.*ANY\(string_to_array`).
		WithArgs(int64(42), int64(7), "battle_pass_reward:12").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`UPDATE battle_pass_reward_grants`).
		WithArgs(int64(12), "granted", "").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	svc := NewBattlePassService(db, nil)
	// A non-nil service is sufficient because an exact existing marker skips
	// assignment. This regression protects grant 12 from matching grant 123.
	svc.subscriptions = &SubscriptionService{}
	result, changedUserID, err := svc.applySubscriptionReward(context.Background(), grant)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Zero(t, changedUserID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBattlePassSubscriptionRewardKeepsGateTransactionUntilGrantCommit(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT value FROM settings WHERE key=\$1 FOR UPDATE`).
		WithArgs(SettingKeyBattlePassEnabled).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("true"))
	mock.ExpectQuery(`(?s)UPDATE battle_pass_reward_grants.*SET status='processing'.*RETURNING id, season_id, user_id, reward_id`).
		WithArgs(int64(101), int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "season_id", "user_id", "reward_id"}).AddRow(int64(101), int64(7), int64(42), int64(11)))
	mock.ExpectQuery(`SELECT config_snapshot FROM battle_pass_seasons WHERE id=\$1`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"config_snapshot"}).AddRow([]byte(`{"rewards":[{"id":11,"level":1,"track":"premium","reward_type":"subscription_days","payload":{"group_id":7,"days":30}}]}`)))
	mock.ExpectQuery(`(?s)SELECT EXISTS\(.*ANY\(string_to_array`).
		WithArgs(int64(42), int64(7), "battle_pass_reward:101").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`UPDATE battle_pass_reward_grants`).
		WithArgs(int64(101), "granted", "").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	svc := NewBattlePassService(db, nil)
	svc.subscriptions = &SubscriptionService{}
	require.NoError(t, svc.processRewardGrantByID(context.Background(), 101, 42))
	require.NoError(t, mock.ExpectationsWereMet())
}
