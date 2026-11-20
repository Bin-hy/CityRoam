import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { ArrowLeft, Check, Copy } from "lucide-react";
import { ApiError } from "@/api/client";
import { getSharedTrip } from "@/api/trips";
import { Alert } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { BudgetPanel } from "@/features/budget/BudgetPanel";
import { parseItinerary } from "@/features/itinerary/types";
import { ResearchPanel } from "@/features/research/ResearchPanel";
import { parseResearch } from "@/features/research/types";
import { ItineraryTimeline } from "@/features/timeline/ItineraryTimeline";
import { itineraryToPlainText } from "./plainText";

function formatDate(value: string): string {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return value;
  return `${parsed.getFullYear()} 年 ${parsed.getMonth() + 1} 月 ${parsed.getDate()} 日`;
}

export function SharePage() {
  const { slug } = useParams();
  const shareQuery = useQuery({
    queryKey: ["share", slug],
    queryFn: () => getSharedTrip(slug as string),
    enabled: Boolean(slug),
    retry: false,
  });
  const [copyState, setCopyState] = useState<"idle" | "copied" | "error">("idle");

  if (shareQuery.isPending) {
    return (
      <main className="mx-auto max-w-5xl space-y-4 px-4 py-10">
        <Skeleton className="h-10 w-64" />
        <Skeleton className="h-80" />
        <p className="text-sm text-muted-foreground">正在加载分享的行程…</p>
      </main>
    );
  }

  if (shareQuery.isError) {
    const notFound = shareQuery.error instanceof ApiError && shareQuery.error.status === 404;
    return (
      <main className="mx-auto max-w-xl px-4 py-16">
        <Alert variant={notFound ? "default" : "destructive"} className="space-y-3">
          <p className="font-medium">{notFound ? "分享不存在或已撤销" : "分享加载失败"}</p>
          <p className="text-sm">
            {notFound ? "链接可能已失效，请联系分享者重新生成。" : "服务暂时不可用，请稍后重试。"}
          </p>
          <div className="flex gap-2">
            {!notFound && (
              <Button size="sm" onClick={() => void shareQuery.refetch()}>
                重试
              </Button>
            )}
            <Button size="sm" variant="outline" asChild>
              <Link to="/">返回首页</Link>
            </Button>
          </div>
        </Alert>
      </main>
    );
  }

  const shared = shareQuery.data;
  const itinerary = parseItinerary(shared.version.plan);
  const research = parseResearch(shared.version.research);
  const poiNames: Record<string, string> = {};
  for (const day of itinerary?.days ?? []) {
    for (const block of day.blocks) {
      if (block.type === "poi" && block.poi_id) poiNames[block.poi_id] = block.title;
    }
  }
  const title = itinerary ? `${itinerary.city === "beijing" || !itinerary.city ? "北京" : itinerary.city} · ${itinerary.days.length} 天` : "分享的行程";

  async function handleCopy() {
    if (!itinerary) return;
    try {
      await navigator.clipboard.writeText(itineraryToPlainText(itinerary));
      setCopyState("copied");
    } catch {
      setCopyState("error");
    }
  }

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
            <h1 className="text-base font-semibold">{title}</h1>
            <p className="text-xs text-muted-foreground">TripPilot 分享的只读行程</p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Badge variant="secondary">AI 生成</Badge>
          <Badge variant="outline">只读</Badge>
        </div>
      </header>

      <main className="mx-auto grid w-full max-w-6xl flex-1 gap-4 px-4 py-6 lg:grid-cols-[minmax(0,1fr)_320px]">
        <section className="min-w-0">
          <Card>
            <CardHeader>
              <CardTitle>行程时间轴</CardTitle>
              <CardDescription>分享时保存的快照，不随原行程后续调整而变化。</CardDescription>
            </CardHeader>
            <CardContent>
              {itinerary ? (
                <ItineraryTimeline itinerary={itinerary} />
              ) : (
                <p className="text-sm text-muted-foreground">行程数据不可用。</p>
              )}
            </CardContent>
          </Card>
        </section>

        <aside className="space-y-4">
          {research && (
            <Card>
              <CardHeader>
                <CardTitle>研究与住宿</CardTitle>
                <CardDescription>住宿建议与外部攻略摘要，仅供参考。</CardDescription>
              </CardHeader>
              <CardContent>
                <ResearchPanel research={research} poiNames={poiNames} />
              </CardContent>
            </Card>
          )}

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
              <CardTitle>来源</CardTitle>
              <CardDescription>分享链接与版本快照。</CardDescription>
            </CardHeader>
            <CardContent className="space-y-2 text-sm text-muted-foreground">
              <p>版本 v{shared.version.seq}</p>
              <p>创建于 {formatDate(shared.created_at)}</p>
              <p className="break-all">行程 {shared.trip_id}</p>
              <Button size="sm" variant="outline" onClick={() => void handleCopy()} disabled={!itinerary}>
                {copyState === "copied" ? <Check className="mr-1 h-4 w-4" /> : <Copy className="mr-1 h-4 w-4" />}
                {copyState === "copied" ? "已复制" : "复制行程文本"}
              </Button>
              {copyState === "error" && (
                <p role="alert" className="text-xs text-destructive">
                  复制失败，请检查浏览器剪贴板权限。
                </p>
              )}
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
