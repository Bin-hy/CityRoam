import * as React from "react";
import { cn } from "@/lib/utils";

interface PopoverContextValue {
  open: boolean;
  setOpen: (open: boolean) => void;
  contentID: string;
  rootRef: React.RefObject<HTMLSpanElement | null>;
}

const PopoverContext = React.createContext<PopoverContextValue | null>(null);

function usePopoverContext(): PopoverContextValue {
  const context = React.useContext(PopoverContext);
  if (!context) throw new Error("Popover components must be used inside <Popover>");
  return context;
}

export interface PopoverProps {
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  children: React.ReactNode;
}

export function Popover({ open: controlledOpen, onOpenChange, children }: PopoverProps) {
  const [uncontrolledOpen, setUncontrolledOpen] = React.useState(false);
  const open = controlledOpen ?? uncontrolledOpen;
  const rootRef = React.useRef<HTMLSpanElement>(null);
  const contentID = React.useId();

  const setOpen = React.useCallback(
    (next: boolean) => {
      if (controlledOpen === undefined) setUncontrolledOpen(next);
      onOpenChange?.(next);
    },
    [controlledOpen, onOpenChange],
  );

  React.useEffect(() => {
    if (!open) return;
    const onPointerDown = (event: PointerEvent) => {
      if (rootRef.current?.contains(event.target as Node)) return;
      setOpen(false);
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      event.stopPropagation();
      setOpen(false);
      rootRef.current?.querySelector<HTMLElement>("button, [role=button]")?.focus();
    };
    document.addEventListener("pointerdown", onPointerDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("pointerdown", onPointerDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [open, setOpen]);

  return (
    <PopoverContext.Provider value={{ open, setOpen, contentID, rootRef }}>
      <span ref={rootRef} className="relative inline-block">
        {children}
      </span>
    </PopoverContext.Provider>
  );
}

export interface PopoverTriggerProps {
  asChild?: boolean;
  children: React.ReactElement<Record<string, unknown>>;
}

export function PopoverTrigger({ asChild, children }: PopoverTriggerProps) {
  const context = usePopoverContext();
  if (!asChild) {
    return (
      <button
        type="button"
        aria-haspopup="dialog"
        aria-expanded={context.open}
        aria-controls={context.open ? context.contentID : undefined}
        onClick={() => context.setOpen(!context.open)}
      >
        {children}
      </button>
    );
  }
  return React.cloneElement(children, {
    "aria-haspopup": "dialog",
    "aria-expanded": context.open,
    "aria-controls": context.open ? context.contentID : undefined,
    onClick: (event: React.MouseEvent) => {
      const previous = children.props.onClick as ((event: React.MouseEvent) => void) | undefined;
      previous?.(event);
      context.setOpen(!context.open);
    },
  });
}

export type PopoverContentProps = React.HTMLAttributes<HTMLDivElement>;

export const PopoverContent = React.forwardRef<HTMLDivElement, PopoverContentProps>(
  ({ className, children, ...props }, forwardedRef) => {
    const context = usePopoverContext();
    const localRef = React.useRef<HTMLDivElement>(null);

    React.useEffect(() => {
      if (context.open) localRef.current?.focus();
    }, [context.open]);

    if (!context.open) return null;

    return (
      <div
        ref={(node) => {
          localRef.current = node;
          if (typeof forwardedRef === "function") forwardedRef(node);
          else if (forwardedRef) forwardedRef.current = node;
        }}
        id={context.contentID}
        role="dialog"
        tabIndex={-1}
        className={cn(
          "absolute right-0 top-full z-50 mt-2 w-72 rounded-md border border-hairline bg-paper p-4 text-ink shadow-lg outline-none focus-visible:ring-2 focus-visible:ring-ring",
          className,
        )}
        {...props}
      >
        {children}
      </div>
    );
  },
);
PopoverContent.displayName = "PopoverContent";