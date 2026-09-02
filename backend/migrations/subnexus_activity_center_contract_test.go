package migrations

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const subnexusActivityCenterMigration = "9001_subnexus_activity_center.sql"

func TestSubNexusActivityCenterMigrationIsEmbeddedOnceAndUsesIdempotentDDL(t *testing.T) {
	content, err := FS.ReadFile(subnexusActivityCenterMigration)
	require.NoError(t, err)

	files, err := fs.Glob(FS, subnexusActivityCenterMigration)
	require.NoError(t, err)
	require.Equal(t, []string{subnexusActivityCenterMigration}, files)

	sql := strings.Join(strings.Fields(stripSQLLineComments(string(content))), " ")
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS activity_center_items")
	require.Contains(t, sql, "CREATE INDEX IF NOT EXISTS idx_activity_center_items_visible")
	require.Contains(t, sql, "CREATE INDEX IF NOT EXISTS idx_activity_center_items_window")

	// This is a compatibility shell for the old 153 table.  Keep the column
	// contract stable so an existing production table can be reused without a
	// rename, data copy, or destructive ALTER.
	for _, column := range []string{
		"id BIGSERIAL PRIMARY KEY",
		"slug VARCHAR(80) NOT NULL UNIQUE",
		"title VARCHAR(120) NOT NULL",
		"subtitle VARCHAR(240) NOT NULL DEFAULT ''",
		"description TEXT NOT NULL DEFAULT ''",
		"icon VARCHAR(64) NOT NULL DEFAULT 'gift'",
		"cover_url TEXT NOT NULL DEFAULT ''",
		"route_path VARCHAR(255) NOT NULL DEFAULT ''",
		"external_url TEXT NOT NULL DEFAULT ''",
		"action_label VARCHAR(40) NOT NULL DEFAULT ''",
		"activity_type VARCHAR(32) NOT NULL DEFAULT 'custom'",
		"enabled BOOLEAN NOT NULL DEFAULT TRUE",
		"sort_order INTEGER NOT NULL DEFAULT 0",
		"start_at TIMESTAMPTZ",
		"end_at TIMESTAMPTZ",
		"metadata JSONB NOT NULL DEFAULT '{}'::jsonb",
		"created_by BIGINT",
		"created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()",
		"updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()",
	} {
		require.Contains(t, sql, column)
	}
}

func TestSubNexusActivityCenterMigrationHasNoSeedOrDestructiveStatements(t *testing.T) {
	content, err := FS.ReadFile(subnexusActivityCenterMigration)
	require.NoError(t, err)
	sql := stripSQLLineComments(string(content))

	for _, forbidden := range []string{
		"INSERT", "UPDATE", "DELETE", "DROP", "TRUNCATE", "ALTER",
		"ACTIVITY_CENTER_CONFIG",
	} {
		require.NotRegexp(t, regexp.MustCompile(`(?i)\b`+forbidden+`\b`), sql,
			"activity-center compatibility migration must not seed, rewrite, or delete existing data")
	}
	// The migration itself must not turn on the runtime switch.  The setting is
	// created/updated explicitly by an administrator after local acceptance.
	require.NotContains(t, strings.ToLower(sql), "subnexus_activity_center_enabled")
}

func stripSQLLineComments(value string) string {
	lines := strings.Split(value, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "--") || trimmed == "" {
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.Join(filtered, "\n")
}
