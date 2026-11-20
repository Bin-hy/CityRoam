import { useEffect, useRef, useState, type ReactNode } from "react";
import { AlertTriangle, ArrowDown, CheckCircle2, XCircle } from "lucide-react";
import type { ResearchChoice } from "@/api/types";
import { Button } from "@/components/ui/button";
import type { RunStatus } from "./deriveRunView";
import { AgentMessage } from "./AgentMessage";
import { DecisionCard } from "./DecisionCard";
import { PlanCard } from "./PlanCard";
import { ToolCard } from "./ToolCard";
import { UserMessage } from "./UserMessage";
import type { ConversationItem, SystemMessage } from "./deriveConversation";
import type { JumpTarget } from "./narrate";
import type { SessionAction } from "./useSession";

const statusLabels: Record<RunStatus, string> = {
  idle: "待开始",
  queued: "排队中",
  running: "进行中",
  awaiting_user: "等待确认",
  completed: "已完成",
  failed: "失败",
  cancelled: "已取消",
};

export interface ConversationRailProps {
  city: string;
  days: number;
  status: RunStatus;
  connected: boolean;
  items: ConversationItem[];
  notices?: SystemMessage[];
  composer: ReactNode;
  pending?: SessionAction | null;
  error?: string | null;
  onJump?: (target: JumpTarget) => void;
  onOpenCanvas?: () => void;
  showCanvasEntry?: boolean;
  onSupplement: (input: string) => void;
  onSelectResearch: (choice: ResearchChoice) => void;
  onAccept: () => void;
  onReject: () => void;
  onCancel: () => void;
}

export function ConversationRail({
  city,
  days,
  status,
  connected,
  items,
  notices = [],
  composer,
  pending = null,
  error,
  onJump,
  onOpenCanvas,
  showCanvasEntry = true,
  onSupplement,
  onSelectResearch,
  onAccept,
  onReject,
  onCancel,
}: ConversationRailProps) {
  const all: ConversationItem[] = [...items, ...notices];
  const threadRef = useRef<HTMLDivElement | null>(null);
  const pinned = useRef(true);
  const count = useRef(all.length);
  const [showNew, setShowNew] = useState(false);

  useEffect(() => {
    const node = threadRef.current;
    if (!node) return;
    if (pinned.current) {
      node.scrollTop = node.scrollHeight;
      setShowNew(false);
    } else if (all.length > count.current) {
      setShowNew(true);
    }
    count.current = all.length;
  }, [all.length]);

  const latestAgent = [...all].reverse().find((item) => item.kind === "agent");
  const active = status === "running" || status === "awaiting_user" || status === "queued";

  return (
    <div className="flex h-full min-h-0 flex-col bg-paper">
      <header className="flex min-h-14 flex-wrap items-center justify-between gap-2 border-b border-hairline px-4 py-2">
        <div className="min-w-0">
          <p className="truncate font-serif text-base text-ink">{city || "新的行程"}</p>
          <p className="text-[11px] text-muted-foreground">{days > 0 ? `${days} 天` : "等待需求"}</p>
        </div>
        <div className="flex items-center gap-2">
          {showCanvasEntry && onOpenCanvas && (
            <Button size="sm" variant="outline" className="lg:hidden" onClick={onOpenCanvas}>
              查看方案
            </Button>
          )}
          <span className="rounded-full border border-hairline px-2 py-0.5 text-[11px] text-ink/70">
            {statusLabels[status]}
          </span>
          <span
            className="inline-flex items-center gap-1 text-[11px] text-muted-foreground"
            title={connected ? "事件流已连接" : "事件流未连接"}
          >
            <span
              aria-hidden
              className={`h-1.5 w-1.5 rounded-full ${connected ? "bg-success" : "bg-muted-foreground/50"}`}
            />
            {connected ? "已连接" : "未连接"}
          </span>
        </div>
      </header>

      <div
        ref={threadRef}
        role="log"
        aria-live="polite"
        aria-relevant="additions"
        aria-label="对话记录"
        onScroll={(event) => {
          const node = event.currentTarget;
          pinned.current = node.scrollHeight - node.scrollTop - node.clientHeight < 80;
          if (pinned.current) setShowNew(false);
        }}
        className="relative min-h-0 flex-1 space-y-4 overflow-y-auto px-4 py-4"
      >
        {all.length === 0 && (
          <p className="pt-6 text-center text-sm text-muted-foreground">描述一下你想怎么玩，我会先理解需求，再研究和排程。</p>
        )}
        {all.map((item) => renderItem(item))}
      </div>

      {showNew && (
        <div className="relative">
          <Button
            type="button"
            size="sm"
            variant="outline"
            className="absolute -top-12 left-1/2 -translate-x-1/2 shadow-sm"
            onClick={() => {
              const node = threadRef.current;
              if (node) node.scrollTop = node.scrollHeight;
              pinned.current = true;
              setShowNew(false);
            }}
          >
            <ArrowDown className="h-3.5 w-3.5" aria-hidden />
            新消息
          </Button>
        </div>
      )}

      <div className="border-t border-hairline px-4 py-3">
        {active && !connected && (
          <p role="status" className="mb-2 rounded-md border border-warning/30 bg-warning/5 px-3 py-1.5 text-xs text-warning">
            连接中断，正在重试…已有内容仍然可以查看。
          </p>
        )}
        {composer}
      </div>

      <p className="sr-only" aria-live="polite">
        {latestAgent && latestAgent.kind === "agent" ? latestAgent.text : ""}
      </p>
    </div>
  );

  function renderItem(item: ConversationItem) {
    switch (item.kind) {
      case "user":
        return <UserMessage key={item.id} message={item} />;
      case "agent":
        return <AgentMessage key={item.id} message={item} onJump={onJump} />;
      case "tool":
        return <ToolCard key={item.id} message={item} />;
      case "decision":
        return (
          <DecisionCard
            key={item.id}
            message={item}
            pending={pending}
            error={error}
            onSupplement={onSupplement}
            onSelectResearch={onSelectResearch}
            onAccept={onAccept}
            onReject={onReject}
            onCancel={onCancel}
          />
        );
      case "plan":
        return (
          <PlanCard
            key={item.id}
            message={item}
            onOpenCanvas={showCanvasEntry ? onOpenCanvas : undefined}
          />
        );
      case "system":
        return (
          <div key={item.id} className="flex items-center gap-2 text-xs text-muted-foreground">
            <CheckCircle2 className="h-3.5 w-3.5 text-success" aria-hidden />
            {item.text}
          </div>
        );
      case "error":
        return (
          <div key={item.id} className="flex items-start gap-2 rounded-md border border-danger/30 bg-danger/5 px-3 py-2 text-xs text-danger">
            <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" aria-hidden />
            <span>
              {item.message}
              {!item.retryable && (
                <span className="ml-2 inline-flex items-center gap-1 text-muted-foreground">
                  <XCircle className="h-3 w-3" aria-hidden />
                  不会自动重试
                </span>
              )}
            </span>
          </div>
        );
    }
  }
}
