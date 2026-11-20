# TripPilot v1.5 交互重构 Tasks

状态：已批准（task）
日期：2026-11-20
依据：[spec.md](./spec.md)（已批准）、[plan.md](./plan.md)（已批准）

## 前置条件

- Go 1.24、Node.js 24、npm 可用；`web/node_modules` 已安装。
- 需要安装一个新依赖：`@radix-ui/react-popover`（用于预算与版本菜单）。安装失败时按 plan 的风险对策退回 Dialog。
- Playwright 浏览器已安装（`npx playwright install`，如未安装）。
- 真实地图手工验收需要 `VITE_AMAP_JS_KEY`；自动化测试不依赖真实 Key，使用地图降级路径。
- 不新增环境变量、不读取或输出 `.env.local` 中的密钥。
- 不重启用户已在 8080 运行的 API 进程；需要手工联调时另起端口或使用独立实例。
- 开始前基线必须通过：`go test ./...`、`cd web && npm test`、`npm run build`。若基线失败，先停下报告，不在失败基线上叠加改动。

## 执行规则

- 严格按任务顺序执行；每个任务完成后运行「验证」，有输出证据再标记完成。
- 每个任务只改列出的文件；发现需要额外文件时先回到 task.md 补任务。
- 每个逻辑阶段结束后提交一次代码；提交前检查 `git status`，不把密钥、临时文件或无关改动加入提交。
- 后端字段增补必须保持向后兼容：新增字段全部可选，旧事件/旧版本按缺省渲染。
- 不新增 LLM 调用、不新增数据库表/迁移、不新增接口。

## 文件清单

### 新增

| 文件 | 职责 |
|---|---|
| `internal/plugins/events_preview.go` | 预览结构、接口、截断、查询/结果摘要 |
| `internal/plugins/events_preview_test.go` | 三类工具预览与降级测试 |
| `internal/runstore/runstore_test.go` | `plan` JSON 内嵌 explanations 测试 |
| `web/src/design/tokens.ts` | JS 侧颜色、字体、日期、天主题 |
| `web/src/design/primitives.tsx` | Surface/Hairline/Chip/Stat/SectionLabel/StatusDot/EmptyHint |
| `web/src/components/ui/popover.tsx` | Radix Popover 封装 |
| `web/src/components/ui/sheet.tsx` | 全屏 Dialog 变体 |
| `web/src/features/conversation/eventParse.ts` | 事件类型守卫、安全取值、去重排序 |
| `web/src/features/conversation/deriveRunView.ts` | 事件 → 运行视图 |
| `web/src/features/conversation/narrate.ts` | 约束标签与确定性文案 |
| `web/src/features/conversation/deriveConversation.ts` | 事件 → 对话条目 |
| `web/src/features/conversation/deriveCanvas.ts` | 事件 + trip/version → 画布模型 |
| `web/src/features/conversation/useSSEEvents.ts` | SSE 连接、重放、去重、重连 |
| `web/src/features/conversation/useSession.ts` | 跨回合会话与动作 |
| `web/src/features/conversation/ConversationRail.tsx` | 对话线程 |
| `web/src/features/conversation/UserMessage.tsx` | 用户消息 |
| `web/src/features/conversation/AgentMessage.tsx` | Agent 阶段消息 |
| `web/src/features/conversation/ToolCard.tsx` | 工具卡 |
| `web/src/features/conversation/DecisionCard.tsx` | 补充/住宿/提案/确认卡 |
| `web/src/features/conversation/PlanCard.tsx` | 方案统计卡 |
| `web/src/features/conversation/Composer.tsx` | 全流程输入框 |
| `web/src/features/conversation/model.test.ts` | 纯函数推导测试 |
| `web/src/features/conversation/Composer.test.tsx` | Composer 模式测试 |
| `web/src/features/canvas/PlanCanvas.tsx` | 画布编排 |
| `web/src/features/canvas/CanvasToolbar.tsx` | 画布工具栏 |
| `web/src/features/canvas/DayTabs.tsx` | 天切换 |
| `web/src/features/canvas/Inspector.tsx` | 选中点检查器 |
| `web/src/features/canvas/CanvasSheet.tsx` | 移动端全屏画布 |
| `web/src/features/canvas/DayList.tsx` | 窄屏纵向清单 |
| `web/src/features/canvas/PlanCanvas.test.tsx` | 画布编排测试 |
| `web/src/features/canvas/Inspector.test.tsx` | 检查器测试 |
| `web/src/features/versions/VersionMenu.tsx` | 版本菜单与恢复 |
| `web/src/features/versions/VersionMenu.test.tsx` | 版本菜单测试 |
| `web/src/features/budget/budgetModel.ts` | 预算纯函数 |
| `web/src/features/budget/budgetModel.test.ts` | 预算纯函数测试 |
| `web/src/features/budget/BudgetPopover.tsx` | 预算摘要浮层 |
| `web/src/features/budget/BudgetPopover.test.tsx` | 预算浮层测试 |
| `web/src/features/research/ResearchNotes.tsx` | 分享页住宿与攻略摘要 |
| `web/src/features/create/PreferenceSheet.tsx` | 偏好弹窗 |
| `web/src/features/timeline/timelineModel.test.ts` | 时间轴模型测试 |

### 修改

| 文件 | 改动 |
|---|---|
| `internal/plugins/hotel.go` | 实现 `EventPreviewer` |
| `internal/plugins/guide.go` | 实现 `EventPreviewer` |
| `internal/plugins/weather.go` | 实现 `EventPreviewer` |
| `internal/plugins/governor.go` | `toolEvent.Query/Preview` 与发布路径 |
| `internal/plugins/governor_test.go` | 预览字段断言 |
| `internal/plugins/test_helpers_test.go` | stub 工具支持预览 |
| `internal/domain/validator/validator.go` | `BaseRuleCount`、`CheckedCount` |
| `internal/domain/validator/validator_test.go` | 计数断言 |
| `internal/agent/schedule.go` | `validation.result.checked` |
| `internal/agent/schedule_test.go` | checked 断言 |
| `internal/runstore/runstore.go` | `planSnapshot` 与 `encodeVersionPlan` |
| `web/package.json` | 新增 `@radix-ui/react-popover` |
| `web/index.html` | 如需字体/标题，仅调整静态标题，不改结构 |
| `web/src/index.css` | token、排版、时间轴样式 |
| `web/tailwind.config.ts` | 颜色/衬线字体映射 |
| `web/src/components/AppHeader.tsx` | 编辑感顶栏 |
| `web/src/components/StateMessage.tsx` | 统一加载/空/错样式 |
| `web/src/features/itinerary/types.ts` | `Explanation`、`explanations`、`explanationMap` |
| `web/src/features/itinerary/types.test.ts` | 解释解析测试 |
| `web/src/features/map/TripMap.tsx` | 编号、住宿、天过滤、选中、无 InfoWindow |
| `web/src/features/map/TripMap.test.tsx` | 新交互断言 |
| `web/src/features/timeline/ItineraryTimeline.tsx` | 天过滤、选中、样式 |
| `web/src/features/timeline/ItineraryTimeline.test.tsx` | 新交互断言 |
| `web/src/features/timeline/timelineModel.ts` | 编号、天过滤元数据 |
| `web/src/features/workspace/WorkspacePage.tsx` | 布局与会话编排 |
| `web/src/features/workspace/WorkspacePage.test.tsx` | 新交互断言 |
| `web/src/features/create/CreatePage.tsx` | 编辑感双栏 |
| `web/src/features/create/CreateForm.tsx` | 单输入 + 偏好标签 |
| `web/src/features/create/CreateForm.test.tsx` | 新交互断言 |
| `web/src/features/share/SharePage.tsx` | 只读叙事 |
| `web/src/features/share/SharePage.test.tsx` | 只读断言 |
| `web/src/features/share/ShareDialog.tsx` | 样式重做 |
| `web/src/features/share/ShareDialog.test.tsx` | 保留能力断言 |
| `web/src/features/trips/MyTripList.tsx` | 编辑感行式列表 |
| `web/src/features/trips/MyTripList.test.tsx` | 样式相关断言 |
| `web/src/features/auth/LoginPage.tsx` | 编辑感登录页 |
| `web/src/features/eval/EvalDashboard.tsx` | 仅 token |
| `web/src/features/eval/EvalDashboard.test.tsx` | 保持通过 |
| `web/e2e/golden.spec.ts` | 新交互与工具卡场景 |

### 删除（新界面落地且测试迁移后）

| 文件 | 原因 |
|---|---|
| `web/src/features/trace/RunTrace.tsx` | 由对话流取代 |
| `web/src/features/trace/traceReducer.ts` | 由 `deriveRunView` 取代 |
| `web/src/features/trace/traceReducer.test.ts` | 迁移到 `model.test.ts` |
| `web/src/features/workspace/RunActions.tsx` | 由 `DecisionCard` + `Composer` 取代 |
| `web/src/features/workspace/RunActions.test.tsx` | 迁移到决策卡/Composer 测试 |
| `web/src/features/research/ResearchPanel.tsx` | 由检查器与 `ResearchNotes` 取代 |
| `web/src/features/research/ResearchPanel.test.tsx` | 删除 |
| `web/src/features/research/BudgetModeToggle.tsx` | 由 `BudgetPopover` 取代 |
| `web/src/features/research/BudgetModeToggle.test.tsx` | 删除 |
| `web/src/features/budget/BudgetPanel.tsx` | 由 `BudgetPopover` 取代 |
| `web/src/features/budget/BudgetPanel.test.tsx` | 迁移到 `budgetModel.test.ts`/`BudgetPopover.test.tsx` |
| `web/src/features/versions/VersionPanel.tsx` | 由 `VersionMenu` 取代 |
| `web/src/features/versions/VersionPanel.test.tsx` | 迁移到 `VersionMenu.test.tsx` |
| `web/src/features/versions/VersionSwitcher.tsx` | 由 `VersionMenu` 取代 |
| `web/src/features/versions/VersionSwitcher.test.tsx` | 删除 |
| `web/src/features/versions/PlanDiff.tsx` | 差异摘要改在版本菜单/检查器呈现 |
| `web/src/features/versions/PlanDiff.test.tsx` | 删除 |
| `web/src/hooks/useRunStream.ts` | 由 `useSSEEvents` 取代 |

## 阶段 A：后端事件增补

### T1 基线验证

**文件：** 无
**依赖：** 无
**步骤：**
1. 运行 `go test ./...`，记录结果。
2. 运行 `cd web && npm test`，记录结果。
3. 运行 `cd web && npm run build`，记录结果。

**验证：** 三条命令均通过；若失败，停止并报告失败项，不进入 T2。

### T2 事件预览基础结构

**文件：** `internal/plugins/events_preview.go`（新建）
**依赖：** T1
**步骤：**
1. 定义 `EventPreview`（`Title`、`Subtitle`、`PriceYuan`，JSON 字段与 plan 一致）。
2. 定义 `EventPreviewer` 接口（`QuerySummary(json.RawMessage) string`、`ResultPreview(json.RawMessage) []EventPreview`）。
3. 定义 `previewLimit = 3`、`clipRunes`、`shortDate`（把 `2006-01-02` 转为 `1/2`）。
4. 定义包内辅助函数 `queryFor(tool agent.ResearchTool, input json.RawMessage) string` 与 `previewFor(tool agent.ResearchTool, output json.RawMessage) []EventPreview`：未实现接口或解析失败时返回空。
5. 确保所有解析错误静默降级，不 panic、不影响工具调用。

**验证：** `go test ./internal/plugins` 编译通过（此时无新测试）。

### T3 酒店工具预览

**文件：** `internal/plugins/hotel.go`
**依赖：** T2
**步骤：**
1. 为 `HotelTool` 实现 `QuerySummary(input)`：解析 `SearchHotelsInput`，输出城市、入住/退房日期、晚数、最高价、最低星级。
2. 为 `HotelTool` 实现 `ResultPreview(output)`：解析 `{hotels:[{name,star,score,price_per_night_yuan}]}`，最多 3 条；`Title` 为酒店名，`Subtitle` 为星级/评分，`PriceYuan` 为每晚价格。
3. 所有字段做零值/空值处理；日期无法解析时省略对应片段。
4. 不输出 `booking_url`、地址原文之外的敏感字段或原始响应。

**验证：** `go test ./internal/plugins -run TestHotel` 与 `go build ./...` 通过。

### T4 攻略工具预览

**文件：** `internal/plugins/guide.go`
**依赖：** T2
**步骤：**
1. 为 `GuideTool` 实现 `QuerySummary(input)`：解析 `SearchNotesInput`，输出搜索词与可选站点。
2. 为 `GuideTool` 实现 `ResultPreview(output)`：解析 `{results:[{title,site_name}]}`，最多 3 条；`Title` 为标题，`Subtitle` 为站点名。
3. 空结果返回空切片而非 nil；不要输出 URL 与摘要原文。

**验证：** `go test ./internal/plugins -run TestGuide` 通过。

### T5 天气工具预览

**文件：** `internal/plugins/weather.go`
**依赖：** T2
**步骤：**
1. 为 `WeatherTool` 实现 `QuerySummary(input)`：解析 `WeatherInput`，输出城市、起始日期、天数。
2. 为 `WeatherTool` 实现 `ResultPreview(output)`：解析 `{days:[{date,day_weather,night_weather,day_temp,night_temp}]}`，最多 3 条；`Title` 为日期，`Subtitle` 为天气与温度。
3. 温度字段缺失时只用天气描述。

**验证：** `go test ./internal/plugins -run TestWeather` 通过。

### T6 预览测试

**文件：** `internal/plugins/events_preview_test.go`（新建）
**依赖：** T3、T4、T5
**步骤：**
1. 表驱动测试三类工具的 Query 摘要：正常输入、缺字段、非法 JSON。
2. 表驱动测试 ResultPreview：3 条上限、超长标题截断、空结果、未知结构。
3. 测试未实现 `EventPreviewer` 的 `stubTool` 返回空 Query/Preview。
4. 断言预览中不出现 URL、令牌或原始响应字段。

**验证：** `go test ./internal/plugins -run TestEventPreview -count=1` 通过。

### T7 Governor 写入查询摘要

**文件：** `internal/plugins/governor.go`
**依赖：** T6
**步骤：**
1. 给 `toolEvent` 增加 `Query string json:"query,omitempty"` 与 `Preview []EventPreview json:"preview,omitempty"`。
2. 在 `call` 开始时计算 `query := queryFor(tool, input)`。
3. 所有 `plugin.tool_called` 发布写入 `Query: query`。
4. 保持 `observe`、额度、缓存 key 计算顺序不变。

**验证：** `go test ./internal/plugins -run TestGovernor -count=1` 通过。

### T8 Governor 写入结果预览

**文件：** `internal/plugins/governor.go`
**依赖：** T7
**步骤：**
1. 成功路径（无缓存、缓存命中、并发合并）在 `plugin.tool_completed` 中写入 `Preview: previewFor(tool, output)`。
2. 失败路径只保留 `Query`，不写 `Preview`。
3. 缓存命中与并发合并仍保留 `Cached`/`Coalesced` 与 `ResultCount`。
4. 不改变已有事件类型、字段语义与发布顺序。

**验证：** `go test ./internal/plugins -run TestGovernor -count=1` 通过。

### T9 Governor 预览测试

**文件：** `internal/plugins/test_helpers_test.go`、`internal/plugins/governor_test.go`
**依赖：** T8
**步骤：**
1. 扩展 `stubTool`：增加 `querySummary string`、`resultPreview []EventPreview` 字段，并实现 `QuerySummary`/`ResultPreview`。
2. 在成功、缓存命中、并发合并、失败、重试测试中分别断言 `Query` 与 `Preview`。
3. 断言失败事件有 `Query` 且 `len(Preview)==0`。
4. 断言未实现预览的路径仍输出旧字段并可解析。

**验证：** `go test ./internal/plugins -count=1` 全部通过。

### T10 校验计数

**文件：** `internal/domain/validator/validator.go`、`internal/domain/validator/validator_test.go`
**依赖：** T1
**步骤：**
1. 增加 `const BaseRuleCount = 12`（C1–C12）。
2. 增加 `func CheckedCount(includesRain bool) int`：`includesRain` 时返回 13，否则 12。
3. 在测试中断言两个分支，并保证 `BaseRuleCount` 与现有检查项数量一致。

**验证：** `go test ./internal/domain/validator -run TestCheckedCount -count=1` 通过。

### T11 校验事件写入 checked

**文件：** `internal/agent/schedule.go`、`internal/agent/schedule_test.go`
**依赖：** T10
**步骤：**
1. 在 `Nodes.Validate` 中判断 `in.Research != nil && len(in.Research.Weather) > 0` 得到 `includesRain`。
2. 在 `validation.result` payload 增加 `"checked": validator.CheckedCount(includesRain)`。
3. 在测试中分别构造有/无天气的输入，断言 `checked` 为 13/12。
4. 保持 `blocking/warnings/violations` 字段不变。

**验证：** `go test ./internal/agent -run TestValidate -count=1` 通过。

### T12 版本落库内嵌解释

**文件：** `internal/runstore/runstore.go`、`internal/runstore/runstore_test.go`（新建）
**依赖：** T1
**步骤：**
1. 定义 `planSnapshot`：匿名嵌入 `domain.Itinerary`，增加 `Explanations []domain.Explanation json:"explanations,omitempty"`。
2. 增加 `encodeVersionPlan(snapshot agent.VersionSnapshot) (json.RawMessage, error)`，序列化 `planSnapshot`。
3. `SaveVersion` 改用 `encodeVersionPlan` 生成 `Plan`，其余字段与调用顺序不变。
4. 测试编码结果同时含行程字段与 `explanations`；测试无解释时 `explanations` 省略；测试 `domain.Itinerary` 可忽略该字段反序列化。
5. 测试不使用数据库，直接测试编码函数；这比 API 集成测试更直接验证落库 JSON。

**验证：** `go test ./internal/runstore -count=1` 通过。

### T13 后端回归

**文件：** 无
**依赖：** T2–T12
**步骤：**
1. 运行 `go test ./...`。
2. 运行 `go test ./... -race`。
3. 运行 `go vet ./...`。
4. 运行 `go build ./...`。

**验证：** 四条命令全部通过；记录输出摘要。

## 阶段 B：设计系统与行程模型

### T14 JS Token

**文件：** `web/src/design/tokens.ts`（新建）
**依赖：** T1
**步骤：**
1. 定义 `palette`：`paper`、`ink`、`forest`、`clay`、`sand`、`hairline`、`success`、`warning`、`danger` 的六位色值。
2. 定义 `dayThemes` 四个颜色；导出 `dayColor(index)`。
3. 定义 `fonts`（标题衬线栈、正文无衬线栈、数字特性说明）。
4. 定义 `formatYuan(value)`、`formatDateLabel(date, index)`、`formatVersion(seq)` 等纯函数。
5. 所有地图/图表颜色只从本文件读取。

**验证：** 新建 `web/src/design/tokens.test.ts`（如实现纯函数）或 `npx tsc -b` 编译通过。

### T15 CSS 与 Tailwind Token

**文件：** `web/src/index.css`、`web/tailwind.config.ts`
**依赖：** T14
**步骤：**
1. 在 `:root` 增加 `--paper`、`--ink`、`--forest`、`--clay`、`--sand`、`--hairline`、`--success`、`--warning`，并让 `--background/--foreground/--primary/--border` 指向同一套值。
2. 设置正文无衬线、标题衬线；启用 `tabular-nums`；圆角 6–10px；阴影只用于浮层。
3. Tailwind 增加 `serif` 字体族与 `paper/ink/forest/clay/sand/hairline` 颜色映射。
4. 把时间轴样式中的硬编码色值改为 `hsl(var(...))`；保留 `tp-block-*` 类名。
5. 不引入暗色模式与新字体文件。

**验证：** `cd web && npm run build` 通过；搜索六位色值，除 `tokens.ts` 与 `index.css` 外无新增命中。

### T16 基础展示组件

**文件：** `web/src/design/primitives.tsx`（新建）
**依赖：** T15
**步骤：**
1. 实现 `Surface`（纸色表面 + 可选发丝线）、`Hairline`、`SectionLabel`。
2. 实现 `Chip`（可点击/只读两种）、`Stat`（数值 + 标签）、`StatusDot`（颜色 + 文字）、`EmptyHint`。
3. 所有组件接受 `className` 并用 `cn` 合并；不写业务逻辑。
4. 触控目标不小于 44×44（对可点击 Chip/按钮）。

**验证：** `cd web && npx tsc -b` 通过。

### T17 Popover 基础组件

**文件：** `web/package.json`、`web/src/components/ui/popover.tsx`（新建）
**依赖：** T1
**步骤：**
1. 安装 `@radix-ui/react-popover`，更新 lockfile。
2. 封装 `Popover`、`PopoverTrigger`、`PopoverContent`；内容默认 portal、`align="end"`、`sideOffset=8`、浮层边框/纸色。
3. 导出 `PopoverAnchor`（如有需要）；不修改现有 Dialog。
4. 若安装失败，停止并报告，不自行改用其他依赖。

**验证：** `cd web && npm run build` 通过。

### T18 全屏 Sheet 组件

**文件：** `web/src/components/ui/sheet.tsx`（新建）
**依赖：** T17
**步骤：**
1. 基于现有 `Dialog` 实现 `Sheet`、`SheetContent`、`SheetHeader`、`SheetTitle`。
2. `SheetContent` 使用全屏定位、无圆角、内部可滚动、底部安全区 padding。
3. 保留 Radix 的焦点陷阱、Escape 关闭与 `DialogTitle` 无障碍要求。
4. 不新增抽屉依赖。

**验证：** `cd web && npm run build` 通过。

### T19 行程解释解析

**文件：** `web/src/features/itinerary/types.ts`、`web/src/features/itinerary/types.test.ts`
**依赖：** T1
**步骤：**
1. 增加 `Explanation { poi_id: string; text: string }`。
2. `Itinerary` 增加 `explanations: Explanation[]`，缺省为空数组。
3. `parseItinerary` 过滤无效解释（空 id/空 text/非字符串），同一 `poi_id` 后者覆盖前者。
4. 增加 `explanationMap(itinerary)` 返回 `Map<string,string>`。
5. 更新测试：有解释、无解释、重复 id、非法结构。

**验证：** `cd web && npm test -- src/features/itinerary/types.test.ts` 通过。

### T20 顶栏与状态组件

**文件：** `web/src/components/AppHeader.tsx`、`web/src/components/StateMessage.tsx`、`web/src/components/AppHeader.test.tsx`
**依赖：** T15、T16
**步骤：**
1. 顶栏改为 48–56px 编辑感高度：品牌衬线字、发丝线、低饱和按钮；保留登录/退出与管理员标签行为。
2. `LoadingState` 改为与最终布局同尺寸的骨架，不再以转圈为主。
3. `EmptyState` 使用 `EmptyHint`/发丝线，保留 title/description/action API。
4. `ErrorState` 保留 role 与重试按钮，视觉改为警示表面。
5. 更新测试中的可访问名称断言（不改变行为）。

**验证：** `cd web && npm test -- src/components/AppHeader.test.tsx` 通过。

### T21 前端基线回归

**文件：** 无
**依赖：** T14–T20
**步骤：**
1. 运行 `cd web && npm test`。
2. 运行 `cd web && npm run build`。

**验证：** 全部通过；若旧测试因样式选择器失败，只改断言，不改行为。

## 阶段 C：对话模型

### T22 事件解析与去重

**文件：** `web/src/features/conversation/eventParse.ts`（新建）
**依赖：** T1
**步骤：**
1. 定义 `RunEvent { seq: number; type: string; payload: Record<string, unknown> }`。
2. 实现 `asRecord`、`asString`、`asNumber`、`asStringList`，所有输入返回安全缺省。
3. 实现 `normalizeEvents(raw)`：过滤非对象、非有限 seq、seq<=0；按 seq 升序；重复 seq 只保留首次。
4. 事件类型保持 `string`，未知事件透传。

**验证：** `cd web && npx tsc -b` 通过。

### T23 运行视图推导

**文件：** `web/src/features/conversation/deriveRunView.ts`（新建）
**依赖：** T22
**步骤：**
1. 定义 `RunView`（status/stage/mode/round/attempt/lastSeq/missing/proposal/researchQuestion/violations/error/checked/draft）。
2. 移植现有 `traceReducer` 的状态迁移规则，不复制技术日志文案。
3. `run.started` 自增 attempt，记录 mode；`validation.result` 记录 checked；`plan.updated` 记录 draft=true；`run.completed` 清 draft。
4. `constraint.needs_input/parsed`、`research.*`、`repair.*`、`plan.updated`、`run.awaiting_user` 的字段映射与 plan 一致。
5. 忽略重复/倒退 seq。

**验证：** `cd web && npx tsc -b` 通过。

### T24 确定性文案

**文件：** `web/src/features/conversation/narrate.ts`（新建）
**依赖：** T23
**步骤：**
1. 实现 `constraintChips(constraints)`：城市、天数、预算、节奏、兴趣、必去、避开；缺失字段不出现。
2. 实现 `understandText(chips)`、`missingLabel`、`researchText(payload)`、`degradeText(reason)`、`validationText(checked, blocking, warnings)`、`repairText(round, blocking, dayIndex)`、`completionText(seq)`。
3. 实现 `supplementDelta(previousInput, nextInput)`：前缀相同则返回新增片段，否则返回完整新输入。
4. 实现 `jumpTargets(violations)`：只对有 `day`/`blocking` 的项生成跳转。
5. 所有文案只用传入字段；无字段时使用中性句式。

**验证：** `cd web && npx tsc -b` 通过。

### T25 对话骨架与阶段槽位

**文件：** `web/src/features/conversation/deriveConversation.ts`（新建）
**依赖：** T22、T23、T24
**步骤：**
1. 定义全部对话条目类型（plan 核心数据结构）。
2. 按 seq 升序遍历；维护 `attempt`、`slots`、`tools`、`openDecision`、`previousInput`。
3. `run.started` 生成用户消息：首次 initial→`initial`；后续 initial→`supplement` + 增量文本；adjustment→`adjust`。
4. 固定阶段槽位：`understand`、`research`、`draft`、`validate`、`repair:{round}`、`complete`；同一槽位原位更新，不重复生成。
5. 阶段消息 id 稳定：`${runId}:${firstSeq}:${slot}`。
6. 返回按首次出现 seq 排序的条目。

**验证：** `cd web && npx tsc -b` 通过。

### T26 工具卡折叠

**文件：** `web/src/features/conversation/deriveConversation.ts`
**依赖：** T25
**步骤：**
1. `plugin.tool_called` 生成 `ToolMessage`：`callKey=${name}:${call_index}`，status=running，记录 query/provider。
2. `plugin.tool_completed` 按 callKey 原位更新：status ok/error、latency、result_count、cached/coalesced/attempts、preview。
3. 同一工具多次调用生成多张卡，按 seq 排列；不做合并。
4. 未收到 completed 的调用保持 running；未知工具名显示原名。
5. 旧事件无 query/preview 时字段缺省，不报错。

**验证：** `cd web && npx tsc -b` 通过。

### T27 决策卡与结果

**文件：** `web/src/features/conversation/deriveConversation.ts`
**依赖：** T26
**步骤：**
1. `constraint.needs_input` 打开 missing 卡；`constraint.parsed` 关闭并写摘要。
2. `research.needs_input` 打开 lodging 卡；后续 `run.started` 或 `plan.updated` 关闭并写摘要。
3. `run.awaiting_user` 根据 `missing/research_question/proposal/stage` 打开 missing/lodging/proposal/confirm；`proposal_rejected` 关闭提案卡并写「已拒绝，等待修改」。
4. `run.completed` 关闭确认/提案卡为已接受；生成完成消息与正式 PlanMessage。
5. `run.cancelled` 关闭为已取消；`run.failed` 生成错误条目与 Agent 错误消息。
6. 任意时刻最多一个 `open` 决策；新决策打开前把旧决策置为 resolved。
7. 生成方案卡统计：天数、POI 数、总价、步行公里；`isDraft` 与 view 一致。

**验证：** `cd web && npx tsc -b` 通过。

### T28 画布推导

**文件：** `web/src/features/conversation/deriveCanvas.ts`（新建）
**依赖：** T19、T23
**步骤：**
1. 定义 `deriveCanvas(events, trip, viewVersion)`。
2. 若传入 `viewVersion`：解析其 plan，source=version，isDraft=false，versionSeq=viewVersion.seq。
3. 否则取当前 attempt 最新的 `plan.updated`：解析 plan/explanations，source=event，isDraft 由是否有后续 `run.completed` 决定。
4. 无事件方案时回退 `trip.latest_version`：解析 plan/explanations，source=version，isDraft=false。
5. research 从 `trip.latest_version.research` 解析；旧快照缺失返回 null。
6. 返回 `CanvasModel` 与 `poiNameMap`（供检查器/攻略使用）。

**验证：** `cd web && npx tsc -b` 通过。

### T29 模型测试：基础事件

**文件：** `web/src/features/conversation/model.test.ts`（新建）
**依赖：** T22–T28
**步骤：**
1. 建固定事件 fixture：首次规划、约束解析、候选、草稿、校验、plan.updated、awaiting、completed。
2. 断言用户消息只有一条、阶段消息数量等于阶段数、无 `#seq` 文案。
3. 断言重复 seq、乱序输入得到同一结果。
4. 断言约束 chips 与方案卡统计正确。

**验证：** `cd web && npm test -- src/features/conversation/model.test.ts` 通过。

### T30 模型测试：工具、决策与降级

**文件：** `web/src/features/conversation/model.test.ts`
**依赖：** T29
**步骤：**
1. 工具卡：called→completed 折叠、失败、缓存、并发合并、无增补字段。
2. 缺信息：needs_input→provide→run.started→constraint.parsed，断言卡片折叠与增量文本。
3. 住宿：needs_input→plan.updated，断言单卡与摘要。
4. 提案：proposal→completed、proposal→rejected、proposal→revise→新提案。
5. 降级/失败/取消：文案存在、卡片关闭、无重复消息。
6. 解释覆盖：重复 poi_id 后者胜出；旧版本无解释返回空 Map。

**验证：** `cd web && npm test -- src/features/conversation/model.test.ts` 通过。

### T31 SSE 连接 Hook

**文件：** `web/src/features/conversation/useSSEEvents.ts`（新建）
**依赖：** T22
**步骤：**
1. 复用现有 `fetchEventSource` 连接逻辑，从 `seq=0` 开始，依赖服务端重放。
2. 维护 `events`、`connected`、`lastSeq`；只接受递增 seq，重复直接丢弃。
3. 终态事件（completed/failed/cancelled）后主动断开；runId 变化时中止旧连接并重置。
4. 保留 10 秒重连间距与空闲中止；重连使用 `Last-Event-ID`。
5. 不解析业务字段，不调用推导函数。

**验证：** `cd web && npx tsc -b` 通过。

### T32 跨回合会话与动作

**文件：** `web/src/features/conversation/useSession.ts`（新建）
**依赖：** T31
**步骤：**
1. 维护 `activeRunId`、`turns`、`connected`、`pending`、`error`。
2. 同 run 的 supplement/revise 继续追加；新 adjustment/replan 把当前回合冻结进 `turns` 再订阅新 run。
3. 实现动作：supplement、revise、accept、reject、cancel、resolveResearch、adjust、replanLodging、addSystemNotice。
4. supplement/revise/accept/reject/resolveResearch 调 `resumeRun`；cancel 调 `cancelRun`；adjust 调 `createAdjustment`；replan 调 `replanTrip`。
5. adjust/replan 使用 `newClientRequestID()`；成功后切换到新 run。
6. 统一 pending key 与错误消息；失败不丢输入。
7. 暴露 `runId/turns/events/connected/pending/error/actions`。

**验证：** `cd web && npx tsc -b` 通过。

## 阶段 D：对话 UI

### T33 用户消息

**文件：** `web/src/features/conversation/UserMessage.tsx`（新建）
**依赖：** T16、T25
**步骤：**
1. 右对齐暖色气泡；显示正文、`补充/调整/首次` 标签。
2. 约束 chips 使用 `Chip`，不换行溢出。
3. 纯文本渲染，不解析 HTML。

**验证：** `cd web && npx tsc -b` 通过。

### T34 Agent 消息

**文件：** `web/src/features/conversation/AgentMessage.tsx`（新建）
**依赖：** T16、T25
**步骤：**
1. 左对齐，指南针/圆点头像 + 阶段小标签；正文 15px，最多两段。
2. `details` 可展开；展开按钮带 `aria-expanded`。
3. `jumps` 渲染为按钮，点击调用 `onJump`；键盘可操作。
4. tone 映射 info/success/warning/error 的 token 颜色。

**验证：** `cd web && npx tsc -b` 通过。

### T35 工具卡

**文件：** `web/src/features/conversation/ToolCard.tsx`（新建）
**依赖：** T16、T26
**步骤：**
1. 折叠态一行：工具中文名、查询摘要、状态、耗时、结果数、展开箭头。
2. 展开态：参数摘要、最多 3 条预览（标题/副标题/价格）、缓存/重试/并发标记。
3. 失败态显示错误消息与「不影响继续」；无预览时显示「未返回结果」。
4. 使用 `aria-expanded` 按钮；默认折叠；同一工具多次调用各自独立。

**验证：** `cd web && npx tsc -b` 通过。

### T36 决策卡：补充与住宿

**文件：** `web/src/features/conversation/DecisionCard.tsx`（新建）
**依赖：** T16、T27
**步骤：**
1. 实现 missing 卡：复用 `MissingFieldsForm`，提交调用 `onSupplement`；已处理折叠为摘要。
2. 实现 lodging 卡：列出候选名称、地址、价格/晚、评分、预订链接；每项按钮选择；提供「不计住宿」。
3. 外链新窗口 + `noopener noreferrer`；只渲染 http/https。
4. 卡片 open 时展开，resolved/cancelled 时只显示勾选/取消摘要。
5. 不在此组件发请求，只调用回调。

**验证：** `cd web && npx tsc -b` 通过。

### T37 决策卡：提案与确认

**文件：** `web/src/features/conversation/DecisionCard.tsx`
**依赖：** T36
**步骤：**
1. 实现 proposal 卡：summary、impact、warnings；接受/拒绝/取消按钮。
2. 实现 confirm 卡：摘要与提醒，接受/拒绝/取消；首次方案拒绝时进入修改态（由 Composer 承担）。
3. 所有按钮有 pending 禁用态与进行中文案。
4. 拒绝提案时文案明确「保留当前版本，可继续描述修改」。
5. 操作错误就地显示，不关闭卡片。

**验证：** `cd web && npx tsc -b` 通过。

### T38 方案卡

**文件：** `web/src/features/conversation/PlanCard.tsx`（新建）
**依赖：** T16、T27
**步骤：**
1. 四格统计：天数、景点数、步行公里、总价；版本号。
2. 草稿态显示「草稿预览 · 待确认」；正式态显示版本号。
3. 移动端显示「查看方案」按钮，调用 `onOpenCanvas`。

**验证：** `cd web && npx tsc -b` 通过。

### T39 Composer

**文件：** `web/src/features/conversation/Composer.tsx`（新建）
**依赖：** T16、T32
**步骤：**
1. 实现 `ComposerMode` 六种状态：initial、supplement、revise、adjust、locked、running、disabled。
2. 运行中显示阶段与「停止」；locked 显示「请在上方选择住宿」；disabled 说明原因。
3. supplement/revise/adjust 支持 1–4 行、Enter 发送、Shift+Enter 换行、字数上限。
4. 发送中禁用重复提交；错误就地显示且保留输入。
5. 接受 `prefill` 与 `focusToken`，用于「调整这一天」预填并聚焦。
6. `aria-label` 固定为「行程输入」；发送按钮固定名称「发送」。

**验证：** `cd web && npx tsc -b` 通过。

### T40 对话线程

**文件：** `web/src/features/conversation/ConversationRail.tsx`（新建）
**依赖：** T33–T39
**步骤：**
1. 顶部标题（城市 · 天数）、状态标签、连接状态点；不显示 trip id。
2. 线程 `role="log"`、`aria-live="polite"`；距底部 80px 内自动滚动，否则显示「新消息」按钮。
3. 按 kind 渲染用户/Agent/工具/决策/方案/系统/错误条目。
4. 隐藏 `aria-live=polite` 区域播报新 Agent 消息。
5. 断线提示条位于 Composer 上方，恢复后消失。
6. Composer 作为底部插槽传入，避免组件感知具体动作。

**验证：** `cd web && npx tsc -b` 通过。

### T41 对话组件测试

**文件：** `web/src/features/conversation/Composer.test.tsx`（新建）
**依赖：** T33–T40
**步骤：**
1. 测试 Composer 六种模式的可编辑/禁用/按钮文案。
2. 测试 Enter 发送、Shift+Enter 换行、空输入不发送、pending 禁用。
3. 测试 ToolCard 折叠/展开/失败。
4. 测试 DecisionCard missing/lodging/proposal 状态与回调。
5. 测试 PlanCard 草稿标识与移动端按钮。

**验证：** `cd web && npm test -- src/features/conversation/Composer.test.tsx` 通过。

## 阶段 E：画布

### T42 预算纯函数

**文件：** `web/src/features/budget/budgetModel.ts`（新建）、`web/src/features/budget/budgetModel.test.ts`（新建）、`web/src/features/budget/BudgetPanel.tsx`
**依赖：** T19
**步骤：**
1. 把 `BudgetBreakdown` 与 `budgetBreakdown` 移到 `budgetModel.ts`。
2. `BudgetPanel.tsx` 改为从 `budgetModel` 导入，行为不变（清理阶段删除）。
3. 补测试：门票/餐饮/交通/住宿、估算标记、无住宿。
4. 导出 `budgetSummary`（总计 + 是否含住宿）供工具栏使用。

**验证：** `cd web && npm test -- src/features/budget/budgetModel.test.ts` 通过。

### T43 预算浮层

**文件：** `web/src/features/budget/BudgetPopover.tsx`（新建）、`web/src/features/budget/BudgetPopover.test.tsx`（新建）
**依赖：** T17、T42
**步骤：**
1. 触发器显示「总计 ¥N」；浮层显示门票/餐饮/交通/住宿拆分与总计。
2. 显示估算提示；`readOnly` 时隐藏住宿口径切换。
3. 提供「计入住宿/不计住宿」开关，调用 `onToggleLodging`；pending 时禁用。
4. 无方案时显示「暂无预算」。
5. 测试开关回调与只读隐藏。

**验证：** `cd web && npm test -- src/features/budget/BudgetPopover.test.tsx` 通过。

### T44 版本菜单

**文件：** `web/src/features/versions/VersionMenu.tsx`（新建）、`web/src/features/versions/VersionMenu.test.tsx`（新建）
**依赖：** T17、T19、`diffModel`
**步骤：**
1. 查询 `listPlanVersions`；触发器显示当前版本 `vN ▾`。
2. 列表显示版本号、时间、原因、状态；点击调用 `onViewVersion(version)`。
3. 选择历史版本后显示差异摘要（新增/移动/删除数量与天/点位），差异用 `diffItineraries`。
4. 提供「恢复此版本」确认与 `restorePlanVersion`；成功后调用 `onRestored(restored)` 并关闭。
5. 提供「返回当前」；只读模式不渲染。
6. 错误就地显示，不关闭菜单。

**验证：** `cd web && npm test -- src/features/versions/VersionMenu.test.tsx` 通过。

### T45 天切换

**文件：** `web/src/features/canvas/DayTabs.tsx`（新建）
**依赖：** T14、T19
**步骤：**
1. 「全部」+ 每天一项：`D1 · 11/21 周六 · ¥320`。
2. 受控 `activeDay: number | null`，点击/键盘左右切换。
3. `role="tablist"`、roving tabindex、`aria-controls` 指向画布区域。
4. 当天花费为 0 时不显示 `¥0`。
5. 日期格式使用 token 的 `formatDateLabel`。

**验证：** `cd web && npx tsc -b` 通过。

### T46 检查器

**文件：** `web/src/features/canvas/Inspector.tsx`（新建）、`web/src/features/canvas/Inspector.test.tsx`（新建）
**依赖：** T14、T19、T42
**步骤：**
1. 默认态：天数、景点数、步行公里、总预算、住宿锚点；研究提醒一句。
2. 选中景点/餐饮：天序号、时间区间、标题、类型、时长、花费、估算标记。
3. 交通衔接：下一段方式、分钟、费用、是否直线估算。
4. 「为什么选它」：从 explanations 读取；缺失显示「本次未生成额外说明」。
5. 攻略提及：该 POI 的 note + 最多 3 条来源；外链新窗口 + `noopener noreferrer`。
6. 操作：「调整这一天」调用 `onAdjustDay(dayIndex)`；上一个/下一个点。
7. 选中住宿：名称、星级/评分、地址、每晚/合计、锚点天、预订链接；只读隐藏口径开关。
8. `region` 带可读标题；测试有/无解释、攻略、住宿。

**验证：** `cd web && npm test -- src/features/canvas/Inspector.test.tsx` 通过。

### T47 地图编号与住宿

**文件：** `web/src/features/map/TripMap.tsx`
**依赖：** T14、T19
**步骤：**
1. 使用 `dayColor` 替代本地 `dayColors`。
2. POI/餐饮按天顺序编号 1..N；交通/休息不编号；住宿标记显示「宿」。
3. 标记内容只使用数字与转义后的标题，不渲染事件文本/HTML。
4. 点击标记调用 `onSelectBlock`；点击住宿调用 `onSelectHotel`。
5. 移除 InfoWindow 创建与打开；详情统一由检查器展示。
6. 保留交通折线与按天颜色；选中标记 zIndex 提升。

**验证：** `cd web && npx tsc -b` 通过。

### T48 地图天过滤与选中

**文件：** `web/src/features/map/TripMap.tsx`、`web/src/features/map/TripMap.test.tsx`
**依赖：** T47
**步骤：**
1. 增加 `activeDay: number | null`、`selectedHotel` 属性。
2. activeDay 非空时只显示该天标记/折线 + 住宿；其他天弱化或隐藏。
3. activeDay/选中变化时 `fitView` 到该天点位 + 住宿；全部时 fit 所有覆盖物。
4. 用 `ResizeObserver` 调用 `map.resize()`；窗口变化防抖。
5. 选中变化只更新 zIndex/样式，不重建覆盖物；仅在 itinerary/activeDay 变化时重建。
6. 无 Key 降级改为按天分组的可点击文字清单，保留配置提示；测试降级与选点回调。

**验证：** `cd web && npm test -- src/features/map/TripMap.test.tsx` 通过。

### T49 时间轴模型

**文件：** `web/src/features/timeline/timelineModel.ts`、`web/src/features/timeline/timelineModel.test.ts`（新建）
**依赖：** T19
**步骤：**
1. `buildTimelineModel(itinerary, activeDay?)`：activeDay 非空时只输出该天组与条目。
2. 为每个条目增加 `dayIndex`、`number`（景点/餐饮编号；交通/休息为 null）。
3. 标题/内容继续转义；交通/餐饮/休息类名不变。
4. 测试全部/单天、编号、空天、转义。

**验证：** `cd web && npm test -- src/features/timeline/timelineModel.test.ts` 通过。

### T50 时间轴组件

**文件：** `web/src/features/timeline/ItineraryTimeline.tsx`、`web/src/features/timeline/ItineraryTimeline.test.tsx`
**依赖：** T49
**步骤：**
1. 增加 `activeDay` 属性，传给 `buildTimelineModel`；activeDay 变化时 `setData` 并 `fit` 该天。
2. 选中只调用 `setSelection`，不重建数据。
3. 样式 token 化：13px、交通虚线、餐饮暖色、休息紫色；选中描边。
4. 点击条目继续回传 block id；保留可缩放/平移。
5. 更新测试断言。

**验证：** `cd web && npm test -- src/features/timeline/ItineraryTimeline.test.tsx` 通过。

### T51 窄屏纵向清单

**文件：** `web/src/features/canvas/DayList.tsx`（新建）
**依赖：** T19、T49
**步骤：**
1. 显示每天分组与块：时间、类型、标题、花费。
2. 支持 `activeDay` 过滤、选中高亮、点击回调。
3. 空方案显示 `EmptyHint`；无横向滚动。
4. 使用按钮/列表语义，键盘可选中。

**验证：** `cd web && npx tsc -b` 通过。

### T52 画布编排

**文件：** `web/src/features/canvas/PlanCanvas.tsx`（新建）、`web/src/features/canvas/PlanCanvas.test.tsx`（新建）
**依赖：** T45、T46、T48、T50
**步骤：**
1. 属性：itinerary、explanations、research、activeDay、selection、readOnly、isDraft、versionSeq、onActiveDayChange、onSelect、onSelectHotel、onAdjustDay、toolbar 插槽。
2. 桌面布局：工具栏 → DayTabs → 地图 → 时间轴；检查器作为右侧区域（或浮层由外层决定）。
3. 无方案时显示引导与骨架，不显示空白。
4. `readOnly` 隐藏调整/口径/版本/分享相关操作。
5. 组合地图/时间轴选中同步；选中状态由父层传入。
6. 测试：有方案渲染、无方案引导、只读隐藏操作。

**验证：** `cd web && npm test -- src/features/canvas/PlanCanvas.test.tsx` 通过。

### T53 画布工具栏

**文件：** `web/src/features/canvas/CanvasToolbar.tsx`（新建）
**依赖：** T43、T44、T45
**步骤：**
1. 左侧：标题/城市·天数、状态、草稿标识。
2. 中间：DayTabs。
3. 右侧：BudgetPopover、VersionMenu、ShareDialog（插槽）。
4. 只读模式隐藏版本/分享/预算开关；窄屏「更多」收纳。
5. 工具栏高度稳定，不因草稿标识出现而跳动。

**验证：** `cd web && npx tsc -b` 通过。

### T54 移动端全屏画布

**文件：** `web/src/features/canvas/CanvasSheet.tsx`（新建）
**依赖：** T18、T45、T46、T48、T51
**步骤：**
1. 基于 `Sheet` 全屏打开；顶部标题、关闭、DayTabs；底部安全区。
2. 三页签：地图 / 时间轴 / 详情；地图选点后显示底部预览卡，「查看详情」切到详情。
3. 预算/版本/分享收进「更多」；各面板独立滚动。
4. 关闭后回到对话原位置（不重置对话滚动）。
5. 打开时焦点进入面板，Escape 关闭。

**验证：** `cd web && npx tsc -b` 通过。

### T55 画布测试汇总

**文件：** `web/src/features/canvas/PlanCanvas.test.tsx`、`Inspector.test.tsx`、`DayTabs`/`DayList` 测试（如需要）
**依赖：** T45–T54
**步骤：**
1. 测试天切换与画布选中同步。
2. 测试检查器有/无解释、攻略外链。
3. 测试只读模式隐藏调整与口径。
4. 测试移动端 Sheet 打开/关闭与页签。
5. 测试预算/版本触发器存在且可打开。

**验证：** `cd web && npm test -- src/features/canvas` 通过。

## 阶段 F：工作台与页面

### T56 工作台布局

**文件：** `web/src/features/workspace/WorkspacePage.tsx`
**依赖：** T40、T52、T53、T54
**步骤：**
1. 使用 `useTrip`、`useSession`、`deriveRunView`、`deriveConversation`、`deriveCanvas`。
2. 状态：activeDay、selection、viewVersion、canvasOpen。
3. ≥1280：对话 400px + 画布（地图 + 检查器）；1024–1279：对话 360px + 画布浮层检查器；<1024：对话全宽 + 吸底输入 + 顶部「查看方案」。
4. 移除旧 `RunTrace`、`RunActions`、`ResearchPanel`、`BudgetPanel`、`VersionPanel` 引用。
5. 保留加载/未授权/404 错误分支；不显示 trip id。
6. 终态时刷新 trip；接受/恢复后刷新 versions。

**验证：** `cd web && npx tsc -b` 通过（旧测试此时预期失败，T58 更新）。

### T57 动作与跳转接线

**文件：** `web/src/features/workspace/WorkspacePage.tsx`
**依赖：** T56
**步骤：**
1. 由 `view` 计算 ComposerMode：running/queued、locked、supplement、revise、adjust、disabled。
2. 接线 supplement/revise/accept/reject/cancel/resolveResearch/adjust/replanLodging。
3. Agent 消息 jumps 与检查器「调整这一天」设置 activeDay/selection 并聚焦 Composer。
4. 版本菜单选择设置 viewVersion；恢复成功刷新 trip/versions 并添加系统消息。
5. 预算切换调用 `replanLodging`，生成新回合。
6. 分享按钮放入工具栏；移动端「更多」。
7. 错误统一显示在 Composer/决策卡内，不丢输入。

**验证：** `cd web && npx tsc -b` 通过。

### T58 工作台测试

**文件：** `web/src/features/workspace/WorkspacePage.test.tsx`
**依赖：** T56、T57
**步骤：**
1. 重写 mock：`useSession` 或 `useSSEEvents` + API、SSE fixture；mock 画布子组件以聚焦编排。
2. 断言对话用户消息、Agent 消息、工具卡、决策卡出现。
3. 断言地图/时间轴/检查器选中同步。
4. 断言移动端「查看方案」打开 Sheet，桌面不显示该按钮。
5. 断言预算与版本触发器存在；只读分享页不渲染。
6. 保留未授权/404 分支测试（如已有）。

**验证：** `cd web && npm test -- src/features/workspace/WorkspacePage.test.tsx` 通过。

### T59 创建页与表单

**文件：** `web/src/features/create/CreatePage.tsx`、`web/src/features/create/CreateForm.tsx`
**依赖：** T15、T16、T17
**步骤：**
1. 页面改为桌面双栏：左侧标题/说明/示例，右侧输入；窄屏纵向。
2. 表单保留单文本输入、示例标签、偏好标签；移除常驻大表单。
3. 偏好标签打开 `PreferenceSheet`；已设置数量显示在标签上。
4. 保留提交逻辑、`client_request_id`、错误就地显示、输入不丢失。
5. 保留可访问名称「行程需求」「开始规划」。

**验证：** `cd web && npx tsc -b` 通过。

### T60 偏好弹窗

**文件：** `web/src/features/create/PreferenceSheet.tsx`（新建）
**依赖：** T18、`composeInput`
**步骤：**
1. 使用 `Sheet` 或 Dialog 展示天数、节奏、人数、预算、每日上限、兴趣、必去、避开、用餐。
2. 受控 `AdvancedOptions`，修改后回写父组件；清空恢复默认。
3. 复用 `composeInput` 的语义，不改变提交格式。
4. 关闭时保留已填值；Escape/焦点管理由基础组件负责。

**验证：** `cd web && npx tsc -b` 通过。

### T61 创建表单测试

**文件：** `web/src/features/create/CreateForm.test.tsx`
**依赖：** T59、T60
**步骤：**
1. 更新选择器以匹配新布局，保留输入/提交/错误断言。
2. 测试示例一键填入、偏好弹窗修改后提交内容含附加约束。
3. 测试提交失败显示错误且输入保留。
4. 测试空输入不提交。

**验证：** `cd web && npm test -- src/features/create/CreateForm.test.tsx` 通过。

### T62 分享页与摘要

**文件：** `web/src/features/share/SharePage.tsx`、`web/src/features/research/ResearchNotes.tsx`（新建）、`web/src/features/share/SharePage.test.tsx`
**依赖：** T46、T52、T19、`ResearchNotes`
**步骤：**
1. 分享页标题区：城市·天数、总预算、AI 生成、只读、复制文本、「我也来规划一版」。
2. 复用 `PlanCanvas` 并传 `readOnly`；不显示版本菜单、分享按钮、调整操作。
3. 新增「旅途中值得知道」：住宿摘要 + 攻略提及/来源 + tips；旧快照缺失显示降级说明。
4. 预算摘要只读；显示快照版本号作为来源。
5. 复制文本继续使用 `itineraryToPlainText`。
6. 更新测试：只读、解释、攻略、无运行轨迹/版本操作。

**验证：** `cd web && npm test -- src/features/share/SharePage.test.tsx` 通过。

### T63 分享弹窗、历史、登录、评测

**文件：** `web/src/features/share/ShareDialog.tsx`、`ShareDialog.test.tsx`、`web/src/features/trips/MyTripList.tsx`、`MyTripList.test.tsx`、`web/src/features/auth/LoginPage.tsx`、`web/src/features/eval/EvalDashboard.tsx`、`EvalDashboard.test.tsx`
**依赖：** T15、T16
**步骤：**
1. 分享弹窗仅重做视觉，保留生成/复制/错误行为与可访问名称。
2. 历史列表改为行式编辑布局；状态用「点 + 文字」；保留空/错/加载。
3. 登录页使用品牌 + 单一登录按钮；保留 `return_to` 与错误映射。
4. 评测面板只替换 token 类名，不改信息结构/功能。
5. 更新受样式影响的测试断言。

**验证：** `cd web && npm test -- src/features/share src/features/trips src/features/auth src/features/eval` 通过。

### T64 页面样式回归

**文件：** 无
**依赖：** T59–T63
**步骤：**
1. 运行 `cd web && npm test`。
2. 运行 `cd web && npm run build`。
3. 检查 390/768/1024/1440 下无横向滚动（开发服务器 + 浏览器）。

**验证：** 测试与构建通过；四档宽度无横向滚动。

## 阶段 G：清理、端到端与验收

### T65 删除旧组件

**文件：** 删除清单中的旧组件与测试
**依赖：** T56–T64
**步骤：**
1. 删除 `features/trace/*`、`RunActions.tsx`、`ResearchPanel.tsx`、`BudgetModeToggle.tsx`、`BudgetPanel.tsx`、`VersionPanel.tsx`、`VersionSwitcher.tsx`、`PlanDiff.tsx` 及其测试。
2. 删除 `hooks/useRunStream.ts`。
3. 全局搜索并修复残留 import。
4. 确认 `MissingFieldsForm`、`itineraryToPlainText`、`diffModel`、`composeInput` 仍被使用。

**验证：** `cd web && npm test` 与 `npm run build` 通过；无残留 import。

### T66 Playwright 端到端

**文件：** `web/e2e/golden.spec.ts`
**依赖：** T65
**步骤：**
1. 重写 mock SSE：用户消息、约束、工具卡、研究、校验、plan.updated、awaiting、completed、proposal。
2. 主流程：创建 → 对话出现 → 工具卡展开 → 接受 → 画布选点 → 检查器理由 → 调整提案 → 接受 → 分享弹窗/分享页。
3. 移动端 390px：对话优先、输入吸底、「查看方案」全屏 Sheet、页签切换、无横向滚动。
4. 无 Key 地图降级：文字清单可点。
5. 旧事件无 query/preview：工具卡降级不报错。

**验证：** `cd web && npm run test:e2e -- e2e/golden.spec.ts` 通过。

### T67 全量回归

**文件：** 无
**依赖：** T66
**步骤：**
1. `go test ./... -race`
2. `go vet ./...`
3. `go build ./...`
4. `cd web && npm test`
5. `cd web && npm run build`
6. `cd web && npm run test:e2e`

**验证：** 全部通过，记录每条命令结果。

### T68 手工验收准备

**文件：** 无
**依赖：** T67
**步骤：**
1. 用真实 `VITE_AMAP_JS_KEY` 启动独立前端/后端实例（不重启用户 8080 进程）。
2. 准备旧版本数据（无 explanations/无增补事件）与插件关闭配置。
3. 列出待执行的 checklist 场景；不在此任务中修改代码。

**验证：** 开发服务器可访问；旧数据与降级路径可复现；记录端口与操作步骤。

## 执行顺序

```
T1
 ├─ T2 → T3 → T4 → T5 → T6
 │        └────────────→ T7 → T8 → T9
 ├─ T10 → T11
 ├─ T12
 └─ T13（后端收口）
T14 → T15 → T16
T17 → T18
T19 → T20 → T21
T22 → T23 → T24 → T25 → T26 → T27 → T28 → T29 → T30
T31 → T32
T33 → T34 → T35 → T36 → T37 → T38 → T39 → T40 → T41
T42 → T43
T44
T45 → T46 → T47 → T48 → T49 → T50 → T51 → T52 → T53 → T54 → T55
T56 → T57 → T58
T59 → T60 → T61
T62 → T63 → T64
T65 → T66 → T67 → T68
```

可并行：T14–T18 与 T2–T12；T42/T44 与 T45–T51；T59–T63 内部可按页面并行，但需在 T64 前收口。
