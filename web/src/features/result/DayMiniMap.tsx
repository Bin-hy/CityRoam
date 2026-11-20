import type { KeyboardEvent } from "react";
import { SectionLabel } from "@/design/primitives";
import { Button } from "@/components/ui/button";
import { dayColor } from "@/design/tokens";
import { routeStyle, type RouteEmphasis } from "@/features/map/routeStyle";
import type { ResultDay, ResultHotel, ResultLeg, ResultPoint, ResultSelection, ResultStop } from "./model";

export interface DayMiniMapProps {
  day: ResultDay;
  hotel: ResultHotel | null;
  selection: ResultSelection;
  onSelect: (selection: ResultSelection) => void;
  onViewOnMap?: (dayIndex: number) => void;
  readOnly?: boolean;
  mapAvailable?: boolean;
}

const WIDTH = 320;
const HEIGHT = 180;
const PAD = 16;

interface Projected {
  x: number;
  y: number;
}

export function DayMiniMap({
  day,
  hotel,
  selection,
  onSelect,
  onViewOnMap,
  readOnly = false,
  mapAvailable = true,
}: DayMiniMapProps) {
  const project = buildProjection(day, hotel);
  const canDraw = mapAvailable && project !== null;
  const color = dayColor(day.index);

  return (
    <section aria-label={`第 ${day.index + 1} 天路线`} className="space-y-2">
      <SectionLabel>当天路线</SectionLabel>

      {canDraw ? (
        <svg
          viewBox={`0 0 ${WIDTH} ${HEIGHT}`}
          className="w-full rounded-md border border-hairline bg-paper/70"
          role="group"
          aria-label={`第 ${day.index + 1} 天路线缩略图`}
        >
          {day.legs.map((leg) => (
            <LegShape
              key={leg.id}
              leg={leg}
              project={project}
              emphasis={emphasisOf(leg, selection, day.index)}
              selected={isLegSelected(leg, selection, day.index)}
              onSelect={onSelect}
            />
          ))}
          {hotel && project(hotel.lat, hotel.lng) && (
            <HotelMarker hotel={hotel} at={project(hotel.lat, hotel.lng)!} />
          )}
          {day.stops.map((stop) =>
            stop.lat !== undefined && stop.lng !== undefined ? (
              <StopMarker
                key={stop.id}
                stop={stop}
                at={project(stop.lat, stop.lng)}
                color={color}
                selected={isStopSelected(stop, selection, day.index)}
                onSelect={onSelect}
              />
            ) : null,
          )}
        </svg>
      ) : (
        <div className="space-y-2 rounded-md border border-dashed border-hairline bg-paper/50 px-3 py-3">
          <p className="text-xs text-muted-foreground">
            {mapAvailable ? "缺少坐标，无法绘制当天路线" : "地图暂不可用，已切换为文字清单"}
          </p>
          {day.stops.length > 0 ? (
            <ul className="flex flex-wrap gap-1.5">
              {day.stops.map((stop) => (
                <li key={stop.id}>
                  <button
                    type="button"
                    onClick={() => onSelect({ kind: "stop", dayIndex: day.index, stopID: stop.id })}
                    className="rounded-full border border-hairline bg-paper px-2.5 py-1 text-xs text-ink hover:border-forest/40"
                  >
                    {stop.order + 1}. {stop.title}
                  </button>
                </li>
              ))}
            </ul>
          ) : (
            <p className="text-xs text-muted-foreground">当天没有停靠点。</p>
          )}
        </div>
      )}

      {!readOnly && onViewOnMap && mapAvailable && (
        <Button size="sm" variant="ghost" className="px-0" onClick={() => onViewOnMap(day.index)}>
          查看路线图
        </Button>
      )}
    </section>
  );
}

function LegShape({
  leg,
  project,
  emphasis,
  selected,
  onSelect,
}: {
  leg: ResultLeg;
  project: (lat: number, lng: number) => Projected | null;
  emphasis: RouteEmphasis;
  selected: boolean;
  onSelect: (selection: ResultSelection) => void;
}) {
  if (leg.geometry === "none") return null;

  const shapes: Array<{ points: Projected[]; mode: string; lineName?: string }> = [];

  if (leg.geometry === "polyline") {
    for (const segment of leg.segments) {
      if (segment.polyline.length < 2) continue;
      const points = segment.polyline.map(([lng, lat]) => project(lat, lng)).filter(isProjected);
      if (points.length > 1) shapes.push({ points, mode: segment.mode, lineName: segment.lineName });
    }
  } else {
    const from = pointAt(leg.from, project);
    const to = pointAt(leg.to, project);
    if (from && to) shapes.push({ points: [from, to], mode: leg.mode });
  }

  if (shapes.length === 0) return null;

  return (
    <g
      role="button"
      tabIndex={0}
      aria-label={`路段：${leg.from.title} → ${leg.to.title}`}
      aria-pressed={selected}
      className="cursor-pointer focus-visible:outline-none"
      onClick={() => onSelect({ kind: "leg", dayIndex: leg.dayIndex, legID: leg.id })}
      onKeyDown={activate(() =>
        onSelect({ kind: "leg", dayIndex: leg.dayIndex, legID: leg.id }),
      )}
    >
      {shapes.map((shape, index) => {
        const style = routeStyle(shape.mode, shape.lineName, emphasis, {
          fromStop: leg.from.title,
          toStop: leg.to.title,
        });
        const points = shape.points.map((point) => `${point.x},${point.y}`).join(" ");
        return (
          <g key={index}>
            <polyline
              points={points}
              fill="none"
              stroke="transparent"
              strokeWidth={14}
              strokeLinecap="round"
            />
            <polyline
              points={points}
              fill="none"
              stroke={style.color}
              strokeWidth={style.weight}
              strokeOpacity={style.opacity}
              strokeDasharray={style.dashed || leg.geometry === "straight" ? "6 4" : undefined}
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </g>
        );
      })}
    </g>
  );
}

function StopMarker({
  stop,
  at,
  color,
  selected,
  onSelect,
}: {
  stop: ResultStop;
  at: Projected | null;
  color: string;
  selected: boolean;
  onSelect: (selection: ResultSelection) => void;
}) {
  if (!at) return null;

  return (
    <g
      role="button"
      tabIndex={0}
      aria-label={`停靠点：${stop.title}`}
      aria-pressed={selected}
      className="cursor-pointer focus-visible:outline-none"
      onClick={() => onSelect({ kind: "stop", dayIndex: stop.dayIndex, stopID: stop.id })}
      onKeyDown={activate(() => onSelect({ kind: "stop", dayIndex: stop.dayIndex, stopID: stop.id }))}
    >
      {selected && <circle cx={at.x} cy={at.y} r={12} fill="none" stroke="#A65B35" strokeWidth={2} />}
      <circle cx={at.x} cy={at.y} r={9} fill={color} stroke="#FAF7F1" strokeWidth={2} />
      <text
        x={at.x}
        y={at.y + 0.5}
        textAnchor="middle"
        dominantBaseline="middle"
        fontSize={10}
        fontWeight={600}
        fill="#FAF7F1"
      >
        {stop.order + 1}
      </text>
    </g>
  );
}

function HotelMarker({ hotel, at }: { hotel: ResultHotel; at: Projected }) {
  return (
    <g role="img" aria-label={`住宿：${hotel.name}`}>
      <rect x={at.x - 8} y={at.y - 8} width={16} height={16} rx={4} fill="#1F4D3A" stroke="#FAF7F1" strokeWidth={2} />
      <text x={at.x} y={at.y + 0.5} textAnchor="middle" dominantBaseline="middle" fontSize={9} fontWeight={600} fill="#FAF7F1">
        宿
      </text>
    </g>
  );
}

function buildProjection(day: ResultDay, hotel: ResultHotel | null): ((lat: number, lng: number) => Projected | null) | null {
  const coords: Array<[number, number]> = [];
  for (const stop of day.stops) {
    if (stop.lat !== undefined && stop.lng !== undefined && validCoord(stop.lat, stop.lng)) {
      coords.push([stop.lat, stop.lng]);
    }
  }
  if (hotel && validCoord(hotel.lat, hotel.lng)) coords.push([hotel.lat, hotel.lng]);
  if (coords.length === 0) return null;

  const lats = coords.map(([lat]) => lat);
  const lngs = coords.map(([, lng]) => lng);
  const midLat = (Math.min(...lats) + Math.max(...lats)) / 2;
  const cos = Math.max(Math.cos((midLat * Math.PI) / 180), 0.2);

  const xs = lngs.map((lng) => lng * cos);
  const minX = Math.min(...xs);
  const maxX = Math.max(...xs);
  const minLat = Math.min(...lats);
  const maxLat = Math.max(...lats);
  const spanX = maxX - minX;
  const spanY = maxLat - minLat;

  return (lat: number, lng: number): Projected | null => {
    if (!validCoord(lat, lng)) return null;
    const x = spanX < 1e-9 ? WIDTH / 2 : PAD + ((lng * cos - minX) / spanX) * (WIDTH - 2 * PAD);
    const y = spanY < 1e-9 ? HEIGHT / 2 : HEIGHT - PAD - ((lat - minLat) / spanY) * (HEIGHT - 2 * PAD);
    return { x, y };
  };
}

function pointAt(point: ResultPoint, project: (lat: number, lng: number) => Projected | null): Projected | null {
  if (point.lat === undefined || point.lng === undefined) return null;
  return project(point.lat, point.lng);
}

function validCoord(lat: number, lng: number): boolean {
  return Number.isFinite(lat) && Number.isFinite(lng) && !(lat === 0 && lng === 0);
}

function isProjected(value: Projected | null): value is Projected {
  return value !== null;
}

function isStopSelected(stop: ResultStop, selection: ResultSelection, dayIndex: number): boolean {
  return selection?.kind === "stop" && selection.dayIndex === dayIndex && selection.stopID === stop.id;
}

function isLegSelected(leg: ResultLeg, selection: ResultSelection, dayIndex: number): boolean {
  return selection?.kind === "leg" && selection.dayIndex === dayIndex && selection.legID === leg.id;
}

function emphasisOf(leg: ResultLeg, selection: ResultSelection, dayIndex: number): RouteEmphasis {
  if (isLegSelected(leg, selection, dayIndex)) return "selected";
  return selection ? "muted" : "default";
}

function activate(run: () => void) {
  return (event: KeyboardEvent<SVGGElement>) => {
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      run();
    }
  };
}