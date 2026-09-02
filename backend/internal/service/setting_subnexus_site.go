package service

import (
	"strconv"
	"strings"
)

// NormalizeSubNexusDefaultLanguage accepts the two locales supported by the
// frontend. Empty means "follow the browser". Unknown values deliberately
// collapse to empty so a malformed setting cannot force a partial locale.
func NormalizeSubNexusDefaultLanguage(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "zh":
		return "zh"
	case "en":
		return "en"
	default:
		return ""
	}
}

// subNexusSiteSetting resolves a namespaced value while retaining compatibility
// with the legacy unprefixed key. Presence of the namespaced key wins even when
// its value is empty/false, which lets an explicit clear override stale legacy
// data during a same-database rollout.
func subNexusSiteSetting(values map[string]string, namespacedKey, legacyKey string) string {
	if value, ok := values[namespacedKey]; ok {
		return value
	}
	return values[legacyKey]
}

func subNexusSiteBool(values map[string]string, namespacedKey, legacyKey string) bool {
	return subNexusSiteSetting(values, namespacedKey, legacyKey) == "true"
}

// appendSubNexusSiteSettings writes both key generations. The old generation
// remains intentionally synchronized so an older binary can be rolled back
// without losing an administrator's latest site configuration.
func appendSubNexusSiteSettings(updates map[string]string, settings *SystemSettings) {
	if updates == nil || settings == nil {
		return
	}
	language := NormalizeSubNexusDefaultLanguage(settings.DefaultLanguage)
	enabled := strconv.FormatBool(settings.CustomerSupportEnabled)
	content := settings.CustomerSupportContent

	updates[SettingKeySubNexusDefaultLanguage] = language
	updates[SettingKeyDefaultLanguage] = language
	updates[SettingKeySubNexusCustomerSupportEnabled] = enabled
	updates[SettingKeyCustomerSupportEnabled] = enabled
	updates[SettingKeySubNexusCustomerSupportContent] = content
	updates[SettingKeyCustomerSupportContent] = content
}
