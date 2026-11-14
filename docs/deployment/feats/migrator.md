# 迁移器

> Parent: [../index.md](../index.md)
> Audience: developer — 开发和演进项目的人或 agent
> Genre: feat — 这个 feature 是什么、为什么值得、什么形态
> Visibility: public — 他人可读，避免写入本机环境
> Maintain: living — 持续更新，保持内容有效

## 迁移器把 beta10 排在 beta9 之前

- 问题：`datamigrate` 用 Masterminds semver 比较版本，`beta10` 与 `beta9` 作为字母数字
  标识按字符串比较，得到 `beta10 < beta9`。beta8/beta9 实例升到
  `v1.0.0-beta10-fork.N` 后 `system_version` 抬不上去，之后所有 `beta10-fork.N`
  数据迁移被静默跳过；上游自身从 beta9 升 beta10 同样不抬版本。
- 上游来源：上游 beta10 的 `migrator.go` 比较逻辑未变，尚未处理；fork 先行修复，可回馈。
- 行为：`parseComparableVersion` 把 `betaN`、`rcN` 拆成 `beta.N`，`beta7-fork.5` 拆成
  `beta.7.fork.5`，只用于比较，存储与展示的版本字符串不变。迁移版本、系统版本与
  构建版本三处比较均改用该函数。
- 提交：`6c07e14e`（`fix(datamigrate): 预发布号按数值比较`）。
- 代码：`internal/ent/migrate/datamigrate/version.go`、
  `internal/ent/migrate/datamigrate/migrator.go`。
- 迁移与测试：无迁移。`version_test.go` 覆盖 beta6 到 1.0.0 的完整顺序；
  `migrator_test.go` 的 `TestMigrator_Run_NumericPrereleaseOrdering` 覆盖 beta9 系统
  执行 beta10 迁移并抬版本、beta10-fork.1 系统跳过 fork.1 与 beta10。
- 同步上游注意：若上游改为 `beta.10` 式命名或自行修复比较，本函数可退化为无害冗余。
  `internal/server/biz/version.go` 的更新检查仍用原生 semver，未改。
