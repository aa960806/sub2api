package migrations

import (
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// 234a is the bridge that lets a 0.2.4 binary and a pre-0.2.4 binary use the
// same database during the rollback window.  Keep the important safety
// properties in a small source contract test so a future edit cannot silently
// turn the bridge into a destructive rename or a last-write-wins copy.
func TestGroupModelAllowlistLegacyCompatMigrationContract(t *testing.T) {
	content, err := FS.ReadFile("234a_group_model_allowlist_legacy_compat.sql")
	require.NoError(t, err)
	sql := strings.Join(strings.Fields(string(content)), " ")

	require.Contains(t, sql, "ADD COLUMN model_allowlist JSONB NOT NULL DEFAULT")
	require.Contains(t, sql, "ADD COLUMN models_list_config JSONB NOT NULL DEFAULT")
	require.Contains(t, sql, "IS DISTINCT FROM")
	require.Contains(t, sql, "conflicting non-empty values")
	require.Contains(t, sql, "unsynchronized values")
	require.Contains(t, sql, "CREATE OR REPLACE FUNCTION public.sub2api_sync_group_model_allowlist_compat()")
	require.Contains(t, sql, "SET search_path = pg_catalog")
	require.Contains(t, sql, "BEFORE INSERT OR UPDATE OF model_allowlist, models_list_config")
	require.Contains(t, sql, "CREATE TRIGGER sub2api_group_model_allowlist_compat_trg")
	require.Contains(t, sql, "IF existing_function IS NULL")

	// The bridge must never remove either column.  235/236 remain responsible
	// for the historical rename/repair behavior; 234a only adds compatibility.
	require.NotContains(t, sql, "DROP COLUMN")
	require.NotContains(t, sql, "RENAME COLUMN")
}

func TestGroupModelAllowlistLegacyCompatMigrationIsOrderedBeforeRename(t *testing.T) {
	files, err := FS.ReadDir(".")
	require.NoError(t, err)
	var names []string
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".sql") {
			names = append(names, file.Name())
		}
	}
	sort.Strings(names)
	compatIndex := sort.SearchStrings(names, "234a_group_model_allowlist_legacy_compat.sql")
	renameIndex := sort.SearchStrings(names, "235_group_model_allowlist.sql")
	require.Less(t, compatIndex, len(names))
	require.Equal(t, "234a_group_model_allowlist_legacy_compat.sql", names[compatIndex])
	require.Less(t, renameIndex, len(names))
	require.Equal(t, "235_group_model_allowlist.sql", names[renameIndex])
	require.Less(t, compatIndex, renameIndex)
}
