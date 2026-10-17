# Design

## Context

动机见 proposal.md 的 Why，行为契约见 `specs/alarm-notification` 与各 MODIFIED 要求。与实现相关的现状：

- `service.Service` 在 `Report` 入库成功后直接写 WARN 日志。注释写明它「构造后没有可变状态」，跨线规则会打破这一点。
- `service.New(store, thresholdC, logger)` 由 `main.go`、`service_test.go`、`httpapi_test.go`、`e2e_integration_test.go` 调用。
- `httpapi` 给每个请求的存储调用 5 秒时限（`storeTimeout`）。上报的响应在 `Report` 返回后才写出。
- `main.run` 在一个 goroutine 里等信号，用 5 秒的 `shutCtx` 调 `srv.Shutdown`，超时就 `srv.Close`。之后 `run` 返回，`defer closeStore()` 关闭连接池。
- `redact.Handler(inner, connURL)` 只认数据库连接串：字符串整串替换，错误值另做口令替换。
- 配置文法的已知键在两处各写一份：`internal/config` 的 `knownKeys`，以及 `scripts/lib/local_env.sh` 的 `case`。

## Goals / Non-Goals

**Goals:**

- 发送通知不占用上报请求的时间，也不让退出超过现有的 5 秒。
- 通知相关代码可以不连网、不连数据库地测试：service 依赖一个接口，发送部分对着 `httptest` 测。
- 除了 `service.New` 新增一个可选参数，现有函数签名与调用点不变。

**Non-Goals:**

- 不做重试、持久化队列、多个 webhook、按设备配置接收方。
- 不签名请求体，也不加鉴权头。需要鉴权的接收方把令牌放进 URL，由 RUN-04 的脱敏兜住。
- 不做滞回。阈值附近反复跳动导致的多条通知，按现有规则照发。

## Decisions

### D1 新增 `internal/notify` 包，service 只依赖一个接口

`internal/notify` 负责队列、发送者、HTTP 发送和退出处理。`service` 包内定义它需要的最小接口：

```go
type Notifier interface {
    Enqueue(notify.Event) // 不阻塞；队列满时丢弃并写 ERROR
}
```

`notify.Event` 包含 `Kind`（`alarm` / `recovered`）、`DeviceID`、`TemperatureC`、`ThresholdC`、`At`。`At` 取 `store.Reading.RecordedAt`。

- **为什么**：service 的单元测试用一个记录事件的假实现，就能覆盖 NTF-02 到 NTF-05 以及并发场景，不用起 HTTP。与 `store.Store` 的接口加假实现模式一致。
- **备选**：在 service 里直接发 HTTP。这样跨线逻辑和网络失败纠缠在一起，测试必须起接收端，所以否决。

### D2 `service.New` 改为接受可选参数

签名改为 `New(s store.Store, thresholdC float64, logger *slog.Logger, opts ...Option)`，并新增 `WithNotifier(n Notifier) Option`。不传时 `notifier == nil`：不维护状态，不入队，行为与现在逐字节相同（NTF-01）。

- **为什么**：四处现有调用不用改。没配置 webhook 时，连状态表都不建，没有引入回归的面。
- **备选**：新增一个位置参数。要改四处调用和测试，收益只是写法更显式，所以否决。

### D3 跨线状态：一把锁、一张表，入队也在锁内

`Service` 新增 `mu sync.Mutex` 与 `overTemp map[string]bool`（只记处于「超温」的设备，值为 true）。

- `Report`：入库成功后加锁，比较旧状态与 `tempC > threshold`。状态变化时更新表，并在**同一把锁内**调用 `Enqueue`，然后解锁。WARN 日志的写法和位置不变。
- `Register` / `Deregister`：存储调用成功后加锁，`delete(overTemp, id)`。
- 加锁前先判断 `notifier == nil`，为 nil 时直接跳过。

在锁内入队，保证队列顺序与状态切换顺序一致（NTF-09）。`Enqueue` 不阻塞，所以持锁的时间很短。全局一把锁而不是每台设备一把：演示规模下竞争可以忽略，代码最简单。

**已知竞态，接受不处理**：`Register` 的存储调用成功后、复位之前，如果同一设备的一条上报已入库并把状态置为「超温」，复位会把它抹掉。结果是下一条超温读数再发一次 `alarm`，只会多发，不会漏发。`Deregister` 与上报交错时，可能留下一个已注销设备的「超温」记录，但重新注册时会被复位，不影响行为。

### D4 发送者：带上限的 channel，一个 goroutine

`notify.New(url string, logger *slog.Logger, opts...)` 创建一个容量 100 的 `chan Event`，并启动一个发送 goroutine。

- `Enqueue`：持 `mu` 检查 `closed`。已关闭就直接丢弃（退出阶段迟到的上报，不 panic）。否则用 `select` 非阻塞发送，失败就写 ERROR `alarm notification dropped`（`reason=queue full`）。入队成功写 INFO `alarm notification queued`，含 `event`、`device_id`、`temperature_c`。
- 发送 goroutine 按顺序取事件，每条构造一个 `POST` 请求，JSON 字段与 NTF-06 一致，`at` 用 RFC 3339。
- `http.Client`：`Timeout` 为 2 秒，可以由测试通过 option 缩短；`CheckRedirect` 返回 `http.ErrUseLastResponse`，不跟随重定向，3xx 按非 2xx 处理。不跟随重定向是为了不把请求体送到配置之外的地址。
- 响应体最多读 4 KiB 后丢弃并关闭，以便复用连接。响应体**从不写日志**。

### D5 失败日志只写固定类别，不写错误原文

发送失败时写 ERROR `alarm notification failed`，字段为 `device_id`、`event`、`reason`：

- `reason=timeout`：`url.Error.Timeout()` 为真。
- `reason=connection failed`：其它 transport 错误。
- `reason=status` 加 `status=<code>`：接收端返回非 2xx。

- **为什么**：`*url.Error` 的文本会带上完整 URL，令牌就在里面。只写类别，在源头上不产生秘密。这与 `pgstore.ErrConnect` 只给固定文案是同一个思路。
- **兜底**：`redact.Handler` 新增可变参数 `extra ...string`，`main` 把 webhook URL 传进去。消息、字符串属性和错误值中出现这个完整 URL 时，都整串替换为 `***`。以后谁误把 `err` 写进日志，也不会泄露。不做路径片段级的替换：令牌片段可能很短或很常见，与现有口令替换只作用于错误值是同一个考虑。

### D6 退出：复用 `shutCtx`，在 `srv.Shutdown` 之后关闭发送者

`notify.Notifier.Close(ctx) (dropped int)`：

1. 持锁置 `closed = true`，然后关闭 channel。
2. 等发送 goroutine 把剩下的事件发完，或者等到 `ctx` 到期。
3. `ctx` 到期时取消发送 goroutine 的基础 context，中断正在进行的请求。返回值为未发出的条数，包括正在发送的那一条。

`main` 在等信号的 goroutine 里，`srv.Shutdown(shutCtx)`（以及超时后的 `srv.Close()`）之后调用 `n.Close(shutCtx)`。`dropped > 0` 时写 WARN `alarm notifications dropped at shutdown`，含 `count`。之后照常 `closeStore`。

- **为什么**：HTTP 先停，就不会再有新的入队。与 HTTP 共用同一个 5 秒 deadline，RUN-06 的上限自然成立。如果 HTTP 已经把 5 秒用完，通知直接全部丢弃并记 WARN。
- **备选**：给通知单独留一段时间。这会让退出最长达到 5 秒加上这段时间，违反 RUN-06，所以否决。

### D7 配置与启动

- `config.Config` 新增 `AlarmWebhookURL string`，`knownKeys` 加上新键，新增 `ErrWebhookInvalid`。
- 校验：`url.Parse` 成功，`Scheme` 为 `http` 或 `https`，`Host` 非空。错误文案是固定文本 `TM_ALARM_WEBHOOK_URL is invalid`，`configHint` 返回 `must be an http or https URL`。不回显值，`url.Parse` 的错误也不拼进来，因为它的文本含原值。
- `configHint` 的「allowed keys」列表加上新键。`scripts/lib/local_env.sh` 的 `case` 和报错文案也同步加上，两处的已知键必须一致。
- `listening` 启动日志加字段 `alarm_notify=enabled|disabled`，不写 URL。
- `config/example.env` 加 `TM_ALARM_WEBHOOK_URL=`，值留空，等同未配置。注释说明：留空即不通知；令牌放在 URL 里属于秘密，只写进 `config/local.env`。

## Risks / Trade-offs

- [状态只在内存，重启丢失] → 可能漏掉一条 `recovered`，或者多发一条 `alarm`。这一点写进了 NTF-05，并在 README 的配置一节说明。演示项目不为此建表。
- [不重试] → 接收端短暂故障时，这次跨线的通知会丢失，而状态已经切到「超温」，之后不会再提醒。ERROR 日志可以查到。需要可靠投递时，应在接收端之前放一个有重试能力的中转，不在本服务里做。
- [阈值附近反复跳动会产生成串的通知] → 接受。以后真遇到了，再单独提一个滞回的 change。
- [队列满时丢弃] → 只有在 100 次跨线积压、接收端又很慢的情况下才会出现。演示规模下基本不会发生，发生时有 ERROR。
- [全局锁] → 每条上报都会在入库后短暂持锁，只有启用通知时才加锁。演示的并发量下可以忽略。
- [webhook 指向 `127.0.0.1` 以外的地址] → 服务会向那个地址发出请求，属于有意的外发流量。仓内示例的值留空，不提交任何真实地址。

## Migration Plan

不改表结构，不需要迁移。升级后，没配置新键的部署行为完全不变。回滚就是删掉配置项或退回版本。
