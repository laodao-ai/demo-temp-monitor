### Task 5: 重建脚本、模拟脚本、README

**Blocked-by:** 1, 3
**R-ID:** DB-01, DB-02, DB-03, ING-04, RUN-01, RUN-02, RUN-04, RUN-05, TST-01

`scripts/db-reset.sh` 只对实际连上的 `temp_monitor_demo` 库在单个事务里从零重建，退出码按 design「Demo Database Reset」。`scripts/simulate.sh [rounds]` 注册三台模拟设备并按轮上报，其中一台每轮超温。两个脚本读 `config/local.env` 时按同一套文法逐行解析，不执行文件内容。README 说明项目、前置工具、配置方法、启动顺序与测试用法。

- [ ] tasks 4.1：重建脚本，含该条列出的全部要求，以及不连库可做的四项验证（不可达端口退出 4 且只有固定文案；两条 psql 都带同一连接串；四个特殊字符口令不泄漏；`$(touch marker)` 不产生 marker），输出贴进本票报告
- [ ] tasks 4.4：模拟脚本，含该条列出的全部要求；验证服务未运行时退出 1、`rounds` 为 `0` 或 `abc` 时退出 2、`$(touch marker)` 不产生 marker，输出贴进本票报告
- [ ] tasks 5.1：README 含该条列出的全部内容；「测过 PostgreSQL 18」这一句先不写，或写成待确认，由 Task 6 在 6.4 通过后补上

