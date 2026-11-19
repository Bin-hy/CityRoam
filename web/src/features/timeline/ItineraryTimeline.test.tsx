import { act, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Itinerary } from "@/features/itinerary/types";
import { ItineraryTimeline } from "./ItineraryTimeline";
import { buildTimelineModel } from "./timelineModel";

const mocked = vi.hoisted(() => {
  const instances: MockTimeline[] = [];

  class MockTimeline {
    handlers: Record<string, (event: unknown) => void> = {};
    data: { items: { length: number }; groups: { length: number } } | null = null;
    options: Record<string, unknown> | null = null;

    constructor() {
      instances.push(this);
    }

    on(name: string, handler: (event: unknown) => void) {
      this.handlers[name] = handler;
    }

    setData(data: { items: { length: number }; groups: { length: number } }) {
      this.data = data;
    }

    setOptions(options: Record<string, unknown>) {
      this.options = options;
    }

    fit() {}
    setSelection() {}
    destroy() {}

    emit(name: string, event: unknown) {
      this.handlers[name]?.(event);
    }
  }

  return { instances, MockTimeline };
});

vi.mock("vis-timeline/standalone", () => ({ Timeline: mocked.MockTimeline }));

const itinerary: Itinerary = {
  city: "beijing",
  days: [
    {
      date: "2026-11-20",
      start_time: "09:00",
      end_time: "18:00",
      day_cost_yuan: 60,
      blocks: [
        { id: "b-1", type: "poi", poi_id: "p1", title: "故宫", start: "09:00", end: "11:00", duration_min: 120, cost_yuan: 60 },
        { id: "b-2", type: "transit", title: "步行至天坛", start: "11:00", end: "11:20", duration_min: 20, cost_yuan: 0, transport: { mode: "walk", from_poi_id: "p1", to_poi_id: "p2", distance_m: 900, duration_min: 20, cost_yuan: 0, estimated: false } },
      ],
    },
    {
      date: "2026-11-21",
      start_time: "09:30",
      end_time: "17:00",
      day_cost_yuan: 40,
      blocks: [
        { id: "b-3", type: "poi", poi_id: "p2", title: "天坛", start: "09:30", end: "11:30", duration_min: 120, cost_yuan: 40 },
      ],
    },
  ],
  totals: { cost_yuan: 100, poi_count: 2, walking_km: 0.9 },
};

describe("buildTimelineModel", () => {
  it("groups blocks by day and keeps block ids", () => {
    const model = buildTimelineModel(itinerary);

    expect(model.groups).toHaveLength(2);
    expect(model.items.map((item) => item.id)).toEqual(["b-1", "b-2", "b-3"]);
    expect(model.items[0].group).toBe(0);
    expect(model.items[2].group).toBe(1);
    expect(model.items[0].start.getHours()).toBe(9);
    expect(model.items[1].className).toContain("tp-block-transit");
  });
});

describe("ItineraryTimeline", () => {
  beforeEach(() => {
    mocked.instances.length = 0;
  });

  it("renders multiple days and returns the clicked block id", () => {
    const onSelect = vi.fn();
    render(<ItineraryTimeline itinerary={itinerary} onSelectBlock={onSelect} />);

    expect(screen.getByTestId("itinerary-timeline")).toBeInTheDocument();
    const instance = mocked.instances[0];
    expect(instance.data?.groups.length).toBe(2);
    expect(instance.data?.items.length).toBe(3);

    act(() => instance.emit("select", { items: ["b-2"] }));
    expect(onSelect).toHaveBeenCalledWith("b-2");

    act(() => instance.emit("select", { items: [] }));
    expect(onSelect).toHaveBeenLastCalledWith(null);
  });

  it("enables time editing only in edit mode", () => {
    render(<ItineraryTimeline itinerary={itinerary} editable />);
    expect(mocked.instances[0].options?.editable).toEqual({
      updateTime: true,
      updateGroup: false,
      add: false,
      remove: false,
    });
  });
});
