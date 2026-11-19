import { useEffect, useRef, useState } from "react";
import { AlertTriangle } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { dayDateLabel, type Itinerary } from "@/features/itinerary/types";
import {
  hasAMapKey,
  loadAMap,
  type AMapInfoWindowInstance,
  type AMapMapInstance,
  type AMapMarkerInstance,
  type AMapNamespace,
} from "@/lib/amap";

export const dayColors = ["#16a34a", "#2563eb", "#d97706", "#dc2626"];

interface TripMapProps {
  itinerary: Itinerary | null;
  selectedBlockID?: string | null;
  onSelectBlock?: (blockID: string) => void;
}

type MapState = "loading" | "ready" | "fallback";

export function TripMap({ itinerary, selectedBlockID, onSelectBlock }: TripMapProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const namespaceRef = useRef<AMapNamespace | null>(null);
  const mapRef = useRef<AMapMapInstance | null>(null);
  const overlaysRef = useRef<unknown[]>([]);
  const markersRef = useRef<Map<string, AMapMarkerInstance>>(new Map());
  const infoRef = useRef<AMapInfoWindowInstance | null>(null);
  const selectRef = useRef(onSelectBlock);
  selectRef.current = onSelectBlock;

  const [state, setState] = useState<MapState>(hasAMapKey() ? "loading" : "fallback");
  const [reason, setReason] = useState(hasAMapKey() ? "" : "未配置 VITE_AMAP_JS_KEY");

  useEffect(() => {
    if (!hasAMapKey()) {
      setState("fallback");
      setReason("未配置 VITE_AMAP_JS_KEY");
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
      });

    return () => {
      disposed = true;
      mapRef.current?.destroy();
      mapRef.current = null;
      namespaceRef.current = null;
      infoRef.current = null;
      markersRef.current.clear();
      overlaysRef.current = [];
    };
  }, []);

  useEffect(() => {
    const map = mapRef.current;
    const namespace = namespaceRef.current;
    if (state !== "ready" || !map || !namespace) return;

    if (overlaysRef.current.length > 0) {
      map.remove(overlaysRef.current);
      overlaysRef.current = [];
    }
    markersRef.current.clear();
    if (!itinerary) return;

    const overlays: unknown[] = [];
    itinerary.days.forEach((day, dayIndex) => {
      const path: [number, number][] = [];
      const color = dayColors[dayIndex % dayColors.length];
      day.blocks.forEach((block) => {
        if (!Number.isFinite(block.lat) || !Number.isFinite(block.lng)) return;
        if (block.lat === 0 && block.lng === 0) return;
        const position: [number, number] = [block.lng as number, block.lat as number];
        path.push(position);
        const marker = new namespace.Marker({
          position,
          title: block.title,
          map,
          zIndex: 10,
        });
        marker.on("click", () => selectRef.current?.(block.id));
        markersRef.current.set(block.id, marker);
        overlays.push(marker);
      });
      if (path.length > 1) {
        overlays.push(
          new namespace.Polyline({
            path,
            strokeColor: color,
            strokeWeight: 4,
            strokeOpacity: 0.85,
            lineJoin: "round",
            map,
          }),
        );
      }
    });
    overlaysRef.current = overlays;
    if (overlays.length > 0) {
      map.setFitView(overlays);
    }
  }, [itinerary, state]);

  useEffect(() => {
    const map = mapRef.current;
    const namespace = namespaceRef.current;
    if (state !== "ready" || !map || !namespace) return;

    markersRef.current.forEach((marker, id) => marker.setzIndex(id === selectedBlockID ? 100 : 10));
    if (!selectedBlockID) {
      infoRef.current?.close();
      infoRef.current = null;
      return;
    }
    const marker = markersRef.current.get(selectedBlockID);
    if (!marker) return;
    map.setZoomAndCenter(14, marker.getPosition());
    const block = itinerary?.days.flatMap((day) => day.blocks).find((item) => item.id === selectedBlockID);
    infoRef.current?.close();
    infoRef.current = new namespace.InfoWindow({
      content: `<strong>${escapeHTML(block?.title ?? "")}</strong>`,
    });
    infoRef.current.open(map, marker.getPosition());
  }, [selectedBlockID, itinerary, state]);

  return (
    <div className="space-y-3">
      <div className="relative min-h-64 overflow-hidden rounded-lg border bg-muted">
        <div ref={containerRef} className={state === "ready" ? "h-72 w-full" : "hidden"} data-testid="trip-map" />
        {state !== "ready" && (
          <MapFallback state={state} reason={reason} itinerary={itinerary} />
        )}
      </div>
      {state === "ready" && <MapLegend itinerary={itinerary} />}
    </div>
  );
}

function MapFallback({ state, reason, itinerary }: { state: MapState; reason: string; itinerary: Itinerary | null }) {
  return (
    <div className="space-y-2 p-4">
      <p className="flex items-center gap-2 text-sm font-medium">
        <AlertTriangle className="h-4 w-4 text-amber-500" />
        {state === "loading" ? "地图加载中…" : "地图暂不可用"}
      </p>
      {reason && <p className="text-xs text-muted-foreground">{reason}。时间轴和预算仍可正常使用。</p>}
      {itinerary && (
        <ul className="grid gap-1 text-xs text-muted-foreground sm:grid-cols-2">
          {itinerary.days.flatMap((day, dayIndex) =>
            day.blocks.map((block) => (
              <li key={block.id}>
                第 {dayIndex + 1} 天 · {block.start} {block.title}
              </li>
            )),
          )}
        </ul>
      )}
    </div>
  );
}

function MapLegend({ itinerary }: { itinerary: Itinerary | null }) {
  if (!itinerary) return null;
  return (
    <div className="flex flex-wrap gap-3 text-xs text-muted-foreground">
      {itinerary.days.map((day, index) => (
        <span key={`${day.date}-${index}`} className="flex items-center gap-1">
          <span
            className="inline-block h-2.5 w-2.5 rounded-full"
            style={{ backgroundColor: dayColors[index % dayColors.length] }}
            aria-hidden
          />
          第 {index + 1} 天 · {dayDateLabel(day.date, index)}
        </span>
      ))}
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
