package migrations

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSubNexusInviteActivitiesMigrationIsConcurrentAndAdditive(t *testing.T) {
	content, err := FS.ReadFile("9009_subnexus_invite_activities_notx.sql")
	require.NoError(t, err)
	sql := strings.ToLower(stripSQLLineComments(string(content)))

	for _, index := range []string{
		"idx_subnexus_invite_activities_affiliate_inviter",
		"idx_subnexus_invite_activities_payment_user",
		"idx_subnexus_invite_activities_reward_user_source",
	} {
		require.Contains(t, sql, "create index concurrently if not exists "+index)
	}
	for _, required := range []string{
		"user_affiliates (inviter_id, user_id)",
		"payment_orders (user_id, status, order_type)",
		"activity_reward_logs (user_id, source)",
		"'completed'",
		"'partially_refunded'",
		"'first_recharge_gift'",
	} {
		require.Contains(t, sql, required)
	}

	// A *_notx migration may only contain idempotent concurrent index DDL.  In
	// particular, it must not seed the runtime switch or mutate financial data.
	for _, forbidden := range []string{
		`\bbegin\b`,
		`\bcommit\b`,
		`\brollback\b`,
		`\bdrop\b`,
		`\btruncate\b`,
		`\bdelete\s+from\b`,
		`\bupdate\s+`,
		`\binsert\s+into\b`,
		`subnexus_invite_activities_enabled`,
	} {
		require.NotRegexp(t, regexp.MustCompile(forbidden), sql)
	}
}
