import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useVirtualizer } from '@tanstack/react-virtual'
import {
  Ban,
  Download,
  FileText,
  Image as ImageIcon,
  Music,
  Play,
  Video as VideoIcon,
} from 'lucide-react'
import type { ReactNode } from 'react'
import { useMemo, useRef } from 'react'

import { ArtifactGallery } from '@/components/artifact-gallery'
import { DocFrame, type DocView } from '../editor/doc-frame'
import { resolveView } from '../lib/store'
import { Badge, Button } from '../ui/controls'
import {
  EmptyState,
  ErrorNotice,
  KeyValue,
  Progress,
  Section,
  Skeleton,
  Tooltip,
} from '../ui/primitives'
import { VideoStateBadge } from '../ui/status'
import { api, assetUrl, qk } from '@/core/api'
import { videoAssetItems } from '@/core/assets'
import { formatAbsolute, formatClock, percent } from '@/core/format'
import type { Chapter, Video } from '@/core/types'
import { cn } from '@/core/utils'

const VIEWS = ['chapters', 'artifacts', 'info'] as const
type View = (typeof VIEWS)[number]

/**
 * The video document.
 *
 * Three sections where the first draft had four tabs and the shell before it had
 * four plus two banners: the pipeline, the gate, the stale warning and the task
 * list all live in the run panel now. What is left in the middle is the thing
 * the video *is* rather than the state of the machine building it.
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
  const active = resolveView<View>(view, VIEWS, 'chapters')

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
    { id: 'chapters', label: 'Chapters', count: chapters.data?.length },
    { id: 'artifacts', label: 'Artifacts', count: artifacts.length },
    { id: 'info', label: 'Info' },
  ]

  return (
    <DocFrame
      crumbs={[
        <span key="ref" className="font-mono font-semibold text-[hsl(var(--accent))]">
          {v.ref}
        </span>,
        <span key="title" className="text-fg" title={v.title}>
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
            as a widget competing with the content below it. */}
        <Progress
          value={v.counts.succeeded}
          total={v.counts.total}
          failed={v.counts.failed}
          running={v.state === 'running'}
          className="h-[2px] shrink-0 rounded-none"
          aria-label={`${percent(v.counts.succeeded, v.counts.total)}% complete`}
        />
        <div className="min-h-0 flex-1 overflow-hidden">
          {active === 'chapters' && (
            <ChapterList chapters={chapters.data ?? []} loading={chapters.isPending} />
          )}
          {active === 'artifacts' && (
            <ArtifactGallery video={v} items={artifacts} loading={assets.isPending} />
          )}
          {active === 'info' && <Info video={v} />}
        </div>
      </div>
    </DocFrame>
  )
}

/* ----------------------------------------------------------------- actions */

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

/* ---------------------------------------------------------------- chapters */

/**
 * Virtualized: a video can carry five hundred chapters, and the first draft of
 * this list rendered every one of them. Only the rows in view exist.
 */
function ChapterList({ chapters, loading }: { chapters: Chapter[]; loading: boolean }) {
  const parent = useRef<HTMLDivElement>(null)
  const rows = useVirtualizer({
    count: chapters.length,
    getScrollElement: () => parent.current,
    estimateSize: () => 44,
    overscan: 12,
  })

  if (loading) {
    return (
      <div className="space-y-1 p-3">
        {Array.from({ length: 10 }, (_, i) => (
          <Skeleton key={i} className="h-9 w-full" />
        ))}
      </div>
    )
  }
  if (chapters.length === 0) {
    return (
      <EmptyState
        icon={<FileText />}
        title="No chapters yet"
        description="The blueprint decides how many there are. Approve it and they appear here."
      />
    )
  }

  return (
    <div ref={parent} className="h-full overflow-y-auto">
      <div className="relative w-full" style={{ height: rows.getTotalSize() }}>
        {rows.getVirtualItems().map((row) => {
          const chapter = chapters[row.index]
          if (!chapter) return null
          return (
            <div
              key={chapter.id}
              className="absolute inset-x-0 top-0 flex items-center gap-3 border-b border-[hsl(var(--border))] px-3 transition-colors hover:bg-[hsl(var(--bg-hover))]"
              style={{ height: row.size, transform: `translateY(${row.start}px)` }}
            >
              <span className="tabular w-8 shrink-0 text-right text-[11px] font-semibold text-subtle">
                {chapter.ordinal}
              </span>
              <div className="min-w-0 flex-1">
                <p className="truncate text-[12.5px] font-medium text-fg">
                  {chapter.title || <span className="text-subtle">Untitled</span>}
                </p>
                {chapter.summary && (
                  <p className="truncate text-[11px] text-muted">{chapter.summary}</p>
                )}
              </div>

              <div className="flex shrink-0 items-center gap-1.5">
                <Marker on={chapter.script.length > 0} label="Script written">
                  <FileText className="h-3 w-3" />
                </Marker>
                <Marker on={Boolean(chapter.audioAssetId)} label="Narration rendered">
                  <Music className="h-3 w-3" />
                </Marker>
                <Marker
                  on={chapter.slideAssetIds.length > 0}
                  label={`${chapter.slideAssetIds.length} of ${chapter.slidePrompts.length} slides drawn`}
                >
                  <ImageIcon className="h-3 w-3" />
                </Marker>
                <Marker on={Boolean(chapter.clipAssetId)} label="Clip composed">
                  <VideoIcon className="h-3 w-3" />
                </Marker>
              </div>

              <span className="tabular w-14 shrink-0 text-right text-[11px] text-subtle">
                {chapter.durationSeconds > 0 ? formatClock(chapter.durationSeconds) : '—'}
              </span>
            </div>
          )
        })}
      </div>
    </div>
  )
}

function Marker({ on, label, children }: { on: boolean; label: string; children: ReactNode }) {
  return (
    <Tooltip label={label}>
      <span
        className={cn(
          'flex h-4 w-4 items-center justify-center rounded-[var(--radius-xs)]',
          on ? 'text-[hsl(var(--success))]' : 'text-[hsl(var(--fg-subtle))] opacity-40',
        )}
      >
        {children}
      </span>
    </Tooltip>
  )
}

/* -------------------------------------------------------------------- info */

function Info({ video }: { video: Video }) {
  return (
    <div className="h-full overflow-y-auto p-4">
      <div className="mx-auto max-w-3xl space-y-5">
        {video.finalAssetId && (
          <Section
            title="Final render"
            actions={
              <Button size="xs" variant="ghost" asChild>
                <a href={assetUrl(video.finalAssetId)} download>
                  <Download className="h-3 w-3" />
                  Download
                </a>
              </Button>
            }
          >
            <video
              controls
              preload="metadata"
              src={assetUrl(video.finalAssetId)}
              className="max-h-[46vh] w-full rounded-[var(--radius-md)] bg-black"
            />
          </Section>
        )}

        {video.topic && (
          <Section title="Topic">
            <p className="text-[12.5px] leading-relaxed text-muted">{video.topic}</p>
          </Section>
        )}

        {video.metadata && (
          <Section title="Publish metadata">
            <div className="surface flex items-start gap-3 p-3">
              {video.effectiveThumbnailAssetId && (
                <img
                  src={assetUrl(video.effectiveThumbnailAssetId)}
                  alt={`Thumbnail for ${video.title}`}
                  className="aspect-video w-40 shrink-0 rounded-[var(--radius-sm)] bg-black object-cover"
                />
              )}
              <div className="min-w-0 flex-1 space-y-1">
                <p className="text-[12.5px] font-medium text-fg">{video.metadata.title}</p>
                <p className="line-clamp-3 text-[11.5px] text-muted">
                  {video.metadata.description}
                </p>
                <div className="flex flex-wrap gap-1 pt-0.5">
                  <Badge tone="neutral">{video.metadata.privacy}</Badge>
                  {video.metadata.tags.slice(0, 6).map((tag) => (
                    <Badge key={tag} tone="neutral">
                      {tag}
                    </Badge>
                  ))}
                </div>
              </div>
            </div>
          </Section>
        )}

        <Section title="Facts">
          <dl className="surface divide-y divide-[hsl(var(--border))] px-3">
            <KeyValue label="Ref">{video.ref}</KeyValue>
            <KeyValue label="Chapters">{video.chapterCount}</KeyValue>
            <KeyValue label="Slides per chapter">{video.slidesPerChapter}</KeyValue>
            <KeyValue label="Thumbnail tiles">{video.thumbnailCells}</KeyValue>
            <KeyValue label="Target duration">{video.targetDurationMinutes} min</KeyValue>
            <KeyValue label="Created">{formatAbsolute(video.createdAt)}</KeyValue>
            <KeyValue label="Started">{formatAbsolute(video.startedAt)}</KeyValue>
            <KeyValue label="Completed">{formatAbsolute(video.completedAt)}</KeyValue>
          </dl>
        </Section>

        {video.error && <ErrorNotice error={new Error(video.error)} />}
      </div>
    </div>
  )
}
