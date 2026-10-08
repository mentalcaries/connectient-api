-- name: GetNotificationPractice :one
SELECT id, name, email, phone, logo
FROM practices
WHERE id = sqlc.arg(id);

-- name: GetNotificationAppointmentContext :one
SELECT p.id AS practice_id, p.name AS practice_name, p.email AS practice_email,
       p.phone AS practice_phone, p.logo AS practice_logo,
       a.first_name, a.last_name, a.email, a.mobile_phone,
       a.appointment_type, a.scheduled_date, a.scheduled_time,
       pr.first_name AS provider_first_name, pr.last_name AS provider_last_name,
       l.name AS location_name, l.address AS location_address
FROM appointments a
JOIN practices p ON p.id = a.practice_id
LEFT JOIN provider pr ON pr.id = a.provider_id
LEFT JOIN practice_locations l ON l.id = a.location_id
WHERE a.id = sqlc.arg(appointment_id)
  AND a.practice_id = sqlc.arg(practice_id);

-- name: ListEligibleNotificationStaffPhones :many
SELECT mobile_phone
FROM users
WHERE practice_id = sqlc.arg(practice_id)
  AND mobile_phone IS NOT NULL
  AND whatsapp_notifications_enabled = TRUE
  AND is_active = TRUE
  AND deleted_at IS NULL
ORDER BY id;

-- name: CreateNotificationLog :exec
INSERT INTO notification_log (practice_id, channel, notification_type)
VALUES (sqlc.arg(practice_id), sqlc.arg(channel), sqlc.arg(notification_type));
