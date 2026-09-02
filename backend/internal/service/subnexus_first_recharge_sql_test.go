package service

import (
	"context"
	"regexp"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"
)

func TestFirstRechargeReserveReplacementUsesLockedCompareAndSwap(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	defer client.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT value FROM settings WHERE key = \$1 FOR UPDATE`).
		WithArgs(SettingKeySubNexusFirstRechargeEnabled).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("true"))
	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs(int64(7), OrderStatusCompleted, paymentOrderTypeBalance, paymentOrderTypeSubscription, paymentOrderTypeFirstRecharge).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT p.order_id, p.status, p.completed_at, COALESCE(o.status, ''), o.expires_at")).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"order_id", "status", "completed_at", "order_status", "expires_at"}).
			AddRow(int64(100), OrderStatusPending, nil, OrderStatusCancelled, time.Now().Add(-time.Hour)))
	mock.ExpectExec(`UPDATE first_recharge_gift_purchases[\s\S]*AND order_id = \$11`).
		WithArgs(
			int64(101),
			9.9,
			12.0,
			OrderStatusPending,
			sqlmock.AnyArg(),
			int64(7),
			OrderStatusPending,
			OrderStatusCancelled,
			OrderStatusExpired,
			OrderStatusFailed,
			int64(100),
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx, err := client.Tx(context.Background())
	require.NoError(t, err)
	svc := NewFirstRechargeGiftService(client, nil)
	require.NoError(t, svc.ReserveTx(context.Background(), tx, 7, 101, 9.9, 12))
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFirstRechargeReserveRechecksDisabledSwitchInsideOrderTransaction(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	defer client.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT value FROM settings WHERE key = \$1 FOR UPDATE`).
		WithArgs(SettingKeySubNexusFirstRechargeEnabled).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("false"))
	mock.ExpectRollback()

	tx, err := client.Tx(context.Background())
	require.NoError(t, err)
	err = NewFirstRechargeGiftService(client, nil).ReserveTx(context.Background(), tx, 7, 101, 9.9, 12)
	require.ErrorIs(t, err, ErrFirstRechargeDisabled)
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}
