package migrations

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSubNexusInviteSignupRewardJobsMigrationIsAdditiveAndFailClosed(t *testing.T) {
	content, err := FS.ReadFile("9012_subnexus_invite_signup_reward_jobs.sql")
	require.NoError(t, err)
	sql := strings.ToLower(stripSQLLineComments(string(content)))

	for _, want := range []string{
		"create table if not exists subnexus_affiliate_signup_reward_jobs",
		"inviter_id bigint not null references users(id) on delete cascade",
		"invitee_user_id bigint not null references users(id) on delete cascade",
		"inviter_amount decimal(20,8) not null default 0",
		"invitee_amount decimal(20,8) not null default 0",
		"status varchar(16) not null default 'pending'",
		"status in ('pending', 'completed', 'skipped')",
		"unique (invitee_user_id)",
		"idx_subnexus_affiliate_signup_reward_jobs_pending",
	} {
		require.Contains(t, sql, want)
	}

	// The queue migration must not seed a switch or mutate existing balances;
	// the independent rollout gate remains off until an operator enables it.
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
