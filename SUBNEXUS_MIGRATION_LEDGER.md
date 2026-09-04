# SubNexus 迁移台账

> 最后更新：2026-09-04。台账记录每个批次的基线、状态、证据、迁移文件和回滚点。状态只能向前追加，不删除历史状态。

## 状态定义

`未开始`：尚未执行；`进行中`：正在盘点/实现；`待证据`：代码已完成但缺运行或线上证据；`通过`：门禁全部满足；`阻塞`：存在明确停止条件，未经修复不得继续。

## 基线

| 项目 | 值 |
| --- | --- |
| 目标仓库 | `F:\MySub2\sub2api` |
| 旧仓库 | `F:\Sub2Api\SubNexus` |
| 迁移分支 | `feature/subnexus-migration` |
| fork `main` 基线 SHA | `d596d0844`（未修改） |
| 最新上游基线 SHA | `5097b31457e6dc9f49e5f5c9c72b925ce79543b3` |
| 迁移分支功能候选 SHA | `b26c42e08fb190f3915f08949aaaba48dbe61a26`（上游同步父提交为 `23d6e8ec0`；当前远端发布指针为 `1da1e85dd7be761b22cd219c2c93d92fd48c6bcf`） |
| 旧项目参考 SHA | `62ea35e1c78416fd83e1e41bbb310b307941811a` |
| 目标版本/Go | `0.2.0` / `1.27.0`（最新上游） |
| 旧版本/Go | `0.1.135` / `1.26.6` |
| 生产数据库状态 | 只读预检与切换前备份已完成；PostgreSQL 18.4 备份已在本机隔离恢复并克隆，生产库仍未执行迁移/DDL/DML |

## Batch 0 门禁

| 编号 | 门禁 | 状态 | 证据/备注 |
| --- | --- | --- | --- |
| B0-1 | 从目标 `main` 建立独立迁移分支 | 通过 | `feature/subnexus-migration`，HEAD 与 `main` 同源 |
| B0-2 | 读取规划、旧项目记忆和线上文档 | 通过 | 记录于 `SUBNEXUS_CHANGE_MEMORY.md` |
| B0-3 | 创建项目上下文、功能矩阵、台账 | 通过 | 上下文、功能矩阵、台账、变更记忆及切换/回滚手册已建立 |
| B0-4 | 旧/新逐文件功能与迁移差异盘点 | 通过（本地） | 已完成保留/排除功能的后端、前端、路由、设置、迁移对象和目标接入点映射；线上表状态仍单独以 B0-5 为准 |
| B0-4a | 同内容/语义改名迁移逐项审计 | 通过（本地静态） | 共 27 组显式 alias：历史 23 组内容相同、2 组语义接管，另有学生优惠/注册冷却 2 组独立表接管；含 DML/索引/约束的重放风险已登记于规划 6.1.1；需隔离库和线上记录验证 adoption |
| B0-4b | 改名迁移 alias/adoption runner 与对象契约 | 本地与生产备份隔离验证通过 | 当前工作树为 27 组显式映射；真实生产备份克隆首次接管、二次幂等和对象契约均通过；生产库仍未执行迁移 |
| B0-8 | 新 fork 上游基线构建/测试 | 通过（本地候选） | 后端默认构建与 `unit` 标签全量测试、`go vet` 通过；前端 `pnpm typecheck`、Vitest（282 个文件/1954 个测试）、`pnpm build` 通过；主机进程 smoke 通过不代表 Docker、持久化 Redis 或生产通过 |

## Release Gate（保留历史编号）

以下项目只阻止上传后的生产发布，不阻止 Batch 1-5 本地开发：

| 编号 | 门禁 | 状态 | 证据/备注 |
| --- | --- | --- | --- |
| B0-5 | 线上容器/数据库/Redis 只读状态 | 通过 | 固定脚本与 SHA256 校验通过；证据 `/srv/subnexus-migration/preflight/20260903072817/evidence.txt`，无迁移或部署 |
| B0-6 | 线上 PostgreSQL、Redis 与应用数据备份 | 通过（创建与结构校验） | `/srv/subnexus-migration/backups/20260903T073714Z`；PG custom dump/list、globals、Redis RDB/check、应用 tar 和全量 SHA256 均通过 |
| B0-7 | 生产备份隔离恢复、候选迁移和旧版本回归 | 通过（Docker 候选 gate 通过；待维护者人工验收） | PostgreSQL 18.4 恢复、Redis 8.8.0 RDB 隔离加载、真实克隆 migration/adoption、关闭态候选启动、旧版 0.1.135 回归及 Docker 候选 runtime gate 均通过；最新 gate 证据 `20260904T104343Z-2854f544-d1ee-44be-9b58-ff465ee160ac`，`result=passed`、`cleanup_failed=false`、迁移数 290、重启前后一致；`cutover_allowed=false`、`manual_review_required=true`，不得据此自动切换 |

## 实施批次

| 批次 | 范围 | 依赖 | 开关策略 | 状态 |
| --- | --- | --- | --- | --- |
| Batch 1 | 签到、排行榜、活动中心、公告扩展 | 本地 B0-1 至 B0-4b/B0-8、`upstream/main=5097b3145` 已同步 | 每项独立默认关闭 | 本地实现完成，待最终证据/维护者验收 |
| Batch 2 | 首充礼包、二开邀请奖励、学生充值优惠、注册 IP 冷却 | Batch 1 规则、订单/Affiliate/Auth 审计 | 默认关闭，奖励/注册 reservation 幂等 | 本地实现完成，待最终证据/维护者验收 |
| Batch 3 | 发票事务系统 | 数据目录、订单快照、邮件和权限审计 | `subnexus_invoice_enabled=false`（public 映射 `invoice_enabled`） | 本地实现完成，待最终证据/维护者验收 |
| Batch 4 | Battle Pass、Channel Monitor V3、默认语言、客服按钮 | 用量/充值/邀请数据合同；上游监控基础 | 所有功能/模式默认关闭或回退安全默认 | 本地实现完成，待最终证据/维护者验收 |
| Batch 5 | 集成、Docker、隔离 PostgreSQL/Redis、旧版本回归和发布候选 | Batch 1-4 本地实现 | 所有功能仍关闭直到逐项批准 | 合成夹具、真实生产克隆的 PostgreSQL 接管、候选关闭态 smoke、旧版 0.1.135 回归、Redis 8 RDB 实际加载及 Docker 候选 runtime gate 均通过；候选仍需维护者人工验收，最终切换和逐项开启未执行 |

## 主审第二轮剩余项收口（2026-09-03）

| 项目 | 状态 | 证据 |
| --- | --- | --- |
| P3-01 简易模式管理直链 | 已完成 | `frontend/src/router/index.ts` 将 `/admin/checkin`、`/admin/leaderboard` 纳入限制；guards 回归测试通过 |
| P3-02 签到/排行榜管理写操作 step-up | 已完成 | 后端配置/排行榜奖励路由统一挂 `StepUpAuthMiddleware`；签到和排行榜页面保存动作接入 TOTP 自动重试 |
| P3-03 敏感路由 step-up 覆盖 | 已完成 | 路由测试覆盖学生优惠 3 条、发票 8 条、签到 1 条、排行榜 2 条敏感写路由，共 14 条 |
| P3-04 注册冷却 helper 错误吞掉 | 已完成 | 删除未使用且会吞错的 `AuthService.registrationIPCooldownEnabled`；OAuth 回滚按已绑定 reservation 参数直接释放 |
| F05 退款/关闭态语义 | 已完成 | 测试固定退款后 `PrepareOrder` 返回 `ErrFirstRechargeAlreadyPurchased`；文档明确关闭态清理仅为后台补偿 |

## 迁移文件登记

| 目标文件名 | 功能 | checksum | 状态 | 备注 |
| --- | --- | --- | --- | --- |
| `9001_subnexus_activity_center.sql` | 活动中心 custom 卡片 | `71a83d4789e33b8d99150f4ad7e48d9195a762c408c6186d1fcb5e2b98016972` | 已创建；本地契约通过 | 独立/幂等表结构，用户和管理查询仅允许 `activity_type='custom'` |
| `9002_subnexus_checkin.sql` | 签到、奖励日志和排行榜基础对象 | `5bdf1548a58ac9e9b3d6304200aa44447562be2916a5a207cc869066c638157c` | 已创建；本地契约通过 | 独立活动表/索引；关闭态不写余额或活动表 |
| `9003_subnexus_invoice_transactions.sql` | 发票事务系统 | `49f7d6cadf50ea4959bcfd5d7a2dc52a79b55b38aa372269c70cdf6756ae8b53` | 已创建；本地契约通过 | 独立表；不修改 `payment_orders` |
| `9004_subnexus_invite_rewards.sql` | 邀请奖励配置/流水 | `435b11b3c2721a06914d6fd593068b1f2e9cf5c0013491009f6ee33079e65e12` | 已创建；本地契约通过 | 奖励日志与幂等约束；默认关闭 |
| `9005_subnexus_first_recharge.sql` | 首充礼包预约 | `600d682ee7b80ab27f8f3f064dfbadf71ab4c321f91d10abab2c9a491a2ce867` | 已创建；本地契约通过 | 预约 CAS/行锁；默认关闭 |
| `9006_subnexus_battle_pass.sql` | Battle Pass | `14152499f8e656d76691b6432e8583de9a3546c288e8a02ad25ef4032173d28d` | 已创建；本地契约通过 | 独立赛季/任务/奖励表；默认关闭 |
| `9007_subnexus_marquee.sql` | 跑马灯手动广播 | `19deec6328c814418b66372c066fd2b439bcbdb5c11659394b5fdf32e509128b` | 已创建；本地契约通过 | 幂等复用 `activity_broadcasts`，只允许 `source='admin'` |
| `9008_subnexus_student_recharge_benefit.sql` | 学生充值优惠 | `f7e2caaf7d0587a5e40cc0f9938166797145fbd6538499355b635ea9ed3e6d24` | 已创建；本地契约通过 | 独立奖励日志；兼容旧 `199_student_recharge_benefit.sql` |
| `9009_subnexus_invite_activities_notx.sql` | 邀请活动查询索引 | `05be0f1771b60af886867b1214d5f3c8e7e6d424e59471a9c7a2fe2a4e003d73` | 已创建；本地契约通过 | 仅并发创建索引，不写设置/余额/订单 |
| `9010_subnexus_registration_ip_cooldown.sql` | 注册 IP 冷却 | `d84e20270be20d7fe06175c480dea2b99f905b56079a0670ca6f757dfb429683` | 已创建；本地契约通过 | 独立表/索引；兼容旧 `159_registration_ip_cooldown.sql` |
| `9011_subnexus_rollout_gates.sql` | 独立 rollout 默认值 | `dc95bd29b26a3807ae0c3457958a673161609adc35318641aa0677b5e11fd03c` | 已创建；本地契约通过 | `ON CONFLICT (key) DO NOTHING`，不覆盖已有设置，全部默认 false |
| `9012_subnexus_invite_signup_reward_jobs.sql` | 邀请注册奖励重试队列 | `7a1bd27748aebf63362045ec171680b5465021fbc756f97a4c2a240385491e0f` | 已创建；本地契约通过 | 持久队列/重试/幂等；默认关闭 |
| `9013_subnexus_leaderboard_rewards.sql` | 排行榜奖励扩展 | `856f2fb34f2ff77a24bafa63e49af177d1d6f12c06b549afe0c21a5c6ec759ae` | 已创建；本地契约通过 | 唯一目标 `(source,period,user_id)`；默认关闭 |

## 本地 Batch 0 证据

| 证据 | 时间 | 结果 | 备注 |
| --- | --- | --- |
| 目标 fork 只读预检脚本 shell 语法 | 2026-09-01 Asia/Shanghai | 通过 | `tools/production-deploy/subnexus-readonly-preflight.sh` 由 Git Bash `bash -n` 校验；未执行生产预检 |
| 预检脚本兼容性加固 | 2026-09-01 Asia/Shanghai | 通过 | 全量读取 `schema_migrations`、Atlas revision 最新行和旧活动设置摘要；PG 会话强制 read-only；证据根目录宽路径保护；Git Bash `bash -n` 退出 0 |
| 迁移分支远端固定 | 2026-09-01 Asia/Shanghai | 通过 | `origin/feature/subnexus-migration` 已推送至 `aa960806/sub2api`，提交 `402d0b0e473bd6c0b8bc80a815a7da335e0a0c5a`；只新增远端分支，未修改 `main` |
| 迁移分支文档发布前固定点 | 2026-09-01 Asia/Shanghai | 通过 | 文档发布前 `origin/feature/subnexus-migration` 与本地均为 `7747627d5646b140e4b716463d5e6342673d343c`；`main` 保持 `d596d0844f274c3e7933c966231851f9f20b0d47`；当前 SHA 以 Git 实时查询为准 |
| 线上预检脚本发布校验（历史固定提交） | 2026-09-01 Asia/Shanghai | 待维护者执行 | 历史提交脚本 SHA256=`ECB985233881E3C20BD20B8D394275D35F50AF1F344EBFADDB1BF13AA9A02E84`；本轮已更新脚本，以下一行是当前固定版本 |
| 线上预检脚本发布校验（历史固定提交，已过期） | 2026-09-01 Asia/Shanghai | 已被本轮 supersede | 历史提交 `dfec06ac1c939e07629d8c70b04c2a509f8007d0` 的脚本 SHA256=`004886DEF59C5AA1AB31B2A44FB482A997D40131575BCC60706390BA80A00F87`；不得用于本轮线上执行 |
| 线上预检脚本发布校验（历史固定提交，已 superseded） | 2026-09-02 Asia/Shanghai | 已被 `093163b291` supersede | 历史批准提交 `7200e5ae1f48d8f78bce43565814378b636c842b`；脚本 SHA256=`D68B6BD54AF75B821257F42FC9A7360E0E9828AD0F561B9045B92137036255D1`；仅保留审计链，禁止线上执行 |
| 线上预检脚本发布校验（历史 Batch 0 资产，已冻结） | 2026-09-02 Asia/Shanghai | 禁止当前执行 | 历史批准提交 `093163b2918fe15af8f909ae716531b9298f75b6`；脚本 SHA256=`42698FFF5751C8CF22724E065ABBC491D4D2192EA01895714F168DCEC76EF1C6`；最终本地候选完成后必须重新审核并固定新值 |
| Go 后端编译级基线 | 2026-09-01 Asia/Shanghai | 通过 | 在 `backend` 模块执行 `go test ./... -run '^$' -count=1 -p=1`，退出码 0；专用 GOTMPDIR/GOCACHE 位于 `F:\MySub2` |
| 前端冻结依赖与锁文件 | 2026-09-01 Asia/Shanghai | 通过 | `pnpm install --frozen-lockfile --ignore-scripts` 完成；`frontend/pnpm-lock.yaml` SHA256 保持 `8DBD1876020E41B644D971414D29100C9F428F39EDE953C03D0442B834F6F3AF`，无 diff |
| 前端 typecheck/Vitest/build | 2026-09-01 Asia/Shanghai | 通过（历史基线） | 当时 `pnpm typecheck`、`pnpm test:run`（249 个文件/1804 个测试）、`pnpm build` 均退出码 0；当前候选的更新结果见 2026-09-03 记录 |
| 同库切换手册 | 2026-09-01 Asia/Shanghai | 已建立 | `SUBNEXUS_CUTOVER_RUNBOOK.md`；仅发布授权后使用 |
| 回滚手册 | 2026-09-01 Asia/Shanghai | 已建立 | `SUBNEXUS_ROLLBACK_RUNBOOK.md`；默认应用回滚，不自动恢复数据库 |
| 线上实时预检 | 2026-09-03 Asia/Shanghai | 通过 | 固定提交与脚本 SHA256 校验、root-only 证据 SHA256、运行时身份和只读标记均通过；详见 B0-5 与线上证据登记 |
| 改名迁移静态审计 | 2026-09-01 Asia/Shanghai | 通过（本地） | 目标/旧迁移按 `SHA256(TrimSpace(SQL))` 比较，确认 23 组同内容改名；另审计 2 组语义接管；含 DML 的文件不得在同库直接重跑 |
| 改名迁移 adoption 本地 runner 验证 | 2026-09-01 Asia/Shanghai | 通过（本地） | 目标/旧文件 checksum 23/23 一致；repository 单测、`go vet`、全后端编译级测试和 integration-only 编译通过；触发器规范化顺序与 `groups.platform NOT NULL` 契约已校正 |
| 目标迁移 SQL 隔离目录验证 | 2026-09-01 Asia/Shanghai | 通过（本机 PostgreSQL 16） | 在临时隔离集群执行目标迁移集合，并读取列、索引有效性/定义、约束、函数、触发器；未使用生产数据库，历史集群已停止 |
| 本地 PostgreSQL 同库接管矩阵 | 2026-09-03 Asia/Shanghai | 通过 | 隔离 PostgreSQL 16（`127.0.0.1:56000`）中目标集合 290 条迁移、旧集合 268 条迁移均成功；旧库交给当前 runner 首次接管、再次幂等和旧集合重复校验均通过，最终 `schema_migrations=371`；目标 checksum（兼容 alias 除外）和索引有效性检查通过。该集群仅用于本地测试，未连接生产库 |
| 本地 Batch 1-4 全量验证 | 2026-09-03 Asia/Shanghai | 通过（本地代码层） | Go 默认/`unit` 全量、`go vet`、前端 Vitest 282/1954、typecheck、build 通过；运行时子门禁另见下方 Batch 5 记录 |

## 线上证据登记

| 证据 | 时间 | 来源 | 状态 |
| --- | --- | --- | --- |
| `schema_migrations` / `atlas_schema_revisions` | 2026-09-03 | 只读预检证据 | 通过；生产未执行 adoption/迁移 |
| 应用容器实时 inspect | 2026-09-03 | `subnexus-cutover` | 通过；healthy，`127.0.0.1:18083 -> 8080` |
| PostgreSQL/Redis 拓扑 | 2026-09-03 | Docker 网络与环境 | 通过；PostgreSQL 18.4、Redis 8.8.0 standalone |
| Nginx 有效配置/端口 | 2026-09-03 | `nginx -T` / inspect | 通过；备份后 `nginx -t` 和 active 状态再次确认 |
| PostgreSQL/Redis/应用备份 | 2026-09-03 | `/srv/subnexus-migration/backups/20260903T073714Z` | 创建与结构校验通过；全部 SHA256 为 `OK` |
| PostgreSQL 隔离恢复验证 | 2026-09-03 | `127.0.0.1:56418` / PostgreSQL 18.4 | 通过；原始恢复库 `sub2api` 保留，`FILE_COPY` 克隆 `subnexus_candidate`；两库 `schema_migrations=268`、核心计数/金额一致、invalid index=0 |
| Redis RDB 隔离恢复验证 | 通过 | 生产 Redis 镜像 ID 的无网络/无端口独立 Redis 8 候选 | `PING=PONG`、`RDB_TOTAL=39026`、`DBSIZE=18520`；RDB 加载 18520、过期 20506；候选容器已自动清理，生产 Redis 身份未变化；证据 `/srv/subnexus-migration/redis-restore/20260903T131430Z-3144728-6273df02-cf80-4b5e-9901-1b5f08c7b008/evidence.txt`，证据 SHA256=`f7f524028f2bacadff58efa669e5a4f91fc7c4b3dd38e09a4f17b1324b0e319a` |
| 真实克隆 migration/adoption | 2026-09-03 | `subnexus_candidate` | 通过；268→371，13/13 SubNexus、27/27 alias、28/28 目标表、45/45 目标索引，二次幂等及 checksum 契约通过，invalid index=0 |
| 真实克隆候选/旧版回归 | 2026-09-03 | `127.0.0.1:18184` / `18185` | 通过；候选全部迁移功能公开开关 false，旧版 0.1.135 可读取迁移后同库；所有进程和临时防火墙已清理 |

## 回滚点登记

| 阶段 | 回滚点 | 数据库动作 |
| --- | --- | --- |
| 文档/盘点 | `d596d0844` | 无 |
| 当前本地候选 | `b26c42e08fb190f3915f08949aaaba48dbe61a26` | 无；所有迁移开关保持关闭 |
| Batch 0 改名迁移 adoption 门禁 | `dfec06ac1c939e07629d8c70b04c2a509f8007d0`（父提交 `df0e6a136`） | 无；仅新增启动前校验与只读预检证据采集 |
| 每个代码批次 | 批次前提交 SHA | 默认不恢复数据库，关闭对应开关 |
| 线上候选 | 旧容器、旧镜像、服务器回滚脚本 | 先回滚应用，不自动恢复数据库 |
| 数据层灾难 | 切换前验证过的备份 | 需明确批准，评估备份后新增数据损失 |

## 追加操作记录

| 时间 | 操作 | 结果 | 线上/数据库影响 |
| --- | --- | --- | --- |
| 2026-09-02 Asia/Shanghai | 提交并推送文档固定点表述修正（`docs: clarify migration branch fixed points`） | 通过；脚本哈希、Shell 语法、差异检查和远端分支指向已复核 | 未访问线上；未执行 SQL、备份、部署、切流或开关修改 |
| 2026-09-02 Asia/Shanghai | 25 组旧迁移接管与预检安全收尾复核（历史记录） | 本地通过；线上待证据 | 当时记录保留审计链；当前 alias 总数已扩展为 27 组，见本台账 B0-4a/B0-4b | 仅使用本机临时 PostgreSQL；未访问线上，未执行生产 SQL、备份、部署、切流或开关修改 |
| 2026-09-02 Asia/Shanghai | 统一 Batch 0 预检批准脚本发布点 | 通过；线上待证据 | 手册和台账统一使用批准脚本提交 `7200e5ae1f48d8f78bce43565814378b636c842b`，并保留父提交关系与脚本 SHA256；`git diff --check` 通过 | 未访问线上；未执行 SQL、备份、部署、切流或开关修改 |
| 2026-09-02 Asia/Shanghai | 收敛预检脚本校验与执行竞态 | 通过；线上待证据 | 手册改为 root shell 内校验批准 Git blob、生成 root-only 临时副本并执行；要求隔离仓库及 `.git` root-owned；脚本 SHA256 不变 | 未访问线上；未执行 SQL、备份、部署、切流或开关修改 |
| 2026-09-02 Asia/Shanghai | 修正独立 clone 的 no-checkout 状态检查顺序 | 通过；线上待证据 | `git clone --no-checkout` 后先 checkout 固定提交，再检查 `git status --porcelain`；避免把暂未 checkout 的受跟踪文件误判为脏工作树 | 未访问线上；未执行 SQL、备份、部署、切流或开关修改 |
| 2026-09-02 Asia/Shanghai | 修正 Docker PortBinding 字段并更新预检批准点 | 通过；线上待证据 | 提交 `093163b2918fe15af8f909ae716531b9298f75b6`；脚本 SHA256=`42698FFF5751C8CF22724E065ABBC491D4D2192EA01895714F168DCEC76EF1C6`；旧批准值已标记 superseded；未访问线上，未执行 SQL、备份、部署、切流或开关修改 |
| 2026-09-02 Asia/Shanghai | 跑马灯独立扩展本地迁移 | Marquee service/repository/迁移定向测试、handler/routes 编译、前端 7 个定向测试、typecheck/build/ESLint 通过；全仓集成仍待 Batch 汇总 | 新实现严格限定 `source='admin'`，关闭态不访问广播表或创建轮询；未访问线上、生产数据库或 Redis，未部署、推送、切流或开启功能 |
| 2026-09-02 Asia/Shanghai | 邀请活动与明确排除项静态审计 | 邀请活动后端 3 组 Go 专项、迁移契约和前端 5 文件/22 测试通过；排除项无新增 SubNexus 文件、路由、handler/service 或迁移 | 仅保留邀请抽奖、累计充值奖励转盘和邀请里程碑；每日消耗转盘、红包雨、运行日历、Media Studio/Creative Workshop 不迁移；未访问线上、未执行 SQL、未部署或开启开关 |
| 2026-09-03 Asia/Shanghai | Batch 1-4 本地候选收敛与文档校准 | 通过（代码层）；Batch 5 运行时证据待办 | 后端 `go test -tags unit ./... -count=1 -p=1 -timeout=30m`、默认 `go test ./... -count=1 -p=1 -timeout=45m`、`go vet` 通过；前端 Vitest 280 文件/1950 测试、typecheck、build 通过；F01-F13 状态统一为“本地实现完成，待最终证据/维护者验收” | 当前只在本地候选工作树；未推送当前改动、未连接线上 PostgreSQL/Redis、未部署、未重启、未切流、未修改生产开关 |
| 2026-09-03 Asia/Shanghai | 前端 feature flag stale-cache fail-closed | 本地专项通过 | `isFeatureFlagEnabled` 在 `publicSettingsLoaded` 非 true 时强制关闭；registry 补齐 marquee、first recharge、student benefit、invite activities；新增 `featureFlags.spec.ts` 4 个测试 | 未访问线上；所有迁移功能仍默认关闭 |
| 2026-09-03 Asia/Shanghai | Batch 5 Docker 能力检查 | Docker daemon 阻塞（静态 config 通过） | `docker compose version` 可用，但 Docker daemon 未运行（Windows named pipe 不存在）；未创建容器或镜像，Docker 运行时门禁保留待办 | 未访问线上；未启动/修改 Docker 服务 |
| 2026-09-03 Asia/Shanghai | 本地候选最终代码复核与 Docker 再检查 | 代码门禁通过；Docker 运行时阻塞 | `go generate ./cmd/server`、Go 默认/`unit` 全量、`go vet`、迁移契约、gofmt、敏感扫描、`pnpm lint:check`、typecheck、Vitest 280/1950、build 通过；修复 `InvoicesView.vue` 3 处 ESLint 多余分号；compose 静态 config 通过。Docker Desktop 启动后 Inference manager 路径错误，未创建容器 | 仅本地操作；未访问旧项目可写路径、线上、生产数据库/Redis，未推送、部署或修改开关 |
| 2026-09-03 Asia/Shanghai | Batch 5 隔离 PostgreSQL 接管和幂等验证 | 通过（PostgreSQL 子门禁） | 在 `F:\MySub2\.subnexus-pg16-20260903`（仅监听 `127.0.0.1:56000`）复核目标 290 条迁移、旧版 268 条迁移、旧库向当前 runner 的首次接管/第二次幂等/旧集合重复执行；接管后 `schema_migrations=371`，目标 checksum（兼容白名单除外）匹配且无 invalid index。Docker 镜像运行仍待办 | 未访问线上；未执行生产 SQL、备份、部署、重启、切流或开关修改 |
| 2026-09-03 Asia/Shanghai | Batch 5 隔离 miniredis 与候选主机 smoke | 通过（轻量运行时子门禁） | miniredis 仅监听 `127.0.0.1:56379`；候选主机进程 `18180` 的 health、setup、管理员登录、关闭态响应和二次启动通过；不代表 Redis 持久化、Docker 镜像或生产通过 | 未访问线上；Redis 数据仅为本地临时测试 |
| 2026-09-03 Asia/Shanghai | 旧版 0.1.135 同库回滚克隆回归 | 通过（隔离克隆） | 在 `subnexus_old_regression_login_20260903`（`schema_migrations=371`、users=1、settings=52）首轮及重启后验证 health/setup/public settings、有效管理员登录、`auth/me`、管理员只读 GET；旧版不存在的新迁移路由按预期 404，data-management deprecated 按预期 503；用户/迁移计数不变，仅第二次登录产生预期审计记录，无 checksum 错误 | 未访问线上；旧版进程和 Redis 均使用本机隔离资源 |
| 2026-09-03 Asia/Shanghai | 再次复核旧项目渠道监控 V3 修正 `62ea35e1c` | 通过（无需代码重放） | 7 个源码/测试文件中 5 个逐字节一致，2 个仅注释/排版/测试位置差异；V3 时间线、尾部空桶、90/80 可用性阈值行为一致；目标专项 Vitest 2 文件/16 测试和受影响文件 ESLint 通过 | 仅读旧项目、只写目标记忆文档；未访问线上、未修改 `main`、未部署、未执行 SQL 或开关变更 |
| 2026-09-03 Asia/Shanghai | 主审报告独立复核与确认问题修复 | 通过（本地代码/文档门禁；待维护者复审） | 补齐 F01 签到管理页/路由/侧栏/i18n；公开设置与 runtime 对非法 channel monitor mode 统一 fail-closed；客服 Markdown 白名单/协议/blank-target 防护；签到冲突目标收紧；活动中心/跑马灯写入在事务内锁定开关；注册冷却设置读取异常拒绝待定 OAuth 完成；发票和学生优惠写操作接入 step-up 及前端 TOTP 重试；校正旧项目参考 SHA；前端全量 282/1954，后端定向 service/repository/routes 测试通过 | 仅修改目标迁移分支；未修改旧项目、`main`、服务器、线上 PostgreSQL/Redis，未推送、部署或修改生产开关 |
| 2026-09-03 Asia/Shanghai | 主审复核后的后端全量门禁收尾 | 通过（本地；待维护者复审） | `go test ./... -run '^$' -count=1 -p=1 -timeout=45m`、`go vet ./...`、`go test -tags unit ./... -count=1 -p=1 -timeout=30m` 均退出码 0；全仓编译、静态分析和 unit 测试通过。前端全量 282/1954、typecheck、lint、build 仍保持通过 | 未访问线上、未连接生产 PostgreSQL/Redis；未修改旧项目、fork `main`、生产开关；未推送或部署 |
| 2026-09-03 Asia/Shanghai | Batch 5 Redis 8 RDB 隔离恢复门禁 | 通过（线上备份的隔离恢复证据） | 通过服务器已安装脚本 `/srv/subnexus-migration/tools/subnexus-redis-restore-check.sh` 执行；候选使用生产 Redis 不可变镜像 ID、`--network none`、无端口发布、只读 RDB 挂载；`PING=PONG`、`DBSIZE=18520`、`RDB_LOADED=18520`、`RDB_EXPIRED=20506`、`RDB_TOTAL=39026`；临时容器自动清理，生产 Redis 身份未变化。证据文件为 `/srv/subnexus-migration/redis-restore/20260903T131430Z-3144728-6273df02-cf80-4b5e-9901-1b5f08c7b008/evidence.txt`，SHA256=`f7f524028f2bacadff58efa669e5a4f91fc7c4b3dd38e09a4f17b1324b0e319a` | 服务器仅读取备份并创建/删除隔离候选容器；未停止、重启或修改生产 Redis、应用、PostgreSQL、Nginx，未迁移、部署、切流或修改开关 |
| 2026-09-03 Asia/Shanghai | Docker 候选门禁主机准备 | 通过（工具/不可变镜像准备；候选运行待办） | 资源为 8 vCPU、约 17 GiB available memory、约 38 GiB free disk；仅安装 Ubuntu `docker-buildx 0.30.1-0ubuntu1`，Docker 服务重启被延后且无容器重启；BuildKit 0.26.2 可用。按固定 digest 预拉 BuildKit/Node/Go，现有 Alpine/PostgreSQL/Redis 镜像按不可变 ID复用；生产应用始终 healthy、三容器 ID 不变 | 仅新增约 71 MB CLI 包和固定构建镜像层；未构建候选、未连接或修改生产数据库/Redis、未停止/重启生产容器、未切流、未修改开关；最终切换仍必须由维护者手动执行 |
| 2026-09-04 Asia/Shanghai | Docker 候选门禁独立复核与脚本修正 | 通过（本地静态/夹具；真实 Docker 待办） | 修复 repository digest 去 tag 后精确匹配、预加载候选 tag 复用前归档 hash/inode 复核、归档路径先做不跟随符号链接规范化；候选客服 namespaced/legacy 开关均写入并验证 false；新增 resolver/preloaded-tag 夹具。`bash -n`、isolated-image-build 静态测试、candidate-check 静态/夹具测试通过 | 仅修改 fork 迁移工作树和记忆文档；未修改旧项目、`main`、服务器或生产数据，未执行真实 Docker build、迁移、部署、重启、切流或开关变更 |
| 2026-09-04 Asia/Shanghai | 发布门禁安全复核二次加固 | 通过（本地静态/夹具；真实 Docker 待办） | 清理仅在明确 not-found 且精确列表为空时继续，超时/未知 Docker 错误硬失败；证据目录与归档路径在创建/读取前拒绝和生产挂载源的物理路径重叠，并拒绝缺失路径父级中的符号链接；候选 gate 拒绝符号链接 Docker socket；候选与本地构建脚本对 `git submodule status --recursive` 错误硬失败。四个脚本 `bash -n`、Docker candidate-check、isolated-image-build、readonly-preflight、Redis restore-check 夹具测试通过 | 仅修改 fork 迁移分支；未修改旧项目、`main`、服务器、生产 PostgreSQL/Redis/Nginx，未执行部署、迁移、重启、切流或开关变更 |
| 2026-09-04 Asia/Shanghai | Registry 端口镜像引用校验回归修复 | 通过（本地静态/夹具；真实 Docker 待办） | 修正候选 gate 与隔离构建脚本的不可变引用校验，支持 `registry.example:5000/repo[:tag]@sha256:<64hex>`，并拒绝非数字/空/超长端口；新增两套正负夹具。四个脚本 `bash -n`、candidate-check 和 isolated-image-build 测试由主线程复跑通过 | 仅修改 fork 迁移分支和记忆文档；未运行真实 Docker build/gate，未修改旧项目、`main`、服务器或生产数据，未部署、重启、切流或修改开关 |
| 2026-09-04 Asia/Shanghai | 清理错误诊断匹配收紧 | 通过（本地静态/夹具；真实 Docker 待办） | 清理仅接受明确 `no such object/container/network/volume`（隔离构建 image 为 `no such image`）诊断，并要求精确列表为空且 daemon 健康；移除宽泛 `not found` 匹配，新增误导性错误与缺失 image 列表查询夹具，主线程复跑四个脚本语法及四套夹具 | 仅修改 fork 迁移分支；未运行真实 Docker build/gate，未修改旧项目、`main`、服务器或生产数据，未部署、重启、切流或修改开关 |
| 2026-09-04 Asia/Shanghai | 本地隔离 Docker 构建首次运行与安全扫描修复 | 进行中（真实 Docker 待重跑） | 首次真实构建在源树安全扫描阶段拒绝合法 `deploy/.env.example`，未创建任何 builder/镜像/卷/网络；新增 `is_safe_env_example_path` 统一允许嵌套 `.env.example`/`.env.sample`，并增加正负夹具；`bash -n`、isolated-image-build 静态测试、`git diff --check` 通过。创建独立 WSL daemon `SubNexusBuild20260904`，预加载五个固定 digest 基础镜像，未共享/清理 Docker Desktop 资产 | 仅修改 fork 迁移分支及本地隔离运行时；未修改旧项目、`main`、服务器、生产数据、部署或开关；修复提交后重新构建并校验归档 |
| 2026-09-04 Asia/Shanghai | 隔离 Docker context 权限归一化修复 | 进行中（真实 Docker 待重跑） | 复现确认 `git archive` 在 WSL 将 Git `100644/100755` 记录为 `0664/0775`，导致固定 context 门禁正确拒绝；提交 `3a095f8a534cb93d176d9147114ddbb1e0cec446` 在解包时统一移除 group/other 写权限并保留执行位；脚本 SHA256=`a01527dbb91de2b7dbd0c4ce7a3b17ee7a6b6ceff4eaf44c4026edcfbdce2ec5`；静态/语法/差异测试通过，并以合成 tar 实际验证 `0777→0755`、`0664→0644`、`0775→0755` | 仅修改 fork 迁移分支及本地隔离运行时；未修改旧项目、`main`、服务器、生产数据，未创建候选镜像/容器/卷/网络；待同步 detached clone 后重跑真实构建 |
| 2026-09-04 Asia/Shanghai | 隔离构建 migration snapshot 误报修复与合同加固 | 进行中（真实 Docker 待重跑） | 提交 `0823fba399e892128ff4474f5f31394593976a29` 的真实构建在 `117_add_payment_order_provider_snapshot.sql` 误报处停止，未创建构建对象；现仅放行 migration-shaped snapshot SQL，继续拒绝 dump/backup/export/压缩快照；删除可变外部 Dockerfile frontend，精确解析大小写/缩进 FROM；锁定 artifact 目录 inode；成功路径比较 baseline+候选镜像，失败/中断清理后比较完整对象基线；成功构建后删除 staging context。Git Bash/WSL 四脚本语法与四套测试、context 清理/对象基线/动态权限夹具及 `git diff --check` 通过 | 只修改 fork 迁移分支和专用 WSL 隔离运行时；一次专用 daemon Node 镜像误删已按固定 digest 恢复，未触碰 Docker Desktop/服务器；提交并复核五个基础镜像后再重跑，生产迁移、部署、切流继续禁止 |
| 2026-09-04 Asia/Shanghai | Buildx PID 限制参数兼容修复 | 进行中（真实 Docker 待重跑） | `75a3a33e6d2a4dc63434879bd66c78337dd904fc` 的真实隔离构建在 builder 创建时因 Buildx 不支持 `pids-limit` driver option 停止，未进入 Dockerfile build；改为 bootstrap 后按唯一完整容器 ID 执行 `docker update --pids-limit 512`，再由 inspect 强制验证；update/验证前失败时只允许按完整身份合同及 PID 0/512 清理，验证后继续严格要求 512。Git Bash/WSL 语法与 isolated-image-build 测试通过 | 失败对象/staging 已清理，专用 daemon 保持五镜像、0 容器/卷、仅默认网络；未访问 Docker Desktop/服务器或生产数据，修复提交并同步后再重跑 |
| 2026-09-04 Asia/Shanghai | Buildx 0.30.1 builder 元数据合同校准 | 进行中（真实 Docker 待重跑） | `2e6c800cad711cb3bb49d7324bbdbf7ffe9581a2` 重试在 builder bootstrap 后、项目 build 前因固定空格 Driver 断言停止；失败基线门禁发现旧清理未恢复对象。按本次真实 inspect 改用 Driver/Status 锚定正则；资源由项目 build 前的 `docker update` 规范化并 strict inspect；builder 身份改为固定 image、精确名称/网络、唯一 state 卷和安全/资源属性，不再依赖新版不存在的 labels | 残留 builder/卷/带 token 空网络经完整 ID核对后定向清理，五镜像/0 容器/0 卷/默认网络基线恢复；未使用 prune，未访问服务器或生产数据；新提交后再重跑 |
| 2026-09-04 Asia/Shanghai | 构建批准锚点、归档兼容与锁路径收尾 | 本地代码门禁通过；真实 Docker Release Gate 待重跑 | 隔离构建强制外部 `SUBNEXUS_APPROVED_BUILD_SCRIPT_SHA256`，核对批准 Git blob/当前执行文件并写入三项脚本 SHA；Dockerfile 合同收紧为真实顶层 ARG/FROM；归档校验覆盖 Docker 29 OCI、旧式 Config、gzip 与未压缩 layer；候选 gate 锁定证据目录 inode。Windows Git Bash 与 WSL/Linux 四套发布脚本语法/夹具通过，WSL 动态归档夹具实际通过；`git diff --check` 通过 | 只修改 fork 迁移分支；未修改旧项目、`main`、服务器或生产数据，未构建/加载候选镜像，未部署、重启、切流或修改开关；候选 gate 不解析无独立签名 metadata 的残余风险已记录，待提交、同步 WSL 后从五镜像/零对象基线重跑 |
| 2026-09-04 Asia/Shanghai | 真实隔离 Docker 构建成功（提交 `fa8ac7fa0c45e83a68010467f26d3def2ecd73fd`） | 通过（构建/归档）；候选 runtime gate 与切换仍待办 | 构建生成镜像 `sha256:9a6d5812a54bd5b74b8977c15503d7e8f67a472cf768d954e2cb01b833321a17`，归档 SHA256=`838633aacb3be5ae6e05a51c3931b8b5f7c0e09ce0b502dbdffa9aa4b6e697c4`；构建后保留 6 个 image ID（5 个固定基础镜像 + 1 个候选），release/gate 两个 tag 指向同一候选 ID；candidate archive validator、静态夹具和运行时收尾校验通过。`BUILD_SCRIPT_SHA256`、`APPROVED_BUILD_SCRIPT_SHA256`、`APPROVED_BUILD_SCRIPT_BLOB_SHA256` 三项均为 `fa41a1e9909d8ec9d39f370eb84d9b97bb7f0f5c7e8258aca7387e987b255cd4`。专用 daemon 收尾为 0 容器、0 卷、仅 `bridge/host/none` 网络 | 仅本地 fork/专用 WSL daemon；未访问线上、未修改旧项目或 `main`，未执行生产 SQL/迁移、部署、重启、切流或开关变更 |
| 2026-09-04 Asia/Shanghai | 候选 runtime gate Bash 参数污染修复（待重跑） | 未通过原因已定位并修复；真实 gate 待新提交重建 | 真实 gate 证据 `20260904T090327Z-c7b49564-9813-441e-b805-d036332f9bff` 失败原因是 `candidate_pg_psql` 等三处跨行 pipeline 嵌套在 `if output="$(...)"`，导致 `SELECT 1;` 输出混入调用方控制文本；新增 PG/Redis/HTTP 独立 exec helper、`bash -n` 与参数/stdin 夹具。当前不可据此切换，`cutover_allowed=false`、`manual_review_required=true` | 仅修改 fork 迁移分支和本地测试；未修改旧项目、`main`、服务器或生产数据。必须以新提交重新同步 WSL、从五个固定基础镜像/零对象基线构建并重新运行 runtime gate |
| 2026-09-04 Asia/Shanghai | 隔离构建锁目录身份复核（待重建） | 本地门禁通过；真实构建/gate 待新批准 SHA | 构建脚本在 artifact 目录 FD 加锁前后比较设备/inode/类型/owner/mode，防止路径替换造成锁与输出目录不一致；新增 Linux/WSL FD 指纹夹具。该改动改变 `SUBNEXUS_APPROVED_BUILD_SCRIPT_SHA256`，上一轮 `9ea1d877` 归档不作为最终候选，`cutover_allowed=false` | 仅修改 fork 和本地隔离脚本/测试；未访问或修改线上资产。须新提交、重新同步 WSL、从固定基础镜像基线构建并运行 candidate gate |
| 2026-09-04 Asia/Shanghai | `c91226ef` 隔离构建与 Docker 29 candidate cleanup 复核 | 构建及全部 runtime smoke 通过；gate 因清理诊断合同失败，修复待重建 | c912 镜像 `4f453fe2...f77ab`、归档 SHA256=`82ff92df...1de64`；candidate health、290 条迁移、关闭态、登录、重启、持久化均通过。Docker 29 的缺失网络诊断为 `[]` + `network <id> not found`，对象实际清理后旧 helper 仍置 `cleanup_failed=true`；新增仅接受完整诊断和精确 ref 的匹配及正反夹具。失败证据 token=`20260904T101111Z-5258308c-4591-4beb-8c84-ad381333293e`，不可发布，`cutover_allowed=false` | 仅使用专用 WSL daemon 和 synthetic 基线；未成功登录服务器，未修改旧项目、`main`、生产数据/容器/Nginx。修复提交后必须重新构建并重跑 gate |
| 2026-09-04 Asia/Shanghai | `02774d028` 隔离候选构建与 Docker 29 runtime gate 通过 | 通过（候选构建、归档和 runtime gate；待维护者人工验收） | 使用提交 `02774d028d076e934a59f04fd1ee98598ac693a1`（tree=`023e96b6c629f7d33e8ac2d43b7bd93f960a36f5`）及批准构建脚本 SHA256=`cbec521753cc5fa18bf96a4fd1dd58b32ff026fd76009189e8015a2d201b8aa3` 生成候选镜像 `sha256:b49b764cfc2ca58d9f054c01ef9e17211b89b8280be30534ff83b4b90490a979`；归档 `/root/subnexus-migration/candidate-artifacts/02774d028d076e934a59f04fd1ee98598ac693a1/candidate-image.tar` 大小 `45179904`，SHA256=`45306dfe47e6093d0be67d2446f7d83f7e82ef3407ef2b0f1ed8816489877786`。gate 证据 `/root/subnexus-migration/docker-candidate/20260904T104343Z-2854f544-d1ee-44be-9b58-ff465ee160ac`，证据 SHA256=`7d5dc1141906ee2dcac51dadc17da788e8cf0d4c172d1c71a781a823edc120fb`；`result=passed`、`cleanup_failed=false`、应用/PostgreSQL/Redis 健康、管理员登录与 `auth/me`、290 条迁移、重启/持久化 sentinel、关闭态公开设置均通过，迁移数重启前后一致。专用 daemon 仅保留 synthetic `prod-*` 基线容器/网络，无卷；未连接线上或生产数据库，`cutover_allowed=false`、`manual_review_required=true` | 仅修改 fork 迁移分支及专用 WSL daemon；旧项目、fork `main`、线上服务器、生产 Docker/PostgreSQL/Redis/Nginx 均未修改，未执行生产迁移、部署、重启、切流或开关变更。旧的 `20260904T101111Z-...` 失败证据继续保留，仅作历史记录，不得用于发布 |
| 2026-09-04 Asia/Shanghai | Docker 29 `ConsoleSize`/json-file 日志合同修复 | 通过（本地静态/夹具；线上 prepare 待重跑） | 提交 `617a2fdc1189a452f251f90a6b8e4d554ac2bd05`；切换脚本 SHA256=`98998993c01f7e071b491c8572895914d100ed15ea686df6b50bd1680239991c`。`Tty=false` 的 `[49,202]` 被严格归一化；`json-file` 仅允许成对 `max-file`/`max-size` 并纳入 `log-config.json`、manifest SHA 和候选创建参数。Windows Git Bash 与隔离 WSL Linux 五套发布夹具均通过；隔离 daemon 实际复现 `max-file=5,max-size=20m`。生产两次 prepare 均在备份/数据库/容器操作前停止，应用 ID `be459424b327...` 仍 healthy，未影响线上 | 仅修改 fork 迁移分支和本地测试；尚未安装新脚本或重跑生产 prepare，未停止/重启/迁移/切流/开关变更 |
| 2026-09-04 Asia/Shanghai | 重复 Docker 环境键 fail-closed 兼容合同（未提交） | 本地静态/夹具通过；线上 prepare 待重跑 | `SERVER_TRUSTED_PROXIES` 重复时默认拒绝；仅显式 token + 精确 allowlist + 最终值 SHA256 才允许 last-wins。新增脱敏 `environment-duplicates.tsv`、规范化 `container.env`/manifest SHA、live replay 与 candidate canonicalization 合同；修复 replay 观测状态覆盖准备合同的回滚风险。Windows Git Bash 与 WSL/Linux `bash -n`、production cutover 测试通过，动态夹具覆盖错误批准、无明文证据、序列漂移和 runtime hash 等价性 | 只修改 fork 迁移分支的脚本/测试/手册；未修改旧项目、`main`、服务器、生产数据或开关，未提交/推送；在重新计算批准脚本 SHA、同步 detached clone 并重新运行线上 prepare 前，`cutover_allowed=false` |
| 2026-09-04 Asia/Shanghai | 线上 prepare 挂载双换行兼容修复（未提交） | 首次 prepare 在只读运行元数据阶段 fail-closed；修复待重跑 | Docker 29 `{{range .Mounts}}...{{end}}` 输出与 CLI 终止换行形成空记录，已让 `capture_mounts` 仅跳过全空记录并保留部分记录拒绝；传播枚举改为跨 Bash/MSYS 可移植判断；新增回归夹具。失败目录 `/srv/subnexus-migration/cutover/20260904140820-3616319` 保留，未生成/使用备份，线上应用仍 healthy | 只修改 fork 分支本地脚本/测试/记忆；未修改旧项目、`main`、服务器业务、数据库、Redis、Nginx 或开关；提交并重新安装新 SHA 后才可再次 prepare |
| 2026-09-04 Asia/Shanghai | 应用数据 owner 合同收口（提交 `66cb41b26`） | 本地与 WSL 门禁通过；线上 prepare 待执行 | 新 run 默认严格 `root-only`；唯一审核非 root 例外为 `1000:1000`/安全叶 mode，父链 root-owned；prepare/switch/rollback 重复锁定 owner、mode、设备号和 inode。旧 manifest 保留旧脚本 UID-only 的 legacy root-UID 兼容；修复空 GID resolver 缺陷并恢复通用路径 helper 原语义。脚本 SHA256=`9b7717eab53f898c659958a19b10c088bace3f7695657cb7e6085e5099c5f847`。线上只读复核确认应用 healthy、数据目录 `1000:1000`/`0755`、候选归档/gate/image SHA 一致；源码 `/srv/subnexus-repo` 仍为旧生产分支，需另建 root-owned detached release source | 未停止/重启/重命名线上容器，未执行 SQL/DML/迁移、切流、开关修改或删除；下一步安装唯一新脚本并执行无停机 `prepare`，生成 `READY` 后停在人工切换前 |
