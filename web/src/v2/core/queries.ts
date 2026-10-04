import { useQuery, useQueryClient } from '@tanstack/react-query'

import { api, qk } from './api'
import type { Video } from './types'

/**
 * The three questions every view of a video asks: the record, its chapters and
 * its tasks.
 *
 * One hook, because the editor and the inspector ask them together and must
 * ask them under the same keys — two panes over one cache is one fetch and one
 * answer.
 *
 * The record is seeded from the library row while it is on its way. That puts
 * the title up on the first frame, and it carries the id the other two are
 * keyed by, so they start at once instead of a round trip later. A row is not
 * the whole record, though, so `ready` stays false until the record proper has
 * arrived; anything that reads more than the name should wait for it.
 */
export function useVideoDoc(ref: string) {
  const client = useQueryClient()
  const video = useQuery({
    queryKey: qk.video(ref),
    queryFn: () => api.getVideo(ref),
    enabled: Boolean(ref),
    placeholderData: () => client.getQueryData<Video[]>(qk.videos)?.find((row) => row.ref === ref),
  })
  const id = video.data?.id

  // Everything inside a video is keyed by its id, because that is what a delta
  // carries; see the note on `qk`.
  const chapters = useQuery({
    queryKey: qk.chapters(id ?? ''),
    queryFn: () => api.listChapters(ref),
    enabled: Boolean(id),
  })
  const tasks = useQuery({
    queryKey: qk.tasks(id ?? ''),
    queryFn: () => api.listTasks(ref),
    enabled: Boolean(id),
  })

  return { video, chapters, tasks, ready: Boolean(video.data) && !video.isPlaceholderData }
}
