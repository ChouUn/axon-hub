# 上游同步记录：v1.0.0-beta10（2026-09-09）

> Parent: [../index.md](../index.md)
> Audience: developer — 开发和演进项目的人或 agent
> Genre: survey — 当时基于什么信息做了什么判断
> Visibility: public — 他人可读，避免写入本机环境
> Maintain: frozen 2026-11-14 — 已定稿；内容与定稿日期不改，内容变化只追加 Outdated / Superseded 段，元数据可维护，定位用的路径与链接可随文件移动更新到原对象与版本的新位置

## 结论

- 上游 `v1.0.0-beta7` → `v1.0.0-beta10` 的 124 个提交以 merge 合入，merge 提交为 `7fdbd2ae`。
- 逐项核对 10 项 fork 改动：8 项保留为 fork 差异，2 项合入后与上游一致。
- 真机验收发现生图渠道拆分的两处回归，已在 `e18cc308` 修复。
- 未运行 lint、build 与仓库 e2e 套件。

## 合入

- 冲突：16 个文件。
  - catalog 主题的四个文件与两份 `providers.json` 快照直接取上游：
    - `internal/server/biz/catalog_filter.go`
    - `internal/server/gql/system.graphql`
    - `internal/server/gql/system.resolvers.go`
    - `frontend/src/features/system/components/general-settings.tsx`
    - `internal/server/biz/catalogdata/providers.json`
    - `frontend/src/features/models/data/providers.json`
  - `internal/server/gql/generated.go`、`internal/ent/internal/schema.go` 取上游后 `make generate`。
  - `frontend/src/routeTree.gen.ts` 用 `@tanstack/router-generator` 重新生成。
  - 其余按改动手工拼接。
- 迁移：
  - 上游新增 data migration `v1.0.0-beta9`。
  - schema 变化由 ent 托管，无冲突。
  - data migration 的注册顺序为 `v1.0.0-beta7-fork.1` → `v1.0.0-beta8` → `v1.0.0-beta9`，
    见[渠道定价](../billing/feats/channel-pricing.md)的
    「上游模型价格时间戳修复回补」。

## 逐项核对

| # | 改动 | 结果 | 状态 |
| --- | --- | --- | --- |
| 1 | [仪表盘分析](../usage/feats/analytics.md)：API 密钥费用分析 | 上游改了分析筛选组件，自动合并无冲突 | 保留 |
| 2 | [仪表盘分析](../usage/feats/analytics.md)：模型详情分析 | 同上 | 保留 |
| 3 | [渠道列表](../channels/feats/channel-list.md)：渠道 tab 按供应商聚合 | 上游 `aa8e7c81` 也补了 `ollama_anthropic`，已合一 | 保留 |
| 4 | [渠道列表](../channels/feats/channel-list.md)：生图渠道拆分 | 在上游重构之上重接。真机发现列表字段与编辑同步两处回归，已修 | 保留 |
| 5 | [Docker 镜像发布到 fork 仓库](../deployment/feats/docker-publish.md) | latest manifest 步骤冲突，保留 `${IMAGE}` | 保留 |
| 6 | 模型目录后端热加载 | 上游版本为超集，fork 版本被取代 | 无差异 |
| 7 | [渠道列表](../channels/feats/channel-list.md)：排序字段为零值时分页错乱 | entgql 版本未变，绕过仍需要。上游无新增分页入口 | 保留 |
| 8 | [供应商模型目录](../channels/feats/model-catalog.md)：Provider 模型成本可为空 | 上游未改，仍为 fork 差异 | 保留 |
| 9 | [渠道定价](../billing/feats/channel-pricing.md)：上游模型价格时间戳修复回补 | 上游 data migration `v1.0.0-beta9` 与 fork 的 `v1.0.0-beta7-fork.1` 并存，测试 helper 改名 | 保留 |
| 10 | 模型目录 `experimental` schema 修复回补 | 与上游一致，无差异 | 无差异 |

与上游一致的两项：

- 模型目录后端热加载（第 6 项）：
  - 上游 `4483c2e4`（PR #2263），fork 提前回补 `d6a605a1`。
  - `7fdbd2ae` 合入后采用上游完整实现。
  - `catalog_filter.go`、系统配置与 provider 快照均以上游为准。
  - 模型页使用后端 `providersCatalog(filtered)`。
- 模型目录 `experimental` schema（第 10 项）：
  - 上游 `6f729f7c`（PR #2305），fork 提前回补 `8c978c6b`。
  - `7fdbd2ae` 合入后与上游一致，无 fork 适配、迁移或待维护差异。

## 验证

- `go test ./...`（主模块）通过。
- 前端 `node --test` 88 个通过。
- `tsc --noEmit` 的错误集合与上游 `v1.0.0-beta10` 干净树一致：190 个上游既有错误，未新增。
- 真机验收：本机 Docker（`axonhub:local`，SQLite）灌入三个验收 fixture 后，用一次性 Playwright 脚本逐项核对，
  21 项通过。核对内容：
  - 渠道 tab 分组与计数。
  - 生图 tab 过滤与徽章。
  - 翻页无重复。
  - 新建、编辑对话框的 API 格式。
  - 两个分析页与模型页。

## 未覆盖范围

- 未运行 lint、build 与仓库 e2e 套件。
