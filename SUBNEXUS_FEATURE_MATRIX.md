# SubNexus 二开功能迁移矩阵

> 本表是逐模块迁移的唯一状态入口。路径是调查线索，不代表目标代码可以直接复制；“待审计”必须完成目标版本差异和关闭态验证后才能改为“保留”。

## 保留功能

| ID | 功能 | 旧项目主要证据 | 目标 fork 当前状态 | 迁移策略 | 拟定开关 | 数据/风险 | 状态 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| F01 | 签到与签到奖励 | `backend/internal/service/activity_service.go`；`backend/internal/handler/activity_handler.go`；迁移 `151_activity_rewards.sql`、`155_activity_checkin_ip.sql`、`163_checkin_frozen.sql`、`184_activity_checkin_streaks.sql`；`frontend/src/components/user/dashboard/UserDashboardCheckIn.vue` | 未发现旧活动 service/迁移；待确认上游公共设置和用户模型 | 按目标 service/repository/handler 重新接入，保留规则和幂等 | `subnexus_checkin_enabled=false` | 独立活动数据；中风险 | 未开始 |
| F02 | 排行榜 | `activity_service.go` / `activity_handler.go`；`frontend/src/views/user/LeaderboardView.vue`；管理端排行组件 | 上游可能有用量统计/排行，待差异审计 | 以上游查询和计费字段为准，只迁移二开展示/规则差异 | `subnexus_leaderboard_enabled=false` | 只读用量优先；中风险 | 未开始 |
| F03 | 活动中心 | `backend/internal/service/activity_center_service.go`、`activity_center_handler.go`、`admin/activity_center_handler.go`；迁移 `152_activity_extensions.sql`、`153_activity_center_items.sql` | 目标无旧活动中心模块，待确认上游入口 | 独立聚合层；不得覆盖上游已有活动 | `subnexus_activity_center_enabled=false` | 活动入口和可见性；中风险 | 未开始 |
| F04 | 公告/跑马灯扩展 | `backend/internal/service/announcement_service.go`、`announcement_handler.go`；`frontend/src/components/common/BroadcastMarquee.vue`；迁移 `045_add_announcements.sql`、`068_add_announcement_notify_mode.sql` | 目标已有 `AnnouncementService`/`AnnouncementHandler`/`announcements` 表，公告核心以上游为准；目标未发现旧活动跑马灯 | 不复制 045/068 或公告核心；仅逐字段审计缺失的活动广播展示/发送策略，必要时独立扩展并加 guard | `subnexus_marquee_enabled=false` | 可能触及通知；低/中风险 | 待审计 |
| F05 | 首充礼包 | `activity_handler.go`、`activity_service.go`、`payment_fulfillment.go`、`setting_service.go`、`settings_view.go`；支付类型 `first_recharge_gift` | 目标有支付/订单基础，但无旧活动闭环 | 复用目标订单、余额和幂等服务；不得复制支付实现 | `subnexus_first_recharge_enabled=false` | 余额/订单；高风险 | 未开始 |
| F06 | 邀请活动与奖励 | `affiliate_service.go`、`affiliate_repo.go`、`admin/affiliate_handler.go`；迁移 `130`-`157`、`160_invite_lottery_activity.sql`、`162_invite_milestone_activity.sql` | 目标有 Affiliate 基础能力，以上游为准 | 只迁移活动奖励层；奖励流水和重复回调必须幂等 | `subnexus_invite_rewards_enabled=false` | 余额/返利；高风险 | 未开始 |
| F07 | 发票事务系统 | `backend/internal/service/invoice_*.go`、`handler/invoice_handler.go`、`handler/admin/invoice_handler.go`；迁移 `210_invoice_transactions.sql`；`frontend/src/views/user/InvoicesView.vue`、`frontend/src/views/admin/InvoicesView.vue` | 目标无发票表和 service；目标迁移编号 `210` 已被上游占用/语义不同，待线上核对 | 以独立表、文件存储、状态机和审计闭环移植；不修改 `payment_orders` | `invoice_enabled=false` | 文件/订单/权限；高风险 | 未开始 |
| F08 | Battle Pass | `backend/internal/service/battle_pass*.go`、`handler/battle_pass_handler.go`、`admin/battle_pass_handler.go`；迁移 `254_battle_pass.sql`；`frontend/src/views/user/BattlePassView.vue`；`BATTLE_PASS_SYSTEM_PLAN.md` | 目标无 Battle Pass 代码/迁移；线上是否已有表待查 | 迁移独立表和默认关闭 runtime；已发布配置快照与奖励幂等规则需保留 | `battle_pass_enabled=false` | 用量/充值/邀请/余额；高风险 | 未开始 |

## 明确排除

| ID | 功能 | 旧项目证据 | 裁决 | 操作 |
| --- | --- | --- | --- | --- |
| X01 | 每日消耗转盘 | `frontend/src/views/user/DailySpinView.vue`；`158_daily_spin_activity.sql`；activity service/handler | 不迁移 | 不复制代码、迁移、路由、入口或数据恢复逻辑 |
| X02 | 红包雨 | `frontend/src/views/user/HourlyRedPacketRainView.vue`；`198_hourly_red_packet_rain.sql`；`hourly_red_packet_rain*.go` | 不迁移 | 仅保留旧仓库历史证据 |
| X03 | 运行日历 | `frontend/src/views/user/UptimeCalendarView.vue`；`154_uptime_calendar.sql`；`uptime_calendar*.go` | 不迁移 | 不建立目标表或后台任务 |
| X04 | Media Studio / Creative Workshop | `frontend/src/views/user/MediaStudio*.vue`；`164_media_studio.sql`；`media_studio*.go`；创意工坊文档 | 不迁移 | 不恢复媒体工作台入口、表或定价逻辑 |

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

## Batch 1 本地审计证据（2026-09-01）

### 目标 fork 缺口与接入点

| 层 | 当前目标事实 | 实施约束 |
| --- | --- | --- |
| 后端 handler/service | `backend/internal/handler/handler.go` 的 `Handlers`/`AdminHandlers` 没有 Activity 或 ActivityCenter；目标 `backend/internal/service` 没有对应实现 | 新建窄化 service/handler，只接入保留的签到、排行、活动中心和公告扩展；不得复制旧项目中排除功能的方法 |
| Wire | `backend/internal/handler/wire.go` 与 `backend/cmd/server/wire.go` 没有活动依赖；`wire_gen.go` 为生成文件 | 先改 provider/wire 输入，再用项目生成流程更新 `wire_gen.go`；不手工复制旧生成文件 |
| 用户路由 | `backend/internal/server/routes/user.go` 目前有公告、兑换、订阅、渠道监控等，无 `/activity/*` 或 `/activity-center` | 每组路由使用独立 feature guard；关闭时稳定返回禁用错误且不得进入写 service |
| 管理路由 | `backend/internal/server/routes/admin.go` 有公告、Affiliate、设置等，无活动路由 | 管理员配置路由可保留但必须按功能开关限制写操作；审计中间件和限流沿用目标组级中间件 |
| 设置 | `setting_parse.go:22-27` 在任意设置存在时提前返回；仅向 defaults map 添加 key 不能覆盖已有库 | 新增独立布尔 key 的数据库默认/兼容迁移，并在读取缺失或错误时 fail-closed；不要复用 `ACTIVITY_CONFIG` 的旧启用默认值 |
| 公开设置/SSR | `setting_public.go`、`handler/dto/settings.go`、`PublicSettingsInjectionPayload` 和前端 `stores/app.ts` 需字段同步；目标 `featureFlags.ts` 只有上游 flags | 只有用户 UI 需要时才公开 `subnexus_*_enabled`；不公开奖励概率或敏感配置；新增字段必须更新 schema drift 测试 |
| 前端入口 | 目标 router/sidebar/dashboard/App.vue 未发现活动中心、排行榜、签到或 BroadcastMarquee | 后端 guard、公开 flag、路由 meta、菜单和全局组件必须一起落地；关闭态不发起轮询或写请求 |

### Batch 1 数据对象映射

旧项目保留的最小数据库对象如下，旧编号不能直接复制到目标：

- `activity_reward_logs`（`151_activity_rewards.sql`）：用户外键、`source/period/user_id` 唯一约束、金额正数检查、用户/来源索引；签到 IP 与冻结字段由 `155_activity_checkin_ip.sql`、`163_checkin_frozen.sql` 追加。
- `activity_checkin_streaks`（`184_activity_checkin_streaks.sql`）：独立用户主键、非负连续天数、最后签到日期；便于回滚时旧版本忽略。
- `activity_broadcasts`（`152_activity_extensions.sql`）：活动广播 CRUD/窗口/优先级；不得与目标 `announcements` 表混用或覆盖。
- `activity_center_items`（`153_activity_center_items.sql`）：独立 slug、窗口、metadata JSONB 和可见索引；只允许批准的活动类型参与 feature gating。

目标上游 `045_add_announcements.sql`/`068_add_announcement_notify_mode.sql` 已存在同名公告核心，不能复制或改写。新迁移必须从目标分支实际文件列表与线上 `schema_migrations` 共同计算唯一文件名和 checksum。

### 关闭态与测试门禁

- `ACTIVITY_CONFIG` 缺失、解析错误或开关缺失均按关闭处理；不能沿用旧 `DefaultActivityConfig()` 中签到/排行/广播默认开启的语义。
- 排行查询只能读取目标 `usage_logs`/`users`，不得改变网关计费；调度器在排行/周期开关关闭时必须 no-op。
- 签到写入须在事务内锁定/创建 streak，重复日期、并发请求和 IP 限制必须幂等；关闭时不写任何活动表或余额。
- 活动中心用户列表关闭时返回稳定禁用响应/空结果（按目标 API 约定），管理端历史读取不绕过权限。
- 需要新增目标测试：缺失设置 fail-closed、关闭 API 无写入、路由/菜单隐藏、调度器 no-op、重复/并发奖励和审计日志；前端只在 flag 开启后加载活动 API。
