import { describe, expect, it } from "vitest";
import type { PlanVersion, Trip } from "@/api/types";
import type { RunEvent } from "./eventParse";
import { normalizeEvents } from "./eventParse";
import { deriveRunView } from "./deriveRunView";
import { deriveConversation, type AgentMessage, type DecisionMessage, type PlanMessage, type ToolMessage, type UserMessage } from "./deriveConversation";
import { deriveCanvas } from "./deriveCanvas";
import { explanationMap, type Itinerary } from "@/features/itinerary/types";

const plan: Itinerary = {
  city: "杭州",
  days: [
    {
      date: "2026-11-21",
      start_time: "09:00",
      end_time: "18:00",
      day_cost_yuan: 260,
      blocks: [
        { id: "b1", type: "poi", poi_id: "poi-0", title: "西湖", start: "09:00", end: "11:00", duration_min: 120, cost_yuan: 0 },
        { id: "b2", type: "meal", title: "午餐", start: "12:00", end: "13:00", duration_min: 60, cost_yuan: 60 },
      ],
    },
    {
      date: "2026-11-22",
      start_time: "09:30",
      end_time: "17:00",
      day_cost_yuan: 200,
      blocks: [
        { id: "b3", type: "poi", poi_id: "poi-1", title: "灵隐寺", start: "09:30", end: "12:00", duration_min: 150, cost_yuan: 200 },
      ],
    },
  ],
  explanations: [
    { poi_id: "poi-0", text: "上午人少。" },
    { poi_id: "poi-0", text: "上午人少，光线也好。" },
  ],
  totals: { cost_yuan: 460, poi_count: 2, walking_km: 3.4 },
};

const constraints = {
  city: "杭州",
  days: 2,
  budget: { total_yuan: 2000 },
  pace: "normal",
  interests: ["亲子"],
  must_visit: ["西湖"],
  avoid: [],
};

function ev(seq: number, type: string, payload: Record<string, unknown> = {}): RunEvent {
  return { seq, type, payload };
}

function happyEvents(): RunEvent[] {
  return [
    ev(1, "run.started", { mode: "initial", input: "杭州 2 天，预算 2000，带孩子" }),
    ev(2, "constraint.parsed", { constraints }),
    ev(3, "candidate.retrieved", { count: 12, attempts: 1 }),
    ev(4, "research.started", { tools: ["search_travel_notes", "get_weather"] }),
    ev(5, "plugin.tool_called", {
      name: "get_weather",
      provider: "amap",
      call_index: 1,
      query: "杭州 · 11/21 起 · 2 天",
    }),
    ev(6, "plugin.tool_completed", {
      name: "get_weather",
      provider: "amap",
      call_index: 1,
      status: "ok",
      latency_ms: 120,
      result_count: 2,
      preview: [
        { title: "11/21", subtitle: "晴 12–20°C" },
        { title: "11/22", subtitle: "多云 10–18°C" },
        { title: "11/23", subtitle: "小雨 9–15°C" },
        { title: "11/24", subtitle: "不应出现" },
      ],
    }),
    ev(7, "research.completed", { status: "ok", hotel: "全季西湖店", guide_matches: 4, weather_days: 2 }),
    ev(8, "plan.drafted"),
    ev(9, "validation.result", { blocking: 0, warnings: 1, checked: 12, violations: [] }),
    ev(10, "plan.updated", { seq: 1, plan, violations: [] }),
    ev(11, "run.awaiting_user", { stage: "present", seq: 1, missing: [] }),
    ev(12, "run.completed", { seq: 1 }),
  ];
}

function agentMessages(items: ReturnType<typeof deriveConversation>): AgentMessage[] {
  return items.filter((item): item is AgentMessage => item.kind === "agent");
}

function tripWithVersion(versionPlan: unknown = plan, research: unknown = null): Trip {
  return {
    id: "trip-1",
    city: "杭州",
    days: 2,
    status: "completed",
    constraints: {},
    created_at: "2026-11-20T00:00:00Z",
    updated_at: "2026-11-20T00:00:00Z",
    latest_version: {
      id: "v1",
      trip_id: "trip-1",
      run_id: "run-1",
      seq: 1,
      plan: versionPlan as Record<string, unknown>,
      constraints: {},
      status: "accepted",
      created_at: "2026-11-20T00:00:00Z",
      research: research as PlanVersion["research"],
    },
  };
}

describe("normalizeEvents", () => {
  it("sorts, dedupes and drops malformed entries", () => {
    const events = normalizeEvents([
      { seq: 2, type: "b", payload: {} },
      { seq: 1, type: "a", payload: {} },
      { seq: 2, type: "dup", payload: {} },
      { seq: 0, type: "bad", payload: {} },
      { seq: "x", type: "bad" },
      "nope",
      { seq: 3, type: "c" },
    ]);
    expect(events.map((event) => [event.seq, event.type])).toEqual([
      [1, "a"],
      [2, "b"],
      [3, "c"],
    ]);
    expect(events[2].payload).toEqual({});
  });
});

describe("deriveConversation basics", () => {
  it("builds one user message, stable stage slots and a settled plan", () => {
    const items = deriveConversation("run-1", happyEvents());
    const users = items.filter((item): item is UserMessage => item.kind === "user");
    expect(users).toHaveLength(1);
    expect(users[0].variant).toBe("initial");
    expect(users[0].constraints).toMatchObject({ city: "杭州", days: 2, budgetYuan: 2000, pace: "适中" });

    const agents = agentMessages(items);
    expect(agents.map((message) => message.stage)).toEqual([
      "understand",
      "draft",
      "research",
      "validate",
      "complete",
    ]);
    expect(agents[0].text).toContain("杭州");
    expect(agents[2].text).toContain("4 篇攻略");
    expect(agents[3].text).toContain("12 项校验");
    expect(agents.every((message) => !message.text.includes("#"))).toBe(true);

    const plans = items.filter((item): item is PlanMessage => item.kind === "plan");
    expect(plans).toHaveLength(1);
    expect(plans[0]).toMatchObject({
      isDraft: false,
      versionSeq: 1,
      stats: { days: 2, pois: 2, totalYuan: 460, walkingKm: 3.4 },
    });

    const decisions = items.filter((item): item is DecisionMessage => item.kind === "decision");
    expect(decisions).toHaveLength(1);
    expect(decisions[0]).toMatchObject({ payload: { decision: "confirm" }, state: "resolved", summary: "已接受。" });
  });

  it("yields the same items for shuffled or duplicated input", () => {
    const events = happyEvents();
    const duplicated = [...events, ev(6, "run.started", { input: "重复" })];
    const shuffled = [...events].reverse();
    expect(deriveConversation("run-1", duplicated)).toEqual(deriveConversation("run-1", events));
    expect(deriveConversation("run-1", shuffled)).toEqual(deriveConversation("run-1", events));
  });

  it("exposes run status, round and checked count", () => {
    const view = deriveRunView(happyEvents());
    expect(view.status).toBe("completed");
    expect(view.attempt).toBe(1);
    expect(view.checked).toBe(12);
    expect(view.draft).toBe(false);
  });
});

describe("tool cards", () => {
  it("folds called into completed and caps previews", () => {
    const items = deriveConversation("run-1", happyEvents());
    const tools = items.filter((item): item is ToolMessage => item.kind === "tool");
    expect(tools).toHaveLength(1);
    expect(tools[0]).toMatchObject({
      callKey: "get_weather:1",
      status: "ok",
      latencyMs: 120,
      resultCount: 2,
      query: "杭州 · 11/21 起 · 2 天",
    });
    expect(tools[0].preview).toHaveLength(3);
  });

  it("keeps failures, cache and coalesced markers", () => {
    const events = [
      ev(1, "run.started", { input: "杭州" }),
      ev(2, "plugin.tool_called", { name: "search_hotels", provider: "flyai", call_index: 1 }),
      ev(3, "plugin.tool_completed", { name: "search_hotels", provider: "flyai", call_index: 1, status: "error", attempts: 3 }),
      ev(4, "plugin.tool_called", { name: "get_weather", provider: "amap", call_index: 2 }),
      ev(5, "plugin.tool_completed", { name: "get_weather", provider: "amap", call_index: 2, status: "ok", cached: true }),
      ev(6, "plugin.tool_called", { name: "search_travel_notes", provider: "deepseek", call_index: 3 }),
      ev(7, "plugin.tool_completed", { name: "search_travel_notes", provider: "deepseek", call_index: 3, status: "ok", coalesced: true }),
    ];
    const tools = deriveConversation("run-1", events).filter(
      (item): item is ToolMessage => item.kind === "tool",
    );
    expect(tools).toHaveLength(3);
    expect(tools[0]).toMatchObject({ status: "error", attempts: 3 });
    expect(tools[0].error?.message).toContain("不影响继续");
    expect(tools[1]).toMatchObject({ status: "ok", cached: true, preview: [] });
    expect(tools[2]).toMatchObject({ status: "ok", coalesced: true });
  });

  it("keeps running and handles completion without a call event", () => {
    const events = [
      ev(1, "plugin.tool_called", { name: "get_weather", provider: "amap", call_index: 1 }),
      ev(2, "plugin.tool_completed", { name: "search_hotels", provider: "flyai", call_index: 2, status: "ok", query: "晚到" }),
    ];
    const tools = deriveConversation("run-1", events).filter(
      (item): item is ToolMessage => item.kind === "tool",
    );
    expect(tools.map((tool) => tool.status)).toEqual(["running", "ok"]);
    expect(tools[1].query).toBe("晚到");
  });
});

describe("missing fields and supplements", () => {
  it("folds the missing card and shows the supplemental delta", () => {
    const events = [
      ev(1, "run.started", { mode: "initial", input: "想去北京" }),
      ev(2, "constraint.needs_input", { missing: ["city", "days"] }),
      ev(3, "run.started", { mode: "initial", input: "想去北京，3 天，预算 3000" }),
      ev(4, "constraint.parsed", { constraints: { ...constraints, city: "北京", days: 3, budget: { total_yuan: 3000 } } }),
    ];
    const items = deriveConversation("run-1", events);
    const users = items.filter((item): item is UserMessage => item.kind === "user");
    expect(users).toHaveLength(2);
    expect(users[1]).toMatchObject({ variant: "supplement", text: "3 天，预算 3000" });
    const decisions = items.filter((item): item is DecisionMessage => item.kind === "decision");
    expect(decisions).toHaveLength(1);
    expect(decisions[0]).toMatchObject({ payload: { decision: "missing", missing: ["city", "days"] }, state: "resolved" });
  });

  it("falls back to the full input when no prefix matches", () => {
    const events = [
      ev(1, "run.started", { mode: "initial", input: "想去北京" }),
      ev(2, "constraint.needs_input", { missing: ["days"] }),
      ev(3, "run.started", { mode: "initial", input: "改成杭州 4 天" }),
    ];
    const users = deriveConversation("run-1", events).filter(
      (item): item is UserMessage => item.kind === "user",
    );
    expect(users[1].text).toBe("改成杭州 4 天");
  });
});

describe("lodging decisions", () => {
  const question = {
    code: "lodging_choice",
    message: "请选择住宿",
    options: [
      { id: "o1", label: "全季西湖店", hotel: { name: "全季西湖店", price_per_night_yuan: 480 } },
      { id: "o2", label: "不带住宿" },
    ],
  };

  it("does not duplicate the lodging card across needs_input and awaiting", () => {
    const events = [
      ev(1, "run.started", { input: "杭州 2 天" }),
      ev(2, "research.needs_input", { question }),
      ev(3, "run.awaiting_user", { stage: "research", research_question: question }),
      ev(4, "plan.updated", { seq: 1, plan, violations: [] }),
    ];
    const decisions = deriveConversation("run-1", events).filter(
      (item): item is DecisionMessage => item.kind === "decision",
    );
    expect(decisions).toHaveLength(1);
    expect(decisions[0]).toMatchObject({ payload: { decision: "lodging" }, state: "resolved" });
  });
});

describe("proposal decisions", () => {
  const proposal = {
    base_version: 1,
    scope: "day",
    day_index: 1,
    summary: "把灵隐寺挪到上午",
    impact: "步行减少 1.2km",
    warnings: ["预约时间需确认"],
  };

  it("accepts a proposal after completion", () => {
    const events = [
      ev(1, "run.started", { mode: "adjustment", input: "第二天轻松点" }),
      ev(2, "run.awaiting_user", { stage: "proposal", proposal }),
      ev(3, "run.completed", { seq: 2 }),
    ];
    const decisions = deriveConversation("run-1", events).filter(
      (item): item is DecisionMessage => item.kind === "decision",
    );
    expect(decisions).toHaveLength(1);
    expect(decisions[0]).toMatchObject({ state: "resolved", summary: "已接受。" });
  });

  it("marks a rejected proposal and allows a new one", () => {
    const events = [
      ev(1, "run.started", { mode: "adjustment", input: "第二天轻松点" }),
      ev(2, "run.awaiting_user", { stage: "proposal", proposal }),
      ev(3, "run.awaiting_user", { stage: "proposal_rejected", proposal: null }),
      ev(4, "run.started", { mode: "adjustment", input: "改成第三天轻松点" }),
      ev(5, "run.awaiting_user", { stage: "proposal", proposal: { ...proposal, summary: "调整第三天" } }),
    ];
    const decisions = deriveConversation("run-1", events).filter(
      (item): item is DecisionMessage => item.kind === "decision",
    );
    expect(decisions).toHaveLength(2);
    expect(decisions[0]).toMatchObject({ state: "resolved", summary: "已拒绝，等待修改。" });
    expect(decisions[1]).toMatchObject({ state: "open", payload: { decision: "proposal" } });
  });
});

describe("degradation, failure and cancellation", () => {
  it("narrates degradation without failing the run", () => {
    const events = [
      ev(1, "run.started", { input: "杭州" }),
      ev(2, "research.started", { tools: [] }),
      ev(3, "research.degraded", { reason: "住宿搜索超时" }),
      ev(4, "research.completed", { status: "degraded", hotel: "", guide_matches: 0 }),
    ];
    const research = agentMessages(deriveConversation("run-1", events)).find(
      (message) => message.stage === "research",
    );
    expect(research?.text).toContain("住宿搜索超时");
  });

  it("closes open cards on failure and emits one error", () => {
    const events = [
      ev(1, "run.started", { input: "杭州" }),
      ev(2, "run.awaiting_user", { stage: "present" }),
      ev(3, "run.failed", { code: "tool_timeout", message: "工具调用超时" }),
    ];
    const items = deriveConversation("run-1", events);
    expect(items.filter((item) => item.kind === "error")).toHaveLength(1);
    const decision = items.find((item): item is DecisionMessage => item.kind === "decision");
    expect(decision?.state).toBe("cancelled");
    expect(agentMessages(items).at(-1)?.tone).toBe("error");
  });

  it("cancels quietly without duplicate messages", () => {
    const items = deriveConversation("run-1", [
      ev(1, "run.started", { input: "杭州" }),
      ev(2, "run.awaiting_user", { stage: "present" }),
      ev(3, "run.cancelled", { reason: "rejected" }),
    ]);
    expect(items.filter((item) => item.kind === "agent")).toHaveLength(1);
    expect((items[1] as DecisionMessage).state).toBe("cancelled");
  });
});

describe("deriveCanvas", () => {
  it("prefers the latest event plan and marks drafts until completion", () => {
    const events = [
      ev(1, "run.started", { input: "杭州" }),
      ev(2, "plan.updated", { seq: 4, plan, violations: [] }),
    ];
    const result = deriveCanvas(events, tripWithVersion());
    expect(result.model).toMatchObject({ source: "event", isDraft: true, versionSeq: 4 });
    expect(result.model.plan?.city).toBe("杭州");
    expect(result.poiNameMap.get("poi-1")).toBe("灵隐寺");

    const settled = deriveCanvas([...events, ev(3, "run.completed", { seq: 4 })], tripWithVersion());
    expect(settled.model.isDraft).toBe(false);
  });

  it("uses an explicit version view and parses research", () => {
    const view: PlanVersion = tripWithVersion().latest_version as PlanVersion;
    const result = deriveCanvas(happyEvents(), tripWithVersion(plan, { status: "ok", notes: ["注意排队"] }), view);
    expect(result.model).toMatchObject({ source: "version", isDraft: false, versionSeq: 1 });
    expect(result.model.explanations.get("poi-0")).toBe("上午人少，光线也好。");
    expect(result.model.research?.notes).toEqual(["注意排队"]);
    expect(explanationMap(result.model.plan).size).toBe(1);
  });

  it("falls back to the latest version and tolerates missing research", () => {
    const result = deriveCanvas([], tripWithVersion({ city: "苏州", days: [], totals: { cost_yuan: 0, poi_count: 0, walking_km: 0 } }));
    expect(result.model).toMatchObject({ source: "version", versionSeq: 1, research: null });
    expect(result.model.plan?.city).toBe("苏州");
  });
});

describe("skill load narration", () => {
  it("merges repeats and never exposes the body, description or hash", () => {
    const events = [
      ev(1, "run.started", { input: "北京三天" }),
      ev(2, "research.started", { tools: [] }),
      ev(3, "research.skill_loaded", {
        name: "research-playbook",
        version: "1.0.0",
        sha256: "SECRET-HASH",
        body: "SECRET-BODY",
        description: "SECRET-DESCRIPTION",
      }),
      ev(4, "research.skill_loaded", { name: "research-playbook", version: "1.0.0", sha256: "SECRET-HASH" }),
      ev(5, "research.completed", { status: "ok" }),
    ];
    const items = deriveConversation("run-1", events);
    const skills = agentMessages(items).filter((message) => message.text.startsWith("已加载技能"));
    expect(skills).toHaveLength(1);
    expect(skills[0]).toMatchObject({ stage: "research", tone: "info", text: "已加载技能 research-playbook@1.0.0" });
    const raw = JSON.stringify(items);
    expect(raw).not.toContain("SECRET-BODY");
    expect(raw).not.toContain("SECRET-DESCRIPTION");
    expect(raw).not.toContain("SECRET-HASH");
  });
});
