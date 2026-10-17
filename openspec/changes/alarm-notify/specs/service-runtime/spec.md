# Spec Delta

## MODIFIED Requirements

### Requirement: RUN-01 配置来源与优先级

服务 SHALL 读取四个配置项：`TM_DATABASE_URL`、`TM_LISTEN_ADDR`、`TM_ALARM_THRESHOLD_C`、`TM_ALARM_WEBHOOK_URL`。每一项的取值来源按优先级依次为：环境变量；当前工作目录下的 `config/local.env`；缺省值。`TM_ALARM_WEBHOOK_URL` 没有缺省值，未配置表示不推送。

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
- **THEN** 服务正常读取配置，监听地址与阈值取缺省值，不推送告警

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

#### Scenario: 推送地址键是已知键

- **WHEN** `config/local.env` 写有合法的 `TM_DATABASE_URL` 和一行 `TM_ALARM_WEBHOOK_URL=http://127.0.0.1:9000/alarm`
- **THEN** 服务不因未知键报错
- **AND** `scripts/db-reset.sh` 与 `scripts/simulate.sh` 读取该文件时也不因未知键报错

### Requirement: RUN-02 缺省监听回环地址

`TM_LISTEN_ADDR` 未配置时，服务 SHALL 监听 `127.0.0.1:8080`。告警阈值未配置时 SHALL 为 50.0。

启动日志 SHALL 写明是否启用了推送（字段 `alarm_push`，取值 `true` 或 `false`），MUST NOT 写出 `TM_ALARM_WEBHOOK_URL` 的值。

#### Scenario: 不配置监听地址

- **WHEN** 只配置了 `TM_DATABASE_URL`，启动服务
- **THEN** 服务监听 `127.0.0.1:8080`，不监听其它网卡地址
- **AND** 启动日志写出监听地址与告警阈值 50

#### Scenario: 启动日志标明推送状态

- **WHEN** 分别在未配置和配置了 `TM_ALARM_WEBHOOK_URL=http://127.0.0.1:9000/hook/tok-s3cret` 两种情况下启动服务
- **THEN** 启动日志的 `alarm_push` 分别为 `false` 和 `true`
- **AND** 启动日志不含 `tok-s3cret`

### Requirement: RUN-03 启动前置条件不满足时立即退出

以下任一情况发生时，服务 SHALL 打印原因并以退出码 1 结束，不开始监听：

- `TM_DATABASE_URL` 未配置；
- `TM_ALARM_THRESHOLD_C` 不是有限数，`NaN`、`Inf`、`-Inf` 都视为非法；[spec-review-amendment]
- `TM_ALARM_WEBHOOK_URL` 已配置，但不是 scheme 为 `http` 或 `https`、带主机名的绝对 URL；报错只指出键名，MUST NOT 回显值；
- `config/local.env` 有不符合 RUN-01 文法的行；[spec-review-amendment]
- 5 秒内连不上数据库；
- 数据库中缺少 `devices` 或 `readings` 表；
- 监听地址无法绑定。

#### Scenario: 缺少数据库连接配置

- **WHEN** 环境变量和 `config/local.env` 都没有 `TM_DATABASE_URL`，启动服务
- **THEN** 进程退出码为 1，stderr 指出缺少 `TM_DATABASE_URL`

#### Scenario: 阈值非法

- **WHEN** `TM_ALARM_THRESHOLD_C=abc`，启动服务
- **THEN** 进程退出码为 1，stderr 指出 `TM_ALARM_THRESHOLD_C` 的值非法

#### Scenario: 阈值为 NaN [spec-review-amendment]

- **WHEN** `TM_ALARM_THRESHOLD_C=NaN`，启动服务
- **THEN** 进程退出码为 1，stderr 指出 `TM_ALARM_THRESHOLD_C` 须为有限数

#### Scenario: 阈值为 Inf [spec-review-amendment]

- **WHEN** `TM_ALARM_THRESHOLD_C=Inf`，启动服务
- **THEN** 进程退出码为 1，stderr 指出 `TM_ALARM_THRESHOLD_C` 须为有限数

#### Scenario: 推送地址非法

- **WHEN** `TM_ALARM_WEBHOOK_URL` 依次为 `ftp://127.0.0.1/tok-s3cret`、`127.0.0.1:9000/tok-s3cret`、`http:///tok-s3cret`，分别启动服务
- **THEN** 每次进程退出码都为 1，stderr 指出 `TM_ALARM_WEBHOOK_URL` 的值非法
- **AND** stderr 不含 `tok-s3cret`

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

服务的日志、HTTP 响应，以及 `scripts/` 下各脚本的输出，MUST NOT 包含 `TM_DATABASE_URL` 的值，也 MUST NOT 包含其中口令的任何片段。[spec-review-amendment] 5xx 响应体 SHALL 固定为 `{"error": "internal error"}`，细节只写服务端日志。

`TM_ALARM_WEBHOOK_URL` 同样视为凭据：上述输出 MUST NOT 包含它的值，也 MUST NOT 包含它的路径和查询参数。

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

#### Scenario: 推送失败的日志不含推送地址

- **WHEN** `TM_ALARM_WEBHOOK_URL` 为 `http://127.0.0.1:<端口>/hook/tok-s3cret?key=k3y-s3cret`
- **AND** 接收端依次处于返回 500、一直不响应、端口无人监听三种状态，每种状态下各上报一条超温读数
- **THEN** 日志中有三条 `alarm push failed`
- **AND** 服务的全部日志输出不含 `tok-s3cret`、`k3y-s3cret`、`/hook/`

### Requirement: RUN-05 仓内只有示例配置

仓库 SHALL 提交 `config/example.env`，其中所有连接信息都是占位值：主机为 `127.0.0.1`，口令为 `CHANGE_ME`，库名和用户名为 `temp_monitor_demo`。`TM_ALARM_WEBHOOK_URL` 在示例中 SHALL 以注释行出现，主机为 `127.0.0.1`，复制示例后默认不推送；`http` 与 `https` 都接受，示例注释 SHALL 提醒地址含 token 时使用 `https`。`config/local.env`、`.pg-ops/`、`/.idea/` MUST 被 `.gitignore` 忽略。

#### Scenario: 本地配置不会被提交

- **WHEN** 开发者创建 `config/local.env` 后执行 `git status --porcelain`
- **THEN** 输出中没有 `config/local.env`

#### Scenario: 示例配置只含占位值

- **WHEN** 检查 `config/example.env`
- **THEN** 其中的连接串主机为 `127.0.0.1`，口令为 `CHANGE_ME`，并有注释说明连接 PgBouncer 的 session 池端口、口令需做 URL 编码

#### Scenario: 示例配置中的推送默认关闭

- **WHEN** 检查 `config/example.env`
- **THEN** 含 `TM_ALARM_WEBHOOK_URL` 的行以 `#` 开头，其中主机为 `127.0.0.1`
- **AND** 注释说明地址中的 token 属于凭据，含 token 时应使用 `https`

### Requirement: RUN-06 收到终止信号时优雅退出

服务收到 SIGINT 或 SIGTERM 时，SHALL 停止接受新连接，最多等待 5 秒让进行中的请求完成，关闭数据库连接后以退出码 0 结束。

- `Shutdown` 超过 5 秒时，服务 SHALL 记一条 WARN，仍关闭连接池，并以退出码 0 结束。[spec-review-amendment]
- main MUST 等 `Shutdown` 返回后才退出。[spec-review-amendment]
- 运行中 `Serve` 返回非预期错误时，服务 SHALL 以退出码 1 结束。[spec-review-amendment]
- 配置了接收端时，进行中的请求结束后，服务 SHALL 在同一个 5 秒预算的剩余时间内继续发送待发推送，不另加等待时间。预算用完时中止在途推送，丢弃剩余推送，并记一条 WARN：消息为 `alarm push dropped at shutdown`，字段 `count` 为丢弃条数（含被中止的那条）。没有丢弃时不记这条 WARN。

#### Scenario: Ctrl-C 退出

- **WHEN** 服务空闲运行中，向进程发送 SIGINT
- **THEN** 进程在 5 秒内以退出码 0 结束

#### Scenario: 退出时推送发不完

- **WHEN** 配置的接收端收到请求后一直不响应，服务已产生 10 条待发推送，向进程发送 SIGINT
- **THEN** 进程在 6 秒内以退出码 0 结束（等待预算仍为 5 秒，多出的 1 秒是关闭连接池与测量的余量）
- **AND** 日志中有一条 WARN `alarm push dropped at shutdown`，`count` 大于 0

#### Scenario: 退出时推送能发完

- **WHEN** 配置的接收端立即返回 200，服务有 3 条待发推送时向进程发送 SIGINT
- **THEN** 接收端收到全部 3 条推送，进程以退出码 0 结束，日志中没有 `alarm push dropped at shutdown`
