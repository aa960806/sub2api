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
  app_name=production-app
  temporary_name=production-app-ui-prior-20260905010101-42
  old_name=original-subnexus
  run_dir="$fixture/run"
  manifest_file="$run_dir/manifest.env"
  store="$fixture/containers"
  if [[ "$resuming" != resume ]]; then
  printf '%s\n' immutable-original-run > "$fixture/original-evidence"
  printf '%s\n' "$settings" > "$fixture/settings"
  printf '%s\n' "$app_name" > "$store/$current.name"
  printf '%s\n' true > "$store/$current.running"
  printf '%s\n' "$old_name" > "$store/$original.name"
  printf '%s\n' false > "$store/$original.running"
  printf '%s\n' original-image > "$store/$original.image"
  printf '%s\n' \
    'state=prepared' 'ui_state=prepared' 'run_id=20260905010101-42' \
    "live_app_id=$current" "live_app_name=$app_name" 'live_app_image_id=sha256:current-image' \
    "target_sha=$target" "ui_base_sha=$base" 'source_root=/unused/source' \
    "candidate_image_id=$image_id" 'candidate_container_id=' 'candidate_container_name=' 'candidate_container_intent=' \
    "ui_rollback_id=$original" "ui_rollback_name=$old_name" 'ui_rollback_image=sha256:original-image' \
    "ui_temporary_name=$temporary_name" 'ui_commit_intent=no' "ui_settings_sha256=$settings" > "$manifest_file"
  printf 'prepared\n' > "$run_dir/READY"
  chmod 600 "$manifest_file"
  fi
  app_id="$current"
  expected_image_id="$image_id"
  target_sha="$target"
  stop_timeout_seconds=1
  SUBNEXUS_CUTOVER_CONFIRM=I_UNDERSTAND_SHORT_PRODUCTION_WINDOW
  SUBNEXUS_CUTOVER_QUIET_CONFIRM=I_HAVE_CHECKED_NO_SETTLEMENT_TASKS
  cutover_active=0
  rollback_active=0

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
  }
  mv() {
    local from="${@: -2:1}" destination="${@: -1}"
    if [[ "$destination" == "$manifest_file" && "$resuming" != resume ]]; then
      if [[ "$scenario" == recovery_state_failed ]] && grep -Fxq 'state=rolling_back' "$from"; then return 1; fi
      if [[ "$scenario" == commit_state_persistent ]] && grep -Fxq 'state=switched' "$from"; then return 1; fi
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
        if [[ "$scenario" == signal && "$id" == "$replacement" ]]; then kill -TERM "$$"; fi ;;
      container)
        [[ "$2" == rm && "$#" == 3 ]] || fail 'fixture forbids force/volume removal'
        id="$3"
        [[ "$(cat "$store/$id.running")" == false ]] || fail 'attempted to remove a running container'
        if [[ "$scenario" == remove_failed && "$id" == "$current" ]]; then return 1; fi
        rm -- "$store/$id.name" "$store/$id.running"
        if [[ "$scenario" == remove_response_lost && "$id" == "$current" ]] || [[ "$scenario" == rollback_remove_response_lost && "$id" == "$replacement" ]]; then return 1; fi ;;
      logs) printf 'bounded previous application log\n' ;;
      *) fail "unexpected fixture Docker operation: $*" ;;
    esac
  }
  ui_assert_anchor() {
    [[ "$(cat "$store/$original.image")" == original-image ]] || fail 'original rollback image drifted'
    if [[ "${1:-}" == restored && "$(cat "$store/$original.name")" == "$app_name" ]]; then return 0; fi
    [[ "$(cat "$store/$original.name")" == "$old_name" && "$(cat "$store/$original.running")" == false ]] || fail 'original rollback identity or state drifted'
  }
  ui_restore_anchor() {
    ui_assert_anchor restored
    if [[ "$(cat "$store/$original.name")" != "$app_name" ]]; then docker_rpc rename "$original" "$app_name"; fi
    docker_rpc start "$original"
  }
  assert_runtime_still_matches_prepare() {
    [[ "$(inspect_container_id_or_empty "$app_name")" == "$current" && "$(cat "$store/$current.running")" == true ]] || fail 'current runtime drifted'
  }
  assert_preserved_container_contract() {
    [[ -f "$store/$current.name" ]] || fail 'current identity missing'
    case "$(cat "$store/$current.name")" in "$temporary_name"|"$app_name") ;; *) fail 'current name drifted' ;; esac
  }
  create_candidate_container() {
    manifest_set candidate_container_name "$app_name"
    manifest_set candidate_container_intent "$(printf '3%.0s' {1..64})"
    [[ -z "$(inspect_container_id_or_empty "$app_name")" ]] || fail 'candidate name occupied'
    printf '%s\n' "$app_name" > "$store/$replacement.name"
    printf 'false\n' > "$store/$replacement.running"
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
    ui_assert_settings_unchanged
  }
  restore_preserved_container() {
    assert_preserved_container_contract
    [[ -z "$(inspect_container_id_or_empty "$app_name")" || "$(inspect_container_id_or_empty "$app_name")" == "$current" ]] || fail 'recovery name occupied'
    if [[ "$(cat "$store/$current.name")" != "$app_name" ]]; then docker_rpc rename "$current" "$app_name"; fi
    docker_rpc start "$current"
    [[ "$scenario" != restore_unhealthy ]]
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
    occupied_temporary)
      printf '%s\n' "$temporary_name" > "$store/$stranger.name"
      printf 'false\n' > "$store/$stranger.running" ;;
    recover_interrupted)
      manifest_set state switching; manifest_set ui_state switching
      docker_rpc stop --time 1 "$current"; docker_rpc rename "$current" "$temporary_name"
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
  if [[ "$scenario" == rollback || "$scenario" == rollback_interrupted || "$scenario" == rollback_marker_interrupted || "$scenario" == rollback_remove_response_lost ]]; then
    # Administrator updates made after deployment must survive rollback.
    printf '%s\n' "$(printf '5%.0s' {1..64})" > "$fixture/settings"
    if [[ "$scenario" == rollback_interrupted ]]; then
      manifest_set state rolling_back; manifest_set ui_state rolling_back
      ui_remove_candidate
    fi
    SUBNEXUS_CUTOVER_CONFIRM=I_UNDERSTAND_APPLICATION_ROLLBACK
    ui_manual_rollback "$run_dir"
    ui_manual_rollback "$run_dir"
  fi
  exit
fi

bash -n "$subject"
bash "$0" --library
bash "$0" --timeout prepare
for phase in switch rollback recover; do
  if bash "$0" --timeout "$phase" >/dev/null 2>&1; then test_fail "$phase accepted a prepare-only Docker timeout"; fi
done
[[ "$(sha256sum "$controller" | awk '{print $1}')" == 19824a87e3e1de5659cb30664750b71c5c10d374f25bda7f52e6524fe477ee65 ]] || test_fail 'approved migration controller changed'
if grep -n $'\r' "$subject"; then test_fail 'UI controller must use LF'; fi
for forbidden in 'docker commit' 'docker build' 'docker image save' 'docker image tag' 'docker system prune' 'nginx -s'; do
  if grep -Fq "$forbidden" "$subject"; then test_fail "forbidden UI controller operation: $forbidden"; fi
done
grep -Fq 'default_transaction_read_only=on' "$subject" || test_fail 'database query boundary is not read-only'
grep -Fq "close_rollout_gates() { fail" "$subject" || test_fail 'gate mutation not blocked'
grep -Fq "restore_rollout_gates() { fail" "$subject" || test_fail 'settings restoration not blocked'
grep -Fq 'docker_rpc container rm "$id"' "$subject" || test_fail 'temporary removal is not an ordinary exact-ID rm'

root="$(mktemp -d /tmp/subnexus-ui-tests.XXXXXX)"
trap 'rm -rf -- "$root"' EXIT
current="$(printf 'a%.0s' {1..64})"
original="$(printf 'b%.0s' {1..64})"
replacement="$(printf 'c%.0s' {1..64})"
temporary_name=production-app-ui-prior-20260905010101-42
case_count=0
for scenario in success stop_failed rename_failed start_failed created_without_id contract_failed unhealthy restore_unhealthy recovery_state_failed signal remove_failed remove_response_lost commit_marker_failed commit_state_failed commit_state_persistent commit_marker_interrupted settings_drift anchor_drift occupied_temporary recover_interrupted rollback rollback_interrupted rollback_without_id rollback_from_recovered rollback_marker_interrupted rollback_remove_response_lost; do
  fixture="$root/$scenario"
  mkdir "$fixture"
  rc=0
  bash "$0" --case "$scenario" "$fixture" > "$fixture/output" 2>&1 || rc=$?
  case "$scenario" in success|remove_response_lost|recover_interrupted|rollback|rollback_interrupted|rollback_without_id|rollback_from_recovered|rollback_remove_response_lost) [[ "$rc" == 0 ]] || { cat "$fixture/output"; test_fail "$scenario failed ($rc)"; } ;; *) [[ "$rc" != 0 ]] || test_fail "$scenario unexpectedly succeeded" ;; esac
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
      [[ ! -f "$fixture/containers/$current.name" && "$(cat "$fixture/containers/$replacement.running")" == true ]] || test_fail 'failed completion metadata lost the healthy replacement'
      [[ -f "$fixture/run/SWITCHED" ]] || test_fail 'failed completion metadata lost the completed marker'
      grep -Fxq 'state=switching' "$fixture/run/manifest.env" || test_fail 'failed completion metadata was not retryable'
      if grep -Eq 'UI_RECOVERED_CURRENT=|UI_SWITCH_COMPLETED=' "$fixture/output"; then test_fail 'failed completion metadata reported success'; fi ;;
    success|remove_response_lost|commit_marker_failed|commit_state_failed|commit_marker_interrupted)
      [[ ! -f "$fixture/containers/$current.name" ]] || test_fail "$scenario retained a new permanent rollback container"
      [[ "$(cat "$fixture/containers/$replacement.name")" == production-app && "$(cat "$fixture/containers/$replacement.running")" == true ]] || test_fail "$scenario lost the committed candidate"
      grep -Fxq 'ui_state=switched' "$fixture/run/manifest.env" || test_fail "$scenario failed commit reconciliation" ;;
    rollback|rollback_interrupted|rollback_without_id|rollback_from_recovered|rollback_marker_interrupted|rollback_remove_response_lost)
      [[ "$(cat "$fixture/containers/$original.name")" == production-app && "$(cat "$fixture/containers/$original.running")" == true ]] || test_fail 'manual rollback did not restore the fixed original'
      [[ ! -f "$fixture/containers/$replacement.name" && ! -f "$fixture/containers/$current.name" ]] || test_fail 'manual rollback retained replacement/current'
      [[ "$(cat "$fixture/settings")" == "$(printf '5%.0s' {1..64})" ]] || test_fail 'manual rollback overwrote administrator settings'
      grep -Fxq 'ui_state=rolled_back_to_original' "$fixture/run/manifest.env" || test_fail 'manual rollback state missing' ;;
    *)
      [[ "$(cat "$fixture/containers/$current.name")" == production-app && "$(cat "$fixture/containers/$current.running")" == true ]] || { cat "$fixture/output"; test_fail "$scenario did not retain/recover current"; }
      [[ ! -f "$fixture/containers/$replacement.name" ]] || test_fail "$scenario left a failed candidate" ;;
  esac
  if [[ "$scenario" != rollback* ]]; then
    [[ "$(cat "$fixture/containers/$original.name")" == original-subnexus && "$(cat "$fixture/containers/$original.running")" == false ]] || test_fail "$scenario mutated the original rollback target"
  fi
  [[ "$(cat "$fixture/original-evidence")" == immutable-original-run ]] || test_fail 'original rollback evidence changed'
  if [[ "$scenario" == restore_unhealthy ]]; then
    [[ ! -f "$fixture/run/ROLLED_BACK" ]] || test_fail 'unhealthy restoration reported rollback success'
    grep -Fxq 'state=rolling_back' "$fixture/run/manifest.env" || test_fail 'unhealthy restoration did not retain a resumable state'
  fi
  if [[ -f "$fixture/actions" ]] && grep -E 'commit|image (save|tag)|rm --force|rm -v' "$fixture/actions"; then test_fail 'fixture observed forbidden Docker operation'; fi
  case_count=$((case_count + 1))
done

# Exercise the real Git allowlist rather than mirroring it with a fixture.
source <(head -n -1 "$controller")
export PATH="$fixture_path"
source "$subject"
git_repo="$root/source"
mkdir "$git_repo"
git -C "$git_repo" init -q
git -C "$git_repo" config user.name fixture
git -C "$git_repo" config user.email fixture@example.invalid
git -C "$git_repo" config core.autocrlf false
mkdir -p "$git_repo/frontend/src/views" "$git_repo/backend/migrations"
mkdir -p "$git_repo/frontend/public"
printf 'old homepage\n' > "$git_repo/frontend/src/views/HomeView.vue"
printf 'SELECT 1;\n' > "$git_repo/backend/migrations/001.sql"
git -C "$git_repo" add .
git -C "$git_repo" commit -qm base
base_sha="$(git -C "$git_repo" rev-parse HEAD)"
printf 'new homepage\n' > "$git_repo/frontend/src/views/HomeView.vue"
printf 'test bitmap\n' > "$git_repo/frontend/public/rain-city-1.jpg"
git -C "$git_repo" add frontend/public/rain-city-1.jpg
git -C "$git_repo" commit -qam ui
ui_sha="$(git -C "$git_repo" rev-parse HEAD)"
ui_assert_source_delta "$git_repo" "$base_sha" "$ui_sha"
printf 'SELECT 2;\n' > "$git_repo/backend/migrations/001.sql"
git -C "$git_repo" commit -qam migration
bad_sha="$(git -C "$git_repo" rev-parse HEAD)"
if (ui_assert_source_delta "$git_repo" "$base_sha" "$bad_sha") >/dev/null 2>&1; then test_fail 'backend/migration drift passed the UI-only check'; fi
printf 'UI cutover tests passed: %s fault/recovery cases and source-contract checks\n' "$case_count"
