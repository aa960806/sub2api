# SubNexus 迁移项目上下文

> 本文件是新 fork 的长期维护入口。任何 AI 或开发者在修改代码前必须先阅读本文件、`SUBNEXUS_CHANGE_MEMORY.md`、`SUBNEXUS_MIGRATION_PLAN.md` 和 `SUBNEXUS_MIGRATION_LEDGER.md`。
>
> 最后更新：2026-09-02（执行顺序改为本地完成 Batch 1-5 后再推送和进入生产 Release Gate）

## 项目身份

- 上游：`https://github.com/Wei-Shaw/sub2api.git`
- 目标 fork：`F:\MySub2\sub2api`
- 旧二开输入：`F:\Sub2Api\SubNexus`
- 当前迁移分支：`feature/subnexus-migration`
- 目标分支基线：`d596d0844`（新 fork `main`，创建迁移分支时的 HEAD）
- 旧二开参考 HEAD：`ccffee6c6`，分支 `alignment/v0.1.181-local`
- 两仓库没有 Git merge-base，不能使用整体 merge、整体覆盖或直接 cherry-pick 作为迁移策略。

## 当前状态

| 状态项 | 当前值 |
| --- | --- |
| 迁移阶段 | 本地优先：Batch 0 控制和 adoption 门禁已完成；正在同步最新上游并准备 Batch 1 |
| 业务代码迁移 | Batch 1 准备中；生产证据不再阻塞本地实现 |
| 新 fork 数据库迁移 | 未新增业务迁移；runner 已增加 25 组显式旧文件名接管门禁（23 组精确映射 + 2 组语义接管） |
| 生产数据库访问 | 本任务尚未执行 |
| 生产部署/切换 | 未执行 |
| 生产开关 | 未修改 |
| 工作区 | Batch 0 文档、预检脚本与 adoption 代码均在迁移分支维护；业务依赖、前端 lockfile 和 VERSION 未改 |

线上服务器的最后历史快照记录在旧项目记忆中，必须用实时服务器检查覆盖，不能直接当作当前事实。特别是旧文档中的 `/www/wwwroot/SubNexus`、`/www/source/SubNexus`、端口 `18080`、root SSH 和 `main` 分支不是当前 OVH 部署的默认值。

## 功能裁决

### 迁移保留

签到/签到奖励、排行榜、活动中心中仍未被上游覆盖的活动能力、公告/跑马灯扩展、首充礼包、二开邀请活动奖励、发票事务系统、Battle Pass。

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
- 同库兼容：25 组显式旧文件名 alias 仅在目标记录缺失、旧/目标 checksum 和数据库对象契约全部通过时处理；23 组只补写目标记录，`189/226` 两组按审计过的事务 replay 规则执行，失败时拒绝启动。
- 核心入口：`backend/internal/server/router.go`、`backend/internal/server/routes/`、`backend/internal/handler/`、`backend/internal/service/`、`backend/internal/repository/`。
- 前端开关入口：`frontend/src/utils/featureFlags.ts`、`frontend/src/stores/app.ts`、`frontend/src/components/layout/AppSidebar.vue`、公共设置 API。

## 默认关闭标准

每个迁移功能必须有后端设置，默认值为 `false`；公共设置和前端入口同步关闭；关闭时 API 只返回稳定的禁用错误/约定 404，不写数据；定时任务、队列、通知和奖励发放 no-op；关闭状态不能改变上游原有行为。开关名必须先搜索目标代码，避免与已有上游设置冲突。

拟定的独立开关（实现前仍需核对）：

```text
subnexus_checkin_enabled
subnexus_leaderboard_enabled
subnexus_activity_center_enabled
subnexus_marquee_enabled
subnexus_first_recharge_enabled
subnexus_invite_rewards_enabled
invoice_enabled       # 若目标已有则复用，不重复创建
battle_pass_enabled   # 若目标已有则复用，不重复创建
```

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
- 不记录密码、Token、API Key、Cookie、JWT/TOTP secret、私钥或完整 `.env`；日志和证据必须脱敏。
- 前端验证优先使用冻结 lockfile；目标 Docker 使用 pnpm 9，禁止因本地 pnpm 版本差异重写 lockfile。

## 下一步入口

1. 在迁移分支整合最新 `upstream/main`，运行更新后的上游基线测试，保持 fork `main` 和旧项目不变。
2. 本地依次完成 Batch 1-4，并在 Batch 5 完成全量后端、前端、PostgreSQL/Redis、Docker 和旧版本回滚验证；所有开关默认关闭。
3. 维护者验收本地候选后才推送迁移分支并固定 release SHA，随后才允许服务器拉取和执行生产只读 preflight。
4. Release Gate 取得可恢复 PostgreSQL/Redis 备份并完成隔离恢复、adoption 和旧版本回归后，才进入受控切换。
