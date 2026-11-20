import type { HTMLAttributes, ReactNode } from "react";
import { cn } from "@/lib/utils";

export function Surface({ className, ...props }: HTMLAttributes<HTMLDivElement>) {
  return <div className={cn("rounded-md border border-hairline bg-card", className)} {...props} />;
}

export function Hairline({ className }: { className?: string }) {
  return <div role="separator" className={cn("h-px w-full bg-hairline", className)} />;
}

export function SectionLabel({ className, children }: { className?: string; children: ReactNode }) {
  return (
    <p className={cn("text-[11px] font-medium uppercase tracking-[0.16em] text-muted-foreground", className)}>
      {children}
    </p>
  );
}

export function Chip({
  active = false,
  onClick,
  className,
  children,
  title,
}: {
  active?: boolean;
  onClick?: () => void;
  className?: string;
  children: ReactNode;
  title?: string;
}) {
  const styles = cn(
    "inline-flex items-center gap-1.5 rounded-full border px-3 text-sm transition-colors",
    active ? "border-forest bg-forest/10 text-forest" : "border-hairline bg-paper/70 text-ink/80",
    onClick ? "min-h-11 hover:border-forest/50 hover:text-forest" : "min-h-7",
    className,
  );
  if (!onClick) {
    return (
      <span className={styles} title={title}>
        {children}
      </span>
    );
  }
  return (
    <button type="button" className={styles} onClick={onClick} aria-pressed={active} title={title}>
      {children}
    </button>
  );
}

export function Stat({
  value,
  label,
  className,
}: {
  value: ReactNode;
  label: ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("min-w-0", className)}>
      <div className="truncate font-mono text-lg tabular-nums text-ink">{value}</div>
      <div className="mt-0.5 truncate text-[11px] text-muted-foreground">{label}</div>
    </div>
  );
}

const dotTones = {
  neutral: "bg-muted-foreground",
  success: "bg-success",
  warning: "bg-warning",
  danger: "bg-danger",
  accent: "bg-forest",
} as const;

export function StatusDot({
  tone = "neutral",
  label,
  className,
}: {
  tone?: keyof typeof dotTones;
  label: ReactNode;
  className?: string;
}) {
  return (
    <span className={cn("inline-flex items-center gap-1.5 text-xs text-muted-foreground", className)}>
      <span aria-hidden className={cn("h-1.5 w-1.5 rounded-full", dotTones[tone])} />
      {label}
    </span>
  );
}

export function EmptyHint({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <p className={cn("rounded-md border border-dashed border-hairline bg-paper/50 px-4 py-6 text-sm text-muted-foreground", className)}>
      {children}
    </p>
  );
}
