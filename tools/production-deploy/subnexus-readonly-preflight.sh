#!/usr/bin/env bash
set -Eeuo pipefail

# Target-fork Batch 0 preflight. This script is intentionally read-only:
# it never runs migrations, backups, DDL/DML, builds, restarts, or cutover.
# Credentials are read from the running application/container environment and
# are used only for the short-lived psql/redis-cli process; they are never
# written to the evidence file or stdout.

# Do not inherit an xtrace setting from a caller: the probe handles credentials
# in shell variables and must not echo their expansions to stderr.
case "$-" in
  *x*) set +x ;;
esac

umask 077

# The script runs as root and invokes host/container tooling by name. Keep the
# lookup path limited to standard root-owned locations so an inherited user
# PATH cannot substitute a different executable.
export PATH='/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin'

app_container="${1:?usage: $0 APP_CONTAINER [PUBLIC_HEALTH_URL] [EVIDENCE_ROOT]}"
public_url="${2:-}"
evidence_root="${3:-/root/subnexus-migration/preflight}"

fail() {
  printf 'ERROR: %s\n' "$*" >&2
  exit 1
}

[[ "$app_container" =~ ^([A-Za-z0-9][A-Za-z0-9_.-]{0,254}|[0-9a-fA-F]{12,64})$ ]] ||
  fail "application container must be a Docker name or hexadecimal ID"

if [[ -n "$public_url" ]]; then
  [[ "$public_url" =~ ^https?://[^@[:space:]]+$ ]] ||
    fail "public health URL must be an absolute http(s) URL"
fi

http_health_check() {
  local url="$1"
  local label="$2"
  local max_time="$3"
  local http_code

  http_code="$(curl --config /dev/null --noproxy '*' --connect-timeout 8 --max-time "$max_time" \
    --output /dev/null --write-out '%{http_code}' -- "$url")" ||
    fail "$label health check failed"
  [[ "$http_code" =~ ^2[0-9][0-9]$ ]] ||
    fail "$label health check returned HTTP $http_code (expected 2xx)"
}

for command_name in docker curl awk sed sort tr date mkdir chmod sha256sum flock cat realpath stat timeout python3; do
  command -v "$command_name" >/dev/null 2>&1 || {
    printf 'ERROR: missing command: %s\n' "$command_name" >&2
    exit 1
  }
done

[[ "$EUID" -eq 0 ]] || fail "run this script as root (for example with sudo)"

docker_binary="$(command -v docker)"
docker() {
  # Bound both daemon RPCs and docker exec calls. PostgreSQL/Redis add their
  # own query/connect limits below, while this outer limit covers DNS and
  # Docker daemon black holes as well.
  timeout --foreground --kill-after=5s 60s "$docker_binary" "$@"
}

# A local health URL is only meaningful when the Docker daemon inspected below
# is the local daemon. Refuse inherited remote overrides and non-default
# contexts so metadata and the host-side curl probe cannot refer to different
# machines.
[[ -z "${DOCKER_HOST:-}" ]] || fail "DOCKER_HOST must be unset for a local preflight"
docker_context="$(docker context show 2>/dev/null)" || fail "cannot determine Docker context"
[[ "$docker_context" == "default" ]] || fail "Docker context must be 'default': $docker_context"
docker_endpoint="$(docker context inspect --format '{{(index .Endpoints "docker").Host}}' default 2>/dev/null)" ||
  fail "cannot determine default Docker endpoint"
[[ "$docker_endpoint" == unix:///* ]] || fail "default Docker endpoint must be a local Unix socket"
docker_daemon_info="$(docker info --format '{{.Name}}|{{.ServerVersion}}|{{.DockerRootDir}}')" ||
  fail "cannot inspect Docker daemon"
IFS='|' read -r docker_daemon_name docker_server_version docker_root_dir <<< "$docker_daemon_info"
[[ -n "$docker_daemon_name" && -n "$docker_server_version" && -n "$docker_root_dir" ]] ||
  fail "Docker daemon identity is incomplete"

evidence_root="$(realpath -m -- "$evidence_root")" || fail "cannot normalize evidence root"
case "$evidence_root" in
  /root/subnexus-migration/preflight|/srv/subnexus-migration/preflight)
    ;;
  *)
    fail "evidence root must be /root/subnexus-migration/preflight or /srv/subnexus-migration/preflight"
    ;;
esac

script_path="$(realpath -e -- "${BASH_SOURCE[0]}")" || fail "cannot resolve preflight script path"
script_sha256="$(sha256sum "$script_path" | awk '{print toupper($1)}')" || fail "cannot hash preflight script"

ensure_secure_directory() {
  local directory="$1"
  local parent="${directory%/*}"

  [[ ! -L "$directory" ]] || fail "directory must not be a symbolic link: $directory"
  if [[ -e "$directory" ]]; then
    [[ -d "$directory" ]] || fail "path exists but is not a directory: $directory"
  else
    [[ -d "$parent" && ! -L "$parent" ]] || fail "secure parent directory is missing: $parent"
    [[ "$(stat -c '%u' -- "$parent")" == "0" ]] || fail "parent directory must be root-owned: $parent"
    mkdir -- "$directory" || fail "cannot create secure directory: $directory"
  fi

  [[ "$(realpath -e -- "$directory")" == "$directory" ]] || fail "directory resolved outside the approved path: $directory"
  [[ "$(stat -c '%u' -- "$directory")" == "0" ]] || fail "directory must be root-owned: $directory"
  chmod 700 -- "$directory"
}

docker inspect "$app_container" >/dev/null 2>&1 || fail "application container not found: $app_container"
app_container_id="$(docker inspect --format '{{.Id}}' "$app_container")" || fail "cannot identify application container"
[[ "$(docker inspect --format '{{.State.Running}}' "$app_container_id")" == "true" ]] ||
  fail "application container is not running: $app_container"

capture_container_networks() {
  local container_id="$1"
  docker inspect --format '{{range $name, $network := .NetworkSettings.Networks}}{{printf "%s|%s|%s|%s\n" $name $network.NetworkID $network.IPAddress $network.GlobalIPv6Address}}{{end}}' "$container_id" |
    LC_ALL=C sort
}

capture_network_member() {
  local network_name="$1"
  local container_id="$2"
  local network_id member member_id member_name member_ipv4 member_ipv6

  network_id="$(docker network inspect --format '{{.Id}}' "$network_name")" ||
    fail "failed to inspect Docker network '$network_name' identity"
  [[ -n "$network_id" ]] || fail "Docker network '$network_name' has no ID"
  member="$(docker network inspect --format '{{range $id, $container := .Containers}}{{printf "%s|%s|%s|%s\n" $id $container.Name $container.IPv4Address $container.IPv6Address}}{{end}}' "$network_name" |
    awk -F'|' -v wanted_id="$container_id" '$1 == wanted_id {print; exit}')" ||
    fail "failed to inspect Docker network '$network_name' members"
  [[ -n "$member" ]] || fail "container '$container_id' is no longer attached to Docker network '$network_name'"
  IFS='|' read -r member_id member_name member_ipv4 member_ipv6 <<< "$member"
  member_name="${member_name#/}"
  printf '%s|%s|%s|%s|%s\n' "$network_id" "$member_id" "$member_name" "$member_ipv4" "$member_ipv6"
}

capture_image_repo_digests() {
  local image_id="$1"
  docker image inspect --format '{{range .RepoDigests}}{{println .}}{{end}}' "$image_id" |
    LC_ALL=C sort |
    tr '\n' ',' |
    sed 's/,$//'
}

capture_container_identity() {
  local container_id="$1"
  docker inspect --format '{{.Id}}|{{.Name}}|{{.Image}}|{{.Config.Image}}|{{.State.Running}}|{{.RestartCount}}' "$container_id"
}

assert_app_identity() {
  local phase="$1"
  local actual_identity actual_networks
  actual_identity="$(capture_container_identity "$app_container_id")" ||
    fail "application identity inspection failed during $phase"
  [[ "$actual_identity" == "$app_identity_start" ]] ||
    fail "application container identity changed during $phase"
  actual_networks="$(capture_container_networks "$app_container_id")" ||
    fail "application network inspection failed during $phase"
  [[ "$actual_networks" == "$app_network_snapshot_start" ]] ||
    fail "application network identity changed during $phase"
}

assert_dependency_identity() {
  local role="$1"
  local container_id="$2"
  local expected_identity="$3"
  local network_name="$4"
  local expected_network_id="$5"
  local expected_ipv4="$6"
  local expected_ipv6="$7"
  local actual_identity network_member actual_network_id actual_member_id actual_name actual_ipv4 actual_ipv6

  actual_identity="$(capture_container_identity "$container_id")" ||
    fail "$role identity inspection failed"
  [[ "$actual_identity" == "$expected_identity" ]] ||
    fail "$role container identity changed"
  network_member="$(capture_network_member "$network_name" "$container_id")" ||
    fail "$role network identity inspection failed"
  IFS='|' read -r actual_network_id actual_member_id actual_name actual_ipv4 actual_ipv6 <<< "$network_member"
  [[ "$actual_network_id" == "$expected_network_id" &&
    "$actual_member_id" == "$container_id" &&
    "$actual_ipv4" == "$expected_ipv4" &&
    "$actual_ipv6" == "$expected_ipv6" ]] ||
    fail "$role network identity or IP changed"
}

assert_runtime_identities() {
  local phase="$1"
  assert_app_identity "$phase"
  assert_dependency_identity "database" "$database_container_id" "$database_identity_start" \
    "$shared_db_network" "$shared_db_network_id" "$database_ipv4" "$database_ipv6"
  assert_dependency_identity "Redis" "$redis_container_id" "$redis_identity_start" \
    "$shared_redis_network" "$shared_redis_network_id" "$redis_ipv4" "$redis_ipv6"
}

app_identity_start="$(capture_container_identity "$app_container_id")" ||
  fail "cannot capture application identity"
app_network_snapshot_start="$(capture_container_networks "$app_container_id")" ||
  fail "cannot capture application network snapshot"
[[ -n "$app_network_snapshot_start" ]] || fail "application container has no Docker networks"
app_env_json="$(docker inspect --format '{{json .Config.Env}}' "$app_container_id")" ||
  fail "cannot capture application environment"

timestamp="$(date +%Y%m%d%H%M%S)"
evidence_dir="$evidence_root/$timestamp"
evidence_file="$evidence_dir/evidence.txt"
lock_file="$evidence_root/.preflight.lock"
evidence_base="${evidence_root%/preflight}"
ensure_secure_directory "$evidence_base"
ensure_secure_directory "$evidence_root"

[[ ! -L "$lock_file" ]] || fail "lock file must not be a symbolic link"
if [[ -e "$lock_file" ]]; then
  [[ -f "$lock_file" ]] || fail "lock path exists but is not a regular file"
  [[ "$(stat -c '%u' -- "$lock_file")" == "0" ]] || fail "lock file must be root-owned"
  [[ "$(stat -c '%h' -- "$lock_file")" == "1" ]] || fail "lock file must have exactly one hard link"
fi
exec 9>>"$lock_file"
chmod 600 -- "$lock_file"
flock -n 9 || fail "another SubNexus preflight is already running"

mkdir -- "$evidence_dir" || fail "evidence directory already exists or cannot be created: $evidence_dir"
chmod 700 -- "$evidence_dir"

env_value() {
  local wanted="$1"
  local raw

  # Parse Docker's JSON array instead of line-splitting Config.Env. A newline
  # in a credential must be rejected, not silently stripped by command
  # substitution or treated as a second environment entry.
  raw="$(printf '%s' "$app_env_json" | python3 -c '
import json
import sys

wanted = sys.argv[1]
try:
    entries = json.load(sys.stdin)
except Exception as exc:
    raise SystemExit(f"invalid Docker environment JSON: {exc}")
if not isinstance(entries, list):
    raise SystemExit("Docker environment is not an array")
matches = [entry[len(wanted) + 1:] for entry in entries
           if isinstance(entry, str) and entry.startswith(wanted + "=")]
if len(matches) > 1:
    raise SystemExit(f"duplicate environment key: {wanted}")
value = matches[0] if matches else ""
if any(character in value for character in ("\r", "\n", "\x1e")):
    raise SystemExit(f"environment value contains a forbidden control character: {wanted}")
sys.stdout.write(value + "\x1e")
' "$wanted")" || fail "cannot safely read application environment key: $wanted"
  [[ -n "$raw" && "${raw: -1}" == $'\x1e' ]] || fail "environment parser returned an invalid value: $wanted"
  printf '%s' "${raw%$'\x1e'}"
}

mapfile -t app_networks < <(printf '%s\n' "$app_network_snapshot_start" | awk -F'|' 'NF && $1 != "" {print $1}')
[[ "${#app_networks[@]}" -gt 0 ]] || fail "application container has no Docker networks"

resolve_shared_container() {
  local endpoint="$1"
  local role="$2"
  local network_name network_id member_id member_name member_ipv4 member_ipv6
  local alias_network alias_name matched already_seen existing members aliases
  local -a matches=()

  for network_name in "${app_networks[@]}"; do
    network_id="$(docker network inspect --format '{{.Id}}' "$network_name")" ||
      fail "failed to inspect application Docker network '$network_name' identity"
    [[ -n "$network_id" ]] || fail "application Docker network '$network_name' has no ID"
    members="$(
      docker network inspect --format '{{range $id, $container := .Containers}}{{printf "%s|%s|%s|%s\n" $id $container.Name $container.IPv4Address $container.IPv6Address}}{{end}}' "$network_name"
    )" || fail "failed to inspect application Docker network '$network_name'"

    while IFS='|' read -r member_id member_name member_ipv4 member_ipv6; do
      [[ -n "$member_id" && -n "$member_name" ]] || continue
      member_name="${member_name#/}"
      matched=false

      if [[ "$endpoint" == "$member_name" \
        || ( -n "$member_ipv4" && "$endpoint" == "${member_ipv4%%/*}" ) \
        || ( -n "$member_ipv6" && "$endpoint" == "${member_ipv6%%/*}" ) ]]; then
        matched=true
      else
        aliases="$(
          docker inspect --format '{{range $networkName, $network := .NetworkSettings.Networks}}{{range $network.Aliases}}{{printf "%s|%s\n" $networkName .}}{{end}}{{end}}' "$member_id"
        )" || fail "failed to inspect container aliases on Docker network '$network_name'"
        while IFS='|' read -r alias_network alias_name; do
          if [[ "$alias_network" == "$network_name" && "$alias_name" == "$endpoint" ]]; then
            matched=true
            break
          fi
        done <<< "$aliases"
      fi

      if [[ "$matched" == true ]]; then
        already_seen=false
        for existing in "${matches[@]}"; do
          if [[ "${existing%%|*}" == "$member_id" ]]; then
            already_seen=true
            break
          fi
        done
        if [[ "$already_seen" == false ]]; then
          matches+=("$member_id|$member_name|$network_name|$network_id|$member_ipv4|$member_ipv6")
        fi
      fi
    done <<< "$members"
  done

  if [[ "${#matches[@]}" -eq 0 ]]; then
    fail "$role endpoint '$endpoint' does not resolve to a container on an application Docker network"
  fi
  if [[ "${#matches[@]}" -ne 1 ]]; then
    fail "$role endpoint '$endpoint' resolves to multiple containers on application Docker networks"
  fi

  printf '%s\n' "${matches[0]}"
}

capture_app_port_bindings() {
  docker inspect --format '{{range $containerPort, $bindings := .HostConfig.PortBindings}}{{if eq $containerPort "8080/tcp"}}{{range $bindings}}{{printf "%s|%s\n" .HostIp .HostPort}}{{end}}{{end}}{{end}}' "$app_container_id" |
    LC_ALL=C sort
}

valid_port() {
  local candidate="$1"
  [[ "$candidate" =~ ^[0-9]{1,5}$ ]] || return 1
  local decimal=$((10#$candidate))
  (( decimal >= 1 && decimal <= 65535 ))
}

valid_ipv4() {
  local candidate="$1"
  local -a octets=()
  local octet

  [[ "$candidate" =~ ^[0-9.]+$ ]] || return 1
  IFS='.' read -r -a octets <<< "$candidate"
  [[ "${#octets[@]}" -eq 4 ]] || return 1
  for octet in "${octets[@]}"; do
    [[ "$octet" =~ ^[0-9]{1,3}$ ]] || return 1
    (( 10#$octet <= 255 )) || return 1
  done
}

capture_ipv4_or_fail() {
  local cidr="$1"
  local role="$2"
  local address="${cidr%%/*}"
  [[ -n "$address" && "$cidr" == */* ]] ||
    fail "$role has no captured Docker IPv4 address"
  valid_ipv4 "$address" || fail "$role captured Docker IPv4 address is invalid"
  printf '%s' "$address"
}

validate_app_port_bindings() {
  local bindings="$1"
  local line host_ip host_port extra
  local eligible_count=0
  local selected_host_ip='' selected_host_port=''
  local -a binding_lines=()

  mapfile -t binding_lines < <(printf '%s\n' "$bindings" | awk 'NF')
  [[ "${#binding_lines[@]}" -gt 0 ]] || fail "no published 8080/tcp port found"
  for line in "${binding_lines[@]}"; do
    IFS='|' read -r host_ip host_port extra <<< "$line"
    [[ -z "${extra:-}" ]] || fail "malformed 8080/tcp port binding"
    valid_port "$host_port" || fail "invalid published 8080/tcp host port: $host_port"
    case "$host_ip" in
      ''|'0.0.0.0'|'127.0.0.1')
        eligible_count=$((eligible_count + 1))
        selected_host_ip="$host_ip"
        selected_host_port="$host_port"
        ;;
      *)
        fail "unsupported published 8080/tcp host IP: $host_ip"
        ;;
    esac
  done
  [[ "$eligible_count" -eq 1 ]] ||
    fail "expected exactly one IPv4 loopback/all-interface 8080/tcp binding, found $eligible_count"
  app_selected_host_ip="$selected_host_ip"
  app_port="$selected_host_port"
}

app_port_bindings="$(capture_app_port_bindings)" || fail "failed to inspect application port bindings"
validate_app_port_bindings "$app_port_bindings"
app_port_start="$app_port"
app_selected_host_ip_start="$app_selected_host_ip"
app_health_status="$(docker inspect --format '{{if .Config.Healthcheck}}{{.State.Health.Status}}{{else}}not_configured{{end}}' "$app_container_id")" ||
  fail "failed to inspect application Docker health status"
case "$app_health_status" in
  healthy|not_configured) ;;
  *) fail "application Docker health status is not ready: $app_health_status" ;;
esac
assert_app_identity "before_local_health"
http_health_check "http://127.0.0.1:${app_port}/health" local 8
assert_app_identity "after_local_health"
if [[ -n "$public_url" ]]; then
  http_health_check "$public_url" public 12
  assert_app_identity "after_public_health"
fi

database_host="$(env_value DATABASE_HOST)"
database_port="$(env_value DATABASE_PORT)"
database_port="${database_port:-5432}"
database_name="$(env_value DATABASE_DBNAME)"
database_name="${database_name:-$(env_value DATABASE_NAME)}"
database_name="${database_name:-$(env_value DATABASE_DB)}"
database_user="$(env_value DATABASE_USER)"
database_password="$(env_value DATABASE_PASSWORD)"
database_sslmode="$(env_value DATABASE_SSLMODE)"
database_sslmode="${database_sslmode:-disable}"
redis_host="$(env_value REDIS_HOST)"
redis_port="$(env_value REDIS_PORT)"
redis_port="${redis_port:-6379}"
redis_username="$(env_value REDIS_USERNAME)"
redis_password="$(env_value REDIS_PASSWORD)"
redis_db="$(env_value REDIS_DB)"
redis_db="${redis_db:-0}"
redis_enable_tls="$(env_value REDIS_ENABLE_TLS)"
redis_enable_tls="${redis_enable_tls:-false}"
[[ -n "$database_host" && -n "$database_name" && -n "$database_user" ]] ||
  fail "DATABASE_HOST, DATABASE_USER, and a database name must be present in app environment"
[[ "$database_name" =~ ^[A-Za-z][A-Za-z0-9_]{0,62}$ ]] ||
  fail "database name must be a simple PostgreSQL identifier; conninfo and URI values are not accepted"
[[ -n "$redis_host" ]] || fail "REDIS_HOST is missing from app environment"
[[ "$database_password" != *$'\n'* && "$database_password" != *$'\r'* ]] || fail "DATABASE_PASSWORD must not contain newline characters"
[[ "$redis_password" != *$'\n'* && "$redis_password" != *$'\r'* ]] || fail "REDIS_PASSWORD must not contain newline characters"
valid_port "$database_port" ||
  fail "DATABASE_PORT must be an integer between 1 and 65535"
case "$database_sslmode" in
  disable|allow|prefer|require|verify-ca|verify-full) ;;
  *) fail "DATABASE_SSLMODE is not a supported PostgreSQL sslmode" ;;
esac
valid_port "$redis_port" ||
  fail "REDIS_PORT must be an integer between 1 and 65535"
[[ "$redis_db" =~ ^[0-9]+$ ]] || fail "REDIS_DB must be a non-negative integer"
case "$redis_enable_tls" in
  true|false) ;;
  *) fail "REDIS_ENABLE_TLS must be true or false" ;;
esac

database_resolution="$(resolve_shared_container "$database_host" database)"
IFS='|' read -r database_container_id database_container shared_db_network shared_db_network_id database_ipv4 database_ipv6 <<< "$database_resolution"
redis_resolution="$(resolve_shared_container "$redis_host" redis)"
IFS='|' read -r redis_container_id redis_container shared_redis_network shared_redis_network_id redis_ipv4 redis_ipv6 <<< "$redis_resolution"

# Pin the actual probe to the captured container addresses. The configured
# Docker aliases are retained as evidence, but are not re-resolved between
# probes where a container replacement could otherwise change the target.
database_connect_host="$(capture_ipv4_or_fail "$database_ipv4" database)"
redis_connect_host="$(capture_ipv4_or_fail "$redis_ipv4" Redis)"

[[ -n "$database_container_id" && -n "$database_container" && -n "$shared_db_network" && -n "$shared_db_network_id" ]] ||
  fail "database container identity is incomplete"
[[ -n "$redis_container_id" && -n "$redis_container" && -n "$shared_redis_network" && -n "$shared_redis_network_id" ]] ||
  fail "redis container identity is incomplete"

[[ "$(docker inspect --format '{{.State.Running}}' "$database_container_id")" == "true" ]] ||
  fail "database container is not running"
[[ "$(docker inspect --format '{{.State.Running}}' "$redis_container_id")" == "true" ]] ||
  fail "redis container is not running"

capture_exec_path() {
  local container_id="$1"
  local tool_name="$2"
  local tool_path

  tool_path="$(docker exec "$container_id" sh -c 'command -v "$1"' sh "$tool_name")" ||
    fail "$tool_name is missing in container $container_id"
  [[ "$tool_path" == /* && "$tool_path" != *$'\r'* && "$tool_path" != *$'\n'* && "$tool_path" != *$'\x1e'* ]] ||
    fail "container tool path is not a safe absolute path: $tool_name"
  docker exec "$container_id" sh -c 'test -x "$1"' sh "$tool_path" ||
    fail "container tool is not executable: $tool_name"
  printf '%s' "$tool_path"
}

database_psql_path="$(capture_exec_path "$database_container_id" psql)"
redis_cli_path="$(capture_exec_path "$redis_container_id" redis-cli)"

db_psql() {
  local forward_stdin=false
  local -a pipeline_status=()
  if [[ "${1:-}" == "--stdin" ]]; then
    forward_stdin=true
    shift
  fi

  assert_runtime_identities "before_postgresql_probe"
  if {
    printf '%s\n' "$database_password"
    if [[ "$forward_stdin" == true ]]; then
      cat
    fi
  } | docker exec -i "$database_container_id" sh -c \
    'IFS= read -r password || exit 1; host="$1"; port="$2"; user="$3"; database="$4"; sslmode="$5"; psql_path="$6"; shift 6; unset PGHOSTADDR PGSERVICE PGSERVICEFILE PGPASSFILE PGSYSCONFDIR PGDATABASE PGUSER PGTARGETSESSIONATTRS PGLOADBALANCEHOSTS PGREQUIRESSL PGSSLCERT PGSSLKEY PGSSLROOTCERT PGSSLCRL PGSSLCRLDIR PGCHANNELBINDING; PGPASSFILE=/dev/null PGPASSWORD="$password" PGHOST="$host" PGPORT="$port" PGSSLMODE="$sslmode" PGCONNECT_TIMEOUT=8 PGOPTIONS="-c default_transaction_read_only=on -c statement_timeout=30s -c lock_timeout=3s" exec "$psql_path" -X -v ON_ERROR_STOP=1 -U "$user" -d "$database" "$@"' \
    sh "$database_connect_host" "$database_port" "$database_user" "$database_name" "$database_sslmode" "$database_psql_path" "$@"; then
    :
  else
    pipeline_status=("${PIPESTATUS[@]}")
    (( pipeline_status[0] == 0 )) || return "${pipeline_status[0]}"
    (( pipeline_status[1] == 0 )) || return "${pipeline_status[1]}"
    return 1
  fi
  assert_runtime_identities "after_postgresql_probe"
}

redis_cli() {
  local -a pipeline_status=()
  assert_runtime_identities "before_redis_probe"
  if printf '%s\n' "$redis_password" |
    docker exec -i "$redis_container_id" sh -c \
      'IFS= read -r password || exit 1; host="$1"; port="$2"; username="$3"; database="$4"; enable_tls="$5"; redis_path="$6"; shift 6; if [ -n "$username" ]; then set -- --user "$username" "$@"; fi; if [ "$enable_tls" = true ]; then set -- --tls "$@"; fi; unset REDISCLI_AUTH REDISCLI_HISTFILE; if [ -n "$password" ]; then REDISCLI_AUTH="$password"; export REDISCLI_AUTH; fi; exec "$redis_path" --raw -h "$host" -p "$port" -n "$database" "$@"' \
      sh "$redis_connect_host" "$redis_port" "$redis_username" "$redis_db" "$redis_enable_tls" "$redis_cli_path" "$@"; then
    :
  else
    pipeline_status=("${PIPESTATUS[@]}")
    (( pipeline_status[0] == 0 )) || return "${pipeline_status[0]}"
    (( pipeline_status[1] == 0 )) || return "${pipeline_status[1]}"
    return 1
  fi
  assert_runtime_identities "after_redis_probe"
}

IFS='|' read -r app_identity_id app_container_name app_image_id app_image app_running app_restart_count <<< "$app_identity_start"
[[ "$app_identity_id" == "$app_container_id" && -n "$app_container_name" && "$app_running" == "true" ]] ||
  fail "application identity snapshot is invalid"
container_image="$app_image"
container_image_id="$app_image_id"
container_image_repo_digests="$(
  docker image inspect --format '{{range .RepoDigests}}{{println .}}{{end}}' "$container_image_id" |
    tr '\n' ',' |
    sed 's/,$//'
)" || fail "failed to inspect application image digest"
container_image_revision="$(
  docker inspect --format '{{with index .Config.Labels "org.opencontainers.image.revision"}}{{.}}{{end}}' "$app_container_id"
)" || fail "failed to inspect application image revision"
database_identity_start="$(capture_container_identity "$database_container_id")" ||
  fail "cannot capture database identity"
redis_identity_start="$(capture_container_identity "$redis_container_id")" ||
  fail "cannot capture Redis identity"
IFS='|' read -r database_identity_id database_container_name database_image_id database_image database_running database_restart_count <<< "$database_identity_start"
IFS='|' read -r redis_identity_id redis_container_name redis_image_id redis_image redis_running redis_restart_count <<< "$redis_identity_start"
[[ "$database_identity_id" == "$database_container_id" && "$database_running" == "true" && -n "$database_container_name" ]] ||
  fail "database identity snapshot is invalid"
[[ "$redis_identity_id" == "$redis_container_id" && "$redis_running" == "true" && -n "$redis_container_name" ]] ||
  fail "Redis identity snapshot is invalid"
database_image_repo_digests="$(capture_image_repo_digests "$database_image_id")" || fail "failed to inspect database image digest"
redis_image_repo_digests="$(capture_image_repo_digests "$redis_image_id")" || fail "failed to inspect Redis image digest"
database_network_snapshot_start="$(capture_network_member "$shared_db_network" "$database_container_id")" ||
  fail "cannot capture database network identity"
redis_network_snapshot_start="$(capture_network_member "$shared_redis_network" "$redis_container_id")" ||
  fail "cannot capture Redis network identity"
assert_runtime_identities "after_identity_capture"
container_uid="$(docker exec "$app_container_id" awk '/^Uid:/{print $2}' /proc/1/status 2>/dev/null || true)"
container_no_new_privs="$(docker exec "$app_container_id" awk '/^NoNewPrivs:/{print $2}' /proc/1/status 2>/dev/null || true)"
container_restarts="$app_restart_count"

{
  printf 'PREFLIGHT_KIND=read-only\n'
  printf 'PREFLIGHT_SCRIPT_SHA256=%s\n' "$script_sha256"
  printf 'CAPTURED_AT=%s\n' "$(date --iso-8601=seconds)"
  printf 'DOCKER_CONTEXT=%s\n' "$docker_context"
  printf 'DOCKER_ENDPOINT=%s\n' "$docker_endpoint"
  printf 'DOCKER_DAEMON_NAME=%s\n' "$docker_daemon_name"
  printf 'DOCKER_SERVER_VERSION=%s\n' "$docker_server_version"
  printf 'DOCKER_ROOT_DIR=%s\n' "$docker_root_dir"
  printf 'APPLICATION_CONTAINER=%s\n' "$app_container"
  printf 'APPLICATION_IMAGE=%s\n' "$container_image"
  printf 'APPLICATION_IMAGE_ID=%s\n' "$container_image_id"
  printf 'APPLICATION_IMAGE_REPO_DIGESTS=%s\n' "${container_image_repo_digests:-'(none)'}"
  printf 'APPLICATION_IMAGE_REVISION=%s\n' "${container_image_revision:-'(none)'}"
  printf 'APPLICATION_ID=%s\n' "$app_container_id"
  printf 'APPLICATION_PORT_HOST_IP=%s\n' "$app_selected_host_ip"
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
  printf 'DATABASE_CONTAINER_ID=%s\n' "$database_container_id"
  printf 'DATABASE_IMAGE=%s\n' "$database_image"
  printf 'DATABASE_IMAGE_ID=%s\n' "$database_image_id"
  printf 'DATABASE_IMAGE_REPO_DIGESTS=%s\n' "${database_image_repo_digests:-'(none)'}"
  printf 'DATABASE_ENDPOINT=%s\n' "$database_host"
  printf 'DATABASE_CONNECT_HOST=%s\n' "$database_connect_host"
  printf 'DATABASE_PORT=%s\n' "$database_port"
  printf 'DATABASE_ENDPOINT_IPV4=%s\n' "${database_ipv4:-'(none)'}"
  printf 'DATABASE_ENDPOINT_IPV6=%s\n' "${database_ipv6:-'(none)'}"
  printf 'DATABASE_NAME=%s\n' "$database_name"
  printf 'DATABASE_SSLMODE=%s\n' "$database_sslmode"
  printf 'DATABASE_NETWORK=%s\n' "$shared_db_network"
  printf 'DATABASE_NETWORK_ID=%s\n' "$shared_db_network_id"
  printf 'REDIS_CONTAINER=%s\n' "$redis_container"
  printf 'REDIS_CONTAINER_ID=%s\n' "$redis_container_id"
  printf 'REDIS_IMAGE=%s\n' "$redis_image"
  printf 'REDIS_IMAGE_ID=%s\n' "$redis_image_id"
  printf 'REDIS_IMAGE_REPO_DIGESTS=%s\n' "${redis_image_repo_digests:-'(none)'}"
  printf 'REDIS_ENDPOINT=%s\n' "$redis_host"
  printf 'REDIS_CONNECT_HOST=%s\n' "$redis_connect_host"
  printf 'REDIS_PORT=%s\n' "$redis_port"
  printf 'REDIS_ENDPOINT_IPV4=%s\n' "${redis_ipv4:-'(none)'}"
  printf 'REDIS_ENDPOINT_IPV6=%s\n' "${redis_ipv6:-'(none)'}"
  printf 'REDIS_USERNAME_CONFIGURED=%s\n' "$([[ -n "$redis_username" ]] && printf true || printf false)"
  printf 'REDIS_DB=%s\n' "$redis_db"
  printf 'REDIS_TLS=%s\n' "$redis_enable_tls"
  printf 'REDIS_NETWORK=%s\n' "$shared_redis_network"
  printf 'REDIS_NETWORK_ID=%s\n' "$shared_redis_network_id"
  printf '\n=== APPLICATION_PORT_BINDINGS ===\n'
  while IFS='|' read -r binding_host_ip binding_host_port; do
    [[ -n "$binding_host_port" ]] || continue
    printf '%s|%s\n' "${binding_host_ip:-0.0.0.0}" "$binding_host_port"
  done <<< "$app_port_bindings"
  printf '\n=== APPLICATION_MOUNTS ===\n'
  docker inspect --format '{{range .Mounts}}{{printf "%s|%s|%s|%s\n" .Type .Source .Destination .RW}}{{end}}' "$app_container_id"
  printf '\n=== APPLICATION_NETWORKS ===\n'
  printf '%s\n' "$app_network_snapshot_start"
} > "$evidence_file"

chmod 600 "$evidence_file"

db_psql --stdin -P pager=off <<'SQL' >> "$evidence_file"
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
    db_psql --stdin -P pager=off <<'SQL' >> "$evidence_file"
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
  if [[ ",$atlas_columns," == *,version,* \
    && ",$atlas_columns," == *,description,* \
    && ",$atlas_columns," == *,type,* \
    && ",$atlas_columns," == *,applied,* \
    && ",$atlas_columns," == *,total,* \
    && ",$atlas_columns," == *,executed_at,* \
    && ",$atlas_columns," == *,hash,* ]]; then
    db_psql --stdin -P pager=off <<'SQL' >> "$evidence_file"
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
      printf 'REQUIRED_COLUMNS=version,description,type,applied,total,executed_at,hash\n'
    } >> "$evidence_file"
  fi
else
  printf '\n=== ATLAS_SCHEMA_REVISIONS ===\nABSENT\n' >> "$evidence_file"
fi

if [[ "$schema_table" != "" && "$schema_table" != "(null)" \
  && ",${schema_columns:-}," == *,filename,* \
  && ",${schema_columns:-}," == *,checksum,* \
  && ",${schema_columns:-}," == *,applied_at,* ]]; then
  db_psql --stdin -P pager=off <<'SQL' >> "$evidence_file"
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
  '189_add_group_allow_live.sql',
  '194_add_group_allow_live.sql',
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
  '226_add_usage_log_effective_model_indexes_notx.sql',
  '226_channel_monitor_quota_mode.sql',
  '247_channel_monitor_quota_mode.sql'
]::text[])
ORDER BY applied_at NULLS FIRST, filename;

SELECT table_name, column_name, data_type, udt_name, is_nullable,
       CASE
         WHEN column_default IS NULL THEN '(null)'
         ELSE 'md5=' || md5(column_default) || ';length=' || length(column_default)::text
       END AS column_default_summary
FROM information_schema.columns
WHERE table_schema = 'public'
  AND (
    (table_name = 'groups' AND column_name IN (
      'peak_rate_enabled', 'peak_start', 'peak_end', 'peak_rate_multiplier',
      'allow_image_generation', 'max_reasoning_effort', 'reasoning_effort_mappings',
      'video_model_prices', 'audio_realtime_price_per_min',
      'audio_tts_price_per_million_chars', 'audio_stt_price_per_hour',
      'search_price_per_1k', 'long_context_pricing_enabled', 'model_pricing', 'allow_live'
    ))
    OR (table_name = 'usage_logs' AND column_name IN (
      'long_context_billing_applied', 'request_type', 'upstream_model_mismatch'
    ))
    OR (table_name = 'ops_system_logs' AND column_name = 'host')
    OR (table_name = 'channel_model_pricing' AND column_name = 'time_pricing')
    OR (table_name = 'channel_monitors' AND column_name IN ('check_mode', 'account_id'))
    OR (table_name = 'channel_monitor_histories' AND column_name = 'quota')
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
       md5(pg_get_indexdef(i.indexrelid)) AS index_definition_md5,
       length(pg_get_indexdef(i.indexrelid)) AS index_definition_length
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
    'idx_usage_logs_effective_upstream_model_created',
    'idx_channel_monitors_account_id'
  ]::text[])
ORDER BY idx.relname;

SELECT c.conrelid::regclass::text AS table_name,
       c.conname,
       c.contype,
       c.convalidated,
       c.confdeltype,
       NULLIF(c.confrelid, 0)::regclass::text AS referenced_table,
       md5(pg_get_constraintdef(c.oid)) AS constraint_definition_md5,
       length(pg_get_constraintdef(c.oid)) AS constraint_definition_length,
       CASE
         WHEN c.conname IN ('channel_monitors_provider_check', 'channel_monitor_request_templates_provider_check') THEN
           'openai=' || (position('openai' IN lower(pg_get_constraintdef(c.oid))) > 0)::text
           || ';anthropic=' || (position('anthropic' IN lower(pg_get_constraintdef(c.oid))) > 0)::text
           || ';gemini=' || (position('gemini' IN lower(pg_get_constraintdef(c.oid))) > 0)::text
           || ';grok=' || (position('grok' IN lower(pg_get_constraintdef(c.oid))) > 0)::text
           || ';antigravity=' || (position('antigravity' IN lower(pg_get_constraintdef(c.oid))) > 0)::text
           || ';kimi=' || (position('kimi' IN lower(pg_get_constraintdef(c.oid))) > 0)::text
           || ';zhipu=' || (position('zhipu' IN lower(pg_get_constraintdef(c.oid))) > 0)::text
           || ';deepseek=' || (position('deepseek' IN lower(pg_get_constraintdef(c.oid))) > 0)::text
         WHEN c.conname = 'channel_monitors_check_mode_check' THEN
           'probe=' || (position('probe' IN lower(pg_get_constraintdef(c.oid))) > 0)::text
           || ';quota=' || (position('quota' IN lower(pg_get_constraintdef(c.oid))) > 0)::text
           || ';quota_probe=' || (position('quota_probe' IN lower(pg_get_constraintdef(c.oid))) > 0)::text
         ELSE '[redacted]'
       END AS known_contract_summary
FROM pg_constraint c
JOIN pg_class tbl ON tbl.oid = c.conrelid
JOIN pg_namespace ns ON ns.oid = tbl.relnamespace
WHERE ns.nspname = 'public'
  AND (
    (tbl.relname = 'ops_ingress_reject_aggregates')
     OR (tbl.relname = 'usage_logs' AND c.conname = 'usage_logs_request_type_check')
     OR (tbl.relname IN ('channel_monitors', 'channel_monitor_request_templates')
         AND c.conname IN (
           'channel_monitors_provider_check',
           'channel_monitor_request_templates_provider_check',
           'channel_monitors_check_mode_check',
           'channel_monitors_account_id_fkey'
         ))
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
       md5(pg_get_functiondef(p.oid)) AS function_definition_md5,
       length(pg_get_functiondef(p.oid)) AS function_definition_length
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
       md5(pg_get_triggerdef(t.oid)) AS trigger_definition_md5,
       length(pg_get_triggerdef(t.oid)) AS trigger_definition_length
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
    db_psql --stdin -P pager=off <<'SQL' >> "$evidence_file"
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
    db_psql --stdin -P pager=off <<'SQL' >> "$evidence_file"
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
    db_psql --stdin -P pager=off <<'SQL' >> "$evidence_file"
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
    db_psql --stdin -P pager=off <<'SQL' >> "$evidence_file"
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

db_psql --stdin -P pager=off <<'SQL' >> "$evidence_file"
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
SELECT indexname, tablename,
       md5(indexdef) AS index_definition_md5,
       length(indexdef) AS index_definition_length
FROM pg_indexes
WHERE schemaname = 'public'
  AND (indexname ILIKE '%invoice%' OR indexname ILIKE '%battle%' OR indexname ILIKE '%activity%' OR indexname ILIKE '%checkin%' OR indexname ILIKE '%leaderboard%')
ORDER BY indexname;
COMMIT;
SQL

settings_table="$(db_psql -Atc "SELECT to_regclass('public.settings')")"
settings_columns="$(db_psql -Atc "SELECT string_agg(column_name, ',' ORDER BY ordinal_position) FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'settings'")"
settings_key_type="$(db_psql -Atc "SELECT COALESCE(MAX(udt_name), '') FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'settings' AND column_name = 'key'")"
settings_value_type="$(db_psql -Atc "SELECT COALESCE(MAX(udt_name), '') FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'settings' AND column_name = 'value'")"
settings_shape_compatible=false
if [[ "$settings_table" != "" && "$settings_table" != "(null)" \
  && ",${settings_columns:-}," == *,key,* \
  && ",${settings_columns:-}," == *,value,* ]]; then
  case "$settings_key_type:$settings_value_type" in
    text:text|text:varchar|varchar:text|varchar:varchar)
      settings_shape_compatible=true
      ;;
  esac
fi

if [[ "$settings_shape_compatible" == true ]]; then
  db_psql --stdin -P pager=off <<'SQL' >> "$evidence_file"
\pset footer off
\pset null '(null)'
\echo === SETTINGS_AND_COUNTS ===
BEGIN READ ONLY;
SET LOCAL default_transaction_read_only = on;
SELECT key,
       CASE
         WHEN key IN (
           'subnexus_checkin_enabled', 'subnexus_leaderboard_enabled',
           'subnexus_activity_center_enabled', 'subnexus_marquee_enabled',
           'subnexus_first_recharge_enabled', 'subnexus_invite_rewards_enabled',
           'invoice_enabled', 'battle_pass_enabled',
           'affiliate_enabled', 'channel_monitor_enabled', 'channel_monitor_show_quota',
           'ALIPAY_MOBILE_PRECREATE_DEEP_LINK'
         ) THEN
           CASE
             WHEN lower(btrim(value)) IN ('true', 'false') THEN 'boolean=' || lower(btrim(value))
             ELSE 'invalid_boolean;value_length=' || length(value)::text
           END
         WHEN key = 'ACTIVITY_CONFIG'
           THEN 'checkin_enabled=' || COALESCE((regexp_match(value, '(?i)"checkin_enabled"[[:space:]]*:[[:space:]]*(true|false)'))[1], 'unknown')
                || ';leaderboard_enabled=' || COALESCE((regexp_match(value, '(?i)"leaderboard_enabled"[[:space:]]*:[[:space:]]*(true|false)'))[1], 'unknown')
                || ';broadcast_enabled=' || COALESCE((regexp_match(value, '(?i)"broadcast_enabled"[[:space:]]*:[[:space:]]*(true|false)'))[1], 'unknown')
                || ';first_recharge_enabled=' || COALESCE((regexp_match(value, '(?i)"first_recharge_enabled"[[:space:]]*:[[:space:]]*(true|false)'))[1], 'unknown')
                || ';value_length=' || length(value)::text
         WHEN key IN ('ACTIVITY_CENTER_CONFIG', 'INVOICE_CONFIG', 'invoice_config', 'BATTLE_PASS_CONFIG', 'battle_pass_config')
           THEN 'enabled=' || COALESCE((regexp_match(value, '(?i)"enabled"[[:space:]]*:[[:space:]]*(true|false)'))[1], 'unknown')
                || ';value_length=' || length(value)::text
         ELSE '[redacted];value_length=' || length(value)::text
       END AS value
FROM public.settings
WHERE key IN (
  'subnexus_checkin_enabled', 'subnexus_leaderboard_enabled',
  'subnexus_activity_center_enabled', 'subnexus_marquee_enabled',
  'subnexus_first_recharge_enabled', 'subnexus_invite_rewards_enabled',
  'invoice_enabled', 'battle_pass_enabled',
  'affiliate_enabled', 'channel_monitor_enabled', 'channel_monitor_show_quota',
  'ALIPAY_MOBILE_PRECREATE_DEEP_LINK',
  'ACTIVITY_CONFIG', 'ACTIVITY_CENTER_CONFIG', 'INVOICE_CONFIG', 'invoice_config',
  'BATTLE_PASS_CONFIG', 'battle_pass_config'
)
ORDER BY key;
SELECT schemaname AS schema_name, relname AS table_name, n_live_tup::bigint AS estimated_rows
FROM pg_stat_user_tables
WHERE schemaname = 'public'
  AND relname IN ('users', 'payment_orders', 'subscriptions', 'usage_logs', 'settings', 'schema_migrations', 'atlas_schema_revisions')
ORDER BY schemaname, relname;
COMMIT;
SQL
elif [[ "$settings_table" != "" && "$settings_table" != "(null)" ]]; then
  {
    printf '\n=== SETTINGS_AND_COUNTS ===\n'
    printf 'SETTINGS_TABLE_SHAPE=MISMATCH\n'
    printf 'SETTINGS_COLUMNS=%s\n' "${settings_columns:-'(none)'}"
    printf 'REQUIRED_COLUMNS=key,value\n'
    printf 'KEY_UDT=%s\n' "${settings_key_type:-'(missing)'}"
    printf 'VALUE_UDT=%s\n' "${settings_value_type:-'(missing)'}"
  } >> "$evidence_file"
else
  printf '\n=== SETTINGS_AND_COUNTS ===\nSETTINGS_TABLE=ABSENT\n' >> "$evidence_file"
fi

{
  printf '\n=== STORAGE_ENV_KEYS ===\n'
  docker inspect --format '{{range .Config.Env}}{{println .}}{{end}}' "$app_container_id" |
    awk -F= '{name=tolower($1); if (name ~ /(upload|storage|invoice|media|file).*(dir|path|root|bucket|endpoint|url)?$/ || name ~ /^(upload|storage|invoice|media|file)(_|$)/) print $1}' |
    sort -u
  printf '\n=== NGINX_RUNTIME ===\n'
  if command -v nginx >/dev/null 2>&1; then
    nginx_version="$(nginx -v 2>&1)" || fail "nginx version inspection failed"
    printf 'NGINX_PRESENT=true\n'
    printf 'NGINX_VERSION=%s\n' "$nginx_version"
    printf 'NGINX_EFFECTIVE_CONFIG=deferred_to_separate_review\n'
  else
    printf 'NGINX_PRESENT=false\n'
    printf 'NGINX_EFFECTIVE_CONFIG=deferred_to_separate_review\n'
  fi
} >> "$evidence_file"

redis_ping="$(redis_cli ping)" || fail "Redis PING failed"
[[ "$redis_ping" == "PONG" ]] || fail "Redis PING returned an unexpected response"
redis_server_info="$(redis_cli info server)" || fail "Redis server INFO failed"
redis_mode="$(printf '%s\n' "$redis_server_info" | awk -F: '/^redis_mode:/{print $2; exit}')"
[[ "$redis_mode" == "standalone" ]] || fail "Redis must run in standalone mode (reported: ${redis_mode:-unknown})"
redis_persistence="$(redis_cli info persistence)" || fail "Redis persistence inspection failed"
redis_dbsize="$(redis_cli dbsize)" || fail "Redis DBSIZE failed"
[[ "$redis_dbsize" =~ ^[0-9]+$ ]] || fail "Redis DBSIZE returned a non-integer response"
redis_keyspace="$(redis_cli info keyspace)" || fail "Redis keyspace inspection failed"
redis_selected_keyspace="$(
  printf '%s\n' "$redis_keyspace" |
    awk -F: -v selected_db="db${redis_db}" '$1 == selected_db {print; exit}'
)"

{
  printf '\n=== REDIS_RUNTIME ===\n'
  printf 'PING=%s\n' "$redis_ping"
  printf 'MODE=%s\n' "$redis_mode"
  printf '%s\n' "$redis_persistence" |
    awk -F: '/^(aof_enabled|rdb_last_save_time|rdb_last_bgsave_status):/{print}'
  printf 'DBSIZE=%s\n' "$redis_dbsize"
  printf 'KEYSPACE=%s\n' "${redis_selected_keyspace:-db${redis_db}:absent}"
} >> "$evidence_file"

assert_app_runtime_state() {
  local phase="$1"
  local current_bindings current_health

  current_bindings="$(capture_app_port_bindings)" ||
    fail "failed to inspect application port bindings during $phase"
  validate_app_port_bindings "$current_bindings"
  [[ "$app_port" == "$app_port_start" && "$app_selected_host_ip" == "$app_selected_host_ip_start" ]] ||
    fail "application port binding changed during $phase"
  current_health="$(docker inspect --format '{{if .Config.Healthcheck}}{{.State.Health.Status}}{{else}}not_configured{{end}}' "$app_container_id")" ||
    fail "failed to inspect application Docker health during $phase"
  case "$current_health" in
    healthy|not_configured) ;;
    *) fail "application Docker health status is not ready during $phase: $current_health" ;;
  esac
  http_health_check "http://127.0.0.1:${app_port_start}/health" "$phase local" 8
  assert_app_identity "$phase"
}

assert_app_runtime_state "before_finalize"
assert_runtime_identities "before_finalize"
script_sha256_final="$(sha256sum "$script_path" | awk '{print toupper($1)}')" ||
  fail "cannot re-hash preflight script"
[[ "$script_sha256_final" == "$script_sha256" ]] ||
  fail "preflight script changed while it was running"
printf '\n=== FINAL_RUNTIME_ASSERTION ===\nFINAL_RUNTIME_IDENTITIES=passed\nFINAL_SCRIPT_INTEGRITY=passed\n' >> "$evidence_file"

chmod 600 "$evidence_file"
sha256sum "$evidence_file" > "$evidence_file.sha256"
chmod 600 "$evidence_file.sha256"

printf 'READ_ONLY_PREFLIGHT_CAPTURED\n'
printf 'EVIDENCE=%s\n' "$evidence_file"
printf 'CHECKSUM=%s\n' "$evidence_file.sha256"
printf 'PREFLIGHT_DECISION=pending_local_review\n'
printf 'NO_MIGRATION_OR_DEPLOYMENT_PERFORMED=true\n'
