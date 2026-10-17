# Spec Delta

## MODIFIED Requirements

### Requirement: ALM-04 每条超温读数各告警一次

系统 SHALL 对每一条超温读数各写一条告警日志。告警日志 MUST NOT 做去重、合并或滞回。本要求只约束 WARN 告警日志；webhook 通知按跨线规则发送，见 `alarm-notification`。

#### Scenario: 连续两条超温读数

- **WHEN** 阈值为 50.0，同一设备连续上报 55.0 和 56.0
- **THEN** 日志中新增两条超温告警，`temperature_c` 分别为 55 和 56

#### Scenario: 启用通知时日志仍逐条写

- **WHEN** 配置了 `TM_ALARM_WEBHOOK_URL`，阈值为 50.0，同一设备连续上报 55.0 和 56.0
- **THEN** 日志中新增两条超温告警
- **AND** 接收端只收到一条 `alarm` 通知
