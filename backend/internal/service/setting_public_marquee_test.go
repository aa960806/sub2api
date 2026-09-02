package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type marqueePublicSettingRepoStub struct{ values map[string]string }

func (r *marqueePublicSettingRepoStub) Get(context.Context, string) (*Setting, error) {
	return nil, errors.New("unexpected Get call")
}
func (r *marqueePublicSettingRepoStub) GetValue(context.Context, string) (string, error) {
	return "", errors.New("unexpected GetValue call")
}
func (r *marqueePublicSettingRepoStub) Set(context.Context, string, string) error {
	return errors.New("unexpected Set call")
}
func (r *marqueePublicSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}
func (r *marqueePublicSettingRepoStub) SetMultiple(context.Context, map[string]string) error {
	return errors.New("unexpected SetMultiple call")
}
func (r *marqueePublicSettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	return nil, errors.New("unexpected GetAll call")
}
func (r *marqueePublicSettingRepoStub) Delete(context.Context, string) error {
	return errors.New("unexpected Delete call")
}

func TestSettingServiceGetPublicSettingsMarqueeFailsClosed(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		raw  string
		want bool
	}{
		{name: "missing"},
		{name: "exact true", raw: "true", want: true},
		{name: "false", raw: "false"},
		{name: "uppercase", raw: "TRUE"},
		{name: "padded", raw: " true "},
		{name: "numeric", raw: "1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := map[string]string{}
			if tc.raw != "" {
				values[SettingKeySubNexusMarqueeEnabled] = tc.raw
			}
			settings, err := NewSettingService(&marqueePublicSettingRepoStub{values: values}, &config.Config{}).
				GetPublicSettings(context.Background())
			require.NoError(t, err)
			require.Equal(t, tc.want, settings.SubNexusMarqueeEnabled)
		})
	}
}
