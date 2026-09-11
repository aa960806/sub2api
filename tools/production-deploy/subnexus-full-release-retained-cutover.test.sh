#!/usr/bin/env bash
set -Eeuo pipefail
script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
subject="$script_dir/subnexus-full-release-retained-cutover.sh"
controller="$script_dir/subnexus-production-cutover.sh"
ui_library="$script_dir/subnexus-ui-cutover.sh"
fixture_path="$PATH"

test_fail() { printf 'RETAINED RELEASE TEST ERROR: %s\n' "$*" >&2; exit 1; }

if [[ "${1:-}" == --case ]]; then
  scenario="$2"; fixture="$3"; resuming="${4:-}"
  mkdir -p "$fixture/run" "$fixture/containers"
  source <(head -n -1 "$controller")
  export PATH="$fixture_path"
  source "$ui_library"
  source "$subject"
  ui_install_overrides
  retained_install_overrides
  current="$retained_live_id"; original="$retained_id"; replacement="$(printf 'c%.0s' {1..64})"; stranger="$(printf 'd%.0s' {1..64})"
  current_image="${retained_live_image#sha256:}"; original_image="${retained_image#sha256:}"; image_id="$(printf '2%.0s' {1..64})"
  current_config=subnexus:current; old_config=subnexus:retained
  app_name=production-app; temporary_name=production-app-ui-prior-20260912010101-42; old_name="$retained_name"
  run_dir="$fixture/run"; manifest_file="$run_dir/manifest.env"; store="$fixture/containers"
  settings="$(printf '1%.0s' {1..64})"
  if [[ "$resuming" != resume ]]; then
    mkdir "$fixture/source-anchor" "$run_dir/retained-contract"
    for id in "$current" "$original"; do
      if [[ "$id" == "$current" ]]; then name="$app_name"; running=true; image="$current_image"; configured="$current_config"; else name="$old_name"; running=false; image="$original_image"; configured="$old_config"; fi
      printf '%s\n' "$name" > "$store/$id.name"; printf '%s\n' "$running" > "$store/$id.running"
      printf '%s\n' "$image" > "$store/$id.image"; printf '%s\n' "$configured" > "$store/$id.config-image"
    done
    printf '%s\n' "$settings" > "$fixture/settings"
    printf '%s\n' 'state=prepared' 'ui_state=prepared' 'run_id=20260912010101-42' \
      "live_app_id=$current" "live_app_name=$app_name" "live_app_image_id=sha256:$current_image" \
      "target_sha=$retained_base_commit" "ui_base_sha=$retained_base_commit" 'source_root=/unused/source' \
      "candidate_image_id=$image_id" 'candidate_container_id=' 'candidate_container_name=' 'candidate_container_intent=' \
      "ui_rollback_id=$original" "ui_rollback_name=$old_name" "ui_rollback_image=$retained_image" \
      "ui_anchor_run=$run_dir/retained-contract" "ui_anchor_manifest_sha256=$retained_source_manifest_sha" 'ui_anchor_state=present' \
      "ui_new_rollback_id=$current" "ui_new_rollback_image=sha256:$current_image" "ui_new_rollback_config_image=$current_config" \
      "ui_new_rollback_name=$temporary_name" 'ui_new_rollback_state=prepared' "ui_temporary_name=$temporary_name" \
      'ui_commit_intent=no' "ui_settings_sha256=$settings" 'retained_current_policy=temporary-until-commit' \
      'retained_commit_phase=none' > "$manifest_file"
    printf 'prepared\n' > "$run_dir/READY"
  fi
  app_id="$current"; target_sha="$retained_base_commit"; expected_image_id="$image_id"; stop_timeout_seconds=1
  cutover_active=0; rollback_active=0
  SUBNEXUS_CUTOVER_CONFIRM=I_UNDERSTAND_SHORT_PRODUCTION_WINDOW
  SUBNEXUS_CUTOVER_QUIET_CONFIRM=I_HAVE_CHECKED_NO_SETTLEMENT_TASKS
  require_commands() { :; }; ulimit() { :; }
  assert_root_owned_regular() { [[ -f "$1" && ! -L "$1" ]] || fail 'invalid fixture file'; }
  ui_load_run() {
    [[ "$1" == "$run_dir" ]] || fail 'wrong fixture run'
    app_id="$current"; app_name=production-app; target_sha="$retained_base_commit"; expected_image_id="$image_id"
    case "$(manifest_value state)" in
      prepared) [[ ! -e "$run_dir/SWITCHED" && ! -e "$run_dir/ROLLED_BACK" ]] || fail 'prepared marker drift' ;;
      switching) [[ ! -e "$run_dir/ROLLED_BACK" ]] || fail 'switching marker drift' ;;
      switched) assert_run_marker SWITCHED switched ;;
      rolling_back) ;;
      rolled_back) assert_run_marker ROLLED_BACK rolled_back ;;
      *) fail 'invalid state' ;;
    esac
    ui_validate_new_rollback_fields
  }
  retained_validate_manifest() { [[ "$(manifest_value retained_current_policy)" == temporary-until-commit ]] || fail 'wrong policy'; }
  retained_assert_source() { :; }; ui_assert_base_image() { :; }
  assert_daemon_still_matches_prepare() { :; }; assert_dependencies_still_match() { :; }
  assert_app_data_source_identity() { :; }; assert_prepared_networks_still_match() { :; }
  ui_settings_hash() { cat "$fixture/settings"; }
  inspect_container_id_or_empty() {
    local ref="$1" file
    if [[ -f "$store/$ref.name" ]]; then printf '%s' "$ref"; return; fi
    for file in "$store"/*.name; do
      [[ -f "$file" ]] || continue
      if [[ "$(cat "$file")" == "$ref" ]]; then file="${file##*/}"; printf '%s' "${file%.name}"; return; fi
    done
  }
  assert_runtime_still_matches_prepare() { [[ "$(inspect_container_id_or_empty "$app_name")" == "$current" && "$(cat "$store/$current.running")" == true ]] || fail 'live runtime changed'; }
  assert_preserved_container_contract() {
    [[ -f "$store/$current.name" ]] || fail 'temporary current missing'
    case "$(cat "$store/$current.name")" in "$temporary_name"|"$app_name") ;; *) fail 'temporary name changed' ;; esac
    [[ "$(cat "$store/$current.image")" == "$current_image" && "$(cat "$store/$current.config-image")" == "$current_config" ]] || fail 'temporary identity drift'
  }
  retained_assert_fallback() {
    [[ -d "$run_dir/retained-contract" && -f "$store/$original.name" ]] || fail 'retained object/contract missing'
    [[ "$(cat "$store/$original.image")" == "$original_image" && "$(cat "$store/$original.config-image")" == "$old_config" ]] || fail 'retained identity drift'
    case "${1:-stopped}" in
      stopped) [[ "$(cat "$store/$original.name")" == "$old_name" && "$(cat "$store/$original.running")" == false ]] || fail 'retained not stopped' ;;
      restored) [[ "$(cat "$store/$original.name")" == "$app_name" && "$(cat "$store/$original.running")" == true ]] || fail 'retained not restored' ;;
      any) case "$(cat "$store/$original.name")" in "$old_name"|"$app_name") ;; *) fail 'retained name drift' ;; esac ;;
      *) fail 'unexpected retained mode' ;;
    esac
  }
  mv() {
    local from="${@: -2:1}" destination="${@: -1}"
    if [[ "$resuming" != resume && ! -e "$fixture/fault-fired" ]]; then
      if [[ "$scenario" == commit_marker_failed && "$destination" == "$run_dir/SWITCHED" ]] ||
         { [[ "$scenario" == commit_state_failed && "$destination" == "$manifest_file" ]] && grep -Fxq state=switched "$from"; }; then
        touch "$fixture/fault-fired"; return 1
      fi
      if [[ "$scenario" == commit_marker_interrupted && "$destination" == "$run_dir/SWITCHED" ]] ||
         [[ "$scenario" == rollback_marker_interrupted && "$destination" == "$run_dir/ROLLED_BACK" ]]; then
        touch "$fixture/fault-fired"; command mv "$@"; exit 77
      fi
    fi
    command mv "$@"
  }
  docker_rpc() {
    printf '%s\n' "$*" >> "$fixture/actions"
    local id format
    case "$1" in
      image) [[ "$2" == inspect ]] || fail 'image mutation forbidden'; printf 'sha256:%s\n' "$image_id" ;;
      inspect)
        format="$3"; id="$(inspect_container_id_or_empty "$4")"; [[ -n "$id" ]] || return 1
        case "$format" in
          '{{.Name}}') printf '/%s\n' "$(cat "$store/$id.name")" ;;
          '{{.Image}}') printf 'sha256:%s\n' "$(cat "$store/$id.image")" ;;
          '{{.Config.Image}}') cat "$store/$id.config-image" ;;
          '{{.State.Running}}') cat "$store/$id.running" ;;
          *) fail 'unexpected inspect' ;;
        esac ;;
      stop)
        id="${@: -1}"; [[ "$scenario" != stop_failed || "$id" != "$current" ]] || return 1
        printf 'false\n' > "$store/$id.running" ;;
      rename)
        id="$2"
        [[ "$scenario" != rename_failed || "$id" != "$current" || "$3" != "$temporary_name" ]] || return 1
        [[ -z "$(inspect_container_id_or_empty "$3")" ]] || return 1
        printf '%s\n' "$3" > "$store/$id.name"
        if [[ "$scenario" == stopped_current_drift && "$id" == "$current" && "$3" == "$temporary_name" ]]; then printf '%s\n' "$stranger" > "$store/$current.image"; fi ;;
      start)
        id="$2"; [[ "$scenario" != start_failed || "$id" != "$replacement" ]] || return 1
        printf 'true\n' > "$store/$id.running"
        if [[ "$scenario" == signal && "$id" == "$replacement" ]]; then kill -TERM "$$"; fi ;;
      container)
        [[ "$2" == rm && "$#" == 3 ]] || fail 'forbidden force/volume removal'
        id="$3"; [[ "$id" != "$original" ]] || fail 'retained target deletion forbidden'
        [[ "$(cat "$store/$id.running")" == false ]] || fail 'removed running container'
        if [[ "$scenario" == remove_current_failed && "$id" == "$current" ]] || [[ "$scenario" == remove_candidate_failed && "$id" == "$replacement" ]]; then return 1; fi
        rm -f -- "$store/$id.name" "$store/$id.running" "$store/$id.image" "$store/$id.config-image" "$store/$id.role"
        if [[ "$scenario" == remove_current_response_lost && "$id" == "$current" ]] || [[ "$scenario" == remove_candidate_response_lost && "$id" == "$replacement" ]]; then return 1; fi ;;
      logs) [[ "$scenario" != log_failed ]] || return 1; printf 'bounded diagnostic log\n' ;;
      *) fail 'unexpected Docker operation' ;;
    esac
  }
  create_candidate_container() {
    manifest_set candidate_container_name "$app_name"; manifest_set candidate_container_intent "$(printf '3%.0s' {1..64})"
    [[ -z "$(inspect_container_id_or_empty "$app_name")" ]] || fail 'candidate name occupied'
    printf '%s\n' "$app_name" > "$store/$replacement.name"; printf 'false\n' > "$store/$replacement.running"
    printf '%s\n' candidate-image > "$store/$replacement.image"; printf '%s\n' candidate-config > "$store/$replacement.config-image"
    printf candidate > "$store/$replacement.role"
    printf 'create candidate %s\n' "$replacement" >> "$fixture/actions"
    [[ "$scenario" != created_without_id ]] || fail 'candidate create response lost'
    candidate_id="$replacement"; manifest_set candidate_container_id "$replacement"; printf '%s\n' "$replacement" > "$run_dir/candidate-container-id"
  }
  assert_candidate_container_identity() { [[ "$1" == "$replacement" && -f "$store/$replacement.role" && "$(cat "$store/$replacement.name")" == "$app_name" ]] || fail 'candidate identity mismatch'; }
  assert_candidate_runtime_contract() { [[ "$scenario" != contract_failed ]] || fail 'candidate runtime mismatch'; }
  wait_for_candidate_health() { [[ "$scenario" != unhealthy && "$scenario" != remove_candidate_failed && "$scenario" != remove_candidate_response_lost ]]; }
  validate_candidate_runtime() {
    assert_candidate_container_identity "$candidate_id"
    if [[ "$scenario" == retained_lost_during_switch ]]; then rm -- "$store/$original.name"; fi
    if [[ "$scenario" == settings_drift ]]; then printf changed > "$fixture/settings"; fi
    ui_assert_settings_unchanged
  }
  restore_preserved_container() {
    assert_preserved_container_contract
    [[ -z "$(inspect_container_id_or_empty "$app_name")" || "$(inspect_container_id_or_empty "$app_name")" == "$current" ]] || fail 'current recovery port occupied'
    if [[ "$(cat "$store/$current.name")" != "$app_name" ]]; then docker_rpc rename "$current" "$app_name"; fi
    if [[ "$(cat "$store/$current.running")" != true ]]; then docker_rpc start "$current"; fi
    [[ "$scenario" != recovery_unhealthy ]]
  }
  retained_restore_fallback() {
    retained_assert_fallback any
    [[ -z "$(inspect_container_id_or_empty "$app_name")" || "$(inspect_container_id_or_empty "$app_name")" == "$original" ]] || fail 'fallback port occupied'
    if [[ "$(cat "$store/$original.name")" != "$app_name" ]]; then docker_rpc rename "$original" "$app_name"; fi
    if [[ "$(cat "$store/$original.running")" != true ]]; then docker_rpc start "$original"; fi
    [[ "$scenario" != rollback_unhealthy ]]
  }
  if [[ "$resuming" == resume ]]; then
    SUBNEXUS_CUTOVER_CONFIRM=I_UNDERSTAND_APPLICATION_ROLLBACK
    case "$scenario" in commit_marker_interrupted) retained_recover_entry "$run_dir" ;; rollback_marker_interrupted) retained_manual_rollback "$run_dir" ;; *) fail 'unknown resume' ;; esac
    exit
  fi
  case "$scenario" in
    retained_missing) rm -- "$store/$original.name" ;;
    retained_image_drift) printf bad > "$store/$original.image" ;;
    retained_config_drift) printf bad > "$store/$original.config-image" ;;
    retained_name_drift) printf wrong-name > "$store/$original.name" ;;
    occupied_temporary) printf '%s\n' "$temporary_name" > "$store/$stranger.name" ;;
  esac
  retained_switch "$run_dir"
  case "$scenario" in
    rollback*|source_anchor_missing_rollback)
      if [[ "$scenario" == source_anchor_missing_rollback ]]; then rmdir "$fixture/source-anchor"; fi
      if [[ "$scenario" == rollback_target_missing ]]; then rm -- "$store/$original.name"; fi
      if [[ "$scenario" == rollback_target_drift ]]; then printf bad > "$store/$original.image"; fi
      SUBNEXUS_CUTOVER_CONFIRM=I_UNDERSTAND_APPLICATION_ROLLBACK
      retained_manual_rollback "$run_dir"
      if [[ "$scenario" == rollback_repeat ]]; then retained_manual_rollback "$run_dir"; fi ;;
  esac
  exit
fi

root="$(mktemp -d /tmp/subnexus-retained-test.XXXXXX)"
trap 'rm -rf -- "$root"' EXIT
source <(head -n -1 "$controller")
export PATH="$fixture_path"
source "$ui_library"
source "$subject"
bash -n "$subject"
[[ "$(hash_file "$controller")" == "$retained_controller_sha" && "$(hash_file "$ui_library")" == "$retained_ui_sha" ]] || test_fail 'library pin drift'
assert_approved_path() { [[ -f "$1" && "$1" == "$root/"* ]] || fail 'unapproved fixture path'; printf '%s|%s\n' "$1" "$root"; }
assert_root_owned_regular() { [[ -f "$1" && ! -L "$1" ]] || fail 'fixture is not regular'; }
retained_target="$retained_base_commit"
retained_tree="$(git -C "$script_dir/../.." rev-parse "$retained_target^{tree}")"
retained_candidate_image="sha256:$(printf '1%.0s' {1..64})"
retained_gate="$root/gate.env"; backup="$(printf '3%.0s' {1..64})"
write_gate() {
  printf '%s\n' version=full-release-retained-compat-v1 result=passed \
    "candidate_commit=$retained_target" "candidate_tree=$retained_tree" "candidate_image=$retained_candidate_image" \
    "previous_live_image=$retained_live_image" "retained_rollback_image=$retained_image" "backup_sha256=$backup" \
    new_old_new=passed new_rollback_new=passed migration_contract=passed api_regression=passed cleanup=passed > "$retained_gate"
  retained_gate_sha="$(hash_file "$retained_gate")"
}
write_gate; retained_validate_gate
[[ "$retained_backup_sha" == "$backup" ]] || test_fail 'backup SHA not captured'
for key in version result candidate_commit candidate_tree candidate_image previous_live_image retained_rollback_image backup_sha256 new_old_new new_rollback_new migration_contract api_regression cleanup; do
  write_gate
  sed -i "s/^$key=.*/$key=wrong/" "$retained_gate"; retained_gate_sha="$(hash_file "$retained_gate")"
  if (retained_validate_gate) > "$root/reject-$key.log" 2>&1; then test_fail "gate accepted wrong $key"; fi
done
for key in result unexpected; do
  write_gate; printf '%s=passed\n' "$key" >> "$retained_gate"; retained_gate_sha="$(hash_file "$retained_gate")"
  if (retained_validate_gate) >/dev/null 2>&1; then test_fail "gate accepted duplicate/unknown $key"; fi
done
write_gate
if (
  require_commands() { :; }; init_docker() { :; }
  docker_rpc() { [[ "$1" == inspect ]] || fail 'prepare performed Docker mutation'; if [[ "$3" == '{{.Id}}' ]]; then printf '%s\n' "$retained_live_id"; else printf '%s\n' "$retained_live_image"; fi; }
  ui_prepare() { touch "$root/unexpected-prepare"; }
  retained_prepare /unused "$retained_target" "$retained_tree" "${retained_candidate_image#sha256:}" /unused/archive "$backup" /unused/gate live "$retained_gate" "$(printf '0%.0s' {1..64})"
) > "$root/prepare-admission.log" 2>&1; then test_fail 'prepare accepted bad gate'; fi
[[ ! -e "$root/unexpected-prepare" ]] || test_fail 'backup reached before compatibility admission'

# A successful prepare only writes release evidence, preserving the pinned
# library boundary while rejecting any Docker mutation. The source anchor is
# represented by a fixture; its separate on-disk hashing is exercised below.
(
  require_commands() { :; }; init_docker() { :; }
  docker_rpc() {
    [[ "$1" == inspect ]] || fail 'prepare attempted Docker mutation'
    if [[ "$3" == '{{.Id}}' ]]; then printf '%s\n' "$retained_live_id"; else printf '%s\n' "$retained_live_image"; fi
  }
  ui_prepare() {
    [[ "$3" == "$retained_base_commit" && "$9" == "$retained_source_anchor" && "${10}" == "$retained_id" && "${11}" == "$retained_image" && "${12}" == "$retained_name" ]] || fail 'prepare changed existing fallback target'
    run_dir="$root/prepared"; mkdir "$run_dir"; manifest_file="$run_dir/manifest.env"
    printf '%s\n' "live_app_id=$retained_live_id" "live_app_image_id=$retained_live_image" > "$manifest_file"
  }
  retained_copy_contract() { touch "$run_dir/fixture-contract-copied"; }
  retained_prepare /unused "$retained_target" "$retained_tree" "${retained_candidate_image#sha256:}" /unused/archive "$backup" /unused/gate live "$retained_gate" "$retained_gate_sha"
  [[ "$(manifest_value retained_current_policy)" == temporary-until-commit && "$(manifest_value retained_commit_phase)" == none && -f "$run_dir/RETAINED_READY" ]] || fail 'prepare readiness/temporary policy missing'
) > "$root/prepare-success.log" 2>&1 || { cat "$root/prepare-success.log"; test_fail 'evidence-only prepare failed'; }

# Actual hash-list validation includes only the fixed metadata allowlist.
(
  run_dir="$root/contract-run"; mkdir -p "$run_dir/retained-contract"; manifest_file="$run_dir/manifest.env"
  assert_root_owned_dir() { [[ -d "$1" && ! -L "$1" ]] || fail 'fixture directory invalid'; }
  for file in "${retained_metadata_files[@]}"; do printf '%s\n' "$file" > "$run_dir/retained-contract/$file"; done
  (cd "$run_dir/retained-contract"; sha256sum -- "${retained_metadata_files[@]}") > "$run_dir/retained-contract.sha256"
  printf '%s\n' "ui_anchor_run=$run_dir/retained-contract" "retained_source_anchor=$retained_source_anchor" \
    "retained_source_manifest_sha256=$retained_source_manifest_sha" "retained_contract_sha256=$(hash_file "$run_dir/retained-contract.sha256")" > "$manifest_file"
  retained_assert_contract_copy
  printf changed >> "$run_dir/retained-contract/container.env"
  if (retained_assert_contract_copy) >/dev/null 2>&1; then test_fail 'copied secret/runtime metadata tamper accepted'; fi
) || test_fail 'retained contract copy validation failed'

# Exact source manifest tampering is rejected before the legacy run validator,
# any Docker operation, or any attempt to restore the retained container.
(
  mkdir "$root/anchor-tamper"; printf bad > "$root/anchor-tamper/manifest.env"
  validate_run_directory() { touch "$root/unexpected-anchor-validation"; }
  if (retained_anchor_context "$root/anchor-tamper" "$retained_id" "$retained_image" "$retained_name" "$retained_source_manifest_sha" stopped) >/dev/null 2>&1; then test_fail 'tampered source anchor accepted'; fi
  [[ ! -e "$root/unexpected-anchor-validation" ]] || test_fail 'source anchor admitted before pin validation'
) || test_fail 'retained source pin gate failed'

# Real Git identities, clean detached checkout, and gate revalidation.
git clone -q --shared --no-checkout "$script_dir/../.." "$root/source"
git -C "$root/source" -c core.autocrlf=false checkout -q --detach "$retained_target"
retained_assert_source "$root/source" "$retained_base_commit" "$retained_target"
printf dirty > "$root/source/untracked"
if (retained_assert_source "$root/source" "$retained_base_commit" "$retained_target") >/dev/null 2>&1; then test_fail 'dirty source accepted'; fi
rm -- "$root/source/untracked"
git -C "$root/source" switch -q -c attached-fixture
if (retained_assert_source "$root/source" "$retained_base_commit" "$retained_target") >/dev/null 2>&1; then test_fail 'attached source accepted'; fi

count=0
for scenario in success stop_failed rename_failed start_failed created_without_id contract_failed unhealthy signal log_failed settings_drift stopped_current_drift retained_missing retained_image_drift retained_config_drift retained_name_drift retained_lost_during_switch occupied_temporary remove_current_failed remove_current_response_lost remove_candidate_failed remove_candidate_response_lost commit_marker_failed commit_state_failed commit_marker_interrupted rollback rollback_repeat rollback_target_missing rollback_target_drift rollback_unhealthy rollback_marker_interrupted source_anchor_missing_rollback; do
  fixture="$root/$scenario"; mkdir "$fixture"
  set +e
  bash "$BASH_SOURCE" --case "$scenario" "$fixture" > "$fixture/output" 2>&1
  rc=$?
  set -e
  case "$scenario" in success|remove_current_response_lost|rollback|rollback_repeat|source_anchor_missing_rollback) [[ "$rc" == 0 ]] || { cat "$fixture/output"; test_fail "$scenario failed ($rc)"; } ;; *) [[ "$rc" != 0 ]] || test_fail "$scenario unexpectedly passed" ;; esac
  if [[ "$scenario" == commit_marker_interrupted || "$scenario" == rollback_marker_interrupted ]]; then
    [[ "$rc" == 77 ]] || { cat "$fixture/output"; test_fail 'interruption did not reach expected boundary'; }
    bash "$BASH_SOURCE" --case "$scenario" "$fixture" resume > "$fixture/resume-output" 2>&1 || { cat "$fixture/resume-output"; test_fail "$scenario resume failed"; }
  fi
  current="$retained_live_id"; original="$retained_id"; replacement="$(printf 'c%.0s' {1..64})"
  case "$scenario" in
    success|remove_current_response_lost|commit_marker_failed|commit_state_failed|commit_marker_interrupted)
      [[ ! -f "$fixture/containers/$current.name" && "$(cat "$fixture/containers/$replacement.running")" == true && -f "$fixture/run/SWITCHED" ]] || { cat "$fixture/output"; test_fail "$scenario did not retain candidate and remove temporary current"; }
      [[ "$(cat "$fixture/containers/$original.running")" == false ]] || test_fail 'success changed fallback' ;;
    rollback|rollback_repeat|rollback_marker_interrupted|source_anchor_missing_rollback)
      [[ ! -f "$fixture/containers/$current.name" && ! -f "$fixture/containers/$replacement.name" ]] || test_fail 'rollback retained current/candidate'
      [[ "$(cat "$fixture/containers/$original.name")" == production-app && "$(cat "$fixture/containers/$original.running")" == true && -f "$fixture/run/ROLLED_BACK" ]] || test_fail 'rollback did not restore existing target' ;;
    rollback_target_missing|rollback_target_drift)
      [[ "$(cat "$fixture/containers/$replacement.running")" == true ]] || test_fail 'invalid fallback destroyed candidate' ;;
    retained_missing|retained_image_drift|retained_config_drift|retained_name_drift|occupied_temporary)
      [[ "$(cat "$fixture/containers/$current.running")" == true && ! -f "$fixture/containers/$replacement.name" ]] || test_fail 'admission mutated live' ;;
    stopped_current_drift)
      [[ ! -f "$fixture/containers/$replacement.name" && ! -f "$fixture/run/SWITCHED" ]] || test_fail 'stopped drift created candidate' ;;
    remove_candidate_failed)
      [[ -f "$fixture/containers/$replacement.name" && "$(cat "$fixture/containers/$current.running")" == false && ! -f "$fixture/run/ROLLED_BACK" ]] || test_fail 'candidate removal failure was hidden' ;;
    rollback_unhealthy)
      [[ ! -f "$fixture/run/ROLLED_BACK" ]] || test_fail 'unhealthy fallback declared success' ;;
    *)
      [[ "$(cat "$fixture/containers/$current.name")" == production-app && "$(cat "$fixture/containers/$current.running")" == true && ! -f "$fixture/containers/$replacement.name" ]] || { cat "$fixture/output"; test_fail "$scenario failed to recover temporary current"; } ;;
  esac
  if [[ -f "$fixture/actions" ]]; then
    if grep -Eq '^(image (tag|save|build)|commit |volume |container rm .*--)' "$fixture/actions"; then test_fail 'forbidden rollback creation/broad removal'; fi
    if grep -Fq "container rm $original" "$fixture/actions"; then test_fail 'deleted existing retained target'; fi
  fi
  count=$((count+1))
done
printf 'Retained release Gate/source admission and %s lifecycle fault scenarios passed.\n' "$count"
