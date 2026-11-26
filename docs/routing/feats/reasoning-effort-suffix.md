# 渠道识别档位后缀

> Parent: [../index.md](../index.md)
> Audience: developer — 开发和演进项目的人或 agent
> Genre: feat — 这个 feature 是什么、为什么值得、什么形态
> Visibility: public — 他人可读，避免写入本机环境
> Maintain: living — 持续更新，保持内容有效

有的上游把推理强度写进模型名，例如 `gemini-4-hydrogen-xhigh`。这里的渠道设置「识别档位后缀」
让这类渠道服务注册好的基础模型（如 `gemini-4-hydrogen`），并按请求的推理强度选出实际发给上游的模型名。
依赖它的是基础模型的路由、渠道测试与 Playground，以及渠道定价的标准价匹配。

## 动机

一个中转渠道的支持模型是 `gemini-4-hydrogen-xhigh`、`gemini-4-argon-high`、`gemini-4-carbon-high`，
注册的模型是基础名 `gemini-4-hydrogen`、`gemini-4-argon`、`gemini-4-carbon`。全局「自动推理强度」保持开启。
没有本功能时：

- 关联：基础模型用「全局精准匹配模型」关联时匹配不到渠道，只能逐个写正则。
- 渠道测试：「自动推理强度」先把 `gemini-4-hydrogen-xhigh` 拆成 `gemini-4-hydrogen` 加推理强度 `xhigh`，
  指定渠道里却没有 `gemini-4-hydrogen`，测试报 `model gemini-4-hydrogen not supported in channel <渠道名>`。
  Playground 指定渠道时也一样。
- 定价：渠道定价窗口的「一键匹配并填充」按渠道模型 ID 精确查标准价，找不到 `gemini-4-hydrogen-xhigh`。
  模型库价格兜底只去掉 `-high`、`-medium`、`-low`，`-xhigh` 与 `-max` 没有覆盖。

## 形态

- 开关：渠道「模型映射」窗口里的「识别档位后缀」，与「自动裁剪模型前缀」「模型名称转小写」同处，
  模型预览以「档位后缀」标出生成的基础名。默认关闭。
- 基础名：开启后，请求名形如 `<基础名>-<档位>` 的渠道模型额外提供 `<基础名>`。
  - 档位为 `low`、`medium`、`high`、`xhigh`、`max`，不区分大小写。
  - 拆分规则与「自动推理强度」相同：取最后一个 `-` 之后的部分；`qwen` 系列的 `-max` 是模型名的一部分，不拆。
  - 请求名包括自动裁剪前缀后的名字，因此 `google/gemini-4-hydrogen-xhigh` 配合裁剪前缀 `google` 同样得到 `gemini-4-hydrogen`。
  - 渠道已有名为 `<基础名>` 的模型（直连、前缀、裁剪或映射）时以已有模型为准，不生成别名。
  - 手动映射的请求名带档位后缀时，只有映射目标的档位相同才计入别名，例如 `m-high → upstream-low` 不会让 `m` 声称有 `high`。
  - 原有的带后缀模型名照常可用。
- 选档：每个请求按推理强度在该基础名的档位里选一个：

  | 请求的推理强度 | 选用的上游模型 |
  | --- | --- |
  | 未指定 | `-medium`；没有时取最接近 medium 的档位，距离相同取较低档 |
  | 指定且渠道有该档位 | 该档位，精确匹配 |
  | 指定但渠道没有该档位 | 用渠道「推理强度映射」换算一次，渠道有换算后的档位就用它 |
  | 以上都没有 | 该渠道不服务这个请求 |

  - 指定的推理强度包括 `none`、`minimal`，以及 Anthropic、Gemini 等协议入站时按原生 thinking 参数换算出的值。
  - 不就近升降档。只有某个基础名缺少请求的档位时才查映射，映射是渠道级的 From/To，
    所以 `high → xhigh` 不会把 `gemini-4-argon-high` 换成不存在的 `-xhigh`。
  - 所有匹配的渠道都因缺少档位被排除时，请求返回无效模型错误（OpenAI 入站为 422 `invalid_model_error`，
    Playground 为 422），错误信息写明缺少的推理强度。有渠道提供该档位、因其他原因（如缺少所需端点）被排除时，不报缺档。
  - 通过基础名选中档位后，请求里的推理参数不再发给上游，以模型名里的档位为准；渠道推理强度映射不再作用于这次请求。
    该渠道开启了请求体透传时，这次尝试改用转换后的请求体，不回放客户端原始请求体。
    「自动推理强度」开启时，客户端请求 `m-high` 会先拆成 `m` 加 `high`，同样走基础名，推理参数同样不发送；
    只有指定渠道（测试、Playground）且渠道有原名时才按原名发送，推理参数照常保留。
  - 选出的上游模型名用于健康门控、配额、计价和 Responses 续接来源，与直接请求该模型一致。
    别名条目的请求名仍是基础名；按模型的协议覆盖还会匹配选中档位的请求名（如 `gemini-4-hydrogen-xhigh`）。
  - 同一关联同时匹配到基础名与带后缀的模型（如正则 `^m(-high)?$`）且两者指向同一上游模型时，固定走基础名的处理，
    不随匹配顺序改变上游请求体。
- 关联：基础模型用「全局精准匹配模型」即可匹配这类渠道。关联到基础名后，「未关联的渠道模型」不再列出它的带后缀模型。
- 渠道测试与 Playground：指定渠道且该渠道有原样的模型名时，「自动推理强度」不拆分模型名。
  这一条与开关无关，测试 `gemini-4-hydrogen-xhigh` 直接发 `gemini-4-hydrogen-xhigh`。
- 定价：渠道模型的标准价按以下顺序取第一个有价格的模型：
  1. 渠道模型 ID 本身；
  2. 去掉档位后缀的基础名；
  3. 去掉自动裁剪前缀后的名字，以及它的基础名（仅定价窗口）。

  价格仍保存在渠道模型 ID 下。规则用于定价窗口的「一键匹配并填充」和逐行选模型，以及未设价时的模型库价格兜底，与此开关无关。
  手动模型映射不参与价格匹配。渠道创建和模型同步时的自动补价仍只按渠道模型 ID 精确查找，
  不写入基础名的价格，以免基础名在模型库调价后被渠道显式价格挡住。
  各处只取启用或停用状态的模型，归档模型不提供价格；价格须有计价项、阶梯或时段覆盖，空价格不挡住后面的候选。

## 边界

- 只改渠道侧的模型名解析，不改「自动推理强度」的系统设置语义，也不改模型关联类型。
- 不清理透传请求以外的原始请求体；开启请求体透传的渠道选中档位后缀模型时放弃透传请求体，响应透传不受影响。
- 「推理强度映射」的编辑与校验见上游的渠道转换选项，本功能只在缺档时读取它。
- 定价的引用与倍率见[渠道定价](../../billing/feats/channel-pricing.md)，渠道测试的超时与事务见[渠道测试](../../channels/feats/channel-test.md)。

## 实现与维护

- 提交：`74a26251`（`feat(channels): 渠道识别模型名中的推理档位后缀`）
- 代码：
  - `llm/reasoning.go`：档位后缀的拆分规则，自动推理强度与定价共用。
  - `internal/objects/channel.go`、`internal/server/biz/channel_settings.go`：渠道设置字段 `reasoningEffortModelSuffix`。
  - `internal/server/biz/channel_model_effort.go`、`internal/server/biz/channel_llm.go`：基础名别名与按推理强度选档。
  - `internal/server/biz/model.go`：未关联渠道模型的判定。
  - `internal/server/orchestrator/candidates_effort.go`、`candidates_condition.go`、`candidates.go`：候选选档与缺档错误。
  - `internal/server/orchestrator/outbound.go`、`pass_through.go`：清除推理参数、跳过请求体透传。
  - `internal/server/orchestrator/auto_reasoning_effort.go`：指定渠道时不拆分渠道自有的模型名。
  - `internal/server/biz/channel_price.go`：模型库价格兜底的候选模型。
  - `internal/server/gql/axonhub.graphql`
  - `frontend/src/features/channels/components/channels-model-mapping-dialog.tsx`
  - `frontend/src/features/channels/components/channels-model-price-dialog.tsx`
  - `frontend/src/features/models/data/reasoning-efforts.ts`：前端的拆分规则与价格候选，与 `llm/reasoning.go` 保持一致。
- 迁移：无 schema migration 或 data migration。设置存于渠道 `settings` JSON，旧数据缺省为关闭。
  渠道模型条目只在进程内缓存，档位表不序列化，无缓存兼容问题。
- 测试：无新增 fixture。
  - `llm/reasoning_test.go`
  - `internal/server/biz/channel_model_effort_test.go`：别名生成、与裁剪前缀和转小写的组合、选档与映射、默认档位、未关联判定。
  - `internal/server/biz/usage_cost_test.go`：价格候选覆盖 `-xhigh`、`-max` 与 qwen 例外。
  - `internal/server/orchestrator/candidates_effort_test.go`：候选选档、缺档错误只在档位清空候选时报告、
    基础名优先于同一上游的带后缀条目、档位请求名参与协议覆盖、映射只在缺档时生效、指定渠道选档、
    清除推理参数、跳过请求体透传。
  - `internal/server/orchestrator/auto_reasoning_effort_test.go`：指定渠道时保留渠道自有的模型名。
  - `internal/server/api/chat_test.go` 的 `TestPlaygroundHandleError_ReasoningEffortUnavailable_Returns422WithEffort`。
  - `frontend/src/features/models/data/reasoning-efforts.test.mjs`：拆分规则与后端一致、价格候选顺序、有效价格判定。
  - `frontend/src/features/channels-override-merge.test.mjs`
- 同步上游：
  - 上游改动 `GetModelEntries`、`filterResolvedCandidatesForRequest` 或 `SpecifiedChannelSelector` 时，
    保留别名生成、聚合前选档和指定渠道选档。
  - 上游调整自动推理强度的后缀列表时，同步 `llm.ModelSuffixReasoningEfforts` 与前端的列表。
  - 上游的价格兜底后缀来自 `cc1c7e5d`（PR #2655），本功能把它换成共享的拆分规则；上游再改这份列表时以共享规则为准。
