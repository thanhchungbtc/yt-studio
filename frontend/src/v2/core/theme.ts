import { Events } from '@wailsio/runtime'
import { create } from 'zustand'

import { Accessibility, SetLook } from '@bindings/services/systemservice'
import { installScrollEdges } from '@/kit/lib/scrollEdges'
import { getPref, setPref, usePrefs } from '@/prefs'

interface A11y {
  reduceTransparency: boolean
  reduceMotion: boolean
  increaseContrast: boolean
}

const media = (q: string) => typeof matchMedia === 'function' && matchMedia(q).matches
const darkQuery = matchMedia('(prefers-color-scheme: dark)')

// macOS's settings win; the web view's media queries are the fallback.
const useA11y = create<A11y>(() => ({
  reduceTransparency: media('(prefers-reduced-transparency: reduce)'),
  reduceMotion: media('(prefers-reduced-motion: reduce)'),
  increaseContrast: media('(prefers-contrast: more)'),
}))

function setA11y(o: Partial<A11y>) {
  useA11y.setState({
    reduceTransparency: !!o.reduceTransparency || media('(prefers-reduced-transparency: reduce)'),
    reduceMotion: !!o.reduceMotion || media('(prefers-reduced-motion: reduce)'),
    increaseContrast: !!o.increaseContrast || media('(prefers-contrast: more)'),
  })
}

export type ResolvedTheme = 'light' | 'dark'

/** The theme in effect, for components that need it (the toaster). */
export const useResolvedTheme = create<{ theme: ResolvedTheme }>(() => ({ theme: 'light' }))

function resolvedTheme(): ResolvedTheme {
  const theme = getPref('theme')
  if (theme !== 'system') return theme
  return darkQuery.matches ? 'dark' : 'light'
}

/** Flips between light and dark, starting from what is shown. */
export function toggleTheme(): void {
  setPref('theme', resolvedTheme() === 'dark' ? 'light' : 'dark')
}

function apply(): void {
  const root = document.documentElement
  const { reduceTransparency, reduceMotion, increaseContrast } = useA11y.getState()
  const { material, fontSize } = usePrefs.getState()
  const theme = resolvedTheme()
  if (root.dataset.theme !== theme) root.dataset.theme = theme
  if (useResolvedTheme.getState().theme !== theme) useResolvedTheme.setState({ theme })
  root.classList.toggle('dark', theme === 'dark')
  root.classList.toggle('light', theme !== 'dark')
  const glass = material === 'liquidGlass' && !reduceTransparency
  root.dataset.material = reduceTransparency
    ? 'solid'
    : material === 'liquidGlass'
      ? 'liquid'
      : material
  root.toggleAttribute('data-glass', glass)
  root.dataset.motion = reduceMotion ? 'reduced' : glass ? 'spring' : 'calm'
  root.toggleAttribute('data-contrast', increaseContrast)
  ;(root.style as CSSStyleDeclaration & { zoom: string }).zoom = String(fontSize / 13)
}

/** Applies the appearance and keeps it applied. */
export function followSystem(): () => void {
  apply()
  void Accessibility()
    .then(setA11y)
    .catch(() => undefined)
  const offA11y = Events.On('system:accessibility', (e) => setA11y(e.data as A11y))
  const offA11yStore = useA11y.subscribe(apply)
  darkQuery.addEventListener('change', apply)

  let look = ''
  const offPrefs = usePrefs.subscribe((s) => {
    apply()
    const next = `${s.theme}|${s.material}`
    if (next !== look) {
      look = next
      void SetLook(s.theme, s.material).catch(() => undefined)
    }
  })

  const offEdges = installScrollEdges()
  const ready = window.setTimeout(
    () => document.documentElement.toggleAttribute('data-ready', true),
    1200,
  )

  return () => {
    offA11y()
    offA11yStore()
    offPrefs()
    darkQuery.removeEventListener('change', apply)
    offEdges()
    window.clearTimeout(ready)
  }
}
