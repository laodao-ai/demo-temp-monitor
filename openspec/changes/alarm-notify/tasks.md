## 1. 配置面：三个新键（切片 T1）

- [ ] 1.1 `internal/config`：新增常量 `KeyDingTalkWebhook`、`KeyDingTalkSecret`、`KeyNotifyCooldown`，导出有序列表 `Keys`（六个键），`knownKeys` 由 `Keys` 派生；`Config` 新增 `DingTalkWebhook`、`DingTalkSecret`、`NotifyCooldown`（缺省 10 分钟）〔RUN-01、NTF-03〕
- [ ] 1.2 `internal/config`：新增哨兵错误与校验——webhook 须为带主机的 http/https 绝对 URL；冷却须能被 `time.ParseDuration` 解析且大于 0；配了 secret 必须配 webhook；错误消息只含键名〔RUN-03〕
- [ ] 1.3 `internal/config` 单元测试：新键的优先级与空值、缺省冷却 10 分钟、`TM_NOTIFY_COOLDOWN=90s`、三类非法值（含 `ftp://`、相对路径、`0`、`-1m`、`ten`、只配 secret）且错误文本不含配置值〔RUN-01、RUN-03〕
- [ ] 1.4 `cmd/temp-monitor` 的 `configHint` 与 `internal/testdb` 的 `configFailure`：allowed keys 改由 `config.Keys` 拼出；三个新错误各配提示，整行文案与 RUN-03 场景逐字一致；同步改 `main_test.go`、`testdb_test.go` 中的期望文案〔RUN-01、RUN-03〕
- [ ] 1.5 `scripts/lib/local_env.sh`：`case` 与 unknown key 提示加入三个新键〔RUN-01〕
- [ ] 1.6 新增 Go 单元测试：用 bash 调 `scripts/lib/local_env.sh` 的 `local_env_get`，① 含三个新键的 local.env 不报错且读出的 `TM_DATABASE_URL` 不变 ② `FOO=bar` 时 stderr 的 allowed keys 与 `strings.Join(config.Keys, ", ")` 相同〔RUN-01〕
- [ ] 1.7 `config/example.env`：加 `TM_DINGTALK_WEBHOOK=`、`TM_DINGTALK_SECRET=`（留空，各一行注释）与 `TM_NOTIFY_COOLDOWN=10m`；加单元测试读取 `config/example.env` 断言两推送键为空、冷却为 `10m`，且以它为 local.env（补连接串）加载后 webhook 为空；README 配置表改为六项并说明推送〔RUN-05、RUN-01〕

## 2. 脱敏：推送密钥（切片 T2）

- [ ] 2.1 `internal/redact`：`Handler(inner, connURL string, secrets ...string)`，空串忽略；额外密钥在消息、字符串属性、分组成员、error 值中全文替换为 `***`；口令规则不变；现有调用不改〔RUN-04〕
- [ ] 2.2 `internal/redact` 单元测试：webhook 全串、`access_token` 值、secret 各出现在消息 / 字符串属性 / 分组 / error 值中均被替换；原有 B1 回归测试（口令为常见词）仍通过〔RUN-04〕

## 3. 钉钉发送方（切片 T3）

- [ ] 3.1 新建 `internal/notify`：`Alarm`、`Notifier`、`Sender` 接口、`Nop`〔NTF-01、NTF-06〕
- [ ] 3.2 `notify.DingTalk`（实现 `Sender`）：组装 text 消息（`超温告警`、设备 ID、温度、阈值、RFC 3339 时间、冷却说明）与 `at.isAtAll=true`；POST `application/json`〔NTF-01〕
- [ ] 3.3 `notify.DingTalk` 加签：可注入时钟；配置 secret 时追加 `timestamp`、`sign`，保留原有查询参数；未配置时 URL 原样〔NTF-04〕
- [ ] 3.4 `notify.DingTalk` 成功判定：仅 2xx 且 JSON `errcode==0` 为成功；响应体最多读 64 KiB；`*url.Error` 只包装其 `.Err`〔NTF-05、RUN-04〕
- [ ] 3.5 `notify` 单元测试（httptest）：请求体字段；加签测试向量（secret `SECtest0000000000000000000000000000000000000000000000000000000000`、ts `1700000000000` ⇒ `szPcgtoCabDjIT69N8i2Omun8ktW8akfg42aQqadSCU=`）；不加签时只有 `access_token`；200+errcode 0 成功；200+errcode 1、502、非 JSON、端口无人监听、挂起超时均返回 error；返回的 error 文本不含 `access_token` 值〔NTF-01、NTF-04、NTF-05、RUN-04〕

## 4. 冷却与接入（切片 T4）

- [ ] 4.1 `notify.Throttled`：`sync.Mutex` + `map[string]time.Time`，判断与记录在同一临界区；派发时记录、失败不回滚；放行后起 goroutine，用 `context.Background()` 派生的 5 秒 ctx 调 `Sender.Send`；成功写 INFO `alarm notification sent`、失败写 ERROR `alarm notification failed`、抑制写 DEBUG `alarm notification suppressed`；可注入时钟；`Wait()` 供测试等待在途推送〔NTF-02、NTF-03、NTF-05〕
- [ ] 4.2 `notify.Throttled` 单元测试（假 Sender + 注入时钟，`-race`）：冷却内抑制、到期再推、不同设备独立、20 条并发只放行 1 次、失败也开始冷却、`Notify` 在 Sender 阻塞时立即返回〔NTF-02、NTF-03〕
- [ ] 4.3 `service.New` 加参数 `n notify.Notifier`；`Report` 在写 WARN 后调用 `Notify(Alarm{...RecordedAt: r.RecordedAt})`；更新 `service.go` 注释；改 4 个调用点（main、`httpapi_test.go`、`e2e_integration_test.go`、`service_test.go`）〔NTF-01、NTF-07〕
- [ ] 4.4 `internal/service` 单元测试（记录型假 Notifier）：超温调用一次且字段正确；等于 / 低于阈值、未注册、ID 非法、存储出错均不调用；WARN 日志断言保持原样〔NTF-01、NTF-07、ALM-01~04 回归〕
- [ ] 4.5 `cmd/temp-monitor` 装配：webhook 为空用 `notify.Nop{}` 并写 INFO `notify disabled`；否则用 `Throttled(DingTalk)` 并写 INFO `notify enabled`（`cooldown`、`signed`）；`redact.Handler` 传入 webhook 全串、解析出的 `access_token` 值、secret〔NTF-06、RUN-04〕
- [ ] 4.6 `cmd/temp-monitor` 单元测试（假 open + 内存存储 + httptest 假钉钉）：未配置时 `notify disabled` 且无出站请求；配置时启动日志 `cooldown=5m0s signed=true` 且不含 webhook / secret；假钉钉挂起时上报 201 且耗时 < 1 秒；假钉钉 500 时上报 201；webhook 指向无人监听的端口时 stderr 有 `alarm notification failed` 且不含 token、secret、webhook 全串〔NTF-02、NTF-05、NTF-06、RUN-04〕
- [ ] 4.7 `cmd/temp-monitor` 单元测试：存储写入出错后恢复，第一次 500 且无推送，第二次 201 且推送一次〔NTF-07〕

## 5. 端到端、约定同步与真机冒烟（切片 T5）

- [ ] 5.1 扩展 `internal/httpapi/e2e_integration_test.go` 的 `TestIntegrationEndToEnd`：接 httptest 假钉钉，超温上报后断言假钉钉在 5 秒内收到一次请求，`text.content` 含设备 ID〔NTF-01、NTF-02〕
- [ ] 5.2 `openspec/config.yaml` 本项目 context：把「模块只有四个……超温告警写日志……不加表外功能」改为包含钉钉推送的说法〔C5，proposal Compliance〕
- [ ] 5.3 `openspec/specs/over-temperature-alarm/spec.md` 的 Purpose：去掉「不做……通知」，改为指向 `alarm-notification`（Purpose 不在 delta 内，直接改主 spec）〔C5〕
- [ ] 5.4 `scripts/test.sh all` 全绿，贴汇总行；`go vet ./...` 无输出〔全部〕
- [ ] 5.5 人工真机冒烟（由人执行）：测试群建自定义机器人，① 安全设置选「加签」，配 webhook 与 secret 启动，触发一条超温读数，群里看到消息且 @所有人 ② 改为关键词「超温告警」、清空 secret 后重启，再触发一条，同样看到 ③ 结果（时间、两次是否成功、日志中的 `alarm notification sent`）记进 hand-off〔NTF-01、NTF-04、NTF-05，proposal A1〕

## 测试覆盖图

```
 代码路径                                         测试类型                 需求
 -----------------------------------------------  -----------------------  -----------------------
 config.Load 新键 / 缺省 / 校验 / 不回显           单元 config_test         RUN-01 RUN-03
 main.configHint, testdb.configFailure 文案        单元 main_test/testdb    RUN-01 RUN-03
 scripts/lib/local_env.sh 新键 + allowed keys      单元(Go 调 bash)         RUN-01
 config/example.env 内容                           单元(读文件断言)          RUN-05
 redact.Handler 额外密钥                           单元 redact_test         RUN-04
 notify.DingTalk 消息体 / 加签 / 成功判定          单元 httptest            NTF-01 NTF-04 NTF-05
 notify.DingTalk *url.Error 不带 token             单元 httptest            RUN-04
 notify.Throttled 冷却 / 并发 / 非阻塞             单元 假 Sender + -race   NTF-02 NTF-03
 service.Report -> Notifier                        单元 假 Notifier         NTF-01 NTF-07
 main.run 装配 / 启动日志 / 隔离 / 脱敏            单元 假 open + httptest  NTF-02 NTF-05 NTF-06 NTF-07 RUN-04
 HTTP -> PG -> 推送 端到端                         集成 TestIntegration...  NTF-01 NTF-02
 与真钉钉的加签、关键词、@所有人                    人工真机冒烟             NTF-01 NTF-04 NTF-05
```

## 依赖顺序

```
 1 (配置) --+
 2 (脱敏) --+--> 4 (冷却与接入) --> 5 (端到端/约定/冒烟)
 3 (发送方) +
```

关键路径：3 → 4 → 5（3 的加签与成功判定是 4.6、5.1、5.5 的前提）。
