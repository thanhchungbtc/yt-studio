import {
  createRootRoute,
  createRoute,
  createRouter,
  lazyRouteComponent,
  Outlet,
  redirect,
  useRouterState,
} from '@tanstack/react-router'

import { AppShell } from '@/components/app-shell'
import { ChannelsRoute } from '@/routes/channels'
import { SettingsRoute } from '@/routes/settings'
import { VideoDetailRoute } from '@/routes/video-detail'
import { VideosLayout } from '@/routes/videos-layout'
import { VideosStartRoute } from '@/routes/videos-start'

/**
 * The shell wraps every route except the workbench experiment, which draws its
 * own title bar, rail and status bar and would otherwise be nested inside a
 * second set of them.
 */
function Root() {
  const pathname = useRouterState({ select: (state) => state.location.pathname })
  return pathname.startsWith('/workbench') ? <Outlet /> : <AppShell />
}

const rootRoute = createRootRoute({ component: Root })

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  beforeLoad: () => {
    throw redirect({ to: '/videos' })
  },
})

/**
 * `/videos` is a layout, not a page: it owns the channel sidebar and mounts it
 * once for both the start pane and every video. Navigating between two videos
 * therefore swaps only the pane to the right of the splitter — the list keeps
 * its scroll, its filter and its query cache.
 */
const videosLayoutRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/videos',
  component: VideosLayout,
})

const videosIndexRoute = createRoute({
  getParentRoute: () => videosLayoutRoute,
  path: '/',
  component: VideosStartRoute,
})

const videoDetailRoute = createRoute({
  getParentRoute: () => videosLayoutRoute,
  path: '$ref',
  component: VideoDetailRoute,
})

/**
 * The thumbnail editor is its own route rather than a modal over the detail
 * pane: it wants the whole width, it is reached from the upload gate as often
 * as from the artifact list, and being a URL means a half-finished design
 * survives a reload. Split out of the initial bundle — it carries a renderer
 * and most sessions never open it.
 */
const thumbnailEditorRoute = createRoute({
  getParentRoute: () => videosLayoutRoute,
  path: '$ref/thumbnail',
  component: lazyRouteComponent(() => import('@/routes/thumbnail-editor'), 'ThumbnailEditorRoute'),
})

const channelsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/channels',
  component: ChannelsRoute,
})

// The operator console is not on the path to first paint, so it is split out of
// the initial bundle.
const schedulerRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/scheduler',
  component: lazyRouteComponent(() => import('@/routes/scheduler')),
})

/**
 * The section lives in the URL rather than in component state so a reload — or
 * the browser's back button after a jump to the scheduler — puts the operator
 * back on the group they were editing. It is validated loosely on purpose: the
 * groups come from the server's settings table, so this route cannot enumerate
 * them, and an unknown value simply falls back to the first section.
 */
const settingsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/settings',
  component: SettingsRoute,
  validateSearch: (search: Record<string, unknown>): { section?: string } =>
    typeof search.section === 'string' && search.section !== '' ? { section: search.section } : {},
})

// The UI experiment. Split out of the initial bundle — it is a whole second
// shell, and a session that never opens it should not pay for it.
const workbenchRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/workbench',
  component: lazyRouteComponent(() => import('@/routes/workbench'), 'WorkbenchRoute'),
})

const routeTree = rootRoute.addChildren([
  indexRoute,
  videosLayoutRoute.addChildren([videosIndexRoute, videoDetailRoute, thumbnailEditorRoute]),
  channelsRoute,
  schedulerRoute,
  settingsRoute,
  workbenchRoute,
])

export const router = createRouter({
  routeTree,
  defaultPreload: 'intent',
  defaultPreloadStaleTime: 0,
})

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}
