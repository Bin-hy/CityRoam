import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
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
          { id: "b-1", type: "poi", title: "故宫", start: "09:00", end: "11:00", duration_min: 120, cost_yuan: 60, lat: 39.9163, lng: 116.3972 },
        ],
      },
      {
        date: "2026-11-21",
        start_time: "09:00",
        end_time: "18:00",
        day_cost_yuan: 40,
        blocks: [
          { id: "b-2", type: "poi", title: "天坛", start: "09:00", end: "11:00", duration_min: 120, cost_yuan: 40, lat: 39.8822, lng: 116.4066 },
        ],
      },
    ],
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
      stage: "present",
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
      status: "draft",
      created_at: "2026-11-19T10:00:00Z",
    },
  };

  const trace = {
    status: "awaiting_user",
    stage: "present",
    round: 0,
    lastSeq: 3,
    entries: [],
    violations: [],
    missing: [],
    error: null,
    proposal: null,
    connected: true,
  };

  return { trip, trace };
});

vi.mock("@/features/trips/useTrip", () => ({
  useTrip: () => ({
    isPending: false,
    isError: false,
    data: fixture.trip,
    refetch: () => Promise.resolve(),
  }),
}));

vi.mock("@/hooks/useRunStream", () => ({
  useRunStream: () => fixture.trace,
}));

vi.mock("@/features/timeline/ItineraryTimeline", () => ({
  ItineraryTimeline: ({
    selectedBlockID,
    onSelectBlock,
  }: {
    selectedBlockID?: string | null;
    onSelectBlock?: (id: string | null) => void;
  }) => (
    <div>
      <span>timeline:{selectedBlockID ?? "none"}</span>
      <button type="button" onClick={() => onSelectBlock?.("b-2")}>
        选择时间块
      </button>
    </div>
  ),
}));

vi.mock("@/features/versions/VersionPanel", () => ({
  VersionPanel: () => <div>version-panel</div>,
}));

vi.mock("@/features/map/TripMap", () => ({
  TripMap: ({
    selectedBlockID,
    onSelectBlock,
  }: {
    selectedBlockID?: string | null;
    onSelectBlock?: (id: string) => void;
  }) => (
    <div>
      <span>map:{selectedBlockID ?? "none"}</span>
      <button type="button" onClick={() => onSelectBlock?.("b-1")}>
        点击地图标记
      </button>
    </div>
  ),
}));

function renderWorkspace() {
  return render(
    <MemoryRouter initialEntries={["/trips/trip-1"]}>
      <Routes>
        <Route path="/trips/:tripId" element={<WorkspacePage />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("WorkspacePage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("links the timeline and the map through one selection", async () => {
    renderWorkspace();

    expect(screen.getByText("timeline:none")).toBeInTheDocument();
    expect(screen.getByText("map:none")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "选择时间块" }));
    expect(screen.getByText("timeline:b-2")).toBeInTheDocument();
    expect(screen.getByText("map:b-2")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "点击地图标记" }));
    expect(screen.getByText("timeline:b-1")).toBeInTheDocument();
    expect(screen.getByText("map:b-1")).toBeInTheDocument();
  });

  it("shows budget numbers that match the plan", () => {
    renderWorkspace();

    expect(screen.getByText("总计").nextSibling).toHaveTextContent("¥100");
    expect(screen.getByText("门票").nextSibling).toHaveTextContent("¥100");
  });
});
