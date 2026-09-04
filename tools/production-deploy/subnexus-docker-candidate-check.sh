#!/usr/bin/env bash
set -Eeuo pipefail

# SubNexus Docker candidate gate.
#
# This is a runtime release gate, not a deployment or build helper. It loads an
# image archive produced by the isolated local build gate, runs that image with
# disposable PostgreSQL/Redis services on a private Docker network, and proves
# that the three production containers were not changed while the gate was
# running. It never stops, restarts, or removes a production object. Every
# object it creates is tagged with a run token and cleanup accepts only the
# exact object ID/name plus that tag.

case "$-" in
  *x*) set +x ;;
esac
umask 077
unset BASH_ENV ENV CDPATH GLOBIGNORE
export PATH='/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin'

readonly gate_name='subnexus-docker-candidate-v1'
readonly default_evidence_root='/srv/subnexus-migration/docker-candidate'
readonly default_rpc_timeout_seconds=120
readonly default_wait_timeout_seconds=240
readonly default_stable_seconds=20
readonly cleanup_timeout_seconds=300
readonly cleanup_rpc_timeout_seconds=15
readonly app_memory_mb=768
readonly postgres_memory_mb=768
readonly redis_memory_mb=256
readonly app_swap_mb=768
readonly postgres_swap_mb=768
readonly redis_swap_mb=256
readonly app_pids_limit=256
readonly postgres_pids_limit=256
readonly redis_pids_limit=128
readonly minimum_docker_free_bytes=8589934592
readonly docker_free_reserve_bytes=4294967296
readonly docker_archive_multiplier=2
# Passing the runtime gate is evidence for review only.  It never authorizes
# an unattended production cutover.
readonly cutover_allowed='false'
readonly manual_review_required='true'

failure_reason=''
gate_result='failed'
interrupted='false'
cleanup_started='false'
cleanup_failed='false'
cleanup_active='false'
cleanup_deadline=''
evidence_stage_dir=''
evidence_final_dir=''
evidence_file=''
base_images_file=''
image_load_log_file=''
network_id=''
network_name=''
postgres_id=''
redis_id=''
app_id=''
postgres_volume=''
redis_volume=''
app_volume=''
candidate_image_id=''
expected_candidate_image_id=''
candidate_image_tag=''
candidate_archive_path=''
candidate_archive_lexical_path=''
candidate_archive_parent=''
candidate_archive_sha256=''
candidate_archive_size=''
candidate_archive_expanded_size=''
candidate_archive_fingerprint=''
observed_archive_sha256=''
create_cidfile=''
evidence_lock_fingerprint=''
run_token=''
source_root=''
approved_sha=''
tree_sha=''
evidence_root=''
rpc_timeout_seconds=''
wait_timeout_seconds=''
stable_seconds=''
docker_binary=''
docker_socket_path=''
docker_socket_fingerprint_start=''
docker_socket_mode=''
docker_context=''
docker_endpoint=''
docker_daemon_summary=''
docker_daemon_identity_start=''
docker_root_dir=''
docker_free_bytes_start=''
docker_free_bytes_before_create=''
docker_required_free_bytes=''
pg_image_ref=''
redis_image_ref=''
pg_build_ref=''
redis_build_ref=''
pg_password=''
redis_password=''
admin_password=''
jwt_secret=''
totp_key=''
pg_user='sub2api_gate'
pg_database='sub2api_gate'
postgres_runtime_image_id=''
redis_runtime_image_id=''
main_bashpid=''
candidate_summary_postgres=''
candidate_summary_redis=''
candidate_summary_app=''
candidate_id_before_cleanup_postgres=''
candidate_id_before_cleanup_redis=''
candidate_id_before_cleanup_app=''
candidate_image_preexisting='false'
candidate_image_retained='false'
candidate_health_postgres=''
candidate_health_redis=''
candidate_health_app=''
candidate_migration_count=''
candidate_migration_count_after_restart=''
candidate_started_at_before_restart=''
candidate_started_at_after_restart=''
candidate_restart_count_before=''
candidate_restart_count_after=''
candidate_login_verified='false'
candidate_public_settings_verified='false'
candidate_default_settings_verified='false'
candidate_sentinel_hash=''
candidate_sentinel_hash_after_restart=''
evidence_publish_failed='false'

declare -A production_ref=()
declare -A production_id=()
declare -A production_identity=()
declare -A production_summary=()
declare -A production_image_id=()
declare -A production_mount_sources=()

fail() {
  failure_reason="$*"
  printf 'ERROR: %s\n' "$*" >&2
  exit 1
}

usage() {
  cat >&2 <<'USAGE'
usage: subnexus-docker-candidate-check.sh SOURCE_ROOT APPROVED_COMMIT_SHA EXPECTED_IMAGE_ID IMAGE_ARCHIVE IMAGE_ARCHIVE_SHA256 PRODUCTION_APP PRODUCTION_POSTGRES PRODUCTION_REDIS [EVIDENCE_ROOT]

Required environment (references are repository digests or full image IDs;
the gate never pulls or builds images):
  SUBNEXUS_CANDIDATE_POSTGRES_IMAGE
  SUBNEXUS_CANDIDATE_REDIS_IMAGE

The source worktree must be root-owned, detached, clean, and already checked
out at APPROVED_COMMIT_SHA. The root-owned image archive must match the exact
SHA-256 and image ID approved from the isolated build host. Production
container arguments are names or full IDs used only for identity snapshots.
USAGE
}

# Return 0 only when an object is absent and the daemon is still reachable;
# return 1 when the object is present; return 2 when absence cannot be
# distinguished from a Docker daemon failure.  Docker uses exit status 1 for
# both "not found" and several transient client/daemon errors, so a diagnostic
# plus an independent exact-list query are required before returning absent.
#
# This helper is intentionally top-level so it can be exercised without
# entering the production candidate lifecycle.  Its docker_rpc dependency and
# runtime variables are resolved when it is called from main().
object_absent() {
  local kind="$1" object_ref="$2" inspect_output inspect_status listing
  local inspect_error

  case "$kind" in
    container)
      if inspect_output="$(docker_rpc inspect "$object_ref" 2>&1)"; then
        return 1
      else
        inspect_status=$?
      fi
      ;;
    network)
      if inspect_output="$(docker_rpc network inspect "$object_ref" 2>&1)"; then
        return 1
      else
        inspect_status=$?
      fi
      ;;
    volume)
      if inspect_output="$(docker_rpc volume inspect "$object_ref" 2>&1)"; then
        return 1
      else
        inspect_status=$?
      fi
      ;;
    image)
      if inspect_output="$(docker_rpc image inspect "$object_ref" 2>&1)"; then
        return 1
      else
        inspect_status=$?
      fi
      ;;
    *) return 2 ;;
  esac
  # Timeout and all non-standard failures are unknown.  Only the normal
  # Docker CLI "not found" status can proceed to the corroborating query.
  [[ "$inspect_status" -eq 1 ]] || return 2
  inspect_error="${inspect_output,,}"
  case "$kind:$inspect_error" in
    container:*'no such object'*|container:*'no such container'*) ;;
    network:*'no such object'*|network:*'no such network'*) ;;
    volume:*'no such object'*|volume:*'no such volume'*) ;;
    image:*'no such object'*|image:*'no such image'*) ;;
    *) return 2 ;;
  esac

  case "$kind" in
    container)
      if [[ "$object_ref" =~ ^[0-9a-f]{64}$ ]]; then
        listing="$(docker_rpc container ls --all --no-trunc --filter "id=$object_ref" --format '{{.ID}}' 2>/dev/null)" || return 2
      else
        listing="$(docker_rpc container ls --all --no-trunc --filter "name=^/${object_ref}$" --format '{{.ID}}' 2>/dev/null)" || return 2
      fi
      ;;
    network)
      if [[ "$object_ref" =~ ^[0-9a-f]{64}$ ]]; then
        listing="$(docker_rpc network ls --no-trunc --filter "id=$object_ref" --format '{{.ID}}' 2>/dev/null)" || return 2
      else
        listing="$(docker_rpc network ls --no-trunc --filter "name=^${object_ref}$" --format '{{.ID}}' 2>/dev/null)" || return 2
      fi
      ;;
    volume)
      listing="$(docker_rpc volume ls --filter "name=^${object_ref}$" --format '{{.Name}}' 2>/dev/null)" || return 2
      ;;
    image)
      listing="$(docker_rpc image ls --no-trunc --filter "reference=$object_ref" --format '{{.ID}}' 2>/dev/null)" || return 2
      ;;
  esac
  [[ -z "$(printf '%s' "$listing" | tr -d '\r\n')" ]] || return 1
  docker_rpc info >/dev/null 2>&1 || return 2
  return 0
}

main() {
[[ "$EUID" -eq 0 ]] || fail 'run this gate as root'
[[ "$#" -eq 8 || "$#" -eq 9 ]] || { usage; exit 2; }

source_root="$1"
approved_sha="$2"
expected_candidate_image_id="$3"
candidate_archive_path="$4"
candidate_archive_sha256="$5"
production_app_ref="$6"
production_postgres_ref="$7"
production_redis_ref="$8"
evidence_root="${9:-/srv/subnexus-migration/docker-candidate}"

[[ "$approved_sha" =~ ^[0-9a-f]{40}$ ]] || fail 'approved commit must be a lowercase 40-character SHA'
[[ "$expected_candidate_image_id" =~ ^[0-9a-f]{64}$ ]] || fail 'expected candidate image ID must be 64 lowercase hexadecimal characters'
[[ "$candidate_archive_sha256" =~ ^[0-9a-f]{64}$ ]] || fail 'candidate archive SHA256 must be 64 lowercase hexadecimal characters'
candidate_image_tag="subnexus-release:${approved_sha}"

pg_image_ref="${SUBNEXUS_CANDIDATE_POSTGRES_IMAGE:-}"
redis_image_ref="${SUBNEXUS_CANDIDATE_REDIS_IMAGE:-}"
[[ -n "$pg_image_ref" ]] || fail 'SUBNEXUS_CANDIDATE_POSTGRES_IMAGE is required'
[[ -n "$redis_image_ref" ]] || fail 'SUBNEXUS_CANDIDATE_REDIS_IMAGE is required'

valid_container_ref() {
  [[ "$1" =~ ^([A-Za-z0-9][A-Za-z0-9_.-]{0,254}|[0-9a-fA-F]{12,64})$ ]]
}

valid_immutable_image_ref() {
  # Keep an optional registry host/port separate from the repository path so
  # a tag colon cannot be confused with a registry port.  Docker's canonical
  # RepoDigests preserve that port and resolve_image_ref compares it exactly.
  [[ "$1" =~ ^(([A-Za-z0-9][A-Za-z0-9._-]*(\:[0-9]{1,5})?\/)?[A-Za-z0-9][A-Za-z0-9._/-]*)(\:[A-Za-z0-9._-]+)?@sha256:[0-9a-f]{64}$|^sha256:[0-9a-f]{64}$ ]]
}

valid_full_id() {
  [[ "$1" =~ ^[0-9a-f]{64}$ ]]
}

valid_positive_integer() {
  [[ "$1" =~ ^[0-9]+$ ]] && (( 10#$1 > 0 ))
}

for production_ref_value in "$production_app_ref" "$production_postgres_ref" "$production_redis_ref"; do
  valid_container_ref "$production_ref_value" || fail 'production container reference contains unsupported characters'
done
for image_ref_value in "$pg_image_ref" "$redis_image_ref"; do
  valid_immutable_image_ref "$image_ref_value" || fail 'every candidate/base image must be pinned by a repository digest or full image ID'
done

parse_bounded_integer_env() {
  local name="$1"
  local default_value="$2"
  local maximum="$3"
  local value="${!name:-$default_value}"
  valid_positive_integer "$value" || fail "$name must be a positive integer"
  (( 10#$value <= maximum )) || fail "$name exceeds its safety limit"
  printf '%s' "$value"
}

rpc_timeout_seconds="$(parse_bounded_integer_env SUBNEXUS_DOCKER_RPC_TIMEOUT_SECONDS 120 300)"
wait_timeout_seconds="$(parse_bounded_integer_env SUBNEXUS_DOCKER_WAIT_TIMEOUT_SECONDS 240 900)"
stable_seconds="$(parse_bounded_integer_env SUBNEXUS_DOCKER_STABLE_SECONDS 20 300)"

for command_name in docker git timeout realpath stat sha256sum awk sed grep tr sort date mkdir chmod flock sync mv rm sleep cat cut df python3; do
  command -v "$command_name" >/dev/null 2>&1 || fail "missing command: $command_name"
done

validate_executable() {
  local command_name="$1"
  local path target mode
  path="$(command -v "$command_name")" || fail "cannot resolve executable: $command_name"
  target="$(realpath -e -- "$path")" || fail "cannot resolve executable target: $command_name"
  [[ "$target" == /* && -f "$target" ]] || fail "executable is not a regular absolute file: $command_name"
  [[ "$(stat -c '%u' -- "$target")" == '0' ]] || fail "executable must be root-owned: $command_name"
  mode="$(stat -c '%a' -- "$target")" || fail "cannot inspect executable mode: $command_name"
  [[ "$mode" =~ ^[0-7]{3,4}$ ]] || fail "executable mode is invalid: $command_name"
  (( (8#$mode & 8#22) == 0 )) || fail "executable must not be group/other writable: $command_name"
  printf '%s' "$target"
}

docker_binary="$(validate_executable docker)"
validate_executable git >/dev/null
validate_executable timeout >/dev/null

mode_is_safe() {
  local path="$1"
  local mode
  mode="$(stat -c '%a' -- "$path")" || return 1
  [[ "$mode" =~ ^[0-7]{3,4}$ ]] || return 1
  (( (8#$mode & 8#22) == 0 ))
}

validate_secure_directory() {
  local directory="$1"
  [[ -d "$directory" && ! -L "$directory" ]] || fail "secure directory is missing or symbolic: $directory"
  [[ "$(realpath -e -- "$directory")" == "$directory" ]] || fail "secure directory is not canonical: $directory"
  [[ "$(stat -c '%u' -- "$directory")" == '0' ]] || fail "secure directory must be root-owned: $directory"
  mode_is_safe "$directory" || fail "secure directory must not be group/other writable: $directory"
}

ensure_secure_directory() {
  local directory="$1"
  local parent="${directory%/*}"
  if [[ ! -e "$directory" && ! -L "$directory" ]]; then
    [[ -d "$parent" && ! -L "$parent" ]] || fail "secure parent directory is missing: $parent"
    [[ "$(stat -c '%u' -- "$parent")" == '0' ]] || fail "secure parent directory must be root-owned: $parent"
    mkdir -- "$directory" || fail "cannot create secure directory: $directory"
    chmod 700 -- "$directory"
  fi
  validate_secure_directory "$directory"
  chmod 700 -- "$directory"
}

case "$evidence_root" in
  /srv/subnexus-migration/docker-candidate|/root/subnexus-migration/docker-candidate) ;;
  *) fail 'evidence root must be the approved root-only Docker candidate directory' ;;
esac
evidence_root="$(realpath -m -- "$evidence_root")" || fail 'cannot normalize evidence root'
evidence_parent="${evidence_root%/*}"
[[ -d "$evidence_parent" && ! -L "$evidence_parent" ]] || fail 'evidence parent directory is missing or symbolic'
[[ "$(stat -c '%u' -- "$evidence_parent")" == '0' ]] || fail 'evidence parent must be root-owned'
for docker_override in DOCKER_HOST DOCKER_CONTEXT DOCKER_CONFIG DOCKER_TLS_VERIFY DOCKER_CERT_PATH DOCKER_API_VERSION; do
  [[ -z "${!docker_override:-}" ]] || fail "$docker_override must be unset for a local Docker gate"
done

docker_rpc() {
  local timeout_seconds="$rpc_timeout_seconds"
  if [[ "$cleanup_active" == 'true' ]]; then
    [[ -n "$cleanup_deadline" && "$SECONDS" -lt "$cleanup_deadline" ]] || return 124
    timeout_seconds="$cleanup_rpc_timeout_seconds"
  fi
  timeout --foreground --kill-after=10s "${timeout_seconds}s" "$docker_binary" "$@"
}

# Cleanup is defined before the first candidate object can be created.  Every
# destructive Docker operation below is guarded by the production identity
# check and accepts only the exact ID/name generated by this run.
cleanup_resources() {
  local role id name observed_name observed_gate observed_token observed_role
  local mountpoint production_source current_production_id cid_path failed='false'
  [[ "$cleanup_started" == 'false' ]] || return 0
  cleanup_started='true'
  cleanup_active='true'
  cleanup_deadline=$((SECONDS + cleanup_timeout_seconds))

  cleanup_budget_available() {
    [[ "$SECONDS" -lt "$cleanup_deadline" ]]
  }

  cleanup_container() {
    role="$1"
    id="$2"
    name="$3"
    cleanup_budget_available || { failed='true'; return 0; }
    cid_path="${create_cidfile}.${role}"
    if [[ -z "$id" && -f "$cid_path" && ! -L "$cid_path" ]]; then
      id="$(tr -d '\r\n' <"$cid_path" 2>/dev/null || true)"
    fi
    if [[ -z "$id" ]]; then
      if ! id="$(docker_rpc inspect --format '{{.Id}}' "$name" 2>/dev/null)"; then
        object_absent container "$name" || failed='true'
        return 0
      fi
    fi
    [[ -n "$id" ]] || return 0
    valid_full_id "$id" || { failed='true'; return 0; }
    if ! observed_name="$(docker_rpc inspect --format '{{.Name}}' "$id" 2>/dev/null)"; then
      object_absent container "$id" || failed='true'
      return 0
    fi
    observed_gate="$(docker_rpc inspect --format '{{index .Config.Labels "com.subnexus.candidate.gate"}}' "$id" 2>/dev/null)" || { failed='true'; return 0; }
    observed_token="$(docker_rpc inspect --format '{{index .Config.Labels "com.subnexus.candidate.token"}}' "$id" 2>/dev/null)" || { failed='true'; return 0; }
    observed_role="$(docker_rpc inspect --format '{{index .Config.Labels "com.subnexus.candidate.role"}}' "$id" 2>/dev/null)" || { failed='true'; return 0; }
    [[ "$observed_name" == "/$name" && "$observed_gate" == "$gate_name" &&
      "$observed_token" == "$run_token" && "$observed_role" == "$role" ]] || {
      failed='true'
      return 0
    }
    [[ "$id" != "${production_id[app]:-}" && "$id" != "${production_id[postgres]:-}" &&
      "$id" != "${production_id[redis]:-}" ]] || { failed='true'; return 0; }
    for current_production_id in \
      "$(docker_rpc inspect --format '{{.Id}}' "${production_ref[app]}" 2>/dev/null || true)" \
      "$(docker_rpc inspect --format '{{.Id}}' "${production_ref[postgres]}" 2>/dev/null || true)" \
      "$(docker_rpc inspect --format '{{.Id}}' "${production_ref[redis]}" 2>/dev/null || true)"; do
      [[ -z "$current_production_id" || "$id" != "$current_production_id" ]] || { failed='true'; return 0; }
    done
    assert_production_unchanged "before_cleanup_container_${role}" || { failed='true'; return 0; }
    # Remove only the exact candidate container. Volumes are removed below
    # through the separately validated, role-scoped volume path; keeping
    # --volumes off prevents an altered container from deleting an unexpected
    # anonymous volume as a side effect.
    cleanup_budget_available || { failed='true'; return 0; }
    if ! docker_checked "cleanup_container_rm_${role}" container rm --force "$id" >/dev/null 2>&1; then
      object_absent container "$id" || failed='true'
    fi
    object_absent container "$id" || failed='true'
  }

  cleanup_network() {
    local network_id_value="$1" network_name_value="$2"
    local network_gate network_token network_observed_name
    cleanup_budget_available || { failed='true'; return 0; }
    if [[ -z "$network_id_value" ]]; then
      if ! network_id_value="$(docker_rpc network inspect --format '{{.Id}}' "$network_name_value" 2>/dev/null)"; then
        object_absent network "$network_name_value" || failed='true'
        return 0
      fi
    fi
    [[ -n "$network_id_value" ]] || return 0
    valid_full_id "$network_id_value" || { failed='true'; return 0; }
    network_observed_name="$(docker_rpc network inspect --format '{{.Name}}' "$network_id_value" 2>/dev/null)" || {
      object_absent network "$network_id_value" || failed='true'
      return 0
    }
    network_gate="$(docker_rpc network inspect --format '{{index .Labels "com.subnexus.candidate.gate"}}' "$network_id_value" 2>/dev/null)" || { failed='true'; return 0; }
    network_token="$(docker_rpc network inspect --format '{{index .Labels "com.subnexus.candidate.token"}}' "$network_id_value" 2>/dev/null)" || { failed='true'; return 0; }
    [[ "$network_observed_name" == "$network_name_value" && "$network_gate" == "$gate_name" &&
      "$network_token" == "$run_token" ]] || { failed='true'; return 0; }
    assert_network_not_production "$network_id_value" || { failed='true'; return 0; }
    assert_production_unchanged before_cleanup_network || { failed='true'; return 0; }
    # Equivalent Docker operation: docker network rm <exact-candidate-id>.
    cleanup_budget_available || { failed='true'; return 0; }
    if ! docker_checked cleanup_network_rm network rm "$network_id_value" >/dev/null 2>&1; then
      object_absent network "$network_id_value" || failed='true'
    fi
    object_absent network "$network_id_value" || failed='true'
  }

  cleanup_volume() {
    local volume_name_value="$1" expected_role="$2"
    local volume_observed_name volume_gate volume_token volume_role
    cleanup_budget_available || { failed='true'; return 0; }
    [[ -n "$volume_name_value" ]] || return 0
    if ! volume_observed_name="$(docker_rpc volume inspect --format '{{.Name}}' "$volume_name_value" 2>/dev/null)"; then
      object_absent volume "$volume_name_value" || failed='true'
      return 0
    fi
    volume_gate="$(docker_rpc volume inspect --format '{{index .Labels "com.subnexus.candidate.gate"}}' "$volume_name_value" 2>/dev/null)" || { failed='true'; return 0; }
    volume_token="$(docker_rpc volume inspect --format '{{index .Labels "com.subnexus.candidate.token"}}' "$volume_name_value" 2>/dev/null)" || { failed='true'; return 0; }
    volume_role="$(docker_rpc volume inspect --format '{{index .Labels "com.subnexus.candidate.role"}}' "$volume_name_value" 2>/dev/null)" || { failed='true'; return 0; }
    [[ "$volume_observed_name" == "$volume_name_value" && "$volume_gate" == "$gate_name" &&
      "$volume_token" == "$run_token" && "$volume_role" == "$expected_role" ]] || { failed='true'; return 0; }
    validate_volume_labels "$volume_name_value" "$expected_role" || { failed='true'; return 0; }
    mountpoint="$(docker_rpc volume inspect --format '{{.Mountpoint}}' "$volume_name_value" 2>/dev/null)" || { failed='true'; return 0; }
    [[ -n "$mountpoint" ]] || { failed='true'; return 0; }
    mountpoint="$(realpath -e -- "$mountpoint" 2>/dev/null || true)"
    [[ "$mountpoint" == "$docker_root_dir/volumes/$volume_name_value/_data" && -d "$mountpoint" && ! -L "$mountpoint" ]] || { failed='true'; return 0; }
    for role in app postgres redis; do
      while IFS= read -r production_source; do
        [[ -n "$production_source" ]] || continue
        [[ "$mountpoint" != "$production_source" && "$mountpoint" != "$production_source"/* ]] || { failed='true'; return 0; }
      done <<< "${production_mount_sources[$role]:-}"
      current_production_id="$(docker_rpc inspect --format '{{.Id}}' "${production_ref[$role]}" 2>/dev/null || true)"
      if valid_full_id "$current_production_id"; then
        while IFS= read -r production_source; do
          [[ -n "$production_source" ]] || continue
          [[ "$mountpoint" != "$production_source" && "$mountpoint" != "$production_source"/* ]] || { failed='true'; return 0; }
        done < <(capture_container_mount_sources "$current_production_id" 2>/dev/null || true)
      fi
    done
    assert_production_unchanged "before_cleanup_volume_${expected_role}" || { failed='true'; return 0; }
    # Equivalent Docker operation: docker volume rm <exact-candidate-volume>.
    cleanup_budget_available || { failed='true'; return 0; }
    if ! docker_checked "cleanup_volume_rm_${expected_role}" volume rm "$volume_name_value" >/dev/null 2>&1; then
      object_absent volume "$volume_name_value" || failed='true'
    fi
    object_absent volume "$volume_name_value" || failed='true'
  }

  cleanup_container app "$app_id" "$app_name"
  cleanup_container redis "$redis_id" "$redis_name"
  cleanup_container postgres "$postgres_id" "$postgres_name"
  cleanup_network "$network_id" "$network_name"
  cleanup_volume "$app_volume" app
  cleanup_volume "$redis_volume" redis
  cleanup_volume "$postgres_volume" postgres

  # Remove only the per-role cidfiles generated by this run.
  for role in app redis postgres; do
    cleanup_budget_available || { failed='true'; break; }
    cid_path="${create_cidfile}.${role}"
    if [[ -e "$cid_path" || -L "$cid_path" ]]; then
      if [[ ! -L "$cid_path" && -f "$cid_path" && "$(stat -c '%u' -- "$cid_path" 2>/dev/null)" == '0' &&
        "$(stat -c '%h' -- "$cid_path" 2>/dev/null)" == '1' && "$cid_path" == "$evidence_root/.${run_token}.cid.$role" ]]; then
        rm -f -- "$cid_path" || failed='true'
      else
        failed='true'
      fi
    fi
  done
  # The approved release image is intentionally retained for the later manual
  # cutover. This runtime gate only removes its own containers, volumes and
  # private network.
  cleanup_active='false'
  [[ "$SECONDS" -lt "$cleanup_deadline" ]] || failed='true'
  cleanup_failed="$failed"
  [[ "$failed" == 'false' ]]
}

capture_candidate_summaries() {
  if [[ -n "$postgres_id" ]]; then
    candidate_id_before_cleanup_postgres="$postgres_id"
    candidate_summary_postgres="$(capture_container_summary "$postgres_id" 2>/dev/null || true)"
  fi
  if [[ -n "$redis_id" ]]; then
    candidate_id_before_cleanup_redis="$redis_id"
    candidate_summary_redis="$(capture_container_summary "$redis_id" 2>/dev/null || true)"
  fi
  if [[ -n "$app_id" ]]; then
    candidate_id_before_cleanup_app="$app_id"
    candidate_summary_app="$(capture_container_summary "$app_id" 2>/dev/null || true)"
  fi
}

docker_context="$(docker_rpc context show 2>/dev/null)" || fail 'cannot determine Docker context'
[[ "$docker_context" == 'default' ]] || fail "Docker context must be default: $docker_context"
  docker_endpoint="$(docker_rpc context inspect --format '{{(index .Endpoints "docker").Host}}' default 2>/dev/null)" || fail 'cannot determine Docker endpoint'
case "$docker_endpoint" in
  unix:///var/run/docker.sock|unix:///run/docker.sock) ;;
  *) fail "Docker endpoint must be the local system Unix socket: $docker_endpoint" ;;
esac
  docker_socket_path="${docker_endpoint#unix://}"
  [[ -S "$docker_socket_path" && ! -L "$docker_socket_path" ]] || fail 'Docker endpoint must be a non-symbolic Unix socket'
[[ "$(stat -c '%u' -- "$docker_socket_path")" == '0' ]] || fail 'Docker socket must be root-owned'
docker_socket_mode="$(stat -c '%a' -- "$docker_socket_path")" || fail 'cannot inspect Docker socket mode'
[[ "$docker_socket_mode" =~ ^[0-7]{3,4}$ ]] || fail 'Docker socket mode is invalid'
# Docker installations commonly expose root:docker 0660. Group access is
# acceptable for the already-root gate; any other-world bits are not.
(( (8#$docker_socket_mode & 8#007) == 0 && (8#$docker_socket_mode & 8#600) == 8#600 )) ||
  fail 'Docker socket mode must be owner rw with no other access'
docker_socket_fingerprint_start="$(stat -Lc '%d|%i|%F|%u|%g|%a' -- "$docker_socket_path")" || fail 'cannot fingerprint Docker socket'
docker_daemon_summary="$(docker_rpc info --format '{{.Name}}|{{.ServerVersion}}|{{.DockerRootDir}}|{{json .SecurityOptions}}')" || fail 'cannot inspect Docker daemon'
docker_daemon_identity_start="$(docker_rpc info --format '{{.ID}}|{{.Name}}|{{.ServerVersion}}|{{.DockerRootDir}}|{{json .SecurityOptions}}')" || fail 'cannot fingerprint Docker daemon'
docker_root_dir="$(docker_rpc info --format '{{.DockerRootDir}}')" || fail 'cannot determine Docker storage directory'
[[ "$docker_root_dir" == /* && ! -L "$docker_root_dir" && -d "$docker_root_dir" ]] || fail 'Docker storage directory is not a secure directory'
[[ "$(stat -c '%u' -- "$docker_root_dir")" == '0' ]] || fail 'Docker storage directory must be root-owned'
mode_is_safe "$docker_root_dir" || fail 'Docker storage directory must not be group/other writable'
[[ "$docker_daemon_summary" == *'name=seccomp'* ]] || fail 'Docker daemon must provide seccomp isolation'

capture_container_identity() {
  local container_id="$1"
  docker_rpc inspect --format '{{json .}}' "$container_id" | python3 -c '
import hashlib
import json
import re
import sys

try:
    obj = json.load(sys.stdin)
    config = obj.get("Config") or {}
    host = obj.get("HostConfig") or {}
    state = obj.get("State") or {}
    network = obj.get("NetworkSettings") or {}
    secret_name = re.compile(r"(?:PASSWORD|SECRET|TOKEN|KEY|COOKIE|PRIVATE)", re.I)
    env = []
    for item in config.get("Env") or []:
        name, sep, value = item.partition("=")
        if secret_name.search(name):
            value = "sha256:" + hashlib.sha256(value.encode()).hexdigest()
        env.append(name + ("=" + value if sep else ""))
    selected = {
        "Id": obj.get("Id"),
        "Name": obj.get("Name"),
        "Image": obj.get("Image"),
        "Config": {
            key: config.get(key) for key in (
                "Image", "Entrypoint", "Cmd", "User", "WorkingDir", "Env",
                "ExposedPorts", "Labels", "StopSignal", "Healthcheck",
            )
        },
        "HostConfig": {
            key: host.get(key) for key in (
                "Binds", "CapAdd", "CapDrop", "CgroupnsMode", "ConsoleSize",
                "CpuPeriod", "CpuQuota", "CpuShares", "CpusetCpus", "CpusetMems",
                "DeviceCgroupRules", "Devices", "DeviceRequests", "IpcMode",
                "LogConfig", "Memory", "MemoryReservation", "MemorySwap", "NanoCpus",
                "NetworkMode", "OomKillDisable", "PidsLimit", "PidMode", "PortBindings",
                "Privileged", "ReadonlyRootfs", "RestartPolicy", "SecurityOpt",
                "ShmSize", "UTSMode", "UsernsMode", "VolumesFrom", "Tmpfs",
            )
        },
        "Mounts": [
            {key: mount.get(key) for key in ("Type", "Name", "Source", "Destination", "Mode", "RW", "Propagation")}
            for mount in (obj.get("Mounts") or [])
        ],
        "Networks": {
            name: {key: value.get(key) for key in ("NetworkID", "EndpointID", "Gateway", "IPAddress", "IPPrefixLen", "IPv6Gateway", "GlobalIPv6Address", "GlobalIPv6PrefixLen", "Aliases", "DriverOpts")}
            for name, value in (network.get("Networks") or {}).items()
        },
        "State": {key: state.get(key) for key in ("Status", "Running", "Paused", "Restarting", "OOMKilled", "Pid", "ExitCode", "StartedAt", "FinishedAt", "RestartCount")},
    }
    selected["Config"]["Env"] = sorted(env)
    print(hashlib.sha256(json.dumps(selected, sort_keys=True, separators=(",", ":")).encode()).hexdigest())
except (ValueError, TypeError, json.JSONDecodeError):
    raise SystemExit(1)
'
}

capture_container_summary() {
  local container_id="$1"
  docker_rpc inspect --format '{{.Id}}|{{.Image}}|{{.Config.Image}}|{{.State.Status}}|{{.State.Running}}|{{.State.StartedAt}}|{{.RestartCount}}' "$container_id"
}

capture_container_mount_sources() {
  local container_id="$1"
  docker_rpc inspect --format '{{range .Mounts}}{{println .Source}}{{end}}' "$container_id"
}

capture_image_id() {
  local image_ref_value="$1"
  local image_id_value
  image_id_value="$(docker_rpc image inspect --format '{{.Id}}' "$image_ref_value")" || return 1
  [[ "$image_id_value" =~ ^sha256:[0-9a-f]{64}$ ]] || return 1
  printf '%s' "${image_id_value#sha256:}"
}

# Resolve an approved image reference without ever pulling it. A repository
# digest is retained for Dockerfile FROM arguments; an image ID is accepted
# only when the daemon can inspect that exact immutable ID.
resolve_image_ref() {
  local requested="$1"
  local resolved_id repo_digests requested_name requested_repo requested_digest candidate
  if [[ "$requested" =~ ^sha256:([0-9a-f]{64})$ ]]; then
    resolved_id="$(capture_image_id "$requested")" || fail 'approved image ID is not present locally'
    printf 'sha256:%s' "$resolved_id"
    return 0
  fi
  [[ "$requested" =~ ^(.+)@sha256:([0-9a-f]{64})$ ]] || fail 'approved repository digest is malformed'
  requested_name="${BASH_REMATCH[1]}"
  requested_digest="${BASH_REMATCH[2]}"
  requested_repo="$requested_name"
  # Docker's RepoDigests omit the tag.  Preserve registry ports while removing
  # a tag only from the final path component (for example, host:5000/app:1).
  if [[ "${requested_name##*/}" == *:* ]]; then
    requested_repo="${requested_name%:*}"
  fi
  resolved_id="$(capture_image_id "$requested")" || fail 'approved repository digest is not present locally'
  repo_digests="$(docker_rpc image inspect --format '{{range .RepoDigests}}{{println .}}{{end}}' "$requested")" || fail 'cannot inspect approved repository digest'
  candidate="${requested_repo}@sha256:${requested_digest}"
  printf '%s\n' "$repo_digests" | grep -Fqx -- "$candidate" || fail 'approved image repository digest is not recorded by Docker'
  printf '%s' "$requested"
}

production_ref[app]="$production_app_ref"
production_ref[postgres]="$production_postgres_ref"
production_ref[redis]="$production_redis_ref"

for role in app postgres redis; do
  production_id[$role]="$(docker_rpc inspect --format '{{.Id}}' "${production_ref[$role]}" 2>/dev/null)" || fail "production $role container was not found"
  valid_full_id "${production_id[$role]}" || fail "production $role did not resolve to a full container ID"
  [[ "$(docker_rpc inspect --format '{{.State.Running}}' "${production_id[$role]}")" == 'true' ]] || fail "production $role container is not running"
  production_identity[$role]="$(capture_container_identity "${production_id[$role]}")" || fail "cannot capture production $role identity"
  production_summary[$role]="$(capture_container_summary "${production_id[$role]}")" || fail "cannot capture production $role summary"
  production_image_ref_resolved="$(docker_rpc inspect --format '{{.Image}}' "${production_id[$role]}")" || fail "cannot resolve production $role image"
  production_image_id[$role]="$(capture_image_id "$production_image_ref_resolved")" || fail "production $role image is not an immutable image ID"
  production_mount_sources[$role]="$(capture_container_mount_sources "${production_id[$role]}")" || fail "cannot capture production $role mount sources"
done
[[ "${production_id[app]}" != "${production_id[postgres]}" &&
  "${production_id[app]}" != "${production_id[redis]}" &&
  "${production_id[postgres]}" != "${production_id[redis]}" ]] || fail 'production references must resolve to three distinct containers'

assert_production_unchanged() {
  local phase="$1"
  local role current_id current_identity socket_fingerprint daemon_identity
  for role in app postgres redis; do
    current_id="$(docker_rpc inspect --format '{{.Id}}' "${production_ref[$role]}" 2>/dev/null)" || {
      printf 'ERROR: production %s cannot be inspected during %s\n' "$role" "$phase" >&2
      return 1
    }
    [[ "$current_id" == "${production_id[$role]}" ]] || {
      printf 'ERROR: production %s container ID changed during %s\n' "$role" "$phase" >&2
      return 1
    }
    current_identity="$(capture_container_identity "$current_id" 2>/dev/null)" || {
      printf 'ERROR: production %s identity cannot be captured during %s\n' "$role" "$phase" >&2
      return 1
    }
    [[ "$current_identity" == "${production_identity[$role]}" ]] || {
      printf 'ERROR: production %s identity changed during %s\n' "$role" "$phase" >&2
      return 1
    }
  done
  socket_fingerprint="$(stat -Lc '%d|%i|%F|%u|%g|%a' -- "$docker_socket_path" 2>/dev/null)" || {
    printf 'ERROR: Docker socket cannot be inspected during %s\n' "$phase" >&2
    return 1
  }
  [[ "$socket_fingerprint" == "$docker_socket_fingerprint_start" ]] || {
    printf 'ERROR: Docker socket identity changed during %s\n' "$phase" >&2
    return 1
  }
  daemon_identity="$(docker_rpc info --format '{{.ID}}|{{.Name}}|{{.ServerVersion}}|{{.DockerRootDir}}|{{json .SecurityOptions}}' 2>/dev/null)" || {
    printf 'ERROR: Docker daemon cannot be fingerprinted during %s\n' "$phase" >&2
    return 1
  }
  [[ "$daemon_identity" == "$docker_daemon_identity_start" ]] || {
    printf 'ERROR: Docker daemon identity changed during %s\n' "$phase" >&2
    return 1
  }
  return 0
}

assert_production_unchanged 'initial_snapshot' || fail 'production identity changed during initial snapshot'

# Evidence and artifact paths are host-side write/read targets.  Refuse to use
# any path that falls inside a source mounted by a production container, even
# when the path is otherwise root-owned and canonical.  This prevents a gate
# run from writing evidence into production data or reading an archive from a
# live application mount.
assert_path_outside_production_mounts() {
  local path="$1" lexical_path physical_path parent_path role production_source canonical_source
  [[ "$path" != *$'\n'* && "$path" != *$'\r'* ]] ||
    fail "controlled path contains a newline: $path"
  lexical_path="$(realpath -m -s -- "$path" 2>/dev/null)" ||
    fail "cannot normalize controlled path: $path"
  if [[ -e "$path" || -L "$path" ]]; then
    physical_path="$(realpath -e -P -- "$path" 2>/dev/null)" ||
      fail "cannot canonicalize controlled path: $path"
    [[ "$physical_path" == "$lexical_path" ]] ||
      fail "controlled path contains a symbolic-link component: $path"
  else
    parent_path="${lexical_path%/*}"
    [[ "$parent_path" != "$lexical_path" ]] || parent_path='/'
    physical_path="$(realpath -e -P -- "$parent_path" 2>/dev/null)" ||
      fail "cannot canonicalize controlled path parent: $path"
    [[ "$physical_path" == "$parent_path" ]] ||
      fail "controlled path parent contains a symbolic-link component: $path"
    physical_path="$lexical_path"
  fi
  for role in app postgres redis; do
    while IFS= read -r production_source; do
      [[ -n "$production_source" ]] || continue
      canonical_source="$(realpath -e -P -- "$production_source" 2>/dev/null)" ||
        fail "cannot canonicalize production $role mount source"
      [[ "$canonical_source" != '/' ]] ||
        fail "production $role mount source is the filesystem root"
      if [[ "$physical_path" == "$canonical_source" ||
        "$physical_path" == "$canonical_source"/* ||
        "$canonical_source" == "$physical_path"/* ]]; then
        fail "controlled path overlaps production $role mount: $path"
      fi
    done <<< "${production_mount_sources[$role]:-}"
  done
}

assert_path_outside_production_mounts "$evidence_root"
ensure_secure_directory "$evidence_root"

source_root="$(realpath -e -- "$source_root")" || fail 'source root does not exist'
[[ -d "$source_root" && ! -L "$source_root" ]] || fail 'source root must be a non-symlink directory'
[[ "$(stat -c '%u' -- "$source_root")" == '0' ]] || fail 'source root must be root-owned'
mode_is_safe "$source_root" || fail 'source root must not be group/other writable'
# The source tree is also read by the gate and must not resolve into a live
# production bind/volume mount, including when Docker reports a symlinked mount
# source.  The raw-prefix loop below remains a cheap defense-in-depth check.
assert_path_outside_production_mounts "$source_root"
for role in app postgres redis; do
  while IFS= read -r mount_source; do
    [[ -n "$mount_source" ]] || continue
    case "$source_root" in
      "$mount_source"|"$mount_source"/*) fail 'source root is inside a production container mount' ;;
    esac
  done <<< "${production_mount_sources[$role]}"
done

git_worktree_root="$(git -C "$source_root" rev-parse --show-toplevel 2>/dev/null)" || fail 'source root is not a Git worktree'
git_worktree_root="$(realpath -e -- "$git_worktree_root")" || fail 'cannot resolve Git worktree root'
[[ "$git_worktree_root" == "$source_root" ]] || fail 'source root is not the Git worktree root'
[[ -d "$source_root/.git" && ! -L "$source_root/.git" ]] || fail 'Git metadata must be a root-owned directory'
[[ "$(stat -c '%u' -- "$source_root/.git")" == '0' ]] || fail 'Git metadata must be root-owned'
git_head="$(git -C "$source_root" rev-parse HEAD 2>/dev/null)" || fail 'cannot read Git HEAD'
[[ "$git_head" == "$approved_sha" ]] || fail 'Git HEAD does not equal the approved full commit SHA'
git_symbolic_head="$(git -C "$source_root" symbolic-ref --quiet --short HEAD 2>/dev/null || true)"
[[ -z "$git_symbolic_head" ]] || fail 'source worktree must be detached'
git_status="$(git -C "$source_root" status --porcelain=v1 --untracked-files=all 2>/dev/null)" || fail 'cannot inspect Git worktree status'
[[ -z "$git_status" ]] || fail 'source worktree must be clean'
git_submodules="$(git -C "$source_root" submodule status --recursive 2>/dev/null)" || fail 'cannot inspect Git submodules'
[[ -z "$git_submodules" ]] || fail 'Git submodules are not allowed in the candidate build context'
tree_sha="$(git -C "$source_root" rev-parse "$approved_sha^{tree}" 2>/dev/null)" || fail 'cannot resolve approved Git tree'
[[ "$tree_sha" =~ ^[0-9a-f]{40}$ ]] || fail 'approved Git tree hash is invalid'

validate_candidate_archive_manifest() {
  timeout --foreground --kill-after=10s 120s python3 - "$candidate_archive_path" "$candidate_image_tag" "$expected_candidate_image_id" <<'PY'
import gzip
import hashlib
import io
import json
import pathlib
import re
import sys
import tarfile

archive, expected_tag, _expected_id = sys.argv[1:]
sha256_re = re.compile(r"^[0-9a-f]{64}$")
config_path_re = re.compile(r"^(?:blobs/sha256/([0-9a-f]{64})|([0-9a-f]{64})\.json)$")
layer_blob_path_re = re.compile(r"^blobs/sha256/[0-9a-f]{64}$")
legacy_layer_path_re = re.compile(r"^(?:[^/]+/)?layer\.tar$")
max_expanded_layer_bytes = 12 * 1024**3


class PrefixReader(io.RawIOBase):
    """Expose bytes already consumed while probing a layer's compression."""

    def __init__(self, prefix, stream):
        self._prefix = memoryview(prefix)
        self._offset = 0
        self._stream = stream

    def readable(self):
        return True

    def readinto(self, target):
        remaining = len(self._prefix) - self._offset
        if remaining:
            count = min(len(target), remaining)
            target[:count] = self._prefix[self._offset:self._offset + count]
            self._offset += count
            return count
        chunk = self._stream.read(len(target))
        if not chunk:
            return 0
        target[:len(chunk)] = chunk
        return len(chunk)


def layer_digest(bundle, member):
    source = bundle.extractfile(member)
    if source is None:
        raise SystemExit("Docker archive layer is unreadable")
    prefix = source.read(2)
    reader = PrefixReader(prefix, source)
    if prefix == b"\x1f\x8b":
        stream = gzip.GzipFile(fileobj=io.BufferedReader(reader), mode="rb")
    else:
        stream = io.BufferedReader(reader)
    digest = hashlib.sha256()
    expanded = 0
    while True:
        chunk = stream.read(1024 * 1024)
        if not chunk:
            break
        expanded += len(chunk)
        if expanded > max_expanded_layer_bytes:
            raise SystemExit("Docker archive expanded layer exceeds 12 GiB")
        digest.update(chunk)
    return digest.hexdigest(), expanded


with tarfile.open(archive, mode="r:") as bundle:
    members = bundle.getmembers()
    if not members or len(members) > 50000:
        raise SystemExit("invalid Docker archive member count")
    names = set()
    total = 0
    for member in members:
        path = pathlib.PurePosixPath(member.name)
        if member.name in names or path.is_absolute() or ".." in path.parts or "\\" in member.name:
            raise SystemExit("unsafe or duplicate Docker archive member")
        names.add(member.name)
        if not (member.isdir() or member.isreg()):
            raise SystemExit("Docker archive contains a link or special member")
        if member.size < 0 or member.size > 12 * 1024**3:
            raise SystemExit("Docker archive member size is unsafe")
        total += member.size
        if total > 12 * 1024**3:
            raise SystemExit("Docker archive expanded size exceeds 12 GiB")
    try:
        manifest_member = bundle.getmember("manifest.json")
    except KeyError as exc:
        raise SystemExit("Docker archive has no manifest.json") from exc
    manifest_file = bundle.extractfile(manifest_member)
    if manifest_file is None or manifest_member.size > 1024 * 1024:
        raise SystemExit("Docker archive manifest is unreadable or too large")
    manifest = json.load(manifest_file)
    if not isinstance(manifest, list) or len(manifest) != 1:
        raise SystemExit("Docker archive must contain exactly one image")
    entry = manifest[0]
    if not isinstance(entry, dict) or entry.get("RepoTags") != [expected_tag]:
        raise SystemExit("Docker archive contains an unexpected tag")
    config = entry.get("Config")
    layers = entry.get("Layers")
    if (
        not isinstance(config, str)
        or not isinstance(layers, list)
        or not layers
        or any(not isinstance(layer, str) for layer in layers)
    ):
        raise SystemExit("Docker archive manifest fields are invalid")
    config_match = config_path_re.fullmatch(config)
    if config_match is None:
        raise SystemExit("Docker archive config path is invalid")
    config_id = config_match.group(1) or config_match.group(2)
    if not sha256_re.fullmatch(config_id):
        raise SystemExit("Docker archive config digest is invalid")
    for referenced in [config, *layers]:
        if not isinstance(referenced, str) or referenced not in names:
            raise SystemExit("Docker archive manifest references a missing member")
    config_member = bundle.getmember(config)
    config_file = bundle.extractfile(config_member)
    if config_file is None or config_member.size > 16 * 1024 * 1024:
        raise SystemExit("Docker archive config is unreadable or too large")
    config_bytes = config_file.read()
    if hashlib.sha256(config_bytes).hexdigest() != config_id:
        raise SystemExit("Docker archive config digest does not match its contents")
    try:
        image_config = json.loads(config_bytes)
    except (TypeError, ValueError) as exc:
        raise SystemExit("Docker archive config is not valid JSON") from exc
    if not isinstance(image_config, dict):
        raise SystemExit("Docker archive config must be a JSON object")
    rootfs = image_config.get("rootfs")
    diff_ids = rootfs.get("diff_ids") if isinstance(rootfs, dict) else None
    if (
        not isinstance(rootfs, dict)
        or rootfs.get("type") != "layers"
        or not isinstance(diff_ids, list)
        or len(diff_ids) != len(layers)
        or not diff_ids
    ):
        raise SystemExit("Docker archive config rootfs does not match manifest layers")
    for diff_id in diff_ids:
        if not isinstance(diff_id, str) or not re.fullmatch(r"sha256:[0-9a-f]{64}", diff_id):
            raise SystemExit("Docker archive config contains an invalid rootfs diff ID")
    expanded_layers = 0
    for layer_path, expected_diff_id in zip(layers, diff_ids):
        if not (layer_blob_path_re.fullmatch(layer_path) or legacy_layer_path_re.fullmatch(layer_path)):
            raise SystemExit("Docker archive layer path is invalid")
        layer_member = bundle.getmember(layer_path)
        observed_diff_id, expanded = layer_digest(bundle, layer_member)
        expanded_layers += expanded
        if expanded_layers > max_expanded_layer_bytes:
            raise SystemExit("Docker archive expanded layers exceed 12 GiB")
        if observed_diff_id != expected_diff_id.removeprefix("sha256:"):
            raise SystemExit("Docker archive layer digest does not match config rootfs diff ID")
print(total)
PY
}

# Resolve the path only after comparing its lexical canonical form.  A
# different result means the input itself or one of its parent components is
# a symlink; accepting the resolved target would weaken the root-only artifact
# contract and introduce a path-replacement race.
candidate_archive_lexical_path="$(realpath -m -s -- "$candidate_archive_path")" || fail 'cannot normalize candidate image archive path'
candidate_archive_path="$(realpath -e -P -- "$candidate_archive_path")" || fail 'candidate image archive does not exist'
[[ "$candidate_archive_lexical_path" == "$candidate_archive_path" ]] || fail 'candidate image archive path must not contain symbolic links'
case "$candidate_archive_path" in
  /srv/subnexus-migration/candidate-artifacts/*|/root/subnexus-migration/candidate-artifacts/*) ;;
  *) fail 'candidate image archive must be below the approved root-only artifact directory' ;;
esac
candidate_archive_parent="${candidate_archive_path%/*}"
artifact_root=''
case "$candidate_archive_path" in
  /srv/subnexus-migration/candidate-artifacts/*) artifact_root='/srv/subnexus-migration/candidate-artifacts' ;;
  /root/subnexus-migration/candidate-artifacts/*) artifact_root='/root/subnexus-migration/candidate-artifacts' ;;
  *) fail 'candidate image archive must be below the approved root-only artifact directory' ;;
esac
validate_secure_directory "$artifact_root"
validate_secure_directory "$candidate_archive_parent"
assert_path_outside_production_mounts "$candidate_archive_path"
assert_path_outside_production_mounts "$candidate_archive_parent"
artifact_cursor="$candidate_archive_parent"
while [[ "$artifact_cursor" != "$artifact_root" ]]; do
  case "$artifact_cursor" in
    "$artifact_root"/*) ;;
    *) fail 'candidate image archive parent escaped the approved artifact directory' ;;
  esac
  artifact_cursor="${artifact_cursor%/*}"
  [[ -n "$artifact_cursor" && "$artifact_cursor" != / ]] || fail 'candidate image archive parent chain is invalid'
  validate_secure_directory "$artifact_cursor"
done
[[ -f "$candidate_archive_path" && ! -L "$candidate_archive_path" ]] || fail 'candidate image archive must be a regular non-symbolic file'
[[ "$(stat -c '%u' -- "$candidate_archive_path")" == '0' ]] || fail 'candidate image archive must be root-owned'
[[ "$(stat -c '%h' -- "$candidate_archive_path")" == '1' ]] || fail 'candidate image archive must have exactly one hard link'
mode_is_safe "$candidate_archive_path" || fail 'candidate image archive must not be group/other writable'
candidate_archive_size="$(stat -c '%s' -- "$candidate_archive_path")" || fail 'cannot inspect candidate image archive size'
[[ "$candidate_archive_size" =~ ^[0-9]+$ && "$candidate_archive_size" -gt 0 && "$candidate_archive_size" -le 12884901888 ]] || fail 'candidate image archive size is outside the 1-byte to 12-GiB safety range'
observed_archive_sha256="$(sha256sum "$candidate_archive_path" | awk '{print $1}')" || fail 'cannot hash candidate image archive'
[[ "$observed_archive_sha256" == "$candidate_archive_sha256" ]] || fail 'candidate image archive SHA256 does not match the approved value'
candidate_archive_fingerprint="$(stat -Lc '%d|%i|%s|%Y|%u|%g|%a|%h' -- "$candidate_archive_path")" || fail 'cannot fingerprint candidate image archive'
candidate_archive_expanded_size="$(validate_candidate_archive_manifest)" || fail 'candidate image archive manifest validation failed'
[[ "$candidate_archive_expanded_size" =~ ^[0-9]+$ && "$candidate_archive_expanded_size" -gt 0 && "$candidate_archive_expanded_size" -le 12884901888 ]] || fail 'candidate image archive expanded size is invalid'

timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
run_uuid="$(cat /proc/sys/kernel/random/uuid 2>/dev/null)" || fail 'cannot obtain kernel run UUID'
[[ "$run_uuid" =~ ^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$ ]] || fail 'kernel run UUID is invalid'
run_token="${timestamp}-${run_uuid}"
safe_suffix="${timestamp,,}-${run_uuid:0:8}"
network_name="subnexus-candidate-net-${safe_suffix}"
postgres_volume="subnexus-candidate-pg-${safe_suffix}"
redis_volume="subnexus-candidate-redis-${safe_suffix}"
app_volume="subnexus-candidate-app-${safe_suffix}"
postgres_name="subnexus-candidate-postgres-${safe_suffix}"
redis_name="subnexus-candidate-redis-${safe_suffix}"
app_name="subnexus-candidate-app-${safe_suffix}"
create_cidfile="$evidence_root/.${run_token}.cid"
evidence_stage_dir="$evidence_root/.${run_token}.stage"
evidence_final_dir="$evidence_root/$run_token"
evidence_file="$evidence_stage_dir/evidence.txt"
image_load_log_file="$evidence_stage_dir/image-load.log"
base_images_file="$evidence_stage_dir/base-images.txt"

[[ ! -e "$evidence_stage_dir" && ! -L "$evidence_stage_dir" ]] || fail 'candidate evidence stage already exists'
[[ ! -e "$evidence_final_dir" && ! -L "$evidence_final_dir" ]] || fail 'candidate evidence destination already exists'
# Lock the already-validated evidence directory inode.  A separate pathname
# lock file would be replaceable by a symlink between validation and redirection.
evidence_lock_fingerprint="$(stat -Lc '%d|%i|%u|%a' -- "$evidence_root")" ||
  fail 'cannot fingerprint candidate evidence directory for locking'
exec 9<"$evidence_root" || fail 'cannot open candidate evidence directory for locking'
[[ "$(stat -Lc '%d|%i|%u|%a' -- "/proc/$$/fd/9")" == "$evidence_lock_fingerprint" ]] ||
  fail 'candidate evidence directory changed before locking'
flock -n 9 || fail 'another Docker candidate gate is running'
main_bashpid="$BASHPID"

on_signal() {
  interrupted='true'
  failure_reason='gate interrupted by signal'
  exit 130
}
trap on_signal INT TERM HUP

on_exit() {
  local initial_status=$?
  local final_status=1
  if [[ -n "$main_bashpid" && "$BASHPID" != "$main_bashpid" ]]; then
    return "$initial_status"
  fi
  trap - EXIT
  # Ignore second signals while exact candidate cleanup is in progress.
  trap '' INT TERM HUP
  set +e
  [[ -n "$failure_reason" ]] || { [[ "$initial_status" -eq 0 ]] || failure_reason='unhandled gate failure'; }
  if declare -F capture_candidate_summaries >/dev/null 2>&1; then
    capture_candidate_summaries >/dev/null 2>&1 || true
  fi
  if declare -F cleanup_resources >/dev/null 2>&1; then
    cleanup_resources >/dev/null 2>&1 || cleanup_failed='true'
  fi
  if declare -F assert_production_unchanged >/dev/null 2>&1; then
    assert_production_unchanged exit_cleanup >/dev/null 2>&1 || cleanup_failed='true'
  fi
  # Take the final production snapshot only after every candidate object has
  # been removed.  Evidence is published only after this check succeeds.
  if [[ "$initial_status" -eq 0 && "$gate_result" == 'passed' && "$cleanup_failed" == 'false' ]]; then
    assert_production_unchanged 'before_evidence_publish' >/dev/null 2>&1 || cleanup_failed='true'
  fi
  if [[ "$initial_status" -eq 0 && "$gate_result" == 'passed' && "$cleanup_failed" == 'false' && "$interrupted" == 'false' ]]; then
    final_status=0
  fi
  if declare -F write_evidence >/dev/null 2>&1; then
    write_evidence "$([[ "$final_status" -eq 0 ]] && printf passed || printf failed)" || final_status=1
  fi
  [[ "$evidence_publish_failed" == 'false' ]] || final_status=1
  if [[ "$final_status" -eq 0 ]]; then
    printf 'DOCKER_CANDIDATE_GATE=passed\n'
    printf 'EVIDENCE=%s\n' "$evidence_final_dir"
  else
    printf 'DOCKER_CANDIDATE_GATE=failed\n' >&2
    [[ -n "$evidence_final_dir" ]] && printf 'EVIDENCE=%s\n' "$evidence_final_dir" >&2
  fi
  exit "$final_status"
}
trap on_exit EXIT

write_evidence() {
  local result="$1"
  local evidence_tmp checksum_tmp checksum_file artifact
  [[ "$result" == 'passed' || "$result" == 'failed' ]] || {
    evidence_publish_failed='true'
    return 1
  }
  # A preflight failure can happen before the private evidence directory is
  # created.  There is nothing safe to publish in that case.
  [[ -n "$evidence_stage_dir" && -d "$evidence_stage_dir" && ! -L "$evidence_stage_dir" ]] || return 0
  [[ ! -e "$evidence_final_dir" && ! -L "$evidence_final_dir" ]] || {
    evidence_publish_failed='true'
    return 1
  }
  evidence_tmp="$evidence_stage_dir/.evidence.txt.tmp"
  checksum_tmp="$evidence_stage_dir/.evidence.txt.sha256.tmp"
  checksum_file="$evidence_stage_dir/evidence.txt.sha256"
  [[ ! -e "$evidence_tmp" && ! -L "$evidence_tmp" ]] || {
    evidence_publish_failed='true'
    return 1
  }
  evidence_value() {
    printf '%s' "${1:-}" | tr '\r\n' '  '
  }
  {
    printf 'gate=%s\n' "$(evidence_value "$gate_name")"
    printf 'result=%s\n' "$(evidence_value "$result")"
    printf 'run_token=%s\n' "$(evidence_value "$run_token")"
    printf 'approved_commit=%s\n' "$(evidence_value "$approved_sha")"
    printf 'tree=%s\n' "$(evidence_value "$tree_sha")"
    printf 'candidate_archive=%s\n' "$(evidence_value "$candidate_archive_path")"
    printf 'candidate_archive_sha256=%s\n' "$(evidence_value "$candidate_archive_sha256")"
    printf 'candidate_archive_size=%s\n' "$(evidence_value "$candidate_archive_size")"
    printf 'candidate_archive_expanded_size=%s\n' "$(evidence_value "$candidate_archive_expanded_size")"
    printf 'candidate_archive_fingerprint=%s\n' "$(evidence_value "$candidate_archive_fingerprint")"
    printf 'docker_daemon=%s\n' "$(evidence_value "$docker_daemon_summary")"
    printf 'docker_root_dir=%s\n' "$(evidence_value "$docker_root_dir")"
    printf 'docker_free_bytes_before_load=%s\n' "$(evidence_value "$docker_free_bytes_start")"
    printf 'docker_free_bytes_before_candidate_create=%s\n' "$(evidence_value "$docker_free_bytes_before_create")"
    printf 'docker_required_free_bytes=%s\n' "$(evidence_value "$docker_required_free_bytes")"
    printf 'candidate_image_tag=%s\n' "$(evidence_value "$candidate_image_tag")"
    printf 'candidate_image_id=%s\n' "$(evidence_value "$candidate_image_id")"
    printf 'candidate_image_expected_id=%s\n' "$(evidence_value "$expected_candidate_image_id")"
    printf 'candidate_image_preexisting=%s\n' "$(evidence_value "$candidate_image_preexisting")"
    printf 'candidate_image_retained=%s\n' "$(evidence_value "$candidate_image_retained")"
    # Passing this disposable gate never authorizes production cutover.  A
    # maintainer must review the evidence and perform the final switch.
    printf 'manual_cutover_required=true\n'
    printf 'cutover_authorized=false\n'
    printf 'failed_gate_requires_manual_review=true\n'
    printf 'candidate_cleanup_policy=exact-run-resources-only;image-retained\n'
    # A passing runtime gate never authorizes an automatic production switch.
    printf 'cutover_allowed=%s\n' "$(evidence_value "$cutover_allowed")"
    printf 'manual_review_required=%s\n' "$(evidence_value "$manual_review_required")"
    printf 'manual_review=required\n'
    printf 'base_images_file=base-images.txt\n'
    printf 'runtime_network_id=%s\n' "$(evidence_value "$network_id")"
    printf 'candidate_postgres_id=%s\n' "$(evidence_value "$candidate_id_before_cleanup_postgres")"
    printf 'candidate_redis_id=%s\n' "$(evidence_value "$candidate_id_before_cleanup_redis")"
    printf 'candidate_app_id=%s\n' "$(evidence_value "$candidate_id_before_cleanup_app")"
    printf 'candidate_postgres=%s\n' "$(evidence_value "$candidate_summary_postgres")"
    printf 'candidate_redis=%s\n' "$(evidence_value "$candidate_summary_redis")"
    printf 'candidate_app=%s\n' "$(evidence_value "$candidate_summary_app")"
    printf 'candidate_health_postgres=%s\n' "$(evidence_value "$candidate_health_postgres")"
    printf 'candidate_health_redis=%s\n' "$(evidence_value "$candidate_health_redis")"
    printf 'candidate_health_app=%s\n' "$(evidence_value "$candidate_health_app")"
    printf 'candidate_migration_count=%s\n' "$(evidence_value "$candidate_migration_count")"
    printf 'candidate_migration_count_after_restart=%s\n' "$(evidence_value "$candidate_migration_count_after_restart")"
    printf 'candidate_started_at_before_restart=%s\n' "$(evidence_value "$candidate_started_at_before_restart")"
    printf 'candidate_started_at_after_restart=%s\n' "$(evidence_value "$candidate_started_at_after_restart")"
    printf 'candidate_restart_count_before=%s\n' "$(evidence_value "$candidate_restart_count_before")"
    printf 'candidate_restart_count_after=%s\n' "$(evidence_value "$candidate_restart_count_after")"
    printf 'candidate_login_verified=%s\n' "$(evidence_value "$candidate_login_verified")"
    printf 'candidate_public_settings_verified=%s\n' "$(evidence_value "$candidate_public_settings_verified")"
    printf 'candidate_default_settings_verified=%s\n' "$(evidence_value "$candidate_default_settings_verified")"
    printf 'candidate_sentinel_hash=%s\n' "$(evidence_value "$candidate_sentinel_hash")"
    printf 'candidate_sentinel_hash_after_restart=%s\n' "$(evidence_value "$candidate_sentinel_hash_after_restart")"
    printf 'production_app_id=%s\n' "$(evidence_value "${production_id[app]:-}")"
    printf 'production_postgres_id=%s\n' "$(evidence_value "${production_id[postgres]:-}")"
    printf 'production_redis_id=%s\n' "$(evidence_value "${production_id[redis]:-}")"
    printf 'production_app_summary=%s\n' "$(evidence_value "${production_summary[app]:-}")"
    printf 'production_postgres_summary=%s\n' "$(evidence_value "${production_summary[postgres]:-}")"
    printf 'production_redis_summary=%s\n' "$(evidence_value "${production_summary[redis]:-}")"
    printf 'failure_reason=%s\n' "$(evidence_value "$failure_reason")"
    printf 'interrupted=%s\n' "$(evidence_value "$interrupted")"
    printf 'cleanup_failed=%s\n' "$(evidence_value "$cleanup_failed")"
    printf '\nartifact_checksums:\n'
    for artifact in "$image_load_log_file" "$base_images_file"; do
      [[ -f "$artifact" ]] && sha256sum "$artifact"
    done
    if [[ "$result" == 'passed' ]]; then
      printf 'SUBNEXUS_DOCKER_CANDIDATE_GATE=passed\n'
    else
      printf 'SUBNEXUS_DOCKER_CANDIDATE_GATE=failed\n'
    fi
  } >"$evidence_tmp" || {
    evidence_publish_failed='true'
    rm -f -- "$evidence_tmp"
    return 1
  }
  chmod 600 -- "$evidence_tmp" || {
    evidence_publish_failed='true'
    rm -f -- "$evidence_tmp"
    return 1
  }
  mv -- "$evidence_tmp" "$evidence_file" || {
    evidence_publish_failed='true'
    rm -f -- "$evidence_tmp"
    return 1
  }
  chmod 600 -- "$evidence_file" || {
    evidence_publish_failed='true'
    return 1
  }
  sync -f "$evidence_file" || {
    evidence_publish_failed='true'
    return 1
  }
  (
    cd -- "$evidence_stage_dir"
    sha256sum evidence.txt >"${checksum_tmp##*/}"
  ) || {
    evidence_publish_failed='true'
    rm -f -- "$checksum_tmp"
    return 1
  }
  chmod 600 -- "$checksum_tmp" || { evidence_publish_failed='true'; return 1; }
  sync -f "$checksum_tmp" || { evidence_publish_failed='true'; return 1; }
  mv -- "$checksum_tmp" "$checksum_file" || { evidence_publish_failed='true'; return 1; }
  sync -f "$checksum_file" || { evidence_publish_failed='true'; return 1; }
  # The stage directory is private and complete at this point.  Rename it in
  # one operation so readers never observe a partially-written evidence set.
  mv -- "$evidence_stage_dir" "$evidence_final_dir" || {
    evidence_publish_failed='true'
    return 1
  }
  sync -f "$evidence_root" || {
    evidence_publish_failed='true'
    return 1
  }
  return 0
}

mkdir -- "$evidence_stage_dir" || fail 'cannot create candidate evidence stage'
chmod 700 -- "$evidence_stage_dir"

docker_checked() {
  local operation="$1"
  shift
  local operation_phase command_status=0
  operation_phase="${operation//[^A-Za-z0-9_.-]/_}"
  assert_production_unchanged "before_${operation_phase}" || return 1
  if docker_rpc "$@"; then
    command_status=0
  else
    command_status=$?
  fi
  assert_production_unchanged "after_${operation_phase}" || command_status=1
  return "$command_status"
}

assert_exact_absent() {
  local kind="$1"
  local name="$2"
  local listing=''
  case "$kind" in
    container)
      listing="$(docker_rpc container ls --all --filter "name=^/${name}$" --format '{{.ID}}' 2>/dev/null)" || fail 'cannot prove candidate container name is unused'
      ;;
    network)
      listing="$(docker_rpc network ls --filter "name=^${name}$" --format '{{.ID}}' 2>/dev/null)" || fail 'cannot prove candidate network name is unused'
      ;;
    volume)
      listing="$(docker_rpc volume ls --filter "name=^${name}$" --format '{{.Name}}' 2>/dev/null)" || fail 'cannot prove candidate volume name is unused'
      ;;
    image)
      if docker_rpc image inspect "$name" >/dev/null 2>&1; then
        fail "candidate image reference already exists: $name"
      fi
      docker_rpc info --format '{{.ID}}' >/dev/null 2>&1 || fail 'cannot distinguish missing image from Docker daemon failure'
      return 0
      ;;
    *) fail 'unsupported object kind for exact absence check' ;;
  esac
  [[ -z "$listing" ]] || fail "candidate $kind name is already in use: $name"
}

validate_network_labels() {
  local id="$1" name="$2" expected_internal="$3" expected_token="$4"
  local observed_name observed_driver observed_internal observed_attachable observed_ipv6 observed_gate observed_token
  observed_name="$(docker_rpc network inspect --format '{{.Name}}' "$id")" || return 1
  observed_driver="$(docker_rpc network inspect --format '{{.Driver}}' "$id")" || return 1
  observed_internal="$(docker_rpc network inspect --format '{{.Internal}}' "$id")" || return 1
  observed_attachable="$(docker_rpc network inspect --format '{{.Attachable}}' "$id")" || return 1
  observed_ipv6="$(docker_rpc network inspect --format '{{.EnableIPv6}}' "$id")" || return 1
  observed_gate="$(docker_rpc network inspect --format '{{index .Labels "com.subnexus.candidate.gate"}}' "$id")" || return 1
  observed_token="$(docker_rpc network inspect --format '{{index .Labels "com.subnexus.candidate.token"}}' "$id")" || return 1
  [[ "$observed_name" == "$name" && "$observed_driver" == 'bridge' &&
    "$observed_internal" == "$expected_internal" && "$observed_attachable" == 'false' &&
    "$observed_ipv6" == 'false' && "$observed_gate" == "$gate_name" &&
    "$observed_token" == "$expected_token" ]]
}

validate_volume_labels() {
  local volume_name="$1" role="$2"
  local observed_name observed_gate observed_token observed_role driver options mountpoint canonical_mountpoint
  observed_name="$(docker_rpc volume inspect --format '{{.Name}}' "$volume_name")" || return 1
  driver="$(docker_rpc volume inspect --format '{{.Driver}}' "$volume_name")" || return 1
  options="$(docker_rpc volume inspect --format '{{json .Options}}' "$volume_name")" || return 1
  mountpoint="$(docker_rpc volume inspect --format '{{.Mountpoint}}' "$volume_name")" || return 1
  observed_gate="$(docker_rpc volume inspect --format '{{index .Labels "com.subnexus.candidate.gate"}}' "$volume_name")" || return 1
  observed_token="$(docker_rpc volume inspect --format '{{index .Labels "com.subnexus.candidate.token"}}' "$volume_name")" || return 1
  observed_role="$(docker_rpc volume inspect --format '{{index .Labels "com.subnexus.candidate.role"}}' "$volume_name")" || return 1
  [[ "$observed_name" == "$volume_name" && "$driver" == 'local' &&
    ( "$options" == '{}' || "$options" == 'null' ) &&
    "$observed_gate" == "$gate_name" && "$observed_token" == "$run_token" && "$observed_role" == "$role" ]] || return 1
  [[ "$mountpoint" == "$docker_root_dir/volumes/$volume_name/_data" ]] || return 1
  canonical_mountpoint="$(realpath -e -- "$mountpoint")" || return 1
  [[ "$canonical_mountpoint" == "$mountpoint" && -d "$canonical_mountpoint" && ! -L "$canonical_mountpoint" ]] || return 1
  [[ "$canonical_mountpoint" == "$docker_root_dir/volumes/$volume_name/_data" ]] || return 1
}

create_isolated_network() {
  local kind="$1" name="$2" internal="$3" output
  [[ "$kind" == 'runtime' && "$internal" == 'true' ]] || fail 'only an internal runtime candidate network is permitted'
  assert_exact_absent network "$name"
  output="$(docker_checked "network_create_${kind}" network create --driver bridge --internal --ipv6=false \
    --label "com.subnexus.candidate.gate=$gate_name" --label "com.subnexus.candidate.token=$run_token" \
    "$name")" || fail "cannot create candidate $kind network"
  output="$(printf '%s' "$output" | tr -d '\r\n')"
  valid_full_id "$output" || fail "Docker returned an invalid candidate $kind network ID"
  network_id="$output"
  validate_network_labels "$output" "$name" "$internal" "$run_token" || fail "candidate $kind network options or labels are invalid"
}

create_isolated_volume() {
  local role="$1" volume_name="$2" output
  assert_exact_absent volume "$volume_name"
  output="$(docker_checked "volume_create_${role}" volume create --name "$volume_name" \
    --label "com.subnexus.candidate.gate=$gate_name" --label "com.subnexus.candidate.token=$run_token" \
    --label "com.subnexus.candidate.role=$role")" || fail "cannot create candidate $role volume"
  output="$(printf '%s' "$output" | tr -d '\r\n')"
  [[ "$output" == "$volume_name" ]] || fail "Docker returned an unexpected candidate $role volume name"
  validate_volume_labels "$volume_name" "$role" || fail "candidate $role volume labels are invalid"
}

assert_network_not_production() {
  local candidate_id="$1" role production_network_ids
  for role in app postgres redis; do
    production_network_ids="$(docker_rpc inspect --format '{{range $name, $network := .NetworkSettings.Networks}}{{println $network.NetworkID}}{{end}}' "${production_id[$role]}")" || return 1
    if printf '%s\n' "$production_network_ids" | grep -Fqx "$candidate_id"; then
      return 1
    fi
  done
  return 0
}

validate_runtime_base_image() {
  local role="$1" ref="$2" expected_id="$3"
  local observed_id os architecture
  observed_id="$(capture_image_id "$ref")" || fail "cannot resolve candidate $role image ID"
  [[ "$observed_id" == "$expected_id" ]] || fail "candidate $role image ID changed during validation"
  os="$(docker_rpc image inspect --format '{{.Os}}' "sha256:$observed_id")" || fail "cannot inspect candidate $role image OS"
  architecture="$(docker_rpc image inspect --format '{{.Architecture}}' "sha256:$observed_id")" || fail "cannot inspect candidate $role image architecture"
  [[ "$os" == 'linux' && "$architecture" == 'amd64' ]] || fail "candidate $role image must be linux/amd64"
  printf '%s=%s|sha256:%s|%s/%s\n' "$role" "$ref" "$observed_id" "$os" "$architecture" >>"$base_images_file"
}

assert_docker_free_space() {
  local phase="$1"
  local available required archive_budget
  [[ "$candidate_archive_size" =~ ^[0-9]+$ ]] || fail 'candidate archive size is unavailable for disk-space check'
  [[ "$candidate_archive_expanded_size" =~ ^[0-9]+$ ]] || fail 'candidate archive expanded size is unavailable for disk-space check'
  available="$(df -P -B1 -- "$docker_root_dir" | awk 'NR == 2 {print $4}')" || fail 'cannot inspect Docker storage free space'
  [[ "$available" =~ ^[0-9]+$ ]] || fail 'Docker storage free space is not numeric'
  archive_budget="$candidate_archive_size"
  (( candidate_archive_expanded_size > archive_budget )) && archive_budget="$candidate_archive_expanded_size"
  required=$((archive_budget * docker_archive_multiplier + docker_free_reserve_bytes))
  (( required < minimum_docker_free_bytes )) && required=$minimum_docker_free_bytes
  docker_required_free_bytes="$required"
  case "$phase" in
    before_load) docker_free_bytes_start="$available" ;;
    before_create) docker_free_bytes_before_create="$available" ;;
  esac
  (( available >= required )) || fail "Docker storage has insufficient free space for candidate ($available < $required bytes)"
}

assert_candidate_archive_unchanged() {
  local current_fingerprint current_sha256
  current_fingerprint="$(stat -Lc '%d|%i|%s|%Y|%u|%g|%a|%h' -- "$candidate_archive_path")" || fail 'candidate image archive cannot be fingerprinted before load'
  [[ "$current_fingerprint" == "$candidate_archive_fingerprint" ]] || fail 'candidate image archive changed after approval'
  current_sha256="$(sha256sum "$candidate_archive_path" | awk '{print $1}')" || fail 'candidate image archive cannot be hashed before load'
  [[ "$current_sha256" == "$candidate_archive_sha256" ]] || fail 'candidate image archive hash changed after approval'
}

prepare_candidate_runtime_images() {
  pg_build_ref="$(resolve_image_ref "$pg_image_ref")"
  redis_build_ref="$(resolve_image_ref "$redis_image_ref")"
  postgres_runtime_image_id="$(capture_image_id "$pg_build_ref")" || fail 'cannot resolve candidate PostgreSQL image ID'
  redis_runtime_image_id="$(capture_image_id "$redis_build_ref")" || fail 'cannot resolve candidate Redis image ID'
  : >"$base_images_file"
  chmod 600 -- "$base_images_file"
  validate_runtime_base_image postgres "$pg_build_ref" "$postgres_runtime_image_id"
  validate_runtime_base_image redis "$redis_build_ref" "$redis_runtime_image_id"
  sync -f "$base_images_file" || fail 'cannot sync candidate base-image evidence'
}

load_candidate_image() {
  local load_status=0 observed_id='' absence_status=0
  # The archive was validated before the lifecycle starts. Re-check its exact
  # inode/hash immediately before either reusing a preloaded tag or loading it,
  # so a replaced archive cannot be paired with an approved image ID.
  assert_candidate_archive_unchanged
  if observed_id="$(capture_image_id "$candidate_image_tag" 2>/dev/null)"; then
    candidate_image_preexisting='true'
    printf 'candidate image tag already loaded; exact approved ID will be reused\n' >"$image_load_log_file"
  else
    if object_absent image "$candidate_image_tag"; then
      :
    else
      absence_status=$?
      if [[ "$absence_status" -eq 1 ]]; then
        fail 'candidate image tag exists but its ID cannot be inspected'
      fi
      fail 'cannot distinguish an absent candidate tag from Docker daemon failure'
    fi
    candidate_image_preexisting='false'
    set +e
    docker_checked candidate_image_load image load --input "$candidate_archive_path" >"$image_load_log_file" 2>&1
    load_status=$?
    set -e
    assert_candidate_archive_unchanged || load_status=1
    observed_id="$(capture_image_id "$candidate_image_tag" 2>/dev/null || true)"
    (( load_status == 0 )) || fail 'candidate image archive failed to load'
  fi
  chmod 600 -- "$image_load_log_file" || fail 'cannot protect candidate image load log'
  sync -f "$image_load_log_file" || fail 'cannot sync candidate image load log'
  valid_full_id "$observed_id" || fail 'loaded candidate tag did not resolve to a full image ID'
  [[ "$observed_id" == "$expected_candidate_image_id" ]] || fail 'loaded candidate image ID does not match the approved artifact manifest'
  candidate_image_id="$observed_id"
  candidate_image_retained='true'
}

json_is_empty_list_or_null() {
  case "$1" in
    ''|'null'|'[]'|'{}'|'map[]') return 0 ;;
    *) return 1 ;;
  esac
}

# The official PostgreSQL/Redis entrypoints and the Sub2API entrypoint start
# as root, repair the ownership of their named volume, and then call
# su-exec/gosu to become their unprivileged service user.  Keep that bootstrap
# path working with an explicit, role-scoped capability allowlist; any
# capability not listed here is still dropped by --cap-drop ALL.
runtime_cap_add_matches() {
  local role="$1"
  local raw="$2"
  local token expected found
  local -a expected_caps=() actual_caps=()
  case "$role" in
    app|redis)
      expected_caps=(CHOWN SETGID SETUID)
      ;;
    postgres)
      # PostgreSQL's image also chmods/enters an existing data directory.
      expected_caps=(CHOWN DAC_OVERRIDE FOWNER SETGID SETUID)
      ;;
    *)
      return 1
      ;;
  esac
  [[ "$raw" == \[*\] ]] || return 1
  raw="${raw#[}"
  raw="${raw%]}"
  [[ -n "$raw" ]] || return 1
  IFS=',' read -r -a actual_caps <<< "$raw"
  [[ "${#actual_caps[@]}" -eq "${#expected_caps[@]}" ]] || return 1
  for token in "${actual_caps[@]}"; do
    token="${token//\"/}"
    token="${token//[[:space:]]/}"
    [[ -n "$token" ]] || return 1
    found='false'
    for expected in "${expected_caps[@]}"; do
      [[ "$token" == "$expected" ]] && found='true'
    done
    [[ "$found" == 'true' ]] || return 1
  done
  # Require every expected capability exactly once. This rejects duplicate
  # entries that could otherwise hide a missing capability behind the length
  # check above.
  for expected in "${expected_caps[@]}"; do
    local occurrences=0
    for token in "${actual_caps[@]}"; do
      token="${token//\"/}"
      token="${token//[[:space:]]/}"
      [[ "$token" == "$expected" ]] && occurrences=$((occurrences + 1))
    done
    [[ "$occurrences" -eq 1 ]] || return 1
  done
  # The length check plus the duplicate check above makes this an exact set,
  # independent of the order Docker uses when serializing CapAdd.
  return 0
}

runtime_cap_drop_matches() {
  local raw="$1"
  [[ "$raw" == '["ALL"]' || "$raw" == '["all"]' ]]
}

runtime_security_opt_matches() {
  local raw="$1"
  [[ "$raw" == '["no-new-privileges:true"]' ]]
}

candidate_mounts() {
  local container_id="$1"
  docker_rpc inspect --format '{{range .Mounts}}{{printf "%s|%s|%s|%s|%s\n" .Type .Name .Destination .RW .Source}}{{end}}' "$container_id"
}

validate_candidate_tmpfs() {
  local role="$1" tmpfs_json="$2"
  printf '%s' "$tmpfs_json" | python3 -c '
import json, sys
role = sys.argv[1]
data = json.load(sys.stdin)
expected = {
    "app": {"/tmp": {"64m", "67108864"}},
    "redis": {"/tmp": {"32m", "33554432"}},
    "postgres": {
        "/tmp": {"64m", "67108864"},
        "/run/postgresql": {"16m", "16777216"},
    },
}[role]
if set(data) != set(expected):
    raise SystemExit(1)
for path, allowed_sizes in expected.items():
    if not isinstance(data[path], str):
        raise SystemExit(1)
    parts = [part.strip() for part in data[path].split(",")]
    if any(not part for part in parts) or len(parts) != len(set(parts)):
        raise SystemExit(1)
    options = set(parts)
    if not any(options == {"rw", "nosuid", "nodev", "noexec", "size=" + size} for size in allowed_sizes):
        raise SystemExit(1)
' "$role"
}

validate_candidate_container() {
  local container_id="$1"
  local role="$2"
  local expected_image_id="$3"
  local expected_volume="$4"
  local expected_destination="$5"
  local expected_user="$6"
  local expected_running="$7"
  local expected_memory expected_swap expected_nano expected_pids
  local image_id config_user memory memory_swap nano_cpus pids_limit network_mode expected_name
  local privileged pid_mode ipc_mode uts_mode cgroupns_mode userns_mode readonly_rootfs restart_policy log_driver
  local publish_all port_bindings cap_add cap_drop security_opt devices device_requests volumes_from binds shm_size labels name gate_label token_label role_label tmpfs_json network_names network_ids network_count
  local mount_line mount_type mount_name mount_destination mount_rw mount_source mount_count=0
  case "$role" in
    app) expected_memory=805306368; expected_swap=805306368; expected_nano=1000000000; expected_pids=256; expected_name="$app_name" ;;
    postgres) expected_memory=805306368; expected_swap=805306368; expected_nano=1000000000; expected_pids=256; expected_name="$postgres_name" ;;
    redis) expected_memory=268435456; expected_swap=268435456; expected_nano=500000000; expected_pids=128; expected_name="$redis_name" ;;
    *) return 1 ;;
  esac
  valid_full_id "$container_id" || return 1
  name="$(docker_rpc inspect --format '{{.Name}}' "$container_id")" || return 1
  image_id="$(docker_rpc inspect --format '{{.Image}}' "$container_id")" || return 1
  config_user="$(docker_rpc inspect --format '{{.Config.User}}' "$container_id")" || return 1
  memory="$(docker_rpc inspect --format '{{.HostConfig.Memory}}' "$container_id")" || return 1
  memory_swap="$(docker_rpc inspect --format '{{.HostConfig.MemorySwap}}' "$container_id")" || return 1
  nano_cpus="$(docker_rpc inspect --format '{{.HostConfig.NanoCpus}}' "$container_id")" || return 1
  pids_limit="$(docker_rpc inspect --format '{{.HostConfig.PidsLimit}}' "$container_id")" || return 1
  network_mode="$(docker_rpc inspect --format '{{.HostConfig.NetworkMode}}' "$container_id")" || return 1
  privileged="$(docker_rpc inspect --format '{{.HostConfig.Privileged}}' "$container_id")" || return 1
  pid_mode="$(docker_rpc inspect --format '{{.HostConfig.PidMode}}' "$container_id")" || return 1
  ipc_mode="$(docker_rpc inspect --format '{{.HostConfig.IpcMode}}' "$container_id")" || return 1
  uts_mode="$(docker_rpc inspect --format '{{.HostConfig.UTSMode}}' "$container_id")" || return 1
  cgroupns_mode="$(docker_rpc inspect --format '{{.HostConfig.CgroupnsMode}}' "$container_id")" || return 1
  userns_mode="$(docker_rpc inspect --format '{{.HostConfig.UsernsMode}}' "$container_id")" || return 1
  readonly_rootfs="$(docker_rpc inspect --format '{{.HostConfig.ReadonlyRootfs}}' "$container_id")" || return 1
  restart_policy="$(docker_rpc inspect --format '{{.HostConfig.RestartPolicy.Name}}' "$container_id")" || return 1
  log_driver="$(docker_rpc inspect --format '{{.HostConfig.LogConfig.Type}}' "$container_id")" || return 1
  publish_all="$(docker_rpc inspect --format '{{.HostConfig.PublishAllPorts}}' "$container_id")" || return 1
  port_bindings="$(docker_rpc inspect --format '{{json .HostConfig.PortBindings}}' "$container_id")" || return 1
  cap_add="$(docker_rpc inspect --format '{{json .HostConfig.CapAdd}}' "$container_id")" || return 1
  cap_drop="$(docker_rpc inspect --format '{{json .HostConfig.CapDrop}}' "$container_id")" || return 1
  security_opt="$(docker_rpc inspect --format '{{json .HostConfig.SecurityOpt}}' "$container_id")" || return 1
  devices="$(docker_rpc inspect --format '{{json .HostConfig.Devices}}' "$container_id")" || return 1
  device_requests="$(docker_rpc inspect --format '{{json .HostConfig.DeviceRequests}}' "$container_id")" || return 1
  volumes_from="$(docker_rpc inspect --format '{{json .HostConfig.VolumesFrom}}' "$container_id")" || return 1
  binds="$(docker_rpc inspect --format '{{json .HostConfig.Binds}}' "$container_id")" || return 1
  shm_size="$(docker_rpc inspect --format '{{.HostConfig.ShmSize}}' "$container_id")" || return 1
  labels="$(docker_rpc inspect --format '{{json .Config.Labels}}' "$container_id")" || return 1
  gate_label="$(docker_rpc inspect --format '{{index .Config.Labels "com.subnexus.candidate.gate"}}' "$container_id")" || return 1
  token_label="$(docker_rpc inspect --format '{{index .Config.Labels "com.subnexus.candidate.token"}}' "$container_id")" || return 1
  role_label="$(docker_rpc inspect --format '{{index .Config.Labels "com.subnexus.candidate.role"}}' "$container_id")" || return 1
  tmpfs_json="$(docker_rpc inspect --format '{{json .HostConfig.Tmpfs}}' "$container_id")" || return 1
  network_names="$(docker_rpc inspect --format '{{range $name, $_ := .NetworkSettings.Networks}}{{println $name}}{{end}}' "$container_id")" || return 1
  network_ids="$(docker_rpc inspect --format '{{range $name, $network := .NetworkSettings.Networks}}{{printf "%s|%s\n" $name $network.NetworkID}}{{end}}' "$container_id")" || return 1
  network_count="$(printf '%s\n' "$network_names" | awk 'NF {count++} END {print count+0}')"
  [[ "$name" == "/$expected_name" && "$gate_label" == "$gate_name" && "$token_label" == "$run_token" && "$role_label" == "$role" ]] || return 1
  [[ "$image_id" == "sha256:$expected_image_id" && "$memory" == "$expected_memory" &&
    "$memory_swap" == "$expected_swap" && "$nano_cpus" == "$expected_nano" &&
    "$pids_limit" == "$expected_pids" ]] || return 1
  if [[ "$role" == 'app' && -n "$expected_user" ]]; then
    [[ "$config_user" == "$expected_user" ]] || return 1
  fi
  [[ "$network_mode" == "$network_name" || "$network_mode" == "$network_id" ]] || return 1
  [[ "$network_count" == '1' && "$network_names" == "$network_name" && "$network_ids" == "$network_name|$network_id" ]] || return 1
  assert_network_not_production "$network_id" || return 1
  [[ "$privileged" == 'false' && -z "$pid_mode" && "$ipc_mode" == 'private' && -z "$uts_mode" &&
    "$cgroupns_mode" == 'private' && -z "$userns_mode" &&
    "$readonly_rootfs" == 'true' && "$restart_policy" == 'no' && "$log_driver" == 'none' &&
    "$publish_all" == 'false' ]] || return 1
  json_is_empty_list_or_null "$port_bindings" || return 1
  runtime_cap_add_matches "$role" "$cap_add" || return 1
  runtime_cap_drop_matches "$cap_drop" || return 1
  runtime_security_opt_matches "$security_opt" || return 1
  json_is_empty_list_or_null "$devices" || return 1
  json_is_empty_list_or_null "$device_requests" || return 1
  json_is_empty_list_or_null "$volumes_from" || return 1
  json_is_empty_list_or_null "$binds" || return 1
  [[ "$shm_size" =~ ^[0-9]+$ && "$shm_size" -le 134217728 ]] || return 1
  [[ "$labels" == *'com.subnexus.candidate.gate'* && "$labels" == *'com.subnexus.candidate.token'* ]] || return 1
  [[ "$(docker_rpc inspect --format '{{index .Config.Labels "com.subnexus.candidate.token"}}' "$container_id")" == "$run_token" ]] || return 1
  [[ "$(docker_rpc inspect --format '{{.State.Running}}' "$container_id")" == "$expected_running" ]] || return 1
  validate_candidate_tmpfs "$role" "$tmpfs_json" || return 1
  validate_volume_labels "$expected_volume" "$role" || return 1
  while IFS='|' read -r mount_type mount_name mount_destination mount_rw mount_source; do
    [[ -n "$mount_type" ]] || continue
    if [[ "$mount_type" == 'volume' ]]; then
      mount_count=$((mount_count + 1))
      [[ "$mount_name" == "$expected_volume" && "$mount_destination" == "$expected_destination" &&
        "$mount_rw" == 'true' && -n "$mount_source" ]] || return 1
    elif [[ "$mount_type" == 'tmpfs' ]]; then
      [[ "$mount_rw" == 'true' && -z "$mount_name" ]] || return 1
      case "$role:$mount_destination" in
        app:/tmp|redis:/tmp|postgres:/tmp|postgres:/run/postgresql) ;;
        *) return 1 ;;
      esac
    else
      return 1
    fi
  done < <(candidate_mounts "$container_id")
  [[ "$mount_count" -eq 1 ]] || return 1
}

validate_network_members() {
  local observed expected
  observed="$(docker_checked candidate_network_members network inspect --format '{{range $containerID, $container := .Containers}}{{println $containerID}}{{end}}' "$network_id" | sort)" || return 1
  expected="$(printf '%s\n' "$postgres_id" "$redis_id" "$app_id" | sort)"
  [[ -n "$observed" && "$observed" == "$expected" ]]
}

assert_candidate_env() {
  local key raw env_lines count path_value
  declare -A expected=(
    [AUTO_SETUP]=true [SERVER_HOST]=0.0.0.0 [SERVER_PORT]=8080 [SERVER_MODE]=release [RUN_MODE]=standard
    [DATABASE_HOST]=postgres-candidate [DATABASE_PORT]=5432 [DATABASE_USER]="$pg_user" [DATABASE_DBNAME]="$pg_database" [DATABASE_SSLMODE]=disable
    [REDIS_HOST]=redis-candidate [REDIS_PORT]=6379 [REDIS_DB]=0 [REDIS_ENABLE_TLS]=false [ADMIN_EMAIL]=gate-admin@example.invalid [DATA_DIR]=/app/data
    [HTTP_PROXY]='' [HTTPS_PROXY]='' [ALL_PROXY]='' [NO_PROXY]='*' [UPDATE_GITHUB_TOKEN]=''
    [DATABASE_PASSWORD]="$pg_password" [REDIS_PASSWORD]="$redis_password" [ADMIN_PASSWORD]="$admin_password"
    [JWT_SECRET]="$jwt_secret" [TOTP_ENCRYPTION_KEY]="$totp_key"
    [CHANNEL_MONITOR_V2_DISABLE_AGGREGATOR]=1 [OPS_ENABLED]=false [OPS_CLEANUP_ENABLED]=false
    [OPS_AGGREGATION_ENABLED]=false [OPS_METRICS_COLLECTOR_CACHE_ENABLED]=false
    [DASHBOARD_AGGREGATION_ENABLED]=false [USAGE_CLEANUP_ENABLED]=false
    [DATABASE_USER_PLATFORM_QUOTA_FLUSHER_ENABLED]=false [BATCH_IMAGE_ENABLED]=false
    [BATCH_IMAGE_QUEUE_ENABLED]=false [BATCH_IMAGE_VERTEX_ENABLED]=false
    [IMAGE_STORAGE_ENABLED]=false [TURNSTILE_REQUIRED]=false [WEBAUTHN_ENABLED]=false
  )
  declare -A allowed=()
  for key in "${!expected[@]}"; do allowed["$key"]='1'; done
  allowed[PATH]='1'
  env_lines="$(docker_rpc inspect --format '{{range .Config.Env}}{{println .}}{{end}}' "$app_id")" || return 1
  while IFS= read -r raw; do
    [[ -n "$raw" ]] || continue
    key="${raw%%=*}"
    [[ "$key" != "$raw" && "$key" =~ ^[A-Za-z_][A-Za-z0-9_]*$ && -n "${allowed[$key]+x}" ]] || return 1
  done <<< "$env_lines"
  count="$(printf '%s\n' "$env_lines" | awk -F= 'NF {seen[$1]++; total++} END {for (key in seen) if (seen[key] != 1) exit 1; print total+0}')" || return 1
  [[ "$count" =~ ^[0-9]+$ ]] || return 1
  for key in "${!expected[@]}"; do
    count="$(printf '%s\n' "$env_lines" | awk -F= -v wanted="$key" '$1 == wanted {count++} END {print count+0}')"
    [[ "$count" == '1' ]] || return 1
    raw="$(printf '%s\n' "$env_lines" | awk -F= -v wanted="$key" '$1 == wanted {print substr($0, length(wanted) + 2)}')"
    [[ "$raw" == "${expected[$key]}" ]] || return 1
  done
  path_value="$(printf '%s\n' "$env_lines" | awk -F= '$1 == "PATH" {print substr($0, 6)}')"
  [[ -z "$path_value" || "$path_value" == '/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin' ]] || return 1
  for key in DATABASE_PASSWORD REDIS_PASSWORD ADMIN_PASSWORD JWT_SECRET TOTP_ENCRYPTION_KEY; do
    count="$(printf '%s\n' "$env_lines" | awk -F= -v wanted="$key" '$1 == wanted {count++} END {print count+0}')"
    [[ "$count" == '1' ]] || return 1
    raw="$(printf '%s\n' "$env_lines" | awk -F= -v wanted="$key" '$1 == wanted {print substr($0, length(wanted) + 2)}')"
    [[ "$raw" == "${expected[$key]}" && -n "$raw" ]] || return 1
  done
  [[ "$env_lines" != *"${production_id[app]}"* && "$env_lines" != *'/var/run/docker.sock'* && "$env_lines" != *'/run/docker.sock'* ]] || return 1
}

capture_pid1_status() {
  local container_id="$1"
  docker_rpc exec --user 0:0 "$container_id" /bin/sh -c 'awk "/^Uid:|^NoNewPrivs:|^CapEff:/{print}" /proc/1/status'
}

validate_pid1_security() {
  local container_id="$1"
  local expected_uid="${2:-}"
  local status uid_line nnp_line cap_eff_line uid_value nnp_value cap_eff_value
  status="$(capture_pid1_status "$container_id")" || return 1
  uid_line="$(printf '%s\n' "$status" | awk '$1 == "Uid:" {print; exit}')"
  nnp_line="$(printf '%s\n' "$status" | awk '$1 == "NoNewPrivs:" {print; exit}')"
  cap_eff_line="$(printf '%s\n' "$status" | awk '$1 == "CapEff:" {print; exit}')"
  uid_value="$(printf '%s\n' "$uid_line" | awk '{print $2}')"
  nnp_value="$(printf '%s\n' "$nnp_line" | awk '{print $2}')"
  cap_eff_value="$(printf '%s\n' "$cap_eff_line" | awk '{print $2}')"
  [[ "$uid_value" =~ ^[1-9][0-9]*$ ]] || return 1
  [[ -z "$expected_uid" || "$uid_value" == "$expected_uid" ]] || return 1
  [[ "$nnp_value" == '1' && "$cap_eff_value" =~ ^0+$ ]]
}

random_hex_32() {
  local first second
  first="$(cat /proc/sys/kernel/random/uuid 2>/dev/null)" || return 1
  second="$(cat /proc/sys/kernel/random/uuid 2>/dev/null)" || return 1
  first="${first//-/}"
  second="${second//-/}"
  printf '%s%s' "$first" "$second"
}

create_candidate_container() {
  local role="$1"
  local container_name="$2"
  local volume_name="$3"
  local destination="$4"
  local expected_user="$5"
  local expected_image_id="$6"
  shift 6
  local output='' cid_path cid_id='' create_status=0
  assert_exact_absent container "$container_name"
  cid_path="${create_cidfile}.${role}"
  [[ "$cid_path" == "$evidence_root/.${run_token}.cid.$role" ]] || fail 'candidate cidfile path escaped the approved evidence root'
  [[ ! -e "$cid_path" && ! -L "$cid_path" ]] || fail "candidate $role cidfile already exists"
  set +e
  output="$(docker_checked "container_create_${role}" create --cidfile "$cid_path" "$@" 2>/dev/null)"
  create_status=$?
  set -e
  if [[ -f "$cid_path" && ! -L "$cid_path" ]]; then
    [[ "$(stat -c '%u' -- "$cid_path")" == '0' && "$(stat -c '%h' -- "$cid_path")" == '1' ]] ||
      fail "candidate $role cidfile ownership/link count is unsafe"
    chmod 600 -- "$cid_path"
    cid_id="$(tr -d '\r\n' <"$cid_path")"
  fi
  output="$(printf '%s' "$output" | tr -d '\r\n')"
  if valid_full_id "$cid_id"; then
    case "$role" in
      postgres) postgres_id="$cid_id" ;;
      redis) redis_id="$cid_id" ;;
      app) app_id="$cid_id" ;;
    esac
  fi
  (( create_status == 0 )) || fail "cannot create candidate $role container"
  valid_full_id "$output" || fail "Docker returned an invalid candidate $role container ID"
  [[ "$cid_id" == "$output" ]] || fail "candidate $role stdout/cidfile identities differ"
  case "$role" in
    postgres) postgres_id="$cid_id" ;;
    redis) redis_id="$cid_id" ;;
    app) app_id="$cid_id" ;;
    *) fail 'unknown candidate container role' ;;
  esac
  validate_candidate_container "$output" "$role" "$expected_image_id" "$volume_name" "$destination" "$expected_user" false ||
    fail "candidate $role container failed pre-start isolation validation"
}

create_candidate_containers() {
  create_isolated_volume postgres "$postgres_volume"
  create_isolated_volume redis "$redis_volume"
  create_isolated_volume app "$app_volume"

  pg_password="$(cat /proc/sys/kernel/random/uuid)$(cat /proc/sys/kernel/random/uuid)"
  redis_password="$(cat /proc/sys/kernel/random/uuid)$(cat /proc/sys/kernel/random/uuid)"
  admin_password="$(cat /proc/sys/kernel/random/uuid)$(cat /proc/sys/kernel/random/uuid)"
  jwt_secret="$(cat /proc/sys/kernel/random/uuid)$(cat /proc/sys/kernel/random/uuid)"
  totp_key="$(random_hex_32)" || fail 'cannot generate candidate TOTP encryption key'
  [[ "$totp_key" =~ ^[0-9a-f]{64}$ ]] || fail 'candidate TOTP encryption key must be exactly 64 lowercase hexadecimal characters'

  create_candidate_container postgres "$postgres_name" "$postgres_volume" /var/lib/postgresql/data '' "$postgres_runtime_image_id" \
    --name "$postgres_name" \
    --pull never \
    --label "com.subnexus.candidate.gate=$gate_name" --label "com.subnexus.candidate.token=$run_token" --label 'com.subnexus.candidate.role=postgres' \
    --network "$network_name" --network-alias postgres-candidate --restart no \
    --memory 768m --memory-swap 768m --cpus 1 --pids-limit 256 \
    --read-only --cap-drop ALL --cap-add CHOWN --cap-add DAC_OVERRIDE --cap-add FOWNER --cap-add SETGID --cap-add SETUID \
    --security-opt no-new-privileges:true --cgroupns private --ipc private --shm-size 64m --log-driver none \
    --tmpfs '/tmp:rw,nosuid,nodev,noexec,size=64m' --tmpfs '/run/postgresql:rw,nosuid,nodev,noexec,size=16m' \
    --mount "type=volume,src=$postgres_volume,dst=/var/lib/postgresql/data" \
    --env "POSTGRES_USER=$pg_user" --env "POSTGRES_PASSWORD=$pg_password" --env "POSTGRES_DB=$pg_database" \
    --env 'PGDATA=/var/lib/postgresql/data/pgdata' --env 'TZ=UTC' \
    --health-cmd "pg_isready -U $pg_user -d $pg_database" --health-interval 5s --health-timeout 3s --health-retries 30 --health-start-period 5s \
    "sha256:$postgres_runtime_image_id"

  create_candidate_container redis "$redis_name" "$redis_volume" /data '' "$redis_runtime_image_id" \
    --name "$redis_name" \
    --pull never \
    --label "com.subnexus.candidate.gate=$gate_name" --label "com.subnexus.candidate.token=$run_token" --label 'com.subnexus.candidate.role=redis' \
    --network "$network_name" --network-alias redis-candidate --restart no \
    --memory 256m --memory-swap 256m --cpus 0.5 --pids-limit 128 \
    --read-only --cap-drop ALL --cap-add CHOWN --cap-add SETGID --cap-add SETUID \
    --security-opt no-new-privileges:true --cgroupns private --ipc private --log-driver none \
    --tmpfs '/tmp:rw,nosuid,nodev,noexec,size=32m' \
    --mount "type=volume,src=$redis_volume,dst=/data" \
    --env "REDISCLI_AUTH=$redis_password" --env 'TZ=UTC' \
    --health-cmd 'redis-cli --no-auth-warning ping' --health-interval 5s --health-timeout 3s --health-retries 30 --health-start-period 3s \
    "sha256:$redis_runtime_image_id" redis-server --appendonly no --save '' --protected-mode yes --bind 0.0.0.0 --port 6379 --requirepass "$redis_password"

  create_candidate_container app "$app_name" "$app_volume" /app/data '' "$candidate_image_id" \
    --name "$app_name" \
    --pull never \
    --label "com.subnexus.candidate.gate=$gate_name" --label "com.subnexus.candidate.token=$run_token" --label 'com.subnexus.candidate.role=app' \
    --network "$network_name" --network-alias app-candidate --restart no \
    --memory 768m --memory-swap 768m --cpus 1 --pids-limit 256 \
    --read-only --cap-drop ALL --cap-add CHOWN --cap-add SETGID --cap-add SETUID \
    --security-opt no-new-privileges:true --cgroupns private --ipc private --shm-size 64m --log-driver none \
    --tmpfs '/tmp:rw,nosuid,nodev,noexec,size=64m' \
    --mount "type=volume,src=$app_volume,dst=/app/data" \
    --env AUTO_SETUP=true --env SERVER_HOST=0.0.0.0 --env SERVER_PORT=8080 --env SERVER_MODE=release --env RUN_MODE=standard \
    --env DATABASE_HOST=postgres-candidate --env DATABASE_PORT=5432 --env DATABASE_USER="$pg_user" --env DATABASE_PASSWORD="$pg_password" --env DATABASE_DBNAME="$pg_database" --env DATABASE_SSLMODE=disable \
    --env REDIS_HOST=redis-candidate --env REDIS_PORT=6379 --env REDIS_PASSWORD="$redis_password" --env REDIS_DB=0 --env REDIS_ENABLE_TLS=false \
    --env ADMIN_EMAIL=gate-admin@example.invalid --env ADMIN_PASSWORD="$admin_password" --env JWT_SECRET="$jwt_secret" --env TOTP_ENCRYPTION_KEY="$totp_key" --env DATA_DIR=/app/data \
    --env HTTP_PROXY= --env HTTPS_PROXY= --env ALL_PROXY= --env NO_PROXY='*' --env UPDATE_GITHUB_TOKEN= \
    --env CHANNEL_MONITOR_V2_DISABLE_AGGREGATOR=1 --env OPS_ENABLED=false --env OPS_CLEANUP_ENABLED=false \
    --env OPS_AGGREGATION_ENABLED=false --env OPS_METRICS_COLLECTOR_CACHE_ENABLED=false \
    --env DASHBOARD_AGGREGATION_ENABLED=false --env USAGE_CLEANUP_ENABLED=false \
    --env DATABASE_USER_PLATFORM_QUOTA_FLUSHER_ENABLED=false --env BATCH_IMAGE_ENABLED=false \
    --env BATCH_IMAGE_QUEUE_ENABLED=false --env BATCH_IMAGE_VERTEX_ENABLED=false \
    --env IMAGE_STORAGE_ENABLED=false --env TURNSTILE_REQUIRED=false --env WEBAUTHN_ENABLED=false \
    --health-cmd 'wget -q -T 5 -O /dev/null http://127.0.0.1:8080/health' --health-interval 5s --health-timeout 5s --health-retries 30 --health-start-period 20s \
    "sha256:$candidate_image_id"

  validate_candidate_container "$postgres_id" postgres "$postgres_runtime_image_id" "$postgres_volume" /var/lib/postgresql/data '' false || fail 'candidate PostgreSQL isolation check failed'
  validate_candidate_container "$redis_id" redis "$redis_runtime_image_id" "$redis_volume" /data '' false || fail 'candidate Redis isolation check failed'
  validate_candidate_container "$app_id" app "$candidate_image_id" "$app_volume" /app/data '' false || fail 'candidate application isolation check failed'
  assert_candidate_env || fail 'candidate application environment is not fail-closed'
  validate_network_members || fail 'candidate network members are not exactly the three candidate containers'
}

candidate_inspect() {
  local operation="$1"
  local container_id="$2"
  local format="$3"
  docker_checked "$operation" inspect --format "$format" "$container_id"
}

candidate_health_status() {
  local role="$1"
  local container_id="$2"
  candidate_inspect "health_${role}" "$container_id" '{{if .Config.Healthcheck}}{{.State.Health.Status}}{{else}}not_configured{{end}}'
}

wait_for_candidate_health() {
  local role="$1"
  local container_id="$2"
  local started=$SECONDS
  local status running
  while :; do
    status="$(candidate_health_status "$role" "$container_id")" || fail "cannot inspect candidate $role health"
    status="${status//$'\r'/}"
    case "$status" in
      healthy)
        case "$role" in
          postgres) candidate_health_postgres="$status" ;;
          redis) candidate_health_redis="$status" ;;
          app) candidate_health_app="$status" ;;
        esac
        return 0
        ;;
      not_configured)
        fail "candidate $role has no healthcheck"
        ;;
      unhealthy)
        fail "candidate $role healthcheck reported unhealthy"
        ;;
      starting|'')
        ;;
      *)
        fail "candidate $role returned unknown health status: $status"
        ;;
    esac
    running="$(candidate_inspect "running_${role}" "$container_id" '{{.State.Running}}')" || fail "cannot inspect candidate $role running state"
    [[ "$running" == 'true' ]] || fail "candidate $role exited before becoming healthy"
    if (( SECONDS - started >= wait_timeout_seconds )); then
      fail "candidate $role did not become healthy before timeout"
    fi
    sleep 2
  done
}

start_candidate_services() {
  docker_checked candidate_start_postgres start "$postgres_id" >/dev/null || fail 'cannot start candidate PostgreSQL'
  validate_candidate_container "$postgres_id" postgres "$postgres_runtime_image_id" "$postgres_volume" /var/lib/postgresql/data '' true || fail 'candidate PostgreSQL start isolation check failed'
  wait_for_candidate_health postgres "$postgres_id"
  validate_pid1_security "$postgres_id" || fail 'candidate PostgreSQL must run as non-root with NoNewPrivs and no effective capabilities'

  docker_checked candidate_start_redis start "$redis_id" >/dev/null || fail 'cannot start candidate Redis'
  validate_candidate_container "$redis_id" redis "$redis_runtime_image_id" "$redis_volume" /data '' true || fail 'candidate Redis start isolation check failed'
  wait_for_candidate_health redis "$redis_id"
  validate_pid1_security "$redis_id" || fail 'candidate Redis must run as non-root with NoNewPrivs and no effective capabilities'

  docker_checked candidate_start_app start "$app_id" >/dev/null || fail 'cannot start candidate application'
  validate_candidate_container "$app_id" app "$candidate_image_id" "$app_volume" /app/data '' true || fail 'candidate application start isolation check failed'
  wait_for_candidate_health app "$app_id"
  validate_pid1_security "$app_id" 1000 || fail 'candidate app must run as non-root with NoNewPrivs and no effective capabilities'
  validate_network_members || fail 'candidate network members changed after start'
}

candidate_pg_psql() {
  local sql="$1"
  shift || true
  local output status=0
  assert_production_unchanged before_candidate_postgresql_exec || return 1
  if output="$({ printf '%s\n' "$pg_password"; printf '%s\n' "$sql"; } |
    docker_rpc exec -i "$postgres_id" /bin/sh -c \
      'IFS= read -r password || exit 1; user="$1"; database="$2"; shift 2; unset PGHOST PGHOSTADDR PGSERVICE PGSERVICEFILE PGPASSFILE PGDATABASE PGUSER; PGPASSWORD="$password" PGCONNECT_TIMEOUT=8 exec psql -X -v ON_ERROR_STOP=1 -U "$user" -d "$database" "$@"' \
      sh "$pg_user" "$pg_database" "$@"); then
    status=0
  else
    status=$?
  fi
  assert_production_unchanged after_candidate_postgresql_exec || status=1
  (( status == 0 )) || return "$status"
  printf '%s' "$output"
}

candidate_redis_cli() {
  local output status=0
  assert_production_unchanged before_candidate_redis_exec || return 1
  if output="$(printf '%s\n' "$redis_password" |
    docker_rpc exec -i "$redis_id" /bin/sh -c \
      'IFS= read -r password || exit 1; unset REDISCLI_AUTH REDISCLI_HISTFILE; REDISCLI_AUTH="$password"; export REDISCLI_AUTH; exec redis-cli --no-auth-warning --raw -h 127.0.0.1 -p 6379 -n 0 "$@"' \
      sh "$@"); then
    status=0
  else
    status=$?
  fi
  assert_production_unchanged after_candidate_redis_exec || status=1
  (( status == 0 )) || return "$status"
  printf '%s' "$output"
}

set_candidate_rollout_gates() {
  local sql
  sql="$(cat <<'SQL'
BEGIN;
INSERT INTO settings (key, value) VALUES
  ('registration_ip_cooldown_enabled', 'false'),
  ('subnexus_activity_center_enabled', 'false'),
  ('subnexus_checkin_enabled', 'false'),
  ('subnexus_leaderboard_enabled', 'false'),
  ('subnexus_marquee_enabled', 'false'),
  ('subnexus_invite_activities_enabled', 'false'),
  ('subnexus_invite_rewards_enabled', 'false'),
  ('subnexus_first_recharge_enabled', 'false'),
  ('battle_pass_enabled', 'false'),
  ('subnexus_student_recharge_benefit_enabled', 'false'),
  ('subnexus_invoice_enabled', 'false'),
  ('channel_monitor_enabled', 'false'),
  ('channel_monitor_mode', 'v1'),
  ('subnexus_customer_support_enabled', 'false'),
  ('customer_support_enabled', 'false')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW();
INSERT INTO settings (key, value) VALUES
  ('subnexus_invite_activities_config', '{"enabled":false,"invite_lottery_enabled":false,"recharge_wheel_enabled":false,"invite_milestone_enabled":false}')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW();
COMMIT;
SQL
)"
  candidate_pg_psql "$sql" >/dev/null || fail 'cannot force candidate rollout gates closed'
}

verify_candidate_rollout_gates() {
  local count mode config
  count="$(candidate_pg_psql "SELECT COUNT(*) FROM settings WHERE key IN ('registration_ip_cooldown_enabled','subnexus_activity_center_enabled','subnexus_checkin_enabled','subnexus_leaderboard_enabled','subnexus_marquee_enabled','subnexus_invite_activities_enabled','subnexus_invite_rewards_enabled','subnexus_first_recharge_enabled','battle_pass_enabled','subnexus_student_recharge_benefit_enabled','subnexus_invoice_enabled','channel_monitor_enabled','subnexus_customer_support_enabled','customer_support_enabled') AND lower(trim(value)) = 'false';" -At | tr -d '[:space:]')" || fail 'cannot verify candidate rollout gate rows'
  [[ "$count" == '14' ]] || fail "candidate rollout gate count is not 14: $count"
  mode="$(candidate_pg_psql "SELECT value FROM settings WHERE key = 'channel_monitor_mode';" -At | tr -d '[:space:]')" || fail 'cannot verify channel monitor mode'
  [[ "$mode" == 'v1' ]] || fail "candidate channel monitor mode is not v1: $mode"
  config="$(candidate_pg_psql "SELECT CASE WHEN (value::jsonb->>'enabled') = 'false' AND (value::jsonb->>'invite_lottery_enabled') = 'false' AND (value::jsonb->>'recharge_wheel_enabled') = 'false' AND (value::jsonb->>'invite_milestone_enabled') = 'false' THEN 'closed' ELSE 'open' END FROM settings WHERE key = 'subnexus_invite_activities_config';" -At | tr -d '[:space:]')" || fail 'cannot verify invitation activity config'
  [[ "$config" == 'closed' ]] || fail 'candidate invitation activity config is not closed'
}

candidate_http() {
  local method="$1"
  local path="$2"
  local payload="${3:-}"
  local token="${4:-}"
  local output status=0
  [[ "$path" == /* && "$path" != *'..'* && "$path" != *$'\n'* && "$path" != *$'\r'* ]] || return 1
  assert_production_unchanged "before_http_${method}_${path//\//_}" || return 1
  if [[ "$method" == 'POST' ]]; then
    if output="$({ printf '%s\n' "$payload"; } |
      docker_rpc exec -i "$app_id" /bin/sh -c \
        'IFS= read -r body || exit 1; unset http_proxy https_proxy all_proxy no_proxy; export HTTP_PROXY= HTTPS_PROXY= ALL_PROXY= NO_PROXY="*"; exec wget -q -T 10 -O - --header="Content-Type: application/json" --post-data="$body" "http://127.0.0.1:8080$1"' \
        sh "$path"); then
      status=0
    else
      status=$?
    fi
  else
    if output="$(printf '%s\n' "$token" |
      docker_rpc exec -i "$app_id" /bin/sh -c \
        'IFS= read -r token || true; unset http_proxy https_proxy all_proxy no_proxy; export HTTP_PROXY= HTTPS_PROXY= ALL_PROXY= NO_PROXY="*"; if [ -n "$token" ]; then exec wget -q -T 10 -O - --header="Authorization: Bearer $token" "http://127.0.0.1:8080$1"; else exec wget -q -T 10 -O - "http://127.0.0.1:8080$1"; fi' \
        sh "$path"); then
      status=0
    else
      status=$?
    fi
  fi
  assert_production_unchanged "after_http_${method}_${path//\//_}" || status=1
  (( status == 0 )) || return "$status"
  [[ -n "$output" ]] || return 1
  printf '%s' "$output"
}

json_value_at() {
  local body="$1" path="$2" expected_type="${3:-any}"
  printf '%s' "$body" | python3 -c '
import json
import sys

def unique_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("duplicate JSON object key")
        result[key] = value
    return result

try:
    value = json.load(sys.stdin, object_pairs_hook=unique_object)
    for part in sys.argv[1].split("."):
        if not isinstance(value, dict) or part not in value:
            raise ValueError("JSON path is missing")
        value = value[part]
    expected = sys.argv[2]
    if expected == "string" and not isinstance(value, str):
        raise ValueError("JSON value is not a string")
    if expected == "bool" and not isinstance(value, bool):
        raise ValueError("JSON value is not a boolean")
    if expected == "any" and isinstance(value, (dict, list)):
        raise ValueError("JSON value is not scalar")
    if isinstance(value, bool):
        print("true" if value else "false")
    elif isinstance(value, str):
        print(value)
    else:
        print(value)
except (ValueError, TypeError, json.JSONDecodeError):
    raise SystemExit(1)
' "$path" "$expected_type"
}

json_unique_field() {
  local body="$1" key="$2"
  printf '%s' "$body" | python3 -c '
import json
import sys

def unique_object(pairs):
    result = {}
    for name, value in pairs:
        if name in result:
            raise ValueError("duplicate JSON object key")
        result[name] = value
    return result

def find(value, wanted, found):
    if isinstance(value, dict):
        if wanted in value:
            found.append(value[wanted])
        for child in value.values():
            find(child, wanted, found)
    elif isinstance(value, list):
        for child in value:
            find(child, wanted, found)

try:
    document = json.load(sys.stdin, object_pairs_hook=unique_object)
    values = []
    find(document, sys.argv[1], values)
    if len(values) != 1 or isinstance(values[0], (dict, list)):
        raise ValueError("field is missing or ambiguous")
    value = values[0]
    if isinstance(value, bool):
        print("true" if value else "false")
    elif isinstance(value, str):
        print(value)
    else:
        print(value)
except (ValueError, TypeError, json.JSONDecodeError):
    raise SystemExit(1)
' "$key"
}

assert_json_bool_false() {
  local body="$1" key="$2" value
  value="$(json_unique_field "$body" "$key")" || fail "public settings JSON is invalid or ambiguous: $key"
  [[ "$value" == 'false' ]] || fail "public settings flag must be false: $key"
}

run_candidate_http_smoke() {
  local health_body setup_body login_body me_body
  local login_payload login_token me_email public_body key
  health_body="$(candidate_http GET /health)" || fail 'candidate /health smoke failed'
  printf '%s' "$health_body" | grep -Eiq 'ok|healthy|success' || fail 'candidate /health response is unexpected'
  setup_body="$(candidate_http GET /setup/status)" || fail 'candidate /setup/status smoke failed'
  setup_needs_setup="$(json_value_at "$setup_body" data.needs_setup bool)" || fail 'candidate setup status parser failed'
  [[ "$setup_needs_setup" == 'false' ]] || fail 'candidate still requires setup after auto setup'
  public_body="$(candidate_http GET /api/v1/settings/public)" || fail 'candidate public settings smoke failed'
  for key in \
    subnexus_activity_center_enabled subnexus_checkin_enabled subnexus_leaderboard_enabled subnexus_marquee_enabled \
    subnexus_invite_activities_enabled subnexus_invite_lottery_enabled subnexus_recharge_wheel_enabled subnexus_invite_milestone_enabled \
    subnexus_first_recharge_enabled subnexus_student_recharge_benefit_enabled battle_pass_enabled invoice_enabled \
    channel_monitor_enabled customer_support_enabled; do
    assert_json_bool_false "$public_body" "$key"
  done
  [[ "$public_body" == *'"channel_monitor_mode":"v1"'* || "$public_body" == *'"channel_monitor_mode": "v1"'* ]] || fail 'public channel monitor mode is not v1'
  candidate_public_settings_verified='true'

  login_payload="{\"email\":\"gate-admin@example.invalid\",\"password\":\"$admin_password\"}"
  login_body="$(candidate_http POST /api/v1/auth/login "$login_payload")" || fail 'candidate administrator login smoke failed'
  login_token="$(json_value_at "$login_body" data.access_token string)" || fail 'candidate login response JSON is invalid'
  [[ "$login_token" =~ ^[A-Za-z0-9._-]{16,4096}$ ]] || fail 'candidate login did not return a valid access token'
  me_body="$(candidate_http GET /api/v1/auth/me '' "$login_token")" || fail 'candidate auth/me smoke failed'
  me_email="$(json_value_at "$me_body" data.email string)" || fail 'candidate auth/me response JSON is invalid'
  [[ "$me_email" == 'gate-admin@example.invalid' ]] || fail 'candidate auth/me returned the wrong administrator'
  candidate_login_verified='true'
}

candidate_migration_count() {
  local count
  count="$(candidate_pg_psql 'SELECT COUNT(*) FROM schema_migrations;' -At | tr -d '[:space:]')" || return 1
  [[ "$count" =~ ^[0-9]+$ ]] || return 1
  printf '%s' "$count"
}

run_candidate_dependency_smoke() {
  local pg_result redis_result
  pg_result="$(candidate_pg_psql 'SELECT 1;' -At | tr -d '[:space:]')" || fail 'candidate PostgreSQL SELECT 1 failed'
  [[ "$pg_result" == '1' ]] || fail 'candidate PostgreSQL returned an unexpected SELECT 1 result'
  redis_result="$(candidate_redis_cli PING | tr -d '[:space:]')" || fail 'candidate Redis PING failed'
  [[ "$redis_result" == 'PONG' ]] || fail 'candidate Redis returned an unexpected PING result'
}

run_candidate_persistence_smoke() {
  local sentinel_path='/app/data/.subnexus-candidate-sentinel'
  local migration_after_restart
  docker_checked candidate_write_sentinel exec --user 1000:1000 "$app_id" /bin/sh -c "printf '%s\\n' candidate-sentinel > '$sentinel_path'" >/dev/null || fail 'candidate data volume is not writable by UID 1000'
  candidate_sentinel_hash="$(docker_checked candidate_hash_sentinel exec --user 1000:1000 "$app_id" sha256sum "$sentinel_path" | awk '{print $1}')" || fail 'cannot hash candidate sentinel'
  [[ "$candidate_sentinel_hash" =~ ^[0-9a-f]{64}$ ]] || fail 'candidate sentinel hash is invalid'
  docker_checked candidate_check_install_marker exec "$app_id" /bin/sh -c 'test -s /app/data/config.yaml && test -s /app/data/.installed' >/dev/null || fail 'candidate install markers are missing from the data volume'
  candidate_migration_count="$(candidate_migration_count)" || fail 'cannot capture candidate migration count'
  candidate_restart_count_before="$(candidate_inspect candidate_restart_count_before "$app_id" '{{.RestartCount}}')" || fail 'cannot capture candidate restart count before restart'
  candidate_started_at_before_restart="$(candidate_inspect candidate_started_at_before "$app_id" '{{.State.StartedAt}}')" || fail 'cannot capture candidate start timestamp before restart'
  [[ "$candidate_restart_count_before" =~ ^[0-9]+$ && -n "$candidate_started_at_before_restart" ]] || fail 'candidate pre-restart identity is invalid'

  docker_checked candidate_app_restart restart "$app_id" >/dev/null || fail 'candidate application restart failed'
  wait_for_candidate_health app "$app_id"
  validate_candidate_container "$app_id" app "$candidate_image_id" "$app_volume" /app/data '' true || fail 'candidate application restart isolation check failed'
  validate_pid1_security "$app_id" 1000 || fail 'candidate app security changed after restart'
  candidate_sentinel_hash_after_restart="$(docker_checked candidate_hash_sentinel_after_restart exec --user 1000:1000 "$app_id" sha256sum "$sentinel_path" | awk '{print $1}')" || fail 'candidate sentinel disappeared after restart'
  [[ "$candidate_sentinel_hash_after_restart" == "$candidate_sentinel_hash" ]] || fail 'candidate data volume sentinel changed after restart'
  migration_after_restart="$(candidate_migration_count)" || fail 'cannot capture candidate migration count after restart'
  [[ "$migration_after_restart" == "$candidate_migration_count" ]] || fail 'candidate migration count changed on second startup'
  candidate_migration_count_after_restart="$migration_after_restart"
  candidate_restart_count_after="$(candidate_inspect candidate_restart_count_after "$app_id" '{{.RestartCount}}')" || fail 'cannot capture candidate restart count after restart'
  candidate_started_at_after_restart="$(candidate_inspect candidate_started_at_after "$app_id" '{{.State.StartedAt}}')" || fail 'cannot capture candidate start timestamp after restart'
  [[ "$candidate_restart_count_after" =~ ^[0-9]+$ && "$candidate_restart_count_after" -ge "$candidate_restart_count_before" ]] || fail 'candidate restart count regressed'
  [[ -n "$candidate_started_at_after_restart" && "$candidate_started_at_after_restart" != "$candidate_started_at_before_restart" ]] || fail 'candidate start timestamp did not change after restart'
  run_candidate_http_smoke
}

run_candidate_stable_window() {
  local started=$SECONDS status role container_id
  while (( SECONDS - started < stable_seconds )); do
    for role in postgres redis app; do
      case "$role" in
        postgres) container_id="$postgres_id" ;;
        redis) container_id="$redis_id" ;;
        app) container_id="$app_id" ;;
        *) fail "unknown candidate role during stable window: $role" ;;
      esac
      status="$(candidate_health_status "$role" "$container_id")" ||
        fail "candidate $role health inspection failed during stable window"
      [[ "$status" == 'healthy' ]] ||
        fail "candidate $role became unhealthy during stable window: $status"
      case "$role" in
        postgres) candidate_health_postgres="$status" ;;
        redis) candidate_health_redis="$status" ;;
        app) candidate_health_app="$status" ;;
      esac
    done
    validate_network_members || fail 'candidate network members changed during stable window'
    assert_production_unchanged stable_window || fail 'production identity changed during candidate stable window'
    sleep 5
  done
}

validate_candidate_image() {
  local image_id config_user image_os image_arch image_env
  image_id="$(capture_image_id "$candidate_image_tag")" || fail 'candidate image was not loaded by its exact immutable tag'
  [[ "$image_id" == "$expected_candidate_image_id" ]] || fail 'candidate image ID changed after archive load'
  candidate_image_id="$image_id"
  [[ "$(docker_rpc image inspect --format '{{index .Config.Labels "com.subnexus.release.gate"}}' "$candidate_image_tag")" == 'subnexus-isolated-build-v1' ]] ||
    fail 'candidate image release-gate label is incorrect'
  [[ "$(docker_rpc image inspect --format '{{index .Config.Labels "com.subnexus.candidate.commit"}}' "$candidate_image_tag")" == "$approved_sha" ]] ||
    fail 'candidate image commit label is incorrect'
  [[ "$(docker_rpc image inspect --format '{{index .Config.Labels "com.subnexus.candidate.tree"}}' "$candidate_image_tag")" == "$tree_sha" ]] ||
    fail 'candidate image tree label is incorrect'
  [[ "$(docker_rpc image inspect --format '{{index .Config.Labels "org.opencontainers.image.revision"}}' "$candidate_image_tag")" == "$approved_sha" ]] ||
    fail 'candidate image OCI revision label is incorrect'
  config_user="$(docker_rpc image inspect --format '{{.Config.User}}' "$candidate_image_tag")" || fail 'cannot inspect candidate image user'
  [[ -z "$config_user" || "$config_user" == '0' || "$config_user" == '0:0' ]] || fail 'candidate image must start through its root entrypoint for controlled privilege drop'
  image_os="$(docker_rpc image inspect --format '{{.Os}}' "$candidate_image_tag")" || fail 'cannot inspect candidate image OS'
  image_arch="$(docker_rpc image inspect --format '{{.Architecture}}' "$candidate_image_tag")" || fail 'cannot inspect candidate image architecture'
  [[ "$image_os" == 'linux' && "$image_arch" == 'amd64' ]] || fail 'candidate application image must be linux/amd64'
  image_env="$(docker_rpc image inspect --format '{{range .Config.Env}}{{println .}}{{end}}' "$candidate_image_tag")" || fail 'cannot inspect candidate image environment'
  for secret_name in DATABASE_PASSWORD REDIS_PASSWORD JWT_SECRET TOTP_ENCRYPTION_KEY ADMIN_PASSWORD; do
    ! printf '%s\n' "$image_env" | grep -Eq "^${secret_name}=" || fail "candidate image embeds forbidden secret environment: $secret_name"
  done
  [[ "$candidate_image_id" != "${production_image_id[app]}" &&
    "$candidate_image_id" != "${production_image_id[postgres]}" &&
    "$candidate_image_id" != "${production_image_id[redis]}" &&
    "$candidate_image_id" != "$postgres_runtime_image_id" &&
    "$candidate_image_id" != "$redis_runtime_image_id" ]] || fail 'candidate application image overlaps a production or dependency image'
}

  # Main candidate lifecycle. The release image was built on an isolated local
  # daemon; this production-host gate only loads that approved archive and runs
  # disposable services on an internal network.
  prepare_candidate_runtime_images
  assert_docker_free_space before_load
  load_candidate_image
  assert_docker_free_space before_create
  validate_candidate_image
  create_isolated_network runtime "$network_name" true
  assert_network_not_production "$network_id" || fail 'candidate runtime network overlaps a production network'
  create_candidate_containers
  start_candidate_services
  run_candidate_dependency_smoke
  set_candidate_rollout_gates
  verify_candidate_rollout_gates
  candidate_default_settings_verified='true'
  run_candidate_http_smoke
  run_candidate_persistence_smoke
  run_candidate_stable_window
  assert_production_unchanged before_evidence_publish || fail 'production identity changed before evidence publish'
  gate_result='passed'
}

main "$@"
