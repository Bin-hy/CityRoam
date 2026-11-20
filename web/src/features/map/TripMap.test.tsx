import type { ComponentProps } from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { act } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ResultDay, ResultHotel, ResultLeg } from "@/features/result/model";
import { TripMap } from "./TripMap";

const amap = vi.hoisted(() => {
  const markers: FakeMarker[] = [];
  const polylines: FakePolyline[] = [];
  const fitCalls: unknown[][] = [];
  const mapInstance = {
    destroy: vi.fn(),
    remove: vi.fn(),
    setFitView: vi.fn((target?: unknown[]) => {
      fitCalls.push(target ?? []);
    }),
    setZoomAndCenter: vi.fn(),
    resize: vi.fn(),
  };

  class FakeMap {
    constructor() {
      return mapInstance as unknown as FakeMap;
    }
  }
  class FakeMarker {
    handlers: Record<string, () => void> = {};
    position: [number, number];
    title: string | undefined;
    content: string | undefined;
    zIndex: number;
    constructor(options: { position: [number, number]; title?: string; content?: string; zIndex?: number }) {
      this.position = options.position;
      this.title = options.title;
      this.content = options.content;
      this.zIndex = options.zIndex ?? 0;
      markers.push(this);
    }
    on(name: string, handler: () => void) {
      this.handlers[name] = handler;
    }
    getPosition() {
      return this.position;
    }
    setzIndex(index: number) {
      this.zIndex = index;
    }
    setContent(content: string) {
      this.content = content;
    }
    setMap() {}
  }
  class FakePolyline {
    options: Record<string, unknown>;
    handlers: Record<string, () => void> = {};
    constructor(options: Record<string, unknown>) {
      this.options = { ...options };
      polylines.push(this);
    }
    on(name: string, handler: () => void) {
      this.handlers[name] = handler;
    }
    setOptions(options: Record<string, unknown>) {
      Object.assign(this.options, options);
    }
    setMap() {}
  }

  return {
    markers,
    polylines,
    fitCalls,
    mapInstance,
    namespace: { Map: FakeMap, Marker: FakeMarker, Polyline: FakePolyline } as unknown,
    hasKey: true,
    load: vi.fn(),
  };
});

vi.mock("@/lib/amap", () => ({
  hasAMapKey: () => amap.hasKey,
  loadAMap: () => amap.load(),
}));

const stops = [
  {
    id: "b-1",
    dayIndex: 0,
    order: 0,
    type: "poi",
    typeLabel: "景点",
    title: "<b>故宫</b>",
    start: "09:00",
    end: "11:00",
    durationMin: 120,
    costYuan: 60,
    lat: 39.9163,
    lng: 116.3972,
    reason: null,
    guides: [],
  },
  {
    id: "b-2",
    dayIndex: 0,
    order: 1,
    type: "meal",
    typeLabel: "餐饮",
    title: "午餐",
    start: "11:30",
    end: "12:30",
    durationMin: 60,
    costYuan: 40,
    lat: 39.911,
    lng: 116.4,
    reason: null,
    guides: [],
  },
  {
    id: "b-3",
    dayIndex: 0,
    order: 2,
    type: "rest",
    typeLabel: "休息",
    title: "休息一下",
    start: "13:00",
    end: "13:30",
    durationMin: 30,
    costYuan: 0,
    lat: 39.9,
    lng: 116.41,
    reason: null,
    guides: [],
  },
];

function leg(overrides: Partial<ResultLeg> = {}): ResultLeg {
  return {
    id: "0:b-1>b-2",
    dayIndex: 0,
    from: { id: "b-1", title: "故宫", lat: 39.9163, lng: 116.3972 },
    to: { id: "b-2", title: "午餐", lat: 39.911, lng: 116.4 },
    mode: "transit",
    modeLabel: "公共交通",
    durationMin: 25,
    distanceM: 4200,
    costYuan: 4,
    estimated: false,
    segments: [
      {
        mode: "walk",
        modeLabel: "步行",
        distanceM: 300,
        durationMin: 4,
        costYuan: 0,
        estimated: false,
        polyline: [
          [116.3972, 39.9163],
          [116.399, 39.912],
        ],
      },
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
          [116.399, 39.912],
          [116.4, 39.911],
        ],
      },
    ],
    geometry: "polyline",
    ...overrides,
  };
}

const day: ResultDay = {
  index: 0,
  date: "2026-11-20",
  label: "11 月 20 日 周五",
  theme: "故宫",
  start: "09:00",
  end: "18:00",
  costYuan: 100,
  stops,
  legs: [leg()],
  gaps: [],
};

const hotel: ResultHotel = { name: "全季酒店", lat: 39.93, lng: 116.42, star: 4 };

function renderMap(overrides: Partial<ComponentProps<typeof TripMap>> = {}) {
  return render(<TripMap day={day} hotel={null} {...overrides} />);
}

beforeEach(() => {
  amap.markers.length = 0;
  amap.polylines.length = 0;
  amap.fitCalls.length = 0;
  amap.hasKey = true;
  amap.load.mockReset();
  amap.load.mockResolvedValue(amap.namespace);
  amap.mapInstance.setFitView.mockClear();
});

describe("TripMap", () => {
  it("plots one day of numbered stops, escapes titles and fits them", async () => {
    const onSelectStop = vi.fn();
    renderMap({ hotel, onSelectStop });

    await waitFor(() => expect(amap.markers.length).toBe(4));
    const stopMarkers = amap.markers.filter((marker) => marker.title !== "全季酒店");
    expect(stopMarkers).toHaveLength(3);
    expect(stopMarkers[0].title).toBe("&lt;b&gt;故宫&lt;/b&gt;");
    expect(stopMarkers[0].content).toContain(">1<");
    expect(stopMarkers[1].content).toContain(">2<");
    expect(stopMarkers[2].content).not.toContain(">3<");
    expect(amap.markers.find((marker) => marker.title === "全季酒店")).toBeDefined();
    expect(amap.mapInstance.setFitView).toHaveBeenCalled();

    act(() => stopMarkers[0].handlers.click?.());
    expect(onSelectStop).toHaveBeenCalledWith("b-1");
  });

  it("draws each segment with its mode style and selects the leg on click", async () => {
    const onSelectLeg = vi.fn();
    renderMap({ onSelectLeg });

    await waitFor(() => expect(amap.polylines).toHaveLength(2));
    expect(amap.polylines[0].options).toMatchObject({
      strokeColor: "#64748b",
      strokeStyle: "dashed",
    });
    expect(amap.polylines[1].options).toMatchObject({
      strokeColor: "#f97316",
      strokeStyle: "solid",
      title: expect.stringContaining("地铁1号线"),
    });

    act(() => amap.polylines[0].handlers.click?.());
    expect(onSelectLeg).toHaveBeenCalledWith("0:b-1>b-2");
  });

  it("emphasizes the selected leg and mutes the others", async () => {
    const second = leg({
      id: "0:b-2>b-3",
      from: { id: "b-2", title: "午餐", lat: 39.911, lng: 116.4 },
      to: { id: "b-3", title: "休息一下", lat: 39.9, lng: 116.41 },
      mode: "walk",
      modeLabel: "步行",
      geometry: "straight",
      segments: [],
    });
    renderMap({ day: { ...day, legs: [leg(), second] }, selectedLegID: "0:b-1>b-2" });

    await waitFor(() => expect(amap.polylines).toHaveLength(3));
    expect(amap.polylines[0].options.strokeWeight).toBe(6);
    expect(amap.polylines[1].options.strokeWeight).toBe(6);
    expect(amap.polylines[2].options.strokeOpacity).toBe(0.25);
    expect(amap.fitCalls[amap.fitCalls.length - 1]).toHaveLength(2);
  });

  it("draws a dashed straight line when there is no polyline and skips missing coordinates", async () => {
    const straight = leg({
      id: "0:b-2>b-3",
      mode: "transit",
      from: { id: "b-2", title: "午餐", lat: 39.911, lng: 116.4 },
      to: { id: "b-3", title: "休息一下", lat: 39.9, lng: 116.41 },
      geometry: "straight",
      segments: [],
    });
    const missing = leg({
      id: "0:b-3>b-4",
      from: { id: "b-3", title: "休息一下" },
      to: { id: "b-4", title: "未知" },
      geometry: "none",
      segments: [],
    });
    renderMap({ day: { ...day, legs: [straight, missing] } });

    await waitFor(() => expect(amap.polylines).toHaveLength(1));
    expect(amap.polylines[0].options.strokeStyle).toBe("dashed");
    expect(amap.polylines[0].options.title).toBe("直线估算");
  });

  it("enlarges and raises the selected stop", async () => {
    renderMap({ selectedStopID: "b-1" });

    await waitFor(() => expect(amap.markers).toHaveLength(3));
    const selected = amap.markers.find((marker) => marker.title === "&lt;b&gt;故宫&lt;/b&gt;");
    const other = amap.markers.find((marker) => marker.title === "午餐");
    expect(selected?.zIndex).toBe(100);
    expect(selected?.content).toContain("width:30px");
    expect(other?.zIndex).toBe(10);
  });

  it("falls back to a clickable list of the day without a key", async () => {
    amap.hasKey = false;
    const onSelectStop = vi.fn();
    const onSelectLeg = vi.fn();
    const onSelectHotel = vi.fn();
    renderMap({ hotel, onSelectStop, onSelectLeg, onSelectHotel });

    expect(screen.getByText(/地图暂不可用，仍可查看当天安排与路线详情/)).toBeInTheDocument();
    expect(amap.load).not.toHaveBeenCalled();

    await userEvent.click(screen.getByRole("button", { name: "1. <b>故宫</b>" }));
    expect(onSelectStop).toHaveBeenCalledWith("b-1");
    await userEvent.click(screen.getByRole("button", { name: /公共交通 · 故宫 → 午餐/ }));
    expect(onSelectLeg).toHaveBeenCalledWith("0:b-1>b-2");
    await userEvent.click(screen.getByRole("button", { name: "宿 · 全季酒店" }));
    expect(onSelectHotel).toHaveBeenCalled();
  });
});