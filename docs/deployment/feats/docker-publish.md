# Docker 镜像发布到 fork 仓库

> Parent: [../index.md](../index.md)
> Audience: developer — 开发和演进项目的人或 agent
> Genre: feat — 这个 feature 是什么、为什么值得、什么形态
> Visibility: public — 他人可读，避免写入本机环境
> Maintain: living — 持续更新，保持内容有效

推送 `v*` tag 后，GitHub Actions 构建 fork 的 Docker 镜像并发布到 `chouun/axon-hub`。
部署 fork 的人依赖它拿到版本号与 tag 一致的镜像。

## 动机

- 有这条流程之前，镜像由手工 `docker build .` 构建，不写 `internal/build/VERSION`。
  镜像自报仓库内 VERSION 文件的上游值，当时是 `v1.0.0-beta8`。
- 启动时 `datamigrate` 把 `system_version` 抬到 `v1.0.0-beta8`，此后所有版本为 `v1.0.0-beta7-fork.N` 的
  fork data migration 都被跳过。
- 手工构建的镜像内容不保证等于 tag。

## 形态

- 构建：`docker-publish.yml` 沿用上游 `v1.0.0-beta11` 的写法，构建步骤写在文件内。
  - 监听 `v*` tag。
  - 构建前把 tag 写入 `internal/build/VERSION`。
  - amd64、arm64 原生构建后合成 manifest。
  - 上游自 `5b9b2e96`（PR #2744，随上游 `v1.0.0-beta12` 发布）起把构建矩阵移到可复用的 `docker-build.yml`，
    镜像名 `looplj/axonhub` 写在其中。本 fork 的 `docker-publish.yml` 不调用它。
  - 上游同期对构建步骤的改动未带入本 fork 的文件：按架构划分 GHA 缓存（`6250e7d3`，PR #2712）、
    action 版本固定到提交哈希。上游 `v1.0.0-rc1` 又把 `docker/login-action` 的固定版本升到 v4.8.0（`e1b488e3`，PR #2941），
    本 fork 仍用 `@v4`。
- 镜像名：由 workflow 级 `env.IMAGE` 指定为 `chouun/axon-hub`。
- 产出：`<tag>`、`<tag>-<arch>`、`latest`。
- 发布：
  1. tag 命名为 `v1.0.0-<上游基线>-fork.N`，`<上游基线>` 是所基于的上游预发布号，如 `v1.0.0-beta10-fork.1`。
  2. `git tag -a vX -m "release: vX"`。
  3. `git push fork release/v1.0.x --follow-tags`。
- 仓库前提：
  - fork 仓库启用 workflow。
  - 配置 repository secrets `DOCKERHUB_USERNAME`、`DOCKERHUB_TOKEN`。
  - `release.yml`（goreleaser）在 Actions 页面单独禁用。

## 边界

- `system_version` 曾被误抬到 `v1.0.0-beta8` 的存量实例不需要手工回拨：
  - 上游 data migration `v1.0.0-beta9` 覆盖了 fork data migration `v1.0.0-beta7-fork.1` 的清理。
  - 版本比较按数值排序预发布号后（见
    [上游 v1.0.0-beta11 同步记录](../../reports/upstream-sync-v1.0.0-beta11.md)的「迁移器版本比较」），
    部署 fork 的发布版本 `v1.0.0-beta10-fork.N` 会正常抬高 `system_version`。

## 实现与维护

- 提交：
  - `4dc42806`（`feat(ci): 发布 Docker 镜像到 fork 仓库`）
  - `7fdbd2ae`（合入上游 `v1.0.0-beta10`）
  - `574f09ec`（合入上游 `v1.0.0-beta12`，保留 fork 的整份文件）
  - `c0ce0483`（合入上游 `v1.0.0-rc1`，保留 fork 的整份文件）
- 代码：`.github/workflows/docker-publish.yml`。
- 迁移与测试：无。
- 同步上游：
  - 上游改动 `docker-publish.yml` 时在该文件冲突，保留 fork 的整份文件。
  - 上游对构建步骤的改动落在 `docker-build.yml`，需要时手工移到 fork 的 `docker-publish.yml`。
