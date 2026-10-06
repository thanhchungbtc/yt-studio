import { getPref as getSetting, usePref as useSetting } from "@/prefs";
import { defaultKeybindings, resolveKeybindings, type KeybindingRule } from "./keybindings";

let cacheUser: KeybindingRule[] | null = null;
let cacheResolved: KeybindingRule[] = defaultKeybindings;

/** Effective keybindings (stable array identity until user rules change). */
export function effectiveKeybindings(): KeybindingRule[] {
  const user = getSetting("keybindings.user");
  if (user !== cacheUser) {
    cacheUser = user;
    cacheResolved = resolveKeybindings(defaultKeybindings, user ?? []);
  }
  return cacheResolved;
}

export function useEffectiveKeybindings(): KeybindingRule[] {
  useSetting("keybindings.user"); // re-render when the user's rules change
  return effectiveKeybindings();
}

// The key shown per command, built once per set of user rules (every menu
// row and palette item asks).
let hintsFor: KeybindingRule[] | null | undefined;
let hints = new Map<string, string>();

/**
 * The key shown for a command: the user's latest binding if they set one,
 * otherwise the first default binding.
 */
export function useKeybindingFor(command: string): string | undefined {
  const user = useSetting("keybindings.user");
  if (user !== hintsFor) {
    hintsFor = user;
    hints = new Map();
    for (const r of effectiveKeybindings()) if (!hints.has(r.command)) hints.set(r.command, r.key);
    for (const r of user ?? []) hints.set(r.command, r.key);
  }
  return hints.get(command);
}
