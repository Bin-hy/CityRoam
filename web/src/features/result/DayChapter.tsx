import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { SectionLabel } from "@/design/primitives";
import { formatYuan } from "@/design/tokens";
import { cn } from "@/lib/utils";
import type { ResultDay, ResultHotel, ResultLeg, ResultSelection, ResultStop } from "./model";
import { DayMiniMap } from "./DayMiniMap";
import { LegDetail } from "./LegDetail";
import { LegRow } from "./LegRow";
import { GuideSection, ReasonSection, StopFacts } from "./StopDetail";

export interface DayChapterProps {
  day: ResultDay;
  hotel: ResultHotel | null;
  selection: ResultSelection;
  onSelect: (selection: ResultSelection) => void;
  onViewOnMap?: (dayIndex: number) => void;
  onAdjustDay?: (dayIndex: number) => void;
  readOnly?: boolean;
  mapAvailable?: boolean;
}

export function DayChapter({
  day,
  hotel,
  selection,
  onSelect,
  onViewOnMap,
  onAdjustDay,
  readOnly = false,
  mapAvailable = true,
}: DayChapterProps) {
  return (
    <article aria-label={`第 ${day.index + 1} 天`} className="space-y-4">
      <header className="space-y-1">
        <SectionLabel>
          第 {day.index + 1} 天 · {day.start}–{day.end}
        </SectionLabel>
        <div className="flex flex-wrap items-baseline justify-between gap-2">
          <h2 className="font-serif text-xl leading-snug text-ink">
            {day.label} · {day.theme}
          </h2>
          {!readOnly && onAdjustDay && (
            <Button size="sm" variant="ghost" onClick={() => onAdjustDay(day.index)}>
              调整这一天
            </Button>
          )}
        </div>
      </header>

      <DayMiniMap
        day={day}
        hotel={hotel}
        selection={selection}
        onSelect={onSelect}
        onViewOnMap={onViewOnMap}
        readOnly={readOnly}
        mapAvailable={mapAvailable}
      />

      {day.stops.length > 0 ? (
        <ol className="space-y-4">
          {day.stops.map((stop) => {
            const leg = nextLeg(day, stop);
            return (
              <li key={stop.id} className="space-y-2">
                <StopBlock
                  stop={stop}
                  selected={isStopSelected(selection, day.index, stop.id)}
                  onSelect={() => onSelect({ kind: "stop", dayIndex: day.index, stopID: stop.id })}
                />
                <StopFacts stop={stop} />
                {stop.note && <p className="text-sm text-muted-foreground">{stop.note}</p>}
                <ReasonSection reason={stop.reason} />
                <GuideSection guides={stop.guides} />
                {leg && (
                  <div className="space-y-2 pl-3">
                    <LegRow
                      leg={leg}
                      selected={isLegSelected(selection, day.index, leg.id)}
                      onSelect={() => onSelect({ kind: "leg", dayIndex: day.index, legID: leg.id })}
                    />
                    {isLegSelected(selection, day.index, leg.id) && (
                      <div className="rounded-md border border-hairline bg-card p-3">
                        <LegDetail
                          day={day}
                          leg={leg}
                          onViewOnMap={onViewOnMap}
                          readOnly={readOnly}
                        />
                      </div>
                    )}
                  </div>
                )}
              </li>
            );
          })}
        </ol>
      ) : (
        <p className="text-sm text-muted-foreground">这一天还没有安排停靠点。</p>
      )}

      {day.gaps.length > 0 && (
        <div className="space-y-1.5 rounded-md border border-dashed border-hairline bg-paper/50 px-3 py-2">
          <SectionLabel>未排时段</SectionLabel>
          <ul className="space-y-0.5 text-sm text-muted-foreground">
            {day.gaps.map((gap) => (
              <li key={gap.id}>
                {gap.start}–{gap.end} · {gap.minutes} 分钟 · {gap.suggestion}
              </li>
            ))}
          </ul>
        </div>
      )}

      <p className="text-right text-sm text-muted-foreground">
        当天合计 <span className="font-mono tabular-nums text-ink">{formatYuan(day.costYuan)}</span>
      </p>
    </article>
  );
}

function StopBlock({
  stop,
  selected,
  onSelect,
}: {
  stop: ResultStop;
  selected: boolean;
  onSelect: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onSelect}
      aria-pressed={selected}
      className={cn(
        "flex w-full flex-wrap items-center gap-x-3 gap-y-1 rounded-md border px-3 py-2 text-left transition-colors",
        selected
          ? "border-clay/40 bg-clay/5"
          : "border-hairline bg-card hover:border-forest/30",
      )}
    >
      <span className="font-mono text-xs tabular-nums text-muted-foreground">
        {stop.start}–{stop.end}
      </span>
      <span className="font-serif text-base text-ink">{stop.title}</span>
      <Badge variant="outline">{stop.typeLabel}</Badge>
    </button>
  );
}

function nextLeg(day: ResultDay, stop: ResultStop): ResultLeg | null {
  return day.legs.find((leg) => leg.from.id === stop.id) ?? null;
}

function isStopSelected(selection: ResultSelection, dayIndex: number, stopID: string): boolean {
  return selection?.kind === "stop" && selection.dayIndex === dayIndex && selection.stopID === stopID;
}

function isLegSelected(selection: ResultSelection, dayIndex: number, legID: string): boolean {
  return selection?.kind === "leg" && selection.dayIndex === dayIndex && selection.legID === legID;
}