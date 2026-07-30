-- +goose Up

-- An asset row now records one video's ownership of a content address rather
-- than the address itself.
--
-- The old primary key was the sha256 alone, with PutAsset doing ON CONFLICT (id)
-- DO NOTHING. That is right for the file -- identical bytes are stored once --
-- but it gave the only row to whichever video produced those bytes first, and
-- left every later video referencing the same address with no row at all. Two
-- things followed: a video's asset list silently omitted anything another video
-- produced first, and video_id could not decide what a delete may reclaim,
-- because it named one owner out of several.
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

CREATE TABLE assets_owned (
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

-- Rows whose video is already gone are dropped rather than carried over: the
-- new foreign key would reject them, and they are exactly the leak this work
-- exists to fix. Their files are reclaimed by `yt-studio sweep`, which runs the
-- ownership repair first so that a file a surviving video still references
-- through its chapters is given its missing row before anything is unlinked.
INSERT INTO assets_owned (id, video_id, chapter_id, kind, path, size, mime, provenance, created_at)
SELECT id, video_id, chapter_id, kind, path, size, mime, provenance, created_at
FROM assets
WHERE video_id IN (SELECT id FROM videos);

DROP TABLE assets;

ALTER TABLE assets_owned RENAME TO assets;

CREATE INDEX idx_assets_video ON assets (video_id);
CREATE INDEX idx_assets_chapter ON assets (chapter_id);

-- +goose Down

CREATE TABLE assets_addressed (
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

-- Collapsing back to one row per content address keeps the earliest owner,
-- which is what the old write path would have recorded.
INSERT INTO assets_addressed (id, video_id, chapter_id, kind, path, size, mime, provenance, created_at)
SELECT id, video_id, chapter_id, kind, path, size, mime, provenance, MIN(created_at)
FROM assets
GROUP BY id;

DROP TABLE assets;

ALTER TABLE assets_addressed RENAME TO assets;

CREATE INDEX idx_assets_video ON assets (video_id);
CREATE INDEX idx_assets_chapter ON assets (chapter_id);
