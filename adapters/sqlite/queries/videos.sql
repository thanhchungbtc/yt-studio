-- name: GetVideoByID :one
SELECT * FROM videos WHERE id = ?;

-- name: GetVideoByRef :one
SELECT * FROM videos WHERE ref = ?;

-- name: ListVideos :many
SELECT * FROM videos
WHERE (CAST(sqlc.arg(channel_id) AS TEXT) = '' OR channel_id = sqlc.arg(channel_id))
  AND (CAST(sqlc.arg(state) AS TEXT) = '' OR state = sqlc.arg(state))
ORDER BY created_at DESC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountVideos :one
SELECT COUNT(*) FROM videos
WHERE (CAST(sqlc.arg(channel_id) AS TEXT) = '' OR channel_id = sqlc.arg(channel_id))
  AND (CAST(sqlc.arg(state) AS TEXT) = '' OR state = sqlc.arg(state));

-- name: CreateVideo :exec
INSERT INTO videos (
    id, channel_id, ref, title, topic, state, chapter_count, images_per_chapter,
    blueprint_asset_id, final_asset_id, metadata_json, upload_json, error,
    created_at, updated_at, started_at, completed_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateVideo :exec
UPDATE videos
SET title = ?, topic = ?, state = ?, chapter_count = ?, images_per_chapter = ?,
    blueprint_asset_id = ?, final_asset_id = ?, metadata_json = ?,
    upload_json = ?, error = ?, updated_at = ?, started_at = ?, completed_at = ?
WHERE id = ?;

-- name: SetVideoState :exec
UPDATE videos
SET state = ?,
    error = ?,
    updated_at = ?,
    started_at = COALESCE(started_at, ?),
    completed_at = ?
WHERE id = ?;

-- name: DeleteVideo :exec
DELETE FROM videos WHERE id = ?;

-- name: SetVideoBlueprintAsset :exec
UPDATE videos SET blueprint_asset_id = ?, updated_at = ? WHERE id = ?;

-- name: SetVideoFinalAsset :exec
UPDATE videos SET final_asset_id = ?, updated_at = ? WHERE id = ?;

-- name: SetVideoMetadata :exec
UPDATE videos SET metadata_json = ?, updated_at = ? WHERE id = ?;

-- name: SetVideoUpload :exec
UPDATE videos SET upload_json = ?, updated_at = ? WHERE id = ?;
