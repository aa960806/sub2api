package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestPublicInviteActivitiesFlagsFailClosedAndExposeValidatedChildren(t *testing.T) {
	cfg := DefaultInviteActivitiesConfig()
	cfg.Enabled = true
	cfg.InviteLotteryEnabled = true
	cfg.RechargeWheelEnabled = true
	cfg.InviteMilestoneEnabled = true
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)

	aggregate, lottery, wheel, milestone := publicInviteActivitiesFlags("true", string(raw))
	require.True(t, aggregate)
	require.True(t, lottery)
	require.True(t, wheel)
	require.True(t, milestone)

	for _, tc := range []struct {
		name       string
		rawEnabled string
		rawConfig  string
	}{
		{name: "missing config", rawEnabled: "true"},
		{name: "malformed config", rawEnabled: "true", rawConfig: "{"},
		{name: "uppercase switch", rawEnabled: "TRUE", rawConfig: string(raw)},
		{name: "spaced switch", rawEnabled: " true ", rawConfig: string(raw)},
		{name: "numeric switch", rawEnabled: "1", rawConfig: string(raw)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			aggregate, lottery, wheel, milestone := publicInviteActivitiesFlags(tc.rawEnabled, tc.rawConfig)
			require.False(t, aggregate)
			require.False(t, lottery)
			require.False(t, wheel)
			require.False(t, milestone)
		})
	}

	cfg.InviteLotteryEnabled = false
	cfg.RechargeWheelEnabled = true
	cfg.InviteMilestoneEnabled = false
	raw, err = json.Marshal(cfg)
	require.NoError(t, err)
	aggregate, lottery, wheel, milestone = publicInviteActivitiesFlags("true", string(raw))
	require.True(t, aggregate)
	require.False(t, lottery)
	require.True(t, wheel)
	require.False(t, milestone)
}

type inviteActivitiesSettingsStub struct {
	values map[string]string
	err    error
	set    map[string]string
}

func (s *inviteActivitiesSettingsStub) Get(context.Context, string) (*Setting, error) {
	return nil, errors.New("unexpected Get")
}
func (s *inviteActivitiesSettingsStub) GetValue(context.Context, string) (string, error) {
	return "", errors.New("unexpected GetValue")
}
func (s *inviteActivitiesSettingsStub) Set(context.Context, string, string) error {
	return errors.New("unexpected Set")
}
func (s *inviteActivitiesSettingsStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
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
func (s *inviteActivitiesSettingsStub) SetMultiple(_ context.Context, values map[string]string) error {
	s.set = make(map[string]string, len(values))
	for key, value := range values {
		s.set[key] = value
	}
	return nil
}
func (s *inviteActivitiesSettingsStub) GetAll(context.Context) (map[string]string, error) {
	return nil, errors.New("unexpected GetAll")
}
func (s *inviteActivitiesSettingsStub) Delete(context.Context, string) error {
	return errors.New("unexpected Delete")
}

type inviteActivitiesRepoStub struct {
	eligible       int
	qualified      int
	recharged      float64
	rewardCounts   map[string]int
	claimed        map[int]bool
	grantInserted  bool
	grantCalls     int
	readCalls      int
	qualifiedCalls int
}

func (r *inviteActivitiesRepoStub) CountEligibleInvitees(context.Context, int64) (int, error) {
	r.readCalls++
	return r.eligible, nil
}
func (r *inviteActivitiesRepoStub) CountQualifiedInvitees(context.Context, int64, float64) (int, error) {
	r.qualifiedCalls++
	return r.qualified, nil
}
func (r *inviteActivitiesRepoStub) SumCompletedRecharge(context.Context, int64) (float64, error) {
	r.readCalls++
	return r.recharged, nil
}
func (r *inviteActivitiesRepoStub) CountRewards(_ context.Context, _ int64, source string) (int, error) {
	r.readCalls++
	return r.rewardCounts[source], nil
}
func (r *inviteActivitiesRepoStub) ListClaimedMilestones(context.Context, int64, string) (map[int]bool, error) {
	r.readCalls++
	return r.claimed, nil
}
func (r *inviteActivitiesRepoStub) GrantReward(context.Context, int64, string, string, float64, string) (bool, error) {
	r.grantCalls++
	return r.grantInserted, nil
}

func inviteActivitiesEnabledSettings(config string) *inviteActivitiesSettingsStub {
	return &inviteActivitiesSettingsStub{values: map[string]string{
		SettingKeySubNexusInviteActivitiesEnabled: "true",
		SettingKeySubNexusInviteActivitiesConfig:  config,
	}}
}

func TestInviteActivitiesDisabledFailsClosedWithoutRepositoryAccess(t *testing.T) {
	repo := &inviteActivitiesRepoStub{eligible: 10}
	settings := &inviteActivitiesSettingsStub{values: map[string]string{}}
	svc := NewInviteActivitiesService(repo, settings)

	status, err := svc.GetInviteLotteryStatus(context.Background(), 7)
	require.NoError(t, err)
	require.False(t, status.Enabled)
	require.Equal(t, 0, repo.readCalls)

	_, err = svc.ClaimInviteLottery(context.Background(), 7)
	require.Equal(t, "INVITE_LOTTERY_DISABLED", infraerrors.Reason(err))
	require.Equal(t, 0, repo.grantCalls)
}

func TestInviteActivitiesSwitchAndPolicyAreStrictAndIndependent(t *testing.T) {
	valid := `{"enabled":false,"invite_lottery_enabled":true,"invite_lottery_prizes":[{"name":"one","amount":1,"probability":1}]}`
	for _, raw := range []string{"TRUE", "1", " true ", "yes"} {
		settings := inviteActivitiesEnabledSettings(valid)
		settings.values[SettingKeySubNexusInviteActivitiesEnabled] = raw
		if cfg := NewInviteActivitiesService(nil, settings).Config(context.Background()); cfg.Enabled {
			t.Fatalf("switch value %q unexpectedly enabled", raw)
		}
	}
	settings := inviteActivitiesEnabledSettings(valid)
	cfg := NewInviteActivitiesService(nil, settings).Config(context.Background())
	require.True(t, cfg.Enabled, "aggregate switch should control a valid independent policy")
	require.True(t, cfg.InviteLotteryEnabled)

	settings.values[SettingKeySubNexusInviteActivitiesConfig] = `{"invite_lottery_enabled":true,"invite_lottery_prizes":[{"name":"zero","amount":1,"probability":0}]}`
	cfg = NewInviteActivitiesService(nil, settings).Config(context.Background())
	require.False(t, cfg.Enabled, "zero-probability-only policy must fail closed")

	settings.values[SettingKeySubNexusInviteActivitiesConfig] = "{bad"
	cfg = NewInviteActivitiesService(nil, settings).Config(context.Background())
	require.False(t, cfg.Enabled, "malformed JSON must fail closed")
}

func TestInviteLotteryClaimUsesQualifiedInviteCountAndIsIdempotent(t *testing.T) {
	config := `{"invite_lottery_enabled":true,"invite_lottery_recharge_limit_enabled":true,"invite_lottery_recharge_threshold":10,"invite_lottery_prizes":[{"name":"fixed","amount":0.5,"probability":1}]}`
	settings := inviteActivitiesEnabledSettings(config)
	repo := &inviteActivitiesRepoStub{
		eligible:      4,
		qualified:     2,
		rewardCounts:  map[string]int{ActivitySourceInviteLottery: 1},
		grantInserted: true,
	}
	svc := NewInviteActivitiesService(repo, settings)

	status, err := svc.GetInviteLotteryStatus(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, 4, status.InvitedCount)
	require.Equal(t, 2, status.QualifiedInvitedCount)
	require.Equal(t, 1, status.RemainingChances)
	require.True(t, status.CanClaim)

	status, err = svc.ClaimInviteLottery(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, "fixed", status.Prize.Name)
	require.InDelta(t, 0.5, status.Prize.Amount, 0.000001)
	require.Equal(t, 1, repo.grantCalls)

	// A duplicate repository result is treated as an idempotent retry and does
	// not synthesize a second prize.
	repo.grantInserted = false
	_, err = svc.ClaimInviteLottery(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, 2, repo.grantCalls)
}

func TestRechargeWheelClaimUsesSingleAmountAndMultiplierAndMilestoneType(t *testing.T) {
	config := `{"recharge_wheel_enabled":true,"recharge_wheel_threshold":10,"recharge_wheel_amounts":[{"amount":2,"probability":1}],"recharge_wheel_multipliers":[{"multiplier":3,"probability":1}]}`
	repo := &inviteActivitiesRepoStub{recharged: 10, rewardCounts: map[string]int{ActivitySourceRechargeWheel: 0}, grantInserted: true}
	svc := NewInviteActivitiesService(repo, inviteActivitiesEnabledSettings(config))
	status, err := svc.ClaimRechargeWheel(context.Background(), 7)
	require.NoError(t, err)
	require.NotNil(t, status.Result)
	require.InDelta(t, 2, status.Result.Amount, 0.000001)
	require.InDelta(t, 3, status.Result.Multiplier, 0.000001)
	require.InDelta(t, 6, status.Result.Total, 0.000001)

	// Compile/runtime regression guard for the userID int64 contract on the
	// milestone method (the old draft accidentally used two int parameters).
	milestoneConfig := `{"invite_milestone_enabled":true,"invite_milestone_tiers":[{"invites":1,"reward":1}]}`
	milestoneRepo := &inviteActivitiesRepoStub{eligible: 1, claimed: map[int]bool{}, grantInserted: true}
	milestoneSvc := NewInviteActivitiesService(milestoneRepo, inviteActivitiesEnabledSettings(milestoneConfig))
	_, err = milestoneSvc.ClaimInviteMilestone(context.Background(), int64(7), 1)
	require.NoError(t, err)
}

type atomicInviteActivitiesRepoStub struct {
	inviteActivitiesRepoStub
	feature string
}

func (r *atomicInviteActivitiesRepoStub) GrantRewardIfEnabled(ctx context.Context, userID int64, source, period string, amount float64, note, feature string, _ InviteActivityClaimRule) (bool, error) {
	r.feature = feature
	return r.inviteActivitiesRepoStub.GrantReward(ctx, userID, source, period, amount, note)
}

func TestInviteActivityClaimUsesAtomicRepositoryGateWhenAvailable(t *testing.T) {
	config := `{"invite_milestone_enabled":true,"invite_milestone_tiers":[{"invites":1,"reward":1}]}`
	repo := &atomicInviteActivitiesRepoStub{
		inviteActivitiesRepoStub: inviteActivitiesRepoStub{eligible: 1, claimed: map[int]bool{}, grantInserted: true},
	}
	svc := NewInviteActivitiesService(repo, inviteActivitiesEnabledSettings(config))
	_, err := svc.ClaimInviteMilestone(context.Background(), 7, 1)
	require.NoError(t, err)
	require.Equal(t, "invite_milestone", repo.feature)
}
