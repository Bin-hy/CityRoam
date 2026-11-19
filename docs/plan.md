# TripPilot 实施计划

| 项目 | 内容 |
|---|---|
| 版本 | v0.1 |
| 状态 | 待确认 |
| 关联 | [项目立项 PRD](prd/项目立项.md) · [系统架构设计](architecture/系统架构设计.md) · [后端架构选型](architecture/后端架构选型.md) |
| 说明 | 本文是实现计划；接口、DDL、状态机、任务拆分以本文为准，PRD 中的示例 DDL 不再单独维护 |

## 1. 目标与验收口径

**总目标**：6 周、约 60 小时，交付一个可运行、可评测、可演示的北京旅行规划 Agent。

**硬验收**

- 一句话输入能产出可执行行程。
- 阻断项能在 3 轮内清零，或明确告知剩余问题。
- 浏览器能看到逐轮修复过程。
- 20 条评测用例有指标表。
- 陌生人按 README 十分钟内跑通。
- 不采集身份信息，密钥只在服务端。

**指标门槛**

| 指标 | 目标 |
|---|---|
| 硬约束满足率 | ≥90% |
| LLM JSON 合法率 | ≥99%（含一次重试） |
| POI 有效率 | 100% |
| 3 天行程 p95 | ≤60s |
| 平均修复轮数 | ≤2 |
| 单次 LLM 成本 | ≤0.5 元 |

## 2. 实现范围

**P0，必须做**

- 北京，1～4 天，40～60 个 POI 快照。
- 自然语言解析 + 结构化约束确认。
- 候选检索、草案生成、C1～C12 校验、最多 3 轮修复。
- 版本、差异、回退。
- SSE 进度、地图、时间轴。
- 分享链接 + 文本复制。
- 20 条评测用例与指标面板。
- Go + Eino 后端，React + Vite 前端，PostgreSQL，高德 JS/Web 服务。

**P1，时间够再做**

- 偏好记忆。
- 分享页地图。
- PDF / 日历导出。
- 多城市。
- 实时变更提醒。

**不做**

- 预订、支付、订单。
- 账号、多人协作、社区。
- 酒店、机票、跨城交通。
- 实时路况、排队时间。
- 原生 App、图像/语音/视频生成。
- 微服务、Redis、消息队列、K8s。

## 3. 仓库结构

```
AutoAgent/
├─ cmd/
│  ├─ api/                 # HTTP 服务入口
│  ├─ cli/                 # 本地调试入口
│  ├─ seed/                # POI 快照导入
│  └─ eval/                # 评测跑批
├─ internal/
│  ├─ api/                 # chi 路由、中间件、SSE
│  ├─ agent/               # Eino Graph、节点、Prompt
│  │  └─ prompts/
│  ├─ domain/              # 约束、行程、校验、调度
│  │  ├─ validator/
│  │  └─ scheduler/
│  ├─ mcp/                 # McpHub、工具适配
│  ├─ tools/               # 高德、快照、直线距离、天气
│  ├─ repo/                # pgx + sqlc
│  ├─ events/              # SSE broker
│  └─ eval/                # 用例、指标
├─ migrations/             # goose SQL
├─ data/poi/beijing.json   # POI 快照
├─ web/                    # React + Vite
├─ deploy/                 # Caddyfile、compose、备份
├─ openapi.yaml
└─ docs/
```

## 4. 核心数据契约

### 4.1 ConstraintSet

```json
{
  "city": "beijing",
  "days": 2,
  "start_date": "2026-11-23",
  "party_size": 1,
  "budget": { "total_yuan": 800, "hard": true },
  "daily": { "max_hours": 9, "start_time": "09:00", "end_time": "21:00" },
  "meals": {
    "enabled": true,
    "lunch_window": ["11:30", "13:30"],
    "dinner_window": ["17:30", "19:30"],
    "budget_per_meal_yuan": 60
  },
  "pace": "relaxed",
  "interests": ["history", "coffee"],
  "must_visit": ["poi_forbidden_city"],
  "avoid": ["theme_park"],
  "walking": { "max_km_per_day": 8 },
  "weather": { "rain_aware": true },
  "hard": ["budget", "must_visit", "daily.max_hours"],
  "soft": ["interests", "pace"]
}
```

- `pace`：`relaxed | normal | packed`。
- `city`、`days` 缺失时进入 `awaiting_user`，不擅自假设。
- `hard` / `soft` 决定 C5 等规则的级别。

### 4.2 DraftPlan（LLM 输出）

```json
{
  "days": [
    {
      "date": "2026-11-23",
      "items": [
        { "poi_id": "poi_forbidden_city", "type": "poi", "duration_min": 180, "start_hint": "09:00" },
        { "poi_id": "poi_lunch_01", "type": "meal", "duration_min": 60, "start_hint": "12:00" }
      ]
    }
  ],
  "explanations": [
    { "poi_id": "poi_forbidden_city", "text": "上午精力好，先走核心景点。" }
  ]
}
```

- LLM 只输出顺序、POI ID、建议时长和开始提示。
- 具体到访时间、交通块由后端 `Scheduler` 计算。
- `poi_id` 必须来自候选集，否则整份草案判非法并重试一次。

### 4.3 Itinerary（落库格式）

```json
{
  "city": "beijing",
  "days": [
    {
      "date": "2026-11-23",
      "start_time": "09:00",
      "end_time": "20:10",
      "blocks": [
        {
          "id": "b1",
          "type": "poi",
          "poi_id": "poi_forbidden_city",
          "title": "故宫博物院",
          "start": "09:00",
          "end": "12:00",
          "duration_min": 180,
          "cost_yuan": 60
        },
        {
          "id": "b2",
          "type": "transit",
          "mode": "walk",
          "from": "poi_forbidden_city",
          "to": "poi_lunch_01",
          "distance_m": 850,
          "duration_min": 12,
          "cost_yuan": 0
        }
      ],
      "day_cost_yuan": 260
    }
  ],
  "totals": { "cost_yuan": 760, "poi_count": 9, "walking_km": 12.4 }
}
```

### 4.4 Violation

```json
{
  "code": "C1",
  "level": "blocking",
  "day": 1,
  "block_id": "b3",
  "poi_id": "poi_forbidden_city",
  "message": "故宫 17:00 闭馆，当前安排 16:30–18:00",
  "suggestion": "提前到 14:00，或换到第二天上午"
}
```

### 4.5 RunEvent

```json
{
  "seq": 42,
  "type": "validation.result",
  "run_id": "uuid",
  "payload": { "blocking": 2, "warnings": 1, "violations": [] },
  "created_at": "2026-11-19T10:00:00Z"
}
```

## 5. 数据库设计

### 5.1 迁移

- `migrations/0001_init.sql`：全部表。
- `migrations/0002_eval_cases.sql`：20 条用例种子。
- 启动时用 goose 执行，带 `pg_advisory_lock(42)`，避免多实例重复迁移。
- POI 快照不写死在 SQL，用 `cmd/seed` 从 `data/poi/beijing.json` upsert。

### 5.2 DDL

```sql
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE trips (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  city text NOT NULL,
  days int NOT NULL CHECK (days BETWEEN 1 AND 4),
  budget_yuan int CHECK (budget_yuan >= 0),
  constraints jsonb NOT NULL DEFAULT '{}',
  status text NOT NULL CHECK (status IN ('parsing','planning','awaiting_user','completed','failed','cancelled')),
  request_id text UNIQUE,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);
CREATE INDEX trips_created_idx ON trips (created_at DESC);

CREATE TABLE runs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  trip_id uuid NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
  mode text NOT NULL CHECK (mode IN ('initial','adjustment','eval')),
  status text NOT NULL CHECK (status IN ('queued','running','awaiting_user','completed','failed','cancelled')),
  stage text NOT NULL DEFAULT 'admit',
  round int NOT NULL DEFAULT 0,
  input text NOT NULL,
  constraints jsonb NOT NULL DEFAULT '{}',
  base_version_id uuid,
  state jsonb NOT NULL DEFAULT '{}',
  proposal jsonb,
  error_code text,
  error_message text,
  request_id text UNIQUE,
  started_at timestamptz,
  finished_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX runs_trip_idx ON runs (trip_id, created_at DESC);

CREATE TABLE plan_versions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  trip_id uuid NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
  run_id uuid NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  seq int NOT NULL,
  plan jsonb NOT NULL,
  constraints jsonb NOT NULL,
  violations jsonb NOT NULL DEFAULT '[]',
  diff_summary jsonb,
  reason text,
  status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','accepted','superseded')),
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (trip_id, seq)
);
CREATE INDEX plan_versions_run_idx ON plan_versions (run_id);

CREATE TABLE run_events (
  id bigserial PRIMARY KEY,
  run_id uuid NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  seq int NOT NULL,
  type text NOT NULL,
  payload jsonb NOT NULL DEFAULT '{}',
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (run_id, seq)
);
CREATE INDEX run_events_run_seq_idx ON run_events (run_id, seq);

CREATE TABLE tool_calls (
  id bigserial PRIMARY KEY,
  run_id uuid NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  trip_id uuid NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
  tool text NOT NULL,
  provider text NOT NULL DEFAULT 'native' CHECK (provider IN ('native','mcp')),
  input jsonb NOT NULL,
  output jsonb,
  status text NOT NULL DEFAULT 'ok' CHECK (status IN ('ok','error','timeout','degraded')),
  latency_ms int,
  error text,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX tool_calls_run_idx ON tool_calls (run_id);

CREATE TABLE evidence (
  id bigserial PRIMARY KEY,
  trip_id uuid NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
  run_id uuid REFERENCES runs(id) ON DELETE SET NULL,
  poi_id text,
  kind text NOT NULL,
  claim text NOT NULL,
  source text NOT NULL,
  source_url text,
  confidence numeric(3,2) CHECK (confidence BETWEEN 0 AND 1),
  fetched_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX evidence_trip_idx ON evidence (trip_id);

CREATE TABLE poi_cache (
  id text PRIMARY KEY,
  city text NOT NULL,
  name text NOT NULL,
  category text NOT NULL,
  area text,
  address text,
  lat double precision NOT NULL,
  lng double precision NOT NULL,
  coord_system text NOT NULL DEFAULT 'GCJ-02',
  open_hours jsonb,
  ticket_yuan int,
  suggested_minutes int,
  rating numeric(2,1) CHECK (rating BETWEEN 0 AND 5),
  outdoor boolean NOT NULL DEFAULT false,
  tags text[] NOT NULL DEFAULT '{}',
  source text NOT NULL,
  source_version text,
  verified_at date,
  fetched_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX poi_cache_city_category_idx ON poi_cache (city, category);

CREATE TABLE plugin_cache (
  key text PRIMARY KEY,
  provider text NOT NULL,
  value jsonb NOT NULL,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX plugin_cache_expires_idx ON plugin_cache (expires_at);

CREATE TABLE share_links (
  slug text PRIMARY KEY,
  trip_id uuid NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
  version_id uuid NOT NULL REFERENCES plan_versions(id) ON DELETE CASCADE,
  revoked_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE eval_cases (
  id text PRIMARY KEY,
  input text NOT NULL,
  expected jsonb NOT NULL,
  tags text[] NOT NULL DEFAULT '{}',
  enabled boolean NOT NULL DEFAULT true
);

CREATE TABLE eval_runs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  case_id text NOT NULL,
  run_id uuid REFERENCES runs(id) ON DELETE SET NULL,
  metrics jsonb NOT NULL,
  passed boolean NOT NULL,
  duration_ms int,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX eval_runs_case_idx ON eval_runs (case_id, created_at DESC);
```

### 5.3 数据规则

- `open_hours` 结构：`[{"days":[1,2,3,4,5,6,0],"start":"09:00","end":"17:00"}]`，`0` 为周日。
- 坐标统一 GCJ-02；快照和高德返回都不得混入 WGS-84。
- `trips.request_id`、`runs.request_id` 用于幂等。
- `runs.state` 保存可恢复的 Agent 状态；`awaiting_user` 后重启仍能继续。
- 行程删除后 7 天清理；工具调用 30 天；评测长期保留。
- `preferences` 表 P1 再加，不进入首版迁移。

## 6. API 设计

### 6.1 端点

| 方法 | 路径 | 用途 | 访问 |
|---|---|---|---|
| POST | `/api/trips` | 创建行程并启动首次规划 | 公开，限流 |
| GET | `/api/trips/{id}` | 行程摘要 + 最新版本 | 公开，UUID 即凭证 |
| DELETE | `/api/trips/{id}` | 软删除 | 公开，UUID 即凭证 |
| POST | `/api/trips/{id}/adjustments` | 提交自然语言调整 | 公开，限流 |
| POST | `/api/trips/{id}/share` | 创建/刷新分享链接 | 公开 |
| GET | `/api/runs/{id}` | 运行状态 | 公开，UUID 即凭证 |
| GET | `/api/runs/{id}/events` | SSE 事件流 | 公开 |
| GET | `/api/runs/{id}/trace` | 完整 trace | 公开，UUID 即凭证 |
| POST | `/api/runs/{id}/resume` | 补充约束 / 接受 / 拒绝 | 公开，UUID 即凭证 |
| GET | `/api/trips/{id}/versions` | 版本列表 | 公开 |
| GET | `/api/trips/{id}/versions/{seq}` | 版本详情 | 公开 |
| GET | `/api/trips/{id}/diff?from=1&to=2` | 版本差异 | 公开 |
| GET | `/api/share/{slug}` | 只读分享页 | 公开 |
| GET | `/api/meta` | 支持城市、默认值、版本 | 公开 |
| GET | `/api/eval/cases` | 评测用例 | Admin token |
| POST | `/api/eval/runs` | 跑评测 | Admin token |
| GET | `/api/eval/runs` | 评测结果 | Admin token |
| GET | `/healthz` | 存活 | 内网/公开 |
| GET | `/readyz` | 数据库就绪 | 内网 |

### 6.2 创建行程

```http
POST /api/trips
Idempotency-Key: 7f3c...
Content-Type: application/json

{
  "input": "北京两天，预算800，喜欢历史和咖啡，不要太赶，周一出发",
  "city": "beijing"
}
```

```json
{
  "trip_id": "uuid",
  "run_id": "uuid",
  "status": "running",
  "events_url": "/api/runs/uuid/events"
}
```

- `city` 可省略，由 Parse 提取；提取不到则进入 `awaiting_user`。
- 同一 `Idempotency-Key` 返回同一 trip/run。

### 6.3 调整

```http
POST /api/trips/{id}/adjustments

{
  "input": "第二天下午轻松点，别安排太远",
  "base_version": 3,
  "client_request_id": "uuid"
}
```

```json
{
  "run_id": "uuid",
  "status": "running",
  "events_url": "/api/runs/uuid/events"
}
```

- 调整先产出提案，run 停在 `awaiting_user`。
- 用户 `accept` 后才生成新版本；`reject` 保留原版本。

### 6.4 恢复/决策

```http
POST /api/runs/{id}/resume

{ "action": "provide_constraints", "constraints": { "start_date": "2026-11-23" } }
```

```json
{ "action": "accept" }
```

```json
{ "action": "reject" }
```

- `provide_constraints`：补齐缺失字段后继续。
- `accept`：首次规划确认，或接受调整提案。
- `reject`：取消本次调整，保留原版本。

### 6.5 SSE

```http
GET /api/runs/{id}/events
Last-Event-ID: 41
```

事件格式：

```
id: 42
event: validation.result
data: {"blocking":2,"warnings":1,"violations":[...]}

: heartbeat
```

| 事件 | 说明 |
|---|---|
| `run.started` | 运行开始 |
| `stage.started` | 阶段开始 |
| `stage.completed` | 阶段结束，含耗时 |
| `constraint.parsed` | 约束解析完成 |
| `constraint.needs_input` | 缺关键字段，等待补充 |
| `tool.started` | 工具开始 |
| `tool.completed` | 工具结束，含耗时/状态 |
| `candidate.retrieved` | 候选数量、必去点命中 |
| `plan.drafted` | 草案生成 |
| `validation.result` | 违规列表 |
| `repair.started` | 修复轮开始 |
| `repair.completed` | 修复轮结束，含前后对比 |
| `plan.updated` | 新版本产生 |
| `run.awaiting_user` | 等待用户确认/调整 |
| `run.completed` | 完成 |
| `run.failed` | 失败，含错误码 |

- 每 2 秒至少一次心跳。
- 客户端断线用 `Last-Event-ID` 续传。
- 事件同时落 `run_events`，保证可回放。

### 6.6 错误格式

```json
{
  "error": {
    "code": "INVALID_INPUT",
    "message": "input 不能为空",
    "request_id": "uuid"
  }
}
```

错误码：`INVALID_INPUT`、`UNSUPPORTED_CITY`、`PARSE_FAILED`、`RETRIEVE_FAILED`、`LLM_INVALID_JSON`、`VALIDATION_ERROR`、`RATE_LIMITED`、`NOT_FOUND`、`CONFLICT`、`RUN_CANCELLED`。

## 7. Agent 与 Eino Graph

### 7.1 职责划分

- **Eino Graph**：`parse → retrieve → draft → validate → repair` 的内循环。
- **Service 状态机**：`await_user`、`accept/reject`、`replan`、`finalize` 的外层流程。
- **Domain**：校验、调度、预算、时间轴，纯函数。
- **Repo**：状态、版本、事件、工具调用持久化。

这样 Eino 只负责它擅长的编排，人工中断和恢复不被塞进图里。

### 7.2 节点

| 节点 | 类型 | 输入 | 输出 |
|---|---|---|---|
| `admit` | Lambda | input | 校验后的输入 |
| `parse` | ChatModel + Schema | input | ConstraintSet / missing |
| `retrieve` | Lambda + Tools | ConstraintSet | 40～60 个候选 |
| `draft` | ChatModel + Schema | 候选 + 约束 | DraftPlan |
| `schedule` | Lambda | DraftPlan | Itinerary |
| `validate` | Lambda | Itinerary | Violations |
| `repair` | ChatModel + Schema | 违规 + 候选 + 受影响天 | DayPatch |
| `present` | Lambda + 模板 | 最优行程 | 版本 + 解释 |

### 7.3 边与停止

```
START → admit → parse
parse → missing ? await_user : retrieve
retrieve → draft → schedule → validate
validate → blocking && can_repair ? repair → schedule → validate
validate → done ? present → await_user
await_user → accept ? finalize
await_user → adjust ? replan → retrieve → draft → schedule → validate
await_user → cancel ? cancelled
```

停止条件：

1. 阻断项清零。
2. 达到 3 轮。
3. 连续两轮阻断数没有下降。
4. 用户取消。
5. 不可恢复错误。

停止后输出：当前最优行程、剩余违规、停止原因、建议放宽的约束。

### 7.4 TripState

```go
type TripState struct {
    TripID       string
    RunID        string
    Mode         string
    Input        string
    Constraints  domain.ConstraintSet
    Missing      []string
    Candidates   []domain.POI
    Draft        domain.DraftPlan
    Plan         domain.Itinerary
    Violations   []domain.Violation
    Round        int
    Best         *domain.Itinerary
    BestViolations []domain.Violation
    Proposal     *domain.AdjustmentProposal
    Error        *domain.RunError
}
```

- 每个节点结束后 `repo.SaveRunState` 写入 `runs.state`。
- `await_user` 时图结束；`resume` 从 DB 恢复 state，重新进入对应子流程。
- 终态 `completed/failed/cancelled` 不允许被迟到回调改回进行中。

### 7.5 修复策略

- 只把阻断项、受影响那一天、同类替代候选发给 LLM。
- LLM 输出 `DayPatch`：

```json
{
  "day_index": 0,
  "items": [
    { "poi_id": "poi_alt_01", "type": "poi", "duration_min": 120, "start_hint": "14:00" }
  ],
  "reason": "故宫时间不够，换成国家博物馆"
}
```

- 合并后重新调度、全量校验，不允许跳过。
- 比较每轮 `(阻断数, 警告数, 成本, 步行距离)`，保留最优解。
- 连续两轮无改善立即停止。

## 8. 校验器与调度器

### 8.1 调度器

输入：`DraftPlan` + 候选 POI + 约束。

1. 每天从 `daily.start_time` 开始。
2. 第一个 POI 不计算市内出发交通；从第二个点开始插入交通块。
3. 到达时间 = 上一块结束 + 路程 + 缓冲；早到则等待。
4. 缓冲：同区域 10 分钟，跨区域 20 分钟。
5. 路程用高德；失败时直线距离估算：步行 4 km/h，公共交通 15 km/h，再加 20% 缓冲，并标 `degraded`。
6. 交通费用未知时按 2 元/段估算，标 `estimated`。
7. 输出完整 `Itinerary`，包含交通块和每天费用。

### 8.2 校验规则

| 编号 | 规则 | 级别 | 实现要点 |
|---|---|---|---|
| C1 | 营业时间内 | 阻断 | 未知营业时间降为警告并标「待确认」 |
| C2 | 闭馆日 | 阻断 | 从 `open_hours.days` 推导；未知降为警告 |
| C3 | 路程 + 缓冲 | 阻断 | 相邻块间隔 ≥ 路程 + 缓冲；降级估算时警告 |
| C4 | 每日时长 | 阻断 | `end - start ≤ max_hours` |
| C5 | 预算 | 硬约束阻断，软约束警告 | 门票 + 餐饮 + 交通 |
| C6 | 餐点 | 缺餐阻断，超窗口 30 分钟警告 | 午餐 11:30–13:30，晚餐 17:30–19:30 |
| C7 | 必去点 | 阻断 | 全部出现在行程中 |
| C8 | 重复 | 阻断 | 同一 POI 不跨天、不跨块重复 |
| C9 | 节奏 | 警告 | 轻松 2–3、正常 3–4、紧凑 4–5 个 POI/天 |
| C10 | 起止时间 | 阻断 | 不早于开始、不晚于结束 |
| C11 | 雨天户外比例 | 警告 | 户外点 >60% 提示 |
| C12 | 最短停留 | 警告 | 不低于建议时长 60% |

### 8.3 接口

```go
func Validate(plan domain.Itinerary, c domain.ConstraintSet, pois map[string]domain.POI, routes domain.RouteProvider) []domain.Violation
```

- 每条规则一个函数，独立单测。
- `Violation` 必须带 `code`、`level`、`day`、`block_id`、`message`、`suggestion`。
- 校验器异常视为阻断，不输出未校验行程。

## 9. 工具与 MCP

### 9.1 统一接口

```go
type Tool interface {
    Name() string
    InputSchema() json.RawMessage
    Call(ctx context.Context, input json.RawMessage) (json.RawMessage, error)
}
```

Native 和 MCP 都实现该接口，Eino 只看到一种工具。

### 9.2 工具清单

| 工具 | 实现 | 超时 | 缓存 | 失败处理 |
|---|---|---|---|---|
| `search_poi` | 快照 + 高德 | 10s | 24h | 扩大范围重试一次 |
| `get_poi_detail` | 快照 | 2s | 无 | 返回快照并标「待确认」 |
| `route` | 高德 | 10s | 24h | 直线距离降级 |
| `weather` | 高德 | 5s | 6h | 跳过，不阻塞 |
| `geocode` | 高德 | 5s | 24h | 要求用户从候选选择 |
| `mcp.xhs_search` | MCP | 15s | 6h | 返回空证据，不阻塞 |
| `mcp.web_search` | MCP | 15s | 6h | 返回空证据，不阻塞 |

### 9.3 McpHub

- 白名单：只允许配置过的 server/tool。
- 超时、重试 2 次、熔断、单次运行最多 5 次 MCP 调用。
- 所有结果归一化为 `evidence`，不直接变成候选；只有带坐标且通过校验的结果才可进入候选集。
- MCP 内容视为不可信输入，限制长度、剥离指令性文本。
- MVP 先接 native；MCP 用 mock server 跑通链路，第三方搜索按需接入。

## 10. LLM 调用与 Prompt

### 10.1 配置

```env
LLM_BASE_URL=https://api.deepseek.com
LLM_API_KEY=...
LLM_MODEL=deepseek-chat
```

### 10.2 调用预算

| 调用 | 次数 | 温度 |
|---|---|---|
| Parse | 1 | 0.2 |
| Draft | 1 | 0.2 |
| Repair | 0～3 | 0.2 |
| Explain | 0～1 | 0.4 |
| Adjustment intent | 0～1 | 0.2 |

- 总调用 ≤7 次，成本 ≤0.5 元。
- Prompt 存 `internal/agent/prompts/`，用 `embed` 打包。
- 所有输出用 JSON Schema 约束；失败重试 1 次，附带解析错误。
- 用户输入只作为数据，不执行其中的指令。
- 解释文本与结构化字段分开，避免污染数据。

## 11. 前端实现

### 11.1 技术栈

React 19 + Vite + TypeScript + Tailwind + shadcn/ui + assistant-ui + TanStack Query + 高德 JS + vis-timeline + jsondiffpatch。

### 11.2 路由

| 路由 | 页面 |
|---|---|
| `/` | 创建页：一句话输入、示例、高级选项 |
| `/trips/:id` | 工作台：轨迹、地图、时间轴、预算、版本 |
| `/share/:slug` | 只读分享页 |
| `/eval` | 内部评测面板 |

### 11.3 关键组件

| 组件 | 职责 |
|---|---|
| `CreateForm` | 输入、示例、提交、幂等 key |
| `ConstraintPanel` | 硬/软约束编辑、缺失字段补充 |
| `RunTrace` | 阶段、工具、校验、修复轮 |
| `ItineraryTimeline` | vis-timeline，按天展示块 |
| `TripMap` | 高德 JS，按天画线、点选联动 |
| `BudgetPanel` | 门票、餐饮、交通、总计 |
| `VersionSwitcher` | 版本切换、回退 |
| `PlanDiff` | jsondiffpatch + diff viewer |
| `ShareDialog` | 生成链接、复制文本 |
| `EvalDashboard` | 用例、指标、失败回放 |

### 11.4 SSE Hook

```ts
useRunStream(runId) → { events, status, plan, reconnecting }
```

- 用 `fetch-event-source`，带 `Last-Event-ID`。
- 收到 `plan.updated` 后失效 TanStack Query 缓存。
- 10 秒无心跳重连，指数退避。
- `run.completed` / `run.failed` 后关闭连接。

### 11.5 地图

- `@amap/amap-jsapi-loader` 加载高德 JS。
- 环境变量：`VITE_AMAP_JS_KEY`、`VITE_AMAP_SECURITY_CODE`。
- 域名白名单在密钥后台配置。
- 坐标为 GCJ-02，不做转换。
- 每天一种颜色，时间轴点击 → 地图高亮。
- 地图加载失败时保留时间轴和文本，不阻塞主流程。
- 分享页首版只做时间轴 + 预算，地图 P1。

### 11.6 状态

- 服务端状态：TanStack Query。
- 运行流：`useReducer` 维护 trace/plan。
- 不引入 Redux/Zustand；跨组件用 Context。
- OpenAPI 生成前端类型，避免手写接口类型。

## 12. 部署与运维

### 12.1 镜像

- `api`：多阶段构建，`golang:1.24` → `distroless/static`。
- `web`：`node:22` 构建 → `caddy:2`，内置 `web/dist` 和 `Caddyfile`。
- `postgres:16`。

### 12.2 Compose

```yaml
services:
  caddy:
    build: ./deploy/web
    ports: ["80:80", "443:443"]
    volumes: [caddy_data:/data]
    depends_on: [api]
  api:
    build: .
    env_file: .env
    depends_on: [postgres]
  postgres:
    image: postgres:16
    volumes: [pgdata:/var/lib/postgresql/data]
volumes:
  caddy_data:
  pgdata:
```

### 12.3 Caddy

- `/` 静态资源。
- `/api/*` 反代 `api:8080`。
- SSE 关闭响应缓冲，读超时放宽。
- 安全头：HSTS、X-Content-Type-Options、Referrer-Policy。
- 可选 `mcp-*` 容器，仅内网。

### 12.4 配置

| 变量 | 用途 |
|---|---|
| `DATABASE_URL` | Postgres |
| `LLM_BASE_URL` / `LLM_API_KEY` / `LLM_MODEL` | 模型 |
| `AMAP_WEB_KEY` | 高德 Web 服务 |
| `AMAP_JS_KEY` / `AMAP_SECURITY_CODE` | 前端地图 |
| `ADMIN_TOKEN` | 评测接口 |
| `RATE_LIMIT_PER_HOUR` | 默认 10 |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | 可选 |

### 12.5 运维

- 启动时 goose 迁移 + advisory lock。
- `/healthz` 存活，`/readyz` 数据库 ping。
- 每日 `pg_dump`，保留 7 天。
- 结构化日志到 stdout。
- 限流按 IP 10 次/小时，单实例内存实现。
- 发布：GitHub Actions → GHCR → SSH `compose pull && up -d`。
- 回滚：保留上一镜像 tag，`compose up` 指定 tag。

## 13. 测试与评测

### 13.1 测试分层

| 层 | 内容 | 工具 |
|---|---|---|
| 单元 | C1–C12、调度、预算、约束合并 | Go testing + testify |
| 集成 | Graph + fake LLM + fixture POI + Postgres | testcontainers / compose |
| API | 路由、幂等、SSE 事件顺序 | httptest |
| 前端 | 组件、SSE reducer | Vitest |
| E2E | 创建 → 修复 → 调整 → 分享 | Playwright |
| 评测 | 20 条用例 + 指标 | `cmd/eval` |

### 13.2 评测用例分布

| 类型 | 数量 |
|---|---|
| 1 天 | 4 |
| 2 天 | 6 |
| 3 天 | 5 |
| 4 天 | 2 |
| 雨天 / 闭馆日 | 2 |
| 必去 / 预算冲突 | 1 |

指标：硬约束满足率、修复轮数、延迟、POI 有效率、JSON 合法率、人工评分。

### 13.3 CI

- `go vet ./...`
- `golangci-lint run`
- `go test -race ./...`
- `npm ci && npm test && npm run build`
- `docker build`
- 评测不跑全量，手动触发，避免消耗 LLM 配额。

## 14. 六周任务拆分

| 周 | 小时 | 任务 | 验收 |
|---|---|---|---|
| W1 | 10 | 仓库/Compose/CI 骨架 2h；北京 POI 快照 50 个 3h；search/detail/route 工具 + CLI 3h；fake planner 输出 Itinerary 2h | CLI 输入需求能产出第一版行程 JSON |
| W2 | 10 | Scheduler 3h；C1–C12 4h；修复循环 + 5 个用例 3h | 5 个用例阻断项 3 轮内清零 |
| W3 | 10 | 真实 LLM Parse/Draft 3h；Repair Prompt 2h；run_events/trace 2h；15 条用例 + 指标表 3h | 首版评测报告完成 |
| W4 | 10 | API + SSE 2h；工作台 + 时间轴 4h；高德地图 2h；版本/diff 2h | 浏览器能看到逐轮修复和版本切换 |
| W5 | 10 | Postgres 持久化 + 断点恢复 3h；缓存/重试/幂等 3h；分享链接 2h；20 条回归 2h | 模拟工具超时后任务仍能完成 |
| W6 | 10 | README/架构图 2h；demo 视频 2h；可选部署 2h；修 bug + 彩排 4h | 陌生人按 README 十分钟跑通 |

**里程碑**

- M0（W1 末）：无外部依赖的 walking skeleton。
- M1（W3 末）：真实 Agent 循环 + 评测。
- M2（W4 末）：浏览器可用。
- M3（W5 末）：可恢复、可分享。
- M4（W6 末）：可演示发布。

## 15. 风险与降级

| 风险 | 降级 |
|---|---|
| 高德配额/密钥问题 | 快照 + 直线距离；地图加载失败保留时间轴 |
| LLM JSON 不稳定 | Schema + 重试 1 次 + 规则兜底 |
| 修复不收敛 | 3 轮上限 + 连续无改善停止 + 输出最优解 |
| 时间不够 | 砍 P1；每周必须有可演示增量 |
| POI 数据不准 | 标 `verified_at`，未知标「待确认」 |
| SSE 断线 | 心跳 + `Last-Event-ID` 续传 + 事件落库 |
| 范围蔓延 | 新需求必须换掉一个同等工作量项 |

## 16. 完成定义

- `docker compose up` 可启动，CLI 可跑。
- 首次规划、修复、调整、版本、分享全链路可用。
- 20 条评测用例有指标表，达到门槛。
- 浏览器主路径可用，地图失败不影响时间轴。
- README 含架构图、运行方式、评测结果、已知限制。
- 3 分钟 demo 视频。
- CI 通过，无密钥入库。
- P1/P2 未做项在 README 明确列出。

## 17. 待确认

| 事项 | 建议默认 |
|---|---|
| 天气 | 只做软提示，失败不阻塞 |
| 分享页地图 | P1，首版时间轴 + 预算 |
| 公网部署 | 本地 demo + 可选 VPS |
| 评测用例 | 20 条，自己和 2 位同学各出一部分 |

确认后进入 `task.md`，按周拆成可勾选任务；`task.md` 通过后再写 `checklist.md` 并开始实现。