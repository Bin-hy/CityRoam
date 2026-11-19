import { diff as jsonDiff } from "jsondiffpatch";
import type { Block, DayPlan, Itinerary } from "@/features/itinerary/types";

export type ChangeKind = "added" | "removed" | "modified";

export interface FieldChange {
  field: string;
  before?: unknown;
  after?: unknown;
}

export interface BlockDiff {
  blockID: string;
  title: string;
  change: ChangeKind;
  fields: FieldChange[];
  before?: Block;
  after?: Block;
}

export interface DayDiff {
  dayIndex: number;
  date: string;
  change: ChangeKind;
  fields: FieldChange[];
  blocks: BlockDiff[];
  before?: DayPlan;
  after?: DayPlan;
}

const fieldLabels: Record<string, string> = {
  title: "标题",
  start: "开始",
  end: "结束",
  duration_min: "时长(分钟)",
  cost_yuan: "花费(¥)",
  type: "类型",
  poi_id: "地点",
  notes: "备注",
  date: "日期",
  start_time: "开始时间",
  end_time: "结束时间",
  day_cost_yuan: "当天花费(¥)",
};

export function fieldLabel(field: string): string {
  return fieldLabels[field] ?? field;
}

export function formatFieldValue(field: string, value: unknown): string {
  if (value === undefined || value === null) return "—";
  if (field === "cost_yuan" || field === "day_cost_yuan") return `¥${String(value)}`;
  return String(value);
}

function presentBlock(block: Block) {
  return {
    type: block.type,
    poi_id: block.poi_id,
    title: block.title,
    start: block.start,
    end: block.end,
    duration_min: block.duration_min,
    cost_yuan: block.cost_yuan,
    notes: block.notes,
  };
}

function presentDay(day: DayPlan) {
  return {
    date: day.date,
    start_time: day.start_time,
    end_time: day.end_time,
    day_cost_yuan: day.day_cost_yuan,
  };
}

function flattenDelta(delta: unknown): FieldChange[] {
  if (!delta || typeof delta !== "object") return [];
  const changes: FieldChange[] = [];
  for (const [field, value] of Object.entries(delta as Record<string, unknown>)) {
    if (Array.isArray(value)) {
      if (value.length >= 3 && value[1] === 0 && value[2] === 0) {
        changes.push({ field, before: value[0] });
        continue;
      }
      if (value.length === 1) {
        changes.push({ field, after: value[0] });
        continue;
      }
      changes.push({ field, before: value[0], after: value[1] });
      continue;
    }
    if (value && typeof value === "object" && "_t" in value) {
      changes.push({ field, after: JSON.stringify(value) });
      continue;
    }
    changes.push({ field, after: value });
  }
  return changes;
}

function blockKey(block: Block, index: number): string {
  return block.id || `index:${index}`;
}

function diffBlocks(before: DayPlan, after: DayPlan): BlockDiff[] {
  const changes: BlockDiff[] = [];
  const beforeByKey = new Map(before.blocks.map((block, index) => [blockKey(block, index), block]));
  const afterByKey = new Map(after.blocks.map((block, index) => [blockKey(block, index), block]));

  after.blocks.forEach((block, index) => {
    const prior = beforeByKey.get(blockKey(block, index));
    if (!prior) {
      changes.push({
        blockID: block.id,
        title: block.title,
        change: "added",
        fields: [],
        after: block,
      });
      return;
    }
    const delta = jsonDiff(presentBlock(prior), presentBlock(block));
    if (!delta) return;
    changes.push({
      blockID: block.id,
      title: block.title,
      change: "modified",
      fields: flattenDelta(delta),
      before: prior,
      after: block,
    });
  });

  before.blocks.forEach((block, index) => {
    if (afterByKey.has(blockKey(block, index))) return;
    changes.push({
      blockID: block.id,
      title: block.title,
      change: "removed",
      fields: [],
      before: block,
    });
  });

  return changes;
}

export function diffItineraries(before: Itinerary, after: Itinerary): DayDiff[] {
  const result: DayDiff[] = [];
  const total = Math.max(before.days.length, after.days.length);

  for (let index = 0; index < total; index += 1) {
    const beforeDay = before.days[index];
    const afterDay = after.days[index];

    if (!beforeDay && afterDay) {
      result.push({
        dayIndex: index,
        date: afterDay.date,
        change: "added",
        fields: [],
        blocks: diffBlocks({ ...afterDay, blocks: [] }, afterDay),
        after: afterDay,
      });
      continue;
    }
    if (beforeDay && !afterDay) {
      result.push({
        dayIndex: index,
        date: beforeDay.date,
        change: "removed",
        fields: [],
        blocks: diffBlocks(beforeDay, { ...beforeDay, blocks: [] }),
        before: beforeDay,
      });
      continue;
    }
    if (!beforeDay || !afterDay) continue;

    const dayDelta = jsonDiff(presentDay(beforeDay), presentDay(afterDay));
    const blocks = diffBlocks(beforeDay, afterDay);
    if (!dayDelta && blocks.length === 0) continue;

    result.push({
      dayIndex: index,
      date: afterDay.date || beforeDay.date,
      change: "modified",
      fields: flattenDelta(dayDelta),
      blocks,
      before: beforeDay,
      after: afterDay,
    });
  }

  return result;
}
