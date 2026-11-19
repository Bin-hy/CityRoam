import { useState } from "react";
import type { PlanVersion } from "@/api/types";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

const statusLabels: Record<string, string> = {
  accepted: "当前",
  superseded: "历史",
  draft: "草稿",
};

function versionDate(version: PlanVersion): string {
  const created = new Date(version.created_at);
  if (Number.isNaN(created.getTime())) return "";
  return `${created.getMonth() + 1} 月 ${created.getDate()} 日`;
}

export interface VersionSwitcherProps {
  versions: PlanVersion[];
  selectedSeq: number | null;
  currentSeq: number | null;
  onSelect: (seq: number) => void;
  onRestore: (seq: number) => void;
  restoring?: boolean;
  error?: string | null;
}

export function VersionSwitcher({
  versions,
  selectedSeq,
  currentSeq,
  onSelect,
  onRestore,
  restoring = false,
  error,
}: VersionSwitcherProps) {
  const [confirmOpen, setConfirmOpen] = useState(false);
  const canRestore = selectedSeq !== null && selectedSeq !== currentSeq;

  return (
    <div className="space-y-3">
      <label className="grid gap-1 text-sm">
        <span className="text-muted-foreground">版本</span>
        <select
          aria-label="版本"
          className="h-9 rounded-md border bg-background px-2 text-sm"
          value={selectedSeq ?? ""}
          onChange={(event) => onSelect(Number(event.target.value))}
        >
          {versions.map((version) => (
            <option key={version.seq} value={version.seq}>
              v{version.seq} · {statusLabels[version.status] ?? version.status}
              {versionDate(version) ? ` · ${versionDate(version)}` : ""}
            </option>
          ))}
        </select>
      </label>

      {canRestore && (
        <Button size="sm" variant="outline" onClick={() => setConfirmOpen(true)} disabled={restoring}>
          回退到此版本
        </Button>
      )}
      {error && (
        <p role="alert" className="text-xs text-destructive">
          {error}
        </p>
      )}

      <Dialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>回退到 v{selectedSeq}</DialogTitle>
            <DialogDescription>
              当前方案会被新版本 v{(versions[0]?.seq ?? 0) + 1} 替代，历史版本仍会保留，可随时再切回。
            </DialogDescription>
          </DialogHeader>
          <div className="flex justify-end gap-2">
            <Button variant="outline" onClick={() => setConfirmOpen(false)}>
              取消
            </Button>
            <Button
              onClick={() => {
                setConfirmOpen(false);
                if (selectedSeq !== null) onRestore(selectedSeq);
              }}
              disabled={restoring}
            >
              {restoring ? "回退中…" : "确认回退"}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
