# Tasks

## 1. 配置

- [ ] 1.1 `internal/config` 加 `KeyWebhookURL = "TM_ALARM_WEBHOOK_URL"`、`Config.AlarmWebhookURL`、`ErrWebhookURLInvalid`（固定文本，只含键名），`knownKeys` 加入新键，校验按 design D6（http/https、Host 非空）。单元测试覆盖：未配置为空串、环境变量优于文件、合法 http 和 https、`ftp://`、无 scheme、无 host 三种非法值返回 `ErrWebhookURLInvalid`，且错误文本不含值 〔RUN-01, RUN-03, RUN-04〕
- [ ] 1.2 `cmd/temp-monitor` 的 `configHint` 为 `ErrWebhookURLInvalid` 给出提示，未知键提示列出四个键；`main_test` 加用例：`TM_ALARM_WEBHOOK_URL` 依次为 `ftp://127.0.0.1/tok-s3cret`、`127.0.0.1:9000/tok-s3cret`、`http:///tok-s3cret` 时退出 1，stderr 含键名，不含 `tok-s3cret` 〔RUN-03, RUN-04〕
- [ ] 1.3 `scripts/lib/local_env.sh` 的 `case` 和未知键提示加入 `TM_ALARM_WEBHOOK_URL`；验证：`config/local.env` 含该键时，`scripts/db-reset.sh` 和 `scripts/simulate.sh` 都不报未知键（可用无人监听的连接串和地址，只看报错文案） 〔RUN-01〕
- [ ] 1.4 `config/example.env` 加一行注释掉的 `# TM_ALARM_WEBHOOK_URL=http://127.0.0.1:9000/alarm`，并注释说明 URL 中的 token 是凭据，含 token 时应使用 https，不配置就不推送；README 的配置表加第四行，「范围」第 3 条补上可选的推送（用 `GLOSSARY.md` 的术语）；验证：`grep -n TM_ALARM_WEBHOOK_URL config/example.env` 只有以 `#` 开头的行 〔RUN-05〕

## 2. 推送包

- [ ] 2.1 新建 `internal/push`：`Alarm`、`Pusher` 接口（`Push(Alarm)`，非阻塞）、`Nop`；`go build ./...` 通过 〔ALM-05〕
- [ ] 2.2 实现 `push.Webhook`（design D1、D3、D4）：容量 100 的队列加 1 个 worker；每次请求超时可配置，缺省 5 秒；不跟随重定向；响应体读至多 64 KiB 后丢弃；失败日志 `alarm push failed` 只带 `device_id`、`reading_id`、`reason`；队列满时记 `alarm push dropped`（`reason=queue full`）。单元测试用 `httptest`（127.0.0.1）覆盖：200 时请求头与 JSON 字段正确（含 `summary`，以及 55.25 与 50.5 的小数写法）；503 只收到 1 次请求且 `reason="http status 503"`；302 不跟随，记 `reason="http status 302"`；不响应时 `reason=timeout`；端口关闭时 `reason="connection error"`；接收端阻塞时连发 150 条，至少 49 条 dropped；`Push` 在接收端阻塞时立即返回 〔ALM-05, ALM-06, ALM-07, ALM-08〕
- [ ] 2.3 实现 `Webhook.Close(ctx) int`（design D5）：停止接收，发送剩余队列，`ctx` 到期时中止在途请求，返回丢弃条数；`Close` 之后的 `Push` 计入丢弃数，不 panic。单元测试覆盖：发得完返回 0；接收端不响应时返回值等于"在途 1 条 + 队列剩余条数"；`Close` 之后的 `Push` 使返回值加 1。都在 `-race` 下运行 〔RUN-06〕
- [ ] 2.4 加一条泄露测试：URL 为 `http://127.0.0.1:<端口>/hook/tok-s3cret?key=k3y-s3cret`，在 500、不响应、端口关闭三种状态下各推一次，日志缓冲区（经过 `redact.Handler`）不含 `tok-s3cret`、`k3y-s3cret`、`/hook/` 〔RUN-04〕

## 3. 业务接入

- [ ] 3.1 `service.New` 增加 `push.Pusher` 参数（design D2）；`Report` 写完 WARN 后调用 `Push`，`Alarm` 字段取自入库返回的 `store.Reading`。更新 `service_test`、`httpapi_test`、e2e 三处调用；`service_test` 用记录型假实现断言：超温推送一次且字段正确；等于阈值、未注册、存储出错时不推送；连续两条推送两次。验证：`scripts/test.sh unit` 全部通过 〔ALM-05, ALM-02, ALM-03, ALM-04〕
- [ ] 3.2 `httpapi_test` 加用例：`Pusher` 的实现阻塞 10 秒时，上报仍在 1 秒内返回 201 〔ALM-06〕

## 4. 启动与退出

- [ ] 4.1 `cmd/temp-monitor`：配置了 URL 时构造 `push.Webhook`，否则传 `push.Nop{}`；启动 INFO `listening` 加字段 `alarm_push=true|false`；退出时在 `srv.Shutdown` 之后用同一个 `shutCtx` 调 `Close`，返回值大于 0 时记 WARN `alarm push dropped at shutdown`（字段 `count`），然后 `closeStore`。`main_test` 覆盖：未配置地址时不发出任何推送、不记 shutdown WARN；接收端不响应、有待发推送时发信号，`run` 在 6 秒内返回 0 并有该 WARN；接收端正常时没有该 WARN；启动日志的 `alarm_push` 在未配置和已配置时分别为 `false` 和 `true`，且不含地址中的 token 〔RUN-06, RUN-02〕

## 5. 集成验证

- [ ] 5.1 e2e 用例（TST-08）加一个监听 127.0.0.1 的 `httptest` 接收端：上报超温读数后，断言接收端收到恰好一次推送，`device_id`、`reading_id` 与上报响应一致；验证：`scripts/test.sh integration` 通过，设备清理不变 〔TST-08, TST-09, ALM-05〕
- [ ] 5.2 手动验证：用 `nc -l 127.0.0.1 9000` 起一个打印请求的本机接收端，配置 `TM_ALARM_WEBHOOK_URL=http://127.0.0.1:9000/alarm` 后运行 `scripts/simulate.sh`，确认收到含 `summary` 的 JSON，启动日志为 `alarm_push=true`；再停掉接收端，确认服务日志出现 `alarm push failed`，且全文不含 URL 路径。贴出输出 〔ALM-05, ALM-07, RUN-04〕
- [ ] 5.3 `scripts/test.sh all` 全部通过，贴出汇总行

## Workflow follow-up

- 归档时修改 `openspec/specs/over-temperature-alarm/spec.md` 的 Purpose，去掉「不做通知」；同步更新 `openspec/INDEX.md` 中该能力的说明。
- `openspec/config.yaml` 的 context 里「超温告警写日志」「不加表外功能」由维护者自行更新，本改动不修改它。
- 归档时把 `openspec/adr/0006-alarm-push-best-effort.md` 的状态从 Proposed 改为 Accepted。
