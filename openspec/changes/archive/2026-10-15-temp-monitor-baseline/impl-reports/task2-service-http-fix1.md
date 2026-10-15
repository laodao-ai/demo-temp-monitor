# Task 2 fix1：5xx ERROR 日志脱敏

## 改了什么
- 新增 `internal/redact`：
  - `Text(text, connURL) (string, bool)`：先替换连接串整体，再替换口令（scheme 后第一个 `:` 到最后一个 `@` 的原始子串）为 `***`。口令提取不出时 ok=false。
  - `Handler(inner, connURL) slog.Handler`：对 message、字符串属性、error 值、group 成员、`With` 预置属性做同样替换。口令提取不出时，error 值改为固定文案 `error details withheld`。
- `internal/httpapi/httpapi.go`：去掉「Stores must return errors that are already free of connection details」注释，改为说明脱敏由注入的 logger 负责，main 在 Task 3 接入。签名未改。
- `internal/httpapi/httpapi_test.go`：测试 logger 用 `redact.Handler` 包装；存储替身返回含连接串与口令 `s3cret-xyz` 的错误；断言 500、固定响应体、ERROR 日志存在、日志全文不含口令与连接串；删掉断言日志含 "boom" 的那条。

## 新增 / 修改的测试
- `TestTextRemovesPasswordAndFragments`（口令 `p(ss;w`、`p&ss`、`p%zz`、`p@ss#w`）
- `TestTextReportsUnextractablePassword`
- `TestHandlerRedactsMessageAttrsErrorsAndGroups`
- `TestHandlerWithholdsErrorTextWhenPasswordUnextractable`
- 修改 `TestStoreErrorsGive500WithFixedBodyAndErrorLog`

## 红 -> 绿证据（当场破坏被测点，已恢复）
- 不替换口令：redact 的 Text、Handler 两个测试和 httpapi 的 `TestStoreErrorsGive500WithFixedBodyAndErrorLog` 变红。
- 不处理 error 值：`TestHandlerRedactsMessageAttrsErrorsAndGroups`、`TestHandlerWithholdsErrorTextWhenPasswordUnextractable` 变红。
- 不替换连接串整体：`TestTextRemovesPasswordAndFragments`、`TestTextReportsUnextractablePassword` 变红。

## 命令输出尾部
`go vet ./...`：无输出。
`go test -race -count=1 ./...`：config、httpapi、redact、service、memstore 均 ok。

## 未做
- 未接入 main（Task 3）。
