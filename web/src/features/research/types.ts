export type ResearchStatus = "ok" | "degraded" | "skipped";

export interface ResearchHotel {
  id?: string;
  name?: string;
  address?: string;
  city?: string;
  lat?: number;
  lng?: number;
  star?: number;
  score?: number | null;
  price_per_night_yuan?: number;
  price_estimated?: boolean;
  booking_url?: string;
  provider?: string;
  fetched_at?: string;
}

export interface GuideCitation {
  title?: string;
  url?: string;
  site?: string;
  published_at?: string | null;
  authority?: number;
}

export interface GuideSignal {
  poi_id?: string;
  weight?: number;
  mentions?: number;
  note?: string;
  citations?: GuideCitation[];
}

export interface AccommodationBrief {
  mentioned?: boolean;
  specified?: string[];
  selected?: ResearchHotel;
  options?: ResearchHotel[];
  anchor_days?: number[];
  nights?: number;
  nightly_yuan?: number;
  total_yuan?: number;
  price_estimated?: boolean;
  include_in_budget?: boolean;
  reason?: string;
}

export interface ResearchBrief {
  status?: ResearchStatus;
  accommodation?: AccommodationBrief;
  guides?: GuideSignal[];
  notes?: string[];
  warnings?: string[];
}

export interface ResearchOption {
  id: string;
  label: string;
  description?: string;
  hotel?: ResearchHotel;
}

export interface ResearchQuestion {
  code: string;
  message: string;
  options: ResearchOption[];
}

const statuses = new Set<ResearchStatus>(["ok", "degraded", "skipped"]);

export function parseResearch(raw: unknown): ResearchBrief | null {
  const value = object(raw);
  if (!value) return null;
  const brief: ResearchBrief = {};
  if (typeof value.status === "string" && statuses.has(value.status as ResearchStatus)) {
    brief.status = value.status as ResearchStatus;
  }
  const accommodation = parseAccommodation(value.accommodation);
  if (accommodation) brief.accommodation = accommodation;
  const guides = parseGuides(value.guides);
  if (guides) brief.guides = guides;
  const notes = stringList(value.notes);
  if (notes) brief.notes = notes;
  const warnings = stringList(value.warnings);
  if (warnings) brief.warnings = warnings;
  return Object.keys(brief).length > 0 ? brief : null;
}

export function parseResearchQuestion(raw: unknown): ResearchQuestion | null {
  const value = object(raw);
  if (!value || typeof value.code !== "string" || !Array.isArray(value.options)) return null;
  const options = value.options.flatMap((item): ResearchOption[] => {
    const option = object(item);
    if (!option || typeof option.id !== "string" || typeof option.label !== "string") return [];
    return [
      {
        id: option.id,
        label: option.label,
        description: optionalString(option.description),
        hotel: parseHotel(option.hotel) ?? undefined,
      },
    ];
  });
  if (options.length === 0) return null;
  return {
    code: value.code,
    message: typeof value.message === "string" ? value.message : "",
    options,
  };
}

function parseAccommodation(raw: unknown): AccommodationBrief | null {
  const value = object(raw);
  if (!value) return null;
  const brief: AccommodationBrief = {
    mentioned: value.mentioned === true,
  };
  const specified = stringList(value.specified);
  if (specified) brief.specified = specified;
  const selected = parseHotel(value.selected);
  if (selected) brief.selected = selected;
  if (Array.isArray(value.options)) {
    const options = value.options.flatMap((item): ResearchHotel[] => {
      const hotel = parseHotel(item);
      return hotel ? [hotel] : [];
    });
    if (options.length > 0) brief.options = options;
  }
  if (Array.isArray(value.anchor_days)) {
    brief.anchor_days = value.anchor_days.filter((item): item is number => typeof item === "number");
  }
  brief.nights = number(value.nights);
  brief.nightly_yuan = number(value.nightly_yuan);
  brief.total_yuan = number(value.total_yuan);
  brief.price_estimated = value.price_estimated === true;
  brief.include_in_budget = value.include_in_budget === true;
  brief.reason = optionalString(value.reason);
  return brief;
}

function parseHotel(raw: unknown): ResearchHotel | null {
  const value = object(raw);
  if (!value) return null;
  const name = optionalString(value.name);
  if (!name && typeof value.id !== "string") return null;
  const hotel: ResearchHotel = {
    id: optionalString(value.id),
    name,
    address: optionalString(value.address),
    city: optionalString(value.city),
    lat: number(value.lat),
    lng: number(value.lng),
    star: number(value.star),
    score: typeof value.score === "number" ? value.score : null,
    price_per_night_yuan: number(value.price_per_night_yuan),
    price_estimated: value.price_estimated === true,
    booking_url: optionalString(value.booking_url),
    provider: optionalString(value.provider),
    fetched_at: optionalString(value.fetched_at),
  };
  return hotel;
}

function parseGuides(raw: unknown): GuideSignal[] | null {
  if (!Array.isArray(raw)) return null;
  const guides = raw.flatMap((item): GuideSignal[] => {
    const value = object(item);
    if (!value) return [];
    const citations = Array.isArray(value.citations)
      ? value.citations.flatMap((entry): GuideCitation[] => {
          const citation = object(entry);
          if (!citation) return [];
          return [
            {
              title: optionalString(citation.title),
              url: optionalString(citation.url),
              site: optionalString(citation.site),
              published_at: optionalString(citation.published_at) ?? null,
              authority: number(citation.authority),
            },
          ];
        })
      : undefined;
    return [
      {
        poi_id: optionalString(value.poi_id),
        weight: number(value.weight),
        mentions: number(value.mentions),
        note: optionalString(value.note),
        citations,
      },
    ];
  });
  return guides.length > 0 ? guides : null;
}

function stringList(raw: unknown): string[] | undefined {
  if (!Array.isArray(raw)) return undefined;
  return raw.filter((item): item is string => typeof item === "string");
}

function optionalString(raw: unknown): string | undefined {
  return typeof raw === "string" && raw.length > 0 ? raw : undefined;
}

function number(raw: unknown): number | undefined {
  return typeof raw === "number" && Number.isFinite(raw) ? raw : undefined;
}

function object(raw: unknown): Record<string, unknown> | null {
  return raw !== null && typeof raw === "object" && !Array.isArray(raw)
    ? (raw as Record<string, unknown>)
    : null;
}
