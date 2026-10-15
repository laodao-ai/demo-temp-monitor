# Task 2 fix 第 2 轮

## 改动
- `internal/httpapi/httpapi.go`：5xx ERROR 日志改传 error 值（`"error", err`），redact 的 error 分支生效；注释同步。
- `internal/redact/redact.go`：`attr` 加注释，说明只处理字符串、组与 error 值。
- `internal/httpapi/httpapi_test.go`：新增 `newEnvConn`（可指定连接串），新增用例
  `TestStoreErrorWithoutExtractablePasswordIsWithheldInLog`（连接串 `postgres://user@h/db`）。

## 红 -> 绿
- 修复前：该用例失败，日志 `error` 属性为底层错误原文（`boom: dial postgres://...`），期望 `error details withheld`。
- 修复后：通过。期望文案是字面量，不由实现重算。

## 命令
- `go vet ./...` 无输出；`go test -race -count=1 ./...` 全部 ok（config、httpapi、redact、service、memstore）。

## 编排层记录

- 票外处置：Task 2 Spec 轴复审指出「main 须用脱敏 slog 包装注入 logger」，判 fold（同 capability、高耦合、低增量），追加为 Task 3 验收项。
- Minor 共 9 条 defer 进 todo 池 T7–T15（source_change=temp-monitor-baseline）。
- 双轴审结论：Standards 第 3 轮 PASS（fix2）；Spec 第 2 轮 PASS（fix1）。fix2 只把 5xx 日志改为传 error 值并补一条测试，Spec 轴已核的行为未变，未再派 Spec 复审。
