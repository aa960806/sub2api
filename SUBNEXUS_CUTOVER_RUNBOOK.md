# SubNexus 同库切换手册

本手册只适用于所有 Batch 通过隔离库验收、候选 release 已固定为完整 40 位 SHA、且维护者明确批准发布之后。当前分支阶段不得执行生产迁移、切流或开启功能。

## 1. 发布前硬门禁

- `feature/subnexus-migration` 已通过代码、后端、前端和 Docker 验证，并从目标 `main` 生成不可变 release SHA。
- 已在生产备份恢复出的隔离 PostgreSQL/Redis 上启动候选版本；目标自动迁移无 checksum mismatch，旧版本连接迁移后克隆库仍可登录并读取核心数据。
- 维护者已保存并校验 PostgreSQL custom-format 备份、Redis 恢复点、应用镜像、旧容器 inspect、Nginx 有效配置和文件存储目录快照。
- 线上只读预检证据来自目标脚本 `tools/production-deploy/subnexus-readonly-preflight.sh`，且已核对 `schema_migrations`、`atlas_schema_revisions`、真实网络、挂载和开关状态。
- 所有迁移功能仍为关闭态；逐项开启顺序固定为 Batch 1 → Batch 2 → Batch 3 → Batch 4，并为每项保留验收记录。

## 2. 只读预检

在生产服务器执行（替换容器名和公网健康 URL；不要把密码写进命令）：

```bash
sudo bash /srv/subnexus-repo/tools/production-deploy/subnexus-readonly-preflight.sh \
  subnexus-cutover https://www.yydsapi.uno/health \
  /srv/subnexus-migration/preflight
```

脚本对应用、Docker、PostgreSQL 和 Redis 只执行读取；唯一写入是证据目录中的 `evidence.txt`、SHA-256 文件和并发锁文件，不执行迁移、备份、DDL/DML、重启或切流。它会全量记录 `schema_migrations`（含旧编号）、Atlas revision 摘要、活动相关对象、开关和估算行数，并输出脱敏的 Nginx/存储配置摘要。请把脱敏后的内容回传到本地台账。证据中不得包含密码、Token、Cookie、JWT/TOTP secret、API Key、完整环境变量或完整 Nginx 配置。

## 3. 备份与候选启动

1. 维护窗口开始前停止旧应用的写流量或将入口置于维护页；数据库和 Redis 保持运行。
2. 生成并校验最终 PostgreSQL custom-format 备份和 Redis 恢复点，同时保存应用镜像及旧容器元数据。备份路径必须落在服务器专用证据目录，权限 `0700/0600`。
3. 构建候选镜像时使用审核过的完整 SHA；候选使用与旧实例相同的数据库、Redis、JWT secret、TOTP encryption key 和持久化目录。
4. 候选启动时保持所有迁移开关为 `false`。如果应用启动触发新增迁移，应先在候选日志确认完成，再进行健康和鉴权 smoke；不要手工重复执行同一迁移。

## 4. 切流与观察

1. 确认旧容器没有运行中的结算、迁移或奖励任务后停止旧应用容器，并立即启动候选容器。
2. 先访问候选本地端口 `/health`、登录、用户/API Key、余额、订阅、订单、用量、模型列表和管理端只读接口。
3. 检查容器 UID、`NoNewPrivs`、重启次数、日志中的 migration/SQL/panic 错误，再切换 Nginx/Cloudflare 上游端口。
4. 切流后执行公网健康、登录、网关只读请求和支付回调模拟；观察至少一个完整任务/结算周期。
5. 只在当前批次验收记录签字后开启一个功能开关。开关开启顺序：活动基础 → 首充/邀请 → 发票 → Battle Pass。任一异常先关闭对应开关并保留候选实例，禁止直接删除新表。

## 5. 数据核对

切换前后记录并比较用户数、余额总额、未完成订单、订阅数、用量窗口、API Key 数量及每项迁移新增表的行数。金额和订单状态以数据库查询结果为准；抽样账户使用脱敏 ID/哈希，不在聊天或日志中传输敏感凭据。

## 6. 保留策略

旧容器、旧镜像、旧源码和配置、切换前数据库/Redis 备份、Nginx 备份及服务器证据至少保留至维护者确认可以清理。禁止在验收完成前执行 `docker system prune -a`、`docker volume prune` 或删除回滚脚本。
