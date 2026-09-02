package service

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

const (
	// This switch is intentionally separate from the legacy JSON payload. A
	// legacy enabled=true value can never turn this migration slice on.
	SettingKeySubNexusCheckInConfig = "SUBNEXUS_CHECKIN_CONFIG"

	CheckInCycleModeReset      = "reset"
	CheckInCycleModeCumulative = "cumulative"
)

// CheckInConfig keeps the old SubNexus admin payload names. Enabled is
// persisted for old clients, but runtime gating always uses the independent
// setting above.
type CheckInConfig struct {
	Enabled       bool    `json:"enabled"`
	IPLimit       bool    `json:"checkin_ip_limit"`
	MinAmount     float64 `json:"checkin_min"`
	MaxAmount     float64 `json:"checkin_max"`
	CycleMode     string  `json:"checkin_cycle_mode"`
	MilestoneDays int     `json:"checkin_milestone_days"`
	MilestoneMin  float64 `json:"checkin_milestone_min"`
	MilestoneMax  float64 `json:"checkin_milestone_max"`

	PaidMode        string  `json:"checkin_paid_mode"`
	FreeMaxCount    int     `json:"checkin_free_max_count"`
	FreeMaxAmount   float64 `json:"checkin_free_max_amount"`
	OverLimitAction string  `json:"checkin_over_limit_action"`
}

// UnmarshalJSON accepts both the legacy ActivityConfig names and the names
// used by the first migration prototype.
func (c *CheckInConfig) UnmarshalJSON(data []byte) error {
	type plain CheckInConfig
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw == nil {
		return errors.New("check-in config must be a JSON object")
	}
	decodeAlias := func(keys []string, dst any) error {
		for _, key := range keys {
			if value, ok := raw[key]; ok {
				if strings.TrimSpace(string(value)) == "null" {
					return fmt.Errorf("check-in config field %q cannot be null", key)
				}
				if err := json.Unmarshal(value, dst); err != nil {
					return fmt.Errorf("check-in config field %q: %w", key, err)
				}
				return nil
			}
		}
		return nil
	}
	if err := decodeAlias([]string{"enabled", "checkin_enabled"}, &decoded.Enabled); err != nil {
		return err
	}
	if err := decodeAlias([]string{"checkin_ip_limit", "ip_limit"}, &decoded.IPLimit); err != nil {
		return err
	}
	if err := decodeAlias([]string{"checkin_min", "min_amount"}, &decoded.MinAmount); err != nil {
		return err
	}
	if err := decodeAlias([]string{"checkin_max", "max_amount"}, &decoded.MaxAmount); err != nil {
		return err
	}
	if err := decodeAlias([]string{"checkin_cycle_mode", "cycle_mode"}, &decoded.CycleMode); err != nil {
		return err
	}
	if err := decodeAlias([]string{"checkin_milestone_days", "milestone_days"}, &decoded.MilestoneDays); err != nil {
		return err
	}
	if err := decodeAlias([]string{"checkin_milestone_min", "milestone_min"}, &decoded.MilestoneMin); err != nil {
		return err
	}
	if err := decodeAlias([]string{"checkin_milestone_max", "milestone_max"}, &decoded.MilestoneMax); err != nil {
		return err
	}
	if err := decodeAlias([]string{"checkin_paid_mode", "paid_mode"}, &decoded.PaidMode); err != nil {
		return err
	}
	if err := decodeAlias([]string{"checkin_free_max_count", "free_max_count"}, &decoded.FreeMaxCount); err != nil {
		return err
	}
	if err := decodeAlias([]string{"checkin_free_max_amount", "free_max_amount"}, &decoded.FreeMaxAmount); err != nil {
		return err
	}
	if err := decodeAlias([]string{"checkin_over_limit_action", "over_limit_action"}, &decoded.OverLimitAction); err != nil {
		return err
	}
	*c = CheckInConfig(decoded)
	return nil
}

type CheckInRecord struct {
	Streak    int
	LastDate  sql.NullTime
	CheckedIn bool
	Amount    float64
	CheckedAt *time.Time
}

type CheckInStatus struct {
	Enabled          bool       `json:"enabled"`
	CheckedIn        bool       `json:"checked_in"`
	Amount           float64    `json:"amount,omitempty"`
	MinAmount        float64    `json:"min_amount"`
	MaxAmount        float64    `json:"max_amount"`
	Streak           int        `json:"streak"`
	NextStreak       int        `json:"next_streak"`
	RuleStartDay     int        `json:"rule_start_day"`
	RuleEndDay       int        `json:"rule_end_day"`
	CycleMode        string     `json:"cycle_mode"`
	MilestoneDays    int        `json:"milestone_days"`
	Milestone        bool       `json:"milestone"`
	ContinuousStreak int        `json:"continuous_streak"`
	CycleDay         int        `json:"cycle_day"`
	CycleCompleted   int        `json:"cycle_completed_days"`
	CheckedAt        *time.Time `json:"checked_at,omitempty"`
	NextAt           time.Time  `json:"next_at"`

	Paid            bool    `json:"paid"`
	Locked          bool    `json:"locked"`
	LimitReached    bool    `json:"limit_reached"`
	OverLimitAction string  `json:"over_limit_action,omitempty"`
	FrozenAmount    float64 `json:"frozen_amount"`
	TodayFrozen     bool    `json:"today_frozen"`
}

// CheckInRepository remains small for lightweight callers and tests.
// Production uses the SQL path so all balance and streak writes share one
// transaction.
type CheckInRepository interface {
	Status(context.Context, int64, string) (CheckInRecord, error)
	Claim(context.Context, int64, string, time.Time, float64, string) error
}

var ErrCheckInAlreadyClaimed = errors.New("check-in already claimed")
var ErrCheckInDisabled = infraerrors.Forbidden("SUBNEXUS_CHECKIN_DISABLED", "check-in is disabled")
var ErrCheckInIPLimited = infraerrors.BadRequest("CHECKIN_IP_LIMITED", "this IP address has already claimed today's check-in")
var ErrCheckInIPRequired = infraerrors.BadRequest("CHECKIN_IP_REQUIRED", "a trusted client IP is required for check-in")

type CheckInService struct {
	repo     CheckInRepository
	settings SettingRepository
	db       *sql.DB
	auth     APIKeyAuthCacheInvalidator
	billing  *BillingCacheService
	notify   func()
}

func NewCheckInService(repo CheckInRepository, settings SettingRepository) *CheckInService {
	return &CheckInService{repo: repo, settings: settings}
}

// NewCheckInServiceWithDependencies is the production constructor. The small
// constructor above remains available to package tests and embedders.
func NewCheckInServiceWithDependencies(repo CheckInRepository, settings SettingRepository, db *sql.DB, auth APIKeyAuthCacheInvalidator, billing *BillingCacheService, settingService *SettingService) *CheckInService {
	s := NewCheckInService(repo, settings)
	s.db = db
	s.auth = auth
	s.billing = billing
	if settingService != nil {
		s.notify = settingService.NotifySettingsUpdated
	}
	return s
}

func DefaultCheckInConfig() CheckInConfig {
	return CheckInConfig{
		Enabled:         false,
		IPLimit:         false,
		MinAmount:       0.01,
		MaxAmount:       0.10,
		CycleMode:       CheckInCycleModeReset,
		MilestoneDays:   7,
		MilestoneMin:    0.10,
		MilestoneMax:    0.50,
		PaidMode:        "off",
		OverLimitAction: "prompt",
	}
}

func (s *CheckInService) strictEnabled(ctx context.Context) bool {
	if s == nil || s.settings == nil {
		return false
	}
	raw, err := s.settings.GetValue(ctx, SettingKeySubNexusCheckInEnabled)
	return err == nil && raw == "true"
}

func (s *CheckInService) Config(ctx context.Context) CheckInConfig {
	defaults := DefaultCheckInConfig()
	if s == nil || s.settings == nil {
		return defaults
	}

	// A configured switch is never enough on its own.  Require a readable,
	// valid JSON policy as well, so a partially migrated or corrupted settings
	// row cannot silently turn on reward writes with guessed defaults.
	configValid := false
	cfg := defaults
	if raw, err := s.settings.GetValue(ctx, SettingKeySubNexusCheckInConfig); err == nil && strings.TrimSpace(raw) != "" {
		var parsed CheckInConfig
		if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
			parsed = normalizeCheckInConfig(parsed)
			if err := validateCheckInConfig(parsed); err == nil {
				cfg = parsed
				configValid = true
			}
		}
	}
	// The embedded JSON bit is compatibility data only; the independent
	// setting remains the sole runtime gate and is fail-closed on read errors.
	cfg.Enabled = configValid && s.strictEnabled(ctx)
	return cfg
}

func (s *CheckInService) UpdateConfig(ctx context.Context, cfg CheckInConfig) (CheckInConfig, error) {
	if s == nil || s.settings == nil {
		return cfg, infraerrors.InternalServer("SUBNEXUS_CHECKIN_SETTINGS_UNAVAILABLE", "check-in settings repository is unavailable")
	}
	cfg = normalizeCheckInConfig(cfg)
	if err := validateCheckInConfig(cfg); err != nil {
		return cfg, err
	}
	payload, err := json.Marshal(cfg)
	if err != nil {
		return cfg, err
	}
	if err := s.settings.SetMultiple(ctx, map[string]string{
		SettingKeySubNexusCheckInEnabled: strconvBool(cfg.Enabled),
		SettingKeySubNexusCheckInConfig:  string(payload),
	}); err != nil {
		return cfg, err
	}
	if s.notify != nil {
		s.notify()
	}
	return cfg, nil
}

func strconvBool(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func normalizeCheckInConfig(cfg CheckInConfig) CheckInConfig {
	def := DefaultCheckInConfig()
	if cfg.MinAmount == 0 && cfg.MaxAmount == 0 {
		cfg.MinAmount, cfg.MaxAmount = def.MinAmount, def.MaxAmount
	}
	if cfg.MilestoneMin == 0 && cfg.MilestoneMax == 0 {
		cfg.MilestoneMin, cfg.MilestoneMax = def.MilestoneMin, def.MilestoneMax
	}
	if cfg.CycleMode == "" {
		cfg.CycleMode = def.CycleMode
	}
	if cfg.MilestoneDays == 0 {
		cfg.MilestoneDays = def.MilestoneDays
	}
	if cfg.PaidMode == "" {
		cfg.PaidMode = def.PaidMode
	}
	if cfg.OverLimitAction == "" {
		cfg.OverLimitAction = def.OverLimitAction
	}
	// Rewards and limits are represented in cents throughout the service. Keep
	// explicit negative values intact so validation can reject them instead of
	// silently converting an unsafe policy into an unlimited one.
	cfg.MinAmount = roundMoneyIfFinite(cfg.MinAmount)
	cfg.MaxAmount = roundMoneyIfFinite(cfg.MaxAmount)
	cfg.MilestoneMin = roundMoneyIfFinite(cfg.MilestoneMin)
	cfg.MilestoneMax = roundMoneyIfFinite(cfg.MilestoneMax)
	cfg.FreeMaxAmount = roundMoneyIfFinite(cfg.FreeMaxAmount)
	return cfg
}

func roundMoneyIfFinite(v float64) float64 {
	// Preserve negative inputs verbatim so a tiny value such as -0.001 cannot
	// round to signed zero and evade the non-negative policy check.
	if v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return v
	}
	return roundMoney(v)
}

func validateCheckInConfig(cfg CheckInConfig) error {
	if cfg.CycleMode != CheckInCycleModeReset && cfg.CycleMode != CheckInCycleModeCumulative {
		return infraerrors.BadRequest("CHECKIN_CYCLE_MODE_INVALID", "check-in cycle mode must be reset or cumulative")
	}
	if cfg.MilestoneDays < 1 || cfg.MilestoneDays > 15 {
		return infraerrors.BadRequest("CHECKIN_MILESTONE_DAYS_INVALID", "check-in milestone days must be between 1 and 15")
	}
	if !hasOpenCheckInRange(cfg.MinAmount, cfg.MaxAmount) {
		return infraerrors.BadRequest("CHECKIN_DAILY_RANGE_INVALID", "daily check-in range must contain a two-decimal amount strictly between its bounds")
	}
	if !hasOpenCheckInRange(cfg.MilestoneMin, cfg.MilestoneMax) {
		return infraerrors.BadRequest("CHECKIN_MILESTONE_RANGE_INVALID", "milestone check-in range must contain a two-decimal amount strictly between its bounds")
	}
	if cfg.FreeMaxCount < 0 || !finiteNonNegative(cfg.FreeMaxAmount) {
		return infraerrors.BadRequest("CHECKIN_FREE_LIMIT_INVALID", "free check-in limits must be non-negative")
	}
	switch cfg.PaidMode {
	case "off", "limit", "hide":
	default:
		return infraerrors.BadRequest("CHECKIN_PAID_MODE_INVALID", "paid mode must be off, limit, or hide")
	}
	if cfg.OverLimitAction != "prompt" && cfg.OverLimitAction != "freeze" {
		return infraerrors.BadRequest("CHECKIN_OVER_LIMIT_ACTION_INVALID", "over-limit action must be prompt or freeze")
	}
	return nil
}

func finiteNonNegative(v float64) bool {
	return v >= 0 && !math.IsNaN(v) && !math.IsInf(v, 0) && v <= 1_000_000
}

func hasOpenCheckInRange(min, max float64) bool {
	if !finiteNonNegative(min) || !finiteNonNegative(max) || max <= min {
		return false
	}
	return cents(max)-cents(min) >= 2
}

func cents(v float64) int64        { return int64(math.Round(v * 100)) }
func centsToMoney(v int64) float64 { return float64(v) / 100 }
func roundMoney(v float64) float64 { return math.Round(v*100) / 100 }

const checkInDateLayout = "2006-01-02"

func checkInDay(now time.Time) (time.Time, string, time.Time) {
	if now.IsZero() {
		now = timezone.Now()
	}
	start := timezone.StartOfDay(now)
	return start, checkInDate(start), start.AddDate(0, 0, 1)
}

// checkInDate is the single calendar-date conversion used by all streak and
// reward comparisons.  PostgreSQL DATE values are often scanned with UTC (or
// the driver's local zone), so comparing Format directly would misclassify a
// boundary-day claim when the server runs outside UTC.
func checkInDate(value time.Time) string {
	return value.In(timezone.Location()).Format(checkInDateLayout)
}

func sameCheckInDate(a, b time.Time) bool {
	return checkInDate(a) == checkInDate(b)
}

// PostgreSQL DATE has no timezone. lib/pq scans it as midnight in a fixed UTC
// location, so converting that value to a negative-offset server timezone
// would incorrectly move it to the previous calendar day. Compare its date
// components as stored against the server-local date of the request instant.
func sameStoredCheckInDate(storedDate, instant time.Time) bool {
	return storedDate.Format(checkInDateLayout) == checkInDate(instant)
}

func checkInCycleCompleted(cfg CheckInConfig, raw int) int {
	if raw <= 0 {
		return 0
	}
	days := cfg.MilestoneDays
	if days <= 0 {
		days = DefaultCheckInConfig().MilestoneDays
	}
	return raw % days
}

func (s *CheckInService) Status(ctx context.Context, userID int64, now time.Time) (*CheckInStatus, error) {
	cfg := s.Config(ctx)
	start, period, next := checkInDay(now)
	if !cfg.Enabled {
		return &CheckInStatus{Enabled: false, CycleMode: cfg.CycleMode, MilestoneDays: cfg.MilestoneDays, NextAt: next}, nil
	}
	if s == nil || (s.db == nil && s.repo == nil) {
		return nil, infraerrors.InternalServer("SUBNEXUS_CHECKIN_REPOSITORY_UNAVAILABLE", "check-in repository is unavailable")
	}
	if s.db == nil {
		// The repository-only compatibility path cannot enforce paid, frozen,
		// or IP policies atomically.  Refuse those policies instead of silently
		// bypassing them when a production dependency is miswired.
		if cfg.PaidMode != "off" || cfg.IPLimit {
			return nil, infraerrors.InternalServer("SUBNEXUS_CHECKIN_REPOSITORY_UNAVAILABLE", "check-in policy requires the SQL repository path")
		}
		rec, err := s.repo.Status(ctx, userID, period)
		if err != nil {
			return nil, err
		}
		return checkInStatusFromRecord(cfg, rec, start, next), nil
	}
	return s.statusFromDB(ctx, userID, start, period, next, cfg)
}

func checkInStatusFromRecord(cfg CheckInConfig, rec CheckInRecord, start, next time.Time) *CheckInStatus {
	base := 0
	if rec.LastDate.Valid && sameStoredCheckInDate(rec.LastDate.Time, start.AddDate(0, 0, -1)) {
		base = rec.Streak
	}
	status := makeCheckInStatus(cfg, base, next)
	if rec.CheckedIn {
		status.CheckedIn = true
		status.Amount = rec.Amount
		status.CheckedAt = rec.CheckedAt
		status.Streak = displayStreak(cfg, rec.Streak)
		status.ContinuousStreak = rec.Streak
		status.CycleDay = cycleDay(cfg, rec.Streak)
		status.CycleCompleted = status.CycleDay
	}
	return status
}

func makeCheckInStatus(cfg CheckInConfig, base int, next time.Time) *CheckInStatus {
	selection := selectCheckInReward(cfg, base+1)
	return &CheckInStatus{
		Enabled:          cfg.Enabled,
		MinAmount:        selection.MinAmount,
		MaxAmount:        selection.MaxAmount,
		NextStreak:       selection.DisplayStreak,
		RuleStartDay:     selection.DisplayStreak,
		RuleEndDay:       selection.DisplayStreak,
		CycleMode:        cfg.CycleMode,
		MilestoneDays:    cfg.MilestoneDays,
		Milestone:        selection.Milestone,
		ContinuousStreak: base,
		CycleDay:         selection.CycleDay,
		CycleCompleted:   checkInCycleCompleted(cfg, base),
		NextAt:           next,
		OverLimitAction:  cfg.OverLimitAction,
	}
}

func displayStreak(cfg CheckInConfig, raw int) int {
	if raw <= 0 {
		return 0
	}
	if cfg.CycleMode == CheckInCycleModeReset {
		return cycleDay(cfg, raw)
	}
	return raw
}

func cycleDay(cfg CheckInConfig, raw int) int {
	if raw <= 0 {
		return 0
	}
	days := cfg.MilestoneDays
	if days <= 0 {
		days = DefaultCheckInConfig().MilestoneDays
	}
	return (raw-1)%days + 1
}

type checkInSelection struct {
	RawStreak     int
	DisplayStreak int
	CycleDay      int
	MinAmount     float64
	MaxAmount     float64
	Milestone     bool
}

func selectCheckInReward(cfg CheckInConfig, raw int) checkInSelection {
	if raw <= 0 {
		raw = 1
	}
	days := cfg.MilestoneDays
	if days <= 0 {
		days = DefaultCheckInConfig().MilestoneDays
	}
	cycle := (raw-1)%days + 1
	milestone := raw%days == 0
	min, max := cfg.MinAmount, cfg.MaxAmount
	if milestone {
		min, max = cfg.MilestoneMin, cfg.MilestoneMax
	}
	display := raw
	if cfg.CycleMode == CheckInCycleModeReset {
		display = cycle
	}
	return checkInSelection{RawStreak: raw, DisplayStreak: display, CycleDay: cycle, MinAmount: min, MaxAmount: max, Milestone: milestone}
}

func (s *CheckInService) statusFromDB(ctx context.Context, userID int64, start time.Time, period string, next time.Time, cfg CheckInConfig) (*CheckInStatus, error) {
	start, period, next = checkInDay(start)
	// Resolve payment access before touching activity state.  In hide mode an
	// unpaid caller must not learn whether a historical or today's reward
	// exists, nor should a status GET perform any write as a side effect.
	var paid bool
	var frozenAmount float64
	var limitReached bool
	if cfg.PaidMode != "off" {
		var err error
		paid, err = s.isPaid(ctx, userID)
		if err != nil {
			return nil, err
		}
		if !paid && cfg.PaidMode == "hide" {
			status := makeCheckInStatus(cfg, 0, next)
			status.Paid = false
			status.Locked = true
			return status, nil
		}
		frozenAmount, err = s.frozenTotal(ctx, userID)
		if err != nil {
			return nil, err
		}
		if !paid && cfg.PaidMode == "limit" && cfg.OverLimitAction == "prompt" {
			limitReached, err = s.freeLimitReached(ctx, userID, cfg)
			if err != nil {
				return nil, err
			}
		}
	}

	streak, last, hasState, err := s.readStreak(ctx, userID)
	if err != nil {
		return nil, err
	}
	base := 0
	if hasState {
		// A pre-existing row with a NULL date is treated as an explicit reset;
		// only a state ending yesterday can continue the streak.  Legacy history
		// is recovered below only when no state row exists at all.
		if last.Valid && sameStoredCheckInDate(last.Time, start.AddDate(0, 0, -1)) {
			base = streak
		}
	} else {
		base, err = s.legacyStreakEnding(ctx, userID, start.AddDate(0, 0, -1))
		if err != nil {
			return nil, err
		}
	}
	status := makeCheckInStatus(cfg, base, next)
	status.Paid = paid
	status.FrozenAmount = frozenAmount
	status.LimitReached = limitReached

	var amount float64
	var created time.Time
	var frozen bool
	err = s.db.QueryRowContext(ctx, `SELECT amount, created_at, frozen FROM activity_reward_logs WHERE user_id=$1 AND source='checkin' AND period=$2 LIMIT 1`, userID, period).Scan(&amount, &created, &frozen)
	if errors.Is(err, sql.ErrNoRows) {
		return status, nil
	}
	if err != nil {
		return nil, err
	}
	status.CheckedIn = true
	status.Amount = amount
	status.CheckedAt = &created
	status.TodayFrozen = frozen
	todayRawStreak := base + 1
	if hasState && last.Valid && sameStoredCheckInDate(last.Time, start) && streak > 0 {
		todayRawStreak = streak
	}
	selection := selectCheckInReward(cfg, todayRawStreak)
	status.Streak = selection.DisplayStreak
	status.NextStreak = selection.DisplayStreak
	status.RuleStartDay = selection.DisplayStreak
	status.RuleEndDay = selection.DisplayStreak
	status.ContinuousStreak = selection.RawStreak
	status.CycleDay = selection.CycleDay
	status.CycleCompleted = selection.CycleDay
	status.Milestone = selection.Milestone
	status.MinAmount, status.MaxAmount = selection.MinAmount, selection.MaxAmount
	return status, nil
}

func (s *CheckInService) readStreak(ctx context.Context, userID int64) (int, sql.NullTime, bool, error) {
	var streak int
	var last sql.NullTime
	err := s.db.QueryRowContext(ctx, `SELECT current_streak, last_checkin_date FROM activity_checkin_streaks WHERE user_id=$1`, userID).Scan(&streak, &last)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, sql.NullTime{}, false, nil
	}
	return streak, last, true, err
}

func (s *CheckInService) legacyStreakEnding(ctx context.Context, userID int64, end time.Time) (int, error) {
	if s == nil || s.db == nil {
		return 0, infraerrors.InternalServer("SUBNEXUS_CHECKIN_REPOSITORY_UNAVAILABLE", "check-in SQL repository is unavailable")
	}
	end = timezone.StartOfDay(end)
	// Do not impose an arbitrary 366-day ceiling here.  Cumulative streaks can
	// span multiple years, and this path is used exactly when the new state row
	// has not been created yet.  The user/source/period index keeps the lookup
	// bounded by that user's actual history; iteration stops at the first gap.
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT period FROM activity_reward_logs WHERE user_id=$1 AND source='checkin' AND period <= $2 ORDER BY period DESC`, userID, checkInDate(end))
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	want, count := checkInDate(end), 0
	for rows.Next() {
		var period string
		if err := rows.Scan(&period); err != nil {
			return 0, err
		}
		if period != want {
			break
		}
		count++
		end = end.AddDate(0, 0, -1)
		want = checkInDate(end)
	}
	return count, rows.Err()
}

func (s *CheckInService) Claim(ctx context.Context, userID int64, clientIP string, now time.Time) (*CheckInStatus, error) {
	cfg := s.Config(ctx)
	if !cfg.Enabled {
		return nil, ErrCheckInDisabled
	}
	start, period, _ := checkInDay(now)
	clientIP = strings.TrimSpace(clientIP)
	if cfg.IPLimit && clientIP == "" {
		// The HTTP boundary is responsible for resolving a trusted client IP.
		// Missing identity must not silently bypass an enabled anti-abuse rule.
		return nil, ErrCheckInIPRequired
	}
	if s == nil || (s.db == nil && s.repo == nil) {
		return nil, infraerrors.InternalServer("SUBNEXUS_CHECKIN_REPOSITORY_UNAVAILABLE", "check-in repository is unavailable")
	}
	if s.db == nil {
		if cfg.PaidMode != "off" || cfg.IPLimit {
			return nil, infraerrors.InternalServer("SUBNEXUS_CHECKIN_REPOSITORY_UNAVAILABLE", "check-in policy requires the SQL repository path")
		}
		rec, err := s.repo.Status(ctx, userID, period)
		if err != nil {
			return nil, err
		}
		selection := selectCheckInReward(cfg, rec.Streak+1)
		amount, err := randomOpenCheckInAmount(selection.MinAmount, selection.MaxAmount)
		if err != nil {
			return nil, err
		}
		if err = s.repo.Claim(ctx, userID, period, start, amount, clientIP); err != nil {
			if errors.Is(err, ErrCheckInAlreadyClaimed) {
				return s.Status(ctx, userID, start)
			}
			return nil, err
		}
		return s.Status(ctx, userID, start)
	}
	_, err := s.claimDB(ctx, userID, clientIP, start, period, cfg)
	if errors.Is(err, ErrCheckInAlreadyClaimed) {
		// A paid user may have completed payment after claiming today's reward.
		// Keep settlement explicit on POST (rather than making Status mutate
		// state), including for an idempotent duplicate claim.
		if settleErr := s.settlePaidFrozen(ctx, userID, cfg); settleErr != nil {
			return nil, settleErr
		}
		return s.Status(ctx, userID, start)
	}
	if err != nil {
		return nil, err
	}
	return s.Status(ctx, userID, start)
}

func (s *CheckInService) settlePaidFrozen(ctx context.Context, userID int64, cfg CheckInConfig) error {
	if s == nil || s.db == nil || cfg.PaidMode == "off" {
		return nil
	}
	paid, err := s.isPaid(ctx, userID)
	if err != nil || !paid {
		return err
	}
	_, err = s.settleFrozen(ctx, userID)
	return err
}

func (s *CheckInService) claimDB(ctx context.Context, userID int64, clientIP string, day time.Time, period string, cfg CheckInConfig) (float64, error) {
	// Callers normally pass the result of checkInDay, but normalize again at
	// the transaction boundary so an internal caller cannot mix a UTC date with
	// the configured server calendar.  The canonical period is authoritative.
	day, period, _ = checkInDay(day)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	// Re-check the independent rollout switch inside the same transaction that
	// mutates the reward log, streak state, and user balance. FOR SHARE makes a
	// concurrent settings update wait for this transaction, giving the toggle
	// and the financial write a deterministic order. A missing/invalid switch
	// is fail-closed and returns before any activity write is attempted.
	if err := requireCheckInGateTx(ctx, tx); err != nil {
		return 0, err
	}
	if cfg.IPLimit && clientIP != "" {
		// Serialize the no-row case as well as existing rows.
		if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, period+":"+clientIP); err != nil {
			return 0, err
		}
		var owner int64
		err = tx.QueryRowContext(ctx, `SELECT user_id FROM activity_reward_logs WHERE source='checkin' AND period=$1 AND ip=$2 LIMIT 1`, period, clientIP).Scan(&owner)
		if err == nil && owner != userID {
			return 0, ErrCheckInIPLimited
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}
	}
	stateInsert, err := tx.ExecContext(ctx, `INSERT INTO activity_checkin_streaks(user_id) VALUES($1) ON CONFLICT DO NOTHING`, userID)
	if err != nil {
		return 0, err
	}
	insertedState, err := stateInsert.RowsAffected()
	if err != nil {
		return 0, err
	}
	var streak int
	var last sql.NullTime
	if err = tx.QueryRowContext(ctx, `SELECT current_streak,last_checkin_date FROM activity_checkin_streaks WHERE user_id=$1 FOR UPDATE`, userID).Scan(&streak, &last); err != nil {
		return 0, err
	}
	if last.Valid && last.Time.Format(checkInDateLayout) == period {
		return 0, ErrCheckInAlreadyClaimed
	}
	if insertedState > 0 {
		streak, err = legacyStreakEndingTx(ctx, tx, userID, day.AddDate(0, 0, -1))
		if err != nil {
			return 0, err
		}
	} else if !last.Valid || !sameStoredCheckInDate(last.Time, day.AddDate(0, 0, -1)) {
		streak = 0
	}
	paid := false
	settled := float64(0)
	if cfg.PaidMode != "off" {
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM payment_orders WHERE user_id=$1 AND status=$2 AND order_type IN ($3,$4,$5))`, userID, OrderStatusCompleted, "balance", "subscription", "first_recharge_gift").Scan(&paid); err != nil {
			return 0, err
		}
		if !paid && cfg.PaidMode == "hide" {
			return 0, infraerrors.BadRequest("CHECKIN_REQUIRE_PAID", "check-in requires a completed payment")
		}
		if paid {
			settled, err = settleFrozenTx(ctx, tx, userID)
			if err != nil {
				return 0, err
			}
		} else if cfg.PaidMode == "limit" {
			var count int
			var usedCents int64
			if err = tx.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(ROUND(amount * 100)),0)::bigint FROM activity_reward_logs WHERE user_id=$1 AND source='checkin' AND frozen=FALSE`, userID).Scan(&count, &usedCents); err != nil {
				return 0, err
			}
			reached := (cfg.FreeMaxCount > 0 && count >= cfg.FreeMaxCount) || (cfg.FreeMaxAmount > 0 && usedCents >= cents(cfg.FreeMaxAmount))
			if reached && cfg.OverLimitAction == "prompt" {
				return 0, infraerrors.BadRequest("CHECKIN_LIMIT_REACHED", "free check-in limit reached, recharge to unlock")
			}
			return s.insertCheckInTx(ctx, tx, userID, day, period, clientIP, cfg, streak, reached, settled)
		}
	}
	return s.insertCheckInTx(ctx, tx, userID, day, period, clientIP, cfg, streak, false, settled)
}

func requireCheckInGateTx(ctx context.Context, tx *sql.Tx) error {
	if tx == nil {
		return infraerrors.ServiceUnavailable("SUBNEXUS_CHECKIN_GATE_UNAVAILABLE", "check-in rollout gate is unavailable")
	}
	var raw string
	if err := tx.QueryRowContext(ctx,
		`SELECT value FROM settings WHERE key=$1 FOR SHARE`,
		SettingKeySubNexusCheckInEnabled,
	).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrCheckInDisabled
		}
		return infraerrors.ServiceUnavailable("SUBNEXUS_CHECKIN_GATE_UNAVAILABLE", "check-in rollout gate is unavailable").WithCause(err)
	}
	if raw != "true" {
		return ErrCheckInDisabled
	}
	return nil
}

func (s *CheckInService) insertCheckInTx(ctx context.Context, tx *sql.Tx, userID int64, day time.Time, period, clientIP string, cfg CheckInConfig, previous int, frozen bool, settled float64) (float64, error) {
	selection := selectCheckInReward(cfg, previous+1)
	amount, err := randomOpenCheckInAmount(selection.MinAmount, selection.MaxAmount)
	if err != nil {
		return 0, infraerrors.BadRequest("CHECKIN_AMOUNT_INVALID", err.Error())
	}
	note := "daily check-in"
	if frozen {
		note += " (frozen)"
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO activity_reward_logs(user_id,source,period,rank,amount,note,ip,frozen) VALUES($1,'checkin',$2,0,$3,$4,$5,$6) ON CONFLICT DO NOTHING`, userID, period, amount, note, clientIP, frozen)
	if err != nil {
		return 0, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if rows == 0 {
		return 0, ErrCheckInAlreadyClaimed
	}
	if !frozen {
		if err = creditUserTx(ctx, tx, userID, amount); err != nil {
			return 0, err
		}
	}
	stateResult, err := tx.ExecContext(ctx, `UPDATE activity_checkin_streaks SET current_streak=$1,last_checkin_date=$2,updated_at=NOW() WHERE user_id=$3`, selection.RawStreak, period, userID)
	if err != nil {
		return 0, err
	}
	stateRows, err := stateResult.RowsAffected()
	if err != nil {
		return 0, err
	}
	if stateRows == 0 {
		return 0, sql.ErrNoRows
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	if !frozen || settled > 0 {
		s.invalidateBalanceCaches(ctx, userID)
	}
	return amount, nil
}

func creditUserTx(ctx context.Context, tx *sql.Tx, userID int64, amount float64) error {
	result, err := tx.ExecContext(ctx, `UPDATE users SET balance=balance+$1,total_recharged=total_recharged+$1,updated_at=NOW() WHERE id=$2 AND deleted_at IS NULL`, amount, userID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrUserNotFound
	}
	return nil
}

func (s *CheckInService) invalidateBalanceCaches(ctx context.Context, userID int64) {
	if s.auth != nil {
		s.auth.InvalidateAuthCacheByUserID(ctx, userID)
	}
	if s.billing != nil {
		go func() {
			cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.billing.InvalidateUserBalance(cacheCtx, userID)
		}()
	}
}

func randomOpenCheckInAmount(min, max float64) (float64, error) {
	minCents, maxCents := cents(min), cents(max)
	count := maxCents - minCents - 1
	if count <= 0 {
		return 0, errors.New("check-in reward range has no two-decimal amount strictly inside it")
	}
	value, err := rand.Int(rand.Reader, big.NewInt(count))
	if err != nil {
		return 0, err
	}
	return float64(minCents+1+value.Int64()) / 100, nil
}

func legacyStreakEndingTx(ctx context.Context, tx *sql.Tx, userID int64, end time.Time) (int, error) {
	if tx == nil {
		return 0, infraerrors.InternalServer("SUBNEXUS_CHECKIN_REPOSITORY_UNAVAILABLE", "check-in transaction is unavailable")
	}
	end = timezone.StartOfDay(end)
	// Recover the complete contiguous history when adopting legacy rows.  A
	// fixed look-back (the old 366-day limit) incorrectly resets cumulative
	// streaks after a year of uninterrupted activity.
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT period FROM activity_reward_logs WHERE user_id=$1 AND source='checkin' AND period <= $2 ORDER BY period DESC`, userID, checkInDate(end))
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	want, count := checkInDate(end), 0
	for rows.Next() {
		var period string
		if err := rows.Scan(&period); err != nil {
			return 0, err
		}
		if period != want {
			break
		}
		count++
		end = end.AddDate(0, 0, -1)
		want = checkInDate(end)
	}
	return count, rows.Err()
}

func (s *CheckInService) isPaid(ctx context.Context, userID int64) (bool, error) {
	if s == nil || s.db == nil {
		return false, infraerrors.InternalServer("SUBNEXUS_CHECKIN_REPOSITORY_UNAVAILABLE", "check-in SQL repository is unavailable")
	}
	var paid bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM payment_orders WHERE user_id=$1 AND status=$2 AND order_type IN ($3,$4,$5))`, userID, OrderStatusCompleted, "balance", "subscription", "first_recharge_gift").Scan(&paid)
	return paid, err
}

func (s *CheckInService) freeLimitReached(ctx context.Context, userID int64, cfg CheckInConfig) (bool, error) {
	if cfg.FreeMaxCount <= 0 && cfg.FreeMaxAmount <= 0 {
		return false, nil
	}
	if s == nil || s.db == nil {
		return false, infraerrors.InternalServer("SUBNEXUS_CHECKIN_REPOSITORY_UNAVAILABLE", "check-in SQL repository is unavailable")
	}
	var count int
	var usedCents int64
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(ROUND(amount * 100)),0)::bigint FROM activity_reward_logs WHERE user_id=$1 AND source='checkin' AND frozen=FALSE`, userID).Scan(&count, &usedCents)
	return (cfg.FreeMaxCount > 0 && count >= cfg.FreeMaxCount) || (cfg.FreeMaxAmount > 0 && usedCents >= cents(cfg.FreeMaxAmount)), err
}

func (s *CheckInService) frozenTotal(ctx context.Context, userID int64) (float64, error) {
	if s == nil || s.db == nil {
		return 0, infraerrors.InternalServer("SUBNEXUS_CHECKIN_REPOSITORY_UNAVAILABLE", "check-in SQL repository is unavailable")
	}
	var totalCents int64
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(ROUND(amount * 100)),0)::bigint FROM activity_reward_logs WHERE user_id=$1 AND source='checkin' AND frozen=TRUE`, userID).Scan(&totalCents)
	return centsToMoney(totalCents), err
}

func settleFrozenTx(ctx context.Context, tx *sql.Tx, userID int64) (float64, error) {
	rows, err := tx.QueryContext(ctx, `UPDATE activity_reward_logs SET frozen=FALSE WHERE user_id=$1 AND source='checkin' AND frozen=TRUE RETURNING ROUND(amount * 100)::bigint`, userID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var totalCents int64
	for rows.Next() {
		var amountCents int64
		if err := rows.Scan(&amountCents); err != nil {
			return 0, err
		}
		totalCents += amountCents
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	// lib/pq keeps the command's result stream open until Rows.Close.  Close
	// it before issuing the balance UPDATE on the same transaction connection.
	if err := rows.Close(); err != nil {
		return 0, err
	}
	total := centsToMoney(totalCents)
	if total > 0 {
		if err := creditUserTx(ctx, tx, userID, total); err != nil {
			return 0, err
		}
	}
	return total, nil
}

func (s *CheckInService) settleFrozen(ctx context.Context, userID int64) (float64, error) {
	if s == nil || s.db == nil {
		return 0, infraerrors.InternalServer("SUBNEXUS_CHECKIN_REPOSITORY_UNAVAILABLE", "check-in SQL repository is unavailable")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	// This helper owns a separate transaction when called after an
	// already-claimed check-in. Re-check and lock the rollout switch before
	// moving frozen rewards into the user's balance, otherwise a concurrent
	// disable between claimDB and this follow-up transaction could still cause
	// a financial write while the feature is closed.
	if err := requireCheckInGateTx(ctx, tx); err != nil {
		return 0, err
	}
	total, err := settleFrozenTx(ctx, tx, userID)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	if total > 0 {
		s.invalidateBalanceCaches(ctx, userID)
	}
	return total, nil
}
