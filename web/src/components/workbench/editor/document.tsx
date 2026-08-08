import { useCallback } from 'react'

import { ChannelDoc } from '../documents/channel-doc'
import { SettingsDoc } from '../documents/settings-doc'
import { VideoDoc } from '../documents/video-doc'
import { Welcome } from '../documents/welcome'
import { useWorkbenchStore, type Tab } from '../lib/store'

/**
 * Which document a tab shows. The only place the `Doc` union is turned into a
 * component, so adding a document type is one case here and one file beside the
 * others.
 */
export function Document({
  tab,
  onNewVideo,
  onOpenPalette,
}: {
  tab: Tab
  onNewVideo: (channel?: string) => void
  onOpenPalette: () => void
}) {
  const setView = useWorkbenchStore((s) => s.setView)
  const setDirty = useWorkbenchStore((s) => s.setDirty)

  const onView = useCallback((view: string) => setView(tab.id, view), [setView, tab.id])
  const onDirty = useCallback((dirty: boolean) => setDirty(tab.id, dirty), [setDirty, tab.id])

  switch (tab.doc.kind) {
    case 'video':
      return <VideoDoc videoRef={tab.doc.ref} view={tab.view} onView={onView} />
    case 'channel':
      return <ChannelDoc slug={tab.doc.slug} onNewVideo={onNewVideo} onDirty={onDirty} />
    case 'settings':
      return <SettingsDoc view={tab.view} onView={onView} />
    case 'welcome':
      return <Welcome onNewVideo={() => onNewVideo()} onOpenPalette={onOpenPalette} />
  }
}
