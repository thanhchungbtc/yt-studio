import * as Radix from '@radix-ui/react-tooltip'
import type { ReactNode } from 'react'

import { Kbd } from './kbd'

/** Mounted once, at the root of the window. */
export function TooltipProvider({ children }: { children: ReactNode }) {
  return (
    <Radix.Provider delayDuration={450} skipDelayDuration={200}>
      {children}
    </Radix.Provider>
  )
}

interface TooltipProps {
  content: ReactNode
  /** The keystroke that does the same thing, shown as key caps. */
  shortcut?: string
  side?: 'top' | 'bottom' | 'left' | 'right'
  disabled?: boolean
  children: ReactNode
}

/**
 * A small glass label that names a control, and the key that does the same.
 *
 * The keystroke is the point. A window that is keyboard-first has to teach its
 * keys somewhere, and the moment someone hovers a control is the moment they
 * are asking what it does.
 */
export function Tooltip({ content, shortcut, side = 'bottom', disabled, children }: TooltipProps) {
  if (disabled || !content) return <>{children}</>
  return (
    <Radix.Root>
      <Radix.Trigger asChild>{children}</Radix.Trigger>
      <Radix.Portal>
        <Radix.Content
          side={side}
          sideOffset={6}
          collisionPadding={8}
          className="glass-pop animate-fade-in z-[60] flex max-w-80 items-center gap-2 rounded-[8px] px-2 py-1 text-[11.5px] text-primary"
        >
          <span>{content}</span>
          {shortcut ? <Kbd keys={shortcut} /> : null}
        </Radix.Content>
      </Radix.Portal>
    </Radix.Root>
  )
}
