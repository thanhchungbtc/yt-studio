import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useParams } from '@tanstack/react-router'
import { useVirtualizer } from '@tanstack/react-virtual'
import {
  Ban,
  Check,
  ChevronDown,
  ChevronRight,
  Download,
  FileJson,
  LayoutGrid,
  Maximize2,
  Pencil,
  Play,
  RefreshCw,
  Rows3,
  X,
} from 'lucide-react'
import { memo, useCallback, useMemo, useRef, useState } from 'react'

import {
  assetKindTone,
  AssetPreview,
  AssetViewerProvider,
  useAssetViewer,
} from '@/components/asset-viewer'
import { StageStrip } from '@/components/stage-strip'
import { RerunDialog, StaleBadge, StaleBanner, StaleDot } from '@/components/stale'
import { TaskStateBadge, VideoStateBadge } from '@/components/state-badges'
import { Badge } from '@/components/ui/badge'
import type { Tone } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/field'
import {
  Divider,
  EmptyState,
  ErrorNotice,
  Kbd,
  KeyValue,
  Modal,
  Mono,
  Panel,
  PanelHeader,
  PanelTitle,
  Progress,
  Ring,
  Segmented,
  Skeleton,
  Toolbar,
  Tooltip,
} from '@/components/ui/primitives'
import { api, assetUrl, qk } from '@/lib/api'
import {
  chapterStillItems,
  downloadName,
  kindMime,
  kindTitle,
  mediaTypeOf,
  shortId,
  videoAssetItems,
} from '@/lib/assets'
import type { ViewerItem } from '@/lib/assets'
import {
  chapterKey,
  formatAbsolute,
  formatBytes,
  formatClock,
  formatRelative,
  percent,
  taskLabel,
} from '@/lib/format'
import { useHotkeys } from '@/lib/hotkeys'
import type { Chapter, GateKind, Task, Video } from '@/lib/types'
import { cn } from '@/lib/utils'

type Tab = 'overview' | 'chapters' | 'tasks' | 'artifacts'

const TABS: Tab[] = ['overview', 'chapters', 'tasks', 'artifacts']

/**
 * The detail pane. It lives to the right of the splitter inside the `/videos`
 * layout, so it owns everything below the toolbar and nothing above it.
 */
export function VideoDetailRoute() {
  const { ref } = useParams({ from: '/videos/$ref' })
  const [tab, setTab] = useState<Tab>('overview')

  const video = useQuery({ queryKey: qk.video(ref), queryFn: () => api.getVideo(ref) })
  const videoId = video.data?.id

  const chapters = useQuery({
    queryKey: qk.chapters(videoId ?? ''),
    queryFn: () => api.listChapters(ref),
    enabled: Boolean(videoId),
  })
  const tasks = useQuery({
    queryKey: qk.videoTasks(videoId ?? ''),
    queryFn: () => api.listVideoTasks(ref),
    enabled: Boolean(videoId),
  })

  // Tabs move under the same modifier the rest of the shell uses, so a whole
  // review pass — open video, scan stages, read chapters — never needs a mouse.
  useHotkeys([
    {
      keys: 'mod+arrowright',
      label: 'Next tab',
      group: 'Video',
      run: () => setTab((prev) => TABS[(TABS.indexOf(prev) + 1) % TABS.length] ?? prev),
    },
    {
      keys: 'mod+arrowleft',
      label: 'Previous tab',
      group: 'Video',
      run: () =>
        setTab((prev) => TABS[(TABS.indexOf(prev) - 1 + TABS.length) % TABS.length] ?? prev),
    },
  ])

  if (video.isPending) {
    return (
      <>
        <Toolbar>
          <Skeleton className="h-4 w-56" />
        </Toolbar>
        <div className="space-y-3 p-4">
          <Skeleton className="h-16 w-full" />
          <Skeleton className="h-40 w-full" />
        </div>
      </>
    )
  }
  if (video.isError || !video.data) {
    return (
      <>
        <Toolbar />
        <ErrorNotice error={video.error ?? new Error('video not found')} className="m-4" />
      </>
    )
  }

  const v = video.data
  const openGate = tasks.data?.find((t) => t.state === 'awaiting_approval')

  return (
    <AssetViewerProvider>
      <Toolbar className="gap-3">
        <Ring
          value={v.counts.succeeded}
          total={v.counts.total}
          failed={v.counts.failed}
          size={18}
        />
        <div className="flex min-w-0 items-baseline gap-2">
          <span className="shrink-0 font-mono text-[12px] font-semibold text-[hsl(var(--accent))]">
            {v.ref}
          </span>
          <h1 className="truncate text-[13.5px] font-medium text-fg" title={v.title}>
            {v.title}
          </h1>
        </div>
        <VideoStateBadge state={v.state} className="shrink-0" />
        <span className="tabular shrink-0 text-[11.5px] text-subtle">
          {v.counts.succeeded}/{v.counts.total}
        </span>

        <div className="ml-auto flex shrink-0 items-center gap-2">
          <VideoActions video={v} openGate={openGate} />
        </div>
      </Toolbar>

      {/* A hairline the width of the pane: progress as window chrome, not as a
          widget competing with the content below it. */}
      <Progress
        value={v.counts.succeeded}
        total={v.counts.total}
        failed={v.counts.failed}
        running={v.state === 'running'}
        className="h-[3px] shrink-0 rounded-none"
        aria-label={`${percent(v.counts.succeeded, v.counts.total)}% complete`}
      />

      {openGate && <GateBanner video={v} task={openGate} />}
      <StaleBanner video={v} tasks={tasks.data ?? []} />
      {v.error && <ErrorNotice error={new Error(v.error)} className="mx-4 mt-3" />}

      <div className="flex shrink-0 items-center gap-3 border-b border-[hsl(var(--border))] px-3 py-2">
        <Segmented
          aria-label="Video sections"
          value={tab}
          onChange={setTab}
          options={[
            { value: 'overview', label: 'Overview' },
            { value: 'chapters', label: 'Chapters', count: chapters.data?.length },
            { value: 'tasks', label: 'Tasks', count: tasks.data?.length },
            { value: 'artifacts', label: 'Artifacts' },
          ]}
          className="w-[380px]"
        />
        <Divider className="hidden sm:block" />
        <span className="hidden items-center gap-1.5 text-[11px] text-subtle sm:flex">
          <Kbd keys="mod+arrowleft" />
          <Kbd keys="mod+arrowright" />
          switch
        </span>
        <span className="ml-auto text-[11px] tabular text-subtle">
          updated {formatRelative(v.updatedAt)}
        </span>
      </div>

      <div className="min-h-0 flex-1 overflow-hidden">
        {tab === 'overview' && (
          <Overview video={v} tasks={tasks.data ?? []} loading={tasks.isPending} />
        )}
        {tab === 'chapters' && (
          <ChapterGrid
            video={v}
            chapters={chapters.data ?? []}
            tasks={tasks.data ?? []}
            loading={chapters.isPending}
          />
        )}
        {tab === 'tasks' && <TaskList tasks={tasks.data ?? []} loading={tasks.isPending} />}
        {tab === 'artifacts' && <ArtifactGallery video={v} chapters={chapters.data ?? []} />}
      </div>
    </AssetViewerProvider>
  )
}

/* ------------------------------------------------------------- video actions */

function VideoActions({ video, openGate }: { video: Video; openGate: Task | undefined }) {
  const queryClient = useQueryClient()
  const [rejecting, setRejecting] = useState(false)
  const [reason, setReason] = useState('')

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: qk.video(video.ref) })
    void queryClient.invalidateQueries({ queryKey: qk.videoTasks(video.id) })
    void queryClient.invalidateQueries({ queryKey: ['videos'] })
  }

  const approve = useMutation({
    mutationFn: () => api.approveGate(video.ref, (openGate?.gate as GateKind) ?? 'blueprint'),
    onSuccess: invalidate,
  })
  const reject = useMutation({
    mutationFn: () =>
      api.rejectGate(video.ref, (openGate?.gate as GateKind) ?? 'blueprint', reason),
    onSuccess: () => {
      setRejecting(false)
      setReason('')
      invalidate()
    },
  })
  const start = useMutation({ mutationFn: () => api.startVideo(video.ref), onSuccess: invalidate })
  const cancel = useMutation({
    mutationFn: () => api.cancelVideo(video.ref),
    onSuccess: invalidate,
  })

  const canStart =
    video.state === 'draft' || video.state === 'cancelled' || video.state === 'failed'
  const canCancel = video.state === 'running' || video.state === 'awaiting_approval'

  useHotkeys([
    {
      keys: 'mod+enter',
      label: 'Approve the open gate',
      group: 'Video',
      run: () => {
        if (openGate && !approve.isPending) approve.mutate()
      },
    },
  ])

  return (
    <>
      {openGate && (
        <>
          <Tooltip label="Approve and let the pipeline continue" keys="mod+enter">
            <Button
              variant="success"
              size="sm"
              onClick={() => approve.mutate()}
              disabled={approve.isPending}
            >
              <Check className="h-3.5 w-3.5" />
              Approve {openGate.gate}
            </Button>
          </Tooltip>
          <Button variant="outline" size="sm" onClick={() => setRejecting(true)}>
            <X className="h-3.5 w-3.5" />
            Reject
          </Button>
        </>
      )}
      {canStart && (
        <Button
          variant="primary"
          size="sm"
          onClick={() => start.mutate()}
          disabled={start.isPending}
        >
          <Play className="h-3.5 w-3.5" />
          {video.state === 'draft' ? 'Start' : 'Resume'}
        </Button>
      )}
      {canCancel && (
        <Button
          variant="ghost"
          size="sm"
          onClick={() => cancel.mutate()}
          disabled={cancel.isPending}
        >
          <Ban className="h-3.5 w-3.5" />
          Cancel
        </Button>
      )}

      <Modal
        open={rejecting}
        onOpenChange={setRejecting}
        title={`Reject the ${openGate?.gate ?? ''} gate`}
        description="The task is marked failed and keeps its reason. Retry it once the input is fixed."
        footer={
          <>
            <Button variant="ghost" onClick={() => setRejecting(false)}>
              Cancel
            </Button>
            <Button variant="danger" onClick={() => reject.mutate()} disabled={reject.isPending}>
              Reject
            </Button>
          </>
        }
      >
        <Textarea
          rows={4}
          value={reason}
          onChange={(e) => setReason(e.target.value)}
          placeholder="The outline repeats itself from chapter 12 onwards."
          aria-label="Rejection reason"
          autoFocus
        />
        {reject.isError && <ErrorNotice error={reject.error} className="mt-3" />}
      </Modal>
    </>
  )
}

/**
 * The artifacts a video owns outside any chapter. Built here rather than read
 * from the assets endpoint so the gate banner and the overview can offer a
 * preview before that list has been fetched.
 */
function videoLevelItems(video: Video): ViewerItem[] {
  const items: ViewerItem[] = []
  if (video.blueprintAssetId) {
    items.push({
      id: video.blueprintAssetId,
      kind: 'blueprint',
      mime: kindMime('blueprint'),
      title: kindTitle('blueprint'),
      subtitle: video.ref,
    })
  }
  if (video.finalAssetId) {
    items.push({
      id: video.finalAssetId,
      kind: 'final',
      mime: kindMime('final'),
      title: kindTitle('final'),
      subtitle: `${video.ref} · ${video.title}`,
    })
  }
  return items
}

function GateBanner({ video, task }: { video: Video; task: Task }) {
  const openViewer = useAssetViewer()
  const items = videoLevelItems(video)
  const blueprint = video.blueprintAssetId
  const openAt = (id: string) =>
    openViewer(
      items,
      items.findIndex((item) => item.id === id),
    )
  return (
    <div className="flex shrink-0 items-start gap-3 border-b border-[hsl(var(--warning)/0.35)] bg-[hsl(var(--warning-soft))] px-4 py-2.5">
      <Badge tone="warning" dot pulse className="mt-[1px] shrink-0">
        Gate open
      </Badge>
      <div className="min-w-0 flex-1 text-[12px] text-fg">
        <p>
          Paused for review after <strong>{taskLabel(task.kind)}</strong>. Nothing downstream is
          running and no resources are held — approving is a single row update, so this can wait as
          long as you need.
        </p>
        <div className="mt-1.5 flex flex-wrap gap-3 text-[11.5px]">
          {task.gate === 'blueprint' && blueprint && (
            <button
              type="button"
              className="text-[hsl(var(--accent))] underline-offset-2 hover:underline"
              onClick={() => openAt(blueprint)}
            >
              Review the blueprint
            </button>
          )}
          {task.gate === 'upload' && video.finalAssetId && (
            <button
              type="button"
              className="text-[hsl(var(--accent))] underline-offset-2 hover:underline"
              onClick={() => openAt(video.finalAssetId ?? '')}
            >
              Preview the final render
            </button>
          )}
        </div>
      </div>
    </div>
  )
}

/* ------------------------------------------------------------------ overview */

function Overview({ video, tasks, loading }: { video: Video; tasks: Task[]; loading: boolean }) {
  const openViewer = useAssetViewer()
  const items = videoLevelItems(video)
  const openAt = (id: string) =>
    openViewer(
      items,
      items.findIndex((item) => item.id === id),
    )

  return (
    <div className="h-full overflow-y-auto p-4">
      <div className="mx-auto max-w-5xl space-y-4">
        <section>
          <h2 className="mb-2 text-[11px] font-semibold uppercase tracking-wider text-subtle">
            Pipeline
          </h2>
          {loading ? <Skeleton className="h-16" /> : <StageStrip tasks={tasks} />}
          {!loading && tasks.length === 0 && (
            <p className="text-[12px] text-muted">
              Nothing is enqueued yet. Start the video to build its DAG.
            </p>
          )}
        </section>

        <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_300px]">
          <div className="space-y-4">
            {video.finalAssetId && (
              <Panel className="overflow-hidden">
                <PanelHeader>
                  <PanelTitle>Final render</PanelTitle>
                  <div className="flex items-center gap-1">
                    <Tooltip label="Open the full-size player">
                      <Button
                        size="xs"
                        variant="ghost"
                        onClick={() => openAt(video.finalAssetId ?? '')}
                      >
                        <Maximize2 className="h-3 w-3" />
                        Expand
                      </Button>
                    </Tooltip>
                    <Button size="xs" variant="ghost" asChild>
                      <a href={assetUrl(video.finalAssetId)} download>
                        <Download className="h-3 w-3" />
                        Download
                      </a>
                    </Button>
                  </div>
                </PanelHeader>
                {/* eslint-disable-next-line jsx-a11y/media-has-caption */}
                <video
                  controls
                  preload="metadata"
                  src={assetUrl(video.finalAssetId)}
                  className="max-h-[46vh] w-full bg-black"
                />
              </Panel>
            )}

            {video.topic && (
              <Panel>
                <PanelHeader>
                  <PanelTitle>Topic</PanelTitle>
                </PanelHeader>
                <p className="px-3 py-2.5 text-[12.5px] leading-relaxed text-muted">
                  {video.topic}
                </p>
              </Panel>
            )}

            {video.metadata && (
              <Panel>
                <PanelHeader>
                  <PanelTitle>Publish metadata</PanelTitle>
                  <Badge tone="neutral">{video.metadata.privacy}</Badge>
                </PanelHeader>
                <div className="space-y-2 px-3 py-2.5 text-[12px]">
                  <p className="font-medium text-fg">{video.metadata.title}</p>
                  <p className="whitespace-pre-wrap text-[11.5px] leading-relaxed text-muted">
                    {video.metadata.description}
                  </p>
                  <div className="flex flex-wrap gap-1 pt-0.5">
                    {video.metadata.tags.map((tag) => (
                      <Badge key={tag} tone="violet">
                        {tag}
                      </Badge>
                    ))}
                  </div>
                </div>
              </Panel>
            )}
          </div>

          <div className="space-y-4">
            <Panel>
              <PanelHeader>
                <PanelTitle>Tasks</PanelTitle>
              </PanelHeader>
              <div className="grid grid-cols-2 gap-x-4 gap-y-1 px-3 py-2.5">
                <Stat label="Succeeded" value={video.counts.succeeded} tone="success" />
                <Stat label="Running" value={video.counts.running} tone="accent" />
                <Stat label="Ready" value={video.counts.ready} tone="info" />
                <Stat label="Blocked" value={video.counts.blocked} tone="muted" />
                <Stat label="Gated" value={video.counts.awaitingApproval} tone="warning" />
                <Stat label="Failed" value={video.counts.failed} tone="danger" />
              </div>
            </Panel>

            <Panel>
              <PanelHeader>
                <PanelTitle>Video</PanelTitle>
                {video.blueprintAssetId && (
                  <Tooltip label="Read the blueprint without leaving the app">
                    <Button
                      size="xs"
                      variant="ghost"
                      onClick={() => openAt(video.blueprintAssetId ?? '')}
                    >
                      <FileJson className="h-3 w-3" />
                      Blueprint
                    </Button>
                  </Tooltip>
                )}
              </PanelHeader>
              <dl className="px-3 py-2">
                <KeyValue label="Ref">{video.ref}</KeyValue>
                <KeyValue label="Chapters">{video.chapterCount}</KeyValue>
                <KeyValue label="Stills / chapter">{video.imagesPerChapter}</KeyValue>
                <KeyValue label="Created">{formatAbsolute(video.createdAt)}</KeyValue>
                <KeyValue label="Started">{formatAbsolute(video.startedAt)}</KeyValue>
                <KeyValue label="Completed">{formatAbsolute(video.completedAt)}</KeyValue>
              </dl>
            </Panel>

            {video.upload && (
              <Panel>
                <PanelHeader>
                  <PanelTitle>Upload</PanelTitle>
                  {video.upload.dryRun && <Badge tone="warning">dry run</Badge>}
                </PanelHeader>
                <dl className="px-3 py-2">
                  <KeyValue label="Remote id">{video.upload.remoteVideoId}</KeyValue>
                  <KeyValue label="Uploaded">{formatAbsolute(video.upload.uploadedAt)}</KeyValue>
                </dl>
              </Panel>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

function Stat({
  label,
  value,
  tone,
}: {
  label: string
  value: number
  tone: 'accent' | 'info' | 'success' | 'warning' | 'danger' | 'muted'
}) {
  const colour = {
    accent: 'text-[hsl(var(--accent))]',
    info: 'text-[hsl(var(--info))]',
    success: 'text-[hsl(var(--success))]',
    warning: 'text-[hsl(var(--warning))]',
    danger: 'text-[hsl(var(--danger))]',
    muted: 'text-muted',
  }[tone]
  return (
    <div className="flex items-baseline justify-between gap-2">
      <span className="text-[11.5px] text-subtle">{label}</span>
      <span
        className={cn('tabular text-[15px] font-semibold', value === 0 ? 'text-subtle' : colour)}
      >
        {value}
      </span>
    </div>
  )
}

/* -------------------------------------------------------------- chapter grid */

const CHAPTER_ROW_HEIGHT = 148

function ChapterGrid({
  video,
  chapters,
  tasks,
  loading,
}: {
  video: Video
  chapters: Chapter[]
  tasks: Task[]
  loading: boolean
}) {
  const parentRef = useRef<HTMLDivElement>(null)

  const tasksByOrdinal = useMemo(() => {
    const map = new Map<number, Task[]>()
    for (const task of tasks) {
      if (task.ordinal < 1) continue
      const list = map.get(task.ordinal)
      if (list) list.push(task)
      else map.set(task.ordinal, [task])
    }
    return map
  }, [tasks])

  const virtualizer = useVirtualizer({
    count: chapters.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => CHAPTER_ROW_HEIGHT,
    overscan: 6,
  })

  if (loading) {
    return (
      <div className="space-y-2 p-4">
        {Array.from({ length: 4 }, (_, i) => (
          <Skeleton key={i} className="h-32 w-full" />
        ))}
      </div>
    )
  }
  if (chapters.length === 0) {
    return (
      <EmptyState
        title="No chapters yet"
        description="Chapters appear as soon as the blueprint has been generated."
      />
    )
  }

  return (
    <div ref={parentRef} className="h-full overflow-y-auto px-4 py-3">
      <div className="relative w-full" style={{ height: virtualizer.getTotalSize() }}>
        {virtualizer.getVirtualItems().map((item) => {
          const chapter = chapters[item.index]
          if (!chapter) return null
          return (
            <div
              key={chapter.id}
              ref={virtualizer.measureElement}
              data-index={item.index}
              className="absolute left-0 top-0 w-full pb-2"
              style={{ transform: `translateY(${item.start}px)` }}
            >
              <ChapterCard
                chapter={chapter}
                videoRef={video.ref}
                videoId={video.id}
                imagesPerChapter={video.imagesPerChapter}
                tasks={tasksByOrdinal.get(chapter.ordinal) ?? []}
              />
            </div>
          )
        })}
      </div>
    </div>
  )
}

/**
 * One chapter: its stills side by side, its narration, and the tasks that
 * produced them. Memoised on the chapter and its task slice, so a delta for
 * chapter 7 never re-renders chapter 8 (§9).
 */
const ChapterCard = memo(function ChapterCard({
  chapter,
  videoRef,
  videoId,
  imagesPerChapter,
  tasks,
}: {
  chapter: Chapter
  videoRef: string
  videoId: string
  imagesPerChapter: number
  tasks: Task[]
}) {
  const queryClient = useQueryClient()
  const openViewer = useAssetViewer()
  const [expanded, setExpanded] = useState(false)
  const [editing, setEditing] = useState(false)
  const [rerunning, setRerunning] = useState(false)
  const [draft, setDraft] = useState(chapter.script)

  // The whole chapter, viewable as one set: opening a still and pressing → walks
  // its siblings, then the narration, then the clip.
  const items = useMemo(() => chapterStillItems(chapter, videoRef), [chapter, videoRef])
  const openAt = useCallback(
    (id: string) =>
      openViewer(
        items,
        items.findIndex((item) => item.id === id),
      ),
    [items, openViewer],
  )

  const save = useMutation({
    mutationFn: () => api.updateChapterScript(chapter.id, draft),
    onSuccess: (updated) => {
      queryClient.setQueryData<Chapter[]>(qk.chapters(videoId), (prev) =>
        prev?.map((c) => (c.id === updated.id ? { ...c, ...updated } : c)),
      )
      setEditing(false)
    },
  })
  const retry = useMutation({
    mutationFn: () => api.retryChapter(videoRef, chapter.ordinal),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: qk.videoTasks(videoId) })
      void queryClient.invalidateQueries({ queryKey: qk.video(videoRef) })
    },
  })

  // The two meanings of "run this again", told apart by what is actually in
  // the chapter. If something failed, nothing below it ever ran and cascading
  // is free. If everything succeeded, re-running throws away work that may
  // have been reviewed — so that goes through the confirmation instead.
  const failed = tasks.some((t) => t.state === 'failed')
  const chapterStale = tasks.some((t) => t.stale)
  const rerunSeeds = useMemo(
    () => tasks.filter((t) => t.state === 'succeeded').map((t) => t.id),
    [tasks],
  )

  const startEdit = useCallback(() => {
    setDraft(chapter.script)
    setEditing(true)
    setExpanded(true)
  }, [chapter.script])

  const stills = Array.from({ length: imagesPerChapter }, (_, i) => chapter.imageAssetIds[i] ?? '')

  return (
    <Panel className="overflow-hidden transition-colors hover:border-[hsl(var(--border-strong))]">
      <div className="flex gap-3 p-3">
        {/* Stills, side by side — this is what the operator actually reviews. */}
        <div className="flex shrink-0 gap-2">
          {stills.map((id, i) => (
            <Still
              key={i}
              id={id}
              slot={i}
              prompt={chapter.imagePrompts[i]}
              alt={`Chapter ${chapter.ordinal}, still ${i + 1}`}
              onOpen={() => openAt(id)}
            />
          ))}
        </div>

        <div className="flex min-w-0 flex-1 flex-col">
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <div className="flex items-center gap-2">
                <Mono className="text-subtle">{chapterKey(videoRef, chapter.ordinal)}</Mono>
                <span className="truncate text-[13px] font-medium text-fg">{chapter.title}</span>
                {chapterStale && <StaleDot />}
              </div>
              <p className="mt-0.5 line-clamp-1 text-[11.5px] text-subtle">{chapter.summary}</p>
            </div>
            <div className="flex shrink-0 items-center gap-1">
              <Tooltip
                label={
                  failed
                    ? 'Retry the failed work in this chapter'
                    : 'Run this chapter again — you will be shown what else it affects'
                }
              >
                <Button
                  size="icon"
                  variant="ghost"
                  onClick={() => (failed ? retry.mutate() : setRerunning(true))}
                  disabled={retry.isPending || (!failed && rerunSeeds.length === 0)}
                  aria-label={`Re-run chapter ${chapter.ordinal}`}
                >
                  <RefreshCw className={cn('h-3.5 w-3.5', retry.isPending && 'animate-spin')} />
                </Button>
              </Tooltip>
              <Tooltip label="Edit the script">
                <Button
                  size="icon"
                  variant="ghost"
                  onClick={startEdit}
                  aria-label={`Edit script of chapter ${chapter.ordinal}`}
                >
                  <Pencil className="h-3.5 w-3.5" />
                </Button>
              </Tooltip>
            </div>
          </div>

          <div className="mt-2 flex flex-wrap items-center gap-1.5">
            {tasks.map((task) => (
              <ChapterTaskChip key={task.id} task={task} />
            ))}
          </div>

          <div className="mt-auto flex items-center gap-3 pt-2 text-[11.5px] text-subtle">
            {chapter.audioAssetId ? (
              <audio
                controls
                preload="none"
                src={assetUrl(chapter.audioAssetId)}
                className="h-7 max-w-[240px]"
                aria-label={`Narration of chapter ${chapter.ordinal}`}
              />
            ) : (
              <span>No narration yet</span>
            )}
            {chapter.durationSeconds > 0 && (
              <span className="tabular">≈ {formatClock(chapter.durationSeconds)}</span>
            )}
            {chapter.clipAssetId && (
              <button
                type="button"
                className="text-[hsl(var(--accent))] underline-offset-2 hover:underline"
                onClick={() => openAt(chapter.clipAssetId ?? '')}
              >
                clip
              </button>
            )}
            <button
              type="button"
              className="ml-auto flex items-center gap-1 text-subtle hover:text-fg"
              onClick={() => setExpanded((prev) => !prev)}
              aria-expanded={expanded}
            >
              {expanded ? (
                <ChevronDown className="h-3 w-3" />
              ) : (
                <ChevronRight className="h-3 w-3" />
              )}
              script &amp; prompts
            </button>
          </div>
        </div>
      </div>

      <RerunDialog
        open={rerunning}
        onOpenChange={setRerunning}
        videoRef={videoRef}
        videoId={videoId}
        taskIds={rerunSeeds}
        what={`chapter ${chapter.ordinal}`}
      />

      {expanded && (
        <div className="border-t border-[hsl(var(--border))] bg-subtle px-3 py-3">
          {editing ? (
            <div className="space-y-2">
              <Textarea
                rows={10}
                value={draft}
                onChange={(e) => setDraft(e.target.value)}
                aria-label={`Script of chapter ${chapter.ordinal}`}
              />
              <div className="flex items-center gap-2">
                <Button
                  variant="primary"
                  size="sm"
                  onClick={() => save.mutate()}
                  disabled={save.isPending || !draft.trim()}
                >
                  Save script
                </Button>
                <Button variant="ghost" size="sm" onClick={() => setEditing(false)}>
                  Cancel
                </Button>
                <span className="text-[11.5px] text-subtle">
                  Saving only stores the text — re-run the chapter to regenerate its narration.
                </span>
              </div>
              {save.isError && <ErrorNotice error={save.error} />}
            </div>
          ) : (
            <>
              <p className="whitespace-pre-wrap font-mono text-[12px] leading-relaxed text-muted">
                {chapter.script || 'The script has not been generated yet.'}
              </p>
              {chapter.imagePrompts.length > 0 && (
                <ul className="mt-3 space-y-1 border-t border-[hsl(var(--border))] pt-2">
                  {chapter.imagePrompts.map((prompt, i) => (
                    <li key={i} className="text-[11.5px] text-subtle">
                      <span className="text-[hsl(var(--violet))]">prompt {i + 1}</span> — {prompt}
                    </li>
                  ))}
                </ul>
              )}
            </>
          )}
        </div>
      )}
    </Panel>
  )
})

/**
 * One still in the chapter grid. It opens the viewer rather than a browser tab:
 * the prompt that produced the image is the first thing an operator wants
 * beside it, and a raw `/assets/…` tab has nothing but pixels.
 */
function Still({
  id,
  slot,
  prompt,
  alt,
  onOpen,
}: {
  id: string
  slot: number
  prompt: string | undefined
  alt: string
  onOpen: () => void
}) {
  if (!id) {
    return (
      <div className="flex h-[86px] w-[152px] flex-col items-center justify-center gap-1 rounded-[var(--radius-sm)] border border-dashed border-[hsl(var(--border-strong))] bg-[hsl(var(--bg-subtle))] text-[11px] text-subtle">
        <span className="tabular text-[10px] uppercase tracking-wider">still {slot + 1}</span>
        <span>pending</span>
      </div>
    )
  }

  const preview = (
    <button
      type="button"
      onClick={onOpen}
      aria-label={`${alt} — open the preview`}
      className="group relative block h-[86px] w-[152px] overflow-hidden rounded-[var(--radius-sm)] border border-[hsl(var(--border))] transition-[border-color,box-shadow] hover:border-[hsl(var(--accent))] hover:elev-2"
    >
      <AssetPreview item={{ id, kind: 'image', mime: kindMime('image'), title: alt }} />
      <span
        className="absolute inset-0 flex items-center justify-center bg-black/45 opacity-0 transition-opacity group-hover:opacity-100"
        aria-hidden
      >
        <Maximize2 className="h-4 w-4 text-white" />
      </span>
      <span
        className="tabular absolute left-1 top-1 rounded-[var(--radius-xs)] bg-black/55 px-1 text-[10px] font-medium leading-[15px] text-white opacity-0 transition-opacity group-hover:opacity-100"
        aria-hidden
      >
        {slot + 1}
      </span>
    </button>
  )

  return prompt ? (
    <Tooltip label={<span className="line-clamp-4 text-[11px]">{prompt}</span>} side="bottom">
      {preview}
    </Tooltip>
  ) : (
    preview
  )
}

const ChapterTaskChip = memo(function ChapterTaskChip({ task }: { task: Task }) {
  const tone =
    task.state === 'succeeded'
      ? 'success'
      : task.state === 'running'
        ? 'accent'
        : task.state === 'failed'
          ? 'danger'
          : task.state === 'ready'
            ? 'info'
            : 'neutral'
  const label = task.index >= 0 ? `${taskLabel(task.kind)} ${task.index + 1}` : taskLabel(task.kind)
  // Staleness outranks the state colour here: a succeeded-but-stale chip that
  // still reads green is the one thing this whole feature exists to prevent.
  return (
    <Tooltip
      label={
        task.stale
          ? `${label} — done, but an input changed after it ran`
          : task.error || `${label} — ${task.state} (attempt ${task.attempt})`
      }
    >
      <span>
        <Badge tone={task.stale ? 'warning' : tone} dot pulse={task.state === 'running'}>
          {label}
          {task.stale && <span className="opacity-70">· stale</span>}
        </Badge>
      </span>
    </Tooltip>
  )
})

/* ----------------------------------------------------------------- task list */

function TaskList({ tasks, loading }: { tasks: Task[]; loading: boolean }) {
  const queryClient = useQueryClient()
  const retry = useMutation({
    mutationFn: (id: string) => api.retryTask(id),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ['video'] }),
  })

  if (loading) return <Skeleton className="m-4 h-64" />
  if (tasks.length === 0)
    return <EmptyState title="No tasks" description="Start the video first." />

  return (
    <div className="h-full overflow-y-auto">
      <table className="w-full border-collapse text-[12px]">
        <thead className="sticky top-0 z-10 bg-subtle text-[11px] uppercase tracking-wide text-subtle">
          <tr className="border-b border-[hsl(var(--border))]">
            <th className="px-4 py-1.5 text-left font-semibold">Task</th>
            <th className="px-2 py-1.5 text-left font-semibold">Chapter</th>
            <th className="px-2 py-1.5 text-left font-semibold">Pool</th>
            <th className="px-2 py-1.5 text-left font-semibold">State</th>
            <th className="px-2 py-1.5 text-right font-semibold">Attempt</th>
            <th className="px-2 py-1.5 text-left font-semibold">Error</th>
            <th className="px-4 py-1.5" />
          </tr>
        </thead>
        <tbody>
          {tasks.map((task) => (
            <TaskRow key={task.id} task={task} onRetry={() => retry.mutate(task.id)} />
          ))}
        </tbody>
      </table>
    </div>
  )
}

const TaskRow = memo(function TaskRow({ task, onRetry }: { task: Task; onRetry: () => void }) {
  return (
    <tr className="border-b border-[hsl(var(--border))] hover:bg-[hsl(var(--bg-hover))]">
      <td className="px-4 py-1.5">
        {taskLabel(task.kind)}
        {task.index >= 0 && <span className="text-subtle"> #{task.index + 1}</span>}
        {task.gate && (
          <Badge tone="warning" className="ml-2">
            gate
          </Badge>
        )}
        {task.stale && <StaleBadge className="ml-2" />}
      </td>
      <td className="px-2 py-1.5 tabular text-muted">{task.ordinal > 0 ? task.ordinal : '—'}</td>
      <td className="px-2 py-1.5 text-muted">{task.pool}</td>
      <td className="px-2 py-1.5">
        <TaskStateBadge state={task.state} />
      </td>
      <td className="px-2 py-1.5 text-right tabular text-muted">
        {task.attempt}/{task.maxAttempts}
      </td>
      <td className="max-w-[280px] truncate px-2 py-1.5 text-[hsl(var(--danger))]">
        {task.error ?? ''}
      </td>
      <td className="px-4 py-1.5 text-right">
        <Button size="xs" variant="ghost" onClick={onRetry}>
          Retry
        </Button>
      </td>
    </tr>
  )
})

/* ------------------------------------------------------------------ artifacts */

type GalleryView = 'grid' | 'list'

/**
 * Every artifact a video has produced, as a gallery rather than a list of
 * hashes. A hash is the right *identity* for a content-addressed store and the
 * wrong thing to show an operator who is asking whether the images came out
 * well — so the image leads, and the address moves into the viewer.
 */
function ArtifactGallery({ video, chapters }: { video: Video; chapters: Chapter[] }) {
  const openViewer = useAssetViewer()
  const [kind, setKind] = useState<string>('all')
  const [view, setView] = useState<GalleryView>('grid')

  const assets = useQuery({
    queryKey: qk.assets(video.id),
    queryFn: () => api.listAssets(video.ref),
  })

  const items = useMemo(
    () => videoAssetItems(assets.data ?? [], chapters, video.ref),
    [assets.data, chapters, video.ref],
  )

  const kinds = useMemo(() => {
    const counts = new Map<string, number>()
    for (const item of items) counts.set(item.kind, (counts.get(item.kind) ?? 0) + 1)
    return [...counts.entries()].sort((a, b) => a[0].localeCompare(b[0]))
  }, [items])

  // A filter that outlives the artifact it matched leaves an empty pane with no
  // explanation, so it collapses back to "all" the moment its kind disappears.
  const active = kind !== 'all' && kinds.some(([name]) => name === kind) ? kind : 'all'
  const shown = useMemo(
    () => (active === 'all' ? items : items.filter((item) => item.kind === active)),
    [items, active],
  )
  const shownBytes = shown.reduce((sum, item) => sum + (item.size ?? 0), 0)

  // Stable across renders of the same filtered set, so the memoised cards below
  // are not thrown away every time the gallery re-renders behind an SSE delta.
  const openAt = useCallback((index: number) => openViewer(shown, index), [openViewer, shown])

  if (assets.isPending) {
    return (
      <div className="grid gap-3 p-4 [grid-template-columns:repeat(auto-fill,minmax(180px,1fr))]">
        {Array.from({ length: 8 }, (_, i) => (
          <Skeleton key={i} className="h-[168px]" />
        ))}
      </div>
    )
  }

  if (items.length === 0) {
    return (
      <EmptyState
        title="No artifacts yet"
        description={`Artifacts appear as tasks complete. ${video.ref} has produced none so far.`}
      />
    )
  }

  return (
    <div className="flex h-full flex-col">
      <div className="flex shrink-0 flex-wrap items-center gap-2 border-b border-[hsl(var(--border))] bg-subtle px-3 py-2 no-select">
        <FilterChip
          label="All"
          count={items.length}
          selected={active === 'all'}
          onClick={() => setKind('all')}
        />
        {kinds.map(([name, count]) => (
          <FilterChip
            key={name}
            label={kindTitle(name)}
            count={count}
            tone={assetKindTone(name)}
            selected={active === name}
            onClick={() => setKind(name)}
          />
        ))}

        <span className="tabular ml-auto text-[11px] text-subtle">
          {shown.length} shown · {formatBytes(shownBytes)}
        </span>
        <Segmented
          aria-label="Artifact layout"
          value={view}
          onChange={setView}
          options={[
            {
              value: 'grid',
              label: (
                <>
                  <LayoutGrid className="h-3.5 w-3.5" />
                  <span className="sr-only">Grid</span>
                </>
              ),
            },
            {
              value: 'list',
              label: (
                <>
                  <Rows3 className="h-3.5 w-3.5" />
                  <span className="sr-only">List</span>
                </>
              ),
            },
          ]}
          className="w-[72px]"
        />
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto p-3">
        {view === 'grid' ? (
          <div className="grid gap-3 [grid-template-columns:repeat(auto-fill,minmax(180px,1fr))]">
            {shown.map((item, index) => (
              <ArtifactCard key={item.id + index} item={item} index={index} onOpen={openAt} />
            ))}
          </div>
        ) : (
          <Panel className="overflow-hidden">
            <ul className="divide-y divide-[hsl(var(--border))]">
              {shown.map((item, index) => (
                <ArtifactRow key={item.id + index} item={item} index={index} onOpen={openAt} />
              ))}
            </ul>
          </Panel>
        )}
      </div>
    </div>
  )
}

function FilterChip({
  label,
  count,
  tone = 'neutral',
  selected,
  onClick,
}: {
  label: string
  count: number
  tone?: Tone
  selected: boolean
  onClick: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={selected}
      className={cn(
        'inline-flex items-center gap-1.5 rounded-full border px-2 py-[1px] text-[11px] font-medium leading-[18px] transition-colors',
        selected
          ? 'border-[hsl(var(--accent))] bg-[hsl(var(--accent))] text-[hsl(var(--accent-fg))]'
          : 'border-[hsl(var(--border))] bg-[hsl(var(--bg-elevated))] text-muted hover:border-[hsl(var(--border-strong))] hover:text-fg',
      )}
    >
      {!selected && tone !== 'neutral' && (
        <span className={cn('h-1.5 w-1.5 rounded-full', TONE_DOT[tone])} aria-hidden />
      )}
      {label}
      <span className={cn('tabular', selected ? 'opacity-80' : 'text-subtle')}>{count}</span>
    </button>
  )
}

const TONE_DOT: Record<Tone, string> = {
  neutral: 'bg-[hsl(var(--fg-subtle))]',
  accent: 'bg-[hsl(var(--accent))]',
  success: 'bg-[hsl(var(--success))]',
  warning: 'bg-[hsl(var(--warning))]',
  danger: 'bg-[hsl(var(--danger))]',
  info: 'bg-[hsl(var(--info))]',
  violet: 'bg-[hsl(var(--violet))]',
}

const ArtifactCard = memo(function ArtifactCard({
  item,
  index,
  onOpen,
}: {
  item: ViewerItem
  index: number
  onOpen: (index: number) => void
}) {
  const media = mediaTypeOf(item.mime)
  return (
    <button
      type="button"
      onClick={() => onOpen(index)}
      aria-label={`Preview ${item.title}`}
      className="group surface overflow-hidden text-left transition-[border-color,box-shadow] hover:border-[hsl(var(--accent))] hover:elev-2"
    >
      <div className="relative aspect-[16/10] overflow-hidden border-b border-[hsl(var(--border))]">
        <AssetPreview item={item} />
        <span
          className="absolute inset-0 flex items-center justify-center bg-black/45 opacity-0 transition-opacity group-hover:opacity-100"
          aria-hidden
        >
          <Maximize2 className="h-4 w-4 text-white" />
        </span>
        {media !== 'image' && (
          <span className="absolute bottom-1 right-1 rounded-[var(--radius-xs)] bg-black/55 px-1 text-[10px] font-medium leading-[15px] text-white">
            {media}
          </span>
        )}
      </div>
      <div className="px-2.5 py-2">
        <p className="truncate text-[12px] font-medium text-fg">{item.title}</p>
        <p className="truncate text-[11px] text-subtle">{item.subtitle}</p>
        <div className="mt-1 flex items-center justify-between gap-2 text-[10.5px] text-subtle">
          <Mono className="truncate text-[10.5px]">{shortId(item.id)}</Mono>
          <span className="tabular shrink-0">{formatBytes(item.size ?? 0)}</span>
        </div>
      </div>
    </button>
  )
})

const ArtifactRow = memo(function ArtifactRow({
  item,
  index,
  onOpen,
}: {
  item: ViewerItem
  index: number
  onOpen: (index: number) => void
}) {
  return (
    <li className="flex items-center gap-3 px-2.5 py-1.5 text-[12px] hover:bg-[hsl(var(--bg-hover))]">
      <button
        type="button"
        onClick={() => onOpen(index)}
        aria-label={`Preview ${item.title}`}
        className="flex min-w-0 flex-1 items-center gap-3 text-left"
      >
        <span className="h-8 w-14 shrink-0 overflow-hidden rounded-[var(--radius-xs)] border border-[hsl(var(--border))]">
          <AssetPreview item={item} />
        </span>
        <span className="min-w-0 flex-1">
          <span className="block truncate font-medium text-fg">{item.title}</span>
          <span className="block truncate text-[11px] text-subtle">{item.subtitle}</span>
        </span>
        <Badge tone={assetKindTone(item.kind)} className="shrink-0">
          {item.kind}
        </Badge>
        <span className="hidden w-28 shrink-0 truncate text-[11px] text-subtle md:block">
          {item.mime}
        </span>
        <span className="tabular w-16 shrink-0 text-right text-muted">
          {formatBytes(item.size ?? 0)}
        </span>
        <span className="tabular hidden w-20 shrink-0 text-right text-[11px] text-subtle lg:block">
          {formatRelative(item.createdAt)}
        </span>
      </button>
      <Tooltip label="Download">
        <a
          href={assetUrl(item.id)}
          download={downloadName(item)}
          className="shrink-0 rounded-[var(--radius-xs)] p-1 text-subtle hover:bg-[hsl(var(--bg-hover))] hover:text-fg"
          aria-label={`Download ${item.title}`}
        >
          <Download className="h-3.5 w-3.5" />
        </a>
      </Tooltip>
    </li>
  )
})
