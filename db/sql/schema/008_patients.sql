-- +goose Up
CREATE TABLE patients (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    practice_id UUID NOT NULL,
    first_name TEXT NOT NULL,
    last_name TEXT NOT NULL,
    email TEXT,
    mobile_phone TEXT,
    home_phone TEXT,
    date_of_birth DATE,
    address_line_1 TEXT,
    address_line_2 TEXT,
    city TEXT,
    email_consent BOOLEAN NOT NULL DEFAULT TRUE,
    whatsapp_consent BOOLEAN NOT NULL DEFAULT TRUE,
    emergency_contact_name TEXT,
    emergency_contact_phone TEXT,
    notes TEXT,
    CONSTRAINT patients_practice_id_fkey FOREIGN KEY (practice_id) REFERENCES practices(id)
);

-- +goose Down
DROP TABLE patients;
