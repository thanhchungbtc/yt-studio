import type { ReactNode } from "react";
import { Dialog as RDialog } from "radix-ui";
import { cn } from "@/kit/lib/cn";
import { keepFocusIfMoved } from "@/kit/lib/focus";

export interface DialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title?: ReactNode;
  description?: ReactNode;
  children: ReactNode;
  className?: string;
  /** Visually hidden title for accessibility when no visible title. */
  label?: string;
}

export function Dialog({ open, onOpenChange, title, description, children, className, label }: DialogProps) {
  return (
    <RDialog.Root open={open} onOpenChange={onOpenChange}>
      <RDialog.Portal>
        <RDialog.Overlay className="fixed inset-0 z-50 animate-fade-in bg-black/20 dark:bg-black/35" />
        <RDialog.Content
          className={cn(
            "glass-pop glass-dense fixed top-[14vh] left-1/2 z-50 w-[min(560px,calc(100vw-48px))] -translate-x-1/2 animate-pop-in rounded-[20px] text-fg outline-none",
            className,
          )}
          aria-describedby={description ? undefined : undefined}
          onCloseAutoFocus={keepFocusIfMoved}
        >
          {title ? (
            <div className="px-5 pt-4 pb-2">
              <RDialog.Title className="text-md font-semibold">{title}</RDialog.Title>
              {description && <RDialog.Description className="mt-1 text-sm text-fg-muted">{description}</RDialog.Description>}
            </div>
          ) : (
            <RDialog.Title className="sr-only">{label ?? "Dialog"}</RDialog.Title>
          )}
          {children}
        </RDialog.Content>
      </RDialog.Portal>
    </RDialog.Root>
  );
}
