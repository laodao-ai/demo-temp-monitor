## Purpose

规定读数超过全局告警阈值时系统如何告警：只写一条结构化 WARN 日志，不做去重、通知或按设备阈值。

## ADDED Requirements

### Requirement: ALM-01 超阈值时写告警日志

读数成功入库，并且温度严格大于告警阈值时，系统 SHALL 写一条级别为 WARN 的日志。日志消息为 `over-temperature alarm`，字段含 `device_id`、`temperature_c`、`threshold_c`。告警 MUST NOT 改变上报接口的响应，上报仍返回 201。

#### Scenario: 读数高于阈值

- **WHEN** 阈值为 50.0，已注册设备 `sensor-01` 上报 `{"temperature_c": 55.0}`
- **THEN** 上报响应状态码为 201
- **AND** 日志中新增恰好一条 WARN，消息为 `over-temperature alarm`，`device_id=sensor-01`，`temperature_c=55`，`threshold_c=50`
- **AND** 断言按日志的结构化字段比较：msg、`device_id` 相等，数值字段按数值相等，不按整行文本比较 [spec-review-amendment]

#### Scenario: 阈值可配置

- **WHEN** `TM_ALARM_THRESHOLD_C=30`，已注册设备上报 `{"temperature_c": 31}`
- **THEN** 日志中新增一条超温告警，`threshold_c=30`

### Requirement: ALM-02 不超过阈值不告警

温度小于或等于阈值时，系统 SHALL NOT 写超温告警日志。

#### Scenario: 读数等于阈值

- **WHEN** 阈值为 50.0，已注册设备上报 `{"temperature_c": 50.0}`
- **THEN** 上报响应状态码为 201，日志中没有新增超温告警

#### Scenario: 读数低于阈值

- **WHEN** 阈值为 50.0，已注册设备上报 `{"temperature_c": 23.5}`
- **THEN** 日志中没有新增超温告警

### Requirement: ALM-03 上报失败不告警

上报因设备未注册、请求不合法或存储出错而失败时，系统 SHALL NOT 写超温告警日志，即使请求里的温度高于阈值。

说明：存储调用超过 5 秒时结果未知，语句可能已提交。已提交时，读数已入库，响应仍为 500，且没有告警日志。[spec-review-amendment]

#### Scenario: 未注册设备上报高温

- **WHEN** 阈值为 50.0，未注册设备 `ghost-01` 上报 `{"temperature_c": 80}`
- **THEN** 响应状态码为 404，日志中没有新增超温告警

#### Scenario: 存储出错

- **WHEN** 阈值为 50.0，存储在写入读数时返回错误，请求温度为 80
- **THEN** 响应状态码为 500，日志中没有新增超温告警

### Requirement: ALM-04 每条超温读数各告警一次

系统 SHALL 对每一条超温读数各写一条告警。系统 MUST NOT 做去重、合并或滞回。

#### Scenario: 连续两条超温读数

- **WHEN** 阈值为 50.0，同一设备连续上报 55.0 和 56.0
- **THEN** 日志中新增两条超温告警，`temperature_c` 分别为 55 和 56
