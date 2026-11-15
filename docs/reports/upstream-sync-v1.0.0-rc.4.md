# 上游同步记录：v1.0.0-rc.4（2026-11-15）

> Parent: [../index.md](../index.md)
> Audience: developer — 开发和演进项目的人或 agent
> Genre: survey — 当时基于什么信息做了什么判断
> Visibility: public — 他人可读，避免写入本机环境
> Maintain: frozen 2026-11-15 — 已定稿；内容与定稿日期不改，内容变化只追加 Outdated / Superseded 段，元数据可维护，定位用的路径与链接可随文件移动更新到原对象与版本的新位置

## 结论

- 上游 `v1.0.0-rc.3` → `v1.0.0-rc.4` 的 12 个提交以 merge 合入，merge 提交为 `7d9536f5`。
- 逐项核对 21 项 fork 改动，全部保留为 fork 差异。
- 上游给请求列表加了可配置的指标配色（`20970e0c`，PR #2883）。
  fork 的共享单元格按上游接入，管理端请求列表与自助用量页的读缓存命中率、首字耗时按同一份设置着色，
  提交 `699a7d8f`，见「逐项核对」第 14、18 项。
- fork 功能可观察的变化见「合入后 fork 功能的行为变化」。
- 发现 1 项由合入引入的问题，已修：fork 的一个计费测试仍按查库次数断言模型库查询的共享，上游改为经缓存读取后失败。
- 没有 schema migration 与 data migration。

## 合入

- 冲突：4 个文件。
  - `.github/workflows/docker-publish.yml`：保留 fork 的整份文件，见第 19 项。
  - `frontend/src/features/requests/components/requests-columns.tsx`：
    - Token、读缓存、耗时列保留 fork 的共享单元格。上游在读缓存与耗时列加的配色在 `699a7d8f` 接入共享单元格。
    - 上游的 `tokensColumnCacheDisplay` 随 fork 一侧删去：fork 的 Token 列不显示缓存率，
      见[上游 v1.0.0-beta12 同步记录](upstream-sync-v1.0.0-beta12.md)「逐项核对」第 18 项。
      上游在 Token 列缓存率上加的配色因此没有落点。
    - 导入取两侧所需。吞吐列的配色与总吞吐行自动合入。
  - `internal/server/api/responses_protocol_integration_test.go`：fork 一侧删去了对「回退到渠道」「查询所有渠道模型」的设置，
    这两项开关已随[模型准入](../routing/feats/model-admission.md)删除。保留 fork 一侧，接上游新增的 `ModelCache` 参数。
  - `internal/server/gql/generated.go`：生成物，按合并后的 schema 重新生成。
- 两侧都改过、Git 自动合并的文件：
  - `Dockerfile`：两侧改动相同，fork 在 rc.3 同步时回补了上游 `c42d57de` 的这一行。
  - `internal/server/biz/channel_price.go`：fork 的模型库兜底倍率与上游改为经缓存读取模型的价格。
    fork 在价格的副本上设倍率，不改缓存中的实体。
  - `internal/server/biz/model.go`：fork 的模型准入与上游的缓存读取改动不相交。
  - `internal/server/biz/channel.go`、`internal/server/biz/fx_module.go`：fork 的健康门控字段、价格倍率校验、
    自助用量服务注册，上游的模型缓存注入。
  - `internal/server/biz/request.go`：fork 的会话归属缓存字段，上游的响应头随终态写入。
  - `internal/server/orchestrator/orchestrator.go`：fork 的健康门控，上游在失败写入时多传响应头。
  - `internal/server/routes.go`：fork 的 `/self-service` 路由组，上游删去的 `WithResponseHeaders` 中间件。
  - `internal/server/gql/system.graphql`：fork 的健康门控策略，上游重置额度的参数。
  - 请求与系统设置的 locale，上游为模型缓存改的测试构造。
- schema migration：无。`internal/ent/schema`、`internal/ent/migrate/migrations/` 没有变化。
- data migration：
  - 上游无新增 data migration，注册顺序不变。
  - fork 镜像版本为 `v1.0.0-rc.4-fork.1`。`internal/server/biz/version_test.go` 的
    `TestCompareVersionsOrdersNumericPrereleases` 已覆盖 `v1.0.0-rc.3-fork.1` < `v1.0.0-rc.4` < `v1.0.0-rc.4-fork.1`。

## 上游带来的行为变化

- 模型缓存（`2c3f67a3`，PR #2984）：
  - 选路按模型 ID 读取模型、模型库价格兜底改读进程内共享的模型缓存。
  - 模型写入提交后失效新旧键并发出通知，另每 30 秒按更新时间增量刷新。
  - 缓存中的实体只读，事务内的读取绕过缓存。
- 请求列表指标配色（`20970e0c`，PR #2883）：
  - 工具栏的「指标配色」为缓存命中率、首字耗时、吞吐、含首字的总吞吐各设开关与四个分界，分五档着色。
    设置存在浏览器的 localStorage。
  - 读缓存列去掉「输入不少于 40000 词元且命中率低于 80% 时标红」，改为按档着色。
  - 吞吐列增加第二行：按总耗时（含首字）计算的吞吐。
- Claude 额度（`4369a606`，PR #2985）：额度改从 OAuth 用量接口读取，额度弹出层列出持有的重置额度，可选定一项兑换。
  Claude CLI 的 User-Agent 与 Stainless 请求头升到当前版本。
- 渠道（`029e1eda`，PR #2986）：没有可用 API Key 时返回错误。此前会 panic，OAuth-only 渠道的端点探测会让进程退出。
- 请求记录（`6770e520`，PR #2993）：响应头随请求终态的更新一并写入，删去 `WithResponseHeaders` 中间件。
  同一提交把 `moejs` 升到 v0.1.0。
- Codex（`82442778`，PR #2994）：第三方 Codex 渠道的模型目录允许 `http://` 地址。
- 插件：非安全 HTTP 源上用 nanoid 生成 ID（`23893b15`，PR #2992），插件目录加入 day card（`37dc356f`，PR #2987）。
- 构建与发布（`c42d57de`，PR #2983）：
  - workflow 的 Go 升到 1.27.4。
  - 新增 `scripts/bump-go-version.sh` 与 `.agents/rules/go-version.md`：升级 Go 时一并改各 `go.mod`、
    `Dockerfile`、workflow 与开发文档中的版本。
  - 上游的 `docker-publish.yml` 加了手动触发，按输入的 tag 与分支发布。

## 逐项核对

| # | 改动 | 结果 | 状态 |
| --- | --- | --- | --- |
| 1 | [健康门控](../routing/feats/health-gate.md)：选路门控 | 选路读模型改经缓存，模型的负载均衡策略随之从缓存读取，模型改动提交后即失效。上游未改门控判定 | 保留 |
| 2 | [健康门控](../routing/feats/health-gate.md)：会话归属与决策记录 | `request.go`、`orchestrator.go` 中 fork 的改动与上游的响应头写入自动合并，互不相交 | 保留 |
| 3 | [模型准入](../routing/feats/model-admission.md) | 上游的 `GetModelByModelID` 改读缓存，状态不符按未找到处理。fork 删去的回退到渠道选路仍不存在，模型列表的准入仍查库 | 保留 |
| 4 | [渠道列表](../channels/feats/channel-list.md)：渠道 tab 按供应商聚合 | 上游未改相关代码 | 保留 |
| 5 | [渠道列表](../channels/feats/channel-list.md)：生图渠道拆分 | 上游未改 `channels-columns.tsx` | 保留 |
| 6 | [渠道列表](../channels/feats/channel-list.md)：排序字段为零值时分页错乱 | 上游仍锁定 ent/contrib `3625dcc2e035`，绕过仍需要 | 保留 |
| 7 | [渠道测试](../channels/feats/channel-test.md) | 上游未改 `server.go`、配置与渠道测试入口 | 保留 |
| 8 | [强制 1 小时提示缓存](../channels/feats/force-one-hour-prompt-cache.md) | 上游只升级了 Claude Code 请求头的版本号，未改缓存断点处理 | 保留 |
| 9 | [供应商模型目录](../channels/feats/model-catalog.md)：Provider 模型成本可为空 | 上游放开 Codex 模型目录的 `http://` 地址，未改目录的 schema | 保留 |
| 10 | [透传](../channels/feats/passthrough.md)：预读事件上限 | 上游未改透传 | 保留 |
| 11 | [透传](../channels/feats/passthrough.md)：渠道上游的错误体被重包装 | 上游未处理，仍为 fork 差异 | 保留 |
| 12 | [仪表盘分析](../usage/feats/analytics.md)：API 密钥费用分析 | 上游未改相关代码 | 保留 |
| 13 | [仪表盘分析](../usage/feats/analytics.md)：模型详情分析 | 上游未改相关代码 | 保留 |
| 14 | [自助用量页](../usage/feats/self-usage.md) | 请求日志与管理端共用单元格，读缓存命中率与首字耗时随之按上游的配色着色，提交 `699a7d8f`。配色在管理端请求列表的工具栏设置，同一浏览器的自助页读同一份设置 | 保留 |
| 15 | [渠道定价](../billing/feats/channel-pricing.md)：渠道标准价格引用与持久倍率 | 模型库兜底改经缓存读取价格，倍率仍在副本上设置。fork 计费测试的断言随之调整，见发现 1 | 保留 |
| 16 | [渠道定价](../billing/feats/channel-pricing.md)：上游模型价格时间戳修复回补 | data migration `v1.0.0-beta7-fork.1` 的注册位置不变 | 保留 |
| 17 | [模型定价阶梯](../billing/feats/model-pricing-tiers.md) | 上游未改相关代码 | 保留 |
| 18 | [请求成本悬浮](../billing/feats/request-cost-breakdown.md) | `requests-columns.tsx` 冲突，Token、读缓存、成本、耗时列保留共享组件。读缓存与耗时的配色接入共享组件，提交 `699a7d8f` | 保留 |
| 19 | [Docker 镜像发布到 fork 仓库](../deployment/feats/docker-publish.md) | `docker-publish.yml` 冲突，保留 fork 的整份文件，上游的手动触发未带入。`Dockerfile` 两侧相同 | 保留 |
| 20 | [渠道性能记录](../channels/feats/channel-performance.md) | 上游未改 `performance.go` | 保留 |
| 21 | [渠道列表](../channels/feats/channel-list.md)：行菜单打开弹窗后页面被锁住 | 上游未改 `channels-columns.tsx` | 保留 |

## 合入后 fork 功能的行为变化

| # | 行为 | 合入前 | 合入后 | 原因 |
| --- | --- | --- | --- | --- |
| 1 | 管理端请求列表与自助用量页，读缓存列的命中率 | 输入不少于 40000 词元且命中率低于 80% 时标红，其余为弱化色 | 按指标配色分五档着色，中间档为正文色加粗 | 上游 `20970e0c`，`699a7d8f` |
| 2 | 管理端请求列表与自助用量页，流式请求的首字耗时 | 不着色 | 按指标配色分五档着色 | 上游 `20970e0c`，`699a7d8f` |

## 发现

| # | 发现 | 来源 | 状态 |
| --- | --- | --- | --- |
| 1 | fork 的计费测试按查库次数断言模型库查询的共享 | 合入引入，merge 提交的主模块测试 | 已修 |

### 1. fork 的计费测试按查库次数断言模型库查询的共享

- 现象：`internal/server/biz/usage_cost_test.go` 的 `TestUsageCost_ModelLibraryMemoKeepsChannelMultipliersSeparate`
  断言两个渠道与费用注入共用一次模型查询（查库计数为 1），merge 提交上计数为 0。
- 原因：上游把模型库兜底改为读共享缓存，缓存在测试安装查询拦截器前已加载，请求内不再查库。
  计费金额与价格引用的断言均通过，产品行为不受影响。
- 修复：`0212f0ec`、`f87aa4b6`，测试改为：
  - 去掉查库计数。
  - 第一次费用注入后修改模型库价格，使共享缓存失效。同一请求的落库与另一渠道的计费仍用请求开始时的价格，新请求用新价格。
    在临时副本中关掉请求内备忘的复用，该测试失败。
  - 渠道倍率没有写进共享缓存中的模型价格，请求内备忘保留未乘倍率的标准价格。

## 验证

- Go 检查在 Docker（`golang:1.27.1`，按 `go.mod` 自动切换到 1.27.4）内运行：
  - merge 提交：
    - 重新生成 ent、gqlgen、`openapi`、`selfusage` 的代码，402 个受版本控制的文件与 merge 结果逐字节一致。
    - 主模块 `go test -tags=nomsgpack ./...`：45 个包通过，1 个包失败，见发现 1。干净的上游 rc.4 树 45 个包全部通过。
    - `llm` 模块 `go test ./...`：43 个包通过。
    - `cmd/axonhub-desktop` 模块的 `go vet ./...`、`go test ./...`：2 个包通过，上游干净树相同。
    - PostgreSQL 专用测试批次（`-run Postgres`，需 `AXONHUB_TEST_PG_DSN`）：8 个顶层测试通过。
  - 提交 `699a7d8f`：主模块 46 个包通过，3220 个顶层测试通过、10 个跳过。
  - 最终提交 `f87aa4b6` 只改了发现 1 的测试，该测试以 `-race -count=3` 运行通过。
  - 两次的 `go vet -tags=nomsgpack ./...` 都只报上游的 `internal/server/plugin/manager_test.go:908`、
    `internal/server/api/stream_lifecycle_test.go:179`、`:232`，干净的上游 rc.4 树上相同。
- 前端：
  - merge 提交：
    - `pnpm install --frozen-lockfile` 通过。
    - 单元测试 491 个，490 个通过，1 个跳过。
    - `pnpm build` 通过。
    - 应用工程的类型检查（`tsc --noEmit -p tsconfig.app.json`）：上游干净树 209 条诊断，merge 提交 191 条。
      按文件、错误码与完整消息比对，比上游多出 4 条文字不同的诊断，都对应上游同一位置的已有错误：
      2 条因依赖的安装路径不同、类型摘要截断不同，另 2 条因 fork 的字段或枚举使展开的类型不同。
    - 与 rc.3 同步后的代码相比没有实际新增。
  - 提交 `699a7d8f`（之后的提交未改前端）：单元测试与构建结果同上，类型检查 191 条，与 merge 提交相比只有安装路径的差异。
  - 46 份 locale 文件没有重复键。
- 真机验收：本机 Docker，用 rc.3 同步后升级过的数据卷（`system_version` 为 `v1.0.0-rc.3-fork.1`）复制出新卷，
  在新卷上升级到最终代码构建的镜像。SQLite 与 PostgreSQL 各一遍，渠道上游用本地 mock。
  - 结果：两种方言各 31 项通过。启动日志无迁移错误，`system_version` 抬到 `v1.0.0-rc.4-fork.1`，不执行 data migration。
  - 升级后两种方言仍能写入、读取并删除 `provider_type` 为 `deepseek` 的额度状态行。
- 管理端界面：在升级后的 PostgreSQL 实例上检查 64 项，全部通过，无控制台错误与未捕获异常。
  - rc.3 的 56 项结果不变。渠道行菜单的弹窗，按点击事件计时：
    - Cancel 后 268–276ms 点击行菜单按钮、471–500ms 点击菜单项重开同一行的弹窗：12 轮在 Cancel 后 903–1098ms 检查，
      弹窗仍打开，内容属于该行。
    - 617–635ms 点击行菜单按钮、818–843ms 点击菜单项，重开同一行或打开另一行的同一弹窗：各 12 轮，内容正确。
    - 约 100ms 时的 9 次行菜单按钮点击落在仍在淡出的遮罩上，与 rc.3 相同。
  - rc.4 新增，指标配色：
    - 默认设置下，三条构造的请求按命中率与首字耗时分为三档：

      | # | 档 | 命中率 | 首字耗时 | 计算后的颜色 |
      | --- | --- | --- | --- | --- |
      | 1 | 红 | 10% | 25s | `rgb(247, 149, 138)` |
      | 2 | 中间 | 85% | 7s | `rgb(41, 37, 36)`，正文色 |
      | 3 | 绿 | 96% | 2s | `rgb(112, 192, 135)` |

    - 同一请求在管理端请求列表与自助用量页的颜色相同。
    - 在配色对话框关闭指标后颜色消失。管理端改过的配色，自助用量页刷新后生效。
    - 吞吐列第二行显示 `Total 3 tok/s`、`Total 30 tok/s`、`Total 100 tok/s`。
  - 用构造的 Claude 额度状态，额度弹出层列出重置额度及各自的可用状态，未执行兑换。
- 模型缓存，端到端，在 PostgreSQL 实例上，不重启、不等 30 秒的增量刷新：
  - 经管理端 GraphQL 禁用模型，修改返回后立即发出的请求返回 422 `model not found`。重新启用后的下一个请求返回 200。
  - 渠道未给该模型设价、倍率为 2 时，按模型库价格计费为 `0.000612`。把模型库价格改为原来的 3 倍后，
    下一个请求计费为 `0.001836`。两条用量记录的价格引用都是 `model_library@2:<模型>`。
- 健康门控的额度兜底，按 rc.3 的设置重跑：
  - A 返回 500 时，请求在 A 失败后转到 B 并成功。
  - A 正常时请求只到 A，B 没有收到请求。

## 未覆盖范围

- 未运行 lint 与仓库 e2e 套件。
- 真机验收的渠道上游都是本地 mock，未连接真实供应商。
- Claude 重置额度的兑换、Codex 的 `http://` 模型目录，只有上游自带的单元测试覆盖。
- 模型缓存只在单实例下验证，未验证多实例的失效通知。
- 桌面应用只跑了它的模块测试，未打包。
