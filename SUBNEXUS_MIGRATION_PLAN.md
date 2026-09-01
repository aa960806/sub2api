# SubNexus 二开功能迁移规划

> 版本：v1.2（2026-09-01，根据线上文档审计更新）
> 状态：Batch 0 本地执行中；尚未执行业务代码迁移、生产迁移或线上切换
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
6. 本地候选实例不得直接连接生产库做常规启动测试：目标 runner 会自动执行迁移，后台任务也可能写库。先使用线上备份恢复出的隔离副本测试，最终切换时才连接线上库。
7. 不执行生产 SQL、部署、切换或修改线上开关，除非进入单独的发布执行阶段并得到明确授权。

## 2. 已确认的基线事实

| 项目 | 旧二开 `SubNexus` | 新 fork `sub2api` |
| --- | --- | --- |
| 分支 | `alignment/v0.1.181-local` | `main`（本规划从此创建迁移分支） |
| HEAD | `ccffee6c6` | `d596d0844` |
| 应用版本 | `0.1.135` | `0.1.185` |
| Go 版本 | `1.26.6` | `1.27.0` |
| Git merge-base | 无 | 无 |
| 迁移文件数量 | 268（含测试文件） | 273（含测试文件） |
| 差异 | 两仓库共有 3131 个路径，其中 1335 个内容不同 | 不能整体覆盖或直接 cherry-pick 旧仓库 |

新 fork 当前迁移文件存在历史编号重复（例如多个 `231_*.sql`）；runner 按文件名排序并以“文件名 + SHA256”记录，因此“下一个数字”不能单独作为唯一性依据。实施批次开始前必须从目标分支实际文件列表和线上 `schema_migrations` 查询结果计算可用名称。

维护者已确认：Creative Workshop/创意工坊与 Media Studio 等同并排除；线上旧版本长期保留；本地先部署测试，最终只在切换阶段短暂关闭线上流量。这里的“本地部署测试”必须使用线上备份恢复出的隔离副本，不是生产数据库本身。

## 3. 功能裁决矩阵

### 3.1 明确迁移的旧二开能力

| 功能 | 迁移裁决 | 初始开关（拟定名，实施前须核对现有 key） | 风险 |
| --- | --- | --- | --- |
| 签到/签到奖励 | 保留二开业务规则和管理配置；接入目标项目的设置、用户和活动模型 | `subnexus_checkin_enabled=false` | 中 |
| 排行榜 | 保留排行计算、展示和权限边界；重新接入目标项目的用量读取 | `subnexus_leaderboard_enabled=false` | 中 |
| 活动中心 | 保留仍不被上游覆盖的活动聚合与入口；上游已有活动以其实现为准 | `subnexus_activity_center_enabled=false` | 中 |
| 广播/滚动公告扩展 | 保留二开字段、展示和发送策略；不覆盖上游公告实现 | `subnexus_marquee_enabled=false` | 低/中 |
| 首充礼包 | 保留资格、订单确认、幂等发放和管理端配置 | `subnexus_first_recharge_enabled=false` | 高（余额/订单） |
| 邀请活动与奖励 | 只迁移二开活动奖励层；基础 Affiliate/邀请能力以上游为准，奖励必须幂等 | `subnexus_invite_rewards_enabled=false` | 高（余额/返利） |
| 发票事务系统 | 迁移用户申请、管理员处理、文件存储、对账和审计闭环 | `invoice_enabled=false`（若目标已存在则复用） | 高 |
| Battle Pass | 迁移赛季、任务进度、经验发放和活动中心联动；保持独立表 | `battle_pass_enabled=false`（若目标已存在则复用） | 高（用量/奖励） |

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

开关命名、错误码和 public-settings 字段必须先搜索目标项目，避免与已有 `affiliate_enabled`、支付开关、插件开关或上游同名设置冲突。

## 5. 分批实施顺序

### Batch 0：基线、盘点和数据库兼容性（只读）

- 固定目标分支基线、旧项目提交和版本清单。
- 生成旧/新路径级差异索引，标注“上游同源、二开专属、排除项、未知”。
- 对旧项目所有业务表、字段、索引、存储目录、Redis key、定时任务和 API 做清单。
- 在生产只读窗口查询 `schema_migrations`、`atlas_schema_revisions`、表/列/索引存在性和应用版本；保存脱敏结果。若无法取得线上结果，不得进入同库切换。
- 使用生产数据库脱敏克隆跑目标 fork 的完整迁移演练，并验证旧版本仍可启动。

### Batch 1：活动基础能力

范围：签到、活动中心基础聚合、排行榜、广播扩展。先迁移数据读取和管理配置，再迁移奖励写入与前端入口。

门禁：关闭开关时旧上游用户请求响应和数据库写入与基线一致；开启后重复签到、重复排行刷新、重复公告发送均幂等；用量查询不得改变网关计费结果。

### Batch 2：首充与邀请奖励

范围：首充礼包资格和发放、二开邀请活动奖励。复用目标项目 payment、affiliate、balance、subscription 服务，不复制旧支付结算实现。

门禁：订单状态变化、退款/部分退款、并发回调、重复 webhook、重复邀请确认都不能重复发放；奖励流水与余额变动可审计；开关关闭时不改变订单结算和 Affiliate 基础行为。

### Batch 3：发票事务系统

范围：独立发票表、用户申请/撤销/重提/历史/下载、管理员接单/释放/驳回/开票/替换/作废/重发邮件、文件存储和对账。

门禁：不修改 `payment_orders` 既有语义；文件目录位于持久化 volume/bind 且权限最小化；上传服务端独立校验类型、大小和 SHA256；状态机、接单抢占、重复回调、邮件重发和下载鉴权有集成测试；`invoice_enabled=false` 时普通用户入口、写 API 和后台任务均关闭，历史数据按既定兼容策略可读。

### Batch 4：Battle Pass

范围：独立赛季、任务、进度、奖励和活动中心联动。旧项目记录显示 `254_battle_pass.sql` 曾在本地验证，线上是否执行必须以真实数据库查询为准。

门禁：扫描器在关闭时不读取/写入 `usage_logs`；充值任务只计算允许的订单状态和净额；邀请任务复用现有 Affiliate 关系但不写返利账；进度重算、退款减少进度、重复发奖、赛季草稿/发布/归档和未来赛季可见性均有测试。

### Batch 5：集成验收和文档收敛

- 执行 PostgreSQL/Redis Testcontainers 或等价隔离环境测试、后端全量/重点包测试、前端 typecheck/Vitest/build、Docker 候选镜像启动和健康检查。
- 运行上游核心回归矩阵：认证、API Key、余额、订阅、订单、退款、用量计费、模型列表、Gateway 各协议、插件、Model Plaza、Grok、批量生图和安全设置。
- 形成精简上下文、功能矩阵、迁移台账、切换手册和回滚手册，删除/归档只属于旧项目的长篇历史规划，不把历史记忆误当作当前实现状态。

每个 Batch 使用独立提交；提交信息包含功能名、开关、迁移文件、测试命令和回滚提交。未通过门禁的批次不得依赖后续批次继续开发或开启。

上述顺序就是“先低风险、后高风险”：活动基础能力主要读用量和写独立活动数据；首充/邀请会触及余额和奖励幂等；发票涉及文件、订单快照和管理员状态机；Battle Pass 同时扫描用量、充值和邀请进度，风险最高。每批完成后可以停在当前版本，不要求一次性开启全部功能。

## 6. 数据库与同库切换方案

### 6.1 迁移编号和 SQL 规则

- 以目标 fork 当前分支实际文件名为准，选择全局唯一的 `NNN_description.sql`；不能复用旧项目编号，也不能因为编号空洞而改写历史文件。
- 每个逻辑变更一个迁移；默认事务执行；只有 `CREATE/DROP INDEX CONCURRENTLY` 才使用 `_notx.sql`，并遵守目标 runner 的非事务限制。
- 新表优先使用 `CREATE TABLE IF NOT EXISTS`、可重复索引创建和显式约束；给旧表加字段时先允许 NULL/安全默认，再分阶段回填和收紧约束。
- 禁止在迁移中删除旧列、重命名旧表、重置余额/订单/用量、覆盖既有设置或写入会开启功能的默认值。
- 每个迁移在空数据库、旧版本数据库、已部分执行数据库和重复启动场景各跑一次；保存文件 SHA256 和 SQL 审查结果。

#### 6.1.1 同内容改名迁移的采用门禁

2026-09-01 的静态审计发现，旧项目与目标 fork 有 23 组“SQL（按 runner 的 `TrimSpace` 规则）内容相同、文件名不同”的迁移。目标 runner 只按完整文件名查询 `schema_migrations`，因此同库启动时会把这些目标文件误判为未执行。映射和重跑风险如下（左侧为旧项目，右侧为目标 fork）：

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

在取得线上记录前，不得假设上述旧文件是否已执行，也不得直接让候选 runner 全量启动。隔离克隆必须先做以下验证：

1. 以旧项目迁移记录和目标文件 checksum 建立逐项 `old_filename -> target_filename -> exact_checksum` 清单；只有旧记录 checksum 与文件 checksum 完全一致时才允许采用。
2. 为目标 runner 增加显式、可审计的 alias/adoption 规则：旧记录存在且目标记录缺失时，在同一 advisory lock 下跳过目标 SQL，并仅记录目标文件名与精确 checksum；不得做全局“同 checksum 即跳过”。
3. `_notx` 索引映射必须额外核对 `to_regclass` 和索引定义；DDL/DML 映射必须在克隆中比较行数、金额、设置和对象定义前后差异。任何 hash 不一致、未知旧文件或对象不匹配都必须硬失败。
4. 在空库、旧库、部分采用、重复启动和旧版本回滚克隆各运行一次；确认旧二进制可忽略新增记录/表，且 alias 记录不会开启任何功能。

这项采用门禁优先于 Batch 1-4 的业务迁移；在门禁未通过前，不得用手工 `INSERT INTO schema_migrations`、删除记录、改名历史文件或关闭 checksum 校验来“让启动通过”。

### 6.2 同库前预检

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
| PostgreSQL | `sub2api-postgres`，PostgreSQL 18 | 从应用 `DATABASE_HOST` 验证容器和共享网络；不打印密码 |
| Redis | `sub2api-redis`，Redis 8，AOF 关闭、RDB 开启 | 以运行实例配置为准，禁止被仓库 Compose 默认值覆盖 |
| Docker 网络 | `sub2api-net`（历史记录另有 CPA 网络） | 不使用曾导致事故的 `sub2api_net` 硬编码；CPA 依赖单独审计 |
| 部署证据/备份 | `/srv/subnexus-migration` | 只追加带时间戳的证据；不得当作 Git 工作树覆盖 |
| 公网入口 | `www.yydsapi.uno`、`image.yydsapi.uno`、`api.yydsapi.uno` | 以 Cloudflare、Nginx、证书和真实 method/path smoke 共同确认 |

历史文档中的 `/www/wwwroot/SubNexus`、`/www/source/SubNexus`、旧端口 `18080`、root SSH 和 `main` 分支属于旧方案或示例，不得直接用于本次迁移。`production-deploy.example.json` 也只是模板；实际 OVH 使用 `ubuntu`、`/srv/subnexus-repo` 和受控生产分支。线上状态变化时，以实时 inspect/有效配置/数据库证据为权威，并在迁移台账中记录快照时间。

### 6.5 生产发布工具边界

旧项目的 `tools/production-deploy/remote-*.sh` 和 `Deploy-SubNexus.ps1` 是针对旧项目提交、迁移编号和生产分支编写的安全工具，不能直接部署本 fork。新 fork 应在 Batch 0/5 另行建立以下工具或等价脚本：

1. 只读 `preflight`：从 live app inspect 派生数据库容器、网络、端口、挂载和 health；数据库会话启用 `BEGIN READ ONLY` 与 `default_transaction_read_only=on`；不执行迁移、构建、重启或切流。
2. 迁移 `apply`：只接受审核后的完整 40 位 release SHA；旧应用继续服务；先生成并验证 PostgreSQL custom-format 备份，再使用 advisory lock、语句超时、文件 checksum 和逐迁移事务执行。
3. 部署 `cutover`：构建使用审核后的 release ref；保存旧容器/镜像/运行元数据；候选健康、鉴权 smoke、非 root、`NoNewPrivs`、restart count 失败时自动回滚。
4. 手工回滚：保留服务器端 `/root/subnexus-rollback.sh` 或新命名的等价脚本；脚本必须作为子进程运行并通过 `bash -n`，不能把带有 `exit` 的长逻辑直接粘贴进交互式 SSH。
5. 所有脚本都必须打印非敏感选择结果（SHA、容器名、网络、端口、备份路径）供人工复核，但绝不输出 `.env`、JWT/TOTP、数据库密码、API Key、Cookie 或私钥。

代码迁移分支不直接作为生产发布分支。只有全部批次完成、验收记录齐全后，才从目标 `main` 合并形成独立的不可变 release ref，再按同一个完整 SHA 执行生产预检和切换。

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

## 10. 开始实施前的硬门禁与待确认项

在 Batch 0 的线上证据门禁完成前不得迁移业务代码。需要维护者提供：

1. 线上 PostgreSQL 的只读 `schema_migrations`/`atlas_schema_revisions` 脱敏结果，或可恢复的数据库备份；尤其确认旧项目 `254_battle_pass.sql` 是否已在生产执行。
2. 线上 Redis 持久化/恢复方式和文件存储实际路径、挂载方式、运行 UID/GID。
3. 功能开启顺序暂按本文建议执行：Batch 1 → Batch 2 → Batch 3 → Batch 4，即活动基础 → 首充/邀请 → 发票 → Battle Pass。每批单独验收和开启；这不是要求一次性开启全部功能。

未满足上述门禁时，允许继续做只读盘点、代码映射和隔离测试，但不得连接线上执行迁移、打开开关或替换线上版本。
