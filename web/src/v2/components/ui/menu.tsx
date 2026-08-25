import * as DropdownMenu from '@radix-ui/react-dropdown-menu'
import type { LucideIcon } from 'lucide-react'
import type { ReactNode } from 'react'

export interface MenuItem {
  label: string
  icon?: LucideIcon
  shortcut?: string
  onSelect: () => void
}

interface MenuProps {
  items: MenuItem[]
  /** The control the menu hangs from. */
  children: ReactNode
  align?: 'start' | 'end'
}

/**
 * A macOS pull-down menu.
 *
 * Radix for the behaviour that is tedious and easy to get subtly wrong — focus
 * return, escape, typeahead, click-outside — and a few lines of CSS for the
 * look: a translucent card, tight rows, and the accent as a full-width
 * highlight rather than a tint.
 */
export function Menu({ items, children, align = 'end' }: MenuProps) {
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>{children}</DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content
          align={align}
          sideOffset={4}
          className="surface-menu z-50 min-w-[176px] rounded-[7px] p-1 text-[13px]"
        >
          {items.map((item) => (
            <DropdownMenu.Item
              key={item.label}
              onSelect={item.onSelect}
              className="menu-item flex cursor-default items-center gap-2 rounded-[4px] px-2 py-[3px] outline-none"
            >
              {item.icon ? <item.icon className="size-[15px] shrink-0" strokeWidth={1.75} /> : null}
              <span className="flex-1 whitespace-nowrap">{item.label}</span>
              {item.shortcut ? (
                <span className="menu-shortcut shrink-0 tabular-nums">{item.shortcut}</span>
              ) : null}
            </DropdownMenu.Item>
          ))}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}
