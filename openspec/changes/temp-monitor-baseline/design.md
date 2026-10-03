## Context

动机见 proposal.md「Why」。下面只列影响做法的现状与约束：

- 仓里没有 Go 代码，从零开始。本机有 Go 1.27.1、psql 18.6、jq、Apple clang（`-race` 可用），没有 docker。
- 远端开发库按 `/pg-dev-init` 的约定建，当前还没建：
  - owner 角色和库同名，默认没有 CREATEDB；
  - PgBouncer 的 transaction 池在 6432，session 池在 7432；
  - 只监听服务器回环地址，本机经 SSH 隧道在 `127.0.0.1` 访问。
- 仓库托管在 GitHub `laodao-ai/temp-monitor`（private），以后公开。

## Goals / Non-Goals

**Goals:**

- 业务逻辑只依赖存储接口。单元测试用内存实现，不需要数据库；连真库测试和内存实现跑同一套契约测试。
- 两层测试由一个脚本统一执行和计数，计数口径不会把编译失败或零用例算成绿。
- 任何会把连接串打印出来的路径都不存在：日志、错误响应、脚本输出都不带连接串。

**Non-Goals:**

- 不做连接池调优、限流、请求超时以外的服务加固。
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
| `internal/store/storetest` | 契约测试套件，接收一个「新建空存储」的工厂函数 | `Run(t, newStore)` |
| `internal/service` | ID 格式校验、注册、注销、上报、阈值判断与告警日志 | `New(store, thresholdC, logger)` |
| `internal/httpapi` | 三个路由、JSON 编解码、错误到状态码的映射 | `NewHandler(svc, logger) http.Handler` |
| `internal/testdb` | 连真库测试的公共准备：从 `go.mod` 向上定位仓根、读配置、建连、检查表存在；任何一步失败都 `t.Fatal` | `Open(t) *pgxpool.Pool` |
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
| `db-reset.sh` 和 `simulate.sh` 里的 psql / curl | psql 用 `PGCONNECT_TIMEOUT=5` 限制建连；curl 用 `--max-time 5` | 脚本以非零退出 |
| 连真库测试建连 | 5 秒 | `t.Fatal` |

写操作的回滚路径：

- **服务**：每个写操作都是单条 `INSERT` 或 `DELETE`，由 PostgreSQL 保证原子性。超时或出错时语句整体不生效，不存在半写状态，不需要应用层回滚。
- **重建脚本**：`db-reset.sh` 用 `--single-transaction` 加 `ON_ERROR_STOP=1`，任一步失败时整体回滚（见 DB-03）。
- **连真库测试**：用 `t.Cleanup` 删除自己创建的 `it-` 设备，读数随之级联删除。

## API

三个路由，均用 Go 标准库 `http.ServeMux` 的方法 + 路径模式注册，路径参数用 `r.PathValue("id")` 取。

- 错误响应统一为 `{"error": "<说明>"}`。
- 5xx 只回 `{"error": "internal error"}`，详细错误只写服务端日志。
- 请求体用 `http.MaxBytesReader` 限制在 1 MiB。

**注册** `POST /devices`

```json
// 请求
{"id": "sensor-01"}
// 201
{"id": "sensor-01", "registered_at": "2026-10-03T12:00:00Z"}
// 400：JSON 无法解析 / 缺 id / id 不匹配 ^[a-z0-9-]{1,64}$
{"error": "invalid device id"}
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
// 400：JSON 无法解析 / 缺 temperature_c / temperature_c 不是数字 / id 格式非法
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
| `TM_ALARM_THRESHOLD_C` | `50.0` | 告警阈值（°C），读数严格大于它才告警；不是合法数字时启动失败 |

- **来源优先级**：环境变量 > `config/local.env` > 缺省。
- **`config/local.env` 的格式**：每行 `KEY=VALUE`，`#` 开头为注释，空行忽略，值不加引号。Go 和 bash `source` 都能读同一份文件。
- **口令**：口令里有特殊字符时，在连接串里做 URL 编码。
- **连接池端口**：`config/example.env` 的注释写明必须连 session 池端口。
- **找配置文件的路径**：服务按当前工作目录找 `config/local.env`，所以从仓根启动。`internal/testdb` 从 `go.mod` 所在目录找同一份文件，因为 `go test` 的工作目录是包目录。

## Test Runner

`scripts/test.sh [unit|integration|all]`，缺省 `all`，其它参数打印用法并退出 2。

| 层 | 命令 | 包含 |
|---|---|---|
| unit | `go test -json -count=1 -race ./...` | 所有不带 build tag 的测试，其中契约测试跑在 memstore 上 |
| integration | `go test -json -count=1 -tags integration -run '^TestIntegration' ./...` | 文件头为 `//go:build integration`、函数名以 `TestIntegration` 开头的测试 |

- `-run '^TestIntegration'` 防止带 tag 编译时把单元测试再跑一遍、重复计数。
- `all` 依次跑两层，事件流合并后一起计数。
- 两层都用 `-count=1` 禁用测试缓存，保证每次都真的执行。

**计数口径**（依据：go1.27.1 实测，见 decision-memo C1）：

- 只统计带 `Test` 字段、`Action` 为 `pass` / `fail` / `skip` 的事件，分别计入 N / M / K。子测试和父测试各算一个。
- 某个包出现包级 `fail` 事件，并且该包没有任何用例级 `fail` 时，M 加 1。这覆盖编译失败、vet 失败、`TestMain` 异常退出、超时。
- 包级 `skip` 不计入 K。没有测试文件的包会产生这种事件。
- T = N + M + K。

**输出与退出码**：

- 每个失败用例的 `output` 事件原样打到 stderr。
- 最后一行打到 stdout：`N passed, M failed, K skipped (T total)`。
- 退出码：N + M = 0 时，stderr 打 `no tests executed`，退出 1。否则退出 `min(M, 125)`。
- 缺 `go` 或 `jq` 时报错并退出 1，不打汇总行。

## Demo Database Reset

`scripts/db-reset.sh` 不接受参数，有参数时打印用法并退出 2。

1. 环境里没有 `TM_DATABASE_URL` 时，用 `set -a; . config/local.env; set +a` 读本地文件。两处都没有就报错退出 1。
2. 守卫：执行 `psql "$TM_DATABASE_URL" -XAtqc 'select current_database()'`，结果必须精确等于 `temp_monitor_demo`。
   - 不相等时打印实际库名，退出 3，什么都不改。
   - 连不上库时 psql 非零退出，脚本跟着退出。
   - 守卫看的是实际连上的库，不靠解析 URL 字符串。
3. 重建：执行 `psql -X -v ON_ERROR_STOP=1 --single-transaction -f db/drop.sql -f db/migrations/0001_init.sql … -f db/seed.sql`。迁移文件按文件名字典序展开。整个过程在一个事务里，失败就整体回滚。
4. 脚本输出里不打印连接串。

## Simulator

`scripts/simulate.sh [rounds]`，`rounds` 缺省 10。

1. 目标地址是 `http://<TM_LISTEN_ADDR>`。`TM_LISTEN_ADDR` 的取值方式和服务一致：先看环境变量，没有就读 `config/local.env`，再没有就用 `127.0.0.1:8080`。这样脚本和服务不会因为配置写在不同地方而对不上。
2. 注册 `sim-01`、`sim-02`、`sim-03`。返回 409 视为已注册，继续执行。
3. 每轮每台设备上报一条读数，轮间 `sleep 0.2`。
   - `sim-01`、`sim-02` 的读数是 18.0–35.0 的随机值，用 `$RANDOM` 生成，保留 1 位小数。
   - `sim-03` 的读数是 50.5–60.0 的随机值。在缺省阈值下，每轮至少产生一条告警。
4. 任一 curl 失败，或状态码不是预期值，就打印该请求和响应，然后退出 1。
5. 结束时不注销设备，方便到库里查看读数。

## Observability

- **日志**：用 `log/slog` 的 `TextHandler` 写到 stderr。
- **启动**：打一条 INFO，内容为监听地址和告警阈值，不含连接串。
- **告警**：打一条 WARN，msg 为 `over-temperature alarm`，字段为 `device_id`、`temperature_c`、`threshold_c`。
- **5xx**：打一条 ERROR，字段为路由、`device_id` 和底层错误。
- 不做 metrics，不做 tracing。

## Failure Modes

| 场景 | 行为 | 可观测 |
|---|---|---|
| 缺 `TM_DATABASE_URL`，或阈值不是数字 | 启动失败，退出 1 | stderr 说明缺哪个键 / 哪个值非法 |
| 启动时连不上库（5 秒 ping 超时） | 启动失败，退出 1 | ERROR，不含连接串 |
| 启动时表不存在 | 启动失败，退出 1 | ERROR，提示先跑 `scripts/db-reset.sh` |
| 监听端口被占用 | 启动失败，退出 1 | ERROR |
| 运行中库断开 | 当次请求回 500；pgxpool 自动重连 | ERROR |
| 单次存储调用超过 5 秒 | context 取消，当次请求回 500，语句不生效 | ERROR |
| 同一 ID 并发注册 | 一个 201，其余 409，由主键约束保证 | — |
| 注销与上报并发 | 见上方并发序列图，两种结果都不留孤儿读数 | — |
| handler panic | net/http 对单个请求做 recover 并断开该连接，进程不退出 | net/http 默认错误日志 |
| SIGINT / SIGTERM | `Server.Shutdown`（5 秒超时）后关闭连接池，退出 0 | INFO |
| 连真库测试时缺配置 / 连不上 / 表不存在 | 对应测试 `t.Fatal`，计入失败，不跳过 | 失败信息给出下一步该做什么 |
| `db-reset.sh` 指向了别的库 | 什么都不改，退出 3 | stderr 打印实际库名 |

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
- **验证**：
  - 单元层带 `-race` 跑。
  - 契约测试含「同一 ID 并发注册只有一个成功」。
  - 契约测试含「并发上报后读数条数正确」。

## Security & Data Protection（TG-17）

- **信任边界**：服务没有鉴权，所以默认只监听 `127.0.0.1`。改成其它地址由使用者自己负责，README 写明这一点。
- **凭据**：
  - 连接串只存在于环境变量和被 gitignore 的 `config/local.env` 里。
  - 日志、HTTP 响应和脚本输出都不打印连接串。
  - `config/example.env` 只放占位口令 `CHANGE_ME` 和主机 `127.0.0.1`。
- **gitignore**：`.gitignore` 预先挡住 `config/local.env`、`.pg-ops/`、`/.idea/`。
- **输入校验**：
  - 设备 ID 用正则限定格式，数据库里的 CHECK 约束再拦一道。
  - 请求体限制在 1 MiB。
  - SQL 一律用参数化查询。
- **历史审计**：打 tag 前对全部提交历史 grep 审计。审计用的名单由人在当时输入，不进仓库（见 tasks）。

## Decisions

决策、被砍候选与三镜代价见 `decision-memo.md`（D1–D15）。

## Risks / Trade-offs

- [连接串误指 transaction 池 6432，pgx 预编译语句报错] → `config/example.env` 注释写明用 7432；代码不做兼容。
- [远端库就绪前连真库层恒失败，暂时打不了 tag] → 这是约定要求的行为；proposal 的 Open Questions 记了负责人和截止时间。
- [新增表时忘了改 `db/drop.sql`] → 这时连跑两次 `db-reset.sh` 会因为建表冲突而失败，暴露遗漏。tasks 里有一条「连跑两次」的验证。
- [`config/local.env` 的值含 shell 特殊字符，`source` 出错] → 示例配置注释要求口令做 URL 编码。
- [连真库测试与人在同一个演示库上操作，ID 撞车] → 测试 ID 用 `it-` 加随机后缀，测完自己清理。

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

- **公开仓约束**：遵守。凭据、真实主机地址、内网 IP、公司名、客户名都不进仓库，也不进提交历史。本仓提交作者用 GitHub noreply 邮箱，打 tag 前做全历史审计。
- **无鉴权、默认监听 `127.0.0.1`**：遵守。
- **「连不上库时失败，不跳过」**：遵守。连真库测试里没有 `t.Skip`。
- **「只接受固定演示库名」**：遵守。按实际连上的库名做守卫。
- **项目四块范围**：遵守。`ListReadings` 只在存储接口和测试里存在，不经 HTTP 暴露，不算新增功能。
- **显式豁免**：无。
