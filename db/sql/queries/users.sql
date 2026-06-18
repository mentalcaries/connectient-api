-- name: CreateUser :one
INSERT INTO users (
    id,
    first_name,
    last_name,
    email,
    mobile_phone,
    role,
    terms_agreed_at
) VALUES (
    sqlc.arg(id),
    sqlc.arg(first_name),
    sqlc.arg(last_name),
    sqlc.arg(email),
    sqlc.arg(mobile_phone),
    sqlc.arg(role),
    sqlc.arg(terms_agreed_at)
)
RETURNING *;

-- name: GetAllUsers :many
SELECT * FROM users;

-- name: GetUser :one
SELECT * FROM users WHERE id = sqlc.arg(id);

-- name: UpdateUser :one
UPDATE users
SET
    first_name = COALESCE(sqlc.narg(first_name), first_name),
    last_name = COALESCE(sqlc.narg(last_name), last_name),
    email = COALESCE(sqlc.narg(email), email),
    mobile_phone = COALESCE(sqlc.narg(mobile_phone), mobile_phone),
    role = COALESCE(sqlc.narg(role), role),
    org_role = COALESCE(sqlc.narg(org_role), org_role),
    is_active = COALESCE(sqlc.narg(is_active), is_active),
    avatar_url = COALESCE(sqlc.narg(avatar_url), avatar_url),
    whatsapp_notifications_enabled = COALESCE(sqlc.narg(whatsapp_notifications_enabled), whatsapp_notifications_enabled),
    terms_agreed_at = COALESCE(sqlc.narg(terms_agreed_at), terms_agreed_at),
    modified_at = NOW()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: UpdateUserPracticeID :one
UPDATE users
SET practice_id = sqlc.arg(practice_id)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteUser :one
UPDATE users
SET deleted_at = NOW()
WHERE id = sqlc.arg(id)
RETURNING *;
