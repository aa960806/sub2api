//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type settingPublicRepoStub struct {
	values map[string]string
	err    error
}

func (s *settingPublicRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	panic("unexpected Get call")
}

func (s *settingPublicRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	panic("unexpected GetValue call")
}

func (s *settingPublicRepoStub) Set(ctx context.Context, key, value string) error {
	panic("unexpected Set call")
}

func (s *settingPublicRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (s *settingPublicRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	panic("unexpected SetMultiple call")
}

func (s *settingPublicRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (s *settingPublicRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
}

func TestSettingService_GetPublicSettings_ExposesRegistrationEmailSuffixWhitelist(t *testing.T) {
	repo := &settingPublicRepoStub{
		values: map[string]string{
			SettingKeyRegistrationEnabled:              "true",
			SettingKeyEmailVerifyEnabled:               "true",
			SettingKeyRegistrationEmailSuffixWhitelist: `["@EXAMPLE.com"," @foo.bar ","*.EDU.CN","@invalid_domain",""]`,
		},
	}
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"@example.com", "@foo.bar", "*.edu.cn"}, settings.RegistrationEmailSuffixWhitelist)
}

func TestSettingService_GetPublicSettings_ExposesTablePreferences(t *testing.T) {
	repo := &settingPublicRepoStub{
		values: map[string]string{
			SettingKeyTableDefaultPageSize: "50",
			SettingKeyTablePageSizeOptions: "[20,50,100]",
		},
	}
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, 50, settings.TableDefaultPageSize)
	require.Equal(t, []int{20, 50, 100}, settings.TablePageSizeOptions)
}

func TestSettingService_GetPublicSettings_ExposesCompactHomeEnabled(t *testing.T) {
	repo := &settingPublicRepoStub{
		values: map[string]string{
			SettingKeyCompactHomeEnabled: "true",
		},
	}
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetPublicSettings(context.Background())

	require.NoError(t, err)
	require.True(t, settings.CompactHomeEnabled)

	missingSettings, err := NewSettingService(&settingPublicRepoStub{values: map[string]string{}}, &config.Config{}).
		GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.False(t, missingSettings.CompactHomeEnabled)
}

func TestSettingService_ChannelMonitorHideThroughputDefaultsToPrivate(t *testing.T) {
	missing := NewSettingService(&settingPublicRepoStub{values: map[string]string{}}, &config.Config{}).GetChannelMonitorRuntime(context.Background())
	require.False(t, missing.Enabled, "missing enablement must fail closed")
	require.True(t, missing.HideThroughput)
	public, err := NewSettingService(&settingPublicRepoStub{values: map[string]string{}}, &config.Config{}).GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.True(t, public.ChannelMonitorHideThroughput)

	for _, value := range []string{"false", "0", "off", "disabled"} {
		runtime := NewSettingService(&settingPublicRepoStub{values: map[string]string{
			SettingKeyChannelMonitorEnabled:        "true",
			SettingKeyChannelMonitorHideThroughput: value,
		}}, &config.Config{}).GetChannelMonitorRuntime(context.Background())
		require.False(t, runtime.HideThroughput, "value=%q", value)
	}
}

func TestSettingService_ChannelMonitorRuntimeFailsClosedOnReadOrModeError(t *testing.T) {
	readFailed := NewSettingService(&settingPublicRepoStub{err: errors.New("settings unavailable")}, &config.Config{}).
		GetChannelMonitorRuntime(context.Background())
	require.False(t, readFailed.Enabled)

	base := map[string]string{SettingKeyChannelMonitorEnabled: "true"}
	for _, rawMode := range []string{"garbage", "V9", " true "} {
		values := make(map[string]string, len(base)+1)
		for key, value := range base {
			values[key] = value
		}
		values[SettingKeyChannelMonitorMode] = rawMode
		runtime := NewSettingService(&settingPublicRepoStub{values: values}, &config.Config{}).
			GetChannelMonitorRuntime(context.Background())
		require.False(t, runtime.Enabled, "invalid mode %q must fail closed", rawMode)
	}
}

func TestSettingService_GetPublicSettings_ChannelMonitorInvalidModeFailsClosed(t *testing.T) {
	for _, rawMode := range []string{"garbage", "V9", " true "} {
		settings, err := NewSettingService(&settingPublicRepoStub{values: map[string]string{
			SettingKeyChannelMonitorEnabled: "true",
			SettingKeyChannelMonitorMode:    rawMode,
		}}, &config.Config{}).GetPublicSettings(context.Background())
		require.NoError(t, err)
		require.False(t, settings.ChannelMonitorEnabled, "invalid mode %q must not expose an enabled monitor", rawMode)
	}
}

func TestSettingService_ChannelMonitorShowQuotaFailsClosed(t *testing.T) {
	// 缺省（迁移插入 'false' / 老库无行）一律不展示。
	missingRuntime := NewSettingService(&settingPublicRepoStub{values: map[string]string{}}, &config.Config{}).GetChannelMonitorRuntime(context.Background())
	require.False(t, missingRuntime.ShowQuota)
	missingPublic, err := NewSettingService(&settingPublicRepoStub{values: map[string]string{}}, &config.Config{}).
		GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.False(t, missingPublic.ChannelMonitorShowQuota)

	// 仅字面 "true" 视为开启；其余值（含异常值）fail-closed。
	runtime := NewSettingService(&settingPublicRepoStub{values: map[string]string{
		SettingKeyChannelMonitorEnabled:   "true",
		SettingKeyChannelMonitorShowQuota: "true",
	}}, &config.Config{}).GetChannelMonitorRuntime(context.Background())
	require.True(t, runtime.ShowQuota)

	for _, value := range []string{"false", "TRUE", "1", "yes", "on", "garbage"} {
		rt := NewSettingService(&settingPublicRepoStub{values: map[string]string{
			SettingKeyChannelMonitorEnabled:   "true",
			SettingKeyChannelMonitorShowQuota: value,
		}}, &config.Config{}).GetChannelMonitorRuntime(context.Background())
		require.False(t, rt.ShowQuota, "value=%q", value)
	}
}

func TestSettingService_GetPublicSettings_ExposesForceEmailOnThirdPartySignup(t *testing.T) {
	repo := &settingPublicRepoStub{
		values: map[string]string{
			SettingKeyForceEmailOnThirdPartySignup: "true",
		},
	}
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.ForceEmailOnThirdPartySignup)
}

func TestSettingService_GetPublicSettings_ExposesAllowUserViewErrorRequests(t *testing.T) {
	repo := &settingPublicRepoStub{
		values: map[string]string{
			SettingKeyAllowUserViewErrorRequests: "true",
		},
	}
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.AllowUserViewErrorRequests)
}

func TestSettingService_GetPublicSettings_SubNexusActivityCenterFailsClosed(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		raw  string
		want bool
	}{
		{name: "missing", want: false},
		{name: "enabled", raw: "true", want: true},
		{name: "disabled", raw: "false", want: false},
		{name: "uppercase is invalid", raw: "TRUE", want: false},
		{name: "numeric is invalid", raw: "1", want: false},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			values := map[string]string{}
			if tc.name != "missing" {
				values[SettingKeySubNexusActivityCenterEnabled] = tc.raw
			}
			settings, err := NewSettingService(&settingPublicRepoStub{values: values}, &config.Config{}).
				GetPublicSettings(context.Background())
			require.NoError(t, err)
			require.Equal(t, tc.want, settings.SubNexusActivityCenterEnabled)
		})
	}
}

func TestSettingService_GetPublicSettings_SubNexusLeaderboardFailsClosed(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		raw  string
		want bool
	}{
		{name: "missing", want: false},
		{name: "enabled", raw: "true", want: true},
		{name: "disabled", raw: "false", want: false},
		{name: "uppercase is invalid", raw: "TRUE", want: false},
		{name: "numeric is invalid", raw: "1", want: false},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			values := map[string]string{}
			if tc.name != "missing" {
				values[SettingKeySubNexusLeaderboardEnabled] = tc.raw
			}
			settings, err := NewSettingService(&settingPublicRepoStub{values: values}, &config.Config{}).
				GetPublicSettings(context.Background())
			require.NoError(t, err)
			require.Equal(t, tc.want, settings.SubNexusLeaderboardEnabled)
		})
	}
}

func TestSettingService_GetPublicSettings_BattlePassFailsClosed(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		raw  string
		want bool
	}{
		{name: "missing", want: false},
		{name: "enabled", raw: "true", want: true},
		{name: "disabled", raw: "false", want: false},
		{name: "uppercase is invalid", raw: "TRUE", want: false},
		{name: "numeric is invalid", raw: "1", want: false},
		{name: "padded is invalid", raw: " true ", want: false},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			values := map[string]string{}
			if tc.name != "missing" {
				values[SettingKeyBattlePassEnabled] = tc.raw
			}
			settings, err := NewSettingService(&settingPublicRepoStub{values: values}, &config.Config{}).
				GetPublicSettings(context.Background())
			require.NoError(t, err)
			require.Equal(t, tc.want, settings.BattlePassEnabled)
		})
	}
}

func TestSettingService_GetPublicSettings_FirstRechargeAndStudentBenefitFailClosed(t *testing.T) {
	t.Parallel()
	firstConfig := `{"enabled":true,"price":9.9,"credited_amount":12,"ratio":1.2121}`
	studentConfig := `{"enabled":true,"bonus_rate":0.05,"min_recharge_amount":10,"per_order_cap":100}`
	settings, err := NewSettingService(&settingPublicRepoStub{values: map[string]string{
		SettingKeySubNexusFirstRechargeEnabled:          "true",
		SettingKeySubNexusFirstRechargeConfig:           firstConfig,
		SettingKeySubNexusStudentRechargeBenefitEnabled: "true",
		SettingKeyStudentRechargeBenefitConfig:          studentConfig,
	}}, &config.Config{}).GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.SubNexusFirstRechargeEnabled)
	require.True(t, settings.SubNexusStudentRechargeBenefitEnabled)

	for _, tc := range []struct {
		name    string
		values  map[string]string
		first   bool
		student bool
	}{
		{name: "missing policies", values: map[string]string{
			SettingKeySubNexusFirstRechargeEnabled:          "true",
			SettingKeySubNexusStudentRechargeBenefitEnabled: "true",
		}},
		{name: "non canonical gates", values: map[string]string{
			SettingKeySubNexusFirstRechargeEnabled:          "TRUE",
			SettingKeySubNexusFirstRechargeConfig:           firstConfig,
			SettingKeySubNexusStudentRechargeBenefitEnabled: "1",
			SettingKeyStudentRechargeBenefitConfig:          studentConfig,
		}},
		{name: "legacy policies disabled", values: map[string]string{
			SettingKeySubNexusFirstRechargeEnabled:          "true",
			SettingKeySubNexusFirstRechargeConfig:           `{"enabled":false,"price":9.9,"credited_amount":12,"ratio":1.2}`,
			SettingKeySubNexusStudentRechargeBenefitEnabled: "true",
			SettingKeyStudentRechargeBenefitConfig:          `{"enabled":false,"bonus_rate":0.05,"min_recharge_amount":10,"per_order_cap":100}`,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NewSettingService(&settingPublicRepoStub{values: tc.values}, &config.Config{}).
				GetPublicSettings(context.Background())
			require.NoError(t, err)
			require.Equal(t, tc.first, got.SubNexusFirstRechargeEnabled)
			require.Equal(t, tc.student, got.SubNexusStudentRechargeBenefitEnabled)
		})
	}
}

func TestSettingService_GetPublicSettings_InvoiceFlagValidatesConfig(t *testing.T) {
	t.Parallel()
	valid := `{"enabled":true,"min_amount":0.01,"max_amount":0,"application_days":30,"max_orders_per_request":10,"item_name":"service","max_file_size_mb":10}`
	for _, tc := range []struct {
		name    string
		raw     string
		rollout string
		want    bool
	}{
		{name: "missing", want: false},
		{name: "missing rollout", raw: valid, want: false},
		{name: "valid enabled", raw: valid, rollout: "true", want: true},
		{name: "non canonical rollout", raw: valid, rollout: "TRUE", want: false},
		{name: "malformed", raw: "{", rollout: "true", want: false},
		{name: "invalid bounds", raw: `{"enabled":true,"min_amount":-1}`, rollout: "true", want: false},
		{name: "wrong enabled type", raw: `{"enabled":"true"}`, rollout: "true", want: false},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			values := map[string]string{}
			if tc.raw != "" {
				values[SettingKeyInvoiceConfig] = tc.raw
			}
			if tc.rollout != "" {
				values[SettingKeySubNexusInvoiceEnabled] = tc.rollout
			}
			settings, err := NewSettingService(&settingPublicRepoStub{values: values}, &config.Config{}).
				GetPublicSettings(context.Background())
			require.NoError(t, err)
			require.Equal(t, tc.want, settings.InvoiceEnabled)
		})
	}
}

func TestSettingServiceNotifySettingsUpdatedCallsRegisteredConsumers(t *testing.T) {
	t.Parallel()
	svc := NewSettingService(&settingPublicRepoStub{}, &config.Config{})
	var calls int
	svc.SetOnUpdateCallback(func() { calls++ })
	svc.NotifySettingsUpdated()
	require.Equal(t, 1, calls)
}

func TestSettingService_GetPublicSettings_ExposesWeChatOAuthModeCapabilities(t *testing.T) {
	svc := NewSettingService(&settingPublicRepoStub{
		values: map[string]string{
			SettingKeyWeChatConnectEnabled:             "true",
			SettingKeyWeChatConnectAppID:               "wx-mp-app",
			SettingKeyWeChatConnectAppSecret:           "wx-mp-secret",
			SettingKeyWeChatConnectMode:                "mp",
			SettingKeyWeChatConnectScopes:              "snsapi_base",
			SettingKeyWeChatConnectOpenEnabled:         "true",
			SettingKeyWeChatConnectMPEnabled:           "true",
			SettingKeyWeChatConnectRedirectURL:         "https://api.example.com/api/v1/auth/oauth/wechat/callback",
			SettingKeyWeChatConnectFrontendRedirectURL: "/auth/wechat/callback",
		},
	}, &config.Config{})

	settings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.WeChatOAuthEnabled)
	require.True(t, settings.WeChatOAuthOpenEnabled)
	require.True(t, settings.WeChatOAuthMPEnabled)
}

func TestSettingService_GetPublicSettings_DoesNotExposeMobileOnlyWeChatAsWebOAuthAvailable(t *testing.T) {
	svc := NewSettingService(&settingPublicRepoStub{
		values: map[string]string{
			SettingKeyWeChatConnectEnabled:             "true",
			SettingKeyWeChatConnectMobileEnabled:       "true",
			SettingKeyWeChatConnectMode:                "mobile",
			SettingKeyWeChatConnectMobileAppID:         "wx-mobile-app",
			SettingKeyWeChatConnectMobileAppSecret:     "wx-mobile-secret",
			SettingKeyWeChatConnectFrontendRedirectURL: "/auth/wechat/callback",
		},
	}, &config.Config{})

	settings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.False(t, settings.WeChatOAuthEnabled)
	require.False(t, settings.WeChatOAuthOpenEnabled)
	require.False(t, settings.WeChatOAuthMPEnabled)
	require.True(t, settings.WeChatOAuthMobileEnabled)
}

func TestSettingService_GetPublicSettings_FallsBackToConfigForWeChatOAuthCapabilities(t *testing.T) {
	svc := NewSettingService(&settingPublicRepoStub{values: map[string]string{}}, &config.Config{
		WeChat: config.WeChatConnectConfig{
			Enabled:             true,
			OpenEnabled:         true,
			OpenAppID:           "wx-open-config",
			OpenAppSecret:       "wx-open-secret",
			FrontendRedirectURL: "/auth/wechat/config-callback",
		},
	})

	settings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.WeChatOAuthEnabled)
	require.True(t, settings.WeChatOAuthOpenEnabled)
	require.False(t, settings.WeChatOAuthMPEnabled)
	require.False(t, settings.WeChatOAuthMobileEnabled)
}
