package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// A legacy migration record is only a claim that SQL was executed.  The
// contracts below verify the durable effects before we adopt that record under
// the target filename.  Keep these checks deliberately declarative and small:
// they are run only when a legacy alias is present, never on the normal path.
type migrationAliasContract struct {
	tables      []string
	columns     []migrationColumnContract
	indexes     []migrationIndexContract
	constraints []migrationConstraintContract
	functions   []migrationFunctionContract
	triggers    []migrationTriggerContract
	settings    []migrationSettingContract
	dataChecks  []migrationDataContract
}

type migrationColumnContract struct {
	table    string
	column   string
	dataType string // PostgreSQL format_type output, e.g. numeric(20,8).
	notNull  bool
}

type migrationIndexContract struct {
	table     string
	name      string
	fragments []string
}

type migrationConstraintContract struct {
	table        string
	name         string
	fragments    []string
	requireValid bool
}

type migrationFunctionContract struct {
	name       string
	argCount   int
	returnType string
	fragments  []string
}

type migrationTriggerContract struct {
	table     string
	name      string
	fragments []string
}

type migrationSettingContract struct {
	key          string
	allowedValue []string
}

type migrationDataContract struct {
	description string
	query       string
	args        []any
}

// migrationAliasContracts is an explicit one-to-one companion to
// legacyMigrationAliases.  Every alias must have a contract; adding an alias
// without one is intentionally rejected by validateLegacyMigrationAliasMap.
var migrationAliasContracts = map[string]migrationAliasContract{
	"158_add_group_peak_rate_multiplier.sql": {
		tables: []string{"groups"},
		columns: []migrationColumnContract{
			{table: "groups", column: "peak_rate_enabled", dataType: "boolean", notNull: true},
			{table: "groups", column: "peak_start", dataType: "character varying(5)", notNull: true},
			{table: "groups", column: "peak_end", dataType: "character varying(5)", notNull: true},
			{table: "groups", column: "peak_rate_multiplier", dataType: "numeric(10,4)", notNull: true},
		},
	},
	"158_enable_grok_media_generation_groups.sql": {
		// allow_image_generation is operator-managed after the one-time backfill;
		// current false values must be preserved during adoption.
		tables: []string{"groups"},
		columns: []migrationColumnContract{
			{table: "groups", column: "platform", dataType: "character varying(50)", notNull: true},
			{table: "groups", column: "allow_image_generation", dataType: "boolean", notNull: true},
		},
	},
	"174_add_usage_log_long_context_billing.sql": {
		tables: []string{"usage_logs"},
		columns: []migrationColumnContract{
			{table: "usage_logs", column: "long_context_billing_applied", dataType: "boolean", notNull: true},
		},
	},
	"174_add_usage_logs_api_key_latest_ip_index_notx.sql": {
		tables: []string{"usage_logs"},
		indexes: []migrationIndexContract{{
			table:     "usage_logs",
			name:      "idx_usage_logs_api_key_latest_ip",
			fragments: []string{"usage_logs", "api_key_id", "created_at DESC", "id DESC", "ip_address", "ip_address IS NOT NULL", "<>"},
		}},
	},
	"175_add_ops_system_logs_host.sql": {
		tables: []string{"ops_system_logs"},
		columns: []migrationColumnContract{
			{table: "ops_system_logs", column: "host", dataType: "character varying(255)"},
		},
	},
	"175a_add_ops_system_logs_host_index_notx.sql": {
		tables: []string{"ops_system_logs"},
		indexes: []migrationIndexContract{{
			table:     "ops_system_logs",
			name:      "idx_ops_system_logs_host_created_at",
			fragments: []string{"ops_system_logs", "host", "created_at DESC"},
		}},
	},
	"180_audit_logs.sql": {
		tables: []string{"audit_logs"},
		columns: []migrationColumnContract{
			{table: "audit_logs", column: "id", dataType: "bigint", notNull: true},
			{table: "audit_logs", column: "created_at", dataType: "timestamp with time zone", notNull: true},
			{table: "audit_logs", column: "actor_user_id", dataType: "bigint"},
			{table: "audit_logs", column: "actor_email", dataType: "character varying(255)", notNull: true},
			{table: "audit_logs", column: "actor_role", dataType: "character varying(32)", notNull: true},
			{table: "audit_logs", column: "auth_method", dataType: "character varying(32)", notNull: true},
			{table: "audit_logs", column: "credential_masked", dataType: "character varying(160)", notNull: true},
			{table: "audit_logs", column: "action", dataType: "character varying(128)", notNull: true},
			{table: "audit_logs", column: "method", dataType: "character varying(16)", notNull: true},
			{table: "audit_logs", column: "path", dataType: "character varying(512)", notNull: true},
			{table: "audit_logs", column: "request_id", dataType: "character varying(64)", notNull: true},
			{table: "audit_logs", column: "client_ip", dataType: "character varying(64)", notNull: true},
			{table: "audit_logs", column: "user_agent", dataType: "character varying(512)", notNull: true},
			{table: "audit_logs", column: "request_body", dataType: "text", notNull: true},
			{table: "audit_logs", column: "status_code", dataType: "integer", notNull: true},
			{table: "audit_logs", column: "latency_ms", dataType: "bigint", notNull: true},
			{table: "audit_logs", column: "extra", dataType: "jsonb", notNull: true},
		},
		constraints: []migrationConstraintContract{{
			table: "audit_logs", name: "audit_logs_pkey", fragments: []string{"primary key", "id"}, requireValid: true,
		}},
		indexes: []migrationIndexContract{
			{table: "audit_logs", name: "idx_audit_logs_created_at_id", fragments: []string{"audit_logs", "created_at DESC", "id DESC"}},
			{table: "audit_logs", name: "idx_audit_logs_actor_created", fragments: []string{"audit_logs", "actor_user_id", "created_at DESC"}},
			{table: "audit_logs", name: "idx_audit_logs_action", fragments: []string{"audit_logs", "action"}},
			{table: "audit_logs", name: "idx_audit_logs_client_ip", fragments: []string{"audit_logs", "client_ip"}},
		},
	},
	"183_ops_ingress_reject_aggregates.sql": {
		tables: []string{"ops_ingress_reject_aggregates"},
		columns: []migrationColumnContract{
			{table: "ops_ingress_reject_aggregates", column: "id", dataType: "bigint", notNull: true},
			{table: "ops_ingress_reject_aggregates", column: "bucket_start", dataType: "timestamp with time zone", notNull: true},
			{table: "ops_ingress_reject_aggregates", column: "reject_reason", dataType: "character varying(64)", notNull: true},
			{table: "ops_ingress_reject_aggregates", column: "route_family", dataType: "character varying(64)", notNull: true},
			{table: "ops_ingress_reject_aggregates", column: "protocol", dataType: "character varying(32)", notNull: true},
			{table: "ops_ingress_reject_aggregates", column: "client_ip", dataType: "inet", notNull: true},
			{table: "ops_ingress_reject_aggregates", column: "user_id", dataType: "bigint", notNull: true},
			{table: "ops_ingress_reject_aggregates", column: "api_key_id", dataType: "bigint", notNull: true},
			{table: "ops_ingress_reject_aggregates", column: "request_count", dataType: "bigint", notNull: true},
			{table: "ops_ingress_reject_aggregates", column: "first_seen", dataType: "timestamp with time zone", notNull: true},
			{table: "ops_ingress_reject_aggregates", column: "last_seen", dataType: "timestamp with time zone", notNull: true},
			{table: "ops_ingress_reject_aggregates", column: "created_at", dataType: "timestamp with time zone", notNull: true},
			{table: "ops_ingress_reject_aggregates", column: "updated_at", dataType: "timestamp with time zone", notNull: true},
		},
		constraints: []migrationConstraintContract{{
			table: "ops_ingress_reject_aggregates", name: "ops_ingress_reject_aggregates_pkey", fragments: []string{"primary key", "id"}, requireValid: true,
		}, {
			table:        "ops_ingress_reject_aggregates",
			name:         "ops_ingress_reject_aggregates_dimensions_unique",
			fragments:    []string{"bucket_start", "reject_reason", "route_family", "protocol", "client_ip", "user_id", "api_key_id"},
			requireValid: true,
		}},
		indexes: []migrationIndexContract{
			{table: "ops_ingress_reject_aggregates", name: "idx_ops_ingress_reject_aggregates_bucket", fragments: []string{"ops_ingress_reject_aggregates", "bucket_start DESC"}},
			{table: "ops_ingress_reject_aggregates", name: "idx_ops_ingress_reject_aggregates_reason_bucket", fragments: []string{"ops_ingress_reject_aggregates", "reject_reason", "bucket_start DESC"}},
			{table: "ops_ingress_reject_aggregates", name: "idx_ops_ingress_reject_aggregates_ip_bucket", fragments: []string{"ops_ingress_reject_aggregates", "client_ip", "bucket_start DESC"}},
		},
	},
	"184_auth_cache_invalidation_outbox.sql": {
		tables: []string{"auth_cache_invalidation_outbox"},
		columns: []migrationColumnContract{
			{table: "auth_cache_invalidation_outbox", column: "id", dataType: "bigint", notNull: true},
			{table: "auth_cache_invalidation_outbox", column: "cache_key", dataType: "character(64)", notNull: true},
			{table: "auth_cache_invalidation_outbox", column: "created_at", dataType: "timestamp with time zone", notNull: true},
			{table: "auth_cache_invalidation_outbox", column: "available_at", dataType: "timestamp with time zone", notNull: true},
			{table: "auth_cache_invalidation_outbox", column: "delivery_stage", dataType: "smallint", notNull: true},
			{table: "auth_cache_invalidation_outbox", column: "attempts", dataType: "integer", notNull: true},
			{table: "auth_cache_invalidation_outbox", column: "last_error", dataType: "text"},
			{table: "auth_cache_invalidation_outbox", column: "claimed_at", dataType: "timestamp with time zone"},
			{table: "auth_cache_invalidation_outbox", column: "claimed_by", dataType: "text"},
		},
		constraints: []migrationConstraintContract{
			{table: "auth_cache_invalidation_outbox", name: "auth_cache_invalidation_outbox_pkey", fragments: []string{"primary key", "id"}, requireValid: true},
			{table: "auth_cache_invalidation_outbox", name: "auth_cache_invalidation_outbox_cache_key_check", fragments: []string{"cache_key", "64"}, requireValid: true},
			{table: "auth_cache_invalidation_outbox", name: "auth_cache_invalidation_outbox_delivery_stage_check", fragments: []string{"delivery_stage", "0", "1"}, requireValid: true},
			{table: "auth_cache_invalidation_outbox", name: "auth_cache_invalidation_outbox_attempts_check", fragments: []string{"attempts", ">= 0"}, requireValid: true},
		},
		indexes: []migrationIndexContract{
			{table: "auth_cache_invalidation_outbox", name: "idx_auth_cache_invalidation_outbox_available", fragments: []string{"auth_cache_invalidation_outbox", "available_at", "id", "claimed_at IS NULL"}},
			{table: "auth_cache_invalidation_outbox", name: "idx_auth_cache_invalidation_outbox_lease", fragments: []string{"auth_cache_invalidation_outbox", "claimed_at", "claimed_at IS NOT NULL"}},
			{table: "auth_cache_invalidation_outbox", name: "idx_auth_cache_invalidation_outbox_cache_key", fragments: []string{"auth_cache_invalidation_outbox", "cache_key"}},
			{table: "auth_cache_invalidation_outbox", name: "idx_auth_cache_invalidation_outbox_created_at", fragments: []string{"auth_cache_invalidation_outbox", "created_at"}},
		},
		functions: []migrationFunctionContract{
			{name: "enqueue_auth_cache_invalidation", argCount: 1, returnType: "void", fragments: []string{"raw_key text", "sha256", "auth_cache_invalidation_outbox", "language plpgsql"}},
			{name: "enqueue_api_key_auth_cache_invalidation", returnType: "trigger", fragments: []string{"tg_op", "old.key", "new.key", "enqueue_auth_cache_invalidation", "language plpgsql"}},
			{name: "enqueue_user_auth_cache_invalidation", returnType: "trigger", fragments: []string{"target_user_id", "api_keys", "auth_cache_invalidation_outbox", "language plpgsql"}},
			{name: "enqueue_group_auth_cache_invalidation", returnType: "trigger", fragments: []string{"target_group_id", "api_keys", "auth_cache_invalidation_outbox", "language plpgsql"}},
			{name: "enqueue_allowed_group_auth_cache_invalidation", returnType: "trigger", fragments: []string{"target_user_id", "target_group_id", "is_exclusive", "api_keys", "language plpgsql"}},
		},
		triggers: []migrationTriggerContract{
			// pg_get_triggerdef emits trigger events in PostgreSQL's canonical
			// order (DELETE before UPDATE, and INSERT before DELETE before UPDATE),
			// regardless of the order used in CREATE TRIGGER.
			{table: "api_keys", name: "trg_api_keys_auth_cache_invalidation", fragments: []string{"after delete or update", "enqueue_api_key_auth_cache_invalidation"}},
			{table: "users", name: "trg_users_auth_cache_invalidation", fragments: []string{"after delete or update", "enqueue_user_auth_cache_invalidation"}},
			{table: "groups", name: "trg_groups_auth_cache_invalidation", fragments: []string{"after delete or update", "enqueue_group_auth_cache_invalidation"}},
			{table: "user_allowed_groups", name: "trg_user_allowed_groups_auth_cache_invalidation", fragments: []string{"after insert or delete or update", "enqueue_allowed_group_auth_cache_invalidation"}},
		},
	},
	"185_group_reasoning_effort_policy.sql": {
		tables: []string{"groups"},
		columns: []migrationColumnContract{
			{table: "groups", column: "max_reasoning_effort", dataType: "character varying(20)", notNull: true},
			{table: "groups", column: "reasoning_effort_mappings", dataType: "jsonb", notNull: true},
		},
	},
	"186_alipay_mobile_precreate_deep_link.sql": {
		tables:   []string{"settings"},
		settings: []migrationSettingContract{{key: "ALIPAY_MOBILE_PRECREATE_DEEP_LINK", allowedValue: []string{"true", "false"}}},
	},
	"188_allow_live_usage_request_type.sql": {
		tables: []string{"usage_logs"},
		constraints: []migrationConstraintContract{{
			table: "usage_logs", name: "usage_logs_request_type_check",
			fragments: []string{"request_type", ">= 0", "<= 5"}, requireValid: true,
		}},
	},
	"189_add_group_allow_live.sql": {
		tables:  []string{"groups"},
		columns: []migrationColumnContract{{table: "groups", column: "allow_live", dataType: "boolean", notNull: true}},
		dataChecks: []migrationDataContract{{
			description: "groups.allow_live keeps the target false default",
			query: `
SELECT COALESCE((
    SELECT lower(btrim(pg_get_expr(d.adbin, d.adrelid))) IN ('false', 'false::boolean')
    FROM pg_attribute a
    JOIN pg_class c ON c.oid = a.attrelid
    JOIN pg_namespace n ON n.oid = c.relnamespace
    JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
    WHERE n.nspname = 'public'
      AND c.relname = 'groups'
      AND a.attname = 'allow_live'
      AND NOT a.attisdropped
), FALSE)`,
		}},
	},
	"191_passkey_credentials.sql": {
		tables: []string{"passkey_user_handles", "passkey_credentials"},
		columns: []migrationColumnContract{
			{table: "passkey_user_handles", column: "user_id", dataType: "bigint", notNull: true},
			{table: "passkey_user_handles", column: "user_handle", dataType: "bytea", notNull: true},
			{table: "passkey_user_handles", column: "created_at", dataType: "timestamp with time zone", notNull: true},
			{table: "passkey_credentials", column: "id", dataType: "bigint", notNull: true},
			{table: "passkey_credentials", column: "user_id", dataType: "bigint", notNull: true},
			{table: "passkey_credentials", column: "credential_id", dataType: "bytea", notNull: true},
			{table: "passkey_credentials", column: "name", dataType: "character varying(100)", notNull: true},
			{table: "passkey_credentials", column: "credential_data", dataType: "jsonb", notNull: true},
			{table: "passkey_credentials", column: "last_used_at", dataType: "timestamp with time zone"},
			{table: "passkey_credentials", column: "created_at", dataType: "timestamp with time zone", notNull: true},
			{table: "passkey_credentials", column: "updated_at", dataType: "timestamp with time zone", notNull: true},
		},
		indexes: []migrationIndexContract{
			{table: "passkey_credentials", name: "passkey_credentials_user_id_idx", fragments: []string{"passkey_credentials", "user_id"}},
			{table: "passkey_credentials", name: "passkey_credentials_last_used_at_idx", fragments: []string{"passkey_credentials", "last_used_at"}},
		},
		constraints: []migrationConstraintContract{
			{table: "passkey_user_handles", name: "passkey_user_handles_pkey", fragments: []string{"primary key", "user_id"}, requireValid: true},
			{table: "passkey_user_handles", name: "passkey_user_handles_user_handle_key", fragments: []string{"unique", "user_handle"}, requireValid: true},
			{table: "passkey_credentials", name: "passkey_credentials_pkey", fragments: []string{"primary key", "id"}, requireValid: true},
			{table: "passkey_credentials", name: "passkey_credentials_credential_id_key", fragments: []string{"unique", "credential_id"}, requireValid: true},
		},
		dataChecks: []migrationDataContract{
			{
				description: "passkey_user_handles.user_id references users(id) with cascade",
				query: `
SELECT EXISTS (
    SELECT 1
    FROM pg_constraint c
    JOIN pg_class child ON child.oid = c.conrelid
    JOIN pg_namespace child_ns ON child_ns.oid = child.relnamespace
    JOIN pg_class parent ON parent.oid = c.confrelid
    JOIN pg_namespace parent_ns ON parent_ns.oid = parent.relnamespace
    JOIN pg_attribute child_col ON child_col.attrelid = child.oid
    JOIN pg_attribute parent_col ON parent_col.attrelid = parent.oid
    WHERE child_ns.nspname = 'public'
      AND parent_ns.nspname = 'public'
      AND child.relname = 'passkey_user_handles'
      AND parent.relname = 'users'
      AND c.contype = 'f'
      AND c.confdeltype = 'c'
      AND c.convalidated
      AND array_length(c.conkey, 1) = 1
      AND array_length(c.confkey, 1) = 1
      AND child_col.attname = 'user_id'
      AND child_col.attnum = ANY (c.conkey)
      AND parent_col.attname = 'id'
      AND parent_col.attnum = ANY (c.confkey)
)`,
			},
			{
				description: "passkey_credentials.user_id references users(id) with cascade",
				query: `
SELECT EXISTS (
    SELECT 1
    FROM pg_constraint c
    JOIN pg_class child ON child.oid = c.conrelid
    JOIN pg_namespace child_ns ON child_ns.oid = child.relnamespace
    JOIN pg_class parent ON parent.oid = c.confrelid
    JOIN pg_namespace parent_ns ON parent_ns.oid = parent.relnamespace
    JOIN pg_attribute child_col ON child_col.attrelid = child.oid
    JOIN pg_attribute parent_col ON parent_col.attrelid = parent.oid
    WHERE child_ns.nspname = 'public'
      AND parent_ns.nspname = 'public'
      AND child.relname = 'passkey_credentials'
      AND parent.relname = 'users'
      AND c.contype = 'f'
      AND c.confdeltype = 'c'
      AND c.convalidated
      AND array_length(c.conkey, 1) = 1
      AND array_length(c.confkey, 1) = 1
      AND child_col.attname = 'user_id'
      AND child_col.attnum = ANY (c.conkey)
      AND parent_col.attname = 'id'
      AND parent_col.attnum = ANY (c.confkey)
)`,
			},
		},
	},
	"195_add_usage_log_upstream_model_mismatch_index_notx.sql": {
		tables: []string{"usage_logs"},
		indexes: []migrationIndexContract{{
			table:     "usage_logs",
			name:      "idx_usage_logs_upstream_model_mismatch_created_at",
			fragments: []string{"usage_logs", "created_at DESC", "id DESC", "upstream_model_mismatch IS TRUE"},
		}},
	},
	"217_group_video_model_prices.sql": {
		tables:  []string{"groups"},
		columns: []migrationColumnContract{{table: "groups", column: "video_model_prices", dataType: "jsonb"}},
	},
	"218_group_audio_voice_pricing.sql": {
		tables: []string{"groups"},
		columns: []migrationColumnContract{
			{table: "groups", column: "audio_realtime_price_per_min", dataType: "numeric(20,8)"},
			{table: "groups", column: "audio_tts_price_per_million_chars", dataType: "numeric(20,8)"},
			{table: "groups", column: "audio_stt_price_per_hour", dataType: "numeric(20,8)"},
		},
	},
	"219_group_search_price_per_1k.sql": {
		tables:  []string{"groups"},
		columns: []migrationColumnContract{{table: "groups", column: "search_price_per_1k", dataType: "numeric(20,8)"}},
	},
	"221_group_model_pricing.sql": {
		// long_context_pricing_enabled is operator-managed after the one-time
		// backfill, so its current value is not a durable migration invariant.
		tables: []string{"groups"},
		columns: []migrationColumnContract{
			{table: "groups", column: "long_context_pricing_enabled", dataType: "boolean", notNull: true},
			{table: "groups", column: "model_pricing", dataType: "jsonb"},
		},
	},
	"222_group_usage_daily_rollups.sql": {
		tables: []string{"usage_group_daily_rollups", "usage_group_rollup_state"},
		columns: []migrationColumnContract{
			{table: "usage_group_daily_rollups", column: "bucket_date", dataType: "date", notNull: true},
			{table: "usage_group_daily_rollups", column: "group_id", dataType: "bigint", notNull: true},
			{table: "usage_group_daily_rollups", column: "actual_cost", dataType: "numeric(20,10)", notNull: true},
			{table: "usage_group_daily_rollups", column: "computed_at", dataType: "timestamp with time zone", notNull: true},
			{table: "usage_group_rollup_state", column: "id", dataType: "smallint", notNull: true},
			{table: "usage_group_rollup_state", column: "closed_before", dataType: "date", notNull: true},
			{table: "usage_group_rollup_state", column: "retained_from", dataType: "timestamp with time zone", notNull: true},
			{table: "usage_group_rollup_state", column: "updated_at", dataType: "timestamp with time zone", notNull: true},
		},
		constraints: []migrationConstraintContract{{
			table: "usage_group_daily_rollups", name: "usage_group_daily_rollups_pkey", fragments: []string{"primary key", "bucket_date", "group_id"}, requireValid: true,
		}, {
			table: "usage_group_rollup_state", name: "usage_group_rollup_state_id_check", fragments: []string{"id", "= 1"}, requireValid: true,
		}, {
			table: "usage_group_rollup_state", name: "usage_group_rollup_state_pkey", fragments: []string{"primary key", "id"}, requireValid: true,
		}},
		dataChecks: []migrationDataContract{{
			description: "usage_group_rollup_state contains singleton row id=1",
			query:       `SELECT (SELECT COUNT(*) FROM public.usage_group_rollup_state WHERE id = 1) = 1`,
		}},
		functions: []migrationFunctionContract{
			{name: "invalidate_group_usage_rollup_state", returnType: "trigger", fragments: []string{"affected_date", "closed_before", "for update", "language plpgsql"}},
			{name: "invalidate_group_usage_rollup_state_after_insert", returnType: "trigger", fragments: []string{"inserted_usage_logs", "for key share", "language plpgsql"}},
		},
		triggers: []migrationTriggerContract{
			{table: "usage_logs", name: "usage_logs_group_rollup_invalidate_insert", fragments: []string{"after insert", "new table", "invalidate_group_usage_rollup_state_after_insert"}},
			{table: "usage_logs", name: "usage_logs_group_rollup_invalidate_delete", fragments: []string{"after delete", "for each row", "invalidate_group_usage_rollup_state"}},
			{table: "usage_logs", name: "usage_logs_group_rollup_invalidate_update", fragments: []string{"after update of", "created_at", "group_id", "actual_cost", "invalidate_group_usage_rollup_state"}},
		},
	},
	"223_group_usage_rollup_timezone.sql": {
		tables:  []string{"usage_group_rollup_state", "usage_group_daily_rollups"},
		columns: []migrationColumnContract{{table: "usage_group_rollup_state", column: "timezone_name", dataType: "text", notNull: true}},
		dataChecks: []migrationDataContract{{
			description: "usage_group_rollup_state timezone_name is non-empty",
			query:       `SELECT COALESCE((SELECT btrim(timezone_name) <> '' FROM public.usage_group_rollup_state WHERE id = 1), FALSE)`,
		}},
		functions: []migrationFunctionContract{
			{name: "invalidate_group_usage_rollup_state", returnType: "trigger", fragments: []string{"configured_timezone", "current_setting", "language plpgsql"}},
			{name: "invalidate_group_usage_rollup_state_after_insert", returnType: "trigger", fragments: []string{"configured_timezone", "current_setting", "language plpgsql"}},
		},
		triggers: []migrationTriggerContract{
			{table: "usage_logs", name: "usage_logs_group_rollup_invalidate_insert", fragments: []string{"after insert", "new table", "invalidate_group_usage_rollup_state_after_insert"}},
			{table: "usage_logs", name: "usage_logs_group_rollup_invalidate_delete", fragments: []string{"after delete", "for each row", "invalidate_group_usage_rollup_state"}},
			{table: "usage_logs", name: "usage_logs_group_rollup_invalidate_update", fragments: []string{"after update of", "created_at", "group_id", "actual_cost", "invalidate_group_usage_rollup_state"}},
		},
	},
	"225_backfill_codex_fingerprint_seed.sql": {
		tables: []string{"accounts"},
		dataChecks: []migrationDataContract{{
			description: "all eligible OpenAI OAuth accounts have canonical Codex fingerprint seeds",
			query: `
SELECT NOT EXISTS (
    SELECT 1
    FROM public.accounts
    WHERE deleted_at IS NULL
      AND platform = 'openai'
      AND type = 'oauth'
      AND COALESCE(extra->>'codex_fingerprint_mode', '') IN ('device', 'session', 'full')
      AND (
          extra->>'codex_fingerprint_seed' IS NULL
          OR btrim(extra->>'codex_fingerprint_seed') = ''
          OR NOT (
              extra->>'codex_fingerprint_seed' ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
              AND extra->>'codex_fingerprint_seed' <> '00000000-0000-0000-0000-000000000000'
          )
      )
)`,
		}},
	},
	"225_channel_model_time_pricing.sql": {
		tables:  []string{"channel_model_pricing"},
		columns: []migrationColumnContract{{table: "channel_model_pricing", column: "time_pricing", dataType: "jsonb"}},
	},
	"226_add_usage_log_effective_model_indexes_notx.sql": {
		tables: []string{"usage_logs"},
		indexes: []migrationIndexContract{
			{table: "usage_logs", name: "idx_usage_logs_effective_requested_model_created", fragments: []string{"usage_logs", "coalesce", "requested_model", "btrim", "model", "created_at DESC", "id DESC"}},
			{table: "usage_logs", name: "idx_usage_logs_effective_upstream_model_created", fragments: []string{"usage_logs", "coalesce", "upstream_model", "btrim", "model", "created_at DESC", "id DESC"}},
		},
	},
	"226_channel_monitor_quota_mode.sql": {
		tables: []string{"channel_monitors", "channel_monitor_request_templates", "channel_monitor_histories", "accounts", "settings"},
		columns: []migrationColumnContract{
			{table: "channel_monitors", column: "check_mode", dataType: "character varying(32)", notNull: true},
			{table: "channel_monitors", column: "account_id", dataType: "bigint"},
			{table: "channel_monitor_histories", column: "quota", dataType: "jsonb"},
		},
		indexes: []migrationIndexContract{{
			table: "channel_monitors", name: "idx_channel_monitors_account_id", fragments: []string{"channel_monitors", "account_id"},
		}},
		constraints: []migrationConstraintContract{
			{
				table: "channel_monitors", name: "channel_monitors_provider_check",
				fragments: []string{"provider", "openai", "anthropic", "gemini", "grok", "antigravity", "kimi", "zhipu", "deepseek"}, requireValid: true,
			},
			{
				table: "channel_monitor_request_templates", name: "channel_monitor_request_templates_provider_check",
				fragments: []string{"provider", "openai", "anthropic", "gemini", "grok", "antigravity", "kimi", "zhipu", "deepseek"}, requireValid: true,
			},
			{
				table: "channel_monitors", name: "channel_monitors_check_mode_check",
				fragments: []string{"check_mode", "probe", "quota", "quota_probe"}, requireValid: true,
			},
		},
		settings: []migrationSettingContract{{key: "channel_monitor_show_quota"}},
		dataChecks: []migrationDataContract{
			{
				description: "channel_monitors.check_mode keeps the target probe default",
				query: `
SELECT COALESCE((
    SELECT regexp_replace(lower(btrim(pg_get_expr(d.adbin, d.adrelid))), '::.*$', '') = '''probe'''
    FROM pg_attribute a
    JOIN pg_class c ON c.oid = a.attrelid
    JOIN pg_namespace n ON n.oid = c.relnamespace
    JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
    WHERE n.nspname = 'public'
      AND c.relname = 'channel_monitors'
      AND a.attname = 'check_mode'
      AND NOT a.attisdropped
), FALSE)`,
			},
			{
				description: "channel_monitors.account_id references accounts(id) with set null",
				query: `
SELECT EXISTS (
    SELECT 1
    FROM pg_constraint c
    JOIN pg_class child ON child.oid = c.conrelid
    JOIN pg_namespace child_ns ON child_ns.oid = child.relnamespace
    JOIN pg_class parent ON parent.oid = c.confrelid
    JOIN pg_namespace parent_ns ON parent_ns.oid = parent.relnamespace
    JOIN pg_attribute child_col ON child_col.attrelid = child.oid
    JOIN pg_attribute parent_col ON parent_col.attrelid = parent.oid
    WHERE child_ns.nspname = 'public'
      AND parent_ns.nspname = 'public'
      AND child.relname = 'channel_monitors'
      AND parent.relname = 'accounts'
      AND c.contype = 'f'
      AND c.confdeltype = 'n'
      AND c.convalidated
      AND array_length(c.conkey, 1) = 1
      AND array_length(c.confkey, 1) = 1
      AND child_col.attname = 'account_id'
      AND child_col.attnum = ANY (c.conkey)
      AND parent_col.attname = 'id'
      AND parent_col.attnum = ANY (c.confkey)
)`,
			},
		},
	},
}

var migrationChecksumPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// validateLegacyMigrationAliasMap is kept callable by unit tests and is also
// used at runtime for a fail-closed check before any alias is adopted.
func validateLegacyMigrationAliasMap() error {
	seenLegacy := make(map[string]string, len(legacyMigrationAliases))
	for target, alias := range legacyMigrationAliases {
		if !migrationChecksumPattern.MatchString(alias.checksum) {
			return fmt.Errorf("legacy migration alias %s has invalid checksum %q", target, alias.checksum)
		}
		if !migrationChecksumPattern.MatchString(alias.expectedLegacyChecksum()) {
			return fmt.Errorf("legacy migration alias %s has invalid legacy checksum %q", target, alias.expectedLegacyChecksum())
		}
		if strings.TrimSpace(alias.legacyFilename) == "" {
			return fmt.Errorf("legacy migration alias %s has empty legacy filename", target)
		}
		if target == alias.legacyFilename {
			return fmt.Errorf("legacy migration alias %s maps to itself", target)
		}
		if previous, exists := seenLegacy[alias.legacyFilename]; exists {
			return fmt.Errorf("legacy migration alias %s is also mapped from %s", alias.legacyFilename, previous)
		}
		seenLegacy[alias.legacyFilename] = target
		if _, ok := migrationAliasContracts[target]; !ok {
			return fmt.Errorf("legacy migration alias %s has no schema contract", target)
		}
	}
	for target := range migrationAliasContracts {
		if _, ok := legacyMigrationAliases[target]; !ok {
			return fmt.Errorf("schema contract %s has no legacy migration alias", target)
		}
		contract := migrationAliasContracts[target]
		for _, index := range contract.indexes {
			if strings.TrimSpace(index.table) == "" {
				return fmt.Errorf("schema contract %s index %s has empty table", target, index.name)
			}
		}
		for _, function := range contract.functions {
			if function.argCount < 0 {
				return fmt.Errorf("schema contract %s function %s has negative argument count", target, function.name)
			}
			if strings.TrimSpace(function.returnType) == "" {
				return fmt.Errorf("schema contract %s function %s has empty return type", target, function.name)
			}
		}
	}
	return nil
}

func validateMigrationAliasContract(ctx context.Context, db migrationQueryConnection, migrationName string) error {
	contract, ok := migrationAliasContracts[migrationName]
	if !ok {
		return fmt.Errorf("no schema contract registered for legacy migration alias %s", migrationName)
	}

	for _, table := range contract.tables {
		exists, err := migrationTableExists(ctx, db, table)
		if err != nil {
			return fmt.Errorf("schema contract %s table %s query: %w", migrationName, table, err)
		}
		if !exists {
			return fmt.Errorf("schema contract %s table %s is missing", migrationName, table)
		}
	}
	for _, column := range contract.columns {
		actualType, actualNotNull, err := lookupMigrationColumn(ctx, db, column)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("schema contract %s column %s.%s is missing", migrationName, column.table, column.column)
			}
			return fmt.Errorf("schema contract %s column %s.%s query: %w", migrationName, column.table, column.column, err)
		}
		if actualType != column.dataType || actualNotNull != column.notNull {
			return fmt.Errorf(
				"schema contract %s column %s.%s mismatch (type=%s not_null=%t expected type=%s not_null=%t)",
				migrationName, column.table, column.column, actualType, actualNotNull, column.dataType, column.notNull,
			)
		}
	}
	for _, index := range contract.indexes {
		valid, ready, definition, err := lookupMigrationIndex(ctx, db, index.name, index.table)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("schema contract %s index %s is missing", migrationName, index.name)
			}
			return fmt.Errorf("schema contract %s index %s query: %w", migrationName, index.name, err)
		}
		if !valid || !ready {
			return fmt.Errorf("schema contract %s index %s is invalid or not ready; repair it before adopting the legacy record", migrationName, index.name)
		}
		for _, fragment := range index.fragments {
			if !migrationContractTextContains(definition, fragment) {
				return fmt.Errorf("schema contract %s index %s definition missing %q", migrationName, index.name, fragment)
			}
		}
	}
	for _, constraint := range contract.constraints {
		definition, validated, err := lookupMigrationConstraint(ctx, db, constraint.table, constraint.name)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("schema contract %s constraint %s.%s is missing", migrationName, constraint.table, constraint.name)
			}
			return fmt.Errorf("schema contract %s constraint %s.%s query: %w", migrationName, constraint.table, constraint.name, err)
		}
		if constraint.requireValid && !validated {
			return fmt.Errorf("schema contract %s constraint %s.%s is not validated", migrationName, constraint.table, constraint.name)
		}
		for _, fragment := range constraint.fragments {
			if !migrationContractTextContains(definition, fragment) {
				return fmt.Errorf("schema contract %s constraint %s.%s definition missing %q", migrationName, constraint.table, constraint.name, fragment)
			}
		}
	}
	for _, function := range contract.functions {
		definition, returnType, err := lookupMigrationFunction(ctx, db, function.name, function.argCount)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("schema contract %s function %s is missing", migrationName, function.name)
			}
			return fmt.Errorf("schema contract %s function %s query: %w", migrationName, function.name, err)
		}
		if function.returnType != "" && !strings.EqualFold(strings.TrimSpace(returnType), function.returnType) {
			return fmt.Errorf("schema contract %s function %s return type mismatch (db=%s expected=%s)", migrationName, function.name, returnType, function.returnType)
		}
		for _, fragment := range function.fragments {
			if !migrationContractTextContains(definition, fragment) {
				return fmt.Errorf("schema contract %s function %s definition missing %q", migrationName, function.name, fragment)
			}
		}
	}
	for _, trigger := range contract.triggers {
		definition, err := lookupMigrationTrigger(ctx, db, trigger.table, trigger.name)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("schema contract %s trigger %s.%s is missing", migrationName, trigger.table, trigger.name)
			}
			return fmt.Errorf("schema contract %s trigger %s.%s query: %w", migrationName, trigger.table, trigger.name, err)
		}
		for _, fragment := range trigger.fragments {
			if !migrationContractTextContains(definition, fragment) {
				return fmt.Errorf("schema contract %s trigger %s.%s definition missing %q", migrationName, trigger.table, trigger.name, fragment)
			}
		}
	}
	for _, setting := range contract.settings {
		value, err := lookupMigrationSetting(ctx, db, setting.key)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("schema contract %s setting %s is missing", migrationName, setting.key)
			}
			return fmt.Errorf("schema contract %s setting %s query: %w", migrationName, setting.key, err)
		}
		if len(setting.allowedValue) > 0 {
			matched := false
			for _, allowed := range setting.allowedValue {
				if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(allowed)) {
					matched = true
					break
				}
			}
			if !matched {
				return fmt.Errorf("schema contract %s setting %s has unsupported value %q", migrationName, setting.key, value)
			}
		}
	}
	for _, check := range contract.dataChecks {
		var ok bool
		if err := db.QueryRowContext(ctx, check.query, check.args...).Scan(&ok); err != nil {
			return fmt.Errorf("schema contract %s data check %s: %w", migrationName, check.description, err)
		}
		if !ok {
			return fmt.Errorf("schema contract %s data check failed: %s", migrationName, check.description)
		}
	}
	return nil
}

func lookupMigrationColumn(ctx context.Context, db migrationQueryConnection, contract migrationColumnContract) (string, bool, error) {
	var dataType string
	var notNull bool
	err := db.QueryRowContext(ctx, `
SELECT format_type(a.atttypid, a.atttypmod), a.attnotnull
FROM pg_attribute a
JOIN pg_class c ON c.oid = a.attrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = 'public'
  AND c.relname = $1
  AND a.attname = $2
  AND NOT a.attisdropped
`, contract.table, contract.column).Scan(&dataType, &notNull)
	return dataType, notNull, err
}

func migrationTableExists(ctx context.Context, db migrationQueryConnection, tableName string) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ctx, `
SELECT EXISTS (
    SELECT 1
    FROM pg_class c
    JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE n.nspname = 'public'
      AND c.relname = $1
      AND c.relkind IN ('r', 'p', 'f')
)
`, tableName).Scan(&exists)
	return exists, err
}

func lookupMigrationIndex(ctx context.Context, db migrationQueryConnection, name, table string) (bool, bool, string, error) {
	var valid bool
	var ready bool
	var definition string
	err := db.QueryRowContext(ctx, `
SELECT i.indisvalid, i.indisready, pg_get_indexdef(i.indexrelid)
FROM pg_class idx
JOIN pg_namespace ns ON ns.oid = idx.relnamespace
JOIN pg_index i ON i.indexrelid = idx.oid
JOIN pg_class tbl ON tbl.oid = i.indrelid
JOIN pg_namespace tbl_ns ON tbl_ns.oid = tbl.relnamespace
WHERE ns.nspname = 'public'
  AND idx.relname = $1
  AND tbl_ns.nspname = 'public'
  AND tbl.relname = $2
  AND idx.relkind IN ('i', 'I')
LIMIT 1
`, name, table).Scan(&valid, &ready, &definition)
	return valid, ready, definition, err
}

func lookupMigrationConstraint(ctx context.Context, db migrationQueryConnection, table, name string) (string, bool, error) {
	var definition string
	var validated bool
	err := db.QueryRowContext(ctx, `
SELECT pg_get_constraintdef(c.oid), c.convalidated
FROM pg_constraint c
JOIN pg_class tbl ON tbl.oid = c.conrelid
JOIN pg_namespace ns ON ns.oid = tbl.relnamespace
WHERE ns.nspname = 'public'
  AND tbl.relname = $1
  AND c.conname = $2
`, table, name).Scan(&definition, &validated)
	return definition, validated, err
}

func lookupMigrationFunction(ctx context.Context, db migrationQueryConnection, name string, argCount int) (string, string, error) {
	var definition string
	var returnType string
	err := db.QueryRowContext(ctx, `
SELECT pg_get_functiondef(p.oid), pg_get_function_result(p.oid)
FROM pg_proc p
JOIN pg_namespace ns ON ns.oid = p.pronamespace
WHERE ns.nspname = 'public'
  AND p.proname = $1
  AND p.pronargs = $2
  AND p.prokind = 'f'
ORDER BY p.oid DESC
LIMIT 1
`, name, argCount).Scan(&definition, &returnType)
	return definition, returnType, err
}

func lookupMigrationTrigger(ctx context.Context, db migrationQueryConnection, table, name string) (string, error) {
	var definition string
	err := db.QueryRowContext(ctx, `
SELECT pg_get_triggerdef(t.oid)
FROM pg_trigger t
JOIN pg_class rel ON rel.oid = t.tgrelid
JOIN pg_namespace ns ON ns.oid = rel.relnamespace
WHERE ns.nspname = 'public'
  AND rel.relname = $1
  AND t.tgname = $2
  AND NOT t.tgisinternal
  AND t.tgenabled IN ('O', 'A')
`, table, name).Scan(&definition)
	return definition, err
}

func lookupMigrationSetting(ctx context.Context, db migrationQueryConnection, key string) (string, error) {
	var value string
	err := db.QueryRowContext(ctx, `
SELECT value
FROM public.settings
WHERE key = $1
`, key).Scan(&value)
	return value, err
}

func migrationContractTextContains(value, fragment string) bool {
	normalize := func(input string) string {
		input = strings.ToLower(strings.ReplaceAll(input, `"`, ""))
		return strings.Join(strings.Fields(input), " ")
	}
	return strings.Contains(normalize(value), normalize(fragment))
}
