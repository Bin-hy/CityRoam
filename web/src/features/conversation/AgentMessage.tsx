import { useState } from "react";
import { ChevronDown, Compass } from "lucide-react";
import { cn } from "@/lib/utils";
import type { AgentMessage as AgentMessageData } from "./deriveConversation";
import type { JumpTarget } from "./narrate";

const stageLabels: Record<AgentMessageData["stage"], string> = {
  understand: "理解需求",
  research: "研究",
  draft: "排程",
  validate: "校验",
  repair: "修复",
  complete: "完成",
};

const toneClasses: Record<AgentMessageData["tone"], string> = {
  info: "text-ink",
  success: "text-success",
  warning: "text-warning",
  error: "text-danger",
};

const dotClasses: Record<AgentMessageData["tone"], string> = {
  info: "bg-forest",
  success: "bg-success",
  warning: "bg-warning",
  error: "bg-danger",
};

export function AgentMessage({
  message,
  onJump,
}: {
  message: AgentMessageData;
  onJump?: (target: JumpTarget) => void;
}) {
  const [open, setOpen] = useState(false);
  const details = message.details ?? [];
  const jumps = message.jumps ?? [];

  return (
    <div className="flex gap-3">
      <div className="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-full border border-hairline bg-paper">
        <Compass className="h-3.5 w-3.5 text-forest" aria-hidden />
      </div>
      <div className="min-w-0 space-y-1.5">
        <div className="flex items-center gap-2">
          <span className={cn("h-1.5 w-1.5 rounded-full", dotClasses[message.tone])} aria-hidden />
          <span className="text-[11px] uppercase tracking-[0.16em] text-muted-foreground">
            {stageLabels[message.stage]}
          </span>
        </div>
        <p className={cn("max-w-[65ch] text-[15px] leading-relaxed", toneClasses[message.tone])}>{message.text}</p>
        {jumps.length > 0 && (
          <div className="flex flex-wrap gap-2">
            {jumps.map((jump) => (
              <button
                key={`${jump.day}-${jump.label}`}
                type="button"
                onClick={() => onJump?.(jump)}
                className="min-h-11 rounded-md border border-hairline bg-paper/60 px-2.5 text-left text-xs text-forest hover:border-forest/40"
              >
                {jump.label}
              </button>
            ))}
          </div>
        )}
        {details.length > 0 && (
          <div>
            <button
              type="button"
              aria-expanded={open}
              onClick={() => setOpen((value) => !value)}
              className="inline-flex min-h-11 items-center gap-1 text-xs text-muted-foreground hover:text-ink"
            >
              细节
              <ChevronDown className={cn("h-3.5 w-3.5 transition-transform", open && "rotate-180")} />
            </button>
            {open && (
              <ul className="mt-1 space-y-1 border-l border-hairline pl-3 text-xs text-muted-foreground">
                {details.map((detail) => (
                  <li key={detail}>{detail}</li>
                ))}
              </ul>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
