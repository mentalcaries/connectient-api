-- name: CreateProcedureType :one
INSERT INTO procedure_types (
    practice_id,
    name,
    value,
    is_active,
    is_default,
    is_primary,
    sort_order
) VALUES (
    sqlc.arg(practice_id),
    sqlc.arg(name),
    sqlc.arg(value),
    sqlc.arg(is_active),
    sqlc.arg(is_default),
    sqlc.arg(is_primary),
    sqlc.arg(sort_order)
)
RETURNING id, created_at, deleted_at, practice_id, name, value, is_active,
          is_default, is_primary, sort_order;

-- name: GetProcedureTypesByPracticeID :many
SELECT * FROM procedure_types
WHERE practice_id = sqlc.arg(practice_id)
AND deleted_at IS NULL
ORDER BY sort_order ASC;

-- name: LockPracticeForProcedureTypeSort :one
SELECT id
FROM practices
WHERE id = sqlc.arg(id)
FOR UPDATE;

-- name: GetNextProcedureTypeSortOrder :one
SELECT (COALESCE(MAX(sort_order), 0) + 1)::integer
FROM procedure_types
WHERE practice_id = sqlc.arg(practice_id);

-- name: ClearPrimaryProcedureType :exec
UPDATE procedure_types
SET is_primary = FALSE
WHERE practice_id = sqlc.arg(practice_id)
  AND is_primary = TRUE;

-- name: GetProcedureTypeForMutation :one
SELECT id, is_default
FROM procedure_types
WHERE id = sqlc.arg(id)
  AND practice_id = sqlc.arg(practice_id)
  AND deleted_at IS NULL;

-- name: PatchProcedureType :execrows
UPDATE procedure_types
SET name = CASE
        WHEN sqlc.arg(set_name)::boolean THEN sqlc.arg(name)::text
        ELSE name
    END,
    is_active = CASE
        WHEN sqlc.arg(set_is_active)::boolean THEN sqlc.arg(is_active)::boolean
        ELSE is_active
    END,
    sort_order = CASE
        WHEN sqlc.arg(set_sort_order)::boolean THEN sqlc.arg(sort_order)::integer
        ELSE sort_order
    END
WHERE id = sqlc.arg(id)
AND practice_id = sqlc.arg(practice_id);

-- name: DeleteProcedureType :execrows
UPDATE procedure_types
SET deleted_at = NOW()
WHERE id = sqlc.arg(id)
AND practice_id = sqlc.arg(practice_id);
