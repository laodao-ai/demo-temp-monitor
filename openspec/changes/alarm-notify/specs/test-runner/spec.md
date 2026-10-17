# Spec Delta

## MODIFIED Requirements

### Requirement: TST-05 单元层不依赖数据库

单元层 SHALL 只使用内存存储实现，并开启数据竞争检测。在没有任何数据库配置、也无法访问网络数据库的机器上，单元层 MUST 能全部通过。单元层 MUST 覆盖注册、注销、上报、告警日志、告警通知与 HTTP 状态码映射。告警通知的用例 MUST 只使用在本机回环地址上启动的假接收方，MUST NOT 访问任何外部地址。

#### Scenario: 无数据库配置跑单元层

- **WHEN** 环境变量和 `config/local.env` 都没有 `TM_DATABASE_URL`，执行 `scripts/test.sh unit`
- **THEN** 退出码为 0，N 大于 0

#### Scenario: 无外网时跑告警通知用例

- **WHEN** 机器无法访问外网，执行 `scripts/test.sh unit`
- **THEN** 告警通知相关的用例全部通过
