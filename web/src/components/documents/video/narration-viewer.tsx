import { Download, ExternalLink } from 'lucide-react'
import { useMemo } from 'react'

import { Button } from '../../ui/controls'
import { Modal } from '../../ui/primitives'
import { assetUrl } from '@/core/api'
import { chapterKey, formatClock } from '@/core/format'
import type { Chapter } from '@/core/types'

/**
 * One chapter's narration, and nothing else.
 *
 * It replaces what the generic artifact viewer did for audio. That viewer took a
 * *list* and an index, so opening one narration mounted a gallery: prev/next
 * arrows, a kind-icon table, an inspector panel, an artifact counter. None of it
 * was reachable from anywhere that had more than one thing to show, and all of
 * it had to exist for the one case that did.
 *
 * The subject here is the chapter, not an asset id — the same reframe the slide
 * viewer will want, because what you are looking at is a slot in the pipeline and
 * the file is merely what currently fills it.
 */
export function NarrationViewer({
  videoRef,
  chapter,
  onClose,
}: {
  videoRef: string
  chapter: Chapter
  onClose: () => void
}) {
  const url = assetUrl(chapter.audioAssetId)

  // The server sends a content type but no disposition, so a bare `download`
  // would save the content hash with no extension.
  const filename = useMemo(() => {
    const slug =
      `${chapterKey(videoRef, chapter.ordinal)} ${chapter.title}`
        .toLowerCase()
        .replace(/[^a-z0-9]+/g, '-')
        .replace(/^-+|-+$/g, '')
        .slice(0, 60) || 'narration'
    return `${slug}-${(chapter.audioAssetId ?? '').slice(0, 8)}.wav`
  }, [videoRef, chapter.ordinal, chapter.title, chapter.audioAssetId])

  return (
    <Modal
      open
      onOpenChange={(next) => {
        if (!next) onClose()
      }}
      title="Narration"
      description={`${chapterKey(videoRef, chapter.ordinal)} · ${chapter.title}`}
      footer={
        <>
          <span className="tabular mr-auto text-[11.5px] text-subtle">
            {chapter.durationSeconds > 0 ? formatClock(chapter.durationSeconds) : '—'}
          </span>
          <Button variant="ghost" size="sm" asChild>
            <a href={url} target="_blank" rel="noreferrer">
              <ExternalLink className="h-3.5 w-3.5" />
              Open raw
            </a>
          </Button>
          <Button variant="outline" size="sm" asChild>
            <a href={url} download={filename}>
              <Download className="h-3.5 w-3.5" />
              Download
            </a>
          </Button>
        </>
      }
    >
      <div className="flex flex-col items-center gap-5 py-2">
        <Waveform />
        {/* Autoplays: opening this is the act of asking to hear it. */}
        <audio controls autoPlay preload="metadata" src={url} className="w-full max-w-md">
          <track kind="captions" />
        </audio>
      </div>
    </Modal>
  )
}

/**
 * A static bar chart standing in for the narration. It is decorative — the
 * server stores no waveform — but an audio artifact with nothing above the
 * transport reads as a broken panel.
 */
function Waveform() {
  const bars = useMemo(
    () => Array.from({ length: 56 }, (_, i) => 20 + Math.abs(Math.sin(i * 0.7)) * 70),
    [],
  )
  return (
    <div className="flex h-24 w-full max-w-md items-center justify-center gap-[3px]" aria-hidden>
      {bars.map((height, i) => (
        <span
          key={i}
          className="w-[3px] rounded-full bg-[hsl(var(--info)/0.45)]"
          style={{ height: `${height}%` }}
        />
      ))}
    </div>
  )
}
