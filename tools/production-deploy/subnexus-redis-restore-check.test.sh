#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
subject="$script_dir/subnexus-redis-restore-check.sh"

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
  local expected="$1"
  grep -Fq -- "$expected" "$subject" || fail "missing invariant: $expected"
}

assert_not_contains() {
  local forbidden="$1"
  if grep -Fq -- "$forbidden" "$subject"; then
    fail "forbidden pattern present: $forbidden"
  fi
}

bash -n "$subject"

info_reader_source="$(sed -n '/^read_single_info_integer() {$/,/^}$/p' "$subject")"
[[ -n "$info_reader_source" ]] || fail 'Redis INFO reader function not found'
eval "$info_reader_source"

fixture_info=$'loading:0\r\nrdb_last_load_keys_loaded:37917\r\nrdb_last_load_keys_expired:1109\r\n'
assert_eq '0' "$(read_single_info_integer "$fixture_info" loading)" 'loading CRLF parsing'
assert_eq '37917' "$(read_single_info_integer "$fixture_info" rdb_last_load_keys_loaded)" 'loaded-key CRLF parsing'
assert_eq '1109' "$(read_single_info_integer "$fixture_info" rdb_last_load_keys_expired)" 'expired-key CRLF parsing'
if (read_single_info_integer $'loading:0\nloading:1\n' loading >/dev/null 2>&1); then
  fail 'duplicate INFO fields must fail closed'
fi
if (read_single_info_integer 'loading:LOADING' loading >/dev/null 2>&1); then
  fail 'non-numeric INFO fields must fail closed'
fi
if (read_single_info_integer 'other:0' loading >/dev/null 2>&1); then
  fail 'missing INFO fields must fail closed'
fi

token_reader_source="$(sed -n '/^read_single_info_token() {$/,/^}$/p' "$subject")"
[[ -n "$token_reader_source" ]] || fail 'Redis INFO token reader function not found'
eval "$token_reader_source"
assert_eq '8.8.0' "$(read_single_info_token $'redis_version:8.8.0\r\n' redis_version)" 'Redis version parsing'
if (read_single_info_token $'redis_version:8.8.0\nredis_version:8.8.1\n' redis_version >/dev/null 2>&1); then
  fail 'duplicate INFO token fields must fail closed'
fi

report_reader_source="$(sed -n '/^read_report_total() {$/,/^}$/p' "$subject")"
[[ -n "$report_reader_source" ]] || fail 'RDB report reader function not found'
eval "$report_reader_source"
fixture_dir="$(mktemp -d)"
trap 'rm -rf -- "$fixture_dir"' EXIT
printf '[offset 1] Checksum OK\r\n[info] 39026 keys read\r\n[info] 1109 already expired\r\n' >"$fixture_dir/report.txt"
assert_eq '39026' "$(read_report_total "$fixture_dir/report.txt")" 'RDB report CRLF parsing'
printf '[info] 39026 keys read\n[info] 39025 keys read\n' >"$fixture_dir/report.txt"
if (read_report_total "$fixture_dir/report.txt" >/dev/null 2>&1); then
  fail 'duplicate RDB report totals must fail closed'
fi

mount_validator_source="$(sed -n '/^validate_candidate_mounts() {$/,/^}$/p' "$subject")"
[[ -n "$mount_validator_source" ]] || fail 'candidate mount validator function not found'
(
  rdb_file='/srv/subnexus-migration/backups/20260903T073714Z/redis-dump.rdb'
  eval "$mount_validator_source"
  validate_candidate_mounts $'bind|/srv/subnexus-migration/backups/20260903T073714Z/redis-dump.rdb|/restore.rdb|false\ntmpfs||/data|true\ntmpfs||/tmp|true'
  if (validate_candidate_mounts $'bind|/srv/subnexus-migration/backups/20260903T073714Z/redis-dump.rdb|/restore.rdb|true' >/dev/null 2>&1); then
    fail 'writable RDB mounts must fail closed'
  fi
  if (validate_candidate_mounts $'bind|/srv/subnexus-migration/backups/20260903T073714Z/redis-dump.rdb|/restore.rdb|false\nbind|/host|/unexpected|true' >/dev/null 2>&1); then
    fail 'unexpected host mounts must fail closed'
  fi
)

container_lookup_source="$(sed -n '/^container_id_if_present() {$/,/^}$/p' "$subject")"
[[ -n "$container_lookup_source" ]] || fail 'container lookup function not found'
(
  eval "$container_lookup_source"
  fixture='present'
  docker() {
    if [[ "$1 ${2:-}" == 'container inspect' && "$fixture" == 'present' ]]; then
      printf '%064d\n' 1
      return 0
    fi
    if [[ "$1 ${2:-}" == 'container ls' && "$fixture" == 'inspect-error-listed' ]]; then
      printf '%s|fixture\n' "$(printf '%064d' 2)"
      return 0
    fi
    if [[ "$1 ${2:-}" == 'container ls' && "$fixture" != 'daemon-error' ]]; then
      return 0
    fi
    return 1
  }
  assert_eq "$(printf '%064d' 1)" "$(container_id_if_present fixture)" 'present container lookup'
  fixture='absent'
  lookup_status=0
  container_id_if_present fixture >/dev/null 2>&1 || lookup_status="$?"
  assert_eq '1' "$lookup_status" 'absent container lookup status'
  fixture='daemon-error'
  lookup_status=0
  container_id_if_present fixture >/dev/null 2>&1 || lookup_status="$?"
  assert_eq '2' "$lookup_status" 'daemon error lookup status'
  fixture='inspect-error-listed'
  lookup_status=0
  container_id_if_present fixture >/dev/null 2>&1 || lookup_status="$?"
  assert_eq '2' "$lookup_status" 'listed-but-uninspectable container status'
)

assert_contains '#!/bin/bash'
assert_contains 'set -Eeuo pipefail'
assert_contains 'case "$-" in'
assert_contains 'set +x'
assert_contains 'umask 077'
assert_contains "export PATH='/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin'"
assert_contains "readonly approved_backup_dir='/srv/subnexus-migration/backups/20260903T073714Z'"
assert_contains "readonly expected_manifest_sha256='9e4f0b156b9e1f5222ffababc88f5e43b65e6fdf9742e2d64abda921f6416540'"
assert_contains "readonly expected_rdb_sha256='2776e94f65acd0fbcbbb10b71c5b59b68fa74406141ab5dfb223f35b0ecbc725'"
assert_contains "readonly expected_check_sha256='4602e1b824ee2826174ffdb971d99129e0825bbfb24d8942c18fe748f6776f1a'"
assert_contains "readonly expected_total_keys='39026'"
assert_contains "readonly approved_script_path='/srv/subnexus-migration/tools/subnexus-redis-restore-check.sh'"
assert_contains 'restore-check script must be root-owned'
assert_contains 'mode_is_not_group_or_other_writable'
assert_contains 'fingerprint_paths'
assert_contains 'for docker_override in DOCKER_HOST DOCKER_CONTEXT DOCKER_CONFIG DOCKER_TLS_VERIFY DOCKER_CERT_PATH DOCKER_API_VERSION'
assert_contains 'default Docker endpoint must be the system Docker Unix socket'
assert_contains 'docker_socket_fingerprint_start'
assert_contains 'Docker daemon must provide seccomp isolation'
assert_contains 'timeout --foreground --kill-after=5s 60s'
assert_contains 'timeout --foreground --kill-after=2s 3s'
assert_contains 'docker create \'
assert_contains '--cidfile "$create_cidfile"'
assert_contains '--network none'
assert_contains '--publish-all=false'
assert_contains '--ipc private'
assert_contains '--restart no'
assert_contains '--read-only'
assert_contains '--cap-drop ALL'
assert_contains '--security-opt no-new-privileges'
assert_contains '--pids-limit 64'
assert_contains '--memory 256m'
assert_contains '--user 0:0'
assert_contains '--log-driver none'
assert_contains '--mount "type=bind,src=$rdb_file,dst=/restore.rdb,readonly"'
assert_contains "--tmpfs '/data:rw,nosuid,nodev,noexec,size=64m'"
assert_contains '"$production_image_id"'
assert_contains "cp /restore.rdb /data/dump.rdb"
assert_contains '--dir /data --dbfilename dump.rdb --save "" --appendonly no'
assert_contains "\$candidate_network_mode\" == 'none'"
assert_contains 'candidate_port_bindings'
assert_contains 'candidate_memory_swap'
assert_contains 'candidate_nano_cpus'
assert_contains 'candidate_pids_limit'
assert_contains 'candidate_publish_all_ports'
assert_contains 'candidate_data_tmpfs'
assert_contains 'candidate_tmp_tmpfs'
assert_contains 'candidate_devices'
assert_contains 'candidate_cap_add'
assert_contains 'candidate_privileged'
assert_contains 'candidate_device_requests'
assert_contains 'candidate_volumes_from'
assert_contains 'bind|$rdb_file|/restore.rdb|false'
assert_contains '"${#restore_volume_names[@]}" -eq 0'
assert_contains 'capture_candidate_isolation'
assert_contains 'capture_candidate_networks'
assert_contains 'validate_candidate_networks'
assert_contains 'capture_published_port_bindings'
assert_contains '.State.StartedAt'
assert_contains '.State.Pid'
assert_contains 'docker_quick exec "$restore_container_id"'
assert_contains 'unset REDISCLI_AUTH'
assert_contains 'candidate_rdb_hash'
assert_contains "redis_version"
assert_contains 'read_single_info_integer "$persistence_info" loading'
assert_contains 'read_single_info_integer "$persistence_info" rdb_last_load_keys_loaded'
assert_contains 'read_single_info_integer "$persistence_info" rdb_last_load_keys_expired'
assert_contains '10#$loaded_keys + 10#$expired_keys'
assert_contains '10#$dbsize > 0'
assert_contains "redis_cli SHUTDOWN NOSAVE"
assert_contains 'docker rm --force --volumes "$cleanup_id"'
assert_contains 'rm -- "$create_cidfile"'
assert_contains "trap '' INT TERM HUP"
assert_contains "trap on_exit EXIT"
assert_contains "assert_production_unchanged 'exit_cleanup'"
assert_contains 'verify_inputs_unchanged'
assert_contains 'APPROVED_REDIS_INPUTS_UNCHANGED=true'
assert_contains 'staged_evidence_file'
assert_contains 'cleanup_evidence_stage'
assert_contains 'sync -f "$staged_evidence_file"'
assert_contains 'mv -- "$evidence_stage_dir" "$evidence_dir"'
assert_contains 'REDIS_8_RESTORE_VALIDATED_PRODUCTION_UNCHANGED'

assert_not_contains 'docker pull'
assert_not_contains 'docker build'
assert_not_contains 'docker restart'
assert_not_contains 'docker stop'
assert_not_contains 'docker kill'
assert_not_contains 'docker prune'
assert_not_contains 'docker volume rm'
assert_not_contains 'docker logs'
assert_not_contains '--network host'
assert_not_contains '--network container:'
assert_not_contains 'seccomp=unconfined'
assert_not_contains '--publish "'
assert_not_contains '    -p '
assert_not_contains '--volumes-from'
assert_not_contains 'src=/var/run/docker.sock'
assert_not_contains 'dst=/var/run/docker.sock'
assert_not_contains 'docker exec "$production_redis_id"'
assert_not_contains 'docker rm --force --volumes "$production_redis_id"'
assert_not_contains 'docker ps -q'
assert_not_contains 'xargs docker'

if grep -n $'\r' "$subject" >/dev/null; then
  fail 'production script must use LF line endings'
fi

final_check_line="$(grep -n -F "assert_production_unchanged 'before_evidence_publish'" "$subject" | cut -d: -f1)"
pass_marker_line="$(grep -n -F "printf 'REDIS_8_RESTORE_GATE=passed" "$subject" | cut -d: -f1)"
publish_line="$(grep -n -F 'mv -- "$evidence_stage_dir" "$evidence_dir"' "$subject" | cut -d: -f1)"
[[ "$final_check_line" =~ ^[0-9]+$ && "$pass_marker_line" =~ ^[0-9]+$ && "$publish_line" =~ ^[0-9]+$ ]] ||
  fail 'atomic evidence ordering lines are missing'
(( final_check_line < pass_marker_line && pass_marker_line < publish_line )) ||
  fail 'passed marker must be written after final checks and before atomic evidence publish'

printf 'subnexus Redis restore-check tests passed\n'
