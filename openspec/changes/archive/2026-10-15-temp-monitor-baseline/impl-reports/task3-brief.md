### Task 3: PostgreSQL 实现、服务进程与连真库测试

**Blocked-by:** 1, 2
**R-ID:** DB-04, DB-05, DEV-01, DEV-04, ING-02, RUN-02, RUN-03, RUN-04, RUN-06, TST-06, TST-07, TST-08, TST-09, ALM-01

服务可以作为进程启动：读配置、连 PostgreSQL、检查表存在、监听回环地址，收到 SIGINT / SIGTERM 时优雅退出。任一前置条件失败就退出 1，stderr 文案按 design「Error Messages」，不含连接串和口令片段。PostgreSQL 存储实现跑同一套契约测试；连真库测试缺配置、连不上、缺表时失败并给出下一步，不跳过。迁移、删表、种子 SQL 随仓提交。

- [ ] tasks 2.4：迁移、删表、种子 SQL 按 design「Data Model & Lifecycle」，种子含 `demo-01`..`demo-03` 且至少一条读数 > 50.0
- [ ] tasks 2.5：PostgreSQL 存储实现，23505 / 23503 映射为哨兵错误，提供表存在检查
- [ ] tasks 2.6：连真库测试的公共准备（从 `go.mod` 定位仓根、读配置、5 秒建连、查表；失败一律 `t.Fatal` 并给出下一步；全文无 `t.Skip`；池 `MaxConns=4` 并在 Cleanup 关闭）
- [ ] tasks 2.7：PostgreSQL 实现的连真库契约测试（`//go:build integration`，函数名 `TestIntegration` 开头），另测 DB-05 的检查约束与级联删除，`it-` 设备在 Cleanup 中删除
- [ ] tasks 3.3：服务进程，含该条列出的全部要求（`MaxConns=4`、5 秒 ping、表检查、启动 INFO、`ReadHeaderTimeout`、`Shutdown` 5 秒与超时 WARN、`Serve` 非预期错误退出 1、错误文本脱敏）
- [ ] tasks 3.4：端到端连真库测试（`httptest.Server` + HTTP 层 + PostgreSQL 实现 + 缓冲区 logger），按 TST-08 的五步验证，`it-` 前缀并清理
- [ ] 服务进程用 Task 2 新增的脱敏 slog 包装（以实际连接串构造）包住全部日志输出，HTTP 层 5xx 日志因此脱敏；端到端连真库测试的缓冲区 logger 同样经该包装（执行期 fold 自 Task 2 Spec 轴复审：同 capability、高耦合、低增量）
- [ ] `go vet -tags integration ./...` 通过
- [ ] tasks 3.5：手动验证不需要数据库的启动失败路径（缺 URL、阈值 `abc`、无人监听端口加口令 `s3cret-xyz`，以及四个未编码特殊字符口令），每种都退出 1、stderr 不含口令及其片段，输出贴进本票报告

