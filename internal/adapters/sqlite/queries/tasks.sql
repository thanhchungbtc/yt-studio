-- name: GetTaskByID :one
SELECT * FROM tasks WHERE id = ?;

-- name: ListTasksByVideo :many
SELECT * FROM tasks WHERE video_id = ? ORDER BY ordinal, idx, kind;

-- name: ListRecentTasks :many
SELECT * FROM tasks ORDER BY updated_at DESC LIMIT ?;

-- name: CountTasksByVideo :one
SELECT
    COUNT(*) AS total,
    CAST(COALESCE(SUM(CASE WHEN state = 'succeeded' THEN 1 ELSE 0 END), 0) AS INTEGER) AS succeeded,
    CAST(COALESCE(SUM(CASE WHEN state = 'failed' THEN 1 ELSE 0 END), 0) AS INTEGER) AS failed,
    CAST(COALESCE(SUM(CASE WHEN state = 'running' THEN 1 ELSE 0 END), 0) AS INTEGER) AS running,
    CAST(COALESCE(SUM(CASE WHEN state = 'ready' THEN 1 ELSE 0 END), 0) AS INTEGER) AS ready,
    CAST(COALESCE(SUM(CASE WHEN state = 'blocked' THEN 1 ELSE 0 END), 0) AS INTEGER) AS blocked,
    CAST(COALESCE(SUM(CASE WHEN state = 'awaiting_approval' THEN 1 ELSE 0 END), 0) AS INTEGER) AS awaiting,
    CAST(COALESCE(SUM(CASE WHEN state = 'cancelled' THEN 1 ELSE 0 END), 0) AS INTEGER) AS cancelled,
    CAST(COALESCE(SUM(CASE WHEN stale = 1 THEN 1 ELSE 0 END), 0) AS INTEGER) AS stale
FROM tasks WHERE video_id = ?;

-- name: ListVideosWithOpenTasks :many
SELECT DISTINCT video_id FROM tasks
WHERE state IN ('blocked', 'ready', 'running', 'awaiting_approval');

-- name: ListTaskDepsByVideo :many
SELECT video_id, from_id, to_id FROM task_deps WHERE video_id = ?;

-- name: InsertTask :exec
INSERT INTO tasks (
    id, video_id, chapter_id, kind, ordinal, idx, state, pool, gate,
    attempt, max_attempts, deps_remaining, error, stale,
    created_at, updated_at, started_at, finished_at, not_before
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO NOTHING;

-- name: InsertTaskDep :exec
INSERT INTO task_deps (video_id, from_id, to_id) VALUES (?, ?, ?)
ON CONFLICT (from_id, to_id) DO NOTHING;

-- name: ApplyTaskTransition :exec
UPDATE tasks
SET state = ?, attempt = ?, deps_remaining = ?, error = ?, stale = ?,
    started_at = ?, finished_at = ?, not_before = ?, updated_at = ?
WHERE id = ?;

-- name: ListStaleTasksByVideo :many
SELECT * FROM tasks WHERE video_id = ? AND stale = 1 ORDER BY ordinal, idx, kind;

-- name: DeleteTasksByVideo :exec
DELETE FROM tasks WHERE video_id = ?;

-- name: DeleteTaskDepsByVideo :exec
DELETE FROM task_deps WHERE video_id = ?;
