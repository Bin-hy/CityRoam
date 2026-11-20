# TripPilot v1.6 结果页三视图 Tasks

状态：草稿（待审批）
日期：2026-11-20

> 所有命令在 `web/` 下执行；本次不自动提交代码，用户明确要求后再提交。
> T19–T27 期间旧组件仍引用旧接口，`tsc -b` 会失败属预期；各任务以目标测试为证据，T28 清理后恢复全绿。

## 文件清单

| 操作 | 文件 | 职责 |
|------|------|------|
| 新建 | `web/src/features/result/model.ts` | 结果模型、派生、选中解析、文案表 |
| 新建 | `web/src/features/result/model.test.ts` | 派生与降级单测 |
| 新建 | `web/src/features/map/routeStyle.ts` | 路段样式、强调、图例 |
| 新建 | `web/src/features/map/routeStyle.test.ts` | 样式单测 |
| 新建 | `web/src/features/result/StopDetail.tsx` | 停靠点事实、理由、攻略区块 |
| 新建 | `web/src/features/result/StopDetail.test.tsx` | 文案、缺失、链接安全 |
| 新建 | `web/src/features/result/LegRow.tsx` | 路段摘要行 |
| 新建 | `web/src/features/result/LegDetail.tsx` | 路段详情 |
| 新建 | `web/src/features/result/LegDetail.test.tsx` | 分段、估算、缺坐标 |
| 新建 | `web/src/features/result/DetailCard.tsx` | 桌面浮层 / 移动底部详情，含住宿 |
| 新建 | `web/src/features/result/DetailCard.test.tsx` | 三类目标、关闭、预订链接 |
| 新建 | `web/src/features/result/DayMiniMap.tsx` | 当天 SVG 小图 |
| 新建 | `web/src/features/result/DayMiniMap.test.tsx` | 几何、强调、无坐标降级 |
| 新建 | `web/src/features/result/DayChapter.tsx` | 单天章节 |
| 新建 | `web/src/features/result/DossierView.tsx` | 手册视图 |
| 新建 | `web/src/features/result/DossierView.test.tsx` | 概览、顺序、留白、来源、分享态 |
| 新建 | `web/src/features/result/ViewSwitcher.tsx` | 三视图切换 |
| 新建 | `web/src/features/result/ViewSwitcher.test.tsx` | 键盘与选中态 |
| 新建 | `web/src/features/result/DayTabs.tsx` | 由 canvas 迁入，新增 `allowAll` |
| 新建 | `web/src/features/result/DayTabs.test.tsx` | 全部/单天、键盘、无「全部」 |
| 新建 | `web/src/features/result/ResultToolbar.tsx` | 标题、切换器、天 tabs、控制槽 |
| 新建 | `web/src/features/result/StoryColumn.tsx` | 地图旁剧本列 |
| 新建 | `web/src/features/result/MapView.tsx` | 单日地图 + 剧本 |
| 新建 | `web/src/features/result/MapView.test.tsx` | 单日、同步、详情、降级 |
| 新建 | `web/src/features/result/BoardView.tsx` | 看板视图 |
| 新建 | `web/src/features/result/BoardView.test.tsx` | 指标、列、留白、详情 |
| 新建 | `web/src/features/result/ResultShell.tsx` | 结果外壳 |
| 新建 | `web/src/features/result/ResultShell.test.tsx` | 默认、切换、状态保持 |
| 新建 | `web/src/features/result/ResultSheet.tsx` | 移动端方案面板 |
| 新建 | `web/src/features/result/ResultSheet.test.tsx` | 视图切换、关闭 |
| 修改 | `web/src/features/map/TripMap.tsx` | 单日接口与路段交互 |
| 修改 | `web/src/features/map/TripMap.test.tsx` | 新接口、折线点击、降级 |
| 修改 | `web/src/features/budget/BudgetPopover.tsx` | 接收 `ResultBudget` |
| 修改 | `web/src/features/budget/BudgetPopover.test.tsx` | 新 props |
| 修改 | `web/src/features/workspace/WorkspacePage.tsx` | 接入新外壳、视图、选中 |
| 修改 | `web/src/features/workspace/WorkspacePage.test.tsx` | 集成与状态保持 |
| 修改 | `web/src/features/share/SharePage.tsx` | 只读手册 |
| 修改 | `web/src/features/share/SharePage.test.tsx` | 无切换器、无地图 |
| 修改 | `web/src/index.css` | 删除 `.vis-*` 样式 |
| 修改 | `web/package.json`、`package-lock.json` | 移除 `vis-timeline`、`vis-data` |
| 删除 | `web/src/features/canvas/` | 旧画布组件与测试 |
| 删除 | `web/src/features/timeline/` | 旧时间轴与测试 |

## T1: 建立结果模型类型与文案表

**文件：** `web/src/features/result/model.ts`
**依赖：** 无
**步骤：**
1. 定义 `ResultViewMode`、`ResultSelection`、`ResultTarget`、`ResultModel`、`ResultOverview`、`ResultPace`、`ResultBudget`、`ResultDay`、`ResultStop`、`ResultPoint`、`ResultLeg`、`ResultLegSegment`、`ResultGap`、`ResultReminder`、`ResultLodging`、`ResultHotel`、`ResultGuide`、`ResultCitation`、`ResultInput`，字段与 plan 一致。
2. 导出 `modeLabels`、`paceLabels`、`typeLabels`。
3. 暂不实现函数，只留类型与常量。

**验证：** `npx tsc -b --pretty false` 退出码 0。

## T2: 派生停靠点与理由/攻略

**文件：** `web/src/features/result/model.ts`
**依赖：** T1
**步骤：**
1. 内部 `toStops(day, dayIndex, explanations, research)`：按源顺序保留 `poi / meal / rest`，生成 `ResultStop`，`order` 从 0 递增。
2. 理由从 `explanations` 按 `block.poi_id` 取值；攻略从 `research.guides` 按 `poi_id` 过滤，引用链接经 `safeURL`。
3. 实现内部 `safeURL(value)`：仅 http/https，其余返回 `undefined`；引用缺标题/URL 时跳过该条。
4. 无 `poi_id` 的停靠点 `reason: null`、`guides: []`。

**验证：** `npx tsc -b --pretty false` 退出码 0。

## T3: 派生路段与分段

**文件：** `web/src/features/result/model.ts`
**依赖：** T2
**步骤：**
1. 内部 `toLegs(day, stops, dayIndex)`：相邻停靠点之间取前一块 `transport`，否则取两者之间首个带 `transport` 的 `transit` 块。
2. `transport.legs` 存在时逐条映射为 `ResultLegSegment`；否则用一个分段承载整段。
3. 生成 `id = \`${dayIndex}:${from.id}>${to.id}\`` 与 `from/to` 的 `ResultPoint`（复制坐标与标题）。
4. 计算 `geometry`：任一分段有折线 → `polyline`；否则首尾坐标齐全 → `straight`；否则 `none`。
5. 汇总 `durationMin / distanceM / costYuan / estimated`；`mode` 取 transport.mode，`modeLabel` 查表，整段为空时回落「交通」。

**验证：** `npx tsc -b --pretty false` 退出码 0。

## T4: 派生留白与提醒

**文件：** `web/src/features/result/model.ts`
**依赖：** T3
**步骤：**
1. 实现 `parseHHMM` 与分钟差；时间无法解析时返回 `null`。
2. 计算「开门→首站、站间、末站→收尾」的空档，只保留 ≥ 45 分钟者；生成 `ResultGap`，`id = \`${dayIndex}:gap:${start}\``。
3. 建议规则：跨 11:30–13:30 → 午饭；跨 17:30–19:30 → 晚饭；开始 ≥ 20:00 → 回住宿休息；≥ 120 分钟 → 自由活动；其余 → 休息。
4. 提醒从 `research.notes` 与 `research.warnings` 依次取非空、去重，生成 `ResultReminder`，`id = \`research:${index}\``。

**验证：** `npx tsc -b --pretty false` 退出码 0。

## T5: 组装顶层模型

**文件：** `web/src/features/result/model.ts`
**依赖：** T4
**步骤：**
1. 实现 `buildResultModel(input)`：`itinerary` 为空返回 `null`。
2. 城市中文名（`beijing → 北京`）、`title = \`${city} · ${days} 天\``、`summary` 取前 4 个不重复停靠点标题用「、」与「与」连接，超过 4 个加「等」。
3. `overview`：`days`、`poiCount = totals.poi_count`、`walkingKM = totals.walking_km`、`pace` 按阈值规则。
4. `budget` 复用 `budgetBreakdown`；`lodgingIncluded` 取 `lodging.include_in_budget`。
5. `lodging` 映射为 `ResultLodging`（`bookingURL` 经 `safeURL`）。
6. 每天 `label` 用 `formatDateLabel`，`theme` 取前三个停靠点标题以「 · 」连接，空则「第 N 天」。

**验证：** `npx tsc -b --pretty false` 退出码 0。

## T6: 实现选中解析

**文件：** `web/src/features/result/model.ts`
**依赖：** T5
**步骤：**
1. `resolveResultSelection(model, selection)`：`stop` 按天与 id 查找；`leg` 按天与 id 查找；`hotel` 返回 `lodging`。
2. 任一层找不到返回 `null`，不抛错。
3. 导出函数签名与 plan 一致。

**验证：** `npx tsc -b --pretty false` 退出码 0。

## T7: 模型测试：停靠点与路段

**文件：** `web/src/features/result/model.test.ts`
**依赖：** T6
**步骤：**
1. 建固定样例：2 天，含 poi、meal、transit 块、`transport.legs` 折线、解释、攻略与住宿。
2. 断言 `transit` 不进入 `stops`，相邻站点生成 `legs`。
3. 断言分段映射、`geometry` 三态、`id` 稳定、排序不变。
4. 断言理由/攻略按 `poi_id` 归位，`javascript:` 与非法 URL 被丢弃。

**验证：** `npx vitest run src/features/result/model.test.ts` 全绿。

## T8: 模型测试：留白、预算、降级、选中

**文件：** `web/src/features/result/model.test.ts`
**依赖：** T7
**步骤：**
1. 断言 ≥ 45 分钟才生成留白，建议按时间窗口正确。
2. 断言 `budget` 与 `budgetBreakdown` 一致，`lodgingIncluded` 正确。
3. 空方案返回 `null`；缺解释/攻略/坐标/住宿时不抛错且字段为默认值。
4. 断言 `resolveResultSelection` 对三类目标与失效目标的结果。

**验证：** `npx vitest run src/features/result/model.test.ts` 全绿。

## T9: 路线样式与测试

**文件：** `web/src/features/map/routeStyle.ts`、`web/src/features/map/routeStyle.test.ts`
**依赖：** T1
**步骤：**
1. 实现 `routeStyle(mode, lineName, emphasis)`：步行 `#64748b` 虚线、公交 `#2563eb`、地铁 `#f97316`、驾车 `#7c3aed`、未知 `#94a3b8` 虚线；`transit` 按线路名含「地铁」判定地铁。
2. `selected`：weight 6、opacity 1；`muted`：weight 4、opacity 0.25。
3. 导出 `modeLegend`（步行、公交、地铁、驾车）。
4. 测试各模式颜色/虚线、强调、未知模式回退、图例项。

**验证：** `npx vitest run src/features/map/routeStyle.test.ts` 全绿。

## T10: 停靠点详情区块与测试

**文件：** `web/src/features/result/StopDetail.tsx`、`web/src/features/result/StopDetail.test.tsx`
**依赖：** T2
**步骤：**
1. 导出 `StopFacts`（时长、花费）、`ReasonSection`（为什么选它 / 本次未生成额外说明）、`GuideSection`（攻略提及 / 暂无攻略提及 / 引用站点与日期）、`StopDetail`（组合 + 调整按钮）。
2. 链接仅 http/https 渲染为 `<a target="_blank" rel="noopener noreferrer">`；其余按纯文本。
3. 测试缺失理由、缺失攻略、含 HTML 文本按纯文本、`javascript:` 不可点击、调整回调。

**验证：** `npx vitest run src/features/result/StopDetail.test.tsx` 全绿。

## T11: 路段行、路段详情与测试

**文件：** `web/src/features/result/LegRow.tsx`、`web/src/features/result/LegDetail.tsx`、`web/src/features/result/LegDetail.test.tsx`
**依赖：** T3、T9
**步骤：**
1. `LegRow`：方式、线路名、耗时、花费、估算标记；`aria-pressed` 表选中。
2. `LegDetail`：分段列表（方式、线路、上/下车站、距离、耗时、花费）、整段估算说明、「按实际路线绘制 / 直线估算，仅供距离参考 / 缺少坐标，仅显示文字」、「查看路线图」按钮。
3. 测试分段渲染、估算文案、缺坐标文案、无按钮时不渲染。

**验证：** `npx vitest run src/features/result/LegDetail.test.tsx` 全绿。

## T12: 详情卡（含住宿）与测试

**文件：** `web/src/features/result/DetailCard.tsx`、`web/src/features/result/DetailCard.test.tsx`
**依赖：** T10、T11
**步骤：**
1. 按 `target.kind` 渲染 `StopDetail` / `LegDetail` / 住宿详情（星级、评分、地址、晚数、锚点、预订链接、计入预算开关）。
2. 桌面用 `role="dialog"` 浮层，Esc 关闭；移动端用底部 Sheet；关闭回调 `onClose`。
3. 测试三类目标、Esc 与关闭按钮、预订链接协议过滤、调整回调透传。

**验证：** `npx vitest run src/features/result/DetailCard.test.tsx` 全绿。

## T13: 当天小图与测试

**文件：** `web/src/features/result/DayMiniMap.tsx`、`web/src/features/result/DayMiniMap.test.tsx`
**依赖：** T2、T3、T9
**步骤：**
1. 计算当天坐标范围，等距投影（经度 × cos 纬度）并留边距；生成 SVG 宽高与点坐标。
2. 绘制 `legs`：`polyline` 按分段、`straight` 虚线、`none` 跳过；样式来自 `routeStyle`，选中加粗、其余淡化。
3. 绘制编号站点与酒店锚点（传入才画）；点击站点/路段调用 `onSelect`。
4. 无任何坐标时渲染停靠点清单与「缺少坐标，无法绘制当天路线」；底部「查看路线图」调用 `onViewOnMap`。
5. 测试投影与元素数量、点击回调、选中样式、降级与按钮。

**验证：** `npx vitest run src/features/result/DayMiniMap.test.tsx` 全绿。

## T14: 每日章节

**文件：** `web/src/features/result/DayChapter.tsx`
**依赖：** T10、T11、T13
**步骤：**
1. 章节头：第 N 天、日期、主题、起止时间、当天合计、调整按钮（只出现一次）。
2. 按「停靠点 → 通往下一站的路段」顺序渲染；停靠点内联 `StopFacts / ReasonSection / GuideSection`。
3. 路段用 `LegRow`，点击选中并展开 `LegDetail`；小图放在章节头下方。
4. 末尾渲染 `gaps`（「未排」+ 建议）与当天合计。

**验证：** `npx tsc -b --pretty false`（此阶段仍应通过）。

## T15: 手册视图与测试

**文件：** `web/src/features/result/DossierView.tsx`、`web/src/features/result/DossierView.test.tsx`
**依赖：** T14
**步骤：**
1. 概览：标题、摘要、天数 / 景点 / 步行 / 预算 / 节奏。
2. `activeDay` 非空只渲染该天，为空渲染全部；切天后滚动到该天顶部（章节 ref + 容器 `scrollTo`）。
3. 收尾：住宿摘要与提醒；无提醒不渲染；`readOnly` 时不显示调整与「查看路线图」。
4. 测试章节顺序、理由/来源、留白、住宿摘要、筛选、readOnly。

**验证：** `npx vitest run src/features/result/DossierView.test.tsx` 全绿。

## T16: 视图切换器与测试

**文件：** `web/src/features/result/ViewSwitcher.tsx`、`web/src/features/result/ViewSwitcher.test.tsx`
**依赖：** T1
**步骤：**
1. 三段按钮，`role="tablist"`，`aria-selected`，左右方向键切换，焦点环。
2. 测试点击、键盘、当前项。

**验证：** `npx vitest run src/features/result/ViewSwitcher.test.tsx` 全绿。

## T17: 迁移天切换与测试

**文件：** `web/src/features/result/DayTabs.tsx`、`web/src/features/result/DayTabs.test.tsx`
**依赖：** T1
**步骤：**
1. 从 `canvas/DayTabs.tsx` 复制实现到新路径，新增 `allowAll`（默认 `true`）。
2. `allowAll=false` 时不渲染「全部」，键盘在具体天之间循环。
3. 测试标签含日期与金额、`allowAll` 两种形态、键盘 Home/End/方向键。

**验证：** `npx vitest run src/features/result/DayTabs.test.tsx` 全绿。

## T18: 结果工具栏

**文件：** `web/src/features/result/ResultToolbar.tsx`
**依赖：** T16、T17
**步骤：**
1. 标题、状态点、草稿徽标、`ViewSwitcher`、可选 `DayTabs`（`showDayTabs` / `allowAll`）。
2. 右侧 `controls` 槽与移动端「更多」弹层；`actionError` 以 `role="alert"` 就地显示。
3. `readOnly` 时不渲染分享与调整入口。

**验证：** `npx tsc -b --pretty false`（此阶段仍应通过）。

## T19: 地图改造与测试

**文件：** `web/src/features/map/TripMap.tsx`、`web/src/features/map/TripMap.test.tsx`
**依赖：** T3、T9
**步骤：**
1. props 改为 `day / hotel / selectedStopID / selectedLegID / onSelectStop / onSelectLeg / onSelectHotel`。
2. 标记取 `day.stops` 中有坐标者，景点/餐饮编号；酒店标记仅在传入时创建。
3. 路段取 `day.legs`：有折线逐分段绘制，`straight` 画虚线直线，`none` 跳过；折线 `click` 选路段，`bubble` 关闭。
4. 选中：路段 `selected` / 其余 `muted`；标记放大置顶；选中段 `setFitView` 该段，否则当天；不弹信息窗。
5. 降级清单改为当天停靠点与路段（均可点选）；图例只列方式、直线估算与 GCJ-02。
6. 重写测试：单日标记、折线样式与点击、选中强调、直线与跳过、无 Key 降级、标题转义。

**验证：** `npx vitest run src/features/map/TripMap.test.tsx` 全绿。

## T20: 地图视图、剧本列与测试

**文件：** `web/src/features/result/StoryColumn.tsx`、`web/src/features/result/MapView.tsx`、`web/src/features/result/MapView.test.tsx`
**依赖：** T12、T19
**步骤：**
1. `StoryColumn`：按时间列出停靠点与路段，展开 `StopDetail` / `LegDetail`；选中滚动进视野。
2. `MapView`：只取 `model.days[activeDay]` 传给 `TripMap`；酒店仅在 `anchorDays` 命中时传入。
3. 选中停靠点/路段时浮出 `DetailCard`；切换天时清空跨天选中（由 WorkspacePage 守卫，视图只上报）。
4. 测试（mock `TripMap`）：只渲染当天、剧本点选同步、详情出现、无选中不浮层。

**验证：** `npx vitest run src/features/result/MapView.test.tsx` 全绿。

## T21: 看板视图与测试

**文件：** `web/src/features/result/BoardView.tsx`、`web/src/features/result/BoardView.test.tsx`
**依赖：** T10、T11、T12
**步骤：**
1. 指标行：总预算、住宿、门票、步行、景点数，全部取 `model`。
2. 每天一列，列头日期与合计；卡内时间、标题、花费、一句理由；卡间插入 `LegRow`。
3. 留白卡与提醒卡进入对应天列；选中卡浮出 `DetailCard`，移动端用底部 Sheet。
4. 移动端按天折叠（原生 `details / summary`），默认展开 `activeDay`。
5. 测试指标、列合计、理由缺失文案、留白、点选详情、调整入口。

**验证：** `npx vitest run src/features/result/BoardView.test.tsx` 全绿。

## T22: 结果外壳与测试

**文件：** `web/src/features/result/ResultShell.tsx`、`web/src/features/result/ResultShell.test.tsx`
**依赖：** T18、T20、T21
**步骤：**
1. 组装 `ResultToolbar` + 当前视图；`model` 为空显示空状态。
2. `viewOnMap(dayIndex)`：设天后切地图；切地图时无有效天落到第 1 天。
3. 清理选中对象失效（由工作台守卫，外壳只暴露回调）。
4. 测试默认视图由 props 决定、切换保留天/选中、`viewOnMap`、空状态。

**验证：** `npx vitest run src/features/result/ResultShell.test.tsx` 全绿。

## T23: 移动端面板与测试

**文件：** `web/src/features/result/ResultSheet.tsx`、`web/src/features/result/ResultSheet.test.tsx`
**依赖：** T22
**步骤：**
1. `SheetHeader` 内放 `ViewSwitcher` 与 `DayTabs`（吸顶），下方渲染 `ResultShell`。
2. 地图视图填满面板；手册一天一屏；看板折叠。
3. 测试打开/关闭、视图切换、关闭后回调、无横向滚动容器类。

**验证：** `npx vitest run src/features/result/ResultSheet.test.tsx` 全绿。

## T24: 预算弹层改造与测试

**文件：** `web/src/features/budget/BudgetPopover.tsx`、`web/src/features/budget/BudgetPopover.test.tsx`
**依赖：** T5
**步骤：**
1. props 改为 `budget: ResultBudget | null`、`includeLodging`、`readOnly`、`pending`、`onToggleLodging`；删除对 `budgetBreakdown` 的调用。
2. 行与合计直接读 `budget`；住宿行按 `budget.lodging > 0` 显示。
3. 更新测试：空预算禁用、行与总计、切换文案、readOnly 隐藏开关。

**验证：** `npx vitest run src/features/budget/BudgetPopover.test.tsx` 全绿。

## T25: 工作台接入与测试

**文件：** `web/src/features/workspace/WorkspacePage.tsx`、`web/src/features/workspace/WorkspacePage.test.tsx`
**依赖：** T22、T23、T24
**步骤：**
1. 新增 `view` 状态（默认 `dossier`），`selection` 改为 `ResultSelection`；`buildResultModel` 用 `useMemo` 依赖 `canvas`。
2. `jumpTo` 映射为 `{ kind: "stop" }`；移动端仍打开面板。
3. 选中守卫：`activeDay` 非空且选中属于其他天时清空；目标不存在时清空。
4. `actionError`：记录 `lastAction`，仅住宿切换失败时把 `session.error` 传给工具栏。
5. 桌面渲染 `ResultShell`，移动端渲染 `ResultSheet`；`BudgetPopover` 接收 `model.budget`。
6. 更新测试：默认手册、切换保持、跳转选中、预算切换、移动端面板。

**验证：** `npx vitest run src/features/workspace/WorkspacePage.test.tsx` 全绿。

## T26: 分享页改造与测试

**文件：** `web/src/features/share/SharePage.tsx`、`web/src/features/share/SharePage.test.tsx`
**依赖：** T15
**步骤：**
1. 用 `parseItinerary` / `parseResearch` 构建 `ResultModel`，渲染 `DossierView readOnly`。
2. 移除 `PlanCanvas`、`BudgetSummary` 侧栏、`ResearchNotes` 独立区块；保留复制、版本来源、页脚说明。
3. 不引用 `TripMap`；`onViewOnMap` 不传。
4. 更新测试：无切换器、无地图容器、理由与来源可见、复制可用、快照缺失字段降级。

**验证：** `npx vitest run src/features/share/SharePage.test.tsx` 全绿。

## T27: 删除旧组件与样式

**文件：** `web/src/features/canvas/`、`web/src/features/timeline/`、`web/src/index.css`
**依赖：** T25、T26
**步骤：**
1. 先 `rg "features/canvas|features/timeline|PlanCanvas|CanvasSheet|CanvasToolbar|Inspector|ItineraryTimeline|timelineModel|DayList" web/src` 确认只剩待删文件。
2. 删除 `features/canvas/`（含 `PlanCanvas`、`Inspector`、`CanvasSheet`、`CanvasToolbar`、`DayList`、旧 `DayTabs` 及测试）与 `features/timeline/`。
3. 删除 `index.css` 中 `.vis-timeline`、`.vis-item.tp-block*`、`.vis-selected` 相关规则。

**验证：** `npx tsc -b --pretty false` 退出码 0；上述 `rg` 无输出（退出码 1 属正常）。

## T28: 移除时间轴依赖

**文件：** `web/package.json`、`web/package-lock.json`
**依赖：** T27
**步骤：**
1. `npm uninstall vis-timeline vis-data`。
2. `rg "vis-timeline|vis-data" web/src web/package.json` 确认无引用。

**验证：** `npm ls vis-timeline vis-data || true` 无这两个包；`npx tsc -b --pretty false` 退出码 0。

## T29: 全量编译与测试

**文件：** 无
**依赖：** T27、T28
**步骤：**
1. `npx tsc -b --pretty false`。
2. `npm test`。
3. `npm run build`。

**验证：** 三条命令均退出码 0，测试 0 失败。

## T30: 人工验收（开发服务器）

**文件：** 无
**依赖：** T29
**步骤：**
1. `npm run dev`，使用既有 8080 后端，不重启后端进程。
2. 完成方案后检查：默认手册；切换地图/看板后回到手册，天与选中保持；地图只有当天、无「全部」、路段可点选并高亮；手册小图「查看路线图」跳转正确。
3. 390px 宽检查三种视图无横向滚动、地图全屏可关闭；分享链接无切换器与地图；无地图 Key 时降级清单可点选。
4. 记录实际观察结果，不只记录预期。

**验证：** 上述场景逐项通过并记录证据（截图或文字记录）。

## 执行顺序

```
T1 → T2 → T3 → T4 → T5 → T6 → T7 → T8
T9 ──────────────────────────────┐
T1 → T10 → T11 → T12 → T13 → T14 → T15
T16 → T17 → T18
T3, T9 → T19 → T20
T10, T11, T12 → T21
T18, T20, T21 → T22 → T23
T5 → T24
T22, T23, T24 → T25 → T26
T25, T26 → T27 → T28 → T29 → T30
```

并行建议：T9 与 T10 可并行；T16/T17 与 T14 可并行；T24 可在 T20 前完成。