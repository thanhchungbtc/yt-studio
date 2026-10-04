import * as DropdownMenu from '@radix-ui/react-dropdown-menu'
import type { LucideIcon } from 'lucide-react'
import type { ReactNode } from 'react'

export interface MenuItem {
  label: string
  icon?: LucideIcon
  shortcut?: string
  /**
   * Destroys something, and says so in red — the only reason a menu row is
   * ever a colour other than the text colour.
   */
  danger?: boolean
  onSelect: () => void
}

/** The glass card both kinds of menu wear; it grows out of its control. */
export const MENU_CONTENT =
  'glass-pop animate-pop-in z-50 min-w-48 overflow-hidden rounded-[12px] p-1 text-[12.5px] outline-none'

export const MENU_ITEM =
  'menu-item relative flex h-7 cursor-default items-center gap-2 rounded-[8px] px-2 outline-none select-none'

/** One row's insides, shared by the pull-down and the contextual menu. */
export function MenuRow({ item }: { item: MenuItem }) {
  const Icon = item.icon
  return (
    <>
      {Icon ? (
        <Icon className="size-3.5 shrink-0 opacity-80" strokeWidth={1.9} />
      ) : (
        <span className="size-3.5 shrink-0" />
      )}
      <span className="flex-1 truncate whitespace-nowrap">{item.label}</span>
      {item.shortcut ? (
        <span className="menu-shortcut shrink-0 tabular-nums">{item.shortcut}</span>
      ) : null}
    </>
  )
}

interface MenuProps {
  items: MenuItem[]
  /** The control the menu hangs from. */
  children: ReactNode
  align?: 'start' | 'end'
  /** Controlled open state, for a menu mounted only once it is asked for. */
  open?: boolean
  onOpenChange?: (open: boolean) => void
}

/**
 * A pull-down menu.
 *
 * Radix for the behaviour that is tedious and easy to get subtly wrong — focus
 * return, escape, typeahead, click-outside — and the glass card for the look.
 */
export function Menu({ items, children, align = 'end', open, onOpenChange }: MenuProps) {
  return (
    <DropdownMenu.Root
      {...(open !== undefined ? { open } : {})}
      {...(onOpenChange ? { onOpenChange } : {})}
    >
      <DropdownMenu.Trigger asChild>{children}</DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content
          align={align}
          sideOffset={5}
          collisionPadding={8}
          className={MENU_CONTENT}
        >
          {items.map((item) => (
            <DropdownMenu.Item
              key={item.label}
              onSelect={item.onSelect}
              data-danger={item.danger ? '' : undefined}
              className={MENU_ITEM}
            >
              <MenuRow item={item} />
            </DropdownMenu.Item>
          ))}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}
