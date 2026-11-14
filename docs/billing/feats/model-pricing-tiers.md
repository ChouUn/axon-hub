# 模型权威定价与全量阶梯

> Parent: [../index.md](../index.md)
> Audience: developer — 开发和演进项目的人或 agent
> Genre: feat — 这个 feature 是什么、为什么值得、什么形态
> Visibility: public — 他人可读，避免写入本机环境
> Maintain: living — 持续更新，保持内容有效

- 行为：模型新建、编辑及目录批量导入统一保存完整标准价格，列表和详情展示基础价格、
  全量阶梯及缓存写入变体。直接展示并编辑已有价格，不设「模型定价」开关，不因开关
  清空或重建价格；零价与未填写价格区分。
- 全量阶梯：输入总 tokens（含缓存）严格超过所填阈值时，全部计费项使用命中档位
  单价；支持多档，以及一次按基础价倍数填满整档后单独改价。填充倍数只是编辑操作，
  不作为另一项计费倍率存储。既有时间覆盖命中时仍优先整组替换价格。
- Anthropic 官方目录导入：provider 为 `anthropic` 且基础价同时含输入价与写缓存价时，
  追加 `one_hour` 写缓存变体，单价为输入价的 2 倍（十进制精确乘法）；每档阶梯按自己的
  输入价计算。5 分钟写入使用基础写缓存价，不额外添加 `five_min`。其他 provider、缺任一
  基础价时不推算；导入后可编辑或删除变体，已有模型不自动回填，需要时手动补价或重新导入。
  该规则补足上游目录仅提供 5 分钟写缓存价的缺口，倍数依据 [Anthropic 官方定价](https://platform.claude.com/docs/en/build-with-claude/prompt-caching#pricing)。
- 编辑交互：新增计费项和缓存写入变体的价格留空，明确填写的零价保留；新增阶梯阈值
  留空且填写有效阈值前不能保存，已有阶梯照常回显。添加按钮显示「添加计费项」，
  四项齐全时禁用并显示「已添加全部计费项」，删除一项后恢复可用。
- 目录选模：切换模型时同步重置价格及阶梯表单数组，无阶梯时清空数组，价格行与阶梯开关
  按已导入数据正确回显，不残留旧模型档位。
- 兼容：旧模型卡价格在 JSON 读取时转换为完整价格，旧成本字段仅作只读摘要；明确
  清除后不会从旧摘要恢复。已有渠道价格、历史价格版本和旧逐项阶梯计算不变。
- 渠道：既有自动补缺逻辑复制完整模型价格（含阶梯和缓存变体），不覆盖已有渠道价格；
  渠道引用、编辑及倍率衔接见「渠道标准价格引用与持久倍率」。
- 提交：`93e5878e`（权威模型定价与全量阶梯）；`2eca7c59`（渠道完整价格引用、倍率衔接与阶梯填充交互）。
  `bf37dae1`（Anthropic 官方目录一小时写缓存价）。
- 代码：`frontend/src/features/models/components/models-price-editor.tsx`、
  `frontend/src/features/models/data/pricing.ts`、`frontend/src/components/model-price-editor.tsx`、
  `internal/objects/model.go`、`internal/objects/price.go`、`internal/server/biz/model.go`、
  `internal/server/biz/cost_calc.go`、`frontend/src/features/models/components/models-batch-create-dialog.tsx`；
  定价说明见 `docs/{zh,en}/guides/cost-tracking.md`。
- 迁移：无新增表字段或迁移；兼容既有数据库及缓存中的模型卡 JSON。已重新生成接口代码。
- 测试与 fixture：`internal/objects/model_test.go`、`internal/objects/price_test.go`、
  `internal/server/biz/model_validation_test.go`、`frontend/src/features/models/pricing.test.mjs`，
  覆盖旧价格兼容、全量阶梯校验、价格保存与清除、整档填充、零价与未填写价格的区分，以及
  Anthropic 来源的一小时价格、各阶梯独立计算、其他来源与缺价时不推算；无新增 fixture。
- 同步上游注意：上游若调整目录导入或提供独立一小时写缓存价，应核对 `priceFromCatalog`
  并优先使用目录值，避免重复推算。
