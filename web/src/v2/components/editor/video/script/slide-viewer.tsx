import { Check, Copy, Info } from 'lucide-react'
import { useState } from 'react'

import { cn } from '../../../../core/utils'
import { Dialog } from '../../../ui/dialog'

/**
 * One slide, large, with the prompt that drew it.
 *
 * A dialog rather than a bare picture on a scrim, and the reasons are all things
 * the picture cannot do for itself: Escape closes it, focus returns to the tile
 * that opened it, the workbench's shortcuts stand down while it is up — ⌘W
 * closing the tab behind a full-window image is not a corner case — and it
 * portals out of the reader, so the chapters no longer scroll under it when the
 * wheel turns.
 *
 * Composed rather than configured. The caller says what to show; nothing about a
 * dialog reaches the call site, so what this looks like is this file's business
 * and can change without touching the reader.
 *
 * Most of the window, not all of it. A sheet with a margin still reads as
 * something laid over the document, and something laid over a document is a
 * thing you expect to be able to dismiss.
 */
export function SlideViewer({
  src,
  title,
  prompt,
  stale,
  onClose,
}: {
  src: string
  title: string
  /** What the image was generated from. Absent before the prompts are written. */
  prompt?: string
  /** An input moved after this was drawn, so the prompt may not be its own. */
  stale?: boolean
  onClose: () => void
}) {
  const [showPrompt, setShowPrompt] = useState(remembered)
  const text = prompt?.trim()

  const toggle = () => {
    setShowPrompt((open) => {
      remembered = !open
      return !open
    })
  }

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

      {/* Beside the close box rather than in the header text, because it acts on
          the picture and not on the dialog. Absent when there is no prompt: a
          control whose only outcome is an empty panel is a control that teaches
          you to stop pressing it. */}
      {text ? (
        <button
          type="button"
          onClick={toggle}
          aria-pressed={showPrompt}
          aria-label={showPrompt ? 'Hide the prompt' : 'Show the prompt'}
          title={showPrompt ? 'Hide the prompt' : 'Show the prompt'}
          className={cn(
            'absolute top-3.5 right-[44px] z-10 flex size-[22px] items-center justify-center',
            'rounded-full transition-colors',
            showPrompt
              ? 'bg-[var(--accent)] text-white'
              : 'text-secondary hover:bg-[var(--hover)] hover:text-primary',
          )}
        >
          <Info className="size-[15px]" strokeWidth={2} />
        </button>
      ) : null}

      {/* Bare, because the picture is the content: a dialog's comfortable
          gutter around an image is just less image. */}
      <Dialog.Body bare>
        <div className="flex min-h-0 flex-1 items-center justify-center overflow-hidden px-5 pb-5">
          <Framed src={src} title={title}>
            {showPrompt && text ? <PromptPanel text={text} stale={stale} /> : null}
          </Framed>
        </div>
      </Dialog.Body>
      <Dialog.Close />
    </Dialog>
  )
}

/**
 * Whether the panel is up, kept across openings.
 *
 * A module variable rather than a store, because it is a preference about a
 * dialog rather than a fact about the video, and it should not outlive the
 * session: it answers "am I reading prompts right now", which is true of an
 * afternoon and not of an installation. Held outside the component because
 * Radix unmounts the dialog on close, so component state would forget the
 * answer between every slide — the one thing a toggle must not do.
 */
let remembered = true

/**
 * The picture, in a box the exact size of the picture.
 *
 * This is the whole reason the overlay looks attached rather than dropped on
 * top. `object-contain` letterboxes: a 1344×768 slide in a dialog most of the
 * window tall leaves deep empty gutters above and below it, and a panel pinned
 * to the *dialog* floats in that dead space with the image nowhere near it.
 *
 * So the frame takes the image's own aspect ratio and the overlay is positioned
 * against the frame. The ratio comes from the file rather than from a constant:
 * the generator's width and height are settings, so 1344×768 is today's answer
 * and not a guarantee, and a hardcoded ratio would silently mis-hug the day
 * somebody changes them.
 *
 * Until the header has been read there is no ratio to use, so the image is laid
 * out the ordinary way and the frame carries nothing. That lasts one paint of a
 * cached image, and the overlay is not drawn during it.
 */
function Framed({
  src,
  title,
  children,
}: {
  src: string
  title: string
  children: React.ReactNode
}) {
  const [ratio, setRatio] = useState<number | null>(null)

  return (
    <div
      className="relative max-h-full max-w-full"
      // `width: 100%` with a ratio and a height cap is how a box fits itself
      // into a container the way `object-contain` fits a picture: the width
      // gives way when the derived height would overflow.
      style={ratio ? { aspectRatio: ratio, width: '100%' } : undefined}
    >
      <img
        src={src}
        alt={title}
        onLoad={(event) => {
          const { naturalWidth, naturalHeight } = event.currentTarget
          if (naturalWidth > 0 && naturalHeight > 0) setRatio(naturalWidth / naturalHeight)
        }}
        className={cn(
          'object-contain',
          // Once the frame has the picture's shape the image simply fills it;
          // before that it is what decides the shape.
          ratio ? 'size-full' : 'max-h-full max-w-full',
        )}
      />
      {ratio ? children : null}
    </div>
  )
}

/**
 * The prompt, floating on the picture it produced.
 *
 * Inset from the image's edges rather than flush to them, so it reads as
 * something resting *on* the photograph — the same reason a Quick Look toolbar
 * does not touch the frame. Centred and capped at a measure: a prompt run the
 * full width of a wide slide is a single line of text a foot long, which is the
 * one shape prose cannot be read in.
 *
 * Three lines and then it scrolls. A prompt is a few hundred characters and
 * would happily take a third of the picture; bounded, it costs about a seventh
 * of a 768-tall slide and the rest of the image survives. The face is the same
 * mono the reader sets narration in, which is what says *generated* here as
 * well as there.
 */
function PromptPanel({ text, stale }: { text: string; stale?: boolean }) {
  return (
    // The wrapper takes no pointer events so the picture underneath stays
    // clickable to either side of the panel; the panel takes its own back.
    <div className="pointer-events-none absolute inset-x-0 bottom-0 flex justify-center p-3">
      <div className="surface-overlay pointer-events-auto w-full max-w-[46rem] rounded-[10px] px-3 py-2.5">
        <div className="flex items-start gap-2">
          <pre className="max-h-[4.4rem] min-w-0 flex-1 overflow-y-auto font-mono text-[12px] leading-[1.55] whitespace-pre-wrap text-white/90 select-text">
            {text}
          </pre>
          <CopyButton text={text} />
        </div>
        {stale ? (
          // Only when it is true, which is what keeps it from being chrome. The
          // prompts are rewritten as a batch, so a slide drawn before that
          // happened is one whose panel would otherwise state, with no hedging
          // at all, the text of a picture that does not exist.
          <p
            className="mt-2 text-[11px] leading-snug"
            style={{ color: 'color-mix(in srgb, var(--running) 82%, white)' }}
          >
            The prompts were rewritten after this slide was drawn — it may not be what produced this
            image.
          </p>
        ) : null}
      </div>
    </div>
  )
}

/**
 * Copy, and say so.
 *
 * The confirmation is the icon becoming a tick for a moment rather than a
 * message appearing somewhere: the question "did that work" is asked of the
 * button that was just pressed, and answering it anywhere else makes you look
 * for the answer.
 */
function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false)

  return (
    <button
      type="button"
      aria-label={copied ? 'Prompt copied' : 'Copy the prompt'}
      title={copied ? 'Copied' : 'Copy the prompt'}
      onClick={() => {
        void navigator.clipboard.writeText(text).then(() => {
          setCopied(true)
          // Long enough to be seen, short enough that the button is ready again
          // before anybody wants it. Nothing cancels it: the panel outlives the
          // timer in every case except closing the dialog, which unmounts the
          // component and takes the pending state with it.
          setTimeout(() => setCopied(false), 1400)
        })
      }}
      className={cn(
        'flex size-[22px] shrink-0 items-center justify-center rounded-[6px]',
        'transition-colors',
        copied ? 'text-[var(--done)]' : 'text-white/60 hover:bg-white/12 hover:text-white',
      )}
    >
      {copied ? (
        <Check className="size-[14px]" strokeWidth={2.5} />
      ) : (
        <Copy className="size-[14px]" strokeWidth={2} />
      )}
    </button>
  )
}
