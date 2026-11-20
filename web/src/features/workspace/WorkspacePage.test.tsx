import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/api/client";
import { WorkspacePage } from "./WorkspacePage";

const fixture = vi.hoisted(() => {
  const itinerary = {
    city: "beijing",
    days: [
      {
        date: "2026-11-20",
        start_time: "09:00",
        end_time: "18:00",
        day_cost_yuan: 60,
        blocks: [
          { id: "b-1", type: "poi", poi_id: "p1", title: "故宫", start: "09:00", end: "11:00", duration_min: 120, cost_yuan: 60 },
        ],
      },
      {
        date: "2026-11-21",
        start_time: "09:00",
        end_time: "18:00",
        day_cost_yuan: 40,
        blocks: [
          { id: "b-2", type: "poi", poi_id: "p2", title: "天坛", start: "09:00", end: "11:00", duration_min: 120, cost_yuan: 40 },
        ],
      },
    ],
    explanations: [{ poi_id: "p1", text: "中轴线起点。" }],
    totals: { cost_yuan: 100, poi_count: 2, walking_km: 1.2 },
  };

  const trip = {
    id: "trip-1",
    city: "beijing",
    days: 2,
    status: "awaiting_user",
    constraints: {},
    created_at: "2026-11-19T10:00:00Z",
    updated_at: "2026-11-19T10:00:00Z",
    latest_run: {
      id: "run-1",
      mode: "initial",
      status: "awaiting_user",
      stage: "proposal",
      round: 0,
      created_at: "2026-11-19T10:00:00Z",
    },
    latest_version: {
      id: "v1",
      trip_id: "trip-1",
      run_id: "run-1",
      seq: 1,
      plan: itinerary,
      constraints: {},
      violations: [],
      status: "accepted",
      created_at: "2026-11-19T10:00:00Z",
    },
  };

  const events = [
    { seq: 1, type: "run.started", payload: { mode: "initial", input: "帮我安排北京两日游" } },
    {
      seq: 2,
      type: "constraint.parsed",
      payload: { constraints: { city: "beijing", days: 2, budget: { total_yuan: 2000 } } },
    },
    { seq: 3, type: "research.started", payload: { tools: ["search_hotels"] } },
    {
      seq: 4,
      type: "plugin.tool_called",
      payload: { name: "search_hotels", provider: "amap", call_index: 0, query: "北京 · 2 晚 · ≤¥500/晚" },
    },
    {
      seq: 5,
      type: "plugin.tool_completed",
      payload: {
        name: "search_hotels",
        provider: "amap",
        call_index: 0,
        status: "ok",
        latency_ms: 820,
        result_count: 1,
        preview: [{ title: "全季酒店", subtitle: "4 星 · 8.8 分", price_yuan: 480 }],
      },
    },
    { seq: 6, type: "plan.updated", payload: { seq: 1, plan: itinerary, violations: [] } },
    {
      seq: 7,
      type: "validation.result",
      payload: {
        checked: 2,
        blocking: 1,
        warnings: 0,
        violations: [
          { day: 2, level: "blocking", code: "cramped", message: "天坛停留偏短" },
        ],
      },
    },
    {
      seq: 8,
      type: "run.awaiting_user",
      payload: {
        stage: "proposal",
        proposal: {
          base_version: 1,
          scope: "day",
          day_index: 1,
          summary: "把第二天改成天坛",
          impact: "步行减少 1.2 km",
          warnings: [],
        },
      },
    },
  ];

  const tripState: {
    isPending: boolean;
    isError: boolean;
    error: unknown;
    data: typeof trip | undefined;
    refetch: () => Promise<unknown>;
  } = {
    isPending: false,
    isError: false,
    error: null,
    data: trip,
    refetch: () => Promise.resolve(),
  };

  const actions = {
    supplement: vi.fn(async () => true),
    revise: vi.fn(async () => true),
    accept: vi.fn(async () => true),
    reject: vi.fn(async () => true),
    cancel: vi.fn(async () => true),
    resolveResearch: vi.fn(async () => true),
    adjust: vi.fn(async () => true),
    replanLodging: vi.fn(async () => true),
    addSystemNotice: vi.fn(),
  };

  const session = {
    activeRunId: "run-1",
    turns: [],
    events,
    connected: true,
    pending: null,
    error: null,
    notices: [],
    actions,
  };

  return { trip, tripState, session, actions };
});

vi.mock("@/features/trips/useTrip", () => ({
  useTrip: () => fixture.tripState,
}));

vi.mock("@/features/conversation/useSession", () => ({
  useSession: () => fixture.session,
}));

vi.mock("@/features/map/TripMap", () => ({
  TripMap: ({
    day,
    selectedStopID,
    onSelectStop,
  }: {
    day: { index: number };
    selectedStopID?: string | null;
    onSelectStop?: (id: string) => void;
  }) => (
    <div data-testid="trip-map" data-day={String(day.index)} data-selected={selectedStopID ?? ""}>
      <button type="button" onClick={() => onSelectStop?.("b-1")}>
        地图选点
      </button>
    </div>
  ),
}));

function renderWorkspace() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={["/trips/trip-1"]}>
        <Routes>
          <Route path="/trips/:tripId" element={<WorkspacePage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function setDesktop(desktop: boolean) {
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    configurable: true,
    value: (query: string) => ({
      matches: desktop,
      media: query,
      onchange: null,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
      addListener: () => undefined,
      removeListener: () => undefined,
      dispatchEvent: () => false,
    }),
  });
}

describe("WorkspacePage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    fixture.tripState.isPending = false;
    fixture.tripState.isError = false;
    fixture.tripState.error = null;
    fixture.tripState.data = fixture.trip;
    setDesktop(true);
  });

  it("renders the conversation thread with tools, plan and decisions", () => {
    renderWorkspace();
    const log = screen.getByRole("log", { name: "对话记录" });

    expect(within(log).getByText("帮我安排北京两日游")).toBeInTheDocument();
    expect(within(log).getByText(/已理解：/)).toBeInTheDocument();
    expect(within(log).getByText("住宿搜索")).toBeInTheDocument();
    expect(within(log).getByText("北京 · 2 晚 · ≤¥500/晚")).toBeInTheDocument();
    expect(within(log).getByRole("region", { name: "调整提案" })).toHaveTextContent("把第二天改成天坛");
    expect(within(log).getByText("行程方案")).toBeInTheDocument();
    expect(screen.queryByText(/trip-1/)).not.toBeInTheDocument();
  });

  it("opens on the dossier and keeps day and view across switches", async () => {
    renderWorkspace();

    expect(screen.getByRole("tab", { name: "手册" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("article", { name: "第 1 天" })).toBeInTheDocument();
    expect(screen.getByRole("article", { name: "第 2 天" })).toBeInTheDocument();

    await userEvent.click(screen.getByRole("tab", { name: /D2/ }));
    expect(screen.queryByRole("article", { name: "第 1 天" })).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("tab", { name: "地图" }));
    expect(screen.getByTestId("trip-map")).toHaveAttribute("data-day", "1");
    expect(screen.getByRole("tab", { name: /D2/ })).toHaveAttribute("aria-selected", "true");

    await userEvent.click(screen.getByRole("tab", { name: "看板" }));
    expect(screen.getByRole("article", { name: "第 2 天" })).toHaveAttribute("data-active", "true");
    expect(screen.queryByRole("tablist", { name: "行程天数" })).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("tab", { name: "手册" }));
    expect(screen.getByRole("article", { name: "第 2 天" })).toBeInTheDocument();
    expect(screen.queryByRole("article", { name: "第 1 天" })).not.toBeInTheDocument();
  });

  it("jumps from a validation message to the selected stop", async () => {
    renderWorkspace();

    await userEvent.click(screen.getByRole("button", { name: /天坛停留偏短/ }));

    const day = screen.getByRole("article", { name: "第 2 天" });
    expect(
      within(day).getByRole("button", { name: /天坛/, pressed: true }),
    ).toBeInTheDocument();
    expect(screen.queryByRole("article", { name: "第 1 天" })).not.toBeInTheDocument();
  });

  it("wires decision, composer prefill and the budget lodging toggle", async () => {
    renderWorkspace();

    await userEvent.click(screen.getByRole("button", { name: "接受调整" }));
    expect(fixture.actions.accept).toHaveBeenCalled();

    await userEvent.click(screen.getAllByRole("button", { name: "调整这一天" })[0]);
    expect(screen.getByLabelText("行程输入")).toHaveValue("调整第 1 天：");

    await userEvent.click(screen.getByRole("button", { name: /总计/ }));
    await userEvent.click(await screen.findByRole("button", { name: "不计住宿" }));
    expect(fixture.actions.replanLodging).toHaveBeenCalledWith(
      { budget: { include_lodging: false } },
      1,
    );
  });

  it("shows budget and version triggers on desktop without a canvas sheet", () => {
    renderWorkspace();

    expect(screen.getByRole("button", { name: /总计/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /v1/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "查看方案" })).not.toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("opens the result sheet on mobile with the three views and day tabs", async () => {
    setDesktop(false);
    renderWorkspace();

    const entries = screen.getAllByRole("button", { name: "查看方案" });
    expect(entries.length).toBeGreaterThan(0);
    await userEvent.click(entries[0]);

    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getAllByText("北京 · 2 天").length).toBeGreaterThan(0);
    expect(within(dialog).getByRole("tab", { name: "手册" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(within(dialog).getByRole("tab", { name: "地图" })).toBeInTheDocument();
    expect(within(dialog).getByRole("tab", { name: "看板" })).toBeInTheDocument();
    expect(within(dialog).queryByRole("tab", { name: "时间轴" })).not.toBeInTheDocument();
    expect(within(dialog).getByRole("tablist", { name: "行程天数" })).toBeInTheDocument();
    expect(within(dialog).queryByRole("tab", { name: "全部" })).not.toBeInTheDocument();

    await userEvent.click(within(dialog).getByRole("tab", { name: "地图" }));
    expect(within(dialog).getByTestId("trip-map")).toHaveAttribute("data-day", "0");
  });

  it("keeps the unauthorized and not-found branches", () => {
    fixture.tripState.isError = true;
    fixture.tripState.data = undefined;
    fixture.tripState.error = new ApiError(401, "unauthorized", "未登录", "req-1");
    const view = renderWorkspace();
    expect(screen.getByText("会话已过期")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "重新登录" })).toBeInTheDocument();
    view.unmount();

    fixture.tripState.error = new ApiError(404, "not_found", "not found", "req-2");
    renderWorkspace();
    expect(screen.getByText("行程不存在")).toBeInTheDocument();
  });
});