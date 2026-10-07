import { cn } from "@/kit/lib/cn";

const SEPARATOR = /[/\-_.]/;

/**
 * Splits text so its end stays readable when shortened: the tail is about
 * the last `keep` characters, starting at a separator when one is near
 * ("feature/jira-12" + "-login-flow"). Short text isn't split.
 */
export function splitMiddle(text: string, keep = 10): [string, string] {
  if (text.length <= keep + 4) return [text, ""];
  let at = text.length - keep;
  for (let i = at; i >= Math.max(1, at - 6); i--) {
    if (SEPARATOR.test(text[i])) {
      at = i;
      break;
    }
  }
  return [text.slice(0, at), text.slice(at)];
}

/**
 * Text shortened in the middle when there's no room ("feature/…-login-flow"),
 * by layout alone: the head gives way with an ellipsis, the tail stays. No
 * measuring, so it costs nothing to render or resize.
 */
export function MiddleTruncate({ text, keep, className }: { text: string; keep?: number; className?: string }) {
  const [head, tail] = splitMiddle(text, keep);
  return (
    <span className={cn("flex min-w-0 overflow-hidden whitespace-pre", className)}>
      <span className="min-w-0 truncate">{head}</span>
      {/* Never shrinks (a fraction of a pixel would cost it a letter);
          past the head, the box clips it, so nothing spills out. */}
      {tail && <span className="shrink-0">{tail}</span>}
    </span>
  );
}
