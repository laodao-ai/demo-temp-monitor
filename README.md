# demo-temp-monitor

一个演示项目：用 Go 写的设备温度监控服务。它接收设备上报的温度，存进 PostgreSQL，温度超过阈值时写一条告警日志。

仓库地址：`github.com/laodao-ai/demo-temp-monitor`。

## 范围

项目只做四块：

1. 设备注册与注销（`POST /devices`、`DELETE /devices/{id}`）。
2. 温度上报（`POST /devices/{id}/readings`）。
3. 超温告警：读数严格大于阈值时，服务写一条 WARN 日志 `over-temperature alarm`。
4. 配置与运行：三个配置项、`127.0.0.1` 默认监听、启动前置检查、优雅退出。

不做鉴权、限流、查询接口、指标和健康检查。

## 无鉴权，只应监听 127.0.0.1

服务没有任何鉴权。默认监听 `127.0.0.1:8080`，只应保持在回环地址。把 `TM_LISTEN_ADDR` 改成其它地址，风险由使用者自己承担。

## 前置工具

- Go 1.27
- psql（PostgreSQL 客户端）
- jq
- curl
- bash
- C 编译器（`go test -race` 需要）

测过的版本：Go 1.27；PostgreSQL 18 待确认。

## 配置

三个配置项，取值来源的优先级是：环境变量，其次 `config/local.env`，最后缺省值。

| 键 | 缺省 | 说明 |
|---|---|---|
| `TM_DATABASE_URL` | 无，必填 | pgx 连接串 |
| `TM_LISTEN_ADDR` | `127.0.0.1:8080` | 服务监听地址 |
| `TM_ALARM_THRESHOLD_C` | `50.0` | 告警阈值（°C），读数严格大于它才告警 |

配置方法：

1. 复制 `config/example.env` 为 `config/local.env`。
2. 填入连接串。`config/local.env` 已被 `.gitignore` 忽略，不会被提交。
3. 连接串里的口令含特殊字符时，做 URL 编码（例如 `@` 写成 `%40`，`#` 写成 `%23`）。
4. 连数据库连接池的 session 池端口，不要连 transaction 池端口。服务使用预处理语句，transaction 池不支持。

`config/local.env` 每行一个 `KEY=VALUE`，值不加引号，不做变量展开，不支持行内注释。服务和 `scripts/` 下的脚本逐行按字面读取它，不执行文件内容。

## 启动顺序

1. 建库：在 PostgreSQL 里建好库 `temp_monitor_demo` 和对应的用户。
2. 隧道：数据库在远端时，先起 SSH 隧道，把本机端口转到数据库的 session 池端口。
3. 配置：按上一节写好 `config/local.env`。
4. 重建表和种子数据：`scripts/db-reset.sh`。
5. 启动服务：在仓根执行 `go run ./cmd/temp-monitor`。服务按当前目录找 `config/local.env`，所以要从仓根启动。
6. 模拟设备上报：`scripts/simulate.sh [rounds]`。`rounds` 缺省为 10。

`scripts/db-reset.sh` 只对库 `temp_monitor_demo` 生效，其它库名一律拒绝。它在一个事务里删表、建表、写种子数据，失败时整体回滚。

退出码：0 成功；1 缺 `TM_DATABASE_URL` 或 `config/local.env` 有语法错误；2 用法错误；3 库名不符；4 psql 失败。

## 测试

统一入口是 `scripts/test.sh [unit|integration|all]`，缺省为 `all`。

- 第一个要跑的命令是 `scripts/test.sh unit`。它只跑单元层，用内存存储，不需要数据库。
- `scripts/test.sh integration` 跑连真库层，需要先配好 `config/local.env` 并执行过 `scripts/db-reset.sh`。
- 没配数据库时直接跑 `scripts/test.sh`（即 `all`）会失败：连真库层连不上库时失败，不会跳过。
- 汇总行是 stdout 的最后一行：`N passed, M failed, K skipped (T total)`。
- 用法错误（参数不是 `unit` / `integration` / `all`）和缺工具（`go`、`jq`）时没有汇总行。
