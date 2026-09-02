import { Youtube } from 'lucide-react'
import { useRef, useState } from 'react'

import { Placeholder } from '../../placeholder'
import { ThumbnailDialog } from '../../../thumbnail/dialog'
import type { ViewProps } from '../view'
import { FinalStrip } from './final'
import { Listing } from './listing'
import { UploadStatus } from './progress'
import { ChapterList } from './chapters'

/**
 * The video as the thing that gets published.
 *
 * The other two modes are about chapters. This one is about the four stages that
 * belong to the video itself — the cut, the listing, the thumbnail, the upload —
 * and it answers one question: what is about to be sent.
 *
 * Two panes. The left is everything that is being published, in the order it
 * matters: the render, then the three strings, then the picture. The right is
 * the chapter list, which is the one thing on this screen that is a control
 * rather than a reading.
 *
 * Read-only throughout, and not a mock of anything. The gate is not here either,
 * and does not need to be: the approval strip renders above every mode, so
 * switching to Upload while the upload gate is open puts the question at the top
 * of the page that answers it — which was the point of building this.
 */
export function UploadView({ video, chapters, tasks }: ViewProps) {
  // The player is owned here because the list seeks it and the list is in the
  // other pane. One element, one position, and the two panes are its two ends.
  const player = useRef<HTMLVideoElement>(null)
  const [seconds, setSeconds] = useState(0)
  // Undefined until the player has read the file's header. The list falls back
  // to the blueprint's planned lengths until then; see the note in `chapters`.
  const [runtime, setRuntime] = useState<number>()
  // The builder is a dialog over this page, and it is opened from two places on
  // it, so the flag lives here rather than in either of them.
  const [building, setBuilding] = useState(false)

  // Before the cut, the listing and the thumbnail exist there is nothing on this
  // page but the shape of what is missing — three em dashes and two empty
  // rectangles. The strip above still says which of the four are done.
  const anything = Boolean(
    video.finalAssetId || video.effectiveThumbnailAssetId || video.metadata?.title,
  )

  return (
    <>
      <FinalStrip video={video} tasks={tasks} onBuild={() => setBuilding(true)} />
      <UploadStatus video={video} tasks={tasks} />

      {anything ? (
        <div className="flex min-h-0 flex-1">
          <div className="min-h-0 flex-1 overflow-y-auto">
            <Listing
              video={video}
              playerRef={player}
              onTime={setSeconds}
              onRuntime={setRuntime}
              onBuild={() => setBuilding(true)}
            />
          </div>
          {/* Fixed. The list is a fixed amount of information — an ordinal, a
              title and a time per chapter — so width past what that needs would
              be taken from the thing being published. */}
          <div className="hairline-l min-h-0 w-[276px] shrink-0 overflow-y-auto">
            <ChapterList
              chapters={chapters}
              seconds={seconds}
              runtime={runtime}
              seekable={Boolean(video.finalAssetId)}
              onSeek={(at) => {
                const element = player.current
                if (!element) return
                element.currentTime = at
                // Deliberately not `play()`. Jumping to a chapter while paused
                // should leave it paused on that frame; a click that started
                // playback would make the list unusable for looking.
                setSeconds(at)
              }}
            />
          </div>
        </div>
      ) : (
        <div className="min-h-0 flex-1">
          <Placeholder
            icon={Youtube}
            title="Nothing to publish yet"
            detail="The cut, the listing and the thumbnail are the last three things the pipeline makes."
          />
        </div>
      )}

      <ThumbnailDialog video={video} open={building} onOpenChange={setBuilding} />
    </>
  )
}
