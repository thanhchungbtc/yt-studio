import { useQuery } from '@tanstack/react-query'
import { useCallback, useRef, useState } from 'react'
import { Panel, PanelGroup, type ImperativePanelHandle } from 'react-resizable-panels'

import { AssetViewerProvider } from './asset-viewer'
import { ActivityBar, StatusBar, TitleBar } from './chrome'
import { CreateVideo } from './create-video'
import { EditorArea } from './editor/editor-area'
import { Explorer } from './explorer'
import { CommandRegistryContext, useCommandRegistry, useCommands, useKeybindings } from './lib/keys'
import { Handle, usePanelSync, usePixelConstraints } from './lib/panels'
import { useActiveTab, useFocusedGroup, useWorkbenchStore } from './lib/store'
import { Palette, type PaletteMode } from './palette'
import { BottomPanel } from './panel/bottom-panel'
import { RunPanel } from './run-panel'
import { TooltipProvider } from './ui/primitives'
import { api, qk } from '@/core/api'
import { useEventStream } from '@/core/events'

/**
 * The workbench.
 *
 *   ┌────────────────────────────────────────────────────────┐  title bar   32
 *   ├──┬───────────┬─────────────────────────┬───────────────┤
 *   │  │           │ tabs ─────────────────  │               │              35
 *   │▣ │ explorer  │ breadcrumb ───────────  │  run panel    │              26
 *   │  │           │                         │  (contextual) │
 *   │  │           │ document                │               │
 *   │⚙ │           ├─────────────────────────┤               │
 *   │  │           │ console │ output        │               │  ⌘J
 *   ├──┴───────────┴─────────────────────────┴───────────────┤
 *   │ status bar                                             │              24
 *   └────────────────────────────────────────────────────────┘
 *
 * Two structural ideas, both borrowed:
 *
 * The rail swaps the *sidebar*, not the main area, so the explorer is a constant
 * and everything else is a document opened into the middle.
 *
 * And a single click opens a *preview* — one italic tab that the next click
 * reuses. That is what makes tabs affordable: browsing costs nothing, and only
 * what you deliberately keep accumulates.
 */
export function Workbench() {
  const registry = useCommandRegistry()
  // Bound here rather than in the shell: this is the component that owns the
  // registry, so it is the one that can bind every chord in it exactly once.
  useKeybindings(registry)

  return (
    <CommandRegistryContext.Provider value={registry}>
      <TooltipProvider>
        <Shell />
      </TooltipProvider>
    </CommandRegistryContext.Provider>
  )
}

function Shell() {
  const connection = useEventStream()

  const explorerVisible = useWorkbenchStore((s) => s.explorerVisible)
  const toggleExplorer = useWorkbenchStore((s) => s.toggleExplorer)
  const asideVisible = useWorkbenchStore((s) => s.asideVisible)
  const toggleAside = useWorkbenchStore((s) => s.toggleAside)
  const bottomVisible = useWorkbenchStore((s) => s.bottomVisible)
  const toggleBottom = useWorkbenchStore((s) => s.toggleBottom)
  const openDoc = useWorkbenchStore((s) => s.open)
  const split = useWorkbenchStore((s) => s.split)
  const close = useWorkbenchStore((s) => s.close)
  const activate = useWorkbenchStore((s) => s.activate)

  const group = useFocusedGroup()
  const tab = useActiveTab()
  const videoRef = tab?.doc.kind === 'video' ? tab.doc.ref : undefined
  const asideAvailable = videoRef !== undefined

  const [palette, setPalette] = useState<PaletteMode | null>(null)
  const [creating, setCreating] = useState<{ channel?: string } | null>(null)

  const shellRef = useRef<HTMLDivElement>(null)
  const explorerPanel = useRef<ImperativePanelHandle>(null)
  const asidePanel = useRef<ImperativePanelHandle>(null)
  const bottomPanel = useRef<ImperativePanelHandle>(null)
  const pct = usePixelConstraints(shellRef)

  // A panel group answers no questions about itself until it has laid out, so
  // the visibility sync waits for each group to say it has.
  const [shellReady, setShellReady] = useState(false)
  const [centerReady, setCenterReady] = useState(false)

  usePanelSync(explorerPanel, explorerVisible, shellReady)
  usePanelSync(asidePanel, asideVisible && asideAvailable, shellReady)
  usePanelSync(bottomPanel, bottomVisible, centerReady)

  const newVideo = useCallback(
    (channel?: string) => setCreating({ ...(channel ? { channel } : {}) }),
    [],
  )

  // Fetched here as well as in the document so the asset viewer — which spans
  // the document *and* the run panel — knows which video it is looking at.
  // React Query dedupes it against the document's own copy.
  const video = useQuery({
    queryKey: qk.video(videoRef ?? ''),
    queryFn: () => api.getVideo(videoRef ?? ''),
    enabled: Boolean(videoRef),
  })

  /** Walks the focused group's tabs, wrapping at both ends. */
  const stepTab = (delta: number) => {
    if (!group || group.tabs.length === 0) return
    const current = group.tabs.findIndex((t) => t.id === group.activeId)
    const next = group.tabs[(current + delta + group.tabs.length) % group.tabs.length]
    if (next) activate(group.id, next.id)
  }

  useCommands([
    {
      id: 'go.quickOpen',
      label: 'Go to a video or channel',
      category: 'Go',
      keys: '$mod+KeyP',
      // Deliberately live while typing: reaching the palette from inside a field
      // is the point of having it.
      whileTyping: true,
      run: () => setPalette((prev) => (prev === 'go' ? null : 'go')),
    },
    {
      id: 'go.commands',
      label: 'Run a command',
      category: 'Go',
      keys: '$mod+Shift+KeyP',
      whileTyping: true,
      run: () => setPalette((prev) => (prev === 'command' ? null : 'command')),
    },
    {
      id: 'video.new',
      label: 'New video',
      category: 'File',
      keys: '$mod+KeyN',
      run: () => newVideo(),
    },
    {
      id: 'file.settings',
      label: 'Settings',
      category: 'File',
      keys: '$mod+Comma',
      run: () => openDoc({ kind: 'settings' }, { preview: false }),
    },
    {
      id: 'file.close',
      label: 'Close the tab',
      category: 'File',
      keys: '$mod+KeyW',
      run: () => {
        if (group && group.activeId) close(group.id, group.activeId)
      },
    },
    {
      id: 'view.primary',
      label: 'Toggle the primary sidebar',
      category: 'View',
      keys: '$mod+Digit1',
      run: toggleExplorer,
    },
    {
      id: 'view.bottom',
      label: 'Toggle the bottom panel',
      category: 'View',
      keys: '$mod+Digit2',
      run: toggleBottom,
    },
    {
      id: 'view.secondary',
      label: 'Toggle the secondary sidebar',
      category: 'View',
      keys: '$mod+Digit3',
      disabled: !asideAvailable,
      run: toggleAside,
    },
    {
      id: 'editor.nextTab',
      label: 'Next tab',
      category: 'Go',
      keys: '$mod+Alt+ArrowRight',
      run: () => stepTab(1),
    },
    {
      id: 'editor.previousTab',
      label: 'Previous tab',
      category: 'Go',
      keys: '$mod+Alt+ArrowLeft',
      run: () => stepTab(-1),
    },
    {
      id: 'view.split',
      label: 'Split the editor',
      category: 'View',
      keys: '$mod+Backslash',
      run: split,
    },
  ])

  return (
    <AssetViewerProvider videoRef={videoRef} videoId={video.data?.id}>
      <div className="flex h-full w-full flex-col overflow-hidden bg-app">
        <TitleBar onOpenPalette={() => setPalette('go')} />

        <div className="flex min-h-0 flex-1">
          <ActivityBar />

          {/* The ref goes here rather than around the activity bar: percentages
              are of the panel group, and measuring 48 pixels of rail into the
              total would inflate every minimum by that much. */}
          <div ref={shellRef} className="flex min-w-0 flex-1">
            <PanelGroup
              direction="horizontal"
              autoSaveId="yt-studio.wb.shell"
              onLayout={() => setShellReady(true)}
              className="min-w-0"
            >
              <Panel
                id="explorer"
                order={1}
                ref={explorerPanel}
                collapsible
                collapsedSize={0}
                defaultSize={pct(288)}
                minSize={pct(200, 20)}
                maxSize={pct(480, 40)}
                onCollapse={() => useWorkbenchStore.setState({ explorerVisible: false })}
                onExpand={() => useWorkbenchStore.setState({ explorerVisible: true })}
                className="min-w-0 border-r border-[hsl(var(--border))]"
              >
                <Explorer onNewVideo={newVideo} />
              </Panel>
              <Handle />

              <Panel id="center" order={2} minSize={pct(360, 30)} className="min-w-0">
                <PanelGroup
                  direction="vertical"
                  autoSaveId="yt-studio.wb.center"
                  onLayout={() => setCenterReady(true)}
                >
                  <Panel id="editors" order={1} minSize={20} className="min-h-0">
                    <EditorArea onNewVideo={newVideo} onOpenPalette={() => setPalette('go')} />
                  </Panel>
                  <Handle direction="vertical" />
                  <Panel
                    id="bottom"
                    order={2}
                    ref={bottomPanel}
                    collapsible
                    collapsedSize={0}
                    defaultSize={32}
                    minSize={12}
                    onCollapse={() => useWorkbenchStore.setState({ bottomVisible: false })}
                    onExpand={() => useWorkbenchStore.setState({ bottomVisible: true })}
                    className="min-h-0"
                  >
                    <BottomPanel />
                  </Panel>
                </PanelGroup>
              </Panel>

              <Handle />
              <Panel
                id="aside"
                order={3}
                ref={asidePanel}
                collapsible
                collapsedSize={0}
                defaultSize={pct(304)}
                minSize={pct(240, 24)}
                maxSize={pct(560, 45)}
                onCollapse={() => useWorkbenchStore.setState({ asideVisible: false })}
                onExpand={() => useWorkbenchStore.setState({ asideVisible: true })}
                className="min-w-0 border-l border-[hsl(var(--border))]"
              >
                {videoRef ? (
                  <RunPanel key={videoRef} videoRef={videoRef} onClose={toggleAside} />
                ) : null}
              </Panel>
            </PanelGroup>
          </div>
        </div>

        <StatusBar connection={connection} />
      </div>

      <Palette
        open={palette !== null}
        mode={palette ?? 'go'}
        onOpenChange={(next) => {
          if (!next) setPalette(null)
        }}
      />
      <CreateVideo
        open={creating !== null}
        onOpenChange={(next) => {
          if (!next) setCreating(null)
        }}
        defaultChannel={creating?.channel}
      />
    </AssetViewerProvider>
  )
}
