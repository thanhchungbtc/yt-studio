import { useCallback, useEffect, useSyncExternalStore } from 'react'

/**
 * The theme, which is a property of the document rather than of any one UI —
 * both shells read it, and `main.tsx` applies it before React mounts so there is
 * no flash of the wrong one.
 *
 * Self-contained on purpose: it is the one preference that has to be readable
 * from outside React, so it does not sit on top of a hook.
 */

export type Theme = 'dark' | 'light'

const KEY = 'yt-studio.theme'

export function applyTheme(theme: Theme): void {
  document.documentElement.classList.toggle('dark', theme === 'dark')
  document.documentElement.classList.toggle('light', theme === 'light')
}

function read(): Theme {
  try {
    // A build older than the JSON-encoding store wrote the bare word, so both
    // spellings are accepted.
    const raw = localStorage.getItem(KEY)?.replace(/^"|"$/g, '')
    if (raw === 'dark' || raw === 'light') return raw
  } catch {
    /* private browsing */
  }
  return document.documentElement.classList.contains('dark') ? 'dark' : 'light'
}

let current: Theme | null = null
const listeners = new Set<() => void>()

function snapshot(): Theme {
  if (current === null) current = read()
  return current
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

export function useTheme(): [Theme, () => void] {
  const theme = useSyncExternalStore(subscribe, snapshot, () => 'dark' as Theme)

  useEffect(() => applyTheme(theme), [theme])

  const toggle = useCallback(() => {
    current = snapshot() === 'dark' ? 'light' : 'dark'
    try {
      localStorage.setItem(KEY, JSON.stringify(current))
    } catch {
      /* the preference simply does not persist */
    }
    for (const listener of listeners) listener()
  }, [])

  return [theme, toggle]
}
