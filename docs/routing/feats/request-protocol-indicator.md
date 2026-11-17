# 请求列表的协议提示

> Parent: [../index.md](../index.md)
> Audience: developer — 开发和演进项目的人或 agent
> Genre: feat — 这个 feature 是什么、为什么值得、什么形态
> Visibility: public — 他人可读，避免写入本机环境
> Maintain: living — 持续更新，保持内容有效

请求列表的模型列展示客户端入站格式与最近一次执行记录的出站格式，便于核对 AxonHub 实际使用的渠道端点。

## 动机

旧提示用橙色标记所有格式差异，只显示出站格式。[上游提交 dfbe2259](https://github.com/looplj/axonhub/commit/dfbe22593ea33d62d1bc04d47a8e5d6c8b25d2bf)
排查缓存命中时，容易把 WebSocket 切到 HTTP/SSE 误读为请求体被重建、缓存已丢失。

## 形态

- **蓝色**表示 Responses 只切换传输方式，例如「Responses WebSocket → Responses HTTP/SSE」。箭头按入站到出站的顺序展示，反向切换同样用蓝色。
- **橙色**表示其它 API 格式变化，提示同样列出入站和出站。
- **绿色**表示两端格式一致，提示显示该格式。
- **灰色**表示缺少入站或出站格式。

## 边界

协议提示反映入站与出站格式。请求体透传和缓存命中各有独立指标。

## 实现与维护

- 代码见[请求列表的模型列](../../../frontend/src/features/requests/components/requests-columns.tsx#L163-L185)。
- 操作示例见 [Codex 渠道手册](../../manuals/configure-codex-http-sse.md#1-关闭渠道上游-websocket)。
- 浏览器验证覆盖 Responses 双向切换、跨 API 格式、格式一致和缺失，以及中英文提示和亮暗配色。
- 无 schema migration、data migration 或持久化 fixture 变更。
