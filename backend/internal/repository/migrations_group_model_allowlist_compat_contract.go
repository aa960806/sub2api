package repository

import "context"

const groupModelAllowlistLegacyCompatMigration = "234a_group_model_allowlist_legacy_compat.sql"

// groupModelAllowlistLegacyCompatSchemaContract is checked whenever the
// migration record is already present.  A filename/checksum row alone cannot
// prove that an operator did not drop one of the compatibility columns or
// disable/remove the trigger afterwards; fail closed before starting an
// application that would make same-database rollback unsafe.
var groupModelAllowlistLegacyCompatSchemaContract = migrationAliasContract{
	tables: []string{"groups"},
	columns: []migrationColumnContract{
		{table: "groups", column: "model_allowlist", dataType: "jsonb", notNull: true, defaultContains: "{}"},
		{table: "groups", column: "models_list_config", dataType: "jsonb", notNull: true, defaultContains: "{}"},
	},
	functions: []migrationFunctionContract{{
		name:       "sub2api_sync_group_model_allowlist_compat",
		argCount:   0,
		returnType: "trigger",
		fragments: []string{
			"language plpgsql",
			"new.model_allowlist",
			"new.models_list_config",
			"is distinct from",
		},
	}},
	triggers: []migrationTriggerContract{{
		table: "groups",
		name:  "sub2api_group_model_allowlist_compat_trg",
		fragments: []string{
			"before insert or update of",
			"model_allowlist",
			"models_list_config",
			"sub2api_sync_group_model_allowlist_compat",
		},
	}},
	dataChecks: []migrationDataContract{{
		description: "groups compatibility columns remain equal",
		query: `
SELECT NOT EXISTS (
    SELECT 1
    FROM public.groups
    WHERE COALESCE(model_allowlist, '{}'::jsonb) IS DISTINCT FROM
          COALESCE(models_list_config, '{}'::jsonb)
)`,
	}},
}

func validateGroupModelAllowlistLegacyCompatContract(ctx context.Context, db migrationQueryConnection) error {
	return validateMigrationSchemaContract(ctx, db, groupModelAllowlistLegacyCompatMigration, groupModelAllowlistLegacyCompatSchemaContract)
}
