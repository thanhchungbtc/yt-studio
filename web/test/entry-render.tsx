import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createRoot } from 'react-dom/client'
import { qk } from '@/core/api'
import { Workbench } from '@/components/workbench'
import { useWorkbenchStore, type Doc } from '@/components/lib/store'

export { useWorkbenchStore }

export function mount(el: HTMLElement, seed: Record<string, unknown>, docs: Doc[], bottom?: string) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  const v = seed.video as { id: string; ref: string }
  qc.setQueryData(qk.videos({}), seed.videos)
  qc.setQueryData(qk.channels, seed.channels)
  qc.setQueryData(['health'], seed.health)
  qc.setQueryData(qk.scheduler, seed.scheduler)
  qc.setQueryData(qk.settings, seed.settings)
  qc.setQueryData(qk.recentTasks, seed.recentTasks)
  if (v) {
    qc.setQueryData(qk.video(v.ref), v)
    qc.setQueryData(qk.chapters(v.id), seed.chapters)
    qc.setQueryData(qk.videoTasks(v.id), seed.tasks)
    qc.setQueryData(qk.assets(v.id), seed.assets)
  }
  const ch = seed.channels as { slug: string }[]
  if (ch?.[0]) qc.setQueryData(qk.channel(ch[0].slug), ch[0])

  for (const doc of docs) useWorkbenchStore.getState().open(doc, { preview: false })
  if (bottom) useWorkbenchStore.getState().showBottom(bottom as 'console' | 'output')

  createRoot(el).render(
    <QueryClientProvider client={qc}>
      <Workbench />
    </QueryClientProvider>,
  )
}
