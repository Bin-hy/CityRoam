import { useEffect, useRef } from "react";
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog";
import { Badge } from "@/components/ui/badge";
import { SectionLabel } from "@/design/primitives";
import { useMediaQuery } from "@/hooks/useMediaQuery";
import { formatYuan } from "@/design/tokens";
import type { ResultLodging, ResultTarget } from "./model";
import { LegDetail } from "./LegDetail";
import { StopDetail } from "./StopDetail";

export interface DetailCardProps {
  target: ResultTarget;
  onClose: () => void;
  onAdjustDay?: (dayIndex: number) => void;
  onViewOnMap?: (dayIndex: number) => void;
  onToggleLodging?: () => void;
  lodgingPending?: boolean;
  readOnly?: boolean;
}

export function DetailCard({
  target,
  onClose,
  onAdjustDay,
  onViewOnMap,
  onToggleLodging,
  lodgingPending = false,
  readOnly = false,
}: DetailCardProps) {
  const desktop = useMediaQuery("(min-width: 768px)");
  const open = target !== null;
  const returnFocusRef = useRef<HTMLElement | null>(null);

  useEffect(() => {
    if (open && document.activeElement instanceof HTMLElement) {
      returnFocusRef.current = document.activeElement;
    }
  }, [open]);

  return (
    <Dialog
      open={open}
      modal={!desktop}
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
    >
      <DialogContent
        overlay={!desktop}
        onInteractOutside={desktop ? (event) => event.preventDefault() : undefined}
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          returnFocusRef.current?.focus();
        }}
        className="max-h-[85dvh] overflow-y-auto max-sm:bottom-0 max-sm:top-auto max-sm:left-0 max-sm:w-full max-sm:max-w-none max-sm:translate-x-0 max-sm:translate-y-0 max-sm:rounded-b-none max-sm:rounded-t-xl"
      >
        <DialogTitle className="sr-only">行程详情</DialogTitle>
        {target?.kind === "stop" && (
          <StopDetail
            day={target.day}
            stop={target.stop}
            readOnly={readOnly}
            onAdjustDay={onAdjustDay}
          />
        )}
        {target?.kind === "leg" && (
          <LegDetail
            day={target.day}
            leg={target.leg}
            onViewOnMap={onViewOnMap}
            readOnly={readOnly}
          />
        )}
        {target?.kind === "hotel" && (
          <HotelDetail
            lodging={target.lodging}
            readOnly={readOnly}
            onToggleLodging={onToggleLodging}
            lodgingPending={lodgingPending}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}

function HotelDetail({
  lodging,
  readOnly,
  onToggleLodging,
  lodgingPending,
}: {
  lodging: ResultLodging;
  readOnly: boolean;
  onToggleLodging?: () => void;
  lodgingPending: boolean;
}) {
  const hotel = lodging.hotel;
  const anchors = lodging.anchorDays.map((day) => `第 ${day + 1} 天`).join("、");
  const booking = safeHref(hotel.bookingURL);

  return (
    <div className="space-y-4">
      <header className="space-y-1">
        <SectionLabel>住宿</SectionLabel>
        <h2 className="font-serif text-lg leading-snug text-ink">{hotel.name}</h2>
        <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
          {hotel.star ? <Badge variant="outline">{hotel.star} 星</Badge> : null}
          {typeof hotel.score === "number" ? <span>{hotel.score} 分</span> : null}
          {lodging.priceEstimated && <Badge variant="warning">价格估算</Badge>}
        </div>
      </header>

      {hotel.address && <p className="text-sm text-muted-foreground">{hotel.address}</p>}

      <dl className="grid grid-cols-2 gap-2 text-sm">
        <div className="rounded-md bg-sand/40 px-2.5 py-1.5">
          <dt className="text-[11px] text-muted-foreground">每晚</dt>
          <dd className="font-mono tabular-nums text-ink">{formatYuan(lodging.nightlyYuan)}</dd>
        </div>
        <div className="rounded-md bg-sand/40 px-2.5 py-1.5">
          <dt className="text-[11px] text-muted-foreground">合计</dt>
          <dd className="font-mono tabular-nums text-ink">{formatYuan(lodging.totalYuan)}</dd>
        </div>
        <div className="rounded-md bg-sand/40 px-2.5 py-1.5">
          <dt className="text-[11px] text-muted-foreground">晚数</dt>
          <dd className="font-mono tabular-nums text-ink">{lodging.nights} 晚</dd>
        </div>
        <div className="rounded-md bg-sand/40 px-2.5 py-1.5">
          <dt className="text-[11px] text-muted-foreground">锚点</dt>
          <dd className="truncate font-mono tabular-nums text-ink">{anchors || "—"}</dd>
        </div>
      </dl>

      {booking && (
        <a
          className="inline-block text-sm text-forest underline underline-offset-2"
          href={booking}
          target="_blank"
          rel="noopener noreferrer"
        >
          查看预订
        </a>
      )}

      {!readOnly && onToggleLodging && (
        <label className="flex min-h-9 items-center gap-2 text-sm text-ink">
          <input
            type="checkbox"
            className="h-4 w-4 accent-forest"
            checked={lodging.includeInBudget}
            disabled={lodgingPending}
            onChange={onToggleLodging}
          />
          计入预算
          {lodgingPending && <span className="text-xs text-muted-foreground">处理中…</span>}
        </label>
      )}
      <p className="text-xs text-muted-foreground">价格估算，库存不保证。</p>
    </div>
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