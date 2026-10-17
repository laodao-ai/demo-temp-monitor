# Spec Delta

## MODIFIED Requirements

### Requirement: ALM-04 每条超温读数各告警一次

系统 SHALL 对每一条超温读数各写一条告警。系统 MUST NOT 做去重、合并或滞回。

本条只约束告警日志。通知按超温段发送，规则见 `over-temperature-notify`；通知是否发出，MUST NOT 影响告警日志是否写入。

#### Scenario: 连续两条超温读数

- **WHEN** 阈值为 50.0，同一设备连续上报 55.0 和 56.0
- **THEN** 日志中新增两条超温告警，`temperature_c` 分别为 55 和 56

#### Scenario: 已通知过的设备仍写告警日志

- **WHEN** 阈值为 50.0，已配置 webhook，同一设备连续上报 55.0 和 56.0，第一条已触发通知
- **THEN** 日志中新增两条超温告警，webhook 只收到一次通知
