# SubNexus 迁移台账

> 台账记录每个批次的基线、状态、证据、迁移文件和回滚点。状态只能向前追加，不删除历史状态。

## 状态定义

`未开始`：尚未执行；`进行中`：正在盘点/实现；`待证据`：代码已完成但缺运行或线上证据；`通过`：门禁全部满足；`阻塞`：存在明确停止条件，未经修复不得继续。

## 基线

| 项目 | 值 |
| --- | --- |
| 目标仓库 | `F:\MySub2\sub2api` |
| 旧仓库 | `F:\Sub2Api\SubNexus` |
| 迁移分支 | `feature/subnexus-migration` |
| 目标基线 SHA | `d596d0844` |
| 旧项目参考 SHA | `ccffee6c6` |
| 目标版本/Go | `0.1.185` / `1.27.0` |
| 旧版本/Go | `0.1.135` / `1.26.6` |
| 生产数据库状态 | 待维护者执行实时只读 preflight |

## Batch 0 门禁

| 编号 | 门禁 | 状态 | 证据/备注 |
| --- | --- | --- | --- |
| B0-1 | 从目标 `main` 建立独立迁移分支 | 通过 | `feature/subnexus-migration`，HEAD 与 `main` 同源 |
| B0-2 | 读取规划、旧项目记忆和线上文档 | 通过 | 记录于 `SUBNEXUS_CHANGE_MEMORY.md` |
| B0-3 | 创建项目上下文、功能矩阵、台账 | 通过 | 上下文、功能矩阵、台账、变更记忆及切换/回滚手册已建立 |
| B0-4 | 旧/新逐文件功能与迁移差异盘点 | 通过（本地） | 已完成保留/排除功能的后端、前端、路由、设置、迁移对象和目标接入点映射；线上表状态仍单独以 B0-5 为准 |
| B0-5 | 线上容器/数据库/Redis 只读状态 | 待证据 | 需维护者在当前 OVH 服务器执行命令 |
| B0-6 | 线上 PostgreSQL 备份或可恢复副本 | 未开始 | 不得用生产库直接做本地测试 |
| B0-7 | 隔离库跑候选迁移并启动旧版本回归 | 未开始 | 需 B0-5/B0-6 后执行 |
| B0-8 | 新 fork 上游基线构建/测试 | 通过（本地基线） | 后端 `go test ./... -run '^$' -count=1 -p=1` 退出 0（GOTMPDIR/GOCACHE 指向 F 盘）；前端 `pnpm typecheck`、`pnpm test:run`（249 files/1804 tests）、`pnpm build` 均退出 0；未代表隔离库或生产通过 |

## 实施批次

| 批次 | 范围 | 依赖 | 开关策略 | 状态 |
| --- | --- | --- | --- | --- |
| Batch 1 | 签到、排行榜、活动中心、公告扩展 | B0，目标设置/路由审计 | 每项独立默认关闭 | 未开始 |
| Batch 2 | 首充礼包、二开邀请奖励 | Batch 1 规则、订单/Affiliate 审计 | 默认关闭，奖励幂等 | 未开始 |
| Batch 3 | 发票事务系统 | 数据目录、订单快照、邮件和权限审计 | `invoice_enabled=false` | 未开始 |
| Batch 4 | Battle Pass | 用量/充值/邀请数据合同 | `battle_pass_enabled=false` | 未开始 |
| Batch 5 | 集成、Docker、切换文档和发布候选 | Batch 1-4 全部验收 | 所有功能仍关闭直到逐项批准 | 未开始 |

## 迁移文件登记

| 目标文件名 | 功能 | checksum | 状态 | 备注 |
| --- | --- | --- | --- | --- |
| 待定 | Batch 1 | 待生成 | 未创建 | 必须避开目标现有文件名，不能复用旧编号 |
| 待定 | Batch 2 | 待生成 | 未创建 | 优先独立表/可选字段 |
| 待定 | Batch 3 | 待生成 | 未创建 | 不修改 `payment_orders` 语义 |
| 待定 | Batch 4 | 待生成 | 未创建 | 仅新增隔离 Battle Pass 表 |

## 本地 Batch 0 证据

| 证据 | 时间 | 结果 | 备注 |
| --- | --- | --- |
| 目标 fork 只读预检脚本 shell 语法 | 2026-09-01 Asia/Shanghai | 通过 | `tools/production-deploy/subnexus-readonly-preflight.sh` 由 Git Bash `bash -n` 校验；未执行生产预检 |
| 预检脚本兼容性加固 | 2026-09-01 Asia/Shanghai | 通过 | 全量读取 `schema_migrations`、Atlas revision 最新行和旧活动设置摘要；PG 会话强制 read-only；证据根目录宽路径保护；Git Bash `bash -n` 退出 0 |
| Go 后端编译级基线 | 2026-09-01 Asia/Shanghai | 通过 | 在 `backend` 模块执行 `go test ./... -run '^$' -count=1 -p=1`，退出码 0；专用 GOTMPDIR/GOCACHE 位于 `F:\MySub2` |
| 前端冻结依赖与锁文件 | 2026-09-01 Asia/Shanghai | 通过 | `pnpm install --frozen-lockfile --ignore-scripts` 完成；`frontend/pnpm-lock.yaml` SHA256 保持 `8DBD1876020E41B644D971414D29100C9F428F39EDE953C03D0442B834F6F3AF`，无 diff |
| 前端 typecheck/Vitest/build | 2026-09-01 Asia/Shanghai | 通过 | `pnpm typecheck`、`pnpm test:run`（249 个文件/1804 个测试）、`pnpm build` 均退出码 0；仅有既有 Browserslist/Vite 警告 |
| 同库切换手册 | 2026-09-01 Asia/Shanghai | 已建立 | `SUBNEXUS_CUTOVER_RUNBOOK.md`；仅发布授权后使用 |
| 回滚手册 | 2026-09-01 Asia/Shanghai | 已建立 | `SUBNEXUS_ROLLBACK_RUNBOOK.md`；默认应用回滚，不自动恢复数据库 |
| 线上实时预检 | 待维护者执行 | 待证据 | 需要脱敏回传脚本输出；脚本不执行迁移/备份/重启/切流 |

## 线上证据登记

| 证据 | 时间 | 来源 | 状态 |
| --- | --- | --- | --- |
| `schema_migrations` / `atlas_schema_revisions` | 待采集 | 当前 OVH PostgreSQL | 待证据 |
| 应用容器实时 inspect | 待采集 | `subnexus-cutover` | 待证据 |
| PostgreSQL/Redis 拓扑 | 待采集 | Docker 网络与环境 | 待证据 |
| Nginx 有效配置/端口 | 待采集 | `nginx -T` / inspect | 待证据 |
| PostgreSQL custom-format 备份 | 待创建 | 线上服务器 root-only 路径 | 未开始 |
| 隔离恢复验证 | 待执行 | 本地/隔离 PostgreSQL | 未开始 |

## 回滚点登记

| 阶段 | 回滚点 | 数据库动作 |
| --- | --- | --- |
| 文档/盘点 | `d596d0844` | 无 |
| 每个代码批次 | 批次前提交 SHA | 默认不恢复数据库，关闭对应开关 |
| 线上候选 | 旧容器、旧镜像、服务器回滚脚本 | 先回滚应用，不自动恢复数据库 |
| 数据层灾难 | 切换前验证过的备份 | 需明确批准，评估备份后新增数据损失 |
