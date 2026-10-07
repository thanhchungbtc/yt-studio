import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'

import { DEFAULT_PREFS, usePrefs, type PrefKey } from '@/prefs'

import { api, qk } from '../../core/api'
import type { Setting } from '../../core/types'

export function useSettingRows() {
  return useQuery({ queryKey: qk.settings, queryFn: api.listSettings })
}

/** Changed from the default (a secret: set at all). */
export function isModified(row: Setting): boolean {
  return row.secret ? row.configured : row.value !== row.default
}

/** Writes a setting, showing it at once and taking it back if the backend refuses. */
export function useSaveSetting() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: ({ key, value }: { key: string; value: string }) => api.updateSetting(key, value),
    onMutate: async ({ key, value }) => {
      await client.cancelQueries({ queryKey: qk.settings })
      const previous = client.getQueryData<Setting[]>(qk.settings)
      client.setQueryData<Setting[]>(qk.settings, (rows) =>
        rows?.map((r) =>
          r.key === key
            ? {
                ...r,
                value: r.secret ? '' : value,
                configured: r.secret ? value !== '' : r.configured,
              }
            : r,
        ),
      )
      return { previous }
    },
    onError: (error, _vars, context) => {
      if (context?.previous) client.setQueryData(qk.settings, context.previous)
      toast.error("Couldn't save the setting", { description: (error as Error).message })
    },
    onSuccess: (row) => {
      client.setQueryData<Setting[]>(qk.settings, (rows) =>
        rows?.map((r) => (r.key === row.key ? row : r)),
      )
    },
  })
}

const PAGE_PREFS: PrefKey[] = ['theme', 'material', 'fontSize', 'keybindings.user']

export function prefModified(key: PrefKey, value: unknown): boolean {
  return JSON.stringify(value) !== JSON.stringify(DEFAULT_PREFS[key])
}

/** How many interface preferences differ from the defaults. */
export function useChangedPrefs(keys: PrefKey[] = PAGE_PREFS): number {
  return usePrefs((s) => keys.filter((k) => prefModified(k, s[k])).length)
}
