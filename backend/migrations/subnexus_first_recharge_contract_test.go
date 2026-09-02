package migrations

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSubNexusFirstRechargeMigrationContract(t *testing.T) {
	content, err := FS.ReadFile("9005_subnexus_first_recharge.sql")
	require.NoError(t, err)
	sql := strings.ToLower(stripSQLLineComments(string(content)))

	for _, want := range []string{
		"create table if not exists first_recharge_gift_purchases",
		"references users(id) on delete cascade",
		"references payment_orders(id) on delete set null",
		"idx_first_recharge_gift_purchases_user",
		"idx_first_recharge_gift_purchases_order",
		"idx_first_recharge_gift_purchases_status",
	} {
		require.Contains(t, sql, want)
	}

	// Startup schema work must never create a purchase, mutate an order, or
	// enable the feature. Existing legacy rows remain untouched for rollback.
	for _, forbidden := range []string{
		`\binsert\s+into\b`,
		`\bupdate\s+`,
		`\bdelete\s+from\b`,
		`\bdrop\s+`,
		`\btruncate\s+`,
		`\balter\s+table\s+payment_orders\b`,
		`subnexus_first_recharge_enabled`,
		`subnexus_first_recharge_config`,
	} {
		require.NotRegexp(t, regexp.MustCompile(`(?i)`+forbidden), sql)
	}
}
