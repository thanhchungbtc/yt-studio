/**
 * The appearance, which is the system's.
 *
 * macOS decides light or dark, and whether the window may be translucent,
 * animate or needs stronger edges — and the window follows, live. Nothing is
 * stored and there is nothing to toggle: a native window whose appearance
 * disagrees with the desktop it is sitting on is the one thing none of them do.
 *
 * Everything lands as attributes on <html>, which the stylesheet keys off:
 *
 *   data-theme     light | dark
 *   data-glass     Liquid Glass surfaces (off under Reduce Transparency)
 *   data-material  liquid | solid
 *   data-motion    spring | reduced
 *   data-contrast  Increase Contrast
 *   data-ready     set shortly after startup, so restored tabs do not animate in
 *
 * The web view's media queries are one source. The desktop binary is the
 * other: WebKit does not report Reduce Transparency, so the shell binds
 * `ytsAccessibility` and fires `yts-accessibility` whenever System Settings
 * changes. In a browser tab the binding is absent and the media queries alone
 * decide.
 *
 * Outside React on purpose: the attributes have to be on <html> before the
 * first component mounts, so nothing paints in the wrong material.
 */

interface Accessibility {
  reduceTransparency: boolean
  reduceMotion: boolean
  increaseContrast: boolean
}

interface AccessibilityBinding {
  ytsAccessibility?: () => Promise<Partial<Accessibility>>
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
  // The class is kept for anything that still selects on it.
  root.classList.toggle('dark', dark)
  root.classList.toggle('light', !dark)
  root.dataset.material = solid ? 'solid' : 'liquid'
  root.toggleAttribute('data-glass', !solid)
  root.dataset.motion = still ? 'reduced' : 'spring'
  root.toggleAttribute('data-contrast', contrast)
}

/** Asks the desktop shell, when there is one, and re-applies. */
async function refreshNative(): Promise<void> {
  const ask = (window as unknown as AccessibilityBinding).ytsAccessibility
  if (!ask) return
  try {
    native = (await ask()) ?? {}
  } catch {
    native = {}
  }
  apply()
}

/**
 * Soft scroll edges: marks `.scroll-edge` scrollers with data-edge-top /
 * data-edge-bottom while content lies beyond that edge, so the stylesheet can
 * fade it out under headers. One passive, capturing listener serves the whole
 * app, and marks are only written when they flip.
 */
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
  // Content can grow without scrolling: refresh when the pointer comes back.
  // Only on arrival, not on every element crossed inside: reading a scroller's
  // extent forces layout, and `pointerover` fires for each row the pointer
  // passes over.
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
 *
 * Returns the unsubscribe for completeness; nothing calls it, because the
 * listeners are meant to outlive everything else in the window.
 */
export function followSystem(): () => void {
  apply()
  void refreshNative()

  const lists = Object.values(QUERIES).map((query) => matchMedia(query))
  for (const list of lists) list.addEventListener('change', apply)

  const onNative = () => void refreshNative()
  window.addEventListener('yts-accessibility', onNative)
  // Belt and braces: System Settings is usually in front while the switch is
  // flipped, so coming back to the window is when it matters.
  window.addEventListener('focus', onNative)

  const offEdges = installScrollEdges()
  // Overlays and restored tabs animate in only after startup.
  const ready = window.setTimeout(
    () => document.documentElement.toggleAttribute('data-ready', true),
    1200,
  )

  return () => {
    for (const list of lists) list.removeEventListener('change', apply)
    window.removeEventListener('yts-accessibility', onNative)
    window.removeEventListener('focus', onNative)
    offEdges()
    window.clearTimeout(ready)
  }
}
