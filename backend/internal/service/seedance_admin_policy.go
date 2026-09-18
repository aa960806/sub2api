package service

import (
	"context"
	"math"
	"net/url"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// Seedance is intentionally limited to dedicated, balance-billed API-key groups.
// Keep these restrictions at the service boundary so imports and bulk edits
// cannot bypass the administrator form's choices.
func validateSeedanceAccount(platform, accountType string, credentials map[string]any) error {
	if platform != PlatformSeedance {
		return nil
	}
	if accountType != AccountTypeAPIKey {
		return infraerrors.BadRequest("SEEDANCE_APIKEY_REQUIRED", "Seedance only supports API key accounts")
	}
	key, _ := credentials["api_key"].(string)
	if strings.TrimSpace(key) == "" {
		return infraerrors.BadRequest("SEEDANCE_APIKEY_REQUIRED", "Seedance API key is required")
	}
	if raw := credentials["model_mapping"]; raw != nil {
		mapping, ok := raw.(map[string]any)
		if !ok {
			return infraerrors.BadRequest("SEEDANCE_MODEL_MAPPING_UNSUPPORTED", "Seedance supports model whitelists, not model alias mappings")
		}
		for from, rawTo := range mapping {
			to, ok := rawTo.(string)
			if !ok || from != to {
				return infraerrors.BadRequest("SEEDANCE_MODEL_MAPPING_UNSUPPORTED", "Seedance supports model whitelists, not model alias mappings")
			}
		}
	}
	if raw, exists := credentials["base_url"]; exists {
		base, ok := raw.(string)
		if !ok {
			return infraerrors.BadRequest("SEEDANCE_BASE_URL_INVALID", "Seedance base URL must be an HTTPS URL")
		}
		if base = strings.TrimSpace(base); base != "" {
			u, err := url.Parse(base)
			if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
				return infraerrors.BadRequest("SEEDANCE_BASE_URL_INVALID", "Seedance base URL must be an HTTPS URL without user information, query or fragment")
			}
		}
	}
	return nil
}

func validateSeedanceAccountMultiplier(platform string, multiplier *float64) error {
	if platform == PlatformSeedance && multiplier != nil && *multiplier != 1 {
		return infraerrors.BadRequest("SEEDANCE_FINAL_PRICE_REQUIRED", "Seedance account rate multiplier must be 1")
	}
	return nil
}

func containsSeedanceAccountQuota(extra map[string]any) bool {
	for _, key := range []string{"quota_limit", "quota_daily_limit", "quota_weekly_limit"} {
		if _, exists := extra[key]; exists {
			return true
		}
	}
	return false
}

func validateSeedanceAccountQuota(platform string, extra map[string]any) error {
	if platform != PlatformSeedance {
		return nil
	}
	account := &Account{Extra: extra}
	for _, limit := range []float64{account.GetQuotaLimit(), account.GetQuotaDailyLimit(), account.GetQuotaWeeklyLimit()} {
		if limit > 0 || math.IsNaN(limit) || math.IsInf(limit, 0) {
			return infraerrors.BadRequest("SEEDANCE_ACCOUNT_QUOTA_UNSUPPORTED", "Seedance account budgets are not supported; use user balance and API key limits")
		}
	}
	return nil
}

func validateSeedanceGroup(platform, subscriptionType string, multiplier float64) error {
	if platform != PlatformSeedance {
		return nil
	}
	if subscriptionType != "" && subscriptionType != SubscriptionTypeStandard {
		return infraerrors.BadRequest("SEEDANCE_BALANCE_ONLY", "Seedance only supports balance billing")
	}
	if multiplier != 1 {
		return infraerrors.BadRequest("SEEDANCE_FINAL_PRICE_REQUIRED", "Seedance uses the final per-request model price; group rate multiplier must be 1")
	}
	return nil
}

func validateSeedancePricing(pricing []ChannelModelPricing) error {
	for _, entry := range pricing {
		if entry.Platform != PlatformSeedance {
			continue
		}
		if entry.BillingMode != BillingModePerRequest || len(entry.Intervals) != 0 || (entry.TimePricing != nil && len(entry.TimePricing.Periods) != 0) {
			return infraerrors.BadRequest("SEEDANCE_PER_REQUEST_PRICING_REQUIRED", "Seedance requires a fixed per-request price without token, video, tiered or time pricing")
		}
		if entry.PerRequestPrice == nil || *entry.PerRequestPrice <= 0 || math.IsNaN(*entry.PerRequestPrice) || math.IsInf(*entry.PerRequestPrice, 0) {
			return infraerrors.BadRequest("SEEDANCE_PRICE_INVALID", "Seedance per-request price must be finite and greater than zero")
		}
	}
	return nil
}

func (s *adminServiceImpl) validateSeedanceAccountGroups(ctx context.Context, account *Account, groupIDs []int64) error {
	if !account.IsSeedance() || len(groupIDs) == 0 {
		return nil
	}
	for _, groupID := range groupIDs {
		group, err := s.groupRepo.GetByIDLite(ctx, groupID)
		if err != nil {
			return err
		}
		if group == nil || group.Platform != PlatformSeedance {
			return infraerrors.BadRequest("SEEDANCE_DEDICATED_GROUP_REQUIRED", "Seedance accounts can only be bound to dedicated Seedance groups")
		}
	}
	return nil
}
