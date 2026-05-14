-- +goose Up
CREATE TABLE patient_registrations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    practice_id UUID NOT NULL,
    appointment_id UUID,
    patient_id UUID,
    patient_name TEXT NOT NULL,
    patient_email TEXT NOT NULL,
    token TEXT NOT NULL UNIQUE,
    token_expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status = ANY (ARRAY['pending', 'completed', 'expired'])),
    sent_by_user_id UUID NOT NULL,
    sent_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    deleted_at TIMESTAMP WITH TIME ZONE,
    CONSTRAINT patient_registrations_practice_id_fkey FOREIGN KEY (practice_id) REFERENCES practices(id),
    CONSTRAINT patient_registrations_appointment_id_fkey FOREIGN KEY (appointment_id) REFERENCES appointments(id),
    CONSTRAINT patient_registrations_sent_by_user_id_fkey FOREIGN KEY (sent_by_user_id) REFERENCES users(id),
    CONSTRAINT patient_registrations_patient_id_fkey FOREIGN KEY (patient_id) REFERENCES patients(id)
);

-- +goose Down
DROP TABLE patient_registrations;
