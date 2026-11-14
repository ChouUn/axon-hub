# 渠道测试

> Parent: [../index.md](../index.md)
> Audience: developer — 开发和演进项目的人或 agent
> Genre: feat — 这个 feature 是什么、为什么值得、什么形态
> Visibility: public — 他人可读，避免写入本机环境
> Maintain: living — 持续更新，保持内容有效

渠道测试是上游已有的管理端功能：对一个渠道、它的单个 Key 或全部 Key 发一次真实请求，验证能否调通
（mutation `testChannel`、`testChannelAPIKey`、`testChannelAPIKeys`）。
本 fork 给它独立的超时上限，并让三个入口都不进请求事务。
生图渠道的测试请求见[渠道列表](channel-list.md)的「生图渠道拆分」。

## 动机

- 渠道测试经 `/admin/graphql` 触发，该路由挂 `server.request_timeout`（默认 30s）。
  正式 `/v1` 接口用 `server.llm_request_timeout`（默认 600s）。
- 生图渠道的上游约 40 秒返回时，测试在 30 秒被掐断并报 `context deadline exceeded`，渠道上游却已成功并计费。
- 慢聊天请求同样可触发，问题出在渠道测试入口的通用超时与事务处理，不限于生图渠道。
  聊天测试通常几秒即回，所以没有暴露。
- 上游的 GraphQL 事务中间件只按操作名 `TestChannel`、`TestChannelAPIKeys` 跳过事务。
  匿名操作与单 Key 测试仍被包进绑定请求 context 的事务，超时即回滚。

## 形态

- 配置 `server.channel_test_timeout`，默认 `180s`。
- 三个测试入口用 `xcontext.DetachWithTimeout` 脱离请求 deadline，以该值为上限。
- 测试脱离请求取消：控制台断开后，测试仍跑到结束或超时。
- `http.Server` 的 `WriteTimeout` 取 `request_timeout`、`llm_request_timeout` 与该值三者的最大值。
- 设为 `0` 时保持上游行为。
- GraphQL 事务中间件对三个 mutation 按字段跳过事务。

## 边界

- 正式接口不受影响。
- 反向代理（nginx、Cloudflare）的读超时需自行放宽。
- 系统设置「重试策略 → 非流式传输超时」若开启，仍会在更早时刻以 pipeline 自己的错误终止。

## 实现与维护

- 提交：
  - `321cfba5`（`fix(channels): 渠道测试改用独立的 channel_test_timeout`）
  - `ea465c71`（`fix(channels): 渠道测试 mutation 按字段跳过请求事务`）
- 代码：
  - `conf/conf.go`
  - `internal/server/config.go`
  - `internal/server/server.go`
  - `internal/server/orchestrator/tester.go`
  - `internal/server/gql/resolver.go`
  - `internal/server/gql/graphql.go`
- 迁移：无。
- 测试：
  - `internal/server/orchestrator/tester_timeout_test.go` 的 `TestTestChannelOutlivesRequestDeadline`：
    用本地慢响应 mock 复现，修复前报同样错误。
  - `internal/server/gql/graphql_skip_tx_test.go`。
  - 本机 Docker（`axonhub:local`，SQLite）对照验证，mock 的生图渠道上游 45 秒返回：
    - 默认配置下 `testChannel` 与 `testChannelAPIKey` 均 45 秒成功。
    - `channel_test_timeout=0` 时两者 30 秒失败并报同样错误。
- 同步上游：
  - `NewSchema`、`Dependencies` 与 `NewTestChannelOrchestrator` 多了一个参数，上游改动这些签名时需重接。
  - 上游为 GraphQL 路由或渠道测试引入自己的超时后，以上游为准并移除本配置。
