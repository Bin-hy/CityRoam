import { useEffect, type ReactNode } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { StatusDot } from "@/design/primitives";
import { DayTabs } from "./DayTabs";
import type { ResultModel, ResultSelection, ResultViewMode } from "./model";
import { ResultShell } from "./ResultShell";
import { ViewSwitcher } from "./ViewSwitcher";

export interface ResultSheetProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
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
}

export function ResultSheet({
  open,
  onOpenChange,
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
}: ResultSheetProps) {
  const hasDay = (day: number | null): day is number =>
    day !== null && !!model?.days.some((item) => item.index === day);

  useEffect(() => {
    if (!open || !model || model.days.length === 0) return;
    const valid =
      activeDay !== null && model.days.some((item) => item.index === activeDay);
    if (!valid) onActiveDayChange(model.days[0].index);
  }, [open, model, activeDay, onActiveDayChange]);

  const changeView = (next: ResultViewMode) => {
    if (next === "map") {
      const selectedDay =
        selection && selection.kind !== "hotel" ? selection.dayIndex : null;
      const target = hasDay(selectedDay)
        ? selectedDay
        : hasDay(activeDay)
          ? activeDay
          : (model?.days[0]?.index ?? 0);
      if (activeDay !== target) onActiveDayChange(target);
    }
    onViewChange(next);
  };

  const subtitle = model
    ? `${model.city} · ${model.overview.days} 天${versionSeq ? ` · v${versionSeq}` : ""}`
    : "等待生成";
  const showDayTabs = view !== "board" && !!model && model.days.length > 0;

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent aria-describedby={undefined}>
        <SheetHeader className="sticky top-0 z-10 shrink-0 gap-2 bg-paper px-4 pb-2 pr-14 pt-3">
          <div className="flex items-start gap-2">
            <div className="min-w-0 flex-1">
              <SheetTitle className="truncate text-base">
                {model?.title ?? "行程方案"}
              </SheetTitle>
              <p className="mt-0.5 flex items-center gap-2 text-xs text-muted-foreground">
                <span className="truncate">{subtitle}</span>
                <StatusDot tone={isDraft ? "warning" : "success"} label={isDraft ? "草稿" : "已确认"} />
                {isDraft && <Badge variant="warning">草稿预览</Badge>}
              </p>
            </div>
            {!readOnly && controls && (
              <Popover>
                <PopoverTrigger asChild>
                  <Button size="sm" variant="ghost">
                    更多
                  </Button>
                </PopoverTrigger>
                <PopoverContent className="w-64 space-y-3">{controls}</PopoverContent>
              </Popover>
            )}
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <ViewSwitcher value={view} onChange={changeView} size="md" />
          </div>

          {showDayTabs && (
            <DayTabs
              days={model.days}
              activeDay={activeDay}
              onChange={onActiveDayChange}
              allowAll={false}
              controlsID="result-sheet-view"
            />
          )}

          {actionError && (
            <p role="alert" className="text-xs text-danger">
              {actionError}
            </p>
          )}
        </SheetHeader>

        <div id="result-sheet-view" className="min-h-0 flex-1 overflow-hidden px-4 pb-3 pt-2">
          <ResultShell
            hideToolbar
            model={model}
            view={view}
            onViewChange={changeView}
            activeDay={activeDay}
            onActiveDayChange={onActiveDayChange}
            selection={selection}
            onSelect={onSelect}
            actionError={actionError}
            isDraft={isDraft}
            versionSeq={versionSeq}
            readOnly={readOnly}
            onAdjustDay={onAdjustDay}
            mapAvailable={mapAvailable}
          />
        </div>
      </SheetContent>
    </Sheet>
  );
}