import * as Radix from '@radix-ui/react-dialog'
import { useEffect, useId, useRef, type ReactNode } from 'react'
import { create } from 'zustand'

import { cn } from '../../core/utils'

/**
 * A dialog, in the middle of the window.
 *
 * It sits slightly above centre — a little under half the free space above it,
 * a little over half below. Dead centre reads as *low*, because the eye takes
 * the optical centre of a rectangle to be above its geometric one, and every
 * macOS alert has been placed this way for thirty years.
 *
 * The buttons are bottom-trailing with the default action last, which is the
 * macOS order and the reverse of the web's. `Return` submits and `Escape`
 * cancels, both without anyone wiring them up: the form lives inside the dialog
 * and the default button is associated with it by id, across the two subtrees
 * they render into.
 */

/**
 * How many dialogs are up.
 *
 * The workbench's shortcuts read this and stand down while one is open.
 * Counting here rather than at each call site means every dialog gets that for
 * free — including the next one, written by someone who never reads this file.
 */
interface ModalState {
  count: number
}

const useModals = create<ModalState>(() => ({ count: 0 }))

/** Whether anything modal is up. Read by the keybindings, not by components. */
export function anyModalOpen(): boolean {
  return useModals.getState().count > 0
}

interface DialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  description?: string
  /**
   * Receives the id to hang the form off, so the footer's default button can
   * submit it from outside the form's own subtree.
   */
  children: (formId: string) => ReactNode
  footer: (formId: string) => ReactNode
  width?: number
  /**
   * Hand the body its own padding and its own scrolling.
   *
   * A dialog that is a form wants a comfortable gutter and one scroller. A
   * dialog that is a *layout* — a list beside a pane — wants neither: the
   * gutter would inset the list away from the edge it should be flush with,
   * and one scroller would move both columns together.
   */
  flush?: boolean
  /**
   * Pin the whole dialog to a height.
   *
   * Needed when the content changes size without the dialog being reopened — a
   * group list beside a pane is exactly that, where one group holds two
   * settings and another holds thirteen. A modal that resizes under the pointer
   * is what everyone means by a dialog that jumps.
   *
   * It is set here, on the dialog, rather than on the body. Sizing the body
   * looks like it should work and does not: `flex-1` is `flex-basis: 0%`, and
   * on the main axis a basis beats a height. Fixing the outside and letting the
   * body take what is left has no such trapdoor.
   */
  height?: number
}

export function Dialog({
  open,
  onOpenChange,
  title,
  description,
  children,
  footer,
  width = 480,
  flush = false,
  height,
}: DialogProps) {
  const formId = useId()
  const content = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    useModals.setState((s) => ({ count: s.count + 1 }))
    return () => useModals.setState((s) => ({ count: Math.max(0, s.count - 1) }))
  }, [open])

  return (
    <Radix.Root open={open} onOpenChange={onOpenChange}>
      <Radix.Portal>
        <Radix.Overlay className="scrim fixed inset-0 z-40" />
        <Radix.Content
          ref={content}
          // Radix opens focus on the first tabbable element, which is whatever
          // happens to come first in the DOM — here the channel pop-up, which
          // already has an answer. `data-autofocus` says where the caret should
          // actually land: the field the dialog is waiting on.
          onOpenAutoFocus={(event) => {
            const target = content.current?.querySelector<HTMLElement>('[data-autofocus]')
            if (!target) return
            event.preventDefault()
            target.focus()
          }}
          style={height === undefined ? { width } : { width, height }}
          // Capped against the window as well as against its own width: a
          // dialog larger than the window it opens in is the one broken layout
          // a fixed size can still produce.
          className="dialog surface-content fixed top-[46%] left-1/2 z-50 flex max-h-[86vh] max-w-[calc(100vw-40px)] -translate-x-1/2 -translate-y-1/2 flex-col overflow-hidden rounded-[12px]"
        >
          <div className={cn('shrink-0 px-6 pt-5', flush ? 'pb-3.5' : 'pb-4')}>
            <Radix.Title className="text-[15px] leading-tight font-semibold text-primary">
              {title}
            </Radix.Title>
            {description ? (
              <Radix.Description className="mt-1 text-[12px] leading-snug text-secondary">
                {description}
              </Radix.Description>
            ) : null}
          </div>

          <div className={flush ? 'flex min-h-0 flex-1' : 'min-h-0 flex-1 overflow-y-auto px-6'}>
            {children(formId)}
          </div>

          <div
            className={cn(
              'hairline-t flex shrink-0 items-center gap-2 px-6 py-3.5',
              flush ? '' : 'mt-4',
            )}
          >
            {footer(formId)}
          </div>
        </Radix.Content>
      </Radix.Portal>
    </Radix.Root>
  )
}
