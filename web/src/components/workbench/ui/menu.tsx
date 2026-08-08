import * as ContextMenuPrimitive from '@radix-ui/react-context-menu'
import * as DropdownMenuPrimitive from '@radix-ui/react-dropdown-menu'
import type { ReactNode } from 'react'

import { cn } from '@/core/utils'

/**
 * Menus. Radix owns focus trapping, dismissal, roving arrow-key navigation,
 * typeahead and the aria roles; only the trigger surface and the styling are
 * here.
 *
 * Two flavours, because the workbench uses both: right-click on a tab or a tree
 * row, and the click-to-open dropdowns in the breadcrumb.
 */

const CONTENT =
  'animate-in-fade z-50 min-w-[180px] rounded-[var(--radius-sm)] border border-[hsl(var(--border-strong))] bg-[hsl(var(--bg-elevated))] p-1 elev-3'

const ITEM =
  'flex cursor-default select-none items-center gap-2 rounded-[var(--radius-xs)] px-2 py-1 text-[12px] outline-none'

/* ------------------------------------------------------------ right-click */

export function ContextMenu({ items, children }: { items: ReactNode; children: ReactNode }) {
  return (
    <ContextMenuPrimitive.Root>
      {/* asChild keeps the row itself as the trigger, so right-clicking anywhere
          on it opens the menu without a wrapper that would disturb layout. */}
      <ContextMenuPrimitive.Trigger asChild>{children}</ContextMenuPrimitive.Trigger>
      <ContextMenuPrimitive.Portal>
        <ContextMenuPrimitive.Content className={CONTENT} collisionPadding={8}>
          {items}
        </ContextMenuPrimitive.Content>
      </ContextMenuPrimitive.Portal>
    </ContextMenuPrimitive.Root>
  )
}

export function MenuItem({
  onSelect,
  tone = 'default',
  disabled,
  shortcut,
  children,
}: {
  onSelect: () => void
  tone?: 'default' | 'danger'
  disabled?: boolean
  shortcut?: ReactNode
  children: ReactNode
}) {
  return (
    <ContextMenuPrimitive.Item
      onSelect={onSelect}
      disabled={disabled}
      className={cn(
        ITEM,
        'data-[disabled]:pointer-events-none data-[disabled]:opacity-40',
        tone === 'danger'
          ? 'text-[hsl(var(--danger))] data-[highlighted]:bg-[hsl(var(--danger)/0.14)]'
          : 'text-fg data-[highlighted]:bg-[hsl(var(--bg-hover))]',
      )}
    >
      {children}
      {shortcut && <span className="ml-auto pl-4 text-subtle">{shortcut}</span>}
    </ContextMenuPrimitive.Item>
  )
}

export function MenuSeparator() {
  return <ContextMenuPrimitive.Separator className="my-1 h-px bg-[hsl(var(--border))]" />
}

/** A non-interactive caption above a group, naming what the menu acts on. */
export function MenuLabel({ children }: { children: ReactNode }) {
  return (
    <ContextMenuPrimitive.Label className="truncate px-2 py-1 text-[10.5px] uppercase tracking-wider text-subtle">
      {children}
    </ContextMenuPrimitive.Label>
  )
}

/* --------------------------------------------------------------- dropdown */

export function Dropdown({
  trigger,
  items,
  align = 'start',
}: {
  trigger: ReactNode
  items: ReactNode
  align?: 'start' | 'center' | 'end'
}) {
  return (
    <DropdownMenuPrimitive.Root>
      <DropdownMenuPrimitive.Trigger asChild>{trigger}</DropdownMenuPrimitive.Trigger>
      <DropdownMenuPrimitive.Portal>
        <DropdownMenuPrimitive.Content
          align={align}
          sideOffset={4}
          collisionPadding={8}
          className={CONTENT}
        >
          {items}
        </DropdownMenuPrimitive.Content>
      </DropdownMenuPrimitive.Portal>
    </DropdownMenuPrimitive.Root>
  )
}

export function DropdownItem({
  onSelect,
  selected,
  children,
}: {
  onSelect: () => void
  selected?: boolean
  children: ReactNode
}) {
  return (
    <DropdownMenuPrimitive.Item
      onSelect={onSelect}
      className={cn(
        ITEM,
        selected
          ? 'bg-[hsl(var(--bg-active))] font-medium text-fg'
          : 'text-fg data-[highlighted]:bg-[hsl(var(--bg-hover))]',
      )}
    >
      {children}
    </DropdownMenuPrimitive.Item>
  )
}
