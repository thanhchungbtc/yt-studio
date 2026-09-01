import * as Radix from '@radix-ui/react-context-menu'
import type { ReactNode } from 'react'

import type { MenuItem } from './menu'

/**
 * A macOS contextual menu: the same card, rows and highlight the pull-down menu
 * uses, opened by the right button instead of by a control.
 *
 * It shares `MenuItem` with the pull-down rather than defining its own. The two
 * differ in what summons them and in nothing else — a list of labels and the
 * things they do — and a second shape for the same idea would be two places to
 * remember when a row needs an icon or a colour.
 *
 * Radix for the parts that are tedious and easy to get subtly wrong: pointer
 * capture, placing the card against a screen edge, typeahead, and closing on the
 * next click anywhere.
 */
export function ContextMenu({ items, children }: { items: MenuItem[]; children: ReactNode }) {
  return (
    <Radix.Root>
      <Radix.Trigger asChild>{children}</Radix.Trigger>
      <Radix.Portal>
        <Radix.Content className="surface-menu z-50 min-w-[176px] rounded-[7px] p-1 text-[13px]">
          {items.map((item) => (
            <Radix.Item
              key={item.label}
              onSelect={item.onSelect}
              data-danger={item.danger ? '' : undefined}
              className="menu-item flex cursor-default items-center gap-2 rounded-[4px] px-2 py-[3px] outline-none"
            >
              {item.icon ? <item.icon className="size-[15px] shrink-0" strokeWidth={1.75} /> : null}
              <span className="flex-1 whitespace-nowrap">{item.label}</span>
              {item.shortcut ? (
                <span className="menu-shortcut shrink-0 tabular-nums">{item.shortcut}</span>
              ) : null}
            </Radix.Item>
          ))}
        </Radix.Content>
      </Radix.Portal>
    </Radix.Root>
  )
}
