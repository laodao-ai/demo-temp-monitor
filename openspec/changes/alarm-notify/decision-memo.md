---
schema_version: 1
change: alarm-notify
branch: repro/alarm-notify-memo-hash
generated_at: 2026-10-17T04:06:53-07:00
decision_hash: b8fa85493133
---

# 决策纪要 · alarm-notify

## 目标态

读数超温时，服务除照旧写 WARN 日志外，再把一条 @所有人 的文本消息推到配置好的钉钉群自定义机器人，通知值班的人。

> 术语旁注（项目无 `openspec/CONTEXT.md`，按规则降级记于此）：「告警」专指 ALM-01~04 的 WARN 日志，每条超温读数一条；「推送」专指本 change 新增的钉钉群消息，受冷却抑制。两者不得混用。

## 拍板决策

- **D1 系统不维护值班表，只推到一个配置好的群** — 依据：人 2026-10-17 选 A「有值班群，只缺推送」；**砍掉的候选**：系统自维护值班表/排班/升级链（等于新开一个与基线同规模的能力模块，且人已有值班群承担路由）
- **D2 渠道只做钉钉群自定义机器人** — 依据：人 2026-10-17 明确「钉钉」；群机器人只收各家规定的消息体，通用 JSON webhook 直推不了；**砍掉的候选**：通用 webhook（钉钉不认任意 JSON）；多渠道可插拔抽象（为「以后可能用得上」加宽）
- **D3 推送 @所有人（`isAtAll: true`）** — 依据：人 2026-10-17 明确「要 @所有人」；值班群常有人免打扰，不 @ 易被淹；**砍掉的候选**：不 @（易漏看）；`atMobiles` 只 @ 值班人（系统不知道谁值班，D1）
- **D4 按设备冷却，默认 10 分钟（`TM_NOTIFY_COOLDOWN`）；冷却过后仍超温则再推一次** — 依据：人 2026-10-17 同意推荐；ALM-04 每条读数一条日志，原样直推会刷屏并撞钉钉限流；只推首条则持续超温无人处理时不再提醒；**砍掉的候选**：只推首条（无后续提醒）；不抑制（刷屏 + 撞限流）
- **D5 尽力而为投递：异步、5 秒超时、失败只写 ERROR 日志、不重试；停机时在途推送丢弃；冷却表仅内存、重启清零** — 依据：人 2026-10-17 同意推荐；持续超温会在下个冷却周期再推（D4），偶发失败不致持续漏报；**砍掉的候选**：失败重试 1 次（收益小）；outbox 表 + 后台重放保证送达（复杂度翻倍，教学示例不值）
- **D6 不推「已恢复」，不做告警开/关状态机** — 依据：人 2026-10-17 同意推荐；「恢复」需滞回防抖，基线 D7 已砍滞回；**砍掉的候选**：低于阈值即推恢复（抖动）；带滞回的状态机（超范围）
- **D7 不做全局限速/聚合** — 依据：人 2026-10-17 同意推荐；多设备同时超温撞 20 条/分钟时，前 20 条已送达，值班的人已知情；**砍掉的候选**：全局令牌桶 + 「另有 N 台超温」聚合（需聚合逻辑，复杂度明显上升）
- **D8 tasks 末条为人工真机冒烟：测试群机器人先开加签推一条、再改关键词「超温告警」推一条，均须在群里看到且 @所有人，结果记进 hand-off** — 依据：人 2026-10-17 同意推荐；钉钉加签/`errcode` 语义只有二手资料（C6），假服务器测试证明不了与真钉钉一致；**砍掉的候选**：只靠 httptest 假服务器（签名写错要到线上才发现）
- **D9 `config/example.env` 中 `TM_DINGTALK_WEBHOOK=` 留空、`TM_DINGTALK_SECRET=` 留空，注释说明填钉钉机器人 webhook 地址** — 依据：人 2026-10-17 同意推荐；留空 = 复制出的 local.env 默认不推送，且不触碰 config.yaml「禁止真实主机地址」的解释歧义；**砍掉的候选**：写 `https://oapi.dingtalk.com/robot/send?access_token=CHANGE_ME` 占位（需人裁定是否违规）
- **D10 配置面：新增 `TM_DINGTALK_WEBHOOK`、`TM_DINGTALK_SECRET`、`TM_NOTIFY_COOLDOWN` 三键，文法与优先级同 RUN-01；webhook 为空 ⇒ 不推送，启动日志记 `notify disabled`；webhook 不是合法 http(s) 绝对 URL、冷却不能被 `time.ParseDuration` 解析或 ≤0、配了 secret 未配 webhook ⇒ 启动失败退出码 1，报错只给键名不回显值** — 依据：主 session 在锚点纪要「我自己定的细节」与拷问第二批附注中提出，人 2026-10-17 对两批分别回复「都同意」「都同意推荐」；冷却为 0 等于每条都推必撞限流；**砍掉的候选**：冷却 0 表示关闭抑制（必撞限流）；secret 无 webhook 时静默忽略（配置错误被吞）
- **D11 消息与成功判据：`msgtype=text`，正文固定含「超温告警」与设备 ID、温度、阈值、时间，`at.isAtAll=true`；配 secret 时按毫秒时间戳加签，`timestamp`、`sign` 拼进 URL；仅 HTTP 2xx 且响应 `errcode==0` 判成功，其余（含网络错、超时、解析失败）一律判失败写 ERROR 日志** — 依据：主 session 在锚点纪要中提出，人 2026-10-17 回复「都同意」；成功判据对「失败时 HTTP 回 200 还是非 200」两种情况都不误判；**砍掉的候选**：只看 HTTP 状态码（钉钉校验失败可能仍回 200，误判成功）
- **D12 脱敏：webhook 全串、`access_token` 值、secret 值在所有日志输出中替换为 `***`；推送失败日志不直接打 `*url.Error` 全文** — 依据：主 session 判断（基于 C3，explore 中已向人说明需脱敏）；三者为长随机串，全文替换不会误伤普通文本（不同于 DB 短口令的误伤顾虑，`internal/redact/redact.go` Handler 注释）；**砍掉的候选**：只对 error 值做替换（消息与字符串属性里拼出的 URL 会漏）
- **D13 注销后 10 分钟内同 ID 重新注册仍处旧冷却期不推，接受** — 依据：人 2026-10-17 同意推荐（Q7）；**砍掉的候选**：注销时清该设备冷却记录（让设备注册路径依赖推送模块）

## 承重约束

- **C1 推送不得改变上报响应，也不得阻塞上报** — 验证方式：读 spec 与上报路径；**证据锚**：`openspec/specs/over-temperature-alarm/spec.md:10`（ALM-01「告警 MUST NOT 改变上报接口的响应」）；`internal/service/service.go:63-77` 告警在 `Report` 同步路径内 ⇒ 推送 MUST 脱离请求 goroutine 与请求 ctx
- **C2 新配置键须同步到全部已知键白名单** — 验证方式：grep 已知键消费方；**证据锚**：`internal/config/config.go:42` knownKeys；`cmd/temp-monitor/main.go:176` 与 `internal/testdb/testdb.go:94` 的 allowed keys 提示；`scripts/lib/local_env.sh:51-53` 脚本侧白名单（RUN-01 要求服务与脚本同一文法，`openspec/specs/service-runtime/spec.md:16`「三个已知键」）⇒ 漏改任一处，写了新键的 local.env 会让 `db-reset.sh` 或服务启动报 unknown key
- **C3 现有脱敏只认数据库连接串** — 验证方式：读 redact 与 main 装配；**证据锚**：`internal/redact/redact.go:66` `Handler(inner, connURL)` 单一密钥；`cmd/temp-monitor/main.go:83`；RUN-04 只约束 `TM_DATABASE_URL` ⇒ webhook 的 `access_token` 与加签 secret 需要新的脱敏面，Go HTTP 客户端错误（`*url.Error`）文本含完整请求 URL
- **C4 冷却状态打破 Service 无可变状态的前提** — 验证方式：读 service；**证据锚**：`internal/service/service.go:24`「Service has no mutable state after construction」；`Report` 被 HTTP 并发调用 ⇒ 冷却的「判断 + 记录」MUST 原子，否则同一设备并发两条超温读数会推两次
- **C5 基线显式把通知列为 Non-Goal，本次收回** — 验证方式：读基线 proposal；**证据锚**：`openspec/changes/archive/2026-10-15-temp-monitor-baseline/proposal.md:53`；`openspec/specs/over-temperature-alarm/spec.md` Purpose「不做……通知」；人 2026-10-17 在 explore 中明确要求通知值班的人 ⇒ Purpose 与 `openspec/config.yaml` 本项目 context 须随本 change 改写
- **C6 钉钉接口语义（加签算法、`errcode`、每机器人约 20 条/分钟限流）只有二手资料** — 验证方式：WebFetch 官方 `open.dingtalk.com/document/robots/custom-robot-access` 与 `.../customize-robot-security-settings`，两页均只返回导航栏（单页应用），未取到正文；多篇二手资料互相一致（掘金 juejin.cn/post/6948442953882796063、阿里云 help.aliyun.com/zh/iot-studio/user-guide/dingtalk-chatbot、51CTO blog.51cto.com/u_16099169/6817714）；**证据锚**：上述命令输出 + 人 2026-10-17 确认以真机冒烟兜底（D8）⇒ 成功判据按 D11 写成对两种失败形态都稳健

## 接受的边角

- 推送失败 / 停机在途 / 重启清空冷却导致单次漏推或多推一条 — 概率低（网络抖动、发版）/ 影响小（持续超温下个周期再推，D4）/ 完美成本高（outbox）；**为何接受**：人 2026-10-17 同意 D5
- 多设备同时超温撞钉钉每分钟 20 条限流，后续 10 分钟推送失败 — 概率低（整片过热）/ 影响中（前 20 条已送达）/ 完美成本中高（聚合）；**为何接受**：人 2026-10-17 同意 D7
- 注销后 10 分钟内以同一 ID 重新注册，新设备首条超温仍在旧冷却期内不推 — 概率极低 / 影响小（WARN 日志照写，冷却过后再推）/ 完美成本低但需让 Deregister 触达冷却表、加耦合；**为何接受**：人 2026-10-17 同意 D13（通则④，不值得为此让设备注销路径依赖推送模块）

## 三镜代价

本次命中 TG-23（投递语义、抑制策略均有 ≥2 合理方案）。

- **D5 尽力而为 vs outbox 保证送达**
  - 系统镜：尽力而为只加一个 goroutine + 超时，无新表、可回退；outbox 需新表、后台重放循环、重放与冷却的交互，耦合与复杂度翻倍。
  - 用户镜：尽力而为偶发漏推一次，持续超温下个冷却周期补上（D4）；outbox 不漏，但停机重放可能把过时告警晚推到群里。
  - 开发循环镜：尽力而为的测试只需假 Notifier + httptest；outbox 要测重放、幂等、崩溃恢复。
  - 主次判定：开发循环与系统复杂度为主，用户镜差异被 D4 的周期重推吸收 ⇒ 选尽力而为。
- **D4 按设备冷却 + 周期重推 vs 只推首条 vs 不抑制**
  - 系统镜：冷却表一处可变状态（C4），其余两者无状态；不抑制会撞钉钉限流，连带后续告警全丢。
  - 用户镜：周期重推在持续超温时每 10 分钟提醒一次；只推首条可能无人跟进；不抑制刷屏。
  - 开发循环镜：冷却需可注入时钟以便测试，成本小。
  - 主次判定：用户镜为主（通知到人且不被刷屏）⇒ 选按设备冷却 + 周期重推。
- **D7 不做全局限速 vs 令牌桶 + 聚合**
  - 系统镜：不做则无新组件；聚合需窗口、计数与合并消息格式。
  - 用户镜：整片过热时前 20 条已送达，后续失败；聚合可一条概括全部。
  - 开发循环镜：聚合多一套时序测试。
  - 主次判定：概率低且值班的人已知情 ⇒ 系统与开发循环为主，选不做。

TG-24（外部计费服务）不命中：钉钉群自定义机器人不计费。
