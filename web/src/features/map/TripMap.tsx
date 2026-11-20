import { useEffect, useRef, useState } from "react";
import { AlertTriangle } from "lucide-react";
import { cn } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { palette } from "@/design/tokens";
import { modeLegend, routeStyle, type RouteEmphasis } from "@/features/map/routeStyle";
import type { ResultDay, ResultHotel, ResultLeg } from "@/features/result/model";
import {
  hasAMapKey,
  loadAMap,
  type AMapMapInstance,
  type AMapMarkerInstance,
  type AMapNamespace,
  type AMapPolylineInstance,
} from "@/lib/amap";

export interface TripMapProps {
  day: ResultDay;
  hotel: ResultHotel | null;
  selectedStopID?: string | null;
  selectedLegID?: string | null;
  onSelectStop?: (stopID: string) => void;
  onSelectLeg?: (legID: string) => void;
  onSelectHotel?: () => void;
  onUnavailable?: () => void;
  className?: string;
}

type MapState = "loading" | "ready" | "fallback";

interface LegShape {
  instance: AMapPolylineInstance;
  mode: string;
  lineName?: string;
  straight: boolean;
}

export function TripMap({
  day,
  hotel,
  selectedStopID,
  selectedLegID,
  onSelectStop,
  onSelectLeg,
  onSelectHotel,
  onUnavailable,
  className,
}: TripMapProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const namespaceRef = useRef<AMapNamespace | null>(null);
  const mapRef = useRef<AMapMapInstance | null>(null);
  const overlaysRef = useRef<unknown[]>([]);
  const markersRef = useRef<Map<string, AMapMarkerInstance>>(new Map());
  const markerMetaRef = useRef<Map<string, { label: string; color: string }>>(new Map());
  const hotelMarkerRef = useRef<AMapMarkerInstance | null>(null);
  const legShapesRef = useRef<Map<string, LegShape[]>>(new Map());
  const fitTargetsRef = useRef<unknown[]>([]);
  const selectStopRef = useRef(onSelectStop);
  const selectLegRef = useRef(onSelectLeg);
  const selectHotelRef = useRef(onSelectHotel);
  const unavailableRef = useRef(onUnavailable);
  selectStopRef.current = onSelectStop;
  selectLegRef.current = onSelectLeg;
  selectHotelRef.current = onSelectHotel;
  unavailableRef.current = onUnavailable;

  const [state, setState] = useState<MapState>(hasAMapKey() ? "loading" : "fallback");
  const [reason, setReason] = useState(hasAMapKey() ? "" : "未配置 VITE_AMAP_JS_KEY");

  useEffect(() => {
    if (!hasAMapKey()) {
      setState("fallback");
      setReason("未配置 VITE_AMAP_JS_KEY");
      unavailableRef.current?.();
      return;
    }
    let disposed = false;
    loadAMap()
      .then((namespace) => {
        if (disposed || !containerRef.current) return;
        namespaceRef.current = namespace;
        mapRef.current = new namespace.Map(containerRef.current, {
          zoom: 12,
          center: [116.397, 39.909],
          viewMode: "2D",
          showLabel: true,
        });
        setState("ready");
      })
      .catch((error: unknown) => {
        if (disposed) return;
        setState("fallback");
        setReason(error instanceof Error ? error.message : "地图加载失败");
        unavailableRef.current?.();
      });

    return () => {
      disposed = true;
      mapRef.current?.destroy();
      mapRef.current = null;
      namespaceRef.current = null;
      hotelMarkerRef.current = null;
      markersRef.current.clear();
      markerMetaRef.current.clear();
      legShapesRef.current.clear();
      overlaysRef.current = [];
      fitTargetsRef.current = [];
    };
  }, []);

  useEffect(() => {
    const map = mapRef.current;
    const namespace = namespaceRef.current;
    if (state !== "ready" || !map || !namespace) return;

    if (overlaysRef.current.length > 0) map.remove(overlaysRef.current);
    overlaysRef.current = [];
    markersRef.current.clear();
    markerMetaRef.current.clear();
    legShapesRef.current.clear();
    hotelMarkerRef.current = null;
    fitTargetsRef.current = [];

    const overlays: unknown[] = [];
    const fitTargets: unknown[] = [];

    const hotelPosition = hotel ? positionOf(hotel.lat, hotel.lng) : null;
    if (hotel && hotelPosition) {
      const marker = new namespace.Marker({
        position: hotelPosition,
        title: escapeHTML(hotel.name),
        map,
        zIndex: 5,
        content: markerContent("宿", palette.forest, false),
      });
      marker.on("click", () => selectHotelRef.current?.());
      hotelMarkerRef.current = marker;
      overlays.push(marker);
      fitTargets.push(marker);
    }

    let number = 0;
    for (const stop of day.stops) {
      if (stop.type === "poi" || stop.type === "meal") number += 1;
      const position = stop.lat !== undefined && stop.lng !== undefined ? positionOf(stop.lat, stop.lng) : null;
      if (!position) continue;

      const label = stop.type === "poi" || stop.type === "meal" ? String(number) : "";
      const marker = new namespace.Marker({
        position,
        title: escapeHTML(stop.title),
        map,
        zIndex: 10,
        content: markerContent(label, palette.forest, false),
      });
      marker.on("click", () => selectStopRef.current?.(stop.id));
      markersRef.current.set(stop.id, marker);
      markerMetaRef.current.set(stop.id, { label, color: palette.forest });
      overlays.push(marker);
      fitTargets.push(marker);
    }

    for (const leg of day.legs) {
      const shapes = buildLegShapes(leg, namespace, map, overlays, (legID) =>
        selectLegRef.current?.(legID),
      );
      for (const shape of shapes) {
        shape.instance.setMap(map);
      }
      legShapesRef.current.set(leg.id, shapes);
    }

    overlaysRef.current = overlays;
    fitTargetsRef.current = fitTargets;
  }, [day, hotel, state]);

  useEffect(() => {
    const map = mapRef.current;
    if (state !== "ready" || !map) return;

    markersRef.current.forEach((marker, id) => {
      const selected = id === selectedStopID;
      marker.setzIndex(selected ? 100 : 10);
      const meta = markerMetaRef.current.get(id);
      if (meta) marker.setContent(markerContent(meta.label, meta.color, selected));
    });
    if (hotelMarkerRef.current) {
      hotelMarkerRef.current.setzIndex(5);
      hotelMarkerRef.current.setContent(markerContent("宿", palette.forest, false));
    }

    for (const [legID, shapes] of legShapesRef.current) {
      const emphasis: RouteEmphasis =
        legID === selectedLegID ? "selected" : selectedLegID || selectedStopID ? "muted" : "default";
      for (const shape of shapes) {
        const style = routeStyle(shape.mode, shape.lineName, emphasis, {
          fromStop: day.legs.find((leg) => leg.id === legID)?.from.title,
          toStop: day.legs.find((leg) => leg.id === legID)?.to.title,
        });
        shape.instance.setOptions?.({
          strokeColor: style.color,
          strokeWeight: style.weight,
          strokeOpacity: style.opacity,
          strokeStyle: shape.straight ? "dashed" : style.dashed ? "dashed" : "solid",
        });
      }
    }

    const selectedShapes = selectedLegID
      ? (legShapesRef.current.get(selectedLegID) ?? []).map((shape) => shape.instance)
      : [];
    if (selectedShapes.length > 0) {
      map.setFitView(selectedShapes);
    } else if (fitTargetsRef.current.length > 0) {
      map.setFitView(fitTargetsRef.current);
    }
  }, [selectedStopID, selectedLegID, day, state]);

  useEffect(() => {
    const map = mapRef.current;
    const container = containerRef.current;
    if (state !== "ready" || !map) return;

    let timer = 0;
    const resize = () => {
      window.clearTimeout(timer);
      timer = window.setTimeout(() => map.resize(), 150);
    };
    window.addEventListener("resize", resize);

    let observer: ResizeObserver | null = null;
    if (container && typeof ResizeObserver !== "undefined") {
      observer = new ResizeObserver(() => map.resize());
      observer.observe(container);
    }

    return () => {
      window.removeEventListener("resize", resize);
      window.clearTimeout(timer);
      observer?.disconnect();
    };
  }, [state]);

  return (
    <div className={cn("flex min-h-64 flex-col gap-2", className)}>
      <div className="relative min-h-64 flex-1 overflow-hidden rounded-md border border-hairline bg-sand/40">
        <div ref={containerRef} className={state === "ready" ? "h-full min-h-64 w-full" : "hidden"} data-testid="trip-map" />
        {state !== "ready" && (
          <MapFallback
            state={state}
            reason={reason}
            day={day}
            hotel={hotel}
            selectedStopID={selectedStopID}
            selectedLegID={selectedLegID}
            onSelectStop={onSelectStop}
            onSelectLeg={onSelectLeg}
            onSelectHotel={onSelectHotel}
          />
        )}
      </div>
      {state === "ready" && <MapLegend />}
    </div>
  );
}

function buildLegShapes(
  leg: ResultLeg,
  namespace: AMapNamespace,
  map: AMapMapInstance,
  overlays: unknown[],
  onSelect: (legID: string) => void,
): LegShape[] {
  const shapes: LegShape[] = [];
  const stops = { fromStop: leg.from.title, toStop: leg.to.title };

  if (leg.geometry === "polyline") {
    for (const segment of leg.segments) {
      if (segment.polyline.length < 2) continue;
      const style = routeStyle(segment.mode, segment.lineName, "default", stops);
      const instance = new namespace.Polyline({
        path: segment.polyline,
        strokeColor: style.color,
        strokeWeight: style.weight,
        strokeOpacity: style.opacity,
        strokeStyle: style.dashed ? "dashed" : "solid",
        lineJoin: "round",
        title: style.title,
        bubble: true,
        map,
      });
      instance.on?.("click", () => onSelect(leg.id));
      overlays.push(instance);
      shapes.push({ instance, mode: segment.mode, lineName: segment.lineName, straight: false });
    }
    return shapes;
  }

  if (leg.geometry === "straight") {
    const from = leg.from.lat !== undefined && leg.from.lng !== undefined ? positionOf(leg.from.lat, leg.from.lng) : null;
    const to = leg.to.lat !== undefined && leg.to.lng !== undefined ? positionOf(leg.to.lat, leg.to.lng) : null;
    if (!from || !to) return shapes;
    const style = routeStyle(leg.mode, undefined, "default", stops);
    const instance = new namespace.Polyline({
      path: [from, to],
      strokeColor: style.color,
      strokeWeight: style.weight,
      strokeOpacity: style.opacity,
      strokeStyle: "dashed",
      lineJoin: "round",
      title: "直线估算",
      bubble: true,
      map,
    });
    instance.on?.("click", () => onSelect(leg.id));
    overlays.push(instance);
    shapes.push({ instance, mode: leg.mode, lineName: undefined, straight: true });
  }

  return shapes;
}


function positionOf(lat: number, lng: number): [number, number] | null {
  if (!Number.isFinite(lat) || !Number.isFinite(lng)) return null;
  if (lat === 0 && lng === 0) return null;
  return [lng, lat];
}

function markerContent(label: string, color: string, selected: boolean): string {
  const size = selected ? 30 : 24;
  const ring = selected ? `0 0 0 4px ${color}33` : "0 1px 3px rgba(0,0,0,0.25)";
  return `<div style="display:flex;align-items:center;justify-content:center;width:${size}px;height:${size}px;border-radius:9999px;background:${color};color:#FAF7F1;font:600 12px/1 ui-monospace,monospace;border:2px solid #FAF7F1;box-shadow:${ring};">${escapeHTML(label)}</div>`;
}

function MapFallback({
  state,
  reason,
  day,
  hotel,
  selectedStopID,
  selectedLegID,
  onSelectStop,
  onSelectLeg,
  onSelectHotel,
}: {
  state: MapState;
  reason: string;
  day: ResultDay;
  hotel: ResultHotel | null;
  selectedStopID?: string | null;
  selectedLegID?: string | null;
  onSelectStop?: (stopID: string) => void;
  onSelectLeg?: (legID: string) => void;
  onSelectHotel?: () => void;
  className?: string;
}) {
  return (
    <div className="space-y-3 p-4">
      <p className="flex items-center gap-2 text-sm font-medium text-ink">
        <AlertTriangle className="h-4 w-4 text-warning" />
        {state === "loading" ? "地图加载中…" : "地图暂不可用"}
      </p>
      {reason && (
        <p className="text-xs text-muted-foreground">
          {reason}。地图暂不可用，仍可查看当天安排与路线详情。
        </p>
      )}
      <section aria-label={`第 ${day.index + 1} 天`} className="space-y-2">
        <h3 className="text-[11px] font-medium uppercase tracking-[0.16em] text-muted-foreground">
          第 {day.index + 1} 天 · {day.label}
        </h3>
        <ul className="flex flex-wrap gap-1.5">
          {day.stops.map((stop) => (
            <li key={stop.id}>
              <button
                type="button"
                aria-pressed={selectedStopID === stop.id}
                onClick={() => onSelectStop?.(stop.id)}
                className="rounded-full border border-hairline bg-paper px-2.5 py-1 text-xs text-ink hover:border-forest/50 aria-pressed:border-forest aria-pressed:bg-forest/10"
              >
                {stop.order + 1}. {stop.title}
              </button>
            </li>
          ))}
        </ul>
        {day.legs.length > 0 && (
          <ul className="flex flex-wrap gap-1.5">
            {day.legs.map((leg) => (
              <li key={leg.id}>
                <button
                  type="button"
                  aria-pressed={selectedLegID === leg.id}
                  onClick={() => onSelectLeg?.(leg.id)}
                  className="rounded-full border border-hairline bg-paper px-2.5 py-1 text-xs text-muted-foreground hover:border-forest/50 aria-pressed:border-forest"
                >
                  {leg.modeLabel} · {leg.from.title} → {leg.to.title}
                </button>
              </li>
            ))}
          </ul>
        )}
        {hotel && onSelectHotel && (
          <button
            type="button"
            onClick={onSelectHotel}
            className="rounded-full border border-hairline bg-paper px-2.5 py-1 text-xs text-ink hover:border-forest/50"
          >
            宿 · {hotel.name}
          </button>
        )}
      </section>
    </div>
  );
}

function MapLegend() {
  return (
    <div className="flex flex-wrap gap-3 text-xs text-muted-foreground">
      {modeLegend.map((item) => (
        <span key={item.label} className="flex items-center gap-1">
          <span
            className="inline-block h-0 w-4 border-t-2"
            style={{ borderColor: item.color, borderTopStyle: item.dashed ? "dashed" : "solid" }}
            aria-hidden
          />
          {item.label}
        </span>
      ))}
      <span className="flex items-center gap-1">
        <span className="inline-block h-0 w-4 border-t-2 border-dashed border-slate-400" aria-hidden />
        直线估算
      </span>
      <Badge variant="outline">GCJ-02</Badge>
    </div>
  );
}

function escapeHTML(value: string): string {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}
