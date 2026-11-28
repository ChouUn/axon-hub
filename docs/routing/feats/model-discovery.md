# 模型发现字段

> Parent: [../index.md](../index.md)
> Audience: developer — 开发和演进项目的人或 agent
> Genre: feat — 这个 feature 是什么、为什么值得、什么形态
> Persistence: durable — 主体长期存在，内容跨多次变更累积
> Describes: target — 写应该是什么样，与代码不一致时不照代码改
> Visibility: public — 他人可读，避免写入本机环境
> Maintain: living — 持续更新，保持内容有效

OpenAI、Anthropic、Gemini 三条协议线路的模型列表从注册模型的模型卡下发同一组发现字段：上下文窗口、输出上限、输入模态与推理档位，供 [OMP](https://github.com/can1357/oh-my-pi) 等按协议各配一个 provider、从模型列表自动发现模型的客户端使用。

## 动机

OMP 19.2.1 发现出的模型沿用所在 provider 的协议，客户端因此按协议各配一个 provider。provider 用读 OpenAI 形状列表的发现方式 `openai-models-list` 时，请求 `{baseUrl}/v1/models`（`baseUrl` 以 `/v1` 结尾时不补 `/v1`），只读 `data[]`，丢弃 `baseUrl` 的查询串。
没有本功能时：

- Anthropic 列表缺上下文窗口与输出上限，OMP 取内置目录或默认值。
- Gemini 只有 `models[]`，Gemini 推理根下的 `/v1/models`（如 `/gemini/v1beta/v1/models`）返回 404，Gemini Contents 的 provider 无法用 `openai-models-list` 发现模型。
- `/v1/models` 的这四个字段随「`/v1/models` 默认返回完整模型信息」设置下发，OMP 带不上 `include`，关闭设置的实例拿不到。

客户端因此无法从三条线路的列表可靠地发现同一组模型及其上下文窗口和输出上限。

## 形态

- 字段：

  | 字段 | 取自模型卡 | 省略条件 |
  | --- | --- | --- |
  | `context_length` | 上下文上限 | 为 0 |
  | `max_output_tokens` | 输出上限 | 为 0 |
  | `input_modalities` | 输入模态 | 为空 |
  | `supported_reasoning_levels` | 推理档位 | 为空，或模型卡未标记支持推理 |

  - 别名取映射目标的模型卡，与目标同值。
  - 模型卡缺值时省略字段，不编造默认值。
- 线路：

  | 线路 | 列表 | 发现字段何时下发 |
  | --- | --- | --- |
  | OpenAI | `GET /v1/models`、`GET /v1/models/{id}` | `include` 省略或为空时总带。`include` 列出字段时只带列出的扩展字段，`include=all` 带全部 |
  | Anthropic | `GET /anthropic/v1/models` | 总带 |
  | Gemini | `GET /gemini/{版本}/v1/models`、`GET /v1beta/v1/models` | 总带。返回 OpenAI 形状 `{"object":"list","data":[…]}`，每行有 `id`、`object`、`created`、`owned_by` 与发现字段 |

  - 「`/v1/models` 默认返回完整模型信息」只控制列表的 `name`、`pricing`、`capabilities` 等其余扩展字段，不影响发现字段，也不影响 `GET /v1/models/{id}`。
  - Gemini 原生列表 `GET /gemini/{版本}/models` 的 `models[]` 不变，供 Gemini 原生客户端读。Gemini 的发现列表与它鉴权相同，`?key=` 优先，其次请求头，`Authorization: Bearer` 可用。
  - 三个列表按请求的 API Key 收录模型，与[模型列表](model-admission.md)相同，都是启用的注册模型及指向它们的有效别名。同一 API Key 请求的三个列表模型集合相同。
- OMP 配置：各协议的 provider 都用 `discovery.type: openai-models-list`，`injectV1` 取默认值，推理仍走各自的 `baseUrl`。Gemini Contents 的 provider 用 `api: google-generative-ai`，`baseUrl` 为 `/gemini/v1beta` 或 `/v1beta`，发现请求落到其下的 `/v1/models`。`/gemini/v1` 以 `/v1` 结尾，OMP 不补 `/v1`，发现请求落到 Gemini 原生列表，读不到 `data[]`。

## 边界

- 一致性：同一 API Key 请求三个列表、`/v1/models` 未带 `include` 时，网关保证三个列表的发现字段同值或同时省略。该行带正的 `max_output_tokens`，或 OMP 内置目录为这个 id 提供输出上限时，OMP 发现出的输出上限三条线路一致。目录可能收录该 id 而未提供输出上限。
  - OMP 19.2.1 的输出上限依次取列表行、内置目录、协议默认值，取到的值再以上下文窗口封顶。协议默认值在 `api: anthropic-messages` 的 provider 为 8,192，其余为 32,768，因此两者都缺时输出上限为 `min(上下文窗口, 协议默认值)`，上下文窗口大于 8,192 时 Anthropic 线路与另两条不同。例如上下文窗口 16,384 时 Anthropic 线路得到 8,192，另两条得到 16,384。
  - 补齐的办法是给模型卡填输出上限。模型卡的输出上限同时是发往 Anthropic Messages 渠道、客户端未指定输出上限时的默认 `max_tokens`。
  - 上下文窗口与输入模态在 OMP 的默认值与协议无关，不受此影响。
- 思考配置的协议差异（Anthropic 模式、Gemini 的 `thinkingLevel` 与 `thinkingBudget`）由客户端处理，网关只下发档位。
- 不改模型列表收录哪些模型，收录规则见[模型入口校验与渠道分发](model-admission.md)。
- Gemini 原生列表不补 `inputTokenLimit`、`outputTokenLimit`：OMP 的自定义 provider 不用 Gemini 原生发现。

## 实现与维护

- 需求：[ticket：OMP 在三条协议线路上从 AxonHub 发现一致的模型元数据](../../tickets/omp-model-discovery.md)。
- 提交：
  - `fceb30dc`（`feat(models): 补充模型发现的输入模态和推理档位`）
  - `f881c154`（`feat(models): Anthropic 模型列表下发输入模态和推理档位`）
  - `4106842f`（`feat(models): 三条协议线路的模型列表下发一致的发现字段`）
- 代码：
  - `internal/server/api/model_discovery.go`：从模型卡生成发现字段，三条线路共用。
  - `internal/server/api/openai.go`：`/v1/models` 与 `/v1/models/{id}` 在 `include` 省略或为空时带发现字段。
  - `internal/server/api/anthropic.go`
  - `internal/server/api/gemini.go`：`ListDiscoveryModels`。
  - `internal/server/routes.go`：两个 Gemini 路由组注册 `GET /v1/models`。
  - `frontend/src/locales/en/models.json`、`frontend/src/locales/zh-CN/models.json`：设置说明。
- 迁移：无 schema migration 或 data migration，无缓存序列化结构变更。
- 测试：`internal/server/api/openai_retrieve_test.go`，无新增 fixture。
  - `TestModelLists_DiscoveryFieldsMatchAcrossProtocols`：三个列表都是 OpenAI 列表形状，对同一模型及其别名下发相同的发现字段，与 OpenAI 模型详情一致，缺输出上限时都省略，Gemini 原生列表的字段不变。
  - `TestOpenAIHandlers_ListModels_DefaultsToBasicAndDiscoveryFields`：设置关闭时，`include` 省略或为空的列表带发现字段、不带其余扩展字段，`include` 只返回列出的扩展字段。
  - `TestOpenAIHandlers_ListModels_UsesExtendedFieldsWhenConfiguredAsDefault`：设置开启时，`include` 省略或为空的列表带全部扩展字段，模型详情仍只带基础字段与发现字段。
  - `TestAnthropicHandlers_ListModels_ReturnsDiscoveryMetadata`：别名取目标的模型卡，空值与未开启推理时省略。
  - `TestPublicModelAPIs_MappedTargetsRequireEnabledRegistration`：Gemini 发现列表与其余列表的模型集合相同。
- 同步上游：
  - 上游改 `OpenAIModel`、`AnthropicModel` 或 `parseOpenAIModelInclude` 时，保留嵌入的 `ModelDiscoveryFields`，以及 `include` 省略或为空时下发发现字段。
  - 上游改「`/v1/models` 默认返回完整模型信息」的语义时，发现字段仍不受它控制。
  - 上游在 Gemini 路由组注册 `/v1/...` 路径时，核对与 `GET /v1/models` 是否冲突。
