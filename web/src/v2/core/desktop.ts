/**
 * The bridge to the native window.
 *
 * A WKWebView is not Chromium, so `-webkit-app-region: drag` does nothing here
 * and the AppKit titlebar is the only region macOS will move the window from —
 * and this window hides it, because the tab strip is drawn by the page. The
 * desktop binary therefore binds two functions onto `window`, and this module
 * is the typed, browser-safe face of them: in a plain browser tab the bindings
 * are absent, every call is a no-op, and the workbench renders the same as it
 * does inside the app.
 *
 * Both calls are a request, not a recipe. The page says *a drag started here*
 * and AppKit runs the gesture; the page does not compute where the window
 * should go. That is what buys edge snapping, dragging between displays of
 * different scale, carrying the window to another Space and the tiling
 * gestures — none of which a hand-rolled loop over pointer deltas can imitate,
 * and all of which are noticed the moment they are missing.
 */

interface DesktopBindings {
  /** Run a window drag from the gesture in progress. Resolves at mouse-up. */
  ytsWindowDrag?: () => Promise<boolean>
  /** Zoom the window, as a double-click on the AppKit titlebar would. */
  ytsWindowZoom?: () => Promise<void>
}

function bindings(): DesktopBindings {
  return window as unknown as DesktopBindings
}

/** Hands the gesture under the pointer to AppKit. A no-op in a browser tab. */
export function beginWindowDrag(): void {
  void bindings().ytsWindowDrag?.()
}

/** Zooms the window; the double-click half of the titlebar contract. */
export function zoomWindow(): void {
  void bindings().ytsWindowZoom?.()
}
