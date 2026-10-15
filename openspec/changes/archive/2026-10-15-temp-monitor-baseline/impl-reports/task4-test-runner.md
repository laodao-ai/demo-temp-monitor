# Task 4 实现记录：测试入口

产物：`scripts/test.sh`。起点 SHA `38bdde5`。

## 实现要点（tasks 4.2）

- 开头用 `BASH_SOURCE` 切到仓根，不依赖外部 `dirname`，缺工具检查先于任何外部命令。
- 参数缺省 `all`；非三值或多于一个参数，打印 `usage: scripts/test.sh [unit|integration|all]` 退出 2。
- 缺 `go` / `jq`：`required tool not found: <工具>; install it and make sure it is on PATH`，退出 1，无汇总行。
- 两层命令与 design 一致：unit `go test -json -count=1 -race ./...`；integration `go test -json -count=1 -p 1 -tags integration -run '^TestIntegration' ./...`。两层分别执行、计数，再求和。
- go test 的 stderr 不进 jq，直接透传。JSON 写临时文件，jq `-s` 一次算 `N M K`，再一次取要打到 stderr 的内容（失败用例 output、`build-output`、只有包级 fail 的包的包级 output）。
- 计数：只数带 `Test` 的 pass/fail/skip；「层 + 包」有包级 fail 且无用例级 fail 时 M 加 1；`go test` 非零退出且 M = 0 时 M 加 1 并打 `go test exited with status <n> but reported no failure; see the go test output above`。
- 任一所选层 N + M = 0：打 `no tests executed in <层> layer; check the integration build tag and the TestIntegration name prefix`，仍输出汇总行，退出 1。
- jq 失败：`cannot parse go test -json output; see the jq error above`，退出 1，无汇总行。
- 退出码 `min(M,125)`。脚本不读 `config/local.env`，输出不含连接串。

## 4.3 场景实测（仓库临时副本，已删除，未提交）

下表每行的期望来自 spec / design 原文，实测输出摘自运行记录。

| 场景 | 期望 | 实测 |
|---|---|---|
| 基线连跑两次 unit（TST-01 无缓存） | 两次总数相同 | 两次均 `144 passed, 0 failed, 0 skipped (144 total)`，exit 0 |
| 独立核对 N | 与 `go test -v` 的 `--- PASS` 行数一致 | `grep -c '^\s*--- PASS'` = 144 |
| 编译失败（service 包加未定义符号的测试文件） | M ≥ 1，退出非零，stderr 含编译错误行 | `133 passed, 1 failed, 0 skipped (134 total)` exit 1；stderr：`internal/service/zz_broken_test.go:3:28: undefined: undefinedSymbol` 与 `FAIL ... [build failed]` |
| 无测试文件的包不计入 K | K 不变 | 基线中 `internal/store`、`internal/store/storetest` 产生包级 skip 事件，K 仍为 0；另新增 `internal/emptypkg`（仅 f.go）后仍 `144 passed, 0 failed, 0 skipped (144 total)` exit 0 |
| 子测试计数 | N 增加 3 | 加 1 父 2 子的测试后 `147 passed, 0 failed, 0 skipped (147 total)` exit 0 |
| 顶层用例全部跳过（无子测试） | stderr 含 `no tests executed`，退出 1 | 删光其它测试，只留一个 `t.Skip`：`0 passed, 0 failed, 1 skipped (1 total)` exit 1；stderr：`no tests executed in unit layer; check ...` |
| `-run` 全部过滤掉 | 退出 1 | integration 层测试函数改名后 `0 passed, 0 failed, 0 skipped (0 total)` exit 1；stderr：`no tests executed in integration layer; check ...` |
| 130 个失败子测试 | 退出 125 | `144 passed, 131 failed, 0 skipped (275 total)` exit 125（130 子 + 1 父） |
| 3 个失败 | 退出 3，stderr 含名字和输出 | `144 passed, 3 failed, 0 skipped (147 total)` exit 3；stderr 含 `TestZZFail1..3` 与 `boom-one/two/three` |
| 非法参数 | 退出 2，打印用法，不运行测试 | `scripts/test.sh e2e` 与 `scripts/test.sh unit extra` 均打印 `usage: scripts/test.sh [unit|integration|all]`，exit 2 |
| all 模式 integration 层零用例（build tag 改成 `integratio`） | stderr 含 `no tests executed in integration layer`，退出 1，stdout 末行为汇总 | stdout：`144 passed, 0 failed, 0 skipped (144 total)`；stderr：`no tests executed in integration layer; check ...`；exit 1 |
| `go test` 被 `kill -9` | 退出非零 | 用 `pgrep` 找到 `go test -json -count=1 -race ./...`（pid 66950，测试含 15 秒 sleep），运行 6 秒后 `kill -9`：脚本 exit 1；stdout `133 passed, 1 failed, 0 skipped (134 total)`；stderr：`scripts/test.sh: line 73: 66950 Killed: 9 ...` 与 `go test exited with status 137 but reported no failure; see the go test output above` |
| 缺 jq（PATH 只含 go） | 退出 1，无汇总行 | exit 1；stdout 空；stderr `required tool not found: jq; install it and make sure it is on PATH` |
| 缺 go（PATH 只含 jq） | 退出 1，无汇总行 | exit 1；stdout 空；stderr `required tool not found: go; install it and make sure it is on PATH` |
| 从别的目录（`/`）以绝对路径调用 | 结果同仓根调用 | `144 passed, 0 failed, 0 skipped (144 total)` exit 0 |

jq 解析失败（`cannot parse go test -json output`）的分支未单独构造场景：go 1.27.1 下 `go test -json` 在编译失败时仍输出合法 JSON 事件，没找到稳定的触发办法。该分支是 `jq` 非零退出时的两行代码，未实测。

## 6.1

```
$ scripts/test.sh unit        # 环境无 TM_* 变量，worktree 无 config/local.env
144 passed, 0 failed, 0 skipped (144 total)
exit=0
```

## 6.2

无数据库配置（worktree 里没有 `config/local.env`，环境无 `TM_*`）：

```
$ scripts/test.sh integration
=== RUN   TestIntegrationEndToEnd
    e2e_integration_test.go:29: TM_DATABASE_URL is not set; copy config/example.env to config/local.env and fill it in
--- FAIL: TestIntegrationEndToEnd (0.00s)
=== RUN   TestIntegrationContract
    pgstore_integration_test.go:24: TM_DATABASE_URL is not set; copy config/example.env to config/local.env and fill it in
--- FAIL: TestIntegrationContract (0.00s)
=== RUN   TestIntegrationConstraints
    pgstore_integration_test.go:31: TM_DATABASE_URL is not set; copy config/example.env to config/local.env and fill it in
--- FAIL: TestIntegrationConstraints (0.00s)
0 passed, 3 failed, 0 skipped (3 total)
exit=3
```

连接串指向不可达端口（`TM_DATABASE_URL=postgres://u:pw@127.0.0.1:1/temp_monitor_demo?sslmode=disable&connect_timeout=5`，仅临时设环境变量）：

```
$ scripts/test.sh integration
=== RUN   TestIntegrationEndToEnd
    e2e_integration_test.go:29: cannot connect to database; check the SSH tunnel and that the URL uses the session pool port 7432
--- FAIL: TestIntegrationEndToEnd (0.00s)
=== RUN   TestIntegrationContract
    pgstore_integration_test.go:24: cannot connect to database; check the SSH tunnel and that the URL uses the session pool port 7432
--- FAIL: TestIntegrationContract (0.00s)
=== RUN   TestIntegrationConstraints
    pgstore_integration_test.go:31: cannot connect to database; check the SSH tunnel and that the URL uses the session pool port 7432
--- FAIL: TestIntegrationConstraints (0.00s)
0 passed, 3 failed, 0 skipped (3 total)
exit=3
```

输出不含连接串或口令（`pw`）。

## 验收对照

- 4.2：见「实现要点」。
- 4.3：见场景表。
- 6.1：N = 144，exit 0。
- 6.2：两种情况 M = 3（连真库层 3 个顶层用例），K = 0，exit 3；无配置时失败信息含 `copy config/example.env to config/local.env`。

## Concerns

- 起手时 worktree 的 HEAD 是 `f08aeab`（仓初始提交），不是任务给的起点 `38bdde5`。我在干净的 worktree 分支上 `git reset --hard 38bdde5` 对齐后开工。
- jq 解析失败分支未实测，原因见 4.3 末尾。

## 票外发现

无。
