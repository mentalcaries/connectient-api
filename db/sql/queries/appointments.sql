-- name: GetAppointments :many
SELECT * FROM appointments;

-- name: CreateAppointment :one
INSERT INTO appointments (
    first_name,
    last_name,
    email,
    mobile_phone,
    requested_date,
    requested_time,
    is_emergency,
    description,
    appointment_type,
    is_scheduled,
    scheduled_date,
    scheduled_time,
    is_cancelled,
    duration_minutes,
    created_by,
    scheduled_by,
    practice_id,
    provider_id,
    location_id,
    patient_id,
    token
) VALUES (
    sqlc.arg(first_name),
    sqlc.arg(last_name),
    sqlc.arg(email),
    sqlc.arg(mobile_phone),
    sqlc.narg(requested_date),
    sqlc.arg(requested_time),
    sqlc.arg(is_emergency),
    sqlc.narg(description),
    sqlc.narg(appointment_type),
    sqlc.arg(is_scheduled),
    sqlc.narg(scheduled_date),
    sqlc.narg(scheduled_time),
    sqlc.arg(is_cancelled),
    sqlc.narg(duration_minutes),
    sqlc.narg(created_by),
    sqlc.narg(scheduled_by),
    sqlc.narg(practice_id),
    sqlc.narg(provider_id),
    sqlc.narg(location_id),
    sqlc.narg(patient_id),
    sqlc.arg(token)
)
RETURNING *;

-- name: GetAppointmentById :one
SELECT * FROM appointments WHERE id = sqlc.arg(id);

-- name: UpdateAppointment :one
UPDATE appointments
SET
    scheduled_date = COALESCE(sqlc.narg(scheduled_date), scheduled_date),
    scheduled_time = COALESCE(sqlc.narg(scheduled_time), scheduled_time),
    is_scheduled = COALESCE(sqlc.narg(is_scheduled), is_scheduled),
    is_cancelled = COALESCE(sqlc.narg(is_cancelled), is_cancelled),
    provider_id = COALESCE(sqlc.narg(provider_id), provider_id),
    location_id = COALESCE(sqlc.narg(location_id), location_id),
    duration_minutes = COALESCE(sqlc.narg(duration_minutes), duration_minutes),
    modified_at = NOW()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: ManageAppointmentByToken :one
UPDATE appointments
SET
    requested_date = COALESCE(sqlc.narg(requested_date), requested_date),
    requested_time = COALESCE(sqlc.narg(requested_time), requested_time),
    is_cancelled = COALESCE(sqlc.narg(is_cancelled), is_cancelled),
    is_emergency = COALESCE(sqlc.narg(is_emergency), is_emergency),
    description = COALESCE(sqlc.narg(description), description),
    appointment_type = COALESCE(sqlc.narg(appointment_type), appointment_type),
    modified_at = NOW()
WHERE token = sqlc.arg(token)
RETURNING *;

-- name: DeleteAppointment :one
DELETE FROM appointments
WHERE id = sqlc.arg(id)
RETURNING id;
