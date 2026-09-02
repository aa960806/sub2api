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

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

const (
	// SettingKeySubNexusLeaderboardConfig stores only the display/reward policy.
	// It is kept separate from the legacy activity JSON so an old binary cannot
	// accidentally enable this migration slice during a rollback.
	SettingKeySubNexusLeaderboardConfig = "SUBNEXUS_LEADERBOARD_CONFIG"

	LeaderboardWindowToday = "today"
	LeaderboardWindowWeek  = "week"
	LeaderboardWindowMonth = "month"

	ActivitySourceLeaderboardWeek  = "leaderboard_week"
	ActivitySourceLeaderboardMonth = "leaderboard_month"
	leaderboardMaxRewardCents      = int64(100_000_000_000) // 1 billion currency units
)

var (
	// ErrLeaderboardDisabled is returned before any usage/affiliate query when
	// the independent rollout switch is off.
	ErrLeaderboardDisabled = infraerrors.Forbidden("LEADERBOARD_DISABLED", "leaderboard is disabled")
	// Alias retained for callers that use the migration-prefixed name.
	ErrSubNexusLeaderboardDisabled = ErrLeaderboardDisabled
	ErrLeaderboardRewardDisabled   = infraerrors.BadRequest("LEADERBOARD_REWARD_DISABLED", "leaderboard reward is disabled")
)

// LeaderboardConfig contains the migrated leaderboard policy. Enabled is kept
// in the JSON payload for compatibility, while the independent setting key is
// the runtime gate.
type LeaderboardConfig struct {
	Enabled        bool      `json:"enabled"`
	WeeklyEnabled  bool      `json:"weekly_enabled"`
	WeeklyTopN     int       `json:"weekly_top_n"`
	WeeklyReward   float64   `json:"weekly_reward"`
	WeeklyRewards  []float64 `json:"weekly_rewards"`
	MonthlyEnabled bool      `json:"monthly_enabled"`
	MonthlyTopN    int       `json:"monthly_top_n"`
	MonthlyReward  float64   `json:"monthly_reward"`
	MonthlyRewards []float64 `json:"monthly_rewards"`
}

type LeaderboardEntry struct {
	Rank         int     `json:"rank"`
	UserID       int64   `json:"user_id"`
	Email        string  `json:"email"`
	Usage        float64 `json:"usage"`
	Requests     int64   `json:"requests"`
	Tokens       int64   `json:"tokens"`
	RewardAmount float64 `json:"reward_amount,omitempty"`
	Rewarded     bool    `json:"rewarded"`
}

type LeaderboardResponse struct {
	Window        string             `json:"window"`
	Title         string             `json:"title"`
	StartDate     string             `json:"start_date"`
	EndDate       string             `json:"end_date"`
	TotalUsage    float64            `json:"total_usage"`
	Requests      int64              `json:"requests"`
	Tokens        int64              `json:"tokens"`
	RewardTopN    int                `json:"reward_top_n"`
	RewardValue   float64            `json:"reward_value"`
	RewardAmounts []float64          `json:"reward_amounts"`
	Entries       []LeaderboardEntry `json:"entries"`
}

// InviteLeaderboardEntry and InviteLeaderboardResponse are display-only
// structures. The invite board never grants or exposes reward metadata.
type InviteLeaderboardEntry struct {
	Rank        int    `json:"rank"`
	UserID      int64  `json:"user_id"`
	Email       string `json:"email"`
	InviteCount int64  `json:"invite_count"`
}

type InviteLeaderboardResponse struct {
	Window       string                   `json:"window"`
	Title        string                   `json:"title"`
	StartDate    string                   `json:"start_date"`
	EndDate      string                   `json:"end_date"`
	TotalInvites int64                    `json:"total_invites"`
	Entries      []InviteLeaderboardEntry `json:"entries"`
}

// LeaderboardRepository serves display reads. Implementations must not update
// usage, affiliate, balance, or reward tables while serving a board.
type LeaderboardRepository interface {
	GetLeaderboard(context.Context, time.Time, time.Time, int, string, string) ([]LeaderboardEntry, error)
	GetInviteLeaderboard(context.Context, time.Time, time.Time, int) ([]InviteLeaderboardEntry, error)
}

// LeaderboardRewardRepository is an optional production extension. Keeping it
// separate from LeaderboardRepository preserves compatibility with existing
// read-only test stubs and embedders.
type LeaderboardRewardRepository interface {
	GetLeaderboardTx(context.Context, *sql.Tx, time.Time, time.Time, int, string, string) ([]LeaderboardEntry, error)
}

// LeaderboardBalanceCache is deliberately narrow so reward settlement can be
// tested without constructing the complete billing cache service.
type LeaderboardBalanceCache interface {
	InvalidateUserBalance(context.Context, int64) error
}

type LeaderboardService struct {
	repo            LeaderboardRepository
	settings        SettingRepository
	settingsUpdated func()
	db              *sql.DB
	auth            APIKeyAuthCacheInvalidator
	billing         LeaderboardBalanceCache
}

func NewLeaderboardService(repo LeaderboardRepository, settings SettingRepository) *LeaderboardService {
	return &LeaderboardService{repo: repo, settings: settings}
}

// NewLeaderboardServiceWithDependencies is the production constructor. The
// small constructor above remains available for display-only callers/tests.
func NewLeaderboardServiceWithDependencies(
	repo LeaderboardRepository,
	settings SettingRepository,
	db *sql.DB,
	auth APIKeyAuthCacheInvalidator,
	billing LeaderboardBalanceCache,
	settingService *SettingService,
) *LeaderboardService {
	svc := NewLeaderboardService(repo, settings)
	svc.db = db
	svc.auth = auth
	svc.billing = billing
	if settingService != nil {
		svc.settingsUpdated = settingService.NotifySettingsUpdated
	}
	return svc
}

// SetRewardDependencies allows an application assembled without Wire to opt
// into settlement while preserving the historical constructor signature.
func (s *LeaderboardService) SetRewardDependencies(db *sql.DB, auth APIKeyAuthCacheInvalidator, billing LeaderboardBalanceCache) {
	if s == nil {
		return
	}
	s.db, s.auth, s.billing = db, auth, billing
}

// SetSettingsUpdatedNotifier connects feature-specific config writes to the
// process-wide public-settings/embedded-frontend cache invalidation hook.
func (s *LeaderboardService) SetSettingsUpdatedNotifier(notifier func()) {
	if s != nil {
		s.settingsUpdated = notifier
	}
}

func DefaultLeaderboardConfig() LeaderboardConfig {
	return LeaderboardConfig{
		Enabled:        false,
		WeeklyEnabled:  false,
		WeeklyTopN:     3,
		WeeklyReward:   1,
		WeeklyRewards:  []float64{1, 1, 1},
		MonthlyEnabled: false,
		MonthlyTopN:    3,
		MonthlyReward:  5,
		MonthlyRewards: []float64{5, 5, 5},
	}
}

// strictEnabled intentionally accepts only the literal lowercase "true".
// This keeps malformed or legacy values fail-closed during a staged rollout.
func (s *LeaderboardService) strictEnabled(ctx context.Context) bool {
	if s == nil || s.settings == nil {
		return false
	}
	raw, err := s.settings.GetValue(ctx, SettingKeySubNexusLeaderboardEnabled)
	return err == nil && raw == "true"
}

// Config returns the effective policy. The independent switch is the only
// runtime gate; the embedded `enabled` JSON field is compatibility data.
func (s *LeaderboardService) Config(ctx context.Context) LeaderboardConfig {
	defaults := DefaultLeaderboardConfig()
	if s == nil || s.settings == nil {
		return defaults
	}
	raw, err := s.settings.GetValue(ctx, SettingKeySubNexusLeaderboardConfig)
	if err != nil || strings.TrimSpace(raw) == "" {
		return defaults
	}
	trimmed := strings.TrimSpace(raw)
	// A JSON null/object mismatch must not silently turn into the normalized
	// defaults. Treat only an object payload as a valid persisted policy.
	if trimmed == "null" || !strings.HasPrefix(trimmed, "{") {
		return defaults
	}
	var cfg LeaderboardConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return defaults
	}
	cfg = normalizeLeaderboardConfig(cfg)
	if err := validateLeaderboardConfig(cfg); err != nil {
		return defaults
	}
	cfg.Enabled = s.strictEnabled(ctx)
	return cfg
}

func (s *LeaderboardService) UpdateConfig(ctx context.Context, cfg LeaderboardConfig) (LeaderboardConfig, error) {
	if s == nil || s.settings == nil {
		return cfg, infraerrors.InternalServer(
			"SUBNEXUS_LEADERBOARD_SETTINGS_UNAVAILABLE",
			"leaderboard settings repository is unavailable",
		)
	}
	cfg = normalizeLeaderboardConfig(cfg)
	if err := validateLeaderboardConfig(cfg); err != nil {
		return cfg, err
	}
	payload, err := json.Marshal(cfg)
	if err != nil {
		return cfg, err
	}
	if err := s.settings.SetMultiple(ctx, map[string]string{
		SettingKeySubNexusLeaderboardEnabled: strconv.FormatBool(cfg.Enabled),
		SettingKeySubNexusLeaderboardConfig:  string(payload),
	}); err != nil {
		return cfg, err
	}
	if s.settingsUpdated != nil {
		s.settingsUpdated()
	}
	return cfg, nil
}

func normalizeLeaderboardConfig(cfg LeaderboardConfig) LeaderboardConfig {
	def := DefaultLeaderboardConfig()
	if cfg.WeeklyTopN <= 0 {
		cfg.WeeklyTopN = def.WeeklyTopN
	}
	if cfg.WeeklyTopN > 100 {
		cfg.WeeklyTopN = 100
	}
	if cfg.MonthlyTopN <= 0 {
		cfg.MonthlyTopN = def.MonthlyTopN
	}
	if cfg.MonthlyTopN > 100 {
		cfg.MonthlyTopN = 100
	}
	if cfg.WeeklyReward == 0 && len(cfg.WeeklyRewards) == 0 {
		cfg.WeeklyReward = def.WeeklyReward
	}
	if cfg.MonthlyReward == 0 && len(cfg.MonthlyRewards) == 0 {
		cfg.MonthlyReward = def.MonthlyReward
	}
	if cfg.WeeklyReward < 0 {
		cfg.WeeklyReward = 0
	}
	if cfg.MonthlyReward < 0 {
		cfg.MonthlyReward = 0
	}
	cfg.WeeklyRewards = normalizeRewardAmounts(cfg.WeeklyRewards, cfg.WeeklyTopN, cfg.WeeklyReward)
	cfg.MonthlyRewards = normalizeRewardAmounts(cfg.MonthlyRewards, cfg.MonthlyTopN, cfg.MonthlyReward)
	if len(cfg.WeeklyRewards) > 0 {
		cfg.WeeklyReward = cfg.WeeklyRewards[0]
	}
	if len(cfg.MonthlyRewards) > 0 {
		cfg.MonthlyReward = cfg.MonthlyRewards[0]
	}
	return cfg
}

func normalizeRewardAmounts(values []float64, topN int, fallback float64) []float64 {
	if topN <= 0 {
		return nil
	}
	if fallback < 0 || math.IsNaN(fallback) || math.IsInf(fallback, 0) {
		fallback = 0
	}
	out := make([]float64, topN)
	for i := range out {
		value := fallback
		if i < len(values) {
			value = values[i]
		}
		if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			value = 0
		}
		out[i] = roundLeaderboardMoney(value)
	}
	return out
}

func validateLeaderboardConfig(cfg LeaderboardConfig) error {
	if cfg.WeeklyTopN < 1 || cfg.WeeklyTopN > 100 || cfg.MonthlyTopN < 1 || cfg.MonthlyTopN > 100 {
		return infraerrors.BadRequest("LEADERBOARD_TOP_N_INVALID", "leaderboard top n must be between 1 and 100")
	}
	for _, values := range [][]float64{cfg.WeeklyRewards, cfg.MonthlyRewards} {
		for _, value := range values {
			if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
				return infraerrors.BadRequest("LEADERBOARD_REWARD_INVALID", "leaderboard rewards must be finite and non-negative")
			}
		}
	}
	return nil
}

func roundLeaderboardMoney(value float64) float64 {
	cents, err := leaderboardMoneyCents(value)
	if err != nil {
		return 0
	}
	return float64(cents) / 100
}

func (s *LeaderboardService) GetLeaderboard(ctx context.Context, window string, limit int, now time.Time) (*LeaderboardResponse, error) {
	cfg := s.Config(ctx)
	if !cfg.Enabled {
		return nil, ErrLeaderboardDisabled
	}
	if s.repo == nil {
		return nil, infraerrors.InternalServer("SUBNEXUS_LEADERBOARD_REPOSITORY_UNAVAILABLE", "leaderboard repository is unavailable")
	}
	window = normalizeLeaderboardWindow(window)
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	start, end, title, err := resolveLeaderboardWindow(window, now)
	if err != nil {
		return nil, err
	}
	rewardTopN, rewardAmounts, source := rewardSettingsForLeaderboardWindow(window, cfg)
	if window == LeaderboardWindowToday {
		rewardTopN = 0
		rewardAmounts = nil
		source = ""
	}
	entries, err := s.repo.GetLeaderboard(ctx, start, end, limit, source, leaderboardPeriodKey(window, start))
	if err != nil {
		return nil, err
	}
	if entries == nil {
		entries = []LeaderboardEntry{}
	}
	resp := &LeaderboardResponse{
		Window:        window,
		Title:         title,
		StartDate:     start.Format("2006-01-02"),
		EndDate:       end.Add(-time.Nanosecond).Format("2006-01-02"),
		RewardTopN:    rewardTopN,
		RewardValue:   firstLeaderboardReward(rewardAmounts),
		RewardAmounts: append([]float64(nil), rewardAmounts...),
		Entries:       make([]LeaderboardEntry, 0, len(entries)),
	}
	for i := range entries {
		entry := entries[i]
		if entry.Rank <= 0 {
			entry.Rank = i + 1
		}
		entry.Email = maskEmail(entry.Email)
		if entry.Rank <= rewardTopN && entry.Rank <= len(rewardAmounts) {
			entry.RewardAmount = rewardAmounts[entry.Rank-1]
		}
		resp.TotalUsage += entry.Usage
		resp.Requests += entry.Requests
		resp.Tokens += entry.Tokens
		resp.Entries = append(resp.Entries, entry)
	}
	return resp, nil
}

func (s *LeaderboardService) GetInviteLeaderboard(ctx context.Context, window string, limit int, now time.Time) (*InviteLeaderboardResponse, error) {
	cfg := s.Config(ctx)
	if !cfg.Enabled {
		return nil, ErrLeaderboardDisabled
	}
	if s.repo == nil {
		return nil, infraerrors.InternalServer("SUBNEXUS_LEADERBOARD_REPOSITORY_UNAVAILABLE", "leaderboard repository is unavailable")
	}
	window = normalizeLeaderboardWindow(window)
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	start, end, title, err := resolveLeaderboardWindow(window, now)
	if err != nil {
		return nil, err
	}
	entries, err := s.repo.GetInviteLeaderboard(ctx, start, end, limit)
	if err != nil {
		return nil, err
	}
	if entries == nil {
		entries = []InviteLeaderboardEntry{}
	}
	resp := &InviteLeaderboardResponse{
		Window:    window,
		Title:     title,
		StartDate: start.Format("2006-01-02"),
		EndDate:   end.Add(-time.Nanosecond).Format("2006-01-02"),
		Entries:   make([]InviteLeaderboardEntry, 0, len(entries)),
	}
	for i := range entries {
		entry := entries[i]
		if entry.Rank <= 0 {
			entry.Rank = i + 1
		}
		entry.Email = maskEmail(entry.Email)
		resp.TotalInvites += entry.InviteCount
		resp.Entries = append(resp.Entries, entry)
	}
	return resp, nil
}

func normalizeLeaderboardWindow(window string) string {
	switch strings.ToLower(strings.TrimSpace(window)) {
	case "", LeaderboardWindowWeek:
		return LeaderboardWindowWeek
	case "today", "day":
		return LeaderboardWindowToday
	case LeaderboardWindowMonth:
		return LeaderboardWindowMonth
	default:
		return strings.ToLower(strings.TrimSpace(window))
	}
}

func resolveLeaderboardWindow(window string, now time.Time) (time.Time, time.Time, string, error) {
	switch normalizeLeaderboardWindow(window) {
	case LeaderboardWindowWeek:
		start := timezone.StartOfWeek(now)
		return start, start.AddDate(0, 0, 7), "本周榜", nil
	case LeaderboardWindowMonth:
		start := timezone.StartOfMonth(now)
		return start, start.AddDate(0, 1, 0), "本月榜", nil
	case LeaderboardWindowToday:
		start := timezone.StartOfDay(now)
		return start, start.AddDate(0, 0, 1), "今日榜", nil
	default:
		return time.Time{}, time.Time{}, "", infraerrors.BadRequest("INVALID_LEADERBOARD_WINDOW", "window must be today, week, or month")
	}
}

func leaderboardPeriodKey(window string, start time.Time) string {
	switch window {
	case LeaderboardWindowMonth:
		return start.Format("2006-01")
	case LeaderboardWindowToday:
		return start.Format("2006-01-02")
	default:
		year, week := start.ISOWeek()
		return strconv.Itoa(year) + "-W" + fmt.Sprintf("%02d", week)
	}
}

func rewardSettingsForLeaderboardWindow(window string, cfg LeaderboardConfig) (int, []float64, string) {
	if window == LeaderboardWindowMonth {
		if !cfg.MonthlyEnabled {
			return 0, nil, ""
		}
		return cfg.MonthlyTopN, cfg.MonthlyRewards, "leaderboard_month"
	}
	if !cfg.WeeklyEnabled {
		return 0, nil, ""
	}
	return cfg.WeeklyTopN, cfg.WeeklyRewards, "leaderboard_week"
}

func firstLeaderboardReward(values []float64) float64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

// LeaderboardRewardLog is the admin-facing subset of activity_reward_logs.
// Only leaderboard sources are returned by ListRewardHistory.
type LeaderboardRewardLog struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Email     string    `json:"email"`
	Source    string    `json:"source"`
	Period    string    `json:"period"`
	Rank      int       `json:"rank"`
	Amount    float64   `json:"amount"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"created_at"`
}

type LeaderboardRewardHistoryResponse struct {
	Items []LeaderboardRewardLog `json:"items"`
	Total int64                  `json:"total"`
}

// ActivityRewardLog aliases the legacy name used by callers that only dealt
// with leaderboard rewards. It intentionally does not expose non-leaderboard
// activity rows through the new service.
type ActivityRewardLog = LeaderboardRewardLog
type ActivityRewardHistoryResponse = LeaderboardRewardHistoryResponse

// GrantLeaderboardRewards settles the period containing now. For scheduled
// settlement callers should pass the previous day or use
// SettleCompletedLeaderboardPeriod, which cannot accidentally settle an open
// period.
func (s *LeaderboardService) GrantLeaderboardRewards(ctx context.Context, window string, now time.Time) (int, float64, error) {
	window = normalizeLeaderboardWindow(window)
	if window == LeaderboardWindowToday {
		return 0, 0, infraerrors.BadRequest("INVALID_LEADERBOARD_WINDOW", "leaderboard rewards support week or month only")
	}
	start, end, _, err := resolveLeaderboardWindow(window, now)
	if err != nil {
		return 0, 0, err
	}
	return s.grantLeaderboardRewardsForBounds(ctx, window, start, end, leaderboardPeriodKey(window, start))
}

// GrantLeaderboardRewardsForPeriod settles an explicit canonical period. Both
// YYYY-W01 and YYYY-W1 are accepted on input, while all newly written logs use
// the legacy-compatible zero-padded YYYY-W01 representation.
func (s *LeaderboardService) GrantLeaderboardRewardsForPeriod(ctx context.Context, window, period string) (int, float64, error) {
	window = normalizeLeaderboardWindow(window)
	start, end, canonical, err := leaderboardPeriodBounds(window, period)
	if err != nil {
		return 0, 0, err
	}
	return s.grantLeaderboardRewardsForBounds(ctx, window, start, end, canonical)
}

// SettleLeaderboardPeriod is a descriptive alias used by schedulers and
// callers migrating from the old ActivityService terminology.
func (s *LeaderboardService) SettleLeaderboardPeriod(ctx context.Context, window, period string) (int, float64, error) {
	return s.GrantLeaderboardRewardsForPeriod(ctx, window, period)
}

// SettleCompletedLeaderboardPeriod settles the last complete week/month.
func (s *LeaderboardService) SettleCompletedLeaderboardPeriod(ctx context.Context, window string, now time.Time) (int, float64, error) {
	window = normalizeLeaderboardWindow(window)
	if window == LeaderboardWindowToday {
		return 0, 0, infraerrors.BadRequest("INVALID_LEADERBOARD_WINDOW", "leaderboard rewards support week or month only")
	}
	current, _, _, err := resolveLeaderboardWindow(window, now)
	if err != nil {
		return 0, 0, err
	}
	var start time.Time
	if window == LeaderboardWindowMonth {
		start = current.AddDate(0, -1, 0)
	} else {
		start = current.AddDate(0, 0, -7)
	}
	end := current
	return s.grantLeaderboardRewardsForBounds(ctx, window, start, end, leaderboardPeriodKey(window, start))
}

// grantLeaderboardRewardsForBounds performs the complete financial operation
// in one transaction. A duplicate unique-key insert is treated as an already
// granted reward and never followed by a balance update.
func (s *LeaderboardService) grantLeaderboardRewardsForBounds(ctx context.Context, window string, start, end time.Time, period string) (int, float64, error) {
	if s == nil || s.settings == nil {
		return 0, 0, ErrLeaderboardDisabled
	}
	// Fast fail keeps a disabled rollout from touching the business tables. The
	// transaction repeats this check to close the toggle race.
	if !s.strictEnabled(ctx) {
		return 0, 0, ErrLeaderboardDisabled
	}
	preCfg := s.Config(ctx)
	topN, rewards, source := rewardSettingsForLeaderboardWindow(window, preCfg)
	if topN <= 0 || source == "" || !hasPositiveLeaderboardReward(rewards) {
		return 0, 0, ErrLeaderboardRewardDisabled
	}
	if s.db == nil {
		return 0, 0, infraerrors.InternalServer("SUBNEXUS_LEADERBOARD_DB_UNAVAILABLE", "leaderboard reward database is unavailable")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := requireLeaderboardGateTx(ctx, tx); err != nil {
		return 0, 0, err
	}
	cfg, err := leaderboardConfigTx(ctx, tx)
	if err != nil {
		return 0, 0, err
	}
	cfg.Enabled = true
	topN, rewards, source = rewardSettingsForLeaderboardWindow(window, cfg)
	if topN <= 0 || source == "" || !hasPositiveLeaderboardReward(rewards) {
		return 0, 0, ErrLeaderboardRewardDisabled
	}
	reader, ok := s.repo.(LeaderboardRewardRepository)
	if !ok || reader == nil {
		return 0, 0, infraerrors.InternalServer("SUBNEXUS_LEADERBOARD_REPOSITORY_UNAVAILABLE", "leaderboard reward repository is unavailable")
	}
	entries, err := reader.GetLeaderboardTx(ctx, tx, start, end, topN, source, period)
	if err != nil {
		return 0, 0, err
	}
	awardedUsers := make([]int64, 0, len(entries))
	var granted int
	var totalCents int64
	for i := range entries {
		entry := entries[i]
		rank := entry.Rank
		if rank <= 0 {
			rank = i + 1
		}
		if rank > topN || entry.UserID <= 0 {
			continue
		}
		amount, amountCents, err := leaderboardRewardForRank(rewards, rank)
		if err != nil {
			return 0, 0, err
		}
		if amountCents <= 0 {
			continue
		}
		note := fmt.Sprintf("%s rank #%d reward", window, rank)
		result, err := tx.ExecContext(ctx, `
			INSERT INTO activity_reward_logs (user_id, source, period, rank, amount, note)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (source, period, user_id) DO NOTHING
		`, entry.UserID, source, period, rank, amount, note)
		if err != nil {
			return 0, 0, err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return 0, 0, err
		}
		if rows == 0 {
			continue
		}
		if err := creditLeaderboardUserTx(ctx, tx, entry.UserID, amount); err != nil {
			return 0, 0, err
		}
		awardedUsers = append(awardedUsers, entry.UserID)
		granted++
		totalCents += amountCents
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}
	for _, userID := range awardedUsers {
		s.invalidateLeaderboardBalanceCaches(ctx, userID)
	}
	return granted, float64(totalCents) / 100, nil
}

func requireLeaderboardGateTx(ctx context.Context, tx *sql.Tx) error {
	if tx == nil {
		return ErrLeaderboardDisabled
	}
	var raw string
	if err := tx.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=$1 FOR UPDATE`, SettingKeySubNexusLeaderboardEnabled).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrLeaderboardDisabled
		}
		return infraerrors.InternalServer("SUBNEXUS_LEADERBOARD_GATE_UNAVAILABLE", "failed to verify leaderboard switch")
	}
	if raw != "true" {
		return ErrLeaderboardDisabled
	}
	return nil
}

func leaderboardConfigTx(ctx context.Context, tx *sql.Tx) (LeaderboardConfig, error) {
	if tx == nil {
		return DefaultLeaderboardConfig(), infraerrors.InternalServer("SUBNEXUS_LEADERBOARD_GATE_UNAVAILABLE", "leaderboard transaction is unavailable")
	}
	var raw string
	if err := tx.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=$1 FOR SHARE`, SettingKeySubNexusLeaderboardConfig).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DefaultLeaderboardConfig(), ErrLeaderboardRewardDisabled
		}
		return DefaultLeaderboardConfig(), infraerrors.InternalServer("SUBNEXUS_LEADERBOARD_CONFIG_UNAVAILABLE", "failed to read leaderboard policy")
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" || !strings.HasPrefix(trimmed, "{") {
		return DefaultLeaderboardConfig(), ErrLeaderboardRewardDisabled
	}
	var cfg LeaderboardConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return DefaultLeaderboardConfig(), ErrLeaderboardRewardDisabled
	}
	cfg = normalizeLeaderboardConfig(cfg)
	if err := validateLeaderboardConfig(cfg); err != nil {
		return DefaultLeaderboardConfig(), ErrLeaderboardRewardDisabled
	}
	return cfg, nil
}

func hasPositiveLeaderboardReward(values []float64) bool {
	for _, value := range values {
		if cents, err := leaderboardMoneyCents(value); err == nil && cents > 0 {
			return true
		}
	}
	return false
}

func leaderboardRewardForRank(values []float64, rank int) (float64, int64, error) {
	if rank <= 0 || rank > len(values) {
		return 0, 0, nil
	}
	cents, err := leaderboardMoneyCents(values[rank-1])
	if err != nil {
		return 0, 0, infraerrors.BadRequest("LEADERBOARD_REWARD_INVALID", "leaderboard rewards must be finite, positive, and within the supported range")
	}
	return float64(cents) / 100, cents, nil
}

func leaderboardMoneyCents(value float64) (int64, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 || value > float64(leaderboardMaxRewardCents)/100 {
		return 0, errors.New("invalid leaderboard reward amount")
	}
	scaled := math.Round(value * 100)
	if scaled < 1 || scaled > float64(leaderboardMaxRewardCents) {
		return 0, errors.New("invalid leaderboard reward amount")
	}
	return int64(scaled), nil
}

func creditLeaderboardUserTx(ctx context.Context, tx *sql.Tx, userID int64, amount float64) error {
	result, err := tx.ExecContext(ctx, `
		UPDATE users
		SET balance = balance + $1, total_recharged = total_recharged + $1, updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
	`, amount, userID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return infraerrors.NotFound("USER_NOT_FOUND", "user not found")
	}
	return nil
}

func (s *LeaderboardService) invalidateLeaderboardBalanceCaches(ctx context.Context, userID int64) {
	if s == nil {
		return
	}
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

// ListRewardHistory returns only leaderboard grants, preventing unrelated
// migrated activity logs from leaking through this namespace.
func (s *LeaderboardService) ListRewardHistory(ctx context.Context, page, pageSize int) (*LeaderboardRewardHistoryResponse, error) {
	if s == nil || s.db == nil {
		return nil, infraerrors.InternalServer("SUBNEXUS_LEADERBOARD_DB_UNAVAILABLE", "leaderboard database is unavailable")
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize
	const countSQL = `SELECT COUNT(*) FROM activity_reward_logs WHERE source IN ($1, $2)`
	var total int64
	if err := s.db.QueryRowContext(ctx, countSQL, ActivitySourceLeaderboardWeek, ActivitySourceLeaderboardMonth).Scan(&total); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT ar.id, ar.user_id, COALESCE(u.email, ''), ar.source, ar.period, ar.rank, ar.amount, ar.note, ar.created_at
		FROM activity_reward_logs ar
		LEFT JOIN users u ON u.id = ar.user_id
		WHERE ar.source IN ($1, $2)
		ORDER BY ar.created_at DESC, ar.id DESC
		LIMIT $3 OFFSET $4
	`, ActivitySourceLeaderboardWeek, ActivitySourceLeaderboardMonth, pageSize, offset)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]LeaderboardRewardLog, 0, pageSize)
	for rows.Next() {
		var item LeaderboardRewardLog
		if err := rows.Scan(&item.ID, &item.UserID, &item.Email, &item.Source, &item.Period, &item.Rank, &item.Amount, &item.Note, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.Email = maskEmail(item.Email)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &LeaderboardRewardHistoryResponse{Items: items, Total: total}, nil
}

// CleanupLeaderboardRewardHistory removes only old leaderboard logs. It is
// intentionally explicit and never runs as part of a normal settlement.
func (s *LeaderboardService) CleanupLeaderboardRewardHistory(ctx context.Context, before time.Time) (int64, error) {
	if s == nil || s.db == nil {
		return 0, infraerrors.InternalServer("SUBNEXUS_LEADERBOARD_DB_UNAVAILABLE", "leaderboard database is unavailable")
	}
	if !s.strictEnabled(ctx) {
		return 0, nil
	}
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM activity_reward_logs
		WHERE source IN ($1, $2) AND created_at < $3
	`, ActivitySourceLeaderboardWeek, ActivitySourceLeaderboardMonth, before)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func leaderboardPeriodBounds(window, period string) (time.Time, time.Time, string, error) {
	window = normalizeLeaderboardWindow(window)
	period = strings.TrimSpace(period)
	loc := timezone.Location()
	switch window {
	case LeaderboardWindowMonth:
		start, err := time.ParseInLocation("2006-01", period, loc)
		if err != nil || start.Format("2006-01") != period {
			return time.Time{}, time.Time{}, "", infraerrors.BadRequest("INVALID_LEADERBOARD_PERIOD", "month period must use YYYY-MM")
		}
		return start, start.AddDate(0, 1, 0), start.Format("2006-01"), nil
	case LeaderboardWindowWeek:
		// Accept the legacy padded form (YYYY-WNN) and the short form emitted
		// briefly by an early migration build (YYYY-WN), but reject trailing
		// characters and malformed widths before parsing integers.
		if len(period) < 7 || len(period) > 8 || period[4] != '-' || period[5] != 'W' {
			return time.Time{}, time.Time{}, "", infraerrors.BadRequest("INVALID_LEADERBOARD_PERIOD", "week period must use YYYY-WNN")
		}
		for _, r := range period[:4] {
			if r < '0' || r > '9' {
				return time.Time{}, time.Time{}, "", infraerrors.BadRequest("INVALID_LEADERBOARD_PERIOD", "week period must use YYYY-WNN")
			}
		}
		weekPart := period[6:]
		for _, r := range weekPart {
			if r < '0' || r > '9' {
				return time.Time{}, time.Time{}, "", infraerrors.BadRequest("INVALID_LEADERBOARD_PERIOD", "week period must use YYYY-WNN")
			}
		}
		year, err := strconv.Atoi(period[:4])
		if err != nil {
			return time.Time{}, time.Time{}, "", infraerrors.BadRequest("INVALID_LEADERBOARD_PERIOD", "week period must use YYYY-WNN")
		}
		week, err := strconv.Atoi(weekPart)
		if err != nil || year < 1 || week < 1 || week > 53 {
			return time.Time{}, time.Time{}, "", infraerrors.BadRequest("INVALID_LEADERBOARD_PERIOD", "week period must use YYYY-WNN")
		}
		// ISO week 1 always contains January 4. Normalize through ISOWeek so
		// invalid week 53 values are rejected for years that have only 52 weeks.
		jan4 := time.Date(year, time.January, 4, 0, 0, 0, 0, loc)
		monday := jan4.AddDate(0, 0, -((int(jan4.Weekday()) + 6) % 7))
		start := monday.AddDate(0, 0, (week-1)*7)
		isoYear, isoWeek := start.ISOWeek()
		if isoYear != year || isoWeek != week {
			return time.Time{}, time.Time{}, "", infraerrors.BadRequest("INVALID_LEADERBOARD_PERIOD", "invalid ISO week period")
		}
		return start, start.AddDate(0, 0, 7), leaderboardPeriodKey(window, start), nil
	default:
		return time.Time{}, time.Time{}, "", infraerrors.BadRequest("INVALID_LEADERBOARD_WINDOW", "leaderboard rewards support week or month only")
	}
}
