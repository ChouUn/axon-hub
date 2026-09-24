# Fork 维护台账

> Parent: [AGENTS.md](../AGENTS.md)

本台账按 feature 和 bugfix topic 记录本 fork 相对上游 AxonHub 的差异，供新增 fork 改动和
同步上游时核对。这里只记录已确认的行为、代码归属和证据；是否保留、调整或删除
fork 改动，由用户决定。

## 维护检查表

### 新增或实质修改 Fork Topic

- [ ] 新增或更新一个独立 topic，不把无关改动混在同一条目中。
- [ ] 明确 topic 属于 feature 还是 bugfix。
- [ ] 记录用户可观察行为、边界、相关提交和主要代码路径。
- [ ] Bugfix 若为上游回补，记录上游来源、fork 中对应的提交及 fork 适配。
- [ ] 明确 schema migration、data migration、测试和 fixture 的影响；没有也要注明。
- [ ] 删除已经失效的描述，保证台账反映当前实现。

### 同步上游 Release

- [ ] 逐项阅读当前 topic，并对照上游在相关代码路径和行为上的变化。
- [ ] 同时核对 schema migration、data migration 及其版本顺序。
- [ ] 报告语义重叠、兼容性变化和迁移冲突，不只依赖 Git 冲突判断。
- [ ] 涉及保留、调整或删除 fork 改动时，先取得用户决定。
- [ ] 按最终实现更新 topic 中的事实、路径和相关提交。

## 上游同步记录

### v1.0.0-beta10（2026-09-09）

- 基线：`v1.0.0-beta7` → `v1.0.0-beta10`（上游 124 个提交），以 merge 合入，
  merge 提交 `7fdbd2ae`。
- 冲突：16 个文件。catalog 主题四个文件与两份 `providers.json` 快照直接取上游；
  `generated.go`、`internal/ent/internal/schema.go` 取上游后 `make generate`；
  `routeTree.gen.ts` 用 `@tanstack/router-generator` 重新生成；其余按 topic 手工拼接。
- 迁移：上游新增 `v1.0.0-beta9` data migration；schema 变化由 ent 托管，无冲突。
  注册顺序 `fork.1` → `beta8` → `beta9`，见「上游模型价格时间戳修复回补」。
- 验证：`go test ./...`（主模块）通过；前端 `node --test` 88 个通过；`tsc --noEmit`
  错误集合与上游 beta10 干净树一致（190 个上游既有错误，未新增）。未运行 lint、build
  与仓库 e2e 套件。
- 真机验收：本机 Docker（`axonhub:local`，SQLite）灌入三个验收 fixture 后，用一次性
  Playwright 脚本逐项核对渠道 tab 分组与计数、生图 tab 过滤与徽章、翻页无重复、
  新建/编辑对话框的 API 格式、两个分析页与模型页，21 项通过。过程中发现生图拆分的两处
  回归（列表字段、编辑同步），已在 `e18cc308` 修复。
- 逐项核对：

| Topic | 结果 |
| --- | --- |
| API 密钥费用分析、模型详情分析 | 上游改了分析筛选组件，自动合并无冲突 |
| 渠道 tab 按供应商聚合 | 上游 `aa8e7c81` 也补了 `ollama_anthropic`，已合一 |
| 生图渠道拆分 | 在上游重构之上重接；真机发现列表字段与编辑同步两处回归，已修 |
| Docker 镜像发布 | latest manifest 步骤冲突，保留 `${IMAGE}` |
| 模型目录后端热加载 | 上游版本为超集，fork 版本被取代 |
| 排序字段为零值时分页错乱 | entgql 版本未变，绕过仍需要；上游无新增分页入口 |
| Provider 模型成本可为空 | 上游未改，仍为 fork 差异 |
| 模型价格时间戳修复回补 | 上游 `beta9` 迁移与 `fork.1` 并存，测试 helper 改名 |
| 模型目录 schema 修复回补 | 与上游一致，无差异 |

## Feature Topics

### 模型入口校验与渠道分发

- 行为：正式请求在 API Key 模型映射后，必须解析到已注册且启用的模型，再按开发者与
  模型的有效关联选择渠道；模型不存在、禁用、归档或无可用关联时拒绝，不回退直连渠道。
- 映射与列表：API Key 映射采用首个匹配、不递归映射；公开模型列表只下发启用的注册
  目标及有效别名，不再合并渠道独有模型。OpenAI 模型详情使用映射目标元数据，保留对外别名。
- 设置：删除渠道回退、渠道模型列表合并及对应黑名单设置；旧 JSON 键被忽略，不能恢复旁路。
- 管理边界：Playground 指定渠道要求系统级 `write_channels`，项目所有者权限不能代替；显式查询渠道模型要求
  `read_channels`。已创建视频任务的查询、删除仍使用任务记录中的渠道，不重新选择推理模型。
- 同步上游注意：保留注册准入及映射目标校验，不能恢复按渠道支持模型直接选路；升级前
  核对未注册的渠道模型及映射目标，先补齐所需模型与关联，不自动注册或放宽准入。
- 提交：尚未提交。
- 代码：`internal/server/orchestrator/candidates.go`、`internal/server/orchestrator/model_mapper.go`、
  `internal/objects/apikey.go`、`internal/server/biz/model.go`、`internal/server/api/`、
  `internal/server/gql/model.resolvers.go`、`internal/server/gql/system.graphql`、
  `frontend/src/features/models/components/models-settings-dialog.tsx`。
- 迁移：无 schema 或 data migration；无缓存序列化结构变更。
- 测试与 fixture：`internal/server/orchestrator/model_admission_test.go`、候选渠道测试、
  `internal/server/biz/model_test.go`、`internal/server/biz/model_list_routability_test.go`、
  `internal/server/api/openai_retrieve_test.go`、`internal/server/api/chat_test.go`、
  `internal/server/gql/model_resolvers_test.go`、`internal/objects/apikey_test.go`，
  覆盖模型状态、关联限制、映射、Responses 续接和管理权限边界；无新增 fixture。

### 健康门控路由策略

- 状态：阶段一已实现。提交：`6298a0cb`（`feat(routing): 新增健康门控路由策略`）。
- 动机：管理端无法直观看到哪个渠道×模型坏了。现有 `(渠道, 模型)` 熔断器只挂在
  `circuit-breaker` 策略下，阈值写死，查询与重置方法无调用方
  （`internal/server/biz/model_circuit_breaker.go:144-149,359-435`）；默认 `adaptive`
  只按渠道扣分并在 5 分钟内衰减，坏渠道仍留在候选中，一个模型失败也会拖累同渠道其他
  模型（`internal/server/orchestrator/lb_strategy_bp.go:49-90`）；粘性渠道不经健康评分
  （`internal/server/orchestrator/candidates.go:709-721`），已坏的粘性渠道每个请求都会先
  失败一次再转移（粘性候选本身不做同渠道重试，`internal/server/orchestrator/outbound.go:694-699`）。
- 参考：CCH（`ding113/claude-code-hub` v0.9.5）的失败口径、全部不可用返回及管理端
  徽章/筛选/重置；不采用其按供应商整体熔断的粒度。
- 业务场景（用户确认）：
  - 长期红：某渠道×模型持续失败，不应频繁成为候选。
  - 间歇红：偶发失败、长期黄绿，本次请求需要绕开，下次仍可使用。
  - 单个渠道×模型损坏时，同渠道其他模型不受影响。
- 已确认决策：
  - D1 新增第五种负载均衡策略 `health-gated`（健康门控），与上游 `adaptive`、`failover`、
    `circuit-breaker`、`round-robin` 平行，可在系统重试策略、API Key profile 与模型设置
    中选择。上游四种策略的行为不变，以降低同步上游的冲突面。
  - D2 粒度：熔断主体为 `(渠道, 上游实际模型)`，即经渠道模型映射后发往上游的模型
    （候选项 `ActualModel`），不是请求模型或别名。
  - D3 全部候选熔断：选熔断最早到期的一个试一次；该次为计数失败或试探名额被占时返回 503，
    错误体为通用的服务不可用文案，不暴露渠道名（沿用 CCH，
    `src/app/v1/_lib/proxy/forwarder.ts:3214-3216,8626`）；结果为「不计入」类（如 400/404/422、429）
    时原样返回上游错误，不改写为 503。
    503 仅适用于尚未向客户端提交响应的失败；流式响应头已发出后
    （`internal/server/api/chat.go:190-197`）发生的中断沿用现有流内错误与终止语义，不重试、
    不改写 HTTP 状态码，健康结果按 R4、R6 记录；客户端取消后不再写响应。
  - D4 单机部署：健康状态只存进程内存，不做跨实例共享，重启后清零。
  - D5 统计范围：只统计走 `health-gated` 策略的流量；其他策略的请求不产生、不改变
    健康状态，保证管理端看到的熔断即实际生效的熔断。
  - D6 健康候选排序：沿用 `adaptive` 的组成去掉 ErrorAware，即 WeightRoundRobin、
    LatencyAware、RateLimitAware、QuotaAware（`internal/server/orchestrator/orchestrator.go:53-59`）。
  - D7 现有 `circuit-breaker` 策略及其熔断器保留原样，不改动、不复用其实例。
- 健康状态规则：
  - R1 状态：健康、不稳、熔断、试探。熔断与试探是互斥且优先的门控状态；仅在非熔断、
    非试探时，最近窗口 W 内有计数失败或首个 token 后断流才显示为不稳，否则为健康。健康
    与不稳同等参与候选（不稳仅用于展示，不降权，沿用 CCH）。熔断的组合在本次选路中不参与
    候选，也跳过粘性优先，但不删除、不改写粘性缓存。普通试探只放行无粘性渠道的新会话，
    同一渠道×模型同时只放一个请求（试探名额）。「无粘性渠道」以 trace/thread 粘性缓存原值
    为准（`internal/server/biz/request.go:1652-1671`），不以粘性渠道是否仍在候选中推断。
    有试探资格时，试探组合与健康候选按同一优先级分组和负载均衡评分正常排序，不前置也不后置。
  - R2 转换：连续计数失败达到 N 进入熔断；熔断到期进入试探；试探连续计数成功 M 次回到
    非门控状态（按 R1 的窗口规则显示健康或不稳），计数失败则重新熔断且时长翻倍（有上限）。
    健康或不稳下计数成功一次即清零连续失败。翻倍级数在恢复后保留，恢复后持续无计数失败
    超过熔断时长上限才清零；其间再次熔断从已达级数继续翻倍。
  - R3 计数单位：一次请求在某 `(渠道, ActualModel)` 上的尝试（含对该组合的同渠道重试）
    结束时记一次结果；中途失败、最终成功记为成功（沿用 CCH），最终失败按该组合最后一次尝试
    的分类记录。同渠道重试可能切换到该
    渠道的另一个 ActualModel（`internal/server/orchestrator/outbound.go:761-765`），各组合分别记录。
  - R4 结果分类：计数成功、计数失败、仅不稳、不计入四类（口径见 R6）。流式请求以完整
    终态为成功，收到 HTTP 200 或首个 token 不算成功。试探遇到「仅不稳」或「不计入」时释放
    试探名额，只更新最近异常信息，试探进度不增不减，保持试探状态等待下一个请求，不重新
    熔断也不延长熔断。
  - R5 无健康候选时的最后一试（D3 的细化）：经模型、权限等既有准入过滤后，若没有健康或
    不稳的候选，先按普通试探规则尝试取得试探名额；因本请求无试探资格（已有粘性渠道）、
    全部仍处于熔断或试探名额已被占用而无法取得时，进入最后一试。最后一试优先选已到期的
    试探组合，没有则选熔断最早到期的组合，按试探处理，计数失败按 R2 翻倍。最后一试与普通
    试探共用试探名额，选中组合的名额已被占用则立即返回 D3 的通用 503，不记健康失败。最后一试
    只尝试一次，不做同渠道重试。该
    出口不得返回模型不存在错误（现有空候选会映射为 `ErrInvalidModel`，
    `internal/server/orchestrator/select_candidates.go:111-115`）。
- 失败口径（R6，参考 CCH `src/app/v1/_lib/proxy/errors.ts:992-1064` 并按 AH 适配）：
  - 计数失败：上游 5xx、网络错误、超时（含 408 与首事件/非流式响应超时）、首个 token 前的
    流中断、空响应检测命中、401/403（按普通失败计数，与 CCH 一致；现有自动禁用规则照常独立
    运作）。
  - 仅不稳：首个 token 后的流中断，不计入连续失败。
  - 不计入：429、400/404/422 及其他未列出的 4xx、客户端取消、本地 RPM 与排队拒绝、熔断
    自身跳过（含试探名额被占）、无状态码且非网络/超时的本地错误（如请求转换失败）。
  - 429 边界：429 不计入本熔断器。仅当上游带可解析的 `Retry-After` 时沿用现有渠道冷却
    （`internal/server/orchestrator/rate_limit_tracking.go:90-102`）；无有效 `Retry-After` 时只影响
    本请求的故障转移，后续请求仍可正常选择该渠道，本 topic 不新增兜底冷却。
  - 与 CCH 的差异：CCH 将 429 及未命中客户端输入白名单的 400 计为供应商失败；AH 中
    429 按上一条处理，400/422 视为请求问题，均不计入。
- 可观测与管理规则：
  - R7 渠道列表：按渠道汇总徽章（N 个模型熔断 / N 个模型不稳），提供「只看异常」筛选。
  - R8 渠道健康详情：每个上游模型一行，含状态、连续失败次数、最近错误与状态码、熔断
    剩余时间、下次试探时间及重置操作。查看要求 `read_channels`，重置要求 `write_channels`。
    重置完全清零该组合：连续失败、试探进度、翻倍级数、最近异常及各截止时间全部清除，回到
    健康，并释放试探名额。重置开启该组合的新统计周期：重置前已发出的请求照常完成业务响应，
    但其迟到结果不改变新周期的健康状态，也不得释放新周期持有的试探名额。
  - R9 统一视图：凭证自动禁用在健康详情中一并展示，机制保持独立。429 冷却不展示：其状态
    由各接口编排器各持一份（`internal/server/orchestrator/orchestrator.go:37`），合并为全局会连带
    改变上游 RPM/TPM 计数行为。
- 配置规则（R10）：系统级参数（N、首次熔断时长、上限、M、W）放入重试策略；渠道级只覆盖
  阈值 N，放入渠道 `settings` JSON（沿用现有可选指针字段惯例，
  `internal/objects/channel.go:220-240`），留空沿用系统值。阈值为 0 时关闭该渠道熔断：
  该渠道各组合既不门控也不计数，管理端显示「熔断已关闭」而非残留状态。阈值由正数改为 0
  时结束原统计周期；由 0 改回正数时按空白健康状态开启新周期，关闭前及关闭期间发起的请求
  结果不回写新周期（与 R8 重置同一语义）。阈值切换在该组合下一次被选路、记录结果或被管理端
  读取时观察并生效；不为配置变更增加跨服务通知。不为配置新增表字段。
- 生命周期（R11）：渠道停用、删除、归档或模型映射变更时不主动清理健康状态，只随时间演进
  或由管理员重置。条目数以实际用过的渠道×上游模型组合为上限，不做定期回收。
- 默认值：N=5；首次熔断 5 分钟，试探失败翻倍，上限 60 分钟；M=2；W=5 分钟。
- 明确不做：不改变上游四种策略的行为；不引入跨实例共享或健康状态持久化；不向上游发
  合成探测请求（试探只用真实业务请求：普通试探仅放行无粘性渠道的请求，R5 最后一试除外）；
  不替代 429 冷却与自动禁用；不稳状态不降权。
  会话归属、请求决策记录与 Webhook 见「健康门控会话归属迟滞与决策记录」。
- 代码：新策略与健康状态使用新文件，不改上游熔断实现：
  `internal/server/biz/health_gate.go`（状态机）、`internal/server/biz/system_health_gate.go`（配置）、
  `internal/server/orchestrator/health_gate_{selector,middleware,outcome,errors,load_balancer}.go`、
  `internal/server/gql/channel_health_gate{.graphql,.go,.resolvers.go}`、
  `frontend/src/features/channels/components/channels-health-gate-dialog.tsx`。
  上游文件只做追加式接入：`internal/objects/routing.go`、`internal/objects/channel.go`（渠道阈值字段）、
  `internal/server/biz/system.go`（策略常量、`RetryPolicy.HealthGate`、归一化调用）、
  `internal/server/biz/channel.go`（状态字段）、`internal/server/biz/channel_query.go`（筛选入参）、
  `internal/server/orchestrator/orchestrator.go`（负载均衡器、中间件、`finalize` 改写最后一试错误）、
  `internal/server/orchestrator/candidates.go`（选路钩子）、`internal/server/orchestrator/load_balancer.go`、
  `internal/server/orchestrator/outbound.go`（最后一试不做同渠道重试）、`internal/server/gql/` 的
  `system.graphql`、`axonhub.graphql`、`gqlgen.yml`、`axonhub.resolvers.go`（`QueryChannels` 一行筛选钩子）
  与生成文件；前端策略下拉（`frontend/src/features/system/components/retry-settings.tsx`、
  `frontend/src/features/apikeys/components/apikeys-*-dialog.tsx`、
  `frontend/src/features/models/components/models-association-dialog.tsx`、
  `frontend/src/features/models/data/schema.ts`）、`frontend/src/features/channels/`、
  `frontend/src/locales/`；用户文档 `docs/{zh,en}/guides/load-balance.md`。
- 迁移：无 schema 或 data migration；重试策略新增可选 `health_gate` 对象、渠道 `settings`
  新增可选 `healthGateFailureThreshold`，旧数据缺省走系统默认。
- 测试与 fixture：`internal/server/biz/health_gate_test.go`（状态机、翻倍与清零、代次失效、
  阈值 0 切换、归一化）、`internal/server/orchestrator/health_gate_outcome_test.go`（失败口径边界）、
  `health_gate_selector_test.go`（门控、粘性跳过、试探资格、最后一试选择）、
  `health_gate_orchestrator_test.go`（端到端熔断、503 与透传、流式首 token 后断流）、
  `internal/server/gql/channel_health_gate_test.go`（状态映射与计数）。
- 同步上游注意：策略枚举与前端策略下拉为追加式冲突点；上游若新增策略、改动负载均衡
  组装或自行实现熔断可视化，需逐条对照 D1–D7、R1–R11。

### 健康门控会话归属迟滞与决策记录

- 状态：已实现。提交：`a4ffb86d`（`feat(routing): 健康门控会话归属迟滞与决策记录`）。
- 依赖：建立在「健康门控路由策略」之上，仅在 `health-gated` 策略下生效；其他策略保持
  上游粘性语义。
- 动机：会话粘性在每次尝试前改写（`internal/server/orchestrator/request_execution.go:105-113`、
  `internal/server/biz/request.go:1269-1283`），故障转移后又被下一次波动切回，每次切换
  都可能落到冷缓存、推高费用。管理端也看不到某次请求为何跳过或转移渠道，熔断发生时
  无通知。
- 参考：CCH 的会话绑定同样只在成功时写入，但故障转移成功即改绑、健康时迁回更高优先级
  供应商（`src/lib/session-manager.ts:1659-1757`），无迁移迟滞，R1 的 thread 主键与 R3–R5
  为 AH 自有需求；
  决策记录参考 CCH 请求表的 `provider_chain` / `routing_trace` jsonb 列
  （`src/drizzle/schema.ts:571-575`）；Webhook 参考 CCH 开闸告警。
- 业务场景（用户确认）：会话不因单次波动在渠道间来回切换，也不长期停在失败渠道上。
- 已确认决策：
  - D1 不迁回，原渠道恢复后已迁走的会话留在新渠道，缓存成本优先。
  - D2 切换到 `health-gated` 时沿用旧粘性记录：新归属记录缺失时读取一次旧渠道记录作为归属，
    连续转移计数从 0 开始；旧记录只含渠道且在尝试前写入，可能指向失败渠道，由 R3/R4 纠正。
  - D3 K 为系统级参数，放在重试策略的健康门控参数中，不提供渠道覆盖。
  - D4 决策记录包含：请求开始时的归属渠道×模型、因熔断被跳过的渠道×模型、是否临时转移、
    是否最后一试、本次是否迁移（迁出、迁入与原因：归属熔断或连续 K 次转移）。
  - D5 Webhook 只在首次熔断与恢复时发送；熔断期间试探失败、翻倍延长不发送。
- 会话归属规则：
  - R1 会话主键：有 thread 时，归属与连续转移计数只以 thread 缓存为权威，选路不读取、不被
    任何 trace 旧值覆盖（现有选路 trace 优先，`internal/server/orchestrator/candidates.go:749-776`，
    `health-gated` 下需改为 thread 优先）；没有 thread 时才用 trace。成功提交迁移时同步改写
    当前 trace 的归属，仅用于一致性与展示，不是 thread 归属生效的前提。归属缓存失效（30 分钟，
    每次成功提交续期）即视为新会话，不回退使用旧 trace 归属；D1 的不迁回只在归属有效期内保证。
    归属以渠道为单位，「归属渠道×模型」指归属渠道上本次请求选用的上游实际模型。仅在
    `prefer_previous_channel` 粘性模式下生效；粘性关闭时不读写归属。
  - R2 归属只在请求成功时写入，失败尝试不改写。提交时以会话当前归属为准重新计算（按会话主键进程内串行），
    同一会话并发请求中晚完成者不得把已迁移的归属写回旧渠道；多实例共享缓存时不保证跨实例串行。
  - R3 单次故障转移：由备用渠道完成本次请求，归属不变，下个请求仍回归属渠道。
  - R4 迁移条件：本次请求结束时归属渠道×模型处于熔断或试探（未获本次准入，含本次请求自身触发的熔断），或同一会话连续 K 次
    请求都需故障转移才完成；归属渠道不在本次候选中也按故障转移计数。迁往完成该次请求的渠道，
    计数清零。归属渠道直接成功时连续转移计数清零；请求整体失败或客户端取消时计数不变。
  - R5 迁移后不迁回（D1）。
  - R6 归属渠道×模型已熔断时不参与粘性优先及同渠道重试，但仍可按阶段一 R5 成为最后一试；
    未熔断时沿用现有粘性候选零次同渠道重试（`internal/server/orchestrator/outbound.go:694-699`），
    失败即转移。
- 决策记录与通知规则：
  - R7 请求决策记录：请求表新增可空 JSON 字段，内容见 D4，请求详情页展示；仅 `health-gated`
    请求写入，请求结束（非流式完成或失败、流式关闭）时写一次；渠道名称按选路时快照保存（归属渠道
    不在候选中时按当时的渠道记录补齐）。
  - R8 Webhook：新增渠道×模型熔断（`channel.health_gate_opened`）与恢复
    （`channel.health_gate_recovered`）事件，复用现有通知器（按事件名匹配订阅，
    `internal/server/biz/webhook_notifier.go:144-170`），异步发送，不在健康状态锁内调用；
    系统 Webhook 设置界面需增加事件选项。事件为进程内转移，多实例不去重。
- 默认值：K=2（`owner_failover_threshold`，≤0 归一为 2）；归属渠道同渠道重试 0 次（沿用现状）。
- 代码：新逻辑放在新文件：`internal/server/biz/session_owner.go`（归属缓存读写、决策持久化）、
  `internal/server/biz/health_gate_notify.go`（熔断转移异步通知）、
  `internal/server/orchestrator/health_gate_session.go`（归属判定与决策提交）、
  `internal/objects/request_routing.go`（决策记录类型）、`internal/server/gql/request_routing.graphql`。
  阶段一文件扩展：`internal/server/biz/health_gate.go`（转移回调，锁外调用）、
  `internal/server/biz/system_health_gate.go`（K）、`internal/server/orchestrator/health_gate_selector.go`
  （thread 优先归属、跳过项与最后一试快照）、`internal/server/orchestrator/health_gate_middleware.go`（成功时提交）。
  上游文件只做追加：`internal/server/biz/request.go`（归属缓存字段）、`internal/server/biz/webhook_notifier.go`
  （两个事件、`.Model.ActualModel` 与 `.Trigger.OpenUntil` 模板变量）、`internal/ent/schema/request.go`（字段）、
  `internal/server/orchestrator/request_execution.go`（`health-gated` 下尝试前不写粘性缓存）、
  `internal/server/orchestrator/orchestrator.go`（每个 Process 的决策槽位）、`internal/server/gql/system.graphql`、
  `internal/server/gql/gqlgen.yml` 与 ent/gqlgen 生成物；前端 `frontend/src/features/requests/`（路由决策卡片）、
  `frontend/src/features/system/components/{retry-settings,webhook-settings}.tsx`、`frontend/src/features/system/data/system.ts`、
  `frontend/src/locales/`；用户文档 `docs/{zh,en}/guides/load-balance.md`。
- 迁移：请求表新增可空 JSON 列 `routing_decision`，由 ent 自动迁移，无 data migration。
  归属使用新缓存键 `axonhub:routing:session-owner:v1:{thread|trace}:%d`，值为
  `{channel_id, consecutive_failovers}`；不改动旧 `previous-channel:v1` 键的值形状
  （`.agent/rules/cache-compat.md`）。`health-gated` 下不在尝试前写旧键，成功提交归属时同步写
  旧键与新键（同一 TTL），便于切回其他策略时保持粘性。
- 测试与 fixture：`internal/server/biz/session_owner_test.go`（thread 权威、D2 旧键种子、新旧键同写与 TTL、
  缓存读错误、决策读写）、`internal/server/biz/health_gate_test.go`（只在首次熔断与恢复产生转移、回调锁外可重入）、
  `internal/server/biz/health_gate_notify_test.go`（事件订阅匹配、渲染字段、异步桥接）、
  `internal/server/orchestrator/health_gate_session_test.go`（单次转移保留归属、连续 K 次迁移、归属熔断立即迁移、
  归属成功清零、失败与取消不写、流式仅完成后提交、粘性关闭不读写）、
  `internal/server/orchestrator/health_gate_selector_test.go`（跳过项、最后一试剔除、归属优先）。
- 同步上游注意：在 `request_execution.go`、`candidates.go`、`biz/request.go` 中按策略分支
  改变粘性写入时机，为冲突高发点；上游改动粘性缓存或请求表结构时需逐条对照 R1–R8。

### 渠道标准价格引用与持久倍率

- 行为：渠道定价引用模型模块维护的完整价格，支持按渠道模型 ID 一键匹配，也可为
  指定渠道模型选择其他标准价格；带入基础单价、全量阶梯、缓存变体及时间覆盖，
  引用后仍可修改。无标准价格的模型不覆盖既有渠道价格，不再从外部目录填充。
- 倍率：按渠道独立保存，缺省为 1，支持显式零值；基础价格不随倍率改写，各类单价
  输入框右侧同一行只读展示实际价格，不另占一行。保存时将倍率写入价格版本快照，
  倍率变更生成新版本；历史版本不重算，普通渠道设置修改不覆盖倍率，自动补缺及
  渠道复制继承倍率。
- 交互：定价窗口使用视口高度，编辑区独立滚动，顶部引用与倍率、底部操作区固定；
  各行价格输入框等宽，每档提供基础价倍数及整档填充，包含缓存变体，填充后可单项修改；
  新版导出同时保存倍率与基础价格，旧数组格式导入按倍率 1 承接。
- 提交：`2eca7c59`（渠道标准价格引用、持久倍率与价格编辑交互）。
- 代码：`frontend/src/features/channels/components/channels-model-price-dialog.tsx`、
  `frontend/src/components/model-price-editor.tsx`、`internal/server/biz/channel_price.go`、
  `internal/server/biz/cost_calc.go`、`internal/objects/channel.go`、`internal/objects/price.go`、
  `internal/server/gql/price.graphql`。
- 迁移：无新增表字段或迁移；渠道设置、当前价格及历史版本使用兼容的 JSON 可选
  字段，旧数据缺省倍率为 1。备份保留倍率快照，不从当前渠道设置重算历史价格。
- 测试与 fixture：`internal/server/biz/channel_price_test.go`、`internal/server/biz/cost_calc_test.go`、
  `internal/server/biz/channel_auto_model_price_test.go`、`internal/server/biz/channel_model_sync_test.go`、
  `frontend/src/features/models/pricing.test.mjs`；无新增 fixture。

### 模型权威定价与全量阶梯

- 行为：模型新建、编辑及目录批量导入统一保存完整标准价格，列表和详情展示基础价格、
  全量阶梯及缓存写入变体。直接展示并编辑已有价格，不设「模型定价」开关，不因开关
  清空或重建价格；零价与未填写价格区分。
- 全量阶梯：输入总 tokens（含缓存）严格超过所填阈值时，全部计费项使用命中档位
  单价；支持多档，以及一次按基础价倍数填满整档后单独改价。填充倍数只是编辑操作，
  不作为另一项计费倍率存储。既有时间覆盖命中时仍优先整组替换价格。
- 编辑交互：新增计费项和缓存写入变体的价格留空，明确填写的零价保留；新增阶梯阈值
  留空且填写有效阈值前不能保存，已有阶梯照常回显。添加按钮显示「添加计费项」，
  四项齐全时禁用并显示「已添加全部计费项」，删除一项后恢复可用。
- 目录选模修复：替换模型卡时同步重置价格及阶梯表单数组，无阶梯时明确清空数组；
  避免数据已填入却不显示价格行、已有阶梯开关未开启，以及切换模型后残留旧档位。
- 兼容：旧模型卡价格在 JSON 读取时转换为完整价格，旧成本字段仅作只读摘要；明确
  清除后不会从旧摘要恢复。已有渠道价格、历史价格版本和旧逐项阶梯计算不变。
- 渠道：既有自动补缺逻辑复制完整模型价格（含阶梯和缓存变体），不覆盖已有渠道价格；
  渠道引用、编辑及倍率衔接见「渠道标准价格引用与持久倍率」。
- 提交：`93e5878e`（权威模型定价与全量阶梯）；`2eca7c59`（渠道完整价格引用、倍率衔接与阶梯填充交互）。
- 代码：`frontend/src/features/models/components/models-price-editor.tsx`、
  `frontend/src/features/models/data/pricing.ts`、`frontend/src/components/model-price-editor.tsx`、
  `internal/objects/model.go`、`internal/objects/price.go`、`internal/server/biz/model.go`、
  `internal/server/biz/cost_calc.go`。
- 迁移：无新增表字段或迁移；兼容既有数据库及缓存中的模型卡 JSON。已重新生成接口代码。
- 测试与 fixture：`internal/objects/model_test.go`、`internal/objects/price_test.go`、
  `internal/server/biz/model_validation_test.go`、`frontend/src/features/models/pricing.test.mjs`，
  覆盖旧价格兼容、全量阶梯校验、价格保存与清除、整档填充及零价与未填写价格的区分；
  无新增 fixture。

### API 密钥费用分析

- 行为：仪表盘按 API 密钥汇总请求数、Token 数和费用，以费用排名，并可展开查看
  模型明细；默认全部收起。
- 筛选：支持密钥模板，以及今天、昨天、本周、本月和全部时间；自然日边界使用
  系统设置中的时区。
- 提交：`d3d4160e`（新增功能）；`7415d058`（统一系统时区日期边界）。
- 代码：`frontend/src/features/analytics/`、`internal/server/gql/analytics.graphql`、
  `internal/server/gql/analytics_helpers.go`。
- 迁移：无。
- 测试与 fixture：`internal/server/gql/analytics_helpers_test.go`、
  `frontend/src/features/analytics-date-range.test.mjs`、
  `scripts/e2e/fixtures/api-key-analytics.sql`。

### 模型详情分析

- 行为：仪表盘以模型为汇总项，以实际请求渠道为明细项，按费用排名且默认全部收起；
  支持按渠道标签筛选。
- 指标：请求数、费用、Token 数、费用/Mtok、成功率、缓存命中率、平均 TTFB 和
  平均输出速度；缓存命中率按缓存输入 Token 占输入 Token 的百分比计算，输入 Token
  为零时显示 0%。
- 输出速度口径：输出 Token 只取 `completion_tokens`（其中已含 reasoning 与 audio），
  与上游 beta8 起（`92f81b32`）的仪表盘吞吐一致。此前 fork 把三者相加，高估
  reasoning 模型的速度，`62011e01` 修正。
- 列顺序：排名、模型/渠道、请求数、Token 数、成功率、缓存命中率、平均 TTFB、
  平均输出速度、费用/Mtok、费用；表格不重复显示内部标题。
- 排序：默认按费用降序。除排名外，各列通过无边框列头按钮在降序、升序和无序间
  切换；进入无序时恢复费用降序。排序同时作用于模型和各自的渠道明细，排名按当前
  顺序重算，模型的展开状态不随排序改变。
- 重试语义：`request_executions` 中每次 `completed` 或 `failed` 执行均视为一次实际请求，
  包括渠道内和渠道间重试；失败执行计入请求数和成功率，费用与 Token 来自成功用量。
- 日期：支持今天、昨天、本周、本月和全部时间；自然日边界使用系统设置中的时区。
- 提交：`7415d058`（`feat(analytics): 新增模型详情分析`）；
  `62011e01`（输出速度口径对齐上游）。
- 代码：`frontend/src/features/analytics/`、`internal/server/gql/analytics.graphql`、
  `internal/server/gql/analytics_helpers.go`。
- 迁移：无。
- 测试与 fixture：`internal/server/gql/analytics_helpers_test.go`、
  `frontend/src/features/analytics-date-range.test.mjs`、
  `scripts/e2e/fixtures/model-analytics.sql`。

### 渠道 tab 按供应商聚合

- 行为：渠道列表顶部 tab 按 `CHANNEL_TYPE_TO_PROVIDER` 把同一供应商的协议变体聚成
  一组：`deepseek` / `deepseek_anthropic` 归入 DeepSeek，`moonshot` /
  `moonshot_anthropic` / `moonshot_coding` 归入 Moonshot。标签与图标取自
  `PROVIDER_CONFIGS`，tab 筛选使用该组实际类型列表而不是类型名前缀；未在映射中的
  类型保留独立分组，不会消失。顺带补齐映射缺失的 `ollama_anthropic`。徽章列原本已
  显示供应商，未改。
- 动机：上游按下划线前缀折叠，且只在裸前缀类型同时存在时才折，导致
  `moonshot_anthropic` + `moonshot_coding`、`opencode_go` + `opencode_go_anthropic`
  各自成组，而 `github_copilot` 又被误并入 GitHub。
- 同步上游注意：上游 `aa8e7c81`（beta8 起）也把 `ollama_anthropic` 映射到 ollama，
  合并后为同一行，不再是 fork 差异。
- 提交：`54fab437`（`feat(channels): 按供应商聚合渠道 tab`）。
- 代码：`frontend/src/features/channels/utils/group-channel-types.ts`、
  `frontend/src/features/channels/components/channels-type-tabs.tsx`、
  `frontend/src/features/channels/index.tsx`、
  `frontend/src/features/channels/data/config_channels.ts`。
- 迁移：无。
- 测试与 fixture：`frontend/src/features/channels/group-channel-types.test.mjs`、
  `scripts/e2e/fixtures/channel-groups.sql`。

### 生图渠道拆分

- 行为：`ChannelSettings.primaryApiFormat` 标记渠道主用途（JSON 列）。新建/编辑对话框对
  默认端点含 `openai/image_generation` 的供应商增加「OpenAI (图像生成)」选项，选中后
  类型仍为供应商基类型，写入 `settings.primaryApiFormat=openai/image_generation`。
  带该标记的渠道在 tab 中单独成组「<供应商> · 生图」（组 key `<provider>:image`），
  徽章追加同一后缀；供应商普通 tab 以 `excludePrimaryApiFormat` 排除生图渠道。
  渠道测试（含单 key 与批量 key）对该标记改发一次 1024x1024、`quality=low`、`n=1`
  的图像生成请求且不走流式，成功判定为响应至少含一张图。已有生图渠道需编辑一次
  并改选该 API 格式后才会分组与改测。
- 边界：该标记不影响路由，渠道仍按默认端点同时承接 `image_edit` /
  `image_variation`。选中该格式后切换供应商时，类型须由
  `getBaseChannelTypeForProvider` 推导，`getChannelTypeForApiFormat` 对图像格式恒为
  undefined，否则表单 `type` 与 baseURL 停在上一家。`excludePrimaryApiFormat`
  谓词必须兼容 `settings` 中不存在该键的旧行，否则普通渠道会被一起滤掉。
  生图测试耗时常超过后台请求的 30 秒上限，测试入口改以 `server.channel_test_timeout`
  为界，见「渠道测试受后台请求超时限制」。
  `countChannelsByType` 改为按 `(type, 是否生图)` 在内存中计数并返回
  `primaryApiFormat`，供分组函数拆组。
- 明确不做：不移除任何现有 API 格式选项（含 Anthropic Messages），不新增渠道类型，
  不改路由与端点解析。`codex` 虽有图像默认端点，但其对话框隐藏 API 格式选择器，
  未加入可选供应商。
- 依赖：建立在「渠道 tab 按供应商聚合」之上。
- 上游 beta10 适配：`getApiFormatsForProvider` 上游已委托给 `protocol-options.ts`，
  fork 只在 `config_providers.ts` 的包装层追加生图格式，不改上游文件。渠道测试的
  `marshalChannelTestBody` 接受上游新增的 `responsesWebSocket` 参数；`testSingleKey`
  同时保留 fork 的 `ch *biz.Channel` 与上游的 `responsesWebSocket`。编辑对话框的
  `settingsPatch` 与新建时 `mergeChannelSettingsForUpdate` 的参数中保留
  `primaryApiFormat`。
- beta10 真机回归：上游列表查询改为按可见列裁剪字段
  （`CHANNEL_QUERY_LIST_NODE_BASE_SELECTION`），fork 须在其中补
  `settings { primaryApiFormat }`，否则徽章无后缀。对话框新增的 `initialRow` 同步
  effect 与打开时的重置逻辑用上游 `getInitialApiFormatForChannel` 覆盖了初始化时选中的
  图像格式，编辑生图渠道显示 Chat Completions 并会在保存时清掉标记。三处统一为
  `getInitialApiFormatForRow`（`e18cc308`）。
- 提交：`d6d17d00`（`feat(channels): 拆分生图渠道`）；`7fdbd2ae`（合入 beta10 时重接）；
  `e18cc308`（补回列表字段与编辑同步）。
- 代码：`internal/objects/channel.go`、`internal/server/gql/axonhub.graphql`、
  `internal/server/gql/axonhub.resolvers.go`、`internal/server/biz/channel_query.go`、
  `internal/server/orchestrator/tester.go`、
  `frontend/src/features/channels/utils/group-channel-types.ts`、
  `frontend/src/features/channels/components/channels-type-tabs.tsx`、
  `frontend/src/features/channels/index.tsx`、
  `frontend/src/features/channels/components/channels-action-dialog.tsx`、
  `frontend/src/features/channels/components/channels-columns.tsx`、
  `frontend/src/features/channels/data/config_channels.ts`、
  `frontend/src/features/channels/data/config_providers.ts`。
- 迁移：无。
- 测试与 fixture：`frontend/src/features/channels/group-channel-types.test.mjs`、
  `internal/server/orchestrator/tester_test.go`、
  `internal/server/biz/channel_query_test.go`、
  `scripts/e2e/fixtures/channel-groups.sql`。

### Docker 镜像发布到 fork 仓库

- 行为：复用上游 `docker-publish.yml`（监听 `v*` tag，amd64 / arm64 原生构建后合成
  manifest，构建前把 tag 写入 `internal/build/VERSION`），镜像名由 workflow 级
  `env.IMAGE` 指定为 `chouun/axon-hub`；产出 `<tag>`、`<tag>-<arch>`、`latest`。
- 发布：tag 命名为 `v1.0.0-<上游基线>-fork.N`（如 `v1.0.0-beta10-fork.1`），
  `git tag -a vX -m "release: vX"` 后 `git push fork release/v1.0.x --follow-tags`。
  fork 仓库需启用 workflow，并配置 repository secrets `DOCKERHUB_USERNAME` /
  `DOCKERHUB_TOKEN`；`release.yml`（goreleaser）在 Actions 页面单独禁用。
- 动机：此前手工 `docker build .` 未写 VERSION，镜像自报仓库内上游值 `v1.0.0-beta8`，
  启动时 `datamigrate` 把 `system_version` 抬到 beta8，此后所有 `v1.0.0-beta7-fork.N`
  数据迁移都会被跳过，且镜像内容不保证等于 tag。
- 存量实例：曾被误抬到 `v1.0.0-beta8` 的实例不再需要手工回拨。上游 `v1.0.0-beta9`
  迁移覆盖了 `fork.1` 的清理，且版本比较修复（`6c07e14e`）后部署
  `v1.0.0-beta10-fork.N` 会正常把 `system_version` 抬上去。
- 同步上游注意：上游 manifest 步骤硬编码 `looplj/axonhub`，合并时会在该处冲突，
  保留 `${IMAGE}`。
- 提交：`4dc42806`（`feat(ci): 发布 Docker 镜像到 fork 仓库`）；
  `7fdbd2ae`（合入 beta10）。
- 代码：`.github/workflows/docker-publish.yml`。
- 迁移与测试：无。

## 提前回补的上游 Feature

### 模型目录后端热加载

- 状态：已随 `v1.0.0-beta10` 合入上游版本，不再是 fork 差异。
- 行为：模型页与渠道模型价格弹窗共用后端 `providersCatalog(filtered)`；后端从
  PublicProviderConf 拉取并缓存，可在系统设置中修改上游 URL 与刷新间隔。
- 上游来源：`4483c2e4`（PR #2263）。
- 提交：`d6a605a1`（提前回补）；`7fdbd2ae`（合入上游版本）。
- 合并处理：`catalog_filter.go`、`system.graphql`、`system.resolvers.go`、
  `general-settings.tsx` 与两份 `providers.json` 快照直接取上游，上游为 fork 版本的
  超集（含 `0d85ba60` 的 hy4 过滤修复）；`internal/ent/internal/schema.go` 由
  `make generate` 重新生成，原 fork 适配无残留。
- 已知差异：无。内嵌快照已随上游更新。

## Bugfix Topics

### 渠道测试受后台请求超时限制

- 问题：渠道测试经 `/admin/graphql` 触发，该路由挂 `server.request_timeout`
  （默认 30s），正式 `/v1` 接口则用 `server.llm_request_timeout`（默认 600s）。
  生图上游约 40 秒返回时，测试在 30 秒被掐断并报 `context deadline exceeded`，
  上游却已成功并计费。聊天测试几秒即回，此前未暴露。
- 行为：新增配置 `server.channel_test_timeout`（默认 `180s`）。单个测试、单 key 测试与
  批量 key 测试入口用 `xcontext.DetachWithTimeout` 脱离请求 deadline，改以该值为上限；
  `http.Server` 的 `WriteTimeout` 取 `request_timeout`、`llm_request_timeout` 与该值的
  最大值。设为 `0` 保持原行为。测试脱离请求取消后，控制台断开也会跑到结束或超时。
  GraphQL 事务中间件对这三个 mutation 改为按字段跳过事务（上游只按操作名跳过前两个，
  匿名操作与单 key 测试仍被包进绑定请求 context 的事务，超时即回滚）。
- 边界：正式接口不受影响；反向代理（nginx、Cloudflare）的读超时需自行放宽。系统设置
  「重试策略 → 非流式传输超时」若开启，仍会在更早时刻以 pipeline 自己的错误终止。
- 提交：`321cfba5`（`fix(channels): 渠道测试改用独立的 channel_test_timeout`）；
  `ea465c71`（`fix(channels): 渠道测试 mutation 按字段跳过请求事务`）。
- 代码：`conf/conf.go`、`internal/server/config.go`、`internal/server/server.go`、
  `internal/server/orchestrator/tester.go`、`internal/server/gql/resolver.go`、
  `internal/server/gql/graphql.go`。
- 迁移与测试：无迁移；`internal/server/orchestrator/tester_timeout_test.go`
  （`TestTestChannelOutlivesRequestDeadline`，本地慢响应 mock 复现，修复前报同样错误）；
  `internal/server/gql/graphql_skip_tx_test.go`。本机 Docker（`axonhub:local`，SQLite）
  对照验证：mock 生图上游 45 秒返回，默认配置下 `testChannel` 与 `testChannelAPIKey`
  均 45 秒成功；`channel_test_timeout=0` 时两者 30 秒失败并报同样错误。
- 同步上游注意：`NewSchema`、`Dependencies` 与 `NewTestChannelOrchestrator` 多了一个
  参数，上游改动这些签名时需重接；若上游为 GraphQL 路由或渠道测试引入自己的超时，
  以上游为准并移除本配置。

### 透传时上游错误体被重包装

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

### 迁移器把 beta10 排在 beta9 之前

- 问题：`datamigrate` 用 Masterminds semver 比较版本，`beta10` 与 `beta9` 作为字母数字
  标识按字符串比较，得到 `beta10 < beta9`。beta8/beta9 实例升到
  `v1.0.0-beta10-fork.N` 后 `system_version` 抬不上去，之后所有 `beta10-fork.N`
  数据迁移被静默跳过；上游自身从 beta9 升 beta10 同样不抬版本。
- 上游来源：上游 beta10 的 `migrator.go` 比较逻辑未变，尚未处理；fork 先行修复，可回馈。
- 行为：`parseComparableVersion` 把 `betaN`、`rcN` 拆成 `beta.N`，`beta7-fork.5` 拆成
  `beta.7.fork.5`，只用于比较，存储与展示的版本字符串不变。迁移版本、系统版本与
  构建版本三处比较均改用该函数。
- 提交：`6c07e14e`（`fix(datamigrate): 预发布号按数值比较`）。
- 代码：`internal/ent/migrate/datamigrate/version.go`、
  `internal/ent/migrate/datamigrate/migrator.go`。
- 迁移与测试：无迁移。`version_test.go` 覆盖 beta6 到 1.0.0 的完整顺序；
  `migrator_test.go` 的 `TestMigrator_Run_NumericPrereleaseOrdering` 覆盖 beta9 系统
  执行 beta10 迁移并抬版本、beta10-fork.1 系统跳过 fork.1 与 beta10。
- 同步上游注意：若上游改为 `beta.10` 式命名或自行修复比较，本函数可退化为无害冗余。
  `internal/server/biz/version.go` 的更新检查仍用原生 semver，未改。

### 排序字段为零值时分页错乱

- 问题：entgql 的 `Cursor.Value` 带 `msgpack:"v,omitempty"`，排序字段值为该类型零值时
  （如 `ordering_weight` 为 0）游标丢掉该值，解码为 nil 后 `entgql.CursorsPredicate`
  退化为只按 id 比较，下一页会重复或漏掉行。表现为渠道页按权重排序、每页 20 条时，
  第一页末尾是权重 0，第二页开头又出现权重 5、10。
- 上游来源：ent/contrib `3625dcc2e035`（本 fork 锁定版本）仍带 `omitempty`，未见修复；
  本 fork 不改依赖，在调用侧绕过。
- 行为：`biz.RestoreZeroCursorValue` 在 `Paginate` 前用排序字段在空实体上取到的零值
  补回 `cursor.Value`（`omitempty` 只省略零值，nil 必然对应零值）。接入
  `ChannelService.QueryChannels`、`Query.channels`、`Query.prompts`（`prompt.order`
  同样默认 0）。默认 ID 排序的 `Field.String()` 为空，跳过不补。渠道页切换排序时重置
  分页游标，避免旧排序下产生的游标被带入新排序。
- 提交：`973928e3`（`fix(channels): 修复排序值为零时游标分页错乱`）。
- 代码：`internal/server/biz/cursor.go`、`internal/server/biz/channel_query.go`、
  `internal/server/gql/ent.resolvers.go`、`frontend/src/features/channels/index.tsx`。
- 触发条件：只在某页最后一行的排序值为零时触发，与 tab 无关。「全部」若带权重的渠道
  不少于 20 个，第一次翻页落在非零权重上看起来正常，翻到零权重之后的下一页同样出错；
  分类 tab 带权重渠道少，第一次翻页就出错。
- 迁移与测试：无迁移；`internal/server/biz/channel_query_test.go`
  （`TestChannelService_QueryChannels_OrderingWeightZeroCursor` 与
  `TestChannelService_QueryChannels_VendorTabZeroCursor`，游标经
  `MarshalGQL`/`UnmarshalGQL` 往返以复现，后者同时覆盖「全部」与分类 tab 逐页遍历）。
- 同步上游注意：若上游 entgql 去掉 `omitempty`，该绕过变为无害冗余，可移除；若新增按
  可为零值字段排序的分页入口，需同样接入。beta10 仍锁定 `3625dcc2e035`，
  `ent.resolvers.go` 的 `Paginate` 调用点未增加，无需改动。

### Provider 模型成本可为空

- 行为：模型目录中的 `cost` 可以缺失或为 `null`，远程拉取和内嵌快照的 provider 数据均可解析；
  不改变渠道价格的存储格式。
- 提交：`d4aa15c7`（`fix(models): 允许提供商成本为空`）。
- 代码：`frontend/src/features/models/data/providers.schema.ts`。
- 迁移与测试：无迁移；无专用测试。

### 上游模型价格时间戳修复回补

- 问题：上游保存渠道模型价格时使用 `time.Now()`，可能使 SQLite 持久化带 monotonic
  后缀的 `updated_at`，导致后续乐观锁更新无法匹配。
- 上游来源：`86eac3a7`（PR #2257）。
- 提交：`4896d9d6`（`fix(channels): 统一模型价格时间戳`）。
- Fork 适配：将上游 beta9 数据清理适配为 `v1.0.0-beta7-fork.1`；仅清理 SQLite，
  PostgreSQL 跳过不适用的文本处理。
- 代码：`internal/server/biz/channel_price.go`、
  `internal/ent/migrate/datamigrate/`。
- 上游 beta10：上游 `v1.0.0-beta9` 数据迁移已合入，包含同一 SQLite 清理，另加
  `settings.providerQuota` 清理，`e4bf324d` 起同样跳过 PostgreSQL。它与 `fork.1`
  同时注册，顺序 `fork.1` → `beta8` → `beta9`，两者对 `updated_at` 的清理幂等。
  `fork.1` 作为已发布迁移保持不变；其测试 helper 改为 `fork` 前缀，避免与
  `v1.0.0-beta9_test.go` 重名（`7fdbd2ae`）。
- 测试：
  `internal/ent/migrate/datamigrate/v1.0.0-beta7-fork.1_test.go`。

### 上游模型目录 schema 修复回补

- 问题：PublicProviderConf 的 DeepSeek 模型把 `experimental` 字段改为裸布尔值，
  `providersDataSchema.parse()` 抛错，`useProvidersData` 与 `useDevelopersData` 均静默回退到
  内嵌的 `providers.json` 快照，导致模型页预设和渠道模型价格识别都停留在该快照。
- 状态：已随 `v1.0.0-beta10` 合入上游版本（`7fdbd2ae`），内容与上游一致，无 fork 差异。
- 上游来源：`6f729f7c`（PR #2305），原样 cherry-pick，无 fork 适配。
- 提交：`8c978c6b`。
- 代码：`frontend/src/features/models/data/providers.schema.ts`。
- 迁移与测试：无迁移；无专用测试。
