package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"testing/fstest"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestValidateGroupModelAllowlistLegacyCompatContract(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	expectGroupModelAllowlistLegacyCompatContract(mock, nil)
	err = validateGroupModelAllowlistLegacyCompatContract(context.Background(), db)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestValidateGroupModelAllowlistLegacyCompatContractFailsWhenTriggerMissing(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	expectGroupModelAllowlistLegacyCompatContract(mock, sql.ErrNoRows)
	err = validateGroupModelAllowlistLegacyCompatContract(context.Background(), db)
	require.Error(t, err)
	require.Contains(t, err.Error(), "trigger groups.sub2api_group_model_allowlist_compat_trg")
	require.Contains(t, err.Error(), "is missing")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyMigrationsFS_ExistingGroupModelAllowlistCompatRecordRequiresContract(t *testing.T) {
	const content = "SELECT 1;"
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	prepareMigrationsBootstrapExpectations(mock)
	mock.ExpectQuery("SELECT checksum FROM schema_migrations WHERE filename = \\$1").
		WithArgs(groupModelAllowlistLegacyCompatMigration).
		WillReturnRows(sqlmock.NewRows([]string{"checksum"}).AddRow(migrationChecksum(content)))
	expectGroupModelAllowlistLegacyCompatContract(mock, nil)
	mock.ExpectExec("SELECT pg_advisory_unlock\\(\\$1\\)").
		WithArgs(migrationsAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = applyMigrationsFS(context.Background(), db, fstest.MapFS{
		groupModelAllowlistLegacyCompatMigration: &fstest.MapFile{Data: []byte(content)},
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyMigrationsFS_ExistingGroupModelAllowlistCompatRecordFailsClosedOnDrift(t *testing.T) {
	const content = "SELECT 1;"
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	prepareMigrationsBootstrapExpectations(mock)
	mock.ExpectQuery("SELECT checksum FROM schema_migrations WHERE filename = \\$1").
		WithArgs(groupModelAllowlistLegacyCompatMigration).
		WillReturnRows(sqlmock.NewRows([]string{"checksum"}).AddRow(migrationChecksum(content)))
	expectGroupModelAllowlistLegacyCompatContract(mock, errors.New("trigger disappeared"))
	mock.ExpectExec("SELECT pg_advisory_unlock\\(\\$1\\)").
		WithArgs(migrationsAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = applyMigrationsFS(context.Background(), db, fstest.MapFS{
		groupModelAllowlistLegacyCompatMigration: &fstest.MapFile{Data: []byte(content)},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "validate existing migration 234a_group_model_allowlist_legacy_compat.sql contract")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyMigrationsFS_NewGroupModelAllowlistCompatMigrationRequiresContract(t *testing.T) {
	const content = "SELECT 1;"
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	prepareMigrationsBootstrapExpectations(mock)
	mock.ExpectQuery("SELECT checksum FROM schema_migrations WHERE filename = \\$1").
		WithArgs(groupModelAllowlistLegacyCompatMigration).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectBegin()
	mock.ExpectExec("SELECT 1;").WillReturnResult(sqlmock.NewResult(0, 1))
	expectGroupModelAllowlistLegacyCompatContract(mock, nil)
	mock.ExpectExec("INSERT INTO schema_migrations \\(filename, checksum\\) VALUES \\(\\$1, \\$2\\)").
		WithArgs(groupModelAllowlistLegacyCompatMigration, migrationChecksum(content)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	mock.ExpectExec("SELECT pg_advisory_unlock\\(\\$1\\)").
		WithArgs(migrationsAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = applyMigrationsFS(context.Background(), db, fstest.MapFS{
		groupModelAllowlistLegacyCompatMigration: &fstest.MapFile{Data: []byte(content)},
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func expectGroupModelAllowlistLegacyCompatContract(mock sqlmock.Sqlmock, triggerErr error) {
	mock.ExpectQuery("SELECT EXISTS \\(").
		WithArgs("groups").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	for _, column := range []string{"model_allowlist", "models_list_config"} {
		mock.ExpectQuery("SELECT format_type\\(").
			WithArgs("groups", column).
			WillReturnRows(sqlmock.NewRows([]string{"format_type", "attnotnull"}).AddRow("jsonb", true))
		mock.ExpectQuery("SELECT pg_get_expr\\(").
			WithArgs("groups", column).
			WillReturnRows(sqlmock.NewRows([]string{"pg_get_expr"}).AddRow("'{}'::jsonb"))
	}
	mock.ExpectQuery("SELECT pg_get_functiondef\\(").
		WithArgs("sub2api_sync_group_model_allowlist_compat", 0).
		WillReturnRows(sqlmock.NewRows([]string{"pg_get_functiondef", "pg_get_function_result"}).AddRow(
			"CREATE FUNCTION ... LANGUAGE plpgsql ... NEW.model_allowlist ... NEW.models_list_config ... IS DISTINCT FROM ...",
			"trigger",
		))
	triggerExpectation := mock.ExpectQuery("SELECT pg_get_triggerdef\\(").
		WithArgs("groups", "sub2api_group_model_allowlist_compat_trg")
	if triggerErr != nil {
		triggerExpectation.WillReturnError(triggerErr)
	} else {
		triggerExpectation.WillReturnRows(sqlmock.NewRows([]string{"pg_get_triggerdef"}).AddRow(
			"CREATE TRIGGER sub2api_group_model_allowlist_compat_trg BEFORE INSERT OR UPDATE OF model_allowlist, models_list_config ON public.groups EXECUTE FUNCTION public.sub2api_sync_group_model_allowlist_compat()",
		))
		mock.ExpectQuery("SELECT NOT EXISTS \\(").
			WillReturnRows(sqlmock.NewRows([]string{"?column?"}).AddRow(true))
	}
}
