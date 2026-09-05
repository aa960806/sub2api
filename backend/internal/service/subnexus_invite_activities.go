package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// Invite activity sources intentionally match the legacy activity_reward_logs
// values.  Keeping the source/period identity allows a same-database cutover
// to recognize rewards already granted by the old binary.
const (
	ActivitySourceInviteLottery   = "invite_lottery"
	ActivitySourceRechargeWheel   = "recharge_wheel"
	ActivitySourceInviteMilestone = "invite_milestone"

	SettingKeySubNexusInviteActivitiesEnabled = "subnexus_invite_activities_enabled"
	SettingKeySubNexusInviteActivitiesConfig  = "subnexus_invite_activities_config"

	inviteActivitiesMaxPrizeItems = 50
	inviteActivitiesMaxTiers      = 20
	inviteActivitiesMaxMoney      = 1_000_000_000.0
)

var (
	ErrInviteActivitiesDisabled = infraerrors.Forbidden(
		"SUBNEXUS_INVITE_ACTIVITIES_DISABLED",
		"invite activities are disabled",
	)
	ErrInviteLotteryDisabled = infraerrors.Forbidden(
		"INVITE_LOTTERY_DISABLED",
		"invite lottery is disabled",
	)
	ErrRechargeWheelDisabled = infraerrors.Forbidden(
		"RECHARGE_WHEEL_DISABLED",
		"recharge wheel is disabled",
	)
	ErrInviteMilestoneDisabled = infraerrors.Forbidden(
		"INVITE_MILESTONE_DISABLED",
		"invite milestone is disabled",
	)
)

// ErrInviteActivityDisabledForFeature maps a repository-side gate rejection
// back to the same public error used by the preflight service check.
func ErrInviteActivityDisabledForFeature(feature string) error {
	switch feature {
	case "invite_lottery":
		return ErrInviteLotteryDisabled
	case "recharge_wheel":
		return ErrRechargeWheelDisabled
	case "invite_milestone":
		return ErrInviteMilestoneDisabled
	default:
		return ErrInviteActivitiesDisabled
	}
}

// InviteLotteryPrize is the administrator-facing prize policy. Probability
// is a relative weight; it is never returned by a user-facing status API.
type InviteLotteryPrize struct {
	Name        string  `json:"name"`
	Amount      float64 `json:"amount"`
	Probability float64 `json:"probability"`
}

type InviteLotteryPrizePublic struct {
	Name   string  `json:"name"`
	Amount float64 `json:"amount"`
}

type RechargeWheelAmount struct {
	Amount      float64 `json:"amount"`
	Probability float64 `json:"probability"`
}

type RechargeWheelMultiplier struct {
	Multiplier  float64 `json:"multiplier"`
	Probability float64 `json:"probability"`
}

type RechargeWheelAmountPublic struct {
	Amount float64 `json:"amount"`
}

type RechargeWheelMultiplierPublic struct {
	Multiplier float64 `json:"multiplier"`
}

type InviteMilestoneTier struct {
	Invites int     `json:"invites"`
	Reward  float64 `json:"reward"`
}

// InviteActivitiesConfig is kept as one JSON policy so all three activities
// can be updated atomically.  The aggregate switch and each child switch are
// both required at runtime; this gives operators a global emergency stop and
// independent staged rollout controls.
type InviteActivitiesConfig struct {
	Enabled bool `json:"enabled"`

	InviteLotteryEnabled              bool                 `json:"invite_lottery_enabled"`
	InviteLotteryPrizes               []InviteLotteryPrize `json:"invite_lottery_prizes"`
	InviteLotteryRechargeLimitEnabled bool                 `json:"invite_lottery_recharge_limit_enabled"`
	InviteLotteryRechargeThreshold    float64              `json:"invite_lottery_recharge_threshold"`

	RechargeWheelEnabled     bool                      `json:"recharge_wheel_enabled"`
	RechargeWheelThreshold   float64                   `json:"recharge_wheel_threshold"`
	RechargeWheelAmounts     []RechargeWheelAmount     `json:"recharge_wheel_amounts"`
	RechargeWheelMultipliers []RechargeWheelMultiplier `json:"recharge_wheel_multipliers"`

	InviteMilestoneEnabled              bool                  `json:"invite_milestone_enabled"`
	InviteMilestoneTiers                []InviteMilestoneTier `json:"invite_milestone_tiers"`
	InviteMilestoneRechargeLimitEnabled bool                  `json:"invite_milestone_recharge_limit_enabled"`
	InviteMilestoneRechargeThreshold    float64               `json:"invite_milestone_recharge_threshold"`
}

type InviteLotteryStatus struct {
	Enabled                  bool                       `json:"enabled"`
	InvitedCount             int                        `json:"invited_count"`
	QualifiedInvitedCount    int                        `json:"qualified_invited_count"`
	UsedChances              int                        `json:"used_chances"`
	RemainingChances         int                        `json:"remaining_chances"`
	LockedChances            int                        `json:"locked_chances"`
	RechargeLimitEnabled     bool                       `json:"recharge_limit_enabled"`
	InviteeRechargeThreshold float64                    `json:"invitee_recharge_threshold"`
	CanClaim                 bool                       `json:"can_claim"`
	Prize                    *InviteLotteryPrizePublic  `json:"prize,omitempty"`
	Prizes                   []InviteLotteryPrizePublic `json:"prizes"`
}

type RechargeWheelResult struct {
	Amount          float64 `json:"amount"`
	Multiplier      float64 `json:"multiplier"`
	Total           float64 `json:"total"`
	AmountIndex     int     `json:"amount_index"`
	MultiplierIndex int     `json:"multiplier_index"`
}

type RechargeWheelStatus struct {
	Enabled          bool                            `json:"enabled"`
	Threshold        float64                         `json:"threshold"`
	RechargedAmount  float64                         `json:"recharged_amount"`
	TotalChances     int                             `json:"total_chances"`
	UsedChances      int                             `json:"used_chances"`
	RemainingChances int                             `json:"remaining_chances"`
	CanClaim         bool                            `json:"can_claim"`
	Result           *RechargeWheelResult            `json:"result,omitempty"`
	Amounts          []RechargeWheelAmountPublic     `json:"amounts"`
	Multipliers      []RechargeWheelMultiplierPublic `json:"multipliers"`
}

type InviteMilestoneTierStatus struct {
	Invites           int     `json:"invites"`
	Reward            float64 `json:"reward"`
	Reached           bool    `json:"reached"`
	RechargeReached   bool    `json:"recharge_reached"`
	BlockedByRecharge bool    `json:"blocked_by_recharge"`
	Claimed           bool    `json:"claimed"`
	Claimable         bool    `json:"claimable"`
}

type InviteMilestoneStatus struct {
	Enabled                  bool                        `json:"enabled"`
	InvitedCount             int                         `json:"invited_count"`
	QualifiedInvitedCount    int                         `json:"qualified_invited_count"`
	RechargeLimitEnabled     bool                        `json:"recharge_limit_enabled"`
	InviteeRechargeThreshold float64                     `json:"invitee_recharge_threshold"`
	JustClaimedReward        float64                     `json:"just_claimed_reward,omitempty"`
	Tiers                    []InviteMilestoneTierStatus `json:"tiers"`
}

// InviteActivitiesRepository is deliberately narrow.  It exposes only the
// read aggregates and one atomic reward operation needed by this migration;
// the upstream AffiliateRepository and payment services remain untouched.
type InviteActivitiesRepository interface {
	CountEligibleInvitees(context.Context, int64) (int, error)
	CountQualifiedInvitees(context.Context, int64, float64) (int, error)
	SumCompletedRecharge(context.Context, int64) (float64, error)
	CountRewards(context.Context, int64, string) (int, error)
	ListClaimedMilestones(context.Context, int64, string) (map[int]bool, error)
	GrantReward(context.Context, int64, string, string, float64, string) (bool, error)
}

// InviteActivitiesAtomicRepository is implemented by the production SQL
// adapter.  Unlike the legacy-shaped GrantReward method, this capability
// re-checks the aggregate and child rollout switches while holding the reward
// transaction open.  Keeping it optional preserves compatibility with narrow
// test/dummy repositories and makes the fail-closed service behavior explicit.
type InviteActivitiesAtomicRepository interface {
	GrantRewardIfEnabled(context.Context, int64, string, string, float64, string, string, InviteActivityClaimRule) (bool, error)
}

// InviteActivityClaimRule is re-evaluated by the SQL repository after it has
// acquired the per-user claim lock.  The service still performs a fast
// preflight check for response quality; this rule closes the stale-count gap
// before the balance mutation.
type InviteActivityClaimRule struct {
	Threshold           float64
	RequiredCount       int
	RechargeLimitEnable bool
}

type InviteActivitiesService struct {
	repo            InviteActivitiesRepository
	settings        SettingRepository
	authCache       APIKeyAuthCacheInvalidator
	billingCache    *BillingCacheService
	settingsUpdated func()
}

func NewInviteActivitiesService(repo InviteActivitiesRepository, settings SettingRepository) *InviteActivitiesService {
	return &InviteActivitiesService{repo: repo, settings: settings}
}

func ProvideInviteActivitiesService(
	repo InviteActivitiesRepository,
	settings SettingRepository,
	authCache APIKeyAuthCacheInvalidator,
	billingCache *BillingCacheService,
	settingService *SettingService,
) *InviteActivitiesService {
	s := NewInviteActivitiesService(repo, settings)
	s.authCache = authCache
	s.billingCache = billingCache
	if settingService != nil {
		s.settingsUpdated = settingService.NotifySettingsUpdated
	}
	return s
}

func DefaultInviteActivitiesConfig() InviteActivitiesConfig {
	return InviteActivitiesConfig{
		Enabled:              false,
		InviteLotteryEnabled: false,
		InviteLotteryPrizes: []InviteLotteryPrize{
			{Name: "Lucky reward", Amount: 0.10, Probability: 50},
			{Name: "Advanced reward", Amount: 0.50, Probability: 30},
			{Name: "Super reward", Amount: 1.00, Probability: 20},
		},
		InviteLotteryRechargeThreshold: 10,
		RechargeWheelEnabled:           false,
		RechargeWheelThreshold:         10,
		RechargeWheelAmounts: []RechargeWheelAmount{
			{Amount: 0.50, Probability: 40}, {Amount: 1.00, Probability: 30},
			{Amount: 2.00, Probability: 20}, {Amount: 5.00, Probability: 10},
		},
		RechargeWheelMultipliers: []RechargeWheelMultiplier{
			{Multiplier: 1, Probability: 40}, {Multiplier: 2, Probability: 30},
			{Multiplier: 3, Probability: 20}, {Multiplier: 5, Probability: 10},
		},
		InviteMilestoneEnabled: false,
		InviteMilestoneTiers: []InviteMilestoneTier{
			{Invites: 5, Reward: 1}, {Invites: 10, Reward: 3},
			{Invites: 20, Reward: 8}, {Invites: 50, Reward: 25},
		},
		InviteMilestoneRechargeThreshold: 10,
	}
}

// Config is intentionally fail-closed.  A missing/invalid policy or a
// non-literal switch value leaves every child disabled and does not return an
// error that could be accidentally ignored by a request handler.
func (s *InviteActivitiesService) Config(ctx context.Context) InviteActivitiesConfig {
	defaults := DefaultInviteActivitiesConfig()
	if s == nil || s.settings == nil {
		return defaults
	}
	values, err := s.settings.GetMultiple(ctx, []string{
		SettingKeySubNexusInviteActivitiesEnabled,
		SettingKeySubNexusInviteActivitiesConfig,
	})
	if err != nil {
		return defaults
	}
	raw, ok := values[SettingKeySubNexusInviteActivitiesConfig]
	if !ok || strings.TrimSpace(raw) == "" || strings.TrimSpace(raw) == "null" {
		return defaults
	}
	var cfg InviteActivitiesConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return defaults
	}
	cfg = normalizeInviteActivitiesConfig(cfg)
	if err := validateInviteActivitiesConfig(cfg); err != nil {
		return defaults
	}
	// The aggregate switch is the sole runtime authority in addition to each
	// child switch.  Do not trim or case-fold: malformed values stay off.
	cfg.Enabled = values[SettingKeySubNexusInviteActivitiesEnabled] == "true"
	return cfg
}

func (s *InviteActivitiesService) UpdateConfig(ctx context.Context, cfg InviteActivitiesConfig) (InviteActivitiesConfig, error) {
	if s == nil || s.settings == nil {
		return cfg, infraerrors.InternalServer("SUBNEXUS_INVITE_ACTIVITIES_SETTINGS_UNAVAILABLE", "invite activity settings repository is unavailable")
	}
	cfg = normalizeInviteActivitiesConfig(cfg)
	if err := validateInviteActivitiesConfig(cfg); err != nil {
		return cfg, err
	}
	payload, err := json.Marshal(cfg)
	if err != nil {
		return cfg, fmt.Errorf("marshal invite activities config: %w", err)
	}
	value := "false"
	if cfg.Enabled {
		value = "true"
	}
	if err := s.settings.SetMultiple(ctx, map[string]string{
		SettingKeySubNexusInviteActivitiesEnabled: value,
		SettingKeySubNexusInviteActivitiesConfig:  string(payload),
	}); err != nil {
		return cfg, err
	}
	if s.settingsUpdated != nil {
		s.settingsUpdated()
	}
	return cfg, nil
}

func (s *InviteActivitiesService) featureEnabled(ctx context.Context, child bool) bool {
	return child && s.Config(ctx).Enabled
}

func (s *InviteActivitiesService) GetInviteLotteryStatus(ctx context.Context, userID int64) (*InviteLotteryStatus, error) {
	cfg := s.Config(ctx)
	status := &InviteLotteryStatus{
		Enabled:                  cfg.Enabled && cfg.InviteLotteryEnabled,
		RechargeLimitEnabled:     cfg.InviteLotteryRechargeLimitEnabled,
		InviteeRechargeThreshold: cfg.InviteLotteryRechargeThreshold,
		Prizes:                   publicInviteLotteryPrizes(cfg.InviteLotteryPrizes),
	}
	if !status.Enabled {
		return status, nil
	}
	if s == nil || s.repo == nil || userID <= 0 {
		return nil, infraerrors.ServiceUnavailable("SUBNEXUS_INVITE_ACTIVITIES_UNAVAILABLE", "invite activity repository is unavailable")
	}
	invited, err := s.repo.CountEligibleInvitees(ctx, userID)
	if err != nil {
		return nil, err
	}
	used, err := s.repo.CountRewards(ctx, userID, ActivitySourceInviteLottery)
	if err != nil {
		return nil, err
	}
	qualified := invited
	if cfg.InviteLotteryRechargeLimitEnabled {
		qualified, err = s.repo.CountQualifiedInvitees(ctx, userID, cfg.InviteLotteryRechargeThreshold)
		if err != nil {
			return nil, err
		}
	}
	status.InvitedCount, status.QualifiedInvitedCount, status.UsedChances = invited, qualified, used
	status.RemainingChances = maxInviteActivityInt(qualified-used, 0)
	status.LockedChances = maxInviteActivityInt(invited-used-status.RemainingChances, 0)
	status.CanClaim = status.RemainingChances > 0 && hasWinnableInviteLottery(cfg.InviteLotteryPrizes)
	return status, nil
}

func (s *InviteActivitiesService) ClaimInviteLottery(ctx context.Context, userID int64) (*InviteLotteryStatus, error) {
	cfg := s.Config(ctx)
	if !cfg.Enabled || !cfg.InviteLotteryEnabled {
		return nil, ErrInviteLotteryDisabled
	}
	if s == nil || s.repo == nil || userID <= 0 {
		return nil, infraerrors.ServiceUnavailable("SUBNEXUS_INVITE_ACTIVITIES_UNAVAILABLE", "invite activity repository is unavailable")
	}
	if !hasWinnableInviteLottery(cfg.InviteLotteryPrizes) {
		return nil, infraerrors.BadRequest("INVITE_LOTTERY_CONFIG_INVALID", "invite lottery prizes are not configured")
	}
	invited, err := s.repo.CountEligibleInvitees(ctx, userID)
	if err != nil {
		return nil, err
	}
	used, err := s.repo.CountRewards(ctx, userID, ActivitySourceInviteLottery)
	if err != nil {
		return nil, err
	}
	qualified := invited
	if cfg.InviteLotteryRechargeLimitEnabled {
		qualified, err = s.repo.CountQualifiedInvitees(ctx, userID, cfg.InviteLotteryRechargeThreshold)
		if err != nil {
			return nil, err
		}
	}
	if qualified-used <= 0 {
		if invited-used > 0 && cfg.InviteLotteryRechargeLimitEnabled {
			return nil, infraerrors.BadRequest("INVITE_LOTTERY_RECHARGE_REQUIRED", "invited users have not reached the required recharge amount")
		}
		return nil, infraerrors.BadRequest("INVITE_LOTTERY_NO_CHANCE", "no invite lottery chance available")
	}
	prize := pickInviteLotteryPrize(cfg.InviteLotteryPrizes)
	period := strconv.Itoa(used + 1)
	inserted, err := s.grantReward(ctx, userID, ActivitySourceInviteLottery, period, prize.Amount, "invite lottery: "+prize.Name, "invite_lottery", InviteActivityClaimRule{Threshold: cfg.InviteLotteryRechargeThreshold, RechargeLimitEnable: cfg.InviteLotteryRechargeLimitEnabled})
	if err != nil {
		return nil, err
	}
	if !inserted {
		return s.GetInviteLotteryStatus(ctx, userID)
	}
	s.invalidateBalanceCaches(ctx, userID)
	status, err := s.GetInviteLotteryStatus(ctx, userID)
	if err != nil {
		return nil, err
	}
	status.Prize = &InviteLotteryPrizePublic{Name: prize.Name, Amount: prize.Amount}
	return status, nil
}

func (s *InviteActivitiesService) GetRechargeWheelStatus(ctx context.Context, userID int64) (*RechargeWheelStatus, error) {
	cfg := s.Config(ctx)
	status := &RechargeWheelStatus{
		Enabled:     cfg.Enabled && cfg.RechargeWheelEnabled,
		Threshold:   cfg.RechargeWheelThreshold,
		Amounts:     publicRechargeWheelAmounts(cfg.RechargeWheelAmounts),
		Multipliers: publicRechargeWheelMultipliers(cfg.RechargeWheelMultipliers),
	}
	if !status.Enabled {
		return status, nil
	}
	if s == nil || s.repo == nil || userID <= 0 {
		return nil, infraerrors.ServiceUnavailable("SUBNEXUS_INVITE_ACTIVITIES_UNAVAILABLE", "invite activity repository is unavailable")
	}
	recharged, err := s.repo.SumCompletedRecharge(ctx, userID)
	if err != nil {
		return nil, err
	}
	used, err := s.repo.CountRewards(ctx, userID, ActivitySourceRechargeWheel)
	if err != nil {
		return nil, err
	}
	status.RechargedAmount = roundInviteMoney(recharged)
	status.TotalChances = rechargeWheelChances(recharged, cfg.RechargeWheelThreshold)
	status.UsedChances = used
	status.RemainingChances = maxInviteActivityInt(status.TotalChances-used, 0)
	status.CanClaim = status.RemainingChances > 0 && hasWinnableRechargeWheel(cfg)
	return status, nil
}

func (s *InviteActivitiesService) ClaimRechargeWheel(ctx context.Context, userID int64) (*RechargeWheelStatus, error) {
	cfg := s.Config(ctx)
	if !cfg.Enabled || !cfg.RechargeWheelEnabled {
		return nil, ErrRechargeWheelDisabled
	}
	if s == nil || s.repo == nil || userID <= 0 {
		return nil, infraerrors.ServiceUnavailable("SUBNEXUS_INVITE_ACTIVITIES_UNAVAILABLE", "invite activity repository is unavailable")
	}
	if cfg.RechargeWheelThreshold <= 0 || !hasWinnableRechargeWheel(cfg) {
		return nil, infraerrors.BadRequest("RECHARGE_WHEEL_CONFIG_INVALID", "recharge wheel is not configured")
	}
	recharged, err := s.repo.SumCompletedRecharge(ctx, userID)
	if err != nil {
		return nil, err
	}
	used, err := s.repo.CountRewards(ctx, userID, ActivitySourceRechargeWheel)
	if err != nil {
		return nil, err
	}
	if rechargeWheelChances(recharged, cfg.RechargeWheelThreshold)-used <= 0 {
		return nil, infraerrors.BadRequest("RECHARGE_WHEEL_NO_CHANCE", "no recharge wheel chance available")
	}
	amount, amountIndex := pickRechargeWheelAmount(cfg.RechargeWheelAmounts)
	multiplier, multiplierIndex := pickRechargeWheelMultiplier(cfg.RechargeWheelMultipliers)
	total := roundInviteMoney(amount * multiplier)
	if amount <= 0 || multiplier <= 0 || total <= 0 {
		return nil, infraerrors.BadRequest("RECHARGE_WHEEL_CONFIG_INVALID", "recharge wheel reward is invalid")
	}
	period := strconv.Itoa(used + 1)
	note := fmt.Sprintf("recharge wheel: $%.2f x%.2f = $%.2f", amount, multiplier, total)
	inserted, err := s.grantReward(ctx, userID, ActivitySourceRechargeWheel, period, total, note, "recharge_wheel", InviteActivityClaimRule{Threshold: cfg.RechargeWheelThreshold})
	if err != nil {
		return nil, err
	}
	if !inserted {
		return s.GetRechargeWheelStatus(ctx, userID)
	}
	s.invalidateBalanceCaches(ctx, userID)
	status, err := s.GetRechargeWheelStatus(ctx, userID)
	if err != nil {
		return nil, err
	}
	status.Result = &RechargeWheelResult{Amount: amount, Multiplier: multiplier, Total: total, AmountIndex: amountIndex, MultiplierIndex: multiplierIndex}
	return status, nil
}

func (s *InviteActivitiesService) GetInviteMilestoneStatus(ctx context.Context, userID int64) (*InviteMilestoneStatus, error) {
	cfg := s.Config(ctx)
	status := &InviteMilestoneStatus{
		Enabled:                  cfg.Enabled && cfg.InviteMilestoneEnabled,
		RechargeLimitEnabled:     cfg.InviteMilestoneRechargeLimitEnabled,
		InviteeRechargeThreshold: cfg.InviteMilestoneRechargeThreshold,
		Tiers:                    make([]InviteMilestoneTierStatus, 0, len(cfg.InviteMilestoneTiers)),
	}
	for _, tier := range cfg.InviteMilestoneTiers {
		status.Tiers = append(status.Tiers, InviteMilestoneTierStatus{Invites: tier.Invites, Reward: tier.Reward})
	}
	if !status.Enabled {
		return status, nil
	}
	if s == nil || s.repo == nil || userID <= 0 {
		return nil, infraerrors.ServiceUnavailable("SUBNEXUS_INVITE_ACTIVITIES_UNAVAILABLE", "invite activity repository is unavailable")
	}
	invited, err := s.repo.CountEligibleInvitees(ctx, userID)
	if err != nil {
		return nil, err
	}
	claimed, err := s.repo.ListClaimedMilestones(ctx, userID, ActivitySourceInviteMilestone)
	if err != nil {
		return nil, err
	}
	qualified := invited
	if cfg.InviteMilestoneRechargeLimitEnabled {
		qualified, err = s.repo.CountQualifiedInvitees(ctx, userID, cfg.InviteMilestoneRechargeThreshold)
		if err != nil {
			return nil, err
		}
	}
	status.InvitedCount, status.QualifiedInvitedCount = invited, qualified
	for i := range status.Tiers {
		t := &status.Tiers[i]
		t.Reached = invited >= t.Invites
		t.RechargeReached = !cfg.InviteMilestoneRechargeLimitEnabled || qualified >= t.Invites
		t.Claimed = claimed[t.Invites]
		t.BlockedByRecharge = t.Reached && !t.Claimed && !t.RechargeReached
		t.Claimable = t.Reached && t.RechargeReached && !t.Claimed && t.Reward > 0
	}
	return status, nil
}

func (s *InviteActivitiesService) ClaimInviteMilestone(ctx context.Context, userID int64, invites int) (*InviteMilestoneStatus, error) {
	cfg := s.Config(ctx)
	if !cfg.Enabled || !cfg.InviteMilestoneEnabled {
		return nil, ErrInviteMilestoneDisabled
	}
	if s == nil || s.repo == nil || userID <= 0 {
		return nil, infraerrors.ServiceUnavailable("SUBNEXUS_INVITE_ACTIVITIES_UNAVAILABLE", "invite activity repository is unavailable")
	}
	var tier *InviteMilestoneTier
	for i := range cfg.InviteMilestoneTiers {
		if cfg.InviteMilestoneTiers[i].Invites == invites {
			tier = &cfg.InviteMilestoneTiers[i]
			break
		}
	}
	if tier == nil || tier.Reward <= 0 {
		return nil, infraerrors.BadRequest("INVITE_MILESTONE_TIER_INVALID", "invite milestone tier is invalid")
	}
	invited, err := s.repo.CountEligibleInvitees(ctx, userID)
	if err != nil {
		return nil, err
	}
	if invited < tier.Invites {
		return nil, infraerrors.BadRequest("INVITE_MILESTONE_NOT_REACHED", "invite milestone has not been reached")
	}
	if cfg.InviteMilestoneRechargeLimitEnabled {
		qualified, err := s.repo.CountQualifiedInvitees(ctx, userID, cfg.InviteMilestoneRechargeThreshold)
		if err != nil {
			return nil, err
		}
		if qualified < tier.Invites {
			return nil, infraerrors.BadRequest("INVITE_MILESTONE_RECHARGE_REQUIRED", "invited users have not reached the required recharge amount")
		}
	}
	inserted, err := s.grantReward(ctx, userID, ActivitySourceInviteMilestone, strconv.Itoa(tier.Invites), tier.Reward, fmt.Sprintf("invite milestone: %d invites = $%.2f", tier.Invites, tier.Reward), "invite_milestone", InviteActivityClaimRule{Threshold: cfg.InviteMilestoneRechargeThreshold, RequiredCount: tier.Invites, RechargeLimitEnable: cfg.InviteMilestoneRechargeLimitEnabled})
	if err != nil {
		return nil, err
	}
	if inserted {
		s.invalidateBalanceCaches(ctx, userID)
	}
	status, err := s.GetInviteMilestoneStatus(ctx, userID)
	if err != nil {
		return nil, err
	}
	if inserted {
		status.JustClaimedReward = tier.Reward
	}
	return status, nil
}

func (s *InviteActivitiesService) grantReward(ctx context.Context, userID int64, source, period string, amount float64, note, feature string, rule InviteActivityClaimRule) (bool, error) {
	if atomicRepo, ok := s.repo.(InviteActivitiesAtomicRepository); ok {
		return atomicRepo.GrantRewardIfEnabled(ctx, userID, source, period, amount, note, feature, rule)
	}
	return s.repo.GrantReward(ctx, userID, source, period, amount, note)
}

func (s *InviteActivitiesService) invalidateBalanceCaches(ctx context.Context, userID int64) {
	if s.authCache != nil {
		s.authCache.InvalidateAuthCacheByUserID(ctx, userID)
	}
	if s.billingCache != nil {
		go func() {
			cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.billingCache.InvalidateUserBalance(cacheCtx, userID)
		}()
	}
}

func normalizeInviteActivitiesConfig(cfg InviteActivitiesConfig) InviteActivitiesConfig {
	// Rollout migrations may persist only switch fields while every activity is
	// closed. Keep the admin response JSON-friendly in that state, but do not
	// synthesize a prize pool for an explicitly enabled activity: validation
	// must continue to fail closed for incomplete active policies.
	defaults := DefaultInviteActivitiesConfig()
	if !cfg.InviteLotteryEnabled && cfg.InviteLotteryPrizes == nil {
		cfg.InviteLotteryPrizes = defaults.InviteLotteryPrizes
	}
	if !cfg.RechargeWheelEnabled && cfg.RechargeWheelAmounts == nil {
		cfg.RechargeWheelAmounts = defaults.RechargeWheelAmounts
	}
	if !cfg.RechargeWheelEnabled && cfg.RechargeWheelMultipliers == nil {
		cfg.RechargeWheelMultipliers = defaults.RechargeWheelMultipliers
	}
	if !cfg.InviteMilestoneEnabled && cfg.InviteMilestoneTiers == nil {
		cfg.InviteMilestoneTiers = defaults.InviteMilestoneTiers
	}
	cfg.InviteLotteryRechargeThreshold = roundInviteMoney(cfg.InviteLotteryRechargeThreshold)
	cfg.RechargeWheelThreshold = roundInviteMoney(cfg.RechargeWheelThreshold)
	cfg.InviteMilestoneRechargeThreshold = roundInviteMoney(cfg.InviteMilestoneRechargeThreshold)
	if cfg.InviteLotteryPrizes != nil {
		prizes := make([]InviteLotteryPrize, 0, len(cfg.InviteLotteryPrizes))
		for _, p := range cfg.InviteLotteryPrizes {
			p.Name = strings.TrimSpace(p.Name)
			if finiteInviteMoney(p.Amount) {
				p.Amount = roundInviteMoney(p.Amount)
			}
			if finiteProbability(p.Probability) {
				p.Probability = math.Round(p.Probability*10000) / 10000
			}
			prizes = append(prizes, p)
			if len(prizes) >= inviteActivitiesMaxPrizeItems {
				break
			}
		}
		cfg.InviteLotteryPrizes = prizes
	}
	if cfg.RechargeWheelAmounts != nil {
		items := make([]RechargeWheelAmount, 0, len(cfg.RechargeWheelAmounts))
		for _, item := range cfg.RechargeWheelAmounts {
			if finiteInviteMoney(item.Amount) {
				item.Amount = roundInviteMoney(item.Amount)
			}
			if finiteProbability(item.Probability) {
				item.Probability = math.Round(item.Probability*10000) / 10000
			}
			items = append(items, item)
			if len(items) >= inviteActivitiesMaxPrizeItems {
				break
			}
		}
		cfg.RechargeWheelAmounts = items
	}
	if cfg.RechargeWheelMultipliers != nil {
		items := make([]RechargeWheelMultiplier, 0, len(cfg.RechargeWheelMultipliers))
		for _, item := range cfg.RechargeWheelMultipliers {
			if finiteInviteMoney(item.Multiplier) {
				item.Multiplier = math.Round(item.Multiplier*100) / 100
			}
			if finiteProbability(item.Probability) {
				item.Probability = math.Round(item.Probability*10000) / 10000
			}
			items = append(items, item)
			if len(items) >= inviteActivitiesMaxPrizeItems {
				break
			}
		}
		cfg.RechargeWheelMultipliers = items
	}
	if cfg.InviteMilestoneTiers != nil {
		tiers := make([]InviteMilestoneTier, 0, len(cfg.InviteMilestoneTiers))
		for _, tier := range cfg.InviteMilestoneTiers {
			if finiteInviteMoney(tier.Reward) {
				tier.Reward = roundInviteMoney(tier.Reward)
			}
			tiers = append(tiers, tier)
			if len(tiers) >= inviteActivitiesMaxTiers {
				break
			}
		}
		sort.SliceStable(tiers, func(i, j int) bool {
			if tiers[i].Invites == tiers[j].Invites {
				return tiers[i].Reward > tiers[j].Reward
			}
			return tiers[i].Invites < tiers[j].Invites
		})
		seen := make(map[int]bool, len(tiers))
		unique := tiers[:0]
		for _, tier := range tiers {
			if seen[tier.Invites] {
				continue
			}
			seen[tier.Invites] = true
			unique = append(unique, tier)
		}
		cfg.InviteMilestoneTiers = unique
	}
	return cfg
}

func validateInviteActivitiesConfig(cfg InviteActivitiesConfig) error {
	if err := validateInviteLotteryPolicy(cfg); err != nil {
		return err
	}
	if err := validateRechargeWheelPolicy(cfg); err != nil {
		return err
	}
	if err := validateInviteMilestonePolicy(cfg); err != nil {
		return err
	}
	return nil
}

func validateInviteLotteryPolicy(cfg InviteActivitiesConfig) error {
	if !finiteInviteMoney(cfg.InviteLotteryRechargeThreshold) || cfg.InviteLotteryRechargeThreshold < 0 {
		return infraerrors.BadRequest("INVITE_LOTTERY_CONFIG_INVALID", "invite lottery recharge threshold is invalid")
	}
	if cfg.InviteLotteryRechargeLimitEnabled && cfg.InviteLotteryRechargeThreshold <= 0 {
		return infraerrors.BadRequest("INVITE_LOTTERY_RECHARGE_THRESHOLD_INVALID", "invite lottery recharge threshold must be positive")
	}
	if len(cfg.InviteLotteryPrizes) > inviteActivitiesMaxPrizeItems {
		return infraerrors.BadRequest("INVITE_LOTTERY_CONFIG_INVALID", "too many invite lottery prizes")
	}
	for _, p := range cfg.InviteLotteryPrizes {
		if p.Name == "" || len([]rune(p.Name)) > 120 || !finiteInviteMoney(p.Amount) || p.Amount <= 0 || !finiteProbability(p.Probability) || p.Probability < 0 {
			return infraerrors.BadRequest("INVITE_LOTTERY_CONFIG_INVALID", "invite lottery prize is invalid")
		}
	}
	if cfg.InviteLotteryEnabled && !hasWinnableInviteLottery(cfg.InviteLotteryPrizes) {
		return infraerrors.BadRequest("INVITE_LOTTERY_CONFIG_INVALID", "invite lottery requires a positive prize and probability")
	}
	return nil
}

func validateRechargeWheelPolicy(cfg InviteActivitiesConfig) error {
	if !finiteInviteMoney(cfg.RechargeWheelThreshold) || cfg.RechargeWheelThreshold < 0 {
		return infraerrors.BadRequest("RECHARGE_WHEEL_CONFIG_INVALID", "recharge wheel threshold is invalid")
	}
	if cfg.RechargeWheelEnabled && cfg.RechargeWheelThreshold <= 0 {
		return infraerrors.BadRequest("RECHARGE_WHEEL_CONFIG_INVALID", "recharge wheel threshold must be positive")
	}
	if len(cfg.RechargeWheelAmounts) > inviteActivitiesMaxPrizeItems || len(cfg.RechargeWheelMultipliers) > inviteActivitiesMaxPrizeItems {
		return infraerrors.BadRequest("RECHARGE_WHEEL_CONFIG_INVALID", "too many recharge wheel entries")
	}
	for _, item := range cfg.RechargeWheelAmounts {
		if !finiteInviteMoney(item.Amount) || item.Amount <= 0 || !finiteProbability(item.Probability) || item.Probability < 0 {
			return infraerrors.BadRequest("RECHARGE_WHEEL_CONFIG_INVALID", "recharge wheel amount is invalid")
		}
	}
	for _, item := range cfg.RechargeWheelMultipliers {
		if !finiteInviteMoney(item.Multiplier) || item.Multiplier <= 0 || !finiteProbability(item.Probability) || item.Probability < 0 {
			return infraerrors.BadRequest("RECHARGE_WHEEL_CONFIG_INVALID", "recharge wheel multiplier is invalid")
		}
	}
	if cfg.RechargeWheelEnabled && !hasWinnableRechargeWheel(cfg) {
		return infraerrors.BadRequest("RECHARGE_WHEEL_CONFIG_INVALID", "recharge wheel requires positive amount and multiplier probabilities")
	}
	return nil
}

func validateInviteMilestonePolicy(cfg InviteActivitiesConfig) error {
	if !finiteInviteMoney(cfg.InviteMilestoneRechargeThreshold) || cfg.InviteMilestoneRechargeThreshold < 0 {
		return infraerrors.BadRequest("INVITE_MILESTONE_CONFIG_INVALID", "invite milestone recharge threshold is invalid")
	}
	if cfg.InviteMilestoneRechargeLimitEnabled && cfg.InviteMilestoneRechargeThreshold <= 0 {
		return infraerrors.BadRequest("INVITE_MILESTONE_RECHARGE_THRESHOLD_INVALID", "invite milestone recharge threshold must be positive")
	}
	if len(cfg.InviteMilestoneTiers) > inviteActivitiesMaxTiers {
		return infraerrors.BadRequest("INVITE_MILESTONE_CONFIG_INVALID", "too many invite milestone tiers")
	}
	seen := map[int]bool{}
	for _, tier := range cfg.InviteMilestoneTiers {
		if tier.Invites <= 0 || tier.Reward <= 0 || !finiteInviteMoney(tier.Reward) || seen[tier.Invites] {
			return infraerrors.BadRequest("INVITE_MILESTONE_CONFIG_INVALID", "invite milestone tier is invalid")
		}
		seen[tier.Invites] = true
	}
	if cfg.InviteMilestoneEnabled && len(cfg.InviteMilestoneTiers) == 0 {
		return infraerrors.BadRequest("INVITE_MILESTONE_CONFIG_INVALID", "invite milestone requires at least one tier")
	}
	return nil
}

func finiteInviteMoney(v float64) bool {
	return v >= 0 && !math.IsNaN(v) && !math.IsInf(v, 0) && v <= inviteActivitiesMaxMoney
}

func finiteProbability(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= 1_000_000
}

func roundInviteMoney(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return v
	}
	return math.Round(v*100) / 100
}

func hasWinnableInviteLottery(prizes []InviteLotteryPrize) bool {
	for _, p := range prizes {
		if p.Amount > 0 && p.Probability > 0 {
			return true
		}
	}
	return false
}

func pickInviteLotteryPrize(prizes []InviteLotteryPrize) InviteLotteryPrize {
	var total float64
	last := InviteLotteryPrize{}
	for _, p := range prizes {
		if p.Amount > 0 && p.Probability > 0 {
			total += p.Probability
			last = p
		}
	}
	if total <= 0 {
		return InviteLotteryPrize{}
	}
	target := secureRandomFloat() * total
	var cursor float64
	for _, p := range prizes {
		if p.Amount <= 0 || p.Probability <= 0 {
			continue
		}
		cursor += p.Probability
		if target < cursor {
			return p
		}
	}
	return last
}

func publicInviteLotteryPrizes(prizes []InviteLotteryPrize) []InviteLotteryPrizePublic {
	out := make([]InviteLotteryPrizePublic, 0, len(prizes))
	for _, p := range prizes {
		out = append(out, InviteLotteryPrizePublic{Name: p.Name, Amount: p.Amount})
	}
	return out
}

func hasWinnableRechargeWheel(cfg InviteActivitiesConfig) bool {
	amount, multiplier := false, false
	for _, item := range cfg.RechargeWheelAmounts {
		amount = amount || (item.Amount > 0 && item.Probability > 0)
	}
	for _, item := range cfg.RechargeWheelMultipliers {
		multiplier = multiplier || (item.Multiplier > 0 && item.Probability > 0)
	}
	return amount && multiplier
}

func pickRechargeWheelAmount(items []RechargeWheelAmount) (float64, int) {
	var total float64
	last := -1
	for i, item := range items {
		if item.Amount > 0 && item.Probability > 0 {
			total += item.Probability
			last = i
		}
	}
	if total <= 0 || last < 0 {
		return 0, 0
	}
	target := secureRandomFloat() * total
	var cursor float64
	for i, item := range items {
		if item.Amount <= 0 || item.Probability <= 0 {
			continue
		}
		cursor += item.Probability
		if target < cursor {
			return item.Amount, i
		}
	}
	return items[last].Amount, last
}

func pickRechargeWheelMultiplier(items []RechargeWheelMultiplier) (float64, int) {
	var total float64
	last := -1
	for i, item := range items {
		if item.Multiplier > 0 && item.Probability > 0 {
			total += item.Probability
			last = i
		}
	}
	if total <= 0 || last < 0 {
		return 0, 0
	}
	target := secureRandomFloat() * total
	var cursor float64
	for i, item := range items {
		if item.Multiplier <= 0 || item.Probability <= 0 {
			continue
		}
		cursor += item.Probability
		if target < cursor {
			return item.Multiplier, i
		}
	}
	return items[last].Multiplier, last
}

func secureRandomFloat() float64 {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err == nil {
		var value uint64
		for _, b := range buf {
			value = (value << 8) | uint64(b)
		}
		return float64(value>>11) / float64(uint64(1)<<53)
	}
	// A failed system RNG must not panic or produce a deterministic reward;
	// returning zero still selects a valid configured item and the audit log
	// records the actual result. The feature remains explicitly opt-in.
	return 0
}

func rechargeWheelChances(recharged, threshold float64) int {
	if !finiteInviteMoney(recharged) || recharged <= 0 || !finiteInviteMoney(threshold) || threshold <= 0 {
		return 0
	}
	value := int(math.Floor((recharged + 1e-8) / threshold))
	if value < 0 {
		return 0
	}
	return value
}

func publicRechargeWheelAmounts(items []RechargeWheelAmount) []RechargeWheelAmountPublic {
	out := make([]RechargeWheelAmountPublic, 0, len(items))
	for _, item := range items {
		out = append(out, RechargeWheelAmountPublic{Amount: item.Amount})
	}
	return out
}

func publicRechargeWheelMultipliers(items []RechargeWheelMultiplier) []RechargeWheelMultiplierPublic {
	out := make([]RechargeWheelMultiplierPublic, 0, len(items))
	for _, item := range items {
		out = append(out, RechargeWheelMultiplierPublic{Multiplier: item.Multiplier})
	}
	return out
}

func maxInviteActivityInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
