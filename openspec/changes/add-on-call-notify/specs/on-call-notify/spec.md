# Spec Delta

## Purpose

规定超温时如何把事件推给值班的人：按设备维护「正常 / 超温中」状态，带滞回地判定进入与恢复，把事件以固定 JSON 尽力投递到配置的单个 webhook，并写明不重试、不持久化带来的已知限制。

## ADDED Requirements

### Requirement: NTF-01 未配置 webhook 时不通知

`TM_NOTIFY_WEBHOOK_URL` 未配置（包括空值）时，系统 SHALL NOT 发出任何通知请求，也 MUST NOT 维护设备的超温状态。上报接口的响应与 ALM-01..ALM-04 的日志行为与本能力引入前完全一致。

#### Scenario: 未配置 webhook 时上报高温

- **WHEN** 未配置 `TM_NOTIFY_WEBHOOK_URL`，阈值为 50.0，已注册设备 `sensor-01` 上报 `{"temperature_c": 55.0}`
- **THEN** 响应状态码为 201，日志新增一条 `over-temperature alarm`
- **AND** 没有任何通知请求发出

### Requirement: NTF-02 进入超温时通知一次

设备处于正常状态，读数入库成功且温度严格大于告警阈值 T 时，系统 SHALL 把该设备转为超温中，并发出一条 `kind` 为 `alarm` 的通知。设备首次上报前、注册后，都处于正常状态。

#### Scenario: 首条超温读数

- **WHEN** 已配置 webhook，阈值 T=50.0，回差 H=2.0，`sensor-01` 处于正常，上报 55.0
- **THEN** 上报响应为 201
- **AND** webhook 收到恰好一条通知，`kind=alarm`，`device_id=sensor-01`，`temperature_c=55`

#### Scenario: 等于阈值不进入超温

- **WHEN** 已配置 webhook，T=50.0，`sensor-01` 处于正常，上报 50.0
- **THEN** 没有通知发出，`sensor-01` 仍为正常

### Requirement: NTF-03 超温中不重复通知

设备处于超温中，读数温度大于 T−H 时，系统 SHALL 保持超温中状态，MUST NOT 发出通知。日志告警仍按 ALM-01 对每条大于 T 的读数各写一条。

#### Scenario: 持续超温

- **WHEN** T=50.0，H=2.0，`sensor-01` 依次上报 55.0、56.0、58.0
- **THEN** webhook 只收到一条通知，`kind=alarm`，`temperature_c=55`
- **AND** 日志新增三条 `over-temperature alarm`

#### Scenario: 在回差区间内抖动

- **WHEN** T=50.0，H=2.0，`sensor-01` 依次上报 51.0、49.9、50.2、49.8、50.1
- **THEN** webhook 只收到一条通知，`kind=alarm`，`temperature_c=51`
- **AND** 日志新增三条 `over-temperature alarm`，温度分别为 51、50.2、50.1

#### Scenario: 停在回差区间内

- **WHEN** T=50.0，H=2.0，`sensor-01` 依次上报 55.0、49.0
- **THEN** webhook 只收到 `alarm` 一条通知，`sensor-01` 仍为超温中
- **AND** 49.0 这条读数不产生日志告警

### Requirement: NTF-04 回落到 T−H 及以下时通知恢复

设备处于超温中，读数入库成功且温度小于或等于 T−H 时，系统 SHALL 把该设备转为正常，并发出一条 `kind` 为 `resolved` 的通知。

#### Scenario: 恢复

- **WHEN** T=50.0，H=2.0，`sensor-01` 依次上报 55.0、47.5
- **THEN** webhook 依次收到两条通知：`kind=alarm`、`temperature_c=55`，`kind=resolved`、`temperature_c=47.5`

#### Scenario: 恰好等于 T−H

- **WHEN** T=50.0，H=2.0，`sensor-01` 依次上报 55.0、48.0
- **THEN** 第二条读数触发一条 `kind=resolved` 的通知

#### Scenario: 恢复后再次超温

- **WHEN** T=50.0，H=2.0，`sensor-01` 依次上报 55.0、40.0、53.0
- **THEN** webhook 依次收到 `alarm`、`resolved`、`alarm` 三条通知

### Requirement: NTF-05 回差可配置

回差 H 取自 `TM_NOTIFY_HYSTERESIS_C`，未配置时 SHALL 为 2.0。H 为 0 时，恢复条件即为温度小于或等于 T，与 ALM-02 对「不超温」的定义一致。

#### Scenario: 回差为 0

- **WHEN** `TM_NOTIFY_HYSTERESIS_C=0`，T=50.0，`sensor-01` 依次上报 51.0、50.0、50.1
- **THEN** webhook 依次收到 `alarm`、`resolved`、`alarm` 三条通知

#### Scenario: 回差可调大

- **WHEN** `TM_NOTIFY_HYSTERESIS_C=5`，T=50.0，`sensor-01` 依次上报 55.0、46.0、45.0
- **THEN** 46.0 不触发通知，45.0 触发 `kind=resolved`

### Requirement: NTF-06 上报失败不改变状态

上报因设备未注册、请求不合法或存储出错而失败时，系统 SHALL NOT 改变任何设备的状态，也 MUST NOT 发出通知。

#### Scenario: 存储出错时上报高温

- **WHEN** `sensor-01` 处于正常，存储在写入读数时返回错误，请求温度为 80
- **THEN** 响应状态码为 500，没有通知发出
- **AND** 之后存储恢复，`sensor-01` 上报 55.0 时发出 `kind=alarm`

#### Scenario: 存储出错时上报低温

- **WHEN** `sensor-01` 处于超温中，存储在写入读数时返回错误，请求温度为 20
- **THEN** 响应状态码为 500，没有通知发出，`sensor-01` 仍为超温中

### Requirement: NTF-07 注册与注销重置状态

设备注册成功或注销成功时，系统 SHALL 把该设备的状态重置为正常，MUST NOT 为此发出通知。

#### Scenario: 超温中注销再注册

- **WHEN** `sensor-01` 处于超温中，客户端注销 `sensor-01` 并重新注册，然后上报 55.0
- **THEN** 注销与注册都不产生通知
- **AND** 上报 55.0 产生一条 `kind=alarm` 通知

### Requirement: NTF-08 并发进入只通知一次

同一设备处于正常时，多条大于 T 的读数并发入库成功，系统 SHALL 只发出一条 `alarm` 通知。

#### Scenario: 并发超温上报

- **WHEN** `sensor-01` 处于正常，20 个并发请求各上报一条 55.0，全部返回 201
- **THEN** webhook 恰好收到一条 `kind=alarm` 通知

### Requirement: NTF-09 通知请求格式

每条通知 SHALL 是一个发往 `TM_NOTIFY_WEBHOOK_URL` 的 HTTP `POST`，`Content-Type` 为 `application/json`，请求体为一个 JSON 对象，字段为：`kind`（`alarm` 或 `resolved`）、`device_id`、`temperature_c`（触发这条通知的读数温度）、`threshold_c`、`hysteresis_c`、`at`（该读数的 `recorded_at`，RFC 3339）。请求 MUST NOT 携带其它鉴权头。

#### Scenario: alarm 请求体

- **WHEN** T=50.0，H=2.0，`sensor-01` 上报 55.0 触发通知，该读数的 `recorded_at` 为 `R`
- **THEN** webhook 收到 `POST`，`Content-Type: application/json`
- **AND** 请求体解析后为 `{"kind":"alarm","device_id":"sensor-01","temperature_c":55,"threshold_c":50,"hysteresis_c":2,"at":R}`，数值字段按数值比较

### Requirement: NTF-10 按事件发生顺序串行投递

系统 SHALL 按事件产生的顺序逐条发送通知，前一条的请求结束（成功、失败或超时）后才发送下一条。

#### Scenario: 先 alarm 后 resolved

- **WHEN** webhook 处理第一条请求需要 1 秒，`sensor-01` 依次上报 55.0、40.0，两次上报间隔远小于 1 秒
- **THEN** webhook 先收到并处理完 `kind=alarm`，之后才收到 `kind=resolved`

### Requirement: NTF-11 投递尽力而为、不重试

webhook 返回 2xx 时视为成功。返回其它状态码（包括 3xx，系统 MUST NOT 跟随重定向）、连接失败或 3 秒内未完成时，视为失败。失败时，系统 SHALL 写一条 WARN 日志，消息为 `notification failed`，字段含 `kind`、`device_id`，并且 MUST NOT 重试。状态转换不因投递失败而回退，上报响应不受影响。

#### Scenario: webhook 返回 500

- **WHEN** webhook 对所有请求返回 500，`sensor-01` 上报 55.0
- **THEN** 上报响应为 201，webhook 只收到一次请求
- **AND** 日志新增一条 WARN `notification failed`，`kind=alarm`，`device_id=sensor-01`
- **AND** 之后 `sensor-01` 再上报 56.0 时不再发出通知

#### Scenario: webhook 超时

- **WHEN** webhook 收到请求后 10 秒才响应，`sensor-01` 上报 55.0
- **THEN** 上报响应为 201，且不等待 webhook
- **AND** 约 3 秒后日志新增一条 WARN `notification failed`

#### Scenario: webhook 返回重定向

- **WHEN** webhook 返回 302 并指向另一地址
- **THEN** 系统不访问重定向目标，日志新增一条 WARN `notification failed`

### Requirement: NTF-12 队列满时丢弃

尚未发送的通知最多保留 100 条。队列已满时新产生的通知 SHALL 被丢弃，系统写一条 WARN 日志，消息为 `notification dropped`，字段含 `kind`、`device_id`。上报响应不受影响。

#### Scenario: 接收端卡住

- **WHEN** webhook 对第一条请求一直不响应，此后又产生 101 条通知
- **THEN** 前 100 条进入队列，第 101 条被丢弃
- **AND** 日志新增一条 WARN `notification dropped`，所有上报响应均为 201

### Requirement: NTF-13 状态只保存在内存

设备状态 SHALL 只保存在服务进程内存中。服务启动时，所有设备都处于正常。这意味着：重启前处于超温中的设备，重启后的下一条超温读数会再发一条 `alarm`；重启前未发出的 `resolved` 不会补发。

#### Scenario: 重启后仍超温

- **WHEN** `sensor-01` 上报 55.0 后进入超温中，服务重启，`sensor-01` 再上报 56.0
- **THEN** 重启后 webhook 收到一条 `kind=alarm`、`temperature_c=56` 的通知
