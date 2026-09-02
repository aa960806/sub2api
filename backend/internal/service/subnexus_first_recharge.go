package service

import (
	"context"
	"database/sql"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	// SettingKeySubNexusFirstRechargeEnabled is the independent rollout switch.
	// It deliberately does not read the legacy ACTIVITY_CONFIG value, so a
	// rollback to an older binary cannot accidentally enable this feature.
	SettingKeySubNexusFirstRechargeEnabled = "subnexus_first_recharge_enabled"
	// SettingKeySubNexusFirstRechargeConfig stores the offer policy as JSON.
	SettingKeySubNexusFirstRechargeConfig = "subnexus_first_recharge_config"
	// Keep reconciliation bounded so a large backlog cannot monopolize the
	// payment-expiry worker.
	firstRechargeReservationReconcileLimit = 100
)

var (
	ErrFirstRechargeDisabled = infraerrors.Forbidden(
		"FIRST_RECHARGE_DISABLED",
		"first recharge gift is disabled",
	)
	ErrFirstRechargeAlreadyPurchased = infraerrors.Conflict(
		"FIRST_RECHARGE_ALREADY_PURCHASED",
		"first recharge gift is only available before the first completed recharge",
	)
	ErrFirstRechargePending = infraerrors.Conflict(
		"FIRST_RECHARGE_PENDING",
		"first recharge gift order is already pending",
	)
	ErrFirstRechargeReservationMismatch = infraerrors.Conflict(
		"FIRST_RECHARGE_RESERVATION_MISMATCH",
		"first recharge gift reservation belongs to another order",
	)
)

// FirstRechargeGiftConfig is intentionally independent from the legacy
// activity JSON. Price is the amount charged at the gateway and
// CreditedAmount is the amount placed in the user's balance after fulfillment.
type FirstRechargeGiftConfig struct {
	Enabled        bool    `json:"enabled"`
	Price          float64 `json:"price"`
	CreditedAmount float64 `json:"credited_amount"`
	Ratio          float64 `json:"ratio"`
}

// FirstRechargeGiftStatus is the user-facing eligibility snapshot. It does
// not expose provider credentials or any internal reservation details.
type FirstRechargeGiftStatus struct {
	Enabled        bool       `json:"enabled"`
	Purchased      bool       `json:"purchased"`
	Pending        bool       `json:"pending"`
	Price          float64    `json:"price"`
	CreditedAmount float64    `json:"credited_amount"`
	Ratio          float64    `json:"ratio"`
	OrderID        *int64     `json:"order_id,omitempty"`
	PurchasedAt    *time.Time `json:"purchased_at,omitempty"`
}

// FirstRechargeGiftService owns only the offer policy and reservation table.
// PaymentService remains responsible for provider calls and balance
// fulfillment; this separation keeps the migration reversible and auditable.
type FirstRechargeGiftService struct {
	db       *dbent.Client
	settings SettingRepository
	notify   func()
}

func NewFirstRechargeGiftService(db *dbent.Client, settings SettingRepository) *FirstRechargeGiftService {
	return &FirstRechargeGiftService{db: db, settings: settings}
}

// firstRechargeGiftService keeps the payment integration lazy so existing
// PaymentService constructors and generated Wire code remain compatible with
// older deployments. The service has no process-global state.
func (s *PaymentService) firstRechargeGiftService() *FirstRechargeGiftService {
	if s == nil || s.entClient == nil || s.configService == nil {
		return nil
	}
	return NewFirstRechargeGiftService(s.entClient, s.configService.settingRepo)
}

func (s *PaymentService) prepareFirstRechargeGiftOrder(ctx context.Context, userID int64) (FirstRechargeGiftConfig, error) {
	giftService := s.firstRechargeGiftService()
	if giftService == nil {
		return FirstRechargeGiftConfig{}, infraerrors.ServiceUnavailable("FIRST_RECHARGE_UNAVAILABLE", "first recharge gift is unavailable")
	}
	return giftService.PrepareOrder(ctx, userID)
}

// GetFirstRechargeGiftStatus exposes the isolated user status to handlers.
func (s *PaymentService) GetFirstRechargeGiftStatus(ctx context.Context, userID int64) (*FirstRechargeGiftStatus, error) {
	giftService := s.firstRechargeGiftService()
	if giftService == nil {
		return nil, infraerrors.ServiceUnavailable("FIRST_RECHARGE_UNAVAILABLE", "first recharge gift is unavailable")
	}
	return giftService.Status(ctx, userID)
}

func (s *PaymentService) GetFirstRechargeGiftConfig(ctx context.Context) FirstRechargeGiftConfig {
	giftService := s.firstRechargeGiftService()
	if giftService == nil {
		return DefaultFirstRechargeGiftConfig()
	}
	return giftService.Config(ctx)
}

func (s *PaymentService) UpdateFirstRechargeGiftConfig(ctx context.Context, cfg FirstRechargeGiftConfig) (FirstRechargeGiftConfig, error) {
	giftService := s.firstRechargeGiftService()
	if giftService == nil {
		return cfg, infraerrors.ServiceUnavailable("FIRST_RECHARGE_UNAVAILABLE", "first recharge gift is unavailable")
	}
	return giftService.UpdateConfig(ctx, cfg)
}

func (s *PaymentService) claimFirstRechargeGiftForFulfillment(ctx context.Context, userID, orderID int64) error {
	giftService := s.firstRechargeGiftService()
	if giftService == nil {
		return infraerrors.ServiceUnavailable("FIRST_RECHARGE_UNAVAILABLE", "first recharge gift is unavailable")
	}
	return giftService.ClaimForFulfillment(ctx, userID, orderID)
}

// claimFirstRechargeGiftForPayment is the payment-specific reservation path.
// A paid order must never be silently dropped only because a stale, unpaid
// first-recharge order was replaced. In that narrow case the caller settles
// the order as a normal recharge for its actual paid amount. Database errors
// and concurrent state errors remain hard failures so they can be retried.
func (s *PaymentService) claimFirstRechargeGiftForPayment(ctx context.Context, userID, orderID int64) (firstRechargeFulfillmentResult, error) {
	giftService := s.firstRechargeGiftService()
	if giftService == nil {
		return firstRechargeFulfillmentResult{}, infraerrors.ServiceUnavailable("FIRST_RECHARGE_UNAVAILABLE", "first recharge gift is unavailable")
	}
	return giftService.ClaimForPaymentFulfillment(ctx, userID, orderID)
}

func (s *PaymentService) markFirstRechargeGiftCompleted(ctx context.Context, userID, orderID int64, price, credited float64, completedAt time.Time) error {
	giftService := s.firstRechargeGiftService()
	if giftService == nil {
		return infraerrors.ServiceUnavailable("FIRST_RECHARGE_UNAVAILABLE", "first recharge gift is unavailable")
	}
	return giftService.MarkCompleted(ctx, userID, orderID, price, credited, completedAt)
}

func (s *PaymentService) releaseFirstRechargeGiftReservation(ctx context.Context, orderID int64, status string) error {
	giftService := s.firstRechargeGiftService()
	if giftService == nil {
		return infraerrors.ServiceUnavailable("FIRST_RECHARGE_UNAVAILABLE", "first recharge gift is unavailable")
	}
	return giftService.Release(ctx, orderID, status)
}

// recordFirstRechargeReservationCleanupFailure makes a cleanup failure
// durable in the payment audit trail. The terminal payment transition remains
// committed and the reservation is retried by the expiry worker.
func (s *PaymentService) recordFirstRechargeReservationCleanupFailure(ctx context.Context, orderID int64, status string, cleanupErr error) {
	if cleanupErr == nil {
		return
	}
	detail := map[string]any{
		"reservation_status": strings.ToUpper(strings.TrimSpace(status)),
		"error":              cleanupErr.Error(),
		"retryable":          true,
	}
	if s == nil || s.entClient == nil {
		slog.Warn("first recharge reservation cleanup failed", "order_id", orderID, "status", status, "error", cleanupErr)
		return
	}
	s.writeAuditLog(ctx, orderID, "FIRST_RECHARGE_RESERVATION_CLEANUP_FAILED", "system", detail)
	slog.Warn("first recharge reservation cleanup failed; queued for retry", "order_id", orderID, "status", status, "error", cleanupErr)
}

// ReconcileFirstRechargeGiftReservations retries cleanup for reservations
// whose payment order is already terminal. It runs regardless of the offer
// switch so disabling the feature still drains old pending markers safely.
func (s *PaymentService) ReconcileFirstRechargeGiftReservations(ctx context.Context) (int, error) {
	giftService := s.firstRechargeGiftService()
	if giftService == nil {
		return 0, infraerrors.ServiceUnavailable("FIRST_RECHARGE_UNAVAILABLE", "first recharge gift is unavailable")
	}
	cleaned, failures, err := giftService.ReconcileStaleReservations(ctx)
	for _, failure := range failures {
		s.recordFirstRechargeReservationCleanupFailure(ctx, failure.OrderID, failure.Status, failure.Err)
	}
	if err != nil {
		return cleaned, err
	}
	if len(failures) > 0 {
		return cleaned, fmt.Errorf("%d first recharge reservation cleanup attempt(s) failed", len(failures))
	}
	return cleaned, nil
}

func (s *FirstRechargeGiftService) SetSettingsUpdatedNotifier(notifier func()) {
	if s != nil {
		s.notify = notifier
	}
}

func DefaultFirstRechargeGiftConfig() FirstRechargeGiftConfig {
	return FirstRechargeGiftConfig{
		Enabled:        false,
		Price:          9.90,
		CreditedAmount: 12,
		Ratio:          1.2,
	}
}

// Config returns the effective runtime policy. Every missing, malformed, or
// invalid setting is treated as disabled (fail-closed). Only the exact
// lowercase string "true" enables the independent switch.
func (s *FirstRechargeGiftService) Config(ctx context.Context) FirstRechargeGiftConfig {
	defaults := DefaultFirstRechargeGiftConfig()
	if s == nil || s.settings == nil {
		return defaults
	}
	rawConfig, err := s.settings.GetValue(ctx, SettingKeySubNexusFirstRechargeConfig)
	if err != nil || strings.TrimSpace(rawConfig) == "" {
		return defaults
	}
	trimmed := strings.TrimSpace(rawConfig)
	if trimmed == "null" || !strings.HasPrefix(trimmed, "{") {
		return defaults
	}
	var cfg FirstRechargeGiftConfig
	if err := json.Unmarshal([]byte(rawConfig), &cfg); err != nil {
		return defaults
	}
	if err := validateFirstRechargeGiftConfig(cfg); err != nil {
		return defaults
	}
	rawEnabled, err := s.settings.GetValue(ctx, SettingKeySubNexusFirstRechargeEnabled)
	if err != nil || rawEnabled != "true" || !cfg.Enabled {
		cfg.Enabled = false
		return cfg
	}
	cfg.Enabled = true
	return cfg
}

// UpdateConfig validates and atomically persists both the policy and the
// independent switch. Disabling remains possible even when a stale/invalid
// offer was previously stored; enabling always requires a valid positive
// price and credited amount.
func (s *FirstRechargeGiftService) UpdateConfig(ctx context.Context, cfg FirstRechargeGiftConfig) (FirstRechargeGiftConfig, error) {
	if s == nil || s.settings == nil {
		return cfg, infraerrors.InternalServer(
			"SUBNEXUS_FIRST_RECHARGE_SETTINGS_UNAVAILABLE",
			"first recharge settings repository is unavailable",
		)
	}
	cfg = normalizeFirstRechargeGiftConfig(cfg)
	if !cfg.Enabled {
		// Keep a usable display configuration while the feature is off. Runtime
		// still short-circuits before touching payment tables.
		if err := validateFirstRechargeGiftAmounts(cfg); err != nil {
			defaults := DefaultFirstRechargeGiftConfig()
			cfg.Price = defaults.Price
			cfg.CreditedAmount = defaults.CreditedAmount
			cfg.Ratio = defaults.Ratio
		}
	} else if err := validateFirstRechargeGiftConfig(cfg); err != nil {
		return cfg, err
	}
	payload, err := json.Marshal(cfg)
	if err != nil {
		return cfg, fmt.Errorf("marshal first recharge config: %w", err)
	}
	if err := s.settings.SetMultiple(ctx, map[string]string{
		SettingKeySubNexusFirstRechargeEnabled: strconv.FormatBool(cfg.Enabled),
		SettingKeySubNexusFirstRechargeConfig:  string(payload),
	}); err != nil {
		return cfg, fmt.Errorf("save first recharge config: %w", err)
	}
	if s.notify != nil {
		s.notify()
	}
	return cfg, nil
}

func normalizeFirstRechargeGiftConfig(cfg FirstRechargeGiftConfig) FirstRechargeGiftConfig {
	if cfg.Price > 0 && math.IsNaN(cfg.Price) == false && math.IsInf(cfg.Price, 0) == false {
		cfg.Price = math.Round(cfg.Price*100) / 100
	}
	if cfg.CreditedAmount > 0 && math.IsNaN(cfg.CreditedAmount) == false && math.IsInf(cfg.CreditedAmount, 0) == false {
		cfg.CreditedAmount = math.Round(cfg.CreditedAmount*100) / 100
	}
	if cfg.Ratio <= 0 && cfg.Price > 0 && cfg.CreditedAmount > 0 {
		cfg.Ratio = cfg.CreditedAmount / cfg.Price
	}
	if cfg.Ratio > 0 && math.IsNaN(cfg.Ratio) == false && math.IsInf(cfg.Ratio, 0) == false {
		cfg.Ratio = math.Round(cfg.Ratio*10000) / 10000
	}
	return cfg
}

func validateFirstRechargeGiftAmounts(cfg FirstRechargeGiftConfig) error {
	if math.IsNaN(cfg.Price) || math.IsInf(cfg.Price, 0) || cfg.Price <= 0 || cfg.Price > 1_000_000 {
		return infraerrors.BadRequest("FIRST_RECHARGE_INVALID", "first recharge gift price must be between 0 and 1000000")
	}
	if math.IsNaN(cfg.CreditedAmount) || math.IsInf(cfg.CreditedAmount, 0) || cfg.CreditedAmount <= 0 || cfg.CreditedAmount > 1_000_000_000 {
		return infraerrors.BadRequest("FIRST_RECHARGE_INVALID", "first recharge credited amount must be positive")
	}
	return nil
}

func validateFirstRechargeGiftConfig(cfg FirstRechargeGiftConfig) error {
	if err := validateFirstRechargeGiftAmounts(cfg); err != nil {
		return err
	}
	if math.IsNaN(cfg.Ratio) || math.IsInf(cfg.Ratio, 0) || cfg.Ratio <= 0 || cfg.Ratio > 1_000_000 {
		return infraerrors.BadRequest("FIRST_RECHARGE_INVALID", "first recharge gift ratio must be positive")
	}
	return nil
}

// PrepareOrder validates eligibility and clears only stale non-completed
// reservations. It intentionally performs no writes when the feature is off.
func (s *FirstRechargeGiftService) PrepareOrder(ctx context.Context, userID int64) (FirstRechargeGiftConfig, error) {
	cfg := s.Config(ctx)
	if !cfg.Enabled {
		return cfg, ErrFirstRechargeDisabled
	}
	if s == nil || s.db == nil || userID <= 0 {
		return cfg, infraerrors.ServiceUnavailable("FIRST_RECHARGE_UNAVAILABLE", "first recharge gift is unavailable")
	}
	completed, err := s.hasCompletedRecharge(ctx, s.db, userID)
	if err != nil {
		return cfg, fmt.Errorf("check first recharge eligibility: %w", err)
	}
	if completed {
		return cfg, ErrFirstRechargeAlreadyPurchased
	}
	purchase, err := s.loadPurchase(ctx, s.db, userID)
	if err != nil {
		return cfg, fmt.Errorf("load first recharge reservation: %w", err)
	}
	if purchase == nil {
		return cfg, nil
	}
	if purchase.completed {
		return cfg, ErrFirstRechargeAlreadyPurchased
	}
	if firstRechargeReservationReplaceable(purchase, time.Now()) {
		return cfg, nil
	}
	return cfg, ErrFirstRechargePending
}

// Status returns eligibility and the current reservation state. Disabled
// features return before querying the payment tables (closed-state isolation).
func (s *FirstRechargeGiftService) Status(ctx context.Context, userID int64) (*FirstRechargeGiftStatus, error) {
	cfg := s.Config(ctx)
	status := &FirstRechargeGiftStatus{
		Enabled:        cfg.Enabled,
		Price:          cfg.Price,
		CreditedAmount: cfg.CreditedAmount,
		Ratio:          cfg.Ratio,
	}
	if !cfg.Enabled {
		return status, nil
	}
	if s == nil || s.db == nil || userID <= 0 {
		return nil, infraerrors.ServiceUnavailable("FIRST_RECHARGE_UNAVAILABLE", "first recharge gift is unavailable")
	}
	completed, err := s.hasCompletedRecharge(ctx, s.db, userID)
	if err != nil {
		return nil, fmt.Errorf("check first recharge status: %w", err)
	}
	if completed {
		status.Enabled = false
		status.Purchased = true
		return status, nil
	}
	purchase, err := s.loadPurchase(ctx, s.db, userID)
	if err != nil {
		return nil, fmt.Errorf("load first recharge status: %w", err)
	}
	if purchase == nil {
		return status, nil
	}
	if purchase.completed {
		status.Enabled = false
		status.Purchased = true
		status.OrderID = purchase.orderID
		status.PurchasedAt = purchase.completedAt
		return status, nil
	}
	if firstRechargeReservationReplaceable(purchase, time.Now()) {
		return status, nil
	}
	status.Pending = true
	status.OrderID = purchase.orderID
	return status, nil
}

// ReserveTx is called inside PaymentService's existing order transaction. The
// unique user index is the final concurrency guard; a lost insert returns a
// deterministic pending error and rolls the payment order back with it.
func (s *FirstRechargeGiftService) ReserveTx(ctx context.Context, tx *dbent.Tx, userID, orderID int64, price, credited float64) error {
	if s == nil || tx == nil || userID <= 0 || orderID <= 0 {
		return infraerrors.ServiceUnavailable("FIRST_RECHARGE_UNAVAILABLE", "first recharge gift is unavailable")
	}
	// PrepareOrder runs before the payment transaction starts, so it is only a
	// hint. Re-read and lock the independent switch here, inside the same
	// transaction that creates the payment order and reservation. A concurrent
	// disable therefore either linearizes before this reservation (and causes a
	// rollback) or waits until the order transaction has committed.
	if err := requireFirstRechargeEnabledTx(ctx, tx.Client()); err != nil {
		return err
	}
	client := tx.Client()
	completed, err := s.hasCompletedRecharge(ctx, client, userID)
	if err != nil {
		return fmt.Errorf("check first recharge transaction eligibility: %w", err)
	}
	if completed {
		return ErrFirstRechargeAlreadyPurchased
	}
	// Lock the single reservation row while deciding whether a stale order can
	// be replaced. Without this lock, two requests can both observe the same
	// expired reservation and commit two payment orders; only the last marker
	// wins, leaving the first paid order unable to fulfill.
	purchase, err := s.loadPurchaseForUpdate(ctx, client, userID)
	if err != nil {
		return fmt.Errorf("load first recharge transaction reservation: %w", err)
	}
	if purchase != nil && purchase.completed {
		return ErrFirstRechargeAlreadyPurchased
	}

	var result sql.Result
	if purchase == nil {
		query := firstRechargeSQL(client, `
			INSERT INTO first_recharge_gift_purchases (user_id, order_id, price, credited_amount, status)
			VALUES (%s, %s, %s, %s, %s)
			ON CONFLICT (user_id) DO NOTHING`, 1, 2, 3, 4, 5)
		result, err = client.ExecContext(ctx, query, userID, orderID, price, credited, OrderStatusPending)
	} else if firstRechargeReservationReplaceable(purchase, time.Now()) {
		// Keep the replacement compare-and-swap even with the row lock. It
		// protects deployments that use a weaker/legacy database isolation mode
		// and makes a lost race deterministic instead of silently stealing the
		// reservation from another order.
		baseQuery := `
			UPDATE first_recharge_gift_purchases
			SET order_id = %s, price = %s, credited_amount = %s, status = %s,
				created_at = %s, completed_at = NULL
			WHERE user_id = %s
			  AND status IN (%s, %s, %s, %s)`
		args := []any{
			orderID,
			price,
			credited,
			OrderStatusPending,
			time.Now(),
			userID,
			OrderStatusPending,
			OrderStatusCancelled,
			OrderStatusExpired,
			OrderStatusFailed,
		}
		if purchase.orderID != nil {
			baseQuery += ` AND order_id = %s`
			args = append(args, *purchase.orderID)
		} else {
			baseQuery += ` AND order_id IS NULL`
		}
		positions := make([]int, len(args))
		for i := range positions {
			positions[i] = i + 1
		}
		query := firstRechargeSQL(client, baseQuery, positions...)
		result, err = client.ExecContext(ctx, query, args...)
	} else {
		return ErrFirstRechargePending
	}
	if err != nil {
		return fmt.Errorf("reserve first recharge gift: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return ErrFirstRechargePending
	}
	return nil
}

// requireFirstRechargeEnabledTx verifies the rollout switch using the
// transaction-bound Ent client. PostgreSQL keeps the settings row locked until
// the enclosing order transaction commits; SQLite is used only by unit tests
// and does not support FOR UPDATE.
func requireFirstRechargeEnabledTx(ctx context.Context, client *dbent.Client) error {
	if client == nil {
		return infraerrors.ServiceUnavailable("FIRST_RECHARGE_UNAVAILABLE", "first recharge gift is unavailable")
	}
	query := firstRechargeSQL(client, `SELECT value FROM settings WHERE key = %s`, 1)
	if client.Driver() != nil && client.Driver().Dialect() == "postgres" {
		query += " FOR UPDATE"
	}
	rows, err := client.QueryContext(ctx, query, SettingKeySubNexusFirstRechargeEnabled)
	if err != nil {
		return infraerrors.InternalServer("FIRST_RECHARGE_DB_UNAVAILABLE", "failed to verify first recharge switch")
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if rowsErr := rows.Err(); rowsErr != nil {
			return infraerrors.InternalServer("FIRST_RECHARGE_DB_UNAVAILABLE", "failed to verify first recharge switch")
		}
		return ErrFirstRechargeDisabled
	}
	var raw string
	if err := rows.Scan(&raw); err != nil {
		return infraerrors.InternalServer("FIRST_RECHARGE_DB_UNAVAILABLE", "failed to verify first recharge switch")
	}
	if raw != "true" {
		return ErrFirstRechargeDisabled
	}
	return nil
}

// ClaimForFulfillment atomically binds balance fulfillment to the one active
// reservation for the user. It keeps the historical strict behavior used by
// administrative/reconciliation callers: a replaced order returns a mismatch.
func (s *FirstRechargeGiftService) ClaimForFulfillment(ctx context.Context, userID, orderID int64) error {
	_, err := s.claimForFulfillment(ctx, userID, orderID, false)
	return err
}

// ClaimForPaymentFulfillment is used only after a provider has confirmed a
// payment. If the user's stale reservation now belongs to another order (or a
// different order already consumed the promotion), it returns a successful
// fallback decision so the paid order can be credited at its actual paid
// amount. It never converts database or transaction failures into success.
func (s *FirstRechargeGiftService) ClaimForPaymentFulfillment(ctx context.Context, userID, orderID int64) (firstRechargeFulfillmentResult, error) {
	return s.claimForFulfillment(ctx, userID, orderID, true)
}

func (s *FirstRechargeGiftService) claimForFulfillment(ctx context.Context, userID, orderID int64, allowPaidFallback bool) (firstRechargeFulfillmentResult, error) {
	result := firstRechargeFulfillmentResult{}
	if s == nil || s.db == nil || userID <= 0 || orderID <= 0 {
		return result, infraerrors.ServiceUnavailable("FIRST_RECHARGE_UNAVAILABLE", "first recharge gift is unavailable")
	}
	tx, err := s.db.Tx(ctx)
	if err != nil {
		return result, fmt.Errorf("begin first recharge fulfillment claim: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	client := tx.Client()
	purchase, err := s.loadPurchase(ctx, client, userID)
	if err != nil {
		return result, fmt.Errorf("load first recharge fulfillment reservation: %w", err)
	}
	if purchase == nil || purchase.orderID == nil || *purchase.orderID != orderID {
		if allowPaidFallback {
			result.FallbackToPaidAmount = true
			return result, nil
		}
		return result, ErrFirstRechargeReservationMismatch
	}
	if purchase.completed {
		// A completed marker for this same order means the balance may already
		// have been redeemed; replaying fulfillment remains idempotent and must
		// retain the promotional amount.
		result.GiftGranted = true
		return result, nil
	}
	if strings.EqualFold(purchase.status, OrderStatusRecharging) {
		result.GiftGranted = true
		return result, tx.Commit()
	}

	blocked, err := s.hasOtherRechargeInProgressOrCompleted(ctx, client, userID, orderID)
	if err != nil {
		return result, fmt.Errorf("check competing recharge fulfillment: %w", err)
	}
	if blocked {
		if allowPaidFallback {
			result.FallbackToPaidAmount = true
			return result, nil
		}
		return result, ErrFirstRechargeAlreadyPurchased
	}
	query := firstRechargeSQL(client, `
		UPDATE first_recharge_gift_purchases
		SET status = %s
		WHERE user_id = %s AND order_id = %s
		  AND status IN (%s, %s, %s, %s)`, 1, 2, 3, 4, 5, 6, 7)
	execResult, err := client.ExecContext(
		ctx,
		query,
		OrderStatusRecharging,
		userID,
		orderID,
		OrderStatusPending,
		OrderStatusCancelled,
		OrderStatusExpired,
		OrderStatusFailed,
	)
	if err != nil {
		return result, fmt.Errorf("claim first recharge fulfillment: %w", err)
	}
	affected, err := execResult.RowsAffected()
	if err != nil || affected != 1 {
		return result, ErrFirstRechargeReservationMismatch
	}
	if err := tx.Commit(); err != nil {
		return result, fmt.Errorf("commit first recharge fulfillment claim: %w", err)
	}
	result.GiftGranted = true
	return result, nil
}

func (s *FirstRechargeGiftService) MarkCompleted(ctx context.Context, userID, orderID int64, price, credited float64, completedAt time.Time) error {
	if s == nil || s.db == nil || userID <= 0 || orderID <= 0 {
		return nil
	}
	query := firstRechargeSQL(s.db, `
		INSERT INTO first_recharge_gift_purchases
			(user_id, order_id, price, credited_amount, status, completed_at)
		VALUES (%s, %s, %s, %s, %s, %s)
		ON CONFLICT (user_id) DO UPDATE SET
			order_id = excluded.order_id,
			price = excluded.price,
			credited_amount = excluded.credited_amount,
			status = excluded.status,
			completed_at = excluded.completed_at
		WHERE first_recharge_gift_purchases.order_id = excluded.order_id`, 1, 2, 3, 4, 5, 6)
	result, err := s.db.ExecContext(ctx, query, userID, orderID, price, credited, OrderStatusCompleted, completedAt, OrderStatusCompleted)
	if err != nil {
		return fmt.Errorf("mark first recharge gift completed: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return ErrFirstRechargeReservationMismatch
	}
	return nil
}

// Release retains the historical order binding. A later order can replace it
// with a compare-and-swap, while a late callback for the old order cannot race
// past a reservation that has already moved to the new order.
func (s *FirstRechargeGiftService) Release(ctx context.Context, orderID int64, status string) error {
	if s == nil || s.db == nil || orderID <= 0 {
		return nil
	}
	status = strings.ToUpper(strings.TrimSpace(status))
	switch status {
	case OrderStatusCancelled, OrderStatusExpired, OrderStatusFailed:
	default:
		status = OrderStatusFailed
	}
	query := firstRechargeSQL(s.db, `
		UPDATE first_recharge_gift_purchases
		SET status = %s
		WHERE order_id = %s AND status NOT IN (%s, %s)`, 1, 2, 3, 4)
	_, err := s.db.ExecContext(ctx, query, status, orderID, OrderStatusCompleted, OrderStatusRecharging)
	if err != nil {
		return fmt.Errorf("release first recharge reservation: %w", err)
	}
	return nil
}

// FirstRechargeReservationCleanupFailure describes one stale reservation that
// could not be released during reconciliation. It is kept separate from the
// query error so one bad row does not prevent later rows from being repaired.
type FirstRechargeReservationCleanupFailure struct {
	OrderID int64
	Status  string
	Err     error
}

// ReconcileStaleReservations finds terminal payment orders whose reservation
// marker was not released. Release is idempotent and compare-and-swap safe,
// so this can run on every payment-expiry cycle and alongside a manual retry.
func (s *FirstRechargeGiftService) ReconcileStaleReservations(ctx context.Context) (int, []FirstRechargeReservationCleanupFailure, error) {
	if s == nil || s.db == nil {
		return 0, nil, infraerrors.ServiceUnavailable("FIRST_RECHARGE_UNAVAILABLE", "first recharge gift is unavailable")
	}
	query := firstRechargeSQL(s.db, `
		SELECT p.order_id, UPPER(o.status), UPPER(p.status)
		FROM first_recharge_gift_purchases p
		JOIN payment_orders o ON o.id = p.order_id
		WHERE UPPER(p.status) NOT IN (%s, %s)
		  AND UPPER(o.status) IN (%s, %s, %s)
		ORDER BY p.id ASC
		LIMIT %s`,
		1, 2, 3, 4, 5, 6)
	rows, err := s.db.QueryContext(ctx, query,
		OrderStatusCompleted,
		OrderStatusRecharging,
		OrderStatusCancelled,
		OrderStatusExpired,
		OrderStatusFailed,
		firstRechargeReservationReconcileLimit,
	)
	if err != nil {
		return 0, nil, fmt.Errorf("query stale first recharge reservations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	type staleReservation struct {
		orderID     int64
		orderStatus string
	}
	candidates := make([]staleReservation, 0, firstRechargeReservationReconcileLimit)
	for rows.Next() {
		var orderID int64
		var orderStatus, reservationStatus string
		if err := rows.Scan(&orderID, &orderStatus, &reservationStatus); err != nil {
			return 0, nil, fmt.Errorf("scan stale first recharge reservation: %w", err)
		}
		candidates = append(candidates, staleReservation{orderID: orderID, orderStatus: orderStatus})
	}
	if err := rows.Err(); err != nil {
		return 0, nil, fmt.Errorf("read stale first recharge reservations: %w", err)
	}
	// Close the read cursor before issuing UPDATEs. This matters for the
	// single-connection SQLite fixtures and also reduces lock duration in
	// production PostgreSQL.
	if err := rows.Close(); err != nil {
		return 0, nil, fmt.Errorf("close stale first recharge reservations: %w", err)
	}
	cleaned := 0
	failures := make([]FirstRechargeReservationCleanupFailure, 0)
	for _, candidate := range candidates {
		if err := s.Release(ctx, candidate.orderID, candidate.orderStatus); err != nil {
			failures = append(failures, FirstRechargeReservationCleanupFailure{
				OrderID: candidate.orderID,
				Status:  candidate.orderStatus,
				Err:     err,
			})
			continue
		}
		cleaned++
	}
	return cleaned, failures, nil
}

type firstRechargePurchaseSnapshot struct {
	orderID     *int64
	status      string
	completed   bool
	completedAt *time.Time
	orderStatus string
	expiresAt   *time.Time
}

// firstRechargeFulfillmentResult tells payment fulfillment whether the
// promotional amount may be credited. FallbackToPaidAmount is deliberately
// explicit: it prevents a replaced first-recharge order from receiving the
// promotional balance twice while preserving the user's paid value.
type firstRechargeFulfillmentResult struct {
	GiftGranted          bool
	FallbackToPaidAmount bool
}

func (s *FirstRechargeGiftService) hasCompletedRecharge(ctx context.Context, client *dbent.Client, userID int64) (bool, error) {
	query := firstRechargeSQL(client, `
		SELECT EXISTS (
			SELECT 1 FROM payment_orders
			WHERE user_id = %s
			  AND status = %s
			  AND order_type IN (%s, %s, %s)
		)`, 1, 2, 3, 4, 5)
	rows, err := client.QueryContext(ctx, query, userID, OrderStatusCompleted, paymentOrderTypeBalance, paymentOrderTypeSubscription, paymentOrderTypeFirstRecharge)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return false, rows.Err()
	}
	var completed bool
	if err := rows.Scan(&completed); err != nil {
		return false, err
	}
	return completed, nil
}

func (s *FirstRechargeGiftService) loadPurchase(ctx context.Context, client *dbent.Client, userID int64) (*firstRechargePurchaseSnapshot, error) {
	return s.loadPurchaseWithLock(ctx, client, userID, false)
}

func (s *FirstRechargeGiftService) loadPurchaseForUpdate(ctx context.Context, client *dbent.Client, userID int64) (*firstRechargePurchaseSnapshot, error) {
	return s.loadPurchaseWithLock(ctx, client, userID, true)
}

func (s *FirstRechargeGiftService) loadPurchaseWithLock(ctx context.Context, client *dbent.Client, userID int64, forUpdate bool) (*firstRechargePurchaseSnapshot, error) {
	query := firstRechargeSQL(client, `
		SELECT p.order_id, p.status, p.completed_at, COALESCE(o.status, ''), o.expires_at
		FROM first_recharge_gift_purchases p
		LEFT JOIN payment_orders o ON o.id = p.order_id
		WHERE p.user_id = %s
		ORDER BY p.id DESC
		LIMIT 1`, 1)
	if forUpdate && client != nil && client.Driver() != nil && client.Driver().Dialect() == "postgres" {
		// Lock only the reservation row; PostgreSQL rejects FOR UPDATE on the
		// nullable side of the LEFT JOIN when the joined order is absent.
		query += " FOR UPDATE OF p"
	}
	var orderID sql.NullInt64
	var purchaseStatus string
	var completedAt sql.NullTime
	var orderStatus string
	var expiresAt sql.NullTime
	rows, err := client.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if stderrors.Is(rows.Err(), sql.ErrNoRows) || rows.Err() == nil {
			return nil, nil
		}
		return nil, rows.Err()
	}
	if err := rows.Scan(&orderID, &purchaseStatus, &completedAt, &orderStatus, &expiresAt); err != nil {
		return nil, err
	}
	snapshot := &firstRechargePurchaseSnapshot{
		status:      strings.ToUpper(strings.TrimSpace(purchaseStatus)),
		completed:   strings.EqualFold(strings.TrimSpace(purchaseStatus), OrderStatusCompleted) || strings.EqualFold(strings.TrimSpace(orderStatus), OrderStatusCompleted),
		orderStatus: strings.TrimSpace(orderStatus),
	}
	if orderID.Valid {
		snapshot.orderID = &orderID.Int64
	}
	if completedAt.Valid {
		value := completedAt.Time
		snapshot.completedAt = &value
	}
	if expiresAt.Valid {
		value := expiresAt.Time
		snapshot.expiresAt = &value
	}
	return snapshot, nil
}

func (s *FirstRechargeGiftService) hasOtherRechargeInProgressOrCompleted(ctx context.Context, client *dbent.Client, userID, orderID int64) (bool, error) {
	query := firstRechargeSQL(client, `
		SELECT EXISTS (
			SELECT 1 FROM payment_orders
			WHERE user_id = %s AND id <> %s
			  AND status IN (%s, %s)
			  AND order_type IN (%s, %s, %s)
		)`, 1, 2, 3, 4, 5, 6, 7)
	rows, err := client.QueryContext(
		ctx,
		query,
		userID,
		orderID,
		OrderStatusCompleted,
		OrderStatusRecharging,
		paymentOrderTypeBalance,
		paymentOrderTypeSubscription,
		paymentOrderTypeFirstRecharge,
	)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return false, rows.Err()
	}
	var blocked bool
	if err := rows.Scan(&blocked); err != nil {
		return false, err
	}
	return blocked, nil
}

func firstRechargeReservationReplaceable(snapshot *firstRechargePurchaseSnapshot, now time.Time) bool {
	if snapshot == nil || snapshot.completed || strings.EqualFold(snapshot.status, OrderStatusRecharging) {
		return false
	}
	return firstRechargeReservationStale(snapshot, now)
}

func firstRechargeReservationStale(snapshot *firstRechargePurchaseSnapshot, now time.Time) bool {
	if snapshot == nil || snapshot.completed {
		return false
	}
	switch strings.ToUpper(strings.TrimSpace(snapshot.orderStatus)) {
	case "", OrderStatusExpired, OrderStatusCancelled, OrderStatusFailed:
		return true
	}
	return snapshot.expiresAt != nil && !now.Before(*snapshot.expiresAt)
}

// firstRechargeSQL keeps the feature usable by the sqlite-backed unit tests
// while emitting PostgreSQL placeholders in production.
func firstRechargeSQL(client *dbent.Client, format string, positions ...int) string {
	if client != nil && client.Driver() != nil && client.Driver().Dialect() == "postgres" {
		args := make([]any, len(positions))
		for i, position := range positions {
			args[i] = "$" + strconv.Itoa(position)
		}
		return fmt.Sprintf(format, args...)
	}
	args := make([]any, len(positions))
	for i := range args {
		args[i] = "?"
	}
	return fmt.Sprintf(format, args...)
}

const (
	paymentOrderTypeBalance       = "balance"
	paymentOrderTypeSubscription  = "subscription"
	paymentOrderTypeFirstRecharge = "first_recharge_gift"
)
