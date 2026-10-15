# 连接信息脱敏用包装 slog.Handler 实现，NewHandler 签名不变

**Status: Accepted** · 来源 change：`temp-monitor-baseline`（2026-10-15）

数据库驱动的错误文本可能带出连接串或口令。这类文本会进入 5xx 日志和启动输出。

`httpapi.NewHandler(svc, logger)` 的签名已在设计中固定。脱敏逻辑要在不改它的前提下接入。

## Decision

- 新建 `internal/redact` 包，提供 `Handler(inner slog.Handler, connURL string) slog.Handler`。它包装任意 slog handler。
- 口令只可能来自驱动错误。包装 handler 只对 error 值调用 `redact.Text`，把整条连接串和口令（含 URL 解码形态）替换成 `***`。
- 消息和字符串属性只替换整条连接串，不替换口令。分组成员按同样规则递归。
- 取不出口令时，error 值整体替换为 `error details withheld`。
- `cmd/temp-monitor/main.go` 用 `slog.New(redact.Handler(slog.NewTextHandler(stderr, nil), cfg.DatabaseURL))` 构造 logger，再传给 `httpapi.NewHandler(svc, logger)`。
- 启动输出里不经 logger 的错误文本，由 main 直接调用 `redact.Text`。`internal/testdb` 同样复用它。

## Considered Options

- **给 `NewHandler` 加脱敏函数参数**：否决。要改已定接口，脱敏要在每个调用点传一次。
- **在每处写日志的地方手工调用脱敏**：否决。漏一处就泄露，且无法覆盖第三方写入的字段。
- **包装 slog.Handler（采用）**：脱敏在一处实现，调用方无感知。

## Consequences

- 系统镜：接口不变，脱敏逻辑集中在 `internal/redact`，main 与 testdb 复用。仅处理字符串、分组、error 值，其他类型的属性原样通过。
- 用户镜：日志里连接信息显示为 `***`，其余内容不变。
- 已知边界：若有调用方把 `err.Error()` 当字符串属性记日志，其中的口令片段不会被替换。调用方须传 error 值，见 `internal/httpapi/httpapi.go` 中 `request failed` 处的注释。
- 开发循环镜：新增日志调用无需记得脱敏。新增属性类型时要确认 `attr` 是否覆盖。
- 主次：系统镜为主，不改已定接口优先。

## 附录：修订历史

- **2026-10-15**（issue B1）：口令替换范围由「消息、字符串属性、分组成员和 error 值」缩到「仅 error 值」，消息和字符串属性只替换整条连接串。原因：口令为 `temp` 这类常见子串时，原规则把 `over-temperature alarm` 改成 `over-***erature alarm`。证据：`internal/redact/redact.go:104,117,127`，测试 `TestHandlerKeepsMessageAndStringAttrsWhenPasswordIsCommonWord`（`internal/redact/redact_test.go:121`）。
