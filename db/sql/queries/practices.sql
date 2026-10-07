-- name: GetPractices :many
SELECT * FROM practices;

-- name: GetPractice :one
SELECT p.*, s.*
FROM practices p
LEFT JOIN practice_settings s ON s.practice_id = p.id
WHERE p.id = sqlc.arg(id);

-- name: GetPracticeByCode :one
SELECT * FROM practices
WHERE practice_code = sqlc.arg(practice_code);

-- name: GetPracticeSettingsOverview :one
SELECT id, name, practice_category, specialty, practice_code, city
FROM practices
WHERE id = sqlc.arg(id);

-- name: GetPracticeProfile :one
SELECT id, name, logo, city, street_address, phone, email, website,
       practice_code, instagram, facebook, has_multiple_providers,
       practice_category, specialty
FROM practices
WHERE id = sqlc.arg(id);

-- name: CreatePractice :one
INSERT INTO practices (name, city, phone, email, practice_code, logo, street_address, facebook, instagram, website, has_multiple_providers, specialty, is_suspended, practice_category)
VALUES (
    sqlc.arg(name),
    sqlc.arg(city),
    sqlc.narg(phone),
    sqlc.narg(email),
    sqlc.arg(practice_code),
    sqlc.narg(logo),
    sqlc.narg(street_address),
    sqlc.narg(facebook),
    sqlc.narg(instagram),
    sqlc.narg(website),
    sqlc.arg(has_multiple_providers),
    sqlc.narg(specialty),
    sqlc.arg(is_suspended),
    sqlc.arg(practice_category)
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
    has_multiple_providers = COALESCE(sqlc.narg(has_multiple_providers), has_multiple_providers),
    specialty = COALESCE(sqlc.narg(specialty), specialty),
    is_suspended = COALESCE(sqlc.narg(is_suspended), is_suspended),
    practice_category = COALESCE(sqlc.narg(practice_category), practice_category),
    is_active = COALESCE(sqlc.narg(is_active), is_active),
    modified_at = NOW()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: PatchPracticeSettingsPractice :execrows
UPDATE practices
SET
    specialty = CASE
        WHEN sqlc.arg(set_specialty)::boolean THEN sqlc.narg(specialty)::text
        ELSE specialty
    END,
    has_multiple_providers = CASE
        WHEN sqlc.arg(set_has_multiple_providers)::boolean THEN sqlc.arg(has_multiple_providers)::boolean
        ELSE has_multiple_providers
    END,
    modified_at = NOW()
WHERE id = sqlc.arg(id);

-- name: UpdatePracticeProfile :execrows
UPDATE practices
SET
    name = sqlc.arg(name),
    street_address = sqlc.arg(street_address),
    city = sqlc.arg(city),
    phone = sqlc.arg(phone),
    email = sqlc.arg(email),
    website = sqlc.arg(website),
    practice_code = sqlc.arg(practice_code),
    facebook = sqlc.arg(facebook),
    instagram = sqlc.arg(instagram),
    specialty = sqlc.arg(specialty),
    has_multiple_providers = sqlc.arg(has_multiple_providers),
    logo = CASE
        WHEN sqlc.arg(set_logo)::boolean THEN sqlc.narg(logo)::text
        ELSE logo
    END,
    modified_at = NOW()
WHERE id = sqlc.arg(id);

-- name: UpdatePracticeCode :execrows
UPDATE practices
SET practice_code = sqlc.arg(practice_code), modified_at = NOW()
WHERE id = sqlc.arg(id);
