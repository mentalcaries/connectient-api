-- +goose Up
ALTER TABLE appointments
    ADD CONSTRAINT appointments_scheduled_by_fkey FOREIGN KEY (scheduled_by) REFERENCES users(id),
    ADD CONSTRAINT appointments_provider_id_fkey FOREIGN KEY (provider_id) REFERENCES provider(id),
    ADD CONSTRAINT appointments_location_id_fkey FOREIGN KEY (location_id) REFERENCES practice_locations(id),
    ADD CONSTRAINT appointments_patient_id_fkey FOREIGN KEY (patient_id) REFERENCES patients(id);

-- +goose Down
ALTER TABLE appointments
    DROP CONSTRAINT IF EXISTS appointments_scheduled_by_fkey,
    DROP CONSTRAINT IF EXISTS appointments_provider_id_fkey,
    DROP CONSTRAINT IF EXISTS appointments_location_id_fkey,
    DROP CONSTRAINT IF EXISTS appointments_patient_id_fkey;
