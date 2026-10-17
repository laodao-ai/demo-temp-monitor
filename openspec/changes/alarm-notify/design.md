# Design

## Context

动机见 proposal.md「Why」，行为契约见本 change 的 specs。影响做法的现状：

- `service.Service` 构造后没有可变状态（`internal/service/service.go:24`）。告警在 `Report` 里、`AddReading` 成功之后判断阈值并写 WARN。
- httpapi 用 `context.WithTimeout(r.Context(), storeTimeout)`（5 秒）包住整个 `svc.Report` 调用，`Report` 拿到的 ctx 既会随客户端断开取消，也会在 5 秒后到期。
- 脱敏是包装的 slog handler（ADR-0005）：消息和字符串属性只替换整条连接串，error 值才做口令替换。教训是替换短子串会误伤正常文本。
- `config.Load` 的已知键是一个固定集合，`scripts/lib/local_env.sh:51` 另有一份同样的白名单，两者必须同步（RUN-01）。
- 现有测试与 main 都按 `service.New(st, threshold, logger)` 和 `redact.Handler(inner, connURL)` 调用。

## Goals / Non-Goals

**Goals:**

- 通知逻辑与 HTTP 发送分开：service 只依赖一个通知接口，单元测试用假实现断言调用，不起网络；webhook 实现单独用 `httptest.Server` 测。
- 现有调用点不改也能编译，未配置 webhook 时行为与现在逐字节一致（除启动日志多一个 `notify` 字段）。
- 任何错误路径都不把 webhook URL 或其中的 token 写进输出。

**Non-Goals:**

- 不做发送队列、后台重试、退避；重试只靠下一条超温读数。
- 不持久化通知状态，不加表，不改迁移。
- 不适配任何厂商的群机器人报文；不做多个 webhook、按设备路由或排班。
- 不对 webhook 的 TLS、代理做额外配置，用 Go 默认行为。`http` 与 `https` 都接受，URL 带 userinfo 也不拒。
- 不支持多个服务实例共用一个库：通知状态在各自进程内存里，多实例会让同一超温段重复通知。
- 启动时不探测 webhook 是否连通（RUN-02）。

术语（超温段、通知状态、未通知 / 已通知、通知接收端）以仓根 `GLOSSARY.md` 为准，代码命名跟随它。

## Decisions

### D1 新包 `internal/notify`：接口 + webhook 实现 + 内存假实现

```
 cmd/temp-monitor (main)
   |-- internal/config        + NotifyWebhookURL
   |-- internal/notify        Notifier 接口, Event, Error{Reason, StatusCode}
   |     |-- webhook.go       net/http 实现
   |     `-- memnotify/       假实现：记录收到的 Event，可注入错误、可阻塞
   |-- internal/httpapi --> internal/service --> internal/store
   |                              `-----------> internal/notify (只依赖接口)
   `-- internal/redact        + 额外秘密
```

- `Notifier` 只有一个方法 `Notify(ctx, Event) error`。`Event` 含 `DeviceID`、`ReadingID`、`TemperatureC`、`RecordedAt`、`ThresholdC`，取自 `AddReading` 返回的 `store.Reading`；JSON 编码时加固定的 `event: "over-temperature"`，`recorded_at` 用与上报响应相同的 `time.RFC3339Nano`（NTF-05）。
- webhook 实现返回的错误统一是 `*notify.Error`，带 `Reason`（`timeout` / `transport` / `status`）和 `StatusCode`。它的 `Error()` 是固定文本，**不包装** `*url.Error`，因为后者的文本含完整 URL。分类：ctx 到期或 `url.Error.Timeout()` 为 `timeout`；其它请求错误为 `transport`；非 2xx 为 `status`。
- `http.Client.CheckRedirect` 返回 `http.ErrUseLastResponse`，3xx 原样作为响应，按 `status` 失败处理（NTF-05）。
- 响应体读掉并丢弃（限长），以便连接复用；内容不进日志。
- 选这个结构而不是把 HTTP 调用直接写进 service：与 store 接口 + memstore 的既有模式一致；service 单测可以精确控制成功、失败、阻塞，不依赖时序。

### D2 通知状态放在 Service 里，用「先占位、失败再撤回」避免锁跨网络调用

Service 新增 `mu sync.Mutex` 和 `notified map[string]uint64`：有这一项表示已通知，值是占位序号 `gen`；没有这一项表示未通知。`Report` 在 `AddReading` 成功之后：

```
 temp <= 阈值:  lock; delete(notified, id); unlock          超温段结束 (NTF-03)
 temp >  阈值:  写 WARN (不变)
                lock
                  notified[id] 存在 ?  --> 是: unlock, return     (NTF-02/07)
                  gen := 下一个序号; notified[id] = gen          占位 = 已通知
                unlock
                err := notifier.Notify(detachedCtx, ev)
                err != nil:
                  写 ERROR (NTF-06)
                  lock; 若 notified[id] == gen 则 delete; unlock  撤回 = 未通知
```

- 锁只保护 map 读写，不跨 `Notify`，一台设备的慢 webhook 不阻塞其它设备。
- `gen` 防止撤回错位：A 占位后发送中，一条 ≤ 阈值读数结束了超温段，B 在新的超温段占位并发送成功；随后 A 失败。没有 `gen` 时 A 会把 B 的已通知清掉，导致下一条超温读数多发一次。
- 并发时被占位压下的读数不补发（NTF-07）：A 发送失败撤回后，要等同一超温段的下一条超温读数重试。这样不必记录被压下的读数。
- map 大小不超过处于已通知的设备数。
- 否决的方案：锁跨网络调用（简单但所有设备串行等 webhook）；`sync.Map` + CAS（撤回逻辑同样需要序号，不比 mutex 简单）。

### D3 注册与注销成功时都清除状态

`Deregister` 在 `DeleteDevice` 成功后删除该设备的项（NTF-08）。`Register` 在 `CreateDevice` 成功后也删除一次。原因：上报与注销并发时，上报可能在注销清理之后才占位，留下孤儿项；注册时再清一次，保证重新注册的设备从未通知开始。孤儿项本身只占一点内存，注册时自然消失。

注销不取消已开始的通知（NTF-08）：发送用的 ctx 不与设备关联，取消需要按设备维护可取消的 ctx，复杂度不值得。发送结束后若失败，撤回时 `gen` 不匹配或项已不存在，不影响新注册的设备。

### D4 同步发送，用脱离请求的 ctx，超时 2 秒

- `notifyCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), notifyTimeout)`，`notifyTimeout = 2 * time.Second` 是 service 包常量。`WithoutCancel` 同时去掉客户端断开和 httpapi 的 5 秒存储超时；2 秒是通知自己的上限（NTF-06）。
- 一条触发通知的上报最坏耗时约为存储 5 秒加通知 2 秒；只有超温段内占位成功的那一条会多等，同段后续超温读数不发通知、不变慢。
- 否决的方案：起 goroutine 异步发（退出时要等它、测试要加同步点）；队列加 worker 加重试（超出教学示例的分量）。

### D5 Service 构造：可选参数，不改现有签名

`New(s store.Store, thresholdC float64, logger *slog.Logger, opts ...Option)`，`WithNotifier(n notify.Notifier) Option`。不传时 notifier 为 nil，即不通知（NTF-09），状态 map 也不维护。现有 `service.New(st, th, logger)` 调用不变。包注释里「构造后没有可变状态」改为说明通知状态。

### D6 配置：第四个键与校验

- `config.Config` 加 `NotifyWebhookURL string`，`KeyNotifyWebhookURL = "TM_NOTIFY_WEBHOOK_URL"`，加入 `knownKeys`。空值即未配置。
- 非空时用 `url.Parse` 校验：无错误、scheme 为 `http` 或 `https`、`Host` 非空；不满足返回哨兵 `ErrWebhookURLInvalid`，文本只含键名。
- main 的 `configHint` 为它加提示（「must be an absolute http or https URL」），`ErrUnknownKey` 的提示改为列出四个键。
- `scripts/lib/local_env.sh` 的白名单和报错提示同步加上新键。

### D7 脱敏：`redact.Handler` 增加可变参数的额外秘密

- `Handler(inner slog.Handler, connURL string, secrets ...string)`。每个非空 secret 按**整串**替换为 `***`，适用于消息、字符串属性、分组成员和 error 值。不拆出 path、token 子串做替换，理由与 ADR-0005 修订相同：短子串（如 `/hook`）替换会误伤正常文本。
- RUN-04 的约束是来源性的：不把 URL 或从中取出的片段写入输出。它靠「从不记录」保证：webhook 实现的错误文本固定、不含 URL；service 写 ERROR 时只记 `device_id`、`reason`、`status_code`，不记 error 值；启动日志只记 `notify=on|off`；配置错误只给键名。整串替换是兜底。
- main 构造 logger 时把 `cfg.NotifyWebhookURL` 作为额外秘密传入。

### D8 main 组装

- 配了 URL 时 `notify.NewWebhook(url)`，传 `service.WithNotifier`；没配时不传。
- `listening` 日志加 `notify` 字段，值为 `on` 或 `off`（RUN-02）。构造 notifier 不发任何请求，启动不探测连通性。

### D9 测试分层

- `internal/notify`：webhook 实现用 `httptest.Server` 测报文字段与 `Content-Type`、2xx 成功、500 与 302 的 `status`、关闭的端口为 `transport`、不响应时 `timeout`、错误文本不含 URL。
- `internal/service`：用 `memnotify` 覆盖 NTF-01 到 NTF-09，含 NTF-07「发送中的通知失败」、NTF-08「注销时通知正在发送」，以及 D2 的撤回错位用例（假实现可阻塞，以便按顺序排出交错）。在 `-race` 下跑并发用例。
- `internal/config`、`cmd/temp-monitor`：新键、非法 URL 不回显、`notify=on|off`、启动时 webhook 收不到请求、webhook 不可达也能启动、通知失败的日志里没有 URL（RUN-04 新场景，用本机 `httptest.Server` 和关闭的端口）。
- 端到端（TST-08）：在现有用例里起 `httptest.Server` 作为通知接收端，断言恰好收到一次通知，且 `reading_id` 等于上报响应的 `id`。

## Risks / Trade-offs

- [读数在阈值附近来回跳时反复通知] -> 已知且接受（无滞回）；README 写明。以后要的话，再加一个恢复阈值配置。
- [重启后状态丢失，重复通知一次] -> 已知且接受，宁可多发不漏发。
- [webhook 慢时，触发通知的那条上报多等最多 2 秒] -> 只影响超温段内占位成功的那一条；超时写 ERROR，并在同一超温段的下一条超温读数时重试。
- [退出时正在发的通知可能丢失] -> `Shutdown` 等进行中的请求最多 5 秒，2 秒的通知通常能发完；超过时走 RUN-06 原有路径。丢失的那次在重启后的下一条超温读数会重发（状态已丢）。
- [通知失败后，若之后没有新的超温读数，值班人一直不知道] -> ERROR 日志可被外部日志告警兜底；本 change 不做后台重试（见 Non-Goals）。
- [群机器人拒收通用 JSON] -> 需要在接收端适配；README 说明报文格式。
- [重启或失败重试会让接收端收到同一超温段的多次通知] -> 报文带 `reading_id`、`recorded_at`，接收端可据此判断和去重。
- [服务不重启时，第二次运行 `simulate.sh` 收不到通知（`sim-03` 仍在同一超温段）] -> 不改脚本；README 演示一节说明：要再看到通知，就重启服务或先上报一条低温读数。
- [误起多个实例导致重复通知] -> 只支持单实例，见 Non-Goals；README 写明。
- [`http` 明文传 token] -> 教学示例的接收端在本机；README 提示非本机接收端用 `https`。
- [`gen` 与状态逻辑的并发 bug 不易察觉] -> D9 的阻塞型假实现覆盖交错顺序，并在 `-race` 下跑。

## Migration Plan

- 新键可选，不配置时行为不变，已有的 `config/local.env` 无需修改。
- 启用：在 `config/local.env` 或环境变量里设置 `TM_NOTIFY_WEBHOOK_URL` 后重启服务。回滚：清空该值后重启。
- 没有数据迁移。

## Open Questions

- 无。
