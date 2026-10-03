## 1. 仓库骨架与配置

- [ ] 1.1 建 `go.mod`（module `github.com/laodao-ai/temp-monitor`，`go 1.27`），引入 `github.com/jackc/pgx/v5`；`go build ./...` 通过 〔RUN-01〕
- [ ] 1.2 `.gitignore` 追加 `config/local.env` 与 `.pg-ops/`（`/.idea/` 已有）；验证：建一个 `config/local.env` 后 `git status --porcelain` 不显示它 〔RUN-05〕
- [ ] 1.3 写 `config/example.env`：三个键的占位值（主机 `127.0.0.1`、端口 7432、口令 `CHANGE_ME`、库名与用户名 `temp_monitor_demo`），注释写明连 session 池端口、口令做 URL 编码 〔RUN-05〕
- [ ] 1.4 实现 `internal/config`：环境变量 > `config/local.env` > 缺省；文件按 design「Configuration Surface」的文法解析；缺 `TM_DATABASE_URL`、阈值不是有限数时返回指明键名的错误 [spec-review-amendment]。单元测试覆盖三种优先级、文件不存在、注释与空行、两种错误，以及：值含 `=` 与 `&`、空值、带引号、无 `=` 的行、未知键、CRLF 行尾、阈值为 `NaN` / `Inf` / `-Inf` [spec-review-amendment] 〔RUN-01, RUN-02, RUN-03〕

## 2. 存储层

- [ ] 2.1 定义 `internal/store`：`Store` 接口（`CreateDevice` / `DeleteDevice` / `AddReading` / `ListReadings`）、`Device`、`Reading`、`ErrNotFound`、`ErrAlreadyExists` 〔DEV-01, DEV-02, ING-01, ING-02〕
- [ ] 2.2 写契约测试套件 `internal/store/storetest.Run(t, newStore)`，覆盖 TST-07 列出的九项行为，其中并发注册用 20 个 goroutine；套件内部为每个用例生成 `it-<随机后缀>` 的设备 ID，两个实现共用；工厂函数返回的存储里可以已有别的数据；「并发注销与上报」一项在 pgstore 上重复若干轮 [spec-review-amendment] 〔TST-07, DEV-02, DEV-03, DEV-04, ING-02〕
- [ ] 2.3 实现 `internal/store/memstore`（一把 `sync.Mutex`，`ListReadings` 返回副本），单元测试调用契约套件，`go test -race` 通过 〔TST-07, TST-05, DEV-04〕
- [ ] 2.4 写 `db/migrations/0001_init.sql`、`db/drop.sql`、`db/seed.sql`（`demo-01`..`demo-03`，至少一条读数 > 50.0），内容按 design「Data Model & Lifecycle」 〔DB-04, DB-05〕
- [ ] 2.5 实现 `internal/store/pgstore`：直接 INSERT/DELETE，23505 → `ErrAlreadyExists`，23503 → `ErrNotFound`；`CheckSchema` 检查两张表存在 〔DEV-01, DEV-04, ING-02, RUN-03〕
- [ ] 2.6 实现 `internal/testdb.Open(t)`：从 `go.mod` 向上定位仓根，读配置，5 秒内建连，检查表存在；缺配置 / 连不上 / 缺表都 `t.Fatal`，失败信息给出下一步（复制 `config/example.env` 或运行 `scripts/db-reset.sh`），全文没有 `t.Skip`；建的池 `MaxConns=4`，并 `t.Cleanup(pool.Close)` [spec-review-amendment] 〔TST-06〕
- [ ] 2.7 写 `pgstore` 连真库测试（`//go:build integration`，函数名 `TestIntegration...`）：调用契约套件，设备 ID 由套件生成（`it-` 加随机后缀）并在 `t.Cleanup` 里删除 [spec-review-amendment]；另测 DB-05 的检查约束与级联删除，级联删除用测试自己建的 `it-` 设备 [spec-review-amendment] 〔TST-07, TST-09, DB-05〕

## 3. 业务逻辑与 HTTP

- [ ] 3.1 实现 `internal/service`：ID 正则校验，注册 / 注销 / 上报，入库成功且温度 > 阈值时写 WARN（`over-temperature alarm`，字段 `device_id` / `temperature_c` / `threshold_c`）。单元测试用 memstore 和写入缓冲区的 slog handler，覆盖等于阈值不告警、未注册不告警、存储出错不告警（用一个总是返回错误的测试替身）、连续两条各告警一次 〔DEV-01, DEV-02, DEV-03, ING-01, ING-02, ALM-01, ALM-02, ALM-03, ALM-04〕
- [ ] 3.2 实现 `internal/httpapi`：三个路由；请求体先 `io.ReadAll(http.MaxBytesReader(…, 1 MiB))` 再 `json.Unmarshal`，超限、尾随非空白内容、空请求体都回 400，未知字段忽略，不检查 `Content-Type` [spec-review-amendment]；`temperature_c` 解码到 `*float64`，存储调用使用从请求 context 派生的 5 秒超时（design「外部依赖的超时与回滚」），错误映射到 400 / 404 / 409 / 500，400 文案按 design「Error Messages」[spec-review-amendment]，5xx 响应体固定为 `{"error": "internal error"}` 并写 ERROR 日志（底层错误脱敏）[spec-review-amendment]。用 `httptest` 加 memstore 做单元测试，覆盖 DEV-01..03 与 ING-01..03 的全部场景 〔DEV-01, DEV-02, DEV-03, ING-01, ING-02, ING-03, RUN-04〕
- [ ] 3.3 实现 `cmd/temp-monitor`：读配置、建 pgxpool（`MaxConns=4`）[spec-review-amendment]、5 秒 ping、`CheckSchema`、绑定监听地址成功后打启动 INFO（监听地址和阈值，不含连接串）[spec-review-amendment]、`ReadHeaderTimeout` 为 5 秒 [spec-review-amendment]、SIGINT/SIGTERM 时 `Shutdown`（5 秒）并关闭连接池；任一前置条件失败就退出 1。另外：[spec-review-amendment]
  - `Shutdown` 超时记 WARN，仍关闭连接池，退出 0；main 等 `Shutdown` 返回后才退出；运行中 `Serve` 返回非预期错误时退出 1；[spec-review-amendment]
  - 输出来自 pgx 的错误文本前，按 design「Security & Data Protection」把连接串和口令替换为 `***`；口令提取不出来时只输出固定文案；[spec-review-amendment]
  - 启动失败的文案按 design「Error Messages」。[spec-review-amendment]
  〔RUN-02, RUN-03, RUN-04, RUN-06〕
- [ ] 3.4 写端到端连真库测试（`//go:build integration`）：`httptest.Server` + `httpapi` + `pgstore` + 缓冲区 logger，按 TST-08 的五步验证，设备 ID 用 `it-` 前缀并清理 〔TST-08, TST-09, ALM-01〕
- [ ] 3.5 手动验证不需要数据库的启动失败路径：缺 `TM_DATABASE_URL`、`TM_ALARM_THRESHOLD_C=abc`、连接串指向无人监听的端口（口令写 `s3cret-xyz`），三种情况都退出 1，stderr 指出原因且不含 `s3cret-xyz`；贴出输出。另外：[spec-review-amendment]
  - 连接串指向无人监听的端口，口令依次换成 `p(ss;w`、`p&ss`、`p%zz`、`p@ss#w`（均不做 URL 编码），每次都退出 1，stderr 只有固定文案 `cannot connect to database` 和下一步提示，不含该口令，也不含 `ss;w`、`ss#w`、`%zz`。[spec-review-amendment]
  〔RUN-03, RUN-04〕

## 4. 脚本

- [ ] 4.1 写 `scripts/db-reset.sh`：开头切到仓根 [spec-review-amendment]；带参数时退出 2；读 `TM_DATABASE_URL`：环境变量非空时直接用，否则按 design「Configuration Surface」的文法从 `config/local.env` 逐行读这一个键，不 `source` [spec-review-amendment]，都没有时退出 1；`current_database()` 不等于 `temp_monitor_demo` 时退出 3；重建命令带同一个连接串，按 design「Demo Database Reset」第 3 步执行，含 `SET lock_timeout = '5s'`，在单个事务里执行 drop → migrations（按文件名字典序）→ seed [spec-review-amendment]；psql 设 `PGCONNECT_TIMEOUT=5`；psql 失败（连不上、SQL 出错、等锁超时）时捕获其退出状态，映射为退出 4 [spec-review-amendment]；psql 的 stderr 先捕获、脱敏后再输出，不打印连接串和口令 [spec-review-amendment]。不连库可做的验证：[spec-review-amendment]
  - 连接串指向无人监听的端口时退出 4，输出只有固定文案；[spec-review-amendment]
  - 脚本里守卫和重建两条 psql 命令都带 `"$TM_DATABASE_URL"`（读脚本确认）；[spec-review-amendment]
  - 口令依次为 `p(ss;w`、`p&ss`、`p%zz`、`p@ss#w`（均不做 URL 编码）时，stdout 和 stderr 不含该口令，也不含 `ss;w`、`ss#w`、`%zz`；[spec-review-amendment]
  - `config/local.env` 含 `TM_DATABASE_URL=$(touch marker)` 时，执行后不产生 `marker` 文件。[spec-review-amendment]
  〔DB-01, DB-02, DB-03, RUN-01, RUN-04〕
- [ ] 4.2 写 `scripts/test.sh`：开头切到仓根 [spec-review-amendment]；按 design「Test Runner」实现层选择、`-count=1`、unit 层 `-race`、integration 层 `-p 1 -tags integration -run '^TestIntegration'` [spec-review-amendment]、两层分别执行与计数再求和、包级 fail 按「层 + 包」判定 [spec-review-amendment]、jq 计数口径、某层 `go test` 非零退出而该层 M = 0 时 M 加 1 [spec-review-amendment]、`go test` 的 stderr 直接透传 [spec-review-amendment]、失败输出与 `build-output` 及包级 output 打到 stderr [spec-review-amendment]、汇总行、退出码 `min(M,125)`、任一所选层零执行时打 `no tests executed in <层名> layer` 并退出 1 [spec-review-amendment]、jq 解析失败时退出 1 且不打汇总行 [spec-review-amendment]、缺 `go` / `jq` 时退出 1 〔TST-01, TST-02, TST-03, TST-04〕
- [ ] 4.3 在仓库的临时副本（scratchpad，不提交）里逐条验证 test.sh 的计数场景：编译失败计入 M、无测试文件的包不计入 K、子测试计数、`-run` 全部过滤掉时退出 1、构造 130 个失败子测试时退出码为 125、非法参数退出 2；另外验证 [spec-review-amendment]：某层零用例的 `all` 退出 1 且 stderr 含 `no tests executed in integration layer`、`go test` 被 `kill -9` 时退出码非零、编译失败时 stderr 含编译错误行、缺 `go` 或 `jq` 时退出 1、3 个失败时退出 3；把每条场景的实测输出贴进实现记录 〔TST-01, TST-02, TST-03, TST-04〕
- [ ] 4.4 写 `scripts/simulate.sh [rounds]`：开头切到仓根，设 `LC_ALL=C` [spec-review-amendment]；按 design「Simulator」实现，`TM_LISTEN_ADDR` 按「环境变量非空则用 > `config/local.env` > 缺省」取值，读文件按 design「Configuration Surface」的文法逐行解析，不 `source` [spec-review-amendment]；以 `:` 开头时目标主机用 `127.0.0.1` [spec-review-amendment]；`rounds` 不是正整数时打印用法并退出 2 [spec-review-amendment]；curl 带 `--max-time 5`；验证服务未运行时退出 1，`rounds` 为 `0` 或 `abc` 时退出 2，`config/local.env` 含 `TM_DATABASE_URL=$(touch marker)` 时不产生 `marker` 文件 [spec-review-amendment] 〔ING-04, RUN-01〕

## 5. 文档

- [ ] 5.1 写 `README.md`：项目说明与四块范围，前置工具（Go 1.27、psql、jq、curl、bash、C 编译器（`-race` 需要））[spec-review-amendment]，测过的版本（Go 1.27、PostgreSQL 18；「测过 PostgreSQL 18」这句在 6.4 通过后确认）[spec-review-amendment]，配置方法，启动顺序（建库 → 隧道 → `config/local.env` → `db-reset.sh` → 启动服务 → `simulate.sh`），测试用法（第一个命令是 `scripts/test.sh unit`，不需要数据库；没配库时直接跑 `scripts/test.sh` 会失败；用法错误和缺工具时没有汇总行）[spec-review-amendment]，以及「无鉴权，只应监听 127.0.0.1」的说明 〔RUN-02, RUN-05, ING-04, TST-01〕

## 6. 验证与收尾

- [ ] 6.1 在没有数据库配置的情况下执行 `scripts/test.sh unit`：退出码 0，N > 0，贴出汇总行 〔TST-05〕
- [ ] 6.2 执行 `scripts/test.sh integration`，这一步不需要远端库，分两个子步骤 [spec-review-amendment]：
  - 没有数据库配置：连真库层用例全部计入 M，K = 0，退出码非零，失败信息提示复制 `config/example.env`，贴出输出；
  - 连接串指向不可达端口：连真库层用例计入 M，退出码非零，贴出输出。[spec-review-amendment]
  〔TST-06〕
- [ ] 6.3 🔒 **依赖远端开发库就绪**（人用 `/pg-dev-init` 以 `DB_NAME=temp_monitor_demo` 建库、起 session 池隧道、填 `config/local.env`）：
  - 在首次重建前启动服务，确认它以退出码 1 结束并提示运行 `db-reset.sh`；
  - 在首次重建前执行 `scripts/test.sh integration`，连真库层用例失败，失败信息提示先运行 `scripts/db-reset.sh`；[spec-review-amendment]
  - 连续执行两次 `scripts/db-reset.sh` 都成功，且两次后的行数一致；
  - 重建后 `demo-01`..`demo-03` 存在，且有 `temperature_c > 50` 的读数；[spec-review-amendment]
  - 服务写入 `sim-01` 后再重建，`sim-01` 消失；[spec-review-amendment]
  - 把连接串临时改到别的库后执行，退出 3 且无改动；
  - 在临时改坏的 `db/seed.sql`（不提交）上执行，退出 4，且原有表和数据不变。[spec-review-amendment]
  〔DB-01, DB-02, DB-03, DB-04, RUN-03, TST-06〕
- [ ] 6.4 🔒 依赖 6.3：执行 `scripts/test.sh`（all），汇总为 `N passed, 0 failed, K skipped (T total)`，N > 0，退出码 0；之后库里没有 `it-` 开头的设备 〔TST-01, TST-07, TST-08, TST-09〕
- [ ] 6.5 🔒 依赖 6.3：从仓根启动服务，执行 `scripts/simulate.sh 3`，库里 `sim-01`..`sim-03` 各新增 3 条读数，服务日志至少 3 条超温 WARN 且全文不含口令；然后 Ctrl-C，进程以 0 退出。另外：[spec-review-amendment]
  - 再启动一次，用 SIGTERM 结束，进程以 0 退出；[spec-review-amendment]
  - 启动后执行 `lsof -nP -iTCP -sTCP:LISTEN`，确认服务只监听 `127.0.0.1`；[spec-review-amendment]
  - 端口被占用：服务运行时再启动一个实例，后者退出 1，stderr 指出地址无法绑定。[spec-review-amendment]
  〔ING-04, ALM-01, RUN-02, RUN-03, RUN-04, RUN-06〕
- [ ] 6.6 打 tag 前审计全部提交历史，结果无命中 [spec-review-amendment]：
  - IPv4：`git log -p -m --all` 的输出用 `grep -oE` 抽出全部 IPv4 token，再排除 `127.0.0.1` 这个 token（不是整行排除）；[spec-review-amendment]
  - 连接串与口令：在同一输出里查 `postgres://` 后跟非 `CHANGE_ME` 口令的连接串，以及 `PGPASSWORD=`；[spec-review-amendment]
  - 邮箱与姓名：`git log --all --format='%ae %ce %an %cn'` 中非 noreply 的邮箱；其中的姓名由人逐个核对；[spec-review-amendment]
  - 公司名和客户名：由人在执行时临时输入作为 grep 关键词，不写入任何文件。
  〔RUN-05〕

> 打 tag `baseline` 在 sdflow-done 把本分支合并到 main 之后进行，前提是 6.4、6.6 已通过。它写进 hand-off，不作为本清单的任务。合并到 main 后、打 tag 前，在 main 上把 6.6 的审计重跑一次，这一步也写进 hand-off。[spec-review-amendment]

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
                   补充边：1.4 ──▶ 2.6 ; 1.4 ──▶ 3.3 ; 2.5 ──▶ 3.3 ; 2.6 ──▶ 3.4 [spec-review-amendment]

 等远端开发库 🔒   6.3 ◀── 4.1 + 3.3 + 4.2 + 2.6 + 2.7 [spec-review-amendment]
                   6.4 ◀── 6.3 + 4.2 + 2.7 + 3.4
                   6.5 ◀── 6.3 + 3.3 + 4.4
                   6.6 ◀── 6.4（最后执行，审计全部历史）
                   5.1 中「测过 PostgreSQL 18」一句 ◀── 6.4 [spec-review-amendment]

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
| `scripts/db-reset.sh` 守卫、重建、回滚 | — | — | 4.1（不连库可验的退出码、口令、不执行文件内容）、6.3 [spec-review-amendment] |
| `scripts/simulate.sh` | — | — | 4.4、6.5 |
| 公开仓约束（gitignore、历史审计） | — | — | 1.2、6.6 |
