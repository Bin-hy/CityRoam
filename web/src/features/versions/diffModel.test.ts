import { describe, expect, it } from "vitest";
import type { Block, DayPlan, Itinerary } from "@/features/itinerary/types";
import { diffItineraries } from "./diffModel";

function block(overrides: Partial<Block> = {}): Block {
  return {
    id: "b1",
    type: "poi",
    title: "故宫",
    start: "09:00",
    end: "11:00",
    duration_min: 120,
    cost_yuan: 30,
    ...overrides,
  };
}

function day(blocks: Block[], overrides: Partial<DayPlan> = {}): DayPlan {
  return {
    date: "2026-11-20",
    start_time: "09:00",
    end_time: "18:00",
    day_cost_yuan: blocks.reduce((sum, item) => sum + item.cost_yuan, 0),
    blocks,
    ...overrides,
  };
}

function itinerary(days: DayPlan[]): Itinerary {
  return { city: "beijing", days, totals: { cost_yuan: 0, poi_count: 0, walking_km: 0 } };
}

describe("diffItineraries", () => {
  it("returns nothing for identical plans", () => {
    const plan = itinerary([day([block()])]);
    expect(diffItineraries(plan, plan)).toEqual([]);
  });

  it("groups modified days and blocks with field changes", () => {
    const before = itinerary([day([block(), block({ id: "b2", title: "景山", cost_yuan: 2 })])]);
    const after = itinerary([
      day([block({ title: "故宫博物院", cost_yuan: 60, end: "11:30", duration_min: 150 }), block({ id: "b3", title: "北海公园", cost_yuan: 10 })], {
        end_time: "19:00",
      }),
    ]);

    const days = diffItineraries(before, after);
    expect(days).toHaveLength(1);
    expect(days[0].change).toBe("modified");
    expect(days[0].fields.map((field) => field.field)).toContain("end_time");
    expect(days[0].fields.find((field) => field.field === "day_cost_yuan")).toMatchObject({
      before: 32,
      after: 70,
    });

    const [changed, added, removed] = days[0].blocks;
    expect(changed.change).toBe("modified");
    expect(changed.fields).toEqual(
      expect.arrayContaining([
        { field: "title", before: "故宫", after: "故宫博物院" },
        { field: "cost_yuan", before: 30, after: 60 },
      ]),
    );
    expect(added).toMatchObject({ blockID: "b3", change: "added" });
    expect(removed).toMatchObject({ blockID: "b2", change: "removed" });
  });

  it("marks added and removed days with their blocks", () => {
    const before = itinerary([day([block()]), day([block({ id: "b9" })], { date: "2026-11-21" })]);
    const after = itinerary([day([block()]), day([block({ id: "b4", title: "天坛" })], { date: "2026-11-22" })]);

    const days = diffItineraries(before, after);
    expect(days).toHaveLength(1);
    expect(days[0]).toMatchObject({ dayIndex: 1, change: "modified", date: "2026-11-22" });
    expect(days[0].blocks.map((item) => [item.blockID, item.change])).toEqual([
      ["b4", "added"],
      ["b9", "removed"],
    ]);

    const onlyAfter = diffItineraries(itinerary([]), itinerary([day([block()])]));
    expect(onlyAfter[0]).toMatchObject({ change: "added" });
    expect(onlyAfter[0].blocks[0]).toMatchObject({ blockID: "b1", change: "added" });

    const onlyBefore = diffItineraries(itinerary([day([block()])]), itinerary([]));
    expect(onlyBefore[0]).toMatchObject({ change: "removed" });
    expect(onlyBefore[0].blocks[0]).toMatchObject({ blockID: "b1", change: "removed" });
  });
});
