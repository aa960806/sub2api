package service

import (
	"context"
	"database/sql"
	"math"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	SettingKeyBattlePassEnabled = "battle_pass_enabled"
	ActivitySourceBattlePass    = "battle_pass"

	BattlePassStatusDraft     = "draft"
	BattlePassStatusScheduled = "scheduled"
	BattlePassStatusPaused    = "paused"
	BattlePassStatusEnded     = "ended"
	BattlePassStatusArchived  = "archived"

	battlePassScanInterval = 15 * time.Second
)

type battlePassSettings interface {
	GetValue(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
}

type BattlePassService struct {
	db              *sql.DB
	settings        battlePassSettings
	authCache       APIKeyAuthCacheInvalidator
	billingCache    battlePassBalanceCache
	subscriptions   *SubscriptionService
	settingsUpdated func()
}

// battlePassBalanceCache is intentionally narrow: battle-pass only invalidates
// data it changed after its SQL transaction has committed.
type battlePassBalanceCache interface {
	InvalidateUserBalance(ctx context.Context, userID int64) error
}

func NewBattlePassService(db *sql.DB, settings battlePassSettings) *BattlePassService {
	return &BattlePassService{db: db, settings: settings}
}

// SetSettingsUpdatedNotifier connects the feature-specific switch to the
// process-wide public-settings and embedded-frontend invalidation hook.
func (s *BattlePassService) SetSettingsUpdatedNotifier(notifier func()) {
	if s != nil {
		s.settingsUpdated = notifier
	}
}

func ProvideBattlePassService(db *sql.DB, settings SettingRepository, apiKeys *APIKeyService, billing *BillingCacheService, subscriptions *SubscriptionService, settingService *SettingService) *BattlePassService {
	service := NewBattlePassService(db, settings)
	service.authCache = apiKeys
	service.billingCache = billing
	service.subscriptions = subscriptions
	if settingService != nil {
		service.SetSettingsUpdatedNotifier(settingService.NotifySettingsUpdated)
	}
	return service
}

func (s *BattlePassService) IsEnabled(ctx context.Context) (bool, error) {
	if s == nil || s.settings == nil {
		return false, nil
	}
	raw, err := s.settings.GetValue(ctx, SettingKeyBattlePassEnabled)
	if err != nil {
		return false, nil
	}
	return raw == "true", nil
}

func (s *BattlePassService) requireEnabled(ctx context.Context) error {
	enabled, err := s.IsEnabled(ctx)
	if err != nil {
		return err
	}
	if !enabled {
		return infraerrors.NotFound("BATTLE_PASS_DISABLED", "battle pass is not available")
	}
	return nil
}

// requireEnabledTx re-reads the rollout switch using the same database
// transaction that is about to mutate Battle Pass state. The request/scheduler
// gate is intentionally not sufficient: an administrator may change the
// switch after that initial check. Missing or malformed settings fail closed.
func (s *BattlePassService) requireEnabledTx(ctx context.Context, tx *sql.Tx) error {
	if s == nil || tx == nil {
		return infraerrors.NotFound("BATTLE_PASS_DISABLED", "battle pass is not available")
	}
	var raw string
	// Lock the switch row for the lifetime of this transaction. The settings
	// repository updates the same row, so a concurrent disable waits until this
	// mutation commits (or observes the disabled value before doing any writes).
	if err := tx.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=$1 FOR UPDATE`, SettingKeyBattlePassEnabled).Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return infraerrors.NotFound("BATTLE_PASS_DISABLED", "battle pass is not available")
		}
		return infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to verify battle pass switch")
	}
	if raw != "true" {
		return infraerrors.NotFound("BATTLE_PASS_DISABLED", "battle pass is not available")
	}
	return nil
}

func (s *BattlePassService) GetSettings(ctx context.Context) (BattlePassSettings, error) {
	enabled, err := s.IsEnabled(ctx)
	if err != nil {
		return BattlePassSettings{}, err
	}
	return BattlePassSettings{Enabled: enabled, TestToolsEnabled: BattlePassTestToolsEnabled()}, nil
}

func (s *BattlePassService) SetEnabled(ctx context.Context, enabled bool) (BattlePassSettings, error) {
	if s == nil || s.settings == nil {
		return BattlePassSettings{}, infraerrors.InternalServer("BATTLE_PASS_SETTINGS_UNAVAILABLE", "battle pass settings repository is unavailable")
	}
	previous, _ := s.IsEnabled(ctx)
	// Enabling is ordered fail-closed: establish the accounting boundary before
	// publishing the switch. A failed snapshot must never leave the runtime on.
	if enabled && !previous {
		if err := s.captureEnableSnapshot(ctx); err != nil {
			return BattlePassSettings{}, infraerrors.InternalServer("BATTLE_PASS_SAVE_SWITCH_FAILED", "failed to capture battle pass enable time")
		}
	}
	value := "false"
	if enabled {
		value = "true"
	}
	if err := s.settings.Set(ctx, SettingKeyBattlePassEnabled, value); err != nil {
		return BattlePassSettings{}, infraerrors.InternalServer("BATTLE_PASS_SAVE_SWITCH_FAILED", "failed to save battle pass switch")
	}
	if s.settingsUpdated != nil {
		s.settingsUpdated()
	}
	if !enabled {
		if err := s.bumpActiveSeasonEpoch(ctx); err != nil {
			return BattlePassSettings{}, infraerrors.InternalServer("BATTLE_PASS_SAVE_SWITCH_FAILED", "failed to record battle pass disable epoch")
		}
	}
	return BattlePassSettings{Enabled: enabled, TestToolsEnabled: BattlePassTestToolsEnabled()}, nil
}

func (s *BattlePassService) captureEnableSnapshot(ctx context.Context) error {
	if s == nil || s.db == nil {
		return sql.ErrConnDone
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE battle_pass_seasons
		SET enabled_at_snapshot = NOW(),
		    statistics_start_at = GREATEST(COALESCE(statistics_start_at, start_at), NOW()),
		    updated_at = NOW()
		WHERE status IN ('scheduled', 'paused')
	`)
	return err
}

func (s *BattlePassService) bumpActiveSeasonEpoch(ctx context.Context) error {
	if s == nil || s.db == nil {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE battle_pass_seasons
		SET activation_epoch = activation_epoch + 1, updated_at = NOW()
		WHERE status IN ('scheduled', 'paused')
	`)
	return err
}

type BattlePassSettings struct {
	Enabled          bool `json:"enabled"`
	TestToolsEnabled bool `json:"test_tools_enabled"`
}

type BattlePassSeason struct {
	ID                int64      `json:"id"`
	Name              string     `json:"name"`
	Description       string     `json:"description"`
	Status            string     `json:"status"`
	RuntimeStatus     string     `json:"runtime_status"`
	Timezone          string     `json:"timezone"`
	StartAt           time.Time  `json:"start_at"`
	EndAt             time.Time  `json:"end_at"`
	PremiumPrice      float64    `json:"premium_price"`
	MaxLevel          int        `json:"max_level"`
	ActivationEpoch   int        `json:"activation_epoch"`
	PublishedAt       *time.Time `json:"published_at,omitempty"`
	EnabledAtSnapshot *time.Time `json:"enabled_at_snapshot,omitempty"`
	UserSideEnabled   bool       `json:"user_side_enabled"`
}

type BattlePassSeasonInput struct {
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	Timezone     string    `json:"timezone"`
	StartAt      time.Time `json:"start_at"`
	EndAt        time.Time `json:"end_at"`
	PremiumPrice float64   `json:"premium_price"`
	MaxLevel     int       `json:"max_level"`
}

type BattlePassCurrent struct {
	Season          *BattlePassSeason       `json:"season"`
	Progress        *BattlePassUserProgress `json:"progress,omitempty"`
	Syncing         bool                    `json:"syncing"`
	UserSideEnabled bool                    `json:"user_side_enabled"`
}

func (s *BattlePassService) GetCurrent(ctx context.Context, now time.Time) (*BattlePassCurrent, error) {
	if err := s.requireUserAccess(ctx, now); err != nil {
		return nil, err
	}
	season, err := s.currentPublishedSeason(ctx, now)
	if err != nil {
		return nil, err
	}
	return &BattlePassCurrent{Season: season, Syncing: false, UserSideEnabled: true}, nil
}

func (s *BattlePassService) ListSeasons(ctx context.Context, now time.Time) ([]BattlePassSeason, error) {
	if s == nil || s.db == nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "battle pass database is unavailable")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, description, status, timezone, start_at, end_at, premium_price, max_level,
		       activation_epoch, published_at, enabled_at_snapshot
		FROM battle_pass_seasons
		ORDER BY id DESC
	`)
	if err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to list battle pass seasons")
	}
	defer rows.Close()
	enabled, _ := s.IsEnabled(ctx)
	items := make([]BattlePassSeason, 0)
	for rows.Next() {
		item, err := scanBattlePassSeason(rows)
		if err != nil {
			return nil, err
		}
		item.RuntimeStatus = runtimeSeasonStatus(item, now)
		item.UserSideEnabled = enabled
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *BattlePassService) CreateSeason(ctx context.Context, draft BattlePassSeasonDraft, adminID int64) (*BattlePassSeasonDetail, error) {
	if s == nil || s.db == nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "battle pass database is unavailable")
	}
	draft.BattlePassSeasonInput = normalizeBattlePassSeasonInput(draft.BattlePassSeasonInput)
	now := time.Now()
	if err := validateDraftSeason(draft.BattlePassSeasonInput, now); err != nil {
		return nil, err
	}
	var createdBy any
	if adminID > 0 {
		createdBy = adminID
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to create battle pass season")
	}
	defer func() { _ = tx.Rollback() }()
	var id int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO battle_pass_seasons (
			name, description, status, timezone, start_at, end_at, premium_price, max_level, created_by
		) VALUES ($1,$2,'draft',$3,$4,$5,$6,$7,$8)
		RETURNING id
	`, draft.Name, draft.Description, draft.Timezone, draft.StartAt, draft.EndAt, draft.PremiumPrice, draft.MaxLevel, createdBy).Scan(&id); err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to create battle pass season")
	}
	if err := replaceSeasonChildren(ctx, tx, id, draft); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to create battle pass season")
	}
	return s.GetSeason(ctx, id, now)
}

func (s *BattlePassService) currentPublishedSeason(ctx context.Context, now time.Time) (*BattlePassSeason, error) {
	if s == nil || s.db == nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "battle pass database is unavailable")
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, description, status, timezone, start_at, end_at, premium_price, max_level,
		       activation_epoch, published_at, enabled_at_snapshot
		FROM battle_pass_seasons
		WHERE status IN ('scheduled', 'paused', 'ended')
		ORDER BY
		  CASE
		    WHEN status='paused' AND end_at > $1 THEN 0
		    WHEN status='scheduled' AND start_at <= $1 AND end_at > $1 THEN 0
		    WHEN status='scheduled' AND start_at > $1 THEN 1
		    ELSE 2
		  END,
		  CASE WHEN status='scheduled' AND start_at > $1 THEN start_at END ASC,
		  CASE WHEN status IN ('scheduled', 'paused') THEN start_at ELSE end_at END DESC
		LIMIT 1
	`, now)
	item, err := scanBattlePassSeason(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, infraerrors.InternalServer("BATTLE_PASS_DB_UNAVAILABLE", "failed to load battle pass season")
	}
	item.RuntimeStatus = runtimeSeasonStatus(item, now)
	item.UserSideEnabled = true
	return &item, nil
}

func normalizeBattlePassSeasonInput(input BattlePassSeasonInput) BattlePassSeasonInput {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.Timezone = strings.TrimSpace(input.Timezone)
	if input.Timezone == "" {
		input.Timezone = "Asia/Shanghai"
	}
	if input.MaxLevel < 1 {
		input.MaxLevel = 1
	}
	return input
}

func validateDraftSeason(input BattlePassSeasonInput, now time.Time) error {
	if input.Name == "" {
		return infraerrors.BadRequest("BATTLE_PASS_SEASON_INVALID", "season name is required")
	}
	if !input.EndAt.After(input.StartAt) {
		return infraerrors.BadRequest("BATTLE_PASS_SEASON_INVALID", "end_at must be after start_at")
	}
	if !input.StartAt.After(now) {
		return infraerrors.BadRequest("BATTLE_PASS_SEASON_INVALID", "start_at must not be in the past")
	}
	if math.IsNaN(input.PremiumPrice) || math.IsInf(input.PremiumPrice, 0) || input.PremiumPrice < 0 || input.PremiumPrice > 1000000 {
		return infraerrors.BadRequest("BATTLE_PASS_SEASON_INVALID", "premium_price must be >= 0")
	}
	if input.MaxLevel > 100 {
		return infraerrors.BadRequest("BATTLE_PASS_SEASON_INVALID", "max_level must not exceed 100")
	}
	if _, err := time.LoadLocation(input.Timezone); err != nil {
		return infraerrors.BadRequest("BATTLE_PASS_SEASON_INVALID", "timezone is invalid")
	}
	return nil
}

func runtimeSeasonStatus(season BattlePassSeason, now time.Time) string {
	switch season.Status {
	case BattlePassStatusDraft, BattlePassStatusEnded, BattlePassStatusArchived, BattlePassStatusPaused:
		return season.Status
	case BattlePassStatusScheduled:
		if now.Before(season.StartAt) {
			return BattlePassStatusScheduled
		}
		if !now.Before(season.EndAt) {
			return BattlePassStatusEnded
		}
		return "active"
	default:
		return season.Status
	}
}

type battlePassSeasonScanner interface {
	Scan(dest ...any) error
}

func scanBattlePassSeason(row battlePassSeasonScanner) (BattlePassSeason, error) {
	var item BattlePassSeason
	var publishedAt, enabledAt sql.NullTime
	err := row.Scan(
		&item.ID, &item.Name, &item.Description, &item.Status, &item.Timezone,
		&item.StartAt, &item.EndAt, &item.PremiumPrice, &item.MaxLevel, &item.ActivationEpoch,
		&publishedAt, &enabledAt,
	)
	if err != nil {
		return BattlePassSeason{}, err
	}
	if publishedAt.Valid {
		t := publishedAt.Time
		item.PublishedAt = &t
	}
	if enabledAt.Valid {
		t := enabledAt.Time
		item.EnabledAtSnapshot = &t
	}
	return item, nil
}
