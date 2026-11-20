import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import type { Meta } from "@/api/types";
import type {
  AccommodationBrief,
  GuideSignal,
  ResearchBrief,
  ResearchHotel,
} from "./types";

export interface ResearchPanelProps {
  research: ResearchBrief | null;
  plugins?: Meta["plugins"] | null;
  loading?: boolean;
  poiNames?: Record<string, string>;
}

export function ResearchPanel({ research, plugins, loading = false, poiNames = {} }: ResearchPanelProps) {
  if (loading) {
    return <p className="text-sm text-muted-foreground">正在整理住宿与攻略参考…</p>;
  }

  const hotelsEnabled = plugins?.hotels?.enabled !== false;
  const guidesEnabled = plugins?.guides?.enabled !== false;
  const accommodation = research?.accommodation ?? null;
  const guides = research?.guides ?? [];

  return (
    <div className="space-y-4">
      <section className="space-y-2" aria-label="住宿建议">
        <div className="flex items-center justify-between">
          <h3 className="text-sm font-medium">住宿建议</h3>
          {research?.status === "degraded" && <Badge variant="warning">部分数据不可用</Badge>}
        </div>
        {!hotelsEnabled ? (
          <p className="text-sm text-muted-foreground">
            住宿插件未启用{plugins?.hotels?.disabled_reason ? `：${plugins.hotels.disabled_reason}` : ""}
          </p>
        ) : accommodation ? (
          <AccommodationView accommodation={accommodation} />
        ) : (
          <p className="text-sm text-muted-foreground">本次未识别到住宿需求。</p>
        )}
      </section>

      <section className="space-y-2 border-t pt-3" aria-label="攻略参考">
        <h3 className="text-sm font-medium">攻略参考</h3>
        {!guidesEnabled ? (
          <p className="text-sm text-muted-foreground">
            攻略插件未启用{plugins?.guides?.disabled_reason ? `：${plugins.guides.disabled_reason}` : ""}
          </p>
        ) : guides.length > 0 ? (
          <ul className="space-y-3">
            {guides.map((guide, index) => (
              <GuideItem key={`${guide.poi_id ?? "guide"}-${index}`} guide={guide} poiNames={poiNames} />
            ))}
          </ul>
        ) : (
          <p className="text-sm text-muted-foreground">
            {research?.status === "degraded" ? "攻略参考不可用。" : "未找到与候选点对应的攻略摘要。"}
          </p>
        )}
        <p className="text-xs text-muted-foreground">外部摘要，仅供参考。</p>
      </section>

      {(research?.notes?.length ?? 0) > 0 && (
        <ul className="space-y-1 text-xs text-muted-foreground">
          {research?.notes?.map((note) => (
            <li key={note}>{note}</li>
          ))}
        </ul>
      )}
      {(research?.warnings?.length ?? 0) > 0 && (
        <ul className="space-y-1 text-xs text-amber-700">
          {research?.warnings?.map((warning) => (
            <li key={warning}>{warning}</li>
          ))}
        </ul>
      )}
    </div>
  );
}

function AccommodationView({ accommodation }: { accommodation: AccommodationBrief }) {
  const selected = accommodation.selected;
  const specified = accommodation.specified ?? [];
  const userPicked = Boolean(selected?.name && specified.includes(selected.name));
  if (selected) {
    return (
      <Card>
        <CardContent className="space-y-2 pt-4">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-medium">{selected.name ?? "已选酒店"}</span>
            {userPicked && <Badge variant="secondary">你指定的酒店</Badge>}
            {selected.star ? <Badge variant="outline">{selected.star} 星</Badge> : null}
            {typeof selected.score === "number" ? <span className="text-xs text-muted-foreground">{selected.score} 分</span> : null}
          </div>
          {selected.address && <p className="text-xs text-muted-foreground">{selected.address}</p>}
          <PriceLine hotel={selected} accommodation={accommodation} />
          <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
            {selected.provider && <span>来源：{selected.provider}</span>}
            {selected.fetched_at && <span>抓取于 {formatDate(selected.fetched_at)}</span>}
            {accommodation.anchor_days?.length ? (
              <span>锚点：{accommodation.anchor_days.map((day) => `第 ${day + 1} 天`).join("、")}</span>
            ) : null}
          </div>
          {safeURL(selected.booking_url) && (
            <a
              className="text-xs text-primary underline"
              href={selected.booking_url}
              target="_blank"
              rel="noopener noreferrer"
            >
              查看预订
            </a>
          )}
          <p className="text-xs text-muted-foreground">价格估算，库存不保证。</p>
        </CardContent>
      </Card>
    );
  }

  const options = accommodation.options ?? [];
  if (options.length === 0) {
    return <p className="text-sm text-muted-foreground">暂未找到可推荐的住宿。</p>;
  }
  return (
    <ul className="space-y-2">
      {options.map((hotel, index) => (
        <li key={`${hotel.id ?? hotel.name ?? "hotel"}-${index}`} className="rounded-md border p-2 text-sm">
          <div className="flex items-center justify-between gap-2">
            <span className="font-medium">{hotel.name ?? "候选酒店"}</span>
            <span className="text-xs text-muted-foreground">
              {(hotel.price_per_night_yuan ?? 0) > 0 ? `¥${hotel.price_per_night_yuan}/晚` : "价格未知"}
            </span>
          </div>
          {hotel.address && <p className="text-xs text-muted-foreground">{hotel.address}</p>}
        </li>
      ))}
    </ul>
  );
}

function PriceLine({ hotel, accommodation }: { hotel: ResearchHotel; accommodation: AccommodationBrief }) {
  const nightly = hotel.price_per_night_yuan ?? accommodation.nightly_yuan ?? 0;
  const total = accommodation.total_yuan ?? 0;
  if (nightly <= 0 && total <= 0) {
    return <p className="text-sm text-muted-foreground">未获取价格，未计入预算。</p>;
  }
  return (
    <p className="text-sm">
      {nightly > 0 && `¥${nightly}/晚`}
      {accommodation.nights ? ` · ${accommodation.nights} 晚` : ""}
      {total > 0 && ` · 合计 ¥${total}`}
      {(hotel.price_estimated || accommodation.price_estimated) && (
        <Badge variant="warning" className="ml-2">
          估算
        </Badge>
      )}
    </p>
  );
}

function GuideItem({ guide, poiNames }: { guide: GuideSignal; poiNames: Record<string, string> }) {
  const name = (guide.poi_id && poiNames[guide.poi_id]) || guide.poi_id || "候选点";
  const weight = guide.weight ?? 0;
  return (
    <li className="space-y-1 text-sm">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-medium">{name}</span>
        {weight > 0 && <Badge variant="outline">权重 {weight.toFixed(1)}</Badge>}
        {(guide.mentions ?? 0) > 0 && (
          <span className="text-xs text-muted-foreground">提及 {guide.mentions} 次</span>
        )}
      </div>
      {guide.note && <p className="text-xs text-muted-foreground">{guide.note}</p>}
      {guide.citations && guide.citations.length > 0 && (
        <ul className="space-y-0.5">
          {guide.citations.map((citation, index) => {
            const label = [citation.title || "外部链接", citation.site, formatDate(citation.published_at ?? "")]
              .filter(Boolean)
              .join(" · ");
            return (
              <li key={`${citation.url ?? "citation"}-${index}`} className="text-xs">
                {safeURL(citation.url) ? (
                  <a
                    className="text-primary underline"
                    href={citation.url}
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    {label}
                  </a>
                ) : (
                  <span className="text-muted-foreground">{label}</span>
                )}
              </li>
            );
          })}
        </ul>
      )}
    </li>
  );
}

function safeURL(value: string | undefined): value is string {
  if (!value) return false;
  try {
    const url = new URL(value);
    return url.protocol === "http:" || url.protocol === "https:";
  } catch {
    return false;
  }
}

function formatDate(value: string): string {
  if (!value) return "";
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return value;
  return `${parsed.getFullYear()}-${String(parsed.getMonth() + 1).padStart(2, "0")}-${String(parsed.getDate()).padStart(2, "0")}`;
}
