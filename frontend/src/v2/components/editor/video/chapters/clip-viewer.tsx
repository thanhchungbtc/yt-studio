import { Dialog } from '../../../ui/dialog'

/**
 * One chapter's clip, played.
 *
 * The platform's video control, in a dialog, and nothing else. Everything worth
 * having here — the scrubber, the fullscreen button, the keyboard, the volume
 * route, and the byte ranges the asset handler already serves so scrubbing does
 * not fetch the whole file — arrives with the element.
 *
 * No autoplay. A modal that starts making noise the instant it opens is a modal
 * you close before you have finished reading its title.
 */
export function ClipViewer({
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
      <Dialog.Body bare>
        <div className="flex min-h-0 flex-1 items-center justify-center overflow-hidden px-5 pb-5">
          <video
            src={src}
            controls
            className="max-h-full max-w-full rounded-[6px] object-contain"
          />
        </div>
      </Dialog.Body>
      <Dialog.Close />
    </Dialog>
  )
}
