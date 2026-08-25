import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider } from '@tanstack/react-router'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'

import { applyTheme } from '@/core/theme'
import { router } from '@/router'

// Almost everything on screen is server state, and the SSE stream keeps it
// current. Refetching on focus or on an interval would be duplicated work.
const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 15_000,
      refetchOnWindowFocus: false,
      retry: (failureCount, error) => {
        const status = (error as { status?: number }).status
        if (status && status >= 400 && status < 500) return false
        return failureCount < 2
      },
    },
  },
})

// Applied before React mounts so there is no flash of the wrong theme. The
// workspace store writes JSON, and a build older than it wrote the bare word,
// so both are accepted here.
;(function restoreTheme() {
  let stored: string | null = null
  try {
    stored = localStorage.getItem('yt-studio.theme')
  } catch {
    stored = null
  }
  const theme = stored?.replace(/^"|"$/g, '')
  const dark =
    theme === 'dark' || theme === 'light'
      ? theme === 'dark'
      : window.matchMedia('(prefers-color-scheme: dark)').matches
  applyTheme(dark ? 'dark' : 'light')
})()

const container = document.getElementById('root')
if (!container) throw new Error('#root is missing from index.html')

createRoot(container).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  </StrictMode>,
)
