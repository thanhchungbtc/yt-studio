import type { LucideIcon } from "lucide-react";
import { create } from "zustand";
import { toast } from "sonner";
import { evaluateWhen } from "./context";

/**
 * A command is a named action. Everything the user can do — from a menu, the
 * palette or a keybinding — goes through a command, which is what makes every
 * shortcut configurable.
 */
export interface Command {
  id: string;
  title: string;
  category?: string;
  icon?: LucideIcon;
  /** Context in which the command is enabled (see evaluateWhen). */
  when?: string;
  /** Hide from the command palette (still bindable). */
  hidden?: boolean;
  run: (args?: unknown) => unknown | Promise<unknown>;
}

interface RegistryState {
  commands: Record<string, Command>;
}

export const useCommandRegistry = create<RegistryState>(() => ({ commands: {} }));

/** Registers commands; returns a function that unregisters them. */
export function registerCommands(...commands: Command[]): () => void {
  useCommandRegistry.setState((s) => {
    const next = { ...s.commands };
    for (const c of commands) next[c.id] = c;
    return { commands: next };
  });
  return () =>
    useCommandRegistry.setState((s) => {
      const next = { ...s.commands };
      for (const c of commands) if (next[c.id] === c) delete next[c.id];
      return { commands: next };
    });
}

export function getCommand(id: string): Command | undefined {
  return useCommandRegistry.getState().commands[id];
}

/** Runs a command by id if it exists and is enabled in the current context. */
export async function executeCommand(id: string, args?: unknown): Promise<boolean> {
  const cmd = getCommand(id);
  if (!cmd || !evaluateWhen(cmd.when)) return false;
  try {
    await cmd.run(args);
  } catch (err) {
    console.error(`command ${id} failed`, err);
    toast.error(cmd.title, { description: String((err as Error)?.message ?? err) });
  }
  return true;
}
