-- name: GetAppointments :many
SELECT * FROM appointments
WHERE practice_id = sqlc.arg(practice_id)
ORDER BY created_at DESC, id DESC;

-- name: GetAppointmentById :one
SELECT * FROM appointments
WHERE id = sqlc.arg(id) AND practice_id = sqlc.arg(practice_id);

-- name: GetConfirmedAppointments :many
SELECT * FROM appointments
WHERE practice_id = sqlc.arg(practice_id)
  AND is_scheduled IS TRUE
  AND is_cancelled IS FALSE
  AND scheduled_date >= sqlc.arg(start_date)
  AND scheduled_date <= sqlc.arg(end_date)
ORDER BY scheduled_date ASC, scheduled_time ASC, id ASC;

-- name: PatchAppointmentContacts :execrows
UPDATE appointments
SET email = CASE WHEN sqlc.arg(set_email)::boolean THEN sqlc.arg(email)::text ELSE email END,
    mobile_phone = CASE WHEN sqlc.arg(set_mobile_phone)::boolean THEN sqlc.arg(mobile_phone)::text ELSE mobile_phone END,
    modified_at = NOW()
WHERE id = sqlc.arg(id)
  AND practice_id = sqlc.arg(practice_id)
  AND deleted_at IS NULL;

-- name: SoftDeleteAppointment :one
UPDATE appointments
SET deleted_at = NOW()
WHERE id = sqlc.arg(id)
  AND practice_id = sqlc.arg(practice_id)
  AND is_scheduled IS FALSE
  AND is_cancelled IS FALSE
  AND deleted_at IS NULL
RETURNING *;

-- name: LockAppointmentForCancellation :one
SELECT * FROM appointments
WHERE id = sqlc.arg(id)
  AND practice_id = sqlc.arg(practice_id)
  AND deleted_at IS NULL
FOR UPDATE;

-- name: CancelAppointment :one
UPDATE appointments
SET is_cancelled = TRUE,
    modified_at = NOW()
WHERE id = sqlc.arg(id)
  AND practice_id = sqlc.arg(practice_id)
  AND is_cancelled IS FALSE
  AND deleted_at IS NULL
RETURNING *;

-- name: GetPracticeProviderLink :one
SELECT id FROM practice_provider
WHERE practice_id = sqlc.arg(practice_id)
  AND provider_id = sqlc.arg(provider_id)
FOR SHARE;

-- name: GetPracticeAppointmentID :one
SELECT id FROM appointments
WHERE id = sqlc.arg(id) AND practice_id = sqlc.arg(practice_id);

-- name: ListProviderBusyAppointments :many
SELECT id, first_name, last_name, appointment_type,
       scheduled_date, scheduled_time, duration_minutes
FROM appointments
WHERE practice_id = sqlc.arg(practice_id)
  AND provider_id = sqlc.arg(provider_id)
  AND is_scheduled IS TRUE
  AND is_cancelled IS NOT TRUE
  AND deleted_at IS NULL
  AND scheduled_date >= sqlc.arg(start_date)
  AND scheduled_date <= sqlc.arg(end_date)
  AND scheduled_time IS NOT NULL
  AND (sqlc.narg(exclude_appointment_id)::uuid IS NULL OR id <> sqlc.narg(exclude_appointment_id)::uuid)
ORDER BY scheduled_date ASC, scheduled_time ASC, id ASC;

-- name: LockAppointmentForScheduling :one
SELECT id, is_scheduled, practice_id, first_name, last_name, appointment_type,
       mobile_phone, location_id, is_cancelled
FROM appointments
WHERE id = sqlc.arg(id)
  AND practice_id = sqlc.arg(practice_id)
  AND deleted_at IS NULL
FOR UPDATE;

-- name: GetSchedulingSettings :one
SELECT multiple_locations_enabled, available_weekdays
FROM practice_settings
WHERE practice_id = sqlc.arg(practice_id);

-- name: GetSchedulingLocation :one
SELECT available_weekdays
FROM practice_locations
WHERE id = sqlc.arg(id)
  AND practice_id = sqlc.arg(practice_id)
  AND is_active IS TRUE
  AND deleted_at IS NULL
FOR SHARE;

-- name: ListSchedulingConflicts :many
SELECT id, first_name, last_name, appointment_type,
       scheduled_date::text AS scheduled_date,
       to_char(scheduled_time, 'HH24:MI:SS') AS start_time,
       to_char(
         scheduled_time + make_interval(mins => COALESCE(duration_minutes, 15)),
         'HH24:MI:SS'
       ) AS end_time
FROM appointments
WHERE practice_id = sqlc.arg(practice_id)
  AND provider_id = sqlc.arg(provider_id)
  AND (sqlc.narg(exclude_appointment_id)::uuid IS NULL OR id <> sqlc.narg(exclude_appointment_id)::uuid)
  AND is_scheduled IS TRUE
  AND is_cancelled IS NOT TRUE
  AND deleted_at IS NULL
  AND scheduled_date = sqlc.arg(scheduled_date)::date
  AND scheduled_time < sqlc.arg(scheduled_time)::time + make_interval(mins => sqlc.arg(duration_minutes)::int)
  AND scheduled_time + make_interval(mins => COALESCE(duration_minutes, 15)) > sqlc.arg(scheduled_time)::time
ORDER BY scheduled_time ASC, id ASC;

-- name: ScheduleAppointment :one
UPDATE appointments
SET is_scheduled = TRUE,
    is_confirmed = sqlc.arg(is_confirmed),
    scheduled_date = sqlc.arg(scheduled_date)::date,
    scheduled_time = sqlc.arg(scheduled_time)::time,
    provider_id = sqlc.arg(provider_id),
    location_id = CASE WHEN sqlc.arg(set_location)::boolean THEN sqlc.narg(location_id)::uuid ELSE location_id END,
    appointment_type = CASE WHEN sqlc.arg(set_appointment_type)::boolean THEN sqlc.narg(appointment_type)::text ELSE appointment_type END,
    duration_minutes = sqlc.arg(duration_minutes),
    scheduled_by = sqlc.arg(scheduled_by),
    modified_at = NOW()
WHERE id = sqlc.arg(id) AND practice_id = sqlc.arg(practice_id)
RETURNING *;

-- name: LockPatientForStaffAppointment :one
SELECT id, first_name, last_name, email, mobile_phone
FROM patients
WHERE id = sqlc.arg(id) AND practice_id = sqlc.arg(practice_id)
FOR UPDATE;

-- name: FindOtherPatientByContact :one
SELECT id
FROM patients
WHERE practice_id = sqlc.arg(practice_id)
  AND id <> sqlc.arg(id)
  AND ((sqlc.narg(email)::text IS NOT NULL AND lower(trim(email)) = sqlc.narg(email)::text)
    OR (sqlc.narg(mobile_phone)::text IS NOT NULL
      AND regexp_replace(mobile_phone, '\D', '', 'g') = sqlc.narg(mobile_phone)::text))
LIMIT 1
FOR SHARE;

-- name: UpdateStaffAppointmentPatient :one
UPDATE patients
SET email = sqlc.narg(email), mobile_phone = sqlc.arg(mobile_phone), updated_at = NOW()
WHERE id = sqlc.arg(id) AND practice_id = sqlc.arg(practice_id)
RETURNING id, first_name, last_name, email, mobile_phone;

-- name: FindPatientByContact :one
SELECT id, first_name, last_name, email, mobile_phone
FROM patients
WHERE practice_id = sqlc.arg(practice_id)
  AND ((sqlc.narg(email)::text IS NOT NULL AND lower(trim(email)) = sqlc.narg(email)::text)
    OR regexp_replace(mobile_phone, '\D', '', 'g') = sqlc.arg(mobile_phone)::text)
ORDER BY created_at ASC, id ASC
LIMIT 1
FOR SHARE;

-- name: CreateStaffAppointmentPatient :one
INSERT INTO patients (practice_id, first_name, last_name, email, mobile_phone)
VALUES (sqlc.arg(practice_id), sqlc.arg(first_name), sqlc.arg(last_name), sqlc.narg(email), sqlc.arg(mobile_phone))
RETURNING id, first_name, last_name, email, mobile_phone;

-- name: CreateStaffAppointment :one
INSERT INTO appointments (
    practice_id, patient_id, first_name, last_name, email, mobile_phone,
    appointment_type, provider_id, location_id, duration_minutes,
    scheduled_date, scheduled_time, is_scheduled, is_cancelled,
    is_confirmed, is_emergency, created_by, scheduled_by
) VALUES (
    sqlc.arg(practice_id), sqlc.arg(patient_id), sqlc.arg(first_name), sqlc.arg(last_name),
    sqlc.arg(email), sqlc.arg(mobile_phone), sqlc.arg(appointment_type), sqlc.arg(provider_id),
    sqlc.narg(location_id), sqlc.arg(duration_minutes), sqlc.arg(scheduled_date)::date,
    sqlc.arg(scheduled_time)::time, TRUE, FALSE, sqlc.arg(is_confirmed), FALSE,
    sqlc.arg(created_by), sqlc.arg(scheduled_by)
)
RETURNING *;

-- name: ConfirmAppointment :one
UPDATE appointments
SET is_confirmed = TRUE,
    email = CASE WHEN sqlc.arg(set_email)::boolean THEN sqlc.arg(email)::text ELSE email END,
    mobile_phone = CASE WHEN sqlc.arg(set_mobile_phone)::boolean THEN sqlc.arg(mobile_phone)::text ELSE mobile_phone END,
    modified_at = NOW()
WHERE id = sqlc.arg(id)
  AND practice_id = sqlc.arg(practice_id)
  AND is_scheduled IS TRUE
  AND is_confirmed IS FALSE
  AND is_cancelled IS FALSE
  AND deleted_at IS NULL
RETURNING *;
