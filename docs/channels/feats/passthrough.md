# 透传

> Parent: [../index.md](../index.md)
> Audience: developer — 开发和演进项目的人或 agent
> Genre: feat — 这个 feature 是什么、为什么值得、什么形态
> Visibility: public — 他人可读，避免写入本机环境
> Maintain: living — 持续更新，保持内容有效

## 透传预读队列循环等待

- 问题：启用流式透传与重试时，首个有效内容前的空 thinking 或被转换器跳过的事件可填满
  64 槽原始队列；消费者等待预读结束，生产者等待消费者，导致请求卡到取消或总超时。
- 来源：上游 [#2150](https://github.com/looplj/axonhub/issues/2150)、
  [#2580](https://github.com/looplj/axonhub/pull/2580)。参考 #2580 的分阶段暂存方案，
  fork 增加原始事件预算，不直接采用无界 backlog；保留重试前事件隔离及接入后的背压。
- 提交：`cbea376e`。
- 行为：每次尝试在消费者接入前暂存最多 4096 个原始事件、64 MiB 的 data/type/id 字节，
  超限返回不可重试的 `ErrPreCommitBufferExceeded`，不暴露失败尝试的内容；接入时有序交付，
  重试重置释放旧暂存。合法的单个 32 MiB SSE 事件仍可通过原始字节预算。
- 边界：未改变首个有效内容前不向客户端输出的语义；长思考仍可能触发客户端或代理的等待上限。
- 代码：`internal/server/orchestrator/pass_through.go`、`state.go`、`outbound.go`。
- 验证：真实 HTTP Anthropic 上游立即发送 200 个空 thinking delta，透传和重试开启时完整
  接收 206 个事件及 `message_stop`；行为回归覆盖跳过事件、事件顺序及事件数/字节上限。
- 迁移：无 schema、data migration 或持久缓存结构变化；无新增 fixture。
- 同步上游：核对 #2580 合并后的实现，不能丢失原始事件资源预算和取消时错误读取的同步保障。

## 透传时上游错误体被重包装

- 问题：Anthropic 出站 `TransformError` 把上游任何错误写死成 `api_error`，
  只保留 message，丢掉上游错误体里的其他字段。Claude Code 2.1.266 起会给对话中段
  `role: system` 消息挂
  `output_config`（按轮次 effort，beta `per-turn-control-2026-07-01`），
  sssaiapi 这类中转不认时返回 400，`raw_body` 内附 Anthropic 原文
  `messages.N.output_config: Extra inputs are not permitted`。CLI 只要在错误 message 里
  看到这段原文就会自动去掉该字段重试；经 AxonHub 后原文被吞，CLI 不重试，用户直接看到
  400。透传关闭时 AxonHub 重组请求会丢掉该字段，反而不会触发。
- 上游来源：上游 beta10 与 unstable 均未处理，fork 先行修复，可回馈。
- 行为：`llm.ResponseError` 新增 `RawUpstream`；出站包装器在请求体已透传
  （`PassThroughApplied`）时挂上上游原始 HTTP 错误；Anthropic 入站遇到带 `RawUpstream`
  且为合法 JSON 的错误时按上游状态码原样写回。系统设置「上游错误策略」为 custom 时
  仍以 custom 为准。透传关闭、OpenAI 协议入站、非 JSON 错误体保持原行为。
- 提交：`07099dd4`（`fix(anthropic): 透传时把上游错误体原样回传客户端`）。
- 代码：`llm/model.go`、`llm/transformer/anthropic/inbound.go`、
  `internal/server/orchestrator/outbound.go`。
- 迁移与测试：无迁移；`llm/transformer/anthropic/inbound_test.go`、
  `internal/server/orchestrator/outbound_test.go`、`internal/server/api/chat_test.go`。
  本机 Docker 真机链路（CLI 2.1.266 真实配置 → AxonHub → sssaiapi）验证：透传开时首个
  400 原样回传、CLI 自动重试 200；custom 策略回自定义文案；透传关时首个请求即 200。
- 同步上游注意：`llm/` 是独立模块，上游若改动 `ResponseError` 或 Anthropic 入站
  `TransformError` 需重接；若上游自行实现错误透传，以上游为准并移除本字段。
