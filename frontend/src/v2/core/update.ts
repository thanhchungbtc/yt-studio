import { Events } from '@wailsio/runtime'
import { create } from 'zustand'

import * as UpdateService from '@bindings/services/updateservice'

/** Where updating stands (services.UpdateState). */
export interface UpdateState {
  state: 'disabled' | 'idle' | 'preparing' | 'ready' | 'failed'
  current: string
  version?: string
  notes?: string[] | null
  error?: string
}

/** Where updating to a `make release` version stands. */
export const useUpdate = create<UpdateState>(() => ({ state: 'idle', current: '' }))

/** Loads the update state and follows it. */
export function connectUpdates(): () => void {
  let live = false
  void UpdateService.State()
    .then((s) => {
      if (!live) useUpdate.setState(s as UpdateState, true)
    })
    .catch(() => undefined)
  return Events.On('update:state', (event) => {
    live = true
    useUpdate.setState(event.data as UpdateState, true)
  })
}

/** Restarts into the ready version. */
export async function restartToUpdate(): Promise<void> {
  if (useUpdate.getState().state === 'ready') await UpdateService.Restart()
}

/** Checks for a new version now. */
export async function checkForUpdates(): Promise<UpdateState> {
  const s = (await UpdateService.Check()) as UpdateState
  useUpdate.setState(s, true)
  return s
}

/** A commit subject as a release note: "feat(ui): add x" is "Add x". */
export function noteText(subject: string): string {
  const s =
    subject
      .replace(
        /^(feat|fix|perf|refactor|chore|docs|style|test|build|ci|revert)(\([^)]*\))?!?:\s*/,
        '',
      )
      .trim() || subject
  return s.charAt(0).toUpperCase() + s.slice(1)
}
