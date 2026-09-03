#!/usr/bin/env bash
set -Eeuo pipefail

# Static and fixture tests for the Docker candidate gate.  The subject is
# intentionally never sourced: sourcing it would run its production-host
# entrypoint.  Every executable fixture below replaces docker_rpc (and, where
# needed, stat/timeout/python3) with a local deterministic function.

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
subject="$script_dir/subnexus-docker-candidate-check.sh"

fail() {
  printf 'TEST ERROR: %s\n' "$*" >&2
  exit 1
}

assert_eq() {
  local expected="$1"
  local actual="$2"
  local label="$3"
  [[ "$actual" == "$expected" ]] || fail "$label: expected '$expected', got '$actual'"
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

line_of() {
  local needle="$1"
  grep -n -F -- "$needle" "$subject" | head -n 1 | cut -d: -f1
}

assert_before() {
  local first="$1"
  local second="$2"
  local first_line second_line
  first_line="$(line_of "$first")"
  second_line="$(line_of "$second")"
  [[ "$first_line" =~ ^[0-9]+$ && "$second_line" =~ ^[0-9]+$ ]] ||
    fail "ordering markers are missing: '$first' / '$second'"
  (( first_line < second_line )) || fail "'$first' must precede '$second'"
}

extract_function() {
  local function_name="$1"
  awk -v wanted="$function_name" '
    $0 == wanted "() {" { in_function = 1 }
    in_function { print }
    in_function && $0 == "}" { exit }
  ' "$subject"
}

[[ -f "$subject" ]] || fail 'candidate gate script is missing'
bash -n "$subject" || fail 'candidate gate script has a shell syntax error'

# The gate is root-only, local-daemon-only, and receives an already-approved
# immutable artifact.  These checks cover argument count, commit/image/archive
# digests, production references, and the no-build/no-remote boundary.
assert_contains '[[ "$EUID" -eq 0 ]] || fail '\''run this gate as root'\'''
assert_contains '[[ "$#" -eq 8 || "$#" -eq 9 ]]'
assert_contains 'usage: subnexus-docker-candidate-check.sh SOURCE_ROOT APPROVED_COMMIT_SHA EXPECTED_IMAGE_ID IMAGE_ARCHIVE IMAGE_ARCHIVE_SHA256 PRODUCTION_APP PRODUCTION_POSTGRES PRODUCTION_REDIS [EVIDENCE_ROOT]'
assert_contains '[[ "$approved_sha" =~ ^[0-9a-f]{40}$ ]]'
assert_contains '[[ "$expected_candidate_image_id" =~ ^[0-9a-f]{64}$ ]]'
assert_contains '[[ "$candidate_archive_sha256" =~ ^[0-9a-f]{64}$ ]]'
assert_contains 'valid_container_ref() {'
assert_contains 'valid_immutable_image_ref() {'
assert_contains 'valid_full_id() {'
assert_contains 'every candidate/base image must be pinned by a repository digest or full image ID'
assert_contains 'SUBNEXUS_CANDIDATE_POSTGRES_IMAGE is required'
assert_contains 'SUBNEXUS_CANDIDATE_REDIS_IMAGE is required'

# Shell/environment hardening and command provenance.
assert_contains 'case "$-" in'
assert_contains 'set +x'
assert_contains 'umask 077'
assert_contains 'unset BASH_ENV ENV CDPATH GLOBIGNORE'
assert_contains "export PATH='/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin'"
assert_contains 'for docker_override in DOCKER_HOST DOCKER_CONTEXT DOCKER_CONFIG DOCKER_TLS_VERIFY DOCKER_CERT_PATH DOCKER_API_VERSION'
assert_contains 'docker_rpc context show'
assert_contains 'docker_rpc context inspect'
assert_contains 'docker_endpoint'
assert_contains 'docker_socket_fingerprint_start'
assert_contains 'docker_daemon_identity_start'
assert_contains 'Docker daemon must provide seccomp isolation'
assert_contains 'validate_executable docker'
assert_contains 'validate_executable git'
assert_contains 'validate_executable timeout'
assert_contains 'timeout --foreground --kill-after=10s 120s'
assert_not_contains 'docker build'
assert_not_contains 'docker pull'
assert_not_contains 'docker push'
assert_not_contains 'ssh '
assert_not_contains 'scp '
assert_not_contains 'docker system prune'
assert_not_contains 'docker container prune'
assert_not_contains 'docker network prune'
assert_not_contains 'docker volume prune'
assert_not_contains 'docker image rm'
assert_not_contains 'docker rmi'
assert_not_contains '--network host'
assert_not_contains '--network container:'
assert_not_contains 'docker stop "$production'
assert_not_contains 'docker restart "$production'
assert_not_contains 'docker rm --force --volumes "$production'

# Production identity is captured before any candidate object is created and
# checked around every Docker operation, at shutdown, and before evidence is
# published.  It includes container, socket, and daemon identity.
assert_contains 'production_id[app]'
assert_contains 'production_id[postgres]'
assert_contains 'production_id[redis]'
assert_contains 'production_identity[$role]'
assert_contains 'production_summary[$role]'
assert_contains 'production_mount_sources[$role]'
assert_contains '{{.State.Running}}'
assert_contains 'production references must resolve to three distinct containers'
assert_contains 'capture_container_identity() {'
assert_contains 'capture_container_mount_sources() {'
assert_contains 'assert_production_unchanged() {'
assert_contains 'assert_production_unchanged '\''initial_snapshot'\'''
assert_contains 'assert_production_unchanged exit_cleanup'
assert_contains 'assert_production_unchanged '\''before_evidence_publish'\'''
assert_contains 'docker_socket_fingerprint_start'
assert_contains 'socket_fingerprint'
assert_contains 'daemon_identity'
assert_contains '[[ -S "$docker_socket_path" && ! -L "$docker_socket_path" ]]'
assert_contains 'assert_path_outside_production_mounts() {'
assert_contains 'controlled path contains a symbolic-link component'
assert_contains 'controlled path overlaps production'
assert_contains 'realpath -e -P -- "$path"'
assert_contains '"$physical_path" == "$canonical_source"/*'
assert_contains '"$canonical_source" == "$physical_path"/*'
assert_contains 'assert_path_outside_production_mounts "$source_root"'
assert_contains 'ensure_secure_directory "$evidence_root"'
assert_contains "git_submodules=\"\$(git -C \"\$source_root\" submodule status --recursive 2>/dev/null)\" || fail 'cannot inspect Git submodules'"

# Archive safety: canonical root-only path, ownership/link/mode/hash checks,
# bounded members, no traversal/links/special files, one manifest image, exact
# tag/config ID, and all manifest references present.
assert_contains 'validate_candidate_archive_manifest() {'
assert_contains 'path.is_absolute()'
assert_contains '".." in path.parts'
# Keep this assertion independent of the shell/Python escaping used for the
# literal backslash in the parser source.
assert_contains 'member.name in names'
assert_contains 'member.isdir() or member.isreg()'
assert_contains 'member.size > 12 * 1024**3'
assert_contains 'total > 12 * 1024**3'
assert_contains 'Docker archive must contain exactly one image'
assert_contains 'entry.get("RepoTags") != [expected_tag]'
assert_contains 'config.removeprefix("blobs/sha256/").removesuffix(".json")'
assert_contains 'config_id != expected_id'
assert_contains 'manifest references a missing member'
assert_contains 'candidate image archive must be below the approved root-only artifact directory'
assert_contains 'candidate_archive_lexical_path="$(realpath -m -s -- "$candidate_archive_path")"'
assert_contains 'candidate image archive path must not contain symbolic links'
assert_contains 'candidate image archive must be root-owned'
assert_contains 'candidate image archive must have exactly one hard link'
assert_contains 'candidate_archive_fingerprint'
assert_contains 'candidate image archive SHA256 does not match the approved value'
assert_contains 'assert_candidate_archive_unchanged'

# Candidate runtime must be private and bounded: internal bridge network,
# exactly one network, no port bindings, read-only roots, explicit tmpfs,
# dropped capabilities, no-new-privileges, and CPU/memory/PID ceilings.
assert_contains 'network create --driver bridge --internal --ipv6=false'
assert_contains 'assert_network_not_production'
assert_contains 'validate_network_labels() {'
assert_contains 'validate_network_members() {'
assert_contains '--network "$network_name"'
assert_contains '--network-alias postgres-candidate'
assert_contains '--network-alias redis-candidate'
assert_contains '--network-alias app-candidate'
assert_contains '--read-only'
assert_contains '--cap-drop ALL'
assert_contains '--security-opt no-new-privileges:true'
assert_contains '--ipc private'
assert_contains '--restart no'
assert_contains '--log-driver none'
assert_contains '--memory 768m --memory-swap 768m --cpus 1 --pids-limit 256'
assert_contains '--memory 256m --memory-swap 256m --cpus 0.5 --pids-limit 128'
assert_contains '{{json .HostConfig.PortBindings}}'
assert_contains '{{.HostConfig.PublishAllPorts}}'
assert_contains 'json_is_empty_list_or_null "$port_bindings"'
assert_contains 'network_count'
# Docker's Go-template printf must receive a single \n escape.  Two
# backslashes would be decoded to a literal "\n" and make the exact network
# identity check fail on every real container.
assert_contains 'printf "%s|%s\n" $name $network.NetworkID'
assert_not_contains 'printf "%s|%s\\n" $name $network.NetworkID'
assert_contains 'validate_candidate_tmpfs() {'
assert_contains '[ADMIN_EMAIL]=gate-admin@example.invalid'
assert_contains "'/tmp:rw,nosuid,nodev,noexec,size=64m'"
assert_contains "'/tmp:rw,nosuid,nodev,noexec,size=32m'"
assert_contains "'/run/postgresql:rw,nosuid,nodev,noexec,size=16m'"
assert_contains 'runtime_cap_add_matches() {'
assert_contains 'runtime_cap_drop_matches() {'
assert_contains 'runtime_security_opt_matches() {'
assert_contains 'validate_pid1_security() {'
assert_contains 'NoNewPrivs:'
assert_contains 'CapEff:'
assert_contains '"$uid_value" =~ ^[1-9][0-9]*$'
assert_contains '"$cap_eff_value" =~ ^0+$'
assert_contains 'validate_pid1_security "$postgres_id"'
assert_contains 'validate_pid1_security "$redis_id"'
assert_contains 'validate_pid1_security "$app_id" 1000'
assert_contains 'for role in postgres redis app'
assert_contains 'candidate_health_status "$role" "$container_id"'
assert_contains 'candidate PostgreSQL must run as non-root'
assert_contains 'candidate Redis must run as non-root'

# All migrated switches are fail-closed in the disposable database and in the
# public API smoke check.  The explicitly excluded legacy features are not
# silently reintroduced by the candidate gate.
for key in \
  registration_ip_cooldown_enabled \
  subnexus_activity_center_enabled \
  subnexus_checkin_enabled \
  subnexus_leaderboard_enabled \
  subnexus_marquee_enabled \
  subnexus_invite_activities_enabled \
  subnexus_invite_rewards_enabled \
  subnexus_first_recharge_enabled \
  battle_pass_enabled \
  subnexus_student_recharge_benefit_enabled \
  subnexus_invoice_enabled \
  channel_monitor_enabled \
  customer_support_enabled; do
  assert_contains "('$key', 'false')"
done
assert_contains "('subnexus_customer_support_enabled', 'false')"
assert_contains "('channel_monitor_mode', 'v1')"
assert_contains "'subnexus_invite_activities_config', '{\"enabled\":false,\"invite_lottery_enabled\":false,\"recharge_wheel_enabled\":false,\"invite_milestone_enabled\":false}'"
assert_contains 'verify_candidate_rollout_gates() {'
assert_contains '[[ "$count" == '\''14'\'' ]]'
assert_contains 'subnexus_customer_support_enabled'
assert_contains 'subnexus_invite_lottery_enabled'
assert_contains 'subnexus_recharge_wheel_enabled'
assert_contains 'subnexus_invite_milestone_enabled'

# Exact cleanup and retention contract.  Only this run's labelled IDs/names
# may be removed; the approved image is loaded/reused and deliberately kept.
assert_contains 'cleanup_resources() {'
assert_contains 'object_absent() {'
assert_contains 'inspect_status'
assert_contains "*'no such object'*"
assert_not_contains "*'not found'*) ;;"
assert_contains 'container ls --all --no-trunc --filter "id=$object_ref"'
assert_contains 'network ls --no-trunc --filter "id=$object_ref"'
assert_contains 'volume ls --filter "name=^${object_ref}$"'
assert_contains 'image ls --no-trunc --filter "reference=$object_ref"'
assert_contains 'observed_gate'
assert_contains 'observed_token'
assert_contains 'observed_role'
assert_contains 'container rm --force "$id"'
assert_not_contains 'container rm --force --volumes'
assert_contains 'network rm "$network_id_value"'
assert_contains 'volume rm "$volume_name_value"'
assert_contains 'assert_network_not_production "$network_id_value"'
assert_contains 'candidate image tag already loaded; exact approved ID will be reused'
assert_contains "candidate_image_retained='true'"
assert_contains "printf 'cutover_allowed=%s\\n'"
assert_contains "printf 'manual_review_required=%s\\n'"
assert_contains "printf 'manual_review=required\\n'"
assert_contains 'The approved release image is intentionally retained'
assert_not_contains 'image rm'
assert_not_contains 'image rmi'
assert_not_contains 'docker_rpc inspect "$id" >/dev/null 2>&1 && failed='''true''''
assert_contains 'trap on_signal INT TERM HUP'
assert_contains 'failure_reason='\''gate interrupted by signal'\''
assert_contains 'exit 130'
assert_contains 'trap on_exit EXIT'
assert_contains 'cleanup_resources >/dev/null 2>&1 || cleanup_failed='\''true'\'''
assert_contains 'evidence_publish_failed'
assert_contains 'mv -- "$evidence_stage_dir" "$evidence_final_dir"'
assert_contains 'sync -f "$evidence_root"'
assert_before 'cleanup_resources() {' 'create_candidate_container() {'
assert_before 'cleanup_resources >/dev/null 2>&1' 'write_evidence "$([[ "$final_status" -eq 0 ]]'
assert_before 'assert_production_unchanged '\''before_evidence_publish'\''' "gate_result='passed'"

# Docker inspect returns status 1 for both a genuine "not found" result and
# several daemon/CLI failures.  Exercise the helper with all four outcomes so
# cleanup can never treat a timeout or an unavailable daemon as proof that an
# object is gone.  The fixture also checks that the corroborating list query is
# exact for each object type.
object_absent_source="$(awk '
  /^object_absent\(\) \{/ { capture = 1 }
  capture && (/^main\(\) \{/ || /^# Cleanup is defined before/) { exit }
  capture { print }
' "$subject")"
[[ "$object_absent_source" == *'object_absent() {'* && "$object_absent_source" == *'return 2'* ]] ||
  fail 'object_absent helper source was not found'
(
  eval "$object_absent_source"
  container_id="$(printf 'a%.0s' {1..64})"
  network_id="$(printf 'b%.0s' {1..64})"
  fixture_root="$(mktemp -d "${TMPDIR:-/tmp}/subnexus-object-absent-test.XXXXXX")"
  trap 'rm -rf -- "$fixture_root"' EXIT
  list_log="$fixture_root/list.log"
  fixture=''

  docker_rpc() {
    local op="${1:-}" arg1="${2:-}" arg2="${3:-}" arg3="${4:-}" arg4="${5:-}" arg5="${6:-}" arg6="${7:-}" arg7="${8:-}"
    case "$fixture" in
      timeout)
        case "$op:$arg1" in
          inspect:*|network:inspect|volume:inspect)
            printf 'Error response from daemon: context deadline exceeded\n' >&2
            return 124
            ;;
        esac
        ;;
      cli-error)
        case "$op:$arg1" in
          inspect:*|network:inspect|volume:inspect)
            printf 'Error response from daemon: permission denied\n' >&2
            return 1
            ;;
        esac
        ;;
      cli-error-not-found)
        case "$op:$arg1" in
          inspect:*|network:inspect|volume:inspect)
            printf 'Error response from daemon: dependency not found\n' >&2
            return 1
            ;;
        esac
        ;;
      not-found-empty|present)
        case "$op:$arg1" in
          info:)
            return 0
            ;;
          inspect:*)
            printf 'Error: No such object: %s\n' "$arg1" >&2
            return 1
            ;;
          network:inspect)
            printf 'Error: No such network: %s\n' "$arg2" >&2
            return 1
            ;;
          volume:inspect)
            printf 'Error response from daemon: get %s: no such volume\n' "$arg2" >&2
            return 1
            ;;
          image:inspect)
            printf 'Error response from daemon: No such image: %s\n' "$arg2" >&2
            return 1
            ;;
          container:ls)
            [[ "$arg2" == '--all' && "$arg3" == '--no-trunc' && "$arg4" == '--filter' &&
              "$arg5" == 'name=^/candidate$' && "$arg6" == '--format' && "$arg7" == '{{.ID}}' ]] || return 99
            printf '%s\n' container >>"$list_log"
            [[ "$fixture" == present ]] && printf '%s\n' "$container_id"
            return 0
            ;;
          network:ls)
            [[ "$arg2" == '--no-trunc' && "$arg3" == '--filter' && "$arg4" == 'name=^candidate-net$' &&
              "$arg5" == '--format' && "$arg6" == '{{.ID}}' ]] || return 99
            printf '%s\n' network >>"$list_log"
            [[ "$fixture" == present ]] && printf '%s\n' "$network_id"
            return 0
            ;;
          volume:ls)
            [[ "$arg2" == '--filter' && "$arg3" == 'name=^candidate-volume$' &&
              "$arg4" == '--format' && "$arg5" == '{{.Name}}' ]] || return 99
            printf '%s\n' volume >>"$list_log"
            [[ "$fixture" == present ]] && printf 'candidate-volume\n'
            return 0
            ;;
          image:ls)
            [[ "$arg2" == '--no-trunc' && "$arg3" == '--filter' && "$arg4" == 'reference=candidate-image' &&
              "$arg5" == '--format' && "$arg6" == '{{.ID}}' ]] || return 99
            printf '%s\n' image >>"$list_log"
            [[ "$fixture" == present ]] && printf 'sha256:%s\n' "$container_id"
            return 0
            ;;
        esac
        ;;
    esac
    return 99
  }

  expect_status() {
    local expected="$1" kind="$2" ref="$3" actual
    if object_absent "$kind" "$ref"; then
      actual=0
    else
      actual=$?
    fi
    [[ "$actual" == "$expected" ]] ||
      fail "object_absent $kind/$ref: expected status $expected, got $actual"
  }

  fixture='not-found-empty'
  : >"$list_log"
  expect_status 0 container candidate
  expect_status 0 network candidate-net
  expect_status 0 volume candidate-volume
  expect_status 0 image candidate-image
  [[ "$(wc -l <"$list_log" | tr -d '[:space:]')" == 4 ]] || fail 'not-found fixture did not perform all exact list corroborations'

  fixture='timeout'
  : >"$list_log"
  expect_status 2 container candidate
  [[ ! -s "$list_log" ]] || fail 'timeout fixture performed a list query'

  fixture='cli-error'
  : >"$list_log"
  expect_status 2 network candidate-net
  [[ ! -s "$list_log" ]] || fail 'daemon-error fixture performed a list query'

  fixture='cli-error-not-found'
  : >"$list_log"
  expect_status 2 volume candidate-volume
  [[ ! -s "$list_log" ]] || fail 'misleading not-found fixture performed a list query'

  fixture='present'
  : >"$list_log"
  expect_status 1 container candidate
  expect_status 1 network candidate-net
  expect_status 1 volume candidate-volume
  expect_status 1 image candidate-image
  [[ "$(wc -l <"$list_log" | tr -d '[:space:]')" == 4 ]] || fail 'present fixture did not detect listed objects'
)

# Image references must compare Docker's canonical RepoDigests (which omit a
# tag) while preserving registry ports.  The reusable tag path must also
# re-check the approved archive before it is accepted.
assert_contains 'requested_repo="${requested_name%:*}"'
assert_contains 'repo_digests="$(docker_rpc image inspect --format '\''{{range .RepoDigests}}{{println .}}{{end}}'\'' "$requested")"'
assert_contains 'grep -Fqx -- "$candidate"'
assert_contains 'object_absent image "$candidate_image_tag"'
load_function_source="$(extract_function load_candidate_image)"
[[ "$load_function_source" == *'assert_candidate_archive_unchanged'* ]] || fail 'load path does not re-check archive integrity'
load_fixture_root="$(mktemp -d "${TMPDIR:-/tmp}/subnexus-load-test.XXXXXX")"
printf '%s\n' approved >"$load_fixture_root/approved.tar"
(
  eval "$load_function_source"
  candidate_image_tag='subnexus-release:fixture'
  candidate_archive_path="$load_fixture_root/approved.tar"
  image_load_log_file="$load_fixture_root/image-load.log"
  expected_candidate_image_id="$(printf 'f%.0s' {1..64})"
  candidate_image_preexisting='false'
  candidate_image_retained='false'
  archive_checks=0
  assert_candidate_archive_unchanged() { ((archive_checks += 1)); }
  capture_image_id() { printf '%s\n' "$expected_candidate_image_id"; }
  valid_full_id() { [[ "$1" =~ ^[0-9a-f]{64}$ ]]; }
  chmod 600 -- "$candidate_archive_path"
  load_candidate_image
  [[ "$candidate_image_preexisting" == 'true' ]] || fail 'preloaded candidate tag was not detected'
  [[ "$candidate_image_retained" == 'true' ]] || fail 'preloaded candidate image was not retained'
  [[ "$archive_checks" -eq 1 ]] || fail 'preloaded candidate tag skipped archive revalidation'
)
rm -rf -- "$load_fixture_root"

# A failed image ID inspection may proceed only after the strict image-absence
# helper proves the tag is genuinely missing.  Present and unknown states must
# terminate before docker image load can touch the tag.
(
  eval "$load_function_source"
  candidate_image_tag='subnexus-release:absence-fixture'
  candidate_archive_path='/fixture/approved.tar'
  image_load_log_file="$(mktemp "${TMPDIR:-/tmp}/subnexus-load-absence.XXXXXX")"
  trap 'rm -f -- "$image_load_log_file"' EXIT
  expected_candidate_image_id="$(printf 'f%.0s' {1..64})"
  candidate_image_preexisting='false'
  candidate_image_retained='false'
  archive_checks=0
  load_done='false'
  absence_mode='missing'
  assert_candidate_archive_unchanged() { archive_checks=$((archive_checks + 1)); }
  capture_image_id() {
    if [[ "$load_done" == 'true' ]]; then
      printf '%s\n' "$expected_candidate_image_id"
    else
      return 1
    fi
  }
  object_absent() {
    [[ "$1" == image && "$2" == "$candidate_image_tag" ]] || return 2
    case "$absence_mode" in
      missing) return 0 ;;
      present) return 1 ;;
      unknown) return 2 ;;
      *) return 2 ;;
    esac
  }
  docker_checked() {
    [[ "$1" == candidate_image_load && "$2" == image && "$3" == load && "$4" == '--input' ]] || return 1
    load_done='true'
    return 0
  }
  valid_full_id() { [[ "$1" =~ ^[0-9a-f]{64}$ ]]; }
  load_candidate_image
  [[ "$candidate_image_preexisting" == 'false' && "$candidate_image_retained" == 'true' ]] ||
    fail 'strictly absent candidate tag was not loaded'
  [[ "$archive_checks" -eq 2 ]] || fail 'candidate archive was not checked before and after loading'
)

for absence_mode in present unknown; do
  if (
    eval "$load_function_source"
    candidate_image_tag='subnexus-release:absence-fixture'
    candidate_archive_path='/fixture/approved.tar'
    image_load_log_file="$(mktemp "${TMPDIR:-/tmp}/subnexus-load-failure.XXXXXX")"
    trap 'rm -f -- "$image_load_log_file"' EXIT
    expected_candidate_image_id="$(printf 'f%.0s' {1..64})"
    candidate_image_preexisting='false'
    candidate_image_retained='false'
    assert_candidate_archive_unchanged() { :; }
    capture_image_id() { return 1; }
    object_absent() {
      if [[ "$absence_mode" == present ]]; then
        return 1
      fi
      return 2
    }
    docker_checked() { fail 'image load was attempted after a non-absent result'; }
    valid_full_id() { return 0; }
    load_candidate_image
  ); then
    fail "candidate image load accepted object_absent=$absence_mode"
  fi
done

if grep -n $'\r' "$subject" >/dev/null; then
  fail 'candidate gate script must use LF line endings'
fi

# Exercise the pure Bash validators with positive and hostile values.
validator_source="$(extract_function valid_immutable_image_ref)"
validator_source+=$'\n'
validator_source+="$(extract_function valid_container_ref)"
validator_source+=$'\n'
validator_source+="$(extract_function valid_full_id)"
validator_source+=$'\n'
validator_source+="$(extract_function valid_positive_integer)"
[[ -n "$validator_source" ]] || fail 'reference validator functions not found'
(
  eval "$validator_source"
  digest="$(printf 'a%.0s' {1..64})"
  valid_immutable_image_ref "node@sha256:$digest"
  valid_immutable_image_ref "registry.example/node:24@sha256:$digest"
  valid_immutable_image_ref "registry.example:5000/node@sha256:$digest"
  valid_immutable_image_ref "registry.example:5000/node:24@sha256:$digest"
  valid_immutable_image_ref "sha256:$digest"
  if valid_immutable_image_ref 'node:24-alpine'; then fail 'mutable image tag accepted'; fi
  if valid_immutable_image_ref "node@sha256:${digest:0:63}"; then fail 'short image digest accepted'; fi
  if valid_immutable_image_ref "node@sha256:${digest^^}"; then fail 'uppercase image digest accepted'; fi
  if valid_immutable_image_ref "registry.example:abc/node@sha256:$digest"; then fail 'non-numeric registry port accepted'; fi
  if valid_immutable_image_ref "registry.example:123456/node@sha256:$digest"; then fail 'overlong registry port accepted'; fi
  if valid_immutable_image_ref "registry.example:/node@sha256:$digest"; then fail 'empty registry port accepted'; fi
  valid_container_ref sub2api-app
  valid_container_ref "$(printf 'a%.0s' {1..64})"
  if valid_container_ref 'bad/name'; then fail 'container reference with slash accepted'; fi
  if valid_container_ref 'bad name'; then fail 'container reference with whitespace accepted'; fi
  valid_full_id "$digest"
  if valid_full_id "${digest:0:63}"; then fail 'short full ID accepted'; fi
  valid_positive_integer 1
  if valid_positive_integer 0 || valid_positive_integer -1 || valid_positive_integer 01x; then
    fail 'invalid positive integer accepted'
  fi
)

# Repository-digest fixture: Docker records RepoDigests without a tag, while
# registry ports remain part of the repository name. The resolver must match
# the canonical form exactly and accept immutable image IDs.
resolve_source="$(extract_function resolve_image_ref)"
[[ -n "$resolve_source" ]] || fail 'image reference resolver function not found'
(
  eval "$resolve_source"
  digest="$(printf 'd%.0s' {1..64})"
  image_id="$(printf 'e%.0s' {1..64})"
  capture_image_id() {
    case "$1" in
      *"@sha256:$digest"|"sha256:$image_id") printf '%s\n' "$image_id" ;;
      *) return 1 ;;
    esac
  }
  docker_rpc() {
    [[ "$1" == image && "$2" == inspect && "$3" == --format ]] || return 1
    case "$4" in
      '{{range .RepoDigests}}{{println .}}{{end}}')
        printf '%s\n' \
          "postgres@sha256:$digest" \
          "registry.example:5000/postgres@sha256:$digest"
        ;;
      *) return 1 ;;
    esac
  }
  fail() { return 1; }
  [[ "$(resolve_image_ref "postgres:18-alpine@sha256:$digest")" == "postgres:18-alpine@sha256:$digest" ]] ||
    fail 'tagged repository digest did not resolve'
  [[ "$(resolve_image_ref "registry.example:5000/postgres:18@sha256:$digest")" == "registry.example:5000/postgres:18@sha256:$digest" ]] ||
    fail 'registry-port repository digest did not resolve'
  [[ "$(resolve_image_ref "sha256:$image_id")" == "sha256:$image_id" ]] ||
    fail 'full image ID did not resolve'
)

# Capability and tmpfs validators are exercised with fixture JSON/arrays.  A
# local python3 shim keeps this test usable on Git Bash installations where the
# python3 launcher is a Windows exit-code shim; Linux uses the real interpreter.
cap_source="$(extract_function runtime_cap_add_matches)"
tmpfs_source="$(extract_function validate_candidate_tmpfs)"
empty_source="$(extract_function json_is_empty_list_or_null)"
[[ -n "$cap_source" && -n "$tmpfs_source" && -n "$empty_source" ]] || fail 'runtime validator functions not found'
(
  eval "$cap_source"
  cap_drop_source="$(extract_function runtime_cap_drop_matches)"
  security_source="$(extract_function runtime_security_opt_matches)"
  eval "$cap_drop_source"
  eval "$security_source"
  runtime_cap_add_matches app '["CHOWN","SETGID","SETUID"]'
  runtime_cap_add_matches postgres '["CHOWN","DAC_OVERRIDE","FOWNER","SETGID","SETUID"]'
  if runtime_cap_add_matches app '["CHOWN","SETGID","SETUID","NET_ADMIN"]'; then fail 'extra capability accepted'; fi
  if runtime_cap_add_matches app '["CHOWN","CHOWN","SETUID"]'; then fail 'duplicate capability accepted'; fi
  if runtime_cap_add_matches app '[]'; then fail 'empty capability set accepted'; fi
  if runtime_cap_add_matches unknown '["CHOWN"]'; then fail 'unknown capability role accepted'; fi
  runtime_cap_drop_matches '["ALL"]'
  runtime_cap_drop_matches '["all"]'
  if runtime_cap_drop_matches '[]' || runtime_cap_drop_matches '["ALL","NET_ADMIN"]'; then
    fail 'capability drop allowlist is not exact'
  fi
  runtime_security_opt_matches '["no-new-privileges:true"]'
  if runtime_security_opt_matches '[]' || runtime_security_opt_matches '["no-new-privileges:true","seccomp=unconfined"]'; then
    fail 'security option allowlist is not exact'
  fi

  eval "$empty_source"
  json_is_empty_list_or_null ''
  json_is_empty_list_or_null 'null'
  json_is_empty_list_or_null '[]'
  json_is_empty_list_or_null '{}'
  if json_is_empty_list_or_null '["published"]'; then fail 'non-empty JSON list accepted as empty'; fi
  if json_is_empty_list_or_null '{"8080/tcp":[{}]}'; then fail 'port binding JSON accepted as empty'; fi

  python3() {
    if command -v python >/dev/null 2>&1; then
      command python "$@"
    else
      command python3 "$@"
    fi
  }
  eval "$tmpfs_source"
  validate_candidate_tmpfs app '{"/tmp":"rw,nosuid,nodev,noexec,size=64m"}'
  validate_candidate_tmpfs redis '{"/tmp":"rw,nosuid,nodev,noexec,size=32m"}'
  validate_candidate_tmpfs postgres '{"/tmp":"rw,nosuid,nodev,noexec,size=64m","/run/postgresql":"rw,nosuid,nodev,noexec,size=16m"}'
  if validate_candidate_tmpfs app '{"/tmp":"rw,nosuid,nodev,noexec,size=65m"}'; then fail 'oversized app tmpfs accepted'; fi
  if validate_candidate_tmpfs app '{"/tmp":"rw,nosuid,nodev,noexec,size=64m","/extra":"rw"}'; then fail 'unexpected tmpfs mount accepted'; fi
  if validate_candidate_tmpfs postgres '{"/tmp":"rw,nosuid,nodev,size=64m","/run/postgresql":"rw,nosuid,nodev,noexec,size=16m"}'; then fail 'exec-enabled tmpfs accepted'; fi
)

# PID1 security fixture: the application keeps its expected UID 1000 while
# PostgreSQL/Redis are allowed to use their image-specific non-root UID.
pid_status_source="$(extract_function capture_pid1_status)"
pid_validator_source="$(extract_function validate_pid1_security)"
[[ -n "$pid_status_source" && -n "$pid_validator_source" ]] || fail 'PID1 validator functions not found'
(
  eval "$pid_status_source"
  eval "$pid_validator_source"
  fixture='app'
  capture_pid1_status() {
    case "$fixture" in
      app) printf 'Uid:\t1000\t1000\t1000\t1000\nNoNewPrivs:\t1\nCapEff:\t0000000000000000\n' ;;
      postgres) printf 'Uid:\t999\t999\t999\t999\nNoNewPrivs:\t1\nCapEff:\t0000000000000000\n' ;;
      root) printf 'Uid:\t0\t0\t0\t0\nNoNewPrivs:\t1\nCapEff:\t0000000000000000\n' ;;
      no-nnp) printf 'Uid:\t999\t999\t999\t999\nNoNewPrivs:\t0\nCapEff:\t0000000000000000\n' ;;
      cap) printf 'Uid:\t999\t999\t999\t999\nNoNewPrivs:\t1\nCapEff:\t0000000000000001\n' ;;
      *) return 1 ;;
    esac
  }
  validate_pid1_security app 1000
  fixture='postgres'
  validate_pid1_security postgres
  fixture='root'
  if validate_pid1_security postgres; then fail 'root PID1 accepted'; fi
  fixture='no-nnp'
  if validate_pid1_security postgres; then fail 'NoNewPrivs=0 accepted'; fi
  fixture='cap'
  if validate_pid1_security postgres; then fail 'effective capability accepted'; fi
  fixture='postgres'
  if validate_pid1_security postgres 1000; then fail 'unexpected UID accepted'; fi
)

# Network label fixture: no Docker daemon is called.  The validator must
# reject an external, attachable, IPv6-enabled, or mislabelled network.
network_source="$(extract_function validate_network_labels)"
[[ -n "$network_source" ]] || fail 'network label validator not found'
(
  eval "$network_source"
  gate_name='subnexus-docker-candidate-v1'
  fixture='valid'
  docker_rpc() {
    [[ "$1" == network && "$2" == inspect && "$3" == --format ]] || return 1
    case "$4" in
      '{{.Name}}') printf '%s\n' candidate-net ;;
      '{{.Driver}}') printf '%s\n' bridge ;;
      '{{.Internal}}') [[ "$fixture" == valid ]] && printf '%s\n' true || printf '%s\n' false ;;
      '{{.Attachable}}') [[ "$fixture" == valid ]] && printf '%s\n' false || printf '%s\n' true ;;
      '{{.EnableIPv6}}') printf '%s\n' false ;;
      '{{index .Labels "com.subnexus.candidate.gate"}}') printf '%s\n' "$gate_name" ;;
      '{{index .Labels "com.subnexus.candidate.token"}}') printf '%s\n' token-1 ;;
      *) return 1 ;;
    esac
  }
  validate_network_labels network-id candidate-net true token-1
  fixture='unsafe'
  if validate_network_labels network-id candidate-net true token-1; then fail 'unsafe network accepted'; fi
)

# Production identity fixture: stable snapshots pass; a changed container ID,
# identity, socket, or daemon fingerprint fails closed.
identity_source="$(extract_function assert_production_unchanged)"
[[ -n "$identity_source" ]] || fail 'production identity assertion not found'
(
  eval "$identity_source"
  declare -A production_ref=([app]=prod-app [postgres]=prod-postgres [redis]=prod-redis)
  declare -A production_id=()
  declare -A production_identity=()
  production_id[app]="$(printf 'a%.0s' {1..64})"
  production_id[postgres]="$(printf 'b%.0s' {1..64})"
  production_id[redis]="$(printf 'c%.0s' {1..64})"
  production_identity[app]="identity-${production_id[app]}"
  production_identity[postgres]="identity-${production_id[postgres]}"
  production_identity[redis]="identity-${production_id[redis]}"
  docker_socket_path='/fixture/docker.sock'
  fixture='stable'
  docker_rpc() {
    if [[ "$1" == inspect && "$2" == --format ]]; then
      case "$3" in
        '{{.Id}}')
          case "$4" in
            prod-app) [[ "$fixture" == stable ]] && printf '%s\n' "${production_id[app]}" || printf '%s\n' "$(printf 'd%.0s' {1..64})" ;;
            prod-postgres) printf '%s\n' "${production_id[postgres]}" ;;
            prod-redis) printf '%s\n' "${production_id[redis]}" ;;
            *) return 1 ;;
          esac
          ;;
        *) return 1 ;;
      esac
      return 0
    fi
    if [[ "$1" == info && "$2" == --format ]]; then
      printf '%s\n' 'daemon-id|docker|27|/var/lib/docker|[name=seccomp]'
      return 0
    fi
    return 1
  }
  capture_container_identity() {
    case "$1" in
      "${production_id[app]}") printf '%s\n' "${production_identity[app]}" ;;
      "${production_id[postgres]}") printf '%s\n' "${production_identity[postgres]}" ;;
      "${production_id[redis]}") printf '%s\n' "${production_identity[redis]}" ;;
      *) return 1 ;;
    esac
  }
  stat() {
    printf '%s\n' '1|2|socket|0|0|660'
  }
  docker_socket_fingerprint_start='1|2|socket|0|0|660'
  docker_daemon_identity_start='daemon-id|docker|27|/var/lib/docker|[name=seccomp]'
  assert_production_unchanged fixture-stable
  fixture='changed'
  if assert_production_unchanged fixture-changed >/dev/null 2>&1; then fail 'changed production identity accepted'; fi
)

# Build temporary Docker-save fixtures and exercise the subject's archive
# parser.  The fixture directory is created by mktemp and is the only path
# removed by this test; no host or Docker path is touched.
fixture_root="$(mktemp -d "${TMPDIR:-/tmp}/subnexus-candidate-test.XXXXXX")"
fixture_root="$(realpath -e -- "$fixture_root")"
trap 'rm -rf -- "$fixture_root"' EXIT
archive_source="$(extract_function validate_candidate_archive_manifest)"
[[ -n "$archive_source" ]] || fail 'archive validator function not found'

archive_id="$(printf 'a%.0s' {1..64})"
commit_sha="$(printf 'b%.0s' {1..40})"
archive_tag="subnexus-release:$commit_sha"

make_archive() {
  local name="$1"
  local manifest="$2"
  local root="$fixture_root/$name"
  mkdir -p -- "$root/blobs/sha256"
  printf '%s' "$manifest" >"$root/manifest.json"
  printf '%s\n' '{}' >"$root/blobs/sha256/$archive_id.json"
  : >"$root/layer.tar"
  tar -cf "$fixture_root/$name.tar" -C "$root" manifest.json "blobs/sha256/$archive_id.json" layer.tar
}

safe_manifest="[{\"Config\":\"blobs/sha256/$archive_id.json\",\"RepoTags\":[\"$archive_tag\"],\"Layers\":[\"layer.tar\"]}]"
make_archive safe "$safe_manifest"
make_archive wrong-tag "[{\"Config\":\"blobs/sha256/$archive_id.json\",\"RepoTags\":[\"subnexus-release:wrong\"],\"Layers\":[\"layer.tar\"]}]"
make_archive missing-layer "[{\"Config\":\"blobs/sha256/$archive_id.json\",\"RepoTags\":[\"$archive_tag\"],\"Layers\":[\"missing.tar\"]}]"
make_archive extra-image "[{\"Config\":\"blobs/sha256/$archive_id.json\",\"RepoTags\":[\"$archive_tag\"],\"Layers\":[\"layer.tar\"]},{\"Config\":\"blobs/sha256/$archive_id.json\",\"RepoTags\":[\"$archive_tag\"],\"Layers\":[\"layer.tar\"]}]"

wrong_config_id="$(printf 'c%.0s' {1..64})"
wrong_config_root="$fixture_root/wrong-config"
mkdir -p -- "$wrong_config_root/blobs/sha256"
printf '%s' "[{\"Config\":\"blobs/sha256/$wrong_config_id.json\",\"RepoTags\":[\"$archive_tag\"],\"Layers\":[\"layer.tar\"]}]" >"$wrong_config_root/manifest.json"
printf '%s\n' '{}' >"$wrong_config_root/blobs/sha256/$wrong_config_id.json"
: >"$wrong_config_root/layer.tar"
tar -cf "$fixture_root/wrong-config.tar" -C "$wrong_config_root" manifest.json "blobs/sha256/$wrong_config_id.json" layer.tar

python3_fixture() {
  if command -v python >/dev/null 2>&1; then
    command python "$@"
  else
    command python3 "$@"
  fi
}

make_hostile_archive() {
  local name="$1"
  local kind="$2"
  python3_fixture - "$fixture_root/$name.tar" "$kind" <<'PY'
import io
import sys
import tarfile

out, kind = sys.argv[1:]
with tarfile.open(out, "w") as archive:
    if kind == "traversal":
        info = tarfile.TarInfo("../escape")
        info.size = 0
        archive.addfile(info, io.BytesIO())
    elif kind == "symlink":
        info = tarfile.TarInfo("link")
        info.type = tarfile.SYMTYPE
        info.linkname = "manifest.json"
        archive.addfile(info)
    elif kind == "special":
        info = tarfile.TarInfo("device")
        info.type = tarfile.CHRTYPE
        info.devmajor = 1
        info.devminor = 3
        archive.addfile(info)
    elif kind == "duplicate":
        for _ in range(2):
            info = tarfile.TarInfo("manifest.json")
            info.size = 0
            archive.addfile(info, io.BytesIO())
    elif kind == "absolute":
        info = tarfile.TarInfo("/absolute")
        info.size = 0
        archive.addfile(info, io.BytesIO())
    elif kind == "backslash":
        info = tarfile.TarInfo("dir\\\\escape")
        info.size = 0
        archive.addfile(info, io.BytesIO())
    else:
        raise SystemExit("unknown fixture")
PY
}

make_hostile_archive traversal traversal
make_hostile_archive symlink symlink
make_hostile_archive special special
make_hostile_archive duplicate duplicate
make_hostile_archive absolute absolute
make_hostile_archive backslash backslash

(
  # The validator invokes timeout/python3; both are local fixture shims here.
  timeout() {
    while [[ "$#" -gt 0 && "$1" == -* ]]; do shift; done
    [[ "$#" -gt 0 ]] || return 1
    shift
    "$@"
  }
  python3() {
    if command -v python >/dev/null 2>&1; then
      command python "$@"
    else
      command python3 "$@"
    fi
  }
  eval "$archive_source"
  run_archive() {
    candidate_archive_path="$1"
    candidate_image_tag="$archive_tag"
    expected_candidate_image_id="$archive_id"
    validate_candidate_archive_manifest
  }
  expanded="$(run_archive "$fixture_root/safe.tar")"
  [[ "$expanded" =~ ^[0-9]+$ && "$expanded" -gt 0 ]] || fail 'safe Docker archive fixture was rejected or not bounded'
  expect_failure() {
    if run_archive "$1" >/dev/null 2>&1; then
      fail "unsafe Docker archive fixture was accepted: $2"
    fi
  }
  # Change the fixture's expected tag/config for targeted checks where needed.
  expect_failure "$fixture_root/wrong-tag.tar" wrong-tag
  expect_failure "$fixture_root/missing-layer.tar" missing-layer
  expect_failure "$fixture_root/extra-image.tar" extra-image
  expect_failure "$fixture_root/wrong-config.tar" wrong-config
  expect_failure "$fixture_root/traversal.tar" traversal
  expect_failure "$fixture_root/symlink.tar" symlink
  expect_failure "$fixture_root/special.tar" special
  expect_failure "$fixture_root/duplicate.tar" duplicate
  expect_failure "$fixture_root/absolute.tar" absolute
  expect_failure "$fixture_root/backslash.tar" backslash
)

printf 'subnexus Docker candidate-check static and fixture tests passed\n'
