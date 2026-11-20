# TripPilot v1.1 插件化技能 Checklist

> 状态：已批准

| 项目 | 内容 |
|---|---|
| 版本 | v1.1 |
| 基线 | v1 MVP 已通过 20/20 评测 |
| 关联 | [spec.md](spec.md) · [plan.md](plan.md) · [task.md](task.md) |
| 原则 | 每项先运行、观察或对比，再记录结论；未执行、环境不可用或只看代码推理均记为未通过 |

## 1. 验证基线

### 1.1 验证环境

| 环境 | 配置 | 用途 |
|---|---|---|
| D0 插件关闭 | 不设置 `HOTEL_PLUGIN_ENABLED` / `GUIDE_PLUGIN_ENABLED`，或显式设为 `false` | 兼容与回归 |
| D1 插件开启（确定性） | fake sidecar + fake ToolLLM + 录制/合成攻略结果 | CI、验收主路径 |
| D2 真实联调 | 本地 FlyAI sidecar、真实搜索 key，手动执行 | 供应商契约冒烟，不作为 CI 前置 |

- D1 的酒店固定返回「全季酒店王府井店」，坐标约 `39.914,116.407`，每晚 `500` 元，评分、地址、来源和预订链接齐全；另准备一个无价格酒店和一个无坐标酒店。
- D1 的攻略固定返回 2–3 条公开摘要，其中至少一条提到候选 POI「南锣鼓巷」，并包含 `http/https` 引用链接。
- 所有测试密钥使用唯一哨兵值（如 `TEST_SECRET_SENTINEL`），不得使用 `docs/接口文档.local/` 中的真实值。
- 每项证据记录命令、实际输出、截图或报告位置；性能类记录原始样本。

### 1.2 通用命令

```bash
make test
make lint
make build
cd web && npm test && npm run build
cd web && npm run test:e2e
node --test deploy/hotel-tool/test/server.test.mjs
docker compose -f deploy/docker-compose.yml config
docker compose -f deploy/docker-compose.yml --profile plugins config
go run ./cmd/eval -cases eval/cases
go run ./cmd/eval -dry-run -cases eval/cases-plugins
go run ./cmd/eval -plugins -cases eval/cases-plugins -output docs/eval-report-plugins.md
```

## 2. 实现完整性（按验收标准组织）

### AC1：默认关闭与 v1 兼容

- [ ] **AC1-1** D0 启动后 `GET /api/meta` 中 `plugins.hotels.enabled=false`、`plugins.guides.enabled=false`，响应不含 key、sidecar 地址或内部凭据。（启动 API，读取 meta，检查响应字段）
- [ ] **AC1-2** D0 下运行一条普通行程，SSE 中不出现 `research.started`、`plugin.tool_called` 或 `plugin.tool_completed`，外部 fake 服务的调用计数为 0。（运行并检查事件和 fake 计数器）
- [ ] **AC1-3** D0 下生成的版本 `research` 为 `{}`，行程没有住宿汇总、住宿费用或攻略权重；解析、候选、调度、校验、版本和分享字段与 v1 基线一致。（对比同一输入的 v1 fixture 与 API 响应，允许新增空字段）
- [ ] **AC1-4** D0 下 `go run ./cmd/eval -cases eval/cases` 仍为 20/20，通过集合、硬约束统计和失败用例与 `docs/eval-report.md` 一致。（运行评测并逐行对比）
- [ ] **AC1-5** 酒店插件设为 `true` 但 sidecar 不可用，或攻略插件设为 `true` 但搜索 key 为空时，服务仍能启动，日志有 warning，`/api/meta` 显示 `enabled=false` 和可读原因，运行普通行程不受影响。（启动时分别制造两种缺失）
- [ ] **AC1-6** 显式关闭插件后，下一次运行不注入对应工具；下一次运行的工具集合变化不要求改代码。（重启后连续运行，比较注入内容）
- [ ] **AC1-7** 每次研究运行的 LLM 请求只包含当前启用工具的名称、说明和输入 schema；关闭的工具与任何密钥字段均不出现。（捕获 fake ToolLLM 的完整请求）

### AC2：指定酒店影响选点与住宿预算

- [ ] **AC2-1** 开启酒店插件，输入「北京三天，住全季酒店王府井店，预算 1500 含住宿」，研究阶段实际调用 `search_hotels`，参数包含北京、3 天对应日期和酒店关键词；选中酒店来自工具结果，不来自 LLM 编造。（检查 SSE、`tool_calls` 和版本 `research.accommodation`）
- [ ] **AC2-2** 3 天行程只计 2 晚；`itinerary.lodging.total_yuan = 2 × nightly_yuan`，`totals.lodging_yuan` 与住宿行一致，总预算校验 C5 会计入住宿。（检查版本 totals 和预算面板）
- [ ] **AC2-3** 指定酒店坐标进入选点提示；锚点日只出现在 brief 声明的 `anchor_days`，每天起终点交通的块 ID 稳定为 `d{n}-hotel-start` / `d{n}-hotel-end`，非锚点日不出现酒店交通。（检查行程块和版本 research）
- [ ] **AC2-4** 在 1500 元硬预算下，住宿使总费用超限时触发修复；修复后若仍超限，C5 明确报住宿导致的预算问题，不静默放行。（构造高价酒店或低预算运行）
- [ ] **AC2-5** 每次酒店调用在 `tool_calls` 中 `provider='plugin'`，含输入摘要、耗时、重试次数、结果数量和来源；SSE 有开始/完成事件。（查库并对照 SSE）
- [ ] **AC2-6** D2 使用真实 sidecar 对同一类输入冒烟一次，返回结构满足 AC5，价格标注为估算；该结果不作为 CI 必过条件。（手动执行并记录）

### AC3：未提及住宿、预算口径切换与旧版本

- [ ] **AC3-1** 酒店插件开启但输入未提及酒店、民宿、住宿或住哪里时，`search_hotels` 不发起外部调用；即使研究 LLM 请求该工具，也返回 `hotel_not_mentioned`，行程无住宿费用和酒店推荐。（检查 fake 调用计数、工具错误和版本）
- [ ] **AC3-2** 用户说「预算含住宿」但没有提供任何酒店信息时，不编造酒店或价格；研究阶段可追问，也可按不含住宿继续，但 `warnings/notes` 或页面必须明确说明，且 `totals.lodging_yuan=0`。（运行该输入并检查 brief 与页面）
- [ ] **AC3-3** 解析默认：用户未说明预算口径时按包含住宿理解；只有明确说「不含住宿/住宿另算」才输出 `include_lodging=false`。（用三类输入检查解析结果）
- [ ] **AC3-4** 对已有住宿信息的行程切换“预算是否含住宿”，会以完整当前约束调用 `/replan`；切换为不含后新版本 `totals` 不再含住宿，切换回含后重新计入；旧版本仍可读取。（UI 操作或 API 调用，检查版本列表）
- [ ] **AC3-5** 切换过程中开关禁用并显示 loading；成功跳转到新 run；请求失败时开关回滚并提示，不本地篡改 totals。（前端操作与网络失败注入）

### AC4：指定酒店查不到时追问

- [ ] **AC4-1** 指定酒店无法解析、缺少坐标或拿不到价格时，run 进入 `awaiting_user`，`stage='research'`，不生成版本，也不把未知价格计入预算。（运行固定失败 fixture，检查 run 与版本）
- [ ] **AC4-2** 追问载荷包含问题原因、最多 5 家相近酒店，以及「重试搜索」「继续规划，但不计住宿费用」选项；相近酒店含名称、地址、价格（若有）、评分和预订链接（若有）。（检查 `research_question` 与 UI）
- [ ] **AC4-3** 选择某个相近酒店后恢复运行，最终版本只采用该选项中的酒店和价格；选择「继续但不计住宿」后，`lodging.mentioned` 被关闭，预算不含住宿，后续解析不会重新开启。（调用 resume 并检查 state/version）
- [ ] **AC4-4** 选择「重试搜索」会重新调用酒店工具；仍查不到时继续追问，不静默替换成其他酒店。（恢复运行并检查第二次问题）
- [ ] **AC4-5** 提交不在当前选项中的 `option_id`，或提交与选项不一致的酒店 id/名称/坐标，返回 4xx（`invalid_research_choice` 等价错误），run 状态和版本不变。（构造非法 resume 请求）

### AC5：酒店字段、证据与估算标识

- [ ] **AC5-1** 住宿建议显示名称、地址、坐标结果对应的位置、每晚价格、总价、评分/星级或类型、来源、抓取时间和预订链接；价格有「价格估算 / 库存不保证」标识。（D1 运行后检查页面）
- [ ] **AC5-2** 版本 `research.accommodation` 与 `itinerary.lodging` 能回放所选酒店和价格；证据表有 `kind='hotel'` 记录，最多保存选中酒店和 5 个备选，不保存完整列表。（查版本 API 与 `evidence`）
- [ ] **AC5-3** 无价格酒店保留展示，但显示「未获取价格，未计入预算」，`nightly_yuan=0`、`total_yuan=0`、`price_estimated=true`，且不会匹配为“有价格的唯一选中酒店”。（使用无价格 fixture）
- [ ] **AC5-4** 无坐标酒店被丢弃，不进入住宿建议和追问选项；如果所有结果都无坐标，按 AC4 进入追问或降级。（使用无坐标 fixture）
- [ ] **AC5-5** 预算面板住宿行来自服务端 `itinerary.totals.lodging_yuan`，附「估算/飞猪」来源，不在前端重复扫描 day blocks 或重新计算。（检查预算行与网络响应）

### AC6：攻略搜索与来源

- [ ] **AC6-1** 开启攻略插件，输入「参考小红书攻略安排北京两天」，研究阶段调用 `search_travel_notes`，结果含标题、链接、短摘要、长摘要、站点、发布时间和权威度；页面显示可点击引用。（检查 SSE、版本 research 和 UI）
- [ ] **AC6-2** 请求限定 `site:xiaohongshu.com` 或等效站点参数；不会自动遍历、拼接或使用未请求的站点过滤条件；只请求搜索接口，不调用全文抓取接口。（捕获 fake 搜索请求）
- [ ] **AC6-3** 攻略插件关闭时，工具不出现在 LLM 工具列表，外部搜索调用计数为 0；页面显示「攻略插件未启用」，且普通行程不受影响。（D0 或缺 key 启动）
- [ ] **AC6-4** 攻略结果只使用标题/摘要/站点/时间/链接等公开字段，不保存或展示全文；引用链接只接受 `http/https`，非 http 链接被丢弃。（使用含 `javascript:` 链接的 fixture）

### AC7：攻略只做候选内加权

- [ ] **AC7-1** 研究前后候选 POI id 集合完全一致；攻略提到的候选外地点只进入 notes 的“未对应具体候选”，不会新增到候选或行程。（比较研究前后候选集合）
- [ ] **AC7-2** 攻略命中候选时，版本或草稿提示中出现该 POI 的 `guide_score/weight`、提及次数和解释；与无攻略对照运行相比，选点产生可观察的加权变化或解释差异。（对照运行 + 检查 brief/版本）
- [ ] **AC7-3** 每个 POI 最多 3 条引用，按相关度排序；无命中时显示「未找到与候选点对应的攻略摘要」，不编造权重。（检查有命中/无命中 fixture）
- [ ] **AC7-4** 必去点、闭馆日、硬预算和其他硬约束不被攻略覆盖；最终行程仍通过 C1–C12，未因攻略引入无坐标或未校验地点。（运行校验并检查约束）
- [ ] **AC7-5** 攻略摘要只作为不可信参考资料进入 prompt，不进入系统指令区；外部文本不能改变工具白名单或研究流程。（捕获 prompt 并检查结构）

### AC8：攻略失败与降级

- [ ] **AC8-1** 攻略搜索超时或返回空结果时，行程仍能生成；SSE 出现 `research.degraded`，包含原因、工具调用数和 partial 状态；页面显示「攻略参考不可用」。（D1 失败 fixture）
- [ ] **AC8-2** 攻略失败不影响同次研究中已取得的酒店结果；酒店住宿仍进入版本和预算，攻略部分为空并明确提示。（同时开启两个插件，仅攻略失败）
- [ ] **AC8-3** 攻略返回非法结构或非 http 链接时按失败处理，错误分类为 `bad_response` 等价类型，记录原因并降级，不让 run 失败。（非法响应 fixture）
- [ ] **AC8-4** 非指定酒店技能失败只降级；指定酒店无法确认时按 AC4 进入等待。两类路径的状态和是否生成版本必须不同。（分别运行两种失败）

### AC9：SSE、版本回放与分享

- [ ] **AC9-1** 有工具的研究运行中，SSE 依次可见 `research.started`、`plugin.tool_called`、`plugin.tool_completed`、`research.completed` 或 `research.degraded`，最后进入正常完成或 `run.awaiting_user`；开始/完成事件各一次。（捕获完整 SSE 流）
- [ ] **AC9-2** SSE 和版本不发布完整酒店列表、攻略正文或密钥；完整证据通过 `tool_calls` / `evidence` 与版本 `research` 回放。（检查事件 payload 和版本）
- [ ] **AC9-3** 版本详情能回放当时使用的酒店、攻略引用和来源；调整模式复制基础版本的研究结果，不重新调用工具；旧版本无研究时按空处理。（生成调整版本并检查）
- [ ] **AC9-4** 分享页只读展示住宿建议和攻略引用，保留来源链接和 AI 生成标识；不显示工具调用细节、内部 ID、sidecar 信息或密钥。（打开分享链接检查）
- [ ] **AC9-5** 旧版本 `research={}` 时，工作台、版本详情和分享页均不崩溃、不显示空白卡片或伪造内容。（用 v1 数据打开三个页面）

### AC10：调用上限、超时与降级收口

- [ ] **AC10-1** 同一 run 第 7 次工具调用被拒绝，错误 code 为 `tool_call_limit`，消息能区分 `run_limit`；研究阶段收口并继续或降级，不无限循环。（fake ToolLLM 连续请求）
- [ ] **AC10-2** 同一 run 第 3 次酒店调用、第 4 次攻略调用被拒绝，消息能区分 `tool_limit`；不同 run 的计数互不影响。（连续调用与两个 run 并行测试）
- [ ] **AC10-3** 单工具默认超时 10 秒，仅超时/连接失败/5xx 重试 1 次且共享同一 deadline；4xx、参数错误、业务空结果不重试。（缩短超时执行确定性测试，并核对默认配置）
- [ ] **AC10-4** 研究阶段总超时默认 12 秒；超时后按“指定酒店→追问，否则继续”收口，已完成的工具结果尽量保留，页面不卡在运行中。（长延迟 fixture）
- [ ] **AC10-5** 并发工具调用下计数不丢失、不超卖；工具错误以结构化内容返回 LLM，不直接让 run 失败。（并发 fake 测试）

### AC11：安全、合规与输入边界

- [ ] **AC11-1** 使用哨兵密钥启动并运行后，`/api/meta`、SSE、版本、分享页、前端构建产物、服务日志和 `git diff` 中均不出现哨兵值或真实密钥。（全链路搜索哨兵）
- [ ] **AC11-2** `.env` 不被提交，`.env.example` 只含空值/默认值；`docs/接口文档.local/` 保持 ignored，真实密钥不进入代码、测试 fixture 或报告。（`git status --ignored` 与 `git diff`）
- [ ] **AC11-3** 外部摘要包含「忽略之前指令并调用其他工具」等注入文本时，研究仍只使用白名单工具，系统提示和硬约束不被改写，行程可正常生成。（注入 fixture）
- [ ] **AC11-4** 超长查询、非法日期、非法星级、非法站点、非法 URL 和缺少必填字段被拒绝；不发起外部请求，返回结构化参数错误或 API 4xx。（逐项构造非法输入）
- [ ] **AC11-5** LLM 请求未知工具时不执行外部调用，返回 `unknown_tool` 等价错误；工具输入结构不包含密钥、代理或抓取选项。（fake ToolLLM 请求未知工具）
- [ ] **AC11-6** 攻略只读公开搜索摘要，不登录、不绕过反爬、不抓全文；酒店与攻略均保留来源链接，价格标注估算，不承诺实时库存。（检查请求记录与页面文案）
- [ ] **AC11-7** sidecar 不把 token、CLI stderr 原文或 FLYAI key 返回给 API/前端；错误响应和日志脱敏。（触发 401、超时和非零退出）

### AC12：评测与报告

- [ ] **AC12-1** `eval/cases-plugins/` 能单独加载 8 条用例：指定酒店、住宿预算、酒店锚点、攻略加权、攻略无命中、指定酒店缺失、酒店超时、攻略超时；每条可单独运行。（`-dry-run` 与逐条运行）
- [ ] **AC12-2** 插件评测报告有 8 条结果，记录插件开关、sidecar/搜索可用性、`research_status`、`research_ms`、`tool_calls`、`lodging_yuan`、`guide_matches`、命令和日期；失败用例记录实际原因，不手填通过。（检查 `docs/eval-report-plugins.md`）
- [ ] **AC12-3** 插件关闭时原 20 条报告仍为 20/20，新指标显示 `skipped/0`；插件开启报告与关闭报告分开存储、可区分。（对比两份报告）
- [ ] **AC12-4** 8 条插件用例的断言基于结构、来源、预算关系和降级状态，不把某家酒店的真实价格或实时库存写死为验收值。（审阅用例与报告）

## 3. 跨切面集成

- [ ] **X-1** 主链顺序为 `parse → retrieve → research（可选）→ draft → schedule → validate/repair → present`；无工具时跳过 research，有工具时 research 在 draft 之前完成。（检查 graph 事件顺序与版本）
- [ ] **X-2** 工具集合来自运行时的已启用注册表，研究节点不包含“先酒店后攻略”或“必须每天回酒店”的硬编码流程。（捕获不同顺序的 fake ToolLLM 调用，均被接受）
- [ ] **X-3** `0002_plugins.sql` 能在已有 v1 数据上执行；旧 `plan_versions` 行得到 `research={}`，旧 `runs.mode` 仍合法，`mode='replan'` 与 `provider='plugin'` 可写入。（在含旧数据的临时库执行迁移）
- [ ] **X-4** `/replan` 返回 202，`base_version` 必须属于当前 trip；非法 city/days/constraints 返回 4xx；同一 `client_request_id` 重复提交返回同一 run，mode 为 `replan`。（API 集成测试）
- [ ] **X-5** `/resume` 的 `resolve_research` 只接受 `awaiting_user` 且存在研究问题的 run；非法状态返回明确 4xx，不接受自由文本酒店。（API 集成测试）
- [ ] **X-6** 缓存命中时不发起外部调用，仍写一条 `tool_calls` 且标记 `cached=true`；不同城市、日期、关键词或站点不会错误命中同一缓存；缓存读写失败视为 miss 并继续。（连续相同/不同查询）
- [ ] **X-7** 错误码可区分 `auth_failed`、`timeout`、`rate_limited`、`bad_response`、`empty_result`、`invalid_input`、`tool_call_limit`；错误消息不含 key/token 和底层网络原文。（逐类触发）
- [ ] **X-8** 工具持久化或证据写入失败只记日志，不影响工具结果和研究收口；全局插件状态不因单次运行失败而关闭。（注入存储失败）
- [ ] **X-9** 3 天行程端到端 p95 ≤60 秒；研究阶段事件 `duration_ms` ≤12000；单次工具调用不突破 10 秒加一次重试的共享 deadline。（固定环境重复运行并统计）
- [ ] **X-10** 单次规划的 token 用量按当时模型单价折算 ≤0.5 元；工具调用次数和缓存命中符合 AC10/N3；无用量记录时本项不得判通过。（从运行用量/账单计算）

## 4. 编译、测试与部署

- [ ] **B-1** `go test -race ./...` 全部通过。
- [ ] **B-2** `go vet ./...` 与 `go build ./...` 通过。
- [ ] **B-3** `sqlc generate` 后生成文件与提交一致，无手改生成文件；迁移与查询无语法错误。
- [ ] **B-4** `go test ./internal/config` 通过；默认关闭、非法 bool/duration、空搜索 key 和默认 12s/6 次限制均有覆盖。
- [ ] **B-5** 前端 `npm test`、`npm run build`、`npm run types` 通过；研究面板、预算开关、追问选项和旧版本兼容测试通过。
- [ ] **B-6** `npm run test:e2e`（Playwright，环境可用时）通过；若环境不可用，记录未执行原因，不记为通过。
- [ ] **B-7** `node --test deploy/hotel-tool/test/server.test.mjs` 通过；healthz、token、参数、超时、非零退出、无坐标/无价格和 stderr 脱敏均有覆盖。
- [ ] **B-8** 默认与 `--profile plugins` 两种 `docker compose config` 均有效；默认不启动 sidecar，不映射宿主机端口；API 镜像未引入 Node。
- [ ] **B-9** 插件关闭时 `go run ./cmd/eval -cases eval/cases` 为 20/20；插件用例 `-dry-run` 能加载 8 条；插件报告能独立生成。
- [ ] **B-10** `git status` 和 `git diff` 中无 `.env`、真实密钥、根目录 `.gitignore` 修改或未预期的生成文件。

## 5. 端到端场景

- [ ] **E2E-1 默认关闭回归**：D0 启动 → 运行一条 v1 行程 → 页面、版本、分享正常 → 无研究卡片和工具事件 → 原 20 条评测 20/20。
- [ ] **E2E-2 指定酒店主路径**：D1 输入「北京三天，住全季酒店王府井店，预算 1500 含住宿」→ 研究调用酒店 → 选中该酒店 → 2 晚住宿计入预算 → 锚点日出现起终点交通 → 页面显示价格估算和来源 → 版本可回放。
- [ ] **E2E-3 酒店缺失与恢复**：D1 输入不存在的指定酒店 → run 等待 → 展示原因和相近酒店 → 选择替代或“继续但不计住宿” → 恢复并生成版本 → 未知价格不计入预算。
- [ ] **E2E-4 攻略加权与降级**：开启攻略 → 输入「参考小红书攻略安排北京两天」→ 候选内命中获得加权和解释 → 不新增 POI → 再制造攻略超时 → 行程仍生成并显示「攻略参考不可用」。
- [ ] **E2E-5 预算口径切换**：已有住宿行程 → 切换到不含住宿 → `/replan` 生成新版本 → 新版本不含住宿费用 → 旧版本仍保留 → 切回含住宿可再次生成。
- [ ] **E2E-6 分享与回放**：打开含研究和无研究的两个分享链接 → 有研究时显示酒店/攻略来源与 AI 标识 → 无研究时保持 v1 展示 → 两者均不泄露内部 ID、工具细节或密钥。

## 6. 审批

请确认本 checklist。确认后四份文档全部通过审批，可以按 `task.md` 开始实现；实现完成后逐项执行本文件并记录证据。
