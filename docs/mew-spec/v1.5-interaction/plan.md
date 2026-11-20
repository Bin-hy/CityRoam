# TripPilot v1.5 交互重构 Plan

状态：已批准（plan）
日期：2026-11-20
依据：[spec.md](./spec.md)（已批准）

## 架构概览

### 分层

```
后端事件层（Go）          前端模型层（纯 TS）              前端 UI 层（React）
─────────────────        ──────────────────────          ─────────────────────
toolEvent 增补     ──►   useSSEEvents（去重/重连）   ──►   ConversationRail
validation.checked        deriveRunView                  ├─ AgentMessage / UserMessage
plan JSON 内嵌            deriveConversation             ├─ ToolCard / DecisionCard / PlanCard
explanations              deriveCanvasModel              └─ Composer
                          （全部纯函数，可单测）              PlanCanvas
                                                            ├─ DayTabs / Map / Timeline
                                                            └─ Inspector / Budget / Version
```

- **事件层只做向后兼容的字段增补**，不新增事件类型、不新增接口、不改数据库结构；`explanations` 通过版本快照的 `plan` JSON 内嵌持久化（现有 `plan` 列是 JSONB，无迁移）。
- **模型层不出现 React**：事件 → 运行视图 / 对话条目 / 画布模型，全部是纯函数；每次事件到达重新推导（上限 200 条事件，成本可忽略），确保刷新与重放结果一致。
- **UI 层按职责拆分**：工作台只做编排（查询、选中、动作），对话组件不感知地图，画布组件不感知 SSE。
- **设计层独立**：颜色、字体、间距、动效统一为 token；组件不再各写 hex。

### 组件划分

| 模块 | 职责 | 对外接口（概念） |
|---|---|---|
| 事件增补（Go） | 工具事件补查询摘要与结果预览；校验事件补检查项数；版本快照持久化解释 | 既有 `toolEvent` 可选字段；`validation.result.checked`；版本 `plan.explanations` |
| `useSSEEvents` | 连接、重放、去重、重连、连接状态 | `(runId) => { events, connected, status }` |
| `deriveRunView` | 状态、阶段、缺失字段、提案、住宿问题、错误、连接中的草稿 | `(events) => RunView` |
| `deriveConversation` | 阶段聚合、工具卡折叠、决策卡开合、用户消息、计划卡、错误 | `(events, ctx) => ConversationItem[]` |
| `deriveCanvasModel` | 当前展示方案、解释、研究、是否草稿、版本号 | `(events, trip) => CanvasModel` |
| `ConversationRail` | 线程渲染、自动滚动、新消息提示、无障碍播报 | `items, composer, onAction` |
| `Composer` | 按模式发送 supplement / revise / adjust / create；运行中显示停止 | `mode, onSubmit, onStop` |
| `PlanCanvas` | 按天切换、地图、时间轴、检查器、工具栏 | `plan, explanations, research, readOnly, selection, actions` |
| `Inspector` | 选中点详情：时间/花费/交通/理由/攻略；默认展示行程概览 | `selection, model, onAdjustDay` |
| 页面层 | 创建页、工作台、分享页、登录、历史、评测 | 复用同一套 token 与画布组件 |

### 架构原则

1. **服务端状态唯一真源**：对话与画布都由事件/版本推导；用户动作只通过既有 API 提交，不在前端改签。
2. **不做第二套协议**：沿用现有 SSE 事件与 `resume / adjustment / replan` 接口；工具卡只是把已有事件变得可读。
3. **演示可解释**：阶段叙述全部来自确定性模板 + 后端已有数据；面试时能逐条说明「这句话由哪个事件生成」。
4. **按需加载不变**：AMap、vis-timeline、diff 继续懒加载；对话模型与 token 是轻量代码。
5. **一次性替换旧交互**：`RunTrace / RunActions / ResearchPanel / VersionPanel` 在新界面落地后删除，不保留双轨。

## 核心数据结构

### 后端（Go）

#### 事件预览接口（`internal/plugins`）

```go
type EventPreview struct {
    Title     string `json:"title"`
    Subtitle  string `json:"subtitle,omitempty"`
    PriceYuan int    `json:"price_yuan,omitempty"`
}

// 由酒店、攻略、天气工具实现；未知工具不实现，事件按旧格式输出。
type EventPreviewer interface {
    QuerySummary(input json.RawMessage) string
    ResultPreview(output json.RawMessage) []EventPreview
}
```

- `ResultPreview` 最多返回 3 条；`Title` ≤ 60 字符、`Subtitle` ≤ 80 字符，超出截断。
- 预览只允许名称、简介、价格等可公开文本；不包含 URL、令牌、原始响应。

#### 工具事件扩展（`internal/plugins/governor.go`）

```go
type toolEvent struct {
    Name        string         `json:"name"`
    Provider    string         `json:"provider"`
    CallIndex   int            `json:"call_index"`
    Status      string         `json:"status"`
    LatencyMS   int64          `json:"latency_ms,omitempty"`
    ResultCount int            `json:"result_count,omitempty"`
    Cached      bool           `json:"cached,omitempty"`
    Coalesced   bool           `json:"coalesced,omitempty"`
    Attempts    int            `json:"attempts,omitempty"`
    Query       string         `json:"query,omitempty"`   // 新增
    Preview     []EventPreview `json:"preview,omitempty"` // 新增
}
```

- `tool_called` 事件带 `Query`；`tool_completed` 事件带 `Preview`（成功、缓存命中、并发合并均适用）。
- 失败时不输出 `Preview`，保留 `Query` 供卡片说明「查了什么」。

#### 校验计数（`internal/domain/validator`）

```go
const BaseRuleCount = 12 // C1–C12

func CheckedCount(includesRain bool) int // includesRain 时 +1（C13）
```

`validation.result` 增加 `"checked": CheckedCount(...)`，其余字段不变。

#### 解释持久化（`internal/runstore/runstore.go`）

```go
// 仅用于落库，不改 domain.Itinerary。
type planSnapshot struct {
    domain.Itinerary
    Explanations []domain.Explanation `json:"explanations,omitempty"`
}
```

- `SaveVersion` 改为序列化 `planSnapshot`，把 `snapshot.Explanations` 内嵌到 `plan` JSON。
- 读取侧继续按 `domain.Itinerary` 反序列化，未知字段被忽略，现有逻辑无需改动。
- 版本接口、分享快照与 `latest_version.plan` 自动带上解释；无需 DDL、无新增字段。

### 前端（TypeScript）

#### 行程与解释（`features/itinerary/types.ts`）

```ts
interface Explanation { poi_id: string; text: string }

interface Itinerary {
  city: string;
  days: DayPlan[];
  lodging?: LodgingSummary;
  totals: Totals;
  explanations: Explanation[]; // 新增，缺省为空数组
}

function parseItinerary(raw: unknown): Itinerary | null // 解析并清洗 explanations
function explanationMap(itinerary: Itinerary | null): Map<string, string> // poi_id → text
```

#### 运行事件与运行视图（`features/conversation/`）

```ts
interface RunEvent {
  seq: number;
  type: string;                    // 保留 string 以兼容未来事件
  payload: Record<string, unknown>;
}

interface RunView {
  status: RunStatus;               // idle/queued/running/awaiting_user/completed/failed/cancelled
  stage: StageKey;                 // understand/research/draft/validate/repair/present/proposal/adjust
  mode: "initial" | "adjustment";
  round: number;
  lastSeq: number;
  missing: string[];
  proposal: AdjustmentProposalView | null;
  researchQuestion: ResearchQuestion | null;
  violations: ViolationView[];
  error: { code: string; message: string } | null;
}
```

#### 对话条目

```ts
type ConversationItem =
  | UserMessage
  | AgentMessage
  | ToolMessage
  | DecisionMessage
  | PlanMessage
  | SystemMessage
  | ErrorMessage;

interface UserMessage {
  kind: "user"; id: string; seq: number;
  text: string;
  variant: "initial" | "supplement" | "revise" | "adjust";
  constraints?: ConstraintChips;
}

interface AgentMessage {
  kind: "agent"; id: string; seq: number;
  stage: "understand" | "research" | "draft" | "validate" | "repair" | "complete";
  tone: "info" | "success" | "warning" | "error";
  text: string;
  details?: string[];            // 展开后的细节
  jumps?: JumpTarget[];          // 可定位到画布的天/点
}

interface ToolMessage {
  kind: "tool"; id: string; seq: number; callKey: string; // `${name}:${callIndex}`
  name: string; provider: string; query: string;
  status: "running" | "ok" | "error";
  latencyMs?: number; resultCount?: number;
  cached?: boolean; coalesced?: boolean; attempts?: number;
  preview: EventPreview[];
  error?: { code: string; message: string };
}

type DecisionPayload =
  | { decision: "missing"; missing: string[] }
  | { decision: "lodging"; question: ResearchQuestion }
  | { decision: "proposal"; proposal: AdjustmentProposalView }
  | { decision: "confirm" };

interface DecisionMessage {
  kind: "decision"; id: string; seq: number;
  payload: DecisionPayload;
  state: "open" | "resolved" | "cancelled";
  summary?: string;              // 折叠后的结果，如「已选择 XX 酒店」
}

interface PlanMessage {
  kind: "plan"; id: string; seq: number;
  versionSeq: number | null;
  stats: { days: number; pois: number; totalYuan: number; walkingKm: number };
  isDraft: boolean;
}

interface SystemMessage {
  kind: "system"; id: string; seq: number;
  text: string;                  // 版本恢复等本地结果，不参与事件重放
}

interface ErrorMessage {
  kind: "error"; id: string; seq: number;
  code: string; message: string; retryable: boolean;
}

interface ConstraintChips {
  city?: string; days?: number; budgetYuan?: number;
  pace?: string; interests?: string[]; mustVisit?: string[]; avoid?: string[];
}

interface JumpTarget { label: string; day: number; blockID?: string; poiID?: string }
interface EventPreview { title: string; subtitle?: string; priceYuan?: number }
```

#### 画布与输入框

```ts
interface CanvasModel {
  plan: Itinerary | null;
  explanations: Map<string, string>;
  research: ResearchBrief | null;
  isDraft: boolean;               // 来自 plan.updated 但尚未完成
  versionSeq: number | null;
  source: "event" | "version";
}

type Selection =
  | { kind: "block"; blockID: string }
  | { kind: "hotel" }
  | null;

type ComposerMode =
  | { kind: "initial" }
  | { kind: "supplement"; missing: string[] }
  | { kind: "revise"; rejected: boolean }
  | { kind: "adjust"; baseVersion: number }
  | { kind: "locked"; reason: "research_choice" }
  | { kind: "running" }
  | { kind: "disabled"; reason: string };
```

#### 会话（跨回合）

```ts
interface TurnEvents { runId: string; events: RunEvent[] }

interface ConversationSession {
  turns: TurnEvents[];            // 本次会话内的多个运行
  activeRunId: string | null;
}
```

- 同一运行内发生 supplement / revise 时，事件继续追加，不新建回合；`run.started` 视为新「尝试」，开启新的阶段槽位。
- 新的 adjustment 运行（新 `run_id`）冻结当前回合并追加一个新回合；刷新后只展示最新运行，历史由版本入口承载（已写入 spec）。
- 版本恢复不产生运行事件：恢复成功后在当前回合末尾追加一条本地 `SystemMessage`；刷新后由画布当前版本与版本菜单体现，与「仅展示最新运行」规则一致。

### 不变量

1. **顺序**：条目按首次出现的 `seq` 排序；后续事件原位更新已有条目（工具卡、阶段消息、决策卡）。
2. **稳定 ID**：`${runId}:${seq}:${slot}`；工具卡用 `${name}:${callIndex}` 折叠，阶段消息用固定槽位名。
3. **无时钟依赖**：推导过程不读取 `Date.now()`；同一事件序列在任何时刻得到同一结果。
4. **单一开放决策**：任意时刻最多一张决策卡为 `open`；后续进展事件将其置为 `resolved` 并生成摘要。
5. **解释覆盖**：同一 `poi_id` 多次解释时后者覆盖前者；缺失时检查器回退到「本次未生成额外说明」。
6. **草稿标识**：`plan.updated` 之后、`run.completed` 之前的方案一律标记为草稿；接受后清除。

## 模块设计

### 后端

#### 1. 事件预览（新增 `internal/plugins/events_preview.go`）

**职责：** 把工具入参/出参压缩成人类可读摘要，供 SSE 事件携带。

- `EventPreview`、`EventPreviewer` 接口、`previewLimit = 3`、`clipRunes`。
- `querySummary(tool, input)` / `resultPreview(tool, output)`：工具未实现接口或解析失败时返回空，不影响调用。
- 预览只取可公开文本；任何解析错误静默降级。

**各工具实现：**

| 工具 | Query 摘要 | Preview（≤3 条） |
|---|---|---|
| `search_hotels` | `北京 · 11/21–11/23 · 2 晚 · ≤¥500/晚 · 4 星以上` | 酒店名 / `4 星 · 8.8 分` / 每晚价格 |
| `search_travel_notes` | 原始搜索词（可带站点） | 标题 / 站点名 |
| `get_weather` | `北京 · 11/21 起 2 天` | 日期 / 天气与温度 |

#### 2. Governor 发布（`internal/plugins/governor.go`）

- 调用开始：计算 `Query`，写入 `tool_called`。
- 成功 / 缓存 / 并发合并：用输出计算 `Preview`，写入 `tool_completed`。
- 失败：只写 `Query`，不写 `Preview`。
- 现有额度、重试、缓存、证据写入顺序不变。

#### 3. 校验计数

- `validator.BaseRuleCount = 12`、`CheckedCount(includesRain bool) int`。
- `Nodes.Validate` 在 `validation.result` 增加 `checked`；下雨检查存在时 +1。

#### 4. 解释持久化（`internal/runstore/runstore.go`）

- 用 `planSnapshot` 包装 `snapshot.Plan` 与 `snapshot.Explanations` 后落库。
- 读取/调整/差异逻辑仍按 `domain.Itinerary` 解析，未知字段自动忽略。
- 影响面：版本详情、行程详情、分享快照自动带解释；无迁移。

#### 5. 兼容性

- 不新增事件类型、不新增接口、不改 OpenAPI（`plan` 为自由对象，SSE 不在契约内）。
- 旧运行无 `query/preview/checked`，前端按缺省渲染。
- 旧版本无内嵌解释，检查器显示「本次未生成额外说明」。

### 前端模型层

#### 文件拆分

| 文件 | 内容 |
|---|---|
| `features/conversation/eventParse.ts` | 安全取值、事件类型守卫、去重排序 |
| `features/conversation/deriveRunView.ts` | 事件 → 运行状态/阶段/缺失/提案/错误 |
| `features/conversation/narrate.ts` | 约束标签、阶段文案模板、跳转目标 |
| `features/conversation/deriveConversation.ts` | 主推导：条目、阶段槽位、工具卡、决策卡 |
| `features/conversation/deriveCanvas.ts` | 事件 + trip → 展示方案/解释/研究/草稿标识 |
| `features/conversation/useSession.ts` | SSE 连接、跨回合会话、动作提交 |
| `features/conversation/model.test.ts` | 固定事件样例的纯函数测试 |

#### 推导算法

按 `seq` 升序遍历事件，维护：

- `attempt`：每次 `run.started` 自增，阶段槽位与工具卡按 attempt 隔离。
- `slots`：阶段消息的固定键（`understand`、`research`、`draft`、`validate`、`repair:{round}`、`plan`、`complete`），已存在则原位更新。
- `tools`：按 `${name}:${callIndex}` 折叠 `tool_called → tool_completed`。
- `openDecision`：任意时刻最多一个；后续进展事件将其置为 `resolved`。

**关键事件映射：**

| 事件 | 产出 |
|---|---|
| `run.started` | 用户消息；`attempt > 1` 且仍为 initial → 显示「补充」增量文本；adjustment → 显示「调整」 |
| `constraint.needs_input` | 打开「补充信息」决策卡 + 理解阶段告警消息 |
| `constraint.parsed` | 关闭补充卡（摘要）+ 更新理解消息与约束标签 |
| `candidate.retrieved` | 更新「挑选与排程」消息（候选数量、尝试次数） |
| `research.started` | 研究阶段消息（工具数量） |
| `plugin.tool_called/completed` | 插入/更新工具卡 |
| `research.completed/degraded` | 更新研究消息（酒店、攻略匹配数、降级原因） |
| `research.needs_input` | 打开「住宿选择」决策卡 |
| `plan.drafted` | 更新排程消息 |
| `validation.result` | 校验消息（`checked` 项通过 / 阻塞问题 + 跳转目标） |
| `repair.started/completed` | 修复消息（轮次、首个阻塞问题、建议） |
| `plan.updated` | 关闭住宿卡；更新方案卡（统计、版本、草稿） |
| `run.awaiting_user` | 按 `missing / research_question / proposal / stage` 打开对应决策卡；`proposal_rejected` 关闭提案卡并进入修改态 |
| `run.completed` | 关闭确认/提案卡为「已接受」；完成消息；方案卡转正式 |
| `run.cancelled` | 关闭卡片为「已取消」；提示消息 |
| `run.failed` | 错误条目 + Agent 错误消息 |

**确定性文案示例：**

- 理解：`我理解你想在北京玩 2 天，预算约 ¥1000，偏好历史、博物馆，节奏轻松。`
- 研究：`找到 5 家住宿候选，优先考虑 XX；匹配到 3 条攻略提及。`
- 降级：`外部资料不完整：住宿搜索超时。我会用已有数据继续。`
- 校验：`12 项约束检查通过。` / `发现 2 个需要处理的问题，已开始自动修复。`
- 修复：`第 1 轮修复：第 2 天总时长超出 35 分钟，建议把国家博物馆挪到上午。`
- 完成：`方案已保存为 v3。`

所有文案只使用事件中已有字段；缺失字段时使用中性句式，不编造。

#### 会话与连接（`useSession.ts`）

- 复用现有 `fetchEventSource` 逻辑，抽成 `useSSEEvents(runId)`：
  - 从 `seq=0` 建立连接，依赖服务端重放；
  - 按 `seq` 去重、只接受递增事件；
  - 维护 `connected`；终态事件后主动断开；
  - 切换 run 时中止旧连接。
- `useSession` 维护 `turns`：
  - 同 run 的 supplement / revise 继续追加事件；
  - 新运行（adjustment / replan）把当前回合冻结进 `turns`，再订阅新 run；
  - 刷新后只有最新运行的回合（历史交给版本菜单）。
- 提供动作封装，统一 pending / error：
  - `supplement(text)` → `resumeRun(action="provide_constraints", input)`；
  - `revise(text)` → `resumeRun(action="revise", input)`；
  - `accept()` / `reject()` → `resumeRun` 对应 action；
  - `cancel()` → `cancelRun(runID)`；
  - `resolveResearch(option)` → `resumeRun(action="resolve_research", research_choice)`；
  - `adjust(text)` → `createAdjustment(tripID, {input, base_version, client_request_id})`；
  - `replanLodging(include)` → `replanTrip(tripID, {base_version, constraints, client_request_id})`。
  - 新运行成功时通过 `onRunStarted(runId)` 切换会话。

### 对话 UI

#### `ConversationRail`

- 顶部：行程标题（城市 · 天数）、状态标签、连接状态点；不再显示 `Trip ID`。
- 线程：`role="log"`、`aria-live="polite"`；距底部 80px 内自动滚动，否则显示「新消息」浮标。
- 用户消息：右对齐暖色气泡，附「补充 / 调整」标签与约束 chips。
- Agent 消息：左对齐，小型指南针头像 + 阶段小标签；正文 15px，最多两段；细节可展开。
- 工具卡：折叠一行（图标、中文工具名、Query、状态、耗时、条数、展开箭头）；展开显示预览与缓存/重试标记；失败用警示色并说明「不影响继续」。
- 决策卡：仅 `open` 态展开；已处理折叠为一行摘要（勾选图标 + 结果）。
- 方案卡：天数 / 景点数 / 步行 / 总价四格统计 + 版本号；移动端「查看方案」按钮。
- 错误：警示表面 + 可重试动作。

#### `Composer`

| 状态 | 行为 |
|---|---|
| running / queued | 显示当前阶段与「停止」按钮，输入禁用 |
| awaiting + lodging | 锁定输入，提示「请在上方选择住宿」；决策卡自动获得视觉焦点 |
| awaiting + missing | 允许文字补充，提交 `provide_constraints` |
| awaiting + proposal / confirm | 允许文字修改，提交 `revise`；决策卡提供接受/拒绝/取消 |
| completed | 允许调整，提交 `createAdjustment` |
| failed / cancelled | 禁用并说明原因 |

- 吸底、发丝分割线、自适应 1–4 行；`Enter` 发送、`Shift+Enter` 换行。
- 发送中断线、限流、409 等错误就地显示，不丢输入。
- 动作进行中禁用重复提交；调整/重建使用现有 `client_request_id` 幂等。

### 工作台布局与画布

#### 断点

| 宽度 | 布局 |
|---|---|
| ≥ 1280 | 对话栏 400px ｜ 画布（地图 + 360px 检查器）｜ 时间轴在画布底部 |
| 1024–1279 | 对话栏 360px ｜ 画布；检查器改为画布内右侧浮层（320px，可关闭） |
| < 1024 | 对话全宽 + 吸底输入；顶部「查看方案」打开全屏画布面板 |

- 全局 `AppHeader` 压缩到 48–56px；工作台不再叠加第二层全宽标题栏。
- 对话栏内部：标题/状态 → 可滚动线程 → 吸底 Composer，三段固定。
- 画布内部：工具栏（天切换 + 预算 + 版本 + 分享 + 草稿标识）→ 地图（自适应高度）→ 时间轴（固定约 200px）。桌面端整页不滚动，画布与线程各自滚动。
- 移动端顶部：返回、行程标题、方案入口、分享超出菜单。

#### `DayTabs`

- 「全部」+ 每天一项：`D1 · 11/21 周六 · ¥320`。
- 选中天时地图聚焦该天、其他天弱化；时间轴只显示该天；「全部」恢复全局。
- 键盘左右切换（roving tabindex），`aria-controls` 指向地图/时间轴区域。

#### 地图（改造 `TripMap`）

- 标记：按天着色 + 顺序编号（景点/餐饮参与编号，交通/休息不编号）；住宿用独立「宿」标记。
- 选中：标记放大并加环形高亮；对应路线加粗；其他天透明度降至约 30%。
- 交互：点标记选中该点并同步时间轴/检查器；点住宿选中住宿；不弹 InfoWindow，详情统一进检查器。
- 聚焦：切换天/选中点时 `fitView` 到该天点位 + 住宿；`ResizeObserver` 驱动 `map.resize()`。
- 降级：无 Key 时展示按天分组的文字清单 + 配置提示，功能不中断。
- 标记内容必须转义；不渲染任何事件文本中的 HTML。

#### 时间轴

- 保留 vis-timeline：按天分组，条目显示时间、类型图标、标题、花费；选中描边；点击双向同步。
- 样式改为 token 驱动：字号 13px、对比度提升、交通为虚线、餐饮暖色、休息紫色。
- 「全部」显示所有天；选中天时缩放该天并隐藏其他组。
- 窄屏不使用横向时间轴，改用纵向 `DayList`（时间、类型、标题、花费），保证可读与可点。

#### 检查器（新增）

- **默认（无选中）**：行程概览——天数、景点数、步行公里、总预算、住宿锚点；若有研究提醒则显示一句「本日提示」。
- **选中景点/餐饮**：
  - 头部：天序号、时间区间、标题、类型。
  - 元信息：时长、花费、是否估算。
  - 交通衔接：下一段方式、分钟、费用、是否直线估算。
  - **为什么选它**：来自 `explanations`；缺失时显示「本次未生成额外说明」。
  - **攻略提及**：该 POI 的攻略 note + 最多 3 条来源（站点/日期），外链新窗口。
  - 操作：「调整这一天」→ 聚焦 Composer 并预填 `调整第 N 天：`；上一个/下一个点。
- **选中住宿**：名称、星级/评分、地址、每晚/合计、锚点天、预订链接、预算口径开关（只读页隐藏）。
- 只读模式（分享页）：隐藏调整与口径开关，保留信息与外链。

#### 工具栏

- **预算**：触发器显示总计；弹出拆分（门票/餐饮/交通/住宿）、总计、估算提示、是否含住宿开关。切换沿用 `replan` 并产生新回合；只读页不显示开关。
- **版本**：触发器 `v3 ▾`；列表显示版本号、时间、变更原因、状态；选择历史版本后画布进入「查看 v2」状态并显示差异摘要（新增/移动/删除数量与对应点位）；提供「恢复此版本」（带确认）与「返回当前」。
- **分享**：沿用现有弹窗与接口，仅重做样式；只读页不显示。
- **草稿标识**：`plan.updated` 后未完成时显示「草稿预览 · 待确认」；接受后消失。

#### 移动端 `CanvasSheet`

- 使用现有 Dialog 基础做全屏面板（无新抽屉依赖），顶部：标题、关闭、天切换；底部安全区留白。
- 标签页：地图 / 时间轴 / 详情；点地图标记显示底部预览卡，点「查看详情」切到详情页签。
- 预算、版本、分享收进面板右上「更多」；所有面板可独立滚动，不横向溢出。

### 页面

#### 创建页

- 桌面双栏：左侧编辑感标题与说明，右侧输入区；窄屏纵向堆叠。
- 输入区：大文本框、示例标签、偏好标签（天数/预算/节奏/兴趣/必去/避开/用餐），点标签打开「偏好」弹窗，不常驻大表单。
- 提交后进入工作台；失败就地提示并保留输入。
- 历史列表（登录时）：每行城市·天数、日期、状态、版本；空态给出引导。
- 游客模式不显示历史列表，行为不变。

#### 分享页

- 只读叙事：标题区（城市·天数、总预算、AI 标识、复制文本、我也来规划）→ 画布（天切换/地图/时间轴/检查器）→ 「旅途中值得知道」（住宿 + 攻略提及与来源 + tips）→ 预算摘要。
- 复用画布组件并传 `readOnly`；不显示版本菜单、分享按钮、运行轨迹。
- 解释与攻略来源随快照展示；旧快照缺失时给出降级说明。

#### 登录 / 历史 / 评测

- 登录页：品牌 + 单一登录按钮 + 错误提示，保留 `return_to`。
- 历史列表：编辑感行式布局，状态用「点 + 文字」，不只用颜色。
- 评测面板：只替换颜色、字体、间距 token，不改结构。

### 设计系统

#### Token（`design/tokens.ts` + CSS 变量）

- 颜色：纸色 `paper`、墨色 `ink`、森林绿 `forest`、陶土 `clay`、沙色 `sand`、发丝线 `hairline`、成功/警示/危险；四天主题色与地图标记共用同一份值。
- 字体：标题衬线栈（Songti SC / Noto Serif SC / Georgia），正文无衬线栈（PingFang SC / 系统字体），数字 `tabular-nums`。
- 圆角 6–10px；阴影只用于浮层与对话框；其余用发丝线与留白分层。
- 间距沿用 4 的倍数；内容最大宽度 1200px；动效 120–180ms，尊重 `prefers-reduced-motion`。

#### 基础组件

`Surface`、`Hairline`、`Chip`、`Stat`、`SectionLabel`、`StatusDot`、`EmptyHint`、`Sheet`（全屏 Dialog 变体）、`Popover`（预算/版本，Radix 基础组件）。

- 业务组件不再直接写六位色值；地图颜色从 token 模块读取。
- 卡片只用于工具卡、决策卡、浮层、移动端预览；其余信息块使用 `Surface + Hairline`。

#### 状态表达

- **加载**：骨架与最终布局同尺寸；对话首条显示「正在连接规划服务…」。
- **空**：无方案时画布显示引导文案，不空白；检查器显示概览占位。
- **错误**：就地警示表面 + 重试；不打断已生成方案。
- **降级**：警示色 + 明确原因（工具失败、研究不完整、地图不可用、旧数据）。
- **断线**：输入框上方细条「连接中断，正在重连…」，恢复后自动消失；不弹窗。
- **运行中**：阶段消息持续推进；Composer 显示阶段与停止，不使用全屏 spinner。

### 可访问性与性能落点

- 地标：对话 `aside`、画布 `main`；跳转链接；天切换 `tablist`；检查器 `region` 带可读标题。
- 新消息进入隐藏 `aria-live=polite` 区域；运行中容器 `aria-busy`。
- 地图不可键盘操作，因此时间轴 / `DayList` 必须提供等价的选点路径。
- 工具卡是带 `aria-expanded` 的按钮；决策卡操作有明确禁用态与进行中文案。
- 对话只在线程事件变化时重算；选中点状态只影响画布，不触发对话重渲染。
- 地图覆盖物仅在天/选中/方案变化时重建；窗口尺寸变化做防抖；弹层与画布按需加载。

## 模块交互

### 1. 首次规划

```
创建页提交 → POST /api/trips → 跳转工作台（runId）
  → SSE 重放/实时事件 → deriveConversation / deriveCanvas
  → plan.updated：画布显示草稿 + 方案卡
  → run.awaiting_user：确认卡展开
  → 接受 → run.completed → 拉取 trip（正式版本）→ 草稿标识消失
```

### 2. 补充信息（同一 run）

```
constraint.needs_input → 补充卡 + Composer 模式 supplement
  → provide_constraints → 同 run 重新执行 → run.started（attempt+1）
  → 输入框以增量文本显示「补充」→ constraint.parsed 关闭卡片
```

### 3. 住宿选择（同一 run）

```
research.needs_input → 住宿卡（Composer 锁定）
  → resolve_research → 后续事件到达 → 卡片折叠为摘要
  → plan.updated：地图出现住宿锚点、预算含住宿
```

### 4. 提案确认 / 拒绝修改（同一 run）

```
run.awaiting_user(proposal) → 提案卡
  ├─ 接受 → run.completed → 新版本
  ├─ 拒绝 → run.awaiting_user(proposal_rejected) → Composer 进入 revise
  └─ Composer 直接输入 → revise → run.started(adjustment) → 新提案
```

### 5. 完成后调整（新 run）

```
Composer(adjust) → POST adjustments → 切换订阅新 run
  → 旧回合冻结进会话历史 → 新回合：用户消息 → 提案卡 → 接受/拒绝
```

### 6. 版本查看与恢复

```
版本菜单 → 选择 vN → 拉取版本详情 → 画布显示该版本 + 差异摘要
  → 恢复 → confirm → restore API → 新 accepted 版本（reason=恢复到 vN）
  → 重新拉取 trip/versions → 画布回到当前版本
  → 当前回合末尾追加本地系统消息「已恢复到 vN」
```

## 文件组织

```
internal/
├── plugins/
│   ├── events_preview.go         新增：Preview 结构、接口、截断与降级
│   ├── events_preview_test.go    新增
│   ├── hotel.go                  修改：QuerySummary / ResultPreview
│   ├── guide.go                  修改：同上
│   ├── weather.go                修改：同上
│   ├── governor.go               修改：toolEvent 增加 query / preview
│   └── governor_test.go          修改：预览字段断言
├── domain/validator/
│   ├── validator.go              修改：BaseRuleCount、CheckedCount
│   └── validator_test.go         修改：计数断言
├── agent/
│   └── schedule.go               修改：validation.result 增加 checked
├── api/
│   └── api_integration_test.go   修改：版本 plan 内嵌解释断言
└── runstore/
    └── runstore.go               修改：planSnapshot 内嵌 explanations

web/
├── package.json                  修改：新增 @radix-ui/react-popover
└── e2e/golden.spec.ts            修改：新交互与工具卡场景

web/src/
├── design/
│   ├── tokens.ts                 新增：JS 侧颜色/字体/日期/天主题
│   └── primitives.tsx            新增：Surface/Hairline/Chip/Stat/StatusDot/EmptyHint
├── components/
│   ├── ui/popover.tsx            新增：Radix Popover 封装
│   ├── ui/sheet.tsx              新增：全屏 Dialog 变体
│   ├── AppHeader.tsx             修改：编辑感顶栏
│   └── StateMessage.tsx          修改：统一空/错/加载样式
├── features/conversation/
│   ├── eventParse.ts             新增：安全取值与事件守卫
│   ├── deriveRunView.ts          新增
│   ├── narrate.ts                新增：约束标签与文案模板
│   ├── deriveConversation.ts     新增
│   ├── deriveCanvas.ts           新增
│   ├── useSSEEvents.ts           新增：连接/重放/去重
│   ├── useSession.ts             新增：跨回合会话与动作
│   ├── ConversationRail.tsx      新增
│   ├── AgentMessage.tsx          新增
│   ├── UserMessage.tsx           新增
│   ├── ToolCard.tsx              新增
│   ├── DecisionCard.tsx          新增（missing/lodging/proposal/confirm）
│   ├── PlanCard.tsx              新增
│   ├── Composer.tsx              新增
│   ├── model.test.ts             新增：纯函数推导测试
│   └── Composer.test.tsx         新增：模式与提交测试
├── features/canvas/
│   ├── PlanCanvas.tsx            新增
│   ├── CanvasToolbar.tsx         新增
│   ├── DayTabs.tsx               新增
│   ├── Inspector.tsx             新增
│   ├── CanvasSheet.tsx           新增：移动端全屏画布
│   ├── DayList.tsx               新增：窄屏纵向时间清单
│   ├── PlanCanvas.test.tsx       新增
│   └── Inspector.test.tsx        新增
├── features/workspace/
│   ├── WorkspacePage.tsx         重写：布局与会话编排
│   └── WorkspacePage.test.tsx    修改
├── features/map/
│   ├── TripMap.tsx               改造：编号标记、住宿、天过滤、选中高亮
│   └── TripMap.test.tsx          修改
├── features/timeline/
│   ├── ItineraryTimeline.tsx     改造：天过滤、选中、样式
│   ├── timelineModel.ts          修改：编号与选中元数据
│   └── ItineraryTimeline.test.tsx 修改
├── features/versions/
│   ├── diffModel.ts              保留：版本/检查器差异
│   ├── diffModel.test.ts         保留
│   ├── VersionMenu.tsx           新增（替代 VersionPanel/VersionSwitcher）
│   └── VersionMenu.test.tsx      新增
├── features/budget/
│   ├── budgetModel.ts            新增：从 BudgetPanel 抽出纯函数
│   ├── budgetModel.test.ts       新增
│   ├── BudgetPopover.tsx         新增（替代 BudgetPanel）
│   └── BudgetPopover.test.tsx    新增
├── features/research/
│   ├── types.ts                  保留
│   ├── types.test.ts             保留
│   └── ResearchNotes.tsx         新增：分享页「值得知道」
├── features/create/
│   ├── CreatePage.tsx            重写
│   ├── CreateForm.tsx            重写：单输入 + 偏好标签
│   ├── PreferenceSheet.tsx       新增
│   └── composeInput.ts           保留
├── features/share/
│   ├── SharePage.tsx             重写：只读画布叙事
│   └── ShareDialog.tsx           样式重做
├── features/trips/MyTripList.tsx 修改
├── features/auth/LoginPage.tsx   修改
├── features/eval/EvalDashboard.tsx 仅 token
├── features/itinerary/
│   ├── types.ts                  修改：解析 explanations
│   └── types.test.ts             修改
├── index.css                     修改：token 与全局排版
└── hooks/useRunStream.ts         删除（由 useSSEEvents 取代）
```

**删除**（新界面落地后）：`features/trace/*`、`features/workspace/RunActions.tsx` 及其测试、`features/research/ResearchPanel.tsx`、`features/research/BudgetModeToggle.tsx` 及其测试、`features/budget/BudgetPanel.tsx` 及其测试、`features/versions/VersionPanel.tsx`、`VersionSwitcher.tsx`、`PlanDiff.tsx` 及其测试。

## 需求覆盖检查

| 需求 | 归属模块 |
|---|---|
| F1 创建与进入对话 | 创建页、`useSession`、`ConversationRail`、历史列表 |
| F2 对话流与重建 | `useSSEEvents`、`deriveRunView`、`deriveConversation`、`narrate` |
| F3 插件 / 工具卡 | `events_preview.go`、`governor.go`、`ToolCard` |
| F4 追问与确认 | `deriveConversation`、`DecisionCard`、`Composer`、`useSession` 动作 |
| F5 方案画布 | `PlanCanvas`、`DayTabs`、`TripMap`、`ItineraryTimeline`、`Inspector` |
| F6 预算、版本与分享 | `BudgetPopover`、`VersionMenu`、`ShareDialog`、`restorePlanVersion` |
| F7 移动端 | 断点布局、`CanvasSheet`、`DayList`、吸底 Composer |
| F8 分享页 | `SharePage`、`PlanCanvas(readOnly)`、`ResearchNotes` |
| F9 全站视觉与状态 | `design/tokens.ts`、`primitives.tsx`、`AppHeader`、各页面、`StateMessage` |
| F10 事件增补 | `EventPreviewer`、`toolEvent.Query/Preview`、`validation.result.checked` |
| N1 确定性与可重放 | 纯函数推导、`seq` 去重、服务端动作为准 |
| N2 性能预算 | memo 推导、选中与画布分离、懒加载、Radix 小依赖 |
| N3 可访问性 | 地标、`aria-live`、键盘天切换/选点、焦点样式 |
| N4 响应式 | 四档断点、`CanvasSheet`、安全区、纵向滚动 |
| N5 兼容与降级 | 旧事件缺省、无 Key 文字清单、旧快照解释缺失文案 |
| N6 成本与架构 | 无新 LLM、无 DDL、无新服务、预览 ≤3 条 |
| N7 安全隐私 | 预览白名单、纯文本、http/https + `noopener` |
| N8 可测试性 | `model.test.ts`、组件测试、mock SSE e2e |
| N9 设计一致性 | token 单一来源、共享组件与措辞 |
| N10 内容语言 | 中文文案、人民币、统一日期格式、隐藏内部术语 |

## 技术决策

| 决策点 | 选择 | 理由 |
|---|---|---|
| 对话 UI | 自研轻量组件，不引入 assistant-ui | 架构文档曾计划 assistant-ui，但实际未安装；本次需要精确控制编辑感视觉与工具卡形态，自研成本可控 |
| 对话推导 | 每次事件对全量事件重推导（memoized） | 单 run ≤200 事件，纯函数更易测试与重放；避免增量 reducer 的状态漂移 |
| 解释持久化 | 版本落库时内嵌到 `plan` JSON | 无 DDL；版本、行程、分享同时受益；读取侧忽略未知字段即可兼容 |
| 工具卡数据 | 工具实现可选 `EventPreviewer`，Governor 统一发布 | 不引入完整工具协议；新增工具按需实现，未实现自动降级 |
| 校验计数 | validator 暴露 `CheckedCount` | 避免在 agent 硬编码 12；C13 存在时自然 +1 |
| 地图/时间轴 | 保留 AMap + vis-timeline，改造而非替换 | 已集成且稳定；替换收益低于成本；窄屏另用 DayList |
| 预算/版本菜单 | 新增 Radix Popover 基础组件 | 无障碍与定位行为完整，体积小（约十余 KB），符合 50KB 预算 |
| 移动端画布 | 全屏 Dialog 变体 | 复用现有 Dialog 的焦点管理与 Escape，不引入 drawer 依赖 |
| 会话历史 | 仅内存保留本次会话回合 | 不新增 run 列表接口；刷新后由版本菜单承载历史，已写入 spec |
| 版本恢复消息 | 本地系统消息 + 版本数据兜底 | restore 不产生运行事件；当前会话可见结果，刷新后由当前版本状态体现 |
| 设计 token | CSS 变量 + JS token 文件双份同源 | CSS 控制样式，地图/图表需要具体色值；禁止组件散落 hex |
| 旧组件 | 一次性替换并删除 | 避免双轨维护；测试同步迁移 |
| 实施顺序 | 后端事件增补 → 模型 → 对话 → 画布 → 全站换皮 | 每层可独立验证；后端先行使界面有真实数据可展示 |

## 测试策略

### 后端

- `events_preview_test.go`：三类工具的 Query/Preview 输出、截断、超长/空结果、未知工具降级。
- `governor_test.go`：成功、缓存命中、并发合并、失败、重试各路径都带正确字段。
- `validator_test.go` / agent 测试：`checked` 在有/无天气时分别为 13/12。
- API/集成测试：完成一次运行后，`latest_version.plan.explanations` 非空；调整运行仍可读取旧版本。
- 回归：`go test ./... -race`、`go vet`、`go build`。

### 前端

- 模型单测（固定事件样例）：首次规划、缺信息、住宿选择、提案接受/拒绝/修改、降级、旧事件无增补字段、补充增量文本、工具卡折叠、单开放决策、解释覆盖。
- 组件测试：Composer 六种模式、ToolCard 展开/失败、DecisionCard 状态、Inspector 有/无解释、DayTabs 键盘、BudgetPopover、VersionMenu。
- 类型与构建：`npm run types`、`tsc -b`、`vite build`。
- Playwright（mock SSE/API）：创建 → 用户消息 → 工具卡 → 确认 → 画布选点 → 检查器理由 → 调整提案 → 接受 → 分享页；移动端 390px 打开方案面板。
- 手工验收：真实 AMap 键下的编号/住宿/路线；键盘全流程；屏幕阅读器播报；四档断点；无 Key/插件关闭/旧数据降级。

## 风险与对策

| 风险 | 对策 |
|---|---|
| 旧版本没有内嵌解释 | 检查器与分享页显示明确降级文案，不报错 |
| supplement 的 `run.started` 携带合并后输入 | 与上一次输入做前缀差量，显示「补充」文本；单测覆盖 |
| SSE 重放与实时事件重叠 | 按 `seq` 去重、只接受递增；连接切换中止旧流 |
| 地图标记 HTML 注入 | 只拼接转义后的标题/编号，不渲染事件文本 |
| vis-timeline 在切换选中时重建 | 选中只更新 selection；仅方案/天变化时 setData |
| 移动端全屏面板滚动/焦点 | 统一 Sheet 组件、安全区 padding、Escape 关闭；390px e2e |
| Popover 依赖安装受阻 | 退回 Dialog 呈现预算/版本，功能不受影响 |
| 范围较大（全站） | 按 P0（工作台 + 事件）与 P1（创建/分享/登录换皮）顺序推进；每阶段可独立验收 |
| 刷新后丢失早前回合 | 已在 spec 明确由版本菜单承载；会话内不丢 |
| 本地版本恢复消息不参与重放 | 明确标注为当前会话提示；刷新后以当前版本状态为准，不伪造运行事件 |

## 工作量估算

| 阶段 | 估算 |
|---|---|
| 后端事件增补与持久化 | 4h |
| Token / 基础组件 | 3h |
| 对话模型与测试 | 5h |
| 对话 UI + Composer + 动作 | 8h |
| 画布（地图/时间轴/检查器/工具栏/移动端） | 10h |
| 创建/分享/登录/历史换皮 | 5h |
| 测试、可访问性、清理旧组件 | 5h |
| **合计** | **约 40h** |

按每周 8–10 小时约 4 周；建议 P0 先交付工作台，P1 完成全站换皮。
