# Tasks

## 1. 配置与脱敏

- [ ] 1.1 `internal/config`：新增 `KeyWebhookURL`、`Config.AlarmWebhookURL`、`ErrWebhookInvalid`，并把新键加入 `knownKeys`。按 design D7 校验：http/https、Host 非空、报错不回显值。在 `config_test.go` 补用例，覆盖环境变量优先、文件读取、空值等同未配置，以及 `ftp://`、相对路径、空 Host 三种非法值（断言错误文本不含 `tok-s3cret`）。验证：`go test ./internal/config` 通过。
- [ ] 1.2 `scripts/lib/local_env.sh`：`case` 和 unknown key 的报错文案加入 `TM_ALARM_WEBHOOK_URL`，与 `knownKeys` 保持一致。验证：在临时目录写一个含 `TM_ALARM_WEBHOOK_URL=http://127.0.0.1:9090/hook` 的 `config/local.env`，用它运行 `scripts/simulate.sh`（服务未启动），失败原因应是连不上服务，而不是 unknown key。
- [ ] 1.3 `internal/redact`：`Handler` 增加可变参数 `extra ...string`。消息、字符串属性和错误值中出现的整串 extra 都替换为 `***`，原有调用不变。在 `redact_test.go` 补用例：错误值是带令牌 URL 的 `*url.Error`，日志中不含令牌；只传 connURL 时，行为与原来的用例一致。验证：`go test ./internal/redact` 通过。

## 2. 通知发送包

- [ ] 2.1 新建 `internal/notify`：`Event`、`New(url, logger, opts...)`、`Enqueue`、`Close(ctx) int`。实现 design D4 到 D6：容量 100、单个发送 goroutine、2 秒超时（可通过 option 调整）、不跟随重定向、响应体不写日志、关闭后 `Enqueue` 静默丢弃。验证：`go build ./...` 通过。
- [ ] 2.2 `internal/notify/notify_test.go` 用 `httptest` 接收端覆盖以下行为：
  - payload 字段与 `Content-Type`（NTF-06）；
  - 500 只收到一次请求，ERROR 含 `status=500`（NTF-08）；
  - 超时与连接失败分别记为 `reason=timeout` 和 `reason=connection failed`，日志不含 URL 中的令牌；
  - 3xx 不跟随，按失败处理；
  - 队列满时丢弃，并写 ERROR；
  - 多条事件按入队顺序到达，前一条结束后才发下一条（NTF-09）；
  - `Close` 在接收端正常时发完、返回 0；接收端卡住时在 ctx 到期后返回未发条数；
  - `Close` 之后调用 `Enqueue` 不 panic。

  验证：`go test -race ./internal/notify` 通过。

## 3. 跨线状态

- [ ] 3.1 `internal/service`：新增 `Notifier` 接口、`Option`、`WithNotifier`，`New` 改为可变参数。按 design D3 实现状态表、锁内入队，以及 Register / Deregister 成功后复位。未传 notifier 时不加锁、不建表。现有四处调用不改。验证：`go build ./...` 通过，`go test ./internal/service ./internal/httpapi` 现有用例全部通过。
- [ ] 3.2 `service_test.go` 用一个记录事件的假 Notifier 补用例：
  - 连续超温只有一条 `alarm`；
  - 等于阈值不触发；
  - 超温、回落、再超温依次触发 `alarm`、`recovered`、`alarm`；
  - 存储出错和未注册设备不改变状态；
  - 超温时注销再注册后，重新触发 `alarm`；
  - 20 个并发超温上报恰好一条 `alarm`；
  - 启用通知时 WARN 日志仍然逐条写（ALM-04 新场景）；
  - 未传 notifier 时不调用任何 Notifier 方法（NTF-01）。

  验证：`go test -race ./internal/service` 通过。

## 4. 组装与退出

- [ ] 4.1 `cmd/temp-monitor/main.go`：
  - 配置了 webhook 时创建 notifier，并通过 `WithNotifier` 传给 service；
  - 把 webhook URL 作为 extra 传给 `redact.Handler`；
  - `listening` 日志加 `alarm_notify=enabled|disabled`；
  - `configHint` 处理 `ErrWebhookInvalid`，allowed keys 加上新键；
  - 在等信号的 goroutine 里，`srv.Shutdown` / `srv.Close` 之后调用 `n.Close(shutCtx)`，`dropped > 0` 时写 WARN 并带上 `count`。

  验证：`go build ./...` 通过。
- [ ] 4.2 `main_test.go` 补用例（用假 open 函数和 `httptest` 接收端）：
  - 非法 webhook 三种取值都退出码为 1，stderr 不含令牌；
  - 启用时启动日志含 `alarm_notify=enabled`，不含令牌；
  - 接收端 3 秒才响应时，超温上报在 1 秒内返回 201（NTF-07）；
  - 接收端不可达时上报仍返回 201，stderr 不含 URL 的路径和查询串（RUN-04 新场景）；
  - SIGINT 时队列可发完、退出码为 0、没有 WARN；
  - 接收端卡住时在 5 秒内以退出码 0 结束，并有丢弃条数的 WARN（RUN-06 新场景）。

  验证：`go test -race ./cmd/temp-monitor` 通过。

## 5. 文档

- [ ] 5.1 `config/example.env` 加 `TM_ALARM_WEBHOOK_URL=` 及注释（留空即不通知；令牌只写进 `config/local.env`）。README 的「配置」一节改为四个配置项，表中加上新键，并说明三点：跨线规则、状态在重启后丢失、不重试。验证：README 中的配置键与 `internal/config` 的 `knownKeys` 一一对应；`git grep -n "TM_ALARM_WEBHOOK_URL=" -- config/example.env` 只有一行，且值为空。
- [ ] 5.2 `openspec/INDEX.md` 的能力表加入 `alarm-notification` 一行。验证：表中链接指向 `specs/alarm-notification/spec.md`。

## 6. 整体验证

- [ ] 6.1 运行 `scripts/test.sh unit`，在无数据库、无外网的条件下退出码为 0，汇总行 M 为 0（TST-05）。
- [ ] 6.2 有演示库时运行 `scripts/test.sh all`，退出码为 0。确认 TST-08 端到端用例不受影响（它不配置 webhook）。
- [ ] 6.3 手工冒烟：本机起一个打印请求的接收端（例如 `nc -lk 9090` 或一个小的 `httptest` 程序），配置 webhook 后启动服务，用 `scripts/simulate.sh` 上报 55、56、45。接收端应依次看到 `alarm` 和 `recovered` 两个请求，stderr 中不含 webhook 路径。

## Workflow follow-up

- 归档前改写 `openspec/specs/over-temperature-alarm/spec.md` 的 Purpose，去掉「不做……通知」，改为指向 `alarm-notification`。
- 由维护者决定是否修改 `openspec/config.yaml` 中「模块只有四个」「超温告警写日志」「不加表外功能」的项目约定。
- 评审通过后归档本 change，并确认归档结果。
