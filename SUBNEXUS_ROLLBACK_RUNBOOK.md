# SubNexus 回滚手册

> 当前权威状态（2026-09-11 Asia/Shanghai）：此前模型监控 run `20260911130706-3232923` 已由维护者切换成功，实际线上为 `33a9601c9330` / `b4b66b9ca08f`。本轮更新候选 `ccb69f00132d` 包含流式生成、私有测试后发布及用户跨分组时间排序，前后端测试、构建和服务器候选 Gate 已通过，完整快照兼容验证进行中，尚不能切换。最新授权明确不新建回滚目标，复用既有 v0.2.4 容器 `e389b3b1c4f6`；最终 switch 仍由维护者手动执行。本轮状态以文末最新记录为准。

回滚默认只恢复应用，不恢复数据库、不改功能开关。所有命令先在维护窗口核对真实容器名、端口、网络、脚本和 release SHA。本轮须等待切换手册第 15.4 节完成全部门禁后登记同一 full-release wrapper/run；当前无可执行命令。不得单独执行控制器、历史 rollback 或手工 `docker stop/start` 绕过 manifest、owner、新回滚目标和依赖身份校验。

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

适用于隔离演练已证明旧版本兼容新增表/可选字段的情况。本轮 switch 进程退出后，如需恢复，只能使用切换手册第 15 节同 run rollback。switch 运行中只等待返回，wrapper 会在失败路径自动尝试恢复切换前 live，不得并发执行 rollback。本轮恢复对象就是 switch 时保留的切换前 live；其完整 ID、`.Image`、`.Config.Image`、唯一名称和状态必须与 prepared manifest 一致。历史 anchor 只在 prepare/switch 提交前承担连续性门禁；一旦 switch 已开始，anchor 缺失或漂移不得阻止自动恢复、人工 recover 或正式 rollback。

先用 `docker ps`/`docker inspect` 做只读确认，再由维护者执行本轮已发布的包装器命令。本轮使用既有入口，不修改或 reload Nginx；回滚后再访问健康接口：

```bash
docker ps --format 'table {{.Names}}\t{{.Image}}\t{{.Status}}'
docker inspect <旧容器名> <候选容器名> --format '{{.Name}} {{.Config.Image}} {{.State.Running}}'
curl -fsS --max-time 8 https://<公网健康域名>/health
```

快速回滚不恢复数据库，因新增隔离表/可选字段应被旧版本忽略；切回后必须验证登录、API Key、余额、订阅、订单、支付回调、用量和健康检查。任何 Nginx 配置修复属于单独授权的操作，不能从本轮 UI 命令推导出来。

历史失败 run 的 `ROLLED_BACK`/失败证据保留审计，不得再次 switch/rollback；历史成功 run 不得作为新的 `prepare`/`switch` 输入。第 13 节 Rain run 的 pre-switch rollback 窗口已因本轮 retained-UI switch 关闭。此前 `20260905055413-3958448` 的 prepare/probe 和切换记录已被后续发布覆盖；v0.2.1 run `20260905114022-4163123` 实际已 switched。旧自动回滚未恢复 PostgreSQL/Redis，当前状态只取信于本轮最终实时检查。

### 早期历史回滚命令已撤回

早期脚本、SHA 和失败/已覆盖 run 仅作审计事实保留，不得作为新的 `prepare`/`switch` 输入；失败或已覆盖 run 也不是 rollback 入口。切换手册第 10/11/12/13/14 节的 switch/rollback 命令均已撤回。当前尚无可执行入口；只有本轮全部前置门禁完成后，切换手册第 15 节登记的同一 wrapper/run `switch` 与 `rollback` 才有效，且默认不恢复 PostgreSQL/Redis。

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
- 该 run 后续已用于较早 UI 切换，其命令已经撤回；它不承担当前生产恢复。当前窗口规则见切换手册第 13/14 节。
- 首次 UI run `20260905160223-175225` 因 prepare 后全量 settings 哈希漂移而失效，probe 在 create 前停止且无 candidate；具体改键与来源未确定。其三个大备份及 sidecar 已校验/记录后删除，manifest/settings/metadata 和 `INVALIDATED_SETTINGS_DRIFT` 保留，禁止复用。第二次 `20260905163008-194872` 在备份前被空间门禁拒绝，同样不能作为回滚入口；固定旧 `be459...` 对象与 anchor 证据未因此删除。

## 7. `F:\Rain` 首页源码直接迁移固定回滚记录（2026-09-06 23:25 Asia/Shanghai，历史，rollback 已关闭）

- UI candidate commit=`245ecd2630b96a9807df89dc02828bbb436e7624`，image=`sha256:e472d61e8db88ec5cdd0c0c4ad9e9db11b28c3495a14af02287c99b6addf23a7`。本轮 run=`/srv/subnexus-migration/cutover/20260906134705-774592` 的切换前历史证据为 `READY=prepared`、`UI_READY=application-refresh-v1`，manifest `state=prepared/ui_state=prepared/ui_commit_intent=no`，SHA=`e3809a4d6a09d469c994d38453551d466e73f49b45903aae17f8683fe63fc897`。
- 维护者执行后返回 `UI_SWITCH_COMPLETED=/srv/subnexus-migration/cutover/20260906134705-774592`；当前 `SWITCHED=switched`、`ROLLED_BACK` 不存在，manifest `state=switched/ui_state=switched/ui_commit_intent=yes`，切换后 manifest SHA=`86afbaa48b5a22cdd193eb7f95238d8a70c74b870b7151476153317a3f0ffe79`。成功 run 的 switch 命令已撤回且严禁重跑。
- UI wrapper=`/srv/subnexus-migration/tools/subnexus-ui-cutover-054507b1-20260906.sh`，SHA=`054507b15851c9547ab347f88ad21d8f9a5203be6123bfb2030e21c88806fd5d`；原控制器 SHA=`19824a87e3e1de5659cb30664750b71c5c10d374f25bda7f52e6524fe477ee65`。切换前最终审计的 prepare evidence SHA=`0f69354a5d7911a66a6c5ef01fc58ac3160bd838a78fd214f8bb7151ac125609`。
- 固定旧 SubNexus 完整容器 ID=`be459424b327ad056ea9bdc02187d6a458fe09082369b354158d6e7f7758beee`，image=`sha256:b24b585a35e0eecff497a4eb7a2be480d9a2818f4b7a9780508f2f42cb5e09cd`，名称=`subnexus-cutover-pre-96b66b3e74c1-20260905085804-4072165`，anchor=`/srv/subnexus-migration/cutover/20260905085804-4072165`，anchor manifest SHA=`e0b49d89e6a28044c5588afb5a536a3e87ceacfa2a7d679d324d65b14a4c100e`。
- 当前生产为 candidate ID=`86104829d490733244c9426a59e82e7a12afa3c590de2fc03f2ccb13344aebbd`、上述 `e472...` image，running/healthy/restart=0，started=`2026-09-06T14:59:13Z`。切换前 live `c3ea071f4526bdb2502444d8f18b9da4c761aa3d51be6f7e5fc19c910ca6300f` 及 temporary name 已删除；除当前生产外无额外 candidate 容器，probe 容器和目录无残留。
- 本轮没有创建新的永久回滚对象；当前生产 `86104829...` 不是回滚目标。Rollback 通过本轮 manifest 锁定上述旧 ID/image/name/anchor；固定旧对象仍 exited/restart=0。回滚默认不恢复 PostgreSQL/Redis，不修改 Nginx 或开关。
- PostgreSQL `8178576aed6f7b1cb94201832e5797907ea4d7698dbfe7b6f862cbc5a3b4f5bf`、Redis `5c7adf42247c67ba90b09248056071a57c2a4e7e0465f922d4ed799ef092533e` 身份未变且 running/restart=0；18 个受保护设置、runtime contract、备份及 sidecar、文件权限和 fixed anchor 已通过切换后审计。公网桌面/移动端 Rain UI、三图、双 Canvas 及未登录交互均通过，客服按原开关保持关闭。
- 该 run 不得作为新的 `prepare`/`switch` 输入，且已消费的 switch 禁止重跑。其 rollback 只曾在本轮 retained-UI switch 前的窗口内有效；当前生产已切换，该入口为 `WITHDRAWN/CLOSED after retained-UI switch`，不承担当前生产恢复。此处不复制命令，避免操作入口漂移。

## 8. 保留二开用户端 UI 的历史回滚合同（2026-09-07，已 switched）

本轮只修改用户端显示层，生产 base=`245ecd2630b96a9807df89dc02828bbb436e7624`，候选 commit=`f6f6dafe1fb2008d0a6f41dc746ae831babc3b18`，image=`sha256:59eb4c84de8b8fec11fb903dc728676e9cffacbcc435ce5ea1b60487cc910fcc`。唯一 run=`/srv/subnexus-migration/cutover/20260907045159-1121373` 保留 `READY=prepared` 和 `UI_READY=application-refresh-v1` 作为准备 marker，且新增 `SWITCHED=switched`；manifest `state=switched/ui_state=switched/ui_commit_intent=yes`，SHA256=`5cc60f478673b2615d96d2993604da934353a6abb66d932e81f268bdfa4acda3`，`ROLLED_BACK` 不存在。

prepare 已把以下固定旧 SubNexus 写入 manifest 并由严格最终审计逐项验证：

- ID：`be459424b327ad056ea9bdc02187d6a458fe09082369b354158d6e7f7758beee`
- image：`sha256:b24b585a35e0eecff497a4eb7a2be480d9a2818f4b7a9780508f2f42cb5e09cd`
- name：`subnexus-cutover-pre-96b66b3e74c1-20260905085804-4072165`
- anchor：`/srv/subnexus-migration/cutover/20260905085804-4072165`
- anchor manifest SHA256：`e0b49d89e6a28044c5588afb5a536a3e87ceacfa2a7d679d324d65b14a4c100e`

本轮 prepare 没有创建回滚容器或回滚镜像。成功 switch 后候选 `232f6c5b...` 保留为当前生产，切换前 live 与 temporary name 按精确身份清理；只有 switch 失败或实际执行 rollback 时，wrapper 才按精确 ID 清理候选。当前生产候选不能升级为新的永久回滚目标。固定旧对象、旧镜像、anchor manifest 及其必要证据不得纳入空间清理。

wrapper=`/srv/subnexus-migration/tools/subnexus-ui-cutover-dd320d09-20260907.sh`，SHA256=`dd320d0982d357704d88bd702805cca69c36304ea6f5db523a572b10bdbdf49a`。严格审计脚本 SHA256=`5239d9c17d03f7bd6dce24daed7e2c1d8216d9e98b854cafec4a364fec13f7f6`，切换前历史 evidence SHA256=`9dc1293e6ab16af8b1f805b30e39eba5ecfa55e2b07015ae251a308b70c234ea`，结果为 `FINAL_PRE_SWITCH_AUDIT=passed`、`FINAL_SWITCH_EXECUTED=false`。

当前线上容器为 `232f6c5b374605760529cfac6b765fe68ba6aafc0d5d0fc8641a6a3030d63511`，`running/healthy/restart=0`；切换后服务器审计输出 `POST_SWITCH_AUDIT=passed`，evidence SHA256=`58edc5b2d6e3ca6535ae10741ce4aed9275609f5d5e8dad1c48407372349afb9`。切换后唯一设置变化为管理员 PUT 产生的 `subnexus_invite_activities_config`，其余 17 项保持 prepare 值；清理证据为 `/srv/subnexus-migration/cleanup-retained-ui-postswitch-20260907-1121373.txt`，SHA256=`282250b4f612f154e60c7d1b42954ac005b9bb8b3e6db11710a08dacf46b6558`。

该节只记录当时的生产与回滚事实。其 wrapper/run 在 2026-09-08 新发布开始后不得作为现行恢复入口；历史命令已撤回，不能复制或改写后执行。

## 9. Rain + Glass 用户端视觉更新回滚合同（2026-09-08 历史准备快照）

- `prepare` 必须要求历史旧 SubNexus anchor 存在，并完整核验其 manifest、容器、镜像、名称、停止状态和运行合同。只有校验通过后才生成本轮 run；`prepare` 只记录当前 live，不改变 Docker 状态。
- prepared manifest 必须固定新目标的 `ui_new_rollback_id`、`ui_new_rollback_image`、`ui_new_rollback_config_image`、`ui_new_rollback_name=production-app-ui-prior-<run-id>` 和 `ui_new_rollback_state=prepared`。任一字段缺失、格式错误或与 live 不一致都必须失败关闭。
- 最终 `switch` 由维护者执行。wrapper 停止当前 live、按唯一名称重命名、验证其 `stopped` 状态及完整身份后才启动候选；候选健康并提交成功后，该 stopped 容器继续保留为本轮新回滚目标，不得删除、`docker commit` 或转换为另一套回滚方案。
- 本轮 `rollback` 仅删除经精确身份校验的候选并恢复新目标到生产名称；不恢复历史旧 SubNexus，不默认恢复 PostgreSQL/Redis，不修改 Nginx 或功能开关。新目标缺失，或 ID、`.Image`、`.Config.Image`、名称、runtime contract 任一漂移时，回滚必须停止并保留现场。
- 历史 anchor 不是本轮恢复对象。现行 wrapper 在 prepare 和 switch 提交前要求它及其容器/镜像完整；若它在 switch 已开始后缺失或漂移，wrapper 仍必须优先恢复经过 manifest 严格绑定的 previous-live，人工 recover/rollback 也不得被该二级证据阻断。空间足够时继续保留全部历史数据；空间不足时，只能在形成精确对象清单、完整身份与 SHA 记录后删除其他已确认无用的历史 run 备份或无引用垃圾。不得执行 `docker system prune`、`docker volume prune` 或模糊清理，历史 anchor、新目标和本轮 run/备份/审计不得纳入清理。
- 候选 commit=`187b128bd32d1e06ad6e08817632e7c6b5ccca92`、tree=`e81b71b7f136da0a62bd51e266f041c6312e6b0b` 已固定并推送；UI wrapper SHA256=`6b1635548887459ad408d56226fdceadbaa8d72b845e8b3a3dac3ae65815233f`，测试 SHA256=`6a682d9f33d308eb648c914519f08e1e1afdc8a041095bfc3dddd6e38db82b26`。镜像、run、备份和最终审计仍待生成；当前没有可执行的本轮 rollback 命令。完成切换前全部门禁后，唯一命令只登记在切换手册第 15 节。

## 10. 分组模型表现监控发布回滚合同（2026-09-11，全部前置通过）

本次 run=`/srv/subnexus-migration/cutover/20260911130706-3232923`，当前 `prepared/prepared/no`。一级回滚目标为本次切换前实际 live `e389b3b1c4f62fd8d9fb0eb04998b0559d21bf39ae4c1a99bdfc90441def3836` / `sha256:44e8dcf019338e050756c86aba8d2ecf73390b4d058da2ebf916aba19bea28d9`，不是此前旧 SubNexus。维护者 switch 时才停止、改名为 `subnexus-cutover-ui-prior-20260911130706-3232923` 并保留同一容器；失败恢复及人工 rollback 继续核验完整 ID、镜像及运行合同。

新增回滚镜像 tag=`subnexus-rollback:model-evaluation-20260911-890828afe0f7`，归档=`/srv/subnexus-migration/model-evaluation-20260911/rollback-image.tar`，SHA256=`a33a982c244bbc12d5b62a8200fc408305cc82186e64ec24a3d663f9a4935fb3`，已核验镜像来源及完整归档。只新增 tag/归档，没有 docker commit，没有预先停止或替换 live。归档为灾备补充，正常回滚恢复 retained-live 容器，不自动恢复 PostgreSQL/Redis。

全量生产快照已通过新版/重启/旧版/新版兼容回归；新任务模型、加密 Key 和 HTML 历史不丢失。备份、探针及最终审计均通过，代理尚未切换。唯一完整单行 switch/rollback 命令见 `SUBNEXUS_CUTOVER_RUNBOOK.md` 第 15.4 节，禁止复用历史入口。
