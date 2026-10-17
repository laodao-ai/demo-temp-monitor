# Spec Delta

## ADDED Requirements

### Requirement: ALM-05 配置 webhook 时推送每条超温告警

配置了 `TM_ALARM_WEBHOOK_URL` 时，每写一条超温告警日志（ALM-01），系统 SHALL 向该 URL 发送一次 HTTP POST。请求头 `Content-Type: application/json`，请求体为 JSON 对象，字段为：`event`（固定为 `over_temperature`）、`device_id`、`temperature_c`、`threshold_c`、`reading_id`（入库读数的 ID）、`recorded_at`（RFC 3339）。推送与告警日志一一对应：不写告警日志的读数（ALM-02、ALM-03）MUST NOT 推送；推送 MUST NOT 去重，由接收端按 `device_id` 合并。

#### Scenario: 超温读数推送一次

- **WHEN** 阈值为 50.0，`TM_ALARM_WEBHOOK_URL` 指向一个测试接收端，已注册设备 `sensor-01` 上报 `{"temperature_c": 55.0}`，响应体 `id` 为 R
- **THEN** 接收端在 5 秒内收到恰好一次 POST，`Content-Type` 为 `application/json`
- **AND** 请求体中 `event=over_temperature`、`device_id=sensor-01`，`temperature_c` 数值为 55，`threshold_c` 数值为 50，`reading_id` 等于 R，`recorded_at` 为合法 RFC 3339 时间

#### Scenario: 连续两条超温读数推送两次

- **WHEN** 阈值为 50.0，配置了 webhook，同一设备连续上报 55.0 和 56.0
- **THEN** 接收端收到两次 POST，`temperature_c` 分别为 55 和 56

#### Scenario: 不告警的读数不推送

- **WHEN** 阈值为 50.0，配置了 webhook，已注册设备上报 50.0，未注册设备 `ghost-01` 上报 80
- **THEN** 接收端在 1 秒内没有收到任何请求

#### Scenario: 未配置 webhook

- **WHEN** 未配置 `TM_ALARM_WEBHOOK_URL`，阈值为 50.0，已注册设备上报 55.0
- **THEN** 上报响应为 201，日志中有一条超温告警，服务不发出任何推送请求，也不新增 ERROR 日志

### Requirement: ALM-06 推送不影响上报

上报响应 MUST NOT 等待推送完成。推送慢、失败或队列已满时，上报的响应状态码、响应体和超温告警日志 SHALL 与未配置 webhook 时相同。

#### Scenario: 接收端响应很慢

- **WHEN** 阈值为 50.0，配置的接收端收到请求后 10 秒才响应，已注册设备上报 55.0
- **THEN** 上报在 1 秒内返回 201
- **AND** 日志中有一条超温告警

### Requirement: ALM-07 推送失败时丢弃并记 ERROR

单次推送超过 5 秒未完成、连接失败或收到非 2xx 响应时，系统 SHALL 丢弃这条推送，并写一条级别为 ERROR 的日志：消息为 `alarm webhook failed`，字段含 `device_id`、`reading_id`、`reason`。`reason` 只取 `timeout`、`connection error`、`http status <状态码>` 之一。系统 MUST NOT 重试。

#### Scenario: 接收端返回 503

- **WHEN** 配置的接收端对每个请求返回 503，已注册设备 `sensor-01` 上报一条超温读数
- **THEN** 上报返回 201
- **AND** 接收端恰好收到 1 次请求（没有重试）
- **AND** 日志中有一条 ERROR `alarm webhook failed`，`device_id=sensor-01`，`reason="http status 503"`

#### Scenario: 接收端不可达

- **WHEN** `TM_ALARM_WEBHOOK_URL` 指向 `127.0.0.1` 上一个没有服务监听的端口，已注册设备上报一条超温读数
- **THEN** 上报返回 201，日志中有一条 ERROR `alarm webhook failed`，`reason="connection error"`

#### Scenario: 接收端超时

- **WHEN** 配置的接收端收到请求后一直不响应，已注册设备上报一条超温读数
- **THEN** 上报后 5 到 7 秒内，日志中出现一条 ERROR `alarm webhook failed`，`reason=timeout`

### Requirement: ALM-08 待发推送超过 100 条时丢弃新推送

系统 SHALL 最多保留 100 条待发推送（不含正在发送的那条）。已满时新产生的推送 SHALL 直接丢弃，并写一条 ERROR 日志：消息为 `alarm webhook dropped`，字段含 `device_id`、`reading_id`，`reason=queue full`。

#### Scenario: 接收端阻塞时连续超温

- **WHEN** 配置的接收端收到请求后一直不响应，已注册设备连续上报 150 条超温读数
- **THEN** 150 次上报都返回 201，日志中有 150 条超温告警
- **AND** 日志中 `alarm webhook dropped`（`reason=queue full`）至少 49 条
