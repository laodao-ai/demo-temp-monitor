# Design

## Context

动机见 proposal.md「Why」，行为契约见 `specs/on-call-notify/spec.md` 和 `specs/service-runtime/spec.md`。下面只列影响做法的现状：

- `internal/service.Service` 构造后没有可变状态。`Report` 先调用 `store.AddReading`，成功后才判断阈值、写 WARN（`internal/service/service.go`）。
- `service.New(s, thresholdC, logger)` 有四个调用点：`cmd/temp-monitor/main.go`、`internal/httpapi/httpapi_test.go`、`internal/service/service_test.go`、`internal/httpapi/e2e_integration_test.go`。
- httpapi 用 `storeTimeout`（5 秒）限制每次存储调用。上报响应在 `Service.Report` 返回后才写出。
- 脱敏：`redact.Handler(inner, connURL)` 只认数据库连接串这一条秘密，签名由 ADR 0005 固定。
- 配置：`internal/config` 用 `knownKeys` 校验 `config/local.env` 的键。`scripts/lib/local_env.sh` 在第 51 行另有一份已知键列表，两边要保持一致（RUN-01）。
- 停服：main 用一个 5 秒的 `shutCtx` 调 `srv.Shutdown`，超时就 `srv.Close`，随后 defer 关连接池（RUN-06）。
- 测试：`cmd/temp-monitor/main_test.go` 通过注入 `openFunc` 和 `listen` 测 `run`，不连库。单元层开 `-race`。

## Goals / Non-Goals

**Goals:**

- 现有的四个 `service.New` 调用点和 ALM 测试不用改，照样通过。
- 状态转换和入队在同一个临界区里完成，从结构上保证 NTF-08（并发只通知一次）和 NTF-10（顺序）。
- 入队永不阻塞上报请求。
- webhook URL 在任何代码路径上都不会进入日志。靠写法保证，不靠事后替换。

**Non-Goals:**

- 不抽象多种渠道，也不做插件机制。`Notifier` 接口只为测试替换而存在。
- 不改 `redact` 包，不改 ADR 0005。
- 不加 metrics，不做通知历史查询。

## Design Diagrams

### 组件与依赖

```
 cmd/temp-monitor (main)
   |
   +--> internal/config ---------- 新增 TM_NOTIFY_WEBHOOK_URL, TM_NOTIFY_HYSTERESIS_C
   +--> internal/notify ---------- Event / Kind / Notifier 接口 / Webhook（队列 + worker）
   |          ^                               |
   +--> internal/service ---------------------+   (service 只依赖 notify.Notifier 接口)
   |          |                               |
   |          +--> internal/store             +--> net/http --> 使用者提供的 webhook
   +--> internal/httpapi --> internal/service
```

### 一次上报的时序

```
 httpapi         Service.Report             store         notify.Webhook          worker
   |  Report(id,t)    |                       |                 |                    |
   |----------------->|  AddReading           |                 |                    |
   |                  |---------------------->|                 |                    |
   |                  |<--------- r ----------|                 |                    |
   |                  |  t > T ? logger.Warn  (ALM-01, 原样)     |                    |
   |                  |  lock mu                                |                    |
   |                  |    transition(state[id], t)             |                    |
   |                  |    if changed: Notify(ev) -- 非阻塞 --> | chan <- ev         |
   |                  |  unlock mu                              |  (满: WARN dropped)|
   |<------ r --------|                                         |                    |
   |  201                                                       |        <- ev ------|
   |                                                            |   POST (3s 超时)   |
   |                                                            |   失败: WARN failed|
```

### 停服顺序

```
 SIGTERM -> shutCtx(5s) -> srv.Shutdown(shutCtx) --> [超时则 srv.Close]
                       -> notifier.Close(shutCtx)  -- 同一期限内继续发队列
                             期限到: 取消在途请求, 剩余计数 -> WARN dropped at shutdown
                       -> closeStore()  (defer, 原样)
```

## Decisions

### D1 新包 `internal/notify`，service 只依赖接口

- `notify.Kind`：取值 `alarm` 和 `resolved`。
- `notify.Event`：`Kind`、`DeviceID`、`TemperatureC`、`ThresholdC`、`HysteresisC`、`At`。`At` 取 `store.Reading.RecordedAt`。
- `notify.Notifier`：只有一个方法 `Notify(Event)`，约定不阻塞、不返回错误。
- `notify.Webhook` 实现这个接口，另外提供 `Close(ctx)`。

备选方案是把 webhook 发送直接写进 service。否决原因：service 的单元测试会被迫起 HTTP 服务；而且 service 包注释写明它只依赖 `store.Store`，接口化之后这条约定可以扩展成「只依赖 store 与 notify 两个接口」。

### D2 `service.New` 加可变参数选项

签名改为 `New(s store.Store, thresholdC float64, logger *slog.Logger, opts ...Option)`，新增 `WithNotifier(n notify.Notifier, hysteresisC float64) Option`。

- 不传选项时，`notifier` 为 nil，`Report`、`Register`、`Deregister` 都跳过状态逻辑，满足 NTF-01，四个现有调用点不用改。
- 备选一：新增构造函数 `NewWithNotifier`。会出现两条构造路径，以后再加选项就要继续加函数。
- 备选二：直接加两个位置参数。要改四个调用点，没有收益。

### D3 状态是 `map[string]bool` 加一把 `sync.Mutex`，入队在锁内

`Service` 新增 `mu sync.Mutex` 和 `over map[string]bool`。`Report` 在 `AddReading` 成功后加锁，然后：

- `!over[id] && t > T`：置为 true，调用 `Notify(alarm)`。
- `over[id] && t <= T-H`：删除该键，调用 `Notify(resolved)`。
- 其它情况：不做任何事。

`Register` 和 `Deregister` 成功后加锁，删除该键（NTF-07）。

入队发生在锁内，所以队列里的顺序就是状态转换的顺序（NTF-10）。同一设备的并发超温只有第一个拿到锁的请求会触发转换（NTF-08）。`Notify` 是非阻塞的 channel 发送，持锁时间只有几次 map 操作加一次 select。

备选方案是 `sync.Map` 加按设备的 CAS。它能保证只通知一次，但 CAS 成功和入队之间有间隙：两个设备、甚至同一设备的 alarm 和 resolved，入队顺序都可能和转换顺序相反。全局只有一把锁，在这个示例的并发量下不构成瓶颈。

不变量：Service 说明「构造后无可变状态」的那段包注释要同步改掉。

### D4 投递：有界 channel 加单个 worker

- `notify.New(url, logger)` 创建 `Webhook`：cap 为 100 的 channel，启动一个 worker goroutine。worker 的 base context 由 Webhook 持有，可以取消。
- `Notify`：先加锁检查 `closed`，再 `select { case ch <- ev: default: WARN "notification dropped" }`（NTF-12）。`closed` 之后的 `Notify` 同样按 dropped 处理。用锁加标志位，是为了避免向已关闭的 channel 发送而 panic。
- worker：逐条取出事件，用 `context.WithTimeout(base, 3s)` 发一次 `POST`，不重试（NTF-11）。
- `Close(ctx)`：
  1. 置 `closed`，关闭 channel，等 worker 把剩余事件发完。
  2. `ctx` 到期就取消 base context，在途请求立即失败。worker 统计还没发的事件数，写一条 `notifications dropped at shutdown`，字段 `count`（RUN-06）。
  3. `Close` 在 worker 退出后才返回。

超时（3 秒）和队列容量（100）是包内常量。测试在包内（`package notify`）通过未导出的构造参数改成毫秒级，写法与 main 里可替换的 `listen` 变量一致。

备选方案：每条事件起一个 goroutine。否决，原因是顺序无法保证（explore 阶段已讨论）。

### D5 HTTP 客户端的细节

- `http.Client` 设 `CheckRedirect: func(...) error { return http.ErrUseLastResponse }`，所以 3xx 原样返回，按失败处理（NTF-11）。
- 请求体用 `encoding/json` 编码 Event，字段名按 NTF-09。`at` 用 `time.RFC3339Nano`，与 httpapi 的 `recorded_at` 格式一致。
- 响应体用 `io.Copy(io.Discard, io.LimitReader(body, 64<<10))` 读完再关闭，便于连接复用，也不会被超大响应拖住。

### D6 webhook URL 不进日志：靠写法保证，不扩展 redact

- `Webhook` 只在构造时保存 `*url.URL`。日志字段只写 `target=u.Host`（主机与端口）。
- 发送失败时用 `errors.As(err, &*url.Error)` 取出 `ue.Err` 写进 `error` 字段。`*url.Error.Error()` 会带完整 URL，所以绝不直接记录这个 error。
- 非 2xx 时只记 `status`。
- 配置校验报错用固定文案的哨兵错误 `ErrWebhookInvalid`。`url.Parse` 返回的 error 带原值，所以不 wrap 它。
- 启动日志 `listening` 增加 `notify_enabled`（bool）和 `hysteresis_c` 两个字段，不写 URL。

备选方案：扩展 `redact.Handler` 支持多条秘密。否决原因：

- 要改 ADR 0005 固定的签名。
- 字符串替换要对 URL 的各个片段（路径、查询、userinfo，以及它们的解码形态）分别做，复杂度和误伤风险都比 D6 高。D6 之下 URL 根本不会进入任何日志记录，也就不需要再替换。

残余风险见下文的 Risks。

### D7 配置

- `config.Config` 新增 `NotifyWebhookURL string` 和 `NotifyHysteresisC float64`。`knownKeys` 增加两个键。
- 新增哨兵错误 `ErrHysteresisInvalid` 和 `ErrWebhookInvalid`，main 的 `configHint` 为它们各配一条提示。`ErrUnknownKey` 的提示列出全部五个键。
- 校验：
  - 回差：`ParseFloat` 成功，且不是 NaN、不是 Inf，且 `>= 0`。
  - URL：`url.Parse` 成功，scheme 为 `http` 或 `https`，`Host` 非空。
- `scripts/lib/local_env.sh` 的 `case` 分支和报错文案同步列出五个键。

### D8 main 组装

- `cfg.NotifyWebhookURL != ""` 时，创建 `notify.New(...)`，并把 `service.WithNotifier(w, cfg.NotifyHysteresisC)` 传给 `service.New`。
- 停服 goroutine 里：`srv.Shutdown(shutCtx)`（超时则 `srv.Close`）之后，调用 `w.Close(shutCtx)`，与 HTTP 停服共用同一个期限。
- 先停 HTTP、再关 notifier：进行中的请求可能还会入队，等它们入完队再收尾。

### D9 测试分层

- `internal/service`：一个同步记录事件的假 Notifier，覆盖 NTF-01..08、NTF-13 的状态语义。并发场景在 `-race` 下跑。
- `internal/notify`：用 `httptest.Server` 覆盖 NTF-09..12、RUN-06 的队列语义，以及 RUN-04 的「日志不含 URL 片段」。`Close` 用作测试的同步点。
- `internal/config` 和 `cmd/temp-monitor`：新键、非法值、启动日志字段、stderr 不含 token、停服时 webhook 卡住。
- 连真库层：`internal/httpapi/e2e_integration_test.go` 增加一条「进入 → 恢复」路径。

## Risks / Trade-offs

- [服务重启丢状态：可能重复发 alarm，也可能漏发 resolved] → 作为 NTF-13 写进 spec，README 里说明。可靠投递留给以后的 outbox change。
- [webhook 长时间不可用时通知全部丢失，只剩 WARN] → 日志告警（ALM-01）不受影响，仍是完整记录。不重试是有意为之，见 proposal「What Changes」的投递一条。
- [以后有人直接 `logger.Warn(..., "error", err)` 记录 `*url.Error`，URL 就会泄露] → `notify` 包内只有一处记录发送错误，在那里加注释说明原因。RUN-04 的两个场景测试断言 stderr 不含 token，回归时会失败。
- [注销与上报竞争：上报的入库发生在注销之前，状态评估却发生在注销之后，会给已删除的设备留下一个 `over` 标记] → 重新注册时（NTF-07）会清掉。剩下的窗口要求「入库、注销、重新注册、评估」恰好按这个顺序交错，影响只是可能多发一条 alarm，接受。
- [并发上报时，状态评估顺序可能与入库提交顺序不同] → 状态以评估顺序为准，spec 不承诺与 `recorded_at` 顺序一致，接受。
- [全局锁] → 临界区里没有 I/O，在示例的负载下可以忽略。
- [队满时每丢一条写一条 WARN，可能刷日志] → 事件已经去重，只有状态转换才产生事件，量级受设备数和抖动频率限制，接受。

## Migration Plan

- 新键都可选，不配就和现在一样，部署不需要额外步骤。
- 回滚：去掉 `TM_NOTIFY_WEBHOOK_URL` 就停用通知。如果回滚到旧版本二进制，还要从 `config/local.env` 删掉这两个新键，否则旧版本会报 unknown key 拒绝启动。
