#!/usr/bin/env bash
set -Eeuo pipefail

# Full application refresh which reuses an already retained rollback container.
# The previous live is only staged for failed-switch recovery and is removed
# after a successful commit. No rollback container, image or image tag is made.
readonly retained_controller_sha='19824a87e3e1de5659cb30664750b71c5c10d374f25bda7f52e6524fe477ee65'
readonly retained_ui_sha='8fdfc8253020d61c1e5f0b13b49c340295e0713f552763c66603d32ae949e38d'
readonly retained_base_commit='33a9601c93304f89a67b7845b4e3b10447edcf96'
readonly retained_live_id='b4b66b9ca08f185ecbe2e41fc768ff0f3c2685d9a97a4e51ef095da35aafad24'
readonly retained_live_image='sha256:9342118b00d127deb7fdc1c62fe19a347acf5563a254a3e9541cf309f492506f'
readonly retained_commit='890828afe0f726abb363029f147e04087fed2bca'
readonly retained_id='e389b3b1c4f62fd8d9fb0eb04998b0559d21bf39ae4c1a99bdfc90441def3836'
readonly retained_image='sha256:44e8dcf019338e050756c86aba8d2ecf73390b4d058da2ebf916aba19bea28d9'
readonly retained_name='subnexus-cutover-ui-prior-20260911130706-3232923'
readonly retained_source_anchor='/srv/subnexus-migration/cutover/20260911130706-3232923'
readonly retained_source_manifest_sha='7d54698f11485ce35c538fab46e9844327eaffc3e65dc5d37de3ae43faa86659'
readonly retained_source_script_sha='7bb384a1700c5c24a90c855f8f444fd6ad01fcae3a21b31b01f4ad3d81cf5c9b'
readonly -a retained_metadata_files=(
  manifest.env READY SWITCHED UI_READY FULL_READY candidate-container-id
  settings-before.tsv settings-before.tsv.sha256 settings-closed.tsv settings-closed.tsv.sha256
  container.env environment-duplicates.tsv networks.txt network-identities.txt network-aliases.txt
  security-opt.txt restart-policy.txt restart-retries.txt workdir.txt entrypoint.txt cmd.txt mounts.txt
  app-data-mount.txt app-data-source.identity ports.txt user.txt resource-policy.txt healthcheck.json
  log-config.json ulimits.txt runtime-contract.sha256 database.identity redis.identity
)
retained_target=''
retained_tree=''
retained_candidate_image=''
retained_gate=''
retained_gate_sha=''
retained_backup_sha=''

retained_usage() {
  printf '%s\n' \
    'usage: subnexus-full-release-retained-cutover.sh prepare CONTROLLER UI_LIBRARY SOURCE TARGET TREE IMAGE_ID ARCHIVE ARCHIVE_SHA CANDIDATE_GATE LIVE COMPAT_GATE COMPAT_SHA [PUBLIC_HEALTH_URL]' \
    '       subnexus-full-release-retained-cutover.sh switch|rollback|recover CONTROLLER UI_LIBRARY RUN_DIRECTORY' \
    'Requires SUBNEXUS_APPROVED_RETAINED_RELEASE_SCRIPT_SHA256 and the existing owner/confirmation variables.' >&2
}

retained_bootstrap_file() {
  local path="$1" cursor mode
  [[ "$path" == /* && -f "$path" && ! -L "$path" && "$(realpath -e -P -- "$path")" == "$path" ]] || return 1
  cursor="$path"
  while :; do
    [[ "$(stat -c '%u' -- "$cursor")" == 0 ]] || return 1
    mode="$(stat -c '%a' -- "$cursor")" || return 1
    (( (8#$mode & 0022) == 0 )) || return 1
    [[ "$cursor" != / ]] || break
    cursor="$(dirname -- "$cursor")"
  done
}

retained_validate_gate() {
  local info
  valid_sha40 "$retained_target" && valid_sha40 "$retained_tree" || fail 'retained release commit/tree is invalid'
  [[ "$retained_candidate_image" =~ ^sha256:[0-9a-f]{64}$ ]] || fail 'retained release candidate image is invalid'
  [[ "$retained_candidate_image" != "$retained_live_image" && "$retained_candidate_image" != "$retained_image" ]] || fail 'retained candidate equals an existing image'
  valid_sha64 "$retained_gate_sha" || fail 'retained compatibility SHA is invalid'
  info="$(assert_approved_path "$retained_gate" candidate_gate)" || fail 'retained compatibility path is unapproved'
  retained_gate="${info%%|*}"
  assert_root_owned_regular "$retained_gate" 'retained compatibility evidence'
  [[ "$(hash_file "$retained_gate")" == "$retained_gate_sha" ]] || fail 'retained compatibility evidence changed'
  retained_backup_sha="$(python3 - "$retained_gate" "$retained_target" "$retained_tree" "$retained_candidate_image" "$retained_live_image" "$retained_image" <<'PY'
import re, sys
path, commit, tree, candidate, previous, rollback = sys.argv[1:]
raw = open(path, 'rb').read(65537)
if len(raw) > 65536 or b'\r' in raw or not raw.endswith(b'\n'):
    raise SystemExit('invalid retained compatibility encoding/size')
values = {}
for line in raw.decode('ascii').splitlines():
    key, sep, value = line.partition('=')
    if not sep or not key or key in values:
        raise SystemExit('duplicate/malformed retained compatibility field')
    values[key] = value
expected = {
    'version': 'full-release-retained-compat-v1', 'result': 'passed',
    'candidate_commit': commit, 'candidate_tree': tree, 'candidate_image': candidate,
    'previous_live_image': previous, 'retained_rollback_image': rollback,
    'new_old_new': 'passed', 'new_rollback_new': 'passed',
    'migration_contract': 'passed', 'api_regression': 'passed', 'cleanup': 'passed',
}
if set(values) != set(expected) | {'backup_sha256'} or any(values[k] != v for k, v in expected.items()):
    raise SystemExit('retained compatibility identities/results mismatch')
if not re.fullmatch('[0-9a-f]{64}', values['backup_sha256']):
    raise SystemExit('invalid retained compatibility backup SHA')
print(values['backup_sha256'])
PY
)" || fail 'retained compatibility evidence rejected'
  [[ "$(hash_file "$retained_gate")" == "$retained_gate_sha" ]] || fail 'retained compatibility changed during validation'
}

retained_assert_source() {
  local source="$1" base="$2" target="$3"
  [[ "$base" == "$retained_base_commit" && "$target" == "$retained_target" ]] || fail 'retained source identities changed'
  git -C "$source" merge-base --is-ancestor "$retained_base_commit" "$target" || fail 'candidate does not descend from actual live'
  git -C "$source" merge-base --is-ancestor "$retained_commit" "$target" || fail 'candidate lacks v0.2.4 compatibility baseline'
  [[ "$(git -C "$source" rev-parse HEAD)" == "$target" && "$(git -C "$source" rev-parse "$target^{tree}")" == "$retained_tree" ]] || fail 'candidate source commit/tree drifted'
  [[ -z "$(git -C "$source" status --porcelain=v1 --untracked-files=all)" ]] || fail 'candidate source is dirty'
  [[ -z "$(git -C "$source" symbolic-ref --quiet --short HEAD 2>/dev/null || true)" ]] || fail 'candidate source must be detached'
  retained_validate_gate
}

# Validate either the pinned original run or its byte-identical recovery
# metadata copy. The old executable is unnecessary: the exact manifest SHA
# authenticates its source and the unchanged controller validates its contract.
# All overrides and old-run variables are confined to this subshell.
retained_anchor_context() (
  local anchor="$1" expected_id="$2" expected_image="$3" expected_name="$4" expected_hash="$5" allowed="${6:-stopped}"
  local outer_lock="${evidence_lock_root:-}"
  [[ "$expected_id" == "$retained_id" && "$expected_image" == "$retained_image" && "$expected_name" == "$retained_name" ]] || fail 'retained target identity changed'
  [[ -z "$expected_hash" || "$expected_hash" == "$retained_source_manifest_sha" ]] || fail 'retained source manifest pin changed'
  assert_root_owned_regular "$anchor/manifest.env" 'retained source manifest'
  [[ "$(hash_file "$anchor/manifest.env")" == "$retained_source_manifest_sha" ]] || fail 'retained source manifest changed'
  validate_self_sha() {
    [[ "$1" == "$retained_source_script_sha" ]] || fail 'retained source script identity changed'
    script_sha256="$retained_source_script_sha"
  }
  SUBNEXUS_APPROVED_CUTOVER_SCRIPT_SHA256="$retained_source_script_sha"
  validate_run_directory "$anchor" rollback
  [[ -z "$outer_lock" || "$outer_lock" == "$evidence_lock_root" ]] || fail 'retained target belongs to another lock root'
  [[ "$(manifest_value state)" == switched && "$(manifest_value ui_state)" == switched && "$(manifest_value full_flow)" == full-release-v1 ]] || fail 'retained source is not a completed full release'
  [[ "$(manifest_value candidate_container_id)" == "$retained_live_id" && "$(manifest_value target_sha)" == "$retained_base_commit" ]] || fail 'retained source identifies another live release'
  [[ "$(manifest_value ui_new_rollback_id)" == "$retained_id" && "$(manifest_value ui_new_rollback_image)" == "$retained_image" && "$(manifest_value ui_new_rollback_name)" == "$retained_name" ]] || fail 'retained source rollback identities disagree'
  assert_daemon_still_matches_prepare
  assert_dependencies_still_match
  assert_app_data_source_identity
  assert_prepared_networks_still_match
  ui_assert_new_rollback_contract "$allowed"
  [[ "$(hash_file "$manifest_file")" == "$retained_source_manifest_sha" ]] || fail 'retained metadata changed while validating'
)

retained_copy_contract() {
  local destination="$run_dir/retained-contract" file listing="$run_dir/retained-contract.sha256"
  [[ ! -e "$destination" && ! -L "$destination" ]] || fail 'retained contract destination already exists'
  mkdir -m 700 -- "$destination"
  for file in "${retained_metadata_files[@]}"; do
    assert_root_owned_regular "$retained_source_anchor/$file" "retained contract $file"
    [[ "$(stat -c '%s' "$retained_source_anchor/$file")" -le 1048576 ]] || fail 'retained contract metadata is too large'
    cp -- "$retained_source_anchor/$file" "$destination/$file"
    chmod 600 "$destination/$file"
  done
  ( cd "$destination"; sha256sum -- "${retained_metadata_files[@]}" ) > "$listing"
  chmod 600 "$listing"
  manifest_set retained_contract_sha256 "$(hash_file "$listing")"
  manifest_set retained_source_anchor "$retained_source_anchor"
  manifest_set retained_source_manifest_sha256 "$retained_source_manifest_sha"
  manifest_set ui_anchor_run "$destination"
  retained_assert_contract_copy
  retained_anchor_context "$destination" "$retained_id" "$retained_image" "$retained_name" "$retained_source_manifest_sha" stopped
}

retained_assert_contract_copy() {
  local listing="$run_dir/retained-contract.sha256" contract="$run_dir/retained-contract" expected file
  [[ "$(manifest_value ui_anchor_run)" == "$contract" && "$(manifest_value retained_source_anchor)" == "$retained_source_anchor" && "$(manifest_value retained_source_manifest_sha256)" == "$retained_source_manifest_sha" ]] || fail 'retained contract location/source changed'
  assert_root_owned_dir "$contract" 'retained contract directory'
  assert_root_owned_regular "$listing" 'retained contract hash list'
  expected="$(manifest_value retained_contract_sha256)"
  valid_sha64 "$expected" && [[ "$(hash_file "$listing")" == "$expected" ]] || fail 'retained contract hash list changed'
  for file in "${retained_metadata_files[@]}"; do assert_root_owned_regular "$contract/$file" "retained contract $file"; done
  # Compare a freshly computed fixed allowlist rather than interpreting paths
  # or arguments from the recorded list.
  [[ "$(cd "$contract"; sha256sum -- "${retained_metadata_files[@]}")" == "$(cat "$listing")" ]] || fail 'retained recovery metadata changed'
}

retained_assert_fallback() {
  retained_assert_contract_copy
  retained_anchor_context "$(manifest_value ui_anchor_run)" "$retained_id" "$retained_image" "$retained_name" "$retained_source_manifest_sha" "${1:-stopped}"
}

retained_restore_fallback() (
  local contract="$(manifest_value ui_anchor_run)" outer_lock="$evidence_lock_root"
  retained_assert_fallback any
  validate_self_sha() { [[ "$1" == "$retained_source_script_sha" ]] || fail 'retained script pin changed'; script_sha256="$retained_source_script_sha"; }
  SUBNEXUS_APPROVED_CUTOVER_SCRIPT_SHA256="$retained_source_script_sha"
  validate_run_directory "$contract" rollback
  [[ "$outer_lock" == "$evidence_lock_root" ]] || fail 'retained restore lock changed'
  preserved_name="$retained_name"
  restore_preserved_container || fail 'existing retained rollback target did not recover health'
  ui_assert_new_rollback_contract restored
  [[ "$(hash_file "$manifest_file")" == "$retained_source_manifest_sha" ]] || fail 'retained metadata changed during restore'
)

retained_prepare() {
  local source="$1" target="$2" tree="$3" image="$4" archive="$5" archive_sha="$6" gate="$7" live="$8" compat="$9" compat_sha="${10}" public="${11:-}"
  retained_target="$target"; retained_tree="$tree"; retained_candidate_image="sha256:${image#sha256:}"
  retained_gate="$compat"; retained_gate_sha="$compat_sha"
  require_commands; init_docker
  [[ "$(docker_rpc inspect --format '{{.Id}}' "$live")" == "$retained_live_id" && "$(docker_rpc inspect --format '{{.Image}}' "$live")" == "$retained_live_image" ]] || fail 'actual previous-live identity changed'
  retained_validate_gate
  # The pinned library's ui_new_rollback_* names represent only our temporary
  # current recovery contract. retained_current_policy makes this explicit;
  # they never replace ui_rollback_* (the existing permanent fallback).
  ui_prepare "$source" "$target" "$retained_base_commit" "${image#sha256:}" "$archive" "$archive_sha" "$gate" "$live" "$retained_source_anchor" "$retained_id" "$retained_image" "$retained_name" "$public"
  [[ "$(manifest_value live_app_id)" == "$retained_live_id" && "$(manifest_value live_app_image_id)" == "$retained_live_image" ]] || fail 'live changed during retained prepare'
  manifest_set retained_flow full-release-retained-v1
  manifest_set retained_target_tree "$retained_tree"
  manifest_set retained_controller_sha256 "$retained_controller_sha"
  manifest_set retained_ui_sha256 "$retained_ui_sha"
  manifest_set retained_compat_gate "$retained_gate"
  manifest_set retained_compat_sha256 "$retained_gate_sha"
  manifest_set retained_compat_backup_sha256 "$retained_backup_sha"
  manifest_set retained_current_policy temporary-until-commit
  manifest_set retained_commit_phase none
  retained_copy_contract
  retained_validate_gate
  printf 'full-release-retained-v1\n' > "$run_dir/RETAINED_READY"
  chmod 600 "$run_dir/RETAINED_READY"
  log "RETAINED_RELEASE_PREPARED_RUN=$run_dir"
  log 'No new rollback object was created; the existing stopped v0.2.4 target remains primary.'
}

retained_validate_manifest() {
  local scope="$1" backup
  [[ "$(manifest_value retained_flow)" == full-release-retained-v1 && "$(manifest_value retained_current_policy)" == temporary-until-commit ]] || fail 'run does not belong to retained-target full release'
  assert_run_marker RETAINED_READY full-release-retained-v1
  [[ "$(manifest_value retained_controller_sha256)" == "$retained_controller_sha" && "$(manifest_value retained_ui_sha256)" == "$retained_ui_sha" ]] || fail 'retained library pins changed'
  [[ "$(manifest_value ui_base_sha)" == "$retained_base_commit" && "$(manifest_value live_app_id)" == "$retained_live_id" && "$(manifest_value live_app_image_id)" == "$retained_live_image" ]] || fail 'retained previous-live identities changed'
  [[ "$(manifest_value ui_rollback_id)" == "$retained_id" && "$(manifest_value ui_rollback_image)" == "$retained_image" && "$(manifest_value ui_rollback_name)" == "$retained_name" ]] || fail 'retained rollback identities changed'
  case "$(manifest_value retained_commit_phase)" in none|removing_current|current_removed) ;; *) fail 'invalid retained commit phase' ;; esac
  retained_target="$(manifest_value target_sha)"; retained_tree="$(manifest_value retained_target_tree)"
  retained_candidate_image="sha256:$(manifest_value candidate_image_id)"
  retained_gate="$(manifest_value retained_compat_gate)"; retained_gate_sha="$(manifest_value retained_compat_sha256)"
  backup="$(manifest_value retained_compat_backup_sha256)"
  valid_sha40 "$retained_tree" && valid_sha64 "$retained_gate_sha" && valid_sha64 "$backup" || fail 'malformed retained compatibility metadata'
  retained_assert_contract_copy
  if [[ "$scope" == switch ]]; then
    retained_validate_gate
    [[ "$retained_backup_sha" == "$backup" ]] || fail 'retained compatibility backup changed'
  fi
}

retained_remove_current() {
  local id="$(manifest_value live_app_id)" name="$(manifest_value ui_temporary_name)" observed
  [[ "$id" == "$retained_live_id" && "$id" != "$retained_id" ]] || fail 'temporary removal target changed'
  observed="$(inspect_container_id_or_empty "$id")" || fail 'cannot inspect temporary current before removal'
  [[ -n "$observed" ]] || return 0
  ui_assert_new_rollback_contract stopped
  [[ "$(docker_rpc inspect --format '{{.Name}}' "$id")" == "/$name" ]] || fail 'temporary current name changed'
  assert_daemon_still_matches_prepare
  docker_rpc container rm "$id" >/dev/null || {
    [[ -z "$(inspect_container_id_or_empty "$id")" ]] || fail 'temporary current removal failed'
  }
  [[ -z "$(inspect_container_id_or_empty "$id")" ]] || fail 'temporary current remains after removal'
}

retained_finish_commit() {
  retained_assert_fallback stopped
  [[ "$(manifest_value ui_commit_intent)" == yes ]] || fail 'retained commit lacks durable intent'
  manifest_set retained_commit_phase removing_current
  retained_remove_current
  manifest_set retained_commit_phase current_removed
  write_run_marker SWITCHED switched || fail 'cannot persist retained switch marker'
  manifest_set state switched || fail 'cannot persist retained switched state'
  manifest_set ui_state switched || fail 'cannot persist retained completion'
  cutover_active=0
  log "RETAINED_RELEASE_SWITCH_COMPLETED=$run_dir"
}

retained_recover_current() {
  local current="$(inspect_container_id_or_empty "$(manifest_value live_app_id)")" phase="$(manifest_value retained_commit_phase)" temp="$(manifest_value ui_temporary_name)" current_name
  if [[ -z "$current" ]]; then
    # Deletion is the irreversible commit boundary. A lost Docker response or
    # marker write must keep the healthy candidate, never seek a removed live.
    [[ "$(manifest_value ui_commit_intent)" == yes && ( "$phase" == removing_current || "$phase" == current_removed ) ]] || fail 'temporary current disappeared before commit; retained rollback remains available'
    retained_assert_fallback stopped
    candidate_id="$(manifest_value candidate_container_id)"
    assert_candidate_container_identity "$candidate_id"
    wait_for_candidate_health && validate_candidate_runtime || fail 'committed candidate needs retained rollback'
    retained_finish_commit
    return 0
  fi
  assert_daemon_still_matches_prepare; assert_dependencies_still_match
  current_name="$(docker_rpc inspect --format '{{.Name}}' "$current")" || fail 'cannot inspect temporary recovery name'
  if [[ "$current_name" == "/$temp" ]]; then ui_assert_new_rollback_contract stopped; else ui_assert_new_rollback_contract any; fi
  # Failure recovery uses the current live even if the separate retained target
  # disappeared. It must never discard the candidate to seek a missing current.
  manifest_set state rolling_back
  ui_remove_candidate
  ui_assert_candidate_absent
  preserved_name="$temp"
  restore_preserved_container || fail 'temporary current did not recover health'
  manifest_set ui_new_rollback_state restored
  write_run_marker ROLLED_BACK rolled_back
  manifest_set state rolled_back
  manifest_set ui_state recovered_current
  cutover_active=0
  ui_assert_settings_unchanged
  log "RETAINED_RELEASE_RECOVERED_CURRENT=$current"
}

retained_switch() {
  [[ "${SUBNEXUS_CUTOVER_CONFIRM:-}" == I_UNDERSTAND_SHORT_PRODUCTION_WINDOW ]] || fail 'retained switch needs production-window confirmation'
  ui_load_run "$1" switch
  retained_validate_manifest switch
  [[ "$(manifest_value state)" == prepared && "$(manifest_value ui_state)" == prepared ]] || fail 'retained switch needs a fresh prepared run'
  [[ "${SUBNEXUS_CUTOVER_QUIET_CONFIRM:-}" == I_HAVE_CHECKED_NO_SETTLEMENT_TASKS ]] || fail 'retained switch needs settlement/migration check'
  ui_expected_settings_hash="$(manifest_value ui_settings_sha256)"
  assert_runtime_still_matches_prepare
  ui_assert_new_rollback_contract prepared
  ui_assert_base_image "$retained_base_commit" "$retained_live_image"
  retained_assert_source "$(manifest_value source_root)" "$retained_base_commit" "$target_sha"
  retained_assert_fallback stopped
  ui_assert_settings_unchanged
  [[ "$(docker_rpc image inspect --format '{{.Id}}' "sha256:$expected_image_id")" == "sha256:$expected_image_id" ]] || fail 'retained candidate image is unavailable'
  local temp="$(manifest_value ui_temporary_name)"
  [[ -z "$(inspect_container_id_or_empty "$temp")" ]] || fail 'temporary current name is occupied'
  manifest_set state switching; manifest_set ui_state switching
  cutover_active=1
  trap on_error ERR
  trap 'rollback_after_failure 129; exit 129' HUP
  trap 'rollback_after_failure 130; exit 130' INT
  trap 'rollback_after_failure 143; exit 143' TERM
  docker_rpc stop --time "$stop_timeout_seconds" "$app_id" >/dev/null || fail 'current live did not stop'
  assert_daemon_still_matches_prepare
  [[ "$(docker_rpc inspect --format '{{.State.Running}}' "$app_id")" == false ]] || fail 'current live remains running'
  docker_rpc rename "$app_id" "$temp" || fail 'cannot stage temporary current'
  manifest_set preserved_container "$temp"; manifest_set ui_new_rollback_state stopped
  ui_assert_new_rollback_contract stopped
  retained_assert_fallback stopped
  create_candidate_container
  assert_candidate_container_identity "$candidate_id"; assert_candidate_runtime_contract
  assert_daemon_still_matches_prepare
  docker_rpc start "$candidate_id" >/dev/null || fail 'retained candidate did not start'
  wait_for_candidate_health || fail 'retained candidate failed health checks'
  validate_candidate_runtime
  ui_assert_settings_unchanged
  retained_assert_fallback stopped
  assert_dependencies_still_match
  ui_assert_new_rollback_contract stopped
  ( ulimit -f 32768; docker_rpc logs --tail 5000 "$app_id" > "$run_dir/previous-container.log" 2>&1 ) || fail 'cannot retain previous application diagnostics'
  chmod 600 "$run_dir/previous-container.log"
  manifest_set ui_state committing; manifest_set ui_commit_intent yes
  retained_finish_commit
  trap - ERR HUP INT TERM
}

retained_manual_rollback() {
  [[ "${SUBNEXUS_CUTOVER_CONFIRM:-}" == I_UNDERSTAND_APPLICATION_ROLLBACK ]] || fail 'retained rollback needs application-rollback confirmation'
  ui_load_run "$1" rollback
  retained_validate_manifest rollback
  [[ "$(manifest_value state)" != prepared ]] || fail 'retained rollback is not applicable before switch'
  ui_expected_settings_hash="$(ui_settings_hash)"
  local observed current temp="$(manifest_value ui_temporary_name)" current_name
  retained_assert_fallback any
  observed="$(inspect_container_id_or_empty "$app_name")" || fail 'cannot inspect production before retained rollback'
  if [[ "$observed" == "$retained_id" ]]; then
    ui_assert_candidate_absent
  elif [[ -n "$observed" && "$observed" != "$retained_live_id" ]]; then
    assert_candidate_container_identity "$observed"
  fi
  current="$(inspect_container_id_or_empty "$retained_live_id")" || fail 'cannot inspect temporary current during rollback'
  if [[ -n "$current" ]]; then
    current_name="$(docker_rpc inspect --format '{{.Name}}' "$current")" || fail 'cannot inspect temporary current name'
    if [[ "$current_name" == "/$temp" ]]; then ui_assert_new_rollback_contract stopped; else ui_assert_new_rollback_contract any; fi
  fi
  manifest_set state rolling_back; manifest_set ui_state rolling_back
  ui_remove_candidate; ui_assert_candidate_absent
  if [[ -n "$current" && "$current_name" == "/$app_name" ]]; then
    [[ -z "$(inspect_container_id_or_empty "$temp")" ]] || fail 'temporary current name occupied during rollback'
    docker_rpc stop --time "$stop_timeout_seconds" "$current" >/dev/null || fail 'cannot stop current for retained rollback'
    [[ "$(docker_rpc inspect --format '{{.State.Running}}' "$current")" == false ]] || fail 'current still owns production port'
    docker_rpc rename "$current" "$temp" || fail 'cannot stage current for retained rollback'
  fi
  retained_restore_fallback
  retained_assert_fallback restored
  if [[ -n "$current" ]]; then retained_remove_current; fi
  ui_assert_settings_unchanged
  write_run_marker ROLLED_BACK rolled_back
  manifest_set state rolled_back; manifest_set ui_state rolled_back_to_original
  log "RETAINED_RELEASE_ROLLBACK_COMPLETED=$retained_id"
}

retained_recover_entry() {
  [[ "${SUBNEXUS_CUTOVER_CONFIRM:-}" == I_UNDERSTAND_APPLICATION_ROLLBACK ]] || fail 'retained recover needs rollback confirmation'
  ui_load_run "$1" rollback
  retained_validate_manifest rollback
  case "$(manifest_value ui_state)" in switching|committing|recovered_current) ;; *) fail 'temporary recovery is not applicable; use retained rollback' ;; esac
  ui_expected_settings_hash="$(ui_settings_hash)"
  retained_recover_current
}

retained_install_overrides() {
  ui_assert_source_delta() { retained_assert_source "$@"; }
  ui_anchor_context() { retained_anchor_context "$@"; }
  ui_recover_current() { retained_recover_current; }
  ui_dispatch() {
    local action="$1"; shift
    mode="$action"; [[ "$mode" != recover ]] || mode=rollback
    case "$action" in
      prepare) retained_prepare "$@" ;;
      switch) require_commands; retained_switch "$1" ;;
      rollback) require_commands; retained_manual_rollback "$1" ;;
      recover) require_commands; retained_recover_entry "$1" ;;
      *) retained_usage; return 2 ;;
    esac
  }
}

retained_main() {
  local action="${1:-}" controller="${2:-}" library="${3:-}" entry expected="${SUBNEXUS_APPROVED_RETAINED_RELEASE_SCRIPT_SHA256:-}" copy
  case "$-" in *x*) set +x ;; esac
  unset BASH_ENV ENV CDPATH GLOBIGNORE TAR_OPTIONS GZIP
  export PATH='/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin'
  [[ "$EUID" == 0 && "$#" -ge 3 ]] || { retained_usage; return 2; }
  shift 3
  case "$action" in prepare) [[ "$#" == 10 || "$#" == 11 ]] || { retained_usage; return 2; } ;; switch|rollback|recover) [[ "$#" == 1 ]] || { retained_usage; return 2; } ;; *) retained_usage; return 2 ;; esac
  umask 077
  entry="$(realpath -e -P -- "${BASH_SOURCE[0]}")"
  retained_bootstrap_file "$entry" && retained_bootstrap_file "$controller" && retained_bootstrap_file "$library" || { printf 'Untrusted retained release/library path\n' >&2; return 1; }
  [[ "$expected" =~ ^[0-9a-f]{64}$ && "$(sha256sum "$entry" | awk '{print $1}')" == "$expected" ]] || { printf 'Retained entry SHA mismatch\n' >&2; return 1; }
  [[ "$(sha256sum "$controller" | awk '{print $1}')" == "$retained_controller_sha" ]] || { printf 'Retained controller pin mismatch\n' >&2; return 1; }
  copy="$(mktemp /tmp/subnexus-retained-ui.XXXXXX)" || return 1
  chmod 600 "$copy"
  if ! cp -- "$library" "$copy" || [[ "$(sha256sum "$copy" | awk '{print $1}')" != "$retained_ui_sha" ]]; then rm -f -- "$copy"; printf 'Retained UI library pin mismatch\n' >&2; return 1; fi
  source "$copy"
  rm -f -- "$copy"
  [[ "$ui_controller_sha" == "$retained_controller_sha" ]] || { printf 'Retained library/controller disagreement\n' >&2; return 1; }
  ui_entry_path="$entry"
  SUBNEXUS_APPROVED_UI_CUTOVER_SCRIPT_SHA256="$expected"
  retained_install_overrides
  ui_load_controller "$controller" "$action" "$@"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then retained_main "$@"; fi
