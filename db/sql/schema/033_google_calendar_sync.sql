-- +goose Up
ALTER TABLE appointments
ADD COLUMN scheduled_timezone TEXT NOT NULL DEFAULT 'America/Port_of_Spain';

ALTER TABLE connected_apps
ADD COLUMN app_calendar_id TEXT;

ALTER TABLE connected_apps
ADD CONSTRAINT connected_apps_practice_provider_unique UNIQUE (practice_id, provider);

ALTER TABLE appointment_calendar_events
ADD CONSTRAINT appointment_calendar_events_appointment_provider_unique UNIQUE (appointment_id, provider);

CREATE TABLE google_oauth_states (
    state TEXT PRIMARY KEY,
    practice_id UUID NOT NULL REFERENCES practices(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE google_oauth_states;

ALTER TABLE appointment_calendar_events
DROP CONSTRAINT appointment_calendar_events_appointment_provider_unique;

ALTER TABLE connected_apps
DROP CONSTRAINT connected_apps_practice_provider_unique,
DROP COLUMN app_calendar_id;

ALTER TABLE appointments
DROP COLUMN scheduled_timezone;
