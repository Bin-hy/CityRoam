import { useRef, type KeyboardEvent } from "react";
import { cn } from "@/lib/utils";
import { formatYuan } from "@/design/tokens";
import type { ResultDay } from "./model";

export interface DayTabsProps {
  days: ResultDay[];
  activeDay: number | null;
  onChange: (day: number | null) => void;
  allowAll?: boolean;
  controlsID?: string;
}

export function DayTabs({
  days,
  activeDay,
  onChange,
  allowAll = true,
  controlsID = "result-view",
}: DayTabsProps) {
  const tabsRef = useRef<Array<HTMLButtonElement | null>>([]);
  const valid = activeDay !== null && activeDay >= 0 && activeDay < days.length;
  const selected = valid ? activeDay : null;
  const selectedPosition = selected !== null ? selected + (allowAll ? 1 : 0) : allowAll ? 0 : -1;
  const count = days.length + (allowAll ? 1 : 0);

  const dayAt = (position: number): number | null =>
    allowAll ? (position === 0 ? null : position - 1) : position;

  const move = (event: KeyboardEvent<HTMLButtonElement>, position: number) => {
    if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
    if (count === 0) return;
    event.preventDefault();
    let next = position;
    if (event.key === "Home") next = 0;
    else if (event.key === "End") next = count - 1;
    else if (event.key === "ArrowLeft") next = (position - 1 + count) % count;
    else next = (position + 1) % count;
    onChange(dayAt(next));
    tabsRef.current[next]?.focus();
  };

  const tab = (position: number, label: string) => {
    const day = dayAt(position);
    const active = selectedPosition === position;
    return (
      <button
        key={position}
        ref={(node) => {
          tabsRef.current[position] = node;
        }}
        type="button"
        role="tab"
        aria-selected={active}
        aria-controls={controlsID}
        tabIndex={active || (selectedPosition < 0 && position === 0) ? 0 : -1}
        onClick={() => onChange(day)}
        onKeyDown={(event) => move(event, position)}
        className={cn(
          "shrink-0 whitespace-nowrap rounded-md px-2.5 py-1 text-xs transition-colors",
          "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
          active ? "bg-forest text-paper" : "text-muted-foreground hover:bg-sand/60 hover:text-ink",
        )}
      >
        {label}
      </button>
    );
  };

  return (
    <div
      role="tablist"
      aria-label="行程天数"
      className="flex items-center gap-1 overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
    >
      {allowAll && tab(0, "全部")}
      {days.map((day, index) => {
        const cost = Number.isFinite(day.costYuan) ? Math.round(day.costYuan) : 0;
        const label = `D${index + 1} · ${day.label}${cost > 0 ? ` · ${formatYuan(cost)}` : ""}`;
        return tab(index + (allowAll ? 1 : 0), label);
      })}
    </div>
  );
}