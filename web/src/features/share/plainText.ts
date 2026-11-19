import type { Itinerary } from "@/features/itinerary/types";
import { dayDateLabel } from "@/features/itinerary/types";

const blockTypeLabels: Record<string, string> = {
  poi: "景点",
  meal: "餐饮",
  transit: "交通",
  rest: "休息",
};

export function itineraryToPlainText(itinerary: Itinerary): string {
  const lines: string[] = [
    `${itinerary.city === "beijing" || !itinerary.city ? "北京" : itinerary.city} ${itinerary.days.length} 天行程`,
    "来源：TripPilot 分享",
    "AI 生成，价格与时间为估算，出行前请核实开放时间与门票。",
    "",
  ];

  itinerary.days.forEach((day, index) => {
    lines.push(`第 ${index + 1} 天 · ${dayDateLabel(day.date, index)}`);
    day.blocks.forEach((block) => {
      const type = blockTypeLabels[block.type] ?? block.type;
      const cost = block.cost_yuan > 0 ? ` ¥${block.cost_yuan}` : "";
      lines.push(`${block.start}-${block.end} ${type} ${block.title}${cost}`.trim());
    });
    lines.push(`当天小计 ¥${day.day_cost_yuan}`);
    lines.push("");
  });

  lines.push(
    `总计：¥${itinerary.totals.cost_yuan} · 景点 ${itinerary.totals.poi_count} 个 · 步行 ${itinerary.totals.walking_km} km`,
  );
  return lines.join("\n");
}
