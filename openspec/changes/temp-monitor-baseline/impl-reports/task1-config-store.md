# Task 1 实现记录：配置与存储契约

起点 SHA `13a725c`。范围：tasks 1.1–1.4、2.1–2.3。

## 做了什么

- `go.mod`：module `github.com/laodao-ai/demo-temp-monitor`，`go 1.27`，`go get github.com/jackc/pgx/v5` 得 v5.11.0。
- `config/example.env`：三个键为占位值，注释写明 session 池端口 7432、口令做 URL 编码。
- `internal/config/config.go`：`Load(getenv, path) (Config, error)`，文法照 design「Configuration Surface」。
- `internal/store/store.go`：`Store` 接口、`Device`、`Reading`、`ErrNotFound`、`ErrAlreadyExists`。
- `internal/store/storetest/storetest.go`：`Run(t, newStore)`，九个用例，ID 为 `it-<随机后缀>` 且 `t.Cleanup` 删除（独立 5 秒 context）。
- `internal/store/memstore`：一把 `sync.Mutex`，`ListReadings` 返回副本。

## 验收证据

| 项 | 证据 |
|---|---|
| 1.1 | `go build ./...` 通过；`go.mod` 含 `go 1.27` 与 pgx v5.11.0 |
| 1.2 | `.gitignore` 已含两行（21d7b25），未重复加。建 `config/local.env` 后 `git status --porcelain` 只显示 `config/`（里面是 example.env）、`go.mod`、`go.sum`、`impl-reports/`，没有 `local.env`；`git check-ignore -v config/local.env` 输出 `.gitignore:12:config/local.env` |
| 1.3 | `config/example.env` 内容 |
| 1.4 | `internal/config` 测试：`TestLoadPriority`（环境变量>文件>缺省、空环境变量视为未设）、`TestLoadFileMissingIsFine`、`TestLoadMissingDatabaseURL`、`TestLoadInvalidThreshold`（abc/NaN/nan/Inf/+Inf/-Inf/inf/Infinity/1e999）、`TestLoadFileGrammar`（注释与空行、值含 `=` 与 `&`、两端空白、CRLF、空值、`$(touch marker)` 不展开、文件里的阈值同样校验）、`TestLoadFileErrors`（无 `=`、未知键、`export` 前缀、双引号、单引号；断言含文件名和行号，且不含行内容） |
| 2.1 | `internal/store/store.go` |
| 2.2 | `storetest.Run` 九项：CreateDevice、CreateDuplicate、DeleteCascadesReadings、DeleteMissing、AddReadingMissingDevice、ReadingsInWriteOrder、ConcurrentCreateSameID(20)、ConcurrentAddReading(20)、ConcurrentDeleteAndReport(20 上报 + 1 注销，25 轮) |
| 2.3 | `memstore.TestContract` 九个子测试全 PASS，`-race` 下 |

红绿过程：config 先用桩实现跑出全红再实现；memstore 先用返回 `unimplemented` 的桩跑出九项全红再实现。

变异检查（当场破坏再恢复）：
- 去掉 NaN/Inf 判断：`TestLoadInvalidThreshold` 各 NaN/Inf 子测试与 `threshold_from_file_validated` 失败。
- memstore 注销不删读数：`DeleteCascadesReadings`、`ConcurrentDeleteAndReport` 失败。
- memstore 创建不判重：`CreateDuplicate`、`ConcurrentCreateSameID` 失败。
- 去掉 memstore 全部锁：`ConcurrentAddReading`、`ConcurrentCreateSameID`、`ConcurrentDeleteAndReport` 失败。
- 去掉 `TrimSuffix(line, "\r")` 不会使 CRLF 用例失败，因为键值本来就 `TrimSpace`；该行是冗余的，保留（行为正确，不影响结果）。

## 命令输出（尾部）

`go test -race -count=1 ./...`：

```
ok  	github.com/laodao-ai/demo-temp-monitor/internal/config	1.232s
?   	github.com/laodao-ai/demo-temp-monitor/internal/store	[no test files]
ok  	github.com/laodao-ai/demo-temp-monitor/internal/store/memstore	1.223s
?   	github.com/laodao-ai/demo-temp-monitor/internal/store/storetest	[no test files]
```

`go vet ./...`：无输出，退出 0。`gofmt -l .`：无输出。

## Concerns

- `go.mod` 里 pgx 目前标 `// indirect`，因为本票没有代码 import 它。后续 pgstore 票 import 后，`go mod tidy` 会改成直接依赖。本票没有跑 `go mod tidy`，否则 require 会被删掉。
- `ListReadings` 对不存在的设备返回空切片，不是 `ErrNotFound`。接口注释与设计都没规定，契约测试依赖这点（注销后查读数为 0 条）。pgstore 需保持一致。
- 契约套件的"注销与上报并发"在两个实现上都跑 25 轮，不只在 pgstore 上。

## 票外发现

无。
