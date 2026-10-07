import { CircleDot, Search, X } from 'lucide-react'
import {
  useDeferredValue,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent,
} from 'react'

import { cn } from '@/kit/lib/cn'
import { Button } from '@/kit/ui/Button'

import { openDoc } from '../editor/dock'
import { QueryContext } from './controls'
import { isModified, useChangedPrefs, useSettingRows } from './data'
import { isSearching, MODIFIED_FILTER, parseQuery, SECTIONS } from './schema'
import { AboutSection, AppearanceSection, GroupSection, KeyboardSection } from './sections'

let pendingSection: string | undefined

/** Opens Settings as a tab, at a section if one is given. ⌘, */
export function openSettings(section?: string): void {
  pendingSection = section
  openDoc({ kind: 'settings' }, 'Settings')
  if (section) window.dispatchEvent(new CustomEvent('settings:section', { detail: section }))
}

const reducedMotion = () => document.documentElement.dataset.motion === 'reduced'

function countHits(root: HTMLElement): Record<string, number> {
  const next: Record<string, number> = {}
  root
    .querySelectorAll<HTMLElement>('[data-settings-section]')
    .forEach(
      (el) =>
        (next[el.dataset.settingsSection!] = el.querySelectorAll('[data-setting-row]').length),
    )
  return next
}

export function SettingsPanel() {
  const [text, setText] = useState('')
  const deferredText = useDeferredValue(text)
  const query = useMemo(() => parseQuery(deferredText), [deferredText])
  const searching = isSearching(query)
  const [active, setActive] = useState(SECTIONS[0]!.id)
  const [hits, setHits] = useState<Record<string, number>>({})
  const scroller = useRef<HTMLDivElement>(null)
  const search = useRef<HTMLInputElement>(null)
  const nav = useRef<HTMLDivElement>(null)

  const settings = useSettingRows()
  const rows = useMemo(() => settings.data ?? [], [settings.data])
  const chosen = useMemo(
    () => new Set(rows.filter((r) => r.key.startsWith('provider.')).map((r) => r.value)),
    [rows],
  )
  const changedPrefs = useChangedPrefs()
  const changedFor = (id: string): number => {
    const group = SECTIONS.find((s) => s.id === id)?.group
    if (!group) return 0
    return rows.filter(
      (r) => r.group === group && (r.backend === '' || chosen.has(r.backend)) && isModified(r),
    ).length
  }
  const changedCount =
    changedPrefs +
    rows.filter((r) => (r.backend === '' || chosen.has(r.backend)) && isModified(r)).length

  // Sections that have nothing to show right now (a group whose backends are all idle).
  const visible = SECTIONS.filter(
    (s) =>
      !s.group ||
      rows.some((r) => r.group === s.group && (r.backend === '' || chosen.has(r.backend))),
  )

  useLayoutEffect(() => {
    const root = scroller.current
    if (!root) return
    setHits(countHits(root))
    root.scrollTop = 0
  }, [query, rows])

  const picked = useRef<string | undefined>(undefined)
  const searchingRef = useRef(searching)
  searchingRef.current = searching
  useEffect(() => {
    const root = scroller.current
    if (!root) return
    const seen = new Set<string>()
    const io = new IntersectionObserver(
      (entries) => {
        for (const e of entries) {
          const id = (e.target as HTMLElement).dataset.settingsSection!
          if (e.isIntersecting) seen.add(id)
          else seen.delete(id)
        }
        if (picked.current || searchingRef.current) return
        const top = SECTIONS.find((s) => seen.has(s.id))
        if (top) setActive(top.id)
      },
      { root, rootMargin: '-24px 0px -66% 0px' },
    )
    root.querySelectorAll('[data-settings-section]').forEach((el) => io.observe(el))
    return () => io.disconnect()
  }, [rows.length])

  const go = (id: string) => {
    if (text) setText('')
    setActive(id)
    picked.current = id
    requestAnimationFrame(() => {
      scroller.current
        ?.querySelector(`[data-settings-section="${id}"]`)
        ?.scrollIntoView({ block: 'start', behavior: reducedMotion() ? 'auto' : 'smooth' })
    })
  }

  useEffect(() => {
    const first = pendingSection
    pendingSection = undefined
    if (first) go(first)
    const onSection = (e: Event) => {
      pendingSection = undefined
      go((e as CustomEvent<string>).detail)
    }
    window.addEventListener('settings:section', onSection)
    return () => window.removeEventListener('settings:section', onSection)
  }, [])

  const onScroll = () => {
    const root = scroller.current
    if (!root || searching || picked.current) return
    if (root.scrollTop + root.clientHeight >= root.scrollHeight - 4)
      setActive(visible[visible.length - 1]!.id)
  }

  const toggleModified = () => {
    const rest = text
      .split(/\s+/)
      .filter((t) => t && t.toLowerCase() !== MODIFIED_FILTER)
      .join(' ')
    setText(query.modified ? rest : `${MODIFIED_FILTER} ${rest}`.trimEnd() + ' ')
    search.current?.focus()
  }

  const onNavKey = (e: KeyboardEvent) => {
    const buttons = [
      ...(nav.current?.querySelectorAll<HTMLButtonElement>('[data-section-link]') ?? []),
    ]
    const i = buttons.indexOf(document.activeElement as HTMLButtonElement)
    const next =
      e.key === 'ArrowDown' || e.key === 'ArrowRight'
        ? i + 1
        : e.key === 'ArrowUp' || e.key === 'ArrowLeft'
          ? i - 1
          : e.key === 'Home'
            ? 0
            : e.key === 'End'
              ? buttons.length - 1
              : -2
    if (next === -2 || i < 0) return
    e.preventDefault()
    const b = buttons[(next + buttons.length) % buttons.length]!
    b.focus()
    go(b.dataset.sectionLink!)
  }

  const total = Object.values(hits).reduce((a, b) => a + b, 0)
  const empty = searching && total === 0

  return (
    <div className="@container/settings h-full bg-content">
      <div
        className="flex h-full @max-[40rem]/settings:flex-col"
        onKeyDown={(e) => {
          if (e.metaKey && e.key === 'f') {
            e.preventDefault()
            search.current?.focus()
            search.current?.select()
          }
        }}
      >
        <nav
          className="flex w-56 shrink-0 flex-col border-r border-line px-2.5 pt-5 @max-[40rem]/settings:w-full @max-[40rem]/settings:border-r-0 @max-[40rem]/settings:border-b @max-[40rem]/settings:pt-3 @max-[40rem]/settings:pb-2"
          aria-label="Settings sections"
        >
          <label className="mb-3 flex h-7 shrink-0 items-center gap-1.5 rounded-full bg-well pr-1 pl-2.5 hairline focus-within:shadow-[0_0_0_2px_var(--focus)] @max-[40rem]/settings:mb-2">
            <Search className="size-3.5 shrink-0 text-fg-subtle" />
            <input
              ref={search}
              value={text}
              onChange={(e) => setText(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Escape') {
                  if (text) setText('')
                  else search.current?.blur()
                }
              }}
              placeholder="Search settings"
              aria-label="Search settings"
              spellCheck={false}
              className="min-w-0 flex-1 bg-transparent text-xs text-fg outline-none placeholder:text-fg-subtle"
            />
            {text && (
              <button
                aria-label="Clear search"
                onClick={() => setText('')}
                className="flex size-4.5 shrink-0 items-center justify-center rounded-full text-fg-subtle hover:bg-hover hover:text-fg"
              >
                <X className="size-3" />
              </button>
            )}
          </label>
          <div
            ref={nav}
            onKeyDown={onNavKey}
            className="flex flex-col gap-0.5 overflow-y-auto @max-[40rem]/settings:-mx-1 @max-[40rem]/settings:flex-row @max-[40rem]/settings:overflow-x-auto @max-[40rem]/settings:px-1 @max-[40rem]/settings:[scrollbar-width:none]"
          >
            {visible.map((s) => {
              const Icon = s.icon
              const current = !searching && active === s.id
              const dim = searching && !hits[s.id]
              const changed = s.id === 'appearance' ? changedPrefs > 0 : changedFor(s.id) > 0
              return (
                <button
                  key={s.id}
                  data-section-link={s.id}
                  onClick={() => go(s.id)}
                  aria-current={current ? 'true' : undefined}
                  tabIndex={active === s.id ? 0 : -1}
                  className={cn(
                    'flex h-8 shrink-0 items-center gap-2.5 rounded-[9px] px-2 text-left text-sm transition-[background-color,color,opacity] duration-100 focus-visible:shadow-[0_0_0_2px_var(--focus)] focus-visible:outline-none',
                    current
                      ? 'bg-[var(--accent-soft)] text-fg'
                      : 'text-fg-muted hover:bg-hover hover:text-fg',
                    dim && 'opacity-40',
                  )}
                >
                  <span
                    className="flex size-5 shrink-0 items-center justify-center rounded-[6px] text-white"
                    style={{ background: s.color }}
                  >
                    <Icon className="size-3" strokeWidth={2.2} />
                  </span>
                  <span className="truncate">{s.title}</span>
                  {searching
                    ? (hits[s.id] ?? 0) > 0 && (
                        <span className="ml-auto pl-1 text-2xs text-fg-subtle tabular-nums">
                          {hits[s.id]}
                        </span>
                      )
                    : changed && (
                        <span
                          className="ml-auto size-1.5 shrink-0 rounded-full bg-accent @max-[40rem]/settings:ml-0"
                          aria-label="Has changed settings"
                        />
                      )}
                </button>
              )
            })}
          </div>
          <div className="mt-auto border-t border-line pt-2 pb-3 @max-[40rem]/settings:hidden">
            <button
              onClick={toggleModified}
              aria-pressed={query.modified}
              disabled={!changedCount && !query.modified}
              className={cn(
                'flex h-8 w-full items-center gap-2.5 rounded-[9px] px-2 text-left text-sm transition-colors duration-100 disabled:opacity-40',
                query.modified
                  ? 'bg-[var(--accent-soft)] text-fg'
                  : 'text-fg-muted hover:bg-hover hover:text-fg',
              )}
            >
              <CircleDot className="size-4 shrink-0 text-accent" />
              Changed settings
              <span className="ml-auto text-2xs text-fg-subtle tabular-nums">{changedCount}</span>
            </button>
          </div>
        </nav>
        <div
          ref={scroller}
          onScroll={onScroll}
          onWheel={() => (picked.current = undefined)}
          onPointerDown={() => (picked.current = undefined)}
          className={cn(
            'scroll-edge @container/content min-h-0 min-w-0 flex-1 overflow-auto [scrollbar-gutter:stable]',
            searching && 'settings-searching',
          )}
        >
          <QueryContext.Provider value={query}>
            <div className="mx-auto max-w-2xl px-8 pt-7 pb-[40vh] @max-[30rem]/content:px-4">
              {empty && (
                <div className="flex flex-col items-center gap-3 py-20 text-center">
                  <Search className="size-7 text-fg-faint" strokeWidth={1.5} />
                  <div className="text-sm text-fg-muted">
                    {query.modified && !query.words.length
                      ? 'Every setting is at its default.'
                      : `No settings match “${query.words.join(' ')}”.`}
                  </div>
                  <Button size="sm" variant="ghost" onClick={() => setText('')}>
                    Clear search
                  </Button>
                </div>
              )}
              {SECTIONS.map((s) => {
                if (s.id === 'appearance') return <AppearanceSection key={s.id} />
                if (s.id === 'keyboard') return <KeyboardSection key={s.id} />
                if (s.id === 'about') return <AboutSection key={s.id} />
                return s.group ? (
                  <GroupSection key={s.id} id={s.id} group={s.group} rows={rows} />
                ) : null
              })}
            </div>
          </QueryContext.Provider>
        </div>
      </div>
    </div>
  )
}
