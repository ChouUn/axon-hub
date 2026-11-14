# Docker 镜像发布到 fork 仓库

> Parent: [../index.md](../index.md)
> Audience: developer — 开发和演进项目的人或 agent
> Genre: feat — 这个 feature 是什么、为什么值得、什么形态
> Visibility: public — 他人可读，避免写入本机环境
> Maintain: living — 持续更新，保持内容有效

- 行为：复用上游 `docker-publish.yml`（监听 `v*` tag，amd64 / arm64 原生构建后合成
  manifest，构建前把 tag 写入 `internal/build/VERSION`），镜像名由 workflow 级
  `env.IMAGE` 指定为 `chouun/axon-hub`；产出 `<tag>`、`<tag>-<arch>`、`latest`。
- 发布：tag 命名为 `v1.0.0-<上游基线>-fork.N`（如 `v1.0.0-beta10-fork.1`），
  `git tag -a vX -m "release: vX"` 后 `git push fork release/v1.0.x --follow-tags`。
  fork 仓库需启用 workflow，并配置 repository secrets `DOCKERHUB_USERNAME` /
  `DOCKERHUB_TOKEN`；`release.yml`（goreleaser）在 Actions 页面单独禁用。
- 动机：此前手工 `docker build .` 未写 VERSION，镜像自报仓库内上游值 `v1.0.0-beta8`，
  启动时 `datamigrate` 把 `system_version` 抬到 beta8，此后所有 `v1.0.0-beta7-fork.N`
  数据迁移都会被跳过，且镜像内容不保证等于 tag。
- 存量实例：曾被误抬到 `v1.0.0-beta8` 的实例不再需要手工回拨。上游 `v1.0.0-beta9`
  迁移覆盖了 `fork.1` 的清理，且版本比较修复（`6c07e14e`）后部署
  `v1.0.0-beta10-fork.N` 会正常把 `system_version` 抬上去。
- 同步上游注意：上游 manifest 步骤硬编码 `looplj/axonhub`，合并时会在该处冲突，
  保留 `${IMAGE}`。
- 提交：`4dc42806`（`feat(ci): 发布 Docker 镜像到 fork 仓库`）；
  `7fdbd2ae`（合入 beta10）。
- 代码：`.github/workflows/docker-publish.yml`。
- 迁移与测试：无。
