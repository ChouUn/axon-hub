# 上游同步记录：v1.0.0-beta11（2026-11-14）

> Parent: [../index.md](../index.md)
> Audience: developer — 开发和演进项目的人或 agent
> Genre: survey — 当时基于什么信息做了什么判断
> Visibility: public — 他人可读，避免写入本机环境
> Maintain: frozen 2026-11-14 — 已定稿；内容与定稿日期不改，内容变化只追加 Outdated / Superseded 段，元数据可维护，定位用的路径与链接可随文件移动更新到原对象与版本的新位置

## 结论

- 上游 `v1.0.0-beta10` → `v1.0.0-beta11` 的 138 个提交以 merge 合入，merge 提交为 `eaef7c06`。
- 逐项核对 20 项 fork 改动：19 项保留为 fork 差异，1 项合入后与上游一致。
- 上游用配额路由取代了配额强制。本 fork 整体采用，健康门控策略接入配额路由。
- 健康门控的行为随上游的配额、重试、流终态与错误分类改动而变化，见「合入后 fork 功能的行为变化」。
- 发现 5 项问题：2 项由合入引入，3 项合入前已存在。5 项均已修。
- 未运行 lint 与仓库 e2e 套件。

## 合入

- 冲突：43 个文件。
  - 生成物不手工合并，按解决后的源文件重新生成：
    - `internal/ent/` 下的生成代码。
    - `internal/server/gql/generated.go`、`models_gen.go`。
    - `frontend/src/routeTree.gen.ts`。
  - 两侧在同一结构里各加字段的文件取并集：
    - `internal/objects/channel.go`、`internal/objects/model.go`
    - `internal/server/gql/axonhub.graphql`
    - `frontend/src/features/channels/data/schema.ts`、`frontend/src/features/channels/utils/merge.ts`
    - `frontend/src/locales/` 下的 `channels.json`、`models.json`
  - 其余按「逐项核对」的结果手工拼接。
- 不在冲突清单但两侧在同一位置各自新增的文件：
  - `internal/server/orchestrator/state.go`：两侧都新增了 `RawStreamBacklog` 字段，保留 fork 的一份。
  - `frontend/src/features/channels/data/config_channels.ts`：为上游新增的三个渠道类型补供应商映射。
- schema migration：上游的变化都是新增可空列、带默认值的列、索引和枚举值，由 ent 托管，无冲突。
  - `requests`：`user_id`、`user_agent`、`response_headers`，索引 `requests_by_project_id_user_id_created_at`。
  - `request_executions`：`channel_api_key_suffix`、`channel_api_key_index`、`upstream_model_id`、`response_headers`。
  - `channels`：`auto_disable_expires_at`，类型枚举新增 `bailian_responses`、`zenmux_video`、`typesafe`。
  - fork 的 `requests.routing_decision` 保留。
- data migration：
  - 上游无新增 data migration。
  - 注册顺序不变：`v1.0.0-beta7` → `v1.0.0-beta7-fork.1` → `v1.0.0-beta8` → `v1.0.0-beta9`。
  - 版本比较改用上游实现，见「迁移器版本比较」。
- 配额设置迁移：上游在启动时把旧的配额强制设置一次性转成配额路由设置
  （`internal/server/biz/quota_routing_migration.go`）。
  - 由系统键 `quota_routing_migration_done` 标记是否已执行，不经过 data migration 的版本比较。
  - 旧设置到默认模式的对应：

    | 旧设置 | 默认模式 |
    | --- | --- |
    | 未保存过 | `remove_on_exhausted`（耗尽时移除） |
    | 已关闭 | `ignore_quota`（忽略额度） |
    | 开启，`exhausted_only` | `remove_on_exhausted` |
    | 开启，`de_prioritize` | `backpressure`（背压） |

  - 旧设置里豁免配额强制的渠道，渠道级模式设为 `ignore_quota`。

## 上游带来的行为变化

- 配额路由取代配额强制（`7ca221d8`，PR #2448）：系统默认模式加渠道级覆盖，耗尽的渠道在选路前被移除。
- 粘性渠道按重试策略做同渠道重试（`40636bbd`，PR #2469）。此前粘性渠道失败后直接换渠道。
- 流终态统一（`2f8de312`、`3786f2c5`、`ac050200`）：
  - 流式请求的成功收紧为全部 choices 收尾且终态为完成。
  - 渠道上游报告的失败、取消、未完成终态在协议转换后仍保留。
- 自动禁用设置由状态码列表改为规则列表（`ad5b5eec`，PR #2453），旧值在读取时归一化。
- `WeightRoundRobin` 的评分公式改变（`5653bf0a`，PR #2518）。
- 请求列表不再查询总数（`c6f2bfcf`，PR #2446）。
- 新增配置 `server.trusted_proxies`（`f4fffa76`，PR #2424），默认为空。
- 新增渠道类型 `bailian_responses`（`96badcb3`）、`zenmux_video`（`e6fdf354`）、`typesafe`（`46f78a02`）。
- `/admin/graphql` 移到独立路由组，service account 类型的 API Key 在其中只读。

## 逐项核对

| # | 改动 | 结果 | 状态 |
| --- | --- | --- | --- |
| 1 | [健康门控](../routing/feats/health-gate.md)：选路门控 | 接入上游配额路由。失败分类改用上游的传输错误判定。行为变化见下节 | 保留 |
| 2 | [健康门控](../routing/feats/health-gate.md)：会话归属与决策记录 | 归属渠道随上游按重试策略做同渠道重试。背压以会话归属为准 | 保留 |
| 3 | [模型准入](../routing/feats/model-admission.md) | 上游改了 `select_candidates.go` 的配额入口，已删除的三项模型设置未被带回 | 保留 |
| 4 | [渠道列表](../channels/feats/channel-list.md)：渠道 tab 按供应商聚合 | 新渠道类型补映射：`bailian_responses` 归 bailian，`zenmux_video` 归 zenmux，`typesafe` 独立成组 | 保留 |
| 5 | [渠道列表](../channels/feats/channel-list.md)：生图渠道拆分 | 上游重构了渠道测试，生图判定在其上重接 | 保留 |
| 6 | [渠道列表](../channels/feats/channel-list.md)：排序字段为零值时分页错乱 | 上游未升级 ent/contrib，绕过仍需要 | 保留 |
| 7 | [渠道测试](../channels/feats/channel-test.md) | 独立超时与按字段跳过事务保留。上游按操作名跳过的 `bulkImportChannels` 并入字段列表 | 保留 |
| 8 | [强制 1 小时提示缓存](../channels/feats/force-one-hour-prompt-cache.md) | 上游在请求列表模型列的同一位置加了上游模型审计图标。顺序为出站协议、透传、1 小时缓存、上游模型审计 | 保留 |
| 9 | [供应商模型目录](../channels/feats/model-catalog.md)：Provider 模型成本可为空 | 上游未改 schema，仍为 fork 差异 | 保留 |
| 10 | [透传](../channels/feats/passthrough.md)：预读队列循环等待 | 上游 `02269f12`（PR #2580）修了同一问题，上限更小。保留 fork 的实现与上限 | 保留 |
| 11 | [透传](../channels/feats/passthrough.md)：渠道上游的错误体被重包装 | 上游未处理，仍为 fork 差异 | 保留 |
| 12 | [仪表盘分析](../usage/feats/analytics.md)：API 密钥费用分析 | 上游给筛选列加了表名限定，fork 的模板筛选在其上保留 | 保留 |
| 13 | [仪表盘分析](../usage/feats/analytics.md)：模型详情分析 | 上游给维度统计加了首 token 时间与输出速度两列，与 fork 的统计并存 | 保留 |
| 14 | [自助用量页](../usage/feats/self-usage.md) | 上游移动了 `/admin/graphql` 的路由组，自助入口并存 | 保留 |
| 15 | [渠道定价](../billing/feats/channel-pricing.md)：渠道标准价格引用与持久倍率 | 模型同步取上游的事务内重读，加 fork 的乐观锁与价格补缺。倍率加载钩子与上游的只读钩子并存 | 保留 |
| 16 | [渠道定价](../billing/feats/channel-pricing.md)：上游模型价格时间戳修复回补 | data migration `v1.0.0-beta7-fork.1` 的注册位置不变 | 保留 |
| 17 | [模型定价阶梯](../billing/feats/model-pricing-tiers.md) | `ModelCard` 保留 fork 的价格字段与自定义 JSON，加上游的 `ReasoningEfforts` | 保留 |
| 18 | [请求成本悬浮](../billing/feats/request-cost-breakdown.md) | 上游改了请求列表的列，fork 的共享单元格保留 | 保留 |
| 19 | [Docker 镜像发布到 fork 仓库](../deployment/feats/docker-publish.md) | 上游未改 `docker-publish.yml` | 保留 |
| 20 | 迁移器版本比较 | 上游 `4910da2c` 自行修复，采用上游实现 | 无差异 |

### 迁移器版本比较

- 问题：迁移器（`internal/ent/migrate/datamigrate`）在启动时比较 `system_version` 与各 data migration 的版本。
  原生 semver 把预发布标识 `beta10` 按字符串比较，得到 `v1.0.0-beta10 < v1.0.0-beta9`。
  `system_version` 为 `v1.0.0-beta9` 的实例升级后版本抬不上去，之后的 data migration 被跳过。
- fork 在 `6c07e14e` 用 `parseComparableVersion` 修正，把预发布标识里的字母词和数字拆开比较。
- 上游 `4910da2c`（PR #2431，随上游 `v1.0.0-beta11` 发布）在 `biz.CompareVersions` 做了同样的拆分，
  迁移器与更新检查共用。
- 合入后迁移器调用 `biz.CompareVersions`，fork 的 `version.go` 与 `version_test.go` 删除。
- 两个实现对已有版本给出相同顺序。上游实现还能正确排序 `v1.0.0-rc.3-fork.1` 这类带点号计数的 fork 版本。
- 保留的 fork 测试：
  - `internal/server/biz/version_test.go` 的 `TestCompareVersionsOrdersNumericPrereleases`：
    `v0.4.0` 到 `v1.0.0` 的完整顺序，含 `-fork.N` 与 `rc.N` 形式。
  - `internal/ent/migrate/datamigrate/migrator_test.go` 的 `TestMigrator_Run_NumericPrereleaseOrdering`：
    构建版本为 `v1.0.0-beta10-fork.1` 时的执行与跳过。

## 合入后 fork 功能的行为变化

下表是合入后可观察到变化的 fork 功能行为，各项都是跟随上游改动的结果。

| # | 行为 | 合入前 | 合入后 | 原因 |
| --- | --- | --- | --- | --- |
| 1 | 健康门控下额度耗尽的渠道 | 由配额强制处理：开启且 `exhausted_only` 时移除，`de_prioritize` 时排到最后，关闭时不处理 | 由配额路由处理，先过滤再做门控判定。过滤后无候选时返回额度耗尽错误，不进入最后一试 | 上游删除了配额强制 |
| 2 | 健康门控下归属渠道的同渠道重试 | 零次，与其他策略的粘性渠道相同 | 按重试策略重试，与其他策略的粘性渠道相同 | 上游 `40636bbd` 取消了粘性渠道的零次重试 |
| 3 | 健康门控的失败分类 | 错误链中含 EOF 即按网络错误计数失败 | `httpclient.Error` 与 `llm.ResponseError` 类型的错误按状态码分类，不因错误链中含 EOF 计数失败 | 上游删除了原判定函数，改用上游的 `IsUpstreamTransportError` |
| 4 | 健康门控的流式结果 | 只看入站协议的结束事件。渠道上游的未完成、失败、取消终态经协议转换成普通结束事件后计数成功，并写入会话归属 | 渠道上游报告完成或未完成（输出上限、内容过滤）计数成功，报告取消不计入，报告失败按流中断处理。会话归属只在生成完成时写入 | 上游的流终态统一 |

## 发现

| # | 发现 | 来源 | 状态 |
| --- | --- | --- | --- |
| 1 | 健康门控把渠道上游的未完成终态记为不稳或失败 | 合入引入，代码审查发现 | 已修 |
| 2 | 透传生效时，流式响应开始输出后的中断不记入健康状态 | 合入前已存在，真机验收发现 | 已修 |
| 3 | `channels-model-price-dialog.tsx` 有 1 个类型错误 | 合入前已存在，`tsc` 对比发现 | 已修 |
| 4 | 合入时为 `health-gated` 的归属渠道单独保留了零次同渠道重试 | 合入引入，复核合入决定时发现 | 已修 |
| 5 | 透传生效且流的关闭晚于请求结束时，失败终态不计入健康状态，已完成的请求不写会话归属 | 合入前已存在，复核发现 2 的修复时发现 | 已修 |

### 1. 未完成终态被记为不稳或失败

- 触发：`health-gated` 策略下，Responses 协议的渠道因输出上限或内容过滤返回 `response.incomplete`，
  客户端用其他协议接入。
- 原因：上游的流终态统一后，`StreamCompleted` 只表示生成成功，未完成终态为否。
  健康门控只读这个字段，于是把合法的未完成终态记为不稳，没有输出时记为计数失败。
  合入前这种终态经协议转换成普通结束事件，记为成功。
- 影响：正常触及输出上限的渠道显示不稳，连续失败不清零，试探成功数不增加。
- 修复：在 merge 提交 `eaef7c06` 内。健康门控同时读取渠道上游的终态（`OutboundStreamTerminal`），
  未完成计数成功，取消不计入。
  上游对渠道性能统计用的是同一口径（`internal/server/orchestrator/outbound.go` 的 `markPerformanceTerminal`）。
- 会话归属：上游把未完成的流式请求记为失败，归属只在请求成功时写入。
  merge 提交按健康结果写入了归属，`3e40e21a` 改为只在生成完成时写入。
- 测试：`health_gate_session_test.go` 的
  `TestHealthGateSession_ProviderTerminalWithoutCompletionIsNotChannelFault`，
  覆盖健康结果与归属、保护窗口、转移计数不变。

### 2. 透传时流中断不记入健康状态

- 触发：请求体透传生效（渠道或全局开启，且入站与出站协议相同）的流式请求，
  在响应开始输出后被渠道上游中断。
- 现象：健康状态保持健康，最近错误为空。[健康门控](../routing/feats/health-gate.md)的失败口径要求记为不稳。
- 原因：透传时转换管线一侧的流由后台协程排空并关闭。
  HTTP 处理函数写完原始流即返回，请求 context 随之取消。
  健康门控在流关闭时看到 context 已取消，按客户端取消记为不计入。
- 证据：
  - 基线镜像 `v1.0.0-beta10-fork.7` 与合入后的镜像表现一致，SQLite 与 PostgreSQL 一致。
  - 关闭全局透传后同一场景显示不稳，最近错误为 `unexpected EOF`。
- 完成的流的健康结果不受影响：健康门控先判断生成是否成功。
- 修复：`8c8b9a6b`。流上带有上游传输错误时不按客户端取消处理。
  真正的客户端断开以 context 错误结束，仍不计入。
  同一根因的其余路径见发现 5。
- 测试：`health_gate_context_test.go` 的
  `TestHealthGateStream_CanceledContextPreservesUpstreamFailure` 与
  `TestHealthGateStream_PassThroughHTTPClientCancellationIsNeutral`。

### 3. 渠道价格对话框的类型错误

- 位置：`frontend/src/features/channels/components/channels-model-price-dialog.tsx` 里把内层价格校验的
  issue 转给外层表单的一行，`ctx.addIssue(issue)` 报 `TS2345`。
- 该行由 fork 提交 `2eca7c59` 加入，本次合入未改动。
- zod 版本在上游 `v1.0.0-beta10` 到 `v1.0.0-beta11` 之间未变，推断这个错误在合入前已存在。
  `2eca7c59` 晚于上一次 `tsc` 对比（见[上游 v1.0.0-beta10 同步记录](upstream-sync-v1.0.0-beta10.md)），
  其间没有做过对比。
- 修复：`0af05bef`。内层价格校验的 issue 不再原样传给外层，改为按 `message` 与 `path` 重新构造。
  表单只读取这两项，提示文案与定位不变。

### 4. merge 提交为归属渠道保留了零次重试

- 上游 `40636bbd` 取消了粘性渠道不做同渠道重试的例外。
  merge 提交 `eaef7c06` 为 `health-gated` 的归属渠道单独保留了它。
- 这条例外由上游提交 `5438dff5` 引入，本 fork 没有改过它。
  合入前[健康门控](../routing/feats/health-gate.md)的规则写的是「沿用粘性候选零次同渠道重试」。
- 修复：`989da875` 删除该特例，归属渠道按重试策略重试。
  最后一试与被健康门控跳过的候选仍不重试。
- 测试：`outbound_test.go` 的子测试 `health-gated owner follows normal same-channel retry rules`。
- 其余保留 fork 实现的决定按同一依据复核，即该行为是否由 fork 改过：
  - 透传预读暂存：fork 提交 `cbea376e`。
  - 渠道模型同步的乐观锁与价格补缺：fork 提交 `2eca7c59`。

### 5. 透传时流的关闭晚于请求结束

- 根因与发现 2 相同：透传时转换管线一侧的流由后台协程关闭。
  关闭时请求 context 可能已因 HTTP 处理函数返回而取消，健康门控把它当成客户端取消。
  `8c8b9a6b` 只处理了流上带传输错误的情形。
- 失败终态：渠道上游在流内报告失败（如 Responses 协议的 `response.failed`）且连接正常结束时，
  流上没有传输错误，结果记为不计入。
  - [健康门控](../routing/feats/health-gate.md)的规则是有输出记不稳，无输出计数失败。
  - 合入前这种终态经协议转换成普通结束事件，计数成功。
- 会话归属：已完成的流式请求在关闭时被当成客户端取消，归属不写入、不续期。
  - 这个判断自 fork 提交 `a4ffb86d` 起就有，合入没有改变它。
- 证据：回归测试在修复前失败。
  - 失败终态：关闭前取消 context，有输出与无输出两种情形下健康状态都保持健康。
  - 会话归属：真实 HTTP 透传请求，客户端完整收到结束事件。
    用关闭屏障让后台关闭晚于处理函数返回，归属未建立。
- 局限：后台关闭与处理函数返回的先后取决于运行时序，修复前两条路径在实际部署中出现的比例未测量。
- 修复：`bfe76e5f`。渠道上游报告的失败终态不按客户端取消处理，会话归属以生成完成为准。
  口径与上游一致：渠道上游已给出的终态优先于之后观察到的 context 错误
  （`internal/server/orchestrator/stream_outcome.go`）。
- 测试：`health_gate_context_test.go` 的
  `TestHealthGateStream_CanceledContextPreservesFailedTerminal` 与
  `TestHealthGateStream_PassThroughHTTPCompletionCommitsOwnerAfterHandlerReturn`。

## 验证

- Go 编译与测试在 Docker（`golang:1.27.1`）内运行：
  - `go build -tags=nomsgpack ./...` 通过。
  - 主模块 `go test -tags=nomsgpack ./...`：40 个包通过。
  - `llm` 模块 `go test ./...`：42 个包通过。
  - PostgreSQL 专用测试（需 `AXONHUB_TEST_PG_DSN`）通过：配额路由迁移、分析性能查询、gc。
  - merge 之后的四个修复提交（`3e40e21a`、`8c8b9a6b`、`989da875`、`bfe76e5f`）完成后重跑主模块
    `go test -tags=nomsgpack ./...`，40 个包通过。
- 为适配合入结果改了 5 处测试的期望，产品代码不变：
  - `bailian_responses_test.go`：上游测试用了未注册的模型，按模型准入补注册。
  - `tester_formats_test.go`：生图用例不断言 `response_format`，不给生图渠道配强制流式。
  - `health_gate_load_balancer_weight_test.go`：同权重同分的候选不断言先后。
  - `health_gate_session_test.go`：背压用例的前置断言改为与旧粘性缓存的实际值一致。
  - `channels-columns-quota.test.mjs`：补 fork 在模块顶层用到的桩。
- 前端：
  - 单元测试 267 个通过，`0af05bef` 之后重跑结果相同。上游 `v1.0.0-beta11` 干净树为 239 个。
  - 应用工程的类型检查（`tsc --noEmit -p tsconfig.app.json`）：上游干净树 190 个错误，合入后 173 个。
    合入后独有 1 个，见发现 3。`0af05bef` 之后为 172 个，该文件无错误。
    根 `tsconfig.json` 不含源码，不带 `-p` 运行不检查应用代码。
- 真机验收：本机 Docker，从 `chouun/axon-hub:v1.0.0-beta10-fork.7` 在同一数据卷上升级到 `bfe76e5f` 构建的镜像，
  SQLite 与 PostgreSQL 各一遍，渠道上游用本地 mock。
  - 结果：两种方言升级后 19 项全部通过。
  - merge 提交 `eaef7c06` 构建的镜像为 18 项通过。未通过的一项是发现 2，升级前的基线镜像同样未通过。
  - 核对内容：
    - 启动日志无迁移错误，`system_version` 抬到新版本。
    - 配额设置迁移完成，旧的「开启、`exhausted_only`」迁成 `remove_on_exhausted`。
    - `requests.routing_decision` 的列与已有数据保留，上游新增列存在。
    - fork 的渠道设置（倍率、强制 1 小时缓存、健康门控阈值、生图标记）保留。
    - 健康门控熔断坏组合后不再命中。
    - 额度耗尽的渠道在 `health-gated` 下零命中，渠道设为忽略额度后命中。
    - 透传保留客户端字段。
    - 生图渠道的测试请求走图像生成端点。
    - 两张分析表、决策记录、成本倍率、自助用量页可查询。
    - 渠道支持但未注册的模型返回 422。
- 管理端界面：在升级后的 PostgreSQL 实例（merge 提交 `eaef7c06` 构建的镜像）上逐页打开，无控制台错误。
  四个路由修复提交之后没有重看这些页面，它们不涉及前端。
  - 渠道列表的 tab 分组、健康门控弹窗、模型价格窗口、渠道编辑窗的配额路由模式。
  - 系统设置的健康门控面板与配额路由设置。
  - 请求列表、两张分析表、自助用量页。
- 模型价格窗口在 `0af05bef` 构建的镜像（SQLite 实例）上重看，无控制台错误：
  - 不改任何值直接保存，没有校验提示，保存成功。
  - 清空一个价格项后保存被拦住，提示显示在该输入框下方。

## 未覆盖范围

- 未运行 lint 与仓库 e2e 套件。
- 未重跑 `selfusage` 与 `openapi` 两个 GraphQL 包的代码生成，两者的 schema 未变，编译与测试通过。
- 未重新生成 `config.schema.json`。
- `go vet` 报告 `internal/server/api/stream_lifecycle_test.go:179` 一处，文件为上游新增，未改。
- 真机验收未覆盖上游新增的三种渠道类型、自动禁用规则与 `server.trusted_proxies`。
- 真机验收未覆盖透传生效时渠道上游的失败终态与会话归属的写入，这两条只由发现 5 的回归测试覆盖。
- 代码审查未逐行覆盖全部生成代码、各供应商转换器的上游内部改动和全部 locale 键。
