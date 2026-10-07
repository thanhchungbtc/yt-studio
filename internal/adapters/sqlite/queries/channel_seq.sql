-- Video refs are minted from a per-channel counter. The increment and the read
-- back run inside one write transaction on the single writer connection, so
-- nothing can interleave between them.

-- name: IncrementVideoSeq :exec
UPDATE channels SET video_seq = video_seq + 1, updated_at = ? WHERE id = ?;

-- name: GetVideoSeq :one
SELECT video_seq FROM channels WHERE id = ?;
