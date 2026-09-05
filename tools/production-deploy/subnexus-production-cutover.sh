#!/usr/bin/env bash
set -Eeuo pipefail

# SubNexus production cutover controller.
#
# This is deliberately a two-phase tool:
#   prepare  - validate an already reviewed release and create backups while
#              the current application remains online.  It never stops or
#              renames a production container and never runs migrations.
#   switch   - the short, operator-approved window.  It revalidates the
#              prepared evidence, closes only the SubNexus rollout gates,
#              preserves the old container, and starts the candidate.
#   rollback - an independent, repeatable application rollback.  It never
#              restores a database automatically.
#
# The script is intended to be installed as a root-owned regular file on the
# production host and invoked only by an operator.  It does not use Compose,
# fetch code, build, pull, prune, or connect to a remote Docker daemon.

case "$-" in
  *x*) set +x ;;
esac

umask 077
unset BASH_ENV ENV CDPATH GLOBIGNORE TAR_OPTIONS GZIP
export PATH='/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin'

readonly tool_name='subnexus-production-cutover-v1'
readonly candidate_tag_prefix='subnexus-release:'
readonly default_evidence_root='/srv/subnexus-migration/cutover'
readonly alternate_evidence_root='/root/subnexus-migration/cutover'
readonly candidate_artifact_root='/srv/subnexus-migration/candidate-artifacts'
readonly alternate_candidate_artifact_root='/root/subnexus-migration/candidate-artifacts'
readonly candidate_gate_root='/srv/subnexus-migration/docker-candidate'
readonly alternate_candidate_gate_root='/root/subnexus-migration/docker-candidate'
readonly production_source_root='/srv/subnexus-repo'
readonly alternate_production_source_root='/root/subnexus-repo'
readonly cutover_approval_token='I_UNDERSTAND_SHORT_PRODUCTION_WINDOW'
readonly rollback_approval_token='I_UNDERSTAND_APPLICATION_ROLLBACK'
readonly environment_duplicate_approval_token='I_UNDERSTAND_DOCKER_ENV_LAST_WINS'
readonly app_data_owner_approval_token='I_UNDERSTAND_NON_ROOT_APP_DATA_OWNER'
readonly app_data_owner_compat_uid='1000'
readonly app_data_owner_compat_gid='1000'
readonly final_free_reserve_bytes=8589934592
readonly postgresql_dump_budget_limit=12884901888
readonly redis_rdb_budget_limit=4294967296
readonly app_data_archive_budget_limit=8589934592
readonly prepare_metadata_budget_bytes=536870912
readonly app_data_archive_exclusion_pattern='./logs/*.log'
readonly app_data_archive_policy_version='exclude-active-logs-v1'

# These are the target-fork rollout controls.  They are intentionally kept in
# one allow-list so a typo cannot make rollback modify an unrelated setting.
readonly rollout_keys=(
  registration_ip_cooldown_enabled
  subnexus_activity_center_enabled
  subnexus_checkin_enabled
  subnexus_leaderboard_enabled
  subnexus_marquee_enabled
  subnexus_invite_activities_enabled
  subnexus_invite_rewards_enabled
  subnexus_first_recharge_enabled
  battle_pass_enabled
  subnexus_student_recharge_benefit_enabled
  subnexus_invoice_enabled
  channel_monitor_enabled
  channel_monitor_mode
  subnexus_customer_support_enabled
  customer_support_enabled
)
readonly rollout_content_keys=(
  subnexus_customer_support_content
  customer_support_content
)
readonly invitation_config_key='subnexus_invite_activities_config'

mode=''
source_root=''
target_sha=''
approved_script_sha=''
expected_image_id=''
candidate_archive=''
candidate_archive_sha=''
candidate_gate_evidence=''
candidate_gate_evidence_sha256=''
candidate_archive_reported_size=''
candidate_archive_expanded_size=''
live_app_ref=''
public_url=''
evidence_root=''
run_dir=''
manifest_file=''
app_id=''
app_name=''
app_image_id=''
database_id=''
redis_id=''
candidate_id=''
candidate_container_name=''
candidate_container_intent=''
preserved_name=''
docker_binary=''
docker_socket=''
docker_socket_fingerprint=''
docker_daemon_identity=''
docker_root_dir=''
evidence_lock_root=''
postgresql_dump_budget_bytes=''
redis_rdb_budget_bytes=''
app_data_archive_budget_bytes=''
app_data_archive_policy_sha256=''
docker_timeout_seconds=''
script_path=''
script_sha256=''
environment_duplicate_mode='strict'
environment_duplicate_keys=''
environment_duplicate_expected_hashes=''
environment_duplicate_evidence_sha256=''
environment_file_sha256=''
environment_duplicate_legacy=0
app_data_owner_policy='root-only'
app_data_owner_uid='0'
app_data_owner_gid='0'
app_data_owner_mode=''
environment_observed_mode=''
environment_observed_keys=''
environment_observed_expected_hashes=''
environment_observed_evidence_sha256=''
environment_observed_file_sha256=''
app_data_owner_manifest_legacy=0
stop_timeout_seconds=''
candidate_health_timeout_seconds=''
rollback_health_timeout_seconds=''
candidate_stability_seconds=''
cutover_active=0
rollback_active=0
lock_fd_open=0

declare -a app_networks=()
declare -a app_network_ids=()
declare -a captured_ports=()
declare -a captured_mounts=()
declare -a captured_security_opts=()
declare -a captured_entrypoint=()
declare -a captured_cmd=()
declare -A before_settings=()
declare -A before_settings_seen=()

usage() {
  cat >&2 <<'USAGE'
usage:
  subnexus-production-cutover.sh prepare SOURCE_ROOT TARGET_SHA APPROVED_SCRIPT_SHA EXPECTED_IMAGE_ID IMAGE_ARCHIVE IMAGE_ARCHIVE_SHA256 CANDIDATE_GATE_EVIDENCE LIVE_APP [PUBLIC_HEALTH_URL] [EVIDENCE_ROOT]
  subnexus-production-cutover.sh switch RUN_DIRECTORY
  subnexus-production-cutover.sh rollback RUN_DIRECTORY

prepare creates a root-only run directory and backups while the live app is
still serving.  switch and rollback require the operator confirmation token:
  SUBNEXUS_CUTOVER_CONFIRM=I_UNDERSTAND_SHORT_PRODUCTION_WINDOW
  SUBNEXUS_CUTOVER_CONFIRM=I_UNDERSTAND_APPLICATION_ROLLBACK

Duplicate Docker environment keys are rejected by default.  A prepare run may
opt into a narrowly reviewed last-wins compatibility case only when all three
values are supplied independently (keys and hashes are comma-separated and
sorted by key):
  SUBNEXUS_CUTOVER_ENV_DUPLICATE_CONFIRM=I_UNDERSTAND_DOCKER_ENV_LAST_WINS
  SUBNEXUS_CUTOVER_ENV_DUPLICATE_KEYS=SERVER_TRUSTED_PROXIES
  SUBNEXUS_CUTOVER_ENV_DUPLICATE_EXPECTED_SHA256=SERVER_TRUSTED_PROXIES=<64 lowercase hex>

The application data bind/volume source is root-owned by default.  If the
existing live data directory intentionally has a non-root owner, prepare may
opt in only with all three values (validated independently):
  SUBNEXUS_CUTOVER_APP_DATA_OWNER_CONFIRM=I_UNDERSTAND_NON_ROOT_APP_DATA_OWNER
  SUBNEXUS_CUTOVER_APP_DATA_OWNER_UID=1000
  SUBNEXUS_CUTOVER_APP_DATA_OWNER_GID=1000

Application data normally must be a root-owned directory.  The only supported
non-root leaf owner is UID/GID 1000, and every phase touching a prepared run
must repeat the explicit acknowledgement and exact owner values:
  SUBNEXUS_CUTOVER_APP_DATA_OWNER_CONFIRM=I_UNDERSTAND_NON_ROOT_APP_DATA_OWNER
  SUBNEXUS_CUTOVER_APP_DATA_OWNER_UID=1000
  SUBNEXUS_CUTOVER_APP_DATA_OWNER_GID=1000

The approved script SHA must be supplied independently; it must not be
computed in the same command that invokes this tool.  The candidate archive
and gate evidence must already be reviewed and copied below their approved
root-only directories.  No mode pulls, builds, fetches, prunes, or restores a
database backup.
USAGE
}

prepare_argument_count_is_valid() {
  case "${1:-}" in
    8|9|10) return 0 ;;
    *) return 1 ;;
  esac
}

log() {
  printf '[%s] %s\n' "$tool_name" "$*"
}

rollback_after_failure() {
  local rc="${1:-1}"
  # A direct fail() exits without firing an ERR trap.  Once the cutover has
  # renamed the old container, perform the same bounded application rollback
  # here.  BASHPID prevents a command-substitution subshell from touching the
  # live Docker state on behalf of the parent shell.
  if [[ "${cutover_active:-0}" == 1 && "${rollback_active:-0}" == 0 &&
        -n "${run_dir:-}" && -f "${manifest_file:-}" &&
        "${BASHPID:-$$}" == "$$" ]]; then
    rollback_active=1
    printf '[%s] ERROR: cutover failed (rc=%s); attempting automatic application rollback.\n' "$tool_name" "$rc" >&2
    rollback_run 1 || printf '[%s] CRITICAL: automatic rollback failed; run the manual rollback command from the handoff document.\n' "$tool_name" >&2
    rollback_active=0
  fi
}

fail() {
  printf '[%s] ERROR: %s\n' "$tool_name" "$*" >&2
  rollback_after_failure 1
  exit 1
}

valid_sha40() {
  [[ "$1" =~ ^[0-9a-f]{40}$ ]]
}

valid_sha64() {
  [[ "$1" =~ ^[0-9a-f]{64}$ ]]
}

valid_container_ref() {
  [[ "$1" =~ ^([A-Za-z0-9][A-Za-z0-9_.-]{0,254}|[0-9a-fA-F]{12,64}|sha256:[0-9a-fA-F]{12,64})$ ]]
}

valid_port() {
  [[ "$1" =~ ^[0-9]{1,5}$ ]] || return 1
  local value=$((10#$1))
  (( value >= 1 && value <= 65535 ))
}

valid_host_ip() {
  case "$1" in
    ''|0.0.0.0|127.0.0.1) return 0 ;;
    *) return 1 ;;
  esac
}

mode_is_safe() {
  local path="$1" mode_value
  mode_value="$(stat -c '%a' -- "$path" 2>/dev/null)" || return 1
  [[ "$mode_value" =~ ^[0-7]{3,4}$ ]] || return 1
  mode_value="${mode_value: -3}"
  (( (8#$mode_value & 8#022) == 0 ))
}

assert_root_owned_regular() {
  local path="$1" label="${2:-file}" resolved
  [[ -f "$path" && ! -L "$path" ]] || fail "$label must be a regular non-symbolic file: $path"
  resolved="$(realpath -e -P -- "$path")" || fail "cannot resolve $label: $path"
  [[ "$resolved" == "$path" ]] || fail "$label contains a symbolic-link component: $path"
  [[ "$(stat -c '%u' -- "$path")" == '0' ]] || fail "$label must be root-owned: $path"
  [[ "$(stat -c '%h' -- "$path")" == '1' ]] || fail "$label must have exactly one hard link: $path"
  mode_is_safe "$path" || fail "$label must not be group/other writable: $path"
}

assert_root_owned_dir() {
  local path="$1" label="${2:-directory}" resolved
  [[ -d "$path" && ! -L "$path" ]] || fail "$label must be a directory and not symbolic: $path"
  resolved="$(realpath -e -P -- "$path")" || fail "cannot resolve $label: $path"
  [[ "$resolved" == "$path" ]] || fail "$label contains a symbolic-link component: $path"
  [[ "$(stat -c '%u' -- "$path")" == '0' ]] || fail "$label must be root-owned: $path"
  mode_is_safe "$path" || fail "$label must not be group/other writable: $path"
}

ensure_root_owned_dir() {
  local path="$1" label="${2:-directory}" parent
  if [[ ! -e "$path" && ! -L "$path" ]]; then
    parent="${path%/*}"
    [[ -d "$parent" && ! -L "$parent" ]] || fail "parent of $label is missing or symbolic: $parent"
    [[ "$(stat -c '%u' -- "$parent")" == '0' ]] || fail "parent of $label must be root-owned: $parent"
    mode_is_safe "$parent" || fail "parent of $label must not be group/other writable: $parent"
    mkdir -- "$path" || fail "cannot create $label: $path"
  fi
  assert_root_owned_dir "$path" "$label"
  chmod 700 -- "$path"
}

path_under() {
  local path="$1" root="$2"
  [[ "$path" == "$root"/* ]]
}

path_equal_or_under() {
  local path="$1" root="$2"
  [[ "$path" == "$root" || "$path" == "$root"/* ]]
}

path_overlaps() {
  local left="$1" right="$2"
  [[ "$left" == "$right" || "$left" == "$right"/* || "$right" == "$left"/* ]]
}

assert_approved_path() {
  local path="$1" kind="$2" lexical resolved root
  [[ "$path" == /* ]] || fail "$kind must be an absolute path"
  lexical="$(realpath -m -s -- "$path")" || fail "cannot normalize $kind without following symbolic links: $path"
  resolved="$(realpath -e -P -- "$path")" || fail "$kind does not exist: $path"
  [[ "$lexical" == "$resolved" ]] || fail "$kind contains a symbolic-link component: $path"
  case "$kind" in
    candidate_archive)
      if path_under "$resolved" "$candidate_artifact_root"; then root="$candidate_artifact_root"
      elif path_under "$resolved" "$alternate_candidate_artifact_root"; then root="$alternate_candidate_artifact_root"
      else fail "$kind is outside the approved artifact root: $resolved"; fi
      ;;
    candidate_gate)
      if path_under "$resolved" "$candidate_gate_root"; then root="$candidate_gate_root"
      elif path_under "$resolved" "$alternate_candidate_gate_root"; then root="$alternate_candidate_gate_root"
      else fail "$kind is outside the approved gate root: $resolved"; fi
      ;;
    source)
      if path_equal_or_under "$resolved" '/srv/subnexus-migration'; then root='/srv/subnexus-migration'
      elif path_equal_or_under "$resolved" '/root/subnexus-migration'; then root='/root/subnexus-migration'
      elif path_equal_or_under "$resolved" "$production_source_root"; then root="$production_source_root"
      elif path_equal_or_under "$resolved" "$alternate_production_source_root"; then root="$alternate_production_source_root"
      else fail "$kind is outside the approved migration/source roots: $resolved"; fi
      ;;
    evidence)
      if [[ "$resolved" == "$default_evidence_root" || "$resolved" == "$default_evidence_root"/* ]]; then root="$default_evidence_root"
      elif [[ "$resolved" == "$alternate_evidence_root" || "$resolved" == "$alternate_evidence_root"/* ]]; then root="$alternate_evidence_root"
      else fail "$kind is outside the approved evidence root: $resolved"; fi
      ;;
    *) fail "unknown approved path kind: $kind" ;;
  esac
  printf '%s|%s' "$resolved" "$root"
}

hash_file() {
  sha256sum -- "$1" | awk '{print tolower($1)}'
}

hash_text() {
  printf '%s' "$1" | sha256sum | awk '{print tolower($1)}'
}

base64_text() {
  printf '%s' "$1" | base64 | tr -d '\r\n'
}

validate_self_sha() {
  local expected="$1" self_path actual
  valid_sha64 "$expected" || fail 'approved cutover script SHA must be 64 lowercase hexadecimal characters'
  self_path="$(realpath -e -P -- "${BASH_SOURCE[0]}")" || fail 'cannot resolve cutover script path'
  assert_root_owned_regular "$self_path" 'cutover script'
  actual="$(hash_file "$self_path")" || fail 'cannot hash cutover script'
  [[ "$actual" == "$expected" ]] || fail "cutover script SHA mismatch: actual=$actual expected=$expected"
  script_path="$self_path"
  script_sha256="$actual"
}

require_commands() {
  local command_name
  for command_name in docker timeout git curl python3 sha256sum stat realpath flock mktemp tar gzip base64 awk sed grep sort tr date mkdir chmod mv rm sleep cat wc df du id cut; do
    command -v "$command_name" >/dev/null 2>&1 || fail "missing required command: $command_name"
  done
}

docker_rpc() {
  timeout --foreground --kill-after=10s "${docker_timeout_seconds}s" "$docker_binary" "$@"
}

validate_docker_timeout() {
  local requested max_timeout phase
  requested="${SUBNEXUS_DOCKER_TIMEOUT_SECONDS:-120}"
  [[ "$requested" =~ ^[0-9]{1,4}$ ]] ||
    fail 'SUBNEXUS_DOCKER_TIMEOUT_SECONDS must be 1..4 decimal digits'
  docker_timeout_seconds=$((10#$requested))
  phase="${mode:-prepare}"
  case "$phase" in
    prepare) max_timeout=1800 ;;
    switch|rollback) max_timeout=600 ;;
    *) fail "cannot validate Docker timeout for unknown phase: $phase" ;;
  esac
  (( docker_timeout_seconds >= 10 && docker_timeout_seconds <= max_timeout )) ||
    fail "SUBNEXUS_DOCKER_TIMEOUT_SECONDS must be between 10 and ${max_timeout} seconds for ${phase}"
}

init_docker() {
  local endpoint daemon_root daemon_root_resolved socket_resolved
  local socket_mode
  validate_docker_timeout
  for override in DOCKER_HOST DOCKER_CONTEXT DOCKER_CONFIG DOCKER_TLS_VERIFY DOCKER_CERT_PATH DOCKER_API_VERSION; do
    [[ -z "${!override:-}" ]] || fail "$override must be unset; only the local default Docker daemon is allowed"
  done
  docker_binary="$(command -v docker)"
  assert_root_owned_regular "$docker_binary" 'Docker executable'
  [[ "$(docker_rpc context show 2>/dev/null)" == 'default' ]] || fail 'Docker context must be default'
  endpoint="$(docker_rpc context inspect --format '{{(index .Endpoints "docker").Host}}' default 2>/dev/null)" ||
    fail 'cannot determine the default Docker endpoint'
  case "$endpoint" in
    unix:///var/run/docker.sock|unix:///run/docker.sock) ;;
    *) fail "default Docker endpoint must be the local system Docker socket: $endpoint" ;;
  esac
  docker_socket="${endpoint#unix://}"
  [[ -S "$docker_socket" && ! -L "$docker_socket" ]] || fail 'Docker socket must be a root-owned non-symbolic Unix socket'
  socket_resolved="$(realpath -e -P -- "$docker_socket")" || fail 'cannot resolve Docker socket'
  # Ubuntu commonly exposes /var/run as a symlink to /run.  Normalize that
  # harmless alias, but refuse any other redirected socket path.
  [[ "$socket_resolved" == '/run/docker.sock' ]] || fail 'Docker socket must resolve to the canonical local system socket'
  docker_socket="$socket_resolved"
  [[ "$(stat -c '%u' -- "$docker_socket")" == '0' ]] || fail 'Docker socket must be root-owned'
  socket_mode="$(stat -c '%a' -- "$docker_socket")" || fail 'cannot inspect Docker socket mode'
  [[ "$socket_mode" =~ ^[0-7]{3,4}$ ]] || fail 'Docker socket mode is invalid'
  (( (8#$socket_mode & 8#007) == 0 && (8#$socket_mode & 8#600) == 8#600 )) ||
    fail 'Docker socket must be owner-readable/writable with no other access'
  docker_socket_fingerprint="$(stat -Lc '%d|%i|%u|%g|%a|%F' -- "$docker_socket")" || fail 'cannot fingerprint Docker socket'
  [[ "$docker_socket_fingerprint" == *'|socket' ]] || fail 'Docker endpoint is not a Unix socket'
  daemon_root="$(docker_rpc info --format '{{.DockerRootDir}}')" || fail 'cannot inspect Docker root'
  [[ -d "$daemon_root" && ! -L "$daemon_root" && "$(stat -c '%u' -- "$daemon_root")" == '0' ]] || fail 'Docker root must be a root-owned directory'
  mode_is_safe "$daemon_root" || fail 'Docker root must not be group/other writable'
  daemon_root_resolved="$(realpath -e -P -- "$daemon_root")" || fail 'cannot resolve Docker root'
  [[ "$daemon_root_resolved" == "$daemon_root" ]] || fail 'Docker root contains a symbolic-link component'
  docker_root_dir="$daemon_root_resolved"
  docker_daemon_identity="$(docker_rpc info --format '{{.ID}}|{{.Name}}|{{.ServerVersion}}|{{.DockerRootDir}}|{{json .SecurityOptions}}')" ||
    fail 'cannot capture Docker daemon identity'
  [[ -n "$docker_daemon_identity" ]] || fail 'Docker daemon identity is empty'
  [[ "$docker_daemon_identity" == *seccomp* ]] || fail 'Docker daemon must provide seccomp isolation'
}

acquire_lock() {
  local root="$1" path_stat fd_stat
  assert_root_owned_dir "$root" 'cutover evidence root'
  # Lock the validated directory inode itself.  A separate lock-file
  # redirection could follow a symlink during the check/open window.
  exec 9<"$root"
  lock_fd_open=1
  path_stat="$(stat -Lc '%d|%i|%u|%h|%a|%F' -- "$root")" || fail 'cannot stat cutover evidence root'
  fd_stat="$(stat -Lc '%d|%i|%u|%h|%a' -- "/proc/${BASHPID:-$$}/fd/9")" || fail 'cannot stat opened cutover lock'
  [[ "$path_stat" == "$fd_stat|directory" ]] || fail 'cutover evidence root changed while opening'
  flock -n 9 || fail 'another SubNexus cutover is already running'
}

manifest_value() {
  local key="$1" file="${2:-$manifest_file}"
  awk -F= -v wanted="$key" '$1 == wanted {sub(/^[^=]*=/, ""); print; exit}' "$file"
}

manifest_has_key() {
  local key="$1" file="${2:-$manifest_file}"
  awk -F= -v wanted="$key" '$1 == wanted { found = 1; exit } END { exit(found ? 0 : 1) }' "$file"
}

manifest_set() {
  local key="$1" value="$2" tmp
  [[ "$key" =~ ^[A-Za-z0-9_.-]+$ ]] || fail 'manifest key is invalid'
  [[ "$value" != *$'\n'* && "$value" != *$'\r'* ]] || fail 'manifest value contains a line break'
  assert_root_owned_regular "$manifest_file" 'cutover manifest'
  # mktemp uses O_EXCL, so a stale/symlinked temporary pathname cannot be
  # followed by the redirection below.  The run directory is root-only, but
  # keep this invariant explicit because manifest updates happen during the
  # short cutover window as well as during prepare.
  tmp="$(mktemp "$run_dir/.manifest.env.tmp.XXXXXX")" || fail 'cannot create manifest temporary file'
  assert_root_owned_regular "$tmp" 'manifest temporary file'
  if ! awk -F= -v wanted="$key" -v replacement="$key=$value" '
    BEGIN { replaced = 0 }
    $1 == wanted { if (!replaced) print replacement; replaced = 1; next }
    { print }
    END { if (!replaced) print replacement }
  ' "$manifest_file" > "$tmp"; then
    rm -f -- "$tmp"
    fail 'cannot update manifest'
  fi
  chmod 600 -- "$tmp"
  mv -f -- "$tmp" "$manifest_file"
}

write_run_marker() {
  local marker_name="$1" marker_value="$2" marker_path temporary
  [[ "$marker_name" == READY || "$marker_name" == SWITCHED || "$marker_name" == ROLLED_BACK ]] ||
    fail 'run marker name is invalid'
  [[ "$marker_value" =~ ^[a-z_]+$ ]] || fail 'run marker value is invalid'
  marker_path="$run_dir/$marker_name"
  if [[ -e "$marker_path" || -L "$marker_path" ]]; then
    assert_root_owned_regular "$marker_path" "cutover $marker_name marker"
    [[ "$(read_one_line "$marker_path")" == "$marker_value" ]] || fail "cutover $marker_name marker has an unexpected value"
    return 0
  fi
  temporary="$(mktemp "$run_dir/.${marker_name}.XXXXXX")" || fail "cannot create cutover $marker_name marker temporary file"
  assert_root_owned_regular "$temporary" "cutover $marker_name marker temporary file"
  printf '%s\n' "$marker_value" > "$temporary" || { rm -f -- "$temporary"; fail "cannot write cutover $marker_name marker"; }
  chmod 600 -- "$temporary"
  mv -f -- "$temporary" "$marker_path"
  assert_root_owned_regular "$marker_path" "cutover $marker_name marker"
  [[ "$(read_one_line "$marker_path")" == "$marker_value" ]] || fail "cutover $marker_name marker verification failed"
}

assert_run_marker() {
  local marker_name="$1" marker_value="$2" marker_path="$run_dir/$1"
  assert_root_owned_regular "$marker_path" "cutover $marker_name marker"
  [[ "$(read_one_line "$marker_path")" == "$marker_value" ]] || fail "cutover $marker_name marker has an unexpected value"
}

validate_manifest_shape() {
  local line key
  local -A seen=()
  while IFS= read -r line; do
    [[ "$line" =~ ^[A-Za-z0-9_.-]+=.*$ ]] || fail 'cutover manifest contains a malformed line'
    key="${line%%=*}"
    [[ -z "${seen[$key]+x}" ]] || fail "cutover manifest contains duplicate key: $key"
    seen[$key]=1
  done < "$manifest_file"
}

read_one_line() {
  local path="$1" value
  value="$(tr -d '\r\n' < "$path")" || fail "cannot read $path"
  printf '%s' "$value"
}

env_value() {
  local key="$1" file="$2"
  awk -F= -v wanted="$key" '$1 == wanted {sub(/^[^=]*=/, ""); sub(/\r$/, ""); print; exit}' "$file"
}

validate_environment_duplicate_inputs() {
  local keys_raw="${1:-}" hashes_raw="${2:-}" item key digest
  local -a keys=() hashes=()
  local -A seen_keys=() seen_hashes=()
  [[ "${#keys_raw}" -le 2048 && "${#hashes_raw}" -le 4096 ]] ||
    fail 'duplicate environment approval input is too long'
  if [[ -n "$keys_raw" ]]; then
    IFS=',' read -r -a keys <<< "$keys_raw"
    for key in "${keys[@]}"; do
      [[ "$key" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]] ||
        fail 'duplicate environment approval key is invalid'
      [[ -z "${seen_keys[$key]+x}" ]] || fail 'duplicate environment approval key is repeated'
      seen_keys[$key]=1
    done
  fi
  if [[ -n "$hashes_raw" ]]; then
    IFS=',' read -r -a hashes <<< "$hashes_raw"
    for item in "${hashes[@]}"; do
      key="${item%%=*}"
      digest="${item#*=}"
      [[ "$item" == *=* && "$key" =~ ^[A-Za-z_][A-Za-z0-9_]*$ && "$digest" =~ ^[0-9a-f]{64}$ ]] ||
        fail 'duplicate environment expected value hash is invalid'
      [[ -z "${seen_hashes[$key]+x}" ]] || fail 'duplicate environment expected value hash is repeated'
      seen_hashes[$key]=1
      [[ -n "${seen_keys[$key]+x}" ]] || fail 'duplicate environment expected value hash has an unapproved key'
    done
  fi
  [[ "${#keys[@]}" -eq "${#hashes[@]}" ]] ||
    fail 'duplicate environment keys and expected hashes must have the same count'
}

validate_app_data_owner_inputs() {
  local confirm="${SUBNEXUS_CUTOVER_APP_DATA_OWNER_CONFIRM:-}"
  local uid="${SUBNEXUS_CUTOVER_APP_DATA_OWNER_UID:-}"
  local gid="${SUBNEXUS_CUTOVER_APP_DATA_OWNER_GID:-}"
  local supplied=0
  [[ -n "$confirm" || -n "$uid" || -n "$gid" ]] && supplied=1
  if (( supplied == 0 )); then
    app_data_owner_policy='root-only'
    app_data_owner_uid='0'
    app_data_owner_gid='0'
    return 0
  fi
  [[ "$confirm" == "$app_data_owner_approval_token" ]] ||
    fail 'non-root application data owner requires explicit confirmation'
  [[ "$uid" =~ ^[1-9][0-9]{0,9}$ && "$gid" =~ ^[1-9][0-9]{0,9}$ ]] ||
    fail 'non-root application data owner UID/GID must be positive decimal values'
  (( 10#$uid <= 4294967295 && 10#$gid <= 4294967295 )) ||
    fail 'non-root application data owner UID/GID is out of range'
  [[ "$uid" == "$app_data_owner_compat_uid" && "$gid" == "$app_data_owner_compat_gid" ]] ||
    fail "only the reviewed non-root application data owner ${app_data_owner_compat_uid}:${app_data_owner_compat_gid} is supported"
  app_data_owner_policy='explicit-uid-gid'
  app_data_owner_uid="$((10#$uid))"
  app_data_owner_gid="$((10#$gid))"
}

app_data_owner_mode_is_safe() {
  local mode_value="${1:-}"
  [[ "$mode_value" =~ ^[0-7]{3}$ ]] || return 1
  # The leaf must remain a usable directory for its owner, while group/other
  # users may not write it.  Special mode bits are intentionally rejected.
  (( (8#$mode_value & 8#700) == 8#700 && (8#$mode_value & 8#022) == 0 ))
}

validate_app_data_owner_runtime_inputs() {
  local supplied=0
  [[ -n "${SUBNEXUS_CUTOVER_APP_DATA_OWNER_CONFIRM:-}" ||
     -n "${SUBNEXUS_CUTOVER_APP_DATA_OWNER_UID:-}" ||
     -n "${SUBNEXUS_CUTOVER_APP_DATA_OWNER_GID:-}" ]] && supplied=1
  if [[ "$app_data_owner_policy" == root-only ]]; then
    (( supplied == 0 )) || fail 'non-root application data owner inputs do not match the prepared root-only policy'
    return 0
  fi
  validate_app_data_owner_inputs
  [[ "$app_data_owner_policy" == explicit-uid-gid &&
     "$app_data_owner_uid" == "$app_data_owner_compat_uid" &&
     "$app_data_owner_gid" == "$app_data_owner_compat_gid" ]] ||
    fail 'runtime application data owner inputs do not match the prepared policy'
}

validate_app_data_owner_manifest() {
  local policy uid gid mode key
  app_data_owner_manifest_legacy=0
  if ! manifest_has_key app_data_owner_policy; then
    # Runs created before the owner contract existed are deliberately treated
    # as legacy root-UID runs.  The old validator required UID 0 but did not
    # constrain the historical GID; preserve that narrow rollback exception.
    # A partially added set of fields is not a valid legacy manifest and must
    # fail closed.
    for key in app_data_owner_uid app_data_owner_gid app_data_owner_mode; do
      manifest_has_key "$key" && fail "legacy manifest unexpectedly contains $key"
    done
    app_data_owner_manifest_legacy=1
    app_data_owner_policy='root-only'
    app_data_owner_uid='0'
    # The pre-contract validator recorded only root UID.  Keep that legacy
    # rollback path compatible while the immutable identity still records the
    # actual historical group and detects any later change.
    app_data_owner_gid=''
    app_data_owner_mode=''
    validate_app_data_owner_runtime_inputs
    return 0
  fi
  policy="$(manifest_value app_data_owner_policy)"
  uid="$(manifest_value app_data_owner_uid)"
  gid="$(manifest_value app_data_owner_gid)"
  mode="$(manifest_value app_data_owner_mode)"
  [[ "$policy" == root-only || "$policy" == explicit-uid-gid ]] ||
    fail 'manifest application data owner policy is invalid'
  [[ "$uid" =~ ^[0-9]{1,10}$ && "$gid" =~ ^[0-9]{1,10}$ ]] ||
    fail 'manifest application data owner UID/GID is invalid'
  (( 10#$uid <= 4294967295 && 10#$gid <= 4294967295 )) ||
    fail 'manifest application data owner UID/GID is out of range'
  app_data_owner_mode_is_safe "$mode" || fail 'manifest application data owner mode is invalid'
  if [[ "$policy" == root-only ]]; then
    [[ "$uid" == 0 && "$gid" == 0 ]] || fail 'root-only manifest has a non-root owner'
  else
    [[ "$uid" == "$app_data_owner_compat_uid" && "$gid" == "$app_data_owner_compat_gid" ]] ||
      fail 'manifest uses an unsupported non-root application data owner'
  fi
  app_data_owner_policy="$policy"
  app_data_owner_uid="$((10#$uid))"
  app_data_owner_gid="$((10#$gid))"
  app_data_owner_mode="$mode"
  validate_app_data_owner_runtime_inputs
}

validate_environment_file() {
  local file="$1" line key value
  local -A seen=()
  assert_root_owned_regular "$file" 'container environment metadata'
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ -n "$line" && "$line" != *$'\r'* && "$line" != *$'\n'* ]] ||
      fail 'container environment metadata contains an empty or multiline entry'
    [[ "$line" == *=* ]] || fail 'container environment metadata entry has no assignment'
    key="${line%%=*}"
    value="${line#*=}"
    [[ "$key" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]] ||
      fail "container environment metadata key is invalid: $key"
    [[ -z "${seen[$key]+x}" ]] ||
      fail "container environment metadata contains duplicate key: $key"
    seen[$key]=1
    # Keep the value expansion explicit so a future change cannot accidentally
    # treat a leading dash or shell metacharacter as an option/command.
    [[ "$value" != *$'\r'* && "$value" != *$'\n'* ]] ||
      fail "container environment metadata value is multiline: $key"
  done < "$file"
}

validate_environment_duplicate_evidence() {
  local evidence_file="$1" env_file="$2" expected_mode="${3:-}" expected_keys="${4:-}" expected_hashes="${5:-}"
  python3 - "$evidence_file" "$env_file" "$expected_mode" "$expected_keys" "$expected_hashes" <<'PY'
import hashlib
import re
import sys

evidence_path, env_path, expected_mode, expected_keys, expected_hashes = sys.argv[1:]

def reject(message):
    raise SystemExit(message)

def read_bytes(path):
    try:
        with open(path, "rb") as handle:
            return handle.read()
    except OSError as exc:
        reject("cannot read environment metadata evidence")

evidence = read_bytes(evidence_path)
env_bytes = read_bytes(env_path)
try:
    evidence_text = evidence.decode("utf-8")
    env_text = env_bytes.decode("utf-8")
except UnicodeDecodeError:
    reject("environment metadata evidence is not UTF-8")
if not evidence_text or not evidence_text.endswith("\n"):
    reject("environment metadata evidence is incomplete")
if any("\r" in line for line in evidence_text.splitlines()):
    reject("environment metadata evidence contains a carriage return")

headers = {}
rows = []
for line in evidence_text.splitlines():
    if line.startswith("duplicate|"):
        rows.append(line.split("|"))
        continue
    if "=" not in line:
        reject("environment metadata evidence contains a malformed line")
    key, value = line.split("=", 1)
    if key in headers:
        reject("environment metadata evidence contains duplicate fields")
    if key not in {
        "format", "mode", "source_entries", "normalized_entries", "duplicate_keys",
        "source_env_sha256", "normalized_env_sha256",
    }:
        reject("environment metadata evidence contains an unknown field")
    headers[key] = value
required = {
    "format", "mode", "source_entries", "normalized_entries", "duplicate_keys",
    "source_env_sha256", "normalized_env_sha256",
}
if set(headers) != required:
    reject("environment metadata evidence is missing required fields")
if headers["format"] != "1":
    reject("environment metadata evidence format is unsupported")
mode = headers["mode"]
if mode not in ("strict", "last-wins"):
    reject("environment metadata evidence mode is invalid")
if expected_mode and mode != expected_mode:
    reject("environment metadata evidence mode changed")

def parse_count(value):
    if not re.fullmatch(r"[0-9]+", value):
        reject("environment metadata evidence count is invalid")
    return int(value)

source_entries = parse_count(headers["source_entries"])
normalized_entries = parse_count(headers["normalized_entries"])
if normalized_entries > source_entries:
    reject("environment metadata evidence entry counts are inconsistent")
for name in ("source_env_sha256", "normalized_env_sha256"):
    if not re.fullmatch(r"[0-9a-f]{64}", headers[name]):
        reject("environment metadata evidence hash is invalid")
if hashlib.sha256(env_bytes).hexdigest() != headers["normalized_env_sha256"]:
    reject("normalized environment metadata hash does not match evidence")

def parse_keys(value):
    if value == "":
        return []
    result = value.split(",")
    if result != sorted(result) or len(set(result)) != len(result):
        reject("environment metadata duplicate key list is not canonical")
    if any(not re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*", item) for item in result):
        reject("environment metadata duplicate key list is invalid")
    return result

duplicate_keys = parse_keys(headers["duplicate_keys"])
if expected_keys:
    if ",".join(duplicate_keys) != expected_keys:
        reject("environment metadata duplicate key list changed")
elif duplicate_keys:
    reject("environment metadata contains unapproved duplicate keys")

expected_map = {}
if expected_hashes:
    for item in expected_hashes.split(","):
        key, separator, digest = item.partition("=")
        if not separator or not re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*", key) or not re.fullmatch(r"[0-9a-f]{64}", digest):
            reject("environment metadata expected hash list is invalid")
        if key in expected_map:
            reject("environment metadata expected hash list is repeated")
        expected_map[key] = digest
    if list(expected_map) != sorted(expected_map):
        reject("environment metadata expected hash list is not canonical")
if set(expected_map) != set(duplicate_keys):
    reject("environment metadata expected hash list does not match duplicate keys")

seen_rows = set()
for row in rows:
    if len(row) != 6 or row[0] != "duplicate":
        reject("environment metadata duplicate evidence row is malformed")
    _, key, count_text, last_index_text, hashes_text, selected_hash = row
    if key in seen_rows or key not in duplicate_keys:
        reject("environment metadata duplicate evidence row is unexpected")
    seen_rows.add(key)
    count = parse_count(count_text)
    if count < 2 or not re.fullmatch(r"[0-9]+", last_index_text):
        reject("environment metadata duplicate evidence count is invalid")
    last_index = int(last_index_text)
    if last_index < 1 or last_index > source_entries:
        reject("environment metadata duplicate evidence position is invalid")
    hashes = hashes_text.split(",")
    if len(hashes) != count or any(not re.fullmatch(r"[0-9a-f]{64}", item) for item in hashes):
        reject("environment metadata duplicate value hash list is invalid")
    if selected_hash != hashes[-1] or expected_map.get(key, selected_hash) != selected_hash:
        reject("environment metadata selected duplicate value hash is not approved")
if set(seen_rows) != set(duplicate_keys):
    reject("environment metadata duplicate evidence rows are incomplete")
if mode == "strict":
    if duplicate_keys or rows or source_entries != normalized_entries:
        reject("strict environment metadata contains duplicate evidence")
else:
    if not duplicate_keys or source_entries <= normalized_entries:
        reject("last-wins environment metadata has no duplicate entries")

lines = env_text.splitlines(keepends=True)
if any(not line.endswith("\n") or "\r" in line for line in lines):
    reject("normalized environment metadata has invalid line endings")
keys = []
for line in lines:
    entry = line[:-1]
    key, separator, value = entry.partition("=")
    if not separator or not re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*", key) or "\x00" in value or "\n" in value:
        reject("normalized environment metadata entry is malformed")
    if key in keys:
        reject("normalized environment metadata contains duplicate keys")
    keys.append(key)
if len(keys) != normalized_entries:
    reject("normalized environment metadata entry count does not match evidence")
PY
}

capture_environment_metadata() {
  local id="$1" env_file="${2:--}" evidence_file="${3:--}" phase="${4:-prepare}"
  local expected_mode="${5:-}" expected_keys="${6:-}" expected_hashes="${7:-}"
  local confirm allowlist result observed_mode observed_keys observed_hashes observed_env_sha observed_evidence_sha
  local source_count normalized_count extra
  [[ "$phase" == prepare || "$phase" == replay || "$phase" == replay-candidate ]] || fail 'environment metadata capture phase is invalid'
  if [[ "$phase" == prepare ]]; then
    confirm="${SUBNEXUS_CUTOVER_ENV_DUPLICATE_CONFIRM:-}"
    allowlist="${SUBNEXUS_CUTOVER_ENV_DUPLICATE_KEYS:-}"
    expected_mode=''
    expected_keys=''
    expected_hashes="${SUBNEXUS_CUTOVER_ENV_DUPLICATE_EXPECTED_SHA256:-}"
    validate_environment_duplicate_inputs "$allowlist" "$expected_hashes"
  else
    confirm=''
    allowlist="$expected_keys"
    validate_environment_duplicate_inputs "$allowlist" "$expected_hashes"
  fi
  result="$(docker_rpc inspect --format '{{json .Config.Env}}' "$id" |
    python3 -c '
import hashlib
import json
import os
import re
import sys

env_path, evidence_path, phase, confirm, allowlist, expected_mode, expected_keys, expected_hashes, approval_token = sys.argv[1:]

def reject(message):
    raise SystemExit(message)

def parse_keys(raw):
    if raw == "":
        return []
    values = raw.split(",")
    if values != sorted(values) or len(values) != len(set(values)):
        reject("duplicate environment key allowlist must be sorted and unique")
    if any(not re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*", value) for value in values):
        reject("duplicate environment key allowlist contains an invalid key")
    return values

def parse_hashes(raw):
    if raw == "":
        return {}
    result = {}
    for item in raw.split(","):
        key, separator, digest = item.partition("=")
        if not separator or not re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*", key) or not re.fullmatch(r"[0-9a-f]{64}", digest):
            reject("duplicate environment expected value hash is invalid")
        if key in result:
            reject("duplicate environment expected value hash is repeated")
        result[key] = digest
    if list(result) != sorted(result):
        reject("duplicate environment expected value hash list must be sorted")
    return result

try:
    raw = json.load(sys.stdin)
except (ValueError, TypeError):
    reject("Docker environment metadata is not valid JSON")
if not isinstance(raw, list):
    reject("Docker environment metadata is not an array")

entries = []
for index, item in enumerate(raw, 1):
    if not isinstance(item, str) or "=" not in item:
        reject("Docker environment metadata contains an unassigned entry")
    key, value = item.split("=", 1)
    if not re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*", key):
        reject("Docker environment metadata contains an invalid key")
    if any(char in value for char in ("\x00", "\r", "\n")):
        reject("Docker environment metadata contains a forbidden control character")
    entries.append((key, value, item, index))

occurrences = {}
for key, value, item, index in entries:
    occurrences.setdefault(key, []).append((value, item, index))
duplicate_keys = sorted(key for key, values in occurrences.items() if len(values) > 1)
allowed = parse_keys(allowlist)
approved_hashes = parse_hashes(expected_hashes)
if phase == "prepare":
    if duplicate_keys:
        if confirm != approval_token:
            reject("duplicate environment metadata requires explicit last-wins confirmation")
        if allowed != duplicate_keys:
            reject("duplicate environment metadata keys do not match the explicit allowlist")
        if set(approved_hashes) != set(duplicate_keys):
            reject("duplicate environment metadata requires an expected hash for every key")
    elif confirm or allowed or approved_hashes:
        reject("duplicate environment approval was supplied but no duplicate key exists")
elif phase in ("replay", "replay-candidate"):
    if expected_mode not in ("strict", "last-wins"):
        reject("prepared environment duplicate mode is invalid")
    if expected_mode == "strict" and duplicate_keys:
        reject("a new duplicate environment key appeared after prepare")
    if expected_mode == "last-wins":
        if allowed != duplicate_keys or set(approved_hashes) != set(duplicate_keys):
            if phase != "replay-candidate" or duplicate_keys:
                reject("prepared duplicate environment contract no longer matches")
else:
    reject("unsupported environment metadata capture phase")

mode = "last-wins" if duplicate_keys else "strict"
if phase == "replay" and mode != expected_mode:
    reject("prepared environment duplicate mode changed")
if phase == "replay-candidate" and mode not in (expected_mode, "strict"):
    reject("candidate environment duplicate mode is invalid")
for key in duplicate_keys:
    selected_hash = hashlib.sha256(occurrences[key][-1][0].encode("utf-8")).hexdigest()
    if approved_hashes.get(key) != selected_hash:
        reject("the selected last environment value does not match its approved hash")

selected = []
for key, values in occurrences.items():
    selected.append((values[-1][2], values[-1][1]))
selected.sort(key=lambda item: item[0])
normalized_entries = [item for _, item in selected]
env_blob = "".join(item + "\n" for item in normalized_entries).encode("utf-8")
source_digest = hashlib.sha256(json.dumps([item for _, _, item, _ in entries], ensure_ascii=False, separators=(",", ":")).encode("utf-8")).hexdigest()
normalized_digest = hashlib.sha256(env_blob).hexdigest()
canonical_hashes = ",".join(key + "=" + approved_hashes[key] for key in sorted(approved_hashes) if key in duplicate_keys)
evidence_lines = [
    "format=1\n",
    "mode=" + mode + "\n",
    "source_entries=" + str(len(entries)) + "\n",
    "normalized_entries=" + str(len(normalized_entries)) + "\n",
    "duplicate_keys=" + ",".join(duplicate_keys) + "\n",
    "source_env_sha256=" + source_digest + "\n",
    "normalized_env_sha256=" + normalized_digest + "\n",
]
for key in duplicate_keys:
    values = occurrences[key]
    value_hashes = [hashlib.sha256(value.encode("utf-8")).hexdigest() for value, _, _ in values]
    evidence_lines.append("duplicate|" + key + "|" + str(len(values)) + "|" + str(values[-1][2]) + "|" + ",".join(value_hashes) + "|" + value_hashes[-1] + "\n")
evidence_blob = "".join(evidence_lines).encode("utf-8")

def write_output(path, data):
    if path == "-":
        return
    flags = os.O_WRONLY | os.O_CREAT | os.O_TRUNC
    if hasattr(os, "O_NOFOLLOW"):
        flags |= os.O_NOFOLLOW
    try:
        fd = os.open(path, flags, 0o600)
        with os.fdopen(fd, "wb") as handle:
            handle.write(data)
            handle.flush()
            os.fsync(handle.fileno())
    except OSError:
        reject("cannot write environment metadata output")

write_output(env_path, env_blob)
write_output(evidence_path, evidence_blob)
print("|".join((mode, ",".join(duplicate_keys), canonical_hashes, normalized_digest,
                hashlib.sha256(evidence_blob).hexdigest(), str(len(entries)),
                str(len(normalized_entries)))))
' "$env_file" "$evidence_file" "$phase" "$confirm" "$allowlist" "$expected_mode" "$expected_keys" "$expected_hashes" "$environment_duplicate_approval_token")" ||
    fail 'cannot safely capture Docker environment metadata'
  IFS='|' read -r observed_mode observed_keys observed_hashes observed_env_sha observed_evidence_sha source_count normalized_count extra <<< "$result"
  [[ -z "${extra:-}" && "$observed_mode" =~ ^(strict|last-wins)$ && "$observed_keys" != *'|'* &&
     "$observed_hashes" != *'|'* && "$observed_env_sha" =~ ^[0-9a-f]{64}$ &&
     "$observed_evidence_sha" =~ ^[0-9a-f]{64}$ && "$source_count" =~ ^[0-9]+$ &&
     "$normalized_count" =~ ^[0-9]+$ ]] || fail 'Docker environment metadata summary is malformed'
  [[ "$normalized_count" -le "$source_count" ]] || fail 'Docker environment metadata counts are inconsistent'
  environment_observed_mode="$observed_mode"
  environment_observed_keys="$observed_keys"
  environment_observed_expected_hashes="$observed_hashes"
  environment_observed_file_sha256="$observed_env_sha"
  environment_observed_evidence_sha256="$observed_evidence_sha"
  if [[ "$phase" == prepare ]]; then
    environment_duplicate_mode="$observed_mode"
    environment_duplicate_keys="$observed_keys"
    environment_duplicate_expected_hashes="$observed_hashes"
    environment_file_sha256="$observed_env_sha"
    environment_duplicate_evidence_sha256="$observed_evidence_sha"
  fi
  if [[ "$env_file" != '-' ]]; then
    assert_root_owned_regular "$env_file" 'container environment metadata'
    assert_root_owned_regular "$evidence_file" 'container environment duplicate evidence'
    validate_environment_file "$env_file"
    validate_environment_duplicate_evidence "$evidence_file" "$env_file" "$observed_mode" "$observed_keys" "$observed_hashes"
    chmod 600 -- "$env_file" "$evidence_file"
    [[ "$(hash_file "$env_file")" == "$observed_env_sha" ]] || fail 'normalized environment metadata hash changed while capturing'
    [[ "$(hash_file "$evidence_file")" == "$observed_evidence_sha" ]] || fail 'environment duplicate evidence hash changed while capturing'
  fi
}

assert_environment_matches_prepare() {
  local id="$1" compare_source="${2:-live}"
  local expected_mode="$environment_duplicate_mode" expected_keys="$environment_duplicate_keys"
  local expected_hashes="$environment_duplicate_expected_hashes" expected_env_sha="$environment_file_sha256"
  local expected_evidence_sha="$environment_duplicate_evidence_sha256"
  local observed_mode observed_keys observed_hashes observed_env_sha observed_evidence_sha
  local capture_phase=replay
  [[ "$compare_source" == candidate ]] && capture_phase=replay-candidate
  capture_environment_metadata "$id" - - "$capture_phase" "$expected_mode" "$expected_keys" "$expected_hashes"
  observed_mode="$environment_observed_mode"
  observed_keys="$environment_observed_keys"
  observed_hashes="$environment_observed_expected_hashes"
  observed_env_sha="$environment_observed_file_sha256"
  observed_evidence_sha="$environment_observed_evidence_sha256"
  if [[ "$compare_source" == candidate && "$expected_mode" == last-wins && "$observed_mode" == strict ]]; then
    # Docker may canonicalize a duplicate override while creating the
    # replacement.  The normalized file hash still proves the selected value
    # is identical; no new duplicate is accepted in the candidate.
    [[ -z "$observed_keys" && -z "$observed_hashes" && "$observed_env_sha" == "$expected_env_sha" ]] ||
      fail 'candidate environment metadata differs from the prepared last-wins value'
  else
    [[ "$observed_mode" == "$expected_mode" && "$observed_keys" == "$expected_keys" &&
       "$observed_hashes" == "$expected_hashes" && "$observed_env_sha" == "$expected_env_sha" ]] ||
      fail 'live/candidate environment metadata differs from the prepared contract'
  fi
  if [[ "$compare_source" == live && "$environment_duplicate_legacy" != 1 ]]; then
    [[ "$observed_evidence_sha" == "$expected_evidence_sha" ]] ||
      fail 'live environment duplicate sequence differs from the prepared evidence'
  fi
}

validate_security_options_file() {
  local file="$1" line lower value nnp_seen=0
  assert_root_owned_regular "$file" 'container security option metadata'
  while IFS= read -r line; do
    [[ -n "$line" ]] || continue
    [[ "$line" != *$'\r'* && "$line" != *$'\n'* && "${#line}" -le 1024 ]] ||
      fail 'container security option metadata is malformed'
    lower="${line,,}"
    case "$lower" in
      privileged|privileged:*|privileged=*)
        fail 'privileged security options are not allowed' ;;
      seccomp=unconfined|seccomp:unconfined|apparmor=unconfined|apparmor:unconfined|\
      label=disable|label:disable|systempaths=unconfined|systempaths:unconfined)
        fail "unsafe security option is not allowed: $line" ;;
    esac
    case "$lower" in
      no-new-privileges)
        value=true
        ;;
      no-new-privileges:*)
        value="${lower#no-new-privileges:}"
        ;;
      no-new-privileges=*)
        value="${lower#no-new-privileges=}"
        ;;
      *)
        continue
        ;;
    esac
    [[ "$value" == true || "$value" == 1 ]] || fail 'security option no-new-privileges must be true'
    (( nnp_seen == 0 )) || fail 'security option metadata contains duplicate no-new-privileges entries'
    nnp_seen=$((nnp_seen + 1))
  done < "$file"
}

capture_container_identity() {
  local id="$1"
  docker_rpc inspect --format '{{.Id}}|{{.Name}}|{{.Image}}|{{.Config.Image}}|{{.State.Running}}|{{.RestartCount}}|{{if .State.Health}}{{.State.Health.Status}}{{else}}no-healthcheck{{end}}' "$id"
}

validate_runtime_contract_supported() {
  # The runtime hash intentionally covers more than the small set of options
  # accepted by docker create below. Reject a non-default option here, while
  # the live container is still running, instead of discovering the mismatch
  # after the production name has been stopped and renamed.
  docker_rpc inspect --format '{{json .}}' "$app_id" |
    python3 -c '
import json
import re
import sys

obj = json.load(sys.stdin)
config = obj.get("Config") or {}
host = obj.get("HostConfig") or {}
networks = (obj.get("NetworkSettings") or {}).get("Networks") or {}
container_id = str(obj.get("Id") or "").removeprefix("sha256:")

def nonempty(value):
    return value not in (None, "", False, 0, [], {})

def reject(condition, label):
    if condition:
        raise SystemExit("unsupported runtime contract field: " + label)

# These fields are reproduced by the default docker create behavior. A
# custom value would require an explicit create flag that this controller does
# not issue.
reject(config.get("AttachStdin") not in (None, False), "Config.AttachStdin")
reject(config.get("AttachStdout") not in (None, True), "Config.AttachStdout")
reject(config.get("AttachStderr") not in (None, True), "Config.AttachStderr")
reject(config.get("OpenStdin") not in (None, False), "Config.OpenStdin")
reject(config.get("StdinOnce") not in (None, False), "Config.StdinOnce")
reject(config.get("Tty") not in (None, False), "Config.Tty")
reject(config.get("Domainname") not in (None, ""), "Config.Domainname")
reject(config.get("Hostname") not in (None, "", container_id[:12]), "Config.Hostname")
reject(config.get("ArgsEscaped") not in (None, False), "Config.ArgsEscaped")
reject(config.get("NetworkDisabled") not in (None, False), "Config.NetworkDisabled")
reject(config.get("MacAddress") not in (None, ""), "Config.MacAddress")
reject(config.get("StopSignal") not in (None, "", "SIGTERM"), "Config.StopSignal")
reject(config.get("StopTimeout") not in (None, 0, 10), "Config.StopTimeout")
if isinstance(config.get("Cmd"), list) and len(config["Cmd"]) == 0:
    reject(True, "Config.Cmd(empty)")
if isinstance(config.get("Entrypoint"), list) and len(config["Entrypoint"]) > 1:
    reject(True, "Config.Entrypoint(multiple)")

for key in ("AutoRemove", "Privileged", "PublishAllPorts", "ReadonlyRootfs"):
    reject(host.get(key) not in (None, False), "HostConfig." + key)

# Docker exposes legacy `-v` mounts in HostConfig.Binds while the canonical
# Mounts entries contain the structured metadata used by capture_mounts.
# Accept only a simple, reproducible bind form and require every Binds entry
# to agree with its corresponding structured Mounts entry. Unsupported bind
# options (relabeling, nocopy, custom propagation, etc.) remain fail-closed.
mount_entries = obj.get("Mounts") or []
if not isinstance(mount_entries, list):
    reject(True, "Mounts")
bind_mounts = {}
for mount in mount_entries:
    if not isinstance(mount, dict):
        reject(True, "Mounts(entry)")
    if mount.get("Type") == "bind":
        source = mount.get("Source")
        destination = mount.get("Destination")
        if not isinstance(source, str) or not isinstance(destination, str):
            reject(True, "Mounts.bind.path")
        bind_mounts[(source, destination)] = mount

raw_binds = host.get("Binds") or []
if not isinstance(raw_binds, list):
    reject(True, "HostConfig.Binds")
for raw_bind in raw_binds:
    if not isinstance(raw_bind, str):
        reject(True, "HostConfig.Binds.entry")
    fields = raw_bind.split(":")
    if len(fields) not in (2, 3):
        reject(True, "HostConfig.Binds.entry")
    source, destination = fields[:2]
    if not source.startswith("/") or not destination.startswith("/"):
        reject(True, "HostConfig.Binds.path")
    options = [] if len(fields) == 2 or fields[2] == "" else fields[2].split(",")
    if len(options) != len(set(options)):
        reject(True, "HostConfig.Binds.options")
    mode = "rw"
    propagation = "rprivate"
    mode_seen = False
    propagation_seen = False
    for option in options:
        if option in ("rw", "ro"):
            if mode_seen:
                reject(True, "HostConfig.Binds.mode")
            mode = option
            mode_seen = True
        elif option in ("private", "rprivate", "shared", "rshared", "slave", "rslave"):
            if propagation_seen:
                reject(True, "HostConfig.Binds.propagation")
            propagation = option
            propagation_seen = True
        else:
            reject(True, "HostConfig.Binds.option")
    mount = bind_mounts.get((source, destination))
    if mount is None:
        reject(True, "HostConfig.Binds.unmatched")
    mount_mode = mount.get("Mode") or ("rw" if mount.get("RW") is not False else "ro")
    if mount_mode not in ("rw", "ro") or mount_mode != mode:
        reject(True, "HostConfig.Binds.mode")
    mount_propagation = mount.get("Propagation") or "rprivate"
    if mount_propagation != propagation:
        reject(True, "HostConfig.Binds.propagation")
for key in ("CapAdd", "CapDrop", "DeviceCgroupRules", "Devices", "DeviceRequests",
            "Dns", "DnsOptions", "DnsSearch", "ExtraHosts", "GroupAdd",
            "Links", "StorageOpt", "Sysctls", "Tmpfs", "VolumesFrom"):
    reject(nonempty(host.get(key)), "HostConfig." + key)
for key in ("BlkioWeightDevice", "BlkioDeviceReadBps", "BlkioDeviceWriteBps",
            "BlkioDeviceReadIOps", "BlkioDeviceWriteIOps"):
    reject(nonempty(host.get(key)), "HostConfig." + key)
reject(host.get("BlkioWeight") not in (None, 0), "HostConfig.BlkioWeight")
reject(host.get("CgroupParent") not in (None, ""), "HostConfig.CgroupParent")
reject(host.get("Init") not in (None, False), "HostConfig.Init")
reject(host.get("KernelMemory") not in (None, 0), "HostConfig.KernelMemory")
reject(host.get("KernelMemoryTCP") not in (None, 0), "HostConfig.KernelMemoryTCP")
reject(host.get("VolumeDriver") not in (None, ""), "HostConfig.VolumeDriver")
reject(host.get("MemorySwappiness") not in (None, -1), "HostConfig.MemorySwappiness")
reject(host.get("OomKillDisable") not in (None, False), "HostConfig.OomKillDisable")
reject(host.get("OomScoreAdj") not in (None, 0), "HostConfig.OomScoreAdj")
reject(host.get("CpuCount") not in (None, 0), "HostConfig.CpuCount")
reject(host.get("CpuPercent") not in (None, 0), "HostConfig.CpuPercent")
reject(host.get("CpuRealtimePeriod") not in (None, 0), "HostConfig.CpuRealtimePeriod")
reject(host.get("CpuRealtimeRuntime") not in (None, 0), "HostConfig.CpuRealtimeRuntime")
reject(host.get("CpusetMems") not in (None, ""), "HostConfig.CpusetMems")
reject(host.get("Isolation") not in (None, ""), "HostConfig.Isolation")
reject(host.get("PidMode") not in (None, ""), "HostConfig.PidMode")
reject(host.get("UTSMode") not in (None, ""), "HostConfig.UTSMode")
reject(host.get("UsernsMode") not in (None, ""), "HostConfig.UsernsMode")
reject(host.get("Runtime") not in (None, "", "runc"), "HostConfig.Runtime")
console_size = host.get("ConsoleSize")
if console_size not in (None, [0, 0]):
    # Docker 29 records historical console dimensions even when Tty=false.
    # They are inert in that mode and docker create resets them to [0, 0], so
    # accept only two non-negative integer dimensions and normalize them in
    # the runtime contract below. Tty=true remains rejected above.
    if (config.get("Tty") not in (None, False) or
        not isinstance(console_size, list) or len(console_size) != 2 or
        any(type(value) is not int or value < 0 for value in console_size)):
        reject(True, "HostConfig.ConsoleSize")
reject(host.get("ShmSize") not in (None, 0, 67108864), "HostConfig.ShmSize")

ipc_mode = host.get("IpcMode")
reject(ipc_mode not in (None, "", "private"), "HostConfig.IpcMode")
cgroupns_mode = host.get("CgroupnsMode")
reject(cgroupns_mode not in (None, "", "private"), "HostConfig.CgroupnsMode")

log_config = host.get("LogConfig") or {}
if not isinstance(log_config, dict):
    reject(True, "HostConfig.LogConfig")
reject(any(key not in ("Type", "Config") for key in log_config),
       "HostConfig.LogConfig.key")
log_type = log_config.get("Type")
reject(log_type not in (None, "", "json-file"), "HostConfig.LogConfig.Type")
log_options = log_config.get("Config")
if log_options in (None, {}):
    log_options = {}
reject(not isinstance(log_options, dict), "HostConfig.LogConfig.Config")
allowed_log_options = {"max-file", "max-size"}
reject(any(not isinstance(key, str) or key not in allowed_log_options
          for key in log_options), "HostConfig.LogConfig.Config.key")
for key, value in log_options.items():
    reject(not isinstance(value, str), "HostConfig.LogConfig.Config.value")
    if key == "max-file":
        reject(not re.fullmatch(r"[1-9][0-9]{0,3}", value) or int(value) > 1000,
              "HostConfig.LogConfig.Config.max-file")
    elif key == "max-size":
        # Docker json-file accepts a positive byte count with an
        # optional binary unit.  Disallow zero/unlimited values so rotation
        # cannot silently be disabled during a cutover.
        reject(not re.fullmatch(r"[1-9][0-9]{0,9}(?:[bBkKmMgGtT])?", value),
              "HostConfig.LogConfig.Config.max-size")
if log_options:
    reject(set(log_options) != allowed_log_options,
           "HostConfig.LogConfig.Config.required")

network_names = sorted(str(name) for name in networks)
reject(not network_names, "NetworkSettings.Networks(empty)")
network_mode = host.get("NetworkMode")
first_network = network_names[0] if network_names else ""
first_id = str((networks.get(first_network) or {}).get("NetworkID") or "")
reject(network_mode not in (None, "", first_network, first_id), "HostConfig.NetworkMode")

for mount in obj.get("Mounts") or []:
    if not isinstance(mount, dict):
        reject(True, "Mounts(entry)")
    mode = mount.get("Mode") or ""
    reject(mode not in ("", "rw", "ro"), "Mounts.Mode")
    if mount.get("Type") == "volume":
        propagation = mount.get("Propagation") or ""
        reject(propagation not in ("", "rprivate"), "Mounts.volume.Propagation")

for mount in host.get("Mounts") or []:
    if not isinstance(mount, dict):
        reject(True, "HostConfig.Mounts(entry)")
    mount_type = mount.get("Type")
    reject(mount_type not in ("bind", "volume"), "HostConfig.Mounts.Type")
    reject(mount.get("Consistency") not in (None, ""), "HostConfig.Mounts.Consistency")
    bind_options = mount.get("BindOptions") or {}
    if not isinstance(bind_options, dict):
        reject(True, "HostConfig.Mounts.BindOptions")
    reject((bind_options.get("Propagation") or "") not in ("", "private", "rprivate", "shared", "rshared", "slave", "rslave"), "HostConfig.Mounts.BindOptions.Propagation")
    for key, value in bind_options.items():
        if key != "Propagation":
            reject(nonempty(value), "HostConfig.Mounts.BindOptions." + str(key))
    volume_options = mount.get("VolumeOptions") or {}
    if not isinstance(volume_options, dict):
        reject(True, "HostConfig.Mounts.VolumeOptions")
    for key, value in volume_options.items():
        reject(nonempty(value), "HostConfig.Mounts.VolumeOptions." + str(key))
' || fail 'live application runtime contract contains options this cutover cannot reproduce safely'
}

capture_runtime_contract_hash() {
  local id="$1"
  # Hash only stable runtime configuration. Container IDs, names, process
  # state, network endpoint IDs/IPs, and image IDs are intentionally excluded;
  # all other fields that can change application behavior or isolation are
  # compared after the candidate is created. Secret environment values stay
  # inside the pipe and are never printed. When the prepared contract explicitly
  # approved a duplicate environment key, canonicalize the array by retaining
  # its last occurrence (the Docker override semantics); strict contracts still
  # reject any duplicate encountered here.
  docker_rpc inspect --format '{{json .}}' "$id" |
    python3 -c '
import hashlib
import json
import shlex
import sys

runtime_env_mode = sys.argv[1] if len(sys.argv) > 1 else "strict"
if runtime_env_mode not in ("strict", "last-wins"):
    raise SystemExit("invalid runtime environment mode")
obj = json.load(sys.stdin)
config = obj.get("Config") or {}
host = obj.get("HostConfig") or {}

config_keys = (
    "Args", "AttachStderr", "AttachStdin", "AttachStdout", "Cmd", "Domainname",
    "Entrypoint", "Env", "ExposedPorts", "Healthcheck", "OpenStdin", "StdinOnce",
    "Shell", "StopSignal", "StopTimeout", "Tty", "User", "Volumes", "WorkingDir",
)
host_keys = (
    "AutoRemove", "Binds", "CapAdd", "CapDrop", "CgroupnsMode", "ConsoleSize",
    "CpuCount", "CpuPercent", "CpuPeriod", "CpuQuota", "CpuRealtimePeriod",
    "CpuRealtimeRuntime", "CpuShares", "CpusetCpus", "CpusetMems", "DeviceCgroupRules",
    "Devices", "DeviceRequests", "Dns", "DnsOptions", "DnsSearch", "ExtraHosts",
    "GroupAdd", "IpcMode", "Isolation", "LogConfig", "Memory", "MemoryReservation",
    "MemorySwap", "MemorySwappiness", "NanoCpus", "NetworkMode", "OomKillDisable",
    "OomScoreAdj", "PidMode", "PidsLimit", "PortBindings", "Privileged", "PublishAllPorts",
    "ReadonlyRootfs", "RestartPolicy", "Runtime", "ShmSize", "Tmpfs", "Ulimits",
    "UTSMode", "UsernsMode", "VolumesFrom", "MaskedPaths", "ReadonlyPaths",
)

def clean(value):
    if isinstance(value, dict):
        return {str(key): clean(value[key]) for key in sorted(value)}
    if isinstance(value, list):
        return [clean(item) for item in value]
    return value

def normalize_healthcheck(value):
    # docker create exposes only a string health command.  Canonicalize an
    # exec-form CMD to the shell-safe equivalent produced by shlex.join so a
    # Compose CMD override remains comparable after recreation.
    if not isinstance(value, dict):
        value = {"Test": ["NONE"]}
    test = value.get("Test")
    if not isinstance(test, list) or not test:
        raise SystemExit("invalid healthcheck test")
    if not all(isinstance(item, str) for item in test):
        raise SystemExit("invalid healthcheck test item")
    if test[0] == "NONE":
        if len(test) != 1:
            raise SystemExit("invalid healthcheck NONE form")
        test = ["NONE"]
    elif test[0] == "CMD":
        if len(test) < 2:
            raise SystemExit("invalid healthcheck CMD form")
        test = ["CMD-SHELL", shlex.join(test[1:])]
    elif test[0] == "CMD-SHELL":
        if len(test) != 2:
            raise SystemExit("invalid healthcheck CMD-SHELL form")
        test = test[:2]
    else:
        raise SystemExit("unsupported healthcheck test form")
    normalized = {"Test": test}
    for key in ("Interval", "Timeout", "StartPeriod", "StartInterval", "Retries"):
        raw = value.get(key, 0)
        if raw is None:
            raw = 0
        if not isinstance(raw, int) or raw < 0:
            raise SystemExit("invalid healthcheck timing")
        normalized[key] = raw
    return normalized

def normalize_ulimits(value):
    entries = []
    for item in value or []:
        if not isinstance(item, dict):
            raise SystemExit("invalid ulimit entry")
        name = item.get("Name")
        soft = item.get("Soft")
        hard = item.get("Hard")
        if not isinstance(name, str) or not isinstance(soft, int) or not isinstance(hard, int):
            raise SystemExit("invalid ulimit value")
        entries.append({"Name": name, "Soft": soft, "Hard": hard})
    return sorted(entries, key=lambda item: (str(item.get("Name")), int(item.get("Soft") or 0), int(item.get("Hard") or 0)))

def normalize_mount_mode(value):
    # Empty and explicit rw are the same semantic mount mode when recreated
    # with --mount. Relabel modes (z/Z) and any unknown option cannot be
    # reproduced by this controller and are rejected before the cutover.
    mode = value or ""
    if mode in ("", "rw"):
        return "rw"
    if mode == "ro":
        return "ro"
    raise SystemExit("unsupported mount mode")

def normalize_mount_propagation(value):
    # Docker commonly reports rprivate even when no propagation flag was
    # supplied. Normalize the equivalent empty form for stable comparison.
    propagation = value or ""
    return "rprivate" if propagation in ("", "rprivate") else propagation

# The cutover controller deliberately adds this harmless hardening option to
# the candidate. Treat it as part of the expected contract even when the old
# container was created without it; all existing security options remain
# required and are compared as a sorted set. Reject every explicit false or
# malformed spelling, including both `:` and `=` forms, so a conflicting
# option cannot be silently combined with the hardening option.
security = set()
for item in (host.get("SecurityOpt") or []):
    text = str(item)
    lower = text.lower()
    if lower == "no-new-privileges" or lower.startswith("no-new-privileges:") or lower.startswith("no-new-privileges="):
        if lower == "no-new-privileges":
            value = "true"
        else:
            value = lower.split(":", 1)[1] if ":" in lower else lower.split("=", 1)[1]
        if value not in ("true", "1"):
            raise SystemExit("conflicting no-new-privileges security option")
        continue
    security.add(text)
security.add("no-new-privileges:true")

mounts = []
for mount in obj.get("Mounts") or []:
    normalized_mount = {key: mount.get(key) for key in ("Type", "Name", "Source", "Destination", "RW")}
    normalized_mount["Mode"] = normalize_mount_mode(mount.get("Mode"))
    normalized_mount["Propagation"] = normalize_mount_propagation(mount.get("Propagation"))
    mounts.append(normalized_mount)
mounts.sort(key=lambda item: (str(item.get("Destination")), str(item.get("Type")), str(item.get("Name"))))

# HostConfig.Binds is a legacy string representation. Normalize it to the
# structured Mounts form so a live `-v` bind and a replacement `--mount` bind
# produce the same runtime contract hash.
contract_bind_mounts = []
for mount in mounts:
    if mount.get("Type") == "bind":
        contract_bind_mounts.append({
            "Source": mount.get("Source"),
            "Destination": mount.get("Destination"),
            "Mode": mount.get("Mode"),
            "RW": mount.get("RW"),
            "Propagation": mount.get("Propagation"),
        })

networks = {}
app_name = str(obj.get("Name") or "").lstrip("/")
container_id = str(obj.get("Id") or "")
container_id_hex = container_id.removeprefix("sha256:")
dynamic_aliases = {
    value for value in (app_name, container_id, container_id_hex,
                        container_id_hex[:12]) if value
}
for name, network in sorted((obj.get("NetworkSettings") or {}).get("Networks" or {}).items()):
    # Endpoint IDs and addresses are allocated anew when the replacement is
    # attached; aliases and driver options are the stable name-resolution
    # contract that must remain unchanged.
    aliases = sorted(set(alias for alias in (network.get("Aliases") or [])
                         if alias not in dynamic_aliases))
    networks[name] = {
        "Aliases": aliases,
        "DriverOpts": clean(network.get("DriverOpts")),
    }

contract = {
    "Config": {key: (normalize_healthcheck(config.get(key)) if key == "Healthcheck" else clean(config.get(key))) for key in config_keys},
    "HostConfig": {key: (normalize_ulimits(host.get(key)) if key == "Ulimits" else clean(host.get(key))) for key in host_keys},
    "HostConfigSecurityOptNormalized": sorted(str(item) for item in security),
    "Mounts": mounts,
    "Networks": networks,
}
contract["Config"]["Healthcheck"] = normalize_healthcheck(config.get("Healthcheck"))
contract["HostConfig"]["Binds"] = contract_bind_mounts
if config.get("Tty") in (None, False):
    contract["HostConfig"]["ConsoleSize"] = [0, 0]
# Docker may serialize an omitted log driver/type as null while `docker
# create` reports the daemon default explicitly.  The supported contract is
# json-file, so canonicalize both forms before hashing.
log_contract = contract["HostConfig"].get("LogConfig") or {}
if isinstance(log_contract, dict):
    contract["HostConfig"]["LogConfig"] = {
        "Type": log_contract.get("Type") or "json-file",
        "Config": log_contract.get("Config") or {},
    }
for key in ("Env", "ExposedPorts"):
    value = contract["Config"].get(key)
    if isinstance(value, list):
        if key == "Env":
            latest = {}
            for item in value:
                if not isinstance(item, str) or "=" not in item:
                    raise SystemExit("invalid runtime environment entry")
                env_key = item.split("=", 1)[0]
                if env_key in latest and runtime_env_mode != "last-wins":
                    raise SystemExit("duplicate runtime environment entry")
                latest[env_key] = item
            value = list(latest.values())
        contract["Config"][key] = sorted(value, key=lambda item: json.dumps(item, sort_keys=True, separators=(",", ":")))
for key in ("CapAdd", "CapDrop", "Dns", "DnsOptions", "DnsSearch", "ExtraHosts", "GroupAdd", "Ulimits", "VolumesFrom"):
    value = contract["HostConfig"].get(key)
    if isinstance(value, list):
        contract["HostConfig"][key] = sorted(value, key=lambda item: json.dumps(item, sort_keys=True, separators=(",", ":")))
contract["HostConfig"]["SecurityOpt"] = sorted(str(item) for item in security)
if contract["HostConfig"].get("PidsLimit") is None:
    # Docker API serializes an unset pointer as null while the CLI resource
    # metadata uses the controller canonical zero (unlimited) value.
    contract["HostConfig"]["PidsLimit"] = 0
# Older Docker-created containers can retain a null OomKillDisable pointer,
# while Docker 29 serializes the same enabled-default behavior as false when
# recreating the container. Keep true distinct so an unsafe value cannot be
# hidden by normalization.
if contract["HostConfig"].get("OomKillDisable") is None:
    contract["HostConfig"]["OomKillDisable"] = False
for key in ("CapAdd", "CapDrop"):
    value = contract["HostConfig"].get(key)
    if isinstance(value, list):
        normalized = []
        for item in value:
            token = str(item).upper()
            if token.startswith("CAP_"):
                token = token[4:]
            normalized.append(token)
        contract["HostConfig"][key] = sorted(normalized)
print(hashlib.sha256(json.dumps(contract, sort_keys=True, separators=(",", ":")).encode("utf-8")).hexdigest())
 ' "${environment_duplicate_mode:-strict}"
}

capture_dependency_identity() {
  local id="$1"
  # Exclude transient state (running/restart count/health) while retaining the
  # immutable container identity and the resource/security contract used by
  # the live application.
  docker_rpc inspect --format '{{.Id}}|{{.Name}}|{{.Image}}|{{.Config.Image}}|{{.HostConfig.NetworkMode}}|{{.HostConfig.RestartPolicy.Name}}|{{.HostConfig.Memory}}|{{.HostConfig.MemorySwap}}|{{.HostConfig.NanoCpus}}|{{.HostConfig.PidsLimit}}|{{.HostConfig.Privileged}}' "$id"
}

inspect_container_id_or_empty() {
  local ref="$1" output rc normalized id
  # As above, keep the caller's errexit state intact while distinguishing an
  # expected missing name from a daemon/transport failure.
  if output="$(docker_rpc inspect --format '{{.Id}}' "$ref" 2>&1)"; then
    id="$(printf '%s' "$output" | tr -d '\r\n')"
    id="${id#sha256:}"
    [[ "$id" =~ ^[0-9a-f]{64}$ ]] || fail "Docker returned an invalid container identity for $ref"
    printf '%s' "$id"
    return 0
  else
    rc=$?
  fi
  normalized="${output,,}"
  case "$normalized" in
    *'no such object:'*|*'no such container:'*)
      # Missing is an expected state for a name/ID lookup during rollback;
      # preserve a successful empty result so callers can branch safely.
      return 0
      ;;
    *) fail "cannot inspect Docker container $ref (rc=$rc)" ;;
  esac
}

inspect_container_state_or_missing() {
  local ref="$1" id state
  id="$(inspect_container_id_or_empty "$ref")" || fail "cannot inspect Docker container $ref"
  if [[ -z "$id" ]]; then
    printf 'missing'
    return 0
  fi
  state="$(docker_rpc inspect --format '{{.State.Status}}' "$id")" || fail "cannot inspect Docker container state $ref"
  [[ "$state" =~ ^(created|running|paused|restarting|removing|exited|dead)$ ]] ||
    fail "Docker returned an invalid state for $ref"
  printf '%s' "$state"
}

assert_dependency_identity() {
  local role="$1" id="$2" identity_file="$3" expected actual running
  expected="$(read_one_line "$identity_file")"
  actual="$(capture_dependency_identity "$id")" || fail "cannot inspect $role dependency"
  [[ "$actual" == "$expected" ]] || fail "$role dependency identity changed"
  running="$(docker_rpc inspect --format '{{.State.Running}}' "$id")" || fail "cannot inspect $role dependency state"
  [[ "$running" == true ]] || fail "$role dependency is not running"
}

assert_dependencies_still_match() {
  local db_id redis_id
  db_id="${1:-$(manifest_value database_id)}"
  redis_id="${2:-$(manifest_value redis_id)}"
  valid_container_ref "$db_id" || fail 'PostgreSQL dependency ID is invalid'
  valid_container_ref "$redis_id" || fail 'Redis dependency ID is invalid'
  [[ "$db_id" != "$redis_id" ]] || fail 'PostgreSQL and Redis IDs must differ'
  assert_dependency_identity database "$db_id" "$run_dir/database.identity"
  assert_dependency_identity redis "$redis_id" "$run_dir/redis.identity"
}

assert_daemon_still_matches_prepare() {
  local expected actual expected_socket actual_socket
  expected="$(manifest_value docker_daemon_identity)"
  [[ -n "$expected" ]] || fail 'prepared Docker daemon identity is missing'
  actual="$(docker_rpc info --format '{{.ID}}|{{.Name}}|{{.ServerVersion}}|{{.DockerRootDir}}|{{json .SecurityOptions}}')" || fail 'cannot recapture Docker daemon identity'
  [[ "$actual" == "$expected" ]] || fail 'Docker daemon identity changed after prepare'
  expected_socket="$(manifest_value docker_socket_fingerprint)"
  [[ -n "$expected_socket" ]] || fail 'prepared Docker socket fingerprint is missing'
  actual_socket="$docker_socket_fingerprint"
  [[ "$actual_socket" == "$expected_socket" ]] || fail 'Docker socket identity changed after prepare'
}

assert_live_identity() {
  local phase="$1" expected actual
  expected="$(read_one_line "$run_dir/live.identity")"
  actual="$(capture_container_identity "$app_id")" || fail "cannot inspect live app during $phase"
  [[ "$actual" == "$expected" ]] || fail "live app identity changed during $phase"
}

capture_networks() {
  docker_rpc inspect --format '{{range $name, $network := .NetworkSettings.Networks}}{{printf "%s|%s|%s|%s\n" $name $network.NetworkID $network.IPAddress $network.GlobalIPv6Address}}{{end}}' "$1" | LC_ALL=C sort
}

resolve_shared_container() {
  local endpoint="$1" role="$2" network_name network_id members member_id member_name member_ipv4 member_ipv6 aliases alias_network alias_name matched existing
  local -a matches=()
  for network_name in "${app_networks[@]}"; do
    network_id="$(docker_rpc network inspect --format '{{.Id}}' "$network_name")" || fail "cannot inspect network $network_name"
    members="$(docker_rpc network inspect --format '{{range $id, $container := .Containers}}{{printf "%s|%s|%s|%s\n" $id $container.Name $container.IPv4Address $container.IPv6Address}}{{end}}' "$network_name")" ||
      fail "cannot inspect members of network $network_name"
    while IFS='|' read -r member_id member_name member_ipv4 member_ipv6; do
      [[ -n "$member_id" && -n "$member_name" ]] || continue
      member_name="${member_name#/}"
      matched=false
      if [[ "$endpoint" == "$member_name" || ( -n "$member_ipv4" && "$endpoint" == "${member_ipv4%%/*}" ) || ( -n "$member_ipv6" && "$endpoint" == "${member_ipv6%%/*}" ) ]]; then
        matched=true
      else
        aliases="$(docker_rpc inspect --format '{{range $networkName, $network := .NetworkSettings.Networks}}{{range $network.Aliases}}{{printf "%s|%s\n" $networkName .}}{{end}}{{end}}' "$member_id")" ||
          fail "cannot inspect aliases for $member_name"
        while IFS='|' read -r alias_network alias_name; do
          [[ "$alias_network" == "$network_name" && "$alias_name" == "$endpoint" ]] && matched=true
        done <<< "$aliases"
      fi
      if [[ "$matched" == true ]]; then
        for existing in "${matches[@]}"; do [[ "${existing%%|*}" == "$member_id" ]] && matched=false; done
        [[ "$matched" == true ]] && matches+=("$member_id|$member_name|$network_name|$network_id|$member_ipv4|$member_ipv6")
      fi
    done <<< "$members"
  done
  [[ "${#matches[@]}" -eq 1 ]] || fail "$role endpoint '$endpoint' must resolve to exactly one container on an app network"
  printf '%s' "${matches[0]}"
}

capture_ports_and_select_health_port() {
  local line container_port host_ip host_port extra selected=''
  captured_ports=()
  while IFS='|' read -r container_port host_ip host_port extra; do
    [[ -z "${extra:-}" && -n "${container_port:-}" ]] || continue
    [[ "$container_port" =~ ^[0-9]+/(tcp|udp|sctp)$ ]] || fail 'live port binding has an invalid container port'
    valid_port "${container_port%%/*}" || fail 'live port binding has an invalid container port number'
    [[ "$host_ip" != *[,[:space:]]* && "$host_ip" != *'|'* ]] || fail 'live port binding host IP contains unsupported characters'
    valid_host_ip "$host_ip" || fail 'live port binding host IP is not loopback/all-interfaces'
    valid_port "$host_port" || fail 'live port binding has an invalid host port'
    if [[ "$container_port" == '8080/tcp' ]]; then
      [[ -z "$selected" ]] || fail 'multiple 8080/tcp host bindings are unsupported'
      selected="$host_port|$host_ip"
    fi
    captured_ports+=("$container_port|$host_ip|$host_port")
  done < <(docker_rpc inspect --format '{{range $containerPort, $bindings := .HostConfig.PortBindings}}{{range $bindings}}{{printf "%s|%s|%s\n" $containerPort .HostIp .HostPort}}{{end}}{{end}}' "$app_id")
  [[ -n "$selected" ]] || fail 'live app has no published 8080/tcp binding'
  local_port="${selected%%|*}"
  selected_host_ip="${selected#*|}"
  printf '%s\n' "${captured_ports[@]}" > "$run_dir/ports.txt"
}

capture_mounts() {
  local line type source name destination mode writable propagation extra app_data_count=0
  captured_mounts=()
  while IFS='|' read -r type source name destination mode writable propagation extra; do
    # Docker's Go template output can contain one template newline plus the
    # CLI's terminating newline. Ignore only a completely empty record;
    # partially populated records remain hard failures.
    [[ -z "${type:-}" && -z "${source:-}" && -z "${name:-}" &&
       -z "${destination:-}" && -z "${mode:-}" && -z "${writable:-}" &&
       -z "${propagation:-}" && -z "${extra:-}" ]] && continue
    [[ -z "${extra:-}" && -n "${type:-}" && -n "${destination:-}" ]] || fail 'live mount metadata is malformed'
    [[ "$destination" != *$'\n'* && "$destination" != *$'\r'* && "$destination" != *'|'* && "$destination" != *','* && "$destination" != *'='* &&
       "$source" != *$'\n'* && "$source" != *$'\r'* && "$source" != *'|'* && "$source" != *','* &&
       "$name" != *$'\n'* && "$name" != *$'\r'* && "$name" != *'|'* && "$name" != *','* && "$name" != *'='* ]] ||
      fail 'live mount path contains an unsupported delimiter'
    [[ "$destination" == /* ]] || fail 'live mount destination must be absolute'
    case "$type" in bind|volume) ;; *) fail "unsupported live mount type: $type" ;; esac
    [[ "$mode" == rw || "$mode" == ro || -z "$mode" ]] || fail 'live mount mode cannot be reproduced safely'
    [[ "$writable" == true || "$writable" == false ]] || fail 'live mount writable flag is invalid'
    if [[ "$mode" == ro ]]; then
      [[ "$writable" == false ]] || fail 'read-only mount mode conflicts with writable flag'
    elif [[ "$mode" == rw || -z "$mode" ]]; then
      [[ "$writable" == true ]] || fail 'read-write mount mode conflicts with writable flag'
    fi
    [[ -z "$propagation" || "$propagation" =~ ^(private|rprivate|shared|rshared|slave|rslave)$ ]] ||
      fail 'live mount propagation is invalid'
    if [[ "$type" == volume && -n "$propagation" && "$propagation" != rprivate ]]; then
      fail 'non-default volume mount propagation cannot be reproduced safely'
    fi
    if [[ "$type" == bind ]]; then
      [[ "$source" == /* ]] || fail 'bind mount source must be absolute'
      [[ -e "$source" && ! -L "$source" ]] || fail 'bind mount source is missing or symbolic'
      [[ "$(realpath -e -P -- "$source")" == "$source" ]] || fail 'bind mount source contains a symbolic-link component'
    else
      [[ "$name" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]{0,254}$ ]] || fail 'volume mount name is invalid'
    fi
    captured_mounts+=("$type|$source|$name|$destination|$mode|$writable|$propagation")
    if [[ "$destination" == '/app/data' ]]; then
      app_data_count=$((app_data_count + 1))
      app_data_type="$type"
      app_data_source="$source"
      app_data_name="$name"
      app_data_writable="$writable"
    fi
  done < <(docker_rpc inspect --format '{{range .Mounts}}{{printf "%s|%s|%s|%s|%s|%t|%s\n" .Type .Source .Name .Destination .Mode .RW .Propagation}}{{end}}' "$app_id")
  [[ "$app_data_count" -eq 1 ]] || fail 'exactly one /app/data mount is required'
  [[ "$app_data_writable" == true ]] || fail '/app/data must be writable for the candidate'
  printf '%s\n' "${captured_mounts[@]}" > "$run_dir/mounts.txt"
  printf '%s|%s|%s\n' "$app_data_type" "$app_data_source" "$app_data_name" > "$run_dir/app-data-mount.txt"
}

validate_mount_recreation_contract() {
  local type source name destination mode writable propagation extra
  local count=0
  local -A destinations=()
  while IFS='|' read -r type source name destination mode writable propagation extra; do
    [[ -n "$type" && -n "$destination" && -z "${extra:-}" ]] || fail 'mount recreation metadata is malformed'
    [[ "$destination" == /* && "$destination" != *$'\r'* && "$destination" != *'|'* && "$destination" != *','* && "$destination" != *'='* ]] ||
      fail 'mount recreation destination is invalid'
    [[ -z "${destinations[$destination]+x}" ]] || fail "duplicate mount destination cannot be reproduced: $destination"
    destinations[$destination]=1
    [[ "$mode" == rw || "$mode" == ro || -z "$mode" ]] || fail 'mount recreation mode is unsupported'
    [[ "$writable" == true || "$writable" == false ]] || fail 'mount recreation writable flag is invalid'
    if [[ "$mode" == ro ]]; then
      [[ "$writable" == false ]] || fail 'read-only mount mode conflicts with writable flag'
    else
      [[ "$writable" == true ]] || fail 'read-write mount mode conflicts with writable flag'
    fi
    # The controller emits --mount and can reproduce Docker's default
    # rprivate propagation. Shared/slave propagation and relabel modes require
    # flags that are not safely inferable during a production window.
    [[ -z "$propagation" || "$propagation" == rprivate ]] ||
      fail 'non-default mount propagation cannot be reproduced safely'
    case "$type" in
      bind)
        [[ "$source" == /* && -d "$source" || "$source" == /* && -f "$source" ]] || fail 'bind mount source is invalid'
        [[ ! -L "$source" && "$(realpath -e -P -- "$source")" == "$source" ]] || fail 'bind mount source is symbolic'
        ;;
      volume)
        [[ "$name" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]{0,254}$ ]] || fail 'volume mount name is invalid'
        ;;
      *) fail "unsupported mount type: $type" ;;
    esac
    count=$((count + 1))
  done < "$run_dir/mounts.txt"
  (( count > 0 )) || fail 'mount recreation metadata is empty'
}

capture_container_arguments() {
  local field="$1" output_file="$2"
  case "$field" in Cmd|Entrypoint) ;; *) fail 'unsupported container argument field' ;; esac
  # Parse the array before emitting records: Docker appends a CLI newline to
  # template output, which must not become an extra empty command argument.
  docker_rpc inspect --format "{{json .Config.$field}}" "$app_id" |
    python3 -c '
import json,sys
value=json.load(sys.stdin)
if value is None:
    value=[]
if not isinstance(value,list) or any(not isinstance(arg,str) or any(c in arg for c in ("\r","\n","\x00")) for arg in value):
    raise SystemExit("container arguments must be strings without CR, LF or NUL")
sys.stdout.write("".join(arg + "\n" for arg in value))
' > "$output_file" || fail "cannot capture container arguments: $field"
}

capture_runtime_metadata() {
  local env_file="$run_dir/container.env" entrypoint_line cmd_line security_line network_id metadata_file
  docker_rpc inspect --format '{{.Id}}' "$live_app_ref" > /dev/null || fail "live application container not found: $live_app_ref"
  app_id="$(docker_rpc inspect --format '{{.Id}}' "$live_app_ref")" || fail 'cannot identify live application container'
  app_id="${app_id#sha256:}"
  valid_sha64 "$app_id" || fail 'live application container ID is invalid'
  app_name="$(docker_rpc inspect --format '{{.Name}}' "$app_id")" || fail 'cannot identify live application name'
  app_name="${app_name#/}"
  valid_container_ref "$app_name" || fail 'live application name is invalid'
  [[ "$(docker_rpc inspect --format '{{.State.Running}}' "$app_id")" == true ]] || fail 'live application is not running'
  live_health="$(docker_rpc inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}running{{end}}' "$app_id")"
  [[ "$live_health" == healthy || "$live_health" == running ]] || fail "live application is not healthy: $live_health"
  app_image_id="$(docker_rpc inspect --format '{{.Image}}' "$app_id")" || fail 'cannot capture live application image ID'
  validate_runtime_contract_supported
  # Keep an operator-readable inspect snapshot without copying credentials
  # from Config.Env into the evidence directory.  The unredacted JSON is used
  # only through the pipe in capture_runtime_contract_hash and is never saved.
  docker_rpc inspect --format '{{json .}}' "$app_id" |
    python3 -c '
import json,sys
obj=json.load(sys.stdin)
config=obj.get("Config")
if isinstance(config,dict):
    config["Env"]=["<redacted>"] if config.get("Env") else []
print(json.dumps(obj, sort_keys=True, separators=(",", ":")))
 ' > "$run_dir/live-app.inspect.json"
  capture_environment_metadata "$app_id" "$env_file" "$run_dir/environment-duplicates.tsv" prepare
  docker_rpc inspect --format '{{range $name, $network := .NetworkSettings.Networks}}{{println $name}}{{end}}' "$app_id" | sed '/^[[:space:]]*$/d' > "$run_dir/networks.txt"
  docker_rpc inspect --format '{{range .HostConfig.SecurityOpt}}{{println .}}{{end}}' "$app_id" | sed '/^[[:space:]]*$/d' > "$run_dir/security-opt.txt"
  validate_security_options_file "$run_dir/security-opt.txt"
  docker_rpc inspect --format '{{.HostConfig.RestartPolicy.Name}}' "$app_id" > "$run_dir/restart-policy.txt"
  docker_rpc inspect --format '{{.HostConfig.RestartPolicy.MaximumRetryCount}}' "$app_id" > "$run_dir/restart-retries.txt"
  docker_rpc inspect --format '{{.Config.WorkingDir}}' "$app_id" > "$run_dir/workdir.txt"
  capture_container_arguments Entrypoint "$run_dir/entrypoint.txt"
  capture_container_arguments Cmd "$run_dir/cmd.txt"
  docker_rpc inspect --format '{{.Config.User}}' "$app_id" > "$run_dir/user.txt"
  docker_rpc inspect --format '{{.HostConfig.Memory}}|{{.HostConfig.MemorySwap}}|{{.HostConfig.MemoryReservation}}|{{.HostConfig.NanoCpus}}|{{.HostConfig.CpuShares}}|{{.HostConfig.CpuQuota}}|{{.HostConfig.CpuPeriod}}|{{.HostConfig.CpusetCpus}}|{{if .HostConfig.PidsLimit}}{{.HostConfig.PidsLimit}}{{else}}0{{end}}' "$app_id" > "$run_dir/resource-policy.txt"
  docker_rpc inspect --format '{{json .Config.Healthcheck}}' "$app_id" > "$run_dir/healthcheck.json"
  docker_rpc inspect --format '{{json .HostConfig.LogConfig}}' "$app_id" > "$run_dir/log-config.json"
  validate_log_config_file "$run_dir/log-config.json"
  docker_rpc inspect --format '{{json .HostConfig.Ulimits}}' "$app_id" |
    python3 -c '
import json,sys
value=json.load(sys.stdin)
if value is None: value=[]
if not isinstance(value,list): raise SystemExit(1)
out=[]
for item in value:
    if not isinstance(item,dict): raise SystemExit(1)
    name=item.get("Name"); soft=item.get("Soft"); hard=item.get("Hard")
    if not isinstance(name,str) or not isinstance(soft,int) or not isinstance(hard,int): raise SystemExit(1)
    out.append((name,soft,hard))
for name,soft,hard in sorted(out): print(f"{name}|{soft}|{hard}")
' > "$run_dir/ulimits.txt"
  docker_rpc inspect --format '{{json .NetworkSettings.Networks}}' "$app_id" |
    python3 -c '
import json,re,sys
value=json.load(sys.stdin)
if not isinstance(value,dict): raise SystemExit(1)
app_name=sys.argv[1]; app_id=sys.argv[2]
app_id_hex=app_id.removeprefix("sha256:")
dynamic={value for value in (app_name, app_id, app_id_hex,
                             app_id_hex[:12]) if value}
for network,data in sorted(value.items()):
    for alias in sorted(set((data or {}).get("Aliases") or [])):
        if not isinstance(alias,str) or not alias or alias in dynamic or not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9_.-]{0,254}",alias):
            raise SystemExit(1)
        print(network + "|" + alias)
' "$app_name" "$app_id" > "$run_dir/network-aliases.txt"
  capture_runtime_contract_hash "$app_id" > "$run_dir/runtime-contract.sha256"
  for metadata_file in "$run_dir"/*.json "$run_dir"/*.env "$run_dir"/*.txt; do
    [[ -f "$metadata_file" && ! -L "$metadata_file" ]] || continue
    chmod 600 -- "$metadata_file"
  done

  [[ "$(docker_rpc inspect --format '{{.HostConfig.Privileged}}' "$app_id")" == false ]] || fail 'privileged live application containers are not eligible'

  mapfile -t app_networks < "$run_dir/networks.txt"
  [[ "${#app_networks[@]}" -gt 0 ]] || fail 'live application has no Docker network'
  : > "$run_dir/network-identities.txt"
  while IFS= read -r network; do
    [[ "$network" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]{0,254}$ ]] || fail "live network name is invalid: $network"
    case "$network" in host|none) fail "live application uses an unsupported special network: $network" ;; esac
    network_id="$(docker_rpc network inspect --format '{{.Id}}' "$network")" || fail "cannot inspect network $network"
    valid_container_ref "$network_id" || fail "network ID is invalid: $network"
    printf '%s|%s\n' "$network" "$network_id" >> "$run_dir/network-identities.txt"
  done < "$run_dir/networks.txt"
  chmod 600 "$run_dir/network-identities.txt"
  mapfile -t app_network_ids < <(cut -d'|' -f2 "$run_dir/network-identities.txt")
  capture_ports_and_select_health_port
  capture_mounts
  validate_mount_recreation_contract
  capture_container_identity "$app_id" > "$run_dir/live.identity"
  printf '%s\n' "$app_id" > "$run_dir/live-app-id"
  printf '%s\n' "$app_name" > "$run_dir/live-app-name"
  printf '%s\n' "$app_image_id" > "$run_dir/live-app-image-id"
  chmod 600 "$run_dir/live.identity" "$run_dir/live-app-id" "$run_dir/live-app-name" "$run_dir/live-app-image-id"
}

resolve_dependencies() {
  local env_file="$run_dir/container.env" db_endpoint redis_endpoint db_match redis_match
  db_endpoint="$(env_value DATABASE_HOST "$env_file")"
  [[ -n "$db_endpoint" ]] || fail 'DATABASE_HOST is missing from live application environment'
  redis_endpoint="$(env_value REDIS_HOST "$env_file")"
  [[ -n "$redis_endpoint" ]] || fail 'REDIS_HOST is missing from live application environment'
  [[ "$db_endpoint" != *://* && "$redis_endpoint" != *://* ]] || fail 'external database/Redis URLs are not allowed'
  db_match="$(resolve_shared_container "$db_endpoint" database)"
  redis_match="$(resolve_shared_container "$redis_endpoint" redis)"
  database_id="${db_match%%|*}"
  redis_id="${redis_match%%|*}"
  valid_container_ref "$database_id" || fail 'PostgreSQL container ID is invalid'
  valid_container_ref "$redis_id" || fail 'Redis container ID is invalid'
  [[ "$database_id" != "$redis_id" && "$database_id" != "$app_id" && "$redis_id" != "$app_id" ]] || fail 'application, PostgreSQL and Redis must be three distinct containers'
  [[ "$(docker_rpc inspect --format '{{.State.Running}}' "$database_id")" == true ]] || fail 'PostgreSQL container is not running'
  [[ "$(docker_rpc inspect --format '{{.State.Running}}' "$redis_id")" == true ]] || fail 'Redis container is not running'
  docker_rpc exec "$database_id" sh -c 'command -v psql >/dev/null && command -v pg_dump >/dev/null && command -v pg_restore >/dev/null' >/dev/null || fail 'PostgreSQL container lacks psql/pg_dump/pg_restore'
  docker_rpc exec "$redis_id" sh -c 'command -v redis-cli >/dev/null && command -v redis-check-rdb >/dev/null' >/dev/null || fail 'Redis container lacks redis-cli/redis-check-rdb'
  printf '%s\n' "$database_id" > "$run_dir/database-id"
  printf '%s\n' "$redis_id" > "$run_dir/redis-id"
  capture_dependency_identity "$database_id" > "$run_dir/database.identity"
  capture_dependency_identity "$redis_id" > "$run_dir/redis.identity"
  chmod 600 "$run_dir/database-id" "$run_dir/redis-id" "$run_dir/database.identity" "$run_dir/redis.identity"
}

db_user_value() {
  local env_file="$run_dir/container.env" value
  value="$(env_value DATABASE_USER "$env_file")"
  [[ -n "$value" ]] || value="$(env_value POSTGRES_USER "$env_file")"
  [[ "$value" =~ ^[A-Za-z_][A-Za-z0-9_]{0,62}$ ]] || fail 'database user is not a simple identifier'
  printf '%s' "$value"
}

db_name_value() {
  local env_file="$run_dir/container.env" value
  value="$(env_value DATABASE_DBNAME "$env_file")"
  [[ -n "$value" ]] || value="$(env_value DATABASE_NAME "$env_file")"
  [[ -n "$value" ]] || value="$(env_value POSTGRES_DB "$env_file")"
  [[ "$value" =~ ^[A-Za-z_][A-Za-z0-9_]{0,62}$ ]] || fail 'database name is not a simple identifier'
  printf '%s' "$value"
}

db_password_value() {
  local value
  value="$(env_value DATABASE_PASSWORD "$run_dir/container.env")"
  [[ -n "$value" ]] || value="$(env_value POSTGRES_PASSWORD "$run_dir/container.env")"
  [[ -n "$value" ]] || fail 'database password is missing from the live application environment'
  printf '%s' "$value"
}

redis_password_value() {
  local value
  value="$(env_value REDIS_PASSWORD "$run_dir/container.env")"
  [[ -n "$value" ]] || value="$(env_value REDISCLI_AUTH "$run_dir/container.env")"
  printf '%s' "$value"
}

redis_username_value() {
  env_value REDIS_USERNAME "$run_dir/container.env"
}

db_psql() {
  local sql="$1" password
  password="$(db_password_value)"
  { printf '%s\n' "$password"; } |
    docker_rpc exec -i "$database_id" sh -c \
      'IFS= read -r password || exit 1; unset PGHOST PGHOSTADDR PGSERVICE PGSERVICEFILE PGPASSFILE PGDATABASE PGUSER; PGPASSWORD="$password" PGCONNECT_TIMEOUT=8 exec psql -X -At -v ON_ERROR_STOP=1 -U "$1" -d "$2" -c "$3"' \
      sh "$(db_user_value)" "$(db_name_value)" "$sql"
}

db_psql_file() {
  local sql_file="$1" password
  password="$(db_password_value)"
  { printf '%s\n' "$password"; cat -- "$sql_file"; } |
    docker_rpc exec -i "$database_id" sh -c \
      'IFS= read -r password || exit 1; unset PGHOST PGHOSTADDR PGSERVICE PGSERVICEFILE PGPASSFILE PGDATABASE PGUSER; PGPASSWORD="$password" PGCONNECT_TIMEOUT=8 exec psql -X -At -v ON_ERROR_STOP=1 -U "$1" -d "$2"' \
      sh "$(db_user_value)" "$(db_name_value)"
}

capture_settings_snapshot() {
  local output_file="$1" partial key_list sql
  key_list="'${rollout_keys[0]}'"
  local key
  for key in "${rollout_keys[@]:1}"; do key_list+=" ,'$key'"; done
  for key in "${rollout_content_keys[@]}"; do key_list+=" ,'$key'"; done
  key_list+=" ,'$invitation_config_key'"
  # PostgreSQL wraps base64 output at 76 columns.  Remove those embedded line
  # breaks so every setting remains one canonical TSV record even for JSON or
  # customer-support content larger than 57 bytes.
  sql="SELECT key || E'\\t' || translate(encode(convert_to(value, 'UTF8'), 'base64'), E'\\n\\r', '') FROM settings WHERE key IN ($key_list) ORDER BY key;"
  partial="$(mktemp "$run_dir/.settings-snapshot.XXXXXX")" || fail 'cannot create rollout setting snapshot temporary file'
  if ! db_psql "$sql" > "$partial"; then
    rm -f -- "$partial"
    fail 'cannot capture rollout setting snapshot'
  fi
  chmod 600 -- "$partial"
  validate_settings_snapshot "$partial"
  mv -f -- "$partial" "$output_file"
  chmod 600 -- "$output_file"
}

validate_settings_snapshot() {
  local file="$1" line key b64 value
  local -A seen=()
  while IFS=$'\t' read -r key b64; do
    [[ -n "$key" && "$b64" =~ ^[A-Za-z0-9+/]*={0,2}$ ]] || fail 'rollout setting snapshot is malformed'
    [[ -z "${seen[$key]+x}" ]] || fail 'rollout setting snapshot contains duplicate keys'
    case " ${rollout_keys[*]} ${rollout_content_keys[*]} $invitation_config_key " in *" $key "*) ;; *) fail "unexpected rollout setting key: $key" ;; esac
    seen[$key]=1
    value="$(printf '%s' "$b64" | base64 -d 2>/dev/null)" || fail "cannot decode rollout setting: $key"
    if [[ "$key" == "$invitation_config_key" ]]; then
      printf '%s' "$value" | python3 -c 'import json,sys; value=json.load(sys.stdin); raise SystemExit(0 if isinstance(value,dict) else 1)' || fail 'invitation rollout setting is not a JSON object'
    elif [[ "$key" == subnexus_customer_support_content || "$key" == customer_support_content ]]; then
      # Content is restored byte-for-byte but never interpolated as SQL; it is
      # held in the base64 expression generated by restore_rollout_gates.
      [[ "${#value}" -le 1048576 ]] || fail "customer support content is unexpectedly large: $key"
    elif [[ "$key" == channel_monitor_mode ]]; then
      [[ "$value" == v1 || "$value" == v2 || "$value" == v3 ]] || fail 'channel monitor mode snapshot is invalid'
    else
      [[ "$value" == true || "$value" == false ]] || fail "rollout setting is not a boolean: $key"
    fi
  done < "$file"
}

validate_closed_settings_snapshot() {
  local file="$1" line key b64 extra expected
  local -A seen=()
  validate_settings_snapshot "$file"
  while IFS=$'\t' read -r key b64 extra; do
    [[ -n "$key" ]] || continue
    [[ -z "${extra:-}" ]] || fail 'closed rollout setting snapshot has extra fields'
    seen[$key]=1
    case "$key" in
      channel_monitor_mode)
        expected="$(base64_text 'v1')" ;;
      subnexus_invite_activities_config)
        expected="$(base64_text '{"enabled":false,"invite_lottery_enabled":false,"recharge_wheel_enabled":false,"invite_milestone_enabled":false}')" ;;
      registration_ip_cooldown_enabled|subnexus_activity_center_enabled|subnexus_checkin_enabled|\
      subnexus_leaderboard_enabled|subnexus_marquee_enabled|subnexus_invite_activities_enabled|\
      subnexus_invite_rewards_enabled|subnexus_first_recharge_enabled|battle_pass_enabled|\
      subnexus_student_recharge_benefit_enabled|subnexus_invoice_enabled|channel_monitor_enabled|\
      subnexus_customer_support_enabled|customer_support_enabled)
        expected="$(base64_text 'false')" ;;
      subnexus_customer_support_content|customer_support_content)
        continue ;;
      *)
        fail "unexpected key in closed rollout setting snapshot: $key" ;;
    esac
    [[ "$b64" == "$expected" ]] || fail "rollout setting is not closed: $key"
  done < "$file"
  local required
  for required in "${rollout_keys[@]}" "$invitation_config_key"; do
    [[ -n "${seen[$required]+x}" ]] || fail "closed rollout setting snapshot is missing: $required"
  done
}

write_closed_settings_snapshot() {
  local source="$run_dir/settings-before.tsv" temporary key b64
  local -a sorted_keys=()
  local -A values=()
  validate_settings_snapshot "$source"
  while IFS=$'\t' read -r key b64; do
    [[ -n "$key" ]] || continue
    values[$key]="$b64"
  done < "$source"
  for key in "${rollout_keys[@]}"; do
    if [[ "$key" == channel_monitor_mode ]]; then
      values[$key]="$(base64_text 'v1')"
    else
      values[$key]="$(base64_text 'false')"
    fi
  done
  values[$invitation_config_key]="$(base64_text '{"enabled":false,"invite_lottery_enabled":false,"recharge_wheel_enabled":false,"invite_milestone_enabled":false}')"
  temporary="$(mktemp "$run_dir/.settings-closed.XXXXXX")" || fail 'cannot create closed rollout setting snapshot temporary file'
  mapfile -t sorted_keys < <(printf '%s\n' "${!values[@]}" | LC_ALL=C sort)
  for key in "${sorted_keys[@]}"; do
    printf '%s\t%s\n' "$key" "${values[$key]}" >> "$temporary"
  done
  chmod 600 -- "$temporary"
  validate_closed_settings_snapshot "$temporary"
  mv -f -- "$temporary" "$run_dir/settings-closed.tsv"
  chmod 600 -- "$run_dir/settings-closed.tsv"
  hash_file "$run_dir/settings-closed.tsv" > "$run_dir/settings-closed.tsv.sha256"
  chmod 600 -- "$run_dir/settings-closed.tsv.sha256"
}

assert_settings_snapshot_integrity() {
  local expected_file="$1" sidecar manifest_key label expected manifest_expected actual
  case "$expected_file" in
    "$run_dir/settings-before.tsv")
      sidecar="$run_dir/settings-before.tsv.sha256"
      manifest_key='settings_snapshot_sha256'
      label='rollout setting snapshot'
      ;;
    "$run_dir/settings-closed.tsv")
      sidecar="$run_dir/settings-closed.tsv.sha256"
      manifest_key='settings_closed_snapshot_sha256'
      label='closed rollout setting snapshot'
      ;;
    *) fail 'settings snapshot path is not one of the immutable run snapshots' ;;
  esac
  assert_root_owned_regular "$expected_file" "$label"
  assert_root_owned_regular "$sidecar" "$label hash"
  expected="$(read_one_line "$sidecar")"
  valid_sha64 "$expected" || fail "$label sidecar hash is invalid"
  manifest_expected="$(manifest_value "$manifest_key")"
  valid_sha64 "$manifest_expected" || fail "$label manifest hash is invalid"
  actual="$(hash_file "$expected_file")" || fail "cannot hash $label"
  [[ "$actual" == "$expected" && "$actual" == "$manifest_expected" ]] || fail "$label hash mismatch"
  if [[ "$manifest_key" == settings_closed_snapshot_sha256 ]]; then
    validate_closed_settings_snapshot "$expected_file"
  else
    validate_settings_snapshot "$expected_file"
  fi
}

assert_rollout_settings_integrity() {
  assert_settings_snapshot_integrity "$run_dir/settings-before.tsv"
  assert_settings_snapshot_integrity "$run_dir/settings-closed.tsv"
}

assert_settings_snapshot_matches_file() {
  local expected_file="$1" temporary expected actual manifest_key
  assert_settings_snapshot_integrity "$expected_file"
  case "$expected_file" in
    "$run_dir/settings-before.tsv") manifest_key='settings_snapshot_sha256' ;;
    "$run_dir/settings-closed.tsv") manifest_key='settings_closed_snapshot_sha256' ;;
    *) fail 'settings comparison path is not an immutable run snapshot' ;;
  esac
  temporary="$(mktemp "$run_dir/.settings-compare.XXXXXX")" || fail 'cannot create rollout setting comparison temporary file'
  if ! capture_settings_snapshot "$temporary"; then
    rm -f -- "$temporary"
    fail 'cannot capture current rollout settings for comparison'
  fi
  expected="$(manifest_value "$manifest_key")"
  actual="$(hash_file "$temporary")" || { rm -f -- "$temporary"; fail 'cannot hash current rollout setting snapshot'; }
  rm -f -- "$temporary"
  [[ "$actual" == "$expected" ]] || fail 'current rollout settings differ from the expected snapshot'
}

settings_snapshot_matches_file() {
  local expected_file="$1"
  # Run the strict comparator in a subshell so its fail-closed diagnostics do
  # not invoke automatic Docker rollback while this helper is probing which
  # side of the gate-closure boundary was reached.
  ( assert_settings_snapshot_matches_file "$expected_file" ) >/dev/null 2>&1
}

close_rollout_gates() {
  local before_snapshot="$run_dir/settings-before.tsv" closed_snapshot="$run_dir/settings-closed.tsv"
  local sql_file key b64 before_hash closed_hash actual
  local -A before_present=() closed_present=()
  assert_rollout_settings_integrity
  before_hash="$(manifest_value settings_snapshot_sha256)"
  closed_hash="$(manifest_value settings_closed_snapshot_sha256)"
  sql_file="$(mktemp "$run_dir/.close-settings.XXXXXX")" || fail 'cannot create rollout gate closure temporary file'
  chmod 600 -- "$sql_file"
  assert_root_owned_regular "$sql_file" 'rollout gate closure temporary file'
  {
    printf 'BEGIN ISOLATION LEVEL SERIALIZABLE;\n'
    printf "SET LOCAL lock_timeout = '5s';\nSET LOCAL statement_timeout = '30s';\n"
    # Serialize the prepare-snapshot comparison and gate closure.  This closes
    # the race between a pre-switch read check and the actual settings update.
    printf 'LOCK TABLE settings IN SHARE ROW EXCLUSIVE MODE;\n'
  } >> "$sql_file"
  while IFS=$'\t' read -r key b64; do
    [[ -n "$key" ]] || continue
    before_present[$key]=1
    printf "DO \$\$ DECLARE actual_value text; BEGIN SELECT value INTO actual_value FROM settings WHERE key='%s' FOR UPDATE; IF NOT FOUND OR actual_value IS DISTINCT FROM convert_from(decode('%s','base64'),'UTF8') THEN RAISE EXCEPTION 'rollout setting changed after prepare: %s'; END IF; END \$\$;\n" "$key" "$b64" "$key" >> "$sql_file"
  done < "$before_snapshot"
  for key in "${rollout_keys[@]}" "${rollout_content_keys[@]}" "$invitation_config_key"; do
    if [[ -z "${before_present[$key]+x}" ]]; then
      printf "DO \$\$ BEGIN IF EXISTS (SELECT 1 FROM settings WHERE key='%s') THEN RAISE EXCEPTION 'rollout setting appeared after prepare: %s'; END IF; END \$\$;\n" "$key" "$key" >> "$sql_file"
    fi
  done
  while IFS=$'\t' read -r key b64; do
    [[ -n "$key" ]] || continue
    closed_present[$key]=1
    printf "INSERT INTO settings (key,value) VALUES ('%s',convert_from(decode('%s','base64'),'UTF8')) ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value,updated_at=NOW();\n" "$key" "$b64" >> "$sql_file"
  done < "$closed_snapshot"
  for key in "${rollout_keys[@]}" "${rollout_content_keys[@]}" "$invitation_config_key"; do
    if [[ -z "${closed_present[$key]+x}" ]]; then
      printf "DELETE FROM settings WHERE key='%s';\n" "$key" >> "$sql_file"
    fi
  done
  printf 'COMMIT;\n' >> "$sql_file"
  actual="$(hash_file "$before_snapshot")" || { rm -f -- "$sql_file"; fail 'cannot rehash rollout setting snapshot before closure'; }
  [[ "$actual" == "$before_hash" ]] || { rm -f -- "$sql_file"; fail 'rollout setting snapshot changed while preparing closure SQL'; }
  actual="$(hash_file "$closed_snapshot")" || { rm -f -- "$sql_file"; fail 'cannot rehash closed rollout setting snapshot before closure'; }
  [[ "$actual" == "$closed_hash" ]] || { rm -f -- "$sql_file"; fail 'closed rollout setting snapshot changed while preparing closure SQL'; }
  if ! db_psql_file "$sql_file" >/dev/null; then
    rm -f -- "$sql_file"
    fail 'could not close SubNexus rollout gates from the prepared settings snapshot'
  fi
  rm -f -- "$sql_file"
}

verify_rollout_gates_closed() {
  local key_list="'${rollout_keys[0]}'" key count mode config
  for key in "${rollout_keys[@]:1}"; do key_list+=" ,'$key'"; done
  # rollout_keys contains fourteen boolean gates plus channel_monitor_mode;
  # the mode is checked separately and must not be counted as a boolean.
  count="$(db_psql "SELECT COUNT(*) FROM settings WHERE key IN ($key_list) AND lower(trim(value)) = 'false';" | tr -d '[:space:]')" || fail 'cannot verify closed rollout gates'
  [[ "$count" == 14 ]] || fail "expected 14 closed boolean rollout gates, got $count"
  mode="$(db_psql "SELECT value FROM settings WHERE key='channel_monitor_mode';" | tr -d '[:space:]')" || fail 'cannot verify channel monitor mode'
  [[ "$mode" == v1 ]] || fail 'channel monitor mode is not v1'
  config="$(db_psql "SELECT CASE WHEN (value::jsonb->>'enabled')='false' AND (value::jsonb->>'invite_lottery_enabled')='false' AND (value::jsonb->>'recharge_wheel_enabled')='false' AND (value::jsonb->>'invite_milestone_enabled')='false' THEN 'closed' ELSE 'open' END FROM settings WHERE key='$invitation_config_key';" | tr -d '[:space:]')" || fail 'cannot verify invitation activity config'
  [[ "$config" == closed ]] || fail 'invitation activity config is not closed'
}

restore_rollout_gates() {
  local snapshot="$run_dir/settings-before.tsv" closed_snapshot="$run_dir/settings-closed.tsv" sql_file line key b64 closed_hash before_hash actual_hash
  local -A closed_present=() before_present=()
  assert_rollout_settings_integrity
  closed_hash="$(manifest_value settings_closed_snapshot_sha256)"
  before_hash="$(manifest_value settings_snapshot_sha256)"
  sql_file="$(mktemp "$run_dir/.restore-settings.XXXXXX")" || fail 'cannot create rollout settings restore temporary file'
  chmod 600 -- "$sql_file"
  assert_root_owned_regular "$sql_file" 'rollout settings restore temporary file'
  {
    printf 'BEGIN ISOLATION LEVEL SERIALIZABLE;\n'
    printf "SET LOCAL lock_timeout = '5s';\nSET LOCAL statement_timeout = '30s';\n"
    # Serialize the compare-and-restore operation with application writes.  A
    # failed lock or any mismatch aborts before the historical snapshot is
    # written, leaving the operator to resolve the concurrent change.
    printf 'LOCK TABLE settings IN SHARE ROW EXCLUSIVE MODE;\n'
  } >> "$sql_file"
  while IFS=$'\t' read -r key b64; do
    [[ -n "$key" ]] || continue
    closed_present[$key]=1
    # b64 is validated to contain no SQL metacharacters; the database performs
    # the UTF-8 decode, so arbitrary historical JSON/URLs are not interpolated.
    printf "DO \$\$ DECLARE actual_value text; BEGIN SELECT value INTO actual_value FROM settings WHERE key='%s' FOR UPDATE; IF NOT FOUND OR actual_value IS DISTINCT FROM convert_from(decode('%s','base64'),'UTF8') THEN RAISE EXCEPTION 'rollout setting CAS mismatch: %s'; END IF; END \$\$;\n" "$key" "$b64" "$key" >> "$sql_file"
  done < "$closed_snapshot"
  for key in "${rollout_keys[@]}" "${rollout_content_keys[@]}" "$invitation_config_key"; do
    if [[ -z "${closed_present[$key]+x}" ]]; then
      printf "DO \$\$ BEGIN IF EXISTS (SELECT 1 FROM settings WHERE key='%s') THEN RAISE EXCEPTION 'unexpected rollout setting appeared: %s'; END IF; END \$\$;\n" "$key" "$key" >> "$sql_file"
    fi
  done
  while IFS=$'\t' read -r key b64; do
    [[ -n "$key" ]] || continue
    before_present[$key]=1
    printf "INSERT INTO settings (key,value) VALUES ('%s',convert_from(decode('%s','base64'),'UTF8')) ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value,updated_at=NOW();\n" "$key" "$b64" >> "$sql_file"
  done < "$snapshot"
  for key in "${rollout_keys[@]}" "${rollout_content_keys[@]}" "$invitation_config_key"; do
    if [[ -z "${before_present[$key]+x}" ]]; then printf "DELETE FROM settings WHERE key='%s';\n" "$key" >> "$sql_file"; fi
  done
  printf 'COMMIT;\n' >> "$sql_file"
  actual_hash="$(hash_file "$closed_snapshot")" || { rm -f -- "$sql_file"; fail 'cannot rehash closed rollout setting snapshot before restore'; }
  [[ "$actual_hash" == "$closed_hash" ]] || { rm -f -- "$sql_file"; fail 'closed rollout setting snapshot changed while preparing restore SQL'; }
  actual_hash="$(hash_file "$snapshot")" || { rm -f -- "$sql_file"; fail 'cannot rehash rollout setting snapshot before restore'; }
  [[ "$actual_hash" == "$before_hash" ]] || { rm -f -- "$sql_file"; fail 'rollout setting snapshot changed while preparing restore SQL'; }
  if ! db_psql_file "$sql_file" >/dev/null; then
    rm -f -- "$sql_file"
    fail 'could not restore rollout settings'
  fi
  rm -f -- "$sql_file"
}

verify_rollout_gates_restored() {
  local verification_file="$run_dir/.settings-after-rollback" expected actual
  assert_rollout_settings_integrity
  [[ ! -e "$verification_file" && ! -L "$verification_file" ]] || fail 'rollback settings verification path already exists'
  capture_settings_snapshot "$verification_file"
  expected="$(manifest_value settings_snapshot_sha256)"
  actual="$(hash_file "$verification_file")" || fail 'cannot hash restored rollout setting snapshot'
  rm -f -- "$verification_file"
  [[ "$actual" == "$expected" ]] || fail 'restored rollout settings differ from the prepared snapshot'
}

validate_gate_evidence() {
  local gate_info actual_sha reported_size expanded_size
  gate_info="$(assert_approved_path "$candidate_gate_evidence" candidate_gate)"
  candidate_gate_evidence="${gate_info%%|*}"
  assert_root_owned_regular "$candidate_gate_evidence" 'candidate gate evidence'
  actual_sha="$(hash_file "$candidate_gate_evidence")" || fail 'cannot hash candidate gate evidence'
  if [[ -n "${candidate_gate_evidence_sha256:-}" ]]; then
    valid_sha64 "$candidate_gate_evidence_sha256" || fail 'candidate gate evidence SHA is invalid'
    [[ "$actual_sha" == "$candidate_gate_evidence_sha256" ]] || fail 'candidate gate evidence changed after prepare'
  fi
  candidate_gate_evidence_sha256="$actual_sha"
  grep -Fqx 'result=passed' "$candidate_gate_evidence" || fail 'candidate gate evidence is not passed'
  grep -Fqx "approved_commit=$target_sha" "$candidate_gate_evidence" || fail 'candidate gate evidence commit does not match target'
  grep -Fqx "candidate_archive_sha256=$candidate_archive_sha" "$candidate_gate_evidence" || fail 'candidate gate evidence archive SHA does not match'
  grep -Fqx "candidate_image_expected_id=$expected_image_id" "$candidate_gate_evidence" || fail 'candidate gate evidence image ID does not match'
  reported_size="$(awk -F= '$1 == "candidate_archive_size" { count++; value=substr($0, index($0, "=") + 1) } END { if (count != 1) exit 1; print value }' "$candidate_gate_evidence")" || fail 'candidate gate evidence must contain exactly one archive size'
  expanded_size="$(awk -F= '$1 == "candidate_archive_expanded_size" { count++; value=substr($0, index($0, "=") + 1) } END { if (count != 1) exit 1; print value }' "$candidate_gate_evidence")" || fail 'candidate gate evidence must contain exactly one expanded archive size'
  [[ "$reported_size" =~ ^[0-9]+$ && "$reported_size" -gt 0 && "$reported_size" -le 12884901888 ]] || fail 'candidate gate archive size is invalid'
  [[ "$expanded_size" =~ ^[0-9]+$ && "$expanded_size" -gt 0 && "$expanded_size" -le 12884901888 ]] || fail 'candidate gate expanded archive size is invalid'
  candidate_archive_reported_size="$reported_size"
  candidate_archive_expanded_size="$expanded_size"
  grep -Fqx 'manual_review=required' "$candidate_gate_evidence" || fail 'candidate gate evidence lacks manual review marker'
  grep -Fqx 'cutover_authorized=false' "$candidate_gate_evidence" || fail 'candidate gate evidence must not self-authorize production cutover'
}

validate_source_tree() {
  local source_info head symbolic status tree submodules source_base
  source_info="$(assert_approved_path "$source_root" source)"
  source_root="${source_info%%|*}"
  source_base="${source_info#*|}"
  [[ "$source_base" == /srv/subnexus-migration || "$source_base" == /root/subnexus-migration ||
     "$source_base" == "$production_source_root" || "$source_base" == "$alternate_production_source_root" ]] ||
    fail 'release source root is outside the approved source roots'
  assert_root_owned_dir "$source_root" 'release source root'
  [[ ( -d "$source_root/.git" || -f "$source_root/.git" ) && ! -L "$source_root/.git" ]] || fail 'release source root is not a regular Git worktree'
  head="$(git -C "$source_root" rev-parse HEAD)" || fail 'cannot read release HEAD'
  [[ "$head" == "$target_sha" ]] || fail 'release source HEAD does not equal approved target SHA'
  symbolic="$(git -C "$source_root" symbolic-ref --quiet --short HEAD 2>/dev/null || true)"
  [[ -z "$symbolic" ]] || fail 'release source must be detached at the approved commit'
  status="$(git -C "$source_root" status --porcelain=v1 --untracked-files=all)" || fail 'cannot inspect release worktree status'
  [[ -z "$status" ]] || fail 'release source worktree is not clean'
  submodules="$(git -C "$source_root" submodule status --recursive)" || fail 'cannot inspect release submodule state'
  [[ -z "$submodules" ]] || fail 'release source must not contain submodules'
  tree="$(git -C "$source_root" rev-parse "$target_sha^{tree}")" || fail 'cannot resolve release tree'
  printf '%s\n' "$tree" > "$run_dir/source-tree"
}

validate_archive() {
  local archive_info archive_path_root actual_sha size free_bytes required archive_budget docker_free_bytes
  archive_info="$(assert_approved_path "$candidate_archive" candidate_archive)"
  candidate_archive="${archive_info%%|*}"
  archive_path_root="${archive_info#*|}"
  assert_root_owned_regular "$candidate_archive" 'candidate image archive'
  actual_sha="$(hash_file "$candidate_archive")" || fail 'cannot hash candidate archive'
  [[ "$actual_sha" == "$candidate_archive_sha" ]] || fail "candidate archive SHA mismatch: actual=$actual_sha expected=$candidate_archive_sha"
  size="$(stat -c '%s' -- "$candidate_archive")" || fail 'cannot inspect candidate archive size'
  [[ "$size" =~ ^[0-9]+$ && "$size" -gt 0 ]] || fail 'candidate archive is empty'
  [[ "$candidate_archive_reported_size" == "$size" ]] || fail 'candidate archive size differs from candidate gate evidence'
  [[ "$candidate_archive_expanded_size" =~ ^[0-9]+$ ]] || fail 'candidate expanded archive size is unavailable'
  free_bytes="$(df -P -B1 "$archive_path_root" | awk 'NR==2 {print $4}')" || fail 'cannot inspect artifact filesystem free space'
  [[ "$free_bytes" =~ ^[0-9]+$ ]] || fail 'artifact filesystem free space is invalid'
  required=$((size + 1073741824))
  (( free_bytes >= required )) || fail 'insufficient free space on the candidate artifact filesystem'
  archive_budget="$size"
  (( candidate_archive_expanded_size > archive_budget )) && archive_budget="$candidate_archive_expanded_size"
  required=$((archive_budget * 2 + 4294967296))
  (( required < 8589934592 )) && required=8589934592
  docker_free_bytes="$(df -P -B1 "$docker_root_dir" | awk 'NR==2 {print $4}')" || fail 'cannot inspect Docker storage free space'
  [[ "$docker_free_bytes" =~ ^[0-9]+$ ]] || fail 'Docker storage free space is invalid'
  (( docker_free_bytes >= required )) || fail "insufficient Docker storage for candidate image load ($docker_free_bytes < $required bytes)"
}

ensure_image_load_log() {
  local path="$run_dir/image-load.log" temporary
  [[ ! -e "$path" && ! -L "$path" ]] || fail 'candidate image load log path already exists'
  temporary="$(mktemp "$run_dir/.image-load.log.XXXXXX")" || fail 'cannot create candidate image load log temporary file'
  assert_root_owned_regular "$temporary" 'candidate image load log temporary file'
  chmod 600 -- "$temporary" || fail 'cannot protect candidate image load log temporary file'
  # Recheck the destination immediately before the rename.  GNU mv -T forces
  # the destination to be treated as one path; it replaces a raced symlink
  # instead of following a symlink-to-directory into an external tree.  The
  # run directory is held under the cutover evidence lock for the prepare
  # operation.
  [[ ! -e "$path" && ! -L "$path" ]] || {
    rm -f -- "$temporary"
    fail 'candidate image load log path appeared during creation'
  }
  mv -T -- "$temporary" "$path" || {
    rm -f -- "$temporary"
    fail 'cannot install candidate image load log'
  }
  assert_root_owned_regular "$path" 'candidate image load log'
}

load_and_validate_candidate_image() {
  local tag="$candidate_tag_prefix$target_sha" existing_id loaded_id labels image_env inspect_error inspect_status actual_archive_sha
  assert_root_owned_regular "$candidate_archive" 'candidate image archive'
  actual_archive_sha="$(hash_file "$candidate_archive")" || fail 'cannot hash candidate image archive before load'
  [[ "$actual_archive_sha" == "$candidate_archive_sha" ]] || fail 'candidate image archive changed before load'
  ensure_image_load_log
  inspect_error="$run_dir/candidate-image-inspect.err"
  if existing_id="$(docker_rpc image inspect --format '{{.Id}}' "$tag" 2>"$inspect_error")"; then
    rm -f -- "$inspect_error"
  else
    inspect_status=$?
    if grep -Eiq 'no such image:|no such object:' "$inspect_error"; then
      existing_id=''
      rm -f -- "$inspect_error"
    else
      cat -- "$inspect_error" >&2 || true
      rm -f -- "$inspect_error"
      fail "cannot inspect existing candidate image tag (status $inspect_status)"
    fi
  fi
  if [[ -n "$existing_id" ]]; then
    existing_id="${existing_id#sha256:}"
    [[ "$existing_id" == "$expected_image_id" ]] || fail 'an existing release tag points to a different image ID'
  else
    log 'Loading the approved candidate archive; the live containers remain untouched.'
    docker_rpc image load --input "$candidate_archive" > "$run_dir/image-load.log" 2>&1 || fail 'candidate image archive load failed'
  fi
  loaded_id="$(docker_rpc image inspect --format '{{.Id}}' "$tag")" || fail 'cannot inspect loaded candidate image tag'
  loaded_id="${loaded_id#sha256:}"
  [[ "$loaded_id" == "$expected_image_id" ]] || fail "loaded candidate image ID mismatch: $loaded_id"
  labels="$(docker_rpc image inspect --format '{{index .Config.Labels "com.subnexus.release.gate"}}|{{index .Config.Labels "com.subnexus.candidate.commit"}}|{{index .Config.Labels "org.opencontainers.image.revision"}}' "$tag")" || fail 'cannot inspect candidate release labels'
  [[ "$labels" == "subnexus-isolated-build-v1|$target_sha|$target_sha" ]] || fail 'candidate image release labels do not match the approved commit'
  [[ "$(docker_rpc image inspect --format '{{.Os}}|{{.Architecture}}' "$tag")" == 'linux|amd64' ]] || fail 'candidate image must be linux/amd64'
  image_env="$(docker_rpc image inspect --format '{{range .Config.Env}}{{println .}}{{end}}' "$tag")" || fail 'cannot inspect candidate image environment'
  for secret_name in DATABASE_PASSWORD REDIS_PASSWORD JWT_SECRET TOTP_ENCRYPTION_KEY ADMIN_PASSWORD; do
    ! printf '%s\n' "$image_env" | grep -Eq "^${secret_name}=" || fail "candidate image embeds forbidden secret environment: $secret_name"
  done
  printf '%s\n' "$loaded_id" > "$run_dir/candidate-image-id"
  chmod 600 "$run_dir/candidate-image-id" "$run_dir/image-load.log"
}

disk_free_bytes() {
  local path="$1" value
  value="$(df -P -B1 -- "$path" | awk 'NR == 2 { print $4 }')" || fail "cannot inspect free disk space for $path"
  [[ "$value" =~ ^[0-9]+$ ]] || fail "free disk space is invalid for $path"
  printf '%s' "$value"
}

redis_used_memory_bytes() {
  local password username output value
  password="$(redis_password_value)"
  username="$(redis_username_value)"
  output="$({ printf '%s\n' "$password"; } |
    docker_rpc exec -i "$redis_id" sh -c \
      'IFS= read -r password || exit 1; unset REDISCLI_AUTH; if [ -n "$password" ]; then REDISCLI_AUTH="$password"; export REDISCLI_AUTH; fi; if [ -n "$1" ]; then exec redis-cli --no-auth-warning --user "$1" INFO memory; else exec redis-cli --no-auth-warning INFO memory; fi' \
      sh "$username")" || fail 'cannot inspect Redis memory use for backup budgeting'
  value="$(printf '%s\n' "$output" | tr -d '\r' | awk -F: '$1 == "used_memory" { count++; value=$2 } END { if (count != 1) exit 1; print value }')" || fail 'Redis used_memory response is missing or ambiguous'
  [[ "$value" =~ ^[0-9]+$ ]] || fail 'Redis used_memory is invalid'
  printf '%s' "$value"
}

initialize_prepare_backup_budgets() {
  local app_source="$1" redis_used app_source_bytes calculated budget_file checks_file
  redis_used="$(redis_used_memory_bytes)"
  app_source_bytes="$(du -sb -- "$app_source" | awk 'NR == 1 { print $1 }')" || fail 'cannot size application data for backup budgeting'
  [[ "$app_source_bytes" =~ ^[0-9]+$ ]] || fail 'application data size is invalid'

  postgresql_dump_budget_bytes="$postgresql_dump_budget_limit"
  calculated=$((redis_used * 2 + 67108864))
  (( calculated < 536870912 )) && calculated=536870912
  (( calculated <= redis_rdb_budget_limit )) || fail 'Redis data exceeds the bounded online backup budget'
  redis_rdb_budget_bytes="$calculated"
  calculated=$((app_source_bytes * 2 + 67108864))
  (( calculated < 536870912 )) && calculated=536870912
  (( calculated <= app_data_archive_budget_limit )) || fail 'application data exceeds the bounded online backup budget'
  app_data_archive_budget_bytes="$calculated"

  budget_file="$run_dir/backup-budgets.env"
  [[ ! -e "$budget_file" && ! -L "$budget_file" ]] || fail 'backup budget evidence path already exists'
  {
    printf 'postgresql_dump_budget_bytes=%s\n' "$postgresql_dump_budget_bytes"
    printf 'redis_used_memory_bytes=%s\n' "$redis_used"
    printf 'redis_rdb_budget_bytes=%s\n' "$redis_rdb_budget_bytes"
    printf 'application_data_source_bytes=%s\n' "$app_source_bytes"
    printf 'application_data_archive_budget_bytes=%s\n' "$app_data_archive_budget_bytes"
    printf 'candidate_archive_expanded_size=%s\n' "$candidate_archive_expanded_size"
    printf 'final_free_reserve_bytes=%s\n' "$final_free_reserve_bytes"
  } > "$budget_file" || fail 'cannot write backup budget evidence'
  chmod 600 -- "$budget_file"
  checks_file="$run_dir/disk-budget-checks.txt"
  [[ ! -e "$checks_file" && ! -L "$checks_file" ]] || fail 'disk budget evidence path already exists'
  : > "$checks_file" || fail 'cannot initialize disk budget evidence'
  chmod 600 -- "$checks_file"
}

assert_prepare_disk_budget() {
  local phase="$1" run_budget=0 docker_budget=0 image_budget archive_budget device free required path
  local -A required_by_device=() path_by_device=()
  [[ "$postgresql_dump_budget_bytes" =~ ^[0-9]+$ && "$redis_rdb_budget_bytes" =~ ^[0-9]+$ && "$app_data_archive_budget_bytes" =~ ^[0-9]+$ ]] || fail 'prepare backup budgets are not initialized'
  assert_root_owned_regular "$run_dir/disk-budget-checks.txt" 'disk budget evidence'
  archive_budget="$candidate_archive_reported_size"
  (( candidate_archive_expanded_size > archive_budget )) && archive_budget="$candidate_archive_expanded_size"
  image_budget=$((archive_budget * 2))
  case "$phase" in
    before_postgresql)
      run_budget=$((postgresql_dump_budget_bytes + redis_rdb_budget_bytes + app_data_archive_budget_bytes + prepare_metadata_budget_bytes))
      docker_budget=$((image_budget + redis_rdb_budget_bytes))
      ;;
    before_redis)
      run_budget=$((redis_rdb_budget_bytes + app_data_archive_budget_bytes + prepare_metadata_budget_bytes))
      docker_budget=$((image_budget + redis_rdb_budget_bytes))
      ;;
    before_application_data)
      run_budget=$((app_data_archive_budget_bytes + prepare_metadata_budget_bytes))
      docker_budget="$image_budget"
      ;;
    before_image_load)
      run_budget="$prepare_metadata_budget_bytes"
      docker_budget="$image_budget"
      ;;
    after_image_load)
      run_budget="$prepare_metadata_budget_bytes"
      docker_budget=0
      ;;
    *) fail "unknown prepare disk-budget phase: $phase" ;;
  esac

  device="$(stat -c '%d' -- "$run_dir")" || fail 'cannot identify cutover evidence filesystem'
  [[ "$device" =~ ^[0-9]+$ ]] || fail 'cutover evidence filesystem identity is invalid'
  required_by_device[$device]="$run_budget"
  path_by_device[$device]="$run_dir"
  device="$(stat -c '%d' -- "$docker_root_dir")" || fail 'cannot identify Docker storage filesystem'
  [[ "$device" =~ ^[0-9]+$ ]] || fail 'Docker storage filesystem identity is invalid'
  required_by_device[$device]="$(( ${required_by_device[$device]:-0} + docker_budget ))"
  path_by_device[$device]="$docker_root_dir"

  for device in "${!required_by_device[@]}"; do
    path="${path_by_device[$device]}"
    free="$(disk_free_bytes "$path")"
    required=$((required_by_device[$device] + final_free_reserve_bytes))
    (( free >= required )) || fail "insufficient disk for safe prepare phase $phase on device $device ($free < $required bytes)"
    printf '%s|%s|%s|%s|%s\n' "$phase" "$device" "$path" "$free" "$required" >> "$run_dir/disk-budget-checks.txt"
  done
  chmod 600 -- "$run_dir/disk-budget-checks.txt"
}

assert_backup_within_budget() {
  local path="$1" budget="$2" label="$3" size
  size="$(stat -c '%s' -- "$path")" || fail "cannot inspect $label size"
  [[ "$size" =~ ^[0-9]+$ && "$size" -gt 0 ]] || fail "$label is empty or has an invalid size"
  (( size <= budget )) || fail "$label exceeded its approved disk budget"
}

backup_postgresql() {
  local dump="$run_dir/postgresql.dump" partial list="$run_dir/postgresql.list" list_partial password file_limit_kib
  partial="$(mktemp "$run_dir/.postgresql.dump.partial.XXXXXX")" || fail 'cannot create PostgreSQL backup temporary file'
  list_partial="$(mktemp "$run_dir/.postgresql.list.partial.XXXXXX")" || { rm -f -- "$partial"; fail 'cannot create PostgreSQL catalog temporary file'; }
  password="$(db_password_value)"
  [[ "$postgresql_dump_budget_bytes" =~ ^[0-9]+$ ]] || fail 'PostgreSQL backup budget is not initialized'
  file_limit_kib=$((postgresql_dump_budget_bytes / 1024))
  db_psql 'SELECT 1;' | tr -d '[:space:]' | grep -Fxq 1 || fail 'PostgreSQL read check failed before backup'
  if ! (
    ulimit -f "$file_limit_kib" || exit 1
    { printf '%s\n' "$password"; } |
      docker_rpc exec -i "$database_id" sh -c \
        'IFS= read -r password || exit 1; unset PGHOST PGHOSTADDR PGSERVICE PGSERVICEFILE PGPASSFILE PGDATABASE PGUSER; PGPASSWORD="$password" PGCONNECT_TIMEOUT=8 exec pg_dump -Fc -U "$1" -d "$2"' \
        sh "$(db_user_value)" "$(db_name_value)"
  ) > "$partial"; then
    rm -f -- "$partial"
    fail 'PostgreSQL backup failed or exceeded its bounded output size; live app was not stopped'
  fi
  [[ -s "$partial" ]] || fail 'PostgreSQL backup is empty'
  assert_backup_within_budget "$partial" "$postgresql_dump_budget_bytes" 'PostgreSQL backup'
  docker_rpc exec -i "$database_id" pg_restore -l < "$partial" > "$list_partial" || { rm -f -- "$partial" "$list_partial"; fail 'PostgreSQL backup catalog validation failed'; }
  [[ "$(wc -l < "$list_partial")" -gt 10 ]] || { rm -f -- "$partial" "$list_partial"; fail 'PostgreSQL backup catalog is unexpectedly short'; }
  mv -f -- "$partial" "$dump"
  mv -f -- "$list_partial" "$list"
  chmod 600 "$dump" "$list"
  hash_file "$dump" > "$dump.sha256"
  hash_file "$list" > "$list.sha256"
  chmod 600 "$dump.sha256" "$list.sha256"
}

backup_redis() {
  local rdb="$run_dir/redis.rdb" partial report="$run_dir/redis-check-rdb.txt" report_partial save_log="$run_dir/redis-save.log" save_log_partial tmp="/tmp/.subnexus-cutover-${run_id}.rdb" password username file_limit_kib
  partial="$(mktemp "$run_dir/.redis.rdb.partial.XXXXXX")" || fail 'cannot create Redis backup temporary file'
  report_partial="$(mktemp "$run_dir/.redis-check-rdb.partial.XXXXXX")" || { rm -f -- "$partial"; fail 'cannot create Redis report temporary file'; }
  save_log_partial="$(mktemp "$run_dir/.redis-save.partial.XXXXXX")" || { rm -f -- "$partial" "$report_partial"; fail 'cannot create Redis log temporary file'; }
  password="$(redis_password_value)"
  username="$(redis_username_value)"
  [[ "$redis_rdb_budget_bytes" =~ ^[0-9]+$ ]] || fail 'Redis backup budget is not initialized'
  file_limit_kib=$((redis_rdb_budget_bytes / 1024))
  if [[ -n "$username" ]]; then
    valid_container_ref "$username" || fail 'Redis username contains unsupported characters'
  fi
  { printf '%s\n' "$password"; } |
    docker_rpc exec -i "$redis_id" sh -c \
      'IFS= read -r password || exit 1; ulimit -f "$3" || exit 1; unset REDISCLI_AUTH; if [ -n "$password" ]; then REDISCLI_AUTH="$password"; export REDISCLI_AUTH; fi; if [ -n "$2" ]; then exec redis-cli --no-auth-warning --user "$2" --rdb "$1"; else exec redis-cli --no-auth-warning --rdb "$1"; fi' \
      sh "$tmp" "$username" "$file_limit_kib" > "$save_log_partial" 2>&1 || {
        docker_rpc exec "$redis_id" rm -f "$tmp" >/dev/null 2>&1 || true
        rm -f -- "$save_log_partial"
        fail 'Redis RDB snapshot failed or exceeded its bounded output size; live app was not stopped'
      }
  docker_rpc exec "$redis_id" redis-check-rdb "$tmp" > "$report_partial" || {
    docker_rpc exec "$redis_id" rm -f "$tmp" >/dev/null 2>&1 || true
    rm -f -- "$report_partial"
    fail 'Redis RDB validation failed in the live Redis container'
  }
  if ! (ulimit -f "$file_limit_kib" && docker_rpc cp "$redis_id:$tmp" "$partial"); then
    docker_rpc exec "$redis_id" rm -f "$tmp" >/dev/null 2>&1 || true
    rm -f -- "$partial" "$report_partial"
    fail 'cannot copy bounded Redis RDB to evidence directory'
  fi
  docker_rpc exec "$redis_id" rm -f "$tmp" >/dev/null || fail 'cannot remove temporary Redis RDB from container'
  [[ -s "$partial" ]] || { rm -f -- "$partial" "$report_partial"; fail 'Redis RDB backup is empty'; }
  assert_backup_within_budget "$partial" "$redis_rdb_budget_bytes" 'Redis RDB backup'
  mv -f -- "$report_partial" "$report"
  mv -f -- "$save_log_partial" "$save_log"
  mv -f -- "$partial" "$rdb"
  chmod 600 "$rdb" "$report" "$run_dir/redis-save.log"
  hash_file "$rdb" > "$rdb.sha256"
  hash_file "$report" > "$report.sha256"
  chmod 600 "$rdb.sha256" "$report.sha256"
}

assert_app_data_path_chain() {
  local path="$1" label="${2:-path}" leaf_uid="${3:-0}" leaf_gid="${4:-0}" expected_mode="${5:-}" allow_legacy="${6:-0}"
  local canonical current component actual_uid actual_gid actual_mode
  local -a components=()
  [[ "$path" == /* ]] || fail "$label must be an absolute path"
  canonical="$(realpath -e -P -- "$path")" || fail "cannot resolve $label"
  [[ "$canonical" == "$path" ]] || fail "$label contains a symbolic-link or non-canonical component"
  [[ "$leaf_uid" =~ ^[0-9]+$ ]] || fail "$label owner contract is invalid"
  [[ "$leaf_gid" == '*' || "$leaf_gid" =~ ^[0-9]+$ ]] || fail "$label owner contract is invalid"
  [[ "$(stat -c '%u' -- '/')" == '0' ]] || fail "$label filesystem root must be root-owned"
  mode_is_safe '/' || fail "$label filesystem root must not be group/other writable"
  IFS='/' read -r -a components <<< "${path#/}"
  current=''
  for component in "${components[@]}"; do
    [[ -n "$component" && "$component" != '.' && "$component" != '..' ]] || fail "$label contains an invalid path component"
    current="${current}/${component}"
    [[ -e "$current" && ! -L "$current" ]] || fail "$label contains a missing or symbolic path component"
    if [[ "$current" != "$path" ]]; then
      [[ "$(stat -c '%u' -- "$current")" == '0' ]] || fail "$label parent component must be root-owned: $current"
      [[ -d "$current" ]] || fail "$label parent component is not a directory: $current"
      mode_is_safe "$current" || fail "$label parent component must not be group/other writable: $current"
    else
      [[ -d "$current" ]] || fail "$label leaf must be a directory: $current"
      actual_uid="$(stat -c '%u' -- "$current")" || fail "cannot inspect $label leaf owner"
      actual_gid="$(stat -c '%g' -- "$current")" || fail "cannot inspect $label leaf group"
      actual_mode="$(stat -c '%a' -- "$current")" || fail "cannot inspect $label leaf mode"
      [[ "$actual_uid" == "$leaf_uid" && ( "$leaf_gid" == '*' || "$actual_gid" == "$leaf_gid" ) ]] ||
        fail "$label leaf owner does not match the prepared owner contract"
      if [[ "$allow_legacy" == 1 && "${app_data_owner_manifest_legacy:-0}" == 1 ]]; then
        mode_is_safe "$current" || fail "$label leaf must not be group/other writable"
      else
        app_data_owner_mode_is_safe "$actual_mode" ||
          fail "$label leaf must be owner-rwx and not group/other writable"
      fi
      [[ -z "$expected_mode" || "$actual_mode" == "$expected_mode" ]] ||
        fail "$label leaf mode changed after prepare"
    fi
  done
}

assert_root_owned_path_chain() {
  local path="$1" label="${2:-path}" canonical current component
  local -a components=()
  [[ "$path" == /* ]] || fail "$label must be an absolute path"
  canonical="$(realpath -e -P -- "$path")" || fail "cannot resolve $label"
  [[ "$canonical" == "$path" ]] || fail "$label contains a symbolic-link or non-canonical component"
  IFS='/' read -r -a components <<< "${path#/}"
  current=''
  for component in "${components[@]}"; do
    [[ -n "$component" && "$component" != '.' && "$component" != '..' ]] || fail "$label contains an invalid path component"
    current="${current}/${component}"
    [[ -e "$current" && ! -L "$current" ]] || fail "$label contains a missing or symbolic path component"
    [[ "$(stat -c '%u' -- "$current")" == '0' ]] || fail "$label path component must be root-owned: $current"
    if [[ "$current" != "$path" ]]; then
      [[ -d "$current" ]] || fail "$label parent component is not a directory: $current"
      mode_is_safe "$current" || fail "$label parent component must not be group/other writable: $current"
    fi
  done
}

resolve_app_data_source() {
  local type source name mountpoint forbidden expected_uid expected_gid
  IFS='|' read -r type source name < "$run_dir/app-data-mount.txt"
  if [[ "$type" == bind ]]; then
    [[ -d "$source" && ! -L "$source" ]] || fail 'application data bind source is missing or symbolic'
    mountpoint="$(realpath -e -P -- "$source")" || fail 'cannot resolve application data bind source'
    [[ "$mountpoint" == "$source" ]] || fail 'application data bind source contains a symbolic-link component'
  else
    [[ -n "$name" ]] || fail 'application data volume name is missing'
    mountpoint="$(docker_rpc volume inspect --format '{{.Mountpoint}}' "$name")" || fail 'cannot inspect application data volume'
    [[ -d "$mountpoint" && ! -L "$mountpoint" ]] || fail 'application data volume mountpoint is invalid'
    local resolved_mountpoint
    resolved_mountpoint="$(realpath -e -P -- "$mountpoint")" || fail 'cannot resolve application data volume mountpoint'
    [[ "$resolved_mountpoint" == "$mountpoint" ]] || fail 'application data volume mountpoint contains a symbolic-link component'
  fi
  case "${app_data_owner_policy:-root-only}" in
    root-only)
      expected_uid='0'
      if [[ -n "${app_data_owner_gid:-}" ]]; then
        expected_gid="$app_data_owner_gid"
      else
        # A manifest created before the owner contract recorded only the
        # historical root UID.  Preserve that exact compatibility boundary;
        # the immutable identity still locks the observed GID and mode.
        expected_gid='*'
      fi
      ;;
    explicit-uid-gid)
      expected_uid="${app_data_owner_uid:-}"
      expected_gid="${app_data_owner_gid:-}"
      [[ "$expected_uid" == "$app_data_owner_compat_uid" && "$expected_gid" == "$app_data_owner_compat_gid" ]] ||
        fail 'application data owner contract is unsupported'
      ;;
    *) fail 'application data owner policy is invalid' ;;
  esac
  assert_app_data_path_chain "$mountpoint" 'application data source' "$expected_uid" "$expected_gid" "${app_data_owner_mode:-}" 1
  [[ "$mountpoint" != '/' ]] || fail 'application data source cannot be the filesystem root'
  for forbidden in \
    "$run_dir" "$candidate_artifact_root" "$alternate_candidate_artifact_root" \
    "$candidate_gate_root" "$alternate_candidate_gate_root"; do
    [[ -n "$forbidden" ]] || continue
    path_overlaps "$mountpoint" "$forbidden" &&
      fail "application data source overlaps a cutover or candidate path: $forbidden"
  done
  printf '%s' "$mountpoint"
}

compute_app_data_source_identity() {
  local type source name mountpoint observed_name driver created_at options_json labels_json metadata_hash fingerprint
  IFS='|' read -r type source name < "$run_dir/app-data-mount.txt"
  mountpoint="$(resolve_app_data_source)" || return 1
  fingerprint="$(stat -Lc '%d,%i,%F,%u,%g,%a' -- "$mountpoint")" || return 1
  case "$type" in
    bind)
      [[ "$source" == "$mountpoint" && -e "$source" && ! -L "$source" ]] || return 1
      printf 'bind|%s|%s|%s|%s|-\n' "$source" "$name" "$mountpoint" "$fingerprint"
      ;;
    volume)
      [[ "$name" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]{0,254}$ ]] || return 1
      observed_name="$(docker_rpc volume inspect --format '{{.Name}}' "$name")" || return 1
      driver="$(docker_rpc volume inspect --format '{{.Driver}}' "$name")" || return 1
      created_at="$(docker_rpc volume inspect --format '{{.CreatedAt}}' "$name")" || return 1
      options_json="$(docker_rpc volume inspect --format '{{json .Options}}' "$name")" || return 1
      labels_json="$(docker_rpc volume inspect --format '{{json .Labels}}' "$name")" || return 1
      [[ "$observed_name" == "$name" && -n "$driver" && -n "$mountpoint" ]] || return 1
      [[ "$driver" != *$'\r'* && "$driver" != *$'\n'* && "$created_at" != *$'\r'* && "$created_at" != *$'\n'* ]] || return 1
      metadata_hash="$(printf '%s\n%s\n%s\n%s\n%s\n%s\n' "$observed_name" "$driver" "$mountpoint" "$created_at" "$options_json" "$labels_json" | hash_text)" || return 1
      printf 'volume|%s|%s|%s|%s|%s\n' "$source" "$name" "$mountpoint" "$fingerprint" "$metadata_hash"
      ;;
    *) return 1 ;;
  esac
}

capture_app_data_source_identity() {
  local identity temporary
  identity="$(compute_app_data_source_identity)" || fail 'cannot capture application data source identity'
  temporary="$(mktemp "$run_dir/.app-data-source.identity.XXXXXX")" || fail 'cannot create application data source identity temporary file'
  printf '%s\n' "$identity" > "$temporary" || { rm -f -- "$temporary"; fail 'cannot write application data source identity'; }
  chmod 600 -- "$temporary"
  mv -f -- "$temporary" "$run_dir/app-data-source.identity"
  chmod 600 -- "$run_dir/app-data-source.identity"
}

validate_app_data_source_identity_file() {
  local type source name mountpoint fingerprint metadata_hash extra
  local device inode file_type uid gid mode_value expected_uid expected_gid expected_mode
  assert_root_owned_regular "$run_dir/app-data-source.identity" 'application data source identity'
  IFS='|' read -r type source name mountpoint fingerprint metadata_hash extra < "$run_dir/app-data-source.identity"
  [[ -z "${extra:-}" && -n "$type" && -n "$mountpoint" && -n "$fingerprint" ]] || fail 'application data source identity is malformed'
  [[ "$source" != *$'\r'* && "$source" != *$'\n'* && "$source" != *'|'* && "$mountpoint" != *$'\r'* && "$mountpoint" != *$'\n'* && "$mountpoint" != *'|'* ]] ||
    fail 'application data source identity contains an invalid path'
  IFS=',' read -r device inode file_type uid gid mode_value extra <<< "$fingerprint"
  [[ -z "${extra:-}" && "$device" =~ ^[0-9]+$ && "$inode" =~ ^[0-9]+$ && -n "$file_type" &&
      "$uid" =~ ^[0-9]+$ && "$gid" =~ ^[0-9]+$ && "$mode_value" =~ ^[0-7]{3,4}$ ]] ||
    fail 'application data source fingerprint is malformed'
  expected_uid="${app_data_owner_uid:-0}"
  expected_gid="${app_data_owner_gid:-0}"
  [[ "$uid" == "$expected_uid" && ( -z "${app_data_owner_gid:-}" || "$gid" == "$expected_gid" ) ]] ||
    fail 'application data source owner changed or disagrees with the prepared contract'
  if [[ "${app_data_owner_manifest_legacy:-0}" == 1 ]]; then
    (( (8#${mode_value: -3} & 8#022) == 0 )) ||
      fail 'legacy application data source mode is group/other writable'
  else
    [[ "$mode_value" =~ ^[0-7]{3}$ ]] ||
      fail 'modern application data source mode must not contain special bits'
    app_data_owner_mode_is_safe "$mode_value" ||
      fail 'application data source mode is not owner-rwx and private to group/other writes'
  fi
  expected_mode="${app_data_owner_mode:-}"
  [[ -z "$expected_mode" || "$mode_value" == "$expected_mode" ]] ||
    fail 'application data source mode disagrees with the prepared manifest'
  case "$type" in
    bind)
      [[ -z "$name" && "$metadata_hash" == '-' ]] || fail 'bind source identity metadata is malformed'
      ;;
    volume)
      [[ "$name" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]{0,254}$ && "$metadata_hash" =~ ^[0-9a-f]{64}$ ]] ||
        fail 'volume source identity metadata is malformed'
      ;;
    *) fail 'application data source identity type is unsupported' ;;
  esac
}

assert_app_data_source_identity() {
  local expected actual
  if [[ -f "${manifest_file:-}" ]]; then
    validate_app_data_owner_manifest
  else
    validate_app_data_owner_inputs
  fi
  validate_app_data_source_identity_file
  expected="$(read_one_line "$run_dir/app-data-source.identity")"
  actual="$(compute_app_data_source_identity)" || fail 'cannot recapture application data source identity'
  [[ "$actual" == "$expected" ]] || fail 'application data source identity changed after prepare'
}

write_application_data_archive_policy() {
  local file="$run_dir/application-data-exclusions.txt" sidecar="$run_dir/application-data-exclusions.txt.sha256" tmp sidecar_tmp
  [[ ! -e "$file" && ! -L "$file" && ! -e "$sidecar" && ! -L "$sidecar" ]] ||
    fail 'application data archive policy paths already exist'
  tmp="$(mktemp "$run_dir/.application-data-exclusions.XXXXXX")" || fail 'cannot create application data archive policy temporary file'
  if ! {
    printf 'policy=%s\n' "$app_data_archive_policy_version"
    printf 'exclude=%s\n' "$app_data_archive_exclusion_pattern"
    printf 'retain=./logs/*.gz and all other application data\n'
    printf 'reason=live application log files are excluded because they may change while the online backup runs\n'
  } > "$tmp"; then
    rm -f -- "$tmp"
    fail 'cannot write application data archive policy'
  fi
  chmod 600 -- "$tmp"
  assert_root_owned_regular "$tmp" 'application data archive policy temporary file'
  mv -f -- "$tmp" "$file"
  assert_root_owned_regular "$file" 'application data archive policy'
  sidecar_tmp="$(mktemp "$run_dir/.application-data-exclusions.sha256.XXXXXX")" || fail 'cannot create application data archive policy hash temporary file'
  if ! hash_file "$file" > "$sidecar_tmp"; then
    rm -f -- "$sidecar_tmp"
    fail 'cannot hash application data archive policy'
  fi
  chmod 600 -- "$sidecar_tmp"
  assert_root_owned_regular "$sidecar_tmp" 'application data archive policy hash temporary file'
  mv -f -- "$sidecar_tmp" "$sidecar"
  assert_root_owned_regular "$sidecar" 'application data archive policy hash'
  app_data_archive_policy_sha256="$(read_one_line "$sidecar")"
  valid_sha64 "$app_data_archive_policy_sha256" || fail 'application data archive policy hash is invalid'
}

validate_application_data_archive_policy() {
  local file="$run_dir/application-data-exclusions.txt" sidecar="$run_dir/application-data-exclusions.txt.sha256" line count=0 expected actual
  assert_root_owned_regular "$file" 'application data archive policy'
  while IFS= read -r line; do
    case "$count" in
      0) [[ "$line" == "policy=$app_data_archive_policy_version" ]] || fail 'application data archive policy version is invalid' ;;
      1) [[ "$line" == "exclude=$app_data_archive_exclusion_pattern" ]] || fail 'application data archive exclusion is invalid' ;;
      2) [[ "$line" == 'retain=./logs/*.gz and all other application data' ]] || fail 'application data archive retention policy is invalid' ;;
      3) [[ "$line" == 'reason=live application log files are excluded because they may change while the online backup runs' ]] || fail 'application data archive policy reason is invalid' ;;
      *) fail 'application data archive policy contains unexpected lines' ;;
    esac
    count=$((count + 1))
  done < "$file"
  [[ "$count" == 4 ]] || fail 'application data archive policy is incomplete'
  assert_root_owned_regular "$sidecar" 'application data archive policy hash'
  expected="$(read_one_line "$sidecar")"
  valid_sha64 "$expected" || fail 'application data archive policy hash is invalid'
  actual="$(hash_file "$file")" || fail 'cannot hash application data archive policy'
  [[ "$actual" == "$expected" ]] || fail 'application data archive policy hash changed'
  app_data_archive_policy_sha256="$actual"
}

backup_application_data() {
  local source="$1" archive="$run_dir/application-data.tar.gz" partial file_limit_kib
  local -a tar_args=()
  validate_application_data_archive_policy
  partial="$(mktemp "$run_dir/.application-data.partial.XXXXXX")" || fail 'cannot create application data backup temporary file'
  [[ "$app_data_archive_budget_bytes" =~ ^[0-9]+$ ]] || fail 'application data backup budget is not initialized'
  file_limit_kib=$((app_data_archive_budget_bytes / 1024))
  tar_args=(--xattrs --acls --numeric-owner --one-file-system "--exclude=$app_data_archive_exclusion_pattern" -C "$source" -czf "$partial")
  if ! (ulimit -f "$file_limit_kib" && exec tar "${tar_args[@]}" .); then
    rm -f -- "$partial"
    fail 'application data backup failed or exceeded its bounded output size; live app was not stopped'
  fi
  [[ -s "$partial" ]] || fail 'application data backup is empty'
  assert_backup_within_budget "$partial" "$app_data_archive_budget_bytes" 'application data backup'
  tar -tzf "$partial" >/dev/null || fail 'application data archive validation failed'
  mv -f -- "$partial" "$archive"
  chmod 600 "$archive"
  hash_file "$archive" > "$archive.sha256"
  chmod 600 "$archive.sha256"
}

assert_backup_hashes() {
  local file expected actual manifest_key manifest_expected
  validate_application_data_archive_policy
  if [[ -f "${manifest_file:-}" ]]; then
    manifest_expected="$(manifest_value application_data_archive_policy_sha256)"
    valid_sha64 "$manifest_expected" || fail 'manifest application data archive policy SHA is invalid'
    [[ "$manifest_expected" == "$app_data_archive_policy_sha256" ]] || fail 'manifest application data archive policy hash mismatch'
  fi
  for file in postgresql.dump postgresql.list redis.rdb redis-check-rdb.txt application-data.tar.gz; do
    [[ -f "$run_dir/$file" && -f "$run_dir/$file.sha256" ]] || fail "prepared backup is missing: $file"
    assert_root_owned_regular "$run_dir/$file" "prepared backup $file"
    assert_root_owned_regular "$run_dir/$file.sha256" "prepared backup hash $file"
    expected="$(read_one_line "$run_dir/$file.sha256")"
    valid_sha64 "$expected" || fail "backup hash is invalid: $file"
    actual="$(hash_file "$run_dir/$file")" || fail "cannot hash prepared backup: $file"
    [[ "$actual" == "$expected" ]] || fail "prepared backup hash mismatch: $file"
    case "$file" in
      postgresql.dump) manifest_key=postgresql_dump_sha256 ;;
      postgresql.list) manifest_key=postgresql_list_sha256 ;;
      redis.rdb) manifest_key=redis_rdb_sha256 ;;
      redis-check-rdb.txt) manifest_key=redis_check_rdb_sha256 ;;
      application-data.tar.gz) manifest_key=application_data_sha256 ;;
    esac
    if [[ -f "${manifest_file:-}" ]]; then
      manifest_expected="$(manifest_value "$manifest_key")"
      valid_sha64 "$manifest_expected" || fail "manifest backup hash is invalid: $file"
      [[ "$manifest_expected" == "$actual" ]] || fail "manifest backup hash mismatch: $file"
    fi
  done
  tar -tzf "$run_dir/application-data.tar.gz" >/dev/null || fail 'prepared application data archive no longer validates'
}

write_initial_manifest() {
  local source_tree app_data_source_value observed_uid observed_gid observed_mode
  source_tree="$(read_one_line "$run_dir/source-tree")"
  app_data_source_value="$(resolve_app_data_source)"
  observed_uid="$(stat -c '%u' -- "$app_data_source_value")" || fail 'cannot inspect application data owner UID'
  observed_gid="$(stat -c '%g' -- "$app_data_source_value")" || fail 'cannot inspect application data owner GID'
  observed_mode="$(stat -c '%a' -- "$app_data_source_value")" || fail 'cannot inspect application data mode'
  [[ "$observed_uid" == "$app_data_owner_uid" && "$observed_gid" == "$app_data_owner_gid" ]] ||
    fail 'application data owner changed before manifest creation'
  app_data_owner_mode_is_safe "$observed_mode" || fail 'application data mode is not safe for the candidate'
  app_data_owner_mode="$observed_mode"
  {
    printf 'tool=%s\n' "$tool_name"
    printf 'state=prepared\n'
    printf 'run_id=%s\n' "$run_id"
    printf 'script_sha256=%s\n' "$script_sha256"
    printf 'target_sha=%s\n' "$target_sha"
    printf 'source_tree=%s\n' "$source_tree"
    printf 'source_root=%s\n' "$source_root"
    printf 'candidate_image_id=%s\n' "$expected_image_id"
    printf 'candidate_archive_sha256=%s\n' "$candidate_archive_sha"
    printf 'candidate_archive=%s\n' "$candidate_archive"
    printf 'candidate_gate_evidence=%s\n' "$candidate_gate_evidence"
    printf 'candidate_gate_evidence_sha256=%s\n' "$candidate_gate_evidence_sha256"
    printf 'live_app_id=%s\n' "$app_id"
    printf 'live_app_name=%s\n' "$app_name"
    printf 'live_app_image_id=%s\n' "$app_image_id"
    printf 'environment_duplicate_mode=%s\n' "$environment_duplicate_mode"
    printf 'environment_duplicate_keys=%s\n' "$environment_duplicate_keys"
    printf 'environment_duplicate_expected_sha256=%s\n' "$environment_duplicate_expected_hashes"
    printf 'environment_file_sha256=%s\n' "$environment_file_sha256"
    printf 'environment_duplicate_evidence_sha256=%s\n' "$environment_duplicate_evidence_sha256"
    printf 'app_data_owner_policy=%s\n' "$app_data_owner_policy"
    printf 'app_data_owner_uid=%s\n' "$observed_uid"
    printf 'app_data_owner_gid=%s\n' "$observed_gid"
    printf 'app_data_owner_mode=%s\n' "$observed_mode"
    printf 'runtime_contract_sha256=%s\n' "$(read_one_line "$run_dir/runtime-contract.sha256")"
    printf 'log_config_sha256=%s\n' "$(hash_file "$run_dir/log-config.json")"
    printf 'database_id=%s\n' "$database_id"
    printf 'redis_id=%s\n' "$redis_id"
    printf 'database_identity_file=database.identity\n'
    printf 'redis_identity_file=redis.identity\n'
    printf 'docker_socket_fingerprint=%s\n' "$docker_socket_fingerprint"
    printf 'docker_daemon_identity=%s\n' "$docker_daemon_identity"
    printf 'postgresql_dump_sha256=%s\n' "$(read_one_line "$run_dir/postgresql.dump.sha256")"
    printf 'postgresql_list_sha256=%s\n' "$(read_one_line "$run_dir/postgresql.list.sha256")"
    printf 'redis_rdb_sha256=%s\n' "$(read_one_line "$run_dir/redis.rdb.sha256")"
    printf 'redis_check_rdb_sha256=%s\n' "$(read_one_line "$run_dir/redis-check-rdb.txt.sha256")"
    printf 'application_data_sha256=%s\n' "$(read_one_line "$run_dir/application-data.tar.gz.sha256")"
    printf 'application_data_archive_policy_sha256=%s\n' "$(read_one_line "$run_dir/application-data-exclusions.txt.sha256")"
    printf 'settings_snapshot_sha256=%s\n' "$(read_one_line "$run_dir/settings-before.tsv.sha256")"
    printf 'app_data_source_identity_sha256=%s\n' "$(hash_file "$run_dir/app-data-source.identity")"
    printf 'local_port=%s\n' "$local_port"
    printf 'host_ip=%s\n' "$selected_host_ip"
    printf 'app_data_source=%s\n' "$app_data_source_value"
    printf 'public_url=%s\n' "$public_url"
    printf 'prepared_at=%s\n' "$(date --iso-8601=seconds)"
    printf 'cutover_stop_timeout_seconds=%s\n' "$stop_timeout_seconds"
    printf 'candidate_health_timeout_seconds=%s\n' "$candidate_health_timeout_seconds"
    printf 'rollback_health_timeout_seconds=%s\n' "$rollback_health_timeout_seconds"
    printf 'candidate_stability_seconds=%s\n' "$candidate_stability_seconds"
    printf 'preserved_container=\n'
    printf 'candidate_container_id=\n'
    printf 'candidate_container_name=\n'
    printf 'candidate_container_intent=\n'
    printf 'settings_closed_snapshot_sha256=%s\n' "$(read_one_line "$run_dir/settings-closed.tsv.sha256")"
  } > "$manifest_file"
  chmod 600 "$manifest_file"
}

validate_bounded_settings() {
  for setting in SUBNEXUS_CUTOVER_STOP_TIMEOUT_SECONDS SUBNEXUS_CANDIDATE_HEALTH_TIMEOUT_SECONDS SUBNEXUS_ROLLBACK_HEALTH_TIMEOUT_SECONDS SUBNEXUS_CANDIDATE_STABILITY_SECONDS; do
    local value="${!setting:-}"
    [[ -z "$value" || "$value" =~ ^[0-9]+$ ]] || fail "$setting must be an integer"
  done
  local stop="${SUBNEXUS_CUTOVER_STOP_TIMEOUT_SECONDS:-5}" health="${SUBNEXUS_CANDIDATE_HEALTH_TIMEOUT_SECONDS:-180}" rollback="${SUBNEXUS_ROLLBACK_HEALTH_TIMEOUT_SECONDS:-180}" stable="${SUBNEXUS_CANDIDATE_STABILITY_SECONDS:-20}"
  (( stop >= 1 && stop <= 30 )) || fail 'cutover stop timeout must be 1..30 seconds'
  (( health >= 30 && health <= 900 )) || fail 'candidate health timeout must be 30..900 seconds'
  (( rollback >= 30 && rollback <= 900 )) || fail 'rollback health timeout must be 30..900 seconds'
  (( stable >= 5 && stable <= 120 )) || fail 'candidate stability window must be 5..120 seconds'
  stop_timeout_seconds="$stop"
  candidate_health_timeout_seconds="$health"
  rollback_health_timeout_seconds="$rollback"
  candidate_stability_seconds="$stable"
}

validate_manifest_bounded_settings() {
  local stop health rollback stable
  stop="$(manifest_value cutover_stop_timeout_seconds)"
  health="$(manifest_value candidate_health_timeout_seconds)"
  rollback="$(manifest_value rollback_health_timeout_seconds)"
  stable="$(manifest_value candidate_stability_seconds)"
  [[ "$stop" =~ ^[0-9]+$ && "$health" =~ ^[0-9]+$ && "$rollback" =~ ^[0-9]+$ && "$stable" =~ ^[0-9]+$ ]] ||
    fail 'prepared timeout settings are missing or malformed'
  (( stop >= 1 && stop <= 30 )) || fail 'prepared cutover stop timeout is outside 1..30 seconds'
  (( health >= 30 && health <= 900 )) || fail 'prepared candidate health timeout is outside 30..900 seconds'
  (( rollback >= 30 && rollback <= 900 )) || fail 'prepared rollback health timeout is outside 30..900 seconds'
  (( stable >= 5 && stable <= 120 )) || fail 'prepared candidate stability window is outside 5..120 seconds'
  stop_timeout_seconds="$stop"
  candidate_health_timeout_seconds="$health"
  rollback_health_timeout_seconds="$rollback"
  candidate_stability_seconds="$stable"
}

validate_resource_policy_file() {
  local file="$1" memory memory_swap memory_reservation nano_cpus cpu_shares cpu_quota cpu_period cpuset pids extra
  assert_root_owned_regular "$file" 'resource policy'
  IFS='|' read -r memory memory_swap memory_reservation nano_cpus cpu_shares cpu_quota cpu_period cpuset pids extra < "$file"
  [[ -z "${extra:-}" ]] || fail 'resource policy has too many fields'
  for value in "$memory" "$memory_swap" "$memory_reservation" "$nano_cpus" "$cpu_shares" "$cpu_quota" "$cpu_period" "$pids"; do
    [[ "$value" =~ ^-?[0-9]+$ ]] || fail 'resource policy contains a non-numeric limit'
  done
  [[ "$cpuset" =~ ^[0-9,-]*$ ]] || fail 'resource policy CPU set is invalid'
}

validate_log_config_file() {
  local file="$1"
  assert_root_owned_regular "$file" 'log configuration metadata'
  if ! python3 - "$file" <<'PY'
import json
import re
import sys

def reject(message):
    raise SystemExit(message)

def reject_duplicate_pairs(pairs):
    value = {}
    for key, item in pairs:
        if key in value:
            reject("duplicate log configuration key")
        value[key] = item
    return value

try:
    with open(sys.argv[1], encoding="utf-8") as handle:
        log_config = json.load(handle, object_pairs_hook=reject_duplicate_pairs)
except (OSError, ValueError, TypeError) as exc:
    raise SystemExit("invalid log configuration metadata") from exc

if not isinstance(log_config, dict):
    reject("log configuration must be an object")
if any(key not in ("Type", "Config") for key in log_config):
    reject("unsupported log configuration field")
log_type = log_config.get("Type")
if log_type not in (None, "", "json-file"):
    reject("unsupported log driver")
options = log_config.get("Config")
if options in (None, {}):
    options = {}
if not isinstance(options, dict):
    reject("log configuration options must be an object")
allowed = {"max-file", "max-size"}
if any(key not in allowed for key in options):
    reject("unsupported json-file option")
for key, value in options.items():
    if not isinstance(value, str):
        reject("log configuration option values must be strings")
    if key == "max-file":
        if not re.fullmatch(r"[1-9][0-9]{0,3}", value) or int(value) > 1000:
            reject("invalid max-file")
    elif key == "max-size":
        if not re.fullmatch(r"[1-9][0-9]{0,9}(?:[bBkKmMgGtT])?", value):
            reject("invalid max-size")
if options and set(options) != allowed:
    reject("max-file and max-size must be supplied together")
if options and log_type not in ("json-file",):
    reject("log driver must be explicit when options are supplied")
PY
  then
    fail 'log configuration metadata is unsupported or malformed'
  fi
}

validate_runtime_metadata_files() {
  local user network network_id extra count=0 type source name destination mode writable propagation mount_extra mount_count=0 app_data_count=0 line entrypoint_count=0
  local file
  local -a checked_security_args=() checked_log_args=()
  for file in container.env networks.txt network-identities.txt network-aliases.txt security-opt.txt restart-policy.txt restart-retries.txt workdir.txt entrypoint.txt cmd.txt mounts.txt app-data-mount.txt app-data-source.identity ports.txt user.txt resource-policy.txt healthcheck.json log-config.json ulimits.txt runtime-contract.sha256; do
    assert_root_owned_regular "$run_dir/$file" "runtime metadata $file"
  done
  validate_environment_file "$run_dir/container.env"
  if [[ "$environment_duplicate_legacy" != 1 ]]; then
    assert_root_owned_regular "$run_dir/environment-duplicates.tsv" 'runtime metadata environment-duplicates.tsv'
    validate_environment_duplicate_evidence "$run_dir/environment-duplicates.tsv" "$run_dir/container.env" "$environment_duplicate_mode" "$environment_duplicate_keys" "$environment_duplicate_expected_hashes"
  fi
  validate_security_options_file "$run_dir/security-opt.txt"
  [[ "$(read_one_line "$run_dir/runtime-contract.sha256")" =~ ^[0-9a-f]{64}$ ]] || fail 'runtime contract hash is invalid'
  assert_root_owned_regular "$run_dir/user.txt" 'container user metadata'
  user="$(read_one_line "$run_dir/user.txt")"
  [[ -z "$user" || "$user" =~ ^[A-Za-z0-9_.:-]{1,128}$ ]] || fail 'container user metadata is invalid'
  validate_resource_policy_file "$run_dir/resource-policy.txt"
  assert_root_owned_regular "$run_dir/network-identities.txt" 'network identity metadata'
  while IFS='|' read -r network network_id extra; do
    [[ -n "$network" && -n "$network_id" && -z "${extra:-}" ]] || fail 'network identity metadata is malformed'
    [[ "$network" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]{0,254}$ ]] || fail 'network identity name is invalid'
    valid_container_ref "$network_id" || fail 'network identity ID is invalid'
    count=$((count + 1))
  done < "$run_dir/network-identities.txt"
  (( count > 0 )) || fail 'network identity metadata is empty'
  while IFS='|' read -r network alias extra; do
    [[ -z "${network:-}" && -z "${alias:-}" && -z "${extra:-}" ]] && continue
    [[ -n "$network" && -n "$alias" && -z "${extra:-}" ]] || fail 'network alias metadata is malformed'
    [[ "$network" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]{0,254}$ && "$alias" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]{0,254}$ ]] || fail 'network alias metadata is invalid'
  done < "$run_dir/network-aliases.txt"
  while IFS='|' read -r type source name destination mode writable propagation mount_extra; do
    [[ -n "$type" && -n "$destination" && -z "${mount_extra:-}" ]] || fail 'mount metadata is malformed'
    [[ "$destination" == /* && "$destination" != *$'\r'* && "$destination" != *'|'* && "$destination" != *','* && "$destination" != *'='* ]] || fail 'mount destination metadata is invalid'
    [[ "$mode" == rw || "$mode" == ro || -z "$mode" ]] || fail 'mount mode metadata is invalid'
    [[ "$writable" == true || "$writable" == false ]] || fail 'mount writable metadata is invalid'
    if [[ "$mode" == ro ]]; then
      [[ "$writable" == false ]] || fail 'mount mode/writable metadata conflicts'
    elif [[ "$mode" == rw || -z "$mode" ]]; then
      [[ "$writable" == true ]] || fail 'mount mode/writable metadata conflicts'
    fi
    [[ -z "$propagation" || "$propagation" =~ ^(private|rprivate|shared|rshared|slave|rslave)$ ]] ||
      fail 'mount propagation metadata is invalid'
    case "$type" in
      bind)
        [[ "$source" == /* && -e "$source" && ! -L "$source" ]] || fail 'bind mount source metadata is invalid'
        [[ "$(realpath -e -P -- "$source")" == "$source" ]] || fail 'bind mount source metadata contains a symbolic-link component'
        ;;
      volume)
        [[ "$name" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]{0,254}$ ]] || fail 'volume mount name metadata is invalid'
        [[ -z "$propagation" || "$propagation" == rprivate ]] || fail 'non-default volume propagation metadata is unsupported'
        ;;
      *) fail "unsupported mount metadata type: $type" ;;
    esac
    mount_count=$((mount_count + 1))
    [[ "$destination" == /app/data ]] && app_data_count=$((app_data_count + 1))
  done < "$run_dir/mounts.txt"
  (( mount_count > 0 && app_data_count == 1 )) || fail 'mount metadata must contain exactly one /app/data mount'
  while IFS= read -r line; do
    [[ "$line" != *$'\r'* ]] || fail 'entrypoint metadata contains a carriage return'
    [[ -z "$line" ]] || entrypoint_count=$((entrypoint_count + 1))
  done < "$run_dir/entrypoint.txt"
  (( entrypoint_count <= 1 )) || fail 'multi-element entrypoint cannot be reproduced safely'
  while IFS= read -r line; do
    [[ "$line" != *$'\r'* ]] || fail 'command metadata contains a carriage return'
  done < "$run_dir/cmd.txt"
  python3 -c 'import json,sys; value=json.load(open(sys.argv[1])); assert value is None or isinstance(value,dict)' "$run_dir/healthcheck.json" || fail 'healthcheck metadata is malformed'
  validate_log_config_file "$run_dir/log-config.json"
  while IFS='|' read -r name soft hard extra; do
    [[ -n "${name:-}" && -z "${extra:-}" && "$name" =~ ^[A-Za-z0-9_.-]{1,64}$ && "$soft" =~ ^-?[0-9]+$ && "$hard" =~ ^-?[0-9]+$ ]] || fail 'ulimit metadata is malformed'
  done < "$run_dir/ulimits.txt"
  append_security_opt_args checked_security_args
  append_log_config_args checked_log_args
  candidate_restart_arg >/dev/null
  validate_mount_recreation_contract
  validate_app_data_source_identity_file
}

prepare_run() {
  prepare_argument_count_is_valid "$#" || { usage; exit 2; }
  source_root="$1"; target_sha="$2"; approved_script_sha="$3"; expected_image_id="$4"; candidate_archive="$5"; candidate_archive_sha="$6"; candidate_gate_evidence="$7"; live_app_ref="$8"; public_url="${9:-}"; evidence_root="${10:-$default_evidence_root}"
  [[ "$EUID" -eq 0 ]] || fail 'prepare must run as root'
  validate_app_data_owner_inputs
  valid_sha40 "$target_sha" || fail 'target SHA must be a lowercase 40-character commit SHA'
  valid_sha64 "$expected_image_id" || fail 'candidate image ID must be 64 lowercase hexadecimal characters'
  valid_sha64 "$candidate_archive_sha" || fail 'candidate archive SHA must be 64 lowercase hexadecimal characters'
  valid_container_ref "$live_app_ref" || fail 'live application container reference is invalid'
  [[ -z "$public_url" || "$public_url" =~ ^https?://[^@[:space:]]+$ ]] || fail 'public health URL must be an absolute http(s) URL'
  validate_self_sha "$approved_script_sha"
  require_commands
  init_docker
  validate_bounded_settings
  local evidence_info evidence_base
  if [[ -e "$evidence_root" || -L "$evidence_root" ]]; then
    evidence_info="$(assert_approved_path "$evidence_root" evidence)"
    evidence_root="${evidence_info%%|*}"
    evidence_base="${evidence_info#*|}"
  else
    evidence_root="$(realpath -m -P -- "$evidence_root")" || fail 'cannot normalize evidence root'
    if path_equal_or_under "$evidence_root" "$default_evidence_root"; then
      evidence_base="$default_evidence_root"
    elif path_equal_or_under "$evidence_root" "$alternate_evidence_root"; then
      evidence_base="$alternate_evidence_root"
    else
      fail 'evidence root must be below an approved evidence root'
    fi
  fi
  [[ "$evidence_base" == "$default_evidence_root" || "$evidence_base" == "$alternate_evidence_root" ]] || fail 'evidence root base is invalid'
  ensure_root_owned_dir "$evidence_root" 'cutover evidence root'
  evidence_lock_root="$evidence_base"
  acquire_lock "$evidence_lock_root"
  run_id="$(date +%Y%m%d%H%M%S)-$$"
  run_dir="$evidence_root/$run_id"
  mkdir -- "$run_dir" || fail 'cutover run directory already exists'
  chmod 700 -- "$run_dir"
  manifest_file="$run_dir/manifest.env"
  trap 'on_error' ERR
  validate_source_tree
  validate_gate_evidence
  validate_archive
  capture_runtime_metadata
  resolve_dependencies
  capture_settings_snapshot "$run_dir/settings-before.tsv"
  printf '%s\n' "$(hash_file "$run_dir/settings-before.tsv")" > "$run_dir/settings-before.tsv.sha256"
  chmod 600 "$run_dir/settings-before.tsv.sha256"
  # Build the immutable closed-state intent while the live database is still
  # untouched.  The switch phase only applies and verifies this snapshot; it
  # must never derive rollback data after the destructive window begins.
  write_closed_settings_snapshot
  app_data_source="$(resolve_app_data_source)"
  capture_app_data_source_identity
  initialize_prepare_backup_budgets "$app_data_source"
  assert_prepare_disk_budget before_postgresql
  backup_postgresql
  assert_prepare_disk_budget before_redis
  backup_redis
  assert_prepare_disk_budget before_application_data
  write_application_data_archive_policy
  backup_application_data "$app_data_source"
  assert_prepare_disk_budget before_image_load
  load_and_validate_candidate_image
  assert_prepare_disk_budget after_image_load
  write_initial_manifest
  # The source identity was captured before the bounded backups and image
  # load.  Recheck it after the manifest is materialized so an owner/mode or
  # inode change during prepare can never produce a READY run.
  assert_app_data_source_identity
  assert_backup_hashes
  assert_live_identity 'prepare-final'
  assert_dependencies_still_match "$database_id" "$redis_id"
  write_run_marker READY prepared
  log "PREPARED_RUN_DIRECTORY=$run_dir"
  log "No production container was stopped, renamed, restarted, or switched."
  trap - ERR
}

validate_run_directory() {
  local validation_scope="${2:-switch}" info resolved root state expected actual gate_info manifest_gate_sha daemon_expected archive_info live_image_id identity_expected closed_hash closed_sidecar_hash archive_policy_hash
  [[ "$validation_scope" == switch || "$validation_scope" == rollback ]] || fail 'unknown run-directory validation scope'
  [[ "$1" == /* ]] || fail 'run directory must be an absolute path'
  info="$(assert_approved_path "$1" evidence)"
  resolved="${info%%|*}"; root="${info#*|}"
  [[ "$resolved" != "$root" ]] || fail 'run directory must be below the evidence root'
  evidence_lock_root="$root"
  run_dir="$resolved"
  manifest_file="$run_dir/manifest.env"
  assert_root_owned_dir "$run_dir" 'cutover run directory'
  assert_root_owned_regular "$manifest_file" 'cutover manifest'
  validate_manifest_shape
  [[ "$(manifest_value tool)" == "$tool_name" ]] || fail 'manifest belongs to a different tool'
  state="$(manifest_value state)"
  [[ "$state" == prepared || "$state" == switching || "$state" == switched || "$state" == rolling_back || "$state" == rolled_back ]] || fail "unsupported cutover state: $state"
  assert_run_marker READY prepared
  case "$state" in
    prepared)
      [[ ! -e "$run_dir/SWITCHED" && ! -L "$run_dir/SWITCHED" && ! -e "$run_dir/ROLLED_BACK" && ! -L "$run_dir/ROLLED_BACK" ]] ||
        fail 'prepared run contains a terminal-state marker'
      ;;
    switching)
      [[ ! -e "$run_dir/ROLLED_BACK" && ! -L "$run_dir/ROLLED_BACK" ]] || fail 'switching run unexpectedly contains a rollback marker'
      ;;
    switched)
      assert_run_marker SWITCHED switched
      [[ ! -e "$run_dir/ROLLED_BACK" && ! -L "$run_dir/ROLLED_BACK" ]] || fail 'switched run unexpectedly contains a rollback marker'
      ;;
    rolling_back) ;;
    rolled_back) assert_run_marker ROLLED_BACK rolled_back ;;
  esac
  [[ "$(manifest_value run_id)" =~ ^[0-9]{14}-[0-9]+$ ]] || fail 'manifest run ID is invalid'
  expected="${SUBNEXUS_APPROVED_CUTOVER_SCRIPT_SHA256:-}"
  valid_sha64 "$expected" || fail 'SUBNEXUS_APPROVED_CUTOVER_SCRIPT_SHA256 must be supplied independently'
  validate_self_sha "$expected"
  [[ "$(manifest_value script_sha256)" == "$script_sha256" ]] || fail 'manifest script SHA does not match current script'
  validate_manifest_bounded_settings
  validate_app_data_owner_manifest
  target_sha="$(manifest_value target_sha)"; valid_sha40 "$target_sha" || fail 'manifest target SHA is invalid'
  expected_image_id="$(manifest_value candidate_image_id)"; valid_sha64 "$expected_image_id" || fail 'manifest candidate image ID is invalid'
  # Both switch and rollback retain the immutable candidate metadata in the
  # manifest, but only switch needs the candidate files themselves.  Rollback
  # must remain available when an operator has removed/corrupted a release
  # archive or one of the prepare-time backups; it restores the preserved
  # container and settings only.
  candidate_archive_sha="$(manifest_value candidate_archive_sha256)"
  valid_sha64 "$candidate_archive_sha" || fail 'manifest archive SHA is invalid'
  candidate_archive="$(manifest_value candidate_archive)"
  candidate_gate_evidence="$(manifest_value candidate_gate_evidence)"
  manifest_gate_sha="$(manifest_value candidate_gate_evidence_sha256)"
  valid_sha64 "$manifest_gate_sha" || fail 'manifest candidate gate evidence SHA is invalid'
  candidate_gate_evidence_sha256="$manifest_gate_sha"
  if [[ "$validation_scope" == switch ]]; then
    archive_info="$(assert_approved_path "$candidate_archive" candidate_archive)"
    candidate_archive="${archive_info%%|*}"
    assert_root_owned_regular "$candidate_archive" 'candidate image archive'
    actual="$(hash_file "$candidate_archive")"; [[ "$actual" == "$candidate_archive_sha" ]] || fail 'candidate archive changed after prepare'
    [[ -n "$candidate_gate_evidence" ]] || fail 'manifest candidate gate evidence path is missing'
    validate_gate_evidence
    assert_backup_hashes
  fi
  [[ -f "$run_dir/settings-before.tsv" ]] || fail 'rollout setting snapshot is missing'
  assert_root_owned_regular "$run_dir/settings-before.tsv" 'rollout setting snapshot'
  assert_root_owned_regular "$run_dir/settings-before.tsv.sha256" 'rollout setting snapshot hash'
  expected="$(read_one_line "$run_dir/settings-before.tsv.sha256")"; actual="$(hash_file "$run_dir/settings-before.tsv")"; [[ "$expected" == "$actual" ]] || fail 'rollout setting snapshot changed'
  expected="$(manifest_value settings_snapshot_sha256)"; valid_sha64 "$expected" || fail 'manifest settings snapshot SHA is invalid'; [[ "$expected" == "$actual" ]] || fail 'manifest settings snapshot hash mismatch'
  validate_settings_snapshot "$run_dir/settings-before.tsv"
  assert_root_owned_regular "$run_dir/app-data-source.identity" 'application data source identity'
  identity_expected="$(manifest_value app_data_source_identity_sha256)"
  valid_sha64 "$identity_expected" || fail 'manifest application data source identity hash is invalid'
  actual="$(hash_file "$run_dir/app-data-source.identity")" || fail 'cannot hash application data source identity'
  [[ "$identity_expected" == "$actual" ]] || fail 'application data source identity hash mismatch'
  validate_app_data_source_identity_file
  if [[ "$validation_scope" == switch ]]; then
    manifest_has_key application_data_archive_policy_sha256 || fail 'manifest application data archive policy SHA is missing'
    archive_policy_hash="$(manifest_value application_data_archive_policy_sha256)"
    validate_application_data_archive_policy
    valid_sha64 "$archive_policy_hash" || fail 'manifest application data archive policy SHA is invalid'
    [[ "$archive_policy_hash" == "$app_data_archive_policy_sha256" ]] || fail 'manifest application data archive policy hash mismatch'
  else
    log 'Rollback does not read application data archive policy evidence; continuing with container and settings recovery checks.'
  fi
  closed_hash="$(manifest_value settings_closed_snapshot_sha256)"
  case "$state" in
    prepared)
      # The immutable closed-state intent is generated during prepare while
      # the live database is untouched.  Persisting it before switch makes a
      # crash after gate closure recoverable without guessing what was written.
      valid_sha64 "$closed_hash" || fail 'prepared closed settings snapshot hash is missing or invalid'
      assert_root_owned_regular "$run_dir/settings-closed.tsv" 'closed rollout setting snapshot'
      assert_root_owned_regular "$run_dir/settings-closed.tsv.sha256" 'closed rollout setting snapshot hash'
      closed_sidecar_hash="$(read_one_line "$run_dir/settings-closed.tsv.sha256")"
      valid_sha64 "$closed_sidecar_hash" || fail 'closed rollout setting sidecar hash is invalid'
      actual="$(hash_file "$run_dir/settings-closed.tsv")" || fail 'cannot hash closed rollout setting snapshot'
      [[ "$actual" == "$closed_hash" && "$actual" == "$closed_sidecar_hash" ]] || fail 'closed rollout setting snapshot hash mismatch'
      validate_closed_settings_snapshot "$run_dir/settings-closed.tsv"
      ;;
    switching|rolling_back)
      # The manifest records the closed-state intent before the first
      # destructive cutover operation.  An interrupted run may therefore be
      # in either of these states before the intent files are written; manual
      # rollback must remain available in that early window.
      if [[ -z "$closed_hash" ]]; then
        [[ ! -e "$run_dir/settings-closed.tsv" && ! -L "$run_dir/settings-closed.tsv" &&
           ! -e "$run_dir/settings-closed.tsv.sha256" && ! -L "$run_dir/settings-closed.tsv.sha256" ]] ||
          fail 'closed settings files exist without a manifest hash'
      else
        valid_sha64 "$closed_hash" || fail 'closed settings snapshot hash is invalid'
        assert_root_owned_regular "$run_dir/settings-closed.tsv" 'closed rollout setting snapshot'
        assert_root_owned_regular "$run_dir/settings-closed.tsv.sha256" 'closed rollout setting snapshot hash'
        closed_sidecar_hash="$(read_one_line "$run_dir/settings-closed.tsv.sha256")"
        valid_sha64 "$closed_sidecar_hash" || fail 'closed rollout setting sidecar hash is invalid'
        actual="$(hash_file "$run_dir/settings-closed.tsv")" || fail 'cannot hash closed rollout setting snapshot'
        [[ "$actual" == "$closed_hash" && "$actual" == "$closed_sidecar_hash" ]] || fail 'closed rollout setting snapshot hash mismatch'
        validate_closed_settings_snapshot "$run_dir/settings-closed.tsv"
      fi
      ;;
    switched|rolled_back)
      valid_sha64 "$closed_hash" || fail 'closed settings snapshot hash is missing or invalid'
      assert_root_owned_regular "$run_dir/settings-closed.tsv" 'closed rollout setting snapshot'
      assert_root_owned_regular "$run_dir/settings-closed.tsv.sha256" 'closed rollout setting snapshot hash'
      closed_sidecar_hash="$(read_one_line "$run_dir/settings-closed.tsv.sha256")"
      valid_sha64 "$closed_sidecar_hash" || fail 'closed rollout setting sidecar hash is invalid'
      actual="$(hash_file "$run_dir/settings-closed.tsv")" || fail 'cannot hash closed rollout setting snapshot'
      [[ "$actual" == "$closed_hash" && "$actual" == "$closed_sidecar_hash" ]] || fail 'closed rollout setting snapshot hash mismatch'
      validate_closed_settings_snapshot "$run_dir/settings-closed.tsv"
      ;;
    *) fail "unsupported cutover state: $state" ;;
  esac
  app_id="$(manifest_value live_app_id)"; valid_container_ref "$app_id" || fail 'manifest live app ID is invalid'
  app_name="$(manifest_value live_app_name)"; valid_container_ref "$app_name" || fail 'manifest live app name is invalid'
  live_image_id="$(manifest_value live_app_image_id)"
  [[ "$live_image_id" =~ ^sha256:[0-9a-f]{64}$ ]] || fail 'manifest live application image ID is invalid'
  environment_duplicate_mode="$(manifest_value environment_duplicate_mode)"
  environment_duplicate_legacy=0
  if ! manifest_has_key environment_duplicate_mode; then
    # A prepared run produced before the duplicate-environment evidence was
    # introduced remains rollback-compatible because its old validator already
    # rejected every duplicate key.  Treat it as an immutable strict contract.
    environment_duplicate_legacy=1
    environment_duplicate_mode=strict
    environment_duplicate_keys=''
    environment_duplicate_expected_hashes=''
    environment_file_sha256=''
    environment_duplicate_evidence_sha256=''
  else
    [[ -n "$environment_duplicate_mode" ]] || fail 'manifest environment duplicate mode is empty'
    [[ "$environment_duplicate_mode" == strict || "$environment_duplicate_mode" == last-wins ]] || fail 'manifest environment duplicate mode is invalid'
    environment_duplicate_keys="$(manifest_value environment_duplicate_keys)"
    environment_duplicate_expected_hashes="$(manifest_value environment_duplicate_expected_sha256)"
    validate_environment_duplicate_inputs "$environment_duplicate_keys" "$environment_duplicate_expected_hashes"
    environment_file_sha256="$(manifest_value environment_file_sha256)"
    environment_duplicate_evidence_sha256="$(manifest_value environment_duplicate_evidence_sha256)"
    valid_sha64 "$environment_file_sha256" || fail 'manifest environment metadata SHA is invalid'
    valid_sha64 "$environment_duplicate_evidence_sha256" || fail 'manifest environment duplicate evidence SHA is invalid'
  fi
  database_id="$(manifest_value database_id)"; redis_id="$(manifest_value redis_id)"
  valid_container_ref "$database_id" || fail 'manifest database ID is invalid'
  valid_container_ref "$redis_id" || fail 'manifest Redis ID is invalid'
  local_port="$(manifest_value local_port)"; valid_port "$local_port" || fail 'manifest local port is invalid'
  selected_host_ip="$(manifest_value host_ip)"; valid_host_ip "$selected_host_ip" || fail 'manifest host IP is invalid'
  public_url="$(manifest_value public_url)"
  [[ -z "$public_url" || "$public_url" =~ ^https?://[^@[:space:]]+$ ]] || fail 'manifest public URL is invalid'
  mapfile -t app_networks < "$run_dir/networks.txt"
  [[ "${#app_networks[@]}" -gt 0 ]] || fail 'prepared network list is empty'
  mapfile -t captured_ports < "$run_dir/ports.txt"
  mapfile -t captured_mounts < "$run_dir/mounts.txt"
  validate_runtime_metadata_files
  if [[ "$environment_duplicate_legacy" == 1 ]]; then
    environment_file_sha256="$(hash_file "$run_dir/container.env")" || fail 'cannot hash legacy environment metadata'
  else
    actual="$(hash_file "$run_dir/container.env")" || fail 'cannot hash normalized environment metadata'
    [[ "$actual" == "$environment_file_sha256" ]] || fail 'normalized environment metadata hash changed'
    actual="$(hash_file "$run_dir/environment-duplicates.tsv")" || fail 'cannot hash environment duplicate evidence'
    [[ "$actual" == "$environment_duplicate_evidence_sha256" ]] || fail 'environment duplicate evidence hash changed'
  fi
  expected="$(manifest_value log_config_sha256)"
  valid_sha64 "$expected" || fail 'manifest log configuration SHA is invalid'
  actual="$(hash_file "$run_dir/log-config.json")" || fail 'cannot hash log configuration metadata'
  [[ "$expected" == "$actual" ]] || fail 'log configuration metadata changed'
  expected="$(manifest_value runtime_contract_sha256)"
  valid_sha64 "$expected" || fail 'manifest runtime contract hash is invalid'
  [[ "$expected" == "$(read_one_line "$run_dir/runtime-contract.sha256")" ]] || fail 'runtime contract hash changed'
  assert_root_owned_regular "$run_dir/database.identity" 'PostgreSQL identity snapshot'
  assert_root_owned_regular "$run_dir/redis.identity" 'Redis identity snapshot'
  [[ "$(manifest_value docker_socket_fingerprint)" =~ ^[0-9]+\|[0-9]+\|0\|[0-9]+\|[0-7]{3,4}\|socket$ ]] || fail 'manifest Docker socket fingerprint is invalid'
  daemon_expected="$(manifest_value docker_daemon_identity)"
  [[ -n "$daemon_expected" ]] || fail 'manifest Docker daemon identity is missing'
  candidate_container_name="$(manifest_value candidate_container_name)"
  candidate_container_intent="$(manifest_value candidate_container_intent)"
  case "$state" in
    prepared)
      [[ -z "$candidate_container_name" && -z "$candidate_container_intent" ]] || fail 'prepared run unexpectedly contains candidate container intent' ;;
    switching|rolling_back|rolled_back)
      # Before create_candidate_container persists its intent, both values are
      # legitimately empty.  If either is present, require the complete pair
      # and bind it to the prepared application name so recovery never falls
      # back to a broad name/image search.
      if [[ -n "$candidate_container_name" || -n "$candidate_container_intent" ]]; then
        [[ "$candidate_container_name" == "$app_name" ]] || fail 'candidate container name does not match the live application name'
        valid_container_ref "$candidate_container_name" || fail 'candidate container name is invalid'
        valid_sha64 "$candidate_container_intent" || fail 'candidate container intent is invalid'
      fi
      ;;
    switched)
      [[ "$candidate_container_name" == "$app_name" ]] || fail 'candidate container name does not match the live application name'
      valid_container_ref "$candidate_container_name" || fail 'candidate container name is invalid'
      valid_sha64 "$candidate_container_intent" || fail 'candidate container intent is invalid' ;;
  esac
  candidate_id="$(manifest_value candidate_container_id)"
  local candidate_file="$run_dir/candidate-container-id" file_candidate_id=''
  if [[ -e "$candidate_file" || -L "$candidate_file" ]]; then
    assert_root_owned_regular "$candidate_file" 'candidate container ID file'
    file_candidate_id="$(read_one_line "$candidate_file")"
    valid_sha64 "$file_candidate_id" || fail 'candidate container ID file is invalid'
  fi
  if [[ -n "$candidate_id" ]]; then
    valid_sha64 "$candidate_id" || fail 'manifest candidate container ID is invalid'
    [[ -z "$file_candidate_id" || "$file_candidate_id" == "$candidate_id" ]] || fail 'candidate container manifest and ID file disagree'
  fi
  if [[ "$state" == prepared ]]; then
    [[ -z "$candidate_id" && -z "$file_candidate_id" ]] || fail 'prepared run unexpectedly contains a candidate container ID'
  elif [[ "$state" == switched ]]; then
    [[ -n "$candidate_id" && "$candidate_id" == "$file_candidate_id" ]] || fail 'switched run lacks a matching candidate container ID record'
  fi
}

assert_prepared_networks_still_match() {
  local network network_id current_network_id extra
  while IFS='|' read -r network network_id extra; do
    [[ -n "$network" && -n "$network_id" && -z "${extra:-}" ]] || fail 'prepared network identity metadata is malformed'
    current_network_id="$(docker_rpc network inspect --format '{{.Id}}' "$network")" || fail "cannot inspect prepared network: $network"
    [[ "$current_network_id" == "$network_id" ]] || fail "prepared network identity changed: $network"
  done < "$run_dir/network-identities.txt"
}

assert_runtime_still_matches_prepare() {
  local expected actual
  expected="$(manifest_value live_app_id)"
  expected="${expected#sha256:}"
  actual="$(docker_rpc inspect --format '{{.Id}}' "$app_name")" || fail 'cannot inspect live application by its prepared name'
  actual="${actual#sha256:}"
  [[ "$actual" == "$expected" ]] || fail 'live application identity no longer matches prepared run'
  [[ "$(docker_rpc inspect --format '{{.State.Running}}' "$expected")" == true ]] || fail 'live application is not running before cutover'
  [[ "$(docker_rpc inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}running{{end}}' "$expected")" == healthy || "$(docker_rpc inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}running{{end}}' "$expected")" == running ]] || fail 'live application is not healthy before cutover'
  assert_dependencies_still_match "$(manifest_value database_id)" "$(manifest_value redis_id)"
  assert_prepared_networks_still_match
  expected="$(read_one_line "$run_dir/live.identity")"
  actual="$(capture_container_identity "$(manifest_value live_app_id)")" || fail 'cannot recapture live application identity'
  [[ "$actual" == "$expected" ]] || fail 'live application runtime changed after prepare'
  expected="$(read_one_line "$run_dir/runtime-contract.sha256")"
  assert_environment_matches_prepare "$(manifest_value live_app_id)" live
  actual="$(capture_runtime_contract_hash "$(manifest_value live_app_id)")" || fail 'cannot recapture live runtime contract'
  [[ "$actual" == "$expected" ]] || fail 'live app runtime contract changed after prepare'
  assert_app_data_source_identity
}

candidate_restart_arg() {
  local policy retries
  policy="$(read_one_line "$run_dir/restart-policy.txt")"
  retries="$(read_one_line "$run_dir/restart-retries.txt")"
  case "$policy" in
    no|always|unless-stopped) printf '%s' "$policy" ;;
    on-failure)
      [[ "$retries" =~ ^[0-9]+$ ]] || fail 'restart retry count is invalid'
      printf 'on-failure:%s' "$retries"
      ;;
    *) fail "unsupported live restart policy: $policy" ;;
  esac
}

append_resource_args() {
  local -n output_args="$1"
  local file="$2" memory memory_swap memory_reservation nano_cpus cpu_shares cpu_quota cpu_period cpuset pids extra
  IFS='|' read -r memory memory_swap memory_reservation nano_cpus cpu_shares cpu_quota cpu_period cpuset pids extra < "$file"
  [[ -z "${extra:-}" ]] || fail 'resource policy has too many fields'
  [[ "$memory" =~ ^[0-9]+$ && "$memory_swap" =~ ^-?[0-9]+$ && "$memory_reservation" =~ ^[0-9]+$ &&
     "$nano_cpus" =~ ^[0-9]+$ && "$cpu_shares" =~ ^[0-9]+$ && "$cpu_quota" =~ ^-?[0-9]+$ &&
     "$cpu_period" =~ ^[0-9]+$ && "$cpuset" =~ ^[0-9,-]*$ && "$pids" =~ ^-?[0-9]+$ ]] ||
    fail 'resource policy contains an invalid limit'
  (( memory > 0 )) && output_args+=(--memory "$memory")
  [[ "$memory_swap" == 0 ]] || output_args+=(--memory-swap "$memory_swap")
  (( memory_reservation > 0 )) && output_args+=(--memory-reservation "$memory_reservation")
  (( nano_cpus > 0 )) && output_args+=(--nano-cpus "$nano_cpus")
  (( cpu_shares > 0 )) && output_args+=(--cpu-shares "$cpu_shares")
  [[ "$cpu_quota" == 0 ]] || output_args+=(--cpu-quota "$cpu_quota")
  (( cpu_period > 0 )) && output_args+=(--cpu-period "$cpu_period")
  [[ -z "$cpuset" ]] || output_args+=(--cpuset-cpus "$cpuset")
  [[ "$pids" == 0 ]] || output_args+=(--pids-limit "$pids")
}

append_log_config_args() {
  local -n output_args="$1"
  local encoded kind key value extra
  validate_log_config_file "$run_dir/log-config.json"
  encoded="$(python3 - "$run_dir/log-config.json" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    value = json.load(handle)
log_type = value.get("Type") or "json-file"
options = value.get("Config") or {}
print("driver|" + log_type)
for key in sorted(options):
    print("option|" + key + "|" + options[key])
PY
  )" || fail 'log configuration metadata cannot be converted to Docker arguments'
  while IFS='|' read -r kind key value extra; do
    [[ -z "${extra:-}" ]] || fail 'log configuration argument metadata has extra fields'
    case "$kind" in
      driver)
        [[ "$key" == json-file && -z "$value" ]] || fail 'log configuration driver metadata is invalid'
        output_args+=(--log-driver "$key")
        ;;
      option)
        [[ "$key" == max-file || "$key" == max-size ]] || fail 'log configuration option metadata is invalid'
        [[ -n "$value" ]] || fail 'log configuration option value is empty'
        output_args+=(--log-opt "$key=$value")
        ;;
      '') ;;
      *) fail 'log configuration argument metadata contains an unknown record' ;;
    esac
  done <<< "$encoded"
}

append_security_opt_args() {
  local -n output_args="$1"
  local option lower value
  while IFS= read -r option; do
    [[ -n "$option" ]] || continue
    lower="${option,,}"
    case "$lower" in
      privileged|privileged:*|privileged=*) fail 'privileged security options are not allowed' ;;
      seccomp=unconfined|seccomp:unconfined|apparmor=unconfined|apparmor:unconfined|\
      label=disable|label:disable|systempaths=unconfined|systempaths:unconfined)
        fail 'unsafe security option is not allowed' ;;
    esac
    if [[ "$lower" == no-new-privileges || "$lower" == no-new-privileges:* || "$lower" == no-new-privileges=* ]]; then
      if [[ "$lower" == no-new-privileges ]]; then
        value=true
      elif [[ "$lower" == *:* ]]; then
        value="${lower#*:}"
      else
        value="${lower#*=}"
      fi
      [[ "$value" == true || "$value" == 1 ]] || fail 'live security options explicitly disable no-new-privileges'
      continue
    fi
    [[ "${#option}" -le 1024 && "$option" != *$'\r'* ]] || fail 'security option metadata is invalid'
    output_args+=(--security-opt "$option")
  done < "$run_dir/security-opt.txt"
  output_args+=(--security-opt no-new-privileges:true)
}

append_ulimit_args() {
  local -n output_args="$1"
  local name soft hard extra
  while IFS='|' read -r name soft hard extra; do
    [[ -z "${name:-}" && -z "${soft:-}" && -z "${hard:-}" && -z "${extra:-}" ]] && continue
    [[ -n "$name" && -z "${extra:-}" && "$name" =~ ^[A-Za-z0-9_.-]{1,64}$ && "$soft" =~ ^-?[0-9]+$ && "$hard" =~ ^-?[0-9]+$ ]] ||
      fail 'ulimit metadata is invalid'
    output_args+=(--ulimit "$name=$soft:$hard")
  done < "$run_dir/ulimits.txt"
}

append_healthcheck_args() {
  local -n output_args="$1"
  local encoded key value extra mode='' command='' interval='' timeout='' start_period='' start_interval='' retries=''
  encoded="$(python3 - "$run_dir/healthcheck.json" <<'PY'
import base64
import json
import shlex
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    value = json.load(handle)
if value is None:
    print("none")
    raise SystemExit(0)
if not isinstance(value, dict):
    raise SystemExit(1)
test = value.get("Test") or []
if not isinstance(test, list) or not test:
    raise SystemExit(1)
if test[0] == "NONE":
    if len(test) != 1:
        raise SystemExit(1)
    print("none")
elif test[0] in ("CMD", "CMD-SHELL"):
    if len(test) < 2:
        raise SystemExit(1)
    if not all(isinstance(item, str) for item in test[1:]):
        raise SystemExit(1)
    if test[0] == "CMD-SHELL" and len(test) != 2:
        raise SystemExit(1)
    command = test[1] if test[0] == "CMD-SHELL" else shlex.join(test[1:])
    print("cmd|" + base64.b64encode(command.encode()).decode())
else:
    raise SystemExit(1)
for json_key in ("Interval", "Timeout", "StartPeriod", "StartInterval", "Retries"):
    raw = value.get(json_key)
    if raw is None:
        continue
    if not isinstance(raw, int) or raw < 0:
        raise SystemExit(1)
    print(json_key.lower() + "|" + str(raw) + "ns")
PY
  )" || fail 'healthcheck metadata cannot be parsed'
  while IFS='|' read -r key value extra; do
    [[ -z "${extra:-}" ]] || fail 'healthcheck metadata has extra fields'
    case "$key" in
      none) mode=none ;;
      cmd) mode=cmd; command="$(printf '%s' "$value" | base64 -d)" || fail 'healthcheck command cannot be decoded' ;;
      interval) interval="$value" ;;
      timeout) timeout="$value" ;;
      startperiod) start_period="$value" ;;
      startinterval) start_interval="$value" ;;
      retries) retries="${value%ns}" ;;
      '') ;;
      *) fail 'healthcheck metadata contains an unknown field' ;;
    esac
  done <<< "$encoded"
  if [[ "$mode" == none ]]; then
    output_args+=(--no-healthcheck)
  elif [[ "$mode" == cmd ]]; then
    output_args+=(--health-cmd "$command")
    [[ -z "$interval" ]] || output_args+=(--health-interval "$interval")
    [[ -z "$timeout" ]] || output_args+=(--health-timeout "$timeout")
    [[ -z "$start_period" ]] || output_args+=(--health-start-period "$start_period")
    [[ -z "$start_interval" ]] || output_args+=(--health-start-interval "$start_interval")
    [[ -z "$retries" ]] || output_args+=(--health-retries "$retries")
  else
    fail 'healthcheck metadata has no usable test'
  fi
}

append_network_alias_args() {
  local -n output_args="$1"
  local network alias extra
  while IFS='|' read -r network alias extra; do
    [[ -n "${network:-}" ]] || continue
    [[ -z "${extra:-}" ]] || fail 'network alias metadata has extra fields'
    [[ "$network" == "${app_networks[0]}" ]] && output_args+=(--network-alias "$alias")
  done < "$run_dir/network-aliases.txt"
  return 0
}

create_candidate_container() {
  local -a args=() line type source name destination writable propagation host_ip host_port container_port workdir
  local mount_spec candidate_file candidate_name candidate_intent
  local user
  candidate_name="$app_name"
  valid_container_ref "$candidate_name" || fail 'candidate container name is invalid'
  candidate_intent="$(hash_text "${tool_name}|$(manifest_value run_id)|$(manifest_value target_sha)|candidate")"
  valid_sha64 "$candidate_intent" || fail 'candidate container intent could not be generated'
  # Persist the name and intent before Docker create. If a later filesystem
  # write fails, rollback can discover only this exact labelled candidate.
  manifest_set candidate_container_name "$candidate_name"
  manifest_set candidate_container_intent "$candidate_intent"
  # `docker_rpc` supplies the `container create` subcommand.  Keep the image
  # as the first positional argument after all options; a second `create`
  # token would be interpreted by Docker as the image name `create:latest`.
  args=(--name "$candidate_name" --label "com.subnexus.cutover.tool=$tool_name" --label 'com.subnexus.cutover.role=candidate' --label "com.subnexus.cutover.run-id=$(manifest_value run_id)" --label "com.subnexus.cutover.target-sha=$(manifest_value target_sha)" --label "com.subnexus.cutover.intent=$candidate_intent" --pull never --restart "$(candidate_restart_arg)")
  append_security_opt_args args
  append_log_config_args args
  args+=(--env-file "$run_dir/container.env")
  user="$(read_one_line "$run_dir/user.txt")"
  [[ -z "$user" || "$user" =~ ^[A-Za-z0-9_.:-]{1,128}$ ]] || fail 'container user metadata is invalid'
  [[ -z "$user" ]] || args+=(--user "$user")
  append_resource_args args "$run_dir/resource-policy.txt"
  append_ulimit_args args
  append_healthcheck_args args
  append_network_alias_args args
  workdir="$(read_one_line "$run_dir/workdir.txt")"; [[ -z "$workdir" ]] || args+=(--workdir "$workdir")
  mapfile -t captured_entrypoint < "$run_dir/entrypoint.txt"
  local entrypoint_count=0
  for line in "${captured_entrypoint[@]}"; do [[ -n "$line" ]] && entrypoint_count=$((entrypoint_count + 1)); done
  (( entrypoint_count <= 1 )) || fail 'multi-element entrypoint cannot be reproduced safely'
  (( entrypoint_count == 1 )) && args+=(--entrypoint "${captured_entrypoint[0]}")
  for line in "${captured_ports[@]}"; do
    IFS='|' read -r container_port host_ip host_port <<< "$line"
    if [[ -z "$host_ip" ]]; then args+=(-p "$host_port:$container_port")
    else args+=(-p "$host_ip:$host_port:$container_port"); fi
  done
  while IFS='|' read -r type source name destination mode writable propagation; do
    [[ -n "$destination" ]] || continue
    if [[ "$type" == bind ]]; then
      mount_spec="type=bind,src=$source,dst=$destination"
      [[ "$writable" == true && "$mode" != ro ]] || mount_spec+=',readonly'
      [[ -z "$propagation" || "$propagation" == rprivate ]] || mount_spec+=",bind-propagation=$propagation"
      args+=(--mount "$mount_spec")
    else
      mount_spec="type=volume,src=$name,dst=$destination"
      [[ "$writable" == true && "$mode" != ro ]] || mount_spec+=',readonly'
      args+=(--mount "$mount_spec")
    fi
  done < "$run_dir/mounts.txt"
  [[ "${#app_networks[@]}" -gt 0 ]] || fail 'prepared network list is empty'
  args+=(--network "${app_networks[0]}")
  args+=("sha256:$expected_image_id")
  mapfile -t captured_cmd < "$run_dir/cmd.txt"
  for line in "${captured_cmd[@]}"; do args+=("$line"); done
  candidate_id="$(docker_rpc container create "${args[@]}")" || fail 'candidate container creation failed'
  valid_sha64 "${candidate_id#sha256:}" || fail 'candidate container ID is invalid'
  candidate_id="${candidate_id#sha256:}"
  candidate_file="$(mktemp "$run_dir/.candidate-container-id.XXXXXX")" || fail 'cannot create candidate identity temporary file'
  printf '%s\n' "$candidate_id" > "$candidate_file" || { rm -f -- "$candidate_file"; fail 'cannot write candidate identity'; }
  chmod 600 "$candidate_file"
  mv -f -- "$candidate_file" "$run_dir/candidate-container-id"
  chmod 600 "$run_dir/candidate-container-id"
  # Persist the exact ID before any network operation can fail, so automatic
  # rollback can remove only this container even if manifest_set later fails.
  manifest_set candidate_container_id "$candidate_id"
  local index network alias alias_network alias_extra
  local -a network_alias_args=()
  for (( index=1; index<${#app_networks[@]}; index++ )); do
    network="${app_networks[$index]}"
    network_alias_args=()
    while IFS='|' read -r alias_network alias alias_extra; do
      [[ -n "${alias_network:-}" ]] || continue
      [[ -z "${alias_extra:-}" ]] || fail 'network alias metadata has extra fields'
      [[ "$alias_network" == "$network" ]] || continue
      network_alias_args+=(--alias "$alias")
    done < "$run_dir/network-aliases.txt"
    docker_rpc network connect "${network_alias_args[@]}" "$network" "$candidate_id" >/dev/null ||
      fail "cannot connect candidate to network $network"
  done
}

assert_candidate_container_identity() {
  local ref="$1" expected_id expected_name expected_intent expected_image observed
  local actual_id actual_name actual_image actual_config_image actual_tool actual_role actual_run actual_sha actual_intent
  expected_id="$candidate_id"
  expected_name="$(manifest_value candidate_container_name)"
  expected_intent="$(manifest_value candidate_container_intent)"
  expected_image="sha256:$(manifest_value candidate_image_id)"
  valid_container_ref "$expected_name" || fail 'candidate container name is invalid'
  valid_sha64 "$expected_intent" || fail 'candidate container intent is invalid'
  [[ "$expected_image" =~ ^sha256:[0-9a-f]{64}$ ]] || fail 'candidate image identity is invalid'
  observed="$(docker_rpc inspect --format '{{.Id}}|{{.Name}}|{{.Image}}|{{.Config.Image}}|{{index .Config.Labels "com.subnexus.cutover.tool"}}|{{index .Config.Labels "com.subnexus.cutover.role"}}|{{index .Config.Labels "com.subnexus.cutover.run-id"}}|{{index .Config.Labels "com.subnexus.cutover.target-sha"}}|{{index .Config.Labels "com.subnexus.cutover.intent"}}' "$ref")" || fail 'cannot inspect candidate identity and labels'
  IFS='|' read -r actual_id actual_name actual_image actual_config_image actual_tool actual_role actual_run actual_sha actual_intent <<< "$observed"
  actual_id="${actual_id#sha256:}"
  actual_name="${actual_name#/}"
  [[ -z "$expected_id" || "$actual_id" == "$expected_id" ]] || fail 'candidate container ID does not match the prepared identity'
  [[ "$actual_name" == "$expected_name" && "$actual_image" == "$expected_image" && "$actual_config_image" == "$expected_image" &&
     "$actual_tool" == "$tool_name" &&
     "$actual_role" == candidate && "$actual_run" == "$(manifest_value run_id)" &&
     "$actual_sha" == "$(manifest_value target_sha)" && "$actual_intent" == "$expected_intent" ]] ||
    fail 'candidate container labels do not match the prepared intent'
}

wait_for_candidate_health() {
  local timeout_seconds="$candidate_health_timeout_seconds" stable="$candidate_stability_seconds" started now state restart_count docker_health healthy_since=0 stable_restart=''
  started="$SECONDS"
  while (( SECONDS - started < timeout_seconds )); do
    state="$(inspect_container_state_or_missing "$candidate_id")"
    case "$state" in
      running) ;;
      missing|created|paused|restarting|removing|exited|dead) return 1 ;;
      *) fail "unexpected candidate container state: $state" ;;
    esac
    restart_count="$(docker_rpc inspect --format '{{.RestartCount}}' "$candidate_id")" || fail 'cannot inspect candidate restart count'
    docker_health="$(docker_rpc inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}no-healthcheck{{end}}' "$candidate_id")" || fail 'cannot inspect candidate Docker health'
    [[ "$docker_health" =~ ^(healthy|starting|unhealthy|no-healthcheck)$ ]] || fail 'candidate Docker health status is invalid'
    if [[ "$state" == running && ( "$docker_health" == healthy || "$docker_health" == no-healthcheck ) ]] && curl --config /dev/null --noproxy '*' -fsS --max-time 3 "http://127.0.0.1:${local_port}/health" >/dev/null 2>&1; then
      now="$SECONDS"
      if [[ "$healthy_since" == 0 || "$restart_count" != "$stable_restart" ]]; then healthy_since="$now"; stable_restart="$restart_count"; fi
      if (( now - healthy_since >= stable )); then return 0; fi
    else
      healthy_since=0; stable_restart=''
    fi
    sleep 1
  done
  return 1
}

validate_candidate_runtime() {
  local uid no_new_privs effective_caps restart_count started_at health_code docker_health log_file
  uid="$(docker_rpc exec "$candidate_id" awk '/^Uid:/{print $2}' /proc/1/status 2>/dev/null || true)"
  no_new_privs="$(docker_rpc exec "$candidate_id" awk '/^NoNewPrivs:/{print $2}' /proc/1/status 2>/dev/null || true)"
  effective_caps="$(docker_rpc exec "$candidate_id" awk '/^CapEff:/{print $2}' /proc/1/status 2>/dev/null || true)"
  [[ "$uid" =~ ^[0-9]+$ && "$uid" != 0 ]] || fail 'candidate PID 1 must run as a non-root UID'
  [[ "$no_new_privs" == 1 ]] || fail 'candidate PID 1 must have NoNewPrivs=1'
  [[ "$effective_caps" =~ ^0+$ ]] || fail 'candidate PID 1 must have no effective capabilities'
  restart_count="$(docker_rpc inspect --format '{{.RestartCount}}' "$candidate_id")"
  [[ "$restart_count" == 0 ]] || fail 'candidate restarted during its initial stability window'
  docker_health="$(docker_rpc inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}no-healthcheck{{end}}' "$candidate_id")" || fail 'cannot inspect final candidate Docker health'
  [[ "$docker_health" == healthy || "$docker_health" == no-healthcheck ]] || fail "candidate Docker health is not ready: $docker_health"
  started_at="$(docker_rpc inspect --format '{{.State.StartedAt}}' "$candidate_id")"
  [[ -n "$started_at" ]] || fail 'candidate start timestamp is missing'
  log_file="$run_dir/candidate-startup.log"
  docker_rpc logs --since "$started_at" "$candidate_id" > "$log_file" 2>&1 || fail 'candidate startup log could not be read'
  if grep -Eiq 'panic|fatal|migration.*(failed|error)|failed.*database|redis.*fail|server.*fail' "$log_file"; then
    fail 'candidate startup log contains a severe error marker'
  fi
  health_code="$(curl --config /dev/null --noproxy '*' -sS -o /dev/null -w '%{http_code}' --max-time 5 "http://127.0.0.1:${local_port}/health" || true)"
  [[ "$health_code" =~ ^2[0-9][0-9]$ ]] || fail "candidate local health returned HTTP ${health_code:-none}"
  if [[ -n "$public_url" ]]; then
    health_code="$(curl --config /dev/null --noproxy '*' -sS -o /dev/null -w '%{http_code}' --max-time 8 "$public_url" || true)"
    [[ "$health_code" =~ ^2[0-9][0-9]$ ]] || fail "candidate public health returned HTTP ${health_code:-none}"
  fi
  assert_candidate_runtime_contract
  verify_rollout_gates_closed
}

assert_candidate_network_identities() {
  local expected_names actual_names network network_id extra actual_id
  local -A expected_name_seen=()
  expected_names="$(awk -F'|' 'NF != 2 || $1 == "" || $2 == "" { exit 1 } { print $1 }' "$run_dir/network-identities.txt" | LC_ALL=C sort)" ||
    fail 'prepared network identity metadata is malformed'
  [[ -n "$expected_names" ]] || fail 'prepared network identity metadata is empty'
  while IFS= read -r network; do
    [[ "$network" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]{0,254}$ ]] || fail 'prepared network identity name is invalid'
    case "$network" in
      host|none) fail "prepared network identity uses an unsupported special network: $network" ;;
    esac
    [[ "${expected_name_seen[$network]:-0}" -eq 0 ]] || fail 'prepared network identity contains a duplicate name'
    expected_name_seen["$network"]=1
  done <<< "$expected_names"
  actual_names="$(docker_rpc inspect --format '{{range $name, $network := .NetworkSettings.Networks}}{{println $name}}{{end}}' "$candidate_id" | sed '/^[[:space:]]*$/d' | LC_ALL=C sort)" ||
    fail 'cannot inspect candidate network names'
  [[ -n "$actual_names" ]] || fail 'candidate has no Docker networks'
  while IFS= read -r network; do
    [[ "$network" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]{0,254}$ ]] || fail 'candidate network name is invalid'
    case "$network" in
      host|none) fail "candidate uses an unsupported special network: $network" ;;
    esac
  done <<< "$actual_names"
  [[ "$actual_names" == "$expected_names" ]] || fail 'candidate network names do not match the prepared live container'
  while IFS='|' read -r network network_id extra; do
    [[ -n "$network" && -n "$network_id" && -z "${extra:-}" ]] || fail 'prepared network identity metadata is malformed'
    actual_id="$(docker_rpc network inspect --format '{{.Id}}' "$network")" || fail "cannot inspect candidate network: $network"
    valid_container_ref "$actual_id" || fail "candidate network ID is invalid: $network"
    [[ "$actual_id" == "$network_id" ]] || fail "candidate network identity changed: $network"
  done < "$run_dir/network-identities.txt"
}

assert_candidate_runtime_contract() {
  local expected_user actual_user expected_resources actual_resources expected_restart actual_restart expected actual
  expected_user="$(read_one_line "$run_dir/user.txt")"
  actual_user="$(docker_rpc inspect --format '{{.Config.User}}' "$candidate_id")" || fail 'cannot inspect candidate Config.User'
  [[ "$actual_user" == "$expected_user" ]] || fail 'candidate Config.User does not match the live container'
  expected_resources="$(read_one_line "$run_dir/resource-policy.txt")"
  actual_resources="$(docker_rpc inspect --format '{{.HostConfig.Memory}}|{{.HostConfig.MemorySwap}}|{{.HostConfig.MemoryReservation}}|{{.HostConfig.NanoCpus}}|{{.HostConfig.CpuShares}}|{{.HostConfig.CpuQuota}}|{{.HostConfig.CpuPeriod}}|{{.HostConfig.CpusetCpus}}|{{if .HostConfig.PidsLimit}}{{.HostConfig.PidsLimit}}{{else}}0{{end}}' "$candidate_id")" || fail 'cannot inspect candidate resource policy'
  [[ "$actual_resources" == "$expected_resources" ]] || fail 'candidate resource limits do not match the live container'
  expected_restart="$(read_one_line "$run_dir/restart-policy.txt")|$(read_one_line "$run_dir/restart-retries.txt")"
  actual_restart="$(docker_rpc inspect --format '{{.HostConfig.RestartPolicy.Name}}|{{.HostConfig.RestartPolicy.MaximumRetryCount}}' "$candidate_id")" || fail 'cannot inspect candidate restart policy'
  [[ "$actual_restart" == "$expected_restart" ]] || fail 'candidate restart policy does not match the live container'
  assert_candidate_network_identities
  expected="$(read_one_line "$run_dir/runtime-contract.sha256")"
  assert_environment_matches_prepare "$candidate_id" candidate
  actual="$(capture_runtime_contract_hash "$candidate_id")" || fail 'cannot inspect candidate runtime contract'
  [[ "$actual" == "$expected" ]] || fail 'candidate runtime contract differs from the prepared live container'
}

assert_preserved_container_contract() {
  local old_id expected actual image_id name actual_id actual_image
  old_id="$(manifest_value live_app_id)"
  old_id="${old_id#sha256:}"
  valid_container_ref "$old_id" || fail 'preserved old container ID is invalid'
  image_id="$(manifest_value live_app_image_id)"
  [[ "$image_id" =~ ^sha256:[0-9a-f]{64}$ ]] || fail 'preserved old image ID is invalid'
  actual="$(docker_rpc inspect --format '{{.Id}}|{{.Image}}|{{.Name}}' "$old_id")" || fail 'cannot inspect preserved old container contract'
  IFS='|' read -r actual_id actual_image name <<< "$actual"
  actual_id="${actual_id#sha256:}"
  name="${name#/}"
  [[ "$actual_id" == "$old_id" && "$actual_image" == "$image_id" ]] || fail 'preserved old container image or identity changed'
  [[ "$name" == "$app_name" || "$name" == "$preserved_name" ]] || fail 'preserved old container has an unexpected name'
  expected="$(read_one_line "$run_dir/runtime-contract.sha256")"
  assert_environment_matches_prepare "$old_id" live
  actual="$(capture_runtime_contract_hash "$old_id")" || fail 'cannot inspect preserved old runtime contract'
  [[ "$actual" == "$expected" ]] || fail 'preserved old container runtime contract changed'
}

restore_preserved_container() {
  local old_id current_id current_name_id observed state docker_health health_code started
  old_id="$(manifest_value live_app_id)"
  old_id="${old_id#sha256:}"
  valid_container_ref "$old_id" || fail 'preserved old container ID is invalid'
  valid_container_ref "$app_name" || fail 'original application container name is invalid'
  valid_container_ref "$preserved_name" || fail 'preserved container name is invalid'
  assert_app_data_source_identity
  current_id=""
  if [[ -n "$preserved_name" ]]; then
    current_id="$(inspect_container_id_or_empty "$preserved_name")" || fail 'cannot inspect preserved old container'
  fi
  if [[ -n "$preserved_name" && "$current_id" == "$old_id" ]]; then
    current_name_id="$(inspect_container_id_or_empty "$app_name")" || fail 'cannot inspect application name before rollback rename'
    [[ -z "$current_name_id" || "$current_name_id" == "$old_id" ]] ||
      fail 'application name is occupied by a different container during rollback'
    assert_preserved_container_contract
    if [[ "$(docker_rpc inspect --format '{{.State.Running}}' "$preserved_name")" == true ]]; then
      assert_daemon_still_matches_prepare
      docker_rpc stop --time "$stop_timeout_seconds" "$preserved_name" >/dev/null || fail 'cannot stop preserved old container before rename'
      [[ "$(docker_rpc inspect --format '{{.State.Running}}' "$preserved_name")" == false ]] || fail 'preserved old container is still running'
    fi
    assert_daemon_still_matches_prepare
    docker_rpc rename "$preserved_name" "$app_name" || fail 'cannot restore old container name'
    observed="$(inspect_container_id_or_empty "$app_name")" || fail 'cannot verify restored old container name'
    [[ "$observed" == "$old_id" ]] || fail 'restored old container name has the wrong identity'
    current_id="$old_id"
  else
    current_id="$(inspect_container_id_or_empty "$app_name")" || fail 'cannot inspect application container during rollback'
  fi
  if [[ "$current_id" == "$old_id" ]]; then
    assert_preserved_container_contract
    if [[ "$(docker_rpc inspect --format '{{.State.Running}}' "$app_name")" != true ]]; then
      assert_daemon_still_matches_prepare
      docker_rpc start "$app_name" >/dev/null || fail 'cannot start old application container'
    fi
    observed="$(inspect_container_id_or_empty "$app_name")" || fail 'cannot verify started old container identity'
    [[ "$observed" == "$old_id" ]] || fail 'started old container has the wrong identity'
  else
    fail 'preserved old application container is unavailable or has the wrong identity'
  fi
  started="$SECONDS"
  while (( SECONDS - started < rollback_health_timeout_seconds )); do
    observed="$(inspect_container_id_or_empty "$app_name")" || fail 'cannot inspect old application identity during rollback health check'
    [[ "$observed" == "$old_id" ]] || fail 'application name changed identity during rollback health check'
    state="$(inspect_container_state_or_missing "$app_name")"
    docker_health="$(docker_rpc inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}no-healthcheck{{end}}' "$app_name")" || fail 'cannot inspect restored application Docker health'
    [[ "$docker_health" =~ ^(healthy|starting|unhealthy|no-healthcheck)$ ]] || fail 'restored application Docker health status is invalid'
    if [[ "$state" == running && ( "$docker_health" == healthy || "$docker_health" == no-healthcheck ) ]] && curl --config /dev/null --noproxy '*' -fsS --max-time 3 "http://127.0.0.1:${local_port}/health" >/dev/null 2>&1; then
      if [[ -z "$public_url" ]]; then return 0; fi
      health_code="$(curl --config /dev/null --noproxy '*' -sS -o /dev/null -w '%{http_code}' --max-time 8 "$public_url" || true)"
      [[ "$health_code" =~ ^2[0-9][0-9]$ ]] && return 0
    fi
    case "$state" in
      running) ;;
      missing|created|paused|restarting|removing|exited|dead) return 1 ;;
      *) fail "unexpected old application state during rollback health check: $state" ;;
    esac
    sleep 1
  done
  return 1
}

remove_exact_candidate() {
  local observed live_id candidate_file="$run_dir/candidate-container-id" candidate_name candidate_intent
  local file_candidate_id=''
  [[ -n "$candidate_id" ]] || candidate_id="$(manifest_value candidate_container_id)"
  candidate_name="$(manifest_value candidate_container_name)"
  candidate_intent="$(manifest_value candidate_container_intent)"
  if [[ -n "$candidate_name" || -n "$candidate_intent" ]]; then
    valid_container_ref "$candidate_name" || fail 'candidate container name is invalid; refusing discovery'
    valid_sha64 "$candidate_intent" || fail 'candidate container intent is invalid; refusing discovery'
  fi
  if [[ -e "$candidate_file" || -L "$candidate_file" ]]; then
    assert_root_owned_regular "$candidate_file" 'candidate container ID file'
    file_candidate_id="$(read_one_line "$candidate_file")"
    valid_sha64 "$file_candidate_id" || fail 'candidate container ID file is invalid; refusing removal'
    if [[ -n "$candidate_id" ]]; then
      [[ "$candidate_id" == "$file_candidate_id" ]] || fail 'candidate container manifest and ID file disagree'
    else
      candidate_id="$file_candidate_id"
    fi
  fi
  if [[ -n "$candidate_id" ]]; then
    valid_sha64 "$candidate_id" || fail 'candidate container ID is invalid; refusing removal'
    observed="$(inspect_container_id_or_empty "$candidate_id")" || fail 'cannot inspect candidate identity before removal'
    [[ -z "$observed" || "$observed" == "$candidate_id" ]] || fail 'candidate identity changed; refusing removal'
  else
    observed=''
  fi
  # The ID file can outlive a failed/manual container removal.  If its exact
  # object is gone, fall back only to the manifest-bound name+intent pair so a
  # recreated candidate cannot be left running under the production name.
  if [[ -z "$observed" && -n "$candidate_name" ]]; then
    observed="$(inspect_container_id_or_empty "$candidate_name")" || fail 'cannot inspect candidate name during recovery'
    if [[ -n "$observed" ]]; then
      live_id="$(manifest_value live_app_id)"
      live_id="${live_id#sha256:}"
      valid_sha64 "$live_id" || fail 'prepared live application ID is invalid during candidate discovery'
      # After a successful rollback the restored old container owns the
      # production name. A stale candidate ID file must never make a repeated
      # rollback identify that old container as the candidate.
      if [[ "$observed" == "$live_id" ]]; then
        candidate_id=''
        return 0
      fi
      candidate_id="$observed"
    fi
  fi
  [[ -n "$observed" ]] || return 0
  valid_sha64 "$candidate_id" || fail 'discovered candidate container ID is invalid; refusing removal'
  assert_candidate_container_identity "$candidate_id"
  assert_daemon_still_matches_prepare
  docker_rpc container rm --force "$candidate_id" >/dev/null || fail 'cannot remove exact failed candidate container'
}

rollback_run() {
  local internal="${1:-0}" state candidate_observed closed_hash candidate_file file_candidate_id
  state="$(manifest_value state)"
  [[ "$state" == switching || "$state" == switched || "$state" == rolling_back || "$state" == rolled_back ]] || fail "rollback is not applicable in state $state"
  rollback_active=1
  validate_app_data_owner_manifest
  # Automatic rollback can be entered from an ERR trap immediately after a
  # failed Docker RPC.  Revalidate the endpoint before issuing any further
  # stop/remove/rename/start operation; a changed daemon must fail closed.
  assert_daemon_still_matches_prepare
  assert_app_data_source_identity
  manifest_set state rolling_back
  candidate_id="$(manifest_value candidate_container_id)"
  candidate_file="$run_dir/candidate-container-id"
  if [[ -e "$candidate_file" || -L "$candidate_file" ]]; then
    assert_root_owned_regular "$candidate_file" 'candidate container ID file'
    file_candidate_id="$(read_one_line "$candidate_file")"
    valid_sha64 "$file_candidate_id" || fail 'candidate container ID file is invalid'
    if [[ -n "$candidate_id" ]]; then
      [[ "$candidate_id" == "$file_candidate_id" ]] || fail 'candidate container manifest and ID file disagree'
    else
      candidate_id="$file_candidate_id"
    fi
  fi
  preserved_name="$(manifest_value preserved_container)"
  if [[ -z "$preserved_name" ]]; then
    app_name="$(manifest_value live_app_name)"
    target_sha="$(manifest_value target_sha)"
    preserved_name="${app_name}-pre-${target_sha:0:12}-$(manifest_value run_id)"
  fi
  if [[ -n "$candidate_id" || -n "$(manifest_value candidate_container_name)" ]]; then
    assert_daemon_still_matches_prepare
    candidate_observed=''
    if [[ -n "$candidate_id" ]]; then
      candidate_observed="$(inspect_container_id_or_empty "$candidate_id")" || fail 'cannot inspect candidate before rollback'
      if [[ -n "$candidate_observed" ]]; then
        if [[ "$(docker_rpc inspect --format '{{.State.Running}}' "$candidate_id")" == true ]]; then docker_rpc stop --time "$stop_timeout_seconds" "$candidate_id" >/dev/null || true; fi
      fi
    fi
    remove_exact_candidate
  fi
  # Restore the old application and prove its health before touching the
  # settings snapshot.  If settings restoration fails, the old service stays
  # available with the rollout gates closed and the operator can retry; doing
  # SQL first could leave the service unavailable after a partial rollback.
  restore_preserved_container
  # Never write settings into a dependency container that may have been
  # replaced while the candidate was running.  Recheck immediately before SQL.
  assert_daemon_still_matches_prepare
  assert_dependencies_still_match "$(manifest_value database_id)" "$(manifest_value redis_id)"
  closed_hash="$(manifest_value settings_closed_snapshot_sha256)"
  if [[ -n "$closed_hash" ]]; then
    # The manifest is written before the destructive window, so an interrupted
    # run can legitimately be in `switching` while the database is still at
    # the original snapshot.  Probe both immutable states before issuing any
    # restore SQL; writing the historical snapshot over an operator change is
    # never an acceptable recovery action.
    if settings_snapshot_matches_file "$run_dir/settings-closed.tsv"; then
      restore_rollout_gates
      verify_rollout_gates_restored
    elif settings_snapshot_matches_file "$run_dir/settings-before.tsv"; then
      log 'Rollout settings already match the prepared snapshot; no restore SQL was issued.'
      verify_rollout_gates_restored
    else
      fail 'current rollout settings match neither the closed nor prepared snapshot; refusing rollback restore'
    fi
  else
    # Compatibility path for an interrupted legacy run created before the
    # closed-state intent was introduced.  Prove the database is still at the
    # prepare snapshot and leave it untouched.
    assert_settings_snapshot_matches_file "$run_dir/settings-before.tsv"
  fi
  manifest_set rolled_back_at "$(date --iso-8601=seconds)"
  write_run_marker ROLLED_BACK rolled_back
  manifest_set state rolled_back
  rollback_active=0
  log "ROLLBACK_COMPLETED=$run_dir"
}

on_error() {
  local rc=$?
  set +e
  rollback_after_failure "$rc"
  exit "$rc"
}

switch_run() {
  [[ "$#" -eq 1 ]] || { usage; exit 2; }
  [[ "${SUBNEXUS_CUTOVER_CONFIRM:-}" == "$cutover_approval_token" ]] || fail 'switch requires SUBNEXUS_CUTOVER_CONFIRM=I_UNDERSTAND_SHORT_PRODUCTION_WINDOW'
  [[ "$EUID" -eq 0 ]] || fail 'switch must run as root'
  require_commands
  validate_run_directory "$1" switch
  local state="$(manifest_value state)" closed_hash closed_sidecar_hash actual_closed_hash
  [[ "$state" == prepared ]] || fail "switch requires a prepared run; current state is $state"
  acquire_lock "$evidence_lock_root"
  init_docker
  assert_daemon_still_matches_prepare
  trap 'on_error' ERR
  validate_app_data_owner_manifest
  app_name="$(manifest_value live_app_name)"
  app_id="$(manifest_value live_app_id)"
  database_id="$(manifest_value database_id)"
  redis_id="$(manifest_value redis_id)"
  local_port="$(manifest_value local_port)"
  selected_host_ip="$(manifest_value host_ip)"
  target_sha="$(manifest_value target_sha)"
  expected_image_id="$(manifest_value candidate_image_id)"
  public_url="$(manifest_value public_url)"
  assert_runtime_still_matches_prepare
  [[ "$(docker_rpc image inspect --format '{{.Id}}' "sha256:$expected_image_id")" == "sha256:$expected_image_id" ]] || fail 'approved candidate image is not loaded on the production host'
  assert_dependencies_still_match "$database_id" "$redis_id"
  # Detect settings drift before the outage starts.  The transactional closure
  # repeats this comparison under a table lock to close the remaining race.
  assert_settings_snapshot_matches_file "$run_dir/settings-before.tsv"
  # Require an explicit human check for settlement/migration workers; the
  # tool cannot infer business-level quiescence from Docker state.
  [[ "${SUBNEXUS_CUTOVER_QUIET_CONFIRM:-}" == 'I_HAVE_CHECKED_NO_SETTLEMENT_TASKS' ]] || fail 'set SUBNEXUS_CUTOVER_QUIET_CONFIRM after checking scheduled settlement/migration workers'
  assert_daemon_still_matches_prepare
  preserved_name="${app_name}-pre-${target_sha:0:12}-$(manifest_value run_id)"
  valid_container_ref "$preserved_name" || fail 'generated preserved container name is invalid'
  [[ -z "$(inspect_container_id_or_empty "$preserved_name")" ]] || fail 'preserved container name already exists'
  manifest_set state switching
  cutover_active=1
  cutover_started_at="$SECONDS"
  docker_rpc stop --time "$stop_timeout_seconds" "$app_id" >/dev/null || fail 'old application did not stop within the cutover budget'
  assert_daemon_still_matches_prepare
  [[ "$(docker_rpc inspect --format '{{.State.Running}}' "$app_id")" == false ]] || fail 'old application is still running after stop'
  docker_rpc rename "$app_id" "$preserved_name" || fail 'cannot preserve old application container'
  assert_daemon_still_matches_prepare
  manifest_set preserved_container "$preserved_name"
  assert_app_data_source_identity
  assert_dependencies_still_match "$database_id" "$redis_id"
  close_rollout_gates
  # The closed snapshot was created during prepare.  Recheck its immutable
  # files and prove the transactional gate update landed exactly as intended.
  closed_hash="$(manifest_value settings_closed_snapshot_sha256)"
  valid_sha64 "$closed_hash" || fail 'closed settings snapshot hash is missing or invalid'
  assert_root_owned_regular "$run_dir/settings-closed.tsv" 'closed rollout setting snapshot'
  assert_root_owned_regular "$run_dir/settings-closed.tsv.sha256" 'closed rollout setting snapshot hash'
  closed_sidecar_hash="$(read_one_line "$run_dir/settings-closed.tsv.sha256")"
  valid_sha64 "$closed_sidecar_hash" || fail 'closed rollout setting sidecar hash is invalid'
  actual_closed_hash="$(hash_file "$run_dir/settings-closed.tsv")" || fail 'cannot hash closed rollout setting snapshot'
  [[ "$actual_closed_hash" == "$closed_hash" && "$actual_closed_hash" == "$closed_sidecar_hash" ]] || fail 'closed rollout setting snapshot changed'
  validate_closed_settings_snapshot "$run_dir/settings-closed.tsv"
  assert_settings_snapshot_matches_file "$run_dir/settings-closed.tsv"
  assert_dependencies_still_match "$database_id" "$redis_id"
  verify_rollout_gates_closed
  assert_prepared_networks_still_match
  assert_daemon_still_matches_prepare
  create_candidate_container
  assert_daemon_still_matches_prepare
  manifest_set candidate_container_id "$candidate_id"
  assert_candidate_container_identity "$candidate_id"
  # Prove Docker reproduced the prepared runtime metadata before executing
  # any candidate process (including an image entrypoint migration).
  assert_candidate_runtime_contract
  docker_rpc start "$candidate_id" >/dev/null || fail 'candidate application failed to start'
  assert_daemon_still_matches_prepare
  wait_for_candidate_health || fail 'candidate did not become healthy within the bounded window'
  validate_candidate_runtime
  assert_app_data_source_identity
  manifest_set switched_at "$(date --iso-8601=seconds)"
  manifest_set outage_seconds "$((SECONDS - cutover_started_at))"
  write_run_marker SWITCHED switched
  manifest_set state switched
  cutover_active=0
  trap - ERR
  log "SWITCH_COMPLETED=$run_dir"
  log "Old container preserved as $preserved_name; database backup was not restored."
}

rollback_entry() {
  [[ "$#" -eq 1 ]] || { usage; exit 2; }
  [[ "${SUBNEXUS_CUTOVER_CONFIRM:-}" == "$rollback_approval_token" ]] || fail 'rollback requires SUBNEXUS_CUTOVER_CONFIRM=I_UNDERSTAND_APPLICATION_ROLLBACK'
  [[ "$EUID" -eq 0 ]] || fail 'rollback must run as root'
  require_commands
  validate_run_directory "$1" rollback
  acquire_lock "$evidence_lock_root"
  init_docker
  assert_daemon_still_matches_prepare
  trap 'on_error' ERR
  rollback_run 0
  trap - ERR
}

main() {
  mode="${1:-}"
  shift || true
  case "$mode" in
    prepare) prepare_run "$@" ;;
    switch) switch_run "$@" ;;
    rollback) rollback_entry "$@" ;;
    -h|--help|help) usage; exit 0 ;;
    *) usage; exit 2 ;;
  esac
}

main "$@"
