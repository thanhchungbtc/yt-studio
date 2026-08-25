/**
 * The status bar. Deliberately empty.
 *
 * Full-width and in the same material as every other pane, so it reads as the
 * floor the window stands on rather than as a strip bolted to the bottom. What
 * lands here is ambient state — the scheduler, the connection, the version —
 * and none of it is worth showing before it can be shown accurately.
 *
 * It draws its own top edge because it is not in the panel group and so has no
 * sash above it to draw one.
 */
export function StatusBar() {
  return (
    <footer className="surface-chrome hairline-t flex h-[24px] shrink-0 items-center gap-3 px-3 text-[11px] text-tertiary" />
  )
}
