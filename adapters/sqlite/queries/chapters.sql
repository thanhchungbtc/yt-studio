-- name: GetChapterByID :one
SELECT * FROM chapters WHERE id = ?;

-- name: ListChaptersByVideo :many
SELECT * FROM chapters WHERE video_id = ? ORDER BY ordinal;

-- name: DeleteChaptersByVideo :exec
DELETE FROM chapters WHERE video_id = ?;

-- name: UpsertChapter :exec
INSERT INTO chapters (
    id, video_id, ordinal, title, summary, script, slide_prompts_json,
    audio_asset_id, slide_asset_ids_json, clip_asset_id, duration_seconds,
    estimated_words, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    ordinal = excluded.ordinal,
    title = excluded.title,
    summary = excluded.summary,
    script = excluded.script,
    slide_prompts_json = excluded.slide_prompts_json,
    audio_asset_id = excluded.audio_asset_id,
    slide_asset_ids_json = excluded.slide_asset_ids_json,
    clip_asset_id = excluded.clip_asset_id,
    duration_seconds = excluded.duration_seconds,
    estimated_words = excluded.estimated_words,
    updated_at = excluded.updated_at;

-- Field-scoped updates. Two slide tasks for the same chapter run concurrently,
-- so a read-modify-write of the whole row would lose one of them; each of these
-- is a single atomic statement instead.
-- name: SetChapterScript :exec
UPDATE chapters SET script = ?, duration_seconds = ?, updated_at = ? WHERE id = ?;

-- The three fields the blueprint plans with, written together because they are
-- edited together: re-budgeting a chapter's words usually means rewriting the
-- summary that earned them. Everything a task produced is left alone -- this
-- statement touches the plan and nothing derived from it.
-- name: SetChapterPlan :exec
UPDATE chapters
SET title = ?, summary = ?, estimated_words = ?, updated_at = ?
WHERE id = ?;

-- name: SetChapterPrompts :exec
UPDATE chapters SET slide_prompts_json = ?, updated_at = ? WHERE id = ?;

-- Written by index for the same reason SetChapterSlide is: the operator is
-- replacing one prompt, and rewriting the whole array would carry back whatever
-- the row held when it was read.
-- name: SetChapterPrompt :exec
UPDATE chapters
SET slide_prompts_json = json_set(slide_prompts_json, CAST(sqlc.arg(path) AS TEXT), CAST(sqlc.arg(prompt) AS TEXT)),
    updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: SetChapterAudio :exec
UPDATE chapters SET audio_asset_id = ?, updated_at = ? WHERE id = ?;

-- name: SetChapterSlide :exec
UPDATE chapters
SET slide_asset_ids_json = json_set(slide_asset_ids_json, CAST(sqlc.arg(path) AS TEXT), CAST(sqlc.arg(asset_id) AS TEXT)),
    updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: SetChapterClip :exec
UPDATE chapters SET clip_asset_id = ?, updated_at = ? WHERE id = ?;
