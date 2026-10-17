# Spec Delta

## Purpose

规定超温与恢复事件如何以 webhook 通知值班方：何时启用、按跨线规则何时发送、状态何时复位、payload 格式，以及发送慢、失败、积压时如何处理。

## ADDED Requirements

### Requirement: NTF-01 配置了 webhook 才通知

服务 SHALL 只在配置了 `TM_ALARM_WEBHOOK_URL` 时发送告警通知。未配置时 MUST NOT 发出任何通知请求，上报、告警日志等其余行为与未引入本能力时相同。启动日志 SHALL 写出通知是否启用，MUST NOT 写出 webhook 地址。

#### Scenario: 未配置 webhook

- **WHEN** 未配置 `TM_ALARM_WEBHOOK_URL`，阈值为 50.0，已注册设备 `sensor-01` 上报 `{"temperature_c": 55.0}`
- **THEN** 上报响应状态码为 201，日志中新增一条超温 WARN
- **AND** 服务没有发出任何通知请求

#### Scenario: 启动日志写明启用状态

- **WHEN** 配置了 `TM_ALARM_WEBHOOK_URL=http://127.0.0.1:9090/hook/tok-s3cret`，启动服务
- **THEN** 启动日志写明告警通知已启用
- **AND** stderr 全文不含 `tok-s3cret`

### Requirement: NTF-02 进入超温时通知一次

服务 SHALL 为每台设备维护「正常」或「超温」状态，初始为「正常」。设备处于「正常」、读数成功入库且温度严格大于阈值时，状态转为「超温」，并 SHALL 发送一条 `event` 为 `alarm` 的通知。设备已处于「超温」时，后续超温读数 MUST NOT 再发通知。

#### Scenario: 连续三条超温读数

- **WHEN** 阈值为 50.0，已注册设备 `sensor-01` 依次上报 55.0、56.0、57.0
- **THEN** 接收端恰好收到一条通知，`event` 为 `alarm`，`temperature_c` 为 55
- **AND** 日志中新增三条超温 WARN

#### Scenario: 读数等于阈值

- **WHEN** 阈值为 50.0，处于「正常」的设备上报 50.0
- **THEN** 接收端没有收到通知

### Requirement: NTF-03 回落时通知恢复

设备处于「超温」、读数成功入库且温度小于或等于阈值时，状态转为「正常」，并 SHALL 发送一条 `event` 为 `recovered` 的通知。设备已处于「正常」时，后续不超温的读数 MUST NOT 发通知。

#### Scenario: 超温后回落再超温

- **WHEN** 阈值为 50.0，已注册设备依次上报 55.0、48.0、47.0、53.0
- **THEN** 接收端依次收到三条通知，`event` 分别为 `alarm`、`recovered`、`alarm`，`temperature_c` 分别为 55、48、53

### Requirement: NTF-04 上报失败不改变状态

上报因设备未注册、请求不合法或存储出错而失败时，服务 MUST NOT 改变设备状态，MUST NOT 发送通知。

#### Scenario: 存储出错时的超温读数

- **WHEN** 阈值为 50.0，处于「正常」的设备上报 80.0，存储在写入时返回错误
- **THEN** 响应状态码为 500，接收端没有收到通知
- **AND** 之后该设备上报 55.0 成功时，接收端收到一条 `alarm`

#### Scenario: 未注册设备上报高温

- **WHEN** 未注册设备 `ghost-01` 上报 80.0
- **THEN** 响应状态码为 404，接收端没有收到通知

### Requirement: NTF-05 注册、注销与重启时状态复位

设备注册成功或注销成功时，服务 SHALL 把该设备的状态复位为「正常」，复位本身 MUST NOT 发送通知。状态只保存在进程内存中：服务重启后所有设备都从「正常」开始，因此重启前已超温、重启后回落的设备不会收到 `recovered`。

#### Scenario: 超温时注销再注册

- **WHEN** 阈值为 50.0，设备 `sensor-01` 上报 55.0 后处于「超温」，然后被注销，再以同一 ID 注册，并上报 56.0
- **THEN** 接收端依次收到两条 `alarm`，没有收到 `recovered`

#### Scenario: 重启后持续超温

- **WHEN** 设备处于「超温」时服务重启，重启后该设备上报一条超温读数
- **THEN** 接收端收到一条 `alarm`

### Requirement: NTF-06 通知请求格式

每条通知 SHALL 是一个发往 `TM_ALARM_WEBHOOK_URL` 的 `POST` 请求，`Content-Type` 为 `application/json`。请求体为一个 JSON 对象，含 `event`（`alarm` 或 `recovered`）、`device_id`、`temperature_c`、`threshold_c` 和 `at`。`at` 是触发这次通知的读数的入库时间，格式为 RFC 3339。接收端返回 2xx 视为发送成功。

#### Scenario: alarm 通知内容

- **WHEN** 阈值为 50.0，处于「正常」的设备 `sensor-01` 上报 55.5 并成功入库
- **THEN** 接收端收到一个 `POST`，`Content-Type` 为 `application/json`
- **AND** 请求体的 `event` 为 `alarm`，`device_id` 为 `sensor-01`，`temperature_c` 按数值等于 55.5，`threshold_c` 按数值等于 50，`at` 为合法的 RFC 3339 时间

### Requirement: NTF-07 通知不影响上报响应

通知 SHALL 在上报响应之外异步发送。接收端慢、不可达或返回错误时，上报响应的状态码和响应体 MUST 与未启用通知时相同，响应 MUST NOT 等待通知发送完成。

#### Scenario: 接收端很慢

- **WHEN** 接收端收到请求后 3 秒才响应，处于「正常」的设备上报一条超温读数
- **THEN** 上报在 1 秒内返回 201

#### Scenario: 接收端不可达

- **WHEN** `TM_ALARM_WEBHOOK_URL` 指向一个没有服务在监听的端口，设备上报一条超温读数
- **THEN** 上报返回 201

### Requirement: NTF-08 发送失败与积压只记日志

单次发送超过 2 秒、连接失败或接收端返回非 2xx 时，服务 SHALL 写一条 ERROR 日志，含 `device_id`、`event` 和失败原因类别（超时、连接失败，或接收端状态码），MUST NOT 重试。待发送的通知超过 100 条时，新的通知 SHALL 被丢弃，并写一条 ERROR 日志。两种情况下设备状态都照常切换。

#### Scenario: 接收端返回 500

- **WHEN** 接收端对所有请求返回 500，处于「正常」的设备上报一条超温读数
- **THEN** 接收端只收到一次请求
- **AND** 日志中新增一条 ERROR，含该设备的 `device_id`、`event=alarm` 和状态码 500
- **AND** 该设备之后的超温读数不再触发通知

### Requirement: NTF-09 同一设备的通知按发生顺序到达

同一设备的 `alarm` 与 `recovered` SHALL 按状态切换的先后顺序发出，前一条发送结束（成功或失败）后才发送下一条。同一设备的多条上报并发到达时，按服务处理它们的先后决定状态，每次状态切换恰好对应一条通知。

#### Scenario: 快速超温又回落

- **WHEN** 阈值为 50.0，设备在 100 毫秒内依次上报 55.0 和 45.0，接收端每个请求 500 毫秒后才响应
- **THEN** 接收端先完整收到 `alarm`，再收到 `recovered`

#### Scenario: 并发超温上报

- **WHEN** 处于「正常」的设备同时有 20 条超温读数并发上报，全部成功入库
- **THEN** 接收端恰好收到一条 `alarm`
