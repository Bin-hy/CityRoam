import { apiRequest } from "./client";
import type { EvalCase, EvalRun } from "./types";

function adminHeaders(token: string): Record<string, string> {
  return { "X-Admin-Token": token };
}

export function listEvalCases(token: string): Promise<{ cases: EvalCase[] }> {
  return apiRequest<{ cases: EvalCase[] }>("/api/eval/cases", { headers: adminHeaders(token) });
}

export function listEvalRuns(token: string): Promise<{ runs: EvalRun[] }> {
  return apiRequest<{ runs: EvalRun[] }>("/api/eval/runs", { headers: adminHeaders(token) });
}

export function startEval(token: string): Promise<{ status: string }> {
  return apiRequest<{ status: string }>("/api/eval/runs", { method: "POST", headers: adminHeaders(token) });
}
