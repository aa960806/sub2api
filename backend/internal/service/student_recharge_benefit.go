package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/shopspring/decimal"
)

// StudentRechargeBenefitService owns the isolated student identity and bonus ledger.
// It intentionally does not reuse the generic activity reward path: student
// bonuses must never affect total_recharged, affiliate rebates, or activity
// eligibility.
type StudentRechargeBenefitService struct {
	db                   *sql.DB
	settingRepo          SettingRepository
	paymentConfigService *PaymentConfigService
	authCacheInvalidator APIKeyAuthCacheInvalidator
	billingCacheService  studentBenefitBalanceCache
}

// studentBenefitBalanceCache keeps the optional feature independent from the
// full billing cache contract. The service only needs to invalidate a user's
// balance after granting or reversing a bonus; requiring every quota-cache
// method would prevent the existing BillingCacheService adapter from being
// wired safely.
type studentBenefitBalanceCache interface {
	InvalidateUserBalance(context.Context, int64) error
}

// NewStudentRechargeBenefitService wires the optional student feature. All
// runtime paths remain disabled when settings are missing or malformed.
func NewStudentRechargeBenefitService(
	db *sql.DB,
	settingRepo SettingRepository,
	paymentConfigService *PaymentConfigService,
	authCacheInvalidator APIKeyAuthCacheInvalidator,
	billingCacheService studentBenefitBalanceCache,
) *StudentRechargeBenefitService {
	return &StudentRechargeBenefitService{
		db:                   db,
		settingRepo:          settingRepo,
		paymentConfigService: paymentConfigService,
		authCacheInvalidator: authCacheInvalidator,
		billingCacheService:  billingCacheService,
	}
}

const (
	// SettingKeySubNexusStudentRechargeBenefitEnabled is the independent
	// rollout switch for the migrated feature.  It is intentionally separate
	// from the legacy JSON payload so an old database value cannot enable the
	// balance-writing path during a same-database rollout.
	SettingKeySubNexusStudentRechargeBenefitEnabled = "subnexus_student_recharge_benefit_enabled"
	SettingKeyStudentRechargeBenefitConfig          = "STUDENT_RECHARGE_BENEFIT_CONFIG"
)

const (
	studentBonusStatusPending  = "pending"
	studentBonusStatusGranted  = "granted"
	studentBonusStatusReversed = "reversed"
	studentBonusStatusFailed   = "failed"
)

var errStudentBenefitGateRead = errors.New("student recharge benefit rollout gate read failed")

type StudentRechargeBenefitConfig struct {
	Enabled           bool    `json:"enabled"`
	BonusRate         float64 `json:"bonus_rate"`
	MinRechargeAmount float64 `json:"min_recharge_amount"`
	PerOrderCap       float64 `json:"per_order_cap"`
}

type StudentRechargeBenefitStatus struct {
	Enabled           bool    `json:"enabled"`
	IsStudent         bool    `json:"is_student"`
	CanUse            bool    `json:"can_use"`
	BonusRate         float64 `json:"bonus_rate"`
	MinRechargeAmount float64 `json:"min_recharge_amount"`
	PerOrderCap       float64 `json:"per_order_cap"`
}

type StudentRechargeBenefitQuote struct {
	StudentRechargeBenefitStatus
	RechargeAmount float64 `json:"recharge_amount"`
	BaseAmount     float64 `json:"base_amount"`
	BonusAmount    float64 `json:"bonus_amount"`
	TotalAmount    float64 `json:"total_amount"`
}

type StudentAccountAdminItem struct {
	UserID       int64      `json:"user_id"`
	Email        string     `json:"email"`
	Username     string     `json:"username"`
	IsStudent    bool       `json:"is_student"`
	GrantedBy    *int64     `json:"granted_by,omitempty"`
	GrantedAt    *time.Time `json:"granted_at,omitempty"`
	RevokedBy    *int64     `json:"revoked_by,omitempty"`
	RevokedAt    *time.Time `json:"revoked_at,omitempty"`
	RevokeReason string     `json:"revoke_reason"`
}

type StudentAccountAuditItem struct {
	ID                int64     `json:"id"`
	UserID            int64     `json:"user_id"`
	UserEmail         string    `json:"user_email"`
	AdminUserID       int64     `json:"admin_user_id"`
	AdminEmail        string    `json:"admin_email"`
	Action            string    `json:"action"`
	PreviousIsStudent bool      `json:"previous_is_student"`
	CurrentIsStudent  bool      `json:"current_is_student"`
	Reason            string    `json:"reason"`
	ClientIP          string    `json:"client_ip"`
	CreatedAt         time.Time `json:"created_at"`
}

type StudentRechargeBenefitSnapshot struct {
	BaseAmount     float64 `json:"base_amount"`
	BonusRate      float64 `json:"bonus_rate"`
	BonusAmount    float64 `json:"bonus_amount"`
	PerOrderCap    float64 `json:"per_order_cap"`
	RechargeAmount float64 `json:"recharge_amount"`
}

type studentBenefitSQLExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func DefaultStudentRechargeBenefitConfig() StudentRechargeBenefitConfig {
	return StudentRechargeBenefitConfig{
		Enabled:           false,
		BonusRate:         0.05,
		MinRechargeAmount: 10,
		PerOrderCap:       100,
	}
}

func (s *StudentRechargeBenefitService) SetStudentRechargeBenefitPaymentConfigService(configService *PaymentConfigService) {
	if s != nil {
		s.paymentConfigService = configService
	}
}

func normalizeStudentRechargeBenefitConfig(cfg StudentRechargeBenefitConfig) StudentRechargeBenefitConfig {
	cfg.BonusRate = roundStudentBenefitRate(cfg.BonusRate)
	cfg.MinRechargeAmount = roundStudentBenefitAmount(cfg.MinRechargeAmount)
	cfg.PerOrderCap = roundStudentBenefitAmount(cfg.PerOrderCap)
	return cfg
}

func validateStudentRechargeBenefitConfig(cfg StudentRechargeBenefitConfig) error {
	if math.IsNaN(cfg.BonusRate) || math.IsInf(cfg.BonusRate, 0) || cfg.BonusRate <= 0 || cfg.BonusRate > 10 {
		return infraerrors.BadRequest("STUDENT_BENEFIT_CONFIG_INVALID", "student bonus rate must be greater than 0 and no more than 10")
	}
	if math.IsNaN(cfg.MinRechargeAmount) || math.IsInf(cfg.MinRechargeAmount, 0) || cfg.MinRechargeAmount < 0 {
		return infraerrors.BadRequest("STUDENT_BENEFIT_CONFIG_INVALID", "student minimum recharge amount must not be negative")
	}
	if math.IsNaN(cfg.PerOrderCap) || math.IsInf(cfg.PerOrderCap, 0) || cfg.PerOrderCap < 0 {
		return infraerrors.BadRequest("STUDENT_BENEFIT_CONFIG_INVALID", "student per-order cap must not be negative")
	}
	return nil
}

func LoadStudentRechargeBenefitConfig(ctx context.Context, repo SettingRepository) (StudentRechargeBenefitConfig, error) {
	cfg := DefaultStudentRechargeBenefitConfig()
	if repo == nil {
		return cfg, nil
	}
	setting, err := repo.Get(ctx, SettingKeyStudentRechargeBenefitConfig)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return cfg, nil
		}
		return cfg, err
	}
	if setting != nil && strings.TrimSpace(setting.Value) != "" {
		var stored StudentRechargeBenefitConfig
		if err := json.Unmarshal([]byte(setting.Value), &stored); err == nil {
			stored = normalizeStudentRechargeBenefitConfig(stored)
			if validateStudentRechargeBenefitConfig(stored) == nil {
				// The legacy enabled bit is only a configuration parameter.  The
				// independent rollout key below is required as well.
				cfg = stored
			}
		}
	}
	gate, err := repo.Get(ctx, SettingKeySubNexusStudentRechargeBenefitEnabled)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			// Preserve the legacy tuning values for an administrator to review,
			// but never inherit its enabled bit without the independent rollout
			// key being present and explicitly true.
			cfg.Enabled = false
			return cfg, nil
		}
		return DefaultStudentRechargeBenefitConfig(), err
	}
	// Require the exact lowercase value and retain the legacy JSON bit as a
	// second safety condition.  Missing, malformed, or differently-cased
	// values remain disabled.
	cfg.Enabled = cfg.Enabled && gate != nil && gate.Value == "true"
	return cfg, nil
}

func (s *StudentRechargeBenefitService) GetStudentRechargeBenefitConfig(ctx context.Context) (StudentRechargeBenefitConfig, error) {
	return LoadStudentRechargeBenefitConfig(ctx, s.settingRepo)
}

// studentRechargeBenefitEnabled is a cheap gate-only read used by background
// workers.  It deliberately does not inspect the legacy JSON payload: a
// missing or malformed rollout key must stop all balance writes.
func (s *StudentRechargeBenefitService) studentRechargeBenefitEnabled(ctx context.Context) (bool, error) {
	if s == nil || s.settingRepo == nil {
		return false, nil
	}
	setting, err := s.settingRepo.Get(ctx, SettingKeySubNexusStudentRechargeBenefitEnabled)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return false, nil
		}
		return false, err
	}
	return setting != nil && setting.Value == "true", nil
}

func (s *StudentRechargeBenefitService) UpdateStudentRechargeBenefitConfig(ctx context.Context, cfg StudentRechargeBenefitConfig) (StudentRechargeBenefitConfig, error) {
	if s == nil || s.settingRepo == nil {
		return StudentRechargeBenefitConfig{}, infraerrors.InternalServer("STUDENT_BENEFIT_SETTINGS_UNAVAILABLE", "student benefit settings repository is unavailable")
	}
	cfg = normalizeStudentRechargeBenefitConfig(cfg)
	if err := validateStudentRechargeBenefitConfig(cfg); err != nil {
		return StudentRechargeBenefitConfig{}, err
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return StudentRechargeBenefitConfig{}, err
	}
	if err := s.settingRepo.SetMultiple(ctx, map[string]string{
		SettingKeyStudentRechargeBenefitConfig:          string(raw),
		SettingKeySubNexusStudentRechargeBenefitEnabled: strconv.FormatBool(cfg.Enabled),
	}); err != nil {
		return StudentRechargeBenefitConfig{}, err
	}
	return cfg, nil
}

func (s *StudentRechargeBenefitService) GetStudentRechargeBenefitStatus(ctx context.Context, userID int64) (*StudentRechargeBenefitStatus, error) {
	cfg, err := s.GetStudentRechargeBenefitConfig(ctx)
	if err != nil {
		return nil, err
	}
	status := &StudentRechargeBenefitStatus{Enabled: cfg.Enabled}
	if !cfg.Enabled {
		return status, nil
	}
	if s.db == nil || userID <= 0 {
		return nil, infraerrors.InternalServer("STUDENT_BENEFIT_UNAVAILABLE", "student benefit service is unavailable")
	}
	isStudent, err := s.isStudentAccount(ctx, userID)
	if err != nil {
		return nil, err
	}
	status.IsStudent = isStudent
	status.CanUse = isStudent
	if isStudent {
		status.BonusRate = cfg.BonusRate
		status.MinRechargeAmount = cfg.MinRechargeAmount
		status.PerOrderCap = cfg.PerOrderCap
	}
	return status, nil
}

func (s *StudentRechargeBenefitService) QuoteStudentRechargeBenefit(ctx context.Context, userID int64, rechargeAmount float64) (*StudentRechargeBenefitQuote, error) {
	status, err := s.GetStudentRechargeBenefitStatus(ctx, userID)
	if err != nil {
		return nil, err
	}
	quote := &StudentRechargeBenefitQuote{
		StudentRechargeBenefitStatus: *status,
		RechargeAmount:               roundStudentBenefitAmount(rechargeAmount),
	}
	if !status.CanUse {
		return quote, nil
	}
	if !isFinitePositive(rechargeAmount) {
		return nil, infraerrors.BadRequest("STUDENT_BENEFIT_AMOUNT_INVALID", "student recharge quote amount is invalid")
	}
	if s.paymentConfigService == nil {
		return nil, infraerrors.InternalServer("STUDENT_BENEFIT_UNAVAILABLE", "student benefit payment configuration is unavailable")
	}
	paymentCfg, err := s.paymentConfigService.GetPaymentConfig(ctx)
	if err != nil {
		return nil, err
	}
	baseAmount := calculateCreditedBalance(rechargeAmount, paymentCfg.BalanceRechargeMultiplier)
	quote.BaseAmount = roundStudentBenefitAmount(baseAmount)
	if !isFinitePositive(baseAmount) || rechargeAmount < status.MinRechargeAmount {
		return quote, nil
	}
	quote.BonusAmount = calculateStudentRechargeBonus(baseAmount, status.BonusRate, status.PerOrderCap)
	quote.TotalAmount = roundStudentBenefitAmount(baseAmount + quote.BonusAmount)
	return quote, nil
}

func (s *StudentRechargeBenefitService) PrepareStudentRechargeBenefit(ctx context.Context, userID int64, orderType string, rechargeAmount, baseAmount float64) (*StudentRechargeBenefitSnapshot, error) {
	if strings.TrimSpace(orderType) != payment.OrderTypeBalance {
		return nil, infraerrors.BadRequest("STUDENT_BENEFIT_ORDER_TYPE_INVALID", "student benefit only supports ordinary balance recharge")
	}
	cfg, err := s.GetStudentRechargeBenefitConfig(ctx)
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return nil, infraerrors.Forbidden("STUDENT_BENEFIT_DISABLED", "student recharge benefit is disabled")
	}
	if s == nil || s.db == nil || userID <= 0 {
		return nil, infraerrors.InternalServer("STUDENT_BENEFIT_UNAVAILABLE", "student benefit service is unavailable")
	}
	isStudent, err := s.isStudentAccount(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !isStudent {
		return nil, infraerrors.Forbidden("STUDENT_BENEFIT_NOT_ELIGIBLE", "student identity has not been granted")
	}
	if !isFinitePositive(rechargeAmount) || rechargeAmount < cfg.MinRechargeAmount {
		return nil, infraerrors.BadRequest("STUDENT_BENEFIT_MIN_RECHARGE", "recharge amount does not meet the student benefit minimum")
	}
	if !isFinitePositive(baseAmount) {
		return nil, infraerrors.BadRequest("STUDENT_BENEFIT_AMOUNT_INVALID", "student benefit base amount is invalid")
	}
	bonus := calculateStudentRechargeBonus(baseAmount, cfg.BonusRate, cfg.PerOrderCap)
	if bonus <= 0 {
		return nil, infraerrors.BadRequest("STUDENT_BENEFIT_AMOUNT_INVALID", "student benefit bonus amount is invalid")
	}
	return &StudentRechargeBenefitSnapshot{
		BaseAmount:     roundStudentBenefitAmount(baseAmount),
		BonusRate:      cfg.BonusRate,
		BonusAmount:    bonus,
		PerOrderCap:    cfg.PerOrderCap,
		RechargeAmount: roundStudentBenefitAmount(rechargeAmount),
	}, nil
}

func (s *StudentRechargeBenefitService) AttachStudentRechargeBenefitOrder(ctx context.Context, exec studentBenefitSQLExecutor, orderID, userID int64, snapshot *StudentRechargeBenefitSnapshot) error {
	if snapshot == nil {
		return nil
	}
	if exec == nil || orderID <= 0 || userID <= 0 {
		return infraerrors.InternalServer("STUDENT_BENEFIT_UNAVAILABLE", "student benefit order attachment is unavailable")
	}
	if err := validateStudentRechargeBenefitSnapshotInTx(ctx, exec, orderID, userID, snapshot); err != nil {
		return err
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, `
		INSERT INTO student_recharge_bonus_logs (
			payment_order_id, user_id, base_amount, bonus_rate, bonus_amount, config_snapshot, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, orderID, userID, snapshot.BaseAmount, snapshot.BonusRate, snapshot.BonusAmount, raw, studentBonusStatusPending)
	if err != nil {
		return fmt.Errorf("attach student recharge benefit: %w", err)
	}
	return nil
}

func validateStudentRechargeBenefitSnapshotInTx(ctx context.Context, exec studentBenefitSQLExecutor, orderID, userID int64, snapshot *StudentRechargeBenefitSnapshot) error {
	var orderType string
	var orderAmount float64
	rows, err := exec.QueryContext(ctx, `
		SELECT order_type, amount
		FROM payment_orders
		WHERE id = $1 AND user_id = $2
		FOR SHARE
	`, orderID, userID)
	if err != nil {
		return err
	}
	if err := scanStudentBenefitSingleRow(rows, &orderType, &orderAmount); err != nil {
		return infraerrors.InternalServer("STUDENT_BENEFIT_ORDER_INVALID", "student benefit order snapshot is invalid").WithCause(err)
	}
	if orderType != payment.OrderTypeBalance || roundStudentBenefitAmount(orderAmount) != snapshot.BaseAmount {
		return infraerrors.BadRequest("STUDENT_BENEFIT_ORDER_INVALID", "student benefit only supports the matching ordinary balance order")
	}
	if enabled, err := studentBenefitGateEnabledInExecutor(ctx, exec); err != nil {
		return err
	} else if !enabled {
		return infraerrors.Forbidden("STUDENT_BENEFIT_DISABLED", "student recharge benefit is disabled")
	}

	var rawConfig string
	rows, err = exec.QueryContext(ctx, `
		SELECT value
		FROM settings
		WHERE key = $1
		FOR SHARE
	`, SettingKeyStudentRechargeBenefitConfig)
	if err != nil {
		return err
	}
	if err := scanStudentBenefitSingleRow(rows, &rawConfig); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return infraerrors.Forbidden("STUDENT_BENEFIT_DISABLED", "student recharge benefit is disabled")
		}
		return err
	}
	var cfg StudentRechargeBenefitConfig
	if err := json.Unmarshal([]byte(rawConfig), &cfg); err != nil {
		return infraerrors.Forbidden("STUDENT_BENEFIT_DISABLED", "student recharge benefit is disabled")
	}
	cfg = normalizeStudentRechargeBenefitConfig(cfg)
	if !cfg.Enabled || validateStudentRechargeBenefitConfig(cfg) != nil {
		return infraerrors.Forbidden("STUDENT_BENEFIT_DISABLED", "student recharge benefit is disabled")
	}
	if snapshot.RechargeAmount < cfg.MinRechargeAmount || snapshot.BonusRate != cfg.BonusRate || snapshot.PerOrderCap != cfg.PerOrderCap || snapshot.BonusAmount != calculateStudentRechargeBonus(snapshot.BaseAmount, cfg.BonusRate, cfg.PerOrderCap) {
		return infraerrors.Conflict("STUDENT_BENEFIT_CONFIG_CHANGED", "student recharge benefit configuration changed; request a new quote")
	}

	var active bool
	rows, err = exec.QueryContext(ctx, `
		SELECT is_student AND revoked_at IS NULL
		FROM student_account_status
		WHERE user_id = $1
		FOR SHARE
	`, userID)
	if err != nil {
		return err
	}
	if err := scanStudentBenefitSingleRow(rows, &active); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return infraerrors.Forbidden("STUDENT_BENEFIT_NOT_ELIGIBLE", "student identity has not been granted")
		}
		return err
	}
	if !active {
		return infraerrors.Forbidden("STUDENT_BENEFIT_NOT_ELIGIBLE", "student identity has not been granted")
	}
	return nil
}

func scanStudentBenefitSingleRow(rows *sql.Rows, destinations ...any) error {
	if rows == nil {
		return sql.ErrNoRows
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return err
		}
		return sql.ErrNoRows
	}
	if err := rows.Scan(destinations...); err != nil {
		return err
	}
	return rows.Err()
}

func (s *StudentRechargeBenefitService) ListStudentAccounts(ctx context.Context, keyword string, limit int) ([]StudentAccountAdminItem, error) {
	if s == nil || s.db == nil {
		return nil, infraerrors.InternalServer("STUDENT_BENEFIT_UNAVAILABLE", "student benefit service is unavailable")
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	keyword = strings.TrimSpace(keyword)
	pattern := "%" + keyword + "%"
	rows, err := s.db.QueryContext(ctx, `
		SELECT u.id, u.email, u.username,
		       COALESCE(st.is_student, FALSE), st.granted_by, st.granted_at,
		       st.revoked_by, st.revoked_at, COALESCE(st.revoke_reason, '')
		FROM users u
		LEFT JOIN student_account_status st ON st.user_id = u.id
		WHERE u.deleted_at IS NULL
		  AND ($1 = '' OR u.email ILIKE $2 OR u.username ILIKE $2 OR CAST(u.id AS TEXT) = $1)
		ORDER BY COALESCE(st.is_student, FALSE) DESC, u.id DESC
		LIMIT $3
	`, keyword, pattern, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]StudentAccountAdminItem, 0)
	for rows.Next() {
		var item StudentAccountAdminItem
		var grantedBy, revokedBy sql.NullInt64
		var grantedAt, revokedAt sql.NullTime
		if err := rows.Scan(&item.UserID, &item.Email, &item.Username, &item.IsStudent, &grantedBy, &grantedAt, &revokedBy, &revokedAt, &item.RevokeReason); err != nil {
			return nil, err
		}
		item.GrantedBy = nullableInt64Ptr(grantedBy)
		item.RevokedBy = nullableInt64Ptr(revokedBy)
		item.GrantedAt = nullableTimePtr(grantedAt)
		item.RevokedAt = nullableTimePtr(revokedAt)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *StudentRechargeBenefitService) SetStudentAccountStatus(ctx context.Context, userID, adminUserID int64, isStudent bool, reason, clientIP string) (*StudentAccountAdminItem, error) {
	if s == nil || s.db == nil {
		return nil, infraerrors.InternalServer("STUDENT_BENEFIT_UNAVAILABLE", "student benefit service is unavailable")
	}
	if userID <= 0 || adminUserID <= 0 {
		return nil, infraerrors.BadRequest("STUDENT_ACCOUNT_INVALID", "student account target is invalid")
	}
	reason = strings.TrimSpace(reason)
	clientIP = strings.TrimSpace(clientIP)
	if len(reason) > 1000 {
		return nil, infraerrors.BadRequest("STUDENT_ACCOUNT_REASON_INVALID", "student status reason is too long")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var email, username string
	if err := tx.QueryRowContext(ctx, `SELECT email, username FROM users WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, userID).Scan(&email, &username); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, infraerrors.NotFound("USER_NOT_FOUND", "user not found")
		}
		return nil, err
	}
	var previous bool
	var hasStatus bool
	if err := tx.QueryRowContext(ctx, `SELECT is_student FROM student_account_status WHERE user_id = $1 FOR UPDATE`, userID).Scan(&previous); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	} else {
		hasStatus = true
	}
	if previous == isStudent && (hasStatus || !isStudent) {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return &StudentAccountAdminItem{UserID: userID, Email: email, Username: username, IsStudent: previous}, nil
	}

	if isStudent {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO student_account_status (
				user_id, is_student, granted_by, granted_at, revoked_by, revoked_at, revoke_reason
			) VALUES ($1, TRUE, $2, NOW(), NULL, NULL, '')
			ON CONFLICT (user_id) DO UPDATE SET
				is_student = TRUE,
				granted_by = EXCLUDED.granted_by,
				granted_at = NOW(),
				revoked_by = NULL,
				revoked_at = NULL,
				revoke_reason = '',
				updated_at = NOW()
		`, userID, adminUserID)
	} else {
		_, err = tx.ExecContext(ctx, `
			UPDATE student_account_status
			SET is_student = FALSE, revoked_by = $2, revoked_at = NOW(), revoke_reason = $3, updated_at = NOW()
			WHERE user_id = $1 AND is_student = TRUE
		`, userID, adminUserID, reason)
	}
	if err != nil {
		return nil, err
	}
	action := "revoke"
	if isStudent {
		action = "grant"
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO student_account_audit_logs (
			user_id, admin_user_id, action, previous_is_student, current_is_student, reason, client_ip
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, userID, adminUserID, action, previous, isStudent, reason, clientIP); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	items, err := s.ListStudentAccounts(ctx, fmt.Sprintf("%d", userID), 1)
	if err != nil || len(items) == 0 {
		return &StudentAccountAdminItem{UserID: userID, Email: email, Username: username, IsStudent: isStudent}, err
	}
	return &items[0], nil
}

func (s *StudentRechargeBenefitService) ListStudentAccountAuditLogs(ctx context.Context, limit int) ([]StudentAccountAuditItem, error) {
	if s == nil || s.db == nil {
		return nil, infraerrors.InternalServer("STUDENT_BENEFIT_UNAVAILABLE", "student benefit service is unavailable")
	}
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT l.id, l.user_id, u.email, l.admin_user_id, a.email, l.action,
		       l.previous_is_student, l.current_is_student, l.reason, l.client_ip, l.created_at
		FROM student_account_audit_logs l
		JOIN users u ON u.id = l.user_id
		JOIN users a ON a.id = l.admin_user_id
		ORDER BY l.id DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]StudentAccountAuditItem, 0)
	for rows.Next() {
		var item StudentAccountAuditItem
		if err := rows.Scan(&item.ID, &item.UserID, &item.UserEmail, &item.AdminUserID, &item.AdminEmail, &item.Action, &item.PreviousIsStudent, &item.CurrentIsStudent, &item.Reason, &item.ClientIP, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *StudentRechargeBenefitService) isStudentAccount(ctx context.Context, userID int64) (bool, error) {
	var active bool
	err := s.db.QueryRowContext(ctx, `
		SELECT is_student AND revoked_at IS NULL
		FROM student_account_status
		WHERE user_id = $1
	`, userID).Scan(&active)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return active, err
}

func (s *StudentRechargeBenefitService) ProcessStudentRechargeBenefits(ctx context.Context, limit int) (int, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	enabled, err := s.studentRechargeBenefitEnabled(ctx)
	if err != nil {
		return 0, err
	}
	if !enabled {
		return 0, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT b.id
		FROM student_recharge_bonus_logs b
		JOIN payment_orders o ON o.id = b.payment_order_id
		WHERE b.status IN ($1, $2)
		  AND o.status = $3
		  AND o.order_type = $4
		ORDER BY b.id
		LIMIT $5
	`, studentBonusStatusPending, studentBonusStatusFailed, OrderStatusCompleted, payment.OrderTypeBalance, limit)
	if err != nil {
		return 0, err
	}
	ids := make([]int64, 0, limit)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	processed := 0
	for _, id := range ids {
		granted, err := s.grantStudentRechargeBenefit(ctx, id)
		if err != nil {
			// Never turn an uncertain rollout-gate read into a ledger write.  A
			// later retry can safely handle the bonus once the setting store is
			// healthy again.
			if errors.Is(err, errStudentBenefitGateRead) {
				return processed, err
			}
			s.markStudentRechargeBenefitFailed(ctx, id, err)
			continue
		}
		if granted {
			processed++
		}
	}
	return processed, nil
}

func (s *StudentRechargeBenefitService) grantStudentRechargeBenefit(ctx context.Context, bonusID int64) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	var userID int64
	var amount float64
	var status, orderStatus, orderType string
	if err := tx.QueryRowContext(ctx, `
		SELECT b.user_id, b.bonus_amount, b.status, o.status, o.order_type
		FROM student_recharge_bonus_logs b
		JOIN payment_orders o ON o.id = b.payment_order_id
		WHERE b.id = $1
		FOR UPDATE OF b
	`, bonusID).Scan(&userID, &amount, &status, &orderStatus, &orderType); err != nil {
		return false, err
	}
	if enabled, err := studentBenefitGateEnabledInExecutor(ctx, tx); err != nil {
		return false, err
	} else if !enabled {
		return false, nil
	}
	if status != studentBonusStatusPending && status != studentBonusStatusFailed {
		return false, nil
	}
	if orderStatus != OrderStatusCompleted || orderType != payment.OrderTypeBalance || !isFinitePositive(amount) {
		return false, infraerrors.BadRequest("STUDENT_BENEFIT_GRANT_INVALID", "student benefit order is not eligible for grant")
	}
	res, err := tx.ExecContext(ctx, `
		UPDATE users
		SET balance = balance + $1, updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
	`, amount, userID)
	if err != nil {
		return false, err
	}
	if affected, err := res.RowsAffected(); err == nil && affected != 1 {
		return false, ErrUserNotFound
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE student_recharge_bonus_logs
		SET status = $1, granted_at = NOW(), last_error = '', updated_at = NOW()
		WHERE id = $2
	`, studentBonusStatusGranted, bonusID); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	s.invalidateStudentBenefitBalanceCaches(ctx, userID)
	return true, nil
}

func (s *StudentRechargeBenefitService) ProcessStudentRechargeRefundReversals(ctx context.Context, limit int) (int, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	enabled, err := s.studentRechargeBenefitEnabled(ctx)
	if err != nil {
		return 0, err
	}
	if !enabled {
		return 0, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT b.id
		FROM student_recharge_bonus_logs b
		JOIN payment_orders o ON o.id = b.payment_order_id
		WHERE b.status = $1
		  AND o.status IN ($2, $3)
		  AND o.refund_amount > 0
		  AND b.reversed_amount < ROUND(
		      b.bonus_amount * LEAST(1::numeric, o.refund_amount / NULLIF(b.base_amount, 0)),
		      8
		  )
		ORDER BY b.id
		LIMIT $4
	`, studentBonusStatusGranted, OrderStatusRefunded, OrderStatusPartiallyRefunded, limit)
	if err != nil {
		return 0, err
	}
	ids := make([]int64, 0, limit)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	processed := 0
	for _, id := range ids {
		changed, err := s.reverseStudentRechargeBenefit(ctx, id)
		if err == nil && changed {
			processed++
		}
	}
	return processed, nil
}

func (s *StudentRechargeBenefitService) reverseStudentRechargeBenefit(ctx context.Context, bonusID int64) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	var userID int64
	var bonusAmount, baseAmount, reversedAmount, refundAmount float64
	var orderStatus string
	if err := tx.QueryRowContext(ctx, `
		SELECT b.user_id, b.bonus_amount, b.base_amount, b.reversed_amount, o.refund_amount, o.status
		FROM student_recharge_bonus_logs b
		JOIN payment_orders o ON o.id = b.payment_order_id
		WHERE b.id = $1 AND b.status = $2
		FOR UPDATE OF b
	`, bonusID, studentBonusStatusGranted).Scan(&userID, &bonusAmount, &baseAmount, &reversedAmount, &refundAmount, &orderStatus); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if enabled, err := studentBenefitGateEnabledInExecutor(ctx, tx); err != nil {
		return false, err
	} else if !enabled {
		return false, nil
	}
	if baseAmount <= 0 || refundAmount <= 0 {
		return false, nil
	}
	desired := calculateStudentRechargeReversal(bonusAmount, refundAmount, baseAmount)
	remaining := roundStudentBenefitAmount(desired - reversedAmount)
	if remaining <= 0 {
		return false, nil
	}
	var balance float64
	if err := tx.QueryRowContext(ctx, `SELECT balance FROM users WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, userID).Scan(&balance); err != nil {
		return false, err
	}
	deduct := roundStudentBenefitAmount(math.Min(math.Max(balance, 0), remaining))
	if deduct <= 0 {
		_, _ = tx.ExecContext(ctx, `UPDATE student_recharge_bonus_logs SET last_error = $1, updated_at = NOW() WHERE id = $2`, "insufficient balance for student bonus reversal", bonusID)
		if err := tx.Commit(); err != nil {
			return false, err
		}
		return false, nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET balance = balance - $1, updated_at = NOW() WHERE id = $2`, deduct, userID); err != nil {
		return false, err
	}
	newReversed := roundStudentBenefitAmount(reversedAmount + deduct)
	newStatus := studentBonusStatusGranted
	if newReversed >= bonusAmount && orderStatus == OrderStatusRefunded {
		newStatus = studentBonusStatusReversed
	}
	lastError := ""
	if newReversed < desired {
		lastError = "insufficient balance for complete student bonus reversal"
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE student_recharge_bonus_logs
		SET status = $1, reversed_amount = $2, reversed_at = NOW(), last_error = $3, updated_at = NOW()
		WHERE id = $4
	`, newStatus, newReversed, lastError, bonusID); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	s.invalidateStudentBenefitBalanceCaches(ctx, userID)
	return true, nil
}

// studentBenefitGateEnabledInExecutor repeats the rollout check on the same
// transaction that would mutate users.balance.  This closes the small race
// where an administrator disables the feature after a worker's initial scan.
func studentBenefitGateEnabledInExecutor(ctx context.Context, exec studentBenefitSQLExecutor) (bool, error) {
	var raw string
	rows, err := exec.QueryContext(ctx, `
		SELECT value
		FROM settings
		WHERE key = $1
		FOR SHARE
	`, SettingKeySubNexusStudentRechargeBenefitEnabled)
	if err != nil {
		return false, fmt.Errorf("%w: %v", errStudentBenefitGateRead, err)
	}
	err = scanStudentBenefitSingleRow(rows, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("%w: %v", errStudentBenefitGateRead, err)
	}
	return raw == "true", nil
}

func (s *StudentRechargeBenefitService) markStudentRechargeBenefitFailed(ctx context.Context, bonusID int64, grantErr error) {
	if s == nil || s.db == nil || bonusID <= 0 || grantErr == nil {
		return
	}
	message := grantErr.Error()
	if len(message) > 1000 {
		message = message[:1000]
	}
	_, _ = s.db.ExecContext(ctx, `
		UPDATE student_recharge_bonus_logs
		SET status = $1, last_error = $2, updated_at = NOW()
		WHERE id = $3 AND status IN ($4, $5)
	`, studentBonusStatusFailed, message, bonusID, studentBonusStatusPending, studentBonusStatusFailed)
}

func (s *StudentRechargeBenefitService) invalidateStudentBenefitBalanceCaches(ctx context.Context, userID int64) {
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
	}
	if s.billingCacheService != nil {
		cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.billingCacheService.InvalidateUserBalance(cacheCtx, userID)
	}
}

func calculateStudentRechargeBonus(baseAmount, rate, cap float64) float64 {
	if !isFinitePositive(baseAmount) || !isFinitePositive(rate) || math.IsNaN(cap) || math.IsInf(cap, 0) || cap < 0 {
		return 0
	}
	bonus := decimal.NewFromFloat(baseAmount).Mul(decimal.NewFromFloat(rate)).Round(8)
	if cap > 0 {
		capValue := decimal.NewFromFloat(cap)
		if bonus.GreaterThan(capValue) {
			bonus = capValue
		}
	}
	return bonus.InexactFloat64()
}

func roundStudentBenefitAmount(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return value
	}
	return decimal.NewFromFloat(value).Round(8).InexactFloat64()
}

func roundStudentBenefitRate(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return value
	}
	return decimal.NewFromFloat(value).Round(6).InexactFloat64()
}

func calculateStudentRechargeReversal(bonusAmount, refundAmount, baseAmount float64) float64 {
	if !isFinitePositive(bonusAmount) || !isFinitePositive(refundAmount) || !isFinitePositive(baseAmount) {
		return 0
	}
	ratio := decimal.NewFromFloat(refundAmount).Div(decimal.NewFromFloat(baseAmount))
	if ratio.GreaterThan(decimal.NewFromInt(1)) {
		ratio = decimal.NewFromInt(1)
	}
	return decimal.NewFromFloat(bonusAmount).Mul(ratio).Round(8).InexactFloat64()
}

func isFinitePositive(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func nullableInt64Ptr(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	copy := value.Int64
	return &copy
}

func nullableTimePtr(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	copy := value.Time
	return &copy
}

var _ studentBenefitSQLExecutor = (*dbent.Client)(nil)
