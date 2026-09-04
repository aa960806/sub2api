# SubNexus 同库切换手册

本手册只适用于本地 Batch 1-5 全部完成、维护者验收、迁移分支已推送、候选 release 已固定为完整 40 位 SHA，且维护者另行明确批准发布之后。当前线上只读 preflight、备份结构校验、PostgreSQL/Redis 隔离恢复、候选 Docker runtime gate 和最新无停机 `prepare` 均已通过；维护窗口 `switch` 仍未执行，因此仍禁止生产迁移、替换容器、切流或功能开启。

## 1. 发布前硬门禁

- 发布前必须确认 `feature/subnexus-migration` 已通过代码、后端、前端和 Docker 验证，经维护者验收后推送并固定不可变 release SHA；未经另行批准不修改 `main`。
- 发布前必须在生产备份恢复出的隔离 PostgreSQL/Redis 上启动候选版本，确认目标自动迁移无 checksum mismatch，且旧版本连接迁移后克隆库仍可登录并读取核心数据。
- 发布前必须保存并校验 PostgreSQL custom-format 备份、Redis 恢复点、应用镜像、旧容器 inspect、单独采集的 Nginx 有效配置和文件存储目录快照。
- 已取得目标脚本 `tools/production-deploy/subnexus-readonly-preflight.sh` 的线上只读证据，并核对 `schema_migrations`、`atlas_schema_revisions`、真实网络、挂载和开关状态；证据和 SHA256 固定点记录在迁移台账与变更记忆中。
- 所有迁移功能仍为关闭态；逐项开启顺序固定为 Batch 1 → Batch 2 → Batch 3 → Batch 4，并为每项保留验收记录。

## 2. 只读预检

以下命令块是历史 Batch 0 预检设计参考，不是可重复执行入口。当前预检已经完成，其实际发布提交、脚本 SHA256 和 root-only 证据路径记录在迁移台账与变更记忆中；环境或脚本变化后必须重新审核，不得直接重跑历史命令。

最终发布清单必须同时记录批准预检脚本发布提交的完整 40 位 SHA 和该文件的 64 位 SHA256；脚本或其依赖环境每次变更后都必须重新生成这两个值，不能沿用历史固定值。服务器上的副本必须与维护者批准的发布清单逐项比对，不一致就停止，不要直接运行未校验副本。

历史批准值已移入迁移台账并冻结。下方命令块只保留校验设计参考，批准提交和 SHA256 使用不可执行占位符；最终候选完成前不能填充或复制到服务器运行。

```bash
set -Eeuo pipefail
repo_root='<approved-repo-root-from-live-inspect>'
app_container='<actual-running-app-container-name>'
public_health_url='<optional-public-health-url>'
evidence_root='/srv/subnexus-migration/preflight'
# Pass user-selected paths/identifiers as positional arguments. The root
# wrapper verifies the approved Git blob, copies it to a root-only temporary
# file, and executes that verified copy so hash-check and execution share one
# privilege boundary and do not have a TOCTOU gap.
sudo bash -s -- "$repo_root" "$app_container" "$public_health_url" "$evidence_root" <<'PREFLIGHT_VERIFY_AND_RUN'
set -Eeuo pipefail
export PATH='/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin'
repo_root="$1"
app_container="$2"
public_health_url="$3"
evidence_root="$4"
script_relative_path='tools/production-deploy/subnexus-readonly-preflight.sh'
script_path="$repo_root/$script_relative_path"
approved_script_commit_sha='<final-approved-commit-sha>'
expected_script_sha256='<final-approved-script-sha256>'
[[ "$approved_script_commit_sha" =~ ^[0-9a-f]{40}$ ]]
[[ "$expected_script_sha256" =~ ^[0-9A-F]{64}$ ]]
[[ -d "$repo_root/.git" ]] || { printf 'ERROR: repo root is not a Git worktree: %s\n' "$repo_root" >&2; exit 1; }
[[ "$(stat -c '%u' -- "$repo_root")" == '0' ]] || { printf 'ERROR: repo root must be root-owned\n' >&2; exit 1; }
[[ "$(stat -c '%u' -- "$repo_root/.git")" == '0' ]] || { printf 'ERROR: Git metadata must be root-owned\n' >&2; exit 1; }
git -C "$repo_root" cat-file -e "$approved_script_commit_sha^{commit}"
test "$(git -C "$repo_root" show "$approved_script_commit_sha:$script_relative_path" | sha256sum | awk '{print toupper($1)}')" = "$expected_script_sha256"
test "$(sha256sum "$script_path" | awk '{print toupper($1)}')" = "$expected_script_sha256"
verified_script="$(mktemp /tmp/subnexus-readonly-preflight.XXXXXX)"
trap 'rm -f -- "$verified_script"' EXIT
chmod 700 -- "$verified_script"
git -C "$repo_root" show "$approved_script_commit_sha:$script_relative_path" > "$verified_script"
test "$(sha256sum "$verified_script" | awk '{print toupper($1)}')" = "$expected_script_sha256"
bash "$verified_script" "$app_container" "$public_health_url" "$evidence_root"
PREFLIGHT_VERIFY_AND_RUN
```

### 执行前置条件

- 使用 root 运行；主机提供 GNU Bash 4+、GNU `timeout`（支持 `--foreground` 和 `--kill-after`）、`stat`、`realpath`、`flock`、`mktemp`，以及 `python3`、`curl`、`sha256sum` 等脚本依赖。用于校验的隔离仓库副本及其 `.git` 元数据必须由 root 持有；不要在旧版本正在使用的工作树上执行。
- 若用 `git clone --no-checkout` 准备隔离仓库，必须先 `git checkout --detach <approved-commit>`，再执行 `git status --porcelain` 空工作树断言；未 checkout 的克隆会把全部受跟踪文件显示为删除，不能据此判定仓库损坏。
- Docker CLI 必须连接本机默认 Docker context（Unix socket）；`DOCKER_HOST` 必须未设置。应用、PostgreSQL 和 Redis 容器必须正在运行并与应用加入同一 Docker 网络。脚本有意拒绝外部/托管数据库或 Redis 地址；仅有 IPv6 的 `8080/tcp` 发布也会被拒绝。
- 应用环境必须提供可解析到该网络容器的 `DATABASE_HOST`、`DATABASE_USER` 和简单 `DATABASE_DBNAME`（最多 63 个 ASCII 字符，以字母开头且其余仅字母、数字和下划线，不能是 PostgreSQL conninfo/URI）；仅配置 `DATABASE_URL` 的实例不满足本预检输入合同。
- PostgreSQL 容器内必须有可执行的 `psql`，Redis 容器内必须有 `redis-cli`。Redis 预检要求 standalone（Redis Cluster 不在本脚本支持范围），且 ACL 用户允许 `PING`、`INFO`、`DBSIZE` 和选择目标逻辑库（`-n` 会执行数据库选择）；建议 Redis server 与 CLI 使用 6.x 或更新的同一主版本。

脚本对应用、Docker、PostgreSQL 和 Redis 只执行读取；唯一写入是证据目录中的 `evidence.txt`、SHA-256 文件和并发锁文件，不执行迁移、备份、DDL/DML、重启或切流。数据库和 Redis 命令在各自的依赖容器内执行，因此只能证明依赖服务自身可连接，不能替代应用容器实际连接路径的 smoke 测试。PostgreSQL 查询设置了会话级超时和只读模式；外层 Docker 超时仍可能留下服务器端 `docker exec` 进程，Redis 命令没有等价的服务端命令超时，超时后应人工确认并清理残留进程。脚本会全量记录 `schema_migrations`（含旧编号）、Atlas revision 摘要、活动相关对象、旧/新活动设置 key 和估算行数，并输出脱敏的存储摘要及 Nginx 存在性/版本标记；有效 Nginx 配置必须按单独的维护者审查步骤采集。请把脱敏后的内容回传到本地台账。证据中不得包含密码、Token、Cookie、JWT/TOTP secret、API Key、完整环境变量或完整 Nginx 配置。

## 3. 本地隔离构建与脚本批准

隔离构建只允许在专用本地/WSL Docker daemon 中执行，绝不使用默认 Docker Desktop、远程 context 或生产 socket。构建入口保留原有位置参数 `SOURCE_ROOT APPROVED_COMMIT_SHA [ARTIFACT_ROOT]`；新增且必填的环境变量 `SUBNEXUS_APPROVED_BUILD_SCRIPT_SHA256` 必须来自维护者批准的发布清单，使用 64 位小写 SHA-256。该值不能在同一条命令中现算现批，否则没有独立批准意义。

执行前应使用受信 wrapper 从批准提交提取脚本 blob 到一次性非符号链接文件，再以该文件启动构建。wrapper 先核对批准清单中的脚本 SHA、Git blob SHA 和临时文件 SHA；构建脚本随后再次核对 Git blob、实际执行文件和外部环境值，并在任何 Docker RPC 之前失败关闭。构建输出 `BUILD_SCRIPT_SHA256`、`APPROVED_BUILD_SCRIPT_SHA256` 和 `APPROVED_BUILD_SCRIPT_BLOB_SHA256`，三者必须一致后才可把归档交给候选 gate。

下面是本地示例。`<approved-build-script-sha256>` 必须从独立审核记录填写，不要用命令替换占位符；示例不会访问服务器或生产数据库：

```bash
set -Eeuo pipefail
source_root='/work/sub2api'
approved_commit_sha='<approved-40-character-commit-sha>'
expected_build_script_sha256='<approved-build-script-sha256>'
script_relative_path='tools/production-deploy/subnexus-isolated-image-build.sh'
script_blob_sha256="$(git -C "$source_root" show "$approved_commit_sha:$script_relative_path" | sha256sum | awk '{print tolower($1)}')"
test "$script_blob_sha256" = "$expected_build_script_sha256"
verified_script="$(mktemp /tmp/subnexus-isolated-image-build.XXXXXX)"
trap 'rm -f -- "$verified_script"' EXIT
chmod 700 -- "$verified_script"
git -C "$source_root" show "$approved_commit_sha:$script_relative_path" >"$verified_script"
test "$(sha256sum "$verified_script" | awk '{print tolower($1)}')" = "$expected_build_script_sha256"
SUBNEXUS_APPROVED_BUILD_SCRIPT_SHA256="$expected_build_script_sha256" \
SUBNEXUS_BUILD_DOCKER_CONTEXT='subnexus-local-20260904' \
SUBNEXUS_LOCAL_DOCKER_CONFIRM='I_UNDERSTAND_LOCAL_ONLY' \
SUBNEXUS_CANDIDATE_NODE_IMAGE='<repo@sha256:digest>' \
SUBNEXUS_CANDIDATE_GOLANG_IMAGE='<repo@sha256:digest>' \
SUBNEXUS_CANDIDATE_ALPINE_IMAGE='<repo@sha256:digest>' \
SUBNEXUS_CANDIDATE_POSTGRES_IMAGE='<repo@sha256:digest>' \
SUBNEXUS_CANDIDATE_BUILDKIT_IMAGE='<repo@sha256:digest>' \
bash "$verified_script" "$source_root" "$approved_commit_sha" '/work/subnexus-artifacts'
```

缺少该环境变量、格式错误、Git blob 与批准值不一致，或执行文件被替换时，脚本必须在 Docker context inspect 之前退出。任何脚本变更都要重新生成批准提交/脚本 SHA 并重新审核。

### 3.1 Docker 环境重复键兼容

`docker inspect .Config.Env` 在少数历史容器中可能包含同名键。切换脚本默认按严格模式拒绝任何重复键，避免把 Docker 的隐式覆盖规则误当成已批准配置。当前线上只读检查确认过的兼容对象是 `SERVER_TRUSTED_PROXIES`；这不是对未来其他键的通行许可。

只有在维护者逐项复核当前 live 容器后，`prepare` 才可以同时接收以下三个独立值：

```text
SUBNEXUS_CUTOVER_ENV_DUPLICATE_CONFIRM=I_UNDERSTAND_DOCKER_ENV_LAST_WINS
SUBNEXUS_CUTOVER_ENV_DUPLICATE_KEYS=SERVER_TRUSTED_PROXIES
SUBNEXUS_CUTOVER_ENV_DUPLICATE_EXPECTED_SHA256=SERVER_TRUSTED_PROXIES=<最终出现值的 64 位小写 SHA256>
```

`EXPECTED_SHA256` 是同名键最后一次出现的值的 SHA-256，不是键名、整行或环境文件的哈希。批准值必须来自单独的只读复核记录；不要在调用 `prepare` 的同一条命令中读取明文、现算现批或把值写入 shell 历史。键和哈希列表必须按键排序、无重复，并且每个重复键都要有对应哈希。确认值、键清单或哈希任一缺失/不匹配，脚本都会在备份、数据库写入、容器停止之前失败关闭。

通过后，脚本只把每个键最后一项写入 root-only 的 `container.env`（0600），并在 `environment-duplicates.tsv` 中记录出现次数、位置和各值哈希；证据文件不记录环境明文。manifest 固定规范化环境文件和证据文件的 SHA-256。`switch` 前会重新检查 live 容器的键序列、选中值哈希和规范化环境哈希；live 序列发生漂移会停止切换。候选容器若由 Docker 创建时已经把重复项规范化为唯一键，这是允许的，前提是规范化文件哈希仍与 prepare 完全一致。旧的无该 evidence 字段的 prepared run 按历史严格无重复合同兼容读取，不会获得 last-wins 豁免。

### 3.2 应用数据目录 owner 合同

应用数据源默认要求 root-owned、目录链不可写且无符号链接。当前线上实时目录 `/srv/subnexus-migration/runtime/subnexus-data` 为 UID/GID `1000:1000`、mode `0755`；这是唯一已审核的非 root 兼容 owner。执行 `prepare` 时必须同时提供：

```text
SUBNEXUS_CUTOVER_APP_DATA_OWNER_CONFIRM=I_UNDERSTAND_NON_ROOT_APP_DATA_OWNER
SUBNEXUS_CUTOVER_APP_DATA_OWNER_UID=1000
SUBNEXUS_CUTOVER_APP_DATA_OWNER_GID=1000
```

脚本只允许该非 root owner 出现在最终 `/app/data` 叶目录；父目录仍必须由 root 持有且不得对 group/other 可写。`prepare`、`switch` 和 `rollback` 会重复校验 owner、mode、设备号和 inode；不得为迁移而 `chown` 或修改线上数据目录。没有 owner 字段的旧 manifest 只走 legacy root-UID 兼容路径（保留旧脚本只校验 UID 的行为），不允许借此为新 prepare 绕过现代 `1000:1000` 合同。

## 4. 备份与候选启动

1. 先让旧应用继续正常服务，使用已批准脚本执行在线 `prepare`；该阶段不得停止、重命名或重启旧应用，不创建候选容器，也不执行生产迁移、DDL/DML 或开关修改。
2. `prepare` 生成并校验 PostgreSQL custom-format 备份、catalog、Redis RDB/校验报告、设置快照、应用持久化数据归档、旧容器运行合同和依赖身份。所有文件必须位于 root-only 证据目录并通过 SHA-256、owner、mode、inode 和预算校验。
3. 在线应用数据归档固定排除 `./logs/*.log`，因为这些活动文本日志在服务期间持续写入；`./logs/*.gz` 和其他应用数据继续归档，任何其他变化或 tar 错误仍 fail-closed。日志轮转若正在原地更新 `.log.gz`，本次 prepare 可能安全失败并需稍后重试，不会为了成功而忽略错误。每个 run 必须包含 `application-data-exclusions.txt`、其 SHA256 sidecar 和 manifest hash；因此该归档不得描述为“包含活动日志的全量副本”。活动日志和旧容器本身继续原地保留。
4. `READY` 只能在镜像 ID/归档/gate 证据、全部备份哈希、设置快照、应用数据源身份、线上容器和 PostgreSQL/Redis 依赖身份再次一致后生成。没有 `READY` 时禁止执行 `switch`。
5. 候选镜像使用审核过的应用提交，所有迁移功能仍默认关闭。候选容器只在维护者执行最终 `switch` 后创建和启动；如启动触发新增迁移，应核对自动迁移日志，不得手工重复执行同一迁移。

## 5. 切流与观察

1. 确认旧容器没有运行中的结算、迁移或奖励任务后停止旧应用容器，并立即启动候选容器。
2. 先访问候选本地端口 `/health`、登录、用户/API Key、余额、订阅、订单、用量、模型列表和管理端只读接口。
3. 检查容器 UID、`NoNewPrivs`、重启次数、日志中的 migration/SQL/panic 错误，再切换 Nginx/Cloudflare 上游端口。
4. 切流后执行公网健康、登录、网关只读请求和支付回调模拟；观察至少一个完整任务/结算周期。
5. 只在当前批次验收记录签字后开启一个功能开关。开关开启顺序：活动基础 → 首充/邀请 → 发票 → Battle Pass。任一异常先关闭对应开关并保留候选实例，禁止直接删除新表。

## 6. 数据核对

切换前后记录并比较用户数、余额总额、未完成订单、订阅数、用量窗口、API Key 数量及每项迁移新增表的行数。金额和订单状态以数据库查询结果为准；抽样账户使用脱敏 ID/哈希，不在聊天或日志中传输敏感凭据。

## 7. 保留策略

旧容器、旧镜像、旧源码和配置、切换前数据库/Redis 备份、Nginx 备份及服务器证据至少保留至维护者确认可以清理。禁止在验收完成前执行 `docker system prune -a`、`docker volume prune` 或删除回滚脚本。

## 8. 当前已准备发布固定点

最新可供维护者人工核对的 prepared run 为：

- run：`/srv/subnexus-migration/cutover/20260904175519-3701605`
- 候选提交：`02774d028d076e934a59f04fd1ee98598ac693a1`
- 候选镜像 ID：`sha256:b49b764cfc2ca58d9f054c01ef9e17211b89b8280be30534ff83b4b90490a979`
- 部署脚本：`/srv/subnexus-migration/tools/subnexus-production-cutover-af82a6877-ba0f4c1e.sh`
- 部署脚本 SHA256：`ba0f4c1eeddcad82978028ae94f2e97b9a94cd54604c45a3bb847392dfb71064`

该 run 已有 `READY=prepared`，但 `cutover_allowed` 仍为 false。只有维护者确认无结算/迁移任务、备份和当前入口状态后，才可在维护窗口执行 `switch`；任何异常优先使用同一脚本的 `rollback`，不自动恢复数据库。

维护者确认上述条件后，人工切换命令（仅此命令会停止/重命名旧应用并启动候选）为：

```bash
sudo -n env -u DOCKER_HOST -u DOCKER_CONTEXT -u DOCKER_CONFIG -u DOCKER_TLS_VERIFY -u DOCKER_CERT_PATH -u DOCKER_API_VERSION SUBNEXUS_CUTOVER_CONFIRM=I_UNDERSTAND_SHORT_PRODUCTION_WINDOW SUBNEXUS_CUTOVER_QUIET_CONFIRM=I_HAVE_CHECKED_NO_SETTLEMENT_TASKS SUBNEXUS_APPROVED_CUTOVER_SCRIPT_SHA256=ba0f4c1eeddcad82978028ae94f2e97b9a94cd54604c45a3bb847392dfb71064 SUBNEXUS_CUTOVER_APP_DATA_OWNER_CONFIRM=I_UNDERSTAND_NON_ROOT_APP_DATA_OWNER SUBNEXUS_CUTOVER_APP_DATA_OWNER_UID=1000 SUBNEXUS_CUTOVER_APP_DATA_OWNER_GID=1000 /srv/subnexus-migration/tools/subnexus-production-cutover-af82a6877-ba0f4c1e.sh switch /srv/subnexus-migration/cutover/20260904175519-3701605
```

执行前必须由维护者实际确认没有结算、迁移或奖励任务，并核对 Nginx/入口配置；`SUBNEXUS_CUTOVER_QUIET_CONFIRM` 不是自动检查的替代品。切换失败或候选异常时，先保留证据和候选容器，再按回滚手册执行回滚命令。
