# 连真库层用共用契约套件加一条端到端用例，随机 ID 自清，连不上即失败

**Status: Accepted** · 来源 change：`temp-monitor-baseline`（2026-10-15）

项目有内存和 PostgreSQL 两个存储实现。连真库层要证明两者行为一致，并证明 HTTP 到 PG 的整条路径可用。

本机没有 docker，连真库层只能用远端开发库。该库由多人或多次运行共用，且带演示数据。

## Decision

- 契约套件 `internal/store/storetest` 由内存实现和 `pgstore` 共用。连真库层把它跑在 `pgstore` 上（`TestIntegrationContract`）。
- 另有一条 HTTP 到 PG 的端到端用例（`internal/httpapi/e2e_integration_test.go` 的 `TestIntegrationEndToEnd`）：注册、超温上报、查读数、看告警日志、注销、确认读数已删。
- 连真库的测试带 build tag `integration`，函数名以 `TestIntegration` 开头。`scripts/test.sh` 用 `-tags integration -run '^TestIntegration' -p 1` 运行。
- 每个用例用 `it-` 加随机后缀的设备 ID，结束时自行删除。
- 测试前不自动重建库。`internal/testdb/testdb.go` 的 `Open` 在连不上或表不存在时 `t.Fatal`，提示先跑 `scripts/db-reset.sh`。
- `testdb` 不调用 `t.Skip`。连不上库就是失败。

## Considered Options

- **`test.sh integration` 先自动重建库**：否决。每跑一次测试就清空演示数据。
- **用专用测试库**：否决。多一个库要建、要配，隔离需求靠随机 ID 已够。
- **连不上时跳过**：否决。跳过会让「未验证」显示为绿。
- **共用契约套件加随机 ID 自清（采用）**。

## Consequences

- 系统镜：两个实现由同一套用例约束，行为分歧会直接暴露。测试与演示数据互不破坏。
- 用户镜：首次跑连真库层前要手动跑一次 `db-reset.sh`。远端库未就绪时，该层恒失败，打 tag 要等库就绪。
- 开发循环镜：新增存储行为只写一次契约用例。被杀或超时的测试可能残留 `it-` 设备，`db-reset.sh` 可清掉。
- 主次：不假绿优先，首次手动建表的一步成本为次。
- 共享库上并发运行时，随机 ID 撞车概率极低，不为此加专用库。

## 附录：修订历史

- 暂无。
