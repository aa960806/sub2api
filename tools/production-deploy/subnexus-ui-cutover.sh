#!/usr/bin/env bash
set -Eeuo pipefail

# Application-only refresh. The migration controller supplies its existing
# runtime/backup checks, but never its switch, gate-writing, or rollback path.
readonly ui_controller_sha='b518d93d93745f92c18805afed2655387c9a8499b5b730267ca9dc5c194a4f1a'
ui_entry_path=''
ui_controller_path=''
ui_anchor_validation=0
ui_expected_settings_hash=''

readonly -a ui_production_source_paths=(
  'frontend/src/components/channels/AvailableChannelsTable.vue'
  'frontend/src/components/common/AnnouncementBell.vue'
  'frontend/src/components/common/DateRangePicker.vue'
  'frontend/src/components/common/AnnouncementPopup.vue'
  'frontend/src/components/common/BaseDialog.vue'
  'frontend/src/components/common/BroadcastMarquee.vue'
  'frontend/src/components/common/ConfirmDialog.vue'
  'frontend/src/components/common/CustomerSupportButton.vue'
  'frontend/src/components/common/CustomerSupportModal.vue'
  'frontend/src/components/common/DataTable.vue'
  'frontend/src/components/common/Pagination.vue'
  'frontend/src/components/home/GlassDropletsCanvas.vue'
  'frontend/src/components/home/RainStreaksCanvas.vue'
  'frontend/src/components/home/RainyBackground.vue'
  'frontend/src/components/layout/AppHeader.vue'
  'frontend/src/components/layout/AppLayout.vue'
  'frontend/src/components/payment/SubscriptionPlanCard.vue'
  'frontend/src/components/payment/AmountInput.vue'
  'frontend/src/components/user/dashboard/UserDashboardCharts.vue'
  'frontend/src/components/user/dashboard/UserDashboardQuickActions.vue'
  'frontend/src/components/user/dashboard/UserDashboardRecentUsage.vue'
  'frontend/src/components/user/dashboard/UserDashboardStats.vue'
  'frontend/src/components/user/dashboard/UserDashboardCheckIn.vue'
  'frontend/src/components/user/monitor/ChannelMonitorV3Card.vue'
  'frontend/src/components/user/monitor/MonitorCard.vue'
  'frontend/src/components/user/monitor/MonitorCardGrid.vue'
  'frontend/src/components/user/monitor/MonitorHero.vue'
  'frontend/src/components/user/monitor/MonitorMetricPair.vue'
  'frontend/src/components/user/profile/ProfileIdentityBindingsSection.vue'
  'frontend/src/components/user/profile/ProfileInfoCard.vue'
  'frontend/src/composables/useUserSurfacePerformance.ts'
  'frontend/src/i18n/locales/en/activityCenter.ts'
  'frontend/src/i18n/locales/en/common.ts'
  'frontend/src/i18n/locales/en/inviteActivities.ts'
  'frontend/src/i18n/locales/en/leaderboard.ts'
  'frontend/src/i18n/locales/en/misc.ts'
  'frontend/src/i18n/locales/zh/activityCenter.ts'
  'frontend/src/i18n/locales/zh/common.ts'
  'frontend/src/i18n/locales/zh/inviteActivities.ts'
  'frontend/src/i18n/locales/zh/leaderboard.ts'
  'frontend/src/i18n/locales/zh/misc.ts'
  'frontend/src/styles/subnexus-legacy-surface.css'
  'frontend/src/styles/user-glass-surface.css'
  'frontend/src/utils/bodyScrollLock.ts'
  'frontend/src/views/user/ActivityCenterView.vue'
  'frontend/src/views/user/AffiliateView.vue'
  'frontend/src/views/user/BattlePassView.vue'
  'frontend/src/views/user/BatchImageGuideView.vue'
  'frontend/src/views/user/ChannelStatusV2View.vue'
  'frontend/src/views/user/ChannelStatusV3View.vue'
  'frontend/src/views/user/InviteLotteryView.vue'
  'frontend/src/views/user/InviteMilestoneView.vue'
  'frontend/src/views/user/InvoicesView.vue'
  'frontend/src/views/user/LeaderboardView.vue'
  'frontend/src/views/user/KeysView.vue'
  'frontend/src/views/user/PaymentView.vue'
  'frontend/src/views/user/PaymentQRCodeView.vue'
  'frontend/src/views/user/RechargeWheelView.vue'
  'frontend/src/views/user/SubscriptionsView.vue'
  'frontend/src/views/user/UsageView.vue'
)

readonly -a ui_evidence_source_paths=(
  'frontend/src/components/common/__tests__/CustomerSupportButton.spec.ts'
  'frontend/src/components/common/__tests__/CustomerSupportModal.spec.ts'
  'frontend/src/components/common/__tests__/ScopedDarkModeStyles.spec.ts'
  'frontend/src/components/home/__tests__/RainPerformance.spec.ts'
  'frontend/src/components/layout/__tests__/SubnexusLegacySurface.spec.ts'
  'frontend/src/components/layout/__tests__/UserGlassSurface.spec.ts'
  'frontend/src/components/layout/__tests__/UserSurfacePerformanceRuntime.spec.ts'
  'frontend/src/components/user/dashboard/__tests__/UserDashboardCheckIn.spec.ts'
  'frontend/src/components/payment/__tests__/AmountInput.spec.ts'
  'frontend/src/composables/__tests__/useUserSurfacePerformance.spec.ts'
  'frontend/src/utils/__tests__/bodyScrollLock.spec.ts'
  'frontend/src/views/user/__tests__/InviteActivitiesViews.spec.ts'
  'frontend/src/views/user/__tests__/LeaderboardView.spec.ts'
  'frontend/src/views/user/__tests__/PaymentView.spec.ts'
  'SUBNEXUS_CHANGE_MEMORY.md'
  'SUBNEXUS_CUTOVER_RUNBOOK.md'
  'SUBNEXUS_FEATURE_MATRIX.md'
  'SUBNEXUS_MIGRATION_LEDGER.md'
  'SUBNEXUS_MIGRATION_PLAN.md'
  'SUBNEXUS_PROJECT_CONTEXT.md'
  'SUBNEXUS_ROLLBACK_RUNBOOK.md'
  'SUBNEXUS_USER_GLASS_SURFACE_PLAN.md'
  'tools/production-deploy/subnexus-ui-cutover.sh'
  'tools/production-deploy/subnexus-ui-cutover.test.sh'
)

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
      ( trap - ERR HUP INT TERM; ui_recover_current ) || printf 'Recovery needs attention; use the UI recover or previous-live rollback command.\n' >&2
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
  local key_list="'${rollout_keys[0]}'" sql key
  for key in "${rollout_keys[@]:1}"; do key_list+=" ,'$key'"; done
  for key in "${rollout_content_keys[@]}"; do key_list+=" ,'$key'"; done
  key_list+=" ,'$invitation_config_key'"
  sql="SELECT key || E'\\t' || translate(encode(convert_to(value, 'UTF8'), 'base64'), E'\\n\\r', '') FROM settings WHERE key IN ($key_list) ORDER BY key;"
  db_psql "$sql" | sha256sum | awk '{print $1}'
}

ui_assert_settings_unchanged() {
  local actual
  valid_sha64 "$ui_expected_settings_hash" || fail 'operation settings hash is missing'
  actual="$(ui_settings_hash)" || fail 'cannot read production settings'
  [[ "$actual" == "$ui_expected_settings_hash" ]] || fail 'production settings changed; no settings were restored'
}

ui_source_path_class() {
  local path="$1" allowed
  for allowed in "${ui_production_source_paths[@]}"; do
    if [[ "$path" == "$allowed" ]]; then
      printf 'production\n'
      return 0
    fi
  done
  for allowed in "${ui_evidence_source_paths[@]}"; do
    if [[ "$path" == "$allowed" ]]; then
      printf 'evidence\n'
      return 0
    fi
  done
  return 1
}

ui_assert_source_delta() {
  local source="$1" base="$2" target="$3" path path_class mode count=0
  valid_sha40 "$base" && valid_sha40 "$target" || fail 'UI source SHA is invalid'
  git -C "$source" cat-file -e "$base^{commit}" || fail 'UI base commit is unavailable'
  git -C "$source" merge-base --is-ancestor "$base" "$target" || fail 'UI target does not descend from the live base'
  local changes
  changes="$(mktemp)" || fail 'cannot create source comparison metadata'
  git -C "$source" diff --no-renames --name-only -z "$base" "$target" > "$changes" || { rm -f -- "$changes"; fail 'cannot compare UI source'; }
  while IFS= read -r -d '' path; do
    path_class="$(ui_source_path_class "$path")" || { rm -f -- "$changes"; fail "UI-only release changes a protected path: $path"; }
    mode="$(git -C "$source" ls-tree "$target" -- "$path" | awk '{print $1}')" || { rm -f -- "$changes"; fail 'cannot inspect an allowed UI release path'; }
    [[ "$mode" == 100644 ]] || { rm -f -- "$changes"; fail "UI release path must be a regular tracked file: $path"; }
    if [[ "$path_class" == production ]]; then count=$((count + 1)); fi
  done < "$changes"
  rm -f -- "$changes"
  (( count > 0 )) || fail 'UI release has no approved user-interface changes'
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

# The fixed historical SubNexus run is a pre-switch continuity anchor. Its
# evidence and container identity are required during prepare and switch, but
# post-switch recovery must depend only on the bound previous-live container.
ui_assert_anchor_if_present() {
  local anchor anchor_state anchor_hash
  anchor="$(manifest_value ui_anchor_run)"
  anchor_state="$(manifest_value ui_anchor_state)"
  anchor_hash="$(manifest_value ui_anchor_manifest_sha256)"
  [[ -n "$anchor" ]] || fail 'historical rollback anchor is missing from the UI manifest'
  [[ "$anchor_state" == present && "$anchor_hash" =~ ^[0-9a-f]{64}$ ]] || fail 'historical rollback anchor presence does not match its manifest state'
  [[ -e "$anchor" || -L "$anchor" ]] || fail 'historical rollback anchor is absent without an approved retirement contract'
  ui_assert_anchor "${1:-stopped}"
}

ui_validate_optional_anchor_path() {
  local anchor="$1" normalized
  normalized="$(realpath -m -P -- "$anchor")" || fail 'cannot normalize historical rollback anchor path'
  if path_equal_or_under "$normalized" "$default_evidence_root" ||
     path_equal_or_under "$normalized" "$alternate_evidence_root"; then
    :
  else
    fail 'historical rollback anchor path is outside the approved evidence roots'
  fi
  [[ "$normalized" != / && "$normalized" != "${default_evidence_root}" && "$normalized" != "${alternate_evidence_root}" ]] ||
    fail 'historical rollback anchor path is invalid'
}

ui_normalize_container_id() {
  local id="${1#sha256:}"
  valid_sha64 "$id" || fail 'Docker returned a malformed live container ID'
  printf '%s' "$id"
}

ui_validate_new_rollback_fields() {
  local new_id new_image new_config_image new_name expected_name candidate_ref
  new_id="$(manifest_value ui_new_rollback_id)"
  new_image="$(manifest_value ui_new_rollback_image)"
  new_config_image="$(manifest_value ui_new_rollback_config_image)"
  new_name="$(manifest_value ui_new_rollback_name)"
  expected_name="$(manifest_value ui_temporary_name)"
  valid_sha64 "$new_id" || fail 'previous-live rollback container ID is malformed'
  [[ "$new_image" =~ ^sha256:[0-9a-f]{64}$ ]] || fail 'previous-live rollback image ID is malformed'
  [[ -n "$new_config_image" && "${#new_config_image}" -le 512 && "$new_config_image" != *[[:space:]]* ]] || fail 'previous-live rollback configured image reference is invalid'
  valid_container_ref "$new_name" || fail 'previous-live rollback container name is invalid'
  [[ "$new_name" == "$expected_name" ]] || fail 'previous-live rollback name does not match the prepared temporary name'
  [[ "$new_id" == "$(manifest_value live_app_id)" ]] || fail 'previous-live rollback ID does not match the prepared live application'
  [[ "$new_image" == "$(manifest_value live_app_image_id)" ]] || fail 'previous-live rollback image does not match the prepared live image'
  [[ "$new_id" != "$(manifest_value ui_rollback_id)" ]] || fail 'previous-live rollback target must differ from the historical rollback target'
  candidate_ref="$(manifest_value candidate_container_id)"
  [[ -z "$candidate_ref" || "$new_id" != "$candidate_ref" ]] || fail 'previous-live rollback target must differ from the candidate container'
  case "$(manifest_value ui_new_rollback_state)" in
    prepared|stopped|restored) ;;
    missing) fail 'previous-live rollback target is marked missing; refusing to continue' ;;
    *) fail 'previous-live rollback target state is unsupported' ;;
  esac
}

ui_assert_new_rollback_contract() {
  local expected_id expected_image expected_config_image expected_name observed actual_name actual_image actual_config_image running mode="${1:-any}"
  ui_validate_new_rollback_fields
  expected_id="$(manifest_value ui_new_rollback_id)"
  expected_image="$(manifest_value ui_new_rollback_image)"
  expected_config_image="$(manifest_value ui_new_rollback_config_image)"
  expected_name="$(manifest_value ui_new_rollback_name)"
  observed="$(inspect_container_id_or_empty "$expected_id")" || fail 'cannot inspect previous-live rollback container'
  [[ "$observed" == "$expected_id" ]] || fail 'previous-live rollback container identity is unavailable'
  actual_name="$(docker_rpc inspect --format '{{.Name}}' "$expected_id")" || fail 'cannot inspect previous-live rollback container name'
  actual_name="${actual_name#/}"
  actual_image="$(docker_rpc inspect --format '{{.Image}}' "$expected_id")" || fail 'cannot inspect previous-live rollback container image'
  [[ "$actual_image" == "$expected_image" ]] || fail 'previous-live rollback container image changed'
  actual_config_image="$(docker_rpc inspect --format '{{.Config.Image}}' "$expected_id")" || fail 'cannot inspect previous-live rollback configured image'
  [[ "$actual_config_image" == "$expected_config_image" ]] || fail 'previous-live rollback configured image changed'
  case "$mode" in
    prepared)
      [[ "$actual_name" == "$app_name" ]] || fail 'prepared live container has an unexpected name'
      running="$(docker_rpc inspect --format '{{.State.Running}}' "$expected_id")" || fail 'cannot inspect prepared live state'
      [[ "$running" == true ]] || fail 'prepared live container is not running' ;;
    stopped)
      [[ "$actual_name" == "$expected_name" ]] || fail 'previous-live rollback container has an unexpected stopped name'
      running="$(docker_rpc inspect --format '{{.State.Running}}' "$expected_id")" || fail 'cannot inspect previous-live rollback state'
      [[ "$running" == false ]] || fail 'previous-live rollback container is still running' ;;
    restored)
      [[ "$actual_name" == "$app_name" ]] || fail 'restored previous-live rollback container has an unexpected name'
      running="$(docker_rpc inspect --format '{{.State.Running}}' "$expected_id")" || fail 'cannot inspect restored rollback state'
      [[ "$running" == true ]] || fail 'restored previous-live rollback container is not running' ;;
    any)
      [[ "$actual_name" == "$app_name" || "$actual_name" == "$expected_name" ]] || fail 'previous-live rollback container has an unexpected name' ;;
    *) fail 'previous-live rollback contract mode is invalid' ;;
  esac
  preserved_name="$expected_name"
  assert_preserved_container_contract
}

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
  local live_id live_image live_config_image anchor_hash
  live_id="$(ui_normalize_container_id "$(docker_rpc inspect --format '{{.Id}}' "$live")")"
  live_image="$(docker_rpc inspect --format '{{.Image}}' "$live")"
  live_config_image="$(docker_rpc inspect --format '{{.Config.Image}}' "$live")"
  [[ "$live_id" != "$old_id" ]] || fail 'live and original rollback containers must differ'
  ui_assert_base_image "$base" "$live_image"
  ui_assert_source_delta "$source" "$base" "$target"
  ui_validate_optional_anchor_path "$anchor"
  [[ -e "$anchor" || -L "$anchor" ]] || fail 'historical rollback anchor must exist during prepare'
  ( ui_anchor_context "$anchor" "$old_id" "$old_image" "$old_name" '' stopped )
  anchor_hash="$(hash_file "$anchor/manifest.env")"
  prepare_run "$source" "$target" "$wrapper_sha" "$image_id" "$archive" "$archive_sha" "$gate" "$live" "$public"
  [[ "$app_id" == "$live_id" && "$app_image_id" == "$live_image" ]] || fail 'live identity changed during UI prepare'
  local new_name
  new_name="$app_name-ui-prior-$(manifest_value run_id)"
  valid_container_ref "$new_name" || fail 'generated previous-live rollback name is invalid'
  [[ -z "$(inspect_container_id_or_empty "$new_name")" ]] || fail 'previous-live rollback name is already occupied'
  manifest_set ui_flow application-refresh-v1
  manifest_set ui_base_sha "$base"
  manifest_set ui_controller_sha256 "$ui_controller_sha"
  manifest_set ui_anchor_run "$anchor"
  manifest_set ui_anchor_manifest_sha256 "$anchor_hash"
  manifest_set ui_anchor_state present
  manifest_set ui_rollback_id "$old_id"
  manifest_set ui_rollback_image "$old_image"
  manifest_set ui_rollback_name "$old_name"
  manifest_set ui_new_rollback_id "$live_id"
  manifest_set ui_new_rollback_image "$live_image"
  manifest_set ui_new_rollback_config_image "$live_config_image"
  manifest_set ui_new_rollback_name "$new_name"
  manifest_set ui_new_rollback_state prepared
  manifest_set ui_temporary_name "$new_name"
  manifest_set ui_commit_intent no
  manifest_set ui_state prepared
  ui_assert_anchor
  ui_assert_new_rollback_contract prepared
  ui_expected_settings_hash="$(ui_settings_hash)"
  valid_sha64 "$ui_expected_settings_hash" || fail 'cannot capture complete settings hash'
  manifest_set ui_settings_sha256 "$ui_expected_settings_hash"
  ui_assert_settings_unchanged
  printf 'application-refresh-v1\n' > "$run_dir/UI_READY"
  chmod 600 "$run_dir/UI_READY"
  log "UI_PREPARED_RUN=$run_dir"
  log 'UI prepare bound the current live container as the previous-live rollback target; Docker state is unchanged and final switch remains manual.'
}

ui_load_run() {
  local path="$1" scope="$2" expected anchor_state
  SUBNEXUS_APPROVED_CUTOVER_SCRIPT_SHA256="${SUBNEXUS_APPROVED_UI_CUTOVER_SCRIPT_SHA256:-}"
  validate_run_directory "$path" "$scope"
  [[ "$(manifest_value ui_flow)" == application-refresh-v1 && "$(manifest_value ui_controller_sha256)" == "$ui_controller_sha" ]] || fail 'run does not belong to the UI controller'
  assert_root_owned_regular "$run_dir/UI_READY" 'UI readiness marker'
  [[ "$(read_one_line "$run_dir/UI_READY")" == application-refresh-v1 ]] || fail 'UI readiness marker is invalid'
  expected="$app_name-ui-prior-$(manifest_value run_id)"
  [[ "$(manifest_value ui_temporary_name)" == "$expected" && "$(manifest_value ui_new_rollback_name)" == "$expected" && "$(manifest_value ui_rollback_id)" != "$app_id" ]] || fail 'UI recovery identities are inconsistent'
  valid_container_ref "$expected" || fail 'UI temporary name is invalid'
  if [[ "$scope" == switch ]]; then
    ui_validate_optional_anchor_path "$(manifest_value ui_anchor_run)"
    anchor_state="$(manifest_value ui_anchor_state)"
    [[ "$anchor_state" == present && "$(manifest_value ui_anchor_manifest_sha256)" =~ ^[0-9a-f]{64}$ ]] || fail 'historical rollback anchor was not captured as present during prepare'
    [[ -e "$(manifest_value ui_anchor_run)" || -L "$(manifest_value ui_anchor_run)" ]] || fail 'historical rollback anchor is absent without an approved retirement contract'
  fi
  ui_validate_new_rollback_fields
  valid_sha40 "$(manifest_value ui_base_sha)" || fail 'UI base commit is invalid'
  valid_sha64 "$(manifest_value ui_settings_sha256)" || fail 'UI settings hash is invalid'
  case "$(manifest_value ui_state)" in prepared|switching|committing|switched|recovered_current|rolling_back|rolled_back_to_new|rolled_back_to_original) ;; *) fail 'unsupported UI state' ;; esac
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
  [[ "$id" != "$(manifest_value ui_new_rollback_id)" ]] || fail 'refusing removal of the previous-live rollback target'
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
  local known observed file_id old_id current_id new_id
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
  old_id="$(manifest_value ui_rollback_id)"; current_id="$(manifest_value live_app_id)"; new_id="$(manifest_value ui_new_rollback_id)"
  [[ "$observed" != "$old_id" && "$observed" != "$current_id" && "$observed" != "$new_id" ]] || return 0
  [[ -z "$known" || "$known" == "$observed" ]] || fail 'candidate ID changed; refusing name-based removal'
  candidate_id="$observed"
  assert_candidate_container_identity "$candidate_id"
  # Persist a discovered ID even when the create response or its metadata was
  # lost.  Terminal rollback re-entry must be able to prove that this exact
  # candidate is gone without falling back to a broad production-name lookup.
  if [[ -z "$(manifest_value candidate_container_id)" ]]; then
    manifest_set candidate_container_id "$candidate_id" || fail 'cannot persist discovered candidate identity'
  fi
  ui_stop_and_remove "$candidate_id" "$app_name"
}

ui_assert_candidate_absent() {
  local candidate_ref candidate_name candidate_intent observed
  candidate_ref="$(manifest_value candidate_container_id)"
  candidate_name="$(manifest_value candidate_container_name)"
  candidate_intent="$(manifest_value candidate_container_intent)"
  if [[ -z "$candidate_ref" ]]; then
    [[ -z "$candidate_name" && -z "$candidate_intent" ]] || fail 'candidate identity metadata is incomplete during rollback re-entry'
    [[ ! -e "$run_dir/candidate-container-id" && ! -L "$run_dir/candidate-container-id" ]] || fail 'candidate identity file exists without a manifest ID'
    case "$(manifest_value ui_state):$(manifest_value ui_commit_intent)" in
      switching:no|rolling_back:no|recovered_current:no) ;;
      *) fail 'recovery cannot prove candidate absence without an identity' ;;
    esac
    return 0
  fi
  valid_sha64 "$candidate_ref" || fail 'completed UI rollback lacks a candidate identity'
  observed="$(inspect_container_id_or_empty "$candidate_ref")" || fail 'cannot inspect candidate during rollback re-entry'
  [[ -z "$observed" ]] || fail 'candidate container still exists during rollback re-entry'
}

ui_finish_commit() {
  preserved_name="$(manifest_value ui_new_rollback_name)"
  ui_assert_new_rollback_contract stopped
  manifest_set ui_new_rollback_state stopped
  write_run_marker SWITCHED switched || fail 'cannot persist UI switch marker'
  manifest_set state switched || fail 'cannot persist UI switch state'
  manifest_set ui_state switched || fail 'cannot persist UI completion state'
  cutover_active=0
  log "UI_SWITCH_COMPLETED=$run_dir"
}

ui_recover_current() {
  local current temp marker current_name
  current="$(inspect_container_id_or_empty "$(manifest_value live_app_id)")" || fail 'cannot inspect temporary current during recovery'
  temp="$(manifest_value ui_new_rollback_name)"
  preserved_name="$temp"
  marker="$run_dir/SWITCHED"
  # Once the terminal marker is durable, reconcile the healthy candidate and
  # stopped previous-live container instead of rolling back a committed switch.
  if [[ -e "$marker" && "$(manifest_value ui_state)" == committing ]]; then
    # The previous-live container is the primary rollback object. A durable
    # marker alone is insufficient to declare the switch committed if that
    # object disappeared or drifted during the metadata write window.
    ui_assert_new_rollback_contract stopped
    candidate_id="$(manifest_value candidate_container_id)"
    assert_candidate_container_identity "$candidate_id"
    wait_for_candidate_health || fail 'committed UI candidate needs manual rollback'
    validate_candidate_runtime
    ui_finish_commit
    return 0
  fi
  if [[ -z "$current" ]]; then
    [[ "$(manifest_value ui_commit_intent)" == yes ]] || fail 'previous-live rollback target is missing before commit'
    fail 'previous-live rollback target disappeared; candidate was left untouched for manual recovery'
  fi
  assert_daemon_still_matches_prepare
  assert_dependencies_still_match
  # Never discard a viable candidate until the previous-live object has been
  # proven recoverable. Failures before the rename may leave it under the live
  # name; once staged under the temporary name it must be stopped as prepared.
  current_name="$(docker_rpc inspect --format '{{.Name}}' "$current")" || fail 'cannot inspect previous-live name before recovery'
  if [[ "$current_name" == "/$temp" ]]; then
    ui_assert_new_rollback_contract stopped
  else
    ui_assert_new_rollback_contract any
  fi
  manifest_set state rolling_back || fail 'cannot persist temporary recovery intent'
  ui_remove_candidate
  ui_assert_candidate_absent
  restore_preserved_container || fail 'temporary current did not recover health'
  manifest_set ui_new_rollback_state restored
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
  ui_assert_new_rollback_contract prepared
  ui_assert_base_image "$(manifest_value ui_base_sha)" "$(manifest_value live_app_image_id)"
  source_root="$(manifest_value source_root)"
  ui_assert_source_delta "$source_root" "$(manifest_value ui_base_sha)" "$target_sha"
  ui_assert_anchor_if_present
  ui_assert_settings_unchanged
  [[ "$(docker_rpc image inspect --format '{{.Id}}' "sha256:$expected_image_id")" == "sha256:$expected_image_id" ]] || fail 'UI candidate image is unavailable'
  local temp="$(manifest_value ui_new_rollback_name)" observed
  observed="$(inspect_container_id_or_empty "$temp")" || fail 'cannot inspect temporary current name'
  [[ -z "$observed" ]] || fail 'temporary current name is occupied'
  manifest_set state switching
  manifest_set ui_state switching
  cutover_active=1
  trap on_error ERR
  trap 'rollback_after_failure 129; exit 129' HUP
  trap 'rollback_after_failure 130; exit 130' INT
  trap 'rollback_after_failure 143; exit 143' TERM
  docker_rpc stop --time "$stop_timeout_seconds" "$app_id" >/dev/null || fail 'current application did not stop'
  assert_daemon_still_matches_prepare
  [[ "$(docker_rpc inspect --format '{{.State.Running}}' "$app_id")" == false ]] || fail 'current application remains running'
  docker_rpc rename "$app_id" "$temp" || fail 'cannot stage the current application for failure recovery'
  manifest_set preserved_container "$temp"
  manifest_set ui_new_rollback_state stopped
  create_candidate_container
  assert_candidate_container_identity "$candidate_id"
  assert_candidate_runtime_contract
  assert_daemon_still_matches_prepare
  docker_rpc start "$candidate_id" >/dev/null || fail 'UI candidate did not start'
  wait_for_candidate_health || fail 'UI candidate failed health stability checks'
  validate_candidate_runtime
  ui_assert_anchor_if_present
  assert_dependencies_still_match
  preserved_name="$temp"
  assert_preserved_container_contract
  # Retain bounded diagnostic output while keeping the previous container as
  # the stopped primary rollback target. Application data and file logs remain
  # on their bind mount.
  ( ulimit -f 32768; docker_rpc logs --tail 5000 "$app_id" > "$run_dir/previous-container.log" 2>&1 ) || fail 'cannot retain previous container diagnostic output'
  chmod 600 "$run_dir/previous-container.log"
  manifest_set ui_state committing
  manifest_set ui_commit_intent yes
  ui_finish_commit
  trap - ERR HUP INT TERM
}

ui_manual_rollback() {
  [[ "${SUBNEXUS_CUTOVER_CONFIRM:-}" == I_UNDERSTAND_APPLICATION_ROLLBACK ]] || fail 'UI rollback requires the application-rollback confirmation'
  ui_load_run "$1" rollback
  [[ "$(manifest_value state)" != prepared ]] || fail 'UI rollback is not applicable before switch'
  ui_expected_settings_hash="$(ui_settings_hash)"
  local observed old_id current_id new_id temp current_name new_state
  old_id="$(manifest_value ui_rollback_id)"; current_id="$(manifest_value live_app_id)"
  new_id="$(manifest_value ui_new_rollback_id)"
  temp="$(manifest_value ui_new_rollback_name)"
  new_state="$(manifest_value ui_state)"
  if [[ "$new_state" == rolled_back_to_new ]]; then
    ui_assert_candidate_absent
    ui_assert_new_rollback_contract any
    preserved_name="$temp"
    restore_preserved_container || fail 'previous-live rollback target did not recover health'
    ui_assert_new_rollback_contract restored
    ui_assert_settings_unchanged
    log "UI_ROLLBACK_NEW_ALREADY_COMPLETED=$new_id"
    return 0
  fi
  observed="$(inspect_container_id_or_empty "$app_name")" || fail 'cannot inspect production name before previous-live rollback'
  if [[ "$observed" == "$new_id" ]]; then
    # An operator may have restored the previous-live container outside this
    # wrapper while leaving the bound candidate under another name. Do not
    # declare rollback complete until that exact candidate is proven absent.
    ui_assert_candidate_absent
    ui_assert_new_rollback_contract any
    manifest_set state rolling_back
    manifest_set ui_state rolling_back
    preserved_name="$temp"
    restore_preserved_container || fail 'previous-live rollback target did not recover health'
    manifest_set ui_new_rollback_state restored
    ui_assert_new_rollback_contract restored
    ui_assert_settings_unchanged
    write_run_marker ROLLED_BACK rolled_back
    manifest_set state rolled_back
    manifest_set ui_state rolled_back_to_new
    log "UI_ROLLBACK_NEW_COMPLETED=$new_id"
    return 0
  fi
  if [[ -n "$observed" && "$observed" != "$old_id" && "$observed" != "$current_id" && "$observed" != "$new_id" ]]; then
    candidate_id="$observed"
    assert_candidate_container_identity "$observed"
  fi
  ui_assert_new_rollback_contract stopped
  manifest_set state rolling_back
  manifest_set ui_state rolling_back
  ui_remove_candidate
  ui_assert_candidate_absent
  observed="$(inspect_container_id_or_empty "$new_id")" || fail 'cannot inspect previous-live rollback target before rollback'
  [[ "$observed" == "$new_id" ]] || fail 'previous-live rollback target is unavailable; historical anchor remains disaster-recovery only'
  preserved_name="$temp"
  assert_preserved_container_contract
  current_name="$(docker_rpc inspect --format '{{.Name}}' "$new_id")" || fail 'cannot inspect previous-live rollback target name'
  [[ "$current_name" == "/$temp" || "$current_name" == "/$app_name" ]] || fail 'previous-live rollback target has an unexpected name before rollback'
  if [[ "$(docker_rpc inspect --format '{{.State.Running}}' "$new_id")" == true ]]; then
    docker_rpc stop --time "$stop_timeout_seconds" "$new_id" >/dev/null || fail 'cannot stop previous-live rollback target before rollback'
  fi
  restore_preserved_container || fail 'previous-live rollback target did not recover health'
  manifest_set ui_new_rollback_state restored
  ui_assert_new_rollback_contract restored
  ui_assert_settings_unchanged
  write_run_marker ROLLED_BACK rolled_back
  manifest_set state rolled_back
  manifest_set ui_state rolled_back_to_new
  log "UI_ROLLBACK_NEW_COMPLETED=$new_id"
}

ui_recover_entry() {
  [[ "${SUBNEXUS_CUTOVER_CONFIRM:-}" == I_UNDERSTAND_APPLICATION_ROLLBACK ]] || fail 'UI recovery requires the application-rollback confirmation'
  ui_load_run "$1" rollback
  case "$(manifest_value ui_state)" in switching|committing|rolling_back|recovered_current) ;; *) fail 'temporary-current recovery is not applicable in this state' ;; esac
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
