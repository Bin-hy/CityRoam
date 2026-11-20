import type { components } from "./schema";

export type { components, operations, paths } from "./schema";

export type Schema<K extends keyof components["schemas"]> = components["schemas"][K];

export type Health = Schema<"Health">;
export type Meta = Schema<"Meta">;
export type CreateTripRequest = Schema<"CreateTripRequest">;
export type CreatedTrip = Schema<"CreatedTrip">;
export type Trip = Schema<"Trip">;
export type PlanVersion = Schema<"PlanVersion">;
export type VersionDiff = Schema<"VersionDiff">;
export type AdjustmentRequest = Schema<"AdjustmentRequest">;
export type ResumeRequest = Schema<"ResumeRequest">;
export type ReplanRequest = Schema<"ReplanRequest">;
export type ResearchChoice = Schema<"ResearchChoice">;
export type ResearchQuestion = Schema<"ResearchQuestion">;
export type ResearchOption = Schema<"ResearchOption">;
export type PluginStatus = Schema<"PluginStatus">;
export type ResearchBrief = Omit<Schema<"ResearchBrief">, "tool_calls" | "duration_ms">;
export type AccommodationBrief = Schema<"AccommodationBrief">;
export type Hotel = Schema<"Hotel">;
export type GuideSignal = Schema<"GuideSignal">;
export type GuideCitation = Schema<"GuideCitation">;
export type RunAction = Schema<"RunAction">;
export type Share = Schema<"Share">;
export type SharedTrip = Schema<"SharedTrip">;
export type EvalCase = Schema<"EvalCase">;
export type EvalRun = Schema<"EvalRun">;
