-- +goose Up
CREATE TABLE appointment_calendar_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    appointment_id UUID NOT NULL,
    provider TEXT NOT NULL DEFAULT 'google_calendar',
    external_event_id TEXT NOT NULL,
    CONSTRAINT appointment_calendar_events_appointment_id_fkey FOREIGN KEY (appointment_id) REFERENCES appointments(id)
);

-- +goose Down
DROP TABLE appointment_calendar_events;
