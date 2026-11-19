# 模型入口校验与渠道分发

> Parent: [../index.md](../index.md)
> Audience: developer — 开发和演进项目的人或 agent
> Genre: feat — 这个 feature 是什么、为什么值得、什么形态
> Visibility: public — 他人可读，避免写入本机环境
> Maintain: living — 持续更新，保持内容有效

正式请求的模型准入规则：请求的模型必须是已注册且启用的模型，渠道按该模型的有效关联选择。
所有正式推理请求和公开模型列表都经过它。

## 动机

上游的模型设置提供三项开关，开启后未注册为模型的渠道模型也能被列出和调用，绕过模型注册与关联：

- 「回退到渠道」（`fallbackToChannelsOnModelNotFound`）：请求的模型在模型关联中找不到时，
  直接查找支持该模型的已启用渠道。
- 「查询所有渠道模型」（`queryAllChannelModels`）：模型 API 返回所有已启用渠道的模型。
- 「模型下发黑名单」（`modelBlacklistRegex`）：从上一项的结果里按正则过滤。

## 形态

- 准入：
  - 正式请求在 API Key 模型映射后，必须解析到已注册且启用的模型，再按该模型的有效关联选择渠道。
    有效关联来自两处：模型自身设置里的关联，以及系统设置里该模型开发商（`developer`，如 deepseek）的关联。
  - 模型不存在、禁用、归档或无可用关联时拒绝，不回退直连渠道。
- 映射：API Key 映射采用首个匹配，不递归映射。
- 模型列表：
  - 公开模型列表只下发启用的注册目标及有效别名，不合并渠道独有模型。
  - OpenAI 模型详情使用映射目标的元数据，保留对外别名。
  - OpenAI 与 Anthropic 模型列表从注册目标的模型卡输出 `input_modalities` 和 `supported_reasoning_levels`，
    供 OMP 等客户端在两条线路上自动发现输入类型和推理档位。值为空时省略。
    模型卡标记支持推理时才输出档位。
    - OpenAI 扩展模型响应可用 `include` 指定这两个字段，普通列表请求随「`/v1/models` 默认返回完整模型信息」设置提供。
    - `/anthropic/v1/models` 的普通请求即可获取满足上述条件的字段。
- 设置：上述三项开关已删除。旧 JSON 键被忽略，不能恢复旁路。

## 边界

- Playground 指定渠道要求系统级 `write_channels`，项目所有者权限不能代替。
- 显式查询渠道模型要求 `read_channels`。
- 已创建视频任务的查询、删除仍使用任务记录中的渠道，不重新选择推理模型。
- 不自动注册模型，不放宽准入。

## 实现与维护

- 提交：
  - `569152cc`（`feat(models): 强制注册模型准入并按关联分发渠道`）
  - `fceb30dc`（`feat(models): 补充模型发现的输入模态和推理档位`）
- 代码：
  - `internal/server/orchestrator/candidates.go`
  - `internal/server/orchestrator/model_mapper.go`
  - `internal/objects/apikey.go`
  - `internal/server/biz/model.go`
  - `internal/server/api/`
  - `internal/server/api/openai.go` 提供公开模型的扩展响应和两个列表共用的模型卡字段读取。
  - `internal/server/api/anthropic.go` 在 Anthropic 模型列表下发输入模态和推理档位。
  - `internal/server/gql/model.resolvers.go`
  - `internal/server/gql/system.graphql`
  - `frontend/src/features/models/components/models-settings-dialog.tsx`
- 迁移：无 schema migration 或 data migration，无缓存序列化结构变更。
- 测试：覆盖模型状态、关联限制、映射、Responses 续接和管理权限边界，无新增 fixture。
  - `internal/server/orchestrator/model_admission_test.go`
  - 候选渠道测试
  - `internal/server/biz/model_test.go`
  - `internal/server/biz/model_list_routability_test.go`
  - `internal/server/api/openai_retrieve_test.go`
    验证基本列表、按字段查询、默认扩展列表，以及 Anthropic 列表对别名、空值和未开启推理的处理。
  - `internal/server/api/chat_test.go`
  - `internal/server/gql/model_resolvers_test.go`
  - `internal/objects/apikey_test.go`
- 同步上游：
  - 保留注册准入及映射目标校验，不能恢复按渠道支持模型直接选路。
  - 升级前核对未注册的渠道模型及映射目标，先补齐所需模型与关联。
  - 上游新增的测试用到未注册的模型时，在测试里补注册模型与关联，不恢复已删除的开关。
    合入上游 `v1.0.0-beta12` 时这样处理了：
    - `internal/server/api/responses_protocol_integration_test.go`
    - `internal/server/` 下的 `codex_models_routes_test.go`、`codex_response_harness_test.go`、
      `codex_responses_routes_test.go`、`codex_images_routes_test.go`
  - 上游改写的管理端界面又用到已删除的设置时，合入时去掉这些引用。
    合入上游 `v1.0.0-rc1` 时，上游 `7f961ddb`（PR #2919）重写的 `models-settings-dialog.tsx` 又带回了三项开关的表单项，
    合入时去掉，其余分区沿用上游。
