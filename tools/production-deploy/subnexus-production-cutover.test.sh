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

for function_name in init_docker acquire_lock prepare_run validate_run_directory \
  validate_environment_file validate_settings_snapshot capture_settings_snapshot close_rollout_gates \
  restore_rollout_gates restore_preserved_container rollback_run switch_run \
  rollback_entry write_run_marker assert_run_marker initialize_prepare_backup_budgets \
  assert_prepare_disk_budget assert_backup_within_budget validate_log_config_file append_log_config_args; do
  assert_contains "$function_name() {"
done

for marker in \
  'for override in DOCKER_HOST DOCKER_CONTEXT DOCKER_CONFIG DOCKER_TLS_VERIFY DOCKER_CERT_PATH DOCKER_API_VERSION' \
  'validate_docker_timeout' \
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
  'database backup was not restored'; do
  assert_contains "$marker"
done
assert_contains 'raw_binds = host.get("Binds") or []'
assert_contains 'HostConfig.Binds.unmatched'
assert_contains 'contract_bind_mounts = []'
assert_contains 'contract["HostConfig"]["Binds"] = contract_bind_mounts'
assert_contains 'console_size = host.get("ConsoleSize")'
assert_contains 'contract["HostConfig"]["ConsoleSize"] = [0, 0]'
assert_contains 'allowed_log_options = {"max-file", "max-size"}'
assert_contains 'HostConfig.LogConfig.Config.required'
assert_contains 'log_config_sha256=%s'
assert_contains 'args+=(--log-driver "$key")'
assert_contains 'args+=(--log-opt "$key=$value")'

for forbidden in \
  'docker pull' 'docker build' 'docker push' 'docker compose' \
  'ssh ' 'scp ' 'docker system prune' 'docker container prune' \
  'docker network prune' 'docker volume prune' 'docker image prune' \
  'docker volume rm' '--network host' '--network container:'; do
  assert_not_contains "$forbidden"
done

prepare_source="$(extract_function prepare_run)"
switch_source="$(extract_function switch_run)"
rollback_source="$(extract_function rollback_run)"
[[ "$prepare_source" == *'prepare_run() {'* ]] || fail 'prepare_run source was not found'
[[ "$switch_source" == *'switch_run() {'* ]] || fail 'switch_run source was not found'
[[ "$rollback_source" == *'rollback_run() {'* ]] || fail 'rollback_run source was not found'

assert_before_text "$prepare_source" 'require_commands' 'init_docker'
assert_before_text "$prepare_source" 'init_docker' 'validate_source_tree'
assert_before_text "$prepare_source" 'capture_settings_snapshot "$run_dir/settings-before.tsv"' 'write_closed_settings_snapshot'
assert_before_text "$prepare_source" 'write_closed_settings_snapshot' 'write_initial_manifest'
assert_before_text "$prepare_source" 'initialize_prepare_backup_budgets "$app_data_source"' 'backup_postgresql'
assert_before_text "$prepare_source" 'assert_prepare_disk_budget before_postgresql' 'backup_postgresql'
assert_before_text "$prepare_source" 'assert_prepare_disk_budget before_redis' 'backup_redis'
assert_before_text "$prepare_source" 'assert_prepare_disk_budget before_application_data' 'backup_application_data "$app_data_source"'
assert_before_text "$prepare_source" 'assert_prepare_disk_budget before_image_load' 'load_and_validate_candidate_image'
assert_before_text "$prepare_source" 'load_and_validate_candidate_image' 'assert_prepare_disk_budget after_image_load'
assert_not_contains 'docker_rpc stop' <(printf '%s\n' "$prepare_source")
assert_not_contains 'docker_rpc rename' <(printf '%s\n' "$prepare_source")

assert_before_text "$switch_source" 'assert_runtime_still_matches_prepare' 'docker_rpc stop --time'
assert_before_text "$switch_source" 'docker_rpc stop --time' 'docker_rpc rename "$app_id" "$preserved_name"'
assert_before_text "$switch_source" 'docker_rpc rename "$app_id" "$preserved_name"' 'close_rollout_gates'
assert_before_text "$switch_source" 'create_candidate_container' 'docker_rpc start "$candidate_id"'
assert_before_text "$switch_source" 'docker_rpc start "$candidate_id"' 'validate_candidate_runtime'
assert_not_contains '"sha256:$candidate_id"'
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
