# TripPilot v1.6 结果页三视图 Plan

状态：草稿（全部分节已呈现，待整份审批）
日期：2026-11-20

## 架构概览

结果区保持「事件 → 推导 → 渲染」的现有链路，只在推导与渲染之间加一层纯模型，三种视图与分享页共用同一份模型。

- **数据层（不变）**：`deriveCanvas(events, trip, version)` 继续产出 `CanvasModel`（plan、explanations、research、isDraft、versionSeq、source）。不新增请求、不新增事件类型。
- **模型层（新增）**：`buildResultModel` 把 `Itinerary + explanations + research` 整理成面向阅读的 `ResultModel`：按天分章、交通折叠成路段、留白补齐、理由与攻略按点归位。纯函数、无 React、无 I/O，工作台与分享页都调用它。
- **外壳层**：`ResultShell` 持有当前视图、当前天与选中对象，渲染 `ResultToolbar` 与三种视图之一；视图切换只改本地枚举，不触发数据请求。
- **三视图**：`DossierView`（手册：概览 + 每日章节 + 当天 SVG 小图 + 收尾）、`MapView`（地图：单日地图 + 剧本列）、`BoardView`（看板：指标 + 分天列 + 浮层详情）。
- **共用详情**：`StopDetail` 与 `LegDetail` 是详情内容的唯一实现，手册内联、地图剧本浮层、看板浮层都复用它。
- **小图**：`DayMiniMap` 用纯 SVG 按路段几何绘制当天路线，不创建地图实例；分享页只用它，不加载 AMap。
- **移除**：`PlanCanvas`、`Inspector`（含 `CanvasSelection`）、`CanvasSheet`、`CanvasToolbar`、`DayList`、`ItineraryTimeline` 与 `timelineModel`，以及 `vis-timeline` 依赖。`WorkspacePage` 改为渲染 `ResultShell`，移动端用 `ResultSheet` 承载视图切换。
- **状态归属**：`activeDay`、`selection`、`view`、`versionSeq` 仍由 `WorkspacePage` 持有；分享页只读，固定手册、无切换器、无选中编辑。

## 核心数据结构

### 视图与选中

```ts
export type ResultViewMode = "dossier" | "map" | "board";

export type ResultSelection =
  | { kind: "stop"; dayIndex: number; stopID: string }   // stopID 即 block.id
  | { kind: "leg"; dayIndex: number; legID: string }
  | { kind: "hotel" }
  | null;

export type ResultTarget =
  | { kind: "stop"; day: ResultDay; stop: ResultStop }
  | { kind: "leg"; day: ResultDay; leg: ResultLeg }
  | { kind: "hotel"; lodging: ResultLodging }
  | null;
```

- 同一时刻只有一个选中对象；选中路段时清空选点，反之亦然。
- `view` 只存在会话内（组件状态），不写 URL、不写后端。

### 顶层模型

```ts
export interface ResultModel {
  city: string;
  title: string;          // "北京 · 4 天"
  summary: string;        // "4 天串起故宫、景山、什刹海与三里屯"
  overview: ResultOverview;
  budget: ResultBudget;
  lodging: ResultLodging | null;
  reminders: ResultReminder[];
  days: ResultDay[];
}

export interface ResultOverview {
  days: number;
  poiCount: number;       // 取 totals.poi_count
  walkingKM: number;      // 取 totals.walking_km
  pace: ResultPace;
}

export type ResultPace = "relaxed" | "balanced" | "packed";

export interface ResultBudget {
  tickets: number;
  meals: number;
  transit: number;
  lodging: number;
  total: number;
  lodgingIncluded: boolean;
  estimated: boolean;
}
```

### 天、停靠点、路段

```ts
export interface ResultDay {
  index: number;
  date: string;
  label: string;          // "11 月 20 日 周五"，无日期时 "第 1 天"
  theme: string;          // 前三个停靠点标题以 " · " 连接，缺失时 "第 1 天"
  start: string;
  end: string;
  costYuan: number;       // day_cost_yuan
  stops: ResultStop[];    // 不含 transit 块
  legs: ResultLeg[];      // 相邻停靠点之间的移动
  gaps: ResultGap[];
}

export interface ResultStop {
  id: string;             // block.id
  dayIndex: number;
  order: number;          // 当天下标，从 0 开始
  type: string;
  typeLabel: string;      // 景点 / 餐饮 / 休息
  title: string;
  start: string;
  end: string;
  durationMin: number;
  costYuan: number;
  lat?: number;
  lng?: number;
  reason: string | null;  // explanations 按 poi_id 归位；缺失为 null
  guides: ResultGuide[];  // research.guides 按 poi_id 归位；缺失为 []
  note?: string;          // block.notes
}

export interface ResultLeg {
  id: string;             // `${dayIndex}:${from.id}>${to.id}`
  dayIndex: number;
  from: ResultPoint;
  to: ResultPoint;
  mode: string;           // transport.mode 原值，可能为空
  modeLabel: string;      // 步行 / 公交 / 地铁 / 驾车 / 交通
  durationMin: number;
  distanceM: number;
  costYuan: number;
  estimated: boolean;     // 整段是否含估算
  segments: ResultLegSegment[];
  geometry: "polyline" | "straight" | "none";
}

export interface ResultPoint {
  id: string;
  title: string;
  lat?: number;
  lng?: number;
}

export interface ResultLegSegment {
  mode: string;
  modeLabel: string;
  lineName?: string;
  fromStop?: string;
  toStop?: string;
  distanceM: number;
  durationMin: number;
  costYuan: number;
  estimated: boolean;
  polyline: [number, number][];   // [lng, lat]；无折线为空数组
}
```

- `ResultLeg` 是「上一站 → 下一站」的阅读单位；`ResultLegSegment` 是其中的方式段（步行 → 地铁 → 步行）。
- `geometry`：任一分段有折线 → `polyline`；无折线但首尾有坐标 → `straight`（渲染为直线并标「直线估算」）；缺坐标 → `none`（地图略过该段，详情给出文字说明）。

### 留白与提醒

```ts
export interface ResultGap {
  id: string;             // `${dayIndex}:gap:${start}`
  dayIndex: number;
  start: string;
  end: string;
  minutes: number;
  suggestion: string;     // 午饭 / 晚饭 / 休息 / 自由活动 / 回住宿休息
}

export interface ResultReminder {
  id: string;             // `research:${index}`
  text: string;
}
```

- 留白只统计 ≥ 45 分钟的时段：开门到首站、站间、末站到收尾。
- 时间无法解析时不生成留白，也不报错。
- 提醒来自 `research.notes` 与 `research.warnings`，去空去重；为空时该块不渲染。

### 住宿与攻略

```ts
export interface ResultLodging {
  hotel: ResultHotel;
  nights: number;
  nightlyYuan: number;
  totalYuan: number;
  anchorDays: number[];   // 从 0 开始的天下标
  includeInBudget: boolean;
  priceEstimated: boolean;
}

export interface ResultHotel {
  name: string;
  address?: string;
  city?: string;
  lat: number;
  lng: number;
  star?: number;
  score?: number | null;
  bookingURL?: string;    // 仅保留 http/https
  provider?: string;
}

export interface ResultGuide {
  poiID: string;
  note?: string;
  citations: ResultCitation[];
}

export interface ResultCitation {
  title?: string;
  url?: string;           // 仅保留 http/https
  site?: string;
  publishedAt?: string | null;
  authority?: number;
}
```

### 构建与解析

```ts
export interface ResultInput {
  itinerary: Itinerary | null;
  explanations?: Map<string, string> | null;
  research?: ResearchBrief | null;
}

export function buildResultModel(input: ResultInput): ResultModel | null;

export function resolveResultSelection(
  model: ResultModel,
  selection: ResultSelection,
): ResultTarget;
```

- `itinerary` 为空时返回 `null`，由视图显示空状态；其余字段一律给安全默认值，不抛错。
- 工作台与分享页分别用 `useMemo` 调用一次，三视图不再各自计算预算或按点找理由。

### 派生规则

1. **停靠点**：按源顺序取 `poi / meal / rest` 块；`transit` 块不进入停靠点列表。
2. **路段**：相邻停靠点之间优先取前一块的 `transport`；否则取两者之间第一个带 `transport` 的 `transit` 块；都没有则不生成路段，也不留空白块。
3. **分段**：`transport.legs` 存在时逐条映射；否则用整段生成一个分段。整段 `mode` 与分段 `mode` 各自映射中文标签。
4. **理由**：`explanations` 以 `poi_id` 建索引；`rest` 或餐饮无 `poi_id` 时为 `null`。
5. **攻略**：`research.guides` 按 `poi_id` 分组；引用链接在模型层过滤非 http/https。
6. **预算**：复用 `budgetBreakdown`；`lodgingIncluded` 取 `lodging.include_in_budget`。
7. **节奏**：按日均停靠点与日均步行判定，阈值见技术决策。
8. **标题与摘要**：全部由已有标题、城市、天数拼装，不调用模型、不新增字段。

## 模块设计

### 模型层 `features/result/model.ts`

**职责：** 唯一的结果推导入口；把方案整理成按天可读的结构，提供选中解析与统一文案表。

**对外接口：**

```ts
buildResultModel(input: ResultInput): ResultModel | null;
resolveResultSelection(model: ResultModel, selection: ResultSelection): ResultTarget;

export const modeLabels: Record<string, string>;   // walk/bus/subway/transit/drive
export const paceLabels: Record<ResultPace, string>; // 轻松/适中/紧凑
export const typeLabels: Record<string, string>;   // poi/meal/rest
```

**依赖：** `itinerary/types`、`research/types`、`budget/budgetModel`、`design/tokens`（日期格式）。不依赖任何组件、不依赖网络。

### 地图样式 `features/map/routeStyle.ts`

**职责：** 地图折线与手册小图共用的路段样式；把模式映射、选中/淡化状态、图例数据收在一处。

**对外接口：**

```ts
export type RouteEmphasis = "default" | "selected" | "muted";

export interface RouteStyle {
  color: string;
  dashed: boolean;
  weight: number;     // default 4 / selected 6 / muted 4
  opacity: number;    // default .9 / selected 1 / muted .25
  title: string;      // 线路名与站点，纯文本
}

export function routeStyle(mode: string, lineName: string | undefined, emphasis: RouteEmphasis): RouteStyle;
export const modeLegend: ReadonlyArray<{ label: string; color: string; dashed: boolean }>;
```

**依赖：** 无。

### 外壳 `features/result/ResultShell.tsx`

**职责：** 结果区的统一容器：工具栏、视图切换、三种视图的挂载点；不持有数据，只分发状态与回调。

**对外接口：**

```ts
export interface ResultShellProps {
  model: ResultModel | null;
  view: ResultViewMode;
  onViewChange: (view: ResultViewMode) => void;
  activeDay: number | null;
  onActiveDayChange: (day: number | null) => void;
  selection: ResultSelection;
  onSelect: (selection: ResultSelection) => void;
  controls?: ReactNode;        // 预算、版本、分享
  actionError?: string | null; // 预算切换等操作的即时错误
  isDraft: boolean;
  versionSeq: number | null;
  readOnly?: boolean;
  onAdjustDay?: (dayIndex: number) => void;
}
```

- 桌面：工具栏吸顶，内容区独立滚动。
- 移动：由 `ResultSheet` 包裹，同一个 `ResultShell` 渲染，保证切换器位置一致。
- 视图切换只改 `view`；切到地图时若无当前天则落到第 1 天；把选中对象所在天补进 `activeDay`。
- 内部提供 `viewOnMap(dayIndex)`：`onActiveDayChange(day)` + `onViewChange("map")`，供手册小图与路段详情调用。
- `model` 为空时显示空状态，不渲染三视图。

**依赖：** `model`、`ResultToolbar`、`DossierView`、`MapView`、`BoardView`、`DetailCard`。

### 工具栏 `features/result/ResultToolbar.tsx`

**职责：** 方案标题、状态、视图切换、天切换与共用控制（预算、版本、分享）；替换 `CanvasToolbar`。

**对外接口：**

```ts
export interface ResultToolbarProps {
  model: ResultModel | null;
  view: ResultViewMode;
  onViewChange: (view: ResultViewMode) => void;
  activeDay: number | null;
  onActiveDayChange: (day: number | null) => void;
  showDayTabs: boolean;   // 手册与地图 true，看板 false
  allowAll: boolean;      // 手册 true，地图 false
  isDraft: boolean;
  versionSeq: number | null;
  controls?: ReactNode;
  actionError?: string | null;
  readOnly?: boolean;
  className?: string;
}
```

- 视图切换放在标题右侧，桌面与移动同一位置；移动端不折叠进「更多」。
- 天切换复用 `DayTabs`，新增 `allowAll` 属性；地图下不出现「全部」。
- `actionError` 以 `role="alert"` 就地显示在控制区下方，不弹 toast、不丢失选中。
- `readOnly` 时不渲染分享与调整入口。

**依赖：** `DayTabs`、`ViewSwitcher`、`StatusDot`、`Badge`。

### 视图切换 `features/result/ViewSwitcher.tsx`

**职责：** 三段式「手册 / 地图 / 看板」控件，桌面与移动共用。

**对外接口：**

```ts
export interface ViewSwitcherProps {
  value: ResultViewMode;
  onChange: (view: ResultViewMode) => void;
  size?: "sm" | "md";
}
```

- `role="tablist"`，左右方向键切换，`aria-selected` 标当前项，焦点环可见。

### 手册 `features/result/DossierView.tsx`

**职责：** 按天阅读的正文：概览、每日章节、收尾；分享页复用同一组件并开启 `readOnly`。

**对外接口：**

```ts
export interface DossierViewProps {
  model: ResultModel;
  activeDay: number | null;
  selection: ResultSelection;
  onSelect: (selection: ResultSelection) => void;
  onViewOnMap: (dayIndex: number) => void;
  onAdjustDay?: (dayIndex: number) => void;
  readOnly?: boolean;
}
```

- `activeDay` 非空时只渲染该天章节（与 `DayTabs` 口径一致），为空渲染全部；切天后回到该天顶部。
- 概览：标题、路线摘要、天数 / 景点 / 步行 / 预算 / 节奏；数字与 `model.overview`、`model.budget` 一致。
- 每天一个 `DayChapter`；末尾住宿摘要与提醒。
- 选中停靠点时该行高亮；理由与攻略就地展开，不跳转。

**依赖：** `DayChapter`、`Surface`/`SectionLabel`/`Stat`、`BudgetSummary`（从 `model.budget` 渲染）。

### 每日章节 `features/result/DayChapter.tsx`

**职责：** 单天的连续叙事：标题、时间锚点、停靠点与路段按顺序交替、留白、当天合计、当天小图。

**对外接口：**

```ts
export interface DayChapterProps {
  day: ResultDay;
  hotel: ResultHotel | null;
  selection: ResultSelection;
  onSelect: (selection: ResultSelection) => void;
  onViewOnMap: (dayIndex: number) => void;
  onAdjustDay?: (dayIndex: number) => void;
  readOnly?: boolean;
}
```

- 顺序由 `day.stops` 与 `day.legs` 的位置关系还原：每个停靠点后渲染通往下一站的路段；末尾渲染 `day.gaps` 与当天合计。
- 停靠点内联 `StopFacts`（时间、时长、花费）、`ReasonSection`、`GuideSection`；三者与详情卡共用。
- 路段行用 `LegRow`，点击即 `onSelect({ kind: "leg", ... })` 并展开 `LegDetail`。
- 调整入口只在章节头出现一次，不逐点重复。

**依赖：** `DayMiniMap`、`LegRow`、`StopFacts`、`ReasonSection`、`GuideSection`、`LegDetail`。

### 当天小图 `features/result/DayMiniMap.tsx`

**职责：** 不依赖 AMap 的当天路线缩略图；用 SVG 按路段几何绘制，支持选点、选段与「查看路线图」。

**对外接口：**

```ts
export interface DayMiniMapProps {
  day: ResultDay;
  hotel: ResultHotel | null;
  selection: ResultSelection;
  onSelect: (selection: ResultSelection) => void;
  onViewOnMap: (dayIndex: number) => void;
}
```

- 投影：以当天坐标为范围做等距投影（经度按纬度余弦修正），留 8px 内边距；无坐标的点不画，但仍出现在章节列表。
- 路段：`geometry === "polyline"` 逐分段画；`straight` 画虚线直线；`none` 跳过。样式来自 `routeStyle`，选中加粗、其余淡化，与地图严格一致。
- 站点：编号圆点，可点击选中；酒店锚点仅在该天被 `anchorDays` 命中时绘制。
- 当天没有任何可用坐标时降级为文字清单（`StopChip` 列表），并提示「缺少坐标，无法绘制当天路线」。
- 底部按钮「查看路线图」调用 `onViewOnMap(day.index)`。

**依赖：** `routeStyle`、`ResultDay` 类型。

### 地图视图 `features/result/MapView.tsx`

**职责：** 单日地图与剧本列的并排阅读；桌面左右分栏，移动端地图全屏、剧本可折叠。

**对外接口：**

```ts
export interface MapViewProps {
  model: ResultModel;
  activeDay: number;                                   // 由外壳保证合法，不出现 null
  onActiveDayChange: (day: number) => void;
  selection: ResultSelection;
  onSelect: (selection: ResultSelection) => void;
  onAdjustDay?: (dayIndex: number) => void;
  readOnly?: boolean;
}
```

- 只把当前天的 `ResultDay` 传给 `TripMap`，不传整个方案，从接口上杜绝多天叠加。
- 选中对象属于本天时由 `DetailCard` 浮出；切换天时清空选中（跨天选中无意义），并 `onActiveDayChange`。
- `TripMap` 不可用时，剧本列仍完整，地图位置显示当天文字清单。

**依赖：** `TripMap`、`StoryColumn`、`DetailCard`。

### 地图组件 `features/map/TripMap.tsx`（改造）

**职责：** 单日 AMap 渲染：站点标记、酒店锚点、路段折线与点击选中。

**对外接口：**

```ts
export interface TripMapProps {
  day: ResultDay;
  hotel: ResultHotel | null;
  selectedStopID?: string | null;
  selectedLegID?: string | null;
  onSelectStop?: (stopID: string) => void;
  onSelectLeg?: (legID: string) => void;
  onSelectHotel?: () => void;
}
```

- 标记：`day.stops` 中带坐标者，景点/餐饮编号，休息不编号。
- 路段：遍历 `day.legs`；每段按 `segments` 逐条绘制折线，无折线且首尾有坐标时画直线并加「直线估算」标题，缺坐标则跳过。
- 点击：折线 `on("click")` 调用 `onSelectLeg(leg.id)`；标记调用 `onSelectStop`；酒店调用 `onSelectHotel`。折线 `bubble` 关闭，避免点线同时触发地图空白。
- 选中：对应路段 `routeStyle(selected)`，其余 `muted`；选点标记放大并置顶。
- 视野：选中路段时 `setFitView` 该段覆盖物；否则 fit 当天标记与酒店锚点。不弹信息窗。
- 降级：`MapFallback` 改为当天文字清单，停靠点与路段都可点选，文案为「地图暂不可用，仍可查看当天安排与路线详情」。
- 图例：只列方式（含虚线步行与「直线估算」）与 GCJ-02，不再列多天颜色。

**依赖：** `routeStyle`、`lib/amap`、`ResultDay` 类型。

### 剧本列 `features/result/StoryColumn.tsx`

**职责：** 地图旁的当天下文；按时间顺序列出停靠点与路段，与地图双向同步。

**对外接口：**

```ts
export interface StoryColumnProps {
  day: ResultDay;
  selection: ResultSelection;
  onSelect: (selection: ResultSelection) => void;
  onSelectStop: (stopID: string) => void;
  onSelectLeg: (legID: string) => void;
  readOnly?: boolean;
}
```

- 每个停靠点一行：序号、时间、标题、花费；展开后是 `StopDetail`。
- 每个路段一行：`LegRow`（方式、线路、耗时、花费、估算标记）；展开后是 `LegDetail`。
- 选中项滚动进视野并高亮；不做自动滚动以外的动画。

**依赖：** `StopDetail`、`LegDetail`、`LegRow`。

### 看板 `features/result/BoardView.tsx`

**职责：** 横向对比与核对：五指标、分天列、留白与提醒、浮层详情。

**对外接口：**

```ts
export interface BoardViewProps {
  model: ResultModel;
  activeDay: number | null;
  selection: ResultSelection;
  onSelect: (selection: ResultSelection) => void;
  onAdjustDay?: (dayIndex: number) => void;
  readOnly?: boolean;
}
```

- 指标：总预算、住宿、门票、步行、景点数，全部取 `model.budget` / `model.overview`。
- 列：每天一列，列头为日期与当天合计；卡内为时间、标题、花费、一句理由（无理由时显示「未生成额外说明」）；卡间插入紧凑 `LegRow`。
- 留白卡与提醒卡排在对应天列内，措辞与手册一致。
- 选中卡在桌面浮出 `DetailCard`，移动端以底部 Sheet 呈现同一组件。
- 天切换不在看板头部显示；`activeDay` 用于初始滚动与高亮对应列（移动端折叠清单默认展开该天）。

**依赖：** `DetailCard`、`StopDetail`、`LegDetail`、`LegRow`、`model` 指标。

### 共用详情

**`features/result/StopDetail.tsx`**

```ts
export interface StopDetailProps {
  day: ResultDay;
  stop: ResultStop;
  readOnly?: boolean;
  onAdjustDay?: (dayIndex: number) => void;
}
```

拆分导出 `StopFacts`、`ReasonSection`、`GuideSection`，供章节内联与浮层详情共用；文案固定为「时长 / 花费 / 为什么选它 / 攻略提及 / 本次未生成额外说明 / 暂无攻略提及」。

**`features/result/LegDetail.tsx`**

```ts
export interface LegDetailProps {
  day: ResultDay;
  leg: ResultLeg;
  onViewOnMap?: (dayIndex: number) => void;
  readOnly?: boolean;
}
```

- 头部：`步行 · 18 分钟 · ¥4`，估算时加「估算」标记。
- 分段列表：每条显示方式、线路名、上/下车站、距离、耗时、花费；站点与线路按纯文本渲染。
- 几何说明：有折线「按实际路线绘制」；直线「直线估算，仅供距离参考」；缺坐标「缺少坐标，仅显示文字」。
- 「查看路线图」仅在手册中出现（地图与看板已在结果区，不重复）。

**`features/result/LegRow.tsx`**

```ts
export interface LegRowProps {
  leg: ResultLeg;
  selected: boolean;
  onSelect: () => void;
}
```

只负责一行摘要与点击，不承载详情，保证手册、剧本、看板同一措辞。

**`features/result/DetailCard.tsx`**

```ts
export interface DetailCardProps {
  target: ResultTarget;
  onClose: () => void;
  onAdjustDay?: (dayIndex: number) => void;
  onViewOnMap?: (dayIndex: number) => void;
  readOnly?: boolean;
}
```

- 根据 `target.kind` 渲染 `StopDetail` / `LegDetail` / 住宿详情。
- 桌面为浮层面板（`role="dialog"`，Esc 关闭，焦点回到触发项）；移动端为底部 Sheet。
- 住宿详情从原 `HotelInspector` 迁移：星级、评分、地址、晚数、锚点、预订链接（http/https）、计入预算开关。

### 移动端 `features/result/ResultSheet.tsx`

**职责：** 移动端方案面板；替换 `CanvasSheet`。

**对外接口：**

```ts
export interface ResultSheetProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  model: ResultModel | null;
  view: ResultViewMode;
  onViewChange: (view: ResultViewMode) => void;
  activeDay: number | null;
  onActiveDayChange: (day: number | null) => void;
  selection: ResultSelection;
  onSelect: (selection: ResultSelection) => void;
  controls?: ReactNode;
  actionError?: string | null;
  isDraft: boolean;
  versionSeq: number | null;
  readOnly?: boolean;
  onAdjustDay?: (dayIndex: number) => void;
}
```

- `SheetHeader` 内为 `ViewSwitcher` 与天切换（吸顶）；没有二级「地图/时间轴/详情」页签。
- 地图视图全屏：`MapView` 填满 Sheet，关闭 Sheet 即回到原对话位置。
- 手册一天一屏：`activeDay` 默认当天或第 1 天；看板为按天折叠清单。

### 工作台接入 `features/workspace/WorkspacePage.tsx`

**职责：** 仍持有 `activeDay`、`selection`、`viewVersion`；新增 `view`；把 `CanvasModel` 交给模型层。

- `const result = useMemo(() => buildResultModel({ itinerary: canvas.model.plan, explanations: canvas.model.explanations, research: canvas.model.research }), [canvas]);`
- `selection` 类型改为 `ResultSelection`；`jumpTo` 把 `JumpTarget` 转成 `{ kind: "stop", dayIndex, stopID }`，仍打开移动端面板。
- 桌面渲染 `ResultShell`；移动端渲染 `ResultSheet`；两者共享同一 `view` 与 `selection`。
- `BudgetPopover` 改为接收 `model.budget`，不再自行调用 `budgetBreakdown`，保证预算口径单一来源。
- 把 `toggleLodging` 的失败信息（`session.error`）作为 `actionError` 传入工具栏。
- 删除 `CanvasSelection`、`PlanCanvas`、`CanvasSheet`、`CanvasToolbar`、`Inspector`、`ItineraryTimeline`、`timelineModel` 的引用。

### 分享页 `features/share/SharePage.tsx`（改造）

**职责：** 只读手册；不出现切换器、版本操作、地图实例或内部标识。

- `buildResultModel({ itinerary: parseItinerary(shared.version.plan), explanations: explanationMap(itinerary), research: parseResearch(shared.version.research) })`。
- 渲染 `DossierView readOnly`（含当天 SVG 小图、住宿摘要、提醒）；保留复制行程文本与版本来源说明。
- 移除 `PlanCanvas`、`BudgetSummary` 侧栏与 `ResearchNotes` 独立区块（理由与攻略已归位到停靠点）；`itineraryToPlainText` 不变。
- 不加载 AMap：分享页不引用 `TripMap`。

### 删除与迁移

| 删除 | 迁移去向 |
|------|----------|
| `canvas/PlanCanvas.tsx` | `ResultShell` |
| `canvas/Inspector.tsx` | `StopDetail` / `LegDetail` / `DetailCard` / `model` |
| `canvas/CanvasSheet.tsx` | `ResultSheet` |
| `canvas/CanvasToolbar.tsx` | `ResultToolbar` |
| `canvas/DayList.tsx` | `DossierView` + `DayChapter` |
| `timeline/ItineraryTimeline.tsx`、`timeline/timelineModel.ts` | 手册章节与地图剧本列 |
| `canvas/DayTabs.tsx` | `result/DayTabs.tsx`（新增 `allowAll`） |
| `vis-timeline` 依赖 | 无（手册与剧本为普通列表） |

### 需求归属

| 需求 | 模块 |
|------|------|
| F1、F2 | `ResultShell`、`ResultToolbar`、`ViewSwitcher`、`WorkspacePage` |
| F3 | `SharePage`、`DossierView` |
| F4–F8 | `DossierView`、`DayChapter`、`DayMiniMap`、`StopDetail` |
| F9–F11 | `MapView`、`TripMap`、`StoryColumn` |
| F12、F13 | `BoardView`、`DetailCard` |
| F14、F15 | `StopDetail`、`LegDetail`、`ResultToolbar`、`DetailCard` |
| F16 | `ResultSheet`、`MapView`、`BoardView` |
| F17 | `model`（默认值与缺失说明）、各组件的空状态 |
| F18 | `routeStyle`、`TripMap`、`DayMiniMap`、`LegRow`、`LegDetail` |

## 模块交互

### 1. 初始化与推导

```
SSE → useSession.events
    → deriveCanvas(events, trip, viewVersion) → CanvasModel
    → buildResultModel({ plan, explanations, research }) → ResultModel
    → ResultShell(model, view, activeDay, selection)
```

- `buildResultModel` 只在 `CanvasModel` 变化时重算（`useMemo` 依赖 `canvas`）；切换视图、选天、选点都不触发重算，也不产生网络请求。
- 视图、天、选中对象由 `WorkspacePage` 持有，桌面与移动面板共享同一份状态。

### 2. 切换视图

```
ViewSwitcher → ResultShell.onViewChange → WorkspacePage.setView
```

- 切到地图：`activeDay = 选中对象所在天 ?? 当前天 ?? 0`；地图不接受 `null`。
- 切到手册/看板：保留 `activeDay` 与 `selection`。
- 内容容器滚动到当前天章节顶部，不逐像素恢复。
- 视图状态只存组件内存；刷新页面回到手册。

### 3. 选天

```
DayTabs → ResultToolbar.onActiveDayChange → WorkspacePage.setActiveDay
```

- 手册：`null` 显示全部；具体天只显示该天章节，并滚到章节顶部。
- 地图：只允许具体天；切天后清空跨天选中（选中对象不在新天时）。
- 看板：不提供天切换；`activeDay` 只用于初始定位与列高亮。
- `WorkspacePage` 统一守卫：`activeDay !== null` 且选中对象属于其他天时清空选中，避免「看不见的选中」。

### 4. 选中停靠点

```
地图标记 / 手册行 / 剧本行 / 看板卡
  → onSelect({ kind: "stop", dayIndex, stopID })
  → WorkspacePage.setSelection
  → resolveResultSelection(model, selection) → ResultTarget
```

- 地图：`TripMap` 只更新标记尺寸与层级，不重建覆盖物；地图与剧本双向高亮。
- 手册：该停靠点内联展开 `StopFacts / ReasonSection / GuideSection`，不浮层。
- 地图与看板：`DetailCard` 依据 `ResultTarget` 渲染同一份 `StopDetail`。
- 对话里的跳转（`jumpTo`）转换为同一个 `stop` 选中，移动端顺带打开面板。

### 5. 选中路段

```
地图折线 / 手册 LegRow / 剧本 LegRow / 看板 LegRow
  → onSelect({ kind: "leg", dayIndex, legID })
  → WorkspacePage.setSelection
```

- `TripMap`：选中段 `routeStyle = selected`，其余 `muted`；按该段覆盖物 `setFitView`，不重建折线。
- 手册：路段行下展开 `LegDetail`；「查看路线图」→ `ResultShell.viewOnMap(day)`（先设天，再切地图，选中保留）。
- 地图与看板：`DetailCard` 渲染同一份 `LegDetail`。
- 选中是单一值：选路段替换选点，反之亦然；关闭详情写回 `null`。

### 6. 详情卡开关

- 打开：`selection` 非空且目标可解析。
- 关闭：点击关闭、按 Esc、点按遮罩 → `onSelect(null)`；焦点回到触发元素。
- 目标失效（方案更新后该点/段不存在）时 `resolveResultSelection` 返回 `null`，`WorkspacePage` 的守卫 effect 清空 `selection`。

### 7. 调整这一天

```
章节头 / DetailCard
  → onAdjustDay(dayIndex)
  → WorkspacePage: setActiveDay, prefill "调整第 N 天：", 关闭移动面板, 聚焦输入框
  → Composer 提交 → session.actions.adjust(text, baseVersion)
  → SSE 新方案 → deriveCanvas → buildResultModel → 三视图更新
```

- 提交中与失败信息仍由 Composer 就地呈现；结果区不复制错误提示。
- 新方案到达后保留 `activeDay` 与 `view`；选中目标不存在时清空。

### 8. 预算口径切换

```
BudgetPopover（ResultToolbar.controls）
  → onToggleLodging
  → session.actions.replanLodging({ ...constraints, budget: { include_lodging: !includeLodging } }, baseVersion)
  → pending 禁用按钮
  → SSE plan.updated → ResultModel.budget 重算 → 工具栏、手册、看板同步更新
```

- 失败时 `session.error` 作为 `actionError` 交给 `ResultToolbar`，就地显示，选中与视图不变。
- 没有 `baseVersion` 时入口禁用，与现有逻辑一致。

### 9. 版本预览

```
VersionMenu → onViewVersion(version)
  → WorkspacePage.setViewVersion
  → deriveCanvas(events, trip, version) → 新 CanvasModel → 新 ResultModel
```

- 三视图展示所选版本；`onReturnCurrent` 清空预览并恢复当前方案。
- 版本、分享、恢复语义不变，不在本次改动范围。

### 10. SSE 更新与地图稳定

- 文本更新：`ResultModel` 变化只重渲染手册/剧本/看板文字。
- 地图：`TripMap` 的覆盖物重建依赖当天的几何签名（站点 id 与坐标、路段 id 与折线长度/端点）；仅文字、理由或预算变化时不重建标记与折线。
- 选中 id 若仍存在则保留强调与视野，不因无关事件跳动。

### 11. 分享页

```
getSharedTrip → parseItinerary / parseResearch
  → buildResultModel(...) → DossierView readOnly
```

- 不渲染外壳、切换器、版本菜单；不引用 `TripMap`，不触发地图加载。
- 小图可点选，仅用于本地高亮与展开详情；无「调整这一天」、无「查看路线图」。
- 复制行程文本与版本来源说明保持现状。

### 12. 移动端面板

```
对话「查看方案」→ ResultSheet(open)
  → ResultShell（同一 view / activeDay / selection）
  → 关闭 Sheet → 回到原对话位置
```

- 地图视图填满面板；手册一天一屏；看板按天折叠。
- 面板与桌面共用状态，关闭再打开仍停在原视图与选中。

### 13. 降级路径汇总

| 场景 | 行为 |
|------|------|
| 无方案 | `ResultShell` 空状态，不渲染三视图 |
| 地图 Key 缺失 / 加载失败 | `TripMap` 文字清单，停靠点与路段可点选；剧本与详情不受影响 |
| 当天无坐标 | 小图降级为停靠点清单，提示缺少坐标；地图仍可画有坐标的段 |
| 路段无折线 | 直线 + 「直线估算」；详情说明估算依据 |
| 路段缺首尾坐标 | 地图跳过该段；详情显示「缺少坐标，仅显示文字」 |
| 无理由 / 无攻略 | 「本次未生成额外说明」「暂无攻略提及」 |
| 旧版本快照 | 以上默认值生效，不抛错、不空白 |

## 文件组织

```
web/src/
├── features/
│   ├── result/                        新增
│   │   ├── model.ts                   ResultModel 类型、buildResultModel、resolveResultSelection、文案表
│   │   ├── ResultShell.tsx            外壳：工具栏 + 视图挂载 + viewOnMap
│   │   ├── ResultToolbar.tsx          标题、切换器、天 tabs、控制槽、actionError
│   │   ├── ViewSwitcher.tsx           手册 / 地图 / 看板
│   │   ├── DossierView.tsx            概览 + 章节 + 收尾
│   │   ├── DayChapter.tsx             单天叙事、留白、合计、小图
│   │   ├── DayMiniMap.tsx             SVG 当天路线
│   │   ├── MapView.tsx                单日地图 + 剧本列 + 浮层详情
│   │   ├── StoryColumn.tsx            地图旁当天下文
│   │   ├── BoardView.tsx              指标 + 分天列 + 浮层详情
│   │   ├── StopDetail.tsx             StopFacts / ReasonSection / GuideSection / StopDetail
│   │   ├── LegDetail.tsx              路段详情
│   │   ├── LegRow.tsx                 路段摘要行
│   │   ├── DetailCard.tsx             桌面浮层 / 移动底部面板
│   │   ├── ResultSheet.tsx            移动端方案面板
│   │   └── DayTabs.tsx                由 canvas/ 迁入，新增 allowAll
│   ├── map/
│   │   ├── routeStyle.ts              新增：模式样式、强调状态、图例
│   │   └── TripMap.tsx                改造为单日组件
│   ├── budget/
│   │   └── BudgetPopover.tsx          改为接收 ResultBudget，不再自行计算
│   ├── share/
│   │   └── SharePage.tsx              改为只读 DossierView
│   └── workspace/
│       └── WorkspacePage.tsx          接入 ResultShell / ResultSheet、view、ResultSelection
├── index.css                          删除 vis-timeline 自定义样式
└── package.json                       移除 vis-timeline、vis-data
```

删除目录：`features/canvas/`、`features/timeline/`（含各自测试）。

测试文件：

```
web/src/features/result/
├── model.test.ts            派生：折叠交通、理由/攻略归位、留白、预算、缺失字段
├── ResultShell.test.tsx     默认手册、切换保持天/选中、viewOnMap
├── DossierView.test.tsx     概览、章节顺序、留白、来源、合计
├── MapView.test.tsx         单日渲染、点选/段选同步、降级
├── BoardView.test.tsx       指标、列合计、浮层详情、调整入口
├── DayMiniMap.test.tsx      几何、强调、无坐标降级、查看路线图
└── ResultSheet.test.tsx     移动端视图切换、一天一屏、关闭返回
web/src/features/map/routeStyle.test.ts   模式颜色/虚线/强调/图例
web/src/features/map/TripMap.test.tsx     更新：单日 props、折线点击、降级清单
web/src/features/workspace/WorkspacePage.test.tsx  更新：外壳接入、状态保持
web/src/features/share/SharePage.test.tsx          更新：只读手册、无切换器、无地图
web/src/features/budget/BudgetPopover.test.tsx     更新：接收预算对象
```

后端、迁移、事件类型与 `openapi` 均无改动。

## 技术决策

| 决策点 | 选择 | 理由 |
|--------|------|------|
| 结果数据来源 | 前端纯函数 `buildResultModel`，不改后端 | N7 边界不变；三视图与分享页同源；可脱离渲染单测 |
| 视图偏好存储 | `WorkspacePage` 组件状态 | F1 刷新回到手册；不引入 localStorage 或 URL 状态；桌面与移动面板共享 |
| 天切换语义 | 手册与地图按 `activeDay` 筛选 | 与现有 `DayTabs` 行为一致；移动端「一天一屏」无需第二套逻辑；「全部」保留连续阅读 |
| 看板天切换 | 不显示天 tabs，只用 `activeDay` 定位与高亮列 | 看板的价值是横向对比；避免三处切换器造成理解负担 |
| 交通块处理 | `transit` 块折叠为 `ResultLeg`，不再作为停靠点 | 交通块与相邻站点是同一段移动，分列会造成重复；段落式阅读需要「点—段—点」 |
| 路段几何 | `polyline / straight / none` 三态 | 有折线按真实走向；无折线明示直线估算；缺坐标跳过，不伪造路线 |
| 手册小图 | 纯 SVG + 等距投影 | 分享页无地图 Key 也能看路线；避免多个 AMap 实例；不增加配额与加载成本 |
| 地图实例 | 单例、单日、按几何签名重建覆盖物 | 从接口上杜绝多天叠加（F9）；文字类事件不重建地图（N1） |
| 路线样式 | 抽出 `routeStyle`，颜色沿用现有值 | 地图与小图严格一致（F18）；图例与强调集中维护 |
| 选中状态 | 单一 `ResultSelection` + `resolveResultSelection` | F2 跨视图保持；详情只有一个解析入口；目标失效自动清空 |
| 详情复用 | `StopDetail` / `LegDetail` 及子区块 | F14 三种视图同一措辞；手册内联与浮层共用实现 |
| 预算口径 | `BudgetPopover` 改为接收 `ResultBudget` | N2 单一数据源；删除工具栏内重复的 `budgetBreakdown` 调用 |
| 时间轴 | 删除 `ItineraryTimeline`、`timelineModel` 与 `vis-timeline` / `vis-data` | 新阅读模型取代甘特色块；减少依赖、样式与测试维护面 |
| 标题 / 摘要 / 每日主题 | 由城市、天数、停靠点标题确定性拼装 | 不新增 LLM 调用（N7）；结果可预测、可测试 |
| 节奏 | 日均停靠点 ≤ 4 且日均步行 < 3.5 km → 轻松；日均停靠点 ≥ 7 或日均步行 ≥ 6 km → 紧凑；其余适中 | 只用已有统计；规则可解释、可调整 |
| 留白 | 只统计 ≥ 45 分钟的空档；建议按时段规则 | 15 分钟空档标「未排」是噪音；固定规则可测 |
| 提醒来源 | `research.notes` 在前、`warnings` 在后，去重 | 与现有 Inspector 口径一致；无提醒不渲染 |
| 链接安全 | 模型层过滤 http/https，渲染层再设 `target="_blank" rel="noopener noreferrer"` | 双保险；N5；所有外部文本按文本节点渲染，AMap 标记内容继续 `escapeHTML` |
| 移动端 | `ResultSheet` 复用 `ResultShell`，地图填满面板 | 一套组件与状态；F16 的三种形态只是布局差异 |
| 看板折叠 | 移动端用原生 `details / summary` | 不新增依赖（N8）；键盘与读屏原生可用 |
| 错误展示 | 只把住宿切换的错误送到工具栏；调整错误留在 Composer | 就地可见且不串台；用 `lastAction` 过滤 `session.error` |
| 滚动 | 内容容器 `scrollTo` + 章节 ref | 简单可靠；不做逐像素恢复，避免 scroll anchoring 干扰 |
| 测试 | 纯模型单测 + RTL + mock AMap | N10；不依赖真实 Key、真实 LLM 或外部插件 |
| 依赖 | 不新增，移除 `vis-timeline` 与 `vis-data` | N8；包体更小，删除无人维护的时间轴样式 |

### 取舍说明

- **筛选 vs 滚动定位**：手册采用筛选，选中某天后只读该天，与移动端一天一屏一致；代价是失去「连续翻完所有天」的默认视野，用「全部」恢复。这是为了让三种视图对「当前天」只有一个含义。
- **小图不是地图**：小图只表达顺序与走向，不追求路网精度；精确路线与缩放交给地图视图。这样分享页可以零依赖地展示路线。
- **不自动播放/动画**：减少动态效果与 N3 一致；切换只做滚动与高亮，不做过渡动画，也避免 100ms 预算被动画占用。
- **删除时间轴的风险**：现有 `ItineraryTimeline` 与 `timelineModel` 的测试会一并删除，覆盖由 `model.test.ts`、`DayChapter`/`MapView`/`BoardView` 测试与既有端到端用例承接；`index.css` 中的 `.vis-*` 样式同步清理。