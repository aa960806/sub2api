package migrations

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSubNexusInviteRewardsMigrationContract(t *testing.T) {
	content, err := FS.ReadFile("9004_subnexus_invite_rewards.sql")
	require.NoError(t, err)
	sql := strings.ToLower(stripSQLLineComments(string(content)))

	for _, want := range []string{
		"alter table user_affiliate_ledger",
		"add column if not exists source_ip varchar(64) not null default ''",
		"idx_subnexus_invite_signup_reward_ip_daily",
		"idx_user_affiliate_signup_reward_inviter_once",
		"idx_user_affiliate_signup_reward_invitee_once",
		"action in ('signup_bonus_inviter', 'signup_bonus_invitee')",
	} {
		require.Contains(t, sql, want)
	}

	// Applying the migration to a live database must not grant rewards or alter
	// existing rows. Index/column DDL is the complete intended side effect.
	for _, forbidden := range []string{
		`\binsert\s+into\b`,
		`\bupdate\s+`,
		`\bdelete\s+from\b`,
		`\bdrop\s+`,
		`\btruncate\s+`,
		`subnexus_invite_rewards_enabled`,
	} {
		require.NotRegexp(t, regexp.MustCompile(`(?i)`+forbidden), sql)
	}
}
