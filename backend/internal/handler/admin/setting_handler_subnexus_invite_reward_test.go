package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSettingHandlerSubNexusInviteRewardsRoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &settingHandlerRepoStub{values: map[string]string{}}
	svc := service.NewSettingService(repo, &config.Config{Default: config.DefaultConfig{UserConcurrency: 5}})
	handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)

	body, err := json.Marshal(map[string]any{
		"subnexus_invite_rewards_enabled":         true,
		"subnexus_invite_reward_inviter_amount":   8.25,
		"subnexus_invite_reward_invitee_amount":   2.5,
		"subnexus_invite_reward_ip_limit_enabled": true,
		"subnexus_invite_reward_ip_daily_limit":   4,
	})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateSettings(c)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, "true", repo.values[service.SettingKeySubNexusInviteRewardsEnabled])
	require.Equal(t, "8.25000000", repo.values[service.SettingKeySubNexusInviteSignupRewardInviter])
	require.Equal(t, "2.50000000", repo.values[service.SettingKeySubNexusInviteSignupRewardInvitee])
	require.Equal(t, "true", repo.values[service.SettingKeySubNexusInviteSignupRewardIPLimit])
	require.Equal(t, "4", repo.values[service.SettingKeySubNexusInviteSignupRewardIPDaily])
	require.NotContains(t, repo.values, "affiliate_signup_reward_enabled")

	var result response.Response
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &result))
	data, ok := result.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, data["subnexus_invite_rewards_enabled"])
	require.Equal(t, 8.25, data["subnexus_invite_reward_inviter_amount"])
	require.Equal(t, 2.5, data["subnexus_invite_reward_invitee_amount"])
	require.Equal(t, true, data["subnexus_invite_reward_ip_limit_enabled"])
	require.Equal(t, float64(4), data["subnexus_invite_reward_ip_daily_limit"])
}

func TestSettingHandlerSubNexusInviteRewardsRejectsInvalidDailyLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &settingHandlerRepoStub{values: map[string]string{}}
	svc := service.NewSettingService(repo, &config.Config{Default: config.DefaultConfig{UserConcurrency: 5}})
	handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)

	body := []byte(`{"subnexus_invite_reward_ip_daily_limit":0}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateSettings(c)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Empty(t, repo.lastUpdates)
}
