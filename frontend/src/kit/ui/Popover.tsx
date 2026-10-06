import type { ReactNode } from "react";
import { Popover as RPopover } from "radix-ui";
import { cn } from "@/kit/lib/cn";
import { keepFocusIfMoved } from "@/kit/lib/focus";

export interface PopoverProps {
  trigger: ReactNode;
  children: ReactNode;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  side?: "top" | "bottom" | "left" | "right";
  align?: "start" | "center" | "end";
  className?: string;
  /** Keep focus in the trigger area (for lightweight info popovers). */
  modal?: boolean;
}

export function Popover({ trigger, children, open, onOpenChange, side = "bottom", align = "start", className, modal }: PopoverProps) {
  return (
    <RPopover.Root open={open} onOpenChange={onOpenChange} modal={modal}>
      <RPopover.Trigger asChild>{trigger}</RPopover.Trigger>
      <RPopover.Portal>
        <RPopover.Content
          side={side}
          align={align}
          sideOffset={6}
          collisionPadding={12}
          onCloseAutoFocus={keepFocusIfMoved}
          className={cn(
            "glass-pop z-40 max-h-[var(--radix-popover-content-available-height)] animate-pop-in overflow-hidden rounded-xl text-fg outline-none",
            className,
          )}
        >
          {children}
        </RPopover.Content>
      </RPopover.Portal>
    </RPopover.Root>
  );
}
