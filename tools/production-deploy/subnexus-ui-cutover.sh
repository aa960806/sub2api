#!/usr/bin/env bash
set -Eeuo pipefail

# Application-only refresh. The migration controller supplies its existing
# runtime/backup checks, but never its switch, gate-writing, or rollback path.
readonly ui_controller_sha='19824a87e3e1de5659cb30664750b71c5c10d374f25bda7f52e6524fe477ee65'
ui_entry_path=''
ui_controller_path=''
ui_anchor_validation=0
ui_expected_settings_hash=''

ui_usage() {
  printf '%s\n' \
    'usage: subnexus-ui-cutover.sh prepare CONTROLLER SOURCE TARGET_SHA BASE_SHA IMAGE_ID ARCHIVE ARCHIVE_SHA GATE LIVE_APP ANCHOR_RUN OLD_ID OLD_IMAGE_ID OLD_NAME [PUBLIC_HEALTH_URL]' \
    '       subnexus-ui-cutover.sh switch|rollback|recover CONTROLLER RUN_DIRECTORY' \
    'Requires SUBNEXUS_APPROVED_UI_CUTOVER_SCRIPT_SHA256 and the usual owner/confirmation variables.' >&2
}

ui_bootstrap_file() {
  local path="$1" resolved cursor mode
  [[ "$path" == /* && -f "$path" && ! -L "$path" ]] || return 1
  resolved="$(realpath -e -P -- "$path")" || return 1
  [[ "$resolved" == "$path" ]] || return 1
  cursor="$path"
  while :; do
    [[ "$(stat -c '%u' -- "$cursor")" == 0 ]] || return 1
    mode="$(stat -c '%a' -- "$cursor")" || return 1
    (( (8#$mode & 0022) == 0 )) || return 1
    [[ "$cursor" != / ]] || break
    cursor="$(dirname -- "$cursor")"
  done
}

ui_load_controller() {
  local controller="$1" full library actual
  shift
  ui_bootstrap_file "$controller" || { printf 'Untrusted controller path\n' >&2; return 1; }
  full="$(mktemp /tmp/subnexus-ui-controller.XXXXXX)" || return 1
  library="$(mktemp /tmp/subnexus-ui-library.XXXXXX)" || { rm -f -- "$full"; return 1; }
  chmod 600 -- "$full" "$library"
  if ! cp -- "$controller" "$full"; then rm -f -- "$full" "$library"; return 1; fi
  actual="$(sha256sum "$full" | awk '{print $1}')"
  if [[ "$actual" != "$ui_controller_sha" || "$(tail -n 1 "$full")" != 'main "$@"' ]]; then
    rm -f -- "$full" "$library"
    printf 'Controller SHA or entrypoint mismatch\n' >&2
    return 1
  fi
  # This exact, pinned file has a single final dispatch line. Keep every
  # definition intact; never regex-extract/eval selected production functions.
  head -n -1 "$full" > "$library"
  ui_controller_path="$controller"
  source "$library"
  rm -f -- "$full" "$library"
  ui_install_overrides
  # The library declares arrays at source scope. Dispatch before this frame
  # returns so those arrays remain available through Bash's dynamic scope.
  ui_dispatch "$@"
}

ui_install_overrides() {
  validate_self_sha() {
    local expected="$1" path actual
    if [[ "$expected" == "$ui_controller_sha" ]]; then path="$ui_controller_path"; else path="$ui_entry_path"; fi
    valid_sha64 "$expected" || fail 'approved script SHA is invalid'
    assert_root_owned_regular "$path" 'approved controller'
    actual="$(hash_file "$path")"
    [[ "$actual" == "$expected" ]] || fail 'approved script SHA mismatch'
    script_path="$path"
    script_sha256="$actual"
  }
  close_rollout_gates() { fail 'UI refresh must not close rollout gates'; }
  restore_rollout_gates() { fail 'UI refresh must not restore historical settings'; }
  db_psql_file() { fail 'UI refresh must not execute SQL files'; }
  verify_rollout_gates_closed() { ui_assert_settings_unchanged; }
  remove_exact_candidate() { ui_remove_candidate; }
  rollback_after_failure() {
    local rc="${1:-1}"
    if [[ "${cutover_active:-0}" == 1 && "${rollback_active:-0}" == 0 && "${BASHPID:-$$}" == "$$" ]]; then
      rollback_active=1
      printf 'UI switch failed (rc=%s); restoring the temporary current container.\n' "$rc" >&2
      ( trap - ERR INT TERM; ui_recover_current ) || printf 'Recovery needs attention; use the UI recover or original-target rollback command.\n' >&2
      rollback_active=0
    fi
  }
  # All helper queries, including backup prechecks, use read-only sessions.
  db_psql() {
    local sql="$1" password
    password="$(db_password_value)"
    { printf '%s\n' "$password"; } |
      docker_rpc exec -i "$database_id" sh -c \
        'IFS= read -r password || exit 1; unset PGHOST PGHOSTADDR PGSERVICE PGSERVICEFILE PGPASSFILE PGDATABASE PGUSER; PGPASSWORD="$password" PGOPTIONS="-c default_transaction_read_only=on -c statement_timeout=30000 -c lock_timeout=5000" PGCONNECT_TIMEOUT=8 exec psql -X -At -v ON_ERROR_STOP=1 -U "$1" -d "$2" -c "$3"' \
        sh "$(db_user_value)" "$(db_name_value)" "$sql"
  }
}

ui_settings_hash() {
  db_psql "SELECT json_build_array(key,value)::text FROM settings ORDER BY key;" | sha256sum | awk '{print $1}'
}

ui_assert_settings_unchanged() {
  local actual
  valid_sha64 "$ui_expected_settings_hash" || fail 'operation settings hash is missing'
  actual="$(ui_settings_hash)" || fail 'cannot read production settings'
  [[ "$actual" == "$ui_expected_settings_hash" ]] || fail 'production settings changed; no settings were restored'
}

ui_assert_source_delta() {
  local source="$1" base="$2" target="$3" path count=0
  valid_sha40 "$base" && valid_sha40 "$target" || fail 'UI source SHA is invalid'
  git -C "$source" cat-file -e "$base^{commit}" || fail 'UI base commit is unavailable'
  git -C "$source" merge-base --is-ancestor "$base" "$target" || fail 'UI target does not descend from the live base'
  local changes
  changes="$(mktemp)" || fail 'cannot create source comparison metadata'
  git -C "$source" diff --no-renames --name-only -z "$base" "$target" > "$changes" || { rm -f -- "$changes"; fail 'cannot compare UI source'; }
  while IFS= read -r -d '' path; do
    case "$path" in
      frontend/src/views/HomeView.vue|frontend/public/rain-city-1.jpg)
        [[ "$(git -C "$source" ls-tree "$target" -- "$path" | awk '{print $1}')" == 100644 ]] || { rm -f -- "$changes"; fail 'UI asset must be a regular tracked file'; }
        count=$((count + 1)) ;;
      SUBNEXUS_CHANGE_MEMORY.md|SUBNEXUS_CUTOVER_RUNBOOK.md|SUBNEXUS_MIGRATION_LEDGER.md|SUBNEXUS_MIGRATION_PLAN.md|SUBNEXUS_PROJECT_CONTEXT.md|tools/production-deploy/subnexus-ui-cutover.sh|tools/production-deploy/subnexus-ui-cutover.test.sh) ;;
      *) rm -f -- "$changes"; fail "UI-only release changes a protected path: $path" ;;
    esac
  done < "$changes"
  rm -f -- "$changes"
  (( count > 0 )) || fail 'UI release has no homepage or image changes'
}

ui_assert_base_image() {
  local base="$1" image_id="$2" labels
  labels="$(docker_rpc image inspect --format '{{index .Config.Labels "com.subnexus.release.gate"}}|{{index .Config.Labels "com.subnexus.candidate.commit"}}|{{index .Config.Labels "org.opencontainers.image.revision"}}' "$image_id")" || fail 'cannot read live image provenance'
  [[ "$labels" == "subnexus-isolated-build-v1|$base|$base" ]] || fail 'live image does not match the UI base commit'
}

ui_anchor_context() {
  local anchor="$1" old_id="$2" old_image="$3" old_name="$4" anchor_hash="$5" allowed="${6:-stopped}" actual
  local expected_lock_root="${evidence_lock_root:-}"
  valid_sha64 "$old_id" && [[ "$old_image" =~ ^sha256:[0-9a-f]{64}$ ]] && valid_container_ref "$old_name" || fail 'original rollback identity is malformed'
  ui_anchor_validation=1
  SUBNEXUS_APPROVED_CUTOVER_SCRIPT_SHA256="$ui_controller_sha"
  validate_run_directory "$anchor" rollback
  [[ -z "$expected_lock_root" || "$expected_lock_root" == "$evidence_lock_root" ]] || fail 'original rollback and UI run must share their controller lock'
  [[ "$(manifest_value state)" == switched ]] || fail 'original rollback evidence is not a switched run'
  [[ "$(manifest_value live_app_id)" == "$old_id" && "$(manifest_value live_app_image_id)" == "$old_image" && "$(manifest_value preserved_container)" == "$old_name" ]] || fail 'original rollback evidence identifies a different container'
  [[ -z "$anchor_hash" || "$(hash_file "$manifest_file")" == "$anchor_hash" ]] || fail 'original rollback manifest changed'
  preserved_name="$old_name"
  assert_daemon_still_matches_prepare
  assert_dependencies_still_match
  assert_prepared_networks_still_match
  assert_app_data_source_identity
  assert_preserved_container_contract
  actual="$(docker_rpc inspect --format '{{.Name}}|{{.State.Status}}' "$old_id")" || fail 'cannot inspect original rollback state'
  if [[ "$allowed" == stopped ]]; then
    [[ "$actual" == "/$old_name|exited" || "$actual" == "/$old_name|created" ]] || fail 'original rollback container must remain stopped under its fixed name'
  else
    [[ "$actual" == "/$old_name|exited" || "$actual" == "/$old_name|created" || "$actual" == "/$app_name|running" || "$actual" == "/$app_name|exited" ]] || fail 'original rollback container has an unexpected state or name'
  fi
}

ui_assert_anchor() (
  local anchor old_id old_image old_name anchor_hash
  anchor="$(manifest_value ui_anchor_run)"
  old_id="$(manifest_value ui_rollback_id)"
  old_image="$(manifest_value ui_rollback_image)"
  old_name="$(manifest_value ui_rollback_name)"
  anchor_hash="$(manifest_value ui_anchor_manifest_sha256)"
  valid_sha64 "$anchor_hash" || fail 'original rollback manifest hash is missing'
  ui_anchor_context "$anchor" "$old_id" "$old_image" "$old_name" "$anchor_hash" "${1:-stopped}"
)

ui_restore_anchor() (
  local anchor old_id old_image old_name anchor_hash
  anchor="$(manifest_value ui_anchor_run)"; old_id="$(manifest_value ui_rollback_id)"
  old_image="$(manifest_value ui_rollback_image)"; old_name="$(manifest_value ui_rollback_name)"
  anchor_hash="$(manifest_value ui_anchor_manifest_sha256)"
  ui_anchor_context "$anchor" "$old_id" "$old_image" "$old_name" "$anchor_hash" restored
  restore_preserved_container || fail 'original rollback target did not recover health'
  [[ "$(hash_file "$manifest_file")" == "$anchor_hash" ]] || fail 'original rollback evidence changed during recovery'
)

ui_prepare() {
  [[ "$#" == 13 ]] || { ui_usage; return 2; }
  local source="$1" target="$2" base="$3" image_id="$4" archive="$5" archive_sha="$6" gate="$7" live="$8" anchor="$9"
  local old_id="${10}" old_image="${11}" old_name="${12}" public="${13}" wrapper_sha="${SUBNEXUS_APPROVED_UI_CUTOVER_SCRIPT_SHA256:-}"
  # The CLI always supplies the final health argument, which may be empty.
  valid_sha64 "$old_id" || fail 'original rollback ID must be complete'
  [[ "$old_image" == sha256:* ]] || old_image="sha256:$old_image"
  validate_self_sha "$wrapper_sha"
  require_commands
  init_docker
  local live_id live_image anchor_hash
  live_id="$(docker_rpc inspect --format '{{.Id}}' "$live")"
  live_image="$(docker_rpc inspect --format '{{.Image}}' "$live")"
  [[ "$live_id" != "$old_id" ]] || fail 'live and original rollback containers must differ'
  ui_assert_base_image "$base" "$live_image"
  ui_assert_source_delta "$source" "$base" "$target"
  ( ui_anchor_context "$anchor" "$old_id" "$old_image" "$old_name" '' stopped )
  anchor_hash="$(hash_file "$anchor/manifest.env")"
  prepare_run "$source" "$target" "$wrapper_sha" "$image_id" "$archive" "$archive_sha" "$gate" "$live" "$public"
  [[ "$app_id" == "$live_id" && "$app_image_id" == "$live_image" ]] || fail 'live identity changed during UI prepare'
  manifest_set ui_flow application-refresh-v1
  manifest_set ui_base_sha "$base"
  manifest_set ui_controller_sha256 "$ui_controller_sha"
  manifest_set ui_anchor_run "$anchor"
  manifest_set ui_anchor_manifest_sha256 "$anchor_hash"
  manifest_set ui_rollback_id "$old_id"
  manifest_set ui_rollback_image "$old_image"
  manifest_set ui_rollback_name "$old_name"
  manifest_set ui_temporary_name "$app_name-ui-prior-$(manifest_value run_id)"
  manifest_set ui_commit_intent no
  manifest_set ui_state prepared
  ui_assert_anchor
  ui_expected_settings_hash="$(ui_settings_hash)"
  valid_sha64 "$ui_expected_settings_hash" || fail 'cannot capture complete settings hash'
  manifest_set ui_settings_sha256 "$ui_expected_settings_hash"
  ui_assert_settings_unchanged
  printf 'application-refresh-v1\n' > "$run_dir/UI_READY"
  chmod 600 "$run_dir/UI_READY"
  log "UI_PREPARED_RUN=$run_dir"
  log 'UI prepare did not create a rollback container or image. Final switch remains manual.'
}

ui_load_run() {
  local path="$1" scope="$2" expected
  # The prepared run is authored by this wrapper; anchor validation below
  # switches validation to the original controller's manifest contract.
  SUBNEXUS_APPROVED_CUTOVER_SCRIPT_SHA256="${SUBNEXUS_APPROVED_UI_CUTOVER_SCRIPT_SHA256:-}"
  validate_run_directory "$path" "$scope"
  [[ "$(manifest_value ui_flow)" == application-refresh-v1 && "$(manifest_value ui_controller_sha256)" == "$ui_controller_sha" ]] || fail 'run does not belong to the UI controller'
  assert_root_owned_regular "$run_dir/UI_READY" 'UI readiness marker'
  [[ "$(read_one_line "$run_dir/UI_READY")" == application-refresh-v1 ]] || fail 'UI readiness marker is invalid'
  expected="$app_name-ui-prior-$(manifest_value run_id)"
  [[ "$(manifest_value ui_temporary_name)" == "$expected" && "$(manifest_value ui_rollback_id)" != "$app_id" ]] || fail 'UI recovery identities are inconsistent'
  valid_container_ref "$expected" || fail 'UI temporary name is invalid'
  valid_sha40 "$(manifest_value ui_base_sha)" || fail 'UI base commit is invalid'
  valid_sha64 "$(manifest_value ui_settings_sha256)" || fail 'UI settings hash is invalid'
  case "$(manifest_value ui_state)" in prepared|switching|committing|switched|recovered_current|rolling_back|rolled_back_to_original) ;; *) fail 'unsupported UI state' ;; esac
  acquire_lock "$evidence_lock_root"
  init_docker
  assert_daemon_still_matches_prepare
  assert_dependencies_still_match
  assert_app_data_source_identity
  assert_prepared_networks_still_match
}

ui_stop_and_remove() {
  local id="$1" name="$2" actual
  valid_sha64 "$id" || fail 'removal requires a complete container ID'
  actual="$(inspect_container_id_or_empty "$id")" || fail 'cannot inspect exact container before removal'
  [[ -n "$actual" ]] || return 0
  [[ "$actual" == "$id" && "$(docker_rpc inspect --format '{{.Name}}' "$id")" == "/$name" ]] || fail 'container removal identity or name changed'
  [[ "$id" != "$(manifest_value ui_rollback_id)" ]] || fail 'refusing removal of the original rollback target'
  assert_daemon_still_matches_prepare
  if [[ "$(docker_rpc inspect --format '{{.State.Running}}' "$id")" == true ]]; then
    docker_rpc stop --time "$stop_timeout_seconds" "$id" >/dev/null || fail 'container did not stop'
  fi
  [[ "$(docker_rpc inspect --format '{{.State.Running}}' "$id")" == false ]] || fail 'container remains running before removal'
  assert_daemon_still_matches_prepare
  # Docker may commit a removal even when the client loses its response.
  # Reconcile by exact identity before classifying the operation as failed.
  docker_rpc container rm "$id" >/dev/null || {
    actual="$(inspect_container_id_or_empty "$id")" || fail 'cannot reconcile stopped container removal'
    [[ -z "$actual" ]] || fail 'exact stopped container removal failed'
  }
  actual="$(inspect_container_id_or_empty "$id")" || fail 'cannot verify stopped container removal'
  [[ -z "$actual" ]] || fail 'container still exists after removal'
}

ui_remove_candidate() {
  local known observed file_id old_id current_id
  known="$(manifest_value candidate_container_id)"
  if [[ -e "$run_dir/candidate-container-id" || -L "$run_dir/candidate-container-id" ]]; then
    assert_root_owned_regular "$run_dir/candidate-container-id" 'candidate identity'
    file_id="$(read_one_line "$run_dir/candidate-container-id")"
    [[ -z "$known" || "$known" == "$file_id" ]] || fail 'candidate ID records disagree'
    known="$file_id"
  fi
  [[ -z "$known" ]] || valid_sha64 "$known" || fail 'candidate ID is malformed'
  observed=''
  if [[ -n "$known" ]]; then
    observed="$(inspect_container_id_or_empty "$known")" || fail 'cannot inspect candidate before removal'
  fi
  if [[ -z "$observed" ]]; then
    observed="$(inspect_container_id_or_empty "$app_name")" || fail 'cannot inspect production name before candidate removal'
  fi
  [[ -n "$observed" ]] || return 0
  old_id="$(manifest_value ui_rollback_id)"; current_id="$(manifest_value live_app_id)"
  [[ "$observed" != "$old_id" && "$observed" != "$current_id" ]] || return 0
  [[ -z "$known" || "$known" == "$observed" ]] || fail 'candidate ID changed; refusing name-based removal'
  candidate_id="$observed"
  assert_candidate_container_identity "$candidate_id"
  ui_stop_and_remove "$candidate_id" "$app_name"
}

ui_finish_commit() {
  write_run_marker SWITCHED switched || fail 'cannot persist UI switch marker'
  manifest_set state switched || fail 'cannot persist UI switch state'
  manifest_set ui_state switched || fail 'cannot persist UI completion state'
  cutover_active=0
  log "UI_SWITCH_COMPLETED=$run_dir"
}

ui_recover_current() {
  local current temp
  current="$(inspect_container_id_or_empty "$(manifest_value live_app_id)")" || fail 'cannot inspect temporary current during recovery'
  temp="$(manifest_value ui_temporary_name)"
  if [[ -z "$current" ]]; then
    # A successful Docker rm followed by a lost response/failed metadata write
    # is committed. Never remove its healthy replacement to seek a deleted ID.
    [[ "$(manifest_value ui_commit_intent)" == yes ]] || fail 'temporary current is missing before commit; use original-target rollback'
    candidate_id="$(manifest_value candidate_container_id)"
    assert_candidate_container_identity "$candidate_id"
    wait_for_candidate_health || fail 'committed UI candidate needs original-target rollback'
    validate_candidate_runtime
    ui_finish_commit
    return 0
  fi
  assert_daemon_still_matches_prepare
  assert_dependencies_still_match
  ui_assert_anchor
  manifest_set state rolling_back || fail 'cannot persist temporary recovery intent'
  ui_remove_candidate
  preserved_name="$temp"
  restore_preserved_container || fail 'temporary current did not recover health'
  write_run_marker ROLLED_BACK rolled_back || fail 'cannot persist temporary recovery marker'
  manifest_set state rolled_back || fail 'cannot persist temporary recovery state'
  manifest_set ui_state recovered_current || fail 'cannot persist temporary recovery completion'
  cutover_active=0
  ui_assert_settings_unchanged
  log "UI_RECOVERED_CURRENT=$current"
}

ui_switch() {
  [[ "${SUBNEXUS_CUTOVER_CONFIRM:-}" == I_UNDERSTAND_SHORT_PRODUCTION_WINDOW ]] || fail 'UI switch requires the production-window confirmation'
  ui_load_run "$1" switch
  [[ "$(manifest_value state)" == prepared && "$(manifest_value ui_state)" == prepared ]] || fail 'UI switch requires a fresh prepared run'
  [[ "${SUBNEXUS_CUTOVER_QUIET_CONFIRM:-}" == I_HAVE_CHECKED_NO_SETTLEMENT_TASKS ]] || fail 'UI switch requires a settlement/migration worker check'
  ui_expected_settings_hash="$(manifest_value ui_settings_sha256)"
  assert_runtime_still_matches_prepare
  ui_assert_base_image "$(manifest_value ui_base_sha)" "$(manifest_value live_app_image_id)"
  source_root="$(manifest_value source_root)"
  ui_assert_source_delta "$source_root" "$(manifest_value ui_base_sha)" "$target_sha"
  ui_assert_anchor
  ui_assert_settings_unchanged
  [[ "$(docker_rpc image inspect --format '{{.Id}}' "sha256:$expected_image_id")" == "sha256:$expected_image_id" ]] || fail 'UI candidate image is unavailable'
  local temp="$(manifest_value ui_temporary_name)" observed
  observed="$(inspect_container_id_or_empty "$temp")" || fail 'cannot inspect temporary current name'
  [[ -z "$observed" ]] || fail 'temporary current name is occupied'
  manifest_set state switching
  manifest_set ui_state switching
  cutover_active=1
  trap on_error ERR
  trap 'rollback_after_failure 130; exit 130' INT
  trap 'rollback_after_failure 143; exit 143' TERM
  docker_rpc stop --time "$stop_timeout_seconds" "$app_id" >/dev/null || fail 'current application did not stop'
  assert_daemon_still_matches_prepare
  [[ "$(docker_rpc inspect --format '{{.State.Running}}' "$app_id")" == false ]] || fail 'current application remains running'
  docker_rpc rename "$app_id" "$temp" || fail 'cannot stage the current application for failure recovery'
  manifest_set preserved_container "$temp"
  create_candidate_container
  assert_candidate_container_identity "$candidate_id"
  assert_candidate_runtime_contract
  assert_daemon_still_matches_prepare
  docker_rpc start "$candidate_id" >/dev/null || fail 'UI candidate did not start'
  wait_for_candidate_health || fail 'UI candidate failed health stability checks'
  validate_candidate_runtime
  ui_assert_anchor
  assert_dependencies_still_match
  preserved_name="$temp"
  assert_preserved_container_contract
  # Retain bounded diagnostic output before deleting the previous container's
  # writable layer. Application data and file logs remain on their bind mount.
  ( ulimit -f 32768; docker_rpc logs --tail 5000 "$app_id" > "$run_dir/previous-container.log" 2>&1 ) || fail 'cannot retain previous container diagnostic output'
  chmod 600 "$run_dir/previous-container.log"
  manifest_set ui_state committing
  manifest_set ui_commit_intent yes
  ui_stop_and_remove "$app_id" "$temp"
  ui_finish_commit
  trap - ERR INT TERM
}

ui_manual_rollback() {
  [[ "${SUBNEXUS_CUTOVER_CONFIRM:-}" == I_UNDERSTAND_APPLICATION_ROLLBACK ]] || fail 'UI rollback requires the application-rollback confirmation'
  ui_load_run "$1" rollback
  [[ "$(manifest_value state)" != prepared ]] || fail 'UI rollback is not applicable before switch'
  ui_assert_anchor restored
  ui_expected_settings_hash="$(ui_settings_hash)"
  local observed old_id current_id temp current_name
  old_id="$(manifest_value ui_rollback_id)"; current_id="$(manifest_value live_app_id)"
  temp="$(manifest_value ui_temporary_name)"
  observed="$(inspect_container_id_or_empty "$app_name")" || fail 'cannot inspect production name before original rollback'
  if [[ -n "$observed" && "$observed" != "$old_id" && "$observed" != "$current_id" ]]; then
    candidate_id="$observed"
    assert_candidate_container_identity "$observed"
  fi
  manifest_set state rolling_back
  manifest_set ui_state rolling_back
  ui_remove_candidate
  observed="$(inspect_container_id_or_empty "$app_name")" || fail 'cannot inspect production name after candidate removal'
  if [[ "$observed" == "$current_id" ]]; then
    observed="$(inspect_container_id_or_empty "$temp")" || fail 'cannot inspect temporary name before original rollback'
    [[ -z "$observed" ]] || fail 'temporary name is occupied before original rollback'
    preserved_name="$temp"
    assert_preserved_container_contract
    docker_rpc stop --time "$stop_timeout_seconds" "$current_id" >/dev/null || fail 'cannot stop current before original rollback'
    [[ "$(docker_rpc inspect --format '{{.State.Running}}' "$current_id")" == false ]] || fail 'current is still running before original rollback'
    docker_rpc rename "$current_id" "$temp" || fail 'cannot stage current before original rollback'
  fi
  observed="$(inspect_container_id_or_empty "$current_id")" || fail 'cannot inspect temporary current before original rollback'
  if [[ -n "$observed" ]]; then
    [[ "$(docker_rpc inspect --format '{{.Name}}' "$current_id")" == "/$temp" ]] || fail 'temporary current has an unexpected identity before original rollback'
    if [[ "$(docker_rpc inspect --format '{{.State.Running}}' "$current_id")" == true ]]; then
      docker_rpc stop --time "$stop_timeout_seconds" "$current_id" >/dev/null || fail 'cannot stop temporary current before original rollback'
    fi
    [[ "$(docker_rpc inspect --format '{{.State.Running}}' "$current_id")" == false ]] || fail 'temporary current still owns the production port'
  fi
  ui_restore_anchor
  ui_assert_settings_unchanged
  observed="$(inspect_container_id_or_empty "$current_id")" || fail 'cannot inspect temporary current after original rollback'
  if [[ -n "$observed" ]]; then
    current_name="$(docker_rpc inspect --format '{{.Name}}' "$current_id")"
    [[ "$current_name" == "/$temp" ]] || fail 'temporary current has an unexpected name after original rollback'
    ui_stop_and_remove "$current_id" "$temp"
  fi
  write_run_marker ROLLED_BACK rolled_back
  manifest_set state rolled_back
  manifest_set ui_state rolled_back_to_original
  log "UI_ROLLBACK_ORIGINAL_COMPLETED=$old_id"
}

ui_recover_entry() {
  [[ "${SUBNEXUS_CUTOVER_CONFIRM:-}" == I_UNDERSTAND_APPLICATION_ROLLBACK ]] || fail 'UI recovery requires the application-rollback confirmation'
  ui_load_run "$1" rollback
  case "$(manifest_value ui_state)" in switching|committing|recovered_current) ;; *) fail 'temporary-current recovery is not applicable in this state' ;; esac
  ui_expected_settings_hash="$(ui_settings_hash)"
  ui_recover_current
}

ui_dispatch() {
  local action="$1"
  shift
  mode="$action"
  [[ "$mode" != recover ]] || mode=rollback
  if [[ "$action" == prepare ]]; then
    if [[ "$#" == 12 ]]; then ui_prepare "$@" ''; else ui_prepare "$@"; fi
  else
    require_commands
    case "$action" in switch) ui_switch "$1" ;; rollback) ui_manual_rollback "$1" ;; recover) ui_recover_entry "$1" ;; esac
  fi
}

ui_main() {
  local action="${1:-}" controller="${2:-}" expected="${SUBNEXUS_APPROVED_UI_CUTOVER_SCRIPT_SHA256:-}" actual
  case "$-" in *x*) set +x ;; esac
  unset BASH_ENV ENV CDPATH GLOBIGNORE TAR_OPTIONS GZIP
  export PATH='/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin'
  [[ "$EUID" == 0 ]] || { printf 'UI cutover must run as root\n' >&2; return 1; }
  [[ "$#" -ge 2 ]] || { ui_usage; return 2; }
  shift 2
  case "$action" in prepare) [[ "$#" == 12 || "$#" == 13 ]] || { ui_usage; return 2; } ;; switch|rollback|recover) [[ "$#" == 1 ]] || { ui_usage; return 2; } ;; *) ui_usage; return 2 ;; esac
  umask 077
  ui_entry_path="$(realpath -e -P -- "${BASH_SOURCE[0]}")"
  ui_bootstrap_file "$ui_entry_path" || { printf 'Untrusted UI controller path\n' >&2; return 1; }
  [[ "$expected" =~ ^[0-9a-f]{64}$ ]] || { printf 'Missing approved UI controller SHA\n' >&2; return 1; }
  actual="$(sha256sum "$ui_entry_path" | awk '{print $1}')"
  [[ "$actual" == "$expected" ]] || { printf 'UI controller SHA mismatch\n' >&2; return 1; }
  ui_load_controller "$controller" "$action" "$@"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then ui_main "$@"; fi
