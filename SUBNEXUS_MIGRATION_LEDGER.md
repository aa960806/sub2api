# SubNexus 迁移台账

> 最后更新：2026-09-03。台账记录每个批次的基线、状态、证据、迁移文件和回滚点。状态只能向前追加，不删除历史状态。

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
| 生产数据库状态 | 只读预检与切换前备份已完成；未执行迁移/DDL/DML，隔离恢复前禁止候选连接生产库 |

## Batch 0 门禁

| 编号 | 门禁 | 状态 | 证据/备注 |
| --- | --- | --- | --- |
| B0-1 | 从目标 `main` 建立独立迁移分支 | 通过 | `feature/subnexus-migration`，HEAD 与 `main` 同源 |
| B0-2 | 读取规划、旧项目记忆和线上文档 | 通过 | 记录于 `SUBNEXUS_CHANGE_MEMORY.md` |
| B0-3 | 创建项目上下文、功能矩阵、台账 | 通过 | 上下文、功能矩阵、台账、变更记忆及切换/回滚手册已建立 |
| B0-4 | 旧/新逐文件功能与迁移差异盘点 | 通过（本地） | 已完成保留/排除功能的后端、前端、路由、设置、迁移对象和目标接入点映射；线上表状态仍单独以 B0-5 为准 |
| B0-4a | 同内容/语义改名迁移逐项审计 | 通过（本地静态） | 共 27 组显式 alias：历史 23 组内容相同、2 组语义接管，另有学生优惠/注册冷却 2 组独立表接管；含 DML/索引/约束的重放风险已登记于规划 6.1.1；需隔离库和线上记录验证 adoption |
| B0-4b | 改名迁移 alias/adoption runner 与对象契约 | 本地实现通过；发布证据待办 | 当前工作树为 27 组显式映射；本地测试不阻塞 Batch 1-5，生产备份隔离恢复仍是 Release Gate |
| B0-8 | 新 fork 上游基线构建/测试 | 通过（本地候选） | 后端默认构建与 `unit` 标签全量测试、`go vet` 通过；前端 `pnpm typecheck`、Vitest（282 个文件/1954 个测试）、`pnpm build` 通过；主机进程 smoke 通过不代表 Docker、持久化 Redis 或生产通过 |

## Release Gate（保留历史编号）

以下项目只阻止上传后的生产发布，不阻止 Batch 1-5 本地开发：

| 编号 | 门禁 | 状态 | 证据/备注 |
| --- | --- | --- | --- |
| B0-5 | 线上容器/数据库/Redis 只读状态 | 通过 | 固定脚本与 SHA256 校验通过；证据 `/srv/subnexus-migration/preflight/20260903072817/evidence.txt`，无迁移或部署 |
| B0-6 | 线上 PostgreSQL、Redis 与应用数据备份 | 通过（创建与结构校验） | `/srv/subnexus-migration/backups/20260903T073714Z`；PG custom dump/list、globals、Redis RDB/check、应用 tar 和全量 SHA256 均通过 |
| B0-7 | 生产备份隔离恢复、候选迁移和旧版本回归 | 进行中 | 服务器空间不足以安全恢复约 67 GB 副本；下一步下载到本机 `F:` 盘，用 PostgreSQL 18/Redis 8 隔离恢复，未通过前禁止生产迁移/候选启动/切流 |

## 实施批次

| 批次 | 范围 | 依赖 | 开关策略 | 状态 |
| --- | --- | --- | --- | --- |
| Batch 1 | 签到、排行榜、活动中心、公告扩展 | 本地 B0-1 至 B0-4b/B0-8、`upstream/main=5097b3145` 已同步 | 每项独立默认关闭 | 本地实现完成，待最终证据/维护者验收 |
| Batch 2 | 首充礼包、二开邀请奖励、学生充值优惠、注册 IP 冷却 | Batch 1 规则、订单/Affiliate/Auth 审计 | 默认关闭，奖励/注册 reservation 幂等 | 本地实现完成，待最终证据/维护者验收 |
| Batch 3 | 发票事务系统 | 数据目录、订单快照、邮件和权限审计 | `subnexus_invoice_enabled=false`（public 映射 `invoice_enabled`） | 本地实现完成，待最终证据/维护者验收 |
| Batch 4 | Battle Pass、Channel Monitor V3、默认语言、客服按钮 | 用量/充值/邀请数据合同；上游监控基础 | 所有功能/模式默认关闭或回退安全默认 | 本地实现完成，待最终证据/维护者验收 |
| Batch 5 | 集成、Docker、隔离 PostgreSQL/Redis、旧版本回归和发布候选 | Batch 1-4 本地实现 | 所有功能仍关闭直到逐项批准 | PostgreSQL 隔离迁移/接管、miniredis 主机 smoke、候选主机进程 smoke、旧版 0.1.135 回滚克隆已通过；生产 PG/Redis/应用备份及结构校验已通过，生产备份实际隔离恢复和 Docker 候选镜像仍待办 |

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
| 隔离恢复验证 | 待执行 | 本地/隔离 PostgreSQL | 未开始 |

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
| 2026-09-03 Asia/Shanghai | Batch 5 隔离 PostgreSQL 接管和幂等验证 | 通过（PostgreSQL 子门禁） | 在 `F:\MySub2\.subnexus-pg16-20260903`（仅监听 `127.0.0.1:56000`）复核目标 290 条迁移、旧版 268 条迁移、旧库向当前 runner 的首次接管/第二次幂等/旧集合重复执行；接管后 `schema_migrations=371`，目标 checksum（兼容白名单除外）匹配且无 invalid index。Redis 持久化恢复、Docker 镜像运行和生产备份克隆仍待办 | 未访问线上；未执行生产 SQL、备份、部署、重启、切流或开关修改 |
| 2026-09-03 Asia/Shanghai | Batch 5 隔离 miniredis 与候选主机 smoke | 通过（轻量运行时子门禁） | miniredis 仅监听 `127.0.0.1:56379`；候选主机进程 `18180` 的 health、setup、管理员登录、关闭态响应和二次启动通过；不代表 Redis 持久化、Docker 镜像或生产通过 | 未访问线上；Redis 数据仅为本地临时测试 |
| 2026-09-03 Asia/Shanghai | 旧版 0.1.135 同库回滚克隆回归 | 通过（隔离克隆） | 在 `subnexus_old_regression_login_20260903`（`schema_migrations=371`、users=1、settings=52）首轮及重启后验证 health/setup/public settings、有效管理员登录、`auth/me`、管理员只读 GET；旧版不存在的新迁移路由按预期 404，data-management deprecated 按预期 503；用户/迁移计数不变，仅第二次登录产生预期审计记录，无 checksum 错误 | 未访问线上；旧版进程和 Redis 均使用本机隔离资源 |
| 2026-09-03 Asia/Shanghai | 再次复核旧项目渠道监控 V3 修正 `62ea35e1c` | 通过（无需代码重放） | 7 个源码/测试文件中 5 个逐字节一致，2 个仅注释/排版/测试位置差异；V3 时间线、尾部空桶、90/80 可用性阈值行为一致；目标专项 Vitest 2 文件/16 测试和受影响文件 ESLint 通过 | 仅读旧项目、只写目标记忆文档；未访问线上、未修改 `main`、未部署、未执行 SQL 或开关变更 |
| 2026-09-03 Asia/Shanghai | 主审报告独立复核与确认问题修复 | 通过（本地代码/文档门禁；待维护者复审） | 补齐 F01 签到管理页/路由/侧栏/i18n；公开设置与 runtime 对非法 channel monitor mode 统一 fail-closed；客服 Markdown 白名单/协议/blank-target 防护；签到冲突目标收紧；活动中心/跑马灯写入在事务内锁定开关；注册冷却设置读取异常拒绝待定 OAuth 完成；发票和学生优惠写操作接入 step-up 及前端 TOTP 重试；校正旧项目参考 SHA；前端全量 282/1954，后端定向 service/repository/routes 测试通过 | 仅修改目标迁移分支；未修改旧项目、`main`、服务器、线上 PostgreSQL/Redis，未推送、部署或修改生产开关 |
| 2026-09-03 Asia/Shanghai | 主审复核后的后端全量门禁收尾 | 通过（本地；待维护者复审） | `go test ./... -run '^$' -count=1 -p=1 -timeout=45m`、`go vet ./...`、`go test -tags unit ./... -count=1 -p=1 -timeout=30m` 均退出码 0；全仓编译、静态分析和 unit 测试通过。前端全量 282/1954、typecheck、lint、build 仍保持通过 | 未访问线上、未连接生产 PostgreSQL/Redis；未修改旧项目、fork `main`、生产开关；未推送或部署 |
