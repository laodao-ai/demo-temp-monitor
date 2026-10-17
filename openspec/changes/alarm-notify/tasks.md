# Tasks

## 1. 配置：第四个键

- [ ] 1.1 在 `internal/config` 加 `KeyNotifyWebhookURL`、`Config.NotifyWebhookURL`、`ErrWebhookURLInvalid`，按 design D6 校验 scheme 与 host；补 `config_test.go`：env 优先于文件、空值即未配置、`ftp://hook-s3cret` 与 `hook-s3cret` 返回哨兵错误且错误文本不含 `hook-s3cret`；验证 `go test ./internal/config` 通过
- [ ] 1.2 在 `scripts/lib/local_env.sh` 的白名单和 unknown key 提示里加上 `TM_NOTIFY_WEBHOOK_URL`；验证 `config/local.env` 含该键时 `scripts/simulate.sh` 和 `scripts/db-reset.sh` 不报未知键（RUN-01 新场景）
- [ ] 1.3 更新 `config/example.env`：加空值的 `TM_NOTIFY_WEBHOOK_URL=`，注释里给出 `http://127.0.0.1:9000/hook/CHANGE_ME` 示例，并说明 URL 视同秘密；验证文件只含占位值（RUN-05）

## 2. 通知包 `internal/notify`

- [ ] 2.1 新建 `internal/notify`：`Notifier` 接口、`Event`、`*Error{Reason, StatusCode}`，`Error()` 为固定文本、不含 URL（design D1）；验证 `go vet ./internal/notify` 通过
- [ ] 2.2 实现 webhook：`POST` JSON（NTF-05 的六个字段，`recorded_at` 用 RFC3339Nano）、`Content-Type: application/json`、不跟随重定向、2xx 成功，失败分类为 `timeout` / `transport` / `status`，读掉并丢弃响应体；用 `httptest.Server` 测报文字段、2xx、500、302（重定向目标未被请求）、关闭的端口、不响应时超时，并用 path、query 含独特 token 的 URL 断言每个错误文本不含该 URL 及这些 token（RUN-04）；验证 `go test -race ./internal/notify` 通过
- [ ] 2.3 实现 `internal/notify/memnotify`：记录收到的 Event，可注入错误，可在调用里阻塞直到测试放行；补自身的简单测试；验证 `go test -race ./internal/notify/...` 通过

## 3. Service：通知状态

- [ ] 3.1 给 `service.New` 加可变参数 `opts ...Option` 与 `WithNotifier`，未传时不通知、不维护状态（design D5），更新包注释；验证现有 `service_test.go`、`httpapi_test.go`、`main_test.go` 不改动即可编译通过
- [ ] 3.2 在 `Report` 中按 design D2 实现通知状态的「占位（已通知）-> 发送 -> 失败按 gen 撤回（未通知）」，Event 取自 `AddReading` 返回的读数，用 `WithoutCancel` 加 2 秒超时（D4），失败写 ERROR `over-temperature notify failed`，含 `device_id`、`reason`，`status` 时含 `status_code`，不记 error 值；用 memnotify 测 NTF-01、NTF-02、NTF-03（含 50.1/49.9/50.1）、NTF-04（存储出错、未注册）、NTF-05（Event 的 `ReadingID`、`RecordedAt` 等于返回的读数）、NTF-06（失败后重试、ERROR 字段）、NTF-07（并发只通知一次；发送中的通知失败时不补发，下一条超温读数再触发）、NTF-09，以及 ALM-04 新场景；验证 `go test -race ./internal/service` 通过
- [ ] 3.3 用阻塞型 memnotify 测 D2 的撤回错位：A 占位阻塞 -> ≤ 阈值读数 -> B 占位成功 -> A 失败，之后再来一条超温读数不再通知；再测请求 ctx 被取消时通知不被取消；验证 `go test -race ./internal/service` 通过
- [ ] 3.4 `Register`、`Deregister` 成功后清除该设备的通知状态（design D3）；测 NTF-08（注销后重新注册再次通知；注销时通知正在发送，用阻塞型 memnotify 确认这次通知照常完成），以及注销失败时通知状态不变；验证 `go test -race ./internal/service` 通过

## 4. 脱敏与组装

- [ ] 4.1 `redact.Handler` 加可变参数 `secrets ...string`，按整串替换消息、字符串属性、分组成员和 error 值（design D7）；补 `redact_test.go`：额外秘密被替换、`over-temperature alarm` 等正常文本不被误伤、不传额外秘密时行为不变；验证 `go test ./internal/redact` 通过
- [ ] 4.2 `cmd/temp-monitor/main.go`：把 webhook URL 作为额外秘密传给 `redact.Handler`；配置了 URL 时构造 webhook notifier 并传 `WithNotifier`；`listening` 日志加 `notify=on|off`；`configHint` 处理 `ErrWebhookURLInvalid`，`ErrUnknownKey` 提示列出四个键；补 `main_test.go`：RUN-02 三个场景（含启动时 webhook 收不到请求、webhook 不可达也能启动）、RUN-03 非法 URL 退出码 1 且 stderr 不含值、RUN-04 新场景（500 / 不可连接 / 超时三种情况下 stderr 不含 `tok-s3cret`、`k-s3cret`）；验证 `go test -race ./cmd/temp-monitor` 通过

## 5. 端到端与文档

- [ ] 5.1 在 `internal/httpapi/e2e_integration_test.go` 起本机 `httptest.Server` 作为通知接收端，按 TST-08 新增第 5 步断言恰好收到一次该设备的通知，且 `reading_id` 等于上报响应的 `id`；验证 `scripts/test.sh integration` 通过
- [ ] 5.2 新增 `openspec/adr/0006-sync-notify-in-memory-state.md`，记录同步发送、内存中的通知状态、失败撤回、无滞回与只支持单实例的决定和否决方案（design D2、D4）；验证格式与现有 ADR 一致
- [ ] 5.3 更新 `README.md`：范围由四块改为五块，配置表加 `TM_NOTIFY_WEBHOOK_URL`；说明报文格式（六个字段）、每个超温段通知一次、阈值附近来回跳与重启会重复通知、群机器人需在接收端适配、非本机接收端用 `https`、只支持单实例；演示一节写明服务不重启时第二次运行 `simulate.sh` 收不到通知，要再看到就重启服务或先上报一条低温读数；术语与 `GLOSSARY.md` 一致；验证 README 中的配置项与 `config/example.env` 一致
- [ ] 5.4 更新 `openspec/config.yaml` 的 context，把模块描述由四个改为五个（加上超温通知 webhook）；`openspec/INDEX.md` 加 `over-temperature-notify` 一行，并链接仓根 `GLOSSARY.md`；验证 `openspec validate alarm-notify --strict` 仍通过

## 6. 集成检查

- [ ] 6.1 执行 `scripts/test.sh unit`，确认在无数据库配置、无外网时退出码为 0（TST-05）
- [ ] 6.2 执行 `scripts/test.sh all`，确认汇总行无失败；`git grep` 确认仓内没有真实 webhook 地址或 token

## Workflow follow-up

- 归档后，手工把 `openspec/specs/over-temperature-alarm/spec.md` 的 Purpose 从「不做去重、通知」改为指向 `over-temperature-notify`，因为增量 spec 不能修改 Purpose。
- 按项目评审要求完成后归档本 change，并确认归档结果。
