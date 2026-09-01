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
    'password="$1"; user="$2"; database="$3"; shift 3; PGPASSWORD="$password" exec psql -X -v ON_ERROR_STOP=1 -U "$user" -d "$database" "$@"' \
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
FROM schema_migrations;
SELECT filename, checksum, applied_at
FROM schema_migrations
ORDER BY applied_at NULLS FIRST, filename;
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
FROM atlas_schema_revisions;
SELECT version, description, type, applied, total, executed_at, hash
FROM atlas_schema_revisions
ORDER BY executed_at NULLS FIRST, version;
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
         WHEN key IN ('ACTIVITY_CONFIG', 'ACTIVITY_CENTER_CONFIG', 'invoice_config', 'battle_pass_config')
           THEN 'enabled=' || COALESCE((regexp_match(value, '"enabled"[[:space:]]*:[[:space:]]*(true|false)'))[1], 'unknown')
                || ';value_length=' || length(value)::text
         WHEN key ILIKE '%secret%' OR key ILIKE '%password%' OR key ILIKE '%token%' THEN '[redacted]'
         ELSE value
       END AS value
FROM settings
WHERE key IN (
  'subnexus_checkin_enabled', 'subnexus_leaderboard_enabled',
  'subnexus_activity_center_enabled', 'subnexus_marquee_enabled',
  'subnexus_first_recharge_enabled', 'subnexus_invite_rewards_enabled',
  'invoice_enabled', 'battle_pass_enabled',
  'affiliate_enabled', 'channel_monitor_enabled',
  'ACTIVITY_CONFIG', 'ACTIVITY_CENTER_CONFIG', 'invoice_config', 'battle_pass_config'
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
