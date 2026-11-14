# CCH 对照与取舍

> Parent: [../index.md](../index.md)
> Audience: developer — 开发和演进项目的人或 agent
> Genre: survey — 当时基于什么信息做了什么判断
> Visibility: public — 他人可读，避免写入本机环境
> Maintain: frozen 2026-11-14 — 已定稿；内容与定稿日期不改，内容变化只追加 Outdated / Superseded 段，元数据可维护，定位用的路径与链接可随文件移动更新到原对象与版本的新位置

CCH 指 `ding113/claude-code-hub`。本 fork 的四个功能在设计时对照了它的实现。
本文记录对照到的做法和 fork 的取舍，路径均相对 CCH 仓库根。

## 结论

| # | 功能 | 取自 CCH | 没有采用 |
| --- | --- | --- | --- |
| 1 | 健康门控：选路门控 | 失败口径、全部不可用时的返回、管理端徽章、筛选与重置 | 按供应商整体熔断的粒度 |
| 2 | 健康门控：会话归属与决策记录 | 会话绑定只在成功时写入、请求表里的决策记录列、开闸告警 | 故障转移成功即改绑、健康时迁回 |
| 3 | 渠道强制 1 小时提示缓存 | 改写请求中已有断点的 TTL | Key 级配置、补 beta 头、按请求偏好计 1h、计费互换 |
| 4 | API Key 自助用量页 | 用 Key 登录的独立只读页 | 展示额度、汇总所属用户名下其他 Key |
| 5 | 请求成本明细悬浮 | 成本列悬浮的分项与折算单价 | 对 Key 持有者隐藏分项与倍率 |

## 1 健康门控：选路门控

对照版本：CCH v0.9.5。

- 参照：CCH 的失败口径、全部不可用返回，以及管理端的徽章、筛选和重置。
- 没有采用：CCH 按供应商整体熔断的粒度。fork 的熔断主体是 `(渠道, 上游实际模型)`。
- 逐条的沿用与差异标注在[健康门控](../routing/feats/health-gate.md)选路门控的 D3、R1、R3、R6。

## 2 健康门控：会话归属与决策记录

- 会话绑定（`src/lib/session-manager.ts:1659-1757`）：
  - CCH 同样只在成功时写入。
  - CCH 故障转移成功即改绑，健康时迁回更高优先级供应商，无迁移迟滞。
  - fork 的 thread 主键（R1）与 R3–R5 的迁移迟滞是自有需求。
- 决策记录：参照 CCH 请求表的 `provider_chain`、`routing_trace` jsonb 列
  （`src/drizzle/schema.ts:571-575`）。
- Webhook：参照 CCH 的开闸告警。

规则编号见[健康门控](../routing/feats/health-gate.md)的会话归属与决策记录。

## 3 渠道强制 1 小时提示缓存

- 配置：CCH 在 Key 与供应商上配置 `cacheTtlPreference`（`inherit`、`5m`、`1h`，Key 优先）。
- 改写范围（`src/app/v1/_lib/proxy/forwarder.ts:774-850,3527-3538`）：
  - 只改写客户端已有的 `system[]` 与 `messages[].content[]` 顶层 `ephemeral` 断点。
  - 不改 `tools`、顶层 `cache_control` 与 `tool_result` 内嵌断点。
- 请求头：1h 时补 `extended-cache-ttl-2025-04-11` beta 头（`forwarder.ts:852-863,8828-8833`）。
- 计费：
  - 响应只有写入总数、没有 5m、1h 分档时，按请求偏好把总数计为 1h
    （`src/app/v1/_lib/proxy/response-handler.ts:6468-6485`）。
  - 另有供应商级计费互换开关 `swapCacheTtlBilling`（`response-handler.ts:6444-6451`）。

fork 没有照搬计费的两项。其余取舍见
[渠道强制 1 小时提示缓存](../channels/feats/force-one-hour-prompt-cache.md)的 D1–D6。

## 4 API Key 自助用量页

- 页面：CCH 用 Key 登录后进入独立只读页 `/my-usage`，页面有额度、按日期统计与逐条日志
  （`src/app/[locale]/my-usage/page.tsx:38-79`）。
- 隐藏的字段：日志顶层字段隐藏用户、Key、供应商名称和错误详情（`src/actions/my-usage.ts:90-110`）。
- 聚合范围：CCH 的额度与按模型统计会一并返回所属用户名下所有 Key 的聚合
  （`src/actions/my-usage.ts:337-360,1014-1049,1112-1121`）。

fork 只取页面形态，范围与口径见[API Key 自助用量页](../usage/feats/self-usage.md)的 D1–D13。

## 5 请求成本明细悬浮

- 管理端（`src/app/[locale]/dashboard/logs/_components/virtualized-logs-table.tsx:318-425,564-649`）：
  CCH 请求日志「成本」列悬浮「计费详情」。
  - 输入、输出、缓存写（5m、1h）、缓存读逐项列出落库金额。
  - 每项下方按「金额 × 1,000,000 ÷ Token 数」折算每百万 Token 单价。
  - 倍率不为 1 时先列基础合计与各倍率，再以删除线基础合计对比最终合计。
- Key 持有者：CCH `/my-usage` 复用同一表格，但后端置空分项与倍率，悬浮只剩合计
  （`src/actions/my-usage.ts:87-109,781-813`）。

fork 对 Key 持有者展示与管理端相同的内容，见
[请求成本明细悬浮](../billing/feats/request-cost-breakdown.md)的 D3。

## 未覆盖范围

- 只记录设计时对照过的点，不是对 CCH 的完整盘点。
- 第 2 至 5 节没有记录所对照的 CCH 版本。
