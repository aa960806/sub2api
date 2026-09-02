package migrations

import (
	"regexp"
	"strings"
	"testing"
)

func TestSubNexusCheckInMigrationContract(t *testing.T) {
	b, err := FS.ReadFile("9002_subnexus_checkin.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(b))
	for _, want := range []string{
		"create table if not exists activity_reward_logs",
		"create table if not exists activity_checkin_streaks",
		"idx_activity_reward_logs_source_period_user",
		"idx_activity_reward_logs_user_created",
		"idx_activity_reward_logs_source_created",
		"idx_activity_reward_logs_user_source_period",
		"idx_activity_reward_logs_source_period_ip",
		"idx_activity_reward_logs_frozen",
		"add column if not exists rank",
		"add column if not exists amount",
		"add column if not exists note",
		"add column if not exists ip",
		"add column if not exists frozen",
		"add column if not exists current_streak",
		"add column if not exists last_checkin_date",
		"add column if not exists updated_at",
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("missing %q", want)
		}
	}

	// This migration must be safe to apply to a live database while the
	// feature remains disabled.  In particular, it may not seed settings or
	// mutate/rewrite historical reward rows.
	sqlWithoutComments := strings.ToLower(stripSQLLineComments(string(b)))
	for _, forbidden := range []string{
		`\binsert\s+into\b`,
		`\bupdate\s+`,
		`\bdelete\s+from\b`,
		`\bdrop\s+`,
		`\btruncate\s+`,
		`\bsubnexus_checkin_enabled\b`,
	} {
		if regexp.MustCompile(forbidden).MatchString(sqlWithoutComments) {
			t.Errorf("migration contains forbidden side effect %q", forbidden)
		}
	}
}
