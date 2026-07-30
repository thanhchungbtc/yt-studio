import { useMemo, useState, type ReactNode } from 'react'

import { CommandPalette } from '@/components/command-palette'
import { CreateVideoDialog } from '@/components/create-video-dialog'
import { ShortcutsSheet } from '@/components/shortcuts-sheet'
import { AppCommandsContext, type AppCommands } from '@/lib/app-commands'
import { useHotkeys } from '@/lib/hotkeys'

/**
 * The window-level surfaces — the command palette, the new-video dialog, the
 * shortcuts sheet — mounted once, above every route, and reachable from
 * anywhere without prop drilling, so ⌘K means the same thing on every screen.
 */
export function WorkspaceProvider({ children }: { children: ReactNode }) {
  const [palette, setPalette] = useState(false)
  const [shortcuts, setShortcuts] = useState(false)
  const [createChannel, setCreateChannel] = useState<string | undefined>(undefined)
  const [creating, setCreating] = useState(false)

  const commands = useMemo<AppCommands>(
    () => ({
      openPalette: () => setPalette(true),
      openShortcuts: () => setShortcuts(true),
      openCreateVideo: (channelSlug) => {
        setCreateChannel(channelSlug)
        setCreating(true)
      },
    }),
    [],
  )

  useHotkeys([
    {
      keys: 'mod+k',
      label: 'Command palette',
      group: 'General',
      // Deliberately live while typing: ⌘K from inside a field is the point.
      whileTyping: true,
      run: () => setPalette((prev) => !prev),
    },
    {
      keys: 'mod+n',
      label: 'New video',
      group: 'General',
      run: () => commands.openCreateVideo(),
    },
    {
      keys: 'shift+?',
      label: 'Keyboard shortcuts',
      group: 'General',
      run: () => setShortcuts(true),
    },
  ])

  return (
    <AppCommandsContext.Provider value={commands}>
      {children}
      <CommandPalette open={palette} onOpenChange={setPalette} />
      <CreateVideoDialog
        open={creating}
        onOpenChange={setCreating}
        defaultChannel={createChannel}
      />
      <ShortcutsSheet open={shortcuts} onOpenChange={setShortcuts} />
    </AppCommandsContext.Provider>
  )
}
