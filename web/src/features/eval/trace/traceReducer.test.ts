import { describe, expect, it } from "vitest";
import { describe as describeEvent, initialTraceState, stageLabels, traceReducer, type RunEvent, type TraceState } from "./traceReducer";

function apply(state: TraceState, event: RunEvent): TraceState {
  return traceReducer(state, { type: "event", event });
}

describe("traceReducer", () => {
  it("tracks stages, validation and repair rounds in order", () => {
    let state = initialTraceState();
    state = apply(state, { seq: 1, type: "run.started", payload: { input: "北京两天" } });
    expect(state.status).toBe("running");
    expect(state.stage).toBe("parse");

    state = apply(state, { seq: 2, type: "constraint.parsed", payload: { constraints: { city: "beijing", days: 2 } } });
    expect(state.stage).toBe("retrieve");

    state = apply(state, { seq: 3, type: "candidate.retrieved", payload: { count: 8, attempts: 1 } });
    expect(state.stage).toBe("draft");

    state = apply(state, { seq: 4, type: "plan.drafted", payload: { plan: {} } });
    expect(state.stage).toBe("schedule");

    state = apply(state, {
      seq: 5,
      type: "validation.result",
      payload: {
        blocking: 2,
        warnings: 1,
        violations: [
          { code: "C1", level: "blocking", day: 1, message: "超出预算" },
          { code: "C2", level: "warning", day: 0, message: "步行较多" },
        ],
      },
    });
    expect(state.stage).toBe("validate");
    expect(state.violations).toHaveLength(2);

    state = apply(state, { seq: 6, type: "repair.started", payload: { round: 1, blocking: 2 } });
    expect(state.stage).toBe("repair");
    expect(state.round).toBe(1);

    state = apply(state, { seq: 7, type: "repair.completed", payload: { round: 1 } });
    state = apply(state, { seq: 8, type: "validation.result", payload: { blocking: 0, warnings: 0, violations: [] } });
    expect(state.violations).toHaveLength(0);

    state = apply(state, { seq: 9, type: "plan.updated", payload: { seq: 1 } });
    expect(state.stage).toBe("present");
    expect(state.entries.map((entry) => entry.seq)).toEqual([1, 2, 3, 4, 5, 6, 7, 8, 9]);
  });

  it("tracks research events and stores the pending question", () => {
    let state = initialTraceState();
    state = apply(state, { seq: 1, type: "research.started", payload: { tools: ["search_hotels", "search_travel_notes"] } });
    expect(state.stage).toBe("research");
    expect(state.entries[0].detail).toBe("2 个工具");

    state = apply(state, {
      seq: 2,
      type: "plugin.tool_completed",
      payload: { name: "search_hotels", status: "ok", latency_ms: 120, result_count: 3 },
    });
    expect(state.entries[1].title).toBe("search_hotels 完成");
    expect(state.entries[1].detail).toBe("120ms · 3 条结果");

    const question = {
      code: "hotel_unconfirmed",
      message: "未找到指定酒店",
      options: [{ id: "continue_without_lodging", label: "继续但不计住宿" }],
    };
    state = apply(state, { seq: 3, type: "research.needs_input", payload: { question } });
    expect(state.researchQuestion?.code).toBe("hotel_unconfirmed");

    state = apply(state, {
      seq: 4,
      type: "run.awaiting_user",
      payload: { stage: "research", research_question: question },
    });
    expect(state.stage).toBe("research");
    expect(state.researchQuestion?.options[0].id).toBe("continue_without_lodging");

    state = traceReducer(state, { type: "reset", lastSeq: 4 });
    expect(state.researchQuestion).toBeNull();
  });

  it("starts adjustment runs at the adjust stage and clears stale cards", () => {
    let state = apply(initialTraceState(), {
      seq: 3,
      type: "run.awaiting_user",
      payload: { stage: "proposal", proposal: { summary: "旧提案" }, missing: ["days"] },
    });
    expect(state.proposal).not.toBeNull();

    state = apply(state, { seq: 4, type: "run.started", payload: { mode: "adjustment", input: "换个景点" } });

    expect(state.status).toBe("running");
    expect(state.stage).toBe("adjust");
    expect(state.proposal).toBeNull();
    expect(state.missing).toEqual([]);
    expect(state.error).toBeNull();

    state = apply(initialTraceState(), { seq: 1, type: "run.started", payload: { mode: "initial" } });
    expect(state.stage).toBe("parse");
  });

  it("labels a rejected proposal stage", () => {
    expect(stageLabels.proposal_rejected).toBe("已拒绝提案，待修改");
  });

  it("moves to awaiting and keeps the proposal", () => {
    const state = apply(initialTraceState(), {
      seq: 4,
      type: "run.awaiting_user",
      payload: { stage: "present", missing: ["days"], proposal: { scope: "day", days: [0] } },
    });

    expect(state.status).toBe("awaiting_user");
    expect(state.stage).toBe("present");
    expect(state.missing).toEqual(["days"]);
    expect(state.proposal).toEqual({ scope: "day", days: [0] });
  });

  it("ignores duplicate and out-of-order events", () => {
    let state = apply(initialTraceState(), { seq: 3, type: "run.started", payload: {} });
    const duplicate = apply(state, { seq: 3, type: "run.started", payload: {} });
    const stale = apply(state, { seq: 2, type: "plan.updated", payload: {} });

    expect(duplicate).toBe(state);
    expect(stale).toBe(state);
    expect(state.lastSeq).toBe(3);
  });

  it("describes skill loads without exposing the hash", () => {
    const event: RunEvent = {
      seq: 4,
      type: "research.skill_loaded",
      payload: { name: "research-playbook", version: "1.0.0", sha256: "SECRET-HASH" },
    };
    const entry = describeEvent(event);
    expect(entry.title).toBe("加载技能 research-playbook");
    expect(entry.detail).toBe("research-playbook@1.0.0");
    expect(entry.level).toBe("info");
    expect(JSON.stringify(entry)).not.toContain("SECRET-HASH");

    let state = apply(initialTraceState(), { seq: 1, type: "research.started", payload: { tools: [] } });
    state = apply(state, event);
    expect(state.stage).toBe("research");
    expect(state.entries.at(-1)?.title).toBe("加载技能 research-playbook");
  });

  it("records failures with the node code", () => {
    const state = apply(initialTraceState(), {
      seq: 2,
      type: "run.failed",
      payload: { code: "llm_failed", message: "模型不可用" },
    });

    expect(state.status).toBe("failed");
    expect(state.error).toEqual({ code: "llm_failed", message: "模型不可用" });
  });

  it("resets while preserving a replay cursor", () => {
    let state = apply(initialTraceState(), { seq: 5, type: "plan.updated", payload: {} });
    state = traceReducer(state, { type: "reset", lastSeq: 12 });

    expect(state.entries).toHaveLength(0);
    expect(state.lastSeq).toBe(12);
    expect(state.status).toBe("idle");
  });
});
