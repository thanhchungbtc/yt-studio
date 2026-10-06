import type { ReactNode } from "react";
import { Tooltip as RTooltip } from "radix-ui";
import { KeybindingHint } from "./Kbd";

export function TooltipProvider({ children }: { children: ReactNode }) {
  return (
    <RTooltip.Provider delayDuration={450} skipDelayDuration={200}>
      {children}
    </RTooltip.Provider>
  );
}

export interface TooltipProps {
  content: ReactNode;
  /** Shows this command's keybinding next to the label. */
  command?: string;
  side?: "top" | "bottom" | "left" | "right";
  children: ReactNode;
  disabled?: boolean;
}

export function Tooltip({ content, command, side = "bottom", children, disabled }: TooltipProps) {
  if (disabled || (!content && !command)) return <>{children}</>;
  return (
    <RTooltip.Root>
      <RTooltip.Trigger asChild>{children}</RTooltip.Trigger>
      <RTooltip.Portal>
        <RTooltip.Content
          side={side}
          sideOffset={6}
          collisionPadding={8}
          className="glass-pop z-50 flex max-w-80 animate-fade-in items-center gap-2 rounded-[8px] px-2 py-1 text-xs text-fg"
        >
          <span>{content}</span>
          {command && <KeybindingHint command={command} />}
        </RTooltip.Content>
      </RTooltip.Portal>
    </RTooltip.Root>
  );
}
