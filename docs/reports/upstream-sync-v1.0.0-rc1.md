# 上游同步记录：v1.0.0-rc1（2026-11-15）

> Parent: [../index.md](../index.md)
> Audience: developer — 开发和演进项目的人或 agent
> Genre: survey — 当时基于什么信息做了什么判断
> Visibility: public — 他人可读，避免写入本机环境
> Maintain: frozen 2026-11-15 — 已定稿；内容与定稿日期不改，内容变化只追加 Outdated / Superseded 段，元数据可维护，定位用的路径与链接可随文件移动更新到原对象与版本的新位置

## 结论

- 上游 `v1.0.0-beta12` → `v1.0.0-rc1` 的 81 个提交以 merge 合入，merge 提交为 `c0ce0483`。
- 逐项核对 22 项 fork 改动：
  - 21 项保留为 fork 差异。
  - beta12 加的透传关闭等待（第 22 项）因上游去掉后台转换协程而失去作用，用户决定删除。
- 上游新增的插件系统让 fork 在 GraphQL 中的两处实现失效：插件入口没有成本加载器，fork 的固定读取在插件主体下被拒绝，见发现 1、2。
- fork 功能其余可观察的变化见「合入后 fork 功能的行为变化」。
- 发现 3 项问题，都由合入引入，都已修。
- [上游 v1.0.0-beta12 同步记录](upstream-sync-v1.0.0-beta12.md)的发现 6（用户决定不修的性能记录数据竞争）
  随上游去掉后台转换协程而消除，重复 30 次的 `-race` 检查未复现。

## 合入

- 冲突：25 个文件。
  - 生成物不手工合并，按解决后的源文件重新生成：
    - `internal/ent/` 下的生成代码。
    - `internal/server/gql/generated.go`，以及 `openapi`、`selfusage` 两个 GraphQL 包的生成代码。
    - `frontend/pnpm-lock.yaml`：`package.json` 取上游，只加回 fork 的 `decimal.js-light`，再用 pnpm 重新生成。
    - `frontend/src/routeTree.gen.ts`。
  - `.github/workflows/docker-publish.yml` 保留 fork 的整份文件，见「逐项核对」第 19 项。
  - 渠道价格对话框保留 fork 的整份文件，见第 15 项。
  - 编排器的冲突以上游的新结构为底，再接回 fork 的改动，见第 1、2、7、10 项。
  - 其余按「逐项核对」的结果手工拼接。
- schema migration：
  - 新增 `plugins`、`plugin_storages` 两张表。
  - `traces` 新增 `last_request_at` 列与索引 `traces_by_project_id_last_request_at`。
- data migration：
  - 上游无新增 data migration。
  - 注册顺序不变：`v1.0.0-beta7` → `v1.0.0-beta7-fork.1` → `v1.0.0-beta8` → `v1.0.0-beta9`。
  - fork 镜像版本为 `v1.0.0-rc1-fork.1`。按 `biz.CompareVersions`，它高于 `v1.0.0-beta12-fork.1`，低于上游之后的 `v1.0.0-rc2`、`v1.0.0-rc.3`、`v1.0.0-rc.4`。

## 上游带来的行为变化

- 插件系统：
  - 插件管理与沙箱页面（`ef983306`，PR #2903），插件市场与请求级、尝试级的请求钩子（`dbdb38c8`，PR #2926）。
  - 插件请求以专用的 Plugin 主体运行（`6c5cfe6e`，PR #2917），网关事件投递给 JS 插件（`a7bea8b2`，PR #2912）。
  - 插件的对外 HTTP 请求只能访问所有者授予的网络规则（`a44a44b2`，PR #2923）。
  - 市场索引地址写死为 GitHub raw 上的 `plugins/registry.json`，不可配置。
  - 新增 `plugins`、`plugin_storages` 两张表。
- 编排器：
  - 流式透传改走转换链（`4af420ca`，PR #2951）：不再有后台转换协程，读取渠道上游与发给客户端在同一个协程里进行。
  - 流的尾部在 `OutboundPersistentStream.Close` 中同步收集（`3be08c28`，PR #2952）。
  - 请求与执行的持久化合并到终态写入（`16329952`，PR #2954）：`requests.channel_id` 不再在选路时落库。
    请求详情在请求进行中显示最近一次执行的渠道。
  - 系统设置在请求开始时取一次快照（`4eebba5f`，PR #2937），负载均衡对每个候选只取一次渠道指标（`e115a216`，PR #2938）。
  - 没有 `Retry-After` 的 429 默认冷却 20 秒（`984394c0`，PR #2609）。
  - 402、520 不再做同渠道重试（`984394c0`）。
- 用量：
  - OpenAI 流在全部输出之后、只带结束原因的结束块之前发出用量，且所有 choice 都正常结束时，这份用量也会记录（`e2759aad`，PR #2705）。
    用量之后还有输出时仍不记录，因为它没算进之后的输出。
  - 终态用量按真实计数判定，不看事件形状（`0fa12b22`，PR #2696）。
  - 用量中的 `cost` 可以是对象（`fd9357e5`，PR #2877）。
- traces 按最后一次请求的时间排序（`1bc54370`，PR #2932）：`traces` 新增 `last_request_at` 列与索引，已有行取 `1970-01-02 00:00:00`。
- 模型目录按需加载，缓存 5 分钟，去掉手动刷新，目录地址的设置移到模型设置对话框；另加可选的 pprof 监听，默认关闭（`7d3a5191`，PR #2936）。
- 管理端：
  - 导航与设置页重组，系统设置拆成 `/system/<分区>`（`7f961ddb`，PR #2919）。
  - 对话框页脚的布局与按钮触控高度统一（`38dac947`，PR #2925）。
  - 模型关联预览显示优先级与权重（`a096e373`，PR #2857）。
  - 渠道价格对话框从模型库填价时乘汇率，默认 7.14（`03ffac6f`，PR #2662）。
- 覆盖：条件新增匹配函数（`52727fd5`，PR #2901），查询路径命中的每个元素都会被设置（`6e20f52d`，PR #2924）。
- 构建：
  - 根 `go.mod` 的 Go 版本升到 `1.27.3`，`llm/go.mod` 仍为 `1.26.4`。
  - 前端依赖升级大版本：react-i18next 17、react-day-picker 10、framer-motion 14、streamdown 2、shiki 4。

## 逐项核对

| # | 改动 | 结果 | 状态 |
| --- | --- | --- | --- |
| 1 | [健康门控](../routing/feats/health-gate.md)：选路门控 | 上游把系统设置改为请求开始时取一次快照，并改了负载均衡与排序的签名。门控仍按当前设置实时解析配置，改由 fork 专用的状态字段 `HealthGatePolicyProvider` 承载。门控在中间件链中的位置不变，上游的插件尝试钩子排在它之前。上游给无 `Retry-After` 的 429 加了 20 秒渠道冷却，402、520 不再做同渠道重试，健康结果的分类不变 | 保留 |
| 2 | [健康门控](../routing/feats/health-gate.md)：会话归属与决策记录 | 上游取消了选路时写 `requests.channel_id`，改为请求结束时写入。fork 跟随这个时点，`health-gated` 下只改内存中的渠道、不刷新会话粘性 | 保留 |
| 3 | [模型准入](../routing/feats/model-admission.md) | 上游重写的模型设置对话框又引用了 fork 已删除的三项设置（找不到模型时回退到渠道、查询全部渠道模型、模型黑名单正则），合入时去掉 | 保留 |
| 4 | [渠道列表](../channels/feats/channel-list.md)：渠道 tab 按供应商聚合 | 上游未改相关代码 | 保留 |
| 5 | [渠道列表](../channels/feats/channel-list.md)：生图渠道拆分 | 上游去掉了渠道页对模型目录的预取，生图分组不读目录 | 保留 |
| 6 | [渠道列表](../channels/feats/channel-list.md)：排序字段为零值时分页错乱 | 上游仍锁定 ent/contrib `3625dcc2e035`，绕过仍需要 | 保留 |
| 7 | [渠道测试](../channels/feats/channel-test.md) | 上游在三个测试入口加了插件主体拒绝，放在 fork 的独立超时之前。GraphQL 跳过事务仍按字段匹配，并覆盖上游的 `bulkImportChannels` | 保留 |
| 8 | [强制 1 小时提示缓存](../channels/feats/force-one-hour-prompt-cache.md) | 上游未改 Anthropic 的缓存断点处理，出站中间件的顺序不变 | 保留 |
| 9 | [供应商模型目录](../channels/feats/model-catalog.md)：Provider 模型成本可为空 | 上游把目录改为按需加载，内置目录已无空成本，可空的 schema 保留 | 保留 |
| 10 | [透传](../channels/feats/passthrough.md)：预读事件上限 | 上游 `4af420ca` 重写透传，去掉了队列与后台协程。以上游实现为底，保留 4096 个事件的上限与超限后不交出已暂存的事件 | 保留 |
| 11 | [透传](../channels/feats/passthrough.md)：渠道上游的错误体被重包装 | 上游未处理，仍为 fork 差异 | 保留 |
| 12 | [仪表盘分析](../usage/feats/analytics.md)：API 密钥费用分析 | 上游把 react-day-picker 升到 10，删掉了日期选择器的 `initialFocus`，见发现 3。页面不在侧边栏里，导航模块按未列出的页面处理，与上游的 `/analytics` 相同 | 保留 |
| 13 | [仪表盘分析](../usage/feats/analytics.md)：模型详情分析 | 与第 12 项共用同一个日期筛选组件，随发现 3 一并修复。插件 GraphQL 入口的固定读取见发现 2 | 保留 |
| 14 | [自助用量页](../usage/feats/self-usage.md) | 上游的插件改造改了系统权限提升的语义，只拦插件主体，自助页不受影响。fork 的自助页路由在合并结果中保留 | 保留 |
| 15 | [渠道定价](../billing/feats/channel-pricing.md)：渠道标准价格引用与持久倍率 | 上游给从模型库填价加了汇率，所在的填价代码 fork 已改为引用标准价，不采用 | 保留 |
| 16 | [渠道定价](../billing/feats/channel-pricing.md)：上游模型价格时间戳修复回补 | data migration `v1.0.0-beta7-fork.1` 的注册位置不变 | 保留 |
| 17 | [模型定价阶梯](../billing/feats/model-pricing-tiers.md) | 上游把模型目录改为按需加载，价格结构未变 | 保留 |
| 18 | [请求成本悬浮](../billing/feats/request-cost-breakdown.md) | 上游新增的插件 GraphQL 入口没有装载 fork 的成本加载器，fork 读取价格版本用的 system bypass 也被禁止，见发现 1、2 | 保留 |
| 19 | [Docker 镜像发布到 fork 仓库](../deployment/feats/docker-publish.md) | 上游只更新了登录 action 的固定版本。保留 fork 的整份 `docker-publish.yml` | 保留 |
| 20 | [渠道性能记录](../channels/feats/channel-performance.md) | 上游未改流式完成的入队点。上游去掉后台转换协程后，beta12 同步记录发现 6 的竞争不再存在 | 保留 |
| 21 | [渠道列表](../channels/feats/channel-list.md)：行菜单打开弹窗后页面被锁住 | 上游升级依赖后，`@radix-ui/react-dismissable-layer` 只剩 1.2.1 一个版本。行菜单的 `modal={false}` 与可取消的延时清理保留 | 保留 |
| 22 | [透传](../channels/feats/passthrough.md)：关闭时等待后台转换协程（beta12 的 `aae3f464`） | 上游去掉了后台转换协程，收尾在关闭时同步完成，等待失去作用。用户决定删除 | 删除 |

## 合入后 fork 功能的行为变化

下表是合入后可观察到变化的 fork 功能行为。第 1–3 行是上游的新行为作用到 fork 功能上，第 4 行是上游的界面重写，第 5 行是发现 1、2 修复后的结果。

| # | 行为 | 合入前 | 合入后 | 原因 |
| --- | --- | --- | --- | --- |
| 1 | 健康门控下，渠道上游返回不带 `Retry-After` 的 429 | 只影响本请求的故障转移，后续请求仍正常选该渠道 | 该渠道冷却 20 秒，期间在同一优先级组内排在最后。健康结果仍不计入 | 上游 `984394c0` 对所有策略加了默认冷却 |
| 2 | 402、520 之后的重试 | 渠道还有其他上游实际模型时，先在同渠道换模型重试 | 直接转到下一个渠道 | 同第 1 行 |
| 3 | OpenAI 流在全部输出之后、结束块之前发出用量，所有 choice 都正常结束 | 不写用量记录，成本悬浮、分析表与自助用量页都不计入 | 写用量记录并计费 | 上游 `e2759aad` |
| 4 | 模型设置对话框 | 上游 beta12 的布局，去掉 fork 删除的三项开关 | 上游重写的分区布局，仍不显示这三项开关 | 上游 `7f961ddb` |
| 5 | 插件读请求成本倍率、模型详情分析与渠道健康状态 | 上游 beta12 没有插件系统 | 经插件 GraphQL 入口读取，结果与具有同样 scope 的用户相同 | 发现 1、2 |

## 发现

| # | 发现 | 来源 | 状态 |
| --- | --- | --- | --- |
| 1 | 插件 GraphQL 入口查询用量记录的成本倍率时报 loader 缺失 | 合入引入，盘点上游改动时发现 | 已修 |
| 2 | 插件 GraphQL 入口可达的 fork 固定读取被拒绝：历史价格版本的倍率、模型详情分析的渠道标签返回 FORBIDDEN，渠道健康状态读不到渠道当前配置 | 合入引入，修发现 1 时的真实入口测试发现 | 已修 |
| 3 | 两张 fork 分析页共用的日期筛选组件仍用 react-day-picker 10 已删除的 `initialFocus` | 合入引入，类型检查对比发现 | 已修 |

### 1. 插件 GraphQL 入口没有装载成本加载器

- 原因：上游 `ef983306`（PR #2903）新增插件 GraphQL 入口 `newPluginGraphqlHandler`。fork 的成本倍率加载器（[请求成本明细悬浮](../billing/feats/request-cost-breakdown.md)）只装在主入口。
- 证据：
  - 经真实插件入口查询 `usageLogs { costPriceMultiplier }`，修复前报 `usage price loader unavailable`，同一查询走主入口正常。
  - 真机上用修复前的中间镜像（`396d30d0`）查询，两种数据库都返回同样的错误，倍率为 null。
- 修复：`7b689d40`，两个入口用同一个中间件装载。

### 2. fork 的固定读取在插件主体下被拒绝

- 原因：上游 `6c5cfe6e` 让插件请求以 Plugin 主体运行，并禁止它提升为 System。fork 在 GraphQL 字段里用 system bypass 做的固定读取，插件调用时一律被拒。
- 影响，均经真实插件入口测试复现：
  - 引用历史价格版本的用量记录，`costPriceMultiplier` 返回 FORBIDDEN。只装上发现 1 的 loader 后出现。
  - `analyticsChannelTags` 与按标签筛选的 `analyticsModelStats` 返回 FORBIDDEN。
  - 渠道的健康门控阈值改为 0 后，健康详情仍显示旧的熔断数，异常筛选仍返回该渠道。
- 处理：改用上游为已授权调用方提供的 `authz.RunAsHost`，只读取调用方已可见的记录所引用的数据。
  - `analyticsModelStats` 在读取前先校验 `read_dashboard`，与 `analyticsChannelTags` 一致。原来的 scope 判定在读取之后才拒绝，权限集合不变。
  - 健康门控策略的读取去掉外层 bypass：系统设置的读取内部已经用 `RunAsHost`。
- 同因排查：fork 新增的其余 bypass 在自助用量页、会话归属与熔断通知里，插件主体走不到，保留：
  - 自助用量页的路由只接受 user/personal API Key。
  - 会话归属只在推理编排中调用，插件能触发的渠道测试入口先拒绝 Plugin 主体。
  - 熔断通知从后台 context 重建。
- 修复：`9a2036ad`。

### 3. 日期选择器的 `initialFocus`

- 原因：上游把 react-day-picker 升到 10，删掉了 `initialFocus`。fork 两张分析页共用的日期筛选组件
  （`frontend/src/features/analytics/components/api-key-analytics-filter-bar.tsx`）仍在使用。
- 证据：合入后的类型检查比上游干净树多出 `api-key-analytics-filter-bar.tsx` 的两条 TS2322。
- 修复：`396d30d0`，改用 `autoFocus`，打开时仍聚焦已选日期或今天。
- 上游自己的 `price-schedule-editor.tsx`、`models-action-dialog.tsx` 仍有 `initialFocus`，不在本次范围内。

## 验证

- Go 编译与测试在 Docker（`golang:1.27.1`，按 `go.mod` 自动切换到 1.27.3）内运行，除注明外均在最终代码 `9a2036ad` 上：
  - 主模块 `go test -tags=nomsgpack ./...`：46 个包通过。
  - `llm` 模块 `go test ./...`：43 个包通过，在 merge 提交上运行。merge 之后的提交未改 `llm/`。
  - PostgreSQL 专用测试批次（`-run Postgres`，需 `AXONHUB_TEST_PG_DSN`）：biz、gql、gql/qb、gc、db 下的 8 个顶层测试通过，其中 1 个是非 PostgreSQL 方言的回退对照。
  - `-race -run 'HealthGate|PassThrough'` 通过，无数据竞争。
  - 删除透传关闭等待之后，`TestHealthGateOrchestrator_CompletedBeforeDeliveryCancellation` 的 `-race -count=30` 与性能记录用例的 `-race -count=10` 通过。之后的提交未改编排器。
  - `go vet -tags=nomsgpack ./...` 只报上游的 `internal/server/plugin/manager_test.go:908`、`internal/server/api/stream_lifecycle_test.go:179`、`:232`，干净的上游 rc1 树上相同。
  - 发现 1 的修复提交 `7b689d40` 单独检出时，它的回归测试通过，不依赖发现 2 的修复。
- 合入时为适配 fork 改动了上游测试，产品代码不变：
  - `pass_through_budget_test.go`：事件数超限的用例改用 fork 的上限，并断言超限时不返回部分结果、不等到超时、不重试。
  - `plugin_endpoint_test.go`：插件可调用的 mutation 字段快照加入 fork 的 `resetChannelHealthGate`。它要求 `write_channels`（`internal/server/gql/channel_health_gate.go:81`）。
  - fork 自己的 `TestHealthGateStream_PassThroughFailedTerminalAfterClientCancellation` 按上游的单协程结构改写，见[健康门控](../routing/feats/health-gate.md)。
- 前端：
  - 单元测试 463 个，462 个通过，1 个跳过。
  - `pnpm build` 通过。
  - 应用工程的类型检查（`tsc --noEmit -p tsconfig.app.json`）：上游干净树 211 条诊断，最终代码 193 条。
    - 按文件、错误码与完整消息比对，merge 提交比上游多出 8 条：6 条对应上游同一位置的已有错误，消息里展开的类型因 fork 的字段而不同，另 2 条是发现 3。
    - 发现 3 修复后，最终代码没有上游之外的类型错误。
- SQLite 上重复执行 AutoMigrate 不重建 `traces` 表，在 merge 提交上用默认配置检查。
- 真机验收：本机 Docker，用 beta12 同步后的镜像（`system_version` 为 `v1.0.0-beta12-fork.1`）建好数据卷，
  在同一数据卷上升级到最终代码 `9a2036ad` 构建的镜像。SQLite 与 PostgreSQL 各一遍，渠道上游用本地 mock。
  - 结果：两种方言各 31 项通过。
  - 核对内容：
    - 启动日志无迁移错误，`system_version` 抬到 `v1.0.0-rc1-fork.1`，不执行 data migration。
    - `traces` 的 `last_request_at` 列与索引、`plugins`、`plugin_storages` 两张表存在。多次重启后表结构、序列与请求 ID 不变，新请求不复用升级前的请求 ID。
    - 各表行数与升级前一致，`requests.routing_decision` 的历史 JSON 保留，fork 的渠道设置经两种更新接口修改后仍保留。
    - 健康门控：先失败后成功计成功，流式正常完成计成功并写入归属，连续三次 5xx 后熔断，首个 token 后断流记为不稳。
    - 无 `Retry-After` 的 429 不计入健康门控，渠道冷却约 20 秒。
    - 透传预读：恰好 4096 个事件时完整交付，4093 个 ping 一个不少。4097 个时请求失败，客户端看不到失败尝试的内容。
    - 流式请求的 chunks 在存储策略开启时写入，透传与非透传都是。
    - 模型库价格兜底按渠道倍率计费，结束块上的用量按渠道倍率计费。
    - 插件 GraphQL 入口：请求成本与成本分项和管理端一致，读到升级前价格版本的倍率 1.25，`analyticsChannelTags` 与管理端一致。插件未获授 `read_channels` 也能读到这些数据。
    - 共享分析接受秒级范围，两张 fork 分析表有数据，自助用量页只返回本 Key 的用量，渠道支持但未注册的模型仍被拒绝。
  - 检查脚本自身的三处口径在这次改正，产品行为不变：
    - 透传响应里的 SSE 写成 `event:ping`，冒号后没有空格，改为解析事件后按 tag 计数。
    - GraphQL 的 `responseChunks` 依赖同时选中 `stream`（`internal/server/biz/request.go:1638-1651`），上游 beta12 就是这样，查询补上了该字段。
    - SQLite 每次启动都会争用配额路由的迁移标记（`internal/server/biz/quota_routing_migration.go:35-52`），冲突的插入也消耗 `systems` 的自增序号，重启比较排除这一项。
- 管理端界面：在升级后的 PostgreSQL 实例上检查 39 项，全部通过，无控制台错误与未捕获异常。
  - beta12 的全部检查项：渠道列表、渠道编辑、价格窗口、请求列表与详情、成本悬浮与来源标记、两张 fork 分析表、共享分析页、自助用量页、健康门控弹窗、Webhook 事件选项等。
  - rc1 新增：
    - 三个分析页的 react-day-picker 10 日历：点选日期、切换月份、预设范围。
    - 价格对话框没有汇率输入，模型库标准价引用与渠道倍率照常。
    - 模型设置对话框没有 fork 删除的三项开关，模型目录没有手动刷新按钮。
    - 插件管理页、`/system/requests`、`/system/integrations`。
    - 从仪表盘进入两张 fork 分析页时，顶部模块保持在管理端。
  - 渠道行菜单的弹窗（beta12 同步记录的发现 7、8）：最终镜像上 31 轮、beta12 镜像上 19 轮，收尾时页面都没有锁住。
    - 关闭后约 100ms 再点行菜单：价格弹窗 10 轮都正常。模型别名、覆盖、代理的点击落在仍在淡出的弹窗遮罩上，第二个弹窗没有打开。
    - 间隔约 200ms 时，四种弹窗各 3 轮都正常。
    - 遮罩的淡出是上游 dialog 组件的 150ms 退出动画（`frontend/src/index.css:986`）。同一协议在 beta12 镜像上结果相同，不是本次合入引入的，没有处理。

## 未覆盖范围

- 未运行 lint 与仓库 e2e 套件。
- 真机验收的渠道上游都是本地 mock，未连接真实供应商。
- 插件系统只经本地的 fixture 插件验证了 GraphQL 入口的读取。以下未在真机上验证：
  - 从插件市场安装：市场索引地址写死为 GitHub raw 上的 `plugins/registry.json`。
  - JS 插件沙箱、网关事件投递、出站网络规则，以及请求级、尝试级钩子对真实请求的改写。
- 上游的 pprof 监听、覆盖条件的匹配函数未在真机上验证。
- 上游文件里的 `initialFocus`（`price-schedule-editor.tsx`、`models-action-dialog.tsx`）与其余上游类型错误未处理。
- 代码审查未逐行覆盖全部生成代码、各供应商转换器的上游内部改动和全部 locale 键。
