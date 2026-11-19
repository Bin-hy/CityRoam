import * as React from "react";
import { cn } from "@/lib/utils";

export function Alert({ className, variant = "default", ...props }: React.HTMLAttributes<HTMLDivElement> & { variant?: "default" | "destructive" | "warning" }) {
  return (
    <div
      role="alert"
      className={cn(
        "rounded-lg border px-4 py-3 text-sm",
        variant === "default" && "bg-secondary text-secondary-foreground",
        variant === "warning" && "border-amber-200 bg-amber-50 text-amber-900",
        variant === "destructive" && "border-destructive/30 bg-destructive/5 text-destructive",
        className,
      )}
      {...props}
    />
  );
}
