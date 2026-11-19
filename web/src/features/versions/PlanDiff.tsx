import { Badge } from "@/components/ui/badge";
import type { BlockDiff, ChangeKind, DayDiff } from "./diffModel";
import { fieldLabel, formatFieldValue } from "./diffModel";

const changeLabels: Record<ChangeKind, string> = {
  added: "新增",
  removed: "删除",
  modified: "修改",
};

const changeVariants: Record<ChangeKind, "success" | "destructive" | "warning"> = {
  added: "success",
  removed: "destructive",
  modified: "warning",
};

function blockSummary(block: BlockDiff): string {
  const value = block.after ?? block.before;
  if (!value) return block.blockID;
  const cost = value.cost_yuan > 0 ? ` · ¥${value.cost_yuan}` : "";
  return `${value.start}–${value.end}${cost}`;
}

function BlockRow({ block }: { block: BlockDiff }) {
  return (
    <li className="rounded-md border bg-background/60 px-2 py-1.5">
      <div className="flex flex-wrap items-center gap-2 text-sm">
        <Badge variant={changeVariants[block.change]}>{changeLabels[block.change]}</Badge>
        <span className="font-medium">{block.title || block.blockID}</span>
        <span className="text-xs text-muted-foreground">{blockSummary(block)}</span>
      </div>
      {block.fields.length > 0 && (
        <ul className="mt-1 space-y-0.5 text-xs text-muted-foreground">
          {block.fields.map((field) => (
            <li key={field.field}>
              {fieldLabel(field.field)}：{formatFieldValue(field.field, field.before)} →{" "}
              <span className="text-foreground">{formatFieldValue(field.field, field.after)}</span>
            </li>
          ))}
        </ul>
      )}
    </li>
  );
}

export function PlanDiff({ days, from, to }: { days: DayDiff[]; from?: number; to?: number }) {
  if (days.length === 0) {
    return <p className="text-sm text-muted-foreground">两个版本内容一致。</p>;
  }

  return (
    <div className="space-y-3">
      {from !== undefined && to !== undefined && (
        <p className="text-xs text-muted-foreground">
          对比 v{from} → v{to}，共 {days.length} 天有变化。
        </p>
      )}
      {days.map((day) => (
        <section key={day.dayIndex} aria-label={`第 ${day.dayIndex + 1} 天`} className="space-y-1.5">
          <header className="flex flex-wrap items-center gap-2 text-sm font-medium">
            <span>第 {day.dayIndex + 1} 天</span>
            {day.date && <span className="text-xs text-muted-foreground">{day.date}</span>}
            <Badge variant={changeVariants[day.change]}>{changeLabels[day.change]}</Badge>
          </header>
          {day.fields.length > 0 && (
            <ul className="space-y-0.5 text-xs text-muted-foreground">
              {day.fields.map((field) => (
                <li key={field.field}>
                  {fieldLabel(field.field)}：{formatFieldValue(field.field, field.before)} →{" "}
                  <span className="text-foreground">{formatFieldValue(field.field, field.after)}</span>
                </li>
              ))}
            </ul>
          )}
          {day.blocks.length > 0 && (
            <ul className="space-y-1">
              {day.blocks.map((block) => (
                <BlockRow key={`${day.dayIndex}-${block.blockID}`} block={block} />
              ))}
            </ul>
          )}
        </section>
      ))}
    </div>
  );
}
