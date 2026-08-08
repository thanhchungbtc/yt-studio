import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { BookOpen, Command, Film, Plus, Tv } from 'lucide-react'

import { PoolMeter } from '@/components/pool-meter'
import { VideoStateDot } from '@/components/state-badges'
import { Button } from '@/components/ui/button'
import {
  Kbd,
  Panel,
  PanelHeader,
  PanelTitle,
  Ring,
  Skeleton,
  Toolbar,
} from '@/components/ui/primitives'
import { api, qk } from '@/core/api'
import { useAppCommands } from '@/core/app-commands'
import { formatRelative, videoStateLabel } from '@/core/format'

/**
 * What fills the detail pane before a video is chosen.
 *
 * A blank pane would be honest but useless, so this is the application's start
 * page: where the server's capacity is going right now, what was touched most
 * recently, and the three keystrokes worth learning.
 */
export function VideosStartRoute() {
  const { openCreateVideo, openPalette } = useAppCommands()

  const videos = useQuery({ queryKey: qk.videos({}), queryFn: () => api.listVideos({}) })
  const scheduler = useQuery({
    queryKey: qk.scheduler,
    queryFn: api.schedulerStatus,
    refetchInterval: 30_000,
  })

  const recent = [...(videos.data?.videos ?? [])]
    .sort((a, b) => b.updatedAt.localeCompare(a.updatedAt))
    .slice(0, 7)

  return (
    <>
      <Toolbar>
        <span className="flex items-center gap-1.5 text-[12.5px] text-muted no-select">
          <Film className="h-3.5 w-3.5" aria-hidden />
          Videos
          <span className="text-subtle">/</span>
          <span className="text-fg">Start</span>
        </span>
      </Toolbar>

      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto max-w-3xl px-6 py-10">
          <header className="mb-8">
            <div className="mb-3 flex h-10 w-10 items-center justify-center rounded-[var(--radius-md)] bg-[hsl(var(--accent))] text-[hsl(var(--accent-fg))] elev-2">
              <svg viewBox="0 0 24 24" className="h-5 w-5" fill="currentColor" aria-hidden>
                <path d="M9 7v10l8-5z" />
              </svg>
            </div>
            <h1 className="text-[20px] font-semibold tracking-[-0.01em] text-fg">yt-studio</h1>
            <p className="mt-1 max-w-lg text-[13px] text-muted">
              Pick a video from the sidebar to open it, or start a new one. Every render is a DAG of
              a few hundred tasks; the scheduler dispatches each the moment its dependencies are
              met.
            </p>

            <div className="mt-4 flex flex-wrap items-center gap-2">
              <Button variant="primary" onClick={() => openCreateVideo()}>
                <Plus className="h-3.5 w-3.5" />
                New video
              </Button>
              <Button variant="outline" onClick={openPalette}>
                <Command className="h-3.5 w-3.5" />
                Command palette
                <Kbd keys="mod+k" className="ml-1" />
              </Button>
              <Button variant="ghost" asChild>
                <a href="/api/docs" target="_blank" rel="noreferrer">
                  <BookOpen className="h-3.5 w-3.5" />
                  API docs
                </a>
              </Button>
            </div>
          </header>

          <div className="grid items-start gap-4 md:grid-cols-[minmax(0,1fr)_260px]">
            <Panel>
              <PanelHeader>
                <PanelTitle>Recently updated</PanelTitle>
                <Link to="/channels" className="text-[11px] text-subtle hover:text-fg">
                  <span className="flex items-center gap-1">
                    <Tv className="h-3 w-3" />
                    Channels
                  </span>
                </Link>
              </PanelHeader>

              {videos.isPending && <Skeleton className="m-3 h-48" />}

              {!videos.isPending && recent.length === 0 && (
                <p className="px-3 py-8 text-center text-[12px] text-muted">
                  Nothing yet. Create a video and its chapters will appear here as they are
                  generated.
                </p>
              )}

              <ul className="divide-y divide-[hsl(var(--border))]">
                {recent.map((video) => (
                  <li key={video.id}>
                    <Link
                      to="/videos/$ref"
                      params={{ ref: video.ref }}
                      className="flex items-center gap-3 px-3 py-2 transition-colors hover:bg-[hsl(var(--bg-hover))]"
                    >
                      <Ring
                        value={video.counts.succeeded}
                        total={video.counts.total}
                        failed={video.counts.failed}
                        size={18}
                      />
                      <span className="min-w-0 flex-1">
                        <span className="flex items-baseline gap-2">
                          <span className="font-mono text-[11px] font-semibold text-[hsl(var(--accent))]">
                            {video.ref}
                          </span>
                          <span className="truncate text-[12.5px] text-fg">{video.title}</span>
                        </span>
                        <span className="mt-0.5 flex items-center gap-1.5 text-[11px] text-subtle">
                          <VideoStateDot state={video.state} className="h-[5px] w-[5px]" />
                          {videoStateLabel(video.state)}
                          <span aria-hidden>·</span>
                          <span className="tabular">
                            {video.counts.succeeded}/{video.counts.total} tasks
                          </span>
                        </span>
                      </span>
                      <span className="shrink-0 text-[11px] tabular text-subtle">
                        {formatRelative(video.updatedAt)}
                      </span>
                    </Link>
                  </li>
                ))}
              </ul>
            </Panel>

            <div className="space-y-4">
              <Panel>
                <PanelHeader>
                  <PanelTitle>Capacity</PanelTitle>
                  <Link to="/scheduler" className="text-[11px] text-subtle hover:text-fg">
                    Console
                  </Link>
                </PanelHeader>
                <div className="space-y-2 px-3 py-3">
                  {scheduler.isPending && <Skeleton className="h-24" />}
                  {scheduler.data?.pools.map((pool) => (
                    <PoolMeter key={pool.pool} stat={pool} compact />
                  ))}
                </div>
              </Panel>

              <Panel>
                <PanelHeader>
                  <PanelTitle>Worth learning</PanelTitle>
                </PanelHeader>
                <dl className="px-3 py-2">
                  {[
                    { keys: 'mod+k', label: 'Jump to any video' },
                    { keys: 'alt+arrowdown', label: 'Next video' },
                    { keys: 'mod+b', label: 'Hide the sidebar' },
                    { keys: 'shift+?', label: 'All shortcuts' },
                  ].map((item) => (
                    <div key={item.keys} className="flex items-center justify-between gap-3 py-1">
                      <dt className="text-[11.5px] text-muted">{item.label}</dt>
                      <dd>
                        <Kbd keys={item.keys} />
                      </dd>
                    </div>
                  ))}
                </dl>
              </Panel>
            </div>
          </div>
        </div>
      </div>
    </>
  )
}
