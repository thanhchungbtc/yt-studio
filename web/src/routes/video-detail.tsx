import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useParams } from '@tanstack/react-router'
import { useVirtualizer } from '@tanstack/react-virtual'
import {
  Ban,
  Check,
  ChevronDown,
  ChevronRight,
  Download,
  Pencil,
  Play,
  RefreshCw,
  X,
} from 'lucide-react'
import { memo, useCallback, useMemo, useRef, useState } from 'react'

import { PageHeader } from '@/components/app-shell'
import { TaskStateBadge, VideoStateBadge } from '@/components/state-badges'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/field'
import {
  EmptyState,
  ErrorNotice,
  KeyValue,
  Modal,
  Mono,
  Panel,
  PanelHeader,
  PanelTitle,
  Progress,
  Skeleton,
  Tooltip,
} from '@/components/ui/primitives'
import { api, assetUrl, qk, thumbUrl } from '@/lib/api'
import {
  chapterKey,
  formatAbsolute,
  formatBytes,
  formatClock,
  percent,
  taskLabel,
} from '@/lib/format'
import type { Asset, Chapter, GateKind, Task, Video } from '@/lib/types'
import { cn } from '@/lib/utils'

type Tab = 'chapters' | 'tasks' | 'artifacts'

export function VideoDetailRoute() {
  const { ref } = useParams({ from: '/videos/$ref' })
  const [tab, setTab] = useState<Tab>('chapters')

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

  if (video.isPending) {
    return (
      <div className="space-y-3 p-4">
        <Skeleton className="h-10 w-72" />
        <Skeleton className="h-40 w-full" />
      </div>
    )
  }
  if (video.isError || !video.data) {
    return <ErrorNotice error={video.error ?? new Error('video not found')} className="m-4" />
  }

  const v = video.data
  const openGate = tasks.data?.find((t) => t.state === 'awaiting_approval')

  return (
    <>
      <PageHeader
        title={`${v.ref} — ${v.title}`}
        subtitle={
          <span className="flex items-center gap-2">
            <VideoStateBadge state={v.state} />
            <span className="tabular">
              {v.counts.succeeded}/{v.counts.total} tasks
            </span>
            {v.counts.failed > 0 && (
              <span className="tabular text-[hsl(var(--danger))]">{v.counts.failed} failed</span>
            )}
            {v.topic && <span className="truncate text-subtle">· {v.topic}</span>}
          </span>
        }
        actions={<VideoActions video={v} openGate={openGate} />}
      />

      <div className="shrink-0 px-4 pt-3">
        <Progress
          value={v.counts.succeeded}
          total={v.counts.total}
          failed={v.counts.failed}
          running={v.state === 'running'}
          aria-label={`${percent(v.counts.succeeded, v.counts.total)}% complete`}
        />
      </div>

      {openGate && <GateBanner video={v} task={openGate} />}
      {v.error && <ErrorNotice error={new Error(v.error)} className="mx-4 mt-3" />}

      <div className="mt-3 flex shrink-0 gap-1 border-b border-[hsl(var(--border))] px-4">
        {(['chapters', 'tasks', 'artifacts'] as Tab[]).map((t) => (
          <button
            key={t}
            type="button"
            onClick={() => setTab(t)}
            className={cn(
              'relative px-3 py-1.5 text-[12.5px] capitalize transition-colors',
              tab === t ? 'text-fg' : 'text-subtle hover:text-fg',
            )}
          >
            {t}
            {tab === t && (
              <span className="absolute inset-x-2 bottom-[-1px] h-[2px] rounded-full bg-[hsl(var(--accent))]" />
            )}
          </button>
        ))}
      </div>

      <div className="min-h-0 flex-1 overflow-hidden">
        {tab === 'chapters' && (
          <ChapterGrid
            video={v}
            chapters={chapters.data ?? []}
            tasks={tasks.data ?? []}
            loading={chapters.isPending}
          />
        )}
        {tab === 'tasks' && <TaskList tasks={tasks.data ?? []} loading={tasks.isPending} />}
        {tab === 'artifacts' && <ArtifactList videoRef={v.ref} videoId={v.id} video={v} />}
      </div>
    </>
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

  return (
    <>
      {openGate && (
        <>
          <Button variant="success" onClick={() => approve.mutate()} disabled={approve.isPending}>
            <Check className="h-3.5 w-3.5" />
            Approve {openGate.gate}
          </Button>
          <Button variant="outline" onClick={() => setRejecting(true)}>
            <X className="h-3.5 w-3.5" />
            Reject
          </Button>
        </>
      )}
      {canStart && (
        <Button variant="primary" onClick={() => start.mutate()} disabled={start.isPending}>
          <Play className="h-3.5 w-3.5" />
          {video.state === 'draft' ? 'Start' : 'Resume'}
        </Button>
      )}
      {canCancel && (
        <Button variant="ghost" onClick={() => cancel.mutate()} disabled={cancel.isPending}>
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
        />
        {reject.isError && <ErrorNotice error={reject.error} className="mt-3" />}
      </Modal>
    </>
  )
}

function GateBanner({ video, task }: { video: Video; task: Task }) {
  const blueprint = video.blueprintAssetId
  return (
    <div className="mx-4 mt-3 flex items-start gap-3 rounded-[var(--radius-md)] border border-[hsl(var(--warning)/0.4)] bg-[hsl(var(--warning-soft))] px-3 py-2.5">
      <Badge tone="warning" dot pulse>
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
            <a
              className="text-[hsl(var(--accent))] underline-offset-2 hover:underline"
              href={assetUrl(blueprint)}
              target="_blank"
              rel="noreferrer"
            >
              Open the blueprint JSON
            </a>
          )}
          {task.gate === 'upload' && video.finalAssetId && (
            <a
              className="text-[hsl(var(--accent))] underline-offset-2 hover:underline"
              href={assetUrl(video.finalAssetId)}
              target="_blank"
              rel="noreferrer"
            >
              Preview the final render
            </a>
          )}
        </div>
      </div>
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
  const [expanded, setExpanded] = useState(false)
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(chapter.script)

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

  const startEdit = useCallback(() => {
    setDraft(chapter.script)
    setEditing(true)
    setExpanded(true)
  }, [chapter.script])

  const stills = Array.from({ length: imagesPerChapter }, (_, i) => chapter.imageAssetIds[i] ?? '')

  return (
    <Panel className="overflow-hidden">
      <div className="flex gap-3 p-3">
        {/* Stills, side by side — this is what the operator actually reviews. */}
        <div className="flex shrink-0 gap-2">
          {stills.map((id, i) => (
            <Still key={i} id={id} alt={`Chapter ${chapter.ordinal}, still ${i + 1}`} />
          ))}
        </div>

        <div className="flex min-w-0 flex-1 flex-col">
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <div className="flex items-center gap-2">
                <Mono className="text-subtle">{chapterKey(videoRef, chapter.ordinal)}</Mono>
                <span className="truncate text-[13px] font-medium text-fg">{chapter.title}</span>
              </div>
              <p className="mt-0.5 line-clamp-1 text-[11.5px] text-subtle">{chapter.summary}</p>
            </div>
            <div className="flex shrink-0 items-center gap-1">
              <Tooltip label="Re-run this chapter and everything downstream">
                <Button
                  size="icon"
                  variant="ghost"
                  onClick={() => retry.mutate()}
                  disabled={retry.isPending}
                  aria-label={`Retry chapter ${chapter.ordinal}`}
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
              <a
                className="text-[hsl(var(--accent))] underline-offset-2 hover:underline"
                href={assetUrl(chapter.clipAssetId)}
                target="_blank"
                rel="noreferrer"
              >
                clip
              </a>
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

function Still({ id, alt }: { id: string; alt: string }) {
  if (!id) {
    return (
      <div className="flex h-[86px] w-[152px] items-center justify-center rounded-[var(--radius-sm)] border border-dashed border-[hsl(var(--border-strong))] text-[11px] text-subtle">
        pending
      </div>
    )
  }
  return (
    <a href={assetUrl(id)} target="_blank" rel="noreferrer" className="group relative block">
      <img
        src={thumbUrl(id)}
        alt={alt}
        width={152}
        height={86}
        loading="lazy"
        decoding="async"
        className="h-[86px] w-[152px] rounded-[var(--radius-sm)] border border-[hsl(var(--border))] object-cover transition-opacity group-hover:opacity-85"
      />
    </a>
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
  return (
    <Tooltip label={task.error || `${label} — ${task.state} (attempt ${task.attempt})`}>
      <span>
        <Badge tone={tone} dot pulse={task.state === 'running'}>
          {label}
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

function ArtifactList({
  videoRef,
  videoId,
  video,
}: {
  videoRef: string
  videoId: string
  video: Video
}) {
  const assets = useQuery({
    queryKey: qk.assets(videoId),
    queryFn: () => api.listAssets(videoRef),
  })

  const grouped = useMemo(() => {
    const map = new Map<string, Asset[]>()
    for (const asset of assets.data ?? []) {
      const list = map.get(asset.kind)
      if (list) list.push(asset)
      else map.set(asset.kind, [asset])
    }
    return [...map.entries()].sort((a, b) => a[0].localeCompare(b[0]))
  }, [assets.data])

  return (
    <div className="grid h-full grid-cols-[1fr_320px] gap-4 overflow-y-auto p-4">
      <div className="space-y-3">
        {assets.isPending && <Skeleton className="h-40" />}
        {grouped.map(([kind, list]) => (
          <Panel key={kind}>
            <PanelHeader>
              <PanelTitle>
                {kind} <span className="text-subtle">({list.length})</span>
              </PanelTitle>
            </PanelHeader>
            <ul className="divide-y divide-[hsl(var(--border))]">
              {list.map((asset) => (
                <li key={asset.id} className="flex items-center gap-3 px-3 py-1.5 text-[12px]">
                  <Mono className="truncate text-subtle">{asset.id.slice(0, 16)}</Mono>
                  <span className="tabular ml-auto text-muted">{formatBytes(asset.size)}</span>
                  <a
                    href={asset.url}
                    target="_blank"
                    rel="noreferrer"
                    className="text-[hsl(var(--accent))] hover:underline"
                    aria-label={`Open ${kind} ${asset.id.slice(0, 8)}`}
                  >
                    <Download className="h-3.5 w-3.5" />
                  </a>
                </li>
              ))}
            </ul>
          </Panel>
        ))}
        {!assets.isPending && grouped.length === 0 && (
          <EmptyState title="No artifacts yet" description="Artifacts appear as tasks complete." />
        )}
      </div>

      <div className="space-y-3">
        <Panel>
          <PanelHeader>
            <PanelTitle>Video</PanelTitle>
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

        {video.metadata && (
          <Panel>
            <PanelHeader>
              <PanelTitle>Metadata</PanelTitle>
            </PanelHeader>
            <div className="space-y-2 px-3 py-2 text-[12px]">
              <p className="font-medium text-fg">{video.metadata.title}</p>
              <p className="whitespace-pre-wrap text-[11.5px] text-muted">
                {video.metadata.description}
              </p>
              <div className="flex flex-wrap gap-1">
                {video.metadata.tags.map((tag) => (
                  <Badge key={tag} tone="violet">
                    {tag}
                  </Badge>
                ))}
              </div>
            </div>
          </Panel>
        )}

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
  )
}
