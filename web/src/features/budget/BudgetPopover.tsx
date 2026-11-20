import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Button } from "@/components/ui/button";
import { formatYuan } from "@/design/tokens";
import type { ResultBudget } from "@/features/result/model";

export interface BudgetPopoverProps {
  budget: ResultBudget | null;
  includeLodging?: boolean;
  readOnly?: boolean;
  pending?: boolean;
  onToggleLodging?: (include: boolean) => void;
}

export function BudgetPopover({
  budget,
  includeLodging = true,
  readOnly = false,
  pending = false,
  onToggleLodging,
}: BudgetPopoverProps) {
  if (!budget) {
    return (
      <Button size="sm" variant="ghost" disabled>
        暂无预算
      </Button>
    );
  }

  const hasLodging = budget.lodging > 0;
  const rows: Array<[string, number]> = [
    ["门票", budget.tickets],
    ["餐饮", budget.meals],
    ["交通", budget.transit],
  ];
  if (hasLodging) rows.push(["住宿", budget.lodging]);

  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button size="sm" variant="ghost" className="tabular-nums">
          总计 {formatYuan(budget.total)}
        </Button>
      </PopoverTrigger>
      <PopoverContent>
        <div className="space-y-3">
          <p className="font-serif text-sm text-ink">预算明细</p>
          <dl className="space-y-1.5 text-sm">
            {rows.map(([label, value]) => (
              <div key={label} className="flex items-center justify-between gap-4">
                <dt className="text-muted-foreground">{label}</dt>
                <dd className="tabular-nums">{formatYuan(value)}</dd>
              </div>
            ))}
            <div className="flex items-center justify-between gap-4 border-t border-hairline pt-2 font-medium">
              <dt>总计</dt>
              <dd className="tabular-nums">{formatYuan(budget.total)}</dd>
            </div>
          </dl>
          {(budget.estimated || hasLodging) && (
            <p className="text-xs text-muted-foreground">
              {budget.estimated && "含估算交通"}
              {budget.estimated && hasLodging && " · "}
              {hasLodging && "住宿价格为估算，库存不保证"}
            </p>
          )}
          {!readOnly && onToggleLodging && (
            <Button
              type="button"
              size="sm"
              variant="outline"
              className="w-full"
              disabled={pending}
              onClick={() => onToggleLodging(!includeLodging)}
            >
              {pending ? "处理中…" : includeLodging ? "不计住宿" : "计入住宿"}
            </Button>
          )}
        </div>
      </PopoverContent>
    </Popover>
  );
}