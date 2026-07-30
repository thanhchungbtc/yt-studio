import { useCallback, useEffect, useRef, useSyncExternalStore } from 'react'

/**
 * Workspace state: the bits of the window a desktop application remembers
 * between launches — how wide the sidebar is, whether it is collapsed, which
 * channel groups are folded shut, which filter the list is under.
 *
 * Deliberately not server state: one localStorage-backed external store, read
 * synchronously on first render so the layout never flashes at the wrong width,
 * and subscribed to through `useSyncExternalStore` so a drag does not re-render
 * the tree through a context.
 */

const PREFIX = 'yt-studio.'

/** The authoritative in-memory copy. `getSnapshot` must be referentially stable. */
const cache = new Map<string, unknown>()
const listeners = new Map<string, Set<() => void>>()

function read<T>(key: string, fallback: T): T {
  try {
    const raw = localStorage.getItem(PREFIX + key)
    return raw === null ? fallback : (JSON.parse(raw) as T)
  } catch {
    // Private browsing, or a value written by an older build.
    return fallback
  }
}

function snapshot<T>(key: string, fallback: T): T {
  if (!cache.has(key)) cache.set(key, read(key, fallback))
  return cache.get(key) as T
}

function publish<T>(key: string, value: T): void {
  cache.set(key, value)
  try {
    localStorage.setItem(PREFIX + key, JSON.stringify(value))
  } catch {
    // The preference simply does not persist.
  }
  for (const listener of listeners.get(key) ?? []) listener()
}

function subscribe(key: string, listener: () => void): () => void {
  const set = listeners.get(key) ?? new Set()
  set.add(listener)
  listeners.set(key, set)
  return () => set.delete(listener)
}

/** `useState` whose value survives a reload and is shared across components. */
export function usePersisted<T>(
  key: string,
  fallback: T,
): [T, (next: T | ((prev: T) => T)) => void] {
  // Every call site passes a constant default; holding it in a ref keeps the
  // subscription from re-attaching when that default is an array or object
  // literal written inline.
  const fallbackRef = useRef(fallback)

  const value = useSyncExternalStore(
    useCallback((listener: () => void) => subscribe(key, listener), [key]),
    useCallback(() => snapshot(key, fallbackRef.current), [key]),
  )

  const set = useCallback(
    (next: T | ((prev: T) => T)) => {
      const previous = snapshot(key, fallbackRef.current)
      publish(key, typeof next === 'function' ? (next as (p: T) => T)(previous) : next)
    },
    [key],
  )

  return [value, set]
}

/* ------------------------------------------------------------------ theme */

export type Theme = 'dark' | 'light'

export function applyTheme(theme: Theme): void {
  document.documentElement.classList.toggle('dark', theme === 'dark')
  document.documentElement.classList.toggle('light', theme === 'light')
}

export function useTheme(): [Theme, () => void] {
  const [theme, setTheme] = usePersisted<Theme>(
    'theme',
    document.documentElement.classList.contains('dark') ? 'dark' : 'light',
  )

  useEffect(() => applyTheme(theme), [theme])

  const toggle = useCallback(
    () => setTheme((prev) => (prev === 'dark' ? 'light' : 'dark')),
    [setTheme],
  )
  return [theme, toggle]
}

/* -------------------------------------------------------------- geometry */

export const SIDEBAR_MIN = 208
export const SIDEBAR_MAX = 460
export const SIDEBAR_DEFAULT = 288

export function useSidebar() {
  const [width, setWidth] = usePersisted('sidebar.width', SIDEBAR_DEFAULT)
  const [collapsed, setCollapsed] = usePersisted('sidebar.collapsed', false)

  const resize = useCallback(
    (next: number) => setWidth(Math.round(Math.min(SIDEBAR_MAX, Math.max(SIDEBAR_MIN, next)))),
    [setWidth],
  )
  const toggle = useCallback(() => setCollapsed((prev) => !prev), [setCollapsed])

  return { width, collapsed, resize, toggle, setCollapsed }
}
