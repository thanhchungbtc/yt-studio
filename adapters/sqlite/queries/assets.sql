-- name: GetAssetByID :one
SELECT * FROM assets WHERE id = ?;

-- name: ListAssetsByVideo :many
SELECT * FROM assets WHERE video_id = ? ORDER BY created_at;

-- name: PutAsset :exec
-- Content-addressed: an identical byte stream re-uses the existing row, which
-- is what makes a partial re-run cheap.
INSERT INTO assets (id, video_id, chapter_id, kind, path, size, mime, provenance, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO NOTHING;
