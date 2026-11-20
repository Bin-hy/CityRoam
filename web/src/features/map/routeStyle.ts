export type RouteEmphasis = "default" | "selected" | "muted";

export interface RouteStyle {
  color: string;
  dashed: boolean;
  weight: number;
  opacity: number;
  title: string;
}

export interface RouteStops {
  fromStop?: string;
  toStop?: string;
}

export const modeLegend: ReadonlyArray<{ label: string; color: string; dashed: boolean }> = [
  { label: "步行", color: "#64748b", dashed: true },
  { label: "公交", color: "#2563eb", dashed: false },
  { label: "地铁", color: "#f97316", dashed: false },
  { label: "驾车", color: "#7c3aed", dashed: false },
];

export function routeStyle(
  mode: string,
  lineName?: string,
  emphasis: RouteEmphasis = "default",
  stops?: RouteStops,
): RouteStyle {
  const title = [lineName, stops?.fromStop, stops?.toStop].filter(Boolean).join(" · ");
  const weight = emphasis === "selected" ? 6 : 4;
  const opacity = emphasis === "selected" ? 1 : emphasis === "muted" ? 0.25 : 0.9;

  if (mode === "drive") {
    return { color: "#7c3aed", dashed: false, weight, opacity, title };
  }
  if (mode === "walk") {
    return { color: "#64748b", dashed: true, weight, opacity, title };
  }
  if (mode === "transit" || mode === "bus" || mode === "subway") {
    const subway = mode === "subway" || lineName?.includes("地铁") === true;
    return { color: subway ? "#f97316" : "#2563eb", dashed: false, weight, opacity, title };
  }
  return { color: "#94a3b8", dashed: true, weight, opacity, title };
}