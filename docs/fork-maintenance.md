# Fork 维护台账

> Parent: [AGENTS.md](../AGENTS.md)

本台账按 feature 和 bugfix topic 记录相对上游 AxonHub 的本地差异，供新增本地改动和
同步上游时核对。这里只记录已确认的行为、代码归属和证据；是否保留、调整或删除
本地改动，由用户决定。

## 维护检查表

### 新增或实质修改 Fork Topic

- [ ] 新增或更新一个独立 topic，不把无关改动混在同一条目中。
- [ ] 明确 topic 属于 feature 还是 bugfix。
- [ ] 记录用户可观察行为、边界、相关提交和主要代码路径。
- [ ] Bugfix 若为上游回补，记录上游来源及本地适配。
- [ ] 明确 schema migration、data migration、测试和 fixture 的影响；没有也要注明。
- [ ] 删除已经失效的描述，保证台账反映当前实现。

### 同步上游 Release

- [ ] 逐项阅读当前 topic，并对照上游在相关代码路径和行为上的变化。
- [ ] 同时核对 schema migration、data migration 及其版本顺序。
- [ ] 报告语义重叠、兼容性变化和迁移冲突，不只依赖 Git 冲突判断。
- [ ] 涉及保留、调整或删除本地改动时，先取得用户决定。
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

## Bugfix Topics

### Provider 模型成本可为空

- 行为：模型目录中的 `cost` 可以缺失或为 `null`，远程和本地 provider 数据均可解析；
  不改变渠道价格的存储格式。
- 提交：`d4aa15c7`（`fix(models): 允许提供商成本为空`）。
- 代码：`frontend/src/features/models/data/providers.schema.ts`。
- 迁移与测试：无迁移；无专用测试。

### 上游模型价格时间戳修复回补

- 问题：上游保存渠道模型价格时使用 `time.Now()`，可能使 SQLite 持久化带 monotonic
  后缀的 `updated_at`，导致后续乐观锁更新无法匹配。
- 上游来源：`86eac3a7`（PR #2257）。
- 本地提交：`4896d9d6`（`fix(channels): 统一模型价格时间戳`）。
- 本地适配：将上游 beta9 数据清理适配为 `v1.0.0-beta7-fork.1`；仅清理 SQLite，
  PostgreSQL 跳过不适用的文本处理。
- 代码：`internal/server/biz/channel_price.go`、
  `internal/ent/migrate/datamigrate/`。
- 测试：
  `internal/ent/migrate/datamigrate/v1.0.0-beta7-fork.1_test.go`。
