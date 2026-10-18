# spec-review-report · alarm-notify

评审日期：2026-10-17 · 评审对象：`openspec/changes/alarm-notify/` 四件套（proposal / design / specs ×2 / tasks）+ `decision-memo.md`（评审基线提交 `fd14628`）
主 session：强档（opus，effort high）· 宿主 `claude` · 规则根 `~/.sdflow/workflow`

## 结论

**无 blocker，建议修订后进设计 HARD-GATE。** 9 面镜（8 个子代理镜 + 2 个跨模型 voice 站点）共报约 60 条，合并去重后 **35 条**：

- **采纳 25 条**：已直接修订进四件套，标 `[spec-review-amendment]`；
- **需拍板 1 条**：Q1，已按推荐方案先行修订，人可推翻；
- **裁掉 9 条**：理由见「已裁掉」区。

最重要的 4 条在 ≥3 面镜独立命中，跨模型 voice 也独立报出：

1. **M1** 响应里没有 `errcode` 时会被当成推送成功（6 面镜）；
2. **M2** 数据库连接串不带口令时，脱敏 handler 会吞掉推送失败的 `error` 字段（4 面镜）；
3. **M3** 默认 `http.Client` 会跟随重定向（5 面镜）；
4. **M4** 推送 goroutine 里的 panic 会让整个服务进程退出（4 面镜）。

---

## 决策登记区

### 拍板三问

- **Q-scope 范围划界认不认？**
  - 自答：锚 proposal「Non-Goals」6 条。范围没有变化：值班表、多渠道、恢复通知、保证送达、全局限速、按设备阈值仍不做。
  - 本轮新增的都是既有范围内的防御深度：errcode 存在性、不跟随重定向、panic 兜底。另新增 ADR-0005 修订任务 2.3，属于合规补位，不是新功能。
  - [ ] 认  [ ] 不认
- **Q-deps 依赖 / 顺序认不认？**
  - 自答：锚 design「切片建议」与 tasks「依赖顺序」。T1 / T2 / T3 并行，然后 T4，然后 T5。
  - 本轮只做了两处归属调整：`Nop` 与包骨架从 T4 移到 T3（S-03）；ADR-0005 修订记录并入 T2。依赖图仍无环。
  - [ ] 认  [ ] 不认
- **Q-risk 风险赌注与对策认不认？**
  - 自答：锚 `sdflow:hr-tg` 判定 hit = TG-08 / 16 / 17 / 26。四类风险的对策：
    - 外部依赖（TG-08）：M1、M3、M10；
    - NFR 时序（TG-16）：M7；
    - 敏感数据（TG-17）：M2、M11；
    - 并发（TG-26）：M4、M8。
  - 剩余赌注不变：A1「钉钉加签与 errcode 语义只核到二手资料」靠 tasks 5.5 人工真机冒烟兜底。
  - [ ] 认  [ ] 不认

<!-- sdflow:gate-questions v1 q="scope,deps,risk" -->

### [需拍板]

**Q1 推送失败日志怎样绕开 ADR-0005 的「取不出口令 ⇒ error 值整体 withheld」**（M2，来源：plan-eng、backend-go、对抗 A、对抗 B）

事实：已核实 `internal/redact/redact.go:128-129`。`pwOK=false` 时，所有 error 值都被替换成 `error details withheld`。数据库连接串不带口令（如 `postgres://user@127.0.0.1/db`，本机 trust 认证）时，`alarm notification failed` 的 `error` 字段整条丢失，NTF-05 场景（`error` 含 `502` / errcode `1`）不成立。

- **推荐 A（已先行写进 design Security 节与 tasks 4.1、4.6）**：`Throttled` 把失败原因以**字符串属性** `"error", err.Error()` 记录。字符串属性会替换整条连接串和额外密钥，推送错误本身不含数据库口令。
  - 系统镜：只改 notify 一处，`redact` 的口令与 withheld 语义原样保留，与 ADR-0005 一致。
  - 用户镜：任何连接串形态下，值班和运维都能看到失败原因。
  - 开发循环镜：ADR-0005「已知边界」已提示「字符串属性不做口令替换」，需要在 4.1 注释写明本处为何例外。成本：一行注释。
- **备选 B**：改 `redact`，`pwOK=false` 时 error 值改做「连接串 + 额外密钥替换」，不再整体 withheld。
  - 系统镜：动 ADR-0005 已定的口令防线。驱动错误若含口令片段，在无口令可取的场景下会直接泄露。
  - 用户镜：同 A。
  - 开发循环镜：需要改 ADR 的语义，还要补口令回归测试。
- **主次判定**：系统镜为主，不削弱已定的口令防线。选 A。

### [自动决策]（高置信，已修订进四件套，默认接受，可覆盖）

| ID | 发现（命中镜） | 处置（修订位置） |
|---|---|---|
| D1 / M1 | `errcode` 解码到 `int` 时，`{}`、`null`、`{"errcode":null}` 会得零值 0，被判成功（design-voice、hr-tg voice、对抗 A、对抗 B） | design TA#8 改为解码到 `*int`、`Unmarshal` 整段解析；NTF-05 正文加「缺失 / null 视为失败」并新增场景「JSON 但缺 errcode」；tasks 3.4、3.5 |
| D2 / M3 | `&http.Client{}` 默认跟随重定向：307/308 会把正文和 token 再 POST 到别处，302 改成 GET 后会按最终响应误判（design-voice、hr-tg voice、plan-eng、backend-go、对抗 B） | design TA#10 加 `CheckRedirect` 返回 `ErrUseLastResponse`；NTF-05 加「MUST NOT 跟随重定向」并新增场景「不跟随重定向」；Failure Modes 加一行；tasks 3.4、3.5 |
| D3 / M4 | 推送 goroutine 没有 `recover`，panic 会终止整个进程，与 NTF-02 冲突（plan-eng、backend-go、对抗 A、对抗 B） | design TA#5 加 `recover` ⇒ ERROR；NTF-02 正文加「panic 不终止进程」并新增场景；Failure Modes 加一行；tasks 4.1、4.2 |
| D4 / M5 | design 声称对 ADR-0005「遵守」，实际改了 ADR 正文写定的 `Handler` 签名，还新增了替换规则（strategy） | design Compliance 改为「扩展」；新增 tasks 2.3（`/sdflow-adr` 追加修订记录）；切片 T2 同步 |
| D5 / M6 | `Throttled` 的 logger 来源没写；若回落到 `slog.Default()` 会绕过脱敏（plan-eng、strategy、backend-go） | design TA#6 写明 `NewThrottled(sender, cooldown, logger)`，logger 必须是经脱敏的那个；组件清单；tasks 4.1 |
| D6 / M7 | 5 秒超时写死，没有注入点，与 Failure Modes「注入短超时」矛盾；NTF-02 第二条 AND（5 秒内出现 ERROR）没有测试；`DingTalk` 的冷却值从哪来也没写（strategy、对抗 A、对抗 B、对抗 C） | design TA#6 加 `sendTimeout`（缺省 5 秒）与 `NewDingTalk(webhook, secret, cooldown)`；Failure Modes 超时行；tasks 3.2、4.1、4.2、4.6（6 秒内断言 ERROR，挂起的 handler 留释放通道） |
| D7 / M8 | 测试的并发纪律：`Wait()` 与并发 `Notify` 的 `Add` 竞争；`run` 层拿不到 `Wait()`；读 buffer 与写 buffer 构成 data race（backend-go、对抗 A、对抗 B、对抗 C） | design TA#5 写明 `Wait()` 的前提；tasks 4.2（先等全部 `Notify` 返回再 `Wait`，用并发安全的 buffer）、4.6（`lockedBuffer` + 带上限的轮询） |
| D8 / M9 | Non-Goals 第 4 条的补救「临时调低日志级别」不存在：`main.go:83` 用 `NewTextHandler(stderr, nil)`，级别固定 INFO（backend-go、对抗 A） | design Non-Goals 第 4 条改写为「需改代码」 |
| D9 / M10 | 没写 `resp.Body.Close`、`LimitReader`、`defer cancel`；用 `Decoder` 会放过尾部多余内容（backend-go、对抗 B） | design TA#8；tasks 3.4、4.1 |
| D10 / M11 | 多个密钥的替换顺序没定义：先替换 token 会破坏全串匹配，违反「全串替换为 `***`」；短 token `tok` 会误伤普通词（backend-go、对抗 C） | design Security 节加「按长度降序」；tasks 2.1、2.2（全串 ⇒ 恰为 `***`，测试用长 token） |
| D11 / M12 | 正文断言用 `55`、`50`、`57` 这类数字子串，会被 RFC 3339 时间或 `10m0s` 误命中（对抗 A、对抗 C） | tasks 3.5 改为整行断言、固定 `RecordedAt`。spec 场景保持「含」措辞不变（spec 不钉死正文格式，格式由 design TA#14 定） |
| D12 / M13 | `RecordedAt` 时区：pgstore 读出的 `timestamptz` 可能带本地时区；task 3.2 没写 `.UTC()`（对抗 C） | design TA#14；tasks 3.2 |
| D13 / M14 | `Query().Encode()` 会重排查询参数；不加签路径若也做往返，会违反 NTF-04「完全相同」（对抗 A、对抗 B） | design TA#7 写明不加签时直接用原串；tasks 3.3、3.5（多参数、非字典序 webhook 逐字比对） |
| D14 / M15 | 回退旧版本时，local.env 里的三个新键会让旧版报 `unknown key` 而无法启动，文档没提（devex） | design Migration Plan 第 4 步；tasks 1.7 README |
| D15 / M16 | 命名对照（`DINGTALK_` 与 `NOTIFY_` 前缀）、缺省 10 分钟的理由、冷却无 webhook 时为何不报错，都没写（devex） | design 新增 TA#15 |
| D16 / M18 | README 没要求写建机器人与安全设置，也没要求写冷却的格式（devex） | tasks 1.7 |
| D17 / M20 | delta RUN-01 新增的「提示列出六个键」接在旧条目后面，继承了旧的 `[spec-review-amendment]` 标记，混淆了新增与既有（strategy） | delta RUN-01 拆成独立条目，不带旧标记 |
| D18 / M21 | `Nop` 的切片归属不一致：design 切片表放 T4，tasks 3.1 放 T3（strategy） | design 切片表：包骨架与 `Nop` 归 T3 |
| D19 / M23 | `config.Keys` 单一源与跨 bash 防漂移测试并入本 change，但没写 fold 理由（strategy） | design TA#12 补一句：同文法（RUN-01）、强耦合、增量小 |
| D20 / M25 | RUN-05 场景「以 example.env 启动，日志含 `notify disabled`」没有测试真正走启动路径（plan-eng、对抗 C） | tasks 4.6 |
| D21 / M26 | Success Metric 3 写的是「集成测试」，实际落在 cmd 层测试 4.6（对抗 C） | proposal 改度量方式措辞 |
| D22 / M27 | 1.4 写「同步改 `main_test.go` 期望」，但该文件没有 allowed keys 断言；README `:14`、`:35` 与 `config.go:1`、`:35` 仍写「三个」，tasks 漏列（对抗 C、对抗 A；已 grep 核实） | tasks 1.4、1.7 |
| D23 / M28 | 只判 `Host` 非空，会放过 `http://:80/x`（对抗 C） | design TA#11 改判 `u.Hostname()` |
| D24 / M29 | design TA#12 写「读脚本文本断言」，tasks 1.6 写「执行 bash」，两者不一致；测试放哪个包也没写（对抗 C） | design TA#12 对齐到 1.6 的行为测试；tasks 1.6 写明放在 `internal/config` |
| D25 / M34 | `WithAttrs` / `WithGroup` 会 new 新 handler，新增的密钥字段必须一并传递，否则 `logger.With` 派生出的 logger 会失去脱敏（对抗 B，`redact.go:96`、`:100`） | design Security 节；tasks 2.1、2.2 |

### [已裁掉]（反静默压制：原始发现 + 裁掉理由，供设计门复核）

| ID | 原始发现（来源） | 裁掉理由 |
|---|---|---|
| X1 | `service.go:24` 行号锚偏差，应为 struct 的 25 或 New 的 32（接地镜） | design TA#2 与 memo C4 引用的正是第 24 行的注释「Service has no mutable state after construction」，锚点正确 |
| X2 | BASE-30：design 正文夹有落选方案，Security 节有「为什么……」考古句（strategy，低置信） | BASE-30 的触发条件是「改一份已存在的设计文档」，本 design 是新写的。正文里的备选与否决理由对实现者是判据，不是演进史 |
| X3 | 冷却表只增不减（backend-go F7、对抗 B） | Non-Goals 第 2 条已登记，并附可证伪假设。设备 ID 须先注册（对抗 B 自证增长有界），两镜也都建议接受 |
| X4 | 时钟回拨会让 `now-last` 为负，设备长期静音，建议改判 `d<0` 视为过期（对抗 B F6a） | 生产路径用 `time.Now()`，自带单调读数，`Sub` 不受墙钟回拨影响。问题只出现在测试注入的无单调读数时钟上，属测试自造场景 |
| X5 | `sign` / `timestamp` / 百分号编码形态的 token 不在脱敏名单；`proxyconnect` 等内层错误可能带 URL（对抗 B F4 的一部分） | 无论查询串怎么重排，请求 URL 里的 `access_token` 值都按值替换，泄露面已覆盖。`sign` 是带时效的派生值，不是长期凭据。`proxyconnect` 内层错误带的是代理地址，不是 webhook。重定向一项已由 D2 吸收 |
| X6 | 对 `errcode != 0` 的失败日志附「检查加签 / 关键词 / IP 白名单」修复提示（devex F4，低置信） | `error` 已含钉钉的 `errmsg`（钉钉给出的原因）。安全设置的说明由 D16 写进 README。在日志里再硬编码提示，是对未核实错误码（A1）的猜测 |
| X7 | testdb 与 main 的提示改由同一组 hint 常量派生（devex F6） | 这是跨包抽常量，属于加宽（通则③）。tasks 1.4 已要求两处文案逐字一致，并有测试兜住 |
| X8 | `service.New` 传 nil Notifier 会 panic（对抗 B 附注） | 只发生在请求 goroutine 里，会被 `net/http` 兜住。4 个调用点都在仓内，同一提交修改，影响低（对抗 B 自评「影响低」） |
| X9 | tasks 5.5 是人工任务，`sdflow-done` 对账时会一直是 `[ ]`（对抗 C） | 5.5 已标「由人执行」，结果记进 hand-off，四件套无需改。属于 `sdflow-done` 阶段的流程处理 |

本轮无 `[ref-check]` 机械裁掉项。首轮引用核 2 条 `quote-mismatch`（M12、X9），原因是主 session 誊写 JSON 时把行号抄错了（应为 spec.md:21、tasks.md:40），不是评审镜引错。修正行号后重跑，36/36 pass，见下方「机械引用核」。

---

## 各镜摘要

| 镜 | 档位 | 报出 | 要点 |
|---|---|---|---|
| strategy（broad） | sonnet / medium | 7 | S-01 ADR-0005 合规（D4）；S-02 标记继承（D17）；S-03 切片归属（D18）；S-04 构造与注入（D5、D6）；S-05 NTF-02 时序无测（D6）；S-06 fold 理由（D19）；S-07 → X2 |
| plan-eng（broad） | sonnet / medium | 5 | F1 withheld（Q1）；F2 logger（D5）；F3 panic（D3）；F4 重定向（D2）；F5 RUN-05 场景（D20）。核验：加签向量用 Python 重算一致 |
| 领域 backend·go | sonnet / medium | 9 | F1 panic；F2 withheld；F3 日志级别（D8）；F4 Body.Close（D9）；F5 `Wait` 可测性（D7）；F6 重定向；F7 → X3；F8 替换顺序（D10）；F9 logger。GO-01 / 03、BE-04 / 08 / 09 通过 |
| 领域 devex | sonnet / medium | 6 | F1 回退（D14）；F2 / F3 命名与缺省（D15）；F4 → X6；F5 README（D16）；F6 → X7。DX-02 三条启动文案 PASS |
| 对抗 A（隐藏假设） | sonnet / medium | 6 | B1 errcode；B2 withheld；B3 超时注入；B4 日志级别；B5 Encode 重排（D13）；B6 杂项 |
| 对抗 B（失败模式） | sonnet / medium | 7+ | F1 errcode；F2 withheld；F3 panic；F4 → D2 / X5；F5 WaitGroup；F6 → X4 / D6；F7 Body。附注 WithAttrs 传递（D25） |
| 对抗 C（乐观估计与边界） | sonnet / medium | 5+7 | 1 NTF-02 时序无测；2 子串断言（D11）；3 替换顺序；4 Metric 与边界（D21）；5 时区（D12）；小项见 D22–D24、X9 |
| 接地（grounding） | haiku / low | 1 | 代码事实与行号锚全部一致；唯一不符项为 X1（裁掉）。`service.New` 恰 4 个调用点已核实 |
| design-voice（codex） | gpt-5.6-sol | 2 | 重定向；errcode 缺失。与各镜同池，进入同一套裁决 |
| hr-tg voice（codex） | gpt-5.6-sol | 2 | 重定向（high）；errcode 缺失 |

跨模型 voice 与主审没有分歧（tension），无 TENSION 条目。

## 机械引用核

`findings_ref_check.py`：合并池 36 条（含已裁掉项的锚），`result=ok`，36 pass / 0 fail / 0 uncheckable，未分批。首轮 2 条 fail 为主 session 誊写错误，修正后重跑，见上。

## 图表核验

按 `design-diagrams.md` 只验存在与正确，不重画：

- 组件 / 依赖图（变更前后）：在，正确。
- 序列图（放行路径、并发竞态）：在，正确。
- 决策图（TG-12）：在。本轮修订后，「HTTP 2xx 且 errcode == 0」一格的语义已细化为「errcode 存在且为 0、3xx 判失败」。图里的「no」分支已包含「非 2xx / JSON 解析失败」，不算过时，未改。
- 测试覆盖图：在。新增的用例都落在已有行（`notify.DingTalk`、`notify.Throttled`、`main.run`）内，不需要新增行。

## 诚实边界

- `subagents="available"` 由宿主 `host=claude` 免探得出，不是机械门。8 个子代理均为 fresh context 真实派发并回传。
- outside-voice：host=claude、后台探针返回 `PROBE_OK`，主 session 已确证 ⇒ 走 async·harness。两站点 sidecar `.rc` 都为 `0`，是 runner 写不了的终态。findings 只取成功时的 stdout，未转录 stderr。
- 领域 / 对抗 / 接地镜的 prompt 段①：`render-review-prefix.sh --layer spec-review` 输出（22687 字节）原文置顶。段③给四件套绝对路径，并要求子代理完整 Read，没有内联全文。广审双镜的 prompt 含四条通则原文整段。
- `metrics.enabled=false`，本轮不落 lens-metric 锚，不调 emitter。
- 本报告的合并、去重与 findings JSON 誊写，是主 session 的信任边界，脚本不保证誊写正确。

## 锚

<!-- sdflow:fanout-capability v1 host="claude" subagents="available" mirrors="broad,domain,adversarial,grounding" -->
<!-- sdflow:step1-broad-review v1 mode="subagent" -->
<!-- sdflow:hr-tg v1 hit="TG-08,TG-16,TG-17,TG-26" declared="TG-01,TG-05,TG-08,TG-10,TG-12,TG-14,TG-15,TG-16,TG-17,TG-18,TG-19,TG-20,TG-22,TG-23,TG-26,TG-28" evidence="新增钉钉 webhook 出站 HTTPS（TG-08）；推送≤5s / 上报<1s（TG-16）；access_token 与加签 secret 脱敏（TG-17）；冷却表 map+mutex 与每次放行起 goroutine（TG-26）" -->
<!-- sdflow:outside-voice v1 site="design-voice" host="claude" runner="codex" reason_code="ok" findings="2" truncated="false" -->
<!-- sdflow:outside-voice v1 site="hr-tg" host="claude" runner="codex" reason_code="ok" findings="2" truncated="false" -->
<!-- sdflow:declared-sites v1 declared="design-voice,hr-tg" -->

## 收敛口

建议进设计 HARD-GATE。需要人做三件事：

1. 勾选上方「拍板三问」；
2. 对 Q1 拍板（推荐 A，已先行修订）；
3. 复核「已裁掉」区。

批准后执行拍板回写（`anchor_writeback.py --domain design --set design_approved=true`），然后 `/sdflow-ship`。

拍板前若再改四件套，须先单独 checkpoint 提交，再跑一次只审增量的窄复核，然后才能回写锚。
