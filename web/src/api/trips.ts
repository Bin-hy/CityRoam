import { apiRequest } from "./client";
import type {
  AdjustmentRequest,
  CreatedTrip,
  CreateTripRequest,
  PlanVersion,
  ReplanRequest,
  ResumeRequest,
  RunAction,
  Share,
  SharedTrip,
  Trip,
  TripList,
  VersionDiff,
} from "./types";

export function listTrips(limit = 20): Promise<TripList> {
  return apiRequest<TripList>(`/api/trips?limit=${limit}`);
}

export function createTrip(body: CreateTripRequest): Promise<CreatedTrip> {
  return apiRequest<CreatedTrip>("/api/trips", { method: "POST", body });
}

export function getTrip(tripID: string): Promise<Trip> {
  return apiRequest<Trip>(`/api/trips/${encodeURIComponent(tripID)}`);
}

export function deleteTrip(tripID: string): Promise<void> {
  return apiRequest<void>(`/api/trips/${encodeURIComponent(tripID)}`, { method: "DELETE" });
}

export function listPlanVersions(tripID: string): Promise<{ versions: PlanVersion[] }> {
  return apiRequest<{ versions: PlanVersion[] }>(`/api/trips/${encodeURIComponent(tripID)}/versions`);
}

export function getPlanVersion(tripID: string, seq: number): Promise<PlanVersion> {
  return apiRequest<PlanVersion>(`/api/trips/${encodeURIComponent(tripID)}/versions/${seq}`);
}

export function diffPlanVersions(tripID: string, from?: number, to?: number): Promise<VersionDiff> {
  const query = new URLSearchParams();
  if (from !== undefined) query.set("from", String(from));
  if (to !== undefined) query.set("to", String(to));
  const suffix = query.size > 0 ? `?${query.toString()}` : "";
  return apiRequest<VersionDiff>(`/api/trips/${encodeURIComponent(tripID)}/diff${suffix}`);
}

export function restorePlanVersion(tripID: string, seq: number): Promise<PlanVersion> {
  return apiRequest<PlanVersion>(`/api/trips/${encodeURIComponent(tripID)}/versions/${seq}/restore`, {
    method: "POST",
  });
}

export function createAdjustment(tripID: string, body: AdjustmentRequest): Promise<RunAction> {
  return apiRequest<RunAction>(`/api/trips/${encodeURIComponent(tripID)}/adjustments`, {
    method: "POST",
    body,
  });
}

export function replanTrip(tripID: string, body: ReplanRequest): Promise<RunAction> {
  return apiRequest<RunAction>(`/api/trips/${encodeURIComponent(tripID)}/replan`, {
    method: "POST",
    body,
  });
}

export function createShare(tripID: string): Promise<Share> {
  return apiRequest<Share>(`/api/trips/${encodeURIComponent(tripID)}/share`, { method: "POST" });
}

export function getSharedTrip(slug: string): Promise<SharedTrip> {
  return apiRequest<SharedTrip>(`/api/share/${encodeURIComponent(slug)}`);
}

export function resumeRun(runID: string, body: ResumeRequest): Promise<RunAction> {
  return apiRequest<RunAction>(`/api/runs/${encodeURIComponent(runID)}/resume`, { method: "POST", body });
}

export function cancelRun(runID: string): Promise<RunAction> {
  return apiRequest<RunAction>(`/api/runs/${encodeURIComponent(runID)}/cancel`, { method: "POST" });
}
