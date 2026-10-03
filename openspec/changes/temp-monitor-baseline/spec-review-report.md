# 设计评审报告 · temp-monitor-baseline

- 评审日期：2026-10-03
- 评审对象：`proposal.md`、`design.md`、`tasks.md`、`specs/*/spec.md`（6 份），决策来源 `decision-memo.md`
- 被审盘面：提交 `318d760`
- 结论：**建议进设计门**。两条需要人拍板（Q1、Q2），合并池 40 条里 33 条归入 30 项自动决策，已修订进四件套；另有 8 项裁掉，改动处标 `[spec-review-amendment]`。

## 本轮派发

主 session 是用户直接调用本 skill 的会话，因此 outside-voice 走后台异步分支。

| 镜 | 执行方式 | 模型档 | 状态 | findings |
|---|---|---|---|---|
| 广审 strategy | fresh 子代理 | 中档 | completed | 13 |
| 广审 plan-eng | fresh 子代理 | 中档 | completed | 13 |
| 领域 backend + backend-go | fresh 子代理 | 中档 | completed | 8 |
| 领域 devex | fresh 子代理 | 中档 | completed | 8 |
| 对抗 · 隐藏假设 | fresh 子代理 | 中档 | completed | 11 |
| 对抗 · 失败模式 | fresh 子代理 | 中档 | completed | 17 |
| 对抗 · 乐观估计与边界 | fresh 子代理 | 中档 | completed | 11 |
| 接地 | fresh 子代理 | 弱档 | completed | 1（23 项核验，22 项相符） |
| outside-voice · design-voice | codex 后台，退出码 0 | — | ok | 3 |
| outside-voice · hr-tg | codex 后台，退出码 0 | — | ok | 4 |

<!-- sdflow:fanout-capability v1 host="claude" subagents="available" mirrors="broad,domain,adversarial,grounding" -->
<!-- sdflow:step1-broad-review v1 mode="subagent" -->
<!-- sdflow:hr-tg v1 hit="TG-04,TG-07,TG-08,TG-17,TG-26" declared="TG-01,TG-04,TG-05,TG-07,TG-08,TG-10,TG-13,TG-14,TG-15,TG-17,TG-18,TG-19,TG-20,TG-21,TG-22,TG-23,TG-26,TG-28" evidence="新建两张表；新增三个 HTTP 路由；依赖 PostgreSQL（PgBouncer session 池）；连接串含口令且仓库以后公开；memstore 互斥锁与并发注册、并发注销上报" -->
<!-- sdflow:declared-sites v1 declared="design-voice,hr-tg" -->
<!-- sdflow:outside-voice v1 site="design-voice" host="claude" runner="codex" reason_code="ok" findings="3" truncated="false" -->
<!-- sdflow:outside-voice v1 site="hr-tg" host="claude" runner="codex" reason_code="ok" findings="4" truncated="false" -->

派发说明：

- 容量：宿主不报告并发上限（`capacity-unknown`）。8 个子代理分 4 批派出，没有遇到容量拒绝，清单没有删减。
- 对抗镜开 3 个：命中 5 个高风险触发。
- 领域镜开 2 个：TG-01 选 `backend` + `backend-go`，TG-28 选 `devex`。
- design-voice 的 context：design「Decisions」只有一行指向 `decision-memo.md`，所以按文内指针摘录了纪要的「拍板决策」「承重约束」「接受的边角」。
- hr-tg 的 context：全新仓没有代码 diff，用 design 对应小节原文代替。
- 六面领域、对抗、接地镜的四件套内容以文件路径给出，由子代理自行读全文，没有把正文粘进 prompt。
- `metrics.enabled=false`，本轮不落 lens-metric 锚。
- 工作流附带改动：`.gitignore` 原先没有忽略 `.outside-voice/`，本轮补了一行 `**/.outside-voice/`。不补的话 checkpoint 提交会把 voice 的 context 带进仓。

## 机械引用核

合并去重后 40 条进引用核：39 条 `pass`，1 条 `uncheckable`（M20，absence 类证据包，未经机械核，直接进裁决），0 条 `fail`。没有机械裁掉项。

## 决策登记区

### 拍板三问

- **Q-scope 范围划界认不认？**
  - 自答：锚 `proposal.md`「Non-Goals」6 条（第 49–56 行）与纪要 D1。四块范围没有被砍窄或加宽。本轮修订只补防御和验证，没有新增功能。唯一带范围含义的是 Q1：🔒 任务留在本 change 还是移出。
  - [ ] 认　[ ] 不认
- **Q-deps 依赖 / 顺序认不认？**
  - 自答：锚 `tasks.md`「任务依赖图」与 6.3–6.6 的 🔒 标记。依赖图补了 4 条漏边（D16）。历史审计改为合并后、打 tag 前在 main 上再跑一次（D17）。远端库未就绪时怎么收口见 Q1。
  - [ ] 认　[ ] 不认
- **Q-risk 风险赌注与对策认不认？**
  - 自答：锚 `sdflow:hr-tg` 判定，命中 TG-04 / 07 / 08 / 17 / 26。对策条目：D1（重建连错库）、D2 与 D3（测试入口假绿）、D5 与 D6（口令泄漏）、D9（超时后写入结果未知）、D10（连接池占满 session 池）。
  - [ ] 认　[ ] 不认

<!-- sdflow:gate-questions v1 q="scope,deps,risk" -->

### 需拍板

**Q1 远端库没就绪时，本 change 怎么收口**（M18；来源：strategy S-4、对抗·边界 F9）

- 问题：6.3–6.6 依赖人配置远端库和临时输入审计名单。阶段三没有人门，tasks 没写这几条没勾完时该停还是算完成。
- 选项 A（推荐）：6.3–6.6 留在本 change。实现做到 6.2 后停下并报「等远端库」，这四条不勾。人配好库后继续跑 6.3–6.6，全勾后才执行 `/sdflow-done`。
  - 依据：纪要 D2 写明「最后的全量变绿任务与 tag 依赖它」；proposal 的 Success Metrics 包含连真库层全过。
- 选项 B：把 6.3–6.6 移进 hand-off，作为合并后的步骤。本 change 不等远端库就能合并。
- 三面后果：
  - 系统：A 下 main 上只出现验证过的基线。B 下 main 会先有一份连真库层没跑过的代码。
  - 用户：A 下分支要挂到远端库就绪。B 下合并快，但打 tag 的判据落在流程之外，靠人记得。
  - 开发循环：A 下 `/sdflow-ship` 会中途停一次，需要人再触发。B 下一次跑完，但 verify 对连真库层只能记「未验证」。
- 主次判定：系统镜为主。这个 change 的目标就是一个验证过的基线。
- 默认处理：未拍板前 tasks 不改这一点。

**Q2 `laodao-ai` 算不算禁止进仓的「公司名」**（M19；来源：strategy S-2）

- 事实：`go.mod` 的 module 路径、proposal、design 都含 `laodao-ai`。项目约定禁止公司名进仓。纪要 D8、D9 记录人指示把仓建在 `laodao-ai` 下，并采用这个 module 名。
- 推荐：按 D8、D9 认定它是 GitHub 组织名，允许出现。在 design「Compliance」写明这一条，6.6 的审计名单排除它。
- 备选：若它属于禁止的公司名，D9 要重定，module 路径、README、仓库地址都要换。
- 这一条只有人知道答案。默认处理：未拍板前不改。

### 自动决策（已修订进四件套，可覆盖）

高严重度：

- **D1 重建命令没带连接串，库名守卫可被绕过**（M1；design-voice、对抗·失败模式）。`design.md` 守卫那条 psql 带了 `"$TM_DATABASE_URL"`，重建那条没带，会连到 psql 的默认目标去删表。处置：重建命令显式带同一个连接串；DB-01 加场景「默认 psql 目标可连时重建只影响演示库」。
- **D2 `all` 模式下某一层零用例不会被发现**（M2；三个对抗镜、plan-eng）。两层合并后只判一次 N+M，集成层 build tag 写错时被单元层盖住。处置：两层各自计数，任一所选层 N+M=0 即退出 1，汇总行再合并。这是把纪要 D12 的意图落到每一层，D12 是人批准的，请留意。
- **D3 `go test` 被杀或 jq 失败时报绿**（M3；对抗·失败模式实测、plan-eng、对抗·边界）。处置：`go test` 非零退出且该层 M=0 时 M 记 1；jq 解析失败退出 1；go 的 stderr 不进 jq。
- **D5 脚本用 `source` 读 `config/local.env`**（M5、M22 一部分；七面镜与 design-voice）。实测：值含 `&` 时变量静默变空；含 `(` 时 bash 把整行连口令打到 stderr；`source` 会执行文件内容，还会覆盖已设置的环境变量。处置：脚本不再 `source`，改为逐行按字面解析，文法与 Go 一致并写死（按第一个 `=` 切分、去行尾 `\r`、空值等同未配置、值带引号报错且只报行号）。纪要里「口令含 shell 特殊字符时 `source` 出错」这条接受的边角随之失效。
- **D6 口令可能出现在输出里**（M6；plan-eng、对抗·隐藏假设、对抗·失败模式、design-voice）。实测：口令含未编码 `%` 时 psql 报错带口令原文；pgx 对连接串解析错误已脱敏，但口令含未编码 `@`、`#` 时 Ping 错误会带口令片段。处置：RUN-04 改为「不含连接串，也不含口令的任何片段」；建连阶段的错误只输出固定文案，不输出驱动或 psql 的原始文本（整段替换口令拦不住被错切的片段）；建连成功之后的错误先把连接串和口令替换为 `***` 再输出；补「口令含未编码特殊字符」的场景与验证。`psql "$URL"` 的 argv 在本机 `ps` 里可见，这一点不处理，记为接受的边角（单人开发机，不属于输出）。

中严重度：

- **D4 编译失败时 stderr 没有原因**（M4；四面镜实测）。编译错误在 `build-output` 事件里，不带 `Test` 字段。处置：`build-output` 和失败包的包级输出也打到 stderr。
- **D7 阈值接受 NaN 与 Inf**（M7；六面镜与 hr-tg）。阈值为 NaN 时告警静默失效。处置：阈值必须是有限数，RUN-03 加场景，1.4 加用例。
- **D8 1 MiB 上限和「非法 JSON」可被绕过**（M8、M44；四面镜实测与 hr-tg）。`Decoder.Decode` 只读第一个 JSON 值，后面跟 2 MiB 空白或垃圾都返回成功。处置：先 `io.ReadAll(MaxBytesReader)` 再 `json.Unmarshal`；多余字段忽略，不检查 Content-Type。
- **D9 「超时则语句不生效」不成立**（M9；backend、hr-tg、两个对抗镜）。服务端可能已提交。处置：改为「结果未知，不会半写」；写明服务不自动重试、上报非幂等、这个窗口内超温读数可能没有告警。hr-tg 建议加幂等键，不采纳，见 X5。
- **D10 连接池没设上限**（M10；backend、对抗·隐藏假设）。session 池每个客户端独占一个后端连接，上限 20；交接文档要求池上限显式设置。处置：服务池和测试池各设 `MaxConns=4`；`testdb.Open` 用 `t.Cleanup` 关池；连真库层加 `-p 1`。
- **D12 db-reset 退出码和 psql 撞车**（M12；devex、对抗·失败模式）。psql 连不上返回 2，SQL 出错返回 3，与脚本自己的 2、3 重合。处置：psql 的失败统一映射为退出 4；配置文件文法错误退出 1。`test.sh` 的撞码不改规则（D4 是人批准的），在用法和 README 写明：用法错误、缺工具时没有汇总行。
- **D13 错误信息缺原因和下一步**（M13、M14；devex）。处置：design 加「Error Messages」表，给出启动失败、三个脚本、HTTP 400 的文案，每条含问题、原因、下一步。
- **D15 条款有场景没验证**（M15；plan-eng、对抗·边界）。处置：TST-07 加第九项「并发注销与上报后无残留读数」；tasks 补 TST-06 的不可达与缺表、TST-04 的缺工具、DB-02 的清运行期数据、DB-04 的种子内容、RUN-03 的端口占用、RUN-06 的 SIGTERM、RUN-02 的监听地址检查。端口占用要先连上库才走得到，所以放在 6.5。
- **D16 任务依赖图漏边**（M16；strategy、plan-eng）。处置：补 1.4→2.6、1.4→3.3、2.5→3.3、2.6→3.4。
- **D17 历史审计时点早于最后几次提交，且有漏洞**（M17；strategy、两个对抗镜）。处置：6.6 保留；另在 hand-off 写明合并后、打 tag 前在 main 上重跑一次。审计改为先抽出 IPv4 再排除 `127.0.0.1`，加查 `%an %cn`，`git log` 加 `-m`。
- **D20 design 缺「切片建议」**（M20；strategy；未经机械核）。处置：补一节，六张垂直切片，票间依赖单向。
- **D24 契约套件和 DB-05 场景会碰种子数据**（M24；两个对抗镜）。DB-05 场景按字面会删 `demo-01`，与 TST-09 冲突。处置：DB-05 场景改用 `it-` 设备；契约套件自己生成 `it-<随机>` ID，两个实现共用。
- **D30 db-reset 等锁没有上限**（M30；hr-tg）。处置：重建会话开头 `SET lock_timeout = '5s'`。

低严重度：

- **D11**（M11；五面镜）未注册的路径和方法由标准库返回纯文本 404 / 405。处置：design 声明这是例外，「统一 JSON」只指已注册路由。
- **D21**（M21、M37；strategy、对抗·边界、devex）README：前置工具补 C 编译器；第一个命令写 `scripts/test.sh unit`；「测过 PostgreSQL 18」这句在 6.4 通过后确认。
- **D22**（M22；strategy、devex）三个脚本开头切到仓根。
- **D23**（M23；backend、plan-eng、对抗·失败模式）Shutdown 超时记 WARN、仍关池、退出 0；main 等 Shutdown 返回；`ReadHeaderTimeout` 定为 5 秒；启动 INFO 在绑定成功后打。
- **D25**（M25；对抗·失败模式实测、对抗·边界）清理用独立的 5 秒 context，不用 `t.Context()`。测试被杀后残留的 `it-` 设备靠 `db-reset.sh` 清，记为接受的边角。
- **D26**（M26；对抗·边界、对抗·失败模式）`rounds` 必须是正整数，否则退出 2；脚本设 `LC_ALL=C`；`TM_LISTEN_ADDR` 以 `:` 开头时补 `127.0.0.1`。
- **D28**（M28；对抗·边界）TST-04 的「全部跳过」限定为顶层用例全部跳过。
- **D29**（M29；对抗·边界）ALM-01 的日志断言按结构化字段匹配，不按整行字面。
- **D33**（M33；strategy）design Non-Goals 改为「不做限流和连接池调优」，已做的请求体上限、超时、优雅退出不再算例外。
- **D34**（M34；backend）API 节加三个路由的幂等性；可观测性写明不做健康检查接口和访问日志；Failure Modes 的 panic 一行注明没有 JSON 响应体。
- **D35**（M35；plan-eng、对抗·边界）「约 5 秒」改为「不超过 10 秒」。
- **D36**（M36；接地镜）纪要 C6 引用的 `pg-dev-init/SKILL.md:43` 实际在第 42 行，已改。

### 已裁掉

- **X1**（M31，strategy S-7）要求给 D4–D7 补三镜和主次判定。裁掉理由：这四条各只有一个被砍候选，纪要已写依据和砍掉理由，且都由人拍过板；补三镜不改变任何产物。
- **X2**（M32，strategy S-8）Non-Goals 各条缺理由和可证伪假设。裁掉理由：BASE-13 落点是模版槽，不属于评审残差；理由在纪要 D1、D2、D7 里。
- **X3**（M38、M37 一部分，devex F6 与 DX-01）缺省值没写理由，没有上手耗时估算。裁掉理由：缺省值出自人批准的 D7、D15；上手路径受 Q1 的远端库制约，估算没有依据。其中「README 先写零配置命令」已采纳进 D21。
- **X4**（M40，对抗·隐藏假设 F11）建表权限前提没验证。裁掉理由：proposal「Assumptions」已列这条和失效后果；失败时整体回滚，不损坏数据。
- **X5**（M9 的一部分，hr-tg）给上报加幂等键和唯一约束，或把告警与写入放进同一事务。裁掉理由：超出四块范围（D1），纪要 D7 明确「不去重」。故障窗口已在 D9 写明。
- **X6**（M43，对抗·边界 F9 的一部分）proposal 写 `.gitignore`「新增 `/.idea/`」，tasks 写「已有」。裁掉理由：`/.idea/` 是本分支上加的，两句话都成立。
- **X8**（strategy S-9 的三个子项）proposal 写了「pgx v5」；TST-05 写了「开启数据竞争检测」；5 秒存储超时没有独立的 spec 条款。裁掉理由：前两处是人已知的选型（纪要 D15），删掉不改变任何产物；5 秒超时的可观察行为已由 RUN-04「运行中存储出错」和 ALM-03 的说明覆盖。
- **X7**（backend F8 的一部分）本机浏览器可对回环地址发跨域 POST。裁掉理由：影响只限演示数据，服务无鉴权是人定的约定；README 的监听说明已覆盖。

## 各镜 findings 与去向

| 池内编号 | 问题 | 命中镜 | 严重度 | 去向 |
|---|---|---|---|---|
| M1 | 重建 psql 未带连接串 | design-voice、对抗·失败模式 | 高 | D1 |
| M2 | `all` 模式单层零用例 | 对抗×3、plan-eng | 高 | D2 |
| M3 | go test 被杀 / jq 失败报绿 | 对抗·失败模式、对抗·边界、plan-eng | 高 | D3 |
| M4 | 编译失败原因不输出 | plan-eng、对抗×3 | 中 | D4 |
| M5 | `source` 读配置 | strategy、plan-eng、backend、devex、对抗×3、design-voice | 高 | D5 |
| M6 | 口令进输出 | plan-eng、对抗·隐藏假设、对抗·失败模式、design-voice | 高 | D6 |
| M7 | 阈值 NaN / Inf | strategy、plan-eng、对抗·边界、对抗·失败模式、hr-tg | 中 | D7 |
| M8 | 1 MiB 与尾随垃圾 | plan-eng、对抗×3、hr-tg | 中 | D8 |
| M9 | 超时后语句不生效 | backend、hr-tg、对抗·隐藏假设、对抗·失败模式 | 中 | D9、X5 |
| M10 | 连接池无上限 | backend、对抗·隐藏假设 | 中 | D10 |
| M11 | 404 / 405 非 JSON | plan-eng、backend、devex、对抗·隐藏假设、对抗·失败模式 | 低 | D11 |
| M12 | 退出码撞车 | devex、对抗·失败模式、strategy | 中 | D12 |
| M13 | 错误信息缺三件套 | devex | 中 | D13 |
| M14 | HTTP 400 文案未按原因定义 | devex | 低 | D13 |
| M15 | 条款无验证 | plan-eng、对抗·边界 | 中 | D15 |
| M16 | 依赖图漏边 | strategy、plan-eng | 中 | D16 |
| M17 | 审计时点与漏洞 | strategy、对抗·隐藏假设、对抗·边界 | 中 | D17 |
| M18 | 🔒 任务收口路径 | strategy、对抗·边界 | 中 | Q1 |
| M19 | `laodao-ai` 是否公司名 | strategy | 中 | Q2 |
| M20 | 缺切片建议 | strategy | 中 | D20 |
| M21 | README 前置工具与版本声明 | strategy、对抗·边界 | 低 | D21 |
| M22 | 脚本工作目录 | strategy、devex | 低 | D22 |
| M23 | Shutdown 细节 | backend、plan-eng、对抗·失败模式 | 低 | D23 |
| M24 | 测试碰种子数据 / 固定 ID | 对抗·隐藏假设、对抗·失败模式 | 中 | D24 |
| M25 | `it-` 设备残留 | 对抗·失败模式、对抗·边界 | 低 | D25 |
| M26 | simulate 参数与环境 | 对抗·边界、对抗·失败模式 | 低 | D26 |
| M28 | 「全部跳过」与子测试 | 对抗·边界 | 低 | D28 |
| M29 | 日志断言字面 | 对抗·边界 | 低 | D29 |
| M30 | 重建等锁无上限 | hr-tg | 中 | D30 |
| M31 | D4–D7 缺三镜 | strategy | 低 | X1 |
| M32 | Non-Goals 缺理由 | strategy | 低 | X2 |
| M33 | Non-Goals 与已做加固矛盾；proposal 与 spec 混入实现细节 | strategy | 低 | D33、X8 |
| M34 | 幂等性 / 豁免声明 | backend | 低 | D34、X7 |
| M35 | 「约 5 秒」 | plan-eng、对抗·边界 | 低 | D35 |
| M36 | 纪要引用行号偏移 | 接地 | 低 | D36 |
| M37 | README 首个命令 / 上手耗时 | devex | 低 | D21、X3 |
| M38 | 缺省值理由 | devex | 低 | X3 |
| M40 | 建表权限前提 | 对抗·隐藏假设 | 低 | X4 |
| M43 | `.gitignore` 表述 | 对抗·边界 | 低 | X6 |
| M44 | 多余字段 / Content-Type 未规定 | strategy | 低 | D8 |

接地镜另核 22 项事实，全部相符：本机工具链版本、Go 标准库与 psql 选项存在、pgx v5.11.0 可拉取、`go test -json` 的三类事件形态、设备 ID 正则与配置键和退出码在各文件逐字一致、30 个 Requirement ID 与 tasks 双向对应。

对抗镜实测后排除的方向：pgx v5.11.0 对连接串形式的口令已脱敏；`t.Cleanup` 在 panic 时会执行；`DELETE /devices/BAD_ID` 能到达 handler 并由正则拒绝；并发注销与上报的两种结果在 PostgreSQL 下成立。

hr-tg 有一条建议（幂等键，见 X5）因超出人定的范围被裁掉。它不需要人在两个方案间拍板，所以不登记 TENSION；人不认可时可在设计门推翻 X5。

## 图表核验

命中的图只验证存在、正确、未过时：C4 Context、C4 Container 与组件图、上报序列图、并发序列图、tasks 的任务依赖图和测试覆盖图都在。组件图与组件清单一致。任务依赖图漏边已按 D16 补。

## 诚实边界

- `findings` 数与合并池实收数的一致性由主 session 保证，脚本不验数值。
- 机械引用核只核引文是否落在所报行，不核引文与结论的语义对应。
- 修订由一个强档子代理按本报告的自动决策执行，主 session 通读了 design 的全部新增行并做了收尾修正；`openspec validate --strict` 通过。
- 修订后的四件套没有再过一轮镜子。拍板前如果人要求实质改动，先做一次只审增量的窄复核。
