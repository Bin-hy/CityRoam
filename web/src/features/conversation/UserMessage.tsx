import { Chip } from "@/design/primitives";
import type { UserMessage as UserMessageData } from "./deriveConversation";

const variantLabels: Record<UserMessageData["variant"], string> = {
  initial: "首次描述",
  supplement: "补充",
  revise: "修改",
  adjust: "调整",
};

export function UserMessage({ message }: { message: UserMessageData }) {
  const chips = message.constraints
    ? [
        message.constraints.city,
        message.constraints.days ? `${message.constraints.days} 天` : "",
        message.constraints.budgetYuan ? `¥${message.constraints.budgetYuan.toLocaleString("zh-CN")}` : "",
        message.constraints.pace,
        ...(message.constraints.interests ?? []),
        ...(message.constraints.mustVisit ?? []).map((item) => `必去 ${item}`),
        ...(message.constraints.avoid ?? []).map((item) => `避开 ${item}`),
      ].filter((item): item is string => Boolean(item))
    : [];

  return (
    <div className="flex justify-end">
      <div className="max-w-[88%] rounded-md rounded-tr-sm border border-clay/20 bg-clay/[0.07] px-4 py-3">
        <div className="mb-1 flex items-center justify-end gap-2">
          <span className="text-[11px] tracking-[0.12em] text-clay">{variantLabels[message.variant]}</span>
        </div>
        <p className="whitespace-pre-wrap text-[15px] leading-relaxed text-ink">{message.text}</p>
        {chips.length > 0 && (
          <div className="mt-2 flex flex-wrap justify-end gap-1.5">
            {chips.map((chip) => (
              <Chip key={chip} className="border-clay/20 bg-paper/60 text-ink/75">
                {chip}
              </Chip>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
