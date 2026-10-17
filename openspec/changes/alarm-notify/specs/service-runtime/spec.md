# Spec Delta

## MODIFIED Requirements

### Requirement: RUN-01 配置来源与优先级

服务 SHALL 读取四个配置项：`TM_DATABASE_URL`、`TM_LISTEN_ADDR`、`TM_ALARM_THRESHOLD_C`、`TM_NOTIFY_WEBHOOK_URL`。每一项的取值来源按优先级依次为：环境变量；当前工作目录下的 `config/local.env`；缺省值。

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
- **THEN** 服务正常读取配置，其余三项取缺省值，即不发通知

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

#### Scenario: 脚本接受 webhook 配置键

- **WHEN** `config/local.env` 写有 `TM_NOTIFY_WEBHOOK_URL=http://127.0.0.1:9000/hook`，执行 `scripts/simulate.sh` 或 `scripts/db-reset.sh`
- **THEN** 脚本不因未知键报错

### Requirement: RUN-02 缺省监听回环地址

`TM_LISTEN_ADDR` 未配置时，服务 SHALL 监听 `127.0.0.1:8080`。告警阈值未配置时 SHALL 为 50.0。`TM_NOTIFY_WEBHOOK_URL` 未配置时，服务 SHALL 不发通知。启动日志 SHALL 写出通知是否开启（`notify=on` 或 `notify=off`），MUST NOT 写出 webhook URL。`notify=on` 只表示配置合法；启动时服务 MUST NOT 向 webhook 发探测请求，能否连通由第一次通知的结果体现。

#### Scenario: 不配置监听地址

- **WHEN** 只配置了 `TM_DATABASE_URL`，启动服务
- **THEN** 服务监听 `127.0.0.1:8080`，不监听其它网卡地址
- **AND** 启动日志写出监听地址、告警阈值 50 与 `notify=off`

#### Scenario: 配置了 webhook

- **WHEN** 配置了 `TM_DATABASE_URL` 和合法的 `TM_NOTIFY_WEBHOOK_URL`，启动服务
- **THEN** 启动日志写出 `notify=on`，不含 webhook URL
- **AND** 启动过程中 webhook 没有收到任何请求

#### Scenario: webhook 不可达也能启动

- **WHEN** `TM_NOTIFY_WEBHOOK_URL` 合法，但指向一个没有服务在监听的端口，其余前置条件满足，启动服务
- **THEN** 服务正常开始监听，启动日志写出 `notify=on`

### Requirement: RUN-03 启动前置条件不满足时立即退出

以下任一情况发生时，服务 SHALL 打印原因并以退出码 1 结束，不开始监听：

- `TM_DATABASE_URL` 未配置；
- `TM_ALARM_THRESHOLD_C` 不是有限数，`NaN`、`Inf`、`-Inf` 都视为非法；[spec-review-amendment]
- `TM_NOTIFY_WEBHOOK_URL` 已配置，但不是 scheme 为 `http` 或 `https`、且含 host 的绝对 URL；报错只指出键名，MUST NOT 回显值；
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

#### Scenario: webhook URL 非法

- **WHEN** `TM_NOTIFY_WEBHOOK_URL=ftp://hook-s3cret` 或 `TM_NOTIFY_WEBHOOK_URL=hook-s3cret`，启动服务
- **THEN** 进程退出码为 1，stderr 指出 `TM_NOTIFY_WEBHOOK_URL` 非法
- **AND** stderr 不含 `hook-s3cret`

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

服务的日志、HTTP 响应，以及 `scripts/` 下各脚本的输出，MUST NOT 包含 `TM_DATABASE_URL` 的值，也 MUST NOT 包含其中口令的任何片段。[spec-review-amendment] 服务与脚本 MUST NOT 把 `TM_NOTIFY_WEBHOOK_URL` 的值或从中取出的任何片段写入这些输出。5xx 响应体 SHALL 固定为 `{"error": "internal error"}`，细节只写服务端日志。

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

#### Scenario: 通知失败的日志不含 webhook URL

- **WHEN** `TM_NOTIFY_WEBHOOK_URL` 为 `http://127.0.0.1:<port>/hook/tok-s3cret?key=k-s3cret`，webhook 依次表现为返回 500、不可连接、超时，每种情况下上报一条触发通知的超温读数
- **THEN** 服务 stderr 全文不含 `tok-s3cret`、`k-s3cret`，也不含该 URL

### Requirement: RUN-05 仓内只有示例配置

仓库 SHALL 提交 `config/example.env`，其中所有连接信息都是占位值：主机为 `127.0.0.1`，口令为 `CHANGE_ME`，库名和用户名为 `temp_monitor_demo`。`TM_NOTIFY_WEBHOOK_URL` 在示例配置中 SHALL 为空值（即不通知），并在注释里给出主机为 `127.0.0.1`、token 为 `CHANGE_ME` 的写法示例。`config/local.env`、`.pg-ops/`、`/.idea/` MUST 被 `.gitignore` 忽略。

#### Scenario: 本地配置不会被提交

- **WHEN** 开发者创建 `config/local.env` 后执行 `git status --porcelain`
- **THEN** 输出中没有 `config/local.env`

#### Scenario: 示例配置只含占位值

- **WHEN** 检查 `config/example.env`
- **THEN** 其中的连接串主机为 `127.0.0.1`，口令为 `CHANGE_ME`，并有注释说明连接 PgBouncer 的 session 池端口、口令需做 URL 编码
- **AND** `TM_NOTIFY_WEBHOOK_URL=` 的值为空，注释中的示例 URL 主机为 `127.0.0.1`、token 为 `CHANGE_ME`，并说明 URL 视同秘密
