import { dayDateLabel, type Itinerary } from "@/features/itinerary/types";

export interface TimelineItem {
  id: string;
  group: number;
  content: string;
  start: Date;
  end: Date;
  className: string;
  title: string;
}

export interface TimelineGroup {
  id: number;
  content: string;
}

export interface TimelineModel {
  items: TimelineItem[];
  groups: TimelineGroup[];
  min: Date | null;
  max: Date | null;
}

export function buildTimelineModel(itinerary: Itinerary): TimelineModel {
  const groups: TimelineGroup[] = itinerary.days.map((day, index) => ({
    id: index,
    content: `第 ${index + 1} 天 · ${dayDateLabel(day.date, index)}`,
  }));

  const items: TimelineItem[] = [];
  itinerary.days.forEach((day, dayIndex) => {
    day.blocks.forEach((block, blockIndex) => {
      const start = blockTime(day.date, block.start || day.start_time || "09:00");
      let end = blockTime(day.date, block.end || block.start || day.start_time || "09:30");
      if (end.getTime() <= start.getTime()) {
        end = new Date(end.getTime() + 30 * 60 * 1000);
      }
      const range = `${clock(start)}–${clock(end)}`;
      const cost = block.cost_yuan > 0 ? ` · ¥${block.cost_yuan}` : "";
      const transport = block.transport;
      const content =
        block.type === "transit" && transport
          ? `${transport.mode === "walk" ? "步行" : transport.mode === "drive" ? "驾车" : "交通"} ${transport.duration_min} 分钟`
          : `${block.title}${cost}`;

      items.push({
        id: block.id || `block-${dayIndex}-${blockIndex}`,
        group: dayIndex,
        content: escapeHTML(content),
        start,
        end,
        className: `tp-block tp-block-${block.type}`,
        title: escapeHTML(`${range} ${block.title}${cost}`),
      });
    });
  });

  const min = items.reduce<Date | null>((earliest, item) => (!earliest || item.start < earliest ? item.start : earliest), null);
  const max = items.reduce<Date | null>((latest, item) => (!latest || item.end > latest ? item.end : latest), null);
  return { items, groups, min, max };
}

export function parseBlockDate(date: string, clockText: string): Date {
  return blockTime(date, clockText);
}

function blockTime(date: string, clockText: string): Date {
  const [hour = "09", minute = "00"] = clockText.split(":");
  const [year, month, day] = date.split("-").map((part) => Number.parseInt(part, 10));
  if (!year || !month || !day) {
    return new Date(`1970-01-01T${hour.padStart(2, "0")}:${minute.padStart(2, "0")}:00`);
  }
  return new Date(year, month - 1, day, Number.parseInt(hour, 10) || 0, Number.parseInt(minute, 10) || 0);
}

function clock(date: Date): string {
  return `${String(date.getHours()).padStart(2, "0")}:${String(date.getMinutes()).padStart(2, "0")}`;
}

function escapeHTML(value: string): string {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}
