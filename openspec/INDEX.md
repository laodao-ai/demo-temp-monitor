# OpenSpec Index

本文件是当前仓库 OpenSpec 资产索引。

术语表：[GLOSSARY.md](../GLOSSARY.md)（规格、设计、代码注释中的领域用语以它为准）。

## 按主题分组（specs/）

### 温度监控服务（temp-monitor-baseline 引入）

| 能力 | 文件 | 作用 |
|---|---|---|
| `device-registry` | [specs/device-registry/spec.md](./specs/device-registry/spec.md) | 设备注册、注销、重新注册与并发注册 |
| `reading-ingest` | [specs/reading-ingest/spec.md](./specs/reading-ingest/spec.md) | 读数上报、校验与模拟上报脚本 |
| `over-temperature-alarm` | [specs/over-temperature-alarm/spec.md](./specs/over-temperature-alarm/spec.md) | 超阈值读数写 WARN 告警 |
| `service-runtime` | [specs/service-runtime/spec.md](./specs/service-runtime/spec.md) | 配置加载、启动前置检查、连接信息脱敏、优雅退出 |
| `demo-database` | [specs/demo-database/spec.md](./specs/demo-database/spec.md) | 迁移、种子、库约束与重建脚本 |
| `test-runner` | [specs/test-runner/spec.md](./specs/test-runner/spec.md) | 测试统一入口、计数口径、契约套件与端到端用例 |
