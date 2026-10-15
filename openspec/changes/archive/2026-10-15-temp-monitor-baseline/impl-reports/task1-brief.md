### Task 1: 配置与存储契约

**Blocked-by:** none
**R-ID:** RUN-01, RUN-02, RUN-03, RUN-05, DEV-01, DEV-02, DEV-03, DEV-04, ING-01, ING-02, TST-05, TST-07

建立 Go 模块骨架。服务的三个配置项能按「环境变量 > `config/local.env` > 缺省」取值，文法错误给出文件名和行号。仓里有示例配置，本机配置被 git 忽略。定义存储接口与哨兵错误，提供内存实现，并有一套两个实现共用的契约测试；内存实现在 `-race` 下通过它。不需要数据库，不经 `scripts/`，`go test -race ./...` 即可验证。

- [ ] tasks 1.1：module `github.com/laodao-ai/demo-temp-monitor`、`go 1.27`、引入 pgx v5，`go build ./...` 通过
- [ ] tasks 1.2：`.gitignore` 挡住 `config/local.env` 与 `.pg-ops/`（两行已在 `21d7b25` 加入，核对即可），建一个 `config/local.env` 后 `git status --porcelain` 不显示它
- [ ] tasks 1.3：`config/example.env` 三个键为占位值，注释写明连 session 池端口 7432、口令做 URL 编码
- [ ] tasks 1.4：配置解析与校验，单元测试覆盖该条列出的全部情形（三种优先级、文件不存在、注释与空行、两种错误、值含 `=` 与 `&`、空值、带引号、无 `=`、未知键、CRLF、阈值 `NaN` / `Inf` / `-Inf`）
- [ ] tasks 2.1：存储接口、`Device`、`Reading`、`ErrNotFound`、`ErrAlreadyExists`
- [ ] tasks 2.2：契约测试套件覆盖 TST-07 的九项行为，并发注册 20 个 goroutine，套件内部生成 `it-<随机后缀>` ID，容忍存储里已有别的数据
- [ ] tasks 2.3：内存实现（一把 `sync.Mutex`，`ListReadings` 返回副本）跑契约套件，`go test -race ./...` 通过

