# 渠道强制 1 小时提示缓存

> Parent: [../index.md](../index.md)
> Audience: developer — 开发和演进项目的人或 agent
> Genre: feat — 这个 feature 是什么、为什么值得、什么形态
> Visibility: public — 他人可读，避免写入本机环境
> Maintain: living — 持续更新，保持内容有效

渠道上的一个开关：开启后，以 Anthropic Messages 格式发往该渠道的请求里，已有的提示缓存断点全部改为
1 小时 TTL。渠道上游支持 1 小时缓存时，用它减少缓存过期后的全量重写。

## 动机

- 人与 agent 协作时，多个会话等待用户回复、或用户离开片刻，请求间隔常超过 5 分钟。
- 5 分钟缓存过期后需按 1.25× 重写全量前缀。1 小时缓存可命中并按 0.1× 计费。
- 在 Claude 系模型负责与用户交互、GPT 子代理连续执行的工作流里，前者更容易遇到这种间隔。
- 上游 AxonHub 没有强制 TTL 的配置：
  - 客户端的 TTL 原样转发。
  - 自己补的断点不带 TTL，即 5 分钟（`llm/transformer/anthropic/ensure_cache_control.go:300-305`）。

## 形态

渠道编辑对话框「透传」下方有一个复选框。请求日志用图标和徽章标出实际按 1 小时缓存计费的请求。

D 编号是确定形态的决策，R 编号是行为规则。

- D1 配置位置：只在渠道上配置，按渠道上游是否支持 1 小时缓存开启。不在 API Key 上配置。
- D2 强制方式：开关式，开启后全部断点改为 1h，只延长不降级。不提供「强制 5m」。
- D3 AxonHub 自己补的断点在开启时同样为 1h。
- D4 不补 `anthropic-beta` 头（官方文档已不要求）。中转站需要时，用渠道已有的请求头覆盖自行添加。
- D5 计费只按渠道上游响应中的真实字段决定。不按请求的 TTL 推断，不比较请求与响应的 TTL，不告警。
- D6 不做计费互换。
- D7 请求日志标记：
  - 请求列表模型列的「出站协议」「透传」图标之后加「1 小时缓存」图标，始终显示，否时灰色。
  - 请求详情的「用量」卡片标题旁加「1 小时缓存」徽章，仅为是时显示。
  - 标记与计费同源，与渠道开关无关。

## 行为规则

- R1 生效条件：
  - 渠道开启本开关，且请求以 Anthropic Messages 格式发往该渠道。
  - 由其他入站格式转换而来的请求与原样透传的请求体都算。
  - 其他出站格式不改。
- R2 改写范围：
  - 请求中所有 `type` 为 `ephemeral` 的 `cache_control` 均设为 `ttl: "1h"`。
  - 覆盖 `tools`、`system`、`messages` 内容块（含 `tool_result` 内嵌内容）与顶层 `cache_control`（自动缓存）。
  - 只改已有断点的 TTL，不为此新增断点。请求里没有断点时不变。
  - 上游自 `697b13a5`（PR #2697，随上游 `v1.0.0-beta12` 发布）起，为由其他入站格式转换而来、
    不带缓存标记的请求加顶层 `cache_control`（自动缓存）。它按 D3 同样改为 1h。
  - 全部断点统一为 1h，满足「长 TTL 必须排在短 TTL 之前」。
- R3 计费：
  - 写缓存 Token 按响应中的 5m、1h 分档分别计价。
  - 响应只有写入总数、没有分档时，按写缓存基础价计价（`internal/server/biz/cost_calc.go:211-241`），
    不因渠道开启本开关而改按 1h 计。
- R4 标记判定：
  - 该请求任一条用量记录的 1 小时写缓存 Token（`prompt_write_cached_tokens_1h`）大于 0 为是，否则为否。
  - 标记与计费使用同一个字段，不另存标记。
  - 详情按请求显示一次，不按执行卡片显示：
    - 一个请求可能有多条用量记录。非流式响应先写用量，再做空响应检测与入站转换，失败后重试
      （`llm/pipeline/non_streaming.go:48-70`）。
    - 用量记录不带执行 ID，无法归属到具体执行卡片。
  - 历史请求按已有用量记录直接显示，无需回填。
  - 只读 1 小时缓存、没有新写入的请求为否：渠道上游返回的读缓存用量不分 TTL。

## 边界

- 与[模型权威定价与全量阶梯](../../billing/feats/model-pricing-tiers.md)的分工：本功能只改请求里的 TTL，
  1 小时写入的单价由定价决定。
  - 单价取 Anthropic 官方目录导入的一小时写缓存价，或手动配置的 `one_hour` 变体。
  - 未配置时，一小时写入按写缓存基础价计费。
- 不做的事见 D1、D2、D4、D5、D6。

## 链接

- [CCH 对照与取舍](../../reports/cch-comparison.md)：决定本功能改写范围与计费取舍的对照。
- 用户文档：[渠道管理](../../zh/guides/channel-management.md)、[成本追踪](../../zh/guides/cost-tracking.md)。

## 实现与维护

- 提交：
  - `bb25b45a`（`feat(channels): 渠道强制 1 小时提示缓存`）
  - `6c9c764d`（`fix(requests): 1 小时缓存标志改为以计费为准`）
- 代码，后端：
  - `internal/objects/channel.go`：`ChannelSettings.ForceOneHourPromptCache`，JSON `forceOneHourPromptCache`。
  - `internal/server/gql/axonhub.graphql`：`ChannelSettings` 与 `ChannelSettingsInput` 追加字段。
  - `internal/server/orchestrator/prompt_cache_ttl.go`：出站原始请求中间件。
    - 按 gjson 定位 R2 所列断点，只用 sjson 改写 `ttl`，其余字节不变。
    - 先复制请求体，不改动重试共享的底层数组。
  - `internal/server/orchestrator/orchestrator.go`：出站中间件的顺序为请求体透传、Responses 传输适配、
    透传请求头、Responses 续接、本中间件、渠道请求体覆盖。覆盖操作仍最终生效。
- 代码，前端：
  - `frontend/src/features/channels/{data/schema.ts,data/channels.ts,utils/merge.ts,components/channels-action-dialog.tsx}`：
    渠道编辑对话框的复选框。
  - `frontend/src/locales/{en,zh-CN}/channels.json`
  - `frontend/src/features/requests/{data/requests.ts,data/schema.ts,components/requests-columns.tsx,components/request-detail-content.tsx}`：
    - 列表与详情各加一个按 `promptWriteCachedTokens1hGT: 0` 过滤的用量计数查询。
    - 模型列第三个图标与用量卡片徽章。
  - `frontend/src/locales/{en,zh-CN}/requests.json`
- 用户文档：`docs/{zh,en}/guides/channel-management.md`、`docs/{zh,en}/guides/cost-tracking.md`。
- 迁移：
  - 无 data migration 与缓存键变化，渠道设置随 settings JSON 保存。
  - fork 的发布版本 `v1.0.0-beta10-fork.3` 曾带 `request_executions.one_hour_prompt_cache` 列，现已从 schema 删除。
    Ent 自动迁移（`WithDropColumn(true)`）会删列。
- 测试：`internal/server/orchestrator/prompt_cache_ttl_test.go`。
  - 各位置断点改为 1h 且其余字节不变。
  - 非 ephemeral 与非断点位置不动。
  - 关闭、非 Anthropic 格式、无断点、字符串 system、非法 JSON 原样返回。
  - 不改动重试共享的请求体。
- 同步上游：
  - 上游调整出站中间件顺序或 Anthropic 缓存断点处理（`ensure_cache_control.go`、`outbound.go` 的自动缓存）时，
    确认本中间件仍在请求体透传之后、覆盖之前执行。
  - 上游改用量记录的 1 小时写缓存字段时，同步请求日志标记的判定。
