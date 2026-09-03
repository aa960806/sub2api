#!/usr/bin/env bash
set -Eeuo pipefail

# Static and helper tests for the local-only image builder.  These tests never
# invoke Docker, never contact a registry, and never access the production
# host.  The real build is intentionally operator-run only after a named local
# rootless/WSL daemon has been inspected.

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
subject="$script_dir/subnexus-isolated-image-build.sh"

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
assert_contains 'symlink or Git submodule is not allowed'
assert_contains 'sensitive tracked file is not allowed'
assert_contains 'is_safe_env_example_path() {'
assert_contains '.env.example|*/.env.example|.env.sample|*/.env.sample'
assert_contains 'validate_dockerfile_pin_contract() {'
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
assert_contains "--driver-opt 'pids-limit=512'"
assert_contains "--driver-opt 'restart-policy=no'"
assert_contains "--driver-opt 'pids-limit=512'"
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
assert_contains 'Driver: docker-container'
assert_contains 'Status: running'
assert_contains 'docker.sock'
assert_contains 'network options or labels are invalid'
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
assert_contains 'CANDIDATE_GATE_TAG'
assert_contains 'docker save'
assert_contains 'candidate-image.tar'
assert_contains 'IMAGE_ARCHIVE_SHA256'
assert_contains 'SHA256SUMS'
assert_contains 'metadata.env'
assert_contains 'manifest.json'
assert_contains 'RepoTags'
assert_contains 'atomic stage rename'
assert_contains 'mv -- "$stage_dir" "$final_dir"'

# Exercise pure Bash validators without sourcing the script's main entrypoint.
validator_source="$(sed -n '/^valid_immutable_image_ref() {$/,/^}$/p' "$subject")"
repository_validator_source="$(sed -n '/^valid_repository_digest_ref() {$/,/^}$/p' "$subject")"
context_source="$(sed -n '/^valid_context_name() {$/,/^}$/p' "$subject")"
tag_source="$(sed -n '/^valid_tag() {$/,/^}$/p' "$subject")"
env_example_source="$(sed -n '/^is_safe_env_example_path() {$/,/^}$/p' "$subject")"
absence_source="$(sed -n '/^assert_exact_absent() {$/,/^}$/p' "$subject")"
[[ -n "$validator_source" && -n "$repository_validator_source" && -n "$context_source" && -n "$tag_source" && -n "$env_example_source" && -n "$absence_source" ]] || fail 'validator functions not found'
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

# Ensure line endings do not make the shell contract platform-dependent.
if grep -n $'\r' "$subject" >/dev/null; then
  fail 'image builder script must use LF line endings'
fi

printf 'subnexus isolated-image-build static tests passed\n'
