package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

type checkInSettingsStub struct {
	value  string
	values map[string]string
	err    error
	errs   map[string]error
}

func (s *checkInSettingsStub) GetValue(_ context.Context, key string) (string, error) {
	if s.errs != nil {
		if err := s.errs[key]; err != nil {
			return "", err
		}
	}
	if s.err != nil {
		return "", s.err
	}
	if s.values != nil {
		return s.values[key], nil
	}
	return s.value, nil
}
func (s *checkInSettingsStub) Set(_ context.Context, key, v string) error {
	if s.values == nil {
		s.values = make(map[string]string)
	}
	s.values[key] = v
	s.value = v
	return nil
}
func (*checkInSettingsStub) Get(context.Context, string) (*Setting, error) { return nil, sql.ErrNoRows }
func (*checkInSettingsStub) GetMultiple(context.Context, []string) (map[string]string, error) {
	return nil, nil
}
func (*checkInSettingsStub) SetMultiple(context.Context, map[string]string) error { return nil }
func (*checkInSettingsStub) GetAll(context.Context) (map[string]string, error)    { return nil, nil }
func (*checkInSettingsStub) Delete(context.Context, string) error                 { return nil }

type checkInRepoStub struct {
	record CheckInRecord
	claims int
}

func (r *checkInRepoStub) Status(context.Context, int64, string) (CheckInRecord, error) {
	return r.record, nil
}
func (r *checkInRepoStub) Claim(context.Context, int64, string, time.Time, float64, string) error {
	r.claims++
	r.record.CheckedIn = true
	r.record.Streak++
	return nil
}

func TestCheckInDisabledFailsClosedWithoutRepositoryAccess(t *testing.T) {
	r := &checkInRepoStub{}
	s := NewCheckInService(r, &checkInSettingsStub{})
	status, err := s.Status(context.Background(), 1, time.Now())
	if err != nil || status.Enabled || r.claims != 0 {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	if _, err = s.Claim(context.Background(), 1, "", time.Now()); err == nil {
		t.Fatal("expected disabled error")
	}
}
func TestCheckInConfigMalformedAndNonFiniteFailClosed(t *testing.T) {
	for _, raw := range []string{"{bad", "{\"enabled\":true,\"min_amount\":NaN}", "{\"enabled\":true,\"min_amount\":0.1,\"max_amount\":0.01}", "null", "{\"min_amount\":null}"} {
		cfg := NewCheckInService(nil, &checkInSettingsStub{values: map[string]string{
			SettingKeySubNexusCheckInEnabled: "true",
			SettingKeySubNexusCheckInConfig:  raw,
		}}).Config(context.Background())
		if cfg.Enabled {
			t.Fatalf("raw %q unexpectedly enabled", raw)
		}
	}
}

func TestCheckInConfigMissingOrUnreadablePolicyFailsClosed(t *testing.T) {
	for name, settings := range map[string]*checkInSettingsStub{
		"missing": {values: map[string]string{SettingKeySubNexusCheckInEnabled: "true"}},
		"config read error": {
			values: map[string]string{SettingKeySubNexusCheckInEnabled: "true"},
			errs:   map[string]error{SettingKeySubNexusCheckInConfig: errors.New("settings unavailable")},
		},
		"switch read error": {
			values: map[string]string{SettingKeySubNexusCheckInConfig: `{"min_amount":0.01,"max_amount":0.10}`},
			errs:   map[string]error{SettingKeySubNexusCheckInEnabled: errors.New("settings unavailable")},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if cfg := NewCheckInService(nil, settings).Config(context.Background()); cfg.Enabled {
				t.Fatal("expected fail-closed disabled config")
			}
		})
	}
}

func TestCheckInConfigRejectsMalformedAliasTypes(t *testing.T) {
	settings := &checkInSettingsStub{values: map[string]string{
		SettingKeySubNexusCheckInEnabled: "true",
		SettingKeySubNexusCheckInConfig:  `{"min_amount":"0.01","max_amount":0.10}`,
	}}
	if cfg := NewCheckInService(nil, settings).Config(context.Background()); cfg.Enabled {
		t.Fatal("expected malformed alias type to disable check-in")
	}
}

func TestCheckInConfigDoesNotClampNegativeFreeLimit(t *testing.T) {
	cfg := DefaultCheckInConfig()
	cfg.FreeMaxAmount = -0.01
	normalized := normalizeCheckInConfig(cfg)
	if normalized.FreeMaxAmount >= 0 {
		t.Fatalf("negative free limit was clamped: %.2f", normalized.FreeMaxAmount)
	}
	if err := validateCheckInConfig(normalized); err == nil {
		t.Fatal("expected negative free limit validation error")
	}
}

func TestCheckInConfigUsesIndependentSwitchOnlyAfterValidPolicy(t *testing.T) {
	settings := &checkInSettingsStub{values: map[string]string{
		SettingKeySubNexusCheckInEnabled: "true",
		SettingKeySubNexusCheckInConfig:  `{"enabled":false,"min_amount":0.01,"max_amount":0.10}`,
	}}
	cfg := NewCheckInService(nil, settings).Config(context.Background())
	if !cfg.Enabled {
		t.Fatal("independent switch should enable a valid policy even when embedded enabled is false")
	}
}

func TestCheckInSwitchRequiresExactTrue(t *testing.T) {
	for _, value := range []string{"TRUE", "1", " true ", "yes"} {
		settings := &checkInSettingsStub{values: map[string]string{
			SettingKeySubNexusCheckInEnabled: value,
			SettingKeySubNexusCheckInConfig:  `{"checkin_min":0.01,"checkin_max":0.10}`,
		}}
		if cfg := NewCheckInService(nil, settings).Config(context.Background()); cfg.Enabled {
			t.Fatalf("switch value %q unexpectedly enabled check-in", value)
		}
	}
}

func TestCheckInIPLimitRequiresTrustedClientIPBeforeRepositoryAccess(t *testing.T) {
	repo := &checkInRepoStub{}
	settings := &checkInSettingsStub{values: map[string]string{
		SettingKeySubNexusCheckInEnabled: "true",
		SettingKeySubNexusCheckInConfig:  `{"checkin_ip_limit":true,"checkin_min":0.01,"checkin_max":0.10}`,
	}}
	svc := NewCheckInService(repo, settings)

	if _, err := svc.Claim(context.Background(), 7, "   ", time.Now()); err != ErrCheckInIPRequired {
		t.Fatalf("Claim() error = %v, want %v", err, ErrCheckInIPRequired)
	}
	if repo.claims != 0 {
		t.Fatalf("missing trusted IP reached reward repository: claims=%d", repo.claims)
	}
}

func TestCheckInClaimIdempotentResult(t *testing.T) {
	cfg := `{"enabled":true,"min_amount":0.01,"max_amount":0.10,"milestone_days":7,"milestone_min":0.10,"milestone_max":0.50}`
	r := &checkInRepoStub{}
	s := NewCheckInService(r, &checkInSettingsStub{values: map[string]string{
		SettingKeySubNexusCheckInEnabled: "true",
		SettingKeySubNexusCheckInConfig:  cfg,
	}})
	if _, err := s.Claim(context.Background(), 1, "", time.Now()); err != nil {
		t.Fatal(err)
	}
	if r.claims != 1 {
		t.Fatalf("claims=%d", r.claims)
	}
}

func TestCheckInCycleCompletedResetsAfterMilestone(t *testing.T) {
	cfg := DefaultCheckInConfig()
	cfg.MilestoneDays = 7
	for _, tc := range []struct {
		raw  int
		want int
	}{
		{raw: 0, want: 0},
		{raw: 1, want: 1},
		{raw: 6, want: 6},
		{raw: 7, want: 0},
		{raw: 8, want: 1},
		{raw: 14, want: 0},
	} {
		if got := checkInCycleCompleted(cfg, tc.raw); got != tc.want {
			t.Errorf("raw streak %d: completed days=%d, want %d", tc.raw, got, tc.want)
		}
	}
}

func TestCheckInDateUsesConfiguredServerCalendar(t *testing.T) {
	loc := timezone.Location()
	localMidnight := time.Date(2026, 9, 2, 0, 0, 0, 0, loc)
	justBefore := localMidnight.Add(-time.Nanosecond)
	start, period, _ := checkInDay(justBefore)
	if got, want := period, localMidnight.AddDate(0, 0, -1).Format(checkInDateLayout); got != want {
		t.Fatalf("period=%q, want previous server-local date %q", got, want)
	}
	if !sameCheckInDate(start, localMidnight.AddDate(0, 0, -1).In(time.UTC)) {
		t.Fatalf("server-local date comparison failed: start=%v", start)
	}
}

func TestCheckInStatusHideUnpaidDoesNotReadActivityState(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()
	settings := &checkInSettingsStub{values: map[string]string{
		SettingKeySubNexusCheckInEnabled: "true",
		SettingKeySubNexusCheckInConfig:  `{"checkin_paid_mode":"hide","checkin_min":0.01,"checkin_max":0.10}`,
	}}
	svc := NewCheckInServiceWithDependencies(nil, settings, db, nil, nil, nil)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM payment_orders`).
		WithArgs(int64(7), OrderStatusCompleted, "balance", "subscription", "first_recharge_gift").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	status, err := svc.Status(context.Background(), 7, time.Now())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if !status.Locked || status.Paid || status.CheckedIn || status.Streak != 0 || status.FrozenAmount != 0 {
		t.Fatalf("hidden unpaid status leaked state: %+v", status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected activity query or write: %v", err)
	}
}

func TestCheckInStatusPaidIsReadOnlyAndReportsFrozenAmount(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()
	settings := &checkInSettingsStub{values: map[string]string{
		SettingKeySubNexusCheckInEnabled: "true",
		SettingKeySubNexusCheckInConfig:  `{"checkin_paid_mode":"limit","checkin_min":0.01,"checkin_max":0.10}`,
	}}
	svc := NewCheckInServiceWithDependencies(nil, settings, db, nil, nil, nil)
	now := time.Now()
	start, period, _ := checkInDay(now)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM payment_orders`).
		WithArgs(int64(8), OrderStatusCompleted, "balance", "subscription", "first_recharge_gift").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT COALESCE\(SUM\(ROUND\(amount \* 100\)\),0\)::bigint`).
		WithArgs(int64(8)).
		WillReturnRows(sqlmock.NewRows([]string{"cents"}).AddRow(int64(250)))
	mock.ExpectQuery(`SELECT current_streak, last_checkin_date`).
		WithArgs(int64(8)).
		WillReturnRows(sqlmock.NewRows([]string{"current_streak", "last_checkin_date"}).AddRow(2, start.AddDate(0, 0, -1)))
	mock.ExpectQuery(`SELECT amount, created_at, frozen`).
		WithArgs(int64(8), period).
		WillReturnError(sql.ErrNoRows)

	status, err := svc.Status(context.Background(), 8, now)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if !status.Paid || status.FrozenAmount != 2.50 || status.CheckedIn {
		t.Fatalf("paid status=%+v, want read-only frozen amount", status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected settlement write or missing read: %v", err)
	}
}

func TestLegacyStreakEndingRecoversMoreThanOneYear(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()
	svc := NewCheckInServiceWithDependencies(nil, nil, db, nil, nil, nil)
	end := timezone.StartOfDay(time.Now())
	rows := sqlmock.NewRows([]string{"period"})
	for i := 0; i < 400; i++ {
		rows.AddRow(end.AddDate(0, 0, -i).Format(checkInDateLayout))
	}
	mock.ExpectQuery(`SELECT DISTINCT period FROM activity_reward_logs`).
		WithArgs(int64(9), end.Format(checkInDateLayout)).
		WillReturnRows(rows)
	got, err := svc.legacyStreakEnding(context.Background(), 9, end)
	if err != nil {
		t.Fatalf("legacyStreakEnding() error = %v", err)
	}
	if got != 400 {
		t.Fatalf("legacy streak=%d, want 400", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestCheckInStatusRecordTreatsNullDateAsReset(t *testing.T) {
	cfg := DefaultCheckInConfig()
	rec := CheckInRecord{Streak: 9, LastDate: sql.NullTime{Valid: false}}
	start, _, next := checkInDay(time.Now())
	status := checkInStatusFromRecord(cfg, rec, start, next)
	if status.ContinuousStreak != 0 || status.CycleCompleted != 0 {
		t.Fatalf("null-date streak was resumed: %+v", status)
	}
}

func TestSettleFrozenUsesExactCentsAndCreditsOnce(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()
	svc := NewCheckInServiceWithDependencies(nil, nil, db, nil, nil, nil)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT value FROM settings WHERE key=\$1 FOR SHARE`).
		WithArgs(SettingKeySubNexusCheckInEnabled).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("true"))
	mock.ExpectQuery(`UPDATE activity_reward_logs SET frozen=FALSE`).
		WithArgs(int64(11)).
		WillReturnRows(sqlmock.NewRows([]string{"cents"}).AddRow(int64(125)).AddRow(int64(250)))
	mock.ExpectExec(`UPDATE users SET balance=balance\+\$1`).
		WithArgs(3.75, int64(11)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	total, err := svc.settleFrozen(context.Background(), 11)
	if err != nil {
		t.Fatalf("settleFrozen() error = %v", err)
	}
	if total != 3.75 {
		t.Fatalf("settled total=%.2f, want 3.75", total)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestSettleFrozenClosedGateDoesNotWrite(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()
	svc := NewCheckInServiceWithDependencies(nil, nil, db, nil, nil, nil)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT value FROM settings WHERE key=\$1 FOR SHARE`).
		WithArgs(SettingKeySubNexusCheckInEnabled).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("false"))
	mock.ExpectRollback()

	_, err = svc.settleFrozen(context.Background(), 11)
	if !errors.Is(err, ErrCheckInDisabled) {
		t.Fatalf("settleFrozen() error = %v, want %v", err, ErrCheckInDisabled)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("closed settlement performed an unexpected write: %v", err)
	}
}

func TestCheckInClaimDBRechecksRolloutGateInsideTransaction(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()
	svc := NewCheckInServiceWithDependencies(nil, nil, db, nil, nil, nil)
	cfg := DefaultCheckInConfig()
	cfg.Enabled = true
	now := timezone.StartOfDay(time.Now())
	_, period, _ := checkInDay(now)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT value FROM settings WHERE key=\$1 FOR SHARE`).
		WithArgs(SettingKeySubNexusCheckInEnabled).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("false"))
	mock.ExpectRollback()

	if _, err := svc.claimDB(context.Background(), 17, "", now, period, cfg); !errors.Is(err, ErrCheckInDisabled) {
		t.Fatalf("claimDB() error = %v, want %v", err, ErrCheckInDisabled)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("claimDB wrote activity state after gate closed: %v", err)
	}
}
