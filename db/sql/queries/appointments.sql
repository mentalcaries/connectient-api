-- name: GetAppointments :many

SELECT * FROM appointments;

-- name: CreateAppointment :one
INSERT INTO appointments (id, created_at, first_name, last_name, email, mobile_phone, requested_date, is_emergency, description, appointment_type, is_scheduled, scheduled_date, created_by, scheduled_by, is_cancelled, requested_time, scheduled_time, practice_id, token)
VALUES (
    gen_random_uuid(), 
    NOW(),
    sqlc.arg(first_name), 
    sqlc.arg(last_name), 
    sqlc.arg(email), 
    sqlc.arg(mobile_phone),
    sqlc.arg(requested_date), 
    sqlc.arg(is_emergency), 
    sqlc.arg(description), 
    sqlc.arg(appointment_type), 
    sqlc.arg(is_scheduled), 
    sqlc.arg(scheduled_date), 
    sqlc.arg(created_by), 
    sqlc.arg(scheduled_by), 
    sqlc.arg(is_cancelled), 
    sqlc.arg(requested_time), 
    sqlc.arg(scheduled_time), 
    sqlc.arg(practice_id), 
    sqlc.arg(token)
)
RETURNING id, created_at, first_name, last_name, email, mobile_phone, requested_date, is_emergency, description, appointment_type, requested_time, practice_id, modified_at, token;

-- name: GetAppointmentById :one
SELECT * FROM appointments WHERE id = $1;

-- name: UpdateAppointment :one
UPDATE appointments
SET scheduled_date = COALESCE(sqlc.narg(scheduled_date), scheduled_date),
    scheduled_time = COALESCE(sqlc.narg(scheduled_time), scheduled_time),
    is_scheduled = COALESCE(sqlc.narg(is_scheduled), is_scheduled),
    is_cancelled = COALESCE(sqlc.narg(is_cancelled), is_cancelled),
    modified_at = NOW()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: ManageAppointmentByToken :one

UPDATE appointments
SET requested_date = COALESCE(sqlc.narg(requested_date), requested_date),
    requested_time = COALESCE(sqlc.narg(requested_time), requested_time),
    is_cancelled = COALESCE(sqlc.narg(is_cancelled), is_cancelled),
    is_emergency = COALESCE(sqlc.narg(is_emergency)), 
    description = COALESCE(sqlc.narg(description)), 
    appointment_type = COALESCE(sqlc.narg(appointment_type)), 
    modified_at = NOW()
WHERE token = sqlc.arg(token)
RETURNING *;

-- name: DeleteAppointment :one
DELETE FROM appointments
WHERE id = $1
RETURNING id;