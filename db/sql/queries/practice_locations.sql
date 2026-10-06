-- name: GetPracticeLocationsByPracticeID :many
SELECT id, created_at, deleted_at, practice_id, name, address, is_active,
       sort_order, available_weekdays
FROM practice_locations
WHERE practice_id = sqlc.arg(practice_id)
  AND deleted_at IS NULL
ORDER BY sort_order ASC;
