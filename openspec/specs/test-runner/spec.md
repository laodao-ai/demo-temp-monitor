# test-runner Specification

## Purpose
规定仓内统一测试入口的行为：两层测试（不依赖数据库的单元层、连真库的集成层）如何选择和执行，汇总行与退出码如何计算，以及哪些情况不得报绿。

## Requirements

### Requirement: TST-01 统一入口与层选择

仓库 SHALL 提供 `scripts/test.sh [unit|integration|all]`，参数缺省为 `all`。`unit` 只跑单元层，`integration` 只跑连真库层，`all` 依次跑两层，两层分别计数后求和。[spec-review-amendment] 其它参数 MUST 打印用法并以退出码 2 结束。每次执行 MUST 真正运行测试，不使用测试缓存。

#### Scenario: 缺省跑两层

- **WHEN** 执行 `scripts/test.sh`，不带参数
- **THEN** 单元层与连真库层的用例都被执行，并计入同一行汇总

#### Scenario: 非法参数

- **WHEN** 执行 `scripts/test.sh e2e`
- **THEN** 退出码为 2，打印用法，不运行任何测试

#### Scenario: 连续执行两次

- **WHEN** 代码不变，连续执行两次 `scripts/test.sh unit`
- **THEN** 两次汇总行中的总数相同，第二次的用例也是实际运行的，不是缓存结果

### Requirement: TST-02 汇总行格式

脚本 SHALL 在 stdout 的最后一行输出 `N passed, M failed, K skipped (T total)`，其中 T = N + M + K。失败用例的输出 SHALL 打印到 stderr。

以下内容也 SHALL 打印到 stderr：[spec-review-amendment]

- 编译失败的原因，即 `build-output` 事件的 `Output`；[spec-review-amendment]
- 贡献了包级失败的包，其包级输出；[spec-review-amendment]
- `go test` 自身的 stderr，原样透传，不经 jq。[spec-review-amendment]

#### Scenario: 全部通过

- **WHEN** 单元层有 40 个用例且全部通过，执行 `scripts/test.sh unit`
- **THEN** stdout 最后一行为 `40 passed, 0 failed, 0 skipped (40 total)`

#### Scenario: 有失败用例

- **WHEN** 有 1 个用例失败
- **THEN** stderr 含该用例的名字和输出，stdout 最后一行的 M 为 1

#### Scenario: 编译失败的原因 [spec-review-amendment]

- **WHEN** 某个测试文件有编译错误，执行 `scripts/test.sh unit`
- **THEN** stderr 含编译器给出的错误行

### Requirement: TST-03 计数口径不得假绿

计数 SHALL 只统计用例级（含子测试）的 pass、fail、skip 结果。某个包整体失败但没有用例级失败时，M MUST 加 1，例如编译失败、vet 失败、测试进程异常退出或超时。这一判定按「层 + 包」进行。[spec-review-amendment] 没有测试文件的包 MUST NOT 计入 K。

某层 `go test` 以非零状态退出、而该层 M 为 0 时，该层 M MUST 加 1，并在 stderr 打印 `go test exited with status <n> but reported no failure`。[spec-review-amendment]

#### Scenario: 测试包编译失败

- **WHEN** 某个测试文件有编译错误，执行 `scripts/test.sh unit`
- **THEN** 汇总行的 M 至少为 1，退出码非零

#### Scenario: go test 进程被杀 [spec-review-amendment]

- **WHEN** 执行 `scripts/test.sh unit` 期间，`go test` 进程被 `kill -9` 杀掉
- **THEN** 退出码非零

#### Scenario: 没有测试文件的包

- **WHEN** `cmd/temp-monitor` 包没有测试文件，执行 `scripts/test.sh unit`
- **THEN** 该包不让 K 增加

#### Scenario: 子测试计数

- **WHEN** 一个测试函数含 2 个子测试且全部通过
- **THEN** N 增加 3（父测试 1 个，子测试 2 个）

### Requirement: TST-04 退出码跟随失败数

退出码规则如下：

- 所选的每一层各自计数。每一层的已执行用例数 N + M 都大于 0 时，脚本 SHALL 以 `min(M, 125)` 作为退出码，M 为各层之和。[spec-review-amendment]
- 任一所选层的 N + M 等于 0 时，脚本 MUST 在 stderr 打印 `no tests executed in <层名> layer`，并以退出码 1 结束。`<层名>` 为 `unit` 或 `integration`。汇总行照常输出。[spec-review-amendment]
- 缺少 `go` 或 `jq` 时，脚本 MUST 报错并以退出码 1 结束，不输出汇总行。[spec-review-amendment]
- 解析 `go test -json` 输出的 jq 失败时，脚本 MUST 在 stderr 报错并以退出码 1 结束，不输出汇总行。[spec-review-amendment]

#### Scenario: 无失败

- **WHEN** 汇总为 `40 passed, 0 failed, 0 skipped (40 total)`
- **THEN** 退出码为 0

#### Scenario: 3 个失败

- **WHEN** 汇总的 M 为 3
- **THEN** 退出码为 3

#### Scenario: 失败数超过 125

- **WHEN** 汇总的 M 为 300
- **THEN** 退出码为 125

#### Scenario: 一个用例都没执行

- **WHEN** 所选层的用例被全部过滤掉，或顶层用例全部跳过（没有子测试）[spec-review-amendment]
- **THEN** stderr 含 `no tests executed`，退出码为 1

#### Scenario: all 模式下连真库层零用例 [spec-review-amendment]

- **WHEN** 连真库测试文件的 build tag 写错，单元层有 40 个用例且全部通过，执行 `scripts/test.sh all`
- **THEN** stderr 含 `no tests executed in integration layer`，退出码为 1
- **AND** stdout 最后一行仍是汇总行

### Requirement: TST-05 单元层不依赖数据库

单元层 SHALL 只使用内存存储实现，并开启数据竞争检测。在没有任何数据库配置、也无法访问网络数据库的机器上，单元层 MUST 能全部通过。单元层 MUST 覆盖注册、注销、上报、告警日志与 HTTP 状态码映射。

#### Scenario: 无数据库配置跑单元层

- **WHEN** 环境变量和 `config/local.env` 都没有 `TM_DATABASE_URL`，执行 `scripts/test.sh unit`
- **THEN** 退出码为 0，N 大于 0

### Requirement: TST-06 连真库层连不上就失败

连真库层的用例在以下任一情况下 SHALL 失败，MUST NOT 跳过：缺少 `TM_DATABASE_URL`；连不上数据库；库中缺少本项目的表。失败信息 MUST 说明下一步该做什么。

#### Scenario: 没有连接配置

- **WHEN** 没有 `TM_DATABASE_URL`，执行 `scripts/test.sh integration`
- **THEN** 连真库层用例计入 M，K 不增加，退出码非零
- **AND** 失败信息提示把 `config/example.env` 复制为 `config/local.env` 并填写

#### Scenario: 数据库不可达

- **WHEN** `TM_DATABASE_URL` 指向一个不可达的地址，执行 `scripts/test.sh integration`
- **THEN** 连真库层用例计入 M，退出码非零

#### Scenario: 表不存在

- **WHEN** 数据库可达但没有执行过 `scripts/db-reset.sh`，执行 `scripts/test.sh integration`
- **THEN** 连真库层用例失败，失败信息提示先运行 `scripts/db-reset.sh`

### Requirement: TST-07 两个存储实现共用一套契约测试

同一套存储契约测试 SHALL 在单元层跑在内存实现上，在连真库层跑在 PostgreSQL 实现上，并覆盖下列行为：

- 注册；
- 重复注册返回「已存在」；
- 注销并级联删除读数；
- 注销不存在的设备返回「不存在」；
- 向不存在的设备写读数返回「不存在」；
- 读数按写入顺序返回；
- 同一 ID 并发注册只成功一次；
- 并发写读数后条数正确；
- 并发注销与上报后没有残留读数，上报结果只可能是成功或不存在。[spec-review-amendment]

#### Scenario: 两层用同一套契约

- **WHEN** 执行 `scripts/test.sh all`，并且数据库已就绪
- **THEN** 内存实现与 PostgreSQL 实现各自通过同一组契约用例

### Requirement: TST-08 连真库层含一条端到端用例

连真库层 SHALL 含一条用例，在 HTTP 接口加 PostgreSQL 存储的组合上依次验证：

1. 注册设备；
2. 上报一条高于阈值的读数，响应为 201；
3. 库中能查到这条读数；
4. 日志中有对应的超温告警；
5. 注销设备后库中该设备的读数为 0 条。

#### Scenario: 端到端

- **WHEN** 数据库已就绪，执行 `scripts/test.sh integration`
- **THEN** 端到端用例通过

### Requirement: TST-09 连真库测试不破坏演示数据

连真库层的用例 SHALL 只使用以 `it-` 开头并带随机后缀的设备 ID，并在用例结束时删除自己创建的设备。连真库层 MUST NOT 自动执行库重建，MUST NOT 修改种子数据。

#### Scenario: 测试后种子数据不变

- **WHEN** 执行 `scripts/db-reset.sh`，然后执行 `scripts/test.sh integration` 且全部通过
- **THEN** 库中的设备集合与重建后相同，没有遗留以 `it-` 开头的设备
