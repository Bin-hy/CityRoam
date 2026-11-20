import { useEffect, useMemo, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Link, useLocation, useParams } from "react-router-dom";
import { loginURL } from "@/api/auth";
import { ApiError } from "@/api/client";
import type { PlanVersion, ResearchChoice } from "@/api/types";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { ErrorState } from "@/components/StateMessage";
import { useMediaQuery } from "@/hooks/useMediaQuery";
import { BudgetPopover } from "@/features/budget/BudgetPopover";
import { Composer, type ComposerMode } from "@/features/conversation/Composer";
import { ConversationRail } from "@/features/conversation/ConversationRail";
import { deriveCanvas } from "@/features/conversation/deriveCanvas";
import {
  deriveConversation,
  type ConversationItem,
} from "@/features/conversation/deriveConversation";
import { deriveRunView, type RunView } from "@/features/conversation/deriveRunView";
import { asString, type RunEvent } from "@/features/conversation/eventParse";
import type { JumpTarget } from "@/features/conversation/narrate";
import { useSession, type SessionAction } from "@/features/conversation/useSession";
import {
  buildResultModel,
  resolveResultSelection,
  type ResultSelection,
  type ResultViewMode,
} from "@/features/result/model";
import { ResultSheet } from "@/features/result/ResultSheet";
import { ResultShell } from "@/features/result/ResultShell";
import { ShareDialog } from "@/features/share/ShareDialog";
import type { RunStatus } from "@/features/conversation/deriveRunView";
import { useTrip } from "@/features/trips/useTrip";
import { VersionMenu } from "@/features/versions/VersionMenu";
import { hasAMapKey } from "@/lib/mapKey";

const terminalStatuses = new Set<RunStatus>(["completed", "failed", "cancelled"]);

export function WorkspacePage() {
  const { tripId } = useParams();
  const location = useLocation();
  const routeRunID = (location.state as { runId?: string } | null)?.runId ?? null;
  const tripQuery = useTrip(tripId);
  const trip = tripQuery.data;
  const queryClient = useQueryClient();

  // Keep the first run id per trip: trip refetches after adjustments must not
  // reset the session's frozen turns.
  const boot = useRef<{ tripID?: string; runID: string | null }>({
    tripID: tripId,
    runID: routeRunID,
  });
  if (boot.current.tripID !== tripId) {
    boot.current = { tripID: tripId, runID: routeRunID };
  }
  if (trip && !boot.current.runID) {
    boot.current = { tripID: tripId, runID: routeRunID ?? trip.latest_run?.id ?? null };
  }
  const initialRunID = boot.current.runID ?? routeRunID ?? null;

  const session = useSession(tripId ?? "", initialRunID);
  const desktop = useMediaQuery("(min-width: 1024px)");

  const [activeDay, setActiveDay] = useState<number | null>(null);
  const [selection, setSelection] = useState<ResultSelection>(null);
  const [resultView, setResultView] = useState<ResultViewMode>("dossier");
  const [viewVersion, setViewVersion] = useState<PlanVersion | null>(null);
  const [canvasOpen, setCanvasOpen] = useState(false);
  const [prefill, setPrefill] = useState("");
  const [focusToken, setFocusToken] = useState(0);
  const lastAction = useRef<SessionAction | null>(null);

  const view = useMemo(() => deriveRunView(session.events), [session.events]);
  const currentCanvas = useMemo(
    () => deriveCanvas(session.events, trip, null),
    [session.events, trip],
  );
  const canvas = useMemo(
    () => (viewVersion ? deriveCanvas(session.events, trip, viewVersion) : currentCanvas),
    [viewVersion, session.events, trip, currentCanvas],
  );
  const result = useMemo(
    () =>
      buildResultModel({
        itinerary: canvas.model.plan,
        explanations: canvas.model.explanations,
        research: canvas.model.research,
      }),
    [canvas],
  );
  const conversation = useMemo(() => {
    const items: ConversationItem[] = [];
    for (const turn of session.turns) {
      items.push(...deriveConversation(turn.runId, turn.events));
    }
    if (session.activeRunId) {
      items.push(...deriveConversation(session.activeRunId, session.events));
    }
    return items;
  }, [session.turns, session.activeRunId, session.events]);

  const fallbackStatus = (trip?.latest_run?.status ?? "idle") as RunStatus;
  const status: RunStatus =
    view.status !== "idle"
      ? view.status
      : session.activeRunId && session.activeRunId !== trip?.latest_run?.id
        ? "queued"
        : fallbackStatus;

  const baseVersion = currentCanvas.model.versionSeq ?? 0;
  const awaitingStage = useMemo(() => lastAwaitingStage(session.events), [session.events]);
  const composerMode = resolveComposerMode(status, view, awaitingStage, baseVersion);

  const constraints = (trip?.constraints ?? {}) as Record<string, unknown>;
  const budget = asRecord(constraints.budget);
  const includeLodging =
    typeof budget.include_lodging === "boolean" ? budget.include_lodging : true;
  const dayCount = currentCanvas.model.plan?.days.length ?? 0;

  const tripID = trip?.id;
  const refetchTrip = tripQuery.refetch;
  useEffect(() => {
    if (!tripID || !terminalStatuses.has(view.status)) return;
    void refetchTrip();
    void queryClient.invalidateQueries({ queryKey: ["plan-versions", tripID] });
  }, [view.status, tripID, refetchTrip, queryClient]);

  useEffect(() => {
    if (activeDay !== null && dayCount > 0 && activeDay >= dayCount) setActiveDay(null);
  }, [activeDay, dayCount]);

  useEffect(() => {
    if (!selection) return;
    if (
      selection.kind !== "hotel" &&
      activeDay !== null &&
      selection.dayIndex !== activeDay
    ) {
      setSelection(null);
      return;
    }
    if (!result || !resolveResultSelection(result, selection)) setSelection(null);
  }, [selection, activeDay, result]);

  useEffect(() => {
    setViewVersion(null);
  }, [session.activeRunId]);

  useEffect(() => {
    setActiveDay(null);
    setSelection(null);
    setResultView("dossier");
    setViewVersion(null);
    setCanvasOpen(false);
    setPrefill("");
  }, [tripId]);

  if (tripQuery.isPending) {
    return <WorkspaceSkeleton />;
  }

  if (tripQuery.isError || !trip) {
    const unauthorized = tripQuery.error instanceof ApiError && tripQuery.error.status === 401;
    const notFound = tripQuery.error instanceof ApiError && tripQuery.error.status === 404;
    return (
      <main className="mx-auto max-w-xl px-4 py-16">
        <ErrorState
          title={unauthorized ? "会话已过期" : notFound ? "行程不存在" : "行程加载失败"}
          description={
            unauthorized
              ? "登录状态已失效，请重新登录后继续查看这次行程。"
              : notFound
                ? "链接可能已失效，或该行程属于其他账号。"
                : "服务暂时不可用，请稍后重试。"
          }
          onRetry={unauthorized ? undefined : () => void tripQuery.refetch()}
          action={
            unauthorized ? (
              <Button size="sm" asChild>
                <a href={loginURL(`${location.pathname}${location.search}`)}>重新登录</a>
              </Button>
            ) : (
              <Button size="sm" variant="outline" asChild>
                <Link to="/">返回首页</Link>
              </Button>
            )
          }
        />
      </main>
    );
  }

  const tripIDValue = trip.id;
  const cityLabel =
    canvas.model.plan?.city || (trip.city === "beijing" ? "北京" : trip.city) || "";
  const days = canvas.model.plan?.days.length || trip.days || 0;
  const lodgingPending = session.pending === "replanLodging";
  const actionError = lastAction.current === "replanLodging" ? session.error : null;

  function focusComposer() {
    setFocusToken((token) => token + 1);
  }

  function jumpTo(target: JumpTarget) {
    setActiveDay(target.day);
    const day = currentCanvas.model.plan?.days[target.day];
    const block = day
      ? target.blockID
        ? day.blocks.find((item) => item.id === target.blockID)
        : target.poiID
          ? day.blocks.find((item) => item.poi_id === target.poiID)
          : undefined
      : undefined;
    const resultDay = result?.days.find((item) => item.index === target.day);
    const stopID =
      block && resultDay?.stops.some((stop) => stop.id === block.id)
        ? block.id
        : resultDay?.stops[0]?.id;
    setSelection(
      stopID ? { kind: "stop", dayIndex: target.day, stopID } : null,
    );
    focusComposer();
    if (!desktop) setCanvasOpen(true);
  }

  function adjustDay(dayIndex: number) {
    setActiveDay(dayIndex);
    setPrefill(`调整第 ${dayIndex + 1} 天：`);
    setCanvasOpen(false);
    focusComposer();
  }

  function toggleLodging() {
    if (baseVersion <= 0) return;
    lastAction.current = "replanLodging";
    void session.actions
      .replanLodging(
        { ...constraints, budget: { ...budget, include_lodging: !includeLodging } },
        baseVersion,
      )
      .then((ok) => {
        if (ok) lastAction.current = null;
      });
  }

  function viewVersionBy(version: PlanVersion) {
    setViewVersion(version);
    setSelection(null);
  }

  function returnCurrent() {
    setViewVersion(null);
    setSelection(null);
  }

  function handleRestored(version: PlanVersion) {
    setViewVersion(null);
    setSelection(null);
    session.actions.addSystemNotice(`已恢复版本 v${version.seq}。`);
    void tripQuery.refetch();
    void queryClient.invalidateQueries({ queryKey: ["plan-versions", tripIDValue] });
  }

  async function submitComposer(text: string): Promise<boolean> {
    let ok = false;
    if (composerMode.kind === "supplement") {
      lastAction.current = "supplement";
      ok = await session.actions.supplement(text);
    } else if (composerMode.kind === "revise") {
      lastAction.current = "revise";
      ok = await session.actions.revise(text);
    } else if (composerMode.kind === "adjust") {
      lastAction.current = "adjust";
      ok = await session.actions.adjust(text, composerMode.baseVersion);
    }
    if (ok) setPrefill("");
    return ok;
  }

  const composer = (
    <Composer
      mode={composerMode}
      pending={session.pending}
      error={session.error}
      stage={view.stage}
      prefill={prefill}
      focusToken={focusToken}
      onSubmit={submitComposer}
      onStop={() => void session.actions.cancel()}
    />
  );

  const share = trip.latest_version ? <ShareDialog tripID={trip.id} /> : null;

  const moreControls = (
    <>
      <BudgetPopover
        budget={result?.budget ?? null}
        includeLodging={includeLodging}
        pending={lodgingPending}
        onToggleLodging={toggleLodging}
      />
      <VersionMenu
        tripID={trip.id}
        currentSeq={currentCanvas.model.versionSeq}
        plan={currentCanvas.model.plan}
        onViewVersion={viewVersionBy}
        onRestored={handleRestored}
        onReturnCurrent={returnCurrent}
      />
      {share}
    </>
  );

  return (
    <div className="flex h-[calc(100dvh-3.5rem)] min-h-0 flex-col overflow-hidden bg-paper">
      <div className="flex min-h-0 flex-1">
        <aside
          aria-label="对话"
          className="flex min-h-0 w-full shrink-0 flex-col border-hairline lg:w-[360px] lg:border-r xl:w-[400px]"
        >
          <ConversationRail
            city={cityLabel}
            days={days}
            status={status}
            connected={session.connected}
            items={conversation}
            notices={session.notices}
            composer={composer}
            pending={session.pending}
            error={session.error}
            onJump={jumpTo}
            onOpenCanvas={() => setCanvasOpen(true)}
            showCanvasEntry={!desktop}
            onSupplement={(input) => {
              lastAction.current = "supplement";
              void session.actions.supplement(input);
            }}
            onSelectResearch={(choice: ResearchChoice) => {
              lastAction.current = "resolveResearch";
              void session.actions.resolveResearch(choice);
            }}
            onAccept={() => {
              lastAction.current = "accept";
              void session.actions.accept();
            }}
            onReject={() => {
              lastAction.current = "reject";
              void session.actions.reject();
            }}
            onCancel={() => {
              lastAction.current = "cancel";
              void session.actions.cancel();
            }}
          />
        </aside>

        {desktop && (
          <main className="hidden min-h-0 flex-1 overflow-hidden px-4 py-3 lg:block">
            <ResultShell
              model={result}
              view={resultView}
              onViewChange={setResultView}
              activeDay={activeDay}
              onActiveDayChange={setActiveDay}
              selection={selection}
              onSelect={setSelection}
              controls={moreControls}
              actionError={actionError}
              isDraft={canvas.model.isDraft}
              versionSeq={canvas.model.versionSeq}
              onAdjustDay={adjustDay}
              mapAvailable={hasAMapKey()}
            />
          </main>
        )}
      </div>

      {!desktop && (
        <ResultSheet
          open={canvasOpen}
          onOpenChange={setCanvasOpen}
          model={result}
          view={resultView}
          onViewChange={setResultView}
          activeDay={activeDay}
          onActiveDayChange={setActiveDay}
          selection={selection}
          onSelect={setSelection}
          controls={moreControls}
          actionError={actionError}
          isDraft={canvas.model.isDraft}
          versionSeq={canvas.model.versionSeq}
          onAdjustDay={adjustDay}
          mapAvailable={hasAMapKey()}
        />
      )}
    </div>
  );
}

function resolveComposerMode(
  status: RunStatus,
  view: RunView,
  awaitingStage: string,
  baseVersion: number,
): ComposerMode {
  if (status === "queued" || status === "running") return { kind: "running" };
  if (status === "awaiting_user") {
    if (view.researchQuestion) return { kind: "locked", reason: "research_choice" };
    if (view.missing.length > 0) return { kind: "supplement", missing: view.missing };
    return { kind: "revise", rejected: awaitingStage === "proposal_rejected" };
  }
  if (status === "completed") {
    return baseVersion > 0
      ? { kind: "adjust", baseVersion }
      : { kind: "disabled", reason: "方案尚未保存，暂时无法调整。" };
  }
  if (status === "failed") {
    return { kind: "disabled", reason: "本次规划失败，可返回首页重新创建。" };
  }
  if (status === "cancelled") {
    return { kind: "disabled", reason: "本次规划已停止。" };
  }
  return { kind: "disabled", reason: "等待规划开始…" };
}

function lastAwaitingStage(events: RunEvent[]): string {
  for (let index = events.length - 1; index >= 0; index -= 1) {
    if (events[index].type === "run.awaiting_user") {
      return asString(events[index].payload.stage);
    }
  }
  return "";
}

function asRecord(value: unknown): Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
}

function WorkspaceSkeleton() {
  return (
    <div className="flex h-[calc(100dvh-3.5rem)] min-h-0 bg-paper">
      <div className="w-full shrink-0 space-y-3 border-hairline p-4 lg:w-[360px] lg:border-r xl:w-[400px]">
        <Skeleton className="h-6 w-32" />
        <Skeleton className="h-3 w-20" />
        <Skeleton className="mt-6 h-20 w-full" />
        <Skeleton className="h-36 w-full" />
        <Skeleton className="h-16 w-full" />
      </div>
      <div className="hidden flex-1 space-y-3 p-4 lg:block">
        <Skeleton className="h-14 w-full" />
        <Skeleton className="h-[46vh] w-full" />
        <Skeleton className="h-48 w-full" />
      </div>
      <p className="sr-only">正在加载行程…</p>
    </div>
  );
}
