### Task 2: 业务逻辑与 HTTP

**Blocked-by:** 1
**R-ID:** DEV-01, DEV-02, DEV-03, ING-01, ING-02, ING-03, ALM-01, ALM-02, ALM-03, ALM-04, RUN-04

注册、注销、上报三项业务可经 HTTP 调用。上报温度严格大于阈值且入库成功时写一条 WARN 告警。错误按 design「API」与「Error Messages」映射为 400 / 404 / 409 / 500，5xx 响应体固定，底层错误只进日志且已脱敏。用 `httptest` 加内存实现验证，不需要数据库。

- [ ] tasks 3.1：ID 校验、注册 / 注销 / 上报、告警日志（msg `over-temperature alarm`，字段 `device_id` / `temperature_c` / `threshold_c`）；单元测试覆盖等于阈值不告警、未注册不告警、存储出错不告警、连续两条各告警一次
- [ ] tasks 3.2：三个路由；请求体读取与 1 MiB 上限、尾随内容、空请求体、未知字段、`temperature_c` 区分缺失与 0；存储调用用请求 context 派生的 5 秒超时；400 文案逐字按 design「Error Messages」；单元测试覆盖 DEV-01..03 与 ING-01..03 的全部场景
- [ ] `go test -race ./...` 通过

