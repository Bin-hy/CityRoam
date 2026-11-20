# TripPilot v1.4 天气工具 Plan

状态：待审批（plan）
日期：2026-11-20
对应 spec：`docs/mew-spec/v1.4-weather-tool/spec.md`

## 架构概览

本期在现有模块边界内完成四件事，不新增进程、存储或供应商：

1. **天气工具**：在 `internal/plugins` 新增研究工具，包装现有高德天气与地理编码能力，实现工具契约、缓存键和来源标识；由 `internal/app/wire.go` 在密钥存在时注册，并交给 governor 统一治理。
2. **Governor 语义修正**：把缓存查询前移到额度预留之前，额度只统计真实上游请求；同一缓存键的并发未命中通过进程内合并只放行一次上游调用；事件区分真实调用、缓存命中与合并命中。
3. **研究链路**：研究循环增加运行内结果复用；天气结果由代码确定性写入研究结论，不经过模型转述；天气失败只产生警告，不把研究标为降级。
4. **确定性校验**：领域层新增 C13「雨天户外占比」检查；agent 校验节点把预报传入并与现有 C1–C12 结果合并，校验器接口签名保持不变。

职责边界：

- `internal/tools` 只管高德 HTTP 调用，不感知 agent 或研究协议。
- `internal/plugins` 负责工具契约、缓存键、错误归类，以及 governor 的额度与并发语义。
- `internal/agent` 负责研究循环、运行内复用、结论收口与校验合并。
- `internal/domain` 保持无 I/O；天气结构、C13 规则都是纯函数。

## 核心数据结构

### domain.WeatherDay

```go
type WeatherDay struct {
    Date         string `json:"date"`          // YYYY-MM-DD
    DayWeather   string `json:"day_weather"`
    NightWeather string `json:"night_weather"`
    DayTemp      string `json:"day_temp"`
    NightTemp    string `json:"night_temp"`
}
```

- 与工具输出 JSON 字段一致，避免二次映射。
- 提供纯函数 `IsRainy(weather string) bool`：包含「雨」「雪」或 `rain`/`snow` 视为降水天气。

### domain.ResearchBrief（修改）

```go
type ResearchBrief struct {
    Status        ResearchStatus      `json:"status"`
    Accommodation *AccommodationBrief `json:"accommodation,omitempty"`
    Guides        []GuideSignal       `json:"guides,omitempty"`
    Weather       []WeatherDay        `json:"weather,omitempty"`
    Notes         []string            `json:"notes,omitempty"`
    Warnings      []string            `json:"warnings,omitempty"`
    ToolCalls     int                 `json:"tool_calls"`
    DurationMS    int64               `json:"duration_ms"`
}
```

- `Weather` 只由 `normalizeResearchBrief` 用工具结果覆盖写入。
- `ResearchBriefSchema` 不新增该字段，模型输出无法携带天气。

### plugins.WeatherInput / WeatherOutput

```go
type WeatherInput struct {
    City      string `json:"city"`
    StartDate string `json:"start_date"` // YYYY-MM-DD
    Days      int    `json:"days"`       // 1..7
}

type WeatherOutput struct {
    Provider  string             `json:"provider"` // "amap"
    City      string             `json:"city"`
    Days      []domain.WeatherDay `json:"days"`
    FetchedAt time.Time          `json:"fetched_at"`
}
```

### 工具契约

`WeatherTool` 实现：

```go
type ResearchTool interface {
    Name() string
    Description() string
    InputSchema() json.RawMessage
    Call(ctx context.Context, input json.RawMessage) (json.RawMessage, error)
}
```

额外实现：

- `Provider() string` → `"amap"`
- `CacheKey(input) (string, error)` → `weather:v1:{city}:{start_date}:{days}`
- `CacheTTL() time.Duration` → 可配置，默认 30 分钟
- `ResultCount(output) int` → 返回天数

### 天气数据源接口（测试缝）

```go
type weatherProvider interface {
    Weather(ctx context.Context, city string) ([]tools.WeatherDay, error)
    Geocode(ctx context.Context, address, city string) ([]tools.Geocode, error)
}
```

生产实现为 `*tools.AMapClient`；测试使用假实现，不访问外网。

### Governor 内部状态

```go
type runCounters struct {
    total    int            // 真实上游调用次数（额度）
    observed int            // 事件序号，命中与真实调用都递增
    byTool   map[string]int // 单工具真实调用次数
}

type flight struct {
    done chan struct{}
    out  json.RawMessage
    code string
    msg  string
}
```

- `flights map[string]*flight` 以缓存键为粒度，执行完成后删除。
- 组内仅 leader 执行上游并在执行前做额度检查；跟随者等待并复用结果，不占额度。
- 上游执行使用独立超时上下文，受工具超时与研究总时限共同约束。
- 无缓存键的工具不参与合并。

### 校验入口（修改）

```go
type ValidateInput struct {
    RunID       string
    Constraints domain.ConstraintSet
    Plan        domain.Itinerary
    POIs        map[string]domain.POI
    Research    *domain.ResearchBrief // 新增
}
```

新增纯函数，不修改 `validator.Validate` 签名：

```go
func CheckRain(plan domain.Itinerary, pois map[string]domain.POI, forecast []domain.WeatherDay) []domain.Violation
```

## 模块设计

### 天气工具（`internal/plugins/weather.go`）

**职责：** 输入校验、城市标准化、调用高德、整形输出、提供缓存键。

**对外接口：** `NewWeatherTool(provider weatherProvider, opts ...WeatherOption)`；工具四方法 + `Provider`、`CacheKey`、`CacheTTL`、`ResultCount`。

**行为：**

1. 解析并校验输入：城市非空；`start_date` 为 `YYYY-MM-DD`；`days` 在 1–7。
2. 城市标准化：含中文字符直接使用；纯 ASCII 时调用一次 `Geocode`，取首条结果的 `City`；解析失败返回 `ToolError{Code: invalid_input}`。
3. 调用 `Weather`，按 `start_date`/`days` 过滤窗口内日期；结果为空时返回空数组而非错误。
4. 上游错误映射为 `ToolError{Code: upstream_unavailable, Retryable: true}`；超时由 governor 归类。
5. 输出 `WeatherOutput`，`FetchedAt` 使用工具时钟。

**依赖：** `internal/tools`（通过 `weatherProvider` 接口）、`internal/domain`、标准库。

**配置：** `WithWeatherCacheTTL`、`WithWeatherClock`、`WithWeatherHTTPTimeout`（传给高德客户端）。

### Governor（`internal/plugins/governor.go`）

**职责：** 额度、缓存、并发合并、事件与持久化。

**新调用顺序：**

1. 取得 run 上下文，计算缓存键（若工具实现 `CacheKeyer`）。
2. 读取缓存；命中则分配观察序号、记录事件（`cached=true`）、写证据并返回；**不占额度**。
3. 若同键已有进行中的合并项，等待其完成；拿到结果后按合并命中记录（`coalesced=true`），**不占额度**。
4. 否则创建同键合并项：leader 执行额度检查；超限则把额度错误作为合并结果返回给所有等待者。
5. 在独立超时上下文中执行上游（`context.WithoutCancel` + 工具超时，受研究总时限约束）。
6. 成功后写缓存、记录事件、写证据、关闭合并项；失败同样关闭合并项，等待者得到同一错误。

**语义：**

- `total`、`byTool` 只统计真实上游执行。
- `observed` 为每次调用分配递增序号，用于事件与调试。
- 缓存/合并命中仍写 `tool_calls` 持久化，但标记为命中，不参与额度。

**兼容：** 无 `CacheKeyer` 的工具保持原有行为，只是额度检查时序不变（没有缓存可查）。

### 研究节点（`internal/agent/research.go`）

**会话新增字段：**

```go
type researchSession struct {
    // 既有字段...
    weather       []domain.WeatherDay
    weatherStatus string            // ok | empty | failed | timeout | unavailable | skipped
    toolResults   map[string]string // 运行内复用：工具名 + 参数 → 外部数据文本
}
```

**行为：**

1. `executeResearchTool` 先查 `toolResults`；命中直接返回既有文本，不执行、不计数、不产生错误。
2. 未命中时调用工具，成功后按"工具名 + `\x00` + 参数原文"写入复用表。
3. 天气工具成功：解析 `days`，按日期去重写入 `session.weather`；`weatherStatus=ok`（无数据为 `empty`）。
4. 天气工具失败：设置 `weatherStatus`（根据错误码区分 timeout/unavailable/invalid），追加警告「天气不可用（原因）」，**不加入** `toolErrors`。
5. 研究收口：`brief.Weather = session.weather`；无数据时置空。`brief.ToolCalls` 仍为真实执行次数。
6. `research.completed` 事件增加 `weather_status` 与 `weather_days`。

**边界：** 复用表生命周期等于一次 `Research` 调用，天然有界；跨研究复用交给 TTL 缓存。

### 雨天校验（`internal/domain/validator/rain.go`）

**规则 C13：**

- 仅在有预报的日期上判断；按 `plan.Days[].Date` 匹配。
- 当天白天或夜间天气含雨/雪，且该天 POI 块中 `Outdoor=true` 占比 > 60% → `LevelBlocking`。
- 消息包含日期、户外点位数量与占比；建议替换为室内点位。
- 总点数为 0 或没有匹配日期时跳过。
- 现有 C11 保持不变，二者可同时出现。

**接入：** `agent.Validate` 先跑既有校验，再在 `Research.Weather` 非空时追加 `CheckRain` 结果；`afterValidate` 逻辑不变。

### 配置与装配（`internal/config`、`internal/app/wire.go`）

新增配置项：

| 环境变量 | 默认值 | 说明 |
|---|---|---|
| `WEATHER_CACHE_TTL` | `30m` | 天气结果缓存有效期 |
| `WEATHER_TOOL_TIMEOUT` | `3s` | 单次天气上游调用超时 |
| `WEATHER_MAX_CALLS_PER_RUN` | `1` | 单次运行真实上游调用上限 |

装配：

- `wireWeather(cfg)`：`AMAP_WEB_KEY` 为空时返回禁用状态，原因 `AMAP_WEB_KEY is empty`；否则构造高德客户端与工具。
- 状态名 `weather`，工具名 `get_weather`，来源 `amap`。
- governor 增加 `WithGovernorToolTimeout("get_weather", ...)` 与 `WithGovernorToolLimit("get_weather", ...)`。
- 禁用的天气插件写入 `Wiring.Disabled`，研究提示词显示为禁用插件。

### 元数据（`internal/api/router.go`）

- `/api/meta` 的 `plugins` 增加 `weather` 段，沿用 `enabled` + `disabled_reason` 结构。
- 现有 `hotels`、`guides` 字段不变，属于向后兼容的新增。

## 模块交互

```
研究循环
  LLM 调用 get_weather
    → agent 运行内复用（命中直接返回）
    → governor：读缓存（命中：cached，不占额度）
    → 同键合并（等待者复用，不占额度）
    → 额度检查（仅真实调用计数）
    → WeatherTool：城市标准化（必要时地理编码）→ 高德天气
    → 写缓存（默认 30m）→ 返回
  → 按日期去重写入研究会话
  → 研究收口覆盖写入 brief.weather
  → 草案提示词可见逐日天气

草案 → 排期
校验
  → 既有 C1–C12
  → 有预报时追加 C13（雨天户外占比）
  → 有 blocking → 修复（换室内）
  → 无 blocking → 出方案
```

replan 与调整复用同一缓存；同一次研究内的重复调用由运行内复用挡住，跨研究由 TTL 缓存挡住。

## 文件组织

```
internal/
├── domain/
│   ├── weather.go                    新建：WeatherDay、IsRainy
│   ├── research.go                   修改：ResearchBrief.Weather
│   └── validator/
│       ├── rain.go                   新建：CheckRain（C13）
│       └── rain_test.go              新建
├── plugins/
│   ├── weather.go                    新建：WeatherTool、输入输出、缓存键
│   ├── weather_test.go               新建
│   ├── governor.go                   修改：缓存先于额度、合并、观察计数、事件标记
│   └── governor_test.go              修改
├── agent/
│   ├── research.go                   修改：运行内复用、天气写入、警告、完成事件
│   ├── research_weather_test.go      新建
│   ├── schedule.go                   修改：ValidateInput.Research、合并 C13
│   ├── graph.go                      修改：把研究结论传给校验
│   └── prompts/draft.tmpl            修改：雨天优先室内、无预报不猜测
├── config/
│   ├── config.go                     修改：三个天气配置项
│   └── config_test.go                修改
├── app/
│   ├── wire.go                       修改：注册天气插件与 governor 参数
│   └── wire_test.go                  修改：状态数从 2 变 3
└── api/
    ├── router.go                     修改：/api/meta 增加 weather 段
    └── meta_test.go                  修改
docs/mew-spec/v1.4-weather-tool/
├── spec.md
├── plan.md
├── task.md
└── checklist.md
```

## 技术决策

| 决策点 | 选择 | 理由 |
|---|---|---|
| 天气形态 | 研究工具，而非固定注入 | 保留模型按需调用与统一治理；确定性回写弥补可选性 |
| 缓存粒度 | 城市 + 起始日 + 天数 | 全局 5 分钟会把不同城市的结果串用 |
| 缓存有效期 | 默认 30 分钟 | 高德每天更新约 3 次，5 分钟命中率过低 |
| 额度语义 | 只统计真实上游请求 | 否则「缓存减少调用」不成立；额度检查放在缓存与合并之后 |
| 并发合并 | 进程内一次性结果表 | 单实例部署足够；不引入 Redis 或分布式锁 |
| 运行内复用 | agent 会话内按「工具 + 参数」缓存 | 生命周期短、天然有界；跨研究交给 TTL 缓存 |
| 合并与取消 | 上游用独立超时上下文，等待者用自身上下文 | 一个等待者取消不应影响其他调用 |
| 天气失败 | 不降级研究，记警告与天气状态 | 天气是可选增强，失败不应污染研究结论状态 |
| C13 接入方式 | agent 合并独立检查函数 | 不扩展现有校验接口签名，避免全量测试改动 |
| 城市标准化 | ASCII 输入先地理编码 | 复用既有能力，不维护硬编码别名表 |
| 预报写入 | 代码覆盖，模型 schema 不含天气 | 从契约层杜绝模型编造天气 |
| 存储 | 复用 JSON 列，无 DDL | 与既有「不加表」的约束一致 |
| 元数据 | `/api/meta` 新增 `weather` 段 | 满足状态可见，且向后兼容 |
| 依赖 | 自行维护一次性结果表，不新增依赖 | 需要区分 leader 与跟随者以准确记录额度与事件 |

## 需求覆盖

| Spec 需求 | 落地位置 |
|---|---|
| F1 天气查询工具 | `plugins/weather.go` |
| F2 工具额度 | `plugins/governor.go`、`app/wire.go` |
| F3 结果缓存 | `plugins/weather.go`（键与 TTL）、`governor.go`（读写） |
| F4 缓存不占额度 | `governor.go` 调用顺序调整 |
| F5 运行内去重 | `agent/research.go`（复用表） |
| F6 研究结论携带天气 | `domain/research.go`、`agent/research.go` |
| F7 雨天校验 | `domain/validator/rain.go`、`agent/schedule.go` |
| F8 开关与状态 | `app/wire.go`、`api/router.go` |
| F9 可观测性 | `governor.go` 事件标记、`agent/research.go` 完成事件 |
