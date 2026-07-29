import {
  createRootRoute,
  createRoute,
  createRouter,
  lazyRouteComponent,
  redirect,
} from '@tanstack/react-router'

import { AppShell } from '@/components/app-shell'
import { ChannelsRoute } from '@/routes/channels'
import { SettingsRoute } from '@/routes/settings'
import { VideoDetailRoute } from '@/routes/video-detail'
import { VideosLayout } from '@/routes/videos-layout'
import { VideosStartRoute } from '@/routes/videos-start'

const rootRoute = createRootRoute({ component: AppShell })

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

const channelsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/channels',
  component: ChannelsRoute,
})

// The operator console is not on the path to first paint, so it is split out of
// the initial bundle (§9).
const schedulerRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/scheduler',
  component: lazyRouteComponent(() => import('@/routes/scheduler')),
})

const settingsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/settings',
  component: SettingsRoute,
})

const routeTree = rootRoute.addChildren([
  indexRoute,
  videosLayoutRoute.addChildren([videosIndexRoute, videoDetailRoute]),
  channelsRoute,
  schedulerRoute,
  settingsRoute,
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
