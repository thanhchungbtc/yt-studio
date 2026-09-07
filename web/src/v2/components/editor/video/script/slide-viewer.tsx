import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Check, Copy, Info, LoaderCircle, Pencil } from 'lucide-react'
import { useState, type KeyboardEvent } from 'react'

import { api, qk } from '../../../../core/api'
import type { Chapter } from '../../../../core/types'
import { cn } from '../../../../core/utils'
import { Button } from '../../../ui/button'
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
  chapterId,
  slot,
  videoId,
  src,
  title,
  prompt,
  stale,
  drawing,
  onClose,
}: {
  chapterId: string
  /** Which slide of the chapter, which is also where an edit is written. */
  slot: number
  /** The cache an edit patches; the response carries the whole chapter. */
  videoId: string
  src: string
  title: string
  /** What the image was generated from. Absent before the prompts are written. */
  prompt?: string
  /** An input moved after this was drawn, so the prompt may not be its own. */
  stale?: boolean
  /** The slide is being drawn again right now. */
  drawing?: boolean
  onClose: () => void
}) {
  const [showPrompt, setShowPrompt] = useState(remembered)
  // Held here rather than in the panel because the dialog is what has to know
  // whether Escape has something to put back before it closes.
  const [draft, setDraft] = useState<string | null>(null)
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
      onEscape={() => {
        if (draft === null) return false
        setDraft(null)
        return true
      }}
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
          <Framed src={src} title={title} drawing={drawing}>
            {showPrompt && text ? (
              <PromptPanel
                text={text}
                stale={stale}
                drawing={drawing}
                chapterId={chapterId}
                slot={slot}
                videoId={videoId}
                draft={draft}
                setDraft={setDraft}
              />
            ) : null}
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
  drawing,
  children,
}: {
  src: string
  title: string
  drawing?: boolean
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
          'object-contain transition-opacity duration-200',
          // Once the frame has the picture's shape the image simply fills it;
          // before that it is what decides the shape.
          ratio ? 'size-full' : 'max-h-full max-w-full',
          // Dimmed while its replacement is being drawn. Still shown, because it
          // is the only picture there is until the new one lands, and a slot
          // that went blank would read as work being destroyed rather than
          // redone — the old slide is kept precisely so it is not.
          drawing && 'opacity-40',
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
function PromptPanel({
  text,
  stale,
  drawing,
  chapterId,
  slot,
  videoId,
  draft,
  setDraft,
}: {
  text: string
  stale?: boolean
  drawing?: boolean
  chapterId: string
  slot: number
  videoId: string
  /** Null while reading; the editor's text otherwise. Owned by the dialog. */
  draft: string | null
  setDraft: (draft: string | null) => void
}) {
  const client = useQueryClient()
  const editing = draft !== null

  const generate = useMutation({
    mutationFn: (next: string) => api.regenerateSlide(chapterId, slot, next),
    // The response is the whole chapter, so the panel's text is the server's
    // copy of it a frame later. The picture is not patched here and does not
    // need to be: the slide runs again from this call, and the viewer is keyed
    // on the slot, so the new asset arrives on the event stream.
    onSuccess: (updated) => {
      client.setQueryData<Chapter[]>(qk.chapters(videoId), (prev) =>
        prev?.map((row) => (row.id === updated.id ? updated : row)),
      )
      setDraft(null)
    },
  })

  const value = draft ?? text
  const unchanged = value.trim() === text.trim()
  const empty = value.trim() === ''
  const busy = generate.isPending || drawing

  const submit = () => {
    if (empty || unchanged || busy) return
    generate.mutate(value.trim())
  }

  // ⌘Return submits, which is what every editor of a multi-line field does;
  // plain Return has to stay a newline in a prompt. Escape is not handled here
  // — Radix listens on the document, so putting the draft back has to happen
  // where the close does, which is the dialog's `onEscape`.
  const onKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
      event.preventDefault()
      submit()
    }
  }

  return (
    // The wrapper takes no pointer events so the picture underneath stays
    // clickable to either side of the panel; the panel takes its own back.
    <div className="pointer-events-none absolute inset-x-0 bottom-0 flex justify-center p-3">
      <div className="surface-overlay pointer-events-auto w-full max-w-[46rem] rounded-[10px] px-3 py-2.5">
        <div className="flex items-start gap-2">
          {editing ? (
            <textarea
              autoFocus
              value={value}
              disabled={busy}
              onChange={(event) => setDraft(event.target.value)}
              onKeyDown={onKeyDown}
              aria-label="Slide prompt"
              // Taller than the reading panel, and only while editing: eight
              // lines is a prompt you can see all of at once, and it is the
              // picture that pays for them, so it pays only while it has to.
              className={cn(
                'field-on-glass min-h-[9rem] min-w-0 flex-1 resize-none rounded-[7px] px-2 py-1.5',
                'font-mono text-[12px] leading-[1.55] text-white',
                'disabled:opacity-50',
              )}
            />
          ) : (
            <pre className="max-h-[4.4rem] min-w-0 flex-1 overflow-y-auto font-mono text-[12px] leading-[1.55] whitespace-pre-wrap text-white/90 select-text">
              {text}
            </pre>
          )}

          {editing ? null : (
            <>
              <CopyButton text={text} />
              {/* The one control that spends money is the one that has to be
                  reached for. A field always live would make redrawing a slide
                  something a person can fall into, and it re-runs a backend
                  task. */}
              <GlassButton
                icon={Pencil}
                label="Edit the prompt"
                disabled={busy}
                onClick={() => setDraft(text)}
              />
            </>
          )}
        </div>

        {editing ? (
          <div className="mt-2 flex items-center justify-end gap-2">
            {generate.error ? (
              <p className="mr-auto text-[11px] leading-snug text-[var(--failed)]">
                {(generate.error as Error).message}
              </p>
            ) : null}
            <Button onClick={() => setDraft(null)} disabled={busy}>
              Cancel
            </Button>
            <Button primary onClick={submit} disabled={empty || unchanged || busy}>
              {busy ? (
                <span className="flex items-center gap-1.5">
                  <LoaderCircle className="size-[13px] animate-spin" strokeWidth={2.5} />
                  Drawing
                </span>
              ) : (
                'Save and generate'
              )}
            </Button>
          </div>
        ) : null}

        {/* While a redraw is running with the panel closed for editing, the
            button is gone and this is the only thing saying so. */}
        {drawing && !editing ? (
          <p className="mt-2 flex items-center gap-1.5 text-[11px] text-white/70">
            <LoaderCircle className="size-[12px] animate-spin" strokeWidth={2.5} />
            Drawing this slide again…
          </p>
        ) : null}

        {stale && !editing && !drawing ? (
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

/** A quiet icon button sized for the glass panel, where the app's own greys
 * would disappear against an arbitrary picture. */
function GlassButton({
  icon: Icon,
  label,
  disabled,
  onClick,
}: {
  icon: typeof Pencil
  label: string
  disabled?: boolean
  onClick: () => void
}) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      disabled={disabled}
      onClick={onClick}
      className={cn(
        'flex size-[22px] shrink-0 items-center justify-center rounded-[6px] transition-colors',
        'text-white/60 hover:bg-white/12 hover:text-white',
        'disabled:pointer-events-none disabled:opacity-40',
      )}
    >
      <Icon className="size-[14px]" strokeWidth={2} />
    </button>
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
