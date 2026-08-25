import {
  DockviewReact,
  type DockviewReadyEvent,
  type DockviewTheme,
  type IDockviewPanelHeaderProps,
  type IDockviewPanelProps,
  type SerializedDockview,
} from 'dockview-react'
import { Clapperboard } from 'lucide-react'
import { useCallback, useEffect, useRef, type FunctionComponent } from 'react'

import { beginWindowDrag } from '../../core/desktop'
import { ChannelEditor } from './channel'
import { useDock } from './dock'
import { NewEditor } from './new'
import { Placeholder } from './placeholder'
import { EditorTab } from './tab'
import { ThumbnailEditor } from './thumbnail'
import { VideoEditor } from './video'

/**
 * The editor area: tabs, splits and everything opened into the middle.
 *
 * Dockview does the hard part — tearing a tab into a split, dragging one group
 * onto another, serialising the result — so this file is a component registry,
 * a theme, and the two lines that make the arrangement survive a reload.
 */

/*
  Versioned, because a restored panel keeps the params it was saved with. When
  those params gain a field the tab draws — the channel token did exactly that —
  an old layout comes back half-formed and looks like a bug rather than like an
  old layout. Bumping the suffix drops it instead, which costs one arrangement
  once and is the only honest answer while the shape is still moving.
*/
const LAYOUT_KEY = 'yts.v2.layout.2'

/**
 * The theme is a class name plus the handful of behaviours that are not CSS.
 * `gap: 0` because macOS butts its panes against each other and separates them
 * with a hairline; a gutter would read as a web layout.
 */
const macOSTheme: DockviewTheme = {
  name: 'macos',
  className: 'dockview-theme-macos',
  gap: 0,
  dndPanelOverlay: 'group',
  dndTabIndicator: 'line',
}

const components: Record<string, FunctionComponent<IDockviewPanelProps>> = {
  video: VideoEditor as FunctionComponent<IDockviewPanelProps>,
  channel: ChannelEditor as FunctionComponent<IDockviewPanelProps>,
  thumbnail: ThumbnailEditor as FunctionComponent<IDockviewPanelProps>,
  new: NewEditor as FunctionComponent<IDockviewPanelProps>,
}

const tabComponent = EditorTab as FunctionComponent<IDockviewPanelHeaderProps>

/** What fills the area when nothing is open. */
function Watermark() {
  return (
    <div className="surface-content h-full">
      <Placeholder
        icon={Clapperboard}
        title="Ready when you are"
        detail="Pick something from the sidebar to open it here."
      />
    </div>
  )
}

export function EditorArea() {
  const api = useDock((s) => s.api)
  const setApi = useDock((s) => s.setApi)
  const container = useRef<HTMLDivElement>(null)

  const onReady = useCallback(
    (event: DockviewReadyEvent) => {
      // A layout written by an older build can name a component this one no
      // longer has, and dockview throws rather than guessing. Dropping it is
      // the right answer: an empty dock is recoverable, a dead one is not.
      const saved = localStorage.getItem(LAYOUT_KEY)
      if (saved) {
        try {
          event.api.fromJSON(JSON.parse(saved) as SerializedDockview)
        } catch {
          localStorage.removeItem(LAYOUT_KEY)
          event.api.clear()
        }
      }
      setApi(event.api)
    },
    [setApi],
  )

  useEffect(() => () => setApi(null), [setApi])

  useEffect(() => {
    if (!api) return
    const subscription = api.onDidLayoutChange(() => {
      try {
        localStorage.setItem(LAYOUT_KEY, JSON.stringify(api.toJSON()))
      } catch {
        /* private browsing, or a quota; the layout simply does not persist */
      }
    })
    return () => subscription.dispose()
  }, [api])

  // The strip is the titlebar, so its empty stretch has to move the window —
  // and that stretch is dockview's own element, mounted and unmounted per
  // group. Delegating from the container reaches every one of them, now and
  // after the next split, without reaching into dockview's internals.
  useEffect(() => {
    const element = container.current
    if (!element) return
    const onPointerDown = (event: PointerEvent) => {
      if (event.button !== 0) return
      const target = event.target
      if (!(target instanceof Element)) return
      if (!target.closest('.dv-void-container')) return
      beginWindowDrag()
    }
    element.addEventListener('pointerdown', onPointerDown)
    return () => element.removeEventListener('pointerdown', onPointerDown)
  }, [])

  return (
    <div ref={container} className="h-full w-full">
      <DockviewReact
        components={components}
        defaultTabComponent={tabComponent}
        watermarkComponent={Watermark}
        onReady={onReady}
        theme={macOSTheme}
        noPanelsOverlay="watermark"
        singleTabMode="default"
        disableFloatingGroups
        className="h-full w-full"
      />
    </div>
  )
}
