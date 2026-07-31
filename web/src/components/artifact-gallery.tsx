import type { ReactNode } from 'react'
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import {
  ArrowDownWideNarrow,
  Copy,
  Download,
  ExternalLink,
  Eye,
  Image as ImageIcon,
  LayoutGrid,
  ListTree,
  Maximize2,
  RefreshCw,
  Rows3,
} from 'lucide-react'

import { AssetPreview, assetKindTone, useAssetViewer } from '@/components/asset-viewer'
import { RerunDialog } from '@/components/stale'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  ContextMenu,
  ContextMenuItem,
  ContextMenuLabel,
  ContextMenuSeparator,
} from '@/components/ui/menu'
import {
  Divider,
  EmptyState,
  FilterChip,
  Mono,
  SearchField,
  Segmented,
  Skeleton,
  Tooltip,
} from '@/components/ui/primitives'
import { assetUrl } from '@/lib/api'
import { downloadName, kindTitle, mediaTypeOf, shortId } from '@/lib/assets'
import type { ViewerItem } from '@/lib/assets'
import { formatBytes, formatRelative } from '@/lib/format'
import { useHotkeys } from '@/lib/hotkeys'
import type { Video } from '@/lib/types'
import { cn } from '@/lib/utils'
import { usePersisted } from '@/lib/workspace'

/*
  The gallery.

  Every artifact a video has produced, as pictures rather than as hashes. A hash
  is the right *identity* for a content-addressed store and the wrong thing to
  show an operator asking whether the stills came out well — so the image leads
  and the address moves into the viewer.

  At fifty chapters this is several hundred images, which is why the grid is
  virtualised by row: the browser is asked for the dozen thumbnails on screen,
  not for four hundred at once. The column count is measured rather than assumed,
  because the pane is behind a splitter and its width is the operator's choice.
*/

type View = 'grid' | 'list'
type GroupBy = 'chapter' | 'none'
type SortKey = 'pipeline' | 'newest' | 'largest'
type Density = 's' | 'm' | 'l'

const TILE_WIDTH: Record<Density, number> = { s: 132, m: 178, l: 248 }
const GAP = 10
const PAD = 12
const GROUP_ROW = 30
const LIST_ROW = 42
/**
 * The caption under a tile: its border, then the title, the subtitle and the
 * address line at fixed leading. Fixed rather than measured, because the
 * virtualiser needs a row height before the row exists — so the card is built to
 * the number rather than the number guessed from the card.
 *
 * Under a group header the subtitle is gone — the header has already said which
 * chapter this is — and the tile is a line shorter.
 */
const CARD_CHROME = 59
const CARD_CHROME_TIGHT = 44

type Row =
  | { type: 'group'; key: string; label: string; count: number; bytes: number }
  | { type: 'grid'; key: string; items: { item: ViewerItem; index: number }[] }
  | { type: 'list'; key: string; item: ViewerItem; index: number }

export function ArtifactGallery({
  video,
  items,
  loading,
}: {
  video: Video
  /** Every artifact, already joined to its chapter and producing task. */
  items: ViewerItem[]
  loading: boolean
}) {
  const openViewer = useAssetViewer()

  const [query, setQuery] = useState('')
  const [kind, setKind] = usePersisted<string>('video.artifacts.kind', 'all')
  const [view, setView] = usePersisted<View>('video.artifacts.view', 'grid')
  const [group, setGroup] = usePersisted<GroupBy>('video.artifacts.group', 'chapter')
  const [sort, setSort] = usePersisted<SortKey>('video.artifacts.sort', 'pipeline')
  const [density, setDensity] = usePersisted<Density>('video.artifacts.density', 'm')
  const [rerunning, setRerunning] = useState<ViewerItem | null>(null)

  const searchRef = useRef<HTMLInputElement>(null)
  const scrollRef = useRef<HTMLDivElement>(null)

  useHotkeys([
    {
      keys: '/',
      label: 'Search the artifacts',
      group: 'Video',
      run: () => searchRef.current?.focus(),
    },
  ])

  const kinds = useMemo(() => {
    const counts = new Map<string, number>()
    for (const item of items) counts.set(item.kind, (counts.get(item.kind) ?? 0) + 1)
    return [...counts.entries()].sort((a, b) => a[0].localeCompare(b[0]))
  }, [items])

  // A filter that outlives the artifact it matched leaves an empty pane with no
  // explanation, so it collapses back to "all" the moment its kind disappears.
  const activeKind = kind !== 'all' && kinds.some(([name]) => name === kind) ? kind : 'all'

  const search = query.trim().toLowerCase()
  const shown = useMemo(() => {
    const filtered = items.filter((item) => {
      if (activeKind !== 'all' && item.kind !== activeKind) return false
      if (!search) return true
      return `${item.title} ${item.subtitle ?? ''} ${item.kind} ${item.mime} ${item.id}`
        .toLowerCase()
        .includes(search)
    })
    if (sort === 'pipeline') return filtered
    const sorted = [...filtered]
    sorted.sort(
      sort === 'newest'
        ? (a, b) => (b.createdAt ?? '').localeCompare(a.createdAt ?? '')
        : (a, b) => (b.size ?? 0) - (a.size ?? 0),
    )
    return sorted
  }, [items, activeKind, search, sort])

  const shownBytes = shown.reduce((sum, item) => sum + (item.size ?? 0), 0)

  /* ------------------------------------------------- measured grid geometry */

  const [width, setWidth] = useState(0)
  useLayoutEffect(() => {
    const node = scrollRef.current
    if (!node) return
    const observer = new ResizeObserver(([entry]) => {
      if (entry) setWidth(entry.contentRect.width)
    })
    observer.observe(node)
    setWidth(node.clientWidth)
    return () => observer.disconnect()
  }, [])

  // Grouping is by chapter and only ever by chapter: it is the only axis along
  // which an operator reviews a video, and the kind axis is already the filter.
  // Sorting by size or age crosses chapters, so it puts the groups away.
  const grouped = group === 'chapter' && sort === 'pipeline'

  const tile = TILE_WIDTH[density]
  const inner = Math.max(0, width - PAD * 2)
  const columns = Math.max(1, Math.floor((inner + GAP) / (tile + GAP)))
  const cellWidth = columns > 0 ? (inner - GAP * (columns - 1)) / columns : tile
  // 16:10 media plus the caption block, which is a fixed height at every density.
  const chrome = grouped ? CARD_CHROME_TIGHT : CARD_CHROME
  const gridRowHeight = Math.round((cellWidth * 10) / 16) + chrome + GAP

  const rows = useMemo<Row[]>(() => {
    const indexed = shown.map((item, index) => ({ item, index }))
    const perRow = view === 'grid' ? columns : 1
    const emit = (cells: { item: ViewerItem; index: number }[], keyPrefix: string): Row[] => {
      if (view === 'list') {
        return cells.map(({ item, index }) => ({
          type: 'list',
          key: `${item.id}:${index}`,
          item,
          index,
        }))
      }
      const out: Row[] = []
      for (let i = 0; i < cells.length; i += perRow) {
        out.push({ type: 'grid', key: `${keyPrefix}:${i}`, items: cells.slice(i, i + perRow) })
      }
      return out
    }

    if (!grouped) return emit(indexed, 'r')

    const groups = new Map<number, { label: string; cells: typeof indexed }>()
    for (const cell of indexed) {
      const ordinal = cell.item.ordinal ?? 0
      const entry = groups.get(ordinal) ?? {
        label: ordinal > 0 ? (cell.item.subtitle ?? `Chapter ${ordinal}`) : 'Whole video',
        cells: [],
      }
      entry.cells.push(cell)
      groups.set(ordinal, entry)
    }

    const out: Row[] = []
    for (const [ordinal, entry] of [...groups.entries()].sort((a, b) => a[0] - b[0])) {
      out.push({
        type: 'group',
        key: `g:${ordinal}`,
        label: entry.label,
        count: entry.cells.length,
        bytes: entry.cells.reduce((sum, cell) => sum + (cell.item.size ?? 0), 0),
      })
      out.push(...emit(entry.cells, `g:${ordinal}`))
    }
    return out
  }, [shown, grouped, view, columns])

  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: (index) => {
      const row = rows[index]
      if (!row) return LIST_ROW
      if (row.type === 'group') return GROUP_ROW
      return row.type === 'grid' ? gridRowHeight : LIST_ROW
    },
    getItemKey: (index) => rows[index]?.key ?? index,
    overscan: 6,
  })

  // Row heights here are computed, not measured — a wider pane, a bigger tile or
  // a switch to the list changes every one of them at once, and the cached
  // measurements have to be thrown away with them.
  useEffect(() => {
    virtualizer.measure()
  }, [virtualizer, gridRowHeight, view, grouped])

  /* ------------------------------------------------------------------ chrome */

  if (loading) {
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
        icon={<ImageIcon />}
        title="No artifacts yet"
        description={`Artifacts appear as tasks complete. ${video.ref} has produced none so far.`}
      />
    )
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex shrink-0 flex-wrap items-center gap-2 border-b border-[hsl(var(--border))] bg-subtle px-3 py-2 no-select">
        <SearchField
          value={query}
          onChange={setQuery}
          inputRef={searchRef}
          placeholder="Search artifacts"
          keys="/"
          className="w-[200px]"
        />
        <Divider />
        <div className="flex min-w-0 flex-wrap items-center gap-1.5">
          <FilterChip
            label="All"
            count={items.length}
            selected={activeKind === 'all'}
            onClick={() => setKind('all')}
          />
          {kinds.map(([name, count]) => (
            <FilterChip
              key={name}
              label={kindTitle(name)}
              count={count}
              tone={assetKindTone(name)}
              selected={activeKind === name}
              onClick={() => setKind(name)}
            />
          ))}
        </div>

        <div className="ml-auto flex shrink-0 items-center gap-2">
          <Tooltip
            label={
              sort === 'pipeline'
                ? 'In pipeline order — chapter by chapter'
                : sort === 'newest'
                  ? 'Newest first'
                  : 'Largest first'
            }
          >
            {/* The span is the tooltip's trigger: `Segmented` is a plain
                component and cannot take the ref Radix would hand it. */}
            <span>
              <Segmented
                aria-label="Sort artifacts"
                value={sort}
                onChange={setSort}
                options={[
                  { value: 'pipeline', label: <ListTree className="h-3.5 w-3.5" /> },
                  { value: 'newest', label: <span className="text-[11px]">new</span> },
                  { value: 'largest', label: <ArrowDownWideNarrow className="h-3.5 w-3.5" /> },
                ]}
                className="w-[104px]"
              />
            </span>
          </Tooltip>
          {view === 'grid' && (
            <Segmented
              aria-label="Tile size"
              value={density}
              onChange={setDensity}
              options={[
                { value: 's', label: <span className="text-[11px]">S</span> },
                { value: 'm', label: <span className="text-[11px]">M</span> },
                { value: 'l', label: <span className="text-[11px]">L</span> },
              ]}
              className="w-[84px]"
            />
          )}
          <Segmented
            aria-label="Artifact layout"
            value={view}
            onChange={setView}
            options={[
              { value: 'grid', label: <LayoutGrid className="h-3.5 w-3.5" /> },
              { value: 'list', label: <Rows3 className="h-3.5 w-3.5" /> },
            ]}
            className="w-[72px]"
          />
        </div>
      </div>

      {/* The scroller is always mounted: it is what the width is measured from,
          and an empty state that replaces it would take the measurement with it
          and leave the grid one render behind on the way back. */}
      <div ref={scrollRef} className="relative min-h-0 flex-1 overflow-y-auto py-3">
        {shown.length === 0 ? (
          <EmptyState
            title="Nothing matches"
            description="No artifact matches the filter and the search together."
            action={
              <Button
                size="sm"
                variant="outline"
                onClick={() => {
                  setQuery('')
                  setKind('all')
                }}
              >
                Clear the filters
              </Button>
            }
          />
        ) : (
          <div className="relative w-full" style={{ height: virtualizer.getTotalSize() }}>
            {virtualizer.getVirtualItems().map((virtual) => {
              const row = rows[virtual.index]
              if (!row) return null
              return (
                <div
                  key={virtual.key}
                  className="absolute left-0 w-full px-3"
                  style={{ height: virtual.size, transform: `translateY(${virtual.start}px)` }}
                >
                  {row.type === 'group' && (
                    <GroupHeader label={row.label} count={row.count} bytes={row.bytes} />
                  )}
                  {row.type === 'grid' && (
                    <div className="flex" style={{ gap: GAP }}>
                      {row.items.map(({ item, index }) => (
                        <ArtifactCard
                          key={`${item.id}:${index}`}
                          item={item}
                          width={cellWidth}
                          subtitle={!grouped}
                          onOpen={() => openViewer(shown, index)}
                          onRerun={() => setRerunning(item)}
                        />
                      ))}
                    </div>
                  )}
                  {row.type === 'list' && (
                    <ArtifactRow
                      item={row.item}
                      onOpen={() => openViewer(shown, row.index)}
                      onRerun={() => setRerunning(row.item)}
                    />
                  )}
                </div>
              )
            })}
          </div>
        )}
      </div>

      <div className="flex h-[24px] shrink-0 items-center gap-3 border-t border-[hsl(var(--border))] bg-subtle px-3 text-[11px] text-subtle no-select">
        <span className="tabular">
          {shown.length === items.length
            ? `${items.length} artifacts`
            : `${shown.length} of ${items.length} artifacts`}
          {' · '}
          {formatBytes(shownBytes)}
        </span>
        <Tooltip
          label={
            sort === 'pipeline'
              ? 'Group the grid by chapter, or lay it out flat'
              : 'Sorting by size or age crosses chapters, so the groups step aside'
          }
        >
          <button
            type="button"
            onClick={() =>
              sort === 'pipeline' && setGroup(group === 'chapter' ? 'none' : 'chapter')
            }
            aria-pressed={group === 'chapter'}
            aria-disabled={sort !== 'pipeline'}
            className={cn(
              'ml-auto flex items-center gap-1.5 rounded-[var(--radius-xs)] px-1.5 py-0.5 transition-colors hover:text-fg',
              grouped && 'text-fg',
              sort !== 'pipeline' && 'opacity-40',
            )}
          >
            <ListTree className="h-3 w-3" />
            {sort === 'pipeline'
              ? grouped
                ? 'Grouped by chapter'
                : 'Flat'
              : 'Grouping is off while sorted'}
          </button>
        </Tooltip>
      </div>

      {rerunning?.taskId && (
        <RerunDialog
          open
          onOpenChange={(open) => {
            if (!open) setRerunning(null)
          }}
          videoRef={video.ref}
          videoId={video.id}
          taskIds={[rerunning.taskId]}
          what={rerunning.title.toLowerCase()}
        />
      )}
    </div>
  )
}

/* ------------------------------------------------------------------- pieces */

function GroupHeader({ label, count, bytes }: { label: string; count: number; bytes: number }) {
  return (
    <div className="flex h-full items-center gap-2 no-select">
      <h3 className="truncate text-[11.5px] font-semibold text-fg">{label}</h3>
      <span className="tabular shrink-0 text-[10.5px] text-subtle">
        {count} · {formatBytes(bytes)}
      </span>
      <span className="h-px min-w-4 flex-1 bg-[hsl(var(--border))]" aria-hidden />
    </div>
  )
}

/**
 * One artifact as a tile. The card is a plain element rather than a button so
 * the download and raw-file affordances can be real anchors on top of it — a
 * link nested in a button is not a link at all.
 */
function ArtifactCard({
  item,
  width,
  subtitle,
  onOpen,
  onRerun,
}: {
  item: ViewerItem
  width: number
  /** Dropped under a group header, which has already named the chapter. */
  subtitle: boolean
  onOpen: () => void
  onRerun: () => void
}) {
  const media = mediaTypeOf(item.mime)
  return (
    <ItemMenu item={item} onOpen={onOpen} onRerun={onRerun}>
      <div
        className="group surface relative overflow-hidden transition-[border-color,box-shadow] hover:border-[hsl(var(--accent))] hover:elev-2"
        style={{ width }}
      >
        <button
          type="button"
          onClick={onOpen}
          aria-label={`Preview ${item.title}`}
          className="block w-full text-left"
        >
          <span className="relative block aspect-[16/10] overflow-hidden border-b border-[hsl(var(--border))]">
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
          </span>
          <span className={cn('block px-2.5 py-1.5', subtitle ? 'h-[58px]' : 'h-[43px]')}>
            <span className="block truncate text-[12px] font-medium leading-[16px] text-fg">
              {item.title}
            </span>
            {subtitle && (
              <span className="block truncate text-[11px] leading-[15px] text-subtle">
                {item.subtitle}
              </span>
            )}
            <span className="flex items-center justify-between gap-2 text-[10.5px] leading-[15px] text-subtle">
              <Mono className="truncate text-[10.5px] leading-[15px]">{shortId(item.id)}</Mono>
              <span className="tabular shrink-0">{formatBytes(item.size ?? 0)}</span>
            </span>
          </span>
        </button>

        <div className="absolute right-1 top-1 flex gap-1 opacity-0 transition-opacity focus-within:opacity-100 group-hover:opacity-100">
          <OverlayLink
            href={assetUrl(item.id) ?? '#'}
            download={downloadName(item)}
            label={`Download ${item.title}`}
          >
            <Download className="h-3 w-3" />
          </OverlayLink>
        </div>
      </div>
    </ItemMenu>
  )
}

function OverlayLink({
  href,
  download,
  label,
  children,
}: {
  href: string
  download?: string
  label: string
  children: ReactNode
}) {
  return (
    <Tooltip label={label}>
      <a
        href={href}
        download={download}
        aria-label={label}
        onClick={(event) => event.stopPropagation()}
        className="flex h-6 w-6 items-center justify-center rounded-[var(--radius-xs)] border border-[hsl(var(--border-strong))] bg-[hsl(var(--bg-elevated))]/90 text-muted backdrop-blur transition-colors hover:text-fg"
      >
        {children}
      </a>
    </Tooltip>
  )
}

/**
 * One artifact as a row. Unlike a tile it keeps its full subtitle even under a
 * group header: a list is read down its columns, and a column that empties out
 * inside every group is harder to read than one that repeats.
 */
function ArtifactRow({
  item,
  onOpen,
  onRerun,
}: {
  item: ViewerItem
  onOpen: () => void
  onRerun: () => void
}) {
  return (
    <ItemMenu item={item} onOpen={onOpen} onRerun={onRerun}>
      <div className="flex h-full items-center gap-3 border-b border-[hsl(var(--border))] pr-1 text-[12px] transition-colors hover:bg-[hsl(var(--bg-hover))]">
        <button
          type="button"
          onClick={onOpen}
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
          <Badge tone={assetKindTone(item.kind)} className="hidden shrink-0 sm:inline-flex">
            {item.kind}
          </Badge>
          <Mono className="hidden w-20 shrink-0 truncate text-[11px] text-subtle lg:block">
            {shortId(item.id)}
          </Mono>
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
      </div>
    </ItemMenu>
  )
}

/** Everything one artifact can be asked to do, on right-click. */
function ItemMenu({
  item,
  onOpen,
  onRerun,
  children,
}: {
  item: ViewerItem
  onOpen: () => void
  onRerun: () => void
  children: ReactNode
}) {
  return (
    <ContextMenu
      items={
        <>
          <ContextMenuLabel>{item.title}</ContextMenuLabel>
          <ContextMenuItem onSelect={onOpen}>
            <Eye className="h-3.5 w-3.5" />
            Open the preview
          </ContextMenuItem>
          <ContextMenuItem
            onSelect={() => {
              const anchor = document.createElement('a')
              anchor.href = assetUrl(item.id) ?? ''
              anchor.download = downloadName(item)
              anchor.click()
            }}
          >
            <Download className="h-3.5 w-3.5" />
            Download
          </ContextMenuItem>
          <ContextMenuItem onSelect={() => window.open(assetUrl(item.id), '_blank', 'noreferrer')}>
            <ExternalLink className="h-3.5 w-3.5" />
            Open the raw artifact
          </ContextMenuItem>
          <ContextMenuSeparator />
          <ContextMenuItem onSelect={() => void navigator.clipboard.writeText(item.id)}>
            <Copy className="h-3.5 w-3.5" />
            Copy the content address
          </ContextMenuItem>
          {item.taskId && (
            <ContextMenuItem onSelect={onRerun}>
              <RefreshCw className="h-3.5 w-3.5" />
              Re-run the step that made this
            </ContextMenuItem>
          )}
        </>
      }
    >
      {children}
    </ContextMenu>
  )
}
