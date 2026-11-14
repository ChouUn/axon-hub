# 渠道测试

> Parent: [../index.md](../index.md)
> Audience: developer — 开发和演进项目的人或 agent
> Genre: feat — 这个 feature 是什么、为什么值得、什么形态
> Visibility: public — 他人可读，避免写入本机环境
> Maintain: living — 持续更新，保持内容有效

## 渠道测试受后台请求超时限制

- 归属：上游已有渠道测试入口的通用超时与事务问题，慢聊天请求同样可触发；并非只属于
  fork 生图渠道拆分功能。
- 问题：渠道测试经 `/admin/graphql` 触发，该路由挂 `server.request_timeout`
  （默认 30s），正式 `/v1` 接口则用 `server.llm_request_timeout`（默认 600s）。
  生图上游约 40 秒返回时，测试在 30 秒被掐断并报 `context deadline exceeded`，
  上游却已成功并计费。聊天测试几秒即回，此前未暴露。
- 行为：新增配置 `server.channel_test_timeout`（默认 `180s`）。单个测试、单 key 测试与
  批量 key 测试入口用 `xcontext.DetachWithTimeout` 脱离请求 deadline，改以该值为上限；
  `http.Server` 的 `WriteTimeout` 取 `request_timeout`、`llm_request_timeout` 与该值的
  最大值。设为 `0` 保持原行为。测试脱离请求取消后，控制台断开也会跑到结束或超时。
  GraphQL 事务中间件对这三个 mutation 改为按字段跳过事务（上游只按操作名跳过前两个，
  匿名操作与单 key 测试仍被包进绑定请求 context 的事务，超时即回滚）。
- 边界：正式接口不受影响；反向代理（nginx、Cloudflare）的读超时需自行放宽。系统设置
  「重试策略 → 非流式传输超时」若开启，仍会在更早时刻以 pipeline 自己的错误终止。
- 提交：`321cfba5`（`fix(channels): 渠道测试改用独立的 channel_test_timeout`）；
  `ea465c71`（`fix(channels): 渠道测试 mutation 按字段跳过请求事务`）。
- 代码：`conf/conf.go`、`internal/server/config.go`、`internal/server/server.go`、
  `internal/server/orchestrator/tester.go`、`internal/server/gql/resolver.go`、
  `internal/server/gql/graphql.go`。
- 迁移与测试：无迁移；`internal/server/orchestrator/tester_timeout_test.go`
  （`TestTestChannelOutlivesRequestDeadline`，本地慢响应 mock 复现，修复前报同样错误）；
  `internal/server/gql/graphql_skip_tx_test.go`。本机 Docker（`axonhub:local`，SQLite）
  对照验证：mock 生图上游 45 秒返回，默认配置下 `testChannel` 与 `testChannelAPIKey`
  均 45 秒成功；`channel_test_timeout=0` 时两者 30 秒失败并报同样错误。
- 同步上游注意：`NewSchema`、`Dependencies` 与 `NewTestChannelOrchestrator` 多了一个
  参数，上游改动这些签名时需重接；若上游为 GraphQL 路由或渠道测试引入自己的超时，
  以上游为准并移除本配置。
