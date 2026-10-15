# Task 2 实现报告：业务逻辑与 HTTP

## 做了什么

- `internal/service/service.go`：`New(store, thresholdC, logger)`、`Register` / `Deregister` / `Report`。ID 正则 `^[a-z0-9-]{1,64}$` 在 service 校验，不合法返回 `service.ErrInvalidID`。入库成功且温度严格大于阈值才写 WARN `over-temperature alarm`（字段 `device_id` / `temperature_c` / `threshold_c`）。`service` 只 import `store`。
- `internal/httpapi/httpapi.go`：`NewHandler(svc, logger)`，三个路由（`POST /devices`、`DELETE /devices/{id}`、`POST /devices/{id}/readings`）。
  - 请求体 `io.ReadAll(http.MaxBytesReader(1 MiB))` 后 `json.Unmarshal`；超限、尾随内容、空体、顶层类型错误均 400；未知字段忽略；不检查 Content-Type。
  - `id` 与 `temperature_c` 解码到指针，区分缺失与空 / 0。
  - 每次存储调用用 `context.WithTimeout(r.Context(), 5s)`。
  - 400 文案逐字取自 design「Error Messages」。404 / 409 文案按 design「API」。
  - 其它错误：响应体固定 `{"error": "internal error"}`，ERROR 日志字段 `route`（`r.Pattern`）、`device_id`、`error`。
- 没有改 tasks.md / tickets.md 复选框，没有打 checkpoint 标签。

## 验收与 Scenario 对应的测试

service（`internal/service/service_test.go`）：

| 要求 | 测试 |
|---|---|
| DEV-01 ID 校验 | `TestRegisterValidatesID` |
| DEV-01 重复注册 | `TestRegisterDuplicate` |
| DEV-02 / DEV-03 注销、读数清空、再注册 | `TestDeregister` |
| ING-01 入库（含 0、负数） | `TestReportStoresReading` |
| ING-03 路径 ID 非法 | `TestReportInvalidIDStoresNothing` |
| ALM-01 高于阈值告警 | `TestAlarmOverThreshold` |
| ALM-01 阈值可配置 | `TestAlarmThresholdIsConfigurable` |
| ALM-02 等于 / 低于阈值不告警 | `TestNoAlarmAtOrBelowThreshold` |
| ALM-03 未注册不告警 | `TestNoAlarmForUnregisteredDevice` |
| ALM-03 存储出错不告警 | `TestNoAlarmWhenStoreFails` |
| ALM-04 连续两条各告警 | `TestEveryOverTemperatureReadingAlarms` |

httpapi（`internal/httpapi/httpapi_test.go`）：

| 要求 | 测试 |
|---|---|
| DEV-01 注册成功 | `TestRegisterCreatesDevice` |
| DEV-01 重复注册 409、读数不变 | `TestRegisterDuplicateKeepsReadings` |
| DEV-01 ID 不合法 / 缺失 / 非 JSON / 尾随 / 空体 | `TestRegisterRejectsBadRequests`、`TestRegisterRejectsBadRequestsWithoutCreating` |
| DEV-01 未知字段、Content-Type | `TestRegisterIgnoresUnknownFieldsAndContentType` |
| DEV-01 超 1 MiB | `TestRegisterBodyTooLarge` |
| DEV-02 注销成功 204、读数清空、之后上报 404 | `TestDeleteRemovesDeviceAndReadings` |
| DEV-02 注销未注册 404 | `TestDeleteUnregistered` |
| DEV-02 注销 ID 非法 400 | `TestDeleteInvalidID` |
| DEV-03 | `TestReRegisterAfterDeleteHasNoReadings` |
| ING-01 | `TestReportStoresReading`、`TestReportZeroAndNegativeAreValid`、`TestReportIgnoresUnknownFieldsAndContentType`、`TestReportAllowsTrailingWhitespace` |
| ING-02 未注册 404 | `TestReportUnregisteredStoresNothing` |
| ING-02 注销与上报并发 | `TestConcurrentDeleteAndReportLeavesNoReadings`（50 轮，`-race`） |
| ING-03 缺字段 / 非数字 / 非 JSON / 空体 / 尾随内容 | `TestReportRejectsBadRequests` |
| ING-03 路径 ID 非法 | `TestReportInvalidPathID` |
| ING-03 超 1 MiB（单个大值、尾随空白）与边界 | `TestReportBodyTooLargeSingleValue`、`TestReportBodyTooLargeTrailingWhitespace`、`TestReportBodyExactlyOneMiBIsAccepted`、`TestReportBodyOneByteOverLimitIsRejected` |
| ALM-01..03 经 HTTP | `TestAlarmOnlyForSuccessfulOverThresholdReports` |
| RUN-04 5xx 固定体、ERROR 日志、响应不泄漏底层错误 | `TestStoreErrorsGive500WithFixedBodyAndErrorLog`（三个路由） |
| 5 秒存储超时、随请求取消 | `TestStoreCallsCarryFiveSecondDeadline`、`TestStoreContextFollowsRequestCancellation` |

## 红绿确认

两个包都先写测试，在实现缺失时编译失败（红），再写实现。之后对关键断言做了破坏验证，每次都确认测试变红后恢复：

- `>` 改 `>=`：`TestNoAlarmAtOrBelowThreshold` 红。
- 告警条件改 `true`：同上红。
- 超时 5s 改 9s：`TestStoreCallsCarryFiveSecondDeadline` 红。
- 去掉 `temperature_c` 缺失判断：`TestReportRejectsBadRequests` 红。
- 5xx 日志不带底层错误：`TestStoreErrorsGive500WithFixedBodyAndErrorLog` 红。
- 1 MiB 上限改为 +1：原有测试不红，补 `TestReportBodyOneByteOverLimitIsRejected` 后红。

## 命令输出尾部

`go vet ./...`：无输出。

`go test -race -count=1 ./...`：

```
ok  	github.com/laodao-ai/demo-temp-monitor/internal/config	1.219s
ok  	github.com/laodao-ai/demo-temp-monitor/internal/httpapi	1.508s
ok  	github.com/laodao-ai/demo-temp-monitor/internal/service	1.245s
?   	github.com/laodao-ai/demo-temp-monitor/internal/store	[no test files]
ok  	github.com/laodao-ai/demo-temp-monitor/internal/store/memstore	1.240s
?   	github.com/laodao-ai/demo-temp-monitor/internal/store/storetest	[no test files]
```

## 提交 SHA

见返回值（报告随同一提交，无法写入自身 SHA）。

## Concerns

- 5xx ERROR 日志的 `error` 字段直接写底层错误文本，httpapi 没有脱敏。httpapi 不知道连接串，无从替换。Task 3 的 pgstore 返回给上层的错误应在 pgstore 内先脱敏（pgstore 持有连接串），或 main 注入脱敏后的 logger。选哪种由 Task 3 定，本票没有预建脱敏函数。
- 请求体顶层类型错误（如 `[]`、`23.5`）归为 `invalid JSON body`；`{"id": 5}` 归为 `invalid device id`。design 未规定这两种情形，按最接近的既有文案处理。
- 路径 ID 非法与请求体错误同时存在时，先报请求体错误（先读体再进 service 校验 ID）。spec 未规定顺序。

## 票外发现

无。
