import { parseResearchQuestion, type ResearchQuestion } from "@/features/research/types";

export type RunStatus =
  | "idle"
  | "queued"
  | "running"
  | "awaiting_user"
  | "completed"
  | "failed"
  | "cancelled";

export interface ViolationView {
  day: number;
  level: string;
  code: string;
  message: string;
  suggestion?: string;
}

export interface TraceEntry {
  seq: number;
  type: string;
  title: string;
  detail?: string;
  level: "info" | "success" | "warning" | "error";
  receivedAt: number;
}

export interface RunEvent {
  seq: number;
  type: string;
  payload: Record<string, unknown>;
  receivedAt?: number;
}

export interface TraceState {
  status: RunStatus;
  stage: string;
  round: number;
  lastSeq: number;
  entries: TraceEntry[];
  violations: ViolationView[];
  missing: string[];
  error: { code: string; message: string } | null;
  proposal: Record<string, unknown> | null;
  researchQuestion: ResearchQuestion | null;
}

export type TraceAction = { type: "reset"; lastSeq?: number } | { type: "event"; event: RunEvent };

const maxEntries = 200;

export const stageLabels: Record<string, string> = {
  admit: "受理",
  parse: "解析需求",
  retrieve: "检索候选",
  research: "研究攻略与住宿",
  draft: "生成草稿",
  schedule: "时间调度",
  validate: "约束校验",
  repair: "修复",
  present: "生成方案",
  adjust: "调整行程",
  proposal: "调整方案",
  proposal_rejected: "已拒绝提案，待修改",
  awaiting: "等待确认",
  running: "运行中",
  completed: "已完成",
  cancelled: "已取消",
  failed: "失败",
};

export function initialTraceState(lastSeq = 0): TraceState {
  return {
    status: "idle",
    stage: "admit",
    round: 0,
    lastSeq,
    entries: [],
    violations: [],
    missing: [],
    error: null,
    proposal: null,
    researchQuestion: null,
  };
}

export function traceReducer(state: TraceState, action: TraceAction): TraceState {
  if (action.type === "reset") {
    return initialTraceState(action.lastSeq ?? 0);
  }
  const event = action.event;
  if (!Number.isFinite(event.seq) || event.seq <= state.lastSeq) {
    return state;
  }

  const next: TraceState = {
    ...state,
    lastSeq: event.seq,
    entries: [...state.entries, describe(event)].slice(-maxEntries),
  };

  switch (event.type) {
    case "run.started":
      next.status = "running";
      next.stage = string(event.payload.mode) === "adjustment" ? "adjust" : "parse";
      next.error = null;
      next.missing = [];
      next.proposal = null;
      break;
    case "constraint.needs_input":
      next.stage = "parse";
      next.missing = strings(event.payload.missing);
      break;
    case "constraint.parsed":
      next.stage = "retrieve";
      next.missing = [];
      break;
    case "candidate.retrieved":
      next.stage = "draft";
      break;
    case "research.started":
      next.stage = "research";
      break;
    case "research.completed":
      next.stage = "draft";
      break;
    case "research.degraded":
      next.stage = "draft";
      break;
    case "research.needs_input":
      next.stage = "research";
      next.researchQuestion = parseResearchQuestion(event.payload.question) ?? state.researchQuestion;
      break;
    case "plan.drafted":
      next.stage = "schedule";
      break;
    case "validation.result":
      next.stage = "validate";
      next.violations = violations(event.payload.violations);
      break;
    case "repair.started":
      next.stage = "repair";
      next.round = Math.max(next.round, integer(event.payload.round, state.round));
      break;
    case "repair.completed":
      next.stage = "repair";
      break;
    case "plan.updated":
      next.stage = "present";
      next.violations = violations(event.payload.violations);
      break;
    case "run.awaiting_user":
      next.status = "awaiting_user";
      next.stage = string(event.payload.stage) || "present";
      next.missing = strings(event.payload.missing);
      next.proposal = object(event.payload.proposal);
      next.researchQuestion = parseResearchQuestion(event.payload.research_question);
      break;
    case "run.completed":
      next.status = "completed";
      next.stage = "completed";
      break;
    case "run.cancelled":
      next.status = "cancelled";
      next.stage = "cancelled";
      break;
    case "run.failed":
      next.status = "failed";
      next.stage = "failed";
      next.error = {
        code: string(event.payload.code) || "run_failed",
        message: string(event.payload.message) || "运行失败",
      };
      break;
  }
  return next;
}

export function describe(event: RunEvent): TraceEntry {
  const base = { seq: event.seq, type: event.type, receivedAt: event.receivedAt ?? Date.now() };
  switch (event.type) {
    case "run.started":
      return { ...base, title: "开始规划", detail: string(event.payload.input), level: "info" };
    case "constraint.parsed":
      return { ...base, title: "约束解析完成", detail: cityDays(event.payload.constraints), level: "info" };
    case "constraint.needs_input":
      return {
        ...base,
        title: "需要补充信息",
        detail: strings(event.payload.missing).map(missingLabel).join("、"),
        level: "warning",
      };
    case "candidate.retrieved":
      return {
        ...base,
        title: `检索到 ${integer(event.payload.count)} 个候选点`,
        detail: integer(event.payload.attempts) > 1 ? `检索尝试 ${integer(event.payload.attempts)} 次` : undefined,
        level: "info",
      };
    case "plan.drafted":
      return { ...base, title: "草稿生成", level: "info" };
    case "research.started":
      return {
        ...base,
        title: "开始研究攻略与住宿",
        detail: countDetail(event.payload.tools, "个工具"),
        level: "info",
      };
    case "plugin.tool_called":
      return {
        ...base,
        title: `调用 ${string(event.payload.name) || "外部工具"}`,
        detail: string(event.payload.provider) || undefined,
        level: "info",
      };
    case "plugin.tool_completed": {
      const failed = string(event.payload.status) === "error";
      const parts = [
        integer(event.payload.latency_ms) > 0 ? `${integer(event.payload.latency_ms)}ms` : "",
        integer(event.payload.result_count) > 0 ? `${integer(event.payload.result_count)} 条结果` : "",
      ].filter(Boolean);
      return {
        ...base,
        title: `${string(event.payload.name) || "外部工具"}${failed ? " 失败" : " 完成"}`,
        detail: parts.join(" · ") || undefined,
        level: failed ? "warning" : "success",
      };
    }
    case "research.skill_loaded": {
      const name = string(event.payload.name);
      const version = string(event.payload.version);
      return {
        ...base,
        title: `加载技能 ${name}`,
        detail: version ? `${name}@${version}` : undefined,
        level: "info",
      };
    }
    case "research.completed":
      return {
        ...base,
        title: "研究完成",
        detail: [string(event.payload.hotel), `${integer(event.payload.guide_matches)} 条攻略`]
          .filter((item) => item && item !== "0 条攻略")
          .join(" · ") || undefined,
        level: string(event.payload.status) === "degraded" ? "warning" : "success",
      };
    case "research.degraded":
      return {
        ...base,
        title: "研究降级",
        detail: string(event.payload.reason) || "部分外部数据不可用",
        level: "warning",
      };
    case "research.needs_input":
      return { ...base, title: "需要确认住宿", detail: "请选择酒店或继续不带住宿", level: "warning" };
    case "validation.result": {
      const blocking = integer(event.payload.blocking);
      const warnings = integer(event.payload.warnings);
      return {
        ...base,
        title: blocking > 0 ? `校验发现 ${blocking} 个阻塞问题` : "校验通过",
        detail: warnings > 0 ? `另有 ${warnings} 个提醒` : undefined,
        level: blocking > 0 ? "warning" : "success",
      };
    }
    case "repair.started":
      return {
        ...base,
        title: `第 ${integer(event.payload.round) || 1} 轮修复`,
        detail: `处理 ${integer(event.payload.blocking)} 个阻塞问题`,
        level: "warning",
      };
    case "repair.completed":
      return { ...base, title: "修复完成", detail: "重新校验中", level: "info" };
    case "plan.updated":
      return {
        ...base,
        title: "方案已更新",
        detail: integer(event.payload.seq) > 0 ? `版本 v${integer(event.payload.seq)}` : undefined,
        level: "success",
      };
    case "run.awaiting_user":
      return { ...base, title: "等待你的确认", level: "info" };
    case "run.completed":
      return { ...base, title: "规划完成", level: "success" };
    case "run.cancelled":
      return { ...base, title: "已取消", level: "warning" };
    case "run.failed":
      return { ...base, title: "规划失败", detail: string(event.payload.message), level: "error" };
    default:
      return { ...base, title: event.type, level: "info" };
  }
}

function cityDays(raw: unknown): string | undefined {
  const constraints = object(raw);
  if (!constraints) return undefined;
  const city = string(constraints.city);
  const days = integer(constraints.days);
  return [city, days > 0 ? `${days} 天` : ""].filter(Boolean).join(" · ") || undefined;
}

function missingLabel(field: string): string {
  return field === "city" ? "城市" : field === "days" ? "天数" : field;
}

function countDetail(raw: unknown, suffix: string): string | undefined {
  return Array.isArray(raw) && raw.length > 0 ? `${raw.length} ${suffix}` : undefined;
}

function violations(raw: unknown): ViolationView[] {
  if (!Array.isArray(raw)) return [];
  return raw.flatMap((item) => {
    const value = object(item);
    if (!value) return [];
    return [
      {
        day: integer(value.day),
        level: string(value.level) || "warning",
        code: string(value.code),
        message: string(value.message),
        suggestion: string(value.suggestion) || undefined,
      },
    ];
  });
}

function strings(raw: unknown): string[] {
  if (!Array.isArray(raw)) return [];
  return raw.filter((item): item is string => typeof item === "string");
}

function object(raw: unknown): Record<string, unknown> | null {
  return raw !== null && typeof raw === "object" && !Array.isArray(raw)
    ? (raw as Record<string, unknown>)
    : null;
}

function integer(raw: unknown, fallback = 0): number {
  const value = typeof raw === "number" ? raw : Number(raw);
  return Number.isFinite(value) ? Math.trunc(value) : fallback;
}

function string(raw: unknown): string {
  return typeof raw === "string" ? raw : "";
}
