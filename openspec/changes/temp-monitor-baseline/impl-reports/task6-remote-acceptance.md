# Task 6：远端库验收 实现记录

- 状态：DONE（6.6 第四项「公司名和客户名」按票面交编排层）
- 分支：`feat/temp-monitor-baseline`，在主仓直接工作
- 服务器版本：`PostgreSQL 18.6 (Ubuntu 18.6-1.pgdg26.04+2) on x86_64-pc-linux-gnu`（`select version()`）
- Go：`go version go1.27.1 darwin/arm64`
- 做法：连库的辅助查询写在仓外 `/tmp/t6/*.sh`，脚本内部读 `config/local.env`，不回显连接串；所有留痕输出已用脚本比对「完整连接串 / 口令 / 口令 4 字符片段」，均未命中（见 6.5 末尾与 6.6）。

## 6.3 七项

起始状态：库里没有 `devices`、`readings` 两张表（`to_regclass` 均为空）。

1. 首次重建前启动服务（`timeout 30 temp-monitor`）：

```
table devices or readings not found; run scripts/db-reset.sh first
rc=1
```

2. 首次重建前 `scripts/test.sh integration`：

```
rc=3
0 passed, 3 failed, 0 skipped (3 total)
=== RUN   TestIntegrationEndToEnd
    e2e_integration_test.go:29: table devices or readings not found; run scripts/db-reset.sh first
--- FAIL: TestIntegrationEndToEnd (0.09s)
=== RUN   TestIntegrationContract
    pgstore_integration_test.go:24: table devices or readings not found; run scripts/db-reset.sh first
--- FAIL: TestIntegrationContract (0.08s)
=== RUN   TestIntegrationConstraints
    pgstore_integration_test.go:31: table devices or readings not found; run scripts/db-reset.sh first
--- FAIL: TestIntegrationConstraints (0.18s)
```

3. 连续两次 `scripts/db-reset.sh`，每次后查行数：

```
rebuilt database temp_monitor_demo
rc=0
3 devices, 6 readings
rebuilt database temp_monitor_demo
rc=0
3 devices, 6 readings
```

4. 重建后 `demo-01`..`demo-03` 存在，且有 `temperature_c > 50`：设备 id 为 `demo-01 / demo-02 / demo-03`；`temperature_c > 50` 的读数 1 条；各设备最大值 `demo-01|22`、`demo-02|34.8`、`demo-03|55.5`。

5. 服务写入 `sim-01` 后再重建：服务日志 `listening addr=127.0.0.1:8080 threshold_c=50`；`POST /devices`（sim-01）与 `POST /devices/sim-01/readings`（60.5）均 201，库里 `sim-01|60.5`。SIGTERM 停服务（rc=0）后 `scripts/db-reset.sh` rc=0；之后 `devices` 中 `sim-01` 行数 0，`readings` 中 `sim-01` 行数 0。

6. 连接串临时改到别的库（`temp_monitor_demo_scratch`，同一账号，由脚本在内存里替换库名，仅作为环境变量传给 `db-reset.sh`）：

```
before: temp_monitor_demo_scratch|0|          （public 下 0 张表）
refusing to reset: only temp_monitor_demo is allowed, connected database is temp_monitor_demo_scratch; check TM_DATABASE_URL
rc=3
after: temp_monitor_demo_scratch|0|
```

7. 临时改坏 `db/seed.sql`（末尾追加对不存在表的 INSERT，之后用备份原样还原，`git status --porcelain db` 为空）。先手工插入一个 `keep-01` 设备，记下快照，再执行：

```
before: 4 devices, 6 readings, md5=a42ac7889a48c86e22dc9f6fe95266ce ids=demo-01,demo-02,demo-03,keep-01
psql failed: psql:db/seed.sql:11: ERROR:  relation "no_such_table" does not exist
LINE 1: INSERT INTO no_such_table VALUES (1);
                    ^; nothing was changed; check the SSH tunnel, then the SQL files under db/
rc=4
after:  4 devices, 6 readings, md5=a42ac7889a48c86e22dc9f6fe95266ce ids=demo-01,demo-02,demo-03,keep-01
```

之后删除 `keep-01`。

## 6.4 `scripts/test.sh`（all）

先 `scripts/db-reset.sh`，再执行：

```
rc=0
160 passed, 0 failed, 0 skipped (160 total)
```

stderr 为空。之后 `select count(*) from devices where id like 'it-%'` 为 0；设备只剩 `demo-01..03`。

## 5.1 补句

服务器版本为 18.6，主版本为 18。`README.md` 第 31 行由「PostgreSQL 18 待确认」改为：

```
测过的版本：Go 1.27；PostgreSQL 18（实测 18.6）。
```

README 中无真实主机、非 127.0.0.1 的 IP、口令、公司名（本次只改这一行）。

## 6.5

从仓根启动服务（`temp-monitor` 二进制由 `go build ./cmd/temp-monitor` 构建，监听缺省 `127.0.0.1:8080`），重建库后执行 `scripts/simulate.sh 3`：

```
registered sim-01 / sim-02 / sim-03
round 1..3 各三行（sim-03 为 50.7 / 58.9 / 52.4）
sim rc=0
重建后 sim 读数：空；执行后 sim-01=3、sim-02=3、sim-03=3
```

- 服务日志：3 条 `level=WARN msg="over-temperature alarm" device_id=sim-03 ... threshold_c=50`（`grep -c level=WARN` = 3）。
- Ctrl-C：向进程发 SIGINT，日志 `shutting down` / `stopped`，`wait` 得 rc=0。
- SIGTERM：第二次启动并跑 `simulate.sh 3`（3 条 WARN），SIGTERM 后 rc=0。
- 只监听本机：`lsof -nP -iTCP -sTCP:LISTEN` 中该进程仅一行 `TCP 127.0.0.1:8080 (LISTEN)`（IPv4，无 IPv6、无通配地址）。
- 端口被占用：服务运行时再启动一个实例：

```
cannot listen on 127.0.0.1:8080: listen tcp 127.0.0.1:8080: bind: address already in use; the port may be in use; free it or set TM_LISTEN_ADDR
rc=1
```

- 不含口令：用 `/tmp/t6/pwcheck.sh` 对本票所有留痕输出（两份服务日志、模拟输出、端口冲突 stderr、首次服务启动失败输出、test.sh all 的 stdout / stderr、integration 失败输出）比对完整连接串、口令、`postgres` 协议头、口令的每个 4 字符窗口，全部 clean。
- 清理：所有服务进程已停；`ps` 与 `lsof -iTCP:8080` 均无残留；最后又执行一次 `db-reset.sh`，库回到 3 设备 6 读数的初始状态。

## 6.6 全历史审计

范围：`git log -p -m --all`（审计时共 34 个提交，已含本票 README 提交 `6302b2f`；报告提交后重跑结果见末尾）。

- IPv4：`grep -oE` 抽出全部 IPv4 token，排除 `127.0.0.1` 后无输出，0 命中。
- 连接串与口令：
  - `postgres` 协议头的连接串共 25 种写法，全部为占位符或测试夹具：`CHANGE_ME`、`u:pw`、`u:s3cret-xyz`、`u:p@ss#w`、`user@h`、`monitor:`、`temp_monitor_demo:` 后接 `CHANGE_ME`，主机均为 `127.0.0.1` 或 `h`。
  - 没有第 2 个真实口令的连接串。
  - 另用 `/tmp/t6/pwhist.sh` 取 `config/local.env` 的真实连接串和口令，在同一份历史里查：完整口令未找到、完整连接串未找到、口令的任一 8 字符窗口未找到。
  - 口令环境变量赋值那个写法（`PGPASSWORD` 后接等号）命中 3 次，全部是 `tasks.md` 里 6.6 条目的说明文字，不是真实赋值。
- 邮箱：全部 34 个提交的作者与提交者邮箱只有一个：`13449619+laodaocheney@users.noreply.github.com`，是 noreply，无命中。
- 姓名（交人核对）：作者与提交者姓名两种：`老刀Cheney`（28 个提交）、`cheneyzhao`（6 个提交，形如本机用户名）。
- 公司名和客户名：NEEDS_CONTEXT（由编排层向人取关键词执行）。本票未做，没有任何关键词写入文件。

## Concerns

- `cheneyzhao` 作为提交者姓名出现在 6 个提交里，是否算「本机用户名泄露」需要人在姓名核对时判断。这是 6.6 本来就交给人的一项。
- 6.3 第 6 项的「无改动」以 scratch 库公开 schema 下表数量（0 → 0）为依据；scratch 库本来是空库，所以只能证明没有建表。库本身未被动过的更强证据是守卫在任何写操作之前退出（`db-reset.sh` 里守卫查询先于 psql 重建）。

## 票外发现

无。

## 提交

- `6302b2f` docs(readme): 6.4 通过后确认测过 PostgreSQL 18
