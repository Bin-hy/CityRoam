import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ResultDay, ResultModel, ResultSelection, ResultStop } from "./model";
import { DossierView } from "./DossierView";

const stop = (overrides: Partial<ResultStop>): ResultStop => ({
  id: "s",
  dayIndex: 0,
  order: 0,
  type: "poi",
  typeLabel: "景点",
  title: "地点",
  start: "09:00",
  end: "10:00",
  durationMin: 60,
  costYuan: 0,
  reason: null,
  guides: [],
  ...overrides,
});

const day1: ResultDay = {
  index: 0,
  date: "2026-11-20",
  label: "11 月 20 日 周五",
  theme: "故宫 · 天坛",
  start: "09:00",
  end: "18:00",
  costYuan: 100,
  stops: [
    stop({
      id: "b-1",
      title: "故宫",
      start: "09:00",
      end: "11:00",
      durationMin: 120,
      costYuan: 60,
      lat: 39.9163,
      lng: 116.3972,
      reason: "中轴线起点。",
      guides: [
        {
          poiID: "p1",
          note: "开门就进。",
          citations: [
            { title: "故宫攻略", url: "https://example.com/gugong", site: "示例旅行" },
          ],
        },
      ],
    }),
    stop({
      id: "b-2",
      dayIndex: 0,
      order: 1,
      title: "天坛",
      start: "11:30",
      end: "13:00",
      durationMin: 90,
      costYuan: 40,
      lat: 39.8822,
      lng: 116.4066,
    }),
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
        { mode: "walk", modeLabel: "步行", distanceM: 1200, durationMin: 20, costYuan: 0, estimated: false, polyline: [] },
      ],
      geometry: "straight",
    },
  ],
  gaps: [
    { id: "0:gap:14:00", dayIndex: 0, start: "14:00", end: "15:00", minutes: 60, suggestion: "自由活动" },
  ],
};

const day2: ResultDay = {
  index: 1,
  date: "2026-11-21",
  label: "11 月 21 日 周六",
  theme: "北海公园",
  start: "09:30",
  end: "17:00",
  costYuan: 40,
  stops: [
    stop({
      id: "b-3",
      dayIndex: 1,
      title: "北海公园",
      start: "09:30",
      end: "11:30",
      durationMin: 120,
      costYuan: 40,
      lat: 39.924,
      lng: 116.388,
    }),
  ],
  legs: [],
  gaps: [],
};

const model: ResultModel = {
  city: "北京",
  title: "北京 · 2 天",
  summary: "2 天串起故宫与天坛",
  overview: { days: 2, poiCount: 3, walkingKM: 1.2, pace: "relaxed" },
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
    hotel: {
      name: "全季酒店",
      address: "东城区东单北大街",
      lat: 39.93,
      lng: 116.42,
      star: 4,
      score: 8.8,
      bookingURL: "https://example.com/book",
    },
    nights: 2,
    nightlyYuan: 480,
    totalYuan: 960,
    anchorDays: [0, 1],
    includeInBudget: false,
    priceEstimated: false,
  },
  reminders: [{ id: "research:0", text: "故宫周一闭馆。" }],
  days: [day1, day2],
};

function renderDossier(overrides: Partial<Parameters<typeof DossierView>[0]> = {}) {
  return render(
    <DossierView
      model={model}
      activeDay={null}
      selection={null}
      onSelect={vi.fn()}
      onViewOnMap={vi.fn()}
      onAdjustDay={vi.fn()}
      {...overrides}
    />,
  );
}

afterEach(() => {
  delete (HTMLElement.prototype as { scrollTo?: unknown }).scrollTo;
});

describe("DossierView", () => {
  it("renders the overview and day chapters in order", () => {
    renderDossier();

    expect(screen.getByRole("heading", { name: "北京 · 2 天" })).toBeInTheDocument();
    expect(screen.getByText("2 天串起故宫与天坛")).toBeInTheDocument();
    expect(screen.getByText("3 个")).toBeInTheDocument();
    expect(screen.getByText("¥1,144")).toBeInTheDocument();
    expect(screen.getByText("不计住宿")).toBeInTheDocument();

    const chapters = screen.getAllByRole("article");
    expect(chapters.map((chapter) => chapter.getAttribute("aria-label"))).toEqual(["第 1 天", "第 2 天"]);
  });

  it("shows reasons, guide sources and gaps inline", () => {
    renderDossier();

    const first = screen.getByRole("article", { name: "第 1 天" });
    expect(within(first).getByText("中轴线起点。")).toBeInTheDocument();
    expect(within(first).getByText("开门就进。")).toBeInTheDocument();
    expect(within(first).getByRole("link", { name: "故宫攻略" })).toHaveAttribute(
      "href",
      "https://example.com/gugong",
    );
    expect(within(first).getByText("未排时段")).toBeInTheDocument();
    expect(within(first).getByText("14:00–15:00 · 60 分钟 · 自由活动")).toBeInTheDocument();
    expect(within(first).getByText(/当天合计/)).toBeInTheDocument();
  });

  it("filters to the active day and scrolls the container", async () => {
    const scrollTo = vi.fn();
    HTMLElement.prototype.scrollTo = scrollTo;
    const view = renderDossier({ activeDay: null });

    view.rerender(
      <DossierView
        model={model}
        activeDay={1}
        selection={null}
        onSelect={vi.fn()}
        onViewOnMap={vi.fn()}
        onAdjustDay={vi.fn()}
      />,
    );

    expect(screen.queryByRole("article", { name: "第 1 天" })).not.toBeInTheDocument();
    expect(screen.getByRole("article", { name: "第 2 天" })).toBeInTheDocument();
    expect(scrollTo).toHaveBeenCalled();
  });

  it("selects stops and expands the selected leg", async () => {
    const onSelect = vi.fn();
    renderDossier({ onSelect, selection: { kind: "leg", dayIndex: 0, legID: "0:b-1>b-2" } });

    expect(screen.getByRole("button", { name: "停靠点：故宫" })).toHaveAttribute(
      "aria-pressed",
      "false",
    );
    expect(screen.getByText("直线估算，仅供距离参考")).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: "查看路线图" }).length).toBeGreaterThan(0);

    await userEvent.click(screen.getByRole("button", { name: /09:00–11:00.*故宫/ }));
    expect(onSelect).toHaveBeenCalledWith({ kind: "stop", dayIndex: 0, stopID: "b-1" });
  });

  it("summarizes lodging and reminders and opens the hotel detail", async () => {
    const onSelect = vi.fn();
    renderDossier({ onSelect });

    expect(screen.getByText("全季酒店")).toBeInTheDocument();
    expect(screen.getByText(/2 晚 · ¥480\/晚 · 合计 ¥960 · 落点在第 1 天、第 2 天/)).toBeInTheDocument();
    expect(screen.getByText("故宫周一闭馆。")).toBeInTheDocument();

    await userEvent.click(screen.getByText("全季酒店"));
    expect(onSelect).toHaveBeenCalledWith({ kind: "hotel" });
  });

  it("hides adjust, map and hotel actions in read-only mode", () => {
    const selection: ResultSelection = { kind: "leg", dayIndex: 0, legID: "0:b-1>b-2" };
    renderDossier({ readOnly: true, selection });

    expect(screen.queryByRole("button", { name: "调整这一天" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "查看路线图" })).not.toBeInTheDocument();
    expect(screen.getByText("全季酒店").closest("button")).toBeNull();
  });

  it("omits the reminders block when there is nothing to say", () => {
    renderDossier({ model: { ...model, reminders: [] } });

    expect(screen.queryByText("提醒")).not.toBeInTheDocument();
  });
});