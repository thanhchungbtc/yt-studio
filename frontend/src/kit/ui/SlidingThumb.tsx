import { useCallback, useLayoutEffect, useRef, useState } from "react";
import { cn } from "@/kit/lib/cn";

/**
 * The selection of a segmented control, gliding between options (Liquid
 * Glass). Render it first inside the control (which must be `relative`);
 * options carry data-thumb-key and draw no background of their own. It
 * measures within its own parent (so it never waits for a parent's ref),
 * doesn't animate on first paint, and follows size changes (zoom, labels).
 */
export function SlidingThumb({ active, className }: { active: string | undefined; className?: string }) {
  const thumb = useRef<HTMLSpanElement>(null);
  const [box, setBox] = useState<{ x: number; y: number; w: number; h: number }>();
  const [moving, setMoving] = useState(false);
  const latest = useRef(active);
  latest.current = active;

  const measure = useCallback(() => {
    const root = thumb.current?.parentElement;
    const key = latest.current;
    const el = !root || key === undefined ? null : root.querySelector<HTMLElement>(`[data-thumb-key="${CSS.escape(key)}"]`);
    setBox((prev) => {
      if (!el) return undefined;
      const next = { x: el.offsetLeft, y: el.offsetTop, w: el.offsetWidth, h: el.offsetHeight };
      return prev && prev.x === next.x && prev.y === next.y && prev.w === next.w && prev.h === next.h ? prev : next;
    });
  }, []);

  // A selection change glides (not the first paint).
  const first = useRef(true);
  useLayoutEffect(() => {
    if (first.current) first.current = false;
    else setMoving(true);
    measure();
  }, [active, measure]);

  // Size changes (zoom, labels) snap.
  useLayoutEffect(() => {
    const root = thumb.current?.parentElement;
    if (!root) return;
    let size = `${root.offsetWidth}x${root.offsetHeight}`;
    const ro = new ResizeObserver(() => {
      const now = `${root.offsetWidth}x${root.offsetHeight}`;
      if (now === size) return;
      size = now;
      setMoving(false);
      measure();
    });
    ro.observe(root);
    return () => ro.disconnect();
  }, [measure]);

  return (
    <span
      ref={thumb}
      aria-hidden
      data-moving={moving || undefined}
      className={cn("sliding-thumb glass-thumb pointer-events-none absolute top-0 left-0 rounded-full", !box && "invisible", className)}
      style={box ? { width: box.w, height: box.h, transform: `translate(${box.x}px, ${box.y}px)` } : undefined}
    />
  );
}
