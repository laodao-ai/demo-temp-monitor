# Spec Delta

## MODIFIED Requirements

### Requirement: TST-05 单元层不依赖数据库

单元层 SHALL 只使用内存存储实现，并开启数据竞争检测。在没有任何数据库配置、也无法访问网络数据库的机器上，单元层 MUST 能全部通过。单元层 MUST 覆盖注册、注销、上报、告警日志、超温通知与 HTTP 状态码映射。单元层的通知用例 MUST 只请求本机回环地址上由测试自己起的接收端，MUST NOT 访问外部网络。

#### Scenario: 无数据库配置跑单元层

- **WHEN** 环境变量和 `config/local.env` 都没有 `TM_DATABASE_URL`，执行 `scripts/test.sh unit`
- **THEN** 退出码为 0，N 大于 0

#### Scenario: 无外网跑单元层

- **WHEN** 机器无法访问外部网络，执行 `scripts/test.sh unit`
- **THEN** 通知相关用例全部通过

### Requirement: TST-08 连真库层含一条端到端用例

连真库层 SHALL 含一条用例，在 HTTP 接口加 PostgreSQL 存储的组合上依次验证：

1. 注册设备；
2. 上报一条高于阈值的读数，响应为 201；
3. 库中能查到这条读数；
4. 日志中有对应的超温告警；
5. 测试在本机回环地址起的通知接收端恰好收到一次该设备的通知，报文中的 `reading_id` 等于第 2 步上报响应中的 `id`；
6. 注销设备后库中该设备的读数为 0 条。

#### Scenario: 端到端

- **WHEN** 数据库已就绪，执行 `scripts/test.sh integration`
- **THEN** 端到端用例通过
