# AGENTS.fork.md

> Parent: 无
> Audience: agent-only — 以 agent 为执行对象的行为指令
> Genre: prompt — 约束模型行为的指令
> Visibility: public — 他人可读，避免写入本机环境
> Maintain: living — 持续更新，保持内容有效

`AGENTS.md` 与上游保持一致。fork 专属的 agent 规则写在本文件，由 `.omp/config.yml` 的
`contextFiles.extra` 加载。

## 仓库维护模式

- 本仓库是 [上游 AxonHub](https://github.com/looplj/axonhub) 的自维护 fork。
- 本 fork 相对上游的改动按模块和功能记录在 [docs/index.md](docs/index.md)。
- 新增或实质修改 fork 改动、同步上游 release 前，MUST 先加载
  [.agents/rules/fork-maintenance.md](.agents/rules/fork-maintenance.md)。
- 上游 release tag 是定期同步的基线。合并或 cherry-pick 上游 release 前，MUST 同时检查
  上游的 schema、data migration 和 fork 改动。
- 本 fork 的专有行为和迁移 SHOULD 与上游历史保持可区分。
- 除非有明确要求，NEVER 仅为使分支看起来像上游而改写或删除既有的 fork 维护改动。

## 文档

- `.agents/rules/docs.md` 要求的中英文同步：
  - fork 自有的文档（`docs/index.md`、`docs/reports/` 与 `docs/<模块>/`）不适用。
  - 上游的文档树 `docs/zh/`、`docs/en/` 仍适用，包括 fork 在其中新增的文件：修改一种语言时 SHOULD 同时修改另一种。
- 本仓库不使用 OpenSpec：doc-writing 归入 spec 的行为契约 MUST 写在所属功能的文档
  （`docs/<模块>/feats/`）中，并保留已有的 D、R 编号。

## 部署

- 本 fork 的部署主要使用 PostgreSQL。

## 数据库迁移兼容性

- `internal/ent/migrate/migrations/`（schema migration）和
  `internal/ent/migrate/datamigrate/`（data migration）都是升级路径代码。
- 新增 fork 迁移前，MUST 对照目标上游 release tag 及其迁移版本。
- NEVER 复用上游迁移版本，也 NEVER 假定看似未使用的版本号已经被 fork 保留。
- 每个 fork data migration MUST 使用 `v1.0.0-<上游基线>-fork.N` 形式、能明确表明 fork 归属的
  版本标识（例如 `v1.0.0-beta10-fork.1`）。
- 版本顺序以 `biz.CompareVersions` 的数值感知比较为准：原生 semver 会把
  `v1.0.0-beta10` 排在 `v1.0.0-beta9` 之前。MUST 验证新迁移与周边上游迁移版本的顺序，并在 `NewMigrator` 中
  按对应的执行顺序注册。
- 已发布的迁移不可变。需要修正时新增幂等迁移，NEVER 修改既有迁移的行为或版本号。
- 每个 fork 迁移 MUST 补充升级路径测试，覆盖相关数据库方言行为。
- 同步上游前，MUST 检查下一个上游 release 是否存在版本或 schema 冲突。
