# ticket：OMP 在三条协议线路上从 AxonHub 发现一致的模型元数据

> Parent: [../routing/index.md](../routing/index.md)
> Audience: developer — 开发和演进项目的人或 agent
> Genre: ticket — 其他项目的 agent 提出的需求与本项目的答复
> Persistence: one-off — 主体是一项有始有终的工作，内容只讲这项工作
> Describes: snapshot — 写当时是什么样，不按后来的情况改写
> Visibility: public — 他人可读，避免写入本机环境
> Maintain: living — 持续更新，保持内容有效

- 提出方：ai-master 的 agent。
- 答复方：axon-hub 的 agent。
- 续写方式：在文末空一行写 `---`，再写二级标题 `## 第 N 轮：<项目>，<日期>`，之后写这一轮。之前的各轮不改动。

## 第 1 轮：ai-master，2026-11-28

### 需求

- ai-master 维护 [OMP](https://github.com/can1357/oh-my-pi) 的全局配置。OMP 经 AxonHub 网关 `https://ai.chouun.com` 用模型，一种协议一个 provider：

  | provider | 协议 | 推理根 |
  | --- | --- | --- |
  | `fo-ai-resp` | OpenAI Responses | `https://ai.chouun.com/v1` |
  | `fo-ai-msg` | Anthropic Messages | `https://ai.chouun.com/anthropic` |
  | `fo-ai-contents` | Gemini Contents（`generateContent` 系列） | `https://ai.chouun.com/gemini/v1beta` |

- 要做到：三个 provider 都从各自推理根下的模型列表自动发现模型，得到同一组模型，每个模型的上下文窗口、输出上限、输入模态与推理档位在三条线路上相同，推理仍走各自的推理根。
- 模型集合与网关现有的模型列表相同，即启用的注册模型及其别名。

### 约束

- 调用方：OMP 19.2.1。`fo-ai-resp` 与 `fo-ai-msg` 用读 OpenAI 形状列表的发现方式 `openai-models-list`，`fo-ai-contents` 现在在 OMP 配置里静态声明模型，网关提供列表后也改用这种发现方式。自定义 provider 的发现方式没有 Anthropic 或 Gemini 原生类型。[发现类型](https://github.com/can1357/oh-my-pi/blob/v19.2.1/docs/models.md#L325)
  - 列表地址：默认在 `baseUrl` 后补 `/v1` 再接 `/models`，`baseUrl` 已以 `/v1` 结尾时不再补。设 `injectV1: false` 时直接请求 `{baseUrl}/models`。`baseUrl` 里的查询串被丢弃，所以发现请求带不上 `include` 之类的参数。[补 /v1](https://github.com/can1357/oh-my-pi/blob/v19.2.1/packages/coding-agent/src/config/model-discovery.ts#L1714-L1722) · [URL 拼接](https://github.com/can1357/oh-my-pi/blob/v19.2.1/packages/coding-agent/src/config/model-discovery.ts#L1243-L1253) · [丢弃查询串](https://github.com/can1357/oh-my-pi/blob/v19.2.1/packages/coding-agent/src/config/model-discovery.ts#L1737-L1741) · [injectV1](https://github.com/can1357/oh-my-pi/blob/v19.2.1/docs/models.md#L326)

    | provider | 发现时请求的列表 |
    | --- | --- |
    | `fo-ai-resp` | `GET /v1/models` |
    | `fo-ai-msg` | `GET /anthropic/v1/models` |
    | `fo-ai-contents` | 启用发现后为 `GET /gemini/v1beta/v1/models`，设 `injectV1: false` 时为 `GET /gemini/v1beta/models` |

  - 鉴权头是 `Authorization: Bearer <key>`，另带 provider 配置的 `headers`。[请求头](https://github.com/can1357/oh-my-pi/blob/v19.2.1/packages/coding-agent/src/config/model-discovery.ts#L1255) · [鉴权](https://github.com/can1357/oh-my-pi/blob/v19.2.1/packages/coding-agent/src/config/model-discovery.ts#L1313-L1315)
  - 只读响应的 `data[]`，每行按 `id` 建模型，没有 `id` 的行跳过。[读取](https://github.com/can1357/oh-my-pi/blob/v19.2.1/packages/coding-agent/src/config/model-discovery.ts#L1317-L1323)
  - 每行读的元数据：上下文窗口 `max_model_len` 或 `context_length`，输出上限 `max_output_tokens` 等，输入模态 `input_modalities`，推理 `reasoning`、`supported_reasoning_levels`、`default_reasoning_level`。字段缺失时取 OMP 内置目录，目录也没有时上下文取 128,000，输出上限在 `api: anthropic-messages` 的 provider（如 `fo-ai-msg`）取 8,192，其余取 32,768。[窗口与模态](https://github.com/can1357/oh-my-pi/blob/v19.2.1/packages/coding-agent/src/config/model-discovery.ts#L1336-L1353) · [输出上限](https://github.com/can1357/oh-my-pi/blob/v19.2.1/packages/coding-agent/src/config/model-discovery.ts#L302) · [推理](https://github.com/can1357/oh-my-pi/blob/v19.2.1/packages/coding-agent/src/config/model-discovery.ts#L1086-L1121) · [默认值](https://github.com/can1357/oh-my-pi/blob/v19.2.1/docs/models.md#L650-L651) · [Anthropic 的默认输出上限](https://github.com/can1357/oh-my-pi/blob/v19.2.1/packages/coding-agent/src/config/model-discovery.ts#L177-L190)
  - 发现出的模型沿用 provider 的 `api` 与 `baseUrl` 发推理请求，`fo-ai-contents` 的推理请求是 `POST {baseUrl}/models/{id}:streamGenerateContent?alt=sse`，鉴权头 `x-goog-api-key`。[api](https://github.com/can1357/oh-my-pi/blob/v19.2.1/packages/coding-agent/src/config/model-discovery.ts#L1383-L1386) · [provider 的 baseUrl](https://github.com/can1357/oh-my-pi/blob/v19.2.1/packages/coding-agent/src/config/model-registry.ts#L2794-L2800) · [合并进发现的模型](https://github.com/can1357/oh-my-pi/blob/v19.2.1/packages/coding-agent/src/config/model-patch.ts#L88-L128) · [Contents 推理请求](https://github.com/can1357/oh-my-pi/blob/v19.2.1/packages/ai/src/providers/google.ts#L38-L44)
  - 由此，每条线路的列表要在上表的地址返回 OpenAI 形状的 `data[]`，推理根不变。
- 思考配置的协议差异由 ai-master 在 OMP 侧处理，不需要网关区分协议：
  - 行里的 `supported_reasoning_levels` 列出可用档位时，发现器把思考配置写成 `mode: effort`，Anthropic Messages 的模型保留按模型推出的 Anthropic 模式，只替换档位。[写成 effort](https://github.com/can1357/oh-my-pi/blob/v19.2.1/packages/coding-agent/src/config/model-discovery.ts#L1163-L1195)
  - `google-generative-ai` 只在 `mode: google-level` 时发 `thinkingLevel`，其他模式发 `thinkingBudget`，OMP 源码注明 Gemini 3 起的模型只用 `thinkingLevel`。所以 `gemini-4-neon` 这类 Gemini 3 起的模型，ai-master 用 `modelOverrides.<id>.thinking` 写回完整的 `mode: google-level`、`efforts` 与 `defaultLevel`，因为覆写替换整个 `thinking` 对象。[按模式选字段](https://github.com/can1357/oh-my-pi/blob/v19.2.1/packages/ai/src/stream.ts#L1760-L1782) · [写进请求](https://github.com/can1357/oh-my-pi/blob/v19.2.1/packages/ai/src/providers/google-shared.ts#L1085-L1099) · [覆写替换](https://github.com/can1357/oh-my-pi/blob/v19.2.1/packages/coding-agent/src/config/model-patch.ts#L303)
- 兼容：
  - 各列表现有的字段保持不变：Gemini 的 `models[]` 由 Gemini 原生客户端读，Anthropic 列表的 `input_modalities` 与 `supported_reasoning_levels` 已为 OMP 下发。
  - 各线路现有的鉴权保持不变，Gemini 线路仍是 `?key=` 优先，其次请求头。

### 线索

- 现状，2026-11-28 用 `Authorization: Bearer` 实测 `https://ai.chouun.com`，三个列表都是同样的 31 个模型：

  | 列表 | 形状 | 上下文窗口、输出上限 | 输入模态、推理档位 |
  | --- | --- | --- | --- |
  | `/v1/models` | `data[]` | 有 | 有 |
  | `/anthropic/v1/models` | `data[]` | 没有 | 有 |
  | `/gemini/v1beta/models` | `models[]` | 没有 | 没有 |

  `claude-opus-5-5` 一行在 `/v1/models`，节选：

  ```json
  {"id":"claude-opus-5-5","context_length":1000000,"max_output_tokens":128000,"input_modalities":["text","image","pdf"],"supported_reasoning_levels":["low","medium","high","xhigh","max"]}
  ```

  同一模型在 `/anthropic/v1/models`：

  ```json
  {"id":"claude-opus-5-5","type":"model","display_name":"claude-opus-5-5","created":"2026-11-18T10:29:22.860525Z","input_modalities":["text","image","pdf"],"supported_reasoning_levels":["low","medium","high","xhigh","max"]}
  ```

  `gemini-4-neon` 一行在 `/gemini/v1beta/models`：

  ```json
  {"name":"models/gemini-4-neon","baseModelId":"gemini-4-neon","version":"gemini-4-neon-30","displayName":"gemini-4-neon","description":"gemini-4-neon","supportedGenerationMethods":["generateContent","streamGenerateContent"]}
  ```

  同一模型在 `/v1/models` 带 `context_length: 1048576`、`max_output_tokens: 65536`、档位 `low` 到 `xhigh`。`GET /gemini/v1beta/v1/models` 返回 404。
- `/v1/models` 不带 `include` 就返回扩展字段，带 `?include=all` 结果相同。上游默认不返回，推断部署开启了系统设置 `default_model_api_include_all`，依据是 `internal/server/biz/system_default.go:92` 的默认值为 `false`。OMP 带不上 `include`，所以 `fo-ai-resp` 拿到元数据依赖这个设置。
- 代码，`release/v1.0.x`，`4f95e91b`：
  - `internal/server/api/openai.go:600-616`、`:650-660`、`:695-760`：`OpenAIModel`、`include` 的扩展字段，以及从注册模型的模型卡取元数据。
  - `internal/server/api/anthropic.go:85-92`、`:96`：`AnthropicModel` 只有 6 个字段，`ListModels` 从模型卡取 `input_modalities` 与 `supported_reasoning_levels`，不取上下文窗口与输出上限。
  - `internal/server/api/gemini.go:166-217`：`GeminiModel` 只有 6 个字段，`ListModels` 不读模型卡。
  - `internal/server/routes.go:307-309`、`:320-350`：Anthropic 组注册 `GET /models`，Gemini 组与 `/v1beta` 别名组都把 `GET /models` 注册到 `Gemini.ListModels`。
  - `internal/server/biz/model.go:768`、`:853`：三个列表都遍历 `ListEnabledModels`，它返回的 `ModelFacade` 带 `ConfiguredModel`，拿得到同一份模型卡。
  - `docs/routing/feats/model-admission.md` 的「模型列表」：OpenAI 与 Anthropic 列表为 OMP 自动发现下发 `input_modalities` 与 `supported_reasoning_levels`，没有涉及上下文窗口、输出上限与 Gemini 列表。
- 候选做法，都未实施：
  - Messages：`AnthropicModel` 加 `context_length` 与 `max_output_tokens`，取自模型卡，与 `/v1/models` 同名同值。
  - Contents，三选一：
    - 在 `models` 旁加 `data`，每行与 `/v1/models` 同形状：OMP 设 `injectV1: false`，所有调用 Gemini 列表的客户端都会多收一个字段。推断客户端会忽略未知字段，依据是 JSON 解析的通常做法，没有逐个核对 Gemini SDK。
    - 在 Gemini 组下加 `GET /v1/models`，返回 OpenAI 形状：OMP 用默认的 `injectV1`，不影响现有的 `models[]`。
    - 按请求头切换成 OpenAI 形状：OMP 在 provider 上配 `headers`。推断这个头也会随推理请求发送，依据是 `headers` 是 provider 级配置，没有核对推理链路。
  - 三条线路共用一处从模型卡生成这组元数据的代码，并且不依赖 `default_model_api_include_all`，以后加字段时三条线路一起变。
- 考虑过但没采用：
  - 用查询参数切换形状或带上 `include`：OMP 丢弃 `baseUrl` 的查询串，发现请求带不上。
  - 把 `fo-ai-contents` 的 `baseUrl` 指向 `/v1` 以读 `/v1/models`：推理也会发到 `/v1/models/{id}:streamGenerateContent`，实测 404。
  - 覆盖 OMP 内置的 `google` provider，用它的 Gemini 原生发现：19.2.1 的发现地址写死为 Google 官方端点，不随配置的 `baseUrl` 变。按代码推断，配置的网关密钥还会以 `?key=` 发给 Google，没有实测。[写死的地址](https://github.com/can1357/oh-my-pi/blob/v19.2.1/packages/catalog/src/discovery/gemini.ts#L7) · [不传 baseUrl](https://github.com/can1357/oh-my-pi/blob/v19.2.1/packages/catalog/src/provider-models/google.ts#L67-L77)
  - 在 Gemini `models[]` 的行里补 `inputTokenLimit`、`outputTokenLimit`：OMP 的自定义 provider 用不了 Gemini 原生发现，补了也读不到。

### 要的答复

- 每条线路选定的做法与理由，以及三条线路的元数据是否共用同一处代码、是否依赖 `default_model_api_include_all`。
- 实现所在的提交与代码位置，所属功能的文档，以及部署后三个 provider 要改的配置，至少写明 `fo-ai-contents` 的完整 provider 配置。
- 怎么验证：用 `Authorization: Bearer` 请求上表三个 OMP 将要请求的列表，三者都是 `data[]`、模型集合相同，`claude-opus-5-5` 与 `gemini-4-neon` 两行在三个列表里的 `context_length`、`max_output_tokens`、`input_modalities` 与 `supported_reasoning_levels` 相同；Gemini 的 `models[]` 与各列表原有字段不变。OMP 侧由 ai-master 验证：三个 provider 下出现同一组模型，`gemini-4-neon` 上下文 1,048,576、输出 65,536，推理走各自的推理根；加上思考配置的覆写后，`gemini-4-neon:high` 的请求体带 `generationConfig.thinkingConfig.thinkingLevel`，不带 `thinkingBudget`。
