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
- 本 fork 的 feature、bugfix topic 按模块记录在 [docs/index.md](docs/index.md)。
- 新增或实质修改 fork topic、同步上游 release 前，MUST 先加载
  [.agent/rules/fork-maintenance.md](.agent/rules/fork-maintenance.md)。
- 上游 release tag 是定期同步的基线。合并或 cherry-pick 上游 release 前，必须同时检查
  上游的 schema、data migration 和 fork 改动。
- 本 fork 的专有行为和迁移应与上游历史保持可区分。除非有明确要求，不要仅为使分支看起来
  像上游而改写或删除既有的 fork 维护改动。

## 文档

- `.agent/rules/docs.md` 要求的中英文同步在本 fork 不适用：fork 不发布文档。

## 数据库迁移兼容性

- 将 `internal/ent/migrate/migrations/`（schema migration）和
  `internal/ent/migrate/datamigrate/`（data migration）都视为升级路径代码。
- 新增 fork 迁移前，必须对照目标上游 release tag 及其迁移版本；不要复用上游迁移版本，
  也不要假定看似未使用的版本号已经被 fork 保留。
- 每个 fork data migration 都要使用 `v1.0.0-<上游基线>-fork.N` 形式、能明确表明 fork 归属的
  版本标识（例如 `v1.0.0-beta10-fork.1`）。版本顺序以 `datamigrate.parseComparableVersion`
  的数值感知比较为准（原生 semver 会把 `beta10` 排在 `beta9` 之前）。必须验证它与周边
  上游版本的顺序，并在 `NewMigrator` 中按对应的执行顺序注册。
- 已发布的迁移视为不可变。需要修正时新增幂等迁移，不要修改既有迁移的行为或版本号。
- 每个 fork 迁移都要补充升级路径测试，覆盖相关数据库方言行为；同步上游前检查下一个
  上游 release 是否存在版本或 schema 冲突。
