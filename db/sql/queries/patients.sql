-- name: ListPatients :many
SELECT id, first_name, last_name, email, mobile_phone, date_of_birth, created_at
FROM patients
WHERE practice_id = sqlc.arg(practice_id)
  AND (
    sqlc.arg(search)::text = ''
    OR first_name ILIKE '%' || sqlc.arg(search)::text || '%'
    OR last_name ILIKE '%' || sqlc.arg(search)::text || '%'
    OR (first_name || ' ' || last_name) ILIKE '%' || sqlc.arg(search)::text || '%'
    OR (last_name || ' ' || first_name) ILIKE '%' || sqlc.arg(search)::text || '%'
    OR email ILIKE '%' || sqlc.arg(search)::text || '%'
    OR mobile_phone ILIKE '%' || sqlc.arg(search)::text || '%'
  )
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(result_limit);

-- name: GetPatient :one
SELECT * FROM patients
WHERE id = sqlc.arg(id) AND practice_id = sqlc.arg(practice_id);

-- name: FindDuplicatePatientEmail :one
SELECT id FROM patients
WHERE practice_id = sqlc.arg(practice_id)
  AND id <> sqlc.arg(id)
  AND email = sqlc.arg(email)
LIMIT 1;

-- name: FindDuplicatePatientMobilePhone :one
SELECT id FROM patients
WHERE practice_id = sqlc.arg(practice_id)
  AND id <> sqlc.arg(id)
  AND mobile_phone = sqlc.arg(mobile_phone)
LIMIT 1;

-- name: UpdatePatient :one
UPDATE patients
SET
    first_name = CASE WHEN sqlc.arg(set_first_name)::boolean THEN sqlc.arg(first_name)::text ELSE first_name END,
    last_name = CASE WHEN sqlc.arg(set_last_name)::boolean THEN sqlc.arg(last_name)::text ELSE last_name END,
    email = CASE WHEN sqlc.arg(set_email)::boolean THEN sqlc.narg(email)::text ELSE email END,
    mobile_phone = CASE WHEN sqlc.arg(set_mobile_phone)::boolean THEN sqlc.narg(mobile_phone)::text ELSE mobile_phone END,
    home_phone = CASE WHEN sqlc.arg(set_home_phone)::boolean THEN sqlc.narg(home_phone)::text ELSE home_phone END,
    date_of_birth = CASE WHEN sqlc.arg(set_date_of_birth)::boolean THEN sqlc.narg(date_of_birth)::date ELSE date_of_birth END,
    address_line_1 = CASE WHEN sqlc.arg(set_address_line_1)::boolean THEN sqlc.narg(address_line_1)::text ELSE address_line_1 END,
    address_line_2 = CASE WHEN sqlc.arg(set_address_line_2)::boolean THEN sqlc.narg(address_line_2)::text ELSE address_line_2 END,
    city = CASE WHEN sqlc.arg(set_city)::boolean THEN sqlc.narg(city)::text ELSE city END,
    email_consent = CASE WHEN sqlc.arg(set_email_consent)::boolean THEN sqlc.arg(email_consent)::boolean ELSE email_consent END,
    whatsapp_consent = CASE WHEN sqlc.arg(set_whatsapp_consent)::boolean THEN sqlc.arg(whatsapp_consent)::boolean ELSE whatsapp_consent END,
    emergency_contact_name = CASE WHEN sqlc.arg(set_emergency_contact_name)::boolean THEN sqlc.narg(emergency_contact_name)::text ELSE emergency_contact_name END,
    emergency_contact_phone = CASE WHEN sqlc.arg(set_emergency_contact_phone)::boolean THEN sqlc.narg(emergency_contact_phone)::text ELSE emergency_contact_phone END,
    notes = CASE WHEN sqlc.arg(set_notes)::boolean THEN sqlc.narg(notes)::text ELSE notes END,
    updated_at = NOW()
WHERE id = sqlc.arg(id) AND practice_id = sqlc.arg(practice_id)
RETURNING *;
