import { createRootRoute, createRouter } from '@tanstack/react-router'

import { Workbench } from '@/components/workbench/workbench'

/**
 * One route, because there is one window.
 *
 * The workbench keeps its own navigation — the explorer, the tab strip and the
 * palette — and remembers what is open across reloads, so a URL per document
 * would be a second, competing source of truth. If deep links are wanted later,
 * the store's `Doc` union is already the shape they would serialise to.
 */
const rootRoute = createRootRoute({ component: Workbench })

export const router = createRouter({ routeTree: rootRoute })

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}
