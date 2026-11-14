# 模型权威定价与全量阶梯

> Parent: [../index.md](../index.md)
> Audience: developer — 开发和演进项目的人或 agent
> Genre: feat — 这个 feature 是什么、为什么值得、什么形态
> Visibility: public — 他人可读，避免写入本机环境
> Maintain: living — 持续更新，保持内容有效

模型页维护的标准价格：每个模型保存一份完整价格，含基础价、全量阶梯和缓存写入变体。
[渠道定价](channel-pricing.md)引用它，再乘各渠道自己的倍率。

## 动机

- 上游给模型和渠道各配一套价目表，同一个模型的价格要在两处各自维护：
  - 模型卡只有 `cost` 四个单价：输入、输出、读缓存、写缓存（`ModelCardCost`）。
  - 渠道价格才是完整的 `ModelPrice`，有计费项、定价模式和阶梯。
- 实际的阶梯价大多是全量的：输入总量超过阈值后，全部计费项都换成该档单价。
  上游的阶梯要在每个计费项上各自配置（`usage_tiered`、`usage_volume`），能细分，但配置不方便。
  常见情形用整组的全量阶梯即可，不必逐项手工配置。

## 形态

- 保存与展示：
  - 模型新建、编辑及目录批量导入统一保存完整标准价格。
  - 列表和详情展示基础价格、全量阶梯及缓存写入变体。
  - 直接展示并编辑已有价格，不设「模型定价」开关，不因开关清空或重建价格。
  - 零价与未填写价格区分。
- 全量阶梯：
  - 输入总 tokens（含缓存）严格超过所填阈值时，全部计费项使用命中档位的单价。
  - 支持多档。
  - 可一次按基础价倍数填满整档，再单独改价。
  - 既有时间覆盖命中时仍优先整组替换价格。
- Anthropic 官方目录导入：
  - provider 为 `anthropic` 且基础价同时含输入价与写缓存价时，追加 `one_hour` 写缓存变体，
    单价为输入价的 2 倍（十进制精确乘法）。
  - 每档阶梯按自己的输入价计算。
  - 5 分钟写入使用基础写缓存价，不额外添加 `five_min`。
  - 其他 provider、缺任一基础价时不推算。
  - 导入后可编辑或删除变体。已有模型不自动回填，需要时手动补价或重新导入。
  - 依据：[供应商模型目录](../../channels/feats/model-catalog.md)仅提供 5 分钟写缓存价，倍数取自
    [Anthropic 官方定价](https://platform.claude.com/docs/en/build-with-claude/prompt-caching#pricing)。
- 编辑交互：
  - 新增计费项和缓存写入变体的价格留空，明确填写的零价保留。
  - 新增阶梯阈值留空，填写有效阈值前不能保存。已有阶梯照常回显。
  - 添加按钮显示「添加计费项」。四项齐全时禁用并显示「已添加全部计费项」，删除一项后恢复可用。
- 目录选模：
  - 切换模型时同步重置价格及阶梯表单数组，无阶梯时清空数组。
  - 价格行与阶梯开关按已导入数据正确回显，不残留旧模型档位。

## 边界

- 填充倍数只是编辑操作，不作为另一项计费倍率存储。计费倍率属于[渠道定价](channel-pricing.md)。
- 与渠道的分工：
  - 既有自动补缺逻辑复制完整模型价格（含阶梯和缓存变体），不覆盖已有渠道价格。
  - 渠道引用、编辑及倍率衔接见[渠道定价](channel-pricing.md)的「渠道标准价格引用与持久倍率」。
- 兼容：
  - 只有 `cost`、没有 `price` 的存量模型卡在 JSON 读取时转换为完整价格 `price`。
    `cost` 由 `price` 生成，仅作只读摘要。
  - `price` 被明确清除（写为 null）后，不会从 `cost` 恢复。
  - 已有渠道价格和历史价格版本不变。
  - 计费项上各自配置的阶梯模式（`usage_tiered`、`usage_volume`）计算不变。
    全量阶梯是价格级的 `volumeTiers`，不是这两种模式。

## 链接

- 用户文档：[成本追踪](../../zh/guides/cost-tracking.md)。

## 实现与维护

- 提交：
  - `93e5878e`：权威模型定价与全量阶梯。
  - `2eca7c59`：渠道完整价格引用、倍率衔接与阶梯填充交互。
  - `bf37dae1`：Anthropic 官方目录一小时写缓存价。
- 代码：
  - `frontend/src/features/models/components/models-price-editor.tsx`
  - `frontend/src/features/models/data/pricing.ts`
  - `frontend/src/components/model-price-editor.tsx`
  - `frontend/src/features/models/components/models-batch-create-dialog.tsx`
  - `internal/objects/model.go`
  - `internal/objects/price.go`
  - `internal/server/biz/model.go`
  - `internal/server/biz/cost_calc.go`
- 用户文档：`docs/{zh,en}/guides/cost-tracking.md`。
- 迁移：无新增表字段或迁移，兼容既有数据库及缓存中的模型卡 JSON。
- 测试：无新增 fixture。
  - 文件：
    - `internal/objects/model_test.go`
    - `internal/objects/price_test.go`
    - `internal/server/biz/model_validation_test.go`
    - `frontend/src/features/models/pricing.test.mjs`
  - 覆盖：
    - 旧价格兼容、全量阶梯校验、价格保存与清除、整档填充、零价与未填写价格的区分。
    - Anthropic 来源的一小时价格、各阶梯独立计算、其他来源与缺价时不推算。
- 同步上游：上游调整目录导入或提供独立的一小时写缓存价时，核对 `priceFromCatalog` 并优先使用目录值，
  避免重复推算。
