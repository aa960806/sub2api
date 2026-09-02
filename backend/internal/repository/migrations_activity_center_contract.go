package repository

import (
	"context"
)

const subnexusActivityCenterMigration = "9001_subnexus_activity_center.sql"

// subnexusActivityCenterSchemaContract mirrors the legacy 153 table contract.
// The runtime only uses the listed columns and indexes; extra columns added by
// a future/older binary remain allowed so the check is forward-compatible.
var subnexusActivityCenterSchemaContract = migrationAliasContract{
	tables: []string{"activity_center_items"},
	columns: []migrationColumnContract{
		{table: "activity_center_items", column: "id", dataType: "bigint", notNull: true, defaultContains: "nextval"},
		{table: "activity_center_items", column: "slug", dataType: "character varying(80)", notNull: true},
		{table: "activity_center_items", column: "title", dataType: "character varying(120)", notNull: true},
		{table: "activity_center_items", column: "subtitle", dataType: "character varying(240)", notNull: true, defaultContains: "''"},
		{table: "activity_center_items", column: "description", dataType: "text", notNull: true, defaultContains: "''"},
		{table: "activity_center_items", column: "icon", dataType: "character varying(64)", notNull: true, defaultContains: "gift"},
		{table: "activity_center_items", column: "cover_url", dataType: "text", notNull: true, defaultContains: "''"},
		{table: "activity_center_items", column: "route_path", dataType: "character varying(255)", notNull: true, defaultContains: "''"},
		{table: "activity_center_items", column: "external_url", dataType: "text", notNull: true, defaultContains: "''"},
		{table: "activity_center_items", column: "action_label", dataType: "character varying(40)", notNull: true, defaultContains: "''"},
		{table: "activity_center_items", column: "activity_type", dataType: "character varying(32)", notNull: true, defaultContains: "custom"},
		{table: "activity_center_items", column: "enabled", dataType: "boolean", notNull: true, defaultContains: "true"},
		{table: "activity_center_items", column: "sort_order", dataType: "integer", notNull: true, defaultContains: "0"},
		{table: "activity_center_items", column: "start_at", dataType: "timestamp with time zone", notNull: false},
		{table: "activity_center_items", column: "end_at", dataType: "timestamp with time zone", notNull: false},
		{table: "activity_center_items", column: "metadata", dataType: "jsonb", notNull: true, defaultContains: "{}"},
		{table: "activity_center_items", column: "created_by", dataType: "bigint", notNull: false},
		{table: "activity_center_items", column: "created_at", dataType: "timestamp with time zone", notNull: true, defaultContains: "now()"},
		{table: "activity_center_items", column: "updated_at", dataType: "timestamp with time zone", notNull: true, defaultContains: "now()"},
	},
	indexes: []migrationIndexContract{
		{table: "activity_center_items", name: "idx_activity_center_items_visible", fragments: []string{"activity_center_items", "enabled", "sort_order", "created_at DESC"}},
		{table: "activity_center_items", name: "idx_activity_center_items_window", fragments: []string{"activity_center_items", "start_at", "end_at"}},
	},
	constraints: []migrationConstraintContract{
		{table: "activity_center_items", name: "activity_center_items_pkey", fragments: []string{"primary key", "id"}, requireValid: true},
		{table: "activity_center_items", name: "activity_center_items_slug_key", fragments: []string{"unique", "slug"}, requireValid: true},
	},
}

func validateSubNexusActivityCenterContract(ctx context.Context, db migrationQueryConnection) error {
	return validateMigrationSchemaContract(ctx, db, subnexusActivityCenterMigration, subnexusActivityCenterSchemaContract)
}
