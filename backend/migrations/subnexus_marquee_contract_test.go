package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSubNexusMarqueeMigrationIsAdditiveAndAnnouncementIndependent(t *testing.T) {
	content, err := FS.ReadFile("9007_subnexus_marquee.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(content))

	require.Contains(t, sql, "create table if not exists activity_broadcasts")
	require.Contains(t, sql, "where source = 'admin'")
	require.Contains(t, sql, "create index if not exists")
	require.NotContains(t, sql, "drop table")
	require.NotContains(t, sql, "truncate")
	require.NotContains(t, sql, "delete from")
	require.NotContains(t, sql, "from announcements")
	require.NotContains(t, sql, "into announcements")
	require.NotContains(t, sql, "daily_spin")
	require.NotContains(t, sql, "red_packet")
	require.NotContains(t, sql, "invite_lottery")
	require.NotContains(t, sql, "recharge_wheel")
}
