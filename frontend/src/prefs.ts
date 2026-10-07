import { create } from 'zustand'
import { persist } from 'zustand/middleware'

import type { KeybindingRule } from '@/kit/commands/keybindings'

/** Interface preferences, kept in the window (the backend's settings are the pipeline's). */
export interface Prefs {
  theme: 'system' | 'light' | 'dark'
  material: 'liquidGlass' | 'translucent' | 'solid'
  fontSize: number
  'keybindings.user': KeybindingRule[]
}

export type PrefKey = keyof Prefs

export const DEFAULT_PREFS: Prefs = {
  theme: 'system',
  material: 'liquidGlass',
  fontSize: 13,
  'keybindings.user': [],
}

export const usePrefs = create<Prefs>()(
  persist(() => ({ ...DEFAULT_PREFS }), { name: 'yts.prefs', version: 1 }),
)

export function getPref<K extends PrefKey>(key: K): Prefs[K] {
  return usePrefs.getState()[key]
}

export function usePref<K extends PrefKey>(key: K): Prefs[K] {
  return usePrefs((s) => s[key])
}

export function setPref<K extends PrefKey>(key: K, value: Prefs[K]): void {
  usePrefs.setState({ [key]: value } as Partial<Prefs>)
}

export function resetPref(key: PrefKey): void {
  setPref(key, DEFAULT_PREFS[key])
}
