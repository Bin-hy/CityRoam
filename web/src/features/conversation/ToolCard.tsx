import { useState, type ReactNode } from "react";
import { AlertTriangle, Check, ChevronDown, Loader2, Wrench } from "lucide-react";
import { cn } from "@/lib/utils";
import { formatYuan } from "@/design/tokens";
import type { ToolMessage } from "./deriveConversation";

const toolLabels: Record<string, string> = {
  search_hotels: "住宿搜索",
  search_travel_notes: "旅行攻略",
  get_weather: "天气查询",
};

export function ToolCard({ message }: { message: ToolMessage }) {
  const [open, setOpen] = useState(false);
  const label = toolLabels[message.name] ?? message.name;
  const statusLabel =
    message.status === "running" ? "进行中" : message.status === "ok" ? "完成" : "失败";

  return (
    <div className="rounded-md border border-hairline bg-card/70">
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
        className="flex min-h-11 w-full items-center gap-2 px-3 py-2 text-left text-xs text-muted-foreground hover:text-ink"
      >
        <Wrench className="h-3.5 w-3.5 shrink-0 text-forest" aria-hidden />
        <span className="shrink-0 font-medium text-ink">{label}</span>
        {message.query && <span className="min-w-0 flex-1 truncate">{message.query}</span>}
        <span
          className={cn(
            "flex shrink-0 items-center gap-1",
            message.status === "error" && "text-danger",
            message.status === "ok" && "text-success",
          )}
        >
          {message.status === "running" && <Loader2 className="h-3 w-3 animate-spin" aria-hidden />}
          {message.status === "ok" && <Check className="h-3 w-3" aria-hidden />}
          {message.status === "error" && <AlertTriangle className="h-3 w-3" aria-hidden />}
          {statusLabel}
        </span>
        {message.latencyMs !== undefined && <span className="shrink-0 tabular-nums">{message.latencyMs}ms</span>}
        {message.resultCount !== undefined && <span className="shrink-0 tabular-nums">{message.resultCount} 条</span>}
        <ChevronDown className={cn("h-3.5 w-3.5 shrink-0 transition-transform", open && "rotate-180")} aria-hidden />
      </button>
      {open && (
        <div className="space-y-2 border-t border-hairline px-3 py-2 text-xs">
          {message.query && (
            <p>
              <span className="text-muted-foreground">查询：</span>
              {message.query}
            </p>
          )}
          <div className="flex flex-wrap gap-1.5">
            {message.cached && <Flag>缓存命中</Flag>}
            {message.coalesced && <Flag>并发合并</Flag>}
            {message.attempts !== undefined && message.attempts > 1 && <Flag>重试 {message.attempts - 1} 次</Flag>}
            {message.preview.length === 0 && message.status === "ok" && (
              <span className="text-muted-foreground">未返回结果</span>
            )}
          </div>
          {message.preview.length > 0 && (
            <ul className="space-y-1">
              {message.preview.map((item) => (
                <li
                  key={`${item.title}-${item.subtitle ?? ""}`}
                  className="flex items-baseline justify-between gap-3 border-b border-hairline/60 pb-1 last:border-0"
                >
                  <span className="min-w-0">
                    <span className="block truncate text-ink">{item.title}</span>
                    {item.subtitle && <span className="block truncate text-muted-foreground">{item.subtitle}</span>}
                  </span>
                  {item.priceYuan !== undefined && (
                    <span className="shrink-0 tabular-nums text-ink">{formatYuan(item.priceYuan)}/晚</span>
                  )}
                </li>
              ))}
            </ul>
          )}
          {message.status === "error" && (
            <p className="text-danger">{message.error?.message ?? "外部工具调用失败，不影响继续。"}</p>
          )}
        </div>
      )}
    </div>
  );
}

function Flag({ children }: { children: ReactNode }) {
  return <span className="rounded-full border border-hairline px-2 py-0.5 text-[11px] text-muted-foreground">{children}</span>;
}
