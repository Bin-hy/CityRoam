import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ResultDay, ResultLeg, ResultLodging, ResultStop } from "./model";
import { DetailCard } from "./DetailCard";

const stop: ResultStop = {
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
};

const day: ResultDay = {
  index: 0,
  date: "2026-11-20",
  label: "11 月 20 日 周五",
  theme: "故宫",
  start: "09:00",
  end: "18:00",
  costYuan: 64,
  stops: [stop],
  legs: [],
  gaps: [],
};

const leg: ResultLeg = {
  id: "0:b-1>b-2",
  dayIndex: 0,
  from: { id: "b-1", title: "故宫" },
  to: { id: "b-2", title: "天坛" },
  mode: "walk",
  modeLabel: "步行",
  durationMin: 18,
  distanceM: 1200,
  costYuan: 0,
  estimated: false,
  segments: [
    {
      mode: "walk",
      modeLabel: "步行",
      distanceM: 1200,
      durationMin: 18,
      costYuan: 0,
      estimated: false,
      polyline: [],
    },
  ],
  geometry: "straight",
};

const lodging: ResultLodging = {
  hotel: {
    name: "全季酒店",
    address: "东城区东单北大街",
    lat: 39.93,
    lng: 116.42,
    star: 4,
    score: 8.8,
    bookingURL: "https://example.com/book",
    provider: "amap",
  },
  nights: 2,
  nightlyYuan: 480,
  totalYuan: 960,
  anchorDays: [0, 1],
  includeInBudget: true,
  priceEstimated: false,
};

const originalMatchMedia = window.matchMedia;

afterEach(() => {
  Object.defineProperty(window, "matchMedia", {
    configurable: true,
    writable: true,
    value: originalMatchMedia,
  });
});

describe("DetailCard", () => {
  it("floats on desktop without blocking the view switcher", () => {
    Object.defineProperty(window, "matchMedia", {
      configurable: true,
      writable: true,
      value: (query: string) => ({
        matches: true,
        media: query,
        onchange: null,
        addEventListener: () => undefined,
        removeEventListener: () => undefined,
        addListener: () => undefined,
        removeListener: () => undefined,
        dispatchEvent: () => false,
      }),
    });

    render(<DetailCard target={{ kind: "stop", day, stop }} onClose={vi.fn()} />);

    expect(screen.getByRole("dialog")).not.toHaveAttribute("aria-modal", "true");
    expect(document.querySelector("[data-radix-dialog-overlay]")).toBeNull();
  });

  it("renders a stop and forwards the adjust action", async () => {
    const onAdjustDay = vi.fn();
    render(
      <DetailCard
        target={{ kind: "stop", day, stop }}
        onClose={vi.fn()}
        onAdjustDay={onAdjustDay}
      />,
    );

    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "故宫" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "调整这一天" }));
    expect(onAdjustDay).toHaveBeenCalledWith(0);
  });

  it("renders a leg with the map action", async () => {
    const onViewOnMap = vi.fn();
    render(
      <DetailCard
        target={{ kind: "leg", day, leg }}
        onClose={vi.fn()}
        onViewOnMap={onViewOnMap}
      />,
    );

    expect(screen.getByText("直线估算，仅供距离参考")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "查看路线图" }));
    expect(onViewOnMap).toHaveBeenCalledWith(0);
  });

  it("renders the hotel with booking link, anchors and budget toggle", async () => {
    const onToggleLodging = vi.fn();
    render(
      <DetailCard
        target={{ kind: "hotel", lodging }}
        onClose={vi.fn()}
        onToggleLodging={onToggleLodging}
      />,
    );

    expect(screen.getByRole("heading", { name: "全季酒店" })).toBeInTheDocument();
    expect(screen.getByText("4 星")).toBeInTheDocument();
    expect(screen.getByText("8.8 分")).toBeInTheDocument();
    expect(screen.getByText("东城区东单北大街")).toBeInTheDocument();
    expect(screen.getByText("第 1 天、第 2 天")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "查看预订" })).toHaveAttribute(
      "href",
      "https://example.com/book",
    );

    await userEvent.click(screen.getByRole("checkbox", { name: /计入预算/ }));
    expect(onToggleLodging).toHaveBeenCalled();
  });

  it("drops a non-http booking link", () => {
    render(
      <DetailCard
        target={{
          kind: "hotel",
          lodging: {
            ...lodging,
            hotel: { ...lodging.hotel, bookingURL: "javascript:alert(1)" },
          },
        }}
        onClose={vi.fn()}
      />,
    );

    expect(screen.queryByRole("link", { name: "查看预订" })).not.toBeInTheDocument();
  });

  it("closes on Escape and the close button", async () => {
    const onClose = vi.fn();
    const view = render(<DetailCard target={{ kind: "stop", day, stop }} onClose={onClose} />);

    await userEvent.keyboard("{Escape}");
    expect(onClose).toHaveBeenCalled();

    view.unmount();
    const second = vi.fn();
    render(<DetailCard target={{ kind: "stop", day, stop }} onClose={second} />);
    await userEvent.click(screen.getByRole("button", { name: "关闭" }));
    expect(second).toHaveBeenCalled();
  });

  it("hides mutation controls in read-only mode", () => {
    render(
      <DetailCard
        target={{ kind: "hotel", lodging }}
        onClose={vi.fn()}
        onToggleLodging={vi.fn()}
        readOnly
      />,
    );

    expect(screen.queryByRole("checkbox", { name: /计入预算/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "调整这一天" })).not.toBeInTheDocument();
  });
});