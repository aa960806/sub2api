package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type inviteRewardSettingRepo struct {
	SettingRepository
	values      map[string]string
	multipleErr error
}

func (r *inviteRewardSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	value, ok := r.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}

func (r *inviteRewardSettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	if r.multipleErr != nil {
		return nil, r.multipleErr
	}
	values := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			values[key] = value
		}
	}
	return values, nil
}

type inviteRewardAffiliateRepo struct {
	AffiliateRepository
	bindResult bool
	bindCalls  int
	grantCalls int
	grantIP    string
	grantLimit bool
	grantDaily int
}

func (r *inviteRewardAffiliateRepo) EnsureUserAffiliate(_ context.Context, userID int64) (*AffiliateSummary, error) {
	return &AffiliateSummary{UserID: userID, AffCode: "SELF1234"}, nil
}

func (r *inviteRewardAffiliateRepo) GetAffiliateByCode(context.Context, string) (*AffiliateSummary, error) {
	return &AffiliateSummary{UserID: 41, AffCode: "INVITER1"}, nil
}

func (r *inviteRewardAffiliateRepo) BindInviter(context.Context, int64, int64) (bool, error) {
	r.bindCalls++
	return r.bindResult, nil
}

func (r *inviteRewardAffiliateRepo) GrantSignupReward(_ context.Context, _, _ int64, _, _ float64, clientIP string, ipLimitEnabled bool, ipDailyLimit int) (AffiliateSignupRewardResult, error) {
	r.grantCalls++
	r.grantIP = clientIP
	r.grantLimit = ipLimitEnabled
	r.grantDaily = ipDailyLimit
	return AffiliateSignupRewardResult{Applied: true}, nil
}

func enabledInviteRewardSettings() map[string]string {
	return map[string]string{
		SettingKeyAffiliateEnabled:                  "true",
		SettingKeySubNexusInviteRewardsEnabled:      "true",
		SettingKeySubNexusInviteSignupRewardInviter: "8.25",
		SettingKeySubNexusInviteSignupRewardInvitee: "2.5",
		SettingKeySubNexusInviteSignupRewardIPLimit: "true",
		SettingKeySubNexusInviteSignupRewardIPDaily: "3",
	}
}

func newInviteRewardAffiliateService(repo AffiliateRepository, settings *inviteRewardSettingRepo) *AffiliateService {
	return NewAffiliateService(repo, NewSettingService(settings, &config.Config{}), nil, nil)
}

func TestAffiliateSignupRewardDisabledKeepsUpstreamBinding(t *testing.T) {
	repo := &inviteRewardAffiliateRepo{bindResult: true}
	settings := &inviteRewardSettingRepo{values: map[string]string{
		SettingKeyAffiliateEnabled:             "true",
		SettingKeySubNexusInviteRewardsEnabled: "false",
	}}

	err := newInviteRewardAffiliateService(repo, settings).BindInviterByCode(context.Background(), 99, "INVITER1")

	require.NoError(t, err)
	require.Equal(t, 1, repo.bindCalls)
	require.Zero(t, repo.grantCalls)
}

func TestAffiliateSignupRewardUsesTrustedIPContext(t *testing.T) {
	repo := &inviteRewardAffiliateRepo{bindResult: true}
	settings := &inviteRewardSettingRepo{values: enabledInviteRewardSettings()}
	ctx := WithAffiliateSignupIP(context.Background(), "203.0.113.9")

	err := newInviteRewardAffiliateService(repo, settings).BindInviterByCode(ctx, 99, "INVITER1")

	require.NoError(t, err)
	require.Equal(t, 1, repo.grantCalls)
	require.Equal(t, "203.0.113.9", repo.grantIP)
	require.True(t, repo.grantLimit)
	require.Equal(t, 3, repo.grantDaily)
}

func TestAffiliateSignupRewardIPLimitFailsClosedWithoutTrustedIP(t *testing.T) {
	repo := &inviteRewardAffiliateRepo{bindResult: true}
	settings := &inviteRewardSettingRepo{values: enabledInviteRewardSettings()}

	err := newInviteRewardAffiliateService(repo, settings).BindInviterByCode(context.Background(), 99, "INVITER1")

	require.NoError(t, err)
	require.Equal(t, 1, repo.bindCalls)
	require.Zero(t, repo.grantCalls)
}

func TestAffiliateSignupRewardSettingsFailureFailsClosed(t *testing.T) {
	repo := &inviteRewardAffiliateRepo{bindResult: true}
	settings := &inviteRewardSettingRepo{
		values:      map[string]string{SettingKeyAffiliateEnabled: "true"},
		multipleErr: errors.New("settings unavailable"),
	}

	err := newInviteRewardAffiliateService(repo, settings).BindInviterByCode(context.Background(), 99, "INVITER1")

	require.NoError(t, err)
	require.Equal(t, 1, repo.bindCalls)
	require.Zero(t, repo.grantCalls)
}

func TestAffiliateSignupRewardLegacySettingsCannotEnableMigration(t *testing.T) {
	repo := &inviteRewardAffiliateRepo{bindResult: true}
	settings := &inviteRewardSettingRepo{values: map[string]string{
		SettingKeyAffiliateEnabled:        "true",
		"affiliate_signup_reward_enabled": "true",
		"affiliate_signup_reward_inviter": "100",
	}}

	err := newInviteRewardAffiliateService(repo, settings).BindInviterByCode(context.Background(), 99, "INVITER1")

	require.NoError(t, err)
	require.Equal(t, 1, repo.bindCalls)
	require.Zero(t, repo.grantCalls)
}

type upstreamOnlyAffiliateRepo struct {
	AffiliateRepository
	bindResult bool
}

func (r *upstreamOnlyAffiliateRepo) EnsureUserAffiliate(_ context.Context, userID int64) (*AffiliateSummary, error) {
	return &AffiliateSummary{UserID: userID, AffCode: "SELF1234"}, nil
}

func (r *upstreamOnlyAffiliateRepo) GetAffiliateByCode(context.Context, string) (*AffiliateSummary, error) {
	return &AffiliateSummary{UserID: 41, AffCode: "INVITER1"}, nil
}

func (r *upstreamOnlyAffiliateRepo) BindInviter(context.Context, int64, int64) (bool, error) {
	return r.bindResult, nil
}

func TestAffiliateSignupRewardOptionalRepositoryCapabilityDoesNotBreakBinding(t *testing.T) {
	repo := &upstreamOnlyAffiliateRepo{bindResult: true}
	settings := &inviteRewardSettingRepo{values: enabledInviteRewardSettings()}
	ctx := WithAffiliateSignupIP(context.Background(), "203.0.113.9")

	err := newInviteRewardAffiliateService(repo, settings).BindInviterByCode(ctx, 99, "INVITER1")

	require.NoError(t, err)
}
