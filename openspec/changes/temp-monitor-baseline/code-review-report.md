---
ship-gate:
  code_review: pass
  reviewed_sha: 891c06e4a81a673543b12e4853d819f9493fddaec5d91d5b96af08faa8faf306
  reviewed_manifest: MDQwMDAwIHRyZWUgYjY5NjdhN2E3ZjZkYjNmMDNkOTQyMjdhNzZjOTY3ZjAzOGQxMTk0MAkuYWdlbnRzADA0MDAwMCB0cmVlIDQ4MWE5YzRlZDg1NjY0YWM5Y2Q4ZWI2ZjY2ODQ1MjNiMThmNzg5N2EJLmNsYXVkZQAxMDA2NDQgYmxvYiA5YzU0NjMyMGU5NGE4NjY5MjZhMDY0NThkNGIzNWIyMmExNzUyOWU0CS5jbGF1ZGVpZ25vcmUAMTAwNjQ0IGJsb2IgZDc5ZmQ1OTI1Yzg3YzYwOTU2YWNmNGVhZjQyNDZmNDEyZDA2ZjllZAkuZ2l0aWdub3JlADEwMDY0NCBibG9iIDg3MjIwMWRmMjZlYTM4NmMwMWRjNWI4MWIwMmNjYmMyY2M3N2ExNTcJQUdFTlRTLm1kADEwMDY0NCBibG9iIDZjMWU4YzNiZTkxYTIzYmEwMzRkNGNjNzAyOTJiMGQ5MWRlMTU5YjIJQ0xBVURFLm1kADEwMDY0NCBibG9iIGFjZjMxM2Y5MjRhMWNhNmMyMTdkMmI5YzhiOTFjZGEzZTU1ODNhZDgJUkVBRE1FLm1kADA0MDAwMCB0cmVlIDE1Njk2ZGYyOTE4MjZjYmNmZTA4NDNkODBlNjUyZDY0ZDJmYmQyNzUJY21kADA0MDAwMCB0cmVlIGUzOTA1NmZkMzlkNmQ0NTFkMDc1MjBiZmQzOGFiNTJiMmJmOGQ0MzIJY29uZmlnADA0MDAwMCB0cmVlIGM0M2MxZjE1MDUzNjExYzQxYzc1ZTliN2Y1MDExNDYwMjIxOTMwYTAJZGIAMTAwNjQ0IGJsb2IgMjI0MDljMDgxNTFkNTgzOTI1NzdhZmQxOTQyYTc4NDkzMGVmZTRlMglnby5tb2QAMTAwNjQ0IGJsb2IgM2Y3MTZkZDEyNTIzYjYxOTQyMzRmMGJiMjdhNmNmOTgwZGUwODQ2YQlnby5zdW0AMDQwMDAwIHRyZWUgOWNjNDZkZjFkYTU1NGM5MjNhMGNjZDA4OTc2OWY0NmZkOWUzOGQ3NwlpbnRlcm5hbAAwNDAwMDAgdHJlZSAwNDkzM2I0ZGI4ZjMzMGIxZjBmMDJiNDAwMTE1YjE4NzQzYjExYzg0CXNjcmlwdHM=
---
## code-review 报告 — temp-monitor-baseline

被审范围：`f08aeab..d0b6fb4`（排除 `openspec/`），34 个文件，3466 行新增；外加自动修复提交 `d0b6fb4`。

### 命中范围

- 栈：backend · go（Go 1.27、pgx v5、net/http、log/slog），bash 脚本，PostgreSQL 迁移与种子。
- 清单：`code-review-base.md` CR-01~11，`domains/backend.md`，`domains/backend-go.md`。
- Step1 自持 scope 审计：
  - 完成度：tasks.md 1.1–5.1 判 DONE（4.3 判 UNVERIFIABLE，证据在 task4 报告）。6.1–6.6 是外部执行项，判 UNVERIFIABLE，各有 impl-reports 旁证。没有 NOT DONE 或 PARTIAL。
  - scope-drift：只报出 `.claude/settings.json` / `.claudeignore` 一项（裁决见「已裁掉」X5）。Non-Goals 没有被实现。
  - 切片偏离对账：design.md「切片建议」六张与 tickets.md Task 1–6 逐张对应。Task 7 是收尾票。`planning-decisions.md` 没有「切片偏离」行，无偏离需要对账。
- trivial_shape：NOT_EXEMPT（`new-codepath:.claude/settings.json`），Step2 照常 fan-out。
- 历史镜本轮条件未命中，按处置表降采样跳过（判据：diff 无 rename，也没有改动 ≥200 行的既有文件）。

<!-- sdflow:fanout-capability v1 host="claude" subagents="available" mirrors="broad,domain,adversarial" -->
<!-- sdflow:step1-broad-review v1 mode="subagent" -->
<!-- sdflow:hr-tg v1 hit="TG-04,TG-07,TG-08,TG-17,TG-26" declared="TG-01,TG-04,TG-05,TG-07,TG-08,TG-10,TG-13,TG-14,TG-15,TG-17,TG-18,TG-19,TG-20,TG-21,TG-22,TG-23,TG-26,TG-28" evidence="新建两张表；新增三个 HTTP 路由；依赖 PostgreSQL（PgBouncer session 池）；连接串含口令且仓库将公开；memstore 互斥锁与并发注册、并发注销上报" -->
<!-- sdflow:declared-sites v1 declared="code-voice,hr-tg" -->
<!-- sdflow:outside-voice v1 site="code-voice" guard="none" host="claude" runner="codex" reason_code="ok" findings="2" truncated="false" -->
<!-- sdflow:outside-voice v1 site="hr-tg" guard="none" host="claude" runner="codex" reason_code="ok" findings="2" truncated="false" -->

派出的镜：
- Step1 scope 审计 1 个。
- 领域镜 2 个：backend、backend-go。
- 对抗镜 3 个：并发竞态、资源泄漏、错误路径。
- 跨模型 voice 2 路：code-voice、hr-tg。两路都以 async 方式在主 session 派出，`.rc` 均为 0。

`metrics.enabled=false`，所以本报告不落 lens-metric 锚和 ref-check 锚。机械引用核仍然跑了：合并池 15 条，pass 12，fail 0，uncheckable 3（证据包形态）。

### Findings（已采纳，按置信降序）

| # | 严重度 | CR | 位置 | 问题 | 来源 | 处置 |
|---|---|---|---|---|---|---|
| A1 | Important | CR-04 | `cmd/temp-monitor/main.go:122` | `Shutdown` 超时后没有关闭在途连接，关池时还要等借出的连接归还。退出可能拖到约 10 秒，超出 RUN-06「最多等待 5 秒」的要求 | 领域镜 backend、code-voice、hr-tg voice | 已修 [impl-review-fix]：超时后调用 `srv.Close()`，新增测试 `TestShutdownTimeoutCancelsInFlightRequest`（先红后绿） |
| A3 | Important | CR-07 | `internal/redact/redact.go:48`、`scripts/db-reset.sh` | README 要求特殊字符口令做 URL 编码。这时脱敏只替换编码形式，错误文本里如果出现解码后的口令，会原样输出 | code-voice | 已修 [impl-review-fix]：解码形式也替换为 `***`；Go 侧新增测试，先红后绿；bash 侧在临时副本实测 |
| A5 | Minor | CR-01 | `scripts/db-reset.sh:58` | PATH 里没有 psql 时，`2>/dev/null` 吞掉 command not found，脚本误报去查 SSH 隧道 | 对抗镜·错误路径（已复现） | 已修 [impl-review-fix]：先 `command -v psql`，缺失时打印 `required tool not found: psql; ...`，退出 4 |
| A6 | Minor | CR-02 | `scripts/lib/local_env.sh:56` | `config/local.env` 不可读时函数仍返回 0。`simulate.sh` 回落默认地址继续跑，`db-reset.sh` 误报缺 URL | 对抗镜·错误路径（已复现） | 已修 [impl-review-fix]：打印 `config/local.env: cannot read file`，两个脚本都退出 1 |
| A4 | Important | CR-07 | `internal/redact/redact.go:69` | slog 包装对 Message 和所有字符串属性做口令子串替换。口令很短或是常见子串（如 `temp`）时，固定告警文案会变成 `over-***erature alarm` | 对抗镜·错误路径（已复现） | defer，见台账 B1 |
| A2 | Minor | CR-BE-02 | `cmd/temp-monitor/main.go:109` | 只设了 `ReadHeaderTimeout`，读请求体没有时限 | 领域镜 backend、hr-tg voice | defer，见台账 T27 |
| A7 | Minor | CR-07 | `.claudeignore:1` | AI 上下文排除和 Read deny 没有覆盖 `config/local.env` | 领域镜 backend | defer，见台账 T28 |
| A9 | Minor | CR-01 | `scripts/simulate.sh:49` | curl 会走 `http_proxy`，提示有误导 | 领域镜 backend | defer，见台账 T29 |
| A11 | Minor | CR-01 | `internal/store/pgstore/pgstore.go:99` | pgx 错误原样返回，没有加操作名 | 领域镜 backend-go | defer，见台账 T30 |
| A13 | Minor | CR-01 | `internal/config/config.go:76` | 读文件失败时丢弃 errno | 领域镜 backend-go | defer，见台账 T31 |

复审第 1 轮的范围限定为修复提交 `d0b6fb4`，结论 CLEAN：4 条修复都解决了问题，没有 scope-drift。复审的 4 条 Minor 合成台账 T32。修复后连真库 integration 层 `16 passed, 0 failed`。修复子代理跑的 unit 层为 `146 passed, 0 failed`。

### 已裁掉（反静默压制，可审计）

- X1 A8 `scripts/db-reset.sh:21`：环境变量有值时不校验 `config/local.env` 的文法。裁掉理由：design「Demo Database Reset」第 1 步明文规定「环境变量非空时直接用它，不读文件」，这是设计行为。
- X2 A10：main 与 testdb 的提示映射重复、testdb 缺阈值分支。裁掉理由：已在池 T16，重复。
- X3 A12：redact 对非 error 的 KindAny 不脱敏。裁掉理由：已在池 T14，重复。
- X4 A15：Step1 报 6.5「端口被占用退出 1」证据缺失。裁掉理由：`task6-remote-acceptance.md:109-112` 有该项命令与输出（`bind: address already in use`）。
- X5 A14 `.claude/settings.json`、`.claudeignore`：Step1 判为 SCOPE-CREEP。裁掉理由：这两个文件由人显式调用的 `/pg-dev-init`（`pgops-guard.sh`）生成，用来挡住含口令的 `.pg-ops/`，在人明确要求的提交 `21d7b25` 中入库。它们属于开发工具配置，不是产品范围。
- 对抗镜·并发竞态、对抗镜·资源泄漏：无爆点（空数组），各自列了已核对的点。

### 修复 / defer 台账

- 自动修 4 项 [impl-review-fix]，见上表 A1、A3、A5、A6。
- 自动选推荐 0 项，没有需要 `T10-choice` 的多方案项。
- A4 存在两种方案（只脱敏 error/驱动文本，或短口令退化），没有客观判据，也涉及设计取舍，所以 defer，不走自动选。

本轮新增待处理项如下，recorder 已确认各自 `source_change` = 本 change。

| id | 池 | 摘要 | critique（裁决理由） |
|---|---|---|---|
| B1 | bug | 短口令或常见子串口令会被替换进固定日志文案与 device_id | 已复现，属本 change 引入；修法需在两种方案间定设计取舍，口令由服务器随机生成时概率低，故 defer |
| T27 | todo | 读请求体无时限，建议加 ReadTimeout/IdleTimeout | design 只规定了 ReadHeaderTimeout；服务只监听 127.0.0.1，改服务端超时策略属设计调整 |
| T28 | todo | `.claudeignore` 与 settings deny 未覆盖 `config/local.env` | 工具配置，不在产品范围；价值真实 |
| T29 | todo | simulate.sh 的 curl 未加 `--noproxy` | 只在设了代理的开发机出现，影响是提示误导 |
| T30 | todo | pgstore 错误未加操作名 | 日志已有 route 与 device_id，定位影响小 |
| T31 | todo | config 读文件失败丢弃 errno | 文案能定位文件，缺原因细节，影响小 |
| T32 | todo | 修复复审 Minor：bash 解码告警、design 文案行、脚本测试、测试时限余量 | 复审判 CLEAN，均为边角 |

### 结论

- [x] 建议进 /sdflow-done
- [x] 本轮新增待处理项已入池（见上方台账，hand-off 会引用）
