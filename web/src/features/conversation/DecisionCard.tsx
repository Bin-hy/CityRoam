import { AlertTriangle, Check, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { MissingFieldsForm } from "@/features/constraints/MissingFieldsForm";
import type { ResearchChoice } from "@/api/types";
import type { DecisionMessage } from "./deriveConversation";
import type { SessionAction } from "./useSession";

export interface DecisionCardProps {
  message: DecisionMessage;
  pending?: SessionAction | null;
  error?: string | null;
  onSupplement: (input: string) => void;
  onSelectResearch: (choice: ResearchChoice) => void;
  onAccept: () => void;
  onReject: () => void;
  onCancel: () => void;
}

export function DecisionCard({
  message,
  pending = null,
  error,
  onSupplement,
  onSelectResearch,
  onAccept,
  onReject,
  onCancel,
}: DecisionCardProps) {
  const busy = pending !== null;
  const label = pendingLabel(pending);

  if (message.state !== "open") {
    const cancelled = message.state === "cancelled";
    return (
      <div className="flex items-start gap-2 rounded-md border border-hairline bg-paper/60 px-3 py-2 text-xs text-muted-foreground">
        {cancelled ? (
          <X className="mt-0.5 h-3.5 w-3.5 shrink-0 text-danger" aria-hidden />
        ) : (
          <Check className="mt-0.5 h-3.5 w-3.5 shrink-0 text-success" aria-hidden />
        )}
        <span>{message.summary || (cancelled ? "已取消。" : "已完成。")}</span>
      </div>
    );
  }

  if (message.payload.decision === "missing") {
    const missing = message.payload.missing;
    return (
      <section aria-label="补充信息" className="rounded-md border border-warning/30 bg-warning/5 p-3">
        <p className="mb-2 text-sm font-medium text-ink">还差一点信息</p>
        <MissingFieldsForm
          missing={missing}
          submitting={busy}
          onSubmit={(supplement) => onSupplement(supplement)}
        />
        {error && <p className="mt-2 text-xs text-danger">{error}</p>}
      </section>
    );
  }

  if (message.payload.decision === "lodging") {
    const question = message.payload.question;
    return (
      <section aria-label="选择住宿" className="space-y-2 rounded-md border border-hairline bg-paper/50 p-3">
        <p className="text-sm font-medium text-ink">{question.message || "请选择住宿，或继续但不计住宿。"}</p>
        <ul className="space-y-2">
          {question.options.map((option) => {
            const hotel = option.hotel;
            const booking = safeURL(hotel?.booking_url);
            return (
              <li key={option.id} className="flex flex-wrap items-start justify-between gap-2 border-t border-hairline/70 pt-2 first:border-0 first:pt-0">
                <div className="min-w-0 space-y-0.5 text-xs">
                  <p className="text-sm text-ink">{option.label}</p>
                  {option.description && <p className="text-muted-foreground">{option.description}</p>}
                  {hotel && (
                    <>
                      {hotel.address && <p className="text-muted-foreground">{hotel.address}</p>}
                      <p className="tabular-nums text-muted-foreground">
                        {(hotel.price_per_night_yuan ?? 0) > 0
                          ? `¥${hotel.price_per_night_yuan}/晚`
                          : "价格未知"}
                        {typeof hotel.score === "number" ? ` · ${hotel.score} 分` : ""}
                      </p>
                      {booking && (
                        <a
                          href={hotel.booking_url}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="inline-block text-forest underline decoration-hairline underline-offset-2"
                        >
                          查看预订
                        </a>
                      )}
                    </>
                  )}
                </div>
                <Button
                  size="sm"
                  variant="outline"
                  disabled={busy}
                  aria-label={`选择 ${option.label}`}
                  onClick={() =>
                    onSelectResearch(
                      option.hotel
                        ? { option_id: option.id, hotel: option.hotel }
                        : { option_id: option.id },
                    )
                  }
                >
                  {busy ? label : "选择"}
                </Button>
              </li>
            );
          })}
        </ul>
        {error && <p className="text-xs text-danger">{error}</p>}
      </section>
    );
  }

  const proposal = message.payload.decision === "proposal" ? message.payload.proposal : null;
  return (
    <section aria-label={proposal ? "调整提案" : "确认方案"} className="space-y-2 rounded-md border border-forest/25 bg-forest/[0.04] p-3">
      <p className="text-sm font-medium text-ink">
        {proposal ? "调整提案" : "方案已就绪，等待你的确认"}
      </p>
      {proposal ? (
        <>
          {proposal.summary && <p className="text-sm text-ink">{proposal.summary}</p>}
          {proposal.impact && <p className="text-xs text-muted-foreground">{proposal.impact}</p>}
          {proposal.warnings.length > 0 && (
            <ul className="space-y-0.5 text-xs text-warning">
              {proposal.warnings.map((warning) => (
                <li key={warning} className="flex gap-1">
                  <AlertTriangle className="mt-0.5 h-3 w-3 shrink-0" aria-hidden />
                  {warning}
                </li>
              ))}
            </ul>
          )}
          <p className="text-xs text-muted-foreground">拒绝后会保留当前版本，可以继续描述修改。</p>
        </>
      ) : (
        <p className="text-xs text-muted-foreground">接受后保存为正式版本；拒绝可以继续描述修改。</p>
      )}
      <div className="flex flex-wrap gap-2">
        <Button size="sm" disabled={busy} onClick={onAccept}>
          {busy ? label : proposal ? "接受调整" : "接受方案"}
        </Button>
        <Button size="sm" variant="outline" disabled={busy} onClick={onReject}>
          拒绝
        </Button>
        <Button size="sm" variant="ghost" disabled={busy} onClick={onCancel}>
          取消
        </Button>
      </div>
      {error && <p className="text-xs text-danger">{error}</p>}
    </section>
  );
}

function pendingLabel(pending: SessionAction | null): string {
  switch (pending) {
    case "accept":
      return "接受中…";
    case "reject":
      return "处理中…";
    case "cancel":
      return "取消中…";
    case "resolveResearch":
    case "supplement":
      return "提交中…";
    default:
      return "处理中…";
  }
}

function safeURL(value: string | undefined): value is string {
  if (!value) return false;
  try {
    const url = new URL(value);
    return url.protocol === "http:" || url.protocol === "https:";
  } catch {
    return false;
  }
}
