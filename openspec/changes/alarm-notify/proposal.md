# Proposal

## Why

超温告警现在只写一条 WARN 日志，值班的人不看日志就不知道设备超温了。需要在设备超温时主动通知到值班的人。基线 change 把「告警通知渠道」列为非目标，本 change 有意放开这一条，把项目从四块扩到五块。

## What Changes

- 新增第五块「超温通知」：设备从正常变为超温时，服务向一个配置的 webhook 发一次通知。值班人是谁由 webhook 接收端决定，服务不管排班。
- 每台设备在内存里有一个「正常 / 超温」状态。入库成功的读数 ≤ 阈值即回到正常；不做滞回，不发恢复通知；注销清状态；重启后状态丢失。
- 通知在上报请求里同步发，超时 2 秒。失败时写 ERROR 日志，上报仍返回 201，并把设备状态退回正常，下一条超温读数再试。
- 告警日志不变：每条超温读数仍各写一条 WARN（ALM-04 只澄清适用范围）。
- 新增配置键 `TM_NOTIFY_WEBHOOK_URL`。未配置时不通知，启动日志标出 `notify=off`；配置了但不合法时启动失败，退出码 1。
- webhook URL 视同秘密：不进仓、不出现在日志、HTTP 响应和脚本输出里。
- 报文是通用 JSON，不适配任何具体厂商的群机器人格式。

## Capabilities

### New Capabilities

- `over-temperature-notify`：超温通知——何时发、发什么、失败如何处理、每台设备的通知状态如何变化。

### Modified Capabilities

- `over-temperature-alarm`：ALM-04 的「不去重」限定为告警日志，通知的发送规则交给 `over-temperature-notify`。
- `service-runtime`：RUN-01 由三个配置键变为四个；RUN-02 启动日志加通知开关；RUN-03 新增 webhook URL 非法的启动失败；RUN-04 脱敏对象加上 webhook URL；RUN-05 示例配置含 webhook 占位值。
- `test-runner`：TST-05 单元层覆盖通知；TST-08 端到端用例加上「webhook 收到一次通知」。

## Impact

- 代码：`internal/config`（新键与校验）、`internal/service`（通知状态与调用）、新增 `internal/notify`（接口、webhook 实现、内存假实现）、`internal/redact`（多一个秘密）、`cmd/temp-monitor/main.go`（组装）、`scripts/lib/local_env.sh`（已知键列表）。
- 配置与文档：`config/example.env`、`README.md`（范围由四块变五块、新配置项）、`openspec/config.yaml` 的 context（模块数）、`openspec/INDEX.md`。
- 外部依赖：无新增 Go 依赖，用标准库 `net/http`。运行时多一个出站 HTTP 调用。
- 接口：HTTP 接口不变。`service.New`、`redact.Handler` 的现有调用点保持可编译。
