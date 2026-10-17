# Spec Delta

## RENAMED Requirements

- FROM: `### Requirement: ALM-04 每条超温读数各告警一次`
- TO: `### Requirement: ALM-04 每条超温读数各写一条告警日志`

## MODIFIED Requirements

### Requirement: ALM-04 每条超温读数各写一条告警日志

系统 SHALL 对每一条成功入库的超温读数各写一条告警日志。告警日志 MUST NOT 做去重、合并或滞回。本要求只约束告警日志；告警通知按告警状态切换发送，见 `alarm-notification`。

#### Scenario: 连续两条超温读数

- **WHEN** 阈值为 50.0，同一设备连续上报 55.0 和 56.0
- **THEN** 日志中新增两条告警日志，`temperature_c` 分别为 55 和 56

#### Scenario: 启用告警通知时告警日志仍逐条写

- **WHEN** 配置了 `TM_ALARM_WEBHOOK_URL`，阈值为 50.0，同一设备连续上报 55.0 和 56.0
- **THEN** 日志中新增两条告警日志
- **AND** 接收方只收到一条 `alarm`
