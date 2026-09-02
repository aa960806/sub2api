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

## 2026-09-01（Asia/Shanghai）— Batch 0 控制资产独立提交

### 目的与授权

- 目的：按照每批独立提交规则固定本地 Batch 0 文档、只读预检脚本和切换/回滚手册。
- 是否得到维护者明确授权：是；未涉及线上变更。
- 是否访问线上：否。

### 变更/命令

- 分支与基线：`feature/subnexus-migration`；提交 `88e214541`（`docs: establish SubNexus migration controls`）。
- 触碰文件：前一条记录列出的 8 个 Batch 0 文档/工具文件。
- 执行的命令类别（脱敏）：`git add`、可执行位校正、`git diff --cached --check`、`git commit`、提交后状态核对。
- 数据库迁移/开关/部署动作：无。

### 验证结果

- 测试/构建/静态检查：提交前 staged diff check 通过；Git Bash `bash -n` 和重点 Go 单测结果见上一条记录。
- 运行时或线上证据：无；等待维护者运行目标 fork 只读 preflight。
- 未验证项目：线上容器拓扑、数据库迁移记录、Redis 持久化、隔离库恢复和业务批次。

### 风险、回滚与下一步

- 风险：预检尚未执行前不得创建/应用 Batch 1 迁移；历史生产路径不能当作实时事实。
- 回滚点/回滚命令：文档阶段可回到提交 `d596d0844`；保留 `88e214541` 作为审计节点，不使用破坏性命令。
- 下一步：维护者执行 `tools/production-deploy/subnexus-readonly-preflight.sh` 并回传脱敏证据；本地继续收敛 Batch 1 代码映射但不接入生产。

## 2026-09-01（Asia/Shanghai）— Batch 0 完成本地基线验证与功能映射收敛

### 目的与授权

- 目的：完成规划要求的本地上游基线构建/测试和保留功能的只读映射，确认是否具备进入线上预检的前置证据。
- 是否得到维护者明确授权：是（维护者要求开始严格执行迁移并持续记录）；本次没有生产变更授权，也未申请生产写入。
- 是否访问线上：否。

### 变更/命令

- 分支与基线：`feature/subnexus-migration`；目标基线 `d596d0844`；当前文档控制提交随后由独立提交保留。
- 触碰文件：`SUBNEXUS_MIGRATION_LEDGER.md`、`SUBNEXUS_FEATURE_MATRIX.md`、本记忆文件；依赖安装仅生成被忽略的 `frontend/node_modules` 和本地构建产物。
- 执行的命令类别（脱敏）：在 `backend` 目录设置 `GOTMPDIR=F:\MySub2\.gotmp`、`GOCACHE=F:\MySub2\.gocache` 后运行 `go test ./... -run '^$' -count=1 -p=1`；在 `frontend` 运行 `pnpm install --frozen-lockfile --ignore-scripts`、`pnpm typecheck`、`pnpm test:run`、`pnpm build`；运行 Git 状态、diff check、锁文件 SHA256 和只读路径盘点。
- 数据库迁移/开关/部署动作：无；未连接生产 PostgreSQL/Redis，未执行 SQL、DDL/DML、迁移、部署、切流或开关开启。

### 验证结果

- 测试/构建/静态检查：后端编译级基线退出码 0；前端 typecheck 退出码 0；Vitest `249` 个文件、`1804` 个测试全部通过；Vite 生产构建退出码 0；`frontend/pnpm-lock.yaml` SHA256 为 `8DBD1876020E41B644D971414D29100C9F428F39EDE953C03D0442B834F6F3AF` 且无 diff；已有 Browserslist、Vite chunk 和测试 mock 警告未导致失败。
- 运行时或线上证据：无。功能映射确认目标没有活动、发票、Battle Pass 对应代码/表/路由；目标有公告、Affiliate、支付和用量基础，按矩阵以上游为准。
- 未验证项目：线上容器/网络/端口/Nginx 实时状态，`schema_migrations` 与 `atlas_schema_revisions` 实际记录，生产备份/Redis 恢复点，隔离 PostgreSQL/Redis 恢复演练，所有迁移功能的关闭态和开启态业务测试。

### 风险、回滚与下一步

- 风险：目标和旧项目没有 Git merge-base；目标迁移文件存在重复数字前缀，且生产 schema 未知。不能据历史编号或旧文档推断可用迁移名，也不能在生产库直接做本地启动测试。
- 回滚点/回滚命令：本次仅文档/依赖缓存变化；代码基线回滚点为 `d596d0844`，保留已提交的 Batch 0 控制资产和历史记忆，不使用破坏性 Git 命令。
- 下一步：维护者在当前 OVH 主机运行 `tools/production-deploy/subnexus-readonly-preflight.sh`，脱敏回传 `evidence.txt`；取得线上迁移记录和可恢复备份后，在隔离库验证候选迁移，再开始 Batch 1 代码与新唯一迁移文件。

## 2026-09-01（Asia/Shanghai）— 加固线上只读预检脚本

### 目的与授权

- 目的：根据目标迁移 runner 和旧二开设置审计，修正线上预检遗漏，确保同库切换前能看到完整历史迁移与 Atlas 状态。
- 是否得到维护者明确授权：是（属于迁移前本地工具和文档工作）；未获生产写入授权。
- 是否访问线上：否。

### 变更/命令

- 分支与基线：`feature/subnexus-migration`；修正提交为 `6ba6c5bd4`。
- 触碰文件：`tools/production-deploy/subnexus-readonly-preflight.sh`、`SUBNEXUS_CUTOVER_RUNBOOK.md`。
- 执行的命令类别（脱敏）：目标迁移 runner/旧项目设置 key 只读检索；Git Bash `bash -n`；`git diff --check`、独立提交和状态核对。
- 数据库迁移/开关/部署动作：无。脚本仅在未来线上执行 SELECT/Redis 读命令，并只写指定证据目录。

### 验证结果

- 测试/构建/静态检查：脚本 Git Bash 语法检查退出码 0；新增 `PGOPTIONS` 强制 PostgreSQL 默认只读；全量 `schema_migrations`、Atlas revision 最新行、旧 `ACTIVITY_CONFIG`/`INVOICE_CONFIG` 等 key 摘要和 Nginx/存储摘要已纳入。
- 运行时或线上证据：无，线上脚本尚未执行。
- 未验证项目：生产容器环境变量是否使用真实容器名而非 Docker 服务别名、Nginx 权限、数据库实际列/记录和 Redis 认证仍待维护者执行。

### 风险、回滚与下一步

- 风险：生产 `DATABASE_HOST`/`REDIS_HOST` 可能是网络别名，脚本会按真实 Docker 容器 inspect 并在不匹配时停止；这是保护性失败，不应绕过。证据目录只能使用专用子目录。
- 回滚点/回滚命令：脚本/文档修正前提交 `00a54e53e`；不修改或删除任何历史迁移和数据。
- 下一步：维护者从可访问的目标分支脚本运行只读 preflight，脱敏回传证据；根据真实记录确定新迁移全局唯一文件名，再进入隔离库演练。

## 2026-09-01（Asia/Shanghai）— 推送迁移分支并准备线上只读预检

### 目的与授权

- 目的：让维护者可以从固定不可变提交取得预检脚本，在当前 OVH 环境采集同库切换所需的实时证据。
- 是否得到维护者明确授权：是（开始执行迁移）；本次远端操作只新增迁移分支，不修改 `main` 或生产环境。
- 是否访问线上：否。

### 变更/命令

- 分支与基线：本地 `feature/subnexus-migration` HEAD `402d0b0e473bd6c0b8bc80a815a7da335e0a0c5a`；已推送为 `origin/feature/subnexus-migration`。
- 触碰文件：无新增代码；远端分支包含既有 Batch 0 文档、手册和预检脚本。
- 执行的命令类别（脱敏）：`git ls-remote` 检查远端、`git push -u origin feature/subnexus-migration`；脚本文件 SHA256 计算。
- 数据库迁移/开关/部署动作：无。

### 验证结果

- 测试/构建/静态检查：远端分支指向固定提交；预检脚本 SHA256=`ECB985233881E3C20BD20B8D394275D35F50AF1F344EBFADDB1BF13AA9A02E84`，Git Bash 语法检查此前退出码 0。
- 运行时或线上证据：无；等待维护者在真实主机执行。
- 未验证项目：服务器实际容器名、应用端口、数据库/Redis 网络别名、Nginx 配置、迁移记录、备份和文件存储。

### 风险、回滚与下一步

- 风险：远端分支是可继续提交的开发分支；线上执行必须使用完整提交 SHA 下载并先校验脚本哈希。历史拓扑仅作默认参数提示，必须以 `docker ps/inspect` 结果为准。
- 回滚点/回滚命令：不涉及生产状态；删除远端迁移分支需维护者另行决定，本地仍保留 `d596d0844` 基线。
- 下一步：在服务器执行下方固定提交的只读 preflight，脱敏回传证据；在证据和隔离备份完成前不创建/应用 Batch 1 迁移。

## 2026-09-01（Asia/Shanghai）— 改名迁移静态审计与 adoption 门禁

### 目的与授权

- 目的：确认目标 fork 与旧二开项目是否存在“SQL 内容相同但迁移文件名不同”的同库启动风险，并把处理规则固化到迁移规划与台账。
- 是否得到维护者明确授权：是（属于已授权的迁移前审计和安全门禁）；未获生产写入授权。
- 是否访问线上：否；仅读取本地两个仓库和目标迁移 runner。

### 变更/命令

- 分支与基线：`feature/subnexus-migration`，基于目标 `main` `d596d0844`；本次为文档审计，未修改 `main`。
- 触碰文件：`SUBNEXUS_MIGRATION_PLAN.md`、`SUBNEXUS_MIGRATION_LEDGER.md`、本记忆文件（追加本条）。
- 执行的命令类别（脱敏）：逐文件读取两仓库迁移 SQL；按目标 runner 的 `TrimSpace(SQL)` 规则计算和比较 SHA256；读取 `applyMigrationsFS` 与现有 checksum 兼容测试；执行 `git diff --check`。
- 数据库迁移/开关/部署动作：无；未执行 SQL、DDL/DML、备份、重启、部署、切流或开关开启。

### 事实与结果

- 发现 23 组 old filename→target filename 的同内容迁移。目标 runner 只按完整文件名查询 `schema_migrations`，同库启动会把目标文件误判为未执行。
- 风险类别包括 `INSERT`、`UPDATE`、`DELETE`、`TRUNCATE`、触发器/函数重建和 `CREATE INDEX CONCURRENTLY`；其中部分 SQL 重跑会覆盖人工设置、改变数据或产生长时间扫描。
- 发现唯一同名但 checksum 不同的历史迁移为 `181_group_duplicate_operation_id.sql`：旧 checksum `cf273ce97ebbd045636fdc724f2c284e8258b7049fdb630e6e6bb1606749f828`，目标 checksum `429011c514dfa3a65dd844cb19dfe32ceeae4068f499b15f915cee97687ed7bd`；差异目前确认包含注释，仍需隔离库确认后决定窄兼容规则或字节兼容。
- 处理规则已写入规划 6.1.1：仅允许显式 alias/adoption；必须同时核对旧文件名、旧 checksum、目标 checksum 和对象契约；旧记录 hash 不符、对象不符或未知映射时硬失败；禁止全局同 checksum 跳过、手工删除记录或关闭 checksum 校验。

### 验证结果

- 本地静态审计和文档 `git diff --check`：通过。
- 运行时或线上证据：无；23 组映射的生产执行状态、对象定义和可安全 adoption 仍待线上记录及隔离数据库验证。
- 未验证项目：线上 `schema_migrations`/Atlas 记录、索引有效性与定义、触发器/函数定义、备份恢复和旧版本回滚。

### 风险、回滚与下一步

- 风险：在 B0-5/B0-6/B0-7 完成前直接启动候选版本可能重复执行历史 DML 或索引迁移，造成数据/锁风险。
- 回滚点/回滚命令：本次仅文档变更；可回到父提交 `c836c2632` 或代码基线 `d596d0844`，不使用破坏性 Git 命令。
- 下一步：先由维护者执行固定提交的只读 preflight 并脱敏回传；取得可恢复备份后，在隔离克隆验证 alias/adoption、同名 checksum 差异和旧版本回滚，再进入 Batch 1 代码迁移。

## 2026-09-01（Asia/Shanghai）— 静态审计分类更正

- 依据实际目标迁移 SQL 复核：`184_auth_cache_invalidation_outbox.sql` 的 `INSERT` 出现在函数体中，迁移执行阶段不会直接写 outbox，但文件会重建函数和触发器；`191_passkey_credentials.sql` 与 `180_audit_logs.sql` 本身均为建表/索引，不含直接 `DELETE` 或 `TRUNCATE`。
- 规划 6.1.1 的表格已改为“函数体含 DML/触发器重建”和“纯建表/索引”；原审计记录保留，以上更正作为后续取信记录。
- 本次仍未访问线上、未执行数据库写入；下一步不变：在隔离克隆验证对象契约和 alias/adoption 后再启动候选版本。

## 2026-09-01（Asia/Shanghai）— 完成改名迁移 adoption 门禁与本地 PostgreSQL 验证

### 目的与授权

- 目的：把静态审计发现的 23 组旧文件名→目标文件名兼容风险固化到目标迁移 runner；确保同库启动不会重放历史 DDL/DML 或 `CREATE INDEX CONCURRENTLY`，并完善线上只读证据采集。
- 是否得到维护者明确授权：是（迁移前本地代码、测试、文档和只读工具工作）；未获生产写入授权。
- 是否访问线上：否。本轮没有连接生产 PostgreSQL/Redis，没有执行生产 SQL、备份、部署、重启、切流或开关修改。

### 变更/命令

- 分支与基线：`feature/subnexus-migration`，基于目标 `main` `d596d0844`；安全门禁提交 `dfec06ac1c939e07629d8c70b04c2a509f8007d0`，已推送到 `origin/feature/subnexus-migration`。
- 触碰文件：`backend/internal/repository/migrations_runner.go`、`migrations_runner_notx_test.go`、`migrations_schema_integration_test.go`、新增 `migrations_legacy_aliases.go` / `migrations_legacy_contracts.go` 及测试；`tools/production-deploy/subnexus-readonly-preflight.sh`；规划和上下文文档随后更新。
- 执行的命令类别（脱敏）：`gofmt`；repository 单测、`go vet`、全后端编译级测试；Git Bash `bash -n`；`git diff --check`；目标/旧仓库 23 组 SQL 的 TrimSpace+SHA256 逐项核对；本机临时 PostgreSQL 16 隔离集群执行 23 个目标迁移并查询列、索引、约束、函数、触发器目录输出；`git commit`、`git push`。
- runner 行为：目标记录缺失时才查询精确旧文件名；旧 checksum、目标 checksum 和数据库对象/数据契约全部通过后，在 advisory lock 下只插入目标 `schema_migrations` 记录，不重放旧 SQL；任一不匹配 fail-closed。Grok 图片开关和 long-context 定价值按可变运维设置处理，Codex seed/rollup 单例仍严格检查。
- 预检脚本：补充 23 组记录、相关对象和后置数据观察；PostgreSQL 查询统一使用 `public` 表名并通过 `PGOPTIONS` 强制只读。当前脚本 SHA256=`004886DEF59C5AA1AB31B2A44FB482A997D40131575BCC60706390BA80A00F87`。

### 验证结果

- `go test ./internal/repository -count=1 -p=1`：通过。
- `go vet ./internal/repository`：通过。
- `go test ./... -run '^$' -count=1 -p=1`：通过。
- `go test -tags integration ./internal/repository -run '^TestMigrationsRunner_LegacyAliasContractsMatchCurrentSchema$' -count=1 -p=1`：编译/运行通过；当前环境无 Docker 时按 harness 约定跳过实际容器测试。
- Git Bash `bash -n tools/production-deploy/subnexus-readonly-preflight.sh`、`git diff --check`：通过。
- 本机 PostgreSQL 16：23 个目标 SQL 均可执行；系统目录查询语法通过。复核时发现 PostgreSQL 会规范化多事件触发器顺序、且 `groups.platform` 实际为 `NOT NULL`，已分别修正契约顺序和非空约束，并补单测。
- 线上证据：仍无；脚本只生成证据文件，不执行迁移或部署。

### 风险、回滚与下一步

- 风险：生产 `schema_migrations`/Atlas 记录、真实对象定义、备份可恢复性、Redis 恢复点和旧版本回归尚未验证；不得据本地合成库结论直接启动生产候选。
- 回滚点/回滚命令：代码可回到 `df0e6a136`（本轮父提交）或目标基线 `d596d0844`；本轮未改变任何数据库状态。线上仅在维护者批准后按回滚手册操作。
- 下一步：维护者在服务器下载并校验上述固定提交的脚本，执行只读 preflight，脱敏回传 `evidence.txt`；取得 PostgreSQL/Redis 可恢复备份并完成隔离恢复、adoption 演练和旧版本回归后，才开始 Batch 1 业务代码迁移。

## 2026-09-01（Asia/Shanghai）— 发布 Batch 0 记忆与线上预检入口

### 目的与授权

- 目的：提交并推送本轮 Batch 0 记忆、台账、规划和切换手册更新，固定维护者可执行的只读预检入口。
- 是否得到维护者明确授权：是（迁移分支和文档发布）；未获生产写入、部署或切流授权。
- 是否访问线上：否。

### 变更/命令

- 文档发布前固定点：`7747627d5646b140e4b716463d5e6342673d343c`，当时已推送 `origin/feature/subnexus-migration`；目标 `main` 仍为 `d596d0844f274c3e7933c966231851f9f20b0d47`。
- 文档提交：`7747627d5646b140e4b716463d5e6342673d343c`；安全门禁代码/脚本仍固定在父提交 `dfec06ac1c939e07629d8c70b04c2a509f8007d0`。后续文档提交不改变脚本内容，当前分支 SHA 以 `git rev-parse HEAD` 为准。
- 预检脚本固定 SHA256：`004886DEF59C5AA1AB31B2A44FB482A997D40131575BCC60706390BA80A00F87`。
- 数据库迁移/开关/部署动作：无；工作树已核对干净。

### 验证结果

- 本地分支与远端分支指向同一 HEAD；`main` 未修改。
- 线上预检尚未执行；等待维护者在服务器按固定提交校验脚本并回传脱敏证据。

### 风险、回滚与下一步

- 风险：没有线上 `schema_migrations`、容器拓扑、备份和隔离恢复证据，不能启动候选版本或创建 Batch 1 业务迁移。
- 回滚点/回滚命令：迁移分支可回到 `df0e6a136`/`d596d0844`；本轮没有数据库状态变化。
- 下一步：执行只读 preflight；证据通过后再安排可恢复备份、隔离库 adoption 演练和旧版本回归。

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

## 2026-09-02（Asia/Shanghai）— 固化 Batch 0 文档固定点

### 目的与授权

- 目的：修正文档中“当前 HEAD”与“文档发布前固定点”的表述，避免后续提交造成版本定位歧义。
- 是否得到维护者明确授权：是（迁移分支文档维护）；未获生产写入、部署、切流或开关修改授权。
- 是否访问线上：否。

### 变更与验证

- 触碰文件：`SUBNEXUS_CHANGE_MEMORY.md`、`SUBNEXUS_MIGRATION_LEDGER.md`。
- 变更内容：仅更新历史固定点说明；未改变迁移 runner、预检脚本、业务代码、依赖或数据库。
- 已执行：提交并推送 `docs: clarify migration branch fixed points`；脚本 SHA256、Git Bash `bash -n`、`git diff --check`、远端分支指向核对通过。
- 当前分支：`feature/subnexus-migration`；`main`/`origin/main` 未修改。当前 SHA 以 `git rev-parse HEAD` 实时查询为准。

### 风险、回滚与下一步

- 风险：线上状态、备份可恢复性和隔离恢复仍未取得证据；Batch 1 业务迁移继续保持门禁阻断。
- 回滚点：文档提交可按 Git 提交回退；本次没有数据库或生产状态变化。
- 下一步：维护者执行固定脚本的线上只读 preflight，回传脱敏证据后继续 B0-5/B0-6/B0-7。

## 2026-09-02（Asia/Shanghai）— 完成 25 组旧迁移接管与预检收尾复核

### 目的与授权

- 目的：继续执行 Batch 0 规划，完成旧 SubNexus 与新 fork 同库切换所需的迁移文件接管门禁、隔离库复核和只读预检脚本安全收敛。
- 是否得到维护者明确授权：是（本地迁移分支代码、测试和文档）；未获生产 SQL、备份、部署、重启、切流或线上开关修改授权。
- 是否访问线上：否。

### 变更与命令

- 分支与基线：`feature/subnexus-migration`，目标 `main` 基线 `d596d0844f274c3e7933c966231851f9f20b0d47`；`main` 未修改。
- 触碰文件：`backend/internal/repository/migrations_runner.go`、`migrations_legacy_aliases.go`、`migrations_legacy_contracts.go` 及其单测/隔离集成测试；`tools/production-deploy/subnexus-readonly-preflight.sh` 及夹具；`SUBNEXUS_MIGRATION_PLAN.md`、`SUBNEXUS_PROJECT_CONTEXT.md`、`SUBNEXUS_MIGRATION_LEDGER.md`、`SUBNEXUS_CUTOVER_RUNBOOK.md`。
- 迁移规则：固定 25 组显式旧文件名接管（23 组精确 checksum 元数据 adoption，`189/226` 两组分别执行契约校验和事务 replay）；禁止全局 checksum 跳过，使用 PostgreSQL advisory lock，失败 fail-closed。
- 执行的命令类别（脱敏）：`gofmt`；`go test ./internal/repository -count=1 -p=1`；`go test ./migrations -count=1 -p=1`；`go vet ./internal/repository`；Git Bash `bash -n tools/production-deploy/subnexus-readonly-preflight.sh`；预检夹具；`git diff --check`；本地 PostgreSQL 16 隔离集群启动/停止及完整旧迁移接管集成测试。
- 数据库迁移/开关/部署动作：仅在本机临时隔离数据库执行测试迁移；集群监听 `127.0.0.1:60001`，测试完成后已停止；未连接生产库，未修改任何线上数据或开关。

### 验证结果

- repository 单测、migrations 单测、`go vet`、Shell 语法和预检夹具均通过。
- `go test -tags legacyintegration ./internal/repository -run '^TestMigrationsRunner_FullLegacyCheckoutHandoff$' -count=1 -p=1` 通过（完整旧迁移目录、当前 runner 接管、重复启动、旧目录重跑及 `189/226` 最终契约）。
- 后端全量 `go test ./... -count=1 -p=1` 的业务包均完成，但 Windows 本地 `internal/service` 保留基线残余：`TestContentModerationRuntimeSnapshotRefreshFailureKeepsStaleConfig` 的 1 秒异步刷新等待存在时序竞态；4 个 `TestPluginPackageInstaller*` 在 Windows 对仍打开的 zip 临时文件执行 `os.Rename` 时失败（`The process cannot access the file because it is being used by another process`）。本轮未修改 `internal/service`；`git diff main...HEAD -- backend/internal/service` 为空，生产部署目标为 Linux，需另开跨平台测试修复任务。
- 线上证据：无；预检脚本只读，输出仍需维护者在服务器执行后脱敏回传。

### 风险、回滚与下一步

- 风险：生产 `schema_migrations`/Atlas 记录、真实对象、备份可恢复性、Redis 恢复点及旧版本回归尚未验证；不能据本地隔离库直接启动候选或创建 Batch 1 业务迁移。
- 回滚点/回滚命令：本轮代码提交前的 `074756ad1b7ab93234ab26b4fcf6d10f0f989363`；数据库无生产变更，应用回滚优先，不恢复数据库。
- 下一步：维护者下载并校验本轮提交中的只读预检脚本，在当前 OVH 服务器执行并回传脱敏 `evidence.txt`；只有 B0-5/B0-6/B0-7 全部通过后才开始 Batch 1，所有新功能继续默认关闭。

## 2026-09-02（Asia/Shanghai）— 固定本轮 Batch 0 发布校验点

### 目的与授权

- 目的：为维护者提供可复核的只读预检脚本提交和 SHA256 固定点，并保持线上执行前的证据门禁。
- 是否得到维护者明确授权：是（迁移分支提交/推送和文档记录）；未获生产写入、部署、切流或开关修改授权。
- 是否访问线上：否。

### 变更与验证

- 控制提交：`7d30a2faae10cc8910bd853f6e2d9282aebb7b29`，由 `feature/subnexus-migration` 指向；目标 `main`/`origin/main` 仍为 `d596d0844f274c3e7933c966231851f9f20b0d47`。
- 预检脚本 SHA256：`D68B6BD54AF75B821257F42FC9A7360E0E9828AD0F561B9045B92137036255D1`；夹具脚本 SHA256：`08D383C33F452E85388DF1E433BB6458490F8513635B87AADC4F580EF65C021E`。
- 已执行：提交前后 `git diff --check`、repository/migrations 单测、`go vet`、Git Bash `bash -n`、预检夹具、完整旧迁移隔离集成测试；本地临时 PostgreSQL 已停止。
- 数据库迁移/开关/部署动作：无生产动作；未执行线上 SQL、备份、重启、切流或功能开关修改。

### 风险、回滚与下一步

- 风险：线上只读证据、可恢复备份和旧版本隔离回归仍未取得；不得直接启动候选或创建 Batch 1 业务迁移。
- 回滚点：`074756ad1b7ab93234ab26b4fcf6d10f0f989363`（本轮控制提交父节点）；数据库无生产变更。
- 下一步：推送并校验迁移分支后，维护者按 `SUBNEXUS_CUTOVER_RUNBOOK.md` 使用完整提交 SHA 和脚本 SHA256 执行只读 preflight，脱敏回传证据；通过 B0-5/B0-6/B0-7 后再继续业务迁移。

## 2026-09-02（Asia/Shanghai）— 统一 Batch 0 预检固定点到当前 tip

### 目的与授权

- 目的：消除手册曾引用父提交而当前分支已前进一提交造成的校验歧义，给线上预检一个唯一、可复核的完整提交。
- 是否得到维护者明确授权：是（迁移分支文档维护）；未获生产 SQL、备份、部署、重启、切流或线上开关修改授权。
- 是否访问线上：否。

### 变更与验证

- 触碰文件：`SUBNEXUS_CUTOVER_RUNBOOK.md`、`SUBNEXUS_MIGRATION_LEDGER.md`、`SUBNEXUS_CHANGE_MEMORY.md`。
- 当前预检固定提交：`7200e5ae1f48d8f78bce43565814378b636c842b`；父提交 `7d30a2faae10cc8910bd853f6e2d9282aebb7b29` 的脚本内容保持相同。
- 预检脚本 SHA256：`D68B6BD54AF75B821257F42FC9A7360E0E9828AD0F561B9045B92137036255D1`。
- 已执行：`git diff --check`（待提交前再次复核）；未改变 runner、预检脚本、业务代码、依赖、数据库或生产状态。

### 风险、回滚与下一步

- 风险：B0-5/B0-6/B0-7 仍待线上证据，Batch 1 业务迁移继续禁止创建/应用。
- 回滚点：本次为文档修正；脚本批准发布点仍为 `7200e5ae1f48d8f78bce43565814378b636c842b`，必要时可回到其父提交 `7d30a2faae10cc8910bd853f6e2d9282aebb7b29`；数据库无变化。
- 下一步：推送后维护者只需替换实时 `repo_root`、应用容器名和公网健康 URL，执行手册中的固定 SHA 只读 preflight，并脱敏回传 `evidence.txt`。

## 2026-09-02（Asia/Shanghai）— 预检门禁校验语义修正并推送

### 目的与授权

- 目的：完成 Batch 0 预检脚本发布点校验的不可变语义，允许维护文档继续前进而不改变已批准脚本资产。
- 是否得到维护者明确授权：是（迁移分支文档、测试和推送）；未获生产 SQL、备份、部署、重启、切流或线上开关修改授权。
- 是否访问线上：否。

### 变更与验证

- 提交：`7b3d3929c`（`docs: make preflight release verification immutable`），已推送 `origin/feature/subnexus-migration`。
- 触碰文件：`SUBNEXUS_CUTOVER_RUNBOOK.md`、`SUBNEXUS_MIGRATION_LEDGER.md`、`SUBNEXUS_CHANGE_MEMORY.md`。
- 校验语义：批准脚本提交固定为 `7200e5ae1f48d8f78bce43565814378b636c842b`；同时校验该提交中的脚本及当前执行文件的 SHA256=`D68B6BD54AF75B821257F42FC9A7360E0E9828AD0F561B9045B92137036255D1`，不再要求服务器工作树 HEAD 等于脚本发布提交。
- 已通过：`git diff --check`、Git Bash `bash -n tools/production-deploy/subnexus-readonly-preflight.sh`、预检夹具、repository/migrations 测试、全仓编译级测试；`main`/`origin/main` 未修改，工作树提交后保持干净。

### 风险、回滚与下一步

- 风险：B0-5 线上只读证据、B0-6 可恢复备份和 B0-7 隔离恢复/旧版本回归仍未取得；不得启动候选或创建/应用 Batch 1 业务迁移。
- 回滚点：文档提交前 `7200e5ae1f48d8f78bce43565814378b636c842b`；无数据库或生产状态变化。
- 下一步：维护者从 `7b3d3929c`（或其后代）检出目标仓库，按 `SUBNEXUS_CUTOVER_RUNBOOK.md` 替换实时路径、容器名和健康 URL，执行只读 preflight 并回传脱敏证据。

## 2026-09-02（Asia/Shanghai）— 回填预检门禁文档推送记录

### 目的与授权

- 目的：记录上一轮门禁语义修正的最终记忆提交，确保后续维护者能从分支 tip 追溯到批准脚本资产。
- 是否得到维护者明确授权：是（迁移分支文档维护和推送）；未获生产 SQL、备份、部署、重启、切流或线上开关修改授权。
- 是否访问线上：否。

### 变更与验证

- 最终记忆提交：`2c0f842cb28846d985eb5ffcf771efddb380780a`，已推送 `origin/feature/subnexus-migration`。
- 变更：台账将“当前 tip”改为“批准脚本发布点”；批准脚本提交仍为 `7200e5ae1f48d8f78bce43565814378b636c842b`，当前分支允许是其后代。
- 已复核：本地/远端迁移分支指向一致，`main`/`origin/main` 为 `d596d0844f274c3e7933c966231851f9f20b0d47`，工作树干净；脚本 SHA256 未变化。

### 风险、回滚与下一步

- 风险：B0-5/B0-6/B0-7 仍待线上证据，业务迁移和候选部署保持禁止。
- 回滚点：`2c0f842cb` 的父提交 `7b3d3929c`；无数据库或生产状态变化。
- 下一步：按手册执行只读 preflight，脱敏回传证据后继续隔离备份/恢复门禁。

## 2026-09-02（Asia/Shanghai）— 收敛预检脚本校验与执行竞态

### 目的与授权

- 目的：消除预检脚本哈希校验完成后到提权执行之间的 TOCTOU 窗口，确保线上执行的字节就是批准版本。
- 是否得到维护者明确授权：是（迁移分支手册和门禁记录维护）；未获生产 SQL、备份、部署、重启、切流或线上开关修改授权。
- 是否访问线上：否。

### 变更与验证

- 触碰文件：`SUBNEXUS_CUTOVER_RUNBOOK.md`、`SUBNEXUS_MIGRATION_LEDGER.md`、`SUBNEXUS_CHANGE_MEMORY.md`。
- 运行方式：一个 root shell 接收路径/容器参数，验证批准提交 `7200e5ae1f48d8f78bce43565814378b636c842b` 与脚本 SHA256，复制 Git blob 到 root-only `mktemp` 文件后执行；同时拒绝非 root-owned 的隔离仓库或 `.git`。
- 脚本内容和 SHA256 未改变：`D68B6BD54AF75B821257F42FC9A7360E0E9828AD0F561B9045B92137036255D1`。
- 未改变：迁移 runner、业务代码、依赖、数据库和生产状态；需在提交后复核 `git diff --check`、Shell 夹具和远端分支。

### 风险、回滚与下一步

- 风险：B0-5/B0-6/B0-7 仍待线上证据；root-owned 隔离仓库准备不正确时预检会 fail-closed。
- 回滚点：本次仅为手册/台账变更，回到提交 `9d44f659d71632382470971ac53b3a511165b851` 即可；数据库无变化。
- 下一步：完成提交推送后，维护者按新 root-shell 命令执行只读 preflight，并回传脱敏证据。

## 2026-09-02（Asia/Shanghai）— 修正独立 clone 的 no-checkout 状态检查顺序

### 目的与授权

- 目的：修复预检一行命令在服务器首次运行时于 `fetch` 后静默退出的问题。
- 是否得到维护者明确授权：是（迁移分支手册、台账和命令维护）；未获生产 SQL、备份、部署、重启、切流或线上开关修改授权。
- 是否访问线上：否。

### 变更与验证

- 原因：`git clone --no-checkout` 在 checkout 前会让 `git status --porcelain` 报全部受跟踪文件为删除；原命令先执行空状态断言，触发 `set -e` 并提前退出。
- 修正：先 fetch、checkout 固定提交 `d557599ec07543ef40d843e465873dd731fd6200`、核对 HEAD，再检查工作树为空；没有改动预检脚本或数据库逻辑。
- 触碰文件：`SUBNEXUS_CUTOVER_RUNBOOK.md`、`SUBNEXUS_MIGRATION_LEDGER.md`、`SUBNEXUS_CHANGE_MEMORY.md`。
- 本地验证：从 `--no-checkout` 克隆复现原状态后，按新顺序 checkout 再检查通过；待提交后再复核文档 shell 语法、脚本夹具和远端分支。

### 风险、回滚与下一步

- 风险：线上尚未执行预检；若存在非干净的既有隔离 clone，新命令会在 checkout 时停止，不覆盖其改动。
- 回滚点：本次文档修正前 `d557599ec07543ef40d843e465873dd731fd6200`；无数据库或生产状态变化。
- 下一步：推送修正后使用新的一行命令重新执行 B0-5。

## 2026-09-02（Asia/Shanghai）— 修正 Docker HostIp 兼容性并轮换预检批准点

### 目的与授权

- 目的：修复线上只读预检在 Docker 端口绑定检查阶段因模板字段 `.HostIP` 拼写错误而退出的问题，并把可执行批准点更新到包含修复的不可变提交。
- 是否得到维护者明确授权：是（迁移分支脚本、夹具和运行文档维护）；未获生产 SQL、备份、部署、重启、切流或线上开关修改授权。
- 是否访问线上：否。此前用户执行旧脚本时仅在应用端口检查阶段失败，未进入 PostgreSQL/Redis 检查，也未修改业务状态。

### 变更与验证

- 代码提交：`093163b2918fe15af8f909ae716531b9298f75b6`（`fix: support Docker HostIp port binding field`），父提交为 `da04e0587105c4f1347c6060bdb7299961835c68`。
- 触碰文件：`tools/production-deploy/subnexus-readonly-preflight.sh`、`tools/production-deploy/subnexus-readonly-preflight.test.sh`、`SUBNEXUS_CUTOVER_RUNBOOK.md`、`SUBNEXUS_MIGRATION_LEDGER.md`、`SUBNEXUS_CHANGE_MEMORY.md`。
- 修复内容：`docker inspect --format` 使用 Docker 实际字段 `.HostIp`；夹具增加 `.HostIp` 正例、`.HostIP` 反例和端口绑定执行测试。
- 当前批准预检资产：脚本提交 `093163b2918fe15af8f909ae716531b9298f75b6`；脚本 SHA256=`42698FFF5751C8CF22724E065ABBC491D4D2192EA01895714F168DCEC76EF1C6`。
- 旧批准资产 `7200e5ae1f48d8f78bce43565814378b636c842b` / `D68B6BD54AF75B821257F42FC9A7360E0E9828AD0F561B9045B92137036255D1` 已明确标记 superseded，历史记录未删除。
- 本地验证：Git Bash `bash -n`（两份脚本）、预检夹具、`git diff --check` 均通过；当前工作树脚本 SHA 与批准值一致。

### 风险、回滚与下一步

- 风险：线上 B0-5 只读证据、B0-6 可恢复备份和 B0-7 隔离恢复/旧版本回归仍未取得；旧命令不得重试，Batch 1 业务迁移仍禁止创建或应用。
- 回滚点：代码可回到父提交 `da04e0587105c4f1347c6060bdb7299961835c68`；数据库和生产状态无变化。旧脚本仅可作为审计参考，不得重新执行。
- 下一步：推送当前提交和文档更新后，维护者使用新批准 SHA、显式容器 `subnexus-cutover` 的单行命令重新执行只读预检，并回传脱敏 `evidence.txt`。
