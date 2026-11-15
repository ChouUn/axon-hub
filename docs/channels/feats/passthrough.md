# 透传

> Parent: [../index.md](../index.md)
> Audience: developer — 开发和演进项目的人或 agent
> Genre: feat — 这个 feature 是什么、为什么值得、什么形态
> Visibility: public — 他人可读，避免写入本机环境
> Maintain: living — 持续更新，保持内容有效

透传是上游已有的开关（`ChannelSettings.passThroughBody`，渠道对话框的「透传」，未设置时沿用系统级设置）：
满足条件时，请求复用客户端的原始请求体，响应原样转回，不经转换管线重新序列化。

- 条件由 `internal/server/orchestrator/pass_through.go` 判定，主要有：
  - 入站与出站的 API 格式相同。
  - 请求的流式模式与实际出站的流式模式一致。
  - 请求体可复用：transcription、translation、image variation 这类 multipart 请求不透传。
- 透传时仍把请求体里的 `model` 改写为映射后的模型。

本 fork 修正了其中三处：流式预读队列的循环等待，渠道上游的错误体被重包装，以及流式透传关闭时读取流状态的数据竞争。

## 透传预读队列循环等待

### 动机

- 启用流式透传与重试时，首个有效内容前的空 thinking 或被转换器跳过的事件可填满 64 槽原始队列。
- 消费者等待预读结束，生产者等待消费者，请求卡到取消或总超时。
- 来源：上游 [#2150](https://github.com/looplj/axonhub/issues/2150)、
  [#2580](https://github.com/looplj/axonhub/pull/2580)。

### 形态

- 每次尝试在消费者接入前暂存原始事件，上限为 4096 个事件、64 MiB 的 data/type/id 字节。
- 单个事件上限为 32 MiB。
- 超限返回不可重试的 `ErrPreCommitBufferExceeded`。超限后的暂存拒绝接入，不暴露失败尝试的内容。
- 消费者接入时有序交付暂存的事件。
- 接入后队列上限为 4096 个事件、128 MiB，队列满时读取渠道上游的一侧等待消费者。
- 重试重置时释放旧暂存。
- 与上游实现的关系：
  - 上游 `02269f12`（#2580，随上游 `v1.0.0-beta11` 发布）修了同一问题，暂存上限为 1024 个事件、8 MiB。
    8 MiB 小于合法的单个 32 MiB SSE 事件，当时保留 fork 自己的实现。
  - 上游 `94cd05b1`（#2656，随上游 `v1.0.0-beta12` 发布）重写了暂存与接入后的队列，字节上限升到上面的取值。
    本 fork 自此以上游实现为底。
  - 与上游的差异有两处：
    - 事件数上限为 4096，上游为 1024。
    - 超限后拒绝接入。上游的接入不检查暂存是否已超限。

### 边界

- 未改变首个有效内容前不向客户端输出的语义。
- 长思考仍可能触发客户端或代理的等待上限。

### 实现与维护

- 提交：`cbea376e`。
- 代码：`internal/server/orchestrator/` 下的 `pass_through.go`、`state.go`、`outbound.go`。
- 迁移：无 schema migration、data migration 或持久缓存结构变化。
- 测试：
  - 真实 HTTP 的 Anthropic 渠道上游立即发送 200 个空 thinking delta，透传和重试开启时完整接收 206 个事件及
    `message_stop`。
  - 行为回归覆盖跳过事件、事件顺序及事件数、字节上限。
  - `pass_through_preread_test.go` 是 fork 的测试。
  - 上游的 `pass_through_budget_test.go` 按本 fork 的事件数上限改写后保留。
  - 无新增 fixture。
- 同步上游：上游改动预读暂存时跟随上游实现，保留上面两处差异，`pass_through_preread_test.go` 的断言保持通过。

## 透传时渠道上游的错误体被重包装

### 动机

- Anthropic 出站 `TransformError` 把渠道上游的任何错误写死成 `api_error`，只保留 message，
  丢掉错误体里的其他字段。
- 触发场景：
  - Claude Code 2.1.266 起给对话中段 `role: system` 消息挂 `output_config`
    （按轮次 effort，beta `per-turn-control-2026-07-01`）。
  - sssaiapi 这类中转不认该字段时返回 400，`raw_body` 内附 Anthropic 原文
    `messages.N.output_config: Extra inputs are not permitted`。
  - CLI 在错误 message 里看到这段原文就自动去掉该字段重试。
    经 AxonHub 后原文被吞，CLI 不重试，用户直接看到 400。
  - 透传关闭时 AxonHub 重组请求会丢掉该字段，不会触发。
- 上游 `v1.0.0-beta10` 未处理。fork 在 2026-09-10 提交修正（`07099dd4`）时，上游 `unstable` 分支也未处理。
  这项修正可回馈上游。

### 形态

- `llm.ResponseError` 新增 `RawUpstream`。
- 出站包装器在请求体已透传（`PassThroughApplied`）时挂上渠道上游的原始 HTTP 错误。
- Anthropic 入站遇到带 `RawUpstream` 且为合法 JSON 的错误时，按渠道上游的状态码原样写回。

### 边界

- 系统设置「上游错误策略」为 custom 时仍以 custom 为准。
- 透传关闭、OpenAI 协议入站、非 JSON 错误体保持原行为。

### 实现与维护

- 提交：`07099dd4`（`fix(anthropic): 透传时把上游错误体原样回传客户端`）。
- 代码：
  - `llm/model.go`
  - `llm/transformer/anthropic/inbound.go`
  - `internal/server/orchestrator/outbound.go`
- 迁移：无。
- 测试：
  - `llm/transformer/anthropic/inbound_test.go`
  - `internal/server/orchestrator/outbound_test.go`
  - `internal/server/api/chat_test.go`
  - 本机 Docker 真机链路（CLI 2.1.266 真实配置 → AxonHub → sssaiapi）：
    - 透传开：首个 400 原样回传，CLI 自动重试后 200。
    - custom 策略：回自定义文案。
    - 透传关：首个请求即 200。
- 同步上游：
  - `llm/` 是独立模块，上游改动 `ResponseError` 或 Anthropic 入站 `TransformError` 时需重接。
  - 上游自行实现错误透传后，以上游为准并移除本字段。

## 流式透传关闭时读取流状态的数据竞争

### 动机

- 流式透传时，转换管线一侧的流由后台协程读完，用于用量结算与请求记录。
- 上游 `v1.0.0-beta12` 关闭透传流时，先关闭原入站流，再等后台协程退出：
  - 客户端在结束事件交付前断开时走这条顺序。
  - 结束事件已交付、等待后台协程超时时同样如此。
- 原入站流的关闭随即读取已收到的事件与终态，后台协程可能仍在写入，构成数据竞争。
  受影响的是请求记录的状态与响应内容。

### 形态

- 原入站流关闭时，先关闭转换流与流生命周期，等后台协程退出，再读取事件与终态。
- 没有流生命周期时，后台协程自己关闭流，不等待。

### 边界

- 关闭等待后台协程退出，没有超时：转换流与流生命周期关闭后，后台协程读到流结束即退出。
- 渠道上游尾部的收集预算（1 秒、事件数、字节数）不变。

### 实现与维护

- 提交：`aae3f464`（`fix(orchestrator): 透传流关闭时等待后台转换协程退出再读取状态`）。
- 代码，上游文件：
  - `internal/server/orchestrator/state.go`：包内字段 `rawStreamDrainDone`，保存后台协程的退出信号。
  - `internal/server/orchestrator/pass_through.go`：赋值。
  - `internal/server/orchestrator/outbound.go`：重试重置时清空。
  - `internal/server/orchestrator/inbound.go`：`InboundPersistentStream.Close` 的等待。
- 迁移：无。
- 测试：`internal/server/orchestrator/pass_through_close_lifecycle_test.go`。
  - 真实 Anthropic 转换与透传，覆盖未交付和交付后等待超时两条关闭分支。
  - 修复前 `-race` 报竞争。
  - 无新增 fixture。
- 同步上游：
  - 上游改动 `InboundPersistentStream.Close` 或 `passThroughDeliveryStream.Close` 的顺序时，确认关闭仍在后台协程退出后读取状态。
  - [健康门控](../../routing/feats/health-gate.md)也读取 `rawStreamDrainDone`。
  - 上游自行修复后，以上游为准并移除本字段。
