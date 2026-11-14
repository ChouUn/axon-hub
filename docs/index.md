# AxonHub Fork 文档

> Parent: 无
> Audience: developer — 开发和演进项目的人或 agent
> Genre: index — 目录页
> Visibility: public — 他人可读，避免写入本机环境
> Maintain: living — 持续更新，保持内容有效

本 fork 相对[上游 AxonHub](https://github.com/looplj/axonhub) 的差异按模块记录。
`zh/`、`en/`、`solutions/` 等目录沿用上游结构，不在本索引内。

用语：

- 「上游」单独出现时指上游 AxonHub。
- 「渠道上游」指渠道指向的供应商或中转服务。

## 模块

- [路由](routing/index.md)：从用户到 AxonHub，包括模型准入、负载均衡策略和会话归属。
- [渠道](channels/index.md)：从 AxonHub 到渠道上游，包括渠道的配置、列表与测试，发往渠道上游的请求处理，以及供应商模型目录。
- [用量与分析](usage/index.md)：仪表盘分析和 API Key 自助用量页。
- [计费](billing/index.md)：模型与渠道的价格、倍率，以及请求成本的展示。
- [部署](deployment/index.md)：镜像构建与发布。

## 上游同步

- [上游 v1.0.0-beta10 同步记录](reports/upstream-sync-v1.0.0-beta10.md)
- [上游 v1.0.0-beta11 同步记录](reports/upstream-sync-v1.0.0-beta11.md)

## 外部项目对照

- [CCH 对照与取舍](reports/cch-comparison.md)：健康门控、1 小时提示缓存、自助用量页和成本悬浮对照 claude-code-hub 时的取舍。
