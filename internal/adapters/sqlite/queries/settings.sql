-- name: GetSetting :one
SELECT * FROM settings WHERE key = ?;

-- name: ListSettings :many
SELECT * FROM settings ORDER BY grp, key;

-- name: UpsertSetting :exec
INSERT INTO settings (key, value, type, grp, description, min_value, max_value, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (key) DO UPDATE SET
    type = excluded.type,
    grp = excluded.grp,
    description = excluded.description,
    min_value = excluded.min_value,
    max_value = excluded.max_value;

-- name: UpdateSettingValue :exec
UPDATE settings SET value = ?, updated_at = ? WHERE key = ?;
