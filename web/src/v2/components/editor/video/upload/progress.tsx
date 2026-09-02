import { LoaderCircle } from 'lucide-react'

import { listTimestamp } from '../../../../core/format'
import type { Task, Video } from '../../../../core/types'

/**
 * Where the publish got to, when that is a thing worth a line.
 *
 * Absent otherwise, on the same rule the stopped strip follows: a video whose
 * upload has not been reached yet has nothing to say here, and the four marks
 * above already say it has not been reached. Two cases are left — it is sending
 * now, and it has landed — and they are the two the page cannot show any other
 * way, because neither is visible in the listing underneath.
 *
 * Failures are not here either. A failed upload puts the reason in the strip at
 * the top of the editor, in the same words it uses for every other stage, and a
 * second copy on this page would be the document disagreeing with itself about
 * how many things went wrong.
 */
export function UploadStatus({ video, tasks }: { video: Video; tasks: Task[] }) {
  const record = video.upload
  const task = tasks.find((candidate) => candidate.kind === 'upload')

  if (task?.state === 'running') {
    return (
      <div className="hairline-b flex shrink-0 items-center gap-3 px-4 py-2">
        <LoaderCircle
          className="size-3.5 shrink-0 animate-spin"
          strokeWidth={2}
          style={{ color: 'var(--running)' }}
        />
        <span className="shrink-0 text-[12px] text-secondary">Uploading</span>
        <Bar percent={task.percent} />
        <span className="w-9 shrink-0 text-right text-[12px] tabular-nums text-tertiary">
          {task.percent === undefined ? '—' : `${task.percent}%`}
        </span>
      </div>
    )
  }

  if (!record) return null

  return (
    <div
      className="hairline-b flex shrink-0 items-center gap-2 px-4 py-2"
      style={{ backgroundColor: record.dryRun ? 'transparent' : 'var(--accent-wash)' }}
    >
      <span
        className="size-2 shrink-0 rounded-full"
        style={{ backgroundColor: record.dryRun ? 'var(--text-tertiary)' : 'var(--done)' }}
      />
      <span className="shrink-0 text-[12px] font-semibold text-primary">
        {record.dryRun ? 'Dry run' : 'Published'}
      </span>
      {/*
        A dry run's receipt carries a youtube.com URL that nothing was ever
        published to, so it is printed and not linked. Making it clickable would
        be the page asserting a video exists at an address where none does — the
        one claim this screen must never make. Only a real publish gets a link.
      */}
      {record.dryRun ? (
        <span className="min-w-0 flex-1 truncate font-mono text-[11px] text-tertiary">
          {record.url} · nothing was sent to YouTube
        </span>
      ) : (
        <a
          href={record.url}
          target="_blank"
          rel="noreferrer"
          className="min-w-0 flex-1 truncate text-[12px] text-[var(--accent)] hover:underline"
        >
          {record.url}
        </a>
      )}
      <span className="shrink-0 text-[12px] text-tertiary">{listTimestamp(record.uploadedAt)}</span>
    </div>
  )
}

/**
 * The bar, and the one case it refuses to guess at.
 *
 * A backend that cannot count what it has sent reports no percentage at all,
 * and a bar drawn empty would read as nought per cent rather than as unknown.
 * So there is a track and nothing in it, which is the honest shape.
 */
function Bar({ percent }: { percent: number | undefined }) {
  return (
    <span
      className="h-1 min-w-0 flex-1 overflow-hidden rounded-full"
      style={{ backgroundColor: 'var(--idle-selection)' }}
    >
      {percent === undefined ? null : (
        <span
          className="block h-full rounded-full transition-[width] duration-300"
          style={{
            width: `${Math.min(100, Math.max(0, percent))}%`,
            backgroundColor: 'var(--accent)',
          }}
        />
      )}
    </span>
  )
}
