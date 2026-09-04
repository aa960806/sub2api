# SubNexus 回滚手册

回滚按风险从低到高执行，默认只回滚应用或关闭功能，不恢复数据库。所有命令先在维护窗口核对真实容器名、端口、网络和 release SHA；不得使用历史文档中的硬编码值代替实时 inspect。对于本次 `subnexus-production-cutover.sh` 生成的 prepared run，优先使用该 run 对应的脚本 `rollback RUN_DIRECTORY`，不要用手工 `docker stop/start` 绕过 manifest、owner 和依赖身份校验。

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

适用于隔离演练已证明旧版本兼容新增表/可选字段的情况：

```bash
SUBNEXUS_CUTOVER_CONFIRM=I_UNDERSTAND_APPLICATION_ROLLBACK SUBNEXUS_CUTOVER_APP_DATA_OWNER_CONFIRM=I_UNDERSTAND_NON_ROOT_APP_DATA_OWNER SUBNEXUS_CUTOVER_APP_DATA_OWNER_UID=1000 SUBNEXUS_CUTOVER_APP_DATA_OWNER_GID=1000 /root/subnexus-production-cutover-<approved-sha>-<script-sha>.sh rollback <RUN_DIRECTORY>
```

先用 `docker ps`/`docker inspect` 和 `nginx -t` 做只读确认；上面的脚本会按 manifest 精确停止候选、恢复旧容器和设置。切流恢复仍需由维护者根据实际 Nginx 配置手动执行，再访问健康接口：

```bash
docker ps --format 'table {{.Names}}\t{{.Image}}\t{{.Status}}'
docker inspect <旧容器名> <候选容器名> --format '{{.Name}} {{.Config.Image}} {{.State.Running}}'
nginx -t && nginx -s reload
curl -fsS --max-time 8 https://<公网健康域名>/health
```

如果 Nginx 已切到候选端口，先恢复切换前配置副本再 reload。快速回滚不恢复数据库，因新增隔离表/可选字段应被旧版本忽略；切回后必须验证登录、API Key、余额、订阅、订单、支付回调、用量和健康检查。

当前 prepared run 的固定人工回滚命令（只有在切换已开始或候选已启动后执行）为：

```bash
sudo -n env -u DOCKER_HOST -u DOCKER_CONTEXT -u DOCKER_CONFIG -u DOCKER_TLS_VERIFY -u DOCKER_CERT_PATH -u DOCKER_API_VERSION SUBNEXUS_CUTOVER_CONFIRM=I_UNDERSTAND_APPLICATION_ROLLBACK SUBNEXUS_APPROVED_CUTOVER_SCRIPT_SHA256=ba0f4c1eeddcad82978028ae94f2e97b9a94cd54604c45a3bb847392dfb71064 SUBNEXUS_CUTOVER_APP_DATA_OWNER_CONFIRM=I_UNDERSTAND_NON_ROOT_APP_DATA_OWNER SUBNEXUS_CUTOVER_APP_DATA_OWNER_UID=1000 SUBNEXUS_CUTOVER_APP_DATA_OWNER_GID=1000 /srv/subnexus-migration/tools/subnexus-production-cutover-af82a6877-ba0f4c1e.sh rollback /srv/subnexus-migration/cutover/20260904175519-3701605
```

该命令只回滚应用容器和已关闭的 rollout gates，不恢复 PostgreSQL/Redis 备份；执行前仍需核对实时容器、依赖身份和入口配置。

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
