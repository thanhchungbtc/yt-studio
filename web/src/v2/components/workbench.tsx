import 'dockview-react/dist/styles/dockview.css'
import '../styles.css'

import { Panel, PanelGroup, PanelResizeHandle } from 'react-resizable-panels'

import { useEventStream } from '../core/events'
import { useKeybindings } from '../core/keys'
import { useWorkbench } from '../store/workbench'
import { EditorArea } from './editor/area'
import { NewVideoDialog } from './new-video'
import { SettingsDialog } from './settings'
import { BottomPanel } from './panel/bottom'
import { PrimarySidebar } from './sidebar/primary'
import { SecondarySidebar } from './sidebar/secondary'
import { StatusBar } from './status-bar'
import { DragRegion } from './ui/drag-region'

/**
 * Workbench V2.
 *
 *   ┌────────────┬───────────────────────────────┬───────────┐
 *   │ ● ● ●      │ tabs                          │           │  38
 *   │ LIBRARY  ✎ ├───────────────────────────────┤ inspector │  30
 *   │ ┌────────┐ │ title                         │           │  50
 *   │ │ videos │ │                               │           │
 *   │ └────────┘ │                               │           │
 *   │ CHANNEL  ▾ │ document                      │           │
 *   │   video    │                               │           │
 *   │   video    │                               │           │
 *   ├────────────┴───────────────────────────────┴───────────┤
 *   │ console                                                │  ⌘J
 *   ├────────────────────────────────────────────────────────┤
 *   │ status                                                 │  24
 *   └────────────────────────────────────────────────────────┘
 *
 * Three things are load-bearing.
 *
 * The traffic lights live in the *sidebar*, not over the documents — which is
 * why the sidebar is the one pane that reaches the top of the window, and why
 * hiding it has to hand its top strip to the column beside it rather than
 * simply removing it. macOS has nowhere else to put those three buttons.
 *
 * The bottom panel spans the whole window rather than sitting under the editor
 * alone. It is about the *session*, not about the document that happens to be
 * open, and a panel indented to the width of one column claims otherwise. That
 * is why the vertical split is the outer one here.
 *
 * And the panes are chrome around exactly one document. Everything translucent
 * is a frame; the single opaque surface is the thing being worked on.
 */
export function WorkbenchV2() {
  useKeybindings()
  // Mounted here and nowhere else: one connection for the whole application,
  // and this is the one component guaranteed to outlive every document that
  // depends on it.
  useEventStream()

  const primaryVisible = useWorkbench((s) => s.primaryVisible)
  const secondaryVisible = useWorkbench((s) => s.secondaryVisible)
  const bottomVisible = useWorkbench((s) => s.bottomVisible)

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <PanelGroup direction="vertical" autoSaveId="yts.v2.rows" className="min-h-0 flex-1">
        <Panel id="upper" order={1} minSize={30}>
          <PanelGroup direction="horizontal" autoSaveId="yts.v2.columns">
            {primaryVisible ? (
              <>
                <Panel id="primary" order={1} defaultSize={22} minSize={14} maxSize={40}>
                  <PrimarySidebar />
                </Panel>
                <Sash direction="horizontal" />
              </>
            ) : null}

            <Panel id="center" order={2} minSize={30}>
              <div className="flex h-full flex-col">
                {/* With the sidebar hidden there is nothing else reaching the
                    top of the window, so this column has to make room for the
                    traffic lights instead. */}
                {primaryVisible ? null : (
                  <DragRegion className="surface-chrome hairline-b h-[38px] shrink-0" />
                )}
                <div className="min-h-0 flex-1">
                  <EditorArea />
                </div>
              </div>
            </Panel>

            {secondaryVisible ? (
              <>
                <Sash direction="horizontal" />
                <Panel id="secondary" order={3} defaultSize={22} minSize={14} maxSize={40}>
                  <SecondarySidebar />
                </Panel>
              </>
            ) : null}
          </PanelGroup>
        </Panel>

        {bottomVisible ? (
          <>
            <Sash direction="vertical" />
            <Panel id="bottom" order={2} defaultSize={28} minSize={10} maxSize={70}>
              <BottomPanel />
            </Panel>
          </>
        ) : null}
      </PanelGroup>

      <StatusBar />
      <NewVideoDialog />
      <SettingsDialog />
    </div>
  )
}

/**
 * The divider between two panes: one device pixel to look at, eight to grab.
 *
 * The sash *is* the seam. Neither pane draws an edge of its own, because two
 * panes drawing their own edges on either side of a handle that occupies space
 * is three things where there should be one — and that is exactly what a
 * doubled, muddy divider is made of.
 *
 * It does not light up while being dragged. A divider that highlights under the
 * cursor is a web idiom; on macOS the feedback is the panes moving, which is
 * the feedback that was asked for.
 */
function Sash({ direction }: { direction: 'horizontal' | 'vertical' }) {
  const horizontal = direction === 'horizontal'
  return (
    <PanelResizeHandle
      className={
        horizontal ? 'seam-v relative z-20 outline-none' : 'seam-h relative z-20 outline-none'
      }
    >
      <div
        className={
          horizontal
            ? 'absolute inset-y-0 -right-1 -left-1 cursor-col-resize'
            : 'absolute inset-x-0 -top-1 -bottom-1 cursor-row-resize'
        }
      />
    </PanelResizeHandle>
  )
}
