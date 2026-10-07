-- name: ListAppointmentExportRows :many
SELECT a.first_name, a.last_name, a.email, a.mobile_phone,
       a.appointment_type, a.requested_date, a.is_scheduled, a.is_cancelled,
       l.name AS location_name, a.description, a.created_at
FROM appointments a
LEFT JOIN practice_locations l
  ON l.id = a.location_id AND l.practice_id = a.practice_id
WHERE a.practice_id = sqlc.arg(practice_id)
  AND a.deleted_at IS NULL
ORDER BY a.created_at DESC, a.id DESC
LIMIT 10001;

-- name: ListRegistrationExportRows :many
SELECT r.id, r.patient_name, r.patient_email, r.patient_phone,
       r.completed_at, COALESCE(d.form_version, '')::text AS form_version,
       COALESCE(d.form_data, '{}'::jsonb)::jsonb AS form_data
FROM patient_registrations r
LEFT JOIN LATERAL (
    SELECT form_version, form_data
    FROM patient_registration_data
    WHERE registration_id = r.id
    ORDER BY submitted_at DESC, id DESC
    LIMIT 1
) d ON TRUE
WHERE r.practice_id = sqlc.arg(practice_id)
  AND r.status = 'completed'
  AND r.deleted_at IS NULL
ORDER BY r.completed_at DESC NULLS LAST, r.id DESC
LIMIT 1001;

-- name: CreateDataExportAuditLog :exec
INSERT INTO data_export_audit_log (
    practice_id, exported_by_user_id, export_type, row_count
) VALUES (
    sqlc.arg(practice_id), sqlc.arg(exported_by_user_id),
    sqlc.arg(export_type), sqlc.arg(row_count)
);
