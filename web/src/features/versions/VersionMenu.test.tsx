import type { ComponentProps } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { PlanVersion } from "@/api/types";
import type { Itinerary } from "@/features/itinerary/types";
import * as api from "@/api/trips";
import { VersionMenu } from "./VersionMenu";

vi.mock("@/api/trips", () => ({
  listPlanVersions: vi.fn(),
  restorePlanVersion: vi.fn(),
}));

const listPlanVersions = vi.mocked(api.listPlanVersions);
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
    explanations: [],
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
    reason: "用户补充需求",
  };
}

const current = version(2, "accepted", 50);
const previous = version(1, "superseded", 30);

function renderMenu(overrides: Partial<ComponentProps<typeof VersionMenu>> = {}) {
  const props = {
    tripID: "trip-1",
    currentSeq: 2,
    plan: plan(50),
    onViewVersion: vi.fn(),
    onRestored: vi.fn(),
    onReturnCurrent: vi.fn(),
    ...overrides,
  };
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <VersionMenu {...props} />
    </QueryClientProvider>,
  );
  return props;
}

describe("VersionMenu", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    listPlanVersions.mockResolvedValue({ versions: [current, previous] });
    restorePlanVersion.mockResolvedValue(version(3, "accepted", 30));
  });

  it("lists versions, previews the diff and restores after confirmation", async () => {
    const props = renderMenu();
    await userEvent.click(screen.getByRole("button", { name: /v2/ }));

    expect(await screen.findByRole("button", { name: /v1/ })).toHaveTextContent("历史");
    expect(screen.getByRole("button", { name: /v1/ })).toHaveTextContent("用户补充需求");

    await userEvent.click(screen.getByRole("button", { name: /v1/ }));
    expect(props.onViewVersion).toHaveBeenCalledWith(previous);
    expect(await screen.findByText("涉及 1 天 · 新增 0 · 调整 1 · 删除 0")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "恢复此版本" }));
    await userEvent.click(screen.getByRole("button", { name: "确认恢复" }));

    await waitFor(() => expect(restorePlanVersion).toHaveBeenCalledWith("trip-1", 1));
    await waitFor(() => expect(props.onRestored).toHaveBeenCalledWith(expect.objectContaining({ seq: 3 })));
    await waitFor(() => expect(screen.queryByText("版本")).not.toBeInTheDocument());
  });

  it("returns to the current version", async () => {
    const props = renderMenu();
    await userEvent.click(screen.getByRole("button", { name: /v2/ }));
    await userEvent.click(await screen.findByRole("button", { name: /v1/ }));
    await userEvent.click(screen.getByRole("button", { name: "返回当前" }));

    expect(props.onReturnCurrent).toHaveBeenCalled();
    expect(screen.queryByText(/涉及 1 天/)).not.toBeInTheDocument();
  });

  it("keeps the menu open and shows restore errors", async () => {
    restorePlanVersion.mockRejectedValueOnce(new Error("恢复失败"));
    renderMenu();
    await userEvent.click(screen.getByRole("button", { name: /v2/ }));
    await userEvent.click(await screen.findByRole("button", { name: /v1/ }));
    await userEvent.click(screen.getByRole("button", { name: "恢复此版本" }));
    await userEvent.click(screen.getByRole("button", { name: "确认恢复" }));

    expect(await screen.findByText("恢复失败")).toBeInTheDocument();
    expect(screen.getByText("版本")).toBeInTheDocument();
  });

  it("renders nothing in read-only mode", () => {
    renderMenu({ readOnly: true });
    expect(screen.queryByRole("button", { name: /v2/ })).not.toBeInTheDocument();
  });
});
