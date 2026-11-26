# 上游同步记录：v1.0.0-rc.5（2026-11-26）

> Parent: [../index.md](../index.md)
> Audience: developer — 开发和演进项目的人或 agent
> Genre: survey — 当时基于什么信息做了什么判断
> Visibility: public — 他人可读，避免写入本机环境
> Maintain: frozen 2026-11-26 — 已定稿；内容与定稿日期不改，内容变化只追加 Outdated / Superseded 段，元数据可维护，定位用的路径与链接可随文件移动更新到原对象与版本的新位置

## 结论

- 上游 `v1.0.0-rc.4` → `v1.0.0-rc.5` 的 71 个提交以 merge 合入（tag 指向 `32e476e1`），基线为 fork 的 `3cb76656`，
  merge 提交为 `5d12065b`。
- 逐项核对 25 项 fork 改动，全部保留为 fork 差异。
- 上游「导入与同步」（`85502c5a`，PR #3067）按四个单价逐项写入模型卡的 `cost`。本 fork 的 `cost` 只是完整价格 `price` 的只读摘要，
  按用户决定保留该功能，改为按模型整体写入 `price`，见「逐项核对」第 17 项。
- 上游「从服务商同步模型价格」（`4af46665`，PR #3038）写入渠道价格时会丢掉 fork 的渠道倍率、全量阶梯，已改为保留，见第 15 项。
- 发现 2 项问题，均已修：上游的 GraphQL `PricingInput.mode` 拒绝自己产生的空模式；端点同步的模型库补项会让带阶梯的渠道价格停止同步，
  修复在提交 `3dbe8df4`。
- schema migration：上游新增 6 张表，`requests` 与 `request_executions` 各新增 1 个字段，由启动期的 ent 自动迁移创建，
  没有版本化 SQL 与 data migration。
- 合入后不能直接降级到 rc.4，见「上游带来的行为变化」的请求体去重。
- 真机升级验收：rc.4 的 PostgreSQL 与 SQLite 数据卷升级到合并结果构建的镜像，两种方言各 41 项通过。

## 合入

- 冲突：27 个文件。
  - 配置与服务器：
    - `conf/conf.go`、`internal/server/config.go`：fork 的 `channel_test_timeout`（180s）与上游的 `llm_stream_request_timeout`（1800s）、
      `llm_stream_idle_timeout`（600s）并存。
    - `internal/server/server.go`：`WriteTimeout` 取上游 `writeTimeout()` 与 `ChannelTestTimeout` 的较大者。
    - `internal/server/routes.go`：fork 的 `/self-service` 路由组与上游的插件服务中间件 `serverPluginMiddleware` 并存。
  - GraphQL：
    - `internal/server/gql/graphql.go`：上游把事务跳过集中到 `skipGraphqlTransaction`，fork 的三个渠道测试字段并入其中，
      fork 的 `usagePriceLoaderMiddleware` 仍挂在管理端与插件两个处理器上。
      fork 的 `graphql_skip_tx_test.go` 删去，用例并入上游的 `graphql_transaction_skip_test.go`。
    - `internal/server/gql/generated.go`、`internal/ent/internal/schema.go`：生成物，按合并后的 schema 重新生成。
  - 选路与透传：
    - `internal/server/orchestrator/candidates.go`：保留 fork 删去的 `selectChannelCadidates`（[模型准入](../routing/feats/model-admission.md)不回退到渠道）。
    - `internal/server/orchestrator/pass_through.go`：fork 的档位后缀跳过透传请求体（`currentEntryUsesEffortSuffix`）先于上游 #3052 的推理强度映射。
  - 价格：
    - `internal/objects/model.go`、`internal/server/biz/model.go`、`internal/server/api/openai.go`、`openai_model_test.go`：
      按第 17 项重写，保留 fork 的模型准入（别名展开、`ConfiguredModel`）。
    - `internal/server/biz/channel_price_test.go`：保留 fork 的导入。
    - `frontend/src/components/model-price-editor.tsx`：fork 的 `itemsPath`、实际价格展示与上游的 `data-testid`、空模式并存。
    - `frontend/src/features/channels/components/channels-model-price-dialog.tsx`：保留 fork 的整份文件，不采用上游 #3060 的默认汇率。
  - 渠道前端：
    - `frontend/src/features/channels/data/channels.ts`、`schema.ts`、`utils/merge.ts`：两侧字段都保留，
      fork 的 `reasoningEffortModelSuffix`、`healthGateFailureThreshold` 与上游的 `syncModelPrices`、`scheduledDisable`、
      `quotaDisabled`、`forceArrayStop` 等并存。
    - `frontend/src/features/channels/components/channels-model-mapping-dialog.tsx`：取上游文件，重新接入 fork 的「识别档位后缀」卡片。
  - 模型前端：`models-action-dialog.tsx`、`models-table.tsx`、`models/data/models.ts`、`models/data/schema.ts`，按第 17 项处理。
  - 其他：
    - `frontend/package.json`：fork 的 `decimal.js-light` 与上游升级后的 `driver.js`。
    - `frontend/src/locales/{en,zh-CN}/base.json`：保留 fork 一侧。上游新增的汇率文案只被不采用的 #3060 对话框使用。
- 两侧都改过、Git 自动合并的 55 个文件中，与 fork 功能相交的处理：
  - `internal/server/biz/channel_price.go`：见第 15 项。
  - `internal/server/orchestrator/health_gate_selector.go` 不冲突但编译失败：上游 `selectEndpointAPIFormat` 多了 `ctx` 参数（`9f5cef8c`，PR #3078）。
  - `internal/server/orchestrator/orchestrator.go`：上游在 `finalizeTransportRequest` 之后插入 Guardian 关联中间件，
    fork 的强制 1 小时提示缓存仍在透传与 Responses 适配之后、渠道请求体覆盖之前。
  - `internal/server/backup/restore.go`：上游已改用 `PrepareChunkedRequestBody`，fork 的渠道与 API Key 解析保留。
  - `frontend/tests/channels.spec.ts`：上游 #3060 的默认汇率用例随 Git 自动合入，用到的控件在 fork 的对话框中不存在，删去。
- schema migration：
  - 新增表：`plugin_schedules`、`plugin_schedule_runs`、`plugin_table_storages`、`plugin_table_operations`、
    `request_body_chunks`、`request_body_chunk_refs`。
  - `requests` 与 `request_executions` 新增 `request_body_chunked`。
  - `internal/ent/migrate/migrations/` 无新增文件，表与字段由启动期的 `Schema.Create`（`internal/server/db/ent.go`）创建。
- data migration：
  - 上游无新增 data migration，注册顺序不变。
  - fork 镜像版本为 `v1.0.0-rc.5-fork.1`。`internal/server/biz/version_test.go` 的 `TestCompareVersionsOrdersNumericPrereleases`
    补充 `v1.0.0-rc.4-fork.3` < `v1.0.0-rc.5` < `v1.0.0-rc.5-fork.1`。
- `go.mod`：`entgo.io/contrib` 仍锁定 `3625dcc2e035`，[渠道列表](../channels/feats/channel-list.md)的零值分页绕过仍需要。

## 上游带来的行为变化

- 超时（`5e31b7f4`，PR #2905）：
  - rc.4 的 `/v1` 等 LLM 路由与 Responses WebSocket 的单个事件都用 `llm_request_timeout`（600s）。
  - rc.5 起非流式请求仍为 600s，流式请求总时长为 `llm_stream_request_timeout`（1800s），
    渠道上游的流超过 `llm_stream_idle_timeout`（600s）不发数据时终止。
  - `WriteTimeout` 随之取三者最大值，fork 再与渠道测试的上限取较大者。
- 请求体去重（`7af565b5`，PR #2999）：
  - 数据库中较大的请求体按项目拆成内容块，跨请求与重试去重，读取时还原完整 JSON。
  - 上游 CHANGELOG 注明：降级前需导出请求日志并恢复到另一个数据库，旧版本读不到拆块存储的请求体。
- URL 前缀（`2a3e6f3a`，PR #3058）：`server.base_path` 让管理端与 API 挂在同一前缀下。
- 渠道：
  - 按 cron 定时禁用与重新启用（`fcd1f0a3`，PR #3050）。
  - 每个 API Key 的额度展示，额度耗尽时禁用该 Key（`328b27eb`，PR #2995）。
  - `forceArrayStop` 转换选项（`53d0f0ef`，PR #3017）。
  - Claude Code OAuth 渠道拉取模型，只有 access token 的凭据不再自动刷新（`e48f2b6d`，PR #3061）。
  - Codex 模型发现改为从 npm 查询客户端版本（`daa5d971`，PR #3045）。
- Codex Guardian 审查请求关联父请求，优先使用父请求的渠道与协议（`9f5cef8c`，PR #3078）。
- 插件：服务端中间件（`2edb4c9a`，PR #3069）、定时任务与动态 SQL（`6d138d10`，PR #3080）、托管表与页面访问（`7dfd509d`，PR #3086）。
- Responses WebSocket 改用 `coder/websocket`（`c140a988`，PR #3091）。
- 权限（`32e476e1`）：
  - API Key 明文只能由直接用户查看。
  - System 不再是 GraphQL 实体（`entgql.SkipAll`）。fork 的前端与自助用量页不查询它。

## 逐项核对

| # | 改动 | 结果 | 状态 |
| --- | --- | --- | --- |
| 1 | [健康门控](../routing/feats/health-gate.md)：选路门控 | 上游 Guardian 审查在配额过滤后固定父渠道。fork 策略只在父渠道通过门控时固定它，熔断时不放行。另接上 `selectEndpointAPIFormat` 的新签名 | 保留 |
| 2 | [健康门控](../routing/feats/health-gate.md)：会话归属与决策记录 | 上游的流事件改为缓存读取（`3b829079`），fork 的流包装重复读取不再产生副作用。Guardian 中间件插在性能记录与健康结果登记之前 | 保留 |
| 3 | [模型准入](../routing/feats/model-admission.md) | 上游的 `ListEnabledModels` 未改。`/v1/models` 的价格按第 17 项改由 `price` 生成，仍取别名的注册目标 | 保留 |
| 4 | [渠道列表](../channels/feats/channel-list.md)：渠道 tab 按供应商聚合 | 上游未改分组逻辑，只在渠道设置追加字段 | 保留 |
| 5 | [渠道列表](../channels/feats/channel-list.md)：生图渠道拆分 | 上游只在 `channels-columns.tsx` 加了一个额度标签 | 保留 |
| 6 | [渠道列表](../channels/feats/channel-list.md)：排序字段为零值时分页错乱 | `entgo.io/contrib` 锁定版本不变，绕过仍需要 | 保留 |
| 7 | [渠道测试](../channels/feats/channel-test.md) | 配置与 `WriteTimeout` 冲突，见「合入」。事务跳过并入上游函数 | 保留 |
| 8 | [强制 1 小时提示缓存](../channels/feats/force-one-hour-prompt-cache.md) | 上游未改缓存断点处理。出站中间件的相对顺序不变，`hooks.attemptStage` 在 rc.4 已存在 | 保留 |
| 9 | [供应商模型目录](../channels/feats/model-catalog.md)：Provider 模型成本可为空 | 上游更新了目录数据（`31a78692`、`f86f7f7d`），未改 `providers.schema.ts` | 保留 |
| 10 | [透传](../channels/feats/passthrough.md)：预读事件上限 | 上游在透传请求体上套用推理强度映射，未改响应事件的预读与 `maxRawPreAttachEvents` | 保留 |
| 11 | [透传](../channels/feats/passthrough.md)：渠道上游的错误体被重包装 | 上游未处理，仍为 fork 差异 | 保留 |
| 12 | [仪表盘分析](../usage/feats/analytics.md)：API 密钥费用分析 | 上游未改 `analytics_helpers.go`。它不读请求体，不受请求体拆块影响 | 保留 |
| 13 | [仪表盘分析](../usage/feats/analytics.md)：模型详情分析 | 上游未改相关代码 | 保留 |
| 14 | [自助用量页](../usage/feats/self-usage.md) | 服务端剥掉 `base_path` 后路由，`/self-service` 无需改动。前端的接口地址与登录页链接改用 `appPath` | 保留 |
| 15 | [渠道定价](../billing/feats/channel-pricing.md)：渠道标准价格引用与持久倍率 | 上游端点同步只保留计费项与时间覆盖。改为保留渠道倍率与全量阶梯，加锁后校验合并结果，无效时跳过或回退到模型库价格。不采用 #3060 的默认汇率 | 保留 |
| 16 | [渠道定价](../billing/feats/channel-pricing.md)：上游模型价格时间戳修复回补 | data migration `v1.0.0-beta7-fork.1` 的注册位置不变 | 保留 |
| 17 | [模型权威定价与全量阶梯](../billing/feats/model-pricing-tiers.md) | 「导入与同步」改为按模型写入完整价格并保存一条来源绑定。去掉上游的逐项来源 `catalogPrices`、`Missing` 与 `editedPrices`，保留 `expectedVersion`。支持上游的空模式，见发现 1 | 保留 |
| 18 | [请求成本悬浮](../billing/feats/request-cost-breakdown.md) | `usagePriceLoaderMiddleware` 保留在上游的事务中间件之前。上游未改 `requests-columns.tsx` | 保留 |
| 19 | [Docker 镜像发布到 fork 仓库](../deployment/feats/docker-publish.md) | 上游未改 `docker-publish.yml` 与 `Dockerfile` | 保留 |
| 20 | [渠道性能记录](../channels/feats/channel-performance.md) | 上游未改 `performance.go` | 保留 |
| 21 | [渠道列表](../channels/feats/channel-list.md)：行菜单打开弹窗后页面被锁住 | 上游未改行菜单的弹窗交互 | 保留 |
| 22 | [模型准入](../routing/feats/model-admission.md)：模型发现的输入模态与推理档位 | 上游未改 `anthropic.go`。`openai.go` 的价格字段按第 17 项改写，模态与档位输出不变 | 保留 |
| 23 | [请求列表的协议提示](../routing/feats/request-protocol-indicator.md) | 渠道前端的 `channels.ts`、`schema.ts` 冲突只涉及新增字段，协议判定代码上游未改 | 保留 |
| 24 | [渠道识别档位后缀](../routing/feats/reasoning-effort-suffix.md) | 上游 #3052 在透传请求体上套用推理强度映射。选中档位后缀模型的尝试先放弃透传请求体，不经映射 | 保留 |
| 25 | [配置 Codex 渠道使用 HTTP/SSE](../manuals/configure-codex-http-sse.md) | 上游改了 Codex 客户端版本的来源，手册不涉及版本号 | 保留 |

## 合入后 fork 功能的行为变化

| # | 行为 | 合入前 | 合入后 | 原因 |
| --- | --- | --- | --- | --- |
| 1 | 健康门控，流式请求在首个 token 前因上游静默而终止 | 由请求总超时（600s）终止，计数失败 | 静默超过 600s 即终止，计数失败；首个 token 后为「仅不稳」 | 上游 `5e31b7f4` |
| 2 | GraphQL mutation 的事务 | 只按字段跳过三个渠道测试 | 另按操作名 `TestChannel`、`TestChannelAPIKeys` 跳过整个 mutation | 上游 `skipGraphqlTransaction` 原有的分支 |
| 3 | 模型页，在已绑定来源的模型上改价 | 无来源绑定 | 改价后取消跟随，别名与来源保留 | 第 17 项 |

## 发现

| # | 发现 | 来源 | 状态 |
| --- | --- | --- | --- |
| 1 | 上游 GraphQL `PricingInput.mode` 拒绝自己产生的空模式 | 上游 rc.5 已有 | 已修 |
| 2 | 端点同步的模型库补项让带阶梯的渠道价格停止同步 | 合入时的适配，审查发现 | 已修 |

### 1. 上游 GraphQL `PricingInput.mode` 拒绝自己产生的空模式

- 现象：上游 #3038 允许写缓存计费项只靠 `five_min`、`one_hour` 变体计价，聚合定价的 `mode` 为空字符串，后端与前端校验都接受。
  但 `PricingInput.mode` 是非空枚举，保存这样的价格时 gqlparser 报 `mode  is not a valid PricingMode`，在校验前就被拒绝。
- 修复：`internal/server/gql/price.graphql` 中 `PricingInput.mode` 改为可空，前端的输入转换统一提交 null，后端按空模式校验。
  输出的 `Pricing.mode` 不改，仍返回空字符串，fork 前端接受它。

### 2. 端点同步的模型库补项让带阶梯的渠道价格停止同步

- 现象：渠道价格带全量阶梯，模型库价格多出渠道没有的计费项（如读缓存）。端点同步把它补进基础档，阶梯里没有，
  违反「阶梯计费项与基础档一致」，整次更新被跳过，端点报的新价格也写不进去。
- 修复：`mergeEndpointChannelModelPrice` 在补项破坏阶梯约束时只叠加端点报价，提交 `3dbe8df4`。
  回归测试 `TestChannelService_EnsureChannelModelPrices_SkipsLibraryGapThatBreaksTiers`。

## 验证

- Go 测试在 Docker（`golang:1.27.1`，按 `go.mod` 自动切换到 1.27.4）内运行，ent、gqlgen、`openapi`、`selfusage` 的代码先重新生成：
  - 主模块 `go test -tags=nomsgpack ./...`：46 个包通过，3482 个顶层测试通过、14 个跳过。
  - `llm` 模块 `go test ./...`：43 个包通过。
  - `cmd/axonhub-desktop` 模块 `go test ./...`：2 个包通过。
  - PostgreSQL 专用测试批次（`-run Postgres`，`AXONHUB_TEST_PG_DSN` 指向临时 PostgreSQL 17 容器）：全部通过。
    上游新增的两个用例需要另一个变量 `AXONHUB_TEST_PAYLOAD_PG_DSN`，且只接受本机地址与专用库名，在容器网络里不满足，未运行。
  - merge 提交 `5d12065b` 的树（不含发现 2 的修复）：依赖 `internal/server/biz` 的 17 个包（含 `biz` 本身，15 个有测试）测试通过。
  - 发现 2 修复后（`3dbe8df4`），`internal/server/biz` 包的测试通过。
- 前端：
  - `pnpm install --frozen-lockfile` 通过。
  - 单元测试 547 个，546 个通过，1 个跳过。
- 镜像：用合并结果（含发现 2 的修复）构建 `axonhub:rc5-local`，构建副本的版本为 `v1.0.0-rc.5-fork.1`。
  `Dockerfile` 内的前端 `pnpm build` 与 Go 构建通过。
- 真机验收：本机 Docker，用 rc.4 同步后升级过的数据卷（`system_version` 为 `v1.0.0-rc.4-fork.1`）复制出新卷，
  在新卷上升级到该镜像。PostgreSQL 与 SQLite 各一遍，渠道上游用本地 mock，`llm_stream_idle_timeout` 设为 3s。
  - 结果：沿用 rc.4 的 32 项（含 DeepSeek 额度状态行）与 rc.5 新增的 9 项，两种方言各 41 项通过。
  - 启动日志无迁移错误，`system_version` 抬到 `v1.0.0-rc.5-fork.1`，不执行 data migration。重启一次后行数与 schema 不变。
  - 6 张新表已创建。存量请求与执行记录的 `request_body_chunked` 为 false（PostgreSQL 281、333 行，SQLite 229、275 行），
    管理端 GraphQL 读回的升级前请求体与库中的 JSON 一致。
  - 请求体去重：发送 80,136 字节的请求体，存为 2 个内容块，管理端读回的完整 JSON 与发送内容一致。
  - 「导入与同步」，预览读取上游的实时目录：
    - 经 GraphQL 以 `FOLLOW` 导入新模型 `claude-haiku-5-5`。写入的 `price` 与来源价格一致，
      含 100,000 词元的阶梯与一小时写缓存（输入价的 2 倍），模型卡的 `catalogPrice` 为跟随。
    - 带 `expectedVersion` 改价后取消跟随。用过期的 `expectedVersion` 再改，返回 `VALIDATION_FAILED`。
    - `/v1/models`：跟随来源时输出四个单价；改为只有输入价后，其余三项省略。
  - 写缓存只靠变体计价的渠道价格（`mode` 提交 null）保存成功，读回 `mode` 为空字符串，按该价格计费的请求费用为 `0.000285`。
    渠道缓存在事务提交后异步刷新（合入前已如此），验收在保存 2 秒后发请求。
  - 流空闲超时：首个 token 前静默，约 3s 后返回 502，健康门控计 1 次失败；首个 token 后静默，约 3s 后截断，
    门控为不稳，失败计数为 0。
- 管理端界面，在升级后的 PostgreSQL 实例上，无控制台错误与未捕获异常：
  - 「导入与同步」展开已入库的模型，显示「当前价格」「来源价格」与可用的「应用方式」选择框。
  - 渠道的「模型定价」对话框显示只靠变体计价的「写入缓存词元」，「5 分钟」「1 小时」分别为 1、2。

## 未覆盖范围

- 未运行 lint、类型检查与仓库 e2e 套件。前端与 Go 的构建只在镜像构建中执行。
- 真机验收的渠道上游都是本地 mock，未连接真实供应商。流空闲超时只用 3s 验证，未验证默认的 600s。
- 管理端界面只检查了 rc.5 改动的两处，rc.4 的 64 项未重跑。
- MySQL 下的端点同步并发：价格行加锁后的读取在 `REPEATABLE READ` 下可能读到快照，与上游相同，未处理。本 fork 部署使用 PostgreSQL。
- 上游的 Guardian 审查、插件定时任务与托管表、URL 前缀、按 cron 禁用渠道未做真机验证，只有单元测试覆盖：
  - Guardian 审查的父渠道固定，fork 的门控另有 `TestHealthGateSelector_GuardianReviewPrefersHealthyParent`。
  - 插件的 4 张新表在真机验收中确认已创建，定时任务与托管表的运行行为未验证。
- 桌面应用只跑了它的模块测试，未打包。
