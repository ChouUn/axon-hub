# 仪表盘分析

> Parent: [../index.md](../index.md)
> Audience: developer — 开发和演进项目的人或 agent
> Genre: feat — 这个 feature 是什么、为什么值得、什么形态
> Visibility: public — 他人可读，避免写入本机环境
> Maintain: living — 持续更新，保持内容有效

fork 在仪表盘上新增的两张分析表：「API 密钥费用分析」按密钥汇总并展开到模型，
「模型详情分析」按模型汇总并展开到实际请求渠道。两张表都按费用排名，供管理员查看费用的去向。

## 动机

- 上游仪表盘自带一批统计图表：按渠道、模型、API 密钥的请求数、Token 与费用，以及渠道成功率、
  最快渠道与模型等（`internal/server/gql/dashboard.graphql`）。
- 这些图表样式多，但没有业务上的意义。

## 形态

### 共用

- 日期筛选：今天、昨天、本周、本月和全部时间。自然日边界使用系统设置中的时区。
  - 上游自己的分析页自 `2851a192`（PR #2775，随上游 `v1.0.0-beta12` 发布）起按秒级时间范围筛选，
    时间取浏览器时区。这两张表不跟随，保持自然日。
  - API 密钥费用分析与上游分析页共用 `AnalyticsFilter`，它的起止时间随上游改为 RFC3339 时间戳。
    前端在发请求前把系统时区的自然日换算成闭区间时间戳。
  - 模型详情分析用自有的 `AnalyticsModelFilter`，起止仍是 `YYYY-MM-DD`，由后端按系统时区解析。
- 按费用排名，明细默认全部收起。

### API 密钥费用分析

- 按 API 密钥汇总请求数、Token 数和费用。
- 每个密钥可展开查看模型明细。
- 另可按密钥模板筛选。

### 模型详情分析

- 以模型为汇总项，以实际请求渠道为明细项。
- 另可按渠道标签筛选。
- 指标：请求数、费用、Token 数、费用/Mtok、成功率、缓存命中率、平均 TTFB 和平均输出速度。
  - 缓存命中率按缓存输入 Token 占输入 Token 的百分比计算，输入 Token 为零时显示 0%。
  - 输出速度的输出 Token 只取 `completion_tokens`（其中已含 reasoning 与 audio），不重复累计
    reasoning 与 audio。口径与上游的仪表盘吞吐一致（上游 `92f81b32`，随 `v1.0.0-beta8` 发布）。
- 列顺序：排名、模型/渠道、请求数、Token 数、成功率、缓存命中率、平均 TTFB、平均输出速度、
  费用/Mtok、费用。表格不重复显示内部标题。
- 排序：
  - 默认按费用降序。
  - 除排名外，各列通过无边框列头按钮在降序、升序和无序间切换。进入无序时恢复费用降序。
  - 排序同时作用于模型和各自的渠道明细，排名按当前顺序重算。
  - 模型的展开状态不随排序改变。
- 重试语义：
  - `request_executions` 中每次 `completed` 或 `failed` 执行均视为一次实际请求，包括渠道内和渠道间重试。
  - 失败执行计入请求数和成功率。
  - 费用与 Token 来自成功用量。

## 边界

- 请求数的口径与 [API Key 自助用量页](self-usage.md)不同，两处的数字不能直接对比：
  - 模型详情分析按 `request_executions` 的执行计数，一次请求的每次重试各算一次。
  - 自助用量页按请求表每行计一次。

## 实现与维护

- 提交：
  - `d3d4160e`：新增 API 密钥费用分析。
  - `7415d058`（`feat(analytics): 新增模型详情分析`）：新增模型详情分析，并统一系统时区日期边界。
  - `62011e01`：输出速度口径对齐上游。
- 代码：
  - `frontend/src/features/analytics/`
    - `utils/date-range.ts`：自然日范围与到时间戳的换算，fork 的文件。
    - `utils/date-time-range.ts`：上游的秒级时间范围。上游的文件名是 `utils/date-range.ts`，与 fork 的文件重名，合入时改名。
  - `internal/server/gql/analytics.graphql`
  - `internal/server/gql/analytics_helpers.go`
- 迁移：无。
- 测试：
  - `internal/server/gql/analytics_helpers_test.go`
  - `frontend/src/features/analytics-date-range.test.mjs`：含自然日到时间戳的换算。
- 同步上游：上游改动 `frontend/src/features/analytics/utils/date-range.ts` 时，改动落到 `utils/date-time-range.ts`，
  上游的 `utils/date-range.test.mjs` 随之指向它。
- fixture：
  - `scripts/e2e/fixtures/api-key-analytics.sql`
  - `scripts/e2e/fixtures/model-analytics.sql`
