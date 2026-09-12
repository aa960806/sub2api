# SubNexus 操作与变更记忆

> 这是追加式项目记忆，不得重写或删除历史条目。每次代码、测试、配置、迁移、文档、部署或诊断操作完成后，必须在本文件末尾追加一条记录。
>
> 详细当前架构见 `SUBNEXUS_PROJECT_CONTEXT.md`；批次状态见 `SUBNEXUS_MIGRATION_LEDGER.md`。
>
> 当前权威状态（2026-09-12 00:45 Asia/Shanghai）：本轮模型监控修复候选 `ccb69f00132d` 全部发布前置已完成，run `20260911163046-3338612` 为 prepared，代理未切换。最新授权不新建回滚目标，复用既有 v0.2.4 容器 `e389b3b1c4f6`；实际线上仍为 `33a9601c9330` / `b4b66b9ca08f`。本轮唯一人工切换命令见切换手册第 15.5 节，所有历史已消费 switch 命令不可重用。

## 2026-09-06（Asia/Shanghai）— 修复 wrapper manifest SHA 后最终前置完成

- UI wrapper 修复提交=`7d51ea811`；服务器脚本 `/srv/subnexus-migration/tools/subnexus-ui-cutover-0d6d2089-20260906.sh`，SHA=`0d6d208962e55f2aa75afdb2b490a6652df3c856e4ec132b4af974a942064ca9`。UI run 使用 wrapper SHA 校验，固定旧 anchor 继续使用原 controller SHA=`19824a87e3e1de5659cb30664750b71c5c10d374f25bda7f52e6524fe477ee65`。
- 最终 run=`/srv/subnexus-migration/cutover/20260906090405-624283`，PID=`624283`，`READY=prepared`，manifest SHA=`7a86d1127e80501110e21af7f688ea98b0c4db0016cf7018b8b7888eb4e13eca`；settings-before=`575bb5c0081341e1d9e8fb54241d491e0e702e26d24bfe7d70412ba22cf741ea`，closed=`6a0ed24c164bb1fa8ebb5edecb8712f77458bfca145c713adfd105257fcda8c3`。
- 备份 sidecar 已通过：PostgreSQL=`dd0f237410dd874b27f88a494fe2725ae9be017b4ca3406834818b7320737c96`，Redis=`f4e5e56424083c5daee02530bfb09dd88b16cf749485c4ff5aa761372d549027`，应用数据=`cbcb055aa41faa3e9e3b7f5808e3c8e709f075d8eb2969fe847e2369c426c093`。
- stopped probe ID=`a73b1505dc3a67c17ae31a6d500d6a576bf14f0b753e03ee2ebc4941e4c2a48c`，状态严格为 `created|false|0|0001-01-01T00:00:00Z`，已按完整 ID 删除；probe evidence=`/srv/subnexus-migration/diagnostics/probe-rain-20260906090405-624283.evidence`，SHA=`e958f8fb5fb4fafcd589b5526dcedd3497df08af5e0ed2d6f25c06ba2ce30701`。无 candidate/probe 残留，应用、PostgreSQL、Redis 与旧 anchor 未改变，未执行 switch/rollback。

## 2026-09-06（Asia/Shanghai）— 受保护设置哈希修复后的最终 UI 前置完成

- 已推送 wrapper 修复提交 `2e60d0d55`；线上 wrapper `/srv/subnexus-migration/tools/subnexus-ui-cutover-7c3a42ac-20260906.sh`，SHA256=`7c3a42ac381f3839b5de5d605d465ee13b005ea9321b28ef47427ece2e910d77`。`ui_settings_hash` 只覆盖 rollout/content/invite 受保护键，避免正常通知和内容设置写入造成误报。
- 最终 prepare run `/srv/subnexus-migration/cutover/20260906100431-660485`，PID=`660485`，`READY=prepared`，`state=prepared`，`ui_state=prepared`，`ui_commit_intent=no`；manifest SHA=`4cdd0bac0157663f9f485847ac92d5cd09d3f6a66b90def09f95f3389c4570b6`。
- `settings-before` 与受保护 UI settings hash 均为 `575bb5c0081341e1d9e8fb54241d491e0e702e26d24bfe7d70412ba22cf741ea`；closed settings SHA=`6a0ed24c164bb1fa8ebb5edecb8712f77458bfca145c713adfd105257fcda8c3`。
- stopped probe ID=`1942a6ddf23e50efe23fa2029ac26f9794da202e3bc1479fa88b341a204ce159` 已精确删除；probe evidence SHA=`f253bc1e057f32b4882f8573a10ed9a5d1c524714fe4318a3078f1df8abbd703`。生产应用、PostgreSQL、Redis 健康，无 candidate/probe 残留，未执行 switch/rollback。
- 定向清理失效 run `624283` 的备份及 sidecar，释放约 5.56 GB；清理 evidence `/srv/subnexus-migration/cleanup-ui-invalid-624283-20260906.txt`，SHA256=`e98d82faee2bb49a256b9dbf69b936a1c44377741cb09ae4f52b6ee56a68a360`。未使用 `docker prune`，未删除固定旧回滚对象。

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

## 2026-09-03（Asia/Shanghai）— 线上只读预检首次失败与脚本修复

### 结果

- 维护者手动执行发布提交 `f0c57c01a615748029758048674ef60cdc3c3a3d` 的只读 preflight。候选仓库已成功拉取并固定到该 SHA；现有生产应用、PostgreSQL、Redis、Nginx 和开关均未被修改。
- 预检在 Redis 检查阶段停止：线上 Redis 未配置密码，脚本却把空值设置为 `REDISCLI_AUTH`，Redis 8 返回 `ERR AUTH <password> called without any password configured`。该错误是脚本认证处理缺陷，不代表 Redis 数据丢失或服务异常。
- 服务器上失败的候选目录和证据目录均保留，未删除或覆盖；没有执行数据库迁移、备份、重启或切流。

### 修复与验证

- `tools/production-deploy/subnexus-readonly-preflight.sh` 现在仅在密码非空时导出 `REDISCLI_AUTH`，无密码时保持未设置；静态回归断言已补齐。
- 使用 Git for Windows Bash 执行 `tools/production-deploy/subnexus-readonly-preflight.test.sh`，测试通过。
- 修复后脚本 SHA256：`8B4B05D30E9E95D518F246CFCAA3F8B52AE2E2DA056A1744159CAF945C15D922`。待提交并推送新的发布 SHA 后重新执行只读 preflight。
- 修复已提交并推送：`745ec2a7fd2a92549e74e86151a9e0c19c15ceb9`；服务器重跑必须校验该 SHA 和上述脚本 SHA256。此前失败的候选目录继续保留，不作为新版本使用。

## 2026-09-03（Asia/Shanghai）— 线上只读预检第二次失败与 Redis 模式解析修复

- 维护者重跑 `745ec2a7fd2a92549e74e86151a9e0c19c15ceb9` 后，Redis 无密码认证已不再报错，但脚本把 `INFO server` 返回的 `redis_mode:standalone`（含隐藏 CR/空白）判定为非 standalone；预检因此安全停止，未执行迁移、备份、重启、切流或开关修改。
- 已将模式解析改为先去除 CR、再 trim 空白后严格比较 `standalone`，并在 `tools/production-deploy/subnexus-readonly-preflight.test.sh` 增加回归断言；Git for Windows Bash 静态测试通过。
- 当前服务器上的两个候选仓库/证据目录均保留；修复提交生成并推送后，必须使用最终 release SHA 和脚本 SHA256 重跑预检。

### 只读确认

- 通过 SSH 在 Redis 容器内读取 `INFO server` 并以十六进制/可见字符检查，确认线上返回值为 `redis_mode:standalone\\r\\n`；问题确实是 CRLF 解析而非 Redis 模式异常。该检查未改变 Redis 状态。
- 最终预检发布提交：`d40a3c43c48c94d3a26a800bcd9415f94a8c192f`；预检脚本 SHA256：`F02D5CB4A5F454E78663F56CD023ACE0EB8FEA5A978CAC8E4155805A697B6E87`。

## 2026-09-03（Asia/Shanghai）— 线上只读预检与生产备份门禁通过

### 预检证据

- 当前远端发布指针为 `1da1e85dd7be761b22cd219c2c93d92fd48c6bcf`；预检脚本 SHA256 为 `F02D5CB4A5F454E78663F56CD023ACE0EB8FEA5A978CAC8E4155805A697B6E87`。
- 维护者在生产服务器手动执行只读预检；证据位于 `/srv/subnexus-migration/preflight/20260903072817/evidence.txt`，同目录 SHA256 校验通过。最终标记为 `FINAL_RUNTIME_IDENTITIES=passed`、`FINAL_SCRIPT_INTEGRITY=passed`、`NO_MIGRATION_OR_DEPLOYMENT_PERFORMED=true`。
- 预检确认生产应用 `subnexus-cutover` 仍健康并绑定 `127.0.0.1:18083 -> 8080`，Nginx 上游仍为该端口；PostgreSQL `sub2api-postgres` 为 18.4，Redis `sub2api-redis` 为 8.8.0 standalone、无密码、AOF 关闭。

### 备份结果与修正记录

- 维护者在生产服务器手动创建 root-only 备份目录 `/srv/subnexus-migration/backups/20260903T073714Z`。没有停止、重启或替换应用、PostgreSQL、Redis、Nginx，也没有执行 SQL DDL/DML、数据库迁移、切流或功能开关修改。
- PostgreSQL custom dump 为 `postgres-sub2api.dump`，精确大小 `4,881,209,204` bytes（终端汇总约 `4.6G`），文件头为 `PGDMP`，`file` 识别为 PostgreSQL custom database dump v1.16-0。由于 `pg_restore` 经 `docker exec` 标准输入读取失败，改用无网络临时容器和只读 bind mount 执行 `pg_restore --list`；`postgres-restore-list.txt` 最终非空且命令退出成功，未重新导出数据库。
- 已生成并验证 `postgres-globals.sql`、排除 `subnexus-data/logs` 的 `subnexus-data.tar.gz`（约 `968K`）、Redis `BGSAVE` 后的 `redis-dump.rdb`（约 `7.7M`）。Redis RDB 使用同镜像、无网络临时容器运行 `redis-check-rdb` 成功；应用数据归档通过 `tar -tzf`。
- `COMPLETE`、`backup-metadata.txt`、`disk-after.txt` 和 `SHA256SUMS` 已生成，清单内全部文件逐项返回 `OK`，最终输出 `BACKUP_COMPLETE`。备份完成后应用健康、PostgreSQL/Redis 运行、Nginx 配置与服务状态均通过；服务器磁盘约 `193G/156G`，剩余约 `38G`。
- 首次进度检查使用 `pgrep -af`，把运行命令中的数据库密码显示在终端。敏感值未写入仓库或本地迁移文档；后续禁止采集/传播完整进程参数，相关密码只允许通过标准输入传给容器内客户端。当前不轮换密码以免打断生产，受控切换完成后必须安排轮换。

### 门禁与下一步

- B0-5 线上只读状态门禁通过；B0-6 生产 PostgreSQL/Redis/应用数据备份创建及结构完整性校验通过。该结论不等于恢复演练通过。
- B0-7 仍在进行中：必须把备份下载到本机 `F:` 盘，先核对 `SHA256SUMS`，再用 PostgreSQL 18 和 Redis 8 隔离实例完成实际恢复、候选 adoption/迁移、关闭态 smoke 与旧版本回归。
- 生产服务器只剩约 `38G`，而源数据库约 `67 GB`，禁止在当前生产磁盘直接创建完整恢复副本。隔离恢复通过前不得启动连接生产库的候选、执行生产迁移、构建/替换生产容器、切流或开启任何迁移功能。
- 本次证据同步只修改 fork 内的迁移记忆、台账、上下文、规划、功能矩阵和切换手册，并创建本地文档提交；暂不推送。服务器已验证的远端发布指针继续固定为 `1da1e85dd7be761b22cd219c2c93d92fd48c6bcf`。

## 2026-09-03（Asia/Shanghai）— 生产备份本地隔离恢复准备

- 本机 `F:` 盘可用空间约 `196 GiB`，足够保存约 `4.6G` 的备份并恢复约 `67 GB` 的数据库；生产服务器只剩约 `38G`，继续禁止在生产盘创建完整恢复副本。
- Docker Desktop client 为 29.2.1，但 daemon 未运行。一次本地启动请求在 30 秒内未就绪，日志仍指向既有 Inference manager socket/path 初始化错误；没有修改 Docker 配置、镜像、卷或现有本地服务，也不再反复启动。
- 从 EnterpriseDB 官方 HTTPS 地址下载 PostgreSQL 18.4 Windows x64 便携二进制包到 `F:\MySub2\.tools`，本地 SHA256 为 `7EFFE34C0BF89027B3F171447D351CBC460F4566C8D0F643DAEC67F140787858`；已解压并确认 `pg_restore`、`postgres` 均为 18.4。该运行时不安装 Windows 服务、不修改注册表，也不使用本机现有 PostgreSQL 16 数据目录。
- 下一步对服务器 root-only 备份目录保存 ACL 快照，只临时授予现有 `ubuntu` SSH 账号读取/遍历权限；通过 SCP 下载到 `F:\MySub2\production-backups\20260903T073714Z` 并验证 SHA256 后立即按快照恢复 ACL。

## 2026-09-03（Asia/Shanghai）— 生产备份下载、PostgreSQL 18 隔离恢复与候选克隆

### 下载与完整性

- 生产备份通过一次性传输目录下载到 `F:\MySub2\production-backups\subnexus-backup-transfer-20260903T073714Z`；服务器 `SHA256SUMS` 登记的 20 个文件在本机全部匹配。
- 下载后的 `postgres-sub2api.dump` 可由 PostgreSQL 18.4 `pg_restore --list` 读取，`subnexus-data.tar.gz` 通过归档列表校验；没有把备份中的密钥、密码或完整配置写入 Git/记忆文档。
- 维护者确认原始 root-only 备份 `/srv/subnexus-migration/backups/20260903T073714Z` 仍存在后，精确删除 `/home/ubuntu/subnexus-backup-transfer-20260903T073714Z` 临时传输副本；终端返回 `TRANSFER_COPY_REMOVED_ORIGINAL_BACKUP_PRESERVED`。生产应用、PostgreSQL、Redis 和 Nginx 未停止或修改。

### PostgreSQL 18.4 隔离恢复

- 便携 PostgreSQL 18.4 集群位于 `F:\MySub2\.production-restore-20260903T073714Z\pgdata`，仅监听 `127.0.0.1:56418`；没有安装系统服务，也没有停止或复用本机现有 PostgreSQL 16/Memurai。
- custom dump 已完整恢复到只供对照的 `sub2api` 数据库，`pg_restore` 退出码为 0；随后使用 PostgreSQL `FILE_COPY` 克隆为可变更的 `subnexus_candidate`，原始恢复库保持不动。
- 两库迁移前均为 `schema_migrations=268`，最新记录 `254_battle_pass.sql`，无 invalid index；数据库大小约 `57 GB`。核心计数一致：users=1670、user_subscriptions=24、payment_orders=3214、channels=14、redeem_codes=3691、api_keys=3628。
- 两库余额合计均为 `159869.99893570`，累计充值均为 `1016953.07800000`。备份命令执行前的在线基线余额为 `159870.30972372`，相差约 `0.31078802`；这是持续营业期间基线查询与 `pg_dump` 一致性快照时点不同造成，其他核心计数及累计充值均与备份元数据一致，不作为恢复损坏。

### 当前门禁

- PostgreSQL 实际恢复子门禁已通过，但 `subnexus_candidate` 尚未运行当前 fork 的 migration/adoption runner；当前仍是未迁移的生产备份克隆。
- 生产 Redis RDB 尚未在独立 Redis 8 环境实际加载，候选关闭态 smoke、生产备份克隆上的旧版回归和 Docker 候选仍待完成。上述门禁通过前继续禁止生产迁移、候选连接生产数据库、切流或开启任何迁移功能。

## 2026-09-03（Asia/Shanghai）— 真实生产克隆 migration/adoption、候选启动与旧版回归

### 候选迁移与数据对账

- 从本地 HEAD `5286c950e2da6a2d83a580b6cb98e9542a7e8e90` 构建 Windows 候选 `0.2.0`，二进制 SHA256=`9E805ED1192420BA2D8A43E53AF507E3AAEA038C14CE818E9EDCA1DB21CE53D6`；构建产物位于 Git 仓库外的 `.production-candidate-20260903T073714Z`。
- 生产备份中 `channel_monitor_enabled=true`、`channel_monitor_mode=v3`，其余迁移开关不存在或为 false。为满足统一关闭门禁，仅在 `subnexus_candidate` 克隆把 `channel_monitor_enabled` 改为 false；`channel_monitor_mode=v3` 保留，原始恢复库仍为 true/v3。生产切换必须保存旧值后执行同类关闭，应用回滚时才能显式恢复旧运维状态。
- 使用候选代码自身的 `repository.ApplyMigrations` 在数据库名精确门禁为 `subnexus_candidate` 的临时构建标签测试中执行首次 migration/adoption 和第二次幂等运行，均通过；临时测试文件已删除且未进入 Git。
- 克隆迁移记录从 268 增至 371；`9001`–`9013` 共 13 条全部存在，27 个 alias 目标全部存在，第二次启动再次通过 checksum/对象契约校验，invalid index=0。由冻结 SQL 自动提取的 28 张表和 45 个索引全部存在。
- 迁移和候选/旧版启动后，users=1670、user_subscriptions=24、payment_orders=3214、api_keys=3628、channels=14、redeem_codes=3691，余额合计 `159869.99893570`、累计充值 `1016953.07800000`，均与原始恢复库一致。用户旧列、订单、订阅、API Key、渠道和兑换码整表摘要一致；users 仅新增 `frozen_balance`/`restrict_public_groups`，accounts 仅新增 `parent_account_id`/`quota_dimension`。

### 隔离启动与回滚结果

- 本机 Memurai 4.1.2（Redis API 7.2.5）对生产 RDB 执行只读检查时明确拒绝 RDB format 14，因此不把它作为 Redis 8 恢复证据。候选 HTTP smoke 只使用空白 Memurai `127.0.0.1:56419`，没有触碰本机已有 `6379`。
- 候选与旧版副本均绑定只允许 `127.0.0.0/8` 的临时出站防火墙规则；候选读取隔离应用数据，并通过非空环境变量把数据库固定到 `127.0.0.1:56418/subnexus_candidate`、Redis 固定到 `127.0.0.1:56419`。Windows 缺少 `Asia/Shanghai` zoneinfo 的首次启动在监听前退出，改用仅限本地测试的 `TZ=UTC` 后成功；不影响 Linux 生产镜像。
- 候选 `/health=ok`、`/setup/status needs_setup=false`；公开设置确认活动中心、签到、排行榜、跑马灯、邀请活动、首充、学生优惠、发票、Battle Pass、渠道监控全部为 false。生产克隆中 4 个到期的计划任务曾尝试外联，均被防火墙拒绝，并只在候选克隆产生 12 条失败测试结果；80 个 accounts 行只变化 `extra.upstream_billing_probe` 和 `updated_at`，核心业务值与生产无关且未外发。
- 旧版副本 `0.1.135`（SHA256=`B2B0862AEF79B63EB6DBB9B44B092780CCB12B21C0DCB16AAF3D5B1914720936`）连接迁移后的同一克隆成功启动，`/health=ok`、`needs_setup=false`；旧版可识别的 Battle Pass/渠道监控均为 false，旧版缺失新公开字段为预期兼容差异。停止旧版后仍为 371 条迁移、13 条 SubNexus 迁移、invalid index=0，28 表/45 索引完整，核心计数和金额不变。
- 候选、旧版副本、隔离 Memurai 均已按端口和可执行路径精确停止，四条临时防火墙规则已删除；PostgreSQL 18 原始恢复库、候选克隆、下载备份和运行日志保留用于审计。下一门禁是使用 Redis 8.8.0 实际加载 production RDB；Docker 候选镜像仍待完成。

## 2026-09-03（Asia/Shanghai）— 线上 Redis 8 RDB 隔离恢复门禁通过

- 维护者在本地 PowerShell 通过一次性 SSH 命令运行服务器脚本；PowerShell 仅是 SSH 客户端，实际脚本由服务器上的 root `/bin/bash --noprofile --norc` 执行。首次 SSH 密码输入失败后重新认证成功，没有产生服务变更。
- 已安装脚本 `/srv/subnexus-migration/tools/subnexus-redis-restore-check.sh` 的 SHA256 与本地批准值一致：`21491AF439DB9EB4A89A71390776DCF13FBCDFFFC2AD596D6A4A659EEB2FC3A6`。
- 脚本使用生产 Redis `sub2api-redis` 的不可变镜像 ID，在 `--network none`、无端口发布、只读 RDB bind mount、受限资源的临时 Redis 8.8.0 容器中加载 RDB；输出 `PING=PONG`、`DBSIZE=18520`、`RDB_LOADED=18520`、`RDB_EXPIRED=20506`、`RDB_TOTAL=39026`。
- 临时容器在脚本结束时按完整容器 ID 自动清理；脚本输出 `REDIS_8_RESTORE_VALIDATED_PRODUCTION_UNCHANGED`。证据文件：`/srv/subnexus-migration/redis-restore/20260903T131430Z-3144728-6273df02-cf80-4b5e-9901-1b5f08c7b008/evidence.txt`；证据 SHA256：`f7f524028f2bacadff58efa669e5a4f91fc7c4b3dd38e09a4f17b1324b0e319a`。
- 本次未停止、重启或修改生产应用、PostgreSQL、Redis、Nginx；未执行生产迁移、DDL/DML、部署、切流或功能开关修改。Redis 持久化恢复子门禁现为通过，下一门禁为 Docker 候选镜像运行验证。

## 2026-09-03（Asia/Shanghai）— Docker 候选门禁主机准备

- 经维护者明确授权，后续发布前置工作改由 AI 通过 SSH 执行；执行边界止于生产切换前。任何生产迁移、生产开关修改、旧应用停止/重启、候选连接生产 PostgreSQL/Redis、Nginx 切流仍须停止并由维护者手动执行。
- 服务器资源只读快照：8 vCPU、约 22 GiB 内存（约 17 GiB available）、4 GiB swap、根分区约 193 GiB/剩余 38 GiB。禁止在服务器恢复约 67 GB 的生产数据库，禁止 Docker prune 或删除旧镜像/回滚资产。
- Docker 29.1.3 原先缺少 Buildx。`apt-get -s` 确认只新增一个包后，安装 Ubuntu 官方 `docker-buildx 0.30.1-0ubuntu1`，占用约 71 MB；安装过程明确延后 `docker.service` 重启并报告无需重启容器。未升级其他包，未执行系统或 Docker 重启。
- 安装后验证 Buildx 0.30.1、BuildKit 0.26.2 可用。按 registry digest 依次预拉 BuildKit、Node 24 Alpine 和 Go 1.27 Alpine 构建镜像；PostgreSQL 18、Redis 8 和 Alpine 3.21 使用服务器已存在的不可变镜像。预拉只增加镜像层，不修改生产容器或生产 tag。
- BuildKit digest=`28a898719c18a33f4e8000685287fa36fd0dd9560c6440227d3a732d79bb41d8`；Node digest=`e67514e5d0f6c46656005e1b693b2ec9d52e80b641307de684d4a015ba7a4eaf`；Go digest=`4c9fe60190a2a3350ddc51de80d0224b8a6698d12bdfc999fee45ea9d6c46dbc`；Alpine image ID=`48b0309ca019d89d40f670aa1bc06e426dc0931948452e8491e3d65087abc07d`；PostgreSQL image ID=`9a8afca54e7861fd90fab5fdf4c42477a6b1cb7d293595148e674e0a3181de15`；Redis image ID=`9d317178eceac8454a2284a9e6df2466b93c745529947f0cd42a0fa9609d7005`。
- 准备前后生产容器 ID 保持为应用 `be459424b327ad056ea9bdc02187d6a458fe09082369b354158d6e7f7758beee`、Redis `5c7adf42247c67ba90b09248056071a57c2a4e7e0465f922d4ed799ef092533e`、PostgreSQL `8178576aed6f7b1cb94201832e5797907ea4d7698dbfe7b6f862cbc5a3b4f5bf`；应用持续 `running/healthy` 且 restart count 为 0。
- 当前尚未构建候选镜像、创建候选运行容器、执行生产 SQL/迁移、修改生产设置、停止服务或切流。Docker 候选脚本仍须完成本地双重审查、测试、提交和固定 SHA 后方可运行。

## 2026-09-04（Asia/Shanghai）— Docker 候选门禁脚本独立复核修正

- 在 fork 的 `feature/subnexus-migration` 工作树独立复核 `subnexus-docker-candidate-check.sh`；未读取或修改 `F:\Sub2Api\SubNexus`，未连接服务器、生产 Docker、PostgreSQL 或 Redis。
- 修复带 tag 的 repository digest 规范化：比较 Docker 实际记录的 `repository@sha256:digest`，保留 registry 端口并使用逐行精确匹配；修复预加载候选 tag 路径在复用前未重新校验归档的问题。
- 修复候选归档路径校验顺序：先使用 `realpath -m -s` 的不跟随符号链接形式与物理路径比较，再执行归档完整性检查，拒绝文件或父目录的符号链接路径；同时把客服 namespaced/legacy 两个开关都强制写入并验证为 false。
- 新增 repository digest、预加载 tag 和归档夹具断言；`bash -n`、isolated-image-build 静态测试、Docker candidate-check 静态/夹具测试均通过。未完成真实 Docker 构建，不能将本次结果视为候选运行门禁通过。

## 2026-09-04（Asia/Shanghai）— 发布门禁安全复核二次加固

- 对候选 Docker 门禁进行独立安全复核后，清理逻辑改为只有明确的“对象不存在”诊断和精确列表查询才允许继续；Docker 超时、信号、CLI/daemon 未知错误一律标记清理失败并保留现场，禁止发布通过证据。
- 候选证据目录和镜像归档在任何创建、读取或写入前，均以物理路径检查与生产应用/PostgreSQL/Redis 的所有挂载源不重叠；同时拒绝受控路径及其父路径中的符号链接，避免把证据写入生产数据目录或从生产挂载读取归档。
- 候选 gate 要求 Docker Unix socket 本身不是符号链接；候选与本地构建脚本对 `git submodule status --recursive` 的读取错误均改为硬失败，不再将检查错误当作“无子模块”。
- 更新 `subnexus-docker-candidate-check.test.sh` 与 `subnexus-isolated-image-build.test.sh` 的静态断言；四个发布脚本全部 `bash -n` 通过，Docker candidate-check、isolated-image-build、readonly-preflight、Redis restore-check 夹具测试均通过。Docker Desktop 本地 daemon 仍不可用，因此尚未生成候选镜像归档或运行 Docker 候选门禁。
- 本轮仍只修改 `F:\MySub2\sub2api` 的迁移分支；未修改 `F:\Sub2Api\SubNexus`、fork `main`、生产服务器、生产 PostgreSQL/Redis/Nginx，也未执行部署、迁移、切流或开关变更。

## 2026-09-04（Asia/Shanghai）— Registry 端口校验回归修复

- 独立审查发现候选 gate 与本地隔离构建的不可变镜像正则此前拒绝合法的私有 Registry 端口引用（例如 `registry.example:5000/postgres:18@sha256:<64hex>`），与后续 `RepoDigests` 解析逻辑不一致。
- 两个脚本现将可选的 Registry 主机/1–5 位数字端口与仓库路径分开校验；仍拒绝可变 tag、非数字/空/超长端口、短或大写 digest。两个测试脚本增加带端口（带/不带 tag）及非法端口夹具。
- 主线程复跑 `bash -n`（4 个发布脚本）、`subnexus-docker-candidate-check.test.sh`、`subnexus-isolated-image-build.test.sh` 均退出码 0；未运行真实 Docker build/gate（本地 Docker daemon 仍不可用），因此 Release Gate 仍未通过。
- 本轮只修改 `F:\MySub2\sub2api` 迁移分支和记忆文档；未修改旧项目、`main`、服务器、生产数据库/Redis/Nginx，未部署、重启、切流或修改开关。

## 2026-09-04（Asia/Shanghai）— 清理错误诊断匹配收紧

- 提交边界复核发现清理辅助函数仍会把任意包含 `not found` 的 Docker CLI 错误误判为“对象不存在”，可能在权限/daemon 故障时继续执行清理。候选 gate 与隔离构建脚本现只接受明确的 `no such object/container/network/volume`（或 image 的 `no such image`）诊断，并要求精确列表为空且 daemon 健康；其他错误硬失败。
- 两套脚本测试新增误导性 `dependency not found` 夹具，并断言缺失 image 必须执行精确列表查询、其他错误不得查询；四个发布脚本语法及四套夹具在主线程复跑通过。
- 这是已推送提交后的独立后续修复，将以新提交发布，不重写远端历史；真实 Docker build/gate 仍未运行，本地 daemon 故障状态不变。
- 本轮仍只修改 `F:\MySub2\sub2api` 迁移分支和记忆文档；未修改旧项目、`main`、服务器、生产数据库/Redis/Nginx，未部署、重启、切流或修改开关。

## 2026-09-04（Asia/Shanghai）— 本地隔离 Docker 构建门禁修复与运行时准备

- 本轮仅访问 `F:\MySub2\sub2api` 及新建的本地 WSL 隔离发行版；未读取或修改 `F:\Sub2Api\SubNexus`，未连接服务器、生产 Docker、PostgreSQL、Redis 或 Nginx。
- 真实隔离构建首次在源树安全检查阶段停止：批准提交包含合法模板 `deploy/.env.example`，构建脚本原先只允许仓库根目录 `.env.example`/`.env.sample`，错误拒绝嵌套模板；当时未创建 BuildKit builder、镜像、卷、网络或候选容器。
- 修复 `tools/production-deploy/subnexus-isolated-image-build.sh`：新增 `is_safe_env_example_path`，仅允许任意目录下 basename 为 `.env.example` 或 `.env.sample` 的模板，其他 `.env*`、证书和密钥文件继续硬失败；源树与解包 context 两处统一使用该规则。测试脚本新增嵌套模板正例和非模板反例。
- 本地验证：两个脚本 `bash -n`、`subnexus-isolated-image-build.test.sh`、`git diff --check` 通过；修复尚未进入批准构建提交，待提交后重新计算提交/脚本 SHA。
- 为避免共享 Docker Desktop 的既有 20 个容器、24 个卷和 4 个自定义网络，创建全新 WSL2 发行版 `SubNexusBuild20260904`（Ubuntu base，运行时目录位于 `F:\MySub2\\.subnexus-isolated-runtime`），安装仅限该发行版的 Docker 29.1.3、Buildx 0.30.1，并启动独立 `/var/lib/subnexus-docker` 与 Unix socket；daemon ID 为 `eb03fe50-80ad-4194-a28e-7ad5a786368c`，初始仅有默认网络，无容器/卷/自定义网络。Docker Desktop daemon 未停止、清理或修改。
- 已在该 daemon 预加载并核对五个固定 digest 基础镜像：BuildKit `28a898719c18a33f4e8000685287fa36fd0dd9560c6440227d3a732d79bb41d8`、Node `e67514e5d0f6c46656005e1b693b2ec9d52e80b641307de684d4a015ba7a4eaf`、Go `4c9fe60190a2a3350ddc51de80d0224b8a6698d12bdfc999fee45ea9d6c46dbc`、Alpine `48b0309ca019d89d40f670aa1bc06e426dc0931948452e8491e3d65087abc07d`、PostgreSQL `9a8afca54e7861fd90fab5fdf4c42477a6b1cb7d293595148e674e0a3181de15`；短仓库名用于匹配 Docker canonical `RepoDigests`。
- 当前状态：本地候选镜像尚未成功构建，Release Gate 仍未通过；未执行生产 SQL/迁移、部署、重启、切流或开关修改。下一步为提交该门禁修复、更新 Linux detached clone，并在同一独立 daemon 重新运行构建；成功后再校验归档、元数据和镜像 ID。
- 回滚点：修复前分支提交 `79ce84dfd976002134500f27bbf8d0df75a19ca4`；本地 WSL 发行版和其 `/work` 数据均为新建隔离资产，可单独停止/注销，不触碰 Docker Desktop 或生产资产。

## 2026-09-04（Asia/Shanghai）— 隔离构建 context 权限归一化

- 第二次真实隔离构建在 `validate_context_tree` 阶段停止，错误为 `fixed Docker context entry is writable by group/other`。独立 WSL daemon 仍保持空闲：没有创建候选镜像、容器、卷或自定义网络，生产 Docker Desktop 和服务器未访问。
- 根因已复现并确认：`git archive --format=tar` 在该 WSL 环境中将 Git 的 `100644/100755` 条目记录为 tar 的 `0664/0775`（GNU tar 输出为 `-rw-rw-r--`/`drwxrwxr-x`），而固定 context 门禁必须拒绝 group/other 写权限。源 Git 树本身没有异常模式。
- 在提交 `3a095f8a534cb93d176d9147114ddbb1e0cec446` 中修复 `extract_fixed_context`：解包文件和目录统一使用 `(member.mode & 0o777) & ~0o022`，保留 owner 权限和执行位，明确丢弃 group/other 写权限；没有放宽 `validate_context_tree`。
- 测试脚本新增权限归一化断言，并用合成 tar 实际执行内嵌解包器，确认目录 `0777→0755`、普通文件 `0664→0644`、可执行文件 `0775→0755`；`bash -n`、`subnexus-isolated-image-build.test.sh` 和 `git diff --check` 均通过。修复脚本 SHA256=`a01527dbb91de2b7dbd0c4ce7a3b17ee7a6b6ceff4eaf44c4026edcfbdce2ec5`。
- 下一步：把该提交同步到 `/work/sub2api` 的 detached clone，确认独立 daemon 仍无残留对象后重跑真实镜像构建；成功前不得运行候选 gate，更不得连接生产数据库或执行线上切换。
- 回滚点：`79ce84dfd976002134500f27bbf8d0df75a19ca4`（仅回退本地构建脚本修复，不涉及生产资产）。

## 2026-09-04（Asia/Shanghai）— 隔离候选构建二次门禁加固

- 使用提交 `0823fba399e892128ff4474f5f31394593976a29` 重跑真实隔离构建，context 权限检查已通过，但在源码扫描阶段因 `backend/migrations/117_add_payment_order_provider_snapshot.sql` 被泛化的 `*snapshot.sql` 规则误判为数据库快照而停止；当次未创建 builder、候选镜像、卷或自定义网络。
- 新增 `is_database_dump_path`：只放行 `backend/migrations/` 下符合迁移命名的普通 `snapshot.sql`，生产 dump/backup/export、压缩快照及其他路径的 snapshot 继续硬失败；对应正反夹具已覆盖。
- 删除根 Dockerfile 的可变 `# syntax=docker/dockerfile:1.7` 外部 frontend 指令，并收紧 Dockerfile 合同解析：大小写/缩进后的每个 `FROM` 都必须精确使用四个批准镜像参数且各一次，只允许批准的 platform 参数，同时拒绝所有 syntax directive 变体，避免新建 BuildKit builder 隐式拉取可变 frontend。
- 本地构建锁改为对已经校验的 artifact 目录 inode 使用 `flock`，不再创建或跟随 `.build.lock` 路径；成功路径会核对“基线镜像 + 候选镜像”，失败/中断清理后会核对完整 Docker 对象基线，能够发现固定镜像被意外删除或额外对象残留；构建成功后在归档前安全删除 staging 中的固定源码 context，最终证据不再保留未单独校验的源码副本。
- 一次独立 daemon 镜像元数据试验误删了该 daemon 内的 Node digest 镜像；Docker events 已确认删除只发生在 `SubNexusBuild20260904` 的专用 daemon，随后按同一固定 digest 重新加载。未触碰 Docker Desktop 或服务器；真实构建重跑前必须再次核对五个基础镜像及 0 容器/0 卷/仅默认网络。
- Git Bash 与 WSL/Linux 均通过四个发布脚本 `bash -n`、isolated-image-build、Docker candidate-check、readonly-preflight、Redis restore-check 测试；动态夹具覆盖 context 实际删除以及失败路径的基础镜像缺失/额外镜像检测；Windows 无可用 Python 时 tar 权限动态夹具明确跳过，WSL 中实际执行并通过。`git diff --check` 通过。
- 当前仍未得到成功的候选镜像/归档，Docker Release Gate 继续为未通过；下一步是提交本次修复、把 WSL detached clone 固定到新 SHA，仅在专用 daemon 重跑真实构建。不得连接生产数据库、启动线上候选、执行生产迁移、停止旧应用或切流。

## 2026-09-04（Asia/Shanghai）— Buildx PID 限制参数兼容修复

- 将加固提交 `75a3a33e6d2a4dc63434879bd66c78337dd904fc` 同步到 `/work/sub2api` 的 clean detached clone 后，在专用 daemon 发起真实构建；构建在创建 BuildKit builder 时停止，Buildx 0.30.1 明确返回 `invalid driver option pids-limit for docker-container driver`，尚未进入 Dockerfile 编译或依赖下载。
- 失败路径已自动删除临时 builder/network/staging；人工复核仍为五个固定基础镜像、0 容器、0 卷、仅 `bridge/host/none`，没有候选镜像或归档，Docker Desktop 和服务器均未访问。
- 保留 PID 上限，不再把 `pids-limit` 作为 Buildx driver option；脚本在 bootstrap 后用唯一完整 builder 容器 ID 执行 `docker update --pids-limit 512`，随后继续通过 inspect 强制验证 `HostConfig.PidsLimit=512`，验证完成前不会运行项目 build。若 update/验证失败，清理仅在其他完整身份合同仍匹配且 PID 为默认 0 或目标 512 时删除该预构建容器；已验证/已构建路径仍严格要求 512。
- 测试明确拒绝重新加入无效 driver option，并要求容器级 PID 更新与失败诊断；Git Bash/WSL 的 `bash -n` 和 isolated-image-build 测试通过。修复提交后须再次同步 detached clone 并从完整 daemon 基线重跑。

## 2026-09-04（Asia/Shanghai）— Buildx 0.30.1 运行时合同校准

- 使用提交 `2e6c800cad711cb3bb49d7324bbdbf7ffe9581a2` 重试时，BuildKit builder 已 bootstrap，但脚本因把对齐输出误写成固定文本 `Driver: docker-container` 而在项目 build 前停止；失败对象基线门禁正确发现 builder 容器、state 卷和专用网络未被旧校验逻辑清理，未产生候选镜像或归档。
- 只读 inspect 将残留对象完整绑定到本次随机 builder 名称/token、固定 BuildKit image ID和带 gate/token 标签的网络；随后仅对该精确 builder 执行 `buildx rm --force`，确认容器和唯一 state 卷消失，再按完整网络 ID删除已为空的专用网络。没有使用 prune 或删除镜像，daemon 已恢复五个固定镜像、0 容器/卷、仅默认网络。
- 按真实 Buildx 0.30.1 / Docker 29.1.3 元数据校准合同：Driver/Status 使用锚定空白正则；bootstrap 初始 `MemorySwap=-1`、`PidsLimit=<nil>` 只允许用于项目 build 前失败清理；正常路径在运行项目 Dockerfile 前用 `docker update` 统一设置内存 4 GiB、memory-swap 4 GiB、CPU quota/period、PID 512 和 restart=no，并在 strict inspect 中要求精确值。
- 新版 Buildx builder 容器的 Config.Labels 为空，因此不再依赖旧 labels；身份校验改为同时要求随机精确名称、固定 BuildKit image ID、专用网络 ID/名称、唯一 `buildx_buildkit_<builder>0_state` 卷挂载到 `/var/lib/buildkit`、私有 IPC/cgroup namespace、init、无端口/设备/docker.sock 及完整资源合同。Git Bash/WSL 的语法与 isolated-image-build 测试通过；真实构建仍待新提交后重跑。

## 2026-09-04（Asia/Shanghai）— 构建批准锚点、归档兼容与锁路径收尾

- 本轮仅修改 `F:\MySub2\sub2api` 的 `feature/subnexus-migration` 工作树；未读取或修改 `F:\Sub2Api\SubNexus`，未修改 fork `main`，未连接线上服务器、生产 PostgreSQL/Redis/Nginx，也未执行部署、迁移、重启、切流或开关变更。
- `subnexus-isolated-image-build.sh` 现在要求外部独立批准值 `SUBNEXUS_APPROVED_BUILD_SCRIPT_SHA256`。在任何 Docker RPC 前，脚本重新检查执行文件的非符号链接/owner/mode，重新计算当前文件 SHA，并将其与批准提交中的 Git blob SHA 及外部值逐项比较；运行期间的自检也继续要求三者一致。`metadata.env` 记录 `BUILD_SCRIPT_SHA256`、`APPROVED_BUILD_SCRIPT_SHA256`、`APPROVED_BUILD_SCRIPT_BLOB_SHA256`。
- Dockerfile 合同解析已改为处理大小写/缩进/续行后的真实指令：四个基础 `ARG` 必须在首个 `FROM` 前各出现一次并带安全默认值；注释伪造、重复/缺失声明、可变或带额外字段的 `FROM`、syntax/escape 指令均拒绝。合法 `GOPROXY=https://goproxy.cn,direct` 等非镜像参数保留兼容。
- 隔离构建归档校验不再把 Docker image ID 与 archive Config digest 混淆：读取并校验 Config 内容 SHA-256，核对 `rootfs.diff_ids` 与构建前 `.RootFS.Layers`；候选 gate 同时校验 Docker 29 OCI Config、旧式 `<digest>.json`、gzip layer 和旧式未压缩 `layer.tar` 的解压后 SHA-256。候选 gate 的并发锁改为锁定已校验证据目录 inode，并在打开 FD 后复核设备/inode/owner/mode，移除可被替换的锁文件路径。
- Windows Git Bash：四个发布脚本 `bash -n`、isolated-image-build、Docker candidate-check、readonly-preflight、Redis restore-check 夹具均通过；Windows 的动态 Python 夹具因系统只有 AppInstaller redirector 按设计跳过。WSL/Linux：同四套 `bash -n`/夹具均通过，动态归档、context 权限与 legacy/gzip layer 夹具实际执行并通过；`git diff --check` 通过（仅报告工作树 CRLF 转换提示）。
- 当前状态仍为“本地代码门禁通过，真实 Docker Release Gate 未通过/待重跑”：尚未生成新的候选镜像归档，不能据此替换线上版本。BuildKit 使用专用 daemon 仍有 privileged 容器和构建网络出站的残余风险；生产候选 gate 不读取无独立签名的 `metadata.env`/`SHA256SUMS`，发布清单必须在外部保存并复核构建脚本 SHA。下一步是提交本轮改动、同步 `/work/sub2api` detached clone，复核专用 daemon 五镜像/零对象基线后再重跑真实隔离构建。
- 本轮回滚点：提交前本地 HEAD `058657b94682d1aa0088e148d4fa1e41ddadd273`；此前已验证的代码回滚点 `79ce84dfd976002134500f27bbf8d0df75a19ca4`。本轮提交 SHA、脚本 SHA及真实构建证据须在后续追加记录中补齐。

## 2026-09-04（Asia/Shanghai）— 真实隔离 Docker 构建成功

- 本轮仅在 `F:\MySub2\sub2api` 的迁移分支和专用 WSL daemon `SubNexusBuild20260904` 执行；没有读取或修改 `F:\Sub2Api\SubNexus`，没有修改 fork `main`，没有连接线上服务器、生产 Docker、PostgreSQL、Redis 或 Nginx。
- 使用已批准提交 `fa8ac7fa0c45e83a68010467f26d3def2ecd73fd` 完成真实隔离构建。BuildKit builder、资源/安全合同、固定源码 context、前端构建、Go 后端构建、镜像加载及 Docker archive 完整性校验均通过。
- 生成候选镜像 ID：`sha256:9a6d5812a54bd5b74b8977c15503d7e8f67a472cf768d954e2cb01b833321a17`；候选归档 SHA256：`838633aacb3be5ae6e05a51c3931b8b5f7c0e09ce0b502dbdffa9aa4b6e697c4`。
- 构建后专用 daemon 保留五个固定基础镜像和一个候选镜像（共 6 个 image ID）；候选镜像同时保留 release tag 与 gate tag，两个 tag 指向同一候选 ID。候选 archive validator、静态夹具和运行时构建收尾校验均通过。
- 构建元数据中的 `BUILD_SCRIPT_SHA256`、`APPROVED_BUILD_SCRIPT_SHA256`、`APPROVED_BUILD_SCRIPT_BLOB_SHA256` 三项均为 `fa41a1e9909d8ec9d39f370eb84d9b97bb7f0f5c7e8258aca7387e987b255cd4`，并已与批准提交中的脚本 blob 和实际执行文件一致。
- 构建结束后的专用 daemon 收尾核对通过：五个固定基础镜像仍在，容器数为 0、卷数为 0、无自定义网络，仅保留 `bridge`、`host`、`none`；未使用 prune，未删除 Docker Desktop 或生产资产。
- 本次只证明本地隔离构建和归档可交接，不等于候选 runtime gate、生产备份克隆上的最终候选验收或线上切换通过。线上仍未访问、未迁移、未部署、未重启、未切流，所有迁移功能继续保持默认关闭；下一步是使用该归档执行本地候选 runtime gate，之后再由维护者决定发布窗口。

## 2026-09-04（Asia/Shanghai）— 候选运行时门禁 Bash 参数污染修复

- 上一轮针对提交 `93d48e257e51d939b6e56623b478ab15e469d005` 的真实候选 gate 未通过：`SELECT 1;` 的返回值混入了 `then/status/else/fi` 及后续函数源码。复核确认 PostgreSQL、Redis 和 HTTP 三处把跨行 pipeline 直接嵌在 `if output="$(...)"` 命令替换中，运行时 Docker exec 参数/输入边界不可靠；这不是线上数据库或 Redis 数据异常。
- 在 `tools/production-deploy/subnexus-docker-candidate-check.sh` 中将四类 Docker exec pipeline 提取为 `candidate_pg_exec`、`candidate_redis_exec`、`candidate_http_post`、`candidate_http_get`，调用方只保留简单的 helper 命令替换；未改变密码 stdin、SQL positional argument、HTTP payload/token 或生产身份检查语义。
- `subnexus-docker-candidate-check.test.sh` 新增四个 helper 的独立 `bash -n` 检查，以及 PostgreSQL/Redis/HTTP 的参数和 stdin 夹具，确认 `SELECT 1;`、`PING`、payload/token 不会被调用方控制文本污染。Git Bash 与 WSL/Linux 的 candidate-check 夹具、四个发布脚本语法检查和 `git diff --check` 均通过；夹具中预期的负面错误输出不代表测试失败。
- 本轮只修改 fork 的迁移分支；未修改 `F:\Sub2Api\SubNexus`、`main`、线上服务器、生产 Docker/PostgreSQL/Redis/Nginx，未执行生产迁移、部署、重启、切流或开关变更。修复提交后必须重新同步 WSL detached clone，从五个固定基础镜像/零运行对象基线重建并重跑 candidate gate；在 gate 通过前 `cutover_allowed=false`。
- 回滚点：本次提交前 `93d48e257e51d939b6e56623b478ab15e469d005`（仅代码回退，不得复用其失败候选归档上线）。

## 2026-09-04（Asia/Shanghai）— 隔离构建锁目录身份复核

- 独立安全复核指出，构建脚本虽然直接对 artifact 目录 FD 加 `flock`，但在路径校验与 FD 打开之间仍缺少身份复核。现新增 `directory_fingerprint`，在 `exec 9<"$artifact_root"` 前后比较设备/inode/类型/owner/mode，路径被替换时立即失败；不创建或跟随可替换的 `.build.lock` 文件。
- `subnexus-isolated-image-build.test.sh` 增加静态合同与 Linux/WSL FD 指纹夹具；Git Bash/WSL 语法和隔离构建夹具均通过。该修复会改变构建脚本批准 SHA，必须与本轮 gate 修复一起重新提交、同步、构建和验收。
- 仅修改 fork 迁移分支和本地测试；未修改旧项目、`main`、线上服务器、生产数据库/Redis/Nginx，未执行线上部署或切换。上一轮 `9ea1d877` 归档不作为最终发布候选。
- 回滚点：本次提交前 `9ea1d877ae980e47e318d435dce0976b23de1c62`；回滚仅作用于本地代码，不删除或恢复任何线上数据。

## 2026-09-04（Asia/Shanghai）— 最新隔离构建与 Docker 29 清理诊断修复

- 使用提交 `c91226ef2b1a3e9caffd19ddfd8f319e95f772b0` 在专用 WSL daemon `eb03fe50-80ad-4194-a28e-7ad5a786368c` 成功完成真实隔离构建；tree=`b37d04bd3b38ac7a955cb3d0dc3269285f4aba19`，镜像 ID=`sha256:4f453fe2b7a8f11383bd07ddda4d3a22137f329c4d435f8b131cc4d7403f77ab`，归档 SHA256=`82ff92df12a6c1dfb8694492501353e6bf55b633267a5ef7aafb7c3b0bb1de64`，构建脚本 SHA256=`cbec521753cc5fa18bf96a4fd1dd58b32ff026fd76009189e8015a2d201b8aa3`。
- 在同一专用 daemon 中用三个无挂载、无端口的 synthetic `prod-*` 容器执行 candidate gate。候选 PostgreSQL、Redis、应用健康检查、290 条空库迁移、关闭态公开设置、管理员登录、数据卷持久化、应用重启和稳定窗口均通过；证据 token=`20260904T101111Z-5258308c-4591-4beb-8c84-ad381333293e`。
- gate 最终按失败发布证据：Docker 29 对已删除网络返回 `[]` 加 `Error response from daemon: network <id> not found`，旧清理 helper 仅认可 `no such network`，因此所有本次候选对象实际删除后仍记录 `cleanup_failed=true`。该证据必须保留为失败历史，不能用于发布；`cutover_allowed=false`、`manual_review_required=true`。
- `object_absent` 现只额外接受两种完整形式：精确错误行，或单独 `[]` 行后跟同一精确错误行；错误中的网络引用必须与请求引用一致，随后仍必须通过精确 `docker network ls` 空集和 daemon 健康复核。新增 Docker 29 正例、错误引用反例及 stdout/stderr 组合夹具；Windows Git Bash 与 WSL/Linux 的语法和完整 candidate-check 测试、`git diff --check` 均通过。
- 本轮没有成功登录线上服务器；未读取或修改生产 Docker、PostgreSQL、Redis、Nginx、配置或流量。下一步为提交本修复、同步 detached clone、以新提交重新隔离构建并重跑 candidate gate；最终切换仍只允许维护者手动执行。
- 回滚点：本次修复前 `c91226ef2b1a3e9caffd19ddfd8f319e95f772b0`。旧项目 `F:\Sub2Api\SubNexus` 和 fork `main` 未修改。

## 2026-09-04（Asia/Shanghai）— `02774d028` 候选构建与 runtime gate 通过

- 本轮仅在 `F:\MySub2\sub2api` 的 `feature/subnexus-migration` 和专用 WSL daemon `SubNexusBuild20260904` 执行；未读取或修改 `F:\Sub2Api\SubNexus`，未修改 fork `main`，未连接线上服务器、生产 Docker、PostgreSQL、Redis 或 Nginx。
- 使用提交 `02774d028d076e934a59f04fd1ee98598ac693a1`（tree=`023e96b6c629f7d33e8ac2d43b7bd93f960a36f5`）和外部批准构建脚本 SHA256=`cbec521753cc5fa18bf96a4fd1dd58b32ff026fd76009189e8015a2d201b8aa3` 完成真实隔离构建。候选镜像 ID 为 `sha256:b49b764cfc2ca58d9f054c01ef9e17211b89b8280be30534ff83b4b90490a979`；归档位于 `/root/subnexus-migration/candidate-artifacts/02774d028d076e934a59f04fd1ee98598ac693a1/candidate-image.tar`，大小 `45179904`，SHA256=`45306dfe47e6093d0be67d2446f7d83f7e82ef3407ef2b0f1ed8816489877786`。
- 在同一专用 daemon 中完成候选 runtime gate。证据目录为 `/root/subnexus-migration/docker-candidate/20260904T104343Z-2854f544-d1ee-44be-9b58-ff465ee160ac`，证据 SHA256=`7d5dc1141906ee2dcac51dadc17da788e8cf0d4c172d1c71a781a823edc120fb`。结果为 `result=passed`、`cleanup_failed=false`；应用、PostgreSQL、Redis 健康检查，290 条迁移，管理员登录与 `auth/me`，重启前后迁移数一致，数据卷 sentinel 持久化和所有公开 rollout 开关关闭态均通过。
- gate 只清理本次候选对象；专用 daemon 当前仍保留用于测试的 synthetic `prod-app`/`prod-postgres`/`prod-redis` 容器及网络（无卷），不得对该 daemon 使用全局 prune。旧失败证据 `20260904T101111Z-...` 保留作历史审计，不能用于发布。
- 当前门禁已从“Docker 待重跑”更新为“候选构建与 runtime gate 通过”；仍必须由维护者人工复核发布清单并批准维护窗口，`cutover_allowed=false`、`manual_review_required=true`。生产迁移、候选上传/启动、停旧容器、切流和功能开关开启均未执行；下一步仅可在维护者批准后准备线上候选资源，并在最终切换前停止。
- 本轮文档回滚点为提交前 `02774d028d076e934a59f04fd1ee98598ac693a1`；代码和专用 daemon 资产均不因文档回滚删除或恢复。

## 2026-09-04（Asia/Shanghai）— 生产 prepare 的 Docker 29 运行时合同修复

- 上一轮线上 `prepare` 两次均在备份、数据库写入、迁移、镜像加载和容器操作之前停止；原因是实时应用的 Docker 29 `HostConfig.ConsoleSize=[49,202]` 与 json-file 日志轮转 `Config={max-file:5,max-size:20m}` 未被旧合同复现。应用容器 `be459424b327...`、PostgreSQL、Redis 和公网流量均未改变。
- 提交 `617a2fdc1189a452f251f90a6b8e4d554ac2bd05` 仅修改生产切换脚本及其测试：`ConsoleSize` 在 `Tty=false` 时严格校验非负二维值并归一化；日志配置只允许 `json-file` 与成对的 `max-file`/`max-size`，捕获为 `log-config.json`、写入 manifest SHA，并在候选 `docker create` 时显式传递 `--log-driver`/`--log-opt`；未知驱动、字段、缺项、零值和非法值 fail-closed。
- 脚本 SHA256=`98998993c01f7e071b491c8572895914d100ed15ea686df6b50bd1680239991c`，测试脚本 SHA256=`bced4ee8b7707186f1c0fa681e6320a5ad18e27856ece887bbe008e66c0a1203`。Windows Git Bash 五套夹具均通过；隔离 WSL Linux 五套夹具均通过，并实际验证 Docker 29 日志参数复现。候选应用代码、镜像 ID、归档 SHA 和 gate 证据仍固定为 `02774d028d076e934a59f04fd1ee98598ac693a1` / `sha256:b49b764cfc2ca58d9f054c01ef9e17211b89b8280be30534ff83b4b90490a979` / `45306dfe47e6093d0be67d2446f7d83f7e82ef3407ef2b0f1ed8816489877786`，无需重建应用镜像。
- 通过 SSH 只读复核（2026-09-04 19:54 Asia/Shanghai）确认线上应用仍 healthy、`127.0.0.1:18083 -> 8080`，PostgreSQL/Redis 仍运行；服务器已保留两个失败 `prepare` 目录，仅含早期 source-tree 审计文件，没有候选容器或生产备份被误用。新脚本尚未安装，尚未重新执行 prepare。
- 下一步：推送提交，使用唯一文件名安装 root-only 新脚本；先核对候选归档/gate SHA、源码 detached HEAD 和 Docker/磁盘基线，再运行不停止线上应用的 `prepare`。只有 `READY`、五项备份、设置快照、依赖/容器身份和 manifest 完整核验通过后，才生成最终 `switch`/`rollback` 单行命令并停在人工切换前。所有迁移功能继续默认关闭。

## 2026-09-04（Asia/Shanghai）— 重复 Docker 环境键的 fail-closed 兼容合同（未提交）

- 本轮仅修改 `F:\MySub2\sub2api` 的 `feature/subnexus-migration` 工作树中的生产切换脚本、其夹具测试和切换手册；未读取或修改 `F:\Sub2Api\SubNexus`，未修改 fork `main`，未连接线上服务器或生产 PostgreSQL/Redis/Nginx，也未执行线上备份、迁移、部署、重启、切流或开关变更。
- 线上只读复核此前发现 `SERVER_TRUSTED_PROXIES` 在 Docker `Config.Env` 中重复。脚本仍默认严格拒绝重复键；兼容路径必须同时提供显式确认 token、精确键 allowlist 和每个重复键最后值的独立 SHA-256。实际环境值不写入 evidence/日志；`container.env` 仅作为 root-only 0600 的规范化运行元数据保存。
- 新增 `environment-duplicates.tsv` 脱敏证据及 manifest/env/evidence SHA 合同，覆盖 prepare、live replay、candidate replay。候选被 Docker 规范化为唯一键时，只要最终规范化环境哈希一致即接受；live 的重复顺序、来源数组或选中值发生漂移则 fail-closed。旧的无新字段 prepared run 继续按历史严格无重复合同回滚兼容。
- 修复一个重要状态边界：replay 现在把观测结果写入独立 observed 状态，不会覆盖准备合同；因此候选 canonicalization 后，后续 preserved-container 合同和自动回滚仍按原始 last-wins 合同复核。
- 测试：Windows Git Bash 的脚本语法、静态/夹具测试通过（系统 `python3` 为 AppInstaller redirector，动态夹具按设计跳过）；`SubNexusBuild20260904` WSL/Linux 中 `bash -n` 与 production cutover 全部夹具通过，动态覆盖无批准/错误 hash/额外 duplicate 拒绝、last-wins 归一化、无明文 evidence、live 序列漂移、candidate canonicalization 和 runtime hash 等价性；`git diff --check` 通过。
- 本轮尚未提交或推送；需由维护者复核差异后重新计算批准脚本 SHA，再决定是否同步 detached clone/重跑线上只读 `prepare`。在此之前 `cutover_allowed=false`，所有迁移功能继续默认关闭。

## 2026-09-04（Asia/Shanghai）— 线上 prepare 挂载输出兼容修复（未提交）

- 新脚本首次在服务器执行 `prepare` 时只读采集到真实 Docker 29 挂载输出的双换行（Go 模板换行加 CLI 终止换行），在 `capture_mounts` 误报 `live mount metadata is malformed`/传播字段错误；失败发生在备份、镜像加载、容器操作和数据库写入之前。失败运行目录 `/srv/subnexus-migration/cutover/20260904140820-3616319` 保留作审计，线上应用 ID 与健康状态未改变。
- 修复仅允许解析器跳过“所有字段为空”的尾部记录，任何部分字段仍 fail-closed；同时将挂载传播校验改为跨 GNU Bash/MSYS 可移植的显式空值或枚举判断。新增夹具覆盖双换行、空传播字段和部分记录拒绝。
- Windows Git Bash 与 WSL（从 `/mnt/f/MySub2/sub2api` 运行）脚本语法、production cutover 全部夹具和 `git diff --check` 通过。修复尚未提交/推送/安装；服务器仍未执行备份、迁移、切换或开关修改。下一步提交并重新计算脚本 SHA，原子安装新工具后重跑 `prepare`。

## 2026-09-04（Asia/Shanghai）— 应用数据 owner 合同收口与线上无停机基线复核

- 在提交 `66cb41b26b5226c6a25e0d5cb93864adaffb2d8f` 中完成应用数据目录 owner 合同：默认新 run 使用严格 `root-only`（UID/GID `0:0`）；线上已审核的非 root 例外只允许 `1000:1000`，必须同时提供确认 token 和 UID/GID，且只可用于 `/app/data` 叶目录。父目录继续要求 root-owned、不可对 group/other 写入；owner、mode、设备号和 inode 在 prepare、switch、rollback 重复校验。
- 修正旧 manifest 兼容 resolver 的空 GID 判断。历史 manifest 没有 owner 字段时仅保留旧脚本“UID=0、历史 GID 不收紧”的兼容边界，并通过真实 resolver/identity 夹具验证；现代 manifest 仍要求完整 owner 字段和安全 mode。恢复通用 `assert_root_owned_path_chain` 原语义，避免无关调用者回归。切换脚本 SHA256=`9b7717eab53f898c659958a19b10c088bace3f7695657cb7e6085e5099c5f847`，测试脚本 SHA256=`65371b1141de90e16b4968f9e1b9f7ad709b5041b7334a74a35a233becba0afb`。
- Windows Git Bash 两脚本 `bash -n`/production cutover 夹具通过；隔离 WSL Linux 两脚本 `bash -n`/production cutover 全部夹具通过；isolated-image-build、docker-candidate-check、readonly-preflight、redis-restore-check 的 Windows/WSL 静态夹具也通过；`git diff --check` 通过。WSL 输出的本机编码警告不影响退出码 0。
- 通过 SSH 只读复核（2026-09-04 23:00 左右）确认线上应用 `subnexus-cutover` 仍 `running/healthy`、端口 `127.0.0.1:18083 -> 8080`，PostgreSQL `sub2api-postgres` 与 Redis `sub2api-redis` 仍运行且 restart=0；未停止、重命名、重启、切流、执行 SQL/DML/迁移或修改开关。实时数据目录 `/srv/subnexus-migration/runtime/subnexus-data` 为 UID/GID `1000:1000`、mode `0755`，父链 root-owned/不可 group-other 写入。
- 线上候选归档 `/srv/subnexus-migration/candidate-artifacts/02774d028d076e934a59f04fd1ee98598ac693a1/candidate-image.tar` 的 SHA256=`45306dfe47e6093d0be67d2446f7d83f7e82ef3407ef2b0f1ed8816489877786`，gate evidence `/srv/subnexus-migration/docker-candidate/20260904T110814Z-be48efa2-3133-4c27-bc9f-a7cbf1d221c9/evidence.txt` 的 SHA256=`1871ed998b92157e30c90daf3c0957570390a67df2fddc273164fe173712de61`，候选 image ID=`sha256:b49b764cfc2ca58d9f054c01ef9e17211b89b8280be30534ff83b4b90490a979`；只读核对均一致，gate 仍标记 `cutover_authorized=false`/`manual_review=required`。
- 线上 `/srv/subnexus-repo` 当前为 ubuntu-owned、分支 `production/ovh-baseline-20260807`、HEAD `681589e6638269069d314dc2c9f6444e6d67fc85`，不是候选 `02774d028...`，因此禁止直接将其作为 release source。下一步在 `/srv/subnexus-migration` 下创建独立 root-owned detached release source，完成后再安装本次唯一新命名脚本并运行无停机 `prepare`；旧失败 run 目录全部保留。
- 当前状态：本地代码已提交并推送迁移分支；Docker 候选 gate 通过；线上 prepare 尚未成功完成，切换、生产迁移、切流和功能开启仍禁止。回滚点为提交前 `843f10fd4`（脚本/测试代码回滚不删除线上资产）。

## 2026-09-04（Asia/Shanghai）— 文档指针收敛与 release source 只读复核

- 文档状态收敛提交为 `67f6f92aea6e988e83a45eef534f38334379a08b`，已推送到 `origin/feature/subnexus-migration`；应用镜像、归档和 Docker runtime gate 仍固定在功能候选提交 `02774d028d076e934a59f04fd1ee98598ac693a1`，不得把文档 tip 当作镜像构建 SHA。
- 通过 SSH 只读复核确认服务器已有独立 release source `/srv/subnexus-migration/release-repos/sub2api-02774d028d076e934a59f04fd1ee98598ac693a1`；该路径不覆盖 `/srv/subnexus-repo`，后续只在其通过 root-owned、detached、clean、无 submodule 和 tree 校验后用于 `prepare`。
- 当前服务器候选归档/evidence/image 仍分别为 `45306dfe47e6093d0be67d2446f7d83f7e82ef3407ef2b0f1ed8816489877786`、`1871ed998b92157e30c90daf3c0957570390a67df2fddc273164fe173712de61`、`sha256:b49b764cfc2ca58d9f054c01ef9e17211b89b8280be30534ff83b4b90490a979`；旧应用、PG、Redis、Nginx 和流量仍未改变。
- 线上只读磁盘剩余约 36.5 GB；准备阶段仍需在不停止服务的前提下生成本次 run 的备份、settings snapshot、manifest 和 `READY`。在 `READY`、人工验收和维护者手动切换前，`switch`、生产迁移、切流及功能开启均禁止。

## 2026-09-04（Asia/Shanghai）— 长数据库备份 Docker 超时边界修复（提交 `c113f7c3d41154cf80f812e5eaae6a539d778da8`）

- 根因：线上 PostgreSQL 只读 dump 约 4.88 GB，切换脚本 `SUBNEXUS_DOCKER_TIMEOUT_SECONDS` 的硬上限 `600` 秒不足；此前两次 `prepare` 均在备份阶段安全失败，未停止、重命名或重启线上应用，也未生成 `READY`。
- 仅修改 `tools/production-deploy/subnexus-production-cutover.sh` 与对应测试：保持默认 `120` 秒和最小 `10` 秒不变，将最大值提高到有界的 `1800` 秒，并同步错误提示；未改变备份内容、数据库迁移、切换顺序、容器身份或功能开关。
- 新增动态边界夹具，实际接受 `10/600/601/1800`，拒绝 `9/1801/invalid`；Git Bash 下该测试及全部 5 个 `tools/production-deploy/*test.sh` 均退出码 0，脚本 `bash -n`、`git diff --check` 通过。依赖 Linux/root/真实 Python 的夹具按平台条件跳过并明确输出。
- 本地提交后切换脚本文件 SHA256=`56d6935418bb3229892653d4d775cefb341e20a44c5adef612b55340741584f4`，Git blob=`deb8782a3817f03a48ded468a58642b5843d1705`；提交尚待推送并以唯一新文件名安装到服务器，旧脚本必须保留。
- 本轮只访问并修改 `F:\MySub2\sub2api` 迁移分支及本地测试；未读取/修改旧项目、fork `main`，未连接或写入线上服务器、PostgreSQL、Redis、Docker、Nginx，未执行迁移、部署、切流或开关变更。
- 当前状态：线上仍无 `READY`，不得执行 `switch`；下一步为推送提交、安装新脚本、仅以 `SUBNEXUS_DOCKER_TIMEOUT_SECONDS=1800` 重新执行一次无停机 `prepare`，随后只读核验备份和身份合同，核验完成即停在人工切换前。

## 2026-09-05（Asia/Shanghai）— 在线应用数据归档活动日志安全修复

- 上一次线上 `prepare` 在应用数据归档阶段安全失败，原因是实时写入的 `/srv/subnexus-migration/runtime/subnexus-data/logs/sub2api.log` 被 GNU tar 报告为 `file changed as we read it`。失败发生在镜像加载、候选容器创建、数据库写入和线上容器操作之前；失败 run、已有备份和日志均保留，线上应用仍为 `be459424b327...`/healthy。
- 提交 `9c0033aa1b152f75e4e15f06f70a15d39e46f4d6` 仅修改 fork 迁移分支的生产切换脚本及夹具：在线应用数据归档固定只排除 `./logs/*.log`，保留 `./logs/*.gz` 和其他数据；不使用 `--ignore-failed-read` 或抑制 `file changed` 警告，因此非日志文件的变化仍 fail-closed。若日志轮转正在原地更新 `.log.gz`，prepare 会安全失败并稍后重试。脚本同时清除 `TAR_OPTIONS`/`GZIP` 环境注入，避免归档参数被外部环境改变。
- 每个新 prepare run 生成 root-only `application-data-exclusions.txt`、独立 SHA256 sidecar 和 manifest 字段 `application_data_archive_policy_sha256`；switch 必须校验完整策略证据。后续提交 `b27ae6652` 将该策略证据排除在 rollback 前置条件之外，因为 rollback 不读取任何应用归档或数据库备份；rollback 仍严格校验 Docker daemon、旧容器、依赖、应用数据目录身份和设置快照。
- Windows Git Bash 和 `SubNexusBuild20260904` WSL/Linux 的 `bash -n`、`subnexus-production-cutover.test.sh` 及五套 `tools/production-deploy/*test.sh` 均通过；Linux 动态夹具实际让活动日志持续写入并验证稳定数据、压缩历史日志和策略哈希。系统 WSL 代理提示不影响退出码。
- 两个代码提交均已推送至 `origin/feature/subnexus-migration`；`main` 与 `F:\Sub2Api\SubNexus` 未修改。此记录写入时尚未安装最终脚本、尚未重新执行线上 `prepare`，没有停止/重命名/重启生产容器，没有执行 DDL/DML、Nginx 切流或开启任何二开功能。下一步只在服务器保留旧脚本的前提下安装唯一新文件名，并用脱离 SSH 的后台方式重跑无停机 `prepare`；只要未得到完整 `READY` 和人工核验，禁止 `switch`。

## 2026-09-05（Asia/Shanghai）— prepare 参数合同修复与发布前门禁

- 发布前复核发现 `prepare_run` 原先错误接受 9–11 个位置参数，而 usage 合同实际为 8–10 个（`SOURCE_ROOT` 至 `LIVE_APP` 必填，公网健康 URL 与 evidence root 可选）。提交 `b8625ff774db40b0eb6f96850b6d91b88d7d57da` 新增独立 `prepare_argument_count_is_valid`，严格接受 8/9/10、拒绝 7/11，并加入回归夹具；没有改变备份、容器、数据库或切换顺序。
- 修复后的切换脚本 SHA256=`d88aaabe7d6d474978d52a80a64745d2e283d1b9474fb3aac53cde3b426d9ff9`，测试脚本 SHA256=`b3f1ff9c34da77e0132edac09bcbfad9fbfd79525878c62046c315902c3e6341`。Windows Git Bash 五套测试、语法检查和 `git diff --check` 通过；Linux-only 动态夹具仅在 Windows 按设计跳过。
- 线上已安装但未执行的历史副本 `ecf1dae36dc7c140299ba5ef2d3dab66c08d53c0ebd9208f2ddab1f7125a2e0a.sh` 保留在 `/srv/subnexus-migration/tools/`；本轮修复脚本必须以全新文件名安装并独立校验 SHA。至此仍未停止、重命名或重启生产容器，未写生产 PostgreSQL/Redis、未切流、未改开关。
- 只读磁盘复核显示根分区可用约 `27917008896` 字节，`cutover` 历史证据约 `8459786012` 字节，仍有一个失败 PG partial 约 `3386727897` 字节；失败 run 和 partial 均保留作审计，prepare 必须通过自身预算门禁后才继续。当前线上 `subnexus-cutover` 仍 healthy，`READY` 尚不存在。

## 2026-09-05（Asia/Shanghai）— 预加载候选镜像日志证据与符号链接安全收尾

- 提交 `ba1c6450d` 修复候选镜像已预加载时缺少 `image-load.log` 的证据不完整问题：每个 prepare run 都会创建 root-owned、模式 `0600` 的空日志；已有文件或符号链接会在 Docker 查询前被拒绝。这样复用已核验的候选镜像不会绕过日志文件完整性门禁。
- 提交 `af82a6877` 将日志证据安装改为 GNU `mv -T`，并保留安装前的目标路径复核；目标在竞态中变成指向目录的符号链接时不会被跟随写入外部目录。测试同时覆盖预加载 tag、空日志、权限和预置符号链接拒绝。
- Windows Git Bash 五套 `tools/production-deploy/*test.sh`、脚本语法检查和 `git diff --check` 通过；在隔离 WSL 的独立临时副本中五套测试和 Linux 动态夹具也通过。测试期间未连接或修改线上服务器、生产 Docker、PostgreSQL、Redis、Nginx 或流量。
- 当时迁移分支 HEAD 为文档提交 `3c18050d1`（部署脚本修复提交为 `af82a687746aaf35dd5e6d00dff93a7571355a31`），工作树干净并已与 `origin/feature/subnexus-migration` 同步。应用候选镜像仍固定于 `02774d028d076e934a59f04fd1ee98598ac693a1`；本次仅改部署脚本/测试，不需要重建应用镜像。切换门禁仍为 `cutover_allowed=false`，线上 `prepare` 已完成，`switch`/`rollback` 未执行。

## 2026-09-05（Asia/Shanghai）— 线上无停机 prepare 成功与人工切换交接

- 本轮先将 `af82a6877` 的生产切换脚本以唯一文件名 `/srv/subnexus-migration/tools/subnexus-production-cutover-af82a6877-ba0f4c1e.sh` 安装到服务器；服务器副本 root-owned、模式 `0700`、SHA256=`ba0f4c1eeddcad82978028ae94f2e97b9a94cd54604c45a3bb847392dfb71064`，`bash -n` 通过。旧脚本副本未覆盖、未删除。
- 第一次重试使用了带换行的重复环境值哈希，脚本在只读环境元数据阶段以 `the selected last environment value does not match its approved hash` fail-closed；该 run `/srv/subnexus-migration/cutover/20260904175414-3700860` 和日志均保留，未生成备份、未触碰容器。随后按脚本实际 `value.encode('utf-8')` 算法使用独立批准哈希 `633e5f7d659d6b1a617824263685c063a145afda7463344f1ab502c27ee99dd9` 重试。
- 成功 run 为 `/srv/subnexus-migration/cutover/20260904175519-3701605`，后台进程已正常退出并生成 `READY` 内容 `prepared`。候选提交=`02774d028d076e934a59f04fd1ee98598ac693a1`，source tree=`023e96b6c629f7d33e8ac2d43b7bd93f960a36f5`，镜像 ID=`sha256:b49b764cfc2ca58d9f054c01ef9e17211b89b8280be30534ff83b4b90490a979`，候选归档 SHA=`45306dfe47e6093d0be67d2446f7d83f7e82ef3407ef2b0f1ed8816489877786`，runtime gate evidence SHA=`1871ed998b92157e30c90daf3c0957570390a67df2fddc273164fe173712de61`。
- 备份与证据尺寸：PostgreSQL custom dump `5071323565` bytes、catalog list `107429` bytes、Redis RDB `6785583` bytes、RDB check `643` bytes、应用数据归档 `79658747` bytes；活动日志策略固定为只排除 `./logs/*.log`，保留 `./logs/*.gz` 和其他应用数据。所有 required sidecar/manifest、settings before/closed 快照、environment duplicate evidence、owner/mode/inode 和依赖身份复核通过；`SERVER_TRUSTED_PROXIES` 以已批准的 last-wins 合同记录，应用数据 owner 为已审核的 `1000:1000`/mode `0755`。
- prepare 全程未停止、重命名、重启或创建线上应用容器，未执行生产 PostgreSQL/Redis 迁移、DDL/DML、Nginx 切流或功能开关修改。完成后线上 `subnexus-cutover` 仍为 ID `be459424b327...`、healthy、restart `0`，PostgreSQL/Redis 仍为原 ID/running，未发现候选容器；失败 run、历史备份和旧版本资产继续保留。
- 当前交接点是 `READY` 但 `cutover_allowed=false`：仅维护者在确认无结算/迁移任务、入口配置和备份证据后，手动执行同一脚本的 `switch`；异常时优先按同一 run 执行应用 `rollback`，不自动恢复数据库。所有迁移功能继续关闭，逐项验收后再开启。

## 2026-09-05（Asia/Shanghai）— 最终交接文档提交

- 提交 `c9d03df0b5e552416b4e466860077fa31e0583e7` 与后续文档提交 `983b8a3cb9fd370ceadc01cc33e2b7ebf7e16f07` 仅更新四份迁移记忆/运行手册，已推送并与 `origin/feature/subnexus-migration` 同步；没有修改应用代码、部署脚本、候选镜像或 `main`。部署脚本 SHA256 仍为 `ba0f4c1eeddcad82978028ae94f2e97b9a94cd54604c45a3bb847392dfb71064`。
- 当前本地工作树干净，流程保持在 `READY=prepared` / `cutover_allowed=false` 的人工切换前节点；本轮未执行 `switch` 或 `rollback`，所有二开功能继续关闭。
- 后续手册命令显式清除 `DOCKER_HOST`、`DOCKER_CONTEXT`、`DOCKER_CONFIG`、`DOCKER_TLS_VERIFY`、`DOCKER_CERT_PATH`、`DOCKER_API_VERSION`，以满足脚本的本地默认 Docker daemon 门禁并避免服务器终端继承错误上下文；该调整仅为文档安全收口。
- 随后进行了一次在线只读复核：`READY` 仍为 `prepared`，脚本仍为 root/`0700` 且 SHA256 一致，`subnexus-cutover` 原容器仍 healthy/restart=0，`sub2api-postgres` 与 `sub2api-redis` 仍 running/restart=0，无候选容器，未执行任何写操作；`/srv` 可用空间约 19 GB。SSH 复核结束后已正常退出。

## 2026-09-05（Asia/Shanghai）— 切换参数缺陷修复、旧 run 自动回滚与磁盘门禁

- 维护者此前执行旧脚本 `ba0f4c1e...` 对 run `/srv/subnexus-migration/cutover/20260904175519-3701605` 的 `switch` 时失败：`create_candidate_container` 把 `create` 重复传给 `docker_rpc`，Docker 将其解释为镜像 `create:latest`，随后拒绝拉取。ERR trap 已恢复旧容器和 rollout gates，输出 `ROLLBACK_COMPLETED`；没有创建候选容器、没有恢复 PostgreSQL/Redis、没有修改 Nginx 或功能开关。但 stop+rename 已发生，存在短暂业务不可用窗口，不能描述为零影响。
- 修复已提交并推送：`c85b5d4419cf36a60d0429d23e003bb060e9b26e`（候选镜像参数直接传给 Docker）和 `8483409d7745584b9148c5ccd2749e703e2b0822`（本地 Docker argv 动态夹具）。生产切换脚本 SHA256=`8076d267ebebce97603acd6cc92ea99d3d0d7a25c3a26a9cb3b37ce57dedf0af`；测试脚本 SHA256=`13d14232456a7a8e7d8d03171713ae586e1ab1caae4445e4835a1c268ac5a21a`。Windows Git Bash 与隔离 WSL Linux 的语法、静态和动态夹具均通过。旧 run 同时含 `READY`/`ROLLED_BACK`，状态为 `rolled_back`，严禁再次 switch，必须新建 prepare run。
- 新脚本已作为唯一新文件安装为 `/srv/subnexus-migration/tools/subnexus-production-cutover-8076d267-20260905.sh`，root-owned、`0700`、`bash -n` 和 SHA 校验通过；旧脚本副本均保留。安装和验证没有停止/重启线上容器，也没有执行 SQL/迁移。
- 使用新脚本启动无停机 `prepare` 时，新 run `/srv/subnexus-migration/cutover/20260905000853-3813358` 在 PostgreSQL 备份前被磁盘预算门禁安全拒绝：可用约 `19552845824` 字节，门禁要求 `23712679936` 字节（包含 PostgreSQL 上限、动态依赖预算、元数据和 `8 GiB` 最低保留）。没有生成数据库备份、没有创建候选容器、没有停止/重命名/重启应用；旧容器仍 healthy，PostgreSQL/Redis 仍 running。
- 服务器 `/srv/subnexus-migration/postgres` 是生产 PostgreSQL 的活动 bind mount，不能清理；不能降低 `8 GiB` 保留、复用旧时间点 dump 或使用默认 Docker prune。下一步必须先取得独立持久化空间（扩容/新挂载）或经维护者批准把明确无引用的构建中间层/失败临时资产转移到离线存储并保留校验，再重新 prepare。当前没有可供 switch 的 prepared run，所有功能继续关闭。

## 2026-09-05（Asia/Shanghai）— 新 prepare 成功，停在人工 switch 前

- 在不停止、重命名或重启线上容器的前提下，使用脚本 `/srv/subnexus-migration/tools/subnexus-production-cutover-8076d267-20260905.sh` 完成新的在线 `prepare`。本次 run 为 `/srv/subnexus-migration/cutover/20260905002953-3824168`，`READY` 内容为 `prepared`，manifest `state=prepared`；准备进程已正常退出，脚本明确记录 `No production container was stopped, renamed, restarted, or switched.`
- 新 run 的应用候选固定为提交 `02774d028d076e934a59f04fd1ee98598ac693a1`、tree=`023e96b6c629f7d33e8ac2d43b7bd93f960a36f5`，镜像 ID=`sha256:b49b764cfc2ca58d9f054c01ef9e17211b89b8280be30534ff83b4b90490a979`；候选归档 SHA256=`45306dfe47e6093d0be67d2446f7d83f7e82ef3407ef2b0f1ed8816489877786`；Docker runtime gate evidence=`/srv/subnexus-migration/docker-candidate/20260904T110814Z-be48efa2-3133-4c27-bc9f-a7cbf1d221c9/evidence.txt`，SHA256=`1871ed998b92157e30c90daf3c0957570390a67df2fddc273164fe173712de61`，`result=passed`、`cleanup_failed=false`、`cutover_authorized=false`、`manual_review_required=true`。
- 本次备份均在独立 root-only run 中重新生成并逐项核验：PostgreSQL custom dump `5084032665` bytes（SHA256=`efff091cfde4049430eca8f7c4229a739770041383406e223cb4370454ee0c19`），catalog `107429` bytes（SHA256=`054f5e44861b0cbd579061cb6fc8aab2a480e55b61189c6dfe30dcc5d3abea0e`），Redis RDB `6648027` bytes（SHA256=`048bbedf3bcc3fc41ac020309d05e0b4803ff429c5c40615df9fabb231c02b57`），RDB 检查报告 SHA256=`9d6ef74caf4ff5583344f4b531724dde6e82c78ec6096768e4e8d4ff15d540c5`，应用数据归档 `80191647` bytes（SHA256=`9600c4bb47c58dac48d98c062c829dbf176657adcbf7cf660d161dada6833bfe`）。在线归档策略仍为只排除 `./logs/*.log`，策略 SHA256=`ee1908db818e2434a9e5a47ec84a02ac10eafd11bc79767e1313f3f6e659826d`。
- 关闭态快照 SHA256=`8de4ae1711229355c234a1fde1cf308e8ad0f869d0d16443659e33014813f2b4`；所有迁移功能在关闭快照中为 `false`（Channel Monitor 模式仅保留安全的 `v1` 值，邀请活动结构配置的 `enabled` 子项也为 `false`）。生产数据库/Redis 未执行迁移、DDL、DML 或恢复。
- 实时身份复核通过：旧应用 `subnexus-cutover` ID=`be459424b327...` 仍 `healthy`、restart=0；`sub2api-postgres` ID=`8178576aed6f...`、`sub2api-redis` ID=`5c7adf42247c...` 均 running、restart=0；当前 run 标签筛选无候选容器。脚本仍为 root:root/`0700`，SHA256=`8076d267ebebce97603acd6cc92ea99d3d0d7a25c3a26a9cb3b37ce57dedf0af`。
- 为满足不降低安全余量的磁盘门禁，先前只读确认后定向删除了两个无标签且无容器引用的 Docker 构建中间镜像（完整 ID 记录在 `/srv/subnexus-migration/cleanup-20260905-dangling-images.txt`）；未使用 `prune`，候选镜像、线上旧镜像、旧容器、数据库和 Redis 均保留。prepare 完成后根分区可用空间约 `18 GB`，旧失败 run 的证据/partial 资产仍保留，后续清理必须单独审计。
- 当前交接点是新的 `READY=prepared`，但 `cutover_authorized=false`：只有维护者在维护窗口确认无结算/迁移任务、入口配置和备份证据后，才可手动执行新的 `switch`。异常时按同一 run 执行 `rollback`；回滚默认只恢复应用容器和关闭态设置，不自动恢复 PostgreSQL/Redis。所有二开功能继续关闭。

## 2026-09-05（Asia/Shanghai）— 第二次 switch 自动回滚、生产迁移事实核验与 Docker 运行时合同修复

- 维护者对 run `/srv/subnexus-migration/cutover/20260905002953-3824168` 执行人工 `switch` 后，候选成功创建、连接两个既有网络、启动并通过健康窗口，但旧脚本报 `candidate runtime contract differs from the prepared live container`，随后输出 `ROLLBACK_COMPLETED`。manifest 已为 `state=rolled_back`，`READY=prepared` 与 `ROLLED_BACK=rolled_back` 同时作为历史证据保留；该 run 与更早的 `20260904175519-3701605` 都不可再次 switch/rollback。
- 自动回滚后只读审计确认：旧应用精确恢复为原 ID `be459424b327...`、名称 `subnexus-cutover`、原镜像，running/healthy/restart=0；PostgreSQL `8178576aed6f...` 与 Redis `5c7adf42247c...` 身份未变、running/restart=0 且启动时间仍是 2026-08-06，证明依赖未重启。失败候选、临时旧容器名称和该 run 标签容器均无残留。候选约运行 34.5 秒；候选日志中 `panic/fatal/migration/migrate/schema` 关键词均为 0，但不能仅以日志推断数据库未迁移。
- PostgreSQL 只读会话随后确认 `9001_subnexus_activity_center.sql` 至 `9013_subnexus_leaderboard_rewards.sql` 已于 `2026-09-05 01:17:03 UTC` 应用；逐条 checksum 与 fork 候选 SQL 按 runner 的 `SHA256(TrimSpace(SQL))` 算法全部一致。自动回滚没有恢复数据库，旧应用已在迁移后同库上恢复健康，符合“应用可回滚、同库数据不丢”的兼容设计。没有手工执行 DDL/DML、重复迁移或数据库/Redis 恢复。
- 运行时合同根因通过生产 Docker 的 stopped probe 精确复现：旧容器 `HostConfig.OomKillDisable=null`，Docker 29 用等价参数新建容器后为 `false`；两者都表示 OOM killer 保持启用，且 `validate_runtime_contract_supported` 原本就允许二者，但旧哈希未归一化。原 live 合同 SHA 为 `adea07f0062ae5617f63da2d83a063a0ffc9911382670c71f01f958312c19b56`，probe 原值不同；仅把 probe 的该字段视为 `null` 后哈希精确等于 live，证明它是本次唯一进入规范化合同的差异。
- 诊断 probe 使用唯一名称创建为 stopped/never-started，只复现环境、端口声明、挂载声明和两个网络连接；没有运行进程、没有绑定监听端口、没有读写应用数据。取证后按精确 ID 仅删除该 probe，并再次确认名称无残留；线上应用始终 running/healthy/restart=0。该探针不是候选发布或清理授权，禁止据此删除其他对象。
- 修复提交 `0d083f6b7cf53c440968f9a63e8bc4002017b53f` 仅修改 fork 迁移分支的切换脚本和测试：把 `OomKillDisable=null` 规范为 `false`，保留 `true` 的不同值并继续 fail-closed；显式 `0.0.0.0` HostIP 不再缩写为空值；在 `docker start`/entrypoint/自动迁移前先校验候选合同，启动健康后仍二次复核。脚本 SHA256=`5291c6041305fa77902a113e2ef181615920bd37cbbd80e46e9fe095d0c21132`，测试 SHA256=`16fe581ecdf400ce6eb4f609b9a8cde1ee243666b9ab02f2199f3fc23e114880`。
- Windows Git Bash 的生产切换、候选 gate、隔离构建脚本语法/静态/故障夹具全部退出 0；依赖真实 Python/Linux 的夹具按设计跳过。`SubNexusBuild20260904` WSL/Linux 中相同三套测试全部退出 0，实际运行了 `OomKillDisable null/false/true`、安全 validator 与 argv 动态夹具；`git diff --check` 通过。独立审查无阻断项。
- 非本次阻断的剩余兼容风险已登记：validator 对部分 Docker 默认字段接受多种等价值，而合同哈希仍原样保留；当前生产 stopped probe 已证明本次除 OOM 字段外无其他规范化差异，且 live `NetworkMode` 使用网络名，因此不在紧急修复中批量放宽。以后若部署形态改为 NetworkID 等形式，应以真实成对 inspect 夹具逐字段修复。

## 2026-09-05（Asia/Shanghai）— 修复脚本安装、全新 prepare 与 stopped probe 最终验收

- 本轮只在 fork 迁移分支和已授权的生产准备流程中操作；`F:\Sub2Api\SubNexus`、fork `main`、应用代码和线上 Nginx 均未修改。将当前生产切换脚本以唯一文件名安装为 `/srv/subnexus-migration/tools/subnexus-production-cutover-5291c604-20260905.sh`，服务器副本 `root:root`/`0700`，`bash -n` 通过，SHA256=`5291c6041305fa77902a113e2ef181615920bd37cbbd80e46e9fe095d0c21132`；旧脚本和历史 run 均保留。
- 针对已经迁移过的生产同库，以新脚本完成全新无停机 `prepare`：run=`/srv/subnexus-migration/cutover/20260905020043-3862867`，后台进程正常退出，`READY=prepared`、`manifest state=prepared`，无 `SWITCHED`/`ROLLED_BACK`。脚本日志明确记录没有停止、重命名、重启或切换生产容器；未执行生产 SQL、DDL/DML、数据库/Redis 恢复、Nginx 修改或功能开关写入。
- 发布固定值：目标提交=`02774d028d076e934a59f04fd1ee98598ac693a1`，source tree=`023e96b6c629f7d33e8ac2d43b7bd93f960a36f5`，候选镜像=`sha256:b49b764cfc2ca58d9f054c01ef9e17211b89b8280be30534ff83b4b90490a979`；候选归档 SHA256=`45306dfe47e6093d0be67d2446f7d83f7e82ef3407ef2b0f1ed8816489877786`；runtime gate evidence SHA256=`1871ed998b92157e30c90daf3c0957570390a67df2fddc273164fe173712de61`。
- 新鲜备份全部由脚本生成并经 sidecar/manifest、owner/mode/inode 和结构校验：PostgreSQL custom dump/list=`5069001531/118440` bytes，SHA256 分别为 `9372851b9a6514d3467922244f94b7e9232c90e1cc703baa85e366f327bc65f9` / `ec1e353c901e73e2589e41aff57c7c2dca4d4306f8827e721fc84bcd84045cc3`；Redis RDB/check=`7346835/644` bytes，SHA256 分别为 `14d03f8d5edaf410fdc76c4cce2ebe38b5e77dae6fc396773c399a976033f008` / `626c1d76b15227fa5b197e9de89bbb97c091ca288a4fb6e6fbf5625d3bd0e98c`；应用归档=`80667716` bytes，SHA256=`8933cfd6b78f96fcc8f18e84bad3845b54f6950a54a5f1e3f80f3fbc21556f2e`。在线归档策略仍为只排除 `./logs/*.log`，策略 SHA256=`ee1908db818e2434a9e5a47ec84a02ac10eafd11bc79767e1313f3f6e659826d`；不忽略非日志文件变化。
- 运行合同与设置证据：settings-before SHA256=`039f45a96f202523e0376ea4f2122aaa485b22ad623011abd0724693b9e78bc3`；关闭态快照 SHA256=`8de4ae1711229355c234a1fde1cf308e8ad0f869d0d16443659e33014813f2b4`，其中迁移功能布尔开关均为 `false`、`channel_monitor_mode=v1`、邀请活动子配置关闭；runtime contract SHA256=`7dc88dd8f76be1a69c6d4f322deb1b1e0eda8be94be61d37cac850091578453d`。应用数据目录身份 SHA256=`58c9f5d4df9a0b174f9c7ff08ca0c084a2d50f4dd96f455cd4722ca161e9cef0`，owner=`1000:1000`、叶目录 mode=`0755`、设备/inode 在 prepare 前后未变；重复环境键仅按已批准的 `SERVER_TRUSTED_PROXIES` last-wins 合同处理，批准值哈希为 `633e5f7d659d6b1a617824263685c063a145afda7463344f1ab502c27ee99dd9`，未记录环境明文。
- 为验证 Docker 29 的运行时合同，在生产 Docker 创建唯一 stopped probe ID=`eb4269d6a147fbec589a528b0f79a470c188656c7fe70e4bc26fdcbdb13c1a0e`；状态为 `created`、`Running=false`、`RestartCount=0`，从未执行 `start`/`exec`，合同 SHA=`7dc88dd8f76be1a69c6d4f322deb1b1e0eda8be94be61d37cac850091578453d` 与新 run 精确相等。取证后仅按完整 ID 普通 `docker rm` 删除，并确认 ID、名称和 run 标签均无残留；无 named volume、无端口监听或应用数据写入。
- 最终只读复核：线上应用完整 ID 前缀 `be459424b327`，`running/healthy/restart=0`，端口仍为 `127.0.0.1:18083->8080/tcp`；PostgreSQL=`8178576aed6f`、Redis=`5c7adf42247c` 均为原容器、`running/restart=0`；无候选容器；`/srv` 可用约 `19383078912` bytes。历史 run `/srv/subnexus-migration/cutover/20260904175519-3701605` 和 `/srv/subnexus-migration/cutover/20260905002953-3824168` 继续标记 `rolled_back`，禁止复用。
- 当前交接状态：`cutover_allowed=false`。所有迁移功能继续默认关闭。只有维护者在维护窗口确认无结算/迁移任务、核对入口配置和备份后，才可执行 `SUBNEXUS_CUTOVER_RUNBOOK.md` 中针对该 run 的单行 `switch`；异常时使用同一 run 的单行应用 `rollback`，默认不恢复 PostgreSQL/Redis。此次未把未捕获最终 stderr 的 quiet-gate 预演当作通过依据，实际 `switch` 会在停容器前重新执行完整门禁。
- （历史状态，已被本记录后续的 `20260905020043-3862867` 新鲜 prepare 取代）当时没有有效 prepared run，禁止执行历史命令；该阶段要求安装修复脚本、重新 prepare 并完成 stopped probe。旧脚本、历史 run、备份、旧容器和旧镜像继续保留，禁止 prune、Nginx 修改和功能开启。

## 2026-09-05（Asia/Shanghai）— 历史命令误重试诊断与交接文档校正

- 维护者回传的终端截图再次执行了两个已终态的旧 `switch` 命令：run=`20260904175519-3701605` 使用旧脚本并重复传入 `create`，Docker 因此寻找不存在的 `create:latest`；run=`20260905002953-3824168` 使用旧脚本并因 Docker 29 的 `OomKillDisable=null/false` 合同序列化差异误报。两次均输出 `ROLLBACK_COMPLETED` 并保持 `state=rolled_back`，禁止再次执行；该截图不代表最新 run 失败。
- 本轮仅在本地修正文档中关闭态快照与活动日志归档策略的 SHA256 录入笔误，并完成全局 64 位摘要长度检查、切换脚本语法/故障夹具检查；未执行线上 `switch`/`rollback`、Nginx 切流、数据库或 Redis 写入、功能开关变更。
- 当前唯一可交接 run 仍为 `/srv/subnexus-migration/cutover/20260905020043-3862867`，脚本为 `/srv/subnexus-migration/tools/subnexus-production-cutover-5291c604-20260905.sh`（SHA256=`5291c6041305fa77902a113e2ef181615920bd37cbbd80e46e9fe095d0c21132`），状态 `READY=prepared`/`cutover_allowed=false`；最终 `switch` 和同 run 应急 `rollback` 只能由维护者在确认维护窗口条件后手动执行。

## 2026-09-05（Asia/Shanghai）— Docker 29 候选网络身份误报修复（待重新 prepare）

- 维护者执行旧脚本 `/srv/subnexus-migration/tools/subnexus-production-cutover-5291c604-20260905.sh` 对 run `20260905020043-3862867` 进行 `switch` 时，候选容器已创建并连接既有网络，但在启动前被报错 `candidate network identities do not match the prepared live container`；脚本随后自动回滚并删除候选，恢复旧应用和关闭态设置。该 run 已进入 `state=rolled_back`，不得重试；这不是数据库恢复或数据丢失事件，但切换窗口可能有短暂不可用。
- 根因是 Docker 29 在候选尚未启动的阶段，`docker inspect` 的 `NetworkSettings.Networks[*].NetworkID` 与 `docker network inspect --format '{{.Id}}'` 可能采用不同表示/暂态值。旧代码直接逐字比较两者，造成同一网络对象的误报。
- 本地修复新增 `assert_candidate_network_identities`：先严格比较候选实际网络名称集合与准备记录名称集合，再逐名称重新读取当前网络对象 ID，并与准备阶段 ID 精确比较；网络被同名重建、缺失、增加或 ID 漂移时仍 fail-closed。新增 shell fixture 覆盖名称-only 候选、对象 ID 漂移和额外网络拒绝。
- 修复只涉及 `tools/production-deploy/subnexus-production-cutover.sh` 及其测试；`main`、`F:\Sub2Api\SubNexus`、线上 Docker/PostgreSQL/Redis/Nginx 和用户流量未被修改。Windows Git Bash 的切换 fixture、脚本语法检查和候选检查均通过；Linux-only 动态 fixture 需在隔离 WSL 中补跑。
- 旧 run `20260905020043-3862867`、`20260904175519-3701605`、`20260905002953-3824168` 均不可复用。网络修复提交并推送后，必须以新脚本文件名重新安装并重新执行完整无停机 `prepare`，生成全新 `READY=prepared` run；在此之前 `cutover_allowed=false`，不得提供或执行 `switch`。
- 本条及本文件此前关于 `20260905020043-3862867` “唯一可交接/READY=prepared”的旧叙述均已被本条覆盖：该 run 已是 `rolled_back`，不存在当前可交接 run。为避免误用，任何操作员只应以本条、台账当前状态和修复后新 `prepare` 生成的 manifest 为准；历史脚本、历史 run、历史候选和历史备份不得作为切换输入。

## 2026-09-05（Asia/Shanghai）— 网络身份修复提交与外部推送阻塞

- 本地 `feature/subnexus-migration` 已提交 `ca2139d1e70877fba8a41e1410e4d7d29b4ef9c0`，包含候选网络名称/对象 ID fail-closed 校验、对应故障夹具以及历史 run 文档收口；工作树干净，`main` 和 `F:\Sub2Api\SubNexus` 未修改。
- 验证：Git Bash 下生产切换、候选 gate、隔离构建和只读预检测试均退出 0；动态 Linux/Python 夹具因当前 Windows AppInstaller `python3` 不可用按设计跳过。此前专用 WSL 记录已确认相同动态夹具在 Linux 通过；本轮未访问线上服务器。
- 新生产切换脚本文件 SHA256=`bffd1987303d3f247a6df2c70cb90a8576a7530864863154f7dcd4d247892b01`，测试脚本 SHA256=`927b441bf1d95c175d793fe9c9bdcf37a63067c694d7fe92300f02ca2f494c41`。
- `git push origin feature/subnexus-migration` 在本机网络策略下无法连接 GitHub，带权限重试因审批服务暂不可用被拒绝；因此远端仍落后 1 个提交，服务器尚未安装/拉取新脚本，也没有执行新的 `prepare`。恢复外网后只需推送该分支，再按本手册重新安装并执行无停机 `prepare`。

## 2026-09-05（Asia/Shanghai）— 跨对话接续、推送恢复与人工操作边界复核

- 读取原任务 `01a05c86-a9f4-7061-bb00-3f7f65498c84` 的可用对话分页及本地台账后接续。用户原话明确要求“随后的更新操作由我来手动在服务器终端执行”，并于本轮再次核对交接边界；因此代理只允许只读 SSH，安装脚本、prepare、创建探针、switch、rollback 和清理均由维护者本人执行服务器终端单行命令。不得仅在最终 switch 前才停止。
- GitHub 网络已恢复；已执行 `git push origin feature/subnexus-migration`，远端从 `16e1bd87d` 更新至 `f21f28030`，包含网络修复 `ca2139d1e70877fba8a41e1410e4d7d29b4ef9c0`。在专用 WSL `SubNexusBuild20260904` 对当前脚本执行 `bash -n` 和完整 `subnexus-production-cutover.test.sh`，退出 0，实际 Linux 动态夹具通过；预期故障注入输出不代表测试失败。脚本及测试 SHA 仍为上一条记录的 `bffd1987...` / `927b441b...`。
- 2026-09-05 12:52 Asia/Shanghai 只读 SSH 复核：旧应用 ID=`be459424b327ad056ea9bdc02187d6a458fe09082369b354158d6e7f7758beee`，镜像=`sha256:b24b585a35e0eecff497a4eb7a2be480d9a2818f4b7a9780508f2f42cb5e09cd`，running/healthy/restart=0，本地 `/health` 返回 `{"status":"ok"}`；PG=`8178576aed6f7b1cb94201832e5797907ea4d7698dbfe7b6f862cbc5a3b4f5bf`、Redis=`5c7adf42247c67ba90b09248056071a57c2a4e7e0465f922d4ed799ef092533e` 均 running/restart=0，启动时间仍为 2026-08-06。三个历史 run 的 `ROLLED_BACK` 和 manifest `state=rolled_back` 全部确认，当前没有新 READY。
- 候选归档 SHA=`45306dfe47e6093d0be67d2446f7d83f7e82ef3407ef2b0f1ed8816489877786`、服务器 gate SHA=`1871ed998b92157e30c90daf3c0957570390a67df2fddc273164fe173712de61` 只读复核一致；gate 为 passed/cleanup_failed=false/cutover_authorized=false。release source 仍为 root-owned、detached、clean，HEAD=`02774d028d076e934a59f04fd1ee98598ac693a1`、tree=`023e96b6c629f7d33e8ac2d43b7bd93f960a36f5`；候选 image ID 存在。服务器通过 HTTPS 读取固定提交脚本到 SHA256 管道（未落盘），结果匹配 `bffd1987303d3f247a6df2c70cb90a8576a7530864863154f7dcd4d247892b01`。
- 新发现的 prepare 阻塞：根分区可用 `19225067520` bytes，现有脚本在该布局的最低预算为 `23712679936` bytes，至少缺约 4.5 GB；预算还需在真正 prepare 时按实时数据重算。`docker system df` 报告约 8.172 GB reclaimable 镜像并不代表这些对象可删除；没有删除/转移任何线上资产，没有使用 prune，也没有降低保留空间。维护者须先安装已核验的新脚本，再解决容量缺口，才可全新 prepare。
- 本轮纠正项目上下文、规划、台账和回滚手册中把旧 run 当作有效的残留叙述，撤回旧的可执行 switch/rollback 命令；切换手册第 10 节记录当前状态和第一条人工安装命令。所有服务器操作均为读取；未安装、备份、修改数据库/Redis、创建探针、停止/重启容器、切换、修改 Nginx 或开启功能。旧项目和 main 未修改。本文档变更回滚点为 `f21f28030`；回退文档不影响线上资产。

## 2026-09-05（Asia/Shanghai）— 新脚本安装、垃圾镜像清理与全新 prepare 完成

- 按用户更新后的授权完成服务器前置操作。新脚本 `/srv/subnexus-migration/tools/subnexus-production-cutover-bffd1987-20260905.sh` 已由 root 安装为 `root:root`/`0700`，固定提交 `ca2139d1e70877fba8a41e1410e4d7d29b4ef9c0`，SHA256=`bffd1987303d3f247a6df2c70cb90a8576a7530864863154f7dcd4d247892b01`；只执行下载、SHA 和 `bash -n`，未执行脚本安装命令本身以外的部署动作。
- 磁盘清理只针对逐个确认无标签、无容器引用的 Docker dangling 镜像，未使用 `prune`、`--force`，未删除候选镜像、线上镜像、容器、数据库、Redis、卷、备份或历史证据。首轮 18 个对象及后续显露的无引用中间层均按完整 ID 删除；剩余 11 个共享层保留。可用空间由约 `19225067520` 增至约 `40937291776` bytes。
- 第一次新 prepare run=`20260905050927-3934297` 在 PostgreSQL 备份阶段因默认 Docker 超时 120 秒安全失败，脚本记录 live app 未停止；孤立的旧 `pg_dump` PID `3935871` 经命令和启动时间精确核对后终止。没有数据库恢复、迁移、容器停止或切流。
- 使用有界 `SUBNEXUS_DOCKER_TIMEOUT_SECONDS=1800` 完成全新无停机 prepare，run=`/srv/subnexus-migration/cutover/20260905051505-3937987`，`READY=prepared`、manifest `state=prepared`，无 `SWITCHED`/`ROLLED_BACK`。PostgreSQL dump/catalog=`5083923841/118440` bytes，Redis RDB/check=`7160556/644` bytes，应用归档=`80910415` bytes；逐项 sidecar 哈希、设置快照、关闭态、网络身份、runtime contract 和 owner 合同核验通过。
- 当前固定候选值：target=`02774d028d076e934a59f04fd1ee98598ac693a1`，image=`sha256:b49b764cfc2ca58d9f054c01ef9e17211b89b8280be30534ff83b4b90490a979`，archive SHA=`45306dfe47e6093d0be67d2446f7d83f7e82ef3407ef2b0f1ed8816489877786`，gate evidence SHA=`1871ed998b92157e30c90daf3c0957570390a67df2fddc273164fe173712de61`，runtime contract SHA=`7dc88dd8f76be1a69c6d4f322deb1b1e0eda8be94be61d37cac850091578453d`，closed settings SHA=`8de4ae1711229355c234a1fde1cf308e8ad0f869d0d16443659e33014813f2b4`。
- 最终只读复核确认旧应用 ID=`be459424b327...`、PG=`8178576aed6f...`、Redis=`5c7adf42247c...` 均为原身份，running/healthy（应用）/restart=0；本次 run 标签无候选容器，磁盘可用约 `40937291776` bytes。switch 尚未执行，`cutover_allowed` 仍须由维护者在维护窗口确认；本记录停在生成人工 switch/rollback 命令前。

## 2026-09-05（Asia/Shanghai）— `Config.Cmd` 修复推送、服务器安装与重新 prepare

- 修复提交 `fbca62fbccb5a783d8d35cb9dcc4025cdb1c4a44` 已推送；生产切换脚本 SHA256=`19824a87e3e1de5659cb30664750b71c5c10d374f25bda7f52e6524fe477ee65`，测试 SHA256=`7e981ff118b795b40b38d22eb0a09667d7ac25977d9f17c5a30590dccece9763`。Git Bash/WSL full test 和 Linux 动态执行通过。
- 第四次历史 switch 因 Docker 模板尾部换行在 `Config.Cmd` 捕获 helper 中变成额外空参数，导致候选 runtime contract 与 prepared live 容器不一致并自动回滚；旧应用继续 healthy，PG/Redis 身份未变。该 run、旧脚本和旧命令均不可复用。
- 按用户最新授权完成前置服务器操作：脚本 `/srv/subnexus-migration/tools/subnexus-production-cutover-19824a87-20260905.sh` 已安装为 `root:root`/`0700`；唯一旧 `runtime-probe.nroC3xIz` 已确认无容器后精确删除。代理可执行安装、备份、`prepare`、never-started probe 和范围明确垃圾清理；仅最终 `switch`/`rollback` 必须由维护者手动执行。
- 新无停机 `prepare` 已正常完成：run `/srv/subnexus-migration/cutover/20260905055413-3958448`，日志 `prepare-19824a87-20260905T055412Z.log`，原 PID `3958448` 已退出，Docker timeout `1800`；`READY=prepared`，manifest `state=prepared`，无终态 marker。所有备份、sidecar、设置快照、环境、owner、依赖和网络合同均通过完整验证。

| 新 run 文件 | 字节数 | SHA256 |
| --- | --- | --- |
| `postgresql.dump` | `5086279866` | `97d11bbd933a2076b1aac25dcd6a5b636e77be10080f68e73fcb3be282c80ce5` |
| `postgresql.list` | `118440` | `d144e195cb9cbd1369aca05b1abd5801374801518f9465af104bd63c359b6e4d` |
| `redis.rdb` | `7143802` | `78afd911bd2f32b1a7add7b5d0752accf701c4950f5738597db803ffa68749e6` |
| `redis-check-rdb.txt` | `644` | `ba2f9650e7cc214dd3a240deb3c9341d7546cf4aebc4bd77d9b8ab7fdc3ffb8c` |
| `application-data.tar.gz` | `80910450` | `a7d9b6a92aabe5690c74baa2da1dfdec8861067cab1ef1c1721379cc1d321e95` |

- 新 run 的 runtime 合同 SHA=`7dc88dd8f76be1a69c6d4f322deb1b1e0eda8be94be61d37cac850091578453d`；settings before SHA=`039f45a96f202523e0376ea4f2122aaa485b22ad623011abd0724693b9e78bc3`，closed SHA=`8de4ae1711229355c234a1fde1cf308e8ad0f869d0d16443659e33014813f2b4`。候选 commit/image/archive/gate 均保持原固定值。
- 使用独立审查后的 `/srv/subnexus-migration/diagnostics/stopped-probe-7505c705-20260905.sh`（SHA=`7505c7050e0939d0edb1ddd8695e936b24d170024213c3027dbbab9070d18aa7`）复用当前脚本完整 `create_candidate_container` 和 `assert_candidate_runtime_contract`；只在独立临时 metadata/manifest 中替换唯一 probe 名，自身 SHA 校验仍绑定已批准原脚本。probe ID=`ac6fc54a18cddb98fd9abce54ff2be6e23fd3ac02b804580d1220eaa770beadd`，验证 `created|false|0|0001-01-01T00:00:00Z`，合同 hash 与 prepared live 一致；未启动或 exec 候选，按精确 ID 无 force 删除后再次确认不存在，临时目录已删除。
- 脱敏证据 `/srv/subnexus-migration/diagnostics/stopped-probe-20260905055413-3958448.evidence` SHA=`87399f0bc40f41dee0600e1efd421f6953f75359cc067ee943d3ce1ba80627e0`，harness 退出 0。记录 `PREPARED_RUN_VALIDATION=passed`、`PROBE_FULL_CREATE_AND_CONTRACT=passed`、`PROBE_REMOVED=true`、`PREPARED_MANIFEST_UNCHANGED=true`、`LIVE_AND_DEPENDENCIES_UNCHANGED=true`、`FINAL_SWITCH_EXECUTED=false`。
- 最后服务器复核时间为 `2026-09-05 14:11:57 Asia/Shanghai`：新 run 仍 prepared，candidate 三个身份字段为空，无 probe/candidate/临时诊断目录残留；旧应用 ID=`be459424b327...` 为 running/healthy/restart=0，`/health` 为 ok，PG=`8178576aed6f...`、Redis=`5c7adf42247c...` 原身份 running/restart=0。可用空间 `35573174272` bytes；未停止/重命名/重启旧应用，未恢复数据库、修改 Nginx 或开启功能，旧应用既有设置保留。
- 本轮前置工作全部完成。最终人工单行 `switch` 和同 run `rollback` 已记录在切换手册第 10 节，显式固定 Docker timeout `120`、脚本 SHA、新 run 和 owner 合同；语法、SHA 和 run 路径检查通过。所有历史失败 run 的可执行命令已撤回，旧项目与 fork `main` 未修改。

## 2026-09-05（Asia/Shanghai）— 维护者执行最终 switch 成功

- 维护者于 `2026-09-05 06:28:07 UTC` 执行本批次唯一批准的 `switch` 命令，run `/srv/subnexus-migration/cutover/20260905055413-3958448` 已写入 `state=switched`、`SWITCHED`，记录切换窗口 `42` 秒；未执行 rollback。
- 新应用容器 ID=`aa1eabd0ac401d83cce20f7a221b324492ef62cc0195408db8ccdf04e7829471`，running/healthy/restart=0；旧容器 ID=`be459424b327...` 已按设计保留为 `subnexus-cutover-pre-02774d028d07-20260905055413-3958448`，exited/restart=0，供同批次应用 rollback 使用。
- PostgreSQL=`8178576aed6f...`、Redis=`5c7adf42247c...` 仍为原身份 running/restart=0；应用 `/health` 返回 `{"status":"ok"}`。数据库备份未恢复，Nginx 与功能开关按流程保持原有状态。
- 本次切换结果已完成只读复核；最终 rollback 仅适用于该 run 的异常恢复，旧脚本、旧 run 和旧命令继续禁止使用。

## 2026-09-05（Asia/Shanghai）— 合并上游 v0.2.1

- 执行 `git fetch --prune upstream --tags`，确认发布标签 `v0.2.1` 的代码提交为 `578785ee7fb35030b094b69624efe25670a36f5f`；随后上游 `main` 的唯一新增提交 `ab99d56e9626e6cd731592dae8553c9758a0efa2` 将 `backend/cmd/server/VERSION` 从 `0.2.0` 同步为 `0.2.1`。
- 在 `feature/subnexus-migration` 完成两次无冲突合并：`459a0c30abf38633ae487f145eabebba6eee3e4f` 合并 `v0.2.1` 标签，`8a0c8af8534b4038e357ab8368eb027e0a489cee` 合并版本同步提交。上游新增功能和迁移 `232`、`233`、`234` 纳入；与 SubNexus 专属功能重叠的删除路径保留当前分支，现有 `9001`-`9013` 迁移、活动、邀请、充值转盘等代码和记忆文件未被删除。
- 验证结果：`git diff --check`、`pnpm typecheck`、`pnpm build`、`go test ./...` 全部通过；随后单独完整运行 Vitest `286/286` 文件、`1987/1987` 测试通过，未修改测试或业务代码。
- 当前工作树保留此前首页 Rain + Glass UI 未提交改动：`frontend/src/views/HomeView.vue`、`frontend/public/images/rain-city-1.jpg`。本轮没有重新构建候选镜像、没有执行服务器命令、数据库/Redis/Nginx 操作或生产切换；后续发布前必须以 `8a0c8af8534b4038e357ab8368eb027e0a489cee` 重新完成构建和 gate。

## 2026-09-05（Asia/Shanghai）— v0.2.1 服务器前置工作完成，停在人工 switch 前

- 候选固定为提交 `bb36764f692ca79ccc9c635fd71dcbb70b9c0449`、tree=`5673c0cb65fc35810006f889591a58bfd96e48d9`，镜像 ID=`sha256:21098ec4f4c922efa92208b640a970eb8602778c0515e8921967f9d75dc5adfd`，归档 SHA=`d1a6e297720d7af32a1ba98a7e4d3c9dc66a72a4fe7aace1ea96d346d7b1b648`。
- 服务器候选 gate run=`20260905T113230Z-1add8fbc-85d4-4d21-af6c-23fbc34af218`，evidence SHA=`ac2cd667c1a317b3d0eab0e11a9b3cbada4b4ecec49850eba08f81263ebb2bf5`；迁移计数 `294`，重启后仍为 `294`，应用/PG/Redis 均 healthy，登录和默认/公开设置检查通过，`cutover_authorized=false`。
- 使用脚本 `/srv/subnexus-migration/tools/subnexus-production-cutover-19824a87-20260905-v021.sh`（SHA=`19824a87e3e1de5659cb30664750b71c5c10d374f25bda7f52e6524fe477ee65`）完成无停机 prepare。该时间点唯一 run=`/srv/subnexus-migration/cutover/20260905114022-4163123`，manifest `state=prepared`，没有 `SWITCHED`/`ROLLED_BACK`，未停止、重命名或重启线上应用；当前状态由文末最新记录覆盖。
- 新鲜备份：PostgreSQL `5110515505` bytes SHA=`3d4cd162c630e8eed1104555beca3bb2dc210b5c26fbb8c7e21cbda8c74005d0`；Redis `9337696` bytes SHA=`f34ca3aabc93c3600e7ff785098da54189634f351ce9021b7f58966b78b457d9`；应用数据 `82391881` bytes SHA=`8d23bc7b1173949397aa190923c26f6a78ee9f729cc3cab8933c18b7db5252cd`。runtime contract SHA=`7dc88dd8f76be1a69c6d4f322deb1b1e0eda8be94be61d37cac850091578453d`，settings-before SHA=`906a5bb9c90eb3f17ac835633a6b5dd2131278d72b03224bfc061111962fc30a`，closed SHA=`6a0ed24c164bb1fa8ebb5edecb8712f77458bfca145c713adfd105257fcda8c3`。
- stopped probe 已通过：ID=`dfc59dc117f32562231eaba12892f90816dc84998fb0c302b22277f6c1f11112`，严格为 `created|false|0|0001-01-01T00:00:00Z`，contract SHA 与 prepared run 一致；按精确 ID 删除，probe 目录无残留，prepared manifest 未变，`FINAL_SWITCH_EXECUTED=false`。
- 服务器清理证据 `/srv/subnexus-migration/cleanup-20260905-v021-dangling-images.txt`（最终 SHA=`4db47e4249f5164fe8fb89642a93fe8bc058484d6b968373dffa2fc753b8677a`）：逐个删除共 27 个无 tag、无容器引用且普通 `docker image rm` 成功的 dangling 中间层，并删除 5 个已验证无用的 `/tmp/subnexus-v021-*` 上传临时文件；最终 dangling 列表为空。未使用任何 prune，未删除卷、数据库、Redis、旧应用、旧镜像、备份或证据。
- 最终只读复核：本轮 UI prepare 后应用 `9753053d8bd9...` running/healthy，PostgreSQL、Redis 原身份 running；旧 SubNexus anchor `be459424...` 仍按固定名称退出，无 candidate/probe 残留。所有二开功能继续关闭，Nginx 未修改。没有创建新的回滚目标，rollback 仍指向服务器原旧 SubNexus 容器。

## 2026-09-06 00:09（Asia/Shanghai）首页 Rain + Glass 发布准备进行中

- 本轮已访问线上，执行范围为候选构建/上传、隔离门禁、脚本安装、定向垃圾清理和在线 prepare；最终 switch/rollback 保留给维护者。此条覆盖上文把 `20260905114022-4163123` 写成 prepared 的旧交接快照：该 v0.2.1 run 实际已 switched，当前生产应用容器 ID 前缀为 `9753053d8bd9`。
- UI 已提交并推送：commit=`b1ed483ea5fc648cb3c15fcf2e7040e68a151a41`，tree=`bb821e2a0003d13cd425ca8ff012dbb26f70b1a6`。改动仅 `frontend/src/views/HomeView.vue` 展示与 `frontend/public/rain-city-1.jpg`；script setup 和 88 项原有模板业务绑定保持一致。原 `/images/rain-city-1.jpg` 引用会被生产网关路由绕过，最终改为 `/rain-city-1.jpg`；保留自定义/精简首页、配置、语言与主题按钮行为。
- 本地 `pnpm build`、首页 13 项测试通过；Puppeteer 覆盖 1440/390/320px 深浅共 6 场景，确认图片 HTTP 200/image/jpeg、2000x3000 可解码、主题可见变化、导航/菜单无横向越界且无页面错误，截图已人工检查。验证脚本和截图留在 `F:\MySub2\tools\verify-rain-home.mjs` 与 `F:\MySub2\home-preview`，不纳入生产包。
- 候选 image=`sha256:32f14750ce73da00dc4c5146b1d9ad6c4420ee2c3dffe098798e41a123c6bd2c`；服务器归档 `/srv/subnexus-migration/candidate-artifacts/rain-b1ed483ea5fc-retry2/candidate-image.tar`，SHA256=`26422d9eaad7ede983b228e84ee756eae313347b0135bf4e2d48138912c3246b`。最终 Docker gate `/srv/subnexus-migration/docker-candidate/20260905T155430Z-940fdcd9-bc3c-4d12-8c72-12ed9e27328b/evidence.txt` 和首页资源证据 `/srv/subnexus-migration/diagnostics/rain-home-b1ed483ea5fc.ENig2O5r` 均 passed；最终证据哈希待统一登记。
- UI 包装器提交 `33d43615c6e17e3f2ae5429f986ad636e971b8cb` 已推送，应用镜像仍绑定 UI commit。安装脚本 `/srv/subnexus-migration/tools/subnexus-ui-cutover-eef1d8f-20260905.sh`，SHA256=`eef1dfa31c71cfe33096d107561c594e0b509455b65db0caec824196d1cec77d`；原控制器 `/srv/subnexus-migration/tools/subnexus-production-cutover-19824a87-20260905-v021.sh`，SHA256=`19824a87e3e1de5659cb30664750b71c5c10d374f25bda7f52e6524fe477ee65`。包装器与原控制器使用独立哈希批准，不能互换。
- 固定回滚对象为之前旧 SubNexus ID=`be459424b327ad056ea9bdc02187d6a458fe09082369b354158d6e7f7758beee`，名称 `subnexus-cutover-pre-96b66b3e74c1-20260905085804-4072165`，anchor run=`20260905085804-4072165`，旧 image ID 前缀 `b24b585`。当前 v0.2.1 容器不替代此对象；本轮不创建新的永久回滚对象。
- 定向清理删除失败 partial 约 3.386 GB 和 7 个无引用构建镜像，prepare 前余量约 19.1 GB 增至 24.61 GB；日志 `/srv/subnexus-migration/cleanup-rain-20260905.txt`，SHA256=`94a4840ce2fd9b3c3dce40c5864a691675e4ca752b85f3dc3f437e550e2829c2`。未使用 prune，未删除旧回滚对象、业务数据、既有备份或审计证据。
- 当前在线 prepare run=`/srv/subnexus-migration/cutover/20260905160223-175225`，PID=`175225`，最近进度为 PostgreSQL dump，尚无 READY。备份结束、manifest 完整性、固定旧回滚合同、stopped probe 和最终状态/空间复核尚待完成；没有发布新 run 的可执行人工命令，也未执行 UI 切换。
- 本次文档只更新 `SUBNEXUS_PROJECT_CONTEXT.md`、`SUBNEXUS_MIGRATION_PLAN.md`、`SUBNEXUS_MIGRATION_LEDGER.md`、本文件及切换/回滚手册，撤回旧的可执行交接命令；后续取得最终 prepare/probe/哈希数据后再追加就绪记录，不能提前写准备完成。

## 2026-09-06 00:41（Asia/Shanghai）设置哈希漂移安全拒绝、空间恢复与第三次 prepare

- 首次 UI run `/srv/subnexus-migration/cutover/20260905160223-175225` 的 prepare 后续曾正常完成，但全量 settings 哈希从 `d66bf0e2c9ee6c1734bfa38cdae508e174562051e18acd093c14b81ab0e9705a` 变为 `af154e9a7a878bfc5295f12e88d4143c5466ab0c83939831fa13d202b71bc90a`。stopped probe 在 create 前安全失败，没有候选容器，未进入切换。
- 全量 settings 与原控制器仅 18 键 snapshot 的范围不对应，不能据此确定具体外部改键；当前哈希后续复验稳定，但具体发生变化的键、时间和来源尚未确定，不记录“原因已定位”。
- 第二次 `/srv/subnexus-migration/cutover/20260905163008-194872` 在备份前被空间门禁安全拒绝：可用 `18797457408` bytes 小于要求 `23715311616` bytes。未因此降低保留预算或复用旧备份，该 run 不可交接。
- 为恢复第三次准备的空间，首次失效 run 的三个大备份及对应 sidecar 已逐项 SHA 校验并记录后精确删除；其 manifest/settings/metadata 保留，并写入 `INVALIDATED_SETTINGS_DRIFT`，因此该 run 即使保留历史 READY 也不能执行。清理日志 `/srv/subnexus-migration/cleanup-rain-invalid-run-20260905160223.txt`，SHA256=`c3e1af6e289292b4b2baa8b76136ea322f19556785a17caf63d6d34c2060d326`，可用空间恢复为 `24025554944` bytes。
- 第三次 prepare 已启动：run `/srv/subnexus-migration/cutover/20260905163754-200276`，PID 200276，当前仍进行中且尚无 READY。UI commit/image、包装器/原控制器 SHA 均不变，固定旧 SubNexus `be459...` 及 anchor run `20260905085804-4072165` 不变，本轮不创建新的永久回滚对象。
- 六份文档的顶部及当前交接入口改为第三次 prepare，前两次均标为不可复用；未发布可执行 switch/rollback 命令。后续必须等待新 run 的全部备份、settings、manifest、固定旧对象、probe 和最终只读复核通过才可交接。

## 2026-09-06（Asia/Shanghai）— Rain + Glass UI 最终前置完成，停在人工 switch 前

- 历史 run `/srv/subnexus-migration/cutover/20260906082131-600835` 曾生成 `READY=prepared`，但因 wrapper manifest SHA 绑定错误不可交接；其后已使用修复后的 wrapper 完成最终 run。
- 最终备份已校验：PostgreSQL `5173037404` bytes SHA=`e6c58c2106d61815e0f12a2eeba6d34260457ac923e7432d14e534c9f54bdecf`；Redis `8739348` bytes SHA=`a92fb868e7a09df119047025eabc8f087ddaecc29ba6b23738976a8275912a5e`；应用数据 `82622462` bytes SHA=`9ffce4c054f56c105c1e0c17b2107bc5535ce7cee666707fd7f28894b7e4e307`；候选、Gate 和设置快照均保持已审计值。
- 历史 run `20260906082131-600835` 的 stopped probe 已完成并删除；该 run 仅作审计，不得交接。
- 最终服务器只读复核确认生产应用 `9753053d8bd9` 仍 running/healthy，PostgreSQL `8178576aed6f`、Redis `5c7adf42247c` 身份未改变；候选/probe 容器无残留，固定旧回滚对象 `be459424b327ad056ea9bdc02187d6a458fe09082369b354158d6e7f7758beee`、名称 `subnexus-cutover-pre-96b66b3e74c1-20260905085804-4072165`、anchor run `20260905085804-4072165` 仍保留。本轮未执行 switch、rollback、Nginx 切换、数据库恢复或功能开关修改，也未创建新的永久回滚对象。
- 前置清理范围已完成并留证：`/srv/subnexus-migration/cleanup-rain-20260905.txt` SHA=`94a4840ce2fd9b3c3dce40c5864a691675e4ca752b85f3dc3f437e550e2829c2`；失效 run 定向清理证据 `/srv/subnexus-migration/cleanup-rain-invalid-run-20260905160223.txt` SHA=`c3e1af6e289292b4b2baa8b76136ea322f19556785a17caf63d6d34c2060d326`；settings 漂移清理证据 `/srv/subnexus-migration/cleanup-rain-settings-drift-20260906.txt` SHA=`63d8afaf6cc2329561ce9fe3624aa731dd9acfb1e26b126f633df4177aa4e6d5`。未使用 prune，未删除数据库、Redis、线上镜像、旧 rollback anchor、有效备份或历史证据。
- 历史交接入口使用过 d24d wrapper，因 manifest SHA 绑定错误已撤回；当前唯一交接入口见文末最新记录。
- 追加失败审计：run `20260906075317-587100` 因漏传 timeout 默认 120 秒在约128秒失败；run `20260906081929-598740` 因不存在的 duplicate-env 审批参数安全拒绝。失败 partial 无残留，相关清理证据 `/srv/subnexus-migration/cleanup-ui-timeout-20260906.txt` SHA=`c94ffd3de43fee309723c2e8db7b284e9253bd9cbacf4691cfefc1c4e646ac93`；未执行 switch/rollback，固定旧 anchor 未变。

## 2026-09-06 22:09（Asia/Shanghai）— `F:\Rain` 首页源码直接迁移全部前置完成

- 目的与范围：按维护者要求，以 `F:\Rain` 中目标网页的真实首页源码作为默认首页 UI 基础，直接迁移页面结构、组件、CSS、三张原图、两层 Canvas、鼠标视差、动画、响应式布局和交互表现。当前项目的站点名、Logo、副标题、模型信息、文档、Model Plaza、登录/后台、主题、语言、客服、providers/footer、`/v1/messages`、权限判断、路由和按钮事件继续绑定既有实现；自定义/精简首页分支保留。未新增或删除功能，未修改后端、数据库迁移、依赖锁文件、功能开关或 Nginx。
- 源码与验证：应用 commit=`245ecd2630b96a9807df89dc02828bbb436e7624`，tree=`b0f55487331dca07031219d59cd4159cab8a610d`，已推送。Vitest `287/287` 文件、`1990/1990` 测试，定向首页 `4/4`，TypeScript、ESLint、生产构建和 UI wrapper `26` 个故障/恢复场景通过；Playwright `1440x900`、`390x844` 无溢出/页面错误，两层 Canvas 非空。三张图片与 `F:\Rain` 原文件 SHA256 完全一致。
- 构建与候选：image=`sha256:e472d61e8db88ec5cdd0c0c4ad9e9db11b28c3495a14af02287c99b6addf23a7`；archive=`/srv/subnexus-migration/candidate-artifacts/rain-245ecd2630b9/candidate-image.tar`，大小 `48065024` B，SHA=`65f06c3e221cfd08d68f84df882b2b3c858e5b8c939715f41c466eee0cb35f15`；source bundle SHA=`46bc0be1aca008eb6297f9935b7a4a0a1164494e49781f329542a3f905d5de57`。
- Gate：Docker evidence=`/srv/subnexus-migration/docker-candidate/20260906T133853Z-42fcab50-97af-46e0-b0c2-a67e512f819e/evidence.txt`，SHA=`d13c2a3095db4699d1a20939818d003e985f2715655f51e9715f286388d14544`；首页 observer=`/srv/subnexus-migration/diagnostics/rain-home-245ecd2630b9.n49MjhQK/evidence.txt`，SHA=`bd70fe35573d4a6c2ac6399cf50f9bec0cd18600b387ed8eea0e2f65ee76f678`。两者均通过且 `cleanup_failed=false`，候选临时资源无残留。
- 定向空间清理：失效 run `/srv/subnexus-migration/cutover/20260906082131-600835` 因 wrapper manifest SHA 绑定错误不可复用。在逐项记录 inode/size/SHA 后，仅删除其 `postgresql.dump[.sha256]`、`redis.rdb[.sha256]`、`application-data.tar.gz[.sha256]` 六个文件，共 `5264399409` B；manifest/settings/metadata 保留。证据 `/srv/subnexus-migration/cleanup-invalid-ui-wrapper-run-20260906082131-600835.txt`，SHA=`8992b5d48686997901fedd33bd89d3c552b624e31516e9e0b83489776abb9b40`。未使用 prune，未修改生产容器、数据库、Redis 或固定 anchor。
- 部署工具：UI wrapper=`/srv/subnexus-migration/tools/subnexus-ui-cutover-054507b1-20260906.sh`，SHA=`054507b15851c9547ab347f88ad21d8f9a5203be6123bfb2030e21c88806fd5d`；原控制器 SHA=`19824a87e3e1de5659cb30664750b71c5c10d374f25bda7f52e6524fe477ee65`。Stopped probe 脚本=`/srv/subnexus-migration/tools/probe-rain-prepared-b0c97b31-20260906.sh`，SHA=`b0c97b316fbe2cd79b16168119330625b8ed3aa2aefe205eedce04d9513acb42`；最终审计脚本=`/srv/subnexus-migration/tools/audit-rain-prepared-3d1e4a67-20260906.sh`，SHA=`3d1e4a6734750256e4c6744e4c6857a63c764c7b7994e6ead5bcc0583c858959`；均为 root:root/0700。
- 新 prepare：PID `774592` 正常退出；run=`/srv/subnexus-migration/cutover/20260906134705-774592`，`READY=prepared`、`UI_READY=application-refresh-v1`，manifest `state=prepared/ui_state=prepared/ui_commit_intent=no`，候选容器字段为空，manifest SHA=`e3809a4d6a09d469c994d38453551d466e73f49b45903aae17f8683fe63fc897`。日志明确记录没有停止、重命名、重启或切换生产容器，也没有创建回滚容器/镜像。
- 全新备份：PostgreSQL `5192049672` B，SHA=`56bf819f775a328716846be359b2c327cfe0f039a7ce5138853726517eaf44c0`；catalog `118684` B，SHA=`38b55fdc3f053550dbe13b67eb63f6823f60a436ba5e3913e15faa4be815bb4c`；Redis `7915377` B，SHA=`251696bef43dac6e6610c7d8d980261fc6827d5e6f3308b4aca8f488d9f43bf5`；应用数据 `82805759` B，SHA=`3fe9e535c8f404cffe733911677361d60f436bfa01a2b86488699ef99b2988c7`。全部实际文件、sidecar 和 manifest 三方哈希通过；`pg_restore -l`、Redis RDB check、应用 tar 复读通过。
- 设置与运行合同：18 个受保护设置的实时/prepare SHA=`3959daf3caed2f8a4c22023db4b7da8be627fb4b8a087bba4d5309cd8223d558`；closed intent SHA=`6a0ed24c164bb1fa8ebb5edecb8712f77458bfca145c713adfd105257fcda8c3`，仅作意图证据且未写入线上；runtime contract SHA=`7dc88dd8f76be1a69c6d4f322deb1b1e0eda8be94be61d37cac850091578453d`。应用数据目录仍为 `1000:1000/0755`，未 chown。
- Stopped probe 与最终审计：probe ID=`e9d9da3e60a7f134d52ff9389ea3eb594d72e3c921031f59408bdb80ffa62cde`，状态始终 `created|false|0|0001-01-01T00:00:00Z`，运行时合同通过后已按完整 ID 删除，临时目录和容器均无残留。证据 `/srv/subnexus-migration/diagnostics/rain-prepared-245ecd2630b9-20260906134705-774592.evidence` 为 root:root/0600，SHA=`0f69354a5d7911a66a6c5ef01fc58ac3160bd838a78fd214f8bb7151ac125609`；manifest 前后 SHA 不变，最终可用空间 `21903691776` B。
- 生产身份前后相同：live=`c3ea071f4526bdb2502444d8f18b9da4c761aa3d51be6f7e5fc19c910ca6300f`，image=`sha256:32f14750ce73da00dc4c5146b1d9ad6c4420ee2c3dffe098798e41a123c6bd2c`，running/healthy/restart=0；PostgreSQL=`8178576aed6f...`、Redis=`5c7adf42247c...` 均 running/restart=0。固定旧 SubNexus 仍为 ID=`be459424b327ad056ea9bdc02187d6a458fe09082369b354158d6e7f7758beee`、image=`sha256:b24b585a35e0eecff497a4eb7a2be480d9a2818f4b7a9780508f2f42cb5e09cd`、name=`subnexus-cutover-pre-96b66b3e74c1-20260905085804-4072165`、anchor manifest SHA=`e0b49d89e6a28044c5588afb5a536a3e87ceacfa2a7d679d324d65b14a4c100e`。
- 人工边界：本轮未执行 switch/rollback，未创建新永久回滚对象。旧 run `20260906100431-660485` 的现行命令已撤回；唯一可执行的 switch/rollback 命令写入切换手册第 13 节，均绑定本次新 run 并显式设置 `SUBNEXUS_DOCKER_TIMEOUT_SECONDS=120`，由维护者手动执行。

## 2026-09-06 23:25（Asia/Shanghai）— `F:\Rain` 首页生产切换成功并完成切换后验收

- 维护者手动执行本轮 `switch` 成功，终端原始成功行是 `UI_SWITCH_COMPLETED=/srv/subnexus-migration/cutover/20260906134705-774592`。同一 run 保留 `READY=prepared`、`UI_READY=application-refresh-v1`，新增 `SWITCHED=switched`，`ROLLED_BACK` 不存在；manifest 已更新为 `state=switched`、`ui_state=switched`、`ui_commit_intent=yes`，切换后 SHA256=`86afbaa48b5a22cdd193eb7f95238d8a70c74b870b7151476153317a3f0ffe79`。
- `2026-09-06T15:19:27Z` 启动的切换后只读审计完成全部实际检查，最终输出 `POST_SWITCH_AUDIT=passed` 且退出码为 `0`。新生产容器完整 ID=`86104829d490733244c9426a59e82e7a12afa3c590de2fc03f2ccb13344aebbd`，image=`sha256:e472d61e8db88ec5cdd0c0c4ad9e9db11b28c3495a14af02287c99b6addf23a7`，状态 `running/healthy/restart=0`，`StartedAt=2026-09-06T14:59:13.131115785Z`；manifest 中的 `live_app_id` 是切换前身份，切换后线上身份以 `candidate_container_id` 为准。
- 切换前线上容器 `c3ea071f4526bdb2502444d8f18b9da4c761aa3d51be6f7e5fc19c910ca6300f`、其切换临时名称、probe 容器和 probe 临时目录均无残留。PostgreSQL `8178576aed6f7b1cb94201832e5797907ea4d7698dbfe7b6f862cbc5a3b4f5bf` 与 Redis `5c7adf42247c67ba90b09248056071a57c2a4e7e0465f922d4ed799ef092533e` 保持原身份并为 `running/restart=0`。
- 全新 PostgreSQL、Redis、应用数据备份的实际文件、sidecar 和 manifest 三方哈希已重新核对通过；18 个受保护设置仍为 SHA256=`3959daf3caed2f8a4c22023db4b7da8be627fb4b8a087bba4d5309cd8223d558`，运行时合同仍为 SHA256=`7dc88dd8f76be1a69c6d4f322deb1b1e0eda8be94be61d37cac850091578453d`。文件 owner/mode、切换前容器日志和 previous-container 证据均通过审计。
- 固定旧 SubNexus 回滚对象仍为 ID=`be459424b327ad056ea9bdc02187d6a458fe09082369b354158d6e7f7758beee`、image=`sha256:b24b585a35e0eecff497a4eb7a2be480d9a2818f4b7a9780508f2f42cb5e09cd`、name=`subnexus-cutover-pre-96b66b3e74c1-20260905085804-4072165`，状态 `exited/restart=0`；anchor=`/srv/subnexus-migration/cutover/20260905085804-4072165`，anchor manifest SHA256=`e0b49d89e6a28044c5588afb5a536a3e87ceacfa2a7d679d324d65b14a4c100e`。本轮未创建新永久回滚对象，未执行 rollback。
- 公网 `https://yydsapi.uno` Playwright 验收在桌面 `1440x1000` 和移动端 `390x844` 均无页面错误或溢出；三张 Rain 原图正常加载，两层 Canvas 均非空。未登录状态下文档、模型广场、登录、语言和主题交互通过，配置驱动的站点名、Logo、副标题正常；客服沿用原配置 `customer_support_enabled=false`，因此按设计不显示。公网 Playwright JSON 报告 SHA256=`9851d28cc2645f79e4325b744fb1c8f80cc25cefae1f338c97eef7ac3d687855`。
- 本 run 的 `switch` 已成功消费，禁止再次执行；所有历史 switch 命令同样禁止复用。在该时间点，唯一恢复入口是切换手册中绑定 `/srv/subnexus-migration/cutover/20260906134705-774592` 的同 run `rollback` 命令；当前状态由文末最新记录覆盖。

## 2026-09-07（Asia/Shanghai）— 保留二开用户端界面完整迁移（本地候选，发布前）

- 维护者将本轮目标明确为：以 `F:\Sub2Api\SubNexus` 中 F01-F13 保留能力的用户端源码为实际显示基础，迁移页面结构、组件层次、样式、文案、弹窗、响应式行为和交互反馈；数据来源、API、请求参数、路由、鉴权、权限判断、功能开关、支付和奖励处理继续使用 `F:\MySub2\sub2api` 当前实现。不得新增或删除业务功能。
- 已覆盖的用户端显示包括活动中心、排行榜、邀请抽奖、累计充值奖励转盘、邀请里程碑、Affiliate 邀请页、公告/跑马灯、客服按钮与 Markdown 弹窗、签到、首充、学生优惠、发票、Battle Pass 和 Channel Monitor V3。通用旧版 primitive 通过 `.subnexus-legacy-surface main` 限定作用域；Dashboard 只在签到组件局部启用，Payment 只在首充和学生优惠区块局部启用，Channel Monitor V1/V2 保持当前实现。
- 充值金额输入恢复旧版默认选中行为并增加组件测试；中文学生资格副文案恢复为“学生身份已生效”。三个邀请活动仍保留当前独立入口，不恢复旧版单入口或活动红点；站点名称、Logo 和可配置内容继续来自当前公共设置。
- 明确排除每日消耗转盘、红包雨、运行日历、Media Studio 和 Creative Workshop/创意工坊。没有恢复这些功能的页面、路由、API、任务或旧活动联动。
- 发布 wrapper 使用相对生产基线的精确 allowlist：34 个 production UI 文件和 18 个测试/记忆/部署证据文件，共 52 个；与当前候选差异精确匹配。API、后端、数据库迁移、router、`package.json`、锁文件、全局 `style.css`、Tailwind 和既有 Rain 首页不在放行范围；删除、符号链接、可执行位以及仅文档/测试的候选均会被拒绝。`.codex-ui-mock-server.mjs` 是本地临时工具，必须排除于提交和制品。
- 已取得的阶段性证据包括：邀请活动 19 项测试通过、scoped dark 编译 7 项测试通过、邀请页面 typecheck 通过、BaseDialog/客服/滚动锁等定向检查通过、生产构建和 ESLint 通过，以及 UI wrapper 26 组故障/恢复与 source-contract 测试通过。最终提交前仍须统一重跑定向及全量 Vitest、typecheck、lint、build、wrapper、`git diff --check`，并完成桌面/移动、明暗主题的实际 Playwright 对比和溢出检查；阶段性证据不能提前写成最终 Release Gate。
- 本轮候选 UI commit/tree 尚未固定，也没有新镜像、归档、远端 Gate、prepare run 或可执行切换命令。生产 base 固定为 `245ecd2630b96a9807df89dc02828bbb436e7624`；提交后必须重新证明完整目标从该 base 派生，并重新计算镜像、归档、source bundle、wrapper 和全部证据 SHA。
- 发布顺序固定为：本地最终验证与提交推送、隔离构建、上传并安装唯一制品、候选 Docker Gate、无停机 `prepare`、全新备份校验、never-started stopped probe、设置/运行时/空间/生产身份最终审计。全部完成后停在新 run 的 `switch` 前，由维护者手动执行 switch；同时只提供绑定同一新 run 的 rollback。
- 固定旧 SubNexus 回滚对象保持不变：ID=`be459424b327ad056ea9bdc02187d6a458fe09082369b354158d6e7f7758beee`，image=`sha256:b24b585a35e0eecff497a4eb7a2be480d9a2818f4b7a9780508f2f42cb5e09cd`，name=`subnexus-cutover-pre-96b66b3e74c1-20260905085804-4072165`，anchor=`/srv/subnexus-migration/cutover/20260905085804-4072165`，anchor manifest SHA256=`e0b49d89e6a28044c5588afb5a536a3e87ceacfa2a7d679d324d65b14a4c100e`。本轮不得创建新的永久回滚容器或镜像，也不得把当前生产应用改作回滚目标。

## 2026-09-07（Asia/Shanghai）— 保留二开用户端界面全部前置完成，停在人工 switch 前

- 范围与功能合同：活动中心、排行榜、Affiliate、邀请抽奖、累计充值奖励转盘、邀请里程碑、公告/跑马灯、客服、签到、首充、学生优惠、发票、Battle Pass 和 Channel Monitor V3 的用户端结构、组件、样式、文案、弹窗与响应式表现已按 `F:\Sub2Api\SubNexus` 对齐；API、请求参数、路由、鉴权、权限、配置、功能开关、支付、奖励、幂等和数据写入继续使用当前项目实现。每日消耗转盘、红包雨、运行日历、Media Studio、Creative Workshop 仍明确排除，未新增或删除业务功能。
- 源码与本地门禁：候选 commit=`f6f6dafe1fb2008d0a6f41dc746ae831babc3b18`，tree=`7b0ee6db2dc96fd97106ca175640b3a15e8ec233`，已推送到 `origin/feature/subnexus-migration`；生产 base=`245ecd2630b96a9807df89dc02828bbb436e7624`。`base..target` 精确为 52 个路径，其中 34 个 production UI、18 个测试/文档/部署证据文件；API、后端、数据库迁移、router、依赖/锁文件、全局样式、Tailwind 和 Rain 首页均未改变。定向/全量前端测试、typecheck、lint、build、wrapper 26 场景、桌面/移动明暗主题视觉对比及关键交互检查均通过。
- 不可变制品：candidate image=`sha256:59eb4c84de8b8fec11fb903dc728676e9cffacbcc435ce5ea1b60487cc910fcc`；archive SHA256=`aae7dbca9336a81414f06fb273f8b66c1723aaef891316e4fe32827bf9650084`；UI wrapper=`/srv/subnexus-migration/tools/subnexus-ui-cutover-dd320d09-20260907.sh`，SHA256=`dd320d0982d357704d88bd702805cca69c36304ea6f5db523a572b10bdbdf49a`；原控制器 SHA256=`19824a87e3e1de5659cb30664750b71c5c10d374f25bda7f52e6524fe477ee65`。
- 候选 Gate：Docker evidence=`/srv/subnexus-migration/docker-candidate/20260907T044220Z-38d5e210-40a5-40f4-a52a-d521af351d7e/evidence.txt`，SHA256=`6ed8c611f8c86893c49f5594f6f2b39cd01b010e26f68a7b0b2afcde82e39c8a`；首页 observer=`/srv/subnexus-migration/diagnostics/rain-home-f6f6dafe1fb2.oNeOWKxE/evidence.txt`，SHA256=`272626c264aaf7d12811460707f231a903af89affbec342315434c1ea7cfac70`。Gate handoff SHA256=`4bf0895d8e031fb20ac283f5f47f757cd33b56023ae2b6e190bcb1d55f183f72`；临时候选资源已清理。
- 无停机 prepare：run=`/srv/subnexus-migration/cutover/20260907045159-1121373`；`READY=prepared`、`UI_READY=application-refresh-v1`，manifest `state=prepared/ui_state=prepared/ui_commit_intent=no`，不存在 `SWITCHED` 或 `ROLLED_BACK`，manifest SHA256=`87b51ae80dccc6ae4590537bcc2636795b0a650db84a7bb40a9531b4ce85d135`；prepare log SHA256=`bc03f557b35d822629ddf22f0522b98e20e734a0aa8e4cb75aeff469be6ed2ad`。
- 全新备份：PostgreSQL `5275269397` B、SHA256=`d5528d86bcdd84c86ba5db056235a94349045f63f985de3d55ce4064053548c3`；catalog `118684` B、SHA256=`7dd511aac04df73ea95f0d4bc74a8b4b3b32b44dca6a5bc3d76d56829ccefc85`；Redis `9703822` B、SHA256=`28052d344e428c96da409c35225ad099b0ba3751f9f5f5ca9491a5c8661546aa`；Redis check SHA256=`657be8ca65447e4a209d77303715e4aa8b1f87f1e871bde83ab7cf6972e45c1c`；应用数据 `71058234` B、SHA256=`f686be37409fa776253f62e9bcc638b6e3028209276a9e6dd93ac478a92819a2`；排除策略 SHA256=`ee1908db818e2434a9e5a47ec84a02ac10eafd11bc79767e1313f3f6e659826d`。实际文件、sidecar、manifest、归档复读和恢复格式检查均通过。
- 运行合同与 probe：18 个受保护设置 SHA256=`3959daf3caed2f8a4c22023db4b7da8be627fb4b8a087bba4d5309cd8223d558`；runtime contract SHA256=`7dc88dd8f76be1a69c6d4f322deb1b1e0eda8be94be61d37cac850091578453d`。probe ID=`44891195f23177ccf3c05e30b54755eb9345980649772218c96385ee5dbdbe51` 从未启动并已按完整 ID 删除，probe log SHA256=`bec9ea3cf132de3839397747d957c2ac7e7522d7a24de499e11404f374f90c1d`；最终可用空间的最近读数约 `56059801600` B。
- 严格最终审计：首次审计因残留查询存在 fail-open 风险而撤回，不作为发布证据。修正版本地脚本 `F:\MySub2\tools\audit-retained-ui-prepared-f6f6dafe1fb2.sh` 与远端 `/srv/subnexus-migration/tools/audit-retained-ui-prepared-5239d9c1-20260907.sh` 的 SHA256 均为 `5239d9c17d03f7bd6dce24daed7e2c1d8216d9e98b854cafec4a364fec13f7f6`；新 evidence=`/srv/subnexus-migration/diagnostics/retained-ui-prepared-f6f6dafe1fb2-20260907045159-1121373-v2.evidence`，SHA256=`9dc1293e6ab16af8b1f805b30e39eba5ecfa55e2b07015ae251a308b70c234ea`，明确输出 `FINAL_PRE_SWITCH_AUDIT=passed` 和 `FINAL_SWITCH_EXECUTED=false`。
- 生产与回滚边界：当前 live=`86104829d490733244c9426a59e82e7a12afa3c590de2fc03f2ccb13344aebbd`，running/healthy/restart=0；PostgreSQL 与 Redis 身份未变。固定旧 SubNexus 仍为 ID=`be459424b327ad056ea9bdc02187d6a458fe09082369b354158d6e7f7758beee`、image=`sha256:b24b585a35e0eecff497a4eb7a2be480d9a2818f4b7a9780508f2f42cb5e09cd`、name=`subnexus-cutover-pre-96b66b3e74c1-20260905085804-4072165`、anchor=`/srv/subnexus-migration/cutover/20260905085804-4072165`、anchor manifest SHA256=`e0b49d89e6a28044c5588afb5a536a3e87ceacfa2a7d679d324d65b14a4c100e`。prepare 未创建永久回滚容器或镜像。
- 人工边界：代理已完成全部前置工作并停止在 `switch` 前。只有新 manifest 仍为 `state=prepared/ui_state=prepared`、switch 进程已退出或未运行且当前生产仍为 `86104829...` 时，第 13 节旧 Rain run rollback 才是有效入口；该成功历史 run 不得作为新的 `prepare`/`switch` 输入，这条 rollback 是明确的状态窗口例外。若实际执行第 13 节 rollback，当前生产身份会改变，本轮新 prepared run 与第 14 节 switch 立即失效；不得继续执行第 14 节任一命令，必须从新生产身份重新 `prepare`、完成 probe 与严格审计并重新交接。维护者启动第 14 节 switch 后必须保持当前终端等待进程返回，不得并发执行任何 rollback；wrapper 会在失败路径自动尝试恢复切换前当前容器。进程退出后重新读取 manifest：仍为 `prepared` 时第 14 节 rollback 会拒绝；已离开 `prepared` 且仍需恢复固定旧 SubNexus 时才使用第 14 节同 run rollback。rollback 默认不恢复 PostgreSQL/Redis、不修改 Nginx 或功能开关。
- 历史语义限定：本文此前记录中的“历史/成功 run 不得复用”均指不得作为新的 `prepare`/`switch` 输入；已失败、已自动回滚或已覆盖的 run 仍不得执行 rollback。`2026-09-06 23:25` 条目所称 Rain run 的“当前唯一恢复入口”是当时状态，现由上一条三阶段窗口合同覆盖；仅第 13 节成功 Rain run 在该窗口内保留 rollback 例外。

## 2026-09-07（Asia/Shanghai）— 保留二开用户端界面切换成功及切换后收口

- 维护者手动执行本轮 `switch` 成功，终端成功行是 `UI_SWITCH_COMPLETED=/srv/subnexus-migration/cutover/20260907045159-1121373`。原有 `READY=prepared`、`UI_READY=application-refresh-v1` 继续作为切换前 marker 保留，新增 `SWITCHED=switched`，`ROLLED_BACK` 不存在；manifest 为 `state=switched/ui_state=switched/ui_commit_intent=yes`，切换后 SHA256=`5cc60f478673b2615d96d2993604da934353a6abb66d932e81f268bdfa4acda3`。
- 当前生产容器完整 ID=`232f6c5b374605760529cfac6b765fe68ba6aafc0d5d0fc8641a6a3030d63511`，image=`sha256:59eb4c84de8b8fec11fb903dc728676e9cffacbcc435ce5ea1b60487cc910fcc`，`running/healthy/restart=0`；manifest 中 `live_app_id=86104829d490733244c9426a59e82e7a12afa3c590de2fc03f2ccb13344aebbd` 仍是切换前身份，当前线上身份以 `candidate_container_id` 为准。旧 live `86104829...`、temporary name、gate/probe 容器和目录均已按精确身份清理。
- 切换后服务器只读审计工具 `/srv/subnexus-migration/tools/audit-retained-ui-switched-d5652bda-20260907.sh` 的 SHA256=`d5652bda85a82e5eaa20db5f5b80a86c691bcdbbb10f58d199636f0ff7f6766d`；正式 evidence `/srv/subnexus-migration/diagnostics/retained-ui-switched-f6f6dafe1fb2-20260907045159-1121373.evidence` 的 SHA256=`58edc5b2d6e3ca6535ae10741ce4aed9275609f5d5e8dad1c48407372349afb9`，明确输出 `POST_SWITCH_AUDIT=passed`。PostgreSQL=`8178576aed6f7b1cb94201832e5797907ea4d7698dbfe7b6f862cbc5a3b4f5bf`、Redis=`5c7adf42247c67ba90b09248056071a57c2a4e7e0465f922d4ed799ef092533e` 身份未变，均 `running/restart=0`；runtime contract 保持 SHA256=`7dc88dd8f76be1a69c6d4f322deb1b1e0eda8be94be61d37cac850091578453d`。
- 设置漂移审计只发现 `subnexus_invite_activities_config` 变化。Nginx 有管理员 `PUT /api/v1/admin/invite-activities/config` 返回 200，最后一次为 `2026-09-07 06:45:51 UTC`；数据库中该配置与 `subnexus_invite_activities_enabled` 的 `updated_at` 同为 `2026-09-07T06:45:51.718553Z`。候选相对生产 base 的后端差异为零，该配置唯一正常写入口是管理员 PUT；因此记录为切换后的管理员操作，未回写或恢复设置。当前 18 项保护设置 SHA256=`eddb4a4333c07c1cea357f12e2adb0bede963f6f0806f6757895c8d3f0f092d4`，其余 17 项保持 prepare 值。
- 公网 `https://yydsapi.uno` 的桌面 `1440x1000` 和移动 `390x844` Playwright 验收通过：无页面错误、请求失败、HTTP 错误或横向溢出，三张 Rain 图片正常解码，两层 Canvas 非空且持续变化；语言、主题、文档、模型广场、登录入口正常，站点名、Logo、副标题继续读取线上配置，客服按既有 `customer_support_enabled=false` 隐藏。JSON 报告 `F:\MySub2\.playwright-qa\production-postswitch-f6f6dafe1\production-home-qa.json` 的 SHA256=`5594bc6c36c2b3fd83728b368587b718609d2fd168b8a16caf5b7ac273a0d63c`，desktop PNG SHA256=`f8ae119bea2b5fd150c44308df997ba4ffd4f2818d13ce2946d7b1ba582e72b9`，mobile PNG SHA256=`20e44cae7b71ecf38c39fab81b1a34c13e2d658c58a9fbc56d3cfd9b8993ba8e`。
- 固定旧 SubNexus 回滚对象仍为 ID=`be459424b327ad056ea9bdc02187d6a458fe09082369b354158d6e7f7758beee`、image=`sha256:b24b585a35e0eecff497a4eb7a2be480d9a2818f4b7a9780508f2f42cb5e09cd`、name=`subnexus-cutover-pre-96b66b3e74c1-20260905085804-4072165`，`exited/restart=0`；anchor=`/srv/subnexus-migration/cutover/20260905085804-4072165`，anchor manifest SHA256=`e0b49d89e6a28044c5588afb5a536a3e87ceacfa2a7d679d324d65b14a4c100e`。本轮没有创建新的永久回滚对象。
- 切换后精确清理了失败审计 partial、一次性迁移上传/诊断副本等 27 个临时文件；清理 evidence=`/srv/subnexus-migration/cleanup-retained-ui-postswitch-20260907-1121373.txt`，root:root/0600，SHA256=`282250b4f612f154e60c7d1b42954ac005b9bb8b3e6db11710a08dacf46b6558`。正式 evidence、备份、应用数据、固定回滚对象和必要发布制品均保留，未使用 prune。
- 本轮 switch 已消费并从现行交接撤回，禁止重跑；第 13 节旧 Rain rollback 状态窗口已关闭并撤回。当前唯一可执行恢复入口是切换手册第 14 节绑定 wrapper `dd320d09...` 与同一 run `20260907045159-1121373` 的 rollback。该命令只在 switch 进程已退出、manifest 已离开 `prepared` 且确需恢复固定旧 SubNexus 时使用；默认不恢复 PostgreSQL/Redis，不修改 Nginx/功能开关，也不创建新的永久回滚对象。
- 本轮 UI 上线不代表 F01-F13 在功能开关开启后的业务验收已经完成；这些业务验收状态保持不变。线上操作仅包括维护者手动 switch、代理的只读复核/审计与上述精确临时文件清理；未执行 rollback、数据库/Redis 恢复、Nginx 修改或功能开关变更。

## 2026-09-07（Asia/Shanghai）— 用户端 Rain + Glass 背景与卡片材质本地实施完成

- 已按 `SUBNEXUS_USER_GLASS_SURFACE_PLAN.md` 完成用户端视觉层实施。`AppLayout.vue` 根据 `route.meta.requiresAuth === true && route.meta.requiresAdmin !== true` 仅为登录用户端挂载现有 `RainyBackground`，并保留管理端、认证页、公开页和支付回调页的原背景路径；页面内容层、侧栏、顶部栏、卡片、表格、分页和下拉菜单通过 `.user-glass-surface` 作用域统一背景透明度、边框、阴影、模糊和层级。
- 直接绕过公共 `.card` 的用户端面板已补充 `user-glass-panel`、`user-glass-inset`、`user-glass-control`、`user-glass-pagination` 和 `user-glass-table` 标记，覆盖 `/dashboard`、`/keys`、`/usage`、`/monitor`（V1/V2/V3）、`/subscriptions`、`/purchase`、`/orders`、`/invoices`、`/profile`、`/available-channels`、`/batch-image` 以及现有用户活动/奖励页面。语义色活动区、按钮、输入框、徽章、进度条、代码块和弹窗内部专用材质保持原样。
- 业务合同未改变：没有修改 API、请求参数、路由定义或守卫、权限判断、登录状态、配置读取、功能开关、按钮事件、数据写入、依赖锁文件、后端或生产配置。Teleport 到 `body` 的 BaseDialog/Select 内容保持公共实现，避免把用户端材质泄漏到管理端；`/payment/result` 和外部支付 popup 仍不挂载用户端雨景。
- 共享雨景组件新增可选水滴层级，并在用户端背景层使用 `z-index: 0`；动画在 `prefers-reduced-motion` 或页面不可见时释放循环/监听器。该变化只影响装饰性显示和资源占用，不改变业务交互。
- 新增源码契约测试 `frontend/src/components/layout/__tests__/UserGlassSurface.spec.ts`，确认用户端路由判定、CSS 作用域隔离和 tooltip opt-in。最终本地验证：`pnpm test:run` 为 `294` 个测试文件、`2037` 个测试全部通过；`pnpm typecheck`、`pnpm lint:check`、`pnpm build`、`git diff --check` 均通过。Vite 构建输出到既有 `backend/internal/web/dist`，仅保留既有动态导入、chunk 大小和 Browserslist 警告。
- 已在本地 Vite `http://127.0.0.1:3100/` 对用户端桌面/移动视口及明暗主题做视觉冒烟：雨景在内容后方，导航和控件可点击，玻璃卡片/表格层级正常，横向滚动和原有错误提示路径未被改变。当前工作树改动尚未提交；本轮未执行服务器操作、线上部署、switch 或 rollback，也未创建新的回滚对象，后续仍沿用既有旧 SubNexus 回滚目标。

## 2026-09-07（Asia/Shanghai）— 用户端玻璃层审核问题修复完成

- 针对审核发现的层级问题，用户端 `RainyBackground` 的 Teleport 水滴 canvas 固定为 `z-index: 9`，位于页面洗色层（`z-index: 1`）之上、主内容（`z-index: 10`）及其内部对话框之下；`.user-glass-surface` 已移除 `isolation: isolate` 和根节点 z-index，避免 body 层 canvas 被整页 stacking context 压到雨景后方。
- `user-glass-inset` 只在非悬停状态提供默认材质，带有现有 `hover:bg-*` 或 `dark:hover:bg-*` 的快捷操作按钮和最近用量行继续使用原有悬停底色；DataTable sticky 表头与固定列改用明暗主题不透明填充并保留 hover/选中行状态；订单页恢复原始 `OrderTable` 结构，移除额外 `overflow-hidden` 包裹。
- 新增/更新源码契约覆盖上述四项回归。修复后 `pnpm test:run` 通过 `294` 个测试文件、`2041` 个测试，`pnpm typecheck`、`pnpm lint:check`、`pnpm build` 和 `git diff --check` 均通过。仅完成本地验证，未执行部署、线上切换、回滚或新回滚对象创建。

## 2026-09-07（Asia/Shanghai）— 用户端玻璃层运行时复核与减少动态兜底

- 运行时复核确认层级为 `wash z1 < Teleport 水滴 z9 < main z10`，顶栏 `z30`、侧栏 `z40` 及主内容内部既有弹层层级保持有效；水滴和雨丝 canvas 均为 `pointer-events:none`，不拦截页面操作。
- 为兼容 `prefers-reduced-motion` 或页面不可见状态，`RainyBackground` 新增 `animated` 通道：用户端仍显示一帧静态雨丝/水滴，动画关闭时不注册 RAF、resize 或点击监听；普通首页默认行为不变。该修正解决浏览器减少动态偏好下 canvas `opacity:0` 导致的视觉缺失。
- 使用本地 Vite `http://127.0.0.1:3100/` 做桌面 `1440x1000`、移动 `390x844`、深浅主题验收；雨图加载完成，两层 canvas 非空，滚动宽度等于视口宽度，用户菜单、移动侧栏和 `/keys` 导航可用。QA 截图保存在 `F:\MySub2\.playwright-qa\user-surface-desktop-dark.png`、`user-surface-desktop-light.png`、`user-surface-mobile-dark.png`；mock-only 的数据形状错误已排除，生产代码无 pageerror/console error。
- 最新验证：`pnpm test:run` 为 `294/294` 测试文件、`2041/2041` 测试通过；`pnpm run typecheck`、`pnpm run lint:check`、`pnpm run build`、`git diff --check` 均通过。当前工作树仍未提交；本轮未访问线上、未部署、未执行 switch/rollback、未创建新的回滚对象，继续沿用既有旧 SubNexus 回滚目标。

## 2026-09-08（Asia/Shanghai）— Rain + Glass 线上发布合同更新

- 维护者已批准视觉审核并授权代理完成提交推送、隔离构建、上传安装、候选 Gate、全新在线备份、无停机 prepare、never-started probe、容量处理和最终切换前审计；最终 switch 由维护者手动执行。交接必须给出绑定本轮同一 wrapper/run 的一整行 switch 和一整行 rollback。
- 本轮发布范围仍仅为用户端背景和卡片材质。复跑完整前端验证为 `294/294` 个测试文件、`2041/2041` 个测试通过，typecheck、lint、production build 与 `git diff --check` 通过；构建只有既有的动态导入、chunk 大小和 Browserslist 警告。不可变候选 commit=`187b128bd32d1e06ad6e08817632e7c6b5ccca92`、tree=`e81b71b7f136da0a62bd51e266f041c6312e6b0b` 已推送；UI wrapper 32 个故障/恢复/source-contract 场景已在 Git Bash 与 WSL/Linux 通过，build/gate/UI wrapper SHA256 分别为 `cbec521753cc5fa18bf96a4fd1dd58b32ff026fd76009189e8015a2d201b8aa3`、`7aed2fbd5a5024b670cb544def5f85f70ee3830a2a720d9abd5e670c0c640ff7`、`6b1635548887459ad408d56226fdceadbaa8d72b845e8b3a3dac3ae65815233f`，wrapper test SHA256=`6a682d9f33d308eb648c914519f08e1e1afdc8a041095bfc3dddd6e38db82b26`。后续只读记账提交不得替代该镜像构建 SHA；image、archive、Gate、备份、run 和审计值尚未生成，当前记录不填入推测值。
- `tools/production-deploy/subnexus-ui-cutover.sh` 的发布合同改为“切换时保留当前 live”：prepare 仍要求历史旧 SubNexus anchor 存在并完整核验，只把当前 live 的完整 ID、`.Image`、`.Config.Image`、唯一 `production-app-ui-prior-<run-id>` 名称和 `prepared` 状态写入 manifest，不改变 Docker 状态。最终 switch 停止并重命名该 live，确认其为 stopped 后再启动候选；候选健康后继续保留该 stopped 容器作为本轮新回滚目标。
- 本轮 rollback 只恢复新回滚目标，不再恢复更早的历史 SubNexus。新目标缺失，或 ID、`.Image`、`.Config.Image`、名称、runtime contract 任一漂移时必须失败关闭；不采用预创建 stopped anchor、`docker commit` 或额外回滚镜像。
- 历史 anchor 不再是本轮实际恢复对象，但现行 wrapper 仍要求其容器/镜像与证据贯穿 switch、失败恢复和 rollback 窗口保持完整，缺失时失败关闭。空间足够时保留其他历史数据；磁盘证据确认不足时，只能在待删对象的完整身份/路径/SHA 与无引用状态逐项确认并保存 root-only 清理记录后，精确删除其他失效 run 备份或垃圾。不得执行 `docker system prune`、`docker volume prune` 或前缀/通配符清理；历史 anchor、本轮新回滚目标、run、备份和最终审计不得删除。
- 2026-09-07 第 14 节发布记录保持历史事实；其 switch 已消费，旧 rollback 在本轮新发布交接中撤回。当前唯一现行交接为切换手册第 15 节，具体命令需等全部前置证据完成后生成。

## 2026-09-09（Asia/Shanghai）- 用户端下拉层级与签到显示本地修复

- 维护者反馈 Rain + Glass 上线后 `/usage` 日期下拉被后续图表覆盖，签到显示 `checkin.title` 等裸翻译键。本轮仅处理显示问题，没有进行服务器操作或再次发布。
- 层级根因是卡片的 `backdrop-filter` 建立独立层叠上下文。`UsageView` 两张筛选卡片和 `UserDashboardCharts` 日期卡片增加 `user-glass-menu-host`，独立用户 CSS 通过 `:has(.date-picker-dropdown, .user-glass-menu)` 在菜单打开时提升整张卡片到局部 `z-index:20`；关闭后恢复默认层级。已有 `Select`、Keys 分组菜单等 Teleport 实现不变。
- 运行时同时发现手机日期菜单固定最小宽度 320px 会越过右侧屏幕。`DateRangePicker` 根节点仅增加 `date-picker-container`，用户 CSS 在 639px 以下让菜单锚定卡片并左右保留 1rem，自定义日期输入纵向排列。规则仍同时要求 `.user-glass-surface` 与显式菜单容器，管理端和其他日期组件使用处不受影响。
- 签到改为正确的 `common.checkin.*` 翻译路径；对照 `F:\Sub2Api\SubNexus\frontend\src\components\user\dashboard\UserDashboardCheckIn.vue`，恢复普通日“每日签到”、角标“礼盒”、底部“连续礼盒”，英文语义对应。礼盒角标使用独立 `milestoneBadge` 翻译，签到排列和角标位置保持原版。接口、开关、权限、签到条件、金额、冻结处理、充值路由和所有请求/业务事件不变。
- 新增真实 vue-i18n 组件回归测试，覆盖中文文案、天数及金额插值、成功后状态、英文切换、冻结状态。完整 `pnpm test:run -- --reporter=dot` 为 `295/295` 文件、`2044/2044` 测试通过；类型检查、`pnpm run lint:check`、`pnpm run build`、`git diff --check` 通过。构建保留既有 Browserslist、混合导入、chunk 大小等警告，没有新增依赖或修改锁文件。
- 本地 Playwright 使用最终生产构建与浏览器内模拟接口，覆盖 1440x1000、768x1024、390x844、320x740 的深浅主题。下拉矩阵 32 个命中测试通过；日期预设、自定义日期请求参数、ESC/外部点击关闭与关闭后的层级恢复通过；签到中英文显示及礼盒角标和天数矩形不相交通过。另 32 个 Keys 列菜单、密钥弹窗、顶栏用户菜单和真实管理路由 `/admin/checkin` 隔离检查通过，浏览器无页面错误。没有使用生产账号或向生产发送业务请求。
- 本地复验工具：`F:\MySub2\.playwright-qa\user-dropdown-qa.cjs`；报告/截图：`F:\MySub2\.playwright-qa\user-dropdown-20260909\report.json`、`aux-report.json` 及同目录 PNG。生产构建预览运行于 `http://127.0.0.1:3101/`，开发前端运行于 `http://127.0.0.1:3100/`；测试模拟数据只存在于自动化浏览器内，手动使用服务仍需本地业务后端。
- 当前修复保留在未提交工作树，尚未更新线上；未创建或删除回滚对象，未执行 switch/rollback。后续发布仍须完成独立前置检查并由维护者手动执行最终切换。
- 维护者随后确认签到只需修改文案；已撤销礼盒格顶部留白和角标位置调整，签到排列恢复原版横向布局，其他下拉层级修复保持不变。

## 2026-09-09（Asia/Shanghai）- 本次 UI 修复发布沿用固定旧 SubNexus 回滚目标

- 本条覆盖 2026-09-08 发布合同中的“保留切换前 live 为新回滚目标”约定，不重写该次历史事实。本次由代理完成发布前置并停在最后切换前，维护者手动执行最终 switch；不新建永久回滚容器、不执行 `docker commit`，也不把当前生产 live 改为本次最终回滚目标。
- 固定恢复对象仍为旧 SubNexus：容器 ID=`be459424b327ad056ea9bdc02187d6a458fe09082369b354158d6e7f7758beee`，image=`sha256:b24b585a35e0eecff497a4eb7a2be480d9a2818f4b7a9780508f2f42cb5e09cd`，name=`subnexus-cutover-pre-96b66b3e74c1-20260905085804-4072165`；anchor=`/srv/subnexus-migration/cutover/20260905085804-4072165`，anchor manifest SHA256=`e0b49d89e6a28044c5588afb5a536a3e87ceacfa2a7d679d324d65b14a4c100e`。这些为固定身份，仍需每次线上实时复核，不能把文档值当作已完成的新证据。
- 当前 UI wrapper 的 prepare 仍建立全新 run 和备份，固定 `ui_anchor_*`、`ui_rollback_id/image/name` 与 `ui_temporary_name`，但不写入 `ui_new_rollback_*`。prepare 不停止、重命名或创建回滚容器；never-started probe 是后续独立验证步骤，不是永久回滚对象。
- switch 过程会暂存切换前 live，供候选提交前的失败恢复使用；候选健康和合同校验通过后，保留诊断日志并按完整 ID 删除该临时前版本容器。此过程恢复与正式 rollback 不同：本次正式 rollback 通过同一新 wrapper/run 恢复固定旧 SubNexus，默认不恢复数据库或 Redis，也不修改 Nginx、设置或功能开关。
- 既有历史回滚对象和证据不会因合同变更自动删除，也不替代固定目标。空间足够时保留；需要清理时按完整身份、精确路径、SHA 与引用关系逐项确认并保留 root-only 审计记录。固定旧 SubNexus 容器/镜像及 anchor、本次有效 run/备份/审计不可删除，不使用 prune 或通配符删除。
- 已同步 `SUBNEXUS_CUTOVER_RUNBOOK.md` 第 15.2 节和 `SUBNEXUS_MIGRATION_LEDGER.md` 当前合同；第 15.1 节和 2026-09-08 台账保留为历史准备快照。本次应用候选、镜像、脚本安装路径/SHA、Gate、备份、prepare run、probe 与最终审计值必须单独生成核验，本条不提供可执行命令，不表示线上前置或切换已经完成。


## 2026-09-09（Asia/Shanghai）— 用户端视觉性能分级与发布准备

- 维护者批准实施标准、轻量、兼容三档视觉模式，并要求完成全部线上前置、停在最终 switch 前；本次不创建新的回滚目标，继续恢复固定旧 SubNexus be459424… / anchor 20260905085804-4072165。
- 头像菜单提供自动和三档显式选择；浏览器本地保存手动偏好，自动判断使用移动视口、减少动态偏好和设备能力。自动运行监测只采样可见页面，经过预热和连续低帧窗口才降为轻量，不将自动结果写作手动选择；用户可以恢复自动。
- 轻量保留静态雨景与稀疏水滴，卸载雨丝，不运行装饰动画或指针视差，减少外层模糊并取消内层重复模糊。兼容卸载雨景和两个 Canvas，恢复原有背景/卡片并关闭基础玻璃滤镜。标准保留动态效果，限制刷新上限并限制瞬态水滴总数，避免长时间运行累积。
- 本地存储受限不会导致页面初始化失败。模式切换保持现有内容节点，不重载业务页面；管理/公开路由不挂用户视觉，API、数据库、权限、路由、配置与业务功能不变。
- 发布前实时核验：生产 subnexus-cutover=617ec2d9fb3b5a2344ab9b288f0fd6b76dfb3a48dba579d20612f7223ec20ccf，image=sha256:4f1336bcf711f35b2a752d3896960cf4df4309903718c05d140f92ffedc4a8e4，running/healthy/restart=0；镜像 provenance=3418ed1a599160d4f1e1b29e4da8d32c1871281e，作为本轮 base。上轮 20260909054636-2120213 已 switched，不可重用。本次从新的不可变 commit/tree 构建，所有新 image/Gate/prepare/probe/audit 证据在完成后单独追加。
- UI wrapper 只新增性能模块及三个测试的精确路径校验，不更改 switch/rollback；28 个故障/恢复场景及来源路径校验通过。新 run 的在线备份和证据仍为必需，它们不是新增回滚容器/镜像。此条是准备记录，不表示切换前验收已完成，也没有执行 switch/rollback。

## 2026-09-09（Asia/Shanghai）— 用户端视觉性能分级发布已完成切换前置

- 本轮不可变候选为 commit=`bf5aae07bb30b380cb1be154c49149c9c64cc7f7`、tree=`11ff7c25d42a22d582c12e3166649c4e34e16dfc`、image=`sha256:db1f6f23238301bbecece8b2e5ff3cccdcca8f09aa87099635e5674ba4877577`，归档 SHA256=`f9c5498da435bf4fea09e1a9012b13474faedd3eeee791617b2048500236b705`。线上安装 wrapper=`/srv/subnexus-migration/tools/subnexus-ui-cutover-83d5ec3f-perf-bf5aae07bb30.sh`、SHA256=`83d5ec3f8fc9f0deee1d2a2b25c24a45cf149192046b7c9e0c52054e84dcfdcf`；固定 controller SHA256=`19824a87e3e1de5659cb30664750b71c5c10d374f25bda7f52e6524fe477ee65`。
- 候选 Gate evidence=`/srv/subnexus-migration/docker-candidate/20260909T075323Z-1706be18-1b8e-46d6-b6d3-b1f82a8a9b88/evidence.txt`，SHA256=`a9778206963647d172fdc8fa7e25c2fc173e705107614cc5f7ba34be5b88fd02`；首页 observer evidence=`/srv/subnexus-migration/diagnostics/rain-home-bf5aae07bb30.E901uCy5/evidence.txt`，SHA256=`79dc5ebb9e76c60c4364bf5bdf7a3dff4ec94035b6ef5ac2008ffa9e4006858a`；Gate handoff=`/srv/subnexus-migration/gate-rain-bf5aae07bb30-20260909T075316Z-2188119.env`，SHA256=`e2ee007aab78244a9c78c165e95f2da3938102e34d3042743a7188be52af5af0`。
- 有效 prepare run=`/srv/subnexus-migration/cutover/20260909083917-2245949`，`READY=prepared`、`UI_READY=application-refresh-v1`，manifest `state=prepared/ui_state=prepared/ui_commit_intent=no`，SHA256=`a703c65f9d2eb18b1d035602a2073b8b6ffd1924136b81d9e173aa00052af8ab`。备份为 PostgreSQL `5575725893` B（SHA256=`7b41e4c47bf2269782eb6aca4bc98b9cc5f70019afb562b0bcf57ac3c67e3ca0`）、catalog `118684` B（SHA256=`f6d6c9eabdc024efcd782a54aece374e87c26ab029b75d5d1f66b922de7ca8c1`）、Redis `11682462` B（SHA256=`3cc8e0411e546a30eaed69de1ebf617c9cf7c7cc2fe4f8fac3f0d08e0671c11a`）、应用数据 `77958589` B（SHA256=`62deb6f737a3d1fbd9ff4f0bddb3a1ab84309a6b58d137508d8838b323114280`）。
- stopped/never-started probe evidence=`/srv/subnexus-migration/diagnostics/probe-rain-bf5aae07bb30-20260909083917-2245949.evidence`，SHA256=`4e432ed2f9c209d73c11c20d36496de42b9f5ac4f559dcd56e3defb9bb051cd`；临时容器 ID=`2064f87a3f620a7fb646f762541e68e1e60c0702c7a078867bea8e8605203af7`，创建后从未启动并已删除。最终审计日志=`/srv/subnexus-migration/rain-bf5aae07bb30-final-audit-20260909083917-2245949.log`，SHA256=`b811178f2050e6a1bfed1b2350756b26660a75b53a3bb78e60b881f49eddb01a`，`FINAL_PRE_SWITCH_AUDIT=passed`、`FINAL_SWITCH_EXECUTED=false`。
- 审计时生产 app=`617ec2d9fb3b5a2344ab9b288f0fd6b76dfb3a48dba579d20612f7223ec20ccf`、image=`sha256:4f1336bcf711f35b2a752d3896960cf4df4309903718c05d140f92ffedc4a8e4`，`running/healthy/restart=0`；PostgreSQL 与 Redis 身份、状态和重启次数未变化；公网 health/home 通过，`/srv` 与 Docker 可用空间约 27.5 GB。固定旧 SubNexus 回滚对象仍为 `be459424b327ad056ea9bdc02187d6a458fe09082369b354158d6e7f7758beee` / `sha256:b24b585a35e0eecff497a4eb7a2be480d9a2818f4b7a9780508f2f42cb5e09cd` / `subnexus-cutover-pre-96b66b3e74c1-20260905085804-4072165`，anchor SHA256=`e0b49d89e6a28044c5588afb5a536a3e87ceacfa2a7d679d324d65b14a4c100e`。本轮没有新建永久回滚对象、没有执行 switch/rollback，也没有使用 prune 或删除历史回滚数据。
- 交接命令必须绑定上述 wrapper、controller 和 run，并使用 `SUBNEXUS_DOCKER_TIMEOUT_SECONDS=600`；switch 需要短时生产窗口和无结算任务确认，rollback 使用应用回滚确认。命令已在交接时给出，代理未执行。

## 2026-09-09（Asia/Shanghai）— 撤回旧交接并修复切换前现网回滚合同

- 独立审查确认上条候选使用的 wrapper 会在成功切换后删除切换前 live，正式 rollback 只能恢复更早的固定旧 SubNexus，不满足维护者明确要求的“可回滚为服务器现在的版本”。因此 `bf5aae07bb30b380cb1be154c49149c9c64cc7f7` 对应镜像、归档、Gate、wrapper、observer、handoff、`20260909080410-2227099`、`20260909083917-2245949` 及其所有 switch/rollback 命令均已撤回，禁止复用。应用 UI 改动本身不因此回退，但必须连同修复后的部署证据重新构建。
- `tools/production-deploy/subnexus-ui-cutover.sh` 已恢复切换前 live 一级回滚合同：prepare 记录其完整 container ID、`.Image`、`.Config.Image`、唯一暂存名称和 `prepared` 状态，不停止、重命名或重启它；switch 才停止并重命名，候选健康后继续保留该 stopped 容器；rollback 精确删除候选后恢复该容器到生产名称。它不会被 `docker commit`、转换为额外镜像或在成功 switch 后删除。
- 新目标的 ID、镜像、配置镜像、名称、状态或 runtime contract 任一漂移均失败关闭；候选删除仍要求完整身份匹配。switch 新增 `SIGHUP` 自动恢复，测试覆盖提交窗口、响应丢失、四类中断、目标缺失/漂移和幂等恢复。Git Bash 完整结果为 `46` 个故障/恢复场景及源码范围合同通过，两个脚本 `bash -n` 与 `git diff --check` 通过。
- 历史旧 SubNexus 容器及 anchor 继续保留，并在 prepare/switch 阶段作为连续性门禁；它不再替代切换前 live 承担本轮正常 rollback。当前仅完成本地修复，尚未形成新的不可变提交、镜像、Gate、prepare、probe 或最终审计；线上仍运行原容器，未执行 stop/rename/restart/switch/rollback，当前没有有效切换命令。

## 2026-09-09（Asia/Shanghai）上游 v0.2.4 本地合并

- 维护者本轮要求对齐 `https://github.com/Wei-Shaw/sub2api.git` 最新版本。同步目标固定为 `upstream/main=98d86915becae9fe9491a91ffc6defd5235c8d2b`，VERSION=`0.2.4`，标签 `v0.2.4=d681d0798064ee0ffff376d19687d12f09fe600f`。
- 代码合并提交=`c76c04dd170c6eb4d34f864150c8e03536f38c24`，tree=`4ed29392a38985a10a4a6ae7f04d94d04374dbc4`；第一父提交=`bf5aae07bb30b380cb1be154c49149c9c64cc7f7`，第二父提交为上述上游 SHA。预合并本地安全分支 `backup/pre-upstream-sync-20260909-173148` 只用于 Git 代码恢复，不是线上回滚对象。
- 使用 `git merge --no-commit --no-ff -X ours upstream/main` 后，对无策略 merge-tree 指出的7个冲突文件逐一复核。补齐 `ChannelMonitorHideUserRanking` 公共设置 key/read、service/DTO、handler 与首页注入，恢复后台 V2/V3 吞吐量 Toggle 和 V2 排行 Toggle；不改变本项目默认关闭和 V3 的业务约定。插件保留已有等效的 Windows ZIP 句柄修复及关闭错误处理。
- 保留 F01-F13、Rain 首页/用户端视觉模式、下拉层级、签到文案，以及所有 `9001`-`9013` SQL。纳入上游模型白名单、MiniMax、Astra/Image 2.5、网关、支付、代理等更新；已执行的历史迁移没有改名、删除或修改。
- 前端验证：`pnpm run typecheck`、`pnpm run lint:check`、`pnpm run build` 通过；完整 Vitest `311/311` 文件、`2174/2174` 测试通过。监控修复新增 V2/V3 操作回归后，定向 3 文件/45 测试和再次 lint/build 通过。适配 provider 数量与 GroupsView 测试的 auth store、模型白名单 API 名称；不通过删除用例规避失败。
- 后端验证：默认 Go 包测试全部通过（首次 repository 因 Windows PATH 缺 sh 失败，加入 Git Bash 后该包复跑通过）；unit 标签全包中 Ent schema 曾在并行生成时读取临时文件，生成结束后该包复跑通过。service 缺公共设置字段的真实合并问题修复后，`go test -tags=unit ./internal/service ./internal/handler ./internal/handler/dto ./internal/server -count=1 -p=1` 全过，其余 unit 包原轮次均通过。`go vet ./...`、受修改包 vet、`go build -tags embed ./...` 通过；Wire 与 Ent 重生成无代码漂移。
- 依赖：上游 package.json 本轮只新增 i18n 构建检查，无依赖版本变化；pnpm 9.15.9 冻结离线 lockfile 校验通过。上游 Astra 指令文件只有11处行尾空白清理，无内容语义变更。部署脚本工作区测试49个故障/恢复场景通过，未纳入本次合并。
- 共享工作区还有原迁移任务在写入，已发送协调消息并保留其原有6文档/2部署脚本改动；这些文件未进入代码合并提交。本次上下文、台账和记忆追加也保留为未提交文档改动。
- 本轮没有 push、访问服务器、数据库 apply/restore、开关开启、镜像部署或 switch/rollback。后续 `0.2.4` 发布必须重新执行完整 Release Gate：`235/236` 将 `groups.models_list_config` 重命名为 `model_allowlist`，旧版同库回滚兼容性需要单独验证；不能沿用仅 UI 更新的脚本范围和历史运行证据。本轮未运行生产备份隔离迁移/旧版回归，也未生成新的上线命令。

## 2026-09-09（Asia/Shanghai）— UI 回滚候选身份证明补强

- 独立复审发现：当 previous-live 已被外部恢复到 `production-app` 名称，而候选仍被外部改名保留时，`ui_manual_rollback` 不能仅凭生产名判断回滚已完成。现行 wrapper 在该分支及自动恢复成功前，均通过 manifest 绑定的候选完整 container ID 精确确认候选不存在；候选仍存在、ID/名称/意图元数据不完整或无法证明时失败关闭，保留 previous-live 和候选现场，不写成功 marker。
- 自动恢复在 `ui_remove_candidate` 后增加同样的 absent proof，覆盖“候选 ID 文件丢失但声明已写、候选被改为非生产名”的故障。只有明确证明候选从未创建的状态才允许无 ID：`candidate_container_id`、`candidate_container_name`、`candidate_container_intent` 均为空、ID 文件不存在、`ui_commit_intent=no`，且状态处于 `switching`、`rolling_back` 或已完成的 `recovered_current`；任何部分元数据或已提交状态都不放行。
- 测试新增 `rollback_already_restored_candidate_present` 与 `recovery_candidate_identity_lost_detached`，并复跑 Git Bash 完整 `56` 个故障/恢复场景及源码范围合同；结果为 `UI cutover tests passed: 56 fault/recovery cases and source-contract checks`，两个脚本 `bash -n`、`git diff --check` 均通过。Windows Git Bash 的 `.config/git/ignore` 权限警告为环境噪声，不影响退出码 `0`。
- 本轮只修改 `tools/production-deploy/subnexus-ui-cutover.sh`、其测试和本记忆/台账文本；未 `git add`、commit、push，未访问或修改线上服务器、Docker、PostgreSQL、Redis、Nginx、功能开关或 `F:\Sub2Api\SubNexus`。当前仍不能切换：主流程还需完成父任务的 v0.2.4 数据库兼容 Gate、不可变镜像/归档、全新线上备份、无停机 prepare、never-started probe 和最终审计。

## 2026-09-09（Asia/Shanghai）— 历史 anchor 与一级恢复解耦

- 二次独立审查指出：历史旧 SubNexus anchor 若同时作为 switch 后恢复门禁，会在该二级证据缺失或漂移时阻止恢复切换前现网版本，违反“用户数据优先、previous-live 可回滚”的一级目标。现行合同因此限定为：anchor 只在 `prepare` 和 `switch` 提交前做连续性校验；一旦 switch 已开始，恢复路径只依赖本轮 manifest 严格绑定的 previous-live。
- `tools/production-deploy/subnexus-ui-cutover.sh` 的 `ui_load_run` 仅在 `scope=switch` 时检查 anchor 路径、状态和哈希；`ui_manual_rollback` 与 `ui_recover_entry` 不再调用 anchor gate。`ui_switch` 仍在停机前及候选提交前两次完整校验 anchor；第二次校验失败以及 `ERR/HUP/INT/TERM` 仍直接自动恢复 previous-live。
- 测试将 `recover_anchor_missing`、`recover_anchor_drift` 和 `rollback_anchor_missing` 改为必须成功恢复，并新增 `rollback_anchor_drift`。每个场景都验证 production 名称/运行状态、候选清理、终态 marker、previous-live 身份、设置保持及未操作历史容器；切换前 `anchor_missing`/`anchor_drift` 仍失败关闭，`anchor_lost_during_switch` 仍自动恢复。
- Git Bash 完整结果为 `50` 个故障/恢复场景及来源合同检查通过；两个脚本 `bash -n`、聚焦 7 场景和 `git diff --check` 通过。仅修改既有 6 个上下文/手册/台账文件与 2 个 UI 部署脚本；未 `git add`、commit、push，未访问服务器、Docker、数据库或线上用户。旧 `bf5aae07...` 制品和命令继续作废，必须待新提交及完整 v0.2.4 数据库兼容 Gate 后重新生成发布证据。

## 2026-09-09（Asia/Shanghai）— 最终候选本地门禁收尾（尚未发布）

- 当前工作树仍位于 `feature/subnexus-migration`，基线 HEAD=`c76c04dd170c6eb4d34f864150c8e03536f38c24`；本次未修改 `main` 或只读源项目 `F:\Sub2Api\SubNexus`，也未访问线上服务器、生产数据库或线上 Docker。
- 本轮待提交改动包括 `234a_group_model_allowlist_legacy_compat.sql`、迁移 runner 合同校验、旧/新列双向同步与冲突 fail-closed 集成夹具，以及 UI retained-live 回滚 wrapper/56 场景测试和记忆文档。`.codex-go-cache*` 仅为本地测试缓存，不得提交或上传。
- 本地验证结果：Git Bash 后端 `go test ./...` 通过；兼容迁移定向测试和 `migrations` 包通过；UI cutover 56 个故障/恢复场景通过；production cutover、Docker candidate、readonly preflight、Redis restore-check 四套脚本夹具通过；isolated image-build 静态合同通过。Windows Git Bash 的 Python 动态夹具因系统 `python3` 为 AppInstaller redirector 而按测试设计跳过，必须在 Linux/WSL 真实解释器环境复跑。
- 隔离 WSL `SubNexusBuild20260904` 当前代码副本仍为旧提交 `187b128bd32d1e06ad6e08817632e7c6b5ccca92`，专用 Docker socket=`unix:///var/run/subnexus-docker-96.sock`，daemon=`ab82cd35-2345-44b4-8709-bdcbd49b22ca`，当前 0 容器/0 卷/0 自定义网络且五个固定基础镜像存在；历史候选镜像保留，禁止 prune。最终提交后必须先同步代码，再跑真实 PostgreSQL/旧版同库回归和不可变镜像构建。
- 当前阶段结论：尚不可切换。必须依次完成提交/push、隔离 Linux PostgreSQL 兼容矩阵、不可变镜像与归档、线上只读候选 Gate、全新备份、无停机 `prepare`、never-started probe 和 `FINAL_PRE_SWITCH_AUDIT=passed`；只有这些证据绑定同一 run 且 manifest=`state=prepared/ui_state=prepared/ui_commit_intent=no` 后，才生成并交付唯一的手动 `switch`/`rollback` 命令。


## 2026-09-09 23:00（Asia/Shanghai）— v0.2.4 发布前实际状态（尚不可切换）

- 本轮授权：代理完成全部发布前置，维护者手动执行最终 switch/rollback；创建以切换前实际 live 为目标的新回滚合同。历史旧 SubNexus 保留为二级资料，不替代本轮 previous-live。
- 当前线上已实际运行旧性能候选 bf5aae07bb30b380cb1be154c49149c9c64cc7f7：container=b9de08a4f4134a1a486bfc68537344af564b7f337e2117209a75a9cb3c2fb9f0，image=sha256:db1f6f23238301bbecece8b2e5ff3cccdcca8f09aa87099635e5674ba4877577，StartedAt=2026-09-09T09:22:17.129414244Z，healthy/restart=0。此前“旧候选未发布”的文档已被本次实时证据覆盖。PG/Redis 完整 ID 与 2026-09-07 记录相同、restart=0。
- 不可变应用候选 commit=890828afe0f726abb363029f147e04087fed2bca，tree=9fc99f6dac3e8f2028b8f2178ec7ca6ae480d425；上游基线仍为 0.2.4/98d86915becae9fe9491a91ffc6defd5235c8d2b。后续部署工具提交 7efd247b5f8629ada049b16e89c1b745bac4a23b 已推送，不改变镜像应用 SHA。
- Linux 真 PostgreSQL 定向 integration 共 8 项通过（234a 双向写入、冲突拒绝、缺失列和 236 修复）；专用本地 Docker daemon ab82cd35-2345-44b4-8709-bdcbd49b22ca 的测试容器已自动清理。新完整发布 wrapper 原样保留 UI-only/controller，额外加入 Gate、live base、target/tree 绑定和候选创建前 stopped 回滚合同；Linux 56 个故障/恢复场景与真实 Gate/parser/source 测试通过。
- 镜像 image=sha256:44e8dcf019338e050756c86aba8d2ecf73390b4d058da2ebf916aba19bea28d9；archive SHA=d09ced7abf8dbdc5e2eeb41bf61dcdf31cf7d0463cfeea03fa17bc2728d4e87e，size=48681984。构建/归档/安装均通过；服务器 source=/srv/subnexus-migration/source-full-890828afe0f7，artifact=/srv/subnexus-migration/candidate-artifacts/full-890828afe0f7。本地镜像和归档位于 candidate-transfer/rain-890828afe0f7，历史 rain 目录名不表示本次为 UI-only。
- 新 wrapper=/srv/subnexus-migration/tools/subnexus-full-release-cutover-737f8603-full-890828afe0f7.sh，SHA=737f86031c94c27bf8b6cbe81150e89c8c5855f76e3ac34be1fe22ef7dd460ac；UI library=/srv/subnexus-migration/tools/subnexus-ui-cutover-8fdfc825-full-890828afe0f7.sh，SHA=8fdfc8253020d61c1e5f0b13b49c340295e0713f552763c66603d32ae949e38d；历史 controller SHA=19824a87e3e1de5659cb30664750b71c5c10d374f25bda7f52e6524fe477ee65。
- 服务器空库 Docker Gate 已通过：/srv/subnexus-migration/docker-candidate/20260909T140121Z-fe2e5421-95e2-4796-bf87-1a07e947c95a/evidence.txt，SHA=0a226b5649bf2353c746c70ef348e6e40986d5cc4b55bd78553524b41d02fba3。该 Gate 不能替代生产数据升级/旧版同库回滚验证。
- 全新生产快照=/srv/subnexus-migration/full-release-compat/production-20260909T134428Z.dump，bytes=5600173919，SHA=321231ffa421029cac73b90fcba65eb5942228b27f577d3cdb869c1dcaf57265，catalog 验证通过；生产数据库约 83.5 GB、375 条 migration。完整副本正在下载，早期中断副本和预分配文件不可作通过证据，必须最终完整 SHA 匹配。恢复仅限本地独立 Docker/internal 网络，不启动连接生产库的候选。
- 为满足新备份空间预算，精确清理失效 prepared run 20260909080410-2227099 的备份文件：该 run 无 candidate、live 已被后续 switched run 替代；删除前重算 PostgreSQL SHA 与 manifest/sidecar 一致。manifest 和审计保留，cleanup=/srv/subnexus-migration/cleanup-full-release-stale-20260909080410.txt，SHA=ad5bffd30e975d8a698ea714a1bf1bd44691ede91fd490493a8604e519635fe7。其余历史/当前回滚对象与备份保留，未 prune。服务器约 26.26 GB 空闲。
- 待完成：完整下载与本地恢复；新版→旧 live 镜像→新版同库 API/写入验证；正式 full prepare（新 PG/catalog/Redis/app 备份及 previous-live 身份）；never-started probe；最终审计；更新最终手册命令。尚未执行 switch/rollback/生产迁移，不得把安装或空库 Gate 通过当作全部前置完成。

## 2026-09-09 23:50（Asia/Shanghai）— 完整备份已校验，隔离恢复进行中

- 5,600,173,919 字节生产快照已完整下载至 `F:\MySub2\production-backups\v024-20260909\production.dump`，下载进程整文件 SHA256 与原快照 `321231ffa421029cac73b90fcba65eb5942228b27f577d3cdb869c1dcaf57265` 一致；WSL 入口亦重新校验通过。下载会话已正常退出，后续不能以预分配文件长度判断下载状态。
- 完整恢复使用本地专用 daemon `ab82cd35-2345-44b4-8709-bdcbd49b22ca`、internal 网络和独立卷，token=`compat-6b1a369a45a54206`，证据/私有诊断目录=`/work/full-release-compat/compat-6b1a369a45a54206`。正在恢复数据，不表示兼容 Gate 已通过；没有向生产库写入或启动连接生产库的候选。
- 容量复核：生产表数据约 40.18 GB、全部索引约 43.32 GB，ops_system_logs 表数据约 27.31 GB。为避免辅助盘拥挤，恢复分为 pre-data/data、ops 索引、剩余 post-data：只把 ops 表及其 12 个索引放入 D 盘辅助 tablespace，其余表和索引在 F 盘。固定快照的 ops TOC ID 为 `5407..5417,5419`，两列表互斥且覆盖原 TOC；保留原顺序并在最后验证索引位置/valid/ready。独立复核和 AST 检查通过。
- 当前实际执行的恢复 harness=`F:\MySub2\tools\release-v024-20260909\compat-local.py`，SHA256=`1b5b2087fe5cc982c05ca0078ecc4641d1a331587cfae3118cfcc5956dfc606b`。仅为加快可丢弃测试副本恢复，在该 token 的 PostgreSQL reload 了 fsync/full_page_writes/synchronous_commit=off、checkpoint_timeout=1800、max_wal_size=8192 MB、maintenance_work_mem=524288 KB；记录在该 token 的 `restore-performance.env`。这不测试持久化断电恢复，也不修改生产 PostgreSQL 参数。
- D 盘测试磁盘 `D:\SubNexusRelease\compat-890828afe0f7.ext4` 当前 70 GiB，挂载在 `/work/compat-storage-890828afe0f7`。此前直接删除被自动审批策略拒绝；测试结束须先卸载，若无法由工具清理，明确交给维护者手动删除，不能留下未说明的空间占用。不得在恢复中删除或重新格式化。
- 无切换的最终审计 helper 已安装：`/srv/subnexus-migration/tools/finalize-prepared-3953fbf0-890828afe0f7.sh`，SHA256=`3953fbf04f6bc4c5ae33a7971246ef3f7869578c3761a90d6db7a93c9c070d5e`，调用已审查的 never-started probe 并要求退出码、临时清理、公网健康和 prepared 身份均通过。正式 prepare helper=`/srv/subnexus-migration/tools/run-full-prepare-83479d3a-890828afe0f7.sh`，SHA256=`83479d3a78f679aa3be2e62292a935e622323cfa153d3732b24e357cca652098`，尚未执行。
- 兼容证据安装 helper=`/srv/subnexus-migration/tools/install-compat-evidence-1ff50cfd-890828afe0f7.py`，SHA256=`1ff50cfd65f958cf1e42af37ae09b0e451ef0c89dc662e507deeab550b7b03c1`。必须在真实 full Gate 成功后上传 evidence/checks 才可执行；同时恢复原快照 root:root/600 并删除上传目录中的同 inode 硬链接，不删除原快照。
- 生产 health 正常，`main` 仍为 `d596d0844f274c3e7933c966231851f9f20b0d47`。最终 prepare/probe/正式发布命令仍待完成，尚不可切换。

## 2026-09-10 00:15（Asia/Shanghai）— 全部数据已恢复，调整本地索引恢复存储

- 原完整恢复已于 `2026-09-09T16:01:07Z` 成功完成 pre-data 和全部 data（含约 4,400 万系统日志、约 1,270 万用量记录），进入 ops 索引阶段。D 盘 loop/NTFS 挂载层读盘过慢，首次主键扫描十余分钟仍未完成，尚无已提交 ops 索引；该瓶颈只发生在本地测试环境。
- 使用 `F:\MySub2\tools\release-v024-20260909\finish-restored-snapshot.py` 完成受控接续：核验原始备份 SHA、专用 daemon、唯一带 token 的 PG、internal 网络、卷、375 条原迁移账本及原恢复进程；先冻结原 Python runner，再终止它的本地 pg_restore 会话，最后结束原 runner，确保其自动 cleanup 不会删除已恢复数据。旧等待会话的 SIGKILL/退出码 1 是此接续的预期结果，不能重新启动旧全量恢复。
- 原已恢复数据保留在同一 token `compat-6b1a369a45a54206` / PG `ceb084f4506c...`；接续状态文件为 `/work/full-release-compat/compat-6b1a369a45a54206/native-storage-finish.json`。当前用标准 `ALTER TABLE ... SET TABLESPACE pg_default` 把 ops 表移回原生 Linux 存储；所有 post-data 索引随后放 D 盘 `compat_logs`，避免重复从跨系统挂载读取大表。继续维持每处至少 10 GiB 的空间门禁。
- 接续 helper 完成所有 post-data、索引 valid/ready/location 和原账本校验后，才生成原 harness 可验证的私有 resume checkpoint，并运行原 new-old-new API 验证。最终 full compatibility evidence 尚未生成，正式 prepare/probe 和线上切换均尚未执行。

## 2026-09-10 01:38（Asia/Shanghai）— v0.2.4 全部线上前置完成

- 全部生产数据、索引、约束恢复成功，原 375 条迁移记录保持一致；完整副本新版/重启/旧版/新版均健康，登录、分组、密钥、订阅、新旧模型配置写入、MiniMax 旧版读取和冲突拒绝均通过。实际 harness evidence=`/work/full-release-compat/compat-6b1a369a45a54206/evidence.env`，SHA=`0485384ea5ea8daaa020eba25d5d4a0fe1f232f0740cfd54ee1575bcda1e93c8`，已安装在 `/srv/subnexus-migration/docker-candidate/full-890828afe0f7-compat-6b1a369a45a54206/evidence.env`。测试容器、具名卷、internal 网络和 token tablespace 子目录已清理。
- 正式 full prepare run=`/srv/subnexus-migration/cutover/20260909171117-2439922`，prepared_at=`2026-09-09T17:25:13+00:00`；READY/UI_READY/FULL_READY 均有效，manifest SHA=`7c3559f16226abe2cbd1634a12e97a194b23d7eb06a489b5305677bf256dcfdf`，`state=prepared/ui_state=prepared/ui_commit_intent=no`，无候选容器 ID。
- 全新备份：PostgreSQL `5615798623` B，SHA=`00d13196aacc45f4f05c543c4e59d93f0619c9e7ceca4c0a1476450ea8fff738`；catalog `118684` B，SHA=`356eddff3c5111eeeeb63f00f0cd581a1cd1b7c81c6ed8f5ef423403cfc891221`；Redis `10095634` B，SHA=`eb3c389618d15fca32e82dad6ace71ba17eaa450f759e9b40768646f919c92bba`；应用数据 `75862137` B，SHA=`bc3909570e4feb51679b686fa1fba071be3472aef3594cf43b4424cc08b733557`。全部 sidecar/manifest 哈希匹配，服务器可用空间 `20699664384` B。
- 新一级回滚目标：实际 live `b9de08a4f4134a1a486bfc68537344af564b7f337e2117209a75a9cb3c2fb9f0` / `sha256:db1f6f23238301bbecece8b2e5ff3cccdcca8f09aa87099635e5674ba4877577`，唯一暂存名=`subnexus-cutover-ui-prior-20260909171117-2439922`。prepare 只固定此身份；用户 switch 时才停止改名并保留，rollback 恢复同一对象，不默认恢复数据库。历史旧 SubNexus anchor 继续保留。
- 最终 probe=`c67656715af0227182e82471bfa1d2c4261fecb6cfbfba3e71f3e3709705d734`，始终 `created/false/restart=0/StartedAt=0001-01-01T00:00:00Z`，运行合同 SHA=`7dc88dd8f76be1a69c6d4f322deb1b1e0eda8be94be61d37cac850091578453d`，已精确删除。最终 evidence=`/srv/subnexus-migration/diagnostics/full-890828afe0f7-final-20260909171117-2439922.evidence`，SHA=`e448ccc554b7f088c306f768c4a2ac0372c397e0abe5b9382f39ecfbcbdf40e0`；facts 同名 `.json` SHA=`36462811ffc980329321b41da1a2265f71ad5583e6e648accf1aa4a3e5bc80fe`。退出码 0，`FINAL_PRE_SWITCH_AUDIT=passed`、`FINAL_METADATA_AUDIT=passed`、`PROBE_EXIT_AND_CLEANUP=passed`、`FINAL_SWITCH_EXECUTED=false`。
- 首次 Python urllib 公网请求收到 403，probe 本身通过但完整审计退出 1，没有切换；随后 curl 检查的本地 health、公网 health、首页实际全部 HTTP 200。最终执行 helper=`finalize-prepared-563826e0-890828afe0f7.sh`，SHA=`563826e0ab8e04a5c626157dec9c010fd1ca0d490d5aa1df1e5fecf8ecf8f1d9`。其 accepted statuses 包含 403，但本次事实记录确实全部为 200；本地 helper 已收紧为必须 200。旧失败审计文本已清理。
- 生产 app 仍为原 b9de08a4...，StartedAt=`2026-09-09T09:22:17.129414244Z`，healthy/restart=0；PG/Redis 身份和启动时间保持原值。没有执行生产 switch/rollback/migration 或修改设置。项目唯一当前交接为切换手册第 15.3 节完整单行 switch/rollback。
- 本地临时测试磁盘 `D:\SubNexusRelease\compat-890828afe0f7.ext4` 已验证无数据子目录并卸载，仍占 70 GiB。此前自动审批拒绝删除；最终须告知维护者手动删除。专用 WSL 已 fstrim，磁盘镜像是否释放 Windows 物理空间需后续确认。

## 2026-09-11（Asia/Shanghai）— 分组模型表现监控本地实现

- 维护者批准实现定时鹈鹕 HTML 监控，并补充每条任务的请求模型配置；明确用户下拉和历史遵守现有分组权限。实现规划及操作说明：`docs/model-evaluation-monitor.md`。
- 新增独立 `subnexus_model_evaluation_enabled`（默认 false）；用户 `/model-evaluations`、管理员 `/admin/model-evaluations`。每任务可配置现有分组、完整 HTTPS URL、加密 Key、请求模型、chat_completions/responses/messages 协议、启停、执行间隔和清理保留策略。原渠道监控模式、网关、计费、认证和 Rain 用户视觉模式均保持现有实现。
- 固定提示词严格为“生成html，内容是svg绘制鹈鹕骑自行车2D动画，不用进行测试。”，模型原值发送。支持从围栏或附说明的响应提取唯一完整 HTML，内部原始字节保留；失败、截断、多文档、敏感凭据回显和超限响应不存为成功输出。更改分组、地址或协议须重填 Key，单改模型等其他字段可保留原 Key。
- 追加 `9014_subnexus_model_evaluations.sql`，仅新增任务、结果和容量租约表以及默认关闭设置；无历史 SQL 修改。数据库短事务与租约限制全局两请求，修订号避免关闭、编辑、删除、清理、租约失效后的过期结果写回；不在 HTTP 期间持有数据库事务。单次超时 180 秒，任务最多 100；HTML 512 KiB、响应 2 MiB，每任务最多 200 条，默认间隔 3600 秒、7 天、50 条。
- 用户列表及详情逐次使用 APIKeyService.GetAvailableGroups，仓储再校验分组/任务/开关；用户不接收 URL 或 Key。后台可单条删历史、删任务、按任务或全部应用保留策略/清空；自动保留覆盖成功及失败记录，软删除分组的任务仍可在后台清理。
- 页面每页 12 条元数据，HTML 详情按需加载；最多 2 个动画，悬停或点击/触屏放大时只运行 1 个，隐藏/离屏/关闭时卸载。无同源权限 iframe、CSP、静态资源清理和 credentialless 隔离主站；真实 Chromium 已验证 SVG/CSS/JS 运行、父页面/存储隔离、外部子资源/请求及顶层跳转阻断。任意内联 JS 的子框架自身导航不能被 sandbox 全面禁止，浏览器边界及外部依赖显示限制已写入规划，不宣称绝对网络隔离。
- 本地验证：前端最终完整 Vitest `316/316` 文件、`2203/2203` 测试、零未处理错误（maxWorkers=4/minWorkers=2）；全量 ESLint 通过，Key 绑定提示补充后的定向 ESLint/8 项测试通过。先前高并行复跑遇到既有 AccountsView.selectAllResults 的异步 mock 报错，未修改该业务/测试文件，单独和最终完整复跑均通过。
- 后端默认 `go test ./...` 除启动 cleanup 测试漏加新服务参数外均通过；补齐该测试且验证新服务未 Start 时可安全 Stop 后，`go test ./cmd/server -count=1`、`go vet ./...`、`go build -tags embed ./...` 通过。后续 HTML 提取/Key 绑定改进的 TestModelEvaluation 定向测试通过；真实 PostgreSQL 16/18 隔离测试通过并发领取、两槽限制、权限、保留、清理/关闭/编辑/超时回写隔离及数量上限。首次 PostgreSQL 参数类型歧义已修复并复验。
- 浏览器页面验收以本地模拟数据完成：1440×1000/390×844、明暗主题、分组筛选/分页、悬停/触屏预览、最多两个动画、编辑模型保留 Key、打开清理不执行删除，页面错误 0、横向溢出 0。证据 `F:\MySub2\.playwright-qa\model-evaluation-20260911\report.json`；图片为测试夹具，不代表实际模型质量。
- 工作区仍为 feature/subnexus-migration；未提交/push、未访问生产服务器、未开启线上功能、未执行 switch/rollback。原有五份部署文档修改保持保留。本次新功能不在先前 890828afe... 的 v0.2.4 制品中，不能用旧切换命令宣称发布本功能；正式上线须从本次代码重新构建并完成发布门禁。
- 两个新增隔离 PostgreSQL 实例和浏览器预览服务均已停止；目录 `F:\MySub2\.model-evaluation-pg-20260911`、`F:\MySub2\.model-evaluation-pg-test-20260911` 保留。后者清理曾被自动审批以 blocked by policy 拒绝，未更换方式重试；这些目录和测试缓存不进入 Git，不是生产回滚对象。
- 最终收尾复验：包含最后 HTML 提取、Key 绑定提示和分组标签样式修改的当前工作树，`pnpm run build`（i18n 检查、vue-tsc、Vite）退出 0；随后 `go vet ./...` 和 `go build -tags embed ./...` 均退出 0，嵌入当前前端产物。`git diff --check` 通过；依赖及锁文件、历史迁移没有修改。前端最终测试和构建日志分别为 `F:\MySub2\.playwright-qa\model-evaluation-frontend-verified.log`、`F:\MySub2\.playwright-qa\model-evaluation-build-verified.log`。尚未调用真实模型供应商，模型请求以测试传输和浏览器夹具验证。

## 2026-09-11 分组模型表现监控发布前置（进行中，尚不可切换）

- 用户授权完成全部线上前置，创建新的回滚镜像，最终切换仍由用户手动执行。实际 live 已为 v0.2.4：`e389b3b1c4f62fd8d9fb0eb04998b0559d21bf39ae4c1a99bdfc90441def3836` / image `sha256:44e8dcf019338e050756c86aba8d2ecf73390b4d058da2ebf916aba19bea28d9`，StartedAt=`2026-09-10T00:38:47.787671032Z`，healthy/restart=0。旧手册 15.3 run 已 switched，禁止再次执行其 switch。本轮无生产停止、重启、切换或迁移。
- 新候选 commit=`33a9601c93304f89a67b7845b4e3b10447edcf96`、tree=`efb0d3a6e9089ecb7eea188f91e5299a3694b8d6` 已推送；image=`sha256:9342118b00d127deb7fdc1c62fe19a347acf5563a254a3e9541cf309f492506f`；归档 `48738304` bytes / SHA=`cbda8db4b80edf5d9f5f106d26649b8226b6793d8578d92e3bc2b7140c708d5c`。source=`/srv/subnexus-migration/source-full-33a9601c9330`、artifact=`/srv/subnexus-migration/candidate-artifacts/full-33a9601c9330`。本地构建两次网络超时后，确认重启后本地 daemon 的 iptables=false 导致无 NAT，改为专用本地 daemon 管理构建网络规则后构建通过，生产 Docker 未改。
- 新 full wrapper 把 base 固定为实际 890828afe...，原 UI library/controller 字节和恢复流程不变；56 场景与 Gate/source 校验通过。安装 full=`subnexus-full-release-cutover-7bb384a1-full-33a9601c9330.sh`，SHA=`7bb384a1700c5c24a90c855f8f444fd6ad01fcae3a21b31b01f4ad3d81cf5c9b`；UI=`subnexus-ui-cutover-8fdfc825-full-33a9601c9330.sh`。服务器候选 Gate=`/srv/subnexus-migration/docker-candidate/20260911T105404Z-8b297a5e-c2fe-4339-a49e-73ed2531893f/evidence.txt`，SHA=`9f303367535e90a7428ec5a017f143958e298d8735b3e2996521ff886199472c`，passed，生产运行身份不变。
- 新回滚 tag=`subnexus-rollback:model-evaluation-20260911-890828afe0f7`，绑定实际 live image；归档 `/srv/subnexus-migration/model-evaluation-20260911/rollback-image.tar`，48681984 bytes / SHA=`a33a982c244bbc12d5b62a8200fc408305cc82186e64ec24a3d663f9a4935fb3`。未 docker commit；归档完整验证 OCI manifest/config/layers。正常回滚仍使用本次 prepare 绑定并在用户 switch 时保留的 actual-live 容器，归档为补充灾备。
- 全新完整快照 `/srv/subnexus-migration/model-evaluation-20260911/production.dump`，5742805036 bytes / SHA=`c0e8a98e1104bfddd9a048012537e822462d941cbe762410466b68ff62f3ffe4`，catalog/完整哈希通过；379 条基线 migration，无 9014 且新开关缺失/false。基线账本 SHA=`b46cd2b9ee34283118f39888863e8f564147ba52153f394856d031b265dafccd`。本地下载至 `D:\SubNexusRelease\model-evaluation-20260911` 正在进行，单连接下载主动停止后转为分段续传，必须整文件哈希通过后才用于恢复。
- 空间不足时仅删除 3 份已逐一核验 SHA 的失效冗余快照：旧 run `20260906134705-774592`、`20260909054636-2120213` 的 postgresql.dump（两者 live/candidate 已不存在），以及前次独立兼容快照 `production-20260909T134428Z.dump`。保留其 sidecar/manifest/审计、最新 v0.2.4 备份、固定旧 SubNexus anchor、本次快照和所有回滚镜像。释放 16348982364 bytes，审计 `/srv/subnexus-migration/cleanup-model-evaluation-expired-snapshots-20260911.json` / SHA=`57a14eb0fa33b08444389d2237806bd7f6aef4bcfc45044bbc4ccbf30e50d3a5`。
- 本地专用 WSL 在无运行容器时完成离线虚拟磁盘压缩，F 盘从约 19 GiB 可用恢复至约 64 GiB；仅精确删除四个前次本地测试遗留且无引用的小型匿名卷。先前删除被拒绝的 70 GiB 辅助 ext4 文件没有删除/重格式化，确认内部 data 为空后重新挂载，用于本轮隔离索引恢复。新本地兼容 helper 位于 `tools/release-model-evaluation-20260911/compat-local.py`（仓库外），全量数据原生卷、索引辅助盘，保持每盘 10 GiB 余量；尚待实际运行。
- 待完成：完整快照下载校验、全量隔离 new/restart/old/new 与新功能数据保留验证、兼容 Gate 安装、正式 prepare 新备份、never-started probe 与最终审计。当前不能交付切换命令；旧 15.3 命令不用于本次功能。

## 2026-09-11 21:07（Asia/Shanghai）— 完整兼容验证通过，正式准备中

- 固定候选仍为 `33a9601c93304f89a67b7845b4e3b10447edcf96` / image `sha256:9342118b00d127deb7fdc1c62fe19a347acf5563a254a3e9541cf309f492506f`；完整快照 `5742805036` bytes / SHA=`c0e8a98e1104bfddd9a048012537e822462d941cbe762410466b68ff62f3ffe4` 已下载并完成全量恢复、索引及约束校验。
- `compat-8b63adeb67e24bd7` 的新版/重启/当前线上旧版/新版共 10 项验证全部通过：登录、分组、密钥、订阅和模型配置写入正常；新增 9014 外的 379 条 migration filename/checksum 不变；开关 false、两个容量槽空闲；新任务的模型、加密 Key 及原始 HTML 在往返版本间保留。测试只访问隔离副本，没有调用供应商。
- 兼容证据已安装：`/srv/subnexus-migration/docker-candidate/full-33a9601c9330-compat-8b63adeb67e24bd7/evidence.env`，SHA=`a813fccfe95616d3dc19755bb89cf14b901d3bc489688192a1e90c6beada729d`，`result=passed/cleanup=passed`。本地副本见 `F:\MySub2\tools\release-model-evaluation-20260911\compat-8b63adeb67e24bd7.env` 及 `.checks.json`。
- 所有本地兼容测试容器、卷和网络已清理，辅助测试盘确认 data 子目录为空后已卸载。此前因策略拒绝删除的辅助 ext4 文件仍保留，未删除或重新格式化；专用构建 daemon 已停止，并回收其虚拟磁盘空闲空间。
- 21:06 再次只读核验：生产 app/PG/Redis 身份及 StartedAt 保持原值，379 条迁移账本 SHA 不变，无执行中的结算任务或 schema DDL，新功能仍缺失/false。正式 prepare helper=`/srv/subnexus-migration/tools/run-full-prepare-1e94cdc8-33a9601c9330.sh`，SHA=`1e94cdc8e833f500dfa8dbc9781fb86f3227b48be2377819038d3636a7875180`（Docker timeout 1800 秒的已安装修正版，不再使用最早 helper 清单中的 aedffb6d）。准备日志=`/srv/subnexus-migration/diagnostics/model-evaluation-33a9601c9330-prepare.log`，退出码同名 `.exit`。
- 尚待正式 prepare 退出 0、never-started probe 与最终审计；尚不可切换，未执行任何生产 switch/rollback/migration。

## 2026-09-11 21:26（Asia/Shanghai）— 分组模型表现监控全部前置完成

- 候选 commit/tree=`33a9601c93304f89a67b7845b4e3b10447edcf96` / `efb0d3a6e9089ecb7eea188f91e5299a3694b8d6`，image=`sha256:9342118b00d127deb7fdc1c62fe19a347acf5563a254a3e9541cf309f492506f`；应用源码与镜像仍绑定此提交，后续文档提交不改变候选。服务器正式 prepare 和最终审计均退出 0。
- 正式 run=`/srv/subnexus-migration/cutover/20260911130706-3232923`，prepared_at=`2026-09-11T13:21:27+00:00`，manifest SHA=`8436989b7f3ca1b5c338429a52c3493dae26c688dfed8c71a04a57b31ec63b10`，READY/UI_READY/FULL_READY 均通过，`state=prepared/ui_state=prepared/ui_commit_intent=no`；无候选 ID、SWITCHED 或 ROLLED_BACK 标记。
- 完整快照兼容 Gate=`/srv/subnexus-migration/docker-candidate/full-33a9601c9330-compat-8b63adeb67e24bd7/evidence.env`，SHA=`a813fccfe95616d3dc19755bb89cf14b901d3bc489688192a1e90c6beada729d`；10 项完整验证和 cleanup 均通过。正式切换备份如下：
- postgresql.dump: `5751049383` bytes / SHA256=`df5055adaceb5130c2d1c2b964911718e7c56d7294e644ebf4ccf532f5d0b551`。
- postgresql.list: `119007` bytes / SHA256=`eaec2ab69e8942df1224d7445fbf2c7e2ce0b741167930e508ccee80a5c0e6b4`。
- redis.rdb: `10781268` bytes / SHA256=`5453b2f3cfd29d0b5ea744f21ac9ab77b88c554d257d9d3575e6d7b7d1ad4e98`。
- redis-check-rdb.txt: `647` bytes / SHA256=`63a08f55ec51b6c58ef15340ca4916a64fae26edb0cc5e668248497551faa3fb`。
- application-data.tar.gz: `78526290` bytes / SHA256=`b387cf4ccc04759963f6fe8458831ea6d6a9c3a678f662f7f0686dced5735ae9`。
- 本次新回滚 tag=`subnexus-rollback:model-evaluation-20260911-890828afe0f7`，archive SHA=`a33a982c244bbc12d5b62a8200fc408305cc82186e64ec24a3d663f9a4935fb3`；一级容器目标=`e389b3b1c4f62fd8d9fb0eb04998b0559d21bf39ae4c1a99bdfc90441def3836`，镜像=`sha256:44e8dcf019338e050756c86aba8d2ecf73390b4d058da2ebf916aba19bea28d9`。用户 switch 才停止改名并保留为 `subnexus-cutover-ui-prior-20260911130706-3232923`；普通 rollback 不恢复数据库。
- 最终证据=`/srv/subnexus-migration/diagnostics/full-33a9601c9330-final-20260911130706-3232923.evidence`，SHA=`807c4ab7eaea72a1bad745b4c1a17a99ad209beb5fddaee0f466db1c838c367f`；facts SHA=`f2367751b502a2d1a735e86f008a949c2ec7cefbcb00bde78560ef912138e722`；`FINAL_PRE_SWITCH_AUDIT=passed`、`FINAL_METADATA_AUDIT=passed`、`PROBE_EXIT_AND_CLEANUP=passed`、`FINAL_SWITCH_EXECUTED=false`。探针从未启动且已精确删除；公网及本地 health/home 全部 HTTP 200，服务器空闲 `22436515840` bytes。
- 生产应用仍为 e389b3b1...，StartedAt=`2026-09-10T00:38:47.787671032Z`，healthy/restart=0；PG/Redis 身份/启动时间不变，379 条迁移账本不变，新功能仍缺失/false，未执行生产 switch/rollback/migration。唯一人工命令已写入切换手册第 15.4 节。
- 本地隔离测试容器/卷/网络已清理，测试辅助盘已卸载，专用 daemon 已停止；F 盘虚拟磁盘再次离线回收后空闲约 63 GiB。此前自动审批拒绝删除的 `D:\SubNexusRelease\compat-890828afe0f7.ext4`（70 GiB）继续保留，未改用其他方式删除。

## 2026-09-11 — 监控私下测试、发布门禁及失败诊断（本地修改）

- 用户截图中的生成请求约 125 秒后返回 HTTP 524，表明上游代理等待响应超时；不能据此判定 Key、模型权限或额度错误。原实现发送非流式请求，现改为三种协议的流式 SSE 接收，仍兼容完整 JSON 响应。必须收到正常完成事件才保存成功 HTML；保留 180 秒总超时、响应/HTML 大小限制、两个并发槽位、不自动重试。未向真实供应商发出验证请求，不能承诺上游不再超时。
- 新增管理员 `POST /tasks/:id/test` 和 `PUT /tasks/:id/publication {published}`（均位于 `/api/v1/admin/model-evaluations`）。任务包含发布状态、测试状态、时间、后台诊断。先保存，再主动测试，测试通过后手动发布；测试允许全局和任务开关关闭。再次测试暂时取消发布，失败不得发布。
- 追加 `9015_subnexus_model_evaluation_publication.sql`，原 9014 不变。已有任务升级后默认未测试、未发布，配置/加密 Key/历史保留，需要管理员逐项测试并发布。修改分组、URL、协议、Key、模型会失效测试并撤销发布；普通元数据修改保留通过状态。当前配置版本之外的历史不向用户展示。
- 用户组列表、历史和详情在服务端受已发布、启用、全局开关及原分组权限约束。测试失败始终只供后台查看；成功测试在发布后可作首个用户样本。后续定时失败在用户 API/界面只显示“生成失败”，清除具体诊断及失败 HTML；后台保留固定、无凭据的诊断。裸域名 URL 现在有明确路径错误提示。
- 验证：前端全量 317 文件 / 2213 项测试通过；定向 ESLint、TypeScript、前端构建通过。后端相关 service/repository/user/admin handler 测试通过，包含真实本地 PostgreSQL 的 4 个主测试、发布鉴权路由测试，以及三种流式协议、截断/错误/超限测试。相关包 go vet 和嵌入最终前端产物的 `go build -tags embed ./...` 均通过。浏览器夹具验证关闭开关时私测、失败禁止发布、成功后手动发布，以及桌面/移动用户诊断隐藏；无页面错误或横向溢出。
- 浏览器报告：`F:\MySub2\.playwright-qa\model-evaluation-workflow\report.json`；前端全量与构建日志前缀 `model-evaluation-workflow-`。本地专用 PG 端口 55439 和 Vite 3107 测试结束后均已停止。没有连接或修改生产，没有创建发布回滚目标或执行切换；这些本地改动需要后续重新完成发布前置，旧候选镜像不包含本次修复。

## 2026-09-11 — 用户监控结果汇总与筛选展示（本地修改）

- 用户确认未选择分组时汇总全部分组结果，按生成时间排序；选定分组时只展示该分组。继续遵守用户已有分组权限、发布状态及开关约束。“全部”不扩大访问范围。
- 已核对原查询支持默认不传 group_id，在权限过滤后统一 `created_at DESC,id DESC` 排序分页；选择/清空分组均回第一页，无需修改后端。用户卡片将分组、生成时间、模型、任务名、状态及耗时置于预览上方，增加排序与筛选说明；保留原预览交互和错误详情隐藏。
- 9 项现有画廊/语言完整性测试、定向 ESLint 和 TypeScript 通过。实际 Chromium 桌面/移动、明暗四组合通过：默认跨分组列表、时间降序、选定分组过滤、翻页后切回全部恢复第一页、预览上方信息位置及无横向溢出。报告：`F:\MySub2\.playwright-qa\model-evaluation-user-feed\report.json`。本轮仅本地 UI 与文档修改，未部署或执行切换。


## 2026-09-11 — 模型监控工作流修复发布前置（进行中，不新建回滚）

- 最新授权：完成全部前置、停在最终切换前，本次不新建回滚目标。旧指示中创建新回滚镜像仅适用于已成功发布的 `33a9601c9330`，不适用于本轮。
- 已实时确认该发布 run `20260911130706-3232923` 为 switched；实际 live=`b4b66b9ca08f185ecbe2e41fc768ff0f3c2685d9a97a4e51ef095da35aafad24`，image=`sha256:9342118b00d127deb7fdc1c62fe19a347acf5563a254a3e9541cf309f492506f`，StartedAt=`2026-09-11T13:32:35.16865245Z`，healthy/restart=0。旧交接命令已撤回。
- 应用 commit=`ccb69f00132dcb9dcbad76e8c3f542231bb002fd`，tree=`4fe51217a193bed8207e23916277d1329bec2f71` 已推送；前端 317 文件/2213 测试及 build 通过，后端完整 `go test -p 2 ./...` 通过。Windows 临时目录转到 F 盘并补 Git sh PATH 后解决测试环境问题，不改应用逻辑。
- 隔离镜像 image=`sha256:c8a9db0d90f3c99924d7c53c7b9b41b4b90f26d053341abc254888e58c12df55`；归档 SHA=`b76430f44eadbe3dac5a0e7c8c775bf2d78f879346f5c17e1f40d4eac878b713`。服务器 candidate Gate `20260911T145454Z-e1e5ca10-25b7-4d7c-bcb7-eccbf059eafd` 已 passed，临时服务清理完成且生产身份未变化。
- 新独立 retained release 入口及 31 个故障/恢复场景通过，运维提交 `e4de7857608ed698c309b8e3c2d7fe92ddcbe58e`。原 full/UI/controller 字节不变。新入口 SHA=`a5b955de3ec51cb1acfc1393290ae2259e8b839fbd3825bda563650e45fd1c3b`，复用 `e389b3b1...` v0.2.4。当前 live 只在人工 switch 的提交前用作临时失败恢复，提交成功即精确删除，不保存为新回滚对象。
- 全新快照 `/srv/subnexus-migration/model-evaluation-workflow-20260911/production.dump`，5756660659 bytes，SHA=`347eff0107d47edd707aadab84f79f11efea240741d3c40dfc16fd67c28f8dfb`；基线 380 条迁移，已有 9014、没有 9015，监控开关 false。数据库传输及完整隔离兼容验证尚未完成，不生成最终切换命令。
- 现有配置/加密 Key/HTML 历史保留；9015 将任务初始化为未测试/未发布，切换后管理员须私有测试通过并明确发布才向用户展示。用户默认按生成时间查看全部有权限且已发布分组结果，分组下拉用于筛选。未发起真实供应商请求。


## 2026-09-12 00:45（Asia/Shanghai）— 模型监控修复全部发布前置完成

- 本次访问生产仅完成授权前置；正式 run `/srv/subnexus-migration/cutover/20260911163046-3338612`，manifest SHA=`20f1c17d67e37c41debaff189ac09a5ecec6369cc6abcc8fbb199192ecd49600`，`prepared/prepared/no`。代理没有执行 switch/rollback/生产迁移。
- 应用固定 `ccb69f00132dcb9dcbad76e8c3f542231bb002fd` / `sha256:c8a9db0d90f3c99924d7c53c7b9b41b4b90f26d053341abc254888e58c12df55`；完整生产副本六阶段兼容测试共 15 项通过并完成精确清理，证据 `/srv/subnexus-migration/docker-candidate/full-ccb69f00132d-compat-eb3d959d95a343cf/evidence.env`，SHA=`e94acd215ff8bd28cb75202948640ac921a8d013227ad366e6689943efe724fc`。全部原配置/Key/HTML 保留，用户权限、错误脱敏、私有测试和明确发布合同验证通过。
- 正式备份如下：
- postgresql.dump: 2754913948 bytes / SHA256=`d88c07d48e47f080ee58b41327f8fb2596fc2c4a70026de3420120893787a9f0`。
- postgresql.list: 121047 bytes / SHA256=`f416fd2ccb66b86aba78920356b946614a12e904f0b95d3c18b8492b54eff24b`。
- redis.rdb: 11992744 bytes / SHA256=`2de507838ef36ab8d6b37edb8dd6c20f1043e0599ad6647d65f48c0b64a14fb6`。
- redis-check-rdb.txt: 647 bytes / SHA256=`e1b928b6fda850b641ae91fba47d954860abfeae88fad904ab133d48c3e82d6b`。
- application-data.tar.gz: 72548825 bytes / SHA256=`024b52ff704d0c4334821ea6b494f6fc61650e74fa1b59edf6e409501c580045`。
- 最终证据 `/srv/subnexus-migration/diagnostics/full-ccb69f00132d-final-20260911163046-3338612.evidence`，SHA=`cc48896506865732eec6f08acb67e2d4d8e1ed6b213674cc11beb9427898a34a`；facts SHA=`e44a0e5479aa2ac91c53c05afb24eb2f40e345dca46289e299a691a4ea2e9c16`，退出 0，never-started probe 已删除。实际 live 仍为 b4b66b9ca08f，StartedAt=`2026-09-11T13:32:35.16865245Z`，healthy/restart=0；PG/Redis 和迁移/设置保持原状。
- 本次不创建回滚目标，继续用 `e389b3b1c4f62fd8d9fb0eb04998b0559d21bf39ae4c1a99bdfc90441def3836` / `sha256:44e8dcf019338e050756c86aba8d2ecf73390b4d058da2ebf916aba19bea28d9`；当前 live 只用于人工切换提交前失败恢复，成功提交即删除临时 current。既有镜像 tag/归档、旧 SubNexus anchor 及其备份保留。完整人工命令只在切换手册 15.5 节。
- 为满足正式备份保留量，仅清理已逐一验证的旧 run `20260907045159-1121373`、`20260908073428-1688478` 的冗余 postgresql.dump，共 10732251260 bytes；审计 `/srv/subnexus-migration/cleanup-workflow-redundant-dumps-20260911.json`，SHA=`60c1f23d1ad9585e5ea9789e8ae58056884f69105c50e7811b54361265b9842c`。当前快照、当前回滚对应备份和所有回滚容器/镜像没有删除。最终服务器空闲 23109042176 bytes。
- 本地完整快照 SHA256 校验后删除两份本轮重复下载临时文件；隔离测试容器、卷和网络已清理，辅助文件系统已卸载，专用 Docker daemon 和 WSL 已停止。专用 ext4.vhdx 停机压缩成功，62781390848→19507707904 bytes，回收 43273682944 bytes 空闲块，F 盘空闲恢复至 67200880640 bytes。先前被自动审批拒绝删除的 70 GiB ext4 文件继续保留，没有更换方式删除。
- 用户明确要求任何用户数据不得受影响。准备阶段仅备份和读取生产；不执行生产 SQL 迁移、重启、切换或数据库恢复。9015 的 ADD COLUMN/CREATE INDEX 仅在维护者手动启动新版本时执行，保留原记录；应用回滚也不恢复旧数据库或历史设置。
- 正式备份体积小于早前兼容快照，追加只读核对确认 1604 项完整备份目录完全一致。生产已有系统日志清理审计（UTC 16:23:51 删除 45359722 条、16:24:35 删除 3914 条），均早于本次 16:30:46 正式 prepare；未调查操作者身份，不将该既有后台操作归因为发布流程。本流程没有执行生产表清理。

## 2026-09-12 — model evaluation reliability pre-switch completion

- Candidate application commit `065d6419f58f3889eb55219b618ebd9b89fc0dbd`, tree `6180c029e05b771ff4c6f90bde9a2a8856e6a5a3`, image `sha256:6e8b4f965f97770d4ffec9c7a5db8f2d3fa50f2718ed02765b9cc00beb3746d8`, archive SHA256 `580fb4aaca77db59aee1b8bba5371f6a2a259e3b23114bca8f4049dc4f3b924e`.
- Deployment controller was updated to accept Docker's actual `AttachStdout/AttachStderr=false` defaults and reproduce a live container's multi-network `NetworkMode`; controller SHA `841174af2353729e95b953cd90cde8ad03cff13c4c5e65722d77b017b9ff9444`, UI library SHA `c4f0a47ddcba3972903761c45857bc2268d0a3f36998ca0b79658fd444cb66d8`, retained wrapper SHA `c52e69b48775f5597b0078cb422322d404a396a192d325dcbbe4d2f0e2acc173`.
- Formal no-schema retained prepare run `/srv/subnexus-migration/cutover/20260912020559-3645138` completed with `state=prepared`, `ui_state=prepared`, `ui_commit_intent=no`, empty candidate container identity, and existing rollback target retained. Final read-only audit evidence `/srv/subnexus-migration/diagnostics/model-eval-065d6419f58f-final-audit.evidence` SHA256 `d97ae28db41fc9c8ad2386de2b3e99c2222c17f862d018c2f68f5f5f77de05d9`; `FINAL_PRE_SWITCH_AUDIT=passed`, `FINAL_SWITCH_EXECUTED=false`.
- Due to capacity, only three superseded switched-run PostgreSQL dump files and their checksum sidecars were removed; current production data, current snapshot, fixed rollback target, and candidate archive were preserved. Cleanup evidence SHA256 `1aa58fbbb81e01766d836bfbf5215813ac16cbc9f3b1742f04fe7c88b875b166`.
- No production switch, rollback, SQL migration, database restore, or feature toggle was executed. Manual switch/rollback commands are bound to run `20260912020559-3645138` and wrapper `no-schema-cutover-065d6419f58f-r2.sh`.
