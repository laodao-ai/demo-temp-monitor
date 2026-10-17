# Spec Delta

## Purpose

规定设备超温时服务如何向配置的 webhook 发通知：每台设备从正常变为超温时通知一次，失败不影响上报并在下一条超温读数时重试。值班人由 webhook 接收端决定，服务不管排班。

## ADDED Requirements

### Requirement: NTF-01 正常变为超温时通知一次

服务 SHALL 为每台已注册设备维护一个通知状态，取值为「正常」或「超温」。服务启动时、设备注册后，状态均为「正常」。读数成功入库、温度严格大于阈值、且设备状态为「正常」时，服务 SHALL 把状态改为「超温」，并向 webhook 发一次通知。阈值与 ALM-01 是同一个阈值。

#### Scenario: 首条超温读数触发通知

- **WHEN** 阈值为 50.0，已配置 webhook，已注册设备 `sensor-01` 状态为正常，上报 `{"temperature_c": 55.0}`
- **THEN** 上报响应状态码为 201
- **AND** webhook 恰好收到一次通知，`device_id` 为 `sensor-01`

#### Scenario: 读数等于阈值不通知

- **WHEN** 阈值为 50.0，已配置 webhook，已注册设备上报 `{"temperature_c": 50.0}`
- **THEN** webhook 没有收到通知

#### Scenario: 重启后重新通知

- **WHEN** 设备已处于超温并通知过，服务重启，该设备再上报一条超温读数
- **THEN** webhook 再收到一次通知

### Requirement: NTF-02 超温期间不重复通知

设备状态为「超温」时，后续成功入库的超温读数 SHALL NOT 再触发通知。告警日志不受影响，仍按 ALM-04 每条各写一条。

#### Scenario: 连续三条超温读数

- **WHEN** 阈值为 50.0，已配置 webhook，同一设备连续上报 55.0、56.0、57.0，且第一次通知成功
- **THEN** webhook 只收到一次通知，`temperature_c` 为 55
- **AND** 日志中新增三条超温告警

### Requirement: NTF-03 读数不超过阈值时回到正常

读数成功入库且温度小于或等于阈值时，服务 SHALL 把该设备状态置为「正常」，且 SHALL NOT 发恢复通知。服务 MUST NOT 做滞回：回到正常只看这一条读数是否 ≤ 阈值。

#### Scenario: 降温后再次超温

- **WHEN** 阈值为 50.0，已配置 webhook，同一设备依次上报 55.0、45.0、56.0，每次通知都成功
- **THEN** webhook 共收到两次通知，`temperature_c` 分别为 55 和 56
- **AND** 上报 45.0 时 webhook 没有收到任何请求

#### Scenario: 在阈值附近来回跳

- **WHEN** 阈值为 50.0，已配置 webhook，同一设备依次上报 50.1、49.9、50.1，每次通知都成功
- **THEN** webhook 共收到两次通知

### Requirement: NTF-04 上报失败不改状态也不通知

上报因设备未注册、请求不合法或存储出错而失败时，服务 SHALL NOT 发通知，也 SHALL NOT 改变任何设备的通知状态。

#### Scenario: 存储出错

- **WHEN** 阈值为 50.0，已配置 webhook，设备状态为正常，存储在写入读数时返回错误，请求温度为 80
- **THEN** 响应状态码为 500，webhook 没有收到通知
- **AND** 该设备下一条成功入库的超温读数触发通知

#### Scenario: 未注册设备上报高温

- **WHEN** 已配置 webhook，未注册设备 `ghost-01` 上报 `{"temperature_c": 80}`
- **THEN** 响应状态码为 404，webhook 没有收到通知

### Requirement: NTF-05 通知报文

通知 SHALL 是一个 `POST` 请求，`Content-Type` 为 `application/json`，请求体为 JSON 对象，恰好含以下字段：`event`，固定为 `"over-temperature"`；`device_id`；`temperature_c`，取触发通知的那条读数的温度；`threshold_c`。响应状态码为 2xx 视为成功，其它状态码（含 3xx）均视为失败，服务 MUST NOT 跟随重定向。

#### Scenario: 报文字段

- **WHEN** 阈值为 50.0，已配置 webhook，设备 `sensor-01` 状态为正常，上报 55.0
- **THEN** webhook 收到 `POST`，`Content-Type` 为 `application/json`
- **AND** 请求体按 JSON 解析后为 `{"event": "over-temperature", "device_id": "sensor-01", "temperature_c": 55, "threshold_c": 50}`，数值字段按数值相等比较

#### Scenario: 重定向视为失败

- **WHEN** webhook 对通知请求返回 302
- **THEN** 服务不请求重定向目标，这次通知按失败处理（见 NTF-06）

### Requirement: NTF-06 通知失败不影响上报并在下次重试

通知失败（超过 2 秒未完成、连接失败或非 2xx 响应）时，上报 SHALL 仍返回 201，服务 SHALL 写一条 ERROR 日志，消息为 `over-temperature notify failed`，字段含 `device_id` 和 `reason`。`reason` 取 `timeout`、`transport`、`status` 之一，取 `status` 时另含 `status_code`。服务 SHALL 把该设备状态退回「正常」，让下一条超温读数再试。客户端断开连接 MUST NOT 取消已开始的通知。

#### Scenario: webhook 返回 500

- **WHEN** 已配置 webhook，webhook 对通知返回 500，设备状态为正常，上报 55.0
- **THEN** 上报响应状态码为 201
- **AND** 日志中新增一条 ERROR，消息为 `over-temperature notify failed`，`reason=status`，`status_code=500`

#### Scenario: 失败后下一条超温读数重试

- **WHEN** 同一设备上报 55.0 时通知失败，随后 webhook 恢复正常，该设备再上报 56.0
- **THEN** webhook 收到 `temperature_c` 为 56 的通知

#### Scenario: webhook 不响应

- **WHEN** 已配置 webhook，webhook 收到请求后不返回响应，设备状态为正常，上报 55.0
- **THEN** 上报在 2 秒通知超时之后返回 201，不会一直等下去
- **AND** 日志中新增一条 ERROR，`reason=timeout`

### Requirement: NTF-07 同一设备并发超温只通知一次

同一台设备的多条超温读数并发入库、且此前状态为「正常」时，服务 SHALL 只发一次通知。只要有一次通知正在发送，就视为该设备已处于「超温」。

#### Scenario: 并发两条超温读数

- **WHEN** 已配置 webhook，设备状态为正常，同一设备的两条超温读数并发上报，通知成功
- **THEN** 两个上报响应都是 201，webhook 只收到一次通知

### Requirement: NTF-08 注销清除通知状态

设备注销成功时，服务 SHALL 清除该设备的通知状态。之后以同一 ID 重新注册的设备，状态从「正常」开始。

#### Scenario: 注销后重新注册

- **WHEN** 已配置 webhook，设备 `sensor-01` 已处于超温并通知过，注销后以同一 ID 重新注册，再上报 55.0
- **THEN** webhook 再收到一次通知

### Requirement: NTF-09 未配置 webhook 时不通知

`TM_NOTIFY_WEBHOOK_URL` 未配置时，服务 SHALL NOT 发出任何通知请求，上报与告警日志的行为与未引入通知前相同。

#### Scenario: 未配置时上报超温

- **WHEN** 未配置 `TM_NOTIFY_WEBHOOK_URL`，已注册设备上报 55.0
- **THEN** 上报响应状态码为 201，日志中新增一条超温告警，没有 `over-temperature notify failed` 日志
