---
schema_version: 1
change: temp-monitor-baseline
branch: feat/temp-monitor-baseline
generated_at: 2026-10-03T20:55:55+08:00
decision_hash: e3796fd55b09
---

# 决策纪要 · temp-monitor-baseline

## 目标态

仓内有一个可运行的 Go 温度监控服务（设备注册/注销、上报入库、超温写日志告警）和统一测试入口，单元层与连真库层在远端开发库就绪后全过，随后打 tag `baseline`。

## 拍板决策

- **D1 范围四块，不加功能** — 依据：人 2026-10-03 启动指令；**砍掉的候选**：查询/列表 HTTP 接口、MQTT、真硬件、鉴权（人明确排除或未要求）。`repro/` 三个重现脚本不在本 change。
- **D2 远端开发库不在本 change 内配置** — 依据：人 2026-10-03「远端开发机后续再配置」；最后的全量变绿任务与 tag 依赖它，开发库就绪前连真库层按约定失败。**砍掉的候选**：本 change 内跑 `/pg-dev-init`。
- **D3 固定演示库名 `temp_monitor_demo`** — 依据：人 2026-10-03 同意推荐；名字直接表明这是可丢弃的演示库，日后 `/pg-dev-init` 的 `DB_NAME` 填同名。**砍掉的候选**：`temp_monitor`（pg-dev-init 按目录名推的缺省，看不出可丢弃）。
- **D4 测试脚本退出码 = 失败数 M，上限 125** — 依据：人 2026-10-03 同意推荐；超过 255 会回绕成 0（假绿），126 起被 shell 保留。**砍掉的候选**：有失败一律 exit 1。
- **D5 设备 ID 由客户端注册时给出，格式 `[a-z0-9-]{1,64}`** — 依据：人 2026-10-03 同意推荐；模拟脚本用固定 ID 最直接。**砍掉的候选**：服务端生成 ID（模拟脚本要回读保存 ID，多一步）。
- **D6 注销 = 硬删除，读数级联删；同 ID 可重新注册；未注册/已注销设备上报返回 404** — 依据：人 2026-10-03 同意推荐。**砍掉的候选**：软删除（要处理唯一约束和状态过滤，教学示例不需要）。
- **D7 全局告警阈值，配置读取，默认 50.0°C；读数严格大于阈值时，入库成功后打一条 WARN 日志，不去重** — 依据：人 2026-10-03 同意推荐。**砍掉的候选**：按设备阈值（注册接口多字段）；去重/滞回（超出「写日志即可」）。
- **D8 仓托管在 GitHub `laodao-ai/temp-monitor`，先 private；作者邮箱用 GitHub noreply** — 依据：人 2026-10-03 指示建到 laodao-ai 下，并确认 1–4 步与 private。已执行：本仓 `user.email=13449619+laodaocheney@users.noreply.github.com`，首个提交重写为 `6a0adbb`，已推 main。**砍掉的候选**：立即 public（审计前内容先公开）。
- **D9 Go module 名 `github.com/laodao-ai/temp-monitor`** — 依据：仓归属已定（D8），import 路径与 `go install` 可直接用。**砍掉的候选**：无组织前缀的 `temp-monitor`（原为避免泄露组织名，D8 后不再需要）。
- **D10 `.idea/` 进 `.gitignore`** — 依据：人 2026-10-03 同意；IDE 配置可能含本机路径。
- **D11 重建库 = 显式删本项目的表再跑迁移和种子，不删 schema、不删库** — 依据：owner 角色默认无 CREATEDB（C6），删库不可行；「库 owner 能删 public schema」在 PG 文档 ddl-schemas 页无正面陈述，且远端库未就绪无法实测（C5），删表只依赖「建表者即表 owner」。**砍掉的候选**：`DROP SCHEMA public CASCADE`（前提未验证）、`DROP OWNED BY CURRENT_USER`（会顺带收回服务器给该角色的授权）、`DROP DATABASE`（无权限）。

- **D12 已执行用例数为 0 时测试脚本退出 1** — 依据：人 2026-10-03 同意推荐；否则 build tag 写错、测试函数名拼错时 M=0 报绿，违反完成标准「已执行用例数 > 0」。汇总行照常输出，stderr 另打 `no tests executed`。这是 D4「退出码跟随失败数」的唯一例外。**砍掉的候选**：严格按字面，靠人看 T 判断。
- **D13 连真库层 = 两个存储实现共用的契约测试（跑在 pgstore 上）+ 一条 HTTP→PG 端到端测试；测试用 `it-` 加随机后缀的设备 ID 并自行清理；测试前不自动重建库，表不存在即失败并提示先跑 `scripts/db-reset.sh`** — 依据：人 2026-10-03 同意推荐。**砍掉的候选**：`test.sh integration` 先自动重建库（每跑一次测试就清空演示数据）。
- **D14 公开前历史审计 = tasks 里一条打 tag 前的人工步骤**：`git log -p --all` 加 grep 查 IP、带真实口令的连接串、公司名/客户名；名单由人在审计时临时输入，不进仓。改为 public 不在本 change。 — 依据：人 2026-10-03 同意推荐。**砍掉的候选**：提交一个审计脚本（名单不能进仓，脚本只能查一半）。
- **D15 主 session 自定的实现缺省（属日常判断，已在 2026-10-03 锚点纪要中逐项呈现给人）**：HTTP 用标准库 `net/http` 路由，三个接口 `POST /devices`、`DELETE /devices/{id}`、`POST /devices/{id}/readings`；驱动 pgx v5；读数时间由服务端取，单位 °C；存储接口含 `ListReadings`，只供契约测试，不经 HTTP 暴露；迁移为纯 SQL 文件，由 `scripts/db-reset.sh` 用 psql 顺序执行，无迁移框架；配置项 `TM_DATABASE_URL` / `TM_LISTEN_ADDR`（缺省 `127.0.0.1:8080`）/ `TM_ALARM_THRESHOLD_C`（缺省 `50.0`），环境变量优先于 gitignore 的 `config/local.env`，仓里只提交 `config/example.env`；`.pg-ops/` 预先进 `.gitignore`；单元层与连真库层用 build tag `integration` 分开；`scripts/test.sh [unit|integration|all]` 缺省 all，用 `go test -json` + jq 计数；模拟上报 `scripts/simulate.sh` 用 bash + curl；服务启动时连不上库立即退出 1；告警日志用 `log/slog` 写 stderr；README 写测过的 Go 与 PG 主版本。

## 承重约束

- **C1 `go test -json` 的计数口径** — 验证方式：go1.27.1 下在 scratchpad 建 ok/broken/notest 三包实测；**证据锚**：编译失败的包只产出一条包级 `{"Action":"fail","Package":"x/broken","FailedBuild":...}`，无任何用例事件；无测试文件的包产出包级 `{"Action":"skip"}`（无 `Test` 字段）；子测试与父测试各产出一条用例事件。⇒ 只数带 `Test` 字段的 pass/fail/skip；包级 fail 且该包无用例级 fail 时计 1 个失败；包级 skip 不计。
- **C2 本机工具链** — 验证方式：命令输出；**证据锚**：`go version go1.27.1 darwin/arm64`、`psql (PostgreSQL) 18.6`、`/usr/bin/jq`、`docker not found`。⇒ 连真库层只能用远端库，不能起本地容器。
- **C3 pgx v5 可拉取** — 验证方式：`go list -m -versions github.com/jackc/pgx/v5`（GOPROXY=goproxy.cn）；**证据锚**：最新 `v5.11.0`。
- **C4 提交历史只含 noreply 身份，远端为 private** — 验证方式：`git log --all --format='%ae %ce' | sort -u` 只输出 noreply 一行；`gh repo view` 输出 `{"visibility":"PRIVATE","branch":"main"}`。
- **C5 远端开发库当前未接入** — 验证方式：仓内无 `.pg-ops/`；**证据锚**：人 2026-10-03 确认「后续再配置」。
- **C6 远端库形态由 pg-dev-init / pg-dev-server 决定** — 验证方式：读 skill 文档；**证据锚**：`~/.claude/skills/pg-dev-init/SKILL.md:43`（owner 角色与库同名）、`:46`（CREATEDB 默认不给）；`~/.claude/skills/pg-dev-server/SKILL.md:183`（PgBouncer transaction 池 6432、session 池 7432，均只监听 127.0.0.1，经 SSH 隧道访问）。⇒ 示例连接串指向 `127.0.0.1:7432`（session 池），pgx 的预编译语句在 session 池下可用。

## 接受的边角

- **连接串误指 transaction 池（6432）时 pgx 预编译语句可能报错** — 概率低（示例配置写的是 7432）、影响小（报错明确，改端口即恢复）、完美成本中（代码里切 exec mode 要额外测两种池）；**为何接受**：示例配置注释写明用 session 池，代码不做兼容。
- **连真库层在远端库就绪前恒失败** — 概率 1（D2 的直接后果）、影响仅限「暂时打不了 tag」；**为何接受**：人明确的约定「连不上库时失败，不跳过」，这正是想要的行为。
- **契约测试在共享演示库上并发跑时与人工操作撞 ID** — 概率极低（`it-` 前缀加随机后缀）、影响为单次测试失败；**为何接受**：不为此加专用测试库。
- **`config/local.env` 里的口令含 shell 特殊字符时 bash `source` 出错** — 概率低、影响为脚本启动即报错；**为何接受**：示例配置注释要求连接串里的口令做 URL 编码。

> 术语旁注（项目无 `openspec/CONTEXT.md`，按规则记于此）：「单元测试 / 单元层」= 不带 build tag、只用内存假实现的 `go test`；「连真库的测试 / 连真库层」= 带 `integration` build tag、连 `TM_DATABASE_URL` 的测试；「读数」= 一条温度上报记录（reading）。

## 三镜代价

命中 TG-23 的三处选择（D11 重建方式、D12 零用例退出、D13 测试隔离）：

- **系统镜**：D11 删表清单要随迁移同步，多一处维护点，但不依赖未验证的权限前提；D13 不自动重建，测试与演示数据互不破坏。
- **用户镜**：D12 让「没跑到用例」显式变红；D13 首次跑连真库层前要手动跑一次 `db-reset.sh`，表不存在时有明确提示。
- **开发循环镜**：D11 新增表时要记得改删表 SQL（由 tasks 里的契约测试与 db-reset 重跑兜底发现）；D12 与 D4 并存，测试脚本多一个分支。
- **主次判定**：三处都以「不假绿、不依赖未验证前提」为主，维护成本为次。

B.7 回扫：ADR 三条件逐条过 D1–D15，无一同时满足「难逆转 + 缺上下文会意外 + 有真实权衡」（均为单文件可改回的实现选择），不提 ADR；术语见上方旁注；scope 内聚：四块共同构成一个可独立验证的基线，`.gitignore` 两项属于同一公开仓约束，无砍窄/加宽/混拼。
