#!/usr/bin/env bash
set -Eeuo pipefail

# Explicit full-release entry. The historical UI-only entry and migration
# controller remain byte-for-byte unchanged. Recovery is delegated to the
# pinned retained-live UI library; only the full-release admission and switch
# orchestration below differ.
readonly full_controller_sha='19824a87e3e1de5659cb30664750b71c5c10d374f25bda7f52e6524fe477ee65'
readonly full_ui_sha='8fdfc8253020d61c1e5f0b13b49c340295e0713f552763c66603d32ae949e38d'
readonly full_base_commit='890828afe0f726abb363029f147e04087fed2bca'
readonly full_required_commit='890828afe0f726abb363029f147e04087fed2bca'
full_target=''
full_tree=''
full_candidate_image=''
full_previous_image=''
full_compat_gate=''
full_compat_sha=''
full_backup_sha=''

full_usage() {
  printf '%s\n' \
    'usage: subnexus-full-release-cutover.sh prepare CONTROLLER UI_LIBRARY SOURCE TARGET TREE IMAGE_ID ARCHIVE ARCHIVE_SHA CANDIDATE_GATE LIVE ANCHOR OLD_ID OLD_IMAGE OLD_NAME COMPAT_GATE COMPAT_SHA [PUBLIC_HEALTH_URL]' \
    '       subnexus-full-release-cutover.sh switch|rollback|recover CONTROLLER UI_LIBRARY RUN_DIRECTORY' \
    'Requires independently supplied SUBNEXUS_APPROVED_FULL_RELEASE_SCRIPT_SHA256 and usual owner/confirmation variables.' >&2
}

full_bootstrap_file() {
  local path="$1" cursor mode
  [[ "$path" == /* && -f "$path" && ! -L "$path" ]] || return 1
  [[ "$(realpath -e -P -- "$path")" == "$path" ]] || return 1
  cursor="$path"
  while :; do
    [[ "$(stat -c '%u' -- "$cursor")" == 0 ]] || return 1
    mode="$(stat -c '%a' -- "$cursor")" || return 1
    (( (8#$mode & 0022) == 0 )) || return 1
    [[ "$cursor" != / ]] || break
    cursor="$(dirname -- "$cursor")"
  done
}

full_gate_value() {
  awk -F= -v key="$1" '$1 == key {n++; value=substr($0,index($0,"=")+1)} END {if(n!=1)exit 1; print value}' "$full_compat_gate"
}

full_validate_gate() {
  local info actual
  valid_sha40 "$full_target" && valid_sha40 "$full_tree" || fail 'full release commit/tree is invalid'
  [[ "$full_candidate_image" =~ ^sha256:[0-9a-f]{64}$ && "$full_previous_image" =~ ^sha256:[0-9a-f]{64}$ ]] || fail 'full release image identities are invalid'
  [[ "$full_candidate_image" != "$full_previous_image" ]] || fail 'full release candidate equals the rollback image'
  valid_sha64 "$full_compat_sha" || fail 'full release compatibility evidence SHA is invalid'
  info="$(assert_approved_path "$full_compat_gate" candidate_gate)" || fail 'full release compatibility path is not approved'
  full_compat_gate="${info%%|*}"
  assert_root_owned_regular "$full_compat_gate" 'full release compatibility evidence'
  actual="$(hash_file "$full_compat_gate")" || fail 'cannot hash full release compatibility evidence'
  [[ "$actual" == "$full_compat_sha" ]] || fail 'full release compatibility evidence SHA mismatch'
  # Never source evidence. Reject duplicates, unknown keys and malformed
  # values rather than allowing contradictory result lines to pass grep.
  python3 - "$full_compat_gate" "$full_target" "$full_tree" "$full_candidate_image" "$full_previous_image" <<'PY'
import re, sys
path, commit, tree, candidate, previous = sys.argv[1:]
expected = {
    'version': 'full-release-compat-v1', 'result': 'passed',
    'candidate_commit': commit, 'candidate_tree': tree,
    'candidate_image': candidate, 'previous_live_image': previous,
    'new_old_new': 'passed', 'migration_contract': 'passed',
    'api_regression': 'passed', 'cleanup': 'passed',
}
raw = open(path, 'rb').read(65537)
if len(raw) > 65536 or b'\r' in raw or not raw.endswith(b'\n'):
    raise SystemExit('invalid compatibility evidence encoding/size')
values = {}
for line in raw.decode('ascii').splitlines():
    key, sep, value = line.partition('=')
    if not sep or not key or key in values:
        raise SystemExit('duplicate or malformed compatibility evidence field')
    values[key] = value
if set(values) != set(expected) | {'backup_sha256'}:
    raise SystemExit('unexpected compatibility evidence fields')
if any(values[key] != value for key, value in expected.items()):
    raise SystemExit('compatibility evidence does not match release identities/results')
if not re.fullmatch('[0-9a-f]{64}', values['backup_sha256']):
    raise SystemExit('invalid compatibility backup SHA')
PY
  [[ "$?" == 0 ]] || fail 'full release compatibility evidence failed validation'
  full_backup_sha="$(full_gate_value backup_sha256)" || fail 'compatibility backup SHA is missing'
  [[ "$(hash_file "$full_compat_gate")" == "$full_compat_sha" ]] || fail 'compatibility evidence changed during validation'
}

full_assert_source() {
  local source="$1" base="$2" target="$3" tree
  [[ "$base" == "$full_base_commit" && "$target" == "$full_target" ]] || fail 'full release source identities do not match approved release'
  valid_sha40 "$target" && valid_sha40 "$full_tree" || fail 'full release source commit/tree is invalid'
  git -C "$source" cat-file -e "$full_required_commit^{commit}" || fail 'required compatibility commit is unavailable'
  git -C "$source" merge-base --is-ancestor "$full_base_commit" "$target" || fail 'full release does not descend from the fixed live base'
  git -C "$source" merge-base --is-ancestor "$full_required_commit" "$target" || fail 'full release omits the required migration compatibility repair'
  [[ "$(git -C "$source" rev-parse HEAD)" == "$target" ]] || fail 'full release source HEAD changed'
  tree="$(git -C "$source" rev-parse "$target^{tree}")" || fail 'cannot resolve full release tree'
  [[ "$tree" == "$full_tree" ]] || fail 'full release source tree does not match compatibility evidence'
  [[ -z "$(git -C "$source" status --porcelain=v1 --untracked-files=all)" ]] || fail 'full release source is dirty'
  [[ -z "$(git -C "$source" symbolic-ref --quiet --short HEAD 2>/dev/null || true)" ]] || fail 'full release source must be detached'
  full_validate_gate
}

full_validate_manifest() {
  local scope="$1" recorded_backup
  [[ "$(manifest_value full_flow)" == full-release-v1 ]] || fail 'run does not belong to the full-release entry'
  [[ "$(manifest_value full_ui_library_sha256)" == "$full_ui_sha" && "$(manifest_value full_controller_sha256)" == "$full_controller_sha" ]] || fail 'full release library pins changed'
  assert_root_owned_regular "$run_dir/FULL_READY" 'full release readiness marker'
  [[ "$(read_one_line "$run_dir/FULL_READY")" == full-release-v1 ]] || fail 'invalid full release readiness marker'
  [[ "$(manifest_value ui_base_sha)" == "$full_base_commit" ]] || fail 'full release base changed'
  full_target="$(manifest_value target_sha)"
  full_tree="$(manifest_value full_target_tree)"
  full_candidate_image="sha256:$(manifest_value candidate_image_id)"
  full_previous_image="$(manifest_value ui_new_rollback_image)"
  [[ "$full_previous_image" == "$(manifest_value live_app_image_id)" ]] || fail 'full release rollback image is inconsistent'
  full_compat_gate="$(manifest_value full_compat_gate)"
  full_compat_sha="$(manifest_value full_compat_sha256)"
  recorded_backup="$(manifest_value full_compat_backup_sha256)"
  valid_sha40 "$full_tree" && valid_sha64 "$full_compat_sha" && valid_sha64 "$recorded_backup" || fail 'full release evidence metadata is malformed'
  if [[ "$scope" == switch ]]; then
    full_validate_gate
    [[ "$full_backup_sha" == "$recorded_backup" ]] || fail 'full release compatibility backup changed'
  fi
  # Rollback deliberately does not require release/Gate/backup artifacts:
  # the independently pinned run and previous-live identities remain primary.
}

full_prepare() {
  local source="$1" target="$2" tree="$3" image_id="$4" archive="$5" archive_sha="$6" gate="$7" live="$8" anchor="$9"
  local old_id="${10}" old_image="${11}" old_name="${12}" compat="${13}" compat_sha="${14}" public="${15:-}"
  full_target="$target"; full_tree="$tree"; full_candidate_image="sha256:${image_id#sha256:}"
  full_compat_gate="$compat"; full_compat_sha="$compat_sha"
  require_commands
  init_docker
  full_previous_image="$(docker_rpc inspect --format '{{.Image}}' "$live")" || fail 'cannot inspect full release previous-live image'
  full_validate_gate
  ui_prepare "$source" "$target" "$full_base_commit" "${image_id#sha256:}" "$archive" "$archive_sha" "$gate" "$live" "$anchor" "$old_id" "$old_image" "$old_name" "$public"
  [[ "$(manifest_value live_app_image_id)" == "$full_previous_image" ]] || fail 'previous-live image changed during full release prepare'
  manifest_set full_flow full-release-v1
  manifest_set full_target_tree "$full_tree"
  manifest_set full_ui_library_sha256 "$full_ui_sha"
  manifest_set full_controller_sha256 "$full_controller_sha"
  manifest_set full_compat_gate "$full_compat_gate"
  manifest_set full_compat_sha256 "$full_compat_sha"
  manifest_set full_compat_backup_sha256 "$full_backup_sha"
  full_validate_gate
  printf 'full-release-v1\n' > "$run_dir/FULL_READY"
  chmod 600 "$run_dir/FULL_READY"
  log "FULL_RELEASE_PREPARED_RUN=$run_dir"
}

full_switch() {
  [[ "${SUBNEXUS_CUTOVER_CONFIRM:-}" == I_UNDERSTAND_SHORT_PRODUCTION_WINDOW ]] || fail 'full release switch requires the production-window confirmation'
  ui_load_run "$1" switch
  [[ "$(manifest_value state)" == prepared && "$(manifest_value ui_state)" == prepared ]] || fail 'full release switch requires a fresh prepared run'
  full_validate_manifest switch
  [[ "${SUBNEXUS_CUTOVER_QUIET_CONFIRM:-}" == I_HAVE_CHECKED_NO_SETTLEMENT_TASKS ]] || fail 'full release switch requires a settlement/migration worker check'
  ui_expected_settings_hash="$(manifest_value ui_settings_sha256)"
  assert_runtime_still_matches_prepare
  ui_assert_new_rollback_contract prepared
  ui_assert_base_image "$full_base_commit" "$(manifest_value live_app_image_id)"
  source_root="$(manifest_value source_root)"
  full_assert_source "$source_root" "$full_base_commit" "$target_sha"
  ui_assert_anchor_if_present
  ui_assert_settings_unchanged
  [[ "$(docker_rpc image inspect --format '{{.Id}}' "sha256:$expected_image_id")" == "sha256:$expected_image_id" ]] || fail 'full release candidate image is unavailable'
  local temp="$(manifest_value ui_new_rollback_name)" observed
  observed="$(inspect_container_id_or_empty "$temp")" || fail 'cannot inspect previous-live name'
  [[ -z "$observed" ]] || fail 'previous-live name is occupied'
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
  # Required before even creating the candidate, including after an ambiguous
  # rename response or an external change to the stopped rollback object.
  ui_assert_new_rollback_contract stopped
  create_candidate_container
  assert_candidate_container_identity "$candidate_id"
  assert_candidate_runtime_contract
  assert_daemon_still_matches_prepare
  docker_rpc start "$candidate_id" >/dev/null || fail 'full release candidate did not start'
  wait_for_candidate_health || fail 'full release candidate failed health stability checks'
  validate_candidate_runtime
  ui_assert_settings_unchanged
  ui_assert_anchor_if_present
  assert_dependencies_still_match
  preserved_name="$temp"
  ui_assert_new_rollback_contract stopped
  ( ulimit -f 32768; docker_rpc logs --tail 5000 "$app_id" > "$run_dir/previous-container.log" 2>&1 ) || fail 'cannot retain previous container diagnostic output'
  chmod 600 "$run_dir/previous-container.log"
  manifest_set ui_state committing
  manifest_set ui_commit_intent yes
  ui_finish_commit
  trap - ERR HUP INT TERM
  log "FULL_RELEASE_SWITCH_COMPLETED=$run_dir"
}

full_install_overrides() {
  # These overrides exist only in this independently pinned full-release
  # process. Running the original UI script still uses its exact UI allowlist.
  ui_assert_source_delta() { full_assert_source "$@"; }
  ui_dispatch() {
    local action="$1"
    shift
    mode="$action"; [[ "$mode" != recover ]] || mode=rollback
    case "$action" in
      prepare) full_prepare "$@" ;;
      switch) require_commands; full_switch "$1" ;;
      rollback|recover)
        require_commands
        # Original loader verifies the full script SHA in the manifest. Do
        # not add source/Gate dependencies to the proven recovery path.
        if [[ "$action" == rollback ]]; then ui_manual_rollback "$1"; else ui_recover_entry "$1"; fi ;;
      *) full_usage; return 2 ;;
    esac
  }
}

full_main() {
  local action="${1:-}" controller="${2:-}" ui_library="${3:-}" entry actual copy expected="${SUBNEXUS_APPROVED_FULL_RELEASE_SCRIPT_SHA256:-}"
  case "$-" in *x*) set +x ;; esac
  unset BASH_ENV ENV CDPATH GLOBIGNORE TAR_OPTIONS GZIP
  export PATH='/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin'
  [[ "$EUID" == 0 ]] || { printf 'Full release must run as root\n' >&2; return 1; }
  [[ "$#" -ge 3 ]] || { full_usage; return 2; }
  shift 3
  case "$action" in prepare) [[ "$#" == 14 || "$#" == 15 ]] || { full_usage; return 2; } ;; switch|rollback|recover) [[ "$#" == 1 ]] || { full_usage; return 2; } ;; *) full_usage; return 2 ;; esac
  umask 077
  entry="$(realpath -e -P -- "${BASH_SOURCE[0]}")"
  full_bootstrap_file "$entry" && full_bootstrap_file "$ui_library" && full_bootstrap_file "$controller" || { printf 'Untrusted full release/library path\n' >&2; return 1; }
  [[ "$expected" =~ ^[0-9a-f]{64}$ ]] || { printf 'Missing approved full release SHA\n' >&2; return 1; }
  actual="$(sha256sum "$entry" | awk '{print $1}')"
  [[ "$actual" == "$expected" ]] || { printf 'Full release SHA mismatch\n' >&2; return 1; }
  [[ "$(sha256sum "$controller" | awk '{print $1}')" == "$full_controller_sha" ]] || { printf 'Controller pin mismatch\n' >&2; return 1; }
  copy="$(mktemp /tmp/subnexus-full-ui.XXXXXX)" || return 1
  chmod 600 "$copy"
  if ! cp -- "$ui_library" "$copy" || [[ "$(sha256sum "$copy" | awk '{print $1}')" != "$full_ui_sha" ]]; then
    rm -f -- "$copy"; printf 'UI library pin mismatch\n' >&2; return 1
  fi
  source "$copy"
  rm -f -- "$copy"
  [[ "$ui_controller_sha" == "$full_controller_sha" ]] || { printf 'UI/controller pin disagreement\n' >&2; return 1; }
  ui_entry_path="$entry"
  SUBNEXUS_APPROVED_UI_CUTOVER_SCRIPT_SHA256="$expected"
  full_install_overrides
  ui_load_controller "$controller" "$action" "$@"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then full_main "$@"; fi
