# 模型入口校验与渠道分发

> Parent: [../index.md](../index.md)
> Audience: developer — 开发和演进项目的人或 agent
> Genre: feat — 这个 feature 是什么、为什么值得、什么形态
> Visibility: public — 他人可读，避免写入本机环境
> Maintain: living — 持续更新，保持内容有效

- 行为：正式请求在 API Key 模型映射后，必须解析到已注册且启用的模型，再按开发者与
  模型的有效关联选择渠道；模型不存在、禁用、归档或无可用关联时拒绝，不回退直连渠道。
- 映射与列表：API Key 映射采用首个匹配、不递归映射；公开模型列表只下发启用的注册
  目标及有效别名，不再合并渠道独有模型。OpenAI 模型详情使用映射目标元数据，保留对外别名。
- 设置：删除渠道回退、渠道模型列表合并及对应黑名单设置；旧 JSON 键被忽略，不能恢复旁路。
- 管理边界：Playground 指定渠道要求系统级 `write_channels`，项目所有者权限不能代替；显式查询渠道模型要求
  `read_channels`。已创建视频任务的查询、删除仍使用任务记录中的渠道，不重新选择推理模型。
- 同步上游注意：保留注册准入及映射目标校验，不能恢复按渠道支持模型直接选路；升级前
  核对未注册的渠道模型及映射目标，先补齐所需模型与关联，不自动注册或放宽准入。
- 提交：`569152cc`（`feat(models): 强制注册模型准入并按关联分发渠道`）。
- 代码：`internal/server/orchestrator/candidates.go`、`internal/server/orchestrator/model_mapper.go`、
  `internal/objects/apikey.go`、`internal/server/biz/model.go`、`internal/server/api/`、
  `internal/server/gql/model.resolvers.go`、`internal/server/gql/system.graphql`、
  `frontend/src/features/models/components/models-settings-dialog.tsx`。
- 迁移：无 schema 或 data migration；无缓存序列化结构变更。
- 测试与 fixture：`internal/server/orchestrator/model_admission_test.go`、候选渠道测试、
  `internal/server/biz/model_test.go`、`internal/server/biz/model_list_routability_test.go`、
  `internal/server/api/openai_retrieve_test.go`、`internal/server/api/chat_test.go`、
  `internal/server/gql/model_resolvers_test.go`、`internal/objects/apikey_test.go`，
  覆盖模型状态、关联限制、映射、Responses 续接和管理权限边界；无新增 fixture。
