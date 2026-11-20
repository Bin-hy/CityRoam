import { useState } from "react";
import { EmptyHint, SectionLabel } from "@/design/primitives";
import { useMediaQuery } from "@/hooks/useMediaQuery";
import { TripMap } from "@/features/map/TripMap";
import { DetailCard } from "./DetailCard";
import {
  resolveResultSelection,
  type ResultDay,
  type ResultModel,
  type ResultSelection,
} from "./model";
import { StoryColumn } from "./StoryColumn";

export interface MapViewProps {
  model: ResultModel;
  activeDay: number;
  onActiveDayChange: (day: number) => void;
  selection: ResultSelection;
  onSelect: (selection: ResultSelection) => void;
  onAdjustDay?: (dayIndex: number) => void;
  readOnly?: boolean;
  onMapUnavailable?: () => void;
}

export function MapView({
  model,
  activeDay,
  selection,
  onSelect,
  onAdjustDay,
  readOnly = false,
  onMapUnavailable,
}: MapViewProps) {
  const desktop = useMediaQuery("(min-width: 768px)");
  const [storyExpanded, setStoryExpanded] = useState(false);
  const day: ResultDay | undefined = model.days[activeDay];

  if (!day) {
    return <EmptyHint>这一天还没有路线。</EmptyHint>;
  }

  const hotel =
    model.lodging && model.lodging.anchorDays.includes(day.index) ? model.lodging.hotel : null;
  const resolved = resolveResultSelection(model, selection);
  const target = resolved && (resolved.kind === "hotel" || resolved.day.index === day.index) ? resolved : null;

  const map = (
    <TripMap
      day={day}
      hotel={hotel}
      selectedStopID={selection?.kind === "stop" ? selection.stopID : null}
      selectedLegID={selection?.kind === "leg" ? selection.legID : null}
      onSelectStop={(stopID) => onSelect({ kind: "stop", dayIndex: day.index, stopID })}
      onSelectLeg={(legID) => onSelect({ kind: "leg", dayIndex: day.index, legID })}
      onSelectHotel={() => onSelect({ kind: "hotel" })}
      onUnavailable={onMapUnavailable}
      className="h-full w-full md:h-[calc(100dvh-18rem)]"
    />
  );

  const story = (
    <StoryColumn
      day={day}
      selection={selection}
      onSelect={onSelect}
      onSelectStop={(stopID) => onSelect({ kind: "stop", dayIndex: day.index, stopID })}
      onSelectLeg={(legID) => onSelect({ kind: "leg", dayIndex: day.index, legID })}
      readOnly={readOnly}
    />
  );

  return (
    <div
      className={
        desktop ? "grid gap-4 md:grid-cols-[minmax(0,1fr)_22rem]" : "relative h-full min-h-0"
      }
    >
      <div className={desktop ? "min-w-0" : "h-full min-h-0"}>{map}</div>

      {desktop ? (
        <aside aria-label="当天剧本" className="min-w-0 space-y-2 md:overflow-y-auto">
          <SectionLabel>当天剧本</SectionLabel>
          {story}
        </aside>
      ) : (
        <details
          className="absolute inset-x-0 bottom-0 z-10 max-h-[45%] overflow-y-auto rounded-t-md border border-hairline bg-paper/95 px-3 py-2 backdrop-blur"
          open={storyExpanded}
          onToggle={(event) => setStoryExpanded(event.currentTarget.open)}
        >
          <summary className="min-h-9 cursor-pointer text-sm font-medium text-ink">当天剧本</summary>
          <div className="mt-3">{story}</div>
        </details>
      )}

      <DetailCard
        target={target}
        onClose={() => onSelect(null)}
        onAdjustDay={onAdjustDay}
        readOnly={readOnly}
      />
    </div>
  );
}