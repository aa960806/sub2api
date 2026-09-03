# SubNexus 二开功能迁移矩阵

> 最后更新：2026-09-03。当前代码已在 `feature/subnexus-migration` 本地完成迁移实现和定向/全量测试；隔离 PostgreSQL、miniredis/候选主机 smoke 和旧版回滚克隆已通过，持久化 Redis、Docker 运行和生产备份克隆仍待完成，维护者验收前不得推送或部署。
> 本表是逐模块迁移的唯一状态入口。路径是调查线索，不代表目标代码可以直接复制；“待证据”不等于可上线。

## 保留功能

| ID | 功能 | 旧项目主要证据 | 目标 fork 当前状态 | 迁移策略 | 拟定开关 | 数据/风险 | 状态 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| F01 | 签到与签到奖励 | `backend/internal/service/activity_service.go`；`backend/internal/handler/activity_handler.go`；迁移 `151_activity_rewards.sql`、`155_activity_checkin_ip.sql`、`163_checkin_frozen.sql`、`184_activity_checkin_streaks.sql`；`frontend/src/components/user/dashboard/UserDashboardCheckIn.vue`；旧管理端 `ActivityView.vue` | 已接入目标 service/repository/handler、冻结奖励结算、Wire、用户入口、独立 `/admin/checkin` 配置页和 `9002_subnexus_checkin.sql`；关闭竞态已在事务内复核 | 只迁移签到策略表单，不复制每日转盘/红包雨等排除项；IP 限制、冻结和幂等保留；关闭时无活动表/余额写入 | `subnexus_checkin_enabled=false` | 独立活动数据；中风险 | 本地修复完成，待最终证据/维护者验收 |
| F02 | 排行榜 | `activity_service.go` / `activity_handler.go`；`frontend/src/views/user/LeaderboardView.vue`；管理端排行组件 | 已接入目标用量查询、管理配置、奖励扫描器、用户/管理员路由、`9002_subnexus_checkin.sql` 与 `9013_subnexus_leaderboard_rewards.sql` | 以上游用量字段为准；排行查询不改变计费，奖励目标唯一约束为 `(source,period,user_id)`，调度器关闭时 no-op | `subnexus_leaderboard_enabled=false` | 用量/余额奖励；中/高风险 | 本地实现完成，待最终证据/维护者验收 |
| F03 | 活动中心 | `backend/internal/service/activity_center_service.go`、`activity_center_handler.go`、`admin/activity_center_handler.go`；迁移 `153_activity_center_items.sql` | 已接入目标 service/repository/handler、Wire、路由、公共 flag、前端和 `9001_subnexus_activity_center.sql` | 仅迁移 `custom` 卡片；旧库其他类型行原样保留但不展示、不修改；不迁移 badges/store/奖励联动或任何排除功能 gate | `subnexus_activity_center_enabled=false`；不继承旧 `ACTIVITY_CENTER_CONFIG`，显式更新时同步旧 JSON 键供回滚 | 同库复用表；旧行保留；中风险 | 本地实现完成，待最终证据/维护者验收 |
| F04 | 公告/跑马灯扩展 | 旧 `activity_service.go`、`activity_broadcasts`、`BroadcastMarquee.vue`；上游公告另用 `announcements` | 已接入独立 Marquee repository/service/handler、管理员 CRUD、用户轮询组件及 `9007_subnexus_marquee.sql`；不改上游 Announcement | 新 SQL 和前端二次过滤只允许 `source='admin'`；不迁移自动奖励广播及排除项联动 | `subnexus_marquee_enabled=false`；仅原始值精确 `true` 开启 | 手动广播；低/中风险 | 本地实现完成，待最终证据/维护者验收 |
| F05 | 首充礼包 | `activity_handler.go`、`activity_service.go`、`payment_fulfillment.go`、`setting_service.go`、`settings_view.go`；支付类型 `first_recharge_gift` | 已接入目标订单/余额履约、预约 CAS 并发保护、管理员配置、用户入口和 `9005_subnexus_first_recharge.sql` | 不复制支付结算；资格、订单状态、退款/过期和重复回调均走目标事务与幂等边界；退款后不恢复首充资格，避免退款后重复领取促销余额；功能关闭时仍独立清理 terminal reservation，不发奖、不写用户业务数据 | `subnexus_first_recharge_enabled=false` | 余额/订单；高风险 | 本地实现完成，待最终证据/维护者验收 |
| F06 | 邀请活动与奖励 | `affiliate_service.go`、`affiliate_repo.go`、`admin/affiliate_handler.go`；迁移 `130`-`157`、`160_invite_lottery_activity.sql`、`161_recharge_wheel_activity.sql`、`162_invite_milestone_activity.sql` | 已接入邀请抽奖、累计充值奖励转盘、邀请里程碑、注册奖励持久重试队列及用户/管理员路由；迁移 `9004`、`9009`、`9012`；基础 Affiliate 仍以上游为准 | 奖励流水、余额写入和重复回调幂等；同一事务内严格 gate；不复制每日消耗转盘、红包雨、运行日历或 Media Studio | `subnexus_invite_rewards_enabled=false`；`subnexus_invite_activities_enabled=false`（子开关也默认关闭） | 余额/返利；高风险 | 本地实现完成，待最终证据/维护者验收 |
| F07 | 发票事务系统 | `backend/internal/service/invoice_*.go`、`handler/invoice_handler.go`、`handler/admin/invoice_handler.go`；迁移 `210_invoice_transactions.sql`；`frontend/src/views/user/InvoicesView.vue`、`frontend/src/views/admin/InvoicesView.vue` | 已接入独立表、文件存储、状态机、审计、邮件、管理员处理和 `9003_subnexus_invoice_transactions.sql`；独立 gate 与 legacy 配置均需通过；管理员配置/状态/文件/邮件写操作有 step-up | 不修改 `payment_orders`；退款闭集、管理员 Note/Reason 长度和所有写事务均在事务内 fail-closed；step-up 开启时前端自动 TOTP 重试 | `subnexus_invoice_enabled=false`（public 字段映射为 `invoice_enabled=false`） | 文件/订单/权限；高风险 | 本地修复完成，待最终证据/维护者验收 |
| F08 | Battle Pass | `backend/internal/service/battle_pass*.go`、`handler/battle_pass_handler.go`、`admin/battle_pass_handler.go`；迁移 `254_battle_pass.sql`；`frontend/src/views/user/BattlePassView.vue`；`BATTLE_PASS_SYSTEM_PLAN.md` | 已接入赛季、任务、扫描器、进度、奖励、管理端 step-up、活动中心联动和 `9006_subnexus_battle_pass.sql` | 关闭时不读取/写入用量；充值/邀请任务按目标数据合同计算，奖励幂等，旧版本可忽略新增表 | `battle_pass_enabled=false` | 用量/充值/邀请/余额；高风险 | 本地实现完成，待最终证据/维护者验收 |
| F09 | 学生充值优惠 | 旧 `student_recharge_benefit` service、支付履约和迁移 `199_student_recharge_benefit.sql` | 已接入目标支付履约、退款反向、scheduler、用户/管理员页面和 `9008_subnexus_student_recharge_benefit.sql`；旧 alias 有独立 checksum/契约；管理员配置与身份变更有 step-up | 独立奖励日志和配置快照；关闭时 scheduler、履约和余额写入均 no-op；不改变基础支付金额语义；step-up 开启时前端自动 TOTP 重试 | `subnexus_student_recharge_benefit_enabled=false` | 余额/退款；高风险 | 本地修复完成，待最终证据/维护者验收 |
| F10 | 注册 IP 冷却 | 旧 `159_registration_ip_cooldown.sql`、AuthService 各注册/OAuth 路径 | 已接入可信客户端 IP 哈希、reservation/finalize/release、所有新用户创建路径和 `9010_subnexus_registration_ip_cooldown.sql` | 独立表；失败自动释放；旧版本可忽略；设置缺失/非法按关闭处理，设置读取异常在注册/待定 OAuth 完成路径 fail-closed 拒绝 | `registration_ip_cooldown_enabled=false` | 注册可用性/反滥用；中风险 | 本地修复完成，待最终证据/维护者验收 |
| F11 | Channel Monitor V3 | 旧 V3 页面、卡片、时间线和模式设置 | 已接入目标 V2 体系的 V3 展示/时间线、模式归一化、runner/maintenance gate 和回归测试；不覆盖上游核心探测协议；公开设置与 runtime 对非法模式统一 fail-closed | 复用 `channel_monitor_enabled`，仅 `channel_monitor_mode=v3` opt-in；缺失模式默认 v1，非法模式关闭；关闭时无探测/维护写入 | `channel_monitor_enabled=false`；`channel_monitor_mode=v1` | 监控/后台任务；中风险 | 本地修复完成，待最终证据/维护者验收 |
| F12 | 默认语言 | 旧 `default_language` 设置和前端 locale 行为 | 已接入 namespaced/legacy 双键读取、管理员保存和前端服务器默认语言；显式浏览器/localStorage 选择优先 | 非法值归空；不覆盖用户显式选择；无独立业务表 | 无独立开关（空值/非法值等同关闭） | UI 设置；低风险 | 本地实现完成，待最终证据/维护者验收 |
| F13 | 客服按钮与 Markdown 弹窗 | 旧 `CustomerSupportButton.vue`、`CustomerSupportModal.vue` 及设置文档 | 已接入公共设置、管理员配置、用户全局按钮/弹窗和 Markdown 安全渲染；显式白名单、协议限制、blank-target opener 防护和 XSS 测试已补齐 | 仅 `customer_support_enabled=true` 且内容非空显示；失败/缺失隐藏；namespaced/legacy 双键同步便于回滚 | `customer_support_enabled=false` | 公共内容/前端；低风险 | 本地修复完成，待最终证据/维护者验收 |

## 明确排除

| ID | 功能 | 旧项目证据 | 裁决 | 操作 |
| --- | --- | --- | --- | --- |
| X01 | 每日消耗转盘 | `frontend/src/views/user/DailySpinView.vue`；`158_daily_spin_activity.sql`；activity service/handler | 不迁移 | 不复制代码、迁移、路由、入口或数据恢复逻辑 |
| X02 | 红包雨 | `frontend/src/views/user/HourlyRedPacketRainView.vue`；`198_hourly_red_packet_rain.sql`；`hourly_red_packet_rain*.go` | 不迁移 | 仅保留旧仓库历史证据 |
| X03 | 运行日历 | `frontend/src/views/user/UptimeCalendarView.vue`；`154_uptime_calendar.sql`；`uptime_calendar*.go` | 不迁移 | 不建立目标表或后台任务 |
| X04 | Media Studio / Creative Workshop | `frontend/src/views/user/MediaStudio*.vue`；`164_media_studio.sql`；`media_studio*.go`；创意工坊文档 | 不迁移 | 不恢复媒体工作台入口、表或定价逻辑 |

### 排除项静态回归结论（2026-09-02）

- 目标 fork 当前提交及迁移工作树没有新增上述四项的 SubNexus 文件、路由、handler/service 或迁移；排除项名称仅出现在审计文档/契约测试的防误迁移断言中。
- `RechargeWheelView.vue` 对应累计充值奖励转盘，属于批准保留的邀请活动切片，不等同于 `DailySpinView.vue` 的每日消耗转盘。
- 目标上游若自行提供同名能力，采用上游代码和设置语义；迁移分支不覆盖、不复制旧项目版本。

## 当前状态与统一开关

- 所有迁移功能默认关闭；`9011_subnexus_rollout_gates.sql` 只以 `ON CONFLICT DO NOTHING` 补齐缺失 gate，不覆盖管理员已有值。
- 当前候选只存在本地工作树；代码门禁、隔离 PostgreSQL、miniredis/候选主机 smoke 和旧版回滚克隆已通过，但 Docker 镜像运行、持久化 Redis 恢复和生产备份克隆仍待验证；未推送当前改动、未部署、未连接线上 PostgreSQL/Redis，Release Gate 尚未通过。
- `RechargeWheelView.vue` 是累计充值奖励转盘，属于 F06；明确排除的是每日消耗转盘。

## 以上游为准的重叠模块

| ID | 模块 | 目标侧事实 | 迁移裁决 | 审计要求 |
| --- | --- | --- | --- | --- |
| U01 | Model Plaza | 目标已有上游实现 | 不复制旧模块 | 只比较必要的显示/定价差异 |
| U02 | Grok/XAI | 目标已有上游实现 | 不引入旧 Grok 实现 | 只保留明确批准的窄化差异 |
| U03 | 插件系统 | 目标已有新版插件系统 | 不复制旧插件链路 | 保持上游 disabled-by-default 语义 |
| U04 | Composite 路由 | 目标已有上游路由 | 不覆盖目标 gateway/scheduler | 做协议、计费和路由回归 |
| U05 | Affiliate 基础能力 | 目标已有基础服务/设置 | 只迁移活动奖励层 | 余额、返利、注册和幂等专项审计 |
| U06 | 支付基础能力 | 目标已有支付 provider/订单 | 不复制旧结算和回调 | 首充/发票只通过目标接口接入 |

## 每项完成定义

迁移项只有同时满足以下条件，状态才能改为“通过”：

1. 旧规则与目标实现的差异已记录，目标上游行为没有被覆盖。
2. 后端、前端、任务、通知和数据库均有关闭态测试；默认值为关闭。
3. 正常、重复、并发、权限、失败恢复和审计用例通过。
4. 目标迁移文件名称唯一、checksum 已登记，并在空库/旧库/重复启动/旧版本回滚克隆上验证。
5. `go test`、前端 typecheck/Vitest/build、Docker 候选和隔离 PostgreSQL/Redis 证据齐全。
6. 具备独立提交和明确回滚点，且未改变 `frontend/pnpm-lock.yaml`、依赖或 VERSION，除非另有批准。

## Batch 1-4 本地实现证据（2026-09-03）

### 目标 fork 缺口与接入点

| 层 | 当前目标事实 | 实施约束 |
| --- | --- | --- |
| 后端 handler/service | 已新增窄化 service/repository/handler，覆盖 F01-F13 的目标接入点 | 不复制旧项目排除功能；所有余额、订单、发票和注册写入均有独立 gate/事务边界 |
| Wire | `go generate ./cmd/server` 已成功，生成文件与 provider 同步 | 继续使用生成流程；生成文件变更必须纳入提交和编译测试 |
| 用户路由 | F01-F11 用户入口均有认证、route meta 和 flag guard；关闭时 fail-closed | 关闭态不发起活动 API、轮询、探测、scheduler 或写 service |
| 管理路由 | F01-F09/F11 管理入口已接入；F01 签到配置页可在关闭态准备策略，发票/学生优惠写操作接入 step-up | 保留审计中间件、权限和 step-up；管理员入口不等于功能开启 |
| 设置 | `setting_parse.go`、public settings、管理员 DTO 和双键兼容已同步 | 缺失、读取错误、非法值和非字面量 `true` 均按关闭处理；`9011` 只补缺失默认 gate |
| 公开设置/SSR | 新增 F01-F11 公开字段及 F12/F13 site 设置，前后端契约测试已同步 | 不公开奖励概率/敏感凭据；加载失败清空缓存，避免旧 true 残留 |
| 前端入口 | 路由、侧栏、Dashboard/App 全局组件与开关已接入 | UI 与 API 双重门禁；排除项无实际入口 |

### Batch 1 数据对象映射

旧项目保留的最小数据库对象如下，旧编号不能直接复制到目标：

- `activity_reward_logs`（目标 `9002_subnexus_checkin.sql`，兼容旧 `151_activity_rewards.sql`）：用户外键、`source/period/user_id` 唯一约束、金额正数检查、用户/来源索引；签到 IP 与冻结字段由同一目标迁移覆盖。
- `activity_checkin_streaks`（`184_activity_checkin_streaks.sql`）：独立用户主键、非负连续天数、最后签到日期；便于回滚时旧版本忽略。
- `activity_broadcasts`（目标 `9007_subnexus_marquee.sql`，兼容旧 `152_activity_extensions.sql`）：活动广播 CRUD/窗口/优先级；不得与目标 `announcements` 表混用或覆盖；新代码只读写 `source='admin'`。
- `activity_center_items`（旧 `153_activity_center_items.sql`，目标 `9001_subnexus_activity_center.sql`）：独立 slug、窗口、metadata JSONB 和可见索引；本批用户端和 CRUD 只允许 `custom`。旧库中的 `daily_spin`、`invite_lottery`、`recharge_wheel`、`invite_milestone`、`hourly_red_packet_rain`、`battle_pass` 等行保留供回滚，但绝不进入新用户列表。

目标上游 `045_add_announcements.sql`/`068_add_announcement_notify_mode.sql` 已存在同名公告核心，不能复制或改写。当前新增业务迁移为 `9001`–`9013`，每个文件的 TrimSpace-SHA256 以台账登记为准；线上 `schema_migrations` 仍须在 Release Gate 只读核对。

### 关闭态与测试门禁

- `ACTIVITY_CONFIG` 缺失、解析错误或开关缺失均按关闭处理；不能沿用旧 `DefaultActivityConfig()` 中签到/排行/广播默认开启的语义。
- 排行查询只能读取目标 `usage_logs`/`users`，不得改变网关计费；调度器在排行/周期开关关闭时必须 no-op。
- 签到写入须在事务内锁定/创建 streak，重复日期、并发请求和 IP 限制必须幂等；关闭时不写任何活动表或余额。
- 活动中心使用独立新开关，不继承旧 `ACTIVITY_CENTER_CONFIG`；用户列表关闭时返回 `{enabled:false,items:[]}` 且不查表，管理列表关闭时为空且不查表，管理写操作返回禁用错误。管理员配置接口始终保留用于显式开启。
- 已通过本地后端全量（默认与 `unit` 标签）、前端 typecheck/Vitest（280 个文件/1950 个测试）/build、迁移契约及重点并发/关闭态测试；隔离 PostgreSQL、miniredis/候选主机 smoke 和旧版回滚克隆已通过，仍待 Docker 候选镜像、持久化 Redis 恢复和生产备份克隆证据。前端只在 flag 开启后加载活动 API。
