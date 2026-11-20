import { describe, expect, it } from "vitest";
import { explanationMap, parseItinerary } from "./types";

function baseItinerary(extra: Record<string, unknown> = {}) {
  return {
    city: "beijing",
    days: [
      {
        date: "2026-11-20",
        start_time: "09:00",
        end_time: "18:00",
        blocks: [
          {
            id: "b1",
            type: "transit",
            title: "前往故宫",
            start: "09:00",
            end: "09:20",
            duration_min: 20,
            cost_yuan: 2,
            transport: {
              mode: "transit",
              from_poi_id: "hotel",
              to_poi_id: "poi-0",
              distance_m: 2400,
              duration_min: 20,
              cost_yuan: 2,
              estimated: false,
              legs: [
                { mode: "walk", distance_m: 300, duration_min: 4, cost_yuan: 0, polyline: [[116.4, 39.9], [116.41, 39.91]] },
                {
                  mode: "transit",
                  line_name: "地铁 1 号线",
                  from_stop: "国贸",
                  to_stop: "建国门",
                  distance_m: 2100,
                  duration_min: 12,
                  cost_yuan: 2,
                  polyline: [[116.41, 39.91], [116.42, 39.92]],
                },
              ],
            },
          },
        ],
        day_cost_yuan: 2,
      },
    ],
    lodging: {
      hotel: { id: "h1", name: "全季酒店", lat: 39.9, lng: 116.4, price_per_night_yuan: 400, provider: "flyai" },
      nights: 1,
      nightly_yuan: 400,
      total_yuan: 400,
      anchor_days: [0, 1.5, "2"],
      include_in_budget: true,
      price_estimated: false,
      source: "flyai",
    },
    totals: { cost_yuan: 2, poi_count: 1, walking_km: 1.2 },
    ...extra,
  };
}

describe("parseItinerary geometry and lodging", () => {
  it("parses legs and lodging", () => {
    const itinerary = parseItinerary(baseItinerary());
    expect(itinerary).not.toBeNull();
    const transport = itinerary?.days[0].blocks[0].transport;
    expect(transport?.legs).toHaveLength(2);
    expect(transport?.legs?.[1].line_name).toBe("地铁 1 号线");
    expect(transport?.legs?.[0].polyline).toEqual([[116.4, 39.9], [116.41, 39.91]]);
    expect(itinerary?.lodging?.hotel.name).toBe("全季酒店");
    expect(itinerary?.lodging?.anchor_days).toEqual([0]);
  });

  it("drops malformed polyline points and non-array legs", () => {
    const payload = baseItinerary();
    payload.days[0].blocks[0].transport.legs = [
      { mode: "walk", polyline: [["x", 1], [116.4], [116.4, 39.9]] },
      "not-an-object",
    ] as never;
    const itinerary = parseItinerary(payload);
    const legs = itinerary?.days[0].blocks[0].transport?.legs;
    expect(legs).toHaveLength(1);
    expect(legs?.[0].polyline).toEqual([[116.4, 39.9]]);

    const badLegs = baseItinerary();
    (badLegs.days[0].blocks[0].transport as Record<string, unknown>).legs = "nope";
    expect(parseItinerary(badLegs)?.days[0].blocks[0].transport?.legs).toBeUndefined();
  });

  it("keeps old responses without geometry working", () => {
    const payload = baseItinerary();
    delete (payload.days[0].blocks[0].transport as Record<string, unknown>).legs;
    delete (payload as Record<string, unknown>).lodging;
    const itinerary = parseItinerary(payload);
    expect(itinerary?.days[0].blocks).toHaveLength(1);
    expect(itinerary?.days[0].blocks[0].transport?.mode).toBe("transit");
    expect(itinerary?.days[0].blocks[0].transport?.legs).toBeUndefined();
    expect(itinerary?.lodging).toBeUndefined();
  });

  it("rejects non-object lodging", () => {
    const payload = baseItinerary({ lodging: ["nope"] });
    expect(parseItinerary(payload)?.lodging).toBeUndefined();
  });
});

describe("parseItinerary explanations", () => {
  it("defaults to an empty list when missing", () => {
    const itinerary = parseItinerary(baseItinerary());
    expect(itinerary?.explanations).toEqual([]);
    expect(explanationMap(itinerary).size).toBe(0);
  });

  it("keeps valid explanations and lets later duplicates win", () => {
    const payload = baseItinerary({
      explanations: [
        { poi_id: "poi-0", text: "  经典路线，适合上午。 " },
        { poi_id: "poi-1", text: "晚餐推荐本地菜。" },
        { poi_id: "poi-0", text: "改为下午更从容。" },
      ],
    });
    const itinerary = parseItinerary(payload);
    expect(itinerary?.explanations).toEqual([
      { poi_id: "poi-0", text: "改为下午更从容。" },
      { poi_id: "poi-1", text: "晚餐推荐本地菜。" },
    ]);
    expect(explanationMap(itinerary).get("poi-0")).toBe("改为下午更从容。");
  });

  it("drops malformed entries", () => {
    const payload = baseItinerary({
      explanations: [
        null,
        "nope",
        [],
        { poi_id: "", text: "无 id" },
        { poi_id: "poi-2", text: "   " },
        { poi_id: 3, text: "id 非字符串" },
        { poi_id: "poi-3", text: 42 },
        { poi_id: "poi-4", text: "有效说明" },
      ],
    });
    expect(parseItinerary(payload)?.explanations).toEqual([{ poi_id: "poi-4", text: "有效说明" }]);
  });

  it("tolerates a non-array explanations field", () => {
    const payload = baseItinerary({ explanations: { poi_id: "poi-0" } });
    expect(parseItinerary(payload)?.explanations).toEqual([]);
  });

  it("returns an empty map for a null itinerary", () => {
    expect(explanationMap(null).size).toBe(0);
  });
});
