import { describe, expect, it } from "vitest";
import type { EvalCase, EvalRun } from "@/api/types";
import { aggregateMetrics, caseResults, latestRunsByCase } from "./evalModel";

function testCase(id: string, enabled = true): EvalCase {
  return { id, input: `${id} 输入`, expected: {}, tags: ["1d"], enabled };
}

function run(overrides: Partial<EvalRun>): EvalRun {
  return {
    id: "run-1",
    case_id: "case-a",
    run_id: "agent-run-1",
    metrics: {},
    passed: true,
    duration_ms: 900,
    created_at: "2026-11-19T10:00:00Z",
    ...overrides,
  };
}

describe("evalModel", () => {
  it("keeps the newest run per case and maps it to cases", () => {
    const cases = [testCase("case-a"), testCase("case-b")];
    const runs = [
      run({ id: "old", case_id: "case-a", passed: false, created_at: "2026-11-18T10:00:00Z" }),
      run({ id: "new", case_id: "case-a", passed: true, created_at: "2026-11-19T10:00:00Z" }),
      run({ id: "other", case_id: "case-b", passed: true, created_at: "2026-11-19T11:00:00Z" }),
    ];

    const latest = latestRunsByCase(runs);
    expect(latest.get("case-a")?.id).toBe("new");

    const rows = caseResults(cases, runs);
    expect(rows.map((row) => [row.testCase.id, row.latest?.id ?? null])).toEqual([
      ["case-a", "new"],
      ["case-b", "other"],
    ]);
    expect(caseResults(cases, []).map((row) => row.latest)).toEqual([null, null]);
  });

  it("aggregates required metrics across runs", () => {
    const runs = [
      run({
        id: "r1",
        case_id: "case-a",
        metrics: { hard_pass: 0.5, rounds: 1, poi_valid: 0.9, json_valid: 1, latency_ms: 900 },
      }),
      run({
        id: "r2",
        case_id: "case-b",
        metrics: { hard_pass: 1, repair_rounds: 3, poi_valid_rate: 1, json_valid_rate: 1, latency_ms: 1100 },
      }),
    ];

    expect(aggregateMetrics(runs)).toEqual([
      { key: "hard", label: "硬约束通过率", value: "75%", samples: 2 },
      { key: "rounds", label: "平均修复轮数", value: "2", samples: 2 },
      { key: "latency", label: "平均延迟", value: "1000 ms", samples: 2 },
      { key: "poi", label: "POI 有效率", value: "95%", samples: 2 },
      { key: "json", label: "JSON 合法率", value: "100%", samples: 2 },
    ]);
  });

  it("falls back to duration and shows dashes for missing metrics", () => {
    const runs = [run({ metrics: { hard_pass: 1 }, duration_ms: 1500 })];
    const metrics = aggregateMetrics(runs);
    expect(metrics.find((metric) => metric.key === "latency")).toMatchObject({ value: "1500 ms", samples: 1 });
    expect(metrics.find((metric) => metric.key === "poi")).toEqual({
      key: "poi",
      label: "POI 有效率",
      value: "—",
      samples: 0,
    });

    expect(aggregateMetrics([]).every((metric) => metric.value === "—" && metric.samples === 0)).toBe(true);
  });
});
