import { useLayoutEffect, useRef, type DependencyList, type RefObject } from "react";

const reducedMotion = typeof matchMedia === "function" ? matchMedia("(prefers-reduced-motion: reduce)") : undefined;

// FLIP by offsetTop (container must be the offset parent) so in-flight animations don't skew it; a `layout` change skips animating.
// `deps`: what can move the items (measuring forces a layout, so not on every render).
export function useFlip(container: RefObject<HTMLElement | null>, layout: string, deps?: DependencyList) {
  const tops = useRef<Map<string, number> | null>(null);
  const lastLayout = useRef(layout);
  useLayoutEffect(() => {
    const root = container.current;
    if (!root) return;
    const els = root.querySelectorAll<HTMLElement>("[data-flip]");
    const prev = tops.current;
    const next = new Map<string, number>();
    const animate = prev && lastLayout.current === layout && !reducedMotion?.matches;
    lastLayout.current = layout;
    // Only what's in (or near) view is worth moving.
    const from = root.scrollTop - root.clientHeight;
    const to = root.scrollTop + root.clientHeight * 2;
    for (const el of els) {
      const key = el.dataset.flip!;
      const top = el.offsetTop;
      next.set(key, top);
      if (!animate || top < from || top > to) continue;
      const was = prev.get(key);
      if (was === undefined) {
        el.animate([{ opacity: 0 }, { opacity: 1 }], { duration: 180, easing: "ease-out" });
      } else if (was !== top) {
        el.animate([{ transform: `translateY(${was - top}px)` }, { transform: "none" }], {
          duration: 240,
          easing: "cubic-bezier(0.2, 0, 0, 1)",
        });
      }
    }
    tops.current = next;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps && [layout, ...deps]);
}
