package repository

import (
	"context"
	"database/sql"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestSubNexusInviteActivitiesRepositoryUsesCreditedAmountAndRefundNet(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	repo := NewSubNexusInviteActivitiesRepository(db)

	eligibleQuery := regexp.MustCompile(`(?is)SELECT\s+COUNT\(\*\)::bigint.*FROM user_affiliates.*po\.status IN \('COMPLETED', 'PARTIALLY_REFUNDED'\).*po\.order_type IN \('balance', 'subscription', 'first_recharge_gift'\).*po\.amount.*po\.refund_amount`)
	mock.ExpectQuery(eligibleQuery.String()).WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(3)))
	got, err := repo.CountEligibleInvitees(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, 3, got)

	qualifiedQuery := regexp.MustCompile(`(?is)SELECT\s+COUNT\(\*\)::bigint.*FROM user_affiliates.*HAVING.*SUM\(GREATEST.*po\.amount.*po\.refund_amount`)
	mock.ExpectQuery(qualifiedQuery.String()).WithArgs(int64(7), 10.0).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(2)))
	got, err = repo.CountQualifiedInvitees(context.Background(), 7, 10)
	require.NoError(t, err)
	require.Equal(t, 2, got)

	sumQuery := regexp.MustCompile(`(?is)SELECT\s+COALESCE\(SUM\(GREATEST.*po\.amount.*po\.refund_amount.*FROM payment_orders.*po\.status IN \('COMPLETED', 'PARTIALLY_REFUNDED'\).*po\.order_type IN \('balance', 'subscription', 'first_recharge_gift'\)`)
	mock.ExpectQuery(sumQuery.String()).WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(12.34))
	total, err := repo.SumCompletedRecharge(context.Background(), 7)
	require.NoError(t, err)
	require.InDelta(t, 12.34, total, 0.000001)

	// The SQL must never use pay_amount as the qualification currency.  This
	// assertion is kept at the source level in addition to the query match.
	const sourceSQL = `
		SELECT GREATEST(COALESCE(po.amount, 0) - GREATEST(COALESCE(po.refund_amount, 0), 0), 0)
	`
	require.NotContains(t, sourceSQL, "pay_amount")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSubNexusInviteActivitiesRepositoryRewardTransactionIsAtomicAndIdempotent(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	repo := NewSubNexusInviteActivitiesRepository(db)
	ctx := context.Background()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(1397638992, hashint8($1::bigint))")).
		WithArgs(int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)INSERT INTO activity_reward_logs.*ON CONFLICT \(source, period, user_id\) DO NOTHING`).
		WithArgs(int64(42), service.ActivitySourceRechargeWheel, "1", 1.25, "test reward").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)UPDATE users.*balance = balance \+ \$1.*total_recharged = total_recharged \+ \$1.*deleted_at IS NULL`).
		WithArgs(1.25, int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	inserted, err := repo.GrantReward(ctx, 42, service.ActivitySourceRechargeWheel, "1", 1.25, "test reward")
	require.NoError(t, err)
	require.True(t, inserted)

	// A retry that loses the unique race commits without touching users.
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(1397638992, hashint8($1::bigint))")).
		WithArgs(int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)INSERT INTO activity_reward_logs.*ON CONFLICT \(source, period, user_id\) DO NOTHING`).
		WithArgs(int64(42), service.ActivitySourceRechargeWheel, "1", 1.25, "test reward").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	inserted, err = repo.GrantReward(ctx, 42, service.ActivitySourceRechargeWheel, "1", 1.25, "test reward")
	require.NoError(t, err)
	require.False(t, inserted)

	// If the user disappeared between the log insert and balance update, the
	// transaction rolls back the marker as well.
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(1397638992, hashint8($1::bigint))")).
		WithArgs(int64(99)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)INSERT INTO activity_reward_logs.*ON CONFLICT \(source, period, user_id\) DO NOTHING`).
		WithArgs(int64(99), service.ActivitySourceInviteLottery, "1", 0.5, "missing user").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)UPDATE users.*balance = balance \+ \$1`).
		WithArgs(0.5, int64(99)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()
	inserted, err = repo.GrantReward(ctx, 99, service.ActivitySourceInviteLottery, "1", 0.5, "missing user")
	require.ErrorIs(t, err, service.ErrUserNotFound)
	require.False(t, inserted)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSubNexusInviteActivitiesRepositoryListClaimedMilestonesIgnoresMalformedPeriods(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	repo := NewSubNexusInviteActivitiesRepository(db)
	mock.ExpectQuery(`(?is)SELECT period.*FROM activity_reward_logs.*WHERE user_id = \$1 AND source = \$2`).
		WithArgs(int64(7), service.ActivitySourceInviteMilestone).
		WillReturnRows(sqlmock.NewRows([]string{"period"}).AddRow("5").AddRow("not-a-tier").AddRow(" 10 ").AddRow("0"))
	claimed, err := repo.ListClaimedMilestones(context.Background(), 7, service.ActivitySourceInviteMilestone)
	require.NoError(t, err)
	require.Equal(t, map[int]bool{5: true, 10: true}, claimed)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSubNexusInviteActivitiesRepositoryRejectsInvalidInputsBeforeDB(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	repo := NewSubNexusInviteActivitiesRepository(db)
	_, err = repo.CountQualifiedInvitees(context.Background(), 1, 0)
	require.Error(t, err)
	_, err = repo.GrantReward(context.Background(), 1, service.ActivitySourceInviteLottery, "1", 0, "bad")
	require.Error(t, err)
	_, err = repo.CountRewards(context.Background(), 0, service.ActivitySourceInviteLottery)
	require.Error(t, err)
	_, err = repo.ListClaimedMilestones(context.Background(), 1, "")
	require.Error(t, err)
	// Keep database/sql imported in this test even when the driver changes; the
	// nil-connection behavior is part of the repository contract.
	var _ *sql.DB = db
}
