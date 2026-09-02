package repository

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func expectAtomicSignupRewardBindingPrefix(mock sqlmock.Sqlmock, now time.Time) {
	profileColumns := []string{
		"user_id", "aff_code", "aff_code_custom", "aff_rebate_rate_percent",
		"inviter_id", "aff_count", "aff_quota", "aff_frozen_quota",
		"aff_history_quota", "created_at", "updated_at",
	}
	profile := func(userID int64, code string) *sqlmock.Rows {
		return sqlmock.NewRows(profileColumns).AddRow(
			userID, code, false, nil, nil, 0, 0.0, 0.0, 0.0, now, now,
		)
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT user_id,.*FROM user_affiliates.*WHERE user_id = \$1`).
		WithArgs(int64(99)).
		WillReturnRows(profile(99, "SELF1234"))
	mock.ExpectQuery(`(?s)SELECT user_id,.*FROM user_affiliates.*WHERE user_id = \$1`).
		WithArgs(int64(41)).
		WillReturnRows(profile(41, "INVITER1"))
	mock.ExpectExec(`(?s)UPDATE user_affiliates SET inviter_id = \$1, updated_at = NOW\(\).*WHERE user_id = \$2 AND inviter_id IS NULL`).
		WithArgs(int64(41), int64(99)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)UPDATE user_affiliates SET aff_count = aff_count \+ 1, updated_at = NOW\(\) WHERE user_id = \$1`).
		WithArgs(int64(41)).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func TestBindInviterAndEnqueueSignupRewardCommitsBindingAndJobTogether(t *testing.T) {
	client, mock := newAffiliateSQLMockClient(t)
	now := time.Now()
	expectAtomicSignupRewardBindingPrefix(mock, now)
	mock.ExpectExec(regexp.QuoteMeta("SAVEPOINT subnexus_signup_reward")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT value FROM settings WHERE key = \$1 FOR UPDATE`).
		WithArgs(service.SettingKeySubNexusInviteRewardsEnabled).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("true"))
	mock.ExpectQuery(`(?s)INSERT INTO subnexus_affiliate_signup_reward_jobs.*ON CONFLICT \(invitee_user_id\) DO NOTHING.*RETURNING id`).
		WithArgs(int64(41), int64(99), 8.25, 2.5, "203.0.113.9", true, 3).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(17)))
	mock.ExpectExec(regexp.QuoteMeta("RELEASE SAVEPOINT subnexus_signup_reward")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	repo := &affiliateRepository{client: client}
	bound, jobID, err := repo.BindInviterAndEnqueueSignupReward(context.Background(), 99, 41, service.AffiliateSignupRewardPending{
		InviterID: 41, InviteeUserID: 99, InviterAmount: 8.25, InviteeAmount: 2.5,
		ClientIP: "203.0.113.9", IPLimitEnabled: true, IPDailyLimit: 3,
	})

	require.NoError(t, err)
	require.True(t, bound)
	require.Equal(t, int64(17), jobID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBindInviterAndEnqueueSignupRewardRollsBackBindingWhenEnqueueFails(t *testing.T) {
	client, mock := newAffiliateSQLMockClient(t)
	expectAtomicSignupRewardBindingPrefix(mock, time.Now())
	mock.ExpectExec(regexp.QuoteMeta("SAVEPOINT subnexus_signup_reward")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT value FROM settings WHERE key = \$1 FOR UPDATE`).
		WithArgs(service.SettingKeySubNexusInviteRewardsEnabled).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("true"))
	mock.ExpectQuery(`(?s)INSERT INTO subnexus_affiliate_signup_reward_jobs.*RETURNING id`).
		WillReturnError(errors.New("queue table unavailable"))
	mock.ExpectExec(regexp.QuoteMeta("ROLLBACK TO SAVEPOINT subnexus_signup_reward")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta("RELEASE SAVEPOINT subnexus_signup_reward")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	repo := &affiliateRepository{client: client}
	bound, jobID, err := repo.BindInviterAndEnqueueSignupReward(context.Background(), 99, 41, service.AffiliateSignupRewardPending{
		InviterID: 41, InviteeUserID: 99, InviterAmount: 8.25, InviteeAmount: 2.5,
	})

	require.ErrorContains(t, err, "queue table unavailable")
	require.False(t, bound)
	require.Zero(t, jobID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func newAffiliateSQLMockClient(t *testing.T) (*dbent.Client, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })
	return client, mock
}

func TestEnqueueSignupRewardClosedGateDoesNotWrite(t *testing.T) {
	client, mock := newAffiliateSQLMockClient(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT value FROM settings WHERE key = \$1 FOR UPDATE`).
		WithArgs(service.SettingKeySubNexusInviteRewardsEnabled).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("false"))
	mock.ExpectCommit()

	repo := &affiliateRepository{client: client}
	jobID, inserted, err := repo.EnqueueSignupReward(context.Background(), service.AffiliateSignupRewardPending{
		InviterID: 41, InviteeUserID: 99, InviterAmount: 8.25, InviteeAmount: 2.5,
		ClientIP: "203.0.113.9", IPLimitEnabled: true, IPDailyLimit: 3,
	})

	require.NoError(t, err)
	require.Zero(t, jobID)
	require.False(t, inserted)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestEnqueueSignupRewardStoresImmutableSnapshot(t *testing.T) {
	client, mock := newAffiliateSQLMockClient(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT value FROM settings WHERE key = \$1 FOR UPDATE`).
		WithArgs(service.SettingKeySubNexusInviteRewardsEnabled).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("true"))
	mock.ExpectQuery(`(?s)INSERT INTO subnexus_affiliate_signup_reward_jobs.*ON CONFLICT \(invitee_user_id\) DO NOTHING.*RETURNING id`).
		WithArgs(int64(41), int64(99), 8.25, 2.5, "203.0.113.9", true, 3).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(17)))
	mock.ExpectCommit()

	repo := &affiliateRepository{client: client}
	jobID, inserted, err := repo.EnqueueSignupReward(context.Background(), service.AffiliateSignupRewardPending{
		InviterID: 41, InviteeUserID: 99, InviterAmount: 8.25, InviteeAmount: 2.5,
		ClientIP: "203.0.113.9", IPLimitEnabled: true, IPDailyLimit: 3,
	})

	require.NoError(t, err)
	require.Equal(t, int64(17), jobID)
	require.True(t, inserted)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestEnqueueSignupRewardFailureIsContainedByOuterTransaction(t *testing.T) {
	client, mock := newAffiliateSQLMockClient(t)
	mock.ExpectBegin()
	outerTx, err := client.Tx(context.Background())
	require.NoError(t, err)
	txCtx := dbent.NewTxContext(context.Background(), outerTx)
	mock.ExpectExec(regexp.QuoteMeta("SAVEPOINT subnexus_signup_reward")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT value FROM settings WHERE key = \$1 FOR UPDATE`).
		WithArgs(service.SettingKeySubNexusInviteRewardsEnabled).
		WillReturnError(errors.New("settings temporarily unavailable"))
	mock.ExpectExec(regexp.QuoteMeta("ROLLBACK TO SAVEPOINT subnexus_signup_reward")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta("RELEASE SAVEPOINT subnexus_signup_reward")).
		WillReturnResult(sqlmock.NewResult(0, 0))

	repo := &affiliateRepository{client: client}
	jobID, inserted, err := repo.EnqueueSignupReward(txCtx, service.AffiliateSignupRewardPending{
		InviterID: 41, InviteeUserID: 99, InviterAmount: 8.25, InviteeAmount: 2.5,
	})
	require.ErrorContains(t, err, "settings temporarily unavailable")
	require.Zero(t, jobID)
	require.False(t, inserted)

	mock.ExpectExec(regexp.QuoteMeta("SELECT 1")).WillReturnResult(sqlmock.NewResult(0, 0))
	_, err = outerTx.Client().ExecContext(txCtx, "SELECT 1")
	require.NoError(t, err)
	mock.ExpectRollback()
	require.NoError(t, outerTx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestProcessSignupRewardTransientFailureSchedulesRetry(t *testing.T) {
	client, mock := newAffiliateSQLMockClient(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT value FROM settings WHERE key = \$1 FOR UPDATE`).
		WithArgs(service.SettingKeySubNexusInviteRewardsEnabled).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("true"))
	mock.ExpectQuery(`(?s)SELECT id, inviter_id, invitee_user_id.*FROM subnexus_affiliate_signup_reward_jobs.*WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(17)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "inviter_id", "invitee_user_id", "inviter_amount", "invitee_amount",
			"client_ip", "ip_limit_enabled", "ip_daily_limit", "status", "attempt_count",
		}).AddRow(int64(17), int64(41), int64(99), 8.25, 2.5, "203.0.113.9", true, 3, "pending", 0))
	mock.ExpectExec(regexp.QuoteMeta("SAVEPOINT subnexus_signup_reward")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(1397638990, hashint8($1::bigint))")).
		WithArgs(int64(99)).
		WillReturnError(errors.New("temporary lock failure"))
	mock.ExpectExec(regexp.QuoteMeta("ROLLBACK TO SAVEPOINT subnexus_signup_reward")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta("RELEASE SAVEPOINT subnexus_signup_reward")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`(?s)UPDATE subnexus_affiliate_signup_reward_jobs.*SET attempt_count = \$1.*WHERE id = \$4 AND status = 'pending'`).
		WithArgs(1, sqlmock.AnyArg(), "lock SubNexus signup reward: temporary lock failure", int64(17)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	repo := &affiliateRepository{client: client}
	result, err := repo.ProcessSignupReward(context.Background(), 17)

	require.ErrorContains(t, err, "temporary lock failure")
	require.True(t, result.RetryScheduled)
	require.Equal(t, int64(41), result.InviterID)
	require.Equal(t, int64(99), result.InviteeUserID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestProcessSignupRewardDeterministicSkipIsDurable(t *testing.T) {
	client, mock := newAffiliateSQLMockClient(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT value FROM settings WHERE key = \$1 FOR UPDATE`).
		WithArgs(service.SettingKeySubNexusInviteRewardsEnabled).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("true"))
	mock.ExpectQuery(`(?s)SELECT id, inviter_id, invitee_user_id.*FROM subnexus_affiliate_signup_reward_jobs.*WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(17)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "inviter_id", "invitee_user_id", "inviter_amount", "invitee_amount",
			"client_ip", "ip_limit_enabled", "ip_daily_limit", "status", "attempt_count",
		}).AddRow(int64(17), int64(41), int64(99), 8.25, 2.5, "203.0.113.9", true, 3, "pending", 0))
	mock.ExpectExec(regexp.QuoteMeta("SAVEPOINT subnexus_signup_reward")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(1397638990, hashint8($1::bigint))")).
		WithArgs(int64(99)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`(?s)SELECT EXISTS .*FROM user_affiliates.*inviter_id = \$2`).
		WithArgs(int64(99), int64(41)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec(regexp.QuoteMeta("RELEASE SAVEPOINT subnexus_signup_reward")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`(?s)UPDATE subnexus_affiliate_signup_reward_jobs.*SET status = \$1.*WHERE id = \$3 AND status = 'pending'`).
		WithArgs("skipped", "inviter_mismatch", int64(17)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	repo := &affiliateRepository{client: client}
	result, err := repo.ProcessSignupReward(context.Background(), 17)

	require.NoError(t, err)
	require.True(t, result.Skipped)
	require.Equal(t, "inviter_mismatch", result.SkipReason)
	require.NoError(t, mock.ExpectationsWereMet())
}
