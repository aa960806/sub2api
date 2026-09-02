package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	SettingKeySubNexusMarqueeEnabled = "subnexus_marquee_enabled"
	MarqueeSourceAdmin               = "admin"
	marqueeContentMaxRunes           = 4000
	marqueeTitleMaxRunes             = 120
	marqueeMaxPriority               = 1000
	marqueeDefaultLimit              = 12
	marqueeMaxLimit                  = 50
)

var ErrMarqueeDisabled = infraerrors.Forbidden(
	"SUBNEXUS_MARQUEE_DISABLED",
	"marquee is disabled",
)

type MarqueeConfig struct {
	Enabled bool `json:"enabled"`
}

type MarqueeBroadcast struct {
	ID        int64      `json:"id"`
	Title     string     `json:"title"`
	Content   string     `json:"content"`
	Source    string     `json:"source"`
	Enabled   bool       `json:"enabled"`
	Priority  int        `json:"priority"`
	StartAt   *time.Time `json:"start_at,omitempty"`
	EndAt     *time.Time `json:"end_at,omitempty"`
	CreatedBy *int64     `json:"created_by,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type MarqueeBroadcastInput struct {
	Title    string     `json:"title"`
	Content  string     `json:"content"`
	Enabled  bool       `json:"enabled"`
	Priority int        `json:"priority"`
	StartAt  *time.Time `json:"start_at"`
	EndAt    *time.Time `json:"end_at"`
}

type MarqueeListResponse struct {
	Enabled bool               `json:"enabled"`
	Items   []MarqueeBroadcast `json:"items"`
}

// MarqueeRepository is intentionally limited to administrator-authored rows.
// The migrated runtime must never surface legacy reward-event broadcasts.
type MarqueeRepository interface {
	ListActiveAdmin(ctx context.Context, now time.Time, limit int) ([]MarqueeBroadcast, error)
	ListAdmin(ctx context.Context) ([]MarqueeBroadcast, error)
	CreateAdmin(ctx context.Context, input MarqueeBroadcastInput, adminID int64) (*MarqueeBroadcast, error)
	UpdateAdmin(ctx context.Context, id int64, input MarqueeBroadcastInput) (*MarqueeBroadcast, error)
	DeleteAdmin(ctx context.Context, id int64) (bool, error)
}

type MarqueeService struct {
	repo            MarqueeRepository
	settingRepo     SettingRepository
	settingsUpdated func()
}

func NewMarqueeService(repo MarqueeRepository, settingRepo SettingRepository) *MarqueeService {
	return &MarqueeService{repo: repo, settingRepo: settingRepo}
}

func (s *MarqueeService) SetSettingsUpdatedNotifier(notifier func()) {
	if s != nil {
		s.settingsUpdated = notifier
	}
}

func (s *MarqueeService) GetConfig(ctx context.Context) MarqueeConfig {
	if s == nil || s.settingRepo == nil {
		return MarqueeConfig{}
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeySubNexusMarqueeEnabled)
	if err != nil {
		return MarqueeConfig{}
	}
	return MarqueeConfig{Enabled: raw == "true"}
}

func (s *MarqueeService) UpdateConfig(ctx context.Context, cfg MarqueeConfig) (MarqueeConfig, error) {
	if s == nil || s.settingRepo == nil {
		return MarqueeConfig{}, infraerrors.InternalServer(
			"SUBNEXUS_MARQUEE_SETTINGS_UNAVAILABLE",
			"marquee settings repository is unavailable",
		)
	}
	value := "false"
	if cfg.Enabled {
		value = "true"
	}
	if err := s.settingRepo.Set(ctx, SettingKeySubNexusMarqueeEnabled, value); err != nil {
		return MarqueeConfig{}, err
	}
	if s.settingsUpdated != nil {
		s.settingsUpdated()
	}
	return cfg, nil
}

func (s *MarqueeService) ListVisible(ctx context.Context, now time.Time, limit int) (*MarqueeListResponse, error) {
	result := &MarqueeListResponse{Items: []MarqueeBroadcast{}}
	if !s.GetConfig(ctx).Enabled {
		return result, nil
	}
	result.Enabled = true
	if s.repo == nil {
		return nil, marqueeRepositoryUnavailable()
	}
	if limit <= 0 {
		limit = marqueeDefaultLimit
	}
	if limit > marqueeMaxLimit {
		limit = marqueeMaxLimit
	}
	items, err := s.repo.ListActiveAdmin(ctx, now, limit)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []MarqueeBroadcast{}
	}
	result.Items = items
	return result, nil
}

func (s *MarqueeService) ListAdmin(ctx context.Context) ([]MarqueeBroadcast, error) {
	if !s.GetConfig(ctx).Enabled {
		return []MarqueeBroadcast{}, nil
	}
	if s.repo == nil {
		return nil, marqueeRepositoryUnavailable()
	}
	items, err := s.repo.ListAdmin(ctx)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []MarqueeBroadcast{}
	}
	return items, nil
}

func (s *MarqueeService) Create(ctx context.Context, input MarqueeBroadcastInput, adminID int64) (*MarqueeBroadcast, error) {
	if err := s.requireEnabled(ctx); err != nil {
		return nil, err
	}
	input, err := normalizeMarqueeInput(input)
	if err != nil {
		return nil, err
	}
	return s.repo.CreateAdmin(ctx, input, adminID)
}

func (s *MarqueeService) Update(ctx context.Context, id int64, input MarqueeBroadcastInput) (*MarqueeBroadcast, error) {
	if err := s.requireEnabled(ctx); err != nil {
		return nil, err
	}
	if id <= 0 {
		return nil, infraerrors.BadRequest("SUBNEXUS_MARQUEE_ID_INVALID", "broadcast id must be positive")
	}
	input, err := normalizeMarqueeInput(input)
	if err != nil {
		return nil, err
	}
	item, err := s.repo.UpdateAdmin(ctx, id, input)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, infraerrors.NotFound("SUBNEXUS_MARQUEE_NOT_FOUND", "administrator broadcast not found")
	}
	return item, err
}

func (s *MarqueeService) Delete(ctx context.Context, id int64) error {
	if err := s.requireEnabled(ctx); err != nil {
		return err
	}
	if id <= 0 {
		return infraerrors.BadRequest("SUBNEXUS_MARQUEE_ID_INVALID", "broadcast id must be positive")
	}
	deleted, err := s.repo.DeleteAdmin(ctx, id)
	if err != nil {
		return err
	}
	if !deleted {
		return infraerrors.NotFound("SUBNEXUS_MARQUEE_NOT_FOUND", "administrator broadcast not found")
	}
	return nil
}

func (s *MarqueeService) requireEnabled(ctx context.Context) error {
	if !s.GetConfig(ctx).Enabled {
		return ErrMarqueeDisabled
	}
	if s.repo == nil {
		return marqueeRepositoryUnavailable()
	}
	return nil
}

func marqueeRepositoryUnavailable() error {
	return infraerrors.InternalServer(
		"SUBNEXUS_MARQUEE_REPOSITORY_UNAVAILABLE",
		"marquee repository is unavailable",
	)
}

func normalizeMarqueeInput(input MarqueeBroadcastInput) (MarqueeBroadcastInput, error) {
	input.Title = strings.TrimSpace(input.Title)
	input.Content = strings.TrimSpace(input.Content)
	if utf8.RuneCountInString(input.Title) > marqueeTitleMaxRunes {
		return input, infraerrors.BadRequest("SUBNEXUS_MARQUEE_INPUT_INVALID", "title exceeds 120 characters")
	}
	if input.Content == "" || utf8.RuneCountInString(input.Content) > marqueeContentMaxRunes {
		return input, infraerrors.BadRequest("SUBNEXUS_MARQUEE_INPUT_INVALID", "content must be between 1 and 4000 characters")
	}
	if input.Priority < 0 || input.Priority > marqueeMaxPriority {
		return input, infraerrors.BadRequest("SUBNEXUS_MARQUEE_INPUT_INVALID", "priority must be between 0 and 1000")
	}
	if input.StartAt != nil && input.EndAt != nil && input.EndAt.Before(*input.StartAt) {
		return input, infraerrors.BadRequest("SUBNEXUS_MARQUEE_WINDOW_INVALID", "end_at must not be before start_at")
	}
	return input, nil
}
