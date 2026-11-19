import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { PlanVersion } from "@/api/types";
import type { Itinerary } from "@/features/itinerary/types";
import * as api from "@/api/trips";
import { VersionPanel } from "./VersionPanel";

vi.mock("@/api/trips", () => ({
  listPlanVersions: vi.fn(),
  getPlanVersion: vi.fn(),
  restorePlanVersion: vi.fn(),
}));

const listPlanVersions = vi.mocked(api.listPlanVersions);
const getPlanVersion = vi.mocked(api.getPlanVersion);
const restorePlanVersion = vi.mocked(api.restorePlanVersion);

function plan(cost: number): Itinerary {
  return {
    city: "beijing",
    days: [
      {
        date: "2026-11-20",
        start_time: "09:00",
        end_time: "18:00",
        day_cost_yuan: cost,
        blocks: [
          { id: "b1", type: "poi", title: "故宫", start: "09:00", end: "11:00", duration_min: 120, cost_yuan: cost },
        ],
      },
    ],
    totals: { cost_yuan: cost, poi_count: 1, walking_km: 1 },
  };
}

function version(seq: number, status: PlanVersion["status"], cost: number): PlanVersion {
  return {
    id: `v${seq}`,
    trip_id: "trip-1",
    run_id: "run-1",
    seq,
    plan: plan(cost) as unknown as PlanVersion["plan"],
    constraints: {},
    violations: [],
    status,
    created_at: "2026-11-19T10:00:00Z",
  };
}

const current = version(2, "accepted", 50);
const previous = version(1, "superseded", 30);

function renderPanel(onRestored = vi.fn()) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <VersionPanel tripID="trip-1" currentVersion={current} onRestored={onRestored} />
    </QueryClientProvider>,
  );
  return onRestored;
}

describe("VersionPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    listPlanVersions.mockResolvedValue({ versions: [current, previous] });
    getPlanVersion.mockResolvedValue(previous);
    restorePlanVersion.mockResolvedValue(version(3, "accepted", 30));
  });

  it("loads the selected version and renders its diff", async () => {
    renderPanel();
    const select = await screen.findByLabelText("版本");
    expect(screen.getByText("当前展示 v2，选择其他版本可查看差异。")).toBeInTheDocument();

    await userEvent.selectOptions(select, "1");

    expect(await screen.findByText("对比 v2 → v1，共 1 天有变化。")).toBeInTheDocument();
    expect(getPlanVersion).toHaveBeenCalledWith("trip-1", 1);
    expect(screen.getByRole("region", { name: "第 1 天" })).toHaveTextContent("花费(¥)：¥50 → ¥30");
  });

  it("restores the selected version after confirmation", async () => {
    const onRestored = renderPanel();
    const select = await screen.findByLabelText("版本");
    await userEvent.selectOptions(select, "1");
    await screen.findByText("对比 v2 → v1，共 1 天有变化。");

    await userEvent.click(screen.getByRole("button", { name: "回退到此版本" }));
    await userEvent.click(screen.getByRole("button", { name: "确认回退" }));

    await waitFor(() => expect(restorePlanVersion).toHaveBeenCalledWith("trip-1", 1));
    await waitFor(() => expect(onRestored).toHaveBeenCalledWith(expect.objectContaining({ seq: 3 })));
  });
});
