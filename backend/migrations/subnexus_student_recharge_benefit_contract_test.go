package migrations

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The fork uses 9008 so a database that already applied the legacy 199
// migration can be adopted without replaying or deleting any student ledger.
func TestSubNexusStudentRechargeBenefitMigrationIsAdditiveAndLegacyCompatible(t *testing.T) {
	content, err := FS.ReadFile("9008_subnexus_student_recharge_benefit.sql")
	require.NoError(t, err)
	sql := strings.ToLower(stripSQLLineComments(string(content)))

	for _, table := range []string{
		"student_account_status",
		"student_account_audit_logs",
		"student_recharge_bonus_logs",
	} {
		require.Contains(t, sql, "create table if not exists "+table)
	}
	for _, index := range []string{
		"idx_student_account_status_active",
		"idx_student_account_audit_user",
		"idx_student_account_audit_admin",
		"idx_student_recharge_bonus_pending",
		"idx_student_recharge_bonus_user",
	} {
		require.Contains(t, sql, "create index if not exists "+index)
	}

	// The migration must be replay-safe and must not mutate financial data.
	for _, forbidden := range []string{
		`\bdrop\s+`,
		`\btruncate\s+`,
		`\bdelete\s+from\b`,
		`\bupdate\s+`,
		`\binsert\s+into\b`,
		`\balter\s+table\s+(users|payment_orders)\b`,
	} {
		require.NotRegexp(t, regexp.MustCompile(`(?i)`+forbidden), sql)
	}
	require.Contains(t, sql, "payment_order_id bigint not null unique references payment_orders(id) on delete restrict")
	require.NotContains(t, sql, "199_student_recharge_benefit.sql")
}
