import { useMutation, useQueryClient } from '@tanstack/react-query'
import { LoaderCircle, Pencil } from 'lucide-react'
import { useState, type KeyboardEvent } from 'react'

import { api, qk } from '../../core/api'
import type { Video } from '../../core/types'
import { cn } from '../../core/utils'
import { Button } from '../ui/button'
import { Dialog } from '../ui/dialog'

/**
 * One thumbnail icon, large, with the prompt that drew it.
 *
 * `SlideViewer` for the grid under the headline, and deliberately the smaller
 * of the two. A slide is a picture somebody reads for a minute of narration, so
 * its viewer takes most of the window and keeps the prompt behind a toggle; an
 * icon is a square of line art on a tile, and the prompt is the only reason to
 * open it at all -- so the prompt is simply there.
 *
 * Opens over the builder rather than replacing it. Radix stacks dialogs and the
 * shell counts them, so Escape closes this one and leaves the builder up, which
 * is what redrawing one cell of a picture you are composing should do.
 */
export function IconViewer({
  video,
  index,
  onClose,
}: {
  video: Video
  /** Which cell of the grid, which is also where an edit is written. */
  index: number
  onClose: () => void
}) {
  const client = useQueryClient()
  const cell = video.thumbnailPlan[index]
  const assetId = video.thumbnailIconIds[index]
  const text = cell?.prompt.trim() ?? ''
  // Held here rather than in the panel because the dialog is what has to know
  // whether Escape has something to put back before it closes.
  const [draft, setDraft] = useState<string | null>(null)
  const editing = draft !== null

  const generate = useMutation({
    mutationFn: (next: string) => api.regenerateThumbnailIcon(video.ref, index, next),
    // The response is the whole video, so the panel's text is the server's copy
    // of it a frame later. The picture is not patched here and does not need to
    // be: the icon task runs again from this call, and the new asset arrives on
    // the event stream.
    onSuccess: (next) => {
      client.setQueryData(qk.video(video.ref), next)
      setDraft(null)
    },
  })

  const value = draft ?? text
  const unchanged = value.trim() === text
  const empty = value.trim() === ''
  const busy = generate.isPending

  const submit = () => {
    if (empty || unchanged || busy) return
    generate.mutate(value.trim())
  }

  // ⌘Return submits, which is what every editor of a multi-line field does;
  // plain Return has to stay a newline in a prompt. Escape is not handled here
  // — Radix listens on the document, so putting the draft back happens where
  // the close does, in the dialog's `onEscape`.
  const onKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
      event.preventDefault()
      submit()
    }
  }

  return (
    <Dialog
      open
      onOpenChange={(next) => {
        if (!next) onClose()
      }}
      width={560}
      onEscape={() => {
        if (draft === null) return false
        setDraft(null)
        return true
      }}
    >
      <Dialog.Header title={cell?.caption || `Cell ${index + 1}`} description="Thumbnail icon" />
      <Dialog.Close />

      <Dialog.Body>
        <div className="flex flex-col gap-4">
          {/* On the tile's own dark plate rather than the dialog's surface: the
              artwork is white line art keyed onto that plate, and on a light
              background it is a white square. */}
          <div
            className="flex aspect-square w-full items-center justify-center overflow-hidden rounded-[10px]"
            style={{ backgroundColor: '#060608' }}
          >
            {assetId ? (
              <img src={`/assets/${assetId}`} alt="" className="size-full object-contain" />
            ) : (
              <span className="text-[12px] text-tertiary">Not drawn yet.</span>
            )}
          </div>

          {editing ? (
            <textarea
              autoFocus
              value={value}
              disabled={busy}
              onChange={(event) => setDraft(event.target.value)}
              onKeyDown={onKeyDown}
              aria-label="Icon prompt"
              className={cn(
                'min-h-[8rem] w-full resize-none rounded-[7px] px-2 py-1.5',
                'font-mono text-[12px] leading-[1.55]',
                'disabled:opacity-50',
              )}
              style={{ boxShadow: '0 0 0 0.5px var(--separator-strong)' }}
            />
          ) : (
            <pre className="max-h-[8rem] overflow-y-auto font-mono text-[12px] leading-[1.55] whitespace-pre-wrap text-secondary select-text">
              {text || 'No prompt yet.'}
            </pre>
          )}

          <div className="flex items-center justify-end gap-2">
            {generate.error ? (
              <p className="mr-auto text-[11px] leading-snug text-[var(--failed)]">
                {generate.error.message}
              </p>
            ) : null}
            {editing ? (
              <>
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
              </>
            ) : (
              // The one control that spends money is the one that has to be
              // reached for: a field always live would make redrawing an icon
              // something a person can fall into, and it re-runs a backend task.
              <Button onClick={() => setDraft(text)} disabled={!text}>
                <span className="flex items-center gap-1.5">
                  <Pencil className="size-[13px]" strokeWidth={2} />
                  Edit the prompt
                </span>
              </Button>
            )}
          </div>
        </div>
      </Dialog.Body>
    </Dialog>
  )
}
