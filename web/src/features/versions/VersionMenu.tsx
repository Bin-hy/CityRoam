import { useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { ChevronDown } from "lucide-react";
import { listPlanVersions, restorePlanVersion } from "@/api/trips";
import type { PlanVersion } from "@/api/types";
import { Button } from "@/components/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { cn } from "@/lib/utils";
import type { Itinerary } from "@/features/itinerary/types";
import { diffItineraries } from "./diffModel";

export interface VersionMenuProps {
  tripID: string;
  currentSeq: number | null;
  plan: Itinerary | null;
  readOnly?: boolean;
  onViewVersion: (version: PlanVersion) => void;
  onRestored: (version: PlanVersion) => void;
  onReturnCurrent: () => void;
}

const statusLabels: Record<PlanVersion["status"], string> = {
  draft: "草稿",
  accepted: "正式",
  superseded: "历史",
};

export function VersionMenu({
  tripID,
  currentSeq,
  plan,
  readOnly = false,
  onViewVersion,
  onRestored,
  onReturnCurrent,
}: VersionMenuProps) {
  const [open, setOpen] = useState(false);
  const [selected, setSelected] = useState<PlanVersion | null>(null);
  const [confirming, setConfirming] = useState(false);
  const versions = useQuery({
    queryKey: ["plan-versions", tripID],
    queryFn: () => listPlanVersions(tripID),
    enabled: open,
  });
  const restore = useMutation({
    mutationFn: (seq: number) => restorePlanVersion(tripID, seq),
    onSuccess: (restored) => {
      onRestored(restored);
      setConfirming(false);
      setSelected(null);
      setOpen(false);
    },
  });

  if (readOnly) return null;

  const diff = selected && plan ? diffItineraries(plan, selectedPlan(selected)) : [];
  const summary = summarize(diff);
  const error = restore.error instanceof Error ? restore.error.message : null;

  return (
    <Popover
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        if (!next) {
          setConfirming(false);
          setSelected(null);
          restore.reset();
        }
      }}
    >
      <PopoverTrigger asChild>
        <Button size="sm" variant="ghost" className="tabular-nums">
          {currentSeq ? `v${currentSeq}` : "版本"}
          <ChevronDown className="h-3.5 w-3.5" aria-hidden />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-80">
        <div className="space-y-3">
          <p className="font-serif text-sm text-ink">版本</p>
          {versions.isPending && <p className="text-xs text-muted-foreground">加载版本…</p>}
          {versions.isError && (
            <div className="space-y-2">
              <p className="text-xs text-danger">版本加载失败。</p>
              <Button size="sm" variant="outline" onClick={() => void versions.refetch()}>
                重试
              </Button>
            </div>
          )}
          <ul className="max-h-60 space-y-1 overflow-y-auto">
            {(versions.data?.versions ?? []).map((version) => (
              <li key={version.id}>
                <button
                  type="button"
                  onClick={() => {
                    setSelected(version);
                    setConfirming(false);
                    restore.reset();
                    onViewVersion(version);
                  }}
                  className={cn(
                    "w-full rounded-md px-2 py-1.5 text-left text-xs hover:bg-sand/50",
                    selected?.id === version.id && "bg-sand/60",
                    version.seq === currentSeq && "border border-forest/30",
                  )}
                >
                  <span className="flex items-center justify-between gap-2">
                    <span className="font-mono tabular-nums text-ink">v{version.seq}</span>
                    <span className="text-muted-foreground">{statusLabels[version.status]}</span>
                  </span>
                  <span className="mt-0.5 block truncate text-muted-foreground">
                    {formatTime(version.created_at)}
                    {version.reason ? ` · ${version.reason}` : ""}
                  </span>
                </button>
              </li>
            ))}
          </ul>
          {versions.data?.versions.length === 0 && (
            <p className="text-xs text-muted-foreground">还没有保存的版本。</p>
          )}
          {selected && (
            <div className="space-y-2 border-t border-hairline pt-2 text-xs">
              <p className="text-ink">
                对比 v{currentSeq ?? "—"} → v{selected.seq}
              </p>
              <p className="text-muted-foreground">
                {summary.days === 0
                  ? "行程没有可见差异。"
                  : `涉及 ${summary.days} 天 · 新增 ${summary.added} · 调整 ${summary.modified} · 删除 ${summary.removed}`}
              </p>
              {confirming ? (
                <div className="space-y-2">
                  <p className="text-warning">恢复后当前方案将被替换，确认继续？</p>
                  <div className="flex gap-2">
                    <Button
                      size="sm"
                      disabled={restore.isPending}
                      onClick={() => restore.mutate(selected.seq)}
                    >
                      {restore.isPending ? "恢复中…" : "确认恢复"}
                    </Button>
                    <Button size="sm" variant="ghost" onClick={() => setConfirming(false)}>
                      取消
                    </Button>
                  </div>
                </div>
              ) : (
                <div className="flex flex-wrap gap-2">
                  <Button size="sm" variant="outline" onClick={() => setConfirming(true)}>
                    恢复此版本
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => {
                      setSelected(null);
                      onReturnCurrent();
                    }}
                  >
                    返回当前
                  </Button>
                </div>
              )}
              {error && <p className="text-danger">{error}</p>}
            </div>
          )}
        </div>
      </PopoverContent>
    </Popover>
  );
}

function selectedPlan(version: PlanVersion): Itinerary {
  return version.plan as unknown as Itinerary;
}

function summarize(diff: ReturnType<typeof diffItineraries>) {
  let added = 0;
  let removed = 0;
  let modified = 0;
  for (const day of diff) {
    for (const block of day.blocks) {
      if (block.change === "added") added += 1;
      else if (block.change === "removed") removed += 1;
      else modified += 1;
    }
  }
  return { days: diff.length, added, removed, modified };
}

function formatTime(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-CN", { month: "numeric", day: "numeric", hour: "2-digit", minute: "2-digit" });
}
