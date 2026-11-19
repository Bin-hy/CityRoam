import { expect, test, type Page, type Route } from "@playwright/test";

const planV1 = {
  city: "beijing",
  days: [
    {
      date: "2026-11-20",
      start_time: "09:00",
      end_time: "18:00",
      day_cost_yuan: 120,
      blocks: [
        { id: "b1", type: "poi", title: "故宫", start: "09:00", end: "11:00", duration_min: 120, cost_yuan: 60 },
        { id: "t1", type: "transit", title: "步行前往景山", start: "11:00", end: "11:20", duration_min: 20, cost_yuan: 0 },
        { id: "b2", type: "poi", title: "景山公园", start: "11:20", end: "12:00", duration_min: 40, cost_yuan: 2 },
      ],
    },
    {
      date: "2026-11-21",
      start_time: "09:00",
      end_time: "17:00",
      day_cost_yuan: 60,
      blocks: [
        { id: "b3", type: "poi", title: "天坛", start: "09:00", end: "11:00", duration_min: 120, cost_yuan: 34 },
      ],
    },
  ],
  totals: { cost_yuan: 180, poi_count: 3, walking_km: 3.4 },
};

const planV2 = {
  ...planV1,
  days: [
    planV1.days[0],
    {
      ...planV1.days[1],
      day_cost_yuan: 50,
      blocks: [
        { id: "b3", type: "poi", title: "国家博物馆", start: "09:30", end: "11:30", duration_min: 120, cost_yuan: 0 },
      ],
    },
  ],
  totals: { cost_yuan: 160, poi_count: 3, walking_km: 2.6 },
};

interface MockState {
  status: "awaiting_user" | "completed" | "queued";
  currentRun: "run-1" | "run-2";
  versionSeq: number;
  accepted: boolean;
}

function newState(): MockState {
  return { status: "awaiting_user", currentRun: "run-1", versionSeq: 1, accepted: false };
}

function version(state: MockState, seq: number) {
  return {
    id: `v${seq}`,
    trip_id: "trip-e2e",
    run_id: "run-1",
    seq,
    plan: seq === 2 ? planV2 : planV1,
    constraints: { city: "beijing", days: 2 },
    violations: [],
    status: seq === state.versionSeq && state.accepted ? "accepted" : "draft",
    created_at: "2026-11-19T10:00:00Z",
  };
}

function trip(state: MockState) {
  return {
    id: "trip-e2e",
    city: "beijing",
    days: 2,
    status: state.status,
    constraints: { city: "beijing", days: 2 },
    created_at: "2026-11-19T10:00:00Z",
    updated_at: "2026-11-19T10:00:00Z",
    latest_run: {
      id: state.currentRun,
      mode: state.currentRun === "run-2" ? "adjustment" : "initial",
      status: state.status,
      stage: state.currentRun === "run-2" && state.status === "awaiting_user" ? "proposal" : state.status === "completed" ? "completed" : "present",
      round: 0,
      created_at: "2026-11-19T10:00:00Z",
    },
    latest_version: version(state, state.versionSeq),
  };
}

function sse(events: Array<{ seq: number; type: string; payload: Record<string, unknown> }>): string {
  return events
    .map((event) => `id: ${event.seq}\nevent: ${event.type}\ndata: ${JSON.stringify(event.payload)}\n\n`)
    .join("");
}

function initialEvents(): string {
  return sse([
    { seq: 1, type: "run.started", payload: { input: "北京两天，历史与博物馆" } },
    { seq: 2, type: "constraint.parsed", payload: { constraints: { city: "beijing", days: 2 } } },
    { seq: 3, type: "candidate.retrieved", payload: { count: 8, attempts: 1 } },
    { seq: 4, type: "plan.drafted", payload: {} },
    { seq: 5, type: "validation.result", payload: { blocking: 0, warnings: 0, violations: [] } },
    { seq: 6, type: "plan.updated", payload: { seq: 1, violations: [] } },
    { seq: 7, type: "run.awaiting_user", payload: { stage: "present", missing: [], proposal: null } },
  ]);
}

function adjustmentEvents(): string {
  return sse([
    { seq: 1, type: "run.started", payload: { input: "第二天下午轻松一点" } },
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
          impact: "阻断 0→0，警告 0→0，费用 180→160 元，步行 3.4→2.6 公里",
          warnings: ["周一多数博物馆闭馆，请确认日期"],
        },
      },
    },
  ]);
}

async function installApiMocks(page: Page, state: MockState) {
  await page.route("**/*", async (route: Route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname;
    if (!path.startsWith("/api/")) {
      await route.continue();
      return;
    }
    const json = (body: unknown, status = 200) => route.fulfill({ status, json: body });

    if (path === "/api/trips" && request.method() === "POST") {
      return json({ trip_id: "trip-e2e", run_id: "run-1", events_url: "/api/runs/run-1/events" }, 202);
    }
    if (path === "/api/trips/trip-e2e" && request.method() === "GET") {
      return json(trip(state));
    }
    if (path === "/api/trips/trip-e2e/versions") {
      return json({ versions: [version(state, 2), version(state, 1)] });
    }
    if (path === "/api/trips/trip-e2e/diff") {
      return json({ from: 1, to: 2, diff: { days: [], blocks: [] } });
    }
    if (path === "/api/runs/run-1/events") {
      return route.fulfill({
        status: 200,
        headers: { "content-type": "text/event-stream", "cache-control": "no-cache" },
        body: initialEvents(),
      });
    }
    if (path === "/api/runs/run-2/events") {
      return route.fulfill({
        status: 200,
        headers: { "content-type": "text/event-stream", "cache-control": "no-cache" },
        body: adjustmentEvents(),
      });
    }
    if (path.endsWith("/resume") && request.method() === "POST") {
      const runID = path.split("/")[3];
      state.status = "completed";
      state.accepted = true;
      if (runID === "run-2") state.versionSeq = 2;
      return json({ run_id: runID, status: "completed" }, 202);
    }
    if (path === "/api/trips/trip-e2e/adjustments" && request.method() === "POST") {
      state.currentRun = "run-2";
      state.status = "queued";
      return json({ run_id: "run-2", events_url: "/api/runs/run-2/events" }, 202);
    }
    if (path === "/api/trips/trip-e2e/share" && request.method() === "POST") {
      return json(
        {
          slug: "e2e-slug",
          url: "/api/share/e2e-slug",
          trip_id: "trip-e2e",
          version_seq: state.versionSeq,
          created_at: "2026-11-19T10:00:00Z",
        },
        201,
      );
    }
    return json({ error: { code: "not_found", message: `unmocked ${path}`, request_id: "e2e" } }, 404);
  });
}

async function startTrip(page: Page) {
  await page.goto("/");
  await page.getByLabel("行程需求").fill("北京两天，历史与博物馆，预算 800 元");
  await page.getByRole("button", { name: "开始规划" }).click();
  await expect(page).toHaveURL(/\/trips\/trip-e2e/);
  await expect(page.getByRole("button", { name: "接受方案" })).toBeVisible();
}

test("create, accept, adjust, and share a trip", async ({ page }) => {
  const state = newState();
  await installApiMocks(page, state);
  await startTrip(page);

  await expect(page.getByText("故宫").first()).toBeVisible();
  await expect(page.getByText("地图暂不可用")).toBeVisible();
  await expect(page.getByText(/未配置 VITE_AMAP_JS_KEY/)).toBeVisible();

  await page.getByRole("button", { name: "接受方案" }).click();
  await expect(page.getByLabel("调整需求")).toBeVisible();

  await page.getByLabel("调整需求").fill("第二天下午轻松一点，少走路");
  await page.getByRole("button", { name: "提交调整" }).click();

  await expect(page.getByText("调整提案")).toBeVisible();
  await expect(page.getByText("第二天改为国家博物馆，减少步行")).toBeVisible();
  await page.getByRole("button", { name: "接受调整" }).click();
  await expect(page.getByText("已完成").first()).toBeVisible();
  await expect(page.getByText("国家博物馆").first()).toBeVisible();

  await page.getByRole("button", { name: "分享" }).click();
  await expect(page.getByText("分享行程")).toBeVisible();
  await expect(page.getByLabel("分享链接")).toHaveValue(
    new URL("/api/share/e2e-slug", "http://127.0.0.1:5173").toString(),
  );
});

test("mobile layout fits without horizontal scrolling", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const state = newState();
  await installApiMocks(page, state);
  await startTrip(page);

  await expect(page.getByRole("tab", { name: "时间轴" })).toBeVisible();
  await page.getByRole("tab", { name: "地图" }).click();
  await expect(page.getByText("地图暂不可用")).toBeVisible();

  const overflow = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
  }));
  expect(overflow.scrollWidth).toBeLessThanOrEqual(overflow.clientWidth + 1);
});
