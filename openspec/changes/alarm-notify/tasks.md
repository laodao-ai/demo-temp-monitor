# Tasks

## 1. 配置与脱敏

- [ ] 1.1 `internal/config`：新增 `KeyWebhookURL`、`Config.AlarmWebhookURL`、`ErrWebhookInvalid`，并把新键加入 `knownKeys`。按 design D7 校验：只接受 http/https，Host 不能为空，报错不回显值。在 `config_test.go` 补用例，覆盖：环境变量优先、从文件读取、空值等同未配置，以及 `ftp://`、相对路径、空 Host 三种非法值（断言错误文本不含 `tok-s3cret`）。验证：`go test ./internal/config` 通过。
- [ ] 1.2 `scripts/lib/local_env.sh`：`case` 分支和 unknown key 的报错文案都加入 `TM_ALARM_WEBHOOK_URL`，与 `knownKeys` 保持一致。验证：在临时目录写一个含 `TM_ALARM_WEBHOOK_URL=http://127.0.0.1:9090/hook` 的 `config/local.env`，在不启动服务的情况下运行 `scripts/simulate.sh`，失败原因应是连不上服务，而不是 unknown key。
- [ ] 1.3 `internal/redact`：`Handler` 增加可变参数 `extra ...string`。消息、字符串属性和错误值中出现的完整 extra 串，都替换为 `***`；原有调用方式不变。在 `redact_test.go` 补用例：错误值是一个带令牌 URL 的 `*url.Error` 时，日志中不含令牌；只传 connURL 时，行为与原有用例一致。验证：`go test ./internal/redact` 通过。

## 2. 告警通知发送包

- [ ] 2.1 新建 `internal/notify`，提供 `Event`、`New(url, logger, opts...)`、`Enqueue`、`Close(ctx) int`。按 design D4 到 D6 实现：
  - channel 容量 100，正在发送的那一条不占容量；
  - 单个发送 goroutine；
  - 2 秒超时，可通过 option 调整；
  - 不跟随重定向；
  - 响应体不写日志；
  - 关闭后 `Enqueue` 静默丢弃。

  验证：`go build ./...` 通过。
- [ ] 2.2 `internal/notify/notify_test.go` 用 `httptest` 起一个假接收方，覆盖以下行为：
  - 请求体字段与 `Content-Type` 符合 NTF-06；同一设备的 `alarm` 与 `recovered` 带相同的 `device_id`；
  - 接收方返回 500 时只收到一次请求，ERROR 含 `status=500`（NTF-08）；
  - 先发的 `alarm` 收到 500，后发的 `recovered` 照常发出并送达（NTF-08 场景「alarm 未送达……」中发送部分的行为）；
  - 超时记为 `reason=timeout`，连接失败记为 `reason=connection failed`，日志中都不含 URL 里的令牌；
  - 3xx 不跟随，按失败处理；
  - 排队边界：接收方阻塞、一条正在发送、排队中已有 100 条时，第 101 条排队请求被丢弃并写 ERROR；在此之前的 100 条都能入队；
  - 多条告警通知按入队顺序到达，前一条结束后才发下一条（NTF-09）；
  - 接收方正常时，`Close` 发完全部并返回 0；接收方卡住时，`Close` 在 ctx 到期后返回未发出的条数；
  - `Close` 之后调用 `Enqueue` 不 panic。

  验证：`go test -race ./internal/notify` 通过。

## 3. 告警状态

- [ ] 3.1 `internal/service`：新增 `Notifier` 接口、`Option`、`WithNotifier`，`New` 改为可变参数。按 design D3 实现：
  - 告警状态表 `alarming`；
  - 在锁内入队；
  - Register / Deregister 成功后复位。

  未传 notifier 时不加锁，也不建表。现有四处调用不改。验证：`go build ./...` 通过；`go test ./internal/service ./internal/httpapi` 中现有用例全部通过。
- [ ] 3.2 `service_test.go` 用一个记录告警通知的假 Notifier 补用例：
  - 连续三条超温读数，只产生一条 `alarm`（NTF-02）；
  - 读数等于阈值，不产生告警通知；
  - 依次上报超温、恢复、再超温，依次产生 `alarm`、`recovered`、`alarm`（NTF-03）；
  - 存储出错和未注册设备的上报，都不改变告警状态（NTF-04）；
  - 设备在告警中时注销、再注册，之后的超温读数重新产生 `alarm`，期间没有 `recovered`（NTF-05）；
  - 假 Notifier 即使"送达失败"，告警状态也照常切换：告警期内后续超温读数不再产生告警通知，读数回落时产生 `recovered`（NTF-08，service 侧）；
  - 20 个并发超温上报，只产生一条 `alarm`（NTF-09）；
  - 启用告警通知时，告警日志仍逐条写入（ALM-04 新场景）；
  - 未传 notifier 时，不调用任何 Notifier 方法（NTF-01）。

  验证：`go test -race ./internal/service` 通过。

## 4. 组装与退出

- [ ] 4.1 `cmd/temp-monitor/main.go`：
  - 配置了 webhook 时，创建 notifier，并通过 `WithNotifier` 传给 service；
  - 把 webhook URL 作为 extra 传给 `redact.Handler`；
  - `listening` 日志加上 `alarm_notify=enabled|disabled`；
  - `configHint` 处理 `ErrWebhookInvalid`，allowed keys 加上新键；
  - 在等信号的 goroutine 里，于 `srv.Shutdown` / `srv.Close` 之后调用 `n.Close(shutCtx)`，`dropped > 0` 时写 WARN，并带上 `count`。

  验证：`go build ./...` 通过。
- [ ] 4.2 `main_test.go` 补用例，使用假的 open 函数和 `httptest` 假接收方：
  - 三种非法 webhook 值都以退出码 1 结束，stderr 不含令牌；
  - 启用告警通知时，启动日志含 `alarm_notify=enabled`，不含令牌；
  - 接收方 3 秒才响应时，超温上报仍在 1 秒内返回 201（NTF-07）；
  - 接收方不可达时，上报仍返回 201，stderr 不含 URL 的路径和查询串（RUN-04 新场景）；
  - 接收方返回 403 且响应体含令牌时，stderr 只有状态码，不含令牌（RUN-04 新场景）；
  - 收到 SIGINT 时，已排队的告警通知全部发完，退出码为 0，没有 WARN；
  - 接收方卡住时，进程在 5 秒内以退出码 0 结束，并有一条带丢弃条数的 WARN（RUN-06 新场景）。

  验证：`go test -race ./cmd/temp-monitor` 通过。

## 5. 文档

- [ ] 5.1 `config/example.env` 加上 `TM_ALARM_WEBHOOK_URL=` 及注释，说明留空即不发告警通知、令牌只写进 `config/local.env`。README 的「配置」一节改为四个配置项，配置表加入新键，并按 `GLOSSARY.md` 的用语说明三点：跨线触发（一个告警期只发 `alarm` 和 `recovered` 各一条）、复位会造成的多发或漏发、不重试的后果。验证：
  - README 中的配置键与 `internal/config` 的 `knownKeys` 一一对应；
  - `git grep -n "TM_ALARM_WEBHOOK_URL=" -- config/example.env` 只有一行，且值为空；
  - `git grep -n "接收端" -- README.md` 没有输出。
- [ ] 5.2 `openspec/INDEX.md` 的能力表加入 `alarm-notification` 一行。验证：表中的链接指向 `specs/alarm-notification/spec.md`。

## 6. 整体验证

- [ ] 6.1 运行 `scripts/test.sh unit`。在没有数据库、没有外网的条件下，退出码为 0，汇总行中 M 为 0（TST-05）。
- [ ] 6.2 有演示库时运行 `scripts/test.sh all`，退出码为 0。确认 TST-08 端到端用例不受影响（它不配置 webhook）。
- [ ] 6.3 手工冒烟：在本机起一个小的 HTTP 程序作为假接收方，返回 200 并打印收到的请求体。配置 webhook 后启动服务，用 `scripts/simulate.sh` 依次上报 55、56、45。验证：
  - 假接收方依次收到 `alarm` 和 `recovered` 两个请求；
  - 服务的 stderr 中不含 webhook 的路径。

## Workflow follow-up

- 评审通过后归档本 change，并确认归档结果：`openspec/specs/alarm-notification/spec.md` 已生成，`over-temperature-alarm` 中 ALM-04 的标题已更新为新名称。
- 注：`over-temperature-alarm` 主 spec 的 Purpose 与 `openspec/config.yaml` 的项目约定已在规划阶段改好，归档时无需再改。
