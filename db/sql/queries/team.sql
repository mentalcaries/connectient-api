-- name: ListPracticeUsers :many
SELECT id, first_name, last_name, email, role, org_role, is_active, created_at
FROM users
WHERE practice_id = sqlc.arg(practice_id)
  AND deleted_at IS NULL
ORDER BY created_at ASC, id ASC;

-- name: GetPracticeUserMutationState :one
SELECT id, role, is_active
FROM users
WHERE id = sqlc.arg(id)
  AND practice_id = sqlc.arg(practice_id)
  AND deleted_at IS NULL;

-- name: PatchPracticeUser :execrows
UPDATE users
SET role = CASE WHEN sqlc.arg(set_role)::boolean THEN sqlc.arg(role)::text ELSE role END,
    is_active = CASE WHEN sqlc.arg(set_is_active)::boolean THEN sqlc.arg(is_active)::boolean ELSE is_active END
WHERE id = sqlc.arg(id)
  AND practice_id = sqlc.arg(practice_id)
  AND deleted_at IS NULL
  AND role <> 'owner';

-- name: SoftDeletePracticeUser :execrows
UPDATE users
SET is_active = FALSE, deleted_at = NOW()
WHERE id = sqlc.arg(id)
  AND practice_id = sqlc.arg(practice_id)
  AND deleted_at IS NULL
  AND role <> 'owner';

-- name: GetPracticeSeatUsage :one
SELECT
  (SELECT COUNT(*) FROM users u
   WHERE u.practice_id = sqlc.arg(practice_id) AND u.deleted_at IS NULL AND u.is_active = TRUE)
  +
  (SELECT COUNT(*) FROM practice_invites i
   WHERE i.practice_id = sqlc.arg(practice_id) AND i.accepted_at IS NULL AND i.token_expires_at > NOW())
  AS used,
  COALESCE((SELECT plan FROM subscription WHERE "referenceId" = sqlc.arg(practice_id)::text LIMIT 1), 'pro')::text AS plan;
