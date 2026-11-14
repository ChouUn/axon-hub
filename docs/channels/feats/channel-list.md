# 渠道列表

> Parent: [../index.md](../index.md)
> Audience: developer — 开发和演进项目的人或 agent
> Genre: feat — 这个 feature 是什么、为什么值得、什么形态
> Visibility: public — 他人可读，避免写入本机环境
> Maintain: living — 持续更新，保持内容有效

渠道列表是管理端的渠道页：顶部 tab 把渠道分组，下方是可排序、分页的表格。页面本身是上游已有的。
本 fork 改了三处：tab 按供应商聚合，生图渠道单独成组，修正排序值为零时的分页错乱。

## 渠道 tab 按供应商聚合

### 动机

- 上游 `fa74d927`（PR #150）提供渠道 tab，`ea31f440`（PR #1734）调整过前缀分组。
- 上游按下划线前缀折叠类型，且只在裸前缀类型同时存在时才折：
  - `moonshot_anthropic` 与 `moonshot_coding`、`opencode_go` 与 `opencode_go_anthropic` 各自成组。
  - `github_copilot` 被误并入 GitHub。

### 形态

- tab 按 `CHANNEL_TYPE_TO_PROVIDER` 把同一供应商的协议变体聚成一组：
  - `deepseek`、`deepseek_anthropic` 归入 DeepSeek。
  - `moonshot`、`moonshot_anthropic`、`moonshot_coding` 归入 Moonshot。
- 标签与图标取自 `PROVIDER_CONFIGS`。
- tab 筛选使用该组的实际类型列表，不用类型名前缀。
- 未在映射中的类型保留独立分组。

### 边界

- 只替换分组依据，没有新增渠道 tab 功能。
- 徽章列原本已显示供应商，未改。
- 映射里补了缺失的 `ollama_anthropic`。上游 `aa8e7c81`（随上游 `v1.0.0-beta8` 发布）也把它映射到 ollama，
  合并后为同一行，不是 fork 差异。

### 实现与维护

- 提交：`54fab437`（`feat(channels): 按供应商聚合渠道 tab`）。
- 代码：
  - `frontend/src/features/channels/utils/group-channel-types.ts`
  - `frontend/src/features/channels/components/channels-type-tabs.tsx`
  - `frontend/src/features/channels/index.tsx`
  - `frontend/src/features/channels/data/config_channels.ts`
- 迁移：无。
- 测试与 fixture：
  - `frontend/src/features/channels/group-channel-types.test.mjs`
  - `scripts/e2e/fixtures/channel-groups.sql`

## 生图渠道拆分

### 动机

- 生图渠道的类型是供应商基类型，与同供应商的对话渠道落在同一个 tab。
- 渠道测试对所有渠道都发对话请求。

### 形态

- 标记：`ChannelSettings.primaryApiFormat`（JSON 列）记录渠道的主用途。
- 对话框：
  - 新建、编辑对话框对默认端点含 `openai/image_generation` 的供应商增加「OpenAI (图像生成)」选项。
  - 选中后类型仍为供应商基类型，写入 `settings.primaryApiFormat=openai/image_generation`。
  - 编辑时正确回显图像格式，保存时保留生图标记。
- tab 与徽章：
  - 带标记的渠道单独成组「<供应商> · 生图」，组 key 为 `<provider>:image`。
  - 徽章追加同一后缀。
  - 供应商普通 tab 以 `excludePrimaryApiFormat` 排除生图渠道。
- 测试：
  - 渠道测试（含单 key 与批量 key）对带标记的渠道改发一次图像生成请求：1024x1024、`quality=low`、
    `n=1`，不走流式。
  - 成功判定为响应至少含一张图。
  - 生图测试耗时常超过后台请求的 30 秒上限，测试入口以 `server.channel_test_timeout` 为界，
    见[渠道测试](channel-test.md)。
- 已有生图渠道需编辑一次并改选该 API 格式后，才会分组与改测。

### 边界

- 标记不影响路由：渠道仍按默认端点同时承接 `image_edit`、`image_variation`。
- 不移除任何现有 API 格式选项（含 Anthropic Messages），不新增渠道类型，不改路由与端点解析。
- `codex` 虽有图像默认端点，但它的对话框隐藏 API 格式选择器，未加入可选供应商。
- 建立在「渠道 tab 按供应商聚合」之上。

### 实现与维护

- 提交：
  - `d6d17d00`（`feat(channels): 拆分生图渠道`）
  - `7fdbd2ae`（合入上游 `v1.0.0-beta10` 时重接）
  - `e18cc308`（补回列表字段与编辑同步）
  - `eaef7c06`（合入上游 `v1.0.0-beta11` 时重接渠道测试）
- 实现约束：
  - 选中图像格式后切换供应商时，类型须由 `getBaseChannelTypeForProvider` 推导。
    `getChannelTypeForApiFormat` 对图像格式恒为 undefined，否则表单 `type` 与 baseURL 停在上一家。
  - `excludePrimaryApiFormat` 谓词必须兼容 `settings` 中不存在该键的旧行，否则普通渠道会被一起滤掉。
  - `countChannelsByType` 按 `(type, 是否生图)` 在内存中计数并返回 `primaryApiFormat`，供分组函数拆组。
  - `CHANNEL_QUERY_LIST_NODE_BASE_SELECTION` 包含 `settings { primaryApiFormat }`，供列表徽章与分组读取。
  - 编辑初始化、`initialRow` 同步与打开时重置统一使用 `getInitialApiFormatForRow`。
- 与上游 `v1.0.0-beta10` 的接合：
  - `getApiFormatsForProvider` 上游已委托给 `protocol-options.ts`。fork 只在 `config_providers.ts`
    的包装层追加生图格式，不改上游文件。
  - 编辑对话框的 `settingsPatch` 与新建时 `mergeChannelSettingsForUpdate` 的参数中保留
    `primaryApiFormat`。
- 与上游 `v1.0.0-beta11` 的接合：
  - 上游按 API 格式构造测试请求（`channelTestAPIFormat`、`buildChannelTestInput`）。
    `channelTestAPIFormat` 先判生图标记，带标记的渠道得到图像生成格式。
  - `testSingleKey` 保留 fork 的 `ch *biz.Channel`，并接受上游的 `responsesWebSocket` 与 API 格式参数。
- 代码：
  - `internal/objects/channel.go`
  - `internal/server/gql/axonhub.graphql`
  - `internal/server/gql/axonhub.resolvers.go`
  - `internal/server/biz/channel_query.go`
  - `internal/server/orchestrator/tester.go`
  - `frontend/src/features/channels/utils/group-channel-types.ts`
  - `frontend/src/features/channels/components/channels-type-tabs.tsx`
  - `frontend/src/features/channels/index.tsx`
  - `frontend/src/features/channels/components/channels-action-dialog.tsx`
  - `frontend/src/features/channels/components/channels-columns.tsx`
  - `frontend/src/features/channels/data/config_channels.ts`
  - `frontend/src/features/channels/data/config_providers.ts`
- 迁移：无。
- 测试与 fixture：
  - `frontend/src/features/channels/group-channel-types.test.mjs`
  - `internal/server/orchestrator/tester_test.go`
  - `internal/server/orchestrator/tester_formats_test.go`
  - `internal/server/biz/channel_query_test.go`
  - `scripts/e2e/fixtures/channel-groups.sql`

## 排序字段为零值时分页错乱

### 动机

- entgql 的 `Cursor.Value` 带 `msgpack:"v,omitempty"`。排序字段值为该类型零值时
  （如 `ordering_weight` 为 0），游标丢掉该值，解码为 nil。
- `entgql.CursorsPredicate` 随之退化为只按 id 比较，下一页重复或漏掉行。
- 表现：渠道页按权重排序、每页 20 条时，第一页末尾是权重 0，第二页开头又出现权重 5、10。
- 触发条件：只在某页最后一行的排序值为零时触发，与 tab 无关。
  - 「全部」若带权重的渠道不少于 20 个，第一次翻页落在非零权重上看起来正常，
    翻到零权重之后的下一页同样出错。
  - 分类 tab 带权重的渠道少，第一次翻页就出错。
- 来源：ent/contrib `3625dcc2e035`（本 fork 锁定版本）仍带 `omitempty`，未见修复。

### 形态

- `biz.RestoreZeroCursorValue` 在 `Paginate` 前，用排序字段在空实体上取到的零值补回 `cursor.Value`。
  `omitempty` 只省略零值，所以 nil 必然对应零值。
- 接入三个分页入口：
  - `ChannelService.QueryChannels`
  - `Query.channels`
  - `Query.prompts`（`prompt.order` 同样默认 0）
- 渠道页切换排序时重置分页游标，避免旧排序下产生的游标被带入新排序。

### 边界

- 不改依赖，在调用侧绕过。
- 默认 ID 排序的 `Field.String()` 为空，跳过不补。

### 实现与维护

- 提交：`973928e3`（`fix(channels): 修复排序值为零时游标分页错乱`）。
- 代码：
  - `internal/server/biz/cursor.go`
  - `internal/server/biz/channel_query.go`
  - `internal/server/gql/ent.resolvers.go`
  - `frontend/src/features/channels/index.tsx`
- 迁移：无。
- 测试：`internal/server/biz/channel_query_test.go`。
  - `TestChannelService_QueryChannels_OrderingWeightZeroCursor`
  - `TestChannelService_QueryChannels_VendorTabZeroCursor`：同时覆盖「全部」与分类 tab 逐页遍历。
  - 两者的游标都经 `MarshalGQL`、`UnmarshalGQL` 往返以复现。
- 同步上游：
  - 上游 entgql 去掉 `omitempty` 后，该绕过变为无害冗余，可移除。
  - 上游新增按可为零值字段排序的分页入口时，需同样接入。
  - 上游 `v1.0.0-beta10` 仍锁定 ent/contrib `3625dcc2e035`，`ent.resolvers.go` 的 `Paginate` 调用点未增加，
    合入时无需改动。
