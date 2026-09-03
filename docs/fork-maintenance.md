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

## Feature Topics

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
- 列顺序：排名、模型/渠道、请求数、Token 数、成功率、缓存命中率、平均 TTFB、
  平均输出速度、费用/Mtok、费用；表格不重复显示内部标题。
- 排序：默认按费用降序。除排名外，各列通过无边框列头按钮在降序、升序和无序间
  切换；进入无序时恢复费用降序。排序同时作用于模型和各自的渠道明细，排名按当前
  顺序重算，模型的展开状态不随排序改变。
- 重试语义：`request_executions` 中每次 `completed` 或 `failed` 执行均视为一次实际请求，
  包括渠道内和渠道间重试；失败执行计入请求数和成功率，费用与 Token 来自成功用量。
- 日期：支持今天、昨天、本周、本月和全部时间；自然日边界使用系统设置中的时区。
- 提交：`7415d058`（`feat(analytics): 新增模型详情分析`）。
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
  `countChannelsByType` 改为按 `(type, 是否生图)` 在内存中计数并返回
  `primaryApiFormat`，供分组函数拆组。
- 明确不做：不移除任何现有 API 格式选项（含 Anthropic Messages），不新增渠道类型，
  不改路由与端点解析。`codex` 虽有图像默认端点，但其对话框隐藏 API 格式选择器，
  未加入可选供应商。
- 依赖：建立在「渠道 tab 按供应商聚合」之上。
- 提交：`d6d17d00`（`feat(channels): 拆分生图渠道`）。
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
- 发布：`git tag -a vX -m "release: vX"` 后 `git push fork release/v1.0.x --follow-tags`。
  fork 仓库需启用 workflow，并配置 repository secrets `DOCKERHUB_USERNAME` /
  `DOCKERHUB_TOKEN`；`release.yml`（goreleaser）在 Actions 页面单独禁用。
- 动机：此前手工 `docker build .` 未写 VERSION，镜像自报仓库内上游值 `v1.0.0-beta8`，
  启动时 `datamigrate` 把 `system_version` 抬到 beta8，此后所有 `v1.0.0-beta7-fork.N`
  数据迁移都会被跳过，且镜像内容不保证等于 tag。
- 存量实例修正：`fork.5` 之前部署过的实例需手工把 `systems.system_version` 回拨为当前
  部署的 fork 版本（migrator 只升不降）。回拨后版本正确的镜像会在每次启动重跑上游
  `v1.0.0-beta8` 数据迁移（幂等，与上游 beta7 正式版行为一致），同步到 beta8 后停止。
- 提交：尚未提交。
- 代码：`.github/workflows/docker-publish.yml`。
- 迁移与测试：无。

## 提前回补的上游 Feature

### 模型目录后端热加载

- 行为：模型页与渠道模型价格弹窗改为共用后端 `providersCatalog(filtered)` 一个数据源；
  后端从 PublicProviderConf `all.json` 拉取并缓存，默认每小时刷新，可在系统设置中修改
  上游 URL 与刷新间隔，模型页可手动刷新目录；拉取失败时回退到内嵌快照。
  `filtered=true` 在后端按 `DefaultDeveloperIDs` 过滤并合并 `catalogdata/models.json`。
- 动机：修复 fork 中模型页预设依赖上游手动同步 `providers.json` 的滞后问题。
- 上游来源：`4483c2e4`（PR #2263）。下一次同步上游 release 时该提交应已包含在内，
  合并时按上游版本为准。
- 提交：`d6a605a1`。
- Fork 适配：`internal/ent/internal/schema.go` 冲突以 fork 版本为准并重新执行
  `go generate` 生成；未引入上游中间提交的其他 schema 变更。
- 代码：`internal/server/biz/catalog*.go`、`internal/server/biz/catalogdata/`、
  `internal/server/gql/system.graphql`、`internal/server/gql/system.resolvers.go`、
  `frontend/src/features/models/data/providers.ts`、
  `frontend/src/features/system/components/catalog-settings.tsx`。
- 迁移：无；catalog 设置存于 system 表的 `catalog_settings` 键。
- 测试：`internal/server/biz/catalog_test.go`、`internal/server/biz/catalog_filter_test.go`。
- 已知差异：`catalogdata/providers.json` 与 `frontend/.../providers.json` 内嵌快照未随上游
  后续 `chore: sync model developers data` 更新，仅影响离线回退。

## Bugfix Topics

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
  可为零值字段排序的分页入口，需同样接入。

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
- 测试：
  `internal/ent/migrate/datamigrate/v1.0.0-beta7-fork.1_test.go`。

### 上游模型目录 schema 修复回补

- 问题：PublicProviderConf 的 DeepSeek 模型把 `experimental` 字段改为裸布尔值，
  `providersDataSchema.parse()` 抛错，`useProvidersData` 与 `useDevelopersData` 均静默回退到
  内嵌的 `providers.json` 快照，导致模型页预设和渠道模型价格识别都停留在该快照。
- 上游来源：`6f729f7c`（PR #2305），原样 cherry-pick，无 fork 适配。
- 提交：`8c978c6b`。
- 代码：`frontend/src/features/models/data/providers.schema.ts`。
- 迁移与测试：无迁移；无专用测试。
