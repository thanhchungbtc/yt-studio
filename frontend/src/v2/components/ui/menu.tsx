import { DropdownMenu as Radix } from 'radix-ui'
import type { ReactNode } from 'react'

import { cn } from '@/kit/lib/cn'
import { keepFocusIfMoved } from '@/kit/lib/focus'
import { Kbd } from '@/kit/ui/Kbd'
import type { MenuEntry } from '@/kit/ui/Menu'

export type MenuItem = MenuEntry

const CONTENT =
  'glass-pop z-50 min-w-48 animate-pop-in overflow-hidden rounded-[12px] p-1 text-sm text-fg outline-none'
const ITEM =
  'relative flex h-7 cursor-default items-center gap-2 rounded-[8px] px-2 text-fg outline-none select-none data-[disabled]:opacity-40 data-[highlighted]:bg-row-highlight'

/** The kit's pull-down, controllable so a caller can mount it on first use. */
export function Menu({
  items,
  children,
  align = 'end',
  open,
  onOpenChange,
}: {
  items: MenuItem[]
  children: ReactNode
  align?: 'start' | 'end'
  open?: boolean
  onOpenChange?: (open: boolean) => void
}) {
  return (
    <Radix.Root open={open} onOpenChange={onOpenChange}>
      <Radix.Trigger asChild>{children}</Radix.Trigger>
      <Radix.Portal>
        <Radix.Content
          align={align}
          sideOffset={5}
          collisionPadding={8}
          className={CONTENT}
          onCloseAutoFocus={keepFocusIfMoved}
        >
          {items.map((e, i) => {
            if (e.type === 'separator')
              return <Radix.Separator key={i} className="mx-1 my-1 h-px bg-line" />
            if (e.type === 'label' || e.type === 'submenu') return null
            const Icon = e.icon
            return (
              <Radix.Item
                key={i}
                disabled={e.disabled}
                onSelect={e.onSelect}
                className={cn(
                  ITEM,
                  e.danger &&
                    'text-danger data-[highlighted]:bg-[color-mix(in_srgb,var(--danger)_12%,transparent)]',
                )}
              >
                {Icon ? (
                  <Icon className="size-3.5 shrink-0 opacity-80" />
                ) : (
                  <span className="size-3.5 shrink-0" />
                )}
                <span className="flex-1 truncate">{e.label}</span>
                {e.shortcut && <Kbd keys={e.shortcut} className="opacity-70" />}
              </Radix.Item>
            )
          })}
        </Radix.Content>
      </Radix.Portal>
    </Radix.Root>
  )
}
