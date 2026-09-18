package service

import "strings"

// DefaultSeedanceBaseURL preserves the provider's API path prefix.
const DefaultSeedanceBaseURL = "https://api.laogou.org/seedance"

// DefaultSeedanceModelIDs is also used by the administrator model picker.
func DefaultSeedanceModelIDs() []string {
	return []string{"seedance2.5", "seedance2.0", "seedance2.0fast", "seedance2.0mini"}
}

// IsSeedance deliberately does not make the account OpenAI compatible.
func (a *Account) IsSeedance() bool {
	return a != nil && a.Platform == PlatformSeedance
}

func (a *Account) GetSeedanceAPIKey() string {
	if !a.IsSeedance() || a.Type != AccountTypeAPIKey {
		return ""
	}
	return strings.TrimSpace(a.GetCredential("api_key"))
}

func (a *Account) GetSeedanceBaseURL() string {
	if !a.IsSeedance() {
		return ""
	}
	if baseURL := strings.TrimSpace(a.GetCredential("base_url")); baseURL != "" {
		return strings.TrimRight(baseURL, "/")
	}
	return DefaultSeedanceBaseURL
}
