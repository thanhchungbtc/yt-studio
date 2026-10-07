import { basename } from "@/kit/lib/format";
import { cn } from "@/kit/lib/cn";

/**
 * A path that truncates its directory part first, so the file name — the
 * part people scan for — stays visible. `quiet` tones it down (as in tool
 * call rows, which step back from the conversation until hovered).
 */
export function PathText({ path, className, quiet }: { path: string; className?: string; quiet?: boolean }) {
  const name = basename(path) || path;
  const dir = path.slice(0, path.length - name.length);
  return (
    <span className={cn("flex min-w-0 items-baseline font-mono text-[11.5px]", className)} title={path}>
      {dir && <span className={cn("min-w-0 truncate", quiet ? "text-fg-faint group-hover/tool:text-fg-subtle" : "text-fg-subtle")}>{dir}</span>}
      <span className={cn("shrink-0", quiet ? "text-fg-muted group-hover/tool:text-fg" : "text-fg")}>{name}</span>
    </span>
  );
}
