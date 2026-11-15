# 请求成本明细悬浮

> Parent: [../index.md](../index.md)
> Audience: developer — 开发和演进项目的人或 agent
> Genre: feat — 这个 feature 是什么、为什么值得、什么形态
> Visibility: public — 他人可读，避免写入本机环境
> Maintain: living — 持续更新，保持内容有效

请求「成本」列上的悬浮明细：列出一次请求的成本由哪几项、按什么单价、乘了多少倍率构成。
管理端请求列表和[API Key 自助用量页](../../usage/feats/self-usage.md)的请求日志共用同一个组件。

## 动机

本功能之前（`1c5509ae~1`，下列行号取自该提交）：

- 管理端请求列表「成本」列与自助用量页请求日志只显示合计
  （`frontend/src/features/requests/components/requests-columns.tsx:521-542`、
  `frontend/src/features/self-usage/index.tsx:464-466`），看不出由哪几项、按什么单价、是否乘了倍率构成。
- 分项只在请求详情页展示，且不含单价与倍率
  （`frontend/src/features/requests/components/request-detail-content.tsx:465-569`）。

## 形态

D 编号是确定形态的决策，R 编号是行为规则。

- D1 范围：管理端请求列表与自助用量页请求日志的「成本」列，两处使用同一悬浮组件。
  请求详情页、用量日志页、自助页汇总卡、趋势与按模型统计不在范围内。
- D2 管理端内容：分项、折算单价、阶梯与倍率。
- D3 自助页内容：与管理端完全一致，含基础合计与倍率。
  不采用 CCH（`ding113/claude-code-hub`）对 Key 持有者隐藏分项与倍率的做法。
- D4 行结构对齐：
  - 已定价的悬浮一律依次列分项、基础合计、倍率、合计，不因倍率取值增减行。
  - 1 倍也显示基础合计与「×1」。
  - 合计行只显示最终金额，无删除线基础合计。
  - 自助页合并记录的倍率不一致时，倍率显示「混合」。

## 行为规则

- R1 分项来源：只读已落库的 `usage_logs.cost_items`，不按当前价格重算。
  - 顺序为输入、输出、读缓存、写缓存、其他项。
  - 写缓存有 5m、1h 变体时分行标注。
  - 其他项显示项目代码原名。
  - 只列数量或金额大于 0 的项。
- R2 每项显示 Token 数、金额与折算单价。
  - 金额用系统币种，6 位小数。
  - 折算单价为金额 × 1,000,000 ÷ 数量，即每百万 Token 的价格。保留 2 至 6 位小数，
    使 0.0028 这类低单价不被舍为 0。数量为 0 时不显示单价。
  - `tierBreakdown` 多于一段时，逐段列出区间上限、数量与金额。
- R3 倍率：
  - 取用量记录 `cost_price_reference_id` 对应价格版本（`channel_model_price_versions.reference_id`）
    的 `price.multiplier`，缺省按 1。
  - 无价格引用（引用上线前的记录）或价格版本已不存在时同样按 1：计费当时不存在倍率，实际按 1 倍计费。
    代码只归档、不删除价格版本。
  - 模型库兜底计费的记录没有价格版本，倍率从引用读出，写法见[渠道定价](channel-pricing.md)。
  - 未定价记录（`total_cost` 为空）不返回倍率。
  - 基础合计 = 合计 ÷ 倍率，1 倍时与合计相同。各项金额为已含倍率的落库金额。
  - 倍率为 0 时落库金额全为 0，无法反推基础合计，显示「-」。
- R4 合计：
  - 悬浮末行合计与单元格数值一致。
  - 合计为空（未定价）的单元格显示「-」且无悬浮。
  - 自助页部分未定价时，悬浮末尾标注未定价记录数。
- R5 多条用量记录：
  - 管理端列表沿用上游做法，只取第一条用量记录（`usageLogs(first: 1)`）。
  - 自助页一行汇总该请求全部用量记录，分项按项目代码与变体合并数量和金额。
  - 自助页各已定价记录倍率一致时列该倍率。不一致时倍率显示「混合」
    （`SelfUsageRequest.costMultiplierMixed`），基础合计显示「-」。
- R6 可见性：
  - 管理端倍率随用量记录一并返回，可见成本即可见倍率，不额外要求渠道读取权限。插件 GraphQL 入口同样如此。
  - 自助页沿用[API Key 自助用量页](../../usage/feats/self-usage.md) R2 的 Key 隔离。
- R7 价格来源标记：
  - 管理端请求列表的成本单元格在成本来自模型库兜底时显示一个硬币图标，悬浮说明成本取自模型库价格。
  - 请求详情页的同类标记是上游的，判断与列表共用，带倍率的引用同样识别。
  - 自助页不显示标记。

## 边界

- 倍率的保存与价格版本由[渠道定价](channel-pricing.md)负责，本功能只读取。
- 不做的范围见 D1。

## 链接

- [CCH 对照与取舍](../../reports/cch-comparison.md)：悬浮内容与自助页可见范围的对照。
- 用户文档：[自助用量页](../../zh/guides/self-usage.md)。

## 实现与维护

- 提交：
  - `1c5509ae`（`feat(requests): 请求成本明细悬浮并对齐自助页请求日志`）
  - `b61412dc`（`fix(requests): 成本悬浮始终显示倍率`）
  - `e28ec340`（`fix(requests): 成本悬浮基础合计与倍率行对齐`）
  - `55ca6e74`（模型库兜底的倍率与价格来源标记）
  - `7b689d40`（`fix(gql): 插件 GraphQL 入口装载成本加载器`）
  - `9a2036ad`（`fix(authz): fork 的宿主固定读取改用 RunAsHost`）：插件入口读取历史价格版本的倍率。
  - `699a7d8f`（共享单元格接入上游的指标配色）
- 代码，前端共享组件：
  - `frontend/src/components/request-usage-cells.tsx`：ID、Token、读写缓存、成本悬浮、耗时单元格。
    读缓存命中率与首字耗时按上游 `frontend/src/features/requests/utils/metric-colors.ts` 的设置着色。
  - `frontend/src/components/request-usage-cost-breakdown.ts`：悬浮行构建。
- 代码，管理端：
  - `frontend/src/features/requests/components/requests-columns.tsx`：对应列的 cell 用共享组件，列定义不变。
  - `frontend/src/features/requests/data/{requests.ts,usage-logs-schema.ts}`：列表与详情查询追加
    `costItems`、`costPriceMultiplier`。
  - `frontend/src/locales/{en,zh-CN}/requests.json`：`requests.costBreakdown.*`。
- 代码，管理端 GraphQL：
  - `internal/server/gql/cost.graphql`：`CostItem.promptWriteCacheVariantCode`。
  - `internal/server/gql/cost.resolvers.go`
  - `internal/server/gql/usage_cost.graphql` 与 `usage_cost.resolvers.go`（新文件）：
    `UsageLog.costPriceMultiplier`。
  - `internal/server/gql/usage_price_loader.go`：单次 GraphQL 操作内按 reference_id 批量查价格版本。
    - 共享查询使用操作上下文，单个字段的上下文只约束自身等待。
    - 查询失败不缓存。
  - `internal/server/biz/usage_price_multiplier.go`：以 `authz.RunAsHost` 固定读取用量记录引用的价格版本。
  - 上游文件追加：
    - `internal/server/gql/gqlgen.yml`：schema 列表。
    - `internal/server/gql/graphql.go`：主入口与插件入口都经 `usagePriceLoaderMiddleware` 注入 loader。
- 代码，自助页：
  - `internal/server/biz/self_usage_cost.go`：本页请求的分项合并与倍率判定。
  - `internal/server/biz/self_usage.go`
  - `internal/server/gql/selfusage/{selfusage.graphql,resolver.go}`
  - `frontend/src/features/self-usage/{api.ts,index.tsx}`
  - `frontend/src/locales/{en,zh-CN}/selfUsage.json`
- 用户文档：`docs/{zh,en}/guides/self-usage.md`。
- 迁移：无 schema migration、data migration 与缓存键变化。
- 测试：
  - `internal/server/biz/self_usage_test.go`：
    - 多条用量记录分项合并。
    - 倍率一致、不一致为混合、版本缺失按 1 参与判定、无引用按 1。
    - 未定价记录不参与倍率判定。
    - 模型库兜底的记录在渠道倍率修改后仍返回计费时的倍率。
    - 其他 Key 的用量不出现。
  - `internal/server/gql/usage_cost_test.go`：
    - 历史价格版本倍率。
    - 无引用与版本缺失返回 1，未定价返回 null，未设变体返回 null。
    - 模型库兜底的引用返回其中保存的倍率。
    - 同批多行只发一次版本查询，且同操作内不重复查询。
    - 一个字段取消不影响同批其他字段。
    - 批次窗口由测试手动关闭，不依赖调度时机。
    - 插件入口读取模型库兜底的倍率（`TestPluginUsageLogFallbackMultiplier`）。
  - `internal/server/gql/plugin_host_read_test.go`：插件入口读取历史价格版本的倍率，缺用户 scope 或缺插件授权时拒绝。
  - `frontend/src/components/request-usage-cost-breakdown.test.mjs`：
    - 排序与零值跳过。
    - 折算单价。
    - 倍率 1、2、0.5 的基础合计。
    - 倍率 0 与混合无基础合计。
    - 阶梯。
  - `frontend/src/components/request-usage-cells.test.mjs`：价格来源标记只在模型库兜底的记录上显示。
- 同步上游：
  - 上游调整请求列表的 Token、缓存、成本、耗时列或 `CostItem` 类型时，同步到共享组件与 `cost.graphql`。
  - `graphql.go` 的 `AroundOperations` 注入与上游已有的操作钩子并列。上游新增 GraphQL 入口时同样装载 `usagePriceLoaderMiddleware`。
  - 读取价格版本用 `authz.RunAsHost`，不用 system bypass：上游 `v1.0.0-rc1` 起插件主体不能提升为 System。
