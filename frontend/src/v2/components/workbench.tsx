import 'dockview-react/dist/styles/dockview.css'
import '../styles.css'

import { useEffect, type ReactNode } from 'react'
import { Panel, PanelGroup, PanelResizeHandle } from 'react-resizable-panels'

import { useEventStream } from '../core/events'
import { connectUpdates } from '../core/update'
import { useKeybindings } from '../core/keys'
import { cn } from '../core/utils'
import { useWorkbench } from '../store/workbench'
import { EditorArea } from './editor/area'
import { DuplicateVideoHost } from './duplicate-video'
import { EditVideoDialog } from './edit-video'
import { NewFromBlueprintDialog } from './new-from-blueprint'
import { NewVideoDialog } from './new-video'
import { SettingsDialog } from './settings'
import { BottomPanel } from './panel/bottom'
import { PrimarySidebar } from './sidebar/primary'
import { SecondarySidebar } from './sidebar/secondary'
import { StatusBar } from './status-bar'
import { TooltipProvider } from './ui/tooltip'

export function WorkbenchV2() {
  useKeybindings()
  // Mounted here and nowhere else: one connection for the whole application,
  // and this is the one component guaranteed to outlive every document that
  // depends on it.
  useEventStream()
  useEffect(connectUpdates, [])

  const primaryVisible = useWorkbench((s) => s.primaryVisible)
  const secondaryVisible = useWorkbench((s) => s.secondaryVisible)
  const bottomVisible = useWorkbench((s) => s.bottomVisible)

  return (
    <TooltipProvider>
      <div className="flex h-full flex-col overflow-hidden">
        <div className="min-h-0 flex-1 px-[var(--gap)] pt-[var(--gap)]">
          <PanelGroup direction="vertical" autoSaveId="yts.v2.rows" className="h-full">
            <Panel id="upper" order={1} minSize={30}>
              <PanelGroup direction="horizontal" autoSaveId="yts.v2.columns">
                {primaryVisible ? (
                  <>
                    <Panel id="primary" order={1} defaultSize={22} minSize={15} maxSize={40}>
                      <Card className="glass-sidebar">
                        <PrimarySidebar />
                      </Card>
                    </Panel>
                    <Gap />
                  </>
                ) : null}

                <Panel id="center" order={2} minSize={30}>
                  <EditorArea />
                </Panel>

                {secondaryVisible ? (
                  <>
                    <Gap />
                    <Panel id="secondary" order={3} defaultSize={22} minSize={18} maxSize={40}>
                      <Card className="glass-sidebar">
                        <SecondarySidebar />
                      </Card>
                    </Panel>
                  </>
                ) : null}
              </PanelGroup>
            </Panel>

            {bottomVisible ? (
              <>
                <Gap vertical />
                <Panel id="bottom" order={2} defaultSize={28} minSize={10} maxSize={70}>
                  <Card className="bg-content">
                    <BottomPanel />
                  </Card>
                </Panel>
              </>
            ) : null}
          </PanelGroup>
        </div>

        <StatusBar />
        <NewVideoDialog />
        <NewFromBlueprintDialog />
        <DuplicateVideoHost />
        <EditVideoDialog />
        <SettingsDialog />
      </div>
    </TooltipProvider>
  )
}

function Card({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <div
      className={cn(
        'glass-card view-enter h-full min-h-0 overflow-hidden rounded-[var(--card-radius)]',
        className,
      )}
    >
      {children}
    </div>
  )
}

function Gap({ vertical = false }: { vertical?: boolean }) {
  return (
    <PanelResizeHandle
      className={cn(
        'group/sep relative flex items-center justify-center outline-none',
        vertical ? 'h-[var(--gap)] cursor-row-resize' : 'w-[var(--gap)] cursor-col-resize',
      )}
    >
      <span
        className={cn(
          'rounded-full bg-accent/70 opacity-0 transition-opacity duration-150',
          'group-hover/sep:opacity-100 group-data-[resize-handle-state=drag]/sep:opacity-100',
          'group-focus-visible/sep:opacity-100',
          vertical ? 'h-[3px] w-12' : 'h-12 w-[3px]',
        )}
      />
    </PanelResizeHandle>
  )
}
