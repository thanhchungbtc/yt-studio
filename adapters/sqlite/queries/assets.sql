-- name: GetAssetByID :one
-- Serving is by content address alone. Every owner's row carries the same path,
-- size and mime -- all three are functions of the address and the kind -- so any
-- one row answers the question.
SELECT * FROM assets WHERE id = ? LIMIT 1;

-- name: ListAssetsByVideo :many
SELECT * FROM assets WHERE video_id = ? ORDER BY created_at;

-- name: PutAsset :exec
-- One row per video per content address. The file is still stored once: two
-- videos that produce identical bytes share it and each records its own
-- ownership, which is what lets a delete tell "nobody else needs this file" from
-- "another video still does".
INSERT INTO assets (id, video_id, chapter_id, kind, path, size, mime, provenance, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id, video_id) DO NOTHING;

-- name: CountAssetOwners :one
-- The whole reference count. Run it once a video's rows are gone: zero owners
-- means the file is unreachable and safe to unlink.
SELECT COUNT(*) AS owners FROM assets WHERE id = ?;

-- name: ListAssetAddresses :many
-- Every address the database still knows about, for the sweep to compare the
-- store against.
SELECT DISTINCT id, kind FROM assets ORDER BY id;

-- name: ListMissingAssetOwners :many
-- Ownership edges present in the data but missing from the assets table.
--
-- Before the (id, video_id) key, a video that reused another video's bytes got
-- no row of its own, so the only surviving record of that reference is the id
-- sitting on the video or on its chapters. This finds those. It normally returns
-- nothing: the anti-join is what makes the one-time repair a no-op once it has
-- run, cheap enough to attempt at every startup.
WITH refs AS (
    SELECT id AS video_id, CAST(NULL AS TEXT) AS chapter_id,
           blueprint_asset_id AS asset_id, CAST('blueprint' AS TEXT) AS kind
    FROM videos WHERE blueprint_asset_id IS NOT NULL AND blueprint_asset_id <> ''
    UNION ALL
    SELECT id AS video_id, CAST(NULL AS TEXT) AS chapter_id,
           final_asset_id AS asset_id, CAST('final' AS TEXT) AS kind
    FROM videos WHERE final_asset_id IS NOT NULL AND final_asset_id <> ''
    UNION ALL
    SELECT id AS video_id, CAST(NULL AS TEXT) AS chapter_id,
           thumbnail_asset_id AS asset_id, CAST('thumbnail' AS TEXT) AS kind
    FROM videos WHERE thumbnail_asset_id IS NOT NULL AND thumbnail_asset_id <> ''
    UNION ALL
    SELECT video_id, id AS chapter_id,
           audio_asset_id AS asset_id, CAST('audio' AS TEXT) AS kind
    FROM chapters WHERE audio_asset_id IS NOT NULL AND audio_asset_id <> ''
    UNION ALL
    SELECT video_id, id AS chapter_id,
           clip_asset_id AS asset_id, CAST('clip' AS TEXT) AS kind
    FROM chapters WHERE clip_asset_id IS NOT NULL AND clip_asset_id <> ''
    UNION ALL
    SELECT c.video_id AS video_id, c.id AS chapter_id,
           CAST(j.value AS TEXT) AS asset_id, CAST('image' AS TEXT) AS kind
    FROM chapters c, json_each(c.image_asset_ids_json) j
    WHERE j.value IS NOT NULL AND j.value <> ''
)
SELECT DISTINCT video_id, chapter_id, asset_id, kind
FROM refs
WHERE NOT EXISTS (
    SELECT 1 FROM assets a WHERE a.id = asset_id AND a.video_id = refs.video_id
);
