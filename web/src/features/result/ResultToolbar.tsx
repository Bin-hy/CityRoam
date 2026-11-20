import type { ReactNode } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { StatusDot } from "@/design/primitives";
import { cn } from "@/lib/utils";
import type { ResultModel, ResultViewMode } from "./model";
import { DayTabs } from "./DayTabs";
import { ViewSwitcher } from "./ViewSwitcher";

export interface ResultToolbarProps {
  model: ResultModel | null;
  view: ResultViewMode;
  onViewChange: (view: ResultViewMode) => void;
  activeDay: number | null;
  onActiveDayChange: (day: number | null) => void;
  showDayTabs: boolean;
  allowAll: boolean;
  isDraft: boolean;
  versionSeq: number | null;
  controls?: ReactNode;
  actionError?: string | null;
  readOnly?: boolean;
  className?: string;
}

export function ResultToolbar({
  model,
  view,
  onViewChange,
  activeDay,
  onActiveDayChange,
  showDayTabs,
  allowAll,
  isDraft,
  versionSeq,
  controls,
  actionError,
  readOnly = false,
  className,
}: ResultToolbarProps) {
  const subtitle = model
    ? `${model.city} · ${model.overview.days} 天${versionSeq ? ` · v${versionSeq}` : ""}`
    : "等待生成";

  return (
    <div
      className={cn(
        "space-y-2 rounded-md border border-hairline bg-paper px-3 py-2",
        className,
      )}
    >
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
        <div className="min-w-0 flex-1">
          <h2 className="truncate font-serif text-base leading-tight text-ink">
            {model?.title ?? "行程方案"}
          </h2>
          <p className="mt-0.5 flex items-center gap-2 text-xs text-muted-foreground">
            <span className="truncate">{subtitle}</span>
            <StatusDot tone={isDraft ? "warning" : "success"} label={isDraft ? "草稿" : "已确认"} />
            {isDraft && <Badge variant="warning">草稿预览</Badge>}
          </p>
        </div>

        <ViewSwitcher value={view} onChange={onViewChange} />

        {!readOnly && controls && (
          <>
            <div className="ml-auto hidden min-h-11 items-center gap-1 md:flex">{controls}</div>
            <div className="md:hidden">
              <Popover>
                <PopoverTrigger asChild>
                  <Button size="sm" variant="ghost">
                    更多
                  </Button>
                </PopoverTrigger>
                <PopoverContent className="w-64 space-y-3">{controls}</PopoverContent>
              </Popover>
            </div>
          </>
        )}
      </div>

      {showDayTabs && model && model.days.length > 0 && (
        <DayTabs
          days={model.days}
          activeDay={activeDay}
          onChange={onActiveDayChange}
          allowAll={allowAll}
        />
      )}

      {actionError && (
        <p role="alert" className="text-xs text-danger">
          {actionError}
        </p>
      )}
    </div>
  );
}