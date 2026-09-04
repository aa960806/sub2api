#!/usr/bin/env bash
set -Eeuo pipefail

# Static and helper tests for the local-only image builder.  These tests never
# invoke Docker, never contact a registry, and never access the production
# host.  The real build is intentionally operator-run only after a named local
# rootless/WSL daemon has been inspected.

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
subject="$script_dir/subnexus-isolated-image-build.sh"
root_dockerfile="$script_dir/../../Dockerfile"

fail() {
  printf 'TEST ERROR: %s\n' "$*" >&2
  exit 1
}

assert_contains() {
  local needle="$1"
  grep -Fq -- "$needle" "$subject" || fail "missing invariant: $needle"
}

assert_not_contains() {
  local needle="$1"
  if grep -Fq -- "$needle" "$subject"; then
    fail "forbidden pattern present: $needle"
  fi
}

bash -n "$subject"
[[ -f "$subject" ]] || fail 'image builder script is missing'

# Keep the script self-contained and hard to accidentally turn into a deploy
# helper.
assert_contains '#!/usr/bin/env bash'
assert_contains 'set -Eeuo pipefail'
assert_contains 'case "$-" in'
assert_contains 'set +x'
assert_contains 'umask 077'
assert_contains 'unset BASH_ENV ENV CDPATH GLOBIGNORE'
assert_contains 'assert_script_unchanged() {'
assert_contains 'script_sha256'
assert_contains "export PATH='/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin'"
assert_contains 'SUBNEXUS_BUILD_DOCKER_CONTEXT'
assert_contains 'SUBNEXUS_LOCAL_DOCKER_CONFIRM=I_UNDERSTAND_LOCAL_ONLY'
assert_contains 'SUBNEXUS_APPROVED_BUILD_SCRIPT_SHA256'
assert_contains 'script_relative_path='
assert_contains 'validate_approved_build_script_sha256() {'
assert_contains 'approved_script_blob_sha256'
assert_contains 'git -C "$source_root" show "$approved_sha:$script_relative_path"'
assert_contains 'executed build script does not match the approved SHA256'
assert_contains 'default, SSH, TCP, remote,'
assert_contains 'production contexts are rejected'
assert_contains 'valid_context_name() {'
assert_contains 'validate_endpoint() {'
assert_contains 'DOCKER_HOST DOCKER_CONTEXT DOCKER_CONFIG DOCKER_TLS_VERIFY DOCKER_CERT_PATH DOCKER_API_VERSION'
assert_contains 'SUBNEXUS_ALLOW_LOCAL_SYSTEM_SOCKET'
assert_contains 'SUBNEXUS_ALLOW_LOCAL_NPIPE'
assert_contains '--context "$docker_context"'
assert_contains 'docker_context'
assert_contains 'unix:///*'
assert_contains 'npipe:///*'
assert_not_contains 'docker pull'
assert_not_contains 'docker push'
assert_not_contains 'ssh '
assert_not_contains 'scp '
assert_not_contains '51.81.211.97'
assert_not_contains 'docker system prune'
assert_not_contains 'docker container prune'
assert_not_contains 'docker network prune'
assert_not_contains 'docker volume prune'

# Fixed source and context provenance.
assert_contains 'symbolic-ref --quiet --short HEAD'
assert_contains 'status --porcelain=v1 --untracked-files=all'
assert_contains 'submodule status --recursive'
assert_contains "submodules=\"\$(git -C \"\$source_root\" submodule status --recursive 2>/dev/null)\" ||"
assert_contains "fail 'cannot inspect Git submodules'"
assert_not_contains 'submodule status --recursive 2>/dev/null) || true'
assert_not_contains "*'not found'*) ;;"
assert_contains 'rev-parse "$approved_sha^{tree}"'
assert_contains 'archive --format=tar "$approved_sha"'
assert_contains 'target.chmod((member.mode & 0o777) & ~0o022)'
assert_contains 'never carry group/other write permission'
assert_contains 'symlink or Git submodule is not allowed'
assert_contains 'sensitive tracked file is not allowed'
assert_contains 'is_safe_env_example_path() {'
assert_contains '.env.example|*/.env.example|.env.sample|*/.env.sample'
assert_contains 'is_database_dump_path() {'
assert_contains '"$relative" == "backend/migrations/$basename"'
assert_contains '^[0-9]{3,}[a-z]?_'
assert_contains 'validate_dockerfile_pin_contract() {'
assert_contains 'local -A seen=()'
assert_contains 'local -A seen=() arg_seen=()'
assert_contains 'opcode="${fields[0],,}"'
assert_contains 'from_seen='
assert_contains 'Dockerfile base ARG must appear before the first FROM'
assert_contains 'Dockerfile base ARG must declare a default assignment'
assert_contains 'Dockerfile ARG has an unsafe default value'
assert_contains 'Dockerfile has an unterminated line continuation'
assert_contains 'Dockerfile must contain exactly four FROM instructions'
assert_contains 'external Dockerfile syntax directives are not allowed'
assert_contains 'directory_fingerprint() {'
assert_contains "stat -Lc '%d|%i|%F|%u|%g|%a'"
assert_contains 'artifact_root_fingerprint='
assert_contains 'artifact_root_fd_fingerprint='
assert_contains 'exec 9<"$artifact_root"'
assert_contains '"/proc/${BASHPID:-$$}/fd/9"'
assert_contains "artifact directory changed before locking"
assert_not_contains '.build.lock'
assert_contains 'safe_remove_context "$context_root"'
assert_contains 'expected_images='
assert_contains 'docker_call image ls -aq --no-trunc'
assert_contains 'assert_baseline_objects_unchanged() {'
assert_contains 'local Docker object baseline was not restored after failed build cleanup'
[[ -f "$root_dockerfile" ]] || fail 'root Dockerfile is missing'
if grep -Eiq '^[[:space:]]*#[[:space:]]*syntax[[:space:]]*=' "$root_dockerfile"; then
  fail 'root Dockerfile must use the digest-pinned BuildKit bundled frontend'
fi
assert_contains 'ARG NODE_IMAGE='
assert_contains 'ARG GOLANG_IMAGE='
assert_contains 'ARG ALPINE_IMAGE='
assert_contains 'ARG POSTGRES_IMAGE='

# Immutable base image contract and no implicit pulls.
for name in SUBNEXUS_CANDIDATE_NODE_IMAGE SUBNEXUS_CANDIDATE_GOLANG_IMAGE \
  SUBNEXUS_CANDIDATE_ALPINE_IMAGE SUBNEXUS_CANDIDATE_POSTGRES_IMAGE \
  SUBNEXUS_CANDIDATE_BUILDKIT_IMAGE; do
  assert_contains "$name"
done
assert_contains 'valid_immutable_image_ref() {'
assert_contains 'valid_repository_digest_ref() {'
assert_contains 'RepoDigests'
assert_contains 'requested_repo'
assert_contains 'requested repository digest'
assert_contains '--pull=false'
assert_contains '--platform linux/amd64'
assert_contains 'validate_base_image() {'
assert_contains 'image inspect'
assert_contains 'docker_call image ls --no-trunc --filter "reference=$name"'
assert_contains 'inspect_status'
assert_contains "*'no such image'*"
assert_contains 'image ls --no-trunc --filter "reference=$name"'

# BuildKit is disposable and bounded; process-group cancellation must not
# leave a build subprocess running after a timeout or signal.
assert_contains 'buildx create --name "$builder_name" --driver docker-container'
assert_contains '--driver-opt "image=$buildkit_image_ref"'
assert_contains '--driver-opt "network=$builder_network_name"'
assert_contains "--driver-opt 'memory=4g'"
assert_contains "--driver-opt 'memory-swap=4g'"
assert_contains "--driver-opt 'cpu-quota=200000'"
assert_contains "--driver-opt 'cpu-period=100000'"
assert_contains "--driver-opt 'restart-policy=no'"
assert_not_contains "--driver-opt 'pids-limit=512'"
assert_contains 'docker_call update --memory 4g --memory-swap 4g'
assert_contains '--cpu-period 100000 --cpu-quota 200000 --pids-limit 512 --restart no "$builder_id"'
assert_contains 'cannot apply the BuildKit container resource limits'
assert_contains 'docker_info_warnings="$(docker_call info --format'
assert_contains 'swap_limit_supported_from_warnings'
assert_contains 'DOCKER_SWAP_LIMIT_SUPPORTED'
assert_contains "builder_validated='true'"
assert_contains 'validate_builder_container "$builder_id" prebuild-cleanup'
assert_contains 'validate_builder_mounts() {'
assert_contains 'validate_builder_mounts "$mounts" "buildx_buildkit_${builder_name}0_state" \'
assert_contains '"${docker_root_dir%/}/volumes/buildx_buildkit_${builder_name}0_state/_data"'
assert_contains 'printf "%s|%s|%s|%t|%s\n" .Type .Name .Destination .RW .Source'
assert_contains 'validate_builder_cleanup_network() {'
assert_contains 'builder_validation_failed=0'
assert_contains 'validate_builder_container "$builder_id" || builder_validation_failed=1'
assert_contains '"$builder_create_attempted" != '\''true'\'' || "$builder_cleanup_done" == '\''true'\'''
assert_contains "\"\$pids_limit\" == '<nil>' || \"\$pids_limit\" == '0' || \"\$pids_limit\" == '512'"
assert_contains "\"\$memory_swap\" == '-1' || \"\$memory_swap\" == '4294967296'"
assert_contains "^Driver:[[:space:]]+docker-container[[:space:]]*\$"
assert_contains "^Status:[[:space:]]+running[[:space:]]*\$"
assert_contains 'setsid --wait'
assert_contains 'build_pid_file'
assert_contains 'subnexus-build-wrapper'
assert_contains 'printf "%s\n" "$$" >"$pid_file"'
assert_contains 'recorded_pgid='
assert_contains 'kill -0 -- "-$recorded_pgid"'
assert_contains 'kill -0 -- "-$build_pgid"'
assert_contains 'kill -TERM -- "-$build_pgid"'
assert_contains 'kill -KILL -- "-$build_pgid"'
assert_contains 'wait "$build_pid"'
assert_contains 'build_status="$status"'
assert_contains 'docker.sock'
assert_contains 'network options or labels are invalid'
assert_contains '[[ "$privileged" == '\''true'\'' || "$privileged" == '\''false'\'' ]]'
assert_contains 'security_opt'
assert_contains '["label=disable"]'
assert_contains 'builder_cleanup_done'
assert_contains 'builder_network_cleanup_done'
assert_contains 'builder_create_attempted'
assert_contains 'builder_network_create_attempted'

# Provenance labels and candidate-gate-compatible archive output.
assert_contains 'com.subnexus.release.gate=$gate_label'
assert_contains 'com.subnexus.candidate.gate=$gate_label'
assert_contains 'com.subnexus.candidate.token=$run_token'
assert_contains 'com.subnexus.candidate.commit=$approved_sha'
assert_contains 'com.subnexus.candidate.tree=$tree_sha'
assert_contains 'org.opencontainers.image.revision=$approved_sha'
assert_contains 'APPROVED_BUILD_SCRIPT_SHA256=%s\n'
assert_contains 'APPROVED_BUILD_SCRIPT_BLOB_SHA256=%s\n'
assert_contains 'CANDIDATE_GATE_TAG'
approval_call_line="$(awk '/^  validate_approved_build_script_sha256$/{print NR; exit}' "$subject")"
source_tree_call_line="$(awk '/^  validate_source_tree$/{print NR; exit}' "$subject")"
first_docker_call_line="$(awk '/context_info=.*docker_call context inspect/{print NR; exit}' "$subject")"
[[ "$source_tree_call_line" =~ ^[0-9]+$ && "$approval_call_line" =~ ^[0-9]+$ &&
  "$first_docker_call_line" =~ ^[0-9]+$ && "$source_tree_call_line" -lt "$approval_call_line" &&
  "$approval_call_line" -lt "$first_docker_call_line" ]] ||
  fail 'source and external script approval checks must precede Docker operations'
assert_contains 'docker save'
assert_contains 'candidate-image.tar'
assert_contains 'IMAGE_ARCHIVE_SHA256'
assert_contains 'SHA256SUMS'
assert_contains 'metadata.env'
assert_contains 'manifest.json'
assert_contains 'RepoTags'
assert_contains "image inspect --format '{{json .RootFS.Layers}}'"
assert_contains 'rootfs_layers_json'
assert_contains 'hashlib.sha256(config_bytes).hexdigest()'
assert_contains 'config digest does not match its content'
assert_contains 'rootfs.diff_ids do not match the built image'
assert_contains 'config_path_re = re.compile'
assert_contains 'atomic stage rename'
assert_contains 'mv -- "$stage_dir" "$final_dir"'

# Exercise pure Bash validators without sourcing the script's main entrypoint.
validator_source="$(sed -n '/^valid_immutable_image_ref() {$/,/^}$/p' "$subject")"
repository_validator_source="$(sed -n '/^valid_repository_digest_ref() {$/,/^}$/p' "$subject")"
approved_script_source="$(sed -n '/^validate_approved_build_script_sha256() {$/,/^}$/p' "$subject")"
context_source="$(sed -n '/^valid_context_name() {$/,/^}$/p' "$subject")"
tag_source="$(sed -n '/^valid_tag() {$/,/^}$/p' "$subject")"
env_example_source="$(sed -n '/^is_safe_env_example_path() {$/,/^}$/p' "$subject")"
dump_path_source="$(sed -n '/^is_database_dump_path() {$/,/^}$/p' "$subject")"
dockerfile_contract_source="$(sed -n '/^validate_dockerfile_pin_contract() {$/,/^}$/p' "$subject")"
remove_context_source="$(sed -n '/^safe_remove_context() {$/,/^}$/p' "$subject")"
directory_fingerprint_source="$(sed -n '/^directory_fingerprint() {$/,/^}$/p' "$subject")"
baseline_objects_source="$(sed -n '/^assert_baseline_objects_unchanged() {$/,/^}$/p' "$subject")"
absence_source="$(sed -n '/^assert_exact_absent() {$/,/^}$/p' "$subject")"
swap_support_source="$(sed -n '/^swap_limit_supported_from_warnings() {$/,/^}$/p' "$subject")"
mount_validator_source="$(sed -n '/^validate_builder_mounts() {$/,/^}$/p' "$subject")"
cleanup_network_source="$(sed -n '/^validate_builder_cleanup_network() {$/,/^cleanup_builder_and_network() {$/p' "$subject" | sed '$d')"
cleanup_source="$(sed -n '/^cleanup_builder_and_network() {$/,/^cleanup_release_tags() {$/p' "$subject" | sed '$d')"
[[ -n "$validator_source" && -n "$repository_validator_source" && -n "$approved_script_source" && -n "$context_source" && -n "$tag_source" && -n "$env_example_source" && -n "$dump_path_source" && -n "$dockerfile_contract_source" && -n "$remove_context_source" && -n "$directory_fingerprint_source" && -n "$baseline_objects_source" && -n "$absence_source" && -n "$swap_support_source" && -n "$mount_validator_source" && -n "$cleanup_network_source" && -n "$cleanup_source" ]] || fail 'validator functions not found'

# The artifact lock must bind the directory that was validated, not merely the
# pathname that happened to be opened.  On Linux/WSL, compare the directory's
# stat tuple with the same tuple through the process FD symlink.  Git Bash may
# not expose a procfs FD view, so retain the static contract above and skip only
# the runtime half when procfs is unavailable.
(
  eval "$directory_fingerprint_source"
  fixture_root="$(mktemp -d "${TMPDIR:-/tmp}/subnexus-directory-lock-test.XXXXXX")"
  trap 'rm -rf -- "$fixture_root"' EXIT
  before="$(directory_fingerprint "$fixture_root")" || fail 'directory fingerprint fixture could not inspect root'
  if [[ -e "/proc/${BASHPID:-$$}/fd/0" ]]; then
    exec 9<"$fixture_root" || fail 'directory fingerprint fixture could not open root'
    through_fd="$(directory_fingerprint "/proc/${BASHPID:-$$}/fd/9")" ||
      fail 'directory fingerprint fixture could not inspect opened FD'
    [[ "$through_fd" == "$before" ]] || fail 'opened FD fingerprint differs from path fingerprint'
  fi
)

# The external approval must bind the exact Git blob and the file that Bash is
# executing.  These checks run without Docker and cover missing, malformed,
# blob-mismatch, and executable-file-mismatch approvals.
(
  eval "$approved_script_source"
  valid_sha256_hex() { [[ "${1:-}" =~ ^[0-9a-f]{64}$ ]]; }
  fail() { return 1; }
  source_root='fixture-source'
  approved_sha="$(printf 'c%.0s' {1..40})"
  script_relative_path='tools/production-deploy/subnexus-isolated-image-build.sh'
  script_fixture_root="$(mktemp -d "${TMPDIR:-/tmp}/subnexus-script-approval-test.XXXXXX")"
  trap 'rm -rf -- "$script_fixture_root"' EXIT
  script_source_path="$script_fixture_root/subnexus-isolated-image-build.sh"
  printf '%s\n' fixture >"$script_source_path"
  owner_is_allowed() { return 0; }
  mode_is_safe() { return 0; }
  blob_content='approved-build-script-blob'
  blob_sha256="$(printf '%s' "$blob_content" | sha256sum | awk '{print tolower($1)}')"
  current_file_sha256="$blob_sha256"
  script_sha256="$(printf 'f%.0s' {1..64})"
  hash_file() { printf '%s' "$current_file_sha256"; }
  git() {
    [[ "$1" == '-C' && "$2" == "$source_root" && "$3" == 'show' &&
      "$4" == "$approved_sha:$script_relative_path" ]] || return 1
    printf '%s' "$blob_content"
  }
  SUBNEXUS_APPROVED_BUILD_SCRIPT_SHA256="$blob_sha256"
  validate_approved_build_script_sha256 || fail 'matching external script approval was rejected'
  [[ "$script_sha256" == "$blob_sha256" ]] || fail 'current script SHA was not refreshed after source validation'
  [[ "$approved_script_blob_sha256" == "$blob_sha256" ]] || fail 'approved blob digest was not retained'
  unset SUBNEXUS_APPROVED_BUILD_SCRIPT_SHA256
  if validate_approved_build_script_sha256; then fail 'missing external script approval was accepted'; fi
  SUBNEXUS_APPROVED_BUILD_SCRIPT_SHA256="${blob_sha256^^}"
  if validate_approved_build_script_sha256; then fail 'uppercase external script approval was accepted'; fi
  SUBNEXUS_APPROVED_BUILD_SCRIPT_SHA256="$(printf 'd%.0s' {1..64})"
  if validate_approved_build_script_sha256; then fail 'blob-mismatched external script approval was accepted'; fi
  SUBNEXUS_APPROVED_BUILD_SCRIPT_SHA256="$blob_sha256"
  current_file_sha256="$(printf 'e%.0s' {1..64})"
  if validate_approved_build_script_sha256; then fail 'executable-file-mismatched approval was accepted'; fi
)
(
  eval "$validator_source"
  eval "$repository_validator_source"
  digest="$(printf 'a%.0s' {1..64})"
  valid_immutable_image_ref "node@sha256:$digest"
  valid_immutable_image_ref "registry.example/node:24@sha256:$digest"
  valid_immutable_image_ref "registry.example:5000/node@sha256:$digest"
  valid_immutable_image_ref "registry.example:5000/node:24@sha256:$digest"
  valid_immutable_image_ref "sha256:$digest"
  if valid_immutable_image_ref 'node:24-alpine'; then fail 'mutable tag accepted'; fi
  if valid_immutable_image_ref "node@sha256:${digest:0:63}"; then fail 'short digest accepted'; fi
  if valid_immutable_image_ref "node@sha256:${digest^^}"; then fail 'uppercase digest accepted'; fi
  if valid_immutable_image_ref "registry.example:abc/node@sha256:$digest"; then fail 'non-numeric registry port accepted'; fi
  if valid_immutable_image_ref "registry.example:123456/node@sha256:$digest"; then fail 'overlong registry port accepted'; fi
  if valid_immutable_image_ref "registry.example:/node@sha256:$digest"; then fail 'empty registry port accepted'; fi
  valid_repository_digest_ref "node@sha256:$digest"
  valid_repository_digest_ref "registry.example:5000/node@sha256:$digest"
  valid_repository_digest_ref "registry.example:5000/node:24@sha256:$digest"
  if valid_repository_digest_ref "registry.example:abc/node@sha256:$digest"; then fail 'non-numeric repository registry port accepted'; fi
  if valid_repository_digest_ref "registry.example:123456/node@sha256:$digest"; then fail 'overlong repository registry port accepted'; fi
  eval "$context_source"
  valid_context_name subnexus-local
  valid_context_name rootless-2026
  if valid_context_name default; then fail 'default context accepted'; fi
  if valid_context_name production; then fail 'production context accepted'; fi
  if valid_context_name 'ssh-prod'; then fail 'SSH-like context accepted'; fi
  eval "$tag_source"
  valid_tag "$(printf 'a%.0s' {1..128})"
  if valid_tag "$(printf 'a%.0s' {1..129})"; then fail 'overlong Docker tag accepted'; fi
  if valid_tag 'bad/tag'; then fail 'tag with slash accepted'; fi
  eval "$env_example_source"
  is_safe_env_example_path '.env.example'
  is_safe_env_example_path 'deploy/.env.example'
  is_safe_env_example_path 'nested/path/.env.sample'
  if is_safe_env_example_path '.env.production'; then fail 'non-example env file accepted'; fi
  if is_safe_env_example_path 'deploy/.env.local'; then fail 'nested non-example env file accepted'; fi
  eval "$dump_path_source"
  if is_database_dump_path 'backend/migrations/117_add_payment_order_provider_snapshot.sql'; then fail 'migration snapshot was treated as a dump'; fi
  if is_database_dump_path 'backend/migrations/006b_guard_users_snapshot.sql'; then fail 'letter-suffixed migration snapshot was treated as a dump'; fi
  is_database_dump_path 'database/production-backup.sql'
  is_database_dump_path 'database/export.sql.gz'
  is_database_dump_path 'database/full.dump'
  is_database_dump_path 'backend/migrations/999_production-backup.sql'
  is_database_dump_path 'backend/migrations/999_archive_snapshot.sql.gz'
  is_database_dump_path 'backend/migrations/123/subdir_snapshot.sql'
  is_database_dump_path 'backend/migrations/123evil_archive_snapshot.sql'
  if is_database_dump_path 'backend/migrations/archive.sql'; then fail 'ordinary migration SQL was treated as a dump'; fi
)

# A daemon warning is the only condition that permits Docker's -1
# MemorySwap representation in strict builder validation.
(
  eval "$swap_support_source"
  if swap_limit_supported_from_warnings '["WARNING: No swap limit support"]'; then
    fail 'no-swap warning was treated as swap-limit support'
  fi
  swap_limit_supported_from_warnings '[]' || fail 'empty warning list was treated as no swap support'
  swap_limit_supported_from_warnings '["WARNING: unrelated"]' || fail 'unrelated warning list was treated as no swap support'
)

# BuildKit mount serialization is order-independent. Docker 29 on WSL may
# include exactly one read-only /usr/lib/wsl bind alongside the state volume.
(
  eval "$mount_validator_source"
  docker_root_dir='/var/lib/docker'
  state_volume='buildx_buildkit_fixture0_state'
  state_source="${docker_root_dir%/}/volumes/$state_volume/_data"
  state_mount="volume|$state_volume|/var/lib/buildkit|true|$state_source"
  wsl_mount='bind||/usr/lib/wsl|false|/usr/lib/wsl'
  state_then_wsl="${state_mount}"$'\n'"${wsl_mount}"
  wsl_then_state="${wsl_mount}"$'\n'"${state_mount}"
  validate_builder_mounts "$state_mount" "$state_volume" "$state_source"
  validate_builder_mounts "$state_then_wsl" "$state_volume" "$state_source"
  validate_builder_mounts "$wsl_then_state" "$state_volume" "$state_source"
  if validate_builder_mounts "$wsl_mount" "$state_volume" "$state_source"; then fail 'missing BuildKit state volume was accepted'; fi
  if validate_builder_mounts "${state_mount}"$'\n'"bind||/tmp|false|/tmp" "$state_volume" "$state_source"; then fail 'unexpected bind mount was accepted'; fi
  if validate_builder_mounts "${state_mount}"$'\n'"bind||/usr/lib/wsl|true|/usr/lib/wsl" "$state_volume" "$state_source"; then fail 'writable WSL bind mount was accepted'; fi
  if validate_builder_mounts "${state_mount}"$'\n'"bind||/host|false|/usr/lib/wsl" "$state_volume" "$state_source"; then fail 'WSL destination/source mismatch was accepted'; fi
  if validate_builder_mounts "${state_mount}"$'\n'"${state_mount}" "$state_volume" "$state_source"; then fail 'duplicate state volume was accepted'; fi
  if validate_builder_mounts "${state_mount}"$'\n'"${wsl_mount}"$'\n'"${wsl_mount}" "$state_volume" "$state_source"; then fail 'duplicate WSL bind mount was accepted'; fi
  if validate_builder_mounts 'bind||/var/run/docker.sock|false|/var/run/docker.sock' "$state_volume" "$state_source"; then fail 'Docker socket mount was accepted'; fi
  wrong_source_mount="volume|$state_volume|/var/lib/buildkit|true|/tmp/$state_volume/_data"
  if validate_builder_mounts "$wrong_source_mount" "$state_volume" "$state_source"; then fail 'unexpected state volume source was accepted'; fi
)

# A strict resource validation failure must still remove only the exact,
# token-labelled builder and its empty network, while preserving a failure
# status for the caller.
(
  eval "$cleanup_network_source"
  eval "$cleanup_source"
  builder_id="$(printf 'a%.0s' {1..64})"
  buildkit_image_id="$(printf 'b%.0s' {1..64})"
  builder_network_id="$(printf 'c%.0s' {1..64})"
  builder_name='subnexus-build-fixture'
  builder_network_name='subnexus-build-net-fixture'
  run_token='fixture-token'
  script_name='subnexus-isolated-image-build-v1'
  builder_create_attempted='true'
  builder_created='true'
  builder_validated='true'
  builder_cleanup_done='false'
  builder_network_create_attempted='true'
  builder_network_cleanup_done='false'
  cleanup_failed='false'
  removed_builder='false'
  removed_network='false'
  validate_builder_container() { return 1; }
  docker_call() {
    local command="$1"
    shift
    case "$command" in
      inspect)
        case "${2:-}" in
          *'.Name'*) printf '/buildx_buildkit_%s0' "$builder_name" ;;
          *'.Image'*) printf 'sha256:%s' "$buildkit_image_id" ;;
          *'NetworkMode'*) printf '%s' "$builder_network_name" ;;
          *) return 1 ;;
        esac
        ;;
      ps)
        [[ "$removed_builder" == 'true' ]] || printf '%s\n' "$builder_id"
        ;;
      buildx)
        case "${1:-}" in
          ls)
            [[ "$removed_builder" == 'true' ]] || printf '%s * docker-container\n' "$builder_name"
            ;;
          rm) removed_builder='true' ;;
          *) return 1 ;;
        esac
        ;;
      network)
        case "${1:-}" in
          inspect)
            if [[ "${2:-}" == '--format' ]]; then
              case "${3:-}" in
                *'isolated-build.gate'*) printf '%s' "$script_name" ;;
                *'isolated-build.token'*) printf '%s' "$run_token" ;;
                *'.Name'*) printf '%s' "$builder_network_name" ;;
                *'.Containers'*) [[ "$removed_builder" == 'true' ]] || printf '%s\n' "$builder_id" ;;
                *) return 1 ;;
              esac
            else
              [[ "$removed_network" == 'true' ]] && return 1
            fi
            ;;
          rm) removed_network='true' ;;
          *) return 1 ;;
        esac
        ;;
      *) return 1 ;;
    esac
  }
  if cleanup_builder_and_network; then fail 'cleanup hid the strict validation failure'; fi
  [[ "$builder_cleanup_done" == 'true' ]] || fail 'builder was not marked cleaned after validation failure'
  [[ "$builder_network_cleanup_done" == 'true' ]] || fail 'network was not marked cleaned after validation failure'
  [[ "$removed_builder" == 'true' ]] || fail 'exact builder was not removed after validation failure'
  [[ "$removed_network" == 'true' ]] || fail 'exact network was not removed after validation failure'
)

# Context cleanup is restricted to the one fixed context below staging.
(
  fixture_root="$(mktemp -d "${TMPDIR:-/tmp}/subnexus-context-cleanup-test.XXXXXX")"
  fixture_root="$(realpath -e -- "$fixture_root")"
  trap 'rm -rf -- "$fixture_root"' EXIT
  stage_dir="$fixture_root/stage"
  context_root="$stage_dir/context"
  outside="$fixture_root/outside"
  mkdir -p -- "$context_root" "$outside"
  owner_is_allowed() { return 0; }
  mode_is_safe() { return 0; }
  eval "$remove_context_source"
  safe_remove_context "$context_root" || fail 'fixed context cleanup was rejected'
  [[ ! -e "$context_root" ]] || fail 'fixed context cleanup left the directory behind'
  if safe_remove_context "$outside"; then fail 'context cleanup accepted a path outside staging'; fi
  [[ -d "$outside" ]] || fail 'context cleanup removed a path outside staging'
)

# Failed builds must restore every Docker object list, including base images.
(
  eval "$baseline_objects_source"
  object_lists_captured='true'
  baseline_containers=''
  baseline_networks=$'bridge\nhost\nnone'
  baseline_volumes=''
  baseline_images=$'sha256:base-a\nsha256:base-b'
  current_images="$baseline_images"
  docker_call() {
    case "$*" in
      'ps -aq --no-trunc') return 0 ;;
      'network ls --format {{.Name}}') printf '%s\n' "$baseline_networks" ;;
      'volume ls -q') return 0 ;;
      'image ls -aq --no-trunc') printf '%s\n' "$current_images" ;;
      *) return 1 ;;
    esac
  }
  assert_baseline_objects_unchanged || fail 'unchanged Docker object baseline was rejected'
  current_images='sha256:base-a'
  if assert_baseline_objects_unchanged; then fail 'missing base image was not detected after cleanup'; fi
  current_images=$'sha256:base-a\nsha256:base-b\nsha256:unexpected'
  if assert_baseline_objects_unchanged; then fail 'unexpected image was not detected after cleanup'; fi
)

# Exercise the Dockerfile parser with case/indent variants and known bypasses.
(
  fixture_root="$(mktemp -d "${TMPDIR:-/tmp}/subnexus-dockerfile-contract-test.XXXXXX")"
  trap 'rm -rf -- "$fixture_root"' EXIT
  context_root="$fixture_root"
  eval "$dockerfile_contract_source"
  fail() { return 1; }
  write_valid_dockerfile() {
    cat >"$context_root/Dockerfile" <<'DOCKERFILE'
ARG NODE_IMAGE=
ARG GOLANG_IMAGE=
ARG ALPINE_IMAGE=
ARG POSTGRES_IMAGE=
ARG GOPROXY=https://goproxy.cn,direct
  from --platform=${BUILDPLATFORM} ${NODE_IMAGE} AS frontend-builder
FROM --platform=${BUILDPLATFORM} ${GOLANG_IMAGE} AS backend-builder
FROM ${POSTGRES_IMAGE} AS pg-client
FROM ${ALPINE_IMAGE}
DOCKERFILE
  }
  write_valid_dockerfile
  validate_dockerfile_pin_contract || { printf 'TEST ERROR: valid Dockerfile contract was rejected\n' >&2; exit 1; }
  cat >"$context_root/Dockerfile" <<'DOCKERFILE'
  aRg NODE_IMAGE=
arg GOLANG_IMAGE=
    ARG ALPINE_IMAGE=
ArG POSTGRES_IMAGE=
ARG GOPROXY=https://goproxy.cn,direct
  from --platform=${BUILDPLATFORM} ${NODE_IMAGE} AS frontend-builder
FROM --platform=${BUILDPLATFORM} ${GOLANG_IMAGE} AS backend-builder
FROM ${POSTGRES_IMAGE} AS pg-client
FROM ${ALPINE_IMAGE}
DOCKERFILE
  validate_dockerfile_pin_contract || { printf 'TEST ERROR: case/indent ARG or FROM was rejected\n' >&2; exit 1; }
  write_valid_dockerfile
  sed -i '1s/^/# /' "$context_root/Dockerfile"
  if validate_dockerfile_pin_contract >/dev/null 2>&1; then printf 'TEST ERROR: commented ARG was accepted\n' >&2; exit 1; fi
  write_valid_dockerfile
  sed -i '1i # FROM ubuntu' "$context_root/Dockerfile"
  validate_dockerfile_pin_contract || { printf 'TEST ERROR: FROM in a comment was counted\n' >&2; exit 1; }
  write_valid_dockerfile
  printf '\nARG NODE_IMAGE=\n' >>"$context_root/Dockerfile"
  if validate_dockerfile_pin_contract >/dev/null 2>&1; then printf 'TEST ERROR: base ARG after the first FROM was accepted\n' >&2; exit 1; fi
  write_valid_dockerfile
  sed -i '2i ARG NODE_IMAGE=' "$context_root/Dockerfile"
  if validate_dockerfile_pin_contract >/dev/null 2>&1; then printf 'TEST ERROR: duplicate base ARG was accepted\n' >&2; exit 1; fi
  write_valid_dockerfile
  sed -i '1c ARG NODE_IMAGE' "$context_root/Dockerfile"
  if validate_dockerfile_pin_contract >/dev/null 2>&1; then printf 'TEST ERROR: base ARG without a default assignment was accepted\n' >&2; exit 1; fi
  write_valid_dockerfile
  sed -i '1c ARG NODE_IMAGE=not valid' "$context_root/Dockerfile"
  if validate_dockerfile_pin_contract >/dev/null 2>&1; then printf 'TEST ERROR: base ARG with an extra default token was accepted\n' >&2; exit 1; fi
  write_valid_dockerfile
  sed -i '1c ARG NODE_IMAGE=$(untrusted)' "$context_root/Dockerfile"
  if validate_dockerfile_pin_contract >/dev/null 2>&1; then printf 'TEST ERROR: base ARG with a substitution default was accepted\n' >&2; exit 1; fi
  sed -i 's|  from --platform=${BUILDPLATFORM} ${NODE_IMAGE}|  from ubuntu|' "$context_root/Dockerfile"
  if validate_dockerfile_pin_contract >/dev/null 2>&1; then printf 'TEST ERROR: lowercase mutable FROM was accepted\n' >&2; exit 1; fi
  write_valid_dockerfile
  sed -i 's|${NODE_IMAGE} AS|${NODE_IMAGE}-evil AS|' "$context_root/Dockerfile"
  if validate_dockerfile_pin_contract >/dev/null 2>&1; then printf 'TEST ERROR: allowed FROM substring was accepted\n' >&2; exit 1; fi
  write_valid_dockerfile
  sed -i '1i # SyNtAx = docker/dockerfile:1.7' "$context_root/Dockerfile"
  if validate_dockerfile_pin_contract >/dev/null 2>&1; then printf 'TEST ERROR: external syntax directive was accepted\n' >&2; exit 1; fi
  write_valid_dockerfile
  sed -i 's|${POSTGRES_IMAGE} AS pg-client|${NODE_IMAGE} AS pg-client|' "$context_root/Dockerfile"
  if validate_dockerfile_pin_contract >/dev/null 2>&1; then printf 'TEST ERROR: duplicate base argument was accepted\n' >&2; exit 1; fi
)

# Run the same strict parser against the repository Dockerfile. This catches
# legitimate non-image ARG defaults such as GOPROXY while retaining the
# immutable base-image contract.
(
  context_root="$(cd -- "$(dirname -- "$root_dockerfile")" && pwd)"
  eval "$dockerfile_contract_source"
  fail() { return 1; }
  validate_dockerfile_pin_contract || fail 'repository Dockerfile pin contract was rejected'
)

# Image absence fixture: transient inspect failures and unrelated errors must
# never be treated as permission to reuse or overwrite an existing tag.
(
  eval "$absence_source"
  fixture='missing'
  fixture_root="$(mktemp -d "${TMPDIR:-/tmp}/subnexus-image-absence-test.XXXXXX")"
  list_log="$fixture_root/list.log"
  trap 'rm -rf -- "$fixture_root"' EXIT
  docker_call() {
    if [[ "$1" == image && "$2" == inspect ]]; then
      case "$fixture" in
        missing) printf 'Error: No such image: candidate\n' >&2; return 1 ;;
        present) return 0 ;;
        transient) printf 'Error: daemon timeout\n' >&2; return 124 ;;
        misleading) printf 'Error: dependency not found\n' >&2; return 1 ;;
        *) return 1 ;;
      esac
    fi
    if [[ "$1" == image && "$2" == ls ]]; then
      printf '%s\n' image >>"$list_log"
      [[ "$fixture" == missing ]] && return 0
      printf '%s\n' 'sha256:existing'
      return 0
    fi
    if [[ "$1" == info ]]; then return 0; fi
    return 1
  }
  : >"$list_log"
  assert_exact_absent image candidate
  [[ "$(wc -l <"$list_log" | tr -d '[:space:]')" == 1 ]] || fail 'missing image was not corroborated with an exact list query'
  fixture='present'
  : >"$list_log"
  if assert_exact_absent image candidate; then fail 'existing image accepted as absent'; fi
  [[ ! -s "$list_log" ]] || fail 'present image unexpectedly performed an absence list query'
  fixture='transient'
  : >"$list_log"
  if assert_exact_absent image candidate; then fail 'transient image inspect error accepted as absent'; fi
  [[ ! -s "$list_log" ]] || fail 'timeout image unexpectedly performed an absence list query'
  fixture='misleading'
  : >"$list_log"
  if assert_exact_absent image candidate; then fail 'misleading not-found error accepted as absent'; fi
  [[ ! -s "$list_log" ]] || fail 'misleading image unexpectedly performed an absence list query'
)

# Exercise the embedded tar extractor against the modes emitted by Git/tar on
# WSL.  This keeps the permission normalization contract executable instead of
# relying only on source-text assertions.
if command -v python3 >/dev/null 2>&1 && python3 -c 'import sys; print(sys.version_info[:2])' >/dev/null 2>&1; then
  extractor_source="$(awk '/^  python3 - "[^"]*context_tar" "[^"]*context_root"/{capture=1; next} capture && /^PY$/{exit} capture{print}' "$subject")"
  [[ -n "$extractor_source" ]] || fail 'embedded context extractor was not found'
  fixture_root="$(mktemp -d "${TMPDIR:-/tmp}/subnexus-context-mode-test.XXXXXX")"
  trap 'rm -rf -- "$fixture_root"' EXIT
  printf '%s\n' "$extractor_source" >"$fixture_root/extractor.py"
  python3 - "$fixture_root/source.tar" <<'PY'
import io
import pathlib
import sys
import tarfile

archive = pathlib.Path(sys.argv[1])
with tarfile.open(archive, mode="w:") as bundle:
    directory = tarfile.TarInfo("nested/")
    directory.type = tarfile.DIRTYPE
    directory.mode = 0o777
    bundle.addfile(directory)
    regular = tarfile.TarInfo("nested/regular")
    regular.mode = 0o664
    regular.size = 1
    bundle.addfile(regular, io.BytesIO(b"x"))
    executable = tarfile.TarInfo("nested/exec.sh")
    executable.mode = 0o775
    executable.size = 2
    bundle.addfile(executable, io.BytesIO(b"x\n"))
PY
  python3 "$fixture_root/extractor.py" "$fixture_root/source.tar" "$fixture_root/context" 134217728
  [[ "$(stat -c '%a' "$fixture_root/context/nested")" == 755 ]] || fail 'directory mode was not normalized to 0755'
  [[ "$(stat -c '%a' "$fixture_root/context/nested/regular")" == 644 ]] || fail 'regular file mode was not normalized to 0644'
  [[ "$(stat -c '%a' "$fixture_root/context/nested/exec.sh")" == 755 ]] || fail 'executable file mode was not normalized to 0755'
else
  printf 'context mode dynamic fixture skipped: usable python3 is unavailable\n'
fi

# Exercise the embedded Docker-save archive validator with a real config
# digest and RootFS diff-ID list.  Windows Git Bash may expose only the
# AppInstaller python3 redirector, so this fixture is skipped unless a real
# interpreter can execute a trivial program.
if command -v python3 >/dev/null 2>&1 && python3 -c 'import hashlib, json, sys; print(sys.version_info[:2])' >/dev/null 2>&1; then
  archive_validator_source="$(awk '/^  python3 - "\$archive_tmp" "\$gate_tag" "\$rootfs_layers_json" <<'"'"'PY'"'"'$/{capture=1; next} capture && /^PY$/{exit} capture{print}' "$subject")"
  [[ -n "$archive_validator_source" ]] || fail 'embedded archive validator was not found'
  fixture_root="$(mktemp -d "${TMPDIR:-/tmp}/subnexus-archive-validator-test.XXXXXX")"
  trap 'rm -rf -- "$fixture_root"' EXIT
  printf '%s\n' "$archive_validator_source" >"$fixture_root/validator.py"
  python3 - "$fixture_root/make-fixtures.py" <<'PY'
import hashlib
import json
import pathlib
import sys
import tarfile

root = pathlib.Path(sys.argv[1]).parent
tag = "subnexus-release:" + "b" * 40
layers = ["sha256:" + "a" * 64, "sha256:" + "c" * 64]
config_bytes = json.dumps({"rootfs": {"type": "layers", "diff_ids": layers}}, separators=(",", ":")).encode()
config_id = hashlib.sha256(config_bytes).hexdigest()
manifest = [{"Config": f"blobs/sha256/{config_id}", "RepoTags": [tag], "Layers": ["layer.tar"]}]
with tarfile.open(root / "valid.tar", "w:") as archive:
    info = tarfile.TarInfo("manifest.json")
    manifest_bytes = json.dumps(manifest, separators=(",", ":")).encode()
    info.size = len(manifest_bytes)
    archive.addfile(info, __import__("io").BytesIO(manifest_bytes))
    info = tarfile.TarInfo(f"blobs/sha256/{config_id}")
    info.size = len(config_bytes)
    archive.addfile(info, __import__("io").BytesIO(config_bytes))
    archive.addfile(tarfile.TarInfo("layer.tar"), __import__("io").BytesIO())

bad_bytes = config_bytes.replace(b"a" * 64, b"d" * 64)
with tarfile.open(root / "bad-rootfs.tar", "w:") as archive:
    info = tarfile.TarInfo("manifest.json")
    manifest_bytes = json.dumps([{"Config": f"blobs/sha256/{hashlib.sha256(bad_bytes).hexdigest()}", "RepoTags": [tag], "Layers": ["layer.tar"]}], separators=(",", ":")).encode()
    info.size = len(manifest_bytes)
    archive.addfile(info, __import__("io").BytesIO(manifest_bytes))
    info = tarfile.TarInfo(f"blobs/sha256/{hashlib.sha256(bad_bytes).hexdigest()}")
    info.size = len(bad_bytes)
    archive.addfile(info, __import__("io").BytesIO(bad_bytes))
    archive.addfile(tarfile.TarInfo("layer.tar"), __import__("io").BytesIO())

bad_hash = config_bytes + b"\n"
with tarfile.open(root / "bad-hash.tar", "w:") as archive:
    info = tarfile.TarInfo("manifest.json")
    manifest_bytes = json.dumps([{"Config": f"blobs/sha256/{config_id}", "RepoTags": [tag], "Layers": ["layer.tar"]}], separators=(",", ":")).encode()
    info.size = len(manifest_bytes)
    archive.addfile(info, __import__("io").BytesIO(manifest_bytes))
    info = tarfile.TarInfo(f"blobs/sha256/{config_id}")
    info.size = len(bad_hash)
    archive.addfile(info, __import__("io").BytesIO(bad_hash))
    archive.addfile(tarfile.TarInfo("layer.tar"), __import__("io").BytesIO())

(root / "tag").write_text(tag)
(root / "layers.json").write_text(json.dumps(layers, separators=(",", ":")))
PY
  archive_tag="$(<"$fixture_root/tag")"
  archive_layers="$(<"$fixture_root/layers.json")"
  python3 "$fixture_root/validator.py" "$fixture_root/valid.tar" "$archive_tag" "$archive_layers"
  for invalid_archive in bad-rootfs.tar bad-hash.tar; do
    if python3 "$fixture_root/validator.py" "$fixture_root/$invalid_archive" "$archive_tag" "$archive_layers" >/dev/null 2>&1; then
      fail "archive validator accepted invalid fixture: $invalid_archive"
    fi
  done
else
  printf 'archive validator dynamic fixture skipped: usable python3 is unavailable\n'
fi

# Ensure line endings do not make the shell contract platform-dependent.
if grep -n $'\r' "$subject" >/dev/null; then
  fail 'image builder script must use LF line endings'
fi

printf 'subnexus isolated-image-build static tests passed\n'
