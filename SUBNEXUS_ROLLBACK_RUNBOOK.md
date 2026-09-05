# SubNexus 回滚手册

> 当前权威状态：2026-09-06（Asia/Shanghai）。UI run `20260905163754-200276` 已 `READY=prepared`，尚未切换；stopped probe、备份、Gate 和最终只读复核通过。首次因后续设置哈希漂移失效，第二次备份前空间不足停止，均不得用于回滚交接。第 6 节固定旧回滚对象不变。

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

适用于隔离演练已证明旧版本兼容新增表/可选字段的情况。旧 `/root/...` 占位脚本示例已撤回，本轮只接受切换手册第 12 节在最终就绪后生成的包装器单行命令；该命令必须恢复既有旧 SubNexus，不能把当前 v0.2.1 容器新建为永久回滚对象。

先用 `docker ps`/`docker inspect` 做只读确认，再由维护者执行本轮已发布的包装器命令。本轮使用既有入口，不修改或 reload Nginx；回滚后再访问健康接口：

```bash
docker ps --format 'table {{.Names}}\t{{.Image}}\t{{.Status}}'
docker inspect <旧容器名> <候选容器名> --format '{{.Name}} {{.Config.Image}} {{.State.Running}}'
curl -fsS --max-time 8 https://<公网健康域名>/health
```

快速回滚不恢复数据库，因新增隔离表/可选字段应被旧版本忽略；切回后必须验证登录、API Key、余额、订阅、订单、支付回调、用量和健康检查。任何 Nginx 配置修复属于单独授权的操作，不能从本轮 UI 命令推导出来。

历史失败 run 的 `ROLLED_BACK`/失败证据保留审计，不得再次 switch/rollback；历史成功 run 也不能直接用作本轮 UI 入口。此前 `20260905055413-3958448` 的 prepare/probe 和切换记录已被后续发布覆盖；v0.2.1 run `20260905114022-4163123` 实际已 switched。旧自动回滚未恢复 PostgreSQL/Redis，当前状态只取信于本轮最终实时检查。

### 历史回滚命令已撤回

旧脚本、旧 SHA 和历史 run 仅作审计事实保留，不得直接重试或复用。切换手册第 10/11 节的旧命令正文已撤回；本轮 rollback 只使用切换手册第 12 节绑定最终 run 的单行命令，由维护者手动执行，默认不恢复 PostgreSQL/Redis。

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

## 6. 本轮 UI 固定回滚对象（2026-09-06 Asia/Shanghai，READY=prepared）

- UI candidate commit=`b1ed483ea5fc648cb3c15fcf2e7040e68a151a41`，image=`sha256:32f14750ce73da00dc4c5146b1d9ad6c4420ee2c3dffe098798e41a123c6bd2c`；本轮 run=`/srv/subnexus-migration/cutover/20260905163754-200276` 已 `READY=prepared`，manifest `state=prepared/ui_state=prepared`。
- UI 包装器 commit=`33d43615c6e17e3f2ae5429f986ad636e971b8cb`；路径 `/srv/subnexus-migration/tools/subnexus-ui-cutover-eef1d8f-20260905.sh`，SHA256=`eef1dfa31c71cfe33096d107561c594e0b509455b65db0caec824196d1cec77d`。原控制器 `/srv/subnexus-migration/tools/subnexus-production-cutover-19824a87-20260905-v021.sh`，SHA256=`19824a87e3e1de5659cb30664750b71c5c10d374f25bda7f52e6524fe477ee65`；二者独立校验。
- 旧 SubNexus 完整容器 ID=`be459424b327ad056ea9bdc02187d6a458fe09082369b354158d6e7f7758beee`，名称 `subnexus-cutover-pre-96b66b3e74c1-20260905085804-4072165`；旧 image ID 前缀 `b24b585`；anchor run=`/srv/subnexus-migration/cutover/20260905085804-4072165`。锚定 run 供包装器读取身份合同，不是让维护者重新执行旧控制器命令的入口。
- 当前线上 v0.2.1 容器 ID 前缀 `9753053d8bd9` 不替代上述旧 SubNexus；本轮不创建新的永久回滚对象。不得删除该旧容器、镜像、anchor manifest 或其备份/证据。
- 新 prepare 的备份/manifest、固定旧对象合同、stopped probe 和最终只读复核均已通过；尚未执行 switch/rollback，不恢复数据库或修改 Nginx。人工命令只见切换手册第 12 节，且必须绑定同一 run。
- 首次 UI run `20260905160223-175225` 因 prepare 后全量 settings 哈希漂移而失效，probe 在 create 前停止且无 candidate；具体改键与来源未确定。其三个大备份及 sidecar 已校验/记录后删除，manifest/settings/metadata 和 `INVALIDATED_SETTINGS_DRIFT` 保留，禁止复用。第二次 `20260905163008-194872` 在备份前被空间门禁拒绝，同样不能作为回滚入口；固定旧 `be459...` 对象与 anchor 证据未因此删除。
