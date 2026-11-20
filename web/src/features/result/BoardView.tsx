import { useEffect, useRef } from "react";
import { Badge } from "@/components/ui/badge";
import { EmptyHint, Stat } from "@/design/primitives";
import { formatYuan } from "@/design/tokens";
import { useMediaQuery } from "@/hooks/useMediaQuery";
import { cn } from "@/lib/utils";
import { DetailCard } from "./DetailCard";
import {
  resolveResultSelection,
  type ResultDay,
  type ResultModel,
  type ResultSelection,
} from "./model";
import { LegRow } from "./LegRow";

export interface BoardViewProps {
  model: ResultModel;
  activeDay: number | null;
  selection: ResultSelection;
  onSelect: (selection: ResultSelection) => void;
  onAdjustDay?: (dayIndex: number) => void;
  readOnly?: boolean;
}

export function BoardView({
  model,
  activeDay,
  selection,
  onSelect,
  onAdjustDay,
  readOnly = false,
}: BoardViewProps) {
  const desktop = useMediaQuery("(min-width: 768px)");
  const columnRefs = useRef<Map<number, HTMLDivElement>>(new Map());
  const target = resolveResultSelection(model, selection);

  useEffect(() => {
    if (activeDay === null) return;
    columnRefs.current.get(activeDay)?.scrollIntoView?.({ block: "nearest", inline: "start" });
  }, [activeDay]);

  return (
    <div className="space-y-4">
      <section aria-label="核心指标" className="grid grid-cols-2 gap-3 rounded-md border border-hairline bg-card px-4 py-3 sm:grid-cols-5">
        <Stat value={formatYuan(model.budget.total)} label="总预算" />
        {model.lodging ? (
          <button
            type="button"
            aria-label="查看住宿详情"
            className="min-w-0 text-left"
            onClick={() => onSelect({ kind: "hotel" })}
          >
            <Stat
              value={formatYuan(model.lodging.totalYuan)}
              label={model.lodging.includeInBudget ? "住宿" : "住宿（未计入）"}
            />
          </button>
        ) : (
          <Stat value="—" label="住宿" />
        )}
        <Stat value={formatYuan(model.budget.tickets)} label="门票" />
        <Stat value={`${model.overview.walkingKM.toFixed(1)} km`} label="步行" />
        <Stat value={`${model.overview.poiCount} 个`} label="景点" />
      </section>

      {model.days.length === 0 && <EmptyHint>还没有行程安排。</EmptyHint>}

      <div className={cn("gap-3", desktop ? "flex overflow-x-auto pb-2" : "space-y-3")}>
        {model.days.map((day, index) => (
          <div
            key={day.index}
            ref={(node) => {
              if (node) columnRefs.current.set(day.index, node);
              else columnRefs.current.delete(day.index);
            }}
            className={cn(desktop && "w-[18rem] shrink-0")}
          >
            {desktop ? (
              <DayColumn
                day={day}
                model={model}
                active={activeDay === day.index}
                selection={selection}
                onSelect={onSelect}
                showReminders={index === 0}
              />
            ) : (
              <details
                open={activeDay === day.index}
                className="rounded-md border border-hairline bg-paper"
              >
                <summary className="flex min-h-11 cursor-pointer items-center justify-between gap-2 px-3 py-2 text-sm text-ink">
                  <span>
                    第 {day.index + 1} 天 · {day.label}
                  </span>
                  <span className="font-mono text-xs tabular-nums text-muted-foreground">
                    {formatYuan(day.costYuan)}
                  </span>
                </summary>
                <div className="p-3">
                  <DayColumn
                    day={day}
                    model={model}
                    active={activeDay === day.index}
                    selection={selection}
                    onSelect={onSelect}
                    showReminders={index === 0}
                    embedded
                  />
                </div>
              </details>
            )}
          </div>
        ))}
      </div>

      <DetailCard
        target={target}
        onClose={() => onSelect(null)}
        onAdjustDay={onAdjustDay}
        readOnly={readOnly}
      />
    </div>
  );
}

function DayColumn({
  day,
  model,
  active,
  selection,
  onSelect,
  showReminders,
  embedded = false,
}: {
  day: ResultDay;
  model: ResultModel;
  active: boolean;
  selection: ResultSelection;
  onSelect: (selection: ResultSelection) => void;
  showReminders: boolean;
  embedded?: boolean;
}) {
  return (
    <article
      aria-label={`第 ${day.index + 1} 天`}
      data-active={active ? "true" : "false"}
      className={cn(
        "space-y-2 rounded-md border bg-card p-3",
        active ? "border-forest/40" : "border-hairline",
        embedded && "border-0 bg-transparent p-0",
      )}
    >
      <header className="space-y-0.5">
        <p className="font-serif text-base text-ink">
          第 {day.index + 1} 天 · {day.date}
        </p>
        <p className="text-xs text-muted-foreground">
          {day.label} · 当天合计{" "}
          <span className="font-mono tabular-nums text-ink">{formatYuan(day.costYuan)}</span>
        </p>
      </header>

      {day.stops.length === 0 && <p className="text-xs text-muted-foreground">还没有停靠点。</p>}

      <ol className="space-y-2">
        {day.stops.map((stop) => {
          const leg = day.legs.find((item) => item.from.id === stop.id);
          const stopSelected =
            selection?.kind === "stop" && selection.dayIndex === day.index && selection.stopID === stop.id;
          const legSelected =
            selection?.kind === "leg" && selection.dayIndex === day.index && selection.legID === leg?.id;
          return (
            <li key={stop.id} className="space-y-1.5">
              <button
                type="button"
                aria-pressed={stopSelected}
                onClick={() => onSelect({ kind: "stop", dayIndex: day.index, stopID: stop.id })}
                className={cn(
                  "w-full rounded-md border px-2.5 py-2 text-left transition-colors",
                  stopSelected
                    ? "border-clay/40 bg-clay/5"
                    : "border-hairline bg-paper/60 hover:border-forest/30",
                )}
              >
                <span className="flex items-baseline justify-between gap-2">
                  <span className="font-mono text-[11px] tabular-nums text-muted-foreground">
                    {stop.start}
                  </span>
                  <span className="min-w-0 flex-1 truncate text-sm text-ink">{stop.title}</span>
                  <span className="font-mono text-[11px] tabular-nums text-muted-foreground">
                    ¥{Math.round(stop.costYuan)}
                  </span>
                </span>
                <span className="mt-1 block truncate text-xs text-muted-foreground">
                  {stop.reason ?? "未生成额外说明"}
                </span>
              </button>

              {leg && (
                <LegRow
                  leg={leg}
                  selected={legSelected}
                  onSelect={() => onSelect({ kind: "leg", dayIndex: day.index, legID: leg.id })}
                />
              )}
            </li>
          );
        })}
      </ol>

      {day.gaps.length > 0 && (
        <div className="space-y-1 rounded-md border border-dashed border-hairline px-2.5 py-2">
          <p className="text-[11px] uppercase tracking-[0.16em] text-muted-foreground">留白</p>
          {day.gaps.map((gap) => (
            <p key={gap.id} className="text-xs text-muted-foreground">
              {gap.start}–{gap.end} · {gap.suggestion}
            </p>
          ))}
        </div>
      )}

      {showReminders && model.reminders.length > 0 && (
        <div className="space-y-1 rounded-md border border-hairline bg-sand/30 px-2.5 py-2">
          <p className="text-[11px] uppercase tracking-[0.16em] text-muted-foreground">提醒</p>
          {model.reminders.map((reminder) => (
            <p key={reminder.id} className="text-xs text-muted-foreground">
              {reminder.text}
            </p>
          ))}
        </div>
      )}

      {day.legs.length === 0 && day.stops.length > 0 && (
        <p className="text-[11px] text-muted-foreground">当天没有记录交通衔接。</p>
      )}
      {model.budget.estimated && <Badge variant="warning">含估算</Badge>}
    </article>
  );
}