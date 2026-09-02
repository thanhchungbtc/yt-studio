import { useState, type RefObject } from 'react'

import type { Video } from '../../../../core/types'
import { openDoc } from '../../dock'
import { Caption } from './caption'

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
 * Read-only throughout. The metadata is the model's, and editing it is a
 * separate step with a backend behind it that does not exist yet.
 */
export function Listing({
  video,
  playerRef,
  onTime,
  onRuntime,
}: {
  video: Video
  playerRef: RefObject<HTMLVideoElement | null>
  onTime: (seconds: number) => void
  /** How long the cut turned out to be; the chapter list uses these units. */
  onRuntime: (seconds: number) => void
}) {
  const metadata = video.metadata

  return (
    <div className="flex flex-col gap-6 px-6 py-5">
      <Player video={video} playerRef={playerRef} onTime={onTime} onRuntime={onRuntime} />

      <Field label="Title">{metadata?.title}</Field>
      <Field label="Description">{metadata?.description}</Field>
      {/* Comma-separated, which is how a tag field is filled in — not one per
          line, which would make six tags look like six paragraphs. */}
      <Field label="Tags">{metadata?.tags.join(', ')}</Field>

      <Thumbnail video={video} />
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
function Thumbnail({ video }: { video: Video }) {
  const id = video.effectiveThumbnailAssetId
  const [size, setSize] = useState<string>()

  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex items-baseline gap-2">
        <Caption>Thumbnail</Caption>
        {size ? <span className="text-[11px] tabular-nums text-tertiary">{size}</span> : null}
        {id ? (
          <button
            type="button"
            onClick={() =>
              openDoc(
                { kind: 'thumbnail', ref: video.ref },
                `${video.title || video.ref} — thumbnail`,
              )
            }
            className="ml-auto text-[11px] text-[var(--accent)] hover:underline"
          >
            Open in builder
          </button>
        ) : null}
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
