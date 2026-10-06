import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'

import { followSystem } from '@/v2/core/theme'
import { WorkbenchV2 } from '@/v2/components/workbench'

// Pipeline events keep the cache current; in-process calls aren't retried.
const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 15_000,
      gcTime: 30 * 60_000,
      refetchOnWindowFocus: false,
      retry: false,
    },
  },
})

followSystem()

const container = document.getElementById('root')
if (!container) throw new Error('#root is missing from index.html')

createRoot(container).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <WorkbenchV2 />
    </QueryClientProvider>
  </StrictMode>,
)
