import { useQuery } from '@tanstack/react-query'
import { Check, ChevronDown, Copy } from 'lucide-react'
import { useState } from 'react'

import { api, qk } from '../../../../core/api'
import { cn } from '../../../../core/utils'
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
 * There is a copy button, which there deliberately was not. The argument
 * against it was that ⌘C already reaches selected text through the Edit menu,
 * so a button would be a second way to do what the platform does. That held
 * while the blueprint was only ever a receipt — something you read to check a
 * hunch. It stopped holding once a blueprint became an *input*: the
 * new-from-blueprint dialog takes one of these documents as the thing you paste
 * to start a video, so copying the whole of it is now a real errand rather than
 * a stray impulse. Selecting four hundred lines inside a scrolling popover to
 * do that is the platform doing the thing badly.
 */
export function BlueprintPopover({ assetId }: { assetId: string }) {
  return (
    <Popover
      align="end"
      trigger={
        <button
          type="button"
          className="-mx-1.5 flex shrink-0 items-center gap-1 rounded-[5px] px-1.5 py-0.5 text-sm text-fg-muted transition-colors hover:bg-[var(--hover)] hover:text-fg data-[state=open]:bg-[var(--hover)] data-[state=open]:text-fg"
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
      <div className="hairline-b flex items-center gap-2 px-3 py-1.5 text-xs text-fg-subtle">
        <span className="font-semibold tracking-[0.05em] uppercase">Blueprint</span>
        <span className="min-w-0 flex-1 truncate font-mono">{assetId.slice(0, 12)}</span>
        {/* Given the bytes rather than the query, so it cannot offer to copy a
            document that has not arrived — and copies exactly what is on
            screen, which is the whole claim a raw view makes. */}
        <CopyButton text={asset.data} />
      </div>
      <div className="max-h-[380px] overflow-auto px-3 py-2.5">
        {asset.error ? (
          <p className="text-sm text-[var(--failed)]">{(asset.error as Error).message}</p>
        ) : (
          <pre className="font-mono text-xs leading-relaxed whitespace-pre text-fg">
            {asset.data ?? ''}
          </pre>
        )}
      </div>
    </>
  )
}

/**
 * Copy, and say so.
 *
 * The confirmation is the icon becoming a tick for a moment rather than a
 * message appearing somewhere: the question "did that work" is asked of the
 * button that was just pressed, and answering it anywhere else makes you look
 * for the answer. The same shape as the slide viewer's, in this popover's
 * colours rather than that panel's white-on-image ones.
 *
 * A `text` that has not loaded disables the button instead of hiding it, so the
 * header does not reflow the moment the fetch lands.
 */
function CopyButton({ text }: { text: string | undefined }) {
  const [copied, setCopied] = useState(false)

  return (
    <button
      type="button"
      disabled={!text}
      aria-label={copied ? 'Blueprint copied' : 'Copy the blueprint'}
      title={copied ? 'Copied' : 'Copy the blueprint'}
      onClick={() => {
        if (!text) return
        void navigator.clipboard.writeText(text).then(() => {
          setCopied(true)
          // Long enough to be seen, short enough that the button is ready again
          // before anybody wants it. Nothing cancels it: closing the popover
          // unmounts this and takes the pending timer with it.
          setTimeout(() => setCopied(false), 1400)
        })
      }}
      className={cn(
        '-mr-1 flex size-[22px] shrink-0 items-center justify-center rounded-[6px]',
        'transition-colors disabled:pointer-events-none disabled:opacity-40',
        copied ? 'text-[var(--done)]' : 'text-fg-subtle hover:bg-[var(--hover)] hover:text-fg',
      )}
    >
      {copied ? (
        <Check className="size-[13px]" strokeWidth={2.5} />
      ) : (
        <Copy className="size-[13px]" strokeWidth={2} />
      )}
    </button>
  )
}
