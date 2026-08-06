-- +goose Up

-- Timestamps are INTEGER unix nanoseconds (UTC). Integer comparison keeps every
-- index range scan cheap and avoids per-row string parsing on read.

CREATE TABLE channels (
    id          TEXT    PRIMARY KEY,
    slug        TEXT    NOT NULL UNIQUE,
    name        TEXT    NOT NULL,
    description TEXT    NOT NULL DEFAULT '',
    credentials TEXT    NOT NULL DEFAULT 'missing',
    video_seq   INTEGER NOT NULL DEFAULT 0,
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
);

CREATE TABLE videos (
    id                 TEXT    PRIMARY KEY,
    channel_id         TEXT    NOT NULL REFERENCES channels (id) ON DELETE CASCADE,
    ref                TEXT    NOT NULL UNIQUE,
    title              TEXT    NOT NULL,
    topic              TEXT    NOT NULL DEFAULT '',
    state              TEXT    NOT NULL,
    chapter_count      INTEGER NOT NULL,
    slides_per_chapter INTEGER NOT NULL,
    blueprint_asset_id TEXT,
    final_asset_id     TEXT,
    metadata_json      TEXT,
    upload_json        TEXT,
    error              TEXT    NOT NULL DEFAULT '',
    created_at         INTEGER NOT NULL,
    updated_at         INTEGER NOT NULL,
    started_at         INTEGER,
    completed_at       INTEGER,
    -- How long the finished video should run. A channel that wants three hours
    -- does not much mind how many chapters that takes, so length is stated here
    -- rather than falling out of chapter_count * words_per_chapter. Zero means
    -- unset, and that derivation stands in.
    target_duration_minutes INTEGER NOT NULL DEFAULT 0,
    -- The image that fronts the video on YouTube, and the grid drawn onto it.
    --
    -- The address is a column rather than a lookup over assets by kind, for the
    -- same reason final_asset_id is: the row naming an address is what makes
    -- that file reachable and owned. An asset held only as a row of its own has
    -- nobody claiming it, and the ownership repair has nothing to find.
    thumbnail_asset_id TEXT,
    -- One icon task exists per cell from the moment the graph expands, so the
    -- width has to be a property of the video rather than of the settings
    -- table. Reading it live would mean a video whose graph is ten wide while
    -- the setting says eight, with nothing on the row to explain the difference.
    thumbnail_cells INTEGER NOT NULL DEFAULT 10,
    -- One caption and one icon prompt per cell.
    thumbnail_plan_json TEXT,
    -- One slot per cell, filled by the icon tasks as they finish. The array is
    -- sized when the plan is written, because json_set appends rather than pads
    -- when the index is past the end -- an out-of-order write into an empty
    -- array would land in the wrong cell, and the icons finish in whatever
    -- order the image pool hands them back.
    thumbnail_icon_ids_json TEXT NOT NULL DEFAULT '[]'
);

CREATE INDEX idx_videos_channel_created ON videos (channel_id, created_at DESC);
CREATE INDEX idx_videos_state ON videos (state);
CREATE INDEX idx_videos_created ON videos (created_at DESC);

CREATE TABLE chapters (
    id                   TEXT    PRIMARY KEY,
    video_id             TEXT    NOT NULL REFERENCES videos (id) ON DELETE CASCADE,
    ordinal              INTEGER NOT NULL,
    title                TEXT    NOT NULL,
    summary              TEXT    NOT NULL DEFAULT '',
    script               TEXT    NOT NULL DEFAULT '',
    slide_prompts_json   TEXT    NOT NULL DEFAULT '[]',
    audio_asset_id       TEXT,
    slide_asset_ids_json TEXT    NOT NULL DEFAULT '[]',
    clip_asset_id        TEXT,
    duration_seconds     REAL    NOT NULL DEFAULT 0,
    created_at           INTEGER NOT NULL,
    updated_at           INTEGER NOT NULL,
    -- The spoken-word budget the blueprint assigned to this one chapter. It is
    -- deliberately uneven -- a deep chapter carries roughly twice a short one.
    -- Zero means the blueprint assigned none, and the per-chapter average
    -- stands in.
    estimated_words INTEGER NOT NULL DEFAULT 0,
    UNIQUE (video_id, ordinal)
);

CREATE INDEX idx_chapters_video_ordinal ON chapters (video_id, ordinal);

-- An asset row records one video's ownership of a content address, not the
-- address itself.
--
-- Keying on the sha256 alone would be right for the file -- identical bytes are
-- stored once -- but it gives the only row to whichever video produced those
-- bytes first, leaving every later video referencing the same address with no
-- row at all. A video's asset list would silently omit anything another video
-- produced first, and video_id could not decide what a delete may reclaim,
-- because it would name one owner out of several.
--
-- Keying on (id, video_id) makes ownership representable. path, size and mime
-- repeat per owner -- a few dozen bytes against a file measured in megabytes --
-- and cannot drift, being functions of the address and the kind.
--
-- chapter_id is deliberately NOT in the key. SQLite permits NULL in a
-- non-INTEGER primary-key column of a rowid table and treats NULLs as distinct
-- in a unique index, so a video-level asset -- a blueprint or a final render,
-- both with a NULL chapter_id -- would insert a duplicate row on every re-run
-- instead of hitting DO NOTHING. It stays an advisory record of the first
-- chapter that used the file; per-chapter previews read the id lists on
-- chapters, not this table.
CREATE TABLE assets (
    id         TEXT    NOT NULL,
    video_id   TEXT    NOT NULL REFERENCES videos (id) ON DELETE CASCADE,
    chapter_id TEXT,
    kind       TEXT    NOT NULL,
    path       TEXT    NOT NULL,
    size       INTEGER NOT NULL,
    mime       TEXT    NOT NULL,
    provenance TEXT    NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    PRIMARY KEY (id, video_id)
);

CREATE INDEX idx_assets_video ON assets (video_id);
CREATE INDEX idx_assets_chapter ON assets (chapter_id);

CREATE TABLE tasks (
    id             TEXT    PRIMARY KEY,
    video_id       TEXT    NOT NULL,
    chapter_id     TEXT,
    kind           TEXT    NOT NULL,
    ordinal        INTEGER NOT NULL,
    idx            INTEGER NOT NULL,
    state          TEXT    NOT NULL,
    pool           TEXT    NOT NULL,
    gate           TEXT    NOT NULL DEFAULT '',
    attempt        INTEGER NOT NULL DEFAULT 0,
    max_attempts   INTEGER NOT NULL DEFAULT 3,
    deps_remaining INTEGER NOT NULL DEFAULT 0,
    error          TEXT    NOT NULL DEFAULT '',
    created_at     INTEGER NOT NULL,
    updated_at     INTEGER NOT NULL,
    started_at     INTEGER,
    finished_at    INTEGER,
    not_before     INTEGER,
    -- Staleness is a flag rather than a state, because the combination that
    -- matters is `succeeded` AND stale: the artifact is intact and may still be
    -- correct, but an input it was derived from has since changed. A new state
    -- value would lose both the success and the pointer to the artifact.
    stale INTEGER NOT NULL DEFAULT 0
);

-- Every column the scheduler filters or orders by is indexed; a test asserts
-- with EXPLAIN QUERY PLAN that no scheduler query falls back to a full scan.
CREATE INDEX idx_tasks_video_ordinal ON tasks (video_id, ordinal, idx);
CREATE INDEX idx_tasks_state ON tasks (state);
CREATE INDEX idx_tasks_video_state ON tasks (video_id, state);
CREATE INDEX idx_tasks_updated ON tasks (updated_at DESC);

-- Partial index: the stale set is a small minority of a large table, and every
-- query over it filters on exactly this predicate.
CREATE INDEX idx_tasks_stale ON tasks (video_id) WHERE stale = 1;

CREATE TABLE task_deps (
    video_id TEXT NOT NULL,
    from_id  TEXT NOT NULL,
    to_id    TEXT NOT NULL,
    PRIMARY KEY (from_id, to_id)
) WITHOUT ROWID;

CREATE INDEX idx_task_deps_video ON task_deps (video_id);

CREATE TABLE settings (
    key         TEXT    PRIMARY KEY,
    value       TEXT    NOT NULL,
    type        TEXT    NOT NULL,
    grp         TEXT    NOT NULL DEFAULT '',
    description TEXT    NOT NULL DEFAULT '',
    min_value   REAL    NOT NULL DEFAULT 0,
    max_value   REAL    NOT NULL DEFAULT 0,
    updated_at  INTEGER NOT NULL
);

-- +goose Down

DROP TABLE settings;
DROP TABLE task_deps;
DROP TABLE tasks;
DROP TABLE assets;
DROP TABLE chapters;
DROP TABLE videos;
DROP TABLE channels;
