import http from "node:http";
import type { AddressInfo } from "node:net";
import { expect, test, type Page, type Route } from "@playwright/test";

interface SseEvent {
  seq: number;
  type: string;
  payload: Record<string, unknown>;
}

class SSEHub {
  private runs = new Map<string, SseEvent[]>();
  private clients = new Map<string, Set<{ res: http.ServerResponse; timer: NodeJS.Timeout }>>();
  private done = new Set<string>();
  private server: http.Server | null = null;
  origin = "";

  async start() {
    if (this.server) return;
    this.server = http.createServer((req, res) => this.handle(req, res));
    await new Promise<void>((resolve) => this.server?.listen(0, "127.0.0.1", () => resolve()));
    const address = this.server.address() as AddressInfo;
    this.origin = `http://127.0.0.1:${address.port}`;
  }

  async stop() {
    for (const set of this.clients.values()) {
      for (const client of set) {
        clearInterval(client.timer);
        client.res.end();
      }
    }
    this.clients.clear();
    this.server?.closeAllConnections();
    await new Promise<void>((resolve) => (this.server ? this.server.close(() => resolve()) : resolve()));
    this.server = null;
  }

  reset() {
    this.runs.clear();
    this.clients.clear();
    this.done.clear();
  }

  seed(runId: string, events: SseEvent[]) {
    this.runs.set(runId, [...events]);
  }

  push(runId: string, event: SseEvent) {
    const events = this.runs.get(runId) ?? [];
    events.push(event);
    this.runs.set(runId, events);
    const clients = this.clients.get(runId);
    if (!clients) return;
    for (const client of clients) client.res.write(formatEvent(event));
    if (["run.completed", "run.failed", "run.cancelled"].includes(event.type)) {
      this.done.add(runId);
      for (const client of clients) {
        clearInterval(client.timer);
        client.res.end();
      }
      this.clients.delete(runId);
    }
  }

  private handle(req: http.IncomingMessage, res: http.ServerResponse) {
    const match = /^\/events\/(.+)$/.exec(new URL(req.url ?? "/", "http://hub").pathname);
    if (!match) {
      res.writeHead(404);
      res.end();
      return;
    }
    const runId = decodeURIComponent(match[1]);
    res.writeHead(200, {
      "content-type": "text/event-stream; charset=utf-8",
      "cache-control": "no-cache",
      connection: "keep-alive",
    });
    const lastID = Number(req.headers["last-event-id"] ?? 0) || 0;
    for (const event of this.runs.get(runId) ?? []) {
      if (event.seq > lastID) res.write(formatEvent(event));
    }
    if (this.done.has(runId)) {
      res.end();
      return;
    }
    const client = { res, timer: setInterval(() => res.write(": ping\n\n"), 4000) };
    let clients = this.clients.get(runId);
    if (!clients) {
      clients = new Set();
      this.clients.set(runId, clients);
    }
    clients.add(client);
    req.on("close", () => {
      clearInterval(client.timer);
      clients?.delete(client);
    });
  }
}

function formatEvent(event: SseEvent): string {
  return `id: ${event.seq}\nevent: ${event.type}\ndata: ${JSON.stringify(event.payload)}\n\n`;
}

const planV1 = {
  city: "beijing",
  days: [
    {
      date: "2026-11-20",
      start_time: "09:00",
      end_time: "18:00",
      day_cost_yuan: 962,
      blocks: [
        {
          id: "b1",
          type: "poi",
          poi_id: "poi-1",
          lat: 39.9163,
          lng: 116.3972,
          title: "故宫",
          start: "09:00",
          end: "11:00",
          duration_min: 120,
          cost_yuan: 60,
        },
        { id: "t1", type: "transit", title: "步行前往景山", start: "11:00", end: "11:20", duration_min: 20, cost_yuan: 0 },
        {
          id: "b2",
          type: "poi",
          poi_id: "poi-2",
          lat: 39.9248,
          lng: 116.3968,
          title: "景山公园",
          start: "11:20",
          end: "12:00",
          duration_min: 40,
          cost_yuan: 2,
        },
      ],
    },
    {
      date: "2026-11-21",
      start_time: "09:00",
      end_time: "17:00",
      day_cost_yuan: 934,
      blocks: [
        {
          id: "b3",
          type: "poi",
          poi_id: "poi-3",
          lat: 39.8822,
          lng: 116.4066,
          title: "天坛",
          start: "09:00",
          end: "11:00",
          duration_min: 120,
          cost_yuan: 34,
        },
      ],
    },
  ],
  lodging: {
    hotel: {
      id: "hotel-1",
      name: "王府井文华酒店",
      address: "东城区王府井大街 1 号",
      lat: 39.9142,
      lng: 116.4105,
      star: 5,
      score: 4.8,
      price_per_night_yuan: 900,
      price_estimated: true,
      booking_url: "https://example.com/hotel",
      provider: "amap",
    },
    nights: 1,
    nightly_yuan: 900,
    total_yuan: 900,
    anchor_days: [0, 1],
    include_in_budget: true,
    price_estimated: true,
    source: "research",
  },
  explanations: [
    { poi_id: "poi-1", text: "世界现存规模最大的木质结构宫殿群。" },
    { poi_id: "poi-3", text: "明清两代皇帝祭天的场所，中轴线地标。" },
  ],
  totals: { cost_yuan: 1896, poi_count: 3, walking_km: 3.4, lodging_yuan: 900 },
};

const planV2 = {
  ...planV1,
  days: [
    planV1.days[0],
    {
      ...planV1.days[1],
      blocks: [
        {
          id: "b3",
          type: "poi",
          poi_id: "poi-4",
          lat: 39.9047,
          lng: 116.4011,
          title: "国家博物馆",
          start: "09:30",
          end: "11:30",
          duration_min: 120,
          cost_yuan: 0,
        },
      ],
    },
  ],
  explanations: [{ poi_id: "poi-1", text: "世界现存规模最大的木质结构宫殿群。" }],
  totals: { cost_yuan: 1856, poi_count: 3, walking_km: 2.6, lodging_yuan: 900 },
};

const researchBrief = {
  status: "ok",
  accommodation: {
    mentioned: true,
    selected: {
      name: "王府井文华酒店",
      address: "东城区王府井大街 1 号",
      star: 5,
      score: 4.8,
      price_per_night_yuan: 900,
      price_estimated: true,
      booking_url: "https://example.com/hotel",
      provider: "amap",
    },
    anchor_days: [0, 1],
    nights: 1,
    nightly_yuan: 900,
    total_yuan: 900,
    price_estimated: true,
    include_in_budget: true,
  },
  guides: [
    {
      poi_id: "poi-1",
      note: "上午人少，建议 9 点前入园。",
      citations: [{ title: "北京旅行攻略", url: "https://example.com/guide", site: "示例旅行" }],
    },
  ],
  notes: ["故宫周一闭馆，请确认出行日期。"],
  warnings: ["热门景点建议提前预约。"],
};

type Phase = "initial" | "accepted" | "adjusting" | "adjusted";

interface MainState {
  phase: Phase;
}

function version(seq: number, createdAt = "2026-11-19T10:00:00Z") {
  return {
    id: `v${seq}`,
    trip_id: "trip-main",
    run_id: seq === 2 ? "run-main-2" : "run-main-1",
    seq,
    plan: seq === 2 ? planV2 : planV1,
    constraints: { city: "beijing", days: 2 },
    violations: [],
    status: seq === 2 && state.phase !== "adjusted" ? "draft" : "accepted",
    created_at: createdAt,
    research: researchBrief,
  };
}

const state: MainState = { phase: "initial" };

function trip() {
  const adjusted = state.phase === "adjusted";
  const completed = adjusted || state.phase === "accepted";
  const runID = adjusted || state.phase === "adjusting" ? "run-main-2" : "run-main-1";
  return {
    id: "trip-main",
    city: "beijing",
    days: 2,
    status: completed ? "completed" : "awaiting_user",
    constraints: { city: "beijing", days: 2 },
    created_at: "2026-11-19T10:00:00Z",
    updated_at: "2026-11-19T10:00:00Z",
    latest_run: {
      id: runID,
      mode: runID === "run-main-2" ? "adjustment" : "initial",
      status: completed ? "completed" : "awaiting_user",
      stage: state.phase === "adjusting" ? "proposal" : "present",
      round: 0,
      created_at: "2026-11-19T10:00:00Z",
    },
    latest_version: version(adjusted ? 2 : 1),
  };
}

function initialEvents(): SseEvent[] {
  return [
    { seq: 1, type: "run.started", payload: { mode: "initial", input: "北京两天，历史与博物馆" } },
    { seq: 2, type: "constraint.parsed", payload: { constraints: { city: "北京", days: 2 } } },
    { seq: 3, type: "research.started", payload: { tools: ["search_travel_notes", "search_hotels"] } },
    {
      seq: 4,
      type: "plugin.tool_called",
      payload: { name: "search_travel_notes", provider: "jina", query: "北京 故宫 攻略", call_index: 0 },
    },
    {
      seq: 5,
      type: "plugin.tool_completed",
      payload: {
        name: "search_travel_notes",
        call_index: 0,
        status: "ok",
        latency_ms: 1200,
        result_count: 2,
        cached: true,
        preview: [
          { title: "故宫游览路线", subtitle: "避开上午高峰" },
          { title: "景山看日落", subtitle: "傍晚最佳" },
        ],
      },
    },
    {
      seq: 6,
      type: "plugin.tool_called",
      payload: { name: "search_hotels", provider: "amap", query: "北京 王府井 酒店", call_index: 1 },
    },
    {
      seq: 7,
      type: "plugin.tool_completed",
      payload: { name: "search_hotels", call_index: 1, status: "ok", result_count: 1, preview: [] },
    },
    { seq: 8, type: "research.completed", payload: { status: "ok", guide_matches: 1, hotel: "王府井文华酒店" } },
    { seq: 9, type: "candidate.retrieved", payload: { count: 8, attempts: 1 } },
    { seq: 10, type: "plan.drafted", payload: {} },
    { seq: 11, type: "validation.result", payload: { checked: 6, blocking: 0, warnings: 0, violations: [] } },
    { seq: 12, type: "plan.updated", payload: { seq: 1, plan: planV1, violations: [] } },
    { seq: 13, type: "run.awaiting_user", payload: { stage: "present", missing: [], proposal: null } },
  ];
}

function adjustmentEvents(): SseEvent[] {
  return [
    { seq: 1, type: "run.started", payload: { mode: "adjustment", input: "第二天下午轻松一点，少走路" } },
    {
      seq: 2,
      type: "run.awaiting_user",
      payload: {
        stage: "proposal",
        missing: [],
        proposal: {
          base_version: 1,
          scope: "day",
          day_index: 1,
          summary: "第二天改为国家博物馆，减少步行",
          impact: "阻断 0→0，警告 0→0，费用 1896→1856 元，步行 3.4→2.6 公里",
          warnings: ["周一多数博物馆闭馆，请确认日期"],
        },
      },
    },
  ];
}

function legacyEvents(): SseEvent[] {
  return [
    { seq: 1, type: "run.started", payload: { input: "旧行程" } },
    { seq: 2, type: "plugin.tool_called", payload: { name: "search_hotels", call_index: 0 } },
    { seq: 3, type: "plugin.tool_completed", payload: { name: "search_hotels", call_index: 0, status: "ok" } },
    {
      seq: 4,
      type: "plan.updated",
      payload: {
        seq: 1,
        violations: [],
        plan: {
          city: "beijing",
          days: [
            {
              date: "2026-11-20",
              start_time: "09:00",
              end_time: "12:00",
              day_cost_yuan: 0,
              blocks: [
                {
                  id: "o1",
                  type: "poi",
                  poi_id: "poi-old",
                  title: "旧版景点",
                  start: "09:00",
                  end: "12:00",
                  duration_min: 180,
                  cost_yuan: 0,
                },
              ],
            },
          ],
          totals: { cost_yuan: 0, poi_count: 1, walking_km: 1 },
        },
      },
    },
    { seq: 5, type: "run.awaiting_user", payload: { stage: "present", missing: [], proposal: null } },
  ];
}

async function installMainMocks(page: Page, hub: SSEHub) {
  state.phase = "initial";
  hub.reset();
  hub.seed("run-main-1", initialEvents());
  hub.seed("run-main-2", adjustmentEvents());

  await page.route("**/*", async (route: Route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname;
    if (url.hostname.endsWith("amap.com")) {
      await route.abort();
      return;
    }
    if (!path.startsWith("/api/")) {
      await route.continue();
      return;
    }
    const events = /^\/api\/runs\/([^/]+)\/events$/.exec(path);
    if (events) {
      await route.continue({ url: `${hub.origin}/events/${events[1]}` });
      return;
    }
    const json = (body: unknown, status = 200) => route.fulfill({ status, json: body });

    if (path === "/api/auth/me") {
      return json({ enabled: false, authenticated: false, provider_name: "" });
    }
    if (path === "/api/trips" && request.method() === "GET") {
      return json({ trips: [] });
    }
    if (path === "/api/trips" && request.method() === "POST") {
      return json({ trip_id: "trip-main", run_id: "run-main-1", events_url: "/api/runs/run-main-1/events" }, 202);
    }
    if (path === "/api/trips/trip-main" && request.method() === "GET") {
      return json(trip());
    }
    if (path === "/api/trips/trip-main/versions") {
      return json({ versions: [version(2), version(1)] });
    }
    if (path === "/api/runs/run-main-1/resume" && request.method() === "POST") {
      state.phase = "accepted";
      hub.push("run-main-1", { seq: 14, type: "run.completed", payload: { seq: 1 } });
      return json({ run_id: "run-main-1", status: "completed" }, 202);
    }
    if (path === "/api/trips/trip-main/adjustments" && request.method() === "POST") {
      state.phase = "adjusting";
      return json({ run_id: "run-main-2", status: "queued", events_url: "/api/runs/run-main-2/events" }, 202);
    }
    if (path === "/api/runs/run-main-2/resume" && request.method() === "POST") {
      state.phase = "adjusted";
      hub.push("run-main-2", { seq: 3, type: "run.completed", payload: { seq: 2 } });
      return json({ run_id: "run-main-2", status: "completed" }, 202);
    }
    if (path === "/api/trips/trip-main/share" && request.method() === "POST") {
      return json(
        {
          slug: "e2e-slug",
          url: "/api/share/e2e-slug",
          trip_id: "trip-main",
          version_seq: state.phase === "adjusted" ? 2 : 1,
          created_at: "2026-11-19T10:00:00Z",
        },
        201,
      );
    }
    if (path === "/api/share/e2e-slug" && request.method() === "GET") {
      return json({
        slug: "e2e-slug",
        trip_id: "trip-main",
        created_at: "2026-11-19T10:00:00Z",
        version: version(state.phase === "adjusted" ? 2 : 1),
      });
    }
    return json({ error: { code: "not_found", message: `unmocked ${path}`, request_id: "e2e" } }, 404);
  });
}

async function installLegacyMocks(page: Page, hub: SSEHub) {
  hub.reset();
  hub.seed("run-old", legacyEvents());

  await page.route("**/*", async (route: Route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname;
    if (url.hostname.endsWith("amap.com")) {
      await route.abort();
      return;
    }
    if (!path.startsWith("/api/")) {
      await route.continue();
      return;
    }
    const events = /^\/api\/runs\/([^/]+)\/events$/.exec(path);
    if (events) {
      await route.continue({ url: `${hub.origin}/events/${events[1]}` });
      return;
    }
    const json = (body: unknown, status = 200) => route.fulfill({ status, json: body });
    if (path === "/api/auth/me") {
      return json({ enabled: false, authenticated: false, provider_name: "" });
    }
    if (path === "/api/trips/trip-old" && request.method() === "GET") {
      return json({
        id: "trip-old",
        city: "beijing",
        days: 1,
        status: "awaiting_user",
        constraints: { city: "beijing", days: 1 },
        created_at: "2026-10-01T10:00:00Z",
        updated_at: "2026-10-01T10:00:00Z",
        latest_run: {
          id: "run-old",
          mode: "initial",
          status: "awaiting_user",
          stage: "present",
          round: 0,
          created_at: "2026-10-01T10:00:00Z",
        },
        latest_version: {
          id: "v-old",
          trip_id: "trip-old",
          run_id: "run-old",
          seq: 1,
          plan: {
            city: "beijing",
            days: [],
            totals: { cost_yuan: 0, poi_count: 0, walking_km: 0 },
          },
          constraints: { city: "beijing", days: 1 },
          violations: [],
          status: "accepted",
          created_at: "2026-10-01T10:00:00Z",
        },
      });
    }
    return json({ error: { code: "not_found", message: `unmocked ${path}`, request_id: "e2e" } }, 404);
  });
}

const hub = new SSEHub();

test.beforeAll(async () => {
  await hub.start();
});

test.afterAll(async () => {
  await hub.stop();
});

test("create, converse, inspect, accept, adjust, and share a trip", async ({ page }) => {
  await installMainMocks(page, hub);

  await page.goto("/");
  await page.getByLabel("行程需求").fill("北京两天，历史与博物馆，预算 800 元");
  await page.getByRole("button", { name: "开始规划" }).click();
  await expect(page).toHaveURL(/\/trips\/trip-main/);

  await expect(page.getByText(/已理解：北京 · 2 天/)).toBeVisible();
  await expect(page.getByRole("button", { name: /旅行攻略/ })).toBeVisible();
  await expect(page.getByText(/参考了 1 篇攻略/)).toBeVisible();

  await page.getByRole("button", { name: /旅行攻略/ }).click();
  await expect(page.getByText("避开上午高峰")).toBeVisible();
  await expect(page.getByText("缓存命中")).toBeVisible();
  await expect(page.getByText("2 条")).toBeVisible();

  await expect(page.getByText("地图暂不可用")).toBeVisible();
  await expect(page.getByText(/时间轴和预算仍可正常使用/)).toBeVisible();
  await expect(page.getByRole("region", { name: "行程概览" }).first()).toBeVisible();

  await page.getByRole("button", { name: /09:00 故宫/ }).first().click();
  await expect(page.getByRole("region", { name: "第 1 天 · 故宫" })).toBeVisible();
  await expect(page.getByText("世界现存规模最大的木质结构宫殿群。")).toBeVisible();
  await expect(page.getByText("上午人少，建议 9 点前入园。").first()).toBeVisible();

  await page.getByRole("button", { name: "接受方案" }).click();
  const thread = page.getByRole("log", { name: "对话记录" });
  await expect(thread.getByText("方案已确认，保存为版本 v1。")).toBeVisible();
  await expect(thread.getByText("已接受。")).toBeVisible();

  await page.getByLabel("行程输入").fill("第二天下午轻松一点，少走路");
  await page.getByRole("button", { name: "发送" }).click();
  await expect(page.getByText("调整提案")).toBeVisible();
  await expect(page.getByText("第二天改为国家博物馆，减少步行")).toBeVisible();
  await expect(page.getByText("周一多数博物馆闭馆，请确认日期")).toBeVisible();
  await page.getByRole("button", { name: "接受调整" }).click();

  await expect(thread.getByText("方案已确认，保存为版本 v2。")).toBeVisible();
  await expect(page.getByText("国家博物馆").first()).toBeVisible();

  await page.getByRole("button", { name: "分享" }).click();
  await expect(page.getByText("分享行程")).toBeVisible();
  await expect(page.getByLabel("分享链接")).toHaveValue(
    new URL("/api/share/e2e-slug", page.url()).toString(),
  );

  await page.goto("/share/e2e-slug");
  await expect(page.getByRole("heading", { name: "北京 · 2 天" })).toBeVisible();
  await expect(page.getByText("只读")).toBeVisible();
  await expect(page.getByText("住宿与攻略")).toBeVisible();
  await expect(page.getByText("王府井文华酒店").first()).toBeVisible();
  await expect(page.getByText("外部摘要，仅供参考。")).toBeVisible();
  await expect(page.getByText("trip-main")).toHaveCount(0);
});

test("mobile conversation keeps the canvas behind a full-screen sheet", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await installMainMocks(page, hub);

  await page.goto("/");
  await page.getByLabel("行程需求").fill("北京两天，历史与博物馆");
  await page.getByRole("button", { name: "开始规划" }).click();
  await expect(page).toHaveURL(/\/trips\/trip-main/);

  await expect(page.getByLabel("行程输入")).toBeVisible();
  await expect(page.getByRole("button", { name: "查看方案" }).first()).toBeVisible();
  await expect(page.getByRole("heading", { name: "行程方案" })).toHaveCount(0);

  await page.getByRole("button", { name: "查看方案" }).first().click();
  const sheet = page.getByRole("dialog");
  await expect(sheet).toBeVisible();
  await expect(sheet.getByRole("tab", { name: "时间轴" })).toBeVisible();
  await expect(page.getByText("地图暂不可用")).toBeVisible();

  await sheet.getByRole("tab", { name: "时间轴" }).click();
  await expect(sheet.locator(".vis-timeline")).toBeVisible();

  await sheet.getByRole("tab", { name: "详情" }).click();
  await expect(sheet.getByRole("region", { name: "行程概览" })).toBeVisible();

  await page.keyboard.press("Escape");
  await expect(sheet).toHaveCount(0);

  const overflow = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
  }));
  expect(overflow.scrollWidth).toBeLessThanOrEqual(overflow.clientWidth + 1);
});

test("legacy events without query or preview degrade without errors", async ({ page }) => {
  await installLegacyMocks(page, hub);

  await page.goto("/trips/trip-old");
  const tool = page.getByRole("button", { name: /住宿搜索/ });
  await expect(tool).toBeVisible();
  await tool.click();
  await expect(page.getByText("未返回结果")).toBeVisible();

  await page.getByRole("button", { name: /09:00 旧版景点/ }).first().click();
  await expect(page.getByText("本次未生成额外说明。")).toBeVisible();
  await expect(page.getByRole("region", { name: "第 1 天 · 旧版景点" })).toBeVisible();
});
