import { Popover as Radix } from 'radix-ui'
import type { ReactNode } from 'react'

interface PopoverProps {
  /** The control it hangs from. */
  trigger: ReactNode
  children: ReactNode
  align?: 'start' | 'center' | 'end'
  width?: number
}

/**
 * A popover: something to glance at, not something to be in.
 *
 * The distinction from a dialog is not size, it is commitment. A dialog is a
 * question you have to answer before the window will do anything else; a
 * popover is a thing you looked at and then stopped looking at. Escape or a
 * click anywhere dismisses it, nothing is blocked while it is up, and it takes
 * no decision with it when it goes.
 *
 * It wears the same glass as the pull-down menu, because everything that
 * floats out of a control is the same material, and it grows out of that
 * control.
 */
export function Popover({ trigger, children, align = 'start', width = 560 }: PopoverProps) {
  return (
    <Radix.Root>
      <Radix.Trigger asChild>{trigger}</Radix.Trigger>
      <Radix.Portal>
        <Radix.Content
          align={align}
          sideOffset={6}
          collisionPadding={12}
          style={{ width }}
          className="popover glass-pop glass-dense animate-pop-in z-50 max-h-[var(--radix-popover-content-available-height)] overflow-hidden rounded-[14px]"
        >
          {children}
        </Radix.Content>
      </Radix.Portal>
    </Radix.Root>
  )
}
