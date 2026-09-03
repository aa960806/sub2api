# SubNexus 二开功能迁移规划

> 版本：v1.8（2026-09-03，生产备份 PostgreSQL 18.4 隔离恢复已通过）
> 状态：最新 `upstream/main` 已合并到独立迁移分支；Batch 1-4 本地实现、合成夹具验证、线上只读预检、生产备份结构校验以及 PostgreSQL 18.4 实际恢复/候选克隆已通过。真实克隆 migration/adoption、Redis RDB 恢复、关闭态与旧版回归和 Docker 候选仍待完成；通过前禁止生产迁移、候选连接生产库、切流或开启功能
> 目标分支：`feature/subnexus-migration`
> 目标仓库：`F:\MySub2\sub2api`

## 1. 目标与不可违背的约束

本次工作是把旧二开项目 `F:\Sub2Api\SubNexus` 中仍需要的业务能力，迁移到本 fork 的上游基线中。最终线上切换使用同一套 PostgreSQL/Redis 数据，要求用户、余额、订阅、订单、用量、API Key、账号和历史记录不丢失；旧版本保留为可启动的回滚候选。

约束如下：

1. 新 fork 已经拥有的上游功能以上游实现为准。旧项目中同名模块不整体复制，只迁移经过差异审计后仍然必要的 fork 专属行为。
2. 每个迁移功能均默认关闭。关闭时不得出现用户入口、业务写入、通知、定时任务副作用或对上游原有行为的改变；管理端可保留配置入口以便验收。
3. 不直接修改 `main`。全部工作在 `feature/subnexus-migration` 或其子分支完成，通过独立提交和评审合并。
4. 已执行迁移文件不可修改、删除、改名或复用编号。所有新结构使用新且唯一的文件名，并由迁移 runner 的 SHA256 校验保护。
5. 代码回滚默认不恢复数据库。只有确认数据库层损坏且得到明确批准时，才使用切换前的数据库备份恢复；新表和可选字段必须设计成旧版本可忽略。
6. 本地开发和功能验收不得直接连接生产库：先使用空库、目标上游迁移和旧项目迁移构造的本地隔离 PostgreSQL/Redis 夹具完成 Batch 1-5；全部本地功能完成后，才在发布阶段使用线上备份恢复出的隔离副本做最终同库验证。
7. 本地 Batch 1-5 全部完成并经维护者验收前，不再推送新的中间提交，不要求服务器拉取，也不执行线上预检、备份、SQL、部署、切换或开关修改。
8. `F:\Sub2Api\SubNexus` 永久作为只读迁移输入；所有文档、代码、提交和测试资产只写入 `F:\MySub2\sub2api`。

## 2. 已确认的基线事实

| 项目 | 旧二开 `SubNexus` | 新 fork `sub2api` |
| --- | --- | --- |
| 分支 | `alignment/v0.1.181-local` | `main`（本规划从此创建迁移分支） |
| HEAD | `62ea35e1c78416fd83e1e41bbb310b307941811a` | fork `main`=`d596d0844`；最新上游=`5097b31457e6dc9f49e5f5c9c72b925ce79543b3`；迁移分支本地候选=`b26c42e08fb190f3915f08949aaaba48dbe61a26`（上游同步父提交=`23d6e8ec0`） |
| 应用版本 | `0.1.135` | `0.2.0`（最新上游） |
| Go 版本 | `1.26.6` | `1.27.0` |
| Git merge-base | 无 | 无 |
| 迁移文件数量 | 268（含测试文件） | 新增 13 个 SubNexus SQL（`9001`–`9013`）及对应契约测试；总文件数随上游和工作树变化，以 Git 为准 |
| 差异 | 两仓库共有 3131 个路径，其中 1335 个内容不同 | 不能整体覆盖或直接 cherry-pick 旧仓库 |

新 fork 当前迁移文件存在历史编号重复（例如多个 `231_*.sql`）；runner 按文件名排序并以“文件名 + SHA256”记录，因此“下一个数字”不能单独作为唯一性依据。本地实施时先从同步后的目标分支实际文件列表选择全局唯一名称并冻结 checksum；最终发布前再用线上 `schema_migrations` 只读证据验证不会冲突，证据不再阻塞本地业务实现。

本轮上游同步证据：`git fetch --prune upstream` 后确认 `upstream/main=5097b31457e6dc9f49e5f5c9c72b925ce79543b3`，其版本为 `0.2.0`；在 `feature/subnexus-migration` 生成 merge commit `23d6e8ec0`。fork 的 `main`、旧项目和线上服务器均未修改。后续所有代码以该 merge 后分支为基线，发现上游新提交时必须重新审计后再同步。

维护者已确认：Creative Workshop/创意工坊与 Media Studio 等同并排除；线上旧版本长期保留；先在本地完成全部功能和测试，维护者验收后才推送并让服务器拉取，最终只在切换阶段短暂关闭线上流量。本地开发使用空库和旧迁移构造的隔离夹具；生产备份恢复出的隔离副本只用于本地候选完成后的最终发布验证，任何阶段都不得让本地候选直接连接生产库。

## 3. 功能裁决矩阵

### 3.1 明确迁移的旧二开能力

| 功能 | 迁移裁决 | 初始开关（拟定名，实施前须核对现有 key） | 风险 |
| --- | --- | --- | --- |
| 签到/签到奖励 | 保留二开业务规则和管理配置；接入目标项目的设置、用户和活动模型 | `subnexus_checkin_enabled=false` | 中 |
| 排行榜 | 保留排行计算、展示和权限边界；重新接入目标项目的用量读取 | `subnexus_leaderboard_enabled=false` | 中 |
| 活动中心 | 仅迁移 `custom` 活动卡片、管理 CRUD 和用户入口；旧库中的转盘、红包、邀请、充值、Battle Pass 等非 `custom` 卡片保留但新用户端永不读取；上游已有活动以上游为准 | `subnexus_activity_center_enabled=false` | 中 |
| 广播/滚动公告扩展 | 保留二开字段、展示和发送策略；不覆盖上游公告实现 | `subnexus_marquee_enabled=false` | 低/中 |
| 首充礼包 | 保留资格、订单确认、幂等发放和管理端配置；退款后不恢复首充资格，关闭态继续清理 terminal reservation | `subnexus_first_recharge_enabled=false` | 高（余额/订单） |
| 邀请活动与奖励 | 只迁移二开活动奖励层、注册奖励重试队列；基础 Affiliate/邀请能力以上游为准，奖励必须幂等 | `subnexus_invite_rewards_enabled=false`；`subnexus_invite_activities_enabled=false`（子开关默认关闭） | 高（余额/返利） |
| 发票事务系统 | 迁移用户申请、管理员处理、文件存储、对账和审计闭环 | `subnexus_invoice_enabled=false`（public 映射为 `invoice_enabled`） | 高 |
| Battle Pass | 迁移赛季、任务进度、经验发放和活动中心联动；保持独立表 | `battle_pass_enabled=false` | 高（用量/奖励） |
| 学生充值优惠 | 迁移资格、奖励日志、退款反向和 scheduler；基础支付以上游为准 | `subnexus_student_recharge_benefit_enabled=false` | 高（余额/退款） |
| 注册 IP 冷却 | 迁移可信客户端 IP 哈希、reservation/finalize/release 和 OAuth 全路径接线 | `registration_ip_cooldown_enabled=false` | 中（注册/反滥用） |
| Channel Monitor V3 | 迁移 V3 展示、时间线和模式边界；探测协议以上游为准 | `channel_monitor_enabled=false`；`channel_monitor_mode=v1` | 中（后台探测） |
| 默认语言 | 迁移服务器默认 locale 和 namespaced/legacy 双键兼容 | 无独立开关；空/非法值不生效 | 低 |
| 客服按钮/Markdown 弹窗 | 迁移公共设置、管理员配置和用户端安全 Markdown 展示 | `customer_support_enabled=false` | 低 |

### 3.2 明确不迁移的旧功能

以下功能在新 fork 中不建立迁移任务、不复制旧表、不恢复旧入口：

- 每日消耗转盘
- 红包雨
- 运行日历
- Media Studio
- Creative Workshop/创意工坊（维护者已确认与 Media Studio 等同）

旧仓库中的相关代码、迁移、测试和文档仅作为“排除项证据”保留在旧仓库，不进入新 fork。

### 3.3 目标项目已具备、以上游为准的模块

Model Plaza、Grok/XAI、插件系统、Composite 路由、Affiliate 基础能力、支付基础接口、批量生图、Prompt Audit/`securityaudit`、中文验证码、Spark Shadow 以及最新账号、网关和调度逻辑，均不得从旧项目整模块替换。只允许做逐文件差异审计：若旧项目有目标 fork 尚不存在且确属保留需求的窄化补丁，单独提出变更、测试和回滚点。

## 4. 迁移方法

### 4.1 代码迁移原则

1. 先建立目标模块清单：路由、handler、service、repository、domain 常量、Wire 注入、前端 API/types/views/router/sidebar/i18n、后台任务和测试。
2. 对每个旧文件建立目标映射，不按目录覆盖。优先把业务规则抽成目标项目可测试的 service/repository，再接入目标项目已有模型、鉴权、设置服务和错误码。
3. 上游已有实现只保留目标版本。二开逻辑通过小型适配层、独立 service 或明确的扩展点接入，避免在网关、计费、调度、认证等核心路径复制旧实现。
4. 不复制旧项目的 `go.mod`、`go.sum`、`frontend/pnpm-lock.yaml`、完整迁移目录、生成的 `wire_gen.go` 或前端构建产物。依赖只有在编译证明必需且经批准后才变更。
5. 修改共享核心路径（认证、计费、余额、订阅、支付、模型路由、代理、迁移 runner、Docker）均按高风险处理，必须有回滚提交和专项测试。

### 4.2 默认关闭标准

每个功能必须同时具备：

- 后端管理员设置，数据库缺省值为 `false`，读取异常按关闭处理；
- 公共设置/前端 feature flag，关闭时隐藏用户菜单、路由入口和首页卡片；
- API 入口在关闭时返回稳定的 feature-disabled 错误（或约定的 404），不得写业务数据；
- 定时任务、队列消费者、通知发送在关闭时不运行或立即 no-op；
- 关闭后不新增活动进度、奖励、返利、发票状态变更或其他业务写入；
- 管理员仍能查看必要历史和执行安全的关闭态维护，但不能借此绕过原有权限或合规校验。

当前所有迁移功能均以字面量 `true` 才视为开启；public settings 未成功加载时前端 feature flag 强制为关闭，即使内存中存在旧缓存也不放行。开关命名、错误码和 public-settings 字段必须先搜索目标项目，避免与已有 `affiliate_enabled`、支付开关、插件开关或上游同名设置冲突。

## 5. 分批实施顺序

### Batch 0：本地基线、盘点和数据库兼容性

- 固定目标分支基线、旧项目提交和版本清单。
- 生成旧/新路径级差异索引，标注“上游同源、二开专属、排除项、未知”。
- 对旧项目所有业务表、字段、索引、存储目录、Redis key、定时任务和 API 做清单。
- 在本地空库、目标上游完整迁移和旧项目已知迁移组合上验证 runner、alias/adoption、重复启动及对象契约。
- 生产 `schema_migrations`、真实对象和生产备份恢复验证移到全部本地批次完成后的 Release Gate；缺少这些证据只阻止发布，不阻止 Batch 1-5 的本地实现。

### Batch 1：活动基础能力（本地实现完成，待最终证据/维护者验收）

范围：签到、活动中心基础聚合、排行榜、广播扩展。先迁移数据读取和管理配置，再迁移奖励写入与前端入口。

活动中心首个切片严格限定为 `activity_type='custom'`：复用旧 `activity_center_items` 表结构但使用目标侧唯一迁移文件；不迁移旧活动 badges、奖励联动、store 状态或 Daily Spin、Invite Lottery、Recharge Wheel、Invite Milestone、Red Packet Rain、Battle Pass 等类型 gate。旧行不删除、不改名，供旧版本回滚继续使用。新开关不继承读取旧 `ACTIVITY_CENTER_CONFIG`，缺失、非法或读取失败均为关闭；管理员显式更新时以同一批设置写入同步旧 JSON 键，保证回滚旧版本后的开关状态一致。用户和管理列表关闭时不查询活动表，管理写操作关闭时拒绝。站内路径及外链由服务端 allowlist 校验。

门禁：关闭开关时旧上游用户请求响应和数据库写入与基线一致；开启后重复签到、重复排行刷新、重复公告发送均幂等；用量查询不得改变网关计费结果。

### Batch 2：首充、邀请、学生优惠与注册保护（本地实现完成，待最终证据/维护者验收）

范围：首充礼包资格和发放、二开邀请活动奖励/注册奖励重试、学生充值优惠和注册 IP 冷却。复用目标项目 payment、affiliate、balance、subscription、Auth 服务，不复制旧支付结算实现。

门禁：订单状态变化、退款/部分退款、并发回调、重复 webhook、重复邀请确认都不能重复发放；退款后的首充资格不得恢复；关闭态 terminal reservation 清理只处理后台补偿，不发奖、不写用户业务数据；奖励流水与余额变动可审计；开关关闭时不改变订单结算和 Affiliate 基础行为。

### Batch 3：发票事务系统（本地实现完成，待最终证据/维护者验收）

范围：独立发票表、用户申请/撤销/重提/历史/下载、管理员接单/释放/驳回/开票/替换/作废/重发邮件、文件存储和对账。

门禁：不修改 `payment_orders` 既有语义；文件目录位于持久化 volume/bind 且权限最小化；上传服务端独立校验类型、大小和 SHA256；状态机、接单抢占、重复回调、邮件重发和下载鉴权有集成测试；`subnexus_invoice_enabled=false` 或 public 映射 `invoice_enabled=false` 时普通用户入口、写 API 和后台任务均关闭，历史数据按既定兼容策略可读。

### Batch 4：Battle Pass、Channel Monitor V3 与站点体验（本地实现完成，待最终证据/维护者验收）

范围：独立赛季、任务、进度、奖励和活动中心联动；Channel Monitor V3 展示/时间线与安全模式归一化；默认语言和客服按钮/Markdown 弹窗的双键兼容。旧项目记录显示 `254_battle_pass.sql` 曾在本地验证，线上是否执行必须以真实数据库查询为准。

门禁：扫描器在关闭时不读取/写入 `usage_logs`；充值任务只计算允许的订单状态和净额；邀请任务复用现有 Affiliate 关系但不写返利账；进度重算、退款减少进度、重复发奖、赛季草稿/发布/归档和未来赛季可见性均有测试。

### Batch 5：集成验收和文档收敛（PG、主机候选 smoke 与旧版回滚通过；持久化 Redis/Docker 待办）

- 执行 PostgreSQL/Redis Testcontainers 或等价隔离环境测试、Docker 候选镜像启动和健康检查；后端全量/重点包测试、前端 typecheck/Vitest/build 已完成。隔离 PostgreSQL 16 的目标迁移、旧迁移和同库接管矩阵已通过（目标 290、旧版 268、接管后 371 条记录）。使用本机 miniredis（`127.0.0.1:56379`）完成非持久化候选主机进程 smoke（`18180`，health、setup、管理员登录、关闭态和二次启动）；旧版 `0.1.135` 在同一 371-record 克隆（`18183`）完成 health、setup、公共设置、有效管理员登录、`auth/me`、管理员只读接口及重启幂等回归。当前前端全量为 282 个测试文件/1954 个测试。持久化 Redis 恢复、Docker daemon/候选镜像运行仍待办；本机 Docker Desktop 当前因 Inference manager 路径错误不可用，不得为此连接线上服务或执行生产命令。
- 运行上游核心回归矩阵：认证、API Key、余额、订阅、订单、退款、用量计费、模型列表、Gateway 各协议、插件、Model Plaza、Grok、批量生图和安全设置。
- 形成并持续维护精简上下文、功能矩阵、迁移台账、切换手册和回滚手册；当前文档已同步本地候选状态，不把历史记忆误当作当前实现状态。

### Release Gate：维护者本地验收后的上传与生产验证

- 维护者先验收本地 Batch 1-5 的功能矩阵、默认关闭行为、测试和候选镜像；验收前不推送新的迁移提交，不让服务器拉取。
- 迁移分支已推送并固定远端发布指针，生产只读 preflight 和 PostgreSQL/Redis/应用数据备份创建及结构校验已经完成；实际恢复能力仍必须由下一步隔离恢复证明。
- 在本机 PostgreSQL 18/Redis 8 隔离恢复库验证 restore、adoption、候选启动和旧版本回归全部通过后，才允许服务器构建固定 release SHA 并进入受控切换。

每个 Batch 使用独立提交；提交信息包含功能名、开关、迁移文件、测试命令和回滚提交。当前先完成 Batch 1 的签到、排行榜、活动中心和公告扩展，再按顺序进入 Batch 2-4；未通过门禁的批次不得依赖后续批次继续开发或开启。维护者验收前只允许本地代码/测试/文档操作，禁止向 `origin` 推送、禁止服务器拉取和任何线上数据库或开关操作。

上述顺序就是“先低风险、后高风险”：活动基础能力主要读用量和写独立活动数据；首充/邀请会触及余额和奖励幂等；发票涉及文件、订单快照和管理员状态机；Battle Pass 同时扫描用量、充值和邀请进度，风险最高。每批完成后可以停在当前版本，不要求一次性开启全部功能。

## 6. 数据库与同库切换方案

### 6.1 迁移编号和 SQL 规则

- 以目标 fork 当前分支实际文件名为准，选择全局唯一的 `NNN_description.sql`；不能复用旧项目编号，也不能因为编号空洞而改写历史文件。
- 每个逻辑变更一个迁移；默认事务执行；只有 `CREATE/DROP INDEX CONCURRENTLY` 才使用 `_notx.sql`，并遵守目标 runner 的非事务限制。
- 新表优先使用 `CREATE TABLE IF NOT EXISTS`、可重复索引创建和显式约束；给旧表加字段时先允许 NULL/安全默认，再分阶段回填和收紧约束。
- 禁止在迁移中删除旧列、重命名旧表、重置余额/订单/用量、覆盖既有设置或写入会开启功能的默认值。
- 每个迁移在空数据库、旧版本数据库、已部分执行数据库和重复启动场景各跑一次；保存文件 SHA256 和 SQL 审查结果。

#### 6.1.1 同内容改名迁移的采用门禁

2026-09-01 的静态审计发现，旧项目与目标 fork 有 23 组“SQL（按 runner 的 `TrimSpace` 规则）内容相同、文件名不同”的迁移。随后确认 2 组文件名不同但 SQL 语义不完全相同、可通过独立后置契约安全接管；本地又为学生充值优惠和注册 IP 冷却加入 2 组独立表 alias，因此当前共 27 组显式 alias。目标 runner 只按完整文件名查询 `schema_migrations`，因此同库启动时会把这些目标文件误判为未执行。下面表格列出 23 组精确内容映射（左侧为旧项目，右侧为目标 fork）：

| 旧文件 | 目标文件 | 重跑分类 |
| --- | --- | --- |
| `175_add_usage_log_long_context_billing.sql` | `174_add_usage_log_long_context_billing.sql` | DDL |
| `177_add_ops_system_logs_host.sql` / `177a_add_ops_system_logs_host_index_notx.sql` | `175_add_ops_system_logs_host.sql` / `175a_add_ops_system_logs_host_index_notx.sql` | DDL/索引 |
| `182_ops_ingress_reject_aggregates.sql` | `183_ops_ingress_reject_aggregates.sql` | 表/索引 |
| `183_auth_cache_invalidation_outbox.sql` | `184_auth_cache_invalidation_outbox.sql` | **函数体含 DML/触发器重建** |
| `189_alipay_mobile_precreate_deep_link.sql` | `186_alipay_mobile_precreate_deep_link.sql` | **含 INSERT** |
| `190_group_reasoning_effort_policy.sql` | `185_group_reasoning_effort_policy.sql` | DDL |
| `193_allow_live_usage_request_type.sql` | `188_allow_live_usage_request_type.sql` | DDL |
| `197_passkey_credentials.sql` | `191_passkey_credentials.sql` | **纯建表/索引** |
| `204_add_usage_logs_api_key_latest_ip_index_notx.sql` | `174_add_usage_logs_api_key_latest_ip_index_notx.sql` | 索引 |
| `205_add_group_peak_rate_multiplier_compat.sql` | `158_add_group_peak_rate_multiplier.sql` | DDL |
| `209_add_usage_log_upstream_model_mismatch_index_notx.sql` | `195_add_usage_log_upstream_model_mismatch_index_notx.sql` | 索引 |
| `235_group_video_model_prices.sql` / `236_group_audio_voice_pricing.sql` / `237_group_search_price_per_1k.sql` | `217_group_video_model_prices.sql` / `218_group_audio_voice_pricing.sql` / `219_group_search_price_per_1k.sql` | DDL |
| `239_group_model_pricing.sql` / `240_enable_grok_media_generation_groups.sql` | `221_group_model_pricing.sql` / `158_enable_grok_media_generation_groups.sql` | **含 UPDATE** |
| `241_group_usage_daily_rollups.sql` / `242_group_usage_rollup_timezone.sql` | `222_group_usage_daily_rollups.sql` / `223_group_usage_rollup_timezone.sql` | **含 INSERT/UPDATE/DELETE** |
| `244_backfill_codex_fingerprint_seed.sql` | `225_backfill_codex_fingerprint_seed.sql` | **含 UPDATE** |
| `245_channel_model_time_pricing.sql` | `225_channel_model_time_pricing.sql` | DDL |
| `246_add_usage_log_effective_model_indexes_notx.sql` | `226_add_usage_log_effective_model_indexes_notx.sql` | 索引 |
| `253_audit_logs.sql` | `180_audit_logs.sql` | **纯建表/索引** |

另外 2 组语义接管规则不属于上表的“内容相同”集合，必须单独执行契约校验：

| 旧文件 | 目标文件 | 接管方式 |
| --- | --- | --- |
| `194_add_group_allow_live.sql` | `189_add_group_allow_live.sql` | 旧 checksum 与目标 checksum 分别精确匹配；验证 `groups.allow_live` 为 `boolean NOT NULL` 且默认值仍为 `false` 后只补写目标元数据 |
| `247_channel_monitor_quota_mode.sql` | `226_channel_monitor_quota_mode.sql` | 旧 checksum 与目标 checksum 分别精确匹配；在同一事务中删除已知重复 `channel_monitors_check_mode_check` 约束，执行完整目标 SQL，再验证 provider/check-mode/外键/设置契约 |

在取得线上记录前，不得假设上述旧文件是否已在生产执行，也不得让未验收候选直接连接生产库。开发阶段先用本地旧迁移组合夹具做以下验证；发布阶段再在生产备份隔离克隆上复跑同一矩阵：

1. 以旧项目迁移记录和目标文件 checksum 建立逐项 `old_filename -> target_filename -> exact_checksum` 清单；23 组精确映射要求 checksum 相同，2 组语义接管分别要求旧/目标 checksum 与审计清单一致。
2. 为目标 runner 增加显式、可审计的 alias/adoption 规则：旧记录存在且目标记录缺失时，在同一 advisory lock 下跳过目标 SQL，并仅记录目标文件名与精确 checksum；不得做全局“同 checksum 即跳过”。
3. `_notx` 索引映射必须额外核对 `to_regclass` 和索引定义；DDL/DML 映射必须在克隆中比较行数、金额、设置和对象定义前后差异。任何 hash 不一致、未知旧文件或对象不匹配都必须硬失败。
4. 在空库、旧库、部分采用、重复启动和旧版本回滚克隆各运行一次；确认旧二进制可忽略新增记录/表，且 alias 记录不会开启任何功能。

这项采用门禁优先于生产候选启动和同库切换，但不阻塞 Batch 1-4 的本地业务实现；任何阶段都不得用手工 `INSERT INTO schema_migrations`、删除记录、改名历史文件或关闭 checksum 校验来“让启动通过”。

#### 6.1.2 目标 runner 的 adoption 实现状态

目标分支已将上述规则固化到 `backend/internal/repository/migrations_runner.go` 和显式 alias/contract 清单中：

- 仅对 27 组审核过的旧文件名→目标文件名进行 adoption：23 组精确内容映射只补写元数据，`189/226` 两组语义接管按显式 replay 规则执行，学生优惠/注册冷却两组只在独立表契约通过时接管；先查目标记录，目标缺失时才查精确旧文件名。
- 同时验证旧记录 checksum、目标文件 checksum，以及表/列/索引有效性与定义、约束、函数、触发器和关键数据契约；任一不符立即 fail-closed。
- 23 组精确映射在同一 PostgreSQL advisory lock 下只补写目标 `schema_migrations` 元数据，不重新执行旧 SQL；`*_notx` 索引也不会重放 `CREATE INDEX CONCURRENTLY`。`189` 与 `226` 使用各自审计过的目标 SQL/后置契约，失败即回滚并拒绝启动。
- Grok 图片开关和 long-context 定价开关属于可被管理员修改的运行时设置，adoption 只核对字段契约并记录当前观察值，不把当前关闭状态误判为迁移失败；Codex seed 和 rollup 单例等不可变数据契约仍严格校验。
- alias 清单、PostgreSQL 规范化触发器事件顺序和完整目标迁移后的契约校验均有单测/integration-only 测试覆盖。线上数据库尚未执行 adoption；实时记录和生产备份隔离恢复是 Release Gate，不是本地 Batch 1-5 的启动条件。

当前新增 SQL 的固定登记（checksum = `SHA256(TrimSpace(SQL))`）如下；任何 SQL 改动都必须重新计算并同步台账，已执行后不得原地修改：

```text
9001_subnexus_activity_center.sql              71a83d4789e33b8d99150f4ad7e48d9195a762c408c6186d1fcb5e2b98016972
9002_subnexus_checkin.sql                      5bdf1548a58ac9e9b3d6304200aa44447562be2916a5a207cc869066c638157c
9003_subnexus_invoice_transactions.sql         49f7d6cadf50ea4959bcfd5d7a2dc52a79b55b38aa372269c70cdf6756ae8b53
9004_subnexus_invite_rewards.sql               435b11b3c2721a06914d6fd593068b1f2e9cf5c0013491009f6ee33079e65e12
9005_subnexus_first_recharge.sql               600d682ee7b80ab27f8f3f064dfbadf71ab4c321f91d10abab2c9a491a2ce867
9006_subnexus_battle_pass.sql                  14152499f8e656d76691b6432e8583de9a3546c288e8a02ad25ef4032173d28d
9007_subnexus_marquee.sql                      19deec6328c814418b66372c066fd2b439bcbdb5c11659394b5fdf32e509128b
9008_subnexus_student_recharge_benefit.sql     f7e2caaf7d0587a5e40cc0f9938166797145fbd6538499355b635ea9ed3e6d24
9009_subnexus_invite_activities_notx.sql       05be0f1771b60af886867b1214d5f3c8e7e6d424e59471a9c7a2fe2a4e003d73
9010_subnexus_registration_ip_cooldown.sql     d84e20270be20d7fe06175c480dea2b99f905b56079a0670ca6f757dfb429683
9011_subnexus_rollout_gates.sql                dc95bd29b26a3807ae0c3457958a673161609adc35318641aa0677b5e11fd03c
9012_subnexus_invite_signup_reward_jobs.sql   7a1bd27748aebf63362045ec171680b5465021fbc756f97a4c2a240385491e0f
9013_subnexus_leaderboard_rewards.sql         856f2fb34f2ff77a24bafa63e49af177d1d6f12c06b549afe0c21a5c6ec759ae
```

### 6.2 本地全部验收后的同库发布预检

必须获得以下证据后才能切换：

1. PostgreSQL 逻辑备份/快照成功，能列目录并校验 SHA256；Redis 持久化和恢复点明确。
2. 记录线上应用镜像/提交、配置摘要、迁移记录、表计数、余额总额、未完成订单、订阅数量、用量窗口和关键索引。
3. 将线上备份恢复到隔离数据库克隆，在克隆上先完成 6.1.1 的改名迁移采用演练，再按候选版本启动；自动迁移成功且无 checksum mismatch，关键行数/金额/对象定义无非预期变化；随后用旧版本连接同一克隆并完成健康检查、登录、余额查询和只读 Gateway 请求。禁止用生产库承担这一步。
4. 候选版本先以全部迁移开关关闭启动，确认旧 API、支付回调和后台任务无异常。
5. 确认文件存储目录、Redis key namespace、定时任务锁和管理员开关不会与旧实例产生双写。

### 6.3 受控切换

1. 进入维护窗口，停止或隔离旧实例的写流量，确认没有运行中的迁移和结算任务。
2. 创建并验证最终备份；记录切换前业务计数和关键账户抽样快照。
3. 启动候选实例连接同一 PostgreSQL/Redis，先保持所有迁移功能关闭；只允许必要的 schema migration。
4. 通过健康、登录、余额、订阅、订单、用量、模型和管理端 smoke 后切换流量。
5. 观察至少一个完整结算/任务周期；逐项开启 Batch 1，再按验收记录开启 Batch 2、3、4。任何异常先关闭对应开关，不立即恢复数据库。
6. 旧版本容器/镜像、源码、配置备份和数据库备份长期保留；维护者确认可以自行删除前，不执行不可逆清理。

### 6.4 当前线上部署边界（以实时检查覆盖历史记录）

旧项目线上文档显示，生产已从早期搬瓦工环境迁移到 OVH。以下是最后一次项目记忆记录的拓扑快照（2026-08-07），只作为盘点起点，不能替代本次实时 `docker inspect`、Nginx 有效配置和数据库查询：

| 项目 | 最后记录值 | 迁移时处理 |
| --- | --- | --- |
| 生产主机 | OVH `51.81.211.97`，SSH 用户 `ubuntu` | 只使用当前主机实时状态；不假设历史 IP/主机仍未变化 |
| 源码工作树 | `/srv/subnexus-repo` | 仅用于 fetch/构建；不覆盖 `/srv/subnexus-migration` 证据和回滚资产 |
| 生产应用 | `subnexus-cutover`，`127.0.0.1:18083 -> 8080` | 从运行容器实时派生网络、端口、挂载、环境和 healthcheck |
| PostgreSQL | `sub2api-postgres`、PostgreSQL 18.4（2026-09-03 只读预检确认） | Release Gate 已验证实际版本、容器和共享网络；后续命令不得打印密码 |
| Redis | `sub2api-redis`，Redis 8，AOF 关闭、RDB 开启 | 以运行实例配置为准，禁止被仓库 Compose 默认值覆盖 |
| Docker 网络 | `sub2api-net`（历史记录另有 CPA 网络） | 不使用曾导致事故的 `sub2api_net` 硬编码；CPA 依赖单独审计 |
| 部署证据/备份 | `/srv/subnexus-migration` | 只追加带时间戳的证据；不得当作 Git 工作树覆盖 |
| 公网入口 | `www.yydsapi.uno`、`image.yydsapi.uno`、`api.yydsapi.uno` | 以 Cloudflare、Nginx、证书和真实 method/path smoke 共同确认 |

历史文档中的 `/www/wwwroot/SubNexus`、`/www/source/SubNexus`、旧端口 `18080`、root SSH 和 `main` 分支属于旧方案或示例，不得直接用于本次迁移。`production-deploy.example.json` 也只是模板；实际 OVH 使用 `ubuntu`、`/srv/subnexus-repo` 和受控生产分支。线上状态变化时，以实时 inspect/有效配置/数据库证据为权威，并在迁移台账中记录快照时间。

### 6.5 生产发布工具边界

旧项目的 `tools/production-deploy/remote-*.sh` 和 `Deploy-SubNexus.ps1` 是针对旧项目提交、迁移编号和生产分支编写的安全工具，不能直接部署本 fork。新 fork 的生产工具只在 Batch 5 完成并进入 Release Gate 后重新审核和固定：

1. 只读 `preflight`：从 live app inspect 派生数据库容器、网络、端口、挂载和 health；数据库会话启用 `BEGIN READ ONLY` 与 `default_transaction_read_only=on`；不执行迁移、构建、重启或切流。
2. 迁移 `apply`：只接受审核后的完整 40 位 release SHA；旧应用继续服务；先生成并验证 PostgreSQL custom-format 备份，再使用 advisory lock、语句超时、文件 checksum 和逐迁移事务执行。
3. 部署 `cutover`：构建使用审核后的 release ref；保存旧容器/镜像/运行元数据；候选健康、鉴权 smoke、非 root、`NoNewPrivs`、restart count 失败时自动回滚。
4. 手工回滚：保留服务器端 `/root/subnexus-rollback.sh` 或新命名的等价脚本；脚本必须作为子进程运行并通过 `bash -n`，不能把带有 `exit` 的长逻辑直接粘贴进交互式 SSH。
5. 所有脚本都必须打印非敏感选择结果（SHA、容器名、网络、端口、备份路径）供人工复核，但绝不输出 `.env`、JWT/TOTP、数据库密码、API Key、Cookie 或私钥。

代码迁移分支在本地完成全部批次并经维护者验收后才推送；从已验收的迁移分支固定独立、不可变的 release ref，再按同一个完整 SHA 执行生产预检和切换。`main` 保持不直接修改，除非维护者另行批准合并。

## 7. 回滚门禁

### 7.1 优先级

1. 功能异常：立即关闭对应开关，保留新版本运行并导出日志。
2. 应用异常：停止候选实例，恢复旧版本容器/镜像连接同一库；只要迁移是新增且向后兼容，旧版本应可继续运行。
3. 数据库异常：冻结写入、保留现场，只有确认数据损坏且批准后才从切换前备份恢复。恢复前必须评估切换期间新增订单、余额和用量，避免静默丢账。

### 7.2 回滚成功标准

- 旧版本启动无迁移 checksum 错误；
- 登录、API Key、余额、订阅、订单、支付回调和用量查询正常；
- 数据库关键计数、余额总额和未完成订单与回滚前的审计记录一致；
- 新增迁移表/字段不阻塞旧版本；
- Redis 恢复后无跨版本 token、锁或缓存污染导致的持续错误。

## 8. 验收清单

### 代码与构建

- `git diff --check`、敏感信息扫描通过；
- `go test ./...`（必要时包含 `-tags unit,integration`）及重点 service/repository/handler 测试通过；
- `go build ./cmd/server` 通过；
- 前端 `pnpm` 版本与 Docker 一致，优先使用 `pnpm install --frozen-lockfile`；typecheck、Vitest、生产构建通过；
- `frontend/pnpm-lock.yaml`、依赖清单、VERSION 和生成产物无意外变化；
- Docker 候选镜像健康、非 root、持久化目录可写且重启后状态保留。

### 功能与数据

- 所有迁移开关默认值和 public settings 均为关闭；关闭态 API/UI/任务/通知/写入逐项验证；
- 每个开启功能有正常、重复、并发、失败、恢复、权限和审计用例；
- 关键数据迁移前后做行数、金额、状态和抽样哈希对账；
- 上游重叠模块与目标 fork 基线做回归，不允许因迁移引入行为漂移；
- 生产前完成真实 PostgreSQL/Redis、支付沙箱和必要上游请求验证；不能以静态编译或单元测试替代。

## 9. 交付文档与台账

本分支最终应包含以下精简文档：

- `SUBNEXUS_MIGRATION_PLAN.md`：本规划和决策记录；
- `SUBNEXUS_FEATURE_MATRIX.md`：旧/新功能逐项裁决、代码映射、开关、迁移和验收状态；
- `SUBNEXUS_PROJECT_CONTEXT.md`：只保留当前架构、运行方式、开关约定、风险边界和维护规则；
- `SUBNEXUS_MIGRATION_LEDGER.md`：每批提交、基线、迁移 checksum、测试证据和回滚点；
- `SUBNEXUS_CUTOVER_RUNBOOK.md`：备份、预检、候选、切流、逐项开启和观察步骤；
- `SUBNEXUS_ROLLBACK_RUNBOOK.md`：开关回退、旧版本启动、数据库恢复条件和数据核对步骤。

旧项目的 `AI_PROJECT_CONTEXT.md`、`AI_CHANGE_MEMORY.md` 和各类历史 Review/Plan 作为迁移输入，不整体复制。精简文档只记录已验证的当前事实；未验证的线上状态必须标注为“待查询”，不得从历史记忆推断。

## 10. 本地实施入口与最终发布硬门禁

本地实施按以下顺序执行，不等待服务器证据：

1. 在 `feature/subnexus-migration` 同步 `upstream/main`，复核上游新增功能和迁移文件，`main` 保持不直接修改。
2. 依次完成 Batch 1 → Batch 2 → Batch 3 → Batch 4，所有功能独立且默认关闭；明确排除每日消耗转盘、红包雨、运行日历和 Media Studio/Creative Workshop。
3. 完成 Batch 5 的后端、前端、隔离 PostgreSQL/miniredis、候选主机和旧版本回滚矩阵，向维护者提交本地验收报告；Docker 镜像与持久化 Redis 恢复仍需运行环境可用后补齐。
4. 维护者验收前只保留本地提交，不再推送，不要求服务器拉取或运行任何迁移资产。

线上 PostgreSQL `schema_migrations`/`atlas_schema_revisions`、Redis/存储拓扑和切换前备份证据已经取得；生产 PostgreSQL custom dump 已由 PostgreSQL 18.4 完整恢复到本机隔离库，并通过 `FILE_COPY` 保留原始恢复库、创建候选克隆。Release Gate 仍缺候选克隆 adoption/启动、Redis 8 RDB 实际加载、关闭态 smoke、该克隆上的旧版本回归和 Docker 候选证据。未满足剩余门禁时可以继续本地修复和测试，但不得让候选连接生产库启动、执行生产迁移、打开开关、切流或替换线上版本。
