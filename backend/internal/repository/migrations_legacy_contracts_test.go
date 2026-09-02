package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"testing/fstest"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestValidateLegacyMigrationAliasMap(t *testing.T) {
	require.NoError(t, validateLegacyMigrationAliasMap())
	require.Len(t, migrationAliasContracts, len(legacyMigrationAliases))
}

func TestAuthCacheTriggerContractUsesPostgresCanonicalEventOrder(t *testing.T) {
	contract, ok := migrationAliasContracts["184_auth_cache_invalidation_outbox.sql"]
	require.True(t, ok)

	definitions := map[string]string{
		"trg_api_keys_auth_cache_invalidation":            "CREATE TRIGGER trg_api_keys_auth_cache_invalidation AFTER DELETE OR UPDATE ON public.api_keys FOR EACH ROW EXECUTE FUNCTION enqueue_api_key_auth_cache_invalidation()",
		"trg_users_auth_cache_invalidation":               "CREATE TRIGGER trg_users_auth_cache_invalidation AFTER DELETE OR UPDATE ON public.users FOR EACH ROW EXECUTE FUNCTION enqueue_user_auth_cache_invalidation()",
		"trg_groups_auth_cache_invalidation":              "CREATE TRIGGER trg_groups_auth_cache_invalidation AFTER DELETE OR UPDATE ON public.groups FOR EACH ROW EXECUTE FUNCTION enqueue_group_auth_cache_invalidation()",
		"trg_user_allowed_groups_auth_cache_invalidation": "CREATE TRIGGER trg_user_allowed_groups_auth_cache_invalidation AFTER INSERT OR DELETE OR UPDATE ON public.user_allowed_groups FOR EACH ROW EXECUTE FUNCTION enqueue_allowed_group_auth_cache_invalidation()",
	}

	for _, trigger := range contract.triggers {
		definition, ok := definitions[trigger.name]
		require.Truef(t, ok, "missing fixture for trigger %s", trigger.name)
		for _, fragment := range trigger.fragments {
			require.Truef(t, migrationContractTextContains(definition, fragment),
				"canonical trigger definition for %s should contain %q", trigger.name, fragment)
		}
	}
}

func TestApplyMigrationsFS_LegacyAliasAdoptsMetadataAfterContractValidation(t *testing.T) {
	const targetName = "219_group_search_price_per_1k.sql"
	const legacyName = "237_group_search_price_per_1k.sql"
	const checksum = "430c2e3595342fe22c59e9676e9b18ea376f076324b77174a21e6f181f57f4b5"

	content, err := migrations.FS.ReadFile(targetName)
	require.NoError(t, err)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	prepareMigrationsBootstrapExpectations(mock)
	mock.ExpectQuery(`SELECT checksum FROM schema_migrations WHERE filename = \$1`).
		WithArgs(targetName).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`SELECT checksum FROM schema_migrations WHERE filename = \$1`).
		WithArgs(legacyName).
		WillReturnRows(sqlmock.NewRows([]string{"checksum"}).AddRow(checksum))
	mock.ExpectQuery(`SELECT EXISTS \(`).
		WithArgs("groups").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT format_type\(`).
		WithArgs("groups", "search_price_per_1k").
		WillReturnRows(sqlmock.NewRows([]string{"format_type", "attnotnull"}).AddRow("numeric(20,8)", false))
	mock.ExpectExec(`INSERT INTO schema_migrations`).
		WithArgs(targetName, checksum).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(`SELECT checksum FROM schema_migrations WHERE filename = \$1`).
		WithArgs(targetName).
		WillReturnRows(sqlmock.NewRows([]string{"checksum"}).AddRow(checksum))
	mock.ExpectExec(`SELECT pg_advisory_unlock`).
		WithArgs(migrationsAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = applyMigrationsFS(context.Background(), db, fstest.MapFS{
		targetName: &fstest.MapFile{Data: content},
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyMigrationsFS_LegacyAliasAdoptsDifferentAuditedChecksum(t *testing.T) {
	const targetName = "189_add_group_allow_live.sql"
	const legacyName = "194_add_group_allow_live.sql"
	const targetChecksum = "51172b10c160e7f560346dbaf736dc8e92feb793cd00169f5fb876c399460862"
	const legacyChecksum = "d6d2e6ac7f201da0cebcc81bdc7b8a5ffff7f93abfb149f17d3dd609fa316ea6"

	content, err := migrations.FS.ReadFile(targetName)
	require.NoError(t, err)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	prepareMigrationsBootstrapExpectations(mock)
	mock.ExpectQuery(`SELECT checksum FROM schema_migrations WHERE filename = \$1`).
		WithArgs(targetName).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`SELECT checksum FROM schema_migrations WHERE filename = \$1`).
		WithArgs(legacyName).
		WillReturnRows(sqlmock.NewRows([]string{"checksum"}).AddRow(legacyChecksum))
	mock.ExpectQuery(`SELECT EXISTS \(`).
		WithArgs("groups").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT format_type\(`).
		WithArgs("groups", "allow_live").
		WillReturnRows(sqlmock.NewRows([]string{"format_type", "attnotnull"}).AddRow("boolean", true))
	mock.ExpectQuery(`SELECT COALESCE\(`).
		WillReturnRows(sqlmock.NewRows([]string{"ok"}).AddRow(true))
	mock.ExpectExec(`INSERT INTO schema_migrations`).
		WithArgs(targetName, targetChecksum).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(`SELECT checksum FROM schema_migrations WHERE filename = \$1`).
		WithArgs(targetName).
		WillReturnRows(sqlmock.NewRows([]string{"checksum"}).AddRow(targetChecksum))
	mock.ExpectExec(`SELECT pg_advisory_unlock`).
		WithArgs(migrationsAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = applyMigrationsFS(context.Background(), db, fstest.MapFS{
		targetName: &fstest.MapFile{Data: content},
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyMigrationsFS_LegacyReplayRunsPreludeAndTargetAtomically(t *testing.T) {
	const targetName = "999_test_legacy_replay.sql"
	const legacyName = "998_test_legacy_replay.sql"
	const targetChecksum = "17db4fd369edb9244b9f91d9aeed145c3d04ad8ba6e95d06247f07a63527d11a"
	const legacyChecksum = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	const prelude = "SET LOCAL search_path = public; SELECT 2;"
	const content = "SELECT 1;"

	legacyMigrationAliases[targetName] = legacyMigrationAlias{
		legacyFilename: legacyName,
		checksum:       targetChecksum,
		legacyChecksum: legacyChecksum,
		replayPrelude:  prelude,
	}
	migrationAliasContracts[targetName] = migrationAliasContract{}
	defer func() {
		delete(legacyMigrationAliases, targetName)
		delete(migrationAliasContracts, targetName)
	}()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	prepareMigrationsBootstrapExpectations(mock)
	mock.ExpectQuery(`SELECT checksum FROM schema_migrations WHERE filename = \$1`).
		WithArgs(targetName).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`SELECT checksum FROM schema_migrations WHERE filename = \$1`).
		WithArgs(legacyName).
		WillReturnRows(sqlmock.NewRows([]string{"checksum"}).AddRow(legacyChecksum))
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT 2;`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`SELECT 1;`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO schema_migrations`).
		WithArgs(targetName, targetChecksum).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(`SELECT checksum FROM schema_migrations WHERE filename = \$1`).
		WithArgs(targetName).
		WillReturnRows(sqlmock.NewRows([]string{"checksum"}).AddRow(targetChecksum))
	mock.ExpectCommit()
	mock.ExpectExec(`SELECT pg_advisory_unlock`).
		WithArgs(migrationsAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = applyMigrationsFS(context.Background(), db, fstest.MapFS{
		targetName: &fstest.MapFile{Data: []byte(content)},
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyMigrationsFS_ExistingSemanticAliasRecordRequiresContract(t *testing.T) {
	const targetName = "189_add_group_allow_live.sql"
	const targetChecksum = "51172b10c160e7f560346dbaf736dc8e92feb793cd00169f5fb876c399460862"

	content, err := migrations.FS.ReadFile(targetName)
	require.NoError(t, err)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	prepareMigrationsBootstrapExpectations(mock)
	mock.ExpectQuery(`SELECT checksum FROM schema_migrations WHERE filename = \$1`).
		WithArgs(targetName).
		WillReturnRows(sqlmock.NewRows([]string{"checksum"}).AddRow(targetChecksum))
	mock.ExpectQuery(`SELECT EXISTS \(`).
		WithArgs("groups").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT format_type\(`).
		WithArgs("groups", "allow_live").
		WillReturnRows(sqlmock.NewRows([]string{"format_type", "attnotnull"}).AddRow("boolean", true))
	mock.ExpectQuery(`SELECT COALESCE\(`).
		WillReturnRows(sqlmock.NewRows([]string{"ok"}).AddRow(true))
	mock.ExpectExec(`SELECT pg_advisory_unlock`).
		WithArgs(migrationsAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = applyMigrationsFS(context.Background(), db, fstest.MapFS{
		targetName: &fstest.MapFile{Data: content},
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyMigrationsFS_ExistingSemanticAliasRecordRejectsMissingContract(t *testing.T) {
	const targetName = "189_add_group_allow_live.sql"
	const targetChecksum = "51172b10c160e7f560346dbaf736dc8e92feb793cd00169f5fb876c399460862"

	content, err := migrations.FS.ReadFile(targetName)
	require.NoError(t, err)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	prepareMigrationsBootstrapExpectations(mock)
	mock.ExpectQuery(`SELECT checksum FROM schema_migrations WHERE filename = \$1`).
		WithArgs(targetName).
		WillReturnRows(sqlmock.NewRows([]string{"checksum"}).AddRow(targetChecksum))
	mock.ExpectQuery(`SELECT EXISTS \(`).
		WithArgs("groups").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec(`SELECT pg_advisory_unlock`).
		WithArgs(migrationsAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = applyMigrationsFS(context.Background(), db, fstest.MapFS{
		targetName: &fstest.MapFile{Data: content},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "validate existing migration 189_add_group_allow_live.sql contract")
	require.Contains(t, err.Error(), "table groups is missing")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyMigrationsFS_LegacyReplayConflictingTargetRecordRollsBack(t *testing.T) {
	const targetName = "999_test_legacy_replay.sql"
	const legacyName = "998_test_legacy_replay.sql"
	const targetChecksum = "17db4fd369edb9244b9f91d9aeed145c3d04ad8ba6e95d06247f07a63527d11a"
	const legacyChecksum = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	const prelude = "SELECT 2;"
	const content = "SELECT 1;"
	const conflictingChecksum = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"

	legacyMigrationAliases[targetName] = legacyMigrationAlias{
		legacyFilename: legacyName,
		checksum:       targetChecksum,
		legacyChecksum: legacyChecksum,
		replayPrelude:  prelude,
	}
	migrationAliasContracts[targetName] = migrationAliasContract{}
	defer func() {
		delete(legacyMigrationAliases, targetName)
		delete(migrationAliasContracts, targetName)
	}()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	prepareMigrationsBootstrapExpectations(mock)
	mock.ExpectQuery(`SELECT checksum FROM schema_migrations WHERE filename = \$1`).
		WithArgs(targetName).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`SELECT checksum FROM schema_migrations WHERE filename = \$1`).
		WithArgs(legacyName).
		WillReturnRows(sqlmock.NewRows([]string{"checksum"}).AddRow(legacyChecksum))
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT 2;`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`SELECT 1;`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO schema_migrations`).
		WithArgs(targetName, targetChecksum).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT checksum FROM schema_migrations WHERE filename = \$1`).
		WithArgs(targetName).
		WillReturnRows(sqlmock.NewRows([]string{"checksum"}).AddRow(conflictingChecksum))
	mock.ExpectRollback()
	mock.ExpectExec(`SELECT pg_advisory_unlock`).
		WithArgs(migrationsAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = applyMigrationsFS(context.Background(), db, fstest.MapFS{
		targetName: &fstest.MapFile{Data: []byte(content)},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "target migration record checksum mismatch")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestValidateMigrationAliasContract_GrokPlatformNullabilityIsExplicit(t *testing.T) {
	contract, ok := migrationAliasContracts["158_enable_grok_media_generation_groups.sql"]
	require.True(t, ok)

	var platform migrationColumnContract
	for _, column := range contract.columns {
		if column.table == "groups" && column.column == "platform" {
			platform = column
			break
		}
	}
	require.Equal(t, migrationColumnContract{
		table:    "groups",
		column:   "platform",
		dataType: "character varying(50)",
		notNull:  true,
	}, platform)
}

func TestApplyMigrationsFS_LegacyAliasAdoptsNonTransactionalIndexWithoutReplayingSQL(t *testing.T) {
	const targetName = "174_add_usage_logs_api_key_latest_ip_index_notx.sql"
	const legacyName = "204_add_usage_logs_api_key_latest_ip_index_notx.sql"
	const checksum = "aec8fad2bbb6993340ac93762bd8df62fccc72a41ffeb63181ee8fa58f223a1d"

	content, err := migrations.FS.ReadFile(targetName)
	require.NoError(t, err)
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	prepareMigrationsBootstrapExpectations(mock)
	mock.ExpectQuery(`SELECT checksum FROM schema_migrations WHERE filename = \$1`).
		WithArgs(targetName).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`SELECT checksum FROM schema_migrations WHERE filename = \$1`).
		WithArgs(legacyName).
		WillReturnRows(sqlmock.NewRows([]string{"checksum"}).AddRow(checksum))
	mock.ExpectQuery(`SELECT EXISTS \(`).
		WithArgs("usage_logs").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT i.indisvalid, i.indisready").
		WithArgs("idx_usage_logs_api_key_latest_ip", "usage_logs").
		WillReturnRows(sqlmock.NewRows([]string{"indisvalid", "indisready", "pg_get_indexdef"}).AddRow(
			true,
			true,
			"CREATE INDEX idx_usage_logs_api_key_latest_ip ON public.usage_logs (api_key_id, created_at DESC, id DESC) INCLUDE (ip_address) WHERE ((ip_address IS NOT NULL) AND ((ip_address)::text <> ''::text))",
		))
	mock.ExpectExec(`INSERT INTO schema_migrations`).
		WithArgs(targetName, checksum).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(`SELECT checksum FROM schema_migrations WHERE filename = \$1`).
		WithArgs(targetName).
		WillReturnRows(sqlmock.NewRows([]string{"checksum"}).AddRow(checksum))
	mock.ExpectExec(`SELECT pg_advisory_unlock`).
		WithArgs(migrationsAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = applyMigrationsFS(context.Background(), db, fstest.MapFS{
		targetName: &fstest.MapFile{Data: content},
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyMigrationsFS_LegacyAliasMissingFallsBackToNormalExecution(t *testing.T) {
	const targetName = "219_group_search_price_per_1k.sql"
	const sqlText = "ALTER TABLE groups ADD COLUMN IF NOT EXISTS search_price_per_1k DECIMAL(20,8);"

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	prepareMigrationsBootstrapExpectations(mock)
	mock.ExpectQuery(`SELECT checksum FROM schema_migrations WHERE filename = \$1`).
		WithArgs(targetName).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`SELECT checksum FROM schema_migrations WHERE filename = \$1`).
		WithArgs("237_group_search_price_per_1k.sql").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectBegin()
	mock.ExpectExec("ALTER TABLE groups ADD COLUMN IF NOT EXISTS search_price_per_1k").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`INSERT INTO schema_migrations`).
		WithArgs(targetName, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	mock.ExpectExec(`SELECT pg_advisory_unlock`).
		WithArgs(migrationsAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = applyMigrationsFS(context.Background(), db, fstest.MapFS{
		targetName: &fstest.MapFile{Data: []byte(sqlText)},
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyMigrationsFS_LegacyAliasChecksumMismatchFailsBeforeContract(t *testing.T) {
	const targetName = "219_group_search_price_per_1k.sql"

	content, err := migrations.FS.ReadFile(targetName)
	require.NoError(t, err)
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	prepareMigrationsBootstrapExpectations(mock)
	mock.ExpectQuery(`SELECT checksum FROM schema_migrations WHERE filename = \$1`).
		WithArgs(targetName).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`SELECT checksum FROM schema_migrations WHERE filename = \$1`).
		WithArgs("237_group_search_price_per_1k.sql").
		WillReturnRows(sqlmock.NewRows([]string{"checksum"}).AddRow("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"))
	mock.ExpectExec(`SELECT pg_advisory_unlock`).
		WithArgs(migrationsAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = applyMigrationsFS(context.Background(), db, fstest.MapFS{
		targetName: &fstest.MapFile{Data: content},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "legacy migration alias")
	require.Contains(t, err.Error(), "checksum mismatch")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyMigrationsFS_LegacyAliasTargetChecksumMismatchFailsClosed(t *testing.T) {
	const targetName = "219_group_search_price_per_1k.sql"
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	prepareMigrationsBootstrapExpectations(mock)
	mock.ExpectQuery(`SELECT checksum FROM schema_migrations WHERE filename = \$1`).
		WithArgs(targetName).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`SELECT checksum FROM schema_migrations WHERE filename = \$1`).
		WithArgs("237_group_search_price_per_1k.sql").
		WillReturnRows(sqlmock.NewRows([]string{"checksum"}).AddRow("430c2e3595342fe22c59e9676e9b18ea376f076324b77174a21e6f181f57f4b5"))
	mock.ExpectExec(`SELECT pg_advisory_unlock`).
		WithArgs(migrationsAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = applyMigrationsFS(context.Background(), db, fstest.MapFS{
		targetName: &fstest.MapFile{Data: []byte("SELECT 1;")},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "target checksum mismatch")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestValidateMigrationAliasContract_ColumnMismatch(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectQuery(`SELECT EXISTS \(`).
		WithArgs("groups").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT format_type\(`).
		WithArgs("groups", "search_price_per_1k").
		WillReturnRows(sqlmock.NewRows([]string{"format_type", "attnotnull"}).AddRow("text", false))

	err = validateMigrationAliasContract(context.Background(), db, "219_group_search_price_per_1k.sql")
	require.Error(t, err)
	require.Contains(t, err.Error(), "column groups.search_price_per_1k mismatch")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestValidateMigrationAliasContract_InvalidIndexFailsClosed(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectQuery(`SELECT EXISTS \(`).
		WithArgs("usage_logs").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT i.indisvalid, i.indisready").
		WithArgs("idx_usage_logs_upstream_model_mismatch_created_at", "usage_logs").
		WillReturnRows(sqlmock.NewRows([]string{"indisvalid", "indisready", "pg_get_indexdef"}).AddRow(false, false, "CREATE INDEX ..."))

	err = validateMigrationAliasContract(context.Background(), db, "195_add_usage_log_upstream_model_mismatch_index_notx.sql")
	require.Error(t, err)
	require.Contains(t, err.Error(), "is invalid")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestValidateMigrationAliasContract_ContractQueryError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectQuery(`SELECT EXISTS \(`).
		WithArgs("groups").
		WillReturnError(errors.New("catalog unavailable"))

	err = validateMigrationAliasContract(context.Background(), db, "219_group_search_price_per_1k.sql")
	require.Error(t, err)
	require.Contains(t, err.Error(), "table groups query")
	require.NoError(t, mock.ExpectationsWereMet())
}
