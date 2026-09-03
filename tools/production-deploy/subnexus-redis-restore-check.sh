#!/bin/bash
set -Eeuo pipefail

# This gate loads one approved production RDB into a disposable Redis that is
# isolated from every Docker network. It never executes a command in, or
# changes the lifecycle of, the production Redis container.

case "$-" in
  *x*) set +x ;;
esac

umask 077
unset BASH_ENV ENV CDPATH GLOBIGNORE
export PATH='/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin'

readonly approved_backup_dir='/srv/subnexus-migration/backups/20260903T073714Z'
readonly expected_manifest_sha256='9e4f0b156b9e1f5222ffababc88f5e43b65e6fdf9742e2d64abda921f6416540'
readonly expected_rdb_sha256='2776e94f65acd0fbcbbb10b71c5b59b68fa74406141ab5dfb223f35b0ecbc725'
readonly expected_check_sha256='4602e1b824ee2826174ffdb971d99129e0825bbfb24d8942c18fe748f6776f1a'
readonly expected_complete_sha256='a4bccf5c13ef5ff8311e59b34d492bc7072a55298723601a6a9c127053aee8fc'
readonly expected_total_keys='39026'
readonly evidence_root='/srv/subnexus-migration/redis-restore'
readonly approved_script_path='/srv/subnexus-migration/tools/subnexus-redis-restore-check.sh'

fail() {
  printf 'ERROR: %s\n' "$*" >&2
  exit 1
}

[[ "$EUID" -eq 0 ]] || fail 'run this script as root (for example with sudo)'
[[ "$#" -eq 1 ]] || fail 'usage: subnexus-redis-restore-check.sh PRODUCTION_REDIS_CONTAINER'
production_redis_ref="$1"
[[ "$production_redis_ref" =~ ^([A-Za-z0-9][A-Za-z0-9_.-]{0,254}|[0-9a-fA-F]{12,64})$ ]] ||
  fail 'production Redis container must be a Docker name or hexadecimal ID'

for command_name in docker timeout realpath stat sha256sum awk sed grep tr sort date mkdir chmod flock tee sleep mv sync rm rmdir; do
  unset -f "$command_name" 2>/dev/null || true
  command -v "$command_name" >/dev/null 2>&1 || fail "missing command: $command_name"
done

docker_binary="$(command -v docker)"
[[ "$docker_binary" == /* && -f "$docker_binary" && ! -L "$docker_binary" ]] || fail 'docker CLI must be a regular executable at an absolute path'
[[ "$(stat -c '%u' -- "$docker_binary")" == '0' ]] || fail 'docker CLI must be root-owned'
mode_is_docker_binary_safe="$(stat -c '%a' -- "$docker_binary")"
[[ "$mode_is_docker_binary_safe" =~ ^[0-7]{3,4}$ ]] || fail 'docker CLI mode is invalid'
(( (8#$mode_is_docker_binary_safe & 8#22) == 0 )) || fail 'docker CLI must not be group- or other-writable'
docker() {
  timeout --foreground --kill-after=5s 60s "$docker_binary" "$@"
}

docker_quick() {
  timeout --foreground --kill-after=2s 3s "$docker_binary" "$@"
}

for docker_override in DOCKER_HOST DOCKER_CONTEXT DOCKER_CONFIG DOCKER_TLS_VERIFY DOCKER_CERT_PATH DOCKER_API_VERSION; do
  [[ -z "${!docker_override:-}" ]] || fail "$docker_override must be unset for a local restore check"
done
docker_context="$(docker context show 2>/dev/null)" || fail 'cannot determine Docker context'
[[ "$docker_context" == 'default' ]] || fail "Docker context must be 'default': $docker_context"
docker_endpoint="$(docker context inspect --format '{{(index .Endpoints "docker").Host}}' default 2>/dev/null)" ||
  fail 'cannot determine default Docker endpoint'
case "$docker_endpoint" in
  unix:///var/run/docker.sock|unix:///run/docker.sock) ;;
  *) fail "default Docker endpoint must be the system Docker Unix socket: $docker_endpoint" ;;
esac
docker_socket_path="${docker_endpoint#unix://}"
[[ -S "$docker_socket_path" ]] || fail 'default Docker endpoint is not a Unix socket'
[[ "$(stat -c '%u' -- "$docker_socket_path")" == '0' ]] || fail 'Docker Unix socket must be root-owned'
docker_socket_fingerprint_start="$(stat -Lc '%d|%i|%F|%u|%g|%a' -- "$docker_socket_path")" ||
  fail 'cannot fingerprint the Docker Unix socket'
docker_security_options="$(docker info --format '{{json .SecurityOptions}}')" || fail 'cannot inspect Docker security options'
[[ "$docker_security_options" == *'name=seccomp'* ]] || fail 'Docker daemon must provide seccomp isolation'

mode_is_not_group_or_other_writable() {
  local path="$1"
  local mode

  mode="$(stat -c '%a' -- "$path")" || return 1
  [[ "$mode" =~ ^[0-7]{3,4}$ ]] || return 1
  (( (8#$mode & 8#22) == 0 ))
}

validate_secure_directory() {
  local directory="$1"

  [[ -d "$directory" && ! -L "$directory" ]] || fail "directory must be a non-symlink directory: $directory"
  [[ "$(realpath -e -- "$directory")" == "$directory" ]] ||
    fail "directory resolved outside the approved path: $directory"
  [[ "$(stat -c '%u' -- "$directory")" == '0' ]] || fail "directory must be root-owned: $directory"
  mode_is_not_group_or_other_writable "$directory" ||
    fail "directory must not be group- or other-writable: $directory"
}

ensure_secure_output_directory() {
  local directory="$1"
  local parent="${directory%/*}"

  if [[ ! -e "$directory" && ! -L "$directory" ]]; then
    validate_secure_directory "$parent"
    mkdir -- "$directory" || fail "cannot create secure directory: $directory"
    chmod 700 -- "$directory"
  fi
  validate_secure_directory "$directory"
}

validate_backup_file() {
  local path="$1"

  [[ -f "$path" && ! -L "$path" ]] || fail "backup input must be a regular non-symlink file: $path"
  [[ "$(realpath -e -- "$path")" == "$path" ]] || fail "backup input resolved outside its approved path: $path"
  [[ "$(stat -c '%u' -- "$path")" == '0' ]] || fail "backup input must be root-owned: $path"
  [[ "$(stat -c '%h' -- "$path")" == '1' ]] || fail "backup input must have exactly one hard link: $path"
  mode_is_not_group_or_other_writable "$path" || fail "backup input must not be group- or other-writable: $path"
}

hash_file() {
  sha256sum -- "$1" | awk 'NF == 2 {print tolower($1)}'
}

assert_file_hash() {
  local path="$1"
  local expected="$2"
  local actual

  actual="$(hash_file "$path")" || fail "cannot hash file: $path"
  [[ "$actual" =~ ^[0-9a-f]{64}$ ]] || fail "invalid SHA256 output for: $path"
  [[ "$actual" == "$expected" ]] || fail "SHA256 mismatch for: $path"
}

fingerprint_paths() {
  local path

  for path in "$@"; do
    printf '%s|' "$path"
    stat -c '%d|%i|%s|%Y|%Z|%u|%g|%a|%h' -- "$path"
  done
}

for trusted_directory in /srv /srv/subnexus-migration /srv/subnexus-migration/tools; do
  validate_secure_directory "$trusted_directory"
done
script_path="$(realpath -e -- "${BASH_SOURCE[0]}")" || fail 'cannot resolve restore-check script path'
[[ "$script_path" == "$approved_script_path" && ! -L "$approved_script_path" && -f "$approved_script_path" ]] ||
  fail "restore-check script must be installed at the approved non-symlink path: $approved_script_path"
[[ "$(stat -c '%u' -- "$script_path")" == '0' ]] || fail 'restore-check script must be root-owned'
[[ "$(stat -c '%h' -- "$script_path")" == '1' ]] || fail 'restore-check script must have exactly one hard link'
mode_is_not_group_or_other_writable "$script_path" || fail 'restore-check script must not be group- or other-writable'
script_hash_start="$(hash_file "$script_path")" || fail 'cannot capture initial restore-check script SHA256'
[[ "$script_hash_start" =~ ^[0-9a-f]{64}$ ]] || fail 'restore-check script SHA256 is invalid'
script_fingerprint_start="$(fingerprint_paths "$script_path")" || fail 'cannot fingerprint restore-check script'

read_report_total() {
  local report="$1"
  local -a totals=()

  mapfile -t totals < <(
    awk '{ sub(/\r$/, ""); if (NF == 4 && $1 == "[info]" && $2 ~ /^[0-9]+$/ && $3 == "keys" && $4 == "read") print $2 }' "$report"
  )
  [[ "${#totals[@]}" -eq 1 ]] || fail 'redis-check-rdb.txt must contain exactly one keys-read total'
  printf '%s' "${totals[0]}"
}

read_single_info_integer() {
  local info="$1"
  local key="$2"
  local -a values=()

  mapfile -t values < <(
    printf '%s\n' "$info" |
      tr -d '\r' |
      awk -F: -v wanted="$key" 'NF == 2 && $1 == wanted {print $2}'
  )
  [[ "${#values[@]}" -eq 1 ]] || fail "Redis INFO must contain exactly one $key field"
  [[ "${values[0]}" =~ ^[0-9]+$ ]] || fail "Redis INFO field $key must be a non-negative integer"
  printf '%s' "${values[0]}"
}

read_single_info_token() {
  local info="$1"
  local key="$2"
  local -a values=()

  mapfile -t values < <(
    printf '%s\n' "$info" |
      tr -d '\r' |
      awk -F: -v wanted="$key" 'NF == 2 && $1 == wanted {print $2}'
  )
  [[ "${#values[@]}" -eq 1 ]] || fail "Redis INFO must contain exactly one $key field"
  [[ "${values[0]}" =~ ^[A-Za-z0-9._-]+$ ]] || fail "Redis INFO field $key contains an invalid value"
  printf '%s' "${values[0]}"
}

validate_candidate_mounts() {
  local mounts="$1"
  local mount_type mount_source mount_destination mount_rw
  local restore_count=0

  while IFS='|' read -r mount_type mount_source mount_destination mount_rw; do
    [[ -n "$mount_type" ]] || continue
    case "$mount_type|$mount_source|$mount_destination|$mount_rw" in
      "bind|$rdb_file|/restore.rdb|false")
        restore_count=$((restore_count + 1))
        ;;
      tmpfs\|*\|/data\|true|tmpfs\|*\|/tmp\|true)
        ;;
      *)
        fail "disposable Redis has an unexpected mount: $mount_type|$mount_source|$mount_destination|$mount_rw"
        ;;
    esac
  done <<< "$mounts"
  [[ "$restore_count" -eq 1 ]] || fail 'disposable Redis must have exactly one approved RDB bind mount'
}

capture_candidate_isolation() {
  docker inspect --format '{{.Image}}|{{.Config.User}}|{{.HostConfig.Memory}}|{{.HostConfig.MemorySwap}}|{{.HostConfig.NanoCpus}}|{{json .HostConfig.PidsLimit}}|{{.HostConfig.NetworkMode}}|{{json .HostConfig.PortBindings}}|{{.HostConfig.PublishAllPorts}}|{{.HostConfig.ReadonlyRootfs}}|{{json .HostConfig.CapAdd}}|{{json .HostConfig.CapDrop}}|{{json .HostConfig.SecurityOpt}}|{{.HostConfig.Privileged}}|{{.HostConfig.PidMode}}|{{.HostConfig.IpcMode}}|{{.HostConfig.UTSMode}}|{{json .HostConfig.Devices}}|{{json .HostConfig.DeviceRequests}}|{{json .HostConfig.VolumesFrom}}|{{.HostConfig.RestartPolicy.Name}}|{{.HostConfig.LogConfig.Type}}|{{json .HostConfig.Tmpfs}}' "$restore_container_id"
}

capture_candidate_networks() {
  docker inspect --format '{{range $name, $network := .NetworkSettings.Networks}}{{println $name}}{{end}}' "$restore_container_id" |
    LC_ALL=C sort
}

validate_candidate_networks() {
  local networks="$1"
  local network_name

  while IFS= read -r network_name; do
    [[ -z "$network_name" || "$network_name" == 'none' ]] ||
      fail "disposable Redis is attached to an unexpected Docker network: $network_name"
  done <<< "$networks"
}

capture_published_port_bindings() {
  docker inspect --format '{{range $port, $bindings := .NetworkSettings.Ports}}{{range $bindings}}{{printf "%s|%s|%s\n" $port .HostIp .HostPort}}{{end}}{{end}}' "$restore_container_id"
}

capture_container_identity() {
  docker inspect --format '{{.Id}}|{{.Name}}|{{.Image}}|{{.Config.Image}}|{{.State.Running}}|{{.RestartCount}}|{{.State.StartedAt}}|{{.State.Pid}}' "$1"
}

production_redis_id="$(docker inspect --format '{{.Id}}' "$production_redis_ref" 2>/dev/null)" ||
  fail "production Redis container not found: $production_redis_ref"
[[ "$production_redis_id" =~ ^[0-9a-f]{64}$ ]] || fail 'production Redis container did not resolve to a full ID'
production_identity_start="$(capture_container_identity "$production_redis_id")" ||
  fail 'cannot capture production Redis identity'
IFS='|' read -r identity_id identity_name production_image_id production_image_label \
  production_running production_restart_count production_started_at production_pid <<< "$production_identity_start"
[[ "$identity_id" == "$production_redis_id" && "$production_running" == 'true' ]] ||
  fail 'production Redis container is not running'
[[ "$production_image_id" =~ ^sha256:[0-9a-f]{64}$ ]] || fail 'production Redis image is not an immutable image ID'
resolved_image_id="$(docker image inspect --format '{{.Id}}' "$production_image_id")" ||
  fail 'cannot inspect the production Redis image'
[[ "$resolved_image_id" == "$production_image_id" ]] || fail 'production Redis image ID changed during inspection'

assert_production_unchanged() {
  local phase="$1"
  local actual_identity actual_id actual_socket_fingerprint

  actual_identity="$(capture_container_identity "$production_redis_id" 2>/dev/null)" || {
    printf 'ERROR: production Redis identity inspection failed during %s\n' "$phase" >&2
    return 1
  }
  actual_id="$(docker inspect --format '{{.Id}}' "$production_redis_ref" 2>/dev/null)" || {
    printf 'ERROR: production Redis reference no longer resolves during %s\n' "$phase" >&2
    return 1
  }
  if [[ "$actual_identity" != "$production_identity_start" || "$actual_id" != "$production_redis_id" ]]; then
    printf 'ERROR: production Redis identity changed during %s\n' "$phase" >&2
    return 1
  fi
  actual_socket_fingerprint="$(stat -Lc '%d|%i|%F|%u|%g|%a' -- "$docker_socket_path" 2>/dev/null)" || {
    printf 'ERROR: Docker Unix socket inspection failed during %s\n' "$phase" >&2
    return 1
  }
  if [[ "$actual_socket_fingerprint" != "$docker_socket_fingerprint_start" ]]; then
    printf 'ERROR: Docker Unix socket identity changed during %s\n' "$phase" >&2
    return 1
  fi
}

for trusted_directory in /srv/subnexus-migration/backups "$approved_backup_dir"; do
  validate_secure_directory "$trusted_directory"
done
backup_dir="$(realpath -e -- "$approved_backup_dir")" || fail 'approved backup directory is missing'
[[ "$backup_dir" == "$approved_backup_dir" && ! -L "$approved_backup_dir" ]] ||
  fail 'approved backup directory must be the canonical non-symlink path'
[[ "$(stat -c '%u' -- "$backup_dir")" == '0' ]] || fail 'approved backup directory must be root-owned'

manifest_file="$backup_dir/SHA256SUMS"
rdb_file="$backup_dir/redis-dump.rdb"
check_file="$backup_dir/redis-check-rdb.txt"
complete_file="$backup_dir/COMPLETE"
for backup_file in "$manifest_file" "$rdb_file" "$check_file" "$complete_file"; do
  validate_backup_file "$backup_file"
done

assert_file_hash "$manifest_file" "$expected_manifest_sha256"
assert_file_hash "$rdb_file" "$expected_rdb_sha256"
assert_file_hash "$check_file" "$expected_check_sha256"
assert_file_hash "$complete_file" "$expected_complete_sha256"

mapfile -t manifest_rdb_hashes < <(
  awk -v wanted="$rdb_file" 'NF == 2 && $2 == wanted {print tolower($1)}' "$manifest_file"
)
[[ "${#manifest_rdb_hashes[@]}" -eq 1 ]] || fail 'SHA256SUMS must contain exactly one approved RDB record'
[[ "${manifest_rdb_hashes[0]}" == "$expected_rdb_sha256" ]] || fail 'SHA256SUMS RDB record is not the approved digest'
reported_total_keys="$(read_report_total "$check_file")"
[[ "$reported_total_keys" == "$expected_total_keys" ]] || fail 'redis-check-rdb.txt keys-read total is not the approved value'

backup_fingerprint_start="$(
  fingerprint_paths "$backup_dir" "$manifest_file" "$rdb_file" "$check_file" "$complete_file"
)" || fail 'cannot fingerprint approved backup inputs'

ensure_secure_output_directory "$evidence_root"
lock_file="$evidence_root/.restore-check.lock"
[[ ! -L "$lock_file" ]] || fail 'restore-check lock must not be a symbolic link'
if [[ -e "$lock_file" ]]; then
  [[ -f "$lock_file" ]] || fail 'restore-check lock path must be a regular file'
  [[ "$(stat -c '%u' -- "$lock_file")" == '0' ]] || fail 'restore-check lock must be root-owned'
  [[ "$(stat -c '%h' -- "$lock_file")" == '1' ]] || fail 'restore-check lock must have exactly one hard link'
fi
exec 9>>"$lock_file"
chmod 600 -- "$lock_file"
flock -n 9 || fail 'another SubNexus Redis restore check is already running'

run_timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
IFS= read -r run_uuid </proc/sys/kernel/random/uuid || fail 'cannot obtain a kernel-generated run UUID'
[[ "$run_uuid" =~ ^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$ ]] ||
  fail 'kernel-generated run UUID is invalid'
run_token="${run_timestamp}-$$-$run_uuid"
restore_container_name="subnexus-redis-restore-check-${run_token,,}"
restore_container_id=''
restore_create_attempted=false
create_cidfile="$evidence_root/.$run_token.cid"
[[ ! -e "$create_cidfile" && ! -L "$create_cidfile" ]] || fail 'disposable Redis CID file path already exists'
evidence_stage_dir=''
declare -a restore_volume_names=()

verify_inputs_unchanged() {
  local actual_backup_fingerprint actual_script_fingerprint actual_hash path expected
  local verification_failed=0

  for path in "$script_path" "$manifest_file" "$rdb_file" "$check_file" "$complete_file"; do
    if [[ ! -f "$path" || -L "$path" || "$(stat -c '%u' -- "$path" 2>/dev/null)" != '0' ]] ||
      ! mode_is_not_group_or_other_writable "$path"; then
      printf 'ERROR: approved input type, owner, or mode changed: %s\n' "$path" >&2
      verification_failed=1
    fi
  done
  for path in /srv /srv/subnexus-migration /srv/subnexus-migration/tools \
    /srv/subnexus-migration/backups "$backup_dir"; do
    if [[ ! -d "$path" || -L "$path" || "$(stat -c '%u' -- "$path" 2>/dev/null)" != '0' ]] ||
      ! mode_is_not_group_or_other_writable "$path"; then
      printf 'ERROR: approved parent directory type, owner, or mode changed: %s\n' "$path" >&2
      verification_failed=1
    fi
  done

  actual_backup_fingerprint="$(
    fingerprint_paths "$backup_dir" "$manifest_file" "$rdb_file" "$check_file" "$complete_file" 2>/dev/null
  )" || verification_failed=1
  actual_script_fingerprint="$(fingerprint_paths "$script_path" 2>/dev/null)" || verification_failed=1
  if [[ "$actual_backup_fingerprint" != "$backup_fingerprint_start" ||
    "$actual_script_fingerprint" != "$script_fingerprint_start" ]]; then
    printf 'ERROR: approved script or backup input identity changed during isolated restore check\n' >&2
    verification_failed=1
  fi

  while IFS='|' read -r path expected; do
    actual_hash="$(hash_file "$path" 2>/dev/null)" || {
      printf 'ERROR: cannot hash approved input during final verification: %s\n' "$path" >&2
      verification_failed=1
      continue
    }
    if [[ "$actual_hash" != "$expected" ]]; then
      printf 'ERROR: approved input changed during isolated restore check: %s\n' "$path" >&2
      verification_failed=1
    fi
  done <<EOF
$script_path|$script_hash_start
$manifest_file|$expected_manifest_sha256
$rdb_file|$expected_rdb_sha256
$check_file|$expected_check_sha256
$complete_file|$expected_complete_sha256
EOF
  return "$verification_failed"
}

container_id_if_present() {
  local reference="$1"
  local resolved_id container_listing listed_id listed_name

  if resolved_id="$(docker container inspect --format '{{.Id}}' "$reference" 2>/dev/null)"; then
    [[ "$resolved_id" =~ ^[0-9a-f]{64}$ ]] || return 2
    printf '%s' "$resolved_id"
    return 0
  fi
  container_listing="$(docker container ls --all --no-trunc --format '{{.ID}}|{{.Names}}' 2>/dev/null)" || return 2
  while IFS='|' read -r listed_id listed_name; do
    [[ -n "$listed_id" ]] || continue
    if [[ "$reference" == "$listed_id" || "$reference" == "$listed_name" ]]; then
      return 2
    fi
  done <<< "$container_listing"
  return 1
}

volume_exists() {
  local volume_name="$1"
  local volume_listing listed_volume

  if docker volume inspect "$volume_name" >/dev/null 2>&1; then
    return 0
  fi
  volume_listing="$(docker volume ls --format '{{.Name}}' 2>/dev/null)" || return 2
  while IFS= read -r listed_volume; do
    [[ "$listed_volume" == "$volume_name" ]] && return 2
  done <<< "$volume_listing"
  return 1
}

cleanup_restore_container() {
  local cleanup_failed=0
  local purpose_label token_label volume_name cleanup_id current_id lookup_status lookup_attempt lookup_limit cid_id
  local -a cid_lines=()

  cleanup_id="$restore_container_id"
  if [[ -e "$create_cidfile" || -L "$create_cidfile" ]]; then
    if [[ -f "$create_cidfile" && ! -L "$create_cidfile" &&
      "$(stat -c '%u' -- "$create_cidfile" 2>/dev/null)" == '0' &&
      "$(stat -c '%h' -- "$create_cidfile" 2>/dev/null)" == '1' ]] &&
      mode_is_not_group_or_other_writable "$create_cidfile"; then
      mapfile -t cid_lines <"$create_cidfile" || cleanup_failed=1
      if [[ "${#cid_lines[@]}" -eq 1 && "${cid_lines[0]}" =~ ^[0-9a-f]{64}$ ]]; then
        cid_id="${cid_lines[0]}"
        if [[ -n "$cleanup_id" && "$cleanup_id" != "$cid_id" ]]; then
          printf 'ERROR: disposable Redis CID file disagrees with the captured container ID\n' >&2
          cleanup_failed=1
        elif [[ -z "$cleanup_id" ]]; then
          cleanup_id="$cid_id"
          restore_container_id="$cleanup_id"
        fi
      else
        printf 'ERROR: disposable Redis CID file is malformed\n' >&2
        cleanup_failed=1
      fi
    else
      printf 'ERROR: disposable Redis CID file type, owner, or mode is unsafe\n' >&2
      cleanup_failed=1
    fi
  fi
  if [[ -z "$cleanup_id" ]]; then
    lookup_limit=1
    [[ "$restore_create_attempted" == true ]] && lookup_limit=10
    lookup_status=1
    for ((lookup_attempt = 1; lookup_attempt <= lookup_limit; lookup_attempt++)); do
      if current_id="$(container_id_if_present "$restore_container_name")"; then
        cleanup_id="$current_id"
        restore_container_id="$current_id"
        lookup_status=0
        break
      else
        lookup_status="$?"
      fi
      [[ "$lookup_attempt" -eq "$lookup_limit" ]] || sleep 0.5
    done
    if [[ "$lookup_status" -eq 2 ]]; then
      printf 'ERROR: cannot determine whether a disposable Redis container was created\n' >&2
      cleanup_failed=1
    fi
  fi

  if [[ -n "$cleanup_id" ]]; then
    if current_id="$(container_id_if_present "$cleanup_id")"; then
      if [[ "$current_id" != "$cleanup_id" || "$cleanup_id" == "$production_redis_id" ]]; then
        printf 'ERROR: refusing to remove a container whose immutable identity does not match this run\n' >&2
        cleanup_failed=1
      elif ! purpose_label="$(docker inspect --format '{{index .Config.Labels "com.subnexus.purpose"}}' "$cleanup_id" 2>/dev/null)" ||
        ! token_label="$(docker inspect --format '{{index .Config.Labels "com.subnexus.run-token"}}' "$cleanup_id" 2>/dev/null)"; then
        printf 'ERROR: cannot verify disposable Redis cleanup labels\n' >&2
        cleanup_failed=1
      elif [[ "$purpose_label" != 'redis-restore-check' || "$token_label" != "$run_token" ]]; then
        printf 'ERROR: refusing to remove a container whose labels do not match this run\n' >&2
        cleanup_failed=1
      elif ! docker rm --force --volumes "$cleanup_id" >/dev/null; then
        printf 'ERROR: failed to remove disposable Redis container %s\n' "$cleanup_id" >&2
        cleanup_failed=1
      fi
    else
      lookup_status="$?"
      if [[ "$lookup_status" -eq 2 ]]; then
        printf 'ERROR: cannot verify disposable Redis container state during cleanup\n' >&2
        cleanup_failed=1
      fi
    fi
  fi

  if [[ -n "$cleanup_id" ]]; then
    if current_id="$(container_id_if_present "$cleanup_id")"; then
      printf 'ERROR: disposable Redis container still exists after cleanup: %s\n' "$current_id" >&2
      cleanup_failed=1
    else
      lookup_status="$?"
      if [[ "$lookup_status" -eq 2 ]]; then
        printf 'ERROR: cannot prove disposable Redis container removal\n' >&2
        cleanup_failed=1
      fi
    fi
  fi

  for volume_name in "${restore_volume_names[@]}"; do
    if volume_exists "$volume_name"; then
      printf 'ERROR: disposable volume still exists after exact container cleanup; refusing broad volume deletion: %s\n' "$volume_name" >&2
      cleanup_failed=1
    else
      lookup_status="$?"
      if [[ "$lookup_status" -eq 2 ]]; then
        printf 'ERROR: cannot inspect disposable anonymous volume: %s\n' "$volume_name" >&2
        cleanup_failed=1
      fi
    fi
  done

  if current_id="$(container_id_if_present "$restore_container_name")"; then
    if [[ "$current_id" == "$cleanup_id" ]]; then
      printf 'ERROR: disposable Redis container name still resolves to the container from this run\n' >&2
    else
      printf 'ERROR: disposable Redis container name was reused by another container; it was not removed\n' >&2
    fi
    cleanup_failed=1
  else
    lookup_status="$?"
    if [[ "$lookup_status" -eq 2 ]]; then
      printf 'ERROR: cannot prove disposable Redis container name removal\n' >&2
      cleanup_failed=1
    fi
  fi

  if [[ -e "$create_cidfile" || -L "$create_cidfile" ]]; then
    if [[ -n "$cleanup_id" && -f "$create_cidfile" && ! -L "$create_cidfile" &&
      "$(stat -c '%u' -- "$create_cidfile" 2>/dev/null)" == '0' &&
      "$(stat -c '%h' -- "$create_cidfile" 2>/dev/null)" == '1' &&
      "${#cid_lines[@]}" -eq 1 && "${cid_lines[0]}" == "$cleanup_id" ]] &&
      mode_is_not_group_or_other_writable "$create_cidfile"; then
      rm -- "$create_cidfile" || cleanup_failed=1
    else
      printf 'ERROR: refusing to remove an unverified disposable Redis CID file\n' >&2
      cleanup_failed=1
    fi
  fi
  if [[ -e "$create_cidfile" || -L "$create_cidfile" ]]; then
    printf 'ERROR: disposable Redis CID file still exists after cleanup\n' >&2
    cleanup_failed=1
  fi
  return "$cleanup_failed"
}

cleanup_evidence_stage() {
  local staged_path

  [[ -n "$evidence_stage_dir" ]] || return 0
  if [[ ! -e "$evidence_stage_dir" && ! -L "$evidence_stage_dir" ]]; then
    return 0
  fi
  if [[ "$evidence_stage_dir" != "$evidence_root/.$run_token.tmp" ||
    ! -d "$evidence_stage_dir" || -L "$evidence_stage_dir" ||
    "$(stat -c '%u' -- "$evidence_stage_dir" 2>/dev/null)" != '0' ]]; then
    printf 'ERROR: refusing to remove an unsafe evidence staging directory\n' >&2
    return 1
  fi
  for staged_path in "$evidence_stage_dir/evidence.txt" "$evidence_stage_dir/evidence.txt.sha256"; do
    if [[ -e "$staged_path" || -L "$staged_path" ]]; then
      if [[ ! -f "$staged_path" || -L "$staged_path" ||
        "$(stat -c '%u' -- "$staged_path" 2>/dev/null)" != '0' ||
        "$(stat -c '%h' -- "$staged_path" 2>/dev/null)" != '1' ]]; then
        printf 'ERROR: refusing to remove an unsafe staged evidence file: %s\n' "$staged_path" >&2
        return 1
      fi
      rm -- "$staged_path" || return 1
    fi
  done
  rmdir -- "$evidence_stage_dir" || {
    printf 'ERROR: evidence staging directory contains unexpected entries or cannot be removed\n' >&2
    return 1
  }
}

on_exit() {
  local original_status="$?"
  local final_status="$original_status"

  trap - EXIT
  trap '' INT TERM HUP
  cleanup_restore_container || final_status=1
  cleanup_evidence_stage || final_status=1
  assert_production_unchanged 'exit_cleanup' || final_status=1
  verify_inputs_unchanged || final_status=1
  exit "$final_status"
}
trap on_exit EXIT
trap 'exit 130' INT
trap 'exit 143' TERM HUP

if existing_container_id="$(container_id_if_present "$restore_container_name")"; then
  fail "refusing to replace an existing container: $restore_container_name ($existing_container_id)"
else
  existing_lookup_status="$?"
  [[ "$existing_lookup_status" -eq 1 ]] || fail 'cannot prove disposable container name is unused'
fi
assert_production_unchanged 'before_candidate_create'

restore_create_attempted=true
trap '' INT TERM HUP
if restore_create_output="$(
  docker create \
    --cidfile "$create_cidfile" \
    --name "$restore_container_name" \
    --label 'com.subnexus.purpose=redis-restore-check' \
    --label "com.subnexus.run-token=$run_token" \
    --network none \
    --publish-all=false \
    --ipc private \
    --restart no \
    --read-only \
    --cap-drop ALL \
    --security-opt no-new-privileges \
    --pids-limit 64 \
    --memory 256m \
    --memory-swap 256m \
    --cpus 1 \
    --user 0:0 \
    --log-driver none \
    --mount "type=bind,src=$rdb_file,dst=/restore.rdb,readonly" \
    --tmpfs '/tmp:rw,nosuid,nodev,noexec,size=16m' \
    --tmpfs '/data:rw,nosuid,nodev,noexec,size=64m' \
    --entrypoint /bin/sh \
    "$production_image_id" \
    -eu -c 'cp /restore.rdb /data/dump.rdb; exec redis-server --dir /data --dbfilename dump.rdb --save "" --appendonly no --bind 127.0.0.1 --protected-mode yes --port 6379 --daemonize no --pidfile "" --logfile ""'
)"; then
  restore_create_status=0
else
  restore_create_status="$?"
fi
trap 'exit 130' INT
trap 'exit 143' TERM HUP
[[ "$restore_create_status" -eq 0 ]] || fail 'failed to create disposable Redis container'
[[ "$restore_create_output" =~ ^[0-9a-f]{64}$ ]] || fail 'Docker did not return a full disposable container ID'
restore_container_id="$restore_create_output"
mapfile -t create_cid_lines <"$create_cidfile" || fail 'cannot read disposable Redis CID file'
[[ "${#create_cid_lines[@]}" -eq 1 && "${create_cid_lines[0]}" == "$restore_container_id" ]] ||
  fail 'disposable Redis CID file does not match the captured full ID'
[[ "$restore_container_id" != "$production_redis_id" ]] || fail 'disposable and production Redis container IDs unexpectedly match'
[[ "$(docker inspect --format '{{.Id}}' "$restore_container_name")" == "$restore_container_id" ]] ||
  fail 'disposable Redis name does not resolve to its captured full ID'

restore_volume_output="$(
  docker inspect --format '{{range .Mounts}}{{if eq .Type "volume"}}{{println .Name}}{{end}}{{end}}' "$restore_container_id"
)" || fail 'cannot inspect disposable Redis volumes'
while IFS= read -r volume_name; do
  [[ -n "$volume_name" ]] && restore_volume_names+=("$volume_name")
done <<< "$restore_volume_output"
candidate_image_id="$(docker inspect --format '{{.Image}}' "$restore_container_id")"
candidate_user="$(docker inspect --format '{{.Config.User}}' "$restore_container_id")"
candidate_memory="$(docker inspect --format '{{.HostConfig.Memory}}' "$restore_container_id")"
candidate_memory_swap="$(docker inspect --format '{{.HostConfig.MemorySwap}}' "$restore_container_id")"
candidate_nano_cpus="$(docker inspect --format '{{.HostConfig.NanoCpus}}' "$restore_container_id")"
candidate_pids_limit="$(docker inspect --format '{{json .HostConfig.PidsLimit}}' "$restore_container_id")"
candidate_network_mode="$(docker inspect --format '{{.HostConfig.NetworkMode}}' "$restore_container_id")"
candidate_port_bindings="$(docker inspect --format '{{json .HostConfig.PortBindings}}' "$restore_container_id")"
candidate_publish_all_ports="$(docker inspect --format '{{.HostConfig.PublishAllPorts}}' "$restore_container_id")"
candidate_readonly_rootfs="$(docker inspect --format '{{.HostConfig.ReadonlyRootfs}}' "$restore_container_id")"
candidate_cap_add="$(docker inspect --format '{{json .HostConfig.CapAdd}}' "$restore_container_id")"
candidate_cap_drop="$(docker inspect --format '{{json .HostConfig.CapDrop}}' "$restore_container_id")"
candidate_security_opt="$(docker inspect --format '{{json .HostConfig.SecurityOpt}}' "$restore_container_id")"
candidate_privileged="$(docker inspect --format '{{.HostConfig.Privileged}}' "$restore_container_id")"
candidate_pid_mode="$(docker inspect --format '{{.HostConfig.PidMode}}' "$restore_container_id")"
candidate_ipc_mode="$(docker inspect --format '{{.HostConfig.IpcMode}}' "$restore_container_id")"
candidate_uts_mode="$(docker inspect --format '{{.HostConfig.UTSMode}}' "$restore_container_id")"
candidate_restart_policy="$(docker inspect --format '{{.HostConfig.RestartPolicy.Name}}' "$restore_container_id")"
candidate_log_driver="$(docker inspect --format '{{.HostConfig.LogConfig.Type}}' "$restore_container_id")"
candidate_devices="$(docker inspect --format '{{json .HostConfig.Devices}}' "$restore_container_id")"
candidate_device_requests="$(docker inspect --format '{{json .HostConfig.DeviceRequests}}' "$restore_container_id")"
candidate_volumes_from="$(docker inspect --format '{{json .HostConfig.VolumesFrom}}' "$restore_container_id")"
candidate_data_tmpfs="$(docker inspect --format '{{with index .HostConfig.Tmpfs "/data"}}{{.}}{{end}}' "$restore_container_id")"
candidate_tmp_tmpfs="$(docker inspect --format '{{with index .HostConfig.Tmpfs "/tmp"}}{{.}}{{end}}' "$restore_container_id")"
restore_mount="$(
  docker inspect --format '{{range .Mounts}}{{if eq .Destination "/restore.rdb"}}{{printf "%s|%s|%s|%t" .Type .Source .Destination .RW}}{{end}}{{end}}' "$restore_container_id"
)"
candidate_mounts="$(
  docker inspect --format '{{range .Mounts}}{{printf "%s|%s|%s|%t\n" .Type .Source .Destination .RW}}{{end}}' "$restore_container_id"
)"
candidate_networks_created="$(capture_candidate_networks)" || fail 'cannot inspect disposable Redis Docker networks'
candidate_published_ports_created="$(capture_published_port_bindings)" ||
  fail 'cannot inspect disposable Redis published ports'
candidate_isolation_start="$(capture_candidate_isolation)" || fail 'cannot capture disposable Redis isolation settings'

[[ "$candidate_image_id" == "$production_image_id" ]] || fail 'disposable Redis does not use the production image ID'
[[ "$candidate_user" == '0:0' ]] || fail 'disposable Redis does not use the approved container user'
[[ "$candidate_memory" == '268435456' && "$candidate_memory_swap" == '268435456' ]] ||
  fail 'disposable Redis memory limit is not exactly 256 MiB without additional swap'
[[ "$candidate_nano_cpus" == '1000000000' ]] || fail 'disposable Redis CPU limit is not exactly one CPU'
[[ "$candidate_pids_limit" == '64' ]] || fail 'disposable Redis PID limit is not exactly 64'
[[ "$candidate_network_mode" == 'none' ]] || fail 'disposable Redis Docker network mode is not none'
[[ "$candidate_port_bindings" == '{}' || "$candidate_port_bindings" == 'null' ]] ||
  fail 'disposable Redis unexpectedly has published ports'
[[ "$candidate_publish_all_ports" == 'false' ]] || fail 'disposable Redis publish-all-ports is enabled'
[[ "$candidate_readonly_rootfs" == 'true' ]] || fail 'disposable Redis root filesystem is not read-only'
[[ "$candidate_cap_add" == '[]' || "$candidate_cap_add" == 'null' ]] ||
  fail 'disposable Redis unexpectedly adds Linux capabilities'
[[ "$candidate_cap_drop" == '["ALL"]' ]] || fail 'disposable Redis does not drop exactly all Linux capabilities'
[[ "$candidate_security_opt" == '["no-new-privileges"]' ||
  "$candidate_security_opt" == '["no-new-privileges:true"]' ]] ||
  fail 'disposable Redis security options are not the approved no-new-privileges setting'
[[ "$candidate_privileged" == 'false' ]] || fail 'disposable Redis is unexpectedly privileged'
[[ -z "$candidate_pid_mode" ]] || fail 'disposable Redis unexpectedly shares a PID namespace'
[[ "$candidate_ipc_mode" == 'private' ]] || fail 'disposable Redis IPC namespace is not private'
[[ -z "$candidate_uts_mode" ]] || fail 'disposable Redis unexpectedly shares a UTS namespace'
[[ -z "$candidate_restart_policy" || "$candidate_restart_policy" == 'no' ]] ||
  fail 'disposable Redis restart policy is not disabled'
[[ "$candidate_log_driver" == 'none' ]] || fail 'disposable Redis logging is not disabled'
[[ "$candidate_devices" == '[]' || "$candidate_devices" == 'null' ]] ||
  fail 'disposable Redis unexpectedly has host devices'
[[ "$candidate_device_requests" == '[]' || "$candidate_device_requests" == 'null' ]] ||
  fail 'disposable Redis unexpectedly has device requests'
[[ "$candidate_volumes_from" == '[]' || "$candidate_volumes_from" == 'null' ]] ||
  fail 'disposable Redis unexpectedly inherits volumes from another container'
[[ "$candidate_data_tmpfs" == *'rw'* && "$candidate_data_tmpfs" == *'size=64m'* ]] ||
  fail 'disposable Redis /data tmpfs is missing or unsafe'
[[ "$candidate_tmp_tmpfs" == *'rw'* && "$candidate_tmp_tmpfs" == *'size=16m'* ]] ||
  fail 'disposable Redis /tmp tmpfs is missing or unsafe'
[[ "$restore_mount" == "bind|$rdb_file|/restore.rdb|false" ]] ||
  fail 'approved RDB is not mounted read-only at /restore.rdb'
validate_candidate_mounts "$candidate_mounts"
validate_candidate_networks "$candidate_networks_created"
[[ -z "$candidate_published_ports_created" ]] || fail 'disposable Redis has an actual host port binding'
[[ "${#restore_volume_names[@]}" -eq 0 ]] || fail 'disposable Redis unexpectedly created an anonymous volume'

assert_production_unchanged 'before_candidate_start'
docker start "$restore_container_id" >/dev/null || fail 'failed to start disposable Redis container'
candidate_running_image="$(docker inspect --format '{{.Image}}' "$restore_container_id")" ||
  fail 'cannot inspect running disposable Redis image'
candidate_running_network="$(docker inspect --format '{{.HostConfig.NetworkMode}}' "$restore_container_id")" ||
  fail 'cannot inspect running disposable Redis network mode'
candidate_running_ports="$(docker inspect --format '{{json .HostConfig.PortBindings}}' "$restore_container_id")" ||
  fail 'cannot inspect running disposable Redis port bindings'
candidate_running_mounts="$(
  docker inspect --format '{{range .Mounts}}{{printf "%s|%s|%s|%t\n" .Type .Source .Destination .RW}}{{end}}' "$restore_container_id"
)" || fail 'cannot inspect running disposable Redis mounts'
candidate_running_networks="$(capture_candidate_networks)" || fail 'cannot inspect running disposable Redis networks'
candidate_published_ports_running="$(capture_published_port_bindings)" ||
  fail 'cannot inspect running disposable Redis published ports'
candidate_isolation_running="$(capture_candidate_isolation)" || fail 'cannot recapture running Redis isolation settings'
[[ "$candidate_running_image" == "$production_image_id" ]] || fail 'running disposable Redis image ID changed'
[[ "$candidate_running_network" == 'none' ]] || fail 'running disposable Redis is not network-isolated'
[[ "$candidate_running_ports" == '{}' || "$candidate_running_ports" == 'null' ]] ||
  fail 'running disposable Redis unexpectedly has published ports'
validate_candidate_mounts "$candidate_running_mounts"
validate_candidate_networks "$candidate_running_networks"
[[ -z "$candidate_published_ports_running" ]] || fail 'running disposable Redis has an actual host port binding'
[[ "$candidate_isolation_running" == "$candidate_isolation_start" ]] ||
  fail 'disposable Redis isolation settings changed after startup'

redis_cli() {
  docker_quick exec "$restore_container_id" /bin/sh -c \
    'unset REDISCLI_AUTH; exec redis-cli --raw -h 127.0.0.1 -p 6379 "$@"' redis-cli "$@"
}

ping_result=''
for ((attempt = 1; attempt <= 15; attempt++)); do
  if ping_result="$(redis_cli PING 2>/dev/null)"; then
    ping_result="$(printf '%s' "$ping_result" | tr -d '\r')"
    [[ "$ping_result" == 'PONG' ]] && break
  fi
  if [[ "$(docker_quick inspect --format '{{.State.Running}}' "$restore_container_id" 2>/dev/null || true)" != 'true' ]]; then
    break
  fi
  sleep 0.5
done
[[ "$ping_result" == 'PONG' ]] || {
  docker inspect --format 'candidate running={{.State.Running}} exit={{.State.ExitCode}} error={{json .State.Error}}' \
    "$restore_container_id" >&2 || true
  fail 'disposable Redis did not become ready with PING=PONG'
}

candidate_rdb_hash_line="$(docker_quick exec "$restore_container_id" sha256sum /data/dump.rdb)" ||
  fail 'cannot hash the staged RDB inside disposable Redis'
candidate_rdb_hash="$(printf '%s\n' "$candidate_rdb_hash_line" | awk 'NF == 2 {print tolower($1)}')"
[[ "$candidate_rdb_hash" == "$expected_rdb_sha256" ]] ||
  fail 'staged RDB inside disposable Redis is not the approved byte sequence'

server_info="$(redis_cli INFO server)" || fail 'cannot read disposable Redis server info'
redis_version="$(read_single_info_token "$server_info" redis_version)"
[[ "$redis_version" == '8.8.0' ]] || fail "disposable Redis version is $redis_version, expected 8.8.0"
persistence_info="$(redis_cli INFO persistence)" || fail 'cannot read disposable Redis persistence info'
loading="$(read_single_info_integer "$persistence_info" loading)"
loaded_keys="$(read_single_info_integer "$persistence_info" rdb_last_load_keys_loaded)"
expired_keys="$(read_single_info_integer "$persistence_info" rdb_last_load_keys_expired)"
[[ "$loading" == '0' ]] || fail 'disposable Redis is still loading'
actual_total_keys=$((10#$loaded_keys + 10#$expired_keys))
[[ "$actual_total_keys" -eq "$expected_total_keys" ]] ||
  fail "RDB loaded+expired key total is $actual_total_keys, expected $expected_total_keys"

dbsize="$(redis_cli DBSIZE | tr -d '\r')" || fail 'cannot read disposable Redis DBSIZE'
[[ "$dbsize" =~ ^[0-9]+$ ]] || fail 'disposable Redis DBSIZE is not a non-negative integer'
(( 10#$dbsize > 0 )) || fail 'disposable Redis restored an empty database'
(( 10#$dbsize <= 10#$loaded_keys )) || fail 'disposable Redis DBSIZE exceeds the RDB loaded-key count'

assert_production_unchanged 'after_candidate_probe'
verify_inputs_unchanged

redis_cli SHUTDOWN NOSAVE >/dev/null 2>&1 || true
candidate_exit_code="$(docker wait "$restore_container_id")" || fail 'failed waiting for disposable Redis shutdown'
[[ "$candidate_exit_code" == '0' ]] || fail "disposable Redis exited with status $candidate_exit_code"
cleanup_restore_container || fail 'disposable Redis cleanup failed'
assert_production_unchanged 'after_candidate_cleanup'
verify_inputs_unchanged

assert_production_unchanged 'before_evidence_stage'
verify_inputs_unchanged
evidence_timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
evidence_dir="$evidence_root/$run_token"
evidence_stage_dir="$evidence_root/.$run_token.tmp"
[[ ! -e "$evidence_dir" && ! -L "$evidence_dir" ]] || fail "evidence directory already exists: $evidence_dir"
[[ ! -e "$evidence_stage_dir" && ! -L "$evidence_stage_dir" ]] ||
  fail "evidence staging directory already exists: $evidence_stage_dir"
mkdir -- "$evidence_stage_dir"
chmod 700 -- "$evidence_stage_dir"
staged_evidence_file="$evidence_stage_dir/evidence.txt"

{
  printf 'TIMESTAMP_UTC=%s\n' "$evidence_timestamp"
  printf 'SCRIPT_SHA256=%s\n' "$script_hash_start"
  printf 'BACKUP_DIR=%s\n' "$backup_dir"
  printf 'MANIFEST_SHA256=%s\n' "$expected_manifest_sha256"
  printf 'RDB_SHA256=%s\n' "$expected_rdb_sha256"
  printf 'CHECK_REPORT_SHA256=%s\n' "$expected_check_sha256"
  printf 'COMPLETE_SHA256=%s\n' "$expected_complete_sha256"
  printf 'PRODUCTION_REDIS_ID=%s\n' "$production_redis_id"
  printf 'PRODUCTION_REDIS_IMAGE_ID=%s\n' "$production_image_id"
  printf 'PRODUCTION_REDIS_IMAGE_LABEL=%s\n' "$production_image_label"
  printf 'PRODUCTION_REDIS_RUNNING=%s\n' "$production_running"
  printf 'PRODUCTION_REDIS_RESTART_COUNT=%s\n' "$production_restart_count"
  printf 'PRODUCTION_REDIS_STARTED_AT=%s\n' "$production_started_at"
  printf 'PRODUCTION_REDIS_PID=%s\n' "$production_pid"
  printf 'DISPOSABLE_REDIS_ID=%s\n' "$restore_container_id"
  printf 'DISPOSABLE_REDIS_IMAGE_ID=%s\n' "$candidate_image_id"
  printf 'DISPOSABLE_REDIS_VERSION=%s\n' "$redis_version"
  printf 'STAGED_RDB_SHA256=%s\n' "$candidate_rdb_hash"
  printf 'PING=%s\n' "$ping_result"
  printf 'LOADING=%s\n' "$loading"
  printf 'RDB_KEYS_LOADED=%s\n' "$loaded_keys"
  printf 'RDB_KEYS_EXPIRED=%s\n' "$expired_keys"
  printf 'RDB_KEYS_TOTAL=%s\n' "$actual_total_keys"
  printf 'DBSIZE=%s\n' "$dbsize"
  printf 'NETWORK_MODE=none\n'
  printf 'PUBLISHED_PORTS=none\n'
  printf 'RDB_SOURCE_MOUNT=readonly\n'
  printf 'APPROVED_REDIS_INPUTS_UNCHANGED=true\n'
  printf 'PRODUCTION_REDIS_UNCHANGED=true\n'
  printf 'DISPOSABLE_CONTAINER_REMOVED=true\n'
} >"$staged_evidence_file"
chmod 600 -- "$staged_evidence_file"
sync -f "$staged_evidence_file"
assert_production_unchanged 'before_evidence_publish'
verify_inputs_unchanged
trap '' INT TERM HUP
printf 'REDIS_8_RESTORE_GATE=passed\n' >>"$staged_evidence_file"
sync -f "$staged_evidence_file"
evidence_sha256="$(hash_file "$staged_evidence_file")" || fail 'cannot hash restore-check evidence'
staged_checksum_file="$evidence_stage_dir/evidence.txt.sha256"
printf '%s  evidence.txt\n' "$evidence_sha256" >"$staged_checksum_file"
chmod 600 -- "$staged_checksum_file"
sync -f "$staged_checksum_file"
sync -f "$evidence_stage_dir"
mv -- "$evidence_stage_dir" "$evidence_dir"
sync -f "$evidence_root" || true
evidence_file="$evidence_dir/evidence.txt"

trap - EXIT INT TERM HUP

tee /dev/stderr >/dev/null <<EOF
PING=$ping_result
DBSIZE=$dbsize
RDB_LOADED=$loaded_keys
RDB_EXPIRED=$expired_keys
RDB_TOTAL=$actual_total_keys
EVIDENCE=$evidence_file
EVIDENCE_SHA256=$evidence_sha256
REDIS_8_RESTORE_VALIDATED_PRODUCTION_UNCHANGED
EOF
