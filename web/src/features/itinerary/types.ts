export interface TransitLeg {
  mode: string;
  line_name?: string;
  from_stop?: string;
  to_stop?: string;
  distance_m: number;
  duration_min: number;
  cost_yuan: number;
  polyline?: [number, number][];
  estimated?: boolean;
}

export interface Transport {
  mode: string;
  from_poi_id: string;
  to_poi_id: string;
  distance_m: number;
  duration_min: number;
  cost_yuan: number;
  estimated: boolean;
  legs?: TransitLeg[];
}

export interface Block {
  id: string;
  type: "poi" | "meal" | "transit" | "rest" | string;
  poi_id?: string;
  lat?: number;
  lng?: number;
  title: string;
  start: string;
  end: string;
  duration_min: number;
  cost_yuan: number;
  transport?: Transport;
  notes?: string;
}

export interface DayPlan {
  date: string;
  start_time: string;
  end_time: string;
  blocks: Block[];
  day_cost_yuan: number;
}

export interface Totals {
  cost_yuan: number;
  poi_count: number;
  walking_km: number;
  lodging_yuan?: number;
}

export interface ItineraryHotel {
  id?: string;
  name: string;
  address?: string;
  city?: string;
  lat: number;
  lng: number;
  star?: number;
  score?: number | null;
  price_per_night_yuan: number;
  price_estimated?: boolean;
  booking_url?: string;
  provider?: string;
}

export interface LodgingSummary {
  hotel: ItineraryHotel;
  nights: number;
  nightly_yuan: number;
  total_yuan: number;
  anchor_days?: number[];
  include_in_budget: boolean;
  price_estimated: boolean;
  source: string;
}

export interface Explanation {
  poi_id: string;
  text: string;
}

export interface Itinerary {
  city: string;
  days: DayPlan[];
  lodging?: LodgingSummary;
  explanations: Explanation[];
  totals: Totals;
}

export function parseItinerary(raw: unknown): Itinerary | null {
  if (raw === null || typeof raw !== "object" || Array.isArray(raw)) return null;
  const value = raw as Partial<Itinerary>;
  if (!Array.isArray(value.days)) return null;
  const lodging = parseLodging(value.lodging);
  return {
    city: typeof value.city === "string" ? value.city : "",
    days: value.days.map((day) => ({
      date: typeof day?.date === "string" ? day.date : "",
      start_time: typeof day?.start_time === "string" ? day.start_time : "",
      end_time: typeof day?.end_time === "string" ? day.end_time : "",
      blocks: Array.isArray(day?.blocks) ? parseBlocks(day.blocks) : [],
      day_cost_yuan: typeof day?.day_cost_yuan === "number" ? day.day_cost_yuan : 0,
    })),
    ...(lodging ? { lodging } : {}),
    explanations: parseExplanations(value.explanations),
    totals: {
      cost_yuan: typeof value.totals?.cost_yuan === "number" ? value.totals.cost_yuan : 0,
      poi_count: typeof value.totals?.poi_count === "number" ? value.totals.poi_count : 0,
      walking_km: typeof value.totals?.walking_km === "number" ? value.totals.walking_km : 0,
      lodging_yuan: typeof value.totals?.lodging_yuan === "number" ? value.totals.lodging_yuan : 0,
    },
  };
}

export function explanationMap(itinerary: Itinerary | null): Map<string, string> {
  const map = new Map<string, string>();
  if (!itinerary) return map;
  for (const explanation of itinerary.explanations) {
    map.set(explanation.poi_id, explanation.text);
  }
  return map;
}

function parseExplanations(raw: unknown): Explanation[] {
  if (!Array.isArray(raw)) return [];
  const byPoi = new Map<string, string>();
  for (const item of raw) {
    if (item === null || typeof item !== "object" || Array.isArray(item)) continue;
    const { poi_id: poiId, text } = item as { poi_id?: unknown; text?: unknown };
    if (typeof poiId !== "string" || poiId.trim() === "") continue;
    if (typeof text !== "string") continue;
    const trimmed = text.trim();
    if (trimmed === "") continue;
    byPoi.set(poiId, trimmed);
  }
  return Array.from(byPoi, ([poi_id, text]) => ({ poi_id, text }));
}

function parseBlocks(raw: unknown[]): Block[] {
  const blocks: Block[] = [];
  for (const item of raw) {
    if (item === null || typeof item !== "object" || Array.isArray(item)) continue;
    const block = { ...(item as Block) };
    const transport = parseTransport((item as { transport?: unknown }).transport);
    if (transport) block.transport = transport;
    blocks.push(block);
  }
  return blocks;
}

function parseTransport(raw: unknown): Transport | undefined {
  if (raw === null || typeof raw !== "object" || Array.isArray(raw)) return undefined;
  const value = raw as Transport;
  const transport: Transport = {
    ...value,
    mode: typeof value.mode === "string" ? value.mode : "",
    from_poi_id: typeof value.from_poi_id === "string" ? value.from_poi_id : "",
    to_poi_id: typeof value.to_poi_id === "string" ? value.to_poi_id : "",
    distance_m: numberOr(value.distance_m, 0),
    duration_min: numberOr(value.duration_min, 0),
    cost_yuan: numberOr(value.cost_yuan, 0),
    estimated: value.estimated === true,
  };
  const legs = parseLegs((raw as { legs?: unknown }).legs);
  if (legs) transport.legs = legs;
  else delete transport.legs;
  return transport;
}

function parseLegs(raw: unknown): TransitLeg[] | undefined {
  if (!Array.isArray(raw)) return undefined;
  const legs: TransitLeg[] = [];
  for (const item of raw) {
    if (item === null || typeof item !== "object" || Array.isArray(item)) continue;
    const value = item as Partial<TransitLeg>;
    if (typeof value.mode !== "string" || value.mode === "") continue;
    const leg: TransitLeg = {
      mode: value.mode,
      distance_m: numberOr(value.distance_m, 0),
      duration_min: numberOr(value.duration_min, 0),
      cost_yuan: numberOr(value.cost_yuan, 0),
    };
    if (typeof value.line_name === "string") leg.line_name = value.line_name;
    if (typeof value.from_stop === "string") leg.from_stop = value.from_stop;
    if (typeof value.to_stop === "string") leg.to_stop = value.to_stop;
    if (value.estimated === true) leg.estimated = true;
    const polyline = parsePolyline(value.polyline);
    if (polyline) leg.polyline = polyline;
    legs.push(leg);
  }
  return legs;
}

function parsePolyline(raw: unknown): [number, number][] | undefined {
  if (!Array.isArray(raw)) return undefined;
  const points: [number, number][] = [];
  for (const item of raw) {
    if (!Array.isArray(item) || item.length < 2) continue;
    const [lng, lat] = item;
    if (typeof lng !== "number" || typeof lat !== "number" || !Number.isFinite(lng) || !Number.isFinite(lat)) continue;
    points.push([lng, lat]);
  }
  return points.length > 0 ? points : undefined;
}

function parseLodging(raw: unknown): LodgingSummary | undefined {
  if (raw === null || typeof raw !== "object" || Array.isArray(raw)) return undefined;
  const value = raw as Partial<LodgingSummary>;
  const hotelRaw = value.hotel;
  const hotel: ItineraryHotel =
    hotelRaw !== null && typeof hotelRaw === "object" && !Array.isArray(hotelRaw)
      ? {
          ...hotelRaw,
          name: typeof hotelRaw.name === "string" ? hotelRaw.name : "",
          lat: numberOr(hotelRaw.lat, 0),
          lng: numberOr(hotelRaw.lng, 0),
          price_per_night_yuan: numberOr(hotelRaw.price_per_night_yuan, 0),
        }
      : { name: "", lat: 0, lng: 0, price_per_night_yuan: 0 };
  const anchorDays = Array.isArray(value.anchor_days)
    ? value.anchor_days.filter((day): day is number => Number.isInteger(day))
    : undefined;
  return {
    hotel,
    nights: numberOr(value.nights, 0),
    nightly_yuan: numberOr(value.nightly_yuan, 0),
    total_yuan: numberOr(value.total_yuan, 0),
    ...(anchorDays ? { anchor_days: anchorDays } : {}),
    include_in_budget: value.include_in_budget === true,
    price_estimated: value.price_estimated === true,
    source: typeof value.source === "string" ? value.source : "",
  };
}

function numberOr(raw: unknown, fallback: number): number {
  return typeof raw === "number" && Number.isFinite(raw) ? raw : fallback;
}

export function dayDateLabel(date: string, index: number): string {
  if (!date) return `第 ${index + 1} 天`;
  const parsed = new Date(`${date}T00:00:00`);
  if (Number.isNaN(parsed.getTime())) return date;
  return `${parsed.getMonth() + 1} 月 ${parsed.getDate()} 日`;
}
