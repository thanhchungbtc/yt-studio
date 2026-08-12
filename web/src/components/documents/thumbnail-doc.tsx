import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, Copy, RotateCcw, Trash2, Undo2, Upload } from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import type { ThumbnailCanvasHandle } from './thumbnail/canvas'
import { ThumbnailCanvas } from './thumbnail/canvas'
import { Button, Field, Input, Select } from '../ui/controls'
import {
  Divider,
  EmptyState,
  ErrorNotice,
  Panel,
  PanelHeader,
  PanelTitle,
  Skeleton,
} from '../ui/primitives'
import { DocFrame } from '../editor/doc-frame'
import { api, assetUrl, qk } from '@/core/api'
import type { Design, DesignElement, TextElement, TileElement } from '@/core/thumbnail/doc'
import { FRAME_HEIGHT, FRAME_WIDTH, isText, isTile, readDesign } from '@/core/thumbnail/doc'
import { fontFamilyOf, resourceUrl, useFont, useImages } from '@/core/thumbnail/loading'
import { imageUrls, measureTracked } from '@/core/thumbnail/render'
import { seedDesign } from '@/core/thumbnail/seed'
import type { Video } from '@/core/types'

/** YouTube's ceiling, mirrored from entity.MaxThumbnailBytes so the operator
 *  sees the budget while they work rather than at the moment Apply is refused. */
const MAX_BYTES = 2 * 1024 * 1024

const AUTOSAVE_MS = 800

/**
 * The thumbnail editor, unchanged apart from what tied it to the old shell: the
 * ref arrives as a prop instead of from the URL, and the tab strip is how you
 * leave rather than a back button in a toolbar.
 */
export function ThumbnailDoc({ videoRef: ref }: { videoRef: string }) {
  const queryClient = useQueryClient()

  const video = useQuery({ queryKey: qk.video(ref), queryFn: () => api.getVideo(ref) })
  const settings = useQuery({ queryKey: qk.settings, queryFn: () => api.listSettings() })

  const fontSetting = settings.data?.find((s) => s.key === 'thumbnail.font')
  const rowsSetting = settings.data?.find((s) => s.key === 'thumbnail.grid.rows')
  const fontOptions = fontSetting?.suggestions?.map((s) => s.value) ?? []

  const [design, setDesign] = useState<Design | undefined>()
  const [selectedId, setSelectedId] = useState<string | undefined>()
  const [history, setHistory] = useState<Design[]>([])
  const [status, setStatus] = useState<string | undefined>()
  const [error, setError] = useState<string | undefined>()
  const canvasRef = useRef<ThumbnailCanvasHandle>(null)

  const font = useFont(design?.font ?? fontSetting?.value ?? 'CabinSketch-Bold.ttf')

  /* --------------------------------------------------------- open the doc */

  // Seeding needs the real face: the fitted headline size falls out of how wide
  // the words actually measure, and the fallback face would size it wrong.
  const seedFrom = useCallback((v: Video, fontFile: string, rows: number): Design | undefined => {
    const canvas = document.createElement('canvas')
    const ctx = canvas.getContext('2d')
    if (!ctx) return undefined
    const family = fontFamilyOf(fontFile)
    return seedDesign(
      {
        headline: v.metadata?.thumbnailText || v.title,
        cells: v.thumbnailPlan.map((cell, i) => ({
          caption: cell.caption,
          assetId: v.thumbnailIconIds[i] || undefined,
        })),
        rows,
        font: fontFile,
      },
      (text, size, tracking) => {
        ctx.font = `${size}px "${family}", sans-serif`
        return measureTracked(ctx, text, tracking)
      },
    )
  }, [])

  // Opened once per video: a saved document if there is one, otherwise the
  // builtin renderer's own layout, so the operator starts by adjusting
  // something real rather than facing an empty frame.
  const loadedFor = useRef<string | undefined>(undefined)
  useEffect(() => {
    const v = video.data
    // Waits for the typeface to settle rather than to succeed: seeding measures
    // with the real face, but a face that will not load must still open the
    // editor in a fallback rather than leaving an empty frame.
    if (!v || !font.settled || loadedFor.current === v.id) return
    if (!settings.data) return
    loadedFor.current = v.id
    const stored = readDesign(v.thumbnailDesign)
    setDesign(
      stored ??
        seedFrom(v, fontSetting?.value ?? 'CabinSketch-Bold.ttf', Number(rowsSetting?.value ?? 2)),
    )
  }, [video.data, settings.data, font.settled, seedFrom, fontSetting?.value, rowsSetting?.value])

  /* ------------------------------------------------------------- editing */

  const urls = useMemo(
    () => (design ? imageUrls(design, (id) => assetUrl(id) ?? '', resourceUrl) : []),
    [design],
  )
  const { images, generation } = useImages(urls)

  const selected = design?.elements.find((el) => el.id === selectedId)

  // Pushed once per gesture rather than per pointer move, so one drag is one
  // undo step.
  const pushHistory = useCallback(() => {
    setHistory((past) => (design ? [...past.slice(-49), design] : past))
  }, [design])

  const edit = useCallback(
    (patch: Partial<DesignElement>) => {
      if (!design || !selectedId) return
      pushHistory()
      setDesign({
        ...design,
        elements: design.elements.map((el) =>
          el.id === selectedId ? ({ ...el, ...patch } as DesignElement) : el,
        ),
      })
    },
    [design, selectedId, pushHistory],
  )

  const undo = useCallback(() => {
    setHistory((past) => {
      const previous = past[past.length - 1]
      if (previous) setDesign(previous)
      return past.slice(0, -1)
    })
  }, [])

  /* ------------------------------------------------------------ autosave */

  const saveDesign = useMutation({
    mutationFn: (doc: Design) => api.saveThumbnailDesign(ref, doc),
    onSuccess: (updated) => {
      // The document is written back into the cache so leaving and returning
      // reopens what was built, without a refetch.
      queryClient.setQueryData(qk.video(ref), updated)
    },
  })

  useEffect(() => {
    if (!design || !loadedFor.current) return
    const timer = window.setTimeout(() => {
      saveDesign.mutate(design)
    }, AUTOSAVE_MS)
    return () => window.clearTimeout(timer)
    // saveDesign is a stable mutation object; depending on it would restart the
    // timer on every status change and never save.
  }, [design])

  /* --------------------------------------------------------------- apply */

  const apply = useMutation({
    mutationFn: async () => {
      const blob = await canvasRef.current?.toBlob()
      if (!blob) throw new Error('the canvas produced no image')
      if (blob.size > MAX_BYTES) {
        throw new Error(
          `the image is ${(blob.size / 1024 / 1024).toFixed(2)} MB, over YouTube's 2 MB limit`,
        )
      }
      return api.applyThumbnailOverride(ref, blob)
    },
    onSuccess: (updated) => {
      queryClient.setQueryData(qk.video(ref), updated)
      setError(undefined)
      setStatus('Applied. This is now the thumbnail this video publishes with.')
    },
    onError: (err: Error) => {
      setStatus(undefined)
      setError(err.message)
    },
  })

  const revert = useMutation({
    mutationFn: () => api.clearThumbnailOverride(ref),
    onSuccess: (updated) => {
      queryClient.setQueryData(qk.video(ref), updated)
      setError(undefined)
      setStatus('Reverted to the rendered thumbnail. Your design is kept.')
    },
  })

  /* ------------------------------------------------------------ keyboard */

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null
      if (target && /^(INPUT|TEXTAREA|SELECT)$/.test(target.tagName)) return
      if (!design || !selectedId) return

      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'z') {
        event.preventDefault()
        undo()
        return
      }
      const step = event.shiftKey ? 10 : 1
      const nudge: Record<string, [number, number]> = {
        ArrowLeft: [-step, 0],
        ArrowRight: [step, 0],
        ArrowUp: [0, -step],
        ArrowDown: [0, step],
      }
      const delta = nudge[event.key]
      if (delta) {
        event.preventDefault()
        const el = design.elements.find((e) => e.id === selectedId)
        if (el) edit({ x: el.x + delta[0], y: el.y + delta[1] })
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [design, selectedId, edit, undo])

  /* ---------------------------------------------------------------- view */

  if (video.isLoading || settings.isLoading) {
    return (
      <DocFrame crumbs={[ref, 'Thumbnail']}>
        <div className="p-4">
          <Skeleton className="h-9 w-full" />
        </div>
      </DocFrame>
    )
  }
  if (video.error) {
    return (
      <DocFrame crumbs={[ref, 'Thumbnail']}>
        <ErrorNotice error={video.error} className="m-4" />
      </DocFrame>
    )
  }
  const v = video.data
  if (!v) {
    return (
      <DocFrame crumbs={[ref, 'Thumbnail']}>
        <EmptyState title="No such video" />
      </DocFrame>
    )
  }

  const hasOverride = Boolean(v.thumbnailOverrideAssetId)

  return (
    <DocFrame
      crumbs={[
        <span key="ref" className="font-mono font-semibold text-[hsl(var(--accent))]">
          {v.ref}
        </span>,
        'Thumbnail',
      ]}
      actions={
        <>
          {font.failed && (
            <span className="text-[11px] text-[hsl(var(--danger))]">
              {design?.font} did not load — laying out in a fallback face
            </span>
          )}
          <span className="text-[11px] text-subtle">
            {saveDesign.isPending ? 'Saving…' : 'Saved'}
          </span>
          <Button variant="ghost" size="xs" onClick={undo} disabled={history.length === 0}>
            <Undo2 className="h-3 w-3" />
            Undo
          </Button>
          {hasOverride && (
            <Button
              variant="outline"
              size="xs"
              onClick={() => revert.mutate()}
              disabled={revert.isPending}
            >
              <RotateCcw className="h-3 w-3" />
              Use rendered
            </Button>
          )}
          <Button
            variant="primary"
            size="xs"
            onClick={() => apply.mutate()}
            disabled={apply.isPending || !design}
          >
            <Upload className="h-3 w-3" />
            {apply.isPending ? 'Applying…' : 'Use this thumbnail'}
          </Button>
        </>
      }
    >
      <div className="flex h-full min-h-0 overflow-hidden">
        <div className="min-w-0 flex-1 overflow-auto p-4">
          {status && (
            <div className="mb-3 rounded-[var(--radius-sm)] border border-[hsl(var(--success))] bg-[hsl(var(--success))]/10 px-3 py-2 text-[12px]">
              <Check className="mr-1.5 inline h-3.5 w-3.5" />
              {status}
            </div>
          )}
          {error && <ErrorNotice error={error} className="mb-3" />}

          {design ? (
            <ThumbnailCanvas
              ref={canvasRef}
              design={design}
              images={images}
              generation={generation}
              fontFamily={font.family}
              selectedId={selectedId}
              onSelect={setSelectedId}
              onChange={setDesign}
              onCommit={pushHistory}
              className="mx-auto max-w-[960px]"
            />
          ) : (
            <Skeleton className="mx-auto aspect-video w-full max-w-[960px]" />
          )}

          {/* A video whose grid has not been planned yet opens on a headline
              and nothing else. That is correct, and it looks exactly like a
              broken screen, so it says which one it is. */}
          {v.thumbnailPlan.length === 0 && (
            <p className="mx-auto mt-3 max-w-[960px] rounded-[var(--radius-sm)] border border-[hsl(var(--border-strong))] bg-subtle px-3 py-2 text-[11.5px] leading-relaxed text-muted">
              This video has no thumbnail grid yet — only the headline is on the frame. The cells
              and their icons appear here once the thumbnail plan and icon tasks have run, and{' '}
              <strong>Reset to the rendered layout</strong> will lay them out. You can still design
              against the backdrop in the meantime.
            </p>
          )}

          <p className="mx-auto mt-3 max-w-[960px] text-[11px] leading-relaxed text-subtle">
            The frame is {FRAME_WIDTH}×{FRAME_HEIGHT}, exactly what gets uploaded. Drag to move,
            corner grips to resize, shift while resizing to keep the aspect, arrow keys to nudge.
            {hasOverride
              ? ' This video is publishing your image; the rendered one is kept and can be restored.'
              : ' This video is publishing the rendered thumbnail until you apply one here.'}
          </p>
        </div>

        <aside className="w-[300px] shrink-0 overflow-auto border-l border-[hsl(var(--border))] p-3">
          <Inspector
            design={design}
            selected={selected}
            fontOptions={fontOptions}
            onEdit={edit}
            onDesign={(next) => {
              pushHistory()
              setDesign(next)
            }}
            onReseed={() => {
              pushHistory()
              setDesign(
                seedFrom(
                  v,
                  fontSetting?.value ?? 'CabinSketch-Bold.ttf',
                  Number(rowsSetting?.value ?? 2),
                ),
              )
              setSelectedId(undefined)
            }}
          />
        </aside>
      </div>
    </DocFrame>
  )
}

/* ---------------------------------------------------------------- panels */

interface InspectorProps {
  design: Design | undefined
  selected: DesignElement | undefined
  fontOptions: string[]
  onEdit: (patch: Partial<DesignElement>) => void
  onDesign: (design: Design) => void
  onReseed: () => void
}

function Inspector({ design, selected, fontOptions, onEdit, onDesign, onReseed }: InspectorProps) {
  if (!design) return null
  return (
    <div className="space-y-3">
      <Panel>
        <PanelHeader>
          <PanelTitle>Frame</PanelTitle>
        </PanelHeader>
        <div className="space-y-2 p-3">
          <Field label="Typeface">
            {(id) => (
              <Select
                id={id}
                value={design.font}
                onChange={(e) => onDesign({ ...design, font: e.target.value })}
              >
                {[design.font, ...fontOptions.filter((f) => f !== design.font)].map((f) => (
                  <option key={f} value={f}>
                    {f}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          <NumberRow
            label="Backdrop brightness"
            hint="Lower is darker. White type over an undimmed photograph is unreadable."
            value={design.scrim}
            min={0}
            max={255}
            onChange={(scrim) => onDesign({ ...design, scrim })}
          />
          <Button variant="outline" size="sm" className="w-full" onClick={onReseed}>
            <Copy className="h-3.5 w-3.5" />
            Reset to the rendered layout
          </Button>
        </div>
      </Panel>

      <Panel>
        <PanelHeader>
          <PanelTitle>{selected ? describe(selected) : 'Nothing selected'}</PanelTitle>
        </PanelHeader>
        <div className="space-y-2 p-3">
          {!selected && (
            <p className="text-[11px] leading-relaxed text-subtle">
              Click anything on the frame to edit it.
            </p>
          )}
          {selected && (
            <>
              <div className="grid grid-cols-2 gap-2">
                <NumberRow label="X" value={selected.x} onChange={(x) => onEdit({ x })} />
                <NumberRow label="Y" value={selected.y} onChange={(y) => onEdit({ y })} />
                <NumberRow label="Width" value={selected.w} onChange={(w) => onEdit({ w })} />
                <NumberRow label="Height" value={selected.h} onChange={(h) => onEdit({ h })} />
              </div>
              <Divider />
              {isText(selected) && <TextInspector el={selected} onEdit={onEdit} />}
              {isTile(selected) && <TileInspector el={selected} onEdit={onEdit} />}
              <Divider />
              <Button
                variant="danger"
                size="sm"
                className="w-full"
                onClick={() =>
                  onDesign({
                    ...design,
                    elements: design.elements.filter((el) => el.id !== selected.id),
                  })
                }
              >
                <Trash2 className="h-3.5 w-3.5" />
                Remove
              </Button>
            </>
          )}
        </div>
      </Panel>
    </div>
  )
}

function TextInspector({
  el,
  onEdit,
}: {
  el: TextElement
  onEdit: (patch: Partial<DesignElement>) => void
}) {
  return (
    <>
      <Field label="Text">
        {(id) => (
          <Input id={id} value={el.text} onChange={(e) => onEdit({ text: e.target.value })} />
        )}
      </Field>
      <div className="grid grid-cols-2 gap-2">
        <NumberRow label="Size" value={el.fontSize} onChange={(fontSize) => onEdit({ fontSize })} />
        <NumberRow
          label="Tracking"
          value={el.tracking}
          onChange={(tracking) => onEdit({ tracking })}
        />
        <NumberRow
          label="Line gap"
          value={el.lineGap}
          onChange={(lineGap) => onEdit({ lineGap })}
        />
        <NumberRow
          label="Outline"
          value={el.strokeWidth}
          min={0}
          onChange={(strokeWidth) => onEdit({ strokeWidth })}
        />
      </div>
      <Field label="Colour">
        {() => <ColorRow value={el.color} onChange={(color) => onEdit({ color })} />}
      </Field>
      <Field label="Align">
        {(id) => (
          <Select
            id={id}
            value={el.align}
            onChange={(e) => onEdit({ align: e.target.value as TextElement['align'] })}
          >
            <option value="left">Left</option>
            <option value="center">Centre</option>
            <option value="right">Right</option>
          </Select>
        )}
      </Field>
      <label className="flex items-center gap-2 text-[12px]">
        <input
          type="checkbox"
          checked={el.uppercase}
          onChange={(e) => onEdit({ uppercase: e.target.checked })}
        />
        Uppercase
      </label>
      <NumberRow
        label="Shadow blur"
        value={el.shadowBlur}
        min={0}
        onChange={(shadowBlur) => onEdit({ shadowBlur })}
      />
    </>
  )
}

function TileInspector({
  el,
  onEdit,
}: {
  el: TileElement
  onEdit: (patch: Partial<DesignElement>) => void
}) {
  return (
    <>
      <Field label="Caption">
        {(id) => (
          <Input id={id} value={el.caption} onChange={(e) => onEdit({ caption: e.target.value })} />
        )}
      </Field>
      <div className="grid grid-cols-2 gap-2">
        <NumberRow
          label="Caption size"
          value={el.captionSize}
          onChange={(captionSize) => onEdit({ captionSize })}
        />
        <NumberRow
          label="Corner"
          value={el.radius}
          min={0}
          onChange={(radius) => onEdit({ radius })}
        />
        <NumberRow
          label="Border"
          value={el.borderWidth}
          min={0}
          onChange={(borderWidth) => onEdit({ borderWidth })}
        />
        <NumberRow
          label="Padding"
          value={el.padding}
          min={0}
          onChange={(padding) => onEdit({ padding })}
        />
      </div>
      <Field label="Border colour">
        {() => (
          <ColorRow value={el.borderColor} onChange={(borderColor) => onEdit({ borderColor })} />
        )}
      </Field>
      <Divider />
      <p className="text-[11px] leading-relaxed text-subtle">
        Icons arrive as art on a dark field. Everything under the first threshold is dropped,
        everything over the second is kept, and the ramp between them is what keeps the edges from
        going jagged.
      </p>
      <div className="grid grid-cols-2 gap-2">
        <NumberRow
          label="Key below"
          value={el.keyBelow}
          min={0}
          max={255}
          onChange={(keyBelow) => onEdit({ keyBelow })}
        />
        <NumberRow
          label="Key above"
          value={el.keyAbove}
          min={0}
          max={255}
          onChange={(keyAbove) => onEdit({ keyAbove })}
        />
      </div>
    </>
  )
}

function NumberRow({
  label,
  hint,
  value,
  min,
  max,
  onChange,
}: {
  label: string
  hint?: string
  value: number
  min?: number
  max?: number
  onChange: (value: number) => void
}) {
  return (
    <Field label={label} hint={hint}>
      {(id) => (
        <Input
          id={id}
          type="number"
          value={Number.isFinite(value) ? Math.round(value) : 0}
          min={min}
          max={max}
          onChange={(e) => {
            const next = Number(e.target.value)
            if (Number.isFinite(next)) onChange(next)
          }}
        />
      )}
    </Field>
  )
}

/** A swatch beside the literal value: the document holds CSS colours, and rgba
 *  strings cannot be expressed in a native colour input. */
function ColorRow({ value, onChange }: { value: string; onChange: (value: string) => void }) {
  return (
    <div className="flex items-center gap-2">
      <input
        type="color"
        value={/^#[0-9a-f]{6}$/i.test(value) ? value : '#ffffff'}
        onChange={(e) => onChange(e.target.value)}
        className="h-7 w-9 shrink-0 rounded-[var(--radius-xs)] border border-[hsl(var(--border-strong))] bg-transparent"
      />
      <Input value={value} onChange={(e) => onChange(e.target.value)} />
    </div>
  )
}

function describe(el: DesignElement): string {
  switch (el.kind) {
    case 'text':
      return 'Text'
    case 'tile':
      return 'Grid cell'
    case 'image':
      return 'Image'
    default:
      return 'Element'
  }
}
