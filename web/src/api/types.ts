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
export type RunAction = Schema<"RunAction">;
export type Share = Schema<"Share">;
export type SharedTrip = Schema<"SharedTrip">;
export type EvalCase = Schema<"EvalCase">;
export type EvalRun = Schema<"EvalRun">;
