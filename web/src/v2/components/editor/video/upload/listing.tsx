import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState, type ReactNode, type RefObject } from 'react'

import { api, qk } from '../../../../core/api'
import type { Video } from '../../../../core/types'
import { Button } from '../../../ui/button'
import { Caption } from '../../../ui/caption'
import { Input, Textarea } from '../../../ui/field'

/**
 * Everything that gets sent, in the form it gets sent in.
 *
 * The fields are printed, not styled. An earlier version of this page drew the
 * title as a heading and the tags as pills, which made it a mock of YouTube —
 * and a mock is a claim about how somebody else's software will lay your text
 * out, which is a claim this application is in no position to make. A label and
 * the string is the whole of what is actually known.
 *
 * So the values are mono, verbatim, and wrap without being reflowed. The
 * description keeps its own blank lines and its own breaks because those bytes
 * are what the upload carries; a description "tidied" for display would be the
 * one thing on the page that is not what publishes.
 *
 * Reading is the default and editing is a mode, because this screen is opened to
 * check what is about to be published far more often than to change it. Pressing
 * Edit swaps the three strings for the controls that write them; nothing else on
 * the page moves.
 */
export function Listing({
  video,
  playerRef,
  onTime,
  onRuntime,
  onBuild,
}: {
  video: Video
  playerRef: RefObject<HTMLVideoElement | null>
  onTime: (seconds: number) => void
  /** How long the cut turned out to be; the chapter list uses these units. */
  onRuntime: (seconds: number) => void
  /** Opens the builder over this video. */
  onBuild: () => void
}) {
  return (
    <div className="flex flex-col gap-6 px-6 py-5">
      <Player video={video} playerRef={playerRef} onTime={onTime} onRuntime={onRuntime} />

      <Fields video={video} />

      <Thumbnail video={video} onBuild={onBuild} />
    </div>
  )
}

/** The draft, as it is typed: tags are one comma-separated line, not a list. */
interface Draft {
  title: string
  description: string
  tags: string
}

/**
 * The three strings, read or written.
 *
 * `undefined` is the read mode rather than a separate flag, so the draft cannot
 * outlive the editing of it: leaving the mode is the same statement as throwing
 * the draft away, and there is no third state where a stale draft sits behind a
 * page showing the saved values.
 */
function Fields({ video }: { video: Video }) {
  const metadata = video.metadata
  const client = useQueryClient()
  const [draft, setDraft] = useState<Draft>()

  const save = useMutation({
    mutationFn: (edit: Draft) =>
      api.saveMetadata(video.ref, {
        // What the server handed over, with the three fields this screen owns
        // replaced. The spread is the whole of the round trip: `thumbnailText`
        // and the two nobody displays go back exactly as they came, so editing a
        // title cannot quietly erase the hook the thumbnail was built around.
        thumbnailText: '',
        categoryId: '',
        privacy: '',
        ...metadata,
        title: edit.title,
        description: edit.description,
        // Split here rather than on the server, because a comma-separated line
        // is a fact about this control and not about the format. Empty entries
        // survive the split — a list is finished with a trailing comma as often
        // as not — and are dropped when they are written.
        tags: edit.tags
          .split(',')
          .map((tag) => tag.trim())
          .filter(Boolean),
      }),
    onSuccess: (next) => {
      // The response is the whole video, so the cache takes it directly. The
      // list is invalidated rather than written: a title change shows up in the
      // sidebar row, which is keyed differently.
      client.setQueryData(qk.video(video.ref), next)
      void client.invalidateQueries({ queryKey: qk.videos })
      setDraft(undefined)
    },
  })

  if (!draft) {
    return (
      <div className="flex flex-col gap-6">
        <Header>
          <button
            type="button"
            onClick={() =>
              setDraft({
                title: metadata?.title ?? '',
                description: metadata?.description ?? '',
                tags: metadata?.tags.join(', ') ?? '',
              })
            }
            className="ml-auto text-[11px] text-[var(--accent)] hover:underline"
          >
            Edit
          </button>
          <Push video={video} />
        </Header>

        <Field label="Title">{metadata?.title}</Field>
        <Field label="Description">{metadata?.description}</Field>
        {/* Comma-separated, which is how a tag field is filled in — not one per
            line, which would make six tags look like six paragraphs. */}
        <Field label="Tags">{metadata?.tags.join(', ')}</Field>
      </div>
    )
  }

  const edit = (patch: Partial<Draft>) => setDraft({ ...draft, ...patch })

  return (
    <div className="flex flex-col gap-6">
      <Header>
        <div className="ml-auto flex items-center gap-2">
          <Button onClick={() => setDraft(undefined)} disabled={save.isPending}>
            Cancel
          </Button>
          <Button primary onClick={() => save.mutate(draft)} disabled={save.isPending}>
            {save.isPending ? 'Saving…' : 'Save'}
          </Button>
        </div>
      </Header>

      {/*
        The server's complaint, verbatim. The limits it enforces are YouTube's,
        and there is no copy of them on this side to check against first — a
        second set of numbers here would be a second thing to keep in step with
        an API neither of them owns.
      */}
      {save.error ? (
        <p className="text-[11px] leading-snug text-[var(--failed)]">{save.error.message}</p>
      ) : null}

      {/*
        Mono in the controls too. The point of this page is that the text on it
        is the text that publishes, and a description that reflowed the moment
        you clicked into it would break that in the one mode where it matters.
      */}
      <Editable label="Title">
        <Input
          value={draft.title}
          onChange={(event) => edit({ title: event.target.value })}
          className="font-mono text-[12px]"
        />
      </Editable>
      <Editable label="Description">
        <Textarea
          value={draft.description}
          onChange={(event) => edit({ description: event.target.value })}
          rows={12}
          className="font-mono text-[12px] leading-[1.6]"
        />
      </Editable>
      <Editable label="Tags" hint="Separated by commas.">
        <Input
          value={draft.tags}
          onChange={(event) => edit({ tags: event.target.value })}
          className="font-mono text-[12px]"
        />
      </Editable>
    </div>
  )
}

/** The section's name, and whatever acts on it. */
/**
 * Send the saved listing to a video that is already on YouTube.
 *
 * Only for a video that is actually up there: absent before the upload, and
 * absent for a dry run, whose receipt names no video to correct.
 *
 * Its own action rather than something Save does, because the two are different
 * statements. Saving records what this video should say; this rewrites what a
 * published one does say, and an edit made to fix a typo before the next render
 * should not reach into a video the world is already watching.
 *
 * The result is spoken rather than left to a toast: this is the one control on
 * the page whose effect is entirely off-screen, so a line saying what happened
 * is the only evidence there is.
 */
function Push({ video }: { video: Video }) {
  const published = video.upload && !video.upload.dryRun
  const push = useMutation({ mutationFn: () => api.pushMetadata(video.ref) })
  if (!published) return null

  return (
    <div className="ml-2 flex items-baseline gap-2">
      <button
        type="button"
        onClick={() => push.mutate()}
        disabled={push.isPending}
        className="text-[11px] text-[var(--accent)] hover:underline disabled:opacity-50"
      >
        {push.isPending ? 'Sending…' : 'Push to YouTube'}
      </button>
      {push.isError && (
        <span className="text-[11px] leading-snug text-[var(--failed)]">{push.error.message}</span>
      )}
      {push.isSuccess && <span className="text-[11px] text-tertiary">Sent</span>}
    </div>
  )
}

function Header({ children }: { children: ReactNode }) {
  return (
    <div className="flex items-baseline gap-2">
      <Caption>Listing</Caption>
      {children}
    </div>
  )
}

/**
 * A label, and the string under it.
 *
 * `pre` rather than `p`: this is the exact text, and the element that means
 * "exact text" is the one that does not collapse whitespace. `pre-wrap` because
 * a description is prose that happens to be preformatted — real `pre` would put
 * a horizontal scrollbar under a paragraph nobody could read.
 */
function Field({ label, children }: { label: string; children: string | undefined }) {
  return (
    <div className="flex flex-col gap-1.5">
      <Caption>{label}</Caption>
      {children ? (
        <pre className="font-mono text-[12px] leading-[1.6] whitespace-pre-wrap text-primary">
          {children}
        </pre>
      ) : (
        <span className="text-[12px] text-tertiary">—</span>
      )}
    </div>
  )
}

/**
 * The same row with a control in it.
 *
 * Deliberately the same shape as `Field` — label above, value below, full width
 * — rather than the dialog's right-aligned label column. A description is a
 * paragraph, and a paragraph in a form row indented past a label column is a
 * column of text half as wide as the thing it is meant to be a preview of.
 */
function Editable({
  label,
  hint,
  children,
}: {
  label: string
  hint?: string
  children: ReactNode
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <Caption>{label}</Caption>
      {children}
      {hint ? <p className="text-[11px] text-tertiary">{hint}</p> : null}
    </div>
  )
}

/**
 * The render, with the thumbnail on the front of it.
 *
 * `poster` is not decoration: an embedded player shows the thumbnail until
 * somebody presses play, so this is the thumbnail where a viewer meets it first.
 *
 * `preload="metadata"` because the file is a whole rendered video, and a page
 * that pulled it on sight would fetch a few hundred megabytes for something
 * nobody has pressed play on.
 */
function Player({
  video,
  playerRef,
  onTime,
  onRuntime,
}: {
  video: Video
  playerRef: RefObject<HTMLVideoElement | null>
  onTime: (seconds: number) => void
  onRuntime: (seconds: number) => void
}) {
  const poster = video.effectiveThumbnailAssetId
  const frame = 'aspect-video w-full overflow-hidden rounded-[10px]'

  if (video.finalAssetId) {
    return (
      <video
        ref={playerRef}
        controls
        preload="metadata"
        poster={poster ? `/assets/${poster}` : undefined}
        src={`/assets/${video.finalAssetId}`}
        // Both, because a seek from the rail has to move the playhead even
        // while the video is paused, and `timeupdate` only fires while it runs.
        onTimeUpdate={(event) => onTime(event.currentTarget.currentTime)}
        onSeeked={(event) => onTime(event.currentTarget.currentTime)}
        // `metadata` is enough to learn the duration, which is the whole reason
        // this is not `preload="none"`: the chapter list is timed in these
        // seconds.
        onLoadedMetadata={(event) => {
          const seconds = event.currentTarget.duration
          if (Number.isFinite(seconds) && seconds > 0) onRuntime(seconds)
        }}
        className={frame}
        style={{ backgroundColor: '#000' }}
      />
    )
  }

  if (poster) {
    return (
      <div className={frame} style={{ backgroundColor: '#000' }}>
        <img src={`/assets/${poster}`} alt="Thumbnail" className="size-full object-contain" />
      </div>
    )
  }

  return (
    <div
      className={`${frame} flex items-center justify-center border border-dashed`}
      style={{ borderColor: 'var(--separator-strong)' }}
    >
      <span className="text-[12px] text-tertiary">The cut has not been rendered yet.</span>
    </div>
  )
}

/**
 * The thumbnail on its own, and its real size.
 *
 * Measured off the loaded image rather than assumed. 1280×720 is what the
 * composer is meant to produce, and a page that printed that whether or not it
 * was true would be hiding exactly the bug worth catching.
 */
/**
 * Send this thumbnail to a video that is already on YouTube.
 *
 * Only for a video actually up there: absent before the upload, and absent for
 * a dry run, whose receipt names no video to re-front. Absent too with no image
 * to send, which is the same case as the Build one beside it.
 *
 * Its own action rather than something the builder does on save, because the
 * two are different statements: the builder composes the image this video
 * should publish with, and this rewrites what a published one does show.
 *
 * The note about caching is not decoration. The call returns as soon as YouTube
 * has the image, but what viewers see is served from a cache that takes its own
 * few minutes -- so "Sent" followed by an unchanged thumbnail reads as a
 * failure, and this is the only place that can say otherwise.
 */
function PublishThumbnail({ video }: { video: Video }) {
  const push = useMutation({ mutationFn: () => api.pushThumbnail(video.ref) })
  if (!video.effectiveThumbnailAssetId) return null
  if (!video.upload || video.upload.dryRun) return null

  return (
    <span className="flex items-baseline gap-2">
      {push.isError && (
        <span className="text-[11px] leading-snug text-[var(--failed)]">{push.error.message}</span>
      )}
      {push.isSuccess && (
        <span className="text-[11px] text-tertiary">Sent — takes a few minutes to appear</span>
      )}
      <button
        type="button"
        onClick={() => push.mutate()}
        disabled={push.isPending}
        className="text-[11px] text-[var(--accent)] hover:underline disabled:opacity-50"
      >
        {push.isPending ? 'Publishing…' : 'Publish'}
      </button>
    </span>
  )
}

function Thumbnail({ video, onBuild }: { video: Video; onBuild: () => void }) {
  const id = video.effectiveThumbnailAssetId
  const [size, setSize] = useState<string>()

  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex items-baseline gap-2">
        <Caption>Thumbnail</Caption>
        {size ? <span className="text-[11px] tabular-nums text-tertiary">{size}</span> : null}
        <div className="ml-auto flex items-baseline gap-3">
          <PublishThumbnail video={video} />
          <button
            type="button"
            onClick={onBuild}
            className="text-[11px] text-[var(--accent)] hover:underline"
          >
            {id ? 'Edit' : 'Build one'}
          </button>
        </div>
      </div>

      {id ? (
        <img
          src={`/assets/${id}`}
          alt="Thumbnail"
          onLoad={(event) =>
            setSize(`${event.currentTarget.naturalWidth}×${event.currentTarget.naturalHeight}`)
          }
          className="aspect-video w-full rounded-[10px] object-cover"
          style={{ boxShadow: '0 0 0 0.5px var(--separator-strong)' }}
        />
      ) : (
        <div
          className="flex aspect-video w-full items-center justify-center rounded-[10px] border border-dashed"
          style={{ borderColor: 'var(--separator-strong)' }}
        >
          <span className="text-[12px] text-tertiary">No thumbnail yet.</span>
        </div>
      )}
    </div>
  )
}
