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

-- name: GetUserAuthorization :one
SELECT u.id, u.email, u.first_name, u.last_name, u.practice_id,
       u.role, u.is_active, u.deleted_at, p.is_suspended
FROM users u
JOIN practices p ON p.id = u.practice_id
WHERE u.id = sqlc.arg(id);

-- name: GetUserWithSubscriptionStatus :one
SELECT
    u.*,
    p.is_suspended AS practice_is_suspended,
    s.status AS subscription_status,
    s."trialEnd" AS subscription_trial_end,
    s."periodEnd" AS subscription_period_end,
    s.plan AS subscription_plan,
    s."cancelAt" AS subscription_cancel_at
FROM users u
LEFT JOIN practices p ON p.id = u.practice_id
LEFT JOIN subscription s ON s."referenceId" = u.practice_id::text
WHERE u.id = sqlc.arg(id);

-- name: GetCurrentUserContext :one
SELECT
    u.id,
    u.email,
    u.first_name,
    u.last_name,
    u.avatar_url,
    u.practice_id,
    u.role,
    u.is_active,
    u.deleted_at,
    p.id AS context_practice_id,
    p.name AS practice_name,
    p.logo AS practice_logo,
    p.city AS practice_city,
    p.street_address AS practice_street_address,
    p.phone AS practice_phone,
    p.email AS practice_email,
    p.website AS practice_website,
    p.practice_code,
    p.instagram AS practice_instagram,
    p.facebook AS practice_facebook,
    p.has_multiple_providers,
    p.practice_category,
    p.specialty AS practice_specialty,
    p.is_suspended,
    s.status AS subscription_status,
    s.plan AS subscription_plan,
    s."trialEnd" AS subscription_trial_end,
    s."periodEnd" AS subscription_period_end,
    s."cancelAt" AS subscription_cancel_at
FROM users u
LEFT JOIN practices p ON p.id = u.practice_id
LEFT JOIN subscription s ON s."referenceId" = u.practice_id::text
WHERE u.id = sqlc.arg(id);

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
