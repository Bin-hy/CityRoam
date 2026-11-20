import { useState } from "react";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ResultDay, ResultModel, ResultSelection } from "./model";
import { MapView } from "./MapView";

vi.mock("@/features/map/TripMap", () => ({
  TripMap: ({
    day,
    hotel,
    onSelectStop,
    onSelectLeg,
    onSelectHotel,
  }: {
    day: ResultDay;
    hotel: { name: string } | null;
    onSelectStop?: (stopID: string) => void;
    onSelectLeg?: (legID: string) => void;
    onSelectHotel?: () => void;
  }) => (
    <div data-testid="map" data-day={String(day.index)} data-hotel={hotel?.name ?? ""}>
      <button type="button" onClick={() => onSelectStop?.("b-1")}>
        地图选点
      </button>
      <button type="button" onClick={() => onSelectLeg?.("0:b-1>b-2")}>
        地图选路段
      </button>
      <button type="button" onClick={() => onSelectHotel?.()}>
        地图选酒店
      </button>
    </div>
  ),
}));

const day1: ResultDay = {
  index: 0,
  date: "2026-11-20",
  label: "11 月 20 日 周五",
  theme: "故宫",
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
  gaps: [],
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
      lat: 39.924,
      lng: 116.388,
    },
  ],
  legs: [],
};

const model: ResultModel = {
  city: "北京",
  title: "北京 · 2 天",
  summary: "2 天串起故宫与天坛",
  overview: { days: 2, poiCount: 3, walkingKM: 1.2, pace: "relaxed" },
  budget: {
    tickets: 100,
    meals: 0,
    transit: 0,
    lodging: 960,
    total: 1060,
    lodgingIncluded: true,
    estimated: false,
  },
  lodging: {
    hotel: { name: "全季酒店", lat: 39.93, lng: 116.42, star: 4, score: 8.8 },
    nights: 1,
    nightlyYuan: 480,
    totalYuan: 480,
    anchorDays: [0],
    includeInBudget: true,
    priceEstimated: false,
  },
  reminders: [],
  days: [day1, day2],
};

function Harness({
  activeDay = 0,
  readOnly = false,
  initialSelection = null,
}: {
  activeDay?: number;
  readOnly?: boolean;
  initialSelection?: ResultSelection;
}) {
  const [selection, setSelection] = useState<ResultSelection>(initialSelection);
  return (
    <MapView
      model={model}
      activeDay={activeDay}
      onActiveDayChange={vi.fn()}
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

describe("MapView", () => {
  it("passes only the active day to the map", () => {
    render(<Harness activeDay={1} />);

    expect(screen.getByTestId("map")).toHaveAttribute("data-day", "1");
    expect(screen.getByText("北海公园")).toBeInTheDocument();
    expect(screen.queryByText("故宫")).not.toBeInTheDocument();
    expect(screen.getByTestId("map")).toHaveAttribute("data-hotel", "");
  });

  it("passes the hotel only on anchor days", () => {
    const view = render(<Harness activeDay={0} />);
    expect(screen.getByTestId("map")).toHaveAttribute("data-hotel", "全季酒店");
    view.unmount();

    render(<Harness activeDay={1} />);
    expect(screen.getByTestId("map")).toHaveAttribute("data-hotel", "");
  });

  it("opens the shared detail when the map selects a stop", async () => {
    render(<Harness />);

    await userEvent.click(screen.getByRole("button", { name: "地图选点" }));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByRole("heading", { name: "故宫" })).toBeInTheDocument();
    expect(within(dialog).getByText("中轴线起点。")).toBeInTheDocument();
  });

  it("keeps story and map selection in sync for legs", async () => {
    render(<Harness />);

    await userEvent.click(screen.getByRole("button", { name: /地图选路段/ }));
    expect(
      within(screen.getByRole("dialog")).getByText("直线估算，仅供距离参考"),
    ).toBeInTheDocument();

    await userEvent.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: /步行.*20 分钟/ }));
    expect(
      within(screen.getByRole("dialog")).getByText("直线估算，仅供距离参考"),
    ).toBeInTheDocument();
  });

  it("selects a stop from the story row", async () => {
    render(<Harness />);

    await userEvent.click(screen.getByRole("button", { name: /故宫.*09:00–11:00/ }));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByRole("heading", { name: "故宫" })).toBeInTheDocument();
  });

  it("does not open the detail without a selection and respects read-only", async () => {
    const view = render(<Harness />);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    view.unmount();

    render(<Harness readOnly initialSelection={{ kind: "stop", dayIndex: 0, stopID: "b-1" }} />);
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).queryByRole("button", { name: "调整这一天" })).not.toBeInTheDocument();
  });

  it("opens the hotel detail from the map", async () => {
    render(<Harness />);

    await userEvent.click(screen.getByRole("button", { name: "地图选酒店" }));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByRole("heading", { name: "全季酒店" })).toBeInTheDocument();
  });
});