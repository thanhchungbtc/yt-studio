import * as Radix from '@radix-ui/react-dialog'
import { X } from 'lucide-react'
import { useEffect, useRef, type ReactNode } from 'react'
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
 * Assembled rather than configured: the shell takes a size and everything else
 * is a part you either include or leave out.
 *
 *   <Dialog open={open} onOpenChange={…}>
 *     <Dialog.Header title="New video" description="…" />
 *     <Dialog.Body>…</Dialog.Body>
 *     <Dialog.Footer>…</Dialog.Footer>
 *     <Dialog.Close />        // only where the footer is not the way out
 *   </Dialog>
 *
 * This used to be four props — a `footer` render prop, a `flush` boolean, and
 * `children` as a function — and the boolean was the tell. `flush` reached into
 * three separate elements to change the padding of each, which is one flag
 * standing in for three decisions that were never the same decision. A part you
 * omit says the same thing without anyone having to know what it does.
 *
 * The shell knows nothing about forms. A dialog whose footer submits a body is
 * one arrangement of two, and the id that ties them together belongs to the
 * screen that has both — `useId` at the call site, into `<form id>` and
 * `<Button form>`, and nothing in here has to carry it.
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
  /**
   * A CSS length as well as a pixel count, so a dialog that wants most of the
   * window can ask for most of the window rather than naming a big number and
   * leaning on the cap below to bring it back.
   */
  width?: number | string
  /**
   * Pin the whole dialog to a height.
   *
   * Needed when the content changes size without the dialog being reopened — a
   * group list beside a pane is exactly that, where one group holds two
   * settings and another holds thirteen. A modal that resizes under the pointer
   * is what everyone means by a dialog that jumps.
   *
   * It is set here, on the shell, rather than on the body. Sizing the body
   * looks like it should work and does not: `flex-1` is `flex-basis: 0%`, and
   * on the main axis a basis beats a height. Fixing the outside and letting the
   * body take what is left has no such trapdoor.
   */
  height?: number | string
  children: ReactNode
}

export function Dialog({ open, onOpenChange, width = 480, height, children }: DialogProps) {
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
          // happens to come first in the DOM — in the new-video dialog that is
          // the channel pop-up, which already has an answer. `data-autofocus`
          // says where the caret should actually land: the field being waited on.
          onOpenAutoFocus={(event) => {
            const target = content.current?.querySelector<HTMLElement>('[data-autofocus]')
            if (!target) return
            event.preventDefault()
            target.focus()
          }}
          // `undefined` is not written, so an unset height leaves the dialog to
          // its content the way an unset one always did.
          style={{ width, height }}
          // Capped against the window as well as against its own width: a
          // dialog larger than the window it opens in is the one broken layout
          // a fixed size can still produce.
          className="dialog surface-content fixed top-[46%] left-1/2 z-50 flex max-h-[86vh] max-w-[calc(100vw-40px)] -translate-x-1/2 -translate-y-1/2 flex-col overflow-hidden rounded-[14px]"
        >
          {children}
        </Radix.Content>
      </Radix.Portal>
    </Radix.Root>
  )
}

/**
 * What the dialog is, and — when a sentence is needed — what it is about.
 *
 * `Radix.Title` rather than a styled span: it is what the dialog announces
 * itself as, so leaving it out would make the whole thing anonymous to anything
 * not reading pixels.
 */
function Header({ title, description }: { title: string; description?: string }) {
  return (
    // The trailing gutter is wider than the leading one whether or not there is
    // a close button in it. Reserving it unconditionally costs nothing — the
    // text is left-aligned and short — and it means the button can be a part the
    // caller adds without the header having to be told it is coming.
    <div className="shrink-0 pt-5 pr-14 pb-4 pl-6">
      <Radix.Title className="text-[15px] leading-tight font-semibold text-primary">
        {title}
      </Radix.Title>
      {description ? (
        <Radix.Description className="mt-1 text-[12px] leading-snug text-secondary">
          {description}
        </Radix.Description>
      ) : null}
    </div>
  )
}

/**
 * The part that takes the leftover height.
 *
 * Padded and scrolling by default, which is what a dialog that asks a question
 * wants. `bare` hands both back: a body that is a *layout* — a list beside a
 * pane, a picture centred in the space — wants no gutter insetting it from the
 * edge it should meet, and no single scroller dragging both halves at once.
 *
 * The bottom padding lives here rather than as a margin on the footer, because
 * it is this element's content that needs the room. It also scrolls with that
 * content, so the last row clears the hairline instead of resting on it.
 */
function Body({ bare = false, children }: { bare?: boolean; children: ReactNode }) {
  return (
    <div className={cn('min-h-0 flex-1', bare ? 'flex' : 'overflow-y-auto px-6 pb-5')}>
      {children}
    </div>
  )
}

/**
 * The bar along the bottom, and the reason it is a part rather than a flag.
 *
 * Bottom-trailing with the default action last, which is the macOS order and
 * the reverse of the web's. A dialog that is only *showing* something has no
 * question to answer and simply leaves this out — where an empty one would be a
 * bar under a hairline announcing that it is empty.
 */
function Footer({ children }: { children: ReactNode }) {
  return <div className="hairline-t flex shrink-0 items-center gap-2 px-6 py-3.5">{children}</div>
}

/**
 * The way out, for a dialog that does not already have one.
 *
 * A dialog that asks a question has Cancel or Done, and a close box beside them
 * would be a second answer to the same question — which is why this is a part
 * rather than something the shell draws for everyone. A dialog that only *shows*
 * something has no such button, and at most of the window wide there is barely
 * any outside left to click: without this, Escape is the only way out, and
 * Escape is not an affordance.
 *
 * `Radix.Close` rather than a handler, so there is nothing to thread: the
 * dialog it sits in already knows how it closes.
 */
function Close() {
  return (
    <Radix.Close
      aria-label="Close"
      className={cn(
        'absolute top-3.5 right-3.5 z-10 flex size-[22px] items-center justify-center',
        'rounded-full text-secondary transition-colors',
        'hover:bg-[var(--hover)] hover:text-primary',
      )}
    >
      <X className="size-[15px]" strokeWidth={2} />
    </Radix.Close>
  )
}

Dialog.Header = Header
Dialog.Body = Body
Dialog.Footer = Footer
Dialog.Close = Close
