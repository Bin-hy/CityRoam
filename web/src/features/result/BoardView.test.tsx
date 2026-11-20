import { useState } from "react";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ResultDay, ResultModel, ResultSelection } from "./model";
import { BoardView } from "./BoardView";

const day1: ResultDay = {
  index: 0,
  date: "2026-11-20",
  label: "11 月 20 日 周五",
  theme: "故宫 · 天坛",
  start: "09:00",
  end: "18:00",
  costYuan: 100,
  stops: [
    {
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
      reason: "中轴线起点。",
      guides: [],
    },
    {
      id: "b-2",
      dayIndex: 0,
      order: 1,
      type: "poi",
      typeLabel: "景点",
      title: "天坛",
      start: "11:30",
      end: "13:00",
      durationMin: 90,
      costYuan: 40,
      lat: 39.8822,
      lng: 116.4066,
      reason: null,
      guides: [],
    },
  ],
  legs: [
    {
      id: "0:b-1>b-2",
      dayIndex: 0,
      from: { id: "b-1", title: "故宫", lat: 39.9163, lng: 116.3972 },
      to: { id: "b-2", title: "天坛", lat: 39.8822, lng: 116.4066 },
      mode: "walk",
      modeLabel: "步行",
      durationMin: 20,
      distanceM: 1200,
      costYuan: 0,
      estimated: false,
      segments: [
        {
          mode: "walk",
          modeLabel: "步行",
          distanceM: 1200,
          durationMin: 20,
          costYuan: 0,
          estimated: false,
          polyline: [],
        },
      ],
      geometry: "straight",
    },
  ],
  gaps: [
    { id: "0:gap:14:00", dayIndex: 0, start: "14:00", end: "15:00", minutes: 60, suggestion: "自由活动" },
  ],
};

const day2: ResultDay = {
  ...day1,
  index: 1,
  date: "2026-11-21",
  label: "11 月 21 日 周六",
  theme: "北海公园",
  costYuan: 40,
  stops: [
    {
      ...day1.stops[0],
      id: "b-3",
      dayIndex: 1,
      title: "北海公园",
      costYuan: 40,
      reason: null,
    },
  ],
  legs: [],
  gaps: [],
};

const model: ResultModel = {
  city: "北京",
  title: "北京 · 2 天",
  summary: "2 天串起故宫与天坛",
  overview: { days: 2, poiCount: 5, walkingKM: 5.2, pace: "balanced" },
  budget: {
    tickets: 100,
    meals: 80,
    transit: 4,
    lodging: 960,
    total: 1144,
    lodgingIncluded: false,
    estimated: false,
  },
  lodging: {
    hotel: { name: "全季酒店", lat: 39.93, lng: 116.42, star: 4, score: 8.8 },
    nights: 2,
    nightlyYuan: 480,
    totalYuan: 960,
    anchorDays: [0],
    includeInBudget: false,
    priceEstimated: false,
  },
  reminders: [{ id: "research:0", text: "故宫周一闭馆。" }],
  days: [day1, day2],
};

function Harness({
  activeDay = 0,
  readOnly = false,
  initialSelection = null,
}: {
  activeDay?: number | null;
  readOnly?: boolean;
  initialSelection?: ResultSelection;
}) {
  const [selection, setSelection] = useState<ResultSelection>(initialSelection);
  return (
    <BoardView
      model={model}
      activeDay={activeDay}
      selection={selection}
      onSelect={setSelection}
      onAdjustDay={vi.fn()}
      readOnly={readOnly}
    />
  );
}

beforeEach(() => {
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    configurable: true,
    value: (query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
      addListener: () => undefined,
      removeListener: () => undefined,
      dispatchEvent: () => false,
    }),
  });
});

describe("BoardView", () => {
  it("shows the five metrics from the model", () => {
    render(<Harness />);

    const metrics = screen.getByRole("region", { name: "核心指标" });
    expect(within(metrics).getByText("¥1,144")).toBeInTheDocument();
    expect(within(metrics).getByText("¥960")).toBeInTheDocument();
    expect(within(metrics).getByText("住宿（未计入）")).toBeInTheDocument();
    expect(within(metrics).getByText("¥100")).toBeInTheDocument();
    expect(within(metrics).getByText("5.2 km")).toBeInTheDocument();
    expect(within(metrics).getByText("5 个")).toBeInTheDocument();
  });

  it("renders day columns with reasons, gaps and reminders", () => {
    render(<Harness />);

    const columns = screen.getAllByRole("article");
    expect(columns.map((column) => column.getAttribute("aria-label"))).toEqual(["第 1 天", "第 2 天"]);
    expect(columns[0]).toHaveAttribute("data-active", "true");
    expect(columns[1]).toHaveAttribute("data-active", "false");

    expect(screen.getByText("中轴线起点。")).toBeInTheDocument();
    expect(screen.getAllByText("未生成额外说明").length).toBeGreaterThan(0);
    expect(screen.getByText("留白")).toBeInTheDocument();
    expect(screen.getByText("14:00–15:00 · 自由活动")).toBeInTheDocument();
    expect(screen.getByText("故宫周一闭馆。")).toBeInTheDocument();
  });

  it("opens the shared stop detail from a card", async () => {
    render(<Harness />);

    await userEvent.click(screen.getByRole("button", { name: /09:00.*故宫/ }));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByRole("heading", { name: "故宫" })).toBeInTheDocument();
  });

  it("opens the leg detail from a route row", async () => {
    render(<Harness />);

    await userEvent.click(screen.getByRole("button", { name: /步行.*20 分钟/ }));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByText("直线估算，仅供距离参考")).toBeInTheDocument();
  });

  it("opens lodging from the metric", async () => {
    render(<Harness />);

    await userEvent.click(screen.getByRole("button", { name: "查看住宿详情" }));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByRole("heading", { name: "全季酒店" })).toBeInTheDocument();
  });

  it("respects read-only detail controls", () => {
    render(<Harness readOnly initialSelection={{ kind: "stop", dayIndex: 0, stopID: "b-1" }} />);

    const dialog = screen.getByRole("dialog");
    expect(within(dialog).queryByRole("button", { name: "调整这一天" })).not.toBeInTheDocument();
  });

  it("omits reminders when there are none", () => {
    render(
      <BoardView
        model={{ ...model, reminders: [] }}
        activeDay={null}
        selection={null}
        onSelect={vi.fn()}
      />,
    );

    expect(screen.queryByText("提醒")).not.toBeInTheDocument();
  });
});