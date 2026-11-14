# 渠道强制 1 小时提示缓存

> Parent: [../index.md](../index.md)
> Audience: developer — 开发和演进项目的人或 agent
> Genre: feat — 这个 feature 是什么、为什么值得、什么形态
> Visibility: public — 他人可读，避免写入本机环境
> Maintain: living — 持续更新，保持内容有效

- 状态：已实现。提交：`bb25b45a`（`feat(channels): 渠道强制 1 小时提示缓存`）、`6c9c764d`
  （`fix(requests): 1 小时缓存标志改为以计费为准`）。
- 依赖：计费依赖「模型权威定价与全量阶梯」中的 Anthropic 官方目录一小时写缓存价，
  或手动配置的 `one_hour` 变体；未配置时，一小时写入按写缓存基础价计费。
- 动机：人与 agent 协作时，多个会话等待用户回复、或用户离开片刻，请求间隔常超过 5 分钟，5 分钟缓存过期后需按
  1.25× 重写全量前缀；1 小时缓存可命中并按 0.1× 计费。按当前用法，与用户对接的是 Claude 系模型，连续运行的
  子代理是 GPT，前者更容易遇到。AxonHub 目前原样转发客户端 TTL，自己补的断点不带 TTL（即 5 分钟，
  `llm/transformer/anthropic/ensure_cache_control.go:130-133`），没有任何强制 TTL 的配置。
- 参考：CCH 在 Key 与供应商上配置 `cacheTtlPreference`（`inherit` / `5m` / `1h`，Key 优先），只改写客户端已有的
  `system[]` 与 `messages[].content[]` 顶层 `ephemeral` 断点，不改 `tools`、顶层 `cache_control` 与 `tool_result`
  内嵌断点（`src/app/v1/_lib/proxy/forwarder.ts:774-850,3527-3538`）；1h 时补 `extended-cache-ttl-2025-04-11`
  beta 头（`forwarder.ts:852-863,8828-8833`）。响应只有写入总数、没有 5m / 1h 分档时，按请求偏好把总数计为 1h
  （`src/app/v1/_lib/proxy/response-handler.ts:6468-6485`），另有供应商级计费互换开关 `swapCacheTtlBilling`
  （`response-handler.ts:6444-6451`）。本 topic 不照搬后两者。
- 已确认决策：
  - D1 配置位置：只在渠道上配置，按上游是否支持 1 小时缓存开启；不在 API Key 上配置。
  - D2 强制方式：开关式，开启后全部断点改为 1h，只延长不降级；不提供「强制 5m」。
  - D3 AxonHub 自己补的断点在开启时同样为 1h。
  - D4 不补 `anthropic-beta` 头（官方文档已不要求）；中转站需要时用渠道已有的请求头覆盖自行添加。
  - D5 计费只按上游响应中的真实字段决定，不按请求的 TTL 推断，也不比较请求与响应的 TTL、不告警。
  - D6 不做计费互换。
  - D7 请求日志标记：在请求列表模型列「出站协议」「透传」图标之后加「1 小时缓存」图标，始终显示、否时灰色；请求
    详情的「用量」卡片标题旁加「1 小时缓存」徽章，仅为是时显示。标记与计费同源，与渠道开关无关。
- 规则：
  - R1 生效条件：渠道开启本开关，且请求以 Anthropic Messages 格式发往该渠道，包括由其他入站格式转换而来的请求
    与原样透传的请求体；其他出站格式不改。未开启的渠道行为不变。
  - R2 改写范围：请求中所有 `type` 为 `ephemeral` 的 `cache_control` 均设为 `ttl: "1h"`，覆盖 `tools`、`system`、
    `messages` 内容块（含 `tool_result` 内嵌内容）与顶层 `cache_control`（自动缓存）。只改已有断点的 TTL，
    不为此新增断点；客户端未使用提示缓存的请求不变。全部断点统一为 1h，满足「长 TTL 必须排在短 TTL 之前」。
  - R3 计费：写缓存 Token 按响应中的 5m / 1h 分档分别计价；响应只有写入总数、没有分档时，沿用现有逻辑按写缓存
    基础价计价（`internal/server/biz/cost_calc.go:211-241`），不因渠道开启本开关而改按 1h 计。
  - R4 标记判定：该请求任一条用量记录的 1 小时写缓存 Token（`prompt_write_cached_tokens_1h`）大于 0 为是，否则为否，
    与计费使用同一个字段，不另存标记。一个请求可能有多条用量记录（非流式响应先写用量、再做空响应检测与入站转换，
    失败后重试，`llm/pipeline/non_streaming.go:47-59`），用量记录不带执行 ID，无法归属到具体执行卡片，因此详情按
    请求显示一次。历史请求按已有用量记录直接显示，无需回填。只读 1 小时缓存、没有新写入的请求为否（上游读缓存不分 TTL）。
- 代码：`internal/objects/channel.go`（`ChannelSettings.ForceOneHourPromptCache`，JSON `forceOneHourPromptCache`）、
  `internal/server/gql/axonhub.graphql`（`ChannelSettings` 与 `ChannelSettingsInput` 追加字段，已重新生成）、
  新增 `internal/server/orchestrator/prompt_cache_ttl.go`（出站原始请求中间件：按 gjson 定位 R2 所列断点，只用 sjson
  改写 `ttl`，其余字节不变；先复制请求体，不改动重试共享的底层数组）、
  `internal/server/orchestrator/orchestrator.go`（中间件插在请求体透传之后、渠道请求体覆盖之前，覆盖操作仍最终生效）；前端
  `frontend/src/features/channels/{data/schema.ts,data/channels.ts,utils/merge.ts,components/channels-action-dialog.tsx}`
  （渠道编辑对话框「透传」下方的复选框）、`frontend/src/locales/{en,zh-CN}/channels.json`、
  `frontend/src/features/requests/{data/requests.ts,data/schema.ts,components/requests-columns.tsx,components/request-detail-content.tsx}`
  （列表与详情各加一个按 `promptWriteCachedTokens1hGT: 0` 过滤的用量计数查询；模型列第三个图标与用量卡片徽章）、
  `frontend/src/locales/{en,zh-CN}/requests.json`；
  `docs/{zh,en}/guides/channel-management.md`、`docs/{zh,en}/guides/cost-tracking.md`。
- 迁移：无 data migration 与缓存键变化；渠道设置随 settings JSON 保存。`v1.0.0-beta10-fork.3` 曾发布
  `request_executions.one_hour_prompt_cache` 列，现已从 schema 删除，Ent 自动迁移（`WithDropColumn(true)`）会删列。
- 测试：`internal/server/orchestrator/prompt_cache_ttl_test.go`（各位置断点改为 1h 且其余字节不变、非 ephemeral 与
  非断点位置不动、关闭 / 非 Anthropic 格式 / 无断点 / 字符串 system / 非法 JSON 原样返回、不改动重试共享的请求体）。
- 同步上游注意：上游若调整出站中间件顺序或 Anthropic 缓存断点处理（`ensure_cache_control.go`），需确认本中间件仍在
  请求体透传之后、覆盖之前执行。上游若改用量记录的 1 小时写缓存字段，需同步请求日志标记的判定。
