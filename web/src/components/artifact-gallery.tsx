import type { ReactNode } from 'react'
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import {
  ArrowDownWideNarrow,
  ChevronDown,
  ChevronRight,
  Copy,
  Download,
  ExternalLink,
  Eye,
  FoldVertical,
  Image as ImageIcon,
  LayoutGrid,
  ListTree,
  Maximize2,
  RefreshCw,
  Rows3,
  UnfoldVertical,
} from 'lucide-react'

import {
  AssetPreview,
  assetKindIcon,
  assetKindTone,
  useAssetViewer,
} from '@/components/asset-viewer'
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
import { assetUrl } from '@/core/api'
import { artifactKindRank, downloadName, kindTitle, mediaTypeOf, shortId } from '@/core/assets'
import type { ViewerItem } from '@/core/assets'
import { formatBytes, formatRelative } from '@/core/format'
import { useHotkeys } from '@/core/hotkeys'
import type { Video } from '@/core/types'
import { cn } from '@/core/utils'
import { usePersisted } from '@/core/workspace'

/*
  The gallery.

  Every artifact a video has produced, as pictures rather than as hashes. A hash
  is the right *identity* for a content-addressed store and the wrong thing to
  show an operator asking whether the slides came out well — so the image leads
  and the address moves into the viewer.

  The shape is a tree, because that is the shape of the thing: a video owns a
  blueprint and a listing, each chapter owns a script, a narration, its slides
  and the clip they were composed into. Two rules keep it from becoming a filing
  cabinet — a kind with one artifact is drawn as that artifact rather than as a
  folder holding it, and the kinds run in pipeline order rather than
  alphabetically, so a chapter reads script, narration, slides, clip.

  At fifty chapters this is several hundred images, which is why every row is
  virtualised: the browser is asked for the dozen thumbnails on screen, not for
  four hundred at once. The column count is measured rather than assumed,
  because the pane is behind a splitter and its width is the operator's choice.
*/

type View = 'grid' | 'list'
type GroupBy = 'chapter' | 'none'
type SortKey = 'pipeline' | 'newest' | 'largest'
type Density = 's' | 'm' | 'l'

const TILE_WIDTH: Record<Density, number> = { s: 132, m: 178, l: 248 }
const GAP = 10
const PAD = 12
/** One step of the tree. Wide enough to read as a level, narrow enough to keep four. */
const INDENT = 18
const SECTION_ROW = 30
const KIND_ROW = 26
/** Two lines and a thumbnail, flat; one line under a header that names the rest. */
const LIST_ROW = 42
const LIST_ROW_TIGHT = 34
/**
 * The caption under a tile: its border, then the title, the subtitle and the
 * address line at fixed leading. Fixed rather than measured, because the
 * virtualiser needs a row height before the row exists — so the card is built to
 * the number rather than the number guessed from the card.
 *
 * Inside the tree the subtitle is gone — a header has already said which chapter
 * this is — and the tile is a line shorter.
 */
const CARD_CHROME = 59
const CARD_CHROME_TIGHT = 44
/** Under a header that has already named it: the address line and nothing else. */
const CARD_CHROME_BARE = 28

interface Cell {
  item: ViewerItem
  /** Position in the flattened, visible order — what the viewer walks. */
  index: number
}

type Row =
  | {
      type: 'section'
      key: string
      depth: number
      height: number
      label: string
      count: number
      bytes: number
    }
  | {
      type: 'kind'
      key: string
      depth: number
      height: number
      artifact: string
      count: number
      bytes: number
      first: number
    }
  | {
      type: 'grid'
      key: string
      depth: number
      height: number
      width: number
      titled: boolean
      cells: Cell[]
    }
  | { type: 'list'; key: string; depth: number; height: number; titled: boolean; cell: Cell }

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
  const [folded, setFolded] = useState<ReadonlySet<string>>(() => new Set())
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
    return [...counts.entries()].sort((a, b) => artifactKindRank(a[0]) - artifactKindRank(b[0]))
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

  // By chapter and only ever by chapter: that is the axis a video is reviewed
  // along. Sorting by size or age crosses chapters, so it flattens the tree.
  const tree = group === 'chapter' && sort === 'pipeline'
  // A folded section would hide the very thing being searched for.
  const searching = search.length > 0

  const tile = TILE_WIDTH[density]

  const { rows, ordered } = useMemo(
    () =>
      buildRows({
        shown,
        tree,
        view,
        available: Math.max(0, width - PAD * 2),
        tile,
        folded: searching ? EMPTY : folded,
      }),
    [shown, tree, view, width, tile, folded, searching],
  )

  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: (index) => rows[index]?.height ?? LIST_ROW,
    getItemKey: (index) => rows[index]?.key ?? index,
    overscan: 6,
  })

  // Row heights are computed, not measured: a wider pane or a bigger tile
  // changes every one at once, taking the cached measurements with them.
  useEffect(() => {
    virtualizer.measure()
  }, [virtualizer, rows])

  const toggle = (key: string) =>
    setFolded((prev) => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })

  /** Every collapsible node, so "collapse all" does not have to guess. */
  const allNodes = useMemo(
    () => rows.flatMap((row) => (row.type === 'section' || row.type === 'kind' ? [row.key] : [])),
    [rows],
  )
  const anyOpen = allNodes.some((key) => !folded.has(key))

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
                ? 'Pipeline order — chapter by chapter, stage by stage'
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
                  className="absolute left-0 w-full"
                  style={{ height: virtual.size, transform: `translateY(${virtual.start}px)` }}
                >
                  <div
                    className="relative h-full"
                    style={{ paddingLeft: PAD + row.depth * INDENT, paddingRight: PAD }}
                  >
                    <Guides depth={row.depth} />

                    {row.type === 'section' && (
                      <SectionHeader
                        label={row.label}
                        count={row.count}
                        bytes={row.bytes}
                        folded={folded.has(row.key)}
                        onToggle={() => toggle(row.key)}
                      />
                    )}
                    {row.type === 'kind' && (
                      <KindHeader
                        artifact={row.artifact}
                        count={row.count}
                        bytes={row.bytes}
                        folded={folded.has(row.key)}
                        onToggle={() => toggle(row.key)}
                        onView={() => openViewer(ordered, row.first)}
                      />
                    )}
                    {row.type === 'grid' && (
                      <div className="flex" style={{ gap: GAP }}>
                        {row.cells.map(({ item, index }) => (
                          <ArtifactCard
                            key={`${item.id}:${index}`}
                            item={item}
                            width={row.width}
                            titled={row.titled}
                            subtitle={!tree}
                            onOpen={() => openViewer(ordered, index)}
                            onRerun={() => setRerunning(item)}
                          />
                        ))}
                      </div>
                    )}
                    {row.type === 'list' && (
                      <ArtifactRow
                        item={row.cell.item}
                        titled={row.titled}
                        subtitle={!tree}
                        onOpen={() => openViewer(ordered, row.cell.index)}
                        onRerun={() => setRerunning(row.cell.item)}
                      />
                    )}
                  </div>
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

        {tree && allNodes.length > 0 && (
          <Tooltip label={anyOpen ? 'Collapse every group' : 'Expand every group'}>
            <button
              type="button"
              onClick={() => setFolded(anyOpen ? new Set(allNodes) : EMPTY)}
              className="flex items-center gap-1.5 rounded-[var(--radius-xs)] px-1.5 py-0.5 transition-colors hover:text-fg"
            >
              {anyOpen ? (
                <FoldVertical className="h-3 w-3" />
              ) : (
                <UnfoldVertical className="h-3 w-3" />
              )}
              {anyOpen ? 'Collapse all' : 'Expand all'}
            </button>
          </Tooltip>
        )}

        <Tooltip
          label={
            sort === 'pipeline'
              ? 'Nest the artifacts by chapter and stage, or lay them out flat'
              : 'Sorting by size or age crosses chapters, so the tree stands down'
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
              tree && 'text-fg',
              sort !== 'pipeline' && 'opacity-40',
            )}
          >
            <ListTree className="h-3 w-3" />
            {sort === 'pipeline' ? (tree ? 'Tree' : 'Flat') : 'Tree is off while sorted'}
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

/* --------------------------------------------------------------- the model */

const EMPTY: ReadonlySet<string> = new Set()

/**
 * The visible rows, and the order the viewer walks.
 *
 * Both come out of one pass because they have to agree: pressing → in the
 * lightbox should land on whatever was drawn next, which is neither the order
 * the assets arrived in nor the order they are stored in. A folded group
 * contributes nothing to either.
 */
function buildRows({
  shown,
  tree,
  view,
  available,
  tile,
  folded,
}: {
  shown: ViewerItem[]
  tree: boolean
  view: View
  available: number
  tile: number
  folded: ReadonlySet<string>
}): { rows: Row[]; ordered: ViewerItem[] } {
  const rows: Row[] = []
  const ordered: ViewerItem[] = []

  /*
    How much caption a tile carries, which is what its height is made of.
    Outside the tree it needs its chapter; inside, a header has already said
    that; and under a header naming a kind whose artifacts are all called the
    same thing — twelve tiles reading "Thumbnail icon" — the title is the
    header repeated, so the address does the distinguishing on its own.
  */
  const chromeFor = (titled: boolean) =>
    !titled ? CARD_CHROME_BARE : tree ? CARD_CHROME_TIGHT : CARD_CHROME

  // Per depth, because an indented row is a narrower row: one fewer column, and
  // every tile in it a little smaller.
  const geometry = (depth: number, titled: boolean) => {
    const usable = Math.max(tile, available - depth * INDENT)
    const columns = Math.max(1, Math.floor((usable + GAP) / (tile + GAP)))
    const width = (usable - GAP * (columns - 1)) / columns
    return { columns, width, height: Math.round((width * 10) / 16) + chromeFor(titled) + GAP }
  }

  const emit = (items: ViewerItem[], depth: number, titled = true) => {
    const cells: Cell[] = items.map((item) => {
      ordered.push(item)
      return { item, index: ordered.length - 1 }
    })
    if (view === 'list') {
      for (const cell of cells) {
        rows.push({
          type: 'list',
          key: `l:${cell.index}`,
          depth,
          height: tree ? LIST_ROW_TIGHT : LIST_ROW,
          cell,
          titled,
        })
      }
      return
    }
    const { columns, width, height } = geometry(depth, titled)
    for (let i = 0; i < cells.length; i += columns) {
      rows.push({
        type: 'grid',
        key: `g:${depth}:${cells[i]?.index ?? i}`,
        depth,
        height,
        titled,
        width,
        cells: cells.slice(i, i + columns),
      })
    }
  }

  if (!tree) {
    emit(shown, 0)
    return { rows, ordered }
  }

  const sections = new Map<number, { label: string; items: ViewerItem[] }>()
  for (const item of shown) {
    const ordinal = item.ordinal ?? 0
    const section = sections.get(ordinal) ?? {
      label: ordinal > 0 ? (item.subtitle ?? `Chapter ${ordinal}`) : 'Whole video',
      items: [],
    }
    section.items.push(item)
    sections.set(ordinal, section)
  }

  for (const [ordinal, section] of [...sections.entries()].sort((a, b) => a[0] - b[0])) {
    const key = `s:${ordinal}`
    rows.push({
      type: 'section',
      key,
      depth: 0,
      height: SECTION_ROW,
      label: section.label,
      count: section.items.length,
      bytes: bytesOf(section.items),
    })
    if (folded.has(key)) continue

    const byKind = new Map<string, ViewerItem[]>()
    for (const item of section.items) {
      const list = byKind.get(item.kind)
      if (list) list.push(item)
      else byKind.set(item.kind, [item])
    }

    /*
      A kind with one artifact is that artifact, not a folder holding it — a
      chapter's script does not deserve a node of its own. Consecutive singles
      are gathered into one run so the grid packs them into a shared row instead
      of giving each a line to itself.
    */
    let run: ViewerItem[] = []
    const flush = () => {
      if (run.length === 0) return
      emit(run, 1)
      run = []
    }

    for (const artifact of [...byKind.keys()].sort(
      (a, b) => artifactKindRank(a) - artifactKindRank(b),
    )) {
      const group = byKind.get(artifact) ?? []
      if (group.length === 1) {
        run.push(...group)
        continue
      }
      flush()
      const kindKey = `k:${ordinal}:${artifact}`
      rows.push({
        type: 'kind',
        key: kindKey,
        depth: 1,
        height: KIND_ROW,
        artifact,
        count: group.length,
        bytes: bytesOf(group),
        first: ordered.length,
      })
      if (folded.has(kindKey)) continue
      // Slides are numbered by slot and worth naming; icons all share a name,
      // and eleven copies of it say nothing the header has not.
      const named = new Set(group.map((item) => item.title)).size > 1
      emit(group, 2, named)
    }
    flush()
  }

  return { rows, ordered }
}

function bytesOf(items: ViewerItem[]): number {
  return items.reduce((sum, item) => sum + (item.size ?? 0), 0)
}

/* -------------------------------------------------------------- the pieces */

/**
 * The vertical rules that make the indentation read as a tree rather than as
 * margin. Drawn per row rather than per branch: the list is virtualised, so
 * there is no element spanning a whole group to hang a border on.
 */
function Guides({ depth }: { depth: number }) {
  if (depth === 0) return null
  return (
    <>
      {Array.from({ length: depth }, (_, level) => (
        <span
          key={level}
          aria-hidden
          className="absolute inset-y-0 w-px bg-[hsl(var(--border))]"
          style={{ left: PAD + level * INDENT + 7 }}
        />
      ))}
    </>
  )
}

function SectionHeader({
  label,
  count,
  bytes,
  folded,
  onToggle,
}: {
  label: string
  count: number
  bytes: number
  folded: boolean
  onToggle: () => void
}) {
  return (
    <div className="flex h-full items-center gap-2 no-select">
      <button
        type="button"
        onClick={onToggle}
        aria-expanded={!folded}
        className="flex min-w-0 items-center gap-1.5 text-left"
      >
        <Chevron folded={folded} />
        <span className="truncate text-[11.5px] font-semibold text-fg">{label}</span>
        <span className="tabular shrink-0 text-[10.5px] text-subtle">
          {count} · {formatBytes(bytes)}
        </span>
      </button>
      <span className="h-px min-w-4 flex-1 bg-[hsl(var(--border))]" aria-hidden />
    </div>
  )
}

function KindHeader({
  artifact,
  count,
  bytes,
  folded,
  onToggle,
  onView,
}: {
  artifact: string
  count: number
  bytes: number
  folded: boolean
  onToggle: () => void
  onView: () => void
}) {
  const Icon = assetKindIcon(artifact)
  return (
    <div className="group flex h-full items-center gap-2 no-select">
      <button
        type="button"
        onClick={onToggle}
        aria-expanded={!folded}
        className="flex min-w-0 items-center gap-1.5 text-left"
      >
        <Chevron folded={folded} />
        <Icon className="h-3 w-3 shrink-0 text-subtle" aria-hidden />
        <span className="truncate text-[11.5px] font-medium text-muted">{kindTitle(artifact)}</span>
        <span className="tabular shrink-0 text-[10.5px] text-subtle">
          {count} · {formatBytes(bytes)}
        </span>
      </button>
      <Tooltip label={`Open all ${count} in the viewer`}>
        <Button
          size="xs"
          variant="ghost"
          onClick={onView}
          className="h-5 opacity-0 transition-opacity focus-visible:opacity-100 group-hover:opacity-100"
        >
          <Eye className="h-3 w-3" />
          View
        </Button>
      </Tooltip>
      <span className="h-px min-w-4 flex-1 bg-[hsl(var(--border))]" aria-hidden />
    </div>
  )
}

function Chevron({ folded }: { folded: boolean }) {
  return folded ? (
    <ChevronRight className="h-3 w-3 shrink-0 text-subtle" aria-hidden />
  ) : (
    <ChevronDown className="h-3 w-3 shrink-0 text-subtle" aria-hidden />
  )
}

/**
 * One artifact as a tile. The card is a plain element rather than a button so
 * the download affordance can be a real anchor on top of it — a link nested in a
 * button is not a link at all.
 */
function ArtifactCard({
  item,
  width,
  titled,
  subtitle,
  onOpen,
  onRerun,
}: {
  item: ViewerItem
  width: number
  /** Dropped under a kind header when every tile under it carries the same name. */
  titled: boolean
  /** Dropped inside the tree, where a header has already named the chapter. */
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
          <span
            className={cn(
              'block px-2.5 py-1.5',
              !titled ? 'h-[27px]' : subtitle ? 'h-[58px]' : 'h-[43px]',
            )}
          >
            {titled && (
              <span className="block truncate text-[12px] font-medium leading-[16px] text-fg">
                {item.title}
              </span>
            )}
            {titled && subtitle && (
              <span className="block truncate text-[11px] leading-[15px] text-subtle">
                {item.subtitle}
              </span>
            )}
            <span
              className={cn(
                'flex items-center justify-between gap-2 text-[10.5px] leading-[15px]',
                titled ? 'text-subtle' : 'text-muted',
              )}
            >
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

function ArtifactRow({
  item,
  titled,
  subtitle,
  onOpen,
  onRerun,
}: {
  item: ViewerItem
  titled: boolean
  subtitle: boolean
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
          <span
            className={cn(
              'shrink-0 overflow-hidden rounded-[var(--radius-xs)] border border-[hsl(var(--border))]',
              subtitle ? 'h-8 w-14' : 'h-6 w-11',
            )}
          >
            <AssetPreview item={item} />
          </span>
          <span className="min-w-0 flex-1">
            {titled ? (
              <span className="block truncate font-medium text-fg">{item.title}</span>
            ) : (
              <Mono className="block truncate text-[11.5px] text-muted">{item.id}</Mono>
            )}
            {titled && subtitle && (
              <span className="block truncate text-[11px] text-subtle">{item.subtitle}</span>
            )}
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
