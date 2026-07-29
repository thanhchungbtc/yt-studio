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
import { VideosRoute } from '@/routes/videos'

const rootRoute = createRootRoute({ component: AppShell })

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  beforeLoad: () => {
    throw redirect({ to: '/videos' })
  },
})

const videosRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/videos',
  component: VideosRoute,
})

const videoDetailRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/videos/$ref',
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
  videosRoute,
  videoDetailRoute,
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
