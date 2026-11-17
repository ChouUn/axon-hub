# 配置 Codex 渠道使用 HTTP/SSE 并保持提示缓存前缀

> Parent: [../index.md](../index.md)
> Audience: user — 使用产物的人或 agent
> Genre: manual — 怎么完成具体任务
> Visibility: public — 他人可读，避免写入本机环境
> Maintain: living — 持续更新，保持内容有效

渠道上游 Krill Code 限制 WebSocket（WS）会话数时，让 OMP 继续通过 WS 接入 AxonHub，由 AxonHub 用 HTTP/SSE 请求渠道上游。
这条 GPT 线路还会丢失放在 `input` 第一条的系统提示，使用「先移除旧占位，再在开头插入同一占位」的请求体覆盖，以免续接回放逐轮改变缓存前缀。

## 1. 关闭渠道上游 WebSocket

1. 打开对应 Codex 渠道的「端点配置」，取消勾选「WebSocket」并保存。渠道保留 HTTP Responses 端点 `openai/responses`，OMP 继续连接 AxonHub 的 `/codex/responses`。[渠道端点配置](../zh/guides/channel-management.md#responses-http-与-websocket-端点) · [Codex 入口](../zh/guides/codex-integration.md#配置-codex-认证与路由)
2. 如果渠道上游的推理地址以 `/backend-api/codex/responses` 结尾，渠道 Base URL 填 `https://example.com/backend-api/codex#`，把 `example.com` 换成实际主机。末尾的单个 `#` 跳过自动追加的 `/v1`，AxonHub 仍会拼上 `/responses`。如 Responses 端点已单独覆盖 Base URL 或路径，以该端点的有效配置为准。[Base URL 特殊配置](../zh/guides/channel-management.md#base-url-特殊配置)
3. 查看 OMP 请求的执行记录。入站 `openai/responses-ws`、出站 `openai/responses` 时，列表的蓝色协议图标会提示「Responses WebSocket → Responses HTTP/SSE」。缓存命中看执行响应的用量。[请求列表的协议提示](../routing/feats/request-protocol-indicator.md) · [Responses 传输适配](../../llm/transformer/openai/responses/protocol_codec.go#L31-L74)

渠道「最大并发」限制执行中的请求数。上游 WS 可以在请求结束后保持空闲连接，因此这个限制无法约束同一上游账号的存活 WS 会话数。重新勾选「WebSocket」会重新引入这项会话配额约束。[渠道并发配置](../../internal/server/orchestrator/channel_limiter_manager.go#L31-L69) · [上游 WS 执行器](../../llm/transformer/openai/responses/websocket_executor.go)

## 2. 给失效的系统提示前置占位并去重

当正式 `developer` 消息位于 `input[0]` 时，线上 Codex WS 探针未能读出正式消息里指定的验证码。把同一消息移到第二条后，探针能读出验证码，保留或去掉 `model.base_instructions` 标记均可。这个对照验证了消息位置的影响，改写发生在哪一跳仍需比较实际出站请求体。Codex HTTP/SSE 的占位探针超时，该路径须按下文验收。

- OMP 18.22.1 使用 `api: openai-codex-responses` 时，会自动把正式系统提示放进 `input`。使用 `api: openai-responses` 时，若该线路需要前置占位，须设置 `compat.supportsDeveloperRole: true`，让正式提示进入 `input`。线上 `/v1/responses` 对照中，两组都前置了占位，正式提示放在 `input` 时能读出验证码，放在顶层 `instructions` 时无法读出。
- 只在承接受影响 GPT Responses 请求的专用渠道做覆盖。数组插入会在 `input` 缺失时创建该字段，共用渠道上的原生 Images 或 Alpha Search 请求也可能被改写。[数组覆盖规则](../zh/guides/request-override.md#数组操作) · [Codex 端点实现](../../internal/server/biz/channel_llm.go#L453-L469) · [Alpha Search 端点实现](../../internal/server/biz/channel_llm.go#L515-L534)

在渠道的「覆盖设置 → 覆盖请求体」中按顺序配置这两条操作。已有「追加到开头」时，把「移除匹配项」放在它前面。

| 顺序 | 操作类型 | 路径 | 匹配路径 | 匹配值 |
| --- | --- | --- | --- | --- |
| 1 | 移除匹配项（`array_remove`） | `input` | `[role,content.0.text]` | `["developer","Follow the developer instructions provided in this conversation."]` |
| 2 | 追加到开头（`array_prepend`） | `input` | | |

第 2 条的「值」直接粘贴下面的 JSON 对象。「若值是 JSON 数组则展开插入」可以保持勾选。第 1 条会同时匹配角色和占位文本，普通 `user` 消息即使包含相同文本也会保留。[覆盖字段说明](../zh/guides/request-override.md#重写参数-override-parameters) · [匹配实现](../../internal/server/orchestrator/override.go#L714-L753)

```json
{"type":"message","role":"developer","content":[{"type":"input_text","text":"Follow the developer instructions provided in this conversation."}]}
```

只配 `array_prepend` 时，AxonHub 在有可回放历史的 HTTP 续接路径上，会把覆盖后的出站 `input` 留作快照。收到带 `previous_response_id` 的下一轮增量时，AxonHub 用快照和增量拼出完整历史，再做覆盖。原来的一条占位随历史回放，又被插入一条，正式系统提示从第二条移动到第三条。`prompt_cache_key` 即使相同，输入前缀也变了。其他对 `input` 做追加或插入的覆盖也可能随回放累积。[出站执行顺序](../../internal/server/orchestrator/orchestrator.go#L343-L370) · [续接回放](../../internal/server/orchestrator/responses_attempt.go#L72-L90) · [快照记录](../../internal/server/orchestrator/responses_attempt.go#L322-L334)

本地用实际续接和请求体覆盖中间件验证了两轮。单独前置时，占位数量从 1 变成 2。先移除再前置时，两轮均只有 1 条占位，正式提示位置和 `prompt_cache_key` 一致。这组验证检查的是出站输入，渠道上游的实际缓存命中须另验。[占位移除与插入的执行顺序](../../internal/server/orchestrator/override.go#L187-L211)

## 3. 用正常 OMP 会话验收

1. 新开一个 OMP 会话，确保两轮都命中同一 AxonHub 渠道、上游模型和凭证。用正常入口连续发请求，第二轮检查入站是否带 `previous_response_id`，以确认实际触发了增量回放。
2. 在 AxonHub 请求详情的执行记录中查看实际**出站**请求体。首轮和续接的 `input[0]` 都应是同一条占位，`input[1]` 都应是原始正式系统提示，`prompt_cache_key` 应一致。若两条相同占位排在数组最前面，检查是否遗漏第 1 条或把顺序配反。[出站请求执行记录](../../internal/server/orchestrator/orchestrator.go#L367-L380)
3. 用能区分正式提示是否生效的指令做一次验证。再查看后续轮次上游响应的 `usage.input_tokens_details.cached_tokens`。如果前缀和缓存键均稳定但命中仍为 0，保留该次出站请求和上游用量用于排查。[缓存命中字段](../../llm/transformer/openai/responses/usage.go#L7-L15)
