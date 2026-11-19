import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { Itinerary } from "@/features/itinerary/types";
import { PlanDiff } from "./PlanDiff";
import { diffItineraries } from "./diffModel";

const before: Itinerary = {
  city: "beijing",
  days: [
    {
      date: "2026-11-20",
      start_time: "09:00",
      end_time: "18:00",
      day_cost_yuan: 30,
      blocks: [
        { id: "b1", type: "poi", title: "故宫", start: "09:00", end: "11:00", duration_min: 120, cost_yuan: 30 },
      ],
    },
  ],
  totals: { cost_yuan: 30, poi_count: 1, walking_km: 1 },
};

const after: Itinerary = {
  city: "beijing",
  days: [
    {
      date: "2026-11-20",
      start_time: "09:00",
      end_time: "18:00",
      day_cost_yuan: 50,
      blocks: [
        { id: "b1", type: "poi", title: "故宫", start: "09:00", end: "11:30", duration_min: 150, cost_yuan: 50 },
        { id: "b2", type: "meal", title: "午餐", start: "12:00", end: "13:00", duration_min: 60, cost_yuan: 40 },
      ],
    },
  ],
  totals: { cost_yuan: 90, poi_count: 2, walking_km: 1.2 },
};

describe("PlanDiff", () => {
  it("renders day and block changes grouped by day", () => {
    render(<PlanDiff days={diffItineraries(before, after)} from={1} to={2} />);

    expect(screen.getByText("对比 v1 → v2，共 1 天有变化。")).toBeInTheDocument();
    const day = screen.getByRole("region", { name: "第 1 天" });
    expect(day).toHaveTextContent("当天花费(¥)：¥30 → ¥50");
    expect(day).toHaveTextContent("修改");
    expect(day).toHaveTextContent("花费(¥)：¥30 → ¥50");
    expect(day).toHaveTextContent("结束：11:00 → 11:30");
    expect(day).toHaveTextContent("新增");
    expect(day).toHaveTextContent("午餐");
    expect(day).toHaveTextContent("12:00–13:00 · ¥40");
  });

  it("renders an empty state for identical plans", () => {
    render(<PlanDiff days={[]} />);
    expect(screen.getByText("两个版本内容一致。")).toBeInTheDocument();
  });
});
