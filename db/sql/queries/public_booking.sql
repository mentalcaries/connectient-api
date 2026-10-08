-- name: GetPublicBookingPractice :one
SELECT
    p.id, p.name, p.logo, p.city, p.street_address, p.phone, p.email, p.website,
    p.practice_code, p.instagram, p.facebook, p.has_multiple_providers,
    p.practice_category, p.specialty, p.is_active, p.is_suspended,
    ps.multiple_locations_enabled, ps.available_weekdays, ps.theme, ps.theme_colors,
    s.status AS subscription_status, s.plan AS subscription_plan,
    s."trialEnd" AS subscription_trial_end,
    s."periodEnd" AS subscription_period_end,
    s."cancelAt" AS subscription_cancel_at
FROM practices p
JOIN practice_settings ps ON ps.practice_id = p.id
LEFT JOIN subscription s ON s."referenceId" = p.id::text
WHERE p.practice_code = sqlc.arg(practice_code);

-- name: GetPublicProcedureTypes :many
SELECT id, name, value, sort_order, is_primary
FROM procedure_types
WHERE practice_id = sqlc.arg(practice_id)
  AND is_active = TRUE
  AND deleted_at IS NULL
ORDER BY sort_order ASC;

-- name: GetPublicProcedureType :one
SELECT value
FROM procedure_types
WHERE practice_id = sqlc.arg(practice_id)
  AND value = sqlc.arg(value)
  AND is_active = TRUE
  AND deleted_at IS NULL;

-- name: GetPublicProvider :one
SELECT pp.provider_id
FROM practice_provider pp
JOIN provider p ON p.id = pp.provider_id
WHERE pp.practice_id = sqlc.arg(practice_id)
  AND pp.provider_id = sqlc.arg(provider_id)
FOR SHARE OF pp;

-- name: GetPublicBookingLocation :one
SELECT available_weekdays
FROM practice_locations
WHERE id = sqlc.arg(id)
  AND practice_id = sqlc.arg(practice_id)
  AND is_active = TRUE
  AND deleted_at IS NULL
FOR SHARE;

-- name: ClaimPublicAppointmentIdempotency :execrows
INSERT INTO public_appointment_request_idempotency (
    practice_id, idempotency_key, request_hash
) VALUES (
    sqlc.arg(practice_id), sqlc.arg(idempotency_key), sqlc.arg(request_hash)
)
ON CONFLICT (practice_id, idempotency_key) DO NOTHING;

-- name: LockPublicAppointmentIdempotency :one
SELECT request_hash, response_body
FROM public_appointment_request_idempotency
WHERE practice_id = sqlc.arg(practice_id)
  AND idempotency_key = sqlc.arg(idempotency_key)
FOR UPDATE;

-- name: CompletePublicAppointmentIdempotency :exec
UPDATE public_appointment_request_idempotency
SET response_body = sqlc.arg(response_body)
WHERE practice_id = sqlc.arg(practice_id)
  AND idempotency_key = sqlc.arg(idempotency_key);

-- name: FindPublicBookingPatient :one
SELECT id
FROM patients
WHERE practice_id = sqlc.arg(practice_id)
  AND lower(first_name) = lower(sqlc.arg(first_name))
  AND lower(last_name) = lower(sqlc.arg(last_name))
  AND (lower(trim(email)) = sqlc.arg(email)
    OR regexp_replace(mobile_phone, '\D', '', 'g') = sqlc.arg(mobile_phone))
ORDER BY created_at ASC, id ASC
LIMIT 1
FOR SHARE;

-- name: PublicBookingContactExists :one
SELECT id
FROM patients
WHERE practice_id = sqlc.arg(practice_id)
  AND (lower(trim(email)) = sqlc.arg(email)
    OR regexp_replace(mobile_phone, '\D', '', 'g') = sqlc.arg(mobile_phone))
LIMIT 1
FOR SHARE;

-- name: CreatePublicBookingPatient :one
INSERT INTO patients (
    practice_id, first_name, last_name, email, mobile_phone
) VALUES (
    sqlc.arg(practice_id), sqlc.arg(first_name), sqlc.arg(last_name),
    sqlc.arg(email), sqlc.arg(mobile_phone)
)
RETURNING id;

-- name: CreatePublicAppointmentRequest :one
INSERT INTO appointments (
    practice_id, patient_id, first_name, last_name, email, mobile_phone,
    requested_date, requested_time, is_emergency, description,
    appointment_type, provider_id, location_id, is_scheduled,
    is_cancelled, is_confirmed
) VALUES (
    sqlc.arg(practice_id), sqlc.narg(patient_id), sqlc.arg(first_name),
    sqlc.arg(last_name), sqlc.arg(email), sqlc.arg(mobile_phone),
    sqlc.arg(requested_date)::date, sqlc.arg(requested_time),
    sqlc.arg(is_emergency), sqlc.narg(description), sqlc.arg(appointment_type),
    sqlc.narg(provider_id), sqlc.narg(location_id), FALSE, FALSE, FALSE
)
RETURNING *;

-- name: GetPublicLocations :many
SELECT id, name, sort_order, available_weekdays
FROM practice_locations
WHERE practice_id = sqlc.arg(practice_id)
  AND is_active = TRUE
  AND deleted_at IS NULL
  AND cardinality(available_weekdays) > 0
ORDER BY sort_order ASC;

-- name: GetPublicProviders :many
SELECT p.id, p.first_name, p.last_name, p.title, p.specialty
FROM practice_provider pp
JOIN provider p ON p.id = pp.provider_id
WHERE pp.practice_id = sqlc.arg(practice_id)
ORDER BY pp.is_main DESC, p.created_at ASC, p.id ASC;
