# SubNexus 同库切换手册

> 当前权威状态：2026-09-08（Asia/Shanghai）。2026-09-07 保留二开用户端界面发布事实完整保留在第 14 节；本轮“用户端雨景背景 + 毛玻璃卡片”仅修改显示层，本地审核和完整前端门禁已通过，候选代码提交及发布脚本哈希已固定，线上镜像、Gate、备份与 `prepare` 值仍待本轮流程生成。第 15 节是唯一现行发布交接；在其具体值全部核验前，不得使用任何历史 `switch`/`rollback` 命令。

本手册的人工命令只适用于候选提交、镜像、脚本哈希、备份、manifest、历史 anchor、新回滚目标身份和 never-started probe 均核验完成之后。本轮最终 `switch` 仍由维护者手动执行；构建或 Gate 通过本身不代表可以切换。

最新授权允许代理完成提交推送、隔离构建、上传安装、候选 Gate、全新备份、无停机 `prepare`、never-started probe、最终审计和范围明确的无用垃圾清理，并停在最终 `switch` 前。历史失败、已回滚或已成功切换的 run 均不得作为新的 `prepare`/`switch` 输入；最终两条单行命令必须绑定第 15 节本轮新 run。

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
3. 检查容器 UID、`NoNewPrivs`、重启次数、日志中的 migration/SQL/panic 错误。本轮继续使用既有入口，不修改 Nginx/Cloudflare 配置或端口。
4. 切流后执行公网健康、登录、网关只读请求和支付回调模拟；观察至少一个完整任务/结算周期。
5. 功能开关启用属于另一次业务发布，本轮 UI 发布不启用额外功能。任一异常保留日志和数据现场，只按第 15 节同一 wrapper/run 恢复切换时保留的 live 新回滚目标，禁止直接删除新表。

## 6. 数据核对

切换前后记录并比较用户数、余额总额、未完成订单、订阅数、用量窗口、API Key 数量及每项迁移新增表的行数。金额和订单状态以数据库查询结果为准；抽样账户使用脱敏 ID/哈希，不在聊天或日志中传输敏感凭据。

## 7. 保留策略

旧容器、旧镜像、旧源码和配置、切换前数据库/Redis 备份、Nginx 备份及服务器证据至少保留至维护者确认可以清理。禁止在验收完成前执行 `docker system prune -a`、`docker volume prune` 或删除回滚脚本。

## 8. 历史发布状态（已失效，不得复用）

四次历史曾生成或尝试生成 `READY` 的 run 都已进入失败/自动回滚终态，全部不可复用：

- `/srv/subnexus-migration/cutover/20260904175519-3701605`：重复 `create` 参数缺陷，`state=rolled_back`。
- `/srv/subnexus-migration/cutover/20260905002953-3824168`：Docker 29 将等价的 `HostConfig.OomKillDisable` 从旧容器的 `null` 序列化为候选的 `false`，旧脚本误判运行时合同不同，`state=rolled_back`。
- `/srv/subnexus-migration/cutover/20260905020043-3862867`：Docker 29 候选网络身份表示差异触发旧脚本误报，`state=rolled_back`。
- 最新第四次 run：候选 `Config.Cmd` 捕获时模板尾部换行产生额外空参数，runtime contract 不一致，`state=rolled_back`。具体路径和旧命令只作审计，不得重试。

第二次候选已成功启动并健康运行约 35 秒，随后脚本自动恢复旧容器和切换前设置。旧应用当前 healthy/restart=0；PostgreSQL 与 Redis 容器身份不变、restart=0；失败候选和临时旧容器名称均无残留。自动回滚没有恢复数据库，目标迁移 `9001`-`9013` 已于 `2026-09-05 01:17:03 UTC` 应用，生产记录的 13 个 checksum 与候选 SQL 全部一致；旧应用已在迁移后同库上恢复健康，证明应用级回滚兼容。

修复提交 `0d083f6b7cf53c440968f9a63e8bc4002017b53f` 将 `OomKillDisable=null/false` 规范为同一安全合同，`true` 仍被拒绝；同时保留显式 `0.0.0.0` 端口 HostIP，并在候选 entrypoint 启动前先校验合同、健康后再次复核。脚本 SHA256=`5291c6041305fa77902a113e2ef181615920bd37cbbd80e46e9fe095d0c21132`，测试 SHA256=`16fe581ecdf400ce6eb4f609b9a8cde1ee243666b9ab02f2199f3fc23e114880`。Windows Git Bash 与 WSL/Linux 发布夹具均通过。

当前严禁执行下方历史 run 的 `switch` 或 `rollback`。所有历史备份和证据仅作审计保留，不能作为新版本发布依据。新脚本必须先完成全新 `prepare` 并获得 `READY=prepared`，之后才能生成新的人工命令。切换前不得修改 Nginx 或开启任何迁移功能。

### 8.1 历史发布固定值（仅审计，全部禁用）

以下值全部属于已回滚 run，只用于审计和故障复盘。它们不是当前发布值，不能作为任何 `switch` 或 `rollback` 输入；新的修复脚本必须先生成全新 `prepare` run 后再单独记录新的固定值。

- 脚本：`/srv/subnexus-migration/tools/subnexus-production-cutover-5291c604-20260905.sh`
- 脚本 SHA256：`5291c6041305fa77902a113e2ef181615920bd37cbbd80e46e9fe095d0c21132`
- 已失效 run：`/srv/subnexus-migration/cutover/20260905020043-3862867`
- 候选提交：`02774d028d076e934a59f04fd1ee98598ac693a1`
- 候选镜像：`sha256:b49b764cfc2ca58d9f054c01ef9e17211b89b8280be30534ff83b4b90490a979`
- 候选归档 SHA256：`45306dfe47e6093d0be67d2446f7d83f7e82ef3407ef2b0f1ed8816489877786`
- runtime gate evidence SHA256：`1871ed998b92157e30c90daf3c0957570390a67df2fddc273164fe173712de61`
- stopped probe 合同 SHA256：`7dc88dd8f76be1a69c6d4f322deb1b1e0eda8be94be61d37cac850091578453d`；probe 从未启动并已删除。

### 8.2 历史切换命令（已禁用）

以下命令对应已回滚的历史 run，禁止执行。新的 run 就绪后必须重新生成命令，不能只替换脚本路径或 SHA：

旧命令正文已撤回；历史脚本 SHA 和 run 路径仅保留在上一节用于审计。新的人工 `switch`/`rollback` 命令必须绑定全新 run 的 manifest，不能复制 Git 历史中的命令。所有迁移功能仍保持关闭。

旧脚本、旧 run、旧容器、旧镜像及全部备份继续保留。禁止 `docker prune`、数据库恢复、手工重复迁移、Nginx 修改或功能开启。

## 9. 网络身份修复历史状态（非本轮执行入口）

上节列出的 `20260905020043-3862867` 已不再是有效 prepared run。维护者使用旧脚本执行 switch 时，候选在启动前触发 Docker 29 网络身份误报，脚本已自动恢复旧应用并删除候选；该 run 的 `READY` 与 `ROLLED_BACK` 证据均保留，但严禁再次 `switch` 或 `rollback`。

本地脚本已改为：

1. 比较候选 `NetworkSettings.Networks` 的网络名称集合与 `network-identities.txt` 的名称集合；
2. 对每个名称重新执行 `docker network inspect --format '{{.Id}}'`；
3. 将当前对象 ID 与 prepare 时记录的 ID 精确比较。

因此 Docker endpoint 的空值/暂态格式差异不会误报，但网络被重建、缺失、增加或 ID 漂移仍会拒绝切换。旧 run 不得套用新脚本，因为 manifest 会绑定脚本 SHA 和准备时的网络证据。

当时流程完成“新脚本安装、prepare 和 stopped probe 验证”，以下为历史进度记录，不是当前人工执行入口：

- 本节列出的失败或已覆盖命令不得作为新的 `prepare`/`switch` 输入，也不得再次执行 rollback；第 13 节旧 Rain rollback 窗口已关闭，当前恢复只使用第 14 节同 run rollback；
- 修复提交 `fbca62fbccb5a783d8d35cb9dcc4025cdb1c4a44` 已推送，脚本 SHA 为 `19824a87e3e1de5659cb30664750b71c5c10d374f25bda7f52e6524fe477ee65`，测试 SHA 为 `7e981ff118b795b40b38d22eb0a09667d7ac25977d9f17c5a30590dccece9763`；Git Bash/WSL full test 和 Linux 动态执行均通过；
- 服务器脚本 `/srv/subnexus-migration/tools/subnexus-production-cutover-19824a87-20260905.sh` 已安装为 `root:root`/`0700`；唯一旧 `runtime-probe.nroC3xIz` 已确认无容器后精确删除；
- 新的无停机 `prepare` run `/srv/subnexus-migration/cutover/20260905055413-3958448` 已 `READY=prepared`；PostgreSQL/Redis/应用归档 sidecar、manifest、runtime/settings 和 owner 合同均通过；
- stopped probe `ac6fc54a18cddb98fd9abce54ff2be6e23fd3ac02b804580d1220eaa770beadd` 已验证 created/false/0 后精确删除，evidence SHA=`87399f0bc40f41dee0600e1efd421f6953f75359cc067ee943d3ce1ba80627e0`，无候选或 probe 残留；
- 该历史批次的最终 `switch` 曾由维护者执行并成功；其后续历史状态见第 13 节。第 13 节 rollback 窗口已关闭，当前发布后的恢复入口仅为第 14 节同 run rollback；第 10 节旧命令已撤回。

此前 `cutover_allowed=false` 的人工确认已完成；候选的关闭态设置快照已验证，线上新容器已健康运行。本轮未开启额外功能或修改 Nginx。

## 10. 历史人工交接（2026-09-05 14:11:57 Asia/Shanghai，命令已撤回）

修复提交 `fbca62fbccb5a783d8d35cb9dcc4025cdb1c4a44` 已推送；脚本 SHA=`19824a87e3e1de5659cb30664750b71c5c10d374f25bda7f52e6524fe477ee65`，测试 SHA=`7e981ff118b795b40b38d22eb0a09667d7ac25977d9f17c5a30590dccece9763`。服务器已安装 `/srv/subnexus-migration/tools/subnexus-production-cutover-19824a87-20260905.sh`（`root:root`/`0700`）；唯一旧 `runtime-probe.nroC3xIz` 已确认无容器后精确删除。

当时 prepare 快照的 run 为 `/srv/subnexus-migration/cutover/20260905055413-3958448`，`READY=prepared`、manifest `state=prepared`，candidate 身份三个字段为空，无 `SWITCHED`/`ROLLED_BACK`。这些字段只代表该时间点，不代表现在可执行。备份和全部运行配置通过过原脚本的完整切换前校验；PostgreSQL dump 为 `5086279866` bytes，Redis RDB 为 `7143802` bytes，应用归档为 `80910450` bytes，完整 SHA 记录见项目变更记忆。

真实 probe 复用了完整候选创建和 runtime contract 校验函数，核实 `created|false|0|0001-01-01T00:00:00Z` 后按精确 ID 删除；临时元数据目录也已删除，原始 manifest 未修改。候选与线上准备记录的 runtime SHA 均为 `7dc88dd8f76be1a69c6d4f322deb1b1e0eda8be94be61d37cac850091578453d`。脱敏证据 `/srv/subnexus-migration/diagnostics/stopped-probe-20260905055413-3958448.evidence` 的 SHA 为 `87399f0bc40f41dee0600e1efd421f6953f75359cc067ee943d3ce1ba80627e0`。

最终复核：新应用 `aa1eabd0ac401d83cce20f7a221b324492ef62cc0195408db8ccdf04e7829471` 为 running/healthy/restart=0，健康接口返回 `{"status":"ok"}`；旧应用 `be459424b327...` 已保留为退出状态，供 rollback 使用；PostgreSQL `8178576aed6f...`、Redis `5c7adf42247c...` 原身份 running/restart=0。磁盘清理只删除了逐个确认无标签、无容器引用的 dangling 构建中间层和无容器残留的临时诊断副本；11 个共享层保留，未使用 prune/force，备份和生产资产保留。

维护者当时完成 switch，结果为 `SWITCH_COMPLETED`，窗口 `42` 秒。该批次的 switch/rollback 命令正文已经撤回，禁止复制 Git 历史中的命令作为本轮输入；原脚本、run、备份和证据继续保留审计。

## 11. v0.2.1 历史人工交接（2026-09-05，已 switched）

该 run `/srv/subnexus-migration/cutover/20260905114022-4163123` 后续实际已 switched，当前 v0.2.1 容器 ID 前缀为 `9753053d8bd9`。当时控制器为 `/srv/subnexus-migration/tools/subnexus-production-cutover-19824a87-20260905-v021.sh`，SHA256=`19824a87e3e1de5659cb30664750b71c5c10d374f25bda7f52e6524fe477ee65`。旧 switch/rollback 命令正文已撤回，该 run 不得用于本轮 UI 切换；原控制器仅作为本轮 UI 包装器验证过的依赖继续使用。

## 12. Rain + Glass UI 上一版历史交接（2026-09-06，后续已 switched，命令已撤回）

| 固定项 | 当前值 |
| --- | --- |
| UI 候选提交/tree | `b1ed483ea5fc648cb3c15fcf2e7040e68a151a41` / `bb821e2a0003d13cd425ca8ff012dbb26f70b1a6` |
| 候选镜像 | `sha256:32f14750ce73da00dc4c5146b1d9ad6c4420ee2c3dffe098798e41a123c6bd2c` |
| 归档 | `/srv/subnexus-migration/candidate-artifacts/rain-b1ed483ea5fc-retry2/candidate-image.tar`；SHA256=`26422d9eaad7ede983b228e84ee756eae313347b0135bf4e2d48138912c3246b` |
| Docker gate | `/srv/subnexus-migration/docker-candidate/20260905T155430Z-940fdcd9-bc3c-4d12-8c72-12ed9e27328b/evidence.txt`；passed；SHA256=`eb8e8a0b8e9c25f7d9b1b6491974751e12d24d3110e31796cf12ae5843b8fc9b` |
| 首页门禁 | `/srv/subnexus-migration/diagnostics/rain-home-b1ed483ea5fc.ENig2O5r`；passed；图片使用 `/rain-city-1.jpg` |
| UI 包装器 | commit=`2e60d0d55`；路径 `/srv/subnexus-migration/tools/subnexus-ui-cutover-7c3a42ac-20260906.sh`；SHA256=`7c3a42ac381f3839b5de5d605d465ee13b005ea9321b28ef47427ece2e910d77` |
| 原控制器 | `/srv/subnexus-migration/tools/subnexus-production-cutover-19824a87-20260905-v021.sh`；SHA256=`19824a87e3e1de5659cb30664750b71c5c10d374f25bda7f52e6524fe477ee65`；与包装器哈希独立校验 |
| 在线 prepare | `/srv/subnexus-migration/cutover/20260906100431-660485`；`READY=prepared`；manifest `state=prepared`, `ui_state=prepared`；SHA=`4cdd0bac0157663f9f485847ac92d5cd09d3f6a66b90def09f95f3389c4570b6` |
| 该次切换前生产应用 | v0.2.1 容器 ID 前缀 `9753053d8bd9`，当时尚未切换 |
| 固定旧回滚对象 | `be459424b327ad056ea9bdc02187d6a458fe09082369b354158d6e7f7758beee`；名称 `subnexus-cutover-pre-96b66b3e74c1-20260905085804-4072165`；anchor run `20260905085804-4072165`；image ID 前缀 `b24b585` |

该 run 后续已用于上一版 Rain UI 切换，该次切换后的线上容器为 `c3ea071f4526...`。上面的状态表只保留审计事实；其 switch/rollback 命令正文已经撤回，不得从 Git 历史复制重试。该次切换未创建新的永久回滚对象，当时的线上容器也未替代此前旧 SubNexus。

已定向删除失败 partial 约 3.386 GB 和 7 个无引用构建镜像，prepare 前空间约 19.1 GB 增至 24.61 GB；清理记录 `/srv/subnexus-migration/cleanup-rain-20260905.txt`，SHA256=`94a4840ce2fd9b3c3dce40c5864a691675e4ca752b85f3dc3f437e550e2829c2`。不使用 prune，不恢复 PostgreSQL/Redis，不修改 Nginx 或开启额外功能。

本轮前两次准备均不得交接：首次 `20260905160223-175225` 的 prepare 曾成功，随后全量 settings 哈希漂移，probe 在 create 前安全停止，无 candidate；具体变化键与来源仍未确定，后续哈希复验稳定不能作为原因已定位的证据。原控制器只保存 18 键快照，不能直接与全量快照对应比较。第二次 `20260905163008-194872` 在备份前因可用 `18797457408 < 23715311616` bytes 被拒绝。

首次失效 run 的三个大备份及 sidecar 已逐项校验/记录后精确删除，manifest/settings/metadata 与 `INVALIDATED_SETTINGS_DRIFT` 保留。清理证据 `/srv/subnexus-migration/cleanup-rain-invalid-run-20260905160223.txt`，SHA256=`c3e1af6e289292b4b2baa8b76136ea322f19556785a17caf63d6d34c2060d326`，清理后可用 `24025554944` bytes；固定旧回滚对象不变。最终 run 备份 SHA 及 stopped probe 证据见变更记忆文末；本段失效 run 不得作为新的 `prepare`/`switch` 输入，也不是 rollback 入口。

## 13. `F:\Rain` 首页源码直接迁移切换结果（2026-09-06 23:25 Asia/Shanghai，历史，rollback 已关闭）

| 固定项 | 当前值 |
| --- | --- |
| UI 候选提交/tree | `245ecd2630b96a9807df89dc02828bbb436e7624` / `b0f55487331dca07031219d59cd4159cab8a610d` |
| 候选镜像 | `sha256:e472d61e8db88ec5cdd0c0c4ad9e9db11b28c3495a14af02287c99b6addf23a7` |
| 候选归档 | `/srv/subnexus-migration/candidate-artifacts/rain-245ecd2630b9/candidate-image.tar`；SHA256=`65f06c3e221cfd08d68f84df882b2b3c858e5b8c939715f41c466eee0cb35f15` |
| Docker Gate | `/srv/subnexus-migration/docker-candidate/20260906T133853Z-42fcab50-97af-46e0-b0c2-a67e512f819e/evidence.txt`；SHA256=`d13c2a3095db4699d1a20939818d003e985f2715655f51e9715f286388d14544` |
| 首页 observer | `/srv/subnexus-migration/diagnostics/rain-home-245ecd2630b9.n49MjhQK/evidence.txt`；SHA256=`bd70fe35573d4a6c2ac6399cf50f9bec0cd18600b387ed8eea0e2f65ee76f678` |
| UI wrapper | `/srv/subnexus-migration/tools/subnexus-ui-cutover-054507b1-20260906.sh`；SHA256=`054507b15851c9547ab347f88ad21d8f9a5203be6123bfb2030e21c88806fd5d` |
| 原控制器 | `/srv/subnexus-migration/tools/subnexus-production-cutover-19824a87-20260905-v021.sh`；SHA256=`19824a87e3e1de5659cb30664750b71c5c10d374f25bda7f52e6524fe477ee65` |
| 切换前 prepare（历史证据） | `/srv/subnexus-migration/cutover/20260906134705-774592`；`READY=prepared`、`UI_READY=application-refresh-v1`；切换前 manifest `state=prepared/ui_state=prepared/ui_commit_intent=no`；SHA256=`e3809a4d6a09d469c994d38453551d466e73f49b45903aae17f8683fe63fc897` |
| 切换前最终审计 | 脚本 `/srv/subnexus-migration/tools/audit-rain-prepared-3d1e4a67-20260906.sh`，SHA256=`3d1e4a6734750256e4c6744e4c6857a63c764c7b7994e6ead5bcc0583c858959`；prepare evidence `/srv/subnexus-migration/diagnostics/rain-prepared-245ecd2630b9-20260906134705-774592.evidence`，SHA256=`0f69354a5d7911a66a6c5ef01fc58ac3160bd838a78fd214f8bb7151ac125609`；probe 从未启动且已删除，manifest/生产/依赖/anchor 前后不变 |
| Switch 结果 | `UI_SWITCH_COMPLETED=/srv/subnexus-migration/cutover/20260906134705-774592`；`SWITCHED=switched`，`ROLLED_BACK` 不存在；切换后 manifest `state=switched/ui_state=switched/ui_commit_intent=yes`；SHA256=`86afbaa48b5a22cdd193eb7f95238d8a70c74b870b7151476153317a3f0ffe79` |
| 当前生产 | candidate ID=`86104829d490733244c9426a59e82e7a12afa3c590de2fc03f2ccb13344aebbd`；image=`sha256:e472d61e8db88ec5cdd0c0c4ad9e9db11b28c3495a14af02287c99b6addf23a7`；running/healthy/restart=0；started=`2026-09-06T14:59:13Z` |
| 旧 pre-live 与临时对象 | 切换前 live ID=`c3ea071f4526bdb2502444d8f18b9da4c761aa3d51be6f7e5fc19c910ca6300f` 及 temporary name 已删除；previous-container 记录和日志已核对；除当前生产外无额外 candidate 容器，probe 容器和目录无残留 |
| 依赖与保护合同 | PostgreSQL ID=`8178576aed6f7b1cb94201832e5797907ea4d7698dbfe7b6f862cbc5a3b4f5bf`、Redis ID=`5c7adf42247c67ba90b09248056071a57c2a4e7e0465f922d4ed799ef092533e` 身份未变且 running/restart=0；18 个受保护设置、runtime contract、备份及 SHA sidecar、文件权限和固定 anchor 均通过切换后审计 |
| 切换后服务器审计 | 本地只读脚本 `F:\MySub2\tools\audit-rain-switched-245ecd2630b9.sh`，SHA256=`5e6063685e4057e5f87b99c0724d363ff8f87d43cf65821c3e06d371ecbdecb3`；`2026-09-06T15:19:27Z` 启动，最终输出 `POST_SWITCH_AUDIT=passed` 且退出码为 `0` |
| 公网验收 | `https://yydsapi.uno` 桌面/移动端均为直接迁移的 Rain UI；三张城市图片加载成功，两层 Canvas 非空，无页面错误或横向溢出；未登录状态下文档、模型广场、登录、语言、主题交互正常，客服按原功能开关保持关闭 |
| 固定旧回滚对象 | ID=`be459424b327ad056ea9bdc02187d6a458fe09082369b354158d6e7f7758beee`；image=`sha256:b24b585a35e0eecff497a4eb7a2be480d9a2818f4b7a9780508f2f42cb5e09cd`；name=`subnexus-cutover-pre-96b66b3e74c1-20260905085804-4072165`；exited/restart=0；anchor=`/srv/subnexus-migration/cutover/20260905085804-4072165`；anchor manifest SHA256=`e0b49d89e6a28044c5588afb5a536a3e87ceacfa2a7d679d324d65b14a4c100e` |

本节保留上一版 Rain UI 的发布事实和固定对象供审计。其 rollback 曾只在本轮 retained-UI switch 前的状态窗口内有效；当前生产已切换为 retained-UI candidate，该窗口已关闭，旧命令已撤回（`WITHDRAWN/CLOSED after retained-UI switch`）。不得从本节或 Git 历史复制执行。

## 14. 保留二开用户端界面切换结果（2026-09-07，历史，已 switched）

| 固定项 | 当前值 |
| --- | --- |
| UI 候选提交/tree | `f6f6dafe1fb2008d0a6f41dc746ae831babc3b18` / `7b0ee6db2dc96fd97106ca175640b3a15e8ec233`；已推送到 `origin/feature/subnexus-migration`，base=`245ecd2630b96a9807df89dc02828bbb436e7624` |
| 精确范围 | 34 个 production UI + 18 个测试/记忆/部署证据 = 52 个路径；API、后端、数据库迁移、router、依赖/锁文件、全局样式、Tailwind 和 Rain 首页未改变 |
| 候选镜像 | `sha256:59eb4c84de8b8fec11fb903dc728676e9cffacbcc435ce5ea1b60487cc910fcc` |
| 候选归档 | `/srv/subnexus-migration/candidate-artifacts/retained-ui-f6f6dafe1fb2/candidate-image.tar`；SHA256=`aae7dbca9336a81414f06fb273f8b66c1723aaef891316e4fe32827bf9650084` |
| UI wrapper | `/srv/subnexus-migration/tools/subnexus-ui-cutover-dd320d09-20260907.sh`；SHA256=`dd320d0982d357704d88bd702805cca69c36304ea6f5db523a572b10bdbdf49a` |
| 原控制器 | `/srv/subnexus-migration/tools/subnexus-production-cutover-19824a87-20260905-v021.sh`；SHA256=`19824a87e3e1de5659cb30664750b71c5c10d374f25bda7f52e6524fe477ee65` |
| Docker Gate | `/srv/subnexus-migration/docker-candidate/20260907T044220Z-38d5e210-40a5-40f4-a52a-d521af351d7e/evidence.txt`；SHA256=`6ed8c611f8c86893c49f5594f6f2b39cd01b010e26f68a7b0b2afcde82e39c8a` |
| 首页 observer | `/srv/subnexus-migration/diagnostics/rain-home-f6f6dafe1fb2.oNeOWKxE/evidence.txt`；SHA256=`272626c264aaf7d12811460707f231a903af89affbec342315434c1ea7cfac70` |
| 切换前 prepare（历史证据） | `/srv/subnexus-migration/cutover/20260907045159-1121373`；`READY=prepared`、`UI_READY=application-refresh-v1`；切换前 manifest `state=prepared/ui_state=prepared/ui_commit_intent=no`；SHA256=`87b51ae80dccc6ae4590537bcc2636795b0a650db84a7bb40a9531b4ce85d135` |
| 全新备份 | PostgreSQL `5275269397` B / SHA256=`d5528d86bcdd84c86ba5db056235a94349045f63f985de3d55ce4064053548c3`；catalog `118684` B / `7dd511aac04df73ea95f0d4bc74a8b4b3b32b44dca6a5bc3d76d56829ccefc85`；Redis `9703822` B / `28052d344e428c96da409c35225ad099b0ba3751f9f5f5ca9491a5c8661546aa`；应用数据 `71058234` B / `f686be37409fa776253f62e9bcc638b6e3028209276a9e6dd93ac478a92819a2`；实际文件、sidecar、manifest 与格式/复读检查全部通过 |
| 设置/运行合同 | 切换前 prepare 的 18 个受保护设置 SHA256=`3959daf3caed2f8a4c22023db4b7da8be627fb4b8a087bba4d5309cd8223d558`；切换后当前 SHA256=`eddb4a4333c07c1cea357f12e2adb0bede963f6f0806f6757895c8d3f0f092d4`（唯一变化为管理员 PUT 的 `subnexus_invite_activities_config`，其余 17 项保持 prepare 值）；runtime contract SHA256=`7dc88dd8f76be1a69c6d4f322deb1b1e0eda8be94be61d37cac850091578453d` |
| Stopped probe | ID=`44891195f23177ccf3c05e30b54755eb9345980649772218c96385ee5dbdbe51`；从未启动，按完整 ID 删除，无临时容器/目录残留；probe log SHA256=`bec9ea3cf132de3839397747d957c2ac7e7522d7a24de499e11404f374f90c1d` |
| Switch 结果 | `UI_SWITCH_COMPLETED=/srv/subnexus-migration/cutover/20260907045159-1121373`；`SWITCHED=switched`，`ROLLED_BACK` 不存在；切换后 manifest `state=switched/ui_state=switched/ui_commit_intent=yes`；SHA256=`5cc60f478673b2615d96d2993604da934353a6abb66d932e81f268bdfa4acda3` |
| 当前生产 | candidate ID=`232f6c5b374605760529cfac6b765fe68ba6aafc0d5d0fc8641a6a3030d63511`；image=`sha256:59eb4c84de8b8fec11fb903dc728676e9cffacbcc435ce5ea1b60487cc910fcc`；`running/healthy/restart=0`；`candidate_container_intent=6f5b820275e40f0738ac841e4c6edb1dd3b92cb9066f1d8b132aaf5c97622ada` |
| 切换后服务器审计 | `/srv/subnexus-migration/tools/audit-retained-ui-switched-d5652bda-20260907.sh`，SHA256=`d5652bda85a82e5eaa20db5f5b80a86c691bcdbbb10f58d199636f0ff7f6766d`；evidence=`/srv/subnexus-migration/diagnostics/retained-ui-switched-f6f6dafe1fb2-20260907045159-1121373.evidence`，SHA256=`58edc5b2d6e3ca6535ae10741ce4aed9275609f5d5e8dad1c48407372349afb9`；`POST_SWITCH_AUDIT=passed` |
| 当前生产/依赖 | 当前应用 candidate=`232f6c5b374605760529cfac6b765fe68ba6aafc0d5d0fc8641a6a3030d63511` 为 `running/healthy/restart=0`；PostgreSQL=`8178576aed6f7b1cb94201832e5797907ea4d7698dbfe7b6f862cbc5a3b4f5bf`、Redis=`5c7adf42247c67ba90b09248056071a57c2a4e7e0465f922d4ed799ef092533e` 身份未变且均 `running/restart=0` |
| 设置漂移裁决 | 切换后唯一变化键为 `subnexus_invite_activities_config`；Nginx 记录管理员 `PUT /api/v1/admin/invite-activities/config` 返回 200，数据库 `updated_at=2026-09-07T06:45:51.718553Z`；未回写或恢复，其他 17 项保持 prepare 值 |
| 切换清理 | 旧 live `86104829d490733244c9426a59e82e7a12afa3c590de2fc03f2ccb13344aebbd`、temporary name、gate/probe 容器及目录已按精确身份清理；失败审计 partial 与迁移上传副本共 27 个临时文件已删除，清理证据 `/srv/subnexus-migration/cleanup-retained-ui-postswitch-20260907-1121373.txt`，SHA256=`282250b4f612f154e60c7d1b42954ac005b9bb8b3e6db11710a08dacf46b6558` |
| 固定旧回滚对象 | ID=`be459424b327ad056ea9bdc02187d6a458fe09082369b354158d6e7f7758beee`；image=`sha256:b24b585a35e0eecff497a4eb7a2be480d9a2818f4b7a9780508f2f42cb5e09cd`；name=`subnexus-cutover-pre-96b66b3e74c1-20260905085804-4072165`；anchor=`/srv/subnexus-migration/cutover/20260905085804-4072165`；anchor manifest SHA256=`e0b49d89e6a28044c5588afb5a536a3e87ceacfa2a7d679d324d65b14a4c100e` |

该轮 switch 已消费并禁止重跑。其 rollback 命令在第 15 节新发布准备开始后退出现行交接，下面正文仅作为当时发布记录保留，不得用于本轮操作。

`WITHDRAWN/CLOSED for 2026-09-08 Rain + Glass cutover`：下列命令是 2026-09-07 的历史文本，不再授权执行。

```bash
sudo -n env -u DOCKER_HOST -u DOCKER_CONTEXT -u DOCKER_CONFIG -u DOCKER_TLS_VERIFY -u DOCKER_CERT_PATH -u DOCKER_API_VERSION SUBNEXUS_APPROVED_UI_CUTOVER_SCRIPT_SHA256=dd320d0982d357704d88bd702805cca69c36304ea6f5db523a572b10bdbdf49a SUBNEXUS_CUTOVER_CONFIRM=I_UNDERSTAND_APPLICATION_ROLLBACK SUBNEXUS_CUTOVER_APP_DATA_OWNER_CONFIRM=I_UNDERSTAND_NON_ROOT_APP_DATA_OWNER SUBNEXUS_CUTOVER_APP_DATA_OWNER_UID=1000 SUBNEXUS_CUTOVER_APP_DATA_OWNER_GID=1000 SUBNEXUS_DOCKER_TIMEOUT_SECONDS=120 bash /srv/subnexus-migration/tools/subnexus-ui-cutover-dd320d09-20260907.sh rollback /srv/subnexus-migration/tools/subnexus-production-cutover-19824a87-20260905-v021.sh /srv/subnexus-migration/cutover/20260907045159-1121373
```

## 15. 用户端 Rain + Glass 发布交接（2026-09-08，准备中）

本轮只统一登录后用户端的雨景背景和毛玻璃卡片材质，现有 API、请求参数、路由、权限、认证、配置、功能开关、按钮事件和业务处理保持不变。2026-09-07 第 14 节继续作为历史事实保留，不得把其中的脚本或 run 作为本轮输入。

| 固定项 | 本轮状态 |
| --- | --- |
| 生产 base | `f6f6dafe1fb2008d0a6f41dc746ae831babc3b18`；最终仍须由线上 live image provenance 实时确认 |
| 候选提交/tree | commit=`187b128bd32d1e06ad6e08817632e7c6b5ccca92`、tree=`e81b71b7f136da0a62bd51e266f041c6312e6b0b`，已推送；base=`f6f6dafe1fb2008d0a6f41dc746ae831babc3b18`，后续记账提交不得替代镜像构建 SHA |
| 候选镜像/归档 | 待隔离构建生成并填写完整 image ID、归档路径、归档 SHA256 和大小 |
| UI wrapper | target Git blob/本地 SHA256=`6b1635548887459ad408d56226fdceadbaa8d72b845e8b3a3dac3ae65815233f`；测试 SHA256=`6a682d9f33d308eb648c914519f08e1e1afdc8a041095bfc3dddd6e38db82b26`；服务器唯一路径与安装后 SHA 待上传时填写 |
| 原控制器 | 沿用已审核控制器时仍须实时核对固定路径和 SHA256，不得从历史章节直接假定 |
| Docker Gate | 待服务器候选 Gate 通过后填写 evidence 路径/SHA256；必须 `result=passed`、`cleanup_failed=false`，且生产身份前后不变 |
| 在线备份/prepare | 待生成全新 run；必须包含本轮 PostgreSQL、catalog、Redis、应用数据归档、全部 sidecar、设置快照和 runtime contract，不得复用第 14 节备份或 run |
| Never-started probe/最终审计 | 待新 run `READY=prepared` 后执行；probe 必须从未启动并按完整 ID 删除，最终证据必须确认无候选/probe 残留和 live/依赖/settings 不变 |
| 人工边界 | 代理完成上述全部前置工作并停在 `state=prepared/ui_state=prepared`；维护者只执行最终给出的本轮单行 `switch`，需要恢复时只执行同 wrapper/run 的单行 `rollback` |

新的回滚合同如下：

1. `prepare` 前历史 SubNexus anchor `/srv/subnexus-migration/cutover/20260905085804-4072165` 必须存在，并对其 manifest、旧容器完整 ID、image、名称、停止状态、依赖和运行合同做完整校验；缺失或漂移必须在备份或 Docker 变更前失败关闭。
2. `prepare` 不停止、重命名或创建容器，只把当时 live 固定为本轮新回滚目标，并在 manifest 记录 `ui_new_rollback_id`、`ui_new_rollback_image`、`ui_new_rollback_config_image`、唯一 `ui_new_rollback_name=production-app-ui-prior-<run-id>` 和 `ui_new_rollback_state=prepared`。
3. 维护者执行 `switch` 时，wrapper 才停止当前 live、按上述唯一名称重命名并验证其完整 ID、`.Image`、`.Config.Image`、名称、runtime contract 和 `stopped` 状态；随后创建并启动候选。候选健康后保留该 stopped 容器，不得删除或提交为镜像。
4. 本轮 `rollback` 删除经身份校验的候选并恢复上述新目标；历史 SubNexus anchor 不再是本轮恢复对象，但现行 wrapper 仍在 switch、失败恢复和 rollback 中把它作为连续性门禁。anchor 或其容器/镜像缺失，以及新目标任一身份字段漂移时，都必须失败关闭。
5. 空间充足时保留全部历史数据。空间不足时，只能在删除清单逐项绑定完整 ID/路径/SHA 并保存 root-only 审计记录后，精确删除其他已失效 run 备份、上传残留或无引用垃圾；历史 anchor 及其容器/镜像必须贯穿本轮 switch/rollback 窗口保留。不得执行 `docker system prune`、`docker volume prune` 或前缀/通配符清理，历史 anchor、本轮新回滚目标及新 run 证据不得删除。

当前尚未生成可执行命令。只有候选 SHA、镜像、脚本 SHA、run、备份、probe 和最终审计全部产生并复核后，才在本节写入两条绑定同一 wrapper/run 的完整单行命令；在此之前任何历史命令或自行替换占位值的命令都无效。
