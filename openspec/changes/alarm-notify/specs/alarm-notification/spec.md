## Purpose

规定超温读数如何推送到钉钉群自定义机器人以通知值班的人：推送条件、消息内容、按设备冷却、加签、成功判定、失败处理，以及推送与上报的隔离。

## ADDED Requirements

### Requirement: NTF-01 超温读数推送到钉钉群

`TM_DINGTALK_WEBHOOK` 已配置，读数成功入库且温度严格大于告警阈值（即 ALM-01 写出超温告警日志），并且该设备不在冷却期（NTF-03）时，系统 SHALL 向 `TM_DINGTALK_WEBHOOK` 发出一次 HTTP POST，`Content-Type` 为 `application/json`。请求体 MUST 满足：

- `msgtype` 为 `text`；
- `text.content` 含固定文字 `超温告警`，以及设备 ID、读数温度、告警阈值、读数入库时间（RFC 3339）；
- `at.isAtAll` 为 `true`。

超温告警日志（ALM-01~ALM-04）的行为 MUST NOT 因推送而改变。

#### Scenario: 推送内容

- **WHEN** 阈值为 50.0，已配置 webhook，已注册设备 `sensor-01` 上报 `{"temperature_c": 55.0}`
- **THEN** webhook 收到恰好一次 POST，请求体解析为 JSON 后 `msgtype` 为 `text`，`at.isAtAll` 为 `true`
- **AND** `text.content` 含 `超温告警`、`设备：sensor-01`、`温度：55°C`、`阈值 50°C`，以及该读数入库时间的 UTC RFC 3339 形式（与 `RecordedAt.UTC().Format(time.RFC3339)` 相同）[spec-review-amendment]
- **AND** 日志中新增一条 `over-temperature alarm` WARN，与未配置 webhook 时相同

#### Scenario: 等于阈值不推送

- **WHEN** 阈值为 50.0，已配置 webhook，已注册设备上报 `{"temperature_c": 50.0}`
- **THEN** webhook 没有收到请求

### Requirement: NTF-02 推送不影响上报

推送 SHALL 在上报请求之外异步进行：上报接口的状态码、响应体与耗时 MUST NOT 取决于推送的结果或耗时。单次推送 SHALL 在 5 秒内结束，超时视为失败（NTF-05）。推送过程中出现 panic 时，服务 MUST NOT 退出，SHALL 写一条 `alarm notification failed` ERROR [spec-review-amendment]。

#### Scenario: 钉钉挂起

- **WHEN** 已配置 webhook，webhook 收到请求后不响应，已注册设备上报一条超温读数
- **THEN** 上报响应状态码为 201，响应在 1 秒内返回
- **AND** 发出请求后 5 秒内（允许 1 秒调度余量），日志中新增一条 `alarm notification failed` ERROR

#### Scenario: 钉钉返回错误

- **WHEN** 已配置 webhook，webhook 对每个请求回 HTTP 500，已注册设备上报一条超温读数
- **THEN** 上报响应状态码为 201，响应体的字段集合（`id`、`device_id`、`temperature_c`、`recorded_at`）与未配置 webhook 时相同 [spec-review-amendment]

#### Scenario: 推送中 panic 不影响服务 [spec-review-amendment]

- **WHEN** 已配置 webhook，发送方在推送时 panic，已注册设备上报一条超温读数，随后再上报一条普通读数
- **THEN** 两次上报都返回 201，服务进程仍在运行
- **AND** 日志中新增一条 `alarm notification failed` ERROR

### Requirement: NTF-03 按设备冷却

系统 SHALL 为每个设备单独记录最近一次发起推送的时刻。同一设备距上次发起推送不足冷却时长时，系统 MUST NOT 推送；达到或超过冷却时长后的下一条超温读数 SHALL 再推送一次。

- 冷却时长取 `TM_NOTIFY_COOLDOWN`，格式为 Go duration（如 `10m`、`90s`），未配置时为 10 分钟。
- 冷却从**发起**推送时开始计时，推送失败也计入。
- 同一设备的多条超温读数并发到达时，系统 MUST 只为其中一条发起推送。
- 冷却记录只在进程内存中，重启后清空。
- 冷却只影响推送；每条超温读数的 WARN 日志照写（ALM-04）。冷却不属于 ALM-04 所禁止的「告警去重」：ALM-04 约束的是 WARN 告警日志，本能力约束的是钉钉推送 [spec-review-amendment]。

#### Scenario: 冷却期内不重复推送

- **WHEN** 冷却为 10 分钟，已注册设备 `sensor-01` 在 t0 上报 55.0，在 t0+5 分钟上报 56.0
- **THEN** webhook 只收到一次请求，`text.content` 含 `55`
- **AND** 日志中新增两条 `over-temperature alarm` WARN

#### Scenario: 冷却期过后再推送

- **WHEN** 冷却为 10 分钟，已注册设备 `sensor-01` 在 t0 上报 55.0，在 t0+10 分钟上报 57.0
- **THEN** webhook 共收到两次请求，第二次的 `text.content` 含 `57`

#### Scenario: 不同设备各自冷却

- **WHEN** 冷却为 10 分钟，已注册设备 `sensor-01` 与 `sensor-02` 在 1 分钟内各上报一条超温读数
- **THEN** webhook 收到两次请求，分别含 `sensor-01` 与 `sensor-02`

#### Scenario: 并发超温只推一次

- **WHEN** 冷却为 10 分钟，同一已注册设备的 20 条超温读数并发上报
- **THEN** 20 条上报都返回 201，webhook 只收到一次请求
- **AND** 日志中新增 20 条 `over-temperature alarm` WARN

#### Scenario: 推送失败也开始冷却

- **WHEN** 冷却为 10 分钟，webhook 对每个请求回 HTTP 500，已注册设备 `sensor-01` 在 t0 与 t0+1 分钟各上报一条超温读数
- **THEN** webhook 只收到一次请求

#### Scenario: 冷却可配置

- **WHEN** `TM_NOTIFY_COOLDOWN=90s`，已注册设备在 t0 与 t0+90 秒各上报一条超温读数
- **THEN** webhook 收到两次请求

### Requirement: NTF-04 加签

`TM_DINGTALK_SECRET` 已配置时，系统 SHALL 在每次推送的请求 URL 上保留 webhook 原有查询参数，并追加：

- `timestamp`：发起推送时刻的 Unix 毫秒时间戳；
- `sign`：以 `timestamp` + 换行符 + secret 为原文、secret 为密钥计算 HMAC-SHA256，再做标准 Base64 编码，最后按 URL 查询参数编码。

`TM_DINGTALK_SECRET` 未配置时，请求 URL MUST 与 `TM_DINGTALK_WEBHOOK` 完全相同，不含 `timestamp` 与 `sign`。

#### Scenario: 加签结果

- **WHEN** secret 为 `SECtest0000000000000000000000000000000000000000000000000000000000`，发起推送时刻为 Unix 毫秒 `1700000000000`，webhook 为 `http://127.0.0.1:<port>/robot/send?access_token=tok`
- **THEN** 请求的查询参数 `access_token` 为 `tok`，`timestamp` 为 `1700000000000`，`sign` 解码后为 `szPcgtoCabDjIT69N8i2Omun8ktW8akfg42aQqadSCU=`

#### Scenario: 不加签

- **WHEN** 未配置 secret，webhook 为 `http://127.0.0.1:<port>/robot/send?access_token=tok`
- **THEN** 请求的查询参数只有 `access_token=tok`

### Requirement: NTF-05 成功判定与失败日志

系统 SHALL 只在 HTTP 状态码为 2xx、且响应体为 JSON 并且其中**存在**整数字段 `errcode` 且值为 0 时判定推送成功（缺少 `errcode` 字段不算 0）[spec-review-amendment]，此时写一条 INFO 日志，消息为 `alarm notification sent`，字段含 `device_id`。其余情况，包括网络错误、5 秒超时、非 2xx（含 3xx，系统不跟随重定向）、响应体不是 JSON、缺少 `errcode`、`errcode` 不为 0 [spec-review-amendment]，系统 SHALL 写一条 ERROR 日志，消息为 `alarm notification failed`，字段含 `device_id` 与 `error`。每条放行的超温读数 MUST 只发一次请求，失败不重试。

#### Scenario: 推送成功

- **WHEN** webhook 回 HTTP 200，响应体 `{"errcode":0,"errmsg":"ok"}`，已注册设备 `sensor-01` 上报一条超温读数
- **THEN** 日志中新增一条 `alarm notification sent` INFO，`device_id=sensor-01`

#### Scenario: HTTP 200 但 errcode 非 0

- **WHEN** webhook 回 HTTP 200，响应体 `{"errcode":1,"errmsg":"x"}`，已注册设备上报一条超温读数
- **THEN** 日志中新增一条 `alarm notification failed` ERROR，`error` 含 errcode `1`
- **AND** 没有 `alarm notification sent`，webhook 只收到一次请求

#### Scenario: 非 2xx

- **WHEN** webhook 回 HTTP 502，已注册设备上报一条超温读数
- **THEN** 日志中新增一条 `alarm notification failed` ERROR，`error` 含 `502`，webhook 只收到一次请求

#### Scenario: HTTP 200 但缺少 errcode [spec-review-amendment]

- **WHEN** webhook 回 HTTP 200，响应体为 `{}`，已注册设备上报一条超温读数
- **THEN** 日志中新增一条 `alarm notification failed` ERROR，没有 `alarm notification sent`

#### Scenario: 重定向不跟随 [spec-review-amendment]

- **WHEN** webhook 回 HTTP 302，`Location` 指向另一个 httptest 服务器，已注册设备上报一条超温读数
- **THEN** 日志中新增一条 `alarm notification failed` ERROR，`error` 含 `302`
- **AND** `Location` 指向的服务器没有收到请求

#### Scenario: 数据库连接串不含口令时失败原因仍可见 [spec-review-amendment]

- **WHEN** `TM_DATABASE_URL` 不含口令（如 `postgres://temp_monitor_demo@127.0.0.1:7432/temp_monitor_demo`），webhook 回 HTTP 502，已注册设备上报一条超温读数
- **THEN** 日志中 `alarm notification failed` ERROR 的 `error` 含 `502`，不是 `error details withheld`

#### Scenario: 响应体不是 JSON

- **WHEN** webhook 回 HTTP 200，响应体为 `<html>gateway</html>`，已注册设备上报一条超温读数
- **THEN** 日志中新增一条 `alarm notification failed` ERROR

#### Scenario: webhook 不可达

- **WHEN** webhook 指向本机一个没有服务监听的端口，已注册设备上报一条超温读数
- **THEN** 日志中新增一条 `alarm notification failed` ERROR

### Requirement: NTF-06 未配置 webhook 不推送

`TM_DINGTALK_WEBHOOK` 未配置时，系统 SHALL 不发出任何推送请求，服务照常启动并处理上报，启动时写一条 INFO 日志 `notify disabled`。已配置时，启动日志 SHALL 写一条 INFO `notify enabled`，字段含 `cooldown` 与 `signed`（是否配置了 secret），MUST NOT 含 webhook 或 secret 的值。

#### Scenario: 未配置

- **WHEN** 只配置了 `TM_DATABASE_URL`，启动服务，已注册设备上报一条超温读数
- **THEN** 启动日志含 `notify disabled`，上报返回 201，日志中有 `over-temperature alarm` WARN，没有 `alarm notification` 开头的日志

#### Scenario: 已配置

- **WHEN** 配置了 webhook 与 secret，`TM_NOTIFY_COOLDOWN=5m`，启动服务
- **THEN** 启动日志含 `notify enabled`，`cooldown=5m0s`，`signed=true`

### Requirement: NTF-07 上报失败不推送

上报因设备未注册、请求不合法或存储出错而失败时，系统 SHALL NOT 推送，即使请求里的温度高于阈值，也 MUST NOT 为该设备开始冷却。

#### Scenario: 未注册设备上报高温

- **WHEN** 已配置 webhook，未注册设备 `ghost-01` 上报 `{"temperature_c": 80}`
- **THEN** 响应状态码为 404，webhook 没有收到请求

#### Scenario: 存储出错后恢复

- **WHEN** 已配置 webhook，冷却为 10 分钟，存储在写入读数时返回错误，设备 `sensor-01` 上报温度 80；随后存储恢复，`sensor-01` 再上报温度 81
- **THEN** 第一次响应状态码为 500，webhook 没有收到请求
- **AND** 第二次响应状态码为 201，webhook 收到一次请求，`text.content` 含 `81`
