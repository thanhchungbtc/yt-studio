import { useEffect } from 'react'
import { tinykeys } from 'tinykeys'

import { closeActive, closeOthers, openDoc } from '../components/editor/dock'
import { newVideo } from '../components/new-video'
import { anyModalOpen } from '../components/ui/dialog'
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

/**
 * What a shortcut must not do while a dialog is up.
 *
 * A dialog is modal: the window behind it is not accepting instructions, so a
 * ⌘W that closed a tab behind it, or a ⌘1 that moved a pane nobody can see,
 * would be the application answering a question it was not asked. Escape and
 * Return belong to the dialog and are never bound here.
 */
type Handler = (event: KeyboardEvent) => void

function windowOnly(run: () => void): Handler {
  return (event) => {
    if (anyModalOpen()) return
    event.preventDefault()
    run()
  }
}

export function useKeybindings(): void {
  useEffect(() => {
    const store = useWorkbench.getState()
    return tinykeys(window, {
      '$mod+Digit1': windowOnly(store.togglePrimary),
      '$mod+Digit2': windowOnly(store.toggleBottom),
      '$mod+Digit3': windowOnly(store.toggleSecondary),

      '$mod+KeyW': windowOnly(closeActive),
      '$mod+Shift+KeyW': windowOnly(closeOthers),

      '$mod+KeyN': windowOnly(() => newVideo()),
      '$mod+Shift+KeyN': windowOnly(() => openDoc({ kind: 'new', of: 'channel' }, 'New Channel')),
    })
  }, [])
}
