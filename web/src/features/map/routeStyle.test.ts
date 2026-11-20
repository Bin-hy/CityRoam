import { describe, expect, it } from "vitest";
import { modeLegend, routeStyle } from "./routeStyle";

describe("routeStyle", () => {
  it("maps walking to a dashed gray line", () => {
    expect(routeStyle("walk", undefined, "default")).toMatchObject({
      color: "#64748b",
      dashed: true,
      weight: 4,
      opacity: 0.9,
    });
  });

  it("maps bus, subway and drive to distinct colors", () => {
    expect(routeStyle("bus", "1路").color).toBe("#2563eb");
    expect(routeStyle("subway", "地铁1号线").color).toBe("#f97316");
    expect(routeStyle("transit", "地铁2号线").color).toBe("#f97316");
    expect(routeStyle("transit", "公交1路").color).toBe("#2563eb");
    expect(routeStyle("drive", undefined).color).toBe("#7c3aed");
  });

  it("falls back to a dashed gray line for unknown modes", () => {
    expect(routeStyle("boat", undefined)).toMatchObject({ color: "#94a3b8", dashed: true });
  });

  it("emphasizes and mutes weighted lines", () => {
    expect(routeStyle("walk", undefined, "selected")).toMatchObject({ weight: 6, opacity: 1 });
    expect(routeStyle("walk", undefined, "muted")).toMatchObject({ weight: 4, opacity: 0.25 });
  });

  it("builds a plain-text title from line and stops", () => {
    expect(
      routeStyle("subway", "地铁1号线", "default", { fromStop: "天安门东", toStop: "天坛东门" }).title,
    ).toBe("地铁1号线 · 天安门东 · 天坛东门");
    expect(routeStyle("walk").title).toBe("");
  });

  it("exposes the four-mode legend", () => {
    expect(modeLegend.map((item) => item.label)).toEqual(["步行", "公交", "地铁", "驾车"]);
  });
});