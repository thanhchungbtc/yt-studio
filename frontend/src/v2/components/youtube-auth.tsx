import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, Copy, ExternalLink, LoaderCircle } from 'lucide-react'
import { useEffect, useState } from 'react'

import { api, qk } from '../core/api'
import { openExternal } from '../core/desktop'
import { Button } from './ui/button'
import { Dialog } from './ui/dialog'
import { Input } from './ui/field'

/**
 * Granting one channel permission to publish.
 *
 * Opened by the thing that needed the permission rather than sat behind a
 * settings screen, because that is the only moment anyone wants to do this. The
 * upload strip asks for it when Approve is pressed and the channel turns out
 * not to be authorized, and hands control back the moment it is — so the
 * operator pressed Approve once and got a publish, with this in the middle.
 *
 * Three steps, and the middle one is a copy-paste because the alternative is
 * worse. A loopback listener would take the redirect automatically, but it
 * needs a port, a port makes a redirect URI the Google console has never seen,
 * and a web-type OAuth client matches its redirects exactly. So the browser
 * lands on an error page whose address bar holds the code, and that gets pasted
 * back here. It looks unfinished and it is the flow that works against a client
 * nobody has to go and edit first.
 */

/**
 * The code out of whatever was pasted.
 *
 * The instruction is to paste the whole URL, because that is what somebody
 * copying out of an address bar has. A bare code still works: it survives both
 * parses unchanged and falls through as itself, which matters because Google's
 * codes contain slashes and look like nothing in particular.
 */
function extractCode(pasted: string): string {
  const raw = pasted.trim()
  if (!raw) return ''
  try {
    const url = raw.startsWith('http') ? new URL(raw) : new URL('http://x?' + raw)
    return url.searchParams.get('code') ?? raw
  } catch {
    return raw
  }
}

interface Props {
  /** Channel slug or id; the route takes either. */
  channel: string
  open: boolean
  onOpenChange: (open: boolean) => void
  /**
   * Called once the channel can publish, with the dialog already closing.
   *
   * The caller resumes whatever it was doing. Nothing here knows what that is,
   * which is why this is a callback and not a publish.
   */
  onAuthorized: () => void
  /** What the operator is authorizing *for*, on the button that does it. */
  confirmLabel?: string
}

export function YouTubeAuthDialog({
  channel,
  open,
  onOpenChange,
  onAuthorized,
  confirmLabel = 'Authorize',
}: Props) {
  const client = useQueryClient()
  const [pasted, setPasted] = useState('')
  const [copied, setCopied] = useState(false)
  /*
    Set when the browser could not be opened for us.

    Worth its own state because the failure is silent everywhere it happens — a
    blocked popup, a shell with no binding — and a button that does nothing is
    the one thing this dialog cannot afford: the URL beside it is still a
    perfectly good way through, and nobody would think to use it unless told.
  */
  const [openFailed, setOpenFailed] = useState(false)
  // Whether the consent page has been opened. The one piece of state the server
  // cannot answer for: everything else on this screen is derived from what is
  // on disk, but nobody knows whether a browser tab was opened except us.
  const [visited, setVisited] = useState(false)

  const auth = useQuery({
    queryKey: qk.channelAuth(channel),
    queryFn: () => api.channelAuth(channel),
    enabled: open,
    // Always re-read on open. This is the screen whose whole purpose is to be
    // right about a file that may have changed since anything last looked.
    staleTime: 0,
  })

  const url = useQuery({
    queryKey: [...qk.channelAuth(channel), 'url'],
    queryFn: () => api.channelAuthUrl(channel),
    // Only once there is a client to build one from, and not at all once the
    // channel is authorized: a consent URL for a grant that already exists is a
    // way to lose the grant.
    enabled: open && auth.data?.clientPresent === true && auth.data.authorized === false,
    staleTime: 0,
  })

  const authorize = useMutation({
    mutationFn: () => api.authorizeChannel(channel, extractCode(pasted)),
    onSuccess: (result) => {
      void client.invalidateQueries({ queryKey: qk.channelAuth(channel) })
      // The channel's credentials field moved with it, and the sidebar reads it.
      void client.invalidateQueries({ queryKey: qk.channels })
      if (result.authorized) {
        onOpenChange(false)
        onAuthorized()
      }
    },
  })

  // Reset between openings. A dialog that reopens showing the last attempt's
  // half-pasted URL is one that has to be closed twice.
  useEffect(() => {
    if (open) return
    setPasted('')
    setCopied(false)
    setVisited(false)
    setOpenFailed(false)
    authorize.reset()
    // Keyed on `open` alone, deliberately. The mutation is the thing being
    // reset, so listing it would re-run this on every state change it makes —
    // which is every state change there is.
  }, [open])

  // Already authorized when it opened — a token placed by hand, or two tabs on
  // the same channel. Nothing to ask, so it gets out of the way and lets the
  // caller carry on rather than making somebody dismiss a dialog about a
  // question that has no remaining answer.
  useEffect(() => {
    if (!open || !auth.data?.authorized || authorize.isPending) return
    onOpenChange(false)
    onAuthorized()
    // Keyed on the answer, not on the callbacks: they are the caller's, and a
    // caller that rebuilds them each render would otherwise fire this twice.
  }, [open, auth.data?.authorized])

  const code = extractCode(pasted)
  const name = auth.data?.channel.name ?? 'this channel'

  const copy = () => {
    if (!url.data) return
    void navigator.clipboard.writeText(url.data).then(() => setCopied(true))
  }

  const openConsent = () => {
    if (!url.data) return
    // The step advances either way. A page that opened by hand from the copied
    // URL leaves the operator at exactly the same place as one this opened, and
    // refusing to move on would be the dialog disbelieving them.
    setVisited(true)
    void openExternal(url.data).then((opened) => setOpenFailed(!opened))
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange} width={520}>
      <Dialog.Header
        title="Authorize YouTube"
        description={`${name} can publish once Google has granted this app permission.`}
      />
      <Dialog.Close />

      <Dialog.Body>
        {auth.isPending ? <Waiting>Reading this channel's credentials…</Waiting> : null}

        {auth.error ? <Problem>{(auth.error as Error).message}</Problem> : null}

        {auth.data && !auth.data.clientPresent ? (
          <NoClient path={auth.data.clientPath} onRecheck={() => void auth.refetch()} busy={auth.isFetching} />
        ) : null}

        {auth.data?.clientPresent && !auth.data.authorized ? (
          <div className="flex flex-col gap-5">
            {auth.data.status === 'expired' ? (
              <Note>
                Google is no longer renewing this channel's grant — revoked from the account, or
                simply unused for too long. Authorizing again replaces it.
              </Note>
            ) : null}

            <Step n={1} title="Open Google and sign in">
              <p className="text-[12px] leading-relaxed text-secondary">
                Sign in with the account that owns {name}. Grant the upload permission it asks for.
              </p>
              {url.isPending ? (
                <Waiting>Building the consent URL…</Waiting>
              ) : url.error ? (
                <Problem>{(url.error as Error).message}</Problem>
              ) : (
                <>
                  <div className="flex items-center gap-2 rounded-[7px] px-2.5 py-1.5" style={{ backgroundColor: 'var(--content)', boxShadow: '0 0 0 0.5px var(--separator-strong)' }}>
                    <code className="min-w-0 flex-1 truncate font-mono text-[10px] text-tertiary">
                      {url.data}
                    </code>
                    <button
                      type="button"
                      onClick={copy}
                      aria-label="Copy the consent URL"
                      className="shrink-0 rounded-[4px] p-1 text-secondary transition-colors hover:bg-[var(--hover)] hover:text-primary"
                    >
                      {copied ? <Check className="size-3" strokeWidth={2.5} /> : <Copy className="size-3" strokeWidth={2} />}
                    </button>
                  </div>
                  <Button primary onClick={openConsent}>
                    <ExternalLink className="mr-1.5 size-3" strokeWidth={2} />
                    Open Google sign-in
                  </Button>
                  {openFailed ? (
                    <p className="text-[11px] leading-relaxed text-[var(--failed)]">
                      This machine would not open a browser. Copy the URL above and paste it into
                      one yourself — the rest of this works the same.
                    </p>
                  ) : null}
                </>
              )}
            </Step>

            <Step n={2} title="Paste the address you land on">
              <p className="text-[12px] leading-relaxed text-secondary">
                Google sends the browser to a page that will not load, starting{' '}
                <code className="font-mono text-[11px] text-primary">http://localhost/?code=</code>.
                That failure is expected — nothing of this app is listening there. Copy the whole
                address out of the bar and paste it below.
              </p>
              <Input
                data-autofocus={visited ? '' : undefined}
                value={pasted}
                spellCheck={false}
                placeholder="http://localhost/?code=4/0AX4XfWi…"
                onChange={(event) => setPasted(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === 'Enter' && code) authorize.mutate()
                }}
                className="font-mono text-[11px]"
              />
              {code && code !== pasted.trim() ? (
                <p className="text-[11px] text-tertiary">
                  Code found: <code className="font-mono text-primary">{code}</code>
                </p>
              ) : null}
            </Step>

            {authorize.error ? <Problem>{(authorize.error as Error).message}</Problem> : null}
          </div>
        ) : null}
      </Dialog.Body>

      {auth.data?.clientPresent && !auth.data.authorized ? (
        <Dialog.Footer>
          <span className="min-w-0 flex-1 truncate text-[11px] text-tertiary">
            The grant is stored on this machine and is not asked for again.
          </span>
          <Button onClick={() => onOpenChange(false)}>Cancel</Button>
          <Button primary disabled={!code || authorize.isPending} onClick={() => authorize.mutate()}>
            {authorize.isPending ? 'Authorizing…' : confirmLabel}
          </Button>
        </Dialog.Footer>
      ) : null}
    </Dialog>
  )
}

/** One numbered step, which is the only structure this screen has. */
function Step({ n, title, children }: { n: number; title: string; children: React.ReactNode }) {
  return (
    <div className="flex gap-3">
      <span
        className="mt-px flex size-[18px] shrink-0 items-center justify-center rounded-full text-[10px] font-semibold text-white"
        style={{ backgroundColor: 'var(--accent)' }}
      >
        {n}
      </span>
      <div className="flex min-w-0 flex-1 flex-col items-start gap-2">
        <h3 className="text-[13px] font-semibold text-primary">{title}</h3>
        {children}
      </div>
    </div>
  )
}

/**
 * There is no OAuth client for this channel, which is not the same as not being
 * authorized and is why it gets its own panel: the remedy is a file, and the
 * only useful thing this screen can do is say exactly where it goes.
 */
function NoClient({ path, onRecheck, busy }: { path?: string; onRecheck: () => void; busy: boolean }) {
  return (
    <div className="flex flex-col gap-4">
      <Note>
        This channel has no OAuth client yet. Google issues one per project, and it is the file
        that lets this app ask for permission at all.
      </Note>
      <ol className="flex list-none flex-col gap-1.5 pl-0 text-[12px] leading-relaxed text-secondary">
        {[
          'In the Google Cloud console, enable the YouTube Data API v3 for your project.',
          'Under APIs & Services → Credentials, create an OAuth 2.0 Client ID.',
          'Add http://localhost to its authorized redirect URIs.',
          'Download the JSON and save it at the path below.',
        ].map((line, i) => (
          <li key={line} className="flex gap-2">
            <span className="shrink-0 font-semibold text-[var(--accent)]">{i + 1}.</span>
            <span>{line}</span>
          </li>
        ))}
      </ol>
      {path ? (
        <code
          className="block rounded-[7px] px-2.5 py-1.5 font-mono text-[11px] break-all text-primary"
          style={{ backgroundColor: 'var(--content)', boxShadow: '0 0 0 0.5px var(--separator-strong)' }}
        >
          {path}
        </code>
      ) : null}
      <div>
        <Button onClick={onRecheck} disabled={busy}>
          {busy ? 'Checking…' : 'Check again'}
        </Button>
      </div>
    </div>
  )
}

function Waiting({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex items-center gap-2 py-6 text-[12px] text-secondary">
      <LoaderCircle className="size-3.5 animate-spin" strokeWidth={2} style={{ color: 'var(--running)' }} />
      {children}
    </div>
  )
}

function Note({ children }: { children: React.ReactNode }) {
  return (
    <p
      className="rounded-[7px] px-3 py-2.5 text-[12px] leading-relaxed text-secondary"
      style={{ backgroundColor: 'var(--accent-wash)' }}
    >
      {children}
    </p>
  )
}

function Problem({ children }: { children: React.ReactNode }) {
  return <p className="text-[12px] leading-relaxed text-[var(--failed)]">{children}</p>
}
