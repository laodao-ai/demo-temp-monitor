## Context

动机见 proposal.md「Why」。下面只列影响做法的现状与约束：

- 仓里没有 Go 代码，从零开始。本机有 Go 1.27.1、psql 18.6、jq、Apple clang（`-race` 可用），没有 docker。
- 远端开发库按 `/pg-dev-init` 的约定建，当前还没建：
  - owner 角色和库同名，默认没有 CREATEDB；
  - PgBouncer 的 transaction 池在 6432，session 池在 7432；
  - 只监听服务器回环地址，本机经 SSH 隧道在 `127.0.0.1` 访问。
- 仓库托管在 GitHub `laodao-ai/demo-temp-monitor`（private），以后公开。

## Goals / Non-Goals

**Goals:**

- 业务逻辑只依赖存储接口。单元测试用内存实现，不需要数据库；连真库测试和内存实现跑同一套契约测试。
- 两层测试由一个脚本统一执行和计数，计数口径不会把编译失败或零用例算成绿。
- 任何会把连接串打印出来的路径都不存在：日志、错误响应、脚本输出都不带连接串，也不带口令的片段。[spec-review-amendment]

**Non-Goals:**

- 不做限流和连接池调优。[spec-review-amendment]
- 不兼容 PgBouncer transaction 池（见 Risks）。
- 不做增量迁移；`db-reset.sh` 总是从零重建。

## Design Diagrams

### C4 Context

```
  ┌──────────────┐  HTTP (127.0.0.1)  ┌──────────────────┐  SSH 隧道   ┌──────────────────────┐
  │ simulate.sh  │ ─────────────────▶ │  temp-monitor    │ ──────────▶ │ 远端开发库             │
  │ (bash+curl)  │                    │  Go 服务          │  127.0.0.1  │ PgBouncer session 池  │
  └──────────────┘                    └──────────────────┘    :7432    │ → PostgreSQL 18       │
  ┌──────────────┐                            │ stderr                 │ 库 temp_monitor_demo  │
  │ 开发者        │── test.sh / db-reset.sh ──┼──────────────────────▶ └──────────────────────┘
  └──────────────┘                            ▼
                                        告警日志 (slog WARN)
```

### C4 Container + 组件 / 依赖图

```
 cmd/temp-monitor (main：组装 + 优雅退出)
   │
   ├──▶ internal/config ──────────────────── 读 env > config/local.env > 缺省
   ├──▶ internal/httpapi ──▶ internal/service ──▶ internal/store (接口 + 类型 + 哨兵错误)
   │                                                   ▲            ▲
   └──▶ internal/store/pgstore ───────────────────────┘            │
                 │ pgx v5 (pgxpool)            internal/store/memstore
                 ▼                                       ▲
           PostgreSQL                internal/store/storetest (契约测试套件)
                                       ▲ 被 memstore 单元测试、pgstore 连真库测试共同调用
 internal/testdb (仅连真库测试用：定位仓根、读配置、建连、查表是否存在)

 db/drop.sql, db/migrations/*.sql, db/seed.sql ◀── scripts/db-reset.sh (psql)
 scripts/test.sh ──▶ go test -json ──▶ jq 计数
 scripts/simulate.sh ──▶ curl ──▶ HTTP 接口
```

依赖方向只能向下：`httpapi → service → store`。`service` 不 import `pgstore` 或 `memstore`，具体实现只在 `main` 和测试里注入。

### 序列图：上报一条超温读数（跨 handler / service / store / logger）

```
 simulate.sh      httpapi            service               store(pg)            slog
     │ POST /devices/sensor-01/readings {"temperature_c":55.0}
     │──────────────▶│                  │                      │                   │
     │               │ 解码+校验字段     │                      │                   │
     │               │ Report(ctx,id,t)─▶│ 校验 ID 格式         │                   │
     │               │                  │ AddReading(ctx,id,t)─▶│ INSERT ... 外键检查│
     │               │                  │◀──── Reading / ErrNotFound ─│            │
     │               │                  │ t > 阈值 ?            │                   │
     │               │                  │──── Warn("over-temperature alarm") ─────▶│
     │               │◀── Reading ───────│                      │                   │
     │◀── 201 + JSON ─│                  │                      │                   │
```

ErrNotFound 时 service 直接返回，不判阈值，也不告警，handler 回 404。

### 序列图：并发注销与上报（TG-26）

```
   请求 A: DELETE /devices/d1            请求 B: POST /devices/d1/readings
          │                                        │
   pg:  DELETE FROM devices ...           pg: INSERT INTO readings ... (外键引用 d1)
          │                                        │
   情形 1：B 的 INSERT 先提交 → A 的 DELETE 级联删掉这条读数 → B 回 201，A 回 204，库里无残留
   情形 2：A 先提交 → B 的 INSERT 违反外键 (23503) → 映射为 ErrNotFound → B 回 404
   memstore：同一把 sync.Mutex 串行化两者，结果只可能是上面两种之一
```

## Component List

| 组件 | 职责 | 对外暴露 |
|---|---|---|
| `cmd/temp-monitor` | 读配置、建连接池、启动前检查表存在、组装、监听、收到 SIGINT/SIGTERM 后优雅退出 | 进程 |
| `internal/config` | 按「环境变量 > `config/local.env` > 缺省」解析三个配置项 | `Load(getenv, path) (Config, error)` |
| `internal/store` | `Store` 接口、`Device` / `Reading` 类型、`ErrNotFound` / `ErrAlreadyExists` | 接口与类型 |
| `internal/store/memstore` | 内存实现，`sync.Mutex` 保护两张 map | `New() *Store` |
| `internal/store/pgstore` | PostgreSQL 实现，把 23505/23503 映射为哨兵错误 | `New(pool) *Store`、`CheckSchema(ctx, pool)` |
| `internal/store/storetest` | 契约测试套件，接收一个返回存储的工厂函数，存储里可以已有别的数据；内部为每个用例生成 `it-<随机后缀>` 的设备 ID，两个实现共用 [spec-review-amendment] | `Run(t, newStore)` |
| `internal/service` | ID 格式校验、注册、注销、上报、阈值判断与告警日志 | `New(store, thresholdC, logger)` |
| `internal/httpapi` | 三个路由、JSON 编解码、错误到状态码的映射 | `NewHandler(svc, logger) http.Handler` |
| `internal/testdb` | 连真库测试的公共准备：从 `go.mod` 向上定位仓根、读配置、建连、检查表存在；任何一步失败都 `t.Fatal`；建的池 `MaxConns=4`，并 `t.Cleanup(pool.Close)` [spec-review-amendment] | `Open(t) *pgxpool.Pool` |
| `db/` | `drop.sql`、`migrations/0001_init.sql`、`seed.sql` | SQL 文件 |
| `scripts/` | `test.sh`、`db-reset.sh`、`simulate.sh` | 命令行 |

`Store` 接口：

```go
type Store interface {
	CreateDevice(ctx context.Context, id string) (Device, error)                // 已存在 → ErrAlreadyExists
	DeleteDevice(ctx context.Context, id string) error                          // 不存在 → ErrNotFound；读数级联删除
	AddReading(ctx context.Context, deviceID string, tempC float64) (Reading, error) // 设备不存在 → ErrNotFound
	ListReadings(ctx context.Context, deviceID string) ([]Reading, error)       // 只供契约测试；按 id 升序
}
```

`ListReadings` 只按设备取，所以存储里已有的别的数据不影响契约测试。[spec-review-amendment]

## Data Model & Lifecycle

```sql
-- db/migrations/0001_init.sql
CREATE TABLE devices (
    id            text        PRIMARY KEY CHECK (id ~ '^[a-z0-9-]{1,64}$'),
    registered_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE readings (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    device_id     text             NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    temperature_c double precision NOT NULL,
    recorded_at   timestamptz      NOT NULL DEFAULT now()
);
CREATE INDEX readings_device_id_idx ON readings (device_id, id);
```

- `db/drop.sql`：`DROP TABLE IF EXISTS readings, devices CASCADE;`。新增表时必须同步改这个文件，`db-reset.sh` 连跑两次会暴露遗漏。
- `db/seed.sql`：设备 `demo-01`、`demo-02`、`demo-03`，每台几条读数，其中至少一条高于 50.0。种子数据入库不触发告警，因为告警只在上报路径上发生。
- **生命周期**：设备只有「已注册」一种持久状态。注销即删行，读数随之级联删除。同一 ID 可再次注册，作为新设备，没有历史读数。
- **ID 前缀约定**：种子数据用 `demo-`，模拟脚本用 `sim-`，连真库测试用 `it-<随机>`，三者互不干扰。

### v_old / v_new（TG-04，D-2 完整列清单）

v_old 中两张表都不存在，所以下表每一列的 v_old 都是「不存在」。

| 表.列 | 类型 | 约束 | 默认值 | nullable |
|---|---|---|---|---|
| `devices.id` | `text` | PRIMARY KEY；CHECK `id ~ '^[a-z0-9-]{1,64}$'` | 无 | NOT NULL（主键隐含） |
| `devices.registered_at` | `timestamptz` | — | `now()` | NOT NULL |
| `readings.id` | `bigint` | PRIMARY KEY；`GENERATED ALWAYS AS IDENTITY` | 自增序列 | NOT NULL（主键隐含） |
| `readings.device_id` | `text` | FOREIGN KEY → `devices(id)` ON DELETE CASCADE | 无 | NOT NULL |
| `readings.temperature_c` | `double precision` | — | 无 | NOT NULL |
| `readings.recorded_at` | `timestamptz` | — | `now()` | NOT NULL |
| 索引 `readings_device_id_idx` | btree `(device_id, id)` | 非唯一 | — | — |

全新库，没有存量数据要迁移。回退方式是 `db/drop.sql`。

### 外部依赖的超时与回滚（D-4）

| 调用点 | 超时 | 超时后的行为 |
|---|---|---|
| 服务启动时建连并 ping | 5 秒 | 退出 1 |
| 服务启动时 `CheckSchema` | 5 秒 | 退出 1 |
| 每个 HTTP 请求里的存储调用 | 5 秒：handler 以请求 context 派生 `context.WithTimeout(5s)` 传给 service | 回 500 `{"error": "internal error"}`，写 ERROR |
| `db-reset.sh` 里的 psql | 建连用 `PGCONNECT_TIMEOUT=5`；会话开头 `SET lock_timeout = '5s'`，等锁超过 5 秒则失败并回滚 [spec-review-amendment] | 退出 4 [spec-review-amendment] |
| `simulate.sh` 里的 curl | `--max-time 5` | 退出 1 [spec-review-amendment] |
| 连真库测试建连 | 5 秒 | `t.Fatal` |
| 连真库测试的清理 | 5 秒：独立 context，由 `context.Background()` 派生，不用 `t.Context()` [spec-review-amendment] | 可能残留 `it-` 设备，靠 `scripts/db-reset.sh` 清（见 Risks）[spec-review-amendment] |

写操作的回滚路径：

- **服务**：每个写操作都是单条 `INSERT` 或 `DELETE`，由 PostgreSQL 保证原子性，不需要应用层回滚。超时时结果未知：语句可能已提交，不会半写。[spec-review-amendment]
  - 服务不自动重试。[spec-review-amendment]
  - `POST /devices/{id}/readings` 非幂等，客户端重试可能产生重复读数。[spec-review-amendment]
  - 超时窗口内已入库的超温读数，可能没有告警日志。[spec-review-amendment]
- **重建脚本**：`db-reset.sh` 用 `--single-transaction` 加 `ON_ERROR_STOP=1`，任一步失败时整体回滚（见 DB-03）。
- **连真库测试**：用 `t.Cleanup` 删除自己创建的 `it-` 设备，读数随之级联删除。清理用上表的独立 5 秒 context。[spec-review-amendment]

## API

三个路由，均用 Go 标准库 `http.ServeMux` 的方法 + 路径模式注册，路径参数用 `r.PathValue("id")` 取。

- 三个已注册路由的 handler，错误响应统一为 `{"error": "<说明>"}`。[spec-review-amendment]
- 未注册的路径和方法不符，由 `http.ServeMux` 返回纯文本 404 / 405，不保证 JSON。[spec-review-amendment]
- 5xx 只回 `{"error": "internal error"}`，详细错误只写服务端日志。
- 400 的文案按原因区分，见「Error Messages」。[spec-review-amendment]
- **请求体读取**（注册与上报相同）：先 `io.ReadAll(http.MaxBytesReader(…, 1 MiB))`，再 `json.Unmarshal`。[spec-review-amendment]
  - 超过 1 MiB、合法 JSON 后有非空白的尾随内容、请求体为空，都回 400。[spec-review-amendment]
  - 未知字段忽略。不检查 `Content-Type`。[spec-review-amendment]
- **幂等性**：`POST /devices` 重复调用回 409；`DELETE /devices/{id}` 重复调用回 404；`POST /devices/{id}/readings` 非幂等。[spec-review-amendment]

**注册** `POST /devices`

```json
// 请求
{"id": "sensor-01"}
// 201
{"id": "sensor-01", "registered_at": "2026-10-03T12:00:00Z"}
// 400：id 不匹配 ^[a-z0-9-]{1,64}$（JSON 无法解析、缺 id 的文案见「Error Messages」）[spec-review-amendment]
{"error": "invalid device id: must match ^[a-z0-9-]{1,64}$"}
// 409：已注册
{"error": "device already registered"}
```

**注销** `DELETE /devices/{id}` → 204 无响应体；id 格式非法 400；未注册 404 `{"error": "device not found"}`。

**上报** `POST /devices/{id}/readings`

```json
// 请求
{"temperature_c": 55.0}
// 201
{"id": 42, "device_id": "sensor-01", "temperature_c": 55.0, "recorded_at": "2026-10-03T12:00:01Z"}
// 400：缺 temperature_c（其它原因的文案见「Error Messages」）[spec-review-amendment]
{"error": "temperature_c is required"}
// 404：设备未注册或已注销
{"error": "device not found"}
```

`temperature_c` 解码到 `*float64`，用来区分「缺字段」和「值为 0」。

## Configuration Surface（TG-28）

| 键 | 缺省 | 说明 |
|---|---|---|
| `TM_DATABASE_URL` | 无，必填 | pgx 连接串；示例 `postgres://temp_monitor_demo:CHANGE_ME@127.0.0.1:7432/temp_monitor_demo?sslmode=disable` |
| `TM_LISTEN_ADDR` | `127.0.0.1:8080` | 服务监听地址；`simulate.sh` 也用它拼目标地址 |
| `TM_ALARM_THRESHOLD_C` | `50.0` | 告警阈值（°C），读数严格大于它才告警；必须是有限数，`NaN`、`Inf`、`-Inf` 和非数字都使启动失败 [spec-review-amendment] |

- **来源优先级**：环境变量 > `config/local.env` > 缺省。
- **`config/local.env` 的文法**：Go 与 `db-reset.sh`、`simulate.sh` 用同一套；`test.sh` 不读这个文件。[spec-review-amendment]
  - 逐行读；去掉行尾 `\r`；空行和首个非空白字符为 `#` 的行忽略。[spec-review-amendment]
  - 其余行按第一个 `=` 切分；键和值各自去掉两端空白。[spec-review-amendment]
  - 没有 `=` 的行、键不在三个已知键里的行：报错，只给文件名和行号，不回显行内容。[spec-review-amendment]
  - 值为空等同未配置。环境变量为空串也等同未配置。[spec-review-amendment]
  - 值以 `"` 或 `'` 开头：报错，只给行号，提示去掉引号。[spec-review-amendment]
  - 值按字面使用，不做变量展开，不支持行内注释，不支持 `export` 前缀。[spec-review-amendment]
  - 脚本逐行按字面解析，不 `source`，文件内容不会被执行。[spec-review-amendment]
- **口令**：口令里有特殊字符时，在连接串里做 URL 编码。
- **连接池端口**：`config/example.env` 的注释写明必须连 session 池端口。
- **找配置文件的路径**：服务按当前工作目录找 `config/local.env`，所以从仓根启动。`internal/testdb` 从 `go.mod` 所在目录找同一份文件，因为 `go test` 的工作目录是包目录。
- **脚本工作目录**：三个脚本开头切到仓根（脚本所在目录的上一级），所以从任何目录调用结果都一样。[spec-review-amendment]

## Test Runner

`scripts/test.sh [unit|integration|all]`，缺省 `all`，其它参数打印用法并退出 2。

| 层 | 命令 | 包含 |
|---|---|---|
| unit | `go test -json -count=1 -race ./...` | 所有不带 build tag 的测试，其中契约测试跑在 memstore 上 |
| integration | `go test -json -count=1 -p 1 -tags integration -run '^TestIntegration' ./...` [spec-review-amendment] | 文件头为 `//go:build integration`、函数名以 `TestIntegration` 开头的测试 |

- `-run '^TestIntegration'` 防止带 tag 编译时把单元测试再跑一遍、重复计数。
- `-p 1` 让连真库层同一时刻只有一个测试包连库（见「Concurrency」的连接池）。[spec-review-amendment]
- `all` 依次跑两层。两层分别执行、分别计数，再求和得出汇总行。[spec-review-amendment]
- 两层都用 `-count=1` 禁用测试缓存，保证每次都真的执行。

**计数口径**（依据：go1.27.1 实测，见 decision-memo C1）：

- 只统计带 `Test` 字段、`Action` 为 `pass` / `fail` / `skip` 的事件，分别计入 N / M / K。子测试和父测试各算一个。
- 某个包出现包级 `fail` 事件，并且该包没有任何用例级 `fail` 时，M 加 1。这覆盖编译失败、vet 失败、`TestMain` 异常退出、超时。判定按「层 + 包」，同一个包在两层各自判定。[spec-review-amendment]
- 某层 `go test` 非零退出，而该层 M = 0 时，该层 M 加 1，stderr 打 `go test exited with status <n> but reported no failure`。这覆盖 `go test` 进程被杀。[spec-review-amendment]
- 包级 `skip` 不计入 K。没有测试文件的包会产生这种事件。
- T = N + M + K。

**输出与退出码**：

- `go test` 的 stderr 不进 jq，直接透传到脚本 stderr。[spec-review-amendment]
- 以下内容打到 stderr：[spec-review-amendment]
  - 每个失败用例的 `output` 事件，原样输出；
  - `build-output` 事件的 `Output`，它带 `ImportPath`，不带 `Package` 和 `Test`；[spec-review-amendment]
  - 贡献了包级 fail 的包，其包级 `output`。[spec-review-amendment]
- 最后一行打到 stdout：`N passed, M failed, K skipped (T total)`。
- 退出码：所选的每一层都要 N + M > 0。任一层为 0 时，stderr 打 `no tests executed in <层名> layer`，退出 1，汇总行照常输出。`<层名>` 取 `unit` 或 `integration`。否则退出 `min(M, 125)`，M 为各层之和。[spec-review-amendment]
- jq 解析失败时，stderr 报错，退出 1，不打汇总行。[spec-review-amendment]
- 缺 `go` 或 `jq` 时报错并退出 1，不打汇总行。
- 用法错误、缺工具、jq 解析失败时都不打汇总行。调用方用「stdout 最后一行是否是汇总行」区分这几种情况和正常结束。[spec-review-amendment]

## Demo Database Reset

`scripts/db-reset.sh` 不接受参数，有参数时打印用法并退出 2。

1. 环境变量 `TM_DATABASE_URL` 非空时直接用它，不读文件。否则按「Configuration Surface」的文法，从 `config/local.env` 读这一个键。两处都没有就报错退出 1。[spec-review-amendment]
2. 守卫：执行 `psql "$TM_DATABASE_URL" -XAtqc 'select current_database()'`，结果必须精确等于 `temp_monitor_demo`。
   - 不相等时打印实际库名，退出 3，什么都不改。
   - 连不上库时 psql 非零退出，脚本退出 4。[spec-review-amendment]
   - 守卫看的是实际连上的库，不靠解析 URL 字符串。
3. 重建：执行 `psql "$TM_DATABASE_URL" -X -v ON_ERROR_STOP=1 --single-transaction -c "SET lock_timeout = '5s'" -f db/drop.sql -f db/migrations/0001_init.sql … -f db/seed.sql`。迁移文件按文件名字典序展开。整个过程在一个事务里，失败就整体回滚。[spec-review-amendment]
   - 守卫和重建用同一个连接串，所以重建只会作用在守卫检查过的库上。[spec-review-amendment]
4. 脚本输出里不打印连接串，也不打印口令的片段。psql 的 stderr 先捕获，按「Security & Data Protection」脱敏后再输出。[spec-review-amendment]

退出码表 [spec-review-amendment]：

| 退出码 | 含义 |
|---|---|
| 0 | 成功 |
| 1 | 缺 `TM_DATABASE_URL`，或 `config/local.env` 有文法错误。`simulate.sh` 遇到文法错误同样退出 1 |
| 2 | 用法错误 |
| 3 | 库名不符 |
| 4 | psql 失败：连不上、SQL 出错、等锁超时 |

脚本自己捕获 psql 的退出状态，再映射为上表的值，不透传 psql 的退出码。[spec-review-amendment]

## Simulator

`scripts/simulate.sh [rounds]`，`rounds` 缺省 10。`rounds` 必须是正整数，否则打印用法并退出 2。脚本设 `LC_ALL=C`。[spec-review-amendment]

1. 目标地址是 `http://<TM_LISTEN_ADDR>`。`TM_LISTEN_ADDR` 的取值方式和服务一致：环境变量非空时直接用它，不读文件；否则按「Configuration Surface」的文法从 `config/local.env` 读这一个键；再没有就用 `127.0.0.1:8080`。这样脚本和服务不会因为配置写在不同地方而对不上。[spec-review-amendment]
   - `TM_LISTEN_ADDR` 以 `:` 开头时，目标主机用 `127.0.0.1`。[spec-review-amendment]
2. 注册 `sim-01`、`sim-02`、`sim-03`。返回 409 视为已注册，继续执行。
3. 每轮每台设备上报一条读数，轮间 `sleep 0.2`。
   - `sim-01`、`sim-02` 的读数是 18.0–35.0 的随机值，用 `$RANDOM` 生成，保留 1 位小数。
   - `sim-03` 的读数是 50.5–60.0 的随机值。在缺省阈值下，每轮至少产生一条告警。
4. 任一 curl 失败，或状态码不是预期值，就打印该请求和响应，然后退出 1。
5. 结束时不注销设备，方便到库里查看读数。

## Observability

- **日志**：用 `log/slog` 的 `TextHandler` 写到 stderr。
- **启动**：监听地址绑定成功后打一条 INFO，内容为监听地址和告警阈值，不含连接串。[spec-review-amendment]
- **告警**：打一条 WARN，msg 为 `over-temperature alarm`，字段为 `device_id`、`temperature_c`、`threshold_c`。
- **5xx**：打一条 ERROR，字段为路由、`device_id` 和脱敏后的底层错误。[spec-review-amendment]
- 不做 metrics、tracing、健康检查接口、访问日志。[spec-review-amendment]

## Error Messages [spec-review-amendment]

文案原文为英文，统一定义在这里。specs 和 tasks 引用时与此逐字一致。`<…>` 是运行时填入的值。文案里不出现连接串，也不出现口令。来自 pgx / psql 的底层错误先脱敏再拼入，做法见「Security & Data Protection」。

| 出处 | 触发条件 | 文案原文 | 下一步提示 |
|---|---|---|---|
| 服务启动 | 缺 `TM_DATABASE_URL` | `TM_DATABASE_URL is not set` | `copy config/example.env to config/local.env, fill it in, and start from the repo root` |
| 服务启动 | 阈值不是有限数 | `TM_ALARM_THRESHOLD_C is invalid` | `must be a finite number, such as 50.0` |
| 服务启动、`db-reset.sh`、`simulate.sh`、`internal/testdb` | `config/local.env` 某行没有 `=` | `config/local.env line <n>: missing "="` | `use KEY=VALUE` |
| 服务启动、`db-reset.sh`、`simulate.sh`、`internal/testdb` | `config/local.env` 某行的键不是三个已知键 | `config/local.env line <n>: unknown key` | `allowed keys: TM_DATABASE_URL, TM_LISTEN_ADDR, TM_ALARM_THRESHOLD_C` |
| 服务启动、`db-reset.sh`、`simulate.sh`、`internal/testdb` | `config/local.env` 某行的值以引号开头 | `config/local.env line <n>: value must not be quoted` | `remove the quotes` |
| 服务启动 | 5 秒内连不上库 | `cannot connect to database` | `check the SSH tunnel and that the URL uses the session pool port 7432` |
| 服务启动 | 缺 `devices` 或 `readings` 表 | `table devices or readings not found` | `run scripts/db-reset.sh first` |
| 服务启动 | 监听地址无法绑定 | `cannot listen on <地址>: <底层错误>` | `the port may be in use; free it or set TM_LISTEN_ADDR` |
| `db-reset.sh` | 带了参数 | `usage: scripts/db-reset.sh (takes no arguments)` | — |
| `db-reset.sh` | 缺 `TM_DATABASE_URL` | `TM_DATABASE_URL is not set` | `copy config/example.env to config/local.env and fill it in` |
| `db-reset.sh` | 库名不符 | `refusing to reset: only temp_monitor_demo is allowed, connected database is <实际库名>` | `check TM_DATABASE_URL` |
| `db-reset.sh` | 守卫查询失败（连不上库） | `cannot connect to database` | `check the SSH tunnel and that the URL uses the session pool port 7432` |
| `db-reset.sh` | 重建失败：SQL 出错、等锁超时 | `psql failed: <脱敏后的 psql stderr>` | `nothing was changed; check the SSH tunnel, then the SQL files under db/` |
| `test.sh` | 参数不是 `unit` / `integration` / `all` | `usage: scripts/test.sh [unit\|integration\|all]` | — |
| `test.sh` | 缺 `go` 或 `jq` | `required tool not found: <go 或 jq>` | `install it and make sure it is on PATH` |
| `test.sh` | 某个所选层 N + M = 0 | `no tests executed in <层名> layer` | `check the integration build tag and the TestIntegration name prefix` |
| `test.sh` | 某层 `go test` 非零退出而该层 M = 0 | `go test exited with status <n> but reported no failure` | `see the go test output above` |
| `test.sh` | jq 解析失败 | `cannot parse go test -json output` | `see the jq error above` |
| `simulate.sh` | `rounds` 不是正整数 | `usage: scripts/simulate.sh [rounds]  (rounds: positive integer, default 10)` | — |
| `simulate.sh` | curl 失败或状态码不是预期值 | `request failed: <方法> <URL> -> <状态码或 curl 错误>` | `start the service first, or check TM_LISTEN_ADDR` |
| `internal/testdb` | 缺 `TM_DATABASE_URL` | `TM_DATABASE_URL is not set` | `copy config/example.env to config/local.env and fill it in` |
| `internal/testdb` | 5 秒内连不上库 | `cannot connect to database` | `check the SSH tunnel and that the URL uses the session pool port 7432` |
| `internal/testdb` | 缺 `devices` 或 `readings` 表 | `table devices or readings not found` | `run scripts/db-reset.sh first` |
| HTTP 400 | 请求体不是合法 JSON、合法 JSON 后有尾随内容、请求体为空 | `invalid JSON body` | — |
| HTTP 400 | 请求体超过 1 MiB | `request body too large` | — |
| HTTP 400 | 注册请求缺 `id` 字段 | `id is required` | — |
| HTTP 400 | `id` 字段或路径中的 ID 不匹配正则 | `invalid device id: must match ^[a-z0-9-]{1,64}$` | — |
| HTTP 400 | 上报请求缺 `temperature_c` | `temperature_c is required` | — |
| HTTP 400 | `temperature_c` 不是 JSON 数字 | `temperature_c must be a number` | — |

- 启动失败、脚本失败时，文案原文和下一步提示写在同一行，用 `; ` 连接。
- HTTP 400 的响应体为 `{"error": "<文案原文>"}`，没有下一步提示。

## Failure Modes

| 场景 | 行为 | 可观测 |
|---|---|---|
| 缺 `TM_DATABASE_URL`，或阈值不是有限数 [spec-review-amendment] | 启动失败，退出 1 | stderr 说明缺哪个键 / 哪个值非法 |
| `config/local.env` 有不符合文法的行 [spec-review-amendment] | 启动失败，退出 1 | stderr 给出文件名和行号，不回显行内容 [spec-review-amendment] |
| 启动时连不上库（5 秒 ping 超时） | 启动失败，退出 1 | ERROR，不含连接串 |
| 启动时表不存在 | 启动失败，退出 1 | ERROR，提示先跑 `scripts/db-reset.sh` |
| 监听端口被占用 | 启动失败，退出 1 | ERROR |
| 运行中库断开 | 当次请求回 500；pgxpool 自动重连 | ERROR |
| 单次存储调用超过 5 秒 | context 取消，当次请求回 500。结果未知：语句可能已提交，不会半写。服务不自动重试 [spec-review-amendment] | ERROR |
| 客户端 5 秒内没发完请求头 | `ReadHeaderTimeout` 为 5 秒，断开该连接 [spec-review-amendment] | — |
| 同一 ID 并发注册 | 一个 201，其余 409，由主键约束保证 | — |
| 注销与上报并发 | 见上方并发序列图，两种结果都不留孤儿读数 | — |
| handler panic | net/http 对单个请求做 recover 并断开该连接，进程不退出；客户端收不到 JSON 响应体 [spec-review-amendment] | net/http 默认错误日志 |
| SIGINT / SIGTERM | `Server.Shutdown`（5 秒超时）后关闭连接池，退出 0。main 等 `Shutdown` 返回后才退出 [spec-review-amendment] | INFO |
| `Shutdown` 超过 5 秒 | 记 WARN，仍关闭连接池，退出 0 [spec-review-amendment] | WARN |
| 运行中 `Serve` 返回非预期错误 | 退出 1 [spec-review-amendment] | ERROR |
| 连真库测试时缺配置 / 连不上 / 表不存在 | 对应测试 `t.Fatal`，计入失败，不跳过 | 失败信息给出下一步该做什么 |
| `db-reset.sh` 指向了别的库 | 什么都不改，退出 3 | stderr 打印实际库名 |
| `db-reset.sh` 里 psql 失败：连不上、SQL 出错、等锁超过 5 秒 | 事务回滚，退出 4 [spec-review-amendment] | 连不上时 stderr 只有固定文案；其余打印脱敏后的 psql 错误 [spec-review-amendment] |

## Concurrency & Shared State（TG-26）

- **memstore**：用一把 `sync.Mutex` 同时保护 `devices` 和 `readings` 两张 map，每个方法整段持锁。
  - 依据：每个操作都会同时读写两张 map，读远少于写，没有用 `RWMutex` 的收益。
  - 用 `ListReadings` 返回副本，不把内部切片暴露给调用方。
- **pgstore**：不做「先查后写」，原子性交给数据库。
  - `CreateDevice` 直接 INSERT，靠主键约束（23505）判重复。
  - `AddReading` 直接 INSERT，靠外键约束（23503）判设备不存在。
  - 这样避免了查和写之间的竞态（GO-03 说的 TOCTOU）。
- **service / httpapi**：只持有构造后不变的字段（store、阈值、logger），没有共享可变状态。
- **goroutine**：除了 net/http 的请求 goroutine 和 main 的信号等待，不启动长期 goroutine。
- **连接池** [spec-review-amendment]：
  - 服务的 pgxpool 设 `MaxConns=4`。[spec-review-amendment]
  - `internal/testdb.Open` 建的池设 `MaxConns=4`，并 `t.Cleanup(pool.Close)`。[spec-review-amendment]
  - 连真库层用 `-p 1`，同一时刻只有一个测试包连库。[spec-review-amendment]
  - 依据：session 池每个客户端独占一个后端连接，池上限 20。各客户端池上限之和不得超过它。[spec-review-amendment]
- **验证**：
  - 单元层带 `-race` 跑。
  - 契约测试含「同一 ID 并发注册只有一个成功」。
  - 契约测试含「并发上报后读数条数正确」。
  - 契约测试含「并发注销与上报后没有残留读数，上报结果只可能是成功或不存在」。[spec-review-amendment]

## Security & Data Protection（TG-17）

- **信任边界**：服务没有鉴权，所以默认只监听 `127.0.0.1`。改成其它地址由使用者自己负责，README 写明这一点。
- **凭据**：
  - 连接串只存在于环境变量和被 gitignore 的 `config/local.env` 里。
  - 日志、HTTP 响应和脚本输出都不打印连接串，也不打印口令的任何片段。[spec-review-amendment]
  - 脱敏做法：服务和脚本输出任何来自 pgx / psql 的错误文本前，把连接串整体和其中的口令替换为 `***`。[spec-review-amendment]
  - 口令取连接串里 scheme 后第一个 `:` 到最后一个 `@` 之间的原始子串。[spec-review-amendment]
  - 口令提取不出来时，不输出底层错误原文，只输出固定文案（见「Error Messages」）。[spec-review-amendment]
  - 脚本对 psql 的 stderr 先捕获，过滤后再输出。[spec-review-amendment]
  - 建连阶段的错误不输出底层错误文本，只输出固定文案。口令写错格式时，驱动会把口令的一段当成主机名报出来，替换整段口令拦不住它。[spec-review-amendment]
  - 建连阶段指：服务启动时建池和 ping、`internal/testdb` 建连、`db-reset.sh` 的守卫查询。建连成功之后的错误才输出脱敏后的底层文本。[spec-review-amendment]
  - `config/example.env` 只放占位口令 `CHANGE_ME` 和主机 `127.0.0.1`。
- **gitignore**：`.gitignore` 预先挡住 `config/local.env`、`.pg-ops/`、`/.idea/`。
- **输入校验**：
  - 设备 ID 用正则限定格式，数据库里的 CHECK 约束再拦一道。
  - 请求体限制在 1 MiB。
  - SQL 一律用参数化查询。
- **历史审计**：打 tag 前对全部提交历史 grep 审计。审计用的名单由人在当时输入，不进仓库（见 tasks）。

## Decisions

决策、被砍候选与三镜代价见 `decision-memo.md`（D1–D15）。

### 切片建议 [spec-review-amendment]

六张垂直切片，每张可以单独验证：

| # | 切片 | 包含的 tasks | 独立验证方式 |
|---|---|---|---|
| 1 | 配置与存储契约 | 1.1–1.4、2.1–2.3 | 不经 `scripts/`，直接 `go test -race ./...` 通过 |
| 2 | 业务逻辑与 HTTP | 3.1、3.2 | `go test -race ./...` 通过（`httptest` 加 memstore） |
| 3 | PostgreSQL 实现、服务进程与连真库测试 | 2.4–2.7、3.3–3.5 | `go vet -tags integration ./...` 通过；按 3.5 手动验证启动失败路径；没有库时按 TST-06 失败并给出下一步 |
| 4 | 测试入口 | 4.2、4.3、6.1、6.2 | 按 4.3 在临时副本里逐条验证计数场景；6.1、6.2 的输出符合预期 |
| 5 | 重建脚本、模拟脚本、README | 4.1、4.4、5.1 | 不连库可验的退出码 1 / 2 / 4；服务未运行时 `simulate.sh` 退出 1 |
| 6 | 远端库验收 🔒 | 6.3–6.6 | 远端库就绪后按各条任务执行 |

票间依赖：1 是其余各张的前置；3 依赖 2（3.3、3.4 依赖 3.2）；4 的 6.2 依赖 3 的 2.6、2.7；5 的 4.1 依赖 3 的 2.4，4.4 依赖 3 的 3.3，5.1 里「测过 PostgreSQL 18」一句等 6 的 6.4 通过后确认；6 依赖 1–5。[spec-review-amendment]

## Risks / Trade-offs

- [连接串误指 transaction 池 6432，pgx 预编译语句报错] → `config/example.env` 注释写明用 7432；代码不做兼容。
- [远端库就绪前连真库层恒失败，暂时打不了 tag] → 这是约定要求的行为；proposal 的 Open Questions 记了负责人和截止时间。
- [新增表时忘了改 `db/drop.sql`] → 这时连跑两次 `db-reset.sh` 会因为建表冲突而失败，暴露遗漏。tasks 里有一条「连跑两次」的验证。
- [连真库测试与人在同一个演示库上操作，ID 撞车] → 测试 ID 用 `it-` 加随机后缀，测完自己清理。
- [`psql "$TM_DATABASE_URL"` 的命令行在本机 `ps` 里可见] → 接受。单人开发机，不处理。[spec-review-amendment]
- [测试进程被杀或超时后，可能残留 `it-` 设备] → 接受。靠 `scripts/db-reset.sh` 清。[spec-review-amendment]

## Migration Plan

- 新仓，没有存量部署。
- 部署顺序：
  1. 人用 `/pg-dev-init` 建库，`DB_NAME=temp_monitor_demo`。
  2. 起 session 池隧道。
  3. 复制 `config/example.env` 为 `config/local.env`，填入口令。
  4. 运行 `scripts/db-reset.sh`。
  5. 运行 `go run ./cmd/temp-monitor`。
- 回退：删掉表（`db/drop.sql`），或直接删分支。

## Compliance

- **公开仓约束**：遵守。凭据、真实主机地址、内网 IP、公司名、客户名都不进仓库，也不进提交历史。本仓提交作者用 GitHub noreply 邮箱，打 tag 前做全历史审计。`laodao-ai` 是 GitHub 组织名，不算公司名，可出现在 module 路径、README 和仓库地址里（设计门 Q2，人 2026-10-15 拍板）。
- **无鉴权、默认监听 `127.0.0.1`**：遵守。
- **「连不上库时失败，不跳过」**：遵守。连真库测试里没有 `t.Skip`。
- **「只接受固定演示库名」**：遵守。按实际连上的库名做守卫。
- **项目四块范围**：遵守。`ListReadings` 只在存储接口和测试里存在，不经 HTTP 暴露，不算新增功能。
- **显式豁免**：无。
