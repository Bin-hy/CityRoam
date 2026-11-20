# TripPilot v1.2 体验修复 Tasks

对应文档：
- [spec.md](file:///Users/binhy/Binhy-Projects/AutoAgent/docs/mew-spec/v1.2-experience/spec.md)
- [plan.md](file:///Users/binhy/Binhy-Projects/AutoAgent/docs/mew-spec/v1.2-experience/plan.md)

## 文件清单

| 操作 | 文件 | 职责 |
|---|---|---|
| 修改 | `internal/app/wire.go` | 插件状态规范名 `hotel` / `guide` |
| 修改 | `internal/app/wire_test.go` | 状态名断言 |
| 修改 | `internal/api/meta_test.go` | 真实 wiring → meta 回归 |
| 修改 | `internal/domain/constraint.go` | `LodgingConstraint.AnchorDays` + merge |
| 修改 | `internal/domain/constraint_test.go` | AnchorDays 合并/归一 |
| 新建 | `internal/domain/draft.go` | `DraftFromPlan` |
| 新建 | `internal/domain/draft_test.go` | 草稿反推 |
| 修改 | `internal/domain/itinerary.go` | `TransitLeg`、`Transport.Legs`、Clone |
| 修改 | `internal/domain/itinerary_test.go` | Clone 深拷贝 legs |
| 修改 | `internal/agent/runner.go` | `ResumeRevise`、RunStore 扩展、reject 分支 |
| 新建 | `internal/agent/revise.go` | `resumeRevise` / `rejectProposal` |
| 修改 | `internal/agent/runner_test.go` | revise/reject 状态迁移 + fake store |
| 修改 | `internal/agent/nodes.go` | `WithHotelResolver` |
| 新建 | `internal/agent/hotel_resolver.go` | 解析器接口 + 指定酒店补齐 |
| 修改 | `internal/agent/research.go` | 地理编码兜底、锚定天、价格 warning |
| 修改 | `internal/agent/research_test.go` | 酒店解析与锚定用例 |
| 修改 | `internal/agent/prompts/*` | `anchor_days` 解析说明 |
| 修改 | `internal/runstore/runstore.go` | `GetPlanVersion`、`UpdateRunMode` |
| 修改 | `internal/repo/store.go` | 手写 `UpdateRunMode` |
| 修改 | `internal/api/runs.go` | revise 校验、真实状态返回 |
| 修改 | `internal/api/runs_test.go` | revise/reject 响应 |
| 修改 | `internal/api/versions.go` | 改用 `domain.DraftFromPlan` |
| 修改 | `internal/tools/route.go` | 分段解析、`city=北京`、legs |
| 新建 | `internal/tools/geocode.go` | `HotelGeocoder` |
| 新建 | `internal/tools/polyline.go` | 折线解析 + DP + 截断 |
| 修改 | `internal/tools/route_test.go` | 几何夹具、city 参数 |
| 新建 | `internal/tools/geocode_test.go` | 地理编码适配器 |
| 新建 | `internal/tools/polyline_test.go` | 简化/截断/非法点 |
| 修改 | `internal/domain/scheduler/scheduler.go` | 每日点数上限降级 |
| 修改 | `internal/domain/scheduler/scheduler_test.go` | 上限用例 |
| 修改 | `cmd/api/main.go` | 注入 HotelResolver |
| 修改 | `web/src/api/types.ts` | ResumeRequest / TransitLeg / Lodging |
| 修改 | `web/src/api/trips.ts` | `cancelRun` |
| 修改 | `web/src/api/trips.test.ts` | revise/cancel 请求 |
| 修改 | `web/src/api/openapi.yaml` | revise、legs、anchor_days |
| 修改 | `web/src/features/itinerary/types.ts` | 解析 legs/lodging |
| 新建 | `web/src/features/itinerary/types.test.ts` | 宽松解析 |
| 修改 | `web/src/features/map/TripMap.tsx` | 逐段绘制、酒店 marker、图例 |
| 修改 | `web/src/features/map/TripMap.test.tsx` | legs/兜底/酒店 |
| 修改 | `web/src/features/trace/traceReducer.ts` | stage label、run.started 模式 |
| 修改 | `web/src/features/trace/traceReducer.test.ts` | 对应断言 |
| 修改 | `web/src/features/workspace/RunActions.tsx` | 反馈框/取消/提案拒绝 |
| 修改 | `web/src/features/workspace/RunActions.test.tsx` | 交互断言 |
| 修改 | `web/src/features/workspace/WorkspacePage.tsx` | 传入 stage |
| 修改 | `docs/环境变量配置.md` | 插件状态说明与自检 |

## T1：插件状态规范名

**文件：** `internal/app/wire.go`、`internal/app/wire_test.go`
**依赖：** 无
**步骤：**

1. 定位 `WirePlugins` 中构造 `plugins.Status{Name: "search_hotels"}` 与 `"search_travel_notes"` 的位置。
2. 将状态名改为 `"hotel"` / `"guide"`；插件 `Name()`、工具注册名、`DisabledPlugins` 名称均不动。
3. 更新 `wire_test.go` 中对状态名的断言（如有）。
4. 全局搜索 `search_hotels` / `search_travel_notes`，确认仅在插件实现/工具注册处保留。

**验证：** `go test ./internal/app/...` 通过；`go test ./internal/api -run TestMeta` 通过。

## T2：meta 真实 wiring 回归

**文件：** `internal/api/meta_test.go`
**依赖：** T1
**步骤：**

1. 构造真实 `app.WirePlugins` 输出（或使用测试中现有 wiring 构造器），取其 `Statuses` 填入 `Deps.PluginStatuses`。
2. 请求 `/api/meta`，断言 `plugins.hotels.enabled` / `plugins.guides.enabled` 与对应 `disabled_reason`。
3. 另加一个“名称未知”状态，断言不会覆盖两个已知插件字段（防止未来误匹配）。

**验证：** `go test ./internal/api -run TestMeta -v`，输出显示 enabled 与原因符合输入。

## T3：约束锚定天

**文件：** `internal/domain/constraint.go`、`internal/domain/constraint_test.go`
**依赖：** 无
**步骤：**

1. `LodgingConstraint` 增加 `AnchorDays []int`（json `anchor_days,omitempty`）。
2. `MergeConstraints`：`patch.Lodging.AnchorDays != nil` 时克隆覆盖，nil 不动。
3. 新增测试：合并覆盖、nil 保留、`NormalizeAnchorDays` 去重/排序/越界过滤。
4. 检查 `ConstraintSet` 的其它 clone/归一函数，补 AnchorDays 深拷贝。

**验证：** `go test ./internal/domain -run TestConstraint` 通过。

## T4：DraftFromPlan 领域化

**文件：** `internal/domain/draft.go`、`internal/domain/draft_test.go`、`internal/api/versions.go`
**依赖：** 无
**步骤：**

1. 将 `internal/api/versions.go` 的 `draftFromPlan` 原样迁入 `domain.DraftFromPlan(plan Itinerary) DraftPlan`，保持 POI/meal 映射与时长逻辑。
2. api 侧改为调用 `domain.DraftFromPlan`，删除本地函数与不再使用的 import。
3. `draft_test.go`：给定含 poi/meal 块的行程，断言 items 的 `poi_id/type/duration_min` 顺序与值。
4. 运行 api 既有版本/调整测试，确认行为不变。

**验证：** `go test ./internal/domain ./internal/api -run 'TestDraft|TestVersion|TestAdjust'` 通过。

## T5：RunStore 扩展与 mode 更新

**文件：** `internal/agent/runner.go`、`internal/runstore/runstore.go`、`internal/repo/store.go`、`internal/agent/runner_test.go`
**依赖：** 无
**步骤：**

1. `runner.go` 定义 `VersionState`，`RunStore` 增加 `GetPlanVersion`、`UpdateRunMode`。
2. `runstore` 实现二者：读 `repo.PlanVersion` 反序列化 plan/constraints/violations；`UpdateRunMode` 调 repo。
3. `repo/store.go` 增加手写方法：`s.pool.Exec(ctx, "UPDATE runs SET mode=$2 WHERE id=$1", runID, mode)`，返回错误。
4. 更新 `runner_test.go` 中的 fake store（以及其他实现 RunStore 的测试桩），补齐新方法。
5. 编译全部包，确认无遗漏实现。

**验证：** `go build ./...` 与 `go test ./internal/agent -run TestRunner` 通过。

## T6：revise 同 run 转 adjustment

**文件：** `internal/agent/runner.go`、`internal/agent/revise.go`、`internal/agent/runner_test.go`
**依赖：** T4、T5
**步骤：**

1. 新增 `ResumeRevise` 常量，`Resume` switch 增加分支，调用 `resumeRevise`。
2. `resumeRevise` 按 plan：校验 status/input 长度；基线取 `Snapshot.Seq`，否则 `PreviousSeq`；置 `Previous`、`PreviousSeq`。
3. 设置 `Mode="adjustment"`、`Input=trimmed`；清 Proposal/Snapshot/Best/BestViolations/History/Suggestions/Round/StopReason/Error；`Stage="adjust"`、`Status=queued`。
4. 依次调用 `UpdateRunMode`、`saveState`、`UpdateRunStatus(queued,"adjust")`，再 `enqueue`（复用 Start 的入队逻辑，避免重复广播逻辑）。
5. 测试：初始 awaiting → revise；断言同 run ID、mode、status、stage、Input、PreviousSeq；断言不产生 cancelled 事件。

**验证：** `go test ./internal/agent -run TestRunnerRevise -v`。

## T7：提案拒绝恢复基线

**文件：** `internal/agent/revise.go`、`internal/agent/runner.go`、`internal/agent/runner_test.go`
**依赖：** T4、T5
**步骤：**

1. `ResumeReject` 改为调用 `rejectProposal`；`Proposal==nil` 时保留旧取消分支。
2. `rejectProposal`：用 `GetPlanVersion(TripID, PreviousSeq)` 取基线；恢复 Plan/Constraints/Violations；`Draft=domain.DraftFromPlan(base.Plan)`。
3. 清 Proposal/Snapshot/Suggestions/StopReason；`Stage="proposal_rejected"`、`Status=awaiting_user`；落库并 `UpdateRunStatus`。
4. 广播 `run.awaiting_user`（payload 含 stage、proposal=null），不广播 cancelled。
5. 读取版本失败时返回错误且不落半成品；测试覆盖成功/失败/legacy 三路。

**验证：** `go test ./internal/agent -run TestRunnerReject -v`。

## T8：API revise 与真实状态

**文件：** `internal/api/runs.go`、`internal/api/runs_test.go`
**依赖：** T6、T7
**步骤：**

1. `resumeRequest` 与 `resumeAction` 支持 `revise`；trim 后长度 1–2000，否则 400 `invalid_input`。
2. 删除/替换 `resumedStatus`：resume 成功后用 `deps.Store.GetRun(runID)` 取实际 status；读取失败时按 kind 回退（accept→completed，revise→queued，其余→queued）。
3. 测试：revise 空输入/超长 400；revise 成功响应 `status=queued`；提案 reject 响应 `awaiting_user`；无提案 reject 仍 `cancelled`。

**验证：** `go test ./internal/api -run TestResume -v`。

## T9：地理编码适配器

**文件：** `internal/tools/geocode.go`、`internal/tools/geocode_test.go`
**依赖：** 无
**步骤：**

1. 新建 `HotelGeocoder{amap *AMapClient; city string}`，`NewHotelGeocoder(amap)` 固定 `city="北京"`。
2. `Resolve(ctx, query)`：去空白；空则返回空；调用 `amap.Geocode(ctx, query, g.city)`。
3. 每个结果转 `domain.Hotel`：`ID="amap:<lng>,<lat>"`、`Name=query`、`Address=FormattedAddress`、`Lat/Lng`、`Provider="amap"`、`PricePerNightYuan=0`、`PriceEstimated=true`。
4. 过滤 `City` 不含“北京”的噪声结果（空 City 保留）；最多返回 5 条。
5. 测试：httptest 模拟 `/v3/geocode/geo`，断言 `city=北京`、坐标解析、provider/价格标记、空响应。

**验证：** `go test ./internal/tools -run TestHotelGeocoder -v`。

## T10：HotelResolver 接口与注入

**文件：** `internal/agent/hotel_resolver.go`、`internal/agent/nodes.go`、`cmd/api/main.go`
**依赖：** T9
**步骤：**

1. `hotel_resolver.go` 定义 `HotelResolver` 接口（`Resolve(ctx, query) ([]domain.Hotel, error)`）。
2. `Nodes` 增加 `hotelResolver HotelResolver` 字段；`nodes.go` 增加 `WithHotelResolver` 选项并在构造时赋值。
3. `cmd/api/main.go`：`amap != nil` 时追加 `agent.WithHotelResolver(tools.NewHotelGeocoder(amap))`。
4. 确保未配置 AMap 时 resolver 为 nil，研究逻辑走原有提问路径。

**验证：** `go build ./...`；`go test ./internal/agent -run TestNodes` 通过。

## T11：指定酒店补齐（去重/限次）

**文件：** `internal/agent/hotel_resolver.go`、`internal/agent/research_test.go`
**依赖：** T10
**步骤：**

1. 实现 `resolveSpecifiedHotels(ctx, resolver, specified, existing []domain.Hotel) ([]domain.Hotel, int)`。
2. 用 `normalizeHotelName` 去重 `specified`；已能被 `matchSpecifiedHotels(existing, specified)` 覆盖的名称跳过。
3. 最多发起 2 次 `Resolve` 调用；用调用方 ctx（research 已有 12s deadline），单次不额外加长。
4. 合并结果：resolver 候选 + existing → `sanitizeHotels(..., limit)` 去重（按名称与坐标）；保持已有工具结果在前。
5. 测试：fake resolver 记录调用次数，断言 ≤2、跳过已命中项、结果去重、provider 保留。

**验证：** `go test ./internal/agent -run TestResolveSpecifiedHotels -v`。

## T12：研究流程接入地理编码

**文件：** `internal/agent/research.go`、`internal/agent/research_test.go`
**依赖：** T11、T3
**步骤：**

1. 调整 `Research` 开头的“无工具直接返回”逻辑：无工具且（无指定酒店或 resolver==nil）才走原 skipped 路径；否则继续构建 session。
2. 无工具路径：用 `fallbackResearchBrief` 生成基础 brief 后，调用 `resolveSpecifiedHotels`，把结果写入 `Accommodation.Options`，再进入 `normalizeResearchBrief`。
3. 有工具路径：工具循环结束后、归一化之前，对 `constraints.Lodging.Hotels` 中未被工具结果命中的名称调用补齐，并把新候选追加到 `brief.Accommodation.Options`。
4. 补齐结果不参与指南信号匹配；只影响住宿。
5. 测试：无工具 + fake resolver → `Status=ok`、selected 非空、provider=amap；工具命中时不调用 resolver。

**验证：** `go test ./internal/agent -run TestResearchHotel -v`。

## T13：归一化与解析提示词

**文件：** `internal/agent/research.go`、`internal/agent/prompts/*`、`internal/agent/research_test.go`
**依赖：** T12、T3
**步骤：**

1. 修改 `normalizeResearchBrief` 的价格 0 分支：当选中项来自地理编码（`Provider=="amap"` 或 `PriceEstimated`）且属于用户指定酒店时，保留选择，设 `IncludeInBudget=false`，追加 warning“酒店价格为未知，未计入预算”。
2. 锚定天计算：`constraints.Lodging.AnchorDays` 归一后非空 → 用它；否则用 LLM 建议的 `proposedAnchorDays`；否则生成 `0..days-1` 全部天。仅在存在 Selected 时写入。
3. 约束解析提示词补充：`anchor_days` 为 0-based；“第 N 天住 X”示例；“全程/每天都住”则留空。
4. 测试：三种优先级各一例；零价格保留 + warning + 不计预算；无酒店时 AnchorDays 为空。

**验证：** `go test ./internal/agent -run 'TestResearch.*Anchor|TestResearch.*Price' -v`。

## T14：领域 TransitLeg

**文件：** `internal/domain/itinerary.go`、`internal/domain/itinerary_test.go`
**依赖：** 无
**步骤：**

1. 增加 `TransitLeg` 类型（字段按 plan 定义，`Polyline [][2]float64`）。
2. `Transport` 增加 `Legs []TransitLeg`（`omitempty`）。
3. `Itinerary.Clone` 深拷贝每一天每个 block 的 `Transport.Legs` 及每条 `Polyline`（`append([][2]float64(nil), ...)`）。
4. 测试：构造带 legs 的行程，Clone 后修改原 legs/点，断言副本不变；无 legs 时保持 nil。

**验证：** `go test ./internal/domain -run TestItineraryClone` 通过。

## T15：折线解析与简化

**文件：** `internal/tools/polyline.go`、`internal/tools/polyline_test.go`
**依赖：** T14
**步骤：**

1. `parsePolyline(raw string) [][2]float64`：按 `;` 分段、按 `,` 拆 `lng,lat`；跳过空项/非数字；连续重复点去重。
2. `simplifyPolyline(points, toleranceMeters)`：Douglas–Peucker；容差内用等距投影（以首点纬度换算米）；点数 <3 直接返回。
3. `capPolyline(points, max)`：超过上限时均匀抽样并保留首末点；`max<2` 时返回直线两端。
4. 测试：非法字符跳过、端点保留、15m 直线压缩为 2 点、61 点截断为 60、极端短数组。

**验证：** `go test ./internal/tools -run TestPolyline -v`。

## T16：RouteService 分段解析

**文件：** `internal/tools/route.go`、`internal/tools/route_test.go`
**依赖：** T14、T15
**步骤：**

1. 扩展响应结构：walking/driving 的 `paths[0].steps[].polyline`；transit 的 `transits[0].segments[]`（`walking.steps[].polyline`、`bus.buslines[].polyline/name/departure_stop/arrival_stop/duration/distance`）。
2. walking/driving：把 steps 折线拼成单条 leg（mode=walk/drive），简化后写入 `Legs`；指标仍取 `paths[0]`。
3. transit：按 segments 顺序生成 legs：步行段 mode=walk；公交/地铁段 mode=transit + `LineName` + 上下车站；整体 `Legs` 简化；总时长/距离/费用仍取 `transits[0]`。
4. `city`/`cityd` 改为 `"北京"`；无 polyline 时 `Legs=nil`，不影响既有指标。
5. 更新 `route_test.go`：原 transit 断言 city 改北京；新增 walking steps 夹具、transit segments 夹具（walk→bus→walk），断言 legs 数量/顺序/线路名；无 polyline 夹具断言 Legs 为空。

**验证：** `go test ./internal/tools -run TestRouteService -v`。

## T17：每日点数上限

**文件：** `internal/domain/scheduler/scheduler.go`、`internal/domain/scheduler/scheduler_test.go`
**依赖：** T14
**步骤：**

1. 在每天 blocks 组装完成后统计 `transport.legs[].polyline` 总点数。
2. 超过 600 时，按单条 leg 点数从大到小将折线降级为直线（仅保留首末点），并置 `leg.Estimated=true`，直到 ≤600。
3. 实现为 scheduler 内部小函数（输入 `[]domain.Block`，原地修改），不改变 Route 接口。
4. 测试：fake RouteProvider 返回 12 条各 60 点的 legs，断言降级后总数 ≤600、最多保留部分完整折线、首末点未变；总点数未超限时不修改。

**验证：** `go test ./internal/domain/scheduler -run TestDayPolylineCap -v`。

## T18：前端类型与宽松解析

**文件：** `web/src/features/itinerary/types.ts`、`web/src/features/itinerary/types.test.ts`
**依赖：** T14（数据结构已定）
**步骤：**

1. 增加 `TransitLeg`、`Transport.legs`、`LodgingSummary`、`Itinerary.lodging` 类型（字段与 plan 一致）。
2. 在 `parseItinerary` 中增加 `parseLegs`、`parseLodging`：只接受数组/对象；`polyline` 过滤非数组、非数字、长度 < 2 的点；`anchor_days` 仅保留整数；非法字段按 `undefined` 处理。
3. 保持现有块解析行为不变；解析失败仍返回原错误语义。
4. 测试：完整 legs/lodging；polyline 含字符串/单点；legs 非数组；lodging 缺失；旧响应（无新字段）不报错。

**验证：** `cd web && npm test -- features/itinerary/types.test.ts`。

## T19：API 客户端与 OpenAPI

**文件：** `web/src/api/types.ts`、`web/src/api/trips.ts`、`web/src/api/trips.test.ts`、`web/src/api/openapi.yaml`
**依赖：** T8
**步骤：**

1. `ResumeRequest` 增加 `{ action: "revise"; input: string }` 联合分支（保留 accept/reject/supplement/resolve_research）。
2. `trips.ts` 增加 `cancelRun(runID)`：`POST /runs/{runID}/cancel`，复用现有请求封装与错误处理。
3. `openapi.yaml`：resume 的 `action` 枚举加 `revise` 并描述语义；Transport 增加 `legs`（TransitLeg schema）；Lodging 增加 `anchor_days`。
4. 测试：revise 请求体正确；cancel 命中正确路径与方法；错误响应映射不变。

**验证：** `cd web && npm test -- api/trips.test.ts`；`npx tsc -b` 无类型错误。

## T20：RunActions 反馈与取消

**文件：** `web/src/features/workspace/RunActions.tsx`、`web/src/features/workspace/WorkspacePage.tsx`、`web/src/features/workspace/RunActions.test.tsx`
**依赖：** T19
**步骤：**

1. `RunActionsProps` 增加可选 `stage?: string`；`WorkspacePage` 从 trace 状态传入 `stage`。
2. awaiting 分支保持研究问题/缺字段表单不变；其余场景新增本地状态 `reviseOpen` 与 `feedback`。
3. 初始方案（`proposal==null && stage!=="proposal_rejected"`）：显示“接受方案”“拒绝”“取消规划”；“拒绝”仅 `setReviseOpen(true)`，不发请求。
4. 提案（`proposal!=null`）：显示“接受调整”“拒绝”“取消规划”；“拒绝”调用 `resume.mutate({action:"reject"})`，成功后由 `onActionComplete` 刷新为 `proposal_rejected`。
5. `stage==="proposal_rejected"`：不显示接受按钮，直接显示反馈框。
6. 反馈框：textarea（maxLength 2000）+ 字数 + “提交修改”，调用 `resume.mutate({action:"revise", input})`；成功后清空输入并 `onActionComplete`。
7. “取消规划”调用 `cancelRun(runID)`，成功后 `onActionComplete`；失败展示 Alert。
8. 测试：点拒绝无网络调用；提交 revise 体正确；proposal 拒绝调用 reject；proposal_rejected 显示反馈框；cancel 路径正确；busy 时按钮禁用。

**验证：** `cd web && npm test -- features/workspace/RunActions.test.tsx`。

## T21：TripMap 逐段绘制与酒店标记

**文件：** `web/src/features/map/TripMap.tsx`、`web/src/features/map/TripMap.test.tsx`
**依赖：** T18
**步骤：**

1. 酒店 marker：`itinerary.lodging?.hotel` 有效坐标时创建一次 Marker（title=酒店名，zIndex 与选中态不冲突），加入 overlays 参与 `setFitView`。
2. 逐天绘制：遍历 blocks；遇到 transit 块时确定 from/to（上一个有效坐标块，否则当天锚定的酒店；下一个有效坐标块，否则当天的返回酒店）。
3. 有 `legs`：逐段 `Polyline`，按 mode 选样式——walk 灰色/绿色虚线，transit 蓝色实线（地铁用橙色），drive 紫色；`line_name` 写入 polyline 的 title 或 infoWindow 内容。
4. 无 `legs`：画一条灰色虚线直线；旧快照路径保持可用。
5. 图例：增加步行/公交/地铁/驾车；保留 GCJ-02 徽标。
6. 测试：mock namespace；断言 legs 数量与样式差异、无 legs 时单条虚线、酒店 marker 存在且 title 正确。

**验证：** `cd web && npm test -- features/map/TripMap.test.tsx`。

## T22：Trace 阶段与 run.started 模式

**文件：** `web/src/features/trace/traceReducer.ts`、`web/src/features/trace/traceReducer.test.ts`
**依赖：** T6（事件 payload 已含 mode）
**步骤：**

1. `stageLabels` 增加 `proposal_rejected: "已拒绝提案，待修改"`。
2. `run.started`：读取 `payload.mode`；`adjustment` → `stage="adjust"`，否则 `"parse"`；同时清空 `proposal`、`missing`、`error`，状态置 running。
3. 保证 revise 后同一 run 追加事件时，旧的“调整提案”卡片不会残留。
4. 测试：两种 mode 的 stage 断言；proposal 被清空；原有事件映射不变。

**验证：** `cd web && npm test -- features/trace/traceReducer.test.ts`。

## T23：环境变量文档同步

**文件：** `docs/环境变量配置.md`
**依赖：** T1
**步骤：**

1. 在插件章节补充：状态名 `hotel`/`guide` 与 `/api/meta` 的对应关系；“未启用”的三种原因（开关 false、密钥缺失、sidecar 不可达）。
2. 增加“启用后自检”两步：`curl /api/meta` 看 `plugins.*.enabled`；前端设置页/研究面板看原因是否消失。
3. 说明关闭 FlyAI 但配置 `AMAP_WEB_KEY` 时，指定酒店仍可通过地理编码锚定（价格缺失 warning）。
4. 校对不出现密钥示例值。

**验证：** 按文档逐步操作，`/api/meta` 输出与文档一致；Markdown 链接有效。

## T24：静态检查与全量测试

**文件：** 无（验证任务）
**依赖：** T1–T23 全部完成
**步骤：**

1. `gofmt -l .`（应为空）；`go vet ./...`。
2. `make test`（`go test -race ./...`）。
3. `cd web && npm test && npm run build`。
4. 使用本地已有数据库运行 `make eval`，与 v1 20 例、v1.1 插件基线对比：通过率不降，p95 增幅 ≤10%；记录实际数字。
5. 抽查一条新生成行程的 plan JSON：`legs` 点数 ≤60/段、≤600/天；快照体积 ≤100KB。

**验证：** 所有命令退出码 0；eval 对比结果与点数/体积记录留档（供 checklist 引用）。

## T25：端到端实跑取证

**文件：** 无（验证任务）
**依赖：** T24
**步骤（浏览器逐项执行）：**

1. 插件：分别开启/关闭 hotel 与 guide，观察 `/api/meta` 与前端“未启用”提示及原因（F1–F3）。
2. 改稿：初始方案点拒绝 → 页面出现反馈框且运行未结束 → 提交“第二天下午轻松一点” → 同 run 出新提案 → 接受 → 版本 seq+1；提案再拒绝 → 回到反馈框、无 cancelled（F4–F8）。
3. 取消：任意 awaiting 状态点取消 → run 变 cancelled（F7）。
4. 酒店：关闭 FlyAI、保留 AMap，输入“我住 XX 酒店” → 每天首尾块为酒店、地图有酒店标记、预算 warning、价格为 0 时不计住宿费（F9–F14）。
5. 地图：有 key 时观察步行/公交/地铁分段着色；移除 key 或伪造旧快照 → 虚线兜底；刷新页面后 awaiting 状态仍可继续 revise（mode 已持久化）（F15–F18）。
6. 旧数据：打开升级前的 trip，时间轴/预算不变，地图可显示。

**验证：** 每步记录实际观察结果（界面文字/事件/接口响应），作为 checklist 验收证据。

## 执行顺序

```
T1 ── T2
T3 ──┬── T12 ── T13
T4 ──┼── T6 ──┐
T5 ──┘        ├── T8 ── T19 ── T20
T6 ── T7 ─────┘
T9 ── T10 ── T11 ── T12
T14 ── T15 ── T16 ── T17
T18（依赖 T14）── T21
T6 ── T22
T1 ── T23
所有任务 ── T24 ── T25
```

- 可并行组：{T1–T5}、{T9–T11}、{T14–T17}、{T18/T21} 与后端组互不阻塞。
- 串行关键路径：T14 → T16 → T24 → T25。
- T20 必须在 T19 之后；T22 依赖 T6 的事件形状，可与 T20/T21 并行。
