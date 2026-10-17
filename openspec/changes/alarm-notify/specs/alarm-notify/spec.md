# Spec Delta

## Purpose

规定如何把超温事件通知给接收方。本能力按超温事件通知，不是按告警日志逐条通知。它为每台设备带回差地判定超温事件的开始与结束，把通知以固定 JSON 尽力投递到配置的单个 webhook，并写明不重试、不持久化带来的已知限制。

## ADDED Requirements

### Requirement: NTF-01 未配置 webhook 时不通知

`TM_NOTIFY_WEBHOOK_URL` 未配置（包括空值）时，系统 SHALL NOT 发出任何通知请求。上报接口的响应，以及 ALM-01..ALM-04 规定的告警日志，与本能力引入前完全一致。

#### Scenario: 未配置 webhook 时上报高温

- **WHEN** 未配置 `TM_NOTIFY_WEBHOOK_URL`，阈值为 50.0，已注册设备 `sensor-01` 上报 `{"temperature_c": 55.0}`
- **THEN** 响应状态码为 201，日志新增一条 `over-temperature alarm`
- **AND** 没有任何通知请求发出

### Requirement: NTF-02 超温事件开始时发 triggered

设备没有未结束的超温事件时，一条超温读数（入库成功且温度严格大于告警阈值 T）SHALL 开始一个超温事件，系统发出一条 `kind` 为 `triggered` 的通知。设备注册后、首次上报前，都没有未结束的超温事件。

#### Scenario: 首条超温读数

- **WHEN** 已配置 webhook，阈值 T=50.0，回差 H=2.0，`sensor-01` 刚注册，上报 55.0
- **THEN** 上报响应为 201
- **AND** webhook 收到恰好一条通知，`kind=triggered`，`device_id=sensor-01`，`temperature_c=55`

#### Scenario: 等于阈值不开始事件

- **WHEN** 已配置 webhook，T=50.0，`sensor-01` 刚注册，依次上报 50.0、55.0
- **THEN** 50.0 不产生通知
- **AND** 55.0 产生一条 `kind=triggered`、`temperature_c=55` 的通知

### Requirement: NTF-03 超温事件进行中不重复通知

设备有未结束的超温事件时，温度大于 T−H 的读数 SHALL NOT 产生通知，事件继续。告警日志仍按 ALM-01，每条超温读数各写一条。

#### Scenario: 持续超温

- **WHEN** T=50.0，H=2.0，`sensor-01` 依次上报 55.0、56.0、58.0
- **THEN** webhook 只收到一条通知，`kind=triggered`，`temperature_c=55`
- **AND** 日志新增三条 `over-temperature alarm`

#### Scenario: 在回差区间内抖动

- **WHEN** T=50.0，H=2.0，`sensor-01` 依次上报 51.0、49.9、50.2、49.8、50.1
- **THEN** webhook 只收到一条通知，`kind=triggered`，`temperature_c=51`
- **AND** 日志新增三条 `over-temperature alarm`，温度分别为 51、50.2、50.1

#### Scenario: 停在回差区间内

- **WHEN** T=50.0，H=2.0，`sensor-01` 依次上报 55.0、49.0、55.0、40.0
- **THEN** webhook 依次收到两条通知：`kind=triggered`、`temperature_c=55`，`kind=resolved`、`temperature_c=40`
- **AND** 49.0 和第二条 55.0 都不产生通知，49.0 也不产生告警日志

### Requirement: NTF-04 回落到 T−H 及以下时发 resolved

设备有未结束的超温事件时，一条入库成功且温度小于或等于 T−H 的读数 SHALL 结束该事件，系统发出一条 `kind` 为 `resolved` 的通知。

#### Scenario: 恢复

- **WHEN** T=50.0，H=2.0，`sensor-01` 依次上报 55.0、47.5
- **THEN** webhook 依次收到两条通知：`kind=triggered`、`temperature_c=55`，`kind=resolved`、`temperature_c=47.5`

#### Scenario: 恰好等于 T−H

- **WHEN** T=50.0，H=2.0，`sensor-01` 依次上报 55.0、48.0
- **THEN** 第二条读数产生一条 `kind=resolved` 的通知

#### Scenario: 恢复后再次超温

- **WHEN** T=50.0，H=2.0，`sensor-01` 依次上报 55.0、40.0、53.0
- **THEN** webhook 依次收到 `triggered`、`resolved`、`triggered` 三条通知

#### Scenario: 没有事件时低温不通知

- **WHEN** T=50.0，H=2.0，`sensor-01` 刚注册，上报 40.0
- **THEN** 没有通知发出

### Requirement: NTF-05 回差可配置

回差 H 取自 `TM_NOTIFY_HYSTERESIS_C`，未配置时 SHALL 为 2.0。H 为 0 时，事件结束的条件就是温度小于或等于 T，与 ALM-02 对「不超温」的定义一致。

#### Scenario: 回差为 0

- **WHEN** `TM_NOTIFY_HYSTERESIS_C=0`，T=50.0，`sensor-01` 依次上报 51.0、50.0、50.1
- **THEN** webhook 依次收到 `triggered`、`resolved`、`triggered` 三条通知

#### Scenario: 回差可调大

- **WHEN** `TM_NOTIFY_HYSTERESIS_C=5`，T=50.0，`sensor-01` 依次上报 55.0、46.0、45.0
- **THEN** 46.0 不产生通知，45.0 产生 `kind=resolved`

### Requirement: NTF-06 上报失败不影响超温事件

上报因设备未注册、请求不合法或存储出错而失败时，系统 SHALL NOT 开始或结束任何超温事件，也 MUST NOT 发出通知。

#### Scenario: 存储出错时上报高温

- **WHEN** `sensor-01` 刚注册，存储在写入读数时返回错误，请求温度为 80
- **THEN** 响应状态码为 500，没有通知发出
- **AND** 之后存储恢复，`sensor-01` 上报 55.0 时产生 `kind=triggered`

#### Scenario: 存储出错时上报低温

- **WHEN** `sensor-01` 上报 55.0 产生 `triggered` 后，存储在写入读数时返回错误，请求温度为 20
- **THEN** 响应状态码为 500，没有通知发出
- **AND** 之后存储恢复，`sensor-01` 上报 40.0 时产生 `kind=resolved`

### Requirement: NTF-07 注册与注销丢弃未结束的事件

设备注册成功或注销成功时，系统 SHALL 丢弃该设备未结束的超温事件，MUST NOT 为此发出通知。因此，在超温事件进行中被注销的设备，接收方不会收到这个事件的 `resolved`。这是已知限制。

#### Scenario: 超温事件进行中注销再注册

- **WHEN** `sensor-01` 上报 55.0 产生 `triggered` 后，客户端注销 `sensor-01` 并重新注册，然后上报 40.0，再上报 55.0
- **THEN** 注销、注册和 40.0 都不产生通知
- **AND** 55.0 产生一条 `kind=triggered` 通知

### Requirement: NTF-08 并发超温只开始一个事件

同一设备没有未结束的超温事件时，多条超温读数并发入库成功，系统 SHALL 只开始一个超温事件，只发出一条 `triggered` 通知。

#### Scenario: 并发超温上报

- **WHEN** `sensor-01` 刚注册，20 个并发请求各上报一条 55.0，全部返回 201
- **THEN** webhook 恰好收到一条 `kind=triggered` 通知

### Requirement: NTF-09 通知请求格式

每条通知 SHALL 是一个发往 `TM_NOTIFY_WEBHOOK_URL` 的 HTTP `POST`，`Content-Type` 为 `application/json`。请求体是一个 JSON 对象，字段如下：

- `kind`：`triggered` 或 `resolved`
- `device_id`
- `temperature_c`：触发这条通知的读数温度
- `threshold_c`
- `hysteresis_c`
- `at`：该读数的 `recorded_at`，RFC 3339
- `since`：仅 `resolved` 有，值为同一事件 `triggered` 的 `at`

请求 MUST NOT 携带鉴权头。

#### Scenario: triggered 请求体

- **WHEN** T=50.0，H=2.0，`sensor-01` 上报 55.0 产生 `triggered`，该读数的 `recorded_at` 为 `R1`
- **THEN** webhook 收到 `POST`，`Content-Type: application/json`
- **AND** 请求体解析后为 `{"kind":"triggered","device_id":"sensor-01","temperature_c":55,"threshold_c":50,"hysteresis_c":2,"at":R1}`，没有 `since` 字段，数值字段按数值比较

#### Scenario: resolved 请求体

- **WHEN** 接着 `sensor-01` 上报 40.0 产生 `resolved`，该读数的 `recorded_at` 为 `R2`
- **THEN** 请求体解析后为 `{"kind":"resolved","device_id":"sensor-01","temperature_c":40,"threshold_c":50,"hysteresis_c":2,"at":R2,"since":R1}`

### Requirement: NTF-10 按事件发生顺序串行投递

系统 SHALL 按通知产生的顺序逐条发送，前一条的请求结束（成功、失败或超时）后，才发送下一条。

#### Scenario: 先 triggered 后 resolved

- **WHEN** webhook 处理第一条请求需要 1 秒，`sensor-01` 依次上报 55.0、40.0，两次上报的间隔远小于 1 秒
- **THEN** webhook 先收到并处理完 `kind=triggered`，之后才收到 `kind=resolved`

### Requirement: NTF-11 投递尽力而为、不重试

成功与否只看状态码，不解析响应体。webhook 返回 2xx 视为成功。以下情况视为失败：

- 返回其它状态码，包括 3xx。系统 MUST NOT 跟随重定向。
- 连接失败。
- 3 秒内未完成。

失败时，系统 SHALL 写一条 WARN 日志，消息为 `notification failed`，字段含 `kind`、`device_id`，并且 MUST NOT 重试。超温事件不因投递失败而改变，上报响应也不受影响。因此 `triggered` 投递失败后，接收方在该事件的 `resolved` 到达之前都得不到通知。这是已知限制。

#### Scenario: triggered 投递失败

- **WHEN** webhook 对第一个请求返回 500、之后的请求返回 200，`sensor-01` 依次上报 55.0、56.0、40.0
- **THEN** 三次上报的响应都是 201，日志新增一条 WARN `notification failed`，`kind=triggered`，`device_id=sensor-01`
- **AND** webhook 一共收到两次请求：一次失败的 `triggered`，一次 `resolved`，其中 `since` 等于 55.0 那条读数的 `recorded_at`
- **AND** 56.0 不产生请求

#### Scenario: webhook 超时

- **WHEN** webhook 收到请求后 10 秒才响应，`sensor-01` 上报 55.0
- **THEN** 上报响应为 201，且不等待 webhook
- **AND** 约 3 秒后日志新增一条 WARN `notification failed`

#### Scenario: webhook 返回重定向

- **WHEN** webhook 返回 302，并指向另一个地址
- **THEN** 系统不访问重定向目标，日志新增一条 WARN `notification failed`

#### Scenario: 返回 200 但响应体含错误码

- **WHEN** webhook 返回 200，响应体为 `{"errcode": 93000}`
- **THEN** 视为成功，日志中没有 `notification failed`

### Requirement: NTF-12 队列满时丢弃

尚未发送的通知最多保留 100 条。队列满时，新产生的通知 SHALL 被丢弃，系统写一条 WARN 日志，消息为 `notification dropped`，字段含 `kind`、`device_id`。上报响应不受影响。被丢弃的通知如果是 `resolved`，接收方就收不到该事件的结束。这是已知限制。

#### Scenario: 接收端卡住

- **WHEN** webhook 对第一条请求一直不响应，此后又产生 101 条通知
- **THEN** 前 100 条进入队列，第 101 条被丢弃
- **AND** 日志新增一条 WARN `notification dropped`，所有上报响应均为 201

### Requirement: NTF-13 超温事件只保存在内存

未结束的超温事件 SHALL 只保存在服务进程内存中，服务启动时没有任何未结束的事件。由此有两个已知限制：

- 重启前处于超温事件中的设备，重启后的下一条超温读数会再产生一条 `triggered`。
- 重启前的事件不会再有 `resolved`。

#### Scenario: 重启后仍超温

- **WHEN** `sensor-01` 上报 55.0 产生 `triggered`，服务重启，`sensor-01` 再上报 56.0
- **THEN** 重启后 webhook 收到一条 `kind=triggered`、`temperature_c=56` 的通知

#### Scenario: 重启后降温

- **WHEN** `sensor-01` 上报 55.0 产生 `triggered`，服务重启，`sensor-01` 上报 40.0
- **THEN** 重启后没有通知发出
