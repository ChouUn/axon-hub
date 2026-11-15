# 上游同步记录：v1.0.0-rc.3（2026-11-15）

> Parent: [../index.md](../index.md)
> Audience: developer — 开发和演进项目的人或 agent
> Genre: survey — 当时基于什么信息做了什么判断
> Visibility: public — 他人可读，避免写入本机环境
> Maintain: frozen 2026-11-15 — 已定稿；内容与定稿日期不改，内容变化只追加 Outdated / Superseded 段，元数据可维护，定位用的路径与链接可随文件移动更新到原对象与版本的新位置

## 结论

- 上游 `v1.0.0-rc2` → `v1.0.0-rc.3` 的 10 个提交以 merge 合入，merge 提交为 `81b77b12`。
- 逐项核对 21 项 fork 改动，全部保留为 fork 差异。
- 两项由用户决定跟随上游：
  - 上游给配额路由的「背压」加了兜底渠道（`8529df6a`，PR #2974），健康门控一并接入，见「逐项核对」第 1 项。
  - 上游把 zh-CN 的「Token」「令牌」统一为「词元」（`8c677814`，PR #2980），fork 自有的 14 条文案一并改为「词元」，见第 12–14、17 项。
- fork 功能可观察的变化见「合入后 fork 功能的行为变化」。
- 发现 1 项由合入引入的问题，已修：上游 rc.3 的 `Dockerfile` 构建不出镜像。
- schema 只在额度状态表的供应商枚举中加了 `deepseek`，没有 data migration。

## 合入

- 冲突：3 个文件。
  - `frontend/src/locales/zh-CN/base.json`：计价模式的「每百万词元」「阶梯词元」取上游，fork 的「逐项全量阶梯（旧）」保留，见第 17 项。
  - `frontend/src/locales/zh-CN/models.json`：fork 一侧只统一过这段的缩进，取上游的「词元」文案并保留 fork 的缩进。
  - `internal/ent/internal/schema.go`：生成物，按合并后的 schema 重新生成。
- 编译适配：上游把 `QuotaRoutingGate.Filter` 改为返回首选与兜底两组。merge 提交中健康门控只取首选组，行为与 rc2 相同，兜底组在 `9092b950` 接入。
- 两侧都改过、Git 自动合并的文件：
  - `internal/server/orchestrator/candidates.go`：fork 删去了找不到模型时回退到渠道的选路并加了健康门控的入口，上游加了兜底渠道的追加，改动不相交。
  - `internal/server/routes.go`：fork 的 `/self-service` 路由组与上游的插件界面 CORS 跳过不相交。
  - `internal/server/biz/fx_module.go`：fork 注册自助用量服务，上游注册 trace 身份缓存的停止钩子。
  - `frontend/src/features/channels/components/channels-columns.tsx`：fork 的健康徽章、生图后缀与非模态行菜单，上游的额度余额行。
  - 额度相关的前端测试与 locale、`internal/ent/migrate/schema.go`、`internal/server/gql/ent.graphql`。
- schema migration：`provider_quota_status.provider_type` 的枚举加 `deepseek`。`internal/ent/migrate/migrations/` 没有变化。
- data migration：
  - 上游无新增 data migration。
  - 注册顺序不变：`v1.0.0-beta7` → `v1.0.0-beta7-fork.1` → `v1.0.0-beta8` → `v1.0.0-beta9`。
  - 上游 tag 从 `v1.0.0-rc2` 起改写为 `v1.0.0-rc.3`。`biz.CompareVersions` 把预发布号按点和数字段切分，两种写法都按数值比较。
  - fork 镜像版本为 `v1.0.0-rc.3-fork.1`。`internal/server/biz/version_test.go` 的 `TestCompareVersionsOrdersNumericPrereleases` 已覆盖
    `v1.0.0-rc2` < `v1.0.0-rc.3` < `v1.0.0-rc.3-fork.1` < `v1.0.0-rc.4`。

## 上游带来的行为变化

- 配额路由（`8529df6a`，PR #2974）：「背压」模式下，还有余量充足的候选时，用量偏高但未耗尽的渠道不再移除，留作兜底排在全部首选之后。
  兜底最多取渠道重试次数个，每个渠道一个，关闭重试时不追加。额度耗尽的渠道不作兜底。
- DeepSeek 额度（`35d3c27f`，PR #2973）：按渠道地址识别 DeepSeek，查询账户余额。渠道列表的额度列显示各币种余额，悬浮显示赠送与充值余额。
- 编排器：HTTP/2 的流重置（`INTERNAL_ERROR`、`REFUSED_STREAM`）与 GOAWAY 归为传输错误，按传输错误重试（`d8455a8d`，PR #2910）。
- Gemini：`toolUsePromptTokenCount` 计入提示词用量（`84533aba`，PR #2906）。
- trace 与 thread（`04aab1e6`，PR #2981）：中间件经缓存解析 trace 与 thread 的身份，请求上下文里只存 `ID`、`ProjectID`、`TraceID`、`ThreadID`。
- 管理端登录（`31d685ae`，PR #2977）：请求没带凭证，或页面在 `/oauth/` 下时，401 不再清会话并跳到登录页，OIDC 回调不被打断。
- 插件：插件界面的资源跳过带凭证的全局 CORS（`8f6b98c7`，PR #2979），插件页面可以复制和保存所显示的内容（`e70fb2a2`，PR #2982）。
- 桌面应用：面板可管理密钥、渠道与模型（`8c677814`，PR #2980）。同一提交把 zh-CN 的「Token」「令牌」统一为「词元」。
- 构建与发布：
  - 根 `go.mod` 升到 Go 1.27.4，`llm/go.mod` 升到 1.26.11。`Dockerfile` 的构建镜像没有随之升级，见发现 1。
  - `release.yml` 只构建一次前端并在 release 页发布下载表（`7def6e43`，PR #2975）。

## 逐项核对

| # | 改动 | 结果 | 状态 |
| --- | --- | --- | --- |
| 1 | [健康门控](../routing/feats/health-gate.md)：选路门控 | 上游给配额路由加了兜底渠道。用户决定健康门控跟随上游：兜底渠道与首选做同样的门控判定，健康的排在全部健康首选之后，没有健康首选时成为候选，见 D8，提交 `9092b950`。上游把 HTTP/2 流重置与 GOAWAY 归为传输错误，门控在读取响应体时遇到这类错误的计数随之改变，见「合入后 fork 功能的行为变化」第 2、3 行 | 保留 |
| 2 | [健康门控](../routing/feats/health-gate.md)：会话归属与决策记录 | 上游的 trace、thread 中间件改为只在上下文里存身份字段。会话归属只读 trace 的 `ID`、`ThreadID` 与 thread 的 `ID`，不受影响。熔断的兜底渠道记入决策记录的跳过项 | 保留 |
| 3 | [模型准入](../routing/feats/model-admission.md) | `candidates.go` 中 fork 删去的回退到渠道选路与上游新增的兜底追加不相交 | 保留 |
| 4 | [渠道列表](../channels/feats/channel-list.md)：渠道 tab 按供应商聚合 | 上游未改相关代码 | 保留 |
| 5 | [渠道列表](../channels/feats/channel-list.md)：生图渠道拆分 | `channels-columns.tsx` 中 fork 在供应商列加的生图后缀与上游的额度余额行自动合并 | 保留 |
| 6 | [渠道列表](../channels/feats/channel-list.md)：排序字段为零值时分页错乱 | 上游仍锁定 ent/contrib `3625dcc2e035`，绕过仍需要 | 保留 |
| 7 | [渠道测试](../channels/feats/channel-test.md) | 上游未改 `server.go`、配置与渠道测试入口 | 保留 |
| 8 | [强制 1 小时提示缓存](../channels/feats/force-one-hour-prompt-cache.md) | 上游未改 Anthropic 的缓存断点处理 | 保留 |
| 9 | [供应商模型目录](../channels/feats/model-catalog.md)：Provider 模型成本可为空 | 上游未改目录的 schema | 保留 |
| 10 | [透传](../channels/feats/passthrough.md)：预读事件上限 | 上游未改透传 | 保留 |
| 11 | [透传](../channels/feats/passthrough.md)：渠道上游的错误体被重包装 | 上游未处理，仍为 fork 差异 | 保留 |
| 12 | [仪表盘分析](../usage/feats/analytics.md)：API 密钥费用分析 | 说明与「词元数」列改用「词元」，提交 `6b164ba7` | 保留 |
| 13 | [仪表盘分析](../usage/feats/analytics.md)：模型详情分析 | 「词元数」「费用/百万词元」两列改用「词元」，提交 `6b164ba7` | 保留 |
| 14 | [自助用量页](../usage/feats/self-usage.md) | 上游的 401 处理只作用于管理端的查询缓存与 `graphqlRequest`，自助用量页用自有的查询客户端。fork 的自助页路由在合并结果中保留。汇总与趋势的 7 条文案改用「词元」，提交 `6b164ba7` | 保留 |
| 15 | [渠道定价](../billing/feats/channel-pricing.md)：渠道标准价格引用与持久倍率 | 上游未改价格对话框 | 保留 |
| 16 | [渠道定价](../billing/feats/channel-pricing.md)：上游模型价格时间戳修复回补 | data migration `v1.0.0-beta7-fork.1` 的注册位置不变 | 保留 |
| 17 | [模型定价阶梯](../billing/feats/model-pricing-tiers.md) | 计价模式文案冲突，fork 的「逐项全量阶梯（旧）」保留。统一全量阶梯的说明与校验提示改用「词元」，提交 `6b164ba7` | 保留 |
| 18 | [请求成本悬浮](../billing/feats/request-cost-breakdown.md) | 上游未改相关代码 | 保留 |
| 19 | [Docker 镜像发布到 fork 仓库](../deployment/feats/docker-publish.md) | 上游未改 `docker-publish.yml`，但 `Dockerfile` 构建不出镜像，见发现 1。`release.yml` 的改动不影响 fork：fork 仓库禁用了它 | 保留 |
| 20 | [渠道性能记录](../channels/feats/channel-performance.md) | 上游未改相关代码 | 保留 |
| 21 | [渠道列表](../channels/feats/channel-list.md)：行菜单打开弹窗后页面被锁住 | `channels-columns.tsx` 中行菜单的 `modal={false}` 在合并结果中保留 | 保留 |

## 合入后 fork 功能的行为变化

第 1–3 行是上游的新行为作用到 fork 功能上，第 4 行是用户决定的文案统一。
发送请求时（收到响应头之前）的 HTTP/2 错误以 `*url.Error` 返回，它满足 `net.Error`，合入前后门控都计数失败，不在表中。

| # | 行为 | 合入前 | 合入后 | 原因 |
| --- | --- | --- | --- | --- |
| 1 | 健康门控加「背压」，还有余量充足的候选时，用量偏高但未耗尽的渠道 | 新会话不可用。首选都不可用时进入最后一试 | 健康的排在全部健康首选之后。没有健康首选时成为候选，不进入最后一试 | 上游 `8529df6a`。用户决定跟随，`9092b950` |
| 2 | 健康门控下，读取非流式响应体时遇到 HTTP/2 流重置或 GOAWAY | 按代码推断：错误链里没有 `net.Error`，也没有状态码，不计入 | 计数失败 | 上游 `d8455a8d` |
| 3 | 健康门控下，流式请求没有渠道上游的终态，也没有生成完成的证据，保留下来的流错误是 HTTP/2 流重置或 GOAWAY，收尾分类时客户端已断开 | 按代码推断：按客户端取消处理，不计入 | 按流中断计：首个 token 前计数失败，之后记不稳 | 上游 `d8455a8d` |
| 4 | 中文界面中 fork 页面的用词 | 「Token」 | 「词元」 | 用户决定跟随上游 `8c677814`，`6b164ba7` |

## 发现

| # | 发现 | 来源 | 状态 |
| --- | --- | --- | --- |
| 1 | 上游 rc.3 的 `Dockerfile` 构建不出镜像 | 合入引入，真机验收构建镜像时发现 | 已修 |

### 1. 上游 rc.3 的 `Dockerfile` 构建不出镜像

- 原因：上游 rc.3 把根 `go.mod` 升到 Go 1.27.4，`Dockerfile` 的构建镜像仍是 `golang:1.27.3-alpine3.24`。
  官方 Go 镜像设了 `GOTOOLCHAIN=local`。`go build` 带了 `GOTOOLCHAIN=auto`，能自动下载工具链。之后的 `scripts/verify-go-sse-dependency.sh` 没带，因版本不足失败。
- 影响：fork 的 `docker-publish.yml` 经这份 `Dockerfile` 构建，rc.3 的 tag 发布不出镜像。
- 证据：在 merge 结果上按原样构建，失败于 `verify-go-sse-dependency.sh` 的 Go 版本检查。
- 修复：`f2225b75`，回补上游 `c42d57de`（PR #2983，随上游 `v1.0.0-rc.4` 发布）中 `Dockerfile` 的一行，改用 `golang:1.27.4-alpine3.24`。
  该提交的其余改动（workflow 的 Go 版本、版本升级脚本、上游 `docker-publish.yml` 的分支发布）不影响 fork 的镜像构建，留待合入 rc.4。

## 验证

- Go 检查在 Docker（`golang:1.27.1`，按 `go.mod` 自动切换到 1.27.4）内运行：
  - merge 提交：
    - 重新生成 ent、gqlgen、`openapi`、`selfusage` 的代码，与 merge 结果逐字节一致。
    - 主模块 `go test -tags=nomsgpack ./...`：46 个包通过。
    - `llm` 模块 `go test ./...`：43 个包通过。
    - `cmd/axonhub-desktop` 模块的 `go vet ./...`、`go test ./...`：2 个包通过，上游干净树相同。
    - PostgreSQL 专用测试批次（`-run Postgres`，需 `AXONHUB_TEST_PG_DSN`）：biz、gql、gc、db 下的 8 个顶层测试通过。
  - 最终提交 `f2225b75`：
    - 主模块 `go test -tags=nomsgpack ./...`：46 个包通过。
    - 健康门控测试 `-race -count=3`：81 个顶层测试各 3 遍通过，无数据竞争。
  - 两次的 `go vet -tags=nomsgpack ./...` 都只报上游的 `internal/server/plugin/manager_test.go:908`、
    `internal/server/api/stream_lifecycle_test.go:179`、`:232`，干净的上游 rc.3 树上相同。
- 前端，代码为 merge 提交：
  - `pnpm install --frozen-lockfile` 通过。
  - 单元测试 470 个，469 个通过，1 个跳过。
  - `pnpm build` 通过。
  - 应用工程的类型检查（`tsc --noEmit -p tsconfig.app.json`）：上游干净树 209 条诊断，merge 提交 191 条。
    - 按文件、错误码与完整消息比对，比上游多出 4 条文字不同的诊断，都对应上游同一位置的已有错误：
      2 条因依赖的安装路径不同、类型摘要截断不同，另 2 条因 fork 的字段或枚举使展开的类型不同。
    - 比 rc2 同步后的代码多 1 条实际新增：上游 `35d3c27f` 的 `quota-badges.tsx` 访问 `channel.providerType` 未收窄类型，干净的上游 rc.3 树上相同。
  - 46 份 locale 文件没有重复键。
- 真机验收：本机 Docker，用 rc2 同步后的镜像（`system_version` 为 `v1.0.0-rc2-fork.1`）的数据卷复制出新卷，
  在新卷上升级到 `f2225b75` 构建的镜像。SQLite 与 PostgreSQL 各一遍，渠道上游用本地 mock。
  - 按原样构建 merge 提交的镜像失败，见发现 1。
  - 结果：两种方言各 31 项通过。启动日志无迁移错误，`system_version` 抬到 `v1.0.0-rc.3-fork.1`，不执行 data migration。
  - 升级后两种方言都能写入、读取并删除 `provider_type` 为 `deepseek` 的额度状态行。
- 管理端界面：在升级后的 PostgreSQL 实例上检查 56 项，全部通过，无控制台错误与未捕获异常。
  - rc2 的 49 项结果不变。
  - 渠道行菜单的弹窗，按点击事件计时：
    - Cancel 后 268–282ms 点击行菜单按钮，458–497ms 点击菜单项重开同一行的弹窗，早于 500ms 的清理时点。
      四类各 3 轮，在 Cancel 后 903–1068ms 检查，弹窗仍打开，内容属于该行。
    - 617–690ms 点击行菜单按钮、817–894ms 点击菜单项，重开同一行或打开另一行的同一弹窗：各 12 轮，内容正确。
    - 别名、覆盖、代理在 104–114ms 的 9 次行菜单按钮点击落在仍在淡出的遮罩上，行菜单没有打开，与 rc2 相同。
      价格 10 轮在 137–154ms 点击按钮、337–371ms 点击菜单项，弹窗都重新打开。
  - rc.3 新增：
    - 中文界面：自助用量页的汇总与趋势、API 密钥详情分析、模型详情分析、仪表盘区块说明、模型价格编辑器的计价模式、
      模型关联条件的「提示词的词元」都显示「词元」，没有残留 fork 的「Token」。
    - 已有额度状态的渠道，额度列照常显示。
- 健康门控的额度兜底，端到端：
  - 设置：健康门控，开启重试，渠道重试次数 1，配额路由为「背压」，两个 mock 渠道提供同一模型。
    在额度状态表写入两行：A 的用量比 0，B 的用量比 0.9，使 B 只对已有会话保留。
  - A 返回 500 时，请求在 A 失败后转到 B 并成功。决策记录无跳过项，不是最后一试。
  - A 正常时请求只到 A，B 没有收到请求。

## 未覆盖范围

- 未运行 lint 与仓库 e2e 套件。
- 真机验收的渠道上游都是本地 mock，未连接真实供应商。
- DeepSeek 余额查询、OIDC 回调与插件界面的 CORS，只有上游自带的单元测试覆盖。
- 合入后行为变化第 2、3 行按代码推断，未构造真实的 HTTP/2 流重置与 GOAWAY。
- 桌面应用只跑了它的模块测试，未打包。
