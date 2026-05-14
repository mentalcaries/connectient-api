-- +goose Up
CREATE TABLE patient_registration_data (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    submitted_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    registration_id UUID NOT NULL,
    form_version TEXT NOT NULL,
    form_data JSONB NOT NULL,
    CONSTRAINT patient_registration_data_registration_id_fkey FOREIGN KEY (registration_id) REFERENCES patient_registrations(id)
);

-- +goose Down
DROP TABLE patient_registration_data;
