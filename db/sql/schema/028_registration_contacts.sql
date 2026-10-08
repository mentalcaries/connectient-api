-- +goose Up
ALTER TABLE patient_registrations
ALTER COLUMN patient_email DROP NOT NULL,
ADD COLUMN patient_phone TEXT,
ADD CONSTRAINT patient_registrations_contact_required
CHECK (patient_email IS NOT NULL OR patient_phone IS NOT NULL);

CREATE UNIQUE INDEX patient_registrations_practice_email_idx
ON patient_registrations (practice_id, patient_name, patient_email)
WHERE patient_email IS NOT NULL;

CREATE UNIQUE INDEX patient_registrations_practice_phone_idx
ON patient_registrations (practice_id, patient_name, patient_phone)
WHERE patient_phone IS NOT NULL;

-- +goose Down
DROP INDEX patient_registrations_practice_phone_idx;
DROP INDEX patient_registrations_practice_email_idx;

ALTER TABLE patient_registrations
DROP CONSTRAINT patient_registrations_contact_required,
DROP COLUMN patient_phone,
ALTER COLUMN patient_email SET NOT NULL;
