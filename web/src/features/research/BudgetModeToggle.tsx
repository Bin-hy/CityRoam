import { useEffect, useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { ApiError } from "@/api/client";
import { replanTrip } from "@/api/trips";
import type { ReplanRequest } from "@/api/types";
import { Alert } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { newClientRequestID } from "@/lib/id";

export interface BudgetModeToggleProps {
  tripID: string;
  baseVersion: number;
  constraints: Record<string, unknown>;
  lodgingMentioned: boolean;
  hasLodging: boolean;
  includeLodging: boolean;
  onRunStarted: (runID: string) => void;
  onReplanned?: () => void;
}

export function BudgetModeToggle({
  tripID,
  baseVersion,
  constraints,
  lodgingMentioned,
  hasLodging,
  includeLodging,
  onRunStarted,
  onReplanned,
}: BudgetModeToggleProps) {
  const [checked, setChecked] = useState(includeLodging);
  const mutation = useMutation({
    mutationFn: (next: boolean) => replanTrip(tripID, replanBody(baseVersion, constraints, next)),
    onMutate: (next) => setChecked(next),
    onError: () => setChecked(includeLodging),
    onSuccess: (action) => {
      onRunStarted(action.run_id);
      onReplanned?.();
    },
  });

  useEffect(() => {
    if (!mutation.isPending) setChecked(includeLodging);
  }, [includeLodging, mutation.isPending]);

  if (!lodgingMentioned && !hasLodging) return null;

  const error = mutation.error
    ? mutation.error instanceof ApiError
      ? mutation.error.message
      : "切换预算口径失败，请重试。"
    : null;

  return (
    <div className="space-y-2 border-t pt-3">
      <div className="flex items-center justify-between gap-2">
        <div>
          <p className="text-sm font-medium">预算口径</p>
          <p className="text-xs text-muted-foreground">住宿是否计入总预算</p>
        </div>
        <div className="flex items-center gap-2">
          <Badge variant={checked ? "secondary" : "outline"}>{checked ? "已计入" : "未计入"}</Badge>
          <Button
            type="button"
            role="switch"
            aria-checked={checked}
            aria-label="住宿计入预算"
            size="sm"
            variant={checked ? "default" : "outline"}
            disabled={mutation.isPending}
            onClick={() => mutation.mutate(!checked)}
          >
            {mutation.isPending ? "重排中…" : checked ? "计入住宿" : "不计住宿"}
          </Button>
        </div>
      </div>
      {error && (
        <Alert variant="destructive" className="text-xs">
          {error}
        </Alert>
      )}
    </div>
  );
}

function replanBody(baseVersion: number, constraints: Record<string, unknown>, includeLodging: boolean): ReplanRequest {
  const budget =
    constraints.budget !== null && typeof constraints.budget === "object" && !Array.isArray(constraints.budget)
      ? (constraints.budget as Record<string, unknown>)
      : {};
  return {
    base_version: baseVersion,
    constraints: {
      ...constraints,
      budget: { ...budget, include_lodging: includeLodging },
    },
    client_request_id: newClientRequestID(),
  };
}
