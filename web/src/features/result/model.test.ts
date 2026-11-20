import { describe, expect, it } from "vitest";
import type { Itinerary } from "@/features/itinerary/types";
import type { ResearchBrief } from "@/features/research/types";
import { buildResultModel, resolveResultSelection } from "./model";

const itinerary: Itinerary = {
  city: "beijing",
  days: [
    {
      date: "2026-11-20",
      start_time: "09:00",
      end_time: "18:00",
      day_cost_yuan: 154,
      blocks: [
        {
          id: "b-1",
          type: "poi",
          poi_id: "p1",
          lat: 39.9163,
          lng: 116.3972,
          title: "故宫",
          start: "09:00",
          end: "11:00",
          duration_min: 120,
          cost_yuan: 60,
        },
        {
          id: "b-2",
          type: "transit",
          title: "前往天坛",
          start: "11:00",
          end: "11:30",
          duration_min: 30,
          cost_yuan: 4,
          transport: {
            mode: "transit",
            from_poi_id: "p1",
            to_poi_id: "p2",
            distance_m: 4200,
            duration_min: 30,
            cost_yuan: 4,
            estimated: false,
            legs: [
              {
                mode: "walk",
                distance_m: 300,
                duration_min: 5,
                cost_yuan: 0,
                polyline: [
                  [116.3972, 39.9163],
                  [116.399, 39.912],
                ],
              },
              {
                mode: "subway",
                line_name: "地铁1号线",
                from_stop: "天安门东",
                to_stop: "天坛东门",
                distance_m: 3900,
                duration_min: 25,
                cost_yuan: 4,
                polyline: [
                  [116.399, 39.912],
                  [116.4066, 39.8822],
                ],
              },
            ],
          },
        },
        {
          id: "b-3",
          type: "poi",
          poi_id: "p2",
          lat: 39.8822,
          lng: 116.4066,
          title: "天坛",
          start: "11:30",
          end: "13:00",
          duration_min: 90,
          cost_yuan: 0,
        },
        {
          id: "b-4",
          type: "meal",
          poi_id: "m1",
          lat: 39.884,
          lng: 116.41,
          title: "午饭",
          start: "13:00",
          end: "14:00",
          duration_min: 60,
          cost_yuan: 80,
        },
        {
          id: "b-5",
          type: "poi",
          poi_id: "p3",
          lat: 39.923,
          lng: 116.396,
          title: "景山",
          start: "15:00",
          end: "17:00",
          duration_min: 120,
          cost_yuan: 10,
        },
      ],
    },
    {
      date: "2026-11-21",
      start_time: "09:00",
      end_time: "17:00",
      day_cost_yuan: 44,
      blocks: [
        {
          id: "b-6",
          type: "poi",
          poi_id: "p4",
          lat: 39.924,
          lng: 116.388,
          title: "北海",
          start: "09:00",
          end: "11:00",
          duration_min: 120,
          cost_yuan: 40,
          transport: {
            mode: "transit",
            from_poi_id: "p4",
            to_poi_id: "p5",
            distance_m: 2000,
            duration_min: 30,
            cost_yuan: 4,
            estimated: true,
          },
        },
        {
          id: "b-7",
          type: "poi",
          poi_id: "p5",
          lat: 39.94,
          lng: 116.386,
          title: "什刹海",
          start: "11:30",
          end: "13:30",
          duration_min: 120,
          cost_yuan: 0,
        },
      ],
    },
  ],
  lodging: {
    hotel: {
      name: "全季酒店",
      address: "东城区",
      lat: 39.93,
      lng: 116.42,
      star: 4,
      score: 8.6,
      price_per_night_yuan: 480,
      price_estimated: true,
      booking_url: "https://example.com/hotel",
      provider: "amap",
    },
    nights: 2,
    nightly_yuan: 480,
    total_yuan: 960,
    anchor_days: [0],
    include_in_budget: true,
    price_estimated: true,
    source: "user",
  },
  explanations: [{ poi_id: "p1", text: "中轴线起点。" }],
  totals: { cost_yuan: 198, poi_count: 5, walking_km: 5.2, lodging_yuan: 960 },
};

const research: ResearchBrief = {
  status: "ok",
  guides: [
    {
      poi_id: "p1",
      weight: 1,
      mentions: 2,
      note: "强烈推荐。",
      citations: [
        {
          title: "故宫攻略",
          url: "https://example.com/gugong",
          site: "马蜂窝",
          published_at: "2026-01-01",
        },
        { title: "坏链接", url: "javascript:alert(1)", site: "unknown" },
        { url: "https://example.com/no-title" },
        { title: "", url: "" },
      ],
    },
  ],
  notes: ["提前预约故宫", "提前预约故宫"],
  warnings: ["周一闭馆"],
};

describe("buildResultModel", () => {
  it("folds transit blocks into legs and keeps stops in order", () => {
    const model = buildResultModel({ itinerary, research });
    expect(model).not.toBeNull();
    const day = model!.days[0];

    expect(day.stops.map((stop) => stop.title)).toEqual(["故宫", "天坛", "午饭", "景山"]);
    expect(day.stops.map((stop) => stop.order)).toEqual([0, 1, 2, 3]);
    expect(day.legs).toHaveLength(1);

    const leg = day.legs[0];
    expect(leg.id).toBe("0:b-1>b-3");
    expect(leg.geometry).toBe("polyline");
    expect(leg.durationMin).toBe(30);
    expect(leg.costYuan).toBe(4);
    expect(leg.segments.map((segment) => segment.mode)).toEqual(["walk", "subway"]);
    expect(leg.segments[1].lineName).toBe("地铁1号线");
    expect(leg.segments[1].fromStop).toBe("天安门东");
    expect(leg.segments[1].toStop).toBe("天坛东门");
    expect(leg.segments[1].polyline).toHaveLength(2);
  });

  it("uses a block-level transport and marks straight geometry", () => {
    const model = buildResultModel({ itinerary, research });
    const day = model!.days[1];
    const leg = day.legs[0];

    expect(leg.id).toBe("1:b-6>b-7");
    expect(leg.geometry).toBe("straight");
    expect(leg.estimated).toBe(true);
    expect(leg.segments).toHaveLength(1);
    expect(leg.segments[0].polyline).toEqual([]);
  });

  it("attaches reasons and only http(s) citations", () => {
    const model = buildResultModel({ itinerary, research });
    const [gugong, tiantan] = model!.days[0].stops;

    expect(gugong.reason).toBe("中轴线起点。");
    expect(tiantan.reason).toBeNull();
    expect(gugong.guides).toHaveLength(1);
    expect(gugong.guides[0].note).toBe("强烈推荐。");
    expect(gugong.guides[0].citations).toHaveLength(3);
    expect(gugong.guides[0].citations[0].url).toBe("https://example.com/gugong");
    expect(gugong.guides[0].citations[1].title).toBe("坏链接");
    expect(gugong.guides[0].citations[1].url).toBeUndefined();
    expect(gugong.guides[0].citations.filter((citation) => citation.url).every((citation) => citation.url?.startsWith("https://"))).toBe(true);
    expect(tiantan.guides).toEqual([]);
  });

  it("builds overview, theme, summary and budget from existing data", () => {
    const model = buildResultModel({ itinerary, research });

    expect(model!.title).toBe("北京 · 2 天");
    expect(model!.summary).toBe("2 天串起故宫、天坛、景山与北海");
    expect(model!.days[0].theme).toBe("故宫 · 天坛 · 景山");
    expect(model!.overview).toEqual({ days: 2, poiCount: 5, walkingKM: 5.2, pace: "relaxed" });
    expect(model!.budget).toMatchObject({
      tickets: 110,
      meals: 80,
      transit: 4,
      lodging: 960,
      total: 1154,
      lodgingIncluded: true,
      estimated: true,
    });
    expect(model!.lodging?.hotel.bookingURL).toBe("https://example.com/hotel");
  });

  it("creates gaps of at least 45 minutes with suggestions", () => {
    const model = buildResultModel({ itinerary, research });
    const gaps = model!.days[0].gaps;

    expect(gaps.map((gap) => [gap.start, gap.end, gap.minutes, gap.suggestion])).toEqual([
      ["14:00", "15:00", 60, "休息"],
      ["17:00", "18:00", 60, "晚饭"],
    ]);
    expect(model!.days[1].gaps).toEqual([
      {
        id: "1:gap:13:30",
        dayIndex: 1,
        start: "13:30",
        end: "17:00",
        minutes: 210,
        suggestion: "自由活动",
      },
    ]);
  });

  it("deduplicates reminders and omits an empty block", () => {
    const model = buildResultModel({ itinerary, research });
    expect(model!.reminders.map((reminder) => reminder.text)).toEqual(["提前预约故宫", "周一闭馆"]);

    const quiet = buildResultModel({ itinerary, research: { status: "ok", guides: [] } });
    expect(quiet!.reminders).toEqual([]);
  });

  it("drops unsafe booking URLs", () => {
    const unsafe: Itinerary = {
      ...itinerary,
      lodging: {
        ...itinerary.lodging!,
        hotel: { ...itinerary.lodging!.hotel, booking_url: "javascript:alert(1)" },
      },
    };
    const model = buildResultModel({ itinerary: unsafe });
    expect(model!.lodging?.hotel.bookingURL).toBeUndefined();
  });

  it("returns null without a plan", () => {
    expect(buildResultModel({ itinerary: null })).toBeNull();
  });

  it("degrades missing times, coordinates, lodging and research", () => {
    const sparse = {
      ...itinerary,
      lodging: undefined,
      explanations: [],
      totals: { cost_yuan: 0, poi_count: 0, walking_km: 0 },
      days: [
        {
          ...itinerary.days[0],
          blocks: [
            {
              ...itinerary.days[0].blocks[0],
              lat: undefined,
              lng: undefined,
              start: "",
              end: "",
              transport: {
                mode: "walk",
                from_poi_id: "p1",
                to_poi_id: "p2",
                distance_m: 500,
                duration_min: 8,
                cost_yuan: 0,
                estimated: true,
              },
            },
            itinerary.days[0].blocks[2],
          ],
        },
      ],
    } as Itinerary;

    const model = buildResultModel({ itinerary: sparse });
    expect(model).not.toBeNull();
    expect(model!.days[0].gaps).toEqual([]);
    expect(model!.days[0].legs[0].geometry).toBe("none");
    expect(model!.days[0].stops[0].reason).toBeNull();
    expect(model!.days[0].stops[0].guides).toEqual([]);
    expect(model!.lodging).toBeNull();
    expect(model!.reminders).toEqual([]);
    expect(model!.budget.lodging).toBe(0);
  });
});

describe("resolveResultSelection", () => {
  const model = buildResultModel({ itinerary, research })!;

  it("resolves stops, legs and the hotel", () => {
    const stop = resolveResultSelection(model, { kind: "stop", dayIndex: 0, stopID: "b-3" });
    expect(stop?.kind).toBe("stop");
    expect(stop?.kind === "stop" && stop.stop.title).toBe("天坛");

    const leg = resolveResultSelection(model, { kind: "leg", dayIndex: 1, legID: "1:b-6>b-7" });
    expect(leg?.kind).toBe("leg");
    expect(leg?.kind === "leg" && leg.leg.to.title).toBe("什刹海");

    const hotel = resolveResultSelection(model, { kind: "hotel" });
    expect(hotel?.kind).toBe("hotel");
    expect(hotel?.kind === "hotel" && hotel.lodging.hotel.name).toBe("全季酒店");
  });

  it("returns null for empty or stale selections", () => {
    expect(resolveResultSelection(model, null)).toBeNull();
    expect(resolveResultSelection(model, { kind: "stop", dayIndex: 9, stopID: "b-1" })).toBeNull();
    expect(resolveResultSelection(model, { kind: "stop", dayIndex: 0, stopID: "missing" })).toBeNull();
    expect(resolveResultSelection(model, { kind: "leg", dayIndex: 0, legID: "missing" })).toBeNull();
  });
});