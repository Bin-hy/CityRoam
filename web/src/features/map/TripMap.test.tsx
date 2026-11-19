import { render, screen, waitFor } from "@testing-library/react";
import { act } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Itinerary } from "@/features/itinerary/types";
import { TripMap } from "./TripMap";

const amap = vi.hoisted(() => {
  const markers: FakeMarker[] = [];
  const infos: FakeInfoWindow[] = [];
  const mapInstance = {
    destroy: vi.fn(),
    remove: vi.fn(),
    setFitView: vi.fn(),
    setZoomAndCenter: vi.fn(),
  };

  class FakeMap {
    constructor() {
      return mapInstance as unknown as FakeMap;
    }
  }
  class FakeMarker {
    handlers: Record<string, () => void> = {};
    position: [number, number];
    constructor(options: { position: [number, number] }) {
      this.position = options.position;
      markers.push(this);
    }
    on(name: string, handler: () => void) {
      this.handlers[name] = handler;
    }
    getPosition() {
      return this.position;
    }
    setzIndex() {}
    setMap() {}
  }
  class FakePolyline {
    constructor(_options: Record<string, unknown>) {}
    setMap() {}
  }
  class FakeInfoWindow {
    open = vi.fn();
    close = vi.fn();
    constructor(_options: Record<string, unknown>) {
      infos.push(this);
    }
  }

  return {
    markers,
    infos,
    mapInstance,
    namespace: { Map: FakeMap, Marker: FakeMarker, Polyline: FakePolyline, InfoWindow: FakeInfoWindow } as unknown,
    hasKey: true,
    load: vi.fn(),
  };
});

vi.mock("@/lib/amap", () => ({
  hasAMapKey: () => amap.hasKey,
  loadAMap: () => amap.load(),
}));

const itinerary: Itinerary = {
  city: "beijing",
  days: [
    {
      date: "2026-11-20",
      start_time: "09:00",
      end_time: "18:00",
      day_cost_yuan: 60,
      blocks: [
        { id: "b-1", type: "poi", poi_id: "p1", lat: 39.9163, lng: 116.3972, title: "故宫", start: "09:00", end: "11:00", duration_min: 120, cost_yuan: 60 },
        { id: "b-2", type: "transit", title: "步行至天坛", start: "11:00", end: "11:20", duration_min: 20, cost_yuan: 0 },
        { id: "b-3", type: "poi", poi_id: "p2", lat: 39.8822, lng: 116.4066, title: "天坛", start: "11:30", end: "13:00", duration_min: 90, cost_yuan: 0 },
      ],
    },
    {
      date: "2026-11-21",
      start_time: "09:30",
      end_time: "17:00",
      day_cost_yuan: 40,
      blocks: [
        { id: "b-4", type: "poi", poi_id: "p3", lat: 39.924, lng: 116.388, title: "北海公园", start: "09:30", end: "11:30", duration_min: 120, cost_yuan: 40 },
      ],
    },
  ],
  totals: { cost_yuan: 100, poi_count: 3, walking_km: 1.2 },
};

describe("TripMap", () => {
  beforeEach(() => {
    amap.markers.length = 0;
    amap.infos.length = 0;
    amap.hasKey = true;
    amap.load.mockReset();
    amap.load.mockResolvedValue(amap.namespace);
    amap.mapInstance.remove.mockClear();
    amap.mapInstance.setFitView.mockClear();
    amap.mapInstance.setZoomAndCenter.mockClear();
  });

  it("renders a textual fallback without a key", () => {
    amap.hasKey = false;
    render(<TripMap itinerary={itinerary} />);

    expect(screen.getByText("地图暂不可用")).toBeInTheDocument();
    expect(screen.getByText(/未配置 VITE_AMAP_JS_KEY/)).toBeInTheDocument();
    expect(screen.getByText(/第 1 天 · 09:00 故宫/)).toBeInTheDocument();
    expect(amap.load).not.toHaveBeenCalled();
  });

  it("falls back when the loader fails", async () => {
    amap.load.mockRejectedValue(new Error("network down"));
    render(<TripMap itinerary={itinerary} />);

    expect(await screen.findByText("地图暂不可用")).toBeInTheDocument();
    expect(screen.getByText(/network down/)).toBeInTheDocument();
  });

  it("plots blocks with coordinates and forwards marker clicks", async () => {
    const onSelect = vi.fn();
    render(<TripMap itinerary={itinerary} onSelectBlock={onSelect} />);

    await waitFor(() => expect(amap.markers.length).toBe(3));
    expect(amap.mapInstance.setFitView).toHaveBeenCalled();
    expect(screen.getByText("GCJ-02")).toBeInTheDocument();

    act(() => amap.markers[0].handlers.click?.());
    expect(onSelect).toHaveBeenCalledWith("b-1");
  });

  it("centers on the selected block", async () => {
    render(<TripMap itinerary={itinerary} selectedBlockID="b-3" />);

    await waitFor(() => expect(amap.infos).toHaveLength(1));
    expect(amap.mapInstance.setZoomAndCenter).toHaveBeenCalledWith(14, [116.4066, 39.8822]);
    expect(amap.infos[0].open).toHaveBeenCalled();
  });
});
