/**
 * The slice of the API v2 actually reads.
 *
 * V2 is self-contained by rule, so it carries its own view of the wire format
 * rather than importing v1's. It is a narrowing, not a fork: every field here
 * is a field the server already sends, and anything v2 does not render is left
 * out until a screen needs it.
 */

export type VideoState =
  'draft' | 'running' | 'awaiting_approval' | 'blocked' | 'completed' | 'failed' | 'cancelled'

export interface Channel {
  id: string
  slug: string
  name: string
  description: string
  credentials: 'missing' | 'valid' | 'expired'
  updatedAt: string
}

export interface Video {
  id: string
  channelId: string
  /** The stable human-facing key — `channel-slug/0007` — used in every route. */
  ref: string
  title: string
  topic: string
  state: VideoState
  updatedAt: string
}
