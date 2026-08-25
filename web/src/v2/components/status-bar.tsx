/**
 * The status bar. Deliberately empty.
 *
 * Opaque and full-width, so it reads as the floor the window stands on. What
 * lands here is ambient state — the scheduler, the connection, the version —
 * and none of it is worth showing before it can be shown accurately.
 */
export function StatusBar() {
  return (
    <footer className="surface-chrome-strong hairline-t flex h-[24px] shrink-0 items-center gap-3 px-3 text-[11px] text-tertiary" />
  )
}
