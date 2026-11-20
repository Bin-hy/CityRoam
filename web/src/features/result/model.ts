import { formatDateLabel } from "@/design/tokens";
import { budgetBreakdown } from "@/features/budget/budgetModel";
import {
  explanationMap,
  type Block,
  type DayPlan,
  type Itinerary,
  type TransitLeg,
  type Transport,
} from "@/features/itinerary/types";
import type { ResearchBrief } from "@/features/research/types";

export type ResultViewMode = "dossier" | "map" | "board";

export type ResultSelection =
  | { kind: "stop"; dayIndex: number; stopID: string }
  | { kind: "leg"; dayIndex: number; legID: string }
  | { kind: "hotel" }
  | null;

export type ResultTarget =
  | { kind: "stop"; day: ResultDay; stop: ResultStop }
  | { kind: "leg"; day: ResultDay; leg: ResultLeg }
  | { kind: "hotel"; lodging: ResultLodging }
  | null;

export type ResultPace = "relaxed" | "balanced" | "packed";

export interface ResultModel {
  city: string;
  title: string;
  summary: string;
  overview: ResultOverview;
  budget: ResultBudget;
  lodging: ResultLodging | null;
  reminders: ResultReminder[];
  days: ResultDay[];
}

export interface ResultOverview {
  days: number;
  poiCount: number;
  walkingKM: number;
  pace: ResultPace;
}

export interface ResultBudget {
  tickets: number;
  meals: number;
  transit: number;
  lodging: number;
  total: number;
  lodgingIncluded: boolean;
  estimated: boolean;
}

export interface ResultDay {
  index: number;
  date: string;
  label: string;
  theme: string;
  start: string;
  end: string;
  costYuan: number;
  stops: ResultStop[];
  legs: ResultLeg[];
  gaps: ResultGap[];
}

export interface ResultStop {
  id: string;
  dayIndex: number;
  order: number;
  type: string;
  typeLabel: string;
  title: string;
  start: string;
  end: string;
  durationMin: number;
  costYuan: number;
  lat?: number;
  lng?: number;
  reason: string | null;
  guides: ResultGuide[];
  note?: string;
}

export interface ResultPoint {
  id: string;
  title: string;
  lat?: number;
  lng?: number;
}

export interface ResultLeg {
  id: string;
  dayIndex: number;
  from: ResultPoint;
  to: ResultPoint;
  mode: string;
  modeLabel: string;
  durationMin: number;
  distanceM: number;
  costYuan: number;
  estimated: boolean;
  segments: ResultLegSegment[];
  geometry: "polyline" | "straight" | "none";
}

export interface ResultLegSegment {
  mode: string;
  modeLabel: string;
  lineName?: string;
  fromStop?: string;
  toStop?: string;
  distanceM: number;
  durationMin: number;
  costYuan: number;
  estimated: boolean;
  polyline: [number, number][];
}

export interface ResultGap {
  id: string;
  dayIndex: number;
  start: string;
  end: string;
  minutes: number;
  suggestion: string;
}

export interface ResultReminder {
  id: string;
  text: string;
}

export interface ResultLodging {
  hotel: ResultHotel;
  nights: number;
  nightlyYuan: number;
  totalYuan: number;
  anchorDays: number[];
  includeInBudget: boolean;
  priceEstimated: boolean;
}

export interface ResultHotel {
  name: string;
  address?: string;
  city?: string;
  lat: number;
  lng: number;
  star?: number;
  score?: number | null;
  bookingURL?: string;
  provider?: string;
}

export interface ResultGuide {
  poiID: string;
  note?: string;
  citations: ResultCitation[];
}

export interface ResultCitation {
  title?: string;
  url?: string;
  site?: string;
  publishedAt?: string | null;
  authority?: number;
}

export interface ResultInput {
  itinerary: Itinerary | null;
  explanations?: Map<string, string> | null;
  research?: ResearchBrief | null;
}

export const modeLabels: Record<string, string> = {
  walk: "步行",
  bus: "公交",
  subway: "地铁",
  transit: "公共交通",
  drive: "驾车",
};

export const paceLabels: Record<ResultPace, string> = {
  relaxed: "轻松",
  balanced: "适中",
  packed: "紧凑",
};

export const typeLabels: Record<string, string> = {
  poi: "景点",
  meal: "餐饮",
  transit: "交通",
  rest: "休息",
};

const STOP_TYPES = new Set(["poi", "meal", "rest"]);
const GAP_MIN_MINUTES = 45;

export function buildResultModel(input: ResultInput): ResultModel | null {
  const itinerary = input.itinerary;
  if (!itinerary) return null;

  const explanations = input.explanations ?? explanationMap(itinerary);
  const research = input.research ?? null;
  const days = itinerary.days.map((day, index) =>
    toResultDay(day, index, explanations, research),
  );
  const budget = budgetBreakdown(itinerary);
  const walkingKM = finite(itinerary.totals?.walking_km, 0);
  const poiCount = finite(
    itinerary.totals?.poi_count,
    days.reduce(
      (sum, day) => sum + day.stops.filter((stop) => stop.type === "poi").length,
      0,
    ),
  );
  const city = cityLabel(itinerary.city);

  return {
    city,
    title: city ? `${city} · ${days.length} 天` : `${days.length} 天行程`,
    summary: summarize(days),
    overview: {
      days: days.length,
      poiCount,
      walkingKM,
      pace: resolvePace(days, walkingKM),
    },
    budget: {
      tickets: budget.tickets,
      meals: budget.meals,
      transit: budget.transit,
      lodging: budget.lodging,
      total: budget.total,
      lodgingIncluded: itinerary.lodging?.include_in_budget === true,
      estimated: budget.estimated,
    },
    lodging: toLodging(itinerary),
    reminders: toReminders(research),
    days,
  };
}

export function resolveResultSelection(
  model: ResultModel,
  selection: ResultSelection,
): ResultTarget {
  if (!selection) return null;

  if (selection.kind === "hotel") {
    return model.lodging ? { kind: "hotel", lodging: model.lodging } : null;
  }

  const day = model.days[selection.dayIndex];
  if (!day) return null;

  if (selection.kind === "stop") {
    const stop = day.stops.find((item) => item.id === selection.stopID);
    return stop ? { kind: "stop", day, stop } : null;
  }

  const leg = day.legs.find((item) => item.id === selection.legID);
  return leg ? { kind: "leg", day, leg } : null;
}

function toResultDay(
  day: DayPlan,
  index: number,
  explanations: Map<string, string>,
  research: ResearchBrief | null,
): ResultDay {
  const stops = toStops(day, index, explanations, research);
  return {
    index,
    date: day.date,
    label: formatDateLabel(day.date, index),
    theme: dayTheme(stops, index),
    start: day.start_time,
    end: day.end_time,
    costYuan: finite(day.day_cost_yuan, 0),
    stops,
    legs: toLegs(day, stops, index),
    gaps: toGaps(day, stops, index),
  };
}

function toStops(
  day: DayPlan,
  dayIndex: number,
  explanations: Map<string, string>,
  research: ResearchBrief | null,
): ResultStop[] {
  const stops: ResultStop[] = [];

  for (const block of day.blocks) {
    if (!STOP_TYPES.has(block.type)) continue;
    const poiID = clean(block.poi_id);
    const reason = poiID ? explanations.get(poiID) ?? null : null;

    stops.push({
      id: block.id,
      dayIndex,
      order: stops.length,
      type: block.type,
      typeLabel: typeLabels[block.type] ?? block.type,
      title: block.title,
      start: block.start,
      end: block.end,
      durationMin: finite(block.duration_min, 0),
      costYuan: finite(block.cost_yuan, 0),
      ...coordinates(block.lat, block.lng),
      reason,
      guides: poiID ? guidesFor(research, poiID) : [],
      ...(clean(block.notes) ? { note: clean(block.notes) } : {}),
    });
  }

  return stops;
}

function toLegs(day: DayPlan, stops: ResultStop[], dayIndex: number): ResultLeg[] {
  const legs: ResultLeg[] = [];

  for (let index = 0; index < stops.length - 1; index += 1) {
    const fromStop = stops[index];
    const toStop = stops[index + 1];
    const transport = transportBetween(day.blocks, fromStop.id, toStop.id);
    if (!transport) continue;

    const segments = toSegments(transport);
    const from = pointOf(fromStop);
    const to = pointOf(toStop);
    const hasPolyline = segments.some((segment) => segment.polyline.length > 1);
    const geometry: ResultLeg["geometry"] = hasPolyline
      ? "polyline"
      : from.lat !== undefined && from.lng !== undefined && to.lat !== undefined && to.lng !== undefined
        ? "straight"
        : "none";

    legs.push({
      id: `${dayIndex}:${fromStop.id}>${toStop.id}`,
      dayIndex,
      from,
      to,
      mode: transport.mode,
      modeLabel: modeLabels[transport.mode] ?? "交通",
      durationMin: finite(transport.duration_min, 0),
      distanceM: finite(transport.distance_m, 0),
      costYuan: finite(transport.cost_yuan, 0),
      estimated: transport.estimated === true || segments.some((segment) => segment.estimated),
      segments,
      geometry,
    });
  }

  return legs;
}

function transportBetween(blocks: Block[], fromID: string, toID: string): Transport | null {
  const fromIndex = blocks.findIndex((block) => block.id === fromID);
  const toIndex = blocks.findIndex((block) => block.id === toID);
  if (fromIndex < 0 || toIndex < 0) return null;

  const fromBlock = blocks[fromIndex];
  if (fromBlock.transport) return fromBlock.transport;

  for (let index = fromIndex + 1; index < toIndex; index += 1) {
    const candidate = blocks[index];
    if (candidate.type === "transit" && candidate.transport) return candidate.transport;
  }

  return null;
}

function toSegments(transport: Transport): ResultLegSegment[] {
  const source: TransitLeg[] =
    transport.legs && transport.legs.length > 0
      ? transport.legs
      : [
          {
            mode: transport.mode,
            distance_m: transport.distance_m,
            duration_min: transport.duration_min,
            cost_yuan: transport.cost_yuan,
            estimated: transport.estimated,
          },
        ];

  return source.map((segment) => ({
    mode: segment.mode,
    modeLabel: modeLabels[segment.mode] ?? "交通",
    ...(clean(segment.line_name) ? { lineName: clean(segment.line_name) } : {}),
    ...(clean(segment.from_stop) ? { fromStop: clean(segment.from_stop) } : {}),
    ...(clean(segment.to_stop) ? { toStop: clean(segment.to_stop) } : {}),
    distanceM: finite(segment.distance_m, 0),
    durationMin: finite(segment.duration_min, 0),
    costYuan: finite(segment.cost_yuan, 0),
    estimated: segment.estimated === true,
    polyline: validPolyline(segment.polyline),
  }));
}

function validPolyline(raw: [number, number][] | undefined): [number, number][] {
  if (!Array.isArray(raw)) return [];
  const points: [number, number][] = [];
  for (const item of raw) {
    if (!Array.isArray(item) || item.length < 2) continue;
    const [lng, lat] = item;
    if (
      typeof lng !== "number" ||
      typeof lat !== "number" ||
      !Number.isFinite(lng) ||
      !Number.isFinite(lat)
    ) {
      continue;
    }
    points.push([lng, lat]);
  }
  return points;
}

function toGaps(day: DayPlan, stops: ResultStop[], dayIndex: number): ResultGap[] {
  const dayStart = parseHHMM(day.start_time);
  const dayEnd = parseHHMM(day.end_time);
  if (dayStart === null || dayEnd === null || dayEnd <= dayStart) return [];

  const timedStops: Array<{ start: number; end: number; startText: string; endText: string }> = [];
  for (const stop of stops) {
    const start = parseHHMM(stop.start);
    const end = parseHHMM(stop.end);
    if (start === null || end === null || end <= start) return [];
    timedStops.push({ start, end, startText: stop.start, endText: stop.end });
  }

  const gaps: ResultGap[] = [];
  let cursor = dayStart;
  let cursorText = day.start_time;

  for (const stop of timedStops) {
    if (stop.start > cursor) {
      pushGap(gaps, dayIndex, cursor, cursorText, stop.start, stop.startText);
    }
    if (stop.end > cursor) {
      cursor = stop.end;
      cursorText = stop.endText;
    }
  }

  if (dayEnd > cursor) {
    pushGap(gaps, dayIndex, cursor, cursorText, dayEnd, day.end_time);
  }

  return gaps;
}

function pushGap(
  gaps: ResultGap[],
  dayIndex: number,
  start: number,
  startText: string,
  end: number,
  endText: string,
): void {
  const minutes = end - start;
  if (minutes < GAP_MIN_MINUTES) return;
  gaps.push({
    id: `${dayIndex}:gap:${startText}`,
    dayIndex,
    start: startText,
    end: endText,
    minutes,
    suggestion: gapSuggestion(start, end, minutes),
  });
}

function gapSuggestion(start: number, end: number, minutes: number): string {
  if (start < 13 * 60 + 30 && end > 11 * 60 + 30) return "午饭";
  if (start < 19 * 60 + 30 && end > 17 * 60 + 30) return "晚饭";
  if (start >= 20 * 60) return "回住宿休息";
  if (minutes >= 120) return "自由活动";
  return "休息";
}

function parseHHMM(value: string): number | null {
  const match = /^(\d{1,2}):(\d{2})$/.exec(value ?? "");
  if (!match) return null;
  const hours = Number(match[1]);
  const minutes = Number(match[2]);
  if (hours > 23 || minutes > 59) return null;
  return hours * 60 + minutes;
}

function toReminders(research: ResearchBrief | null): ResultReminder[] {
  const seen = new Set<string>();
  const reminders: ResultReminder[] = [];
  const candidates = [...(research?.notes ?? []), ...(research?.warnings ?? [])];

  for (const candidate of candidates) {
    const text = clean(candidate);
    if (!text || seen.has(text)) continue;
    seen.add(text);
    reminders.push({ id: `research:${reminders.length}`, text });
  }

  return reminders;
}

function toLodging(itinerary: Itinerary): ResultLodging | null {
  const lodging = itinerary.lodging;
  if (!lodging) return null;
  const hotel = lodging.hotel;

  return {
    hotel: {
      name: clean(hotel.name) ?? "住宿",
      ...(clean(hotel.address) ? { address: clean(hotel.address) } : {}),
      ...(clean(hotel.city) ? { city: clean(hotel.city) } : {}),
      lat: finite(hotel.lat, 0),
      lng: finite(hotel.lng, 0),
      ...(typeof hotel.star === "number" && Number.isFinite(hotel.star)
        ? { star: hotel.star }
        : {}),
      score: typeof hotel.score === "number" && Number.isFinite(hotel.score) ? hotel.score : null,
      ...(safeURL(hotel.booking_url) ? { bookingURL: safeURL(hotel.booking_url) } : {}),
      ...(clean(hotel.provider) ? { provider: clean(hotel.provider) } : {}),
    },
    nights: finite(lodging.nights, 0),
    nightlyYuan: finite(lodging.nightly_yuan, 0),
    totalYuan: finite(lodging.total_yuan, 0),
    anchorDays: Array.isArray(lodging.anchor_days)
      ? lodging.anchor_days.filter((day) => Number.isInteger(day))
      : [],
    includeInBudget: lodging.include_in_budget === true,
    priceEstimated: lodging.price_estimated === true,
  };
}

function guidesFor(research: ResearchBrief | null, poiID: string): ResultGuide[] {
  const guides = research?.guides ?? [];
  return guides
    .filter((guide) => guide.poi_id === poiID)
    .map((guide) => ({
      poiID,
      ...(clean(guide.note) ? { note: clean(guide.note) } : {}),
      citations: (guide.citations ?? []).flatMap((citation) => {
        const title = clean(citation.title);
        const url = safeURL(citation.url);
        if (!title && !url) return [];
        return [
          {
            ...(title ? { title } : {}),
            ...(url ? { url } : {}),
            ...(clean(citation.site) ? { site: clean(citation.site) } : {}),
            publishedAt: citation.published_at ?? null,
            ...(typeof citation.authority === "number" && Number.isFinite(citation.authority)
              ? { authority: citation.authority }
              : {}),
          },
        ];
      }),
    }));
}

function pointOf(stop: ResultStop): ResultPoint {
  return {
    id: stop.id,
    title: stop.title,
    ...(stop.lat !== undefined && stop.lng !== undefined
      ? { lat: stop.lat, lng: stop.lng }
      : {}),
  };
}

function coordinates(lat: unknown, lng: unknown): { lat: number; lng: number } | Record<string, never> {
  if (
    typeof lat !== "number" ||
    typeof lng !== "number" ||
    !Number.isFinite(lat) ||
    !Number.isFinite(lng) ||
    (lat === 0 && lng === 0)
  ) {
    return {};
  }
  return { lat, lng };
}

function resolvePace(days: ResultDay[], walkingKM: number): ResultPace {
  if (days.length === 0) return "relaxed";
  const stops = days.reduce((sum, day) => sum + day.stops.length, 0);
  const avgStops = stops / days.length;
  const walkPerDay = walkingKM / days.length;

  if (avgStops >= 7 || walkPerDay >= 6) return "packed";
  if (avgStops <= 4 && walkPerDay < 3.5) return "relaxed";
  return "balanced";
}

function dayTheme(stops: ResultStop[], index: number): string {
  const titles = placeTitles(stops).slice(0, 3);
  return titles.length > 0 ? titles.join(" · ") : `第 ${index + 1} 天`;
}

function summarize(days: ResultDay[]): string {
  const seen = new Set<string>();
  const titles: string[] = [];

  for (const day of days) {
    for (const title of placeTitles(day.stops)) {
      if (seen.has(title)) continue;
      seen.add(title);
      titles.push(title);
      if (titles.length === 4) break;
    }
    if (titles.length === 4) break;
  }

  if (titles.length === 0) return `${days.length} 天行程`;
  const head = `${days.length} 天串起`;

  if (titles.length === 1) return `${head}${titles[0]}`;
  return `${head}${titles.slice(0, -1).join("、")}与${titles[titles.length - 1]}`;
}

function placeTitles(stops: ResultStop[]): string[] {
  const titles = stops
    .filter((stop) => stop.type === "poi")
    .map((stop) => clean(stop.title))
    .filter((title): title is string => Boolean(title));
  if (titles.length > 0) return titles;
  return stops
    .map((stop) => clean(stop.title))
    .filter((title): title is string => Boolean(title));
}

function cityLabel(city: string): string {
  const value = clean(city);
  if (!value) return "";
  return value === "beijing" ? "北京" : value;
}

function clean(value: unknown): string | undefined {
  if (typeof value !== "string") return undefined;
  const text = value.trim();
  return text === "" ? undefined : text;
}

function finite(value: unknown, fallback: number): number {
  return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}

function safeURL(value: unknown): string | undefined {
  if (typeof value !== "string") return undefined;
  const text = value.trim();
  if (!text) return undefined;
  try {
    const url = new URL(text);
    return url.protocol === "http:" || url.protocol === "https:" ? text : undefined;
  } catch {
    return undefined;
  }
}