package service

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
)

var subNexusInviteSignupRewardSettingKeys = []string{
	SettingKeySubNexusInviteRewardsEnabled,
	SettingKeySubNexusInviteSignupRewardInviter,
	SettingKeySubNexusInviteSignupRewardInvitee,
	SettingKeySubNexusInviteSignupRewardIPLimit,
	SettingKeySubNexusInviteSignupRewardIPDaily,
}

type SubNexusInviteSignupRewardRuntime struct {
	Enabled        bool
	InviterAmount  float64
	InviteeAmount  float64
	IPLimitEnabled bool
	IPDailyLimit   int
}

// GetSubNexusInviteSignupRewardRuntime loads the balance-writing policy in one
// repository call. Once the gate is enabled, every dependent value must be
// present and valid; storage or parsing failures therefore fail closed.
func (s *SettingService) GetSubNexusInviteSignupRewardRuntime(ctx context.Context) (SubNexusInviteSignupRewardRuntime, error) {
	var runtime SubNexusInviteSignupRewardRuntime
	if s == nil || s.settingRepo == nil {
		return runtime, fmt.Errorf("SubNexus invite reward settings unavailable")
	}
	values, err := s.settingRepo.GetMultiple(ctx, subNexusInviteSignupRewardSettingKeys)
	if err != nil {
		return runtime, fmt.Errorf("read SubNexus invite reward settings: %w", err)
	}
	enabled, ok := parseStrictBoolSetting(values, SettingKeySubNexusInviteRewardsEnabled)
	if !ok || !enabled {
		return runtime, nil
	}
	runtime.Enabled = true
	if runtime.InviterAmount, ok = parseSubNexusInviteRewardAmount(values, SettingKeySubNexusInviteSignupRewardInviter); !ok {
		return SubNexusInviteSignupRewardRuntime{}, fmt.Errorf("invalid SubNexus inviter reward amount")
	}
	if runtime.InviteeAmount, ok = parseSubNexusInviteRewardAmount(values, SettingKeySubNexusInviteSignupRewardInvitee); !ok {
		return SubNexusInviteSignupRewardRuntime{}, fmt.Errorf("invalid SubNexus invitee reward amount")
	}
	if runtime.IPLimitEnabled, ok = parseStrictBoolSetting(values, SettingKeySubNexusInviteSignupRewardIPLimit); !ok {
		return SubNexusInviteSignupRewardRuntime{}, fmt.Errorf("invalid SubNexus invite reward IP limit switch")
	}
	if runtime.IPDailyLimit, ok = parseSubNexusInviteRewardDailyLimit(values, SettingKeySubNexusInviteSignupRewardIPDaily); !ok {
		return SubNexusInviteSignupRewardRuntime{}, fmt.Errorf("invalid SubNexus invite reward IP daily limit")
	}
	return runtime, nil
}

func parseStrictBoolSetting(values map[string]string, key string) (bool, bool) {
	raw, exists := values[key]
	if !exists {
		return false, false
	}
	// Rollout switches use an exact serialized value. This prevents an
	// accidentally copied "TRUE"/" true " from enabling balance writes.
	switch raw {
	case "true":
		return true, true
	case "false":
		return false, true
	default:
		return false, false
	}
}

func parseSubNexusInviteRewardAmount(values map[string]string, key string) (float64, bool) {
	raw, exists := values[key]
	if !exists {
		return 0, false
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > SubNexusInviteSignupRewardAmountMax {
		return 0, false
	}
	return math.Round(value*1e8) / 1e8, true
}

func parseSubNexusInviteRewardDailyLimit(values map[string]string, key string) (int, bool) {
	raw, exists := values[key]
	if !exists {
		return 0, false
	}
	limit, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || limit <= 0 || limit > SubNexusInviteSignupRewardIPDailyMax {
		return 0, false
	}
	return limit, true
}

func parseSubNexusInviteRewardAmountOrDefault(raw string) float64 {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > SubNexusInviteSignupRewardAmountMax {
		return SubNexusInviteSignupRewardAmountDefault
	}
	return math.Round(value*1e8) / 1e8
}
