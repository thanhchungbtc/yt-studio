import { useEffect } from 'react'
import { tinykeys } from 'tinykeys'

import { closeActive, openDoc } from '../components/editor/dock'
import { useWorkbench } from '../store/workbench'

/**
 * The keyboard, which is the only way to move the panes.
 *
 * There are no toggle buttons in the chrome. Every one of them was a control
 * standing in for a keystroke, taking permanent space to save a person who
 * already knows the keystroke nothing.
 *
 * The panes are numbered by where they are — ⌘1 left, ⌘2 below, ⌘3 right —
 * rather than named by what they hold. A number is a position, and a position
 * is the thing you are actually reaching for; ⌘B would have to be remembered as
 * standing for something.
 *
 * `$mod` is tinykeys' portable Command-or-Control. Only macOS is supported
 * today, but writing Meta by hand is a decision this file has no reason to
 * make.
 */
export function useKeybindings(): void {
  useEffect(() => {
    const store = useWorkbench.getState()
    return tinykeys(window, {
      '$mod+Digit1': (event) => {
        event.preventDefault()
        store.togglePrimary()
      },
      '$mod+Digit2': (event) => {
        event.preventDefault()
        store.toggleBottom()
      },
      '$mod+Digit3': (event) => {
        event.preventDefault()
        store.toggleSecondary()
      },
      '$mod+KeyN': (event) => {
        event.preventDefault()
        openDoc({ kind: 'new', of: 'video' }, 'New Video')
      },
      '$mod+Shift+KeyN': (event) => {
        event.preventDefault()
        openDoc({ kind: 'new', of: 'channel' }, 'New Channel')
      },
      '$mod+KeyW': (event) => {
        event.preventDefault()
        closeActive()
      },
    })
  }, [])
}
