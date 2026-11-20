import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { Check, Copy } from "lucide-react";
import { ApiError } from "@/api/client";
import { getSharedTrip } from "@/api/trips";
import { Alert } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { SectionLabel } from "@/design/primitives";
import { formatYuan } from "@/design/tokens";
import { explanationMap, parseItinerary } from "@/features/itinerary/types";
import { parseResearch } from "@/features/research/types";
import { DossierView } from "@/features/result/DossierView";
import {
  buildResultModel,
  type ResultSelection,
} from "@/features/result/model";
import { itineraryToPlainText } from "./plainText";
import { hasAMapKey } from "@/lib/mapKey";

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
  const [selection, setSelection] = useState<ResultSelection>(null);

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
  const result = buildResultModel({
    itinerary,
    explanations: explanationMap(itinerary),
    research,
  });

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
    <div className="flex min-h-screen flex-col bg-paper">
      <header className="border-b border-hairline">
        <div className="mx-auto flex w-full max-w-4xl flex-wrap items-center justify-between gap-3 px-4 py-4 lg:px-8">
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5 text-xs text-muted-foreground">
            <span className="font-serif text-base text-ink">TripPilot · 分享的行程</span>
            <Badge variant="secondary">AI 生成</Badge>
            <Badge variant="outline">只读</Badge>
            {result && <span className="tabular-nums">总预算 {formatYuan(result.budget.total)}</span>}
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <Button size="sm" variant="outline" onClick={() => void handleCopy()} disabled={!itinerary}>
              {copyState === "copied" ? <Check className="mr-1 h-4 w-4" /> : <Copy className="mr-1 h-4 w-4" />}
              {copyState === "copied" ? "已复制" : "复制行程文本"}
            </Button>
            <Button size="sm" asChild>
              <Link to="/">我也来规划一版</Link>
            </Button>
          </div>
        </div>
        {copyState === "error" && (
          <div className="mx-auto w-full max-w-4xl px-4 pb-4 lg:px-8">
            <p role="alert" className="text-xs text-danger">
              复制失败，请检查浏览器剪贴板权限。
            </p>
          </div>
        )}
      </header>

      <main className="mx-auto w-full max-w-4xl flex-1 space-y-6 px-4 py-6 lg:px-8">
        {result ? (
          <DossierView
            model={result}
            activeDay={null}
            selection={selection}
            onSelect={setSelection}
            readOnly
            mapAvailable={hasAMapKey()}
          />
        ) : (
          <p className="text-sm text-muted-foreground">分享快照中没有可用的行程数据。</p>
        )}

        {!research && (
          <p className="rounded-md border border-dashed border-hairline bg-paper/50 px-3 py-2 text-xs text-muted-foreground">
            此分享快照未包含住宿与攻略摘要。
          </p>
        )}

        <div className="space-y-1.5 border-t border-hairline pt-4">
          <SectionLabel>来源</SectionLabel>
          <p className="font-mono text-sm tabular-nums text-ink">版本 v{shared.version.seq}</p>
          <p className="text-xs text-muted-foreground">创建于 {formatDate(shared.created_at)}</p>
          <p className="text-xs leading-relaxed text-muted-foreground">
            分享时保存的快照，不随原行程后续调整而变化。
          </p>
        </div>
      </main>

      <footer className="border-t border-hairline px-4 py-3 text-center text-xs text-muted-foreground lg:px-8">
        AI 生成，价格与时间为估算；出行前请核实开放时间与门票。
      </footer>
    </div>
  );
}