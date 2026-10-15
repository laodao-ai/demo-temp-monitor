---
ship-gate:
  verify: PASS
  reviewed_sha: 891c06e4a81a673543b12e4853d819f9493fddaec5d91d5b96af08faa8faf306
  reviewed_manifest: MDQwMDAwIHRyZWUgYjY5NjdhN2E3ZjZkYjNmMDNkOTQyMjdhNzZjOTY3ZjAzOGQxMTk0MAkuYWdlbnRzADA0MDAwMCB0cmVlIDQ4MWE5YzRlZDg1NjY0YWM5Y2Q4ZWI2ZjY2ODQ1MjNiMThmNzg5N2EJLmNsYXVkZQAxMDA2NDQgYmxvYiA5YzU0NjMyMGU5NGE4NjY5MjZhMDY0NThkNGIzNWIyMmExNzUyOWU0CS5jbGF1ZGVpZ25vcmUAMTAwNjQ0IGJsb2IgZDc5ZmQ1OTI1Yzg3YzYwOTU2YWNmNGVhZjQyNDZmNDEyZDA2ZjllZAkuZ2l0aWdub3JlADEwMDY0NCBibG9iIDg3MjIwMWRmMjZlYTM4NmMwMWRjNWI4MWIwMmNjYmMyY2M3N2ExNTcJQUdFTlRTLm1kADEwMDY0NCBibG9iIDZjMWU4YzNiZTkxYTIzYmEwMzRkNGNjNzAyOTJiMGQ5MWRlMTU5YjIJQ0xBVURFLm1kADEwMDY0NCBibG9iIGFjZjMxM2Y5MjRhMWNhNmMyMTdkMmI5YzhiOTFjZGEzZTU1ODNhZDgJUkVBRE1FLm1kADA0MDAwMCB0cmVlIDE1Njk2ZGYyOTE4MjZjYmNmZTA4NDNkODBlNjUyZDY0ZDJmYmQyNzUJY21kADA0MDAwMCB0cmVlIGUzOTA1NmZkMzlkNmQ0NTFkMDc1MjBiZmQzOGFiNTJiMmJmOGQ0MzIJY29uZmlnADA0MDAwMCB0cmVlIGM0M2MxZjE1MDUzNjExYzQxYzc1ZTliN2Y1MDExNDYwMjIxOTMwYTAJZGIAMTAwNjQ0IGJsb2IgMjI0MDljMDgxNTFkNTgzOTI1NzdhZmQxOTQyYTc4NDkzMGVmZTRlMglnby5tb2QAMTAwNjQ0IGJsb2IgM2Y3MTZkZDEyNTIzYjYxOTQyMzRmMGJiMjdhNmNmOTgwZGUwODQ2YQlnby5zdW0AMDQwMDAwIHRyZWUgOWNjNDZkZjFkYTU1NGM5MjNhMGNjZDA4OTc2OWY0NmZkOWUzOGQ3NwlpbnRlcm5hbAAwNDAwMDAgdHJlZSAwNDkzM2I0ZGI4ZjMzMGIxZjBmMDJiNDAwMTE1YjE4NzQzYjExYzg0CXNjcmlwdHM=
---
# Verify 报告：temp-monitor-baseline

**结论：PASS**（无核心缺口；有 4 条 Minor 缺口，见文末）

- 日期：2026-10-15
- change：`temp-monitor-baseline`
- 核验 HEAD：`260d3baa0956a45c330feec0ff61e4e9ff796685`（分支 `feat/temp-monitor-baseline`，工作树干净）
- 方法：逐条读 specs 与 tasks.md，对照源码 / SQL / 脚本 / 测试，不以复选框或已有报告的措辞判定。

## 本次独立执行的命令

| 命令 | 结果 |
|---|---|
| `scripts/test.sh unit`（HEAD 260d3ba） | `146 passed, 0 failed, 0 skipped (146 total)`，退出 0 |
| `scripts/test.sh integration`（HEAD 260d3ba） | `0 passed, 3 failed, 0 skipped (3 total)`，退出 3。原因是本机到远端库的 SSH 隧道当前未起，stderr 为 `cannot connect to database; check the SSH tunnel ...`。**本次未能在真库上重跑连真库层**，该层证据取自实现期报告（见下表「实现期聚合覆盖」行与 6.3–6.5 行）。这次失败同时实测了 TST-06「连不上就失败、不跳过」 |
| `impl_route.py suite-scope --base f08aeab --head 181b2d9` 重跑 | 与 task7-verify.md 报告头 JSON 按键排序后逐字一致 |
| 仓库临时副本里跑 db-reset.sh / simulate.sh / test.sh 的不连库场景 | 见下表 4.1、4.4、TST-01 行 |
| `git check-ignore -v config/local.env .pg-ops/x .idea/x` | 三项分别命中 `.gitignore:12`、`:9`、`:5` |

## 逐需求核对表

| 需求/任务 | 代码出处(文件:行/迁移/测试) | 状态 |
|---|---|---|
| DEV-01 注册设备（201、`registered_at` RFC 3339、ID 正则、空体/尾随内容/超 1 MiB 回 400、未知字段忽略） | `internal/httpapi/httpapi.go:44,50-75,122-139`；`internal/service/service.go:20,35-40`；测试 `TestRegisterCreatesDevice`、`TestRegisterRejectsBadRequests`、`TestRegisterRejectsBadRequestsWithoutCreating`、`TestRegisterIgnoresUnknownFieldsAndContentType`、`TestRegisterBodyTooLarge`（`internal/httpapi/httpapi_test.go:128-216`），本次 unit 层通过 | ✅实现 |
| DEV-01 重复注册 409 且读数不变 | `httpapi.go:147-148`；`TestRegisterDuplicateKeepsReadings`（httpapi_test.go:155） | ✅实现 |
| DEV-02 注销 204 并删读数、未注册 404、非法 ID 400 | `httpapi.go:77-86`；memstore `memstore.go:37-46`；pg 由 `db/migrations/0001_init.sql:9` 的 `ON DELETE CASCADE` 保证；`TestDeleteRemovesDeviceAndReadings`、`TestDeleteUnregistered`、`TestDeleteInvalidID`（httpapi_test.go:218-242） | ✅实现 |
| DEV-03 注销后重新注册无历史读数 | `TestReRegisterAfterDeleteHasNoReadings`（httpapi_test.go:244） | ✅实现 |
| DEV-04 并发注册同 ID 只成功一次（两实现，20 并发） | `internal/store/storetest/storetest.go:166` `testConcurrentCreate`；memstore 走 `TestContract`（unit 本次通过），pgstore 走 `TestIntegrationContract`（task6 6.4 / task7 integration 通过）；pg 依赖主键 + `pgstore.go:91` 的 23505 映射 | ✅实现 |
| ING-01 上报入库（201、响应字段、0 与负数有效） | `httpapi.go:88-116`；`service.go:51-66`；`TestReportStoresReading`、`TestReportZeroAndNegativeAreValid`（httpapi_test.go:259,286） | ✅实现 |
| ING-02 未注册设备 404 不入库；注销与上报并发无残留 | `pgstore.go:114`（23503 映射）；`TestReportUnregisteredStoresNothing`、`TestConcurrentDeleteAndReportLeavesNoReadings`（httpapi_test.go:323,331）；契约 `testConcurrentDeleteAndReport`（storetest.go:225，25 轮） | ✅实现 |
| ING-03 上报校验（非法 JSON、尾随内容、空体、缺字段、非数字、路径 ID、超 1 MiB 两种构造、未知字段忽略） | `httpapi.go:90-101,122-139`；`TestReportRejectsBadRequests`、`TestReportInvalidPathID`、`TestReportBodyTooLargeSingleValue`、`TestReportBodyTooLargeTrailingWhitespace`、`TestReportBodyOneByteOverLimitIsRejected`、`TestReportIgnoresUnknownFieldsAndContentType`（httpapi_test.go:300-421,576） | ✅实现 |
| ING-04 `scripts/simulate.sh [rounds]` | `scripts/simulate.sh:18-25`（rounds 校验）、`:28-38`（地址取值与 `:` 前缀）、`:49`（`--max-time 5`）、`:80`（409 继续）、`:92-94`（温度区间）；本次实测：`0`/`abc` 退出 2，服务未运行退出 1，`config/local.env` 含 `$(touch marker)` 不产生文件；真服务场景见 `impl-reports/task6-remote-acceptance.md` 6.5（3×3 读数、3 条 WARN） | ✅实现 |
| ALM-01 超阈值写 WARN（字段、响应仍 201、阈值可配） | `service.go:59-64`；`TestAlarmOverThreshold`、`TestAlarmThresholdIsConfigurable`（service_test.go:156,175）；`TestAlarmOnlyForSuccessfulOverThresholdReports`（httpapi_test.go:424） | ✅实现 |
| ALM-02 等于/低于阈值不告警 | `service.go:59`（严格 `>`）；`TestNoAlarmAtOrBelowThreshold`（service_test.go:190） | ✅实现 |
| ALM-03 上报失败不告警 | `service.go:55-58`；`TestNoAlarmForUnregisteredDevice`、`TestNoAlarmWhenStoreFails`（service_test.go:206,216） | ✅实现 |
| ALM-04 每条超温读数各告警一次 | `TestEveryOverTemperatureReadingAlarms`（service_test.go:226） | ✅实现 |
| RUN-01 配置来源与优先级、local.env 文法（含 `=`/`&`、空值、CRLF、引号、未知键、无 `=`、不回显行内容） | `internal/config/config.go:35-102`；脚本侧 `scripts/lib/local_env.sh:22-66`；`TestLoadPriority`、`TestLoadFileGrammar`、`TestLoadFileErrors`（config_test.go:26,94,141） | ✅实现 |
| RUN-02 缺省监听 127.0.0.1:8080、阈值 50、启动日志 | `config.go:19-20,56-58`；`cmd/temp-monitor/main.go:104`；只监听回环的实测见 task6 6.5（`lsof` 仅 `127.0.0.1:8080`） | ✅实现 |
| RUN-03 前置条件失败退出 1（缺 URL、阈值非有限、文法错、连不上、缺表、端口占用） | `main.go:69-103`；`config.go:59-63`；`TestStartupFailuresBeforeDatabase`、`TestOpenFailureMessages`、`TestListenFailureExitsOne`（main_test.go:54,113,159）；缺表实测见 task6 6.3 第 1 项 | ✅实现 |
| RUN-04 连接信息不出现在输出中（含四种未编码口令、5xx 固定文案） | `internal/redact/redact.go:40-122`；`main.go:78,83-94`；`pgstore.go:32,41-53`（连不上只返回固定错误）；`httpapi.go:149-158`；`scripts/db-reset.sh:57-67,79-82,96-103`；`TestUnreachableDatabaseNeverPrintsPassword`（main_test.go:92，五种口令）、`TestStoreErrorsGive500WithFixedBodyAndErrorLog`（httpapi_test.go:458）；本次实测 db-reset 五种口令均退出 4、输出不含口令与片段 | ✅实现（另见 Minor 缺口 1） |
| RUN-05 仓内只有示例配置、gitignore | `config/example.env:5-8`；`.gitignore:5,9,12`；本次 `git check-ignore` 三项命中 | ✅实现 |
| RUN-06 SIGINT/SIGTERM 优雅退出、Shutdown 超时 WARN 后仍退 0、Serve 异常退 1 | `main.go:46,112-141`；`TestGracefulShutdown`、`TestShutdownTimeoutCancelsInFlightRequest`（main_test.go:186,255）；SIGINT/SIGTERM 真进程实测见 task6 6.5 | ✅实现 |
| DB-01 重建脚本库名守卫、退出码 0–4、带参数退 2、同一连接串 | `scripts/db-reset.sh:15-18,21-30,79-86,96`；本次实测：带参数退 2，无配置退 1，不可达端口退 4；库名不符退 3 见 task6 6.3 第 6 项 | ✅实现 |
| DB-02 一个事务内 drop → migrations → seed，重复执行结果一致 | `db-reset.sh:89-97`（`--single-transaction`、`ON_ERROR_STOP=1`、`lock_timeout`、`LC_ALL=C` 排序）；task6 6.3 第 3、5 项 | ✅实现 |
| DB-03 失败整体回滚、退 4 | `db-reset.sh:96-104`；task6 6.3 第 7 项（md5 前后一致，rc=4） | ✅实现 |
| DB-04 迁移、删表、种子随仓提交，含 demo-01..03 与 >50 读数 | `db/migrations/0001_init.sql`、`db/drop.sql:2`、`db/seed.sql:2-10`（`demo-03` 55.5） | ✅实现 |
| DB-05 数据库层约束（CHECK、FK、CASCADE） | `0001_init.sql:3,9`；`TestIntegrationConstraints` 四个子测试（`internal/store/pgstore/pgstore_integration_test.go:30-`） | ✅实现 |
| TST-01 统一入口、层选择、`-count=1` | `scripts/test.sh:14-24,80,111-116`；本次实测 `scripts/test.sh e2e` 退出 2 | ✅实现 |
| TST-02 汇总行格式与 stderr 内容 | `test.sh:57-69,87,118`；task4-test-runner.md「4.3 场景实测」表 | ✅实现 |
| TST-03 计数口径不假绿（包级失败、进程被杀、无测试文件包、子测试） | `test.sh:44-54,95-99`；task4-test-runner.md 4.3 表（编译失败、kill -9、空包、子测试四行） | ✅实现（另见 Minor 缺口 3） |
| TST-04 退出码 `min(M,125)`、零执行层退 1、缺工具退 1 | `test.sh:26-31,101-104,120-126`；task4 4.3 表（130 失败→125、3 失败→3、零用例、缺 go/jq） | ✅实现 |
| TST-05 单元层不依赖数据库、`-race` | `test.sh:112`；本次 `scripts/test.sh unit` 146 passed 退出 0；task4 6.1（无 local.env） | ✅实现 |
| TST-06 连真库层连不上就失败、不跳过、提示下一步 | `internal/testdb/testdb.go:30-60`（`grep -rn 't\.Skip' internal cmd` 无命中）；task4 6.2 两子步骤；task6 6.3 第 2 项（缺表）；本次隧道未起时实测 3 failed、0 skipped | ✅实现 |
| TST-07 两实现共用契约套件（九项） | `storetest.go:25-45`（九个用例）；`internal/store/memstore/memstore_test.go:10`；`pgstore_integration_test.go:23` | ✅实现 |
| TST-08 端到端用例五步 | `internal/httpapi/e2e_integration_test.go:28` `TestIntegrationEndToEnd`；task6 6.4 all 160 passed | ✅实现 |
| TST-09 只用 `it-` 随机 ID 并清理 | `storetest.go:48-64`；task6 6.4 后 `it-%` 计数 0；task7-verify.md「附加核对」计数 0 | ✅实现 |
| tasks 1.1–1.4 | `go.mod`；`.gitignore`；`config/example.env`；`internal/config` + 测试 | ✅实现 |
| tasks 2.1–2.7 | `internal/store/store.go`、`storetest`、`memstore`、`db/*.sql`、`pgstore`、`testdb`、`pgstore_integration_test.go` | ✅实现 |
| tasks 3.1–3.5 | `service`、`httpapi`、`cmd/temp-monitor`、`e2e_integration_test.go`；3.5 输出见 `impl-reports/task3-pg-service.md`「tasks 3.5 手动验证」，HEAD 上由 `TestStartupFailuresBeforeDatabase` / `TestUnreachableDatabaseNeverPrintsPassword` 复验 | ✅实现 |
| tasks 4.1–4.4 | 三个脚本 + `scripts/lib/local_env.sh`；4.3 见 task4-test-runner.md；4.1/4.4 见 task5-scripts-readme.md 与本次复测 | ✅实现 |
| task 5.1 README | `README.md:7-73`（范围、前置工具、测过版本 `:31`、配置、启动顺序、测试用法、无鉴权说明） | ✅实现 |
| tasks 6.1–6.2 | `impl-reports/task4-test-runner.md`「6.1」「6.2」 | ✅实现 |
| tasks 6.3–6.5（远端库） | `impl-reports/task6-remote-acceptance.md`（提交 `181b2d9` 前完成，6.3 七项、6.4 `160 passed`、6.5 全部子项） | ✅实现 |
| task 6.6 历史审计 | task6-remote-acceptance.md「6.6 全历史审计」及「编排层复核」（公司名关键词 0 命中，姓名人已核对） | ✅实现 |
| 实现期聚合覆盖 | `impl-reports/task7-verify.md`：unit / integration 两行均退出 0，均锚 SHA `181b2d94b83f54a871898b7ba830eed5c17ec935`（通过行 SHA 一致）；e2e 行为「未覆盖（本仓无此层）」，不参与 SHA 核验；报告头分域 JSON 以其 base `f08aeab` / head `181b2d9` 重跑 `suite-scope` 提交范围模式，按键排序后与报告逐字一致。锚语义：**实现期结束时命中域内聚合套件通过**，不代表最终代码通过全量回归 | ✅实现 |

## 缺口清单

### 核心缺口

无。

### Minor 缺口

1. **db-reset.sh 对含非法 `%` 转义的口令多打一行 bash 警告**（可接受，建议入 bug 池）。
   - 位置：`scripts/db-reset.sh:50`，`printf -v password_decoded '%b' "${password//%/\\x}"`。
   - 来源：代码审自动修复提交 `d0b6fb4`，在 task5/task6 的实测之后引入。
   - 复现：口令为 `p%zz`、端口无人监听时，stderr 第一行为 `scripts/db-reset.sh: line 50: printf: missing hex digit for \x`，第二行才是固定文案。退出码仍为 4。
   - 影响：不含口令与片段（本次逐字节核对），RUN-04 的不泄露要求成立。但 DB-01「连不上库」和 tasks 4.1 要求的「输出只有固定文案」不再严格成立。
   - 修法：该 `printf` 加 `2>/dev/null`，或先校验 `%` 后是两位十六进制再解码。

2. **连真库层未在最终 HEAD 上重跑**（已知残余风险，可接受）。
   - 本次隧道未起，`scripts/test.sh integration` 连不上库。
   - `181b2d9` 之后改动的非 openspec 文件只有 `cmd/temp-monitor/main.go`、`internal/redact/redact.go`、`scripts/db-reset.sh`、`scripts/lib/local_env.sh` 及两份单元测试。连真库层测试代码和 `pgstore` / `httpapi` / `testdb` 都没有改动。
   - `db-reset.sh` 在 `d0b6fb4` 新增的分支（psql 缺失检查、解码口令脱敏）没有在真库上重跑过 6.3。

3. **test.sh 的 jq 解析失败分支未实测**（可接受）。代码在 `scripts/test.sh:83-90`。task4-test-runner.md 已注明：找不到稳定触发方法，未实测。

4. **代码审 defer 项**（可接受，已入池）。
   - B1：短口令或常见子串口令会被替换进固定日志文案（`internal/redact/redact.go:76,103`）。例如口令为 `temp` 时，告警消息变成 `over-***erature alarm`，影响 ALM-01 的日志文案。口令由服务器随机生成，概率低。
   - T27–T31：读请求体无时限、`.claudeignore` 未覆盖 `config/local.env` 等。
   - 均见 `code-review-report.md` 台账。
