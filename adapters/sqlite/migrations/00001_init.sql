-- +goose Up

-- Timestamps are INTEGER unix nanoseconds (UTC). Integer comparison keeps every
-- index range scan cheap and avoids per-row string parsing on read.

CREATE TABLE channels (
    id                TEXT    PRIMARY KEY,
    slug              TEXT    NOT NULL UNIQUE,
    name              TEXT    NOT NULL,
    description       TEXT    NOT NULL DEFAULT '',
    tone              TEXT    NOT NULL DEFAULT '',
    voice             TEXT    NOT NULL DEFAULT '',
    image_style       TEXT    NOT NULL DEFAULT '',
    language          TEXT    NOT NULL DEFAULT 'en-US',
    words_per_chapter INTEGER NOT NULL DEFAULT 450,
    credentials       TEXT    NOT NULL DEFAULT 'missing',
    video_seq         INTEGER NOT NULL DEFAULT 0,
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL
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
    completed_at       INTEGER
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
    UNIQUE (video_id, ordinal)
);

CREATE INDEX idx_chapters_video_ordinal ON chapters (video_id, ordinal);

CREATE TABLE assets (
    id         TEXT    PRIMARY KEY,
    video_id   TEXT    NOT NULL,
    chapter_id TEXT,
    kind       TEXT    NOT NULL,
    path       TEXT    NOT NULL,
    size       INTEGER NOT NULL,
    mime       TEXT    NOT NULL,
    provenance TEXT    NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL
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
    not_before     INTEGER
);

-- Every column the scheduler filters or orders by is indexed; a test asserts
-- with EXPLAIN QUERY PLAN that no scheduler query falls back to a full scan.
CREATE INDEX idx_tasks_video_ordinal ON tasks (video_id, ordinal, idx);
CREATE INDEX idx_tasks_state ON tasks (state);
CREATE INDEX idx_tasks_video_state ON tasks (video_id, state);
CREATE INDEX idx_tasks_updated ON tasks (updated_at DESC);

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
    min_value   INTEGER NOT NULL DEFAULT 0,
    max_value   INTEGER NOT NULL DEFAULT 0,
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
