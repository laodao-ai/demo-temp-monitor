# Spec Delta

## MODIFIED Requirements

### Requirement: TST-08 连真库层含一条端到端用例

连真库层 SHALL 含一条用例，在 HTTP 接口、PostgreSQL 存储和推送的组合上依次验证：

1. 注册设备；
2. 上报一条高于阈值的读数，响应为 201；
3. 库中能查到这条读数；
4. 日志中有对应的超温告警；
5. 用例自己起的本机接收端收到恰好一次推送，`device_id` 与 `reading_id` 和这条读数一致；
6. 注销设备后库中该设备的读数为 0 条。

接收端 MUST 只监听 `127.0.0.1`，用例 MUST NOT 访问外部地址。

#### Scenario: 端到端

- **WHEN** 数据库已就绪，执行 `scripts/test.sh integration`
- **THEN** 端到端用例通过
