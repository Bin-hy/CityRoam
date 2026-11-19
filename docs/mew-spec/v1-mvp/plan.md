# TripPilot v1 MVP Plan

> 状态：已批准（2026-11-19）

| 项目 | 内容 |
|---|---|
| 版本 | v0.1 |
| 状态 | 待审批 |
| 关联 | [spec.md](spec.md) · [项目立项](../../prd/项目立项.md) · [系统架构设计](../../architecture/系统架构设计.md) |
| 说明 | 本文回答「怎么做」；接口、DDL、任务拆分分别见本文第 7、8 节与后续 `task.md` |

## 1. 架构概览

**模块化单体**，一个 Go 二进制、一个 React SPA、一个 PostgreSQL。

```
Browser（React + Vite）
   │ HTTPS + SSE
Caddy
   ├─ /          → 静态资源
   └─ /api/*     → Go API（chi）
                    ├─ api/        HTTP、中间件、SSE
                    ├─ agent/      Runner + Eino Graph + Prompt
                    ├─ domain/     约束、调度、校验、diff
                    ├─ tools/      高德、快照、直线距离
                    ├─ mcp/        McpHub
                    ├─ repo/       pgx + sqlc
                    └─ events/     Broker + run_events
                          │
                    PostgreSQL 16
                          │
                    LLM / 高德 / MCP
```

**四条硬边界**

1. **编排确定性**：阶段、停止条件、校验、冲突处理由代码控制；LLM 只负责选点、排序、修复提案和解释。
2. **闭集选择**：LLM 只能输出候选集中的 POI ID；非法 ID 直接判失败。
3. **领域纯函数**：校验、调度、预算、diff 不依赖数据库和网络，可单独测试。
4. **单写者**：一个 run 同一时刻只有一个执行协程；状态写 `runs.state`，事件写 `run_events`。

## 2. 组件划分

| 组件 | 职责 | 不负责 |
|---|---|---|
| `api` | 路由、参数校验、限流、SSE、错误映射 | 业务规则、SQL |
| `agent` | Eino Graph、Run 生命周期、Prompt、修复合并 | 外部 API 细节、SQL |
| `domain` | 约束、调度、C1–C12、预算、diff | IO、数据库、HTTP |
| `tools` | 高德、POI 快照、直线距离、天气 | 行程规则 |
| `mcp` | 连接、发现、白名单、超时、熔断、归一化 | 选点决策 |
| `repo` | trips、runs、versions、events、share、eval | 业务判断 |
| `events` | 进程内发布订阅、SSE 格式化、回放 | UI |
| `eval` | 20 条用例、跑批、指标 | 生产请求路径 |
| `web` | 页面、SSE 消费、地图、时间轴、diff | 校验权威 |

## 3. 核心数据结构

### 3.1 ConstraintSet

```go
type ConstraintSet struct {
    City      string            `json:"city"`
    Days      int               `json:"days"`
    StartDate string            `json:"start_date,omitempty"`
    PartySize int               `json:"party_size"`
    Budget    BudgetConstraint  `json:"budget"`
    Daily     DailyConstraint   `json:"daily"`
    Meals     MealConstraint    `json:"meals"`
    Pace      Pace              `json:"pace"`
    Interests []string          `json:"interests"`
    MustVisit []string          `json:"must_visit"`
    Avoid     []string          `json:"avoid"`
    Walking   WalkingConstraint `json:"walking"`
    Weather   WeatherConstraint `json:"weather"`
    Hard      []string          `json:"hard"`
    Soft      []string          `json:"soft"`
    Missing   []string          `json:"missing,omitempty"`
}

type BudgetConstraint struct {
    TotalYuan int  `json:"total_yuan"`
    Hard      bool `json:"hard"`
}

type DailyConstraint struct {
    MaxHours  int    `json:"max_hours"`
    StartTime string `json:"start_time"`
    EndTime   string `json:"end_time"`
}

type MealConstraint struct {
    Enabled           bool      `json:"enabled"`
    LunchWindow       [2]string `json:"lunch_window"`
    DinnerWindow      [2]string `json:"dinner_window"`
    BudgetPerMealYuan int       `json:"budget_per_meal_yuan"`
}

type WalkingConstraint struct {
    MaxKMPerDay float64 `json:"max_km_per_day"`
}

type WeatherConstraint struct {
    RainAware bool `json:"rain_aware"`
}

type Pace string

const (
    PaceRelaxed Pace = "relaxed"
    PaceNormal  Pace = "normal"
    PacePacked  Pace = "packed"
)
```

`Missing` 只由 Parse 写入；`City` 或 `Days` 缺失时不得进入 Retrieve。

### 3.2 POI

```go
type POI struct {
    ID               string      `json:"id"`
    City             string      `json:"city"`
    Name             string      `json:"name"`
    Category         string      `json:"category"`
    Area             string      `json:"area"`
    Address          string      `json:"address"`
    Lat              float64     `json:"lat"`
    Lng              float64     `json:"lng"`
    CoordSystem      string      `json:"coord_system"` // 固定 GCJ-02
    OpenHours        []OpenRange `json:"open_hours"`
    TicketYuan       *int        `json:"ticket_yuan,omitempty"`
    SuggestedMinutes int         `json:"suggested_minutes"`
    Rating           *float64    `json:"rating,omitempty"`
    Outdoor          bool        `json:"outdoor"`
    Tags             []string    `json:"tags"`
    Source           string      `json:"source"`
    VerifiedAt       *time.Time  `json:"verified_at,omitempty"`
}

type OpenRange struct {
    Days  []int  `json:"days"`  // 0=周日，1=周一
    Start string `json:"start"` // "09:00"
    End   string `json:"end"`   // "17:00"
}
```

### 3.3 DraftPlan（LLM 唯一输出）

```go
type DraftPlan struct {
    Days         []DraftDay    `json:"days"`
    Explanations []Explanation `json:"explanations"`
}

type DraftDay struct {
    Date  string      `json:"date"`
    Items []DraftItem `json:"items"`
}

type DraftItem struct {
    POIID       string `json:"poi_id"`
    Type        string `json:"type"` // poi | meal
    DurationMin int    `json:"duration_min"`
    StartHint   string `json:"start_hint"`
}

type Explanation struct {
    POIID string `json:"poi_id"`
    Text  string `json:"text"`
}
```

LLM 不输出交通、精确时间、费用；这些由 `Scheduler` 计算。

### 3.4 Itinerary

```go
type Itinerary struct {
    City   string    `json:"city"`
    Days   []DayPlan `json:"days"`
    Totals Totals    `json:"totals"`
}

type DayPlan struct {
    Date        string  `json:"date"`
    StartTime   string  `json:"start_time"`
    EndTime     string  `json:"end_time"`
    Blocks      []Block `json:"blocks"`
    DayCostYuan int     `json:"day_cost_yuan"`
}

type Block struct {
    ID          string     `json:"id"`
    Type        string     `json:"type"` // poi | meal | transit | rest
    POIID       string     `json:"poi_id,omitempty"`
    Title       string     `json:"title"`
    Start       string     `json:"start"`
    End         string     `json:"end"`
    DurationMin int        `json:"duration_min"`
    CostYuan    int        `json:"cost_yuan"`
    Transport   *Transport `json:"transport,omitempty"`
    Notes       string     `json:"notes,omitempty"`
}

type Transport struct {
    Mode        string `json:"mode"` // walk | transit | drive
    FromPOIID   string `json:"from_poi_id"`
    ToPOIID     string `json:"to_poi_id"`
    DistanceM   int    `json:"distance_m"`
    DurationMin int    `json:"duration_min"`
    CostYuan    int    `json:"cost_yuan"`
    Estimated   bool   `json:"estimated"`
}

type Totals struct {
    CostYuan   int     `json:"cost_yuan"`
    POICount   int     `json:"poi_count"`
    WalkingKM  float64 `json:"walking_km"`
}
```

### 3.5 Violation

```go
type Level string

const (
    LevelBlocking Level = "blocking"
    LevelWarning  Level = "warning"
)

type Violation struct {
    Code       string `json:"code"` // C1..C12
    Level      Level  `json:"level"`
    Day        int    `json:"day"` // 1-based
    BlockID    string `json:"block_id,omitempty"`
    POIID      string `json:"poi_id,omitempty"`
    Message    string `json:"message"`
    Suggestion string `json:"suggestion,omitempty"`
}
```

### 3.6 Run 与事件

```go
type RunStatus string

const (
    RunQueued       RunStatus = "queued"
    RunRunning      RunStatus = "running"
    RunAwaitingUser RunStatus = "awaiting_user"
    RunCompleted    RunStatus = "completed"
    RunFailed       RunStatus = "failed"
    RunCancelled    RunStatus = "cancelled"
)

type Run struct {
    ID            string
    TripID        string
    Mode          string // initial | adjustment | eval
    Status        RunStatus
    Stage         string
    Round         int
    Input         string
    Constraints   ConstraintSet
    BaseVersionID string
    State         json.RawMessage
    Proposal      *AdjustmentProposal
    ErrorCode     string
    ErrorMessage  string
    RequestID     string
}

type RunEvent struct {
    Seq       int64           `json:"seq"`
    Type      string          `json:"type"`
    Payload   json.RawMessage `json:"payload"`
    CreatedAt time.Time       `json:"created_at"`
}
```

### 3.7 调整提案

```go
type AdjustmentProposal struct {
    BaseVersion int        `json:"base_version"`
    Scope       string     `json:"scope"` // day | block | poi | pace | budget
    DayIndex    int        `json:"day_index,omitempty"`
    Patch       DayPatch   `json:"patch"`
    Summary     string     `json:"summary"`
    Impact      string     `json:"impact"`
    Warnings    []string   `json:"warnings,omitempty"`
}

type DayPatch struct {
    DayIndex int         `json:"day_index"`
    Items    []DraftItem `json:"items"`
    Reason   string      `json:"reason"`
}
```

### 3.8 TripState

```go
type TripState struct {
    TripID         string
    RunID          string
    Mode           string
    Input          string
    Constraints    ConstraintSet
    Missing        []string
    Candidates     []POI
    Draft          DraftPlan
    Plan           Itinerary
    Violations     []Violation
    Round          int
    Best           *Itinerary
    BestViolations []Violation
    Proposal       *AdjustmentProposal
    Error          *RunError
}

type RunError struct {
    Code    string
    Message string
    Cause   error
}
```

## 4. 核心接口

```go
type LLMClient interface {
    GenerateJSON(ctx context.Context, prompt string, schema json.RawMessage) (json.RawMessage, Usage, error)
}

type Tool interface {
    Name() string
    InputSchema() json.RawMessage
    Call(ctx context.Context, input json.RawMessage) (json.RawMessage, error)
}

type RouteProvider interface {
    Route(ctx context.Context, from, to POI, mode string) (Transport, error)
}

type POIRepository interface {
    Search(ctx context.Context, q POIQuery) ([]POI, error)
    Get(ctx context.Context, id string) (POI, error)
}

type Validator interface {
    Validate(plan Itinerary, c ConstraintSet, pois map[string]POI, routes RouteProvider) []Violation
}

type Scheduler interface {
    Build(draft DraftPlan, c ConstraintSet, pois map[string]POI, routes RouteProvider) (Itinerary, error)
}

type EventPublisher interface {
    Publish(ctx context.Context, runID string, typ string, payload any) error
}

type Store interface {
    CreateTripAndRun(ctx context.Context, trip Trip, run Run) error
    SaveRunState(ctx context.Context, runID string, state json.RawMessage) error
    SaveVersion(ctx context.Context, v PlanVersion) error
    AppendEvent(ctx context.Context, runID string, typ string, payload any) (RunEvent, error)
    ListEventsAfter(ctx context.Context, runID string, seq int64) ([]RunEvent, error)
    GetRun(ctx context.Context, runID string) (Run, error)
    UpdateRunStatus(ctx context.Context, runID string, status RunStatus, stage string) error
}
```

接口只定义 Agent 需要的最小能力；SQL 细节不泄漏到领域层。

## 5. 模块设计

### 5.1 `api`

**职责**：HTTP 入口、参数校验、限流、SSE、错误映射。

**对外接口**

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/trips` | 创建行程 + 启动 run |
| GET | `/api/trips/{id}` | 行程摘要 |
| DELETE | `/api/trips/{id}` | 软删除 |
| POST | `/api/trips/{id}/adjustments` | 提交调整 |
| POST | `/api/trips/{id}/share` | 创建分享链接 |
| GET | `/api/trips/{id}/versions` | 版本列表 |
| GET | `/api/trips/{id}/versions/{seq}` | 版本详情 |
| GET | `/api/trips/{id}/diff` | 两版差异 |
| GET | `/api/runs/{id}` | 运行状态 |
| GET | `/api/runs/{id}/events` | SSE |
| GET | `/api/runs/{id}/trace` | 完整 trace |
| POST | `/api/runs/{id}/resume` | 补充约束 / 接受 / 拒绝 |
| POST | `/api/runs/{id}/cancel` | 取消运行 |
| GET | `/api/share/{slug}` | 只读分享 |
| GET | `/api/meta` | 城市、默认值、版本 |
| GET | `/api/eval/cases` | 用例 |
| POST | `/api/eval/runs` | 跑评测 |
| GET | `/api/eval/runs` | 评测结果 |
| GET | `/healthz` | 存活 |
| GET | `/readyz` | 数据库就绪 |

**依赖**：`agent.Runner`、`repo.Store`、`events.Broker`、配置。

**中间件**：request ID、recover、请求体大小限制、IP 限流 10 次/小时、Admin token、超时。

### 5.2 `agent`

**职责**：Eino Graph、Run 生命周期、Prompt、修复合并、状态恢复。

**内部结构**

- `Runner`：启动/恢复/取消 run；持有并发信号量（默认 3）；每个 run 一个 goroutine。
- `Graph`：Eino 图，节点 `admit → parse → retrieve → draft → schedule → validate → repair → present`。
- `Nodes`：每个节点是 `compose.InvokableLambda`；LLM 节点通过 `LLMClient` 调用。
- `Repair`：把 `Violation` 和替代候选转成 Prompt，解析 `DayPatch`，合并后重新调度。
- `Prompts`：`parse.tmpl`、`draft.tmpl`、`repair.tmpl`、`adjust.tmpl`、`explain.tmpl`，用 `embed` 打包。
- `State`：每个节点结束写 `runs.state`；`awaiting_user` 后从 DB 恢复。

**对外接口**

```go
type Runner interface {
    Start(ctx context.Context, runID string) error
    Resume(ctx context.Context, runID string, action ResumeAction) error
    Cancel(ctx context.Context, runID string) error
    Recover(ctx context.Context) error
}
```

**依赖**：`LLMClient`、`ToolRegistry`、`Store`、`EventPublisher`、`Validator`、`Scheduler`。

### 5.3 `domain`

**职责**：所有确定性规则，纯函数。

**子模块**

| 子模块 | 职责 |
|---|---|
| `constraint` | 默认值、合并、缺失字段检测 |
| `scheduler` | 计算到访时间、交通块、等待、费用 |
| `validator` | C1–C12 |
| `diff` | 版本 JSON 差异 |
| `budget` | 门票、餐饮、交通汇总 |

**依赖**：仅标准库和领域类型。

### 5.4 `tools`

**职责**：外部数据访问，统一实现 `Tool`。

| 工具 | 实现 | 说明 |
|---|---|---|
| `search_poi` | 快照 + 高德 | 40～60 个候选，必去点保证在集合内 |
| `get_poi_detail` | 快照 | 营业时间、门票、建议时长 |
| `route` | 高德 | 失败时直线距离降级 |
| `weather` | 高德 | 软提示，失败不阻塞 |
| `geocode` | 高德 | 可选 |

**依赖**：高德客户端、快照仓库、`plugin_cache`。

### 5.5 `mcp`

**职责**：MCP 连接和治理，不参与选点。

- 白名单：只允许配置过的 server/tool。
- 超时 15 秒、重试 2 次、熔断、单 run 最多 5 次。
- 结果归一化为 `Evidence`；只有带坐标且通过校验的结果才可进入候选集。
- MVP 先接 native，MCP 用 mock server 跑通链路。

**依赖**：MCP Go SDK、`plugin_cache`、`EventPublisher`。

### 5.6 `repo`

**职责**：PostgreSQL 持久化。

- `pgx/v5` 连接池 + `sqlc` 生成查询。
- `goose` 迁移，启动时 `pg_advisory_lock`。
- 事务：创建 trip+run；保存 version+更新 run。
- 幂等：`request_id` 唯一；冲突返回已有记录。
- 版本序号：`SELECT max(seq) ... FOR UPDATE` 后插入。

**依赖**：PostgreSQL 16。

### 5.7 `events`

**职责**：事件发布、SSE 回放、心跳。

- `Broker`：按 run 维护订阅者；慢客户端丢弃并让其重连。
- `Publish`：先写 `run_events`，再广播；`seq` 由数据库唯一约束保证。
- `SSE`：`Last-Event-ID` 或 `?after=` 回放；每 2 秒心跳；连接关闭清理订阅。

**依赖**：`Store`。

### 5.8 `eval`

**职责**：20 条用例、跑批、指标。

- `CaseLoader`：从 `eval/cases/*.json` 或 `eval_cases` 表加载。
- `Runner`：逐条创建 eval run，记录指标。
- `Metrics`：硬约束满足率、修复轮数、延迟、POI 有效率、JSON 合法率。
- CLI：`cmd/eval` 输出 Markdown 表。

**依赖**：`agent.Runner`、`Store`。

### 5.9 `web`

**职责**：创建页、工作台、分享页、评测面板。

**核心模块**

| 模块 | 职责 |
|---|---|
| `CreateForm` | 一句话输入、示例、幂等 key |
| `ConstraintPanel` | 硬/软约束编辑、缺失字段补充 |
| `RunTrace` | 阶段、工具、校验、修复轮 |
| `ItineraryTimeline` | vis-timeline 展示时间块 |
| `TripMap` | 高德 JS，按天画线、联动 |
| `BudgetPanel` | 门票、餐饮、交通、总计 |
| `VersionSwitcher` | 版本切换、回退 |
| `PlanDiff` | 版本差异 |
| `ShareDialog` | 分享链接、复制文本 |
| `EvalDashboard` | 用例、指标、失败回放 |

**依赖**：REST API、SSE、高德 JS、OpenAPI 生成类型。

## 6. 模块交互

### 6.1 首次规划

```
Client → POST /api/trips
API → Store.CreateTripAndRun（事务）
API → Runner.Start(runID)（异步）
API → 202 {trip_id, run_id, events_url}

Runner → Publish(run.started)
Runner → Eino Graph:
  admit → parse
  parse → missing ? Publish(constraint.needs_input) + RunAwaitingUser
  parse → ok ? retrieve → draft → schedule → validate
  validate → blocking ? repair → schedule → validate（≤3 轮）
  validate → done ? present
Runner → Store.SaveVersion + UpdateRunStatus(awaiting_user)
Runner → Publish(run.awaiting_user)

Client ← SSE 全程接收
```

### 6.2 修复循环

```
validate → []Violation
if blocking > 0 && round < 3 && improved:
    alternatives = retrieve 同类别、同区域、营业中的替代点
    DayPatch = LLM(repair, violations, affectedDay, alternatives)
    plan = merge(plan, DayPatch)
    plan = schedule(plan)
    violations = validate(plan)
    round++
else:
    stop
```

- 只改受影响的天；合并后全量校验。
- `improved` 比较 `(blocking, warning, cost, walking_km)`。
- 连续两轮无改善立即停止。
- 停止后保存最优解 + 剩余违规 + 放宽建议。

### 6.3 调整与重规划

```
POST /api/trips/{id}/adjustments
  → 创建 run(mode=adjustment, base_version)
  → parse intent → scope
  → 只对受影响天检索替代点
  → draft → schedule → validate
  → proposal
  → RunAwaitingUser

POST /api/runs/{id}/resume {action: accept}
  → 生成新 version，旧版本 superseded

POST /api/runs/{id}/resume {action: reject}
  → 不生成新版本，旧版本保持 accepted
```

- 未受影响的天必须逐字段一致；否则拒绝提案并重新修复。
- 无法识别调整对象时追问，不猜测。

### 6.4 SSE

- `Publish` 写入 `run_events` 后广播。
- SSE 客户端带 `Last-Event-ID` 时先回放，再订阅。
- 事件类型：`run.started`、`stage.started`、`stage.completed`、`constraint.parsed`、`constraint.needs_input`、`tool.started`、`tool.completed`、`candidate.retrieved`、`plan.drafted`、`validation.result`、`repair.started`、`repair.completed`、`plan.updated`、`run.awaiting_user`、`run.completed`、`run.failed`。
- 每 2 秒心跳；10 秒无消息客户端重连。

### 6.5 恢复与并发

- 启动时扫描 `runs.status IN ('queued','running')`：`queued` 重新入队，`running` 标记为 `failed` 并提示重试（避免单实例重启后悬挂）。
- `awaiting_user` 不自动恢复，等待用户动作。
- 信号量限制 3 个并发 run；超出的保持 `queued`。
- `cancel` 调用 context cancel，状态置 `cancelled`；终态不可被迟到回调改回。

### 6.6 事务与幂等

- 创建 trip+run 在一个事务内。
- 保存版本 + 更新 run 在一个事务内。
- `trips.request_id`、`runs.request_id` 唯一；重复请求返回已有 ID。
- 版本号在事务内 `FOR UPDATE` 计算，避免并发跳号。
- 分享 slug 用随机 12 位，不可枚举。

## 7. API 摘要

请求/响应细节沿用 [详细实现稿](../../plan.md#6-api-设计)，本计划只固定边界：

- `POST /api/trips`：`{input, city?, client_request_id?}` → `202 {trip_id, run_id, events_url}`。
- `POST /api/trips/{id}/adjustments`：`{input, base_version, client_request_id?}` → `202 {run_id, events_url}`。
- `POST /api/runs/{id}/resume`：`{action: provide_constraints|accept|reject, constraints?}`。
- `POST /api/runs/{id}/cancel`：无 body → `202`。
- `GET /api/runs/{id}/events`：SSE，支持 `Last-Event-ID`。
- 错误统一 `{error: {code, message, request_id}}`。

## 8. 数据库摘要

| 表 | 作用 | 关键约束 |
|---|---|---|
| `trips` | 规划任务 | `request_id` 唯一；`days 1..4` |
| `runs` | 每次执行 | `state` 存恢复状态；`request_id` 唯一 |
| `plan_versions` | 行程版本 | `(trip_id, seq)` 唯一 |
| `run_events` | 事件回放 | `(run_id, seq)` 唯一 |
| `tool_calls` | 工具调用 | `run_id` 索引 |
| `evidence` | 来源与证据 | `trip_id` 索引 |
| `poi_cache` | POI 快照 | 主键 `id`；坐标 GCJ-02 |
| `plugin_cache` | MCP/外部缓存 | `expires_at` 索引 |
| `share_links` | 分享链接 | `slug` 主键 |
| `eval_cases` | 20 条用例 | `id` 主键 |
| `eval_runs` | 评测结果 | `case_id` 索引 |

完整 DDL、索引和字段类型见 [详细实现稿](../../plan.md#52-ddl)。迁移以该 DDL 为准。

## 9. 文件组织

```
cmd/
├─ api/main.go                 — HTTP 入口、依赖装配、迁移
├─ cli/main.go                 — 本地跑一次规划
├─ seed/main.go                — 导入北京 POI 快照
└─ eval/main.go                — 跑 20 条评测

internal/
├─ api/
│  ├─ router.go                — chi 路由
│  ├─ middleware.go            — request ID、限流、recover、admin
│  ├─ trips.go                 — 创建/查询/删除/调整
│  ├─ runs.go                  — 状态/trace/resume/cancel
│  ├─ sse.go                   — SSE 回放、心跳
│  ├─ share.go                 — 分享
│  ├─ eval.go                  — 评测接口
│  └─ errors.go                — 错误映射
├─ agent/
│  ├─ runner.go                — Run 生命周期、并发、恢复
│  ├─ graph.go                 — Eino 图
│  ├─ nodes.go                 — admit/parse/retrieve/draft/schedule/validate/repair/present
│  ├─ repair.go                — DayPatch 合并
│  ├─ state.go                 — TripState 编解码
│  └─ prompts/
│     ├─ parse.tmpl
│     ├─ draft.tmpl
│     ├─ repair.tmpl
│     ├─ adjust.tmpl
│     └─ explain.tmpl
├─ domain/
│  ├─ constraint.go            — ConstraintSet、默认值、合并
│  ├─ poi.go                   — POI、OpenRange
│  ├─ itinerary.go             — DraftPlan、Itinerary、Block
│  ├─ violation.go             — Violation、Level
│  ├─ scheduler/scheduler.go   — 时间、交通、费用
│  ├─ validator/validator.go   — 入口
│  ├─ validator/rules.go       — C1–C12
│  ├─ diff/diff.go             — 版本差异
│  └─ budget/budget.go         — 预算汇总
├─ tools/
│  ├─ tool.go                  — Tool 接口、Registry
│  ├─ amap.go                  — 高德客户端
│  ├─ snapshot.go              — 快照搜索
│  ├─ route.go                 — 路径与降级
│  └─ weather.go               — 天气
├─ mcp/
│  ├─ hub.go                   — 白名单、超时、熔断
│  ├─ adapter.go               — MCP Tool → Tool
│  └─ normalize.go             — Evidence 归一化
├─ repo/
│  ├─ store.go                 — Store 实现
│  ├─ queries/                 — sqlc SQL
│  └─ sqlc/                    — 生成代码
├─ events/
│  ├─ broker.go                — 订阅者管理
│  └─ sse.go                   — SSE 编码
└─ eval/
   ├─ cases.go                 — 用例加载
   ├─ runner.go                — 跑批
   └─ metrics.go               — 指标

migrations/
├─ 0001_init.sql
└─ 0002_eval_cases.sql

data/poi/beijing.json           — 40～60 个 POI
eval/cases/*.json               — 20 条用例
web/                            — React + Vite
deploy/                         — Caddyfile、compose、备份
openapi.yaml                    — API 契约
.env.example                    — 环境变量模板
```

## 10. 技术决策

| 决策点 | 选择 | 理由 |
|---|---|---|
| 语言 | Go 1.24+ | 类型安全、单二进制、并发和内存优势 |
| Agent 编排 | Eino Graph | 图、状态、条件边、回调齐全；只编排，不替代领域代码 |
| 架构 | 模块化单体 | 单人 60 小时；事务和调试简单 |
| LLM 输出 | 只输出 DraftPlan/DayPatch | 缩小幻觉面；时间交通由代码算 |
| 选点 | 闭集 + ID 校验 | 防编造地点 |
| 校验 | 纯函数 C1–C12 | 可单测、可解释、可回归 |
| 存储 | PostgreSQL 16 | 关系 + JSONB；不引入 Redis |
| 事件 | SSE + `run_events` | 单向推送足够；可回放 |
| 地图 | 高德 JS | GCJ-02 对齐，北京精度最好 |
| 前端 | React + Vite + assistant-ui + vis-timeline | 生态成熟、静态部署、少造轮子 |
| API 契约 | OpenAPI 3.1 | 前后端类型同源 |
| MCP | 可选扩展位 | 不阻塞 MVP；第三方能力后接 |
| 部署 | Docker Compose + Caddy | 单机 2C4G 足够 |
| 限流 | 进程内 | 单实例；重启丢失可接受 |
| 并发 | 信号量 3 | 保护 LLM 配额和内存 |
| 认证 | 无账号；UUID/slug 即凭证 | 首版不做账号体系 |

## 11. 需求覆盖

| 需求 | 主要模块 |
|---|---|
| F1 解析 | `agent.parse`、`LLMClient`、`ConstraintSet` |
| F2 约束确认 | `domain.constraint`、`web/ConstraintPanel` |
| F3 候选检索 | `tools.search_poi`、`poi_cache`、`agent.retrieve` |
| F4 草案 | `agent.draft`、`domain.scheduler`、`validator` |
| F5 校验 | `domain.validator` C1–C12 |
| F6 修复 | `agent.repair`、`Runner` 停止条件 |
| F7 调整 | `agent` 调整意图、`AdjustmentProposal`、`resume` |
| F8 版本 | `repo.plan_versions`、`domain.diff`、`web/VersionSwitcher` |
| F9 地图/时间轴 | `web/TripMap`、`web/ItineraryTimeline` |
| F10 分享 | `api.share`、`share_links`、`web/SharePage` |
| F11 追踪/SSE | `events`、`run_events`、`web/RunTrace` |
| F12 评测 | `internal/eval`、`cmd/eval`、`web/EvalDashboard` |

| 非功能 | 机制 |
|---|---|
| N1 性能 | 并行检索、缓存、SSE、少 LLM 调用 |
| N2 可靠性 | 超时/重试、`runs.state`、stop conditions |
| N3 成本 | 调用预算、POI/路径缓存 |
| N4 安全 | 服务端密钥、限流、长度限制、prompt 隔离 |
| N5 合规 | 来源字段、估算标识、AI 标识 |
| N6 可移植 | Compose、环境变量 |
| N7 可观测 | `run_events`、`tool_calls`、`/healthz` |
| N8 可用性 | 地图失败降级、移动端布局 |
| N9 部署范围 | 单实例、无 HA |

## 12. 风险与取舍

| 风险/取舍 | 处理 |
|---|---|
| 高德配额或 Key 问题 | 快照 + 直线距离；地图失败保留时间轴 |
| LLM JSON 不稳定 | Schema + 一次重试 + 非法 ID 拒绝 |
| 修复不收敛 | 3 轮 + 无改善停止 + 最优解 |
| SSE 断线 | 心跳 + `Last-Event-ID` + 事件落库 |
| 单实例重启 | `queued` 重入队；`running` 标失败；`awaiting_user` 保留 |
| 无账号体系 | UUID/slug 即凭证；分享页只读 |
| 不做 SSR | 分享页 OG 后补；首版静态部署 |
| 不做 Redis | 进程内限流和事件；多实例时再换 |

## 13. 审批

请确认本 plan。确认后进入 `docs/mew-spec/v1-mvp/task.md`，按文件清单和依赖顺序拆成 2–5 分钟可完成的任务；在 `task.md` 通过前不写实现代码。