-- +goose Up

-- Three columns that make a video's plan survive from the blueprint to the
-- script writer, instead of being re-derived or guessed at each hop.

-- The delivery rate a word count is turned into a duration with. It was a
-- constant in the app layer at 150 while the blueprint prompt budgeted at 130,
-- so a video planned as 145 minutes displayed as 126. It belongs to the channel
-- rather than the binary: a slow late-night read and a brisk documentary read
-- are different speeds, and both are correct for their channel.
ALTER TABLE channels ADD COLUMN words_per_minute INTEGER NOT NULL DEFAULT 130;

-- How long the finished video should run. Until now length was whatever fell
-- out of chapter_count * words_per_chapter, which is the wrong way round for a
-- channel that wants three hours and does not much mind how many chapters that
-- takes.
--
-- Zero means unset, and the old derivation still applies — so every existing
-- video keeps the length it was planned with.
ALTER TABLE videos ADD COLUMN target_duration_minutes INTEGER NOT NULL DEFAULT 0;

-- The spoken-word budget the blueprint assigned to this one chapter. It is
-- deliberately uneven — a deep chapter carries roughly twice a short one — and
-- it previously survived only inside the summary text, where the script writer
-- recovered it with a regular expression over our own formatting.
--
-- Zero means the blueprint did not assign one, and the channel's per-chapter
-- average stands in.
ALTER TABLE chapters ADD COLUMN estimated_words INTEGER NOT NULL DEFAULT 0;

-- +goose Down

ALTER TABLE chapters DROP COLUMN estimated_words;
ALTER TABLE videos DROP COLUMN target_duration_minutes;
ALTER TABLE channels DROP COLUMN words_per_minute;
