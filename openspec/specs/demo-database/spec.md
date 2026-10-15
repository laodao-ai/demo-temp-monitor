# demo-database Specification

## Purpose
规定演示库的表结构、种子数据如何随仓提交，以及一条命令重建演示库的脚本：它只对固定演示库名生效，失败时不留下半成品。

## Requirements

### Requirement: DB-01 重建脚本只接受固定演示库名

`scripts/db-reset.sh` SHALL 先查询实际连接到的库名（`current_database()`），只有库名精确等于 `temp_monitor_demo` 时才继续。库名不符时 MUST 打印实际库名、不做任何修改，并以退出码 3 结束。脚本不接受参数，带参数时打印用法并以退出码 2 结束。

守卫与重建 MUST 使用同一个连接串 `TM_DATABASE_URL`。[spec-review-amendment] 退出码 SHALL 为：0 成功；1 缺 `TM_DATABASE_URL` 或 `config/local.env` 有文法错误；2 用法错误；3 库名不符；4 psql 失败（连不上、SQL 出错、等锁超时）。脚本 MUST 自己捕获 psql 的退出状态再映射，不透传。[spec-review-amendment]

#### Scenario: 指向别的库

- **WHEN** `TM_DATABASE_URL` 指向库 `other_db`，执行 `scripts/db-reset.sh`
- **THEN** 退出码为 3，stderr 含 `other_db`
- **AND** `other_db` 中的对象没有任何变化

#### Scenario: 带参数调用

- **WHEN** 执行 `scripts/db-reset.sh temp_monitor_demo`
- **THEN** 退出码为 2，打印用法，不连接数据库

#### Scenario: 没有连接配置

- **WHEN** 环境变量没有 `TM_DATABASE_URL`，`config/local.env` 也不存在，执行 `scripts/db-reset.sh`
- **THEN** 退出码为 1，stderr 指出缺少 `TM_DATABASE_URL`

#### Scenario: 本机默认 psql 目标也可连 [spec-review-amendment]

- **WHEN** 不带连接参数的 `psql` 能连上本机的另一个库，`TM_DATABASE_URL` 指向 `temp_monitor_demo`，执行 `scripts/db-reset.sh`
- **THEN** 只有 `temp_monitor_demo` 被修改
- **AND** psql 默认目标库中的对象没有任何变化

#### Scenario: 连不上库 [spec-review-amendment]

- **WHEN** `TM_DATABASE_URL` 指向一个没有服务在监听的端口，执行 `scripts/db-reset.sh`
- **THEN** 退出码为 4，stderr 为固定文案 `cannot connect to database` 加下一步提示，不含 psql 的原始错误文本

### Requirement: DB-02 一条命令重建演示库

库名守卫通过后，`scripts/db-reset.sh` SHALL 在一个事务里依次执行三步：删除本项目的表；按文件名顺序执行 `db/migrations/` 下的全部迁移；执行 `db/seed.sql`。重建 MUST 不需要删库或建库权限。连续执行两次 MUST 都成功，并且结果一致。

#### Scenario: 空库首次重建

- **WHEN** `temp_monitor_demo` 中没有本项目的表，执行 `scripts/db-reset.sh`
- **THEN** 退出码为 0，库中存在 `devices` 与 `readings` 表，并含种子数据

#### Scenario: 连续重建两次

- **WHEN** 连续执行两次 `scripts/db-reset.sh`
- **THEN** 两次退出码都为 0
- **AND** 第二次之后的设备与读数行数，与第一次之后相同

#### Scenario: 重建清除运行期数据

- **WHEN** 服务运行期间注册了 `sim-01` 并写入读数，然后执行 `scripts/db-reset.sh`
- **THEN** 库中不再有 `sim-01`，只剩种子数据

### Requirement: DB-03 重建失败整体回滚

重建中任一 SQL 语句失败，或等锁超过 5 秒时，脚本 SHALL 以退出码 4 结束，库中对象 MUST 保持执行前的状态。[spec-review-amendment]

#### Scenario: 种子数据出错

- **WHEN** `db/seed.sql` 含一条会失败的语句，执行 `scripts/db-reset.sh`
- **THEN** 退出码为 4 [spec-review-amendment]
- **AND** 执行前已存在的表和数据保持不变

### Requirement: DB-04 迁移与种子数据随仓提交

建表迁移、删表 SQL 与种子数据 SHALL 作为 SQL 文件提交在 `db/` 下。种子数据 MUST 含设备 `demo-01`、`demo-02`、`demo-03`，以及至少一条温度高于 50.0 的读数。

#### Scenario: 种子数据内容

- **WHEN** 执行 `scripts/db-reset.sh` 后查询库
- **THEN** `devices` 中有 `demo-01`、`demo-02`、`demo-03`
- **AND** `readings` 中至少有一条 `temperature_c > 50`

### Requirement: DB-05 数据库层约束

数据库 SHALL 自己保证三条约束，不依赖服务代码：设备 ID 唯一，并且匹配 `^[a-z0-9-]{1,64}$`；读数必须引用已存在的设备；删除设备时级联删除其读数。

#### Scenario: 直接写入非法设备 ID

- **WHEN** 用 SQL 直接插入 id 为 `Bad_ID` 的设备
- **THEN** 插入因检查约束失败

#### Scenario: 直接删除设备

- **WHEN** 测试先用 SQL 建一台 `it-` 开头的设备并写入读数，再用 SQL 直接删除这台设备 [spec-review-amendment]
- **THEN** `readings` 中不再有这台设备的行 [spec-review-amendment]
