# SubNexus 同库切换手册

本手册只适用于本地 Batch 1-5 全部完成、维护者验收、迁移分支已推送、候选 release 已固定为完整 40 位 SHA，且维护者另行明确批准发布之后。当前阶段禁止执行本手册中的服务器命令、生产迁移、备份、切流或功能开启。

## 1. 发布前硬门禁

- 发布前必须确认 `feature/subnexus-migration` 已通过代码、后端、前端和 Docker 验证，经维护者验收后推送并固定不可变 release SHA；未经另行批准不修改 `main`。
- 发布前必须在生产备份恢复出的隔离 PostgreSQL/Redis 上启动候选版本，确认目标自动迁移无 checksum mismatch，且旧版本连接迁移后克隆库仍可登录并读取核心数据。
- 发布前必须保存并校验 PostgreSQL custom-format 备份、Redis 恢复点、应用镜像、旧容器 inspect、单独采集的 Nginx 有效配置和文件存储目录快照。
- 发布前必须取得目标脚本 `tools/production-deploy/subnexus-readonly-preflight.sh` 的线上只读证据，并核对 `schema_migrations`、`atlas_schema_revisions`、真实网络、挂载和开关状态；当前阶段尚未取得这些证据。
- 所有迁移功能仍为关闭态；逐项开启顺序固定为 Batch 1 → Batch 2 → Batch 3 → Batch 4，并为每项保留验收记录。

## 2. 只读预检

以下内容是历史 Batch 0 预检资产，仅用于审计，不是当前可执行入口。最终本地候选完成后必须重新审核脚本、生成新的提交 SHA 和 SHA256，并由维护者明确批准后才能在生产服务器执行。

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

## 3. 备份与候选启动

1. 维护窗口开始前停止旧应用的写流量或将入口置于维护页；数据库和 Redis 保持运行。
2. 生成并校验最终 PostgreSQL custom-format 备份和 Redis 恢复点，同时保存应用镜像及旧容器元数据。备份路径必须落在服务器专用证据目录，权限 `0700/0600`。
3. 构建候选镜像时使用审核过的完整 SHA；候选使用与旧实例相同的数据库、Redis、JWT secret、TOTP encryption key 和持久化目录。
4. 候选启动时保持所有迁移开关为 `false`。如果应用启动触发新增迁移，应先在候选日志确认完成，再进行健康和鉴权 smoke；不要手工重复执行同一迁移。

## 4. 切流与观察

1. 确认旧容器没有运行中的结算、迁移或奖励任务后停止旧应用容器，并立即启动候选容器。
2. 先访问候选本地端口 `/health`、登录、用户/API Key、余额、订阅、订单、用量、模型列表和管理端只读接口。
3. 检查容器 UID、`NoNewPrivs`、重启次数、日志中的 migration/SQL/panic 错误，再切换 Nginx/Cloudflare 上游端口。
4. 切流后执行公网健康、登录、网关只读请求和支付回调模拟；观察至少一个完整任务/结算周期。
5. 只在当前批次验收记录签字后开启一个功能开关。开关开启顺序：活动基础 → 首充/邀请 → 发票 → Battle Pass。任一异常先关闭对应开关并保留候选实例，禁止直接删除新表。

## 5. 数据核对

切换前后记录并比较用户数、余额总额、未完成订单、订阅数、用量窗口、API Key 数量及每项迁移新增表的行数。金额和订单状态以数据库查询结果为准；抽样账户使用脱敏 ID/哈希，不在聊天或日志中传输敏感凭据。

## 6. 保留策略

旧容器、旧镜像、旧源码和配置、切换前数据库/Redis 备份、Nginx 备份及服务器证据至少保留至维护者确认可以清理。禁止在验收完成前执行 `docker system prune -a`、`docker volume prune` 或删除回滚脚本。
