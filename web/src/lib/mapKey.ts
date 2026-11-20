export const AMAP_KEY = ((import.meta.env.VITE_AMAP_JS_KEY as string | undefined) ?? "").trim();

export function hasAMapKey(): boolean {
  return AMAP_KEY.length > 0;
}