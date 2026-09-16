# Grok 视频请求兼容修复

## 问题与范围

2026-09-16 使用维护者提供的 `image.yydsapi.uno` 接口复现：视频客户端向 `/v1/videos` 提交 multipart 表单，网关把表单原样发给只接受 JSON 的 xAI `/v1/videos/generations`，返回 `415`。同一接口和模型使用官方 JSON 可以成功生成并下载视频。

修复仅作用于 Grok 视频生成入口；不修改账号凭证获取、模型映射、任务归属、一次性结算规则、图片接口、视频编辑/延长接口或数据库结构。客户端仍可使用 `/v1/videos` 或 `/v1/videos/generations`。

## 参数转换

| 客户端字段 | 转给 xAI 的字段 |
| --- | --- |
| `model`、`prompt` | 保留原值，沿用已有账号模型映射 |
| `seconds=6` | `duration: 6` |
| `size=1280x720` | `resolution: "720p"`、`aspect_ratio: "16:9"` |
| `size=720x1280` | `resolution: "720p"`、`aspect_ratio: "9:16"` |
| 显式 `resolution` / `aspect_ratio` | 优先于从 `size` 推导的值 |
| `input_reference` 图片文件 | `image.url` 内嵌图片 data URL |
| `input_reference` / `image_url` 图片地址 | `image.url` |
| 原生 JSON 的 `image.file_id` 和额外字段 | 保留 |

输入在内容审核、账号调度和待结算参数记录之前统一为 JSON，避免实际生成参数与审核/计费快照不一致。损坏表单、重复字段、冲突时长、非图片文件及超过 20 MiB 的单项上传返回 `400 invalid_request_error`。

当前 xAI 文档规定生成时长为 **1–15 秒**，未指定时默认 8 秒。截图中的 20 秒选项不符合该接口要求；客户端应选 6、10、12 秒或手工输入不超过 15 秒。网关明确拒绝越界值，不会把 20 秒偷偷改为 15 秒。此修复不提供视频拼接或延长功能。

正常模型分辨率为 480p/720p；1080p 是否可用由所选模型的上游能力决定。`size` 用来推导上游支持的分辨率档位与最接近的比例，不代表任意像素尺寸输出。

## 官方 JSON 请求示例

```http
POST /v1/videos/generations
Authorization: Bearer <项目 API Key>
Content-Type: application/json
```

```json
{
  "model": "grok-imagine-video",
  "prompt": "A blue ball rolls slowly on a white tabletop. Static camera, no text.",
  "duration": 6,
  "resolution": "720p",
  "aspect_ratio": "16:9"
}
```

创建成功取得 `request_id` 后，使用同一 Key 查询 `/v1/videos/{request_id}`，等到 `status=done` 再访问响应中的 `/v1/videos/{request_id}/content`。查询未完成任务可能返回 HTTP 202。下载地址需要认证，支持 Range 分段读取；创建任务成功不等于视频生成完成。

## 验证方式

普通测试不使用真实凭据、不生成付费视频。`grok_video_live_test.go` 只在显式 `-tags live` 且提供环境变量时运行；每次会创建一条真实视频请求，不自动重试创建，任务 ID 会立即保存以便继续查询。

- `SUBNEXUS_GROK_LIVE_URL`：测试网关基址，例如 `https://image.yydsapi.uno/v1`。
- `SUBNEXUS_GROK_LIVE_KEY`：仅从进程环境读取，不写入源码或结果文件。
- `SUBNEXUS_GROK_LIVE_OUTPUT`：视频、脱敏结果和任务 ID 的输出目录。
- `SUBNEXUS_GROK_LIVE_REFERENCE_IMAGE`：可选的本地参考图片，覆盖上传图片路径。

执行 `go test -tags live ./internal/service -run '^TestGrokVideoLiveMultipartLifecycle$' -count=1 -v -timeout 10m`，依次验证真实转发、创建任务、轮询完成、完整 MP4 下载、Range 请求和重复查询。该测试将本地修复后的转发器连接到指定网关，不启动本地业务库；它不代表修复代码已部署到生产。

官方依据：

- <https://docs.x.ai/openapi.json>
- <https://docs.x.ai/developers/model-capabilities/video/generation>
- <https://docs.x.ai/developers/model-capabilities/video/image-to-video>

## 2026-09-16 验证结果

使用维护者授权的同一接口和 Key，原版 multipart 请求复现 HTTP 415，官方 JSON 对照请求成功。修复后的真实转发器完成了以下两条完整链路：

| 场景 | 任务 ID | 生成与下载验证 |
| --- | --- | --- |
| multipart 文本生成 | `76490b8b-0234-9f48-9e47-678de7ea92f8` | 62.87 秒完成；MP4 971898 bytes；Range 206 |
| multipart 上传参考图 | `cbff3d60-a7cd-9c4b-8fc5-1e0da7152af0` | 63.46 秒完成；MP4 1006931 bytes；Range 206 |

两份文件均经 ffprobe 确认 H.264、1280×720、约 6.04 秒，ffmpeg 完整解码无错误；重复查询保持 `done`。原生 JSON 对照另生成一条视频，共三条成功测试视频，使用正常接口计费。

本地验证：默认 service 的 `Grok|VideoBilling` 回归、handler 的 unit 标签回归、OAuth Heavy multipart 独立回归及 `go test ./cmd/server` 通过。扩展 service/handler unit 回归共通过 1021 项（含子用例），但该轮使用仓库外 Go overlay 跳过了未被选择的旧 settings 测试中的一处编译错误：`setting_service_public_test.go:525` 引用了不存在的 `PublicSettingsInjectionPayload.PaymentBalanceDisabled`。仓库中的该文件和业务设置实现均未修改，因此不能将此结果称为全仓 unit 测试通过。真实生成测试不使用该 overlay。

脱敏日志、任务 ID 和视频保存在 `F:\MySub2\candidate-transfer\grok-video-fix`，不纳入 Git。**代码仅在本地完成修复；未部署或切换线上版本。**
