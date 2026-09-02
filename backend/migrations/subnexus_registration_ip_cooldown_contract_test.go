package migrations

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// 9010 is the fork-side adoption name for the legacy registration cooldown
// schema.  Keep this contract deliberately static: production adoption must
// never depend on a migration that mutates users, settings, or registrations.
func TestSubNexusRegistrationIPCooldownMigrationIsAdditiveAndStable(t *testing.T) {
	content, err := FS.ReadFile("9010_subnexus_registration_ip_cooldown.sql")
	require.NoError(t, err)
	sql := strings.ToLower(stripSQLLineComments(string(content)))

	for _, want := range []string{
		"create table if not exists registration_ip_cooldowns",
		"ip_hash char(64) primary key",
		"last_registered_at timestamptz",
		"last_user_id bigint references users(id) on delete set null",
		"reservation_token char(64)",
		"reserved_until timestamptz",
		"idx_registration_ip_cooldowns_last_registered_at",
		"idx_registration_ip_cooldowns_reserved_until",
	} {
		require.Contains(t, sql, want)
	}

	// Applying or replaying this migration must not change existing business
	// rows or silently enable the registration control.
	for _, forbidden := range []string{
		`\bdrop\s+`,
		`\btruncate\s+`,
		`\bdelete\s+from\b`,
		`\bupdate\s+`,
		`\binsert\s+into\b`,
		`\bregistration_ip_cooldown_enabled\b`,
		`\bregistration_ip_cooldown_seconds\b`,
	} {
		require.NotRegexp(t, regexp.MustCompile(`(?i)`+forbidden), sql)
	}
}
