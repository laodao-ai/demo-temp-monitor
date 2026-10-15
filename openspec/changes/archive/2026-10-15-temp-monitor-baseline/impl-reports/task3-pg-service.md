# Task 3 实现报告：PostgreSQL 实现、服务进程与连真库测试

起点 SHA `fcd9e94`。没有 `config/local.env`，没有读过它。

## 验收证据

| 验收项 | 证据 |
|---|---|
| tasks 2.4 迁移、删表、种子 SQL | `db/migrations/0001_init.sql`、`db/drop.sql`、`db/seed.sql`。种子含 `demo-01`..`demo-03`，6 条读数，其中 `demo-03` 有 55.5。在本机临时 PostgreSQL 18.6 上连跑两次（同一事务、`ON_ERROR_STOP=1`、`lock_timeout=5s`）都成功，行数 3 / 6，`temperature_c>50` 有 1 条 |
| tasks 2.5 pgstore | `internal/store/pgstore/pgstore.go`。直接 INSERT / DELETE；`CreateDevice` 把 23505 映射为 `ErrAlreadyExists`，`AddReading` 把 23503 映射为 `ErrNotFound`；`CheckSchema` 缺表返回 `ErrSchemaMissing`。单元测试 `TestMapErr`（含被包装的错误、其它错误码、非 pg 错误） |
| tasks 2.6 testdb | `internal/testdb/testdb.go`：`go.mod` 定位仓根、读配置、`pgstore.Connect`（5 秒、`MaxConns=4`）、`CheckSchema`、`t.Cleanup(pool.Close)`。全文无 `t.Skip`（`grep -rn 't\.Skip' internal cmd` 无命中）。单元测试 `TestFindRoot`、`TestConfigFailure`（绝对路径输出为 `config/local.env line <n>: …`，加下一步提示） |
| tasks 2.7 pgstore 连真库测试 | `internal/store/pgstore/pgstore_integration_test.go`：`TestIntegrationContract`（storetest.Run）、`TestIntegrationConstraints`（CHECK 23514、主键 23505、外键 23503、级联删除）。`it-` 设备在 Cleanup 用独立 5 秒 context 删除 |
| tasks 3.3 服务进程 | `cmd/temp-monitor/main.go`：`MaxConns=4` 与 5 秒 ping 在 `pgstore.Connect`；`CheckSchema`；绑定成功后 INFO（`addr`、`threshold_c`）；`ReadHeaderTimeout` 5 秒；SIGINT/SIGTERM → `Shutdown` 5 秒，超时 WARN，仍关池，退出 0；main 等 `Shutdown` 返回；`Serve` 非预期错误退出 1。`run(ctx, getenv, envPath, stderr, open)` 的 `open` 是测试用接缝。单元测试见下 |
| tasks 3.4 端到端 | `internal/httpapi/e2e_integration_test.go` `TestIntegrationEndToEnd`：httptest.Server + httpapi + service + pgstore；五步（201、`ListReadings` 查到、日志恰好一条 WARN、204 后读数 0）。缓冲区 logger 经 `redact.Handler`（以 `pool.Config().ConnString()` 构造） |
| 服务进程用脱敏 slog 包装 | `main.go`：`slog.New(redact.Handler(slog.NewTextHandler(stderr, nil), cfg.DatabaseURL))`，全部日志（含 httpapi 的 5xx ERROR）走它 |
| `go vet -tags integration ./...` | 通过，见下 |
| tasks 3.5 | 见下 |

`cmd/temp-monitor/main_test.go` 的用例（不连库）：
- `TestStartupFailuresBeforeDatabase`：缺 URL、阈值 `abc`、`NaN`，文案与下一步逐字比对，退出 1。
- `TestUnreachableDatabaseNeverPrintsPassword`：口令 `s3cret-xyz`、`p(ss;w`、`p&ss`、`p%zz`、`p@ss#w`，stderr 必须等于固定两段文案，10 秒内退出 1。
- `TestOpenFailureMessages`、`TestOtherDriverErrorWithoutExtractablePasswordIsWithheld`：缺表文案；其它驱动错误脱敏（连接串与口令 → `***`）；口令提取不出时只输出 `error details withheld`。
- `TestListenFailureExitsOne`：端口占用，退出 1，文案含 `cannot listen on <addr>: …; the port may be in use; …`，并关闭存储。
- `TestGracefulShutdown`：取消 ctx 代替信号，启动 INFO 含地址和 `threshold_c=42.5`，退出 0，存储已关闭，日志不含连接串和口令。

红灯确认：
- 把 `pgstore.Connect` 的 ping 失败改成带驱动文本返回后，`TestUnreachableDatabaseNeverPrintsPassword` 四个子用例失败（输出里出现 `user=temp_monitor_demo`，`p@ss#w` 一例出现 `ss#w@127.0.0.1`）；恢复后通过。
- 在临时库删掉 `devices_id_check` 后，`TestIntegrationConstraints/CheckRejectsBadID` 失败（`error = <nil>`）；用 SQL 重建后恢复。

## 命令输出

`go vet ./...`、`go vet -tags integration ./...`：均无输出，通过。

`go test -race -count=1 ./...`：
```
ok  	.../cmd/temp-monitor	1.825s
ok  	.../internal/config	1.648s
ok  	.../internal/httpapi	1.826s
ok  	.../internal/redact	1.672s
ok  	.../internal/service	1.430s
?   	.../internal/store	[no test files]
ok  	.../internal/store/memstore	1.752s
ok  	.../internal/store/pgstore	1.751s
?   	.../internal/store/storetest	[no test files]
ok  	.../internal/testdb	1.751s
```

### 连真库层：无配置（`TM_DATABASE_URL` 未设，无 `config/local.env`）
`go test -tags integration -run '^TestIntegration' -p 1 -count=1 ./...`
```
--- FAIL: TestIntegrationEndToEnd (0.00s)
    e2e_integration_test.go:29: TM_DATABASE_URL is not set; copy config/example.env to config/local.env and fill it in
--- FAIL: TestIntegrationContract (0.00s)
    pgstore_integration_test.go:24: TM_DATABASE_URL is not set; copy config/example.env to config/local.env and fill it in
--- FAIL: TestIntegrationConstraints (0.00s)
    pgstore_integration_test.go:31: TM_DATABASE_URL is not set; copy config/example.env to config/local.env and fill it in
FAIL
```

### 连真库层：不可达端口
`TM_DATABASE_URL='postgres://u:pw@127.0.0.1:1/temp_monitor_demo?sslmode=disable&connect_timeout=5'`
```
--- FAIL: TestIntegrationEndToEnd (0.00s)
    e2e_integration_test.go:29: cannot connect to database; check the SSH tunnel and that the URL uses the session pool port 7432
--- FAIL: TestIntegrationContract (0.00s)
    pgstore_integration_test.go:24: cannot connect to database; check the SSH tunnel and that the URL uses the session pool port 7432
--- FAIL: TestIntegrationConstraints (0.00s)
    pgstore_integration_test.go:31: cannot connect to database; check the SSH tunnel and that the URL uses the session pool port 7432
FAIL
```

### 连真库层：缺表（本机临时库，未执行重建）
```
--- FAIL: TestIntegrationEndToEnd (0.01s)
    e2e_integration_test.go:29: table devices or readings not found; run scripts/db-reset.sh first
--- FAIL: TestIntegrationContract (0.00s)
    pgstore_integration_test.go:24: table devices or readings not found; run scripts/db-reset.sh first
--- FAIL: TestIntegrationConstraints (0.00s)
    pgstore_integration_test.go:31: table devices or readings not found; run scripts/db-reset.sh first
```

### 额外：本机临时 PostgreSQL 18.6 上跑通
本机装有 PostgreSQL 18.6 服务端。我在 `/tmp` 起了一个一次性实例（127.0.0.1:54329、trust 认证、库名 `temp_monitor_demo`），用 `psql` 按 design 第 3 步的方式重建，跑完后已停止并删除。它不是远端开发库，也不是 session 池，**不替代 Task 6 的验收**。
- `go test -tags integration -run '^TestIntegration' -p 1 -count=1 -v ./...`：`TestIntegrationEndToEnd`、`TestIntegrationContract`（9 个子用例）、`TestIntegrationConstraints`（4 个子用例）全部 PASS。
- 加 `-race -count=3` 再跑，通过；之后库里 `it-` 设备 0 个，设备总数仍为 3。
- 编译出的二进制连该库：`POST /devices`、`POST /devices/sim-x/readings`（77）返回 201，日志有 WARN `over-temperature alarm`；SIGTERM 退出 0，日志依次为 listening / shutting down / stopped；再起第二个实例占同一端口，退出 1，文案 `cannot listen on 127.0.0.1:18080: listen tcp 127.0.0.1:18080: bind: address already in use; the port may be in use; free it or set TM_LISTEN_ADDR`；另一个实例 SIGINT 退出 0。

## tasks 3.5 手动验证

仓根执行，无 `config/local.env`，环境变量按下表设置，二进制由 `go build ./cmd/temp-monitor` 产出。每次都检查 stderr 不含口令及片段（脚本逐个 `grep -F`，无命中）。

| 命令 | stderr | 退出码 |
|---|---|---|
| `tm-bin`（无任何配置） | `TM_DATABASE_URL is not set; copy config/example.env to config/local.env, fill it in, and start from the repo root` | 1 |
| `TM_DATABASE_URL='postgres://u:pw@127.0.0.1:1/temp_monitor_demo?sslmode=disable' TM_ALARM_THRESHOLD_C=abc tm-bin` | `TM_ALARM_THRESHOLD_C is invalid; must be a finite number, such as 50.0` | 1 |
| 口令 `s3cret-xyz`，`…@127.0.0.1:1/temp_monitor_demo?sslmode=disable` | `cannot connect to database; check the SSH tunnel and that the URL uses the session pool port 7432` | 1 |
| 口令 `p(ss;w` | 同上一行 | 1 |
| 口令 `p&ss` | 同上一行 | 1 |
| 口令 `p%zz` | 同上一行 | 1 |
| 口令 `p@ss#w` | 同上一行 | 1 |

不含 `s3cret-xyz`、`p(ss;w`、`p&ss`、`p%zz`、`p@ss#w`，也不含 `ss;w`、`ss#w`、`%zz`。

## 其它

- `go.mod`：`go mod tidy` 后 pgx 变成直接依赖，新增间接依赖 puddle、x/sync（待办池里的 Minor 已顺带满足）。
- `pgstore.Connect` / `ErrConnect` / `ErrSchemaMissing` 是 main 和 testdb 共用的建连入口，保证两处都是 `MaxConns=4`、5 秒、不带驱动文本。
- `internal/config` 未改。testdb 在自己这边把绝对路径替换回 `config/local.env`，满足 design 的文案原文。
- 红灯：`TestMapErr`、`TestFindRoot`、`TestConfigFailure`、main 的各用例是新代码一起写的；仅上面两处做了反向破坏确认。

## Concerns

- 没有连过真正的远端开发库（session 池 7432、隧道）。上面的通过结果来自本机临时 PostgreSQL，直连、无 PgBouncer。PgBouncer session 池下的行为留给 Task 6。
- 运行中 `Serve` 返回非预期错误（退出 1）与 `Shutdown` 超过 5 秒的 WARN 两条路径，代码已写，没有自动化测试（很难在不加更多接缝的前提下触发）。
- `main` 里 `Serve` 非预期错误后的日志只有一条 ERROR，没有额外的关闭连接动作；`Shutdown` 超时后不调用 `srv.Close()`，`pool.Close()` 会等进行中的请求（每个存储调用最多 5 秒）释放连接。

## 票外发现

无。

## 提交

见返回值中的 SHA。
