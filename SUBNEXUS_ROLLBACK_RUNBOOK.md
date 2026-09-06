# SubNexus 回滚手册

> 当前权威状态：2026-09-06 23:25（Asia/Shanghai）。本轮 UI run `20260906134705-774592` 已成功 switched，切换后生产、公网、依赖、备份和固定旧回滚对象审计均通过，尚未 rollback。历史 run 均不得用于回滚交接；第 7 节固定旧回滚对象不变，并指向绑定本轮成功 run 的唯一回滚入口。

回滚按风险从低到高执行，默认只回滚应用或关闭功能，不恢复数据库。所有命令先在维护窗口核对真实容器名、端口、网络、脚本和 release SHA。本轮只能使用完成最终核验的 UI 包装器及其绑定 run，不得单独执行旧控制器的历史 rollback 命令，也不得用手工 `docker stop/start` 绕过 manifest、owner、固定旧回滚对象和依赖身份校验。

本次线上应用数据目录的已审核 owner 是 `1000:1000`、叶目录 mode `0755`。执行 `prepare`、`switch` 或 `rollback` 时，只有在实时 `stat` 与 prepared manifest 一致的前提下，才同时传入以下三项环境变量；不得通过 `chown` 来“修复”不一致：

```text
SUBNEXUS_CUTOVER_APP_DATA_OWNER_CONFIRM=I_UNDERSTAND_NON_ROOT_APP_DATA_OWNER
SUBNEXUS_CUTOVER_APP_DATA_OWNER_UID=1000
SUBNEXUS_CUTOVER_APP_DATA_OWNER_GID=1000
```

没有 owner 字段的历史 manifest 按 legacy root-UID 兼容路径读取，仅用于旧版本回滚；新建 run 不得省略现代 owner 合同。

## 1. 功能异常

立即把对应迁移功能开关改回 `false`，确认 API/UI/队列/定时任务停止写入，再保留日志和证据。不要删除新增表或修改已执行迁移文件。记录异常时间、功能开关、请求 ID 和数据库行数。

## 2. 应用异常：快速回滚

适用于隔离演练已证明旧版本兼容新增表/可选字段的情况。旧 `/root/...` 占位脚本示例已撤回，本轮只接受切换手册第 13 节的包装器单行命令；该命令必须恢复既有旧 SubNexus，不能把当前线上容器新建为永久回滚对象。

先用 `docker ps`/`docker inspect` 做只读确认，再由维护者执行本轮已发布的包装器命令。本轮使用既有入口，不修改或 reload Nginx；回滚后再访问健康接口：

```bash
docker ps --format 'table {{.Names}}\t{{.Image}}\t{{.Status}}'
docker inspect <旧容器名> <候选容器名> --format '{{.Name}} {{.Config.Image}} {{.State.Running}}'
curl -fsS --max-time 8 https://<公网健康域名>/health
```

快速回滚不恢复数据库，因新增隔离表/可选字段应被旧版本忽略；切回后必须验证登录、API Key、余额、订阅、订单、支付回调、用量和健康检查。任何 Nginx 配置修复属于单独授权的操作，不能从本轮 UI 命令推导出来。

历史失败 run 的 `ROLLED_BACK`/失败证据保留审计，不得再次 switch/rollback；历史成功 run 也不能直接用作本轮 UI 入口。此前 `20260905055413-3958448` 的 prepare/probe 和切换记录已被后续发布覆盖；v0.2.1 run `20260905114022-4163123` 实际已 switched。旧自动回滚未恢复 PostgreSQL/Redis，当前状态只取信于本轮最终实时检查。

### 历史回滚命令已撤回

旧脚本、旧 SHA 和历史 run 仅作审计事实保留，不得直接重试或复用。切换手册第 10/11/12 节的旧命令正文已撤回；本轮 rollback 只使用切换手册第 13 节绑定新 run 的单行命令，由维护者手动执行，默认不恢复 PostgreSQL/Redis。

## 3. 应用无法启动

保留候选容器、日志和数据库现场，先确认是否为配置、镜像权限、Redis 或连接问题：

```bash
docker logs --tail=300 <候选容器名>
docker inspect <候选容器名> --format '{{json .Mounts}}'
docker inspect <候选容器名> --format '{{json .NetworkSettings.Networks}}'
```

修复配置后仍失败则停止候选、启动旧容器并恢复入口。不要为“让旧版本启动”而手工删除 `schema_migrations` 记录或迁移表。

## 4. 数据库灾难恢复（最后手段）

只有确认数据已损坏、旧版本无法兼容迁移后的库、并得到维护者明确批准时才恢复切换前备份。恢复前冻结写入，评估备份之后新增的订单、余额和用量；恢复会丢失备份时间之后的数据，不能静默执行。

恢复完成后必须校验：旧版本启动无 checksum 错误；用户、API Key、余额、订阅、订单、支付回调和用量正常；关键计数/金额与切换前审计记录一致；Redis 恢复点与数据库时间一致。恢复命令由维护者根据实时容器和备份路径填写，禁止直接复制历史示例中的密码或容器名。

## 5. 记录与复盘

每次回滚追加 `SUBNEXUS_CHANGE_MEMORY.md` 和 `SUBNEXUS_MIGRATION_LEDGER.md`：写明触发条件、开关、候选/旧版本 SHA、是否恢复数据库、备份校验和、验证结果和下一步。旧版本和备份在维护者确认前长期保留。

## 6. 上一版 UI 固定回滚记录（2026-09-06，历史）

- UI candidate commit=`b1ed483ea5fc648cb3c15fcf2e7040e68a151a41`，image=`sha256:32f14750ce73da00dc4c5146b1d9ad6c4420ee2c3dffe098798e41a123c6bd2c`；本轮 run=`/srv/subnexus-migration/cutover/20260906100431-660485` 已 `READY=prepared`，manifest `state=prepared/ui_state=prepared`，manifest SHA=`4cdd0bac0157663f9f485847ac92d5cd09d3f6a66b90def09f95f3389c4570b6`。
- UI 包装器 commit=`2e60d0d55`，路径 `/srv/subnexus-migration/tools/subnexus-ui-cutover-7c3a42ac-20260906.sh`，SHA256=`7c3a42ac381f3839b5de5d605d465ee13b005ea9321b28ef47427ece2e910d77`。原控制器 `/srv/subnexus-migration/tools/subnexus-production-cutover-19824a87-20260905-v021.sh`，SHA256=`19824a87e3e1de5659cb30664750b71c5c10d374f25bda7f52e6524fe477ee65`；二者独立校验。
- 旧 SubNexus 完整容器 ID=`be459424b327ad056ea9bdc02187d6a458fe09082369b354158d6e7f7758beee`，名称 `subnexus-cutover-pre-96b66b3e74c1-20260905085804-4072165`；旧 image ID 前缀 `b24b585`；anchor run=`/srv/subnexus-migration/cutover/20260905085804-4072165`。锚定 run 供包装器读取身份合同，不是让维护者重新执行旧控制器命令的入口。
- 该次切换前的线上 v0.2.1 容器 ID 前缀为 `9753053d8bd9`，未替代上述旧 SubNexus；该次切换未创建新的永久回滚对象。不得删除该旧容器、镜像、anchor manifest 或其备份/证据。
- 该 run 后续已用于上一版 UI 切换，现行命令已经撤回。当前人工命令只见切换手册第 13 节，且必须绑定新 run。
- 首次 UI run `20260905160223-175225` 因 prepare 后全量 settings 哈希漂移而失效，probe 在 create 前停止且无 candidate；具体改键与来源未确定。其三个大备份及 sidecar 已校验/记录后删除，manifest/settings/metadata 和 `INVALIDATED_SETTINGS_DRIFT` 保留，禁止复用。第二次 `20260905163008-194872` 在备份前被空间门禁拒绝，同样不能作为回滚入口；固定旧 `be459...` 对象与 anchor 证据未因此删除。

## 7. `F:\Rain` 首页源码直接迁移固定回滚入口（2026-09-06 23:25 Asia/Shanghai，已 switched）

- UI candidate commit=`245ecd2630b96a9807df89dc02828bbb436e7624`，image=`sha256:e472d61e8db88ec5cdd0c0c4ad9e9db11b28c3495a14af02287c99b6addf23a7`。本轮 run=`/srv/subnexus-migration/cutover/20260906134705-774592` 的切换前历史证据为 `READY=prepared`、`UI_READY=application-refresh-v1`，manifest `state=prepared/ui_state=prepared/ui_commit_intent=no`，SHA=`e3809a4d6a09d469c994d38453551d466e73f49b45903aae17f8683fe63fc897`。
- 维护者执行后返回 `UI_SWITCH_COMPLETED=/srv/subnexus-migration/cutover/20260906134705-774592`；当前 `SWITCHED=switched`、`ROLLED_BACK` 不存在，manifest `state=switched/ui_state=switched/ui_commit_intent=yes`，切换后 manifest SHA=`86afbaa48b5a22cdd193eb7f95238d8a70c74b870b7151476153317a3f0ffe79`。成功 run 的 switch 命令已撤回且严禁重跑。
- UI wrapper=`/srv/subnexus-migration/tools/subnexus-ui-cutover-054507b1-20260906.sh`，SHA=`054507b15851c9547ab347f88ad21d8f9a5203be6123bfb2030e21c88806fd5d`；原控制器 SHA=`19824a87e3e1de5659cb30664750b71c5c10d374f25bda7f52e6524fe477ee65`。切换前最终审计的 prepare evidence SHA=`0f69354a5d7911a66a6c5ef01fc58ac3160bd838a78fd214f8bb7151ac125609`。
- 固定旧 SubNexus 完整容器 ID=`be459424b327ad056ea9bdc02187d6a458fe09082369b354158d6e7f7758beee`，image=`sha256:b24b585a35e0eecff497a4eb7a2be480d9a2818f4b7a9780508f2f42cb5e09cd`，名称=`subnexus-cutover-pre-96b66b3e74c1-20260905085804-4072165`，anchor=`/srv/subnexus-migration/cutover/20260905085804-4072165`，anchor manifest SHA=`e0b49d89e6a28044c5588afb5a536a3e87ceacfa2a7d679d324d65b14a4c100e`。
- 当前生产为 candidate ID=`86104829d490733244c9426a59e82e7a12afa3c590de2fc03f2ccb13344aebbd`、上述 `e472...` image，running/healthy/restart=0，started=`2026-09-06T14:59:13Z`。切换前 live `c3ea071f4526bdb2502444d8f18b9da4c761aa3d51be6f7e5fc19c910ca6300f` 及 temporary name 已删除；除当前生产外无额外 candidate 容器，probe 容器和目录无残留。
- 本轮没有创建新的永久回滚对象；当前生产 `86104829...` 不是回滚目标。Rollback 通过本轮 manifest 锁定上述旧 ID/image/name/anchor；固定旧对象仍 exited/restart=0。回滚默认不恢复 PostgreSQL/Redis，不修改 Nginx 或开关。
- PostgreSQL `8178576aed6f7b1cb94201832e5797907ea4d7698dbfe7b6f862cbc5a3b4f5bf`、Redis `5c7adf42247c67ba90b09248056071a57c2a4e7e0465f922d4ed799ef092533e` 身份未变且 running/restart=0；18 个受保护设置、runtime contract、备份及 sidecar、文件权限和 fixed anchor 已通过切换后审计。公网桌面/移动端 Rain UI、三图、双 Canvas 及未登录交互均通过，客服按原开关保持关闭。
- 唯一现行 rollback 单行命令位于切换手册第 13 节；仅在当前 UI 确需恢复时由维护者手动执行。此处不复制命令，避免两个操作入口发生漂移。
