# Tasks

## 1. 配置与项目边界

- [ ] 1.1 修改 `openspec/config.yaml` 的 context：模块数从四个改为五个，第五个是「超温通知值班（单 webhook）」。核验：`openspec context --json` 能正常解析，context 里出现「五个」。
- [ ] 1.2 修改 `internal/config`：新增 `TM_NOTIFY_WEBHOOK_URL`、`TM_NOTIFY_HYSTERESIS_C` 两个键，`Config` 加对应字段，回差缺省 2.0，新增哨兵错误 `ErrHysteresisInvalid` 和 `ErrWebhookInvalid`，按 design D7 校验。在 `config_test.go` 补用例：两个新键的来源优先级；回差取 `-1`、`abc`、`NaN`、`Inf` 时返回 `ErrHysteresisInvalid`；URL 取 `not a url tok-s3cret`、`ftp://127.0.0.1/tok-s3cret`、`/tok-s3cret` 时返回 `ErrWebhookInvalid`，且错误文本不含 `tok-s3cret`；`config/local.env` 里写新键不报 unknown key。核验：`go test -race ./internal/config` 通过。
- [ ] 1.3 修改 `scripts/lib/local_env.sh`：已知键的 `case` 分支和 unknown key 报错文案都列出五个键。核验：用写有两个新键的临时 `config/local.env` 执行 `scripts/simulate.sh 1`，stderr 不含 `unknown key`；`config/local.env` 写一个未知键时，仍然报 unknown key 并列出五个键。
- [ ] 1.4 修改 `config/example.env`：加入 `TM_NOTIFY_HYSTERESIS_C=2.0`；加入一行注释掉的 `# TM_NOTIFY_WEBHOOK_URL=http://127.0.0.1:9090/notify`，并加注释说明 URL 视同凭据、不配置即不通知。修改 README 的配置表，加入这两个键。核验：把 `config/example.env` 复制为 `config/local.env` 后，`config.Load` 得到的 `NotifyWebhookURL` 为空（在 config 测试里加一条用例，读取 example.env）。

## 2. 通知投递 `internal/notify`

- [ ] 2.1 新建 `internal/notify`：定义 `Kind`（`alarm`、`resolved`）、`Event`、`Notifier` 接口，Event 的 JSON 编码按 NTF-09 的字段名，`at` 用 RFC3339Nano。核验：单元测试解码请求体，断言字段与数值和 NTF-09 场景一致。
- [ ] 2.2 实现 `Webhook`：容量 100 的有界 channel，单个 worker，每次 3 秒超时、只发一回，不跟随重定向，响应体限量读完再关闭（design D4、D5）。容量和超时可在包内测试里改小。用 `httptest` 补用例：先 alarm 后 resolved 的顺序，接收端第一条慢 1 秒时也不乱序（NTF-10）；返回 500 时只收到一次请求，并有 WARN `notification failed`（NTF-11）；超时；302 时不访问重定向目标；接收端卡住时，第 101 条被丢弃，并有 WARN `notification dropped`（NTF-12）；`Notify` 在队满时不阻塞。核验：`go test -race ./internal/notify` 通过。
- [ ] 2.3 实现 `Close(ctx)`：期限内把队列发完；期限到时取消在途请求，写一条 WARN `notifications dropped at shutdown`，字段 `count`；`Close` 之后调用 `Notify` 不 panic，按 dropped 处理。补用例：接收端卡住时调用 `Close`，在期限加少量余量内返回，`count` 大于 0；接收端正常时调用 `Close`，队列里的事件全部送达。核验：`go test -race ./internal/notify` 通过。
- [ ] 2.4 落实日志不含 URL（design D6）：日志只写 `target=host:port`，发送错误只记 `*url.Error` 里的底层错误，非 2xx 只记 `status`，并在唯一的错误记录处加注释说明原因。补用例：URL 为 `http://u-s3cret:p-s3cret@127.0.0.1:<无人监听端口>/hook/tok-s3cret?key=k3y-s3cret` 时，日志缓冲区不含这四个片段；接收端返回 403、路径含 `tok-s3cret` 时，日志有 `status=403`，且不含 `tok-s3cret`。核验：`go test -race ./internal/notify` 通过。
- [ ] 2.5 新增 `openspec/adr/0006-on-call-notify-best-effort.md`，按现有 ADR 的格式记录三件事：内存状态加单 worker 串行投递、不重试、不扩展 redact 而是从写法上不让 URL 进日志。备选方案取自 design D3、D4、D6。核验：文件存在，Status 为 Accepted，来源 change 写 `add-on-call-notify`。

## 3. 状态机 `internal/service`

- [ ] 3.1 修改 `service.New`：改为可变参数选项，新增 `WithNotifier(n, hysteresisC)`；在 `Report` 入库成功后，按 design D3 在锁内完成状态转换并调用 `Notify`；更新包注释和 `Service` 的注释，去掉「构造后无可变状态」的说法。在 `service_test.go` 用同步记录事件的假 Notifier，补 NTF-01..NTF-05 各场景的用例：未配置 notifier；首条超温；等于阈值；持续超温；回差区间内抖动；停在区间内；恢复；恰好等于 T−H；恢复后再超温；H=0；H=5。核验：`go test -race ./internal/service` 通过，现有的 ALM 用例没有改动且全部通过。
- [ ] 3.2 修改 `Register`、`Deregister`：成功后重置该设备的状态。补 NTF-06 用例（存储出错时上报高温和低温，状态都不变）和 NTF-07 用例（超温中注销再注册，期间不产生通知，之后超温产生 alarm）。核验：`go test -race ./internal/service` 通过。
- [ ] 3.3 补 NTF-08 用例：20 个 goroutine 对同一设备并发 `Report(55.0)`，假 Notifier 恰好收到一条 alarm。补 NTF-13 用例：同一个 store 换一个新的 Service 实例，模拟重启，再报超温时重新发 alarm。核验：`go test -race -count=20 -run 'Concurrent|Restart' ./internal/service` 全部通过。

## 4. 服务组装 `cmd/temp-monitor`

- [ ] 4.1 修改 main：配置了 URL 时创建 `notify.Webhook`，传入 `service.WithNotifier`；启动日志 `listening` 增加 `notify_enabled` 和 `hysteresis_c` 两个字段；`configHint` 为两个新哨兵错误各给一条提示，`ErrUnknownKey` 的提示列出五个键。在 `main_test.go` 补用例：回差非法、URL 非法时退出码为 1，stderr 指出键名且不含 `tok-s3cret`；URL 指向无人监听的端口时服务照常开始监听；启动日志含 `notify_enabled=true`，且不含 URL 的路径。核验：`go test -race ./cmd/temp-monitor` 通过。
- [ ] 4.2 修改停服流程：`srv.Shutdown` 之后，在同一个 `shutCtx` 内调用 `Close`（design D8）。在 `main_test.go` 补用例：通知启用、接收端卡住、队列里有待发通知时取消 ctx，`run` 在 6 秒内返回 0，stderr 含 `notifications dropped at shutdown`；同一用例里，stderr 不含 URL 中的 token。现有的 `TestGracefulShutdown` 不用改，照样通过。核验：`go test -race ./cmd/temp-monitor` 通过。
- [ ] 4.3 README 新增「超温通知」一节：怎么启用；payload 示例（`127.0.0.1` 占位）；按事件加滞回的语义；不重试；URL 视同凭据；已知限制（重启丢状态、队满与停服时丢弃、回差区间内保持超温中）。同步修改「范围」一节，删掉「只写日志」的说法。核验：README 里的命令和键名与 `config/example.env`、spec 一致，用 `grep -n TM_NOTIFY README.md config/example.env` 对照。

## 5. 端到端与整体核验

- [ ] 5.1 在 `internal/httpapi/e2e_integration_test.go` 增加一条用例：真实 PostgreSQL 加 `httptest` webhook，注册、上报 55.0、上报 40.0，调用 `Close` 后，接收端依次收到 `alarm`、`resolved`，`at` 与各自上报响应里的 `recorded_at` 相等。核验：`scripts/test.sh integration` 通过。
- [ ] 5.2 整体核验：`scripts/test.sh`（all）输出 `N passed, 0 failed, K skipped (T total)`，退出码为 0；`openspec validate add-on-call-notify --strict` 通过；`git grep -nE 'tok-|s3cret'` 只命中测试代码。

## Workflow follow-up

- 归档前，把 `openspec/specs/over-temperature-alarm/spec.md` 的 Purpose 改为「只写日志、不做去重；推送给值班的人见 `on-call-notify`」。
- 归档后，在 `openspec/INDEX.md` 的表里加一行 `on-call-notify`。
- 按项目的评审要求完成评审后，归档本 change，并核验归档结果。
