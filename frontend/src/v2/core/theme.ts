import { Events } from '@wailsio/runtime'

import { Accessibility as nativeAccessibility } from '@bindings/services/systemservice'

// WebKit doesn't report Reduce Transparency; the app does.

interface Accessibility {
  reduceTransparency: boolean
  reduceMotion: boolean
  increaseContrast: boolean
}

const QUERIES = {
  dark: '(prefers-color-scheme: dark)',
  reduceTransparency: '(prefers-reduced-transparency: reduce)',
  reduceMotion: '(prefers-reduced-motion: reduce)',
  increaseContrast: '(prefers-contrast: more)',
} as const

let native: Partial<Accessibility> = {}

function matches(query: string): boolean {
  return typeof matchMedia === 'function' && matchMedia(query).matches
}

function apply(): void {
  const root = document.documentElement
  const dark = matches(QUERIES.dark)
  const solid = !!native.reduceTransparency || matches(QUERIES.reduceTransparency)
  const still = !!native.reduceMotion || matches(QUERIES.reduceMotion)
  const contrast = !!native.increaseContrast || matches(QUERIES.increaseContrast)

  const theme = dark ? 'dark' : 'light'
  if (root.dataset.theme !== theme) root.dataset.theme = theme
  root.classList.toggle('dark', dark)
  root.classList.toggle('light', !dark)
  root.dataset.material = solid ? 'solid' : 'liquid'
  root.toggleAttribute('data-glass', !solid)
  root.dataset.motion = still ? 'reduced' : 'spring'
  root.toggleAttribute('data-contrast', contrast)
}

async function refreshNative(): Promise<void> {
  try {
    native = await nativeAccessibility()
  } catch {
    native = {}
  }
  apply()
}

function mark(element: HTMLElement): void {
  const top = element.scrollTop > 1
  const bottom = element.scrollTop + element.clientHeight < element.scrollHeight - 1
  if (element.hasAttribute('data-edge-top') !== top) element.toggleAttribute('data-edge-top', top)
  if (element.hasAttribute('data-edge-bottom') !== bottom) {
    element.toggleAttribute('data-edge-bottom', bottom)
  }
}

function installScrollEdges(): () => void {
  const onScroll = (event: Event) => {
    const target = event.target
    if (target instanceof HTMLElement && target.classList.contains('scroll-edge')) mark(target)
  }
  // Re-mark only on entering a scroller; reading its extent forces layout.
  let current: HTMLElement | null = null
  const onEnter = (event: Event) => {
    const target = event.target
    const element = target instanceof Element ? target.closest<HTMLElement>('.scroll-edge') : null
    if (element === current) return
    current = element
    if (element) mark(element)
  }
  document.addEventListener('scroll', onScroll, { capture: true, passive: true })
  document.addEventListener('pointerover', onEnter, { passive: true })
  return () => {
    document.removeEventListener('scroll', onScroll, { capture: true })
    document.removeEventListener('pointerover', onEnter)
  }
}

/**
 * Applies the system appearance and keeps it applied.
 */
export function followSystem(): () => void {
  apply()
  void refreshNative()

  const lists = Object.values(QUERIES).map((query) => matchMedia(query))
  for (const list of lists) list.addEventListener('change', apply)

  const offNative = Events.On('system:accessibility', (event) => {
    native = event.data as Accessibility
    apply()
  })

  const offEdges = installScrollEdges()
  const ready = window.setTimeout(
    () => document.documentElement.toggleAttribute('data-ready', true),
    1200,
  )

  return () => {
    for (const list of lists) list.removeEventListener('change', apply)
    offNative()
    offEdges()
    window.clearTimeout(ready)
  }
}
