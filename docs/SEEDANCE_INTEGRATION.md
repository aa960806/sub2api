# Seedance 接入与维护

## 来源与范围

本实现直接迁入维护者提供的 `D:\ZCJ\src\backend` 源码，来源基线为供应商的 sub2api v0.2.4 企业版。当前项目基线为 v0.2.5。原文件清单和 SHA256 见 `seedance-source-manifest.json`。

保留原包的 Seedance provider、能力表、请求/响应 DTO、参考图账号亲和、签名下载地址解析、Range 下载、余额冻结/结算原语和模型优先/通用价格兜底。没有将 Seedance 加入 `IsOpenAICompatible`，没有复用或替换 Grok 视频接口，也没有新增侧栏。

原包没有前端，因此新增后台账号/分组/定价选项及现有“使用 Key”弹窗中的媒体调用说明。平台接线按照交付文档逐项补入当前版本，不覆盖上游整文件。

必要的安全适配与原包差异：

| 原包行为 | 本项目适配 | 原因 |
| --- | --- | --- |
| Redis 任务 24h、待退款明细 48h 过期 | 独立 PostgreSQL 日志保存任务、金额快照、账号及重试依据 | 缓存丢失不能导致冻结款失去恢复依据 |
| 上游成功后保存任务失败，只记录日志 | 先持久化请求意图；再冻结、提交；保存失败可以从意图恢复 | 避免返回不可查询的任务 |
| 创建出错就退冻结款 | 只有确定拒绝或明确失败才退款；超时/502/HTML 响应保留待确认状态 | 错误响应不证明上游未建单 |
| 过期扫描直接退款 | 后台查询真实状态并完成结算，不按任务年龄退款 | 避免给仍在生成或已经出片的任务退款 |
| 结算失败只记录日志，仍可下载 | 余额确认、配额记账和用量写入全部成功才允许下载 | 防止未结算出片及账务遗漏 |
| 客户端幂等键原样传上游 | 按用户、API Key、幂等键生成持久化的唯一上游键 | 多用户共用上游账号时不能互相碰撞 |
| 账号槽位不可用仍放行 | 保留任务、稍后重试 | 不绕过现有并发限制 |
| 在途任务依赖未删除的账号和 Key | 历史任务专用查询和结算支持软删除记录；新建仍拒绝删除对象 | 删除账号或 Key 不能丢失已接收任务的资金结算 |

以上适配保持原包供应商协议不变。供应商返回的下载 URL 仍按原代码移除 `/seedance` 前缀，并进行同源及路径校验。

## 第一版支持边界

- 独立 `seedance` 平台，API Key 类型上游账号，独立余额分组。
- 模型：`seedance2.0mini`、`seedance2.0fast`、`seedance2.0`、`seedance2.5`。时长、参考图数量、720p 分辨率按原能力表校验。
- 价格必须为正的固定 `per_request` 单价。按真实模型查价，未配置时回落到 `seedance` 通用价格。同一模型的不同时长使用同一个单价。
- 不支持订阅、合成分组、模型别名映射、阶梯价、分组/账号/用户专属倍率；后台或运行时明确拒绝，不静默切换收费方式。
- 用户余额及 API Key 总额度、5h/1d/7d 窗口限制用于准入。未结清的 Seedance 任务计入预留额度；结算中短暂重复计入预留会保守拒绝新任务，结算完成后恢复。
- 上游账号的总/日/周预算限制暂不支持异步预留，Seedance 明确拒绝这些配置；其他平台保持原行为。上游自己的余额和限流仍由供应商控制。
- 请求/任务只对原用户和原 API Key 可见。API Key 余额或额度耗尽后，仍可查询和下载它已经提交的任务；禁用账号、禁用 Key、IP 限制及身份校验不被绕过。

## 配置

默认 `gateway.media.enabled=false`，对应环境变量：

```dotenv
GATEWAY_MEDIA_ENABLED=false
GATEWAY_MEDIA_MAX_IMAGE_BYTES=12582912
GATEWAY_MEDIA_MAX_IMAGES_TOTAL_BYTES=73400320
```

1. 后台创建 Seedance API Key 账号，填写供应商令牌和实际可用 base URL。交付文档地址为 `https://api.laogou.org/seedance`。
2. 创建独立 Seedance 余额分组，绑定该账号，并配置模型白名单及 `per_request` 单价。倍率为 1。
3. 为专用测试用户创建该分组的 API Key。用用户余额/Key 额度限制测试预算。
4. 完成供应商接口验通后，在隔离候选环境开启媒体开关。后台“测试连接”只检查受保护的 balance JSON，不能证明生成成功。
5. 真实测试必须收到创建任务、最终 `succeeded` 且 `downloadable=true`，并实际下载有效视频；同时核对余额、冻结款、Key 配额和用量仅记一次。

2026-09-18 此前直连测试中，`/v1/media/videos` 返回过 502，交付包指定的 `/seedance/v1/*` 返回网站 HTML。此前测试不能作为成功出片证据。供应商应先确认实际接入地址、已有幂等请求是否落单，以及上游幂等键有效期。本次本地实现未使用真实 Key 再次付费生成，也未配置或启用生产环境。

## 用户 API（沿用交付包）

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| GET | `/v1/media/models` | 读取能力表 |
| POST | `/v1/media/files` | 上传参考图，JSON `image_b64` |
| POST | `/v1/media/videos` | 建立异步任务，必须携带 `Idempotency-Key` |
| GET | `/v1/media/videos/{task_id}` | 查询任务 |
| GET | `/v1/media/videos/{task_id}/content` | 鉴权下载，支持 Range |

同时保留根 `/media/*` 五个镜像入口。使用本项目分发的 Key，而不是供应商 Key。

```http
POST /v1/media/videos
Authorization: Bearer <本项目用户Key>
Content-Type: application/json
Idempotency-Key: <本次独立生成唯一值；重试保持不变>

{"model":"seedance2.0mini","prompt":"A paper boat floating on a quiet lake","duration":5,"ratio":"16:9","resolution":"720p","camera_movement":"auto"}
```

创建返回 202 及本地 `task_id`，表示任务已被可靠接收，不代表上游已生成。客户端以相同键、相同请求体重试会取得相同任务；相同键换内容返回 409。后台 worker 完成提交、轮询与结算，用户不持续轮询也能完成计费或明确失败退款。

查询的 `status=succeeded` 还需同时满足 `downloadable=true` 才能下载。未就绪的内容入口返回 409。接口不提供生成视频的编辑器，也不自动转换为画布客户端专有协议。

## 任务恢复与运维

每个任务由带期限、带 token 校验的数据库租约串行处理。SQL 操作和保存状态都有截止时间，租约长于一次处理窗口。冻结、确认和释放继续使用现有账务仓储的唯一请求 ID；公共余额/订单/订阅实现不因 Seedance 被替换。

状态顺序：

```text
prepared → reserving → reserved → submitting → submitted
                                      ↓           ↓
                             submission_unknown   capturing → settled
                                      ↓           releasing → released
                                 manual_review
```

- 明确余额不足在提交上游前拒绝，任务保留为 `rejected`。
- 提交超时、连接丢失、502 或非预期响应会以原账号、原请求、原上游键有限重试。最多 3 次提交，首个任务接收超过 10 分钟不再重放不明确的提交；进入 `manual_review`，不擅自退款。
- `capturing` / `releasing` 的意图先落库，再执行幂等账务操作。崩溃后沿原方向继续，不能从确认扣费切换成退款。
- 结算记录和任务 ID 不按 TTL 清除。已完成任务会清掉生成 prompt/参考图请求内容，保留查账及去重所需信息。过期参考图元数据分批自动清理。
- 修改上游账号 Key 或 base URL 会使旧任务的凭据指纹校验失败。先停止新建并处理在途任务，再更换凭据；不要把旧任务切换到另一个账号。
- 原账号、用户或 API Key 被软删除后，已接收任务仍可由后台完成历史结算；不会恢复它们的登录或调用权限。已删 Key 不再累加失效的授权配额/时间窗，但保留同一笔扣费去重和用量记录。专用结算入口校验任务、原身份、金额、方向和已冻结凭据，通用账务实现保持不变。真实硬删除或移除凭据需要人工核对，不能伪造身份补账。

只读排查示例（不显示 prompt 或 API 凭据）：

```sql
SELECT task_id, user_id, api_key_id, unit_price, phase,
       record->>'job_id' AS upstream_job_id,
       record->>'upstream_key' AS upstream_idempotency_key,
       record->>'last_error' AS last_error,
       created_at, updated_at
FROM subnexus_seedance_tasks
WHERE phase NOT IN ('settled', 'released', 'rejected')
ORDER BY created_at;
```

`manual_review` 需供应商按上游幂等键确认任务、费用和可恢复的 job ID，再进行有依据的专项恢复。当前没有一键强制退款入口；不要直接改用户余额、删除任务日志或通过新幂等键重建同一笔请求。正常传输/存储故障会自动恢复，结果长期不明时资金留有完整核对依据。

## 上线、停止与回滚

`9017_subnexus_seedance_media.sql` 只新增独立表/索引，不重写或清理用户、余额、订单、历史用量与订阅数据。日常 Seedance 请求会通过现有账务代码改变对应用户的余额和冻结款，这是新服务正常计费，不是发布时的数据迁移。

首次发布保持默认关闭；启用前验证完整出片和账务。关闭媒体开关会停止新建/上传，但仍保留后台恢复、查询和下载。回滚到不含 Seedance 的旧镜像之前，必须先停止新建并处理全部非终态任务，或者保留能够处理这些任务的受控 worker；单纯切旧镜像不会自动结清冻结款。新增表应保留，不执行破坏性回滚迁移。

## 后续合并上游

媒体业务保持在 `backend/internal/custom/business/media`，桥接在独立 `service/media_bridge.go` 和 `handler/media_bridge.go`。上游合并重点检查平台目录、账号/分组校验、鉴权链、模型白名单、定价接口、Wire 构造及账务幂等原语。不要整体替换上游文件，也不要把 Seedance 加入文本平台兼容列表。

验证至少包括前端 typecheck/build/测试、后端单元测试、独立 PostgreSQL 的任务/计费集成测试、现有路由回归。供应商协议变动时优先对照原包 provider 及能力表，避免仅改 base URL 就假定两种 `/v1/media` 与 `/seedance/v1` 协议等价。

## 本地验证结果（2026-09-18）

- 后端最终全量 unit 测试通过，共 58 个有测试的包；最终包含前端的 Go embed 编译通过。
- 前端完整回归曾通过348文件/2494测试，后续变更的模型广场29项、账号/语言101项测试通过；最终typecheck、build与所有改动文件ESLint通过。
- 真实隔离PostgreSQL媒体集成测试通过：包含资金去重、并发预留、崩溃恢复、归档去重、在途软删除、非法历史结算拒绝，以及过期参考图清理。临时测试数据库已停止。
- 验证日志与来源/范围核对保存在 `F:\MySub2\candidate-transfer\seedance-*`。没有运行生产实例、真实供应商出片、Docker全量集成套件或golangci-lint；本地结果不能代替上线验收。
