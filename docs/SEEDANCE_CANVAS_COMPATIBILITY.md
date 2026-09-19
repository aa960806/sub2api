# Seedance 无限画布兼容修复

日期：2026-09-19。状态：本地实现与模拟上游验证完成，尚未部署；未发起真实付费生成。

## 原因与修复范围

无限画布会从 `/v1/models` 读取 OpenAI 格式的模型列表，但此前 Seedance 分组仅允许
`/v1/media/*`，因此直接返回 `This endpoint is not supported for this platform`。
放开模型列表仍不足以修复生成：画布对 Seedance 使用 Ark 格式的
`/v1/contents/generations/tasks`，并用没有 Authorization 的 GET 下载返回的视频 URL。

本次仅为 Seedance 增加下游协议适配，继续复用 `D:\ZCJ` 原包迁入的 provider 与现有任务生命周期。

| 入口 | 行为 |
| --- | --- |
| `GET /v1/models`（以及既有 `/models`） | Seedance key 返回 `object=list,data=[{id,...}]`，按分组模型白名单过滤；其他平台仍走原处理器 |
| `POST /v1/contents/generations/tasks` | `content` 转换为原有 prompt/参考图，返回 202 与 `id` |
| `GET /v1/contents/generations/tasks/:task_id` | 核对用户与原 API key，返回画布识别的状态与 `content.video_url` |
| `GET /v1/media/results/:task_id?expires=...&signature=...` | 仅凭已结算任务的短期签名下载，支持 Range；不暴露 API key 或上游凭据 |

Grok/OpenAI 图片、视频、文字路由与计费代码未改。其他平台不能进入 Seedance 的建单或查询入口。
原生 `/v1/media/*` 的鉴权与强制 Idempotency-Key 合同保留，不新增数据库迁移或生产设置。

## 画布配置

- Base URL 使用本站 API 域名，例如 `https://image.yydsapi.uno`，也可带 `/v1`。
- 必须使用绑定 **Seedance 分组** 的本站 key；媒体开关、账户调度和原有按次价格仍需配置有效。
- 推荐首次使用 `seedance2.0`、**5 秒、720p**，文字或 PNG/JPEG/WebP 参考图。
- 自动比例交给供应商默认比例处理；固定比例沿用供应商的六种比例。

| 模型 | 可用时长 | 最大参考图数 |
| --- | --- | --- |
| `seedance2.0mini` | 5、10 秒 | 9 |
| `seedance2.0fast` | 5、10、15 秒 | 9 |
| `seedance2.0` | 5、10、15 秒 | 9 |
| `seedance2.5` | 30 秒 | 30 |

所有型号仅支持 720p。当前画布默认的 **6 秒不受该供应商支持**，需要主动选 5/10/15 秒；
不会静默改成其他时长。画布源码把时长限制在最多 15 秒，因此当前画布不适合使用仅支持 30 秒的
`seedance2.5`；它仍可通过原生媒体 API 使用。

本供应商只支持参考图，不支持参考音频、参考视频、火山 `asset://` 或首尾帧角色。
参考图可全部使用内联 data URL，或全部使用公网 HTTPS 直链；暂不混用两类来源，以保持提示词中的图序。
内联图片校验格式、张数、单图及总字节上限，上传到同一上游账号后创建任务；图片字节不写入任务日志。

原包没有音频与水印开关。本适配接受画布默认的 `generate_audio=true, watermark=false`，
响应 `warnings` 明确说明最终音频/水印由供应商决定，不承诺这些开关生效；非默认值会明确拒绝。
画布当前不会展示响应 warnings，因此使用方应先阅读此限制。

## 任务、权限与下载

- `content` 中的文字和图片先转换成现有审核器可读取的 `prompt + images`，审核通过后才上传/冻结资金。
- 客户端有 Idempotency-Key 时继续按用户和 key 隔离，重复提交返回同一任务，不重复计费。
- 画布不传 Idempotency-Key 时，每次 POST 生成独立键并在响应头返回；同一任务的后台重试仍使用固定上游键。
  不传键的两个独立 POST 不保证合并为同一任务。网络中断后，不应盲目连续重新点击生成。
- 上游成功但结算尚未完成时返回 `running`，不提前暴露视频；结算完成才返回 `succeeded + content.video_url`。
- 查询仍检查原用户和原 key；额度耗尽后允许读取已付任务，但不绕过 key 停用、用户状态等既有鉴权。
- 下载 URL 的 HMAC 有效期为 10 分钟，只能读取一个已结算 Seedance 任务。它是短期持有者凭证，
  获得链接者在到期前可下载该视频；停用 key 后不能领取新链接，但已签发链接仍可能在有效期内可用。
  JWT secret 派生独立用途密钥，不暴露 JWT/secret；重启保留有效性，轮换 secret 会使旧链接立即失效。
- 过期后用原 key 重新查询任务可取得新链接。网关保留 Host，并从 TLS 或代理的 `X-Forwarded-Proto: https`
  构造下载地址；未改变现有 CORS 策略。

## 验证与边界

已通过：

```sh
go test -tags=unit -p=2 ./internal/custom/business/media ./internal/server/routes ./internal/server/middleware ./internal/handler
go build ./cmd/server
```

`TestSeedanceCanvasClientContract` 执行 2026-09-19 抓取的画布实际 JS，连接本地真实 handler/provider，
上游为 HTTP 模拟服务；覆盖模型列表、无幂等头建单、文字/双图、轮询、结算和无 Authorization 下载。
参考源与 SHA 在 `backend/internal/custom/business/media/testdata/canvas/README.md`。
同时复跑原 Grok 画布合同脚本，结果通过。

另有运行时测试覆盖平台隔离、分组模型白名单、审核阻断、跨用户/key、重复提交、结算失败恢复、
签名篡改/过期/跨任务/过长有效期、密钥轮换、Range 及媒体关闭后仍可查询。

这些证明接口适配可工作，不代表真实上游必定出片。当前画布本身最多轮询约 10 分钟；
超过后任务仍由服务端继续处理，可用任务 ID 查询，不应据此认定失败并自动退款。
生产开关、定价、供应商余额/权限、真实出片及浏览器网络/CORS 仍需发布后验证。
