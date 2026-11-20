import type { Itinerary } from "@/features/itinerary/types";

export interface BudgetBreakdown {
  tickets: number;
  meals: number;
  transit: number;
  lodging: number;
  total: number;
  poiCount: number;
  walkingKM: number;
  estimated: boolean;
  hasLodging: boolean;
}

export interface BudgetSummary {
  total: number;
  hasLodging: boolean;
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

  const lodging = itinerary.totals.lodging_yuan ?? 0;
  return {
    tickets,
    meals,
    transit,
    lodging,
    total: total + lodging,
    poiCount,
    walkingKM: itinerary.totals.walking_km,
    estimated,
    hasLodging: lodging > 0,
  };
}

export function budgetSummary(itinerary: Itinerary | null): BudgetSummary {
  if (!itinerary) return { total: 0, hasLodging: false };
  const budget = budgetBreakdown(itinerary);
  return { total: budget.total, hasLodging: budget.hasLodging };
}
