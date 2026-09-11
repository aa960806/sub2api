#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
subject="$script_dir/subnexus-full-release-cutover.sh"
controller="$script_dir/subnexus-production-cutover.sh"
ui_library="$script_dir/subnexus-ui-cutover.sh"
fixture_path="$PATH"
source <(head -n -1 "$controller")
export PATH="$fixture_path"
source "$ui_library"
source "$subject"
test_fail() { printf 'FULL RELEASE TEST ERROR: %s\n' "$*" >&2; exit 1; }
root="$(mktemp -d /tmp/subnexus-full-test.XXXXXX)"
trap 'rm -rf -- "$root"' EXIT
bash -n "$subject"
[[ "$(hash_file "$controller")" == "$full_controller_sha" ]] || test_fail 'controller pin drift'
[[ "$(hash_file "$ui_library")" == "$full_ui_sha" ]] || test_fail 'UI pin drift'
[[ "$(ui_source_path_class backend/migrations/234a_group_model_allowlist_legacy_compat.sql || true)" == '' ]] || test_fail 'original UI allowlist admits backend migration'

# The Gate parser is exercised on actual files. Only the approved filesystem
# root/ownership boundary is replaced, so the test remains non-root portable.
assert_approved_path() { [[ -f "$1" && "$1" == "$root/"* ]] || fail 'fixture gate outside root'; printf '%s|%s\n' "$1" "$root"; }
assert_root_owned_regular() { [[ -f "$1" && ! -L "$1" ]] || fail 'fixture evidence is not regular'; }
full_target="$full_required_commit"
full_tree="$(git -C "$script_dir/../.." rev-parse "$full_target^{tree}")"
full_candidate_image="sha256:$(printf '1%.0s' {1..64})"
full_previous_image="sha256:$(printf '2%.0s' {1..64})"
full_compat_gate="$root/compat.env"
backup="$(printf '3%.0s' {1..64})"
write_gate() {
  printf '%s\n' version=full-release-compat-v1 result=passed \
    "candidate_commit=$full_target" "candidate_tree=$full_tree" \
    "candidate_image=$full_candidate_image" "previous_live_image=$full_previous_image" \
    "backup_sha256=$backup" new_old_new=passed migration_contract=passed api_regression=passed cleanup=passed > "$full_compat_gate"
  full_compat_sha="$(hash_file "$full_compat_gate")"
}
expect_gate_failure() {
  local scenario="$1"
  if (full_validate_gate) > "$root/$scenario.log" 2>&1; then test_fail "Gate accepted $scenario"; fi
}
write_gate
full_validate_gate
[[ "$full_backup_sha" == "$backup" ]] || test_fail 'backup identity not captured'
for scenario in candidate_commit candidate_tree candidate_image previous_live_image new_old_new migration_contract api_regression cleanup version result backup_sha256; do
  write_gate
  python3 - "$full_compat_gate" "$scenario" <<'PY'
import sys
p, key = sys.argv[1:]
lines = open(p).read().splitlines()
open(p,'w').write('\n'.join(key+'=wrong' if l.startswith(key+'=') else l for l in lines)+'\n')
PY
  full_compat_sha="$(hash_file "$full_compat_gate")"
  expect_gate_failure "$scenario"
done
write_gate
printf 'result=passed\n' >> "$full_compat_gate"
full_compat_sha="$(hash_file "$full_compat_gate")"
expect_gate_failure duplicate
write_gate
printf 'unreviewed=passed\n' >> "$full_compat_gate"
full_compat_sha="$(hash_file "$full_compat_gate")"
expect_gate_failure unknown
write_gate
printf 'cleanup=failed\n' >> "$full_compat_gate"
expect_gate_failure changed_after_review
write_gate
full_candidate_image="$full_previous_image"
expect_gate_failure identical_images
full_candidate_image="sha256:$(printf '1%.0s' {1..64})"
write_gate

# Admission failures must precede backups and the first live stop, not merely
# result in an eventual failure after touching the deployment.
if (
  require_commands() { :; }
  init_docker() { :; }
  docker_rpc() { [[ "$1" == inspect ]] || fail 'prepare performed a Docker write'; printf '%s\n' "$full_previous_image"; }
  ui_prepare() { touch "$root/unexpected-prepare"; }
  full_prepare /unused "$full_target" "$full_tree" "${full_candidate_image#sha256:}" /unused/archive "$backup" /unused/gate live /unused/anchor "$backup" "$full_previous_image" old "$full_compat_gate" "$(printf '0%.0s' {1..64})"
) > "$root/prepare-reject.log" 2>&1; then test_fail 'prepare accepted unapproved compatibility evidence'; fi
[[ ! -e "$root/unexpected-prepare" ]] || test_fail 'prepare reached backups before validating compatibility evidence'
mkdir "$root/admission-run"
printf 'full-release-v1\n' > "$root/admission-run/FULL_READY"
printf '%s\n' state=prepared ui_state=prepared full_flow=full-release-v1 \
  "full_ui_library_sha256=$full_ui_sha" "full_controller_sha256=$full_controller_sha" \
  "ui_base_sha=$full_base_commit" "target_sha=$full_target" "full_target_tree=$full_tree" \
  "candidate_image_id=${full_candidate_image#sha256:}" "ui_new_rollback_image=$full_previous_image" \
  "live_app_image_id=$full_previous_image" "full_compat_gate=$full_compat_gate" \
  "full_compat_sha256=$(printf '0%.0s' {1..64})" "full_compat_backup_sha256=$backup" > "$root/admission-run/manifest.env"
if (
  ui_load_run() { run_dir="$root/admission-run"; manifest_file="$run_dir/manifest.env"; }
  assert_runtime_still_matches_prepare() { touch "$root/unexpected-switch"; }
  docker_rpc() { touch "$root/unexpected-docker"; fail 'invalid Gate reached Docker'; }
  SUBNEXUS_CUTOVER_CONFIRM=I_UNDERSTAND_SHORT_PRODUCTION_WINDOW
  full_switch "$root/admission-run"
) > "$root/switch-reject.log" 2>&1; then test_fail 'switch accepted changed compatibility evidence'; fi
[[ ! -e "$root/unexpected-switch" && ! -e "$root/unexpected-docker" ]] || test_fail 'switch reached live operations before validating compatibility evidence'

# Validate the pinned history, detached checkout and exact tree against real
# Git objects in an independent temporary clone, without touching the source.
git clone -q --shared --no-checkout "$script_dir/../.." "$root/source"
git -C "$root/source" -c core.autocrlf=false checkout -q --detach "$full_target"
full_assert_source "$root/source" "$full_base_commit" "$full_target"
wrong_commit="$(git -C "$root/source" rev-parse "$full_base_commit^")"
if (full_assert_source "$root/source" "$wrong_commit" "$full_target") >/dev/null 2>&1; then test_fail 'wrong live base accepted'; fi
if (full_assert_source "$root/source" "$full_base_commit" "$wrong_commit") >/dev/null 2>&1; then test_fail 'wrong target accepted'; fi
good_tree="$full_tree"
full_tree="$(printf '4%.0s' {1..40})"
if (full_assert_source "$root/source" "$full_base_commit" "$full_target") >/dev/null 2>&1; then test_fail 'wrong target tree accepted'; fi
full_tree="$good_tree"
printf 'dirty\n' > "$root/source/untracked-release-input"
if (full_assert_source "$root/source" "$full_base_commit" "$full_target") >/dev/null 2>&1; then test_fail 'dirty source accepted'; fi
rm -- "$root/source/untracked-release-input"
git -C "$root/source" switch -q -c fixture-attached
if (full_assert_source "$root/source" "$full_base_commit" "$full_target") >/dev/null 2>&1; then test_fail 'attached source accepted'; fi

# Reuse the filesystem-only fault model and all its assertions with the full
# switch orchestration. The original test and UI entry stay unchanged. Gate
# admission was tested above; here the model substitutes those two admission
# calls and exercises stop/rename/create/health/recovery/rollback failures.
python3 - "$script_dir/subnexus-ui-cutover.test.sh" "$root/full-fixture.sh" "$script_dir" "$subject" <<'PY'
import pathlib, sys
src, dest, directory, subject = sys.argv[1:]
text = pathlib.Path(src).read_text()
old_dir = 'script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"'
assert text.count(old_dir) == 1
text = text.replace(old_dir, 'script_dir='+repr(directory))
call = '  ui_switch "$run_dir"\n'
assert text.count(call) == 1
inject = '''  source "$FULL_TEST_SUBJECT"
  full_validate_manifest() { :; }
  full_assert_source() { :; }
  ui_switch() { full_switch "$@"; }
'''
text = text.replace(call, inject+call)
rename = '''        printf '%s\\n' "$3" > "$store/$id.name" ;;'''
assert text.count(rename) == 1
text = text.replace(rename, '''        printf '%s\\n' "$3" > "$store/$id.name"
        if [[ "$scenario" == full_stopped_drift && "$id" == "$current" && "$3" == "$temporary_name" ]]; then
          printf '%s\\n' "$stranger" > "$store/$current.image"
        fi ;;''')
pathlib.Path(dest).write_text(text)
PY
export FULL_TEST_SUBJECT="$subject"
bash "$root/full-fixture.sh" > "$root/full-lifecycle.log" 2>&1 || { cat "$root/full-lifecycle.log"; test_fail 'full release lifecycle regression'; }
tail -n 1 "$root/full-lifecycle.log"
mkdir "$root/stopped-drift"
if bash "$root/full-fixture.sh" --case full_stopped_drift "$root/stopped-drift" > "$root/stopped-drift/output" 2>&1; then test_fail 'stopped rollback drift accepted'; fi
if grep -Fq 'create candidate' "$root/stopped-drift/actions"; then test_fail 'candidate created before stopped rollback contract verification'; fi
[[ ! -f "$root/stopped-drift/run/SWITCHED" ]] || test_fail 'stopped rollback drift reported switch success'
printf 'Full release Gate/source admission and retained-live fault tests passed.\n'
