#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
subject="$script_dir/subnexus-ui-cutover.sh"
controller="$script_dir/subnexus-production-cutover.sh"
fixture_path="$PATH"

test_fail() { printf 'UI TEST ERROR: %s\n' "$*" >&2; exit 1; }

if [[ "${1:-}" == --library ]]; then
  source "$subject"
  ui_bootstrap_file() { [[ -f "$1" ]]; }
  ui_dispatch() {
    [[ "$1" == fixture-probe ]] || test_fail 'controller dispatched the production entrypoint'
    [[ "$(type -t prepare_run)" == function && "$(type -t validate_run_directory)" == function ]] || test_fail 'pinned controller definitions unavailable'
    app_networks=(fixture-network)
    before_settings[fixture-key]=fixture-value
    [[ "${app_networks[0]}" == fixture-network && "${before_settings[fixture-key]}" == fixture-value ]] || test_fail 'controller array scope was lost'
  }
  ui_load_controller "$controller" fixture-probe
  exit
fi

if [[ "${1:-}" == --timeout ]]; then
  source <(head -n -1 "$controller")
  source "$subject"
  require_commands() { :; }
  ui_prepare() { validate_docker_timeout; }
  ui_switch() { validate_docker_timeout; }
  ui_manual_rollback() { validate_docker_timeout; }
  ui_recover_entry() { validate_docker_timeout; }
  SUBNEXUS_DOCKER_TIMEOUT_SECONDS=1800
  ui_dispatch "$2" /unused
  exit
fi

if [[ "${1:-}" == --case ]]; then
  scenario="$2"
  fixture="$3"
  resuming="${4:-}"
  mkdir -p "$fixture/run" "$fixture/containers"
  # The pinned controller is loaded without its production dispatch. Every
  # Docker/dependency boundary below is replaced by filesystem-only fixtures.
  source <(head -n -1 "$controller")
  export PATH="$fixture_path"
  source "$subject"
  ui_install_overrides
  current="$(printf 'a%.0s' {1..64})"
  original="$(printf 'b%.0s' {1..64})"
  replacement="$(printf 'c%.0s' {1..64})"
  stranger="$(printf 'd%.0s' {1..64})"
  target="$(printf 'e%.0s' {1..40})"
  base="$(printf 'f%.0s' {1..40})"
  settings="$(printf '1%.0s' {1..64})"
  image_id="$(printf '2%.0s' {1..64})"
  current_image="$(printf '9%.0s' {1..64})"
  current_config_image=subnexus:current
  app_name=production-app
  temporary_name=production-app-ui-prior-20260905010101-42
  old_name=original-subnexus
  run_dir="$fixture/run"
  manifest_file="$run_dir/manifest.env"
  store="$fixture/containers"
  if [[ "$resuming" != resume ]]; then
  mkdir "$fixture/original-anchor"
  printf '%s\n' immutable-original-run > "$fixture/original-evidence"
  printf '%s\n' "$settings" > "$fixture/settings"
  printf '%s\n' "$app_name" > "$store/$current.name"
  printf '%s\n' true > "$store/$current.running"
  printf '%s\n' "$current_image" > "$store/$current.image"
  printf '%s\n' "$current_config_image" > "$store/$current.config-image"
  printf '%s\n' "$old_name" > "$store/$original.name"
  printf '%s\n' false > "$store/$original.running"
  printf '%s\n' original-image > "$store/$original.image"
  printf '%s\n' original:fixed > "$store/$original.config-image"
  printf '%s\n' \
    'state=prepared' 'ui_state=prepared' 'run_id=20260905010101-42' \
    "live_app_id=$current" "live_app_name=$app_name" "live_app_image_id=sha256:$current_image" \
    "target_sha=$target" "ui_base_sha=$base" 'source_root=/unused/source' \
    "candidate_image_id=$image_id" 'candidate_container_id=' 'candidate_container_name=' 'candidate_container_intent=' \
    "ui_rollback_id=$original" "ui_rollback_name=$old_name" 'ui_rollback_image=sha256:original-image' \
    "ui_anchor_run=$fixture/original-anchor" 'ui_anchor_manifest_sha256=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' 'ui_anchor_state=present' \
    "ui_new_rollback_id=$current" "ui_new_rollback_image=sha256:$current_image" \
    "ui_new_rollback_config_image=$current_config_image" \
    "ui_new_rollback_name=$temporary_name" 'ui_new_rollback_state=prepared' \
    "ui_temporary_name=$temporary_name" 'ui_commit_intent=no' "ui_settings_sha256=$settings" > "$manifest_file"
  printf 'prepared\n' > "$run_dir/READY"
  chmod 600 "$manifest_file"
  if [[ "$scenario" == anchor_missing ]]; then
    rmdir "$fixture/original-anchor"
    rm -- "$store/$original.name" "$store/$original.running" "$store/$original.image" "$store/$original.config-image"
  fi
  fi
  app_id="$current"
  expected_image_id="$image_id"
  target_sha="$target"
  stop_timeout_seconds=1
  SUBNEXUS_CUTOVER_CONFIRM=I_UNDERSTAND_SHORT_PRODUCTION_WINDOW
  SUBNEXUS_CUTOVER_QUIET_CONFIRM=I_HAVE_CHECKED_NO_SETTLEMENT_TASKS
  cutover_active=0
  rollback_active=0

  # Git Bash on Windows cannot reliably lower this limit. Production still
  # executes the real bounded diagnostic write.
  ulimit() { :; }

  assert_root_owned_regular() { [[ -f "$1" && ! -L "$1" ]] || fail "bad fixture file $1"; }
  ui_load_run() {
    [[ "$1" == "$run_dir" ]] || fail 'wrong fixture run'
    app_id="$current"; app_name=production-app; expected_image_id="$image_id"; target_sha="$target"
    case "$(manifest_value state)" in
      prepared) [[ ! -f "$run_dir/SWITCHED" && ! -f "$run_dir/ROLLED_BACK" ]] || fail 'prepared terminal marker' ;;
      switching) [[ ! -f "$run_dir/ROLLED_BACK" ]] || fail 'switching rollback marker' ;;
      switched) assert_run_marker SWITCHED switched ;;
      rolling_back) ;;
      rolled_back) assert_run_marker ROLLED_BACK rolled_back ;;
      *) fail 'unexpected fixture run state' ;;
    esac
    if [[ "${2:-}" == switch ]]; then
      [[ -e "$(manifest_value ui_anchor_run)" || -L "$(manifest_value ui_anchor_run)" ]] || fail 'historical rollback anchor is absent'
    fi
    ui_validate_new_rollback_fields
  }
  mv() {
    local from="${@: -2:1}" destination="${@: -1}"
    if [[ "$destination" == "$manifest_file" && "$resuming" != resume ]]; then
      if [[ "$scenario" == recovery_state_failed ]] && grep -Fxq 'state=rolling_back' "$from"; then return 1; fi
      if [[ "$scenario" == commit_state_persistent ]] && grep -Fxq 'state=switched' "$from"; then return 1; fi
      if [[ "$scenario" == previous_live_missing_after_commit_intent ]] && grep -Fxq 'ui_commit_intent=yes' "$from"; then
        command mv "$@"
        rm -- "$store/$current.name" "$store/$current.running" "$store/$current.image" "$store/$current.config-image"
        return 1
      fi
    fi
    if [[ ! -e "$fixture/fault-fired" && "$resuming" != resume ]]; then
      if [[ "$scenario" == commit_marker_failed && "$destination" == "$run_dir/SWITCHED" ]] ||
         { [[ "$scenario" == commit_state_failed && "$destination" == "$manifest_file" ]] && grep -Fxq 'state=switched' "$from"; }; then
        touch "$fixture/fault-fired"
        return 1
      fi
      if [[ "$scenario" == commit_marker_interrupted && "$destination" == "$run_dir/SWITCHED" ]] ||
         [[ "$scenario" == rollback_marker_interrupted && "$destination" == "$run_dir/ROLLED_BACK" ]]; then
        touch "$fixture/fault-fired"
        command mv "$@"
        exit 77
      fi
    fi
    command mv "$@"
  }
  assert_daemon_still_matches_prepare() { :; }
  assert_dependencies_still_match() { :; }
  assert_app_data_source_identity() { :; }
  assert_prepared_networks_still_match() { :; }
  ui_assert_source_delta() { :; }
  ui_assert_base_image() { :; }
  ui_settings_hash() { cat "$fixture/settings"; }
  inspect_container_id_or_empty() {
    local ref="$1" file
    if [[ -f "$store/$ref.name" ]]; then printf '%s' "$ref"; return; fi
    for file in "$store"/*.name; do
      [[ -f "$file" ]] || continue
      if [[ "$(cat "$file")" == "$ref" ]]; then file="${file##*/}"; printf '%s' "${file%.name}"; return; fi
    done
  }
  docker_rpc() {
    printf '%s\n' "$*" >> "$fixture/actions"
    local id format
    case "$1" in
      image) printf 'sha256:%s\n' "$image_id" ;;
      inspect)
        format="$3"; id="$(inspect_container_id_or_empty "$4")"
        [[ -n "$id" ]] || return 1
        case "$format" in
          '{{.Name}}') printf '/%s\n' "$(cat "$store/$id.name")" ;;
          '{{.Image}}') printf 'sha256:%s\n' "$(cat "$store/$id.image")" ;;
          '{{.Config.Image}}') cat "$store/$id.config-image" ;;
          '{{.State.Running}}') cat "$store/$id.running" ;;
          *) fail "unexpected fixture inspect: $format" ;;
        esac ;;
      stop)
        id="${@: -1}"
        if [[ "$scenario" == stop_failed && "$id" == "$current" ]]; then return 1; fi
        printf 'false\n' > "$store/$id.running" ;;
      rename)
        id="$2"
        if [[ "$scenario" == rename_failed && "$id" == "$current" && "$3" == "$temporary_name" ]]; then return 1; fi
        [[ -z "$(inspect_container_id_or_empty "$3")" ]] || return 1
        printf '%s\n' "$3" > "$store/$id.name" ;;
      start)
        id="$2"
        if [[ "$scenario" == start_failed && "$id" == "$replacement" ]]; then return 1; fi
        printf 'true\n' > "$store/$id.running"
        if [[ "$scenario" == signal && "$id" == "$replacement" ]]; then kill -TERM "$$"; fi
        if [[ "$scenario" == hup_signal && "$id" == "$replacement" ]]; then kill -HUP "$$"; fi ;;
      container)
        [[ "$2" == rm && "$#" == 3 ]] || fail 'fixture forbids force/volume removal'
        id="$3"
        [[ "$(cat "$store/$id.running")" == false ]] || fail 'attempted to remove a running container'
        if [[ "$scenario" == remove_failed && "$id" == "$replacement" ]]; then return 1; fi
        rm -- "$store/$id.name" "$store/$id.running" "$store/$id.image" "$store/$id.config-image" "$store/$id.role" 2>/dev/null || true
        if [[ "$scenario" == remove_response_lost && "$id" == "$replacement" ]] || [[ "$scenario" == rollback_remove_response_lost && "$id" == "$replacement" ]]; then return 1; fi ;;
      logs)
        [[ "$scenario" != log_failed ]] || return 1
        case "$scenario" in
          recovery_target_image_drift) printf '%s\n' "$stranger" > "$store/$current.image"; return 1 ;;
          recovery_target_config_image_drift) printf '%s\n' subnexus:other > "$store/$current.config-image"; return 1 ;;
          recovery_target_name_drift) printf '%s\n' production-app-ui-unexpected > "$store/$current.name"; return 1 ;;
          recovery_target_state_drift) printf '%s\n' true > "$store/$current.running"; return 1 ;;
        esac
        printf 'bounded previous application log\n' ;;
      *) fail "unexpected fixture Docker operation: $*" ;;
    esac
  }
  ui_assert_anchor() {
    [[ "$(cat "$store/$original.image")" == original-image ]] || fail 'original rollback image drifted'
    if [[ "${1:-}" == restored && "$(cat "$store/$original.name")" == "$app_name" ]]; then return 0; fi
    [[ "$(cat "$store/$original.name")" == "$old_name" && "$(cat "$store/$original.running")" == false ]] || fail 'original rollback identity or state drifted'
  }
  assert_runtime_still_matches_prepare() {
    [[ "$(inspect_container_id_or_empty "$app_name")" == "$current" && "$(cat "$store/$current.running")" == true ]] || fail 'current runtime drifted'
  }
  assert_preserved_container_contract() {
    [[ -f "$store/$current.name" ]] || fail 'current identity missing'
    case "$(cat "$store/$current.name")" in "$temporary_name"|"$app_name") ;; *) fail 'current name drifted' ;; esac
    [[ "$(cat "$store/$current.image")" == "$current_image" ]] || fail 'current image drifted'
    [[ "$(cat "$store/$current.config-image")" == "$current_config_image" ]] || fail 'current configured image drifted'
  }
  create_candidate_container() {
    manifest_set candidate_container_name "$app_name"
    manifest_set candidate_container_intent "$(printf '3%.0s' {1..64})"
    [[ -z "$(inspect_container_id_or_empty "$app_name")" ]] || fail 'candidate name occupied'
    printf '%s\n' "$app_name" > "$store/$replacement.name"
    printf 'false\n' > "$store/$replacement.running"
    printf '%s\n' candidate-image > "$store/$replacement.image"
    printf '%s\n' candidate-image:latest > "$store/$replacement.config-image"
    printf 'candidate\n' > "$store/$replacement.role"
    printf 'create candidate %s\n' "$replacement" >> "$fixture/actions"
    [[ "$scenario" != created_without_id ]] || fail 'create response lost before ID persistence'
    candidate_id="$replacement"
    printf '%s\n' "$replacement" > "$run_dir/candidate-container-id"
    manifest_set candidate_container_id "$replacement"
  }
  assert_candidate_container_identity() {
    [[ "$1" == "$replacement" && -f "$store/$replacement.name" && "$(cat "$store/$replacement.name")" == "$app_name" && "$(cat "$store/$replacement.role")" == candidate ]] || fail 'candidate identity/intent mismatch'
  }
  assert_candidate_runtime_contract() { [[ "$scenario" != contract_failed ]] || fail 'candidate contract mismatch'; }
  wait_for_candidate_health() { [[ "$scenario" != unhealthy && "$scenario" != restore_unhealthy && "$scenario" != recovery_state_failed ]]; }
  validate_candidate_runtime() {
    assert_candidate_container_identity "$candidate_id"
    if [[ "$scenario" == settings_drift ]]; then printf '%s\n' "$(printf '4%.0s' {1..64})" > "$fixture/settings"; fi
    if [[ "$scenario" == anchor_lost_during_switch ]]; then
      rmdir "$fixture/original-anchor"
      rm -- "$store/$original.name" "$store/$original.running" "$store/$original.image" "$store/$original.config-image"
    fi
    ui_assert_settings_unchanged
  }
  restore_preserved_container() {
    assert_preserved_container_contract
    [[ -z "$(inspect_container_id_or_empty "$app_name")" || "$(inspect_container_id_or_empty "$app_name")" == "$current" ]] || fail 'recovery name occupied'
    if [[ "$(cat "$store/$current.name")" != "$app_name" ]]; then docker_rpc rename "$current" "$app_name"; fi
    if [[ "$(cat "$store/$current.running")" != true ]]; then docker_rpc start "$current"; fi
    printf 'restore health gate %s\n' "$current" >> "$fixture/actions"
    [[ "$scenario" != restore_unhealthy && "$scenario" != rollback_already_restored_unhealthy && "$scenario" != rollback_completed_unhealthy ]]
  }

  if [[ "$resuming" == resume ]]; then
    SUBNEXUS_CUTOVER_CONFIRM=I_UNDERSTAND_APPLICATION_ROLLBACK
    case "$scenario" in
      commit_marker_interrupted) ui_recover_entry "$run_dir" ;;
      rollback_marker_interrupted) ui_manual_rollback "$run_dir" ;;
      *) fail 'unknown resume case' ;;
    esac
    exit
  fi
  case "$scenario" in
    anchor_drift) printf 'changed-image\n' > "$store/$original.image" ;;
    new_rollback_id_drift) manifest_set ui_new_rollback_id "$stranger" ;;
    new_rollback_image_drift) manifest_set ui_new_rollback_image "sha256:$stranger" ;;
    new_rollback_config_image_drift) manifest_set ui_new_rollback_config_image subnexus:other ;;
    new_rollback_name_drift) manifest_set ui_new_rollback_name production-app-ui-prior-other ;;
    new_rollback_state_drift) manifest_set ui_new_rollback_state missing ;;
    remove_failed|remove_response_lost)
      manifest_set state switching; manifest_set ui_state switching
      docker_rpc stop --time 1 "$current"; docker_rpc rename "$current" "$temporary_name"
      manifest_set ui_new_rollback_state stopped
      create_candidate_container
      SUBNEXUS_CUTOVER_CONFIRM=I_UNDERSTAND_APPLICATION_ROLLBACK
      ui_recover_entry "$run_dir"
      exit ;;
    recovery_candidate_identity_lost_detached)
      manifest_set state switching; manifest_set ui_state switching
      docker_rpc stop --time 1 "$current"; docker_rpc rename "$current" "$temporary_name"
      manifest_set ui_new_rollback_state stopped
      create_candidate_container
      docker_rpc rename "$replacement" production-app-candidate-detached
      manifest_set candidate_container_id ''
      rm -- "$run_dir/candidate-container-id"
      printf 'recovery-invocation-begins\n' >> "$fixture/actions"
      SUBNEXUS_CUTOVER_CONFIRM=I_UNDERSTAND_APPLICATION_ROLLBACK
      ui_recover_entry "$run_dir"
      exit ;;
    occupied_temporary)
      printf '%s\n' "$temporary_name" > "$store/$stranger.name"
      printf 'false\n' > "$store/$stranger.running" ;;
    recover_interrupted)
      manifest_set state switching; manifest_set ui_state switching
      docker_rpc stop --time 1 "$current"; docker_rpc rename "$current" "$temporary_name"
      SUBNEXUS_CUTOVER_CONFIRM=I_UNDERSTAND_APPLICATION_ROLLBACK
      ui_recover_entry "$run_dir"
      exit ;;
    recover_anchor_drift|recover_anchor_missing)
      manifest_set state switching; manifest_set ui_state switching
      docker_rpc stop --time 1 "$current"; docker_rpc rename "$current" "$temporary_name"
      if [[ "$scenario" == recover_anchor_drift ]]; then
        printf 'changed-image\n' > "$store/$original.image"
      else
        rmdir "$fixture/original-anchor"
        rm -- "$store/$original.name" "$store/$original.running" "$store/$original.image" "$store/$original.config-image"
      fi
      SUBNEXUS_CUTOVER_CONFIRM=I_UNDERSTAND_APPLICATION_ROLLBACK
      ui_recover_entry "$run_dir"
      exit ;;
    rollback_without_id|rollback_from_recovered)
      manifest_set state switching; manifest_set ui_state switching
      docker_rpc stop --time 1 "$current"; docker_rpc rename "$current" "$temporary_name"
      if [[ "$scenario" == rollback_without_id ]]; then
        create_candidate_container
        manifest_set candidate_container_id ''
        rm -- "$run_dir/candidate-container-id"
      else
        ui_expected_settings_hash="$settings"
        ui_recover_current
      fi
      printf '%s\n' "$(printf '5%.0s' {1..64})" > "$fixture/settings"
      SUBNEXUS_CUTOVER_CONFIRM=I_UNDERSTAND_APPLICATION_ROLLBACK
      ui_manual_rollback "$run_dir"
      exit ;;
  esac
  ui_switch "$run_dir"
  if [[ "$scenario" == rollback_anchor_missing ]]; then
    rmdir "$fixture/original-anchor"
    rm -- "$store/$original.name" "$store/$original.running" "$store/$original.image" "$store/$original.config-image"
  elif [[ "$scenario" == rollback_anchor_drift ]]; then
    printf 'changed-image\n' > "$store/$original.image"
  fi
  if [[ "$scenario" == rollback_target_missing ]]; then
    rm -- "$store/$current.name" "$store/$current.running" "$store/$current.image" "$store/$current.config-image"
  elif [[ "$scenario" == rollback_target_image_drift ]]; then
    printf '%s\n' "$stranger" > "$store/$current.image"
  elif [[ "$scenario" == rollback_target_config_image_drift ]]; then
    printf '%s\n' subnexus:other > "$store/$current.config-image"
  elif [[ "$scenario" == rollback_target_name_drift ]]; then
    printf '%s\n' production-app-ui-unexpected > "$store/$current.name"
  elif [[ "$scenario" == rollback_target_state_drift ]]; then
    printf '%s\n' true > "$store/$current.running"
  fi
  if [[ "$scenario" == rollback || "$scenario" == rollback_interrupted || "$scenario" == rollback_marker_interrupted || "$scenario" == rollback_remove_response_lost || "$scenario" == rollback_anchor_missing || "$scenario" == rollback_anchor_drift || "$scenario" == rollback_already_restored || "$scenario" == rollback_already_restored_stopped || "$scenario" == rollback_already_restored_unhealthy || "$scenario" == rollback_already_restored_candidate_present || "$scenario" == rollback_completed_unhealthy || "$scenario" == rollback_completed_candidate_present || "$scenario" == rollback_target_missing || "$scenario" == rollback_target_image_drift || "$scenario" == rollback_target_config_image_drift || "$scenario" == rollback_target_name_drift || "$scenario" == rollback_target_state_drift ]]; then
    # Administrator updates made after deployment must survive rollback.
    printf '%s\n' "$(printf '5%.0s' {1..64})" > "$fixture/settings"
    if [[ "$scenario" == rollback_interrupted ]]; then
      manifest_set state rolling_back; manifest_set ui_state rolling_back
      ui_remove_candidate
    elif [[ "$scenario" == rollback_already_restored || "$scenario" == rollback_already_restored_stopped || "$scenario" == rollback_already_restored_unhealthy || "$scenario" == rollback_completed_unhealthy || "$scenario" == rollback_already_restored_candidate_present ]]; then
      docker_rpc stop --time 1 "$replacement"
      if [[ "$scenario" == rollback_already_restored_candidate_present ]]; then
        docker_rpc rename "$replacement" production-app-candidate-detached
      else
        docker_rpc container rm "$replacement"
      fi
      docker_rpc rename "$current" "$app_name"
      if [[ "$scenario" != rollback_already_restored_stopped ]]; then docker_rpc start "$current"; fi
      if [[ "$scenario" == rollback_completed_unhealthy ]]; then
        manifest_set state rolling_back
        manifest_set ui_state rolling_back
        manifest_set ui_new_rollback_state restored
        write_run_marker ROLLED_BACK rolled_back
        manifest_set state rolled_back
        manifest_set ui_state rolled_back_to_new
      fi
    elif [[ "$scenario" == rollback_completed_candidate_present ]]; then
      manifest_set state rolling_back
      manifest_set ui_state rolling_back
      write_run_marker ROLLED_BACK rolled_back
      manifest_set state rolled_back
      manifest_set ui_state rolled_back_to_new
    fi
    if [[ "$scenario" == rollback_already_restored_candidate_present ]]; then
      printf 'rollback-invocation-begins\n' >> "$fixture/actions"
    fi
    SUBNEXUS_CUTOVER_CONFIRM=I_UNDERSTAND_APPLICATION_ROLLBACK
    ui_manual_rollback "$run_dir"
    if [[ "$scenario" != rollback_target_* && "$scenario" != rollback_already_restored_unhealthy && "$scenario" != rollback_completed_unhealthy && "$scenario" != rollback_completed_candidate_present ]]; then
      ui_manual_rollback "$run_dir"
    fi
  fi
  exit
fi

bash -n "$subject"
bash "$0" --library
bash "$0" --timeout prepare
for phase in switch rollback recover; do
  if bash "$0" --timeout "$phase" >/dev/null 2>&1; then test_fail "$phase accepted a prepare-only Docker timeout"; fi
done
[[ "$(sha256sum "$controller" | awk '{print $1}')" == 307fe13b1260af226ffa2ded52df5aba5d6a2285fa5d35bad2f0fee69eab823c ]] || test_fail 'approved migration controller changed'
if grep -n $'\r' "$subject"; then test_fail 'UI controller must use LF'; fi
for forbidden in 'docker commit' 'docker build' 'docker image save' 'docker image tag' 'docker system prune' 'nginx -s'; do
  if grep -Fq "$forbidden" "$subject"; then test_fail "forbidden UI controller operation: $forbidden"; fi
done
grep -Fq 'default_transaction_read_only=on' "$subject" || test_fail 'database query boundary is not read-only'
grep -Fq 'rollout_keys[0]' "$subject" || test_fail 'protected settings hash does not use the key allowlist'
grep -Fq "close_rollout_gates() { fail" "$subject" || test_fail 'gate mutation not blocked'
grep -Fq "restore_rollout_gates() { fail" "$subject" || test_fail 'settings restoration not blocked'
grep -Fq 'docker_rpc container rm "$id"' "$subject" || test_fail 'temporary removal is not an ordinary exact-ID rm'
grep -Fq 'ui_anchor_state present' "$subject" || test_fail 'fixed rollback anchor state is not required'
grep -Fq 'manifest_set ui_new_rollback_id "$live_id"' "$subject" || test_fail 'prepare does not bind the previous live container as primary rollback'
grep -Fq 'ui_assert_new_rollback_contract stopped' "$subject" || test_fail 'switch does not prove the stopped previous-live rollback contract'
grep -Fq 'manifest_set ui_state rolled_back_to_new' "$subject" || test_fail 'manual rollback does not target the previous live container'
grep -Fq "trap 'rollback_after_failure 129; exit 129' HUP" "$subject" || test_fail 'SIGHUP does not trigger automatic recovery'
grep -Fq 'trap - ERR HUP INT TERM' "$subject" || test_fail 'successful switch does not clear the SIGHUP recovery trap'
[[ "$(grep -Ec '^[[:space:]]+ui_assert_anchor_if_present([[:space:]]|$)' "$subject")" == 2 ]] || test_fail 'historical anchor validation leaked outside the two switch gates'
if grep -Fq 'ui_stop_and_remove "$app_id" "$temp"' "$subject"; then test_fail 'successful switch deletes the primary rollback container'; fi
if grep -Fq 'ui_restore_anchor' "$subject"; then test_fail 'normal UI rollback can restore the historical disaster-recovery anchor'; fi

root="$(mktemp -d /tmp/subnexus-ui-tests.XXXXXX)"
trap 'rm -rf -- "$root"' EXIT
current="$(printf 'a%.0s' {1..64})"
original="$(printf 'b%.0s' {1..64})"
replacement="$(printf 'c%.0s' {1..64})"
temporary_name=production-app-ui-prior-20260905010101-42
case_count=0
for scenario in success stop_failed rename_failed start_failed created_without_id contract_failed unhealthy restore_unhealthy recovery_state_failed signal hup_signal log_failed recovery_target_image_drift recovery_target_config_image_drift recovery_target_name_drift recovery_target_state_drift recovery_candidate_identity_lost_detached remove_failed remove_response_lost commit_marker_failed commit_state_failed commit_state_persistent commit_marker_interrupted previous_live_missing_after_commit_intent settings_drift anchor_drift anchor_lost_during_switch occupied_temporary anchor_missing new_rollback_id_drift new_rollback_image_drift new_rollback_config_image_drift new_rollback_name_drift new_rollback_state_drift recover_interrupted recover_anchor_drift recover_anchor_missing rollback rollback_interrupted rollback_without_id rollback_from_recovered rollback_marker_interrupted rollback_remove_response_lost rollback_anchor_missing rollback_anchor_drift rollback_already_restored rollback_already_restored_stopped rollback_already_restored_unhealthy rollback_already_restored_candidate_present rollback_completed_unhealthy rollback_completed_candidate_present rollback_target_missing rollback_target_image_drift rollback_target_config_image_drift rollback_target_name_drift rollback_target_state_drift; do
  fixture="$root/$scenario"
  mkdir "$fixture"
  rc=0
  bash "$0" --case "$scenario" "$fixture" > "$fixture/output" 2>&1 || rc=$?
  case "$scenario" in success|remove_response_lost|recover_interrupted|recover_anchor_drift|recover_anchor_missing|rollback|rollback_interrupted|rollback_without_id|rollback_from_recovered|rollback_remove_response_lost|rollback_anchor_missing|rollback_anchor_drift|rollback_already_restored|rollback_already_restored_stopped) [[ "$rc" == 0 ]] || { cat "$fixture/output"; test_fail "$scenario failed ($rc)"; } ;; *) [[ "$rc" != 0 ]] || test_fail "$scenario unexpectedly succeeded" ;; esac
  if [[ "$scenario" == commit_marker_interrupted || "$scenario" == rollback_marker_interrupted ]]; then
    bash "$0" --case "$scenario" "$fixture" resume >> "$fixture/output" 2>&1 || { cat "$fixture/output"; test_fail "$scenario resume failed"; }
  fi
  case "$scenario" in
    recovery_state_failed)
      [[ "$(cat "$fixture/containers/$current.name")" == "$temporary_name" && "$(cat "$fixture/containers/$current.running")" == false ]] || test_fail 'failed recovery state write mutated temporary current'
      [[ ! -f "$fixture/run/ROLLED_BACK" ]] || test_fail 'failed recovery state write left a terminal marker'
      grep -Fxq 'state=switching' "$fixture/run/manifest.env" || test_fail 'failed recovery state write was not retryable'
      if grep -Eq 'UI_RECOVERED_CURRENT=|UI_SWITCH_COMPLETED=' "$fixture/output"; then test_fail 'failed recovery state write reported success'; fi ;;
    commit_state_persistent)
      [[ -f "$fixture/containers/$current.name" && "$(cat "$fixture/containers/$current.name")" == "$temporary_name" && "$(cat "$fixture/containers/$current.running")" == false && "$(cat "$fixture/containers/$replacement.running")" == true ]] || test_fail 'failed completion metadata lost the candidate or primary rollback target'
      [[ -f "$fixture/run/SWITCHED" ]] || test_fail 'failed completion metadata lost the completed marker'
      grep -Fxq 'state=switching' "$fixture/run/manifest.env" || test_fail 'failed completion metadata was not retryable'
      if grep -Eq 'UI_RECOVERED_CURRENT=|UI_SWITCH_COMPLETED=' "$fixture/output"; then test_fail 'failed completion metadata reported success'; fi ;;
    success|commit_state_failed|commit_marker_interrupted)
      [[ -f "$fixture/containers/$current.name" ]] || test_fail "$scenario discarded the primary rollback container"
      [[ "$(cat "$fixture/containers/$current.name")" == "$temporary_name" && "$(cat "$fixture/containers/$current.running")" == false ]] || test_fail "$scenario did not retain the stopped primary rollback target"
      [[ "$(cat "$fixture/containers/$replacement.name")" == production-app && "$(cat "$fixture/containers/$replacement.running")" == true ]] || test_fail "$scenario lost the committed candidate"
      grep -Fxq 'ui_state=switched' "$fixture/run/manifest.env" || test_fail "$scenario failed commit reconciliation"
      grep -Fxq 'ui_new_rollback_state=stopped' "$fixture/run/manifest.env" || test_fail "$scenario lost the primary rollback state"
      if grep -Fq "container rm $current" "$fixture/actions"; then test_fail "$scenario removed the primary rollback container"; fi ;;
    remove_response_lost|commit_marker_failed)
      [[ "$(cat "$fixture/containers/$current.name")" == production-app && "$(cat "$fixture/containers/$current.running")" == true ]] || test_fail "$scenario did not restore the previous live container"
      [[ ! -f "$fixture/containers/$replacement.name" ]] || test_fail "$scenario left a failed candidate"
      grep -Fxq 'ui_state=recovered_current' "$fixture/run/manifest.env" || test_fail "$scenario failed recovery state" ;;
    previous_live_missing_after_commit_intent)
      [[ ! -f "$fixture/containers/$current.name" ]] || test_fail 'missing previous-live fixture unexpectedly retained the deleted target'
      [[ "$(cat "$fixture/containers/$replacement.name")" == production-app && "$(cat "$fixture/containers/$replacement.running")" == true ]] || test_fail 'missing previous-live recovery mutated the healthy candidate'
      [[ ! -f "$fixture/run/SWITCHED" ]] || test_fail 'missing previous-live recovery reported a committed switch'
      if grep -Eq 'UI_RECOVERED_CURRENT=|UI_SWITCH_COMPLETED=' "$fixture/output"; then test_fail 'missing previous-live recovery reported success'; fi ;;
    remove_failed)
      [[ "$(cat "$fixture/containers/$current.name")" == "$temporary_name" && "$(cat "$fixture/containers/$current.running")" == false ]] || test_fail 'failed candidate removal mutated the stopped primary rollback target'
      [[ "$(cat "$fixture/containers/$replacement.name")" == production-app && "$(cat "$fixture/containers/$replacement.running")" == false ]] || test_fail 'failed candidate removal left an unexpected candidate state'
      grep -Fxq 'state=rolling_back' "$fixture/run/manifest.env" || test_fail 'failed candidate removal did not retain a resumable state' ;;
    recovery_target_image_drift|recovery_target_config_image_drift|recovery_target_name_drift|recovery_target_state_drift)
      [[ "$(cat "$fixture/containers/$replacement.name")" == production-app && "$(cat "$fixture/containers/$replacement.running")" == true ]] || test_fail "$scenario removed or stopped the viable candidate"
      [[ ! -f "$fixture/run/ROLLED_BACK" ]] || test_fail "$scenario reported recovery success"
      grep -Fxq 'state=switching' "$fixture/run/manifest.env" || test_fail "$scenario advanced recovery state before validating previous-live"
      if grep -Fq "container rm $replacement" "$fixture/actions"; then test_fail "$scenario removed the viable candidate before validating previous-live"; fi
      if grep -Fq 'UI_RECOVERED_CURRENT=' "$fixture/output"; then test_fail "$scenario reported a failed recovery as successful"; fi ;;
    recovery_candidate_identity_lost_detached)
      [[ "$(cat "$fixture/containers/$replacement.name")" == production-app-candidate-detached && "$(cat "$fixture/containers/$replacement.running")" == false ]] || test_fail 'recovery mutated the detached candidate with missing identity metadata'
      [[ "$(cat "$fixture/containers/$current.name")" == "$temporary_name" && "$(cat "$fixture/containers/$current.running")" == false ]] || test_fail 'recovery modified previous-live without proving candidate absence'
      [[ ! -f "$fixture/run/ROLLED_BACK" ]] || test_fail 'recovery reported completion without proving candidate absence'
      grep -Fxq 'state=rolling_back' "$fixture/run/manifest.env" || test_fail 'recovery did not retain a resumable state after candidate identity loss'
      if grep -Fq 'UI_RECOVERED_CURRENT=' "$fixture/output"; then test_fail 'recovery reported success with an untracked detached candidate'; fi
      if awk 'seen { print } /^recovery-invocation-begins$/ { seen=1 }' "$fixture/actions" | grep -Eq "(stop --time [^ ]+|rename|start|container rm) $current([[:space:]]|$)"; then
        test_fail 'recovery modified previous-live after candidate identity became unprovable'
      fi ;;
    anchor_missing)
      [[ "$(cat "$fixture/containers/$current.name")" == production-app && "$(cat "$fixture/containers/$current.running")" == true ]] || test_fail 'missing anchor mutated the live container'
      [[ ! -f "$fixture/containers/$replacement.name" ]] || test_fail 'missing anchor created a candidate'
      [[ ! -f "$fixture/run/SWITCHED" ]] || test_fail 'missing anchor reported a switch' ;;
    recover_anchor_drift|recover_anchor_missing)
      [[ "$(cat "$fixture/containers/$current.name")" == production-app && "$(cat "$fixture/containers/$current.running")" == true ]] || test_fail "$scenario did not restore the immediate previous live container"
      [[ ! -f "$fixture/containers/$replacement.name" ]] || test_fail "$scenario retained a candidate"
      [[ -f "$fixture/run/ROLLED_BACK" ]] || test_fail "$scenario did not persist recovery completion"
      grep -Fxq 'ui_state=recovered_current' "$fixture/run/manifest.env" || test_fail "$scenario recovery state missing"
      grep -Fxq 'ui_new_rollback_state=restored' "$fixture/run/manifest.env" || test_fail "$scenario previous-live state missing"
      grep -Fq 'UI_RECOVERED_CURRENT=' "$fixture/output" || test_fail "$scenario did not report recovery success"
      if grep -Fq "$original" "$fixture/actions"; then test_fail "$scenario operated on the historical anchor"; fi ;;
    rollback|rollback_interrupted|rollback_without_id|rollback_from_recovered|rollback_marker_interrupted|rollback_remove_response_lost|rollback_anchor_missing|rollback_anchor_drift|rollback_already_restored|rollback_already_restored_stopped)
      [[ "$(cat "$fixture/containers/$current.name")" == production-app && "$(cat "$fixture/containers/$current.running")" == true ]] || test_fail 'manual rollback did not restore the immediate previous live container'
      [[ ! -f "$fixture/containers/$replacement.name" ]] || test_fail 'manual rollback retained the candidate'
      [[ "$(cat "$fixture/settings")" == "$(printf '5%.0s' {1..64})" ]] || test_fail 'manual rollback overwrote administrator settings'
      grep -Fxq 'ui_state=rolled_back_to_new' "$fixture/run/manifest.env" || test_fail 'manual rollback state missing'
      grep -Fxq 'ui_new_rollback_state=restored' "$fixture/run/manifest.env" || test_fail 'primary rollback target state missing'
      grep -Fq "restore health gate $current" "$fixture/actions" || test_fail 'manual rollback bypassed the restoration health gate'
      ;;
    rollback_target_missing|rollback_target_image_drift|rollback_target_config_image_drift|rollback_target_name_drift|rollback_target_state_drift)
      [[ "$(cat "$fixture/containers/$replacement.name")" == production-app && "$(cat "$fixture/containers/$replacement.running")" == true ]] || test_fail "$scenario mutated the live candidate"
      [[ ! -f "$fixture/run/ROLLED_BACK" ]] || test_fail "$scenario reported rollback success" ;;
    rollback_completed_candidate_present)
      [[ "$(cat "$fixture/containers/$replacement.name")" == production-app && "$(cat "$fixture/containers/$replacement.running")" == true ]] || test_fail 'terminal rollback removed or stopped the candidate'
      [[ "$(cat "$fixture/containers/$current.name")" == "$temporary_name" && "$(cat "$fixture/containers/$current.running")" == false ]] || test_fail 'terminal rollback mutated the previous-live target'
      grep -Fxq 'ui_state=rolled_back_to_new' "$fixture/run/manifest.env" || test_fail 'terminal rollback state was unexpectedly changed'
      if grep -Fq 'UI_ROLLBACK_NEW_ALREADY_COMPLETED=' "$fixture/output"; then test_fail 'terminal rollback accepted an extant candidate'; fi
      if grep -Fq "container rm $replacement" "$fixture/actions"; then test_fail 'terminal rollback removed an extant candidate'; fi ;;
    rollback_already_restored_candidate_present)
      [[ "$(cat "$fixture/containers/$replacement.name")" == production-app-candidate-detached && "$(cat "$fixture/containers/$replacement.running")" == false ]] || test_fail 'already-restored rollback mutated the detached candidate'
      [[ "$(cat "$fixture/containers/$current.name")" == production-app && "$(cat "$fixture/containers/$current.running")" == true ]] || test_fail 'already-restored rollback mutated the previous-live target'
      grep -Fxq 'ui_state=switched' "$fixture/run/manifest.env" || test_fail 'already-restored rollback advanced state with an extant candidate'
      if grep -Eq 'UI_ROLLBACK_NEW_(ALREADY_)?COMPLETED=' "$fixture/output"; then test_fail 'already-restored rollback accepted an extant candidate'; fi
      if awk 'seen { print } /^rollback-invocation-begins$/ { seen=1 }' "$fixture/actions" | grep -Eq "(stop --time [^ ]+|rename|start|container rm) $current([[:space:]]|$)"; then
        test_fail 'already-restored rollback modified the previous-live target after detecting an extant candidate'
      fi ;;
    *)
      [[ "$(cat "$fixture/containers/$current.name")" == production-app && "$(cat "$fixture/containers/$current.running")" == true ]] || { cat "$fixture/output"; test_fail "$scenario did not retain/recover current"; }
      [[ ! -f "$fixture/containers/$replacement.name" ]] || test_fail "$scenario left a failed candidate" ;;
  esac
  if [[ "$scenario" != rollback* && "$scenario" != anchor_missing && "$scenario" != anchor_lost_during_switch && "$scenario" != recover_anchor_missing ]]; then
    [[ "$(cat "$fixture/containers/$original.name")" == original-subnexus && "$(cat "$fixture/containers/$original.running")" == false ]] || test_fail "$scenario mutated the original rollback target"
  fi
  [[ "$(cat "$fixture/original-evidence")" == immutable-original-run ]] || test_fail 'original rollback evidence changed'
  if [[ "$scenario" == restore_unhealthy ]]; then
    [[ ! -f "$fixture/run/ROLLED_BACK" ]] || test_fail 'unhealthy restoration reported rollback success'
    grep -Fxq 'state=rolling_back' "$fixture/run/manifest.env" || test_fail 'unhealthy restoration did not retain a resumable state'
  fi
  if [[ "$scenario" == rollback_already_restored_unhealthy ]]; then
    [[ ! -f "$fixture/run/ROLLED_BACK" ]] || test_fail 'unhealthy already-restored target reported rollback success'
    grep -Fxq 'state=rolling_back' "$fixture/run/manifest.env" || test_fail 'unhealthy already-restored target did not retain a resumable state'
    grep -Fq "restore health gate $current" "$fixture/actions" || test_fail 'already-running rollback bypassed the restoration health gate'
    if grep -Eq 'UI_ROLLBACK_NEW_(ALREADY_)?COMPLETED=' "$fixture/output"; then test_fail 'unhealthy already-restored target reported completion'; fi
  fi
  if [[ "$scenario" == rollback_completed_unhealthy ]]; then
    grep -Fxq 'ui_state=rolled_back_to_new' "$fixture/run/manifest.env" || test_fail 'completed rollback fixture lost its terminal state'
    grep -Fq "restore health gate $current" "$fixture/actions" || test_fail 'completed rollback bypassed the restoration health gate'
    if grep -Fq 'UI_ROLLBACK_NEW_ALREADY_COMPLETED=' "$fixture/output"; then test_fail 'unhealthy completed rollback was reported healthy'; fi
  fi
  if [[ "$scenario" == rollback_anchor_missing || "$scenario" == rollback_anchor_drift ]]; then
    if grep -Fq "$original" "$fixture/actions"; then test_fail "$scenario operated on the historical anchor"; fi
  fi
  if [[ -f "$fixture/actions" ]] && grep -E 'commit|image (save|tag)|rm --force|rm -v' "$fixture/actions"; then test_fail 'fixture observed forbidden Docker operation'; fi
  case_count=$((case_count + 1))
done

# Exercise every path exported by the real Git allowlist.
source <(head -n -1 "$controller")
export PATH="$fixture_path"
source "$subject"
[[ "$(ui_normalize_container_id "sha256:$current")" == "$current" ]] || test_fail 'prefixed Docker live ID was not normalized'
[[ "$(ui_normalize_container_id "$current")" == "$current" ]] || test_fail 'plain Docker live ID was not preserved'
if (ui_normalize_container_id sha256:short) >/dev/null 2>&1; then test_fail 'malformed Docker live ID was accepted'; fi
git_repo="$root/source"
mkdir "$git_repo"
git -C "$git_repo" init -q
git -C "$git_repo" config user.name fixture
git -C "$git_repo" config user.email fixture@example.invalid
git -C "$git_repo" config core.autocrlf false
all_allowed_paths=("${ui_production_source_paths[@]}" "${ui_evidence_source_paths[@]}")
duplicate_allowed_paths="$(printf '%s\n' "${all_allowed_paths[@]}" | sort | uniq -d)"
[[ -z "$duplicate_allowed_paths" ]] || test_fail "duplicate UI allowlist path: $duplicate_allowed_paths"
[[ "$(ui_source_path_class frontend/src/composables/useUserSurfacePerformance.ts)" == production ]] || test_fail 'visual performance preference must be an exact production UI path'
for evidence_path in \
  frontend/src/composables/__tests__/useUserSurfacePerformance.spec.ts \
  frontend/src/components/home/__tests__/RainPerformance.spec.ts \
  frontend/src/components/layout/__tests__/UserSurfacePerformanceRuntime.spec.ts; do
  [[ "$(ui_source_path_class "$evidence_path")" == evidence ]] || test_fail "visual performance test must be an exact evidence path: $evidence_path"
done
for path in "${all_allowed_paths[@]}"; do
  mkdir -p "$git_repo/$(dirname -- "$path")"
  printf 'base %s\n' "$path" > "$git_repo/$path"
done
git -C "$git_repo" add .
git -C "$git_repo" commit -qm base
base_sha="$(git -C "$git_repo" rev-parse HEAD)"
for path in "${all_allowed_paths[@]}"; do
  printf 'updated %s\n' "$path" >> "$git_repo/$path"
done
git -C "$git_repo" commit -qam all-allowed-paths
ui_sha="$(git -C "$git_repo" rev-parse HEAD)"
ui_assert_source_delta "$git_repo" "$base_sha" "$ui_sha"

assert_forbidden_path() {
  local forbidden_path="$1" bad_sha
  git -C "$git_repo" checkout -q -f "$ui_sha"
  mkdir -p "$git_repo/$(dirname -- "$forbidden_path")"
  printf 'forbidden %s\n' "$forbidden_path" > "$git_repo/$forbidden_path"
  git -C "$git_repo" add -- "$forbidden_path"
  git -C "$git_repo" commit -qm "forbidden $forbidden_path"
  bad_sha="$(git -C "$git_repo" rev-parse HEAD)"
  if (ui_assert_source_delta "$git_repo" "$base_sha" "$bad_sha") >/dev/null 2>&1; then
    test_fail "protected path passed the UI-only check: $forbidden_path"
  fi
}

for forbidden_path in \
  frontend/src/api/forbidden.ts \
  backend/migrations/001.sql \
  frontend/package.json \
  frontend/pnpm-lock.yaml \
  frontend/src/style.css \
  frontend/tailwind.config.js \
  frontend/src/router/index.ts \
  frontend/src/composables/useUnreviewedSurfacePerformance.ts \
  frontend/src/composables/__tests__/useUnreviewedSurfacePerformance.spec.ts \
  frontend/src/components/home/__tests__/UnreviewedRainPerformance.spec.ts \
  frontend/src/components/layout/__tests__/UnreviewedUserSurfacePerformance.spec.ts \
  frontend/src/views/HomeView.vue \
  frontend/src/components/common/LocaleSwitcher.vue \
  frontend/src/components/home/GlassPane.vue \
  frontend/src/components/home/RainGatewayHome.vue \
  frontend/src/components/home/RainGlyph.vue \
  frontend/src/components/home/__tests__/RainGatewayHome.spec.ts \
  frontend/public/rain-city-1.jpg \
  frontend/public/rain-city-2.jpg \
  frontend/public/rain-city-3.jpg; do
  assert_forbidden_path "$forbidden_path"
done

git -C "$git_repo" checkout -q -f "$base_sha"
printf 'evidence only\n' >> "$git_repo/SUBNEXUS_CHANGE_MEMORY.md"
git -C "$git_repo" commit -qam evidence-only
evidence_only_sha="$(git -C "$git_repo" rev-parse HEAD)"
if (ui_assert_source_delta "$git_repo" "$base_sha" "$evidence_only_sha") >/dev/null 2>&1; then
  test_fail 'evidence-only release satisfied the production UI count'
fi

assert_invalid_allowed_mode() {
  local mutation="$1" invalid_path="$2" bad_sha blob
  git -C "$git_repo" checkout -q -f "$base_sha"
  printf 'valid production UI change\n' >> "$git_repo/frontend/src/views/user/ActivityCenterView.vue"
  git -C "$git_repo" add -- frontend/src/views/user/ActivityCenterView.vue
  case "$mutation" in
    delete)
      git -C "$git_repo" rm -q -- "$invalid_path" ;;
    symlink)
      blob="$(printf 'elsewhere' | git -C "$git_repo" hash-object -w --stdin)"
      git -C "$git_repo" update-index --add --cacheinfo 120000 "$blob" "$invalid_path" ;;
    executable)
      git -C "$git_repo" update-index --chmod=+x "$invalid_path" ;;
    *) test_fail "unknown allowed-path mutation: $mutation" ;;
  esac
  git -C "$git_repo" commit -qm "invalid allowed path $mutation"
  bad_sha="$(git -C "$git_repo" rev-parse HEAD)"
  if (ui_assert_source_delta "$git_repo" "$base_sha" "$bad_sha") >/dev/null 2>&1; then
    test_fail "invalid allowed path mode passed: $mutation $invalid_path"
  fi
}

assert_invalid_allowed_mode delete frontend/src/views/user/InviteLotteryView.vue
assert_invalid_allowed_mode symlink SUBNEXUS_CHANGE_MEMORY.md
assert_invalid_allowed_mode executable tools/production-deploy/subnexus-ui-cutover.test.sh
printf 'UI cutover tests passed: %s fault/recovery cases and source-contract checks\n' "$case_count"
