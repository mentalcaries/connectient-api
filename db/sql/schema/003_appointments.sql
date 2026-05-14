-- +goose Up
CREATE TABLE appointments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    modified_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    first_name VARCHAR NOT NULL,
    last_name VARCHAR NOT NULL,
    email VARCHAR NOT NULL,
    mobile_phone VARCHAR NOT NULL,
    requested_date DATE,
    requested_time TEXT DEFAULT '',
    is_emergency BOOLEAN DEFAULT FALSE,
    description TEXT,
    appointment_type TEXT,
    is_scheduled BOOLEAN DEFAULT FALSE,
    scheduled_date DATE,
    scheduled_time TIME WITHOUT TIME ZONE,
    is_cancelled BOOLEAN DEFAULT FALSE,
    duration_minutes INTEGER,
    created_by UUID,
    scheduled_by UUID,
    practice_id UUID,
    provider_id UUID,
    location_id UUID,
    patient_id UUID,
    token TEXT NOT NULL UNIQUE,
    CONSTRAINT appointments_practice_id_fkey FOREIGN KEY (practice_id) REFERENCES practices(id)
);

CREATE UNIQUE INDEX IF NOT EXISTS appt_token ON appointments(token);

-- +goose Down
DROP TABLE appointments;