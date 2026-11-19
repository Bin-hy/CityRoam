import { load } from "@amap/amap-jsapi-loader";

export const AMAP_KEY = ((import.meta.env.VITE_AMAP_JS_KEY as string | undefined) ?? "").trim();
export const AMAP_SECURITY_CODE = ((import.meta.env.VITE_AMAP_SECURITY_CODE as string | undefined) ?? "").trim();

export function hasAMapKey(): boolean {
  return AMAP_KEY.length > 0;
}

export interface AMapMapInstance {
  destroy(): void;
  remove(overlays: unknown | unknown[]): void;
  setFitView(overlays?: unknown[]): void;
  setZoomAndCenter(zoom: number, center: [number, number]): void;
}

export interface AMapMarkerInstance {
  on(event: "click", handler: () => void): void;
  getPosition(): [number, number];
  setzIndex(index: number): void;
  setMap(map: AMapMapInstance | null): void;
}

export interface AMapPolylineInstance {
  setMap(map: AMapMapInstance | null): void;
}

export interface AMapInfoWindowInstance {
  open(map: AMapMapInstance, position: [number, number]): void;
  close(): void;
}

export interface AMapNamespace {
  Map: new (container: HTMLElement, options: Record<string, unknown>) => AMapMapInstance;
  Marker: new (options: Record<string, unknown>) => AMapMarkerInstance;
  Polyline: new (options: Record<string, unknown>) => AMapPolylineInstance;
  InfoWindow: new (options: Record<string, unknown>) => AMapInfoWindowInstance;
}

let pending: Promise<AMapNamespace> | null = null;

export function loadAMap(): Promise<AMapNamespace> {
  if (!AMAP_KEY) {
    return Promise.reject(new Error("未配置 VITE_AMAP_JS_KEY"));
  }
  if (!pending) {
    if (AMAP_SECURITY_CODE) {
      (window as unknown as { _AMapSecurityConfig?: { securityJsCode: string } })._AMapSecurityConfig = {
        securityJsCode: AMAP_SECURITY_CODE,
      };
    }
    pending = load({ key: AMAP_KEY, version: "2.0" })
      .then((namespace) => namespace as AMapNamespace)
      .catch((error: unknown) => {
        pending = null;
        throw error;
      });
  }
  return pending;
}
