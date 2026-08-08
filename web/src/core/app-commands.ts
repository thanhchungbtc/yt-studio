import { createContext, useContext } from 'react'

/**
 * The window-level actions: things that mean the same thing on every screen and
 * are therefore owned by the shell rather than by a route.
 *
 * The context lives in its own module so the palette — which consumes these
 * commands and is mounted by their provider — does not import the provider back.
 */
export interface AppCommands {
  openPalette: () => void
  openCreateVideo: (channelSlug?: string) => void
  openShortcuts: () => void
}

export const AppCommandsContext = createContext<AppCommands | null>(null)

export function useAppCommands(): AppCommands {
  const value = useContext(AppCommandsContext)
  if (!value) throw new Error('useAppCommands must be used inside <WorkspaceProvider>')
  return value
}
