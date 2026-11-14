# 供应商模型目录

> Parent: [../index.md](../index.md)
> Audience: developer — 开发和演进项目的人或 agent
> Genre: feat — 这个 feature 是什么、为什么值得、什么形态
> Visibility: public — 他人可读，避免写入本机环境
> Maintain: living — 持续更新，保持内容有效

供应商模型目录是上游的 provider 与模型清单，有内嵌快照和远程拉取两个来源，模型页从中选模。
本 fork 只改了前端的解析 schema：模型的 `cost` 可以为 `null`。

## 动机

上游 `v1.0.0-beta10` 的 `providerModelSchema.cost` 只允许缺省，不接受 `null`。

## 形态

目录中模型的 `cost` 缺失或为 `null` 时，远程拉取和内嵌快照的 provider 数据都能解析。

## 边界

- 只修正目录解析，不改变渠道价格的存储格式。
- 与[模型权威定价与全量阶梯](../../billing/feats/model-pricing-tiers.md)无关：后者决定目录价格导入后怎么保存。

## 实现与维护

- 提交：`d4aa15c7`（`fix(models): 允许提供商成本为空`）。
- 代码：`frontend/src/features/models/data/providers.schema.ts`。
- 迁移：无。
- 测试：无专用测试。
