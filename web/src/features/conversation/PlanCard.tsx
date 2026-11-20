import { Button } from "@/components/ui/button";
import { Stat, Surface } from "@/design/primitives";
import { formatYuan } from "@/design/tokens";
import type { PlanMessage } from "./deriveConversation";

export function PlanCard({ message, onOpenCanvas }: { message: PlanMessage; onOpenCanvas?: () => void }) {
  return (
    <Surface className="space-y-3 border-hairline bg-card/80 p-4">
      <div className="flex items-center justify-between gap-3">
        <p className="font-serif text-base text-ink">行程方案</p>
        <span className="rounded-full border border-hairline px-2 py-0.5 text-[11px] text-muted-foreground">
          {message.isDraft ? "草稿预览 · 待确认" : message.versionSeq ? `版本 v${message.versionSeq}` : "正式版本"}
        </span>
      </div>
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <Stat value={`${message.stats.days} 天`} label="天数" />
        <Stat value={`${message.stats.pois} 个`} label="景点" />
        <Stat value={`${message.stats.walkingKm.toFixed(1)} km`} label="步行" />
        <Stat value={formatYuan(message.stats.totalYuan)} label="总价" />
      </div>
      {onOpenCanvas && (
        <Button size="sm" variant="outline" className="w-full md:hidden" onClick={onOpenCanvas}>
          查看方案
        </Button>
      )}
    </Surface>
  );
}
