# SubNexus 迁移项目上下文

> 本文件是新 fork 的长期维护入口。任何 AI 或开发者在修改代码前必须先阅读本文件、`SUBNEXUS_CHANGE_MEMORY.md`、`SUBNEXUS_MIGRATION_PLAN.md` 和 `SUBNEXUS_MIGRATION_LEDGER.md`。
>
> 当前权威状态：2026-09-06（Asia/Shanghai）。首页 Rain + Glass UI 候选镜像及门禁不变；最终 run `20260905163754-200276` 已 `READY=prepared`，stopped probe、备份、Gate 和最终只读复核均通过，尚未 switch。首次 run 因后续设置哈希漂移已失效，第二次因备份前磁盘预算不足停止；二者均禁止交接。本文旧交接段、旧 run 和旧命令仅用于审计，以本节及文末最新记录为准。

## 项目身份

- 上游：`https://github.com/Wei-Shaw/sub2api.git`
- 目标 fork：`F:\MySub2\sub2api`
- 旧二开输入：`F:\Sub2Api\SubNexus`
- 当前迁移分支：`feature/subnexus-migration`
- 目标 fork `main`：`d596d0844`（保持不变）
- 最新上游基线：`upstream/main=ab99d56e9626e6cd731592dae8553c9758a0efa2`（版本 `0.2.1`，发布标签 `v0.2.1=578785ee7fb35030b094b69624efe25670a36f5f`）
- 当前迁移分支：`feature/subnexus-migration`；应用候选固定为 `b1ed483ea5fc648cb3c15fcf2e7040e68a151a41`，部署包装器提交为 `33d43615c6e17e3f2ae5429f986ad636e971b8cb`。当前分支 tip 必须以 `git rev-parse HEAD` 实时核对；后续部署脚本/文档提交不会改变已固定的应用镜像；`main` 未修改。
- 旧二开参考 HEAD：`62ea35e1c78416fd83e1e41bbb310b307941811a`，分支 `alignment/v0.1.181-local`
- 两仓库没有 Git merge-base，不能使用整体 merge、整体覆盖或直接 cherry-pick 作为迁移策略。

## 当前状态

| 状态项 | 当前值 |
| --- | --- |
| 迁移阶段 | 既有迁移及上游 v0.2.1 已发布；本轮仅发布默认首页 UI。UI 候选构建、Docker gate、首页资源门禁、在线 prepare、stopped probe 和最终只读复核均通过；尚未 switch |
| 业务代码迁移 | F01-F13 已接入目标后端、前端、路由、Wire、设置和测试；所有迁移功能默认关闭 |
| 新 fork 数据库迁移 | 已新增 `9001`–`9013` 共 13 个业务/兼容 SQL；runner 有 27 组显式旧文件名接管门禁（23 组内容映射、2 组语义接管、2 组独立表接管） |
| 生产数据库访问 | 第二次候选启动约 35 秒并于 `2026-09-05 01:17:03 UTC` 应用 `9001`-`9013`；13 条 checksum 与候选 SQL 全部一致。自动回滚未恢复数据库，旧应用已在迁移后同库上恢复健康；未手工执行迁移或恢复 PostgreSQL/Redis |
| 生产部署/切换 | 上一批 v0.2.1 run `20260905114022-4163123` 实际已 switched，当前线上容器 ID 前缀 `9753053d8bd9`；本轮最终 UI run `20260905163754-200276` 已 prepared，尚未切换；首次和第二次 UI run 均不可交接 |
| 生产开关 | 本轮未开启功能、修改首页配置、执行数据库恢复或修改 Nginx；UI 包装器的设置校验和恢复流程以本轮 manifest 为准，不能把历史 closed snapshot 直接用作本轮输入 |
| 工作区 | UI 提交 `b1ed483ea5fc648cb3c15fcf2e7040e68a151a41` 与部署包装器提交 `33d43615c6e17e3f2ae5429f986ad636e971b8cb` 已推送；本轮候选 image=`sha256:32f14750ce73da00dc4c5146b1d9ad6c4420ee2c3dffe098798e41a123c6bd2c` |
| 当前磁盘 | 第二次 prepare 在 `18797457408 < 23715311616` bytes 时被预算门禁拒绝。校验并精确删除首次失效 run 的三个大备份及 sidecar 后，最终 prepare 通过并继续保持 8 GiB 保留；未使用 prune |
| 本地测试产物 | 生产备份位于 `F:\MySub2\production-backups`；PostgreSQL 18.4 隔离集群位于 `F:\MySub2\.production-restore-20260903T073714Z` 并仅监听 `127.0.0.1:56418`，当前用于 Release Gate；均未纳入 Git且不属于生产资产 |

线上服务器的最后历史快照记录在旧项目记忆中，必须用实时服务器检查覆盖，不能直接当作当前事实。特别是旧文档中的 `/www/wwwroot/SubNexus`、`/www/source/SubNexus`、端口 `18080`、root SSH 和 `main` 分支不是当前 OVH 部署的默认值。

## 功能裁决

### 迁移保留

签到/签到奖励、排行榜、活动中心 `custom` 卡片、公告/跑马灯扩展、首充礼包、二开邀请活动奖励、发票事务系统、Battle Pass、学生充值优惠、注册 IP 冷却、Channel Monitor V3、默认语言和客服按钮/Markdown 弹窗。

### 明确排除

每日消耗转盘、红包雨、运行日历、Media Studio，以及与 Media Studio 等同的 Creative Workshop/创意工坊。排除项不得复制代码、迁移、入口或恢复旧表。

### 以上游为准

Model Plaza、Grok/XAI、插件系统、Composite 路由、Affiliate 基础能力、支付基础接口、批量生图、Prompt Audit/`securityaudit`、中文验证码、Spark Shadow，以及目标 fork 已有的账号、网关、调度实现。旧项目同名代码只能用于差异审计，不能整模块替换目标实现。

## 技术架构速览

- 后端：Go、Gin、Ent 生成模型、原始 SQL repository、Redis、Wire 依赖注入。
- 前端：Vue 3、Vite、TypeScript、Pinia、vue-router、vue-i18n、Axios、TailwindCSS。
- 数据：PostgreSQL + Redis；迁移由 `backend/internal/repository/migrations_runner.go` 启动时自动执行。
- 迁移追踪：`schema_migrations(filename, checksum, applied_at)`；checksum 为 `SHA256(TrimSpace(SQL))`。
- 事务规则：普通 `.sql` 在事务中执行；`*_notx.sql` 仅用于并发索引等明确非事务场景。
- 同库兼容：27 组显式旧文件名 alias 仅在目标记录缺失、旧/目标 checksum 和数据库对象契约全部通过时处理；23 组只补写目标记录，`189/226` 两组按审计过的事务 replay 规则执行，学生优惠/注册冷却各有独立表契约，失败时拒绝启动。
- 核心入口：`backend/internal/server/router.go`、`backend/internal/server/routes/`、`backend/internal/handler/`、`backend/internal/service/`、`backend/internal/repository/`。
- 前端开关入口：`frontend/src/utils/featureFlags.ts`、`frontend/src/stores/app.ts`、`frontend/src/components/layout/AppSidebar.vue`、公共设置 API。

## 默认关闭标准

每个迁移功能必须有后端设置，默认值为 `false`；公共设置和前端入口同步关闭；关闭时只读接口可以稳定返回 `200` 携带 `enabled:false`/空列表，写接口返回约定的禁用错误（通常为 `403` 或 `404`），任何情况下不写业务数据；定时任务、队列、通知和奖励发放 no-op；关闭状态不能改变上游原有行为。开关名必须先搜索目标代码，避免与已有上游设置冲突。

当前独立开关（均默认关闭；缺失/非法按关闭处理）：

```text
subnexus_checkin_enabled
subnexus_leaderboard_enabled
subnexus_activity_center_enabled
subnexus_marquee_enabled
subnexus_first_recharge_enabled
subnexus_invite_rewards_enabled
subnexus_invite_activities_enabled
subnexus_invoice_enabled
battle_pass_enabled
subnexus_student_recharge_benefit_enabled
registration_ip_cooldown_enabled
```

邀请活动还要求 `subnexus_invite_activities_enabled` 与对应子开关同时为字面量 `true`；Channel Monitor V3 复用 `channel_monitor_enabled` 并要求 `channel_monitor_mode=v3`，缺失/非法模式回退 `v1`。默认语言和客服设置使用 namespaced/legacy 双键，空值或非法值不产生行为。

## 数据与回滚原则

1. 新 fork 上游已有迁移以上游文件为准；已执行文件不可改名、修改、删除或复用编号。
2. 新迁移必须使用目标仓库中全局唯一的文件名，优先新增独立表和可选字段，不删除/重命名核心表字段。
3. 代码回滚优先于数据库恢复。新增表和可选字段必须让旧版本可忽略；数据库恢复只在确认数据层损坏并获得明确批准后进行。
4. 本地开发先使用空库和旧项目迁移构造的隔离副本；Batch 1-5 全部完成后，Release Gate 再使用线上备份恢复出的隔离副本。任何本地候选都不能直接连接生产数据库启动。
5. 线上切换时旧版本容器、镜像、源码、配置、PostgreSQL 备份、Redis 备份和回滚脚本长期保留，由维护者后续自行删除。

## 生产环境已知边界（历史快照，需实时复核）

旧项目线上文档最后记录：OVH `51.81.211.97`，SSH 用户 `ubuntu`；应用容器 `subnexus-cutover`，本地端口 `127.0.0.1:18083 -> 8080`；PostgreSQL 容器 `sub2api-postgres`；Redis 容器 `sub2api-redis`；Docker 网络 `sub2api-net`；源码工作树 `/srv/subnexus-repo`；证据/部署目录 `/srv/subnexus-migration`。这些值只能用于生成盘点命令，不能跳过实时 `docker inspect`、Nginx 有效配置和数据库查询。

旧项目的生产工具（`tools/production-deploy/`）绑定旧提交、旧分支和旧迁移 checksum，不能直接用于本 fork。后续应新建目标侧只读 preflight、迁移 apply、候选 cutover 和回滚工具，复用其安全思想而不是其参数或迁移列表。

## 维护规则

- 任何操作前检查 `git status --short --branch`，不得覆盖用户已有改动。
- 每次操作完成后立即追加 `SUBNEXUS_CHANGE_MEMORY.md`，记录时间、目的、风险、命令/文件、结果、未完成项和回滚点。
- 每个迁移批次使用独立提交；提交前检查 `git diff --check`、敏感信息、依赖清单、`frontend/pnpm-lock.yaml`、VERSION 和生成产物。
- 旧项目 `F:\Sub2Api\SubNexus` 永久只读；所有写入只允许发生在 fork。维护者完成本地验收前只创建本地提交，不再推送，不执行服务器命令。
- 未经明确批准不执行生产迁移、部署、切换、开关开启、依赖安装或删除操作。
- 最新授权允许代理完成安装脚本、全新备份/`prepare`、never-started probe 验收及范围明确的无用垃圾清理；仅最终 `switch` 和 `rollback` 必须停下交给维护者手动执行。任何历史失败 run 不得重试或复用。
- 不记录密码、Token、API Key、Cookie、JWT/TOTP secret、私钥或完整 `.env`；日志和证据必须脱敏。
- 前端验证优先使用冻结 lockfile；目标 Docker 使用 pnpm 9，禁止因本地 pnpm 版本差异重写 lockfile。

## 下一步入口

1. 已完成线上只读 preflight 和生产 PostgreSQL/Redis/应用数据备份结构校验；服务器备份目录为 `/srv/subnexus-migration/backups/20260903T073714Z`，所有 SHA256 均通过。
2. 备份已下载并通过 20 个文件 SHA256；PostgreSQL 18.4 原始恢复库、真实克隆 migration/adoption、候选全部关闭态、旧版回归和 Redis 8.8.0 RDB 隔离加载均通过。Redis 证据位于台账记录的 root-only 路径。
3. 最终 run `/srv/subnexus-migration/cutover/20260905163754-200276` 的备份、manifest、固定旧回滚对象、全量设置快照与 stopped probe 已通过，最终只读复核确认生产健康；尚未 switch。首次/第二次 UI run 禁止复用，最终 switch 和 rollback 由维护者执行。

## 2026-09-05 上游 v0.2.1 合并状态

- 已在 `feature/subnexus-migration` 合并上游发布标签 `v0.2.1`（代码提交 `578785ee7fb35030b094b69624efe25670a36f5f`）及其后唯一的版本同步提交 `ab99d56e9626e6cd731592dae8553c9758a0efa2`；当前合并 tip 为 `8a0c8af8534b4038e357ab8368eb027e0a489cee`。
- 合并保留现有 SubNexus 业务代码、`9001`-`9013` 迁移和项目记忆文件，并纳入上游 0.2.1 的网关、模型、定价、用量记录和前端管理能力；未修改 fork `main`、旧项目或服务器。
- 本地验证：`git diff --check`、`pnpm typecheck`、`pnpm build`、`go test ./...` 通过；单独完整运行 Vitest `286/286` 文件、`1987/1987` 测试通过。
- 当前未提交内容仍仅为首页 Rain + Glass UI 的 `frontend/src/views/HomeView.vue` 和 `frontend/public/images/rain-city-1.jpg`；不得在后续同步中覆盖。0.2.1 合并后的候选镜像、Docker gate 和线上切换尚未重新执行。

## v0.2.1 发布前历史快照（2026-09-05，已被本轮状态覆盖）

上游 `v0.2.1` 已由当前分支提交 `bb36764f692ca79ccc9c635fd71dcbb70b9c0449` 构建并完成服务器候选 gate。候选镜像为 `sha256:21098ec4f4c922efa92208b640a970eb8602778c0515e8921967f9d75dc5adfd`，归档 SHA=`d1a6e297720d7af32a1ba98a7e4d3c9dc66a72a4fe7aace1ea96d346d7b1b648`，gate evidence=`/srv/subnexus-migration/docker-candidate/20260905T113230Z-1add8fbc-85d4-4d21-af6c-23fbc34af218/evidence.txt`，SHA=`ac2cd667c1a317b3d0eab0e11a9b3cbada4b4ecec49850eba08f81263ebb2bf5`。

当时唯一可交接 run 是 `/srv/subnexus-migration/cutover/20260905114022-4163123`，该历史批次随后已 switched；其 prepare/probe 记录仅作审计，不能作为本轮入口。

## Rain + Glass UI 首次准备快照（2026-09-06 00:09 Asia/Shanghai，后续已失效）

- 应用提交=`b1ed483ea5fc648cb3c15fcf2e7040e68a151a41`，tree=`bb821e2a0003d13cd425ca8ff012dbb26f70b1a6`；仅默认首页展示和 `frontend/public/rain-city-1.jpg` 改动，原脚本与 88 项模板业务绑定不变。图片最终路径为 `/rain-city-1.jpg`，避免生产 `/images/` 网关保留路径。
- 候选镜像=`sha256:32f14750ce73da00dc4c5146b1d9ad6c4420ee2c3dffe098798e41a123c6bd2c`；归档 `/srv/subnexus-migration/candidate-artifacts/rain-b1ed483ea5fc-retry2/candidate-image.tar`，SHA256=`26422d9eaad7ede983b228e84ee756eae313347b0135bf4e2d48138912c3246b`。
- Docker gate `/srv/subnexus-migration/docker-candidate/20260905T155430Z-940fdcd9-bc3c-4d12-8c72-12ed9e27328b/evidence.txt` 和首页证据 `/srv/subnexus-migration/diagnostics/rain-home-b1ed483ea5fc.ENig2O5r` 均通过；最终证据哈希待统一登记。
- UI 包装器 `/srv/subnexus-migration/tools/subnexus-ui-cutover-eef1d8f-20260905.sh` 的 SHA256=`eef1dfa31c71cfe33096d107561c594e0b509455b65db0caec824196d1cec77d`；原控制器仍为 `subnexus-production-cutover-19824a87-20260905-v021.sh`，SHA256=`19824a87e3e1de5659cb30664750b71c5c10d374f25bda7f52e6524fe477ee65`，二者独立校验。
- 在线 prepare run=`/srv/subnexus-migration/cutover/20260905160223-175225`，PID=`175225`，最近进度为 PostgreSQL dump，尚无 READY；不能据构建/gate 通过提前宣告准备完成。
- 固定回滚对象始终为旧 SubNexus `be459424b327ad056ea9bdc02187d6a458fe09082369b354158d6e7f7758beee`，名称 `subnexus-cutover-pre-96b66b3e74c1-20260905085804-4072165`，锚定 run `20260905085804-4072165`，旧 image ID 前缀 `b24b585`。本轮不新建永久回滚对象；当前 v0.2.1 容器 `9753053d8bd9...` 不替代该旧对象。

## Rain + Glass UI 最新准备状态（2026-09-06 00:41 Asia/Shanghai）

- 首次 run `20260905160223-175225` 的 prepare 曾成功，但全量 settings 哈希随后从 `d66bf0e2c9ee6c1734bfa38cdae508e174562051e18acd093c14b81ab0e9705a` 漂移为 `af154e9a7a878bfc5295f12e88d4143c5466ab0c83939831fa13d202b71bc90a`，probe 在创建候选前安全停止，无 candidate。具体被改动键和来源未确定；全量 settings 与原控制器仅 18 键快照不能直接对应比较，后续哈希复验稳定不代表已定位原因。
- 第二次 run `20260905163008-194872` 在备份前因可用 `18797457408` bytes 小于预算 `23715311616` bytes 被拒绝。首次 run 的三个大备份及 sidecar 经 SHA 校验/记录后精确删除，manifest/settings/metadata 保留并写入 `INVALIDATED_SETTINGS_DRIFT`；该 run 不可复用。
- 清理日志 `/srv/subnexus-migration/cleanup-rain-invalid-run-20260905160223.txt`，SHA256=`c3e1af6e289292b4b2baa8b76136ea322f19556785a17caf63d6d34c2060d326`；清理后可用 `24025554944` bytes。
- 第三次在线 prepare `/srv/subnexus-migration/cutover/20260905163754-200276`（PID 200276）已 `READY=prepared`；候选镜像、脚本和固定旧回滚对象不变。stopped probe 已精确删除且生产状态未改变，尚未执行 switch/rollback。
