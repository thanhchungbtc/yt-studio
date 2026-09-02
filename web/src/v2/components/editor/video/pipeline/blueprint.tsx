import { useQuery } from '@tanstack/react-query'
import { ChevronDown } from 'lucide-react'

import { api, qk } from '../../../../core/api'
import { Popover } from '../../../ui/popover'

/**
 * The blueprint, as the model returned it, on a glance.
 *
 * A popover and not a document: reading the raw plan is something you do for
 * ten seconds to check a hunch, and a tab you have to close afterwards charges
 * more for that than it is worth.
 *
 * Shown verbatim. The server already stores it indented, and re-formatting the
 * bytes would make this a *rendering* of the blueprint rather than the
 * blueprint — which is the entire point of a raw view. If it ever stops being
 * JSON it still shows, because nothing here parses it.
 *
 * There is no copy button. The text is selectable and ⌘C reaches it through the
 * Edit menu the window installs; a button would be a second way to do a thing
 * the platform already does.
 */
export function BlueprintPopover({ assetId }: { assetId: string }) {
  return (
    <Popover
      align="end"
      trigger={
        <button
          type="button"
          className="-mx-1.5 flex shrink-0 items-center gap-1 rounded-[5px] px-1.5 py-0.5 text-[12px] text-secondary transition-colors hover:bg-[var(--hover)] hover:text-primary data-[state=open]:bg-[var(--hover)] data-[state=open]:text-primary"
        >
          Blueprint
          <ChevronDown className="size-3 opacity-60" strokeWidth={2.5} />
        </button>
      }
    >
      <Body assetId={assetId} />
    </Popover>
  )
}

/**
 * Fetched here rather than in the trigger, so nothing is read until the
 * popover is actually opened — the content only mounts on open.
 */
function Body({ assetId }: { assetId: string }) {
  const asset = useQuery({
    queryKey: qk.asset(assetId),
    queryFn: () => api.assetText(assetId),
    // The address is a hash of the bytes, so there is no such thing as a stale
    // copy: a changed blueprint is a different asset under a different key.
    staleTime: Infinity,
  })

  return (
    <>
      <div className="hairline-b flex items-center gap-2 px-3 py-1.5 text-[11px] text-tertiary">
        <span className="font-semibold tracking-[0.05em] uppercase">Blueprint</span>
        <span className="min-w-0 flex-1 truncate font-mono">{assetId.slice(0, 12)}</span>
      </div>
      <div className="max-h-[380px] overflow-auto px-3 py-2.5">
        {asset.error ? (
          <p className="text-[12px] text-[var(--failed)]">{(asset.error as Error).message}</p>
        ) : (
          <pre className="font-mono text-[11.5px] leading-relaxed whitespace-pre text-primary">
            {asset.data ?? ''}
          </pre>
        )}
      </div>
    </>
  )
}
