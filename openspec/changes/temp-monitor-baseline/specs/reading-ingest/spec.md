## Purpose

规定已注册设备如何上报温度读数并持久化，包括请求体校验、对未注册设备的拒绝，以及仓内用于模拟设备上报的脚本的行为。

## ADDED Requirements

### Requirement: ING-01 上报读数入库

系统 SHALL 接受 `POST /devices/{id}/readings`，请求体为 `{"temperature_c": <数字>}`，单位摄氏度。设备已注册时，系统 MUST 持久化这条读数，读数时间取服务端当前时间，并返回 201。响应体含 `id`、`device_id`、`temperature_c`、`recorded_at`。

#### Scenario: 已注册设备上报

- **WHEN** `sensor-01` 已注册，客户端发送 `POST /devices/sensor-01/readings`，请求体 `{"temperature_c": 23.5}`
- **THEN** 响应状态码为 201，响应体 `device_id` 为 `sensor-01`，`temperature_c` 为 23.5，`recorded_at` 为合法 RFC 3339 时间
- **AND** 存储中 `sensor-01` 的读数多出一条，温度为 23.5

#### Scenario: 温度为 0 视为有效值

- **WHEN** `sensor-01` 已注册，请求体为 `{"temperature_c": 0}`
- **THEN** 响应状态码为 201，存储中多出一条温度为 0 的读数

#### Scenario: 负温度视为有效值

- **WHEN** `sensor-01` 已注册，请求体为 `{"temperature_c": -12.5}`
- **THEN** 响应状态码为 201

### Requirement: ING-02 拒绝未注册设备的上报

设备未注册或已注销时，系统 SHALL 返回 404，响应体为 `{"error": "device not found"}`，并且 MUST NOT 持久化任何读数。

#### Scenario: 未注册设备上报

- **WHEN** `ghost-01` 从未注册，客户端发送 `POST /devices/ghost-01/readings`，请求体 `{"temperature_c": 20}`
- **THEN** 响应状态码为 404
- **AND** 存储中没有 `ghost-01` 的读数

#### Scenario: 注销与上报并发

- **WHEN** 对同一设备并发执行一次注销和一次上报
- **THEN** 上报要么返回 201，此时这条读数随注销一起被删除；要么返回 404
- **AND** 两种情况下存储中都没有该设备的读数残留

### Requirement: ING-03 校验上报请求

请求不合法时，系统 SHALL 返回 400，响应体含 `error` 字段，并且 MUST NOT 持久化读数。不合法的情形包括：请求体不是合法 JSON；缺少 `temperature_c`；`temperature_c` 不是数字；路径中的设备 ID 不匹配 `^[a-z0-9-]{1,64}$`；请求体超过 1 MiB。

#### Scenario: 缺少温度字段

- **WHEN** `sensor-01` 已注册，请求体为 `{"temp": 23.5}`
- **THEN** 响应状态码为 400，响应体为 `{"error": "temperature_c is required"}`

#### Scenario: 温度不是数字

- **WHEN** `sensor-01` 已注册，请求体为 `{"temperature_c": "hot"}`
- **THEN** 响应状态码为 400，存储中读数条数不变

#### Scenario: 请求体超过 1 MiB

- **WHEN** `sensor-01` 已注册，请求体大小超过 1 MiB
- **THEN** 响应状态码为 400，存储中读数条数不变

### Requirement: ING-04 模拟设备上报脚本

仓内 SHALL 提供 `scripts/simulate.sh [rounds]`，`rounds` 缺省为 10。它直接调用服务的 HTTP 接口模拟设备上报，不经过 MQTT 或真实硬件。

- 目标地址为 `http://${TM_LISTEN_ADDR:-127.0.0.1:8080}`。
- 先注册 `sim-01`、`sim-02`、`sim-03`；注册返回 409 视为已注册并继续。
- 之后每轮每台设备各上报一条读数。`sim-01`、`sim-02` 的温度在 18.0–35.0 之间，`sim-03` 的温度在 50.5–60.0 之间。
- 任一请求失败或状态码不是预期值时，MUST 打印该请求与响应并以退出码 1 结束。

#### Scenario: 服务运行时模拟上报

- **WHEN** 服务在缺省地址运行，执行 `scripts/simulate.sh 3`
- **THEN** 脚本退出码为 0
- **AND** 存储中 `sim-01`、`sim-02`、`sim-03` 各新增 3 条读数
- **AND** 在缺省阈值 50.0 下，服务日志新增至少 3 条超温告警

#### Scenario: 设备已注册时重复运行

- **WHEN** 已经运行过一次 `scripts/simulate.sh`，再运行一次
- **THEN** 注册步骤得到 409 但脚本继续执行，退出码为 0

#### Scenario: 服务未运行

- **WHEN** 缺省地址上没有服务在监听，执行 `scripts/simulate.sh`
- **THEN** 脚本打印失败的请求，退出码为 1
