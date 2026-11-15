# 上游同步记录：v1.0.0-rc2（2026-11-15）

> Parent: [../index.md](../index.md)
> Audience: developer — 开发和演进项目的人或 agent
> Genre: survey — 当时基于什么信息做了什么判断
> Visibility: public — 他人可读，避免写入本机环境
> Maintain: frozen 2026-11-15 — 已定稿；内容与定稿日期不改，内容变化只追加 Outdated / Superseded 段，元数据可维护，定位用的路径与链接可随文件移动更新到原对象与版本的新位置

## Outdated

2026-11-15：「验证」中渠道行菜单弹窗一条记的是 Cancel 到点击行菜单按钮的时间，不是重开弹窗的时间。重开弹窗的是随后对菜单项的点击，约晚 200ms。

- 「Cancel 后 268–277ms 重开同一行的弹窗」：268–277ms 点击行菜单按钮，468–487ms 点击菜单项重开弹窗。重开仍早于 500ms 的清理时点，该条其余结论不变。
- 「约 600ms 后重开」：615–634ms 点击行菜单按钮，821–857ms 点击菜单项。
- 「约 100ms 时」的 19 次都是行菜单按钮的点击：
  - 别名、覆盖、代理 9 次在 103–108ms，落在仍在淡出的遮罩上，行菜单没有打开。
  - 价格 10 次在 138–198ms，随后在 341–401ms 点击菜单项，弹窗都重新打开。

2026-11-15：「验证」中类型检查一条把比上游多出的 4 条诊断都归因于 fork 的字段，实际只有 2 条如此。4 条都对应上游同一位置的已有错误，「没有上游之外的新增」的结论不变。

- `quota-badges.tsx`、`models-batch-create-dialog.tsx` 的 2 条：fork 的字段或枚举使消息里展开的类型不同。
- `apikeys-profiles-dialog.tsx` 的 2 条 TS2719：两棵树的依赖安装路径不同，类型摘要的截断位置随之不同。

## 结论

- 上游 `v1.0.0-rc1` → `v1.0.0-rc2` 的 12 个提交以 merge 合入，merge 提交为 `377be5b4`。
- 逐项核对 21 项 fork 改动，全部保留为 fork 差异，fork 功能没有可观察的行为变化。
- 上游的渠道别名预览（`19a0984f`，PR #2859）在两处代码上与 fork 改动重叠：
  - 编辑对话框的关闭重置被抽成 `resetDialogState`，fork 的生图格式回显接入其中，见「逐项核对」第 5 项。
  - 上游自带一份渠道行弹窗的可取消延时清理，与 fork 的实现并存，见第 21 项。
- rc2 没有 schema migration 与 data migration。
- 测试与真机验收没有发现合入引入的问题，merge 之后没有修复提交。

## 合入

- 冲突：5 个文件。
  - `.agent/rules/fork-maintenance.md`：上游把 `.agent/` 改名为 `.agents/`，这份 fork 规则随之移到 `.agents/rules/`。
    `AGENTS.fork.md` 中指向它和 `docs.md` 的路径同步更新。
  - `.gitignore`：两侧新增的条目都保留。
  - `frontend/src/components/price-schedule-editor.tsx`：只涉及注释，见第 17 项。
  - `frontend/src/features/channels/components/channels-action-dialog.tsx`：见第 5 项。
  - `frontend/src/features/channels/components/channels-dialogs.tsx`：见第 21 项。
- 两侧都改过、Git 自动合并的文件：
  - `conf/conf.go`、`internal/server/server.go`、`internal/server/routes.go`：见第 7、14 项。
  - `internal/server/gql/model.graphql`、`model.resolvers.go`：上游在两个文件里都只加了别名预览查询，与 fork 的改动不相交。
    - `model.graphql` 中 fork 改的是模型卡的价格字段，见第 17 项。
    - `model.resolvers.go` 中 fork 改的是 `QueryModels` 的模型准入与 `read_channels` 限制，见第 3 项。
  - 渠道的 locale 文件、`frontend/package.json`。
- schema migration：无。
- data migration：
  - 上游无新增 data migration。
  - 注册顺序不变：`v1.0.0-beta7` → `v1.0.0-beta7-fork.1` → `v1.0.0-beta8` → `v1.0.0-beta9`。
  - fork 镜像版本为 `v1.0.0-rc2-fork.1`。
    `biz.CompareVersions` 把预发布号按点和数字段切分后逐段比较（`internal/server/biz/version.go` 的 `prereleaseTokens`），
    据此它高于 `v1.0.0-rc2` 与 `v1.0.0-rc1-fork.1`，低于上游之后的 `v1.0.0-rc.3`、`v1.0.0-rc.4`。

## 上游带来的行为变化

- 渠道别名预览（`19a0984f`，PR #2859）：
  - 编辑对话框的支持模型面板在每个模型下列出它的别名与来源（映射、前缀、去前缀、小写），按别名搜索时保留所属模型。
  - 新增「设置模型别名」按钮，从编辑对话框转到模型映射弹窗。有未保存的修改时先确认，确认后丢弃修改。
  - 模型映射弹窗按正在编辑的设置实时预览渠道对外的模型名与别名。
  - 新增 GraphQL 查询 `previewChannelModelEntries`，按未保存的设置计算渠道对外的模型名，不写库，要求 `read_channels`。
  - OAuth 获取的凭据计入未保存修改。Codex、Claude Code 渠道打开时按已存凭据初始化认证方式。
- 桌面应用（`16084817`，PR #2959）：
  - 新增独立 Go 模块 `cmd/axonhub-desktop`，链接本仓库的服务端代码。
  - 服务端新增 `GET /admin/desktop-login`：只接受回环地址的连接，凭桌面进程签发、60 秒有效的一次性 code 登录。
    服务端单独运行时没有签发 code 的入口。
  - 监听地址改用 `net.JoinHostPort` 拼接，`server.host` 为 IPv6 地址时加方括号。
  - `release.yml` 增加打包桌面应用的 job，新增 `desktop-release.yml`，后者只能被调用或手动触发。
  - agent 规则目录 `.agent/` 改名为 `.agents/`，新增 `.agents/skills/` 下的三个设计类 skill 与 `skills-lock.json`。
- 协议转换：
  - Gemini：工具结果里含 JSON Schema `$ref` 时按文本发送，避免 Gemini 把它当作媒体引用返回 400（`59bb6734`，PR #2972）。
  - OpenAI Responses：没有 call ID 的工具输出保留工具名（`f544bbd5`，PR #2960）。
- 插件：解压后的插件文件不再 fsync，修复 Windows 上带界面的插件加载失败（`3db55064`，PR #2966）。
- 依赖：
  - Go：aws-sdk-go-v2、gjson、sjson、oauth2、sync 等小版本升级。
  - 前端大版本：eslint 10、`@hookform/resolvers` 7、globals 17、lint-staged 17。另有 11 个小版本升级。

## 逐项核对

| # | 改动 | 结果 | 状态 |
| --- | --- | --- | --- |
| 1 | [健康门控](../routing/feats/health-gate.md)：选路门控 | 上游未改编排器与负载均衡 | 保留 |
| 2 | [健康门控](../routing/feats/health-gate.md)：会话归属与决策记录 | 上游未改相关代码 | 保留 |
| 3 | [模型准入](../routing/feats/model-admission.md) | fork 在 `QueryModels` 中的准入与 `read_channels` 限制，与上游新增的预览查询在 `model.resolvers.go` 中自动合并，未受影响。上游新增的别名预览只计算渠道对外的模型名，不参与选路，要求 `read_channels`。模型映射弹窗里预览的说明写着「客户端可使用的模型名称和别名」：上游默认开启找不到模型时回退到渠道，fork 删除了这项回退，这些名字还须注册为启用的模型并有关联才能调用。说明文案沿用上游 | 保留 |
| 4 | [渠道列表](../channels/feats/channel-list.md)：渠道 tab 按供应商聚合 | 上游未改相关代码 | 保留 |
| 5 | [渠道列表](../channels/feats/channel-list.md)：生图渠道拆分 | 上游把编辑对话框的关闭重置抽成 `resetDialogState`，「设置模型别名」跳转前也调用它。其中 API 格式的重置改用 fork 的 `getInitialApiFormatForRow`，两条路径都回显生图格式 | 保留 |
| 6 | [渠道列表](../channels/feats/channel-list.md)：排序字段为零值时分页错乱 | 上游仍锁定 ent/contrib `3625dcc2e035`，绕过仍需要 | 保留 |
| 7 | [渠道测试](../channels/feats/channel-test.md) | 上游改了 `server.go` 的监听地址与 `conf.go` 的配置文件选择，fork 在 `WriteTimeout` 中加入的渠道测试超时与 `server.channel_test_timeout` 默认值在合并结果中保留 | 保留 |
| 8 | [强制 1 小时提示缓存](../channels/feats/force-one-hour-prompt-cache.md) | 上游未改 Anthropic 的缓存断点处理 | 保留 |
| 9 | [供应商模型目录](../channels/feats/model-catalog.md)：Provider 模型成本可为空 | 上游未改目录的 schema | 保留 |
| 10 | [透传](../channels/feats/passthrough.md)：预读事件上限 | 上游未改透传 | 保留 |
| 11 | [透传](../channels/feats/passthrough.md)：渠道上游的错误体被重包装 | 上游未处理，仍为 fork 差异 | 保留 |
| 12 | [仪表盘分析](../usage/feats/analytics.md)：API 密钥费用分析 | 上游未改相关代码 | 保留 |
| 13 | [仪表盘分析](../usage/feats/analytics.md)：模型详情分析 | 上游未改相关代码 | 保留 |
| 14 | [自助用量页](../usage/feats/self-usage.md) | 上游在 `/admin` 的免认证组加了桌面登录路由，fork 的 `/self-service` 路由组在合并结果中保留 | 保留 |
| 15 | [渠道定价](../billing/feats/channel-pricing.md)：渠道标准价格引用与持久倍率 | 上游未改价格对话框 | 保留 |
| 16 | [渠道定价](../billing/feats/channel-pricing.md)：上游模型价格时间戳修复回补 | data migration 的注册位置不变 | 保留 |
| 17 | [模型定价阶梯](../billing/feats/model-pricing-tiers.md) | `model.graphql` 中模型卡的价格字段与上游新增的预览查询自动合并。`price-schedule-editor.tsx` 上游只改了 `portalContainer` 注释里的规则路径：fork 当初在这里加 `renderItems` 时删掉了这行注释，合入时保留 `renderItems` 并恢复上游的注释 | 保留 |
| 18 | [请求成本悬浮](../billing/feats/request-cost-breakdown.md) | 上游未改相关代码 | 保留 |
| 19 | [Docker 镜像发布到 fork 仓库](../deployment/feats/docker-publish.md) | 上游未改 `docker-publish.yml`。新增的桌面打包挂在 `release.yml` 下，fork 仓库在 Actions 页面禁用了它，`desktop-release.yml` 不会自动运行 | 保留 |
| 20 | [渠道性能记录](../channels/feats/channel-performance.md) | 上游未改相关代码 | 保留 |
| 21 | [渠道列表](../channels/feats/channel-list.md)：行菜单打开弹窗后页面被锁住 | 上游 `19a0984f` 在 `channels-dialogs.tsx` 加了等效的延时清理：没有打开的弹窗而仍有当前行时，500ms 后清空，重新打开时取消。fork 的 `scheduleCurrentRowClear` 与它并存：别名、覆盖、代理等弹窗关闭时同时启动两份计时器，价格弹窗由自身的 `setOpen(null)` 关闭，只启动上游的计时器。重开弹窗时，已启动的计时器都会取消。行菜单的 `modal={false}` 保留 | 保留 |

## 验证

- Go 检查在 Docker（`golang:1.27.1`，按 `go.mod` 自动切换到 1.27.3）内运行，代码为 merge 提交：
  - 重新生成 ent、gqlgen、`openapi`、`selfusage` 的代码，与 merge 结果逐字节一致。
  - 主模块 `go test -tags=nomsgpack ./...`：46 个包通过。
  - `llm` 模块 `go test ./...`：43 个包通过。
  - `cmd/axonhub-desktop` 模块的 `go vet ./...`、`go test ./...`：2 个包通过，上游干净树相同。
  - PostgreSQL 专用测试批次（`-run Postgres`，需 `AXONHUB_TEST_PG_DSN`）：biz、gql、gc、db 下的 8 个顶层测试通过。
  - `go vet -tags=nomsgpack ./...` 只报上游的 `internal/server/plugin/manager_test.go:908`、`internal/server/api/stream_lifecycle_test.go:179`、`:232`，干净的上游 rc2 树上相同。
  - 源码以只读方式挂载时，`TestProviderQuotaService_FillsCodexPeriodQuotaOnlyWithUsableCostAndRatio` 在 merge 结果与干净的上游 rc2 树上同样失败。
    上游 `internal/server/biz/provider_quota_cost_test.go:262` 的 DSN 写成 `file=ent?mode=memory&_fk=0`，SQLite 把它当作工作目录下的磁盘文件。可写挂载下通过，上面的计数即可写挂载的结果。
- 前端：
  - `pnpm install --frozen-lockfile` 通过，Git 自动合并的 `pnpm-lock.yaml` 不需要重新生成。
  - 单元测试 466 个，465 个通过，1 个跳过。
  - `pnpm build` 通过。
  - 应用工程的类型检查（`tsc --noEmit -p tsconfig.app.json`）：上游干净树 208 条诊断，merge 提交 190 条。
    - 按文件、错误码与完整消息比对，比上游多出 4 条，都对应上游同一位置的已有错误，消息里展开的类型因 fork 的字段而不同。
    - 与 rc1 同步后的代码相比没有新增诊断，`@hookform/resolvers` 7 没有给 fork 的表单带来类型错误。
  - 两份渠道 locale 各 1075 个键，没有重复键。
- 真机验收：本机 Docker，用 rc1 同步后的镜像（`system_version` 为 `v1.0.0-rc1-fork.1`）的数据卷复制出新卷，
  在新卷上升级到 merge 提交构建的镜像。SQLite 与 PostgreSQL 各一遍，渠道上游用本地 mock。
  - 结果：两种方言各 31 项通过。
  - 核对内容与 [rc1 同步记录](upstream-sync-v1.0.0-rc1.md)的「验证」相同。启动日志无迁移错误，`system_version` 抬到 `v1.0.0-rc2-fork.1`，不执行 data migration。
- 管理端界面：在升级后的 PostgreSQL 实例上检查 49 项，全部通过，无控制台错误与未捕获异常。
  - rc1 的 39 项结果不变。
  - rc2 新增：
    - 别名预览：配置两个映射到同一上游模型的别名和一个前缀后，编辑对话框在该模型下列出两个别名与带前缀的名字。按其中一个别名搜索，保留上游模型与同组别名。
    - 「设置模型别名」：不改动时直接打开同一渠道的映射弹窗。改动备注后出现确认框，确认后打开映射弹窗，不发出渠道保存请求，重开编辑时备注为原值。
    - 生图渠道：API 格式改为对话格式后不保存关闭，或经「设置模型别名」离开，重开编辑都回显「OpenAI (Image Generation)」。经「设置模型别名」离开时不发出渠道保存请求。
  - 渠道行菜单的弹窗：别名、覆盖、代理、价格四类，按点击事件计时，最终关闭后页面都没有锁住。
    - Cancel 后 268–277ms 重开同一行的弹窗，此时遮罩已移除，尚未到 500ms 的清理时点。四类各 3 轮，在 Cancel 后 904–1051ms 检查，弹窗仍打开，内容属于该行。
      按源码，重开取消的是别名、覆盖、代理的两份计时器，以及价格弹窗的上游计时器。
    - 约 600ms 后重开同一行或打开另一行的同一弹窗：各 12 轮，内容正确。
    - 约 100ms 时，19 次点击中有 9 次落在仍在淡出的遮罩上，第二个弹窗没有打开，与 rc1 的结果相同。

## 未覆盖范围

- 未运行 lint 与仓库 e2e 套件。
- 真机验收的渠道上游都是本地 mock，未连接真实供应商。
- 桌面应用只跑了它的模块测试，未打包，也未运行桌面登录的完整流程。
- `server.host` 为 IPv6 地址时的监听、Gemini 的 `$ref` 工具结果、Responses 无 call ID 的工具输出、Windows 上的插件加载，只有上游自带的单元测试覆盖。
