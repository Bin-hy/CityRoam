import { useEffect, useRef } from "react";
import { Badge } from "@/components/ui/badge";
import { EmptyHint, SectionLabel, Stat } from "@/design/primitives";
import { formatYuan } from "@/design/tokens";
import { cn } from "@/lib/utils";
import { paceLabels, type ResultModel, type ResultSelection } from "./model";
import { DayChapter } from "./DayChapter";

export interface DossierViewProps {
  model: ResultModel;
  activeDay: number | null;
  selection: ResultSelection;
  onSelect: (selection: ResultSelection) => void;
  onViewOnMap?: (dayIndex: number) => void;
  onAdjustDay?: (dayIndex: number) => void;
  readOnly?: boolean;
  mapAvailable?: boolean;
}

export function DossierView({
  model,
  activeDay,
  selection,
  onSelect,
  onViewOnMap,
  onAdjustDay,
  readOnly = false,
  mapAvailable = true,
}: DossierViewProps) {
  const rootRef = useRef<HTMLDivElement>(null);
  const chapterRef = useRef<HTMLDivElement>(null);
  const days = activeDay === null ? model.days : model.days.filter((day) => day.index === activeDay);
  const count = days.length;

  useEffect(() => {
    if (activeDay === null) return;
    const root = rootRef.current;
    const target = chapterRef.current;
    if (!root || !target || typeof root.scrollTo !== "function") return;
    root.scrollTo({ top: target.offsetTop, behavior: "smooth" });
  }, [activeDay]);

  return (
    <div ref={rootRef} className="space-y-6">
      <section aria-label="行程概览" className="space-y-3">
        <div className="space-y-1">
          <SectionLabel>行程</SectionLabel>
          <h1 className="font-serif text-2xl leading-tight text-ink">{model.title}</h1>
          <p className="text-sm text-muted-foreground">{model.summary}</p>
        </div>

        <div className="grid grid-cols-2 gap-3 rounded-md border border-hairline bg-card px-4 py-3 sm:grid-cols-5">
          <Stat value={`${model.overview.days} 天`} label="天数" />
          <Stat value={`${model.overview.poiCount} 个`} label="景点" />
          <Stat value={`${model.overview.walkingKM.toFixed(1)} km`} label="步行" />
          <Stat value={formatYuan(model.budget.total)} label="预算" />
          <Stat value={paceLabels[model.overview.pace]} label="节奏" />
        </div>

        <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
          <span>门票 {formatYuan(model.budget.tickets)}</span>
          <span>· 餐饮 {formatYuan(model.budget.meals)}</span>
          <span>· 交通 {formatYuan(model.budget.transit)}</span>
          <span>· 住宿 {formatYuan(model.budget.lodging)}</span>
          {!model.budget.lodgingIncluded && <Badge variant="outline">不计住宿</Badge>}
          {model.budget.estimated && <Badge variant="warning">含估算</Badge>}
        </div>
      </section>

      {days.map((day, index) => (
        <div key={day.index} ref={index === 0 ? chapterRef : undefined}>
          <DayChapter
            day={day}
            hotel={hotelForDay(model, day.index)}
            selection={selection}
            onSelect={onSelect}
            onViewOnMap={onViewOnMap}
            onAdjustDay={onAdjustDay}
            readOnly={readOnly}
            mapAvailable={mapAvailable}
          />
        </div>
      ))}

      {count === 0 && <EmptyHint>这一天还没有安排。</EmptyHint>}

      {(model.lodging || model.reminders.length > 0) && (
        <section aria-label="收尾" className="space-y-4 border-t border-hairline pt-5">
          {model.lodging && (
            <div className="space-y-2">
              <SectionLabel>住宿</SectionLabel>
              <LodgingSummary
                model={model}
                readOnly={readOnly}
                onSelect={() => onSelect({ kind: "hotel" })}
              />
            </div>
          )}

          {model.reminders.length > 0 && (
            <div className="space-y-1.5">
              <SectionLabel>提醒</SectionLabel>
              <ul className="space-y-1 text-sm text-muted-foreground">
                {model.reminders.map((reminder) => (
                  <li key={reminder.id}>{reminder.text}</li>
                ))}
              </ul>
            </div>
          )}
        </section>
      )}
    </div>
  );
}

function LodgingSummary({
  model,
  readOnly,
  onSelect,
}: {
  model: ResultModel;
  readOnly: boolean;
  onSelect: () => void;
}) {
  const lodging = model.lodging;
  if (!lodging) return null;
  const hotel = lodging.hotel;
  const anchors = lodging.anchorDays.map((day) => `第 ${day + 1} 天`).join("、");

  const content = (
    <>
      <span className="flex flex-wrap items-center gap-2">
        <span className="font-serif text-base text-ink">{hotel.name}</span>
        {hotel.star ? <Badge variant="outline">{hotel.star} 星</Badge> : null}
        {typeof hotel.score === "number" ? (
          <span className="text-xs text-muted-foreground">{hotel.score} 分</span>
        ) : null}
      </span>
      <span className="mt-1 block text-xs text-muted-foreground">
        {hotel.address ? `${hotel.address} · ` : ""}
        {lodging.nights} 晚 · {formatYuan(lodging.nightlyYuan)}/晚 · 合计{" "}
        {formatYuan(lodging.totalYuan)}
        {anchors ? ` · 落点在${anchors}` : ""}
        {lodging.includeInBudget ? "" : " · 未计入预算"}
      </span>
    </>
  );

  if (readOnly) {
    return <div className="rounded-md border border-hairline bg-card px-4 py-3 text-sm">{content}</div>;
  }

  return (
    <button
      type="button"
      onClick={onSelect}
      className={cn(
        "w-full rounded-md border border-hairline bg-card px-4 py-3 text-left text-sm transition-colors",
        "hover:border-forest/30",
      )}
    >
      {content}
    </button>
  );
}

function hotelForDay(model: ResultModel, dayIndex: number) {
  if (!model.lodging) return null;
  return model.lodging.anchorDays.includes(dayIndex) ? model.lodging.hotel : null;
}