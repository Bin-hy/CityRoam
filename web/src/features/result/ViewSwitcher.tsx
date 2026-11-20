import { useRef, type KeyboardEvent } from "react";
import { cn } from "@/lib/utils";
import type { ResultViewMode } from "./model";

export interface ViewSwitcherProps {
  value: ResultViewMode;
  onChange: (view: ResultViewMode) => void;
  size?: "sm" | "md";
}

const views: Array<{ mode: ResultViewMode; label: string }> = [
  { mode: "dossier", label: "手册" },
  { mode: "map", label: "地图" },
  { mode: "board", label: "看板" },
];

export function ViewSwitcher({ value, onChange, size = "sm" }: ViewSwitcherProps) {
  const tabsRef = useRef<Array<HTMLButtonElement | null>>([]);

  const move = (event: KeyboardEvent<HTMLButtonElement>, index: number) => {
    if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
    event.preventDefault();
    let next = index;
    if (event.key === "Home") next = 0;
    else if (event.key === "End") next = views.length - 1;
    else if (event.key === "ArrowLeft") next = (index - 1 + views.length) % views.length;
    else next = (index + 1) % views.length;
    onChange(views[next].mode);
    tabsRef.current[next]?.focus();
  };

  return (
    <div role="tablist" aria-label="结果视图" className="inline-flex rounded-md border border-hairline bg-paper p-0.5">
      {views.map((view, index) => {
        const active = view.mode === value;
        return (
          <button
            key={view.mode}
            ref={(node) => {
              tabsRef.current[index] = node;
            }}
            type="button"
            role="tab"
            aria-selected={active}
            tabIndex={active ? 0 : -1}
            onClick={() => onChange(view.mode)}
            onKeyDown={(event) => move(event, index)}
            className={cn(
              "rounded-[5px] font-medium transition-colors",
              "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
              size === "md" ? "px-3 py-1.5 text-sm" : "px-2.5 py-1 text-xs",
              active ? "bg-forest text-paper" : "text-muted-foreground hover:text-ink",
            )}
          >
            {view.label}
          </button>
        );
      })}
    </div>
  );
}