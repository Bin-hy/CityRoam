export interface AdvancedOptions {
  days: string;
  pace: string;
  partySize: string;
  budgetYuan: string;
  maxHours: string;
  interests: string;
  mustVisit: string;
  avoid: string;
  meals: boolean;
}

export const emptyAdvanced: AdvancedOptions = {
  days: "",
  pace: "",
  partySize: "",
  budgetYuan: "",
  maxHours: "",
  interests: "",
  mustVisit: "",
  avoid: "",
  meals: false,
};

const paceLabels: Record<string, string> = { relaxed: "轻松", normal: "适中", packed: "紧凑" };

export function countAdvanced(options: AdvancedOptions): number {
  let count = 0;
  if (options.days) count++;
  if (options.pace) count++;
  if (options.partySize) count++;
  if (options.budgetYuan) count++;
  if (options.maxHours) count++;
  if (options.interests.trim()) count++;
  if (options.mustVisit.trim()) count++;
  if (options.avoid.trim()) count++;
  if (options.meals) count++;
  return count;
}

export function composeInput(text: string, options: AdvancedOptions): string {
  const parts: string[] = [];
  if (options.days) parts.push(`共 ${options.days} 天`);
  if (options.pace) parts.push(`节奏${paceLabels[options.pace] ?? options.pace}`);
  if (options.partySize) parts.push(`${options.partySize} 人同行`);
  if (options.budgetYuan) parts.push(`总预算 ${options.budgetYuan} 元`);
  if (options.maxHours) parts.push(`每天游玩不超过 ${options.maxHours} 小时`);
  if (options.interests.trim()) parts.push(`感兴趣：${options.interests.trim()}`);
  if (options.mustVisit.trim()) parts.push(`必须去：${options.mustVisit.trim()}`);
  if (options.avoid.trim()) parts.push(`避开：${options.avoid.trim()}`);
  if (options.meals) parts.push("需要安排午餐和晚餐");

  const base = text.trim();
  if (parts.length === 0) return base;
  return `${base}${base ? "。" : ""}${parts.join("，")}。`;
}
