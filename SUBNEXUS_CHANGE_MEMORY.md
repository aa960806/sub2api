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

- 旧项目参考分支 `alignment/v0.1.181-local` / HEAD `62ea35e1c78416fd83e1e41bbb310b307941811a` / 应用版本 `0.1.135` / Go `1.26.6`。
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

### 交接记录

- 代码提交 `093163b2918fe15af8f909ae716531b9298f75b6` 与文档提交 `ba63facd0ae49dd0f508204af4b585d1e01490eb` 已推送到 `origin/feature/subnexus-migration`；远端当前 tip 为 `ba63facd0ae49dd0f508204af4b585d1e01490eb`，目标 `main`/`origin/main` 仍为 `d596d0844f274c3e7933c966231851f9f20b0d47`。
- 服务器命令策略：在已存在的 `/root/subnexus-migration/preflight` 中只执行受限 `git fetch`，验证批准提交及脚本 SHA256，从 Git blob 复制 root-only 临时文件后执行；不 checkout、不检查未 checkout 工作树、不停止或重启任何容器。
- 当前指定生产应用容器为 `subnexus-cutover`；`subnexus-bepusdt-test` 为测试容器，禁止自动选择或触碰。预检证据根目录为 `/srv/subnexus-migration/preflight`，公网 URL 暂留空，仅执行容器本地 health 检查。

## 2026-09-02（Asia/Shanghai）— 纠正为本地优先迁移并拉取最新上游

### 最新授权与顺序

- 维护者重申：只允许修改 fork `F:\MySub2\sub2api`；旧项目 `F:\Sub2Api\SubNexus` 永久只读。必须先在本地完成全部保留二开功能和测试，之后才推送，最后由服务器拉取固定版本。
- 本条记录 supersede 此前所有“先执行服务器预检/B0-5/B0-6/B0-7，再开始 Batch 1”的下一步表述；历史记录保留用于审计，但不再是当前执行入口。
- 生产 B0-5/B0-6/B0-7 改为本地 Batch 1-5 与维护者验收后的 Release Gate。旧预检批准 SHA/脚本 SHA 均冻结，当前禁止执行任何服务器命令。

### 已执行与当前状态

- 仅在 fork 添加只读上游远端 `upstream=https://github.com/Wei-Shaw/sub2api.git` 并执行 `git fetch --prune --no-tags upstream`；未修改旧项目、生产环境或 fork `main`，未推送。
- 已抓取 `upstream/main`=`5097b31457e6dc9f49e5f5c9c72b925ce79543b3`，其应用版本为 `0.2.0`；当前迁移分支相对共同基线有 25 个本地提交和 57 个上游提交，待在本地分支合并并验证。
- 当前开始修正规划、上下文、台账和切换手册；随后在本地迁移分支整合最新上游并启动 Batch 1。所有功能继续默认关闭，明确排除每日消耗转盘、红包雨、运行日历、Media Studio/Creative Workshop。

### 安全边界与下一步

- 维护者本地验收前只允许本地提交，不再 `git push`，不执行服务器预检、备份、部署、拉取、重启、切流或开关修改。
- 下一步：提交本地顺序修正文档，合并 `upstream/main`，运行更新后的基线和 migration runner 测试，再实现 Batch 1 的首个默认关闭切片。

## 2026-09-02（Asia/Shanghai）— 补齐 Channel Monitor V3 时间线边界测试

### 目的与范围

- 只读比对旧项目与目标 fork 的 V3 页面、卡片、时间线组件和时间线算法；四个实现文件内容一致，旧项目未被修改。
- 确认 Passkey 已由最新上游完整提供，核心 service 内容一致，归类为“以上游为准”，不重复迁移。
- 在目标 fork 新增 `frontend/src/features/channel-monitor-v2/__tests__/monitorTimeline.spec.ts`，覆盖尾部空桶、全空数据和非法时间戳三个边界。

### 安全边界与下一步

- 本次仅修改迁移分支的一份前端测试和本记忆文件；未访问服务器或生产 PostgreSQL/Redis，未部署、推送、切流或修改开关。
- V3 继续复用 `channel_monitor_enabled` 与互斥的 `channel_monitor_mode=v3`；缺失/非法模式保持 `v1`，不会默认启用 V3。
- 下一步：运行 V3 专项测试并将 V3、学生充值优惠及其余活动候选纳入完整功能裁决矩阵。

### 验证补记

- `pnpm exec vitest run src/features/channel-monitor-v2/__tests__/monitorTimeline.spec.ts src/features/channel-monitor-v2/__tests__/monitorFormat.spec.ts`：2 个文件、16 个测试全部通过。
- 首次 Go 专项运行因系统盘临时空间不足在链接阶段失败，路由包已通过；未删除任何系统文件。改用 `F:\MySub2\.gotmp` 和 `F:\MySub2\.gocache` 后重跑 `internal/service` 与 `internal/server/routes` 成功。

## 2026-09-02（Asia/Shanghai）— Battle Pass 主审财务边界修正

### 发现与修正

- 主审确认 Battle Pass 后端、路由、Wire、默认关闭公开设置和管理员 step-up 已接入，但补发现两个边界问题。
- `SetEnabled(true)` 在 service 缺少数据库依赖时原先会跳过统计边界快照并写成开启；现改为返回失败，开关保持关闭。
- 订阅奖励原先用 `LIKE '%battle_pass_reward:<id>%'` 判断幂等标记，奖励 ID 前缀可能碰撞；现改为按订阅 notes 的换行分隔完整行精确匹配，并新增回归测试。

### 验证与安全边界

- 已对四个修改文件执行 `gofmt`。专项测试首次编译被并行中的跑马灯测试文件缺少临时 stub 阻断，属于共享工作区尚未完成状态；待跑马灯代理交付后立即重跑，当前不能标记通过。
- 本次只修改目标 fork 的 Battle Pass service/测试和本记忆文件；未修改旧项目、服务器、生产数据库、Redis、远端分支或功能开关。

## 2026-09-02（Asia/Shanghai）— 首充礼包管理员配置 UI 交付与主审

### 实现与验证

- 新增独立管理员页面 `frontend/src/views/admin/FirstRechargeGiftView.vue`，并在 `frontend/src/api/admin/payment.ts` 增加独立配置 GET/PUT 类型和调用。
- 增加 `/admin/first-recharge-gift` 路由、管理员侧栏入口、中英文导航和页面文案；简单模式隐藏并阻断该页面，功能关闭时管理员入口仍可达以便后续显式开启。
- 页面只接受显式布尔开启值；加载失败不展示保存表单并保持关闭，非法金额不提交，保存失败恢复最近一次服务端确认配置。
- 代理验证：`pnpm typecheck` 通过；API、页面、路由和侧栏 4 个定向测试文件共 8 个测试通过；新增文件 ESLint 与 `git diff --check` 通过。主审确认该页面不直接改变支付金额、订单或用户资格。

### 风险与下一步

- 当前仅为本地迁移工作区证据，仍需与首充订单创建、回调、取消/过期、退款及学生优惠互斥路径一起运行全仓测试。
- 未修改旧项目、服务器或生产数据，未部署、推送、切流或开启功能。

## 2026-09-02（Asia/Shanghai）— 跑马灯广播独立迁移

### 审计裁决

- 旧实现把 `activity_broadcasts`、签到和多种已排除的转盘/抽奖活动放在同一 `ActivityService`。目标 fork 只迁移管理员手动广播，不复制旧 `broadcastActivityReward`、奖励阈值/模板、清理任务或任何转盘、红包雨、邀请抽奖、充值转盘联动。
- 上游 `AnnouncementService`、`AnnouncementHandler`、`announcements`/`announcement_reads` 和既有前端公告弹窗保持不变。新 API 使用 `/marquee/*` 与 `/admin/marquee/*`，两套功能没有表、service、handler 或前端 store 复用。
- 同库切换继续复用 `activity_broadcasts` 以保留旧手动消息；repository 的列表、更新和删除全部强制 `source='admin'`，创建时 source 也由服务端固定。旧系统奖励广播不删除，供旧版本回滚，但新版本不展示、不编辑。

### 实现

- 新增严格 fail-closed 的 `subnexus_marquee_enabled`。只有数据库原始字符串逐字等于 `true` 才开启；缺失、`TRUE`、`1`、带空格值和读取异常全部关闭。
- 新增 `MarqueeService`/repository、用户列表 handler、管理员配置和 CRUD handler、Wire provider 与路由。关闭态用户返回 `{enabled:false,items:[]}`，管理员列表返回空数组，所有创建/更新/删除拒绝，且上述路径均不会访问 `activity_broadcasts`。
- 新增 `9007_subnexus_marquee.sql`，仅幂等创建/复用旧表并添加 `source='admin'` 的部分索引；没有 DML、删除、覆盖或 announcements 变更。TrimSpace SQL checksum 为 `19deec6328c814418b66372c066fd2b439bcbdb5c11659394b5fdf32e509128b`。
- 新增全局 `BroadcastMarquee.vue` 与管理员 `MarqueeView.vue`。用户组件仅在“认证成功、公共设置已成功加载、开关显式 true”同时成立时请求并建立 30 秒轮询；关闭/失败会清空消息并阻止旧请求回填，浏览器再按 `source='admin'` 二次过滤。

### 验证与剩余门禁

- 通过 `go test ./migrations -run TestSubNexusMarqueeMigration -count=1`。
- 通过 `go test ./internal/service -run "Test(Marquee|SettingServiceGetPublicSettingsMarquee)" -count=1`、`go test -tags unit ./internal/repository -run TestMarqueeRepository -count=1`，以及 handler/admin/routes 编译检查。
- 通过 `pnpm vitest run src/api/__tests__/marquee.spec.ts src/components/common/__tests__/BroadcastMarquee.spec.ts src/views/admin/__tests__/MarqueeView.spec.ts`：3 个文件、7 个测试。
- 通过 `pnpm typecheck`、`pnpm build`、新增前端文件 ESLint 和 `git diff --check`。构建只有既有动态/静态 import、Browserslist 数据和 chunk size 警告；全仓测试及 Wire 生成由并发迁移项完成共享接线后统一执行。
- 本次只修改 `F:\MySub2\sub2api`；旧项目仅作只读审计。未访问服务器、生产 PostgreSQL/Redis，未部署、推送、切流或修改生产开关。

## 2026-09-02（Asia/Shanghai）— 迁移收拢续作工作区审计

- 核对目标仓库当前分支为 `feature/subnexus-migration`，HEAD 为上游同步合并提交 `23d6e8ec0`；`main`、旧项目 `F:\Sub2Api\SubNexus` 和服务器均未操作。
- 发现学生充值优惠、邀请活动仍处于共享工作区接线阶段，`cmd/server` 需待 Wire 重新生成；注册 IP 冷却代理因服务限流未交付，后续由主线程实现。
- 本次仅进行本地状态读取并开始收拢计划，未执行部署、生产数据库/Redis 访问、重启、切流或生产开关修改。

## 2026-09-02（Asia/Shanghai）— 继续收拢：学生优惠与认证安全门禁

- 共享工作区已出现学生充值优惠的用户/管理员 handler、路由、支付接线和 scheduler provider；待主线程核对生命周期并重新生成 Wire。
- 客服支持/默认语言代理只留下设计结论，需主线程确认实际文件是否写入；邀请活动代理因服务限流未完成，必须以共享工作区实况为准补齐。
- 注册 IP 冷却尚未迁移；下一步只在目标 fork 增加独立迁移、设置解析和所有新用户创建路径的 reservation/finalize/release，不连接服务器或生产数据库。

## 2026-09-02（Asia/Shanghai）— 学生优惠 Wire 类型修复

- 将 `StudentRechargeBenefitService` 对缓存的依赖收窄为仅含 `InvalidateUserBalance` 的内部接口；原先完整 `BillingCache` 与 `*BillingCacheService` 不兼容，阻止 `cmd/server` 编译。
- 未改变余额写入、订单履约或开关逻辑；仅修正依赖边界，旧项目、服务器和生产数据均未触碰。

## 2026-09-02（Asia/Shanghai）— Wire 生成前接线收拢

- 为学生优惠 scheduler 增加应用清理阶段的 `Stop`，避免服务退出时遗留后台扫描协程。
- 已确认生成文件仍缺学生优惠参数；下一步仅在本地重新生成 Wire 并运行 `cmd/server` 编译检查。

## 2026-09-02（Asia/Shanghai）— Wire 生成后测试签名同步

- Wire 生成成功；同步 `wire_gen_test.go` 的 `provideCleanup` 调用，补入 `StudentRechargeBenefitScheduler` 的 nil 测试参数。
- 这是测试接线同步，不改变运行时行为；仍未访问服务器或生产数据库。

## 2026-09-02（Asia/Shanghai）— 注册 IP 冷却设置模型

- 增加目标 fork 的 `registration_ip_cooldown_enabled` / `registration_ip_cooldown_seconds` 设置键、系统设置字段和管理员 DTO；默认关闭，秒数由服务层限制在 1..86400，默认 300。
- 更新请求字段使用指针，旧版本或旧前端不发送字段时保留现值，满足同库回滚兼容要求。

## 2026-09-02（Asia/Shanghai）— 注册 IP 冷却核心实现

- 新增 `backend/internal/service/auth_registration_ip_cooldown.go`：使用可信客户端 IP + JWT secret 的 SHA-256 哈希，支持 120 秒 reservation、并发冲突、冷却剩余时间、finalize/release，并尊重 Ent transaction context。
- 新增迁移 `9010_subnexus_registration_ip_cooldown.sql`，只创建独立表/索引/注释，不修改 users、订单、余额或旧迁移。
- 尚未把 reservation 接入所有 AuthService 创建入口；下一步逐路径接线并补充 SQL mock/回滚测试。

## 2026-09-02（Asia/Shanghai）— 邀请活动路由、Handler 与公共开关接线

### 本地变更

- 新增独立用户 handler `backend/internal/handler/subnexus_invite_activities_handler.go`，接入邀请抽奖、充值双层转盘、邀请里程碑的 GET 状态和 POST 领奖；里程碑请求严格要求正整数 `invites`。未引入每日消耗转盘、红包雨、运行日历或 Media Studio。
- 新增管理员 handler `backend/internal/handler/admin/subnexus_invite_activities_handler.go`，提供 `/admin/invite-activities/config` 的 GET/PUT。管理员可在总开关关闭时准备经校验的策略；配置错误通过统一错误响应返回。
- 更新 Handler/Wire/Provider 与生成文件，仓储使用 `NewSubNexusInviteActivitiesRepository`，服务使用 `ProvideInviteActivitiesService`。用户端路径为 `/activity/invite-lottery`、`/activity/recharge-wheel`、`/activity/invite-milestone`（GET/POST）。
- 公共设置新增 `subnexus_invite_activities_enabled` 总开关及三个经 JSON 策略验证的子开关字段；缺失、非法、读取失败和非字面 `true` 均 fail-closed。初始化默认总开关为 `false`，配置 JSON 不会被自动开启。

### 验证与安全边界

- `go generate ./cmd/server` 成功；`go test ./cmd/server -run '^$' -count=1`、`go test ./internal/server/routes -count=1` 及 handler/service/repository 编译级测试通过。
- 新增公共活动开关 fail-closed 单测；邀请活动核心定向测试继续通过。未修改 `main`、旧项目、`frontend/pnpm-lock.yaml`、VERSION，未访问服务器或生产 PostgreSQL/Redis，未部署、推送、重启、切流或开启任何开关。

## 2026-09-02（Asia/Shanghai）— 注册 IP 冷却全注册路径接线

### 实现与回滚边界

- 在目标 fork 的 `AuthService` 六条新用户创建路径接入 reservation：普通邮箱注册、旧 OAuth、TokenPair OAuth、已验证邮箱 OAuth、pending OAuth 邮箱注册和 pending OAuth 已验证注册。
- 普通/即时 OAuth 在用户写入成功后立即 finalize；创建错误、邮箱唯一性竞态和策略失败自动 release。pending OAuth 将 reservation 绑定到用户，`FinalizeOAuthEmailAccount` 在身份/浏览器会话事务内 finalize。
- `RollbackOAuthEmailAccountCreation` 无论邀请码恢复或用户删除是否出错都会按用户释放 reservation；补齐 pending OAuth 在数据库客户端为空或事务开启失败时的账户回滚，避免孤儿账户和残留冷却。
- 管理设置审计 diff 新增 `registration_ip_cooldown_enabled` 与 `registration_ip_cooldown_seconds`；新增 unit/SQL mock 测试覆盖默认值、fail-closed、token/user guard 和回滚释放。

### 验证与安全边界

- 通过 `go test -tags unit ./internal/service` 认证相关测试（含新增冷却测试）、`go test -tags unit ./internal/handler/admin -run TestDiffSettings_DetectsRegistrationIPCooldown`，以及 `go test ./internal/handler ./internal/service -run '^$' -count=0` 编译检查。
- 仅修改 `F:\MySub2\sub2api` 迁移分支；未修改 `F:\Sub2Api\SubNexus`、`main`、服务器、生产 PostgreSQL/Redis，未部署、推送、重启、切流或开启功能开关。

## 2026-09-02（Asia/Shanghai）— 注册 IP 冷却迁移契约补充

- 对照旧项目全部认证调用点完成只读审计：普通注册、各 OAuth 直达/完成/待定流程和 OIDC 已验证邮箱快速路径均已注入可信客户端 IP；未发现遗漏入口。
- 新增 `backend/migrations/subnexus_registration_ip_cooldown_contract_test.go`，静态验证 `9010_subnexus_registration_ip_cooldown.sql` 的独立表、字段、索引和无 DML/无开关写入约束；目标与旧 `159_registration_ip_cooldown.sql` 的 SQL SHA256 保持一致。
- 验证：注册冷却/认证专项 Go 测试、相关包编译检查和迁移契约测试通过；未访问旧项目以外的可写路径、服务器或生产 PostgreSQL/Redis，未部署、推送、重启、切流或开启任何开关。

## 2026-09-02（Asia/Shanghai）— 发票写事务独立开关复核

- 审计发现发票 service 层检查 `subnexus_invoice_enabled` 后到 repository 写事务之间存在关闭竞态，且部分管理员写事务没有事务内开关复核；这会让 legacy `INVOICE_CONFIG.enabled=true` 在独立开关关闭后仍可能触发写入。
- 在目标 fork 的 `backend/internal/repository/invoice_repo.go` 为提交、取消、重提、管理员接单/释放/驳回、开票、替换文件和作废事务统一增加 `ensureInvoiceEnabledInTx`；该 helper 在同一事务内以 `FOR SHARE` 读取 legacy 配置和 namespaced gate，缺失、非字面 `true` 或 legacy 配置关闭均 fail-closed，并在任何业务写入前返回。
- 新增 `backend/internal/repository/invoice_rollout_gate_test.go` sqlmock 回归测试，覆盖 gate 缺失、`false`、非规范 `TRUE` 和双开场景。通过 `go test ./internal/repository -run TestEnsureInvoiceEnabledInTxRequiresIndependentGate -count=1`。
- 本次只修改目标 fork；未修改旧项目、服务器、生产数据库/Redis、`main`、`frontend/pnpm-lock.yaml` 或 `VERSION`，未部署、重启、切流或开启功能。

## 2026-09-02（Asia/Shanghai）— 首充预约并发保护

- 审计发现首充礼包在替换已过期/取消预约时，多个并发订单可同时读取同一旧行并依次覆盖 `order_id`；先创建但后覆盖的订单一旦付款，会因预约归属不一致而无法履约，形成资金悬挂风险。
- 在 `backend/internal/service/subnexus_first_recharge.go` 的 `ReserveTx` 中，PostgreSQL 事务现在对预约行执行 `FOR UPDATE OF p`（仅锁预约表行，兼容左连接），并在替换更新中加入旧 `order_id` compare-and-swap；旧订单为空时使用 `order_id IS NULL`。竞争事务得到确定的 pending 错误并回滚其订单。
- 新增 `backend/internal/service/subnexus_first_recharge_sql_test.go`，用 PostgreSQL sqlmock 固化行锁与 `$11` 旧订单 CAS 条件；首充专项测试（含既有 SQLite 状态测试）通过。
- 本次未修改旧项目、服务器、生产数据库/Redis、`main`、`frontend/pnpm-lock.yaml` 或 `VERSION`，未部署、切流或开启功能。

## 2026-09-02（Asia/Shanghai）— 邀请活动与明确排除项静态审计

### 审计范围与裁决

- 只读对照 `F:\Sub2Api\SubNexus` 与目标 fork 的 handler、service、repository、迁移、用户/管理员路由、前端 API、路由 meta、侧栏和页面；旧项目保持只读，目标 `main`、服务器和生产数据均未触碰。
- 本次保留的邀请活动切片是邀请抽奖、充值奖励转盘和邀请里程碑。目标实现使用独立 `InviteActivitiesService`/repository、`/activity/invite-lottery`、`/activity/recharge-wheel`、`/activity/invite-milestone` 及管理员 `/admin/invite-activities/config`，并复用 `activity_reward_logs` 的 source/period 幂等身份。
- `RechargeWheelView.vue` 是“累计充值达到门槛后的充值奖励转盘”，不是用户明确排除的“每日消耗转盘”；该差异已记录，不能按名称误删或误迁。
- 在目标 fork 的当前提交和迁移工作树中，未发现 SubNexus 版本的每日消耗转盘、红包雨、运行日历或 Media Studio/Creative Workshop 的新增文件、目标路由、后端 handler/service 或目标迁移。文档和契约测试中的排除项文字仅用于审计，不能视为运行入口；今后若上游提供同名功能，严格以上游实现为准，不复制旧项目实现。
- 活动中心 repository/service 和前端再次限定 `activity_type='custom'`；旧库中的排除活动行不被新版本列表、编辑或删除，保留以支持旧版本回滚。邀请活动开关缺失、非法 JSON、非字面量 `true` 或子开关未同时开启时均 fail-closed，关闭态不访问活动数据表。

### 本地专项验证

- `go test -tags unit ./internal/service -run 'Invite|SubNexusInvite|Setting.*Invite' -count=1`：通过。
- `go test -tags unit ./internal/repository -run 'SubNexusInvite|InviteActivities|AffiliateSignup' -count=1`：通过。
- `go test ./migrations -run 'SubNexusInvite|Invite' -count=1`：通过。
- `pnpm exec vitest run src/api/__tests__/inviteActivities.spec.ts src/utils/__tests__/inviteActivities.spec.ts src/router/__tests__/invite-activities-routes.spec.ts src/components/layout/__tests__/AppSidebar.inviteActivities.spec.ts src/views/user/__tests__/InviteActivitiesViews.spec.ts`：5 个文件、22 个测试通过。
- 本轮仅有本地代码/文档验证；尚未连接线上 PostgreSQL/Redis，未执行迁移、部署、重启、切流或开启任何功能开关。

### 未完成门禁与回滚点

- 邀请活动仍属于本地迁移中的高风险余额功能，需与 Batch 2 其余支付/Affiliate 生命周期、全仓测试、隔离 PostgreSQL/Redis 和旧版本回归一起验收后才能标记通过。
- 回滚优先使用本批次提交前的代码 SHA，数据库不做恢复；新增索引和活动表均保持可被旧版本忽略。生产发布前仍需由维护者完成 Release Gate，当前不得执行服务器命令。

## 2026-09-03（Asia/Shanghai）— 发票事务退款闭集与释放原因校验

- 对照支付服务的退款状态闭集审计发票资格判断，确认 `REFUND_PENDING` 也必须视为不可开票状态；同步补入 `ListEligibleOrders` 的不可开票原因统计，避免展示统计与实际资格判断不一致。
- `invoiceRepository.Release` 现在先 Trim 原因并限制为最多 1000 个 rune，与 `Reject`、替换文件和作废操作及数据库 `admin_note VARCHAR(1000)` 的约束一致；空值或超长值在开启事务前稳定返回 `INVALID_INVOICE_STATUS_TRANSITION`。
- 新增 repository 回归测试：六种退款状态（含 `REFUND_PENDING`）均被拒绝；超长 Unicode 释放原因不会触发数据库访问。已通过 `go test ./internal/repository -run 'Test(ValidateInvoiceOrders|InvoiceRelease|EnsureInvoiceEnabledInTx)' -count=1 -p=1` 与 `go vet ./internal/repository`。
- 本轮仅修改目标 fork 的发票 repository/测试和本记忆文档；未修改旧项目、`main`、服务器、生产 PostgreSQL/Redis，未部署、重启、切流或开启发票开关。Release 仍按已批准规划允许其他管理员释放，不新增接单人限制。
- 后续边界修复：`adminTransition` 对 `Note` 优先、`Reason` 兜底后的最终 `admin_note` 统一 Trim 并限制最多 1000 个 rune，避免管理员提交超长 Note 绕过 Release/Accept 的字段校验而触发数据库长度错误；新增 Note 优先与 Reason 兜底两条前置拒绝测试。`go test ./internal/repository -run 'Invoice|invoice' -count=1 -p=1` 通过。

## 2026-09-03（Asia/Shanghai）— 本地收尾：奖励冲突目标与签到事务门禁

- 将 `backend/internal/repository/subnexus_invite_activities_repo.go` 两处奖励幂等写入从宽泛 `ON CONFLICT DO NOTHING` 收紧为 `ON CONFLICT (source, period, user_id) DO NOTHING`，并同步 sqlmock 断言；该目标由 `9002/9013` 的唯一索引契约提供，避免吞掉非预期唯一约束错误。
- 在 `backend/internal/service/subnexus_checkin.go` 的独立冻结结算事务中再次锁定并校验 `subnexus_checkin_enabled`；关闭、缺失、非法或读取失败均在余额写入前返回，避免关闭竞态。新增/更新 `subnexus_checkin_test.go` 覆盖关闭态无写入。
- Affiliate gate 修复（同一事务 `FOR UPDATE`、缺失/非字面 `true` fail-closed）及签到专项测试均通过；当前正在运行后端全量默认构建测试。前端全量 Vitest 279 文件/1946 测试和 typecheck 已通过。
- 本轮只修改目标 fork 工作树；旧项目、fork `main`、服务器、生产 PostgreSQL/Redis、远端分支和所有生产开关均未触碰。Release Gate 仍未通过，不能部署或推送。

## 2026-09-03（Asia/Shanghai）— 本地候选收尾、文档校准与前端开关加固

### 本轮目的

- 按维护者“先在本地完成全部迁移，再上传/部署”的顺序收敛五份精简文档，消除旧的 Batch 进行中、25 组 alias 和未开始状态描述。
- 固化当前事实：目标 fork 迁移分支 `feature/subnexus-migration`，HEAD=`23d6e8ec0e773e74146976a39f6573b3da68660a`；fork `main`=`d596d0844f274c3e7933c966231851f9f20b0d47` 未修改；`upstream/main`=`5097b31457e6dc9f49e5f5c9c72b925ce79543b3` 已同步。

### 文档与代码变更

- 更新 `SUBNEXUS_FEATURE_MATRIX.md`：登记 F01-F13（签到、排行榜、活动中心 custom、跑马灯、首充、邀请活动/注册奖励、发票、Battle Pass、学生优惠、注册 IP 冷却、Channel Monitor V3、默认语言、客服弹窗），并统一标记为“本地实现完成，待最终证据/维护者验收”；明确每日消耗转盘、红包雨、运行日历和 Media Studio/Creative Workshop 不迁移。
- 更新 `SUBNEXUS_MIGRATION_LEDGER.md`、`SUBNEXUS_PROJECT_CONTEXT.md`、`SUBNEXUS_MIGRATION_PLAN.md`：Batch 1-4 本地代码完成，Batch 5 的 Docker、隔离 PostgreSQL/Redis、候选启动和旧版本回归待执行；登记 `9001`-`9013` 及 `SHA256(TrimSpace(SQL))`；alias 从历史 25 组扩展为当前 27 组（23 内容映射、2 语义接管、学生优惠/注册冷却各 1 组）。
- `frontend/src/utils/featureFlags.ts` 在 `publicSettingsLoaded` 非 true 时强制 fail-closed，防止请求失败后 stale cache 放行；补齐 invite activities、marquee、first recharge、student benefit 的 registry 项，并新增 `frontend/src/utils/__tests__/featureFlags.spec.ts`。

### 验证

- 后端：`go test -tags unit ./... -count=1 -p=1 -timeout=30m`、`go test ./... -count=1 -p=1 -timeout=45m`、重点 `go vet` 通过。
- 前端：Vitest 280 个文件/1950 个测试、`pnpm typecheck`、`pnpm build` 和 feature flag 定向测试通过；构建仅有既有 Browserslist、动态/静态 import 与 chunk size 警告。
- 迁移契约、关闭态、并发、事务 gate 和 SQL checksum 已在本地验证；当前未执行 Docker 候选、隔离 Redis 恢复和旧版本回归。
- 本机 `docker compose version` 可用，但 Docker daemon 连接 `npipe:////./pipe/dockerDesktopLinuxEngine` 失败（daemon 未运行）；未尝试启动 daemon、创建容器或连接任何外部服务，因此 Batch 5 运行时证据仍为待办。

### 安全边界与下一步

- 本轮只读/写 `F:\MySub2\sub2api` 迁移工作树；`F:\Sub2Api\SubNexus` 仍只读。未修改 fork `main`，未推送当前改动，未访问服务器、线上 PostgreSQL/Redis，未执行 SQL、备份、部署、重启、切流或生产开关修改。
- 所有迁移功能及新增 rollout gate 默认关闭；Release Gate 尚未通过，当前候选不得上传或让服务器拉取。
- 下一步仅在本地执行 Batch 5 运行时验证并等待维护者验收；验收后才按规划生成新的发布 SHA 和线上命令。

## 2026-09-03（Asia/Shanghai）— 本地候选最终代码复核与 Docker 运行时门禁

### 本地操作与结果

- 重新执行 `go generate ./cmd/server`，Wire 生成文件与当前 provider 接线一致。
- 通过 `go test -tags unit ./... -count=1 -p=1 -timeout=30m`、`go test ./... -count=1 -p=1 -timeout=45m`、`go vet ./...` 和迁移契约测试；后端退出码均为 0。
- 通过 `pnpm typecheck`、`pnpm vitest run`（280 个文件/1950 个测试）和 `pnpm build`。首次全量 `pnpm lint:check` 发现新增 `InvoicesView.vue` 三处多余分号，已做最小修复；修复后完整 `pnpm lint:check` 和该文件 ESLint 均通过。
- 通过 `git diff --check`、gofmt、迁移 `9001`–`9013` checksum（13/13）和敏感文件扫描；未生成二进制、数据库、日志、`.env` 或构建产物。

### Docker 门禁与安全边界

- 根目录没有 compose 文件；使用 `deploy/docker-compose.local.yml` 和 `deploy/docker-compose.yml` 配合 `.env.example` 做静态 `docker compose config --quiet`，两者均通过。
- 为完成本地验证仅启动了本机 Docker Desktop，未创建容器、未挂载项目数据、未执行 PostgreSQL/Redis、未连接任何外部服务。Docker Desktop 因本机 Inference manager 路径错误（日志中的 `<HOME>\\AppData\\Local\\Docker\\run\\dockerInference`）未提供可用 daemon；运行时候选、隔离 PostgreSQL/Redis 和旧版本回归仍为 Batch 5 待办。
- 本轮只写入 `F:\MySub2\sub2api`；旧项目 `F:\Sub2Api\SubNexus`、fork `main`、服务器、生产 PostgreSQL/Redis、生产开关和远端分支均未触碰。未推送、未部署、未执行服务器命令。

### 未完成项与回滚点

- 当前代码候选可提交但不能宣称 Release Gate 通过；维护者验收前继续保持所有功能关闭。
- 若本地 Docker 恢复，优先运行隔离 compose 健康检查和旧版本回滚克隆；失败不影响旧线上版本。应用回滚仍使用提交前代码 SHA，数据库不自动恢复。

## 2026-09-03（Asia/Shanghai）— 本地迁移候选提交固定

- 将当前 311 个迁移代码、测试、SQL 和项目记忆文件固定为本地提交 `b26c42e08fb190f3915f08949aaaba48dbe61a26`（`feat: migrate SubNexus features to upstream baseline`）。提交前 `git diff --cached --check`、gofmt、敏感扫描、依赖校验和全量测试均已通过。
- 提交后工作树干净，当前分支仍为 `feature/subnexus-migration`，相对 `origin/feature/subnexus-migration` 仅本地领先；未推送、未修改 `main`、旧项目或服务器。
- 该提交是可回滚的本地候选，不代表 Release Gate 通过；所有迁移开关保持关闭，Docker/隔离 PostgreSQL/Redis/旧版本回归仍受本机 Docker daemon 故障阻塞。

## 2026-09-03（Asia/Shanghai）— 提交后文档字段校准

- 将上下文和台账中的“当前 HEAD”改为明确的“功能代码候选 SHA=`b26c42e08fb190f3915f08949aaaba48dbe61a26`”，并说明文档收尾提交应以 `git rev-parse HEAD` 实时获取，避免把文档提交误当作代码回滚点。
- 本次仅修改迁移文档并提交；未改变业务代码、迁移 SQL、开关默认值或依赖，未访问旧项目可写路径、服务器、线上数据库/Redis，未推送。

## 2026-09-03（Asia/Shanghai）— Batch 5 隔离 PostgreSQL 接管矩阵

- 在本机专用 PostgreSQL 16 集群 `F:\MySub2\.subnexus-pg16-20260903`（仅监听 `127.0.0.1:56000`）完成 Batch 5 的数据库子门禁：目标迁移集合 290/290 成功，旧项目迁移集合 268/268 成功。
- 使用旧库执行当前 runner 的首次接管、第二次幂等运行以及旧迁移集合重复校验；接管后 `schema_migrations` 共 371 条，目标迁移 checksum（兼容 alias 白名单除外）匹配，invalid index 数量为 0。
- 临时运行时测试和测试数据均位于本机隔离集群，已移除临时测试文件；没有访问 `F:\Sub2Api\SubNexus` 的可写路径，没有连接线上 PostgreSQL/Redis，没有执行生产 SQL、备份、部署、重启、切流或开关修改。
- 当前 Batch 5 仍未全部完成：隔离 Redis、候选应用启动/健康检查、Docker 镜像运行和旧版本回滚克隆待验证；所有迁移功能继续保持默认关闭。

## 2026-09-03（Asia/Shanghai）— Batch 5 运行时 smoke 与旧版同库回归收口

### 本轮目的

- 在不触碰生产和旧项目写路径的前提下，补齐候选主机进程、隔离 Redis 和旧版回滚克隆的运行时证据，并把文档从“全部受 Docker 阻塞”校准为分项状态。

### 已执行与结果

- 隔离 PostgreSQL `127.0.0.1:56000` 继续使用 `F:\MySub2\.subnexus-pg16-20260903`；目标 290/290、旧集合 268/268、同库接管后 `schema_migrations=371`、checksum/索引契约均通过。
- 使用仅监听 `127.0.0.1:56379` 的 miniredis 完成候选主机进程 smoke（候选端口 `18180`）：health、setup、自动初始化、管理员登录、全部关闭态检查和二次启动通过。该 Redis 为临时非持久化夹具，不能替代生产 Redis 恢复演练。
- 从 `F:\Sub2Api\SubNexus\backend\server.exe` 的旧版本 `0.1.135` 在 `subnexus_old_regression_login_20260903` 克隆（`schema_migrations=371`、users=1、settings=52）启动 `18183`；首轮 PID `46736`、重启后 PID `47404`。两次启动的 health/setup/public settings、有效管理员登录、旧/新 token 的 `auth/me` 和管理员只读 GET 均通过；旧版不识别新增签到/排行榜/邀请活动路由而返回预期 404，数据管理弃用代理返回预期 503。
- 旧版回归前后 users=1、settings=52、schema_migrations=371、合规记录=1；没有重复创建用户或迁移，audit_logs 仅因第二次登录增加一条预期记录。新增活动/发票/Battle Pass 等表未被旧版删除。证据日志和计数文件保存在本机 `.old-version-regression-20260903\clone\`。
- 旧版启动日志显示会读取 GitHub model-price-repo；这是本机测试配置的外部只读依赖，生产发布前需单独确认价格文件缓存/网络策略，不得把该访问误认为连接线上业务。

### 尚未完成

- Docker daemon 仍因本机 Inference manager 路径错误不可用；未创建容器或镜像。持久化 Redis/AOF 或 RDB 恢复、生产 PostgreSQL（历史记录的 PostgreSQL 18 版本需实时确认）备份隔离克隆、Docker 候选镜像验证和上游核心回归仍是 Release Gate 前置条件。
- 当前所有迁移开关继续默认关闭；候选代码功能提交仍为 `b26c42e08fb190f3915f08949aaaba48dbe61a26`，文档收尾尚未提交/推送，不能让服务器拉取或执行服务器命令。

### 安全与回滚

- 本轮只使用 `F:\MySub2\sub2api`、本机隔离 PostgreSQL/miniredis 和临时旧版进程；未写入 `F:\Sub2Api\SubNexus`，未触碰 fork `main`、PID `79272`/端口 `18080`、Memurai PID `4644`/端口 `6379`，未连接线上 PostgreSQL/Redis，未执行生产 SQL、备份、部署、重启、切流或开关修改。
- `.old-version-regression-20260903`、`.rollback-validation-*`、`.subnexus-pg16-*` 等日志、二进制和数据库目录是本轮本地隔离测试产物，已被 `.gitignore` 排除；收口时必须按 PID/路径精确停止临时进程并在确认审计需要后再删除目录，禁止使用宽泛递归删除。

## 2026-09-03（Asia/Shanghai）— 运行时收尾、资源停用与文档提交

- 旧版回归、回滚验证、临时 miniredis 和隔离 PostgreSQL 进程已按可执行路径/端口精确停止；停止前确认没有进程仍引用目标目录。
- 受保护的本地业务进程 PID `79272`（端口 `18080`）和 Memurai PID `4644`（端口 `6379`）始终保持运行；旧项目 `F:\Sub2Api\SubNexus`、fork `main`、服务器和生产 PostgreSQL/Redis 均未触碰。
- 本地编译级复核 `go test ./... -run '^$' -count=1 -p=1` 和前端 `pnpm typecheck` 通过；13 个目标迁移 checksum 复核无不匹配，`git diff --check` 通过。
- 文档/矩阵收尾提交链起点为 `95ac9f02044c25a1a681b516596ea2214b1fe8dc`；当前文档提交以 `git rev-parse HEAD` 实时查询。功能代码候选仍为 `b26c42e08fb190f3915f08949aaaba48dbe61a26`，当前分支 `feature/subnexus-migration` 相对远端领先，未推送。
- `.old-version-regression-20260903`、`.rollback-validation-*`、`.subnexus-pg16-20260903`、`.runtime-*` 和本轮专用编译缓存已停止使用并保留在 `F:\MySub2` 作为本地审计材料；未纳入 Git。由于其中包含原始日志/数据库快照，本轮不做不可恢复删除，后续清理必须按绝对路径逐项确认后执行。
- Release Gate 仍未通过：Docker daemon/候选镜像、持久化 Redis 恢复、生产 PostgreSQL 实际版本与备份隔离克隆、上游核心回归和维护者验收未完成；所有迁移开关继续默认关闭，禁止服务器拉取或执行服务器命令。
- 应用回滚仍优先使用候选前代码 SHA，数据库不自动恢复；只有确认数据损坏且获得明确批准时才使用已验证备份。

## 2026-09-03（Asia/Shanghai）— 上下文索引补充

- 在 `SUBNEXUS_PROJECT_CONTEXT.md` 的当前状态表新增本地测试产物说明：隔离日志、数据库快照和缓存已停止使用，保留在 `F:\MySub2`、未纳入 Git，也不是生产资产。
- 本次仅修改项目记忆文档；功能代码候选、迁移 SQL、默认关闭开关、fork `main`、旧项目和线上服务均未改变。

## 2026-09-03（Asia/Shanghai）— 再次复核 SubNexus 渠道监控同步状态

### 复核范围与结论

- 只读检查旧项目 `F:\Sub2Api\SubNexus` 当前分支 `alignment/v0.1.181-local` / HEAD `62ea35e1c78416fd83e1e41bbb310b307941811a`（`fix(monitor): improve V3 availability timeline`）。该提交除旧项目自己的 `AI_CHANGE_MEMORY.md` 外，只涉及 7 个前端渠道监控源码/测试文件。
- 对照目标 fork `F:\MySub2\sub2api` 的 `feature/subnexus-migration`：7 个源码/测试文件中 5 个与旧提交逐字节一致；`monitorFormat.ts` 仅注释不同，`monitorFormat.spec.ts` 仅测试位置/缩进不同，V3 阈值（90/80）和全部断言行为一致。后端、迁移 SQL、配置、路由没有该提交新增差异。
- 结论：这次旧项目的渠道监控 V3 时间线修正已经包含在目标候选 `b26c42e08fb190f3915f08949aaaba48dbe61a26` 中，无需重复 cherry-pick 或覆盖代码；保持目标现有实现，避免引入只为追求字节一致的无行为改动。

### 本地验证与安全边界

- 在目标 fork `frontend` 执行 `pnpm exec vitest run src/features/channel-monitor-v2/__tests__/monitorFormat.spec.ts src/features/channel-monitor-v2/__tests__/monitorTimeline.spec.ts`：2 个文件、16 个测试全部通过。
- 对 7 个受影响文件执行 ESLint：通过；仅有现有 TypeScript 版本兼容性提示，无 lint 错误。`git diff --check` 通过，目标工作树在记录前无业务源码差异。
- 本次全程只读旧项目；未修改旧项目文件、fork `main`、服务器、线上 PostgreSQL/Redis、生产开关或部署状态，未推送任何新提交。所有迁移功能继续默认关闭。

### 当前回滚点与下一步

- 功能代码回滚点仍为 `b26c42e08fb190f3915f08949aaaba48dbe61a26`；本次只新增审计记忆，不改变代码回滚点。
- Docker、持久化 Redis 恢复、生产备份隔离克隆、维护者验收等 Release Gate 仍未完成；在这些门禁通过前不得让服务器拉取或切流。

## 2026-09-03（Asia/Shanghai）— 主审报告独立复核与确认问题修复

### 本轮范围

- 仅修改 `F:\MySub2\sub2api` 的 `feature/subnexus-migration` 工作树；`F:\Sub2Api\SubNexus`、fork `main`、服务器和生产 PostgreSQL/Redis 均保持不动。
- 对主审报告逐项复核后，修复了可以由代码证据直接确认的问题；首充退款资格和关闭态过期清理仍保留为待产品确认的既有语义，活动中心/跑马灯写入竞态和注册冷却设置读取异常已按 fail-closed 方式修复。

### 已修改

- 新增 `frontend/src/views/admin/CheckInSettingsView.vue`，只包含签到策略配置；新增 `/admin/checkin` 管理路由、侧栏入口及中英文 i18n。页面在功能关闭时仍可读取/保存策略，真正奖励写入仍由服务端独立开关和合法 JSON 双重门控。
- `setting_public.go` 与 runtime 使用同一合法模式校验；非法 `channel_monitor_mode` 现在对公开设置和运行时均 fail-closed，不再出现前端显示 V1 但后端关闭探测的分叉。
- 客服 Markdown 恢复显式标签/属性/协议白名单，限制 data URL 为图片，并在保留 `target=_blank` 时强制补 `noopener noreferrer`。
- 签到奖励日志改为 `ON CONFLICT (source,period,user_id) DO NOTHING`，与唯一索引契约一致，避免吞掉非预期唯一冲突。
- 发票配置、发票状态/文件/邮件变更和学生优惠配置、grant/revoke 路由接入 step-up；对应前端调用接入 `useStepUp`，服务端要求时弹出 TOTP 后重试同一请求。
- 将上下文、迁移计划、台账和变更记忆中的旧项目参考 SHA 从过期的 `ccffee6c6` 校正为 `62ea35e1c78416fd83e1e41bbb310b307941811a`。
- 待定 OAuth 完成现在对注册冷却开关读取错误直接失败，不再在设置存储不可用时跳过 reservation finalize；正常关闭或缺失设置仍不访问冷却表。

### 验证边界

- 本轮修改尚未推送，功能开关仍保持默认关闭；前端全量 282 个测试文件/1954 个测试、Go 定向 service/repository/routes 测试和后端全量门禁均通过，仍需维护者重新审核。

## 2026-09-03（Asia/Shanghai）— 主审复核后的后端全量门禁收尾

- 在 `F:\MySub2\sub2api\backend` 执行 `go test ./... -run '^$' -count=1 -p=1 -timeout=45m`，退出码 0；全仓 Go 包编译级检查通过。
- 执行 `go vet ./...`，退出码 0；未发现静态分析问题。
- 执行 `go test -tags unit ./... -count=1 -p=1 -timeout=30m`，退出码 0；所有带 `unit` 标签测试通过，`internal/service` 用时约 170.7 秒。
- 本轮没有访问线上服务器、生产 PostgreSQL/Redis，也没有修改 `F:\Sub2Api\SubNexus`、fork `main` 或生产开关；当前改动仍是 `feature/subnexus-migration` 的本地未提交工作树变更。
- Release Gate 仍未通过：生产备份隔离恢复、持久化 Redis 恢复、Docker 候选镜像和维护者验收尚未完成；不得据此推送、部署或切流。

## 2026-09-03（Asia/Shanghai）— 第二轮主审剩余项全部收口

### 本轮修改

- 仅修改 `F:\MySub2\sub2api` 的 `feature/subnexus-migration` 工作树；未修改 `F:\Sub2Api\SubNexus`、fork `main`，未访问服务器、生产 PostgreSQL/Redis，也未执行部署或开关变更。
- 简易模式直链限制补入 `/admin/checkin` 和 `/admin/leaderboard`，并保留管理员重定向到 dashboard 的既有行为。
- 签到配置、排行榜配置和排行榜奖励三个管理写入口接入 `StepUpAuthMiddleware`；签到/排行榜管理页使用 `useStepUp` 和 `TotpStepUpDialog`，支持验证后单次重试、取消静默返回以及 TOTP 未启用/API key 禁止的明确提示。
- 敏感路由回归测试扩展到学生优惠、发票全部写入口，以及签到/排行榜全部敏感写入口，共 14 条，均验证先返回 `428 Precondition Required`。
- 删除会吞掉设置读取错误的未使用 `AuthService.registrationIPCooldownEnabled` helper；OAuth 回滚在调用方明确绑定 reservation 时直接释放，避免二次设置读取造成错误吞没或孤儿 reservation。
- 新增首充退款语义回归测试：已完成首充订单即使状态变为 `REFUNDED`，`PrepareOrder` 仍返回 `ErrFirstRechargeAlreadyPurchased`，不恢复促销购买资格。关闭态 terminal reservation 清理继续保留为独立后台补偿，只处理终态预约，不发奖、不写用户业务数据。
- 同步 `SUBNEXUS_FEATURE_MATRIX.md`、`SUBNEXUS_MIGRATION_PLAN.md` 和 `SUBNEXUS_MIGRATION_LEDGER.md`，将 P3-01 至 P3-04 及 F05 两项语义从待确认改为明确实现/测试策略。

### 定向验证

- `go test -tags unit ./internal/server/routes ./internal/service -run 'Test(SubNexusSensitiveAdminRoutesRequireStepUp|FirstRechargeRefundDoesNotRestorePurchaseEligibility)' -count=1 -p=1 -timeout=15m` 通过。
- `frontend`: `pnpm exec vitest run src/views/admin/__tests__/CheckInSettingsView.spec.ts src/views/admin/__tests__/LeaderboardSettingsView.spec.ts src/router/__tests__/guards.spec.ts` 通过，3 个文件/40 个测试。
- 全量门禁仍需在提交前重新执行；所有迁移功能继续默认关闭，Release Gate（生产备份隔离恢复、持久化 Redis、Docker、维护者验收）仍未通过。

## 2026-09-03（Asia/Shanghai）— 剩余项收口后的全量门禁

- 后端最终执行并通过：`go test ./... -run '^$' -count=1 -p=1 -timeout=45m`、`go vet ./...`、`go test -tags unit ./... -count=1 -p=1 -timeout=30m`。
- 前端最终执行并通过：`pnpm typecheck`、`pnpm test:run`（282 个文件/1954 个测试）、`pnpm lint:check`、`pnpm build`。构建仅输出已有的 Browserslist、chunk size 和动态导入提示，没有错误。
- `git diff --check` 通过；构建生成的 `backend/internal/web/dist` 未产生 Git 工作树变更。提交前将再次核对 staged 文件，只包含本次迁移代码、测试和文档。
- 当前仍只允许本地提交：不推送、不连接服务器或生产 PostgreSQL/Redis、不执行线上迁移/部署/切换；所有迁移开关默认关闭，Release Gate 仍需维护者另行完成。

## 2026-09-03（Asia/Shanghai）— 本地提交完成

- `feature/subnexus-migration` 已创建本地提交 `fix: close remaining SubNexus migration review items`；最终提交 SHA 以 `git rev-parse HEAD` 为准（本次记忆更新随同提交 amend）。
- 提交范围为第二轮主审剩余项的实现、回归测试及迁移文档；未包含构建产物、密钥、环境文件或线上证据。
- 提交后仍不推送、不部署；维护者验收和 Release Gate 完成前，服务器不得拉取该分支。

## 2026-09-03（Asia/Shanghai）— Release Gate 线上只读基线与候选分支发布

### 本轮操作

- 通过 SSH 只读连接 `ubuntu@51.81.211.97`，未执行服务器拉代码、SQL/DDL/DML、备份、配置写入、重启、切流或生产开关修改。
- 目标分支 `feature/subnexus-migration` 已推送到 fork `origin`，当前候选提交为 `90d7d4b502fd88bc853b4dd9c4b1cd1fbf659838`；`main` 和旧项目保持不变。
- 线上只读事实：应用容器 `subnexus-cutover`，镜像 `subnexus-git:62ea35e1-20260901135157`，健康，绑定 `127.0.0.1:18083 -> 8080`，数据目录 `/srv/subnexus-migration/runtime/subnexus-data -> /app/data`；PostgreSQL 容器 `sub2api-postgres`（18.4）和 Redis 容器 `sub2api-redis`（8.8.0）均运行中。
- Nginx `subnexus_backend` 当前指向 `127.0.0.1:18083`。线上 PostgreSQL 数据库 `sub2api` 约 `67 GB`，只读基线计数：users=1670、accounts=1677、user_subscriptions=24、payment_orders=3210、channels=14、redeem_codes=3688、usage_logs=11343932、activity_reward_logs=13697、hourly_red_packet_rounds=85。
- 线上 `schema_migrations` 最新可见记录为 `254_battle_pass.sql`；`atlas_schema_revisions` 存在历史基线记录。Redis 为 standalone、AOF 关闭、db0 约 50848 keys。`ops_preaggregation_hourly`、`ops_preaggregation_daily`、`ops_metrics_collector`、`ops_alert_evaluator` 有近期成功心跳，切换前需再次确认无运行中的结算/迁移任务。
- 服务器已有旧镜像、旧容器和历史备份资产；磁盘约 193G/150G（78%），可用约 43G。新备份前必须先检查空间和备份恢复策略，禁止 `prune` 或删除旧回滚资产。

### 当前门禁与下一步

- 生产仍未迁移、未切流、未开启任何迁移功能；以上仅为基线，不代表 Release Gate 已通过。
- 下一步由维护者在服务器终端手动执行经过提交 SHA 与脚本 SHA256 校验的只读 preflight。预检只写 root-only 证据目录，不改变业务状态；回传脱敏证据后再生成备份/隔离恢复/候选镜像命令。
- 回滚原则不变：优先保留旧容器和旧镜像，通过应用/Nginx 切回，不恢复数据库；只有确认数据损坏并得到明确批准时才使用经校验的备份。
