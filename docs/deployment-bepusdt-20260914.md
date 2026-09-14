# BEpusdt BSC 支付发布交接（2026-09-14）

状态：所有发布前置工作已完成，**尚未执行生产应用切换或启用 USDT**。

## 人工命令

在 OVH 服务器终端执行切换：

```bash
sudo -n bash /srv/subnexus-migration/tools/release-bepusdt-8b4ba1b1.sh switch
```

回滚到已保留的旧版本：

```bash
sudo -n bash /srv/subnexus-migration/tools/release-bepusdt-8b4ba1b1.sh rollback
```

同一入口的 `check` 只做前置检查，不切换应用、不启用支付。若应用已切换但支付配置激活失败，修复所报配置问题后可重跑同一条 `switch` 命令；脚本会核对状态及镜像，接着完成配置，不重复切换应用。

## 支付配置与范围

- 固定网络：BSC/BEP20（`usdt.bep20`）。充值、订阅继续使用现有 `/purchase` 页面和原支付选择器，无新侧栏或独立充值页面。
- 收银台：[https://ustd.yydsapi.uno](https://ustd.yydsapi.uno)。专属 HTTPS、公开收银台路径和静态资源已准备并验证，证书已纳入自动续期。原独角数卡域名的既有路由保留。
- BEpusdt 的现有全局收银台配置仍为 `shop.yydsapi.uno`，会在手动切换新应用成功后，经官方管理 API 更新为用户指定的 `ustd.yydsapi.uno`。该设置属于现有共享 BEpusdt 服务；后续收银台链接使用新域名，已有链接和商户回调不删除、不改写。
- 新应用通过内部 Docker 网络连接现有 BEpusdt 容器，无需将订单创建或管理接口开放到公网。添加内部网络连接时，应用网络合同及服务启动时间不变。
- 回调为 `https://www.yydsapi.uno/api/v1/payment/webhook/bepusdt`，返回页面为 `/payment/result`。CNY 计价，原充值倍率 10、其他支付方式、限额和手续费保持原配置。
- Token 和 BEpusdt 管理凭据仅在服务器受限目录 `/srv/subnexus-migration/payment-bepusdt-20260914/` 保存，文件 root/0600，不进入 Git。
- 切换成功后才通过现有管理 API 创建并启用 BEpusdt Provider。准备阶段未写入本项目的生产支付配置、订单或用户账务数据。

## 已验证

- 前端全量：317 个文件 / 2224 项测试通过；类型检查、构建通过。
- Linux 后端全量 `go test -p 2 ./...` 通过；带 `unit` 标签的支付 provider/handler/service 定向测试通过。
- 实际 BEpusdt v1.24.2：BSC 未付款测试订单创建、查询、取消成功；用户指定的新域名收银台与资源访问成功。未进行链上转账，未用生产用户订单模拟到账。
- 隔离 candidate gate 通过；新→当前→新→既有回滚→新真实轮换通过。仅使用独立 PostgreSQL/Redis 和 synthetic fixture，余额、Key、订单、用量、订阅、模型监控与迁移指纹不变。
- 支付激活脚本已在隔离候选应用的真实管理 API 上验证创建、重复执行、启用和回滚前停用；生产配置未启用。
- 正式 prepare 完成，PG/Redis/app-data 备份校验通过。首次备份超过原 120 秒上限，在生产未停止的情况下，将准备阶段时限设为 600 秒后完成。
- 最终所有停止服务前的校验通过；从未启动的 runtime probe 已精确删除；线上 app/PG/Redis 的身份、启动时间不变，健康检查 HTTP 200。
- 本次无数据库迁移、恢复、用户数据清理或新永久回滚对象。服务器约剩余 24 GiB，无需清理已有回滚数据。

## 固定身份与证据

| 项目 | 固定值 |
| --- | --- |
| 应用提交 | `8b4ba1b18dee925cbd241fdd6fd4f9f91148f4c5` |
| 应用 tree | `6e3f050a27c6e20506e7a77260583cd93bea9fca` |
| 候选镜像 | `sha256:c833028c6525e1c76bf448562ef1c4f6cc5afed397ff5048fec118edea9c09da` |
| 归档 SHA256 | `145bbb6a8ef50a17641e745942ff228bdb1ff743f52323bab2f1b892123a8ced` |
| 现网应用 | `c130626b47a2911dd2a9118083aaaba3a4805d89183d44d3a3529811a7db164a` |
| prepare run | `/srv/subnexus-migration/cutover/20260914060246-716990` |
| 状态 | `prepared/prepared/no` |
| manifest SHA256 | `4b5996caaefb8e6c47f2bf10843b8d3499b412bf2456a0603353bb54e6da4d6a` |
| launcher SHA256 | `35472981dd6a45180c0641ea51786c3c52eec11697100a525664c688ca6ce370` |

候选门禁：`/srv/subnexus-migration/docker-candidate/20260914T054955Z-e507685b-6e97-4e6b-bdf9-782df39e0bc6/evidence.txt`。

兼容门禁：`/srv/subnexus-migration/docker-candidate/compat-bepusdt-5a409562dc/evidence.env`，SHA256 `3372725e324de610b12fbde4c2beab0f8ab65d057531511e5feb15af7b81bc92`。

最终审计：`/srv/subnexus-migration/diagnostics/final-bepusdt-20260914060246-716990.evidence`，SHA256 `dafaa86f5a16a72b2f823ca88d89ccb0ff35a54dd51ede2e9a25980fc38112e4`。

## 回滚约束

沿用容器 `e389b3b1c4f62fd8d9fb0eb04998b0559d21bf39ae4c1a99bdfc90441def3836`，镜像 `sha256:44e8dcf019338e050756c86aba8d2ecf73390b4d058da2ebf916aba19bea28d9`；不恢复旧数据库。

该旧版本没有 BEpusdt 支付实现。回滚入口会先检查未处理 USDT 订单；发现未处理订单便拒绝切换，保留新应用处理到账。无未处理订单时先停用新 USDT 下单，等待并检查并发订单，再回滚应用。不能跳过入口直接启动旧容器。已完成订单及用户余额、账务记录保留。
