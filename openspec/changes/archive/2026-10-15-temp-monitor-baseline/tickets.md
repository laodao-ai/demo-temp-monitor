---
impl-pipeline: tickets
---

## Global Constraints

逐字摘自 `design.md`：

- 业务逻辑只依赖存储接口。单元测试用内存实现，不需要数据库；连真库测试和内存实现跑同一套契约测试。
- 两层测试由一个脚本统一执行和计数，计数口径不会把编译失败或零用例算成绿。
- 任何会把连接串打印出来的路径都不存在：日志、错误响应、脚本输出都不带连接串，也不带口令的片段。[spec-review-amendment]
- 不做限流和连接池调优。[spec-review-amendment]
- 不兼容 PgBouncer transaction 池（见 Risks）。
- 不做增量迁移；`db-reset.sh` 总是从零重建。
- 依赖方向只能向下：`httpapi → service → store`。`service` 不 import `pgstore` 或 `memstore`，具体实现只在 `main` 和测试里注入。
- 文案原文为英文，统一定义在这里。specs 和 tasks 引用时与此逐字一致。`<…>` 是运行时填入的值。文案里不出现连接串，也不出现口令。来自 pgx / psql 的底层错误先脱敏再拼入，做法见「Security & Data Protection」。
- 启动失败、脚本失败时，文案原文和下一步提示写在同一行，用 `; ` 连接。
- HTTP 400 的响应体为 `{"error": "<文案原文>"}`，没有下一步提示。
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
- **输入校验**：
  - 设备 ID 用正则限定格式，数据库里的 CHECK 约束再拦一道。
  - 请求体限制在 1 MiB。
  - SQL 一律用参数化查询。
- **连接池** [spec-review-amendment]：
  - 服务的 pgxpool 设 `MaxConns=4`。[spec-review-amendment]
  - `internal/testdb.Open` 建的池设 `MaxConns=4`，并 `t.Cleanup(pool.Close)`。[spec-review-amendment]
  - 连真库层用 `-p 1`，同一时刻只有一个测试包连库。[spec-review-amendment]
- **pgstore**：不做「先查后写」，原子性交给数据库。
- **goroutine**：除了 net/http 的请求 goroutine 和 main 的信号等待，不启动长期 goroutine。
- **脚本工作目录**：三个脚本开头切到仓根（脚本所在目录的上一级），所以从任何目录调用结果都一样。[spec-review-amendment]
- 脚本逐行按字面解析，不 `source`，文件内容不会被执行。[spec-review-amendment]

Compliance（逐字）：

- **公开仓约束**：遵守。凭据、真实主机地址、内网 IP、公司名、客户名都不进仓库，也不进提交历史。本仓提交作者用 GitHub noreply 邮箱，打 tag 前做全历史审计。`laodao-ai` 是 GitHub 组织名，不算公司名，可出现在 module 路径、README 和仓库地址里（设计门 Q2，人 2026-10-15 拍板）。
- **无鉴权、默认监听 `127.0.0.1`**：遵守。
- **「连不上库时失败，不跳过」**：遵守。连真库测试里没有 `t.Skip`。
- **「只接受固定演示库名」**：遵守。按实际连上的库名做守卫。
- **项目四块范围**：遵守。`ListReadings` 只在存储接口和测试里存在，不经 HTTP 暴露，不算新增功能。
- **显式豁免**：无。

票内「tasks x.y」指 `tasks.md` 的对应条目，验收以该条目原文的全部要求为准。

### Task 1: 配置与存储契约

**Blocked-by:** none
**R-ID:** RUN-01, RUN-02, RUN-03, RUN-05, DEV-01, DEV-02, DEV-03, DEV-04, ING-01, ING-02, TST-05, TST-07

建立 Go 模块骨架。服务的三个配置项能按「环境变量 > `config/local.env` > 缺省」取值，文法错误给出文件名和行号。仓里有示例配置，本机配置被 git 忽略。定义存储接口与哨兵错误，提供内存实现，并有一套两个实现共用的契约测试；内存实现在 `-race` 下通过它。不需要数据库，不经 `scripts/`，`go test -race ./...` 即可验证。

- [x] tasks 1.1：module `github.com/laodao-ai/demo-temp-monitor`、`go 1.27`、引入 pgx v5，`go build ./...` 通过
- [x] tasks 1.2：`.gitignore` 挡住 `config/local.env` 与 `.pg-ops/`（两行已在 `21d7b25` 加入，核对即可），建一个 `config/local.env` 后 `git status --porcelain` 不显示它
- [x] tasks 1.3：`config/example.env` 三个键为占位值，注释写明连 session 池端口 7432、口令做 URL 编码
- [x] tasks 1.4：配置解析与校验，单元测试覆盖该条列出的全部情形（三种优先级、文件不存在、注释与空行、两种错误、值含 `=` 与 `&`、空值、带引号、无 `=`、未知键、CRLF、阈值 `NaN` / `Inf` / `-Inf`）
- [x] tasks 2.1：存储接口、`Device`、`Reading`、`ErrNotFound`、`ErrAlreadyExists`
- [x] tasks 2.2：契约测试套件覆盖 TST-07 的九项行为，并发注册 20 个 goroutine，套件内部生成 `it-<随机后缀>` ID，容忍存储里已有别的数据
- [x] tasks 2.3：内存实现（一把 `sync.Mutex`，`ListReadings` 返回副本）跑契约套件，`go test -race ./...` 通过

### Task 2: 业务逻辑与 HTTP

**Blocked-by:** 1
**R-ID:** DEV-01, DEV-02, DEV-03, ING-01, ING-02, ING-03, ALM-01, ALM-02, ALM-03, ALM-04, RUN-04

注册、注销、上报三项业务可经 HTTP 调用。上报温度严格大于阈值且入库成功时写一条 WARN 告警。错误按 design「API」与「Error Messages」映射为 400 / 404 / 409 / 500，5xx 响应体固定，底层错误只进日志且已脱敏。用 `httptest` 加内存实现验证，不需要数据库。

- [x] tasks 3.1：ID 校验、注册 / 注销 / 上报、告警日志（msg `over-temperature alarm`，字段 `device_id` / `temperature_c` / `threshold_c`）；单元测试覆盖等于阈值不告警、未注册不告警、存储出错不告警、连续两条各告警一次
- [x] tasks 3.2：三个路由；请求体读取与 1 MiB 上限、尾随内容、空请求体、未知字段、`temperature_c` 区分缺失与 0；存储调用用请求 context 派生的 5 秒超时；400 文案逐字按 design「Error Messages」；单元测试覆盖 DEV-01..03 与 ING-01..03 的全部场景
- [x] `go test -race ./...` 通过

### Task 3: PostgreSQL 实现、服务进程与连真库测试

**Blocked-by:** 1, 2
**R-ID:** DB-04, DB-05, DEV-01, DEV-04, ING-02, RUN-02, RUN-03, RUN-04, RUN-06, TST-06, TST-07, TST-08, TST-09, ALM-01

服务可以作为进程启动：读配置、连 PostgreSQL、检查表存在、监听回环地址，收到 SIGINT / SIGTERM 时优雅退出。任一前置条件失败就退出 1，stderr 文案按 design「Error Messages」，不含连接串和口令片段。PostgreSQL 存储实现跑同一套契约测试；连真库测试缺配置、连不上、缺表时失败并给出下一步，不跳过。迁移、删表、种子 SQL 随仓提交。

- [x] tasks 2.4：迁移、删表、种子 SQL 按 design「Data Model & Lifecycle」，种子含 `demo-01`..`demo-03` 且至少一条读数 > 50.0
- [x] tasks 2.5：PostgreSQL 存储实现，23505 / 23503 映射为哨兵错误，提供表存在检查
- [x] tasks 2.6：连真库测试的公共准备（从 `go.mod` 定位仓根、读配置、5 秒建连、查表；失败一律 `t.Fatal` 并给出下一步；全文无 `t.Skip`；池 `MaxConns=4` 并在 Cleanup 关闭）
- [x] tasks 2.7：PostgreSQL 实现的连真库契约测试（`//go:build integration`，函数名 `TestIntegration` 开头），另测 DB-05 的检查约束与级联删除，`it-` 设备在 Cleanup 中删除
- [x] tasks 3.3：服务进程，含该条列出的全部要求（`MaxConns=4`、5 秒 ping、表检查、启动 INFO、`ReadHeaderTimeout`、`Shutdown` 5 秒与超时 WARN、`Serve` 非预期错误退出 1、错误文本脱敏）
- [x] tasks 3.4：端到端连真库测试（`httptest.Server` + HTTP 层 + PostgreSQL 实现 + 缓冲区 logger），按 TST-08 的五步验证，`it-` 前缀并清理
- [x] 服务进程用 Task 2 新增的脱敏 slog 包装（以实际连接串构造）包住全部日志输出，HTTP 层 5xx 日志因此脱敏；端到端连真库测试的缓冲区 logger 同样经该包装（执行期 fold 自 Task 2 Spec 轴复审：同 capability、高耦合、低增量）
- [x] `go vet -tags integration ./...` 通过
- [x] tasks 3.5：手动验证不需要数据库的启动失败路径（缺 URL、阈值 `abc`、无人监听端口加口令 `s3cret-xyz`，以及四个未编码特殊字符口令），每种都退出 1、stderr 不含口令及其片段，输出贴进本票报告

### Task 4: 测试入口

**Blocked-by:** 1, 3
**R-ID:** TST-01, TST-02, TST-03, TST-04, TST-05, TST-06

`scripts/test.sh [unit|integration|all]` 统一执行两层测试并给出汇总行 `N passed, M failed, K skipped (T total)`。计数口径不把编译失败、零用例、进程被杀算成绿，退出码跟随失败数。

- [x] tasks 4.2：按 design「Test Runner」实现该条列出的全部要求
- [x] tasks 4.3：在仓库的临时副本（不提交）里逐条验证该条列出的全部计数场景，每条场景的实测输出贴进本票报告
- [x] tasks 6.1：无数据库配置时 `scripts/test.sh unit` 退出 0、N > 0，汇总行贴进本票报告
- [x] tasks 6.2：`scripts/test.sh integration` 在「无数据库配置」与「连接串指向不可达端口」两种情况下，连真库层用例都计入 M、退出码非零；无配置时 K = 0 且失败信息提示复制 `config/example.env`；两段输出贴进本票报告。执行时临时移开 `config/local.env`（若存在）并在结束后放回，不改其内容

### Task 5: 重建脚本、模拟脚本、README

**Blocked-by:** 1, 3
**R-ID:** DB-01, DB-02, DB-03, ING-04, RUN-01, RUN-02, RUN-04, RUN-05, TST-01

`scripts/db-reset.sh` 只对实际连上的 `temp_monitor_demo` 库在单个事务里从零重建，退出码按 design「Demo Database Reset」。`scripts/simulate.sh [rounds]` 注册三台模拟设备并按轮上报，其中一台每轮超温。两个脚本读 `config/local.env` 时按同一套文法逐行解析，不执行文件内容。README 说明项目、前置工具、配置方法、启动顺序与测试用法。

- [x] tasks 4.1：重建脚本，含该条列出的全部要求，以及不连库可做的四项验证（不可达端口退出 4 且只有固定文案；两条 psql 都带同一连接串；四个特殊字符口令不泄漏；`$(touch marker)` 不产生 marker），输出贴进本票报告
- [x] tasks 4.4：模拟脚本，含该条列出的全部要求；验证服务未运行时退出 1、`rounds` 为 `0` 或 `abc` 时退出 2、`$(touch marker)` 不产生 marker，输出贴进本票报告
- [x] tasks 5.1：README 含该条列出的全部内容；「测过 PostgreSQL 18」这一句先不写，或写成待确认，由 Task 6 在 6.4 通过后补上

### Task 6: 远端库验收

**Blocked-by:** 1, 2, 3, 4, 5
**R-ID:** DB-01, DB-02, DB-03, DB-04, RUN-02, RUN-03, RUN-04, RUN-05, RUN-06, ING-04, ALM-01, TST-01, TST-06, TST-07, TST-08, TST-09

在远端开发库上完成连真库验收：重建脚本、全量测试、模拟上报与服务生命周期都按 spec 工作，然后审计全部提交历史。前置条件是 SSH 隧道已起、`config/local.env` 已填好；不满足时报 `BLOCKED`（等远端库），不勾任何一条（设计门 Q1 选 A）。`config/local.env` 含口令：MUST NOT 把它的内容打印到任何输出或报告里；工作目录不是主仓根时（如独立 worktree），只用 `cp` 从主仓根复制这个文件，用完删除副本。

- [x] tasks 6.3：该条列出的全部七项验证，输出贴进本票报告
- [x] tasks 6.4：`scripts/test.sh`（all）汇总 `N passed, 0 failed, K skipped (T total)`、N > 0、退出 0，之后库里没有 `it-` 开头的设备
- [x] tasks 5.1 补句：6.4 通过后，README 写明测过 PostgreSQL 18
- [x] tasks 6.5：该条列出的全部验证（`simulate.sh 3`、WARN 条数与不含口令、Ctrl-C 与 SIGTERM 退出 0、只监听 `127.0.0.1`、端口被占用时退出 1）
- [x] tasks 6.6：审计全部提交历史，IPv4、连接串与口令、邮箱三项按该条做法无命中；公司名和客户名一项需要人在执行时临时输入关键词，不写入任何文件——这一项报 `NEEDS_CONTEXT` 交编排层向人取关键词

### Task 7: 实现验证（收尾，不计入 3–6 预算）

**Blocked-by:** 1, 2, 3, 4, 5, 6
**R-ID:** all

按「聚合套件发现契约」运行本 change 命中域的全部层并全部通过。报告头以管道直贴（不手工转录）`suite-scope` 输出的 JSON（`base` / `head` / `domains_configured` / `hit_domains` / `layers` / `skipped_layers` / `mapping`）。证据落 `impl-reports/task7-verify.md`，每层一行 `<层> | <命令原文> | <退出码> | <SHA>`。

- [x] 命中域各层证据齐全并通过（或记未覆盖 / 不在命中域）
