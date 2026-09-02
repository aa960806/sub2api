package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/lib/pq"
)

const (
	SettingKeySubNexusActivityCenterEnabled = "subnexus_activity_center_enabled"
	settingKeyLegacyActivityCenterConfig    = "ACTIVITY_CENTER_CONFIG"
	ActivityCenterTypeCustom                = "custom"
	activityCenterMetadataMaxBytes          = 16 * 1024
	activityCenterDescriptionMaxRunes       = 2000
	activityCenterURLMaxRunes               = 2048
)

var (
	ErrActivityCenterDisabled = infraerrors.Forbidden(
		"SUBNEXUS_ACTIVITY_CENTER_DISABLED",
		"activity center is disabled",
	)
	activityCenterSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
)

type ActivityCenterConfig struct {
	Enabled bool `json:"enabled"`
}

type ActivityCenterItem struct {
	ID           int64           `json:"id"`
	Slug         string          `json:"slug"`
	Title        string          `json:"title"`
	Subtitle     string          `json:"subtitle"`
	Description  string          `json:"description"`
	Icon         string          `json:"icon"`
	CoverURL     string          `json:"cover_url"`
	RoutePath    string          `json:"route_path"`
	ExternalURL  string          `json:"external_url"`
	ActionLabel  string          `json:"action_label"`
	ActivityType string          `json:"activity_type"`
	Enabled      bool            `json:"enabled"`
	SortOrder    int             `json:"sort_order"`
	StartAt      *time.Time      `json:"start_at,omitempty"`
	EndAt        *time.Time      `json:"end_at,omitempty"`
	Metadata     json.RawMessage `json:"metadata"`
	CreatedBy    *int64          `json:"created_by,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type ActivityCenterItemInput struct {
	Slug         string          `json:"slug"`
	Title        string          `json:"title"`
	Subtitle     string          `json:"subtitle"`
	Description  string          `json:"description"`
	Icon         string          `json:"icon"`
	CoverURL     string          `json:"cover_url"`
	RoutePath    string          `json:"route_path"`
	ExternalURL  string          `json:"external_url"`
	ActionLabel  string          `json:"action_label"`
	ActivityType string          `json:"activity_type"`
	Enabled      bool            `json:"enabled"`
	SortOrder    int             `json:"sort_order"`
	StartAt      *time.Time      `json:"start_at"`
	EndAt        *time.Time      `json:"end_at"`
	Metadata     json.RawMessage `json:"metadata"`
}

type ActivityCenterListResponse struct {
	Enabled bool                 `json:"enabled"`
	Items   []ActivityCenterItem `json:"items"`
}

type ActivityCenterRepository interface {
	ListVisible(ctx context.Context, now time.Time) ([]ActivityCenterItem, error)
	ListAdmin(ctx context.Context) ([]ActivityCenterItem, error)
	Create(ctx context.Context, input ActivityCenterItemInput, adminID int64) (*ActivityCenterItem, error)
	Update(ctx context.Context, id int64, input ActivityCenterItemInput) (*ActivityCenterItem, error)
	Delete(ctx context.Context, id int64) (bool, error)
}

type ActivityCenterService struct {
	repo            ActivityCenterRepository
	settingRepo     SettingRepository
	settingsUpdated func()
}

func NewActivityCenterService(repo ActivityCenterRepository, settingRepo SettingRepository) *ActivityCenterService {
	return &ActivityCenterService{repo: repo, settingRepo: settingRepo}
}

// SetSettingsUpdatedNotifier wires the process-level cache invalidation hook.
// The hook is optional so unit tests and lightweight callers can continue to
// construct the service with only repository dependencies.
func (s *ActivityCenterService) SetSettingsUpdatedNotifier(notifier func()) {
	if s != nil {
		s.settingsUpdated = notifier
	}
}

func (s *ActivityCenterService) GetConfig(ctx context.Context) ActivityCenterConfig {
	if s == nil || s.settingRepo == nil {
		return ActivityCenterConfig{}
	}
	value, err := s.settingRepo.GetValue(ctx, SettingKeySubNexusActivityCenterEnabled)
	if err != nil {
		return ActivityCenterConfig{}
	}
	// Rollout switches use an exact serialized value.  Do not trim or
	// case-fold malformed values into an enabled state during staged rollout.
	return ActivityCenterConfig{Enabled: value == "true"}
}

func (s *ActivityCenterService) UpdateConfig(ctx context.Context, cfg ActivityCenterConfig) (ActivityCenterConfig, error) {
	if s == nil || s.settingRepo == nil {
		return ActivityCenterConfig{}, infraerrors.InternalServer(
			"SUBNEXUS_ACTIVITY_CENTER_SETTINGS_UNAVAILABLE",
			"activity center settings repository is unavailable",
		)
	}
	legacyValue, err := json.Marshal(cfg)
	if err != nil {
		return ActivityCenterConfig{}, err
	}
	if err := s.settingRepo.SetMultiple(ctx, map[string]string{
		SettingKeySubNexusActivityCenterEnabled: strconv.FormatBool(cfg.Enabled),
		settingKeyLegacyActivityCenterConfig:    string(legacyValue),
	}); err != nil {
		return ActivityCenterConfig{}, err
	}
	if s.settingsUpdated != nil {
		s.settingsUpdated()
	}
	return cfg, nil
}

func (s *ActivityCenterService) ListVisible(ctx context.Context, now time.Time) (*ActivityCenterListResponse, error) {
	cfg := s.GetConfig(ctx)
	result := &ActivityCenterListResponse{Enabled: cfg.Enabled, Items: []ActivityCenterItem{}}
	if !cfg.Enabled {
		return result, nil
	}
	if s.repo == nil {
		return nil, infraerrors.InternalServer(
			"SUBNEXUS_ACTIVITY_CENTER_REPOSITORY_UNAVAILABLE",
			"activity center repository is unavailable",
		)
	}
	items, err := s.repo.ListVisible(ctx, now)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []ActivityCenterItem{}
	}
	result.Items = items
	return result, nil
}

func (s *ActivityCenterService) ListAdmin(ctx context.Context) ([]ActivityCenterItem, error) {
	if !s.GetConfig(ctx).Enabled {
		return []ActivityCenterItem{}, nil
	}
	if s.repo == nil {
		return nil, infraerrors.InternalServer(
			"SUBNEXUS_ACTIVITY_CENTER_REPOSITORY_UNAVAILABLE",
			"activity center repository is unavailable",
		)
	}
	items, err := s.repo.ListAdmin(ctx)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []ActivityCenterItem{}
	}
	return items, nil
}

func (s *ActivityCenterService) Create(ctx context.Context, input ActivityCenterItemInput, adminID int64) (*ActivityCenterItem, error) {
	if err := s.requireEnabled(ctx); err != nil {
		return nil, err
	}
	input, err := normalizeActivityCenterItemInput(input)
	if err != nil {
		return nil, err
	}
	item, err := s.repo.Create(ctx, input, adminID)
	if isActivityCenterSlugDuplicate(err) {
		return nil, infraerrors.BadRequest("SUBNEXUS_ACTIVITY_CENTER_SLUG_DUPLICATE", "activity slug already exists")
	}
	return item, err
}

func (s *ActivityCenterService) Update(ctx context.Context, id int64, input ActivityCenterItemInput) (*ActivityCenterItem, error) {
	if err := s.requireEnabled(ctx); err != nil {
		return nil, err
	}
	if id <= 0 {
		return nil, infraerrors.BadRequest("SUBNEXUS_ACTIVITY_CENTER_ITEM_INVALID", "activity id must be positive")
	}
	input, err := normalizeActivityCenterItemInput(input)
	if err != nil {
		return nil, err
	}
	item, err := s.repo.Update(ctx, id, input)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, infraerrors.NotFound("SUBNEXUS_ACTIVITY_CENTER_ITEM_NOT_FOUND", "custom activity item not found")
	}
	if isActivityCenterSlugDuplicate(err) {
		return nil, infraerrors.BadRequest("SUBNEXUS_ACTIVITY_CENTER_SLUG_DUPLICATE", "activity slug already exists")
	}
	return item, err
}

func (s *ActivityCenterService) Delete(ctx context.Context, id int64) error {
	if err := s.requireEnabled(ctx); err != nil {
		return err
	}
	if id <= 0 {
		return infraerrors.BadRequest("SUBNEXUS_ACTIVITY_CENTER_ITEM_INVALID", "activity id must be positive")
	}
	deleted, err := s.repo.Delete(ctx, id)
	if err != nil {
		return err
	}
	if !deleted {
		return infraerrors.NotFound("SUBNEXUS_ACTIVITY_CENTER_ITEM_NOT_FOUND", "custom activity item not found")
	}
	return nil
}

func (s *ActivityCenterService) requireEnabled(ctx context.Context) error {
	if !s.GetConfig(ctx).Enabled {
		return ErrActivityCenterDisabled
	}
	if s.repo == nil {
		return infraerrors.InternalServer(
			"SUBNEXUS_ACTIVITY_CENTER_REPOSITORY_UNAVAILABLE",
			"activity center repository is unavailable",
		)
	}
	return nil
}

func normalizeActivityCenterItemInput(input ActivityCenterItemInput) (ActivityCenterItemInput, error) {
	input.Slug = strings.ToLower(strings.TrimSpace(input.Slug))
	input.Title = strings.TrimSpace(input.Title)
	input.Subtitle = strings.TrimSpace(input.Subtitle)
	input.Description = strings.TrimSpace(input.Description)
	input.Icon = strings.TrimSpace(input.Icon)
	input.CoverURL = strings.TrimSpace(input.CoverURL)
	input.RoutePath = strings.TrimSpace(input.RoutePath)
	input.ExternalURL = strings.TrimSpace(input.ExternalURL)
	input.ActionLabel = strings.TrimSpace(input.ActionLabel)
	input.ActivityType = strings.ToLower(strings.TrimSpace(input.ActivityType))

	if input.ActivityType == "" {
		input.ActivityType = ActivityCenterTypeCustom
	}
	if input.ActivityType != ActivityCenterTypeCustom {
		return input, infraerrors.BadRequest(
			"SUBNEXUS_ACTIVITY_CENTER_TYPE_UNSUPPORTED",
			"only custom activity items are supported",
		)
	}
	if input.Slug == "" || !activityCenterSlugPattern.MatchString(input.Slug) || utf8.RuneCountInString(input.Slug) > 80 {
		return input, infraerrors.BadRequest(
			"SUBNEXUS_ACTIVITY_CENTER_ITEM_INVALID",
			"slug must be 1-80 lowercase letters, numbers, underscores, or hyphens",
		)
	}
	if input.Title == "" || utf8.RuneCountInString(input.Title) > 120 {
		return input, infraerrors.BadRequest("SUBNEXUS_ACTIVITY_CENTER_ITEM_INVALID", "title must be 1-120 characters")
	}
	if utf8.RuneCountInString(input.Subtitle) > 240 || utf8.RuneCountInString(input.Icon) > 64 || utf8.RuneCountInString(input.ActionLabel) > 40 {
		return input, infraerrors.BadRequest("SUBNEXUS_ACTIVITY_CENTER_ITEM_INVALID", "one or more activity fields exceed their maximum length")
	}
	if utf8.RuneCountInString(input.Description) > activityCenterDescriptionMaxRunes {
		return input, infraerrors.BadRequest("SUBNEXUS_ACTIVITY_CENTER_ITEM_INVALID", "description exceeds 2000 characters")
	}
	if utf8.RuneCountInString(input.RoutePath) > 255 {
		return input, infraerrors.BadRequest("SUBNEXUS_ACTIVITY_CENTER_ITEM_INVALID", "route_path exceeds 255 characters")
	}
	if utf8.RuneCountInString(input.ExternalURL) > activityCenterURLMaxRunes || utf8.RuneCountInString(input.CoverURL) > activityCenterURLMaxRunes {
		return input, infraerrors.BadRequest("SUBNEXUS_ACTIVITY_CENTER_ITEM_INVALID", "activity URL exceeds 2048 characters")
	}
	if input.Icon == "" {
		input.Icon = "gift"
	}
	if input.StartAt != nil && input.EndAt != nil && input.EndAt.Before(*input.StartAt) {
		return input, infraerrors.BadRequest("SUBNEXUS_ACTIVITY_CENTER_WINDOW_INVALID", "end_at must not be before start_at")
	}
	if input.RoutePath != "" && input.ExternalURL != "" {
		return input, infraerrors.BadRequest("SUBNEXUS_ACTIVITY_CENTER_TARGET_INVALID", "set either route_path or external_url, not both")
	}
	if err := validateActivityRoutePath(input.RoutePath); err != nil {
		return input, err
	}
	if err := validateActivityHTTPURL("external_url", input.ExternalURL); err != nil {
		return input, err
	}
	if err := validateActivityHTTPURL("cover_url", input.CoverURL); err != nil {
		return input, err
	}

	if len(input.Metadata) == 0 {
		input.Metadata = json.RawMessage(`{}`)
	}
	if len(input.Metadata) > activityCenterMetadataMaxBytes {
		return input, infraerrors.BadRequest("SUBNEXUS_ACTIVITY_CENTER_METADATA_INVALID", "metadata exceeds 16 KiB")
	}
	var metadata map[string]any
	if err := json.Unmarshal(input.Metadata, &metadata); err != nil || metadata == nil {
		return input, infraerrors.BadRequest("SUBNEXUS_ACTIVITY_CENTER_METADATA_INVALID", "metadata must be a JSON object")
	}
	canonicalMetadata, err := json.Marshal(metadata)
	if err != nil {
		return input, infraerrors.BadRequest("SUBNEXUS_ACTIVITY_CENTER_METADATA_INVALID", "metadata must be a JSON object")
	}
	input.Metadata = canonicalMetadata
	return input, nil
}

func validateActivityRoutePath(value string) error {
	if value == "" {
		return nil
	}
	if strings.ContainsAny(value, "\r\n\\") || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") {
		return infraerrors.BadRequest("SUBNEXUS_ACTIVITY_CENTER_TARGET_INVALID", "route_path must be a local absolute path")
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.IsAbs() || parsed.Host != "" {
		return infraerrors.BadRequest("SUBNEXUS_ACTIVITY_CENTER_TARGET_INVALID", "route_path must be a valid local path")
	}
	return nil
}

func validateActivityHTTPURL(field, value string) error {
	if value == "" {
		return nil
	}
	if strings.ContainsAny(value, "\r\n") {
		return infraerrors.BadRequest("SUBNEXUS_ACTIVITY_CENTER_TARGET_INVALID", field+" must be an http or https URL")
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return infraerrors.BadRequest("SUBNEXUS_ACTIVITY_CENTER_TARGET_INVALID", field+" must be an http or https URL without credentials")
	}
	return nil
}

func isActivityCenterSlugDuplicate(err error) bool {
	if err == nil {
		return false
	}
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && pqErr.Code == "23505" {
		return pqErr.Constraint == "activity_center_items_slug_key" || strings.Contains(strings.ToLower(pqErr.Detail), "slug")
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "activity_center_items_slug_key") ||
		(strings.Contains(message, "duplicate key") && strings.Contains(message, "slug"))
}
