import { useCallback, useEffect, useRef, type RefObject } from 'react'

const SECTION = '[data-chapter]'

const reducedMotion = () => document.documentElement.dataset.motion === 'reduced'

/** The first section still showing at the top of the scroller. */
function topmost(sections: HTMLElement[], scrollTop: number): HTMLElement | null {
  let lo = 0
  let hi = sections.length - 1
  let found: HTMLElement | null = null
  while (lo <= hi) {
    const mid = (lo + hi) >> 1
    const section = sections[mid]!
    if (section.offsetTop + section.offsetHeight > scrollTop) {
      found = section
      hi = mid - 1
    } else {
      lo = mid + 1
    }
  }
  return found
}

/** Scroll anchoring, which WebKit lacks: keeps the section being read still. */
export function useScrollAnchor(scroller: RefObject<HTMLElement | null>, key: unknown) {
  useEffect(() => {
    const root = scroller.current
    if (!root) return
    const sections = [...root.querySelectorAll<HTMLElement>(SECTION)]
    if (sections.length === 0) return

    let anchor: HTMLElement | null = null
    let offset = 0
    let last = root.scrollTop
    // Anything moving the anchor besides the scroll itself is a shift to undo.
    const reconcile = () => {
      const scrollTop = root.scrollTop
      offset -= scrollTop - last
      last = scrollTop
      if (anchor) {
        const shift = anchor.offsetTop - scrollTop - offset
        if (Math.abs(shift) >= 1) {
          root.scrollTop = scrollTop + shift
          last = root.scrollTop
        }
      }
      anchor = topmost(sections, last)
      offset = anchor ? anchor.offsetTop - last : 0
    }

    const resize = new ResizeObserver(reconcile)
    for (const section of sections) resize.observe(section)
    root.addEventListener('scroll', reconcile, { passive: true })
    reconcile()
    return () => {
      resize.disconnect()
      root.removeEventListener('scroll', reconcile)
    }
  }, [scroller, key])
}

/** One glide at a time toward a section's live position; any user input ends it. */
export function useGlide(scroller: RefObject<HTMLElement | null>) {
  const current = useRef<{ frame: number; done: () => void } | null>(null)

  const cancel = useCallback(() => {
    const glide = current.current
    if (!glide) return
    current.current = null
    cancelAnimationFrame(glide.frame)
    glide.done()
  }, [])

  useEffect(() => {
    const root = scroller.current
    if (!root) return
    const options = { passive: true } as const
    for (const type of ['wheel', 'pointerdown', 'keydown', 'touchstart'] as const) {
      root.addEventListener(type, cancel, options)
    }
    return () => {
      for (const type of ['wheel', 'pointerdown', 'keydown', 'touchstart'] as const) {
        root.removeEventListener(type, cancel)
      }
      cancel()
    }
  }, [scroller, cancel])

  const glide = useCallback(
    (target: HTMLElement, done: () => void) => {
      const root = scroller.current
      if (!root) return
      cancel()
      const goal = () => Math.min(target.offsetTop, root.scrollHeight - root.clientHeight)

      if (reducedMotion()) {
        root.scrollTop = goal()
        done()
        return
      }

      // Skip most of a long way at once; glide only the last stretch.
      const reach = root.clientHeight * 1.5
      const gap = goal() - root.scrollTop
      if (Math.abs(gap) > reach) root.scrollTop = goal() - Math.sign(gap) * reach

      const started = performance.now()
      let last = started
      const tick = (now: number) => {
        const remaining = goal() - root.scrollTop
        if (Math.abs(remaining) < 1 || now - started > 900) {
          root.scrollTop = goal()
          current.current = null
          done()
          return
        }
        const ease = 1 - Math.pow(0.75, (now - last) / 16.67)
        last = now
        const step = remaining * ease
        root.scrollTop += Math.abs(step) < 1 ? Math.sign(remaining) : step
        if (current.current) current.current.frame = requestAnimationFrame(tick)
      }
      current.current = { frame: requestAnimationFrame(tick), done }
    },
    [scroller, cancel],
  )

  return glide
}
