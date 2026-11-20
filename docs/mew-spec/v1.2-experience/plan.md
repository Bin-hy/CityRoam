# TripPilot v1.2 体验修复 Plan

对应 spec：[spec.md](file:///Users/binhy/Binhy-Projects/AutoAgent/docs/mew-spec/v1.2-experience/spec.md)

## 1. 架构概览

不引入新服务、不新增数据表。四项修复落在既有的分层里，各层职责保持：

```
装配层   wire.go / router.go(meta)       → 插件状态规范化（F1–F3）
运行层   runner.go / revise.go / runstore → 拒绝改稿、同 run 转 adjustment（F4–F8）
研究层   research.go / hotel_resolver.go  → 酒店解析优先级、全天锚定（F9–F14）
领域层   domain/constraint + domain/draft → AnchorDays、DraftFromPlan
工具层   tools/route.go / polyline.go     → 分段几何解析、简化、城市参数（F15–F18）
前端     RunActions / TripMap / types     → 反馈表单、逐段绘制、酒店标记（F4–F8, F15–F18）
```

核心设计选择：

| 决策 | 选择 | 理由 / 被否方案 |
|---|---|---|
| 拒绝改稿 | 同一个 run 内转入 `mode=adjustment` | 前端 SSE 订阅、run ID、已存基线版本天然延续；新建 run 会让旧 run 变 cancelled，前端要重建订阅，且“取消”语义与“改稿”混淆 |
| 提案拒绝后的基线 | 按 `PreviousSeq` 从 `plan_versions` 回读恢复 | 版本表是事实源；不在 TripState 里复制 BasePlan/BaseDraft/BaseCandidates，避免状态 JSON 膨胀 |
| 取消入口 | 保持 `POST /runs/{id}/cancel`，前端新增按钮 | 与 revise 分离，取消仍是显式终态（F7） |
| 酒店解析 | 工具 → 地理编码 → 提问，确定性代码兜底 | LLM 只负责提议，地理编码不依赖模型；工具未命中时才降级 |
| 路线几何 | 后端存分段折线，前端逐段渲染 | 前端无法获得 AMap 分段数据；后端一次解析可被快照复现（N9） |
| 数据库 | 无 DDL；仅新增按 seq 查版本的读接口与一条 mode 更新 | 所有新字段都在既有 JSON 列（`plan`/`constraints`/`run.state`）内 |

## 2. 状态机变化（F4–F8）

```
                    ┌──────────── accept ───────────► completed
awaiting_user ──────┤
 (proposal)         ├──────────── reject ───────────► awaiting_user (stage=proposal_rejected, 基线已恢复)
                    ├──────────── revise(input) ────► running (mode=adjustment, 同 run)
                    └──────────── cancel ───────────► cancelled

awaiting_user (初始方案) ── revise(input) ──► running (mode=adjustment, 同 run)
                         └─ cancel ────────► cancelled
```

要点：

- `awaiting_user` 保持为唯一人工闸门状态，不新增状态值；只新增 stage 标记 `proposal_rejected`。
- revise 时把用户反馈写入 `state.Input`，复用现有 `executeAdjustment` 路径，不新建 run、不产生新 run ID。
- 服务端同时更新 `runs.mode`（手写一条 `UPDATE`，无 DDL），保证刷新页面/恢复后前端拿到的模式正确。
- 初始方案阶段的“拒绝”不再终止运行，改为进入反馈收集；旧客户端仍调用 `reject` 时，服务端对“有提案”场景执行基线恢复，对“无提案”场景保留原取消行为（兼容，不在 UI 暴露）。

## 3. 关键时序

### A. 拒绝 → 改稿

```
用户点“拒绝”          (前端仅展开反馈框，不发请求)
用户提交反馈  ──► POST /runs/{id}/resume {action:"revise", input}
                 └► ResumeRevise: 基线=Snapshot.Seq（或 PreviousSeq）
                    state.Mode="adjustment"; Input=反馈; Status=queued
                    DB: UpdateRunMode + UpdateRunStatus
                 └► 同 run 继续: adjust → validate → present
                 └► 再次 awaiting_user(proposal)，SSE 沿用同一 run
```

提案拒绝（`proposal_rejected` 状态下的“拒绝”按钮）→ `ResumeReject`：读基线版本恢复 Plan/Draft/Violations/Constraints，清空 Proposal/Snapshot，状态仍 `awaiting_user`，广播 `run.awaiting_user`，不广播 cancelled。

### B. 酒店解析（F9–F14）

```
constraints.Lodging (名称/地址/价格/anchor_days)
  └► research: 先跑已启用工具（FlyAI）
        ├─ 命中指定酒店 → 采用（provider=flyai）
        └─ 未命中且提供了名称/地址 → HotelResolver(AMap Geocode, ≤2 次, 5s)
              ├─ 唯一命中 → 采用（provider=amap，价格可缺）
              ├─ 多候选 → ResearchQuestion（现有 UI 复用）
              └─ 无命中 → ResearchQuestion
  └► anchor_days 优先级：用户显式 > LLM 建议 > 默认全部天
  └► 价格缺失 → warnings + 不计入预算（不阻断）
  └► 输出 Lodging 汇总（含坐标），排期与地图共用
```

### C. 路线几何（F15–F18）

```
scheduler 请求 transit(prev,next,hotel?)
  └► RouteService.fetch
       ├─ walking: steps[].polyline
       ├─ driving: steps[].polyline
       └─ transit: segments[].walking.steps[] + bus.buslines[].polyline
            └─ 拼接为 TransitLeg[] {mode, line_name?, from/to_stop?, duration, distance, cost, polyline}
            └─ Douglas–Peucker 简化（≈15m）→ 截断 ≤60 点/leg、≤600 点/天
  └─ 失败/超限 → 单段 estimated 直线（legs 为空）
前端 TripMap: 有 legs 按模式分色逐段画；无 legs 画灰色虚线直线（旧快照兼容）
```

### D. 插件状态（F1–F3）

```
wire.go 生成 Status.Name = "hotel" / "guide"（规范名）
  └► router handleMeta 现有匹配分支直接命中
  └► 前端 ResearchPanel 显示 启用/未启用 + disabled_reason
补充回归测试：用真实 wiring 输出的 Status 走 meta，断言 enabled 与原因
```

## 4. 核心数据结构

### 4.1 领域层（internal/domain）

**约束：新增可选的锚定天**

```go
type LodgingConstraint struct {
	Mentioned            bool     `json:"mentioned"`
	Hotels               []string `json:"hotels,omitempty"`
	Area                 string   `json:"area,omitempty"`
	MaxPricePerNightYuan int      `json:"max_price_per_night_yuan,omitempty"`
	AnchorDays           []int    `json:"anchor_days,omitempty"` // 0-based 天索引，nil=未指定
}
```

- `MergeConstraints`：`patch.Lodging.AnchorDays != nil` 时覆盖并克隆；nil 表示“本轮未修改”。
- 归一化统一走现有 `domain.NormalizeAnchorDays(days, tripDays)`（去重、排序、过滤越界）。
- 解析 prompt 中明确：用户“第 N 天住 X” → `anchor_days: [N-1]`；“全程住 X” → 留空，由研究层默认全天。

**行程：分段几何**

```go
type TransitLeg struct {
	Mode        string       `json:"mode"`                // walk | transit | drive
	LineName    string       `json:"line_name,omitempty"` // 地铁 1 号线 / 公交 52 路
	FromStop    string       `json:"from_stop,omitempty"`
	ToStop      string       `json:"to_stop,omitempty"`
	DistanceM   int          `json:"distance_m"`
	DurationMin int          `json:"duration_min"`
	CostYuan    int          `json:"cost_yuan"`
	Polyline    [][2]float64 `json:"polyline,omitempty"` // [lng, lat]，GCJ-02
	Estimated   bool         `json:"estimated,omitempty"`
}

type Transport struct {
	Mode        string       `json:"mode"`
	FromPOIID   string       `json:"from_poi_id"`
	ToPOIID     string       `json:"to_poi_id"`
	DistanceM   int          `json:"distance_m"`
	DurationMin int          `json:"duration_min"`
	CostYuan    int          `json:"cost_yuan"`
	Estimated   bool         `json:"estimated"`
	Legs        []TransitLeg `json:"legs,omitempty"` // 空=旧快照或全兜底，前端画虚线
}
```

- `Itinerary.Clone` 增加 Legs 与 Polyline 的深拷贝。
- `LodgingSummary` 已具备 `Hotel`（含坐标）、`AnchorDays`、`PriceEstimated`、`Source`，不新增字段；`Hotel.Provider` 取值扩展为 `flyai` / `amap`。

**草稿重建（从 api 提升为领域能力）**

```go
// 由已定稿行程反推可编辑草稿，供调整运行与提案拒绝恢复共用。
func DraftFromPlan(plan Itinerary) DraftPlan
```

现有 `internal/api/versions.go:draftFromPlan` 逻辑原样迁移，api 侧改为调用 `domain.DraftFromPlan`。

### 4.2 研究层（internal/agent）

```go
type HotelResolver interface {
	// 按名称/地址查询，返回候选（已按城市过滤）。无结果返回空切片。
	Resolve(ctx context.Context, query string) ([]domain.Hotel, error)
}

func WithHotelResolver(r HotelResolver) NodeOption
```

`internal/tools/geocode.go` 提供适配器：

```go
type HotelGeocoder struct { amap *AMapClient; city string } // city="北京"
func NewHotelGeocoder(amap *AMapClient) *HotelGeocoder
func (g *HotelGeocoder) Resolve(ctx context.Context, query string) ([]domain.Hotel, error)
```

- 调 `AMapClient.Geocode`（已存在，当前无调用方），最多 2 次调用、5s 超时由 research 层控制。
- 输出：`ID="amap:<lng>,<lat>"`、`Name=query`、`Address=formatted_address`、`Provider="amap"`、`PricePerNightYuan=0`、`PriceEstimated=true`。

研究结果字段（已有，语义收紧）：

- `AccommodationBrief.AnchorDays`：由 `约束显式 > LLM 建议 > 全部天` 计算后写入，恒非空（有酒店时）。
- `AccommodationBrief.IncludeInBudget / PriceEstimated`：地理编码来源 → `PriceEstimated=true`；`price==0` 且为用户指定酒店时不再清空选择，改为 warning + 不计预算。
- `ResearchBrief.Warnings`：新增固定文案，如“酒店价格为未知，未计入预算”。

### 4.3 运行层（internal/agent）

`TripState` 不新增字段。沿用 `Mode / Input / Plan / Draft / Violations / Proposal / Previous / PreviousSeq / Snapshot / Stage`；新增 stage 取值 `proposal_rejected`。

```go
const ResumeRevise ResumeKind = "revise" // Input=用户修改意见
```

RunStore 扩展：

```go
type VersionState struct {
	Seq         int
	Plan        domain.Itinerary
	Constraints domain.ConstraintSet
	Violations  []domain.Violation
}

type RunStore interface {
	// 既有方法不变 …
	GetPlanVersion(ctx context.Context, tripID string, seq int) (VersionState, error)
	UpdateRunMode(ctx context.Context, runID, mode string) error
}
```

- `runstore` 适配层实现：`GetPlanVersion` 走现有 `GetPlanVersionBySeq`（plan/constraints/violations 反序列化）；`UpdateRunMode` 调 `repo.Store.UpdateRunMode`。
- `VersionState.Draft` 不存，恢复时用 `domain.DraftFromPlan(plan)` 重建。

Runner 内部方法：

```go
func (r *Runner) resumeRevise(ctx context.Context, run RunRecord, state *TripState, input string) error
func (r *Runner) rejectProposal(ctx context.Context, run RunRecord, state *TripState) error
```

- `resumeRevise`：校验 `awaiting_user` + input 非空 ≤2000 → 计算基线 seq（`Snapshot.Seq`，无则 `PreviousSeq`）→ 备份 `Previous=Plan`、`PreviousSeq=基线` → `Mode="adjustment"`、`Input=反馈` → 清 Proposal/Snapshot/Best/History/Suggestions/Round/StopReason/Error，`Stage="adjust"` → `UpdateRunMode` + 落库 + 置 queued → 入队（同 run）。
- `rejectProposal`：`Proposal==nil` 时保留旧取消行为；否则 `GetPlanVersion(TripID, PreviousSeq)` 恢复 Plan/Draft/Violations/Constraints → 清 Proposal/Snapshot → `Stage="proposal_rejected"`、`Status=awaiting_user` → 落库 + 广播 `run.awaiting_user`。
- 两者均复用现有并发信号量与 `execute` 的 adjustment 分支，不新增执行路径。

### 4.4 API 层（internal/api）

```go
type resumeRequest struct {
	Action         string                 `json:"action"` // accept|reject|supplement|resolve_research|revise
	Input          string                 `json:"input"`
	ResearchChoice *agent.ResearchChoice  `json:"research_choice,omitempty"`
}
```

- `revise` 校验：`strings.TrimSpace(input)` 长度 1–2000，否则 400 `invalid_input`。
- `reject` 的响应状态改为读取 resume 后的实际状态（`deps.Store.GetRun`），替换静态 `resumedStatus` 映射；`revise` 返回 `queued`。

OpenAPI（`web/src/api/openapi.yaml`）：

- `/runs/{runID}/resume`：`action` 枚举增加 `revise`；补充 reject 语义说明。
- `Transport`：新增可选 `legs` 数组及 `TransitLeg` schema；`lodging` schema 补充 `anchor_days`。
- `/api/meta` 无结构变化。

### 4.5 前端类型（web/src）

```ts
export interface TransitLeg {
  mode: string;
  line_name?: string;
  from_stop?: string;
  to_stop?: string;
  distance_m: number;
  duration_min: number;
  cost_yuan: number;
  polyline?: [number, number][];
  estimated?: boolean;
}

export interface Transport { /* 既有字段 */ legs?: TransitLeg[] }

export interface LodgingSummary {
  hotel: { id: string; name: string; address?: string; lat: number; lng: number;
           price_per_night_yuan: number; price_estimated?: boolean; provider?: string };
  nights: number; nightly_yuan: number; total_yuan: number;
  anchor_days?: number[]; include_in_budget: boolean; price_estimated: boolean; source: string;
}

export interface Itinerary { /* 既有字段 */ lodging?: LodgingSummary }

export type ResumeRequest =
  | { action: "accept" | "reject" }
  | { action: "supplement" | "resolve_research"; input?: string; research_choice?: ResearchChoice }
  | { action: "revise"; input: string };
```

- `parseItinerary` 增加 legs / lodging 的宽松解析：过滤非数组、非数字坐标、长度 < 2 的点；非法数据按缺失处理，不抛错。
- `api/trips.ts` 新增 `cancelRun(runID): Promise<void>`。

## 5. 模块设计

### M1 插件状态装配（F1–F3）

**职责：** 让 `wire` 产出的插件状态名与 API/前端约定一致，未启用时给出可读原因。
**对外接口：** `app.WirePlugins(...)` 签名不变；`Wiring.Statuses` 的 `Name` 改为规范名 `hotel` / `guide`。
**依赖：** `config`、`plugins`、app 包内既有 Governor 包装。
**改动：**

- `internal/app/wire.go`：状态名由 `search_hotels` / `search_travel_notes` 改为 `hotel` / `guide`（插件自身 `Name()` 与工具注册名保持不变，仅装配层对外状态改名）。
- `internal/api/router.go:handleMeta`：匹配分支不变（已兼容 `hotel`/`guide`），不重写。
- 回归测试：用 `WirePlugins` 真实输出构造 `Deps.PluginStatuses`，断言 `/api/meta` 中 `hotels.enabled` / `guides.enabled` 与 disabled_reason 正确。

### M2 运行恢复与改稿（F4–F8）

**职责：** 承接 `revise`，在同一个 run 内切换到 adjustment 执行；提案被拒时回退基线并继续等待输入。
**对外接口：** `Runner.Resume` 签名不变；`RunStore` 新增 `GetPlanVersion`、`UpdateRunMode`。
**依赖：** `domain.DraftFromPlan`、`runstore`、`repo`、`events`。
**模块内部：**

- 新文件 `internal/agent/revise.go`：`resumeRevise`、`rejectProposal`，被 `runner.go` 的 `Resume` switch 调用。
- `runner.go`：新增 `ResumeRevise` 常量与分支；`ResumeReject` 改走 `rejectProposal`；`RunStore` 接口扩展。
- `runstore/runstore.go`：`GetPlanVersion` 复用 `repo.GetPlanVersionBySeq` 并反序列化成 `VersionState`；`UpdateRunMode` 调 `repo.Store.UpdateRunMode`。
- `repo/store.go`：手写 `UpdateRunMode`（`s.pool.Exec("UPDATE runs SET mode=$2 WHERE id=$1")`），不引入 sqlc 重新生成；本机未安装 sqlc，避免把生成器变成构建前置条件。
- `api/runs.go`：`resumeRequest` 增加 `revise`；`resumeAction` 校验 input；响应 `status` 改为 `deps.Store.GetRun` 的实际值（读取失败时按 kind 回退）。
- `api/versions.go`：`draftFromPlan` 改为调用 `domain.DraftFromPlan`，删除本地实现。

**设计约束：** `executeAdjustment` 不改——它已从 `TripState` 取 Plan/Draft/Candidates，候选为空时会通过 `adjustmentCandidates` 补检索，因此恢复后的状态可直接复用。

### M3 酒店解析与锚定（F9–F14）

**职责：** 把“用户指定酒店”变成可排期的 Lodging 锚点，优先级：工具结果 → 地理编码 → 提问；锚定天默认覆盖全程。
**对外接口：** `HotelResolver` 接口 + `WithHotelResolver` 选项。
**依赖：** `tools.AMapClient.Geocode`（已有，无调用方）。
**模块内部：**

- 新文件 `internal/agent/hotel_resolver.go`：接口定义 + `resolveSpecifiedHotels`（名称去重、≤2 次查询、结果按城市/名称过滤、与工具结果合并去重）。
- 新文件 `internal/tools/geocode.go`：`HotelGeocoder` 适配器，`city="北京"`，把 `Geocode` 转为 `domain.Hotel`（provider=`amap`、价格 0）。
- `internal/agent/research.go`：无工具但存在指定酒店且有 resolver 时仍执行地理编码路径；工具结束后补齐未命中的指定酒店；`normalizeResearchBrief` 调整价格 0 清理与锚定天计算。
- `internal/domain/constraint.go`：`LodgingConstraint.AnchorDays` + merge 逻辑。
- 解析提示词：明确 0-based `anchor_days` 与“全程入住则不填”。
- `cmd/api/main.go`：存在 AMap key 时注入 `HotelResolver`。
- 排期/展示侧不改：`scheduleOptions`、`present` 已消费 `AccommodationBrief`。

### M4 路线分段几何（F15–F18）

**职责：** 从 AMap 响应提取真实折线并简化；旧数据/失败时保持直线兜底。
**对外接口：** `RouteService.Route` 签名不变，返回的 `Transport.Legs` 填充分段。
**依赖：** `domain.TransitLeg`、scheduler 既有调用点（无需改调用签名）。
**模块内部：**

- `internal/tools/route.go`：响应结构扩展 walking/driving 的 `steps[].polyline`、transit 的 `segments[].walking.steps[]` 与 `bus.buslines[].polyline`；按模式拼装 legs；transit 的 `city`/`cityd` 改为 `"北京"`；路程指标仍取 `paths[0]`/`transits[0]`。
- 新文件 `internal/tools/polyline.go`：解析 `"lng,lat;..."`、Douglas–Peucker 简化（≈15m）、点数截断。
- `internal/domain/itinerary.go`：`TransitLeg`、`Transport.Legs`、`Clone` 深拷贝。
- scheduler：组装 DayPlan 后统计每日 legs 点数，超过 600 时对低优先级段降级为直线（保留首末点），限制逻辑集中且不影响 Route 单测。

### M5 前端体验（F4–F8, F15–F18）

- `RunActions.tsx`：初始方案 awaiting 提供接受、拒绝（本地展开反馈框，不发请求）、取消；反馈框提交 `revise`；提案 awaiting 提供接受调整/拒绝；`proposal_rejected` 展示反馈框。
- `TripMap.tsx`：按 `transport.legs` 逐段绘制；无 legs 画灰色虚线；按 mode 配色；酒店 marker 来自 `itinerary.lodging.hotel`，参与 `setFitView`；图例补充步行/公交/地铁/驾车。
- `features/itinerary/types.ts`：`TransitLeg`、`lodging` 类型 + 宽松解析。
- `features/trace/traceReducer.ts`：`stageLabels` 增加 `proposal_rejected`；`run.started` 按 payload.mode 设置 stage（adjustment → adjust）并清理 proposal。
- `api/trips.ts`：`cancelRun`；`api/types.ts` 的 `ResumeRequest` 增加 revise。

## 6. 模块交互

```
web ──HTTP──► api ──► agent.Runner ──► runstore ──► repo ──► Postgres
                        │  │
                        │  └──► Nodes(research/adjust) ──► HotelResolver ──► tools.AMapClient
                        └──► events.Broker ──► SSE ──► web
scheduler ──► RouteProvider(RouteService) ──► AMap
wire ──► plugins（状态名规范化）
```

无循环依赖：`domain` 不依赖 `agent/tools`；`agent` 通过接口依赖 `tools` 适配器；`runstore` 只做 `repo ↔ agent` 转换。

## 7. 文件组织

```
internal/
├── agent/
│   ├── nodes.go                  修改  新增 WithHotelResolver
│   ├── hotel_resolver.go         新建  HotelResolver + 指定酒店补齐/去重
│   ├── research.go               修改  地理编码兜底、锚定天、价格 warning
│   ├── runner.go                 修改  ResumeRevise、RunStore 接口、reject 分支
│   ├── revise.go                 新建  resumeRevise / rejectProposal
│   ├── research_test.go          修改  工具未命中→解析、全天锚定、价格缺失
│   └── runner_test.go            修改  revise/reject 状态迁移
├── domain/
│   ├── constraint.go             修改  Lodging.AnchorDays + merge
│   ├── itinerary.go              修改  TransitLeg、Transport.Legs、Clone
│   ├── draft.go                  新建  DraftFromPlan
│   ├── constraint_test.go        修改  anchor_days 合并/归一
│   └── draft_test.go             新建  DraftFromPlan
├── tools/
│   ├── amap.go                   不变  （Geocode 已存在）
│   ├── geocode.go                新建  HotelGeocoder
│   ├── route.go                  修改  分段解析、city=北京、legs 装配
│   ├── polyline.go               新建  解析 + Douglas–Peucker + 截断
│   ├── route_test.go             修改  walking/transit 几何夹具、city 参数
│   ├── polyline_test.go          新建  简化/截断/非法点
│   └── geocode_test.go           新建  城市过滤、坐标解析
├── runstore/runstore.go          修改  GetPlanVersion、UpdateRunMode
├── repo/store.go                 修改  手写 UpdateRunMode
├── app/wire.go                   修改  状态名 hotel/guide
├── app/wire_test.go              修改  规范名
├── api/
│   ├── runs.go                   修改  revise 校验、真实状态返回
│   ├── runs_test.go              修改  revise/reject 响应与状态
│   ├── versions.go               修改  改用 domain.DraftFromPlan
│   ├── meta_test.go              修改  真实 wiring 的状态回归
│   └── router.go                 不变  （已兼容规范名）
cmd/api/main.go                   修改  注入 HotelResolver
web/src/
├── api/types.ts                  修改  ResumeRequest/TransitLeg/Lodging
├── api/trips.ts                  修改  cancelRun
├── api/trips.test.ts             修改  revise/cancel 请求
├── api/openapi.yaml              修改  revise、legs、anchor_days
├── features/itinerary/types.ts   修改  解析 legs/lodging
├── features/itinerary/types.test.ts 新建 宽松解析
├── features/map/TripMap.tsx      修改  逐段绘制、酒店 marker、图例
├── features/map/TripMap.test.tsx 修改  legs/兜底/酒店断言
├── features/trace/traceReducer.ts 修改 stage label、run.started 模式
├── features/trace/traceReducer.test.ts 修改 对应断言
├── features/workspace/RunActions.tsx 修改 反馈框/取消/提案拒绝
└── features/workspace/RunActions.test.tsx 修改 交互断言
docs/环境变量配置.md              修改  插件状态说明与启用后自检
```

## 8. 实施顺序

```
M1（独立）──┐
M3（domain.AnchorDays）──► M3 research ──┐
M4（domain.TransitLeg）──► route ────────┤
M2（RunStore + revise）──────────────────┼──► M5 前端
                                         └──► 全量测试/联调
```

M1 可先行并单独验证；M2 与 M3/M4 无代码依赖，可并行；M5 依赖 M2 的接口与 M4 的数据形状，放在后段。

## 9. 技术决策

| # | 决策点 | 选择 | 理由 | 被否方案 |
|---|---|---|---|---|
| D1 | 拒绝后的执行载体 | 同 run 转 `mode=adjustment` | SSE/run ID/版本基线连续；Recover、并发信号量、取消路径全部复用 | 新建 adjustment run：前端要重建订阅，旧 run 需人为置 cancelled，语义混乱 |
| D2 | 提案拒绝的基线恢复 | 按 `PreviousSeq` 从 `plan_versions` 回读 | 版本表是事实源，不在 `run.state` 复制大对象 | TripState 增 Base*：状态膨胀、易与版本不一致 |
| D3 | resume 响应状态 | 读取 `runs.status` 实际值 | reject 现在是“恢复后等待”而不再是终态，静态映射必错 | 给 `resumedStatus` 加分支：仍无法区分“有提案/无提案” |
| D4 | `runs.mode` 更新 | 手写 SQL `UPDATE` | 本机/CI 无 sqlc；避免把代码生成器变成构建前置 | 改 sqlc 查询并重生成：引入工具链依赖 |
| D5 | 插件状态命名 | 装配层输出 `hotel` / `guide` | 与 API 契约、前端既有分支一致；插件内部工具名不动 | 改 router 映射兼容两种名字：掩盖问题 |
| D6 | 酒店解析优先级 | 工具 → 地理编码（≤2 次）→ 提问 | 工具给价格与预订链接；解析器保证“用户明确说了就能用” | 只靠 LLM 研究：已证实会漏；只靠地理编码：丢价格 |
| D7 | 锚定天默认值 | 全部天（有酒店时） | 用户说“住这家”通常意味着每天从酒店出发/返回 | 仅第一天：用户已反馈起点不是酒店 |
| D8 | 价格缺失处理 | 保留酒店 + warning + 不计预算 | 锚定价值大于预算精度 | 价格 0 视为无效：本次 bug 的成因之一 |
| D9 | transit 城市参数 | `"beijing"` → `"北京"` | 集成接口对城市名更可靠；当前值可能导致无路线→直线 | 保持原值：无法解释/修复“一直是直线” |
| D10 | 几何来源 | 后端解析 AMap steps/segments，存 `legs` | 与锁定的路线一致、可复现；前端只负责画 | 前端二次请求 AMap：密钥暴露、口径分叉 |
| D11 | 折线简化 | Douglas–Peucker ≈15m，保留首末点 | 视觉几乎无损，快照体积可控 | 全量点：快照膨胀；只留端点：等同直线 |
| D12 | 点数上限 | ≤60 点/段、≤600 点/天 | 满足 N3，map 渲染稳定 | 仅总量限制：单段仍可能过大 |
| D13 | 旧快照 | `legs=nil` → 灰色虚线 | 零迁移、零重算 | 启动时批量回填：需重调 AMap、成本高 |
| D14 | 数据库 | 无 DDL | 新字段全在既有 JSON 列；`mode` 列已存在 | 新表/新列：无必要迁移风险 |
| D15 | 攻略来源 | 保持 DeepSeek WebSearch | 已在 v1.1 验证；小红书缺失是已知上游限制 | 新增搜索供应商：超出本次范围 |

## 10. 测试策略

### 10.1 Go 单元/集成（`make test` = `go test -race ./...`）

| 层 | 用例 |
|---|---|
| 装配 | `wire_test`：状态名恰为 `hotel`/`guide`；`meta_test`：用真实 wiring 状态请求 `/api/meta`，断言 enabled/reason（F1–F3 守卫） |
| 领域 | `constraint_test`：AnchorDays 合并、nil 不覆盖、越界归一；`draft_test`：`DraftFromPlan` 保持闭集 POI/时长；`itinerary_test`：Clone 深拷贝 legs |
| 工具 | `route_test`：walking steps 折线、transit segments（步行+公交+步行）、city=北京、无 polyline 时 legs 为空；`polyline_test`：非法点、DP 端点保留、点数上限；`geocode_test`：城市过滤、坐标解析、provider=amap |
| 研究 | `research_test`：无工具+有 resolver 仍解析；工具未命中→resolver；多候选→提问；零价格不清空+warning；锚定天三种优先级 |
| 运行 | `runner_test`：revise 后 mode/status/stage/Input 正确且同 run；提案 reject 恢复基线且广播 `run.awaiting_user` 而非 cancelled；legacy reject（无提案）仍取消；cancel 不变 |
| API | `runs_test`：revise 空输入/超长 → 400；revise 响应 queued；reject 响应 awaiting_user（提案场景）；旧状态兼容 |

### 10.2 前端（`cd web && npm test`）

- `RunActions`：点“拒绝”不发请求、仅展开反馈；提交调用 `revise` 且携带输入；`proposal_rejected` 展示反馈框；取消调用 cancel。
- `TripMap`：给定 legs 按 mode 生成不同样式元素；legs 缺失时画虚线；酒店 marker 出现且参与视野。
- `itinerary/types`：legs/lodging 非法数据被过滤，不抛错。

### 10.3 端到端（浏览器实跑，checklist 阶段逐条取证）

- 插件：开启 hotel/guide 后 `/api/meta` 为 enabled；关闭时前端显示原因（F1–F3）。
- 改稿：初始方案拒绝 → 输入“第二天轻松点” → 同 run 产出提案 → 接受 → 新版本 seq+1；提案拒绝 → 回到待修改且无 cancelled 事件（F4–F8）。
- 酒店：仅启用 AMap、关闭 FlyAI，提示词写“我住 XX 酒店” → 行程每天首尾为酒店、地图有酒店标记、预算有 warning（F9–F14）。
- 地图：有 key 时路线按步行/公交/地铁分段着色；断网/无 key 时虚线兜底（F15–F18）。
- 回归：打开升级前生成的旧行程，地图仍可显示（虚线），时间轴/预算不变。

### 10.4 评测基线保护

- 运行 `make eval`，与 v1 20 例、v1.1 插件基线对比：通过率不降；时长 p95 增幅 ≤10%（N2）。
- 几何与地理编码不参与 LLM 决策，评测输出差异应仅限 `legs`/lodging 字段。

### 10.5 构建前置与密钥（不新增供应商）

| 能力 | 必需配置 | 无配置时的行为 |
|---|---|---|
| 酒店工具（飞猪 FlyAI） | `HOTEL_PLUGIN_ENABLED=true` + `HOTEL_TOOL_TOKEN` + `FLYAI_API_KEY`（sidecar） | 状态未启用并给出原因 |
| 酒店地理编码 | `AMAP_WEB_KEY` | 无解析器，指定酒店走提问 |
| 攻略 | `GUIDE_PLUGIN_ENABLED=true` + `GUIDE_SEARCH_API_KEY`（DeepSeek） | 状态未启用并给出原因 |
| 地图渲染/路线 | `AMAP_WEB_KEY`（后端）、`VITE_AMAP_JS_KEY`（前端） | 直线兜底/地图占位 |

密钥只放 `deploy/.env`、`web/.env.local`，不写入代码、日志或测试夹具；本计划不打印任何密钥值。

## 11. 风险与回滚

| 风险 | 影响 | 缓解 | 回滚 |
|---|---|---|---|
| transit 城市参数变化导致时长/费用口径变化 | 评测预算轻微漂移 | 夹具对比 + `make eval` 基线；仅改城市参数不改取数路径 | 单行回退为原参数 |
| 地理编码误匹配 | 锚到错误酒店 | 城市过滤 + 名称匹配 + 多候选提问；单次最多 2 次查询 | 关闭 `AMAP_WEB_KEY` 即退回“提问”路径 |
| 提案拒绝时版本读取失败 | 无法恢复基线 | 返回 500、状态保持 awaiting_user、不落半成品；UI 提示重试/取消 | 用户可显式取消；无数据破坏 |
| 同 run 重入破坏 SSE 序号 | 前端丢事件 | 复用现有事件写入与 `Last-Event-ID`；`run.started` payload 已带 mode/input | 前端按 run ID 重建订阅的既有逻辑兜底 |
| 折线过大 | 快照/SSE 膨胀 | DP + 双上限；超限段降级直线 | 调小上限或关闭 legs（常量） |
| 旧客户端仍发 reject | 行为变化 | 有提案→恢复等待（更安全）；无提案→仍取消 | 无需回滚 |
| 状态名变更影响其他消费者 | 未知依赖 | 仓库内仅 meta 消费；补真实回归测试 | 保留别名映射一天即可回退 |

回滚总策略：全部改动为增量字段与新增分支，无 DDL。回滚 = revert 对应提交；旧快照、旧版本、旧 run 状态均可被旧代码读取。数据库无需执行 down migration。

## 12. Spec 覆盖自检

| Spec | 落点 |
|---|---|
| F1–F3 | M1 + 真实 wiring 的 meta 回归测试 |
| F4–F8 | M2（revise/恢复/取消）+ M5 交互 |
| F9–F14 | M3 + D6–D8、D14 |
| F15–F18 | M4 + M5 + D9–D13 |
| N1（兼容旧数据） | D13 + 全部 `omitempty` |
| N2（p95 ≤10%） | 几何/解析均为确定性本地计算；评测对比 |
| N3（点数/体积） | D11、D12 |
| N9（无新表） | D14 |
| AC1–AC20 | 由测试策略 + checklist 覆盖 |

无未归属的 spec 条目；无与 spec 冲突的决策。
