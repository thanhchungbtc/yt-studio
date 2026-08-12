import { useMutation, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, Pause, Play, RotateCw } from 'lucide-react'
import type { ReactNode } from 'react'
import { useEffect, useRef, useState } from 'react'

import { Tooltip } from '../../ui/primitives'
import { api, assetUrl, qk } from '@/core/api'
import { formatClock } from '@/core/format'
import type { Chapter } from '@/core/types'
import { cn } from '@/core/utils'
import type { Cell } from './stages'

/* ------------------------------------------------------------------ shared */

/**
 * What every unfilled cell draws. Four states, one shape each, identical in
 * every column — so a glance down a column reads as a single picture rather than
 * four columns' worth of separate conventions.
 */
export function Placeholder({ cell, className }: { cell: Cell; className?: string }) {
  switch (cell.state) {
    case 'running':
      return (
        <span
          className={cn(
            'sweep h-4 w-10 rounded-[var(--radius-xs)] bg-[hsl(var(--bg-hover))]',
            className,
          )}
          aria-label="running"
        />
      )
    case 'queued':
      return (
        <span
          className={cn('h-1.5 w-1.5 rounded-full bg-[hsl(var(--info))] pulse-live', className)}
          aria-label="queued"
        />
      )
    case 'failed':
      return null // the caller draws a retry affordance instead
    default:
      return (
        <span className={cn('text-[11px] text-[hsl(var(--fg-subtle))] opacity-50', className)}>
          ·
        </span>
      )
  }
}

/** A failed cell is the one place the table offers to act rather than report. */
function Retry({ cell, videoRef, videoId }: { cell: Cell; videoRef: string; videoId: string }) {
  const queryClient = useQueryClient()
  const retry = useMutation({
    mutationFn: () => api.retryTask(cell.task?.id ?? ''),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: qk.videoTasks(videoId) })
      void queryClient.invalidateQueries({ queryKey: qk.video(videoRef) })
    },
  })

  return (
    <Tooltip label={cell.task?.error ?? 'Failed — run it again'}>
      <button
        type="button"
        aria-label="Run this step again"
        disabled={retry.isPending || !cell.task}
        onClick={(event) => {
          event.stopPropagation()
          retry.mutate()
        }}
        className="flex items-center gap-1 rounded-[var(--radius-xs)] px-1 py-0.5 text-[11px] text-[hsl(var(--danger))] transition-colors hover:bg-[hsl(var(--danger)/0.12)] disabled:opacity-50"
      >
        <AlertTriangle className="h-3 w-3" />
        <RotateCw className={cn('h-3 w-3', retry.isPending && 'animate-spin')} />
      </button>
    </Tooltip>
  )
}

/** The amber flag a stale artifact carries: intact, but its inputs moved. */
function StaleFlag() {
  return (
    <Tooltip label="Stale — an input changed after this ran">
      <span className="text-[10px] leading-none text-[hsl(var(--warning))]">⚑</span>
    </Tooltip>
  )
}

function CellShell({
  cell,
  videoRef,
  videoId,
  children,
}: {
  cell: Cell
  videoRef: string
  videoId: string
  children: ReactNode
}) {
  if (cell.state === 'failed') return <Retry cell={cell} videoRef={videoRef} videoId={videoId} />
  if (cell.state !== 'done') return <Placeholder cell={cell} />
  return (
    <span className="flex items-center gap-1">
      {children}
      {cell.stale && <StaleFlag />}
    </span>
  )
}

/* ------------------------------------------------------------------ script */

/**
 * Words written, against what the blueprint budgeted in the column to its left.
 * No tick: a word count already means the script exists.
 */
export function ScriptCell({
  cell,
  words,
  videoRef,
  videoId,
  onOpen,
}: {
  cell: Cell
  words: number
  videoRef: string
  videoId: string
  onOpen: () => void
}) {
  if (cell.state !== 'done') {
    return (
      <div className="flex items-center">
        <CellShell cell={cell} videoRef={videoRef} videoId={videoId}>
          {null}
        </CellShell>
      </div>
    )
  }

  return (
    <button
      type="button"
      onClick={onOpen}
      data-script-words={words}
      className="flex items-center gap-1 rounded-[var(--radius-xs)] px-1 py-0.5 text-[11.5px] text-fg transition-colors hover:bg-[hsl(var(--bg-hover))]"
    >
      <span className="tabular">{words}w</span>
      {cell.stale && <StaleFlag />}
    </button>
  )
}

/* --------------------------------------------------------------- narration */

/**
 * Plays where it sits. Hearing twelve seconds of a chapter should not cost a
 * navigation, and the duration is the one number the cell has to report anyway.
 */
export function NarrationCell({
  cell,
  assetId,
  seconds,
  videoRef,
  videoId,
}: {
  cell: Cell
  assetId: string | undefined
  seconds: number
  videoRef: string
  videoId: string
}) {
  const [playing, setPlaying] = useState(false)
  const audio = useRef<HTMLAudioElement | null>(null)

  useEffect(() => {
    return () => {
      audio.current?.pause()
      audio.current = null
    }
  }, [])

  const toggle = () => {
    if (!assetId) return
    if (!audio.current) {
      audio.current = new Audio(assetUrl(assetId))
      audio.current.addEventListener('ended', () => setPlaying(false))
    }
    if (playing) {
      audio.current.pause()
      setPlaying(false)
    } else {
      void audio.current.play()
      setPlaying(true)
    }
  }

  return (
    <CellShell cell={cell} videoRef={videoRef} videoId={videoId}>
      <button
        type="button"
        onClick={toggle}
        aria-label={playing ? 'Pause the narration' : 'Play the narration'}
        className="flex items-center gap-1.5 rounded-[var(--radius-xs)] px-1 py-0.5 text-[11.5px] text-fg transition-colors hover:bg-[hsl(var(--bg-hover))]"
      >
        {playing ? (
          <Pause className="h-3 w-3 text-[hsl(var(--accent))]" />
        ) : (
          <Play className="h-3 w-3 text-subtle" />
        )}
        <span className="tabular">{seconds > 0 ? formatClock(seconds) : '—'}</span>
      </button>
    </CellShell>
  )
}

/* ------------------------------------------------------------------ slides */

/**
 * The pictures themselves, at whatever size the column can afford. Slots the
 * pipeline has not filled are drawn as dashed outlines at their eventual count,
 * so the cell has its final width from the moment the blueprint lands.
 */
export function SlidesCell({
  chapter,
  cells,
  thumbWidth,
  onOpenSlide,
}: {
  chapter: Chapter
  cells: Cell[]
  thumbWidth: number | null
  onOpenSlide: (slot: number) => void
}) {
  // Beyond four slots a thumbnail is smaller than it is legible, so the cell
  // stops pretending to be a contact sheet and becomes a progress strip.
  if (thumbWidth === null) {
    return (
      <div className="flex flex-wrap items-center gap-[3px]">
        {cells.map((cell, slot) => (
          <Tooltip key={slot} label={chapter.slidePrompts[slot] ?? `Slide ${slot + 1}`}>
            <button
              type="button"
              aria-label={`Slide ${slot + 1}`}
              onClick={() => onOpenSlide(slot)}
              className={cn(
                'h-3 w-3 rounded-[2px] transition-transform hover:scale-125',
                cell.state === 'done' && !cell.stale && 'bg-[hsl(var(--success))]',
                cell.state === 'done' && cell.stale && 'bg-[hsl(var(--warning))]',
                cell.state === 'running' && 'bg-[hsl(var(--accent))] pulse-live',
                cell.state === 'queued' && 'bg-[hsl(var(--info))]',
                cell.state === 'failed' && 'bg-[hsl(var(--danger))]',
                cell.state === 'pending' && 'bg-[hsl(var(--fg)/0.12)]',
              )}
            />
          </Tooltip>
        ))}
      </div>
    )
  }

  const height = Math.round((thumbWidth * 9) / 16)

  return (
    <div className="flex items-center gap-1">
      {cells.map((cell, slot) => {
        const assetId = chapter.slideAssetIds[slot]
        const prompt = chapter.slidePrompts[slot]
        return (
          <Tooltip key={slot} label={prompt || `Slide ${slot + 1} — no prompt yet`}>
            <button
              type="button"
              aria-label={`Slide ${slot + 1}`}
              onClick={() => onOpenSlide(slot)}
              style={{ width: thumbWidth, height }}
              className={cn(
                'relative shrink-0 overflow-hidden rounded-[var(--radius-xs)] transition-shadow',
                cell.state === 'done'
                  ? 'checker ring-1 ring-[hsl(var(--border))] hover:ring-[hsl(var(--accent))]'
                  : 'border border-dashed border-[hsl(var(--border-strong))]',
              )}
            >
              {cell.state === 'done' && assetId && (
                <img
                  src={assetUrl(assetId)}
                  alt={`Slide ${slot + 1}`}
                  loading="lazy"
                  className="h-full w-full object-cover"
                />
              )}
              {cell.state === 'running' && <span className="sweep absolute inset-0" />}
              {cell.state === 'failed' && (
                <span className="absolute inset-0 flex items-center justify-center bg-[hsl(var(--danger-soft))]">
                  <AlertTriangle className="h-3 w-3 text-[hsl(var(--danger))]" />
                </span>
              )}
              {cell.stale && (
                <span className="absolute right-0 top-0 h-0 w-0 border-l-[10px] border-t-[10px] border-l-transparent border-t-[hsl(var(--warning))]" />
              )}
            </button>
          </Tooltip>
        )
      })}
    </div>
  )
}

/* -------------------------------------------------------------------- clip */

/**
 * The composed chapter. No duration and no poster frame: the duration is the
 * narration's, printed one column to the left, and the poster would be the first
 * slide, printed one column to the right. All this cell has to say is whether
 * the compose step happened, and let you watch it.
 */
export function ClipCell({
  cell,
  videoRef,
  videoId,
  onOpen,
}: {
  cell: Cell
  videoRef: string
  videoId: string
  onOpen: () => void
}) {
  return (
    <CellShell cell={cell} videoRef={videoRef} videoId={videoId}>
      <Tooltip label="Play the composed chapter">
        <button
          type="button"
          aria-label="Play the composed chapter"
          onClick={onOpen}
          className="flex h-5 w-5 items-center justify-center rounded-[var(--radius-xs)] text-[hsl(var(--success))] transition-colors hover:bg-[hsl(var(--bg-hover))]"
        >
          <Play className="h-3.5 w-3.5" />
        </button>
      </Tooltip>
    </CellShell>
  )
}
