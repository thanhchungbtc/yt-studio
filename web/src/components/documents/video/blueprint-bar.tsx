import type { ReactNode } from 'react'
import { ChevronDown, ChevronRight, ExternalLink } from 'lucide-react'

import { Tooltip } from '../../ui/primitives'
import { assetUrl } from '@/core/api'
import type { Video } from '@/core/types'

/**
 * The plan, above the table that fulfils it.
 *
 * Everything here is either a field the blueprint wrote or a count of them.
 * Nothing is derived: an estimated runtime would need a words-per-minute rate we
 * would have to invent, and a number that looks measured but is guessed is worse
 * than no number.
 *
 * `2 slides each` lives here rather than in forty rows because it is identical
 * on every one of them — per-row space is for what varies per row.
 */
export function BlueprintBar({
  video,
  chapters,
  estimatedWords,
  collapsed,
  onToggle,
}: {
  video: Video
  chapters: number
  estimatedWords: number
  collapsed: boolean
  onToggle: () => void
}) {
  return (
    <div className="shrink-0 border-b border-[hsl(var(--border))] bg-subtle px-3 py-1.5 no-select">
      <div className="flex items-start gap-2">
        <button
          type="button"
          onClick={onToggle}
          aria-expanded={!collapsed}
          aria-label={collapsed ? 'Show the blueprint' : 'Hide the blueprint'}
          className="mt-[1px] flex h-4 w-4 shrink-0 items-center justify-center rounded-[var(--radius-xs)] text-subtle hover:bg-[hsl(var(--bg-hover))] hover:text-fg"
        >
          {collapsed ? <ChevronRight className="h-3 w-3" /> : <ChevronDown className="h-3 w-3" />}
        </button>

        <div className="min-w-0 flex-1">
          {!collapsed && video.topic && (
            <p className="text-[11.5px] leading-snug text-muted">{video.topic}</p>
          )}
          <p className="tabular text-[11px] text-subtle">
            <Fact>{chapters}</Fact> chapters
            {estimatedWords > 0 && (
              <>
                {' · '}
                <Fact>{estimatedWords.toLocaleString()}</Fact> words
              </>
            )}
            {video.targetDurationMinutes > 0 && (
              <>
                {' · '}
                <Fact>{video.targetDurationMinutes}m</Fact> target
              </>
            )}
            {video.slidesPerChapter > 0 && (
              <>
                {' · '}
                <Fact>{video.slidesPerChapter}</Fact> slides each
              </>
            )}
          </p>
        </div>

        {video.blueprintAssetId && (
          <Tooltip label="Open the raw blueprint">
            <a
              href={assetUrl(video.blueprintAssetId)}
              target="_blank"
              rel="noreferrer"
              className="flex h-5 shrink-0 items-center gap-1 rounded-[var(--radius-xs)] px-1.5 text-[11px] text-subtle transition-colors hover:bg-[hsl(var(--bg-hover))] hover:text-fg"
            >
              raw
              <ExternalLink className="h-3 w-3" />
            </a>
          </Tooltip>
        )}
      </div>
    </div>
  )
}

function Fact({ children }: { children: ReactNode }) {
  return <span className="font-medium text-muted">{children}</span>
}
