import { Dialog } from '../../../ui/dialog'

/**
 * One slide, large.
 *
 * A dialog rather than a bare picture on a scrim, and the reasons are all things
 * the picture cannot do for itself: Escape closes it, focus returns to the tile
 * that opened it, the workbench's shortcuts stand down while it is up — ⌘W
 * closing the tab behind a full-window image is not a corner case — and it
 * portals out of the reader, so the chapters no longer scroll under it when the
 * wheel turns.
 *
 * Composed rather than configured. The caller says `src` and `title`; nothing
 * about a dialog reaches the call site, so what this looks like is this file's
 * business and can change without touching the reader.
 *
 * Most of the window, not all of it. A sheet with a margin still reads as
 * something laid over the document, and something laid over a document is a
 * thing you expect to be able to dismiss.
 */
export function SlideViewer({
  src,
  title,
  onClose,
}: {
  src: string
  title: string
  onClose: () => void
}) {
  return (
    <Dialog
      open
      onOpenChange={(next) => {
        if (!next) onClose()
      }}
      width="92vw"
      height="88vh"
    >
      <Dialog.Header title={title} />
      {/* Bare, because the picture is the content: a dialog's comfortable
          gutter around an image is just less image. */}
      <Dialog.Body bare>
        <div className="flex min-h-0 flex-1 items-center justify-center overflow-hidden px-5 pb-5">
          <img src={src} alt={title} className="max-h-full max-w-full object-contain" />
        </div>
      </Dialog.Body>
      <Dialog.Close />
    </Dialog>
  )
}
