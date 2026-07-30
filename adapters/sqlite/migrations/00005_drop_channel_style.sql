-- +goose Up

-- The per-channel style configuration is being reconsidered, so its columns go
-- rather than linger half-used. What replaces them is a later decision; leaving
-- tone and voice in place meanwhile would make them look like the answer.
--
-- The narration figures they carried are now domain constants
-- (entity.DefaultWordsPerChapter, entity.DefaultWordsPerMinute), so a video is
-- still planned and timed at one speed — just not a configurable one.
ALTER TABLE channels DROP COLUMN tone;
ALTER TABLE channels DROP COLUMN voice;
ALTER TABLE channels DROP COLUMN image_style;
ALTER TABLE channels DROP COLUMN language;
ALTER TABLE channels DROP COLUMN words_per_chapter;
ALTER TABLE channels DROP COLUMN words_per_minute;

-- +goose Down

ALTER TABLE channels ADD COLUMN tone              TEXT    NOT NULL DEFAULT '';
ALTER TABLE channels ADD COLUMN voice             TEXT    NOT NULL DEFAULT '';
ALTER TABLE channels ADD COLUMN image_style       TEXT    NOT NULL DEFAULT '';
ALTER TABLE channels ADD COLUMN language          TEXT    NOT NULL DEFAULT 'en-US';
ALTER TABLE channels ADD COLUMN words_per_chapter INTEGER NOT NULL DEFAULT 450;
ALTER TABLE channels ADD COLUMN words_per_minute  INTEGER NOT NULL DEFAULT 130;
