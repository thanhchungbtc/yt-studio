import {
  DockviewReact,
  type DockviewApi,
  type DockviewReadyEvent,
  type DockviewTheme,
  type IDockviewPanelHeaderProps,
  type IDockviewPanelProps,
  type SerializedDockview,
} from 'dockview-react'
import { Clapperboard, Plus } from 'lucide-react'
import { useCallback, useEffect, useRef, type FunctionComponent } from 'react'

import { beginWindowDrag } from '../../core/desktop'
import { useWorkbench } from '../../store/workbench'
import { newVideo } from '../new-video'
import { HeaderButton } from '../ui/header-button'
import { ChannelEditor } from './channel'
import { useDock, type DocPanelParams } from './dock'
import { NewEditor } from './new'
import { Placeholder } from './placeholder'
import { EditorTab } from './tab'
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
const LAYOUT_KEY = 'yts.v2.layout.4'

/**
 * The theme is a class name plus the handful of behaviours that are not CSS.
 * Every group is its own floating card, so the gap between two of them is the
 * window's gap — the same eight pixels that separate every card in the window.
 */
const glassTheme: DockviewTheme = {
  name: 'glass',
  className: 'dockview-theme-glass',
  gap: 8,
  dndPanelOverlay: 'group',
  dndTabIndicator: 'line',
}

const components: Record<string, FunctionComponent<IDockviewPanelProps>> = {
  video: VideoEditor as FunctionComponent<IDockviewPanelProps>,
  channel: ChannelEditor as FunctionComponent<IDockviewPanelProps>,
  new: NewEditor as FunctionComponent<IDockviewPanelProps>,
}

const tabComponent = EditorTab as FunctionComponent<IDockviewPanelHeaderProps>

/** What fills the area when nothing is open: an empty card, ready. */
function Watermark() {
  return (
    <div className="glass-card h-full overflow-hidden rounded-[var(--card-radius)] bg-content">
      <Placeholder
        icon={Clapperboard}
        title="Ready when you are"
        detail="Pick something from the library to open it here, or start a new video."
        shortcut="⌘N"
      />
    </div>
  )
}

/** The trailing edge of every tab strip: one glass pill of actions. */
function HeaderActions() {
  return (
    <div className="flex h-full items-center pr-1">
      <div className="glass-pill">
        <HeaderButton
          icon={Plus}
          label="New Video"
          shortcut="⌘N"
          onClick={() => newVideo()}
          className="size-6"
        />
      </div>
    </div>
  )
}

/**
 * Groups along the top edge double as the window's titlebar, and the top-left
 * one makes room for the traffic lights when the library is hidden — there is
 * nothing else up there for them to sit in.
 */
function markTitlebarGroups(api: DockviewApi) {
  const rects = api.groups.map((group) => ({ group, rect: group.element.getBoundingClientRect() }))
  if (rects.length === 0) return
  const top = Math.min(...rects.map((entry) => entry.rect.top))
  const along = rects.filter((entry) => Math.abs(entry.rect.top - top) < 2)
  const leftmost = along.reduce<(typeof along)[number] | undefined>(
    (best, entry) => (!best || entry.rect.left < best.rect.left ? entry : best),
    undefined,
  )
  const inset = !useWorkbench.getState().primaryVisible
  for (const { group } of rects) {
    group.element.toggleAttribute('data-titlebar', along.some((entry) => entry.group === group))
    group.element.toggleAttribute('data-traffic-lights', inset && leftmost?.group === group)
  }
}

export function EditorArea() {
  const api = useDock((s) => s.api)
  const setApi = useDock((s) => s.setApi)
  const setActiveDoc = useDock((s) => s.setActiveDoc)
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

  // Re-marked after anything that can move a group's top-left corner: a split,
  // a resize of the window, the library coming or going.
  useEffect(() => {
    if (!api) return
    let frame = 0
    const mark = () => {
      cancelAnimationFrame(frame)
      frame = requestAnimationFrame(() => markTitlebarGroups(api))
    }
    mark()
    const relaid = api.onDidLayoutChange(mark)
    const unsubscribe = useWorkbench.subscribe((state, previous) => {
      if (state.primaryVisible !== previous.primaryVisible) mark()
    })
    window.addEventListener('resize', mark)
    return () => {
      cancelAnimationFrame(frame)
      relaid.dispose()
      unsubscribe()
      window.removeEventListener('resize', mark)
    }
  }, [api])

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

  // The inspector sits outside the dock and follows the front document, so the
  // front document has to be published somewhere it can subscribe to.
  //
  // Synced immediately as well as on change: a restored layout activates its
  // panel inside `fromJSON`, which has already happened by the time this runs.
  // And to the layout event as well as the activation one, because closing the
  // last tab leaves no panel to announce itself.
  useEffect(() => {
    if (!api) {
      setActiveDoc(null)
      return
    }
    const sync = () => {
      const params = api.activePanel?.params as DocPanelParams | undefined
      setActiveDoc(params?.doc ?? null)
    }
    sync()
    const activated = api.onDidActivePanelChange(sync)
    const relaid = api.onDidLayoutChange(sync)
    return () => {
      activated.dispose()
      relaid.dispose()
    }
  }, [api, setActiveDoc])

  // The strip is the titlebar, so its empty stretch has to move the window —
  // and that stretch is dockview's own element, mounted and unmounted per
  // group. Delegating from the container reaches every one of them, now and
  // after the next split, without reaching into dockview's internals.
  //
  // Dockview claims that same stretch: `.dv-void-container` is a `draggable`
  // drag source for the whole group. So one press would start an AppKit window
  // drag and an HTML5 group drag at once — the window slides while the page
  // paints a drag ghost and arms drop overlays over every pane, and those
  // outlive the gesture, because AppKit takes the event stream and the
  // `dragend` that would have cleared them never arrives. The two cannot share
  // a press, and this strip is the titlebar, so the window wins.
  //
  // Cancelling in the capture phase is dockview's own way out rather than a
  // trick against it: its drag source checks `defaultPrevented` before it does
  // anything, and returns without registering transfer data or building a
  // ghost, so there is nothing left half-started. Only the group handle goes.
  // Tabs carry their own drag source, so reordering, tearing one into a split
  // and dropping a group onto another all still work.
  useEffect(() => {
    const element = container.current
    if (!element) return

    const voidSpace = (event: Event) =>
      event.target instanceof Element && event.target.closest('.dv-void-container')

    const onPointerDown = (event: PointerEvent) => {
      if (event.button !== 0 || !voidSpace(event)) return
      beginWindowDrag()
    }
    const onDragStart = (event: DragEvent) => {
      if (voidSpace(event)) event.preventDefault()
    }

    element.addEventListener('pointerdown', onPointerDown)
    element.addEventListener('dragstart', onDragStart, true)
    return () => {
      element.removeEventListener('pointerdown', onPointerDown)
      element.removeEventListener('dragstart', onDragStart, true)
    }
  }, [])

  return (
    <div ref={container} className="h-full w-full">
      <DockviewReact
        components={components}
        defaultTabComponent={tabComponent}
        watermarkComponent={Watermark}
        rightHeaderActionsComponent={HeaderActions}
        onReady={onReady}
        theme={glassTheme}
        noPanelsOverlay="watermark"
        singleTabMode="default"
        // Every open document stays laid out, and switching tabs flips its
        // visibility. The default detaches a hidden tab's content and attaches
        // it again on the way back, which costs a full layout of the document
        // — a pipeline of three hundred tasks — inside the click, before the
        // tab even looks pressed. Kept, the switch is a repaint.
        defaultRenderer="always"
        disableFloatingGroups
        className="h-full w-full"
      />
    </div>
  )
}
