# 上游同步记录：v1.0.0-beta12（2026-11-15）

> Parent: [../index.md](../index.md)
> Audience: developer — 开发和演进项目的人或 agent
> Genre: survey — 当时基于什么信息做了什么判断
> Visibility: public — 他人可读，避免写入本机环境
> Maintain: frozen 2026-11-15 — 已定稿；内容与定稿日期不改，内容变化只追加 Outdated / Superseded 段，元数据可维护，定位用的路径与链接可随文件移动更新到原对象与版本的新位置

## 结论

- 上游 `v1.0.0-beta11` → `v1.0.0-beta12` 的 141 个提交以 merge 合入，merge 提交为 `574f09ec`。
- 逐项核对 19 项 fork 改动，全部保留为 fork 差异。
- 上游把交付确认纳入流式请求的完成，健康门控随之调整健康结果与会话归属的登记时点，见发现 1。
- fork 功能其余可观察的变化见「合入后 fork 功能的行为变化」。
- 发现 8 项问题：3 项由合入引入，3 项在上游代码中，2 项合入前已存在。7 项已修，发现 6 由用户决定不修。
- 以下取舍由用户决定：
  - 透传预读以上游实现为底，保留 4096 个事件的上限（逐项核对第 10 项）。
  - 模型库价格兜底乘渠道倍率（发现 2）。
  - 客户端断开之前已观察到的失败一律计入健康门控（发现 5）。
  - 性能记录的上游数据竞争不修（发现 6）。
- 未运行 lint 与仓库 e2e 套件。

## 合入

- 冲突：37 个文件。
  - 生成物不手工合并，按解决后的源文件重新生成：
    - `internal/ent/` 下的生成代码。
    - `internal/server/gql/generated.go`，以及 `openapi`、`selfusage` 两个 GraphQL 包的生成代码。
    - `frontend/src/routeTree.gen.ts`。
  - 两侧在同一结构里各加字段的文件取并集：
    - `internal/objects/channel.go`
    - `frontend/src/features/channels/data/schema.ts`、`frontend/src/features/channels/data/channels.ts`
    - `frontend/src/features/system/components/webhook-settings.tsx`
    - `frontend/src/locales/` 下的 `channels.json`、`system.json`
  - `.github/workflows/docker-publish.yml` 保留 fork 的整份文件，见「逐项核对」第 19 项。
  - 其余按「逐项核对」的结果手工拼接。
- 不在冲突清单但被 Git 并错位置的文件：`internal/server/gql/axonhub.graphql`。
  - fork 给 `ChannelSettingsInput` 加的 `primaryApiFormat`、`healthGateFailureThreshold`、`forceOneHourPromptCache`
    被并进了上游新增的 `UpdateChannelSettingsInput`。
  - 已移回 `ChannelSettingsInput`。
- 上游的文件与 fork 的文件重名：`frontend/src/features/analytics/utils/date-range.ts`。
  - 上游的内容放到 `utils/date-time-range.ts`，上游的 `utils/date-range.test.mjs` 改为指向它。
  - 原因见「逐项核对」第 12、13 项。
- schema migration：上游没有新增表或列。
  - `channels.type` 枚举新增 `cloudflare`。
  - `provider_quota_status.provider_type` 枚举删除 `wafer`。
  - `requests`、`threads`、`traces`、`usage_logs`、`data_storages` 的 Policy 改了 API Key 的授权规则，不涉及表结构。
- 启动清理：上游在建表迁移之前删除 `provider_quota_status` 中 `provider_type` 为 `wafer` 的行
  （`internal/server/db/removed_provider_quotas.go`，上游 `e06cf9eb`，PR #2745）。各数据库方言都执行。
- data migration：
  - 上游无新增 data migration。
  - 注册顺序不变：`v1.0.0-beta7` → `v1.0.0-beta7-fork.1` → `v1.0.0-beta8` → `v1.0.0-beta9`。
- 构建：
  - `go.mod` 与 `llm/go.mod` 的 Go 版本为 `1.26.4`。
  - `Dockerfile` 的构建镜像固定为 `node:26-alpine` 与 `golang:1.27.3-alpine3.24`。
  - 上游新增的 `.github/workflows/` 下的 `docker-build.yml`、`docker-pr.yml`、`vulncheck.yml` 与
    `.github/dependabot.yml` 原样进入。

## 上游带来的行为变化

- 流的终态、交付确认与用量结算分开（`3dcfa3ce`，PR #2659）：
  - 流式请求记为完成，要求结束事件已交付给客户端。
  - 渠道上游已生成完、客户端在结束事件交付前断开的请求不记为完成。
- Responses 协议新增 HTTP 与 WebSocket 端点（`94cd05b1`，PR #2656）。
- 同渠道重试：
  - 重试前刷新渠道凭证（`8262e95a`，PR #2545）。
  - 渠道可覆盖系统的同渠道重试次数（`090e542e`，PR #2660），上限 10（`07ce14d8`，PR #2663）。
  - 渠道可把 429 纳入同渠道重试（`c31867e0`，PR #2668）。
- 配额路由按上游实际模型判断额度（`f30e3c9b`，PR #2667）：同一渠道内只移除额度耗尽的模型。
- 渠道没有为模型设价时按模型库价格计费（`cc1c7e5d`，PR #2655）。
- 分析页按秒级时间范围筛选（`2851a192`，PR #2775）。
- 仪表盘降低聚合查询的负载（`532a8032`，PR #2755；`dc68318c`，PR #2780）。
- 渠道：
  - 多 Key 的选择策略可配置（`9c1ea3bf`，PR #2548）。
  - 渠道级响应超时（`9142eb6a`，PR #2630）。
  - 新增只改渠道设置的补丁接口 `updateChannelSettings`（`a14d1013`，PR #2699）。
    - 它的字段表不含 fork 的 `primaryApiFormat`、`healthGateFailureThreshold`、`forceOneHourPromptCache`，
      也不含上游自己后加的 `maxSingleChannelRetries`、`responseTimeout`。
    - 管理端写这些字段的入口是渠道编辑对话框，它同时提交渠道字段，走完整更新接口 `updateChannel`。
    - 直接调用补丁接口改这些字段会被 GraphQL 以未知字段拒绝。
  - 新增渠道类型 `cloudflare`（`0d82a8e4`，PR #2674）。
  - 渠道测试按模型协议指定的端点选择测试格式（`48984732`，PR #2685）。
- Anthropic 出站为由其他协议转换而来、不带缓存标记的请求开启自动缓存（`697b13a5`，PR #2697）。
- 请求列表的 Token 列显示缓存命中率（`605b828d`，PR #2661）。
- Webhook 新增供应商额度状态变化的三个事件（`677166b4`，PR #2828）。
- service account 类型的 API Key 按授予的 scope 鉴权，项目级授权只在所属项目内生效（`e5c904ef`，PR #2741）。
- 移除 Wafer 额度供应商（`e06cf9eb`，PR #2745）。
- 新增服务端的 Claude auto-mode 审查与相关系统设置（`03437bb1`，PR #2722；`8e467537`，PR #2777）。
- 备份恢复的上传上限提高（`b9415562`，PR #2625）。
- 请求体与响应体写入 jsonb 前去掉 NUL 转义（`f322df79`，PR #2779）。

## 逐项核对

| # | 改动 | 结果 | 状态 |
| --- | --- | --- | --- |
| 1 | [健康门控](../routing/feats/health-gate.md)：选路门控 | 随上游的流生命周期调整健康结果的登记时点，见发现 1。配额路由按上游实际模型过滤后再做门控判定 | 保留 |
| 2 | [健康门控](../routing/feats/health-gate.md)：会话归属与决策记录 | 流式请求的成功随上游包含交付确认。Webhook 事件选项与上游新增的额度事件并存 | 保留 |
| 3 | [模型准入](../routing/feats/model-admission.md) | 上游新增的 5 个测试文件用了未注册的模型，改为在测试里注册 | 保留 |
| 4 | [渠道列表](../channels/feats/channel-list.md)：渠道 tab 按供应商聚合 | 上游为新类型 `cloudflare` 自带供应商映射 | 保留 |
| 5 | [渠道列表](../channels/feats/channel-list.md)：生图渠道拆分 | 上游扩展了测试格式的选择顺序，生图标记的判断仍排在最前 | 保留 |
| 6 | [渠道列表](../channels/feats/channel-list.md)：排序字段为零值时分页错乱 | 上游仍锁定 ent/contrib `3625dcc2e035`，绕过仍需要 | 保留 |
| 7 | [渠道测试](../channels/feats/channel-test.md) | 独立超时与按字段跳过事务保留 | 保留 |
| 8 | [强制 1 小时提示缓存](../channels/feats/force-one-hour-prompt-cache.md) | 上游在出站中间件里加了 Responses 的传输适配与续接，本中间件仍在请求体透传之后、渠道请求体覆盖之前。上游的自动缓存断点在开关开启时同样为 1 小时 | 保留 |
| 9 | [供应商模型目录](../channels/feats/model-catalog.md)：Provider 模型成本可为空 | 上游未改相关 schema | 保留 |
| 10 | [透传](../channels/feats/passthrough.md)：预读队列循环等待 | 上游 `94cd05b1` 重写了预读暂存，字节上限升到 64 MiB。改以上游实现为底，保留 4096 个事件的上限与超限后拒绝接入 | 保留 |
| 11 | [透传](../channels/feats/passthrough.md)：渠道上游的错误体被重包装 | 上游未处理，仍为 fork 差异 | 保留 |
| 12 | [仪表盘分析](../usage/feats/analytics.md)：API 密钥费用分析 | 与上游分析页共用的筛选随上游改为时间戳。前端把系统时区的自然日换算成时间戳，口径不变 | 保留 |
| 13 | [仪表盘分析](../usage/feats/analytics.md)：模型详情分析 | 自有的筛选不变，仍按系统时区的自然日 | 保留 |
| 14 | [自助用量页](../usage/feats/self-usage.md) | 上游改了 API Key 的授权规则。自助页用自己的认证入口与查询，未改 | 保留 |
| 15 | [渠道定价](../billing/feats/channel-pricing.md)：渠道标准价格引用与持久倍率 | 上游新增的模型库价格兜底不乘渠道倍率，改为乘倍率，见发现 2 | 保留 |
| 16 | [渠道定价](../billing/feats/channel-pricing.md)：上游模型价格时间戳修复回补 | data migration `v1.0.0-beta7-fork.1` 的注册位置不变 | 保留 |
| 17 | [模型定价阶梯](../billing/feats/model-pricing-tiers.md) | 上游的兜底从模型库读价格，读到的是 fork 的完整价格，含阶梯与缓存变体 | 保留 |
| 18 | [请求成本悬浮](../billing/feats/request-cost-breakdown.md) | 上游在 Token 列显示缓存命中率。保留 fork 的共享单元格与独立的读缓存列，不采用上游的显示 | 保留 |
| 19 | [Docker 镜像发布到 fork 仓库](../deployment/feats/docker-publish.md) | 上游把构建矩阵移到可复用的 `docker-build.yml`，镜像名写在其中。保留 fork 的整份 `docker-publish.yml` | 保留 |

## 合入后 fork 功能的行为变化

下表是合入后可观察到变化的 fork 功能行为。第 1、2 行跟随上游的语义变化，第 3–5 行是上游新行为接入 fork 功能的结果，第 6 行是复审时统一的口径。

| # | 行为 | 合入前 | 合入后 | 原因 |
| --- | --- | --- | --- | --- |
| 1 | 健康门控下流式请求的会话归属 | 生成完成即写入归属 | 结束事件交付给客户端后才写入。生成完成但客户端在交付前断开时，健康结果计数成功，归属、保护窗口与转移计数不变 | 上游 `3dcfa3ce` 把交付确认纳入请求完成 |
| 2 | 健康门控下额度耗尽的判断 | 按渠道移除 | 按上游实际模型移除，同渠道其他模型仍参与门控判定 | 上游 `f30e3c9b` |
| 3 | 渠道未设价的模型的成本 | 不计费 | 按模型库价格乘渠道倍率计费，请求列表的成本单元格显示来源标记 | 上游 `cc1c7e5d` 加了兜底，fork 让它乘倍率 |
| 4 | 强制 1 小时提示缓存开启时，由其他协议转换而来、不带缓存标记的请求 | 不改写 | 带上上游加的顶层自动缓存，TTL 为 1 小时 | 上游 `697b13a5` |
| 5 | 自助用量页的未定价记录数 | 渠道未设价的用量都算未定价 | 模型库有价格的算已定价 | 同第 3 行 |
| 6 | 健康门控下，上游已返回失败、客户端随后断开 | 流式在响应开始后计入，提交前与非流式不计入 | 一律计入。最后一试原样返回取消错误，不改写为 503 | 发现 5，用户决定统一 |

## 发现

| # | 发现 | 来源 | 状态 |
| --- | --- | --- | --- |
| 1 | 健康门控把正常完成的流式请求记为不稳，不写会话归属。AxonHub 收尾时取消执行 context 被当成客户端取消 | 合入引入，合入后的测试发现 | 已修 |
| 2 | 模型库价格兜底不乘渠道倍率 | 合入引入，盘点上游改动时发现 | 已修 |
| 3 | 流式透传在交付前关闭时，读取流状态与后台转换协程的写入构成数据竞争 | 上游代码，修发现 1 时的 `-race` 检查发现 | 已修 |
| 4 | 流式完成的性能记录入队后被失败标记改写，构成数据竞争 | 上游代码，修发现 1 时的 `-race` 检查发现 | 已修 |
| 5 | 提交前与非流式请求中，客户端断开之前已观察到的失败被改记为不计入 | 合入前已存在，代码审查发现 | 已修 |
| 6 | 流式请求在客户端断开后收尾时，性能记录的写入与入队、指标汇总之间存在数据竞争 | 上游代码，修发现 5 时的 `-race` 检查发现 | 未修，用户决定 |
| 7 | 渠道行菜单打开价格弹窗后，关闭弹窗页面被锁住 | 合入引入，管理端界面检查发现 | 已修 |
| 8 | 渠道页弹窗关闭后 500ms 内重新打开，弹窗不出现且页面被锁住 | 合入前已存在，排查发现 7 时发现 | 已修 |

### 1. 健康门控的流式结果与上游的流生命周期错位

- 触发：`health-gated` 策略下的流式请求，透传关闭时最明显。
- 原因：上游 `3dcfa3ce` 把流的终态、交付确认与用量结算分开（`internal/server/orchestrator/inbound.go`、`stream_lifecycle.go`）。
  - `StreamCompleted` 只在 `InboundPersistentStream.Close` 并入交付确认之后才为真。
  - 健康门控原先在内层流关闭时读取它，这时它还是假，于是把正常完成记为不稳，归属不写入。
  - 上游的流生命周期在收尾时主动取消执行 context。健康门控按 context 已取消判为客户端取消：
    - 上游 500 被记为不计入。
    - 最后一试的 500 没有改写为通用 503。
- 证据：
  - 合入后主模块测试中健康门控的两个用例失败：
    - `TestHealthGateStream_PassThroughHTTPCompletionCommitsOwnerAfterHandlerReturn`
    - `TestHealthGateOrchestrator_LastResortErrorContract`
  - 新增的 `TestHealthGateOrchestrator_StreamCompletionCommitsAfterInboundClose` 经真实的 `Process`、交付与关闭，
    修复前健康状态为不稳。
- 修复：`4fb761b3`。
  - 流式请求在返回给客户端的流关闭、透传的后台转换结束之后登记健康结果并提交会话。
  - 健康结果按渠道上游的证据分类，会话只看最终的请求完成。
  - 客户端取消改读下游连接与服务关闭的 context。
  - 规则见[健康门控](../routing/feats/health-gate.md)选路门控 R4、「取消与收尾」与会话归属 R2。
- 测试：`health_gate_lifecycle_test.go`，并按上游的交付确认改写了上面第一个用例的 fixture，断言不变。

### 2. 模型库价格兜底不乘渠道倍率

- 触发：渠道没有为模型设价，模型库有该模型的价格。
- 原因：上游 `cc1c7e5d` 的兜底直接用模型库价格计费。fork 的渠道倍率作用于标准价格，兜底未经过它。
- 处理：兜底乘渠道当前的倍率，倍率随用量记录的价格引用保存，成本悬浮显示倍率，请求列表显示价格来源标记。
- 修复：`55ca6e74`。规则见[渠道定价](../billing/feats/channel-pricing.md)与[请求成本明细悬浮](../billing/feats/request-cost-breakdown.md)。

### 3. 流式透传关闭时的数据竞争

- 位置：`passThroughDeliveryStream.Close` 先关闭原入站流再等后台转换协程。
  `InboundPersistentStream.Close` 的注释写明要先让后台协程停下再读共享状态，但实际不等它退出。
- 触发：流式透传，客户端在结束事件交付前断开，或交付后等待后台协程超时。
- 证据：不依赖健康门控的回归测试在修复前 `-race` 报竞争。
- 修复：`aae3f464`，见[透传](../channels/feats/passthrough.md)。

### 4. 流式完成的性能记录被改写

- 位置：`enqueueCompletedPerformance` 把请求持有的记录指针入队，提交前的错误回调随后改写同一条记录。
- 触发：渠道上游报告完成，空响应检测仍命中。
- 证据：`TestHealthGateOrchestrator_EmptyResponseDetectionPrecedesProviderTerminal` 在修复前 `-race` 报竞争。
- 修复：`46f2da5c`，见[渠道性能记录](../channels/feats/channel-performance.md)。

### 5. 客户端断开之前已观察到的失败被改记为不计入

- 触发：`health-gated` 策略下，上游返回失败（失败终态、空响应检测、5xx 等）之后、AxonHub 收尾之前，客户端断开。
  流式请求在响应开始写给客户端之前，以及非流式请求。
- 原因：收尾时用已取消的 context 重新分类最终错误，结果为不计入。
  - 响应开始之后的流式路径按 beta11 的修复（`8c8b9a6b`、`bfe76e5f`）已经保留上游失败，两条路径口径不一。
  - 健康门控最初的提交 `6298a0cb` 有一条测试，按不计入断言了非流式最后一试的这一情形。
- 证据：发现 1 修复中新增的 `TestHealthGateOrchestrator_PreCommitPassThroughFailureSurvivesCancellation`
  把这个结果断言成了正确，代码审查指出它与已有规则矛盾。修复前该用例的连续失败为 0。
- 处理：统一为客户端断开之前已观察到的失败照常计入，流式与非流式相同。最后一试仍返回原取消错误。
  这一取舍由用户决定，`6298a0cb` 那条测试的健康断言随之改为计失败。
- 修复：`9ff6b55a`。规则见[健康门控](../routing/feats/health-gate.md)选路门控 D3 与 R6。

### 6. 性能记录在收尾时仍被后台协程写入

- 触发：流式请求经过转换管线的后台协程（透传，或 Responses 同格式请求），客户端在结束事件交付前断开。
- 原因：处理函数关闭流时，`recordPerformanceStream.Close` 先读取性能记录并入队，之后才等后台协程退出。
  - 后台协程此时仍可能调用 `MarkSuccess`、`MarkFirstToken` 写同一条记录。
  - 指标 worker 读取记录里的首 token 时间，与这次写入之间没有同步。
- 归属：
  - 读取完成标志与 worker 读取首 token 时间这两处在上游 beta12 原样存在。
  - 发现 4 的修复把入队改为复制整条记录，复制本身是新的读取点，根因相同。
- 证据：`TestHealthGateOrchestrator_CompletedBeforeDeliveryCancellation` 在 `-race` 下间歇报出这三处。
  `4fb761b3` 上的同一 race 检查曾经通过，竞争是间歇出现的。
- 影响：渠道性能指标（延迟、首 token、吞吐）可能取到不完整的值，进而影响按延迟排序的负载均衡。
  计费金额与健康门控的结果不读这些字段，未受影响。实际偏差没有测量。
- 未修的理由：
  - 问题在上游代码里，只影响指标。
  - 修复需要给性能记录加锁并改 2–6 个上游文件，约 120–220 行，之后连续同步上游时冲突面大。

### 7、8. 渠道行菜单打开弹窗后页面被锁住

- 触发：渠道列表的行菜单打开价格弹窗，Cancel 后再打开一次并 Cancel，随后点击页面无反应，`body` 留着 `pointer-events: none`。
- 对照实测：同一浏览器、同一 PostgreSQL 数据副本，Cancel 后约 0.1 秒重新打开，每轮刷新页面。

  | 入口 | 合入前镜像 | 合入后镜像 |
  | --- | --- | --- |
  | 价格 | 0/10 | 10/10 |
  | 模型别名、覆盖、代理 | 各 3/3 | 各 3/3 |

- 发现 7 的原因：上游 `7313fbc9` 升级依赖后，菜单与弹窗依赖两份不同版本的 `@radix-ui/react-dismissable-layer`，
  各自记录 `body` 的原值，弹窗把菜单写入的 `none` 当成原值并在关闭时恢复。
  - 版本来自各提交的 lockfile。
  - 具体的写入顺序没有抓到运行时 trace，由源码推断。
- 发现 8 的原因：上游的弹窗关闭后 500ms 清空当前行。重新打开时旧的延时仍会触发，清掉新弹窗的当前行。
  其他三个入口在合入前镜像上同样触发，依据是这一条。
- 修复：`d234b0eb`（行菜单非模态）、`a4e9787b`（延时清理可取消），见[渠道列表](../channels/feats/channel-list.md)。

## 验证

- Go 编译与测试在 Docker（`golang:1.27.1`）内运行，下列结果均在最终代码上：
  - 主模块 `go test -tags=nomsgpack ./...`：43 个包通过。
  - `llm` 模块 `go test ./...`：43 个包通过。merge 之后的提交未改 `llm/`。
  - PostgreSQL 专用测试（需 `AXONHUB_TEST_PG_DSN`）通过：biz、gql、gql/qb、gc、db。
  - `go vet -tags=nomsgpack ./...` 只报上游的 `internal/server/api/stream_lifecycle_test.go:179`、`:232`。
  - `-race -run 'HealthGate|PassThrough'`：发现 3、4 修复后通过。发现 5 修复后间歇报出发现 6。
- 合入时为适配 fork 改动了上游测试的期望，产品代码不变：
  - 上游新增的 5 个测试文件补注册模型，见「逐项核对」第 3 项。
  - `pass_through_budget_test.go`：事件数超限的用例改用 fork 的上限 `maxRawPreAttachEvents`，见第 10 项。
- 前端：
  - 单元测试 433 个，432 个通过，1 个跳过。merge 提交为 419 个，418 个通过，1 个跳过。
  - 应用工程的类型检查（`tsc --noEmit -p tsconfig.app.json`）：上游干净树 204 条诊断，合入后 186 条。
    - 按文件、错误码与完整消息比对，有 6 条文字不同。逐项核对后，都是上游已有的错误，差异来自类型展开、截断与属性数量。
    - 未发现合入新增的类型错误。最终代码与 merge 提交的诊断集合完全一致。
- 真机验收：本机 Docker，用 beta11 同步后的镜像（`system_version` 为 `v1.0.0-beta11-fork.1`）建好数据卷，
  在同一数据卷上升级到 `a4e9787b` 构建的镜像。SQLite 与 PostgreSQL 各一遍，渠道上游用本地 mock。
  - 结果：两种方言各 21 项通过，1 项跳过。跳过的是 Webhook 事件选项，由管理端界面检查覆盖。
  - 核对内容：
    - 启动日志无迁移错误，`system_version` 抬到 `v1.0.0-beta12-fork.1`，重启不重复执行 data migration。
    - 各表行数与升级前从数据卷直接读出的一致，`requests.routing_decision` 的列与历史 JSON 保留。
    - fork 的渠道设置保留，经 `updateChannel` 与上游的 `updateChannelSettings` 修改渠道后仍保留。
    - `provider_quota_status` 中 `wafer` 的行删除，其他供应商的额度状态保留。
    - 健康门控：
      - 先失败后成功的非流式请求计成功。
      - 流式请求在透传开、关与 Anthropic 协议下正常完成，计成功并写入归属。
      - 连续三次 5xx 后熔断。
      - 流中断记为不稳，不计失败。
    - 模型库价格兜底：渠道倍率为 0.5、2 与未设时按倍率计费，引用中保存倍率。渠道和模型库都没有价格时不计费。
    - 透传保留客户端字段，生图渠道的测试请求走图像生成端点。
    - 共享分析接受秒级时间范围，两张 fork 分析表、自助用量页可查询，自助用量页只返回本 Key 的用量。
    - 渠道支持但未注册的模型返回 422。
- 管理端界面：在升级后的 PostgreSQL 实例上检查 27 项，全部通过，无控制台错误与未捕获异常。
  - 渠道列表的 tab 分组、筛选、额度显示、健康门控弹窗、渠道编辑、价格窗口、`cloudflare` 类型选项。
  - 系统设置的健康状态与 Webhook 事件选项。
  - 两张 fork 分析表与上游的共享分析页。
  - 请求列表与详情：显式价格与兜底价格的成本悬浮、来源标记、透传与直连的流式请求、中断与 5xx 的请求。
  - 自助用量页。
  - 渠道行菜单的弹窗（发现 7、8）：价格 10 轮，模型别名、覆盖、代理各 3 轮，Cancel 后约 0.1 秒重新打开，
    均未锁页，第二次打开的弹窗内容正确。用 Esc 关闭弹窗、点击空白处关闭行菜单各 3 轮，也未锁页。
  - 额外观察：Esc 关闭后约 0.1 秒重新打开，并在弹窗出现后 7–12ms 内再按 Esc，3 轮弹窗都没有关闭。
    - 等待后再按 Esc 或点 Cancel 都能关闭，页面没有锁住。
    - 弹窗出现后 141ms 与约 500ms 按下的 Esc 正常关闭。
    - 由 DOM 推断，Esc 被仍在退出动画中的行菜单处理。未与合入前的镜像对比，未处理。

## 未覆盖范围

- 未运行 lint 与仓库 e2e 套件。
- 真机验收的渠道上游都是本地 mock，未连接真实供应商。
- 真机验收未覆盖的上游新功能：
  - Responses 协议的 HTTP 与 WebSocket 端点。
  - 同渠道重试的凭证刷新、渠道级重试次数与 429 重试。
  - 多 Key 选择策略与渠道级响应超时。
  - `cloudflare` 渠道的实际请求。管理端只看了渠道类型选项。
  - Claude auto-mode 审查（`server.safeguards`）、service account 的 scope 鉴权、备份恢复的上传上限。
- 渠道弹窗的两处修复没有自动化测试，只由浏览器实测覆盖。
- 代码审查未逐行覆盖全部生成代码、各供应商转换器的上游内部改动和全部 locale 键。
