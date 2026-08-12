import { create } from 'zustand'
import { persist } from 'zustand/middleware'

/**
 * Everything the window remembers: which documents are open, how they are
 * arranged, and which panels are showing.
 *
 * One store rather than a scatter of persisted keys, because after tabs these
 * facts stopped being independent — closing a tab picks the next active one,
 * emptying a group removes it, splitting moves focus. Those are transitions on
 * a single value, and modelling them as separate `useState`s is how you get a
 * window that can be in a state it should not be able to reach.
 */

/* ------------------------------------------------------------------- docs */

export type Doc =
  | { kind: 'welcome' }
  | { kind: 'video'; ref: string }
  | { kind: 'channel'; slug: string }
  | { kind: 'thumbnail'; ref: string }
  | { kind: 'settings' }

/** A document's identity, and therefore its tab's. Opening the same doc twice in
 *  a group reuses the tab rather than making a second one. */
export function docId(doc: Doc): string {
  switch (doc.kind) {
    case 'video':
      return `video:${doc.ref}`
    case 'channel':
      return `channel:${doc.slug}`
    case 'thumbnail':
      // Distinct from the video's own tab: the editor is a different document
      // about the same video, and both can be open at once.
      return `thumbnail:${doc.ref}`
    default:
      return doc.kind
  }
}

/** What the tab shows. Short on purpose — a tab strip is a ledger, not a title. */
export function docTitle(doc: Doc): string {
  switch (doc.kind) {
    case 'video':
      return doc.ref
    case 'channel':
      return doc.slug
    case 'thumbnail':
      return `${doc.ref} thumbnail`
    case 'settings':
      return 'Settings'
    case 'welcome':
      return 'Welcome'
  }
}

/* ------------------------------------------------------------------- tabs */

export interface Tab {
  id: string
  doc: Doc
  /**
   * A preview tab is the single italic slot a group keeps for "I am just
   * looking". The next preview open replaces it in place, which is the whole
   * mechanism that stops a tab strip filling up while you browse.
   */
  preview: boolean
  /** Unsaved edits. A dirty tab is never a preview tab and never closes silently. */
  dirty: boolean
  /** Which section of the document is showing — the second breadcrumb segment. */
  view?: string
}

export interface Group {
  id: string
  tabs: Tab[]
  activeId: string | null
}

export type BottomView = 'console' | 'output'
export type Filter = 'all' | 'live' | 'gated' | 'done' | 'failed'

interface State {
  groups: Group[]
  focusedGroupId: string

  explorerVisible: boolean
  asideVisible: boolean
  bottomVisible: boolean
  bottomView: BottomView

  filter: Filter
  folded: string[]
  /** Blueprint table column widths, by column id. Absent means the default. */
  columnWidths: Record<string, number>

  open: (doc: Doc, options?: { preview?: boolean; groupId?: string }) => void
  pin: (groupId: string, tabId: string) => void
  close: (groupId: string, tabId: string) => void
  closeOthers: (groupId: string, tabId: string) => void
  closeAll: (groupId: string) => void
  activate: (groupId: string, tabId: string) => void
  setDirty: (tabId: string, dirty: boolean) => void
  setView: (tabId: string, view: string) => void
  moveTab: (fromGroup: string, tabId: string, toGroup: string) => void

  split: () => void
  focusGroup: (groupId: string) => void

  toggleExplorer: () => void
  toggleAside: () => void
  toggleBottom: () => void
  showBottom: (view: BottomView) => void

  setColumnWidth: (id: string, width: number) => void
  resetColumnWidth: (id: string) => void

  setFilter: (filter: Filter) => void
  toggleFold: (channelId: string) => void
  setFolded: (folded: string[]) => void
}

function newId(): string {
  return typeof crypto !== 'undefined' && crypto.randomUUID
    ? crypto.randomUUID()
    : `g${Date.now()}${Math.floor(Math.random() * 1e6)}`
}

function emptyGroup(): Group {
  return { id: newId(), tabs: [], activeId: null }
}

/** The group a mutation applies to, with its index, or nothing if it has gone. */
function locate(groups: Group[], groupId: string): { group: Group; index: number } | null {
  const index = groups.findIndex((g) => g.id === groupId)
  const group = groups[index]
  return group ? { group, index } : null
}

function replaceGroup(groups: Group[], index: number, group: Group): Group[] {
  const next = groups.slice()
  next[index] = group
  return next
}

/**
 * Removing a tab has to answer "what is active now?". Right-hand neighbour
 * first, then left — the same answer an editor gives, and the one that keeps a
 * run of closes moving in a single direction.
 */
function withoutTab(group: Group, tabId: string): Group {
  const index = group.tabs.findIndex((t) => t.id === tabId)
  if (index === -1) return group
  const tabs = group.tabs.filter((t) => t.id !== tabId)
  if (group.activeId !== tabId) return { ...group, tabs }
  const next = tabs[index] ?? tabs[index - 1] ?? null
  return { ...group, tabs, activeId: next?.id ?? null }
}

const FIRST_GROUP = emptyGroup()

export const useWorkbenchStore = create<State>()(
  persist(
    (set, get) => ({
      groups: [FIRST_GROUP],
      focusedGroupId: FIRST_GROUP.id,

      explorerVisible: true,
      asideVisible: true,
      bottomVisible: false,
      bottomView: 'console',

      filter: 'all',
      folded: [],
      columnWidths: {},

      open: (doc, options) => {
        const preview = options?.preview ?? true
        const state = get()
        const groupId = options?.groupId ?? (state.focusedGroupId || state.groups[0]?.id || '')
        const found =
          locate(state.groups, groupId) ?? locate(state.groups, state.groups[0]?.id ?? '')
        if (!found) return

        const { group, index } = found
        const id = docId(doc)
        const existing = group.tabs.findIndex((t) => t.id === id)

        if (existing !== -1) {
          // Already open here. A pinning open promotes it; a preview open leaves
          // it exactly as it was — clicking the same row twice must not demote a
          // tab the operator has deliberately kept.
          const tabs = preview
            ? group.tabs
            : group.tabs.map((t) => (t.id === id ? { ...t, preview: false } : t))
          set({
            groups: replaceGroup(state.groups, index, { ...group, tabs, activeId: id }),
            focusedGroupId: group.id,
          })
          return
        }

        const tab: Tab = { id, doc, preview, dirty: false }
        let tabs: Tab[]
        if (preview) {
          // The preview slot keeps its position, so browsing the explorer does
          // not make the tab strip shuffle under the pointer.
          const slot = group.tabs.findIndex((t) => t.preview && !t.dirty)
          tabs = slot === -1 ? [...group.tabs, tab] : replaceAt(group.tabs, slot, tab)
        } else {
          tabs = [...group.tabs, tab]
        }

        set({
          groups: replaceGroup(state.groups, index, { ...group, tabs, activeId: id }),
          focusedGroupId: group.id,
        })
      },

      pin: (groupId, tabId) => {
        const state = get()
        const found = locate(state.groups, groupId)
        if (!found) return
        set({
          groups: replaceGroup(state.groups, found.index, {
            ...found.group,
            tabs: found.group.tabs.map((t) => (t.id === tabId ? { ...t, preview: false } : t)),
          }),
        })
      },

      close: (groupId, tabId) => {
        const state = get()
        const found = locate(state.groups, groupId)
        if (!found) return
        const group = withoutTab(found.group, tabId)
        // An emptied group folds away — unless it is the last one, which stays
        // as the window's floor and shows the welcome document.
        if (group.tabs.length === 0 && state.groups.length > 1) {
          const groups = state.groups.filter((g) => g.id !== groupId)
          set({
            groups,
            focusedGroupId: groups[Math.max(0, found.index - 1)]?.id ?? groups[0]?.id ?? '',
          })
          return
        }
        set({ groups: replaceGroup(state.groups, found.index, group) })
      },

      closeOthers: (groupId, tabId) => {
        const state = get()
        const found = locate(state.groups, groupId)
        if (!found) return
        // Dirty tabs survive: "close others" is a tidying gesture, not a
        // decision to discard work the operator has not been asked about.
        const tabs = found.group.tabs.filter((t) => t.id === tabId || t.dirty)
        set({
          groups: replaceGroup(state.groups, found.index, {
            ...found.group,
            tabs,
            activeId: tabId,
          }),
        })
      },

      closeAll: (groupId) => {
        const state = get()
        const found = locate(state.groups, groupId)
        if (!found) return
        const tabs = found.group.tabs.filter((t) => t.dirty)
        set({
          groups: replaceGroup(state.groups, found.index, {
            ...found.group,
            tabs,
            activeId: tabs[0]?.id ?? null,
          }),
        })
      },

      activate: (groupId, tabId) => {
        const state = get()
        const found = locate(state.groups, groupId)
        if (!found) return
        set({
          groups: replaceGroup(state.groups, found.index, { ...found.group, activeId: tabId }),
          focusedGroupId: groupId,
        })
      },

      /**
       * Dirt pins. A document with unsaved edits must not be the tab that the
       * next single click quietly replaces — this is the one rule the reference
       * has that matters more here than there, because our drafts live in forms
       * rather than in files on disk.
       */
      setDirty: (tabId, dirty) =>
        set((state) => ({
          groups: state.groups.map((group) => ({
            ...group,
            tabs: group.tabs.map((t) =>
              t.id === tabId ? { ...t, dirty, preview: dirty ? false : t.preview } : t,
            ),
          })),
        })),

      setView: (tabId, view) =>
        set((state) => ({
          groups: state.groups.map((group) => ({
            ...group,
            tabs: group.tabs.map((t) => (t.id === tabId ? { ...t, view } : t)),
          })),
        })),

      moveTab: (fromGroup, tabId, toGroup) => {
        const state = get()
        const from = locate(state.groups, fromGroup)
        const to = locate(state.groups, toGroup)
        if (!from || !to || fromGroup === toGroup) return
        const tab = from.group.tabs.find((t) => t.id === tabId)
        if (!tab) return

        let groups = replaceGroup(state.groups, from.index, withoutTab(from.group, tabId))
        const target = groups[to.index]
        if (!target) return
        const existing = target.tabs.some((t) => t.id === tabId)
        groups = replaceGroup(groups, to.index, {
          ...target,
          tabs: existing ? target.tabs : [...target.tabs, tab],
          activeId: tabId,
        })
        // A group emptied by the move folds away, exactly as closing its last
        // tab would.
        if (groups[from.index]?.tabs.length === 0 && groups.length > 1) {
          groups = groups.filter((_, i) => i !== from.index)
        }
        set({ groups, focusedGroupId: toGroup })
      },

      split: () => {
        const state = get()
        // Two groups is the useful case — thumbnail beside its video, settings
        // beside what it changes. Beyond that a 300px column stops being an
        // editor, so the split is capped rather than unbounded.
        if (state.groups.length >= 3) return
        const found = locate(state.groups, state.focusedGroupId)
        const active = found?.group.tabs.find((t) => t.id === found.group.activeId)
        const group: Group = active
          ? { id: newId(), tabs: [{ ...active, preview: false }], activeId: active.id }
          : emptyGroup()
        const index = found ? found.index + 1 : state.groups.length
        const groups = state.groups.slice()
        groups.splice(index, 0, group)
        set({ groups, focusedGroupId: group.id })
      },

      focusGroup: (groupId) => set({ focusedGroupId: groupId }),

      toggleExplorer: () => set((s) => ({ explorerVisible: !s.explorerVisible })),
      toggleAside: () => set((s) => ({ asideVisible: !s.asideVisible })),
      toggleBottom: () => set((s) => ({ bottomVisible: !s.bottomVisible })),
      showBottom: (view) => set({ bottomVisible: true, bottomView: view }),

      setColumnWidth: (id, width) =>
        set((s) => ({ columnWidths: { ...s.columnWidths, [id]: width } })),
      resetColumnWidth: (id) =>
        set((s) => {
          const next = { ...s.columnWidths }
          delete next[id]
          return { columnWidths: next }
        }),

      setFilter: (filter) => set({ filter }),
      toggleFold: (channelId) =>
        set((s) => ({
          folded: s.folded.includes(channelId)
            ? s.folded.filter((id) => id !== channelId)
            : [...s.folded, channelId],
        })),
      setFolded: (folded) => set({ folded }),
    }),
    {
      name: 'yt-studio.wb.layout',
      version: 1,
      // Dirt is a fact about this session's unsaved forms; a reload has already
      // discarded them, so persisting the flag would mark clean tabs dirty.
      partialize: (state) => ({
        groups: state.groups.map((group) => ({
          ...group,
          tabs: group.tabs.map((tab) => ({ ...tab, dirty: false })),
        })),
        focusedGroupId: state.focusedGroupId,
        explorerVisible: state.explorerVisible,
        asideVisible: state.asideVisible,
        bottomVisible: state.bottomVisible,
        bottomView: state.bottomView,
        filter: state.filter,
        folded: state.folded,
        columnWidths: state.columnWidths,
      }),
      merge: (persisted, current) => {
        const stored = persisted as Partial<State> | undefined
        const groups = sanitise(stored?.groups)
        return {
          ...current,
          ...stored,
          groups,
          // A focus that names a group this build no longer has would leave
          // every `open` writing into nowhere.
          focusedGroupId: groups.some((g) => g.id === stored?.focusedGroupId)
            ? (stored?.focusedGroupId ?? '')
            : (groups[0]?.id ?? ''),
        }
      },
    },
  ),
)

function replaceAt<T>(items: T[], index: number, value: T): T[] {
  const next = items.slice()
  next[index] = value
  return next
}

/** localStorage is not a schema. Anything unrecognisable becomes a fresh window. */
function sanitise(groups: unknown): Group[] {
  if (!Array.isArray(groups) || groups.length === 0) return [emptyGroup()]
  const clean: Group[] = []
  for (const raw of groups) {
    if (!raw || typeof raw !== 'object') continue
    const group = raw as Partial<Group>
    if (typeof group.id !== 'string' || !Array.isArray(group.tabs)) continue
    const tabs = group.tabs.filter(isTab).map((tab) => ({ ...tab, dirty: false }))
    clean.push({
      id: group.id,
      tabs,
      activeId: tabs.some((t) => t.id === group.activeId)
        ? (group.activeId ?? null)
        : (tabs[0]?.id ?? null),
    })
  }
  return clean.length > 0 ? clean : [emptyGroup()]
}

function isTab(value: unknown): value is Tab {
  if (!value || typeof value !== 'object') return false
  const tab = value as Partial<Tab>
  if (typeof tab.id !== 'string' || !tab.doc || typeof tab.doc !== 'object') return false
  switch (tab.doc.kind) {
    case 'video':
    case 'thumbnail':
      return typeof tab.doc.ref === 'string' && tab.doc.ref.length > 0
    case 'channel':
      return typeof tab.doc.slug === 'string' && tab.doc.slug.length > 0
    case 'settings':
    case 'welcome':
      return true
    default:
      return false
  }
}

/* ---------------------------------------------------------------- selectors */

export function useFocusedGroup(): Group | undefined {
  return useWorkbenchStore((s) => s.groups.find((g) => g.id === s.focusedGroupId) ?? s.groups[0])
}

/** The document in front in the focused group — what the run panel follows. */
export function useActiveTab(): Tab | undefined {
  const group = useFocusedGroup()
  return group?.tabs.find((t) => t.id === group.activeId)
}

/**
 * The view a document should show: what the tab remembers, else the document's
 * own default. Kept per tab rather than per document type, so two videos open
 * side by side can sit on different sections.
 */
export function resolveView<T extends string>(
  view: string | undefined,
  views: readonly T[],
  fallback: T,
): T {
  return views.includes(view as T) ? (view as T) : fallback
}
