import { useEffect, useMemo, useState } from "react";
import { Link, useLocation, useParams } from "react-router-dom";
import { ArrowLeft, Loader2 } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ErrorState } from "@/components/StateMessage";
import { useMediaQuery } from "@/hooks/useMediaQuery";
import { useRunStream } from "@/hooks/useRunStream";
import { useTrip } from "@/features/trips/useTrip";
import { parseItinerary } from "@/features/itinerary/types";
import { RunTrace } from "@/features/trace/RunTrace";
import { stageLabels, type RunStatus } from "@/features/trace/traceReducer";
import { ItineraryTimeline } from "@/features/timeline/ItineraryTimeline";
import { TripMap } from "@/features/map/TripMap";
import { BudgetPanel } from "@/features/budget/BudgetPanel";
import { VersionPanel } from "@/features/versions/VersionPanel";
import { ShareDialog } from "@/features/share/ShareDialog";
import { RunActions } from "./RunActions";

const terminalStatuses = new Set(["awaiting_user", "completed", "failed", "cancelled"]);

export function WorkspacePage() {
  const { tripId } = useParams();
  const location = useLocation();
  const routeRunID = (location.state as { runId?: string } | null)?.runId;
  const tripQuery = useTrip(tripId);
  const trip = tripQuery.data;
  const [overrideRunID, setOverrideRunID] = useState<string | null>(null);
  const runID = overrideRunID ?? routeRunID ?? trip?.latest_run?.id;
  const trace = useRunStream(runID, { enabled: Boolean(runID) });
  const itinerary = useMemo(() => parseItinerary(trip?.latest_version?.plan), [trip?.latest_version?.plan]);
  const [selectedBlockID, setSelectedBlockID] = useState<string | null>(null);
  const isDesktop = useMediaQuery("(min-width: 1024px)");

  const refetchTrip = tripQuery.refetch;
  useEffect(() => {
    if (terminalStatuses.has(trace.status)) {
      void refetchTrip();
    }
  }, [trace.status, refetchTrip]);

  if (tripQuery.isPending) {
    return <WorkspaceSkeleton />;
  }

  if (tripQuery.isError || !trip) {
    return (
      <main className="mx-auto max-w-xl px-4 py-16">
        <ErrorState
          title="行程加载失败"
          description="链接可能已失效，或服务暂时不可用。"
          onRetry={() => void tripQuery.refetch()}
          action={
            <Button size="sm" variant="outline" asChild>
              <Link to="/">返回首页</Link>
            </Button>
          }
        />
      </main>
    );
  }

  const tripStatus = trip.latest_run?.status;
  const traceTerminal = trace.status === "completed" || trace.status === "failed" || trace.status === "cancelled";
  const tripTerminal = tripStatus === "completed" || tripStatus === "failed" || tripStatus === "cancelled";
  const status: RunStatus = traceTerminal
    ? trace.status
    : tripTerminal
      ? tripStatus
      : trace.status !== "idle"
        ? trace.status
        : ((tripStatus ?? "idle") as RunStatus);
  const stage = trace.lastSeq > 0 && !tripTerminal ? trace.stage : (trip.latest_run?.stage ?? trace.stage);

  const mapCard = (
    <Card>
      <CardHeader>
        <CardTitle>地图</CardTitle>
        <CardDescription>按天展示路线与标记，点击标记同步时间轴。</CardDescription>
      </CardHeader>
      <CardContent>
        {itinerary ? (
          <TripMap itinerary={itinerary} selectedBlockID={selectedBlockID} onSelectBlock={setSelectedBlockID} />
        ) : (
          <div className="flex h-64 items-center justify-center rounded-lg bg-muted text-sm text-muted-foreground">
            等待方案生成…
          </div>
        )}
      </CardContent>
    </Card>
  );

  const timelineCard = (
    <Card>
      <CardHeader>
        <CardTitle>时间轴</CardTitle>
        <CardDescription>
          {selectedBlockID ? `已选中 ${selectedBlockID}，地图将同步高亮。` : "按天查看景点、餐饮与交通。"}
        </CardDescription>
      </CardHeader>
      <CardContent>
        {itinerary ? (
          <ItineraryTimeline
            itinerary={itinerary}
            selectedBlockID={selectedBlockID}
            onSelectBlock={setSelectedBlockID}
          />
        ) : (
          <div className="flex h-32 items-center justify-center text-sm text-muted-foreground">
            <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            等待方案生成…
          </div>
        )}
      </CardContent>
    </Card>
  );

  return (
    <div className="flex min-h-screen flex-col">
      <header className="flex flex-wrap items-center justify-between gap-3 border-b bg-card/80 px-4 py-3 backdrop-blur">
        <div className="flex items-center gap-3">
          <Button variant="ghost" size="icon" asChild>
            <Link to="/" aria-label="返回首页">
              <ArrowLeft className="h-4 w-4" />
            </Link>
          </Button>
          <div>
            <h1 className="text-base font-semibold">
              {trip.city === "beijing" || !trip.city ? "北京" : trip.city} · {trip.days} 天
            </h1>
            <p className="text-xs text-muted-foreground">Trip {trip.id.slice(0, 8)}</p>
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
          <Badge variant={status === "failed" ? "destructive" : status === "awaiting_user" ? "warning" : "secondary"}>
            {stageLabels[stage] ?? stage}
          </Badge>
          <span>{status}</span>
          {trip.latest_version && <ShareDialog tripID={trip.id} />}
        </div>
      </header>

      <main className="grid flex-1 gap-4 p-4 lg:grid-cols-[300px_minmax(0,1fr)_320px]">
        <aside className="order-2 min-h-[420px] lg:order-1">
          <RunTrace state={trace} connected={trace.connected} />
        </aside>

        <section className="order-1 space-y-4 lg:order-2">
          <RunActions
            tripID={trip.id}
            runID={runID ?? null}
            status={status}
            missing={trace.missing}
            proposal={trace.proposal}
            baseVersion={trip.latest_version?.seq ?? null}
            onRunStarted={setOverrideRunID}
            onActionComplete={() => void tripQuery.refetch()}
          />

          {isDesktop ? (
            <>
              {mapCard}
              {timelineCard}
            </>
          ) : (
            <Tabs defaultValue="timeline" className="space-y-3">
              <TabsList className="grid w-full grid-cols-2">
                <TabsTrigger value="timeline">时间轴</TabsTrigger>
                <TabsTrigger value="map">地图</TabsTrigger>
              </TabsList>
              <TabsContent value="timeline" className="mt-0">
                {timelineCard}
              </TabsContent>
              <TabsContent value="map" className="mt-0">
                {mapCard}
              </TabsContent>
            </Tabs>
          )}
        </section>

        <aside className="order-3 space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>预算</CardTitle>
              <CardDescription>门票、餐饮、交通与总计。</CardDescription>
            </CardHeader>
            <CardContent>
              <BudgetPanel itinerary={itinerary} />
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>版本</CardTitle>
              <CardDescription>方案历史与差异。</CardDescription>
            </CardHeader>
            <CardContent>
              <VersionPanel
                tripID={trip.id}
                currentVersion={trip.latest_version ?? null}
                onRestored={() => void tripQuery.refetch()}
              />
            </CardContent>
          </Card>
        </aside>
      </main>
      <footer className="border-t px-4 py-3 text-xs text-muted-foreground">
        AI 生成，价格与时间为估算；出行前请核实开放时间与门票。
      </footer>
    </div>
  );
}

function WorkspaceSkeleton() {
  return (
    <div className="min-h-screen p-4">
      <div className="mb-4 flex items-center gap-3">
        <Skeleton className="h-9 w-9" />
        <div className="space-y-1">
          <Skeleton className="h-4 w-32" />
          <Skeleton className="h-3 w-20" />
        </div>
      </div>
      <div className="grid gap-4 lg:grid-cols-[300px_minmax(0,1fr)_320px]">
        <Skeleton className="h-[480px]" />
        <div className="space-y-4">
          <Skeleton className="h-72" />
          <Skeleton className="h-56" />
        </div>
        <div className="space-y-4">
          <Skeleton className="h-48" />
          <Skeleton className="h-40" />
        </div>
      </div>
      <p className="mt-4 flex items-center justify-center gap-2 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" />
        正在加载行程…
      </p>
    </div>
  );
}
