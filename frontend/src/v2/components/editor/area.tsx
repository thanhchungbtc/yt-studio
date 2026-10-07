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

import { useWorkbench } from '../../store/workbench'
import { newVideo } from '../new-video'
import { IconButton } from '@/kit/ui/Button'
import { ChannelEditor } from './channel'
import { useDock, type DocPanelParams } from './dock'
import { NewEditor } from './new'
import { Placeholder } from './placeholder'
import { KeybindingsPanel } from '../keybindings'
import { SettingsPanel } from '../settings'
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
  settings: SettingsPanel as FunctionComponent<IDockviewPanelProps>,
  keybindings: KeybindingsPanel as FunctionComponent<IDockviewPanelProps>,
}

const tabComponent = EditorTab as FunctionComponent<IDockviewPanelHeaderProps>

function Watermark() {
  return (
    <div className="glass-card h-full overflow-clip rounded-[var(--card-radius)] bg-content">
      <Placeholder
        icon={Clapperboard}
        title="Ready when you are"
        detail="Pick something from the library to open it here, or start a new video."
        command="video.new"
      />
    </div>
  )
}

function HeaderActions() {
  return (
    <div className="flex h-full items-center pr-1">
      <div className="glass-pill">
        <IconButton
          icon={Plus}
          label="New Video"
          command="video.new"
          size="xs"
          onClick={() => newVideo()}
        />
      </div>
    </div>
  )
}

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
    group.element.toggleAttribute(
      'data-titlebar',
      along.some((entry) => entry.group === group),
    )
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

  // The tab strip's empty stretch drags the window, so cancel dockview's group drag there.
  useEffect(() => {
    const element = container.current
    if (!element) return
    const onDragStart = (event: DragEvent) => {
      if (event.target instanceof Element && event.target.closest('.dv-void-container')) {
        event.preventDefault()
      }
    }
    element.addEventListener('dragstart', onDragStart, true)
    return () => element.removeEventListener('dragstart', onDragStart, true)
  }, [])

  // Dockview can keep a tab's layer at an intermediate size when a resize lands mid-switch.
  useEffect(() => {
    const element = container.current
    if (!element) return
    let frame = 0
    const relayout = new ResizeObserver(() => {
      cancelAnimationFrame(frame)
      frame = requestAnimationFrame(() =>
        useDock.getState().api?.layout(element.clientWidth, element.clientHeight, true),
      )
    })
    relayout.observe(element)
    return () => {
      cancelAnimationFrame(frame)
      relayout.disconnect()
    }
  }, [])

  // Fade a tab strip's clipped end so a cut-off tab reads as "more", not broken.
  useEffect(() => {
    const element = container.current
    if (!element) return
    const mark = (strip: HTMLElement) => {
      const start = strip.scrollLeft > 1
      const end = strip.scrollLeft + strip.clientWidth < strip.scrollWidth - 1
      if (strip.hasAttribute('data-fade-start') !== start)
        strip.toggleAttribute('data-fade-start', start)
      if (strip.hasAttribute('data-fade-end') !== end) strip.toggleAttribute('data-fade-end', end)
    }
    let frame = 0
    const markAll = () => {
      cancelAnimationFrame(frame)
      frame = requestAnimationFrame(() =>
        element.querySelectorAll<HTMLElement>('.dv-tabs-container').forEach(mark),
      )
    }
    const onScroll = (event: Event) => {
      if (
        event.target instanceof HTMLElement &&
        event.target.classList.contains('dv-tabs-container')
      ) {
        mark(event.target)
      }
    }
    const resize = new ResizeObserver(markAll)
    resize.observe(element)
    const touchesTabs = (record: MutationRecord) => {
      const target = record.target instanceof Element ? record.target : record.target.parentElement
      if (target?.closest('.dv-tabs-and-actions-container')) return true
      return [...record.addedNodes].some(
        (node) =>
          node instanceof Element &&
          (node.matches('.dv-tabs-container') || node.querySelector('.dv-tabs-container') !== null),
      )
    }
    const mutation = new MutationObserver((records) => {
      if (records.some(touchesTabs)) markAll()
    })
    mutation.observe(element, { childList: true, subtree: true, characterData: true })
    element.addEventListener('scroll', onScroll, { capture: true, passive: true })
    markAll()
    return () => {
      cancelAnimationFrame(frame)
      resize.disconnect()
      mutation.disconnect()
      element.removeEventListener('scroll', onScroll, { capture: true })
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
        // The default detaches hidden tabs; re-attaching relays out the document.
        defaultRenderer="always"
        disableFloatingGroups
        className="h-full w-full"
      />
    </div>
  )
}
