package repository

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"
)

func TestAffiliateSignupRewardFailureRollsBackToSavepointInOuterTransaction(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })

	mock.ExpectBegin()
	outerTx, err := client.Tx(context.Background())
	require.NoError(t, err)
	txCtx := dbent.NewTxContext(context.Background(), outerTx)

	mock.ExpectExec(regexp.QuoteMeta("SAVEPOINT subnexus_signup_reward")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(1397638990, hashint8($1::bigint))")).
		WithArgs(int64(99)).
		WillReturnError(errors.New("lock failed"))
	mock.ExpectExec(regexp.QuoteMeta("ROLLBACK TO SAVEPOINT subnexus_signup_reward")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta("RELEASE SAVEPOINT subnexus_signup_reward")).
		WillReturnResult(sqlmock.NewResult(0, 0))

	repo := &affiliateRepository{client: client}
	_, err = repo.GrantSignupReward(txCtx, 41, 99, 8.25, 2.5, "203.0.113.9", true, 3)
	require.ErrorContains(t, err, "lock SubNexus signup reward")

	// The optional failure was contained, so callers can continue using and
	// eventually commit or roll back their surrounding registration transaction.
	mock.ExpectExec(regexp.QuoteMeta("SELECT 1")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	_, err = outerTx.Client().ExecContext(txCtx, "SELECT 1")
	require.NoError(t, err)

	mock.ExpectRollback()
	require.NoError(t, outerTx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAffiliateSignupRewardReusesOuterTransactionAndReleasesSavepoint(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })

	mock.ExpectBegin()
	outerTx, err := client.Tx(context.Background())
	require.NoError(t, err)
	txCtx := dbent.NewTxContext(context.Background(), outerTx)

	mock.ExpectExec(regexp.QuoteMeta("SAVEPOINT subnexus_signup_reward")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(1397638990, hashint8($1::bigint))")).
		WithArgs(int64(99)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`(?s)SELECT EXISTS .*FROM user_affiliates.*user_id = \$1.*inviter_id = \$2`).
		WithArgs(int64(99), int64(41)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`(?s)SELECT EXISTS .*FROM user_affiliate_ledger.*source_user_id = \$1`).
		WithArgs(int64(99)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(regexp.QuoteMeta("RELEASE SAVEPOINT subnexus_signup_reward")).
		WillReturnResult(sqlmock.NewResult(0, 0))

	repo := &affiliateRepository{client: client}
	result, err := repo.GrantSignupReward(txCtx, 41, 99, 8.25, 2.5, "", false, 3)
	require.NoError(t, err)
	require.True(t, result.Skipped)
	require.Equal(t, "already_granted", result.SkipReason)

	mock.ExpectRollback()
	require.NoError(t, outerTx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAffiliateSignupRewardAlreadyGrantedSkipsBeforeBalanceWrite(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(1397638990, hashint8($1::bigint))")).
		WithArgs(int64(99)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`(?s)SELECT EXISTS .*FROM user_affiliates.*user_id = \$1.*inviter_id = \$2`).
		WithArgs(int64(99), int64(41)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`(?s)SELECT EXISTS .*FROM user_affiliate_ledger.*source_user_id = \$1`).
		WithArgs(int64(99)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectCommit()

	repo := &affiliateRepository{client: client}
	result, err := repo.GrantSignupReward(context.Background(), 41, 99, 8.25, 2.5, "", false, 3)

	require.NoError(t, err)
	require.False(t, result.Applied)
	require.True(t, result.Skipped)
	require.Equal(t, "already_granted", result.SkipReason)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAffiliateSignupRewardIPDailyLimitSkipsBeforeBalanceWrite(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(1397638990, hashint8($1::bigint))")).
		WithArgs(int64(99)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`(?s)SELECT EXISTS .*FROM user_affiliates.*user_id = \$1.*inviter_id = \$2`).
		WithArgs(int64(99), int64(41)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`(?s)SELECT EXISTS .*FROM user_affiliate_ledger.*source_user_id = \$1`).
		WithArgs(int64(99)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(1397638991, hashtext($1))")).
		WithArgs("203.0.113.9").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`(?s)SELECT COUNT\(DISTINCT source_user_id\).*source_ip = \$1.*created_at >= CURRENT_DATE`).
		WithArgs("203.0.113.9").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
	mock.ExpectCommit()

	repo := &affiliateRepository{client: client}
	result, err := repo.GrantSignupReward(context.Background(), 41, 99, 8.25, 2.5, "203.0.113.9", true, 3)

	require.NoError(t, err)
	require.False(t, result.Applied)
	require.True(t, result.Skipped)
	require.Equal(t, "ip_daily_limit", result.SkipReason)
	require.NoError(t, mock.ExpectationsWereMet())
}
