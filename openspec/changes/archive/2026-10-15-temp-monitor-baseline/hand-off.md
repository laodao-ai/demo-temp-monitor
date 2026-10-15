# Hand-off：temp-monitor-baseline

- 日期：2026-10-15
- verify：PASS（无核心缺口，4 条 Minor 缺口），见 `verify-report.md`
- 收尾 HEAD：`260d3ba`

## ✅ 完成了什么

以下各条的锚点已在收尾时复核存在（测试函数 grep 命中、SHA 可 `git cat-file`、迁移行可读）。

- **设备注册 / 注销 / 重新注册**（DEV-01..04）：`internal/httpapi/httpapi.go`、`internal/service/service.go`；测试 `TestRegisterCreatesDevice`、`TestRegisterDuplicateKeepsReadings`、`TestDeleteRemovesDeviceAndReadings`、`TestReRegisterAfterDeleteHasNoReadings`；并发注册由契约用例 `testConcurrentCreate` 覆盖两种存储。
- **读数上报与校验**（ING-01..03）：`TestReportStoresReading`、`TestConcurrentDeleteAndReportLeavesNoReadings`、`TestReportBodyOneByteOverLimitIsRejected` 等；契约用例 `testConcurrentDeleteAndReport`。
- **模拟上报脚本**（ING-04）：`scripts/simulate.sh`；真服务验收见 `impl-reports/task6-remote-acceptance.md` 6.5。
- **超温告警**（ALM-01..04）：`TestAlarmOverThreshold`、`TestNoAlarmAtOrBelowThreshold`、`TestNoAlarmWhenStoreFails`、`TestEveryOverTemperatureReadingAlarms`。
- **配置、启动、脱敏、优雅退出**（RUN-01..06）：`internal/config`、`internal/redact`、`cmd/temp-monitor`；`TestLoadFileGrammar`、`TestStartupFailuresBeforeDatabase`、`TestUnreachableDatabaseNeverPrintsPassword`、`TestGracefulShutdown`、`TestShutdownTimeoutCancelsInFlightRequest`。
- **数据库脚本与约束**（DB-01..05）：`db/migrations/0001_init.sql:3,9`（CHECK、`ON DELETE CASCADE`）、`scripts/db-reset.sh`；`TestIntegrationConstraints`；回滚等场景见 task6 6.3。
- **测试入口与契约套件**（TST-01..09）：`scripts/test.sh`；`TestContract`（memstore）、`TestIntegrationContract`（pgstore）、`TestIntegrationEndToEnd`；`internal`、`cmd` 下无 `t.Skip`。
- **实现期聚合覆盖**：`impl-reports/task7-verify.md`，unit 与 integration 两层均锚 `181b2d9`。含义是实现期结束时命中域内套件通过，不代表最终代码通过全量回归。
- 收尾时 verify 在 `260d3ba` 上重跑 `scripts/test.sh unit`：146 passed，退出 0。

## ⏳ 未完成 / 延后

### 本 change 记入 issues 池的项（`openspec/issues/open/{ID}.md`）

- bug：B1（短口令或常见子串口令被替换进固定日志文案，如 `temp` 让告警变成 `over-***erature alarm`）
- todo：T1、T2、T4、T5、T6、T7、T8、T9、T10、T11、T12、T13、T14、T15、T16、T17、T18、T19、T20、T21、T22、T23、T24、T25、T26、T27、T28、T29、T30、T31、T32

### 延后的多方案决策

- **B1 的修法**：两种方案是「只脱敏 error 和驱动文本」或「短口令退化处理」。代码审没有客观判据，涉及设计取舍，所以 defer，未自动选。口令由服务器随机生成，触发概率低。
- 实现期唯一一次 `T10-choice`（Task 2 的 5xx 日志脱敏）已按客观判据定为「脱敏 slog.Handler 包装」，记在 `impl-reports/planning-decisions.md`，不算拿不准的项。

### verify 的 Minor 缺口

1. `scripts/db-reset.sh:50` 的 `printf` 遇到非法 `%` 转义的口令会多打一行 bash 警告。不泄露口令，退出码仍是 4，但「只输出固定文案」不再严格成立。已在 T32 里。
2. 连真库层没有在最终 HEAD 上重跑：收尾时 SSH 隧道没起，`scripts/test.sh integration` 是 0 passed、3 failed。`181b2d9` 之后没改连真库层的代码，但 `d0b6fb4` 给 `db-reset.sh` 加的分支没在真库上重跑过 6.3。
3. `scripts/test.sh:83-90` 的 jq 解析失败分支没有实测。
4. 代码审 defer 项已入池（B1、T27–T31）。

## ▶ 下一阶段建议

- **先在真库上补跑一次**：起隧道后跑 `scripts/test.sh all` 和 `scripts/db-reset.sh`，补上 Minor 缺口 2。不需要开 change。
- **开一个加固 change，优先级高**，一起清：
  - B1（先定修法）
  - T32 里的 `db-reset.sh` printf 告警，以及 T25、T26（db-reset 的 psql `-w` 和连接错误文案）
  - T27（http.Server 读请求体无时限，属设计调整）
  - T28（`.claudeignore` 和 deny 规则未覆盖 `config/local.env`）
  - T23（test.sh 失败输出可能带连接串）
- **再开一个代码质量清理 change，优先级低**：T1–T22、T24、T29–T31 中的重复代码、doc comment、错误包装和测试整理。
- T15（design.md Component List 缺 `internal/redact`）是文档项，本 change 归档时未补，可并入上面任一 change。
- roadmap：未检测到 roadmap 关联标记（本仓无 `openspec/roadmaps/`），不产回填草稿。

ADR 同步跳过：本仓无 `openspec/adr/`，`adr.py refs` 退出 2。
