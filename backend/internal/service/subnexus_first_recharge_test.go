package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

type firstRechargeSettingRepoStub struct {
	values map[string]string
	err    error
}

func (r *firstRechargeSettingRepoStub) Get(_ context.Context, key string) (*Setting, error) {
	value, ok := r.values[key]
	if !ok {
		return nil, ErrSettingNotFound
	}
	return &Setting{Key: key, Value: value}, nil
}

func (r *firstRechargeSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	if r.err != nil {
		return "", r.err
	}
	value, ok := r.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}

func (r *firstRechargeSettingRepoStub) Set(_ context.Context, key, value string) error {
	if r.values == nil {
		r.values = make(map[string]string)
	}
	r.values[key] = value
	return nil
}

func (r *firstRechargeSettingRepoStub) GetMultiple(context.Context, []string) (map[string]string, error) {
	return nil, errors.New("unexpected GetMultiple call")
}

func (r *firstRechargeSettingRepoStub) SetMultiple(_ context.Context, values map[string]string) error {
	if r.values == nil {
		r.values = make(map[string]string)
	}
	for key, value := range values {
		r.values[key] = value
	}
	return nil
}

func (r *firstRechargeSettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	return nil, errors.New("unexpected GetAll call")
}

func (r *firstRechargeSettingRepoStub) Delete(context.Context, string) error {
	return errors.New("unexpected Delete call")
}

func TestFirstRechargeConfigRequiresExactIndependentSwitch(t *testing.T) {
	t.Parallel()

	payload := `{"enabled":true,"price":9.9,"credited_amount":12,"ratio":1.2}`
	for _, tc := range []struct {
		name    string
		switchV string
		err     error
		enabled bool
	}{
		{name: "exact true", switchV: "true", enabled: true},
		{name: "uppercase is closed", switchV: "TRUE"},
		{name: "whitespace is closed", switchV: " true "},
		{name: "read failure is closed", switchV: "true", err: errors.New("settings unavailable")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &firstRechargeSettingRepoStub{
				values: map[string]string{
					SettingKeySubNexusFirstRechargeEnabled: tc.switchV,
					SettingKeySubNexusFirstRechargeConfig:  payload,
				},
				err: tc.err,
			}
			cfg := NewFirstRechargeGiftService(nil, repo).Config(context.Background())
			require.Equal(t, tc.enabled, cfg.Enabled)
		})
	}
}

func TestFirstRechargeRefundDoesNotRestorePurchaseEligibility(t *testing.T) {
	ctx := context.Background()
	client, db := newFirstRechargeStateTestClient(t)
	repo := &firstRechargeSettingRepoStub{values: map[string]string{
		SettingKeySubNexusFirstRechargeEnabled: "true",
		SettingKeySubNexusFirstRechargeConfig:  `{"enabled":true,"price":9.9,"credited_amount":12,"ratio":1.2}`,
	}}
	service := NewFirstRechargeGiftService(client, repo)

	insertFirstRechargeTestOrder(t, db, 600, 31, OrderStatusPending, paymentOrderTypeFirstRecharge, time.Now().Add(time.Hour))
	reserveFirstRechargeTestOrder(t, ctx, client, service, 31, 600)
	setFirstRechargeTestOrderStatus(t, db, 600, OrderStatusRecharging)
	require.NoError(t, service.MarkCompleted(ctx, 31, 600, 9.9, 12, time.Now()))
	setFirstRechargeTestOrderStatus(t, db, 600, OrderStatusRefunded)

	_, err := service.PrepareOrder(ctx, 31)
	require.ErrorIs(t, err, ErrFirstRechargeAlreadyPurchased)
}

func TestFirstRechargeReplacementRejectsLateOldOrder(t *testing.T) {
	ctx := context.Background()
	client, db := newFirstRechargeStateTestClient(t)
	service := NewFirstRechargeGiftService(client, nil)

	insertFirstRechargeTestOrder(t, db, 100, 7, OrderStatusPending, paymentOrderTypeFirstRecharge, time.Now().Add(time.Hour))
	reserveFirstRechargeTestOrder(t, ctx, client, service, 7, 100)
	require.NoError(t, service.Release(ctx, 100, OrderStatusCancelled))
	setFirstRechargeTestOrderStatus(t, db, 100, OrderStatusCancelled)

	insertFirstRechargeTestOrder(t, db, 101, 7, OrderStatusPending, paymentOrderTypeFirstRecharge, time.Now().Add(time.Hour))
	reserveFirstRechargeTestOrder(t, ctx, client, service, 7, 101)

	setFirstRechargeTestOrderStatus(t, db, 100, OrderStatusRecharging)
	err := service.ClaimForFulfillment(ctx, 7, 100)
	require.ErrorIs(t, err, ErrFirstRechargeReservationMismatch)

	setFirstRechargeTestOrderStatus(t, db, 100, OrderStatusFailed)
	require.NoError(t, service.ClaimForFulfillment(ctx, 7, 101))
	requireFirstRechargeReservation(t, db, 7, 101, OrderStatusRecharging)
}

func TestFirstRechargePaymentClaimFallsBackForReplacedReservation(t *testing.T) {
	ctx := context.Background()
	client, db := newFirstRechargeStateTestClient(t)
	service := NewFirstRechargeGiftService(client, nil)

	insertFirstRechargeTestOrder(t, db, 110, 17, OrderStatusPending, paymentOrderTypeFirstRecharge, time.Now().Add(time.Hour))
	reserveFirstRechargeTestOrder(t, ctx, client, service, 17, 110)
	require.NoError(t, service.Release(ctx, 110, OrderStatusCancelled))
	setFirstRechargeTestOrderStatus(t, db, 110, OrderStatusCancelled)

	insertFirstRechargeTestOrder(t, db, 111, 17, OrderStatusPending, paymentOrderTypeFirstRecharge, time.Now().Add(time.Hour))
	reserveFirstRechargeTestOrder(t, ctx, client, service, 17, 111)

	claim, err := service.ClaimForPaymentFulfillment(ctx, 17, 110)
	require.NoError(t, err)
	require.False(t, claim.GiftGranted)
	require.True(t, claim.FallbackToPaidAmount)
	requireFirstRechargeReservation(t, db, 17, 111, OrderStatusPending)

	// The replacement order can still receive the one-time promotion. The
	// already-paid old order is settled separately at its actual paid amount.
	claim, err = service.ClaimForPaymentFulfillment(ctx, 17, 111)
	require.NoError(t, err)
	require.True(t, claim.GiftGranted)
	require.False(t, claim.FallbackToPaidAmount)
	requireFirstRechargeReservation(t, db, 17, 111, OrderStatusRecharging)
}

func TestFirstRechargeCompletedMarkerCannotOverwriteReplacement(t *testing.T) {
	ctx := context.Background()
	client, db := newFirstRechargeStateTestClient(t)
	service := NewFirstRechargeGiftService(client, nil)

	insertFirstRechargeTestOrder(t, db, 115, 19, OrderStatusCancelled, paymentOrderTypeFirstRecharge, time.Now().Add(-time.Hour))
	reserveFirstRechargeTestOrder(t, ctx, client, service, 19, 115)
	require.NoError(t, service.Release(ctx, 115, OrderStatusCancelled))
	setFirstRechargeTestOrderStatus(t, db, 115, OrderStatusCancelled)
	insertFirstRechargeTestOrder(t, db, 116, 19, OrderStatusPending, paymentOrderTypeFirstRecharge, time.Now().Add(time.Hour))
	reserveFirstRechargeTestOrder(t, ctx, client, service, 19, 116)

	err := service.MarkCompleted(ctx, 19, 115, 9.9, 12, time.Now())
	require.ErrorIs(t, err, ErrFirstRechargeReservationMismatch)
	requireFirstRechargeReservation(t, db, 19, 116, OrderStatusPending)
}

func TestFirstRechargePaymentClaimFallsBackWhenAnotherRechargeCompleted(t *testing.T) {
	ctx := context.Background()
	client, db := newFirstRechargeStateTestClient(t)
	service := NewFirstRechargeGiftService(client, nil)

	insertFirstRechargeTestOrder(t, db, 120, 18, OrderStatusPending, paymentOrderTypeFirstRecharge, time.Now().Add(time.Hour))
	reserveFirstRechargeTestOrder(t, ctx, client, service, 18, 120)
	insertFirstRechargeTestOrder(t, db, 121, 18, OrderStatusCompleted, paymentOrderTypeBalance, time.Now().Add(time.Hour))

	claim, err := service.ClaimForPaymentFulfillment(ctx, 18, 120)
	require.NoError(t, err)
	require.False(t, claim.GiftGranted)
	require.True(t, claim.FallbackToPaidAmount)
	requireFirstRechargeReservation(t, db, 18, 120, OrderStatusPending)
}

func TestFirstRechargeLatePaymentCanRecoverBeforeReplacement(t *testing.T) {
	ctx := context.Background()
	client, db := newFirstRechargeStateTestClient(t)
	service := NewFirstRechargeGiftService(client, nil)

	insertFirstRechargeTestOrder(t, db, 200, 8, OrderStatusPending, paymentOrderTypeFirstRecharge, time.Now().Add(time.Hour))
	reserveFirstRechargeTestOrder(t, ctx, client, service, 8, 200)
	require.NoError(t, service.Release(ctx, 200, OrderStatusCancelled))
	setFirstRechargeTestOrderStatus(t, db, 200, OrderStatusRecharging)

	require.NoError(t, service.ClaimForFulfillment(ctx, 8, 200))
	requireFirstRechargeReservation(t, db, 8, 200, OrderStatusRecharging)
}

func TestFirstRechargeClaimRejectsAnotherCompletedRecharge(t *testing.T) {
	ctx := context.Background()
	client, db := newFirstRechargeStateTestClient(t)
	service := NewFirstRechargeGiftService(client, nil)

	insertFirstRechargeTestOrder(t, db, 300, 9, OrderStatusPending, paymentOrderTypeFirstRecharge, time.Now().Add(time.Hour))
	reserveFirstRechargeTestOrder(t, ctx, client, service, 9, 300)
	insertFirstRechargeTestOrder(t, db, 301, 9, OrderStatusCompleted, paymentOrderTypeBalance, time.Now().Add(time.Hour))

	err := service.ClaimForFulfillment(ctx, 9, 300)
	require.ErrorIs(t, err, ErrFirstRechargeAlreadyPurchased)
	requireFirstRechargeReservation(t, db, 9, 300, OrderStatusPending)
}

func TestFirstRechargeCompletedMarkerRepairsAndRemainsUnique(t *testing.T) {
	ctx := context.Background()
	client, db := newFirstRechargeStateTestClient(t)
	service := NewFirstRechargeGiftService(client, nil)
	completedAt := time.Now().UTC().Truncate(time.Second)

	require.NoError(t, service.MarkCompleted(ctx, 10, 400, 9.9, 12, completedAt))
	require.NoError(t, service.MarkCompleted(ctx, 10, 400, 9.9, 12, completedAt))
	requireFirstRechargeReservation(t, db, 10, 400, OrderStatusCompleted)

	err := service.MarkCompleted(ctx, 10, 401, 9.9, 12, completedAt)
	require.ErrorIs(t, err, ErrFirstRechargeReservationMismatch)
	requireFirstRechargeReservation(t, db, 10, 400, OrderStatusCompleted)
}

func TestFirstRechargeReconcileStaleReservationsReleasesTerminalOrder(t *testing.T) {
	ctx := context.Background()
	client, db := newFirstRechargeStateTestClient(t)
	service := NewFirstRechargeGiftService(client, nil)

	insertFirstRechargeTestOrder(t, db, 500, 22, OrderStatusFailed, paymentOrderTypeFirstRecharge, time.Now().Add(-time.Hour))
	reserveFirstRechargeTestOrder(t, ctx, client, service, 22, 500)
	// ReserveTx is intentionally allowed to create a pending marker for a
	// payment order that later becomes terminal; this is the failure state the
	// periodic reconciler repairs. Reconciliation does not consult the rollout
	// switch, so it also drains stale markers while the offer is disabled or
	// its setting is not present during a rolling upgrade.
	setFirstRechargeTestOrderStatus(t, db, 500, OrderStatusFailed)

	cleaned, failures, err := service.ReconcileStaleReservations(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, cleaned)
	require.Empty(t, failures)
	requireFirstRechargeReservation(t, db, 22, 500, OrderStatusFailed)
}

func TestFirstRechargeCleanupFailureIsRecordedInPaymentAudit(t *testing.T) {
	ctx := context.Background()
	client, db := newFirstRechargeStateTestClient(t)
	service := &PaymentService{entClient: client}

	service.recordFirstRechargeReservationCleanupFailure(ctx, 501, OrderStatusFailed, errors.New("temporary database failure"))

	var action, detail string
	err := db.QueryRowContext(ctx,
		`SELECT action, detail FROM payment_audit_logs WHERE order_id = ?`, "501").Scan(&action, &detail)
	require.NoError(t, err)
	require.Equal(t, "FIRST_RECHARGE_RESERVATION_CLEANUP_FAILED", action)
	require.Contains(t, detail, "temporary database failure")
}

func newFirstRechargeStateTestClient(t *testing.T) (*dbent.Client, *sql.DB) {
	t.Helper()
	dsn := fmt.Sprintf("file:first_recharge_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	statements := []string{
		`CREATE TABLE settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		`CREATE TABLE payment_orders (
			id INTEGER PRIMARY KEY,
			user_id INTEGER NOT NULL,
			status TEXT NOT NULL,
			order_type TEXT NOT NULL,
			expires_at DATETIME
		)`,
		`CREATE TABLE first_recharge_gift_purchases (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL UNIQUE,
			order_id INTEGER UNIQUE,
			price NUMERIC NOT NULL DEFAULT 0,
			credited_amount NUMERIC NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'PENDING',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			completed_at DATETIME
		)`,
		`CREATE TABLE payment_audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			order_id TEXT NOT NULL,
			action TEXT NOT NULL,
			detail TEXT NOT NULL DEFAULT '',
			operator TEXT NOT NULL DEFAULT 'system',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
	}
	for _, statement := range statements {
		_, err = db.Exec(statement)
		require.NoError(t, err)
	}
	_, err = db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)`, SettingKeySubNexusFirstRechargeEnabled, "true")
	require.NoError(t, err)
	driver := entsql.OpenDB(dialect.SQLite, db)
	client := dbent.NewClient(dbent.Driver(driver))
	t.Cleanup(func() { _ = client.Close() })
	return client, db
}

func insertFirstRechargeTestOrder(t *testing.T, db *sql.DB, id, userID int64, status, orderType string, expiresAt time.Time) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO payment_orders (id, user_id, status, order_type, expires_at) VALUES (?, ?, ?, ?, ?)`,
		id,
		userID,
		status,
		orderType,
		expiresAt,
	)
	require.NoError(t, err)
}

func setFirstRechargeTestOrderStatus(t *testing.T, db *sql.DB, id int64, status string) {
	t.Helper()
	_, err := db.Exec(`UPDATE payment_orders SET status = ? WHERE id = ?`, status, id)
	require.NoError(t, err)
}

func reserveFirstRechargeTestOrder(t *testing.T, ctx context.Context, client *dbent.Client, service *FirstRechargeGiftService, userID, orderID int64) {
	t.Helper()
	tx, err := client.Tx(ctx)
	require.NoError(t, err)
	require.NoError(t, service.ReserveTx(ctx, tx, userID, orderID, 9.9, 12))
	require.NoError(t, tx.Commit())
}

func requireFirstRechargeReservation(t *testing.T, db *sql.DB, userID, orderID int64, status string) {
	t.Helper()
	var actualOrderID int64
	var actualStatus string
	err := db.QueryRow(
		`SELECT order_id, status FROM first_recharge_gift_purchases WHERE user_id = ?`,
		userID,
	).Scan(&actualOrderID, &actualStatus)
	require.NoError(t, err)
	require.Equal(t, orderID, actualOrderID)
	require.Equal(t, status, actualStatus)
}
