import type { EvalCase, EvalRun } from "@/api/types";

export interface CaseResult {
  testCase: EvalCase;
  latest: EvalRun | null;
}

export interface MetricSummary {
  key: string;
  label: string;
  value: string;
  samples: number;
}

function numericValue(metrics: Record<string, unknown>, keys: string[]): number | null {
  for (const key of keys) {
    const value = metrics[key];
    if (typeof value === "number" && Number.isFinite(value)) return value;
    if (typeof value === "string" && value.trim() !== "" && Number.isFinite(Number(value))) return Number(value);
  }
  return null;
}

function percentage(value: number): number {
  return value <= 1 ? value * 100 : value;
}

export function latestRunsByCase(runs: EvalRun[]): Map<string, EvalRun> {
  const latest = new Map<string, EvalRun>();
  runs.forEach((run) => {
    const current = latest.get(run.case_id);
    if (!current || Date.parse(run.created_at) >= Date.parse(current.created_at)) {
      latest.set(run.case_id, run);
    }
  });
  return latest;
}

export function caseResults(cases: EvalCase[], runs: EvalRun[]): CaseResult[] {
  const latest = latestRunsByCase(runs);
  return cases.map((testCase) => ({ testCase, latest: latest.get(testCase.id) ?? null }));
}

export function aggregateMetrics(runs: EvalRun[]): MetricSummary[] {
  const specs = [
    {
      key: "hard",
      label: "硬约束通过率",
      keys: ["hard_pass", "hard_constraint_pass"],
      format: "percent" as const,
    },
    {
      key: "rounds",
      label: "平均修复轮数",
      keys: ["rounds", "repair_rounds"],
      format: "number" as const,
    },
    {
      key: "latency",
      label: "平均延迟",
      keys: ["latency_ms"],
      format: "ms" as const,
    },
    {
      key: "poi",
      label: "POI 有效率",
      keys: ["poi_valid", "poi_valid_rate"],
      format: "percent" as const,
    },
    {
      key: "json",
      label: "JSON 合法率",
      keys: ["json_valid", "json_valid_rate"],
      format: "percent" as const,
    },
  ];

  return specs.map((spec) => {
    const values: number[] = [];
    runs.forEach((run) => {
      const metrics = (run.metrics ?? {}) as Record<string, unknown>;
      const raw = numericValue(metrics, spec.keys);
      if (raw !== null) values.push(raw);
      else if (spec.key === "latency" && typeof run.duration_ms === "number") values.push(run.duration_ms);
    });
    if (values.length === 0) {
      return { key: spec.key, label: spec.label, value: "—", samples: 0 };
    }
    const average = values.reduce((sum, value) => sum + value, 0) / values.length;
    let value: string;
    if (spec.format === "percent") value = `${Math.round(percentage(average))}%`;
    else if (spec.format === "ms") value = `${Math.round(average)} ms`;
    else value = Number.isInteger(average) ? String(average) : average.toFixed(1);
    return { key: spec.key, label: spec.label, value, samples: values.length };
  });
}
