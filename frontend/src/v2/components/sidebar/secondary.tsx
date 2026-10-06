import { Inspector } from './inspector'
import { PaneHeader } from '../ui/pane-header'

export function SecondarySidebar() {
  return (
    <div className="flex h-full flex-col">
      <PaneHeader title="Inspector" shortcut="⌘3" />
      <Inspector />
    </div>
  )
}
