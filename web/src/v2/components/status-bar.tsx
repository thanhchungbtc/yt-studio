import { Settings } from 'lucide-react'

import { openSettings } from './settings'
import { HeaderButton } from './ui/header-button'

/**
 * The status bar.
 *
 * Full-width and in the same material as every other pane, so it reads as the
 * floor the window stands on rather than as a strip bolted to the bottom. What
 * else lands here is ambient state — the scheduler, the connection, the version
 * — and none of it is worth showing before it can be shown accurately.
 *
 * The gear is here rather than in the sidebar or a menu because settings belong
 * to the *application*, not to anything you have open. The trailing corner of
 * the floor is where macOS has always put the controls that are about the whole
 * window and nothing in it.
 *
 * It draws its own top edge because it is not in the panel group and so has no
 * sash above it to draw one.
 */
export function StatusBar() {
  return (
    <footer className="surface-chrome hairline-t flex h-[24px] shrink-0 items-center gap-3 px-2 text-[11px] text-tertiary">
      <span className="min-w-0 flex-1" />
      <HeaderButton
        icon={Settings}
        label="Settings"
        onClick={openSettings}
        className="size-[18px]"
      />
    </footer>
  )
}
