package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

type studentBenefitSettingRepoStub struct {
	SettingRepository
	value     string
	getError  error
	setValue  string
	values    map[string]string
	setValues map[string]string
}

func (s *studentBenefitSettingRepoStub) Get(_ context.Context, key string) (*Setting, error) {
	if s.getError != nil {
		return nil, s.getError
	}
	if s.values != nil {
		if value, ok := s.values[key]; ok {
			return &Setting{Key: key, Value: value}, nil
		}
	}
	if key == SettingKeyStudentRechargeBenefitConfig && s.value != "" {
		return &Setting{Key: key, Value: s.value}, nil
	}
	return nil, ErrSettingNotFound
}

func (s *studentBenefitSettingRepoStub) Set(_ context.Context, _, value string) error {
	s.setValue = value
	return nil
}

func (s *studentBenefitSettingRepoStub) GetMultiple(_ context.Context, _ []string) (map[string]string, error) {
	return s.values, nil
}

func (s *studentBenefitSettingRepoStub) SetMultiple(_ context.Context, values map[string]string) error {
	if s.setValues == nil {
		s.setValues = make(map[string]string)
	}
	for key, value := range values {
		s.setValues[key] = value
		s.values = ensureStudentBenefitSettingValues(s.values)
		s.values[key] = value
		if key == SettingKeyStudentRechargeBenefitConfig {
			s.setValue = value
		}
	}
	return nil
}

func ensureStudentBenefitSettingValues(values map[string]string) map[string]string {
	if values == nil {
		return make(map[string]string)
	}
	return values
}

func enabledStudentBenefitRepo(t *testing.T) *studentBenefitSettingRepoStub {
	t.Helper()
	cfg := DefaultStudentRechargeBenefitConfig()
	cfg.Enabled = true
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	return &studentBenefitSettingRepoStub{
		value:  string(raw),
		values: map[string]string{SettingKeySubNexusStudentRechargeBenefitEnabled: "true"},
	}
}

func TestStudentRechargeBenefitConfigDefaultsOffAndFailsClosed(t *testing.T) {
	ctx := context.Background()

	missing, err := LoadStudentRechargeBenefitConfig(ctx, &studentBenefitSettingRepoStub{getError: ErrSettingNotFound})
	require.NoError(t, err)
	require.Equal(t, DefaultStudentRechargeBenefitConfig(), missing)
	require.False(t, missing.Enabled)

	malformed, err := LoadStudentRechargeBenefitConfig(ctx, &studentBenefitSettingRepoStub{value: `{"enabled":true}`})
	require.NoError(t, err)
	require.Equal(t, DefaultStudentRechargeBenefitConfig(), malformed)
	require.False(t, malformed.Enabled)
}

func TestStudentRechargeBenefitRequiresExactIndependentGate(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultStudentRechargeBenefitConfig()
	cfg.Enabled = true
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)

	legacyOnly := &studentBenefitSettingRepoStub{value: string(raw)}
	loaded, err := LoadStudentRechargeBenefitConfig(ctx, legacyOnly)
	require.NoError(t, err)
	require.False(t, loaded.Enabled, "legacy JSON must not implicitly enable the migrated feature")

	for _, gate := range []string{"TRUE", " true ", "1", "yes"} {
		repo := &studentBenefitSettingRepoStub{
			value:  string(raw),
			values: map[string]string{SettingKeySubNexusStudentRechargeBenefitEnabled: gate},
		}
		loaded, err := LoadStudentRechargeBenefitConfig(ctx, repo)
		require.NoError(t, err)
		require.False(t, loaded.Enabled, "gate %q must remain closed", gate)
	}

	independent := &studentBenefitSettingRepoStub{
		value:  string(raw),
		values: map[string]string{SettingKeySubNexusStudentRechargeBenefitEnabled: "true"},
	}
	loaded, err = LoadStudentRechargeBenefitConfig(ctx, independent)
	require.NoError(t, err)
	require.True(t, loaded.Enabled)
}

func TestStudentRechargeBenefitConfigValidationAndCalculation(t *testing.T) {
	cfg := DefaultStudentRechargeBenefitConfig()
	require.NoError(t, validateStudentRechargeBenefitConfig(cfg))
	require.Equal(t, 5.0, calculateStudentRechargeBonus(100, cfg.BonusRate, cfg.PerOrderCap))
	require.Equal(t, 100.0, calculateStudentRechargeBonus(5000, cfg.BonusRate, cfg.PerOrderCap))

	cfg.BonusRate = 0
	require.Error(t, validateStudentRechargeBenefitConfig(cfg))
	cfg = DefaultStudentRechargeBenefitConfig()
	cfg.MinRechargeAmount = -1
	require.Error(t, validateStudentRechargeBenefitConfig(cfg))
	cfg = DefaultStudentRechargeBenefitConfig()
	cfg.PerOrderCap = -1
	require.Error(t, validateStudentRechargeBenefitConfig(cfg))
}

func TestUpdateStudentRechargeBenefitConfigUsesIndependentSetting(t *testing.T) {
	repo := &studentBenefitSettingRepoStub{}
	svc := NewStudentRechargeBenefitService(nil, repo, nil, nil, nil)
	cfg := StudentRechargeBenefitConfig{
		Enabled:           true,
		BonusRate:         0.12345678,
		MinRechargeAmount: 12.345678901,
		PerOrderCap:       0,
	}

	updated, err := svc.UpdateStudentRechargeBenefitConfig(context.Background(), cfg)
	require.NoError(t, err)
	require.Equal(t, 0.123457, updated.BonusRate)
	require.Equal(t, 12.3456789, updated.MinRechargeAmount)
	require.NotEmpty(t, repo.setValue)

	var stored StudentRechargeBenefitConfig
	require.NoError(t, json.Unmarshal([]byte(repo.setValue), &stored))
	require.Equal(t, updated, stored)
	require.Equal(t, string(repo.setValues[SettingKeyStudentRechargeBenefitConfig]), repo.setValue)
	require.Equal(t, "true", repo.setValues[SettingKeySubNexusStudentRechargeBenefitEnabled])
}

func TestStudentRechargeBenefitScannerDoesNothingWhenGateIsClosed(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := &studentBenefitSettingRepoStub{values: map[string]string{
		SettingKeySubNexusStudentRechargeBenefitEnabled: "false",
	}}
	svc := NewStudentRechargeBenefitService(db, repo, nil, nil, nil)

	processed, err := svc.ProcessStudentRechargeBenefits(context.Background(), 10)
	require.NoError(t, err)
	require.Zero(t, processed)
	require.NoError(t, mock.ExpectationsWereMet(), "closed scanner must not query ledger or mutate balances")
}

func TestStudentRechargeBenefitGateReadFailureDoesNotWriteLedger(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT b.user_id, b.bonus_amount").WithArgs(int64(9)).WillReturnRows(
		sqlmock.NewRows([]string{"user_id", "bonus_amount", "status", "order_status", "order_type"}).
			AddRow(int64(42), 5.0, studentBonusStatusPending, OrderStatusCompleted, payment.OrderTypeBalance),
	)
	mock.ExpectQuery("SELECT value").WithArgs(SettingKeySubNexusStudentRechargeBenefitEnabled).
		WillReturnError(errors.New("settings unavailable"))
	mock.ExpectRollback()

	svc := NewStudentRechargeBenefitService(db, nil, nil, nil, nil)
	granted, err := svc.grantStudentRechargeBenefit(context.Background(), 9)
	require.Error(t, err)
	require.ErrorIs(t, err, errStudentBenefitGateRead)
	require.False(t, granted)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStudentRechargeBenefitDisabledStatusDoesNotRequireDatabase(t *testing.T) {
	repo := &studentBenefitSettingRepoStub{getError: ErrSettingNotFound}
	svc := NewStudentRechargeBenefitService(nil, repo, nil, nil, nil)

	status, err := svc.GetStudentRechargeBenefitStatus(context.Background(), 42)
	require.NoError(t, err)
	require.False(t, status.Enabled)
	require.False(t, status.IsStudent)
	require.False(t, status.CanUse)
}

func TestQuoteStudentRechargeBenefitCalculatesBaseAmountFromServerPaymentConfig(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectQuery("SELECT is_student AND revoked_at IS NULL").WithArgs(int64(42)).WillReturnRows(
		sqlmock.NewRows([]string{"active"}).AddRow(true),
	)
	repo := enabledStudentBenefitRepo(t)
	repo.values = map[string]string{
		SettingBalanceRechargeMult:                      "10",
		SettingKeySubNexusStudentRechargeBenefitEnabled: "true",
	}
	svc := NewStudentRechargeBenefitService(db, repo, nil, nil, nil)
	svc.SetStudentRechargeBenefitPaymentConfigService(NewPaymentConfigService(nil, repo, nil))

	quote, err := svc.QuoteStudentRechargeBenefit(context.Background(), 42, 100)
	require.NoError(t, err)
	require.Equal(t, 100.0, quote.RechargeAmount)
	require.Equal(t, 1000.0, quote.BaseAmount)
	require.Equal(t, 50.0, quote.BonusAmount)
	require.Equal(t, 1050.0, quote.TotalAmount)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPrepareStudentRechargeBenefitRejectsSpecialOrdersAndIneligibleUsers(t *testing.T) {
	svc := NewStudentRechargeBenefitService(nil, &studentBenefitSettingRepoStub{getError: ErrSettingNotFound}, nil, nil, nil)
	_, err := svc.PrepareStudentRechargeBenefit(context.Background(), 42, payment.OrderTypeSubscription, 100, 1000)
	require.Error(t, err)
	require.Contains(t, err.Error(), "ordinary balance recharge")

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectQuery("SELECT is_student AND revoked_at IS NULL").WithArgs(int64(42)).WillReturnError(sql.ErrNoRows)
	svc = NewStudentRechargeBenefitService(db, enabledStudentBenefitRepo(t), nil, nil, nil)
	_, err = svc.PrepareStudentRechargeBenefit(context.Background(), 42, payment.OrderTypeBalance, 100, 1000)
	require.Error(t, err)
	require.Contains(t, err.Error(), "identity has not been granted")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPrepareStudentRechargeBenefitCapturesServerSnapshot(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectQuery("SELECT is_student AND revoked_at IS NULL").WithArgs(int64(42)).WillReturnRows(
		sqlmock.NewRows([]string{"active"}).AddRow(true),
	)
	svc := NewStudentRechargeBenefitService(db, enabledStudentBenefitRepo(t), nil, nil, nil)

	snapshot, err := svc.PrepareStudentRechargeBenefit(context.Background(), 42, payment.OrderTypeBalance, 100, 1000)
	require.NoError(t, err)
	require.Equal(t, 1000.0, snapshot.BaseAmount)
	require.Equal(t, 0.05, snapshot.BonusRate)
	require.Equal(t, 50.0, snapshot.BonusAmount)
	require.Equal(t, 100.0, snapshot.RechargeAmount)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAttachStudentRechargeBenefitRevalidatesInsideOrderTransaction(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := enabledStudentBenefitRepo(t)
	mock.ExpectQuery("SELECT order_type, amount").WithArgs(int64(9), int64(42)).WillReturnRows(
		sqlmock.NewRows([]string{"order_type", "amount"}).AddRow(payment.OrderTypeBalance, 1000.0),
	)
	mock.ExpectQuery("SELECT value").WithArgs(SettingKeySubNexusStudentRechargeBenefitEnabled).WillReturnRows(
		sqlmock.NewRows([]string{"value"}).AddRow("true"),
	)
	mock.ExpectQuery("SELECT value").WithArgs(SettingKeyStudentRechargeBenefitConfig).WillReturnRows(
		sqlmock.NewRows([]string{"value"}).AddRow(repo.value),
	)
	mock.ExpectQuery("SELECT is_student AND revoked_at IS NULL").WithArgs(int64(42)).WillReturnRows(
		sqlmock.NewRows([]string{"active"}).AddRow(true),
	)
	mock.ExpectExec("INSERT INTO student_recharge_bonus_logs").WithArgs(
		int64(9), int64(42), 1000.0, 0.05, 50.0, sqlmock.AnyArg(), studentBonusStatusPending,
	).WillReturnResult(sqlmock.NewResult(1, 1))

	svc := NewStudentRechargeBenefitService(db, repo, nil, nil, nil)
	err = svc.AttachStudentRechargeBenefitOrder(context.Background(), db, 9, 42, &StudentRechargeBenefitSnapshot{
		BaseAmount:     1000,
		BonusRate:      0.05,
		BonusAmount:    50,
		PerOrderCap:    100,
		RechargeAmount: 100,
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSetStudentAccountStatusRepeatedOperationsAreIdempotent(t *testing.T) {
	tests := []struct {
		name       string
		student    bool
		statusRows *sqlmock.Rows
		statusErr  error
	}{
		{name: "repeat grant", student: true, statusRows: sqlmock.NewRows([]string{"is_student"}).AddRow(true)},
		{name: "repeat revoke without status row", student: false, statusErr: sql.ErrNoRows},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT email, username FROM users").WithArgs(int64(42)).WillReturnRows(
				sqlmock.NewRows([]string{"email", "username"}).AddRow("student@example.com", "student"),
			)
			statusQuery := mock.ExpectQuery("SELECT is_student FROM student_account_status").WithArgs(int64(42))
			if tt.statusErr != nil {
				statusQuery.WillReturnError(tt.statusErr)
			} else {
				statusQuery.WillReturnRows(tt.statusRows)
			}
			mock.ExpectCommit()

			svc := NewStudentRechargeBenefitService(db, nil, nil, nil, nil)
			item, err := svc.SetStudentAccountStatus(context.Background(), 42, 7, tt.student, "", "127.0.0.1")
			require.NoError(t, err)
			require.Equal(t, tt.student, item.IsStudent)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestGrantStudentRechargeBenefitUpdatesBalanceOnlyAndIsIdempotent(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT b.user_id, b.bonus_amount").WithArgs(int64(9)).WillReturnRows(
		sqlmock.NewRows([]string{"user_id", "bonus_amount", "status", "order_status", "order_type"}).
			AddRow(int64(42), 5.0, studentBonusStatusPending, OrderStatusCompleted, payment.OrderTypeBalance),
	)
	mock.ExpectQuery("SELECT value").WithArgs(SettingKeySubNexusStudentRechargeBenefitEnabled).WillReturnRows(
		sqlmock.NewRows([]string{"value"}).AddRow("true"),
	)
	mock.ExpectExec("UPDATE users\\s+SET balance = balance \\+ \\$1, updated_at = NOW\\(\\)").
		WithArgs(5.0, int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE student_recharge_bonus_logs").
		WithArgs(studentBonusStatusGranted, int64(9)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	svc := NewStudentRechargeBenefitService(db, nil, nil, nil, nil)
	granted, err := svc.grantStudentRechargeBenefit(context.Background(), 9)
	require.NoError(t, err)
	require.True(t, granted)
	require.NoError(t, mock.ExpectationsWereMet())

	db2, mock2, err := sqlmock.New()
	require.NoError(t, err)
	defer db2.Close()
	mock2.ExpectBegin()
	mock2.ExpectQuery("SELECT b.user_id, b.bonus_amount").WithArgs(int64(9)).WillReturnRows(
		sqlmock.NewRows([]string{"user_id", "bonus_amount", "status", "order_status", "order_type"}).
			AddRow(int64(42), 5.0, studentBonusStatusGranted, OrderStatusCompleted, payment.OrderTypeBalance),
	)
	mock2.ExpectQuery("SELECT value").WithArgs(SettingKeySubNexusStudentRechargeBenefitEnabled).WillReturnRows(
		sqlmock.NewRows([]string{"value"}).AddRow("true"),
	)
	mock2.ExpectRollback()
	svc = NewStudentRechargeBenefitService(db2, &studentBenefitSettingRepoStub{
		values: map[string]string{SettingKeySubNexusStudentRechargeBenefitEnabled: "true"},
	}, nil, nil, nil)
	granted, err = svc.grantStudentRechargeBenefit(context.Background(), 9)
	require.NoError(t, err)
	require.False(t, granted)
	require.NoError(t, mock2.ExpectationsWereMet())
}

func TestStudentRechargeReversalUsesCreditedBalanceRatio(t *testing.T) {
	require.Equal(t, 25.0, calculateStudentRechargeReversal(50, 500, 1000))
	require.Equal(t, 50.0, calculateStudentRechargeReversal(50, 1200, 1000))
	require.Zero(t, calculateStudentRechargeReversal(50, 100, 0))
}

func TestBuildWeChatPaymentOAuthStartURLPreservesStudentBenefit(t *testing.T) {
	raw, err := buildWeChatPaymentOAuthStartURL(CreateOrderRequest{
		PaymentType:    payment.TypeWxpay,
		Amount:         100,
		OrderType:      payment.OrderTypeBalance,
		StudentBenefit: true,
	}, "snsapi_base")
	require.NoError(t, err)
	parsed, err := url.Parse(raw)
	require.NoError(t, err)
	require.Equal(t, "true", parsed.Query().Get("student_benefit"))
}
