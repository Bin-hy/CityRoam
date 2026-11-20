import { useEffect, useRef } from "react";
import { EmptyHint } from "@/design/primitives";
import { formatYuan } from "@/design/tokens";
import { cn } from "@/lib/utils";
import type { ResultDay, ResultSelection } from "./model";
import { LegDetail } from "./LegDetail";
import { LegRow } from "./LegRow";
import { StopDetail } from "./StopDetail";

export interface StoryColumnProps {
  day: ResultDay;
  selection: ResultSelection;
  onSelect: (selection: ResultSelection) => void;
  onSelectStop: (stopID: string) => void;
  onSelectLeg: (legID: string) => void;
  readOnly?: boolean;
}

export function StoryColumn({
  day,
  selection,
  onSelect,
  onSelectStop,
  onSelectLeg,
  readOnly = false,
}: StoryColumnProps) {
  const itemsRef = useRef<Map<string, HTMLDivElement>>(new Map());
  const selectedKey = keyOf(selection, day.index);

  useEffect(() => {
    if (!selectedKey) return;
    const node = itemsRef.current.get(selectedKey);
    node?.scrollIntoView?.({ block: "nearest", behavior: "smooth" });
  }, [selectedKey]);

  if (day.stops.length === 0) {
    return <EmptyHint>这一天还没有安排停靠点。</EmptyHint>;
  }

  return (
    <div className="space-y-3">
      {day.stops.map((stop) => {
        const leg = day.legs.find((item) => item.from.id === stop.id);
        const stopKey = `stop:${stop.id}`;
        const legKey = leg ? `leg:${leg.id}` : "";
        const stopSelected =
          selection?.kind === "stop" && selection.dayIndex === day.index && selection.stopID === stop.id;
        const legSelected =
          selection?.kind === "leg" && selection.dayIndex === day.index && selection.legID === leg?.id;

        return (
          <div
            key={stop.id}
            ref={(node) => {
              if (node) itemsRef.current.set(stopKey, node);
              else itemsRef.current.delete(stopKey);
            }}
            className="space-y-2"
          >
            <button
              type="button"
              aria-pressed={stopSelected}
              onClick={() => {
                onSelectStop(stop.id);
                onSelect({ kind: "stop", dayIndex: day.index, stopID: stop.id });
              }}
              className={cn(
                "flex w-full items-center gap-2 rounded-md border px-3 py-2 text-left transition-colors",
                stopSelected
                  ? "border-clay/40 bg-clay/5"
                  : "border-hairline bg-card hover:border-forest/30",
              )}
            >
              <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-forest font-mono text-[11px] text-paper">
                {stop.order + 1}
              </span>
              <span className="min-w-0 flex-1">
                <span className="block truncate text-sm text-ink">{stop.title}</span>
                <span className="block font-mono text-[11px] tabular-nums text-muted-foreground">
                  {stop.start}–{stop.end} · {formatYuan(stop.costYuan)}
                </span>
              </span>
            </button>

            {stopSelected && (
              <div className="rounded-md border border-hairline bg-card p-3">
                <StopDetail day={day} stop={stop} readOnly={readOnly} />
              </div>
            )}

            {leg && (
              <div
                ref={(node) => {
                  if (node) itemsRef.current.set(legKey, node);
                  else itemsRef.current.delete(legKey);
                }}
                className="space-y-2 pl-4"
              >
                <LegRow
                  leg={leg}
                  selected={legSelected}
                  onSelect={() => {
                    onSelectLeg(leg.id);
                    onSelect({ kind: "leg", dayIndex: day.index, legID: leg.id });
                  }}
                />
                {legSelected && (
                  <div className="rounded-md border border-hairline bg-card p-3">
                    <LegDetail day={day} leg={leg} readOnly={readOnly} />
                  </div>
                )}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}

function keyOf(selection: ResultSelection, dayIndex: number): string | null {
  if (!selection) return null;
  if (selection.kind === "stop" && selection.dayIndex === dayIndex) return `stop:${selection.stopID}`;
  if (selection.kind === "leg" && selection.dayIndex === dayIndex) return `leg:${selection.legID}`;
  return null;
}