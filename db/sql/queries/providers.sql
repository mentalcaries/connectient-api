-- name: GetProvidersByPracticeID :many
SELECT p.id, p.created_at, p.first_name, p.last_name, p.title, p.specialty,
       pp.is_main
FROM practice_provider pp
JOIN provider p ON p.id = pp.provider_id
WHERE pp.practice_id = sqlc.arg(practice_id)
ORDER BY pp.is_main DESC, p.created_at ASC, p.id ASC;

-- name: LockPracticeForProviderMutation :one
SELECT id
FROM practices
WHERE id = sqlc.arg(id)
FOR UPDATE;

-- name: CreateProvider :one
INSERT INTO provider (first_name, last_name, title, specialty)
VALUES (sqlc.arg(first_name), sqlc.arg(last_name), sqlc.narg(title), sqlc.arg(specialty))
RETURNING id, created_at, first_name, last_name, title, specialty;

-- name: CreatePracticeProviderLink :one
INSERT INTO practice_provider (practice_id, provider_id, is_main)
VALUES (sqlc.arg(practice_id), sqlc.arg(provider_id), sqlc.arg(is_main))
RETURNING id;

-- name: GetProviderForMutation :one
SELECT p.id, p.created_at, p.first_name, p.last_name, p.title, p.specialty,
       pp.is_main
FROM practice_provider pp
JOIN provider p ON p.id = pp.provider_id
WHERE pp.practice_id = sqlc.arg(practice_id)
  AND pp.provider_id = sqlc.arg(provider_id);

-- name: PatchProvider :execrows
UPDATE provider p
SET
    first_name = CASE
        WHEN sqlc.arg(set_first_name)::boolean THEN sqlc.arg(first_name)::text
        ELSE p.first_name
    END,
    last_name = CASE
        WHEN sqlc.arg(set_last_name)::boolean THEN sqlc.arg(last_name)::text
        ELSE p.last_name
    END,
    title = CASE
        WHEN sqlc.arg(set_title)::boolean THEN sqlc.narg(title)::text
        ELSE p.title
    END,
    specialty = CASE
        WHEN sqlc.arg(set_specialty)::boolean THEN sqlc.arg(specialty)::text
        ELSE p.specialty
    END
FROM practice_provider pp
WHERE p.id = sqlc.arg(provider_id)
  AND pp.provider_id = p.id
  AND pp.practice_id = sqlc.arg(practice_id);

-- name: ClearMainProvider :exec
UPDATE practice_provider
SET is_main = FALSE
WHERE practice_id = sqlc.arg(practice_id)
  AND is_main = TRUE;

-- name: SetMainProvider :execrows
UPDATE practice_provider
SET is_main = TRUE
WHERE practice_id = sqlc.arg(practice_id)
  AND provider_id = sqlc.arg(provider_id);

-- name: DeletePracticeProviderLink :execrows
DELETE FROM practice_provider
WHERE practice_id = sqlc.arg(practice_id)
  AND provider_id = sqlc.arg(provider_id)
  AND is_main = FALSE;
