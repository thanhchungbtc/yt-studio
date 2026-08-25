import { create } from 'zustand'
import { persist } from 'zustand/middleware'

/**
 * What the workbench remembers between launches.
 *
 * Only the things a person would be annoyed to set twice: which panes are
 * showing, which scope the source list is in, and what was selected in it. Pane
 * *sizes* are not here — `react-resizable-panels` persists those itself under
 * `autoSaveId`, and a second copy would be a second answer.
 *
 * The open documents are not here either. Dockview owns the tab layout, and it
 * serialises the whole grid — splits, order, active tab — in one object; see
 * the editor area.
 */

/** What the primary sidebar is listing. */
export type SidebarScope = 'videos' | 'channels'

interface WorkbenchState {
  primaryVisible: boolean
  secondaryVisible: boolean
  bottomVisible: boolean
  scope: SidebarScope
  /** The selected row, as a video `ref` or a channel `slug`. */
  selected: string | null

  togglePrimary: () => void
  toggleSecondary: () => void
  toggleBottom: () => void
  setScope: (scope: SidebarScope) => void
  select: (id: string | null) => void
}

export const useWorkbench = create<WorkbenchState>()(
  persist(
    (set) => ({
      primaryVisible: true,
      secondaryVisible: false,
      bottomVisible: false,
      scope: 'videos',
      selected: null,

      togglePrimary: () => set((s) => ({ primaryVisible: !s.primaryVisible })),
      toggleSecondary: () => set((s) => ({ secondaryVisible: !s.secondaryVisible })),
      toggleBottom: () => set((s) => ({ bottomVisible: !s.bottomVisible })),
      setScope: (scope) => set({ scope }),
      select: (selected) => set({ selected }),
    }),
    { name: 'yts.v2.workbench' },
  ),
)
