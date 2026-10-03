## Purpose

规定服务从哪里读配置、各配置项的缺省值、监听地址、启动时哪些情况直接失败，以及连接信息不进仓库、不进输出的约束。

## ADDED Requirements

### Requirement: RUN-01 配置来源与优先级

服务 SHALL 读取三个配置项：`TM_DATABASE_URL`、`TM_LISTEN_ADDR`、`TM_ALARM_THRESHOLD_C`。每一项的取值来源按优先级依次为：环境变量；当前工作目录下的 `config/local.env`；缺省值。`config/local.env` 每行为 `KEY=VALUE`，`#` 开头的行和空行忽略。

#### Scenario: 环境变量优先于本地文件

- **WHEN** `config/local.env` 写有 `TM_LISTEN_ADDR=127.0.0.1:9000`，环境变量设置 `TM_LISTEN_ADDR=127.0.0.1:9100`
- **THEN** 服务监听 `127.0.0.1:9100`

#### Scenario: 本地文件优先于缺省值

- **WHEN** 环境变量没有设置 `TM_ALARM_THRESHOLD_C`，`config/local.env` 写有 `TM_ALARM_THRESHOLD_C=30`
- **THEN** 告警阈值为 30

#### Scenario: 本地文件不存在

- **WHEN** `config/local.env` 不存在，环境变量设置了 `TM_DATABASE_URL`
- **THEN** 服务正常读取配置，其余两项取缺省值

### Requirement: RUN-02 缺省监听回环地址

`TM_LISTEN_ADDR` 未配置时，服务 SHALL 监听 `127.0.0.1:8080`。告警阈值未配置时 SHALL 为 50.0。

#### Scenario: 不配置监听地址

- **WHEN** 只配置了 `TM_DATABASE_URL`，启动服务
- **THEN** 服务监听 `127.0.0.1:8080`，不监听其它网卡地址
- **AND** 启动日志写出监听地址与告警阈值 50

### Requirement: RUN-03 启动前置条件不满足时立即退出

以下任一情况发生时，服务 SHALL 打印原因并以退出码 1 结束，不开始监听：

- `TM_DATABASE_URL` 未配置；
- `TM_ALARM_THRESHOLD_C` 不是合法数字；
- 5 秒内连不上数据库；
- 数据库中缺少 `devices` 或 `readings` 表；
- 监听地址无法绑定。

#### Scenario: 缺少数据库连接配置

- **WHEN** 环境变量和 `config/local.env` 都没有 `TM_DATABASE_URL`，启动服务
- **THEN** 进程退出码为 1，stderr 指出缺少 `TM_DATABASE_URL`

#### Scenario: 阈值非法

- **WHEN** `TM_ALARM_THRESHOLD_C=abc`，启动服务
- **THEN** 进程退出码为 1，stderr 指出 `TM_ALARM_THRESHOLD_C` 的值非法

#### Scenario: 数据库不可达

- **WHEN** `TM_DATABASE_URL` 指向一个没有服务在监听的端口，启动服务
- **THEN** 进程在约 5 秒内以退出码 1 结束

#### Scenario: 表不存在

- **WHEN** 数据库可连接但没有执行过 `scripts/db-reset.sh`，启动服务
- **THEN** 进程退出码为 1，stderr 提示先运行 `scripts/db-reset.sh`

### Requirement: RUN-04 连接信息不出现在输出中

服务的日志、HTTP 响应，以及 `scripts/` 下各脚本的输出，MUST NOT 包含 `TM_DATABASE_URL` 的值或其中的口令。5xx 响应体 SHALL 固定为 `{"error": "internal error"}`，细节只写服务端日志。

#### Scenario: 连库失败的日志

- **WHEN** `TM_DATABASE_URL` 的口令为 `s3cret-xyz`，数据库不可达，启动服务
- **THEN** stderr 全文不含 `s3cret-xyz`

#### Scenario: 运行中存储出错

- **WHEN** 处理上报时存储返回错误
- **THEN** 响应状态码为 500，响应体为 `{"error": "internal error"}`

### Requirement: RUN-05 仓内只有示例配置

仓库 SHALL 提交 `config/example.env`，其中所有连接信息都是占位值：主机为 `127.0.0.1`，口令为 `CHANGE_ME`，库名和用户名为 `temp_monitor_demo`。`config/local.env`、`.pg-ops/`、`/.idea/` MUST 被 `.gitignore` 忽略。

#### Scenario: 本地配置不会被提交

- **WHEN** 开发者创建 `config/local.env` 后执行 `git status --porcelain`
- **THEN** 输出中没有 `config/local.env`

#### Scenario: 示例配置只含占位值

- **WHEN** 检查 `config/example.env`
- **THEN** 其中的连接串主机为 `127.0.0.1`，口令为 `CHANGE_ME`，并有注释说明连接 PgBouncer 的 session 池端口、口令需做 URL 编码

### Requirement: RUN-06 收到终止信号时优雅退出

服务收到 SIGINT 或 SIGTERM 时，SHALL 停止接受新连接，最多等待 5 秒让进行中的请求完成，关闭数据库连接后以退出码 0 结束。

#### Scenario: Ctrl-C 退出

- **WHEN** 服务空闲运行中，向进程发送 SIGINT
- **THEN** 进程在 5 秒内以退出码 0 结束
