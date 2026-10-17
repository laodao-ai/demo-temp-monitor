# Spec Delta

## Purpose

规定设备的告警状态切换时如何向接收方发送告警通知：何时启用、按跨线规则何时发送、何时复位、请求格式，以及发送慢、失败、积压时如何处理。术语见仓库根目录 `GLOSSARY.md`。

## ADDED Requirements

### Requirement: NTF-01 配置了 webhook 才发告警通知

服务 SHALL 只在配置了 `TM_ALARM_WEBHOOK_URL` 时发送告警通知。未配置时 MUST NOT 发出任何告警通知请求，上报、告警日志等其余行为与未引入本能力时相同。启动日志 SHALL 写出告警通知是否启用，MUST NOT 写出 webhook 地址。

#### Scenario: 未配置 webhook

- **WHEN** 未配置 `TM_ALARM_WEBHOOK_URL`，阈值为 50.0，已注册设备 `sensor-01` 上报 `{"temperature_c": 55.0}`
- **THEN** 上报响应状态码为 201，日志中新增一条告警日志
- **AND** 服务没有发出任何告警通知请求

#### Scenario: 启动日志写明启用状态

- **WHEN** 配置了 `TM_ALARM_WEBHOOK_URL=http://127.0.0.1:9090/hook/tok-s3cret`，启动服务
- **THEN** 启动日志写明告警通知已启用
- **AND** stderr 全文不含 `tok-s3cret`

### Requirement: NTF-02 进入告警中时发 alarm

服务 SHALL 为每台设备维护告警状态，取值为「正常」或「告警中」，初始为「正常」。设备处于「正常」时，一条超温读数成功入库，状态即转为「告警中」，告警期开始，服务 SHALL 发出一条 `event` 为 `alarm` 的告警通知。设备已处于「告警中」时，后续超温读数 MUST NOT 再发告警通知。

#### Scenario: 连续三条超温读数

- **WHEN** 阈值为 50.0，已注册设备 `sensor-01` 依次上报 55.0、56.0、57.0
- **THEN** 接收方恰好收到一条告警通知，`event` 为 `alarm`，`temperature_c` 为 55
- **AND** 日志中新增三条告警日志

#### Scenario: 读数等于阈值

- **WHEN** 阈值为 50.0，处于「正常」的设备上报 50.0
- **THEN** 接收方没有收到告警通知

### Requirement: NTF-03 恢复时发 recovered

设备处于「告警中」时，一条温度小于或等于阈值的读数成功入库，即为恢复：状态转为「正常」，告警期结束，服务 SHALL 发出一条 `event` 为 `recovered` 的告警通知。设备已处于「正常」时，后续不超温的读数 MUST NOT 发告警通知。

#### Scenario: 告警期结束后再次超温

- **WHEN** 阈值为 50.0，已注册设备依次上报 55.0、48.0、47.0、53.0
- **THEN** 接收方依次收到三条告警通知，`event` 分别为 `alarm`、`recovered`、`alarm`，`temperature_c` 分别为 55、48、53

### Requirement: NTF-04 上报失败不改变告警状态

上报因设备未注册、请求不合法或存储出错而失败时，服务 MUST NOT 改变设备的告警状态，MUST NOT 发送告警通知。

#### Scenario: 存储出错时的高温上报

- **WHEN** 阈值为 50.0，处于「正常」的设备上报 80.0，存储在写入时返回错误
- **THEN** 响应状态码为 500，接收方没有收到告警通知
- **AND** 之后该设备上报 55.0 成功时，接收方收到一条 `alarm`

#### Scenario: 未注册设备上报高温

- **WHEN** 未注册设备 `ghost-01` 上报 80.0
- **THEN** 响应状态码为 404，接收方没有收到告警通知

### Requirement: NTF-05 注册、注销与重启时复位

设备注册成功或注销成功时，服务 SHALL 将该设备复位：告警状态置为「正常」，MUST NOT 发送告警通知。处于「告警中」的设备被注销时，其告警期以复位结束，接收方收不到 `recovered`。告警状态只保存在进程内存中，服务重启即复位所有设备：重启前处于告警中、重启后读数回落的设备收不到 `recovered`。

#### Scenario: 告警中注销再注册

- **WHEN** 阈值为 50.0，设备 `sensor-01` 上报 55.0 后处于「告警中」，然后被注销，再以同一 ID 注册，并上报 56.0
- **THEN** 接收方依次收到两条 `alarm`，没有收到 `recovered`

#### Scenario: 重启后持续超温

- **WHEN** 设备处于「告警中」时服务重启，重启后该设备上报一条超温读数
- **THEN** 接收方收到一条 `alarm`

### Requirement: NTF-06 告警通知请求格式

每条告警通知 SHALL 是一个发往 `TM_ALARM_WEBHOOK_URL` 的 `POST` 请求，`Content-Type` 为 `application/json`。请求体为一个 JSON 对象，含 `event`（`alarm` 或 `recovered`）、`device_id`、`temperature_c`、`threshold_c`、`at`；`at` 为触发这次通知的读数的入库时间，格式为 RFC 3339。接收方返回 2xx 视为送达。`device_id` 是同一告警期内 `alarm` 与 `recovered` 的关联键。

#### Scenario: alarm 请求内容

- **WHEN** 阈值为 50.0，处于「正常」的设备 `sensor-01` 上报 55.5 并成功入库
- **THEN** 接收方收到一个 `POST`，`Content-Type` 为 `application/json`
- **AND** 请求体的 `event` 为 `alarm`，`device_id` 为 `sensor-01`，`temperature_c` 按数值等于 55.5，`threshold_c` 按数值等于 50，`at` 为合法的 RFC 3339 时间

#### Scenario: 同一告警期的两条告警通知可按设备关联

- **WHEN** 设备 `sensor-01` 先后触发 `alarm` 与 `recovered`
- **THEN** 两条请求体的 `device_id` 都为 `sensor-01`

### Requirement: NTF-07 告警通知不影响上报响应

告警通知 SHALL 在上报响应之外异步发送。接收方慢、不可达或返回错误时，上报响应的状态码和响应体 MUST 与未启用告警通知时相同，响应 MUST NOT 等待告警通知发送完成。

#### Scenario: 接收方很慢

- **WHEN** 接收方收到请求后 3 秒才响应，处于「正常」的设备上报一条超温读数
- **THEN** 上报在 1 秒内返回 201

#### Scenario: 接收方不可达

- **WHEN** `TM_ALARM_WEBHOOK_URL` 指向一个没有服务在监听的端口，设备上报一条超温读数
- **THEN** 上报返回 201

### Requirement: NTF-08 发送失败与积压只记日志

单次发送超过 2 秒、连接失败或接收方返回非 2xx 时，服务 SHALL 写一条 ERROR 日志，含 `device_id`、`event` 和失败原因类别（超时、连接失败，或接收方状态码），MUST NOT 重试。排队中已有 100 条告警通知（不含正在发送的一条）时，新的告警通知 SHALL 被丢弃，并写一条 ERROR 日志。这两种情况下，告警状态都照常切换。

#### Scenario: 接收方返回 500

- **WHEN** 接收方对所有请求返回 500，处于「正常」的设备上报一条超温读数
- **THEN** 接收方只收到一次请求
- **AND** 日志中新增一条 ERROR，含该设备的 `device_id`、`event=alarm` 和状态码 500

#### Scenario: alarm 未送达，告警期内不再发告警通知，恢复时仍发 recovered

- **WHEN** 阈值为 50.0，接收方对第一个请求返回 500、之后返回 200，设备依次上报 55.0、56.0、57.0、45.0
- **THEN** 接收方共收到两次请求：第一次是 `alarm`（返回 500），第二次是 `recovered`
- **AND** 56.0 和 57.0 没有触发告警通知

#### Scenario: 排队已满时丢弃

- **WHEN** 接收方不响应，正在发送一条，排队中已有 100 条，此时又有一台设备进入「告警中」
- **THEN** 这条新的告警通知被丢弃，日志中新增一条 ERROR，含该设备的 `device_id`
- **AND** 该设备的告警状态仍转为「告警中」

### Requirement: NTF-09 同一设备的告警通知按切换顺序发出

同一设备的 `alarm` 与 `recovered` SHALL 按告警状态切换的先后顺序发出，前一条发送结束（送达或失败）后才发送下一条。同一设备的多条上报并发到达时，按服务处理它们的先后决定告警状态，每次切换恰好对应一条告警通知。

#### Scenario: 快速超温又回落

- **WHEN** 阈值为 50.0，设备在 100 毫秒内依次上报 55.0 和 45.0，接收方每个请求 500 毫秒后才响应
- **THEN** 接收方先完整收到 `alarm`，再收到 `recovered`

#### Scenario: 并发超温上报

- **WHEN** 处于「正常」的设备同时有 20 条超温读数并发上报，全部成功入库
- **THEN** 接收方恰好收到一条 `alarm`
