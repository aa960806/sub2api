//go:build integration

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

const (
	groupModelAllowlistCompatTrigger = "sub2api_group_model_allowlist_compat_trg"
)

// The integration harness has already applied all migrations, including 234a.
// Each test therefore changes the groups shape inside its own transaction,
// removes the installed trigger, and replays 234a.  Transactional DDL rolls
// back with the test and leaves the shared harness database unchanged.
func TestMigration234aLegacyOnlyAddsColumnAndSynchronizesBothDirections(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()
	setGroupModelAllowlistShape(t, tx, "legacy-only")

	var id int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO groups (name, platform, rate_multiplier, status, models_list_config)
VALUES ('migration-234a-legacy-only', 'anthropic', 1, 'active', '{"source":"legacy-insert"}'::jsonb)
RETURNING id
`).Scan(&id))

	applyGroupModelAllowlistLegacyCompat(ctx, t, tx)
	requireGroupModelAllowlistCompatShape(ctx, t, tx)
	requireGroupModelAllowlistCompatTrigger(ctx, t, tx)
	requireGroupModelAllowlistCompatPair(ctx, t, tx, id, `{"source":"legacy-insert"}`)

	// An old binary updates only models_list_config; the new column must follow.
	_, err := tx.ExecContext(ctx,
		`UPDATE groups SET models_list_config = '{"source":"legacy-update"}'::jsonb WHERE id = $1`, id)
	require.NoError(t, err)
	requireGroupModelAllowlistCompatPair(ctx, t, tx, id, `{"source":"legacy-update"}`)

	// A new binary updates only model_allowlist; the legacy column must follow.
	_, err = tx.ExecContext(ctx,
		`UPDATE groups SET model_allowlist = '{"source":"modern-update"}'::jsonb WHERE id = $1`, id)
	require.NoError(t, err)
	requireGroupModelAllowlistCompatPair(ctx, t, tx, id, `{"source":"modern-update"}`)

	// Inserts from either binary are also accepted and mirrored.
	var legacyInsertID, modernInsertID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO groups (name, platform, rate_multiplier, status, models_list_config)
VALUES ('migration-234a-legacy-insert-2', 'anthropic', 1, 'active', '{"source":"legacy-insert-2"}'::jsonb)
RETURNING id
`).Scan(&legacyInsertID))
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO groups (name, platform, rate_multiplier, status, model_allowlist)
VALUES ('migration-234a-modern-insert', 'anthropic', 1, 'active', '{"source":"modern-insert"}'::jsonb)
RETURNING id
`).Scan(&modernInsertID))
	requireGroupModelAllowlistCompatPair(ctx, t, tx, legacyInsertID, `{"source":"legacy-insert-2"}`)
	requireGroupModelAllowlistCompatPair(ctx, t, tx, modernInsertID, `{"source":"modern-insert"}`)

	_, err = tx.ExecContext(ctx, "SAVEPOINT compat_insert_conflict")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `
INSERT INTO groups (name, platform, rate_multiplier, status, model_allowlist, models_list_config)
VALUES ('migration-234a-insert-conflict', 'anthropic', 1, 'active', '{"source":"new"}'::jsonb, '{"source":"old"}'::jsonb)
`)
	require.Error(t, err)
	require.ErrorContains(t, err, "columns must agree on insert")
	_, err = tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT compat_insert_conflict")
	require.NoError(t, err)
}

func TestMigration234aModernOnlyAddsLegacyColumnAndIsIdempotent(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()
	setGroupModelAllowlistShape(t, tx, "modern-only")

	var id int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO groups (name, platform, rate_multiplier, status, model_allowlist)
VALUES ('migration-234a-modern-only', 'anthropic', 1, 'active', '{"source":"modern-before"}'::jsonb)
RETURNING id
`).Scan(&id))

	applyGroupModelAllowlistLegacyCompat(ctx, t, tx)
	requireGroupModelAllowlistCompatShape(ctx, t, tx)
	requireGroupModelAllowlistCompatTrigger(ctx, t, tx)
	requireGroupModelAllowlistCompatPair(ctx, t, tx, id, `{"source":"modern-before"}`)

	// Running the compatibility migration again must leave data and objects
	// intact instead of attempting to recreate an existing trigger.
	applyGroupModelAllowlistLegacyCompat(ctx, t, tx)
	requireGroupModelAllowlistCompatPair(ctx, t, tx, id, `{"source":"modern-before"}`)
	requireGroupModelAllowlistCompatTrigger(ctx, t, tx)
}

func TestMigration234aBackfillsEmptySideAndPreservesEmptyRows(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()
	setGroupModelAllowlistShape(t, tx, "both")

	var oldOnlyID, newOnlyID, emptyID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO groups (name, platform, rate_multiplier, status, model_allowlist, models_list_config)
VALUES ('migration-234a-backfill-old', 'anthropic', 1, 'active', '{}'::jsonb, '{"source":"old"}'::jsonb)
RETURNING id
`).Scan(&oldOnlyID))
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO groups (name, platform, rate_multiplier, status, model_allowlist, models_list_config)
VALUES ('migration-234a-backfill-new', 'anthropic', 1, 'active', '{"source":"new"}'::jsonb, '{}'::jsonb)
RETURNING id
`).Scan(&newOnlyID))
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO groups (name, platform, rate_multiplier, status, model_allowlist, models_list_config)
VALUES ('migration-234a-backfill-empty', 'anthropic', 1, 'active', '{}'::jsonb, '{}'::jsonb)
RETURNING id
`).Scan(&emptyID))

	applyGroupModelAllowlistLegacyCompat(ctx, t, tx)
	requireGroupModelAllowlistCompatPair(ctx, t, tx, oldOnlyID, `{"source":"old"}`)
	requireGroupModelAllowlistCompatPair(ctx, t, tx, newOnlyID, `{"source":"new"}`)
	requireGroupModelAllowlistCompatPair(ctx, t, tx, emptyID, `{}`)
}

func TestMigration234aRejectsConflictingNonEmptyValues(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()
	setGroupModelAllowlistShape(t, tx, "both")

	_, err := tx.ExecContext(ctx, `
INSERT INTO groups (name, platform, rate_multiplier, status, model_allowlist, models_list_config)
VALUES ('migration-234a-conflict', 'anthropic', 1, 'active', '{"source":"new"}'::jsonb, '{"source":"old"}'::jsonb)
`)
	require.NoError(t, err)

	migrationSQL, err := dbmigrations.FS.ReadFile(groupModelAllowlistLegacyCompatMigration)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.Error(t, err)
	require.ErrorContains(t, err, "conflicting non-empty values")
}

func TestMigration234aRejectsConflictingUpdate(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()
	setGroupModelAllowlistShape(t, tx, "both")

	var id int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO groups (name, platform, rate_multiplier, status, model_allowlist, models_list_config)
VALUES ('migration-234a-update-conflict', 'anthropic', 1, 'active', '{"source":"same"}'::jsonb, '{"source":"same"}'::jsonb)
RETURNING id
`).Scan(&id))
	applyGroupModelAllowlistLegacyCompat(ctx, t, tx)

	// A caller that explicitly writes different values to both columns must be
	// rejected instead of silently choosing whichever column happened to be
	// listed last.  Roll back to a savepoint so the rest of the fixture remains
	// queryable after PostgreSQL marks the statement failed.
	_, err := tx.ExecContext(ctx, "SAVEPOINT compat_update_conflict")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `
UPDATE groups
SET model_allowlist = '{"source":"new"}'::jsonb,
    models_list_config = '{"source":"old"}'::jsonb
WHERE id = $1`, id)
	require.Error(t, err)
	require.ErrorContains(t, err, "columns must agree on update")
	_, err = tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT compat_update_conflict")
	require.NoError(t, err)
	requireGroupModelAllowlistCompatPair(ctx, t, tx, id, `{"source":"same"}`)
}

func setGroupModelAllowlistShape(t *testing.T, tx *sql.Tx, shape string) {
	t.Helper()
	dropGroupModelAllowlistCompatTrigger(t, tx)
	ctx := context.Background()
	_, err := tx.ExecContext(ctx, `ALTER TABLE groups DROP COLUMN IF EXISTS model_allowlist`)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `ALTER TABLE groups DROP COLUMN IF EXISTS models_list_config`)
	require.NoError(t, err)

	switch shape {
	case "legacy-only":
		addGroupModelAllowlistColumn(t, tx, "models_list_config")
	case "modern-only":
		addGroupModelAllowlistColumn(t, tx, "model_allowlist")
	case "both":
		addGroupModelAllowlistColumn(t, tx, "model_allowlist")
		addGroupModelAllowlistColumn(t, tx, "models_list_config")
	default:
		t.Fatalf("unknown group model allowlist fixture shape %q", shape)
	}
	_, err = tx.ExecContext(ctx, `DELETE FROM groups WHERE name LIKE 'migration-234a-%'`)
	require.NoError(t, err)
}

func dropGroupModelAllowlistCompatTrigger(t *testing.T, tx *sql.Tx) {
	t.Helper()
	_, err := tx.ExecContext(context.Background(), `DROP TRIGGER IF EXISTS sub2api_group_model_allowlist_compat_trg ON groups`)
	require.NoError(t, err)
}

func addGroupModelAllowlistColumn(t *testing.T, tx *sql.Tx, column string) {
	t.Helper()
	// Column names are constants selected above, never caller input.
	_, err := tx.ExecContext(context.Background(), fmt.Sprintf(
		`ALTER TABLE groups ADD COLUMN %s JSONB NOT NULL DEFAULT '{}'::jsonb`, column))
	require.NoError(t, err)
}

func applyGroupModelAllowlistLegacyCompat(ctx context.Context, t *testing.T, tx *sql.Tx) {
	t.Helper()
	migrationSQL, err := dbmigrations.FS.ReadFile(groupModelAllowlistLegacyCompatMigration)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err)
}

func requireGroupModelAllowlistCompatShape(ctx context.Context, t *testing.T, tx *sql.Tx) {
	t.Helper()
	for _, column := range []string{"model_allowlist", "models_list_config"} {
		var nullable, dataType, defaultValue string
		require.NoError(t, tx.QueryRowContext(ctx, `
SELECT is_nullable, data_type, COALESCE(column_default, '')
FROM information_schema.columns
WHERE table_schema = current_schema() AND table_name = 'groups' AND column_name = $1
`, column).Scan(&nullable, &dataType, &defaultValue))
		require.Equal(t, "NO", nullable, "%s nullability", column)
		require.Equal(t, "jsonb", dataType, "%s data type", column)
		require.Contains(t, defaultValue, "'{}'::jsonb", "%s default", column)
	}
}

func requireGroupModelAllowlistCompatTrigger(ctx context.Context, t *testing.T, tx *sql.Tx) {
	t.Helper()
	var exists bool
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1 FROM pg_trigger
  WHERE tgrelid = 'groups'::regclass
    AND tgname = $1
    AND NOT tgisinternal
)
`, groupModelAllowlistCompatTrigger).Scan(&exists))
	require.True(t, exists, "compatibility trigger should be installed")
}

func requireGroupModelAllowlistCompatPair(ctx context.Context, t *testing.T, tx *sql.Tx, id int64, expected string) {
	t.Helper()
	var modern, legacy string
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT model_allowlist::text, models_list_config::text FROM groups WHERE id = $1
`, id).Scan(&modern, &legacy))
	require.JSONEq(t, expected, modern)
	require.JSONEq(t, expected, legacy)
}
