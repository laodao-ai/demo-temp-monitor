# Task 5 实现记录：重建脚本、模拟脚本、README

## 产物

- `scripts/db-reset.sh`（tasks 4.1）
- `scripts/simulate.sh`（tasks 4.4）
- `scripts/lib/local_env.sh`：两个脚本共用的 `config/local.env` 逐行解析函数，只被 `source` 的是脚本自己的 lib，不是配置文件。bash 3.2 兼容（macOS 默认 bash）。
- `README.md`（tasks 5.1）

`shellcheck -x -P .` 对三个 shell 文件无告警。

## 前置问题

派发时 worktree 的起点是 `f08aeab`，不是票里写的 `38bdde5`，里面没有 Go 代码、`db/`、`config/` 和 change 目录。已在自己的分支上 `git reset --hard 38bdde5`（该分支此前没有任何自己的提交），之后的工作都基于 `38bdde5`。

## 4.1 `db-reset.sh` 逐项对照

| 要求 | 实现位置 / 证据 |
|---|---|
| 开头切到仓根 | `cd "$(... dirname BASH_SOURCE)/.."`；下面从 `/tmp` 调用的场景证明 |
| 带参数退出 2 | 场景 usage |
| 环境变量非空直接用；否则读文件一个键，不 `source`；都没有退出 1 | `local_env_get`（逐行 `read`，不执行内容）；场景 no config |
| 文件文法错误退出 1，只给行号不回显内容 | 场景 bad line（3 种） |
| `current_database()` 不符退出 3，打印实际库名 | 桩 psql 场景 mismatch |
| 重建带同一连接串，`SET lock_timeout = '5s'`，单事务 drop → migrations（字典序）→ seed | 脚本第 75 行；桩 psql 记录的实参见下 |
| `PGCONNECT_TIMEOUT=5` | `export PGCONNECT_TIMEOUT=5` |
| psql 失败捕获并映射为 4，stderr 先捕获脱敏后输出 | 桩 psql 场景 rebuild fails |
| 守卫阶段只输出固定文案 | 场景 unreachable port |
| 口令提取不出时重建失败也只输出固定文案 | 代码分支（未单独实测，见 Concerns） |

### 实测：不连库四项

命令均在 worktree 根执行，驱动脚本在 `/tmp`（不入仓）。

```
--- usage (arg): scripts/db-reset.sh temp_monitor_demo
exit=2
usage: scripts/db-reset.sh (takes no arguments)
--- no config
exit=1
TM_DATABASE_URL is not set; copy config/example.env to config/local.env and fill it in
```

1. 不可达端口（`postgres://u:s3cret-xyz@127.0.0.1:1/temp_monitor_demo?sslmode=disable&connect_timeout=5`）：

```
exit=4
cannot connect to database; check the SSH tunnel and that the URL uses the session pool port 7432
```

2. 两条 psql 都带同一连接串（`grep -n '"$TM_DATABASE_URL"' scripts/db-reset.sh`）：

```
58:if ! actual_db=$(psql "$TM_DATABASE_URL" -XAtqc 'select current_database()' 2>/dev/null); then
75:if ! err=$(psql "$TM_DATABASE_URL" -Xq -v ON_ERROR_STOP=1 --single-transaction \
```

桩 psql 记录的两次调用（连接串相同）：

```
postgres://u:p@ss#w@127.0.0.1:7432/other_db -XAtqc select current_database()
postgres://u:p@ss#w@127.0.0.1:7432/other_db -Xq -v ON_ERROR_STOP=1 --single-transaction -c SET lock_timeout = '5s' -f db/drop.sql -f db/migrations/0001_init.sql -f db/seed.sql
```

3. 四个口令（均不做 URL 编码，指向不可达端口），stdout+stderr 均不含口令及片段 `ss;w`、`ss#w`、`%zz`：

```
--- pw=p(ss;w   exit=4 leak=no output: cannot connect to database; check the SSH tunnel and that the URL uses the session pool port 7432
--- pw=p&ss     exit=4 leak=no output: (同上)
--- pw=p%zz     exit=4 leak=no output: (同上)
--- pw=p@ss#w   exit=4 leak=no output: (同上)
```

4. `config/local.env` 含 `TM_DATABASE_URL=$(touch marker)`，从 `/tmp` 目录调用：

```
exit=4
cannot connect to database; check the SSH tunnel and that the URL uses the session pool port 7432
marker check: ls: /tmp/marker: No such file or directory
marker check: ls: marker: No such file or directory
```

### 补充实测

- 文法错误（文件第 3 行分别为 `oops s3cret-xyz` / `FOO=1` / `TM_DATABASE_URL="x"`），均退出 1：
  - `config/local.env line 3: missing "="; use KEY=VALUE`
  - `config/local.env line 3: unknown key; allowed keys: TM_DATABASE_URL, TM_LISTEN_ADDR, TM_ALARM_THRESHOLD_C`
  - `config/local.env line 3: value must not be quoted; remove the quotes`
  - 第一条的输出不含 `s3cret-xyz`。
- 文件里的 URL 含 `=`、`&`，行尾 CRLF：脚本读到整串并走到连库（退出 4，固定文案），没有被截断成"缺 URL"。
- 桩 psql（`/tmp/stub/psql`，不入仓）：
  - 守卫返回 `other_db`：`exit=3`，`refusing to reset: only temp_monitor_demo is allowed, connected database is other_db; check TM_DATABASE_URL`
  - 守卫返回 `temp_monitor_demo`，重建成功：`exit=0`，`rebuilt database temp_monitor_demo`
  - 重建失败，桩把连接串和口令 `p@ss#w` 写进 stderr：`exit=4`，`psql failed: ERROR: boom for *** password ***; nothing was changed; check the SSH tunnel, then the SQL files under db/`

没有连真库跑，所以 DB-02/DB-03 的真库场景（首次重建、连续两次、回滚）留给 6.3。

## 4.4 `simulate.sh` 逐项对照

| 要求 | 证据 |
|---|---|
| 开头切到仓根，`LC_ALL=C` | 脚本头部；从 `/tmp` 调用的场景 |
| `TM_LISTEN_ADDR`：环境变量非空 > 文件 > 缺省，逐行解析不 `source` | 场景 env overrides file、file addr |
| 以 `:` 开头时主机用 `127.0.0.1` | 文件写 `TM_LISTEN_ADDR=:18099`，请求到了 `127.0.0.1:18099`，退出 0 |
| `rounds` 非正整数退出 2 并打印用法 | 下面 |
| curl 带 `--max-time 5` | 脚本 `request` 函数 |
| 注册 409 视为已注册并继续 | 场景 again |
| sim-01/02 在 18.0–35.0，sim-03 在 50.5–60.0 | `tenths 180 350` / `tenths 505 600`；实测值 19.1–33.6、50.7–59.8 |
| 请求失败打印请求与响应，退出 1 | 下面 |

实测（本机 8080 无人监听，已用 `lsof` 确认 0 个监听者）：

```
--- service not running (scripts/simulate.sh)
exit=1
request failed: POST http://127.0.0.1:8080/devices -> curl error 7; start the service first, or check TM_LISTEN_ADDR
request body: {"id":"sim-01"}
--- rounds=0   exit=2  usage: scripts/simulate.sh [rounds]  (rounds: positive integer, default 10)
--- rounds=abc exit=2  (同上)
--- rounds=-1  exit=2  (同上)
--- rounds=1.5 exit=2  (同上)
--- two args (1 2) exit=2 (同上)
--- file TM_DATABASE_URL=$(touch marker), cwd=/tmp, 服务未运行
exit=1
request failed: POST http://127.0.0.1:8080/devices -> curl error 7; ...
marker check: ls: /tmp/marker: No such file or directory
marker check: ls: marker: No such file or directory
--- file line "junk"
exit=1
config/local.env line 1: missing "="; use KEY=VALUE
```

成功路径用一个本地假服务（`/tmp/fake_srv.py`，只回 201/409，不入仓）验证：

- `rounds=2`，退出 0；服务端日志显示 3 次注册 201、6 条读数 201。
- 再跑一次：3 次注册得 409，输出 `sim-0N already registered, continuing`，6 条读数后退出 0。
- 环境变量 `TM_LISTEN_ADDR=127.0.0.1:18099` 覆盖文件里的 `127.0.0.1:1`，请求成功。

## 5.1 README 对照

| 要求 | README 位置 |
|---|---|
| 项目说明与四块范围 | 「范围」 |
| 前置工具（Go 1.27、psql、jq、curl、bash、C 编译器，`-race` 需要） | 「前置工具」 |
| 测过的版本：Go 1.27；PostgreSQL 18 | 「前置工具」末行：`Go 1.27；PostgreSQL 18 待确认`，待 Task 6（6.4 通过后）确认 |
| 配置方法 | 「配置」（含优先级、复制 example.env、URL 编码、session 池端口、文法） |
| 启动顺序：建库 → 隧道 → `config/local.env` → `db-reset.sh` → 启动服务 → `simulate.sh` | 「启动顺序」 |
| 测试用法：第一个命令 `scripts/test.sh unit` 不需数据库；没配库时直接跑 `scripts/test.sh` 会失败；用法错误和缺工具时无汇总行 | 「测试」 |
| 「无鉴权，只应监听 127.0.0.1」 | 独立小节 |

README 没有真实主机、IP（只有 `127.0.0.1`）、口令、公司名、客户名；`laodao-ai` 只出现在仓库地址里；没有描述 `.pg-ops/`。

## Concerns

- `scripts/lib/local_env.sh` 是票外新增的共享文件（两个脚本共用一份解析器，避免文法在两处各写一遍而漂移）。如果编排层要求每个脚本自包含，把函数内联即可。
- 口令提取失败时，重建失败分支只输出固定文案，这条分支没有单独实测。
- 真库场景（DB-02 首次重建与连续两次、DB-03 回滚、DB-04 种子内容、"默认 psql 目标也可连"）没有执行，属 6.3。
- `scripts/test.sh` 不在本票范围，没有碰。

## 票外发现

无。

## 提交

见返回值（报告随同一个提交，SHA 以返回值为准）。
