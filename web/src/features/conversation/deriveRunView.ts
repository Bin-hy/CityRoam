import { parseResearchQuestion, type ResearchQuestion } from "@/features/research/types";
import { asNumber, asRecord, asString, asStringList, type RunEvent } from "./eventParse";

export type RunStatus =
  | "idle"
  | "queued"
  | "running"
  | "awaiting_user"
  | "completed"
  | "failed"
  | "cancelled";

export type RunStage =
  | "understand"
  | "research"
  | "draft"
  | "validate"
  | "repair"
  | "present"
  | "proposal"
  | "adjust";

export interface ViolationView {
  day: number;
  level: string;
  code: string;
  message: string;
  suggestion?: string;
}

export interface AdjustmentProposalView {
  baseVersion: number;
  scope: string;
  dayIndex: number;
  summary: string;
  impact: string;
  warnings: string[];
}

export interface RunView {
  status: RunStatus;
  stage: RunStage;
  mode: "initial" | "adjustment";
  round: number;
  attempt: number;
  lastSeq: number;
  missing: string[];
  proposal: AdjustmentProposalView | null;
  researchQuestion: ResearchQuestion | null;
  violations: ViolationView[];
  error: { code: string; message: string } | null;
  checked: number;
  draft: boolean;
}

export function initialRunView(): RunView {
  return {
    status: "idle",
    stage: "understand",
    mode: "initial",
    round: 0,
    attempt: 0,
    lastSeq: 0,
    missing: [],
    proposal: null,
    researchQuestion: null,
    violations: [],
    error: null,
    checked: 0,
    draft: false,
  };
}

export function deriveRunView(events: RunEvent[]): RunView {
  let view = initialRunView();
  for (const event of events) {
    if (event.seq <= view.lastSeq) continue;
    view = applyEvent(view, event);
  }
  return view;
}

export function applyEvent(view: RunView, event: RunEvent): RunView {
  const next: RunView = { ...view, lastSeq: event.seq };
  const payload = event.payload;
  switch (event.type) {
    case "run.started":
      next.status = "running";
      next.mode = asString(payload.mode) === "adjustment" ? "adjustment" : "initial";
      next.stage = next.mode === "adjustment" ? "adjust" : "understand";
      next.attempt = view.attempt + 1;
      next.round = 0;
      next.missing = [];
      next.proposal = null;
      next.researchQuestion = null;
      next.violations = [];
      next.error = null;
      next.checked = 0;
      next.draft = false;
      break;
    case "constraint.needs_input":
      next.stage = "understand";
      next.missing = asStringList(payload.missing);
      break;
    case "constraint.parsed":
      next.stage = "understand";
      next.missing = [];
      break;
    case "candidate.retrieved":
    case "plan.drafted":
      next.stage = "draft";
      break;
    case "research.started":
      next.stage = "research";
      break;
    case "research.completed":
    case "research.degraded":
      next.stage = "draft";
      next.researchQuestion = null;
      break;
    case "research.needs_input":
      next.stage = "research";
      next.researchQuestion = parseResearchQuestion(payload.question) ?? view.researchQuestion;
      break;
    case "validation.result":
      next.stage = "validate";
      next.violations = parseViolations(payload.violations);
      next.checked = asNumber(payload.checked, 0);
      break;
    case "repair.started":
      next.stage = "repair";
      next.round = Math.max(view.round, Math.trunc(asNumber(payload.round, view.round)));
      break;
    case "repair.completed":
      next.stage = "repair";
      break;
    case "plan.updated":
      next.stage = "present";
      next.violations = parseViolations(payload.violations);
      next.draft = true;
      break;
    case "run.awaiting_user":
      next.status = "awaiting_user";
      next.stage = stageFromAwaiting(asString(payload.stage));
      next.missing = asStringList(payload.missing);
      next.proposal = parseProposal(payload.proposal);
      next.researchQuestion = parseResearchQuestion(payload.research_question);
      break;
    case "run.completed":
      next.status = "completed";
      next.stage = "present";
      next.draft = false;
      next.missing = [];
      next.proposal = null;
      next.researchQuestion = null;
      next.error = null;
      break;
    case "run.cancelled":
      next.status = "cancelled";
      next.stage = "present";
      next.missing = [];
      next.proposal = null;
      next.researchQuestion = null;
      break;
    case "run.failed":
      next.status = "failed";
      next.stage = "present";
      next.error = {
        code: asString(payload.code, "run_failed"),
        message: asString(payload.message, "运行失败"),
      };
      break;
  }
  return next;
}

function stageFromAwaiting(stage: string): RunStage {
  if (stage === "proposal" || stage === "proposal_rejected") return "proposal";
  if (stage === "research") return "research";
  return "present";
}

export function parseProposal(raw: unknown): AdjustmentProposalView | null {
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

export function parseViolations(raw: unknown): ViolationView[] {
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
      suggestion: asString(value.suggestion) || undefined,
    });
  }
  return violations;
}
