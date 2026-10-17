# Design

## Context

动机见 proposal.md 的 Why，行为契约见 `specs/alarm-notification` 与各 MODIFIED 要求，术语见 `GLOSSARY.md`。下面列出与实现有关的现状：

- `service.Service` 在 `Report` 入库成功后，直接写告警日志（WARN）。它的注释写明「构造后没有可变状态」，而告警状态会打破这一点。
- `service.New(store, thresholdC, logger)` 有四处调用：`main.go`、`service_test.go`、`httpapi_test.go`、`e2e_integration_test.go`。
- `httpapi` 给每个请求的存储调用 5 秒时限（`storeTimeout`）。上报的响应要等 `Report` 返回后才写出。
- `main.run` 在一个 goroutine 里等信号，用 5 秒的 `shutCtx` 调 `srv.Shutdown`，超时就调 `srv.Close`。之后 `run` 返回，由 `defer closeStore()` 关闭连接池。
- `redact.Handler(inner, connURL)` 只认数据库连接串：字符串做整串替换，错误值另做口令替换。
- 配置文法的已知键写在两处：`internal/config` 的 `knownKeys`，和 `scripts/lib/local_env.sh` 的 `case`。

## Goals / Non-Goals

**Goals:**

- 发送告警通知不占用上报请求的时间，也不让退出超过现有的 5 秒。
- 告警通知相关的代码可以在不连外网、不连数据库的情况下测试：service 依赖一个接口；发送部分用 `httptest` 起的假接收方来测。
- 现有函数签名与调用点保持不变，唯一的改动是 `service.New` 新增一个可选参数。

**Non-Goals:**

- 不做重试、持久化队列、多个 webhook，也不支持按设备配置接收方。
- 不对请求体签名，也不加鉴权头。需要鉴权的接收方把令牌放进 URL，由 RUN-04 的脱敏兜住。
- 不做滞回。温度在阈值附近反复跳动时，产生的多条告警通知照规则发送。

## Decisions

跨线触发、告警状态只放内存、不重试，这三项决定及被否决的方案，记录在 `openspec/adr/0006-alarm-notification-edge-triggered.md`。本节只写实现方式。

### D1 新增 `internal/notify` 包，service 只依赖一个接口

`internal/notify` 负责排队、发送者、HTTP 发送和退出处理。`service` 包内只定义它需要的最小接口：

```go
type Notifier interface {
    Enqueue(notify.Event) // 不阻塞；排队已满时丢弃并写 ERROR
}
```

`notify.Event` 表示一条待发的告警通知，字段有 `Kind`（`alarm` 或 `recovered`）、`DeviceID`、`TemperatureC`、`ThresholdC`、`At`。`At` 取自 `store.Reading.RecordedAt`。

- **为什么**：service 的单元测试只要换上一个记录告警通知的假实现，就能覆盖 NTF-02 到 NTF-05 以及并发场景，不用起 HTTP。这与 `store.Store` 用接口加假实现的做法一致。
- **备选**：在 service 里直接发 HTTP。这样告警状态的逻辑会和网络失败缠在一起，测试也必须起一个假接收方，所以否决。

### D2 `service.New` 改为接受可选参数

签名改为 `New(s store.Store, thresholdC float64, logger *slog.Logger, opts ...Option)`，并新增 `WithNotifier(n Notifier) Option`。不传这个参数时 `notifier == nil`，此时不维护告警状态，也不入队，行为与现在完全相同（NTF-01）。

- **为什么**：现有四处调用都不用改。没配置 webhook 时连告警状态表都不建，不会引入回归。
- **备选**：新增一个位置参数。四处调用和测试都要改，换来的只是写法更显式，所以否决。

### D3 告警状态：一把锁，一张表，入队也在锁内

`Service` 新增 `mu sync.Mutex` 和 `alarming map[string]bool`。表里只记处于「告警中」的设备，值为 true；不在表里就是「正常」。

- `Report`：入库成功后加锁，比较旧的告警状态和 `tempC > threshold` 的结果。状态有变化时，更新表，并在**同一把锁内**调用 `Enqueue`，然后解锁。告警日志的写法和位置不变。
- `Register` / `Deregister`：存储调用成功后加锁，执行 `delete(alarming, id)`，即复位。复位不入队。
- 加锁前先判断 `notifier == nil`，为 nil 就直接跳过。

在锁内入队，可以保证排队顺序与状态切换顺序一致（NTF-09）。`Enqueue` 不阻塞，所以持锁时间很短。用全局一把锁而不是每台设备一把，是因为演示规模下锁竞争可以忽略，代码也最简单。

**已知竞态，接受不处理**：
- `Register` 的存储调用成功之后、复位之前，同一设备可能有一条上报已经入库，并把状态置为「告警中」，复位会把这个状态抹掉。后果是下一条超温读数会再发一次 `alarm`，只会多发，不会漏发。
- `Deregister` 与上报交错时，表里可能残留一条已注销设备的「告警中」记录。重新注册时会复位，所以不影响行为。

### D4 发送者：一个带缓冲的 channel 加一个 goroutine

`notify.New(url string, logger *slog.Logger, opts...)` 创建一个容量为 100 的 `chan Event`，并启动一个发送 goroutine。发送 goroutine 已经取出的那一条不占 channel 容量，这正好对应 NTF-08 里"排队中 100 条，不含正在发送的一条"。

- `Enqueue`：先持 `mu` 检查 `closed`。已关闭就直接丢弃，这样退出阶段迟到的上报不会 panic。未关闭就用 `select` 非阻塞发送：
  - 发送失败，写 ERROR `alarm notification dropped`（`reason=queue full`）。
  - 入队成功，写 INFO `alarm notification queued`，含 `event`、`device_id`、`temperature_c`。
- 发送 goroutine 按顺序取出告警通知，每条构造一个 `POST` 请求。JSON 字段与 NTF-06 一致，`at` 用 RFC 3339 格式。
- `http.Client` 的设置：
  - `Timeout` 为 2 秒，测试可以通过 option 缩短。
  - `CheckRedirect` 返回 `http.ErrUseLastResponse`，不跟随重定向，3xx 按非 2xx 处理。这样请求体不会被送到配置之外的地址。
- 响应体最多读 4 KiB，读完丢弃并关闭，以便复用连接。响应体**从不写日志**。
- 发送失败不回传给 service，告警状态不回退（见 ADR 0006）。

### D5 失败日志只写固定类别，不写错误原文

发送失败时写 ERROR `alarm notification failed`，字段为 `device_id`、`event`、`reason`：

- `reason=timeout`：`url.Error.Timeout()` 为真。
- `reason=connection failed`：其它 transport 错误。
- `reason=status`，并附 `status=<code>`：接收方返回非 2xx。

- **为什么**：`*url.Error` 的文本会带上完整 URL，而令牌就在 URL 里。只写类别，从源头上就不会产生秘密。这与 `pgstore.ErrConnect` 只给固定文案的思路相同。
- **兜底**：`redact.Handler` 新增可变参数 `extra ...string`，`main` 把 webhook URL 传进去。消息、字符串属性、错误值里只要出现完整的 webhook URL，都整串替换为 `***`。这样即使以后有人误把 `err` 写进日志，也不会泄露。
- 不做路径片段级的替换：令牌片段可能很短或很常见，替换会误伤正常文本。现有的口令替换只作用于错误值，也是出于同样的考虑。

### D6 退出：复用 `shutCtx`，在 `srv.Shutdown` 之后关闭发送者

`notify.Notifier.Close(ctx) (dropped int)` 的步骤：

1. 持锁置 `closed = true`，然后关闭 channel。
2. 等发送 goroutine 把剩下的告警通知发完，或者等到 `ctx` 到期。
3. 如果 `ctx` 到期，取消发送 goroutine 的基础 context，中断正在进行的请求。返回未发出的条数，包括正在发送的那一条。

`main` 在等信号的 goroutine 里调用 `n.Close(shutCtx)`，时机是 `srv.Shutdown(shutCtx)` 之后（如果 Shutdown 超时，则在 `srv.Close()` 之后）。`dropped > 0` 时写 WARN `alarm notifications dropped at shutdown`，含 `count`。之后照常执行 `closeStore`。

- **为什么**：先停 HTTP，就不会再有新的告警通知入队。告警通知与 HTTP 共用同一个 5 秒期限，RUN-06 的上限自然成立。如果 HTTP 已经用完这 5 秒，告警通知就全部丢弃，并写 WARN。
- **备选**：给告警通知单独留一段时间。这样退出最长会达到 5 秒再加上这段时间，违反 RUN-06，所以否决。

### D7 配置与启动

- `config.Config` 新增 `AlarmWebhookURL string`，`knownKeys` 加入新键，新增 `ErrWebhookInvalid`。
- 校验条件：`url.Parse` 成功，`Scheme` 为 `http` 或 `https`，`Host` 非空。
  - 错误文案用固定文本 `TM_ALARM_WEBHOOK_URL is invalid`，`configHint` 返回 `must be an http or https URL`。
  - 不回显这个值；`url.Parse` 的错误也不拼进来，因为它的文本含有原值。
- `configHint` 的「allowed keys」列表加入新键。`scripts/lib/local_env.sh` 的 `case` 和报错文案同步修改，两处的已知键必须一致。
- `listening` 启动日志新增字段 `alarm_notify=enabled|disabled`，不写 URL。
- `config/example.env` 加入 `TM_ALARM_WEBHOOK_URL=`，值留空，等同未配置。注释说明两点：留空就不发告警通知；URL 里的令牌属于秘密，只写进 `config/local.env`。

## Risks / Trade-offs

- [告警状态只在内存中，重启会复位] → 可能收不到一条 `recovered`，或者多收一条 `alarm`。已写进 NTF-05，README 的配置一节也会说明。演示项目不为此建表。
- [注销时复位，不发 `recovered`] → 如果接收方是值班平台，这个告警期可能一直不关闭。注销是运维有意的操作，所以接受。已写进 NTF-05。
- [不重试] → 一条 `alarm` 没送达，告警状态已经是「告警中」，整个告警期内都不会再发告警通知。值班人员可能直到恢复时才收到一条孤立的 `recovered`。ERROR 日志是唯一的信号。需要可靠投递时，应在接收方之前放一个带重试的中转，不在本服务里做。见 NTF-08 和 ADR 0006。
- [温度在阈值附近反复跳动，会产生成串的告警通知] → 接受。以后真遇到了，再单独提一个做滞回的 change。
- [排队已满时丢弃] → 只有在 100 次状态切换积压、同时接收方又很慢时才会发生。演示规模下基本不会出现，出现时有 ERROR 日志。
- [全局锁] → 启用告警通知后，每条上报入库后都会短暂持锁。演示的并发量下可以忽略。
- [webhook 指向 `127.0.0.1` 以外的地址] → 服务会向那个地址发请求，这是有意的外发流量。仓内示例的值留空，不提交任何真实地址。

## Migration Plan

不改表结构，不需要迁移。升级后，没配置新键的部署，行为完全不变。回滚时删掉这个配置项，或者退回旧版本即可。
