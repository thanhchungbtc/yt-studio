import { create } from "zustand";

/**
 * Context keys drive `when` clauses of commands and keybindings, like VS
 * Code: `composerFocus && !paletteOpen`, `sessionBusy || permissionPending`.
 */
export type ContextKeys = Record<string, boolean | string | number | undefined>;

export const useContextKeys = create<ContextKeys>(() => ({}));

export function setContextKey(key: string, value: boolean | string | number | undefined) {
  if (useContextKeys.getState()[key] !== value) useContextKeys.setState({ [key]: value });
}

export function getContextKeys(): ContextKeys {
  return useContextKeys.getState();
}

/**
 * Evaluates a `when` clause. Supports `a && b`, `a || b`, `!a`,
 * `key == value` and `key != value`. `&&` binds tighter than `||`.
 */
export function evaluateWhen(expr: string | undefined, ctx: ContextKeys = getContextKeys()): boolean {
  if (!expr || !expr.trim()) return true;
  return expr.split("||").some((alt) => alt.split("&&").every((term) => evalTerm(term.trim(), ctx)));
}

function evalTerm(term: string, ctx: ContextKeys): boolean {
  if (!term) return true;
  const neq = term.split("!=");
  if (neq.length === 2) return String(ctx[neq[0].trim()] ?? "") !== unquote(neq[1].trim());
  const eq = term.split("==");
  if (eq.length === 2) return String(ctx[eq[0].trim()] ?? "") === unquote(eq[1].trim());
  if (term.startsWith("!")) return !evalTerm(term.slice(1).trim(), ctx);
  return !!ctx[term];
}

function unquote(s: string): string {
  return s.replace(/^['"]|['"]$/g, "");
}
