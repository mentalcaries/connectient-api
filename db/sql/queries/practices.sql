-- name: GetPractices :many
SELECT * FROM practices;

-- name: GetPractice :one
SELECT * FROM practices
WHERE ID = sqlc.arg(id);

-- name: CreatePractice :one
INSERT INTO practices (name, city, phone, email, practice_code, logo, street_address, facebook, instagram, website)
VALUES (
    sqlc.arg(name),
    sqlc.arg(city),
    sqlc.arg(phone),
    sqlc.arg(email),
    sqlc.narg(practice_code),
    sqlc.narg(logo),
    sqlc.narg(street_address),
    sqlc.narg(facebook),
    sqlc.narg(instagram),
    sqlc.narg(website)
)
RETURNING *; 

-- name: UpdatePractice :one
UPDATE practices
SET
    name = COALESCE(sqlc.narg(name), name),
    city = COALESCE(sqlc.narg(city), city),
    phone = COALESCE(sqlc.narg(phone), phone),
    email = COALESCE(sqlc.narg(email), email),
    practice_code = COALESCE(sqlc.narg(practice_code), practice_code),
    logo = COALESCE(sqlc.narg(logo), logo),
    street_address = COALESCE(sqlc.narg(street_address), street_address),
    facebook = COALESCE(sqlc.narg(facebook), facebook),
    instagram = COALESCE(sqlc.narg(instagram), instagram),
    website = COALESCE(sqlc.narg(website), website),
    is_active = COALESCE(sqlc.narg(is_active), is_active),
    modified_at = NOW()
WHERE id = sqlc.arg(id)
RETURNING *;