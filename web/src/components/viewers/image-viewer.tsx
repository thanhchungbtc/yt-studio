import { ImageOff, Sparkles } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'

import { Button, Textarea } from '../ui/controls'
import { ErrorNotice, Modal } from '../ui/primitives'
import { cn } from '@/core/utils'

/**
 * An image, the prompt it was drawn from, and a button that draws it again.
 *
 * It knows nothing about what it is showing. Not that it is a slide, or a
 * thumbnail cell; not which chapter or slot it belongs to; not which endpoint
 * redraws it. The caller supplies a picture, some words, and a function —
 * everything specific stays at the call site, which is why the same component
 * serves a chapter's slide and a thumbnail grid cell without a discriminator.
 */
export function ImageViewer({
  title,
  subtitle,
  src,
  prompt,
  onGenerate,
  onClose,
}: {
  title: string
  subtitle?: string
  /** The picture. Absent means nothing has been drawn into this slot yet. */
  src?: string
  /** The words behind it. Absent means none has been written yet. */
  prompt?: string
  /** Runs the draw. Resolves when the request is accepted, not when it lands. */
  onGenerate: (prompt: string) => Promise<void>
  onClose: () => void
}) {
  const [draft, setDraft] = useState(prompt ?? '')
  const [error, setError] = useState<unknown>()
  const [drawing, setDrawing] = useState(false)
  const shown = useRef(src)

  // A prompt changed underneath us is a new subject, not an edit to discard.
  useEffect(() => setDraft(prompt ?? ''), [prompt])

  /**
   * The completion signal, with no knowledge of tasks, pools or the scheduler.
   *
   * The server keeps the old picture in place while the new one is drawn, and
   * the store is content-addressed — so a *change* of `src` is the redraw
   * arriving, and there is nothing else to subscribe to.
   */
  useEffect(() => {
    if (src === shown.current) return
    shown.current = src
    setDrawing(false)
  }, [src])

  const generate = async () => {
    setError(undefined)
    setDrawing(true)
    try {
      await onGenerate(draft.trim())
    } catch (failure) {
      // A refused request never draws, so the button must come back.
      setError(failure)
      setDrawing(false)
    }
  }

  return (
    <Modal
      open
      wide
      onOpenChange={(next) => {
        if (!next) onClose()
      }}
      title={title}
      {...(subtitle ? { description: subtitle } : {})}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Close
          </Button>
          <Button
            variant="primary"
            disabled={!draft.trim() || drawing}
            onClick={() => void generate()}
          >
            <Sparkles className={cn('h-3.5 w-3.5', drawing && 'animate-pulse')} />
            {drawing ? 'Drawing…' : 'Generate'}
          </Button>
        </>
      }
    >
      <div className="space-y-3">
        <div className="checker flex min-h-[220px] items-center justify-center rounded-[var(--radius-sm)] p-3">
          {src ? (
            <img
              src={src}
              alt={title}
              className="max-h-[46vh] max-w-full rounded-[var(--radius-xs)] object-contain elev-2"
            />
          ) : (
            <span className="flex flex-col items-center gap-2 text-[12px] text-subtle">
              <ImageOff className="h-6 w-6" strokeWidth={1.5} />
              Nothing drawn here yet.
            </span>
          )}
        </div>

        {prompt === undefined ? (
          <p className="text-[11.5px] text-subtle">
            No prompt has been written for this slot yet.
          </p>
        ) : (
          <Textarea
            rows={5}
            value={draft}
            onChange={(event) => setDraft(event.target.value)}
            spellCheck={false}
            aria-label="Prompt"
          />
        )}

        {error !== undefined && <ErrorNotice error={error} />}
      </div>
    </Modal>
  )
}
