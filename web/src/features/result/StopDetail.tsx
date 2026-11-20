import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { SectionLabel } from "@/design/primitives";
import { formatYuan } from "@/design/tokens";
import type { ResultCitation, ResultDay, ResultGuide, ResultStop } from "./model";

export interface StopDetailProps {
  day: ResultDay;
  stop: ResultStop;
  readOnly?: boolean;
  onAdjustDay?: (dayIndex: number) => void;
}

export function StopFacts({ stop }: { stop: ResultStop }) {
  return (
    <dl className="grid grid-cols-2 gap-2 text-sm">
      <div className="rounded-md bg-sand/40 px-2.5 py-1.5">
        <dt className="text-[11px] text-muted-foreground">时长</dt>
        <dd className="font-mono tabular-nums text-ink">{stop.durationMin} 分钟</dd>
      </div>
      <div className="rounded-md bg-sand/40 px-2.5 py-1.5">
        <dt className="text-[11px] text-muted-foreground">花费</dt>
        <dd className="font-mono tabular-nums text-ink">{formatYuan(stop.costYuan)}</dd>
      </div>
    </dl>
  );
}

export function ReasonSection({ reason }: { reason: string | null }) {
  return (
    <div className="space-y-1">
      <SectionLabel>为什么选它</SectionLabel>
      {reason ? (
        <p className="text-sm leading-relaxed text-ink">{reason}</p>
      ) : (
        <p className="text-sm text-muted-foreground">本次未生成额外说明。</p>
      )}
    </div>
  );
}

export function GuideSection({ guides }: { guides: ResultGuide[] }) {
  return (
    <div className="space-y-1.5">
      <SectionLabel>攻略提及</SectionLabel>
      {guides.length > 0 ? (
        guides.map((guide, index) => (
          <div
            key={`${guide.poiID}-${index}`}
            className="space-y-1 rounded-md border border-hairline bg-paper/60 px-2.5 py-2"
          >
            {guide.note && <p className="text-sm leading-relaxed text-ink">{guide.note}</p>}
            {guide.citations.length > 0 && (
              <ul className="space-y-0.5">
                {guide.citations.map((citation, citationIndex) => (
                  <Citation key={`${citation.url ?? citation.title ?? citation.site}-${citationIndex}`} citation={citation} />
                ))}
              </ul>
            )}
          </div>
        ))
      ) : (
        <p className="text-sm text-muted-foreground">暂无攻略提及。</p>
      )}
    </div>
  );
}

export function StopDetail({ day, stop, readOnly = false, onAdjustDay }: StopDetailProps) {
  return (
    <div className="space-y-4">
      <header className="space-y-1">
        <SectionLabel>
          第 {day.index + 1} 天 · {stop.start}–{stop.end}
        </SectionLabel>
        <h2 className="font-serif text-lg leading-snug text-ink">{stop.title}</h2>
        <Badge variant="outline">{stop.typeLabel}</Badge>
      </header>

      <StopFacts stop={stop} />

      {stop.note && <p className="text-sm text-muted-foreground">{stop.note}</p>}

      <ReasonSection reason={stop.reason} />
      <GuideSection guides={stop.guides} />

      {!readOnly && onAdjustDay && (
        <div className="border-t border-hairline pt-3">
          <Button size="sm" variant="outline" onClick={() => onAdjustDay(day.index)}>
            调整这一天
          </Button>
        </div>
      )}
    </div>
  );
}

function Citation({ citation }: { citation: ResultCitation }) {
  const date = formatCitationDate(citation.publishedAt);
  const meta = [citation.site, date].filter(Boolean).join(" · ");
  const url = safeHref(citation.url);

  return (
    <li className="text-xs text-muted-foreground">
      {url ? (
        <a
          className="text-forest underline underline-offset-2"
          href={url}
          target="_blank"
          rel="noopener noreferrer"
        >
          {citation.title || citation.site || url}
        </a>
      ) : (
        <span>{citation.title || citation.site || "来源"}</span>
      )}
      {meta && <span> · {meta}</span>}
    </li>
  );
}

function safeHref(value: string | undefined): string | undefined {
  if (!value) return undefined;
  try {
    const url = new URL(value);
    return url.protocol === "http:" || url.protocol === "https:" ? value : undefined;
  } catch {
    return undefined;
  }
}

function formatCitationDate(value: string | null | undefined): string {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleDateString("zh-CN", { year: "numeric", month: "numeric", day: "numeric" });
}