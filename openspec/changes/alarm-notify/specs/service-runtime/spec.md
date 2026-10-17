# Spec Delta

## MODIFIED Requirements

### Requirement: RUN-01 配置来源与优先级

服务 SHALL 读取四个配置项：`TM_DATABASE_URL`、`TM_LISTEN_ADDR`、`TM_ALARM_THRESHOLD_C`、`TM_ALARM_WEBHOOK_URL`。`TM_ALARM_WEBHOOK_URL` 没有缺省值，未配置即不发告警通知。每一项的取值来源按优先级依次为：环境变量；当前工作目录下的 `config/local.env`；缺省值。

`config/local.env` 的文法如下，服务与 `scripts/` 下的脚本 MUST 使用同一套文法：[spec-review-amendment]

- 逐行读，去掉行尾 `\r`。空行和首个非空白字符为 `#` 的行忽略。[spec-review-amendment]
- 其余行按第一个 `=` 切分，键和值各自去掉两端空白。[spec-review-amendment]
- 没有 `=` 的行、键不在四个已知键里的行 MUST 报错。报错只给文件名和行号，MUST NOT 回显行内容。[spec-review-amendment]
- 值以 `"` 或 `'` 开头时 MUST 报错，只给行号，并提示去掉引号。[spec-review-amendment]
- 值按字面使用，不做变量展开，不支持行内注释，不支持 `export` 前缀。[spec-review-amendment]
- 文件里的值为空，等同未配置。环境变量为空串，也等同未配置。[spec-review-amendment]
- 脚本逐行按字面解析，MUST NOT `source` 这个文件，文件内容不会被执行。[spec-review-amendment]

#### Scenario: 环境变量优先于本地文件

- **WHEN** `config/local.env` 写有 `TM_LISTEN_ADDR=127.0.0.1:9000`，环境变量设置 `TM_LISTEN_ADDR=127.0.0.1:9100`
- **THEN** 服务监听 `127.0.0.1:9100`

#### Scenario: 本地文件优先于缺省值

- **WHEN** 环境变量没有设置 `TM_ALARM_THRESHOLD_C`，`config/local.env` 写有 `TM_ALARM_THRESHOLD_C=30`
- **THEN** 告警阈值为 30

#### Scenario: 本地文件不存在

- **WHEN** `config/local.env` 不存在，环境变量设置了 `TM_DATABASE_URL`
- **THEN** 服务正常读取配置，监听地址和告警阈值取缺省值，不发告警通知

#### Scenario: 本地文件配置 webhook

- **WHEN** 环境变量没有设置 `TM_ALARM_WEBHOOK_URL`，`config/local.env` 写有 `TM_ALARM_WEBHOOK_URL=http://127.0.0.1:9090/hook`，`scripts/simulate.sh` 与服务读取同一文件
- **THEN** 服务启用告警通知
- **AND** `scripts/simulate.sh` 不因这一行报「未知键」

#### Scenario: 值含 `&` 与 `=` [spec-review-amendment]

- **WHEN** 环境变量未设置 `TM_DATABASE_URL`，`config/local.env` 写有 `TM_DATABASE_URL=postgres://temp_monitor_demo:CHANGE_ME@127.0.0.1:7432/temp_monitor_demo?sslmode=disable&connect_timeout=5`
- **THEN** 服务与 `scripts/db-reset.sh` 读到的值都是 `=` 之后的这一整串，没有被截断或改写

#### Scenario: 空值等同未配置 [spec-review-amendment]

- **WHEN** 环境变量未设置 `TM_LISTEN_ADDR`，`config/local.env` 写有 `TM_LISTEN_ADDR=`，启动服务
- **THEN** 服务监听 `127.0.0.1:8080`

#### Scenario: 畸形行报错不回显内容 [spec-review-amendment]

- **WHEN** `config/local.env` 的第 3 行为 `oops s3cret-xyz`（没有 `=`），启动服务
- **THEN** 进程退出码为 1，stderr 含 `config/local.env` 和行号 3
- **AND** stderr 不含 `s3cret-xyz`

### Requirement: RUN-03 启动前置条件不满足时立即退出

以下任一情况发生时，服务 SHALL 打印原因并以退出码 1 结束，不开始监听：

- `TM_DATABASE_URL` 未配置；
- `TM_ALARM_THRESHOLD_C` 不是有限数，`NaN`、`Inf`、`-Inf` 都视为非法；[spec-review-amendment]
- `config/local.env` 有不符合 RUN-01 文法的行；[spec-review-amendment]
- `TM_ALARM_WEBHOOK_URL` 已配置，但不是带主机名的 `http` 或 `https` 绝对 URL；报错只指出键名，MUST NOT 回显它的值；
- 5 秒内连不上数据库；
- 数据库中缺少 `devices` 或 `readings` 表；
- 监听地址无法绑定。

#### Scenario: 缺少数据库连接配置

- **WHEN** 环境变量和 `config/local.env` 都没有 `TM_DATABASE_URL`，启动服务
- **THEN** 进程退出码为 1，stderr 指出缺少 `TM_DATABASE_URL`

#### Scenario: webhook 地址非法

- **WHEN** `TM_ALARM_WEBHOOK_URL` 依次为 `ftp://127.0.0.1/tok-s3cret`、`/hook/tok-s3cret`、`http:///tok-s3cret`，分别启动服务
- **THEN** 每次进程退出码都为 1，stderr 指出 `TM_ALARM_WEBHOOK_URL` 须为 http 或 https URL
- **AND** stderr 不含 `tok-s3cret`

#### Scenario: 阈值非法

- **WHEN** `TM_ALARM_THRESHOLD_C=abc`，启动服务
- **THEN** 进程退出码为 1，stderr 指出 `TM_ALARM_THRESHOLD_C` 的值非法

#### Scenario: 阈值为 NaN [spec-review-amendment]

- **WHEN** `TM_ALARM_THRESHOLD_C=NaN`，启动服务
- **THEN** 进程退出码为 1，stderr 指出 `TM_ALARM_THRESHOLD_C` 须为有限数

#### Scenario: 阈值为 Inf [spec-review-amendment]

- **WHEN** `TM_ALARM_THRESHOLD_C=Inf`，启动服务
- **THEN** 进程退出码为 1，stderr 指出 `TM_ALARM_THRESHOLD_C` 须为有限数

#### Scenario: 数据库不可达

- **WHEN** `TM_DATABASE_URL` 指向一个没有服务在监听的端口，启动服务
- **THEN** 进程在不超过 10 秒内以退出码 1 结束 [spec-review-amendment]

#### Scenario: 表不存在

- **WHEN** 数据库可连接但没有执行过 `scripts/db-reset.sh`，启动服务
- **THEN** 进程退出码为 1，stderr 提示先运行 `scripts/db-reset.sh`

#### Scenario: 端口被占用 [spec-review-amendment]

- **WHEN** 另一个进程已占用 `TM_LISTEN_ADDR` 的端口，启动服务
- **THEN** 进程退出码为 1，stderr 指出地址无法绑定，并提示端口被占用或改 `TM_LISTEN_ADDR`

### Requirement: RUN-04 连接信息不出现在输出中

服务的日志、HTTP 响应，以及 `scripts/` 下各脚本的输出，MUST NOT 包含 `TM_DATABASE_URL` 的值，也 MUST NOT 包含其中口令的任何片段。[spec-review-amendment] 服务的日志与 stderr 同样 MUST NOT 包含 `TM_ALARM_WEBHOOK_URL` 的值，也 MUST NOT 包含它的路径或查询串，因为接收方常把令牌放在这两处。5xx 响应体 SHALL 固定为 `{"error": "internal error"}`，细节只写服务端日志。

#### Scenario: 连库失败的日志

- **WHEN** `TM_DATABASE_URL` 的口令为 `s3cret-xyz`，数据库不可达，启动服务
- **THEN** stderr 全文不含 `s3cret-xyz`

#### Scenario: 口令含未编码的特殊字符 [spec-review-amendment]

- **WHEN** `TM_DATABASE_URL` 的口令依次为 `p(ss;w`、`p&ss`、`p%zz`、`p@ss#w`，均未做 URL 编码，连接串指向无人监听的端口
- **AND** 每种口令下分别启动服务、执行 `scripts/db-reset.sh`
- **THEN** 服务启动失败的 stderr 不含这些口令，也不含 `ss;w`、`ss#w`、`%zz` 这些片段
- **AND** `scripts/db-reset.sh` 的 stdout 和 stderr 同样不含这些口令和片段
- **AND** 两者对连不上库只输出固定文案，不输出驱动或 psql 的原始错误文本

#### Scenario: 运行中存储出错

- **WHEN** 处理上报时存储返回错误
- **THEN** 响应状态码为 500，响应体为 `{"error": "internal error"}`

#### Scenario: 告警通知发送失败的日志

- **WHEN** `TM_ALARM_WEBHOOK_URL=http://127.0.0.1:9/hook/tok-s3cret?key=k3y-xyz`，该端口无人监听，设备上报一条触发 `alarm` 的读数
- **THEN** 日志中有一条告警通知发送失败的 ERROR
- **AND** stderr 全文不含 `tok-s3cret`、`k3y-xyz`

#### Scenario: 接收方返回错误的日志

- **WHEN** `TM_ALARM_WEBHOOK_URL` 的路径含 `tok-s3cret`，接收方返回 403，响应体含 `tok-s3cret`
- **THEN** 日志中的 ERROR 只含状态码 403
- **AND** stderr 全文不含 `tok-s3cret`

### Requirement: RUN-06 收到终止信号时优雅退出

服务收到 SIGINT 或 SIGTERM 时，SHALL 停止接受新连接，最多等待 5 秒让进行中的请求完成，关闭数据库连接后以退出码 0 结束。

- `Shutdown` 超过 5 秒时，服务 SHALL 记一条 WARN，仍关闭连接池，并以退出码 0 结束。[spec-review-amendment]
- main MUST 等 `Shutdown` 返回后才退出。[spec-review-amendment]
- 运行中 `Serve` 返回非预期错误时，服务 SHALL 以退出码 1 结束。[spec-review-amendment]
- 启用告警通知时，服务 SHALL 在 HTTP 请求处理完之后、同一个 5 秒时限内，继续发送已排队的告警通知。时限到时仍未发出的告警通知 SHALL 丢弃，并写一条 WARN，含丢弃条数。处理告警通知 MUST NOT 让退出超过 5 秒。

#### Scenario: Ctrl-C 退出

- **WHEN** 服务空闲运行中，向进程发送 SIGINT
- **THEN** 进程在 5 秒内以退出码 0 结束

#### Scenario: 退出时接收方卡住

- **WHEN** 启用告警通知，接收方收到请求后不再响应，队列中还有 3 条告警通知未发出，向进程发送 SIGINT
- **THEN** 进程在 5 秒内以退出码 0 结束
- **AND** 日志中有一条 WARN，写明丢弃的告警通知条数

#### Scenario: 退出时队列可发完

- **WHEN** 启用告警通知，接收方立即返回 200，队列中有 2 条告警通知未发出，向进程发送 SIGINT
- **THEN** 接收方收到这 2 条告警通知，进程以退出码 0 结束，日志中没有丢弃告警通知的 WARN
