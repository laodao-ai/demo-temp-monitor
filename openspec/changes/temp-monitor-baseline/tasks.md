## 1. 仓库骨架与配置

- [ ] 1.1 建 `go.mod`（module `github.com/laodao-ai/temp-monitor`，`go 1.27`），引入 `github.com/jackc/pgx/v5`；`go build ./...` 通过 〔RUN-01〕
- [ ] 1.2 `.gitignore` 追加 `config/local.env` 与 `.pg-ops/`（`/.idea/` 已有）；验证：建一个 `config/local.env` 后 `git status --porcelain` 不显示它 〔RUN-05〕
- [ ] 1.3 写 `config/example.env`：三个键的占位值（主机 `127.0.0.1`、端口 7432、口令 `CHANGE_ME`、库名与用户名 `temp_monitor_demo`），注释写明连 session 池端口、口令做 URL 编码 〔RUN-05〕
- [ ] 1.4 实现 `internal/config`：环境变量 > `config/local.env` > 缺省；缺 `TM_DATABASE_URL`、阈值非数字时返回指明键名的错误。单元测试覆盖三种优先级、文件不存在、注释与空行、两种错误 〔RUN-01, RUN-02, RUN-03〕

## 2. 存储层

- [ ] 2.1 定义 `internal/store`：`Store` 接口（`CreateDevice` / `DeleteDevice` / `AddReading` / `ListReadings`）、`Device`、`Reading`、`ErrNotFound`、`ErrAlreadyExists` 〔DEV-01, DEV-02, ING-01, ING-02〕
- [ ] 2.2 写契约测试套件 `internal/store/storetest.Run(t, newStore)`，覆盖 TST-07 列出的八项行为，其中并发注册用 20 个 goroutine 〔TST-07, DEV-02, DEV-03, DEV-04, ING-02〕
- [ ] 2.3 实现 `internal/store/memstore`（一把 `sync.Mutex`，`ListReadings` 返回副本），单元测试调用契约套件，`go test -race` 通过 〔TST-07, TST-05, DEV-04〕
- [ ] 2.4 写 `db/migrations/0001_init.sql`、`db/drop.sql`、`db/seed.sql`（`demo-01`..`demo-03`，至少一条读数 > 50.0），内容按 design「Data Model & Lifecycle」 〔DB-04, DB-05〕
- [ ] 2.5 实现 `internal/store/pgstore`：直接 INSERT/DELETE，23505 → `ErrAlreadyExists`，23503 → `ErrNotFound`；`CheckSchema` 检查两张表存在 〔DEV-01, DEV-04, ING-02, RUN-03〕
- [ ] 2.6 实现 `internal/testdb.Open(t)`：从 `go.mod` 向上定位仓根，读配置，5 秒内建连，检查表存在；缺配置 / 连不上 / 缺表都 `t.Fatal`，失败信息给出下一步（复制 `config/example.env` 或运行 `scripts/db-reset.sh`），全文没有 `t.Skip` 〔TST-06〕
- [ ] 2.7 写 `pgstore` 连真库测试（`//go:build integration`，函数名 `TestIntegration...`）：调用契约套件，设备 ID 用 `it-` 加随机后缀并在 `t.Cleanup` 里删除；另测 DB-05 的检查约束与级联删除 〔TST-07, TST-09, DB-05〕

## 3. 业务逻辑与 HTTP

- [ ] 3.1 实现 `internal/service`：ID 正则校验，注册 / 注销 / 上报，入库成功且温度 > 阈值时写 WARN（`over-temperature alarm`，字段 `device_id` / `temperature_c` / `threshold_c`）。单元测试用 memstore 和写入缓冲区的 slog handler，覆盖等于阈值不告警、未注册不告警、存储出错不告警（用一个总是返回错误的测试替身）、连续两条各告警一次 〔DEV-01, DEV-02, DEV-03, ING-01, ING-02, ALM-01, ALM-02, ALM-03, ALM-04〕
- [ ] 3.2 实现 `internal/httpapi`：三个路由，`MaxBytesReader` 1 MiB，`temperature_c` 解码到 `*float64`，存储调用使用从请求 context 派生的 5 秒超时（design「外部依赖的超时与回滚」），错误映射到 400 / 404 / 409 / 500，5xx 响应体固定为 `{"error": "internal error"}` 并写 ERROR 日志。用 `httptest` 加 memstore 做单元测试，覆盖 DEV-01..03 与 ING-01..03 的全部场景 〔DEV-01, DEV-02, DEV-03, ING-01, ING-02, ING-03, RUN-04〕
- [ ] 3.3 实现 `cmd/temp-monitor`：读配置、建 pgxpool、5 秒 ping、`CheckSchema`、启动 INFO（监听地址和阈值，不含连接串）、`ReadHeaderTimeout`、SIGINT/SIGTERM 时 `Shutdown`（5 秒）并关闭连接池；任一前置条件失败就退出 1 〔RUN-02, RUN-03, RUN-04, RUN-06〕
- [ ] 3.4 写端到端连真库测试（`//go:build integration`）：`httptest.Server` + `httpapi` + `pgstore` + 缓冲区 logger，按 TST-08 的五步验证，设备 ID 用 `it-` 前缀并清理 〔TST-08, TST-09, ALM-01〕
- [ ] 3.5 手动验证不需要数据库的启动失败路径：缺 `TM_DATABASE_URL`、`TM_ALARM_THRESHOLD_C=abc`、连接串指向无人监听的端口（口令写 `s3cret-xyz`），三种情况都退出 1，stderr 指出原因且不含 `s3cret-xyz`；贴出输出 〔RUN-03, RUN-04〕

## 4. 脚本

- [ ] 4.1 写 `scripts/db-reset.sh`：带参数时退出 2；读 `TM_DATABASE_URL`（环境变量优先，否则 source `config/local.env`），都没有时退出 1；`current_database()` 不等于 `temp_monitor_demo` 时退出 3；在单个事务里执行 drop → migrations（按文件名字典序）→ seed；psql 设 `PGCONNECT_TIMEOUT=5`；不打印连接串 〔DB-01, DB-02, DB-03, RUN-04〕
- [ ] 4.2 写 `scripts/test.sh`：按 design「Test Runner」实现层选择、`-count=1`、unit 层 `-race`、integration 层 `-tags integration -run '^TestIntegration'`、jq 计数口径、失败输出打到 stderr、汇总行、退出码 `min(M,125)`、零执行时退出 1、缺 `go` / `jq` 时退出 1 〔TST-01, TST-02, TST-03, TST-04〕
- [ ] 4.3 在仓库的临时副本（scratchpad，不提交）里逐条验证 test.sh 的计数场景：编译失败计入 M、无测试文件的包不计入 K、子测试计数、`-run` 全部过滤掉时退出 1、构造 130 个失败子测试时退出码为 125、非法参数退出 2；把每条场景的实测输出贴进实现记录 〔TST-01, TST-03, TST-04〕
- [ ] 4.4 写 `scripts/simulate.sh [rounds]`：按 design「Simulator」实现，`TM_LISTEN_ADDR` 按「环境变量 > `config/local.env` > 缺省」取值，curl 带 `--max-time 5`；验证服务未运行时退出 1 〔ING-04〕

## 5. 文档

- [ ] 5.1 写 `README.md`：项目说明与四块范围，前置工具（Go 1.27、psql、jq、curl、bash），测过的版本（Go 1.27、PostgreSQL 18），配置方法，启动顺序（建库 → 隧道 → `config/local.env` → `db-reset.sh` → 启动服务 → `simulate.sh`），测试用法，以及「无鉴权，只应监听 127.0.0.1」的说明 〔RUN-02, RUN-05, ING-04, TST-01〕

## 6. 验证与收尾

- [ ] 6.1 在没有数据库配置的情况下执行 `scripts/test.sh unit`：退出码 0，N > 0，贴出汇总行 〔TST-05〕
- [ ] 6.2 在没有数据库配置的情况下执行 `scripts/test.sh integration`：连真库层用例全部计入 M，K = 0，退出码非零，失败信息提示复制 `config/example.env`，贴出输出 〔TST-06〕
- [ ] 6.3 🔒 **依赖远端开发库就绪**（人用 `/pg-dev-init` 以 `DB_NAME=temp_monitor_demo` 建库、起 session 池隧道、填 `config/local.env`）：
  - 在首次重建前启动服务，确认它以退出码 1 结束并提示运行 `db-reset.sh`；
  - 连续执行两次 `scripts/db-reset.sh` 都成功，且两次后的行数一致；
  - 把连接串临时改到别的库后执行，退出 3 且无改动；
  - 在临时改坏的 `db/seed.sql`（不提交）上执行，退出非零，且原有表和数据不变。
  〔DB-01, DB-02, DB-03, DB-04, RUN-03〕
- [ ] 6.4 🔒 依赖 6.3：执行 `scripts/test.sh`（all），汇总为 `N passed, 0 failed, K skipped (T total)`，N > 0，退出码 0；之后库里没有 `it-` 开头的设备 〔TST-01, TST-07, TST-08, TST-09〕
- [ ] 6.5 🔒 依赖 6.3：从仓根启动服务，执行 `scripts/simulate.sh 3`，库里 `sim-01`..`sim-03` 各新增 3 条读数，服务日志至少 3 条超温 WARN 且全文不含口令；然后 Ctrl-C，进程以 0 退出 〔ING-04, ALM-01, RUN-02, RUN-04, RUN-06〕
- [ ] 6.6 打 tag 前审计全部提交历史：`git log -p --all` 加 grep 查 IPv4（排除 `127.0.0.1`）、`postgres://` 后跟非 `CHANGE_ME` 口令的连接串、`git log --all --format='%ae %ce'` 中非 noreply 的邮箱；公司名和客户名由人在执行时临时输入作为 grep 关键词，不写入任何文件；结果无命中 〔RUN-05〕

> 打 tag `baseline` 在 sdflow-done 把本分支合并到 main 之后进行，前提是 6.4、6.6 已通过。它写进 hand-off，不作为本清单的任务。

## 任务依赖图

```
 层 0（无前置）   1.1   1.2   1.3   5.1
                   │
 层 1             ├──▶ 1.4
                   ├──▶ 2.1 ──▶ 2.2 ──▶ 2.3 ──▶ 3.1 ──▶ 3.2 ──▶ 3.3 ──┬──▶ 3.5
                   │     │       │                       │       │     └──▶ 4.4
                   │     └──▶ 2.5 ────────┐              │       │
                   ├──▶ 2.4 ──▶ 4.1       ├──▶ 2.7 ◀──── 2.2     │
                   └──▶ 2.6 ──────────────┤                       │
                                           └──▶ 3.4 ◀──── 3.2 ◀───┘
                   4.2 ──▶ 4.3 ; 4.2 ──▶ 6.1 ; 4.2 ──▶ 6.2 （2.6 + 2.7 也是 6.2 的前置）

 等远端开发库 🔒   6.3 ◀── 4.1 + 3.3
                   6.4 ◀── 6.3 + 4.2 + 2.7 + 3.4
                   6.5 ◀── 6.3 + 3.3 + 4.4
                   6.6 ◀── 6.4（最后执行，审计全部历史）

 关键路径（编码）：1.1 → 2.1 → 2.2 → 2.3 → 3.1 → 3.2 → 3.3 → 4.4
```

## 测试覆盖图

| 代码路径 | 单元层（memstore，`-race`） | 连真库层（`integration` tag） | 人工 / 脚本验证 |
|---|---|---|---|
| `internal/config` 优先级与校验 | 1.4 | — | 3.5、6.5 |
| 存储契约（注册、重复、注销级联、不存在、顺序、并发） | 2.3 跑 storetest | 2.7 跑同一份 storetest | — |
| `pgstore` 错误码映射、`CheckSchema` | — | 2.7、3.4 | 6.5 |
| 数据库约束（CHECK、FK、CASCADE） | — | 2.7 | 6.3 |
| `service` 校验、注册、注销、上报、告警判定 | 3.1 | 3.4 | 6.5 |
| `httpapi` 路由、解码、状态码、5xx 文案、1 MiB 上限 | 3.2 | 3.4 | — |
| `cmd/temp-monitor` 启动失败路径、优雅退出、日志不含口令 | — | — | 3.5（无库可验的失败路径）、6.3（缺表）、6.5（正常启动与 SIGINT） |
| `scripts/test.sh` 计数与退出码 | — | — | 4.3、6.1、6.2、6.4 |
| `scripts/db-reset.sh` 守卫、重建、回滚 | — | — | 6.3 |
| `scripts/simulate.sh` | — | — | 4.4、6.5 |
| 公开仓约束（gitignore、历史审计） | — | — | 1.2、6.6 |
