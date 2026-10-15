## Why

temp-monitor 是一个温度监控教学示例，仓里目前只有 openspec 工作流骨架，没有任何业务代码。
后续的重现脚本（`repro/`）要以一个可运行、测试全过、打了 tag 的基线为起点，所以先搭这个基线。
这个仓以后会公开，基线从第一天起就不能带入凭据、真实主机地址、公司名或客户名。

## What Changes

- 新增 Go 服务 `cmd/temp-monitor`：设备注册与注销、温度上报入库、超温写日志告警，默认监听 `127.0.0.1:8080`，无鉴权。
- 新增存储层：一个存储接口，两个实现。内存假实现给单元测试用，PostgreSQL 实现（pgx v5）给连真库的测试用。
- 新增 `db/`：建表迁移、种子数据、删表 SQL，随仓提交。
- 新增 `scripts/db-reset.sh`：一条命令重建演示库，只接受库名 `temp_monitor_demo`。
- 新增 `scripts/simulate.sh`：用 bash + curl 模拟设备，直接调服务的上报接口。
- 新增 `scripts/test.sh`：统一测试入口，分单元层和连真库层，输出一行汇总，退出码跟随失败数。
- 新增配置读取：环境变量优先，其次是 gitignore 的 `config/local.env`。仓里只提交 `config/example.env`，内容全是占位值。
- `.gitignore` 新增 `/.idea/`、`config/local.env`、`.pg-ops/`。
- 新增 README：运行方式、前置工具、测过的 Go 与 PostgreSQL 主版本。

## Capabilities

### New Capabilities

- `device-registry`: 设备注册与注销，包括 ID 格式、重复注册、注销后级联删除读数。
- `reading-ingest`: 已注册设备上报温度读数并入库，以及模拟上报脚本。
- `over-temperature-alarm`: 读数超过全局阈值时写一条 WARN 日志。
- `service-runtime`: 服务配置的来源与优先级、缺省值、监听地址、启动时连库失败即退出。
- `demo-database`: 迁移与种子数据、`db-reset.sh` 的库名守卫与重建行为。
- `test-runner`: 统一测试入口、单元层与连真库层的划分、汇总行格式、退出码规则。

### Modified Capabilities

无（仓里还没有任何 spec）。

## Impact

- 技术栈命中 **TG-01（后端服务变更）**，下游选用领域清单 `backend` + `backend-go`。
- 新增依赖：`github.com/jackc/pgx/v5`（Go module 依赖）；运行与测试需要 Go 1.27、psql、jq、curl、bash、C 编译器（`-race` 需要）。[spec-review-amendment]
- 外部系统：远端开发库（PostgreSQL 18，经 PgBouncer session 池 + SSH 隧道访问），本 change 不配置它，只消费它。
- Go module 名：`github.com/laodao-ai/demo-temp-monitor`。

## Success Metrics

- 服务能启动并连上演示库；`scripts/simulate.sh` 跑完后，库里有模拟设备的读数，服务日志里至少有一条超温 WARN。
- `scripts/test.sh unit` 在不配置数据库的机器上全过，已执行用例数大于 0。
- 远端开发库就绪并执行一次 `scripts/db-reset.sh` 后，`scripts/test.sh`（缺省 all）输出 `N passed, 0 failed, K skipped (T total)`，N > 0，退出码 0。
- 打 tag 前的历史审计无命中：IP、带真实口令的连接串、公司名、客户名。
- 上述全部满足后，main 上打 tag `baseline`。

## Non-Goals

- `repro/` 下的三个重现脚本：基线打完 tag 后另做。
- 查询类 HTTP 接口（设备列表、读数查询）、MQTT 接入、真硬件接入、鉴权。
- 告警去重、滞回、按设备设置阈值、告警通知渠道（只写日志）。
- 配置远端开发库（建库、建角色、起隧道）：由人日后用 `/pg-dev-init` 完成。
- 把仓库改为 public。
- 迁移框架与增量迁移：重建总是从零开始。

## Compliance

- 公开仓约束：凭据、真实主机地址、内网 IP、公司名、客户名不进仓库，也不进提交历史。
  - 示例配置只用占位值，主机只写 `127.0.0.1`（隧道本地端）。
  - 提交作者邮箱用 GitHub noreply。
  - 打 tag 前做一次全历史审计。
- 服务无鉴权，所以缺省只监听 `127.0.0.1`。
- 不处理个人数据，不涉及第三方数据。

## Requirement Priorities

| 优先级 | 能力 | 理由 |
|---|---|---|
| P0 | `device-registry`、`reading-ingest`、`over-temperature-alarm` | 人指定的三块业务功能 |
| P0 | `test-runner` | 人指定的第四块，也是打 tag 的判据 |
| P0 | `demo-database`、`service-runtime` | 连真库测试与服务运行的前提 |
| P1 | README | 教学示例的入口说明，不影响测试判据 |

## Stakeholders & External Dependencies

| 对象 | 关系 | 是否知晓 / 需配合 | 权力级别 |
|---|---|---|---|
| 仓库维护者（人） | 拍板全部范围与公开时机 | 知晓 | 决策 |
| 远端开发库 | 上游依赖；连真库层和 tag 依赖它 | 需人用 `/pg-dev-init` 以 `DB_NAME=temp_monitor_demo` 配置 | 否决（不就绪即无法打 tag） |
| GitHub `laodao-ai/demo-temp-monitor` | 代码托管，当前 private | 已建 | 知情 |

## Open Questions

| 问题 | 负责人 | 截止 | 是否阻塞实现 |
|---|---|---|---|
| 远端开发库何时配置（`/pg-dev-init`，`DB_NAME=temp_monitor_demo`，起 session 池隧道） | 仓库维护者 | 打 tag `baseline` 之前 | 不阻塞编码和单元层；阻塞连真库层变绿和打 tag |

## Assumptions

| 假设 | 若不成立 |
|---|---|
| 远端库按 pg-dev-init 缺省建成：owner 角色与库同名，session 池端口 7432，经 SSH 隧道在本机 `127.0.0.1` 可达 | 只需改 `config/local.env` 里的连接串，代码不变 |
| owner 角色能在演示库里建表、删自己建的表 | `db-reset.sh` 失败；需要服务器侧给权限 |
| 本机有 Go 1.27、psql、jq、curl、bash、C 编译器 | 测试脚本或重建脚本启动即报缺工具 |

## Decisions

决策与被砍候选见 `decision-memo.md`。
