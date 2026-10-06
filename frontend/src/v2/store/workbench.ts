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
  /**
   * The selected rows, as video `ref`s or a channel `slug`.
   *
   * A list rather than one value because the source list selects the way Finder
   * does — ⌘ to add, ⇧ to extend — and everything downstream of it either wants
   * the whole set or the first of it.
   */
  selected: string[]

  togglePrimary: () => void
  toggleSecondary: () => void
  toggleBottom: () => void
  setScope: (scope: SidebarScope) => void
  select: (ids: string[]) => void
}

export const useWorkbench = create<WorkbenchState>()(
  persist(
    (set) => ({
      primaryVisible: true,
      secondaryVisible: false,
      bottomVisible: false,
      scope: 'videos',
      selected: [],

      togglePrimary: () => set((s) => ({ primaryVisible: !s.primaryVisible })),
      toggleSecondary: () => set((s) => ({ secondaryVisible: !s.secondaryVisible })),
      toggleBottom: () => set((s) => ({ bottomVisible: !s.bottomVisible })),
      setScope: (scope) => set({ scope }),
      select: (selected) => set({ selected }),
    }),
    {
      name: 'yts.v2.workbench',
      /*
        Bumped when `selected` went from one value to a list.

        Without this a browser holding the old shape rehydrates a string into a
        field everything now calls `.includes` and `.filter` on, and the sidebar
        throws on the first render after the update. The migration is one line
        and the alternative is a crash only the people who used the previous
        build ever see.
      */
      version: 1,
      migrate: (persisted, version) => {
        const state = persisted as Partial<WorkbenchState> & { selected?: unknown }
        if (version >= 1) return state as WorkbenchState
        const previous = state.selected
        return {
          ...state,
          selected: typeof previous === 'string' ? [previous] : [],
        } as WorkbenchState
      },
    },
  ),
)
