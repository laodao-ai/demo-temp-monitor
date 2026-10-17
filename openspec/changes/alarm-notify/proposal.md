## Why

超温告警现在只写一条 WARN 日志，值班的人不盯日志就不知道设备超温。基线把「告警通知渠道」列为 Non-Goal；值班团队已有钉钉值班群，现在缺的只是把超温推到群里这一步。

## What Changes

- 新增钉钉推送：读数入库成功且超过阈值时，服务在写 WARN 日志之外，异步向配置的钉钉群自定义机器人推一条文本消息，正文固定含「超温告警」与设备 ID、温度、阈值、读数时间，并 @所有人。
- 新增按设备冷却：同一设备在冷却期（默认 10 分钟）内只推一条；冷却期过后仍超温则再推一条。WARN 日志仍每条超温读数一条，ALM-01~04 不变。
- 新增加签：配置了加签密钥时，每次推送按钉钉加签规则在 URL 上附带 `timestamp` 与 `sign`；未配置则不加签。
- 新增投递判定：只有 HTTP 2xx 且响应 `errcode` 为 0 才算推送成功；其余情况写一条 ERROR 日志，不重试。推送单次最长 5 秒，不影响上报的响应与耗时。
- 新增三个配置键 `TM_DINGTALK_WEBHOOK`、`TM_DINGTALK_SECRET`、`TM_NOTIFY_COOLDOWN`。webhook 未配置时不推送，服务照常启动；配置值非法时启动失败。服务与 `scripts/` 的 `local.env` 解析同步接受这三个键。
- 扩展脱敏：webhook 地址、其中的 `access_token`、加签密钥不出现在任何日志与输出中。
- 文档与项目约定同步：README 配置表、`config/example.env`（新键留空）、`openspec/config.yaml` 本项目 context、`over-temperature-alarm` 主 spec 的 Purpose。

## Capabilities

### New Capabilities

- `alarm-notification`: 超温读数推送到钉钉群机器人——推送条件、消息内容、按设备冷却、加签、成功判定、失败处理、与上报的隔离。

### Modified Capabilities

- `service-runtime`: RUN-01 配置项由三个增至六个（已知键白名单随之扩大）；RUN-03 增加三种配置非法时的启动失败；RUN-04 增加 webhook 地址与加签密钥不出现在输出中；RUN-05 示例配置新增两个留空的推送键。

`over-temperature-alarm` 的 Requirement 不变；其 Purpose 中「不做……通知」一句与本 change 矛盾，随本 change 改写（Purpose 不在 delta 中，由任务直接改主 spec）。

## Impact

- **代码**：`internal/config`（新键、校验）、`internal/service`（超温时交给推送模块）、新增 `internal/notify`（钉钉适配器 + 冷却）、`internal/redact`（多密钥脱敏）、`cmd/temp-monitor`（装配、启动日志、错误提示）、`internal/testdb`（allowed keys 提示）、`scripts/lib/local_env.sh`（脚本侧已知键）。
- **API**：服务自身的 HTTP 接口不变；新增一个出站 HTTPS 调用（钉钉机器人 webhook）。
- **依赖**：不新增 Go 模块依赖（标准库 `net/http`、`crypto/hmac`、`crypto/sha256`、`encoding/base64`）；运行时需要能访问钉钉公网接口（或经 `HTTPS_PROXY`）。
- **文档与约定**：`README.md`、`config/example.env`、`openspec/config.yaml`、`openspec/specs/over-temperature-alarm/spec.md` 的 Purpose。
- **技术栈触发**：TG-01（后端服务变更，backend + backend-go 清单）、TG-28（新增配置键与错误文案，devex 清单）。

模块关系（箭头 = 依赖方向）：

```
 cmd/temp-monitor --+--> internal/config  (新键 + 校验)
                    |
                    +--> internal/redact  (多密钥脱敏)
                    |
                    +--> internal/notify  (新: 钉钉适配器 + 冷却) --> 钉钉 webhook (公网 HTTPS)
                    |         ^
                    +--> internal/service --+  (超温时调用 notify 接口)
                    |
                    +--> internal/httpapi (不变)

 internal/testdb ------> internal/config  (allowed keys 提示)
 scripts/lib/local_env.sh                 (脚本侧已知键, 与 config 同一文法)
```

## Success Metrics

- 超温到推送发出 — 基准：无推送 → 目标：钉钉可达时，超温读数入库后 5 秒内向 webhook 发出请求 — 度量方式：集成测试中 httptest 假钉钉服务器收到请求并断言等待上限 5 秒
- 持续超温推送条数 — 基准：若每条超温读数都推，同一设备每条读数一条 → 目标：同一设备每个冷却期最多 1 条 — 度量方式：单元测试用可注入时钟，在冷却期内连续上报 N 条超温读数，断言推送次数为 1，越过冷却期再上报断言为 2
- 推送不拖累上报 — 基准：上报返回 201 → 目标：钉钉挂起不响应时，上报仍返回 201 且响应耗时小于 1 秒 — 度量方式：集成测试中假钉钉服务器挂起请求，测量上报请求耗时并断言状态码

## Non-Goals

- 值班表、排班、升级链：值班群负责路由。假设：值班团队的钉钉群内的成员即当班责任人；若不成立（例如群里人员过多需要只提醒当班者），需另开 change 引入值班人配置并改用 `atMobiles`。
- 钉钉以外的渠道（飞书、企业微信、邮件、通用 webhook）与多渠道抽象：值班群在钉钉。假设：值班团队短期不迁移 IM；若迁移，需新增一个适配器。
- 「已恢复」通知与告警开/关状态机：需要滞回，基线已砍。假设：值班的人收到推送后会去看设备现状；若不成立，需另开 change 设计带滞回的恢复判定。
- 保证送达（重试、outbox、重启后重放）：尽力而为，持续超温在下个冷却期重推。假设：漏推只由偶发网络错误或进程重启造成，不会连续多个冷却期失败；若不成立（例如出网长期不通），需要引入重试或备用渠道。
- 全局限速与多设备聚合：假设同时超温的设备数通常不超过钉钉单机器人每分钟约 20 条的上限；若不成立，超出部分推送失败并写 ERROR 日志，需另开 change 做聚合。
- 按设备阈值、日志去重：维持基线决定，ALM-01~04 不变。

## Compliance

- `openspec/config.yaml` 本项目 context「模块只有四个……超温告警写日志……不加表外功能」：**显式豁免**。人于 2026-10-17 明确要求超温通知到值班群；本 change 同步改写该段 context，使约定与目标一致。
- 「不接 MQTT，不接真硬件」「无鉴权，服务默认监听 127.0.0.1」：遵守，均不变。
- 「连接配置从不进 git 的本地文件或环境变量读，仓里只放占位值的示例配置」：遵守。推送的 webhook 与密钥只从环境变量或 `config/local.env` 读；`config/example.env` 中两项留空。
- 「禁止凭据、真实主机地址、公司名、客户名、内网 IP 写进仓或提交历史」：遵守。示例配置不写钉钉地址；测试使用本机 httptest 服务器与虚构 token。
- 「连接信息不出现在输出中」（RUN-04）：扩展到 webhook 地址与加签密钥。

## Requirement Priorities

| 优先级 | 需求 |
|---|---|
| P0 | 超温推送到钉钉群并 @所有人；推送不影响上报；按设备冷却；加签；成功判定与失败日志；未配置不推送；新配置键（服务与脚本）；密钥脱敏 |
| P1 | README、`config/example.env`、`openspec/config.yaml` context、`over-temperature-alarm` Purpose 同步 |
| P1 | 人工真机冒烟（加签与关键词两种安全设置各推一次） |

## Stakeholders & External Dependencies

| 方 | 角色 | 是否知晓 / 需配合 | 权力级别 |
|---|---|---|---|
| 仓库所有者（本次需求方） | 定需求、做真机冒烟 | 知晓；需在测试群建机器人并执行冒烟 | 决策 |
| 值班团队 | 推送接收方 | 需求方即代表；需在值班群添加自定义机器人并选定安全设置（加签或关键词「超温告警」） | 知情 |
| 钉钉开放平台（群自定义机器人接口） | 外部依赖 | 公开接口，无需对方配合；接口语义只核到二手资料，见假设 A1 | — |

## Open Questions

无。钉钉接口语义的核实已定为人工真机冒烟任务（见 Assumptions A1）。

## Assumptions

- **A1 钉钉接口语义**：加签为「毫秒时间戳 + 换行 + 密钥」作原文、以密钥做 HMAC-SHA256、Base64 后 URL 编码，`timestamp` 与 `sign` 作为 URL 参数；响应体含 `errcode`，0 为成功。依据仅为多篇二手资料（官方文档页面抓取只得到导航）。若不成立：加签推送全部失败并写 ERROR 日志；由人工真机冒烟发现，修正签名实现。
- **A2 出网可达**：部署机能直连钉钉公网接口，或设置了 `HTTPS_PROXY`。若不成立：每次推送超时失败并写 ERROR 日志，上报不受影响。
- **A3 限流**：单个机器人每分钟约 20 条，超限后被限流一段时间。若不成立（上限更低）：多设备同时超温时更早出现推送失败；冷却参数可调大缓解。
