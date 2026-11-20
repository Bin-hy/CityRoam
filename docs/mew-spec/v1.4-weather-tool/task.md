# TripPilot v1.4 天气工具 Tasks

状态：待审批（task）
日期：2026-11-20
对应 plan：`docs/mew-spec/v1.4-weather-tool/plan.md`

## 文件清单

| 操作 | 文件 | 职责 |
|---|---|---|
| 新建 | `internal/domain/weather.go` | WeatherDay、IsRainy |
| 新建 | `internal/domain/weather_test.go` | 天气判定单测 |
| 修改 | `internal/domain/research.go` | ResearchBrief.Weather |
| 新建 | `internal/domain/validator/rain.go` | C13 雨天户外占比 |
| 新建 | `internal/domain/validator/rain_test.go` | C13 单测 |
| 修改 | `internal/config/config.go` | 三个天气配置项 |
| 修改 | `internal/config/config_test.go` | 配置单测 |
| 新建 | `internal/plugins/weather.go` | WeatherTool 与输入输出 |
| 新建 | `internal/plugins/weather_test.go` | 工具单测 |
| 修改 | `internal/plugins/governor.go` | 额度时序、合并、观察计数 |
| 修改 | `internal/plugins/governor_test.go` | 额度与合并单测 |
| 修改 | `internal/app/wire.go` | 注册天气插件 |
| 修改 | `internal/app/wire_test.go` | 状态数量与天气状态 |
| 修改 | `internal/api/router.go` | `/api/meta` 增加 weather |
| 修改 | `internal/api/meta_test.go` | 元数据单测 |
| 修改 | `internal/agent/research.go` | 运行内复用、天气写入、警告、事件 |
| 新建 | `internal/agent/research_weather_test.go` | 研究天气单测 |
| 修改 | `internal/agent/schedule.go` | 校验输入与 C13 合并 |
| 修改 | `internal/agent/graph.go` | 传递研究结论到校验 |
| 修改 | `internal/agent/prompts/draft.tmpl` | 天气使用规则 |

## T1: 领域天气结构与雨天判定

**文件：** `internal/domain/weather.go`、`internal/domain/weather_test.go`
**依赖：** 无

**步骤：**
1. 定义 `WeatherDay`，字段与工具 JSON 一致：`date`、`day_weather`、`night_weather`、`day_temp`、`night_temp`。
2. 实现 `IsRainy(weather string) bool`：转小写后包含 `雨`、`雪`、`rain` 或 `snow` 即为真。
3. 表驱动测试：晴、多云、阴、小雨、雷阵雨、雨夹雪、小雪、Rain、Snow、空字符串。

**验证：** `go test ./internal/domain -run TestIsRainy` 通过。

## T2: 研究结论增加天气字段

**文件：** `internal/domain/research.go`
**依赖：** T1

**步骤：**
1. 在 `ResearchBrief` 的 `Guides` 之后增加 `Weather []WeatherDay`，JSON 标签 `weather,omitempty`。
2. 确认现有构造点无需初始化该字段（零值为 nil）。

**验证：** `go build ./internal/domain` 通过。

## T3: C13 雨天户外占比检查

**文件：** `internal/domain/validator/rain.go`、`internal/domain/validator/rain_test.go`
**依赖：** T1

**步骤：**
1. 实现 `CheckRain(plan domain.Itinerary, pois map[string]domain.POI, forecast []domain.WeatherDay) []domain.Violation`。
2. 建立「日期 → 是否降水」映射；没有预报的日期跳过。
3. 对每个降水日，统计 `BlockTypePOI` 且能在 `pois` 中找到的块；总数为 0 时跳过。
4. 户外占比超过 0.6 时追加 `Code: "C13"`、`Level: LevelBlocking`、`Day: 日期序号`，消息包含日期与占比，建议换成室内点位。
5. 测试：雨天 3/4 户外 → blocking；雨天 1/3 户外 → 无；晴天 → 无；无预报 → 无；未知 POI 不计入。

**验证：** `go test ./internal/domain/validator -run TestCheckRain` 通过。

## T4: 天气配置项

**文件：** `internal/config/config.go`、`internal/config/config_test.go`
**依赖：** 无

**步骤：**
1. 增加字段：`WeatherCacheTTL`、`WeatherToolTimeout`、`WeatherMaxCallsPerRun`。
2. 解析 `WEATHER_CACHE_TTL`（默认 `30m`）、`WEATHER_TOOL_TIMEOUT`（默认 `3s`）、`WEATHER_MAX_CALLS_PER_RUN`（默认 `1`，范围 1–10）。
3. 非法值沿用现有错误返回方式。

**验证：** `go test ./internal/config` 通过，覆盖默认值与非法值。

## T5: 天气工具骨架与缓存键

**文件：** `internal/plugins/weather.go`、`internal/plugins/weather_test.go`
**依赖：** T1、T4

**步骤：**
1. 定义 `WeatherInput`、`WeatherOutput`、输入 schema（city、start_date、days 必填，days 1–7）。
2. 定义 `weatherProvider` 接口：`Weather` 与 `Geocode`。
3. 实现 `Name() "get_weather"`、`Description()`、`Provider() "amap"`、`InputSchema()`。
4. 实现 `CacheKey`：`weather:v1:{标准化城市}:{start_date}:{days}`，城市去空格转小写。
5. 实现 `CacheTTL`（默认 30 分钟，可配置）与 `ResultCount`（返回天数）。
6. 测试：输入校验、缓存键稳定性、不同城市键不同。

**验证：** `go test ./internal/plugins -run TestWeatherTool` 通过（不访问外网）。

## T6: 天气工具调用与城市标准化

**文件：** `internal/plugins/weather.go`、`internal/plugins/weather_test.go`
**依赖：** T5

**步骤：**
1. `Call` 先校验输入，非法返回 `ToolError{Code: invalid_input}`。
2. 城市为纯 ASCII 时调用 `Geocode`，取首条结果的 `City`；为空则用 `Province`；仍为空返回 `invalid_input`。
3. 调用 `Weather`，按 `start_date` 与 `days` 过滤日期；无数据返回空数组。
4. 上游错误映射为 `ToolError{Code: upstream_unavailable, Retryable: true}`。
5. 输出填入 `provider`、标准化城市、`days`、`fetched_at`。
6. 测试：中文城市直接查询；ASCII 城市触发一次地理编码；窗口过滤；上游错误映射。

**验证：** `go test ./internal/plugins -run TestWeatherTool` 通过。

## T7: Governor 观察计数与额度拆分

**文件：** `internal/plugins/governor.go`、`internal/plugins/governor_test.go`
**依赖：** 无

**步骤：**
1. `runCounters` 增加 `observed`。
2. 新增 `observe(runID, tool) int`：只递增事件序号，不检查额度。
3. `reserve` 改为只做额度检查与计数，不再返回序号。
4. 调整调用点：事件序号一律来自 `observe`。
5. 更新既有测试中对序号的断言。

**验证：** `go test ./internal/plugins -run TestGovernor` 通过。

## T8: 缓存先于额度

**文件：** `internal/plugins/governor.go`、`internal/plugins/governor_test.go`
**依赖：** T7

**步骤：**
1. 计算缓存键并读取缓存，命中时分配观察序号、记录 `cached=true` 事件、写证据并返回。
2. 缓存命中路径不调用 `reserve`。
3. 缓存未命中路径保持原有 reserve → 执行 → 写缓存顺序。
4. 测试：`maxCalls=1` 下同一请求第二次命中缓存仍成功；额度计数不变；事件带 `cached`。

**验证：** `go test ./internal/plugins -run 'TestGovernorCache|TestGovernorQuota'` 通过。

## T9: 同键并发合并

**文件：** `internal/plugins/governor.go`、`internal/plugins/governor_test.go`
**依赖：** T8

**步骤：**
1. Governor 增加 `flightMu` 与 `flights map[string]*flight`；`flight` 保存 `done`、`out`、`code`、`msg`。
2. 缓存未命中后：加锁查看同键合并项；存在则作为跟随者等待 `done` 或自身上下文取消。
3. 不存在则创建合并项并作为 leader：执行额度检查；超限时把错误码与消息写入合并项并唤醒等待者。
4. leader 使用独立超时上下文执行上游（受工具超时与研究剩余时限约束），完成后写缓存并唤醒等待者，最后删除合并项。
5. 每个调用者记录自己的事件：leader 为普通调用，跟随者为 `coalesced=true`。
6. 测试：10 个并发同键请求只触发 1 次上游；额度只记 1；结果一致；跟随者取消时返回超时信封且不影响 leader。

**验证：** `go test -race ./internal/plugins -run TestGovernorCoalesce` 通过。

## T10: 装配天气插件

**文件：** `internal/app/wire.go`、`internal/app/wire_test.go`
**依赖：** T4、T5、T6、T9

**步骤：**
1. `WirePlugins` 追加 `wireWeather(cfg)`，放在酒店与攻略之后，保持既有状态顺序。
2. 密钥为空：返回禁用状态，原因 `AMAP_WEB_KEY is empty`，无工具；写入 `Disabled`。
3. 密钥存在：用 `tools.NewAMapClient(cfg.AMapWebKey, tools.WithAMapHTTPClient(...))` 构造客户端，传入 `plugins.NewWeatherTool` 与缓存 TTL。
4. governor 增加 `WithGovernorToolTimeout("get_weather", ...)` 与 `WithGovernorToolLimit("get_weather", ...)`。
5. 更新测试的状态数量（2 → 3）并断言天气启用/禁用原因。

**验证：** `go test ./internal/app` 通过。

## T11: 元数据暴露天气状态

**文件：** `internal/api/router.go`、`internal/api/meta_test.go`
**依赖：** T10

**步骤：**
1. `meta.Plugins` 增加 `Weather pluginMeta`，JSON 字段 `weather`。
2. 状态遍历增加 `case "weather"`。
3. 测试：启用时无误导原因；禁用时原因可见；不影响 hotels/guides。

**验证：** `go test ./internal/api -run TestMeta` 通过。

## T12: 研究循环的复用与天气记录

**文件：** `internal/agent/research.go`、`internal/agent/research_weather_test.go`
**依赖：** T2、T5、T6

**步骤：**
1. `researchSession` 增加 `weather []domain.WeatherDay`、`weatherStatus string`、`toolResults map[string]string`。
2. 将 `session.toolCalls++` 从研究循环移入 `executeResearchTool`，放在复用判断之后。
3. 复用键为「工具名 + `\x00` + 参数原文」；命中直接返回既有外部数据文本，不执行、不计数。
4. 成功调用后写入复用表；天气工具额外解析 `days` 并按日期去重写入 `weather`。
5. 天气失败：根据错误码设置 `weatherStatus`（`timeout`/`unavailable`/`invalid`），追加警告，不加入 `toolErrors`。
6. `research.completed` 事件增加 `weather_status` 与 `weather_days`。
7. 测试：重复调用只执行一次；天气成功写入会话；失败只产生警告且研究不降级。

**验证：** `go test ./internal/agent -run TestResearchWeather` 通过。

## T13: 研究结论覆盖天气

**文件：** `internal/agent/research.go`、`internal/agent/research_weather_test.go`
**依赖：** T12

**步骤：**
1. 在研究收口阶段（`normalizeResearchBrief` 调用前后均可）用会话天气覆盖 `brief.Weather`。
2. 会话天气为空时置 `nil`，即使模型输出里带了天气字段也丢弃。
3. 测试：模型返回伪造天气 → 结论使用工具结果；工具无结果 → 结论无天气。

**验证：** `go test ./internal/agent -run TestResearchWeather` 通过。

## T14: 校验节点合并 C13

**文件：** `internal/agent/schedule.go`、`internal/agent/graph.go`、`internal/agent/schedule_test.go`（如需）
**依赖：** T3、T13

**步骤：**
1. `ValidateInput` 增加 `Research *domain.ResearchBrief`。
2. `Validate` 在既有校验结果后，当 `Research.Weather` 非空时追加 `validator.CheckRain` 结果，再统计阻塞与警告。
3. `graph.validate` 传入 `Research: state.Research`。
4. 测试：雨天户外密集的排期产生 blocking；无预报时结果与现状一致；修复分支仍被触发。

**验证：** `go test ./internal/agent -run 'TestValidate|TestGraph'` 通过。

## T15: 草案提示词天气规则

**文件：** `internal/agent/prompts/draft.tmpl`
**依赖：** T13

**步骤：**
1. 在研究规则列表中增加一条：`weather` 来自天气工具；降水日优先室内点位；没有 `weather` 字段时不得猜测天气。
2. 不改动输出格式与其他规则。

**验证：** `go test ./internal/agent -run TestPrompt` 通过。

## T16: 全量验证

**文件：** 无
**依赖：** T1–T15

**步骤：**
1. 运行编译与静态检查。
2. 运行全量测试与并发敏感包测试。

**验证：**
- `go build ./...`
- `go vet ./...`
- `go test ./...`
- `go test -race ./internal/plugins ./internal/agent`

## 执行顺序

```
T1 → T2 → T3
T4 ─────────────┐
T5 → T6 → T12 → T13 → T14
T7 → T8 → T9 ───┴→ T10 → T11
T13 → T15
T1–T15 → T16
```

可并行：T4 与 T1–T3；T7–T9 与 T5–T6。其余按箭头顺序执行。
