import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { ResultDay, ResultLeg, ResultLegSegment } from "./model";
import { LegDetail } from "./LegDetail";
import { LegRow } from "./LegRow";

const day: ResultDay = {
  index: 0,
  date: "2026-11-20",
  label: "11 月 20 日 周五",
  theme: "故宫 · 天坛",
  start: "09:00",
  end: "18:00",
  costYuan: 64,
  stops: [],
  legs: [],
  gaps: [],
};

function segment(overrides: Partial<ResultLegSegment> = {}): ResultLegSegment {
  return {
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
      [116.399, 39.912],
      [116.4066, 39.8822],
    ],
    ...overrides,
  };
}

function leg(overrides: Partial<ResultLeg> = {}): ResultLeg {
  return {
    id: "0:b-1>b-2",
    dayIndex: 0,
    from: { id: "b-1", title: "故宫" },
    to: { id: "b-2", title: "天坛" },
    mode: "transit",
    modeLabel: "公共交通",
    durationMin: 25,
    distanceM: 4200,
    costYuan: 4,
    estimated: false,
    segments: [
      segment({ mode: "walk", modeLabel: "步行", lineName: undefined, fromStop: undefined, toStop: undefined, distanceM: 300, durationMin: 4, costYuan: 0 }),
      segment({ mode: "transit", modeLabel: "公共交通" }),
    ],
    geometry: "polyline",
    ...overrides,
  };
}

describe("LegRow", () => {
  it("summarizes the leg and exposes the selected state", async () => {
    const onSelect = vi.fn();
    render(<LegRow leg={leg({ estimated: true })} selected onSelect={onSelect} />);

    const row = screen.getByRole("button", { name: /公共交通.*地铁1号线.*25 分钟/ });
    expect(row).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByText("估算")).toBeInTheDocument();

    await userEvent.click(row);
    expect(onSelect).toHaveBeenCalled();
  });
});

describe("LegDetail", () => {
  it("renders every segment with line, stops, distance and cost", () => {
    render(<LegDetail day={day} leg={leg()} />);

    expect(screen.getByText("天安门东 → 天坛东门")).toBeInTheDocument();
    expect(screen.getByText(/3\.9 km · 21 分钟 · ¥4/)).toBeInTheDocument();
    expect(screen.getByText("按实际路线绘制")).toBeInTheDocument();
    expect(screen.queryByText(/整段为距离估算/)).not.toBeInTheDocument();
  });

  it("marks an estimated leg and a straight-line geometry", () => {
    render(
      <LegDetail
        day={day}
        leg={leg({
          estimated: true,
          geometry: "straight",
          segments: [segment({ estimated: true, polyline: [] })],
        })}
      />,
    );

    expect(screen.getByText(/整段为距离估算/)).toBeInTheDocument();
    expect(screen.getByText("直线估算，仅供距离参考")).toBeInTheDocument();
    expect(screen.getAllByText("估算").length).toBeGreaterThan(0);
  });

  it("explains that missing coordinates only allow text", () => {
    render(<LegDetail day={day} leg={leg({ geometry: "none" })} />);

    expect(screen.getByText("缺少坐标，仅显示文字")).toBeInTheDocument();
  });

  it("only shows the map action when a callback is provided", async () => {
    const onViewOnMap = vi.fn();
    const view = render(<LegDetail day={{ ...day, index: 1 }} leg={leg({ dayIndex: 1 })} onViewOnMap={onViewOnMap} />);

    await userEvent.click(screen.getByRole("button", { name: "查看路线图" }));
    expect(onViewOnMap).toHaveBeenCalledWith(1);

    view.unmount();
    render(<LegDetail day={day} leg={leg()} />);
    expect(screen.queryByRole("button", { name: "查看路线图" })).not.toBeInTheDocument();
  });
});