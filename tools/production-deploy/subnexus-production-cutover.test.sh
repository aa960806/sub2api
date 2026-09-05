#!/usr/bin/env bash
set -Eeuo pipefail

# Static contracts and local fault fixtures for the production cutover
# controller. The subject is never sourced: doing so would execute its
# production entrypoint. Fixtures extract individual helpers and replace
# Docker/database calls with local shell functions.

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
subject="$script_dir/subnexus-production-cutover.sh"

fail() {
  printf 'TEST ERROR: %s\n' "$*" >&2
  exit 1
}

assert_contains() {
  local needle="$1" file="${2:-$subject}"
  grep -Fq -- "$needle" "$file" || fail "missing invariant: $needle"
}

assert_not_contains() {
  local needle="$1" file="${2:-$subject}"
  if grep -Fq -- "$needle" "$file"; then
    fail "forbidden pattern present: $needle"
  fi
}

extract_function() {
  local function_name="$1"
  awk -v wanted="$function_name" '
    $0 == wanted "() {" { capture = 1 }
    capture { print }
    capture && $0 == "}" { exit }
  ' "$subject"
}

assert_before_text() {
  local text="$1" first="$2" second="$3" first_line second_line
  first_line="$(printf '%s\n' "$text" | grep -n -F -- "$first" | head -n 1 | cut -d: -f1 || true)"
  second_line="$(printf '%s\n' "$text" | grep -n -F -- "$second" | head -n 1 | cut -d: -f1 || true)"
  [[ "$first_line" =~ ^[0-9]+$ && "$second_line" =~ ^[0-9]+$ ]] ||
    fail "ordering markers are missing: '$first' / '$second'"
  (( first_line < second_line )) || fail "'$first' must precede '$second'"
}

[[ -f "$subject" ]] || fail 'production cutover script is missing'
bash -n "$subject" || fail 'production cutover script has a shell syntax error'
if grep -n $'\r' "$subject" >/dev/null; then
  fail 'production cutover script must use LF line endings'
fi

# ---------------------------------------------------------------------------
# Static safety and phase contracts
# ---------------------------------------------------------------------------

for function_name in init_docker acquire_lock prepare_argument_count_is_valid prepare_run validate_run_directory \
  validate_environment_file validate_settings_snapshot capture_settings_snapshot close_rollout_gates \
  restore_rollout_gates restore_preserved_container rollback_run switch_run \
  rollback_entry write_run_marker assert_run_marker initialize_prepare_backup_budgets \
  assert_prepare_disk_budget assert_backup_within_budget validate_log_config_file append_log_config_args \
  write_application_data_archive_policy validate_application_data_archive_policy ensure_image_load_log \
  assert_candidate_network_identities; do
  assert_contains "$function_name() {"
done

for marker in \
  'for override in DOCKER_HOST DOCKER_CONTEXT DOCKER_CONFIG DOCKER_TLS_VERIFY DOCKER_CERT_PATH DOCKER_API_VERSION' \
  'validate_docker_timeout' \
  'unset BASH_ENV ENV CDPATH GLOBIGNORE TAR_OPTIONS GZIP' \
  'docker_rpc context show' \
  'docker_rpc context inspect' \
  'default Docker endpoint must be the local system Docker socket' \
  'docker_socket_fingerprint' \
  'Docker daemon must provide seccomp isolation' \
  'assert_root_owned_path_chain' \
  '[[ -d "$path" && ! -L "$path" ]]' \
  'exec 9<"$root"' \
  '/proc/${BASHPID:-$$}/fd/9' \
  '[[ "$path_stat" == "$fd_stat|directory" ]]' \
  'settings-before.tsv.sha256' \
  'settings_closed_snapshot_sha256' \
  "translate(encode(convert_to(value, 'UTF8'), 'base64')" \
  'BEGIN ISOLATION LEVEL SERIALIZABLE' \
  'LOCK TABLE settings IN SHARE ROW EXCLUSIVE MODE' \
  'rollout setting CAS mismatch' \
  'rollout_content_keys' \
  'channel_monitor_mode' \
  'expected 14 closed boolean rollout gates' \
  'candidate-container-id.XXXXXX' \
  'candidate_archive_expanded_size' \
  'final_free_reserve_bytes=8589934592' \
  'ulimit -f "$file_limit_kib"' \
  'candidate_container_intent' \
  'com.subnexus.cutover.intent' \
  'assert_dependencies_still_match' \
  'validate_environment_file' \
  'healthcheck.json' \
  'ulimits.txt' \
  'network-aliases.txt' \
  'log-config.json' \
  'runtime-contract.sha256' \
  'environment_duplicate_approval_token' \
  'SUBNEXUS_CUTOVER_ENV_DUPLICATE_CONFIRM' \
  'SUBNEXUS_CUTOVER_ENV_DUPLICATE_KEYS' \
  'SUBNEXUS_CUTOVER_ENV_DUPLICATE_EXPECTED_SHA256' \
  'environment-duplicates.tsv' \
  'capture_environment_metadata' \
  'assert_environment_matches_prepare' \
  'capture_runtime_contract_hash' \
  'duplicate runtime environment entry' \
  'replay-candidate' \
  'environment_observed_mode' \
  'manifest_has_key' \
  'runtime_env_mode != "last-wins"' \
  'app_data_owner_approval_token' \
  'SUBNEXUS_CUTOVER_APP_DATA_OWNER_CONFIRM' \
  'SUBNEXUS_CUTOVER_APP_DATA_OWNER_UID' \
  'SUBNEXUS_CUTOVER_APP_DATA_OWNER_GID' \
  'app_data_owner_policy' \
  'app_data_owner_mode' \
  'app_data_owner_mode_is_safe' \
  'validate_app_data_owner_inputs' \
  'validate_app_data_owner_manifest' \
  'validate_app_data_owner_runtime_inputs' \
  'database backup was not restored'; do
  assert_contains "$marker"
done
assert_contains 'raw_binds = host.get("Binds") or []'
assert_contains 'HostConfig.Binds.unmatched'
assert_contains 'contract_bind_mounts = []'
assert_contains 'contract["HostConfig"]["Binds"] = contract_bind_mounts'
assert_contains 'console_size = host.get("ConsoleSize")'
assert_contains 'contract["HostConfig"]["ConsoleSize"] = [0, 0]'
assert_contains 'if contract["HostConfig"].get("OomKillDisable") is None:'
assert_contains 'contract["HostConfig"]["OomKillDisable"] = False'
assert_contains 'allowed_log_options = {"max-file", "max-size"}'
assert_contains 'HostConfig.LogConfig.Config.required'
assert_contains 'log_config_sha256=%s'
assert_contains 'args+=(--log-driver "$key")'
assert_contains 'args+=(--log-opt "$key=$value")'
assert_contains 'app_data_archive_exclusion_pattern='
assert_contains 'application-data-exclusions.txt'
assert_contains "app_data_archive_exclusion_pattern='./logs/*.log'"
assert_contains 'Rollback does not read application data archive policy evidence'
assert_not_contains '--ignore-failed-read'
assert_not_contains 'warning=no-file-changed'

# Docker operations remain bounded, while allowing enough time for a large
# read-only database dump streamed through `docker exec`.
docker_timeout_source="$(extract_function validate_docker_timeout)"
[[ "$docker_timeout_source" == *'validate_docker_timeout() {'* ]] ||
  fail 'validate_docker_timeout source was not found'
eval "$docker_timeout_source"
mode=prepare
unset SUBNEXUS_DOCKER_TIMEOUT_SECONDS
validate_docker_timeout
[[ "$docker_timeout_seconds" == 120 ]] || fail 'Docker timeout default changed unexpectedly'
for accepted_timeout in 10 600 601 1800; do
  SUBNEXUS_DOCKER_TIMEOUT_SECONDS="$accepted_timeout" validate_docker_timeout
  [[ "$docker_timeout_seconds" == "$accepted_timeout" ]] ||
    fail "Docker timeout was not preserved: $accepted_timeout"
done
SUBNEXUS_DOCKER_TIMEOUT_SECONDS=010 validate_docker_timeout
[[ "$docker_timeout_seconds" == 10 ]] || fail 'leading-zero Docker timeout was not parsed as decimal 10'
for rejected_timeout in 9 1801 invalid; do
  if (SUBNEXUS_DOCKER_TIMEOUT_SECONDS="$rejected_timeout" validate_docker_timeout >/dev/null 2>&1); then
    fail "invalid Docker timeout was accepted: $rejected_timeout"
  fi
done
for malformed_timeout in 00009 01800 10000 18446744073709551626; do
  if (SUBNEXUS_DOCKER_TIMEOUT_SECONDS="$malformed_timeout" validate_docker_timeout >/dev/null 2>&1); then
    fail "malformed/overflow Docker timeout was accepted: $malformed_timeout"
  fi
done
mode=switch
for accepted_timeout in 10 600; do
  SUBNEXUS_DOCKER_TIMEOUT_SECONDS="$accepted_timeout" validate_docker_timeout
done
for rejected_timeout in 601 1800; do
  if (SUBNEXUS_DOCKER_TIMEOUT_SECONDS="$rejected_timeout" validate_docker_timeout >/dev/null 2>&1); then
    fail "switch accepted prepare-only Docker timeout: $rejected_timeout"
  fi
done
mode=rollback
SUBNEXUS_DOCKER_TIMEOUT_SECONDS=600 validate_docker_timeout
if (SUBNEXUS_DOCKER_TIMEOUT_SECONDS=1800 validate_docker_timeout >/dev/null 2>&1); then
  fail 'rollback accepted prepare-only Docker timeout'
fi
mode=unexpected
if (SUBNEXUS_DOCKER_TIMEOUT_SECONDS=120 validate_docker_timeout >/dev/null 2>&1); then
  fail 'unknown cutover phase accepted a Docker timeout'
fi

# The owner acknowledgement is intentionally declared exactly once.  A
# duplicate readonly declaration aborts Bash before any phase can fail closed.
[[ "$(grep -Ec '^readonly app_data_owner_approval_token=' "$subject")" == 1 ]] ||
  fail 'application data owner approval token must have exactly one declaration'

for forbidden in \
  'docker pull' 'docker build' 'docker push' 'docker compose' \
  'ssh ' 'scp ' 'docker system prune' 'docker container prune' \
  'docker network prune' 'docker volume prune' 'docker image prune' \
  'docker volume rm' '--network host' '--network container:'; do
  assert_not_contains "$forbidden"
done

prepare_source="$(extract_function prepare_run)"
prepare_argument_count_source="$(extract_function prepare_argument_count_is_valid)"
validate_run_source="$(extract_function validate_run_directory)"
manifest_writer_source="$(extract_function write_initial_manifest)"
switch_source="$(extract_function switch_run)"
rollback_source="$(extract_function rollback_run)"
candidate_contract_source="$(extract_function assert_candidate_runtime_contract)"
[[ "$prepare_source" == *'prepare_run() {'* ]] || fail 'prepare_run source was not found'
[[ "$prepare_argument_count_source" == *'prepare_argument_count_is_valid() {'* ]] ||
  fail 'prepare argument-count helper source was not found'
[[ "$switch_source" == *'switch_run() {'* ]] || fail 'switch_run source was not found'
[[ "$rollback_source" == *'rollback_run() {'* ]] || fail 'rollback_run source was not found'
[[ "$candidate_contract_source" == *'assert_candidate_runtime_contract() {'* ]] ||
  fail 'candidate runtime contract source was not found'
assert_contains 'assert_candidate_network_identities' <(printf '%s\n' "$candidate_contract_source")
assert_not_contains '$network.NetworkID' <(printf '%s\n' "$candidate_contract_source")

eval "$prepare_argument_count_source"
for accepted_argument_count in 8 9 10; do
  prepare_argument_count_is_valid "$accepted_argument_count" ||
    fail "valid prepare argument count was rejected: $accepted_argument_count"
done
for rejected_argument_count in 7 11; do
  if prepare_argument_count_is_valid "$rejected_argument_count"; then
    fail "invalid prepare argument count was accepted: $rejected_argument_count"
  fi
done
assert_contains 'prepare_argument_count_is_valid "$#"' <(printf '%s\n' "$prepare_source")

assert_before_text "$prepare_source" 'require_commands' 'init_docker'
assert_before_text "$prepare_source" 'init_docker' 'validate_source_tree'
assert_before_text "$prepare_source" 'capture_settings_snapshot "$run_dir/settings-before.tsv"' 'write_closed_settings_snapshot'
assert_before_text "$prepare_source" 'write_closed_settings_snapshot' 'write_initial_manifest'
assert_before_text "$prepare_source" 'validate_app_data_owner_inputs' 'init_docker'
assert_contains 'validate_app_data_owner_manifest' <(printf '%s\n' "$validate_run_source")
for owner_manifest_key in app_data_owner_policy app_data_owner_uid app_data_owner_gid app_data_owner_mode; do
  assert_contains "${owner_manifest_key}=%s" <(printf '%s\n' "$manifest_writer_source")
done
assert_contains 'application_data_archive_policy_sha256=%s' <(printf '%s\n' "$manifest_writer_source")
assert_before_text "$prepare_source" 'initialize_prepare_backup_budgets "$app_data_source"' 'backup_postgresql'
assert_before_text "$prepare_source" 'assert_prepare_disk_budget before_postgresql' 'backup_postgresql'
assert_before_text "$prepare_source" 'assert_prepare_disk_budget before_redis' 'backup_redis'
assert_before_text "$prepare_source" 'assert_prepare_disk_budget before_application_data' 'backup_application_data "$app_data_source"'
assert_before_text "$prepare_source" 'write_application_data_archive_policy' 'backup_application_data "$app_data_source"'
assert_before_text "$prepare_source" 'assert_prepare_disk_budget before_image_load' 'load_and_validate_candidate_image'
assert_before_text "$prepare_source" 'load_and_validate_candidate_image' 'assert_prepare_disk_budget after_image_load'
assert_not_contains 'docker_rpc stop' <(printf '%s\n' "$prepare_source")
assert_not_contains 'docker_rpc rename' <(printf '%s\n' "$prepare_source")

load_image_source="$(
  extract_function ensure_image_load_log
  extract_function load_and_validate_candidate_image
)"
[[ "$load_image_source" == *'ensure_image_load_log() {'* &&
   "$load_image_source" == *'load_and_validate_candidate_image() {'* ]] ||
  fail 'candidate image load helper source was not found'
assert_contains 'candidate image load log path already exists'
assert_contains 'mktemp "$run_dir/.image-load.log.XXXXXX"'
assert_contains 'assert_root_owned_regular "$path" '\''candidate image load log'\'''
assert_before_text "$load_image_source" 'ensure_image_load_log' 'docker_rpc image inspect'
assert_before_text "$load_image_source" 'candidate image load log path already exists' 'mv -T -- "$temporary" "$path"'
assert_contains 'mv -T -- "$temporary" "$path"'

# Fixture 0b: a preloaded candidate tag still produces a controlled image-load
# log, and a stale symlink at that path is rejected before Docker is queried.
if [[ "$OSTYPE" == linux* ]] && command -v stat >/dev/null 2>&1; then
  (
    set -Eeuo pipefail
    eval "$load_image_source"
    fixture_root="$(mktemp -d "${TMPDIR:-/tmp}/subnexus-image-load-log.XXXXXX")"
    trap 'rm -rf -- "$fixture_root"' EXIT
    run_dir="$fixture_root/run"
    mkdir -- "$run_dir"
    chmod 700 -- "$run_dir"
    candidate_archive="$fixture_root/candidate.tar"
    printf 'approved archive fixture\n' > "$candidate_archive"
    candidate_archive_sha='fixture-archive-sha'
    candidate_tag_prefix='subnexus-release:'
    target_sha="$(printf 'a%.0s' {1..40})"
    expected_image_id="$(printf 'b%.0s' {1..64})"
    load_calls=0
    assert_root_owned_regular() { :; }
    hash_file() { printf '%s\n' "$candidate_archive_sha"; }
    fail() { printf 'image-load-log fixture failure: %s\n' "$*" >&2; exit 77; }
    docker_rpc() {
      [[ "$1" == image && "$2" == inspect ]] || {
        [[ "$1" == image && "$2" == load ]] && { load_calls=$((load_calls + 1)); return 99; }
        return 98
      }
      case "${4:-}" in
        '{{.Id}}')
          printf 'sha256:%s\n' "$expected_image_id"
          ;;
        '{{index .Config.Labels "com.subnexus.release.gate"}}|{{index .Config.Labels "com.subnexus.candidate.commit"}}|{{index .Config.Labels "org.opencontainers.image.revision"}}')
          printf 'subnexus-isolated-build-v1|%s|%s\n' "$target_sha" "$target_sha"
          ;;
        '{{.Os}}|{{.Architecture}}') printf 'linux|amd64\n' ;;
        '{{range .Config.Env}}{{println .}}{{end}}') printf 'SAFE_FIXTURE=1\n' ;;
        *) return 97 ;;
      esac
    }
    load_and_validate_candidate_image
    [[ "$load_calls" == 0 ]] || fail 'preloaded tag unexpectedly triggered image load'
    [[ -f "$run_dir/image-load.log" && ! -L "$run_dir/image-load.log" ]] || fail 'preloaded tag did not create a regular image-load log'
    [[ "$(stat -c '%a' -- "$run_dir/image-load.log")" == 600 ]] || fail 'image-load log mode is not 600'
    [[ ! -s "$run_dir/image-load.log" ]] || fail 'preloaded image-load log should be empty'
    rm -f -- "$run_dir/image-load.log"
    ln -s -- "$fixture_root/escape" "$run_dir/image-load.log"
    if ( ensure_image_load_log ); then
      fail 'symbolic image-load log path was accepted'
    fi
  )
else
  printf 'subnexus preloaded image-load log fixture skipped (requires Linux stat)\n'
fi

assert_before_text "$switch_source" 'assert_runtime_still_matches_prepare' 'docker_rpc stop --time'
assert_before_text "$switch_source" 'docker_rpc stop --time' 'docker_rpc rename "$app_id" "$preserved_name"'
assert_before_text "$switch_source" 'docker_rpc rename "$app_id" "$preserved_name"' 'close_rollout_gates'
assert_before_text "$switch_source" 'create_candidate_container' 'docker_rpc start "$candidate_id"'
assert_before_text "$switch_source" 'assert_candidate_runtime_contract' 'docker_rpc start "$candidate_id"'
assert_before_text "$switch_source" 'docker_rpc start "$candidate_id"' 'validate_candidate_runtime'
assert_not_contains '"sha256:$candidate_id"'
assert_contains 'args=(--name "$candidate_name"'
assert_not_contains 'args=(create --name'
assert_contains 'docker_rpc container create "${args[@]}"'

# Candidate containers created before start can expose an empty or unstable
# EndpointSettings.NetworkID on Docker 29.  The helper must use the attached
# network names plus authoritative network objects, while still rejecting
# object replacement and unexpected attachments.
candidate_network_source="$(extract_function assert_candidate_network_identities)"
[[ "$candidate_network_source" == *'assert_candidate_network_identities() {'* ]] ||
  fail 'candidate network identity helper source was not found'
(
  set -Eeuo pipefail
  eval "$candidate_network_source"
  fixture_root="$(mktemp -d "${TMPDIR:-/tmp}/subnexus-network-identity.XXXXXX")"
  trap 'rm -rf -- "$fixture_root"' EXIT
  run_dir="$fixture_root/run"
  mkdir -- "$run_dir"
  network0_id="$(printf 'a%.0s' {1..64})"
  network1_id="$(printf 'b%.0s' {1..64})"
  drift_id="$(printf 'c%.0s' {1..64})"
  candidate_id="$(printf 'd%.0s' {1..64})"
  printf 'net0|%s\nnet1|%s\n' "$network0_id" "$network1_id" > "$run_dir/network-identities.txt"
  valid_container_ref() { [[ "${1:-}" =~ ^[0-9a-f]{64}$ ]]; }
  fail() { printf 'network identity fixture failure: %s\n' "$*" >&2; exit 77; }
  names_mode=stable
  ids_mode=stable
  docker_rpc() {
    if [[ "${1:-}" == inspect ]]; then
      [[ "${2:-}" == --format ]] || fail "unexpected inspect call: $*"
      [[ "$names_mode" == stable ]] && printf 'net0\nnet1\n' || printf 'net0\nnet-extra\n'
    elif [[ "${1:-}" == network && "${2:-}" == inspect ]]; then
      [[ "${3:-}" == --format && "${4:-}" == '{{.Id}}' ]] || fail "unexpected network format: $*"
      case "${5:-}" in
        net0) printf '%s\n' "$network0_id" ;;
        net1) [[ "$ids_mode" == stable ]] && printf '%s\n' "$network1_id" || printf '%s\n' "$drift_id" ;;
        *) fail "unexpected network name: ${5:-}" ;;
      esac
    else
      fail "unexpected Docker call: $*"
    fi
  }
  # The candidate mock intentionally exposes names only; no endpoint NetworkID
  # is returned, matching the Docker 29 created-container behavior.
  assert_candidate_network_identities
  ids_mode=drift
  if (assert_candidate_network_identities >/dev/null 2>&1); then
    fail 'network object ID drift was accepted'
  fi
  ids_mode=stable
  names_mode=drift
  if (assert_candidate_network_identities >/dev/null 2>&1); then
    fail 'candidate network name drift was accepted'
  fi
  names_mode=stable
  printf 'bad/name|%s\n' "$network0_id" > "$run_dir/network-identities.txt"
  if (assert_candidate_network_identities >/dev/null 2>&1); then
    fail 'malformed prepared network name was accepted'
  fi
  printf 'host|%s\n' "$network0_id" > "$run_dir/network-identities.txt"
  if (assert_candidate_network_identities >/dev/null 2>&1); then
    fail 'special prepared network was accepted'
  fi
  printf 'net0|%s\nnet0|%s\n' "$network0_id" "$network0_id" > "$run_dir/network-identities.txt"
  if (assert_candidate_network_identities >/dev/null 2>&1); then
    fail 'duplicate prepared network name was accepted'
  fi
)

# Exercise candidate-container argument construction with a local Docker mock.
# This catches a duplicated subcommand that static text checks alone can miss.
create_candidate_source="$(awk '
  /^create_candidate_container\(\) \{$/ { capture = 1 }
  capture && /^assert_candidate_container_identity\(\) \{$/ { exit }
  capture { print }
' "$subject")"
[[ "$create_candidate_source" == *'create_candidate_container() {'* &&
   "$create_candidate_source" == *'docker_rpc container create "${args[@]}"'* ]] ||
  fail 'candidate-container helper source was not found'
(
  set -Eeuo pipefail
  eval "$create_candidate_source"
  fixture_root="$(mktemp -d "${TMPDIR:-/tmp}/subnexus-create-args.XXXXXX")"
  trap 'rm -rf -- "$fixture_root"' EXIT
  run_dir="$fixture_root/run"
  mkdir -- "$run_dir"
  for metadata_file in container.env user.txt resource-policy.txt healthcheck.json \
    network-aliases.txt mounts.txt entrypoint.txt cmd.txt workdir.txt ulimits.txt; do
    : >"$run_dir/$metadata_file"
  done
  app_name='subnexus-cutover'
  tool_name='subnexus-production-cutover-v1'
  expected_image_id="$(printf 'b%.0s' {1..64})"
  expected_candidate_id="$(printf 'd%.0s' {1..64})"
  app_networks=('sub2api-net')
  captured_ports=(
    '8080/tcp|0.0.0.0|18083'
    '9090/tcp||19090'
    '7070/tcp|127.0.0.1|17070'
  )
  captured_entrypoint=()
  candidate_id=''
  candidate_restart_arg() { printf 'unless-stopped\n'; }
  valid_container_ref() { [[ "$1" == "$app_name" ]]; }
  valid_sha64() { [[ "$1" =~ ^[0-9a-f]{64}$ ]]; }
  hash_text() { printf 'a%.0s' {1..64}; }
  manifest_value() {
    case "$1" in
      run_id) printf 'fixture-run\n' ;;
      target_sha) printf 'c%.0s' {1..40} ;;
      *) printf '\n' ;;
    esac
  }
  manifest_set() { :; }
  append_security_opt_args() { :; }
  append_log_config_args() { :; }
  append_resource_args() { :; }
  append_ulimit_args() { :; }
  append_healthcheck_args() { :; }
  append_network_alias_args() { :; }
  read_one_line() { head -n 1 -- "$1"; }
  fail() { printf 'candidate-create fixture failure: %s\n' "$*" >&2; exit 77; }
  docker_rpc() {
    [[ "$1" == container && "$2" == create ]] || fail "unexpected Docker command: $*"
    shift 2
    local image_token='' arg
    for arg in "$@"; do
      [[ "$arg" != create ]] || fail 'bare create token was passed as an image/command argument'
      if [[ "$arg" == sha256:* ]]; then
        [[ -z "$image_token" ]] || fail 'multiple image tokens were passed'
        image_token="$arg"
      fi
    done
    [[ "$image_token" == "sha256:$expected_image_id" ]] ||
      fail "candidate image token mismatch: $image_token"
    local args_text
    args_text="$(printf '%s\n' "$@")"
    [[ "$args_text" == *$'0.0.0.0:18083:8080/tcp\n'* ]] ||
      fail 'explicit 0.0.0.0 port binding was not preserved'
    [[ "$args_text" == *$'19090:9090/tcp\n'* ]] ||
      fail 'empty HostIP port binding was not reproduced'
    [[ "$args_text" == *$'127.0.0.1:17070:7070/tcp\n'* ]] ||
      fail 'loopback port binding was not preserved'
    printf '%s\n' "$expected_candidate_id"
  }
  create_candidate_container
  [[ "$candidate_id" == "$expected_candidate_id" ]] ||
    fail 'candidate ID was not captured from Docker mock'
  [[ "$(cat "$run_dir/candidate-container-id")" == "$candidate_id" ]] ||
    fail 'candidate ID evidence did not match Docker mock'
)
assert_not_contains 'write_closed_settings_snapshot' <(printf '%s\n' "$switch_source")
assert_contains 'cutover_active=1' <(printf '%s\n' "$switch_source")
assert_contains 'SUBNEXUS_CUTOVER_QUIET_CONFIRM' <(printf '%s\n' "$switch_source")

for forbidden in pg_restore postgresql.dump redis.rdb application-data.tar.gz \
  'docker volume rm' 'backup_postgresql' 'backup_redis' 'backup_application_data'; do
  assert_not_contains "$forbidden" <(printf '%s\n' "$rollback_source")
done
assert_contains 'restore_preserved_container' <(printf '%s\n' "$rollback_source")
assert_contains 'assert_dependencies_still_match' <(printf '%s\n' "$rollback_source")
assert_contains 'restore_rollout_gates' <(printf '%s\n' "$rollback_source")
assert_contains 'settings_snapshot_matches_file' <(printf '%s\n' "$rollback_source")
assert_contains 'current rollout settings match neither the closed nor prepared snapshot' <(printf '%s\n' "$rollback_source")
assert_contains 'state" == rolled_back' <(printf '%s\n' "$rollback_source")
assert_before_text "$rollback_source" 'restore_preserved_container' 'restore_rollout_gates'
assert_before_text "$rollback_source" 'write_run_marker ROLLED_BACK rolled_back' 'manifest_set state rolled_back'
assert_contains 'assert_run_marker READY prepared'

assert_contains 'for metadata_file in "$run_dir"/*.json "$run_dir"/*.env "$run_dir"/*.txt; do'
assert_contains '[[ -f "$metadata_file" && ! -L "$metadata_file" ]] || continue'
assert_not_contains 'chmod 600 "$run_dir"/*.sha256'

# ---------------------------------------------------------------------------
# Fixture 0a: application-data owner input and manifest contracts
# ---------------------------------------------------------------------------

owner_input_source="$(
  extract_function validate_app_data_owner_inputs
  extract_function app_data_owner_mode_is_safe
  extract_function validate_app_data_owner_runtime_inputs
)"
[[ "$owner_input_source" == *'validate_app_data_owner_inputs() {'* &&
   "$owner_input_source" == *'validate_app_data_owner_runtime_inputs() {'* ]] ||
  fail 'application data owner input helper source was not found'
(
  set -Eeuo pipefail
  eval "$owner_input_source"
  fail() { printf 'owner input fixture failure: %s\n' "$*" >&2; exit 77; }
  app_data_owner_approval_token='I_UNDERSTAND_NON_ROOT_APP_DATA_OWNER'
  app_data_owner_compat_uid='1000'
  app_data_owner_compat_gid='1000'

  unset SUBNEXUS_CUTOVER_APP_DATA_OWNER_CONFIRM \
    SUBNEXUS_CUTOVER_APP_DATA_OWNER_UID \
    SUBNEXUS_CUTOVER_APP_DATA_OWNER_GID
  validate_app_data_owner_inputs
  [[ "$app_data_owner_policy" == root-only && "$app_data_owner_uid" == 0 &&
     "$app_data_owner_gid" == 0 ]] || fail 'unset owner inputs did not select root-only'

  SUBNEXUS_CUTOVER_APP_DATA_OWNER_CONFIRM='I_UNDERSTAND_NON_ROOT_APP_DATA_OWNER'
  SUBNEXUS_CUTOVER_APP_DATA_OWNER_UID=1000
  SUBNEXUS_CUTOVER_APP_DATA_OWNER_GID=1000
  validate_app_data_owner_inputs
  [[ "$app_data_owner_policy" == explicit-uid-gid &&
     "$app_data_owner_uid" == 1000 && "$app_data_owner_gid" == 1000 ]] ||
    fail 'approved non-root owner inputs were not accepted'

  expect_failure() {
    if ( "$@" >/dev/null 2>&1 ); then
      return 1
    fi
    return 0
  }
  # Every non-root input is independently required.
  unset SUBNEXUS_CUTOVER_APP_DATA_OWNER_CONFIRM
  expect_failure validate_app_data_owner_inputs || fail 'missing owner token was accepted'
  SUBNEXUS_CUTOVER_APP_DATA_OWNER_CONFIRM='I_UNDERSTAND_NON_ROOT_APP_DATA_OWNER'
  unset SUBNEXUS_CUTOVER_APP_DATA_OWNER_UID
  expect_failure validate_app_data_owner_inputs || fail 'missing owner UID was accepted'
  SUBNEXUS_CUTOVER_APP_DATA_OWNER_UID=1000
  unset SUBNEXUS_CUTOVER_APP_DATA_OWNER_GID
  expect_failure validate_app_data_owner_inputs || fail 'missing owner GID was accepted'

  # Only the reviewed 1000:1000 compatibility owner is allowed.
  SUBNEXUS_CUTOVER_APP_DATA_OWNER_GID=1001
  expect_failure validate_app_data_owner_inputs || fail 'unreviewed owner GID was accepted'
  SUBNEXUS_CUTOVER_APP_DATA_OWNER_GID=1000
  SUBNEXUS_CUTOVER_APP_DATA_OWNER_UID=0
  expect_failure validate_app_data_owner_inputs || fail 'root UID was accepted as non-root opt-in'
  SUBNEXUS_CUTOVER_APP_DATA_OWNER_UID=4294967296
  SUBNEXUS_CUTOVER_APP_DATA_OWNER_GID=1000
  expect_failure validate_app_data_owner_inputs || fail 'out-of-range owner UID was accepted'

  # Runtime validation rejects stale owner variables for a root-only run and
  # requires the same explicit values for an opted-in run.
  unset SUBNEXUS_CUTOVER_APP_DATA_OWNER_CONFIRM \
    SUBNEXUS_CUTOVER_APP_DATA_OWNER_UID \
    SUBNEXUS_CUTOVER_APP_DATA_OWNER_GID
  app_data_owner_policy=root-only
  app_data_owner_uid=0
  app_data_owner_gid=0
  validate_app_data_owner_runtime_inputs
  SUBNEXUS_CUTOVER_APP_DATA_OWNER_UID=1000
  expect_failure validate_app_data_owner_runtime_inputs || fail 'owner input leaked into root-only runtime policy'
  app_data_owner_policy=explicit-uid-gid
  SUBNEXUS_CUTOVER_APP_DATA_OWNER_CONFIRM='I_UNDERSTAND_NON_ROOT_APP_DATA_OWNER'
  SUBNEXUS_CUTOVER_APP_DATA_OWNER_UID=1000
  SUBNEXUS_CUTOVER_APP_DATA_OWNER_GID=1000
  validate_app_data_owner_runtime_inputs

  for accepted_mode in 700 750 755; do
    app_data_owner_mode_is_safe "$accepted_mode" || fail "safe owner mode was rejected: $accepted_mode"
  done
  for rejected_mode in 770 707 600 1777 4755 0755; do
    if app_data_owner_mode_is_safe "$rejected_mode"; then
      fail "unsafe owner mode was accepted: $rejected_mode"
    fi
  done
)

owner_manifest_source="$(
  extract_function manifest_value
  extract_function manifest_has_key
  extract_function validate_app_data_owner_inputs
  extract_function app_data_owner_mode_is_safe
  extract_function validate_app_data_owner_runtime_inputs
  extract_function validate_app_data_owner_manifest
)"
[[ "$owner_manifest_source" == *'validate_app_data_owner_manifest() {'* ]] ||
  fail 'application data owner manifest helper source was not found'
(
  set -Eeuo pipefail
  eval "$owner_manifest_source"
  fail() { printf 'owner manifest fixture failure: %s\n' "$*" >&2; exit 77; }
  app_data_owner_approval_token='I_UNDERSTAND_NON_ROOT_APP_DATA_OWNER'
  app_data_owner_compat_uid='1000'
  app_data_owner_compat_gid='1000'
  fixture_root="$(mktemp -d "${TMPDIR:-/tmp}/subnexus-cutover-owner-manifest.XXXXXX")"
  trap 'rm -rf -- "$fixture_root"' EXIT
  manifest_file="$fixture_root/manifest.env"
  app_data_owner_policy=root-only
  app_data_owner_uid=0
  app_data_owner_gid=0
  app_data_owner_mode=''
  unset SUBNEXUS_CUTOVER_APP_DATA_OWNER_CONFIRM \
    SUBNEXUS_CUTOVER_APP_DATA_OWNER_UID \
    SUBNEXUS_CUTOVER_APP_DATA_OWNER_GID

  # A fully absent owner field set is the explicit legacy root-only path.
  printf 'state=prepared\n' > "$manifest_file"
  validate_app_data_owner_manifest
  [[ "$app_data_owner_manifest_legacy" == 1 &&
     "$app_data_owner_policy" == root-only ]] || fail 'legacy owner manifest was not root-only'

  # Partial fields are not a legacy manifest and must fail closed.
  printf 'app_data_owner_policy=root-only\napp_data_owner_uid=0\n' > "$manifest_file"
  if (validate_app_data_owner_manifest >/dev/null 2>&1); then
    fail 'partial owner manifest was accepted'
  fi

  printf 'app_data_owner_policy=explicit-uid-gid\napp_data_owner_uid=1000\napp_data_owner_gid=1000\napp_data_owner_mode=755\n' > "$manifest_file"
  SUBNEXUS_CUTOVER_APP_DATA_OWNER_CONFIRM='I_UNDERSTAND_NON_ROOT_APP_DATA_OWNER'
  SUBNEXUS_CUTOVER_APP_DATA_OWNER_UID=1000
  SUBNEXUS_CUTOVER_APP_DATA_OWNER_GID=1000
  validate_app_data_owner_manifest
  [[ "$app_data_owner_manifest_legacy" == 0 &&
     "$app_data_owner_policy" == explicit-uid-gid &&
     "$app_data_owner_uid" == 1000 && "$app_data_owner_gid" == 1000 &&
     "$app_data_owner_mode" == 755 ]] || fail 'explicit owner manifest was not loaded exactly'
  unset SUBNEXUS_CUTOVER_APP_DATA_OWNER_CONFIRM
  if (validate_app_data_owner_manifest >/dev/null 2>&1); then
    fail 'explicit owner manifest without repeated acknowledgement was accepted'
  fi
)

# ---------------------------------------------------------------------------
# Fixture 1: inspect errors are distinguished from an absent container
# ---------------------------------------------------------------------------

inspect_source="$(extract_function inspect_container_id_or_empty)"
[[ "$inspect_source" == *'inspect_container_id_or_empty() {'* ]] || fail 'inspect helper source missing'
(
  set -Eeuo pipefail
  eval "$inspect_source"
  fixture='present'
  bare_id="$(printf 'a%.0s' {1..64})"
  full_id="sha256:$bare_id"
  fail() { exit 77; }
  docker_rpc() {
    case "$fixture" in
      present) printf '%s\n' "$full_id"; return 0 ;;
      absent) printf 'Error: No such object: fixture\n' >&2; return 1 ;;
      timeout) printf 'Error: context deadline exceeded\n' >&2; return 124 ;;
      denied) printf 'Error: permission denied\n' >&2; return 1 ;;
    esac
  }
  [[ "$(inspect_container_id_or_empty fixture)" == "$bare_id" ]] || fail 'present inspect did not return the canonical bare ID'
  full_id="$bare_id"
  [[ "$(inspect_container_id_or_empty fixture)" == "$bare_id" ]] || fail 'bare Docker ID was not preserved'
  fixture=absent
  [[ -z "$(inspect_container_id_or_empty fixture)" ]] || fail 'missing inspect did not return an empty ID'
  fixture=timeout
  if (inspect_container_id_or_empty fixture >/dev/null 2>&1); then
    fail 'timeout inspect was treated as absent'
  fi
  fixture=denied
  if (inspect_container_id_or_empty fixture >/dev/null 2>&1); then
    fail 'permission inspect was treated as absent'
  fi
)

# ---------------------------------------------------------------------------
# Fixture 2: path-chain and directory-lock symlink protection
# ---------------------------------------------------------------------------

path_source="$(
  extract_function mode_is_safe
  extract_function assert_root_owned_regular
  extract_function assert_root_owned_dir
  extract_function app_data_owner_mode_is_safe
  extract_function assert_app_data_path_chain
  extract_function assert_root_owned_path_chain
  extract_function acquire_lock
)"
(
  set -Eeuo pipefail
  eval "$path_source"
  fail() { exit 77; }
  fixture_root="$(mktemp -d "${TMPDIR:-/tmp}/subnexus-cutover-path.XXXXXX")"
  trap 'rm -rf -- "$fixture_root"' EXIT
  mkdir -- "$fixture_root/real-dir"
  printf data > "$fixture_root/real-file"
  ln -s -- "$fixture_root/real-file" "$fixture_root/file-link"
  ln -s -- "$fixture_root/real-dir" "$fixture_root/dir-link"
  if (assert_root_owned_regular "$fixture_root/file-link" fixture-file >/dev/null 2>&1); then
    fail 'regular-file symlink was accepted'
  fi
  if (assert_root_owned_dir "$fixture_root/dir-link" fixture-dir >/dev/null 2>&1); then
    fail 'directory symlink was accepted'
  fi
  if (assert_root_owned_path_chain "$fixture_root/dir-link" fixture-chain >/dev/null 2>&1); then
    fail 'symlink path component was accepted by the path-chain guard'
  fi
  if (acquire_lock "$fixture_root/dir-link" >/dev/null 2>&1); then
    fail 'symlink evidence root was accepted by acquire_lock'
  fi
)

if [[ "$OSTYPE" == linux* && "${EUID:-1}" == 0 && -r /proc/self/fd ]] && command -v flock >/dev/null 2>&1; then
  (
    set -Eeuo pipefail
    eval "$path_source"
    fail() { exit 77; }
    fixture_root="$(mktemp -d "${TMPDIR:-/tmp}/subnexus-cutover-lock.XXXXXX")"
    trap 'rm -rf -- "$fixture_root"' EXIT
    mkdir -- "$fixture_root/lock-root"
    acquire_lock "$fixture_root/lock-root"
    [[ "${lock_fd_open:-0}" == 1 ]] || fail 'directory lock descriptor was not opened'
    exec 9>&-
  )
else
  printf 'subnexus cutover directory-lock success fixture skipped (requires Linux /proc, root, and flock)\n'
fi

# ---------------------------------------------------------------------------
# Fixture 2a: application-data owner leaf, parent-chain, and identity guards
# ---------------------------------------------------------------------------

owner_path_source="$(
  extract_function mode_is_safe
  extract_function app_data_owner_mode_is_safe
  extract_function assert_app_data_path_chain
  extract_function assert_root_owned_path_chain
  extract_function path_overlaps
  extract_function resolve_app_data_source
  extract_function manifest_value
  extract_function manifest_has_key
  extract_function validate_app_data_owner_inputs
  extract_function validate_app_data_owner_runtime_inputs
  extract_function validate_app_data_owner_manifest
  extract_function validate_app_data_source_identity_file
  extract_function compute_app_data_source_identity
  extract_function assert_app_data_source_identity
  extract_function read_one_line
)"
[[ "$owner_path_source" == *'assert_app_data_path_chain() {'* &&
   "$owner_path_source" == *'assert_root_owned_path_chain() {'* &&
   "$owner_path_source" == *'resolve_app_data_source() {'* &&
   "$owner_path_source" == *'assert_app_data_source_identity() {'* ]] ||
  fail 'application data owner path/identity helper source was not found'

if [[ "$OSTYPE" == linux* && "${EUID:-1}" == 0 ]] &&
   command -v chown >/dev/null 2>&1 && command -v realpath >/dev/null 2>&1; then
  (
    set -Eeuo pipefail
    eval "$owner_path_source"
    fail() { printf 'owner path fixture failure: %s\n' "$*" >&2; exit 77; }
    assert_root_owned_regular() { :; }
    app_data_owner_approval_token='I_UNDERSTAND_NON_ROOT_APP_DATA_OWNER'
    app_data_owner_compat_uid='1000'
    app_data_owner_compat_gid='1000'
    fixture_root="$(mktemp -d /root/subnexus-cutover-owner.XXXXXX)"
    trap 'rm -rf -- "$fixture_root"' EXIT
    chmod 700 -- "$fixture_root"
    parent="$fixture_root/runtime"
    leaf="$parent/subnexus-data"
    mkdir -- "$parent" "$leaf"
    chmod 755 -- "$parent" "$leaf"
    chown 0:0 -- "$fixture_root" "$parent" "$leaf"

    expect_path_failure() {
      if (assert_app_data_path_chain "$@" >/dev/null 2>&1); then
        return 1
      fi
      return 0
    }

    # Root-owned data remains the default contract.
    assert_root_owned_path_chain "$leaf" owner-root 755

    # The reviewed non-root compatibility owner is allowed only at the leaf.
    chown 1000:1000 -- "$leaf"
    assert_app_data_path_chain "$leaf" owner-explicit 1000 1000 755 0
    chmod 775 -- "$leaf"
    expect_path_failure "$leaf" owner-group-write 1000 1000 775 || fail 'group-writable data leaf was accepted'
    chmod 555 -- "$leaf"
    expect_path_failure "$leaf" owner-no-write 1000 1000 555 || fail 'non-writable data leaf was accepted'
    chmod 755 -- "$leaf"
    chown 1001:1000 -- "$leaf"
    expect_path_failure "$leaf" owner-wrong-uid 1000 1000 755 || fail 'wrong data leaf UID was accepted'
    chown 1000:1001 -- "$leaf"
    expect_path_failure "$leaf" owner-wrong-gid 1000 1000 755 || fail 'wrong data leaf GID was accepted'
    chown 1000:1000 -- "$leaf"

    # Every parent remains root-owned and private, and path symlinks fail.
    chown 1000:1000 -- "$parent"
    expect_path_failure "$leaf" owner-nonroot-parent 1000 1000 755 || fail 'non-root parent was accepted'
    chown 0:0 -- "$parent"
    chmod 775 -- "$parent"
    expect_path_failure "$leaf" owner-writable-parent 1000 1000 755 || fail 'writable parent was accepted'
    chmod 755 -- "$parent"
    ln -s -- "$leaf" "$fixture_root/data-link"
    expect_path_failure "$fixture_root/data-link" owner-symlink 1000 1000 755 || fail 'symlink data path was accepted'
    rm -f -- "$fixture_root/data-link"

    # Special mode bits and an unexpected post-prepare mode are rejected.
    chmod 4755 -- "$leaf"
    expect_path_failure "$leaf" owner-special-mode 1000 1000 4755 || fail 'special data mode was accepted'
    chmod a-s -- "$leaf"
    chmod 755 -- "$leaf"
    expect_path_failure "$leaf" owner-mode-drift 1000 1000 700 || fail 'mode drift was accepted'

    # Exercise the actual bind-source resolver, not only its path helper.
    run_dir="$fixture_root/run"
    mkdir -- "$run_dir"
    chmod 700 -- "$run_dir"
    candidate_artifact_root="$fixture_root/artifacts"
    alternate_candidate_artifact_root="$fixture_root/alt-artifacts"
    candidate_gate_root="$fixture_root/gate"
    alternate_candidate_gate_root="$fixture_root/alt-gate"
    printf 'bind|%s||\n' "$leaf" > "$run_dir/app-data-mount.txt"
    app_data_owner_policy=explicit-uid-gid
    app_data_owner_uid=1000
    app_data_owner_gid=1000
    app_data_owner_mode=755
    [[ "$(resolve_app_data_source)" == "$leaf" ]] || fail 'explicit bind owner resolver returned the wrong source'
    app_data_owner_policy=root-only
    chown 0:0 -- "$leaf"
    app_data_owner_uid=0
    app_data_owner_gid=0
    [[ "$(resolve_app_data_source)" == "$leaf" ]] || fail 'root-only resolver rejected a root-owned data leaf'
    # A root-only resolver must reject the non-root leaf even if the path
    # helper itself supports explicit owner values.
    chown 1000:1000 -- "$leaf"
    if (resolve_app_data_source >/dev/null 2>&1); then
      fail 'root-only resolver accepted a non-root data leaf'
    fi

    # Capture an identity and prove owner/mode/inode drift is detected.
    app_data_owner_policy=explicit-uid-gid
    app_data_owner_uid=1000
    app_data_owner_gid=1000
    app_data_owner_mode=755
    chown 1000:1000 -- "$leaf"
    fingerprint="$(stat -Lc '%d,%i,%F,%u,%g,%a' -- "$leaf")"
    printf 'bind|%s||%s|%s|-\n' "$leaf" "$leaf" "$fingerprint" > "$run_dir/app-data-source.identity"
    manifest_file="$fixture_root/no-manifest"
    SUBNEXUS_CUTOVER_APP_DATA_OWNER_CONFIRM='I_UNDERSTAND_NON_ROOT_APP_DATA_OWNER'
    SUBNEXUS_CUTOVER_APP_DATA_OWNER_UID=1000
    SUBNEXUS_CUTOVER_APP_DATA_OWNER_GID=1000
    assert_app_data_source_identity
    chmod 750 -- "$leaf"
    if (assert_app_data_source_identity >/dev/null 2>&1); then
      fail 'application data mode/inode identity drift was accepted'
    fi
    chmod 755 -- "$leaf"
    mv -- "$leaf" "$leaf.old"
    mkdir -- "$leaf"
    chmod 755 -- "$leaf"
    chown 1000:1000 -- "$leaf"
    if (assert_app_data_source_identity >/dev/null 2>&1); then
      fail 'application data inode replacement was accepted'
    fi

    # A pre-owner-contract manifest remains rollback-compatible through the
    # actual manifest/resolver/identity path: its historical validator only
    # constrained the leaf UID to zero, so a non-zero but non-writable GID is
    # retained as a legacy exception.
    rm -rf -- "$leaf"
    mv -- "$leaf.old" "$leaf"
    chmod 755 -- "$leaf"
    chown 0:1001 -- "$leaf"
    manifest_file="$fixture_root/legacy-manifest"
    printf 'state=prepared\n' > "$manifest_file"
    app_data_owner_policy=explicit-uid-gid
    app_data_owner_uid=1000
    app_data_owner_gid=1000
    app_data_owner_mode=755
    unset SUBNEXUS_CUTOVER_APP_DATA_OWNER_CONFIRM \
      SUBNEXUS_CUTOVER_APP_DATA_OWNER_UID \
      SUBNEXUS_CUTOVER_APP_DATA_OWNER_GID
    validate_app_data_owner_manifest
    [[ "$app_data_owner_manifest_legacy" == 1 &&
       "$app_data_owner_policy" == root-only &&
       -z "$app_data_owner_gid" ]] || fail 'legacy owner manifest was not loaded through the resolver contract'
    fingerprint="$(stat -Lc '%d,%i,%F,%u,%g,%a' -- "$leaf")"
    printf 'bind|%s||%s|%s|-\n' "$leaf" "$leaf" "$fingerprint" > "$run_dir/app-data-source.identity"
    [[ "$(resolve_app_data_source)" == "$leaf" ]] || fail 'legacy resolver rejected a historically valid root-UID leaf'
    assert_app_data_source_identity
  )
else
  printf 'subnexus application-data owner fixtures skipped (requires Linux root and chown)\n'
fi

# ---------------------------------------------------------------------------
# Fixture 3: environment metadata is deterministic (no duplicate keys)
# ---------------------------------------------------------------------------

environment_source="$(extract_function validate_environment_file)"
(
  set -Eeuo pipefail
  eval "$environment_source"
  fail() { exit 77; }
  assert_root_owned_regular() { :; }
  fixture_root="$(mktemp -d "${TMPDIR:-/tmp}/subnexus-cutover-env.XXXXXX")"
  trap 'rm -rf -- "$fixture_root"' EXIT
  printf 'DATABASE_HOST=postgres\nDATABASE_PASSWORD=secret=with=equals\n' > "$fixture_root/valid.env"
  validate_environment_file "$fixture_root/valid.env"
  printf 'DATABASE_HOST=postgres\nDATABASE_HOST=redis\n' > "$fixture_root/duplicate.env"
  if (validate_environment_file "$fixture_root/duplicate.env" >/dev/null 2>&1); then
    fail 'duplicate environment key was accepted'
  fi
  printf 'DATABASE-HOST=postgres\n' > "$fixture_root/invalid.env"
  if (validate_environment_file "$fixture_root/invalid.env" >/dev/null 2>&1); then
    fail 'invalid environment key was accepted'
  fi
  printf 'DATABASE_HOST\n' > "$fixture_root/unassigned.env"
  if (validate_environment_file "$fixture_root/unassigned.env" >/dev/null 2>&1); then
    fail 'unassigned environment entry was accepted'
  fi
)

# ---------------------------------------------------------------------------
# Fixture 3a: Docker template/CLI double-newline mount output is tolerated
# only when the complete record is empty; malformed partial records fail.
# ---------------------------------------------------------------------------

mount_source="$(extract_function capture_mounts)"
[[ "$mount_source" == *'capture_mounts() {'* ]] || fail 'mount capture helper source was not found'
(
  set -Eeuo pipefail
  fail() { printf 'mount fixture failure: %s\n' "$*" >&2; exit 77; }
  fixture_root="$(mktemp -d "${TMPDIR:-/tmp}/subnexus-cutover-mounts.XXXXXX")"
  trap 'rm -rf -- "$fixture_root"' EXIT
  mkdir -- "$fixture_root/data"
  run_dir="$fixture_root/run"
  mkdir -- "$run_dir"
  app_id=fixture-app
  docker_rpc() {
    [[ "${1:-}" == inspect ]] || return 98
    printf 'bind|%s||/app/data|rw|true|rprivate\n\n' "$fixture_root/data"
  }
  eval "$mount_source"
  capture_mounts
  [[ "$(cat "$run_dir/mounts.txt")" == "bind|$fixture_root/data||/app/data|rw|true|rprivate" ]] ||
    fail 'mount capture did not ignore the trailing empty record'
  docker_rpc() {
    [[ "${1:-}" == inspect ]] || return 98
    printf 'bind|%s||/app/data|rw|true|\n\n' "$fixture_root/data"
  }
  capture_mounts
  [[ "$(cat "$run_dir/mounts.txt")" == "bind|$fixture_root/data||/app/data|rw|true|" ]] ||
    fail 'mount capture did not accept an empty propagation field'
  printf 'bind|%s||/app/data|rw|\n\n' "$fixture_root/data" > "$run_dir/invalid-mount-output"
  if (
    docker_rpc() { [[ "${1:-}" == inspect ]] || return 98; cat "$run_dir/invalid-mount-output"; }
    capture_mounts >/dev/null 2>&1
  ); then
    fail 'partial mount record was accepted'
  fi
)

# ---------------------------------------------------------------------------
# Fixture 3b: duplicate Docker environment entries require explicit approval,
# retain Docker's last-wins value without recording plaintext evidence, and
# remain comparable after Docker canonicalizes the candidate to unique keys.
# This fixture requires a real Python 3 interpreter; Windows AppInstaller's
# `python3` redirector is intentionally treated as unavailable.
# ---------------------------------------------------------------------------

if command -v python3 >/dev/null 2>&1 && python3 -c 'import hashlib, json' >/dev/null 2>&1; then
  environment_duplicate_source="$({
    awk '
      /^validate_environment_duplicate_inputs\(\) \{/ { capture = 1 }
      /^validate_security_options_file\(\) \{/ { capture = 0 }
      capture { print }
    ' "$subject"
  })"
  [[ "$environment_duplicate_source" == *'capture_environment_metadata() {'* &&
     "$environment_duplicate_source" == *'assert_environment_matches_prepare() {'* ]] ||
    fail 'duplicate environment helper source was not found'
  (
    set -Eeuo pipefail
    eval "$environment_duplicate_source"
    fail() { printf 'duplicate environment fixture failure: %s\n' "$*" >&2; exit 77; }
    assert_root_owned_regular() { :; }
    hash_file() { sha256sum -- "$1" | awk '{print $1}'; }
    expect_failure() {
      if ("$@" >/dev/null 2>&1); then
        return 0
      fi
      return 1
    }
    environment_duplicate_approval_token='I_UNDERSTAND_DOCKER_ENV_LAST_WINS'
    environment_duplicate_mode='strict'
    environment_duplicate_keys=''
    environment_duplicate_expected_hashes=''
    environment_duplicate_evidence_sha256=''
    environment_file_sha256=''
    environment_duplicate_legacy=0
    environment_observed_mode=''
    environment_observed_keys=''
    environment_observed_expected_hashes=''
    environment_observed_evidence_sha256=''
    environment_observed_file_sha256=''
    fixture_root="$(mktemp -d "${TMPDIR:-/tmp}/subnexus-cutover-env-duplicate.XXXXXX")"
    trap 'rm -rf -- "$fixture_root"' EXIT
    fixture_json="$fixture_root/env.json"
    cat >"$fixture_json" <<'JSON'
["A=1", "SERVER_TRUSTED_PROXIES=old-value", "B=2", "SERVER_TRUSTED_PROXIES=new-value"]
JSON
    docker_rpc() { [[ "${1:-}" == inspect ]] || return 98; cat -- "$fixture_json"; }
    final_hash="$(printf %s 'new-value' | sha256sum | awk '{print $1}')"
    env_file="$fixture_root/container.env"
    evidence_file="$fixture_root/environment-duplicates.tsv"
    unset SUBNEXUS_CUTOVER_ENV_DUPLICATE_CONFIRM \
      SUBNEXUS_CUTOVER_ENV_DUPLICATE_KEYS \
      SUBNEXUS_CUTOVER_ENV_DUPLICATE_EXPECTED_SHA256
    if expect_failure capture_environment_metadata fixture "$env_file" "$evidence_file" prepare; then
      fail 'duplicate environment metadata was accepted without approval'
    fi
    SUBNEXUS_CUTOVER_ENV_DUPLICATE_CONFIRM="$environment_duplicate_approval_token"
    if expect_failure capture_environment_metadata fixture "$env_file" "$evidence_file" prepare; then
      fail 'duplicate environment metadata was accepted without an allowlist/hash'
    fi
    SUBNEXUS_CUTOVER_ENV_DUPLICATE_KEYS=SERVER_TRUSTED_PROXIES
    wrong_hash="$(printf %s 'old-value' | sha256sum | awk '{print $1}')"
    SUBNEXUS_CUTOVER_ENV_DUPLICATE_EXPECTED_SHA256="SERVER_TRUSTED_PROXIES=$wrong_hash"
    if expect_failure capture_environment_metadata fixture "$env_file" "$evidence_file" prepare; then
      fail 'duplicate environment metadata was accepted with an incorrect approval hash'
    fi
    SUBNEXUS_CUTOVER_ENV_DUPLICATE_EXPECTED_SHA256="SERVER_TRUSTED_PROXIES=$(printf %064d 0)"
    if expect_failure capture_environment_metadata fixture "$env_file" "$evidence_file" prepare; then
      fail 'duplicate environment metadata was accepted with a zero hash'
    fi
    SUBNEXUS_CUTOVER_ENV_DUPLICATE_EXPECTED_SHA256="SERVER_TRUSTED_PROXIES=$final_hash"
    capture_environment_metadata fixture "$env_file" "$evidence_file" prepare
    [[ "$environment_duplicate_mode" == last-wins &&
       "$environment_duplicate_keys" == SERVER_TRUSTED_PROXIES &&
       "$environment_duplicate_expected_hashes" == "SERVER_TRUSTED_PROXIES=$final_hash" ]] ||
      fail 'approved duplicate environment contract was not recorded'
    [[ "$(< "$env_file")" == $'A=1\nB=2\nSERVER_TRUSTED_PROXIES=new-value' ]] ||
      fail 'last-wins environment normalization was incorrect'
    if grep -Fq -- old-value "$evidence_file" || grep -Fq -- new-value "$evidence_file"; then
      fail 'duplicate environment evidence contains plaintext values'
    fi
    grep -Fq -- "duplicate|SERVER_TRUSTED_PROXIES|2|4|" "$evidence_file" ||
      fail 'duplicate environment evidence did not record occurrence metadata'
    prepared_mode="$environment_duplicate_mode"
    prepared_keys="$environment_duplicate_keys"
    prepared_hashes="$environment_duplicate_expected_hashes"
    prepared_env_sha="$environment_file_sha256"
    prepared_evidence_sha="$environment_duplicate_evidence_sha256"

    # A replay of the unchanged live array must pass and must not mutate the
    # prepared globals used by later candidate/rollback checks.
    capture_environment_metadata fixture - - replay "$prepared_mode" "$prepared_keys" "$prepared_hashes"
    [[ "$environment_observed_mode" == last-wins &&
       "$environment_observed_file_sha256" == "$prepared_env_sha" ]] ||
      fail 'unchanged duplicate environment replay did not match'
    [[ "$environment_duplicate_mode" == "$prepared_mode" &&
       "$environment_duplicate_keys" == "$prepared_keys" &&
       "$environment_duplicate_expected_hashes" == "$prepared_hashes" ]] ||
      fail 'environment replay mutated prepared contract globals'

    # Docker may canonicalize the candidate to the normalized unique array.
    cat >"$fixture_json" <<'JSON'
["A=1", "B=2", "SERVER_TRUSTED_PROXIES=new-value"]
JSON
    assert_environment_matches_prepare fixture candidate
    [[ "$environment_duplicate_mode" == "$prepared_mode" &&
       "$environment_duplicate_keys" == "$prepared_keys" ]] ||
      fail 'candidate replay changed prepared contract globals'

    # A live duplicate sequence change is rejected even when its selected
    # final value hash remains unchanged.
    cat >"$fixture_json" <<'JSON'
["A=1", "SERVER_TRUSTED_PROXIES=other-value", "B=2", "SERVER_TRUSTED_PROXIES=new-value"]
JSON
    if expect_failure assert_environment_matches_prepare fixture live; then
      fail 'live duplicate sequence drift was accepted'
    fi

    # A second duplicate key outside the reviewed allowlist is rejected.
    cat >"$fixture_json" <<'JSON'
["A=1", "SERVER_TRUSTED_PROXIES=old-value", "B=2", "SERVER_TRUSTED_PROXIES=new-value", "C=x", "C=y"]
JSON
    if expect_failure capture_environment_metadata fixture - - replay "$prepared_mode" "$prepared_keys" "$prepared_hashes"; then
      fail 'unapproved additional duplicate environment key was accepted'
    fi

    # Strict mode remains the default for containers without duplicates.
    cat >"$fixture_json" <<'JSON'
["A=1", "B=2"]
JSON
    unset SUBNEXUS_CUTOVER_ENV_DUPLICATE_CONFIRM \
      SUBNEXUS_CUTOVER_ENV_DUPLICATE_KEYS \
      SUBNEXUS_CUTOVER_ENV_DUPLICATE_EXPECTED_SHA256
    capture_environment_metadata fixture "$env_file" "$evidence_file" prepare
    [[ "$environment_duplicate_mode" == strict && -z "$environment_duplicate_keys" &&
       "$(grep '^source_entries=' "$evidence_file")" == source_entries=2 &&
       "$(grep '^normalized_entries=' "$evidence_file")" == normalized_entries=2 ]] ||
      fail 'strict environment metadata default was not preserved'

    # The runtime contract hash must reject duplicates in strict mode, while
    # last-wins mode must hash identically to the canonical unique candidate.
    runtime_hash_source="$(awk '
      /^capture_runtime_contract_hash\(\) \{/ { capture = 1 }
      /^capture_dependency_identity\(\) \{/ { capture = 0 }
      capture { print }
    ' "$subject")"
    [[ "$runtime_hash_source" == *'capture_runtime_contract_hash() {'* ]] ||
      fail 'runtime contract hash helper source was not found'
    eval "$runtime_hash_source"
    runtime_json="$fixture_root/runtime.json"
    cat >"$runtime_json" <<'JSON'
{"Id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","Name":"/fixture","Config":{"Env":["A=1","SERVER_TRUSTED_PROXIES=old-value","SERVER_TRUSTED_PROXIES=new-value"],"Tty":false},"HostConfig":{"SecurityOpt":[],"LogConfig":{"Type":"json-file","Config":{}}},"Mounts":[],"NetworkSettings":{"Networks":{}}}
JSON
    docker_rpc() { [[ "${1:-}" == inspect ]] || return 98; cat -- "$runtime_json"; }
    environment_duplicate_mode=strict
    if expect_failure capture_runtime_contract_hash fixture; then
      fail 'strict runtime contract hash accepted a duplicate environment entry'
    fi
    environment_duplicate_mode=last-wins
    last_wins_runtime_hash="$(capture_runtime_contract_hash fixture)"
    cat >"$runtime_json" <<'JSON'
{"Id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","Name":"/fixture","Config":{"Env":["A=1","SERVER_TRUSTED_PROXIES=new-value"],"Tty":false},"HostConfig":{"SecurityOpt":[],"LogConfig":{"Type":"json-file","Config":{}}},"Mounts":[],"NetworkSettings":{"Networks":{}}}
JSON
    canonical_runtime_hash="$(capture_runtime_contract_hash fixture)"
    [[ "$last_wins_runtime_hash" == "$canonical_runtime_hash" ]] ||
      fail 'last-wins runtime hash differs from the canonical candidate hash'

    # Docker 29 serializes an unset OomKillDisable pointer as false on a new
    # container. Both values mean the OOM killer remains enabled and must have
    # one stable contract hash; true must remain observably different.
    cat >"$runtime_json" <<'JSON'
{"Id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","Name":"/fixture","Config":{"Env":["A=1"],"Tty":false},"HostConfig":{"OomKillDisable":null,"SecurityOpt":[],"LogConfig":{"Type":"json-file","Config":{}}},"Mounts":[],"NetworkSettings":{"Networks":{}}}
JSON
    unset_oom_runtime_hash="$(capture_runtime_contract_hash fixture)"
    cat >"$runtime_json" <<'JSON'
{"Id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","Name":"/fixture","Config":{"Env":["A=1"],"Tty":false},"HostConfig":{"OomKillDisable":false,"SecurityOpt":[],"LogConfig":{"Type":"json-file","Config":{}}},"Mounts":[],"NetworkSettings":{"Networks":{}}}
JSON
    false_oom_runtime_hash="$(capture_runtime_contract_hash fixture)"
    [[ "$unset_oom_runtime_hash" == "$false_oom_runtime_hash" ]] ||
      fail 'unset and false OomKillDisable values produced different runtime hashes'
    cat >"$runtime_json" <<'JSON'
{"Id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","Name":"/fixture","Config":{"Env":["A=1"],"Tty":false},"HostConfig":{"OomKillDisable":true,"SecurityOpt":[],"LogConfig":{"Type":"json-file","Config":{}}},"Mounts":[],"NetworkSettings":{"Networks":{}}}
JSON
    true_oom_runtime_hash="$(capture_runtime_contract_hash fixture)"
    [[ "$true_oom_runtime_hash" != "$false_oom_runtime_hash" ]] ||
      fail 'true OomKillDisable value was hidden by runtime normalization'
  )
else
  printf 'subnexus duplicate environment fixtures skipped (requires usable python3)\n'
fi

# ---------------------------------------------------------------------------
# Fixture 4: rollout snapshot validation, gate closure, and restore SQL
# ---------------------------------------------------------------------------

if command -v base64 >/dev/null 2>&1 && command -v python3 >/dev/null 2>&1 &&
   python3 -c 'import json' >/dev/null 2>&1; then
  settings_source="$(
    extract_function base64_text
    extract_function hash_file
    extract_function validate_settings_snapshot
    extract_function validate_closed_settings_snapshot
    extract_function write_closed_settings_snapshot
    extract_function assert_settings_snapshot_integrity
    extract_function assert_rollout_settings_integrity
    extract_function assert_settings_snapshot_matches_file
  )"
  close_source="$(extract_function close_rollout_gates)"
  restore_settings_source="$(extract_function restore_rollout_gates)"
  (
    set -Eeuo pipefail
    eval "$settings_source"
    eval "$close_source"
    eval "$restore_settings_source"
    fail() { exit 77; }
    assert_root_owned_regular() { :; }
    read_one_line() { tr -d '\r\n' < "$1"; }
    valid_sha64() { [[ "${1:-}" =~ ^[0-9a-f]{64}$ ]]; }
    manifest_value() { awk -F= -v wanted="$1" '$1 == wanted {sub(/^[^=]*=/, ""); print; exit}' "$manifest_file"; }
    rollout_keys=(
      registration_ip_cooldown_enabled subnexus_activity_center_enabled
      subnexus_checkin_enabled subnexus_leaderboard_enabled subnexus_marquee_enabled
      subnexus_invite_activities_enabled subnexus_invite_rewards_enabled
      subnexus_first_recharge_enabled battle_pass_enabled
      subnexus_student_recharge_benefit_enabled subnexus_invoice_enabled
      channel_monitor_enabled channel_monitor_mode subnexus_customer_support_enabled
      customer_support_enabled
    )
    rollout_content_keys=(subnexus_customer_support_content customer_support_content)
    invitation_config_key='subnexus_invite_activities_config'
    fixture_root="$(mktemp -d "${TMPDIR:-/tmp}/subnexus-cutover-settings.XXXXXX")"
    trap 'rm -rf -- "$fixture_root"' EXIT
    b64() { printf '%s' "$1" | base64 | tr -d '\r\n'; }
    valid_file="$fixture_root/valid.tsv"
    {
      printf 'registration_ip_cooldown_enabled\t%s\n' "$(b64 false)"
      printf 'channel_monitor_mode\t%s\n' "$(b64 v1)"
      printf 'subnexus_customer_support_content\t%s\n' "$(b64 'support text / preserved')"
      printf 'subnexus_invite_activities_config\t%s\n' "$(b64 '{"enabled":false,"invite_lottery_enabled":false}')"
    } > "$valid_file"
    validate_settings_snapshot "$valid_file"
    printf 'registration_ip_cooldown_enabled\t%s\nregistration_ip_cooldown_enabled\t%s\n' "$(b64 false)" "$(b64 false)" > "$fixture_root/duplicate.tsv"
    if (validate_settings_snapshot "$fixture_root/duplicate.tsv" >/dev/null 2>&1); then
      fail 'duplicate rollout key was accepted'
    fi
    printf 'unexpected_key\t%s\n' "$(b64 false)" > "$fixture_root/unexpected.tsv"
    if (validate_settings_snapshot "$fixture_root/unexpected.tsv" >/dev/null 2>&1); then
      fail 'unexpected rollout key was accepted'
    fi

    run_dir="$fixture_root/run"
    mkdir -- "$run_dir"
    printf 'registration_ip_cooldown_enabled\t%s\nsubnexus_customer_support_content\t%s\n' \
      "$(b64 true)" "$(b64 'historical content; do not interpolate')" > "$run_dir/settings-before.tsv"
    before_hash="$(sha256sum "$run_dir/settings-before.tsv" | awk '{print $1}')"
    printf '%s\n' "$before_hash" > "$run_dir/settings-before.tsv.sha256"
    write_closed_settings_snapshot
    closed_hash="$(sha256sum "$run_dir/settings-closed.tsv" | awk '{print $1}')"
    manifest_file="$fixture_root/manifest.env"
    {
      printf 'settings_snapshot_sha256=%s\n' "$before_hash"
      printf 'settings_closed_snapshot_sha256=%s\n' "$closed_hash"
    } > "$manifest_file"
    close_sql="$fixture_root/close.sql"
    db_psql_file() { cp -- "$1" "$close_sql"; }
    close_rollout_gates
    grep -Fq 'BEGIN ISOLATION LEVEL SERIALIZABLE;' "$close_sql" || fail 'gate closure is not serializable'
    grep -Fq 'LOCK TABLE settings IN SHARE ROW EXCLUSIVE MODE;' "$close_sql" || fail 'gate closure did not lock settings'
    grep -Fq 'rollout setting changed after prepare' "$close_sql" || fail 'gate closure omitted prepared-state CAS assertion'
    grep -Fq "decode('$(b64 v1)'" "$close_sql" || fail 'gate closure omitted mode reset'
    grep -Fq "decode('$(b64 false)'" "$close_sql" || fail 'gate closure omitted false gate values'
    grep -Fq "decode('$(b64 '{"enabled":false,"invite_lottery_enabled":false,"recharge_wheel_enabled":false,"invite_milestone_enabled":false}')'" "$close_sql" || fail 'gate closure omitted disabled invite config'
    ! grep -Fq 'historical content; do not interpolate' "$close_sql" || fail 'gate closure interpolated raw content'
    grep -Fq 'COMMIT;' "$close_sql" || fail 'gate closure is not transactional'

    restore_sql="$fixture_root/restore.sql"
    db_psql_file() { cp -- "$1" "$restore_sql"; }
    restore_rollout_gates
    grep -Fq 'BEGIN ISOLATION LEVEL SERIALIZABLE;' "$restore_sql" || fail 'restore SQL is not serializable'
    grep -Fq 'LOCK TABLE settings IN SHARE ROW EXCLUSIVE MODE;' "$restore_sql" || fail 'restore SQL did not lock settings'
    grep -Fq 'rollout setting CAS mismatch' "$restore_sql" || fail 'restore SQL omitted CAS assertion'
    grep -Fq "decode('$(b64 true)'" "$restore_sql" || fail 'restore SQL omitted boolean snapshot'
    grep -Fq "decode('$(b64 'historical content; do not interpolate')'" "$restore_sql" || fail 'restore SQL omitted content snapshot'
    grep -Fq "DELETE FROM settings WHERE key='channel_monitor_mode';" "$restore_sql" || fail 'restore SQL did not remove absent keys'
    ! grep -Fq 'historical content; do not interpolate' "$restore_sql" || fail 'restore SQL interpolated raw content'
  )
else
  printf 'subnexus rollout settings fixtures skipped (requires base64 and usable python3)\n'
fi

# ---------------------------------------------------------------------------
# Fixture 5: healthcheck/ulimit/resource/network-alias argument contracts
# ---------------------------------------------------------------------------

if command -v python3 >/dev/null 2>&1 && python3 -c 'import json' >/dev/null 2>&1; then
  metadata_source="$(
    extract_function read_one_line
    extract_function candidate_restart_arg
    extract_function append_resource_args
    extract_function append_security_opt_args
    extract_function append_ulimit_args
    extract_function append_healthcheck_args
    extract_function append_network_alias_args
  )"
  (
    set -Eeuo pipefail
    eval "$metadata_source"
    fail() { exit 77; }
    fixture_root="$(mktemp -d "${TMPDIR:-/tmp}/subnexus-cutover-meta.XXXXXX")"
    trap 'rm -rf -- "$fixture_root"' EXIT
    run_dir="$fixture_root/run"
    mkdir -- "$run_dir"
    printf 'on-failure\n' > "$run_dir/restart-policy.txt"
    printf '3\n' > "$run_dir/restart-retries.txt"
    [[ "$(candidate_restart_arg)" == 'on-failure:3' ]] || fail 'restart policy was not reproduced'
    printf '1048576|2097152|0|1000000|1024|0|100000|1-2|128\n' > "$run_dir/resource-policy.txt"
    printf 'nofile|1024|2048\n' > "$run_dir/ulimits.txt"
    printf 'no-new-privileges:true\n' > "$run_dir/security-opt.txt"
    printf '{"Test":["CMD-SHELL","curl -fsS http://127.0.0.1:8080/health"],"Interval":100000000,"Retries":3}\n' > "$run_dir/healthcheck.json"
    printf 'net0|app-alias\nnet1|second-alias\n' > "$run_dir/network-aliases.txt"
    app_networks=(net0 net1)
    args=()
    append_resource_args args "$run_dir/resource-policy.txt"
    append_security_opt_args args
    append_ulimit_args args
    append_healthcheck_args args
    append_network_alias_args args
    args_text="$(printf '%s\n' "${args[@]}")"
    [[ "$args_text" == *'--memory'* && "$args_text" == *'--pids-limit'* ]] || fail 'resource arguments were not reproduced'
    [[ "$args_text" == *'--ulimit'* && "$args_text" == *'nofile=1024:2048'* ]] || fail 'ulimit argument was not reproduced'
    [[ "$args_text" == *'--health-cmd'* && "$args_text" == *'--health-interval'* ]] || fail 'healthcheck arguments were not reproduced'
    [[ "$args_text" == *'--network-alias'* && "$args_text" == *'app-alias'* && "$args_text" != *'second-alias'* ]] || fail 'first-network alias contract was not enforced'
    printf 'no-new-privileges:false\n' > "$run_dir/security-opt.txt"
    if (append_security_opt_args args >/dev/null 2>&1); then
      fail 'explicit no-new-privileges=false was accepted'
    fi
  )
else
  printf 'subnexus runtime metadata fixtures skipped (requires usable python3)\n'
fi

# Fixture 5b: Docker 29 legacy HostConfig.Binds is accepted only when it
# exactly agrees with a structured Mounts bind; unsupported options remain
# rejected. This fixture is Linux-only because the subject invokes python3.
if command -v python3 >/dev/null 2>&1 && python3 -c 'import json' >/dev/null 2>&1; then
  runtime_supported_source="$(extract_function validate_runtime_contract_supported)"
  [[ "$runtime_supported_source" == *'validate_runtime_contract_supported() {'* ]] || fail 'runtime contract helper source was not found'
  (
    set -Eeuo pipefail
    eval "$runtime_supported_source"
    fail() { printf 'runtime fixture failure: %s\n' "$*" >&2; exit 77; }
    fixture_root="$(mktemp -d "${TMPDIR:-/tmp}/subnexus-cutover-bind.XXXXXX")"
    trap 'rm -rf -- "$fixture_root"' EXIT
    fixture_json="$fixture_root/inspect.json"
    cat >"$fixture_json" <<'JSON'
{"Id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","Config":{"Tty":false},"HostConfig":{"Binds":["/srv/subnexus-migration/runtime/subnexus-data:/app/data:rw"],"ConsoleSize":[49,202],"LogConfig":{"Type":"json-file","Config":{"max-file":"5","max-size":"20m"}}},"Mounts":[{"Type":"bind","Source":"/srv/subnexus-migration/runtime/subnexus-data","Destination":"/app/data","Mode":"rw","RW":true,"Propagation":"rprivate"}],"NetworkSettings":{"Networks":{"sub2api-net":{"NetworkID":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}}}}
JSON
    app_id='fixture-app'
    docker_rpc() { [[ "$1" == inspect ]] || return 98; cat "$fixture_json"; }
    validate_runtime_contract_supported
    FIXTURE_JSON="$fixture_json" python3 - <<'PY'
import json
import os
from pathlib import Path
path = Path(os.environ["FIXTURE_JSON"])
data = json.loads(path.read_text())
data["HostConfig"]["OomKillDisable"] = True
path.write_text(json.dumps(data))
PY
    if (
      fail() { return 77; }
      validate_runtime_contract_supported >/dev/null 2>&1
    ); then
      fail 'OomKillDisable=true was accepted'
    fi
    FIXTURE_JSON="$fixture_json" python3 - <<'PY'
import json
import os
from pathlib import Path
path = Path(os.environ["FIXTURE_JSON"])
data = json.loads(path.read_text())
data["HostConfig"]["OomKillDisable"] = False
path.write_text(json.dumps(data))
PY
    validate_runtime_contract_supported
    FIXTURE_JSON="$fixture_json" python3 - <<'PY'
import json
import os
from pathlib import Path
path = Path(os.environ["FIXTURE_JSON"])
data = json.loads(path.read_text())
data["HostConfig"]["Binds"] = ["/srv/subnexus-migration/runtime/subnexus-data:/app/data:z"]
path.write_text(json.dumps(data))
PY
    # The production helper calls the caller-provided `fail` hook on a
    # rejected contract.  Use a returning hook in this expected-failure
    # subshell so the assertion can observe the non-zero status instead of
    # terminating the whole fixture early.
    if (
      fail() { return 77; }
      validate_runtime_contract_supported >/dev/null 2>&1
    ); then
      fail 'unsupported bind relabel option was accepted'
    fi
    FIXTURE_JSON="$fixture_json" python3 - <<'PY'
import json
import os
from pathlib import Path
path = Path(os.environ["FIXTURE_JSON"])
data = json.loads(path.read_text())
data["HostConfig"]["ConsoleSize"] = [49, -1]
path.write_text(json.dumps(data))
PY
    if (
      fail() { return 77; }
      validate_runtime_contract_supported >/dev/null 2>&1
    ); then
      fail 'negative ConsoleSize dimension was accepted'
    fi
    FIXTURE_JSON="$fixture_json" python3 - <<'PY'
import json
import os
from pathlib import Path
path = Path(os.environ["FIXTURE_JSON"])
data = json.loads(path.read_text())
data["HostConfig"]["ConsoleSize"] = [0, 0]
data["HostConfig"]["LogConfig"]["Config"]["labels"] = "unexpected"
path.write_text(json.dumps(data))
PY
    if (
      fail() { return 77; }
      validate_runtime_contract_supported >/dev/null 2>&1
    ); then
      fail 'unsupported log configuration option was accepted'
    fi
  )
else
  printf 'subnexus HostConfig.Binds fixtures skipped (requires usable python3)\n'
fi

# Fixture 5c: json-file rotation options are reproduced exactly and all
# drivers/options outside the narrow allow-list fail closed.
if command -v python3 >/dev/null 2>&1 && python3 -c 'import json' >/dev/null 2>&1; then
  log_config_source="$(
    extract_function validate_log_config_file
    extract_function append_log_config_args
  )"
  [[ "$log_config_source" == *'validate_log_config_file() {'* &&
     "$log_config_source" == *'append_log_config_args() {'* ]] ||
    fail 'log configuration helper source was not found'
  (
    set -Eeuo pipefail
    eval "$log_config_source"
    fail() { exit 77; }
    assert_root_owned_regular() { :; }
    fixture_root="$(mktemp -d "${TMPDIR:-/tmp}/subnexus-cutover-log.XXXXXX")"
    trap 'rm -rf -- "$fixture_root"' EXIT
    run_dir="$fixture_root/run"
    mkdir -- "$run_dir"
    printf '%s\n' '{"Type":"json-file","Config":{"max-file":"5","max-size":"20m"}}' > "$run_dir/log-config.json"
    args=()
    append_log_config_args args
    args_text="$(printf '%s\n' "${args[@]}")"
    [[ "$args_text" == *'--log-driver'* && "$args_text" == *'json-file'* &&
       "$args_text" == *'--log-opt'* && "$args_text" == *'max-file=5'* &&
       "$args_text" == *'max-size=20m'* ]] ||
      fail 'json-file rotation arguments were not reproduced'
    printf '%s\n' '{"Type":"json-file","Config":{"max-size":"20m"}}' > "$run_dir/log-config.json"
    if (validate_log_config_file "$run_dir/log-config.json" >/dev/null 2>&1); then
      fail 'partial json-file rotation configuration was accepted'
    fi
    printf '%s\n' '{"Type":"syslog","Config":{}}' > "$run_dir/log-config.json"
    if (validate_log_config_file "$run_dir/log-config.json" >/dev/null 2>&1); then
      fail 'unsupported log driver was accepted'
    fi
    printf '%s\n' '{"Type":"json-file","Config":{"max-file":"5","max-size":"20m"},"Unexpected":true}' > "$run_dir/log-config.json"
    if (validate_log_config_file "$run_dir/log-config.json" >/dev/null 2>&1); then
      fail 'unsupported log configuration field was accepted'
    fi
    printf '%s\n' '{"Type":"json-file","Config":{"max-file":"5","labels":"secret"}}' > "$run_dir/log-config.json"
    if (validate_log_config_file "$run_dir/log-config.json" >/dev/null 2>&1); then
      fail 'unsupported json-file option was accepted'
    fi
  )
else
  printf 'subnexus log configuration fixtures skipped (requires usable python3)\n'
fi

# Fixture 5d: online application-data backup excludes only the reviewed active
# log glob.  A writer keeps the active log changing for the whole archive; the
# stable data and compressed historical log must still be present.
if [[ "$OSTYPE" == linux* ]] &&
   command -v tar >/dev/null 2>&1 && command -v gzip >/dev/null 2>&1 &&
   command -v sha256sum >/dev/null 2>&1; then
  archive_source="$(
    extract_function read_one_line
    extract_function valid_sha64
    extract_function hash_file
    extract_function write_application_data_archive_policy
    extract_function validate_application_data_archive_policy
    extract_function backup_application_data
  )"
  [[ "$archive_source" == *'backup_application_data() {'* &&
     "$archive_source" == *'validate_application_data_archive_policy() {'* ]] ||
    fail 'application data archive helper source was not found'
  (
    set -Eeuo pipefail
    eval "$archive_source"
    fail() { printf 'application data archive fixture failure: %s\n' "$*" >&2; exit 77; }
    assert_root_owned_regular() { :; }
    assert_backup_within_budget() {
      local path="$1" budget="$2" label="$3" size
      size="$(stat -c '%s' -- "$path")" || fail "cannot inspect $label size"
      (( size > 0 && size <= budget )) || fail "$label exceeded fixture budget"
    }
    app_data_archive_policy_version='exclude-active-logs-v1'
    app_data_archive_exclusion_pattern='./logs/*.log'
    app_data_archive_budget_bytes=536870912
    fixture_root="$(mktemp -d "${TMPDIR:-/tmp}/subnexus-cutover-archive.XXXXXX")"
    writer_pid=''
    cleanup_archive_fixture() {
      if [[ -n "$writer_pid" ]]; then
        kill "$writer_pid" 2>/dev/null || true
        wait "$writer_pid" 2>/dev/null || true
      fi
      rm -rf -- "$fixture_root"
    }
    trap cleanup_archive_fixture EXIT
    source="$fixture_root/source"
    run_dir="$fixture_root/run"
    mkdir -p -- "$source/logs/history" "$run_dir"
    printf 'stable application data\n' > "$source/config.txt"
    printf 'compressed historical log\n' | gzip -c > "$source/logs/history/2026-09-03.log.gz"
    active="$source/logs/sub2api.log"
    printf 'initial active log\n' > "$active"
    write_application_data_archive_policy
    (
      while :; do
        printf 'active log update %s\n' "$RANDOM" >> "$active"
        sleep 0.001
      done
    ) &
    writer_pid=$!
    backup_application_data "$source"
    [[ -s "$active" ]] || fail 'active log writer did not run'
    kill "$writer_pid" 2>/dev/null || true
    wait "$writer_pid" 2>/dev/null || true
    writer_pid=''
    listing="$fixture_root/listing.txt"
    tar -tzf "$run_dir/application-data.tar.gz" > "$listing"
    grep -Fqx './config.txt' "$listing" || fail 'stable application data was omitted'
    grep -Fqx './logs/history/2026-09-03.log.gz' "$listing" || fail 'compressed historical log was omitted'
    if grep -Fqx './logs/sub2api.log' "$listing"; then
      fail 'active log was included despite the fixed exclusion policy'
    fi
    [[ "$(tar -xOzf "$run_dir/application-data.tar.gz" ./config.txt)" == 'stable application data' ]] ||
      fail 'stable application data content changed'
    validate_application_data_archive_policy
    printf 'exclude=./logs/*.log\n' > "$run_dir/application-data-exclusions.txt"
    if (validate_application_data_archive_policy >/dev/null 2>&1); then
      fail 'truncated application data archive policy was accepted'
    fi
    {
      printf 'policy=%s\n' "$app_data_archive_policy_version"
      printf 'exclude=%s\n' "$app_data_archive_exclusion_pattern"
      printf 'retain=./logs/*.gz and all other application data\n'
      printf 'reason=live application log files are excluded because they may change while the online backup runs\n'
    } > "$run_dir/application-data-exclusions.txt"
    hash_file "$run_dir/application-data-exclusions.txt" > "$run_dir/application-data-exclusions.txt.sha256"
    validate_application_data_archive_policy
    [[ -s "$run_dir/application-data.tar.gz.sha256" &&
       "$(cat "$run_dir/application-data.tar.gz.sha256")" == "$(hash_file "$run_dir/application-data.tar.gz")" ]] ||
      fail 'application data archive hash sidecar is invalid'
  )
else
  printf 'subnexus application-data archive fixture skipped (requires Linux tar/gzip)\n'
fi

# ---------------------------------------------------------------------------
# Fixture 6: rollback orchestration keeps the old app and never restores DB
# ---------------------------------------------------------------------------

manifest_source="$(
  extract_function manifest_value
  extract_function manifest_has_key
  extract_function manifest_set
)"
rollback_helpers="$(
  extract_function valid_container_ref
  extract_function valid_sha64
  extract_function read_one_line
  extract_function app_data_owner_mode_is_safe
  extract_function validate_app_data_owner_inputs
  extract_function validate_app_data_owner_runtime_inputs
  extract_function validate_app_data_owner_manifest
  extract_function write_run_marker
  extract_function inspect_container_id_or_empty
  extract_function assert_candidate_container_identity
  extract_function remove_exact_candidate
  extract_function rollback_run
)"
(
  set -Eeuo pipefail
  eval "$manifest_source"
  eval "$rollback_helpers"
  fail() { exit 77; }
  log() { :; }
  # These guards are verified by the static ordering assertions above; the
  # rollback orchestration fixture supplies no-op implementations so it can
  # focus on exact candidate removal and old-container/settings ordering.
  assert_daemon_still_matches_prepare() { :; }
  assert_app_data_source_identity() { :; }
  assert_candidate_container_identity() { [[ "$1" != "$old_id" ]] || fail 'restored old container was treated as the candidate'; }
  assert_root_owned_regular() { :; }
  settings_snapshot_matches_file() { [[ "$1" == "$run_dir/settings-closed.tsv" ]]; }
  fixture_root="$(mktemp -d "${TMPDIR:-/tmp}/subnexus-cutover-rollback.XXXXXX")"
  trap 'rm -rf -- "$fixture_root"' EXIT
  run_dir="$fixture_root/run"
  mkdir -- "$run_dir"
  manifest_file="$run_dir/manifest.env"
  manifest_probe="$fixture_root/manifest-probe.env"
  printf 'state=switched\n' > "$manifest_probe"
  manifest_has_key state "$manifest_probe" || fail 'manifest key presence was not detected'
  if manifest_has_key environment_duplicate_mode "$manifest_probe"; then
    fail 'missing manifest key was reported as present'
  fi
  printf 'environment_duplicate_mode=\n' >> "$manifest_probe"
  manifest_has_key environment_duplicate_mode "$manifest_probe" || fail 'empty manifest key presence was not detected'
  old_id="$(printf 'a%.0s' {1..64})"
  candidate_fixture="$(printf 'b%.0s' {1..64})"
  db_fixture="$(printf 'c%.0s' {1..64})"
  redis_fixture="$(printf 'd%.0s' {1..64})"
  target_fixture="$(printf 'e%.0s' {1..40})"
  {
    printf 'tool=subnexus-production-cutover-v1\n'
    printf 'state=switched\n'
    printf 'run_id=20260904000000-1\n'
    printf 'target_sha=%s\n' "$target_fixture"
    printf 'live_app_id=%s\n' "$old_id"
    printf 'live_app_name=subnexus-cutover\n'
    printf 'database_id=%s\n' "$db_fixture"
    printf 'redis_id=%s\n' "$redis_fixture"
    printf 'candidate_container_id=%s\n' "$candidate_fixture"
    printf 'candidate_container_name=subnexus-cutover\n'
    printf 'candidate_container_intent=%s\n' "$(printf intent | sha256sum | awk '{print $1}')"
    printf 'settings_closed_snapshot_sha256=%s\n' "$(printf closed | sha256sum | awk '{print $1}')"
    printf 'preserved_container=subnexus-cutover-pre-old\n'
  } > "$manifest_file"
  candidate_present=true
  candidate_running=true
  settings_restored=0
  db_restore_called=0
  restore_preserved_container() { old_restored=$((old_restored + 1)); }
  assert_dependencies_still_match() { dependencies_checked=1; }
  restore_rollout_gates() { settings_restored=$((settings_restored + 1)); }
  verify_rollout_gates_restored() { settings_verified=1; }
  db_psql_file() { db_restore_called=1; return 1; }
  old_restored=0
  dependencies_checked=0
  settings_verified=0
  candidate_remove_count=0
  docker_calls="$fixture_root/docker.calls"
  : > "$docker_calls"
  docker_rpc() {
    local op="${1:-}" arg2="${2:-}" arg3="${3:-}" arg4="${4:-}"
    printf '%s|%s|%s|%s\n' "$op" "$arg2" "$arg3" "$arg4" >> "$docker_calls"
    case "$op:$arg2:$arg3:$arg4" in
      inspect:--format:\{\{.Id\}\}:$candidate_fixture)
        if [[ "$candidate_present" == true ]]; then printf '%s\n' "$candidate_fixture"; else printf 'Error: No such object: %s\n' "$candidate_fixture" >&2; return 1; fi ;;
      inspect:--format:\{\{.Id\}\}:subnexus-cutover)
        [[ "$old_restored" -gt 0 ]] && printf '%s\n' "$old_id" || { printf 'Error: No such object: subnexus-cutover\n' >&2; return 1; } ;;
      inspect:--format:\{\{.State.Running\}\}:$candidate_fixture)
        [[ "$candidate_running" == true ]] && printf 'true\n' || printf 'false\n' ;;
      stop:--time:*) candidate_running=false ;;
      container:rm:--force:$candidate_fixture) candidate_present=false; candidate_remove_count=$((candidate_remove_count + 1)) ;;
      container:rm:--force:$old_id) fail 'rollback attempted to remove the restored old container' ;;
      *) return 0 ;;
    esac
  }
  stop_timeout_seconds=5
  rollback_health_timeout_seconds=30
  rollback_run 1
  [[ "$(manifest_value state)" == rolled_back ]] || fail 'rollback did not persist rolled_back state'
  [[ -f "$run_dir/ROLLED_BACK" ]] || fail 'rollback marker was not written'
  [[ "$candidate_present" == false ]] || fail 'candidate container was not removed'
  [[ "$old_restored" == 1 ]] || fail 'old application was not restored'
  [[ "$dependencies_checked" == 1 && "$settings_restored" == 1 && "$settings_verified" == 1 ]] || fail 'rollback ordering/verification was incomplete'
  [[ "$db_restore_called" == 0 ]] || fail 'rollback attempted a database restore'
  rollback_run 1
  [[ "$(manifest_value state)" == rolled_back ]] || fail 'repeated rollback did not preserve rolled_back state'
  [[ "$candidate_remove_count" == 1 ]] || fail 'repeated rollback attempted to remove the restored old container'
  [[ "$old_restored" == 2 ]] || fail 'repeated rollback did not revalidate the restored old application'
  if grep -Eiq 'database|redis|pg_restore|rdb|volume' "$docker_calls"; then
    fail 'rollback touched a database/Redis/volume Docker object'
  fi
)

# An interrupted run may reach `switching` before gate closure.  When the
# database still matches the original snapshot, rollback must restore the old
# container but issue no settings writes.  An unknown database state must fail
# closed instead of guessing which snapshot to apply.
(
  set -Eeuo pipefail
  eval "$manifest_source"
  eval "$rollback_helpers"
  fail() { exit 77; }
  log() { :; }
  assert_daemon_still_matches_prepare() { :; }
  assert_app_data_source_identity() { :; }
  assert_candidate_container_identity() { :; }
  assert_root_owned_regular() { :; }
  assert_dependencies_still_match() { :; }
  restore_preserved_container() { old_restored=$((old_restored + 1)); }
  verify_rollout_gates_restored() { settings_verified=$((settings_verified + 1)); }
  restore_rollout_gates() { settings_restored=$((settings_restored + 1)); }
  fixture_root="$(mktemp -d "${TMPDIR:-/tmp}/subnexus-cutover-early-rollback.XXXXXX")"
  trap 'rm -rf -- "$fixture_root"' EXIT
  run_dir="$fixture_root/run"
  mkdir -- "$run_dir"
  manifest_file="$run_dir/manifest.env"
  closed_fixture="$(printf closed | sha256sum | awk '{print $1}')"
  target_fixture="$(printf 'e%.0s' {1..40})"
  {
    printf 'tool=subnexus-production-cutover-v1\n'
    printf 'state=switching\n'
    printf 'run_id=20260904000000-2\n'
    printf 'target_sha=%s\n' "$target_fixture"
    printf 'live_app_id=%s\n' "$(printf 'a%.0s' {1..64})"
    printf 'live_app_name=subnexus-cutover\n'
    printf 'database_id=%s\n' "$(printf 'c%.0s' {1..64})"
    printf 'redis_id=%s\n' "$(printf 'd%.0s' {1..64})"
    printf 'settings_closed_snapshot_sha256=%s\n' "$closed_fixture"
    printf 'preserved_container=subnexus-cutover-pre-old\n'
  } > "$manifest_file"
  candidate_id=''
  old_restored=0
  settings_restored=0
  settings_verified=0
  snapshot_probe='before'
  settings_snapshot_matches_file() {
    [[ "$snapshot_probe" == 'closed' && "$1" == "$run_dir/settings-closed.tsv" ]] ||
      [[ "$snapshot_probe" == 'before' && "$1" == "$run_dir/settings-before.tsv" ]]
  }
  rollback_run 1
  [[ "$old_restored" == 1 ]] || fail 'early rollback did not restore old application'
  [[ "$settings_restored" == 0 ]] || fail 'early rollback wrote closed settings over original state'
  [[ "$settings_verified" == 1 ]] || fail 'early rollback did not verify original settings state'
  [[ "$(manifest_value state)" == rolled_back ]] || fail 'early rollback did not finish successfully'

  rollback_run 1
  [[ "$old_restored" == 2 ]] || fail 'rolled-back state was not repeatable'
  [[ "$settings_restored" == 0 && "$settings_verified" == 2 ]] || fail 'repeated rollback changed original settings'
  [[ "$(manifest_value state)" == rolled_back ]] || fail 'repeated rollback did not preserve the terminal state'

  # Recreate a switching manifest and make both snapshot probes fail.  The
  # rollback must stop before marking the run complete.
  sed -i 's/^state=.*/state=switching/' "$manifest_file"
  snapshot_probe='unknown'
  if (rollback_run 1 >/dev/null 2>&1); then
    fail 'rollback accepted an unknown settings state'
  fi
)

# Automatic rollback must be armed only after the old container has been
# renamed and must call the same rollback_run path on a failure.
auto_source="$(extract_function rollback_after_failure)"
error_source="$(extract_function on_error)"
assert_contains 'rollback_run 1' <(printf '%s\n' "$auto_source")
assert_contains 'rollback_after_failure' <(printf '%s\n' "$error_source")
set -Eeuo pipefail
eval "$auto_source"
tool_name='subnexus-production-cutover-v1'
auto_fixture_root="$(mktemp -d "${TMPDIR:-/tmp}/subnexus-cutover-auto.XXXXXX")"
trap 'rm -rf -- "$auto_fixture_root"' EXIT
touch "$auto_fixture_root/manifest.env"
auto_marker="$auto_fixture_root/called"
cutover_active=1
rollback_active=0
run_dir="$auto_fixture_root"
manifest_file="$auto_fixture_root/manifest.env"
rollback_run() { printf '%s\n' "$1" > "$auto_marker"; }
rollback_after_failure 42
[[ "$(cat "$auto_marker")" == 1 ]] || fail 'automatic rollback did not invoke rollback_run'

printf 'subnexus production cutover static and fault-fixture tests passed\n'
