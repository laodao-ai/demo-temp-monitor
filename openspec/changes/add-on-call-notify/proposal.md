## Why

超温现在只写一条 WARN 日志，没人盯日志就没人知道。值班的人需要被主动推送：设备进入超温时收到一次，恢复时再收到一次。读数在阈值附近抖动时，不能被刷屏。

## What Changes

- 新增可选配置 `TM_NOTIFY_WEBHOOK_URL`。配置后，服务把超温事件以 JSON POST 到这一个 webhook，接收方就是值班的人或群。不配置时，通知链路整条不启用，行为与现在完全相同。
- 新增配置 `TM_NOTIFY_HYSTERESIS_C`（回差，缺省 2.0，非负有限数）。
- 按事件通知，而不是按读数通知：每台设备有「正常 / 超温中」两种状态。
  - 读数入库成功且 `temp > T` 时，从正常进入超温中，发一条 `alarm`。
  - 读数入库成功且 `temp <= T - H` 时，从超温中回到正常，发一条 `resolved`。
  - 读数落在 `(T-H, T]` 区间时，状态保持不变，不发通知。
- 投递是尽力而为：内存里一个有界队列，由单个 worker 按顺序串行发送。每次只发一回，超时 3 秒，不重试。失败、队满、停服时剩下没发的通知，都只记 WARN 日志。上报接口的响应不受影响。
- 设备状态只放内存。设备注册或注销成功时，清掉它的状态。
- webhook URL 视同凭据，不能出现在日志和任何输出中。
- 停服时，在 RUN-06 的 5 秒预算内尽量把队列发完，超时就丢掉。
- 修改 `config/example.env`：加入两个新键。webhook 那一行注释掉，主机写 `127.0.0.1` 占位。
- 修改 `scripts/lib/local_env.sh`：已知键加入这两个新键，保持与服务同一套文法。

### 显式推翻基线的 Non-Goals

基线 `temp-monitor-baseline` 的 Non-Goals 里有「告警通知渠道（只写日志）」和「告警滞回」两条。本 change 在**通知这一条路径上**推翻这两条：

- 日志告警 ALM-01..ALM-04 一字不改，仍然是每条超温读数各写一条 WARN，不去重、不滞回。
- 滞回只用于判断通知状态，不影响日志。

项目上下文里「模块只有四个」那句话随本 change 一起更新为五个模块（见 tasks）。

## Capabilities

### New Capabilities

- `on-call-notify`: 超温事件的状态机（进入、恢复、滞回），通知的触发条件、payload、投递语义（单 webhook、串行、不重试、队满与停服丢弃），以及已知限制。

### Modified Capabilities

- `service-runtime`:
  - RUN-01：配置项从三个增加到五个，`config/local.env` 的已知键同步更新。
  - RUN-03：回差非法或 webhook URL 格式非法时，启动失败。
  - RUN-04：webhook URL 与数据库连接串同等对待，不出现在输出中。
  - RUN-05：示例配置加入新键的占位值。
  - RUN-06：停服时在同一个 5 秒预算内处理通知队列。

`over-temperature-alarm` 的需求不变。它的 Purpose 里写着「不做……通知」，这句话在归档时改成指向 `on-call-notify`。

## Impact

- 代码：
  - 新增 `internal/notify`：事件类型、Notifier 接口、webhook 实现。
  - `internal/service`：状态机，以及调用 Notifier。`service.New` 改为接收可变参数选项，现有调用点不用改。
  - `internal/config`：新增两个键和对应的哨兵错误。
  - `cmd/temp-monitor`：组装 notifier、启动日志、停服顺序。
  - `scripts/lib/local_env.sh`：已知键列表。
- 依赖：只用标准库 `net/http`，不新增依赖。
- 外部系统：一个由使用者自行提供的 webhook 接收端。仓里不写任何真实地址。
- 测试：
  - 单元层：用假 Notifier 测状态机；用 `httptest` 测 webhook 投递；补配置和启动的测试。
  - 连真库层：端到端测试加一条「进入 → 恢复」路径。
- 文档：README、`openspec/INDEX.md`、项目上下文的模块数。

## Non-Goals

- 排班、多接收人、按设备路由。接收方只有一个 webhook。
- 重试、持久化队列（outbox）、跨重启保留状态。
- 冷却时间、按设备设阈值或回差。
- 通知渠道的适配：任何 IM 的专有消息格式、签名、鉴权头。payload 是固定的 JSON，接收端要什么格式自己转。
- 改动 `scripts/simulate.sh`。`sim-03` 每轮都超温，演示时只会触发一次 `alarm`，这已经够用。
- 演示用的 webhook 接收端脚本。
