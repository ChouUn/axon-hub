# 渠道性能记录

> Parent: [../index.md](../index.md)
> Audience: developer — 开发和演进项目的人或 agent
> Genre: feat — 这个 feature 是什么、为什么值得、什么形态
> Visibility: public — 他人可读，避免写入本机环境
> Maintain: living — 持续更新，保持内容有效

渠道性能记录是上游已有的机制：每次发往渠道上游的尝试结束时，把成功与否、延迟等记成一条性能记录，
异步交给渠道指标（`internal/server/biz/channel_metrics.go`）汇总，负载均衡评分与渠道列表读取这些指标。
本 fork 修正了其中一处：流式完成的记录被之后的失败标记改写。

## 流式完成的记录被之后的失败标记改写

### 动机

- 上游在流式请求生成完成时，把请求持有的性能记录指针交给异步队列。
- 提交前的错误回调（如空响应检测命中）随后把同一条记录标为失败，再交一次。
- 队列的消费者读第一条时与这次改写构成数据竞争，第一条可能读成失败。

### 形态

- 流式完成时交给队列的是记录的副本。
- 两次入队的内容各自固定：第一条为成功，第二条为失败。入队次数不变。

### 边界

- 同类问题：流式请求在客户端断开后收尾时，入队时的复制与后台转换协程对同一条记录的写入竞争，
  见[上游 v1.0.0-beta12 同步记录](../../reports/upstream-sync-v1.0.0-beta12.md)的发现 6。
  上游 `v1.0.0-rc1` 去掉后台转换协程（`4af420ca`）后，写入与入队在同一协程里先后进行，这处竞争不再存在，
  见[上游 v1.0.0-rc1 同步记录](../../reports/upstream-sync-v1.0.0-rc1.md)。
- 只改流式完成的入队点（`enqueueCompletedPerformance`），其他入队点不变。
- 同一次尝试先记成功、再记失败，是上游的口径，本 fork 未改。

### 实现与维护

- 提交：`46f2da5c`（`fix(orchestrator): 流式完成的性能记录按快照入队`）。
- 代码：`internal/server/orchestrator/performance.go`。
- 迁移：无。
- 测试：`internal/server/orchestrator/health_gate_lifecycle_test.go` 的
  `TestHealthGateOrchestrator_EmptyResponseDetectionPrecedesProviderTerminal`。
  - 修复前 `-race` 报竞争。
  - 断言渠道指标的成功与失败各一次。
  - 无新增 fixture。
- 同步上游：
  - 上游自行修复后，以上游为准。
  - 上游 `v1.0.0-rc1` 之后仍要复制：经 channel 交给指标 worker 只保证发送前的写入可见，挡不住发送后错误回调对原记录的改写。
