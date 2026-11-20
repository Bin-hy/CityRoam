import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { SectionLabel } from "@/design/primitives";
import { formatYuan } from "@/design/tokens";
import type { ResultDay, ResultLeg } from "./model";

export interface LegDetailProps {
  day: ResultDay;
  leg: ResultLeg;
  onViewOnMap?: (dayIndex: number) => void;
  readOnly?: boolean;
}

const geometryNotes: Record<ResultLeg["geometry"], string> = {
  polyline: "按实际路线绘制",
  straight: "直线估算，仅供距离参考",
  none: "缺少坐标，仅显示文字",
};

export function LegDetail({ day, leg, onViewOnMap, readOnly = false }: LegDetailProps) {
  return (
    <div className="space-y-4">
      <header className="space-y-1">
        <SectionLabel>
          第 {day.index + 1} 天 · {leg.from.title} → {leg.to.title}
        </SectionLabel>
        <h2 className="flex flex-wrap items-center gap-2 font-serif text-lg leading-snug text-ink">
          {leg.modeLabel}
          <span className="font-sans text-sm font-normal text-muted-foreground">
            {leg.durationMin} 分钟
            {leg.costYuan > 0 ? ` · ${formatYuan(leg.costYuan)}` : ""}
            {leg.distanceM > 0 ? ` · ${(leg.distanceM / 1000).toFixed(1)} km` : ""}
          </span>
          {leg.estimated && <Badge variant="warning">估算</Badge>}
        </h2>
      </header>

      <div className="space-y-1.5">
        <SectionLabel>分段</SectionLabel>
        <ul className="space-y-1.5">
          {leg.segments.map((segment, index) => (
            <li
              key={`${segment.mode}-${index}`}
              className="rounded-md border border-hairline bg-paper/60 px-2.5 py-2 text-sm"
            >
              <p className="flex flex-wrap items-center gap-x-2 gap-y-1 text-ink">
                <span className="font-medium">{segment.modeLabel}</span>
                {segment.lineName && <span>{segment.lineName}</span>}
                {segment.estimated && <Badge variant="warning">估算</Badge>}
              </p>
              {(segment.fromStop || segment.toStop) && (
                <p className="mt-0.5 text-xs text-muted-foreground">
                  {segment.fromStop ?? "—"} → {segment.toStop ?? "—"}
                </p>
              )}
              <p className="mt-0.5 font-mono text-xs tabular-nums text-muted-foreground">
                {segment.distanceM > 0 ? `${(segment.distanceM / 1000).toFixed(1)} km · ` : ""}
                {segment.durationMin} 分钟 · {formatYuan(segment.costYuan)}
              </p>
            </li>
          ))}
        </ul>
      </div>

      {leg.estimated && (
        <p className="text-xs text-warning">整段为距离估算，实际时间与车次以出发时为准。</p>
      )}
      <p className="text-xs text-muted-foreground">{geometryNotes[leg.geometry]}</p>

      {!readOnly && onViewOnMap && (
        <div className="border-t border-hairline pt-3">
          <Button size="sm" variant="outline" onClick={() => onViewOnMap(leg.dayIndex)}>
            查看路线图
          </Button>
        </div>
      )}
    </div>
  );
}