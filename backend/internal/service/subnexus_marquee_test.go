package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type marqueeSettingRepoStub struct {
	values map[string]string
	getErr error
	setErr error
	sets   map[string]string
}

func (r *marqueeSettingRepoStub) Get(context.Context, string) (*Setting, error) {
	return nil, errors.New("unexpected Get call")
}
func (r *marqueeSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	if r.getErr != nil {
		return "", r.getErr
	}
	value, ok := r.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}
func (r *marqueeSettingRepoStub) Set(_ context.Context, key, value string) error {
	if r.setErr != nil {
		return r.setErr
	}
	if r.sets == nil {
		r.sets = map[string]string{}
	}
	r.sets[key] = value
	if r.values == nil {
		r.values = map[string]string{}
	}
	r.values[key] = value
	return nil
}
func (r *marqueeSettingRepoStub) GetMultiple(context.Context, []string) (map[string]string, error) {
	return nil, errors.New("unexpected GetMultiple call")
}
func (r *marqueeSettingRepoStub) SetMultiple(context.Context, map[string]string) error {
	return errors.New("unexpected SetMultiple call")
}
func (r *marqueeSettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	return nil, errors.New("unexpected GetAll call")
}
func (r *marqueeSettingRepoStub) Delete(context.Context, string) error {
	return errors.New("unexpected Delete call")
}

type marqueeRepoStub struct {
	listVisibleCalls int
	listAdminCalls   int
	createCalls      int
	updateCalls      int
	deleteCalls      int
	lastLimit        int
	lastInput        MarqueeBroadcastInput
	updateErr        error
	deleteResult     bool
}

func (r *marqueeRepoStub) ListActiveAdmin(context.Context, time.Time, int) ([]MarqueeBroadcast, error) {
	r.listVisibleCalls++
	return []MarqueeBroadcast{{ID: 1, Source: MarqueeSourceAdmin}}, nil
}
func (r *marqueeRepoStub) ListAdmin(context.Context) ([]MarqueeBroadcast, error) {
	r.listAdminCalls++
	return []MarqueeBroadcast{{ID: 1, Source: MarqueeSourceAdmin}}, nil
}
func (r *marqueeRepoStub) CreateAdmin(_ context.Context, input MarqueeBroadcastInput, _ int64) (*MarqueeBroadcast, error) {
	r.createCalls++
	r.lastInput = input
	return &MarqueeBroadcast{ID: 1, Source: MarqueeSourceAdmin, Content: input.Content}, nil
}
func (r *marqueeRepoStub) UpdateAdmin(_ context.Context, _ int64, input MarqueeBroadcastInput) (*MarqueeBroadcast, error) {
	r.updateCalls++
	r.lastInput = input
	if r.updateErr != nil {
		return nil, r.updateErr
	}
	return &MarqueeBroadcast{ID: 1, Source: MarqueeSourceAdmin, Content: input.Content}, nil
}
func (r *marqueeRepoStub) DeleteAdmin(context.Context, int64) (bool, error) {
	r.deleteCalls++
	return r.deleteResult, nil
}

func TestMarqueeServiceFeatureGateIsExactAndFailClosed(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		raw    string
		getErr error
		want   bool
	}{
		{name: "missing"},
		{name: "read error", getErr: errors.New("settings unavailable")},
		{name: "false", raw: "false"},
		{name: "uppercase", raw: "TRUE"},
		{name: "padded", raw: " true "},
		{name: "numeric", raw: "1"},
		{name: "exact true", raw: "true", want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := map[string]string{}
			if tc.raw != "" {
				values[SettingKeySubNexusMarqueeEnabled] = tc.raw
			}
			svc := NewMarqueeService(&marqueeRepoStub{}, &marqueeSettingRepoStub{values: values, getErr: tc.getErr})
			require.Equal(t, tc.want, svc.GetConfig(context.Background()).Enabled)
		})
	}
}

func TestMarqueeServiceClosedStateNeverTouchesBusinessRepository(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		values map[string]string
		getErr error
	}{
		{name: "missing", values: map[string]string{}},
		{name: "malformed", values: map[string]string{SettingKeySubNexusMarqueeEnabled: " true "}},
		{name: "read error", values: map[string]string{}, getErr: errors.New("settings unavailable")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &marqueeRepoStub{deleteResult: true}
			svc := NewMarqueeService(repo, &marqueeSettingRepoStub{values: tc.values, getErr: tc.getErr})
			ctx := context.Background()

			visible, err := svc.ListVisible(ctx, time.Now(), 12)
			require.NoError(t, err)
			require.False(t, visible.Enabled)
			require.Empty(t, visible.Items)
			adminItems, err := svc.ListAdmin(ctx)
			require.NoError(t, err)
			require.Empty(t, adminItems)
			_, err = svc.Create(ctx, MarqueeBroadcastInput{Content: "message"}, 3)
			require.ErrorIs(t, err, ErrMarqueeDisabled)
			_, err = svc.Update(ctx, 1, MarqueeBroadcastInput{Content: "message"})
			require.ErrorIs(t, err, ErrMarqueeDisabled)
			require.ErrorIs(t, svc.Delete(ctx, 1), ErrMarqueeDisabled)

			require.Zero(t, repo.listVisibleCalls)
			require.Zero(t, repo.listAdminCalls)
			require.Zero(t, repo.createCalls)
			require.Zero(t, repo.updateCalls)
			require.Zero(t, repo.deleteCalls)
		})
	}
}

func TestMarqueeServiceEnabledCRUDNormalizesAndValidatesInput(t *testing.T) {
	t.Parallel()
	repo := &marqueeRepoStub{deleteResult: true}
	settings := &marqueeSettingRepoStub{values: map[string]string{SettingKeySubNexusMarqueeEnabled: "true"}}
	svc := NewMarqueeService(repo, settings)
	ctx := context.Background()

	item, err := svc.Create(ctx, MarqueeBroadcastInput{Title: " title ", Content: " message ", Priority: 5}, 3)
	require.NoError(t, err)
	require.Equal(t, "title", repo.lastInput.Title)
	require.Equal(t, "message", item.Content)

	invalid := []MarqueeBroadcastInput{
		{Content: ""},
		{Content: strings.Repeat("x", marqueeContentMaxRunes+1)},
		{Title: strings.Repeat("x", marqueeTitleMaxRunes+1), Content: "message"},
		{Content: "message", Priority: -1},
		{Content: "message", Priority: marqueeMaxPriority + 1},
	}
	for _, input := range invalid {
		_, err := svc.Create(ctx, input, 3)
		require.Error(t, err)
	}
	require.Equal(t, 1, repo.createCalls)

	repo.updateErr = sql.ErrNoRows
	_, err = svc.Update(ctx, 99, MarqueeBroadcastInput{Content: "missing"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
	require.NoError(t, svc.Delete(ctx, 1))
}

func TestMarqueeServiceConfigWriteNotifiesOnlyAfterSuccess(t *testing.T) {
	t.Parallel()
	settings := &marqueeSettingRepoStub{values: map[string]string{}}
	svc := NewMarqueeService(&marqueeRepoStub{}, settings)
	notifications := 0
	svc.SetSettingsUpdatedNotifier(func() { notifications++ })

	cfg, err := svc.UpdateConfig(context.Background(), MarqueeConfig{Enabled: true})
	require.NoError(t, err)
	require.True(t, cfg.Enabled)
	require.Equal(t, "true", settings.sets[SettingKeySubNexusMarqueeEnabled])
	require.Equal(t, 1, notifications)

	settings.setErr = errors.New("write failed")
	_, err = svc.UpdateConfig(context.Background(), MarqueeConfig{})
	require.Error(t, err)
	require.Equal(t, 1, notifications)
}
