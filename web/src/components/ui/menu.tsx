import * as ContextMenuPrimitive from '@radix-ui/react-context-menu'
import type { ReactNode } from 'react'

import { cn } from '@/core/utils'

/**
 * A right-click menu on any row. Radix owns focus trapping, dismissal, roving
 * arrow-key navigation, typeahead and the aria roles; only the trigger surface
 * and the styling are here.
 */
export function ContextMenu({ items, children }: { items: ReactNode; children: ReactNode }) {
  return (
    <ContextMenuPrimitive.Root>
      {/* asChild keeps the row itself as the trigger, so right-clicking
          anywhere on it opens the menu without adding a wrapper element that
          would disturb the layout. */}
      <ContextMenuPrimitive.Trigger asChild>{children}</ContextMenuPrimitive.Trigger>
      <ContextMenuPrimitive.Portal>
        <ContextMenuPrimitive.Content
          className="animate-in-fade z-50 min-w-[176px] rounded-[var(--radius-sm)] border border-[hsl(var(--border-strong))] bg-[hsl(var(--bg-elevated))] p-1 elev-3"
          collisionPadding={8}
        >
          {items}
        </ContextMenuPrimitive.Content>
      </ContextMenuPrimitive.Portal>
    </ContextMenuPrimitive.Root>
  )
}

/** One row of a context menu. `tone` marks the destructive ones. */
export function ContextMenuItem({
  onSelect,
  tone = 'default',
  children,
}: {
  onSelect: () => void
  tone?: 'default' | 'danger'
  children: ReactNode
}) {
  return (
    <ContextMenuPrimitive.Item
      onSelect={onSelect}
      className={cn(
        'flex cursor-default select-none items-center gap-2 rounded-[var(--radius-xs)] px-2 py-1 text-[12px] outline-none',
        tone === 'danger'
          ? 'text-[hsl(var(--danger))] data-[highlighted]:bg-[hsl(var(--danger)/0.14)]'
          : 'text-fg data-[highlighted]:bg-[hsl(var(--bg-hover))]',
      )}
    >
      {children}
    </ContextMenuPrimitive.Item>
  )
}

/** A hairline between groups of items. */
export function ContextMenuSeparator() {
  return <ContextMenuPrimitive.Separator className="my-1 h-px bg-[hsl(var(--border))]" />
}

/** A non-interactive caption above a group, naming what the menu acts on. */
export function ContextMenuLabel({ children }: { children: ReactNode }) {
  return (
    <ContextMenuPrimitive.Label className="truncate px-2 py-1 text-[10.5px] uppercase tracking-wider text-subtle">
      {children}
    </ContextMenuPrimitive.Label>
  )
}
