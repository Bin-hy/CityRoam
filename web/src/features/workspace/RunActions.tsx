import { useState, type FormEvent } from "react";
import { useMutation } from "@tanstack/react-query";
import { ApiError } from "@/api/client";
import { createAdjustment, resumeRun } from "@/api/trips";
import type { AdjustmentRequest, ResumeRequest } from "@/api/types";
import { Alert } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Textarea } from "@/components/ui/textarea";
import { MissingFieldsForm } from "@/features/constraints/MissingFieldsForm";
import type { ResearchQuestion, ResearchOption } from "@/features/research/types";
import type { RunStatus } from "@/features/trace/traceReducer";

const MAX_ADJUSTMENT = 2000;

function text(value: unknown): string {
  return typeof value === "string" ? value : "";
}

function warnings(value: unknown): string[] {
  return Array.isArray(value) ? value.filter((item): item is string => typeof item === "string") : [];
}

function errorMessage(error: unknown): string | null {
  if (!error) return null;
  if (error instanceof ApiError) return error.message;
  return error instanceof Error ? error.message : "操作失败，请重试。";
}

export interface RunActionsProps {
  tripID: string;
  runID: string | null;
  status: RunStatus;
  missing: string[];
  proposal: Record<string, unknown> | null;
  researchQuestion?: ResearchQuestion | null;
  baseVersion: number | null;
  onRunStarted: (runID: string) => void;
  onActionComplete: () => void;
}

export function RunActions({
  tripID,
  runID,
  status,
  missing,
  proposal,
  researchQuestion = null,
  baseVersion,
  onRunStarted,
  onActionComplete,
}: RunActionsProps) {
  const [adjustment, setAdjustment] = useState("");

  const resume = useMutation({
    mutationFn: (body: ResumeRequest) => resumeRun(runID as string, body),
    onSuccess: () => onActionComplete(),
  });
  const adjust = useMutation({
    mutationFn: (body: AdjustmentRequest) => createAdjustment(tripID, body),
    onSuccess: (action) => {
      setAdjustment("");
      onRunStarted(action.run_id);
      onActionComplete();
    },
  });

  if (!runID) return null;

  const error = errorMessage(resume.error ?? adjust.error);
  const busy = resume.isPending || adjust.isPending;

  function submitAdjustment(event: FormEvent) {
    event.preventDefault();
    const input = adjustment.trim();
    if (!input || baseVersion === null) return;
    adjust.mutate({ input, base_version: baseVersion });
  }

  if (status === "awaiting_user") {
    if (researchQuestion) {
      return (
        <Card>
          <CardHeader>
            <CardTitle>确认住宿</CardTitle>
            <CardDescription>{researchQuestion.message || "请选择要使用的酒店，或继续但不计住宿。"}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-2">
            {researchQuestion.options.map((option) => (
              <ResearchOptionCard
                key={option.id}
                option={option}
                disabled={busy}
                onSelect={() =>
                  resume.mutate({
                    action: "resolve_research",
                    research_choice: option.hotel
                      ? { option_id: option.id, hotel: option.hotel }
                      : { option_id: option.id },
                  })
                }
              />
            ))}
            {error && (
              <Alert variant="destructive" className="text-sm">
                {error}
              </Alert>
            )}
          </CardContent>
        </Card>
      );
    }

    if (missing.length > 0) {
      return (
        <Card>
          <CardHeader>
            <CardTitle>补充信息</CardTitle>
            <CardDescription>Agent 需要这些信息才能继续规划。</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <MissingFieldsForm
              missing={missing}
              submitting={resume.isPending}
              onSubmit={(supplement) => resume.mutate({ action: "provide_constraints", input: supplement })}
            />
            {error && (
              <Alert variant="destructive" className="text-sm">
                {error}
              </Alert>
            )}
          </CardContent>
        </Card>
      );
    }

    const summary = text(proposal?.summary);
    const impact = text(proposal?.impact);
    const notes = warnings(proposal?.warnings);

    return (
      <Card>
        <CardHeader>
          <CardTitle>{proposal ? "调整提案" : "等待你的确认"}</CardTitle>
          <CardDescription>
            {proposal ? "确认后生成新版本；拒绝则保留当前版本。" : "接受后保存为最终版本，拒绝则取消本次运行。"}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          {summary && <p className="text-sm font-medium">{summary}</p>}
          {impact && <p className="text-xs text-muted-foreground">{impact}</p>}
          {notes.length > 0 && (
            <ul className="list-disc space-y-0.5 pl-5 text-xs text-amber-700">
              {notes.map((note) => (
                <li key={note}>{note}</li>
              ))}
            </ul>
          )}
          <div className="flex flex-wrap gap-2">
            <Button size="sm" disabled={busy} onClick={() => resume.mutate({ action: "accept" })}>
              {resume.isPending ? "提交中…" : proposal ? "接受调整" : "接受方案"}
            </Button>
            <Button size="sm" variant="outline" disabled={busy} onClick={() => resume.mutate({ action: "reject" })}>
              拒绝
            </Button>
          </div>
          {error && (
            <Alert variant="destructive" className="text-sm">
              {error}
            </Alert>
          )}
        </CardContent>
      </Card>
    );
  }

  if (status === "completed" && baseVersion !== null) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>调整行程</CardTitle>
          <CardDescription>用自然语言描述改动，Agent 会先给出提案。</CardDescription>
        </CardHeader>
        <CardContent>
          <form className="space-y-2" onSubmit={submitAdjustment}>
            <Textarea
              aria-label="调整需求"
              value={adjustment}
              maxLength={MAX_ADJUSTMENT}
              placeholder="例如：第二天下午轻松一点，减少步行。"
              onChange={(event) => setAdjustment(event.target.value)}
            />
            <div className="flex items-center justify-between gap-2">
              <span className="text-xs text-muted-foreground">
                {adjustment.length}/{MAX_ADJUSTMENT}
              </span>
              <Button type="submit" size="sm" disabled={busy || !adjustment.trim()}>
                {adjust.isPending ? "提交中…" : "提交调整"}
              </Button>
            </div>
          </form>
          {error && (
            <Alert variant="destructive" className="mt-2 text-sm">
              {error}
            </Alert>
          )}
        </CardContent>
      </Card>
    );
  }

  return null;
}

function ResearchOptionCard({
  option,
  disabled,
  onSelect,
}: {
  option: ResearchOption;
  disabled: boolean;
  onSelect: () => void;
}) {
  const hotel = option.hotel;
  const booking = safeURL(hotel?.booking_url);
  return (
    <div className="rounded-md border p-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <p className="text-sm font-medium">{option.label}</p>
          {option.description && <p className="text-xs text-muted-foreground">{option.description}</p>}
        </div>
        <Button
          size="sm"
          variant="outline"
          aria-label={`选择 ${option.label}`}
          disabled={disabled}
          onClick={onSelect}
        >
          选择
        </Button>
      </div>
      {hotel && (
        <div className="mt-2 space-y-1 text-xs text-muted-foreground">
          {hotel.name && <p className="text-sm text-foreground">{hotel.name}</p>}
          {hotel.address && <p>{hotel.address}</p>}
          <p>
            {(hotel.price_per_night_yuan ?? 0) > 0 ? `¥${hotel.price_per_night_yuan}/晚` : "价格未知"}
            {typeof hotel.score === "number" ? ` · ${hotel.score} 分` : ""}
          </p>
          {booking && (
            <a className="text-primary underline" href={hotel.booking_url} target="_blank" rel="noopener noreferrer">
              查看预订
            </a>
          )}
        </div>
      )}
    </div>
  );
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
