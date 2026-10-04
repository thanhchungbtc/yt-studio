import * as Radix from '@radix-ui/react-context-menu'
import { Slot } from '@radix-ui/react-slot'
import { useEffect, useRef, useState, type MouseEvent, type ReactNode } from 'react'

import { MENU_CONTENT, MENU_ITEM, MenuRow, type MenuItem } from './menu'

/**
 * A contextual menu: the same card, rows and highlight the pull-down menu
 * uses, opened by the right button instead of by a control.
 *
 * It shares `MenuItem` with the pull-down rather than defining its own. The two
 * differ in what summons them and in nothing else.
 *
 * Mounted on first use. A source list carries one of these per row, and a Radix
 * menu per row was a few thousand components on screen for a gesture made on
 * one of them. Until the first right-click the child stands alone; that click
 * mounts the menu and is handed to it again, at the same point, so it opens
 * where it always did. After that it stays mounted.
 */
export function ContextMenu({ items, children }: { items: MenuItem[]; children: ReactNode }) {
  const trigger = useRef<HTMLElement>(null)
  const [mounted, setMounted] = useState(false)
  const [replay, setReplay] = useState<{ x: number; y: number } | null>(null)

  useEffect(() => {
    if (!replay) return
    setReplay(null)
    trigger.current?.dispatchEvent(
      new window.MouseEvent('contextmenu', {
        bubbles: true,
        cancelable: true,
        button: 2,
        clientX: replay.x,
        clientY: replay.y,
      }),
    )
  }, [replay])

  if (!mounted) {
    return (
      <Slot
        ref={trigger}
        onContextMenu={(event: MouseEvent) => {
          event.preventDefault()
          setMounted(true)
          setReplay({ x: event.clientX, y: event.clientY })
        }}
      >
        {children}
      </Slot>
    )
  }

  return (
    <Radix.Root>
      <Radix.Trigger ref={trigger} asChild>
        {children}
      </Radix.Trigger>
      <Radix.Portal>
        <Radix.Content collisionPadding={8} className={MENU_CONTENT}>
          {items.map((item) => (
            <Radix.Item
              key={item.label}
              onSelect={item.onSelect}
              data-danger={item.danger ? '' : undefined}
              className={MENU_ITEM}
            >
              <MenuRow item={item} />
            </Radix.Item>
          ))}
        </Radix.Content>
      </Radix.Portal>
    </Radix.Root>
  )
}
