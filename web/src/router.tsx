import { createRootRoute, createRouter } from '@tanstack/react-router'

import { WorkbenchV2 } from '@/v2/components/workbench'

/**
 * One route, because there is one window.
 *
 * The workbench keeps its own navigation — the source list, the tab strip — and
 * remembers what is open across reloads, so a URL per document would be a
 * second, competing source of truth.
 */
const rootRoute = createRootRoute({ component: WorkbenchV2 })

export const router = createRouter({ routeTree: rootRoute })

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}
