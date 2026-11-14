---
description: "新增或实质修改 fork 改动、同步上游 release 时的维护检查表"
---

# Fork 维护检查表

本检查表用于新增 fork 改动和同步上游时核对。fork 相对上游 AxonHub 的差异按模块和功能记录在
[docs/index.md](../../docs/index.md)，只记录已确认的行为、代码归属和证据。
是否保留、调整或删除 fork 改动，由用户决定。

## 新增或实质修改 fork 改动

- [ ] MUST 把改动写进所属功能的文档（`docs/<模块>/feats/<功能>.md`）。没有对应功能时新增一份，并挂到模块索引。
- [ ] MUST 按功能归属，NEVER 按提交的 `feat:` / `fix:` 前缀分类：
  - fork 自有功能的行为、实现和验证统一维护在该功能的文档中，NEVER 按修复批次另立文档或修订小节。
  - 对上游已有功能的修正记录在所修功能的文档中。
- [ ] MUST 记录用户可观察行为、边界、相关提交和主要代码路径。
- [ ] 修正若为上游回补，MUST 记录上游来源、fork 中对应的提交及 fork 适配。
- [ ] MUST 写明对 schema migration、data migration、测试和 fixture 的影响，没有影响时写「无」。
- [ ] MUST 删除已经失效的描述，保证文档反映当前实现。
- [ ] 已与上游完全一致且无兼容负担的改动 MUST 只保留在上游同步记录。
- [ ] 已发布迁移等仍须维护的差异 MUST 继续保留在所属功能的文档中。

## 同步上游 Release

- [ ] MUST 逐项阅读各功能文档，并对照上游在相关代码路径和行为上的变化。
- [ ] MUST 同时核对 schema migration、data migration 及其版本顺序。
- [ ] MUST 报告语义重叠、兼容性变化和迁移冲突，NEVER 只依赖 Git 冲突判断。
- [ ] 涉及保留、调整或删除 fork 改动时，MUST 先取得用户决定。
- [ ] MUST 按最终实现更新功能文档中的事实、路径和相关提交。
- [ ] MUST 在 `docs/reports/` 新增本次同步记录，并挂到 `docs/index.md` 的「上游同步」。
