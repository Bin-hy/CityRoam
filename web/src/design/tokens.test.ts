import { describe, expect, it } from "vitest";
import { dayColor, dayThemes, formatDateLabel, formatVersion, formatYuan } from "./tokens";

describe("design tokens", () => {
  it("wraps day colors in both directions", () => {
    expect(dayColor(0)).toBe(dayThemes[0].color);
    expect(dayColor(4)).toBe(dayThemes[0].color);
    expect(dayColor(-1)).toBe(dayThemes[3].color);
    expect(dayColor(Number.NaN)).toBe(dayThemes[0].color);
  });

  it("formats yuan with grouping and no decimals", () => {
    expect(formatYuan(1280.4)).toBe("¥1,280");
    expect(formatYuan(Number.NaN)).toBe("¥0");
  });

  it("formats dates with the weekday", () => {
    expect(formatDateLabel("2026-11-21", 0)).toBe("11 月 21 日 周六");
    expect(formatDateLabel("", 2)).toBe("第 3 天");
    expect(formatDateLabel("not-a-date", 0)).toBe("not-a-date");
  });

  it("formats version labels", () => {
    expect(formatVersion(3)).toBe("v3");
    expect(formatVersion(2.9)).toBe("v2");
  });
});
