# SubNexus 迁移项目上下文

> 本文件是新 fork 的长期维护入口。任何 AI 或开发者在修改代码前必须先阅读本文件、`SUBNEXUS_CHANGE_MEMORY.md`、`SUBNEXUS_MIGRATION_PLAN.md` 和 `SUBNEXUS_MIGRATION_LEDGER.md`。
>
> 当前权威状态：2026-09-09（Asia/Shanghai）。`F:\Sub2Api\SubNexus` 保留二开用户端界面和 `F:\Rain` 首页已迁入，本轮包含登录后用户端视觉性能分级、菜单层级与签到文案修复，所有功能/API 仍使用当前项目实现。旧候选 `bf5aae07...` 的发布证据已撤回；修复后的候选、镜像、Gate、全新备份、prepare 和以切换前 live 为目标的回滚合同尚待重新生成。历史生产事实保留，以本文文末最新记录为准。

## 项目身份

- 上游：`https://github.com/Wei-Shaw/sub2api.git`
- 目标 fork：`F:\MySub2\sub2api`
- 旧二开输入：`F:\Sub2Api\SubNexus`
- 当前迁移分支：`feature/subnexus-migration`
- 目标 fork `main`：`d596d0844`（保持不变）
- 最新本地上游基线：`upstream/main=98d86915becae9fe9491a91ffc6defd5235c8d2b`（版本 `0.2.4`，2026-09-09 合并提交 `c76c04dd170c6eb4d34f864150c8e03536f38c24`）；以下 `0.2.1` 生产信息仍为历史快照，不代表本轮已部署。
- 当前迁移分支：`feature/subnexus-migration`；预期生产应用基线为 `f6f6dafe1fb2008d0a6f41dc746ae831babc3b18`。本轮 Rain + Glass 不可变候选 commit=`187b128bd32d1e06ad6e08817632e7c6b5ccca92`、tree=`e81b71b7f136da0a62bd51e266f041c6312e6b0b` 已推送；生产基线仍须线上实时复核，`main` 未修改。
- 旧二开参考 HEAD：`62ea35e1c78416fd83e1e41bbb310b307941811a`，分支 `alignment/v0.1.181-local`
- 两仓库没有 Git merge-base，不能使用整体 merge、整体覆盖或直接 cherry-pick 作为迁移策略。

## 当前状态

| 状态项 | 当前值 |
| --- | --- |
| 迁移阶段 | 既有业务迁移、上游 v0.2.1、`F:\Rain` 首页和保留二开用户端界面均已发布；Rain + Glass 用户端视觉更新正在完成线上候选、Gate、全新备份与无停机 prepare，最终 switch 尚未执行 |
| 业务代码迁移 | F01-F13 后端、API、路由、Wire、设置与功能开关保持当前实现；本轮只迁移对应用户端显示层，所有迁移功能继续遵守现有开关 |
| 新 fork 数据库迁移 | 已新增 `9001`–`9013` 共 13 个业务/兼容 SQL；runner 有 27 组显式旧文件名接管门禁（23 组内容映射、2 组语义接管、2 组独立表接管） |
| 生产数据库访问 | 第二次候选启动约 35 秒并于 `2026-09-05 01:17:03 UTC` 应用 `9001`-`9013`；13 条 checksum 与候选 SQL 全部一致。自动回滚未恢复数据库，旧应用已在迁移后同库上恢复健康；未手工执行迁移或恢复 PostgreSQL/Redis |
| 生产部署/切换 | 最后已验证的生产容器为 `232f6c5b374605760529cfac6b765fe68ba6aafc0d5d0fc8641a6a3030d63511`，`running/healthy/restart=0`；历史 run `/srv/subnexus-migration/cutover/20260907045159-1121373` 已 switched，但第 14 节 rollback 随本轮发布开始已撤回。线上身份须实时复核；当前没有可执行入口，完成本轮全部门禁后仅使用第 15 节同一 wrapper/run 的 switch/rollback |
| 生产开关 | 本轮未开启功能、修改首页配置、执行数据库/Redis 恢复或修改 Nginx；切换后唯一配置变化为管理员 PUT 写入的 `subnexus_invite_activities_config`，当前 18 项受保护设置 SHA256=`eddb4a4333c07c1cea357f12e2adb0bede963f6f0806f6757895c8d3f0f092d4`，其余 17 项保持 prepare 值；客服继续按原配置 `customer_support_enabled=false` 不显示 |
| 工作区 | 不可变候选 commit=`187b128bd32d1e06ad6e08817632e7c6b5ccca92`、tree=`e81b71b7f136da0a62bd51e266f041c6312e6b0b` 已推送。镜像、Gate、全新备份、prepare run 和最终审计尚待生成，后续记账提交不替代镜像构建 SHA |
| 当前磁盘 | 切换后只读审计读数为 `55796555776` bytes、74%；迁移临时文件和失败审计 partial 已按精确清单清理，未使用 prune，固定回滚对象与必要证据均保留 |
| 本地测试产物 | 生产备份位于 `F:\MySub2\production-backups`；PostgreSQL 18.4 隔离集群位于 `F:\MySub2\.production-restore-20260903T073714Z` 并仅监听 `127.0.0.1:56418`，当前用于 Release Gate；均未纳入 Git且不属于生产资产 |

线上服务器的最后历史快照记录在旧项目记忆中，必须用实时服务器检查覆盖，不能直接当作当前事实。特别是旧文档中的 `/www/wwwroot/SubNexus`、`/www/source/SubNexus`、端口 `18080`、root SSH 和 `main` 分支不是当前 OVH 部署的默认值。

## 功能裁决

### 迁移保留

签到/签到奖励、排行榜、活动中心 `custom` 卡片、公告/跑马灯扩展、首充礼包、二开邀请活动奖励、发票事务系统、Battle Pass、学生充值优惠、注册 IP 冷却、Channel Monitor V3、默认语言和客服按钮/Markdown 弹窗。

### 明确排除

每日消耗转盘、红包雨、运行日历、Media Studio，以及与 Media Studio 等同的 Creative Workshop/创意工坊。排除项不得复制代码、迁移、入口或恢复旧表。

### 以上游为准

Model Plaza、Grok/XAI、插件系统、Composite 路由、Affiliate 基础能力、支付基础接口、批量生图、Prompt Audit/`securityaudit`、中文验证码、Spark Shadow，以及目标 fork 已有的账号、网关、调度实现。旧项目同名代码只能用于差异审计，不能整模块替换目标实现。

## 技术架构速览

- 后端：Go、Gin、Ent 生成模型、原始 SQL repository、Redis、Wire 依赖注入。
- 前端：Vue 3、Vite、TypeScript、Pinia、vue-router、vue-i18n、Axios、TailwindCSS。
- 数据：PostgreSQL + Redis；迁移由 `backend/internal/repository/migrations_runner.go` 启动时自动执行。
- 迁移追踪：`schema_migrations(filename, checksum, applied_at)`；checksum 为 `SHA256(TrimSpace(SQL))`。
- 事务规则：普通 `.sql` 在事务中执行；`*_notx.sql` 仅用于并发索引等明确非事务场景。
- 同库兼容：27 组显式旧文件名 alias 仅在目标记录缺失、旧/目标 checksum 和数据库对象契约全部通过时处理；23 组只补写目标记录，`189/226` 两组按审计过的事务 replay 规则执行，学生优惠/注册冷却各有独立表契约，失败时拒绝启动。
- 核心入口：`backend/internal/server/router.go`、`backend/internal/server/routes/`、`backend/internal/handler/`、`backend/internal/service/`、`backend/internal/repository/`。
- 前端开关入口：`frontend/src/utils/featureFlags.ts`、`frontend/src/stores/app.ts`、`frontend/src/components/layout/AppSidebar.vue`、公共设置 API。

## 默认关闭标准

每个迁移功能必须有后端设置，默认值为 `false`；公共设置和前端入口同步关闭；关闭时只读接口可以稳定返回 `200` 携带 `enabled:false`/空列表，写接口返回约定的禁用错误（通常为 `403` 或 `404`），任何情况下不写业务数据；定时任务、队列、通知和奖励发放 no-op；关闭状态不能改变上游原有行为。开关名必须先搜索目标代码，避免与已有上游设置冲突。

当前独立开关（均默认关闭；缺失/非法按关闭处理）：

```text
subnexus_checkin_enabled
subnexus_leaderboard_enabled
subnexus_activity_center_enabled
subnexus_marquee_enabled
subnexus_first_recharge_enabled
subnexus_invite_rewards_enabled
subnexus_invite_activities_enabled
subnexus_invoice_enabled
battle_pass_enabled
subnexus_student_recharge_benefit_enabled
registration_ip_cooldown_enabled
```

邀请活动还要求 `subnexus_invite_activities_enabled` 与对应子开关同时为字面量 `true`；Channel Monitor V3 复用 `channel_monitor_enabled` 并要求 `channel_monitor_mode=v3`，缺失/非法模式回退 `v1`。默认语言和客服设置使用 namespaced/legacy 双键，空值或非法值不产生行为。

## 数据与回滚原则

1. 新 fork 上游已有迁移以上游文件为准；已执行文件不可改名、修改、删除或复用编号。
2. 新迁移必须使用目标仓库中全局唯一的文件名，优先新增独立表和可选字段，不删除/重命名核心表字段。
3. 代码回滚优先于数据库恢复。新增表和可选字段必须让旧版本可忽略；数据库恢复只在确认数据层损坏并获得明确批准后进行。
4. 本地开发先使用空库和旧项目迁移构造的隔离副本；Batch 1-5 全部完成后，Release Gate 再使用线上备份恢复出的隔离副本。任何本地候选都不能直接连接生产数据库启动。
5. 线上切换时旧版本容器、镜像、源码、配置、PostgreSQL 备份、Redis 备份和回滚脚本长期保留，由维护者后续自行删除。

## 生产环境已知边界（历史快照，需实时复核）

旧项目线上文档最后记录：OVH `51.81.211.97`，SSH 用户 `ubuntu`；应用容器 `subnexus-cutover`，本地端口 `127.0.0.1:18083 -> 8080`；PostgreSQL 容器 `sub2api-postgres`；Redis 容器 `sub2api-redis`；Docker 网络 `sub2api-net`；源码工作树 `/srv/subnexus-repo`；证据/部署目录 `/srv/subnexus-migration`。这些值只能用于生成盘点命令，不能跳过实时 `docker inspect`、Nginx 有效配置和数据库查询。

旧项目的生产工具（`tools/production-deploy/`）绑定旧提交、旧分支和旧迁移 checksum，不能直接用于本 fork。后续应新建目标侧只读 preflight、迁移 apply、候选 cutover 和回滚工具，复用其安全思想而不是其参数或迁移列表。

## 维护规则

- 任何操作前检查 `git status --short --branch`，不得覆盖用户已有改动。
- 每次操作完成后立即追加 `SUBNEXUS_CHANGE_MEMORY.md`，记录时间、目的、风险、命令/文件、结果、未完成项和回滚点。
- 每个迁移批次使用独立提交；提交前检查 `git diff --check`、敏感信息、依赖清单、`frontend/pnpm-lock.yaml`、VERSION 和生成产物。
- 旧项目 `F:\Sub2Api\SubNexus` 永久只读；所有写入只允许发生在 fork。维护者完成本地验收前只创建本地提交，不再推送，不执行服务器命令。
- 未经明确批准不执行生产迁移、部署、切换、开关开启、依赖安装或删除操作。
- 最新授权允许代理完成安装脚本、全新备份/`prepare`、never-started probe 验收及范围明确的无用垃圾清理；仅最终 `switch` 和 `rollback` 必须停下交给维护者手动执行。任何历史失败 run 不得重试或复用。
- 不记录密码、Token、API Key、Cookie、JWT/TOTP secret、私钥或完整 `.env`；日志和证据必须脱敏。
- 前端验证优先使用冻结 lockfile；目标 Docker 使用 pnpm 9，禁止因本地 pnpm 版本差异重写 lockfile。

## 下一步入口

1. 已完成线上只读 preflight 和生产 PostgreSQL/Redis/应用数据备份结构校验；服务器备份目录为 `/srv/subnexus-migration/backups/20260903T073714Z`，所有 SHA256 均通过。
2. 备份已下载并通过 20 个文件 SHA256；PostgreSQL 18.4 原始恢复库、真实克隆 migration/adoption、候选全部关闭态、旧版回归和 Redis 8.8.0 RDB 隔离加载均通过。Redis 证据位于台账记录的 root-only 路径。
3. 上一轮 Rain run `/srv/subnexus-migration/cutover/20260906134705-774592` 和 retained-UI run `/srv/subnexus-migration/cutover/20260907045159-1121373` 均已由维护者完成 switch，并作为历史记录保留；两者 rollback 窗口在本轮 Rain + Glass 发布开始后均已关闭。当前没有可执行入口，完成本轮全部门禁后只使用切换手册第 15 节同一新 run 的 switch/rollback。

## 2026-09-05 上游 v0.2.1 合并状态

- 已在 `feature/subnexus-migration` 合并上游发布标签 `v0.2.1`（代码提交 `578785ee7fb35030b094b69624efe25670a36f5f`）及其后唯一的版本同步提交 `ab99d56e9626e6cd731592dae8553c9758a0efa2`；当前合并 tip 为 `8a0c8af8534b4038e357ab8368eb027e0a489cee`。
- 合并保留现有 SubNexus 业务代码、`9001`-`9013` 迁移和项目记忆文件，并纳入上游 0.2.1 的网关、模型、定价、用量记录和前端管理能力；未修改 fork `main`、旧项目或服务器。
- 本地验证：`git diff --check`、`pnpm typecheck`、`pnpm build`、`go test ./...` 通过；单独完整运行 Vitest `286/286` 文件、`1987/1987` 测试通过。
- 当时快照中未提交内容仍仅为首页 Rain + Glass UI 的 `frontend/src/views/HomeView.vue` 和 `frontend/public/images/rain-city-1.jpg`；该内容随后已纳入候选并完成 Docker gate、线上切换和切换后审计，当前状态以文末最新记录为准。

## v0.2.1 发布前历史快照（2026-09-05，已被本轮状态覆盖）

上游 `v0.2.1` 已由当前分支提交 `bb36764f692ca79ccc9c635fd71dcbb70b9c0449` 构建并完成服务器候选 gate。候选镜像为 `sha256:21098ec4f4c922efa92208b640a970eb8602778c0515e8921967f9d75dc5adfd`，归档 SHA=`d1a6e297720d7af32a1ba98a7e4d3c9dc66a72a4fe7aace1ea96d346d7b1b648`，gate evidence=`/srv/subnexus-migration/docker-candidate/20260905T113230Z-1add8fbc-85d4-4d21-af6c-23fbc34af218/evidence.txt`，SHA=`ac2cd667c1a317b3d0eab0e11a9b3cbada4b4ecec49850eba08f81263ebb2bf5`。

当时唯一可交接 run 是 `/srv/subnexus-migration/cutover/20260905114022-4163123`，该历史批次随后已 switched；其 prepare/probe 记录仅作审计，不能作为本轮入口。

## Rain + Glass UI 首次准备快照（2026-09-06 00:09 Asia/Shanghai，后续已失效）

- 应用提交=`b1ed483ea5fc648cb3c15fcf2e7040e68a151a41`，tree=`bb821e2a0003d13cd425ca8ff012dbb26f70b1a6`；仅默认首页展示和 `frontend/public/rain-city-1.jpg` 改动，原脚本与 88 项模板业务绑定不变。图片最终路径为 `/rain-city-1.jpg`，避免生产 `/images/` 网关保留路径。
- 候选镜像=`sha256:32f14750ce73da00dc4c5146b1d9ad6c4420ee2c3dffe098798e41a123c6bd2c`；归档 `/srv/subnexus-migration/candidate-artifacts/rain-b1ed483ea5fc-retry2/candidate-image.tar`，SHA256=`26422d9eaad7ede983b228e84ee756eae313347b0135bf4e2d48138912c3246b`。
- Docker gate `/srv/subnexus-migration/docker-candidate/20260905T155430Z-940fdcd9-bc3c-4d12-8c72-12ed9e27328b/evidence.txt` 和首页证据 `/srv/subnexus-migration/diagnostics/rain-home-b1ed483ea5fc.ENig2O5r` 均通过；最终证据哈希待统一登记。
- UI 包装器 `/srv/subnexus-migration/tools/subnexus-ui-cutover-7c3a42ac-20260906.sh` 的 SHA256=`7c3a42ac381f3839b5de5d605d465ee13b005ea9321b28ef47427ece2e910d77`；原控制器仍为 `subnexus-production-cutover-19824a87-20260905-v021.sh`，SHA256=`19824a87e3e1de5659cb30664750b71c5c10d374f25bda7f52e6524fe477ee65`，二者独立校验。
- 在线 prepare run=`/srv/subnexus-migration/cutover/20260905160223-175225`，PID=`175225`，最近进度为 PostgreSQL dump，尚无 READY；不能据构建/gate 通过提前宣告准备完成。
- 固定回滚对象始终为旧 SubNexus `be459424b327ad056ea9bdc02187d6a458fe09082369b354158d6e7f7758beee`，名称 `subnexus-cutover-pre-96b66b3e74c1-20260905085804-4072165`，锚定 run `20260905085804-4072165`，旧 image ID 前缀 `b24b585`。本轮不新建永久回滚对象；当前 v0.2.1 容器 `9753053d8bd9...` 不替代该旧对象。

## Rain + Glass UI 历史准备状态（2026-09-06 00:41 Asia/Shanghai）

- 首次 run `20260905160223-175225` 的 prepare 曾成功，但全量 settings 哈希随后从 `d66bf0e2c9ee6c1734bfa38cdae508e174562051e18acd093c14b81ab0e9705a` 漂移为 `af154e9a7a878bfc5295f12e88d4143c5466ab0c83939831fa13d202b71bc90a`，probe 在创建候选前安全停止，无 candidate。具体被改动键和来源未确定；全量 settings 与原控制器仅 18 键快照不能直接对应比较，后续哈希复验稳定不代表已定位原因。
- 第二次 run `20260905163008-194872` 在备份前因可用 `18797457408` bytes 小于预算 `23715311616` bytes 被拒绝。首次 run 的三个大备份及 sidecar 经 SHA 校验/记录后精确删除，manifest/settings/metadata 保留并写入 `INVALIDATED_SETTINGS_DRIFT`；该 run 不可复用。
- 清理日志 `/srv/subnexus-migration/cleanup-rain-invalid-run-20260905160223.txt`，SHA256=`c3e1af6e289292b4b2baa8b76136ea322f19556785a17caf63d6d34c2060d326`；清理后可用 `24025554944` bytes。
- 最终在线 prepare `/srv/subnexus-migration/cutover/20260906100431-660485`（PID 660485）已 `READY=prepared`；候选镜像、脚本和固定旧回滚对象不变。stopped probe 已精确删除且生产状态未改变，尚未执行 switch/rollback。

## `F:\Rain` 首页源码直接迁移切换前快照（2026-09-06 22:09 Asia/Shanghai，历史）

- 本轮以 `F:\Rain` 的真实首页源码为 UI 基础，迁移页面结构、玻璃容器、三张原图、字体/图标表达、两层 Canvas、鼠标视差、动画和响应式行为；默认首页入口改用 `RainGatewayHome`。自定义首页与精简首页分支仍保留。
- 站点名称、Logo、副标题、文档、模型广场、登录/后台、主题、语言、客服、providers/footer 和 `/v1/messages` 继续使用当前项目配置、路由、权限和事件；未增加或删除业务功能，后端、数据库迁移、依赖锁文件、功能开关和 Nginx 均未改动。
- 应用 commit=`245ecd2630b96a9807df89dc02828bbb436e7624`，tree=`b0f55487331dca07031219d59cd4159cab8a610d`；Vitest `287/287` 文件、`1990/1990` 测试、定向首页 `4/4`、TypeScript、ESLint、生产构建和 UI wrapper `26` 个故障/恢复场景通过。Playwright 在 `1440x900` 与 `390x844` 无溢出/页面错误，两层 Canvas 非空；三张图片与 `F:\Rain` SHA256 完全一致。
- 候选 image=`sha256:e472d61e8db88ec5cdd0c0c4ad9e9db11b28c3495a14af02287c99b6addf23a7`；归档 SHA=`65f06c3e221cfd08d68f84df882b2b3c858e5b8c939715f41c466eee0cb35f15`；Docker Gate SHA=`d13c2a3095db4699d1a20939818d003e985f2715655f51e9715f286388d14544`；首页 observer SHA=`bd70fe35573d4a6c2ac6399cf50f9bec0cd18600b387ed8eea0e2f65ee76f678`。
- 新 run=`/srv/subnexus-migration/cutover/20260906134705-774592`，`READY=prepared`、`UI_READY=application-refresh-v1`、manifest `state=prepared/ui_state=prepared/ui_commit_intent=no`，manifest SHA=`e3809a4d6a09d469c994d38453551d466e73f49b45903aae17f8683fe63fc897`。PostgreSQL、Redis、应用数据、设置和 sidecar 三方哈希均通过。
- 最终审计证据 `/srv/subnexus-migration/diagnostics/rain-prepared-245ecd2630b9-20260906134705-774592.evidence`，SHA=`0f69354a5d7911a66a6c5ef01fc58ac3160bd838a78fd214f8bb7151ac125609`。Probe 容器 `e9d9da3e...` 从未启动并已按完整 ID 删除；run manifest 前后未变，生产应用、PostgreSQL、Redis、18 个受保护设置和固定 anchor 未变。
- 固定回滚对象当时仍为旧 SubNexus `be459424b327ad056ea9bdc02187d6a458fe09082369b354158d6e7f7758beee`，image=`sha256:b24b585a35e0eecff497a4eb7a2be480d9a2818f4b7a9780508f2f42cb5e09cd`，名称 `subnexus-cutover-pre-96b66b3e74c1-20260905085804-4072165`，anchor=`/srv/subnexus-migration/cutover/20260905085804-4072165`。该快照时尚未执行 switch/rollback；当前状态由下节覆盖。

## `F:\Rain` 首页生产切换后状态（2026-09-06 23:25 Asia/Shanghai，上一版生产快照）

- 维护者手动 `switch` 的原始成功行是 `UI_SWITCH_COMPLETED=/srv/subnexus-migration/cutover/20260906134705-774592`。同一 run 的 `READY=prepared`、`UI_READY=application-refresh-v1` 保留，`SWITCHED=switched`，`ROLLED_BACK` 不存在；manifest=`state=switched/ui_state=switched/ui_commit_intent=yes`，SHA256=`86afbaa48b5a22cdd193eb7f95238d8a70c74b870b7151476153317a3f0ffe79`。
- `2026-09-06T15:19:27Z` 启动的切换后只读审计最终输出 `POST_SWITCH_AUDIT=passed` 且退出码为 `0`。新生产容器 ID=`86104829d490733244c9426a59e82e7a12afa3c590de2fc03f2ccb13344aebbd`，image=`sha256:e472d61e8db88ec5cdd0c0c4ad9e9db11b28c3495a14af02287c99b6addf23a7`，`running/healthy/restart=0`，`StartedAt=2026-09-06T14:59:13.131115785Z`。manifest 的 `live_app_id` 是切换前容器，当前线上身份取 `candidate_container_id`。
- 切换前容器 `c3ea071f4526bdb2502444d8f18b9da4c761aa3d51be6f7e5fc19c910ca6300f`、其临时名称、probe 容器和 probe 临时目录均无残留；PostgreSQL `8178576aed6f7b1cb94201832e5797907ea4d7698dbfe7b6f862cbc5a3b4f5bf` 与 Redis `5c7adf42247c67ba90b09248056071a57c2a4e7e0465f922d4ed799ef092533e` 保持原身份且 `running/restart=0`。
- PostgreSQL、Redis、应用数据备份的实际文件、sidecar 和 manifest 三方哈希重算通过；18 个受保护设置 SHA256=`3959daf3caed2f8a4c22023db4b7da8be627fb4b8a087bba4d5309cd8223d558`，runtime contract SHA256=`7dc88dd8f76be1a69c6d4f322deb1b1e0eda8be94be61d37cac850091578453d`。文件权限、previous-container 日志及固定 anchor 检查均通过。
- 固定旧 SubNexus 回滚对象保持为 ID=`be459424b327ad056ea9bdc02187d6a458fe09082369b354158d6e7f7758beee`、image=`sha256:b24b585a35e0eecff497a4eb7a2be480d9a2818f4b7a9780508f2f42cb5e09cd`、name=`subnexus-cutover-pre-96b66b3e74c1-20260905085804-4072165`、`exited/restart=0`；anchor=`/srv/subnexus-migration/cutover/20260905085804-4072165`，anchor manifest SHA256=`e0b49d89e6a28044c5588afb5a536a3e87ceacfa2a7d679d324d65b14a4c100e`。本轮未创建新永久回滚对象，未执行 rollback。
- 公网 `https://yydsapi.uno` Playwright 验收在 `1440x1000` 与 `390x844` 均无页面错误或溢出；三张 Rain 原图及双 Canvas 正常，未登录文档、模型广场、登录、语言和主题交互通过，配置站点名、Logo、副标题正常；客服因原配置 `customer_support_enabled=false` 按设计不显示。公网 Playwright JSON 报告 SHA256=`9851d28cc2645f79e4325b744fb1c8f80cc25cefae1f338c97eef7ac3d687855`。
- 本 run 不得作为新的 `prepare`/`switch` 输入，已消费的 switch 禁止重跑；所有历史 switch 命令也不得复用。第 13 节旧 Rain rollback 窗口已因 retained-UI switch 关闭，当前恢复只使用第 14 节同 run rollback。

## 保留二开用户端界面工作包（2026-09-07，当前）

本工作包采用分离的来源合同：用户看到的页面结构、组件、CSS、文案、弹窗、动画和响应式行为以 `F:\Sub2Api\SubNexus` 中保留二开实现为准；功能和数据以当前 `F:\MySub2\sub2api` 为准。按钮外观与位置可使用旧版实现，点击事件、路由、权限、API 地址、请求参数、配置读取和业务处理必须继续绑定当前代码。

当前覆盖活动中心、排行榜、Affiliate、邀请抽奖、累计充值奖励转盘、邀请里程碑、公告/跑马灯、客服、签到、首充、学生优惠、发票、Battle Pass 和 Channel Monitor V3。共享旧版视觉 primitive 只在 `.subnexus-legacy-surface main` 内生效；Dashboard/Payment 采用局部 opt-in，AppHeader/AppSidebar 及 Channel Monitor V1/V2 不受全局覆盖。AmountInput 保留旧版默认选择，中文学生资格副文案为“学生身份已生效”。

每日消耗转盘、红包雨、运行日历、Media Studio、Creative Workshop 明确排除；旧活动红点、旧单入口和旧奖励联动也不恢复。F10 注册 IP 冷却与 F12 默认语言没有新增页面，其当前功能合同原样保留。

本轮候选已经固定并推送。精确发布范围为 34 个 production UI 文件和 18 个测试/记忆/部署证据文件，共 52 个；wrapper 已证明会拒绝 API、后端、数据库迁移、router、依赖、锁文件、全局样式、Tailwind、上一版 Rain 首页、删除、符号链接和可执行位变化。`.codex-ui-mock-server.mjs` 未进入提交或制品。

### 本轮精确发布 allowlist（52 项）

34 个 production UI 路径：

```text
frontend/src/components/common/AnnouncementBell.vue
frontend/src/components/common/AnnouncementPopup.vue
frontend/src/components/common/BaseDialog.vue
frontend/src/components/common/BroadcastMarquee.vue
frontend/src/components/common/ConfirmDialog.vue
frontend/src/components/common/CustomerSupportButton.vue
frontend/src/components/common/CustomerSupportModal.vue
frontend/src/components/layout/AppHeader.vue
frontend/src/components/layout/AppLayout.vue
frontend/src/components/payment/AmountInput.vue
frontend/src/components/user/dashboard/UserDashboardCheckIn.vue
frontend/src/components/user/monitor/ChannelMonitorV3Card.vue
frontend/src/i18n/locales/en/activityCenter.ts
frontend/src/i18n/locales/en/common.ts
frontend/src/i18n/locales/en/inviteActivities.ts
frontend/src/i18n/locales/en/leaderboard.ts
frontend/src/i18n/locales/en/misc.ts
frontend/src/i18n/locales/zh/activityCenter.ts
frontend/src/i18n/locales/zh/common.ts
frontend/src/i18n/locales/zh/inviteActivities.ts
frontend/src/i18n/locales/zh/leaderboard.ts
frontend/src/i18n/locales/zh/misc.ts
frontend/src/styles/subnexus-legacy-surface.css
frontend/src/utils/bodyScrollLock.ts
frontend/src/views/user/ActivityCenterView.vue
frontend/src/views/user/AffiliateView.vue
frontend/src/views/user/BattlePassView.vue
frontend/src/views/user/ChannelStatusV3View.vue
frontend/src/views/user/InviteLotteryView.vue
frontend/src/views/user/InviteMilestoneView.vue
frontend/src/views/user/InvoicesView.vue
frontend/src/views/user/LeaderboardView.vue
frontend/src/views/user/PaymentView.vue
frontend/src/views/user/RechargeWheelView.vue
```

18 个测试、记忆和部署证据路径：

```text
frontend/src/components/common/__tests__/CustomerSupportButton.spec.ts
frontend/src/components/common/__tests__/CustomerSupportModal.spec.ts
frontend/src/components/common/__tests__/ScopedDarkModeStyles.spec.ts
frontend/src/components/layout/__tests__/SubnexusLegacySurface.spec.ts
frontend/src/components/payment/__tests__/AmountInput.spec.ts
frontend/src/utils/__tests__/bodyScrollLock.spec.ts
frontend/src/views/user/__tests__/InviteActivitiesViews.spec.ts
frontend/src/views/user/__tests__/LeaderboardView.spec.ts
frontend/src/views/user/__tests__/PaymentView.spec.ts
SUBNEXUS_CHANGE_MEMORY.md
SUBNEXUS_CUTOVER_RUNBOOK.md
SUBNEXUS_FEATURE_MATRIX.md
SUBNEXUS_MIGRATION_LEDGER.md
SUBNEXUS_MIGRATION_PLAN.md
SUBNEXUS_PROJECT_CONTEXT.md
SUBNEXUS_ROLLBACK_RUNBOOK.md
tools/production-deploy/subnexus-ui-cutover.sh
tools/production-deploy/subnexus-ui-cutover.test.sh
```

上述两组路径已经与 `245ecd2630b96a9807df89dc02828bbb436e7624..f6f6dafe1fb2008d0a6f41dc746ae831babc3b18` 差异集合完全一致。wrapper 仍逐项拒绝删除、重命名、符号链接、可执行位或其他文件模式变化。

本轮不可变制品为 commit=`f6f6dafe1fb2008d0a6f41dc746ae831babc3b18`、tree=`7b0ee6db2dc96fd97106ca175640b3a15e8ec233`、image=`sha256:59eb4c84de8b8fec11fb903dc728676e9cffacbcc435ce5ea1b60487cc910fcc`、archive SHA256=`aae7dbca9336a81414f06fb273f8b66c1723aaef891316e4fe32827bf9650084`。Docker Gate evidence SHA256=`6ed8c611f8c86893c49f5594f6f2b39cd01b010e26f68a7b0b2afcde82e39c8a`，首页 observer SHA256=`272626c264aaf7d12811460707f231a903af89affbec342315434c1ea7cfac70`。

### 切换前历史证据（已由下方当前线上状态覆盖）

run `/srv/subnexus-migration/cutover/20260907045159-1121373` 当时已完成无停机 prepare：`READY=prepared`、`UI_READY=application-refresh-v1`，manifest SHA256=`87b51ae80dccc6ae4590537bcc2636795b0a650db84a7bb40a9531b4ce85d135`。该切换前快照没有 `SWITCHED`/`ROLLED_BACK`；后续状态见下方当前线上记录。全新 PostgreSQL、Redis、应用数据备份及 sidecar、受保护设置和 runtime contract 均通过；probe 从未启动并已删除。

严格最终审计使用远端脚本 `/srv/subnexus-migration/tools/audit-retained-ui-prepared-5239d9c1-20260907.sh`，SHA256=`5239d9c17d03f7bd6dce24daed7e2c1d8216d9e98b854cafec4a364fec13f7f6`；evidence SHA256=`9dc1293e6ab16af8b1f805b30e39eba5ecfa55e2b07015ae251a308b70c234ea`，结果为 `FINAL_PRE_SWITCH_AUDIT=passed`、`FINAL_SWITCH_EXECUTED=false`。

固定旧 SubNexus 始终为 ID `be459424b327ad056ea9bdc02187d6a458fe09082369b354158d6e7f7758beee`、image `sha256:b24b585a35e0eecff497a4eb7a2be480d9a2818f4b7a9780508f2f42cb5e09cd`、name `subnexus-cutover-pre-96b66b3e74c1-20260905085804-4072165`、anchor `/srv/subnexus-migration/cutover/20260905085804-4072165`。prepare 只在 manifest 中绑定该对象，没有创建新永久回滚容器或镜像。以上为切换前历史快照；第 13 节 rollback 窗口现已因后续 retained-UI switch 关闭，当前恢复只使用第 14 节同 run rollback。

## 当前线上状态（2026-09-07，切换后）

保留二开用户端 UI 的唯一生产 run 为 `/srv/subnexus-migration/cutover/20260907045159-1121373`。切换前的 `READY=prepared` 和 `UI_READY=application-refresh-v1` 仍是准备阶段 marker；切换后新增 `SWITCHED=switched`，manifest 为 `state=switched/ui_state=switched/ui_commit_intent=yes`，SHA256=`5cc60f478673b2615d96d2993604da934353a6abb66d932e81f268bdfa4acda3`。当前容器 ID=`232f6c5b374605760529cfac6b765fe68ba6aafc0d5d0fc8641a6a3030d63511`，image=`sha256:59eb4c84de8b8fec11fb903dc728676e9cffacbcc435ce5ea1b60487cc910fcc`，`running/healthy/restart=0`；固定旧回滚对象 `be459424...` 仍 `exited/restart=0`，没有创建新的永久回滚对象。

切换后服务器审计工具 `/srv/subnexus-migration/tools/audit-retained-ui-switched-d5652bda-20260907.sh` 的 SHA256=`d5652bda85a82e5eaa20db5f5b80a86c691bcdbbb10f58d199636f0ff7f6766d`，正式 evidence `/srv/subnexus-migration/diagnostics/retained-ui-switched-f6f6dafe1fb2-20260907045159-1121373.evidence` 的 SHA256=`58edc5b2d6e3ca6535ae10741ce4aed9275609f5d5e8dad1c48407372349afb9`，结果为 `POST_SWITCH_AUDIT=passed`。PostgreSQL=`8178576aed6f7b1cb94201832e5797907ea4d7698dbfe7b6f862cbc5a3b4f5bf`、Redis=`5c7adf42247c67ba90b09248056071a57c2a4e7e0465f922d4ed799ef092533e` 身份未变，均 `running/restart=0`。公网 `https://yydsapi.uno` 桌面/移动 Playwright 验收通过，无页面错误、请求失败或横向溢出，Rain 图片、双 Canvas、配置驱动品牌和现有导航/登录/语言/主题行为正常。

切换后设置审计只发现 `subnexus_invite_activities_config` 变化；Nginx 记录管理员 `PUT /api/v1/admin/invite-activities/config` 成功，数据库 `updated_at=2026-09-07T06:45:51.718553Z`，未回写或恢复，其他 17 项保护设置保持 prepare 值。清理证据 `/srv/subnexus-migration/cleanup-retained-ui-postswitch-20260907-1121373.txt` 的 SHA256=`282250b4f612f154e60c7d1b42954ac005b9bb8b3e6db11710a08dacf46b6558`；失败审计 partial、迁移上传副本等 27 个临时文件已精确删除，正式证据、备份、应用数据和固定回滚对象保留。第 13 节旧 Rain rollback 已关闭；需要恢复时只使用切换手册第 14 节同 run rollback。

## 当前发布任务（2026-09-09，用户端视觉性能与显示修复）

当前应用改动包含 Rain + Glass 用户端页面、标准/轻量/兼容三档视觉性能模式，以及 `/usage` 日期菜单层级、窄屏日期菜单和签到翻译文案修复。管理端、认证/公开页和外部支付页隔离；数据、API、路由、权限、配置、功能开关、按钮和业务逻辑不变。业务/前端验证沿用已记录的完整结果，发布 wrapper 另有独立故障恢复测试。

发布仍处于本地到线上前置阶段。旧候选 `bf5aae07bb30b380cb1be154c49149c9c64cc7f7` 的镜像、Gate、wrapper 和 prepared run 因无法恢复切换前现网版本而全部作废。修复后的不可变 commit/tree、镜像 ID、归档 SHA、Docker Gate、全新备份、prepare run、probe 和最终审计必须重新生成；最终 `switch` 由维护者手动执行，代理必须停在 `state=prepared/ui_state=prepared`。

本轮回滚模型已变更：`prepare` 先要求历史旧 SubNexus anchor 存在并完整校验，然后仅把当前 live 的完整 ID、`.Image`、`.Config.Image`、唯一 `production-app-ui-prior-<run-id>` 名称和状态固化到新 manifest，Docker 状态保持不变。最终 switch 时才停止和重命名该 live，并把它作为 stopped 新回滚目标保留；本轮 rollback 只恢复该目标，不恢复更早的旧 SubNexus。目标缺失或 ID/image/configured image/name/runtime contract 漂移时必须失败关闭。

历史 anchor 不是本轮实际恢复对象，只在 prepare 和 switch 提交前作为强制连续性门禁，并继续保留为二级灾备证据。一旦 switch 已开始，其缺失或漂移不得阻止 automatic recover、manual recover 或 rollback 恢复经过 manifest 严格绑定的 previous-live。空间足够时保留全部历史数据；容量证据确认不足时，仅可在保存完整 ID/路径/SHA 的精确清理审计后删除其他已失效 run 备份或无引用垃圾。不允许 `docker system prune`、`docker volume prune` 或模糊清理，历史 anchor、本轮新回滚目标与新 run 证据不可删除。最终只接受切换手册第 15 节中绑定同一 wrapper/run 的两条单行命令。

## 本地上游对齐（2026-09-09，v0.2.4）

- 当前本地代码已合并上游 `98d86915becae9fe9491a91ffc6defd5235c8d2b`，merge commit=`c76c04dd170c6eb4d34f864150c8e03536f38c24`，tree=`4ed29392a38985a10a4a6ae7f04d94d04374dbc4`。保留 F01-F13、Rain 首页、用户端视觉性能模式、菜单层级与签到文案修复。
- 合并补齐监控用户排行设置的公共读取、DTO、页面注入与后台开关，恢复 V2/V3 吞吐量开关；默认关闭和 V3 模式语义保持本项目约定。上游 MiniMax、模型白名单、Astra/Image 2.5、支付、网关和代理修复已纳入。
- 前端全量 Vitest 311 文件/2174 测试通过；后续监控修复定向 3 文件/45 测试通过（新增 2 项），lint、类型检查与生产构建通过。后端默认和 unit 标签包均通过，受失败或改动影响的包已复跑；`go vet`、embed 构建、Wire/Ent 生成一致性通过。
- 本轮没有推送或访问服务器，没有迁移数据库、发布镜像、创建回滚对象或执行 switch/rollback。原有 6 个文档、2 个部署脚本未提交改动仍留在工作区，没有纳入上述代码合并提交；本文追加记录也留在工作区。
- 后续发布必须按包含后端和数据库迁移的完整更新重新取证。上游 `235/236` 会重命名 `groups.models_list_config` 为 `model_allowlist`；旧二进制可能仍依赖旧列，不能把仅 UI 更新的同库回滚结论直接复用。本轮未运行生产备份的隔离迁移/旧版回归 Gate，也没有生成有效切换命令。`9001`-`9013` 内容保持原样。


## 2026-09-09 23:00 发布前更新

实际线上为 bf5aae07... / b9de08a4...；本次应用候选为 890828afe... / image 44e8dcf0...。构建、服务器候选 Gate、全新生产快照已完成；生产备份完整下载/隔离 new-old-new Gate、正式 prepare/probe 和最终审计尚待完成，目前不可切换。以 SUBNEXUS_CHANGE_MEMORY.md 同时间条目为权威详细证据；本轮新一级回滚目标必须是切换前实际 live，历史旧 SubNexus 仅保留为二级资料。
