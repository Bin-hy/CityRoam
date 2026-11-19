import { Badge } from "@/components/ui/badge";
import type { Itinerary } from "@/features/itinerary/types";

export interface BudgetBreakdown {
  tickets: number;
  meals: number;
  transit: number;
  total: number;
  poiCount: number;
  walkingKM: number;
  estimated: boolean;
}

export function budgetBreakdown(itinerary: Itinerary): BudgetBreakdown {
  let tickets = 0;
  let meals = 0;
  let transit = 0;
  let total = 0;
  let poiCount = 0;
  let estimated = false;

  for (const day of itinerary.days) {
    for (const block of day.blocks) {
      const cost = Number.isFinite(block.cost_yuan) ? block.cost_yuan : 0;
      total += cost;
      switch (block.type) {
        case "poi":
          tickets += cost;
          poiCount++;
          break;
        case "meal":
          meals += cost;
          break;
        case "transit":
          transit += cost;
          break;
      }
      if (block.transport?.estimated) {
        estimated = true;
      }
    }
  }

  return {
    tickets,
    meals,
    transit,
    total,
    poiCount,
    walkingKM: itinerary.totals.walking_km,
    estimated,
  };
}

export function BudgetPanel({ itinerary }: { itinerary: Itinerary | null }) {
  if (!itinerary) {
    return <p className="text-sm text-muted-foreground">暂无预算。</p>;
  }
  const budget = budgetBreakdown(itinerary);
  const rows: Array<[string, number]> = [
    ["门票", budget.tickets],
    ["餐饮", budget.meals],
    ["交通", budget.transit],
  ];

  return (
    <div className="space-y-3">
      <dl className="space-y-2 text-sm">
        {rows.map(([label, value]) => (
          <div key={label} className="flex items-center justify-between">
            <dt className="text-muted-foreground">{label}</dt>
            <dd>¥{value}</dd>
          </div>
        ))}
        <div className="flex items-center justify-between border-t pt-2 font-semibold">
          <dt>总计</dt>
          <dd>¥{budget.total}</dd>
        </div>
      </dl>
      <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
        <span>{budget.poiCount} 个景点</span>
        <span>·</span>
        <span>步行 {budget.walkingKM.toFixed(1)} km</span>
        {budget.estimated && <Badge variant="warning">含估算交通</Badge>}
      </div>
      <p className="text-xs text-muted-foreground">价格为估算，门票以景区实际公示为准。</p>
    </div>
  );
}
