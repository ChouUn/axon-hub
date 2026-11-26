# 渠道定价

> Parent: [../index.md](../index.md)
> Audience: developer — 开发和演进项目的人或 agent
> Genre: feat — 这个 feature 是什么、为什么值得、什么形态
> Visibility: public — 他人可读，避免写入本机环境
> Maintain: living — 持续更新，保持内容有效

渠道定价是渠道页为每个渠道模型设置价格的窗口，请求成本按这里的价格计算。窗口本身是上游已有的。
本 fork 改了两处：价格可引用[模型权威定价与全量阶梯](model-pricing-tiers.md)的标准价格并按渠道保存倍率，
另回补了上游的价格时间戳修复。

## 渠道标准价格引用与持久倍率

### 动机

- 渠道的价格大多是标准价乘一个统一倍率。
- 上游的定价窗口有一个倍率输入框，但它只在填价时把目录单价乘一次：
  - 写进渠道的是折算后的具体数值。
  - 倍率本身不保存，每次打开窗口重置为 1（上游 `v1.0.0-beta10` 的
    `frontend/src/features/channels/components/channels-model-price-dialog.tsx`）。上游的 `ModelPrice` 没有倍率字段。
- 渠道价格因此仍是一项项具体数值，手动逐个配置不方便。
- 标准价已由[模型权威定价与全量阶梯](model-pricing-tiers.md)维护一份，渠道引用它即可，不必再各自维护一套。

### 形态

- 引用：
  - 渠道定价引用模型模块维护的完整价格。
  - 可按渠道模型 ID 一键匹配，也可为指定渠道模型选择其他标准价格。
    匹配依次尝试渠道模型 ID、去掉推理档位后缀的基础名、去掉自动裁剪前缀后的名字及其基础名，
    取第一个有标准价的模型，价格仍存在渠道模型 ID 下。手动模型映射不参与匹配。
    档位后缀的拆分规则见[渠道识别档位后缀](../../routing/feats/reasoning-effort-suffix.md)。
  - 带入基础单价、全量阶梯、缓存变体及时间覆盖，引用后仍可修改。
  - 无标准价格的模型不覆盖既有渠道价格，不从外部目录填充。
- 倍率：
  - 按渠道独立保存，缺省为 1，支持显式零值。
  - 基础价格不随倍率改写。各类单价输入框右侧同一行只读展示实际价格，不另占一行。
  - 保存时将倍率写入价格版本快照，倍率变更生成新版本。
  - 历史版本不重算。
  - 普通渠道设置修改不覆盖倍率。
  - 自动补缺及渠道复制继承倍率。
- 未设价时的兜底：
  - 渠道没有为模型设价、模型库有该模型的价格时，按模型库的标准价格计费。兜底由上游 `cc1c7e5d`（PR #2655）引入。
  - 模型库没有精确的模型 ID 时，去掉推理档位后缀再找一次。上游只去掉 `-high`、`-medium`、`-low`，
    本 fork 改用与「自动推理强度」相同的拆分规则，覆盖 `-xhigh`、`-max`，不区分大小写，`qwen` 系列的 `-max` 不拆。
  - 兜底价格乘渠道当前的倍率。上游的兜底不乘倍率。
  - 兜底价格取自上游 `v1.0.0-rc.4` 引入的共享模型缓存，倍率乘在价格副本上，缓存中的标准价格不变。
  - 兜底没有价格版本快照，倍率随用量记录的 `cost_price_reference_id` 保存：

    | 渠道倍率 | 引用写法 |
    | --- | --- |
    | 未设或为 1 | `model_library:<模型>`，与上游相同 |
    | 其他取值，含 0 | `model_library@<倍率>:<模型>`，倍率为十进制小数 |

  - 已有记录的倍率从引用读出，之后修改渠道倍率不改变它。
  - 模型库也没有价格的模型不计费。
- 交互：
  - 定价窗口使用视口高度，编辑区独立滚动，顶部引用与倍率、底部操作区固定。
  - 各行价格输入框等宽。
  - 每档提供基础价倍数及整档填充，包含缓存变体，填充后可单项修改。
- 导入导出：
  - 新版导出同时保存倍率与基础价格。
  - 旧数组格式导入按倍率 1 承接。

### 边界

- 标准价格的内容与编辑由[模型权威定价与全量阶梯](model-pricing-tiers.md)负责，本功能只引用。
- 倍率在请求成本里的展示见[请求成本明细悬浮](request-cost-breakdown.md)。

### 实现与维护

- 提交：
  - `2eca7c59`（渠道标准价格引用、持久倍率与价格编辑交互）
  - `0af05bef`（价格对话框转发校验错误的类型修正）
  - `55ca6e74`（模型库价格兜底乘渠道倍率）
  - `0212f0ec`、`f87aa4b6`（兜底计费的测试适配上游的共享模型缓存）
- 代码：
  - `frontend/src/features/channels/components/channels-model-price-dialog.tsx`
  - `frontend/src/components/model-price-editor.tsx`
  - `internal/server/biz/channel_price.go`
  - `internal/server/biz/usage_log.go`：兜底计费。
  - `internal/server/biz/usage_price_multiplier.go`：从引用读兜底倍率。
  - `internal/server/biz/cost_calc.go`
  - `internal/objects/channel.go`
  - `internal/objects/price.go`
  - `internal/server/gql/price.graphql`
- 迁移：无新增表字段或迁移。
  - 渠道设置、当前价格及历史版本使用兼容的 JSON 可选字段，旧数据缺省倍率为 1。
  - 备份保留倍率快照，不从当前渠道设置重算历史价格。
- 测试：无新增 fixture。
  - `internal/server/biz/channel_price_test.go`
  - `internal/server/biz/cost_calc_test.go`
  - `internal/server/biz/channel_auto_model_price_test.go`
  - `internal/server/biz/channel_model_sync_test.go`
  - `internal/server/biz/usage_cost_test.go`：兜底乘倍率、引用写法、修改渠道倍率后历史记录不变、
    倍率不写进共享模型缓存、同一请求内模型库价格变化时沿用请求开始时的价格。
  - `frontend/src/features/models/pricing.test.mjs`
  - `frontend/src/features/models/data/reasoning-efforts.test.mjs`：价格候选的顺序与去重。
- 同步上游：
  - 上游 `03ffac6f`（PR #2662，随上游 `v1.0.0-rc1` 发布）在价格对话框从模型库填价时乘汇率，默认 7.14。
    本 fork 的对话框改为引用标准价、不从外部目录填充，没有这段填价代码，所以合入时保留 fork 的整份对话框，不采用汇率。
  - 上游再改 `channels-model-price-dialog.tsx` 时，以 fork 的文件为准，只取与引用、倍率无关的改动。
  - 上游再改 `modelLibraryPriceSuffixes` 时，以共享的 `llm.SplitReasoningEffortModelSuffix` 为准，不恢复独立的后缀列表。

## 上游模型价格时间戳修复回补

### 动机

- 上游保存渠道模型价格时使用 `time.Now()`，可能使 SQLite 持久化带 monotonic 后缀的 `updated_at`，
  导致后续乐观锁更新无法匹配。
- 上游的修复是 `86eac3a7`（PR #2257），fork 提前回补。

### 形态

- 保存渠道模型价格时使用统一的时间戳，`updated_at` 不带 monotonic 后缀。
- 存量数据由 data migration `v1.0.0-beta7-fork.1` 清理：
  - 它由上游 data migration `v1.0.0-beta9` 的数据清理适配而来。
  - 仅清理 SQLite，PostgreSQL 跳过不适用的文本处理。

### 边界

- data migration `v1.0.0-beta7-fork.1` 已发布，保持不变。
- 上游 data migration `v1.0.0-beta9` 已随上游 `v1.0.0-beta10` 合入，与 `v1.0.0-beta7-fork.1` 同时注册：
  - 它包含同一 SQLite 清理，另加 `settings.providerQuota` 清理，`e4bf324d` 起同样跳过 PostgreSQL。
  - data migration 的注册顺序为 `v1.0.0-beta7-fork.1` → `v1.0.0-beta8` → `v1.0.0-beta9`。
  - `v1.0.0-beta7-fork.1` 与 `v1.0.0-beta9` 对 `updated_at` 的清理幂等。

### 实现与维护

- 提交：
  - `4896d9d6`（`fix(channels): 统一模型价格时间戳`）
  - `7fdbd2ae`：`v1.0.0-beta7-fork.1` 的测试 helper 改为 `fork` 前缀，避免与 `v1.0.0-beta9_test.go` 重名。
- 代码：
  - `internal/server/biz/channel_price.go`
  - `internal/ent/migrate/datamigrate/`
- 测试：`internal/ent/migrate/datamigrate/v1.0.0-beta7-fork.1_test.go`。
