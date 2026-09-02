package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBattlePassMigrationCreatesIsolatedTables(t *testing.T) {
	content, err := FS.ReadFile("9006_subnexus_battle_pass.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(content))
	require.Contains(t, sql, "create table if not exists battle_pass_seasons")
	require.Contains(t, sql, "create table if not exists battle_pass_pause_windows")
	require.Contains(t, sql, "create table if not exists battle_pass_purchases")
	require.NotContains(t, sql, "alter table users")
	require.NotContains(t, sql, "alter table usage_logs")
	require.NotContains(t, sql, "alter table payment_orders")
}
