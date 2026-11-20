import { asNumber, asRecord, asString, asStringList } from "./eventParse";
import type { ViolationView } from "./deriveRunView";

export interface ConstraintChips {
  city?: string;
  days?: number;
  budgetYuan?: number;
  pace?: string;
  interests?: string[];
  mustVisit?: string[];
  avoid?: string[];
}

export interface JumpTarget {
  label: string;
  day: number;
  blockID?: string;
  poiID?: string;
}

const paceLabels: Record<string, string> = {
  relaxed: "轻松",
  normal: "适中",
  packed: "紧凑",
};

const missingLabels: Record<string, string> = {
  city: "城市",
  days: "天数",
  start_date: "出发日期",
  budget: "预算",
};

export function constraintChips(raw: unknown): ConstraintChips {
  const value = asRecord(raw);
  if (!value) return {};
  const chips: ConstraintChips = {};
  const city = asString(value.city);
  if (city) chips.city = city;
  const days = Math.trunc(asNumber(value.days, 0));
  if (days > 0) chips.days = days;
  const budget = asNumber(asRecord(value.budget)?.total_yuan, 0);
  if (budget > 0) chips.budgetYuan = Math.trunc(budget);
  const pace = asString(value.pace);
  if (pace && pace !== "unknown") chips.pace = paceLabels[pace] ?? pace;
  const interests = asStringList(value.interests);
  if (interests.length > 0) chips.interests = interests;
  const mustVisit = asStringList(value.must_visit);
  if (mustVisit.length > 0) chips.mustVisit = mustVisit;
  const avoid = asStringList(value.avoid);
  if (avoid.length > 0) chips.avoid = avoid;
  return chips;
}

export function understandText(chips: ConstraintChips): string {
  const parts = [
    chips.city,
    chips.days ? `${chips.days} 天` : "",
    chips.budgetYuan ? `预算 ¥${chips.budgetYuan.toLocaleString("zh-CN")}` : "",
    chips.pace,
  ].filter(Boolean);
  if (parts.length === 0) return "已记录你的需求，开始规划行程。";
  return `已理解：${parts.join(" · ")}，开始规划行程。`;
}

export function missingLabel(field: string): string {
  return missingLabels[field] ?? field;
}

export function researchText(payload: Record<string, unknown>): string {
  const hotel = asString(payload.hotel);
  const guides = Math.trunc(asNumber(payload.guide_matches, 0));
  const weatherDays = Math.trunc(asNumber(payload.weather_days, 0));
  const parts: string[] = [];
  if (guides > 0) parts.push(`参考了 ${guides} 篇攻略`);
  if (hotel) parts.push(`住宿初选「${hotel}」`);
  if (weatherDays > 0) parts.push(`查看了 ${weatherDays} 天天气`);
  if (parts.length === 0) return "研究完成，继续生成行程草稿。";
  return `${parts.join("，")}。`;
}

export function degradeText(reason: string): string {
  if (!reason) return "部分外部数据暂不可用，先按已有信息继续。";
  return `部分外部数据暂不可用（${reason}），先按已有信息继续。`;
}

export function validationText(checked: number, blocking: number, warnings: number): string {
  const basis = checked > 0 ? `已完成 ${checked} 项校验` : "已完成约束校验";
  if (blocking > 0) {
    const warn = warnings > 0 ? `、${warnings} 个提醒` : "";
    return `${basis}，发现 ${blocking} 个阻塞问题${warn}。`;
  }
  if (warnings > 0) return `${basis}，没有阻塞问题，另有 ${warnings} 个提醒。`;
  return `${basis}，全部通过。`;
}

export function repairText(round: number, blocking: number, dayIndex?: number): string {
  const suffix = dayIndex && dayIndex > 0 ? `（第 ${dayIndex} 天）` : "";
  return `第 ${round} 轮修复：处理 ${blocking} 个阻塞问题${suffix}。`;
}

export function completionText(seq: number): string {
  return seq > 0 ? `方案已确认，保存为版本 v${seq}。` : "方案已确认。";
}

export function supplementDelta(previousInput: string, nextInput: string): string {
  const previous = previousInput.trim();
  const next = nextInput.trim();
  if (!previous) return next;
  if (!next.startsWith(previous)) return next;
  return next.slice(previous.length).replace(/^[\s，,、。.;；:：]+/, "").trim() || next;
}

export function jumpTargets(violations: ViolationView[]): JumpTarget[] {
  return violations
    .filter((violation) => violation.day > 0 && violation.level === "blocking")
    .map((violation) => ({
      label: `第 ${violation.day} 天 · ${violation.message}`,
      day: violation.day - 1,
    }));
}
