import { parseItinerary } from "@/features/itinerary/types";
import { parseResearchQuestion, type ResearchQuestion } from "@/features/research/types";
import {
  asNumber,
  asRecord,
  asString,
  asStringList,
  normalizeEvents,
  type RunEvent,
} from "./eventParse";
import type { AdjustmentProposalView, ViolationView } from "./deriveRunView";
import {
  completionText,
  constraintChips,
  degradeText,
  jumpTargets,
  missingLabel,
  repairText,
  researchText,
  supplementDelta,
  understandText,
  validationText,
  type ConstraintChips,
  type JumpTarget,
} from "./narrate";

export interface EventPreview {
  title: string;
  subtitle?: string;
  priceYuan?: number;
}

export interface UserMessage {
  kind: "user";
  id: string;
  seq: number;
  text: string;
  variant: "initial" | "supplement" | "revise" | "adjust";
  constraints?: ConstraintChips;
}

export interface AgentMessage {
  kind: "agent";
  id: string;
  seq: number;
  stage: "understand" | "research" | "draft" | "validate" | "repair" | "complete";
  tone: "info" | "success" | "warning" | "error";
  text: string;
  details?: string[];
  jumps?: JumpTarget[];
}

export interface ToolMessage {
  kind: "tool";
  id: string;
  seq: number;
  callKey: string;
  name: string;
  provider: string;
  query: string;
  status: "running" | "ok" | "error";
  latencyMs?: number;
  resultCount?: number;
  cached?: boolean;
  coalesced?: boolean;
  attempts?: number;
  preview: EventPreview[];
  error?: { code: string; message: string };
}

export type DecisionPayload =
  | { decision: "missing"; missing: string[] }
  | { decision: "lodging"; question: ResearchQuestion }
  | { decision: "proposal"; proposal: AdjustmentProposalView }
  | { decision: "confirm" };

export interface DecisionMessage {
  kind: "decision";
  id: string;
  seq: number;
  payload: DecisionPayload;
  state: "open" | "resolved" | "cancelled";
  summary?: string;
}

export interface PlanMessage {
  kind: "plan";
  id: string;
  seq: number;
  versionSeq: number | null;
  stats: { days: number; pois: number; totalYuan: number; walkingKm: number };
  isDraft: boolean;
}

export interface SystemMessage {
  kind: "system";
  id: string;
  seq: number;
  text: string;
}

export interface ErrorMessage {
  kind: "error";
  id: string;
  seq: number;
  code: string;
  message: string;
  retryable: boolean;
}

export type ConversationItem =
  | UserMessage
  | AgentMessage
  | ToolMessage
  | DecisionMessage
  | PlanMessage
  | SystemMessage
  | ErrorMessage;

interface SlotRef {
  index: number;
  id: string;
}

export function deriveConversation(
  runId: string,
  rawEvents: unknown,
  priorInput = "",
): ConversationItem[] {
  const events = normalizeEvents(rawEvents);
  const items: ConversationItem[] = [];
  const slots = new Map<string, SlotRef>();
  const calls = new Map<string, SlotRef & { completed: boolean }>();
  const decisions = new Map<string, SlotRef>();
  let attempt = 0;
  let previousInput = priorInput;
  let chips: ConstraintChips = {};
  let openDecisionID: string | null = null;
  let degradeReason = "";

  const push = <T extends ConversationItem>(item: T): T => {
    items.push(item);
    return item;
  };

  const agentSlot = (
    event: RunEvent,
    slot: string,
    data: Omit<AgentMessage, "kind" | "id" | "seq">,
  ): AgentMessage => {
    const key = `${attempt}:${slot}`;
    const ref = slots.get(key);
    if (ref) {
      const existing = items[ref.index] as AgentMessage;
      const updated: AgentMessage = { ...existing, ...data };
      items[ref.index] = updated;
      return updated;
    }
    const message: AgentMessage = {
      kind: "agent",
      id: `${runId}:${event.seq}:${slot}`,
      seq: event.seq,
      ...data,
    };
    slots.set(key, { index: items.length, id: message.id });
    return push(message);
  };

  const resolveDecision = (
    id: string,
    state: "resolved" | "cancelled",
    summary?: string,
  ) => {
    const ref = decisions.get(id);
    if (!ref) return;
    const existing = items[ref.index] as DecisionMessage;
    items[ref.index] = { ...existing, state, ...(summary ? { summary } : {}) };
    if (openDecisionID === id) openDecisionID = null;
  };

  const openDecision = (
    event: RunEvent,
    decision: DecisionPayload,
    summaryForPrevious?: string,
  ): DecisionMessage => {
    if (openDecisionID && summaryForPrevious) {
      resolveDecision(openDecisionID, "resolved", summaryForPrevious);
    }
    const id = `${runId}:${event.seq}:decision:${decision.decision}`;
    const message: DecisionMessage = {
      kind: "decision",
      id,
      seq: event.seq,
      payload: decision,
      state: "open",
    };
    decisions.set(id, { index: items.length, id });
    openDecisionID = id;
    return push(message);
  };

  const closeOpen = (state: "resolved" | "cancelled", summary?: string) => {
    if (openDecisionID) resolveDecision(openDecisionID, state, summary);
  };

  const openDecisionKind = (): DecisionPayload["decision"] | null => {
    if (!openDecisionID) return null;
    const ref = decisions.get(openDecisionID);
    if (!ref) return null;
    return (items[ref.index] as DecisionMessage).payload.decision;
  };

  const planMessage = (event: RunEvent, payload: Record<string, unknown>, isDraft: boolean) => {
    const plan = parseItinerary(payload.plan);
    const versionSeq = Math.trunc(asNumber(payload.seq, 0)) || null;
    const stats = {
      days: plan?.days.length ?? 0,
      pois: plan?.totals.poi_count ?? 0,
      totalYuan: plan?.totals.cost_yuan ?? 0,
      walkingKm: plan?.totals.walking_km ?? 0,
    };
    const existing = slots.get(`${attempt}:plan`);
    if (existing) {
      const current = items[existing.index] as PlanMessage;
      items[existing.index] = { ...current, versionSeq: versionSeq ?? current.versionSeq, stats, isDraft };
      return;
    }
    const message: PlanMessage = {
      kind: "plan",
      id: `${runId}:${event.seq}:plan`,
      seq: event.seq,
      versionSeq,
      stats,
      isDraft,
    };
    slots.set(`${attempt}:plan`, { index: items.length, id: message.id });
    push(message);
  };

  for (const event of events) {
    const payload = event.payload;
    switch (event.type) {
      case "run.started": {
        const mode = asString(payload.mode) === "adjustment" ? "adjustment" : "initial";
        const input = asString(payload.input);
        attempt += 1;
        chips = {};
        degradeReason = "";
        const variant: UserMessage["variant"] =
          mode === "adjustment" ? "adjust" : attempt === 1 ? "initial" : "supplement";
        const text =
          variant === "supplement" ? supplementDelta(previousInput, input) : input.trim();
        previousInput = input;
        push({
          kind: "user",
          id: `${runId}:${event.seq}:user`,
          seq: event.seq,
          text,
          variant,
        });
        closeOpen("resolved", "已提供信息，继续处理。");
        break;
      }
      case "constraint.needs_input": {
        const missing = asStringList(payload.missing);
        openDecision(event, { decision: "missing", missing });
        agentSlot(event, "understand", {
          stage: "understand",
          tone: "warning",
          text: `还需要补充：${missing.map(missingLabel).join("、")}。`,
        });
        break;
      }
      case "constraint.parsed": {
        chips = constraintChips(payload.constraints);
        closeOpen("resolved", "已补充信息。");
        const user = [...items].reverse().find((item): item is UserMessage => item.kind === "user");
        if (user) {
          const index = items.indexOf(user);
          items[index] = { ...user, constraints: chips };
        }
        agentSlot(event, "understand", {
          stage: "understand",
          tone: "info",
          text: understandText(chips),
        });
        break;
      }
      case "candidate.retrieved": {
        const count = Math.trunc(asNumber(payload.count, 0));
        const attempts = Math.trunc(asNumber(payload.attempts, 0));
        agentSlot(event, "draft", {
          stage: "draft",
          tone: "info",
          text: count > 0 ? `已检索到 ${count} 个候选点，正在挑选与排程。` : "正在挑选与排程。",
          ...(attempts > 1 ? { details: [`检索尝试 ${attempts} 次`] } : {}),
        });
        break;
      }
      case "research.started": {
        const tools = asStringList(payload.tools);
        agentSlot(event, "research", {
          stage: "research",
          tone: "info",
          text: "正在研究攻略、住宿与天气…",
          ...(tools.length > 0 ? { details: [`工具：${tools.join("、")}`] } : {}),
        });
        break;
      }
      case "research.skill_loaded": {
        const name = asString(payload.name);
        const version = asString(payload.version);
        if (!name || !version) break;
        agentSlot(event, `skill:${name}@${version}`, {
          stage: "research",
          tone: "info",
          text: `已加载技能 ${name}@${version}`,
        });
        break;
      }
      case "research.completed": {
        const degraded = asString(payload.status) === "degraded";
        agentSlot(event, "research", {
          stage: "research",
          tone: degraded ? "warning" : "success",
          text: degraded && degradeReason ? degradeText(degradeReason) : researchText(payload),
        });
        break;
      }
      case "research.degraded": {
        degradeReason = asString(payload.reason);
        agentSlot(event, "research", {
          stage: "research",
          tone: "warning",
          text: degradeText(degradeReason),
        });
        break;
      }
      case "research.needs_input": {
        const question = asQuestion(payload.question);
        if (question && openDecisionKind() !== "lodging") {
          openDecision(event, { decision: "lodging", question }, "已处理上一项选择。");
        }
        break;
      }
      case "plan.drafted": {
        agentSlot(event, "draft", {
          stage: "draft",
          tone: "info",
          text: "草稿已生成，正在做时间排程。",
        });
        break;
      }
      case "validation.result": {
        const blocking = Math.trunc(asNumber(payload.blocking, 0));
        const warnings = Math.trunc(asNumber(payload.warnings, 0));
        const checked = Math.trunc(asNumber(payload.checked, 0));
        const jumps = jumpTargets(parseViolationList(payload.violations));
        agentSlot(event, "validate", {
          stage: "validate",
          tone: blocking > 0 ? "warning" : "success",
          text: validationText(checked, blocking, warnings),
          ...(jumps.length > 0 ? { jumps } : {}),
        });
        break;
      }
      case "repair.started": {
        const round = Math.max(1, Math.trunc(asNumber(payload.round, 1)));
        const blocking = Math.trunc(asNumber(payload.blocking, 0));
        agentSlot(event, `repair:${round}`, {
          stage: "repair",
          tone: "warning",
          text: repairText(round, blocking),
        });
        break;
      }
      case "repair.completed": {
        const round = Math.max(1, Math.trunc(asNumber(payload.round, 1)));
        const key = `${attempt}:repair:${round}`;
        if (slots.has(key)) {
          agentSlot(event, `repair:${round}`, {
            stage: "repair",
            tone: "info",
            text: "本轮修复完成，正在重新校验。",
          });
        }
        break;
      }
      case "plan.updated": {
        closeOpen("resolved", "已根据选择更新方案。");
        planMessage(event, payload, true);
        break;
      }
      case "run.awaiting_user": {
        const stage = asString(payload.stage);
        const missing = asStringList(payload.missing);
        const proposal = parseProposalView(payload.proposal);
        const question = asQuestion(payload.research_question);
        if (missing.length > 0) {
          if (openDecisionKind() !== "missing") {
            openDecision(event, { decision: "missing", missing }, "已处理上一项选择。");
          }
        } else if (proposal) {
          openDecision(event, { decision: "proposal", proposal }, "已处理上一项选择。");
        } else if (question) {
          if (openDecisionKind() !== "lodging") {
            openDecision(event, { decision: "lodging", question }, "已处理上一项选择。");
          }
        } else if (stage === "proposal_rejected") {
          closeOpen("resolved", "已拒绝，等待修改。");
        } else {
          openDecision(event, { decision: "confirm" }, "已处理上一项选择。");
        }
        break;
      }
      case "run.completed": {
        closeOpen("resolved", "已接受。");
        const seq = Math.trunc(asNumber(payload.seq, 0));
        const ref = slots.get(`${attempt}:plan`);
        if (ref) {
          const current = items[ref.index] as PlanMessage;
          items[ref.index] = {
            ...current,
            versionSeq: seq || current.versionSeq,
            isDraft: false,
          };
        }
        agentSlot(event, "complete", {
          stage: "complete",
          tone: "success",
          text: completionText(seq),
        });
        break;
      }
      case "run.cancelled": {
        closeOpen("cancelled", "已取消。");
        agentSlot(event, "complete", {
          stage: "complete",
          tone: "warning",
          text: "本次运行已取消，可以继续描述修改。",
        });
        break;
      }
      case "run.failed": {
        closeOpen("cancelled", "运行失败。");
        const code = asString(payload.code, "run_failed");
        const message = asString(payload.message, "运行失败");
        push({
          kind: "error",
          id: `${runId}:${event.seq}:error`,
          seq: event.seq,
          code,
          message,
          retryable: code !== "interrupted",
        });
        agentSlot(event, "complete", {
          stage: "complete",
          tone: "error",
          text: `本次运行失败：${message}`,
        });
        break;
      }
      case "plugin.tool_called": {
        const message = toolFromCalled(runId, event);
        calls.set(message.callKey, { index: items.length, id: message.id, completed: false });
        push(message);
        break;
      }
      case "plugin.tool_completed": {
        const callKey = toolCallKey(payload);
        const ref = calls.get(callKey);
        if (ref) {
          const current = items[ref.index] as ToolMessage;
          items[ref.index] = { ...current, ...toolCompleted(payload) };
          ref.completed = true;
        } else {
          const message = { ...toolFromCalled(runId, event), ...toolCompleted(payload) };
          calls.set(callKey, { index: items.length, id: message.id, completed: true });
          push(message);
        }
        break;
      }
    }
  }

  return items;
}

function toolCallKey(payload: Record<string, unknown>): string {
  return `${asString(payload.name, "unknown")}:${Math.trunc(asNumber(payload.call_index, 0))}`;
}

function toolFromCalled(runId: string, event: RunEvent): ToolMessage {
  const callKey = toolCallKey(event.payload);
  return {
    kind: "tool",
    id: `${runId}:${event.seq}:tool:${callKey}`,
    seq: event.seq,
    callKey,
    name: asString(event.payload.name, "unknown"),
    provider: asString(event.payload.provider),
    query: asString(event.payload.query),
    status: "running",
    preview: [],
  };
}

function toolCompleted(payload: Record<string, unknown>): Partial<ToolMessage> {
  const status = asString(payload.status) === "error" ? "error" : "ok";
  const latency = Math.trunc(asNumber(payload.latency_ms, 0));
  const count = Math.trunc(asNumber(payload.result_count, 0));
  const attempts = Math.trunc(asNumber(payload.attempts, 0));
  return {
    status,
    ...(latency > 0 ? { latencyMs: latency } : {}),
    ...(count > 0 ? { resultCount: count } : {}),
    cached: payload.cached === true,
    coalesced: payload.coalesced === true,
    ...(attempts > 1 ? { attempts } : {}),
    preview: parsePreview(payload.preview),
    ...(status === "error" ? { error: { code: "tool_failed", message: "外部工具调用失败，不影响继续。" } } : {}),
  };
}

function parsePreview(raw: unknown): EventPreview[] {
  if (!Array.isArray(raw)) return [];
  const previews: EventPreview[] = [];
  for (const item of raw.slice(0, 3)) {
    const value = asRecord(item);
    if (!value) continue;
    const title = asString(value.title);
    if (!title) continue;
    const subtitle = asString(value.subtitle);
    const price = asNumber(value.price_yuan, 0);
    previews.push({
      title,
      ...(subtitle ? { subtitle } : {}),
      ...(price > 0 ? { priceYuan: price } : {}),
    });
  }
  return previews;
}

function parseProposalView(raw: unknown): AdjustmentProposalView | null {
  const value = asRecord(raw);
  if (!value) return null;
  return {
    baseVersion: Math.trunc(asNumber(value.base_version, 0)),
    scope: asString(value.scope),
    dayIndex: Math.trunc(asNumber(value.day_index, 0)),
    summary: asString(value.summary),
    impact: asString(value.impact),
    warnings: asStringList(value.warnings),
  };
}

function parseViolationList(raw: unknown): ViolationView[] {
  if (!Array.isArray(raw)) return [];
  const violations: ViolationView[] = [];
  for (const item of raw) {
    const value = asRecord(item);
    if (!value) continue;
    violations.push({
      day: Math.trunc(asNumber(value.day, 0)),
      level: asString(value.level, "warning"),
      code: asString(value.code),
      message: asString(value.message),
    });
  }
  return violations;
}

function asQuestion(raw: unknown): ResearchQuestion | null {
  return parseResearchQuestion(raw);
}
