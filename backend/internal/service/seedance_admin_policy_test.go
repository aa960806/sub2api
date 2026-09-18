//go:build unit

package service

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSeedanceAccountIsolation(t *testing.T) {
	a := &Account{Platform: PlatformSeedance, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": " key ", "base_url": "https://example.com/seedance/"}}
	require.True(t, a.IsSeedance())
	require.False(t, a.IsOpenAICompatible())
	require.False(t, IsUpstreamBillingProbeIdentity(a.Platform, a.Type))
	require.False(t, isConcreteRequestPlatform(a.Platform), "Seedance must not enter composite scheduling")
	platforms := schedulerSnapshotPlatforms()
	require.Contains(t, platforms[:], PlatformSeedance, "Seedance scheduler snapshots must be rebuilt with other platforms")
	require.Equal(t, "key", a.GetSeedanceAPIKey())
	require.Equal(t, "https://example.com/seedance", a.GetSeedanceBaseURL())
	a.Type = AccountTypeOAuth
	require.Empty(t, a.GetSeedanceAPIKey())
}

func TestSeedanceAccountPolicyRejectsUnsupportedCredentials(t *testing.T) {
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeUpstream, AccountTypeSetupToken, AccountTypeBedrock} {
		_, err := buildAccountForCreate(&CreateAccountInput{Platform: PlatformSeedance, Type: accountType, Credentials: map[string]any{"api_key": "test"}}, nil)
		require.ErrorContains(t, err, "only supports API key")
	}
	for _, base := range []string{"http://example.com", "https://user:secret@example.com", "https://example.com?key=secret", "file:///tmp"} {
		require.Error(t, validateSeedanceAccount(PlatformSeedance, AccountTypeAPIKey, map[string]any{"api_key": "test", "base_url": base}))
	}
	require.Error(t, validateSeedanceAccount(PlatformSeedance, AccountTypeAPIKey, nil))
	require.NoError(t, validateSeedanceAccount(PlatformOpenAI, AccountTypeOAuth, nil), "existing platform policy is untouched")
	require.NoError(t, validateSeedanceAccount(PlatformSeedance, AccountTypeAPIKey, map[string]any{"api_key": "test", "base_url": "https://example.com/seedance"}))
	require.Error(t, validateSeedanceAccount(PlatformSeedance, AccountTypeAPIKey, map[string]any{"api_key": "test", "model_mapping": map[string]any{"seedance2.0": "seedance2.0mini"}}))
	require.NoError(t, validateSeedanceAccount(PlatformSeedance, AccountTypeAPIKey, map[string]any{"api_key": "test", "model_mapping": map[string]any{"seedance2.0": "seedance2.0"}}))
	for _, multiplier := range []float64{0, 2, math.NaN(), math.Inf(1)} {
		require.Error(t, validateSeedanceAccountMultiplier(PlatformSeedance, &multiplier))
	}
	multiplier := 1.0
	require.NoError(t, validateSeedanceAccountMultiplier(PlatformSeedance, &multiplier))
	require.NoError(t, validateSeedanceAccountMultiplier(PlatformSeedance, nil))
}

func TestSeedanceAccountQuotaDoesNotOfferUnreservedBudgets(t *testing.T) {
	for _, key := range []string{"quota_limit", "quota_daily_limit", "quota_weekly_limit"} {
		extra := map[string]any{key: 10.0}
		require.True(t, containsSeedanceAccountQuota(extra))
		require.ErrorContains(t, validateSeedanceAccountQuota(PlatformSeedance, extra), "API key limits")
		require.NoError(t, validateSeedanceAccountQuota(PlatformOpenAI, extra))
		extra[key] = 0.0
		require.NoError(t, validateSeedanceAccountQuota(PlatformSeedance, extra), "zero permits clearing an old limit")
	}
	require.NoError(t, validateSeedanceAccountQuota(PlatformSeedance, nil))
}

func TestSeedanceGroupsRejectSubscriptionAndRateMultiplier(t *testing.T) {
	s := &adminServiceImpl{}
	_, err := s.CreateGroup(context.Background(), &CreateGroupInput{Platform: PlatformSeedance, RateMultiplier: 1, SubscriptionType: SubscriptionTypeSubscription})
	require.ErrorContains(t, err, "balance billing")
	_, err = s.CreateGroup(context.Background(), &CreateGroupInput{Platform: PlatformSeedance, RateMultiplier: 2})
	require.ErrorContains(t, err, "must be 1")
	require.NoError(t, validateSeedanceGroup(PlatformSeedance, SubscriptionTypeStandard, 1))
	require.NoError(t, validateSeedanceGroup(PlatformOpenAI, SubscriptionTypeSubscription, 2))
}

func TestSeedanceGroupUpdateCannotChangeExistingBillingContract(t *testing.T) {
	for _, platform := range []string{PlatformSeedance, PlatformOpenAI} {
		group := &Group{ID: 5, Platform: platform, RateMultiplier: 1, SubscriptionType: SubscriptionTypeStandard}
		repo := &groupRepoStubForAdmin{getByID: group}
		s := &adminServiceImpl{groupRepo: repo}
		target := PlatformSeedance
		if platform == PlatformSeedance {
			target = PlatformOpenAI
		}
		_, err := s.UpdateGroup(context.Background(), group.ID, &UpdateGroupInput{Platform: target})
		require.ErrorContains(t, err, "dedicated Seedance group")
		require.Nil(t, repo.updated)
	}
	repo := &groupRepoStubForAdmin{getByID: &Group{ID: 5, Platform: PlatformSeedance, RateMultiplier: 1, SubscriptionType: SubscriptionTypeStandard}}
	s := &adminServiceImpl{groupRepo: repo}
	_, err := s.UpdateGroup(context.Background(), 5, &UpdateGroupInput{SubscriptionType: SubscriptionTypeSubscription})
	require.ErrorContains(t, err, "balance billing")
	require.Nil(t, repo.updated)
}

func TestSeedanceAccountCannotBindTextOrCompositeGroup(t *testing.T) {
	a := &Account{Platform: PlatformSeedance, Type: AccountTypeAPIKey}
	for _, platform := range []string{PlatformSeedance, PlatformOpenAI, PlatformComposite} {
		s := &adminServiceImpl{groupRepo: &groupRepoStubForAdmin{getByID: &Group{ID: 1, Platform: platform}}}
		err := s.validateSeedanceAccountGroups(context.Background(), a, []int64{1})
		if platform == PlatformSeedance {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
}

func TestSeedancePricingRequiresFiniteFixedPerRequestPrice(t *testing.T) {
	price := 1.25
	valid := ChannelModelPricing{Platform: PlatformSeedance, Models: []string{"seedance2.0mini"}, BillingMode: BillingModePerRequest, PerRequestPrice: &price}
	require.NoError(t, validatePricingEntries([]ChannelModelPricing{valid}))
	for _, amount := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		entry := valid.Clone()
		entry.PerRequestPrice = &amount
		require.Error(t, validatePricingEntries([]ChannelModelPricing{entry}))
	}
	for _, mode := range []BillingMode{BillingModeToken, BillingModeImage, BillingModeVideo, ""} {
		entry := valid.Clone()
		entry.BillingMode = mode
		require.ErrorContains(t, validatePricingEntries([]ChannelModelPricing{entry}), "fixed per-request price")
	}
	entry := valid.Clone()
	entry.Intervals = []PricingInterval{{TierLabel: "5s", PerRequestPrice: &price}}
	require.ErrorContains(t, validatePricingEntries([]ChannelModelPricing{entry}), "fixed per-request price")
	entry = valid.Clone()
	entry.Platform = ""
	entry.BillingMode = BillingModeToken
	_, err := normalizeGroupModelPricing(PlatformSeedance, []ChannelModelPricing{entry})
	require.ErrorContains(t, err, "fixed per-request price", "omitted pricing platform inherits Seedance before validation")
	entry.Platform = PlatformOpenAI
	require.NoError(t, validatePricingEntries([]ChannelModelPricing{entry}), "text pricing rules are unchanged")
}
