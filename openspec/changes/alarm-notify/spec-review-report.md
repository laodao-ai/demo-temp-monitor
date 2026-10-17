# spec-review-report · alarm-notify

- 评审对象：`openspec/changes/alarm-notify/` 四件套（proposal / design / specs ×2 / tasks），拍板决策单一源 `decision-memo.md`
- 评审时间：2026-10-17 · 分支 `feat/alarm-notify` · 评审基线提交 `fd14628`（C1）
- 宿主：`host=claude`，档位 strong=opus / mid=sonnet / light=haiku，effort strong=high / mid=medium / light=low
- 规则根：`~/.sdflow/workflow`（resolve-workflow.sh 解析，未降级）
- `metrics.enabled=false` ⇒ 本轮不落 lens-metric 锚、不调 emitter

<!-- sdflow:fanout-capability v1 host="claude" subagents="available" mirrors="broad,domain,adversarial,grounding" -->
<!-- sdflow:step1-broad-review v1 mode="subagent" -->

## 一、本轮 roster（全部为 fresh-context 子代理，实际派出并全部收回）

| 镜 | 数量 | effort | 回收 | findings |
|---|---|---|---|---|
| 广审 strategy | 1 | medium | ✅ | 4 |
| 广审 plan-eng | 1 | medium | ✅ | 7 |
| 领域 backend(+backend-go) | 1 | medium | ✅ | 6 |
| 领域 devex | 1 | medium | ✅ | 6 |
| 对抗 #1 隐藏假设 | 1 | medium | ✅ | 5 |
| 对抗 #2 失败模式 | 1 | medium | ✅ | 8 |
| 对抗 #3 乐观估计与边界 | 1 | medium | ✅ | 9 |
| 接地镜 | 1 | low | ✅ | 2（记录级，0 阻塞） |
| outside-voice design-voice | 1 | medium（同族 fallback） | ✅ | 3 |
| outside-voice hr-tg | 1 | medium（同族 fallback） | ✅ | 5 |

对抗镜按高风险开 3 面（HR-TG 命中 4 项）。原始 findings 共 55 条，合并去重后 31 条（M01–M32，M20 已并入 M19）。

**派发形态说明（如实登记）**：
- 段①稳定前缀：`render-review-prefix.sh --layer spec-review` 的 stdout 原文作为领域、对抗、接地 6 镜的 prompt 开头。
- 广审双镜：prompt 原文带入四条通则区块。
- 段③（四件套当前内容）：以绝对路径交付，由子代理自读全文，没有内联进 prompt。

## 二、触发判定

主 session 判定命中：TG-01、05、08、10、12、14、15、16、17、18、19、20、22、23、26、28（TG-24 不命中：钉钉群机器人不计费；TG-07 不命中：服务自身 HTTP 接口不变）。领域清单：backend + backend-go（TG-01）、devex（TG-28）。

hit:[TG-08,TG-16,TG-17,TG-26]｜依据模型判定:[TG-01,TG-05,TG-08,TG-10,TG-12,TG-14,TG-15,TG-16,TG-17,TG-18,TG-19,TG-20,TG-22,TG-23,TG-26,TG-28]

<!-- sdflow:hr-tg v1 hit="TG-08,TG-16,TG-17,TG-26" declared="TG-01,TG-05,TG-08,TG-10,TG-12,TG-14,TG-15,TG-16,TG-17,TG-18,TG-19,TG-20,TG-22,TG-23,TG-26,TG-28" evidence="新增钉钉公网出站调用(TG-08)、5s/1s 数字 NFR(TG-16)、access_token 与加签 secret 脱敏(TG-17)、按设备冷却表 + 每次放行一个 goroutine(TG-26)" -->

画图核验（只验不画）：变更前后组件图、正常推送序列图、并发序列图（TG-26）、是否推送决策图（TG-12）、tasks 测试覆盖图（TG-18）都在，与 amendment 后的设计一致。amendment 新增的分支（3xx、缺 errcode、panic）走的都是决策图上已有的「否 → ERROR」出口，不需要改图。

## 三、outside-voice

- 两站点 preflight 均为 `missing-deps`（跨模型 runner `codex` 不可用）⇒ 映射 `reason_code="preflight-error"`，走**同族 fallback**。
- fallback 做法：以 `outside-voice.sh render-prompt` 的输出为 prompt，派只读型 claude 子代理执行。
- ⚠️ **本轮没有跨模型第二意见**：两站点都是同族（runner==host）。
- run 目录 `20261017T121754Z-ZnGKYi`。
- 另有废弃 run 目录 `20261017T121649Z-va5CrF`：BSD awk 不支持多字节标题比较，导致 design-voice context 抽取错误。按不可变纪律保留不改，没有用它派发过任何请求。
- ⚠️ **记账误差（如实登记）**：`dispatch-manifest.tsv` 中两站点的任务标识写反了。正确对应是 design-voice ↔ `…aaede7445a3871537`，hr-tg ↔ `…af71fc8f656a05556`。按追加纪律未改写原行，以本段为准。
- 两站点结果都已在手，Step3 barrier 满足。

<!-- sdflow:outside-voice v1 site="design-voice" host="claude" runner="claude" reason_code="preflight-error" findings="3" truncated="false" -->
<!-- sdflow:outside-voice v1 site="hr-tg" host="claude" runner="claude" reason_code="preflight-error" findings="5" truncated="false" -->
<!-- sdflow:declared-sites v1 declared="design-voice,hr-tg" -->

## 四、机械引用核

`findings_ref_check.py` 对合并池 31 条：`pass` 30，`uncheckable` 1（M18，证据包形态，未经机械核，直进对抗裁决），`fail` 0。未分批。

## 五、决策登记区

### 拍板三问

```
  ┌───────────────────────────────────────────────────────────────────┐
  │ Q-scope 范围划界认不认？                                              │
  │   自答：锚 proposal Non-Goals（值班表/多渠道/恢复通知/保证送达/全局限速 │
  │   五条均附可证伪假设）+ design Non-Goals 4 条；本轮 amendment 未扩范围，│
  │   只补实现约束与测试。README 正文同步（新 1.8）是目标态必需，非加宽。    │  [x] 认  [ ] 不认
  │ Q-deps  依赖/顺序认不认？                                             │
  │   自答：锚 design「切片建议」T1/T2/T3 → T4 → T5 与 tasks「依赖顺序」；   │
  │   5.5 真机冒烟的落位未定（见 Q1），T4 是否拆分见 Q2。                    │  [x] 认  [ ] 不认
  │ Q-risk  风险赌注与对策认不认？                                         │
  │   自答：锚 sdflow:hr-tg hit=TG-08,16,17,26；最大风险是推送失败原因被     │
  │   withheld（6 镜独立命中，D-A1 已修）、缺 errcode 误判成功（D-A2 已修）、 │
  │   钉钉语义仅二手资料（A1，靠 5.5 真机冒烟兜底，落位见 Q1）。             │  [x] 认  [ ] 不认
  └───────────────────────────────────────────────────────────────────┘
```
<!-- sdflow:gate-questions v1 q="scope,deps,risk" -->

### [需拍板]

**Q1 · 5.5 人工真机冒烟在「阶段三无人类门」里怎么落位**（M09；strategy S-1 高，对抗 #3 F6 同指）
- 问题：`tasks.md:40` 的 5.5 要由人执行。阶段三由 `/sdflow-ship` 一路驱动到 merge，中间没有人类门。现在的写法会让 implementer 子代理关不了 T5，verify 要么卡在一条永远勾不上的 `[ ]`，要么被假绿勾掉；而钉钉加签和 `errcode` 语义只核到二手资料（A1）。
- **推荐 A**：5.5 移出出票范围，改为 merge 后由人执行。verify 把它记为 deferred（附 A1 风险），hand-off 列为待人执行，冒烟失败就按 A1 开 bug。
  - 系统镜：不改管线，可回退；代价是加签实现会先进 main，冒烟后才知真伪。
  - 用户镜：webhook 未配置时行为与旧版一致（NTF-06），先合并不影响任何现有用户；要到人配了 webhook 才暴露。
  - 开发循环镜：不靠人卡管线，零流程开销。
- 备选 B：放到 merge 前，作为 `sdflow-ship` 的人工暂停点。
  - 系统镜：要给阶段三加一道人类门，改动工作流约定。
  - 用户镜：main 上的加签实现必然真机验证过。
  - 开发循环镜：管线要停下等人，ship 无法一次跑到底。
- 主次判定：开发循环镜为主。功能默认关闭，A1 出错的影响只限于「配了 webhook 的人看到 ERROR 日志」，可见、可回退 ⇒ 选 A。
- 本轮**未改** 5.5 的文字，等拍板后回写。

**Q2 · 切片草图是否调整**（M10；strategy S-3 低，对抗 #3 F6 中）
- 问题：T1/T2/T3 是按层切的，每张都能独立单测，但不是垂直切片。T4 一张票装了 4.1–4.7 共 7 项，横跨 notify、service、main 三个包，还有并发测试；本轮 amendment 又给它加了构造签名、超时字段、recover。
- **推荐**：T1/T2/T3 不动（它们并行无依赖，层内可验证）。T4 拆成两张：T4a「Throttled + Nop + service 接入」（4.1–4.4）和 T4b「main 装配 + 启动日志 + 脱敏装配」（4.5–4.7）。拆完共 6 张票，仍在 3–6 的预算内。
  - 系统镜：只动草图，不动设计。
  - 用户镜：无可感知差异。
  - 开发循环镜：单票审查面减半，T4b 可以单独回滚。
- 备选：保持 5 张。代价是 T4 一张票的审查与返工面最大。
- 主次判定：开发循环镜为主。注意 `sdflow-implement` 出票时会自行切 3–6 张票，草图只是建议，所以这项影响小。

### [TENSION]

**T1 · 冷却表要不要清理**（M18；hr-tg voice #3 中 vs plan-eng「已审无发现」）
- voice 的视角：`Deregister` 后条目不删，用新 ID 循环「注册 → 超温 → 注销」可以让 map 无界增长；同名设备重新注册还会被旧冷却抑制。voice 建议在放行时顺手 O(n) 清掉过期条目。
- 主审视角：design Non-Goals 第 2 条已写明假设「条目数不超过曾经超温过的设备 ID 数，若持续高基数变化需加过期清理」；D13（同名重注册仍被冷却抑制）是人 2026-10-17 明确接受的；本项目无鉴权，可以刷 ID 的人本来就能刷库表。
- **推荐**：维持现状，不改设计。
  - 系统镜：清理只是几行代码，但会改变人已拍板的 D13 行为。
  - 用户镜：教学示例中不可感知。
  - 开发循环镜：多一组时序测试。
- 主次判定：人已明确拍板 D13，且假设可证伪 ⇒ 维持。如果人想顺手做，把它合进 4.1 即可，成本低。

### [自动决策]（高置信，已写回四件套，标 `[spec-review-amendment]`；均可在设计门推翻）

| ID | 发现 | 命中镜 | 处置 |
|---|---|---|---|
| **D-A1** M01 | **DB 连接串不含口令时，推送失败的 `error` 被整体替换为 `error details withheld`**（`redact.go:127-130` 的 `!pwOK` 分支对所有 error 值生效），NTF-05 要求的状态码 / errcode 丢失，真机冒烟时也无法分辨加签和关键词错误。主 session 已读码确认，`main_test.go:158` 证明无口令连接串是受支持的形态。 | 对抗#1、对抗#2、领域 backend、plan-eng、design-voice、hr-tg（6 镜独立命中） | design TA13：推送日志以**字符串属性** `err.Error()` 记录，不受口令 withheld 规则影响，仍经连接串与额外密钥全文替换；Failure Modes 增一行；NTF-05 增场景「连接串不含口令 + 502 ⇒ error 含 502」；tasks 4.6 增用例。备选（改 redact 的 withheld 分支）会触动 ADR-0005 语义，不取。 |
| **D-A2** M02 | 2xx 且是合法 JSON、但缺 `errcode` 的响应（`{}`），按 `int` 解码得 0，被误判成功，写出 `alarm notification sent` 却没送达，D11 自称的稳健性破了。 | 领域 backend、对抗#2、对抗#3 | `errcode` 解码为 `*int`，缺失或类型不对即失败；NTF-05 正文改为「存在且为 0」并增场景；tasks 3.4/3.5。 |
| **D-A3** M03 | 5 秒推送超时在 Throttled 里写死、没有注入点，与 Failure Modes「注入短超时」矛盾；NTF-02「5 秒内写 ERROR」没有任何测试；挂起型假钉钉可能让 `httptest.Server.Close` 卡死 Cleanup。 | plan-eng、领域 backend、对抗#1、对抗#2、对抗#3 | 包内字段 `Throttled.timeout`（缺省 5s）；4.2 增 50ms 超时用例；3.5/4.6 规定挂起 handler 的写法。 |
| **D-A4** M04 | 没给 T3/T4 的构造函数签名：消息正文要冷却值但 `Alarm` 里没有，logger 注入点也没定（用 `slog.Default()` 会绕过脱敏）；冷却值的装配没有测试能抓住。 | strategy、plan-eng、领域 backend、对抗#2 | design TA1 定 `NewDingTalk(webhook, secret, cooldown)` 与 `NewThrottled(s, cooldown, logger)`，规定必须用脱敏 logger；4.6 断言正文含 `5m0s`。 |
| **D-A5** M05 | `&http.Client{}` 默认跟随重定向：301/302 会把 POST 改成 GET，307/308 会把正文转发到别的主机，失败原因也被掩盖。 | hr-tg、plan-eng | `CheckRedirect` 返回 `ErrUseLastResponse`，3xx 判失败；NTF-05 增场景；3.4/3.5。 |
| **D-A6** M07 | 推送 goroutine 不在 `http.Server` 的 recover 范围内，一次 panic 就让进程退出，违背 NTF-02。 | 领域 backend | goroutine 入口加 `recover` 写 ERROR；NTF-02 增一句与一个场景；4.1/4.2。 |
| **D-A7** M11 | 回滚说明「清空 `TM_DINGTALK_WEBHOOK` 即停推」不成立：空环境变量等同未配置，会回落到 local.env 的值（主 session 读码确认 `config.go:52-57`）。 | 对抗#3 | design External Dependency 与 Migration Plan 改为「两处都删」；README 1.8 写明。 |
| **D-A8** M12 | README 正文还写着「只写告警日志」「三个配置项」（`README.md:3,13,14,35`），任务只点名了配置表；启用推送的上手要点（关键词必须逐字为「超温告警」、出网或代理、自检方式）只写在 design 里；配了真 webhook 后 `scripts/simulate.sh` 会给真群 @所有人。 | 领域 devex（F2/F4）、design-voice #2 | 新增 task 1.8，列出 6 项必含内容。 |
| D-A9 M06 | 没写 `resp.Body.Close()`、`LimitReader`，也没写超出 64 KiB 时怎么判。 | hr-tg、领域 backend | TA8 写明；3.4。 |
| D-A10 M08 | 推送 goroutine 并发写 `bytes.Buffer` 日志，测试同时在读，构成数据竞争（integration 层不带 `-race`，查不出来）。 | 对抗#1、对抗#2、对抗#3 | 4.2/5.1 改用加锁 buffer，先等收到请求再读日志。 |
| D-A11 M13 | errcode 失败的 ERROR 只透传 errmsg，没给修法提示（DX-02 三件套缺一）。 | 领域 devex | TA8 固定四种 error 文本，含修法提示。 |
| D-A12 M14 | `WithAttrs`/`WithGroup` 按字段字面量重建 handler，新增 secrets 字段时容易漏掉。 | 对抗#2 | Security 节写明；2.1/2.2 增用例。 |
| D-A13 M15 | 非 error 的 `KindAny` 值原样通过 redact，以后有人记 `*url.URL` 就会泄露 token。 | design-voice #3 | TA13 限定推送日志只带 `device_id` 与 `error` 两个字符串字段；Security 注明；不扩展 redact。 |
| D-A14 M16 | 加签后实际请求的 URL 与配置的 webhook 全串不同，全串替换覆盖不到它。 | plan-eng | Security 注明真正起保护作用的是 token 替换和 `.Err` 剥离。 |
| D-A15 M17 | `suppressed` 是 DEBUG，服务默认 INFO 级别不输出，Failure Modes 却写成「可见行为」。 | hr-tg | 表述改为「默认不输出（预期）」；4.2 用 Debug 级 handler。 |
| D-A16 M19 | 冷却恰好等于时长的边界未点名；「冷却可配置」场景在 main 层拿不到注入时钟。 | plan-eng、对抗#3 | 4.2 点名边界与 90s 的注入时钟用例。 |
| D-A17 M22 | pgstore 返回本地时区，漏写 `.UTC()` 只有 PG 路径会错。 | 对抗#1、对抗#3 | TA14 写明 `RecordedAt.UTC().Format(time.RFC3339)`；NTF-01 场景与 3.5 断言它。 |
| D-A18 M23 | NTF-01 场景断言 `55`、`50` 会被时间串命中，属于假绿风险。 | 对抗#2 | 改为 `温度：55°C`、`阈值 50°C` 等带上下文的片段。 |
| D-A19 M32 | NTF-02「响应体与未配置时相同」按字面不可测（id 与时间每次不同）。 | 对抗#2 | 改为「字段集合相同」。 |
| D-A20 M24 | `u.Query()` 静默丢掉无法解析的查询对，加签时 `access_token` 会消失。 | 对抗#2 | 配置校验增加 `url.ParseQuery`；RUN-03 增一项；1.2。 |
| D-A21 M25 | RUN-03 三个推送场景没写 `TM_DATABASE_URL` 已配置，会先命中 ErrNoURL；三项校验顺序未定。 | 对抗#1 | 三个场景补前提；1.2 定顺序 webhook → 冷却 → secret。 |
| D-A22 M26 | 「告警」与「推送」的术语区分只写在 decision-memo；归档后主 spec 的 ALM-04「MUST NOT 去重」与冷却看起来冲突。 | strategy | NTF-03 增一句说明冷却不属于 ALM-04 的告警去重。 |
| D-A23 M27 | `config.go:1`、`:35` 注释写「three」，改完会过时。 | 领域 devex | 1.1 顺带改。 |
| D-A24 M28 | 新键命名（DINGTALK 与 NOTIFY 两种前缀混用，冷却不带单位后缀）以及冷却/secret 校验的不对称没有说明理由。 | 领域 devex | TA13 补命名说明。 |
| D-A25 M30 | 组件清单漏了 `internal/testdb`。 | plan-eng | 补一行。 |

### [已裁掉]（反静默压制：原始发现 + 裁掉理由，供设计门复核）

- **X1 · M21 冷却下限只挡住 0、挡不住 1ns；10 分钟冷却与钉钉限流时长相同可能同步撞限流**（对抗#3 F8，低）。裁掉理由：D10 由人拍板为「> 0」，短冷却属于配置者的选择，日志里能看到 ERROR；限流同步问题属于 D7 已接受的整片过热边角。可在 README 配置表提示「建议 ≥ 1m」，不改校验。
- **X2 · M29 环境变量覆盖 local.env 时，报错不说值来自哪个来源**（devex F6，低）。裁掉理由：这是存量风格，并非本 change 引入，提出方自己也标了「可选」（通则④）。
- **X3 · M31 decision-memo 引用 `main.go:176` 而非函数头 `:167`**（接地 G1，低）。裁掉理由：第 176 行正是提示文本本身，引用成立。
- **X4 · 接地 G2**（info）：已核对 MODIFIED 需求没有悄悄改动未声明的段落。这是确认项，不是缺陷。
- 机械裁掉（`[ref-check]`）：无。

## 六、各镜 findings 摘要（按裁决归属）

- **strategy**：S-1→Q1；S-2→D-A4；S-3→Q2；S-4→D-A22。BASE-01/08/10/12/13/14/18/22/26/30 已审无发现。
- **plan-eng**：P-1→D-A3；P-2→D-A1；P-3→D-A4；P-4→D-A14；P-5→D-A5；P-6→D-A25；P-7→D-A16。BASE-16/19 无发现，BASE-05/06/17 其余部分无发现。
- **领域 backend**：F1→D-A1；F2→D-A2；F3→D-A6；F4→D-A3/D-A4；F5→D-A9；F6→D-A3。GO-01/02/03、BE-08/09 无违反。
- **领域 devex**：F1→D-A11；F2/F4→D-A8；F3→D-A23；F5→D-A24；F6→X2。DX-01/04/05 判 N/A，DX-02 启动期文案通过。
- **对抗 #1**：H1→D-A1；H2→D-A3；H3→D-A10；H4→D-A17；H5→D-A21。
- **对抗 #2**：F1→D-A2；F2→D-A3；F3→D-A4；F4→D-A1；F5→D-A12；F6→D-A10；F7→D-A18/D-A19；F8→D-A20。
- **对抗 #3**：F1→D-A2；F2/F3→D-A3；F4→D-A10；F5→D-A7；F6→Q1/Q2；F7→D-A16；F8→X1；F9→D-A17。
- **接地**：G1→X3；G2→X4。全部代码事实（签名、4 个调用点、行号、字段、常量、日志文本、测试名、主 spec 原文、ADR、加签测试向量）都已核对一致。
- **design-voice**：#1→D-A1；#2→D-A8；#3→D-A13。
- **hr-tg**：#1→D-A1；#2→D-A5；#3→T1；#4→D-A15；#5→D-A9。

## 七、amendment 清单（相对 C1 `fd14628`）

- `design.md`：TA1 构造函数；TA5 超时字段与 recover；TA8 成功判定细则与 error 文本；TA10 不跟随重定向；TA11 校验查询串；TA13 推送日志字段与命名；TA14 UTC 时间；External Dependency 与 Migration Plan 的回滚；Failure Modes 增 4 行并改 1 行；Component List 增 testdb；Security 增 3 条。
- `specs/alarm-notification/spec.md`：NTF-01 场景断言；NTF-02 panic 要求、场景与「字段集合」表述；NTF-03 术语说明；NTF-05 正文与 3 个新场景。
- `specs/service-runtime/spec.md`：RUN-03 增查询串非法一项，三个场景补 `TM_DATABASE_URL` 前提。
- `tasks.md`：1.1、1.2、新 1.8、2.1、2.2、3.4、3.5、4.1、4.2、4.6、5.1。
- `proposal.md`：无改动。
- `openspec validate alarm-notify --strict` 通过。

## 八、设计门拍板记录

- **2026-10-17 设计门已拍板批准**：人回复「都同意推荐，批准」。
  - 拍板三问：均为「认」。
  - Q1：选 A（5.5 移到 merge 后由人执行，verify 记 deferred）。
  - Q2：拆分 T4，T4a 为 4.1–4.4，T4b 为 4.5–4.7。
  - T1：维持现状，不清理冷却表。
  - 全部 [自动决策] 维持采纳，[已裁掉] 维持。
- **拍板前二次修订**：提交 `2879ff4`（checkpoint spec-review-amendment），内容是 5.5 的落位与切片调整。
- **窄复核**：1 个 fresh 子代理（medium），审增量 `fd14628..2879ff4`，即主 session 写回的 amendment 加上二次修订。
  - 结论：无阻塞项，4 条 medium/low，主 session 已全部修正（标 `[spec-review-amendment]`）。
  - N1：5.5 第 ③ 步原写「记进 hand-off」，与 merge 后执行矛盾（那时 hand-off 已归档）。改为用 `/sdflow-issues` 记录；decision-memo D8 注明 Q1 修订。
  - N2：缺 errcode 时没有对应的 error 文本。新增第五种 `dingtalk response has no integer errcode`，3.5 断言该文本。
  - N3：查询串非法这一分支的提示文案会误导，也没有测试。提示改为 `… with a valid query string`，RUN-03 场景与 1.3 增 `;` 用例。
  - N4：T4a 的覆盖列补 NTF-05；4.2 写明 panic 场景由哪两条用例证明；1.8 的 README 引文改正，并补上第 9 行。
  - 诚实边界：N1–N4 的修正**没有再做第二轮独立复核**。它们都是窄复核本身给出的一致性修补，没有引入新的设计决策（通则④，不再循环）。

## 九、收敛口

建议进入**设计 HARD-GATE**：人过本报告，勾选拍板三问，并对 Q1（推荐 A）、Q2（推荐拆 T4）、T1（推荐维持）给出拍板。
- 批准后执行 `/sdflow-ship`。
- 若 Q1/Q2 的拍板需要改四件套：先单独 checkpoint 提交，再跑窄复核（只审增量），然后回写 ship-gate 锚。
