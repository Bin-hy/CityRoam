import { useEffect, useRef } from "react";
import { DataSet } from "vis-data";
import { Timeline, type TimelineOptions } from "vis-timeline/standalone";
import "vis-timeline/styles/vis-timeline-graph2d.css";
import type { Itinerary } from "@/features/itinerary/types";
import { buildTimelineModel, type TimelineGroup, type TimelineItem } from "./timelineModel";

interface ItineraryTimelineProps {
  itinerary: Itinerary;
  selectedBlockID?: string | null;
  onSelectBlock?: (blockID: string | null) => void;
  editable?: boolean;
  onMoveBlock?: (blockID: string, start: Date, end: Date) => void;
}

export function ItineraryTimeline({
  itinerary,
  selectedBlockID,
  onSelectBlock,
  editable = false,
  onMoveBlock,
}: ItineraryTimelineProps) {
  const container = useRef<HTMLDivElement>(null);
  const timelineRef = useRef<Timeline | null>(null);
  const selectRef = useRef(onSelectBlock);
  const moveRef = useRef(onMoveBlock);
  selectRef.current = onSelectBlock;
  moveRef.current = onMoveBlock;

  useEffect(() => {
    if (!container.current) return;
    const timeline = new Timeline(container.current, [], [], baseOptions());
    timeline.on("select", (event: { items: Array<string | number> }) => {
      const first = event.items[0];
      selectRef.current?.(first === undefined ? null : String(first));
    });
    timeline.on("timechange", (event: { id?: string | number; start: Date; end: Date }) => {
      if (event.id !== undefined) {
        moveRef.current?.(String(event.id), event.start, event.end);
      }
    });
    timelineRef.current = timeline;
    return () => {
      timeline.destroy();
      timelineRef.current = null;
    };
  }, []);

  useEffect(() => {
    const timeline = timelineRef.current;
    if (!timeline) return;
    const model = buildTimelineModel(itinerary);
    timeline.setData({
      items: new DataSet<TimelineItem>(model.items),
      groups: new DataSet<TimelineGroup>(model.groups),
    });
    timeline.setOptions({
      ...baseOptions(),
      editable: editable ? { updateTime: true, updateGroup: false, add: false, remove: false } : false,
      min: model.min ?? undefined,
      max: model.max ?? undefined,
    });
    timeline.fit({ animation: false });
  }, [itinerary, editable]);

  useEffect(() => {
    const timeline = timelineRef.current;
    if (!timeline) return;
    if (selectedBlockID) {
      timeline.setSelection([selectedBlockID]);
    } else {
      timeline.setSelection([]);
    }
  }, [selectedBlockID]);

  return <div ref={container} className="min-h-56 w-full" data-testid="itinerary-timeline" />;
}

function baseOptions(): TimelineOptions {
  return {
    stack: true,
    stackSubgroups: true,
    zoomable: true,
    moveable: true,
    selectable: true,
    showCurrentTime: false,
    showTooltips: false,
    margin: { item: { horizontal: 0, vertical: 4 } },
    orientation: "top",
    format: {
      minorLabels: { hour: "HH:mm", minute: "HH:mm" },
      majorLabels: { day: "M 月 D 日", weekday: "ddd" },
    },
    start: undefined,
    end: undefined,
  };
}
