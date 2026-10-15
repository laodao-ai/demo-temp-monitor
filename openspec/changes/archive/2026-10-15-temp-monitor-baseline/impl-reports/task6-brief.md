### Task 6: 远端库验收

**Blocked-by:** 1, 2, 3, 4, 5
**R-ID:** DB-01, DB-02, DB-03, DB-04, RUN-02, RUN-03, RUN-04, RUN-05, RUN-06, ING-04, ALM-01, TST-01, TST-06, TST-07, TST-08, TST-09

在远端开发库上完成连真库验收：重建脚本、全量测试、模拟上报与服务生命周期都按 spec 工作，然后审计全部提交历史。前置条件是 SSH 隧道已起、`config/local.env` 已填好；不满足时报 `BLOCKED`（等远端库），不勾任何一条（设计门 Q1 选 A）。`config/local.env` 含口令：MUST NOT 把它的内容打印到任何输出或报告里；工作目录不是主仓根时（如独立 worktree），只用 `cp` 从主仓根复制这个文件，用完删除副本。

- [ ] tasks 6.3：该条列出的全部七项验证，输出贴进本票报告
- [ ] tasks 6.4：`scripts/test.sh`（all）汇总 `N passed, 0 failed, K skipped (T total)`、N > 0、退出 0，之后库里没有 `it-` 开头的设备
- [ ] tasks 5.1 补句：6.4 通过后，README 写明测过 PostgreSQL 18
- [ ] tasks 6.5：该条列出的全部验证（`simulate.sh 3`、WARN 条数与不含口令、Ctrl-C 与 SIGTERM 退出 0、只监听 `127.0.0.1`、端口被占用时退出 1）
- [ ] tasks 6.6：审计全部提交历史，IPv4、连接串与口令、邮箱三项按该条做法无命中；公司名和客户名一项需要人在执行时临时输入关键词，不写入任何文件——这一项报 `NEEDS_CONTEXT` 交编排层向人取关键词

