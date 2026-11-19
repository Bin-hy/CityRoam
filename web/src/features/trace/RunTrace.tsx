import { AlertTriangle, CheckCircle2, Circle, Loader2, Wrench, XCircle } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { stageLabels, type TraceEntry, type TraceState } from "./traceReducer";

const statusLabels: Record<TraceState["status"], string> = {
  idle: "等待开始",
  queued: "排队中",
  running: "运行中",
  awaiting_user: "待确认",
  completed: "已完成",
  failed: "失败",
  cancelled: "已取消",
};

export function RunTrace({ state, connected }: { state: TraceState; connected?: boolean }) {
  const blocking = state.violations.filter((violation) => violation.level === "blocking");
  const warnings = state.violations.filter((violation) => violation.level !== "blocking");

  return (
    <section className="flex h-full min-h-0 flex-col rounded-xl border bg-card">
      <header className="space-y-2 border-b p-4">
        <div className="flex items-center justify-between gap-2">
          <h2 className="text-sm font-semibold">运行轨迹</h2>
          <span className="flex items-center gap-1 text-xs text-muted-foreground">
            <span
              className={cn("h-2 w-2 rounded-full", connected ? "bg-emerald-500" : "bg-muted-foreground/40")}
              aria-hidden
            />
            {statusLabels[state.status]}
          </span>
        </div>
        <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
          <Badge variant={state.status === "failed" ? "destructive" : "secondary"}>
            {stageLabels[state.stage] ?? state.stage}
          </Badge>
          <span>修复轮次 {state.round}</span>
          <span>事件 {state.lastSeq}</span>
        </div>
      </header>

      {state.error && (
        <p className="border-b bg-destructive/5 p-3 text-sm text-destructive">{state.error.message}</p>
      )}

      {(blocking.length > 0 || warnings.length > 0) && (
        <div className="space-y-2 border-b p-4">
          {state.violations.map((violation, index) => (
            <div key={`${violation.code}-${violation.day}-${index}`} className="flex gap-2 text-xs">
              {violation.level === "blocking" ? (
                <XCircle className="mt-0.5 h-3.5 w-3.5 shrink-0 text-destructive" />
              ) : (
                <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0 text-amber-500" />
              )}
              <p className="text-muted-foreground">
                {violation.day > 0 && <span className="text-foreground">第 {violation.day} 天：</span>}
                {violation.message}
                {violation.suggestion && <span className="block text-foreground/70">{violation.suggestion}</span>}
              </p>
            </div>
          ))}
        </div>
      )}

      <ol className="min-h-0 flex-1 space-y-3 overflow-y-auto p-4">
        {state.entries.length === 0 && (
          <li className="flex items-center gap-2 text-sm text-muted-foreground">
            <Loader2 className="h-4 w-4 animate-spin" />
            等待 Agent 事件…
          </li>
        )}
        {state.entries.map((entry) => (
          <li key={entry.seq} className="flex gap-2">
            <EntryIcon entry={entry} />
            <div className="min-w-0 space-y-0.5">
              <p className="text-sm leading-tight">{entry.title}</p>
              {entry.detail && <p className="truncate text-xs text-muted-foreground">{entry.detail}</p>}
              <p className="text-[10px] text-muted-foreground/70">#{entry.seq}</p>
            </div>
          </li>
        ))}
      </ol>
    </section>
  );
}

function EntryIcon({ entry }: { entry: TraceEntry }) {
  switch (entry.level) {
    case "success":
      return <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-emerald-500" />;
    case "warning":
      return <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-amber-500" />;
    case "error":
      return <XCircle className="mt-0.5 h-4 w-4 shrink-0 text-destructive" />;
    default:
      return entry.type === "repair.started" ? (
        <Wrench className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
      ) : (
        <Circle className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground/50" />
      );
  }
}
