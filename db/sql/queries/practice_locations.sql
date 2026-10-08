-- name: GetPracticeLocationsByPracticeID :many
SELECT id, created_at, deleted_at, practice_id, name, address, is_active,
       sort_order, available_weekdays
FROM practice_locations
WHERE practice_id = sqlc.arg(practice_id)
  AND deleted_at IS NULL
ORDER BY sort_order ASC;

-- name: LockPracticeForLocationSort :one
SELECT id
FROM practices
WHERE id = sqlc.arg(id)
FOR UPDATE;

-- name: GetNextPracticeLocationSortOrder :one
SELECT (COALESCE(MAX(sort_order), 0) + 1)::integer
FROM practice_locations
WHERE practice_id = sqlc.arg(practice_id);

-- name: CreatePracticeLocation :one
INSERT INTO practice_locations (
    practice_id,
    name,
    address,
    is_active,
    sort_order
) VALUES (
    sqlc.arg(practice_id),
    sqlc.arg(name),
    sqlc.narg(address),
    TRUE,
    sqlc.arg(sort_order)
)
RETURNING id, created_at, deleted_at, practice_id, name, address, is_active,
          sort_order, available_weekdays;

-- name: PatchPracticeLocation :execrows
UPDATE practice_locations
SET
    name = CASE
        WHEN sqlc.arg(set_name)::boolean THEN sqlc.arg(name)::text
        ELSE name
    END,
    address = CASE
        WHEN sqlc.arg(set_address)::boolean THEN sqlc.narg(address)::text
        ELSE address
    END,
    is_active = CASE
        WHEN sqlc.arg(set_is_active)::boolean THEN sqlc.arg(is_active)::boolean
        ELSE is_active
    END,
    sort_order = CASE
        WHEN sqlc.arg(set_sort_order)::boolean THEN sqlc.arg(sort_order)::integer
        ELSE sort_order
    END,
    deleted_at = CASE
        WHEN sqlc.arg(set_deleted_at)::boolean THEN sqlc.narg(deleted_at)::timestamptz
        ELSE deleted_at
    END,
    available_weekdays = CASE
        WHEN sqlc.arg(set_available_weekdays)::boolean THEN sqlc.arg(available_weekdays)::smallint[]
        ELSE available_weekdays
    END
WHERE id = sqlc.arg(id)
  AND practice_id = sqlc.arg(practice_id);
