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
  Maximize2,
  Pencil,
  Play,
  RefreshCw,
  X,
} from 'lucide-react'
import { memo, useCallback, useMemo, useRef, useState } from 'react'

import { ArtifactGallery } from '@/components/artifact-gallery'
import { AssetPreview, AssetViewerProvider, useAssetViewer } from '@/components/asset-viewer'
import { StageStrip } from '@/components/stage-strip'
import { RerunDialog, StaleBanner, StaleDot } from '@/components/stale'
import { TaskStateDot, VideoStateBadge } from '@/components/state-badges'
import { TaskTable } from '@/components/task-table'
import { Badge } from '@/components/ui/badge'
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
  artifactKindFor,
  chapterSlideItems,
  kindMime,
  kindTitle,
  producingTask,
  producingTaskId,
  thumbnailCellItems,
  videoAssetItems,
} from '@/lib/assets'
import type { ViewerItem } from '@/lib/assets'
import {
  chapterKey,
  formatAbsolute,
  formatClock,
  formatRelative,
  percent,
  taskLabel,
} from '@/lib/format'
import { useHotkeys } from '@/lib/hotkeys'
import type { Chapter, GateKind, Task, TaskKind, Video } from '@/lib/types'
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
  // Fetched once here rather than by each tab that wants it: the overview, the
  // task inspector and the gallery all ask the same question, and the count in
  // the tab strip has to be answered before any of them is opened.
  const assets = useQuery({
    queryKey: qk.assets(videoId ?? ''),
    queryFn: () => api.listAssets(ref),
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
    [assets.data, chapters.data, video.data, tasks.data],
  )

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
    <AssetViewerProvider videoRef={v.ref} videoId={v.id}>
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

      {openGate && (
        <GateBanner video={v} task={openGate} chapterCount={chapters.data?.length ?? 0} />
      )}
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
            { value: 'artifacts', label: 'Artifacts', count: artifacts.length },
          ]}
          className="w-[400px]"
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
          <Overview
            video={v}
            tasks={tasks.data ?? []}
            chapters={chapters.data ?? []}
            artifacts={artifacts}
            loading={tasks.isPending}
          />
        )}
        {tab === 'chapters' && (
          <ChapterGrid
            video={v}
            chapters={chapters.data ?? []}
            tasks={tasks.data ?? []}
            loading={chapters.isPending}
          />
        )}
        {/* Keyed on the video: a selection, a cursor and a folded group belong to
            the video they were made in, and the route component itself is reused
            when the sidebar moves to the next one. */}
        {tab === 'tasks' && (
          <TaskTable
            key={v.id}
            video={v}
            tasks={tasks.data ?? []}
            chapters={chapters.data ?? []}
            artifacts={artifacts}
            loading={tasks.isPending}
          />
        )}
        {tab === 'artifacts' && (
          <ArtifactGallery key={v.id} video={v} items={artifacts} loading={assets.isPending} />
        )}
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
function videoLevelItems(video: Video, tasks: Task[] = []): ViewerItem[] {
  const items: ViewerItem[] = []
  if (video.blueprintAssetId) {
    items.push({
      id: video.blueprintAssetId,
      kind: 'blueprint',
      mime: kindMime('blueprint'),
      title: kindTitle('blueprint'),
      subtitle: video.ref,
      taskId: producingTaskId(tasks, 'blueprint', -1, -1),
    })
  }
  if (video.finalAssetId) {
    items.push({
      id: video.finalAssetId,
      kind: 'final',
      mime: kindMime('final'),
      title: kindTitle('final'),
      subtitle: `${video.ref} · ${video.title}`,
      taskId: producingTaskId(tasks, 'final', -1, -1),
    })
  }
  if (video.thumbnailAssetId) {
    items.push({
      id: video.thumbnailAssetId,
      kind: 'thumbnail',
      mime: kindMime('thumbnail'),
      title: kindTitle('thumbnail'),
      subtitle: video.metadata?.thumbnailText ?? video.ref,
      taskId: producingTaskId(tasks, 'thumbnail', -1, -1),
    })
  }
  return items
}

function GateBanner({
  video,
  task,
  chapterCount,
}: {
  video: Video
  task: Task
  chapterCount: number
}) {
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
        {task.gate === 'blueprint' ? (
          <p>
            Paused for review after <strong>{taskLabel(task.kind)}</strong>. Nothing downstream
            exists yet: approving is what builds the rest of the pipeline, for the{' '}
            <strong>{chapterCount}</strong> chapters below. Nothing is running and no resources are
            held, so this can wait as long as you need.
          </p>
        ) : (
          <p>
            Paused for review after <strong>{taskLabel(task.kind)}</strong>. Nothing downstream is
            running and no resources are held — approving is a single row update, so this can wait
            as long as you need.
          </p>
        )}
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
          {/* The gate sits on the thumbnail so that this link exists: what is
              being approved is the listing, and the thumbnail is most of it. */}
          {task.gate === 'upload' && video.thumbnailAssetId && (
            <button
              type="button"
              className="text-[hsl(var(--accent))] underline-offset-2 hover:underline"
              onClick={() => openAt(video.thumbnailAssetId ?? '')}
            >
              Review the thumbnail
            </button>
          )}
        </div>
      </div>
    </div>
  )
}

/**
 * The thumbnail grid as cells rather than as one composed picture.
 *
 * The composed thumbnail above it says whether the result works; this says which
 * tile is wrong. Each cell opens the viewer on itself, where its prompt is
 * editable — the grid is the only place a cell that failed, and so left no
 * artifact, can be reached at all.
 */
function ThumbnailGrid({ video, tasks }: { video: Video; tasks: Task[] }) {
  const openViewer = useAssetViewer()
  const items = useMemo(() => thumbnailCellItems(video, tasks), [video, tasks])

  if (items.length === 0) return null

  return (
    <Panel>
      <PanelHeader>
        <PanelTitle>Thumbnail grid</PanelTitle>
        <Badge tone="neutral">{items.length} cells</Badge>
      </PanelHeader>
      <div className="grid grid-cols-4 gap-2 px-3 py-2.5">
        {items.map((item, cell) => {
          const task = producingTask(tasks, 'thumbnail_icon', -1, cell)
          return (
            <button
              key={item.id}
              type="button"
              onClick={() => openViewer(items, cell)}
              aria-label={`${item.title} — open to edit its prompt`}
              className="group flex flex-col gap-1 text-left"
            >
              <span
                className={cn(
                  'relative block aspect-square overflow-hidden rounded-[var(--radius-sm)] border transition-colors',
                  item.pending
                    ? 'border-dashed border-[hsl(var(--border-strong))]'
                    : 'border-[hsl(var(--border))] group-hover:border-[hsl(var(--accent))]',
                )}
              >
                <AssetPreview item={item} />
                <TileWorkingMark task={task} />
              </span>
              <span className="truncate text-[10.5px] uppercase tracking-wider text-subtle">
                {video.thumbnailPlan[cell]?.caption || `cell ${cell + 1}`}
              </span>
            </button>
          )
        })}
      </div>
    </Panel>
  )
}

/* ------------------------------------------------------------------ overview */

function Overview({
  video,
  tasks,
  chapters,
  artifacts,
  loading,
}: {
  video: Video
  tasks: Task[]
  chapters: Chapter[]
  artifacts: ViewerItem[]
  loading: boolean
}) {
  const openViewer = useAssetViewer()
  const items = videoLevelItems(video, tasks)
  const openAt = (id: string) =>
    openViewer(
      items,
      items.findIndex((item) => item.id === id),
    )

  // What each stage of the pipeline left behind, so a stage tile can offer to
  // show it. Grouped once here rather than filtered per tile: thirteen tiles
  // scanning the whole asset list on every render is thirteen passes for one
  // answer.
  const artifactsByStage = useMemo(() => {
    const byKind = new Map<TaskKind, ViewerItem[]>()
    for (const task of tasks) {
      if (byKind.has(task.kind)) continue
      const artifact = artifactKindFor(task.kind)
      if (!artifact) continue
      const produced = artifacts.filter((item) => item.kind === artifact)
      if (produced.length > 0) byKind.set(task.kind, produced)
    }
    return byKind
  }, [artifacts, tasks])

  return (
    <div className="h-full overflow-y-auto p-4">
      <div className="mx-auto max-w-5xl space-y-4">
        <section>
          <h2 className="mb-2 text-[11px] font-semibold uppercase tracking-wider text-subtle">
            Pipeline
          </h2>
          {loading ? (
            <Skeleton className="h-16" />
          ) : (
            <StageStrip
              tasks={tasks}
              videoRef={video.ref}
              videoId={video.id}
              artifacts={artifactsByStage}
            />
          )}
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
                  {/* The listing as YouTube will show it: the thumbnail is what
                      the title is read against, not a separate artifact. */}
                  {video.thumbnailAssetId && (
                    <button
                      type="button"
                      className="block w-full overflow-hidden rounded"
                      onClick={() => openAt(video.thumbnailAssetId ?? '')}
                    >
                      <img
                        src={assetUrl(video.thumbnailAssetId)}
                        alt={`Thumbnail for ${video.title}`}
                        className="aspect-video w-full bg-black object-cover"
                      />
                    </button>
                  )}
                  <p className="font-medium text-fg">{video.metadata.title}</p>
                  {/* The hook competes for the same glance the title does, so it
                      reads next to it rather than buried under the tags. */}
                  {video.metadata.thumbnailText && (
                    <p className="font-semibold tracking-wide text-[hsl(var(--accent))]">
                      {video.metadata.thumbnailText}
                    </p>
                  )}
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

            <ThumbnailGrid video={video} tasks={tasks} />
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
                {/*
                  The chapter count a video is created with is a target the
                  blueprint is briefed with, not a promise. Until an outline
                  exists there is no real number to show, and once one does the
                  target is the less interesting of the two.
                */}
                <KeyValue label="Chapters">
                  {chapters.length === 0 ? (
                    <span className="text-muted">~{video.chapterCount} target</span>
                  ) : (
                    <>
                      {chapters.length}
                      {chapters.length !== video.chapterCount && (
                        <span className="ml-1.5 text-subtle">({video.chapterCount} asked for)</span>
                      )}
                    </>
                  )}
                </KeyValue>
                <KeyValue label="Slides / chapter">{video.slidesPerChapter}</KeyValue>
                <KeyValue label="Thumbnail tiles">{video.thumbnailCells}</KeyValue>
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
                slidesPerChapter={video.slidesPerChapter}
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
 * One chapter: its slides side by side, its narration, and the tasks that
 * produced them. Memoised on the chapter and its task slice, so a delta for
 * chapter 7 never re-renders chapter 8.
 */
const ChapterCard = memo(function ChapterCard({
  chapter,
  videoRef,
  videoId,
  slidesPerChapter,
  tasks,
}: {
  chapter: Chapter
  videoRef: string
  videoId: string
  slidesPerChapter: number
  tasks: Task[]
}) {
  const queryClient = useQueryClient()
  const openViewer = useAssetViewer()
  const [expanded, setExpanded] = useState(false)
  const [editing, setEditing] = useState(false)
  const [rerunning, setRerunning] = useState(false)
  const [draft, setDraft] = useState(chapter.script)

  // The whole chapter, viewable as one set: opening a slide and pressing → walks
  // its siblings, then the narration, then the clip.
  const items = useMemo(
    () => chapterSlideItems(chapter, videoRef, tasks),
    [chapter, videoRef, tasks],
  )
  const openAt = useCallback(
    (id: string) =>
      openViewer(
        items,
        items.findIndex((item) => item.id === id),
      ),
    [items, openViewer],
  )
  // Slides open by slot rather than by address: an empty slot has no address,
  // and it is the one whose prompt the operator most wants to get at.
  const openSlide = useCallback(
    (slot: number) =>
      openViewer(
        items,
        items.findIndex((item) => item.kind === 'image' && item.slot === slot),
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
  // The cascade: every task of this chapter and everything downstream of them,
  // reset and run. Reachable only from inside the dialog, once it has said what
  // that sweeps up.
  const retry = useMutation({
    mutationFn: () => api.retryChapter(videoRef, chapter.ordinal),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: qk.videoTasks(videoId) })
      void queryClient.invalidateQueries({ queryKey: qk.video(videoRef) })
      setRerunning(false)
    },
  })

  // One verb for "run this again", whatever state the chapter is in: re-run its
  // own tasks and flag what is below. Rebuilding the rest of the video is the
  // explicit choice inside the dialog.
  const chapterStale = tasks.some((t) => t.stale)
  const rerunSeeds = useMemo(() => tasks.map((t) => t.id), [tasks])

  const startEdit = useCallback(() => {
    setDraft(chapter.script)
    setEditing(true)
    setExpanded(true)
  }, [chapter.script])

  const slides = Array.from({ length: slidesPerChapter }, (_, i) => chapter.slideAssetIds[i] ?? '')

  return (
    <Panel className="overflow-hidden transition-colors hover:border-[hsl(var(--border-strong))]">
      <div className="flex gap-3 p-3">
        {/* Slides, side by side — this is what the operator actually reviews. */}
        <div className="flex shrink-0 gap-2">
          {slides.map((id, i) => (
            <Slide
              key={i}
              id={id}
              slot={i}
              prompt={chapter.slidePrompts[i]}
              task={producingTask(tasks, 'image', chapter.ordinal, i)}
              alt={`Chapter ${chapter.ordinal}, slide ${i + 1}`}
              onOpen={() => openSlide(i)}
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
              <Tooltip label="Run this chapter again — you will be shown what else it affects">
                <Button
                  size="icon"
                  variant="ghost"
                  onClick={() => setRerunning(true)}
                  disabled={retry.isPending || rerunSeeds.length === 0}
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
        onCascade={() => retry.mutate()}
        cascadePending={retry.isPending}
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
              {chapter.slidePrompts.length > 0 && (
                <ul className="mt-3 space-y-1 border-t border-[hsl(var(--border))] pt-2">
                  {chapter.slidePrompts.map((prompt, i) => (
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
 * One slide in the chapter grid. It opens the viewer rather than a browser tab:
 * the prompt that produced the image is the first thing an operator wants
 * beside it, and a raw `/assets/…` tab has nothing but pixels.
 */
function Slide({
  id,
  slot,
  prompt,
  task,
  alt,
  onOpen,
}: {
  id: string
  slot: number
  prompt: string | undefined
  /** The image task for this slot, so the tile can say it is being drawn. */
  task: Task | undefined
  alt: string
  onOpen: () => void
}) {
  // A slot with nothing in it still opens: the viewer is where its prompt is
  // edited, and a slide that has not come out is the usual reason to want that.
  if (!id) {
    return (
      <button
        type="button"
        onClick={onOpen}
        aria-label={`${alt} — not drawn yet; open to edit its prompt`}
        className="flex h-[86px] w-[152px] flex-col items-center justify-center gap-1 rounded-[var(--radius-sm)] border border-dashed border-[hsl(var(--border-strong))] bg-[hsl(var(--bg-subtle))] text-[11px] text-subtle transition-colors hover:border-[hsl(var(--accent))] hover:text-muted"
      >
        <span className="tabular text-[10px] uppercase tracking-wider">slide {slot + 1}</span>
        <span className="flex items-center gap-1.5">
          {task && <TaskStateDot state={task.state} />}
          {tileState(task)}
        </span>
      </button>
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
      {/* The picture on a tile being redrawn is the *previous* one, so the mark
          is the only thing saying so. Always visible, unlike the hover chrome. */}
      <TileWorkingMark task={task} />
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

/**
 * The one word an empty tile can say about itself. "Pending" covers a slot the
 * pipeline has not reached; the rest are the states an operator is waiting on.
 */
function tileState(task: Task | undefined): string {
  if (!task) return 'pending'
  switch (task.state) {
    case 'running':
      return 'drawing'
    case 'ready':
      return 'queued'
    case 'failed':
      return 'failed'
    case 'blocked':
      return task.attempt > 0 && task.error ? 'retrying' : 'pending'
    default:
      return 'pending'
  }
}

/**
 * A corner mark on a tile whose artifact is being replaced.
 *
 * Only for work in flight or a failure: a tile that is simply done should carry
 * no chrome at all, and staleness is already said by the chapter's own dot and
 * the banner above the pane.
 */
function TileWorkingMark({ task }: { task: Task | undefined }) {
  if (!task) return null
  const working = task.state === 'running' || task.state === 'ready'
  const retrying = task.state === 'blocked' && task.attempt > 0 && Boolean(task.error)
  if (!working && !retrying && task.state !== 'failed') return null

  return (
    <span
      className="absolute right-1 top-1 flex items-center gap-1 rounded-[var(--radius-xs)] bg-black/60 px-1 py-[1px] text-[10px] font-medium leading-[14px] text-white"
      title={task.error || `${taskLabel(task.kind)} — ${task.state}`}
    >
      <TaskStateDot state={task.state} />
      {tileState(task)}
    </span>
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
  // Staleness outranks the state colour: a succeeded-but-stale chip must not
  // read green.
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
