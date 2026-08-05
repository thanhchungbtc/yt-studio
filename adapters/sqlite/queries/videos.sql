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
    id, channel_id, ref, title, topic, state, chapter_count, slides_per_chapter,
    target_duration_minutes, thumbnail_cells, blueprint_asset_id, final_asset_id,
    thumbnail_asset_id, thumbnail_plan_json, thumbnail_icon_ids_json,
    metadata_json, upload_json, error, created_at, updated_at, started_at, completed_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateVideo :exec
UPDATE videos
SET title = ?, topic = ?, state = ?, chapter_count = ?, slides_per_chapter = ?,
    target_duration_minutes = ?, thumbnail_cells = ?,
    blueprint_asset_id = ?, final_asset_id = ?, thumbnail_asset_id = ?,
    thumbnail_plan_json = ?, thumbnail_icon_ids_json = ?,
    metadata_json = ?, upload_json = ?, error = ?, updated_at = ?,
    started_at = ?, completed_at = ?
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

-- name: SetVideoThumbnailAsset :exec
UPDATE videos SET thumbnail_asset_id = ?, updated_at = ? WHERE id = ?;

-- name: SetVideoThumbnailPlan :exec
-- The plan and the slots its icons will land in are written together: a plan
-- with no slots, or slots sized for a plan that was replaced, would put an icon
-- in the wrong cell.
UPDATE videos
SET thumbnail_plan_json = ?, thumbnail_icon_ids_json = ?, updated_at = ?
WHERE id = ?;

-- name: SetVideoThumbnailIcon :exec
-- One icon at its index. json_set makes this a single atomic statement, so two
-- concurrent icon tasks cannot lose each other's write.
UPDATE videos
SET thumbnail_icon_ids_json = json_set(thumbnail_icon_ids_json, CAST(sqlc.arg(path) AS TEXT), CAST(sqlc.arg(asset_id) AS TEXT)),
    updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: SetVideoThumbnailCellPrompt :exec
-- One cell's prompt, for an operator redrawing a single icon. Indexed for the
-- same reason as the icon above, and scoped to the prompt so the caption the
-- plan wrote for this cell survives an edit to what it pictures.
UPDATE videos
SET thumbnail_plan_json = json_set(thumbnail_plan_json, CAST(sqlc.arg(path) AS TEXT), CAST(sqlc.arg(prompt) AS TEXT)),
    updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: SetVideoMetadata :exec
UPDATE videos SET metadata_json = ?, updated_at = ? WHERE id = ?;

-- name: SetVideoUpload :exec
UPDATE videos SET upload_json = ?, updated_at = ? WHERE id = ?;
