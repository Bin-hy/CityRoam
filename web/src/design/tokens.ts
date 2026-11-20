export const palette = {
  paper: "#FAF7F1",
  ink: "#1F2420",
  forest: "#1F4D3A",
  clay: "#A65B35",
  sand: "#E8DECF",
  hairline: "#DED5C6",
  success: "#2F6B4F",
  warning: "#B7791F",
  danger: "#B4443D",
} as const;

export const dayThemes = [
  { color: "#1F4D3A", soft: "#E7EFE9" },
  { color: "#2B4C7E", soft: "#E8EDF5" },
  { color: "#A65B35", soft: "#F3E7DF" },
  { color: "#6B3F63", soft: "#EEE7EC" },
] as const;

export function dayColor(index: number): string {
  if (!Number.isFinite(index)) return dayThemes[0].color;
  const normalized = ((Math.trunc(index) % dayThemes.length) + dayThemes.length) % dayThemes.length;
  return dayThemes[normalized].color;
}

export const fonts = {
  serif: '"Noto Serif SC", "Songti SC", STSong, Georgia, serif',
  sans: 'ui-sans-serif, system-ui, -apple-system, "PingFang SC", "Microsoft YaHei", sans-serif',
  numeric: 'ui-monospace, SFMono-Regular, Menlo, monospace',
} as const;

export function formatYuan(value: number): string {
  const amount = Number.isFinite(value) ? Math.round(value) : 0;
  return `¥${new Intl.NumberFormat("zh-CN").format(amount)}`;
}

export function formatDateLabel(date: string, index: number): string {
  if (!date) return `第 ${index + 1} 天`;
  const parsed = new Date(`${date}T00:00:00`);
  if (Number.isNaN(parsed.getTime())) return date;
  const weekdays = ["周日", "周一", "周二", "周三", "周四", "周五", "周六"];
  return `${parsed.getMonth() + 1} 月 ${parsed.getDate()} 日 ${weekdays[parsed.getDay()]}`;
}

export function formatVersion(seq: number): string {
  return `v${Number.isFinite(seq) ? Math.trunc(seq) : 0}`;
}
