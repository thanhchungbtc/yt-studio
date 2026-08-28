/**
 * The theme, which is the system's.
 *
 * macOS decides light or dark — in System Settings, or on its own at sunset when
 * Appearance is set to Auto — and the window follows. Nothing is stored and
 * there is nothing to toggle: the whole design of this UI is a native window,
 * and a native window whose appearance disagrees with the desktop it is sitting
 * on is the one thing none of them does. A switch inside the window would be a
 * second answer to a question the platform has already asked.
 *
 * It follows *live*, not once at startup. The material behind the page is an
 * AppKit `NSVisualEffectView`, which re-draws itself the instant the appearance
 * changes; a page that only read the appearance when it loaded would leave dark
 * text on the light material until the next reload.
 *
 * Outside React on purpose. The class has to be on `<html>` before the first
 * component mounts — see `main.tsx` — and the token block it selects is CSS, so
 * every surface in the app is already subscribed to this without asking.
 */

type Theme = 'dark' | 'light'

/** The media query macOS answers; also the thing that tells us it changed. */
function preference(): MediaQueryList {
  return window.matchMedia('(prefers-color-scheme: dark)')
}

function applyTheme(theme: Theme): void {
  document.documentElement.classList.toggle('dark', theme === 'dark')
  document.documentElement.classList.toggle('light', theme === 'light')
}

/**
 * Applies the system appearance and keeps it applied.
 *
 * Returns the unsubscribe for completeness; nothing calls it, because the
 * listener is meant to outlive everything else in the window.
 */
export function followSystem(): () => void {
  const media = preference()
  const sync = () => applyTheme(media.matches ? 'dark' : 'light')
  sync()
  media.addEventListener('change', sync)
  return () => media.removeEventListener('change', sync)
}
