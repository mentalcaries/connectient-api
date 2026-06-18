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
RETURNING id;

-- name: GetProcedureTypesByPracticeID :many
SELECT * FROM procedure_types
WHERE practice_id = sqlc.arg(practice_id)
AND deleted_at IS NULL
ORDER BY sort_order ASC;

-- name: UpdateProcedureType :exec
UPDATE procedure_types
SET
    name       = COALESCE(sqlc.narg(name), name),
    is_active  = COALESCE(sqlc.narg(is_active), is_active),
    sort_order = COALESCE(sqlc.narg(sort_order), sort_order)
WHERE id = sqlc.arg(id)
AND practice_id = sqlc.arg(practice_id);

-- name: DeleteProcedureType :exec
UPDATE procedure_types
SET deleted_at = NOW()
WHERE id = sqlc.arg(id)
AND practice_id = sqlc.arg(practice_id);