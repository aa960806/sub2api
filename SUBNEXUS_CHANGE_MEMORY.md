# SubNexus 操作与变更记忆

> 这是追加式项目记忆，不得重写或删除历史条目。每次代码、测试、配置、迁移、文档、部署或诊断操作完成后，必须在本文件末尾追加一条记录。
>
> 详细当前架构见 `SUBNEXUS_PROJECT_CONTEXT.md`；批次状态见 `SUBNEXUS_MIGRATION_LEDGER.md`。

## 记录规则

- 时间使用 `Asia/Shanghai`，同时写明是否访问线上环境。
- 记录真实执行的命令类别和目标路径，但绝不记录密码、Token、API Key、Cookie、JWT/TOTP secret、私钥或完整环境变量。
- 明确区分“计划”“已执行”“已验证”“未验证”“阻塞”；静态检查不能代替运行时证据。
- 每条记录写出触碰文件、测试结果、生产状态、回滚点和下一步恢复入口。
- 发现旧记忆与实时状态冲突时，不删除旧记录，在新条目中说明冲突和取信依据。

## 2026-09-01（Asia/Shanghai）— 启动新 fork 的 SubNexus 迁移 Batch 0

### 请求与目标

- 维护者要求将 `F:\Sub2Api\SubNexus` 的保留二开功能迁移到 `F:\MySub2\sub2api`，以新 fork 上游实现为准，最终同库切换并保留旧版本回滚能力。
- 明确排除每日消耗转盘、红包雨、运行日历、Media Studio；Creative Workshop 已确认与 Media Studio 等同，也排除。
- 要求所有迁移功能默认关闭，并在项目本地建立可供后续 AI 快速理解的记忆文档。

### 本次已执行操作

- 确认目标仓库 `main` HEAD 为 `d596d0844`，工作区原本干净。
- 从 `main` 创建分支 `feature/subnexus-migration`，未修改 `main`。
- 创建并更新 `SUBNEXUS_MIGRATION_PLAN.md` 至 v1.2。
- 读取旧项目 `AI_PROJECT_CONTEXT.md`、`AI_CHANGE_MEMORY.md`、生产切换/OVH/部署工具文档。
- 对旧、新仓库执行只读路径和迁移文件盘点；确认两仓库无 merge-base，不能整体覆盖或直接 cherry-pick。
- 创建本文件、`SUBNEXUS_PROJECT_CONTEXT.md`、`SUBNEXUS_MIGRATION_LEDGER.md`、`SUBNEXUS_FEATURE_MATRIX.md`（本条目随文档创建完成后补充最终路径）。
- 启动两个只读调查子任务：功能映射和目标架构审计；子任务禁止修改文件。

### 事实与结果

- 旧项目参考分支 `alignment/v0.1.181-local` / HEAD `ccffee6c6` / 应用版本 `0.1.135` / Go `1.26.6`。
- 新 fork `main` / HEAD `d596d0844` / 应用版本 `0.1.185` / Go `1.27.0`。
- 旧项目含活动、发票和 Battle Pass 实现；目标 fork 当前不存在对应的 `activity_service.go`、`invoice_service.go`、`battle_pass.go` 和 `254_battle_pass.sql`。
- 目标 fork 已有不同上游迁移集合，并存在重复数字前缀；后续不能复用旧的 151/210/254 文件名。
- 生产服务器尚未访问；线上迁移状态、实际开关和表存在性仍为待查询状态。

### 安全边界

- 本次未执行生产 SQL、未连接生产 PostgreSQL/Redis、未部署、未切换流量、未开启功能。
- 未修改业务代码、依赖、`frontend/pnpm-lock.yaml`、VERSION 或目标迁移目录。
- 未记录任何敏感凭据。

### 验证

- `git status --short --branch` 确认当前分支为 `feature/subnexus-migration`。
- `git diff --check` 通过（当前新增文档为未跟踪文件，不产生业务 diff）。

### 回滚点与下一步

- 文档阶段回滚点：目标分支基线 `d596d0844`；删除本批新增文档即可回到未开始迁移状态，但不得使用破坏性 Git 命令覆盖用户文件。
- 下一步：完成本地功能矩阵和目标架构盘点，向维护者提供基于当前 OVH 拓扑的只读 preflight 命令；收到脱敏结果后再进入隔离数据库演练。

## 2026-09-01（Asia/Shanghai）— Batch 0 本地预检资产与基线测试

### 目的与授权

- 目的：按迁移规划补齐目标 fork 专用的只读线上预检资产、同库切换手册和回滚手册，并验证上游基线重点包。
- 是否得到维护者明确授权：是（开始迁移并建立持续记忆）；本次仅执行本地读写和本地测试。
- 是否访问线上：否。

### 变更/命令

- 分支与基线：`feature/subnexus-migration`，基线 `d596d0844`。
- 触碰文件：`tools/production-deploy/subnexus-readonly-preflight.sh`、`SUBNEXUS_CUTOVER_RUNBOOK.md`、`SUBNEXUS_ROLLBACK_RUNBOOK.md`、`SUBNEXUS_MIGRATION_PLAN.md`、`SUBNEXUS_MIGRATION_LEDGER.md`、`SUBNEXUS_PROJECT_CONTEXT.md`。
- 执行的命令类别（脱敏）：Git 状态/历史检查；旧新仓库路径和迁移文件只读盘点；Git Bash `bash -n` 脚本语法检查；Go 单元测试（`GOTMPDIR`/`GOCACHE` 指向 `F:\MySub2`）。
- 数据库迁移/开关/部署动作：未执行；预检脚本设计为只读，不运行迁移、备份、DDL/DML、重启或切流。

### 验证结果

- 测试/构建/静态检查：`internal/config`、`internal/repository`、`internal/server/routes` 重点单测通过；设置/公告/支付相关 service 定向单测通过；预检脚本 `bash -n` 通过。
- 运行时或线上证据：无；Docker 本地集成环境当前不可用，线上 OVH 预检待维护者执行。
- 未验证项目：完整后端测试、前端 typecheck/Vitest/build、隔离 PostgreSQL/Redis 恢复演练、线上 `schema_migrations`/`atlas_schema_revisions` 和文件存储状态。

### 风险、回滚与下一步

- 风险：目标 fork 与旧项目迁移记录可能存在同库差异；在取得真实记录前不能创建或应用业务迁移。
- 回滚点/回滚命令：文档/脚本阶段回滚点仍为 `d596d0844`；不执行破坏性 Git 回滚，保留所有用户文档改动。
- 下一步：请维护者在当前 OVH 服务器执行 `SUBNEXUS_CUTOVER_RUNBOOK.md` 第 2 节的只读预检命令，脱敏回传证据；同时完成 Batch 1 最终代码映射后，再设计第一个新迁移文件。

## 后续记录模板

```text
## YYYY-MM-DD（Asia/Shanghai）— <批次/操作标题>

### 目的与授权
- 目的：
- 是否得到维护者明确授权：
- 是否访问线上：是/否（若是，写明只读或变更）

### 变更/命令
- 分支与基线：
- 触碰文件：
- 执行的命令类别（脱敏）：
- 数据库迁移/开关/部署动作：

### 验证结果
- 测试/构建/静态检查：
- 运行时或线上证据：
- 未验证项目：

### 风险、回滚与下一步
- 风险：
- 回滚点/回滚命令：
- 下一步：
```
