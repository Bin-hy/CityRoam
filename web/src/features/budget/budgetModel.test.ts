import { describe, expect, it } from "vitest";
import type { Itinerary } from "@/features/itinerary/types";
import { budgetBreakdown, budgetSummary } from "./budgetModel";

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
        {
          id: "b-2",
          type: "transit",
          title: "前往天坛",
          start: "11:00",
          end: "11:20",
          duration_min: 20,
          cost_yuan: 4,
          transport: {
            mode: "transit",
            from_poi_id: "p1",
            to_poi_id: "p2",
            distance_m: 3000,
            duration_min: 20,
            cost_yuan: 4,
            estimated: true,
          },
        },
        { id: "b-3", type: "meal", title: "午餐", start: "12:00", end: "13:00", duration_min: 60, cost_yuan: 40 },
      ],
    },
  ],
  explanations: [],
  totals: { cost_yuan: 104, poi_count: 1, walking_km: 1.2 },
};

describe("budgetBreakdown", () => {
  it("splits tickets, meals and transit", () => {
    expect(budgetBreakdown(itinerary)).toMatchObject({
      tickets: 60,
      meals: 40,
      transit: 4,
      lodging: 0,
      total: 104,
      poiCount: 1,
      estimated: true,
      hasLodging: false,
    });
  });

  it("adds the lodging total and flags it", () => {
    const withLodging = { ...itinerary, totals: { ...itinerary.totals, lodging_yuan: 960 } };
    expect(budgetBreakdown(withLodging)).toMatchObject({ lodging: 960, total: 1064, hasLodging: true });
  });
});

describe("budgetSummary", () => {
  it("returns the total with lodging state", () => {
    expect(budgetSummary(itinerary)).toEqual({ total: 104, hasLodging: false });
    expect(budgetSummary(null)).toEqual({ total: 0, hasLodging: false });
  });
});
