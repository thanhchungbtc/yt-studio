/**
 * Soft scroll edges: marks `.scroll-edge` scrollers with data-edge-top /
 * data-edge-bottom while content lies beyond that edge, so the stylesheet can
 * fade it out under headers (Liquid Glass). One passive, capturing listener
 * serves the whole app; marks only change when they flip.
 */
function mark(el: HTMLElement) {
  const top = el.scrollTop > 1;
  const bottom = el.scrollTop + el.clientHeight < el.scrollHeight - 1;
  if (el.hasAttribute("data-edge-top") !== top) el.toggleAttribute("data-edge-top", top);
  if (el.hasAttribute("data-edge-bottom") !== bottom) el.toggleAttribute("data-edge-bottom", bottom);
}

export function installScrollEdges(): () => void {
  const onScroll = (e: Event) => {
    const t = e.target;
    if (t instanceof HTMLElement && t.classList.contains("scroll-edge")) mark(t);
  };
  // Content can grow without scrolling: refresh when the pointer comes back.
  const onEnter = (e: Event) => {
    const el = (e.target as HTMLElement | null)?.closest?.<HTMLElement>(".scroll-edge");
    if (el) mark(el);
  };
  document.addEventListener("scroll", onScroll, { capture: true, passive: true });
  document.addEventListener("pointerover", onEnter, { passive: true });
  // Overlays and new tabs animate in only after startup.
  const ready = setTimeout(() => document.documentElement.toggleAttribute("data-ready", true), 1200);
  return () => {
    document.removeEventListener("scroll", onScroll, { capture: true });
    document.removeEventListener("pointerover", onEnter);
    clearTimeout(ready);
  };
}
