### Task 4: 测试入口

**Blocked-by:** 1, 3
**R-ID:** TST-01, TST-02, TST-03, TST-04, TST-05, TST-06

`scripts/test.sh [unit|integration|all]` 统一执行两层测试并给出汇总行 `N passed, M failed, K skipped (T total)`。计数口径不把编译失败、零用例、进程被杀算成绿，退出码跟随失败数。

- [ ] tasks 4.2：按 design「Test Runner」实现该条列出的全部要求
- [ ] tasks 4.3：在仓库的临时副本（不提交）里逐条验证该条列出的全部计数场景，每条场景的实测输出贴进本票报告
- [ ] tasks 6.1：无数据库配置时 `scripts/test.sh unit` 退出 0、N > 0，汇总行贴进本票报告
- [ ] tasks 6.2：`scripts/test.sh integration` 在「无数据库配置」与「连接串指向不可达端口」两种情况下，连真库层用例都计入 M、退出码非零；无配置时 K = 0 且失败信息提示复制 `config/example.env`；两段输出贴进本票报告。执行时临时移开 `config/local.env`（若存在）并在结束后放回，不改其内容

