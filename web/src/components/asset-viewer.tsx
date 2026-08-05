import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ChevronLeft,
  ChevronRight,
  Download,
  ExternalLink,
  File as FileIcon,
  FileJson,
  FileText,
  Film,
  Image as ImageIcon,
  Maximize2,
  Minimize2,
  Music,
  PanelRight,
  RefreshCw,
  Sparkles,
  Tag,
  X,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import * as DialogPrimitive from '@radix-ui/react-dialog'

import { Badge, TONE_FILL } from '@/components/ui/badge'
import type { Tone } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/field'
import { CopyButton, ErrorNotice, Kbd, Skeleton, Tooltip } from '@/components/ui/primitives'
import { RerunDialog } from '@/components/stale'
import { api, assetUrl, qk } from '@/lib/api'
import { downloadName, mediaTypeOf, pendingIconId, pendingSlideId, shortId } from '@/lib/assets'
import type { MediaType, ViewerItem } from '@/lib/assets'
import { formatAbsolute, formatBytes } from '@/lib/format'
import { useHotkeys } from '@/lib/hotkeys'
import type { Chapter, Task } from '@/lib/types'
import { cn } from '@/lib/utils'

/* ------------------------------------------------------------ kind vocabulary */

const KIND_ICON: Record<string, LucideIcon> = {
  blueprint: FileJson,
  script: FileText,
  prompt: Sparkles,
  audio: Music,
  image: ImageIcon,
  thumbnail: ImageIcon,
  thumbnail_icon: ImageIcon,
  thumbnail_plan: FileJson,
  clip: Film,
  final: Film,
  metadata: Tag,
}

const KIND_TONE: Record<string, Tone> = {
  blueprint: 'info',
  script: 'neutral',
  prompt: 'violet',
  audio: 'info',
  image: 'violet',
  thumbnail: 'violet',
  thumbnail_icon: 'violet',
  thumbnail_plan: 'info',
  clip: 'accent',
  final: 'success',
  metadata: 'warning',
}

/**
 * Written out per tone rather than composed from a variable, because Tailwind
 * only emits classes it can see as literals.
 */
const TILE_TONE: Record<Tone, string> = {
  neutral: 'bg-[hsl(var(--bg-hover))] text-subtle',
  accent: 'bg-[hsl(var(--accent-soft))] text-[hsl(var(--accent))]',
  success: 'bg-[hsl(var(--success-soft))] text-[hsl(var(--success))]',
  warning: 'bg-[hsl(var(--warning-soft))] text-[hsl(var(--warning))]',
  danger: 'bg-[hsl(var(--danger-soft))] text-[hsl(var(--danger))]',
  info: 'bg-[hsl(var(--info-soft))] text-[hsl(var(--info))]',
  violet: 'bg-[hsl(var(--violet-soft))] text-[hsl(var(--violet))]',
}

export function assetKindIcon(kind: string): LucideIcon {
  return KIND_ICON[kind] ?? FileIcon
}

export function assetKindTone(kind: string): Tone {
  return KIND_TONE[kind] ?? 'neutral'
}

/* --------------------------------------------------------------------- preview */

/**
 * The visual stand-in for one artifact, at any size: the image itself where
 * there is one, and a tinted, typed tile where there is not. Used by the
 * gallery, the filmstrip and the chapter grid, so a slide looks the same
 * wherever it is shown.
 */
export function AssetPreview({ item, className }: { item: ViewerItem; className?: string }) {
  const media = mediaTypeOf(item.mime)
  const Icon = assetKindIcon(item.kind)

  // A pending slot claims an image MIME but has no bytes behind its id; asking
  // for them would draw a broken picture at every size the tile is used.
  if (media === 'image' && !item.pending) {
    return (
      <img
        src={assetUrl(item.id)}
        alt={item.title}
        loading="lazy"
        decoding="async"
        className={cn('h-full w-full bg-[hsl(var(--bg-hover))] object-cover', className)}
      />
    )
  }

  return (
    <div
      className={cn(
        'flex h-full w-full items-center justify-center',
        TILE_TONE[assetKindTone(item.kind)],
        className,
      )}
      aria-hidden
    >
      <Icon className="h-1/3 max-h-7 w-1/3 max-w-7 opacity-80" strokeWidth={1.5} />
    </div>
  )
}

/* --------------------------------------------------------------------- context */

type OpenViewer = (items: ViewerItem[], index?: number) => void

const ViewerContext = createContext<OpenViewer>(() => {})

/** Opens the artifact viewer over whatever is currently on screen. */
export function useAssetViewer(): OpenViewer {
  return useContext(ViewerContext)
}

/**
 * One viewer for the whole pane. Every slide, clip and blueprint opens into the
 * same surface, so the operator learns its keys once — and a chapter card does
 * not have to carry modal state per row.
 *
 * The video is passed in so the viewer can offer to re-run the step that
 * produced whatever is on screen: reviewing an artifact and deciding to redo it
 * is one thought, and it should not need a trip to the task table.
 */
export function AssetViewerProvider({
  children,
  videoRef,
  videoId,
}: {
  children: ReactNode
  videoRef?: string
  videoId?: string
}) {
  const [state, setState] = useState<{ items: ViewerItem[]; index: number } | null>(null)
  const [rerunning, setRerunning] = useState<ViewerItem | null>(null)

  const open = useCallback<OpenViewer>((items, index = 0) => {
    if (items.length === 0) return
    setState({ items, index: Math.min(Math.max(index, 0), items.length - 1) })
  }, [])

  return (
    <ViewerContext.Provider value={open}>
      {children}
      {state && (
        <AssetLightbox
          items={state.items}
          index={state.index}
          onIndex={(index) => setState((prev) => (prev ? { ...prev, index } : prev))}
          onClose={() => setState(null)}
          onRerun={videoRef && videoId ? setRerunning : undefined}
          videoRef={videoRef}
          videoId={videoId}
        />
      )}
      {rerunning?.taskId && videoRef && videoId && (
        <RerunDialog
          open
          onOpenChange={(open) => !open && setRerunning(null)}
          videoRef={videoRef}
          videoId={videoId}
          taskIds={[rerunning.taskId]}
          what={rerunning.title.toLowerCase()}
        />
      )}
    </ViewerContext.Provider>
  )
}

/* -------------------------------------------------------------------- lightbox */

/**
 * The task that produced what is on screen, live.
 *
 * Read out of the video's task list, which the event stream already patches per
 * delta — so this costs no request of its own and updates as the scheduler
 * moves. It is what lets the inspector say "drawing" rather than "queued" until
 * something happens to disagree with it.
 */
function useLiveTask(
  taskId: string | undefined,
  videoRef: string | undefined,
  videoId: string | undefined,
): Task | undefined {
  const tasks = useQuery({
    queryKey: qk.videoTasks(videoId ?? ''),
    queryFn: () => api.listVideoTasks(videoRef ?? ''),
    enabled: Boolean(taskId) && Boolean(videoRef) && Boolean(videoId),
  })
  return useMemo(
    () => (taskId ? tasks.data?.find((t) => t.id === taskId) : undefined),
    [tasks.data, taskId],
  )
}

/**
 * Keeps a generated tile current while the viewer is open.
 *
 * The viewer is handed a snapshot of items, but anything redrawn from the
 * inspector lands under a *different* content address — so the artifact the
 * snapshot names stops being the one in that position. Re-resolving by the
 * coordinate that does not change (chapter and slot for a slide, cell for an
 * icon) means the operator watches the new picture arrive in place instead of
 * looking at the old one until they close and reopen the panel.
 *
 * Everything else is returned untouched.
 */
function useLiveTile(
  item: ViewerItem | undefined,
  videoRef: string | undefined,
  videoId: string | undefined,
): ViewerItem | undefined {
  const isSlide = item?.chapterId !== undefined && item.slot !== undefined
  const isIcon = item?.cell !== undefined
  const keyed = Boolean(videoRef) && Boolean(videoId)

  // The detail route has already fetched both; the shared keys make these cache
  // reads that stay subscribed rather than second requests.
  const chapters = useQuery({
    queryKey: qk.chapters(videoId ?? ''),
    queryFn: () => api.listChapters(videoRef ?? ''),
    enabled: isSlide && keyed,
  })
  const video = useQuery({
    queryKey: qk.video(videoRef ?? ''),
    queryFn: () => api.getVideo(videoRef ?? ''),
    enabled: isIcon && keyed,
  })

  return useMemo(() => {
    if (!item) return item
    if (item.chapterId !== undefined && item.slot !== undefined) {
      const chapter = chapters.data?.find((c) => c.id === item.chapterId)
      if (!chapter) return item
      const id = chapter.slideAssetIds[item.slot] ?? ''
      const prompt = chapter.slidePrompts[item.slot]
      if (id === item.id && prompt === item.prompt) return item
      return { ...item, id: id || pendingSlideId(chapter.id, item.slot), pending: !id, prompt }
    }
    if (item.cell !== undefined && video.data) {
      const id = video.data.thumbnailIconIds[item.cell] ?? ''
      const prompt = video.data.thumbnailPlan[item.cell]?.prompt
      if (id === item.id && prompt === item.prompt) return item
      return { ...item, id: id || pendingIconId(video.data.id, item.cell), pending: !id, prompt }
    }
    return item
  }, [item, chapters.data, video.data])
}

function AssetLightbox({
  items,
  index,
  onIndex,
  onClose,
  onRerun,
  videoRef,
  videoId,
}: {
  items: ViewerItem[]
  index: number
  onIndex: (index: number) => void
  onClose: () => void
  /** Offered only where the viewer knows which video it is inside. */
  onRerun?: (item: ViewerItem) => void
  videoRef?: string
  videoId?: string
}) {
  const item = useLiveTile(items[index], videoRef, videoId)
  const [actualSize, setActualSize] = useState(false)
  const [inspector, setInspector] = useState(true)
  const stripRef = useRef<HTMLDivElement>(null)

  const go = useCallback(
    (delta: number) => onIndex((index + delta + items.length) % items.length),
    [index, items.length, onIndex],
  )

  // A new artifact is always shown fitted; carrying 1:1 across a step would land
  // the operator mid-image.
  useEffect(() => setActualSize(false), [index])

  // The neighbours are immutable and almost always next, so prefetching them
  // removes the flash for one request.
  useEffect(() => {
    for (const neighbour of [items[index + 1], items[index - 1]]) {
      if (!neighbour || neighbour.pending || mediaTypeOf(neighbour.mime) !== 'image') continue
      const url = assetUrl(neighbour.id)
      if (url) new Image().src = url
    }
  }, [items, index])

  useEffect(() => {
    const active = stripRef.current?.querySelector('[data-active="true"]')
    active?.scrollIntoView({ block: 'nearest', inline: 'center' })
  }, [index])

  const download = useCallback(() => {
    if (!item) return
    const anchor = document.createElement('a')
    anchor.href = assetUrl(item.id) ?? ''
    anchor.download = downloadName(item)
    anchor.click()
  }, [item])

  useHotkeys([
    {
      keys: 'arrowright',
      label: 'Next artifact',
      group: 'Preview',
      hidden: true,
      run: () => go(1),
    },
    {
      keys: 'arrowleft',
      label: 'Previous artifact',
      group: 'Preview',
      hidden: true,
      run: () => go(-1),
    },
    {
      keys: 'f',
      label: 'Fit or actual size',
      group: 'Preview',
      hidden: true,
      run: () => setActualSize((prev) => !prev),
    },
    {
      keys: 'i',
      label: 'Toggle the inspector',
      group: 'Preview',
      hidden: true,
      run: () => setInspector((prev) => !prev),
    },
    { keys: 'd', label: 'Download', group: 'Preview', hidden: true, run: download },
  ])

  if (!item) return null
  const media = mediaTypeOf(item.mime)
  const Icon = assetKindIcon(item.kind)

  return (
    <DialogPrimitive.Root open onOpenChange={(open) => !open && onClose()}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Overlay className="animate-in-fade fixed inset-0 z-40 bg-black/70 backdrop-blur-[3px]" />
        <DialogPrimitive.Content
          aria-describedby={undefined}
          onOpenAutoFocus={(event) => {
            // Keep focus on the dialog itself; auto-focusing the first toolbar
            // button pops its tooltip open the moment the viewer appears.
            event.preventDefault()
            ;(event.target as HTMLElement).focus()
          }}
          className={cn(
            'animate-in-zoom fixed inset-3 z-50 flex flex-col overflow-hidden sm:inset-6',
            'rounded-[var(--radius-xl)] border border-[hsl(var(--border-strong))] bg-[hsl(var(--bg-elevated))] elev-3',
          )}
        >
          {/* ------------------------------------------------------ header */}
          <header className="flex h-12 shrink-0 items-center gap-3 border-b border-[hsl(var(--border))] bg-subtle px-3 no-select">
            <span
              className={cn(
                'flex h-7 w-7 shrink-0 items-center justify-center rounded-[var(--radius-sm)]',
                TILE_TONE[assetKindTone(item.kind)],
              )}
            >
              <Icon className="h-4 w-4" strokeWidth={1.75} />
            </span>

            <div className="flex min-w-0 flex-col">
              <DialogPrimitive.Title className="truncate text-[13px] font-semibold leading-tight text-fg">
                {item.title}
              </DialogPrimitive.Title>
              {item.subtitle && (
                <span className="truncate text-[11px] leading-tight text-subtle">
                  {item.subtitle}
                </span>
              )}
            </div>

            <Badge tone={assetKindTone(item.kind)} className="ml-1 shrink-0">
              {item.kind}
            </Badge>

            {items.length > 1 && (
              <div className="ml-2 flex shrink-0 items-center gap-1">
                <Tooltip label="Previous" keys="arrowleft">
                  <Button size="icon" variant="ghost" onClick={() => go(-1)} aria-label="Previous">
                    <ChevronLeft className="h-4 w-4" />
                  </Button>
                </Tooltip>
                <span className="tabular w-14 text-center text-[11.5px] text-subtle">
                  {index + 1} / {items.length}
                </span>
                <Tooltip label="Next" keys="arrowright">
                  <Button size="icon" variant="ghost" onClick={() => go(1)} aria-label="Next">
                    <ChevronRight className="h-4 w-4" />
                  </Button>
                </Tooltip>
              </div>
            )}

            <div className="ml-auto flex shrink-0 items-center gap-1">
              {media === 'image' && !item.pending && (
                <Tooltip label={actualSize ? 'Fit to window' : 'Actual size'} keys="f">
                  <Button
                    size="icon"
                    variant="ghost"
                    onClick={() => setActualSize((prev) => !prev)}
                    aria-label={actualSize ? 'Fit to window' : 'Actual size'}
                  >
                    {actualSize ? (
                      <Minimize2 className="h-4 w-4" />
                    ) : (
                      <Maximize2 className="h-4 w-4" />
                    )}
                  </Button>
                </Tooltip>
              )}
              {/* There are no bytes behind a pending slot, so neither of these
                  has anything to open or write. */}
              {!item.pending && (
                <>
                  <Tooltip label="Open the raw artifact">
                    <Button size="icon" variant="ghost" asChild>
                      <a
                        href={assetUrl(item.id)}
                        target="_blank"
                        rel="noreferrer"
                        aria-label="Open the raw artifact"
                      >
                        <ExternalLink className="h-4 w-4" />
                      </a>
                    </Button>
                  </Tooltip>
                  <Tooltip label="Download" keys="d">
                    <Button size="icon" variant="ghost" onClick={download} aria-label="Download">
                      <Download className="h-4 w-4" />
                    </Button>
                  </Tooltip>
                </>
              )}
              {/* Re-runs the step that made this artifact and nothing else.
                  Absent when the viewer cannot tell which task that was, and on a
                  pending slot, where Generate in the inspector is the way to run
                  it — with the prompt in front of the operator. */}
              {onRerun && item.taskId && !item.pending && (
                <Tooltip label="Re-run the step that made this">
                  <Button
                    size="icon"
                    variant="ghost"
                    onClick={() => onRerun(item)}
                    aria-label="Re-run the step that made this artifact"
                  >
                    <RefreshCw className="h-4 w-4" />
                  </Button>
                </Tooltip>
              )}
              <Tooltip label={inspector ? 'Hide details' : 'Show details'} keys="i">
                <Button
                  size="icon"
                  variant="ghost"
                  onClick={() => setInspector((prev) => !prev)}
                  aria-label={inspector ? 'Hide details' : 'Show details'}
                  className={cn(inspector && 'text-fg')}
                >
                  <PanelRight className="h-4 w-4" />
                </Button>
              </Tooltip>
              <span className="mx-1 h-4 w-px bg-[hsl(var(--border))]" aria-hidden />
              <Tooltip label="Close" keys="escape">
                <DialogPrimitive.Close asChild>
                  <Button size="icon" variant="ghost" aria-label="Close">
                    <X className="h-4 w-4" />
                  </Button>
                </DialogPrimitive.Close>
              </Tooltip>
            </div>
          </header>

          {/* -------------------------------------------------------- body */}
          <div className="flex min-h-0 flex-1">
            <div className="group relative flex min-w-0 flex-1 flex-col bg-[hsl(var(--bg-subtle))]">
              <Stage
                item={item}
                media={media}
                actualSize={actualSize}
                onToggleSize={setActualSize}
              />

              {items.length > 1 && (
                <>
                  <EdgeButton side="left" onClick={() => go(-1)} />
                  <EdgeButton side="right" onClick={() => go(1)} />
                </>
              )}
            </div>

            {inspector && <Inspector item={item} videoRef={videoRef} videoId={videoId} />}
          </div>

          {/* --------------------------------------------------- filmstrip */}
          {items.length > 1 && (
            <div
              ref={stripRef}
              className="flex h-[70px] shrink-0 items-center gap-2 overflow-x-auto border-t border-[hsl(var(--border))] bg-subtle px-3 no-select"
            >
              {items.map((entry, i) => (
                <button
                  key={`${entry.id}-${i}`}
                  type="button"
                  data-active={i === index}
                  onClick={() => onIndex(i)}
                  title={entry.title}
                  aria-label={entry.title}
                  aria-current={i === index}
                  className={cn(
                    'relative h-[50px] w-[88px] shrink-0 overflow-hidden rounded-[var(--radius-sm)]',
                    'border transition-[opacity,border-color]',
                    i === index
                      ? 'border-[hsl(var(--accent))] opacity-100 ring-1 ring-[hsl(var(--accent))]'
                      : 'border-[hsl(var(--border))] opacity-55 hover:opacity-100',
                  )}
                >
                  <AssetPreview item={entry} />
                </button>
              ))}
            </div>
          )}
        </DialogPrimitive.Content>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  )
}

function EdgeButton({ side, onClick }: { side: 'left' | 'right'; onClick: () => void }) {
  const Icon = side === 'left' ? ChevronLeft : ChevronRight
  return (
    <button
      type="button"
      onClick={onClick}
      aria-label={side === 'left' ? 'Previous artifact' : 'Next artifact'}
      className={cn(
        'absolute top-1/2 z-10 flex h-9 w-9 -translate-y-1/2 items-center justify-center',
        'rounded-full border border-[hsl(var(--border-strong))] bg-[hsl(var(--bg-elevated))]/85 text-muted backdrop-blur',
        'opacity-0 transition-opacity elev-2 hover:text-fg focus-visible:opacity-100 group-hover:opacity-100',
        side === 'left' ? 'left-3' : 'right-3',
      )}
    >
      <Icon className="h-5 w-5" />
    </button>
  )
}

/* ----------------------------------------------------------------------- stage */

function Stage({
  item,
  media,
  actualSize,
  onToggleSize,
}: {
  item: ViewerItem
  media: MediaType
  actualSize: boolean
  onToggleSize: (next: boolean) => void
}) {
  const url = assetUrl(item.id)

  // Nothing has been drawn here yet. Said plainly rather than as a broken image,
  // because the panel beside it is where the operator does something about it.
  if (item.pending) {
    return (
      <div className="flex min-h-0 flex-1 flex-col items-center justify-center gap-3 p-8 text-center">
        <ImageIcon className="h-8 w-8 text-subtle" strokeWidth={1.5} />
        <p className="text-[12.5px] text-muted">
          This slide has not been drawn yet.
          <br />
          Edit the prompt beside it and generate.
        </p>
      </div>
    )
  }

  if (media === 'image') {
    return (
      <div
        className={cn(
          'checker flex min-h-0 flex-1 p-6',
          actualSize ? 'items-start justify-start overflow-auto' : 'items-center justify-center',
        )}
      >
        <img
          src={url}
          alt={item.title}
          onClick={() => onToggleSize(!actualSize)}
          className={cn(
            'rounded-[var(--radius-sm)] elev-3',
            actualSize
              ? 'max-w-none cursor-zoom-out'
              : 'max-h-full max-w-full cursor-zoom-in object-contain',
          )}
        />
      </div>
    )
  }

  if (media === 'video') {
    return (
      <div className="flex min-h-0 flex-1 items-center justify-center bg-black p-4">
        {/* eslint-disable-next-line jsx-a11y/media-has-caption */}
        <video controls autoPlay preload="metadata" src={url} className="max-h-full max-w-full" />
      </div>
    )
  }

  if (media === 'audio') {
    return (
      <div className="flex min-h-0 flex-1 flex-col items-center justify-center gap-5 p-8">
        <Waveform />
        <audio controls preload="metadata" src={url} className="w-full max-w-md" />
      </div>
    )
  }

  if (media === 'text') return <TextStage item={item} />

  return (
    <div className="flex min-h-0 flex-1 flex-col items-center justify-center gap-3 p-8 text-center">
      <FileIcon className="h-8 w-8 text-subtle" strokeWidth={1.5} />
      <p className="text-[12.5px] text-muted">
        {item.mime} cannot be previewed in the browser. Download it to inspect the bytes.
      </p>
    </div>
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

function TextStage({ item }: { item: ViewerItem }) {
  // Content-addressed: the body behind this hash can never change, so it is
  // fetched once and kept for the session.
  const body = useQuery({
    queryKey: ['asset-text', item.id],
    queryFn: async () => {
      const response = await fetch(assetUrl(item.id) ?? '')
      if (!response.ok) throw new Error(`failed to read artifact (${response.status})`)
      const text = await response.text()
      try {
        return JSON.stringify(JSON.parse(text), null, 2)
      } catch {
        return text
      }
    },
    staleTime: Infinity,
  })

  return (
    <div className="min-h-0 flex-1 overflow-auto p-4">
      {body.isPending && <Skeleton className="h-full min-h-[240px] w-full" />}
      {body.isError && (
        <p className="text-[12px] text-[hsl(var(--danger))]">{String(body.error)}</p>
      )}
      {body.data !== undefined && (
        <pre className="whitespace-pre-wrap break-words rounded-[var(--radius-sm)] border border-[hsl(var(--border))] bg-[hsl(var(--bg-elevated))] p-4 font-mono text-[12px] leading-relaxed text-fg">
          {body.data}
        </pre>
      )}
    </div>
  )
}

/* ------------------------------------------------------------------- inspector */

function Inspector({
  item,
  videoRef,
  videoId,
}: {
  item: ViewerItem
  videoRef?: string
  videoId?: string
}) {
  const queryClient = useQueryClient()
  // Pulled out of the item so the narrowing below survives into the callbacks,
  // which close over them rather than over a prop.
  const { chapterId, slot, cell } = item
  const task = useLiveTask(item.taskId, videoRef, videoId)

  return (
    <aside className="w-[320px] shrink-0 overflow-y-auto border-l border-[hsl(var(--border))] bg-[hsl(var(--bg-panel))]">
      {chapterId !== undefined && slot !== undefined && videoRef && videoId && (
        <PromptEditor
          key={`${chapterId}:${slot}`}
          label="Slide prompt"
          ariaLabel={`Prompt for slide ${slot + 1}`}
          noun="slide"
          task={task}
          prompt={item.prompt}
          missing="The prompt step has not run for this chapter yet, so there is nothing to edit here."
          hint={
            (item.pending
              ? 'Generating saves this prompt and draws this slide. '
              : 'Generating saves this prompt and redraws this slide only. ') +
            'What it fed — this chapter’s clip, and the render below it — keeps its artifact and is flagged for you to decide on.'
          }
          generate={async (prompt) => {
            const updated = await api.regenerateSlide(chapterId, slot, prompt)
            queryClient.setQueryData<Chapter[]>(qk.chapters(videoId), (prev) =>
              prev?.map((c) => (c.id === updated.id ? { ...c, ...updated } : c)),
            )
            void queryClient.invalidateQueries({ queryKey: qk.videoTasks(videoId) })
            void queryClient.invalidateQueries({ queryKey: qk.video(videoRef) })
          }}
        />
      )}

      {cell !== undefined && videoRef && videoId && (
        <PromptEditor
          key={`icon:${cell}`}
          label="Icon prompt"
          ariaLabel={`Prompt for thumbnail cell ${cell + 1}`}
          noun="icon"
          task={task}
          prompt={item.prompt}
          missing="The thumbnail plan has not run yet, so this cell has nothing to picture."
          hint={
            (item.pending
              ? 'Generating saves this prompt and draws this cell. '
              : 'Generating saves this prompt and redraws this cell only. ') +
            'The caption above and the style the grid shares are left alone. The composed thumbnail is flagged — and the upload gate rides on it, so this reopens the publish decision.'
          }
          generate={async (prompt) => {
            const updated = await api.regenerateThumbnailIcon(videoRef, cell, prompt)
            queryClient.setQueryData(qk.video(videoRef), updated)
            void queryClient.invalidateQueries({ queryKey: qk.videoTasks(videoId) })
          }}
        />
      )}

      {item.notes?.map((note) => (
        <section key={note.label} className="border-b border-[hsl(var(--border))] px-3 py-3">
          <div className="mb-1.5 flex items-center justify-between gap-2">
            <h3 className="text-[11px] font-semibold uppercase tracking-wider text-subtle">
              {note.label}
            </h3>
            <CopyButton value={note.body} label={`Copy the ${note.label.toLowerCase()}`} />
          </div>
          <p
            className={cn(
              'whitespace-pre-wrap text-[12px] leading-relaxed text-muted',
              note.mono && 'font-mono text-[11.5px]',
            )}
          >
            {note.body}
          </p>
        </section>
      ))}

      {/* A pending slot has no bytes, so it has no hash, no size and no date —
          a section of blanks under a heading that promises an artifact. */}
      {!item.pending && (
        <section className="px-3 py-3">
          <h3 className="mb-1.5 text-[11px] font-semibold uppercase tracking-wider text-subtle">
            Artifact
          </h3>
          <dl className="space-y-0">
            <Row label="Kind">{item.kind}</Row>
            <Row label="Type">{item.mime}</Row>
            {item.size !== undefined && <Row label="Size">{formatBytes(item.size)}</Row>}
            {item.createdAt && <Row label="Created">{formatAbsolute(item.createdAt)}</Row>}
            <div className="flex items-baseline justify-between gap-3 py-1">
              <dt className="shrink-0 text-[11.5px] text-subtle">Address</dt>
              <dd className="flex min-w-0 items-center gap-1">
                <Tooltip label={item.id}>
                  <span className="truncate font-mono text-[11.5px] text-fg">
                    {shortId(item.id)}
                  </span>
                </Tooltip>
                <CopyButton value={item.id} label="Copy the content address" />
              </dd>
            </div>
          </dl>
          <p className="mt-2 text-[11px] leading-relaxed text-subtle">
            The address is the hash of the bytes: an identical re-run produces this same artifact
            and writes nothing new.
          </p>
        </section>
      )}

      <section className="border-t border-[hsl(var(--border))] px-3 py-3">
        <h3 className="mb-2 text-[11px] font-semibold uppercase tracking-wider text-subtle">
          Keys
        </h3>
        <ul className="space-y-1.5 text-[11.5px] text-muted">
          <ShortcutRow keys="arrowleft" label="Previous artifact" />
          <ShortcutRow keys="arrowright" label="Next artifact" />
          <ShortcutRow keys="f" label="Fit or actual size" />
          <ShortcutRow keys="i" label="Hide these details" />
          <ShortcutRow keys="d" label="Download" />
          <ShortcutRow keys="escape" label="Close" />
        </ul>
      </section>
    </aside>
  )
}

/**
 * The prompt a slide was drawn from, and the button that draws it again.
 *
 * Editing and generating are one action deliberately. A prompt that could be
 * saved on its own would let the text drift from the picture beside it with
 * nothing on screen to say which of the two was current; because Generate is
 * the only way to write one, what the server holds is always what drew the
 * slide — or what is drawing it right now.
 */
function PromptEditor({
  label,
  ariaLabel,
  noun,
  prompt,
  missing,
  hint,
  task,
  generate,
}: {
  label: string
  ariaLabel: string
  /** What is being drawn, for the status line: "slide", "cell". */
  noun: string
  /** Undefined where the step that writes prompts has not run. */
  prompt: string | undefined
  /** What to say in that case, in the vocabulary of the thing being drawn. */
  missing: string
  hint: string
  /** The task that draws this, live, so the panel can say where it is. */
  task: Task | undefined
  /** Writes the prompt, starts the re-run, and reconciles the caches. */
  generate: (prompt: string) => Promise<void>
}) {
  const [draft, setDraft] = useState(prompt ?? '')
  const run = useMutation({ mutationFn: () => generate(draft) })

  const trimmed = draft.trim()
  const unsaved = prompt !== undefined && trimmed !== prompt
  // A generation that has not finished. The old picture stays on screen while
  // the new one is drawn, so without this the panel looks idle for as long as
  // the provider takes — and longer, if the task is waiting for a pool slot.
  const working = task ? task.state === 'running' || task.state === 'ready' : false
  const busy = run.isPending || working

  return (
    <section className="border-b border-[hsl(var(--border))] px-3 py-3">
      <div className="mb-1.5 flex items-center justify-between gap-2">
        <h3 className="text-[11px] font-semibold uppercase tracking-wider text-subtle">{label}</h3>
        {unsaved ? (
          <Badge tone="warning">unsaved</Badge>
        ) : (
          <CopyButton value={draft} label={`Copy the ${label.toLowerCase()}`} />
        )}
      </div>

      {prompt === undefined ? (
        <p className="text-[11.5px] leading-relaxed text-subtle">{missing}</p>
      ) : (
        <>
          <Textarea
            value={draft}
            onChange={(event) => setDraft(event.target.value)}
            rows={7}
            spellCheck={false}
            aria-label={ariaLabel}
            className="font-mono text-[11.5px] leading-relaxed"
          />
          <div className="mt-2 flex items-center gap-2">
            {/* Still enabled while it draws: changing your mind mid-generation is
                legitimate, and the scheduler's generation counter discards the
                answer to the question you stopped asking. */}
            <Button
              size="sm"
              variant="primary"
              disabled={trimmed === '' || run.isPending}
              onClick={() => run.mutate()}
            >
              <Sparkles className={cn('h-3.5 w-3.5', busy && 'animate-pulse')} />
              {run.isPending ? 'Starting…' : working ? 'Generate again' : 'Generate'}
            </Button>
            {unsaved && (
              <Button size="sm" variant="ghost" onClick={() => setDraft(prompt)}>
                Revert
              </Button>
            )}
          </div>
          <DrawStatus task={task} noun={noun} />
          <p className="mt-2 text-[11px] leading-relaxed text-subtle">{hint}</p>
          {run.isError && <ErrorNotice error={run.error} className="mt-2" />}
        </>
      )}
    </section>
  )
}

/**
 * Where the generation actually is.
 *
 * Worth its own line because a redrawn tile cannot show this by itself: the
 * previous picture stays on screen until the new one lands, so queued, drawing
 * and retrying-after-a-provider-error all look identical. The wait is often the
 * pool rather than the provider — a redraw during a fifty-chapter render sits in
 * `ready` behind everything else holding an image slot — so that case is named
 * rather than left as a spinner.
 */
function DrawStatus({ task, noun }: { task: Task | undefined; noun: string }) {
  // A retry's delay is the one number here that changes on its own, so it gets
  // the only timer, and only while one is pending.
  const retryAt = task?.state === 'blocked' && task.notBefore ? Date.parse(task.notBefore) : 0
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    if (!retryAt) return undefined
    const timer = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(timer)
  }, [retryAt])

  if (!task) return null

  const attempt = task.attempt > 1 ? ` · attempt ${task.attempt} of ${task.maxAttempts}` : ''

  switch (task.state) {
    case 'running':
      return (
        <StatusLine tone="accent" pulse>
          Drawing the {noun}…{attempt}
        </StatusLine>
      )
    case 'ready':
      return (
        <StatusLine tone="info" pulse>
          Queued — waiting for a slot in the {task.pool} pool.
        </StatusLine>
      )
    case 'blocked':
      // A retryable failure and an unmet dependency are both `blocked`; the
      // attempt count is what separates them, and the deadline only refines the
      // wording.
      if (task.attempt > 0 && task.error) {
        const seconds = retryAt ? Math.max(0, Math.round((retryAt - now) / 1000)) : 0
        return (
          <StatusLine tone="warning" detail={task.error}>
            Attempt {task.attempt} failed — trying again {retryAt ? `in ${seconds}s` : 'shortly'}.
          </StatusLine>
        )
      }
      return <StatusLine tone="info">Waiting on an earlier step.</StatusLine>
    case 'failed':
      return (
        <StatusLine tone="danger" detail={task.error}>
          Gave up after {task.attempt} {task.attempt === 1 ? 'attempt' : 'attempts'}. Change the
          prompt and generate again, or fix the backend and retry.
        </StatusLine>
      )
    case 'cancelled':
      return <StatusLine tone="neutral">Cancelled with the video.</StatusLine>
    // Nothing to announce: the picture above is the statement. A stale one is
    // already flagged in the task table and the banner.
    case 'succeeded':
    case 'awaiting_approval':
      return null
    default:
      return null
  }
}

function StatusLine({
  tone,
  pulse,
  detail,
  children,
}: {
  tone: Tone
  pulse?: boolean
  /** The provider's own words, when there are any. */
  detail?: string
  children: ReactNode
}) {
  return (
    <div className="mt-2 flex items-start gap-2">
      <span
        aria-hidden
        className={cn(
          'mt-[5px] h-[7px] w-[7px] shrink-0 rounded-full',
          TONE_FILL[tone],
          pulse && 'pulse-live',
        )}
      />
      <div className="min-w-0 text-[11px] leading-relaxed">
        <p className="text-fg">{children}</p>
        {detail && (
          <p className="mt-0.5 break-words font-mono text-[10.5px] text-subtle">{detail}</p>
        )}
      </div>
    </div>
  )
}

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex items-baseline justify-between gap-3 py-1">
      <dt className="shrink-0 text-[11.5px] text-subtle">{label}</dt>
      <dd className="min-w-0 truncate text-right text-[12px] text-fg">{children}</dd>
    </div>
  )
}

function ShortcutRow({ keys, label }: { keys: string; label: string }) {
  return (
    <li className="flex items-center justify-between gap-3">
      <span className="min-w-0 truncate">{label}</span>
      <Kbd keys={keys} />
    </li>
  )
}
