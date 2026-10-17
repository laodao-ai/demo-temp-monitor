# Proposal

## Why

现在设备超温时，服务只写告警日志，值班的人不会收到任何消息，只有翻日志才能发现。这次要让设备进入告警中、以及恢复正常这两件事，主动送到接收方，由接收方转给值班人员。baseline 当初把「告警通知渠道」列为 Non-Goal，本 change 有意撤销这一条。术语见仓库根目录 `GLOSSARY.md`。

## What Changes

- 新增可选配置 `TM_ALARM_WEBHOOK_URL`。配置后，服务把告警通知以 JSON POST 到这个地址。由接收方（值班平台或群机器人）决定通知哪位值班人员，服务不维护值班人员或排班。
- 告警通知按跨线规则发送：每台设备有「正常」「告警中」两种告警状态。设备进入告警中时发一条 `alarm`，恢复时发一条 `recovered`。同一个告警期内不重复发送。`device_id` 用来关联同一告警期的两条告警通知。
- 告警状态只放在内存里。注册、注销、服务重启时，设备会复位，不发告警通知，所以可能多发一条 `alarm`，或收不到 `recovered`。
- 告警通知异步发送，不影响上报响应，上报接口仍返回 201。只有一个发送者，按状态切换的顺序发出。排队上限 100 条，满了就丢弃并写 ERROR。发送失败不重试：如果一条 `alarm` 没送达，这个告警期内不会再通知，恢复时照常发 `recovered`。
- 收到终止信号时，在现有的 5 秒退出时限内尽量发完已排队的告警通知，剩下的丢弃，并写一条 WARN 记录丢弃条数。
- webhook 地址按凭据对待：日志和 stderr 里不能出现它的值。地址格式非法时启动失败，报错不回显这个值。
- 告警日志保持不变，仍然每条超温读数写一条。ALM-04 改名，并明确只约束告警日志。
- 未配置 `TM_ALARM_WEBHOOK_URL` 时，行为与现在完全一样。

## Capabilities

### New Capabilities

- `alarm-notification`：设备告警状态的维护与告警通知的发送。包括跨线规则、复位、请求格式、异步发送与失败处理、启用条件。

### Modified Capabilities

- `over-temperature-alarm`：ALM-04 改名为「每条超温读数各写一条告警日志」，明确只约束告警日志；告警通知的去重规则交给 `alarm-notification`。
- `service-runtime`：
  - RUN-01：已知配置键从三个增加到四个。
  - RUN-03：webhook 地址非法时启动失败。
  - RUN-04：脱敏范围覆盖 webhook 地址。
  - RUN-06：退出时处理已排队的告警通知。
- `test-runner`：TST-05 单元层的覆盖范围增加告警通知。

## Impact

- **代码**：
  - `internal/config`：新增配置键与校验。
  - `internal/service`：新增告警状态，注册、注销时复位。
  - 新增一个发送告警通知的包。
  - `internal/redact`：覆盖 webhook 地址。
  - `cmd/temp-monitor/main.go`：组装各部分，并在退出时处理队列。
  - `scripts/lib/local_env.sh`：已知键加入新配置项。
- **配置与文档**：
  - `config/example.env` 加入新键，值留空。
  - README 的配置表。
  - `openspec/INDEX.md` 加入新能力。
  - 新增 ADR `openspec/adr/0006-alarm-notification-edge-triggered.md`，记录跨线触发、状态只放内存、不重试这三项决定。
  - 新增术语表 `GLOSSARY.md`。
- **依赖**：只用标准库 `net/http`，不新增第三方依赖。
- **数据库**：不改表结构。
- **项目约定**：`openspec/config.yaml` 的 `context` 已从「模块只有四个……超温告警写日志」改为五个模块，加入了可选的告警通知，并补充了 webhook 示例值留空的约定。`over-temperature-alarm` 主 spec 的 Purpose 已去掉「不做……通知」，改为指向 `alarm-notification`。这两处都在规划阶段改好。
