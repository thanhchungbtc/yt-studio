import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createRoot } from 'react-dom/client'

import { openDoc } from '@/v2/components/editor/dock'
import { WorkbenchV2 } from '@/v2/components/workbench'
import { qk } from '@/v2/core/api'
import type { Channel, Video } from '@/v2/core/types'
import { useWorkbench } from '@/v2/store/workbench'

export { openDoc, useWorkbench }

interface Seed {
  channels: Channel[]
  videos: Video[]
}

/**
 * Mounts Workbench V2 against a pre-seeded cache.
 *
 * The cache is filled from a live server rather than from fixtures, so the
 * sidebar groups real channels around real videos and the row that renders is
 * the row an operator would see.
 */
export function mount(el: HTMLElement, seed: Seed): void {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  client.setQueryData(qk.channels, seed.channels)
  client.setQueryData(qk.videos, seed.videos)

  createRoot(el).render(
    <QueryClientProvider client={client}>
      <WorkbenchV2 />
    </QueryClientProvider>,
  )
}
