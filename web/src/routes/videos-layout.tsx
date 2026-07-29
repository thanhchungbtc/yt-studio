import { Outlet, useRouterState } from '@tanstack/react-router'
import { PanelLeft } from 'lucide-react'

import { VideoSidebar } from '@/components/video-sidebar'
import { Button } from '@/components/ui/button'
import { Tooltip } from '@/components/ui/primitives'
import { Splitter } from '@/components/ui/splitter'
import { useHotkeys } from '@/lib/hotkeys'
import { SIDEBAR_MAX, SIDEBAR_MIN, SIDEBAR_DEFAULT, useSidebar } from '@/lib/workspace'

/**
 * The `/videos` workspace: sidebar, splitter, detail.
 *
 * This is a layout route, so the sidebar is mounted once for both `/videos` and
 * `/videos/$ref`. Switching video swaps only what is to the right of the
 * splitter — the list does not remount, does not refetch and does not lose its
 * scroll position, which is what makes moving between two videos feel like
 * changing tabs in an application rather than following a link.
 */
export function VideosLayout() {
  const { width, collapsed, resize, toggle } = useSidebar()
  const activeRef = useRouterState({
    select: (state) => {
      const match = /^\/videos\/([^/]+)/.exec(state.location.pathname)
      return match?.[1] ? decodeURIComponent(match[1]) : undefined
    },
  })

  useHotkeys([{ keys: 'mod+b', label: 'Show or hide the sidebar', group: 'Sidebar', run: toggle }])

  return (
    <div className="flex min-h-0 flex-1">
      {collapsed ? (
        // Collapsed, the sidebar leaves a rail behind so it can be brought back
        // from where it went, rather than only from the keyboard.
        <div className="flex w-8 shrink-0 flex-col items-center border-r border-[hsl(var(--border))] bg-panel pt-2">
          <Tooltip label="Show the sidebar" keys="mod+b" side="right">
            <Button size="icon" variant="ghost" aria-label="Show the sidebar" onClick={toggle}>
              <PanelLeft className="h-4 w-4" />
            </Button>
          </Tooltip>
        </div>
      ) : (
        <>
          <div className="min-h-0 shrink-0 border-r border-[hsl(var(--border))]" style={{ width }}>
            <VideoSidebar activeRef={activeRef} />
          </div>
          <Splitter
            width={width}
            onResize={resize}
            min={SIDEBAR_MIN}
            max={SIDEBAR_MAX}
            onDoubleClick={() => resize(SIDEBAR_DEFAULT)}
            aria-label="Resize the video list"
          />
        </>
      )}

      <section aria-label="Video" className="flex min-w-0 flex-1 flex-col overflow-hidden bg-app">
        <Outlet />
      </section>
    </div>
  )
}
