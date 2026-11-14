# 迁移器

> Parent: [../index.md](../index.md)
> Audience: developer — 开发和演进项目的人或 agent
> Genre: feat — 这个 feature 是什么、为什么值得、什么形态
> Visibility: public — 他人可读，避免写入本机环境
> Maintain: living — 持续更新，保持内容有效

迁移器（`internal/ent/migrate/datamigrate`）是上游的 data migration 执行器：启动时比较
`system_version` 与每个已注册迁移的版本，决定执行还是跳过，升级后抬高 `system_version`。
本 fork 只改了其中的版本比较，使预发布号按数值排序。
fork 的每个 data migration 和每次升级都依赖这个顺序。

## 动机

- `datamigrate` 用 Masterminds semver 比较版本。预发布标识 `beta10` 与 `beta9` 是字母数字标识，按字符串比较，
  得到 `v1.0.0-beta10 < v1.0.0-beta9`。
- `system_version` 为 `v1.0.0-beta8` 或 `v1.0.0-beta9` 的实例升到 fork 的发布版本 `v1.0.0-beta10-fork.N` 后，
  `system_version` 抬不上去，之后所有版本为 `v1.0.0-beta10-fork.N` 的 data migration 被静默跳过。
- 上游自身从 `v1.0.0-beta9` 升到 `v1.0.0-beta10` 同样不抬版本。上游 `v1.0.0-beta10` 的 `migrator.go`
  比较逻辑未变，这项修正可回馈上游。

## 形态

- `parseComparableVersion` 把预发布标识里的字母词和数字拆成两段：`v1.0.0-beta10` 按 `beta.10` 比较，
  `v1.0.0-rc1` 按 `rc.1`，`v1.0.0-beta7-fork.5` 按 `beta.7.fork.5`。
- 迁移版本、系统版本与构建版本三处比较都用该函数。

## 边界

- 拆分结果只用于比较，存储与展示的版本字符串不变。
- `internal/server/biz/version.go` 的更新检查仍用原生 semver。
- fork data migration 的版本命名与注册顺序由 [AGENTS.fork.md](../../../AGENTS.fork.md) 的
  「数据库迁移兼容性」规定。

## 实现与维护

- 提交：`6c07e14e`（`fix(datamigrate): 预发布号按数值比较`）。
- 代码：
  - `internal/ent/migrate/datamigrate/version.go`
  - `internal/ent/migrate/datamigrate/migrator.go`
- 迁移：无。
- 测试：
  - `version_test.go` 覆盖 `v1.0.0-beta6` 到 `v1.0.0` 的完整顺序，含带 `-fork.N` 后缀的版本。
  - `migrator_test.go` 的 `TestMigrator_Run_NumericPrereleaseOrdering`，构建版本设为 `v1.0.0-beta10-fork.1`，
    覆盖两种情形：
    - `system_version` 为 `v1.0.0-beta9` 时，执行 data migration `v1.0.0-beta10` 并把 `system_version` 抬到构建版本。
    - `system_version` 已是 `v1.0.0-beta10-fork.1` 时，跳过 data migration `v1.0.0-beta7-fork.1` 与 `v1.0.0-beta10`。
- 同步上游：上游改为 `beta.10` 式命名或自行修复比较后，本函数退化为无害冗余。
