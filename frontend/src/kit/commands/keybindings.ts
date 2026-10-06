import { matchKeybindingPress, parseKeybinding, type KeybindingPress } from "tinykeys";
import { evaluateWhen } from "./context";
import { executeCommand, getCommand } from "./registry";

/**
 * A keybinding rule, in the same spirit as VS Code's keybindings.json.
 * `key` uses tinykeys syntax: modifiers joined with "+", presses separated by
 * spaces for chords, `$mod` = ⌘ on macOS. A rule whose command starts with
 * "-" removes a default binding of that command.
 */
export interface KeybindingRule {
  key: string;
  command: string;
  when?: string;
  args?: unknown;
}

export const defaultKeybindings: KeybindingRule[] = [
  { key: "$mod+k", command: "workbench.commandPalette" },
  { key: "$mod+Shift+p", command: "workbench.commandPalette" },
  { key: "$mod+p", command: "workbench.goToVideo" },
  { key: "$mod+1", command: "workbench.togglePrimarySidebar" },
  { key: "$mod+2", command: "workbench.togglePanel" },
  { key: "$mod+3", command: "workbench.toggleSecondarySidebar" },
  { key: "$mod+j", command: "workbench.togglePanel" },
  { key: "$mod+Shift+l", command: "workbench.toggleTheme" },
  { key: "$mod+n", command: "video.new" },
  { key: "$mod+Alt+n", command: "video.newFromBlueprint" },
  { key: "$mod+Shift+n", command: "channel.new" },
  { key: "$mod+d", command: "video.duplicate", when: "!inputFocus" },
  { key: "$mod+w", command: "workbench.closeTab" },
  { key: "$mod+Shift+w", command: "workbench.closeOtherTabs" },
  { key: "$mod+Shift+BracketRight", command: "workbench.nextTab" },
  { key: "$mod+Shift+BracketLeft", command: "workbench.previousTab" },
  { key: "Control+Tab", command: "workbench.nextTab" },
  { key: "Control+Shift+Tab", command: "workbench.previousTab" },
  { key: "$mod+Comma", command: "workbench.openSettings" },
  { key: "$mod+Alt+Comma", command: "workbench.openKeybindings" },
  { key: "$mod+f", command: "console.find", when: "!inputFocus" },
  { key: "$mod+Equal", command: "view.zoomIn" },
  { key: "$mod+Minus", command: "view.zoomOut" },
  { key: "$mod+0", command: "view.resetZoom" },
];

/** Combines defaults with user rules: user rules win, "-cmd" removes. */
export function resolveKeybindings(defaults: KeybindingRule[], user: KeybindingRule[]): KeybindingRule[] {
  const removed = user.filter((r) => r.command.startsWith("-"));
  const kept = defaults.filter(
    (d) => !removed.some((r) => r.command.slice(1) === d.command && (!r.key || r.key === d.key)),
  );
  return [...kept, ...user.filter((r) => !r.command.startsWith("-"))];
}

interface CompiledRule extends KeybindingRule {
  presses: KeybindingPress[];
}

function compile(rules: KeybindingRule[]): CompiledRule[] {
  const out: CompiledRule[] = [];
  for (const r of rules) {
    try {
      out.push({ ...r, presses: parseKeybinding(r.key) });
    } catch {
      console.warn("invalid keybinding", r.key);
    }
  }
  return out;
}

const MODIFIER_KEYS = new Set(["Shift", "Meta", "Alt", "Control", "CapsLock", "Fn"]);

/**
 * Installs the global key dispatcher. Rules are matched last-to-first, so
 * user rules override defaults. Supports multi-press chords.
 */
export function installKeybindings(getRules: () => KeybindingRule[], beforeDispatch?: () => void): () => void {
  let lastRules: KeybindingRule[] | null = null;
  let compiledRules: CompiledRule[] = [];
  // getRules should return a stable array; we recompile only when it changes.
  const rules = () => {
    const r = getRules();
    if (r !== lastRules) {
      lastRules = r;
      compiledRules = compile(r);
    }
    return compiledRules;
  };
  let pending: CompiledRule[] | null = null;
  let step = 0;
  let timer: ReturnType<typeof setTimeout> | undefined;

  const reset = () => {
    pending = null;
    step = 0;
    clearTimeout(timer);
  };

  const onKeyDown = (event: KeyboardEvent) => {
    if (event.isComposing || event.keyCode === 229 || MODIFIER_KEYS.has(event.key)) return;
    if (document.body.dataset.recordingKeys === "true") return;
    beforeDispatch?.();

    const candidates = pending ?? rules();
    const matches: CompiledRule[] = [];
    for (let i = candidates.length - 1; i >= 0; i--) {
      const r = candidates[i];
      const press = r.presses[step];
      if (press && matchKeybindingPress(event, press) && evaluateWhen(r.when) && getCommand(r.command)) {
        matches.push(r);
      }
    }
    if (matches.length === 0) {
      if (pending) {
        reset();
        event.preventDefault();
      }
      return;
    }

    const complete = matches.find((r) => r.presses.length === step + 1);
    const longer = matches.filter((r) => r.presses.length > step + 1);
    if (complete && longer.length === 0) {
      event.preventDefault();
      event.stopPropagation();
      reset();
      void executeCommand(complete.command, complete.args);
      return;
    }
    // Wait for the next press of a chord.
    event.preventDefault();
    event.stopPropagation();
    pending = longer.length ? longer : matches;
    step++;
    clearTimeout(timer);
    timer = setTimeout(() => {
      if (complete) void executeCommand(complete.command, complete.args);
      reset();
    }, 1200);
  };

  window.addEventListener("keydown", onKeyDown, true);
  return () => {
    window.removeEventListener("keydown", onKeyDown, true);
    reset();
  };
}

// ---- Display & recording ------------------------------------------------

const SYMBOLS: Record<string, string> = {
  $mod: "⌘",
  Meta: "⌘",
  Control: "⌃",
  Alt: "⌥",
  Shift: "⇧",
  Enter: "↩",
  Escape: "Esc",
  Backspace: "⌫",
  Delete: "⌦",
  Tab: "⇥",
  ArrowUp: "↑",
  ArrowDown: "↓",
  ArrowLeft: "←",
  ArrowRight: "→",
  Space: "Space",
  " ": "Space",
  Comma: ",",
  Period: ".",
  Slash: "/",
  Backslash: "\\",
  BracketLeft: "[",
  BracketRight: "]",
  Semicolon: ";",
  Quote: "'",
  Backquote: "`",
  Minus: "-",
  Equal: "=",
  PageUp: "⇞",
  PageDown: "⇟",
  Home: "↖",
  End: "↘",
};
const MOD_ORDER = ["Control", "Alt", "Shift", "$mod", "Meta"];

/** "$mod+Shift+p" → ["⇧⌘P"]; chords produce one entry per press. */
export function formatKeybinding(key: string): string[] {
  return key
    .trim()
    .split(/\s+/)
    .map((press) => {
      const parts = press.split(/(?<=\w|\])\+/);
      const last = parts.pop() ?? "";
      const mods = parts.sort((a, b) => MOD_ORDER.indexOf(a) - MOD_ORDER.indexOf(b)).map((m) => SYMBOLS[m] ?? m);
      return mods.join("") + formatKey(last);
    });
}

function formatKey(k: string): string {
  if (SYMBOLS[k]) return SYMBOLS[k];
  if (/^Key[A-Z]$/.test(k)) return k.slice(3);
  if (/^Digit\d$/.test(k)) return k.slice(5);
  if (/^F\d+$/.test(k)) return k;
  return k.length === 1 ? k.toUpperCase() : k;
}

const CODE_KEYS = new Set([
  "Comma", "Period", "Slash", "Backslash", "BracketLeft", "BracketRight", "Semicolon", "Quote", "Backquote", "Minus", "Equal",
]);

/** Converts a keydown event into a keybinding press string for recording. */
export function eventToPress(e: KeyboardEvent): string | undefined {
  if (MODIFIER_KEYS.has(e.key)) return undefined;
  const mods: string[] = [];
  if (e.ctrlKey) mods.push("Control");
  if (e.altKey) mods.push("Alt");
  if (e.shiftKey) mods.push("Shift");
  if (e.metaKey) mods.push("$mod");
  let key: string;
  if (/^Key[A-Z]$/.test(e.code)) key = e.altKey ? e.code : e.code.slice(3).toLowerCase();
  else if (/^Digit\d$/.test(e.code)) key = e.altKey || e.shiftKey ? e.code : e.code.slice(5);
  else if (CODE_KEYS.has(e.code)) key = e.code;
  else if (e.key === " ") key = "Space";
  else key = e.key;
  return [...mods, key].join("+");
}
