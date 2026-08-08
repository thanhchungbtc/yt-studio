import { Workbench } from '@/components/workbench/workbench'

/**
 * `/workbench` — the UI experiment, mounted beside the shipping shell rather
 * than over it.
 *
 * It draws its own title bar, rail, panels and status bar, so the root route
 * steps aside for this path instead of nesting one window inside another. Both
 * UIs run against the same API and the same query cache, which is the point:
 * you can switch between them mid-session and compare like for like.
 *
 * Everything it owns lives under `components/workbench`. Nothing outside that
 * directory imports from it, so deleting the experiment is deleting a folder.
 */
export function WorkbenchRoute() {
  return <Workbench />
}
