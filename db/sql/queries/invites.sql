-- name: ListPracticeInvites :many
SELECT id, first_name, last_name, email, role, org_role, created_at, token_expires_at
FROM practice_invites
WHERE practice_id = sqlc.arg(practice_id)
  AND accepted_at IS NULL
  AND token_expires_at > NOW()
ORDER BY created_at DESC, id DESC;

-- name: FindPracticeUserByEmail :one
SELECT id FROM users
WHERE practice_id = sqlc.arg(practice_id)
  AND lower(email) = lower(sqlc.arg(email))
  AND deleted_at IS NULL
LIMIT 1;

-- name: GetPracticeNameForInvite :one
SELECT name FROM practices WHERE id = sqlc.arg(id);

-- name: UpsertPracticeInvite :one
INSERT INTO practice_invites (
    practice_id, email, first_name, last_name, org_role, role,
    invited_by, token, token_expires_at, accepted_at
) VALUES (
    sqlc.arg(practice_id), sqlc.arg(email), sqlc.arg(first_name),
    sqlc.arg(last_name), sqlc.narg(org_role), sqlc.arg(role),
    sqlc.arg(invited_by), sqlc.arg(token), sqlc.arg(token_expires_at), NULL
)
ON CONFLICT (practice_id, email) DO UPDATE SET
    first_name = EXCLUDED.first_name,
    last_name = EXCLUDED.last_name,
    org_role = EXCLUDED.org_role,
    role = EXCLUDED.role,
    invited_by = EXCLUDED.invited_by,
    token = EXCLUDED.token,
    token_expires_at = EXCLUDED.token_expires_at,
    accepted_at = NULL
RETURNING id, token, token_expires_at;

-- name: DeletePracticeInvite :execrows
DELETE FROM practice_invites
WHERE id = sqlc.arg(id) AND practice_id = sqlc.arg(practice_id);

-- name: GetPracticeInviteForResend :one
SELECT i.id, i.email, i.first_name, i.last_name, i.role, i.org_role,
       p.name AS practice_name
FROM practice_invites i
JOIN practices p ON p.id = i.practice_id
WHERE i.id = sqlc.arg(id)
  AND i.practice_id = sqlc.arg(practice_id)
  AND i.accepted_at IS NULL;

-- name: RotatePracticeInvite :execrows
UPDATE practice_invites
SET token = sqlc.arg(token), token_expires_at = sqlc.arg(token_expires_at)
WHERE id = sqlc.arg(id)
  AND practice_id = sqlc.arg(practice_id)
  AND accepted_at IS NULL;

-- name: GetInviteValidation :one
SELECT i.id, i.first_name, i.last_name, i.email, i.role, i.org_role,
       i.token_expires_at, i.accepted_at, p.name AS practice_name
FROM practice_invites i
JOIN practices p ON p.id = i.practice_id
WHERE i.token = sqlc.arg(token);

-- name: GetInviteAcceptanceTarget :one
SELECT practice_id
FROM practice_invites
WHERE token = sqlc.arg(token);

-- name: LockInviteAcceptance :one
SELECT id, practice_id, email, role, org_role, invited_by,
       token_expires_at, accepted_at
FROM practice_invites
WHERE token = sqlc.arg(token)
FOR UPDATE;

-- name: GetIdentityMembershipForUpdate :one
SELECT id, practice_id, email, first_name, last_name, is_active, deleted_at
FROM users
WHERE id = sqlc.arg(id)
FOR UPDATE;

-- name: CreateInvitedMembership :exec
INSERT INTO users (
    id, practice_id, email, first_name, last_name, mobile_phone,
    org_role, role, invited_by, is_active, terms_agreed_at
) VALUES (
    sqlc.arg(id), sqlc.arg(practice_id), sqlc.arg(email),
    sqlc.arg(first_name), sqlc.arg(last_name), sqlc.narg(mobile_phone),
    sqlc.narg(org_role), sqlc.arg(role), sqlc.arg(invited_by), TRUE, NOW()
);

-- name: MarkInviteAccepted :execrows
UPDATE practice_invites
SET accepted_at = NOW()
WHERE id = sqlc.arg(id) AND accepted_at IS NULL;
