#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
subject="$script_dir/subnexus-readonly-preflight.sh"

fail() {
  printf 'TEST ERROR: %s\n' "$*" >&2
  exit 1
}

assert_eq() {
  local expected="$1"
  local actual="$2"
  local label="$3"
  [[ "$actual" == "$expected" ]] ||
    fail "$label: expected '$expected', got '$actual'"
}

assert_contains() {
  local expected="$1"
  grep -Fq -- "$expected" "$subject" || fail "missing invariant: $expected"
}

assert_not_contains() {
  local forbidden="$1"
  if grep -Fq -- "$forbidden" "$subject"; then
    fail "forbidden pattern present: $forbidden"
  fi
}

bash -n "$subject"

# Evaluate the production resolver itself. Its closing brace is the only
# unindented brace in this function, so the extraction remains deterministic.
resolver_source="$(sed -n '/^resolve_shared_container() {$/,/^}$/p' "$subject")"
[[ -n "$resolver_source" ]] || fail "resolver function not found"
eval "$resolver_source"

ipv4_source="$(sed -n '/^valid_ipv4() {$/,/^}$/p' "$subject")"
[[ -n "$ipv4_source" ]] || fail "IPv4 validator not found"
eval "$ipv4_source"
capture_ipv4_source="$(sed -n '/^capture_ipv4_or_fail() {$/,/^}$/p' "$subject")"
[[ -n "$capture_ipv4_source" ]] || fail "IPv4 capture helper not found"
eval "$capture_ipv4_source"

valid_ipv4 '172.20.0.3'
if valid_ipv4 '172.20.0.999' || valid_ipv4 '2001:db8::3' || valid_ipv4 '172.20.0.3/16'; then
  fail "invalid or IPv6 addresses must fail closed"
fi
assert_eq '172.20.0.3' "$(capture_ipv4_or_fail '172.20.0.3/16' database)" "captured IPv4 normalization"
if (capture_ipv4_or_fail '2001:db8::3/64' database >/dev/null 2>&1); then
  fail "IPv6-only dependency must fail closed"
fi

fixture="normal"
docker() {
  if [[ "$1" == "network" && "$2" == "inspect" ]]; then
    local format="${4}"
    local network_name="${5}"
    if [[ "$format" == "{{.Id}}" ]]; then
      case "$fixture:$network_name" in
        normal:app-net|ambiguous:app-net) printf '%s\n' 'app-net-id' ;;
        ambiguous:other-net) printf '%s\n' 'other-net-id' ;;
        inspect-error:app-net) return 1 ;;
        *) return 1 ;;
      esac
      return 0
    fi
    case "$fixture:$network_name" in
      normal:app-net)
        printf '%s\n' \
          'app-id|subnexus-cutover|172.20.0.2/16|' \
          'db-id|sub2api-postgres|172.20.0.3/16|' \
          'redis-id|sub2api-redis|172.20.0.4/16|'
        ;;
      ambiguous:app-net)
        printf '%s\n' \
          'app-id|subnexus-cutover|172.20.0.2/16|' \
          'db-id|sub2api-postgres|172.20.0.3/16|'
        ;;
      ambiguous:other-net)
        printf '%s\n' 'other-db-id|other-postgres|172.21.0.3/16|'
        ;;
      inspect-error:app-net)
        return 1
        ;;
    esac
    return 0
  fi

  if [[ "$1" == "inspect" && "$2" == "--format" ]]; then
    local format="${3}"
    local container_id="${4}"
    if [[ "$format" == *NetworkSettings.Networks* ]]; then
      case "$fixture:$container_id" in
        normal:app-id|ambiguous:app-id)
          printf '%s\n' 'app-net|subnexus-cutover'
          ;;
        normal:db-id|ambiguous:db-id)
          printf '%s\n' 'app-net|sub2api-postgres' 'app-net|postgres'
          ;;
        normal:redis-id)
          printf '%s\n' 'app-net|sub2api-redis' 'app-net|redis'
          ;;
        ambiguous:other-db-id)
          printf '%s\n' 'other-net|other-postgres' 'other-net|postgres'
          ;;
      esac
    fi
    return 0
  fi

  return 1
}

app_networks=(app-net)
assert_eq 'db-id|sub2api-postgres|app-net|app-net-id|172.20.0.3/16|' "$(resolve_shared_container postgres database)" "database alias"
assert_eq 'redis-id|sub2api-redis|app-net|app-net-id|172.20.0.4/16|' "$(resolve_shared_container redis redis)" "redis alias"
assert_eq 'db-id|sub2api-postgres|app-net|app-net-id|172.20.0.3/16|' "$(resolve_shared_container sub2api-postgres database)" "container name"
assert_eq 'db-id|sub2api-postgres|app-net|app-net-id|172.20.0.3/16|' "$(resolve_shared_container 172.20.0.3 database)" "container IPv4"

fixture="ambiguous"
app_networks=(app-net other-net)
if (resolve_shared_container postgres database >/dev/null 2>&1); then
  fail "ambiguous alias must fail closed"
fi

fixture="inspect-error"
app_networks=(app-net)
if (resolve_shared_container postgres database >/dev/null 2>&1); then
  fail "Docker network inspection failure must propagate"
fi

assert_contains 'umask 077'
assert_contains 'case "$-" in'
assert_contains 'set +x'
assert_contains '/srv/subnexus-migration/preflight'
assert_contains '[[ ! -L "$lock_file" ]]'
assert_contains 'READ_ONLY_PREFLIGHT_CAPTURED'
assert_contains 'PREFLIGHT_DECISION=pending_local_review'
assert_contains 'http_code'
assert_contains 'expected 2xx'
assert_contains 'database_psql_path='
assert_contains 'redis_cli_path='
assert_contains 'database_connect_host='
assert_contains 'redis_connect_host='
assert_contains 'sh "$database_connect_host"'
assert_contains 'sh "$redis_connect_host"'
assert_contains 'assert_runtime_identities "before_finalize"'
assert_contains 'script_sha256_final='
assert_contains '.HostIp'
assert_contains 'PGHOST="$host" PGPORT="$port" PGSSLMODE="$sslmode"'
assert_contains 'database name must be a simple PostgreSQL identifier'
assert_contains 'unset PGHOSTADDR PGSERVICE PGSERVICEFILE PGPASSFILE'
assert_contains 'PGPASSFILE=/dev/null'
assert_contains 'trigger_definition_md5'
assert_contains 'SETTINGS_TABLE_SHAPE=MISMATCH'
assert_contains 'Redis must run in standalone mode'
assert_not_contains 'PGSERVICE='
assert_not_contains 'READ_ONLY_PREFLIGHT_OK'
assert_not_contains 'sh "$database_password"'
assert_not_contains 'sh "$redis_password"'
assert_not_contains 'nginx -T'
assert_not_contains '.HostIP'

# Docker's PortBinding field is HostIp (lowercase p). Keep a small executable
# fixture here so a template spelling regression fails before production use.
port_source="$(sed -n '/^capture_app_port_bindings() {$/,/^}$/p' "$subject")"
[[ -n "$port_source" ]] || fail "port binding helper not found"
(
  eval "$port_source"
  docker() {
    if [[ "$1" == "inspect" && "$2" == "--format" && "$3" == *HostConfig.PortBindings* ]]; then
      printf '%s\n' '127.0.0.1|18083'
      return 0
    fi
    return 1
  }
  app_container_id='fixture-app'
  assert_eq '127.0.0.1|18083' "$(capture_app_port_bindings)" "Docker HostIp port binding"
)

printf 'subnexus readonly preflight tests passed\n'
