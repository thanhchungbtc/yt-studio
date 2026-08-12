import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'

import { Button, Textarea } from '../../ui/controls'
import { ErrorNotice, Modal } from '../../ui/primitives'
import { api, qk } from '@/core/api'
import type { Chapter } from '@/core/types'
import { wordsIn } from './stages'

/**
 * The script, read and edited.
 *
 * A dialog rather than the popover I first drew: a popover anchored to a table
 * cell is fine for glancing at a paragraph and cramped for rewriting one, and
 * this is the surface where a bad chapter actually gets fixed. Editing also
 * wants a focus trap and a deliberate dismissal, which a popover does not give.
 *
 * Re-running narration is left alone on save. The script and the audio drawn
 * from it are separate artifacts, and the scheduler already has a word for
 * "this is intact but its input moved" — the save marks the downstream stale
 * and lets the operator decide when to spend the TTS budget.
 */
export function ScriptDialog({
  chapter,
  videoRef,
  videoId,
  estimatedWords,
  onClose,
}: {
  chapter: Chapter
  videoRef: string
  videoId: string
  estimatedWords: number
  onClose: () => void
}) {
  const queryClient = useQueryClient()
  const [draft, setDraft] = useState(chapter.script)
  const [editing, setEditing] = useState(false)

  // A different chapter in the same mounted dialog is a different document.
  useEffect(() => {
    setDraft(chapter.script)
    setEditing(false)
  }, [chapter.id, chapter.script])

  const save = useMutation({
    mutationFn: () => api.updateChapterScript(chapter.id, draft),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: qk.chapters(videoId) })
      void queryClient.invalidateQueries({ queryKey: qk.videoTasks(videoId) })
      void queryClient.invalidateQueries({ queryKey: qk.video(videoRef) })
      setEditing(false)
    },
  })

  const dirty = draft !== chapter.script
  const words = wordsIn(draft)

  return (
    <Modal
      open
      wide
      // A half-typed rewrite must not be thrown away by a stray click outside.
      onOpenChange={(next) => {
        if (!next && !(editing && dirty)) onClose()
      }}
      title={`${chapter.ordinal}. ${chapter.title || 'Untitled'}`}
      description={chapter.summary}
      footer={
        editing ? (
          <>
            <span className="mr-auto text-[11.5px] text-subtle">
              <span className="tabular font-medium text-muted">{words}</span> words
              {estimatedWords > 0 && (
                <>
                  {' '}
                  · budget <span className="tabular">{estimatedWords}</span>
                </>
              )}
            </span>
            <Button
              variant="ghost"
              disabled={save.isPending}
              onClick={() => {
                setDraft(chapter.script)
                setEditing(false)
              }}
            >
              Cancel
            </Button>
            <Button
              variant="primary"
              disabled={!dirty || save.isPending}
              onClick={() => save.mutate()}
            >
              {save.isPending ? 'Saving…' : 'Save'}
            </Button>
          </>
        ) : (
          <>
            <span className="mr-auto text-[11.5px] text-subtle">
              <span className="tabular font-medium text-muted">{words}</span> words
              {estimatedWords > 0 && (
                <>
                  {' '}
                  · budget <span className="tabular">{estimatedWords}</span>
                </>
              )}
            </span>
            <Button variant="ghost" onClick={onClose}>
              Close
            </Button>
            <Button variant="outline" onClick={() => setEditing(true)}>
              Edit
            </Button>
          </>
        )
      }
    >
      {editing ? (
        <Textarea
          rows={18}
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
          aria-label="Chapter script"
          autoFocus
        />
      ) : (
        <p className="whitespace-pre-wrap font-mono text-[12.5px] leading-relaxed text-fg">
          {chapter.script || <span className="text-subtle">Not written yet.</span>}
        </p>
      )}
      {save.isError && <ErrorNotice error={save.error} className="mt-3" />}
    </Modal>
  )
}
