//go:build unit

package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// activityCenterTestSettingRepo is deliberately small and in-memory.  The
// service must fail closed when the new setting is absent, malformed, or
// unreadable; the legacy JSON setting is intentionally never consulted.
type activityCenterTestSettingRepo struct {
	values           map[string]string
	getErr           error
	setErr           error
	getCalls         []string
	setCalls         []string
	setMultipleCalls int
}

func (r *activityCenterTestSettingRepo) Get(_ context.Context, key string) (*Setting, error) {
	value, ok := r.values[key]
	if !ok {
		return nil, ErrSettingNotFound
	}
	return &Setting{Key: key, Value: value}, nil
}

func (r *activityCenterTestSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	r.getCalls = append(r.getCalls, key)
	if r.getErr != nil {
		return "", r.getErr
	}
	value, ok := r.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}

func (r *activityCenterTestSettingRepo) Set(_ context.Context, key, value string) error {
	r.setCalls = append(r.setCalls, key)
	if r.setErr != nil {
		return r.setErr
	}
	if r.values == nil {
		r.values = make(map[string]string)
	}
	r.values[key] = value
	return nil
}

func (r *activityCenterTestSettingRepo) GetMultiple(context.Context, []string) (map[string]string, error) {
	return nil, errors.New("unexpected GetMultiple call")
}

func (r *activityCenterTestSettingRepo) SetMultiple(_ context.Context, values map[string]string) error {
	r.setMultipleCalls++
	if r.setErr != nil {
		return r.setErr
	}
	if r.values == nil {
		r.values = make(map[string]string)
	}
	for key, value := range values {
		r.values[key] = value
	}
	return nil
}

func (r *activityCenterTestSettingRepo) GetAll(context.Context) (map[string]string, error) {
	return nil, errors.New("unexpected GetAll call")
}

func (r *activityCenterTestSettingRepo) Delete(context.Context, string) error {
	return errors.New("unexpected Delete call")
}

type activityCenterTestRepo struct {
	listVisibleCalls int
	listAdminCalls   int
	createCalls      int
	updateCalls      int
	deleteCalls      int
	createdInput     ActivityCenterItemInput
	updatedInput     ActivityCenterItemInput
	item             ActivityCenterItem
	list             []ActivityCenterItem
	updateErr        error
	deleteResult     bool
}

func (r *activityCenterTestRepo) ListVisible(context.Context, time.Time) ([]ActivityCenterItem, error) {
	r.listVisibleCalls++
	return r.list, nil
}

func (r *activityCenterTestRepo) ListAdmin(context.Context) ([]ActivityCenterItem, error) {
	r.listAdminCalls++
	return r.list, nil
}

func (r *activityCenterTestRepo) Create(_ context.Context, input ActivityCenterItemInput, _ int64) (*ActivityCenterItem, error) {
	r.createCalls++
	r.createdInput = input
	item := r.item
	if item.ID == 0 {
		item.ID = 1
	}
	item.Slug = input.Slug
	item.Title = input.Title
	item.ActivityType = input.ActivityType
	return &item, nil
}

func (r *activityCenterTestRepo) Update(_ context.Context, _ int64, input ActivityCenterItemInput) (*ActivityCenterItem, error) {
	r.updateCalls++
	r.updatedInput = input
	if r.updateErr != nil {
		return nil, r.updateErr
	}
	item := r.item
	item.ID = 1
	item.Slug = input.Slug
	item.Title = input.Title
	item.ActivityType = input.ActivityType
	return &item, nil
}

func (r *activityCenterTestRepo) Delete(context.Context, int64) (bool, error) {
	r.deleteCalls++
	return r.deleteResult, nil
}

func TestActivityCenterServiceFailsClosedForMissingInvalidAndLegacySettings(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	cases := []struct {
		name   string
		values map[string]string
		getErr error
	}{
		{name: "missing", values: map[string]string{}},
		{name: "invalid uppercase", values: map[string]string{SettingKeySubNexusActivityCenterEnabled: "TRUE"}},
		{name: "invalid numeric", values: map[string]string{SettingKeySubNexusActivityCenterEnabled: "1"}},
		{name: "invalid whitespace", values: map[string]string{SettingKeySubNexusActivityCenterEnabled: " true "}},
		{name: "legacy setting is ignored", values: map[string]string{"ACTIVITY_CENTER_CONFIG": `{"enabled":true}`}},
		{name: "read error", values: map[string]string{}, getErr: errors.New("settings unavailable")},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			repo := &activityCenterTestRepo{list: []ActivityCenterItem{{ID: 9}}}
			settings := &activityCenterTestSettingRepo{values: tc.values, getErr: tc.getErr}
			svc := NewActivityCenterService(repo, settings)

			cfg := svc.GetConfig(ctx)
			require.False(t, cfg.Enabled)
			result, err := svc.ListVisible(ctx, time.Now())
			require.NoError(t, err)
			require.False(t, result.Enabled)
			require.Empty(t, result.Items)
			require.Empty(t, repo.listVisibleCalls)
		})
	}
}

func TestActivityCenterServiceClosedStateDoesNotTouchRepository(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	settings := &activityCenterTestSettingRepo{values: map[string]string{}}
	repo := &activityCenterTestRepo{deleteResult: true}
	svc := NewActivityCenterService(repo, settings)
	input := ActivityCenterItemInput{Slug: "custom", Title: "Custom"}

	_, err := svc.ListAdmin(ctx)
	require.NoError(t, err)
	_, err = svc.Create(ctx, input, 42)
	require.ErrorIs(t, err, ErrActivityCenterDisabled)
	_, err = svc.Update(ctx, 1, input)
	require.ErrorIs(t, err, ErrActivityCenterDisabled)
	err = svc.Delete(ctx, 1)
	require.ErrorIs(t, err, ErrActivityCenterDisabled)

	require.Zero(t, repo.listAdminCalls)
	require.Zero(t, repo.createCalls)
	require.Zero(t, repo.updateCalls)
	require.Zero(t, repo.deleteCalls)
}

func TestActivityCenterServiceUpdateConfigDualWritesWithoutLegacyReadInheritance(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	settings := &activityCenterTestSettingRepo{values: map[string]string{
		settingKeyLegacyActivityCenterConfig: `{"enabled":true}`,
	}}
	svc := NewActivityCenterService(&activityCenterTestRepo{}, settings)

	// The old JSON key is retained only as a compatibility write for an old
	// binary.  Runtime reads must continue to use the strict new boolean key.
	require.False(t, svc.GetConfig(ctx).Enabled)
	updated, err := svc.UpdateConfig(ctx, ActivityCenterConfig{Enabled: true})
	require.NoError(t, err)
	require.True(t, updated.Enabled)
	require.Equal(t, "true", settings.values[SettingKeySubNexusActivityCenterEnabled])
	require.Equal(t, `{"enabled":true}`, settings.values[settingKeyLegacyActivityCenterConfig])
	require.Equal(t, 1, settings.setMultipleCalls)
}

func TestActivityCenterServiceUpdateConfigNotifiesSettingsConsumersAfterWrite(t *testing.T) {
	t.Parallel()
	settings := &activityCenterTestSettingRepo{values: map[string]string{}}
	var notifications int
	svc := NewActivityCenterService(&activityCenterTestRepo{}, settings)
	svc.SetSettingsUpdatedNotifier(func() { notifications++ })

	_, err := svc.UpdateConfig(context.Background(), ActivityCenterConfig{Enabled: true})
	require.NoError(t, err)
	require.Equal(t, 1, notifications)

	settings.setErr = errors.New("write failed")
	_, err = svc.UpdateConfig(context.Background(), ActivityCenterConfig{Enabled: false})
	require.Error(t, err)
	require.Equal(t, 1, notifications, "failed writes must not invalidate caches")
}

func TestActivityCenterServiceValidatesCustomInputAndNormalizesEnabledPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	settings := &activityCenterTestSettingRepo{values: map[string]string{
		SettingKeySubNexusActivityCenterEnabled: "true",
	}}
	repo := &activityCenterTestRepo{deleteResult: true}
	svc := NewActivityCenterService(repo, settings)

	invalid := []ActivityCenterItemInput{
		{Slug: "custom", Title: "Title", ActivityType: "battle_pass"},
		{Slug: "Bad Slug", Title: "Title"},
		{Slug: "custom", Title: "Title", RoutePath: "/local", ExternalURL: "https://example.com"},
		{Slug: "custom", Title: "Title", ExternalURL: "javascript:alert(1)"},
		{Slug: "custom", Title: "Title", Metadata: json.RawMessage(`[]`)},
		{Slug: "custom", Title: "Title", Description: strings.Repeat("x", activityCenterDescriptionMaxRunes+1)},
		{Slug: "custom", Title: "Title", ExternalURL: "https://" + strings.Repeat("x", activityCenterURLMaxRunes)},
	}
	for _, input := range invalid {
		_, err := svc.Create(ctx, input, 1)
		require.Error(t, err, "input should be rejected: %+v", input)
	}
	require.Zero(t, repo.createCalls)

	item, err := svc.Create(ctx, ActivityCenterItemInput{
		Slug:         "  My-Activity  ",
		Title:        "  Hello  ",
		ActivityType: " CUSTOM ",
		Metadata:     json.RawMessage(`{"b":2,"a":1}`),
	}, 7)
	require.NoError(t, err)
	require.NotNil(t, item)
	require.Equal(t, "my-activity", repo.createdInput.Slug)
	require.Equal(t, "Hello", repo.createdInput.Title)
	require.Equal(t, ActivityCenterTypeCustom, repo.createdInput.ActivityType)
	require.Equal(t, "gift", repo.createdInput.Icon)
	require.JSONEq(t, `{"a":1,"b":2}`, string(repo.createdInput.Metadata))

	_, err = svc.Update(ctx, 1, ActivityCenterItemInput{Slug: "updated", Title: "Updated"})
	require.NoError(t, err)
	require.Equal(t, 1, repo.updateCalls)
	require.Equal(t, ActivityCenterTypeCustom, repo.updatedInput.ActivityType)
	err = svc.Delete(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, 1, repo.deleteCalls)
}

func TestActivityCenterServiceUpdateMapsMissingItemToNotFound(t *testing.T) {
	t.Parallel()
	settings := &activityCenterTestSettingRepo{values: map[string]string{SettingKeySubNexusActivityCenterEnabled: "true"}}
	repo := &activityCenterTestRepo{updateErr: sql.ErrNoRows}
	svc := NewActivityCenterService(repo, settings)

	_, err := svc.Update(context.Background(), 1, ActivityCenterItemInput{Slug: "custom", Title: "Title"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}
