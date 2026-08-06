import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate, useSearch } from '@tanstack/react-router'
import {
  AlertCircle,
  ArrowRight,
  Check,
  Clapperboard,
  Frame,
  Gauge,
  Image as ImageIcon,
  Loader2,
  Mic,
  Minus,
  Pencil,
  PenLine,
  Plug,
  Plus,
  RotateCcw,
  Server,
  ShieldCheck,
  SlidersHorizontal,
  Timer,
  Wand2,
  type LucideIcon,
} from 'lucide-react'
import type { KeyboardEvent as ReactKeyboardEvent, ReactNode } from 'react'
import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react'

import { PageHeader } from '@/components/app-shell'
import { Button } from '@/components/ui/button'
import { Input, Select, Textarea } from '@/components/ui/field'
import {
  CopyButton,
  EmptyState,
  ErrorNotice,
  Kbd,
  SearchField,
  Segmented,
  Skeleton,
  Tooltip,
} from '@/components/ui/primitives'
import { Switch } from '@/components/ui/switch'
import { api, qk } from '@/lib/api'
import { formatRelative, poolLabel } from '@/lib/format'
import { useHotkeys } from '@/lib/hotkeys'
import type { PoolStat, Preset, PresetValue, Setting } from '@/lib/types'
import { cn } from '@/lib/utils'

interface GroupMeta {
  title: string
  blurb: string
  icon: LucideIcon
  /** The rail's heading this section files under. */
  category: string
}

/**
 * The sections, in the order the rail lists them. A section is a task the
 * operator is doing rather than a subsystem that reads the rows, which is why
 * the four in the middle are the pipeline in the order it runs — write,
 * narrate, draw, package. Backend knobs sit with the stage they shape and are
 * marked with the backend that reads them, so "Providers" can stay the
 * seven-line answer to who does each job instead of collecting a backend's
 * settings every time one is registered.
 */
const GROUPS: Record<string, GroupMeta> = {
  pools: {
    category: 'Running',
    title: 'Concurrency',
    blurb:
      'Enforced across every video and channel. Lowering a limit takes effect as running tasks finish; a running provider call is not preemptible.',
    icon: Gauge,
  },
  retries: {
    category: 'Running',
    title: 'Retries',
    blurb: 'What happens when a task fails for a reason another attempt could survive.',
    icon: Timer,
  },
  gates: {
    category: 'Approval',
    title: 'Approval gates',
    blurb:
      'What stands between a generation and a public video. A gate costs nothing while it is open; the dry run is the one whose failure mode is silent.',
    icon: ShieldCheck,
  },
  presets: {
    category: 'Backends',
    title: 'Presets',
    blurb:
      'One click across every provider row. A preset writes only the rows it names — the gates and the upload dry run are never among them.',
    icon: Wand2,
  },
  providers: {
    category: 'Backends',
    title: 'Providers',
    blurb:
      'One row per port: who does each job. Each list holds the backends this build registered, and a change applies to the next task.',
    icon: Plug,
  },
  writing: {
    category: 'Pipeline',
    title: 'Writing',
    blurb:
      'The model behind the blueprint, the scripts, the prompts and the metadata — and how far off-target a blueprint may land before it is written again.',
    icon: PenLine,
  },
  narration: {
    category: 'Pipeline',
    title: 'Narration',
    blurb:
      'How a chapter sounds. Voice, language and pace travel with the request, so a channel can be given its own later without any of this moving.',
    icon: Mic,
  },
  slides: {
    category: 'Pipeline',
    title: 'Slides',
    blurb: 'The checkpoint chapter artwork is drawn with, and the geometry it is drawn at.',
    icon: ImageIcon,
  },
  thumbnail: {
    category: 'Pipeline',
    title: 'Thumbnail',
    blurb:
      'The listing artifact: the shared style its icons are drawn in, and the type and grid they are set under.',
    icon: Frame,
  },
  video: {
    category: 'New videos',
    title: 'Defaults',
    blurb:
      'What a new video is created with when the request leaves the field blank. Read once, at creation — an existing video keeps what it was made with.',
    icon: Clapperboard,
  },
  server: {
    category: 'System',
    title: 'Server',
    blurb: 'Applied live — none of these need a restart.',
    icon: Server,
  },
}

const GROUP_ORDER = Object.keys(GROUPS)

/**
 * The one section that is not a group of the settings table: it writes rows
 * rather than holding them. It sits in the rail beside the real groups because
 * from the operator's side it is the same question — which backends run this —
 * answered in one click instead of seven.
 */
const PRESETS = 'presets'

function groupMeta(group: string): GroupMeta {
  return GROUPS[group] ?? { title: group, blurb: '', icon: SlidersHorizontal, category: 'Other' }
}

/** What a row is doing right now. Absent means the row is at rest. */
interface RowState {
  saving?: boolean
  applied?: boolean
  error?: unknown
}

/**
 * The settings screen is a plain CRUD surface over the settings table, with no
 * privileged file access anywhere.
 *
 * Two things shape it. Drafts live here rather than in each row, so the pane can
 * offer one "apply everything" instead of making the operator hunt for the rows
 * they touched — and so a row that saves itself the moment it is flipped (a
 * switch, a backend) can sit beside one that waits for Enter without the two
 * disagreeing about what "unsaved" means. And the rail on the left is navigation
 * rather than a table of contents: one section is mounted at a time, because
 * thirty keys in one column means every visit scrolls past six answers to reach
 * the one being changed. Drafts survive the move, so leaving a section mid-edit
 * loses nothing and the dock below still names what is unsaved.
 */
export function SettingsRoute() {
  const queryClient = useQueryClient()
  const settings = useQuery({ queryKey: qk.settings, queryFn: api.listSettings })
  const presets = useQuery({ queryKey: qk.presets, queryFn: api.listPresets })

  // Free from cache — the status bar keeps this query warm. It turns an abstract
  // limit into "and here is what that limit is currently doing".
  const { data: status } = useQuery({ queryKey: qk.scheduler, queryFn: api.schedulerStatus })

  const navigate = useNavigate()
  const { section: requested } = useSearch({ from: '/settings' })

  const [query, setQuery] = useState('')
  const searchRef = useRef<HTMLInputElement>(null)

  const [drafts, setDrafts] = useState<Record<string, string>>({})
  const [rowStates, setRowStates] = useState<Record<string, RowState>>({})
  const draftsRef = useRef(drafts)
  draftsRef.current = drafts
  const timers = useRef(new Map<string, ReturnType<typeof setTimeout>>())
  useEffect(() => {
    const pending = timers.current
    return () => pending.forEach(clearTimeout)
  }, [])

  const rows = useMemo(() => settings.data ?? [], [settings.data])

  // A value that has caught up with the server — saved here or changed by
  // another client — is no longer a draft, whichever of the two did it.
  useEffect(() => {
    setDrafts((prev) => {
      let changed = false
      const next: Record<string, string> = {}
      for (const row of rows) {
        const draft = prev[row.key]
        if (draft === undefined) continue
        if (draft === row.value) changed = true
        else next[row.key] = draft
      }
      return changed || Object.keys(next).length !== Object.keys(prev).length ? next : prev
    })
  }, [rows])

  const needle = query.trim().toLowerCase()
  const hasPresets = (presets.data?.length ?? 0) > 0

  /**
   * Which backends are actually in service, read off the routing table rather
   * than asked for: the `provider.*` rows are the whole answer, and a second
   * endpoint reporting it could only ever disagree with them.
   */
  const backends = useMemo(
    () => new Set(rows.filter((row) => row.key.startsWith('provider.')).map((row) => row.value)),
    [rows],
  )

  /**
   * Every section the rail offers, with all of its rows. The filter narrows what
   * a section shows, never which sections exist — the rail is how the operator
   * finds out what this build has, so it must not shrink under them.
   */
  const sections = useMemo(() => {
    const map = new Map<string, Setting[]>()
    // The presets section owns no rows; it is listed when the server has any.
    if (hasPresets) map.set(PRESETS, [])
    for (const setting of rows) {
      const list = map.get(setting.group)
      if (list) list.push(setting)
      else map.set(setting.group, [setting])
    }
    return [...map.entries()].sort(
      (a, b) =>
        (GROUP_ORDER.indexOf(a[0]) + 1 || 99) - (GROUP_ORDER.indexOf(b[0]) + 1 || 99) ||
        a[0].localeCompare(b[0]),
    )
  }, [rows, hasPresets])

  const matches = useMemo(() => {
    const map = new Map<string, Setting[]>()
    for (const [group, list] of sections) {
      map.set(
        group,
        needle
          ? list.filter(
              (setting) =>
                setting.key.toLowerCase().includes(needle) ||
                setting.description.toLowerCase().includes(needle),
            )
          : list,
      )
    }
    return map
  }, [sections, needle])

  /**
   * The section the pane renders. The URL carries the request so a reload comes
   * back to it; what is actually rendered is the first section with something to
   * render, so filtering from a section the needle misses lands on the matches
   * instead of on a blank pane — and clearing the filter returns to where the
   * operator was, since the URL never changed.
   */
  const active = useMemo(() => {
    const shows = (group: string) => !needle || (matches.get(group)?.length ?? 0) > 0
    if (requested !== undefined && matches.has(requested) && shows(requested)) return requested
    return sections.find(([group]) => shows(group))?.[0] ?? ''
  }, [sections, matches, needle, requested])

  const selectSection = useCallback(
    // Replaced rather than pushed: the back button is for leaving settings, not
    // for retracing which section was looked at on the way through.
    (group: string) =>
      void navigate({ to: '/settings', search: { section: group }, replace: true }),
    [navigate],
  )

  const dirtyKeys = useMemo(
    () => rows.filter((row) => drafts[row.key] !== undefined).map((row) => row.key),
    [rows, drafts],
  )
  const saving = dirtyKeys.some((key) => rowStates[key]?.saving)

  /**
   * Applied in sequence rather than at once: the store serialises writes anyway,
   * and a failure part-way through then names the row it stopped at instead of
   * scattering six half-answers across the pane.
   */
  const apply = useCallback(
    async (keys: string[]) => {
      for (const key of keys) {
        const value = draftsRef.current[key]
        if (value === undefined) continue
        setRowStates((prev) => ({ ...prev, [key]: { saving: true } }))
        try {
          const updated = await api.updateSetting(key, value)
          queryClient.setQueryData<Setting[]>(qk.settings, (prev) =>
            prev?.map((row) => (row.key === updated.key ? updated : row)),
          )
          setDrafts((prev) => {
            const next = { ...prev }
            delete next[key]
            return next
          })
          setRowStates((prev) => ({ ...prev, [key]: { applied: true } }))
          clearTimeout(timers.current.get(key))
          timers.current.set(
            key,
            setTimeout(() => {
              timers.current.delete(key)
              setRowStates((prev) => {
                const next = { ...prev }
                delete next[key]
                return next
              })
            }, 1800),
          )
        } catch (error) {
          setRowStates((prev) => ({ ...prev, [key]: { error } }))
        }
      }
      void queryClient.invalidateQueries({ queryKey: qk.scheduler })
    },
    [queryClient],
  )

  const draft = useCallback((key: string, value: string) => {
    setDrafts((prev) => ({ ...prev, [key]: value }))
  }, [])

  const revert = useCallback((key: string) => {
    setDrafts((prev) => {
      const next = { ...prev }
      delete next[key]
      return next
    })
    setRowStates((prev) => {
      const next = { ...prev }
      delete next[key]
      return next
    })
  }, [])

  const commit = useCallback((key: string) => void apply([key]), [apply])

  /** Draft and apply in one move, for a control that has no half-changed state. */
  const set = useCallback(
    (key: string, value: string) => {
      setDrafts((prev) => ({ ...prev, [key]: value }))
      draftsRef.current = { ...draftsRef.current, [key]: value }
      void apply([key])
    },
    [apply],
  )

  const discardAll = useCallback(() => {
    setDrafts({})
    setRowStates({})
  }, [])

  useHotkeys([
    {
      keys: 'mod+s',
      label: 'Apply pending settings',
      group: 'Settings',
      whileTyping: true,
      run: () => {
        ;(document.activeElement as HTMLElement | null)?.blur()
        if (dirtyKeys.length > 0) void apply(dirtyKeys)
      },
    },
    {
      keys: '/',
      label: 'Filter settings',
      group: 'Settings',
      run: () => searchRef.current?.focus(),
    },
  ])

  const total = rows.length
  const shown = needle ? [...matches.values()].reduce((sum, list) => sum + list.length, 0) : total

  return (
    <>
      <PageHeader
        title="Settings"
        subtitle="One row per key, held in the database. Every change applies to the next task rather than the next restart."
        actions={
          <>
            <span className="tabular hidden text-[11.5px] text-subtle sm:inline">
              {shown === total ? `${total} keys` : `${shown} of ${total}`}
            </span>
            <SearchField
              value={query}
              onChange={setQuery}
              placeholder="Filter settings"
              inputRef={searchRef}
              keys="/"
              className="w-64"
            />
          </>
        }
      />

      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-5xl px-4 py-5">
          {settings.isPending && (
            <div className="space-y-4">
              {Array.from({ length: 3 }, (_, i) => (
                <Skeleton key={i} className="h-44" />
              ))}
            </div>
          )}
          {settings.isError && <ErrorNotice error={settings.error} />}

          {sections.length > 0 && (
            <div className="grid gap-6 lg:grid-cols-[184px_minmax(0,1fr)]">
              {/* Vertical beside the pane, a scrolling strip of chips above it
                  when there is no room — hiding it would strand the operator in
                  whichever section happened to be first. */}
              {/* No "Sections" title above it: the categories are headings
                  already, and a heading over the headings was the third grey
                  left-aligned line in a column that only needed two. */}
              <nav aria-label="Setting groups" className="lg:sticky lg:top-0 lg:self-start">
                <div className="-mx-4 flex gap-1 overflow-x-auto px-4 pb-1 lg:mx-0 lg:flex-col lg:gap-0.5 lg:overflow-x-visible lg:px-0 lg:pb-4">
                  {sections.map(([group, list], i) => (
                    <RailItem
                      key={group}
                      group={group}
                      // The category heading is drawn by the first section under
                      // it, so the rail stays one flat list of buttons and the
                      // horizontal strip can simply drop the headings.
                      category={
                        groupMeta(group).category === groupMeta(sections[i - 1]?.[0] ?? '').category
                          ? undefined
                          : groupMeta(group).category
                      }
                      count={
                        group === PRESETS
                          ? (presets.data?.length ?? 0)
                          : (matches.get(group)?.length ?? 0)
                      }
                      active={active === group}
                      // Measured against every row, not the visible ones: an
                      // unsaved edit in a filtered-out row still needs saying.
                      dirty={list.some((row) => drafts[row.key] !== undefined)}
                      empty={needle !== '' && (matches.get(group)?.length ?? 0) === 0}
                      onSelect={() => selectSection(group)}
                    />
                  ))}
                </div>
              </nav>

              <div className="min-w-0">
                {active === PRESETS && <PresetBar rows={rows} running={status?.running ?? 0} />}

                {active !== '' && active !== PRESETS && (
                  <GroupSection
                    group={active}
                    rows={matches.get(active) ?? []}
                    pools={status?.pools}
                    backends={backends}
                    drafts={drafts}
                    rowStates={rowStates}
                    needle={needle}
                    onDraft={draft}
                    onSet={set}
                    onCommit={commit}
                    onRevert={revert}
                  />
                )}

                {active === '' && (
                  <EmptyState
                    icon={<SlidersHorizontal />}
                    title="Nothing matches"
                    description={`None of the ${total} settings mention “${query.trim()}”.`}
                    action={
                      <Button variant="outline" size="sm" onClick={() => setQuery('')}>
                        Clear the filter
                      </Button>
                    }
                  />
                )}
              </div>
            </div>
          )}
        </div>
      </div>

      {dirtyKeys.length > 0 && (
        <PendingBar
          keys={dirtyKeys}
          saving={saving}
          onApply={() => void apply(dirtyKeys)}
          onDiscard={discardAll}
        />
      )}
    </>
  )
}

/* -------------------------------------------------------------------- rail */

function RailItem({
  group,
  category,
  count,
  active,
  dirty,
  empty,
  onSelect,
}: {
  group: string
  /** Set on the first section of a category, which draws the heading for it. */
  category: string | undefined
  count: number
  active: boolean
  dirty: boolean
  /** No row here survives the current filter; shown, but not worth going to. */
  empty: boolean
  onSelect: () => void
}) {
  const { title, icon: Icon } = groupMeta(group)
  return (
    <>
      {/* A label, not an item, and told apart on three axes at once: a rule
          above it, smaller and wider-tracked type, and a left edge the buttons
          below are indented past. Any one of the three alone still reads as
          another row in the list. */}
      {category !== undefined && (
        <p className="mt-3 hidden border-t border-[hsl(var(--border))] px-1 pb-1 pt-2.5 text-[9.5px] font-semibold uppercase tracking-[0.13em] text-subtle first:mt-0 first:border-t-0 first:pt-0 lg:block">
          {category}
        </p>
      )}
      <button
        type="button"
        onClick={onSelect}
        disabled={empty}
        aria-current={active ? 'true' : undefined}
        className={cn(
          'relative flex shrink-0 items-center gap-2 rounded-[var(--radius-sm)] py-1.5 pl-2.5 pr-2 text-left text-[12.5px] transition-colors lg:w-full lg:pl-3',
          // The selected section is the pane: it carries the accent outright, so
          // which one is open is answered without comparing two greys.
          active
            ? 'bg-[hsl(var(--accent-soft))] font-semibold text-[hsl(var(--accent))]'
            : 'text-muted hover:bg-[hsl(var(--bg-hover))] hover:text-fg',
          empty && 'pointer-events-none opacity-40',
        )}
      >
        {active && (
          <span
            aria-hidden
            className="absolute inset-y-1 left-[1px] hidden w-[2.5px] rounded-full bg-[hsl(var(--accent))] lg:block"
          />
        )}
        <Icon
          className={cn(
            'h-3.5 w-3.5 shrink-0',
            active ? 'text-[hsl(var(--accent))]' : 'text-subtle',
          )}
        />
        <span className="min-w-0 flex-1 truncate">{title}</span>
        {dirty ? (
          <span
            className="h-1.5 w-1.5 shrink-0 rounded-full bg-[hsl(var(--warning))]"
            aria-label="unsaved changes"
          />
        ) : (
          <span
            className={cn(
              'tabular text-[10.5px]',
              active ? 'text-[hsl(var(--accent)/0.8)]' : 'text-subtle',
            )}
          >
            {count}
          </span>
        )}
      </button>
    </>
  )
}

/* ----------------------------------------------------------------- presets */

/**
 * The presets strip: one click that moves every provider row at once.
 *
 * Which preset is in force is derived here rather than read from the server. A
 * stored "current preset" would start lying the moment one backend was changed
 * by hand, and this page holds both sides of the comparison already — so a
 * preset is in force exactly when every row it names already holds the value it
 * would write, and the same computation gives the diff to show before applying.
 */
function PresetBar({ rows, running }: { rows: Setting[]; running: number }) {
  const queryClient = useQueryClient()
  const presets = useQuery({ queryKey: qk.presets, queryFn: api.listPresets })
  const [pending, setPending] = useState('')
  const [error, setError] = useState<unknown>()

  const current = useMemo(() => new Map(rows.map((row) => [row.key, row.value])), [rows])

  const diffs = useMemo(
    () =>
      (presets.data ?? []).map((preset) => ({
        preset,
        changes: preset.values.filter((value) => current.get(value.key) !== value.value),
      })),
    [presets.data, current],
  )

  const apply = useCallback(
    async (name: string) => {
      setPending(name)
      setError(undefined)
      try {
        const changed = await api.applyPreset(name)
        queryClient.setQueryData<Setting[]>(qk.settings, (prev) =>
          prev?.map((row) => changed.find((next) => next.key === row.key) ?? row),
        )
        void queryClient.invalidateQueries({ queryKey: qk.scheduler })
      } catch (failure) {
        setError(failure)
      } finally {
        setPending('')
      }
    },
    [queryClient],
  )

  // Nothing to say if the server has no presets, and an unreachable list is
  // already reported by the settings query above it.
  if (diffs.length === 0) return null

  return (
    <section aria-labelledby={`heading-${PRESETS}`}>
      <SectionHeader group={PRESETS} />

      {running > 0 && (
        <p className="mb-2 flex items-center gap-1.5 px-0.5 text-[11.5px] text-[hsl(var(--warning))]">
          <AlertCircle className="h-3.5 w-3.5 shrink-0" aria-hidden />
          {running} task{running === 1 ? ' is' : 's are'} running. A backend is resolved when a task
          is dispatched, so the work already in flight finishes on the old one and the rest starts
          on the new.
        </p>
      )}

      {error !== undefined && <ErrorNotice error={error} className="mb-2" />}

      <ul className="grid gap-2.5 sm:grid-cols-2 xl:grid-cols-3">
        {diffs.map(({ preset, changes }) => (
          <PresetCard
            key={preset.name}
            preset={preset}
            changes={changes}
            current={current}
            saving={pending === preset.name}
            disabled={pending !== '' && pending !== preset.name}
            onApply={() => void apply(preset.name)}
          />
        ))}
      </ul>
    </section>
  )
}

function PresetCard({
  preset,
  changes,
  current,
  saving,
  disabled,
  onApply,
}: {
  preset: Preset
  changes: PresetValue[]
  current: Map<string, string>
  saving: boolean
  disabled: boolean
  onApply: () => void
}) {
  const inForce = changes.length === 0
  return (
    <li
      className={cn(
        'flex flex-col rounded-[var(--radius-md)] border bg-[hsl(var(--bg-elevated))] p-3 elev-1 transition-colors',
        inForce ? 'border-[hsl(var(--accent))]' : 'border-[hsl(var(--border))]',
      )}
    >
      <div className="flex items-center gap-1.5">
        <h3 className="text-[12.5px] font-semibold text-fg">{preset.title}</h3>
        <span className="font-mono text-[11px] text-subtle">{preset.name}</span>
        {inForce && (
          <span className="ml-auto shrink-0">
            <Pill tone="success">
              <Check className="h-3 w-3" aria-hidden />
              In force
            </Pill>
          </span>
        )}
      </div>

      <p className="mt-1 text-[11.5px] leading-[1.5] text-muted">{preset.description}</p>

      {!inForce && (
        <ul className="mt-2 space-y-0.5">
          {changes.map((change) => (
            <li key={change.key} className="flex items-center gap-1 font-mono text-[10.5px]">
              <span className="min-w-0 flex-1 truncate text-subtle">{change.key}</span>
              <span className="shrink-0 text-subtle line-through">
                {current.get(change.key) || '—'}
              </span>
              <ArrowRight className="h-3 w-3 shrink-0 text-subtle" aria-hidden />
              <span className="shrink-0 font-medium text-fg">{change.value}</span>
            </li>
          ))}
        </ul>
      )}

      <div className="mt-2.5 flex items-center gap-2 pt-0.5">
        <Button
          variant={inForce ? 'outline' : 'primary'}
          size="sm"
          disabled={inForce || saving || disabled}
          onClick={onApply}
        >
          {saving ? (
            <Loader2 className="h-3.5 w-3.5 animate-spin" aria-hidden />
          ) : (
            <Wand2 className="h-3.5 w-3.5" aria-hidden />
          )}
          {inForce ? 'Applied' : `Apply ${changes.length} change${changes.length === 1 ? '' : 's'}`}
        </Button>
      </div>
    </li>
  )
}

/* ----------------------------------------------------------------- section */

interface RowHandlers {
  onDraft: (key: string, value: string) => void
  onSet: (key: string, value: string) => void
  onCommit: (key: string) => void
  onRevert: (key: string) => void
}

function GroupSection({
  group,
  rows,
  pools,
  backends,
  drafts,
  rowStates,
  needle,
  ...handlers
}: RowHandlers & {
  group: string
  rows: Setting[]
  pools: PoolStat[] | undefined
  backends: Set<string>
  drafts: Record<string, string>
  rowStates: Record<string, RowState>
  needle: string
}) {
  return (
    <section aria-labelledby={`heading-${group}`}>
      <SectionHeader group={group} />

      <ul className="divide-y divide-[hsl(var(--border))] overflow-hidden rounded-[var(--radius-md)] border border-[hsl(var(--border))] bg-[hsl(var(--bg-elevated))] elev-1">
        {rows.map((setting) => (
          <SettingRow
            key={setting.key}
            setting={setting}
            draft={drafts[setting.key]}
            state={rowStates[setting.key]}
            pool={poolFor(setting.key, pools)}
            idle={setting.backend !== '' && !backends.has(setting.backend)}
            needle={needle}
            {...handlers}
          />
        ))}
      </ul>
    </section>
  )
}

/** Shared by the settings groups and by presets, which is a section like them. */
function SectionHeader({ group }: { group: string }) {
  const { title, blurb, icon: Icon } = groupMeta(group)
  return (
    <div className="mb-2.5 flex items-start gap-2.5 px-0.5">
      <span className="mt-[1px] flex h-7 w-7 shrink-0 items-center justify-center rounded-[var(--radius-sm)] bg-[hsl(var(--accent-soft))] text-[hsl(var(--accent))]">
        <Icon className="h-[15px] w-[15px]" />
      </span>
      <div className="min-w-0">
        <h2 id={`heading-${group}`} className="text-[13.5px] font-semibold text-fg">
          {title}
        </h2>
        {blurb && (
          <p className="mt-0.5 max-w-2xl text-[11.5px] leading-[1.5] text-subtle">{blurb}</p>
        )}
      </div>
    </div>
  )
}

/** `pool.image.limit` is the limit of a live pool; say what it is doing. */
function poolFor(key: string, pools: PoolStat[] | undefined): PoolStat | undefined {
  const match = /^pool\.(\w+)\.limit$/.exec(key)
  return match ? pools?.find((stat) => stat.pool === match[1]) : undefined
}

/* --------------------------------------------------------------------- row */

type ControlKind = 'switch' | 'segmented' | 'select' | 'stepper' | 'number' | 'text' | 'textarea'

/**
 * The control follows the shape of the value, not its storage type: a fixed set
 * small enough to read at once is worth spending the width on, a bounded integer
 * is worth a pair of nudge buttons, and a paragraph-length string needs room to
 * be read before it is edited.
 */
function controlKind(setting: Setting): ControlKind {
  if (setting.options.length > 0) {
    const short = setting.options.every((option) => option.length <= 9)
    const count = setting.options.length
    // One registered backend is a statement, not a choice, and a segmented
    // control of one reads as a disabled label; leave it a dropdown.
    return count >= 2 && count <= 3 && short ? 'segmented' : 'select'
  }
  if (setting.type === 'bool') return 'switch'
  if (setting.type === 'int') {
    const bounded = setting.min !== setting.max
    return bounded && setting.max - setting.min <= 256 ? 'stepper' : 'number'
  }
  // A float never gets the stepper: its useful steps are tenths, and a control
  // that walks by one would take fifteen clicks to cross 0.5..2.0.
  if (setting.type === 'float') return 'number'
  return setting.value.length > 44 ? 'textarea' : 'text'
}

/** Memoised per row: typing in one field does not re-render the other thirty. */
const SettingRow = memo(function SettingRow({
  setting,
  draft,
  state,
  pool,
  idle,
  needle,
  onDraft,
  onSet,
  onCommit,
  onRevert,
}: RowHandlers & {
  setting: Setting
  draft: string | undefined
  state: RowState | undefined
  pool: PoolStat | undefined
  /** This row's backend is not the one selected, so nothing reads it today. */
  idle: boolean
  needle: string
}) {
  const value = draft ?? setting.value
  const dirty = draft !== undefined && draft !== setting.value
  const kind = controlKind(setting)
  const stacked = kind === 'textarea'
  const id = `setting-${setting.key}`
  const hint = `${id}-hint`

  const commit = () => {
    if (dirty) onCommit(setting.key)
  }
  const onKeyDown = (event: ReactKeyboardEvent) => {
    if (event.key === 'Enter' && kind !== 'textarea') {
      event.preventDefault()
      commit()
    }
    if (event.key === 'Escape' && dirty) {
      event.stopPropagation()
      onRevert(setting.key)
    }
  }

  const control = (
    <SettingControl
      id={id}
      describedBy={hint}
      kind={kind}
      setting={setting}
      value={value}
      dirty={dirty}
      onDraft={(next) => onDraft(setting.key, next)}
      onSet={(next) => onSet(setting.key, next)}
      onCommit={commit}
      onKeyDown={onKeyDown}
    />
  )

  return (
    <li
      className={cn(
        'group/row relative px-4 py-3 transition-colors',
        dirty ? 'bg-[hsl(var(--accent)/0.09)]' : 'hover:bg-[hsl(var(--bg-hover)/0.45)]',
        // Parked, not hidden: the row still holds a value and still saves, and
        // coming back to full strength under the cursor says so.
        idle && 'opacity-55 transition-opacity focus-within:opacity-100 hover:opacity-100',
      )}
    >
      {dirty && (
        <span aria-hidden className="absolute inset-y-0 left-0 w-[2px] bg-[hsl(var(--accent))]" />
      )}

      <div className={cn('flex gap-6', stacked ? 'flex-col gap-2.5' : 'items-start')}>
        <div className="min-w-0 flex-1">
          <div className="flex min-w-0 items-center gap-1.5">
            {/* A segmented control is a tablist, not a labelable element, so the
                key is plain text there and the control carries its own name. */}
            {kind === 'segmented' ? (
              <span className="truncate font-mono text-[12px] font-medium text-fg">
                <Highlight text={setting.key} needle={needle} />
              </span>
            ) : (
              <label htmlFor={id} className="truncate font-mono text-[12px] font-medium text-fg">
                <Highlight text={setting.key} needle={needle} />
              </label>
            )}
            {setting.backend !== '' && <BackendTag name={setting.backend} idle={idle} />}
            <CopyButton
              value={setting.key}
              label="Copy key"
              className="opacity-0 transition-opacity group-hover/row:opacity-100 group-focus-within/row:opacity-100"
            />
            <RowStatus dirty={dirty} state={state} />
          </div>

          <p id={hint} className="mt-1 max-w-xl text-[12px] leading-[1.55] text-muted">
            <Highlight text={setting.description} needle={needle} />
          </p>

          <div className="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-subtle">
            {(setting.type === 'int' || setting.type === 'float') &&
              setting.min !== setting.max && (
                <span className="tabular">
                  {setting.min}–{setting.max}
                </span>
              )}
            {pool && <PoolNote stat={pool} />}
            <span className="tabular opacity-0 transition-opacity group-hover/row:opacity-100">
              written {formatRelative(setting.updatedAt)}
            </span>
          </div>

          {state?.error !== undefined && <ErrorNotice error={state.error} className="mt-2" />}
        </div>

        <div
          className={cn(
            'flex shrink-0 items-center gap-1.5',
            stacked ? 'w-full' : 'w-[264px] justify-end',
          )}
        >
          <div className="min-w-0 flex-1">{control}</div>
          {kind !== 'switch' && kind !== 'segmented' && kind !== 'select' && (
            <div className="flex w-[52px] shrink-0 justify-end gap-1">
              {dirty && (
                <>
                  <Tooltip label="Apply" keys={stacked ? 'mod+s' : 'enter'}>
                    <Button
                      size="icon"
                      variant="primary"
                      onClick={commit}
                      disabled={state?.saving}
                      aria-label={`Apply ${setting.key}`}
                    >
                      <Check className="h-3.5 w-3.5" />
                    </Button>
                  </Tooltip>
                  <Tooltip label="Revert" keys="escape">
                    <Button
                      size="icon"
                      variant="ghost"
                      onClick={() => onRevert(setting.key)}
                      aria-label={`Revert ${setting.key}`}
                    >
                      <RotateCcw className="h-3.5 w-3.5" />
                    </Button>
                  </Tooltip>
                </>
              )}
            </div>
          )}
        </div>
      </div>
    </li>
  )
})

/**
 * Says which backend reads a row, for the rows only one does.
 *
 * It earns its place on the idle case: a `runware.width` that changes nothing
 * because the slide port is pointed at `mock` is the kind of thing an operator
 * otherwise diagnoses by rebuilding the binary.
 */
function BackendTag({ name, idle }: { name: string; idle: boolean }) {
  return (
    <span
      title={
        idle
          ? `Not in use: no port is pointed at ${name}. The value is kept and applies when one is.`
          : `Read by ${name}, the backend selected for this step.`
      }
      className={cn(
        'shrink-0 whitespace-nowrap rounded-full px-1.5 py-[1px] text-[10px] font-medium',
        idle
          ? 'bg-[hsl(var(--fg)/0.07)] text-subtle'
          : 'bg-[hsl(var(--accent-soft))] text-[hsl(var(--accent))]',
      )}
    >
      {name}
      {idle ? ' · idle' : ''}
    </span>
  )
}

function RowStatus({ dirty, state }: { dirty: boolean; state: RowState | undefined }) {
  if (state?.saving) {
    return (
      <Pill tone="muted">
        <Loader2 className="h-3 w-3 animate-spin" aria-hidden />
        Saving
      </Pill>
    )
  }
  if (state?.error !== undefined) {
    return (
      <Pill tone="danger">
        <AlertCircle className="h-3 w-3" aria-hidden />
        Rejected
      </Pill>
    )
  }
  if (state?.applied) {
    return (
      <Pill tone="success">
        <Check className="h-3 w-3" aria-hidden />
        Applied
      </Pill>
    )
  }
  if (dirty) {
    return (
      <Pill tone="warning">
        <span className="h-1.5 w-1.5 rounded-full bg-current" aria-hidden />
        Unsaved
      </Pill>
    )
  }
  return null
}

function Pill({
  tone,
  children,
}: {
  tone: 'muted' | 'success' | 'warning' | 'danger'
  children: ReactNode
}) {
  const tones = {
    muted: 'text-subtle',
    success: 'text-[hsl(var(--success))]',
    warning: 'text-[hsl(var(--warning))]',
    danger: 'text-[hsl(var(--danger))]',
  }
  return (
    <span
      role="status"
      className={cn(
        'animate-in-fade inline-flex shrink-0 items-center gap-1 whitespace-nowrap text-[10.5px] font-medium',
        tones[tone],
      )}
    >
      {children}
    </span>
  )
}

function PoolNote({ stat }: { stat: PoolStat }) {
  const saturated = stat.inFlight >= stat.limit && stat.limit > 0
  return (
    <span
      className={cn('tabular', saturated && stat.queued > 0 ? 'text-[hsl(var(--warning))]' : '')}
    >
      {poolLabel(stat.pool)} pool · {stat.inFlight} busy
      {stat.queued > 0 ? ` · ${stat.queued} queued` : ''}
    </span>
  )
}

/* ---------------------------------------------------------------- controls */

function SettingControl({
  id,
  describedBy,
  kind,
  setting,
  value,
  dirty,
  onDraft,
  onSet,
  onCommit,
  onKeyDown,
}: {
  id: string
  describedBy: string
  kind: ControlKind
  setting: Setting
  value: string
  dirty: boolean
  onDraft: (value: string) => void
  onSet: (value: string) => void
  onCommit: () => void
  onKeyDown: (event: ReactKeyboardEvent) => void
}) {
  switch (kind) {
    case 'switch':
      return (
        <div className="flex items-center justify-end gap-2">
          <span
            className={cn(
              'text-[12px] tabular-nums',
              value === 'true' ? 'font-semibold text-[hsl(var(--accent))]' : 'text-subtle',
            )}
          >
            {value === 'true' ? 'Enabled' : 'Disabled'}
          </span>
          <Switch
            id={id}
            aria-describedby={describedBy}
            checked={value === 'true'}
            onCheckedChange={(next) => onSet(String(next))}
          />
        </div>
      )

    case 'segmented':
      return (
        <Segmented
          aria-label={setting.key}
          className="w-full"
          value={value}
          onChange={onSet}
          options={setting.options.map((option) => ({ value: option, label: option }))}
        />
      )

    case 'select':
      return (
        <Select
          id={id}
          aria-describedby={describedBy}
          className="font-medium"
          value={setting.options.includes(value) ? value : ''}
          onChange={(event) => onSet(event.target.value)}
        >
          {!setting.options.includes(value) && (
            <option value="" disabled>
              {value || '—'}
            </option>
          )}
          {setting.options.map((option) => (
            <option key={option} value={option}>
              {option}
            </option>
          ))}
        </Select>
      )

    case 'stepper':
      return (
        <Stepper
          id={id}
          describedBy={describedBy}
          value={value}
          min={setting.min}
          max={setting.max}
          dirty={dirty}
          onDraft={onDraft}
          onCommit={onCommit}
          onKeyDown={onKeyDown}
        />
      )

    case 'textarea':
      return (
        <Textarea
          id={id}
          aria-describedby={describedBy}
          value={value}
          rows={3}
          spellCheck={false}
          onChange={(event) => onDraft(event.target.value)}
          onBlur={onCommit}
          onKeyDown={onKeyDown}
          className={cn(dirty && 'border-[hsl(var(--accent))]')}
        />
      )

    default:
      return (
        <Input
          id={id}
          aria-describedby={describedBy}
          value={value}
          type={setting.type === 'int' || setting.type === 'float' ? 'number' : 'text'}
          spellCheck={false}
          autoComplete="off"
          {...(setting.type === 'float' ? { step: 'any' } : {})}
          {...((setting.type === 'int' || setting.type === 'float') && setting.min !== setting.max
            ? { min: setting.min, max: setting.max }
            : {})}
          onChange={(event) => onDraft(event.target.value)}
          onBlur={onCommit}
          onKeyDown={onKeyDown}
          className={cn('tabular', dirty && 'border-[hsl(var(--accent))]')}
        />
      )
  }
}

/**
 * A bounded integer with its two useful edits attached. The field stays a text
 * input the operator can type into — the buttons are for the nudge, not a
 * replacement for knowing the number.
 */
function Stepper({
  id,
  describedBy,
  value,
  min,
  max,
  dirty,
  onDraft,
  onCommit,
  onKeyDown,
}: {
  id: string
  describedBy: string
  value: string
  min: number
  max: number
  dirty: boolean
  onDraft: (value: string) => void
  onCommit: () => void
  onKeyDown: (event: ReactKeyboardEvent) => void
}) {
  const current = Number.parseInt(value, 10)
  const valid = Number.isFinite(current)
  const step = (delta: number) => {
    const base = valid ? current : min
    onDraft(String(Math.min(max, Math.max(min, base + delta))))
  }

  return (
    <div
      className={cn(
        'flex h-8 w-full items-center rounded-[var(--radius-sm)] border bg-[hsl(var(--bg))] transition-colors',
        dirty ? 'border-[hsl(var(--accent))]' : 'border-[hsl(var(--border-strong))]',
      )}
    >
      <StepButton
        label="Decrease"
        disabled={valid && current <= min}
        onClick={() => step(-1)}
        icon={<Minus className="h-3 w-3" />}
      />
      <input
        id={id}
        aria-describedby={describedBy}
        type="number"
        inputMode="numeric"
        min={min}
        max={max}
        value={value}
        onChange={(event) => onDraft(event.target.value)}
        onBlur={onCommit}
        onKeyDown={onKeyDown}
        className="tabular h-full w-full min-w-0 border-0 bg-transparent text-center text-[13px] font-medium text-fg [appearance:textfield] focus:outline-none [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
      />
      <StepButton
        label="Increase"
        disabled={valid && current >= max}
        onClick={() => step(1)}
        icon={<Plus className="h-3 w-3" />}
      />
    </div>
  )
}

function StepButton({
  label,
  disabled,
  onClick,
  icon,
}: {
  label: string
  disabled: boolean
  onClick: () => void
  icon: ReactNode
}) {
  return (
    <button
      type="button"
      aria-label={label}
      disabled={disabled}
      onClick={onClick}
      className="flex h-full w-8 shrink-0 items-center justify-center text-subtle transition-colors hover:bg-[hsl(var(--bg-hover))] hover:text-fg disabled:pointer-events-none disabled:opacity-35"
    >
      {icon}
    </button>
  )
}

/* ------------------------------------------------------------ pending bar */

/**
 * The dock that appears the moment anything is unsaved. It is a sibling of the
 * scroll area rather than an overlay on it, so it never covers the row it is
 * talking about.
 */
function PendingBar({
  keys,
  saving,
  onApply,
  onDiscard,
}: {
  keys: string[]
  saving: boolean
  onApply: () => void
  onDiscard: () => void
}) {
  return (
    <div className="animate-in-slide flex shrink-0 items-center gap-3 border-t border-[hsl(var(--border))] bg-[hsl(var(--bg-elevated))] px-4 py-2.5 elev-2">
      <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-[hsl(var(--warning-soft))] text-[hsl(var(--warning))]">
        <Pencil className="h-3.5 w-3.5" />
      </span>
      <div className="min-w-0">
        <p className="text-[12.5px] font-medium text-fg">
          {keys.length} unsaved change{keys.length === 1 ? '' : 's'}
        </p>
        <p className="truncate font-mono text-[11px] text-subtle">{keys.join('  ·  ')}</p>
      </div>
      <div className="ml-auto flex shrink-0 items-center gap-2">
        <Button variant="ghost" size="sm" onClick={onDiscard} disabled={saving}>
          Discard
        </Button>
        <Button variant="primary" size="sm" onClick={onApply} disabled={saving}>
          {saving ? (
            <Loader2 className="h-3.5 w-3.5 animate-spin" aria-hidden />
          ) : (
            <Check className="h-3.5 w-3.5" aria-hidden />
          )}
          Apply all
          <Kbd keys="mod+s" className="ml-0.5 opacity-70" />
        </Button>
      </div>
    </div>
  )
}

/* ------------------------------------------------------------------- misc */

/** Marks every occurrence of the filter, so a match is visible without hunting. */
function Highlight({ text, needle }: { text: string; needle: string }) {
  if (!needle) return <>{text}</>
  const parts: ReactNode[] = []
  const haystack = text.toLowerCase()
  let cursor = 0
  for (;;) {
    const at = haystack.indexOf(needle, cursor)
    if (at < 0) break
    if (at > cursor) parts.push(text.slice(cursor, at))
    parts.push(
      <mark key={at} className="rounded-[2px] bg-[hsl(var(--accent)/0.22)] px-[1px] text-inherit">
        {text.slice(at, at + needle.length)}
      </mark>,
    )
    cursor = at + needle.length
  }
  if (parts.length === 0) return <>{text}</>
  if (cursor < text.length) parts.push(text.slice(cursor))
  return <>{parts}</>
}

export default SettingsRoute
