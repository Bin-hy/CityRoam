import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { getPlanVersion, listPlanVersions, restorePlanVersion } from "@/api/trips";
import type { PlanVersion } from "@/api/types";
import { Alert } from "@/components/ui/alert";
import { Skeleton } from "@/components/ui/skeleton";
import { parseItinerary } from "@/features/itinerary/types";
import { PlanDiff } from "./PlanDiff";
import { VersionSwitcher } from "./VersionSwitcher";
import { diffItineraries } from "./diffModel";

export interface VersionPanelProps {
  tripID: string;
  currentVersion: PlanVersion | null;
  onRestored?: (version: PlanVersion) => void;
}

export function VersionPanel({ tripID, currentVersion, onRestored }: VersionPanelProps) {
  const queryClient = useQueryClient();
  const versionsQuery = useQuery({
    queryKey: ["versions", tripID],
    queryFn: () => listPlanVersions(tripID),
  });
  const versions = useMemo(() => versionsQuery.data?.versions ?? [], [versionsQuery.data]);
  const current = currentVersion ?? versions.find((version) => version.status === "accepted") ?? null;
  const [selectedSeq, setSelectedSeq] = useState<number | null>(null);
  const activeSeq = selectedSeq ?? current?.seq ?? versions[0]?.seq ?? null;

  useEffect(() => {
    if (selectedSeq !== null && versions.length > 0 && !versions.some((version) => version.seq === selectedSeq)) {
      setSelectedSeq(null);
    }
  }, [selectedSeq, versions]);

  const comparing = current !== null && activeSeq !== null && activeSeq !== current.seq;
  const comparison = useQuery({
    queryKey: ["version", tripID, activeSeq],
    queryFn: () => getPlanVersion(tripID, activeSeq as number),
    enabled: comparing,
  });

  const diffDays = useMemo(() => {
    if (!comparing) return [];
    const before = parseItinerary(current?.plan);
    const after = parseItinerary(comparison.data?.plan);
    if (!before || !after) return [];
    return diffItineraries(before, after);
  }, [comparing, current?.plan, comparison.data?.plan]);

  const restore = useMutation({
    mutationFn: (seq: number) => restorePlanVersion(tripID, seq),
    onSuccess: async (restored) => {
      setSelectedSeq(null);
      await queryClient.invalidateQueries({ queryKey: ["versions", tripID] });
      await queryClient.invalidateQueries({ queryKey: ["trip", tripID] });
      onRestored?.(restored);
    },
  });

  if (versionsQuery.isPending) {
    return <Skeleton className="h-20" />;
  }
  if (versionsQuery.isError) {
    return (
      <Alert variant="destructive" className="text-sm">
        版本加载失败，请稍后重试。
      </Alert>
    );
  }
  if (versions.length === 0) {
    return <p className="text-sm text-muted-foreground">尚无版本，方案生成后会出现在这里。</p>;
  }

  return (
    <div className="space-y-3">
      <VersionSwitcher
        versions={versions}
        selectedSeq={activeSeq}
        currentSeq={current?.seq ?? null}
        onSelect={setSelectedSeq}
        onRestore={(seq) => restore.mutate(seq)}
        restoring={restore.isPending}
        error={restore.isError ? (restore.error instanceof Error ? restore.error.message : "回退失败") : null}
      />

      {comparing &&
        (comparison.isPending ? (
          <p className="text-sm text-muted-foreground">加载差异…</p>
        ) : comparison.isError ? (
          <Alert variant="destructive" className="text-sm">
            版本内容加载失败。
          </Alert>
        ) : (
          <PlanDiff days={diffDays} from={current?.seq} to={activeSeq ?? undefined} />
        ))}

      {!comparing && <p className="text-sm text-muted-foreground">当前展示 v{current?.seq ?? activeSeq}，选择其他版本可查看差异。</p>}
    </div>
  );
}
