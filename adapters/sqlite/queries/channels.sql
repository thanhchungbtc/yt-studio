-- name: GetChannelByID :one
SELECT * FROM channels WHERE id = ?;

-- name: GetChannelBySlug :one
SELECT * FROM channels WHERE slug = ?;

-- name: ListChannels :many
SELECT * FROM channels ORDER BY name;

-- name: CreateChannel :exec
INSERT INTO channels (
    id, slug, name, description, tone, voice, image_style, language,
    words_per_chapter, words_per_minute, credentials, video_seq, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateChannel :exec
UPDATE channels
SET name = ?, description = ?, tone = ?, voice = ?, image_style = ?,
    language = ?, words_per_chapter = ?, words_per_minute = ?, credentials = ?, updated_at = ?
WHERE id = ?;

-- name: DeleteChannel :exec
DELETE FROM channels WHERE id = ?;

-- Seeds are upserts by natural key, so running the seed a second time updates
-- in place instead of creating a duplicate.
-- name: UpsertChannelBySlug :exec
INSERT INTO channels (
    id, slug, name, description, tone, voice, image_style, language,
    words_per_chapter, words_per_minute, credentials, video_seq, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (slug) DO UPDATE SET
    name = excluded.name,
    description = excluded.description,
    tone = excluded.tone,
    voice = excluded.voice,
    image_style = excluded.image_style,
    language = excluded.language,
    words_per_chapter = excluded.words_per_chapter,
    words_per_minute = excluded.words_per_minute,
    updated_at = excluded.updated_at;
