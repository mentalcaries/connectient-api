-- name: CheckPracticeCodeExists :one
SELECT id FROM practices where practice_code = sqlc.arg(practice_code);