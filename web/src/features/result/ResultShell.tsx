import { useState, type ReactNode } from "react";
import { EmptyHint } from "@/design/primitives";
import { BoardView } from "./BoardView";
import { DossierView } from "./DossierView";
import { MapView } from "./MapView";
import type { ResultModel, ResultSelection, ResultViewMode } from "./model";
import { ResultToolbar } from "./ResultToolbar";

export interface ResultShellProps {
  model: ResultModel | null;
  view: ResultViewMode;
  onViewChange: (view: ResultViewMode) => void;
  activeDay: number | null;
  onActiveDayChange: (day: number | null) => void;
  selection: ResultSelection;
  onSelect: (selection: ResultSelection) => void;
  controls?: ReactNode;
  actionError?: string | null;
  isDraft: boolean;
  versionSeq: number | null;
  readOnly?: boolean;
  onAdjustDay?: (dayIndex: number) => void;
  mapAvailable?: boolean;
  hideToolbar?: boolean;
}

export function ResultShell({
  model,
  view,
  onViewChange,
  activeDay,
  onActiveDayChange,
  selection,
  onSelect,
  controls,
  actionError,
  isDraft,
  versionSeq,
  readOnly = false,
  onAdjustDay,
  mapAvailable = true,
  hideToolbar = false,
}: ResultShellProps) {
  const [mapFailed, setMapFailed] = useState(false);
  const mapUsable = mapAvailable && !mapFailed;
  const dayForMap =
    model && model.days.length > 0
      ? activeDay !== null && model.days.some((day) => day.index === activeDay)
        ? activeDay
        : 0
      : 0;

  const viewOnMap = (dayIndex: number) => {
    onActiveDayChange(dayIndex);
    onViewChange("map");
  };

  const changeView = (next: ResultViewMode) => {
    if (next === "map") {
      const selectedDay =
        selection && selection.kind !== "hotel" ? selection.dayIndex : null;
      const target =
        selectedDay !== null && model?.days.some((day) => day.index === selectedDay)
          ? selectedDay
          : dayForMap;
      if (activeDay !== target) onActiveDayChange(target);
    }
    onViewChange(next);
  };

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      {!hideToolbar && (
        <ResultToolbar
          model={model}
          view={view}
          onViewChange={changeView}
          activeDay={activeDay}
          onActiveDayChange={onActiveDayChange}
          showDayTabs={view !== "board"}
          allowAll={view === "dossier"}
          isDraft={isDraft}
          versionSeq={versionSeq}
          controls={controls}
          actionError={actionError}
          readOnly={readOnly}
          className="shrink-0"
        />
      )}

      <div role="tabpanel" aria-label="结果内容" className="min-h-0 flex-1 overflow-y-auto">
        {!model ? (
          <EmptyHint>方案生成后会显示在这里。</EmptyHint>
        ) : (
          <>
            {view === "dossier" && (
              <DossierView
                model={model}
                activeDay={activeDay}
                selection={selection}
                onSelect={onSelect}
                onViewOnMap={viewOnMap}
                onAdjustDay={onAdjustDay}
                readOnly={readOnly}
                mapAvailable={mapUsable}
              />
            )}
            {view === "map" && (
              <MapView
                model={model}
                activeDay={dayForMap}
                onActiveDayChange={(day) => onActiveDayChange(day)}
                selection={selection}
                onSelect={onSelect}
                onAdjustDay={onAdjustDay}
                readOnly={readOnly}
                onMapUnavailable={() => setMapFailed(true)}
              />
            )}
            {view === "board" && (
              <BoardView
                model={model}
                activeDay={activeDay}
                selection={selection}
                onSelect={onSelect}
                onAdjustDay={onAdjustDay}
                readOnly={readOnly}
              />
            )}
          </>
        )}
      </div>
    </div>
  );
}