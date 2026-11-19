export interface Transport {
  mode: string;
  from_poi_id: string;
  to_poi_id: string;
  distance_m: number;
  duration_min: number;
  cost_yuan: number;
  estimated: boolean;
}

export interface Block {
  id: string;
  type: "poi" | "meal" | "transit" | "rest" | string;
  poi_id?: string;
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
}

export interface Itinerary {
  city: string;
  days: DayPlan[];
  totals: Totals;
}

export function parseItinerary(raw: unknown): Itinerary | null {
  if (raw === null || typeof raw !== "object" || Array.isArray(raw)) return null;
  const value = raw as Partial<Itinerary>;
  if (!Array.isArray(value.days)) return null;
  return {
    city: typeof value.city === "string" ? value.city : "",
    days: value.days.map((day) => ({
      date: typeof day?.date === "string" ? day.date : "",
      start_time: typeof day?.start_time === "string" ? day.start_time : "",
      end_time: typeof day?.end_time === "string" ? day.end_time : "",
      blocks: Array.isArray(day?.blocks) ? day.blocks : [],
      day_cost_yuan: typeof day?.day_cost_yuan === "number" ? day.day_cost_yuan : 0,
    })),
    totals: {
      cost_yuan: typeof value.totals?.cost_yuan === "number" ? value.totals.cost_yuan : 0,
      poi_count: typeof value.totals?.poi_count === "number" ? value.totals.poi_count : 0,
      walking_km: typeof value.totals?.walking_km === "number" ? value.totals.walking_km : 0,
    },
  };
}

export function dayDateLabel(date: string, index: number): string {
  if (!date) return `第 ${index + 1} 天`;
  const parsed = new Date(`${date}T00:00:00`);
  if (Number.isNaN(parsed.getTime())) return date;
  return `${parsed.getMonth() + 1} 月 ${parsed.getDate()} 日`;
}
