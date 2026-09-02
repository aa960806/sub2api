//go:build legacyintegration

package repository

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestMigrationsRunner_FullLegacyCheckoutHandoff(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("SUBNEXUS_LEGACY_TEST_DSN"))
	legacyDir := strings.TrimSpace(os.Getenv("SUBNEXUS_LEGACY_MIGRATIONS_DIR"))
	if dsn == "" || legacyDir == "" {
		t.Skip("set SUBNEXUS_LEGACY_TEST_DSN and SUBNEXUS_LEGACY_MIGRATIONS_DIR")
	}

	info, err := os.Stat(legacyDir)
	require.NoError(t, err)
	require.True(t, info.IsDir(), "legacy migrations path must be a directory")

	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	require.NoError(t, db.PingContext(ctx))

	var databaseName string
	require.NoError(t, db.QueryRowContext(ctx, "SELECT current_database()").Scan(&databaseName))
	require.True(t, strings.HasPrefix(databaseName, "subnexus_legacy_handoff_test_"),
		"refusing to reset database without the required test prefix: %s", databaseName)

	_, err = db.ExecContext(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	require.NoError(t, err)

	legacyFS := os.DirFS(filepath.Clean(legacyDir))
	require.NoError(t, applyMigrationsFS(ctx, db, legacyFS), "apply complete legacy migration set")

	var legacyMigrationCount int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&legacyMigrationCount))
	require.GreaterOrEqual(t, legacyMigrationCount, 250)

	require.NoError(t, ApplyMigrations(ctx, db), "handoff legacy schema to current runner")
	require.NoError(t, ApplyMigrations(ctx, db), "current runner must be idempotent after handoff")
	require.NoError(t, applyMigrationsFS(ctx, db, legacyFS), "legacy migration set must remain metadata-compatible")

	assertMigrationChecksum(t, ctx, db, "189_add_group_allow_live.sql", "51172b10c160e7f560346dbaf736dc8e92feb793cd00169f5fb876c399460862")
	assertMigrationChecksum(t, ctx, db, "226_channel_monitor_quota_mode.sql", "c36c6c0ec6cc8727bb986e8cdc645990dcf8dad8f56a8c4647422e24e9dff88d")

	var providerConstraint string
	require.NoError(t, db.QueryRowContext(ctx, `
SELECT pg_get_constraintdef(c.oid)
FROM pg_constraint c
JOIN pg_class rel ON rel.oid = c.conrelid
JOIN pg_namespace ns ON ns.oid = rel.relnamespace
WHERE ns.nspname = 'public'
  AND rel.relname = 'channel_monitors'
  AND c.conname = 'channel_monitors_provider_check'
`).Scan(&providerConstraint))
	for _, provider := range []string{"openai", "anthropic", "gemini", "grok", "antigravity", "kimi", "zhipu", "deepseek"} {
		require.Contains(t, providerConstraint, provider)
	}

	var checkModeConstraint string
	require.NoError(t, db.QueryRowContext(ctx, `
SELECT pg_get_constraintdef(c.oid)
FROM pg_constraint c
JOIN pg_class rel ON rel.oid = c.conrelid
JOIN pg_namespace ns ON ns.oid = rel.relnamespace
WHERE ns.nspname = 'public'
  AND rel.relname = 'channel_monitors'
  AND c.conname = 'channel_monitors_check_mode_check'
`).Scan(&checkModeConstraint))
	for _, mode := range []string{"probe", "quota", "quota_probe"} {
		require.Contains(t, checkModeConstraint, mode)
	}
}

func assertMigrationChecksum(t *testing.T, ctx context.Context, db *sql.DB, filename, expected string) {
	t.Helper()
	var actual string
	require.NoError(t, db.QueryRowContext(ctx,
		"SELECT checksum FROM schema_migrations WHERE filename = $1", filename).Scan(&actual))
	require.Equal(t, expected, actual)
}
