# SubNexus 迁移项目上下文

> 本文件是新 fork 的长期维护入口。任何 AI 或开发者在修改代码前必须先阅读本文件、`SUBNEXUS_CHANGE_MEMORY.md`、`SUBNEXUS_MIGRATION_PLAN.md` 和 `SUBNEXUS_MIGRATION_LEDGER.md`。
>
> 最后更新：2026-09-05（新脚本 `19824a87` 的 run 已 `READY=prepared`，stopped probe 验收通过，当前停在维护者人工 switch 前）

## 项目身份

- 上游：`https://github.com/Wei-Shaw/sub2api.git`
- 目标 fork：`F:\MySub2\sub2api`
- 旧二开输入：`F:\Sub2Api\SubNexus`
- 当前迁移分支：`feature/subnexus-migration`
- 目标 fork `main`：`d596d0844`（保持不变）
- 最新上游基线：`upstream/main=5097b31457e6dc9f49e5f5c9c72b925ce79543b3`（版本 `0.2.0`）
- 当前迁移分支：`feature/subnexus-migration`；当前分支 tip 必须以 `git rev-parse HEAD` 实时核对（最新网络身份修复提交为 `ca2139d1e70877fba8a41e1410e4d7d29b4ef9c0`）；应用功能候选提交：`02774d028d076e934a59f04fd1ee98598ac693a1`（镜像与 gate 均由此提交构建）；`main` 未修改
- 旧二开参考 HEAD：`62ea35e1c78416fd83e1e41bbb310b307941811a`，分支 `alignment/v0.1.181-local`
- 两仓库没有 Git merge-base，不能使用整体 merge、整体覆盖或直接 cherry-pick 作为迁移策略。

## 当前状态

| 状态项 | 当前值 |
| --- | --- |
| 迁移阶段 | Batch 0、Batch 1-5 本地验证、线上只读预检/历史备份、PostgreSQL 18.4 恢复、Redis 8 RDB 隔离恢复、真实克隆 migration/adoption、候选关闭态、旧版回归和 Docker runtime gate 已完成；新脚本 `19824a87` 已完成无停机 prepare 并生成 `READY=prepared`，stopped probe 验收通过，当前停在人工 switch 前 |
| 业务代码迁移 | F01-F13 已接入目标后端、前端、路由、Wire、设置和测试；所有迁移功能默认关闭 |
| 新 fork 数据库迁移 | 已新增 `9001`–`9013` 共 13 个业务/兼容 SQL；runner 有 27 组显式旧文件名接管门禁（23 组内容映射、2 组语义接管、2 组独立表接管） |
| 生产数据库访问 | 第二次候选启动约 35 秒并于 `2026-09-05 01:17:03 UTC` 应用 `9001`-`9013`；13 条 checksum 与候选 SQL 全部一致。自动回滚未恢复数据库，旧应用已在迁移后同库上恢复健康；未手工执行迁移或恢复 PostgreSQL/Redis |
| 生产部署/切换 | 新 run `20260905055413-3958448` 已由维护者成功 `switch`，窗口 42 秒；新应用 healthy，旧容器已保留供同批次 rollback。四次历史失败 run 禁止复用 |
| 生产开关 | 自动回滚已恢复旧应用切换前设置；候选下次启动前仍由 closed snapshot 强制关闭迁移功能。不要把旧应用当前既有的 Channel Monitor/客服设置状态误写为候选默认开启 |
| 工作区 | 修复提交 `fbca62fbccb5a783d8d35cb9dcc4025cdb1c4a44` 已推送；脚本 SHA=`19824a87e3e1de5659cb30664750b71c5c10d374f25bda7f52e6524fe477ee65`，测试 SHA=`7e981ff118b795b40b38d22eb0a09667d7ac25977d9f17c5a30590dccece9763`；应用候选仍固定为 `02774d028d076e934a59f04fd1ee98598ac693a1` |
| 当前磁盘 | 2026-09-05 14:11:57 Asia/Shanghai 只读可用 `35573174272` bytes（约 35.57 GB），继续保持 8 GiB 保留，不复用旧备份 |
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
3. Docker 候选门禁仍有效；历史失败 run 均已自动回滚并禁止复用。新 run `/srv/subnexus-migration/cutover/20260905055413-3958448` 已 `READY=prepared`，备份与 manifest 哈希已记录；stopped probe 验收通过且无残留。当前停在维护者人工 switch 前；此前旧 SHA、旧命令和旧 run 不得重试。
