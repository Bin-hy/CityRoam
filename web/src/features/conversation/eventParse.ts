export interface RunEvent {
  seq: number;
  type: string;
  payload: Record<string, unknown>;
}

export function asRecord(raw: unknown): Record<string, unknown> | null {
  return raw !== null && typeof raw === "object" && !Array.isArray(raw)
    ? (raw as Record<string, unknown>)
    : null;
}

export function asString(raw: unknown, fallback = ""): string {
  return typeof raw === "string" ? raw : fallback;
}

export function asNumber(raw: unknown, fallback = 0): number {
  const value = typeof raw === "number" ? raw : Number(raw);
  return Number.isFinite(value) ? value : fallback;
}

export function asStringList(raw: unknown): string[] {
  if (!Array.isArray(raw)) return [];
  return raw.filter((item): item is string => typeof item === "string");
}

export function normalizeEvents(raw: unknown): RunEvent[] {
  if (!Array.isArray(raw)) return [];
  const seen = new Set<number>();
  const events: RunEvent[] = [];
  for (const item of raw) {
    const value = asRecord(item);
    if (!value) continue;
    const seq = asNumber(value.seq, Number.NaN);
    if (!Number.isFinite(seq) || seq <= 0 || seen.has(seq)) continue;
    seen.add(seq);
    events.push({
      seq: Math.trunc(seq),
      type: asString(value.type),
      payload: asRecord(value.payload) ?? {},
    });
  }
  return events.sort((a, b) => a.seq - b.seq);
}

export function terminalEvent(event: Pick<RunEvent, "type">): boolean {
  return event.type === "run.completed" || event.type === "run.failed" || event.type === "run.cancelled";
}
