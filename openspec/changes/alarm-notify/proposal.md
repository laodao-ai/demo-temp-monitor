# Proposal

## Why

目前超温只写一条 WARN 日志。值班的人只有去翻日志才能知道发生了超温，没有人会盯着日志看。这次改动把每条超温告警推送给一个外部接收端（通过 webhook），由接收端去找值班的人。术语见 `GLOSSARY.md`。

基线 `temp-monitor-baseline` 的 Non-Goals 里排除了「告警通知渠道（只写日志）」。本改动有意推翻这一条，其余 Non-Goals（告警去重、滞回、按设备阈值）保持不变。

## What Changes

- 新增可选配置项 `TM_ALARM_WEBHOOK_URL`。不配置时行为与现在完全一致。
- 配置后，每条超温告警（与 ALM-01 的告警条件相同）都会异步推送一次：POST 一条 JSON 到该地址，内容包括结构化字段，以及一句给人看的 `summary`。推送不去重，`device_id` 作为合并键，由接收端负责合并。
- 推送失败（超时、连不上、非 2xx）或待发推送已满 100 条时，丢弃这次推送并记一条 ERROR 日志（`alarm push failed` / `alarm push dropped`），不重试，不落库。上报响应仍为 201，WARN 日志照写。
- 推送地址视为凭据：接收端的 token 常放在地址里，所以日志和启动报错都不得输出它的值、路径或查询参数。
- 地址不是合法的 http(s) 绝对地址时，启动失败。`http` 与 `https` 都接受，示例配置的注释提醒带 token 的地址用 `https`。
- 启动日志用 `alarm_push=true|false` 标明是否启用推送。
- 优雅退出时，在原有 5 秒预算内尽量发完待发推送；没发完的丢弃，并记一条 WARN（`alarm push dropped at shutdown`）写明丢弃条数。
- `config/local.env` 的已知键从 3 个变成 4 个，服务与 `scripts/lib/local_env.sh` 同步修改。
- `config/example.env` 增加一行注释掉的占位地址，主机为 `127.0.0.1`。

## Capabilities

### New Capabilities

（无。推送属于超温告警的一部分，归入现有的 `over-temperature-alarm`。）

### Modified Capabilities

- `over-temperature-alarm`：Purpose 去掉「不做通知」；新增 ALM-05 至 ALM-08：推送每条超温告警、推送不影响上报、推送失败和待发推送已满时的处理。ALM-01 至 ALM-04 不变。
- `service-runtime`：RUN-01 增加第四个配置键；RUN-02 的启动日志标明是否启用推送；RUN-03 增加 URL 非法时启动失败；RUN-04 把「不出现在输出中」扩展到推送地址；RUN-05 示例配置增加占位行和 https 提醒；RUN-06 退出时处理待发推送。
- `test-runner`：TST-08 的端到端用例增加一步，断言本机接收端收到推送。

## Impact

- 代码：`internal/config`（新键与校验）、`internal/service`（`Report` 在告警时调用推送接口）、新包 `internal/push`（推送接口、webhook 实现、空实现）、`cmd/temp-monitor/main.go`（组装与退出顺序）、`scripts/lib/local_env.sh`（已知键）、`config/example.env`、`README.md`（配置表与范围说明）。
- 接口：`service.New` 增加一个推送接口参数，4 处调用点要跟着改。HTTP 接口和响应不变。
- 依赖：只用标准库 `net/http`，不加第三方依赖。
- 外部系统：需要一个能接收 JSON POST 的接收端。仓库里不出现任何真实地址。
- 文档：新增 `GLOSSARY.md`（超温告警、推送、接收端）；新增 `openspec/adr/0006-alarm-push-best-effort.md`。
- 决策记录：推送的送达保证（只发一次、不重试、不落库）见 ADR 0006。
- 风险（未解决的前提）：**接收端是否按 `device_id` 合并重复告警，目前不知道。** 如果不合并，持续超温时每次上报都会推一次，可能刷屏。届时另开一个改动，做按设备的触发/恢复推送。
- 项目上下文：`openspec/config.yaml` 的 context 里还写着「超温告警写日志」「不加表外功能」。本改动落地后这段描述会过时，需要维护者自行更新，本改动不修改它。
