-- name: GetConnectedApps :many
SELECT provider, connected_account_email, is_connected
FROM connected_apps
WHERE practice_id = sqlc.arg(practice_id)
ORDER BY provider;

-- name: CreateGoogleOAuthState :exec
WITH expired AS (
    DELETE FROM google_oauth_states WHERE expires_at <= NOW()
)
INSERT INTO google_oauth_states (state, practice_id, user_id, expires_at)
VALUES (sqlc.arg(state), sqlc.arg(practice_id), sqlc.arg(user_id), sqlc.arg(expires_at));

-- name: ConsumeGoogleOAuthState :one
DELETE FROM google_oauth_states
WHERE state = sqlc.arg(state) AND expires_at > NOW()
RETURNING practice_id, user_id;

-- name: UpsertGoogleConnection :one
INSERT INTO connected_apps (
    practice_id, provider, connected_account_email, access_token,
    refresh_token, token_expires_at, is_connected, last_error, updated_at
) VALUES (
    sqlc.arg(practice_id), 'google_calendar', sqlc.arg(connected_account_email),
    sqlc.arg(access_token), sqlc.narg(refresh_token), sqlc.arg(token_expires_at),
    TRUE, NULL, NOW()
)
ON CONFLICT (practice_id, provider) DO UPDATE SET
    connected_account_email = EXCLUDED.connected_account_email,
    access_token = EXCLUDED.access_token,
    refresh_token = COALESCE(EXCLUDED.refresh_token, connected_apps.refresh_token),
    token_expires_at = EXCLUDED.token_expires_at,
    is_connected = TRUE,
    last_error = NULL,
    app_calendar_id = CASE
        WHEN connected_apps.connected_account_email IS DISTINCT FROM EXCLUDED.connected_account_email THEN NULL
        ELSE connected_apps.app_calendar_id
    END,
    updated_at = NOW()
RETURNING *;

-- name: GetGoogleConnection :one
SELECT * FROM connected_apps
WHERE practice_id = sqlc.arg(practice_id)
  AND provider = 'google_calendar'
  AND is_connected = TRUE;

-- name: GetGoogleConnectionAny :one
SELECT * FROM connected_apps
WHERE practice_id = sqlc.arg(practice_id)
  AND provider = 'google_calendar';

-- name: UpdateGoogleConnectionTokens :exec
UPDATE connected_apps
SET access_token = sqlc.arg(access_token),
    token_expires_at = sqlc.arg(token_expires_at),
    is_connected = TRUE, last_error = NULL, updated_at = NOW()
WHERE id = sqlc.arg(id);

-- name: MarkGoogleConnectionFailed :exec
UPDATE connected_apps
SET is_connected = FALSE, last_error = sqlc.arg(last_error), updated_at = NOW()
WHERE id = sqlc.arg(id);

-- name: SetGoogleAppCalendar :exec
UPDATE connected_apps
SET app_calendar_id = sqlc.narg(app_calendar_id), updated_at = NOW()
WHERE id = sqlc.arg(id);

-- name: GetAppointmentCalendarMapping :one
SELECT external_event_id
FROM appointment_calendar_events
WHERE appointment_id = sqlc.arg(appointment_id)
  AND provider = 'google_calendar';

-- name: UpsertAppointmentCalendarMapping :exec
INSERT INTO appointment_calendar_events (appointment_id, provider, external_event_id)
VALUES (sqlc.arg(appointment_id), 'google_calendar', sqlc.arg(external_event_id))
ON CONFLICT (appointment_id, provider) DO UPDATE SET
    external_event_id = EXCLUDED.external_event_id,
    updated_at = NOW();

-- name: DeleteAppointmentCalendarMapping :exec
DELETE FROM appointment_calendar_events
WHERE appointment_id = sqlc.arg(appointment_id)
  AND provider = 'google_calendar';

-- name: DeleteGoogleCalendarMappingsForPractice :exec
DELETE FROM appointment_calendar_events e
USING appointments a
WHERE e.appointment_id = a.id
  AND e.provider = 'google_calendar'
  AND a.practice_id = sqlc.arg(practice_id);

-- name: DeleteGoogleConnection :exec
DELETE FROM connected_apps
WHERE practice_id = sqlc.arg(practice_id)
  AND provider = 'google_calendar';

-- name: ListGoogleCalendarBackfillAppointments :many
SELECT a.id, a.first_name, a.last_name, a.mobile_phone, a.appointment_type,
       a.scheduled_date, a.scheduled_time, a.duration_minutes,
       a.scheduled_timezone
FROM appointments a
LEFT JOIN appointment_calendar_events e
  ON e.appointment_id = a.id AND e.provider = 'google_calendar'
WHERE a.practice_id = sqlc.arg(practice_id)
  AND a.is_scheduled = TRUE
  AND a.is_confirmed = TRUE
  AND a.is_cancelled = FALSE
  AND a.deleted_at IS NULL
  AND a.scheduled_date >= CURRENT_DATE
  AND e.id IS NULL
ORDER BY a.scheduled_date, a.scheduled_time, a.id;
