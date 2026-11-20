import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { ResultDay, ResultHotel, ResultLeg, ResultSelection, ResultStop } from "./model";
import { DayMiniMap } from "./DayMiniMap";

const stop = (overrides: Partial<ResultStop> = {}): ResultStop => ({
  id: "b-1",
  dayIndex: 0,
  order: 0,
  type: "poi",
  typeLabel: "景点",
  title: "故宫",
  start: "09:00",
  end: "11:00",
  durationMin: 120,
  costYuan: 60,
  lat: 39.9163,
  lng: 116.3972,
  reason: null,
  guides: [],
  ...overrides,
});

const leg: ResultLeg = {
  id: "0:b-1>b-2",
  dayIndex: 0,
  from: { id: "b-1", title: "故宫", lat: 39.9163, lng: 116.3972 },
  to: { id: "b-2", title: "天坛", lat: 39.8822, lng: 116.4066 },
  mode: "transit",
  modeLabel: "公共交通",
  durationMin: 25,
  distanceM: 4200,
  costYuan: 4,
  estimated: false,
  segments: [
    {
      mode: "subway",
      modeLabel: "地铁",
      lineName: "地铁1号线",
      fromStop: "天安门东",
      toStop: "天坛东门",
      distanceM: 3900,
      durationMin: 21,
      costYuan: 4,
      estimated: false,
      polyline: [
        [116.3972, 39.9163],
        [116.399, 39.912],
        [116.4066, 39.8822],
      ],
    },
  ],
  geometry: "polyline",
};

const day: ResultDay = {
  index: 0,
  date: "2026-11-20",
  label: "11 月 20 日 周五",
  theme: "故宫 · 天坛",
  start: "09:00",
  end: "18:00",
  costYuan: 64,
  stops: [stop(), stop({ id: "b-2", order: 1, title: "天坛", lat: 39.8822, lng: 116.4066 })],
  legs: [leg],
  gaps: [],
};

const hotel: ResultHotel = {
  name: "全季酒店",
  lat: 39.93,
  lng: 116.42,
  star: 4,
};

function renderMap(overrides: {
  day?: ResultDay;
  hotel?: ResultHotel | null;
  selection?: ResultSelection;
  onSelect?: (selection: ResultSelection) => void;
  onViewOnMap?: (dayIndex: number) => void;
  mapAvailable?: boolean;
} = {}) {
  return render(
    <DayMiniMap
      day={overrides.day ?? day}
      hotel={overrides.hotel ?? null}
      selection={overrides.selection ?? null}
      onSelect={overrides.onSelect ?? vi.fn()}
      onViewOnMap={overrides.onViewOnMap ?? vi.fn()}
      mapAvailable={overrides.mapAvailable}
    />,
  );
}

describe("DayMiniMap", () => {
  it("projects coordinates with latitude order preserved and draws stops and routes", () => {
    renderMap({ hotel });

    const first = screen.getByRole("button", { name: "停靠点：故宫" });
    const second = screen.getByRole("button", { name: "停靠点：天坛" });
    const firstCircle = first.querySelector("circle");
    const secondCircle = second.querySelector("circle");

    expect(Number(firstCircle?.getAttribute("cx"))).toBeLessThan(Number(secondCircle?.getAttribute("cx")));
    expect(Number(firstCircle?.getAttribute("cy"))).toBeLessThan(Number(secondCircle?.getAttribute("cy")));
    expect(screen.getByRole("img", { name: "住宿：全季酒店" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "路段：故宫 → 天坛" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "查看路线图" })).toBeInTheDocument();
  });

  it("forwards stop and leg selections", async () => {
    const onSelect = vi.fn();
    renderMap({ onSelect });

    await userEvent.click(screen.getByRole("button", { name: "停靠点：天坛" }));
    expect(onSelect).toHaveBeenCalledWith({ kind: "stop", dayIndex: 0, stopID: "b-2" });

    await userEvent.click(screen.getByRole("button", { name: "路段：故宫 → 天坛" }));
    expect(onSelect).toHaveBeenCalledWith({ kind: "leg", dayIndex: 0, legID: "0:b-1>b-2" });
  });

  it("thickens the selected leg and mutes the rest", () => {
    const { container } = renderMap({ selection: { kind: "leg", dayIndex: 0, legID: "0:b-1>b-2" } });

    const visible = container.querySelectorAll("polyline")[1];
    expect(visible.getAttribute("stroke-width")).toBe("6");
    expect(visible.getAttribute("stroke-opacity")).toBe("1");
  });

  it("mutes legs when a stop is selected", () => {
    const { container } = renderMap({ selection: { kind: "stop", dayIndex: 0, stopID: "b-1" } });

    const visible = container.querySelectorAll("polyline")[1];
    expect(visible.getAttribute("stroke-opacity")).toBe("0.25");
    expect(screen.getByRole("button", { name: "停靠点：故宫" })).toHaveAttribute("aria-pressed", "true");
  });

  it("draws straight-line estimates dashed", () => {
    const straightDay: ResultDay = {
      ...day,
      legs: [{ ...leg, geometry: "straight", mode: "drive", segments: [] }],
    };
    const { container } = renderMap({ day: straightDay });

    const visible = container.querySelectorAll("polyline")[1];
    expect(visible.getAttribute("stroke-dasharray")).toBe("6 4");
  });

  it("falls back to a text list when nothing can be projected", async () => {
    const onSelect = vi.fn();
    const blind: ResultDay = {
      ...day,
      stops: [
        stop({ lat: undefined, lng: undefined, title: "故宫" }),
        stop({ id: "b-2", order: 1, title: "天坛", lat: 0, lng: 0 }),
      ],
    };

    const { container } = renderMap({ day: blind, onSelect });

    expect(screen.getByText("缺少坐标，无法绘制当天路线")).toBeInTheDocument();
    expect(container.querySelector("svg")).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "2. 天坛" }));
    expect(onSelect).toHaveBeenCalledWith({ kind: "stop", dayIndex: 0, stopID: "b-2" });
  });

  it("falls back to a clickable text list when the map is unavailable", async () => {
    const onSelect = vi.fn();
    const { container } = renderMap({ mapAvailable: false, onSelect });

    expect(screen.getByText("地图暂不可用，已切换为文字清单")).toBeInTheDocument();
    expect(container.querySelector("svg")).toBeNull();
    expect(screen.queryByRole("button", { name: "查看路线图" })).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "1. 故宫" }));
    expect(onSelect).toHaveBeenCalledWith({ kind: "stop", dayIndex: 0, stopID: "b-1" });
  });

  it("requests the full map for this day", async () => {
    const onViewOnMap = vi.fn();
    renderMap({ onViewOnMap });

    await userEvent.click(screen.getByRole("button", { name: "查看路线图" }));
    expect(onViewOnMap).toHaveBeenCalledWith(0);
  });
});