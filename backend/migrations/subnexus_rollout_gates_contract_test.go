package migrations

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Existing databases skip InitializeDefaultSettings once any setting exists.
// Keep every migrated runtime gate explicitly present and disabled in the
// additive post-migration seed; ON CONFLICT is required to preserve operator
// choices during a staged rollout or application rollback.
func TestSubNexusRolloutGatesMigrationIsCompleteAndFailClosed(t *testing.T) {
	content, err := FS.ReadFile("9011_subnexus_rollout_gates.sql")
	require.NoError(t, err)
	sql := strings.ToLower(stripSQLLineComments(string(content)))

	for _, key := range []string{
		"registration_ip_cooldown_enabled",
		"subnexus_activity_center_enabled",
		"subnexus_checkin_enabled",
		"subnexus_leaderboard_enabled",
		"subnexus_marquee_enabled",
		"subnexus_invite_activities_enabled",
		"subnexus_invite_rewards_enabled",
		"subnexus_first_recharge_enabled",
		"battle_pass_enabled",
		"subnexus_student_recharge_benefit_enabled",
		"subnexus_invoice_enabled",
	} {
		require.Regexp(t, regexp.MustCompile(`\('`+regexp.QuoteMeta(key)+`',\s*'false'\)`), sql,
			"missing disabled rollout gate: "+key)
	}

	for _, forbidden := range []string{
		`\bupdate\s+`,
		`\bdelete\s+from\b`,
		`\bdrop\s+`,
		`\btruncate\s+`,
	} {
		require.NotRegexp(t, regexp.MustCompile(`(?i)`+forbidden), sql)
	}
	require.Contains(t, sql, "on conflict (key) do nothing")
}
