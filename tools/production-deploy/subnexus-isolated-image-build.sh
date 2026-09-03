#!/usr/bin/env bash
set -Eeuo pipefail

# Build one fixed SubNexus commit on an explicitly named, local-only Docker
# context.  This script creates a disposable BuildKit builder and network,
# never pulls or pushes images, never uses SSH, and never contacts a server.
# The resulting image archive is suitable for the production-host candidate
# gate, but this script itself does not run a candidate or touch production.

case "$-" in
  *x*) set +x ;;
esac
umask 077
unset BASH_ENV ENV CDPATH GLOBIGNORE
export PATH='/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin'

readonly script_name='subnexus-isolated-image-build-v1'
readonly gate_label='subnexus-isolated-build-v1'
readonly default_timeout_seconds=7200
readonly max_timeout_seconds=21600
readonly max_archive_bytes=12884901888
readonly max_context_file_bytes=134217728

failure_reason=''
interrupted='false'
cleanup_failed='false'
cleanup_started='false'
build_pid=''
build_pgid=''
build_pid_file=''
builder_created='false'
builder_create_attempted='false'
builder_id=''
builder_name=''
builder_network_id=''
builder_network_name=''
builder_network_create_attempted='false'
builder_cleanup_done='false'
builder_network_cleanup_done='false'
release_image_created='false'
gate_tag_created='false'
gate_tag_preexisting='false'
release_tag=''
gate_tag=''
run_token=''
run_uuid=''
timestamp=''
source_root=''
approved_sha=''
tree_sha=''
artifact_root=''
stage_dir=''
final_dir=''
context_root=''
archive_stage=''
archive_final=''
metadata_stage=''
metadata_final=''
checksums_stage=''
checksums_final=''
base_images_stage=''
build_log_stage=''
builder_inspect_stage=''
archive_sha256=''
archive_size=''
script_sha256=''
image_id=''
commit_epoch=''
commit_date=''
docker_binary=''
docker_context=''
docker_endpoint=''
docker_socket_fingerprint_start=''
docker_daemon_id_start=''
docker_daemon_name=''
docker_server_version=''
docker_root_dir=''
docker_swarm_state=''
docker_security_options=''
docker_rpc_timeout_seconds=''
build_timeout_seconds=''
node_image_ref=''
golang_image_ref=''
alpine_image_ref=''
postgres_image_ref=''
buildkit_image_ref=''
node_image_id=''
golang_image_id=''
alpine_image_id=''
postgres_image_id=''
buildkit_image_id=''
source_tar=''
baseline_containers=''
baseline_networks=''
baseline_volumes=''
baseline_images=''
build_status=''
script_source_path=''

fail() {
  failure_reason="$*"
  printf 'ERROR: %s\n' "$*" >&2
  exit 1
}

usage() {
  cat >&2 <<'USAGE'
Usage:
  SUBNEXUS_BUILD_DOCKER_CONTEXT=<named-local-context> \
  SUBNEXUS_LOCAL_DOCKER_CONFIRM=I_UNDERSTAND_LOCAL_ONLY \
  SUBNEXUS_CANDIDATE_NODE_IMAGE=<repo@sha256:digest> \
  SUBNEXUS_CANDIDATE_GOLANG_IMAGE=<repo@sha256:digest> \
  SUBNEXUS_CANDIDATE_ALPINE_IMAGE=<repo@sha256:digest> \
  SUBNEXUS_CANDIDATE_POSTGRES_IMAGE=<repo@sha256:digest> \
  SUBNEXUS_CANDIDATE_BUILDKIT_IMAGE=<repo@sha256:digest> \
  subnexus-isolated-image-build.sh SOURCE_ROOT APPROVED_COMMIT_SHA [ARTIFACT_ROOT]

The Docker context must be explicitly named and local (for example a
rootless/WSL context named subnexus-local). The default, SSH, TCP, remote,
and production contexts are rejected. The daemon must have no containers,
custom networks, or volumes before this script starts; it never prunes them.

On success the script prints RELEASE_TAG, CANDIDATE_GATE_TAG, IMAGE_ID,
IMAGE_ARCHIVE, IMAGE_ARCHIVE_SHA256, TREE_SHA, and METADATA. The archive is
saved with only CANDIDATE_GATE_TAG (subnexus-release:<commit SHA>) so it can
be consumed by the production candidate gate.
USAGE
}

valid_sha40() {
  [[ "${1:-}" =~ ^[0-9a-f]{40}$ ]]
}

valid_sha256_hex() {
  [[ "${1:-}" =~ ^[0-9a-f]{64}$ ]]
}

valid_full_image_id() {
  [[ "${1:-}" =~ ^sha256:[0-9a-f]{64}$ ]]
}

# Accept the same immutable syntax used by the runtime candidate gate. Build
# arguments below additionally require repository@sha256 refs because Docker
# FROM cannot portably consume a bare content ID.
valid_immutable_image_ref() {
  # Keep an optional registry host/port separate from the repository path so
  # a tag colon cannot be confused with a registry port.  Build args retain
  # the canonical repository reference for Docker's digest checks.
  [[ "${1:-}" =~ ^(([A-Za-z0-9][A-Za-z0-9._-]*(\:[0-9]{1,5})?\/)?[A-Za-z0-9][A-Za-z0-9._/-]*)(\:[A-Za-z0-9._-]+)?@sha256:[0-9a-f]{64}$|^sha256:[0-9a-f]{64}$ ]]
}

valid_repository_digest_ref() {
  [[ "${1:-}" =~ ^(([A-Za-z0-9][A-Za-z0-9._-]*(\:[0-9]{1,5})?\/)?[A-Za-z0-9][A-Za-z0-9._/-]*)(\:[A-Za-z0-9._-]+)?@sha256:[0-9a-f]{64}$ ]]
}

valid_context_name() {
  local value="${1:-}"
  [[ "$value" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$ ]] || return 1
  case "${value,,}" in
    default|prod|production|remote|server|live|ssh-*|prod-*|production-*|*prod*|*production*|*remote*|*server*|*live*) return 1 ;;
  esac
}

valid_tag() {
  [[ "${1:-}" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$ ]]
}

is_safe_env_example_path() {
  case "${1:-}" in
    .env.example|*/.env.example|.env.sample|*/.env.sample) return 0 ;;
    *) return 1 ;;
  esac
}

valid_positive_integer() {
  [[ "${1:-}" =~ ^[0-9]+$ ]] && (( 10#${1:-0} > 0 ))
}

mode_is_safe() {
  local path="$1" mode
  mode="$(stat -c '%a' -- "$path")" || return 1
  [[ "$mode" =~ ^[0-7]{3,4}$ ]] || return 1
  (( (8#$mode & 8#022) == 0 ))
}

owner_is_allowed() {
  local path="$1" owner
  owner="$(stat -c '%u' -- "$path")" || return 1
  [[ "$owner" == "$EUID" || "$owner" == '0' ]]
}

validate_secure_directory() {
  local path="$1"
  [[ -d "$path" && ! -L "$path" ]] || return 1
  [[ "$(realpath -e -- "$path")" == "$path" ]] || return 1
  owner_is_allowed "$path" || return 1
  mode_is_safe "$path" || return 1
}

ensure_secure_directory() {
  local path="$1" parent
  parent="${path%/*}"
  if [[ ! -e "$path" && ! -L "$path" ]]; then
    [[ -d "$parent" && ! -L "$parent" ]] || fail "secure parent is missing: $parent" || return 1
    owner_is_allowed "$parent" || fail "secure parent is not owned by the invoking user: $parent" || return 1
    mode_is_safe "$parent" || fail "secure parent is writable by group/other: $parent" || return 1
    mkdir -- "$path" || fail "cannot create secure directory: $path" || return 1
  fi
  validate_secure_directory "$path" || fail "unsafe artifact directory: $path" || return 1
  chmod 700 -- "$path" || fail "cannot protect artifact directory: $path" || return 1
}

hash_file() {
  sha256sum -- "$1" | awk 'NF == 2 {print tolower($1)}'
}

assert_script_unchanged() {
  local current
  [[ -n "$script_source_path" && -f "$script_source_path" && ! -L "$script_source_path" ]] || return 1
  current="$(hash_file "$script_source_path")" || return 1
  [[ "$current" == "$script_sha256" ]]
}

safe_remove_stage() {
  local path="$1"
  [[ -z "$path" ]] && return 0
  [[ "$path" == "$artifact_root/.$run_token.stage" ]] || {
    printf 'ERROR: refusing to remove an unexpected staging path: %s\n' "$path" >&2
    return 1
  }
  [[ -d "$path" && ! -L "$path" ]] || return 0
  [[ "$(realpath -e -- "$path" 2>/dev/null)" == "$path" ]] || return 1
  owner_is_allowed "$path" || return 1
  mode_is_safe "$path" || return 1
  rm -rf -- "$path"
}

docker_call() {
  timeout --foreground --kill-after=10s "${docker_rpc_timeout_seconds}s" \
    "$docker_binary" --context "$docker_context" "$@"
}

docker_quick_call() {
  timeout --foreground --kill-after=5s 20s \
    "$docker_binary" --context "$docker_context" "$@"
}

docker_socket_fingerprint() {
  if [[ "$docker_endpoint" == unix://* ]]; then
    stat -Lc '%d|%i|%F|%u|%g|%a' -- "${docker_endpoint#unix://}"
  else
    printf '%s' "$docker_endpoint"
  fi
}

assert_daemon_unchanged() {
  local phase="$1" current_id current_socket current_root
  current_id="$(docker_call info --format '{{.ID}}' 2>/dev/null)" || {
    printf 'ERROR: Docker daemon unavailable during %s\n' "$phase" >&2
    return 1
  }
  current_socket="$(docker_socket_fingerprint 2>/dev/null)" || return 1
  current_root="$(docker_call info --format '{{.DockerRootDir}}' 2>/dev/null)" || return 1
  [[ "$current_id" == "$docker_daemon_id_start" ]] || {
    printf 'ERROR: Docker daemon identity changed during %s\n' "$phase" >&2
    return 1
  }
  [[ "$current_socket" == "$docker_socket_fingerprint_start" ]] || {
    printf 'ERROR: Docker endpoint identity changed during %s\n' "$phase" >&2
    return 1
  }
  [[ "$current_root" == "$docker_root_dir" ]] || {
    printf 'ERROR: Docker root directory changed during %s\n' "$phase" >&2
    return 1
  }
}

assert_exact_absent() {
  local kind="$1" name="$2" listing inspect_output inspect_status inspect_error
  case "$kind" in
    network)
      listing="$(docker_call network ls --filter "name=^${name}$" --format '{{.ID}}')" || return 1
      ;;
    builder)
      listing="$(docker_call buildx ls | awk -v wanted="$name" '$1 == wanted || $1 == wanted "*" {print $1}')" || return 1
      ;;
    image)
      # Docker uses status 1 for both a missing image and some client/daemon
      # errors. Require an explicit not-found diagnostic plus an exact listing
      # query before treating the tag as available for creation.
      if inspect_output="$(docker_call image inspect "$name" 2>&1)"; then
        return 1
      else
        inspect_status=$?
      fi
      [[ "$inspect_status" -eq 1 ]] || return 1
      inspect_error="${inspect_output,,}"
      case "$inspect_error" in
        *'no such object'*|*'no such image'*) ;;
        *) return 1 ;;
      esac
      listing="$(docker_call image ls --no-trunc --filter "reference=$name" --format '{{.ID}}')" || return 1
      [[ -z "$(printf '%s' "$listing" | tr -d '\r\n')" ]] || return 1
      docker_call info >/dev/null 2>&1 || return 1
      return 0
      ;;
    *) return 1 ;;
  esac
  [[ -z "$listing" ]]
}

capture_object_lists() {
  baseline_containers="$(docker_call ps -aq --no-trunc | sort -u)" || return 1
  baseline_networks="$(docker_call network ls --format '{{.Name}}' | sort -u)" || return 1
  baseline_volumes="$(docker_call volume ls -q | sort -u)" || return 1
  baseline_images="$(docker_call image ls -aq --no-trunc | sort -u)" || return 1
}

assert_empty_build_daemon() {
  local custom_networks
  [[ -z "$baseline_containers" ]] || fail 'local build daemon already has containers; refusing to risk another workload' || return 1
  [[ -z "$baseline_volumes" ]] || fail 'local build daemon already has volumes; use a fresh rootless/WSL daemon' || return 1
  custom_networks="$(printf '%s\n' "$baseline_networks" | grep -Ev '^(bridge|host|none)$' || true)"
  [[ -z "$custom_networks" ]] || fail 'local build daemon already has custom networks; refusing to share them' || return 1
}

validate_endpoint() {
  case "$docker_endpoint" in
    unix:///*)
      local socket="${docker_endpoint#unix://}" socket_owner socket_mode
      [[ "$socket" != '/var/run/docker.sock' && "$socket" != '/run/docker.sock' ]] || {
        [[ "${SUBNEXUS_ALLOW_LOCAL_SYSTEM_SOCKET:-}" == 'YES' ]] ||
          fail 'system Docker socket is rejected; use a rootless/WSL context or explicitly opt in' || return 1
      }
      [[ -S "$socket" && ! -L "$socket" ]] || fail 'Docker endpoint is not a non-symlink Unix socket' || return 1
      socket_owner="$(stat -c '%u' -- "$socket")" || return 1
      [[ "$socket_owner" == "$EUID" || ( "$EUID" == '0' && "$socket_owner" == '0' ) ]] ||
        fail 'Docker socket is not owned by the invoking user' || return 1
      socket_mode="$(stat -c '%a' -- "$socket")" || return 1
      [[ "$socket_mode" =~ ^[0-7]{3,4}$ ]] || return 1
      (( (8#$socket_mode & 8#002) == 0 && (8#$socket_mode & 8#600) == 8#600 )) ||
        fail 'Docker socket permissions are too broad or not owner-rw' || return 1
      ;;
    npipe:///*|npipe:////./pipe/*|npipe:////localhost/pipe/*)
      [[ "${SUBNEXUS_ALLOW_LOCAL_NPIPE:-}" == 'YES' ]] ||
        fail 'Windows named pipes require SUBNEXUS_ALLOW_LOCAL_NPIPE=YES' || return 1
      ;;
    *)
      fail 'Docker context endpoint must be a local Unix socket or explicitly approved local named pipe' || return 1
      ;;
  esac
}

validate_base_image() {
  local role="$1" ref="$2" image_id os architecture repo_digests digest requested_name requested_repo requested_component
  valid_immutable_image_ref "$ref" || fail "$role image reference is not immutable: $ref" || return 1
  valid_repository_digest_ref "$ref" || fail "$role image must use repository@sha256 digest syntax: $ref" || return 1
  image_id="$(docker_call image inspect --format '{{.Id}}' "$ref")" ||
    fail "$role image is not present locally; pre-load the exact digest on the isolated daemon" || return 1
  valid_full_image_id "$image_id" || fail "$role image did not resolve to a full content ID" || return 1
  digest="${ref##*@sha256:}"
  repo_digests="$(docker_call image inspect --format '{{json .RepoDigests}}' "$ref")" || return 1
  # Docker normally records RepoDigests without the source tag (for example,
  # node:24@sha256:... is recorded as node@sha256:...).  Compare both the
  # repository and digest, rather than accepting a digest belonging to another
  # repository.
  requested_name="${ref%@sha256:*}"
  requested_component="${requested_name##*/}"
  if [[ "$requested_component" == *:* ]]; then
    requested_repo="${requested_name%:*}"
  else
    requested_repo="$requested_name"
  fi
  [[ "$repo_digests" == *"\"${requested_repo}@sha256:${digest}\""* ]] ||
    fail "$role image RepoDigests does not contain the requested repository digest" || return 1
  os="$(docker_call image inspect --format '{{.Os}}' "$ref")" || return 1
  architecture="$(docker_call image inspect --format '{{.Architecture}}' "$ref")" || return 1
  [[ "$os" == 'linux' && "$architecture" == 'amd64' ]] ||
    fail "$role image must be linux/amd64" || return 1
  printf '%s|%s|%s|%s/%s\n' "$role" "$ref" "${image_id#sha256:}" "$os" "$architecture" >>"$base_images_stage"
  printf '%s' "${image_id#sha256:}"
}

validate_source_tree() {
  local worktree head symbolic status submodules tree_mode tree_type path
  source_root="$(realpath -e -- "$source_root")" || fail 'source root does not exist' || return 1
  [[ -d "$source_root" && ! -L "$source_root" ]] || fail 'source root must be a non-symlink directory' || return 1
  owner_is_allowed "$source_root" || fail 'source root is not owned by the invoking user' || return 1
  mode_is_safe "$source_root" || fail 'source root is writable by group/other' || return 1
  worktree="$(git -C "$source_root" rev-parse --show-toplevel 2>/dev/null)" || fail 'source root is not a Git worktree' || return 1
  worktree="$(realpath -e -- "$worktree")" || return 1
  [[ "$worktree" == "$source_root" ]] || fail 'source root is not the Git worktree root' || return 1
  head="$(git -C "$source_root" rev-parse HEAD 2>/dev/null)" || return 1
  [[ "$head" == "$approved_sha" ]] || fail 'worktree HEAD is not the approved commit' || return 1
  symbolic="$(git -C "$source_root" symbolic-ref --quiet --short HEAD 2>/dev/null || true)"
  [[ -z "$symbolic" ]] || fail 'source worktree must be detached at the approved commit' || return 1
  status="$(git -C "$source_root" status --porcelain=v1 --untracked-files=all 2>/dev/null)" || return 1
  [[ -z "$status" ]] || fail 'source worktree must be clean' || return 1
  submodules="$(git -C "$source_root" submodule status --recursive 2>/dev/null)" ||
    fail 'cannot inspect Git submodules' || return 1
  [[ -z "$submodules" ]] || fail 'Git submodules are not allowed in the fixed build context' || return 1
  tree_sha="$(git -C "$source_root" rev-parse "$approved_sha^{tree}" 2>/dev/null)" || return 1
  [[ "$tree_sha" =~ ^[0-9a-f]{40}$ ]] || fail 'approved tree SHA is invalid' || return 1
  while IFS=' ' read -r tree_mode tree_type _; do
    case "$tree_mode:$tree_type" in
      120000:blob|160000:commit)
        fail "symlink or Git submodule is not allowed in the fixed build tree: $tree_mode $tree_type" || return 1
        ;;
    esac
  done < <(git -C "$source_root" ls-tree -r "$approved_sha")
  while IFS= read -r -d '' path; do
    case "$path" in
      .env|*/.env|.env.*|*/.env.*|*.pem|*.key|*.p12|*.pfx|*.jks)
        is_safe_env_example_path "$path" ||
          fail "sensitive tracked file is not allowed in the build source: $path" || return 1
        ;;
    esac
  done < <(git -C "$source_root" ls-tree -r -z --name-only "$approved_sha")
  commit_epoch="$(git -C "$source_root" show -s --format=%ct "$approved_sha")" || return 1
  [[ "$commit_epoch" =~ ^[0-9]+$ ]] || fail 'approved commit timestamp is invalid' || return 1
  commit_date="$(date -u -d "@$commit_epoch" +%Y-%m-%dT%H:%M:%SZ)" || return 1
}

extract_fixed_context() {
  local context_tar="$stage_dir/source.tar"
  source_tar="$context_tar"
  git -C "$source_root" archive --format=tar "$approved_sha" >"$context_tar" ||
    fail 'git archive failed for the approved commit' || return 1
  chmod 600 -- "$context_tar"
  python3 - "$context_tar" "$context_root" "$max_context_file_bytes" <<'PY'
import pathlib
import shutil
import stat
import sys
import tarfile

archive = pathlib.Path(sys.argv[1])
root = pathlib.Path(sys.argv[2])
max_file = int(sys.argv[3])
root.mkdir(parents=True, exist_ok=False)
members = []
seen = set()
total = 0
with tarfile.open(archive, mode="r:") as bundle:
    for member in bundle.getmembers():
        name = member.name
        path = pathlib.PurePosixPath(name)
        if not name or name in seen or path.is_absolute() or ".." in path.parts or "\\" in name:
            raise SystemExit("unsafe or duplicate git archive path")
        seen.add(name)
        if not (member.isdir() or member.isreg()):
            raise SystemExit("git archive contains a symlink or special file")
        if member.size < 0 or member.size > max_file:
            raise SystemExit("git archive member is too large")
        total += member.size
        if total > 8 * 1024**3:
            raise SystemExit("git archive is too large")
        members.append(member)
    for member in sorted(members, key=lambda item: (not item.isdir(), item.name)):
        target = root.joinpath(*pathlib.PurePosixPath(member.name).parts)
        parent = target.parent
        parent.mkdir(parents=True, exist_ok=True)
        if parent.is_symlink() or target.exists() or target.is_symlink():
            raise SystemExit("archive extraction would overwrite or follow a link")
        if member.isdir():
            target.mkdir()
            target.chmod(member.mode & 0o777)
            continue
        source = bundle.extractfile(member)
        if source is None:
            raise SystemExit("archive member cannot be read")
        with target.open("xb") as output:
            shutil.copyfileobj(source, output, length=1024 * 1024)
        target.chmod(member.mode & 0o777)
PY
  rm -- "$context_tar"
  source_tar=''
}

validate_context_tree() {
  local path relative basename
  [[ -d "$context_root" && ! -L "$context_root" ]] || fail 'fixed Docker context is not a directory' || return 1
  owner_is_allowed "$context_root" || fail 'fixed Docker context is not owned by the invoking user' || return 1
  mode_is_safe "$context_root" || fail 'fixed Docker context is writable by group/other' || return 1
  while IFS= read -r -d '' path; do
    [[ ! -L "$path" ]] || fail 'symbolic link found in fixed Docker context' || return 1
    owner_is_allowed "$path" || fail 'fixed Docker context entry has an unsafe owner' || return 1
    mode_is_safe "$path" || fail 'fixed Docker context entry is writable by group/other' || return 1
    [[ -f "$path" || -d "$path" ]] || fail 'special file found in fixed Docker context' || return 1
    if [[ -f "$path" ]]; then
      [[ "$(stat -c '%h' -- "$path")" == '1' ]] || fail 'hard-linked file found in fixed Docker context' || return 1
      [[ "$(stat -c '%s' -- "$path")" -le "$max_context_file_bytes" ]] || fail 'fixed Docker context file is too large' || return 1
    fi
    relative="${path#"$context_root"/}"
    basename="${relative##*/}"
    case "$relative" in
      .env|.env.*|*.pem|*.key|*.p12|*.pfx|*.jks)
        is_safe_env_example_path "$relative" ||
          fail "sensitive file found in fixed Docker context: $relative" || return 1
        ;;
    esac
    case "$basename" in
      *dump.sql|*dump.sql.gz|*.dump|*.dump.gz|*backup.sql|*backup.sql.gz|*snapshot.sql|*snapshot.sql.gz|*export.sql|*export.sql.gz)
        fail "database dump found in fixed Docker context: $relative" || return 1
        ;;
    esac
  done < <(find "$context_root" -mindepth 1 -print0)
  [[ -f "$context_root/Dockerfile" && ! -L "$context_root/Dockerfile" ]] || fail 'fixed Docker context has no Dockerfile' || return 1
  [[ -f "$context_root/.dockerignore" && ! -L "$context_root/.dockerignore" ]] || fail 'fixed Docker context has no .dockerignore' || return 1
}

validate_dockerfile_pin_contract() {
  local dockerfile="$context_root/Dockerfile" line
  for line in 'ARG NODE_IMAGE=' 'ARG GOLANG_IMAGE=' 'ARG ALPINE_IMAGE=' 'ARG POSTGRES_IMAGE='; do
    grep -Fq -- "$line" "$dockerfile" || fail "Dockerfile is missing required base argument: $line" || return 1
  done
  # Every FROM expression must refer to one of the four supplied digest args;
  # this catches a newly-added mutable base before it reaches Docker.
  while IFS= read -r line; do
    [[ "$line" == FROM\ * ]] || continue
    case "$line" in
      *'${NODE_IMAGE}'*|*'${GOLANG_IMAGE}'*|*'${ALPINE_IMAGE}'*|*'${POSTGRES_IMAGE}'*) ;;
      *) fail "Dockerfile has an unapproved or mutable FROM: $line" || return 1 ;;
    esac
  done <"$dockerfile"
}

create_builder_network() {
  local output network_internal network_driver network_attachable network_ipv6 labels
  assert_exact_absent network "$builder_network_name" || fail 'BuildKit network name is already in use' || return 1
  builder_network_create_attempted='true'
  output="$(docker_call network create --driver bridge --ipv6=false \
    --label "com.subnexus.isolated-build.gate=$script_name" \
    --label "com.subnexus.isolated-build.token=$run_token" "$builder_network_name")" ||
    fail 'cannot create isolated BuildKit network' || return 1
  output="${output//$'\r'/}"; output="${output//$'\n'/}"
  [[ "$output" =~ ^[0-9a-f]{64}$ ]] || fail 'Docker returned an invalid BuildKit network ID' || return 1
  builder_network_id="$output"
  network_driver="$(docker_call network inspect --format '{{.Driver}}' "$builder_network_id")" || return 1
  network_internal="$(docker_call network inspect --format '{{.Internal}}' "$builder_network_id")" || return 1
  network_attachable="$(docker_call network inspect --format '{{.Attachable}}' "$builder_network_id")" || return 1
  network_ipv6="$(docker_call network inspect --format '{{.EnableIPv6}}' "$builder_network_id")" || return 1
  labels="$(docker_call network inspect --format '{{json .Labels}}' "$builder_network_id")" || return 1
  [[ "$network_driver" == 'bridge' && "$network_internal" == 'false' &&
    "$network_attachable" == 'false' && "$network_ipv6" == 'false' &&
    "$labels" == *"$script_name"* && "$labels" == *"$run_token"* ]] ||
    fail 'isolated BuildKit network options or labels are invalid' || return 1
}

validate_builder_container() {
  local id="$1" observed_name observed_image network_mode ports privileged pid_mode ipc_mode mounts labels
  local memory memory_swap cpu_quota cpu_period pids_limit restart_policy security_opt devices device_requests volumes_from
  [[ "$id" =~ ^[0-9a-f]{64}$ ]] || return 1
  observed_name="$(docker_call inspect --format '{{.Name}}' "$id")" || return 1
  observed_image="$(docker_call inspect --format '{{.Image}}' "$id")" || return 1
  network_mode="$(docker_call inspect --format '{{.HostConfig.NetworkMode}}' "$id")" || return 1
  memory="$(docker_call inspect --format '{{.HostConfig.Memory}}' "$id")" || return 1
  memory_swap="$(docker_call inspect --format '{{.HostConfig.MemorySwap}}' "$id")" || return 1
  cpu_quota="$(docker_call inspect --format '{{.HostConfig.CpuQuota}}' "$id")" || return 1
  cpu_period="$(docker_call inspect --format '{{.HostConfig.CpuPeriod}}' "$id")" || return 1
  pids_limit="$(docker_call inspect --format '{{.HostConfig.PidsLimit}}' "$id")" || return 1
  ports="$(docker_call inspect --format '{{json .HostConfig.PortBindings}}' "$id")" || return 1
  privileged="$(docker_call inspect --format '{{.HostConfig.Privileged}}' "$id")" || return 1
  pid_mode="$(docker_call inspect --format '{{.HostConfig.PidMode}}' "$id")" || return 1
  ipc_mode="$(docker_call inspect --format '{{.HostConfig.IpcMode}}' "$id")" || return 1
  mounts="$(docker_call inspect --format '{{range .Mounts}}{{printf "%s|%s|%s\n" .Type .Source .Destination}}{{end}}' "$id")" || return 1
  labels="$(docker_call inspect --format '{{json .Config.Labels}}' "$id")" || return 1
  restart_policy="$(docker_call inspect --format '{{.HostConfig.RestartPolicy.Name}}' "$id")" || return 1
  security_opt="$(docker_call inspect --format '{{json .HostConfig.SecurityOpt}}' "$id")" || return 1
  devices="$(docker_call inspect --format '{{json .HostConfig.Devices}}' "$id")" || return 1
  device_requests="$(docker_call inspect --format '{{json .HostConfig.DeviceRequests}}' "$id")" || return 1
  volumes_from="$(docker_call inspect --format '{{json .HostConfig.VolumesFrom}}' "$id")" || return 1
  [[ "$observed_name" == "/buildx_buildkit_${builder_name}0" &&
    "$observed_image" == "sha256:$buildkit_image_id" &&
    ( "$network_mode" == "$builder_network_name" || "$network_mode" == "$builder_network_id" ) &&
    "$memory" == '4294967296' && "$memory_swap" == '4294967296' &&
    "$cpu_quota" == '200000' && "$cpu_period" == '100000' &&
    "$pids_limit" == '512' &&
    ( "$ports" == '{}' || "$ports" == 'null' ) && "$pid_mode" == '' && "$ipc_mode" == 'private' &&
    "$restart_policy" == 'no' &&
    "$labels" == *'com.docker.buildx.builder'* && "$labels" == *'com.docker.buildx.driver'* ]] || return 1
  [[ "$privileged" == 'true' || "$privileged" == 'false' ]] || return 1
  [[ "$security_opt" != *'seccomp=unconfined'* && "$security_opt" != *'apparmor=unconfined'* ]] || return 1
  [[ "$devices" == '[]' || "$devices" == 'null' ]] || return 1
  [[ "$device_requests" == '[]' || "$device_requests" == 'null' ]] || return 1
  [[ "$volumes_from" == '[]' || "$volumes_from" == 'null' ]] || return 1
  [[ "$mounts" != *'docker.sock'* ]] || return 1
}

create_builder() {
  local output builder_listing
  assert_exact_absent builder "$builder_name" || fail 'BuildKit builder name is already in use' || return 1
  builder_create_attempted='true'
  output="$(docker_call buildx create --name "$builder_name" --driver docker-container \
    --driver-opt "image=$buildkit_image_ref" --driver-opt "network=$builder_network_name" \
    --driver-opt 'memory=4g' --driver-opt 'memory-swap=4g' \
    --driver-opt 'cpu-quota=200000' --driver-opt 'cpu-period=100000' \
    --driver-opt 'pids-limit=512' --driver-opt 'restart-policy=no')" ||
    fail 'cannot create isolated BuildKit builder' || return 1
  output="${output//$'\r'/}"; output="${output//$'\n'/}"
  [[ "$output" == "$builder_name" ]] || fail 'BuildKit returned an unexpected builder name' || return 1
  builder_created='true'
  docker_call buildx inspect --bootstrap "$builder_name" >"$builder_inspect_stage" 2>&1 ||
    fail 'isolated BuildKit builder failed to bootstrap' || return 1
  grep -Fq 'Driver: docker-container' "$builder_inspect_stage" || fail 'builder driver is not docker-container' || return 1
  grep -Fq 'Status: running' "$builder_inspect_stage" || fail 'isolated BuildKit builder is not running' || return 1
  builder_listing="$(docker_call ps --all --no-trunc --filter "name=^/buildx_buildkit_${builder_name}0$" --format '{{.ID}}')" ||
    fail 'cannot locate isolated BuildKit container' || return 1
  [[ "$builder_listing" =~ ^[0-9a-f]{64}$ ]] || fail 'BuildKit builder container identity is ambiguous' || return 1
  builder_id="$builder_listing"
  validate_builder_container "$builder_id" || fail 'BuildKit builder container isolation validation failed' || return 1
}

kill_build_group() {
  local attempt recorded_pgid
  if [[ -z "$build_pgid" ]]; then
    # A signal can arrive between starting setsid and the inner shell writing
    # its PID.  Use the record when it is already available before falling
    # back to terminating the setsid waiter itself.
    if [[ -n "$build_pid_file" && -s "$build_pid_file" ]]; then
      recorded_pgid="$(tr -d '\r\n' <"$build_pid_file" 2>/dev/null || true)"
      if [[ "$recorded_pgid" =~ ^[0-9]+$ && "$recorded_pgid" != "$$" ]]; then
        build_pgid="$recorded_pgid"
      fi
    fi
  fi
  if [[ -z "$build_pgid" ]]; then
    # If the session leader has not published its PID yet, terminate the
    # setsid waiter itself rather than waiting indefinitely for a failed build.
    if [[ -n "$build_pid" && "$build_pid" =~ ^[0-9]+$ && "$build_pid" != "$$" ]]; then
      kill -TERM "$build_pid" 2>/dev/null || true
      for attempt in 1 2 3 4 5; do
        kill -0 "$build_pid" 2>/dev/null || return 0
        sleep 1
      done
      kill -KILL "$build_pid" 2>/dev/null || true
    fi
    return 0
  fi
  [[ "$build_pgid" =~ ^[0-9]+$ && "$build_pgid" != "$$" ]] || return 0
  kill -TERM -- "-$build_pgid" 2>/dev/null || true
  for attempt in 1 2 3 4 5; do
    kill -0 -- "-$build_pgid" 2>/dev/null || break
    sleep 1
  done
  kill -KILL -- "-$build_pgid" 2>/dev/null || true
  # With job control enabled, `setsid --wait` can remain as a parent outside
  # the new session while the actual Docker process lives in the recorded
  # process group.  Ensure that waiter cannot outlive the group indefinitely.
  if [[ -n "$build_pid" && "$build_pid" =~ ^[0-9]+$ && "$build_pid" != "$$" ]]; then
    kill -KILL "$build_pid" 2>/dev/null || true
  fi
}

run_build() {
  local elapsed=0 status=0 attempt recorded_pgid
  build_pid=''; build_pgid=''
  build_pid_file="$stage_dir/build.pid"
  rm -f -- "$build_pid_file"
  set +e
  # The inner shell is the session leader created by setsid.  It records its
  # own PID before exec so cleanup always targets the real Docker process
  # group, even when setsid had to fork a waiter for an interactive caller.
  setsid --wait /bin/sh -c '
    pid_file=$1
    shift
    printf "%s\n" "$$" >"$pid_file" || exit 125
    export DOCKER_BUILDKIT=1
    exec "$@"
  ' subnexus-build-wrapper "$build_pid_file" "$docker_binary" --context "$docker_context" buildx build \
    --builder "$builder_name" --platform linux/amd64 --load --pull=false --no-cache \
    --provenance=false --sbom=false --progress=plain \
    --label "com.subnexus.release.gate=$gate_label" \
    --label "com.subnexus.candidate.gate=$gate_label" \
    --label "com.subnexus.candidate.token=$run_token" \
    --label "com.subnexus.candidate.commit=$approved_sha" \
    --label "com.subnexus.candidate.tree=$tree_sha" \
    --label "org.opencontainers.image.revision=$approved_sha" \
    --label "org.opencontainers.image.created=$commit_date" \
    --build-arg "NODE_IMAGE=$node_image_ref" \
    --build-arg "GOLANG_IMAGE=$golang_image_ref" \
    --build-arg "ALPINE_IMAGE=$alpine_image_ref" \
    --build-arg "POSTGRES_IMAGE=$postgres_image_ref" \
    --build-arg "COMMIT=$approved_sha" \
    --build-arg "DATE=$commit_date" \
    --file "$context_root/Dockerfile" --tag "$release_tag" "$context_root" \
    >"$build_log_stage" 2>&1 &
  build_pid=$!
  # Wait briefly for the inner session leader to publish its PID.  A missing
  # or malformed record is treated as a hard failure before any build timeout
  # loop can proceed with an untracked process.
  for attempt in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20; do
    if [[ -s "$build_pid_file" ]]; then
      recorded_pgid="$(tr -d '\r\n' <"$build_pid_file" 2>/dev/null || true)"
      if [[ "$recorded_pgid" =~ ^[0-9]+$ && "$recorded_pgid" != "$$" ]] &&
        kill -0 -- "-$recorded_pgid" 2>/dev/null; then
        build_pgid="$recorded_pgid"
        break
      fi
    fi
    kill -0 "$build_pid" 2>/dev/null || break
    sleep 0.1
  done
  if [[ -z "$build_pgid" ]]; then
    failure_reason='isolated Docker build process group could not be established'
    kill_build_group
    wait "$build_pid" 2>/dev/null
    status=$?
    rm -f -- "$build_pid_file"
    build_pid=''; build_pgid=''; build_pid_file=''
    (( status == 0 )) && status=1
    build_status="$status"
    fail "$failure_reason" || return 1
  fi
  while kill -0 "$build_pid" 2>/dev/null; do
    sleep 10
    elapsed=$((elapsed + 10))
    assert_daemon_unchanged build_in_progress || {
      failure_reason='Docker daemon identity changed while the image was building'
      kill_build_group
      break
    }
    if (( elapsed >= build_timeout_seconds )); then
      failure_reason='candidate image build exceeded its timeout'
      kill_build_group
      break
    fi
  done
  wait "$build_pid"
  status=$?
  rm -f -- "$build_pid_file" || status=1
  build_pid=''; build_pgid=''
  build_pid_file=''
  set -e
  build_status="$status"
  (( status == 0 )) || fail 'isolated Docker image build failed' || return 1
}

capture_image_id() {
  local ref="$1" value
  value="$(docker_call image inspect --format '{{.Id}}' "$ref")" || return 1
  valid_full_image_id "$value" || return 1
  printf '%s' "${value#sha256:}"
}

prepare_tags_and_archive() {
  local existing_id='' archive_tmp manifest_status=0
  image_id="$(capture_image_id "$release_tag")" || fail 'built release tag does not resolve to a full image ID' || return 1
  [[ "$image_id" != "$node_image_id" && "$image_id" != "$golang_image_id" &&
    "$image_id" != "$alpine_image_id" && "$image_id" != "$postgres_image_id" &&
    "$image_id" != "$buildkit_image_id" ]] || fail 'candidate image unexpectedly equals a base image' || return 1
  release_image_created='true'
  if existing_id="$(capture_image_id "$gate_tag" 2>/dev/null)"; then
    gate_tag_preexisting='true'
    fail "candidate gate tag already exists locally ($gate_tag -> $existing_id); refusing to overwrite it" || return 1
  fi
  docker_call info >/dev/null 2>&1 || fail 'cannot distinguish an absent candidate gate tag from a Docker daemon failure' || return 1
  docker_call tag "$release_tag" "$gate_tag" || fail 'cannot create the candidate gate tag' || return 1
  gate_tag_created='true'
  # Saving a single tag normally produces one RepoTags entry. Temporarily
  # remove the unique tag to make that invariant explicit on all Docker
  # versions, then restore it after the archive is complete.
  docker_call image rm "$release_tag" >/dev/null || fail 'cannot isolate the archive tag' || return 1
  archive_tmp="$stage_dir/candidate-image.tar.tmp"
  docker_call save --output "$archive_tmp" "$gate_tag" || fail 'docker save failed' || return 1
  [[ -f "$archive_tmp" && ! -L "$archive_tmp" ]] || fail 'docker save did not create a regular archive' || return 1
  archive_size="$(stat -c '%s' -- "$archive_tmp")" || return 1
  [[ "$archive_size" =~ ^[0-9]+$ && "$archive_size" -gt 0 && "$archive_size" -le "$max_archive_bytes" ]] ||
    fail 'saved Docker archive is outside the approved size range' || return 1
  archive_sha256="$(hash_file "$archive_tmp")" || return 1
  valid_sha256_hex "$archive_sha256" || fail 'saved Docker archive SHA256 is invalid' || return 1
  python3 - "$archive_tmp" "$gate_tag" "$image_id" <<'PY'
import json
import pathlib
import tarfile
import sys

archive, expected_tag, expected_id = sys.argv[1:]
with tarfile.open(archive, mode="r:") as bundle:
    members = bundle.getmembers()
    if not members or len(members) > 100000:
        raise SystemExit("invalid Docker archive member count")
    names = set()
    for member in members:
        path = pathlib.PurePosixPath(member.name)
        if member.name in names or path.is_absolute() or ".." in path.parts or "\\" in member.name:
            raise SystemExit("unsafe Docker archive member")
        names.add(member.name)
        if not (member.isdir() or member.isreg()):
            raise SystemExit("Docker archive contains a link or special member")
    manifest_member = bundle.getmember("manifest.json")
    stream = bundle.extractfile(manifest_member)
    manifest = json.load(stream)
    if not isinstance(manifest, list) or len(manifest) != 1:
        raise SystemExit("Docker archive must contain exactly one image")
    entry = manifest[0]
    if entry.get("RepoTags") != [expected_tag]:
        raise SystemExit("Docker archive must contain exactly the candidate gate tag")
    config = entry.get("Config")
    layers = entry.get("Layers")
    if not isinstance(config, str) or not isinstance(layers, list) or not layers:
        raise SystemExit("Docker archive manifest fields are invalid")
    config_id = pathlib.PurePosixPath(config).name.removesuffix(".json")
    if config_id != expected_id:
        raise SystemExit("Docker archive config does not match the built image ID")
    for referenced in [config, *layers]:
        if referenced not in names:
            raise SystemExit("Docker archive references a missing member")
PY
  mv -- "$archive_tmp" "$archive_stage"
  docker_call tag "$gate_tag" "$release_tag" || fail 'cannot restore unique release tag after archive save' || return 1
}

write_metadata() {
  local metadata_tmp checksums_tmp artifact artifact_path checksum_value
  metadata_tmp="$stage_dir/metadata.env.tmp"
  checksums_tmp="$stage_dir/SHA256SUMS.tmp"
  {
    printf 'BUILD_SCRIPT=%s\n' "$script_name"
    printf 'BUILD_SCRIPT_SHA256=%s\n' "$script_sha256"
    printf 'RUN_TOKEN=%s\n' "$run_token"
    printf 'APPROVED_COMMIT_SHA=%s\n' "$approved_sha"
    printf 'TREE_SHA=%s\n' "$tree_sha"
    printf 'COMMIT_DATE=%s\n' "$commit_date"
    printf 'RELEASE_TAG=%s\n' "$release_tag"
    printf 'CANDIDATE_GATE_TAG=%s\n' "$gate_tag"
    printf 'IMAGE_ID=%s\n' "$image_id"
    printf 'IMAGE_ID_REF=sha256:%s\n' "$image_id"
    printf 'IMAGE_ARCHIVE=candidate-image.tar\n'
    printf 'IMAGE_ARCHIVE_SHA256=%s\n' "$archive_sha256"
    printf 'IMAGE_ARCHIVE_SIZE=%s\n' "$archive_size"
    printf 'DOCKER_CONTEXT=%s\n' "$docker_context"
    printf 'DOCKER_ENDPOINT=%s\n' "$docker_endpoint"
    printf 'DOCKER_DAEMON_ID=%s\n' "$docker_daemon_id_start"
    printf 'DOCKER_DAEMON_NAME=%s\n' "$docker_daemon_name"
    printf 'DOCKER_SERVER_VERSION=%s\n' "$docker_server_version"
    printf 'DOCKER_ROOT_DIR=%s\n' "$docker_root_dir"
    printf 'BASE_IMAGES_FILE=base-images.txt\n'
    printf 'SUBNEXUS_CANDIDATE_NODE_IMAGE=%s\n' "$node_image_ref"
    printf 'SUBNEXUS_CANDIDATE_GOLANG_IMAGE=%s\n' "$golang_image_ref"
    printf 'SUBNEXUS_CANDIDATE_ALPINE_IMAGE=%s\n' "$alpine_image_ref"
    printf 'SUBNEXUS_CANDIDATE_POSTGRES_IMAGE=%s\n' "$postgres_image_ref"
    printf 'SUBNEXUS_CANDIDATE_BUILDKIT_IMAGE=%s\n' "$buildkit_image_ref"
    printf 'BUILD_NETWORK=%s\n' "$builder_network_name"
    printf 'BUILD_NETWORK_ID=%s\n' "$builder_network_id"
    printf 'BUILDER=%s\n' "$builder_name"
    printf 'BUILDER_ID=%s\n' "$builder_id"
    printf 'BUILD_LOG=build.log\n'
    printf 'BUILDER_INSPECT=builder.inspect\n'
    printf 'BASE_IMAGES=base-images.txt\n'
    printf 'BUILDER_CLEANED=true\n'
    printf 'LOCAL_DOCKER_ONLY=true\n'
    printf 'SERVER_CONNECTION=false\n'
  } >"$metadata_tmp"
  chmod 600 -- "$metadata_tmp"
  mv -- "$metadata_tmp" "$metadata_stage"
  : >"$checksums_tmp"
  for artifact in candidate-image.tar metadata.env base-images.txt build.log builder.inspect; do
    case "$artifact" in
      candidate-image.tar) artifact_path="$archive_stage" ;;
      metadata.env) artifact_path="$metadata_stage" ;;
      base-images.txt) artifact_path="$base_images_stage" ;;
      build.log) artifact_path="$build_log_stage" ;;
      builder.inspect) artifact_path="$builder_inspect_stage" ;;
      *) return 1 ;;
    esac
    [[ -f "$artifact_path" && ! -L "$artifact_path" ]] || {
      failure_reason="missing build evidence artifact: $artifact"
      return 1
    }
    checksum_value="$(hash_file "$artifact_path")" || return 1
    valid_sha256_hex "$checksum_value" || return 1
    printf '%s  %s\n' "$checksum_value" "$artifact" >>"$checksums_tmp"
  done
  chmod 600 -- "$checksums_tmp"
  mv -- "$checksums_tmp" "$checksums_stage"
}

cleanup_builder_and_network() {
  local observed_name observed_gate observed_token members status=0
  local builder_listing network_listing builder_count network_count
  local -a builder_ids=() network_ids=()

  if [[ "$builder_cleanup_done" != 'true' &&
    ( "$builder_create_attempted" == 'true' || "$builder_created" == 'true' ) ]]; then
    # Resolve an ID even when `buildx create` timed out before returning its
    # name. An empty result is safe only after a successful daemon query proves
    # that the exact generated builder/container name is absent.
    if [[ -z "$builder_id" ]]; then
      if builder_listing="$(docker_call ps --all --no-trunc \
        --filter "name=^/buildx_buildkit_${builder_name}0$" --format '{{.ID}}' 2>/dev/null)"; then
        mapfile -t builder_ids < <(printf '%s\n' "$builder_listing" | sed '/^[[:space:]]*$/d')
        builder_count="${#builder_ids[@]}"
        if [[ "$builder_count" -eq 1 && "${builder_ids[0]}" =~ ^[0-9a-f]{64}$ ]]; then
          builder_id="${builder_ids[0]}"
        elif [[ "$builder_count" -gt 1 ]]; then
          status=1
        fi
      else
        status=1
      fi
    fi
    if [[ "$status" -eq 0 && -n "$builder_id" ]]; then
      if observed_name="$(docker_call inspect --format '{{.Name}}' "$builder_id" 2>/dev/null)"; then
        [[ "$observed_name" == "/buildx_buildkit_${builder_name}0" ]] || status=1
        [[ "$status" -eq 0 ]] && validate_builder_container "$builder_id" || status=1
      else
        # The container may have exited and been removed by Buildx already;
        # confirm the daemon is responsive, then let buildx rm reconcile its
        # exact builder metadata.
        docker_call info >/dev/null 2>&1 || status=1
      fi
    fi
    if [[ "$status" -eq 0 ]]; then
      if docker_call buildx rm --force "$builder_name" >/dev/null 2>&1; then
        :
      else
        # A failed remove is acceptable only if both the exact builder and its
        # exact container are provably absent afterwards.
        if builder_listing="$(docker_call buildx ls 2>/dev/null)"; then
          printf '%s\n' "$builder_listing" | awk -v wanted="$builder_name" \
            '$1 == wanted || $1 == wanted "*" {found=1} END {exit found ? 0 : 1}' && status=1 || true
        else
          status=1
        fi
      fi
      if builder_listing="$(docker_call ps --all --no-trunc \
        --filter "name=^/buildx_buildkit_${builder_name}0$" --format '{{.ID}}' 2>/dev/null)"; then
        [[ -z "$(printf '%s' "$builder_listing" | tr -d '\r\n')" ]] || status=1
      else
        status=1
      fi
      if builder_listing="$(docker_call buildx ls 2>/dev/null)"; then
        printf '%s\n' "$builder_listing" | awk -v wanted="$builder_name" \
          '$1 == wanted || $1 == wanted "*" {found=1} END {exit found ? 0 : 1}' && status=1 || true
      else
        status=1
      fi
    fi
    [[ "$status" -eq 0 ]] && builder_cleanup_done='true'
  fi

  if [[ "$builder_network_cleanup_done" != 'true' &&
    ( "$builder_network_create_attempted" == 'true' || -n "$builder_network_id" ) ]]; then
    if [[ -z "$builder_network_id" ]]; then
      if network_listing="$(docker_call network ls --no-trunc \
        --filter "name=^${builder_network_name}$" --format '{{.ID}}' 2>/dev/null)"; then
        mapfile -t network_ids < <(printf '%s\n' "$network_listing" | sed '/^[[:space:]]*$/d')
        network_count="${#network_ids[@]}"
        if [[ "$network_count" -eq 1 && "${network_ids[0]}" =~ ^[0-9a-f]{64}$ ]]; then
          builder_network_id="${network_ids[0]}"
        elif [[ "$network_count" -gt 1 ]]; then
          status=1
        fi
      else
        status=1
      fi
    fi
    if [[ "$status" -eq 0 && -n "$builder_network_id" ]]; then
      observed_gate="$(docker_call network inspect --format '{{index .Labels "com.subnexus.isolated-build.gate"}}' "$builder_network_id" 2>/dev/null || true)"
      observed_token="$(docker_call network inspect --format '{{index .Labels "com.subnexus.isolated-build.token"}}' "$builder_network_id" 2>/dev/null || true)"
      observed_name="$(docker_call network inspect --format '{{.Name}}' "$builder_network_id" 2>/dev/null || true)"
      [[ "$observed_name" == "$builder_network_name" && "$observed_gate" == "$script_name" &&
        "$observed_token" == "$run_token" ]] || status=1
      members="$(docker_call network inspect --format '{{range $id, $container := .Containers}}{{println $id}}{{end}}' "$builder_network_id" 2>/dev/null || true)"
      [[ -z "$members" ]] || status=1
      if [[ "$status" -eq 0 ]]; then
        docker_call network rm "$builder_network_id" >/dev/null 2>&1 || {
          docker_call network inspect "$builder_network_id" >/dev/null 2>&1 && status=1 || true
        }
        docker_call network inspect "$builder_network_id" >/dev/null 2>&1 && status=1 || true
      fi
    elif [[ "$status" -eq 0 ]]; then
      # No exact network exists; the daemon query above was successful.
      :
    fi
    [[ "$status" -eq 0 ]] && builder_network_cleanup_done='true'
  fi

  [[ "$status" -eq 0 ]] || cleanup_failed='true'
  return "$status"
}

cleanup_release_tags() {
  local observed='' status=0
  if [[ "$gate_tag_created" == 'true' ]]; then
    observed="$(capture_image_id "$gate_tag" 2>/dev/null || true)"
    if [[ "$observed" == "$image_id" ]]; then
      docker_call image rm "$gate_tag" >/dev/null 2>&1 || status=1
    elif [[ -n "$observed" ]]; then
      status=1
    fi
  fi
  if [[ "$release_image_created" == 'true' ]]; then
    observed="$(capture_image_id "$release_tag" 2>/dev/null || true)"
    if [[ "$observed" == "$image_id" ]]; then
      docker_call image rm "$release_tag" >/dev/null 2>&1 || status=1
    elif [[ -n "$observed" ]]; then
      status=1
    fi
  fi
  [[ "$status" -eq 0 ]]
}

assert_no_unexpected_objects() {
  local current containers networks volumes custom
  containers="$(docker_call ps -aq --no-trunc | sort -u)" || return 1
  [[ -z "$containers" ]] || return 1
  networks="$(docker_call network ls --format '{{.Name}}' | sort -u)" || return 1
  custom="$(printf '%s\n' "$networks" | grep -Ev '^(bridge|host|none)$' || true)"
  [[ -z "$custom" ]] || return 1
  volumes="$(docker_call volume ls -q | sort -u)" || return 1
  [[ -z "$volumes" ]] || return 1
  [[ "$networks" == "$baseline_networks" ]]
}

on_signal() {
  interrupted='true'
  failure_reason='image build interrupted by signal'
  kill_build_group || true
  exit 130
}

on_error() {
  local status=$?
  [[ -n "$failure_reason" ]] || failure_reason="command failed with status $status"
  return "$status"
}

on_exit() {
  local initial_status=$?
  local final_status="$initial_status"
  trap - EXIT INT TERM HUP
  set +e
  if [[ "$cleanup_started" == 'false' ]]; then
    cleanup_started='true'
    kill_build_group
    cleanup_builder_and_network || final_status=1
    if [[ "$initial_status" -ne 0 || "$interrupted" == 'true' ]]; then
      cleanup_release_tags || final_status=1
    fi
    if [[ -n "$docker_daemon_id_start" && -n "$docker_context" ]]; then
      assert_daemon_unchanged exit_cleanup || final_status=1
    fi
    if [[ "$initial_status" -ne 0 || "$final_status" -ne 0 ]]; then
      safe_remove_stage "$stage_dir" || final_status=1
    fi
  fi
  if [[ "$initial_status" -eq 0 && "$final_status" -eq 0 && "$interrupted" == 'false' ]]; then
    if [[ -n "$stage_dir" && -d "$stage_dir" && -n "$final_dir" && ! -e "$final_dir" && ! -L "$final_dir" ]]; then
      mv -- "$stage_dir" "$final_dir" || final_status=1
      sync -f "$artifact_root" >/dev/null 2>&1 || final_status=1
    else
      final_status=1
    fi
  fi
  if [[ "$final_status" -eq 0 ]]; then
    printf 'ISOLATED_IMAGE_BUILD=passed\n'
    printf 'RELEASE_TAG=%s\n' "$release_tag"
    printf 'CANDIDATE_GATE_TAG=%s\n' "$gate_tag"
    printf 'IMAGE_ID=%s\n' "$image_id"
    printf 'IMAGE_ARCHIVE=%s\n' "$archive_final"
    printf 'IMAGE_ARCHIVE_SHA256=%s\n' "$archive_sha256"
    printf 'TREE_SHA=%s\n' "$tree_sha"
    printf 'METADATA=%s\n' "$metadata_final"
  else
    printf 'ISOLATED_IMAGE_BUILD=failed\n' >&2
    [[ -n "$failure_reason" ]] && printf 'FAILURE_REASON=%s\n' "$failure_reason" >&2
  fi
  exit "$final_status"
}

main() {
  local value context_info image_ref
  [[ "$#" -eq 2 || "$#" -eq 3 ]] || { usage; return 2; }
  source_root="$1"
  approved_sha="$2"
  artifact_root="${3:-${SUBNEXUS_BUILD_ARTIFACT_ROOT:-}}"
  valid_sha40 "$approved_sha" || fail 'approved commit must be a lowercase 40-character SHA' || return 1
  [[ -n "${SUBNEXUS_LOCAL_DOCKER_CONFIRM:-}" && "${SUBNEXUS_LOCAL_DOCKER_CONFIRM}" == 'I_UNDERSTAND_LOCAL_ONLY' ]] ||
    fail 'set SUBNEXUS_LOCAL_DOCKER_CONFIRM=I_UNDERSTAND_LOCAL_ONLY to confirm a local-only build' || return 1
  docker_context="${SUBNEXUS_BUILD_DOCKER_CONTEXT:-}"
  [[ -n "$docker_context" ]] || fail 'SUBNEXUS_BUILD_DOCKER_CONTEXT is required; default context is forbidden' || return 1
  valid_context_name "$docker_context" || fail 'Docker context name is unsafe or production-like' || return 1
  for value in DOCKER_HOST DOCKER_CONTEXT DOCKER_CONFIG DOCKER_TLS_VERIFY DOCKER_CERT_PATH DOCKER_API_VERSION; do
    [[ -z "${!value:-}" ]] || fail "$value must be unset for an explicit local build context" || return 1
  done
  node_image_ref="${SUBNEXUS_CANDIDATE_NODE_IMAGE:-}"
  golang_image_ref="${SUBNEXUS_CANDIDATE_GOLANG_IMAGE:-}"
  alpine_image_ref="${SUBNEXUS_CANDIDATE_ALPINE_IMAGE:-}"
  postgres_image_ref="${SUBNEXUS_CANDIDATE_POSTGRES_IMAGE:-}"
  buildkit_image_ref="${SUBNEXUS_CANDIDATE_BUILDKIT_IMAGE:-}"
  for image_ref in "$node_image_ref" "$golang_image_ref" "$alpine_image_ref" "$postgres_image_ref" "$buildkit_image_ref"; do
    valid_repository_digest_ref "$image_ref" || fail 'all five build images must be repository@sha256 digest references' || return 1
  done
  valid_positive_integer "${SUBNEXUS_BUILD_DOCKER_RPC_TIMEOUT_SECONDS:-120}" || fail 'SUBNEXUS_BUILD_DOCKER_RPC_TIMEOUT_SECONDS must be positive' || return 1
  docker_rpc_timeout_seconds="${SUBNEXUS_BUILD_DOCKER_RPC_TIMEOUT_SECONDS:-120}"
  (( 10#$docker_rpc_timeout_seconds <= 300 )) || fail 'Docker RPC timeout exceeds 300 seconds' || return 1
  valid_positive_integer "${SUBNEXUS_BUILD_TIMEOUT_SECONDS:-$default_timeout_seconds}" || fail 'SUBNEXUS_BUILD_TIMEOUT_SECONDS must be positive' || return 1
  build_timeout_seconds="${SUBNEXUS_BUILD_TIMEOUT_SECONDS:-$default_timeout_seconds}"
  (( 10#$build_timeout_seconds <= max_timeout_seconds )) || fail 'build timeout exceeds the safety limit' || return 1
  docker_binary="$(command -v docker)" || fail 'docker CLI is required' || return 1
  [[ "$docker_binary" == /* && -f "$docker_binary" ]] || fail 'docker CLI must be an absolute regular executable' || return 1
  for value in git timeout realpath stat sha256sum awk sed grep tr sort date mkdir chmod flock sync mv rm sleep cat find python3 tar setsid id; do
    command -v "$value" >/dev/null 2>&1 || fail "missing command: $value" || return 1
  done
  script_source_path="$(realpath -e -- "${BASH_SOURCE[0]}")" || fail 'cannot resolve the image-build script path' || return 1
  [[ -f "$script_source_path" && ! -L "$script_source_path" ]] || fail 'image-build script must be a non-symlink file' || return 1
  owner_is_allowed "$script_source_path" || fail 'image-build script owner is unsafe' || return 1
  mode_is_safe "$script_source_path" || fail 'image-build script is writable by group/other' || return 1
  script_sha256="$(hash_file "$script_source_path")" || fail 'cannot hash the image-build script' || return 1
  valid_sha256_hex "$script_sha256" || fail 'image-build script SHA256 is invalid' || return 1
  validate_source_tree
  if [[ -z "$artifact_root" ]]; then
    artifact_root="$(dirname -- "$source_root")/.subnexus-candidate-artifacts"
  fi
  artifact_root="$(realpath -m -- "$artifact_root")" || fail 'cannot normalize artifact root' || return 1
  case "$artifact_root" in
    /|/tmp|/var|/home|/root|/srv) fail 'artifact root is too broad' || return 1 ;;
    "$source_root"|"$source_root"/*) fail 'artifact root must be outside the source worktree' || return 1 ;;
  esac
  [[ -d "${artifact_root%/*}" ]] || fail 'artifact root parent directory must already exist' || return 1
  ensure_secure_directory "$artifact_root"
  exec 9>>"$artifact_root/.build.lock"
  chmod 600 -- "$artifact_root/.build.lock"
  flock -n 9 || fail 'another isolated image build is already running' || return 1
  timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
  run_uuid="$(cat /proc/sys/kernel/random/uuid 2>/dev/null)" || fail 'cannot obtain kernel UUID' || return 1
  [[ "$run_uuid" =~ ^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$ ]] || fail 'kernel UUID is invalid' || return 1
  run_token="${timestamp}-${run_uuid}"
  release_tag="subnexus-release:${approved_sha}-${timestamp}-${run_uuid:0:8}"
  gate_tag="subnexus-release:${approved_sha}"
  valid_tag "${release_tag#*:}" || fail 'generated release tag is invalid' || return 1
  stage_dir="$artifact_root/.$run_token.stage"
  final_dir="$artifact_root/$run_token"
  context_root="$stage_dir/context"
  archive_stage="$stage_dir/candidate-image.tar"
  archive_final="$final_dir/candidate-image.tar"
  metadata_stage="$stage_dir/metadata.env"
  metadata_final="$final_dir/metadata.env"
  checksums_stage="$stage_dir/SHA256SUMS"
  checksums_final="$final_dir/SHA256SUMS"
  base_images_stage="$stage_dir/base-images.txt"
  build_log_stage="$stage_dir/build.log"
  builder_inspect_stage="$stage_dir/builder.inspect"
  [[ ! -e "$stage_dir" && ! -L "$stage_dir" && ! -e "$final_dir" && ! -L "$final_dir" ]] ||
    fail 'generated artifact path already exists' || return 1
  mkdir -- "$stage_dir"; chmod 700 -- "$stage_dir"
  : >"$base_images_stage"; chmod 600 -- "$base_images_stage"
  trap on_signal INT TERM HUP
  trap on_error ERR
  trap on_exit EXIT
  context_info="$(docker_call context inspect "$docker_context" --format '{{(index .Endpoints "docker").Host}}')" || fail 'cannot inspect the selected Docker context' || return 1
  docker_endpoint="$context_info"
  validate_endpoint
  docker_context="${docker_context}" # keep the explicit context visible in evidence
  docker_daemon_id_start="$(docker_call info --format '{{.ID}}')" || fail 'cannot inspect selected Docker daemon' || return 1
  docker_daemon_name="$(docker_call info --format '{{.Name}}')" || return 1
  docker_server_version="$(docker_call info --format '{{.ServerVersion}}')" || return 1
  docker_root_dir="$(docker_call info --format '{{.DockerRootDir}}')" || return 1
  docker_swarm_state="$(docker_call info --format '{{.Swarm.LocalNodeState}}')" || return 1
  docker_security_options="$(docker_call info --format '{{json .SecurityOptions}}')" || return 1
  [[ "$docker_daemon_id_start" =~ ^[A-Za-z0-9:_-]+$ ]] || fail 'Docker daemon ID is invalid' || return 1
  [[ ! "$docker_daemon_name" =~ (^|[^0-9])([0-9]{1,3}\.){3}[0-9]{1,3}([^0-9]|$) &&
    "${docker_daemon_name,,}" != *prod* && "${docker_daemon_name,,}" != *production* ]] ||
    fail 'Docker daemon name looks like a production daemon' || return 1
  [[ "$docker_swarm_state" == 'inactive' || "$docker_swarm_state" == 'pending' ]] || fail 'active Docker Swarm is not allowed for local build' || return 1
  [[ "$docker_security_options" == *'name=seccomp'* ]] || fail 'Docker daemon must provide seccomp isolation' || return 1
  [[ "$docker_root_dir" == /* && ! -L "$docker_root_dir" && -d "$docker_root_dir" ]] || fail 'Docker root directory is unsafe' || return 1
  owner_is_allowed "$docker_root_dir" || fail 'Docker root directory owner is unsafe' || return 1
  mode_is_safe "$docker_root_dir" || fail 'Docker root directory is writable by group/other' || return 1
  docker_socket_fingerprint_start="$(docker_socket_fingerprint)" || fail 'cannot fingerprint Docker endpoint' || return 1
  capture_object_lists || fail 'cannot snapshot local Docker objects' || return 1
  assert_empty_build_daemon
  node_image_id="$(validate_base_image node "$node_image_ref")" || return 1
  golang_image_id="$(validate_base_image golang "$golang_image_ref")" || return 1
  alpine_image_id="$(validate_base_image alpine "$alpine_image_ref")" || return 1
  postgres_image_id="$(validate_base_image postgres "$postgres_image_ref")" || return 1
  buildkit_image_id="$(validate_base_image buildkit "$buildkit_image_ref")" || return 1
  extract_fixed_context
  validate_context_tree
  validate_dockerfile_pin_contract
  builder_network_name="subnexus-build-net-${timestamp,,}-${run_uuid:0:8}"
  builder_name="subnexus-build-${timestamp,,}-${run_uuid:0:8}"
  assert_exact_absent image "$release_tag" || fail 'generated release tag is already in use' || return 1
  assert_exact_absent image "$gate_tag" || fail 'candidate gate tag already exists on the isolated daemon' || return 1
  create_builder_network
  create_builder
  run_build
  assert_script_unchanged || fail 'image-build script changed while the image was building' || return 1
  assert_daemon_unchanged after_build
  prepare_tags_and_archive
  assert_script_unchanged || fail 'image-build script changed before archive publication' || return 1
  cleanup_builder_and_network || fail 'isolated BuildKit cleanup failed' || return 1
  assert_no_unexpected_objects || fail 'local Docker daemon was left with unexpected build objects' || return 1
  write_metadata
  chmod 600 -- "$archive_stage" "$metadata_stage" "$checksums_stage"
  sync -f "$archive_stage"; sync -f "$metadata_stage"; sync -f "$checksums_stage"
  # on_exit performs the final atomic stage rename and prints the handoff.
  return 0
}

main "$@"
