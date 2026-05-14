-- name: CreateUser :one
INSERT INTO users (
    first_name,
    last_name, 
    email,
    mobile_phone,
    practice_id
) VALUES (
    sqlc.arg(first_name),
    sqlc.arg(last_name),
    sqlc.arg(email),
    sqlc.arg(mobile_phone),
    sqlc.arg(practice_id)
)
RETURNING *; 

-- name: GetAllUsers :many
SELECT * FROM users;

-- name: GetUser :one

SELECT * from users WHERE ID = sqlc.arg(id);

-- name: UpdateUser :one

UPDATE users
SET
    first_name =COALESCE(sqlc.narg(first_name), first_name),
    last_name =COALESCE(sqlc.narg(last_name), last_name),
    email =COALESCE(sqlc.narg(email), email),
    mobile_phone =COALESCE(sqlc.narg(mobile_phone), mobile_phone),
    role = COALESCE(sqlc.narg(role), role),
    is_active = COALESCE(sqlc.narg(is_active), is_active),
    avatar_url = COALESCE(sqlc.narg(avatar_url), avatar_url)
WHERE id = sqlc.arg(id)
RETURNING *; 

-- name: DeleteUser :one
UPDATE users
SET deleted_at = NOW()
WHERE id = sqlc.arg(id)
RETURNING *;