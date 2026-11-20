import type { PlanVersion, Trip } from "@/api/types";
import { explanationMap, parseItinerary, type Itinerary } from "@/features/itinerary/types";
import { parseResearch, type ResearchBrief } from "@/features/research/types";
import { asNumber, normalizeEvents } from "./eventParse";

export interface CanvasModel {
  plan: Itinerary | null;
  explanations: Map<string, string>;
  research: ResearchBrief | null;
  isDraft: boolean;
  versionSeq: number | null;
  source: "event" | "version";
}

export interface CanvasResult {
  model: CanvasModel;
  poiNameMap: Map<string, string>;
}

export function deriveCanvas(
  rawEvents: unknown,
  trip: Trip | undefined,
  viewVersion?: PlanVersion | null,
): CanvasResult {
  const research = parseResearch(trip?.latest_version?.research);
  if (viewVersion) {
    const plan = parseItinerary(viewVersion.plan);
    return result(plan, research, false, viewVersion.seq, "version");
  }

  const events = normalizeEvents(rawEvents);
  let lastStart = -1;
  for (let index = 0; index < events.length; index += 1) {
    if (events[index].type === "run.started") lastStart = index;
  }

  let eventPlan: Itinerary | null = null;
  let eventVersionSeq: number | null = null;
  let eventPlanSeq = 0;
  for (let index = Math.max(0, lastStart); index < events.length; index += 1) {
    const event = events[index];
    if (event.type !== "plan.updated") continue;
    const plan = parseItinerary(event.payload.plan);
    if (!plan) continue;
    eventPlan = plan;
    eventVersionSeq = Math.trunc(asNumber(event.payload.seq, 0)) || null;
    eventPlanSeq = event.seq;
  }

  if (eventPlan) {
    if (
      trip?.latest_version &&
      eventVersionSeq !== null &&
      trip.latest_version.seq > eventVersionSeq
    ) {
      return result(parseItinerary(trip.latest_version.plan), research, false, trip.latest_version.seq, "version");
    }
    const completedAfter = events.some(
      (event) => event.seq > eventPlanSeq && event.type === "run.completed",
    );
    return result(eventPlan, research, !completedAfter, eventVersionSeq, "event");
  }

  if (trip?.latest_version) {
    return result(parseItinerary(trip.latest_version.plan), research, false, trip.latest_version.seq, "version");
  }

  return result(null, research, false, null, "version");
}

function result(
  plan: Itinerary | null,
  research: ResearchBrief | null,
  isDraft: boolean,
  versionSeq: number | null,
  source: "event" | "version",
): CanvasResult {
  const poiNameMap = new Map<string, string>();
  if (plan) {
    for (const day of plan.days) {
      for (const block of day.blocks) {
        if (block.poi_id) poiNameMap.set(block.poi_id, block.title);
      }
    }
  }
  return {
    model: {
      plan,
      explanations: explanationMap(plan),
      research,
      isDraft,
      versionSeq,
      source,
    },
    poiNameMap,
  };
}
