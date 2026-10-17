# Proposal

## Why

超温告警现在只写一条 WARN 日志，值班的人不会收到任何消息，只有翻日志才能发现设备超温。这次要让超温和恢复这两件事主动送到值班的人那里。baseline 当初把「告警通知渠道」列为 Non-Goal，本 change 有意撤销这一条。

## What Changes

- 新增可选配置 `TM_ALARM_WEBHOOK_URL`。配置后，服务把告警事件以 JSON POST 到这个地址。谁在值班、怎么升级，由接收方（值班平台或群机器人）决定，服务不维护值班人或排班。
- 通知按「跨线」触发：每台设备有「正常 / 超温」两个状态。进入超温时发一条 `alarm`，回落到阈值及以下时发一条 `recovered`。持续超温不重复通知。
- 设备状态只放内存。重启、注册、注销都会把该设备的状态复位为「正常」。
- 通知异步发送，不影响上报响应，上报接口仍返回 201。只有一个发送者，按入队顺序发送；队列有上限，满了就丢弃并写 ERROR。发送失败不重试。
- 收到终止信号时，在现有的 5 秒退出时限内尽量发完队列中的通知，剩下的丢弃，并写一条 WARN 记录丢弃条数。
- webhook 地址按凭据对待：日志、stderr 中不得出现它的值。地址格式非法时启动失败，报错不回显值。
- WARN 告警日志保持不变，仍然每条超温读数写一条。ALM-04 改为只约束日志。
- 未配置 `TM_ALARM_WEBHOOK_URL` 时，行为与现在完全一样。

## Capabilities

### New Capabilities

- `alarm-notification`：超温和恢复事件的 webhook 通知，包括跨线触发规则、状态复位、异步发送与失败处理、退出时的处理、启用状态和 payload 格式。

### Modified Capabilities

- `over-temperature-alarm`：ALM-04「每条超温读数各告警一次」明确为只约束 WARN 日志，通知的去重规则交给 `alarm-notification`。
- `service-runtime`：
  - RUN-01 已知配置键从三个增加到四个。
  - RUN-03 增加 webhook 地址非法时启动失败。
  - RUN-04 的脱敏范围覆盖 webhook 地址。
  - RUN-06 退出时处理未发完的通知。
- `test-runner`：TST-05 单元层的覆盖范围增加告警通知。

## Impact

- **代码**：
  - `internal/config`：新增配置键与校验。
  - `internal/service`：新增跨线状态，Register / Deregister 时复位状态。
  - 新增一个通知发送包。
  - `internal/redact`：脱敏覆盖 webhook 地址。
  - `cmd/temp-monitor/main.go`：组装通知发送者，退出时处理队列。
  - `scripts/lib/local_env.sh`：已知键加上新键。
- **配置与文档**：`config/example.env` 加上新键（值留空）；README 的配置表；`openspec/INDEX.md` 加上新能力。
- **依赖**：只用标准库的 `net/http`，不新增第三方依赖。
- **数据库**：不改表结构。
- **项目约定**：`openspec/config.yaml` 的 `context` 中写有「模块只有四个……超温告警写日志」「不加表外功能」，与本 change 冲突。是否修改这段约定由维护者决定，不在本 change 的产出范围内。`over-temperature-alarm` 主 spec 的 Purpose 中「不做……通知」也需要在归档时改写。
