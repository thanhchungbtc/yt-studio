import { PanelRight } from 'lucide-react'

import { PaneHeader } from '../ui/pane-header'
import { Inspector } from './inspector'

export function SecondarySidebar() {
  return (
    <div className="flex h-full flex-col">
      <PaneHeader title="Inspector" icon={PanelRight} command="workbench.toggleSecondarySidebar" />
      <Inspector />
    </div>
  )
}
