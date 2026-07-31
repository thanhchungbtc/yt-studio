-- +goose Up

-- The thumbnail's grid: how wide it is, what it says, and what landed.

-- One icon task exists per cell from the moment the graph expands, so the width
-- has to be a property of the video rather than of the settings table. Reading
-- it live would mean a video whose graph is ten wide while the setting says
-- eight, with nothing on the row to explain the difference.
ALTER TABLE videos ADD COLUMN thumbnail_cells INTEGER NOT NULL DEFAULT 10;

-- The plan: one caption and one icon prompt per cell.
ALTER TABLE videos ADD COLUMN thumbnail_plan_json TEXT;

-- One slot per cell, filled by the icon tasks as they finish. The array is
-- sized when the plan is written, because json_set appends rather than pads
-- when the index is past the end -- an out-of-order write into an empty array
-- would land in the wrong cell, and the icons finish in whatever order the
-- image pool hands them back.
ALTER TABLE videos ADD COLUMN thumbnail_icon_ids_json TEXT NOT NULL DEFAULT '[]';

-- +goose Down

ALTER TABLE videos DROP COLUMN thumbnail_icon_ids_json;
ALTER TABLE videos DROP COLUMN thumbnail_plan_json;
ALTER TABLE videos DROP COLUMN thumbnail_cells;
