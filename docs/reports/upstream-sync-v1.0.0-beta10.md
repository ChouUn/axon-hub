# 上游同步记录：v1.0.0-beta10（2026-09-09）

> Parent: [../index.md](../index.md)
> Audience: developer — 开发和演进项目的人或 agent
> Genre: survey — 当时基于什么信息做了什么判断
> Visibility: public — 他人可读，避免写入本机环境
> Maintain: living — 持续更新，保持内容有效

- 基线：`v1.0.0-beta7` → `v1.0.0-beta10`（上游 124 个提交），以 merge 合入，
  merge 提交 `7fdbd2ae`。
- 冲突：16 个文件。catalog 主题四个文件与两份 `providers.json` 快照直接取上游；
  `generated.go`、`internal/ent/internal/schema.go` 取上游后 `make generate`；
  `routeTree.gen.ts` 用 `@tanstack/router-generator` 重新生成；其余按 topic 手工拼接。
- 迁移：上游新增 `v1.0.0-beta9` data migration；schema 变化由 ent 托管，无冲突。
  注册顺序 `fork.1` → `beta8` → `beta9`，见「上游模型价格时间戳修复回补」。
- 验证：`go test ./...`（主模块）通过；前端 `node --test` 88 个通过；`tsc --noEmit`
  错误集合与上游 beta10 干净树一致（190 个上游既有错误，未新增）。未运行 lint、build
  与仓库 e2e 套件。
- 真机验收：本机 Docker（`axonhub:local`，SQLite）灌入三个验收 fixture 后，用一次性
  Playwright 脚本逐项核对渠道 tab 分组与计数、生图 tab 过滤与徽章、翻页无重复、
  新建/编辑对话框的 API 格式、两个分析页与模型页，21 项通过。过程中发现生图拆分的两处
  回归（列表字段、编辑同步），已在 `e18cc308` 修复。
- 逐项核对：

| Topic | 结果 |
| --- | --- |
| API 密钥费用分析、模型详情分析 | 上游改了分析筛选组件，自动合并无冲突 |
| 渠道 tab 按供应商聚合 | 上游 `aa8e7c81` 也补了 `ollama_anthropic`，已合一 |
| 生图渠道拆分 | 在上游重构之上重接；真机发现列表字段与编辑同步两处回归，已修 |
| Docker 镜像发布 | latest manifest 步骤冲突，保留 `${IMAGE}` |
| 模型目录后端热加载 | 上游版本为超集，fork 版本被取代 |
| 排序字段为零值时分页错乱 | entgql 版本未变，绕过仍需要；上游无新增分页入口 |
| Provider 模型成本可为空 | 上游未改，仍为 fork 差异 |
| 模型价格时间戳修复回补 | 上游 `beta9` 迁移与 `fork.1` 并存，测试 helper 改名 |
| 模型目录 schema 修复回补 | 与上游一致，无差异 |

- 模型目录后端热加载：上游 `4483c2e4`（PR #2263），fork 提前回补 `d6a605a1`；
  `7fdbd2ae` 合入后采用上游完整实现，不再有 fork 差异。`catalog_filter.go`、系统配置与
  provider 快照均以上游为准；模型页使用后端 `providersCatalog(filtered)`。
- 模型目录 `experimental` schema：上游 `6f729f7c`（PR #2305），fork 提前回补 `8c978c6b`；
  `7fdbd2ae` 合入后与上游一致，无 fork 适配、迁移或待维护差异。
