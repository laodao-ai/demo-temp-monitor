## Purpose

规定设备如何通过服务的 HTTP 接口注册和注销，包括设备 ID 的格式、重复注册、注销后读数的去向，以及同一 ID 的重新注册。

## ADDED Requirements

### Requirement: DEV-01 注册设备

系统 SHALL 接受 `POST /devices`，请求体为 `{"id": "<设备 ID>"}`。设备 ID MUST 匹配 `^[a-z0-9-]{1,64}$`。注册成功返回 201，响应体含 `id` 和服务端生成的 `registered_at`（RFC 3339 时间）。

#### Scenario: 注册新设备

- **WHEN** 客户端发送 `POST /devices`，请求体 `{"id": "sensor-01"}`，且 `sensor-01` 尚未注册
- **THEN** 响应状态码为 201，响应体 `id` 等于 `sensor-01`，`registered_at` 为合法 RFC 3339 时间
- **AND** 之后向 `sensor-01` 上报读数返回 201

#### Scenario: 重复注册

- **WHEN** `sensor-01` 已注册，客户端再次发送 `POST /devices`，请求体 `{"id": "sensor-01"}`
- **THEN** 响应状态码为 409，响应体为 `{"error": "device already registered"}`
- **AND** 该设备已有的读数不变

#### Scenario: 设备 ID 不合法

- **WHEN** 请求体的 `id` 为 `Sensor_01`、空字符串、65 个字符的字符串，或缺少 `id` 字段，或请求体不是合法 JSON
- **THEN** 响应状态码为 400，响应体含 `error` 字段，系统中不新增任何设备

### Requirement: DEV-02 注销设备

系统 SHALL 接受 `DELETE /devices/{id}`。注销成功返回 204，无响应体，同时 MUST 删除该设备的全部读数。

#### Scenario: 注销已注册设备

- **WHEN** `sensor-01` 已注册且有 3 条读数，客户端发送 `DELETE /devices/sensor-01`
- **THEN** 响应状态码为 204
- **AND** 存储中 `sensor-01` 的读数为 0 条
- **AND** 之后向 `sensor-01` 上报读数返回 404

#### Scenario: 注销未注册设备

- **WHEN** `ghost-01` 从未注册，客户端发送 `DELETE /devices/ghost-01`
- **THEN** 响应状态码为 404，响应体为 `{"error": "device not found"}`

#### Scenario: 注销时 ID 不合法

- **WHEN** 客户端发送 `DELETE /devices/BAD_ID`
- **THEN** 响应状态码为 400

### Requirement: DEV-03 注销后可重新注册

设备注销后，系统 SHALL 允许以同一 ID 重新注册。重新注册的设备 MUST 没有历史读数。

#### Scenario: 同一 ID 注销后再注册

- **WHEN** `sensor-01` 有读数，客户端先注销 `sensor-01`，再发送 `POST /devices`，请求体 `{"id": "sensor-01"}`
- **THEN** 注册响应状态码为 201
- **AND** 存储中 `sensor-01` 的读数为 0 条

### Requirement: DEV-04 并发注册同一 ID 只成功一次

多个请求同时注册同一个尚未注册的 ID 时，系统 SHALL 只让其中一个成功，其余 MUST 得到「已存在」结果（HTTP 层为 409）。两个存储实现都 MUST 满足这一点。

#### Scenario: 并发注册

- **WHEN** 20 个并发调用同时以 `race-01` 注册，此前 `race-01` 未注册
- **THEN** 恰好 1 个调用成功，其余 19 个返回「已存在」
- **AND** 存储中只有一个 `race-01`
