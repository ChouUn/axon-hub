# API Key 自助用量页

> Parent: [../index.md](../index.md)
> Audience: developer — 开发和演进项目的人或 agent
> Genre: feat — 这个 feature 是什么、为什么值得、什么形态
> Visibility: public — 他人可读，避免写入本机环境
> Maintain: living — 持续更新，保持内容有效

用 API Key 登录的独立只读页（`/self-usage`）：只持有 Key 的人在这里查看这把 Key 自己的用量、成本、
按天趋势和请求日志，不需要管理员账号。

## 动机

- 只持有 API Key 的使用者无法查看自己的用量：
  - 普通推理 Key 可达的路由只有推理与模型列表（`58f73057~1` 的 `internal/server/routes.go:170-259`）。
  - 上游 `v1.0.0-beta10` 的 `apiKeyQuotaUsages`（上游 PR #1779、#1838）对普通推理 Key 不可用
    （`internal/server/middleware/auth.go:141-170`、
    `internal/server/gql/openapi/openapi.graphql:149-182`、`internal/scopes/rule_apikey_scope.go:78-87`）：
    - 只接受 service_account 类型的 Key，需要 `read_api_keys`。
    - 可查同项目内隐私规则允许的非 personal Key。
    - 只返回已配额度的 profile。
  - 管理端的 Key 统计与请求列表需要用户 JWT。按项目授权后可见项目内非 personal Key 及本人的
    personal Key，项目 owner 权限更宽（`internal/scopes/rule_user_project_scope_requests.go:17-19`）。
- 要覆盖的场景：拿到 Key 的人自己能看到用了多少、花了多少、哪些请求失败，不需要管理员账号，
  也看不到别人的数据。

## 形态

D 编号是确定形态的决策，R 编号是行为规则。

- D1 只做页面：用 Key 登录的独立只读用量页，不进入管理后台。
  页面背后的查询接口不作为对外契约，不写入用户文档的 API 说明。
- D2 只看本 Key：统计、趋势、日志只含登录所用 Key 自身的请求。
  不汇总创建者名下的其他 Key，也不汇总项目。
- D3 内容：用量统计、按天趋势、逐条日志。不展示额度。
- D4 可用范围：所有 `user` 与 `personal` 类型的 Key 默认可用，不设开关。
- D5 登录保持：
  - Key 只保存在当前标签页，同一标签页刷新后保持登录。
  - 关闭标签页或登出即结束页面登录状态，不吊销 Key。
  - 页面每次查询携带该 Key 认证，后端不新增登录凭证类型。
- D6 失败请求：
  - 日志列出本 Key 的全部请求并标出成功或失败。
  - 请求数分成功、失败两项。
  - Token 与费用按已落库的用量记录计入。失败或取消的请求若留有用量记录也计入，与渠道上游的实际消耗一致。
- D7 时间范围：
  - 预设今天、昨天、近 7 天、近 30 天、本月。
  - 可自定义起止日期，跨度最长 90 天。
  - 自然日边界使用系统设置中的时区。
- D8 查询接口为独立 GraphQL 入口 `POST /self-service/v1/graphql`。
  - 独立 gqlgen 包，不改管理端与 OpenAPI 的 schema。
  - 只注册 POST，不开 playground。
- D9 页面地址 `/self-usage`，管理端登录页有一行指向该页的链接。
- D10 限流：
  - 同一 IP 每分钟认证失败 20 次后，该 IP 在窗口剩余时间内提交错误 Key 一律 429。
  - 正确 Key 不受该计数影响，认证失败计数表已满时也照常放行。
    这样伪造转发头锁不死他人。轮换 IP 的爆破由 Key 长度抵御。
  - 认证成功后每个 Key 每分钟最多 60 次查询，查询计数表另计。
  - 两张计数表各上限 10000，进程内计数，多实例不共享。
- D11 只统计 `source=api` 的请求，不含 Playground 与测试请求。
- D12 请求日志与管理端对齐：请求日志复用管理端请求列表的共享单元格组件，管理端请求列表的对应列用同一组件。
  - 列依次为：
    1. ID：`#编号`、请求原始状态徽章、流式或非流式徽章。
    2. 模型。
    3. Token：总计 / 输入 | 输出 / 推理。
    4. 读缓存：数量与命中率。
    5. 写缓存：数量与写入率。
    6. 成本：悬浮明细见[请求成本明细悬浮](../../billing/feats/request-cost-breakdown.md)。
    7. 耗时：仅已完成请求。流式显示首字与总耗时，非流式显示总耗时。
    8. 创建时间。
  - 不设单独的状态列与流式列。
  - 管理端专有列不出现：渠道、客户端 IP、调用方、API 格式、透传与协议标记、详情入口。
- D13 措辞：本页金额统一称「成本」（汇总卡、趋势、按模型统计、请求日志），与管理端请求列表一致。
  不改管理端与分析页文案。

## 行为规则

- R1 认证：
  - 只接受启用状态、所属项目为 active 的 `user`、`personal` Key。
  - `service_account`、`noauth`、禁用或归档的 Key 拒绝，不回退到系统 noauth Key。
    推理路由的 `WithAPIKeyConfig` 会回退（`internal/server/middleware/auth.go:27-29,52-54`），
    所以本页用专用认证入口，不改推理路由。
  - Key 的 IP 白名单照常生效。
  - Key 只经 `Authorization: Bearer` 请求头传输，不进入 URL，日志不记录明文。
  - 用户被停用或删除时，其 personal Key 随之禁用或归档
    （`internal/server/biz/user.go:227-231,626-629`），沿用该联动。
- R2 隔离：
  - 后端以认证 Key 的 ID 作为所有查询的显式条件，即使查询走系统 bypass 也保留该条件。
  - 查询参数不接受 Key 的 ID、名称或明文。
  - 访问不属于本 Key 的请求按不存在处理。
- R3 统计口径：
  - 时间锚点统一为请求创建时间（`requests.created_at`）。用量经 `request_id` 归属到所属请求，
    跨日完成的请求其用量计在请求创建日，保证统计、趋势与日志可对账。
  - 请求数按请求表每行一次：`completed` 计成功，`failed` 与 `canceled` 计失败，进行中的请求不计入。
    不与用量表连接计数，因为一次请求可有多条用量记录。
  - Token（输入、输出、缓存读取、缓存写入、推理）与费用为该请求全部用量记录之和，
    不按请求最终状态过滤（D6）。缺价不影响 Token 统计。
  - 模型统一使用 `requests.model_id`，即 API Key 模型映射后的请求模型，不是客户端原始别名。
    不使用 `usage_logs.model_id`（上游实际模型）展示、分组或筛选。
- R4 趋势：
  - 后端按系统时区的自然日分桶并补零，返回日历日期。前端不按浏览器时区重新分桶。
  - 展示请求数（成功、失败）、Token 与成本。
- R5 日志：
  - 按请求创建时间倒序、同时间按 ID 倒序分页，默认每页 20 条，后端上限 100 条。列见 D12。
  - 时间按系统时区显示并标注时区。
  - 可按时间范围、模型与状态筛选。筛选为成功、失败、进行中三类，行内徽章显示请求原始状态。
  - 不展示渠道、上游实际模型、请求与响应正文、错误详情。
- R6 成本：按系统设置的币种格式显示。
  - 单条用量未定价不按 0 计入。
  - 渠道未设价、由模型库价格兜底计费的用量算已定价，见[渠道定价](../../billing/feats/channel-pricing.md)。
  - 汇总（每日、每模型、总计）：

    | 情形 | 显示 |
    | --- | --- |
    | 无用量 | 0 |
    | 有用量但全部未定价 | 「—」 |
    | 部分未定价 | 已知成本，并标注未定价记录数 |
    | 真实零价 | 0 |

  - 日志行按 D12 与管理端一致：

    | 情形 | 显示 |
    | --- | --- |
    | 无用量记录 | Token、缓存与成本列为「-」 |
    | 全部未定价 | 成本为「-」 |
    | 部分未定价 | 已知成本，未定价记录数在成本悬浮中标注 |

- R7 页面：
  - 独立公开路由，不经管理端登录。提供登出。中英文界面。
  - 页面使用独立的 Key 认证状态、请求与错误处理，不附加管理端 JWT 与项目上下文，
    不清除或改写管理端登录。管理端的全局 401 处理会清除管理登录并跳转 `/sign-in`
    （`frontend/src/main.tsx:82-87`、`frontend/src/gql/graphql.ts:103-111,147-155`）。
  - 认证失败只清除本页 Key 并回到本页登录。
  - 根布局的管理端查询与命令菜单不在本页触发。
  - Key 只存于本页的 sessionStorage 与内存，不进入管理端 `authStore`、localStorage 或 URL。
  - 登出或换 Key 时清除旧查询缓存。
- R8 元数据：页面查询接口随结果提供系统币种与时区，不要求 `read_settings`，不暴露其他系统设置。
  管理端 `useGeneralSettings` 需要 `read_settings`
  （`frontend/src/features/system/data/system.ts:1124-1145`）。
- R9 查询开销：
  - 时间跨度由后端校验，不超过 90 天。
  - 过滤条件为 Key ID、项目 ID 与 `source=api` 等值加 UTC 半开时间范围，时区换算只用于分桶。
  - 请求查询须命中 `requests_by_api_key_id_created_at`。
  - 用量经 `request_id` 关联（`usage_logs_by_request_id`）。

## 边界

- 与上游 OpenAPI `apiKeyQuotaUsages` 功能不重叠，不修改其权限。
- 不复用管理端分析组件：上游 PR #2510 重做了管理端分析页，分开可避免交叉冲突。
- 成本悬浮的内容由[请求成本明细悬浮](../../billing/feats/request-cost-breakdown.md)规定。
- 不做的事见 D1、D2、D3、D11。

## 链接

- [CCH 对照与取舍](../../reports/cch-comparison.md)：页面形态的对照，以及范围与口径上的不同取舍。
- 用户文档：[自助用量页](../../zh/guides/self-usage.md)。

## 实现与维护

- 提交：`58f73057`、`1c5509ae`。
- 代码，新文件：
  - `internal/server/middleware/self_service_auth.go`：严格 Bearer、仅 user/personal、无 noauth 回退、
    IP 白名单。
  - `internal/server/middleware/self_service_rate_limit.go`：D10，进程内固定窗口，计数表上限 10000。
  - `internal/server/biz/self_usage.go`：
    - 按 Key、项目与 `source=api` 过滤后窄 bypass 查询。
    - UTC 15 分钟桶按系统时区归日并补零。
  - `internal/server/gql/selfusage/`：独立 gqlgen 包，仅 POST。
  - `frontend/src/routes/self-usage.tsx`
  - `frontend/src/features/self-usage/`：独立 fetch、sessionStorage、页面内 QueryClient。
  - `frontend/src/locales/{en,zh-CN}/selfUsage.json`
- 代码，上游文件只做追加：
  - `internal/server/server.go`：handler 构造器。
  - `internal/server/routes.go`：`/self-service` 路由组。`Handlers` 新增 `SelfUsageGraphql` 字段，
    其余字段的差异只是 gofmt 重新对齐的缩进。
  - `internal/server/biz/fx_module.go`
  - `internal/server/static/embed.go`：API 前缀。
  - `frontend/src/routes/__root.tsx`：`/self-usage` 跳过标题同步、初始化守卫与命令菜单。
  - `frontend/src/features/auth/sign-in/index.tsx` 与 `frontend/src/locales/{en,zh-CN}/base.json`：登录页链接。
  - `frontend/vite.config.ts`：开发代理。
  - `frontend/src/routeTree.gen.ts`：Vite 插件生成。
- 用户文档：`docs/{zh,en}/guides/self-usage.md`，并在 `docs/{zh,en}/index.md` 指南表格追加链接。
  不列内部查询接口。
- 迁移：无 schema migration、data migration 与缓存键变化。
- 测试：
  - `internal/server/middleware/self_service_auth_test.go`：
    - 认证矩阵：缺失、非 Bearer、伪造、AllowNoAuth 开启、service_account、禁用 Key、归档项目、IP 白名单。
    - 失败超限后错误 Key 429 而正确 Key 放行。
    - 伪造转发头与失败计数表已满均不影响正确 Key。
  - `internal/server/middleware/self_service_rate_limit_test.go`：失败计数、Key 查询上限、窗口重置、
    计数表上限。
  - `internal/server/biz/self_usage_test.go`：隔离、多条用量、失败与取消用量、进行中请求、跨日归属、
    费用空值与部分未定价、补零、Asia/Kolkata 与 America/Los_Angeles 夏令时、参数校验、
    日志分页不加载请求与响应正文等大字段。
  - `internal/server/gql/selfusage/graphql_test.go`：经真实中间件链的 401/403/429、GET 拒绝、
    BAD_USER_INPUT、alias 与 fragment 重复根字段在执行前拒绝。
  - `internal/server/static/embed_test.go`：`/self-service` 未知路径返回 JSON 404。
  - `frontend/src/features/self-usage/{date-range,cost}.test.mjs`：时区与夏令时的日期预设、费用展示。
  - 无新增 fixture。
  - MySQL 与 PostgreSQL 的分桶表达式未在真实数据库上运行验证。
- 同步上游：新路由组在 `internal/server/routes.go` 追加。
