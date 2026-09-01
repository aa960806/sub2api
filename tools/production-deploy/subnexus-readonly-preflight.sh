#!/usr/bin/env bash
set -Eeuo pipefail

# Target-fork Batch 0 preflight. This script is intentionally read-only:
# it never runs migrations, backups, DDL/DML, builds, restarts, or cutover.
# Credentials are read from the running application/container environment and
# are used only for the short-lived psql/redis-cli process; they are never
# written to the evidence file or stdout.

app_container="${1:?usage: $0 APP_CONTAINER [PUBLIC_HEALTH_URL] [EVIDENCE_ROOT]}"
public_url="${2:-}"
evidence_root="${3:-/root/subnexus-migration/preflight}"

case "$evidence_root" in
  ""|/|/root|/root/|/srv|/srv/|/var|/var/|/home|/home/)
    printf 'ERROR: evidence root is too broad: %s\n' "$evidence_root" >&2
    exit 1
    ;;
esac

for command_name in docker curl awk grep sed sort tr date mkdir chmod sha256sum flock; do
  command -v "$command_name" >/dev/null 2>&1 || {
    printf 'ERROR: missing command: %s\n' "$command_name" >&2
    exit 1
  }
done

fail() {
  printf 'ERROR: %s\n' "$*" >&2
  exit 1
}

docker inspect "$app_container" >/dev/null 2>&1 || fail "application container not found: $app_container"
[[ "$(docker inspect --format '{{.State.Running}}' "$app_container")" == "true" ]] ||
  fail "application container is not running: $app_container"

timestamp="$(date +%Y%m%d%H%M%S)"
evidence_dir="$evidence_root/$timestamp"
evidence_file="$evidence_dir/evidence.txt"
lock_file="$evidence_root/.preflight.lock"
mkdir -p "$evidence_dir"
chmod 700 "$evidence_root" "$evidence_dir"
exec 9>"$lock_file"
flock -n 9 || fail "another SubNexus preflight is already running"

env_value() {
  docker inspect --format '{{range .Config.Env}}{{println .}}{{end}}' "$app_container" |
    awk -F= -v wanted="$1" '$1 == wanted {sub(/^[^=]*=/, ""); print; exit}'
}

app_port="$(docker inspect --format '{{range $containerPort, $bindings := .HostConfig.PortBindings}}{{range $bindings}}{{if eq $containerPort "8080/tcp"}}{{println .HostPort}}{{end}}{{end}}{{end}}' "$app_container" | awk 'NF {print; exit}')"
[[ -n "$app_port" ]] || fail "no published 8080/tcp port found"
curl -fsS --max-time 8 "http://127.0.0.1:${app_port}/health" >/dev/null || fail "local health check failed"
if [[ -n "$public_url" ]]; then
  curl -fsS --max-time 12 "$public_url" >/dev/null || fail "public health check failed"
fi

database_container="$(env_value DATABASE_HOST)"
database_name="$(env_value DATABASE_DBNAME)"
database_name="${database_name:-$(env_value DATABASE_NAME)}"
database_name="${database_name:-$(env_value DATABASE_DB)}"
database_user="$(env_value DATABASE_USER)"
database_password="$(env_value DATABASE_PASSWORD)"
redis_container="$(env_value REDIS_HOST)"
redis_password="$(env_value REDIS_PASSWORD)"
redis_db="$(env_value REDIS_DB)"
redis_db="${redis_db:-0}"
[[ -n "$database_container" && -n "$database_name" && -n "$database_user" ]] ||
  fail "DATABASE_HOST, DATABASE_USER, and a database name must be present in app environment"
[[ -n "$redis_container" ]] || fail "REDIS_HOST is missing from app environment"
docker inspect "$database_container" >/dev/null 2>&1 || fail "database container not found: $database_container"
docker inspect "$redis_container" >/dev/null 2>&1 || fail "redis container not found: $redis_container"
[[ "$(docker inspect --format '{{.State.Running}}' "$database_container")" == "true" ]] ||
  fail "database container is not running"
[[ "$(docker inspect --format '{{.State.Running}}' "$redis_container")" == "true" ]] ||
  fail "redis container is not running"

mapfile -t app_networks < <(docker inspect --format '{{range $name, $_ := .NetworkSettings.Networks}}{{println $name}}{{end}}' "$app_container" | awk 'NF')
shared_db_network=""
shared_redis_network=""
for network_name in "${app_networks[@]}"; do
  members="$(docker network inspect --format '{{range .Containers}}{{println .Name}}{{end}}' "$network_name")"
  if printf '%s\n' "$members" | grep -Fxq "$database_container"; then
    shared_db_network="$network_name"
  fi
  if printf '%s\n' "$members" | grep -Fxq "$redis_container"; then
    shared_redis_network="$network_name"
  fi
done
[[ -n "$shared_db_network" ]] || fail "application and database do not share a Docker network"
[[ -n "$shared_redis_network" ]] || fail "application and redis do not share a Docker network"
docker exec "$database_container" sh -c 'command -v psql >/dev/null' || fail "psql is missing in database container"
docker exec "$redis_container" sh -c 'command -v redis-cli >/dev/null' || fail "redis-cli is missing in redis container"

db_psql() {
  docker exec -i "$database_container" sh -c \
    'password="$1"; user="$2"; database="$3"; shift 3; PGPASSWORD="$password" PGOPTIONS="-c default_transaction_read_only=on -c statement_timeout=30s -c lock_timeout=3s" exec psql -X -v ON_ERROR_STOP=1 -U "$user" -d "$database" "$@"' \
    sh "$database_password" "$database_user" "$database_name" "$@"
}

redis_cli() {
  docker exec "$redis_container" sh -c \
    'password="$1"; database="$2"; shift 2; REDISCLI_AUTH="$password" exec redis-cli --raw -n "$database" "$@"' \
    sh "$redis_password" "$redis_db" "$@"
}

container_image="$(docker inspect --format '{{.Config.Image}}' "$app_container")"
container_id="$(docker inspect --format '{{.Id}}' "$app_container")"
container_uid="$(docker exec "$app_container" awk '/^Uid:/{print $2}' /proc/1/status 2>/dev/null || true)"
container_no_new_privs="$(docker exec "$app_container" awk '/^NoNewPrivs:/{print $2}' /proc/1/status 2>/dev/null || true)"
container_restarts="$(docker inspect --format '{{.RestartCount}}' "$app_container")"

{
  printf 'PREFLIGHT_KIND=read-only\n'
  printf 'CAPTURED_AT=%s\n' "$(date --iso-8601=seconds)"
  printf 'APPLICATION_CONTAINER=%s\n' "$app_container"
  printf 'APPLICATION_IMAGE=%s\n' "$container_image"
  printf 'APPLICATION_ID=%s\n' "$container_id"
  printf 'APPLICATION_LOCAL_PORT=%s\n' "$app_port"
  printf 'APPLICATION_UID=%s\n' "${container_uid:-unknown}"
  printf 'APPLICATION_NO_NEW_PRIVS=%s\n' "${container_no_new_privs:-unknown}"
  printf 'APPLICATION_RESTARTS=%s\n' "$container_restarts"
  printf 'APPLICATION_LOCAL_HEALTH=passed\n'
  if [[ -n "$public_url" ]]; then
    printf 'APPLICATION_PUBLIC_HEALTH=passed\n'
  else
    printf 'APPLICATION_PUBLIC_HEALTH=skipped\n'
  fi
  printf 'DATABASE_CONTAINER=%s\n' "$database_container"
  printf 'DATABASE_NAME=%s\n' "$database_name"
  printf 'DATABASE_NETWORK=%s\n' "$shared_db_network"
  printf 'REDIS_CONTAINER=%s\n' "$redis_container"
  printf 'REDIS_DB=%s\n' "$redis_db"
  printf 'REDIS_NETWORK=%s\n' "$shared_redis_network"
  printf '\n=== APPLICATION_MOUNTS ===\n'
  docker inspect --format '{{range .Mounts}}{{printf "%s|%s|%s|%s\n" .Type .Source .Destination .RW}}{{end}}' "$app_container"
  printf '\n=== APPLICATION_NETWORKS ===\n'
  printf '%s\n' "${app_networks[@]}"
} > "$evidence_file"

chmod 600 "$evidence_file"

db_psql -P pager=off <<'SQL' >> "$evidence_file"
\pset footer off
\pset null '(null)'
\echo === DATABASE_SESSION ===
BEGIN READ ONLY;
SET LOCAL default_transaction_read_only = on;
SET LOCAL lock_timeout = '3s';
SET LOCAL statement_timeout = '30s';
SELECT current_database() AS database_name,
       current_user AS database_user,
       current_setting('transaction_read_only') AS transaction_read_only,
       current_setting('server_version') AS postgres_version;
SELECT to_regclass('public.schema_migrations') AS schema_migrations_table,
       to_regclass('public.atlas_schema_revisions') AS atlas_schema_revisions_table;
COMMIT;
SQL

schema_table="$(db_psql -Atc "SELECT to_regclass('public.schema_migrations')")"
if [[ "$schema_table" != "" && "$schema_table" != "(null)" ]]; then
  schema_columns="$(db_psql -Atc "SELECT string_agg(column_name, ',' ORDER BY ordinal_position) FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'schema_migrations'")"
  if [[ ",$schema_columns," == *,filename,* && ",$schema_columns," == *,checksum,* && ",$schema_columns," == *,applied_at,* ]]; then
    db_psql -P pager=off <<'SQL' >> "$evidence_file"
\pset footer off
\pset null '(null)'
\echo === SCHEMA_MIGRATIONS ===
BEGIN READ ONLY;
SET LOCAL default_transaction_read_only = on;
SELECT COUNT(*) AS migration_count,
       COALESCE(MAX(applied_at)::text, '(none)') AS latest_applied_at,
       COALESCE(MAX(filename), '(none)') AS lexicographic_latest_filename
FROM public.schema_migrations;
SELECT filename, checksum, applied_at
FROM public.schema_migrations
ORDER BY applied_at NULLS FIRST, filename;
SELECT filename, checksum, applied_at
FROM public.schema_migrations
ORDER BY applied_at DESC NULLS LAST, filename DESC
LIMIT 1;
COMMIT;
SQL
  else
    {
      printf '\n=== SCHEMA_MIGRATIONS ===\n'
      printf 'SCHEMA_MISMATCH_COLUMNS=%s\n' "${schema_columns:-'(none)'}"
      printf 'REQUIRED_COLUMNS=filename,checksum,applied_at\n'
    } >> "$evidence_file"
  fi
else
  printf '\n=== SCHEMA_MIGRATIONS ===\nABSENT\n' >> "$evidence_file"
fi

atlas_table="$(db_psql -Atc "SELECT to_regclass('public.atlas_schema_revisions')")"
if [[ "$atlas_table" != "" && "$atlas_table" != "(null)" ]]; then
  atlas_columns="$(db_psql -Atc "SELECT string_agg(column_name, ',' ORDER BY ordinal_position) FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'atlas_schema_revisions'")"
  if [[ ",$atlas_columns," == *,version,* && ",$atlas_columns," == *,hash,* && ",$atlas_columns," == *,applied,* && ",$atlas_columns," == *,executed_at,* ]]; then
    db_psql -P pager=off <<'SQL' >> "$evidence_file"
\pset footer off
\pset null '(null)'
\echo === ATLAS_SCHEMA_REVISIONS ===
BEGIN READ ONLY;
SET LOCAL default_transaction_read_only = on;
SELECT COUNT(*) AS revision_count,
       COALESCE(MAX(executed_at)::text, '(none)') AS latest_executed_at
FROM public.atlas_schema_revisions;
SELECT version, description, type, applied, total, executed_at, hash
FROM public.atlas_schema_revisions
ORDER BY executed_at NULLS FIRST, version;
SELECT version, description, type, applied, total, executed_at, hash
FROM public.atlas_schema_revisions
ORDER BY executed_at DESC NULLS LAST, version DESC
LIMIT 1;
COMMIT;
SQL
  else
    {
      printf '\n=== ATLAS_SCHEMA_REVISIONS ===\n'
      printf 'SCHEMA_MISMATCH_COLUMNS=%s\n' "${atlas_columns:-'(none)'}"
      printf 'REQUIRED_COLUMNS=version,hash,applied,executed_at\n'
    } >> "$evidence_file"
  fi
else
  printf '\n=== ATLAS_SCHEMA_REVISIONS ===\nABSENT\n' >> "$evidence_file"
fi

if [[ "$schema_table" != "" && "$schema_table" != "(null)" \
  && ",${schema_columns:-}," == *,filename,* \
  && ",${schema_columns:-}," == *,checksum,* \
  && ",${schema_columns:-}," == *,applied_at,* ]]; then
  db_psql -P pager=off <<'SQL' >> "$evidence_file"
\pset footer off
\pset null '(null)'
\echo === RENAMED_MIGRATION_CONTRACT_INVENTORY ===
BEGIN READ ONLY;
SET LOCAL default_transaction_read_only = on;
-- Exact old/target filename pairs are checked locally against the reviewed
-- allowlist; production evidence supplies only the recorded rows.
SELECT filename, checksum, applied_at
FROM public.schema_migrations
WHERE filename = ANY (ARRAY[
  '205_add_group_peak_rate_multiplier_compat.sql',
  '158_add_group_peak_rate_multiplier.sql',
  '240_enable_grok_media_generation_groups.sql',
  '158_enable_grok_media_generation_groups.sql',
  '175_add_usage_log_long_context_billing.sql',
  '174_add_usage_log_long_context_billing.sql',
  '204_add_usage_logs_api_key_latest_ip_index_notx.sql',
  '174_add_usage_logs_api_key_latest_ip_index_notx.sql',
  '177_add_ops_system_logs_host.sql',
  '175_add_ops_system_logs_host.sql',
  '177a_add_ops_system_logs_host_index_notx.sql',
  '175a_add_ops_system_logs_host_index_notx.sql',
  '253_audit_logs.sql',
  '180_audit_logs.sql',
  '182_ops_ingress_reject_aggregates.sql',
  '183_ops_ingress_reject_aggregates.sql',
  '183_auth_cache_invalidation_outbox.sql',
  '184_auth_cache_invalidation_outbox.sql',
  '190_group_reasoning_effort_policy.sql',
  '185_group_reasoning_effort_policy.sql',
  '189_alipay_mobile_precreate_deep_link.sql',
  '186_alipay_mobile_precreate_deep_link.sql',
  '193_allow_live_usage_request_type.sql',
  '188_allow_live_usage_request_type.sql',
  '197_passkey_credentials.sql',
  '191_passkey_credentials.sql',
  '209_add_usage_log_upstream_model_mismatch_index_notx.sql',
  '195_add_usage_log_upstream_model_mismatch_index_notx.sql',
  '235_group_video_model_prices.sql',
  '217_group_video_model_prices.sql',
  '236_group_audio_voice_pricing.sql',
  '218_group_audio_voice_pricing.sql',
  '237_group_search_price_per_1k.sql',
  '219_group_search_price_per_1k.sql',
  '239_group_model_pricing.sql',
  '221_group_model_pricing.sql',
  '241_group_usage_daily_rollups.sql',
  '222_group_usage_daily_rollups.sql',
  '242_group_usage_rollup_timezone.sql',
  '223_group_usage_rollup_timezone.sql',
  '244_backfill_codex_fingerprint_seed.sql',
  '225_backfill_codex_fingerprint_seed.sql',
  '245_channel_model_time_pricing.sql',
  '225_channel_model_time_pricing.sql',
  '246_add_usage_log_effective_model_indexes_notx.sql',
  '226_add_usage_log_effective_model_indexes_notx.sql'
]::text[])
ORDER BY applied_at NULLS FIRST, filename;

SELECT table_name, column_name, data_type, udt_name, is_nullable, column_default
FROM information_schema.columns
WHERE table_schema = 'public'
  AND (
    (table_name = 'groups' AND column_name IN (
      'peak_rate_enabled', 'peak_start', 'peak_end', 'peak_rate_multiplier',
      'allow_image_generation', 'max_reasoning_effort', 'reasoning_effort_mappings',
      'video_model_prices', 'audio_realtime_price_per_min',
      'audio_tts_price_per_million_chars', 'audio_stt_price_per_hour',
      'search_price_per_1k', 'long_context_pricing_enabled', 'model_pricing'
    ))
    OR (table_name = 'usage_logs' AND column_name IN (
      'long_context_billing_applied', 'request_type', 'upstream_model_mismatch'
    ))
    OR (table_name = 'ops_system_logs' AND column_name = 'host')
    OR (table_name = 'channel_model_pricing' AND column_name = 'time_pricing')
    OR table_name IN (
      'audit_logs',
      'ops_ingress_reject_aggregates',
      'auth_cache_invalidation_outbox',
      'passkey_user_handles',
      'passkey_credentials',
      'usage_group_daily_rollups',
      'usage_group_rollup_state'
    )
  )
ORDER BY table_name, ordinal_position;

SELECT idx.relname AS index_name,
       tbl.relname AS table_name,
       i.indisvalid,
       i.indisready,
       i.indisunique,
       replace(pg_get_indexdef(i.indexrelid), E'\n', ' ') AS index_definition
FROM pg_class idx
JOIN pg_index i ON i.indexrelid = idx.oid
JOIN pg_class tbl ON tbl.oid = i.indrelid
JOIN pg_namespace ns ON ns.oid = tbl.relnamespace
WHERE ns.nspname = 'public'
  AND idx.relname = ANY (ARRAY[
    'idx_usage_logs_api_key_latest_ip',
    'idx_ops_system_logs_host_created_at',
    'idx_audit_logs_created_at_id',
    'idx_audit_logs_actor_created',
    'idx_audit_logs_action',
    'idx_audit_logs_client_ip',
    'idx_ops_ingress_reject_aggregates_bucket',
    'idx_ops_ingress_reject_aggregates_reason_bucket',
    'idx_ops_ingress_reject_aggregates_ip_bucket',
    'idx_auth_cache_invalidation_outbox_available',
    'idx_auth_cache_invalidation_outbox_lease',
    'idx_auth_cache_invalidation_outbox_cache_key',
    'idx_auth_cache_invalidation_outbox_created_at',
    'passkey_credentials_user_id_idx',
    'passkey_credentials_last_used_at_idx',
    'idx_usage_logs_upstream_model_mismatch_created_at',
    'idx_usage_logs_effective_requested_model_created',
    'idx_usage_logs_effective_upstream_model_created'
  ]::text[])
ORDER BY idx.relname;

SELECT c.conrelid::regclass::text AS table_name,
       c.conname,
       c.contype,
       c.convalidated,
       c.confdeltype,
       NULLIF(c.confrelid, 0)::regclass::text AS referenced_table,
       replace(pg_get_constraintdef(c.oid), E'\n', ' ') AS constraint_definition
FROM pg_constraint c
JOIN pg_class tbl ON tbl.oid = c.conrelid
JOIN pg_namespace ns ON ns.oid = tbl.relnamespace
WHERE ns.nspname = 'public'
  AND (
    (tbl.relname = 'ops_ingress_reject_aggregates')
    OR (tbl.relname = 'usage_logs' AND c.conname = 'usage_logs_request_type_check')
    OR (tbl.relname = 'auth_cache_invalidation_outbox')
    OR tbl.relname IN (
      'audit_logs',
      'passkey_user_handles',
      'passkey_credentials',
      'usage_group_daily_rollups',
      'usage_group_rollup_state'
    )
  )
ORDER BY table_name, c.conname;

SELECT n.nspname AS schema_name,
       p.proname,
       pg_get_function_identity_arguments(p.oid) AS arguments,
       replace(pg_get_functiondef(p.oid), E'\n', ' ') AS function_definition
FROM pg_proc p
JOIN pg_namespace n ON n.oid = p.pronamespace
WHERE n.nspname = 'public'
  AND p.proname = ANY (ARRAY[
    'enqueue_auth_cache_invalidation',
    'enqueue_api_key_auth_cache_invalidation',
    'enqueue_user_auth_cache_invalidation',
    'enqueue_group_auth_cache_invalidation',
    'enqueue_allowed_group_auth_cache_invalidation',
    'invalidate_group_usage_rollup_state',
    'invalidate_group_usage_rollup_state_after_insert'
  ]::text[])
ORDER BY p.proname, arguments;

SELECT n.nspname AS schema_name,
       c.relname AS table_name,
       t.tgname,
       t.tgenabled,
       replace(pg_get_triggerdef(t.oid), E'\n', ' ') AS trigger_definition
FROM pg_trigger t
JOIN pg_class c ON c.oid = t.tgrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE NOT t.tgisinternal
  AND n.nspname = 'public'
  AND t.tgname = ANY (ARRAY[
    'trg_api_keys_auth_cache_invalidation',
    'trg_users_auth_cache_invalidation',
    'trg_groups_auth_cache_invalidation',
    'trg_user_allowed_groups_auth_cache_invalidation',
    'usage_logs_group_rollup_invalidate_insert',
    'usage_logs_group_rollup_invalidate_delete',
    'usage_logs_group_rollup_invalidate_update'
  ]::text[])
ORDER BY c.relname, t.tgname;

COMMIT;
SQL

  groups_table="$(db_psql -Atc "SELECT to_regclass('public.groups')")"
  groups_columns="$(db_psql -Atc "SELECT string_agg(column_name, ',' ORDER BY column_name) FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'groups'")"
  if [[ "$groups_table" != "" && "$groups_table" != "(null)" && ",$groups_columns," == *,platform,* && ",$groups_columns," == *,allow_image_generation,* ]]; then
    db_psql -P pager=off <<'SQL' >> "$evidence_file"
\pset footer off
\pset null '(null)'
\echo === RENAMED_MIGRATION_DATA_POSTCONDITIONS_GROUPS ===
\echo NOTE=mutable_operator_setting_nonzero_is_not_an_alias_failure
BEGIN READ ONLY;
SET LOCAL default_transaction_read_only = on;
SELECT 'grok_allow_image_generation_false' AS check_name,
       COUNT(*)::bigint AS observed_rows
FROM public.groups
WHERE platform = 'grok' AND allow_image_generation = FALSE;
COMMIT;
SQL
  else
    printf '\n=== RENAMED_MIGRATION_DATA_POSTCONDITIONS_GROUPS ===\nSKIPPED_REQUIRED_OBJECT_ABSENT\n' >> "$evidence_file"
  fi

  if [[ "$groups_table" != "" && "$groups_table" != "(null)" && ",$groups_columns," == *,long_context_pricing_enabled,* ]]; then
    db_psql -P pager=off <<'SQL' >> "$evidence_file"
\pset footer off
\pset null '(null)'
\echo === RENAMED_MIGRATION_DATA_POSTCONDITIONS_LONG_CONTEXT ===
\echo NOTE=mutable_operator_setting_nonzero_is_not_an_alias_failure
BEGIN READ ONLY;
SET LOCAL default_transaction_read_only = on;
SELECT 'groups_long_context_not_true' AS check_name,
       COUNT(*)::bigint AS observed_rows
FROM public.groups
WHERE long_context_pricing_enabled IS DISTINCT FROM TRUE;
COMMIT;
SQL
  else
    printf '\n=== RENAMED_MIGRATION_DATA_POSTCONDITIONS_LONG_CONTEXT ===\nSKIPPED_REQUIRED_OBJECT_ABSENT\n' >> "$evidence_file"
  fi

  accounts_table="$(db_psql -Atc "SELECT to_regclass('public.accounts')")"
  accounts_columns="$(db_psql -Atc "SELECT string_agg(column_name, ',' ORDER BY column_name) FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'accounts'")"
  if [[ "$accounts_table" != "" && "$accounts_table" != "(null)" && ",$accounts_columns," == *,deleted_at,* && ",$accounts_columns," == *,platform,* && ",$accounts_columns," == *,type,* && ",$accounts_columns," == *,extra,* ]]; then
    db_psql -P pager=off <<'SQL' >> "$evidence_file"
\pset footer off
\pset null '(null)'
\echo === RENAMED_MIGRATION_DATA_POSTCONDITIONS_CODEX_SEED ===
BEGIN READ ONLY;
SET LOCAL default_transaction_read_only = on;
SELECT 'invalid_codex_fingerprint_seed' AS check_name,
       COUNT(*)::bigint AS violating_rows
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
  );
COMMIT;
SQL
  else
    printf '\n=== RENAMED_MIGRATION_DATA_POSTCONDITIONS_CODEX_SEED ===\nSKIPPED_REQUIRED_OBJECT_ABSENT\n' >> "$evidence_file"
  fi

  rollup_state_table="$(db_psql -Atc "SELECT to_regclass('public.usage_group_rollup_state')")"
  rollup_state_columns="$(db_psql -Atc "SELECT string_agg(column_name, ',' ORDER BY column_name) FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'usage_group_rollup_state'")"
  if [[ "$rollup_state_table" != "" && "$rollup_state_table" != "(null)" && ",$rollup_state_columns," == *,id,* ]]; then
    db_psql -P pager=off <<'SQL' >> "$evidence_file"
\pset footer off
\pset null '(null)'
\echo === RENAMED_MIGRATION_DATA_POSTCONDITIONS_ROLLUP ===
BEGIN READ ONLY;
SET LOCAL default_transaction_read_only = on;
SELECT 'usage_group_rollup_state_singleton' AS check_name,
       COUNT(*)::bigint AS id_one_rows
FROM public.usage_group_rollup_state
WHERE id = 1;
COMMIT;
SQL
  else
    printf '\n=== RENAMED_MIGRATION_DATA_POSTCONDITIONS_ROLLUP ===\nSKIPPED_REQUIRED_OBJECT_ABSENT\n' >> "$evidence_file"
  fi
elif [[ "$schema_table" != "" && "$schema_table" != "(null)" ]]; then
  printf '\n=== RENAMED_MIGRATION_CONTRACT_INVENTORY ===\nSKIPPED_SCHEMA_MIGRATIONS_COLUMNS_MISMATCH=%s\n' "${schema_columns:-'(none)'}" >> "$evidence_file"
else
  printf '\n=== RENAMED_MIGRATION_CONTRACT_INVENTORY ===\nSKIPPED_SCHEMA_MIGRATIONS_ABSENT\n' >> "$evidence_file"
fi

db_psql -P pager=off <<'SQL' >> "$evidence_file"
\pset footer off
\pset null '(null)'
\echo === SCHEMA_OBJECT_INVENTORY ===
BEGIN READ ONLY;
SET LOCAL default_transaction_read_only = on;
SELECT table_name
FROM information_schema.tables
WHERE table_schema = 'public'
  AND (table_name ILIKE '%invoice%' OR table_name ILIKE '%battle%' OR table_name ILIKE '%activity%' OR table_name ILIKE '%checkin%' OR table_name ILIKE '%leaderboard%')
ORDER BY table_name;
SELECT table_name, column_name, data_type, is_nullable
FROM information_schema.columns
WHERE table_schema = 'public'
  AND (table_name ILIKE '%invoice%' OR table_name ILIKE '%battle%' OR table_name ILIKE '%activity%' OR table_name ILIKE '%checkin%' OR table_name ILIKE '%leaderboard%')
ORDER BY table_name, ordinal_position;
SELECT indexname, tablename, indexdef
FROM pg_indexes
WHERE schemaname = 'public'
  AND (indexname ILIKE '%invoice%' OR indexname ILIKE '%battle%' OR indexname ILIKE '%activity%' OR indexname ILIKE '%checkin%' OR indexname ILIKE '%leaderboard%')
ORDER BY indexname;
COMMIT;
SQL

settings_table="$(db_psql -Atc "SELECT to_regclass('public.settings')")"
if [[ "$settings_table" != "" && "$settings_table" != "(null)" ]]; then
  db_psql -P pager=off <<'SQL' >> "$evidence_file"
\pset footer off
\pset null '(null)'
\echo === SETTINGS_AND_COUNTS ===
BEGIN READ ONLY;
SET LOCAL default_transaction_read_only = on;
SELECT key,
       CASE
         WHEN key = 'ACTIVITY_CONFIG'
           THEN 'checkin_enabled=' || COALESCE((regexp_match(value, '(?i)"checkin_enabled"[[:space:]]*:[[:space:]]*(true|false)'))[1], 'unknown')
                || ';leaderboard_enabled=' || COALESCE((regexp_match(value, '(?i)"leaderboard_enabled"[[:space:]]*:[[:space:]]*(true|false)'))[1], 'unknown')
                || ';broadcast_enabled=' || COALESCE((regexp_match(value, '(?i)"broadcast_enabled"[[:space:]]*:[[:space:]]*(true|false)'))[1], 'unknown')
                || ';first_recharge_enabled=' || COALESCE((regexp_match(value, '(?i)"first_recharge_enabled"[[:space:]]*:[[:space:]]*(true|false)'))[1], 'unknown')
                || ';value_length=' || length(value)::text
         WHEN key IN ('ACTIVITY_CENTER_CONFIG', 'INVOICE_CONFIG', 'invoice_config', 'BATTLE_PASS_CONFIG', 'battle_pass_config')
           THEN 'enabled=' || COALESCE((regexp_match(value, '(?i)"enabled"[[:space:]]*:[[:space:]]*(true|false)'))[1], 'unknown')
                || ';value_length=' || length(value)::text
         WHEN key ILIKE '%secret%' OR key ILIKE '%password%' OR key ILIKE '%token%' THEN '[redacted]'
         ELSE value
       END AS value
FROM public.settings
WHERE key IN (
  'subnexus_checkin_enabled', 'subnexus_leaderboard_enabled',
  'subnexus_activity_center_enabled', 'subnexus_marquee_enabled',
  'subnexus_first_recharge_enabled', 'subnexus_invite_rewards_enabled',
  'invoice_enabled', 'battle_pass_enabled',
  'affiliate_enabled', 'channel_monitor_enabled',
  'ALIPAY_MOBILE_PRECREATE_DEEP_LINK',
  'ACTIVITY_CONFIG', 'ACTIVITY_CENTER_CONFIG', 'INVOICE_CONFIG', 'invoice_config',
  'BATTLE_PASS_CONFIG', 'battle_pass_config'
)
ORDER BY key;
SELECT relname AS table_name, n_live_tup::bigint AS estimated_rows
FROM pg_stat_user_tables
WHERE relname IN ('users', 'payment_orders', 'subscriptions', 'usage_logs', 'settings', 'schema_migrations', 'atlas_schema_revisions')
ORDER BY relname;
COMMIT;
SQL
else
  printf '\n=== SETTINGS_AND_COUNTS ===\nSETTINGS_TABLE=ABSENT\n' >> "$evidence_file"
fi

{
  printf '\n=== STORAGE_ENV_KEYS ===\n'
  docker inspect --format '{{range .Config.Env}}{{println .}}{{end}}' "$app_container" |
    awk -F= '{name=tolower($1); if (name ~ /(upload|storage|invoice|media|file).*(dir|path|root|bucket|endpoint|url)?$/ || name ~ /^(upload|storage|invoice|media|file)(_|$)/) print $1}' |
    sort -u
  printf '\n=== NGINX_RUNTIME ===\n'
  if command -v nginx >/dev/null 2>&1; then
    nginx_test="$(nginx -t 2>&1 || true)"
    printf 'NGINX_TEST=%s\n' "$(printf '%s' "$nginx_test" | sed -E 's/(password|secret|token|authorization)[^;]*/\1 [redacted]/Ig' | tr '\n' ' ')"
    nginx -T 2>&1 |
      awk '/^[[:space:]]*(listen|server_name|proxy_pass|root|alias|client_max_body_size)[[:space:]]/ {print}' |
      sed -E 's#(https?://)([^/@:]+):[^/@]+@#\1[redacted]@#g; s/(password|secret|token|authorization)[^;]*/\1 [redacted]/Ig' || true
  else
    printf 'NGINX_COMMAND=unavailable\n'
  fi
} >> "$evidence_file"

{
  printf '\n=== REDIS_RUNTIME ===\n'
  printf 'PING=%s\n' "$(redis_cli ping)"
  redis_cli info persistence | awk -F: '/^(aof_enabled|rdb_last_save_time|rdb_last_bgsave_status):/{print}'
  printf 'DBSIZE=%s\n' "$(redis_cli dbsize)"
  printf 'KEYSPACE=%s\n' "$(redis_cli info keyspace | awk -F: '/^db[0-9]+:/{print; exit}')"
} >> "$evidence_file"

chmod 600 "$evidence_file"
sha256sum "$evidence_file" > "$evidence_file.sha256"
chmod 600 "$evidence_file.sha256"

printf 'READ_ONLY_PREFLIGHT_OK\n'
printf 'EVIDENCE=%s\n' "$evidence_file"
printf 'CHECKSUM=%s\n' "$evidence_file.sha256"
printf 'NO_MIGRATION_OR_DEPLOYMENT_PERFORMED=true\n'
