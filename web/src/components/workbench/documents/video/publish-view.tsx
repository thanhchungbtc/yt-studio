import { Download, ExternalLink, ImageOff } from 'lucide-react'

import { Badge, Button } from '../../ui/controls'
import { EmptyState, KeyValue, Section } from '../../ui/primitives'
import { assetUrl } from '@/core/api'
import { formatAbsolute } from '@/core/format'
import type { Video } from '@/core/types'

/**
 * What the video becomes: the thumbnail, the listing, the render, the receipt.
 *
 * These are the only things a chapter table has no row for — they belong to the
 * video rather than to any chapter — which is the whole reason this is a second
 * view rather than a block bolted underneath forty rows.
 */
export function PublishView({ video }: { video: Video }) {
  const nothingYet = !video.metadata && !video.finalAssetId && !video.effectiveThumbnailAssetId

  if (nothingYet) {
    return (
      <EmptyState
        icon={<ImageOff />}
        title="Nothing to publish yet"
        description="The thumbnail, the listing and the final render appear here once the pipeline reaches them."
      />
    )
  }

  return (
    <div className="h-full overflow-y-auto p-4">
      <div className="mx-auto max-w-3xl space-y-5">
        <Section title="Listing">
          <div className="surface flex items-start gap-3 p-3">
            {video.effectiveThumbnailAssetId ? (
              <img
                src={assetUrl(video.effectiveThumbnailAssetId)}
                alt={`Thumbnail for ${video.title}`}
                className="aspect-video w-48 shrink-0 rounded-[var(--radius-sm)] bg-black object-cover"
              />
            ) : (
              <div className="checker flex aspect-video w-48 shrink-0 items-center justify-center rounded-[var(--radius-sm)] text-subtle">
                <ImageOff className="h-5 w-5" />
              </div>
            )}

            <div className="min-w-0 flex-1 space-y-1.5">
              <p className="text-[13px] font-medium text-fg">
                {video.metadata?.title ?? video.title}
              </p>
              {video.metadata?.description && (
                <p className="line-clamp-4 whitespace-pre-wrap text-[11.5px] leading-relaxed text-muted">
                  {video.metadata.description}
                </p>
              )}
              <div className="flex flex-wrap gap-1 pt-0.5">
                {video.metadata && <Badge tone="neutral">{video.metadata.privacy}</Badge>}
                {video.metadata?.tags.map((tag) => (
                  <Badge key={tag} tone="neutral">
                    {tag}
                  </Badge>
                ))}
              </div>
              {/* The hand-built thumbnail wins over the rendered one when it
                  exists, and saying so is the only way to explain why the
                  picture above is not the one the pipeline drew. */}
              {video.thumbnailOverrideAssetId && (
                <p className="pt-0.5 text-[11px] text-subtle">
                  Using a thumbnail built by hand; the rendered one is kept.
                </p>
              )}
            </div>
          </div>
        </Section>

        {video.finalAssetId && (
          <Section
            title="Final render"
            actions={
              <Button size="xs" variant="ghost" asChild>
                <a href={assetUrl(video.finalAssetId)} download>
                  <Download className="h-3 w-3" />
                  Download
                </a>
              </Button>
            }
          >
            <video
              controls
              preload="metadata"
              src={assetUrl(video.finalAssetId)}
              className="max-h-[50vh] w-full rounded-[var(--radius-md)] bg-black"
            />
          </Section>
        )}

        {video.upload && (
          <Section title="Upload">
            <dl className="surface divide-y divide-[hsl(var(--border))] px-3">
              <KeyValue label="Uploaded">{formatAbsolute(video.upload.uploadedAt)}</KeyValue>
              <KeyValue label="Remote id">{video.upload.remoteVideoId || '—'}</KeyValue>
              <KeyValue label="Dry run">{video.upload.dryRun ? 'yes' : 'no'}</KeyValue>
            </dl>
            {video.upload.url && (
              <a
                href={video.upload.url}
                target="_blank"
                rel="noreferrer"
                className="inline-flex items-center gap-1 text-[11.5px] text-[hsl(var(--accent))] hover:underline"
              >
                Open on YouTube
                <ExternalLink className="h-3 w-3" />
              </a>
            )}
          </Section>
        )}
      </div>
    </div>
  )
}
