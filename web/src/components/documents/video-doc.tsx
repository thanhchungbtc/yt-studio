import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Ban, Play } from 'lucide-react'
import { useMemo, useState } from 'react'

import { DocFrame, type DocView } from '../editor/doc-frame'
import { resolveView } from '../lib/store'
import { Button } from '../ui/controls'
import { ErrorNotice, Progress, Skeleton } from '../ui/primitives'
import { VideoStateBadge } from '../ui/status'
import { BlueprintBar } from './video/blueprint-bar'
import { BlueprintTable } from './video/blueprint-table'
import { PublishView } from './video/publish-view'
import { columnTotals } from './video/stages'
import { api, qk } from '@/core/api'
import { videoAssetItems } from '@/core/assets'
import { percent } from '@/core/format'
import type { Video } from '@/core/types'

const VIEWS = ['blueprint', 'publish'] as const
type View = (typeof VIEWS)[number]

/**
 * The video document: the blueprint as a table, and what it publishes.
 *
 * It used to be three tabs — Chapters, Artifacts, Info — which split one object
 * along a seam that does not exist. An artifact *belongs to* a chapter, and
 * "Info" was the leftovers. Now every artifact the pipeline produces has a fixed
 * position in the row of the chapter that owns it, so reaching one is pointing
 * at a known place rather than navigating to it.
 */
export function VideoDoc({
  videoRef,
  view,
  onView,
}: {
  videoRef: string
  view: string | undefined
  onView: (view: string) => void
}) {
  const active = resolveView<View>(view, VIEWS, 'blueprint')
  const [planCollapsed, setPlanCollapsed] = useState(false)

  const video = useQuery({ queryKey: qk.video(videoRef), queryFn: () => api.getVideo(videoRef) })
  const videoId = video.data?.id

  const chapters = useQuery({
    queryKey: qk.chapters(videoId ?? ''),
    queryFn: () => api.listChapters(videoRef),
    enabled: Boolean(videoId),
  })
  const tasks = useQuery({
    queryKey: qk.videoTasks(videoId ?? ''),
    queryFn: () => api.listVideoTasks(videoRef),
    enabled: Boolean(videoId),
  })
  const assets = useQuery({
    queryKey: qk.assets(videoId ?? ''),
    queryFn: () => api.listAssets(videoRef),
    enabled: Boolean(videoId),
  })

  // Built once for the whole document: the lightbox walks every artifact the
  // video owns, whichever cell it was opened from.
  const artifacts = useMemo(
    () =>
      videoAssetItems(
        assets.data ?? [],
        chapters.data ?? [],
        video.data?.ref ?? '',
        tasks.data ?? [],
        video.data,
      ),
    [assets.data, chapters.data, tasks.data, video.data],
  )

  const totals = useMemo(
    () => columnTotals(chapters.data ?? [], video.data?.slidesPerChapter ?? 0),
    [chapters.data, video.data?.slidesPerChapter],
  )

  if (video.isPending) {
    return (
      <DocFrame crumbs={[videoRef]}>
        <div className="space-y-3 p-4">
          <Skeleton className="h-14 w-full" />
          <Skeleton className="h-40 w-full" />
        </div>
      </DocFrame>
    )
  }
  if (video.isError || !video.data) {
    return (
      <DocFrame crumbs={[videoRef]}>
        <ErrorNotice error={video.error ?? new Error('video not found')} className="m-4" />
      </DocFrame>
    )
  }

  const v = video.data
  const views: DocView[] = [
    { id: 'blueprint', label: 'Blueprint', count: chapters.data?.length },
    { id: 'publish', label: 'Publish' },
  ]

  return (
    <DocFrame
      crumbs={[
        <span key="ref" className="font-mono font-semibold text-[hsl(var(--accent))]">
          {v.ref}
        </span>,
        <span key="title" title={v.title}>
          {v.title}
        </span>,
      ]}
      views={views}
      activeView={active}
      onSelectView={onView}
      actions={
        <>
          <VideoStateBadge state={v.state} />
          <VideoActions video={v} />
        </>
      }
    >
      <div className="flex h-full min-h-0 flex-col">
        {/* A hairline the width of the document: progress as window chrome, not
            as a widget competing with the table below it. */}
        <Progress
          value={v.counts.succeeded}
          total={v.counts.total}
          failed={v.counts.failed}
          running={v.state === 'running'}
          className="h-[2px] shrink-0 rounded-none"
          aria-label={`${percent(v.counts.succeeded, v.counts.total)}% complete`}
        />

        {v.error && <ErrorNotice error={new Error(v.error)} className="m-3" />}

        {active === 'blueprint' ? (
          <>
            <BlueprintBar
              video={v}
              chapters={chapters.data?.length ?? 0}
              estimatedWords={totals.estimatedWords}
              collapsed={planCollapsed}
              onToggle={() => setPlanCollapsed((prev) => !prev)}
            />
            <div className="min-h-0 flex-1">
              <BlueprintTable
                video={v}
                chapters={chapters.data ?? []}
                tasks={tasks.data ?? []}
                artifacts={artifacts}
                loading={chapters.isPending}
              />
            </div>
          </>
        ) : (
          <div className="min-h-0 flex-1">
            <PublishView video={v} />
          </div>
        )}
      </div>
    </DocFrame>
  )
}

function VideoActions({ video }: { video: Video }) {
  const queryClient = useQueryClient()

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: qk.video(video.ref) })
    void queryClient.invalidateQueries({ queryKey: qk.videoTasks(video.id) })
    void queryClient.invalidateQueries({ queryKey: ['videos'] })
  }

  const start = useMutation({ mutationFn: () => api.startVideo(video.ref), onSuccess: invalidate })
  const cancel = useMutation({
    mutationFn: () => api.cancelVideo(video.ref),
    onSuccess: invalidate,
  })

  const canStart =
    video.state === 'draft' || video.state === 'cancelled' || video.state === 'failed'
  const canCancel = video.state === 'running' || video.state === 'awaiting_approval'

  return (
    <>
      {/* Approve and reject are deliberately absent: a gate belongs beside the
          pipeline it is holding up, which is the run panel. */}
      {canStart && (
        <Button
          variant="primary"
          size="xs"
          disabled={start.isPending}
          onClick={() => start.mutate()}
        >
          <Play className="h-3 w-3" />
          {video.state === 'draft' ? 'Start' : 'Resume'}
        </Button>
      )}
      {canCancel && (
        <Button
          variant="ghost"
          size="xs"
          disabled={cancel.isPending}
          onClick={() => cancel.mutate()}
        >
          <Ban className="h-3 w-3" />
          Cancel
        </Button>
      )}
    </>
  )
}
