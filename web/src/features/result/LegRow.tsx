import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { formatYuan } from "@/design/tokens";
import type { ResultLeg } from "./model";

export interface LegRowProps {
  leg: ResultLeg;
  selected: boolean;
  onSelect: () => void;
}

export function LegRow({ leg, selected, onSelect }: LegRowProps) {
  const lineNames = uniqueLines(leg);

  return (
    <button
      type="button"
      onClick={onSelect}
      aria-pressed={selected}
      className={cn(
        "flex w-full flex-wrap items-center gap-x-2 gap-y-1 rounded-md border px-2.5 py-1.5 text-left text-xs transition-colors",
        selected
          ? "border-forest/40 bg-forest/5 text-forest"
          : "border-hairline bg-paper/60 text-muted-foreground hover:border-forest/30",
      )}
    >
      <span className="font-medium text-ink">{leg.modeLabel}</span>
      {lineNames && <span>{lineNames}</span>}
      <span className="font-mono tabular-nums">{leg.durationMin} 分钟</span>
      {leg.costYuan > 0 && <span className="font-mono tabular-nums">{formatYuan(leg.costYuan)}</span>}
      {leg.estimated && <Badge variant="warning">估算</Badge>}
    </button>
  );
}

function uniqueLines(leg: ResultLeg): string {
  const seen = new Set<string>();
  const lines: string[] = [];
  for (const segment of leg.segments) {
    const line = segment.lineName?.trim();
    if (!line || seen.has(line)) continue;
    seen.add(line);
    lines.push(line);
  }
  return lines.join(" / ");
}