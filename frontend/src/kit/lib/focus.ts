/**
 * For Radix onCloseAutoFocus: overlays give focus back to their trigger
 * when they close, unless something else took it meanwhile (an inline name
 * field opened from a menu, a dialog, the composer). This runs once the
 * overlay is gone — after its closing animation — so any focused element
 * (other than another open menu) was focused on purpose.
 */
export function keepFocusIfMoved(e: Event) {
  const active = document.activeElement;
  if (active && active !== document.body && !active.closest("[role='menu']")) e.preventDefault();
}

export function afterMenus(fn: () => void, until = performance.now() + 500) {
  if (!document.querySelector("[role='menu']") || performance.now() > until) return fn();
  requestAnimationFrame(() => afterMenus(fn, until));
}
