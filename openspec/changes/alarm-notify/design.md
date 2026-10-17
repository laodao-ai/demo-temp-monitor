# Design

## Context

动机见 proposal.md「Why」，行为要求见本改动的 delta spec（ALM-05 至 ALM-08，RUN-01 至 RUN-06，TST-08），术语见 `GLOSSARY.md`，送达保证的取舍见 ADR 0006。和设计有关的现状：

- 告警在 `internal/service/service.go` 的 `Report` 里同步执行：入库成功、温度大于阈值时写一条 WARN。`Service` 构造后没有可变状态。
- 存储层的写法是：`internal/store` 定义接口，`service` 只依赖接口，另有一个内存假实现（单元测试用）和一个真实现。推送照搬这个写法。
- 日志统一经过 `redact.Handler`（ADR-0005）。它只替换数据库连接串和口令，不认识 webhook URL。
- `cmd/temp-monitor/main.go` 收到信号后用 5 秒的 `shutCtx` 调 `srv.Shutdown`；超时就 `srv.Close`，然后 `closeStore`。
- `config/local.env` 的已知键有两处维护：`internal/config` 的 `knownKeys`，以及 `scripts/lib/local_env.sh` 里的 `case`。

## Goals / Non-Goals

**Goals:**

- webhook 慢、挂或者刷屏，都不能拖慢上报，也不能让内存无限增长。
- webhook URL 不能从任何输出路径泄露，而且要靠结构保证，不靠"写日志时记得脱敏"。
- 不配置 webhook 时，代码路径与现在等价，现有测试只需机械地改构造函数的调用。

**Non-Goals:**

- 不做请求签名（HMAC）和自定义请求头。需要鉴权时，由接收端把 token 放进 URL。
- 不适配特定接收端的报文格式，只发一种通用 JSON。
- 不做多个 webhook，也不按设备路由。

## Decisions

### D1 新包 `internal/push`：接口、webhook 实现、空实现

```
service.Report --Push(Alarm)--> push.Pusher (接口)
                                     |
                       +-------------+-------------+
                       |                           |
                   push.Nop                   push.Webhook
            (未配置时；单元测试)        chan Alarm (cap 100) --> 1 个 worker --> POST
```

- `push.Alarm` 包含 `DeviceID`、`TemperatureC`、`ThresholdC`、`ReadingID`、`RecordedAt`，由 `Report` 用入库返回的 `store.Reading` 填写。
- `Pusher` 只有一个方法 `Push(Alarm)`，**必须非阻塞**。`Webhook` 的实现是对 channel 做 `select` 发送，带 `default` 分支：队列满时走 `default`，记 `alarm push dropped`（ALM-08）。
- 一个 worker 串行发送。这样同一设备的推送按顺序到达，在途请求最多 1 个。
- 接口定义在 `push` 里，`service` import `push`，方向与 `service` import `store` 相同，不会循环依赖。

否决的方案：

- **每条告警起一个 goroutine**：webhook 挂掉时，goroutine 数量等于"超温速率 × 5 秒"，没有上限；退出时也数不清丢了多少条。
- **在 `httpapi` 里推送**：告警规则（入库成功且严格大于阈值）在 `service` 里。放到 HTTP 层会出现第二份判断，ALM-03 很容易两处不一致。
- **挂成 slog handler，看到 WARN 就推送**：靠日志消息字符串触发业务行为，太脆弱；而且拿不到 `reading_id`。

### D2 `service.New` 增加 `push.Pusher` 参数

签名改为 `New(s store.Store, thresholdC float64, logger *slog.Logger, p push.Pusher)`。4 个调用点（main、`service_test`、`httpapi_test`、e2e）都显式传入，测试传 `push.Nop{}` 或记录型假实现。不接受 nil，避免 `Report` 里出现空指针分支。

在 `Report` 里，先写 WARN 日志，再调 `Push`。这样即使 `Push` 丢弃了推送，WARN 日志仍然完整，作为底账。

否决：用 `WithPusher` 选项保留旧签名。只有 4 个调用点，显式参数让"这个 Service 会不会推送"在构造处一眼可见。

### D3 发送细节

- `http.Client` 不设整体超时。每个请求用 `context.WithTimeout(base, 5s)`，其中 `base` 是 `Webhook` 内部的 context，退出时取消（见 D5）。超时时长做成构造参数，缺省 5 秒，测试可以调短。
- `CheckRedirect` 返回 `http.ErrUseLastResponse`：不跟随重定向，3xx 按"非 2xx"处理。这样请求不会被带到配置之外的地址。
- 响应体最多读 64 KiB 后丢弃，然后关闭，以便复用连接。
- 请求体用 `encoding/json` 编码。`recorded_at` 用 `time.RFC3339Nano`，与上报响应的格式一致。
- `summary` 由 `push` 生成，格式为 `<device_id> 温度 <t>°C，超过阈值 <th>°C`，数字用 `strconv.FormatFloat(v, 'f', -1, 64)`，即能精确还原原值的最短十进制写法（55、55.25、50.5）。`summary` 只给人看，接收端不应解析它，结构化信息以其它字段为准。

### D4 失败原因用枚举，日志里不出现 `err`

webhook 失败日志只写 `reason` 字段，取值为 `timeout`、`connection error`、`http status N` 三种之一（ALM-07），**不把 `err` 本身作为属性写进日志**。原因是：`net/http` 返回的 `*url.Error` 在 `Error()` 里会带上完整 URL（`Post "http://…/tok": …`），而 `redact.Handler` 只认识数据库连接串。

判定方法：`errors.Is(err, context.DeadlineExceeded)` 或 `url.Error.Timeout()` 为真时记 `timeout`；其它传输层错误记 `connection error`；拿到响应但状态码不在 2xx 时记 `http status N`。

否决：扩展 `redact.Handler`，让它也替换 webhook URL。这要改 ADR-0005 已定的签名，而且只能替换整串，挡不住 URL 片段（比如只打印路径的错误）。用枚举从结构上就杜绝了泄露。RUN-04 的新场景负责防止回归。

`config.ErrWebhookURLInvalid` 沿用现有做法：错误文本固定，只含键名。

### D5 退出顺序：共用 `shutCtx`

```
SIGINT
  |
  v
srv.Shutdown(shutCtx)        ---+
  | (超时则 srv.Close)          |  同一个 5 秒 shutCtx
  v                             |
webhook.Close(shutCtx) -> n  ---+  停止接收；发送剩余队列；
  |                                shutCtx 到期时取消 base，中止在途请求，
  |                                n = 未发出的条数 (含被中止的)
  v
n > 0 ? WARN "alarm push dropped at shutdown" count=n
  |
  v
closeStore()  -> exit 0
```

- `Close` 之后再调 `Push` 不入队，计入 `n`。这种情况只在 `Shutdown` 超时后 `srv.Close`、仍有请求在收尾时出现。这样计数完整，也不会向已关闭的 channel 发送而 panic（channel 不关闭，用一个 closed 标志加 mutex 保护）。
- `Pusher` 接口里不放 `Close`。`main` 在配置了地址时另外持有具体类型 `*push.Webhook` 并在退出时调用 `Close`；未配置时传 `push.Nop{}`，跳过这一步，也不记那条 WARN。
- RUN-06 的场景允许 6 秒，多出的 1 秒给 `closeStore` 和进程退出留余量。

### D6 配置

- `config.Config` 增加 `AlarmWebhookURL string`。校验规则：`url.Parse` 成功，scheme 为 `http` 或 `https`，`Host` 非空。`knownKeys` 增加该键；`configHint` 里未知键的提示列出四个键。
- `scripts/lib/local_env.sh` 的 `case` 和未知键提示同步加上第四个键。脚本自身不读这个键。
- 启动 INFO `listening` 增加一个字段 `alarm_push=true|false`（RUN-02），方便排查"为什么没收到推送"，不输出 URL。
- `http` 与 `https` 都接受。地址含 token 时应使用 `https`，这一点只在 `example.env` 的注释里提醒，代码不强制。原因是内网接收端常用 http，而本地测试需要 http。

### D7 测试分层

- `push` 单元测试：`httptest.NewServer`（只监听 127.0.0.1）分别模拟 200（核对字段与 `summary`）、503、一直不响应、端口关闭，以及 3xx 不跟随；队列满；`Close` 时发得完和发不完两种情况；日志不含 URL 片段（RUN-04）。超时调成 100ms 量级，保证单元层跑得快。
- `service` 单元测试：用记录型 `Pusher` 断言 ALM-05 的对应关系（ALM-02、ALM-03 的情形不推送；Alarm 字段取自入库结果）。
- `cmd/temp-monitor` 单元测试：非法 URL 退出 1 且不回显值；启动日志的 `alarm_push` 取值正确；退出时推送发不完会记 WARN。
- e2e（TST-08）：在现有用例中加一个 httptest 接收端，断言收到一次推送，`reading_id` 一致。

## Risks / Trade-offs

- [接收端不合并重复告警，持续超温时刷屏] → 已作为前提写进 proposal。如果出现，另开改动做按设备的触发/恢复推送。本设计的 `Pusher` 接口届时可以加一层有状态的包装，`service` 不用改。
- [接收端要求固定格式（字段名或结构不同）] → 现在只发通用 JSON。大多数接收端的通用 webhook 都支持字段映射；只会原样展示的接收端，至少能显示 `summary`。如果都不满足，就要改 ALM-05 的报文字段，属于一次 spec 改动。
- [推送会丢：发送失败、队列满、退出时没发完] → 这是有意的取舍（不重试、不落库）。持续超温时后续读数会再次推送；每条告警都有 WARN 日志可查；每次丢弃都有 ERROR 或 WARN 记录。
- [webhook 挂掉期间每条超温读数都多一条 ERROR] → 速率与 WARN 相同，不会放大日志量。不做限频，避免引入状态。
- [推送到达时设备可能已注销，`reading_id` 查不到] → 推送只是事件告知，不保证读数仍然存在。接收端只用推送里的字段，不回查。

## Migration Plan

- 不需要迁移数据库。不配置 `TM_ALARM_WEBHOOK_URL` 时行为不变，可以直接上线。
- 启用：在 `config/local.env` 或环境变量里配置 URL，然后重启服务。回滚：删掉配置后重启。
- 归档本改动时，同步修改 `openspec/specs/over-temperature-alarm/spec.md` 的 Purpose（去掉「不做通知」），在 `openspec/INDEX.md` 里更新该能力的说明，并把 ADR 0006 改为 Accepted。
