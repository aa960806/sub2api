package repository

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

// TestLegacyMigrationAliasesStaticContract keeps the adoption allowlist
// explicit and tied to the embedded target SQL. It deliberately does not
// inspect the legacy checkout: production adoption is based on the recorded
// legacy filename/checksum, while this test protects the target-side contract.
func TestLegacyMigrationAliasesStaticContract(t *testing.T) {
	const expectedAliasCount = 23

	require.Len(t, legacyMigrationAliases, expectedAliasCount)

	targets := make(map[string]struct{}, len(legacyMigrationAliases))
	legacyNames := make(map[string]struct{}, len(legacyMigrationAliases))

	for targetName, alias := range legacyMigrationAliases {
		require.NotEmpty(t, targetName)
		require.NotEmpty(t, alias.legacyFilename)
		require.NotEqual(t, targetName, alias.legacyFilename,
			"an alias must represent a filename change")

		if _, exists := targets[targetName]; exists {
			t.Fatalf("duplicate target migration alias: %s", targetName)
		}
		targets[targetName] = struct{}{}
		if _, exists := legacyNames[alias.legacyFilename]; exists {
			t.Fatalf("duplicate legacy migration alias: %s", alias.legacyFilename)
		}
		legacyNames[alias.legacyFilename] = struct{}{}

		content, err := fs.ReadFile(migrations.FS, targetName)
		require.NoErrorf(t, err, "target migration %s must be embedded", targetName)
		actual := sha256.Sum256([]byte(strings.TrimSpace(string(content))))
		require.Equal(t, hex.EncodeToString(actual[:]), alias.checksum,
			"target migration %s checksum must match its adoption rule", targetName)
	}

	for targetName := range targets {
		_, overlaps := legacyNames[targetName]
		require.False(t, overlaps,
			"a target migration filename must not also be used as a legacy alias")
	}
}

func TestGroupDuplicateOperationIDLegacyChecksumCompatibility(t *testing.T) {
	const migrationName = "181_group_duplicate_operation_id.sql"
	const legacyChecksum = "cf273ce97ebbd045636fdc724f2c284e8258b7049fdb630e6e6bb1606749f828"
	const targetChecksum = "429011c514dfa3a65dd844cb19dfe32ceeae4068f499b15f915cee97687ed7bd"

	content, err := fs.ReadFile(migrations.FS, migrationName)
	require.NoError(t, err)
	actual := sha256.Sum256([]byte(strings.TrimSpace(string(content))))
	require.Equal(t, targetChecksum, hex.EncodeToString(actual[:]))
	require.True(t, isMigrationChecksumCompatible(migrationName, legacyChecksum, targetChecksum))
	require.False(t, isMigrationChecksumCompatible(migrationName, strings.Repeat("f", 64), targetChecksum))
}
