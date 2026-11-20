import { Badge } from "@/components/ui/badge";
import { SectionLabel } from "@/design/primitives";
import { formatYuan } from "@/design/tokens";
import type { AccommodationBrief, GuideCitation, GuideSignal, ResearchBrief } from "./types";

export interface ResearchNotesProps {
  research: ResearchBrief | null;
  poiNames?: Record<string, string>;
}

export function ResearchNotes({ research, poiNames = {} }: ResearchNotesProps) {
  if (!research) {
    return (
      <section aria-label="旅途中值得知道" className="space-y-2">
        <SectionLabel>旅途中值得知道</SectionLabel>
        <h2 className="font-serif text-xl text-ink">住宿与攻略</h2>
        <p className="rounded-md border border-dashed border-hairline bg-paper/60 px-4 py-3 text-sm leading-relaxed text-muted-foreground">
          此分享快照未包含住宿与攻略摘要。较早生成的行程只保留方案本身。
        </p>
      </section>
    );
  }

  const accommodation = research.accommodation ?? null;
  const guides = research.guides ?? [];
  const tips = [...(research.notes ?? []), ...(research.warnings ?? [])].filter((tip) => tip.trim() !== "");

  return (
    <section aria-label="旅途中值得知道" className="space-y-5">
      <header className="flex flex-wrap items-baseline justify-between gap-2">
        <div className="space-y-1">
          <SectionLabel>旅途中值得知道</SectionLabel>
          <h2 className="font-serif text-xl text-ink">住宿与攻略</h2>
        </div>
        {research.status === "degraded" && <Badge variant="warning">部分数据不可用</Badge>}
      </header>

      {accommodation ? <Accommodation accommodation={accommodation} /> : null}
      {guides.length > 0 ? <Guides guides={guides} poiNames={poiNames} /> : null}
      {!accommodation && guides.length === 0 && tips.length === 0 ? (
        <p className="text-sm text-muted-foreground">分享快照未包含住宿与攻略信息。</p>
      ) : null}

      {tips.length > 0 && (
        <div className="space-y-1.5">
          <SectionLabel>小提示</SectionLabel>
          <ul className="space-y-1 text-sm leading-relaxed text-ink/80">
            {tips.map((tip) => (
              <li key={tip}>{tip}</li>
            ))}
          </ul>
        </div>
      )}

      {guides.length > 0 && <p className="text-xs text-muted-foreground">外部摘要，仅供参考。</p>}
    </section>
  );
}

function Accommodation({ accommodation }: { accommodation: AccommodationBrief }) {
  const selected = accommodation.selected;
  const options = (accommodation.options ?? []).filter((hotel) => hotel.name || hotel.id).slice(0, 3);
  const anchors = accommodation.anchor_days?.map((day) => `第 ${day + 1} 天`).join("、");

  return (
    <div className="space-y-2">
      <SectionLabel>住宿</SectionLabel>
      {selected ? (
        <div className="space-y-1.5">
          <div className="flex flex-wrap items-center gap-2">
            <p className="font-serif text-base text-ink">{selected.name ?? "已选酒店"}</p>
            {selected.star ? <Badge variant="outline">{selected.star} 星</Badge> : null}
            {typeof selected.score === "number" ? (
              <span className="text-xs text-muted-foreground">{selected.score} 分</span>
            ) : null}
            {(selected.price_estimated || accommodation.price_estimated) && <Badge variant="warning">估算</Badge>}
          </div>
          {selected.address && <p className="text-sm text-muted-foreground">{selected.address}</p>}
          <PriceLine accommodation={accommodation} nightly={selected.price_per_night_yuan ?? accommodation.nightly_yuan ?? 0} />
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
            {selected.provider && <span>来源：{selected.provider}</span>}
            {anchors && <span>锚点：{anchors}</span>}
          </div>
          {safeURL(selected.booking_url) && (
            <a
              className="inline-block text-sm text-forest underline underline-offset-2"
              href={selected.booking_url}
              target="_blank"
              rel="noopener noreferrer"
            >
              查看预订
            </a>
          )}
        </div>
      ) : options.length > 0 ? (
        <ul className="space-y-1.5">
          {options.map((hotel, index) => (
            <li key={`${hotel.id ?? hotel.name}-${index}`} className="flex items-baseline justify-between gap-3 text-sm">
              <span className="min-w-0 truncate text-ink">{hotel.name ?? "候选酒店"}</span>
              <span className="shrink-0 font-mono text-xs tabular-nums text-muted-foreground">
                {(hotel.price_per_night_yuan ?? 0) > 0 ? `${formatYuan(hotel.price_per_night_yuan ?? 0)}/晚` : "价格未知"}
              </span>
            </li>
          ))}
        </ul>
      ) : (
        <p className="text-sm text-muted-foreground">本次行程未指定住宿。</p>
      )}
    </div>
  );
}

function PriceLine({ accommodation, nightly }: { accommodation: AccommodationBrief; nightly: number }) {
  const total = accommodation.total_yuan ?? 0;
  if (nightly <= 0 && total <= 0) {
    return <p className="text-sm text-muted-foreground">未获取价格，未计入预算。</p>;
  }
  const parts = [
    nightly > 0 ? `${formatYuan(nightly)}/晚` : "",
    accommodation.nights ? `${accommodation.nights} 晚` : "",
    total > 0 ? `合计 ${formatYuan(total)}` : "",
  ].filter(Boolean);
  return <p className="text-sm tabular-nums text-ink">{parts.join(" · ")}</p>;
}

function Guides({ guides, poiNames }: { guides: GuideSignal[]; poiNames: Record<string, string> }) {
  return (
    <div className="space-y-2">
      <SectionLabel>攻略提及</SectionLabel>
      <ul className="space-y-3">
        {guides.map((guide, index) => {
          const name = (guide.poi_id && poiNames[guide.poi_id]) || guide.poi_id || "行程点";
          const citations = (guide.citations ?? []).filter((citation) => citation.title || citation.url).slice(0, 3);
          return (
            <li key={`${guide.poi_id ?? "guide"}-${index}`} className="space-y-1 border-l-2 border-hairline pl-3">
              <p className="text-sm font-medium text-ink">{name}</p>
              {guide.note && <p className="text-sm leading-relaxed text-ink/80">{guide.note}</p>}
              {citations.length > 0 && (
                <ul className="space-y-0.5">
                  {citations.map((citation, citationIndex) => (
                    <Citation key={`${citation.url ?? citation.title}-${citationIndex}`} citation={citation} />
                  ))}
                </ul>
              )}
            </li>
          );
        })}
      </ul>
    </div>
  );
}

function Citation({ citation }: { citation: GuideCitation }) {
  const date = formatCitationDate(citation.published_at);
  const meta = [citation.site, date].filter(Boolean).join(" · ");
  return (
    <li className="text-xs text-muted-foreground">
      {safeURL(citation.url) ? (
        <a
          className="text-forest underline underline-offset-2"
          href={citation.url}
          target="_blank"
          rel="noopener noreferrer"
        >
          {citation.title || citation.site || "外部链接"}
        </a>
      ) : (
        <span>{citation.title || citation.site || "来源"}</span>
      )}
      {meta && <span> · {meta}</span>}
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

function formatCitationDate(value: string | null | undefined): string {
  if (!value) return "";
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return value;
  return `${parsed.getMonth() + 1} 月 ${parsed.getDate()} 日`;
}
