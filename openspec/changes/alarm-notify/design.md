## Context

动机见 proposal.md「Why」。与做法相关的现状：

- 超温判定在 `internal/service/service.go` 的 `(*Service).Report` 里，`store.AddReading` 成功后、温度严格大于阈值时同步调用 `s.logger.Warn("over-temperature alarm", ...)`。`Report` 运行在 HTTP 请求 goroutine 上，ctx 来自 `internal/httpapi/httpapi.go` 的 `context.WithTimeout(r.Context(), storeTimeout)`（`storeTimeout = 5 * time.Second`）。
- `service.New(s store.Store, thresholdC float64, logger *slog.Logger)` 有 4 个调用点：`cmd/temp-monitor/main.go:116`、`internal/httpapi/httpapi_test.go:57`、`internal/httpapi/e2e_integration_test.go:36`、`internal/service/service_test.go:49`。
- 配置在 `internal/config/config.go`：`Config` 三个字段，`knownKeys` 白名单，`Load(getenv, path)`。错误为哨兵错误，消息只含键名。allowed keys 提示在 `cmd/temp-monitor/main.go` 的 `configHint` 与 `internal/testdb/testdb.go` 的 `configFailure` 各拼一次，`scripts/lib/local_env.sh` 的 `case` 与报错文案再写一次。
- 脱敏在 `internal/redact`：`Handler(inner slog.Handler, connURL string) slog.Handler`。口令只对 error 值替换，消息与字符串属性只替换整条连接串（ADR-0005 与 issue B1 的修订）。
- `store.Reading` 字段：`ID`、`DeviceID`、`TemperatureC`、`RecordedAt time.Time`。
- 关闭流程在 `run`：收到信号后 `srv.Shutdown`，上限 `shutdownTimeout = 5 * time.Second`（RUN-06）。

## Goals / Non-Goals

**Goals:**

- 推送与上报完全隔离：推送在独立 goroutine、独立 ctx 上执行，上报响应与耗时不受钉钉状态影响。
- 冷却的「判断 + 记录」在一次加锁内完成，并发上报同一设备只放行一次。
- 推送模块可在无网络环境下完整测试：发送方可替换为 httptest 服务器，时钟可注入。
- 新配置键在服务与脚本两侧同一文法、同一白名单。

**Non-Goals:**（每条附可证伪假设，D-3）

- 不为推送 goroutine 做停机等待。假设：停机时恰有推送在途的概率低，丢失的那条会在设备仍超温时于下个冷却期重推；若运维发现发版时段系统性漏推，需在 `run` 中加有界等待。
- 不清理冷却表中的旧条目。假设：冷却表条目数不超过曾经超温过的设备 ID 数，教学示例中为数十量级；若设备 ID 持续高基数变化（例如每次上报换 ID），需加过期清理。
- 不校验 webhook 的主机是不是钉钉。假设：配置者填的是钉钉机器人地址；若填错，推送失败会写 ERROR 日志，可从日志发现。
- 不做「推送已被冷却抑制」的可见日志（仅 DEBUG）。假设：值班的人与运维不需要逐条知道哪条读数没推；若需要排查，可临时调低日志级别。

## Design Diagrams

### 组件 / 依赖图（变更前后）

```
 变更前:
   main --> config, redact, service, httpapi, pgstore
   httpapi --> service --> store
                       \-> slog (WARN)

 变更后:
   main --> config (+3 键), redact (+多密钥), notify (新), service, httpapi, pgstore
   httpapi --> service --> store
                       +-> slog (WARN, 不变)
                       +-> notify.Notifier (接口)
                              ^
                              | 实现
                   +----------+-----------+
                   |                      |
               notify.Nop          notify.Throttled --(go, 5s ctx)--> notify.DingTalk --HTTPS--> 钉钉 webhook
               (未配置)            (冷却表 + mutex)                    (Sender 实现)
```

### 序列图：一条超温读数（推送放行）

```
 client    httpapi         service          store     slog      Throttled          DingTalk         钉钉
   |  POST    |               |               |         |           |                  |               |
   |--------->| Report(ctx5s) |               |         |           |                  |               |
   |          |-------------->| AddReading    |         |           |                  |               |
   |          |               |-------------->|         |           |                  |               |
   |          |               |<---- ok ------|         |           |                  |               |
   |          |               | Warn(alarm)   |         |           |                  |               |
   |          |               |------------------------>|           |                  |               |
   |          |               | Notify(alarm) |         |           |                  |               |
   |          |               |------------------------------------>| lock; allow?     |               |
   |          |               |               |         |           | yes, last=now    |               |
   |          |               |               |         |           | unlock; go ----->| Send(bg ctx 5s)
   |          |               |<----------------- 立即返回 ----------|                  | POST json     |
   |          |<-- reading ---|               |         |           |                  |-------------->|
   |<-- 201 --|               |               |         |           |                  |<- 200 errcode |
   |          |               |               |         |<----------------------------- Info sent /    |
   |          |               |               |         |           |                  | Error failed  |
```

### 序列图：同一设备两条超温读数并发（TG-26 竞态）

```
 req A (55.0)                 Throttled (mutex)                 req B (56.0)
     | Notify(sensor-01) -------> lock                                |
     |                            last 不存在 -> 放行, last=t0        |
     |                            unlock                              |
     |                                    <------- Notify(sensor-01)  |
     |                            lock                                |
     |                            t0 距今 < 冷却 -> 抑制 (DEBUG)       |
     |                            unlock                              |
     v                                                                v
  结果: 只有 A 触发一次 Send; A、B 各自的 WARN 日志照写 (ALM-04)
```

### 决策图：是否推送（TG-12）

```
 Report 成功 && temp > threshold ?
   |no --> 结束 (无日志, 无推送)
   |yes
   v
 写 WARN 日志 (ALM-01)
   v
 notifier 是 Nop (未配置 webhook) ? --yes--> 结束
   |no
   v
 lock; 该设备 last 存在且 now-last < cooldown ? --yes--> unlock, DEBUG suppressed, 结束
   |no
   v
 last = now; unlock; go Send
   v
 HTTP 2xx 且 errcode == 0 ? --yes--> INFO "alarm notification sent"
   |no (含网络错 / 超时 / 非 2xx / JSON 解析失败 / errcode != 0)
   v
 ERROR "alarm notification failed" (不重试; last 不回滚)
```

## Component List

| 组件 | 职责 | 技术 | 部署 |
|---|---|---|---|
| `internal/notify` 包：`Alarm`、`Notifier` 接口、`Nop` | 定义推送边界；`Nop` 用于未配置与测试 | Go 标准库 | 编进服务二进制 |
| `notify.Throttled` | 按设备冷却、启动推送 goroutine、记成功/失败日志 | `sync.Mutex` + `map[string]time.Time`，可注入时钟 | 同上 |
| `notify.DingTalk` | 组装钉钉 text 消息、加签、发请求、判定成功 | `net/http`、`crypto/hmac`、`crypto/sha256`、`encoding/base64`、`encoding/json` | 同上 |
| `internal/config`（改） | 新增 3 个键、校验、导出有序键列表 `Keys` | Go | 同上 |
| `internal/redact`（改） | 额外密钥全文替换 | Go | 同上 |
| `internal/service`（改） | 超温时调用 `Notifier.Notify` | Go | 同上 |
| `cmd/temp-monitor`（改） | 装配 notifier、启动日志、错误提示 | Go | 同上 |
| `internal/testdb`（改）[spec-review-amendment] | allowed keys 提示改由 `config.Keys` 派生 | Go | 测试辅助包 |
| `scripts/lib/local_env.sh`（改） | 脚本侧接受 3 个新键 | bash | 仓内脚本 |

## Technical Approach

以下是实现层面的选择。需求层决策及被砍候选见 Decisions 节指向的纪要。

1. **接口形状**：`type Notifier interface { Notify(a Alarm) }`，无返回值、不阻塞。`Alarm{DeviceID string; TemperatureC, ThresholdC float64; RecordedAt time.Time}`，`RecordedAt` 取 `store.Reading.RecordedAt`。发送方单独定义 `type Sender interface { Send(ctx context.Context, a Alarm) error }`，`DingTalk` 实现它，`Throttled` 持有它。备选：`Notify` 返回 error 由 service 处理——否决，service 拿到 error 也只能写日志，且会诱导同步调用。
   构造函数（T3/T4 共用，main 装配与 5.1 端到端用例都经由它们）[spec-review-amendment]：`notify.NewDingTalk(webhook, secret string, cooldown time.Duration) *DingTalk`（`cooldown` 只用于正文末行）；`notify.NewThrottled(s Sender, cooldown time.Duration, logger *slog.Logger) *Throttled`。main 用同一个 `cfg.NotifyCooldown` 调这两个构造函数。`logger` MUST 是 main 构造的脱敏 logger，`notify` 包内 MUST NOT 使用 `slog.Default()`。包内可改字段：`Throttled.timeout`（缺省 5 秒）、`Throttled.now`、`DingTalk.now`。
2. **`service.New` 加第四个参数 `n notify.Notifier`**，4 个调用点同步改，测试传 `notify.Nop{}` 或记录型假实现。`Service` 自身仍无可变状态，冷却状态在 `Throttled`；`service.go:24` 的注释保持成立。备选：函数式 option——否决，只有一个新依赖，加 option 机制是加宽。
3. **冷却放在 `Throttled` 而不是 `Service`**：冷却只服务于推送，日志不受冷却影响（ALM-04）。冷却在**派发时**记录 `last = now`，推送失败不回滚。理由：失败多为网络或限流，立刻重放只会加重限流；持续超温会在下个冷却期重推（纪要 D4/D5）。
4. **并发**：`sync.Mutex` 保护 `map[string]time.Time`，判断与写入在同一临界区（GO-01、GO-03）。备选 `sync.Map` + `LoadOrStore`——否决，「过期才覆盖」需要比较后写，`sync.Map` 写不成原子的「判断 + 覆盖」，还得再加锁。
5. **goroutine 生命周期**（GO-02）：每次放行启动一个 goroutine，`ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)`，不继承请求 ctx（请求结束即取消）。每个 goroutine 最多存活 5 秒。`Throttled` 内含 `sync.WaitGroup`，导出 `Wait()` 仅供测试等待在途推送；`run` 停机不调用它（Non-Goals 第 1 条）。5 秒取自字段 `Throttled.timeout`，测试可改短 [spec-review-amendment]。goroutine 入口 `defer` 一个 `recover`：发生 panic 时写 ERROR `alarm notification failed`，`error` 为 `panic: <值>`，进程不退出（推送 goroutine 不在 `http.Server` 的 recover 范围内）[spec-review-amendment]。
6. **时钟注入**：`Throttled` 持有 `now func() time.Time`，构造函数缺省 `time.Now`，包内测试替换。`DingTalk` 同样持有 `now`，用于加签时间戳。
7. **加签**：配置了 secret 时，`ts := now().UnixMilli()`；原文 `fmt.Sprintf("%d\n%s", ts, secret)`；`HMAC-SHA256(key=secret, msg=原文)`；`base64.StdEncoding`；解析 webhook URL，`q := u.Query(); q.Set("timestamp", ts); q.Set("sign", sig); u.RawQuery = q.Encode()`（`Encode` 负责 URL 编码）。测试向量（独立用 Python 标准库算出）：secret `SECtest0000000000000000000000000000000000000000000000000000000000`、ts `1700000000000` ⇒ sign `szPcgtoCabDjIT69N8i2Omun8ktW8akfg42aQqadSCU=`，URL 中为 `szPcgtoCabDjIT69N8i2Omun8ktW8akfg42aQqadSCU%3D`。
8. **成功判定**：`resp.StatusCode` 在 200–299 且响应体（最多读 64 KiB）解析为 `{"errcode": int, "errmsg": string}` 且 `errcode == 0`。其余全部算失败，error 文本含 HTTP 状态码或 `errcode`/`errmsg`。[spec-review-amendment] 具体做法：
   - 解码目标的 `errcode` 字段类型为 `*int`。字段缺失（`{}`、`null`、`{"status":"ok"}`）或类型不对（`"0"`）一律判失败，不当作 0。
   - 拿到 `resp` 后立即 `defer resp.Body.Close()`。读取用 `io.LimitReader(resp.Body, 64<<10)`；超过上限导致 JSON 截断时按「响应体不是 JSON」判失败。
   - error 文本固定为以下四种，均不含 URL：
     - `dingtalk http <状态码>`
     - `dingtalk errcode <n>: <errmsg>; check that TM_DINGTALK_SECRET matches the robot's signing setting, or that its keyword is 超温告警`
     - `dingtalk response is not valid JSON`
     - `dingtalk request: <*url.Error 的 .Err>; check outbound access or HTTPS_PROXY`
9. **不打 `*url.Error` 全文**：`http.Client.Do` 返回的 `*url.Error` 的 `Error()` 含完整请求 URL（含 `access_token` 与 `sign`）。`DingTalk.Send` 用 `errors.As` 取出 `*url.Error` 后只包装其 `.Err`，再由脱敏 handler 兜底（见 Security 节）。
10. **HTTP 客户端**：`&http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}`，使用 `http.DefaultTransport`（读 `HTTPS_PROXY`），超时由 ctx 控制。不跟随重定向，3xx 按非 2xx 判失败。这样请求体不会被转发到 `Location` 指向的其它主机，POST 也不会被改成 GET [spec-review-amendment]。
11. **webhook 校验**：`url.Parse` 成功、`Host` 非空、scheme 为 `http` 或 `https`（纪要 D10；`http` 供 httptest 假服务器使用）；`url.ParseQuery(u.RawQuery)` 不报错，否则加签时 `u.Query()` 会静默丢掉 `access_token` [spec-review-amendment]。
12. **白名单单一源**：`config` 导出 `Keys = []string{KeyDatabaseURL, KeyListenAddr, KeyThreshold, KeyDingTalkWebhook, KeyDingTalkSecret, KeyNotifyCooldown}`，`knownKeys`、`main.configHint`、`testdb.configFailure` 都从它派生。shell 侧无法引用 Go 常量，加一条 Go 测试读取 `scripts/lib/local_env.sh`，断言其 `case` 分支与 allowed keys 文案都包含 `config.Keys` 的全部键（C2）。
13. **启动日志**：未配置 ⇒ `logger.Info("notify disabled")`；已配置 ⇒ `logger.Info("notify enabled", "cooldown", cfg.NotifyCooldown.String(), "signed", secret != "")`。不打印 webhook。
    **推送日志字段** [spec-review-amendment]：`notify` 包的日志只带两个字段，`device_id`（字符串）和 `error`（字符串，即 `err.Error()`），MUST NOT 携带 URL、`*http.Request`、`*http.Response` 或其它结构体。`error` 用字符串属性而不用 error 值，原因是 `redact` 的口令规则：DB 连接串里提取不出口令时，所有 error 值都会被整体替换为 `error details withheld`（`redact.go:127-130`），NTF-05 要求的状态码与 errcode 就丢了。推送错误文本不可能含 DB 口令，作为字符串属性仍会经过连接串与额外密钥的全文替换。
    **命名** [spec-review-amendment]：渠道相关的两个键带 `DINGTALK`；冷却与渠道无关，用 `NOTIFY`。冷却值是 Go duration 字符串，与 `time.ParseDuration` 对齐，因此不带单位后缀。webhook 未配置时冷却键不报错，而 secret 报错：`config/example.env` 自带 `TM_NOTIFY_COOLDOWN=10m`，复制出的 local.env 不该因此启动失败；单独配了 secret 则是配置错误。
14. **消息正文**（`msgtype: text`）：

    ```
    超温告警
    设备：sensor-01
    温度：55°C（阈值 50°C）
    时间：2026-10-17T11:00:00Z
    该设备 10m0s 内不再重复推送
    ```

    数值用 `strconv.FormatFloat(v, 'f', -1, 64)`，时间用 `RecordedAt.UTC().Format(time.RFC3339)`（pgstore 返回的 `timestamptz` 带本地时区，必须显式 `.UTC()`）[spec-review-amendment]，冷却用 `time.Duration.String()`。

## Data Model & Lifecycle

| 对象 | 字段（类型 / 约束） | 创建 | 更新 | 删除 |
|---|---|---|---|---|
| `config.Config` 新字段 | `DingTalkWebhook string`（空 = 不推送；非空须通过 Technical Approach 第 11 条校验）；`DingTalkSecret string`（可空；非空时 webhook 必须非空）；`NotifyCooldown time.Duration`（缺省 `10 * time.Minute`；须 > 0） | 启动时 `Load` | 不更新 | 进程退出 |
| `notify.Alarm` | `DeviceID string`（已通过 `IDPattern`）；`TemperatureC float64`；`ThresholdC float64`；`RecordedAt time.Time` | 每条放行前的超温读数 | 不可变 | 推送 goroutine 结束 |
| `Throttled` 冷却表 | `map[string]time.Time`，键为设备 ID，值为最近一次派发时刻 | 设备首次放行 | 每次放行覆盖 | 不删除；进程退出时整体消失 |

不涉及数据库，`db/migrations` 不变。

## External Dependency: DingTalk Robot（TG-08 / D-4）

- **超时**：单次推送上限 5 秒（ctx 超时，包含连接、TLS、发送、读响应）。
- **写操作回滚路径**：消息一经钉钉接受无法撤回；不重试，所以不会产生重复消息。功能回滚 = 把 `TM_DINGTALK_WEBHOOK` 从环境变量和 `config/local.env` 两处都删掉后重启（即刻停推），或回退本 change 的提交。只把环境变量设为空串不够：空值等同未配置，会回落到 `config/local.env` 里的值（RUN-01）[spec-review-amendment]。

请求示例（加签时 URL 附 `&timestamp=...&sign=...`）：

```json
POST <TM_DINGTALK_WEBHOOK>[&timestamp=1700000000000&sign=szPcgtoCabDjIT69N8i2Omun8ktW8akfg42aQqadSCU%3D]
Content-Type: application/json

{
  "msgtype": "text",
  "text": { "content": "超温告警\n设备：sensor-01\n温度：55°C（阈值 50°C）\n时间：2026-10-17T11:00:00Z\n该设备 10m0s 内不再重复推送" },
  "at": { "isAtAll": true }
}
```

成功响应：

```json
{ "errcode": 0, "errmsg": "ok" }
```

失败响应的形状（`errcode` 非 0，`errmsg` 为钉钉给出的原因；具体错误码未核实，见 proposal 假设 A1，实现与测试不依赖具体错误码）：

```
{ "errcode": <非 0 整数>, "errmsg": "<原因>" }
```

## Failure Modes

| 失败 | 触发条件 | 处理者 | 可见行为 | 测试 |
|---|---|---|---|---|
| 网络不可达 / DNS 失败 | 出网不通、无代理 | `DingTalk.Send` 返回 error，`Throttled` 记日志 | ERROR `alarm notification failed`，含 `device_id`、脱敏后的 `error`；上报 201 | 单元：指向已关闭的 httptest 地址 |
| 超时 | 5 秒内未收到完整响应 | 同上（ctx deadline） | 同上；goroutine 5 秒内结束 | 单元：httptest 挂起处理函数 + 注入短超时 |
| HTTP 非 2xx | 钉钉返回 4xx/5xx | 同上 | ERROR，error 含状态码 | 单元 |
| `errcode != 0` | 加签不符、关键词不含、限流、token 失效 | 同上 | ERROR，error 含 `errcode` 与 `errmsg` | 单元：httptest 回 200 + `{"errcode":1,"errmsg":"x"}` |
| 响应体不是 JSON | 代理或网关返回 HTML | 同上 | ERROR | 单元 |
| 停机时在途 | SIGTERM 时 goroutine 未结束 | 不处理 | 该条丢失，无日志 | 不测（接受的边角） |
| 重定向 [spec-review-amendment] | 钉钉、代理或网关回 3xx | `DingTalk.Send`（不跟随） | ERROR，error 含状态码 | 单元：httptest 回 302 |
| 2xx 但无 `errcode` [spec-review-amendment] | 网关回 200 + `{}` / `{"status":"ok"}` | `DingTalk.Send` | ERROR，不写 `alarm notification sent` | 单元：httptest 回 200 + `{}` |
| 推送 goroutine panic [spec-review-amendment] | Sender 或消息组装出现未预期的 panic | `Throttled` 的 recover | ERROR `alarm notification failed`，`error` 以 `panic:` 开头；进程不退出 | 单元：假 Sender panic |
| DB 连接串无口令 [spec-review-amendment] | `TM_DATABASE_URL` 走 pgpass / trust，不含口令 | `Throttled` 以字符串属性记 error | ERROR 的 `error` 仍含状态码 / errcode，不被替换为 `error details withheld` | 单元：main，无口令连接串 + 假钉钉 502 |
| 冷却内 | 同设备冷却期内再次超温 | `Throttled` | 不推送；写 DEBUG `alarm notification suppressed`，服务默认级别 INFO 下不输出（预期行为）[spec-review-amendment] | 单元：注入时钟 + Debug 级 handler |
| 配置非法 | webhook 格式错、冷却非法、secret 无 webhook | `config.Load` | 启动失败退出码 1，stderr 只含键名 + 提示 | 单元：config、main |

## Observability

| 事件 | 级别 | 消息 | 字段 |
|---|---|---|---|
| 启动，未配置 | INFO | `notify disabled` | — |
| 启动，已配置 | INFO | `notify enabled` | `cooldown`、`signed` |
| 推送成功 | INFO | `alarm notification sent` | `device_id` |
| 推送失败 | ERROR | `alarm notification failed` | `device_id`、`error` |
| 冷却抑制 | DEBUG | `alarm notification suppressed` | `device_id` |

不加指标，项目没有指标系统。所有日志经脱敏 handler。

## Non-Functional Requirements

| 项 | 数字 |
|---|---|
| 单次推送上限 | 5 秒 |
| 钉钉挂起时上报耗时 | < 1 秒（与无推送时同量级） |
| 同设备推送频率 | 每冷却期 ≤ 1 条，缺省冷却 10 分钟 |
| 并发推送 goroutine | ≤ 5 秒内放行的设备数；每个 ≤ 5 秒 |

## Security & Data Protection（TG-17）

- **信任边界**：服务 → 钉钉公网接口。出站数据只有设备 ID、温度、阈值、时间，不含连接信息。
- **密钥**：`TM_DINGTALK_WEBHOOK`（含 `access_token`）与 `TM_DINGTALK_SECRET` 只从环境变量或 `config/local.env` 读；`config/example.env` 两项留空。
- **不落日志**：`redact.Handler` 签名扩为 `Handler(inner slog.Handler, connURL string, secrets ...string) slog.Handler`，现有调用不需改。`main` 传入 webhook 全串、从中解析出的 `access_token` 值、secret 值（空串忽略）。这些密钥在**消息、字符串属性、分组成员、error 值**中一律全文替换为 `***`。口令规则（只替换 error 值）不变。经 `WithAttrs` / `WithGroup` 派生的 handler MUST 带上同一组 secrets（`redact.go:96`、`:100` 现在是按字段字面量重建 handler，新增字段时两处都要改）[spec-review-amendment]。
  - 加签后实际发出的请求 URL 与配置的 `TM_DINGTALK_WEBHOOK` 全串不同：`q.Encode()` 会重排并重新编码查询参数，还会追加 `timestamp`/`sign`。对请求 URL 起保护作用的是 `access_token` 值的替换和 `*url.Error` 只取 `.Err` 这两条，全串替换只兜住配置值本身 [spec-review-amendment]。
  - 非 error 的 `KindAny` 值（`*url.URL`、`*http.Request`、`fmt.Stringer` 等）会原样通过 `redact`（`redact.go:135`）。这一面靠 Technical Approach 第 13 条的推送日志字段约束兜住，不扩展 redact [spec-review-amendment]。
  - 为什么额外密钥可以全文替换，而口令不行：issue B1 的误伤来自短而常见的口令；webhook 全串与 `access_token` 是长随机串，secret 是 `SEC` 开头的长串，不会是普通文本的子串。若人工配置了很短的 secret，宁可误伤也不泄露。
- **明文传输**：钉钉机器人地址本身是 https；配置校验接受 http 以便本机测试（纪要 D10），配置成公网 http 地址时 token 会明文传输，由配置者负责。
- **配置报错**：沿用不回显值的约定，只给键名与行号。
- **最小权限**：钉钉机器人只能往它所在的群发消息，服务不持有其它钉钉凭据。

## Decisions

本 change 的决策全文与砍掉的候选见 [`decision-memo.md`](./decision-memo.md)。

## Risks / Trade-offs

- [钉钉接口语义只核到二手资料（proposal A1）] → 成功判定对两种失败形态都不误判；tasks 末条人工真机冒烟（加签、关键词各一次）。
- [多设备同时超温撞限流] → 接受（纪要 D7）；ERROR 日志可见；冷却可调大。
- [停机或失败漏推一条] → 接受（纪要 D5）；持续超温下个冷却期重推。
- [`service.New` 签名变化影响 4 个调用点] → 全在仓内，同一提交里一起改。
- [额外密钥全文替换可能误伤极短 secret] → 接受，泄露代价更高。
- [冷却表永不清理] → 见 Non-Goals 第 2 条的假设。

## Migration Plan

1. 部署新版本。不配置 `TM_DINGTALK_WEBHOOK` 时行为与旧版一致，只多一行 `notify disabled` 日志。
2. 值班群添加自定义机器人，安全设置选「加签」（推荐）或关键词「超温告警」。
3. 在 `config/local.env` 或环境变量填 `TM_DINGTALK_WEBHOOK`（加签时再填 `TM_DINGTALK_SECRET`），重启服务。
4. 回滚：从环境变量和 `config/local.env` 两处删掉 `TM_DINGTALK_WEBHOOK` 并重启（只把环境变量置空会回落到 local.env 的值）[spec-review-amendment]；或回退提交。无数据迁移。

## Compliance

- **ADR-0005（脱敏用包装 slog.Handler，`httpapi.NewHandler` 签名不变）**：遵守。`NewHandler` 不变；`redact.Handler` 以变参扩展，现有调用不改；口令替换规则不变。额外密钥的全文替换是新增规则，不改 ADR-0005 已定的口令语义。归档时由 `sdflow-adr sync` 判断是否需要在 ADR-0005 追加修订记录。
- **ADR-0003（连真库层：共用契约套件 + 一条端到端用例）**：遵守。端到端用例增加「httptest 假钉钉收到一次推送」的断言，不新增连真库用例。
- **ADR-0001 / 0002 / 0004**：不涉及。
- **RUN-01（服务与脚本同一文法）**：遵守，`scripts/lib/local_env.sh` 同步扩键，并加 Go 测试防漂移。
- **RUN-04（连接信息不出现在输出中）**：扩展到推送密钥，见 Security 节。
- **项目 context「超温告警写日志、不加表外功能」**：显式豁免，理由与处理见 proposal Compliance。
- **D-6（跨模块共享数据模型）**：不命中。`notify.Alarm` 只在服务进程内使用，不跨产品、不进数据库。

## 切片建议

| 票 | 内容 | 覆盖 | Blocked-by |
|---|---|---|---|
| T1 | 配置三键 + 校验 + `config.Keys` 单一源 + main/testdb 提示 + `local_env.sh` + 防漂移测试 + `example.env`/README | RUN-01、RUN-03、RUN-05 | — |
| T2 | `redact.Handler` 变参密钥 + 全文替换 + 测试 | RUN-04 | — |
| T3 | `notify.DingTalk`：消息体、加签、成功判定、`*url.Error` 处理，httptest 测试 | NTF-01（消息形状）、NTF-04、NTF-05 | — |
| T4 | `notify.Throttled` + `Nop` + `service.New` 接入 + main 装配与启动日志 | NTF-01、NTF-02、NTF-03、NTF-06、NTF-07 | T1、T2、T3 |
| T5 | 端到端用例扩展 + `openspec/config.yaml` context + `over-temperature-alarm` Purpose + 人工真机冒烟 | 全部 NTF 的端到端证据 | T4 |

```
 T1 --+
 T2 --+--> T4 --> T5
 T3 --+
```
