import { useQueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'
import { tinykeys } from 'tinykeys'

import { duplicateVideos } from '../components/duplicate-video'
import { closeActive, closeOthers, openDoc } from '../components/editor/dock'
import { newVideo } from '../components/new-video'
import { openSettings } from '../components/settings'
import { anyModalOpen } from '../components/ui/dialog'
import { useWorkbench } from '../store/workbench'
import { qk } from './api'
import type { Video } from './types'

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
  const client = useQueryClient()
  useEffect(() => {
    const store = useWorkbench.getState()

    /**
     * ⌘D on whatever the source list has selected.
     *
     * The selection holds video refs and channel slugs in one list, so the
     * videos are found by intersecting it with the library rather than by
     * asking which scope is showing — which makes the channels scope a no-op
     * for free, and keeps this from duplicating a channel by coincidence.
     *
     * Read from the cache, not subscribed to: a keystroke wants the list as it
     * stands when it fires, and a subscription here would re-run the whole
     * keymap every time a task moved.
     */
    const duplicateSelection = () => {
      const wanted = new Set(useWorkbench.getState().selected)
      const library = client.getQueryData<Video[]>(qk.videos) ?? []
      duplicateVideos(library.filter((video) => wanted.has(video.ref)))
    }

    return tinykeys(window, {
      '$mod+Digit1': windowOnly(store.togglePrimary),
      '$mod+Digit2': windowOnly(store.toggleBottom),
      '$mod+Digit3': windowOnly(store.toggleSecondary),

      '$mod+KeyW': windowOnly(closeActive),
      '$mod+Shift+KeyW': windowOnly(closeOthers),

      '$mod+KeyD': windowOnly(duplicateSelection),

      '$mod+KeyN': windowOnly(() => newVideo()),
      '$mod+Shift+KeyN': windowOnly(() => openDoc({ kind: 'new', of: 'channel' }, 'New Channel')),

      '$mod+Comma': windowOnly(openSettings),
    })
  }, [client])
}
