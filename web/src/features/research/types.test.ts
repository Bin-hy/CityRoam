import { describe, expect, it } from "vitest";
import { parseResearch, parseResearchQuestion } from "./types";

describe("parseResearch", () => {
  it("returns null for empty or malformed research", () => {
    expect(parseResearch(undefined)).toBeNull();
    expect(parseResearch(null)).toBeNull();
    expect(parseResearch({})).toBeNull();
    expect(parseResearch([])).toBeNull();
  });

  it("parses accommodation and drops internal metrics", () => {
    const brief = parseResearch({
      status: "ok",
      tool_calls: 3,
      duration_ms: 1200,
      accommodation: {
        mentioned: true,
        specified: ["全季"],
        selected: { id: "h1", name: "全季", lat: 39.8, lng: 116.3, price_per_night_yuan: 480, score: 4.7 },
        anchor_days: [0, 1],
        nights: 2,
        total_yuan: 960,
        price_estimated: true,
        include_in_budget: true,
      },
      guides: [{ poi_id: "poi-0", weight: 1.2, mentions: 3, citations: [{ title: "攻略", url: "https://example.com/a" }] }],
      warnings: ["价格估算"],
    });

    expect(brief?.status).toBe("ok");
    expect(brief?.accommodation?.selected?.name).toBe("全季");
    expect(brief?.guides?.[0].citations?.[0].url).toBe("https://example.com/a");
    expect(brief?.warnings).toEqual(["价格估算"]);
    expect(brief).not.toHaveProperty("tool_calls");
    expect(brief).not.toHaveProperty("duration_ms");
  });

  it("keeps a degraded status even without details", () => {
    expect(parseResearch({ status: "degraded" })).toEqual({ status: "degraded" });
    expect(parseResearch({ status: "unknown" })).toBeNull();
  });
});

describe("parseResearchQuestion", () => {
  it("parses hotel options and rejects malformed choices", () => {
    const question = parseResearchQuestion({
      code: "hotel_unconfirmed",
      message: "未找到指定酒店",
      options: [
        { id: "hotel-1", label: "全季", hotel: { name: "全季", price_per_night_yuan: 480 } },
        { id: "continue_without_lodging", label: "不带住宿继续" },
        { id: "missing-label" },
      ],
    });

    expect(question?.options).toHaveLength(2);
    expect(question?.options[0].hotel?.name).toBe("全季");
    expect(question?.options[1].label).toBe("不带住宿继续");
    expect(parseResearchQuestion({ code: "x", options: [] })).toBeNull();
    expect(parseResearchQuestion(null)).toBeNull();
  });
});
