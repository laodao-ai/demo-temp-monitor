## MODIFIED Requirements

### Requirement: RUN-01 配置来源与优先级

服务 SHALL 读取六个配置项：`TM_DATABASE_URL`、`TM_LISTEN_ADDR`、`TM_ALARM_THRESHOLD_C`、`TM_DINGTALK_WEBHOOK`、`TM_DINGTALK_SECRET`、`TM_NOTIFY_COOLDOWN`。每一项的取值来源按优先级依次为：环境变量；当前工作目录下的 `config/local.env`；缺省值。`TM_DINGTALK_WEBHOOK` 与 `TM_DINGTALK_SECRET` 没有缺省值，未配置即为空；`TM_NOTIFY_COOLDOWN` 的缺省值为 10 分钟。

`config/local.env` 的文法如下，服务与 `scripts/` 下的脚本 MUST 使用同一套文法：[spec-review-amendment]

- 逐行读，去掉行尾 `\r`。空行和首个非空白字符为 `#` 的行忽略。[spec-review-amendment]
- 其余行按第一个 `=` 切分，键和值各自去掉两端空白。[spec-review-amendment]
- 没有 `=` 的行、键不在六个已知键里的行 MUST 报错。报错只给文件名和行号，MUST NOT 回显行内容。报错的提示 MUST 列出全部六个已知键，服务与脚本列出的键 MUST 一致。[spec-review-amendment]
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
- **THEN** 服务正常读取配置，其余各项取缺省值或为空，推送未启用

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

#### Scenario: 脚本接受推送配置键

- **WHEN** `config/local.env` 除 `TM_DATABASE_URL` 外还写有 `TM_DINGTALK_WEBHOOK=https://example.invalid/robot/send?access_token=tok`、`TM_DINGTALK_SECRET=SECabc`、`TM_NOTIFY_COOLDOWN=10m`，执行 `scripts/db-reset.sh`
- **THEN** 脚本不报 unknown key，读到的 `TM_DATABASE_URL` 与只写这一行时相同

#### Scenario: 未知键的提示列出六个键

- **WHEN** `config/local.env` 的第 2 行为 `FOO=bar`，分别启动服务与执行 `scripts/db-reset.sh`
- **THEN** 两者的 stderr 都含 `line 2: unknown key` 与 `allowed keys: TM_DATABASE_URL, TM_LISTEN_ADDR, TM_ALARM_THRESHOLD_C, TM_DINGTALK_WEBHOOK, TM_DINGTALK_SECRET, TM_NOTIFY_COOLDOWN`

### Requirement: RUN-03 启动前置条件不满足时立即退出

以下任一情况发生时，服务 SHALL 打印原因并以退出码 1 结束，不开始监听：

- `TM_DATABASE_URL` 未配置；
- `TM_ALARM_THRESHOLD_C` 不是有限数，`NaN`、`Inf`、`-Inf` 都视为非法；[spec-review-amendment]
- `TM_DINGTALK_WEBHOOK` 已配置但不是带主机的绝对 URL，或 scheme 不是 `http`、`https`，或查询串无法解析 [spec-review-amendment]；
- `TM_NOTIFY_COOLDOWN` 已配置但不是合法的 Go duration，或不大于 0；
- `TM_DINGTALK_SECRET` 已配置而 `TM_DINGTALK_WEBHOOK` 未配置；
- `config/local.env` 有不符合 RUN-01 文法的行；[spec-review-amendment]
- 5 秒内连不上数据库；
- 数据库中缺少 `devices` 或 `readings` 表；
- 监听地址无法绑定。

推送相关的三种配置错误，报错 MUST 只给键名与修正提示，MUST NOT 回显配置值。

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

#### Scenario: webhook 非法

- **WHEN** `TM_DATABASE_URL` 已配置 [spec-review-amendment]，分别设置 `TM_DINGTALK_WEBHOOK` 为 `not a url tok-123`、`ftp://example.invalid/robot/send?access_token=tok-123`、`/robot/send?access_token=tok-123`、`https://example.invalid/robot/send?access_token=tok-123;x` [spec-review-amendment]，启动服务
- **THEN** 每次进程退出码都为 1，stderr 为一行 `TM_DINGTALK_WEBHOOK is invalid; must be an absolute http or https URL with a valid query string` [spec-review-amendment]
- **AND** stderr 不含 `tok-123`

#### Scenario: http 的 webhook 可用于本机假服务器

- **WHEN** `TM_DINGTALK_WEBHOOK=http://127.0.0.1:9999/robot/send?access_token=tok`，其余配置合法，启动服务
- **THEN** 配置校验通过，服务开始监听

#### Scenario: 冷却非法

- **WHEN** `TM_DATABASE_URL` 已配置 [spec-review-amendment]，分别设置 `TM_NOTIFY_COOLDOWN` 为 `ten`、`0`、`-1m`，启动服务
- **THEN** 每次进程退出码都为 1，stderr 为一行 `TM_NOTIFY_COOLDOWN is invalid; must be a positive duration, such as 10m`

#### Scenario: 配了 secret 未配 webhook

- **WHEN** `TM_DATABASE_URL` 已配置，设置 `TM_DINGTALK_SECRET=SECabc`，未设置 `TM_DINGTALK_WEBHOOK`，启动服务 [spec-review-amendment]
- **THEN** 进程退出码为 1，stderr 为一行 `TM_DINGTALK_SECRET is set but TM_DINGTALK_WEBHOOK is not; set TM_DINGTALK_WEBHOOK or remove TM_DINGTALK_SECRET`
- **AND** stderr 不含 `SECabc`

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

服务的日志与 stderr MUST NOT 包含 `TM_DINGTALK_WEBHOOK` 的值、其中 `access_token` 查询参数的值，也 MUST NOT 包含 `TM_DINGTALK_SECRET` 的值。这些值出现在日志消息、字符串字段、分组字段或错误文本中时，SHALL 被替换为 `***`。

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

#### Scenario: 推送失败的日志不含推送密钥

- **WHEN** `TM_DINGTALK_WEBHOOK=http://127.0.0.1:<无人监听的端口>/robot/send?access_token=tok-5f2a9c`，`TM_DINGTALK_SECRET=SECsecret-7d1e`，已注册设备上报一条超温读数
- **THEN** 日志中有一条 `alarm notification failed` ERROR
- **AND** 服务 stderr 全文不含 `tok-5f2a9c`，不含 `SECsecret-7d1e`，不含 webhook 全串

#### Scenario: 日志字段携带推送密钥时被替换

- **WHEN** 日志记录的消息、字符串字段、分组字段或 error 值中含 `TM_DINGTALK_WEBHOOK` 全串、其 `access_token` 值或 `TM_DINGTALK_SECRET` 值
- **THEN** 输出中这些值都被替换为 `***`，其余文本不变

### Requirement: RUN-05 仓内只有示例配置

仓库 SHALL 提交 `config/example.env`，其中所有连接信息都是占位值：主机为 `127.0.0.1`，口令为 `CHANGE_ME`，库名和用户名为 `temp_monitor_demo`。`TM_DINGTALK_WEBHOOK` 与 `TM_DINGTALK_SECRET` MUST 留空，并有注释说明分别填钉钉群自定义机器人的 webhook 地址与加签密钥；`TM_NOTIFY_COOLDOWN` 写缺省值 `10m`。`config/local.env`、`.pg-ops/`、`/.idea/` MUST 被 `.gitignore` 忽略。

#### Scenario: 本地配置不会被提交

- **WHEN** 开发者创建 `config/local.env` 后执行 `git status --porcelain`
- **THEN** 输出中没有 `config/local.env`

#### Scenario: 示例配置只含占位值

- **WHEN** 检查 `config/example.env`
- **THEN** 其中的连接串主机为 `127.0.0.1`，口令为 `CHANGE_ME`，并有注释说明连接 PgBouncer 的 session 池端口、口令需做 URL 编码

#### Scenario: 示例配置的推送键留空

- **WHEN** 检查 `config/example.env`
- **THEN** 其中有 `TM_DINGTALK_WEBHOOK=` 与 `TM_DINGTALK_SECRET=` 两行，等号后为空，各有一行注释说明用途
- **AND** 有 `TM_NOTIFY_COOLDOWN=10m`
- **AND** 复制为 `config/local.env` 并填好连接串后启动服务，启动日志含 `notify disabled`
