import type { RefObject } from 'react'
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { PanelResizeHandle, type ImperativePanelHandle } from 'react-resizable-panels'

import { cn } from '@/core/utils'

/**
 * The panel layer, on `react-resizable-panels`.
 *
 * Hand-rolling the splitter was fine for two panes side by side. It stops being
 * fine the moment the layout nests — editor groups inside a vertical split with
 * the bottom panel, inside a horizontal split with two sidebars — because that
 * is where constraint propagation, keyboard resizing and collapse-to-zero all
 * have to agree with each other. The library has already had those arguments.
 */

/**
 * Panels are sized in percentages, which is right for a window that resizes and
 * wrong for a minimum width: 12% of an ultrawide is a 350px floor on a sidebar
 * that wants 200. This measures the group and converts, so the constraints are
 * stated in the units they are actually reasoned about in.
 */
export function usePixelConstraints(
  ref: RefObject<HTMLElement | null>,
): (px: number, ceiling?: number) => number {
  const [size, setSize] = useState(0)

  useLayoutEffect(() => {
    const element = ref.current
    if (!element) return
    setSize(element.getBoundingClientRect().width)
    const observer = new ResizeObserver((entries) => {
      const width = entries[0]?.contentRect.width
      if (width) setSize(width)
    })
    observer.observe(element)
    return () => observer.disconnect()
  }, [ref])

  return useCallback(
    (px: number, ceiling = 90) => {
      // Before the first measurement, a percentage that is merely plausible beats
      // zero — a 0% minimum lets the first paint collapse a panel to nothing.
      if (size <= 0) return Math.min(15, ceiling)
      // The ceiling is what keeps a *set* of minimums from summing past 100%.
      // Three panels each insisting on 320 real pixels is unsatisfiable in a
      // 700px window, and the library's answer to an impossible layout is worse
      // than simply letting the panes get thin.
      return Math.min(ceiling, (px / size) * 100)
    },
    [size],
  )
}

/**
 * Keeps a collapsible panel in step with the store.
 *
 * Both ends can move it — a keyboard command, or a drag that crosses the
 * collapse threshold — so this syncs one way on visibility changes and reports
 * the other way through the panel's own callbacks.
 */
export function usePanelSync(
  ref: RefObject<ImperativePanelHandle | null>,
  visible: boolean,
  /** Whether the owning group has laid out at least once. */
  ready: boolean,
): void {
  // Only act on a change of intent. Calling expand() on every render would fight
  // the user mid-drag.
  const previous = useRef<boolean | null>(null)

  useEffect(() => {
    // Gated on the group's first layout. Before that the imperative API does not
    // merely fail — it logs and throws, so an ungated call on mount both takes
    // the window down and leaves three errors in the console explaining it.
    // `previous` is deliberately left alone here, so the sync runs for real the
    // moment the group is ready.
    if (!ready) return
    if (previous.current === visible) return
    previous.current = visible

    const panel = ref.current
    if (!panel) return
    if (visible && panel.isCollapsed()) panel.expand()
    else if (!visible && !panel.isCollapsed()) panel.collapse()
  }, [ready, ref, visible])
}

/**
 * The drag handle. One pixel of layout, seven of hit area, and it only paints
 * when it is being pointed at — furniture should be findable, not visible.
 */
export function Handle({
  direction = 'horizontal',
  className,
}: {
  direction?: 'horizontal' | 'vertical'
  className?: string
}) {
  const vertical = direction === 'vertical'
  return (
    <PanelResizeHandle
      className={cn(
        'group relative z-20 shrink-0 no-select',
        vertical ? '-my-[3px] h-[7px] cursor-row-resize' : '-mx-[3px] w-[7px] cursor-col-resize',
        className,
      )}
    >
      <span
        aria-hidden
        className={cn(
          'absolute bg-transparent transition-colors duration-100',
          'group-hover:bg-[hsl(var(--accent)/0.7)] group-data-[resize-handle-state=drag]:bg-[hsl(var(--accent))]',
          vertical ? 'inset-x-0 top-[3px] h-px' : 'inset-y-0 left-[3px] w-px',
        )}
      />
    </PanelResizeHandle>
  )
}
