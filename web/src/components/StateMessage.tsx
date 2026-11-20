import type { ReactNode } from "react";
import { AlertTriangle, Inbox } from "lucide-react";
import { Alert } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";

export function LoadingState({ label = "加载中…", rows = 3 }: { label?: string; rows?: number }) {
  return (
    <div className="space-y-3" role="status" aria-label={label}>
      {Array.from({ length: rows }, (_, index) => (
        <Skeleton
          key={index}
          className={cn("rounded-md bg-sand/60", index === 0 ? "h-16" : index === 1 ? "h-24" : "h-12")}
        />
      ))}
      <p className="text-xs tracking-wide text-muted-foreground">{label}</p>
    </div>
  );
}

export function EmptyState({
  title,
  description,
  action,
}: {
  title: string;
  description?: string;
  action?: ReactNode;
}) {
  return (
    <div className="flex flex-col items-center gap-2 rounded-md border border-dashed border-hairline bg-paper/50 px-6 py-10 text-center">
      <Inbox className="h-5 w-5 text-muted-foreground" />
      <p className="font-serif text-base text-ink">{title}</p>
      {description && <p className="max-w-md text-xs text-muted-foreground">{description}</p>}
      {action}
    </div>
  );
}

export function ErrorState({
  title = "加载失败",
  description,
  onRetry,
  action,
}: {
  title?: string;
  description?: string;
  onRetry?: () => void;
  action?: ReactNode;
}) {
  return (
    <Alert variant="destructive" className="space-y-3 rounded-md border-danger/30 bg-danger/5">
      <p className="flex items-center gap-2 font-medium">
        <AlertTriangle className="h-4 w-4" />
        {title}
      </p>
      {description && <p className="text-sm text-ink/80">{description}</p>}
      <div className="flex flex-wrap gap-2">
        {onRetry && (
          <Button size="sm" onClick={onRetry}>
            重试
          </Button>
        )}
        {action}
      </div>
    </Alert>
  );
}
