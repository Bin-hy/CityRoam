import { useEffect, useRef, useState } from "react";
import { Send, Square } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/utils";
import type { SessionAction } from "./useSession";

export type ComposerMode =
  | { kind: "initial" }
  | { kind: "supplement"; missing: string[] }
  | { kind: "revise"; rejected: boolean }
  | { kind: "adjust"; baseVersion: number }
  | { kind: "locked"; reason: "research_choice" }
  | { kind: "running" }
  | { kind: "disabled"; reason: string };

const MAX_LENGTH = 2000;

const placeholders: Record<"initial" | "supplement" | "revise" | "adjust", string> = {
  initial: "描述城市、天数、预算和偏好…",
  supplement: "补充缺失的信息…",
  revise: "描述希望怎么修改当前方案…",
  adjust: "描述想调整哪一天、哪些安排…",
};

const stageLabels: Record<string, string> = {
  understand: "理解需求",
  research: "研究攻略与住宿",
  draft: "排程",
  validate: "校验",
  repair: "修复",
  present: "生成方案",
  proposal: "生成调整提案",
  adjust: "调整行程",
};

export interface ComposerProps {
  mode: ComposerMode;
  pending?: SessionAction | null;
  error?: string | null;
  stage?: string;
  prefill?: string;
  focusToken?: number | string;
  onSubmit: (text: string) => Promise<boolean> | boolean;
  onStop?: () => void;
}

export function Composer({
  mode,
  pending = null,
  error,
  stage,
  prefill,
  focusToken,
  onSubmit,
  onStop,
}: ComposerProps) {
  const [value, setValue] = useState(prefill ?? "");
  const [submitting, setSubmitting] = useState(false);
  const inputRef = useRef<HTMLTextAreaElement | null>(null);

  useEffect(() => {
    if (prefill !== undefined) setValue(prefill);
  }, [prefill]);

  useEffect(() => {
    if (focusToken !== undefined) inputRef.current?.focus();
  }, [focusToken]);

  if (mode.kind === "running") {
    return (
      <div className="rounded-md border border-hairline bg-card px-3 py-3">
        <div className="flex items-center justify-between gap-3">
          <p role="status" className="flex items-center gap-2 text-sm text-ink">
            <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-forest" aria-hidden />
            正在{stageLabels[stage ?? ""] ?? "处理"}…
          </p>
          <Button type="button" size="sm" variant="outline" onClick={onStop} disabled={pending === "cancel"}>
            <Square className="h-3.5 w-3.5" aria-hidden />
            {pending === "cancel" ? "取消中…" : "停止"}
          </Button>
        </div>
        {error && <p className="mt-2 text-xs text-danger">{error}</p>}
      </div>
    );
  }

  if (mode.kind === "locked" || mode.kind === "disabled") {
    const hint = mode.kind === "locked" ? "请在上方选择住宿，或选择不计住宿。" : mode.reason;
    return (
      <div className="space-y-1.5 rounded-md border border-hairline bg-card p-3">
        <Textarea aria-label="行程输入" rows={2} disabled value="" placeholder={hint} />
        <div className="flex items-center justify-between gap-2">
          <span className="text-xs text-muted-foreground">{hint}</span>
          <Button type="button" size="sm" disabled>
            发送
          </Button>
        </div>
        {error && <p className="text-xs text-danger">{error}</p>}
      </div>
    );
  }

  const busy = pending !== null || submitting;
  const rows = Math.min(4, Math.max(1, value.split("\n").length));

  const submit = async () => {
    const text = value.trim();
    if (!text || busy) return;
    setSubmitting(true);
    try {
      const ok = await Promise.resolve(onSubmit(text));
      if (ok) setValue("");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="space-y-1.5">
      <div
        className={cn(
          "rounded-md border border-hairline bg-card p-2 transition-colors focus-within:border-forest/50",
        )}
      >
        <Textarea
          ref={inputRef}
          aria-label="行程输入"
          rows={rows}
          value={value}
          maxLength={MAX_LENGTH}
          placeholder={placeholders[mode.kind]}
          className="min-h-0 resize-none border-0 bg-transparent p-1 text-[15px] shadow-none focus-visible:ring-0"
          onChange={(event) => setValue(event.target.value)}
          onKeyDown={(event) => {
            if (event.key !== "Enter" || event.shiftKey) return;
            if (event.nativeEvent.isComposing) return;
            event.preventDefault();
            void submit();
          }}
        />
        <div className="flex items-center justify-between gap-2 px-1">
          <span className="text-[11px] tabular-nums text-muted-foreground">
            {value.length}/{MAX_LENGTH}
          </span>
          <Button type="button" size="sm" disabled={busy || !value.trim()} onClick={() => void submit()}>
            <Send className="h-3.5 w-3.5" aria-hidden />
            {submitting || pending ? "发送中…" : "发送"}
          </Button>
        </div>
      </div>
      {mode.kind === "supplement" && (
        <p className="px-1 text-xs text-warning">请补充：{mode.missing.join("、")}</p>
      )}
      {mode.kind === "revise" && mode.rejected && (
        <p className="px-1 text-xs text-muted-foreground">已拒绝提案，当前版本保持不变。</p>
      )}
      {error && (
        <p role="alert" className="px-1 text-xs text-danger">
          {error}
        </p>
      )}
    </div>
  );
}
