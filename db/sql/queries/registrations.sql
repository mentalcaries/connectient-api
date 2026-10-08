-- name: ListRegistrations :many
SELECT id, patient_name, patient_email, patient_phone, status, sent_at,
       completed_at, appointment_id, created_at, token
FROM patient_registrations
WHERE practice_id = sqlc.arg(practice_id)
  AND deleted_at IS NULL
  AND (sqlc.arg(search)::text = '' OR patient_name ILIKE '%' || sqlc.arg(search)::text || '%')
  AND (sqlc.narg(appointment_id)::uuid IS NULL OR appointment_id = sqlc.narg(appointment_id)::uuid)
ORDER BY created_at DESC, id DESC;

-- name: GetRegistrationDetail :one
SELECT r.id, r.patient_name, r.patient_email, r.patient_phone, r.status,
       r.sent_at, r.completed_at, r.appointment_id, r.created_at,
       d.form_data, d.form_version
FROM patient_registrations r
LEFT JOIN patient_registration_data d ON d.registration_id = r.id
WHERE r.id = sqlc.arg(id)
  AND r.practice_id = sqlc.arg(practice_id)
  AND r.deleted_at IS NULL;

-- name: GetRegistrationForLink :one
SELECT id, patient_name, patient_email, patient_phone, status, token,
       token_expires_at, sent_at, appointment_id
FROM patient_registrations
WHERE id = sqlc.arg(id)
  AND practice_id = sqlc.arg(practice_id)
  AND deleted_at IS NULL;

-- name: GetRegistrationDuplicate :one
SELECT id
FROM patient_registrations
WHERE practice_id = sqlc.arg(practice_id)
  AND patient_name = sqlc.arg(patient_name)
  AND (
    (sqlc.narg(patient_email)::text IS NOT NULL AND patient_email = sqlc.narg(patient_email)::text)
    OR (sqlc.narg(patient_phone)::text IS NOT NULL AND patient_phone = sqlc.narg(patient_phone)::text)
  )
LIMIT 1;

-- name: GetRegistrationAppointment :one
SELECT id, first_name, email, mobile_phone
FROM appointments
WHERE id = sqlc.arg(id)
  AND practice_id = sqlc.arg(practice_id)
  AND deleted_at IS NULL;

-- name: CreatePatientRegistration :one
INSERT INTO patient_registrations (
    practice_id, appointment_id, patient_name, patient_email, patient_phone,
    token, token_expires_at, status, sent_by_user_id, sent_at
) VALUES (
    sqlc.arg(practice_id), sqlc.narg(appointment_id), sqlc.arg(patient_name),
    sqlc.narg(patient_email), sqlc.narg(patient_phone), sqlc.arg(token),
    sqlc.arg(token_expires_at), 'pending', sqlc.arg(sent_by_user_id), NULL
)
RETURNING id, token, token_expires_at, status;

-- name: MarkRegistrationSent :execrows
UPDATE patient_registrations
SET sent_at = NOW()
WHERE id = sqlc.arg(id)
  AND practice_id = sqlc.arg(practice_id)
  AND deleted_at IS NULL
  AND status <> 'completed';

-- name: UpdateRegistrationEmail :execrows
UPDATE patient_registrations
SET patient_email = sqlc.arg(patient_email)
WHERE id = sqlc.arg(id)
  AND practice_id = sqlc.arg(practice_id)
  AND deleted_at IS NULL
  AND status <> 'completed';

-- name: RestoreRegistrationEmail :execrows
UPDATE patient_registrations
SET patient_email = sqlc.narg(previous_email)
WHERE id = sqlc.arg(id)
  AND practice_id = sqlc.arg(practice_id)
  AND patient_email = sqlc.arg(expected_email);

-- name: UpdateRegistrationPhone :execrows
UPDATE patient_registrations
SET patient_phone = sqlc.arg(patient_phone)
WHERE id = sqlc.arg(id)
  AND practice_id = sqlc.arg(practice_id)
  AND deleted_at IS NULL
  AND status <> 'completed';

-- name: RestoreRegistrationPhone :execrows
UPDATE patient_registrations
SET patient_phone = sqlc.narg(previous_phone)
WHERE id = sqlc.arg(id)
  AND practice_id = sqlc.arg(practice_id)
  AND patient_phone = sqlc.arg(expected_phone);

-- name: RotateRegistrationToken :one
UPDATE patient_registrations
SET token = sqlc.arg(token), token_expires_at = sqlc.arg(token_expires_at),
    status = 'pending'
WHERE id = sqlc.arg(id)
  AND practice_id = sqlc.arg(practice_id)
  AND deleted_at IS NULL
  AND status <> 'completed'
RETURNING token, token_expires_at, status;

-- name: RestoreRegistrationToken :execrows
UPDATE patient_registrations
SET token = sqlc.arg(previous_token),
    token_expires_at = sqlc.arg(previous_token_expires_at),
    status = sqlc.arg(previous_status),
    sent_at = sqlc.narg(previous_sent_at)
WHERE id = sqlc.arg(id)
  AND practice_id = sqlc.arg(practice_id)
  AND token = sqlc.arg(expected_token);

-- name: SoftDeleteRegistration :one
UPDATE patient_registrations
SET deleted_at = NOW()
WHERE id = sqlc.arg(id)
  AND practice_id = sqlc.arg(practice_id)
  AND deleted_at IS NULL
  AND status <> 'completed'
RETURNING id;

-- name: GetRegistrationDeleteState :one
SELECT status
FROM patient_registrations
WHERE id = sqlc.arg(id)
  AND practice_id = sqlc.arg(practice_id)
  AND deleted_at IS NULL;

-- name: GetPublicRegistrationBootstrap :one
SELECT
    r.id, r.practice_id, r.appointment_id, r.status, r.token_expires_at,
    p.name AS practice_name, p.logo AS practice_logo,
    p.practice_category, p.is_active, p.is_suspended,
    ps.dental_history_enabled, ps.tmj_history_enabled,
    ps.physiotherapy_history_enabled, ps.optometry_history_enabled,
    ps.custom_form_sections, ps.theme, ps.theme_colors,
    a.first_name, a.last_name, a.email, a.mobile_phone,
    s.status AS subscription_status, s.plan AS subscription_plan,
    s."trialEnd" AS subscription_trial_end,
    s."periodEnd" AS subscription_period_end,
    s."cancelAt" AS subscription_cancel_at
FROM patient_registrations r
JOIN practices p ON p.id = r.practice_id
LEFT JOIN practice_settings ps ON ps.practice_id = r.practice_id
LEFT JOIN appointments a
  ON a.id = r.appointment_id AND a.practice_id = r.practice_id AND a.deleted_at IS NULL
LEFT JOIN subscription s ON s."referenceId" = r.practice_id::text
WHERE r.token = sqlc.arg(token)
  AND r.deleted_at IS NULL;

-- name: MarkRegistrationExpired :execrows
UPDATE patient_registrations
SET status = 'expired'
WHERE id = sqlc.arg(id)
  AND deleted_at IS NULL
  AND status <> 'completed';

-- name: LockPublicRegistration :one
SELECT r.id, r.practice_id, r.status, r.token_expires_at,
       p.is_active, p.is_suspended,
       s.status AS subscription_status, s.plan AS subscription_plan,
       s."trialEnd" AS subscription_trial_end,
       s."periodEnd" AS subscription_period_end,
       s."cancelAt" AS subscription_cancel_at
FROM patient_registrations r
JOIN practices p ON p.id = r.practice_id
LEFT JOIN subscription s ON s."referenceId" = r.practice_id::text
WHERE r.token = sqlc.arg(token)
  AND r.deleted_at IS NULL
FOR UPDATE OF r;

-- name: FindRegistrationPatientByEmail :one
SELECT id FROM patients
WHERE practice_id = sqlc.arg(practice_id)
  AND email = sqlc.arg(email)
  AND first_name ILIKE sqlc.arg(first_name)
  AND last_name ILIKE sqlc.arg(last_name)
LIMIT 1;

-- name: FindRegistrationPatientByPhone :one
SELECT id FROM patients
WHERE practice_id = sqlc.arg(practice_id)
  AND mobile_phone = sqlc.arg(mobile_phone)
  AND first_name ILIKE sqlc.arg(first_name)
  AND last_name ILIKE sqlc.arg(last_name)
LIMIT 1;

-- name: CreateRegistrationPatient :one
INSERT INTO patients (
    practice_id, first_name, last_name, email, mobile_phone
) VALUES (
    sqlc.arg(practice_id), sqlc.arg(first_name), sqlc.arg(last_name),
    sqlc.narg(email), sqlc.narg(mobile_phone)
)
RETURNING id;

-- name: EnrichRegistrationPatient :execrows
UPDATE patients
SET home_phone = sqlc.narg(home_phone),
    date_of_birth = sqlc.arg(date_of_birth),
    address_line_1 = sqlc.narg(address_line_1),
    address_line_2 = sqlc.narg(address_line_2),
    city = sqlc.narg(city),
    email_consent = sqlc.arg(email_consent),
    emergency_contact_name = sqlc.narg(emergency_contact_name),
    emergency_contact_phone = sqlc.narg(emergency_contact_phone),
    updated_at = NOW()
WHERE id = sqlc.arg(id)
  AND practice_id = sqlc.arg(practice_id);

-- name: InsertPatientRegistrationData :exec
INSERT INTO patient_registration_data (registration_id, form_version, form_data)
VALUES (sqlc.arg(registration_id), sqlc.arg(form_version), sqlc.arg(form_data));

-- name: CompletePatientRegistration :execrows
UPDATE patient_registrations
SET patient_id = sqlc.arg(patient_id), status = 'completed', completed_at = NOW()
WHERE id = sqlc.arg(id)
  AND status <> 'completed'
  AND deleted_at IS NULL;
