-- +goose Up

-- Staleness is a flag rather than a state, because the combination that matters
-- is `succeeded` AND stale: the artifact is intact and may still be correct,
-- but an input it was derived from has since changed. A new state value would
-- lose both the success and the pointer to the artifact.
--
-- Existing rows are not stale: nothing has been re-run under the old code that
-- we could retroactively know about.
ALTER TABLE tasks ADD COLUMN stale INTEGER NOT NULL DEFAULT 0;

-- Partial index: the stale set is a small minority of a large table, and every
-- query over it filters on exactly this predicate.
CREATE INDEX idx_tasks_stale ON tasks (video_id) WHERE stale = 1;

-- +goose Down

DROP INDEX idx_tasks_stale;
ALTER TABLE tasks DROP COLUMN stale;
