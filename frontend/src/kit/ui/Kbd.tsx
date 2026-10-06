import { formatKeybinding } from "@/kit/commands/keybindings";
import { useKeybindingFor } from "@/kit/commands/useKeybindings";
import { cn } from "@/kit/lib/cn";

/** Renders a keybinding string ("$mod+k") as key caps. */
export function Kbd({ keys, className }: { keys: string; className?: string }) {
  return (
    <span className={cn("inline-flex items-center gap-1", className)}>
      {formatKeybinding(keys).map((press, i) => (
        <kbd
          key={i}
          className="rounded-[4px] bg-well px-1 py-px font-sans text-2xs leading-none tracking-wide text-fg-muted shadow-[inset_0_-0.5px_0_var(--line-strong)]"
        >
          {press}
        </kbd>
      ))}
    </span>
  );
}

/** Shows the current keybinding of a command (reflects user overrides). */
export function KeybindingHint({ command, className }: { command: string; className?: string }) {
  const key = useKeybindingFor(command);
  if (!key) return null;
  return <Kbd keys={key} className={className} />;
}
