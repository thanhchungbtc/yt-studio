import { useQueryClient } from '@tanstack/react-query'

import { ImageViewer } from '../../viewers/image-viewer'
import { api, assetUrl, qk } from '@/core/api'
import { chapterKey } from '@/core/format'
import type { Chapter, Video } from '@/core/types'

/**
 * The slide half of the generic image viewer: everything the viewer refuses to
 * know, kept in one small place.
 *
 * The subject is a *slot*, not an asset. A slide that failed or has not been
 * drawn has no artifact behind it, and that slot is exactly the one whose prompt
 * you want to change — so addressing it by chapter and index is what makes an
 * empty square openable at all.
 */
export function SlideViewer({
  video,
  chapter,
  slot,
  onClose,
}: {
  video: Video
  /** Read live by the caller, so a redraw lands here without reopening. */
  chapter: Chapter
  slot: number
  onClose: () => void
}) {
  const queryClient = useQueryClient()

  return (
    <ImageViewer
      title={`Slide ${slot + 1}`}
      subtitle={`${chapterKey(video.ref, chapter.ordinal)} · ${chapter.title}`}
      src={assetUrl(chapter.slideAssetIds[slot])}
      prompt={chapter.slidePrompts[slot]}
      onClose={onClose}
      onGenerate={async (prompt) => {
        // Saving the prompt and drawing from it are one call: there is no way to
        // store a prompt the picture on screen was not drawn from.
        const updated = await api.regenerateSlide(chapter.id, slot, prompt)
        queryClient.setQueryData<Chapter[]>(qk.chapters(video.id), (prev) =>
          prev?.map((c) => (c.id === updated.id ? updated : c)),
        )
        // The redraw is a task; the run panel and the table's own cells report it.
        void queryClient.invalidateQueries({ queryKey: qk.videoTasks(video.id) })
      }}
    />
  )
}
