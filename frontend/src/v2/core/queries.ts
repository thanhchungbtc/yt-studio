import { useQuery, useQueryClient } from '@tanstack/react-query'

import { api, qk } from './api'
import type { Video } from './types'

export function useVideoDoc(ref: string) {
  const client = useQueryClient()
  const video = useQuery({
    queryKey: qk.video(ref),
    queryFn: () => api.getVideo(ref),
    enabled: Boolean(ref),
    placeholderData: () => client.getQueryData<Video[]>(qk.videos)?.find((row) => row.ref === ref),
  })
  const id = video.data?.id

  // Keyed by id, not ref: deltas carry the id.
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
