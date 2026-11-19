import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { Itinerary } from "@/features/itinerary/types";
import { BudgetPanel, budgetBreakdown } from "./BudgetPanel";

const itinerary: Itinerary = {
  city: "beijing",
  days: [
    {
      date: "2026-11-20",
      start_time: "09:00",
      end_time: "18:00",
      day_cost_yuan: 104,
      blocks: [
        { id: "b-1", type: "poi", title: "故宫", start: "09:00", end: "11:00", duration_min: 120, cost_yuan: 60 },
        { id: "b-2", type: "transit", title: "前往天坛", start: "11:00", end: "11:20", duration_min: 20, cost_yuan: 4,
          transport: { mode: "transit", from_poi_id: "p1", to_poi_id: "p2", distance_m: 3000, duration_min: 20, cost_yuan: 4, estimated: true } },
        { id: "b-3", type: "meal", title: "午餐", start: "12:00", end: "13:00", duration_min: 60, cost_yuan: 40 },
      ],
    },
  ],
  totals: { cost_yuan: 104, poi_count: 1, walking_km: 1.2 },
};

describe("budgetBreakdown", () => {
  it("agrees with the itinerary totals and splits costs", () => {
    const budget = budgetBreakdown(itinerary);

    expect(budget).toMatchObject({
      tickets: 60,
      meals: 40,
      transit: 4,
      total: 104,
      poiCount: 1,
      estimated: true,
    });
    expect(budget.total).toBe(itinerary.totals.cost_yuan);
  });
});

describe("BudgetPanel", () => {
  it("shows cost rows and the estimate badge", () => {
    render(<BudgetPanel itinerary={itinerary} />);

    expect(screen.getByText("门票").nextSibling).toHaveTextContent("¥60");
    expect(screen.getByText("餐饮").nextSibling).toHaveTextContent("¥40");
    expect(screen.getByText("交通").nextSibling).toHaveTextContent("¥4");
    expect(screen.getByText("总计").nextSibling).toHaveTextContent("¥104");
    expect(screen.getByText("含估算交通")).toBeInTheDocument();
    expect(screen.getByText(/价格为估算/)).toBeInTheDocument();
  });

  it("shows an empty state", () => {
    render(<BudgetPanel itinerary={null} />);
    expect(screen.getByText("暂无预算。")).toBeInTheDocument();
  });
});
