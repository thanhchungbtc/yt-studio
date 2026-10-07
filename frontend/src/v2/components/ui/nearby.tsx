import { createContext, useCallback, useContext, useLayoutEffect, useRef, useState } from 'react'

// WebKit ignores content-visibility: auto, so draw only what's near.
type Watch = (element: Element, onChange: (near: boolean) => void) => () => void

const Nearby = createContext<Watch | null>(null)

export const NearbyProvider = Nearby.Provider

export function useNearbyRoot(): { root: (element: HTMLElement | null) => void; watch: Watch } {
  const listeners = useRef(new Map<Element, (near: boolean) => void>())
  const observer = useRef<IntersectionObserver | null>(null)
  const [element, setElement] = useState<HTMLElement | null>(null)

  useLayoutEffect(() => {
    if (!element) return
    const next = new IntersectionObserver(
      (entries) => {
        // A hidden scroller reports everything as gone; ignore it.
        const bounds = entries[0]?.rootBounds
        if (!bounds || bounds.height === 0) return
        for (const entry of entries) listeners.current.get(entry.target)?.(entry.isIntersecting)
      },
      { root: element, rootMargin: '150% 0px' },
    )
    observer.current = next
    // Items' layout effects ran first; observe what they queued.
    for (const target of listeners.current.keys()) next.observe(target)
    return () => {
      next.disconnect()
      observer.current = null
    }
  }, [element])

  const watch = useCallback<Watch>((target, onChange) => {
    listeners.current.set(target, onChange)
    observer.current?.observe(target)
    return () => {
      listeners.current.delete(target)
      observer.current?.unobserve(target)
    }
  }, [])

  return { root: setElement, watch }
}

export function useNear(
  element: { current: Element | null },
  eager: boolean,
  onAway?: () => void,
): boolean {
  const watch = useContext(Nearby)
  const [near, setNear] = useState(eager || !watch)
  const away = useRef(onAway)
  away.current = onAway
  useLayoutEffect(() => {
    const target = element.current
    if (!watch || !target) return
    return watch(target, (next) => {
      if (!next) away.current?.()
      setNear(next)
    })
  }, [watch, element])
  return near
}
