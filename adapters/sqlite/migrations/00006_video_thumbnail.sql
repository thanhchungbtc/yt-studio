-- +goose Up

-- The address of the image that fronts the video on YouTube.
--
-- It is a column rather than a lookup over the assets table by kind, for the
-- same reason final_asset_id is: the row that names an address is what makes
-- that file reachable and owned. An asset the database holds only as a row of
-- its own has nobody claiming it, and the ownership repair has nothing to find.
--
-- NULL means the thumbnail task has not run, which is every video that existed
-- before this column did.
ALTER TABLE videos ADD COLUMN thumbnail_asset_id TEXT;

-- +goose Down

ALTER TABLE videos DROP COLUMN thumbnail_asset_id;
